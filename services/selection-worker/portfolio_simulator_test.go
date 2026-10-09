package main

import (
	"math"
	"strings"
	"testing"
)

// 撮合引擎单元测试:全部使用合成日K数据,不依赖本地行情库。
// 日期取连续整数(YYYYMMDD),引擎不校验星期,测试语义与交易日一致。

type pfBar struct {
	date   int
	yclose float64
	open   float64
	high   float64
	low    float64
	close  float64
}

func pfKline(bars []pfBar) []FormulaKline {
	rows := make([]FormulaKline, len(bars))
	for i, b := range bars {
		high, low := b.high, b.low
		if high == 0 {
			high = math.Max(b.open, b.close)
		}
		if low == 0 {
			low = math.Min(b.open, b.close)
		}
		rows[i] = FormulaKline{Date: b.date, YClose: b.yclose, Open: b.open, High: high, Low: low, Close: b.close}
	}
	return rows
}

func pfBuy(symbol string, score float64, signalDate int, strategy string) portfolioPendingBuy {
	return portfolioPendingBuy{symbol: symbol, score: score, signalDate: signalDate, strategyName: strategy}
}

func pfTrades(t *testing.T, sim *portfolioSimulator, runID string) []HistoricalBacktestTrade {
	t.Helper()
	return sim.finalize(runID)
}

func pfSingleTrade(t *testing.T, sim *portfolioSimulator, runID string) HistoricalBacktestTrade {
	t.Helper()
	trades := pfTrades(t, sim, runID)
	if len(trades) != 1 {
		t.Fatalf("len(trades) = %d, want 1: %+v", len(trades), trades)
	}
	return trades[0]
}

// 主板涨停(10%)开盘买不进:昨收 10.00,次日开盘 11.00 = 涨停价。
func TestPortfolioLimitUpSkip(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 10},
			{date: 20260106, yclose: 10, open: 11, close: 11},
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 100000, MaxPositions: 1})
	sim.onDayOpen(20260105, "2026-01-05", klines, nil)
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260105, "策略A")})

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "skipped" || trade.Reason != "涨停无法买入" {
		t.Fatalf("trade = %+v, want skipped(涨停无法买入)", trade)
	}
}

func TestPortfolioStrategyOpeningGapConstraint(t *testing.T) {
	maxGap := 6.0
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10.7, close: 10.7}}),
	}
	buy := pfBuy("000001", 90, 20260104, "低位三连阳首板")
	buy.maxOpenGap = &maxGap
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 100000, MaxPositions: 1})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{buy})

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "skipped" || trade.Reason != "开盘涨幅7.00%高于策略上限6.00%" {
		t.Fatalf("trade = %+v, want skipped by strategy opening-gap constraint", trade)
	}
}

// 可卖日全天封死跌停,止损卖单顺延到下一可交易日开盘。
func TestPortfolioLimitDownDelay(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 9.5},   // 买入日不触发止损
			{date: 20260106, yclose: 9.5, open: 8.5, close: 8.5}, // 可卖日跌停封死,触发后顺延
			{date: 20260107, yclose: 8.5, open: 9.0, close: 9.0}, // 开盘可卖,顺延成交
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1}) // 多留 50 覆盖买入费率,保证满 1000 股
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	if sim.positions["000001"].pendingExitSet {
		t.Fatal("entry-day low must not place a sell order")
	}
	sim.onDayClose(20260106, historicalDateText(20260106), klines)
	if !sim.positions["000001"].pendingExitSet {
		t.Fatal("limit-down stop should wait for the next tradable open")
	}
	sim.onDayOpen(20260107, "2026-01-07", klines, nil) // 顺延成交

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "closed" {
		t.Fatalf("trade status = %s, want closed", trade.Status)
	}
	if trade.EntryDate != "2026-01-05" || trade.ExitDate != "2026-01-07" {
		t.Fatalf("entry/exit = %s/%s, want 2026-01-05/2026-01-07", trade.EntryDate, trade.ExitDate)
	}
	if trade.Reason != "止损(盘中触及-3%)" {
		t.Fatalf("reason = %s, want 止损(盘中触及-3%%)", trade.Reason)
	}
	if math.Abs(trade.Pnl+1014) > 0.01 { // 1000股 10→9.0,扣双边费率
		t.Fatalf("pnl = %v, want -1014", trade.Pnl)
	}
}

