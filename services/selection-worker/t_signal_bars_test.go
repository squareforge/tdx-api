package main

import (
	"testing"
	"time"
)

func loc() *time.Location {
	return time.FixedZone("CST", 8*3600)
}

func ts(day int, hour, minute, second int) time.Time {
	return time.Date(2026, 9, day, hour, minute, second, 0, loc())
}

// TestMinuteBarBuilderBasic 基本合成:单分钟多条快照 → OHLC/量额正确。
func TestMinuteBarBuilderBasic(t *testing.T) {
	b := NewMinuteBarBuilder()
	base := ts(18, 10, 0, 0)
	var done *MinuteBar
	// 模拟真实 3 秒间隔连续推送 20 个采样点(0s-57s)。
	for i := 0; i < 20; i++ {
		price := 10.0 + float64(i)*0.01
		done = b.Append(price, 1000+float64(i*2), 100000+float64(i*200), "", 0, 0, base.Add(time.Duration(i*3)*time.Second))
	}
	// 跨分钟:触发切bar。
	done = b.Append(10.4, 1050, 105000, "", 0, 0, ts(18, 10, 1, 1))
	if done == nil {
		t.Fatal("跨分钟应返回完成bar")
	}
	if done.Open != 10.0 || done.High != 10.19 || done.Low != 10.0 || done.Close != 10.19 {
		t.Fatalf("OHLC 合成错误: %+v", done)
	}
	if done.Volume != 38*100 { // 累计量差(手)×100
		t.Fatalf("volume 应为 3800,得到 %v", done.Volume)
	}
	if !done.Complete || !done.Trusted {
		t.Fatalf("bar 应完整可信: %+v", done)
	}
	cur := b.Current()
	if cur == nil || cur.Close != 10.4 {
		t.Fatalf("新bar未建立: %+v", cur)
	}
}

// TestMinuteBarBuilderAuction 竞价撮合价初始化 9:30 bar 的 open。
func TestMinuteBarBuilderAuction(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(9.80, 0, 0, "", 0, 0, ts(18, 9, 20, 0)) // 集合竞价
	b.Append(9.85, 0, 0, "", 0, 0, ts(18, 9, 25, 0)) // 9:25 撮合价
	b.Append(9.90, 0, 0, "", 0, 0, ts(18, 9, 27, 0)) // 竞价结果期
	if b.AuctionPrice() != 9.90 {
		t.Fatalf("竞价价应为 9.90,得到 %v", b.AuctionPrice())
	}
	done := b.Append(10.00, 100, 10000, "", 0, 0, ts(18, 9, 30, 5))
	if done != nil {
		t.Fatal("9:30 第一根bar不应立即完成")
	}
	cur := b.Current()
	if cur == nil || cur.Open != 9.90 {
		t.Fatalf("9:30 bar 的 open 应用竞价价初始化: %+v", cur)
	}
}

// TestMinuteBarBuilderLunch 午休必须切bar,上午尾与下午头不合并。
func TestMinuteBarBuilderLunch(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(10.0, 100, 10000, "", 0, 0, ts(18, 11, 29, 30))
	done := b.Append(10.1, 110, 11000, "", 0, 0, ts(18, 11, 30, 1))
	if done == nil {
		t.Fatal("午休到达应完成当前bar")
	}
	if b.Current() != nil {
		t.Fatal("午休期间不应存在当前bar")
	}
	done = b.Append(10.2, 120, 12000, "", 0, 0, ts(18, 13, 0, 30))
	if done != nil {
		t.Fatal("13:00 第一条快照只开新bar,不应有完成bar")
	}
	cur := b.Current()
	if cur == nil || cur.Open != 10.2 {
		t.Fatalf("下午第一根bar未建立: %+v", cur)
	}
}

// TestMinuteBarBuilderGap 断流超阈值:立即切bar且标记不可信。
func TestMinuteBarBuilderGap(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(10.0, 100, 10000, "", 0, 0, ts(18, 10, 10, 0))
	done := b.Append(10.5, 150, 15000, "", 0, 0, ts(18, 10, 11, 0)) // gap 60s
	if done == nil {
		t.Fatal("断流应触发切bar")
	}
	if done.Trusted {
		t.Fatal("断流区段应标记不可信")
	}
}

// TestMinuteBarBuilderVolumeReset 累计量回退:重置基准,bar标记不可信。
func TestMinuteBarBuilderVolumeReset(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(10.0, 1000, 100000, "", 0, 0, ts(18, 10, 10, 0))
	done := b.Append(9.9, 500, 50000, "", 0, 0, ts(18, 10, 11, 0)) // 累计量回退
	if done == nil || done.Trusted {
		t.Fatal("累计量回退的bar应不可信")
	}
}

// TestMinuteBarBuilderVWAP VWAP = 累计额 / 累计量。
func TestMinuteBarBuilderVWAP(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(10.0, 100, 100000, "", 0, 0, ts(18, 10, 10, 0))
	b.Append(11.0, 200, 320000, "", 0, 0, ts(18, 10, 10, 10))
	vwap := b.VWAP()
	// 累计额 320000 / (200手×100) = 16 元/股。
	if vwap != 16.0 {
		t.Fatalf("VWAP 应为 16,得到 %v", vwap)
	}
}

