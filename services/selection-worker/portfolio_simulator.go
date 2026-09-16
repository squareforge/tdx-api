package main

import (
	"fmt"
	"sort"
	"strings"
)

// 组合模拟撮合引擎:在历史回测逐日信号的基础上,模拟单账户资金池的买卖流水。
// 信号于 T 日收盘产生,撮合在 T+1 日开盘执行;退出条件由 T 日收盘判定,次日开盘执行。
// 约束:涨停买不进、跌停卖不出、100股整手、T+1、最大持仓数、等权分配。

type portfolioSimConfig struct {
	InitialCash   float64 `json:"initial_cash"`
	MaxPositions  int     `json:"max_positions"`
	BuyCost       float64 `json:"buy_cost"`  // 买入费率(佣金+过户),与策略回测引擎一致
	SellCost      float64 `json:"sell_cost"` // 卖出费率(佣金+过户+印花税),与策略回测引擎一致
	BuyBase       string  `json:"buy_base"`  // 买入基准价: open(次日开盘) | close(信号日收盘)
	BuyOffset     float64 `json:"buy_offset"`
	SellBase      string  `json:"sell_base"` // 卖出价: next_open(次日开盘) | same_close(当日收盘)
	TakeProfit    float64 `json:"take_profit"`
	TrailDrawdown float64 `json:"trail_drawdown"`
	StopLoss      float64 `json:"stop_loss"`
	MaxHoldDays   int     `json:"max_hold_days"`
}

type portfolioSimPosition struct {
	symbol         string
	shares         int
	entryPrice     float64
	entryDate      int
	signalDate     int
	strategyName   string
	lastClose      float64 // 最近可用收盘价(权益估值用)
	peakClose      float64 // 持仓期间最高收盘价(回撤止损基准)
	holdDays       int     // 持有交易日数
	pendingExit    string  // 昨日收盘判定、次日开盘执行的退出原因
	pendingExitSet bool
}

type portfolioPendingBuy struct {
	symbol       string
	score        float64
	signalDate   int
	strategyName string
	signalClose  float64 // 信号日收盘价,买入基准"close"时作为限价参考
}

type portfolioSimulator struct {
	cfg         portfolioSimConfig
	cash        float64
	positions   map[string]*portfolioSimPosition
	trades      []HistoricalBacktestTrade
	equityPeak  float64
	maxDrawdown float64
}

func newPortfolioSimulator(cfg portfolioSimConfig) *portfolioSimulator {
	// 费率默认值与策略回测引擎一致:无论请求层是否填充,引擎内兜底保证费用开启
	if cfg.BuyCost <= 0 {
		cfg.BuyCost = 0.0005 // 佣金+过户
	}
	if cfg.SellCost <= 0 {
		cfg.SellCost = 0.001 // 佣金+过户+印花税
	}
	if cfg.BuyBase != "close" {
		cfg.BuyBase = "open"
	}
	if cfg.SellBase != "same_close" {
		cfg.SellBase = "next_open"
	}
	if cfg.TakeProfit <= 0 {
		cfg.TakeProfit = 10
	}
	if cfg.TrailDrawdown <= 0 {
		cfg.TrailDrawdown = 5
	}
	if cfg.StopLoss <= 0 {
		cfg.StopLoss = 3
	}
	if cfg.MaxHoldDays <= 0 {
		cfg.MaxHoldDays = 15
	}
	return &portfolioSimulator{
		cfg:         cfg,
		cash:        cfg.InitialCash,
		positions:   map[string]*portfolioSimPosition{},
		trades:      []HistoricalBacktestTrade{},
		equityPeak:  cfg.InitialCash,
		maxDrawdown: 0,
	}
}

func (sim *portfolioSimulator) equity() float64 {
	total := sim.cash
	for _, pos := range sim.positions {
		total += float64(pos.shares) * pos.lastClose
	}
	return total
}

