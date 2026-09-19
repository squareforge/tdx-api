package main

import (
	"testing"
	"time"
)

func newTestEngine() *TSignalEngine {
	return &TSignalEngine{
		lastS5: map[string]time.Time{},
		state: TSignalState{
			Day: "20260918",
			Config: TSignalConfig{
				Symbol:         "sz000001",
				PositionShares: 3000,
				TrendVeto:      true,
				Rules:          DefaultTSignalRuleParams(),
				Fees:           DefaultTSignalFeeConfig(),
			},
			PrevClose: 10.0,
		},
		builder: NewMinuteBarBuilder(),
	}
}

// TestEngineHealthWithoutMarket 没有行情服务时，配置与健康接口仍应可安全读取。
func TestEngineHealthWithoutMarket(t *testing.T) {
	e := NewTSignalEngine(nil, nil)
	if e.builder == nil {
		t.Fatal("新引擎应预先初始化分钟bar合成器")
	}
	if e.state.Config.Rules.S1DevPct == 0 || e.state.Config.Fees.MinRound == 0 {
		t.Fatalf("新引擎应提供默认配置，得到 %+v", e.state.Config)
	}
	health := e.Health()
	if health["vwap"] != 0.0 || health["ma5"] != 0.0 {
		t.Fatalf("无行情时健康指标应为0，得到 %+v", health)
	}
}

// TestPassFiltersTimeWindow 时间窗口过滤:仅 10:00-11:00 / 13:30-14:30 放行。
func TestPassFiltersTimeWindow(t *testing.T) {
	e := newTestEngine()
	cases := []struct {
		barTime string
		want    bool
	}{
		{"2026-09-18T09:45:00+08:00", false},
		{"2026-09-18T10:00:00+08:00", true},
		{"2026-09-18T10:59:00+08:00", true},
		{"2026-09-18T11:01:00+08:00", false},
		{"2026-09-18T13:30:00+08:00", true},
		{"2026-09-18T14:29:00+08:00", true},
		{"2026-09-18T14:31:00+08:00", false},
	}
	for _, c := range cases {
		got := e.passFilters(TSignalDirSellFirst, 10.0, 9.9, 10.08, c.barTime)
		if got != c.want {
			t.Fatalf("时间窗口 %s 期望 %v,得到 %v", c.barTime, c.want, got)
		}
	}
}

// TestPassFiltersCostGate 成本门槛:预期往返收益 < 0.3% 不发。
func TestPassFiltersCostGate(t *testing.T) {
	e := newTestEngine()
	// 目标差 0.1%(10.00→9.99) - 印花0.05 - 佣金0.05 - 滑点0.1 < 0.3 → 不发。
	if e.passFilters(TSignalDirSellFirst, 10.0, 9.99, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("低收益信号应被成本门槛过滤")
	}
	// 目标差 1%(10.00→9.90) → 通过。
	if !e.passFilters(TSignalDirSellFirst, 10.0, 9.90, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("1%预期收益应通过成本门槛")
	}
}

// TestPassFiltersTrendVeto 趋势否决:强势单边不发卖出。
func TestPassFiltersTrendVeto(t *testing.T) {
	e := newTestEngine()
	e.state.Trend = TSignalTrendVeto{Status: "strong_up"}
	if e.passFilters(TSignalDirSellFirst, 10.0, 9.9, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("强势单边不应发卖出信号")
	}
	if !e.passFilters(TSignalDirBuyBack, 10.0, 10.1, 9.92, "2026-09-18T10:30:00+08:00") {
		t.Fatal("强势单边不应阻止买回信号")
	}
	e.state.Trend = TSignalTrendVeto{Status: "limit"}
	if e.passFilters(TSignalDirSellFirst, 10.0, 9.9, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("涨跌停附近不应发信号")
	}
}