// TestMinuteBarBuilderVWAPAfterComplete 切bar后VWAP仍应包含刚完成的分钟成交。
func TestMinuteBarBuilderVWAPAfterComplete(t *testing.T) {
	b := NewMinuteBarBuilder()
	b.Append(10.0, 100, 100000, "", 0, 0, ts(18, 10, 10, 0))
	b.Append(11.0, 200, 320000, "", 0, 0, ts(18, 10, 10, 30))
	b.Append(12.0, 250, 450000, "", 0, 0, ts(18, 10, 11, 0))
	if got := b.VWAP(); got != 18.0 {
		t.Fatalf("切bar后VWAP应使用最新累计额和累计量，得到 %v", got)
	}
}

// TestLastFiveMinuteDirection 连续可信分钟才可组成5分钟方向确认。
func TestLastFiveMinuteDirection(t *testing.T) {
	b := NewMinuteBarBuilder()
	for i := 0; i < 5; i++ {
		b.completed = append(b.completed, MinuteBar{
			Time: ts(18, 10, i, 0).Format(time.RFC3339), Open: 10, Close: 9.8, Complete: true, Trusted: true,
		})
	}
	direction, ok := b.LastFiveMinuteDirection()
	if !ok || direction != "down" {
		t.Fatalf("连续下跌5分钟K应确认为 down，得到 %q, %v", direction, ok)
	}
	b.completed[2].Trusted = false
	if _, ok := b.LastFiveMinuteDirection(); ok {
		t.Fatal("包含不可信分钟的5分钟K不应通过确认")
	}
	b.completed[2].Trusted = true
	b.completed[2].Time = ts(18, 10, 3, 0).Format(time.RFC3339)
	if _, ok := b.LastFiveMinuteDirection(); ok {
		t.Fatal("分钟时间缺口不应通过5分钟确认")
	}
}

func TestFiveMinuteBarsAggregatesOHLCV(t *testing.T) {
	b := NewMinuteBarBuilder()
	for i := 0; i < 5; i++ {
		b.completed = append(b.completed, MinuteBar{
			Time: ts(18, 10, i, 0).Format(time.RFC3339),
			Open: 10 + float64(i)/10, High: 10.5 + float64(i)/10,
			Low: 9.8 - float64(i)/10, Close: 10.1 + float64(i)/10,
			Volume: 1000, Amount: 10000, Complete: true, Trusted: true,
		})
	}
	bars := b.FiveMinuteBars()
	if len(bars) != 1 {
		t.Fatalf("应聚合为一根5分钟bar，得到 %d", len(bars))
	}
	bar := bars[0]
	if bar.Open != 10 || bar.Close != 10.5 || bar.High != 10.9 || bar.Low != 9.4 || bar.Volume != 5000 || bar.Amount != 50000 {
		t.Fatalf("5分钟OHLCV聚合错误: %+v", bar)
	}
}

// TestParseQuoteServerTime ServerTime 解析与回退。
func TestParseQuoteServerTime(t *testing.T) {
	fallback := ts(18, 10, 30, 15)
	cases := []struct {
		raw  string
		want string
	}{
		{"103015", "10:30:15"},
		{"103015123", "10:30:15"},
		{"999999", "10:30:15"}, // 非法时分秒,回退
		{"", "10:30:15"},
	}
	for _, c := range cases {
		got := parseQuoteServerTime(c.raw, fallback)
		if got.Format("15:04:05") != c.want {
			t.Fatalf("parseQuoteServerTime(%q) = %v, want %s", c.raw, got, c.want)
		}
	}
	// 与本地时间偏差超3分钟:回退。
	got := parseQuoteServerTime("104500", fallback)
	if got.Format("15:04:05") != "10:30:15" {
		t.Fatalf("偏差过大应回退,得到 %v", got)
	}
	// 前导零缺失的短格式(HMMSS):在本地时间附近时正常解析。
	got = parseQuoteServerTime("93015", ts(18, 9, 30, 15))
	if got.Format("15:04:05") != "09:30:15" {
		t.Fatalf("HMMSS 应解析为 09:30:15,得到 %v", got)
	}
}

// TestMinuteBarMACD MACD 序列计算。
func TestMinuteBarMACD(t *testing.T) {
	b := NewMinuteBarBuilder()
	base := 10.0
	for i := 0; i < 40; i++ {
		b.Append(base+float64(i)*0.01, 100, 10000, "", 0, 0, ts(18, 9, 30, 0).Add(time.Duration(i)*time.Minute))
	}
	dif, dea, hist, ok := b.MACD()
	if !ok {
		t.Fatal("40根bar后MACD应可计算")
	}
	_ = dif
	_ = dea
	if hist == 0 {
		t.Fatal("上升趋势MACD柱不应为0")
	}
}

// TestMinuteBarRecentMove 短窗口涨跌幅。
func TestMinuteBarRecentMove(t *testing.T) {
	b := NewMinuteBarBuilder()
	base := ts(18, 10, 30, 0)
	b.Append(10.00, 100, 10000, "", 0, 0, base)
	b.Append(10.10, 110, 11000, "", 0, 0, base.Add(20*time.Second))
	b.Append(10.20, 120, 12000, "", 0, 0, base.Add(40*time.Second))
	move := b.RecentMovePct(60 * time.Second)
	if move < 1.9 || move > 2.1 {
		t.Fatalf("60s涨跌幅应为2%%,得到 %v", move)
	}
}