func portfolioPriceLimitRate(symbol string) float64 {
	switch {
	case strings.HasPrefix(symbol, "30"), strings.HasPrefix(symbol, "68"):
		return 0.20
	case strings.HasPrefix(symbol, "8"), strings.HasPrefix(symbol, "4"), strings.HasPrefix(symbol, "92"):
		return 0.30
	default:
		return 0.10
	}
}

func portfolioKlineOnDate(rows []FormulaKline, date int) (FormulaKline, bool) {
	index := sort.Search(len(rows), func(i int) bool { return rows[i].Date >= date })
	if index >= len(rows) || rows[index].Date != date {
		return FormulaKline{}, false
	}
	return rows[index], true
}

// onDayOpen 处理当日开盘:先执行昨日收盘触发的卖出,再执行昨日信号的买入。
func (sim *portfolioSimulator) onDayOpen(date int, dateText string, klines map[string][]FormulaKline, buys []portfolioPendingBuy) {
	// 1) 卖出(先卖释放现金)
	for _, pos := range sim.positions {
		if !pos.pendingExitSet {
			continue
		}
		row, ok := portfolioKlineOnDate(klines[pos.symbol], date)
		if !ok {
			continue // 停牌缺K线,顺延到下一交易日
		}
		if row.Open <= row.YClose*(1-portfolioPriceLimitRate(pos.symbol)) {
			continue // 跌停开盘卖不出,顺延
		}
		sim.executeSell(pos, row.Open, date, dateText)
	}

	// 2) 买入(按得分降序,先买高分)
	sorted := append([]portfolioPendingBuy{}, buys...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].score == sorted[j].score {
			return sorted[i].symbol < sorted[j].symbol
		}
		return sorted[i].score > sorted[j].score
	})
	for _, buy := range sorted {
		sim.tryBuy(buy, date, dateText, klines)
	}
}

func (sim *portfolioSimulator) tryBuy(buy portfolioPendingBuy, date int, dateText string, klines map[string][]FormulaKline) {
	if _, exists := sim.positions[buy.symbol]; exists {
		sim.appendSkip(buy, dateText, "已有持仓")
		return
	}
	if len(sim.positions) >= sim.cfg.MaxPositions {
		sim.appendSkip(buy, dateText, "持仓已满")
		return
	}
	row, ok := portfolioKlineOnDate(klines[buy.symbol], date)
	if !ok {
		sim.appendSkip(buy, dateText, "停牌或缺少K线")
		return
	}
	if row.Open <= 0 {
		sim.appendSkip(buy, dateText, "开盘价无效")
		return
	}
	if row.Open >= row.YClose*(1+portfolioPriceLimitRate(buy.symbol)) {
		sim.appendSkip(buy, dateText, "涨停无法买入")
		return
	}
	// 买入限价 = 基准价 × (1+offset)。基准"open"用当日开盘价,"close"用信号日收盘价。
	basePrice := row.Open
	if sim.cfg.BuyBase == "close" && buy.signalClose > 0 {
		basePrice = buy.signalClose
	}
	limitPrice := basePrice * (1 + sim.cfg.BuyOffset/100)
	if limitPrice <= 0 {
		sim.appendSkip(buy, dateText, "买入限价无效")
		return
	}
	// 限价买单成交条件:当日最低价须不高于限价;开盘即低于限价则按开盘价成交。
	if row.Low > limitPrice {
		sim.appendSkip(buy, dateText, "未跌至买入限价")
		return
	}
	fillPrice := limitPrice
	if row.Open <= limitPrice {
		fillPrice = row.Open
	}
	target := sim.equity() / float64(sim.cfg.MaxPositions)
	shares := int(target/fillPrice/100) * 100
	if shares < 100 {
		sim.appendSkip(buy, dateText, "资金不足一手")
		return
	}
	// 买入总支出含费率:现金约束与降级重算都以含费单价为准
	cost := float64(shares) * fillPrice * (1 + sim.cfg.BuyCost)
	if cost > sim.cash {
		shares = int(sim.cash/(fillPrice*(1+sim.cfg.BuyCost))/100) * 100
		if shares < 100 {
			sim.appendSkip(buy, dateText, "现金不足")
			return
		}
		cost = float64(shares) * fillPrice * (1 + sim.cfg.BuyCost)
	}
	sim.cash -= cost
	sim.positions[buy.symbol] = &portfolioSimPosition{
		symbol:       buy.symbol,
		shares:       shares,
		entryPrice:   fillPrice,
		entryDate:    date,
		signalDate:   buy.signalDate,
		strategyName: buy.strategyName,
		lastClose:    fillPrice,
		peakClose:    fillPrice,
	}
}

