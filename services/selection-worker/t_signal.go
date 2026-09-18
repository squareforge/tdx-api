package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	workbench "workbench-core"
)

// 日内做T信号引擎,对齐 docs/INTRADAY_T_SIGNAL_V1.md。
// 只出信号、不接下单;默认「先卖后买」+ 趋势否决;信号粒度1分钟触发+5分钟方向确认。

// 信号方向枚举(文档4.1)。
const (
	TSignalDirSellFirst  = "SELL_FIRST"  // 高抛
	TSignalDirBuyBack    = "BUY_BACK"    // 低吸买回
	TSignalDirReduce     = "REDUCE"      // 减仓提示
	TSignalDirStandAside = "STAND_ASIDE" // 观望提示
)

// 信号状态(文档4.4 生命周期)。
const (
	TSignalStatusNew       = "NEW"       // 刚触发
	TSignalStatusActive    = "ACTIVE"    // 有效期内等待执行
	TSignalStatusExecuted  = "EXECUTED"  // 已按信号成交(人工标记)
	TSignalStatusCancelled = "CANCELLED" // 过期/趋势否决/人工撤销
)

// 规则编码(文档4.2)。
const (
	TSignalRuleS1Vwap   = "S1_VWAP_REGRESSION"
	TSignalRuleS2Ma     = "S2_MA_SUPPORT"
	TSignalRuleS3Macd   = "S3_MACD_DIVERGENCE"
	TSignalRuleS4Break  = "S4_OPEN_BREAKOUT"
	TSignalRuleS5Spike  = "S5_SPIKE_REVERSAL"
	TSignalRuleFollowUp = "FOLLOW_UP_BUY_BACK"
)

// TSignalRuleParams S1-S5 初始参数(文档4.2,回测后校准)。
type TSignalRuleParams struct {
	S1Enabled bool    `json:"s1_enabled"`
	S1DevPct  float64 `json:"s1_dev_pct"`  // 偏离触发阈值 ±0.5%
	S1BackPct float64 `json:"s1_back_pct"` // 回归确认阈值 0.2%
	S1StopPct float64 `json:"s1_stop_pct"` // 止损偏离 0.8%

	S2Enabled bool    `json:"s2_enabled"`
	S2StopPct float64 `json:"s2_stop_pct"` // 跌破/突破均线止损 0.3%

	S3Enabled  bool `json:"s3_enabled"`
	S3Lookback int  `json:"s3_lookback"` // 背离比较窗口(根bar)

	S4Enabled   bool    `json:"s4_enabled"` // 默认关(追涨类,先卖后买模式下不推荐)
	S4TargetPct float64 `json:"s4_target_pct"`
	S4VolRatio  float64 `json:"s4_vol_ratio"` // 量比阈值

	S5Enabled  bool    `json:"s5_enabled"`
	S5MovePct  float64 `json:"s5_move_pct"` // 急拉急跌幅度 1.5%
	S5StopPct  float64 `json:"s5_stop_pct"` // 继续同向止损 0.5%
	S5Cooldown int     `json:"s5_cooldown"` // 同向冷却分钟
}

func DefaultTSignalRuleParams() TSignalRuleParams {
	return TSignalRuleParams{
		S1Enabled: true, S1DevPct: 0.5, S1BackPct: 0.2, S1StopPct: 0.8,
		S2Enabled: true, S2StopPct: 0.3,
		S3Enabled: true, S3Lookback: 10,
		S4Enabled: false, S4TargetPct: 0.8, S4VolRatio: 1.5,
		S5Enabled: true, S5MovePct: 1.5, S5StopPct: 0.5, S5Cooldown: 15,
	}
}

// TSignalFeeConfig 费率(文档4.3 成本门槛)。
type TSignalFeeConfig struct {
	Commission float64 `json:"commission"` // 单边佣金 %
	StampTax   float64 `json:"stamp_tax"`  // 卖出印花税 %
	Slippage   float64 `json:"slippage"`   // 单边滑点 %
	MinRound   float64 `json:"min_round"`  // 最低往返预期收益 %(低于不发)
}

func DefaultTSignalFeeConfig() TSignalFeeConfig {
	return TSignalFeeConfig{Commission: 0.025, StampTax: 0.05, Slippage: 0.05, MinRound: 0.3}
}