// 100 股整手:目标金额 15700、开盘 10.50 → 1400 股(1495 向下取整)。
func TestPortfolioRoundLot(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{{date: 20260105, yclose: 10.5, open: 10.5, close: 10.5}}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 15700, MaxPositions: 1})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "open" {
		t.Fatalf("trade status = %s, want open", trade.Status)
	}
	if trade.Shares != 1400 {
		t.Fatalf("shares = %d, want 1400", trade.Shares)
	}
}

// 现金受约束降级买入:持仓浮盈后新信号的目标金额超过现金,
// 降级为按全部现金向下取整买入 400 股。
func TestPortfolioCashConstrainedBuy(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 19.9, open: 19.9, close: 21}, // 收盘小涨,浮盈拉高权益但未达止盈10%
			{date: 20260106, yclose: 21, open: 50, close: 50},
		}),
		"000002": pfKline([]pfBar{{date: 20260106, yclose: 50, open: 24, close: 24}}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 20000, MaxPositions: 2})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")}) // 500股*19.9=9950,现金剩10050
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, []portfolioPendingBuy{pfBuy("000002", 80, 20260105, "策略B")})

	trades := pfTrades(t, sim, "run1")
	if len(trades) != 2 {
		t.Fatalf("len(trades) = %d, want 2: %+v", len(trades), trades)
	}
	b := trades[0] // finalize 按 EntryDate 降序,次日买入的 000002 在前
	if b.Status != "open" || b.Shares != 400 {
		t.Fatalf("trade = %+v, want open with 400 shares(全现金降级买入)", b)
	}
	if math.Abs(sim.cash-440.225) > 0.01 {
		t.Fatalf("cash = %v, want 440.225(含买入费率)", sim.cash)
	}
}

// 目标金额买不起一手:target 低于 100 股单价 → 跳过。
func TestPortfolioInsufficientForOneLot(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10, close: 10}}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 900, MaxPositions: 2})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "skipped" || trade.Reason != "资金不足一手" {
		t.Fatalf("trade = %+v, want skipped(资金不足一手)", trade)
	}
}

// 买入日不触发止损;下一可卖日若低开跌破止损价,按开盘价成交。
func TestPortfolioTPlusOneAndStopPriority(t *testing.T) {
	bars := make([]pfBar, 0, 26)
	start := 20251201
	for i := 0; i < 24; i++ {
		bars = append(bars, pfBar{date: start + i, yclose: 20, open: 20, close: 20})
	}
	bars = append(bars,
		pfBar{date: 20251225, yclose: 20, open: 10, close: 9.0}, // 买入日,收盘跌破-8%且<MA20
		pfBar{date: 20251226, yclose: 9.0, open: 9.1, close: 9.1},
	)
	klines := map[string][]FormulaKline{"000001": pfKline(bars)}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1}) // 多留 50 覆盖买入费率
	sim.onDayOpen(20251225, "2025-12-25", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20251224, "策略A")})
	sim.onDayClose(20251225, historicalDateText(20251225), klines)
	if sim.positions["000001"].pendingExitSet {
		t.Fatal("entry-day stop must not carry over to the next open")
	}
	sim.onDayOpen(20251226, "2025-12-26", klines, nil)
	if len(sim.positions) != 1 {
		t.Fatal("next-day open must not execute an entry-day stop")
	}
	sim.onDayClose(20251226, "2025-12-26", klines)

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "closed" {
		t.Fatalf("trade status = %s, want closed", trade.Status)
	}
	if trade.EntryDate != "2025-12-25" || trade.ExitDate != "2025-12-26" {
		t.Fatalf("entry/exit = %s/%s, want 2025-12-25/2025-12-26(T+1)", trade.EntryDate, trade.ExitDate)
	}
	if trade.Reason != "止损(盘中触及-3%)" || trade.ExitPrice != 9.1 {
		t.Fatalf("trade = %+v, want next-day stop at opening price 9.1", trade)
	}
}