// TestPassFiltersSameDirection 同向未了结信号不重复发。
func TestPassFiltersSameDirection(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{
		ID: "T-1", Direction: TSignalDirSellFirst, Status: TSignalStatusActive,
	})
	if e.passFilters(TSignalDirSellFirst, 10.0, 9.9, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("存在未了结同向信号时不应重复发")
	}
	if !e.passFilters(TSignalDirBuyBack, 10.0, 10.1, 9.92, "2026-09-18T10:30:00+08:00") {
		t.Fatal("反向信号不应被阻止")
	}
}

// TestPassFiltersOpenSellFirst 已成交但未回补时，不得再次发出卖出信号。
func TestPassFiltersOpenSellFirst(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{
		ID: "T-1", Direction: TSignalDirSellFirst, Status: TSignalStatusExecuted,
	})
	if e.passFilters(TSignalDirSellFirst, 10.0, 9.9, 10.08, "2026-09-18T10:30:00+08:00") {
		t.Fatal("已成交未回补卖单存在时不应再次发出 SELL_FIRST")
	}
}

func TestProtectiveAndTimedBuyBackAlerts(t *testing.T) {
	e := newTestEngine()
	e.state.Config.Rules.S6MaxHoldMinutes = 150
	sell := TSignalRecord{
		ID: "T-S6", Symbol: "sz000001", Direction: TSignalDirSellFirst,
		Rule: TSignalRuleS6Vwap, RefPrice: 10, Target: 9.93, Stop: 10.40,
		Status: TSignalStatusExecuted, Time: ts(18, 10, 0, 0).Format(time.RFC3339),
	}
	e.state.Signals = append(e.state.Signals, sell)
	e.state.Ledger = append(e.state.Ledger, TSignalLedgerEntry{ID: "L-T-S6", SellSignalID: sell.ID, SellPrice: 10, Shares: 1000})

	// A touched high must alert even if the minute closes back below the stop.
	stopBar := MinuteBar{Time: ts(18, 10, 5, 0).Format(time.RFC3339), High: 10.41, Close: 10.20, Trusted: true, Complete: true}
	e.checkProtectiveBuyBack(&stopBar)
	if len(e.state.Signals) != 2 || e.state.Signals[1].Rule != TSignalRuleStopExit {
		t.Fatalf("分钟内触及止损应发回补提示: %+v", e.state.Signals)
	}

	// Remove the pending stop alert and verify 150 trading minutes excludes lunch.
	e.state.Signals = e.state.Signals[:1]
	timedBar := MinuteBar{Time: ts(18, 14, 0, 0).Format(time.RFC3339), High: 10.20, Close: 10.10, Trusted: true, Complete: true}
	e.checkTimedBuyBack(&timedBar)
	if len(e.state.Signals) != 2 || e.state.Signals[1].Rule != TSignalRuleCloseExit {
		t.Fatalf("达到最长交易分钟应发回补提示: %+v", e.state.Signals)
	}
	if got := tradingMinuteOffset(ts(18, 13, 0, 0)) - tradingMinuteOffset(ts(18, 11, 30, 0)); got != 0 {
		t.Fatalf("午休不应计入持有分钟，得到 %d", got)
	}
}