// TSignalConfig 做T配置(标的订阅、底仓、参数、开关)。
type TSignalConfig struct {
	Symbol         string            `json:"symbol"`          // 做T标的,如 sz000001
	PositionShares int               `json:"position_shares"` // 底仓股数(手动录入)
	TrendVeto      bool              `json:"trend_veto"`      // 趋势否决开关
	Rules          TSignalRuleParams `json:"rules"`
	Fees           TSignalFeeConfig  `json:"fees"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
}

// TSignalFollowUp 后续回补条件(文档4.4,SELL_FIRST 必须绑定 BUY_BACK 条件)。
type TSignalFollowUp struct {
	Rule string  `json:"rule"` // BUY_BACK_AT_VWAP 等
	Ref  float64 `json:"ref"`  // 回补参考价
}

// TSignalRecord 一条信号(文档4.5 JSON 结构)。
type TSignalRecord struct {
	ID           string           `json:"id"`
	Symbol       string           `json:"symbol"`
	Time         string           `json:"time"`
	Direction    string           `json:"direction"`
	Rule         string           `json:"rule"`
	Trigger      string           `json:"trigger"`
	RefPrice     float64          `json:"ref_price"` // 触发参考价
	Tolerance    float64          `json:"tolerance"` // 触发价余量 ±
	Target       float64          `json:"target"`
	Stop         float64          `json:"stop"`
	ExpireAt     string           `json:"expire_at"`
	PositionHint string           `json:"position_hint"` // 底仓的1/3
	FollowUp     *TSignalFollowUp `json:"follow_up,omitempty"`
	Status       string           `json:"status"`
	AckAt        string           `json:"ack_at,omitempty"`
}

// TSignalLedgerEntry 当日账本条目(卖出→买回配对)。
type TSignalLedgerEntry struct {
	ID           string  `json:"id"`
	SellSignalID string  `json:"sell_signal_id"`
	BuySignalID  string  `json:"buy_signal_id,omitempty"`
	SellPrice    float64 `json:"sell_price"`
	BuyPrice     float64 `json:"buy_price,omitempty"`
	SellTime     string  `json:"sell_time"`
	BuyTime      string  `json:"buy_time,omitempty"`
	Shares       int     `json:"shares"`
	NetPnl       float64 `json:"net_pnl,omitempty"` // 含费净盈亏(配对后)
	Closed       bool    `json:"closed"`
}

// TSignalTrendVeto 趋势否决四态(文档3.3)。
type TSignalTrendVeto struct {
	Status    string `json:"status"` // strong_up/strong_down/range/limit
	Detail    string `json:"detail"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// TSignalCooldown 连续亏损暂停(文档5.5)。
type TSignalCooldown struct {
	Active  bool    `json:"active"`
	Reason  string  `json:"reason"`
	LossPnl float64 `json:"loss_pnl,omitempty"` // 当日闭环累计盈亏
}

// TSignalState 引擎完整状态,JSON 序列化后存 t_signal_state 表。
type TSignalState struct {
	Day       string               `json:"day"` // 交易日 YYYYMMDD
	Config    TSignalConfig        `json:"config"`
	Signals   []TSignalRecord      `json:"signals"`
	Ledger    []TSignalLedgerEntry `json:"ledger"`
	Trend     TSignalTrendVeto     `json:"trend"`
	Cooldown  TSignalCooldown      `json:"cooldown"`
	PrevClose float64              `json:"prev_close"` // 昨收
	PrevVol   float64              `json:"prev_vol"`   // 昨日全天量(手)
	Counter   int                  `json:"counter"`    // 当日信号序号
	UpdatedAt string               `json:"updated_at,omitempty"`
}

// TSignalEngine 做T信号引擎。
type TSignalEngine struct {
	store  *AppStore
	market *MarketServiceClient
	mu     sync.RWMutex

	state        TSignalState
	builder      *MinuteBarBuilder
	cancel       context.CancelFunc
	streamCancel context.CancelFunc
	wake         chan struct{}

	lastSnapshot  time.Time
	lastMinuteKey string
	consecLosses  int                  // 连亏笔数(内存)
	difHistory    []float64            // 1分钟MACD dif 序列(背离比较)
	lastS5        map[string]time.Time // S5 同向冷却
	s1HighPending bool                 // S1 超买状态机
	s1LowPending  bool                 // S1 超卖状态机
	s1HighPeak    float64              // S1 超买期间最大偏离(%)
	s1LowTrough   float64              // S1 超卖期间最小偏离(%)
	s1HighAt      time.Time            // S1 超买起点,回归确认最多3分钟
	s1LowAt       time.Time            // S1 超卖起点,回归确认最多3分钟
	prevCloseMA   bool                 // S2 上一根bar收盘与均线关系
}

func NewTSignalEngine(store *AppStore, market *MarketServiceClient) *TSignalEngine {
	return &TSignalEngine{
		store:   store,
		market:  market,
		builder: NewMinuteBarBuilder(),
		state: TSignalState{
			Day: dateText(),
			Config: TSignalConfig{
				Rules: DefaultTSignalRuleParams(),
				Fees:  DefaultTSignalFeeConfig(),
			},
		},
		lastS5: map[string]time.Time{},
		wake:   make(chan struct{}, 1),
	}
}

// dateText 当日日期 YYYYMMDD。
func dateText() string {
	return time.Now().Format("20060102")
}

// Start 启动引擎:恢复状态后进入订阅循环。
func (e *TSignalEngine) Start() {
	if e == nil {
		return
	}
	e.restore()
	if e.market == nil || !e.market.Enabled() {
		log.Printf("日内做T信号引擎未连接行情服务")
		return
	}
	if !envBool("TSIGNAL_ENABLED", true) {
		log.Printf("日内做T信号引擎未启用(TSIGNAL_ENABLED=false)")
		return
	}
	go e.refreshPrevDayVolume()
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	go e.run(ctx)
	log.Printf("日内做T信号引擎已启动")
}

func (e *TSignalEngine) Stop() {
	if e != nil && e.cancel != nil {
		e.cancel()
	}
}

func (e *TSignalEngine) run(ctx context.Context) {
	for {
		symbol := e.currentSymbol()
		if symbol == "" {
			select {
			case <-ctx.Done():
				return
			case <-e.wake:
			case <-time.After(30 * time.Second):
			}
			continue
		}
		streamCtx, cancel := context.WithCancel(ctx)
		e.mu.Lock()
		e.streamCancel = cancel
		e.mu.Unlock()
		err := e.market.SubscribeQuotes(streamCtx, []string{normalizeMonitorSymbol(symbol)}, e.handleEvent)
		e.mu.Lock()
		e.streamCancel = nil
		e.mu.Unlock()
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("做T信号行情订阅断开: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-time.After(5 * time.Second):
		}
	}
}

func (e *TSignalEngine) requestStreamRestart() {
	e.mu.RLock()
	cancel := e.streamCancel
	e.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *TSignalEngine) currentSymbol() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return strings.TrimSpace(e.state.Config.Symbol)
}

// handleEvent 处理 SSE 快照:合成分钟bar,bar 完成时评估规则。
func (e *TSignalEngine) handleEvent(event MarketQuoteStreamEvent) {
	if event.Type != "quote" || event.Snapshot == nil || event.Snapshot.Quote == nil || event.Snapshot.Quote.Kline == nil {
		return
	}
	quote := event.Snapshot.Quote
	price := quote.Kline.Close.Float64()
	if price <= 0 {
		return
	}
	symbol := event.Snapshot.Symbol
	if symbol == "" {
		symbol = event.Snapshot.Code
	}
	now := time.Now()
	quoteTime := parseQuoteServerTime(quote.ServerTime, now)
	serverTime := quote.ServerTime
	refreshPrevDay := false

	e.mu.Lock()
	defer func() {
		e.mu.Unlock()
		if refreshPrevDay {
			go e.refreshPrevDayVolume()
		}
	}()

	quoteDay := quoteTime.Format("20060102")
	if e.state.Day == "" {
		e.state.Day = quoteDay
	} else if e.state.Day != quoteDay {
		e.resetDailyLocked(quoteTime)
		e.persist()
		refreshPrevDay = true
	}
	if e.state.PrevClose <= 0 {
		e.state.PrevClose = quote.Kline.Last.Float64()
	}

	// 重建 builder(首次启动)。
	if e.builder == nil {
		e.builder = NewMinuteBarBuilder()
	}
	volume := float64(quote.Kline.Volume)
	amount := quote.Kline.Amount.Float64()
	done := e.builder.Append(price, volume, amount, serverTime, quote.Intuition, quote.Rate, quoteTime)
	e.lastSnapshot = now

	// S5 快照级检测(急拉急跌反转)。
	if e.state.Config.Rules.S5Enabled && done == nil {
		e.evaluateS5(price, quoteTime)
		e.tickLifecycleAt(quoteTime)
	}

	// 分钟bar完成:更新指标序列 + 规则评估 + 生命周期推进。
	if done != nil {
		e.onMinuteBar(done)
	}
}