func TestPortfolioSameCloseStillObservesTPlusOne(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 10.3},
			{date: 20260106, yclose: 10.3, open: 10.2, close: 10.2},
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{
		InitialCash: 10050, MaxPositions: 1, SellBase: "same_close", TakeProfit: 2,
	})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
	sim.onDayClose(20260105, "2026-01-05", klines)
	if pos := sim.positions["000001"]; pos == nil || pos.pendingExitSet {
		t.Fatalf("entry-day high must not schedule a sale: %+v", pos)
	}
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "open" {
		t.Fatalf("entry-day high must not execute at next open: %+v", trade)
	}
}

func TestPortfolioIntradayRiskExits(t *testing.T) {
	cases := []struct {
		name          string
		first, second pfBar
		wantPrice     float64
		wantReason    string
		trail         float64
	}{
		{"take profit touches high", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.6, low: 9.9, close: 10}, 10.5, "止盈(盘中触及+5%)", 20},
		{"take profit equals high", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.5, low: 9.9, close: 10}, 10.5, "止盈(盘中触及+5%)", 20},
		{"stop loss touches low", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.2, low: 9.6, close: 10}, 9.7, "止损(盘中触及-3%)", 0},
		{"stop loss equals low", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.2, low: 9.7, close: 10}, 9.7, "止损(盘中触及-3%)", 0},
		{"take profit gaps up", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10.7, high: 10.8, low: 10.6, close: 10.7}, 10.7, "止盈(盘中触及+5%)", 20},
		{"stop loss gaps down", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 9.5, high: 9.6, low: 9.4, close: 9.5}, 9.5, "止损(盘中触及-3%)", 0},
		{"both limits touched", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.6, low: 9.6, close: 10}, 9.7, "止损(盘中触及-3%)", 0},
		{"open take profit precedes intraday stop", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10.6, high: 10.7, low: 9.6, close: 10}, 10.6, "止盈(盘中触及+5%)", 0},
		{"trail from prior peak", pfBar{date: 20260105, yclose: 10, open: 10, high: 10.4, low: 10, close: 10.3}, pfBar{date: 20260106, yclose: 10.3, open: 10.4, high: 10.5, low: 9.9, close: 10.3}, 9.975, "回撤(峰值回撤≥5%)", 0},
		{"trail gaps below prior threshold", pfBar{date: 20260105, yclose: 10, open: 10, high: 10.4, low: 10, close: 10.3}, pfBar{date: 20260106, yclose: 10.3, open: 9.75, high: 10, low: 9.75, close: 10}, 9.75, "回撤(峰值回撤≥5%)", 0},
		{"same day peak then trough", pfBar{date: 20260105, yclose: 10, open: 10, close: 10}, pfBar{date: 20260106, yclose: 10, open: 10, high: 10.4, low: 9.85, close: 10.2}, 9.88, "回撤(峰值回撤≥5%)", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			klines := map[string][]FormulaKline{"000001": pfKline([]pfBar{tc.first, tc.second})}
			for _, sellBase := range []string{"next_open", "same_close"} {
				trail := tc.trail
				if trail == 0 {
					trail = 5
				}
				sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1, SellBase: sellBase, TakeProfit: 5, StopLoss: 3, TrailDrawdown: trail})
				sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
				sim.onDayClose(20260105, "2026-01-05", klines)
				sim.onDayOpen(20260106, "2026-01-06", klines, nil)
				sim.onDayClose(20260106, "2026-01-06", klines)
				trade := pfSingleTrade(t, sim, "run")
				if trade.Status != "closed" || trade.ExitDate != "2026-01-06" || math.Abs(trade.ExitPrice-tc.wantPrice) > 1e-8 || trade.Reason != tc.wantReason {
					t.Fatalf("sell_base=%s: trade = %+v, want price %.3f reason %s", sellBase, trade, tc.wantPrice, tc.wantReason)
				}
			}
		})
	}
}