// TestAckAndLedger 信号成交后的记账与配对。
func TestAckAndLedger(t *testing.T) {
	e := newTestEngine()
	sell := TSignalRecord{
		ID: "T-1", Symbol: "sz000001", Direction: TSignalDirSellFirst,
		Rule: TSignalRuleS1Vwap, RefPrice: 10.0, Target: 9.8, Stop: 10.08,
		Status: TSignalStatusNew, Time: "2026-09-18T10:30:00+08:00",
		FollowUp: &TSignalFollowUp{Rule: "BUY_BACK_AT_VWAP", Ref: 9.8},
	}
	e.state.Signals = append(e.state.Signals, sell)
	if err := e.AckSignal("T-1", "EXECUTED"); err != nil {
		t.Fatalf("标记成交失败: %v", err)
	}
	if len(e.state.Ledger) != 1 || e.state.Ledger[0].Closed {
		t.Fatalf("卖出成交应开未配对账本条目: %+v", e.state.Ledger)
	}
	if e.state.Ledger[0].Shares != 1000 { // 底仓3000的1/3
		t.Fatalf("仓位应为底仓1/3(1000),得到 %v", e.state.Ledger[0].Shares)
	}
	if !e.hasOpenSellFirst() {
		t.Fatal("应存在未回补卖单")
	}

	buy := TSignalRecord{
		ID: "T-2", Symbol: "sz000001", Direction: TSignalDirBuyBack,
		Rule: TSignalRuleFollowUp, RefPrice: 9.8, Target: 10.0, Stop: 9.77,
		Status: TSignalStatusNew, Time: "2026-09-18T13:40:00+08:00",
	}
	e.state.Signals = append(e.state.Signals, buy)
	if err := e.AckSignal("T-2", "EXECUTED"); err != nil {
		t.Fatalf("标记买回成交失败: %v", err)
	}
	if len(e.state.Ledger) != 1 || !e.state.Ledger[0].Closed {
		t.Fatalf("买回应配对关闭账本条目: %+v", e.state.Ledger)
	}
	// 净盈亏:(10.0-9.8)*1000 - 佣金双边 - 印花税 - 滑点 > 0。
	if e.state.Ledger[0].NetPnl <= 0 {
		t.Fatalf("该笔应为盈利,得到 %v", e.state.Ledger[0].NetPnl)
	}
	if e.hasOpenSellFirst() {
		t.Fatal("配对后不应再有未回补卖单")
	}
}

func TestReplayMinuteBarsCancelsGapThroughTargetWithoutMutatingLiveState(t *testing.T) {
	live := newTestEngine()
	live.state.Config.TrendVeto = false
	live.state.Config.Rules = DefaultTSignalRuleParams()
	live.state.Config.Rules.S1Enabled = false
	live.state.Config.Rules.S3Enabled = false
	live.state.Config.Rules.S4Enabled = false
	live.state.Config.Rules.S5Enabled = false

	bar := func(hour, minute int, open, high, low, close float64) MinuteBar {
		return MinuteBar{
			Time:     ts(18, hour, minute, 0).Format(time.RFC3339),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   1000,
			Amount:   close * 1000,
			Complete: true,
			Trusted:  true,
		}
	}
	bars := []MinuteBar{
		bar(9, 59, 10.00, 10.00, 9.80, 9.90),
		bar(10, 0, 10.20, 10.20, 10.05, 10.10),
		bar(10, 1, 10.10, 10.10, 9.98, 10.00),
		bar(10, 2, 10.00, 10.00, 9.93, 9.95),
		bar(10, 3, 9.95, 10.10, 9.88, 9.90),
		// 10:04 completes a downward 5-minute window; the high touches MA while close remains below it.
		bar(10, 4, 9.90, 10.10, 9.82, 9.85),
		bar(10, 5, 9.55, 9.62, 9.55, 9.60),
		bar(10, 6, 9.60, 9.67, 9.60, 9.65),
		bar(10, 7, 9.65, 9.72, 9.65, 9.68),
		bar(10, 8, 9.68, 9.95, 9.68, 9.70),
		// 10:09 completes an upward 5-minute window and retests the MA from above.
		bar(10, 9, 9.70, 9.77, 9.63, 9.75),
	}

	result, err := live.ReplayMinuteBars(TSignalReplayRequest{Bars: bars, PrevClose: 10})
	if err != nil {
		t.Fatalf("回放计算失败: %v", err)
	}
	if len(result.Frames) != len(bars) {
		t.Fatalf("回放帧数应等于分钟bar数，得到 %d", len(result.Frames))
	}
	last := result.Frames[len(result.Frames)-1]
	if len(last.Signals) != 1 || last.Signals[0].Direction != TSignalDirSellFirst || last.Signals[0].Status != TSignalStatusCancelled {
		t.Fatalf("下一根开盘越过目标价时应取消卖出计划，得到 %+v", last.Signals)
	}
	if len(last.Ledger) != 0 {
		t.Fatalf("未成交计划不应写入账本，得到 %+v", last.Ledger)
	}
	if len(live.state.Signals) != 0 || len(live.state.Ledger) != 0 {
		t.Fatalf("回放不得修改实时引擎状态，signals=%+v ledger=%+v", live.state.Signals, live.state.Ledger)
	}
}