// onMinuteBar 分钟bar完成时的处理:趋势否决、S1-S4、follow_up、过期与收盘检查。
func (e *TSignalEngine) onMinuteBar(bar *MinuteBar) {
	// 更新 MACD dif 序列(背离比较用)。
	if dif, _, _, ok := e.builder.MACD(); ok {
		e.difHistory = append(e.difHistory, dif)
		if len(e.difHistory) > 120 {
			e.difHistory = e.difHistory[len(e.difHistory)-120:]
		}
	}
	// S2 状态:记录上一根bar收盘与均线关系(本bar完成时评估)。
	if bar.Complete {
		e.updateTrendVeto(bar)
		if !bar.Trusted {
			e.tickLifecycleAt(barTime(bar))
			return
		}
		e.evaluateS1(bar)
		e.evaluateS2(bar)
		e.evaluateS3(bar)
		e.evaluateS4(bar)
		e.checkFollowUpBuyBack(bar)
		e.tickLifecycleAt(barTime(bar))
		e.persist()
	}
}

// updateTrendVeto 趋势否决四态判定(文档3.3)。
func (e *TSignalEngine) updateTrendVeto(bar *MinuteBar) {
	cfg := &e.state.Config
	if !cfg.TrendVeto {
		e.state.Trend.Status = "range"
		e.state.Trend.Detail = "趋势否决关闭"
		e.state.Trend.UpdatedAt = nowText()
		return
	}
	vwap := e.builder.VWAP()
	if vwap <= 0 {
		e.state.Trend.Status = "range"
		e.state.Trend.UpdatedAt = nowText()
		return
	}
	devPct := (bar.Close - vwap) / vwap * 100
	volRatio := e.volumeRatio()
	status, detail := "range", ""
	switch {
	case e.nearLimitPrice(bar.Close):
		status, detail = "limit", "涨跌停附近,不操作"
	case devPct > 1 && volRatio > 1.5:
		status, detail = "strong_up", fmt.Sprintf("强势单边:偏离VWAP %.2f%%,量比 %.2f,不发卖出信号", devPct, volRatio)
	case devPct < -1 && volRatio > 1.5:
		status, detail = "strong_down", fmt.Sprintf("弱势单边:偏离VWAP %.2f%%,量比 %.2f,只提示减仓", devPct, volRatio)
	default:
		status, detail = "range", fmt.Sprintf("震荡:偏离VWAP %.2f%%,量比 %.2f", devPct, volRatio)
	}
	e.state.Trend = TSignalTrendVeto{Status: status, Detail: detail, UpdatedAt: nowText()}
	e.cancelSignalsForTrend(status)
}

func (e *TSignalEngine) cancelSignalsForTrend(status string) {
	for i := range e.state.Signals {
		sig := &e.state.Signals[i]
		if sig.Status != TSignalStatusNew && sig.Status != TSignalStatusActive {
			continue
		}
		cancel := status == "limit" ||
			(status == "strong_up" && sig.Direction == TSignalDirSellFirst) ||
			(status == "strong_down" && sig.Direction == TSignalDirBuyBack)
		if cancel {
			sig.Status = TSignalStatusCancelled
			sig.AckAt = nowText()
		}
	}
}

// volumeRatio 量比 = 当日每分钟均量 / (昨日全天量/240)。昨日数据缺失时返回0(保守)。
func (e *TSignalEngine) volumeRatio() float64 {
	if e.state.PrevVol <= 0 {
		return 0
	}
	minutes := float64(len(e.builder.Completed()))
	if minutes <= 0 {
		minutes = 1
	}
	dayAvg := e.state.PrevVol / 240
	if dayAvg <= 0 {
		return 0
	}
	// 当日累计量(手) = 当前bar起始累计量 + 当前bar成交量。
	acc := e.builder.CumulativeVolumeHands()
	return (acc / minutes) / dayAvg
}

// nearLimitPrice 距涨跌停 < 1% 判定(主板10%/创业板科创20%/北交所30%)。
func (e *TSignalEngine) nearLimitPrice(price float64) bool {
	if e.state.PrevClose <= 0 {
		return false
	}
	pct := 10.0
	switch {
	case strings.HasPrefix(strings.ToLower(e.state.Config.Symbol), "sz300"), strings.HasPrefix(strings.ToLower(e.state.Config.Symbol), "sz301"),
		strings.HasPrefix(strings.ToLower(e.state.Config.Symbol), "sh688"), strings.HasPrefix(strings.ToLower(e.state.Config.Symbol), "sh689"):
		pct = 20
	case strings.HasPrefix(strings.ToLower(e.state.Config.Symbol), "bj"):
		pct = 30
	}
	upLimit := e.state.PrevClose * (1 + pct/100)
	downLimit := e.state.PrevClose * (1 - pct/100)
	return price > upLimit*0.99 || price < downLimit*1.01
}