func TestPortfolioEntryDayExtremesDoNotScheduleExit(t *testing.T) {
	klines := map[string][]FormulaKline{"000001": pfKline([]pfBar{
		{date: 20260105, yclose: 10, open: 10, high: 10.6, low: 9.6, close: 10},
		{date: 20260106, yclose: 10, open: 10, high: 10.1, low: 9.9, close: 10},
	})}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1, TakeProfit: 5, TrailDrawdown: 5})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
	sim.onDayClose(20260105, "2026-01-05", klines)
	if pos := sim.positions["000001"]; pos == nil || pos.pendingExitSet || pos.peakHigh != 10 {
		t.Fatalf("entry-day high/low must not trigger exit or raise peak: %+v", pos)
	}
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	sim.onDayClose(20260106, "2026-01-06", klines)
	trade := pfSingleTrade(t, sim, "run")
	if trade.Status != "open" {
		t.Fatalf("position should stay open when next-day prices miss all limits: %+v", trade)
	}
}

func TestPortfolioLimitBuyDoesNotUseEarlierHighForTrailingStop(t *testing.T) {
	klines := map[string][]FormulaKline{"000001": pfKline([]pfBar{
		{date: 20260105, yclose: 10, open: 10.4, high: 11, low: 10, close: 10.1},
		{date: 20260106, yclose: 10.1, open: 10.1, high: 10.2, low: 9.75, close: 10},
	})}
	buy := pfBuy("000001", 90, 20260104, "策略A")
	buy.signalClose = 10
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1, BuyBase: "close", TrailDrawdown: 5})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{buy})
	sim.onDayClose(20260105, "2026-01-05", klines)
	if pos := sim.positions["000001"]; pos == nil || pos.entryPrice != 10 || pos.peakHigh != 10.1 {
		t.Fatalf("limit buy must use fill and close, not the full entry-day high: %+v", pos)
	}
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	sim.onDayClose(20260106, "2026-01-06", klines)
	if trade := pfSingleTrade(t, sim, "run"); trade.Status != "open" {
		t.Fatalf("prior entry-day high must not trigger trailing stop: %+v", trade)
	}
}

func TestPortfolioMaxHoldUsesSellBase(t *testing.T) {
	klines := map[string][]FormulaKline{"000001": pfKline([]pfBar{
		{date: 20260105, yclose: 10, open: 10, close: 10},
		{date: 20260106, yclose: 10, open: 10, close: 10.2},
		{date: 20260107, yclose: 10.2, open: 10.3, close: 10.3},
	})}
	for _, tc := range []struct {
		base, date string
		price      float64
	}{{"same_close", "2026-01-06", 10.2}, {"next_open", "2026-01-07", 10.3}} {
		sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10050, MaxPositions: 1, SellBase: tc.base, MaxHoldDays: 2})
		sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
		sim.onDayClose(20260105, "2026-01-05", klines)
		sim.onDayOpen(20260106, "2026-01-06", klines, nil)
		sim.onDayClose(20260106, "2026-01-06", klines)
		sim.onDayOpen(20260107, "2026-01-07", klines, nil)
		trade := pfSingleTrade(t, sim, "run")
		if trade.Status != "closed" || trade.ExitDate != tc.date || trade.ExitPrice != tc.price || trade.Reason != "持有超过2个交易日" {
			t.Fatalf("sell_base=%s: trade = %+v", tc.base, trade)
		}
	}
}