func TestReplayS6UsesNextBarOpenAndClosesAtTarget(t *testing.T) {
	live := newTestEngine()
	live.state.Config.TrendVeto = false
	live.state.Config.Rules = DefaultTSignalRuleParams()
	live.state.Config.Rules.S1Enabled = false
	live.state.Config.Rules.S2Enabled = false
	live.state.Config.Rules.S3Enabled = false
	live.state.Config.Rules.S4Enabled = false
	live.state.Config.Rules.S5Enabled = false
	live.state.Config.Rules.S6Enabled = true
	live.state.PrevVol = 2400
	live.state.PrevDayReturn = 0

	closes := []float64{10, 10, 10, 10, 10, 10.30, 10.25}
	bars := make([]MinuteBar, 0, 36)
	for group, close := range closes {
		for minute := 0; minute < 5; minute++ {
			at := ts(18, 9, 30+group*5+minute, 0)
			bars = append(bars, MinuteBar{
				Time: at.Format(time.RFC3339), Open: close, High: close, Low: close, Close: close,
				Volume: 1000, Amount: close * 1000, Complete: true, Trusted: true,
			})
		}
	}
	entryAt := ts(18, 10, 5, 0)
	bars = append(bars, MinuteBar{
		Time: entryAt.Format(time.RFC3339), Open: 10.20, High: 10.22, Low: 10.10, Close: 10.12,
		Volume: 1000, Amount: 10.12 * 1000, Complete: true, Trusted: true,
	})

	result, err := live.ReplayMinuteBars(TSignalReplayRequest{Bars: bars, PrevClose: 10})
	if err != nil {
		t.Fatalf("S6回放失败: %v", err)
	}
	last := result.Frames[len(result.Frames)-1]
	if len(last.Signals) != 2 || last.Signals[0].Rule != TSignalRuleS6Vwap || last.Signals[1].Direction != TSignalDirBuyBack {
		t.Fatalf("S6应形成卖出和目标回补信号，得到 %+v", last.Signals)
	}
	if len(last.Ledger) != 1 || !last.Ledger[0].Closed {
		t.Fatalf("S6回放应闭环，得到 %+v", last.Ledger)
	}
	if last.Ledger[0].SellPrice != 10.20 {
		t.Fatalf("卖出应使用下一根开盘价10.20，得到 %.3f", last.Ledger[0].SellPrice)
	}
	if last.Ledger[0].NetPnl <= 0 || result.Summary.ClosedTrades != 1 || result.Summary.Wins != 1 {
		t.Fatalf("目标回补应产生一笔含费盈利，ledger=%+v summary=%+v", last.Ledger, result.Summary)
	}
	if len(live.state.Signals) != 0 || len(live.state.Ledger) != 0 {
		t.Fatal("隔离回放不得污染实时引擎")
	}
}

func TestUpdateConfigAllowsExplicitEmptyReset(t *testing.T) {
	e := newTestEngine()
	e.state.Config.Symbol = "sh600570"
	if err := e.UpdateConfig(TSignalConfig{}, true); err != nil {
		t.Fatalf("清空配置失败: %v", err)
	}
	if e.state.Config.Symbol != "" || e.state.Config.PositionShares != 0 || e.state.Config.Rules.S1DevPct != 0 || e.state.Config.Fees.MinRound != 0 {
		t.Fatalf("清空配置后不应自动恢复默认值: %+v", e.state.Config)
	}
}