// evaluateS1 S1 VWAP 回归(文档4.2):偏离±0.5%后回归至0.2%以内触发。
func (e *TSignalEngine) evaluateS1(bar *MinuteBar) {
	params := e.state.Config.Rules
	if !params.S1Enabled {
		return
	}
	vwap := e.builder.VWAP()
	if vwap <= 0 {
		return
	}
	devPct := (bar.Close - vwap) / vwap * 100

	// 高抛侧:记录超买峰值,回落 S1BackPct 后仍保留足够目标空间时触发。
	// 原先要求回到 VWAP 0.2% 内才卖出,目标又是 VWAP,会被成本门槛全部过滤。
	now := barTime(bar)
	minTargetDev := e.minimumTargetDeviationPct()
	if devPct >= params.S1DevPct {
		if !e.s1HighPending || devPct > e.s1HighPeak {
			e.s1HighPeak, e.s1HighAt = devPct, now
		}
		e.s1HighPending = true
	} else if e.s1HighPending {
		expired := now.Sub(e.s1HighAt) > 3*time.Minute
		if expired || devPct <= -params.S1StopPct {
			e.s1HighPending, e.s1HighPeak = false, 0
		} else if e.s1HighPeak-devPct >= params.S1BackPct && devPct >= minTargetDev {
			peak := e.s1HighPeak
			pullback := peak - devPct
			e.s1HighPending, e.s1HighPeak = false, 0
			e.emitSignal(bar, TSignalDirSellFirst, TSignalRuleS1Vwap,
				fmt.Sprintf("1min偏离VWAP峰值%.2f%%后回落%.2f%%(当前%.2f%%)", peak, pullback, devPct),
				bar.Close, vwap, vwap*(1+params.S1StopPct/100), &TSignalFollowUp{Rule: "BUY_BACK_AT_VWAP", Ref: vwap})
		}
	}

	// 低吸侧:记录超卖谷值,反弹 S1BackPct 后仍保留足够目标空间时触发。
	if devPct <= -params.S1DevPct {
		if !e.s1LowPending || devPct < e.s1LowTrough {
			e.s1LowTrough, e.s1LowAt = devPct, now
		}
		e.s1LowPending = true
	} else if e.s1LowPending {
		expired := now.Sub(e.s1LowAt) > 3*time.Minute
		if expired || devPct >= params.S1StopPct {
			e.s1LowPending, e.s1LowTrough = false, 0
		} else if devPct-e.s1LowTrough >= params.S1BackPct && -devPct >= minTargetDev && e.hasOpenSellFirst() {
			trough := e.s1LowTrough
			rebound := devPct - trough
			e.s1LowPending, e.s1LowTrough = false, 0
			e.emitSignal(bar, TSignalDirBuyBack, TSignalRuleS1Vwap,
				fmt.Sprintf("1min偏离VWAP谷值%.2f%%后反弹%.2f%%(当前%.2f%%)", trough, rebound, devPct),
				bar.Close, vwap, vwap*(1-params.S1StopPct/100), nil)
		}
	}
}

func (e *TSignalEngine) minimumTargetDeviationPct() float64 {
	fees := e.state.Config.Fees
	return fees.MinRound + fees.StampTax + fees.Commission*2 + fees.Slippage*2
}

func barTime(bar *MinuteBar) time.Time {
	if bar != nil {
		if parsed, err := time.Parse(time.RFC3339, bar.Time); err == nil {
			return parsed
		}
	}
	return time.Now()
}

// evaluateS2 S2 分时均线支撑/压力(文档4.2):回踩不破/上冲不破。
func (e *TSignalEngine) evaluateS2(bar *MinuteBar) {
	params := e.state.Config.Rules
	if !params.S2Enabled {
		return
	}
	ma := e.builder.MA(5)
	if ma <= 0 {
		return
	}
	closeAbove := bar.Close > ma
	// 支撑确认:上一根bar收盘在均线上方,本bar回踩未破 → BUY_BACK。
	if e.prevCloseMA && closeAbove && bar.Low <= ma {
		if e.hasOpenSellFirst() {
			high, _ := e.builder.PrevExtreme(20)
			target := high
			if target <= 0 {
				target = bar.Close * 1.005
			}
			e.emitSignal(bar, TSignalDirBuyBack, TSignalRuleS2Ma,
				fmt.Sprintf("1min价回踩分时均线%.3f不破", ma),
				bar.Close, target, ma*(1-params.S2StopPct/100), nil)
		}
		e.prevCloseMA = closeAbove
		return
	}
	// 压力受阻:上一根bar收盘在均线下方,本bar上冲未破 → SELL_FIRST。
	if !e.prevCloseMA && !closeAbove && bar.High >= ma {
		_, low := e.builder.PrevExtreme(20)
		target := low
		if target <= 0 {
			target = bar.Close * 0.995
		}
		e.emitSignal(bar, TSignalDirSellFirst, TSignalRuleS2Ma,
			fmt.Sprintf("1min价上冲分时均线%.3f未破", ma),
			bar.Close, target, ma*(1+params.S2StopPct/100), &TSignalFollowUp{Rule: "BUY_BACK_AT_MA", Ref: ma})
	}
	e.prevCloseMA = closeAbove
}

// evaluateS3 S3 MACD 背离(文档4.2):价创新高/新低而DIF不创新高/新低。
func (e *TSignalEngine) evaluateS3(bar *MinuteBar) {
	params := e.state.Config.Rules
	if !params.S3Enabled {
		return
	}
	if len(e.difHistory) < params.S3Lookback+1 {
		return
	}
	bars := e.builder.Completed()
	if len(bars) < params.S3Lookback+1 {
		return
	}
	lookback := params.S3Lookback
	prevHigh := 0.0
	prevLow := 0.0
	prevDifHigh := e.difHistory[len(e.difHistory)-1-lookback]
	prevDifLow := prevDifHigh
	for i := len(bars) - 2; i >= 0 && i >= len(bars)-1-lookback; i-- {
		if prevHigh == 0 || bars[i].Close > prevHigh {
			prevHigh = bars[i].Close
		}
		if prevLow == 0 || bars[i].Close < prevLow {
			prevLow = bars[i].Close
		}
	}
	// dif 历史窗口比较(含当前bar的dif)。
	curDif := e.difHistory[len(e.difHistory)-1]
	for i := len(e.difHistory) - 1 - lookback; i < len(e.difHistory)-1; i++ {
		if e.difHistory[i] > prevDifHigh {
			prevDifHigh = e.difHistory[i]
		}
		if e.difHistory[i] < prevDifLow {
			prevDifLow = e.difHistory[i]
		}
	}
	// 顶背离:价创新高,DIF不创新高 → SELL_FIRST。
	if bar.Close > prevHigh && curDif <= prevDifHigh {
		e.emitSignal(bar, TSignalDirSellFirst, TSignalRuleS3Macd,
			fmt.Sprintf("价创新高%.3f,1min MACD DIF不创新高(%.4f <= %.4f)", bar.Close, curDif, prevDifHigh),
			bar.Close, prevHigh, bar.Close*1.003, &TSignalFollowUp{Rule: "BUY_BACK_AT_PREV_HIGH", Ref: prevHigh})
	}
	// 底背离:价创新低,DIF不创新低 → BUY_BACK。
	if bar.Close < prevLow && curDif >= prevDifLow {
		if e.hasOpenSellFirst() {
			e.emitSignal(bar, TSignalDirBuyBack, TSignalRuleS3Macd,
				fmt.Sprintf("价创新低%.3f,1min MACD DIF不创新低(%.4f >= %.4f)", bar.Close, curDif, prevDifLow),
				bar.Close, prevLow, bar.Close*0.997, nil)
		}
	}
}