// 等权分配:同日两个信号、初始 100000、最大持仓 2 → 各 50000。
func TestPortfolioEqualWeight(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10, close: 10}}),
		"000002": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10, close: 10}}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 100000, MaxPositions: 2})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{
		pfBuy("000001", 90, 20260104, "策略A"),
		pfBuy("000002", 80, 20260104, "策略B"),
	})

	trades := pfTrades(t, sim, "run1")
	if len(trades) != 2 {
		t.Fatalf("len(trades) = %d, want 2: %+v", len(trades), trades)
	}
	for _, trade := range trades {
		if trade.Status != "open" {
			t.Fatalf("trade = %+v, want open", trade)
		}
	}
	// 000001 先买 5000 股(目标 50000);000002 因费率+等权整手截断降为 4900 股
	shares := map[string]int{}
	for _, trade := range trades {
		shares[trade.Symbol] = trade.Shares
	}
	if shares["000001"] != 5000 || shares["000002"] != 4900 {
		t.Fatalf("shares = %v, want 000001:5000/000002:4900(费率导致第二笔整手截断)", shares)
	}
}

// 已持仓股票再次出信号不重复买入。
func TestPortfolioDuplicateSignal(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 10},
			{date: 20260106, yclose: 10, open: 10, close: 10},
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10000, MaxPositions: 2})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, []portfolioPendingBuy{pfBuy("000001", 95, 20260105, "策略A")})

	trades := pfTrades(t, sim, "run1")
	if len(trades) != 2 {
		t.Fatalf("len(trades) = %d, want 2: %+v", len(trades), trades)
	}
	if trades[1].Status != "skipped" || trades[1].Reason != "已有持仓" {
		t.Fatalf("trade = %+v, want skipped(已有持仓)", trades[1])
	}
}

// 最大持仓数:同信号日按得分降序买入,超出上限的跳过。
func TestPortfolioMaxPositions(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10, close: 10}}),
		"000002": pfKline([]pfBar{{date: 20260105, yclose: 10, open: 10, close: 10}}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10000, MaxPositions: 1})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{
		pfBuy("000002", 80, 20260104, "策略B"),
		pfBuy("000001", 90, 20260104, "策略A"),
	})

	trades := pfTrades(t, sim, "run1")
	if len(trades) != 2 {
		t.Fatalf("len(trades) = %d, want 2: %+v", len(trades), trades)
	}
	if trades[0].Status != "open" || trades[0].Symbol != "000001" {
		t.Fatalf("trades[0] = %+v, want open 000001(得分高优先)", trades[0])
	}
	if trades[1].Status != "skipped" || trades[1].Reason != "持仓已满" {
		t.Fatalf("trades[1] = %+v, want skipped(持仓已满)", trades[1])
	}
}

// 涨跌停幅度按板块区分:创业板/科创板 20%,北交所 30%,主板 10%。
func TestPortfolioLimitRateByBoard(t *testing.T) {
	cases := []struct {
		symbol string
		want   float64
	}{
		{"300001", 0.20},
		{"688001", 0.20},
		{"830001", 0.30},
		{"430001", 0.30},
		{"920001", 0.30},
		{"600001", 0.10},
		{"000001", 0.10},
	}
	for _, c := range cases {
		if got := portfolioPriceLimitRate(c.symbol); got != c.want {
			t.Fatalf("rate(%s) = %v, want %v", c.symbol, got, c.want)
		}
	}
}

