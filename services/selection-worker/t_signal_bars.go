package main

import (
	"strconv"
	"strings"
	"time"
)

// MinuteBar 由行情快照合成的1分钟K线(不做复权)。
// 单位:价格为元,成交量为股(快照Volume单位为手,×100换算),成交额为元。
type MinuteBar struct {
	Time     string  `json:"time"` // 分钟起点,格式 2006-01-02T15:04:00+08:00
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`   // 股
	Amount   float64 `json:"amount"`   // 元
	Complete bool    `json:"complete"` // 是否已切bar(分钟结束)
	Trusted  bool    `json:"trusted"`  // 数据可信(无断流/累计量回退)
}

// MinuteBarSnapshot 一条快照的特征,用于急拉急跌等短窗口检测。
type MinuteBarSnapshot struct {
	Time      time.Time
	Price     float64
	Intuition int
	Rate      float64
}

const minuteBarGapThreshold = 30 * time.Second // 断流判定阈值(文档2.3)

// quoteSession 快照所属交易时段。
type quoteSession int

const (
	sessionClosed    quoteSession = iota
	sessionAuction                // 9:15-9:25 集合竞价
	sessionPreOpen                // 9:25-9:30 竞价结果/开盘前
	sessionMorning                // 9:30-11:30
	sessionLunch                  // 11:30-13:00 午休
	sessionAfternoon              // 13:00-15:00
)

func classifyQuoteSession(t time.Time) quoteSession {
	hm := t.Hour()*100 + t.Minute()
	switch {
	case hm >= 915 && hm < 925:
		return sessionAuction
	case hm >= 925 && hm < 930:
		return sessionPreOpen
	case hm >= 930 && hm < 1130:
		return sessionMorning
	case hm >= 1130 && hm < 1300:
		return sessionLunch
	case hm >= 1300 && hm < 1500:
		return sessionAfternoon
	default:
		return sessionClosed
	}
}

func minuteStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}

func minuteStartText(t time.Time) string {
	return minuteStart(t).Format("2006-01-02T15:04:00+08:00")
}

// parseQuoteServerTime 解析快照 ServerTime(整数串)。
// TDX ServerTime 常见格式为 HHMMSS / HHMMSSmmm,可能因前导零缺失缩短(如 93005)。
// 解析失败或与本地时间偏差超过3分钟时回退本地时间,保证分钟归属稳定。
func parseQuoteServerTime(raw string, fallback time.Time) time.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fallback
	}
	var hour, minute, second int
	switch len(s) {
	case 9, 8, 6: // HHMMSSmmm / HHMMSSmm / HHMMSS
		hour, _ = strconv.Atoi(s[0:2])
		minute, _ = strconv.Atoi(s[2:4])
		second, _ = strconv.Atoi(s[4:6])
	case 5: // HMMSS
		hour, _ = strconv.Atoi(s[0:1])
		minute, _ = strconv.Atoi(s[1:3])
		second, _ = strconv.Atoi(s[3:5])
	default:
		return fallback
	}
	if hour > 23 || minute > 59 || second > 59 {
		return fallback
	}
	t := time.Date(fallback.Year(), fallback.Month(), fallback.Day(), hour, minute, second, 0, fallback.Location())
	diff := t.Sub(fallback)
	if diff < -3*time.Minute || diff > 3*time.Minute {
		return fallback
	}
	return t
}

// MinuteBarBuilder 快照流 → 1分钟bar合成器。
// 边界处理对齐 INTRADAY_T_SIGNAL_V1.md 2.3:集合竞价初始化、午休切bar、
// 断流标记不完整、累计量回退标记不可信。
type MinuteBarBuilder struct {
	current   *MinuteBar
	auction   float64 // 9:25 撮合价,用于初始化 9:30 bar 的 open
	barVol    float64 // 当前bar起始累计量(手)
	barAmt    float64 // 当前bar起始累计额(元)
	lastVol   float64 // 最近一条快照累计量(手),用于回退检测
	lastAmt   float64 // 最近一条快照累计额(元),用于VWAP
	lastSeen  time.Time
	gapMarked bool
	lastGap   time.Time
	firstBase bool // 首个合法累计量基准是否建立
	lastQuote time.Time

	completed []MinuteBar         // 最近N根完成bar(指标计算)
	snapshots []MinuteBarSnapshot // 最近90秒快照(急拉急跌检测)
}

func NewMinuteBarBuilder() *MinuteBarBuilder {
	return &MinuteBarBuilder{}
}

// Append 处理一条快照;bar 切分时返回完成的bar(可能为nil)。
// price 为最新价(元),volume 为当日累计成交量(手),amount 为当日累计成交额(元)。
func (b *MinuteBarBuilder) Append(price, volume, amount float64, serverTime string, intuition int, rate float64, now time.Time) *MinuteBar {
	if price <= 0 {
		return nil
	}
	quoteTime := parseQuoteServerTime(serverTime, now)
	session := classifyQuoteSession(quoteTime)

	// 集合竞价/竞价结果期:记录 9:25 撮合价(竞价期最后快照 close),不合成bar。
	if session == sessionAuction || session == sessionPreOpen {
		b.auction = price
		b.lastSeen = quoteTime
		return nil
	}
	// 午休:强制切bar,丢弃快照(禁止上午尾与下午头合并)。
	if session == sessionLunch {
		b.lastSeen = quoteTime
		return b.complete()
	}
	// 盘前盘后快照:收盘后允许继续修正最后一根bar的close(文档2.3收盘规则)。
	if session == sessionClosed {
		if b.current != nil && (b.lastQuote.IsZero() || quoteTime.After(b.lastQuote)) {
			b.updatePrice(price, quoteTime)
		}
		return nil
	}

	// 累计量基准初始化:开盘前累计量为0或异常值时丢弃,取第一个合法值(文档2.2)。
	if !b.firstBase {
		if volume <= 0 {
			b.lastSeen = quoteTime
			return nil
		}
		b.firstBase = true
		b.lastVol = volume
		b.lastAmt = amount
	}

	// 累计量回退(数据源切换等):重置基准,当前bar标记不可信(文档2.3)。
	if b.lastVol > 0 && volume < b.lastVol && b.current != nil {
		b.gapMarked = true
	}

	// 断流检测:gap 超过阈值时立即切bar,不得把跨gap的高低点合入同一根bar。
	var done *MinuteBar
	if !b.lastSeen.IsZero() && quoteTime.Sub(b.lastSeen) > minuteBarGapThreshold {
		b.gapMarked = true
		b.lastGap = quoteTime
		b.snapshots = nil
		done = b.complete()
	}
	b.lastSeen = quoteTime

	// 新分钟或当前bar不存在:切bar并开新bar。
	minuteKey := minuteStartText(quoteTime)
	if b.current == nil || minuteKey != b.current.Time {
		if b.current != nil {
			done = b.complete()
		}
		b.current = &MinuteBar{
			Time:     minuteKey,
			Open:     price,
			High:     price,
			Low:      price,
			Close:    price,
			Volume:   0,
			Amount:   0,
			Complete: false,
			Trusted:  true,
		}
		b.barVol, b.barAmt = volume, amount
		// 开盘第一根bar的open用9:25撮合价初始化(文档2.3)。
		if b.auction > 0 && strings.HasSuffix(minuteKey, "T09:30:00+08:00") {
			b.current.Open = b.auction
			if b.auction > b.current.High {
				b.current.High = b.auction
			}
			if b.auction < b.current.Low {
				b.current.Low = b.auction
			}
		}
	}

	b.updateTrade(price, volume, amount, quoteTime, intuition, rate)
	return done
}

// updateTrade 用盘中成交快照更新当前bar与快照序列。
func (b *MinuteBarBuilder) updateTrade(price, volume, amount float64, quoteTime time.Time, intuition int, rate float64) {
	c := b.current
	if c == nil {
		return
	}
	if price > c.High {
		c.High = price
	}
	if price < c.Low {
		c.Low = price
	}
	barVolHands := volume - b.barVol
	if barVolHands > 0 {
		c.Volume = barVolHands * 100
	}
	barAmt := amount - b.barAmt
	if barAmt > 0 {
		c.Amount = barAmt
	}
	b.updatePrice(price, quoteTime)
	b.lastVol = volume
	b.lastAmt = amount
	b.snapshots = append(b.snapshots, MinuteBarSnapshot{Time: quoteTime, Price: price, Intuition: intuition, Rate: rate})
	b.trimSnapshots(quoteTime)
}

// updatePrice 仅更新收盘价(收盘后修正用)。
func (b *MinuteBarBuilder) updatePrice(price float64, quoteTime time.Time) {
	if b.current == nil {
		return
	}
	b.current.Close = price
	b.lastQuote = quoteTime
}

func (b *MinuteBarBuilder) trimSnapshots(now time.Time) {
	cutoff := now.Add(-90 * time.Second)
	idx := -1
	for i, s := range b.snapshots {
		if s.Time.After(cutoff) {
			idx = i
			break
		}
	}
	if idx > 0 {
		b.snapshots = append([]MinuteBarSnapshot(nil), b.snapshots[idx:]...)
	}
}

// complete 结束当前bar并返回。
func (b *MinuteBarBuilder) complete() *MinuteBar {
	if b.current == nil {
		return nil
	}
	bar := b.current
	bar.Complete = true
	if b.gapMarked {
		bar.Trusted = false
	}
	b.gapMarked = false
	b.current = nil
	b.completed = append(b.completed, *bar)
	if len(b.completed) > 240 {
		b.completed = b.completed[len(b.completed)-240:]
	}
	return bar
}

// Current 返回当前未完成bar(无则nil)。
func (b *MinuteBarBuilder) Current() *MinuteBar {
	return b.current
}

// Completed 返回最近N根完成bar(升序)。
func (b *MinuteBarBuilder) Completed() []MinuteBar {
	return b.completed
}

// AuctionPrice 返回集合竞价撮合价(9:25)。
func (b *MinuteBarBuilder) AuctionPrice() float64 {
	return b.auction
}

// VWAP 当日成交量加权均价(元/股),用累计成交额/累计成交量。
func (b *MinuteBarBuilder) VWAP() float64 {
	if b.lastVol <= 0 {
		return 0
	}
	return b.lastAmt / (b.lastVol * 100)
}

// CumulativeVolumeHands 返回最近快照的当日累计成交量(手)。
func (b *MinuteBarBuilder) CumulativeVolumeHands() float64 {
	return b.lastVol
}

// MA 最近 period 根bar(含当前bar)的收盘均值。
func (b *MinuteBarBuilder) MA(period int) float64 {
	if period <= 0 {
		return 0
	}
	sum := 0.0
	count := 0
	if b.current != nil {
		sum += b.current.Close
		count++
	}
	for i := len(b.completed) - 1; i >= 0 && count < period; i-- {
		sum += b.completed[i].Close
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// MACD 1分钟MACD(12,26,9),返回 dif/dea/hist;bar不足时 ok=false。
func (b *MinuteBarBuilder) MACD() (dif, dea, hist float64, ok bool) {
	closes := make([]float64, 0, 241)
	for _, bar := range b.completed {
		closes = append(closes, bar.Close)
	}
	if b.current != nil {
		closes = append(closes, b.current.Close)
	}
	if len(closes) < 35 { // EMA26 需要足够样本
		return 0, 0, 0, false
	}
	ema12 := closes[0]
	ema26 := closes[0]
	dea = 0.0
	prevDif := 0.0
	for i := 1; i < len(closes); i++ {
		ema12 = ema12*11/13 + closes[i]*2/13
		ema26 = ema26*25/27 + closes[i]*2/27
		prevDif = ema12 - ema26
		dea = dea*8/10 + prevDif*2/10
	}
	return prevDif, dea, 2 * (prevDif - dea), true
}

// RecentMovePct 最近 window 内涨跌幅(百分比)。
func (b *MinuteBarBuilder) RecentMovePct(window time.Duration) float64 {
	if len(b.snapshots) < 2 {
		return 0
	}
	latest := b.snapshots[len(b.snapshots)-1]
	cutoff := latest.Time.Add(-window)
	var first *MinuteBarSnapshot
	for i := range b.snapshots {
		if b.snapshots[i].Time.After(cutoff) {
			first = &b.snapshots[i]
			break
		}
	}
	if first == nil || first.Price <= 0 {
		return 0
	}
	return (latest.Price - first.Price) / first.Price * 100
}

// RecentAvgIntuition 最近 window 内现量均值。
func (b *MinuteBarBuilder) RecentAvgIntuition(window time.Duration) float64 {
	if len(b.snapshots) == 0 {
		return 0
	}
	latest := b.snapshots[len(b.snapshots)-1]
	cutoff := latest.Time.Add(-window)
	sum, count := 0, 0
	for _, s := range b.snapshots {
		if s.Time.After(cutoff) {
			sum += s.Intuition
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

// SnapshotTrusted 断流后的一个完整阈值窗口内不允许快照级信号，避免跨 gap 误判急拉急跌。
func (b *MinuteBarBuilder) SnapshotTrusted(now time.Time) bool {
	return b.lastGap.IsZero() || now.Sub(b.lastGap) > minuteBarGapThreshold
}

// LastFiveMinuteDirection 返回最近一根完整5分钟K的方向。仅接受同一交易时段内连续的5根分钟bar，
// 午休、断流或数据缺口都不会被拼成确认bar。
func (b *MinuteBarBuilder) LastFiveMinuteDirection() (string, bool) {
	if len(b.completed) < 5 {
		return "", false
	}
	for end := len(b.completed) - 1; end >= 4; end-- {
		endTime, err := time.Parse(time.RFC3339, b.completed[end].Time)
		if err != nil || endTime.Minute()%5 != 4 {
			continue
		}
		start := end - 4
		valid := true
		if !b.completed[start].Trusted {
			continue
		}
		for i := start + 1; i <= end; i++ {
			left, leftErr := time.Parse(time.RFC3339, b.completed[i-1].Time)
			right, rightErr := time.Parse(time.RFC3339, b.completed[i].Time)
			if leftErr != nil || rightErr != nil || !b.completed[i].Trusted || !right.Equal(left.Add(time.Minute)) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		open, close := b.completed[start].Open, b.completed[end].Close
		if close > open {
			return "up", true
		}
		if close < open {
			return "down", true
		}
		return "flat", true
	}
	return "", false
}

// DayHighFirst15m 开盘15分钟(9:30-9:45)内完成bar的最高价(用于S4突破)。
func (b *MinuteBarBuilder) DayHighFirst15m() float64 {
	high := 0.0
	for _, bar := range b.completed {
		if strings.Contains(bar.Time, "T09:3") || strings.Contains(bar.Time, "T09:4") {
			if bar.High > high {
				high = bar.High
			}
		}
	}
	return high
}

// PrevExtreme 最近 lookback 根bar(含当前bar)的最高/最低价。
func (b *MinuteBarBuilder) PrevExtreme(lookback int) (high, low float64) {
	if b.current != nil {
		high, low = b.current.High, b.current.Low
	}
	count := 0
	for i := len(b.completed) - 1; i >= 0 && count < lookback; i-- {
		bar := b.completed[i]
		if high == 0 || bar.High > high {
			high = bar.High
		}
		if low == 0 || bar.Low < low {
			low = bar.Low
		}
		count++
	}
	return high, low
}