// evaluateS4 S4 早盘放量突破(文档4.2,默认关闭):9:45后突破开盘15分钟高点且量比>1.5。
func (e *TSignalEngine) evaluateS4(bar *MinuteBar) {
	params := e.state.Config.Rules
	if !params.S4Enabled {
		return
	}
	ref := e.builder.DayHighFirst15m()
	if ref <= 0 || bar.Close <= ref {
		return
	}
	if e.volumeRatio() <= params.S4VolRatio {
		return
	}
	if !strings.Contains(bar.Time, "T09:4") && !strings.Contains(bar.Time, "T10:") {
		return
	}
	e.emitSignal(bar, TSignalDirBuyBack, TSignalRuleS4Break,
		fmt.Sprintf("突破开盘15分钟高点%.3f,量比%.2f", ref, e.volumeRatio()),
		bar.Close, bar.Close*(1+params.S4TargetPct/100), ref, nil)
}

// evaluateS5 S5 急拉急跌反转(文档4.2):短窗口涨跌超阈值+现量放大+滞涨/止跌。
// 快照级触发,同向冷却窗口内不重复发。
func (e *TSignalEngine) evaluateS5(price float64, now time.Time) {
	params := e.state.Config.Rules
	if !params.S5Enabled {
		return
	}
	// 时间窗口(文档4.3):S5 同样只在 10:00-11:00 / 13:30-14:30 触发。
	hm := now.Hour()*100 + now.Minute()
	if !((hm >= 1000 && hm < 1100) || (hm >= 1330 && hm < 1430)) {
		return
	}
	cooldown := time.Duration(params.S5Cooldown) * time.Minute
	move := e.builder.RecentMovePct(60 * time.Second)
	avgIntuition := e.builder.RecentAvgIntuition(60 * time.Second)
	snaps := e.snapshotsCopy()
	if len(snaps) == 0 {
		return
	}
	latest := snaps[len(snaps)-1]
	if !e.builder.SnapshotTrusted(latest.Time) {
		return
	}
	intuitionBoost := avgIntuition > 0 && float64(latest.Intuition) > avgIntuition*2

	// 急拉反转:1分钟内涨超阈值+现量放大+出现滞涨(最新价回落/涨速转负) → SELL_FIRST。
	if move > params.S5MovePct && intuitionBoost {
		peak := 0.0
		for _, s := range snaps {
			if s.Price > peak {
				peak = s.Price
			}
		}
		stalled := latest.Price < peak*0.998 || latest.Rate < 0
		if stalled {
			if last, ok := e.lastS5[TSignalDirSellFirst]; ok && now.Sub(last) < cooldown {
				return
			}
			start := e.s5MoveStart(snaps, true)
			if !e.passFilters(TSignalDirSellFirst, price, start, price*(1+params.S5StopPct/100), minuteStartText(now)) ||
				!e.fiveMinuteDirectionAllows(TSignalDirSellFirst) {
				return
			}
			e.lastS5[TSignalDirSellFirst] = now
			e.emitSignalFromPrice(now, TSignalDirSellFirst, TSignalRuleS5Spike,
				fmt.Sprintf("1分钟内急拉%.2f%%,现量放大后滞涨", move),
				price, start, price*(1+params.S5StopPct/100), &TSignalFollowUp{Rule: "BUY_BACK_AT_SPIKE_START", Ref: start})
			return
		}
	}
	// 急跌反转:1分钟内跌超阈值+现量放大+止跌(最新价回升) → BUY_BACK。
	if move < -params.S5MovePct && intuitionBoost {
		bottom := price
		for _, s := range snaps {
			if s.Price < bottom {
				bottom = s.Price
			}
		}
		stopped := latest.Price > bottom*1.002 || latest.Rate > 0
		if stopped && e.hasOpenSellFirst() {
			if last, ok := e.lastS5[TSignalDirBuyBack]; ok && now.Sub(last) < cooldown {
				return
			}
			start := e.s5MoveStart(snaps, false)
			if !e.passFilters(TSignalDirBuyBack, price, start, price*(1-params.S5StopPct/100), minuteStartText(now)) ||
				!e.fiveMinuteDirectionAllows(TSignalDirBuyBack) {
				return
			}
			e.lastS5[TSignalDirBuyBack] = now
			e.emitSignalFromPrice(now, TSignalDirBuyBack, TSignalRuleS5Spike,
				fmt.Sprintf("1分钟内急跌%.2f%%,现量放大后止跌", move),
				price, start, price*(1-params.S5StopPct/100), nil)
		}
	}
}

func (e *TSignalEngine) snapshotsCopy() []MinuteBarSnapshot {
	out := make([]MinuteBarSnapshot, len(e.builder.snapshots))
	copy(out, e.builder.snapshots)
	return out
}

// s5MoveStart 急拉/急跌窗口起点价(目标价)。
func (e *TSignalEngine) s5MoveStart(snaps []MinuteBarSnapshot, up bool) float64 {
	if len(snaps) == 0 {
		return 0
	}
	start := snaps[0].Price
	if up {
		for _, s := range snaps {
			if s.Price < start {
				start = s.Price
			}
		}
	} else {
		for _, s := range snaps {
			if s.Price > start {
				start = s.Price
			}
		}
	}
	return start
}

// checkFollowUpBuyBack 检查已执行未回补的 SELL_FIRST 是否满足回补条件(文档4.4)。
func (e *TSignalEngine) checkFollowUpBuyBack(bar *MinuteBar) {
	for i := range e.state.Signals {
		sig := &e.state.Signals[i]
		if sig.Status != TSignalStatusExecuted || sig.Direction != TSignalDirSellFirst || sig.FollowUp == nil {
			continue
		}
		if e.ledgerPaired(sig.ID) {
			continue
		}
		ref := sig.FollowUp.Ref
		if ref <= 0 {
			continue
		}
		if bar.Close <= ref {
			e.emitSignal(bar, TSignalDirBuyBack, TSignalRuleFollowUp,
				fmt.Sprintf("回补条件触发:现价%.3f <= 参考价%.3f(%s)", bar.Close, ref, sig.FollowUp.Rule),
				bar.Close, ref, bar.Close*0.997, nil)
		}
	}
}

// tickLifecycle 推进信号生命周期:过期自动取消、收盘检查(文档4.4/5.4)。
func (e *TSignalEngine) tickLifecycle() {
	e.tickLifecycleAt(time.Now())
}