// 同一股票多条策略信号只买一次:保留最高得分,拼接触发策略名。
func TestMergePendingBuy(t *testing.T) {
	buys := mergePendingBuy(nil, "000001", 80, 20260105, "策略A", 0)
	buys = mergePendingBuy(buys, "000001", 90, 20260105, "策略B", 0)
	buys = mergePendingBuy(buys, "000001", 70, 20260105, "策略A", 0) // 重复策略不重复拼接
	if len(buys) != 1 {
		t.Fatalf("len(buys) = %d, want 1", len(buys))
	}
	if buys[0].score != 90 {
		t.Fatalf("score = %v, want 90(保留最高)", buys[0].score)
	}
	if buys[0].strategyName != "策略A·策略B" {
		t.Fatalf("strategyName = %q, want 策略A·策略B", buys[0].strategyName)
	}
	buys = mergePendingBuy(buys, "000002", 70, 20260105, "策略A", 0)
	if len(buys) != 2 {
		t.Fatalf("len(buys) = %d, want 2", len(buys))
	}
}

// 组合摘要统计:一盈一亏两笔平仓,校验期末权益、胜率与计数。
func TestPortfolioSummary(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 11.3}, // 买入日只更新收盘估值
			{date: 20260106, yclose: 11.3, open: 11.3, close: 11.3},
		}),
		"000002": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 9.0},   // 买入日不触发止损
			{date: 20260106, yclose: 9.0, open: 9.1, close: 9.1}, // 次日仍低于止损价
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 20000, MaxPositions: 2})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{
		pfBuy("000001", 90, 20260104, "策略A"),
		pfBuy("000002", 80, 20260104, "策略B"),
	}) // 各 10000 → 各 1000 股
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	sim.onDayClose(20260106, historicalDateText(20260106), klines)
	_ = pfTrades(t, sim, "run1")

	summary := sim.summary()
	if summary["closed_count"] != 2 {
		t.Fatalf("closed_count = %v, want 2", summary["closed_count"])
	}
	if summary["win_count"] != 1 {
		t.Fatalf("win_count = %v, want 1", summary["win_count"])
	}
	if summary["win_rate"] != 50.0 {
		t.Fatalf("win_rate = %v, want 50", summary["win_rate"])
	}
	// 盈亏(含双边费率):A +1283.70(1000股 10→11.3 止盈) / B -822.69(900股 10→9.1 止损)
	// → 期末权益 ≈ 20461.01,总收益 ≈ +2.31%
	if math.Abs(summary["final_equity"].(float64)-20461.01) > 0.5 {
		t.Fatalf("final_equity = %v, want ≈20461.01", summary["final_equity"])
	}
	if got := summary["total_return"].(float64); math.Abs(got-2.31) > 0.1 {
		t.Fatalf("total_return = %v, want ≈+2.31", got)
	}
}

// 买入费率生效:全仓买入时 1000 股被费率截断为 900 股,盈亏扣双边费率。
func TestPortfolioCosts(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": pfKline([]pfBar{
			{date: 20260105, yclose: 10, open: 10, close: 9.0}, // 买入日收盘触发止损
			{date: 20260106, yclose: 9.0, open: 9.5, close: 9.5},
		}),
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 10000, MaxPositions: 1})
	sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{pfBuy("000001", 90, 20260104, "策略A")})
	sim.onDayClose(20260105, historicalDateText(20260105), klines)
	sim.onDayOpen(20260106, "2026-01-06", klines, nil)
	sim.onDayClose(20260106, historicalDateText(20260106), klines)

	trade := pfSingleTrade(t, sim, "run1")
	if trade.Status != "closed" {
		t.Fatalf("trade status = %s, want closed", trade.Status)
	}
	// 无费率时全仓 1000 股;含 0.05% 买入费率后 10005 > 10000,降级为 900 股
	if trade.Shares != 900 {
		t.Fatalf("shares = %d, want 900(买入费率截断)", trade.Shares)
	}
	// 无费率盈亏 = 900*(9.5-10) = -450;扣双边费率后 = -463.05
	if math.Abs(trade.Pnl+463.05) > 0.01 {
		t.Fatalf("pnl = %v, want -463.05(扣双边费率)", trade.Pnl)
	}
	if math.Abs(sim.cash-9536.95) > 0.01 {
		t.Fatalf("cash = %v, want 9536.95", sim.cash)
	}
}