func (sim *portfolioSimulator) executeSell(pos *portfolioSimPosition, price float64, date int, dateText string) {
	// 卖出收入扣费率,盈亏按含费双边成本计算(与策略回测引擎 tradeReturn 语义一致)
	proceeds := float64(pos.shares) * price * (1 - sim.cfg.SellCost)
	entryCost := float64(pos.shares) * pos.entryPrice * (1 + sim.cfg.BuyCost)
	sim.cash += proceeds
	pnl := proceeds - entryCost
	pnlRate := 0.0
	if entryCost > 0 {
		pnlRate = (proceeds/entryCost - 1) * 100
	}
	sim.trades = append(sim.trades, HistoricalBacktestTrade{
		Symbol:       pos.symbol,
		Status:       "closed",
		EntryDate:    historicalDateText(pos.entryDate),
		EntryPrice:   pos.entryPrice,
		Shares:       pos.shares,
		ExitDate:     dateText,
		ExitPrice:    price,
		Pnl:          pnl,
		PnlRate:      pnlRate,
		HoldDays:     pos.holdDays,
		StrategyName: pos.strategyName,
		SignalDate:   historicalDateText(pos.signalDate),
		Reason:       pos.pendingExit,
	})
	delete(sim.positions, pos.symbol)
}

func (sim *portfolioSimulator) appendSkip(buy portfolioPendingBuy, attemptDate, reason string) {
	sim.trades = append(sim.trades, HistoricalBacktestTrade{
		Symbol:       buy.symbol,
		Status:       "skipped",
		EntryDate:    attemptDate,
		StrategyName: buy.strategyName,
		SignalDate:   historicalDateText(buy.signalDate),
		Reason:       reason,
	})
}

// onDayClose 收盘结算:更新持仓估值与峰值,判定退出条件,记录最大回撤。
// 退出判定按"止损 > 回撤 > 止盈 > 超时"优先级;卖出价由 SellBase 决定
// (same_close 当日收盘直接成交,next_open 次日开盘执行)。
func (sim *portfolioSimulator) onDayClose(date int, dateText string, klines map[string][]FormulaKline) {
	for _, pos := range sim.positions {
		row, ok := portfolioKlineOnDate(klines[pos.symbol], date)
		if !ok {
			continue // 停牌,沿用上次收盘
		}
		pos.lastClose = row.Close
		pos.holdDays++
		if row.Close > pos.peakClose {
			pos.peakClose = row.Close
		}
		var reason string
		switch {
		case row.Close <= pos.entryPrice*(1-sim.cfg.StopLoss/100):
			reason = fmt.Sprintf("止损(收盘≤-%g%%)", sim.cfg.StopLoss)
		case row.Close <= pos.peakClose*(1-sim.cfg.TrailDrawdown/100):
			reason = fmt.Sprintf("回撤(峰值回撤≥%g%%)", sim.cfg.TrailDrawdown)
		case row.Close >= pos.entryPrice*(1+sim.cfg.TakeProfit/100):
			reason = fmt.Sprintf("止盈(收盘收益≥%g%%)", sim.cfg.TakeProfit)
		case pos.holdDays >= sim.cfg.MaxHoldDays:
			reason = fmt.Sprintf("持有超过%d个交易日", sim.cfg.MaxHoldDays)
		}
		if reason == "" {
			continue
		}
		if sim.cfg.SellBase == "same_close" {
			pos.pendingExit = reason
			sim.executeSell(pos, row.Close, date, dateText)
		} else {
			pos.pendingExit, pos.pendingExitSet = reason, true
		}
	}
	equity := sim.equity()
	if equity > sim.equityPeak {
		sim.equityPeak = equity
	}
	drawdown := 0.0
	if sim.equityPeak > 0 {
		drawdown = (sim.equityPeak - equity) / sim.equityPeak * 100
	}
	if drawdown > sim.maxDrawdown {
		sim.maxDrawdown = drawdown
	}
}