func (e *TSignalEngine) tickLifecycleAt(now time.Time) {
	for i := range e.state.Signals {
		sig := &e.state.Signals[i]
		if sig.Status == TSignalStatusNew {
			sig.Status = TSignalStatusActive
		}
		if sig.Status == TSignalStatusActive {
			if sig.ExpireAt != "" {
				if expire, err := time.Parse(time.RFC3339, sig.ExpireAt); err == nil && now.After(expire) {
					sig.Status = TSignalStatusCancelled
					sig.AckAt = nowText()
				}
			}
		}
	}
	// 14:55 后:存在未回补卖单时提示「放弃回补/隔日处理」。
	hm := now.Hour()*100 + now.Minute()
	if hm >= 1455 && hm < 1500 {
		for _, sig := range e.state.Signals {
			if sig.Status == TSignalStatusExecuted && sig.Direction == TSignalDirSellFirst && !e.ledgerPaired(sig.ID) {
				e.emitStandAside("收盘前仍有未回补卖单,请决策:放弃回补/隔日处理")
				break
			}
		}
	}
}

// emitSignal 分钟bar触发的信号(带数据质量过滤)。
func (e *TSignalEngine) emitSignal(bar *MinuteBar, direction, rule, trigger string, ref, target, stop float64, followUp *TSignalFollowUp) {
	if !bar.Trusted {
		return
	}
	if !e.passFilters(direction, ref, target, stop, bar.Time) {
		return
	}
	if !e.fiveMinuteDirectionAllows(direction) {
		return
	}
	e.emitSignalFromPrice(barTime(bar), direction, rule, trigger, ref, target, stop, followUp)
}

func (e *TSignalEngine) fiveMinuteDirectionAllows(direction string) bool {
	trend, ok := e.builder.LastFiveMinuteDirection()
	if !ok {
		return false
	}
	switch direction {
	case TSignalDirSellFirst:
		return trend == "down"
	case TSignalDirBuyBack:
		return trend == "up"
	default:
		return true
	}
}

// emitSignalFromPrice 生成并持久化一条信号(文档4.5 结构)。
func (e *TSignalEngine) emitSignalFromPrice(now time.Time, direction, rule, trigger string, ref, target, stop float64, followUp *TSignalFollowUp) {
	if ref <= 0 || target <= 0 || stop <= 0 {
		return // 文档4.1:没有目标价和止损价的信号视为噪音,不得发出
	}
	if direction == TSignalDirBuyBack && !e.hasOpenSellFirst() {
		return // 先卖后买:买回信号必须绑定未了结卖单
	}
	e.state.Counter++
	expire := now.Add(2 * time.Hour)
	if hm := now.Hour()*100 + now.Minute(); hm < 1100 {
		expire = time.Date(now.Year(), now.Month(), now.Day(), 11, 0, 0, 0, now.Location())
	} else if hm < 1430 {
		expire = time.Date(now.Year(), now.Month(), now.Day(), 14, 30, 0, 0, now.Location())
	} else {
		expire = time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
	}
	record := TSignalRecord{
		ID:           fmt.Sprintf("T-%s-%04d-%s", e.state.Day, e.state.Counter, rule),
		Symbol:       e.state.Config.Symbol,
		Time:         now.Format(time.RFC3339),
		Direction:    direction,
		Rule:         rule,
		Trigger:      trigger,
		RefPrice:     ref,
		Tolerance:    ref * 0.001, // 文档2.4:触发价留±0.1%余量
		Target:       target,
		Stop:         stop,
		ExpireAt:     expire.Format(time.RFC3339),
		PositionHint: "底仓的1/3",
		FollowUp:     followUp,
		Status:       TSignalStatusNew,
	}
	e.state.Signals = append(e.state.Signals, record)
	e.state.UpdatedAt = nowText()
	e.persist()

	e.notifyWebhook("t-signal.triggered", fmt.Sprintf("做T信号[%s] %s %s 触发价%.3f 目标%.3f 止损%.3f", record.ID, direction, rule, ref, target, stop), record)
}

// emitStandAside 观望/减仓类提示(不带目标止损,仅提示)。
func (e *TSignalEngine) emitStandAside(message string) {
	// 一天只提示一次,避免刷屏。
	for _, sig := range e.state.Signals {
		if sig.Direction == TSignalDirStandAside && strings.Contains(sig.Trigger, message) {
			return
		}
	}
	e.state.Counter++
	now := time.Now()
	record := TSignalRecord{
		ID:        fmt.Sprintf("T-%s-%04d-%s", e.state.Day, e.state.Counter, "NOTICE"),
		Symbol:    e.state.Config.Symbol,
		Time:      now.Format(time.RFC3339),
		Direction: TSignalDirStandAside,
		Rule:      "STAND_ASIDE",
		Trigger:   message,
		Status:    TSignalStatusNew,
	}
	e.state.Signals = append(e.state.Signals, record)
	e.persist()
	e.notifyWebhook("t-signal.notice", message, record)
}