func TestLimitUpLeaderKeepsHighestStreakAndLowestVolume(t *testing.T) {
	bar := func(date int, yclose, close, vol float64) FormulaKline {
		return FormulaKline{Date: date, YClose: yclose, Open: yclose, High: close, Low: yclose, Close: close, Vol: vol}
	}
	klines := map[string][]FormulaKline{
		"000001": {bar(20260101, 10, 10, 100), bar(20260102, 10, 11, 100), bar(20260103, 11, 12.1, 100), bar(20260104, 12.1, 13.31, 100), bar(20260105, 13.31, 14.641, 100)},
		"000002": {bar(20260101, 10, 10, 50), bar(20260102, 10, 11, 50), bar(20260103, 11, 12.1, 50), bar(20260104, 12.1, 13.31, 50), bar(20260105, 13.31, 14.641, 50)},
		"000003": {bar(20260101, 10, 10, 10), bar(20260102, 10, 10, 10), bar(20260103, 10, 10, 10), bar(20260104, 10, 11, 10), bar(20260105, 11, 12.1, 10)},
	}
	for symbol, rows := range klines {
		if count := limitUpStreak(symbol, rows, 20, 0.25); count != map[string]int{"000001": 4, "000002": 4, "000003": 2}[symbol] {
			t.Fatalf("%s streak = %d", symbol, count)
		}
	}
	result := &StrategyRunResult{Config: StrategyConfig{Pass: StrategyPassConfig{TopN: 10}}, KlineCache: klines}
	matches := []StrategySelectionItem{{Symbol: "000001"}, {Symbol: "000002"}, {Symbol: "000003"}}
	got := finalizeStrategyMatches(Strategy{ID: "template-jq-limit-up-leader"}, result, matches)
	if len(got) != 2 || got[0].Symbol != "000002" || got[1].Symbol != "000001" || !strings.Contains(got[0].RankReason, "最高连板4板") {
		t.Fatalf("leader order = %+v", got)
	}
}

func TestFirstBoardPositionUsesSignalDayOnly(t *testing.T) {
	rows := make([]FormulaKline, 61)
	for i := range rows {
		rows[i] = FormulaKline{Date: 20250101 + i, YClose: 10, Open: 10, High: 15, Low: 8, Close: 10}
	}
	rows[0].Date = 20240101
	rows[60].Date = 20251001
	rows[60].Close = 11
	rule := StrategyFactorRule{Params: map[string]interface{}{"position_lookback": 60, "max_position": 50, "min_listing_days": 250}}
	if hit, _, _, reason := evaluateFirstBoardPosition("000001", rows, rule); !hit {
		t.Fatalf("first board rejected: %s", reason)
	}
	rows[59].Close = 11
	if hit, _, _, _ := evaluateFirstBoardPosition("000001", rows, rule); hit {
		t.Fatal("consecutive board must not count as first board")
	}
}

func TestJQDailyExitRulesUseCloseAndObserveTPlusOne(t *testing.T) {
	for _, tt := range []struct {
		name, policy string
		bars         []FormulaKline
		exitPrice    float64
	}{
		{name: "first board", policy: "first_board_close", exitPrice: 9.8, bars: []FormulaKline{
			{Date: 20260105, YClose: 10, Open: 9.65, High: 10.2, Low: 9.5, Close: 9.5},
			{Date: 20260106, YClose: 9.5, Open: 9.6, High: 10, Low: 9.5, Close: 9.8},
		}},
		{name: "leader", policy: "leader_close", exitPrice: 10.8, bars: []FormulaKline{
			{Date: 20260105, YClose: 10, Open: 10, High: 11, Low: 10, Close: 11},
			{Date: 20260106, YClose: 11, Open: 10.5, High: 11, Low: 10.4, Close: 10.8},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			klines := map[string][]FormulaKline{"000001": tt.bars}
			sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 100000, MaxPositions: 1, BuyBase: "close", BuyOffset: -2, SellBase: "next_open", StopLoss: 1})
			buy := pfBuy("000001", 90, 20260104, tt.name)
			buy.buyAtOpen = true
			buy.exitPolicy = tt.policy
			sim.onDayOpen(20260105, "2026-01-05", klines, []portfolioPendingBuy{buy})
			if pos := sim.positions["000001"]; pos == nil || pos.entryPrice != tt.bars[0].Open {
				t.Fatalf("entry must use opening price: %+v", pos)
			}
			sim.onDayClose(20260105, "2026-01-05", klines)
			if len(sim.positions) != 1 {
				t.Fatal("entry-day close must not sell T+1 shares")
			}
			sim.onDayOpen(20260106, "2026-01-06", klines, nil)
			sim.onDayClose(20260106, "2026-01-06", klines)
			trade := pfSingleTrade(t, sim, "run")
			if trade.Status != "closed" || trade.ExitPrice != tt.exitPrice || trade.ExitDate != "2026-01-06" {
				t.Fatalf("trade = %+v", trade)
			}
		})
	}
}