func TestUpdateConfigChangesSymbolWithoutConfirmationWhenNoIntradayBars(t *testing.T) {
	e := newTestEngine()
	next := e.state.Config
	next.Symbol = "sh600570"
	if err := e.UpdateConfig(next, false); err != nil {
		t.Fatalf("没有分时bar时切换标的不应要求确认: %v", err)
	}
	if e.state.Config.Symbol != "sh600570" {
		t.Fatalf("标的未更新: %+v", e.state.Config)
	}
}

func TestUpdateConfigRequiresConfirmationWhenIntradayBarsExist(t *testing.T) {
	e := newTestEngine()
	e.builder.current = &MinuteBar{Time: "2026-09-18T10:30:00+08:00", Close: 10}
	next := e.state.Config
	next.Symbol = "sh600570"
	if err := e.UpdateConfig(next, false); err == nil {
		t.Fatal("已有分时bar时切换标的应要求确认")
	}
	if err := e.UpdateConfig(next, true); err != nil {
		t.Fatalf("确认后切换标的失败: %v", err)
	}
	if len(e.BarsSnapshot()) != 0 {
		t.Fatalf("确认切换后应清空分时bar: %+v", e.BarsSnapshot())
	}
}

// TestAckCancel 放弃信号。
func TestAckCancel(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{ID: "T-1", Status: TSignalStatusNew})
	if err := e.AckSignal("T-1", "CANCELLED"); err != nil {
		t.Fatalf("标记放弃失败: %v", err)
	}
	if e.state.Signals[0].Status != TSignalStatusCancelled {
		t.Fatal("状态应为 CANCELLED")
	}
}

// TestAckSignalIdempotency 重复成交确认不能重复记账。
func TestAckSignalIdempotency(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{
		ID: "T-1", Direction: TSignalDirSellFirst, RefPrice: 10,
		Status: TSignalStatusNew, Time: "2026-09-18T10:30:00+08:00",
	})
	if err := e.AckSignal("T-1", "EXECUTED"); err != nil {
		t.Fatalf("首次成交确认失败: %v", err)
	}
	if err := e.AckSignal("T-1", "EXECUTED"); err == nil {
		t.Fatal("重复成交确认应返回错误")
	}
	if len(e.state.Ledger) != 1 {
		t.Fatalf("重复确认不应新增账本条目，得到 %+v", e.state.Ledger)
	}
}

// TestTickLifecyclePromotesNew 生命周期应从 NEW 推进到 ACTIVE。
func TestTickLifecyclePromotesNew(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{ID: "T-1", Status: TSignalStatusNew})
	e.tickLifecycleAt(ts(18, 10, 30, 0))
	if e.state.Signals[0].Status != TSignalStatusActive {
		t.Fatalf("NEW 应推进为 ACTIVE，得到 %s", e.state.Signals[0].Status)
	}
}

// TestCooldownConsecutiveLosses 连亏3笔触发冷却。
func TestCooldownConsecutiveLosses(t *testing.T) {
	e := newTestEngine()
	e.updateCooldown(-10)
	if e.state.Cooldown.Active {
		t.Fatal("一笔亏损不应触发冷却")
	}
	e.updateCooldown(-10)
	if e.state.Cooldown.Active {
		t.Fatal("两笔亏损不应触发冷却")
	}
	e.updateCooldown(-10)
	if !e.state.Cooldown.Active {
		t.Fatal("连亏3笔应触发冷却")
	}
}

// TestCooldownDailyLoss 单日亏损达底仓1%触发冷却。
func TestCooldownDailyLoss(t *testing.T) {
	e := newTestEngine()
	// 底仓市值 3000*10=30000,1% = 300。
	e.state.Ledger = append(e.state.Ledger, TSignalLedgerEntry{Closed: true, NetPnl: -350})
	e.updateCooldown(-350)
	if !e.state.Cooldown.Active {
		t.Fatal("单日亏损达底仓1%应触发冷却")
	}
}