// finalize 回测结束时,把未平仓持仓转为 open 记录并排序返回全部交易。
func (sim *portfolioSimulator) finalize(runID string) []HistoricalBacktestTrade {
	for _, pos := range sim.positions {
		sim.trades = append(sim.trades, HistoricalBacktestTrade{
			Symbol:       pos.symbol,
			Status:       "open",
			EntryDate:    historicalDateText(pos.entryDate),
			EntryPrice:   pos.entryPrice,
			Shares:       pos.shares,
			HoldDays:     pos.holdDays,
			StrategyName: pos.strategyName,
			SignalDate:   historicalDateText(pos.signalDate),
		})
	}
	sort.SliceStable(sim.trades, func(i, j int) bool {
		left, right := sim.trades[i], sim.trades[j]
		if left.Status == "skipped" || right.Status == "skipped" {
			if left.Status == "skipped" && right.Status != "skipped" {
				return false
			}
			if right.Status == "skipped" && left.Status != "skipped" {
				return true
			}
			return left.SignalDate > right.SignalDate
		}
		return left.EntryDate > right.EntryDate
	})
	for i := range sim.trades {
		sim.trades[i].RunID = runID
	}
	return sim.trades
}

func (sim *portfolioSimulator) summary() map[string]interface{} {
	closedCount, openCount, skippedCount, winCount := 0, 0, 0, 0
	for _, trade := range sim.trades {
		switch trade.Status {
		case "closed":
			closedCount++
			if trade.Pnl > 0 {
				winCount++
			}
		case "open":
			openCount++
		case "skipped":
			skippedCount++
		}
	}
	winRate := 0.0
	if closedCount > 0 {
		winRate = float64(winCount) / float64(closedCount) * 100
	}
	finalEquity := sim.equity()
	totalReturn := 0.0
	if sim.cfg.InitialCash > 0 {
		totalReturn = (finalEquity - sim.cfg.InitialCash) / sim.cfg.InitialCash * 100
	}
	return map[string]interface{}{
		"initial_cash":  sim.cfg.InitialCash,
		"max_positions": sim.cfg.MaxPositions,
		"final_equity":  round2(finalEquity),
		"total_return":  round2(totalReturn),
		"max_drawdown":  round2(sim.maxDrawdown),
		"closed_count":  closedCount,
		"open_count":    openCount,
		"skipped_count": skippedCount,
		"win_count":     winCount,
		"win_rate":      round2(winRate),
	}
}

func round2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

// mergePendingBuy 把当日新信号合并进待买列表：同一股票多条策略信号只买一次，
// 保留最高得分并拼接触发策略名。
func mergePendingBuy(buys []portfolioPendingBuy, symbol string, score float64, signalDate int, strategyName string, signalClose float64) []portfolioPendingBuy {
	for i := range buys {
		if buys[i].symbol != symbol {
			continue
		}
		if score > buys[i].score {
			buys[i].score = score
		}
		if !strings.Contains(buys[i].strategyName, strategyName) {
			if buys[i].strategyName != "" {
				buys[i].strategyName += "·"
			}
			buys[i].strategyName += strategyName
		}
		return buys
	}
	return append(buys, portfolioPendingBuy{symbol: symbol, score: score, signalDate: signalDate, strategyName: strategyName, signalClose: signalClose})
}