func TestFirstBoardOpeningGapAndFlatConstraint(t *testing.T) {
	klines := map[string][]FormulaKline{
		"000001": {{Date: 20260105, YClose: 10, Open: 9.65, High: 10, Low: 9.6, Close: 9.8}},
		"000002": {{Date: 20260105, YClose: 10, Open: 9.8, High: 10, Low: 9.7, Close: 9.9}},
	}
	minGap, maxGap := -4.0, -3.0
	buys := []portfolioPendingBuy{
		{symbol: "000001", score: 100, signalDate: 20260104, minOpenGap: &minGap, maxOpenGap: &maxGap, buyAtOpen: true, onlyWhenFlat: true, exitPolicy: "first_board_close"},
		{symbol: "000002", score: 90, signalDate: 20260104, minOpenGap: &minGap, maxOpenGap: &maxGap, buyAtOpen: true, onlyWhenFlat: true, exitPolicy: "first_board_close"},
	}
	sim := newPortfolioSimulator(portfolioSimConfig{InitialCash: 100000, MaxPositions: 2, BuyBase: "close", BuyOffset: -2})
	sim.onDayOpen(20260105, "2026-01-05", klines, buys)
	if pos := sim.positions["000001"]; pos == nil || pos.entryPrice != 9.65 {
		t.Fatalf("first board must buy at open: %+v", pos)
	}
	if len(sim.trades) != 1 || !strings.Contains(sim.trades[0].Reason, "开盘涨幅") {
		t.Fatalf("out-of-range open should be skipped: %+v", sim.trades)
	}
}

func TestMergedDailyStrategyKeepsItsOpeningGap(t *testing.T) {
	minGap, maxGap := -4.0, -3.0
	daily := StrategyExecutionConfig{
		MinOpenGap: &minGap, MaxOpenGap: &maxGap, BuyAtOpen: true,
		ExitPolicy: "first_board_close", OnlyWhenFlat: true,
	}
	buys := mergePendingBuy(nil, "000001", 90, 20260105, "首板低开", 10, daily)
	buys = mergePendingBuy(buys, "000001", 80, 20260105, "普通策略", 10)
	if len(buys) != 1 || !buys[0].buyAtOpen || buys[0].exitPolicy != daily.ExitPolicy ||
		buys[0].minOpenGap == nil || *buys[0].minOpenGap != minGap ||
		buys[0].maxOpenGap == nil || *buys[0].maxOpenGap != maxGap {
		t.Fatalf("daily execution constraints changed: %+v", buys)
	}
	buys = mergePendingBuy(buys, "000001", 100, 20260105, "更高分普通策略", 10)
	if buys[0].buyAtOpen || buys[0].exitPolicy != "" || buys[0].minOpenGap != nil || buys[0].maxOpenGap != nil {
		t.Fatalf("winning generic execution was not selected: %+v", buys[0])
	}
}