// TestNearLimitPrice 涨跌停附近判定(主板10%)。
func TestNearLimitPrice(t *testing.T) {
	e := newTestEngine()
	if !e.nearLimitPrice(10.95) { // 涨停10.99的99.6%内
		t.Fatal("接近涨停应判定为涨跌停附近")
	}
	if !e.nearLimitPrice(9.05) {
		t.Fatal("接近跌停应判定为涨跌停附近")
	}
	if e.nearLimitPrice(10.5) {
		t.Fatal("中间价位不应判定为涨跌停附近")
	}
}

// TestFollowUpBuyBack 回补条件触发。
func TestFollowUpBuyBack(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{
		ID: "T-1", Direction: TSignalDirSellFirst, Status: TSignalStatusExecuted,
		FollowUp: &TSignalFollowUp{Rule: "BUY_BACK_AT_VWAP", Ref: 9.8},
	})
	// 13:30-13:34 是连续完整的上涨5分钟K，满足 BUY_BACK 的方向确认。
	for minute := 30; minute <= 34; minute++ {
		e.builder.completed = append(e.builder.completed, MinuteBar{
			Time: ts(18, 13, minute, 0).Format(time.RFC3339), Open: 9.5, Close: 9.9, Complete: true, Trusted: true,
		})
	}
	bar := &MinuteBar{Time: "2026-09-18T13:35:00+08:00", Close: 9.7, Trusted: true}
	before := len(e.state.Signals)
	e.checkFollowUpBuyBack(bar)
	// checkFollowUpBuyBack 内部走 emitSignal→passFilters,时间窗口 13:35 内应触发。
	if len(e.state.Signals) != before+1 {
		t.Fatalf("回补条件满足应发 BUY_BACK 信号,信号数 %d → %d", before, len(e.state.Signals))
	}
	if e.state.Signals[before].Direction != TSignalDirBuyBack {
		t.Fatalf("应发 BUY_BACK,得到 %s", e.state.Signals[before].Direction)
	}
}

// TestS3ExcludesCurrentBar 回归当前bar不能被拿来当作“此前高点”，否则背离永远无法触发。
func TestS3ExcludesCurrentBar(t *testing.T) {
	e := newTestEngine()
	e.state.Config.Rules.S3Lookback = 2
	prices := []float64{102, 100, 99.8, 100, 101}
	for i, price := range prices {
		e.builder.completed = append(e.builder.completed, MinuteBar{
			Time: ts(18, 10, i, 0).Format(time.RFC3339), Open: price, Close: price, Complete: true, Trusted: true,
		})
	}
	// 最近5分钟K从102回落至101，允许 SELL_FIRST；当前DIF低于此前峰值，构成顶背离。
	e.difHistory = []float64{0.2, 0.5, 1.0, 0.8, 0.6}
	e.evaluateS3(&e.builder.completed[len(e.builder.completed)-1])
	if len(e.state.Signals) != 1 || e.state.Signals[0].Direction != TSignalDirSellFirst {
		t.Fatalf("当前bar创新高且DIF未创新高应触发 SELL_FIRST，得到 %+v", e.state.Signals)
	}
}

// TestSignalExpiry 信号过期自动取消。
func TestSignalExpiry(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{
		ID: "T-1", Status: TSignalStatusNew,
		ExpireAt: time.Now().Add(-time.Minute).Format(time.RFC3339),
	})
	e.tickLifecycle()
	if e.state.Signals[0].Status != TSignalStatusCancelled {
		t.Fatal("过期信号应自动取消")
	}
}

// TestResetDaily 跨日重置。
func TestResetDaily(t *testing.T) {
	e := newTestEngine()
	e.state.Signals = append(e.state.Signals, TSignalRecord{ID: "T-1", Status: TSignalStatusNew})
	e.state.Ledger = append(e.state.Ledger, TSignalLedgerEntry{ID: "L-1"})
	e.state.Cooldown = TSignalCooldown{Active: true}
	e.resetDaily()
	if len(e.state.Signals) != 0 || len(e.state.Ledger) != 0 {
		t.Fatal("跨日应清空信号与账本")
	}
	if e.state.Cooldown.Active {
		t.Fatal("跨日应清除冷却")
	}
}