// passFilters 全局过滤(文档4.3)。
func (e *TSignalEngine) passFilters(direction string, ref, target, stop float64, barTime string) bool {
	// 冷却:单日亏损达底仓1%或连亏3笔,当日停止出信号。
	if e.state.Cooldown.Active {
		return false
	}
	if e.state.Config.PositionShares < 3 || e.state.Config.PositionShares%3 != 0 {
		return false
	}
	// 时间窗口:只在 10:00-11:00 与 13:30-14:30 发新信号。
	t, err := time.Parse(time.RFC3339, barTime)
	if err != nil {
		return false
	}
	hm := t.Hour()*100 + t.Minute()
	if !((hm >= 1000 && hm < 1100) || (hm >= 1330 && hm < 1430)) {
		return false
	}
	// 成本门槛:预期往返收益 < 0.3% 不发(文档4.3)。
	fees := e.state.Config.Fees
	roundPct := mathAbs(target-ref)/ref*100 - fees.StampTax - fees.Commission*2 - fees.Slippage*2
	if roundPct < fees.MinRound {
		return false
	}
	// 趋势否决:强势单边不发卖出,弱势单边不发买回。
	if e.state.Config.TrendVeto {
		switch e.state.Trend.Status {
		case "strong_up":
			if direction == TSignalDirSellFirst {
				return false
			}
		case "strong_down":
			if direction == TSignalDirBuyBack {
				return false
			}
		case "limit":
			return false
		}
	}
	// 同向未了结信号不重复发(文档4.3)。
	if direction == TSignalDirSellFirst && e.hasOpenSellFirst() {
		return false
	}
	for _, sig := range e.state.Signals {
		if sig.Direction == direction && (sig.Status == TSignalStatusNew || sig.Status == TSignalStatusActive) {
			return false
		}
	}
	return true
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// hasOpenSellFirst 是否存在已执行未回补的卖出信号(先卖后买的前置条件)。
func (e *TSignalEngine) hasOpenSellFirst() bool {
	for _, sig := range e.state.Signals {
		if sig.Direction == TSignalDirSellFirst && sig.Status == TSignalStatusExecuted && !e.ledgerPaired(sig.ID) {
			return true
		}
	}
	return false
}

// ledgerPaired 卖出信号是否已在账本配对回补。
func (e *TSignalEngine) ledgerPaired(sellSignalID string) bool {
	for _, entry := range e.state.Ledger {
		if entry.SellSignalID == sellSignalID && entry.Closed {
			return true
		}
	}
	return false
}

// AckSignal 人工标记信号 EXECUTED / CANCELLED(文档4.4)。
func (e *TSignalEngine) AckSignal(id, action string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.state.Signals {
		sig := &e.state.Signals[i]
		if sig.ID != id {
			continue
		}
		if sig.Status != TSignalStatusNew && sig.Status != TSignalStatusActive {
			return fmt.Errorf("信号 %s 当前状态为 %s,不能再次确认", id, sig.Status)
		}
		switch action {
		case "EXECUTED":
			if e.state.Config.PositionShares < 3 || e.state.Config.PositionShares%3 != 0 {
				return fmt.Errorf("请先录入可被3整除的底仓股数")
			}
			if err := e.onExecuted(*sig); err != nil {
				return err
			}
			sig.Status = TSignalStatusExecuted
			sig.AckAt = nowText()
		case "CANCELLED":
			sig.Status = TSignalStatusCancelled
			sig.AckAt = nowText()
		default:
			return fmt.Errorf("不支持的操作: %s", action)
		}
		e.state.UpdatedAt = nowText()
		e.persist()
		e.notifyWebhook("t-signal.acked", fmt.Sprintf("做T信号[%s] 已标记 %s", sig.ID, action), *sig)
		return nil
	}
	return fmt.Errorf("信号不存在: %s", id)
}

// onExecuted 信号成交后的记账与配对(文档3.2 买卖成对)。
func (e *TSignalEngine) onExecuted(sig TSignalRecord) error {
	shares := e.state.Config.PositionShares / 3
	switch sig.Direction {
	case TSignalDirSellFirst:
		entry := TSignalLedgerEntry{
			ID:           fmt.Sprintf("L-%s", sig.ID),
			SellSignalID: sig.ID,
			SellPrice:    sig.RefPrice,
			SellTime:     sig.Time,
			Shares:       shares,
			Closed:       false,
		}
		e.state.Ledger = append(e.state.Ledger, entry)
	case TSignalDirBuyBack:
		for i := range e.state.Ledger {
			entry := &e.state.Ledger[i]
			if entry.Closed {
				continue
			}
			entry.BuySignalID = sig.ID
			entry.BuyPrice = sig.RefPrice
			entry.BuyTime = sig.Time
			entry.Closed = true
			// 净盈亏(含费):(卖-买)*股数 - 佣金双边 - 印花税。
			fees := e.state.Config.Fees
			gross := (entry.SellPrice - entry.BuyPrice) * float64(entry.Shares)
			cost := (entry.SellPrice+entry.BuyPrice)*float64(entry.Shares)*(fees.Commission/100) +
				entry.SellPrice*float64(entry.Shares)*(fees.StampTax/100) +
				(entry.SellPrice+entry.BuyPrice)*float64(entry.Shares)*(fees.Slippage/100)
			entry.NetPnl = gross - cost
			e.updateCooldown(entry.NetPnl)
			return nil
		}
		return fmt.Errorf("没有可配对的卖出成交记录")
	}
	return nil
}

// updateCooldown 冷却判定(文档5.5):单日亏损达底仓1%或连亏3笔。
func (e *TSignalEngine) updateCooldown(netPnl float64) {
	total := 0.0
	for _, entry := range e.state.Ledger {
		if entry.Closed {
			total += entry.NetPnl
		}
	}
	e.state.Cooldown.LossPnl = total
	positionValue := float64(e.state.Config.PositionShares) * e.state.PrevClose
	if netPnl < 0 {
		e.consecLosses++
	} else {
		e.consecLosses = 0
	}
	if positionValue > 0 && total < -positionValue*0.01 {
		e.state.Cooldown = TSignalCooldown{Active: true, Reason: "单日做T亏损达底仓1%,当日停止出信号", LossPnl: total}
		log.Printf("做T信号冷却: %s", e.state.Cooldown.Reason)
		return
	}
	if e.consecLosses >= 3 {
		e.state.Cooldown = TSignalCooldown{Active: true, Reason: "连续亏损3笔,当日停止出信号", LossPnl: total}
		log.Printf("做T信号冷却: %s", e.state.Cooldown.Reason)
		return
	}
	e.state.Cooldown.Active = false
}

// resetDaily 跨日重置当日信号、账本与冷却。
func (e *TSignalEngine) resetDaily() {
	e.mu.Lock()
	e.resetDailyLocked(time.Now())
	day := e.state.Day
	e.persist()
	e.mu.Unlock()
	go e.refreshPrevDayVolume()
	log.Printf("做T信号引擎已重置(新交易日 %s)", day)
}

func (e *TSignalEngine) resetDailyLocked(at time.Time) {
	e.state.Day = at.Format("20060102")
	e.state.Signals = nil
	e.state.Ledger = nil
	e.state.Cooldown = TSignalCooldown{}
	e.state.Counter = 0
	e.state.PrevClose = 0
	e.state.PrevVol = 0
	e.builder = NewMinuteBarBuilder()
	e.difHistory = nil
	e.consecLosses = 0
	e.s1HighPending, e.s1LowPending = false, false
	e.s1HighPeak, e.s1LowTrough = 0, 0
	e.s1HighAt, e.s1LowAt = time.Time{}, time.Time{}
	e.lastS5 = map[string]time.Time{}
}

// notifyWebhook 通过项目已有 Webhook 通道推送(文档1.1)。
func (e *TSignalEngine) notifyWebhook(event, message string, record TSignalRecord) {
	if e.store == nil {
		return
	}
	hooks, err := e.store.ListEnabledWebhooks()
	if err != nil {
		return
	}
	logs := sendWebhooks(context.Background(), hooks, WebhookEvent{
		Event: event, Status: "triggered", Message: message,
		MatchedCount: 1, MatchedSymbols: []string{record.Symbol}, Result: record,
	})
	if len(logs) > 0 {
		log.Printf("做T信号Webhook: %s", strings.Join(logs, "; "))
	}
}

// restore 启动时从 t_signal_state 恢复状态;跨日则重置。
func (e *TSignalEngine) restore() {
	if e.store == nil {
		return
	}
	record, err := e.store.GetTSignalState("t-signal-engine")
	if err != nil || record.StateJSON == "" {
		e.state = TSignalState{Config: TSignalConfig{Rules: DefaultTSignalRuleParams(), Fees: DefaultTSignalFeeConfig()}}
		e.builder = NewMinuteBarBuilder()
		return
	}
	if err := json.Unmarshal([]byte(record.StateJSON), &e.state); err != nil {
		log.Printf("做T信号状态恢复失败: %v", err)
		e.state = TSignalState{Config: TSignalConfig{Rules: DefaultTSignalRuleParams(), Fees: DefaultTSignalFeeConfig()}}
	}
	if e.state.Config.Rules.S1DevPct == 0 {
		e.state.Config.Rules = DefaultTSignalRuleParams()
	}
	if e.state.Config.Fees.MinRound == 0 {
		e.state.Config.Fees = DefaultTSignalFeeConfig()
	}
	if e.state.Day != "" && e.state.Day != dateText() {
		e.resetDailyLocked(time.Now())
	}
	if e.state.Day == "" {
		e.state.Day = dateText()
	}
	e.builder = NewMinuteBarBuilder()
	e.consecLosses = 0
	for _, entry := range e.state.Ledger {
		if entry.Closed && entry.NetPnl < 0 {
			e.consecLosses++
		} else if entry.Closed {
			e.consecLosses = 0
		}
	}
}

// persist 状态落库(t_signal_state 表,workbench-core 提供存取)。
func (e *TSignalEngine) persist() {
	if e.store == nil {
		return
	}
	raw, err := json.Marshal(e.state)
	if err != nil {
		return
	}
	_, err = e.store.UpsertTSignalState(workbench.TSignalStateRecord{ID: "t-signal-engine", StateJSON: string(raw)})
	if err != nil {
		log.Printf("做T信号状态持久化失败: %v", err)
	}
}

// UpdateConfig 更新配置(API调用);换标的需确认并清空当日信号与账本(UI文档3.1)。
func (e *TSignalEngine) UpdateConfig(cfg TSignalConfig, force bool) error {
	e.mu.Lock()
	oldSymbol := strings.TrimSpace(e.state.Config.Symbol)
	newSymbol := strings.TrimSpace(cfg.Symbol)
	if oldSymbol != "" && newSymbol != oldSymbol && !force {
		e.mu.Unlock()
		return fmt.Errorf("当日已锁定标的 %s,换标的需确认(force=true)且将清空当日信号与账本", oldSymbol)
	}
	if cfg.PositionShares < 0 || (cfg.PositionShares > 0 && cfg.PositionShares%3 != 0) {
		e.mu.Unlock()
		return fmt.Errorf("底仓股数必须为0或3的整数倍")
	}
	if newSymbol != oldSymbol {
		e.resetDailyLocked(time.Now())
	}
	if cfg.Rules.S1DevPct == 0 {
		cfg.Rules = DefaultTSignalRuleParams()
	}
	if cfg.Fees.MinRound == 0 {
		cfg.Fees = DefaultTSignalFeeConfig()
	}
	cfg.UpdatedAt = nowText()
	e.state.Config = cfg
	e.state.UpdatedAt = nowText()
	e.persist()
	e.mu.Unlock()
	// 标的变化后异步刷新昨日量(量比基准)。
	go e.refreshPrevDayVolume()
	if newSymbol != oldSymbol {
		e.requestStreamRestart()
	}
	return nil
}

// refreshPrevDayVolume 拉取昨日全天成交量(量比基准,文档3.3)。
func (e *TSignalEngine) refreshPrevDayVolume() {
	symbol := strings.TrimSpace(e.currentSymbol())
	e.refreshPrevDayVolumeForSymbol(symbol)
}

func (e *TSignalEngine) refreshPrevDayVolumeForSymbol(symbol string) {
	if symbol == "" || e.market == nil {
		return
	}
	code := intradayQuoteCode(symbol)
	resp, err := e.market.KlineHistory(context.Background(), code, "day", 5)
	if err != nil || resp == nil {
		return
	}
	today := time.Now().Format("20060102")
	for i := len(resp.List) - 1; i >= 0; i-- {
		k := resp.List[i]
		if k == nil || k.Volume <= 0 {
			continue
		}
		day := k.Time.Format("20060102")
		if day < today {
			e.mu.Lock()
			if strings.TrimSpace(e.state.Config.Symbol) != symbol {
				e.mu.Unlock()
				return
			}
			e.state.PrevVol = float64(k.Volume)
			e.state.PrevClose = k.Close.Float64()
			e.state.UpdatedAt = nowText()
			e.persist()
			e.mu.Unlock()
			return
		}
	}
}

// State 返回状态副本(API用)。
func (e *TSignalEngine) State() TSignalState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// BarsSnapshot 返回分钟bar序列(含当前未完成bar,升序)。
func (e *TSignalEngine) BarsSnapshot() []MinuteBar {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.builder == nil {
		return nil
	}
	bars := append([]MinuteBar(nil), e.builder.Completed()...)
	if cur := e.builder.Current(); cur != nil {
		bars = append(bars, *cur)
	}
	return bars
}

// Health 数据流健康度(UI文档4.2 state接口)。
func (e *TSignalEngine) Health() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	streaming := !e.lastSnapshot.IsZero() && time.Since(e.lastSnapshot) < 30*time.Second
	vwap, ma5, volRatio := 0.0, 0.0, 0.0
	if e.builder != nil {
		vwap = e.builder.VWAP()
		ma5 = e.builder.MA(5)
		volRatio = e.volumeRatio()
	}
	return map[string]interface{}{
		"last_snapshot_at": func() string {
			if e.lastSnapshot.IsZero() {
				return ""
			}
			return e.lastSnapshot.Format(time.RFC3339)
		}(),
		"streaming":   streaming,
		"vwap":        vwap,
		"ma5":         ma5,
		"vol_ratio":   volRatio,
		"day":         e.state.Day,
		"prev_close":  e.state.PrevClose,
		"prev_volume": e.state.PrevVol,
	}
}
