package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	workbench "workbench-core"
)

const historicalBacktestRunLock = "historical-backtest"

type historicalBacktestRequest struct {
	StartDate     string   `json:"start_date"`
	EndDate       string   `json:"end_date"`
	StrategyIDs   []string `json:"strategy_ids"`
	Horizons      []int    `json:"horizons"`
	TargetReturn  float64  `json:"target_return"`
	DrawdownLimit float64  `json:"drawdown_limit"`
	InitialCash   float64  `json:"initial_cash"`
	MaxPositions  int      `json:"max_positions"`
	BuyCost       float64  `json:"buy_cost"`
	SellCost      float64  `json:"sell_cost"`
	BuyBase       string   `json:"buy_base"`
	BuyOffset     float64  `json:"buy_offset"`
	SellBase      string   `json:"sell_base"`
	TakeProfit    float64  `json:"take_profit"`
	TrailDrawdown float64  `json:"trail_drawdown"`
	StopLoss      float64  `json:"stop_loss"`
	MaxHoldDays   int      `json:"max_hold_days"`
}

type historicalStrategyPlan struct {
	strategy Strategy
	config   StrategyConfig
	symbols  []string
}

type historicalHorizonAccumulator struct {
	Completed       int     `json:"completed"`
	Pending         int     `json:"pending"`
	SuccessCount    int     `json:"success_count"`
	SuccessRate     float64 `json:"success_rate"`
	AverageReturn   float64 `json:"average_return"`
	AverageMaxGain  float64 `json:"average_max_gain"`
	AverageDrawdown float64 `json:"average_max_drawdown"`
	returnTotal     float64
	gainTotal       float64
	drawdownTotal   float64
}

type historicalStrategySummary struct {
	StrategyID   string                                   `json:"strategy_id"`
	StrategyName string                                   `json:"strategy_name"`
	SignalCount  int                                      `json:"signal_count"`
	Horizons     map[string]*historicalHorizonAccumulator `json:"horizons"`
}

type historicalConsensusSummary struct {
	SignalDate    string   `json:"signal_date"`
	Symbol        string   `json:"symbol"`
	StrategyCount int      `json:"strategy_count"`
	Strategies    []string `json:"strategies"`
	TrackingJSON  string   `json:"tracking_json"`
}

type historicalConsensusAccumulator struct {
	date       string
	symbol     string
	strategies map[string]bool
	tracking   string
}

func handleHistoricalBacktests(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/historical-backtests" {
		handleHistoricalBacktestOperations(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := appStore.ListHistoricalBacktestRuns(limit)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, map[string]interface{}{"items": items})
	case http.MethodPost:
		var req historicalBacktestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		run, err := automationRunner.startHistoricalBacktest(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, run)
	default:
		errorResponse(w, "只支持GET或POST请求")
	}
}

func handleHistoricalBacktestOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/historical-backtests/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "signals" {
		if r.Method != http.MethodGet {
			errorResponse(w, "只支持GET请求")
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page, err := appStore.ListHistoricalBacktestSignals(id, HistoricalBacktestSignalQuery{
			StrategyID: r.URL.Query().Get("strategy_id"), SignalDate: r.URL.Query().Get("signal_date"),
			Symbol: r.URL.Query().Get("symbol"), Limit: limit, Offset: offset,
		})
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, page)
		return
	}
	if len(parts) == 2 && parts[1] == "trades" {
		if r.Method != http.MethodGet {
			errorResponse(w, "只支持GET请求")
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page, err := appStore.ListHistoricalBacktestTrades(id, HistoricalBacktestTradeQuery{
			Status: r.URL.Query().Get("status"), Symbol: r.URL.Query().Get("symbol"), Limit: limit, Offset: offset,
		})
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, page)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		if err := appStore.RequestHistoricalBacktestCancel(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		item, _ := appStore.GetHistoricalBacktestRun(id)
		successResponse(w, item)
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet {
		errorResponse(w, "不支持的回测操作")
		return
	}
	item, err := appStore.GetHistoricalBacktestRun(id)
	if err != nil {
		errorResponse(w, notFoundMessage(err, "回测任务不存在"))
		return
	}
	successResponse(w, item)
}

func normalizeHistoricalBacktestRequest(req historicalBacktestRequest) (historicalBacktestRequest, time.Time, time.Time, error) {
	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(req.StartDate), time.Local)
	if err != nil {
		return req, time.Time{}, time.Time{}, errors.New("开始日期格式应为 YYYY-MM-DD")
	}
	if strings.TrimSpace(req.EndDate) == "" {
		req.EndDate = time.Now().In(time.Local).Format("2006-01-02")
	}
	end, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(req.EndDate), time.Local)
	if err != nil {
		return req, time.Time{}, time.Time{}, errors.New("结束日期格式应为 YYYY-MM-DD")
	}
	if end.Before(start) {
		return req, time.Time{}, time.Time{}, errors.New("结束日期不能早于开始日期")
	}
	if end.Sub(start) > 5*366*24*time.Hour {
		return req, time.Time{}, time.Time{}, errors.New("单次历史回测区间不能超过5年")
	}
	req.StartDate = start.Format("2006-01-02")
	req.EndDate = end.Format("2006-01-02")
	seen := map[int]bool{}
	horizons := make([]int, 0, len(req.Horizons))
	for _, horizon := range req.Horizons {
		if horizon > 0 && horizon <= 30 && !seen[horizon] {
			seen[horizon] = true
			horizons = append(horizons, horizon)
		}
	}
	if len(horizons) == 0 {
		horizons = []int{3, 5, 10}
	}
	sort.Ints(horizons)
	req.Horizons = horizons
	if req.InitialCash <= 0 || req.InitialCash > 1e9 {
		req.InitialCash = 100000
	}
	if req.MaxPositions <= 0 || req.MaxPositions > 20 {
		req.MaxPositions = 5
	}
	if req.BuyCost <= 0 {
		req.BuyCost = 0.0005 // 与策略回测引擎默认一致(佣金+过户)
	}
	if req.SellCost <= 0 {
		req.SellCost = 0.001 // 与策略回测引擎默认一致(佣金+过户+印花税)
	}
	if req.BuyBase != "close" {
		req.BuyBase = "open"
	}
	if req.SellBase != "same_close" {
		req.SellBase = "next_open"
	}
	if req.TakeProfit <= 0 || req.TakeProfit > 100 {
		req.TakeProfit = 10
	}
	if req.TrailDrawdown <= 0 || req.TrailDrawdown > 100 {
		req.TrailDrawdown = 5
	}
	if req.StopLoss <= 0 || req.StopLoss > 100 {
		req.StopLoss = 3
	}
	if req.MaxHoldDays <= 0 || req.MaxHoldDays > 250 {
		req.MaxHoldDays = 15
	}
	// 信号达标统计复用组合退出的止盈/回撤参数,避免两套口径不一致。
	req.TargetReturn = req.TakeProfit
	req.DrawdownLimit = req.TrailDrawdown
	req.StrategyIDs = normalizeIDList(req.StrategyIDs)
	return req, start, end, nil
}

func normalizeIDList(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (r *AutomationRunner) historicalBacktestStrategies(ids []string) ([]Strategy, error) {
	all, err := r.store.ListStrategies()
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, id := range ids {
		requested[id] = true
	}
	strategies := make([]Strategy, 0, len(all))
	for _, strategy := range all {
		if len(requested) > 0 {
			if requested[strategy.ID] {
				strategies = append(strategies, strategy)
			}
			continue
		}
		if strategy.Readonly && strategy.Enabled {
			strategies = append(strategies, strategy)
		}
	}
	if len(strategies) == 0 {
		return nil, errors.New("没有可回测的策略")
	}
	if len(requested) > 0 && len(strategies) != len(requested) {
		return nil, errors.New("部分回测策略不存在")
	}
	return strategies, nil
}

func (r *AutomationRunner) startHistoricalBacktest(raw historicalBacktestRequest) (HistoricalBacktestRun, error) {
	req, _, _, err := normalizeHistoricalBacktestRequest(raw)
	if err != nil {
		return HistoricalBacktestRun{}, err
	}
	strategies, err := r.historicalBacktestStrategies(req.StrategyIDs)
	if err != nil {
		return HistoricalBacktestRun{}, err
	}
	if !r.acquireRun(historicalBacktestRunLock) {
		return HistoricalBacktestRun{}, errors.New("已有历史回测正在执行")
	}
	strategyIDs := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		strategyIDs = append(strategyIDs, strategy.ID)
	}
	run, err := r.store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		Status: "running", StartDate: req.StartDate, EndDate: req.EndDate, StrategyIDs: strategyIDs,
		StrategySnapshotJSON: mustJSON(strategies), Horizons: req.Horizons,
		TargetReturn: req.TargetReturn, DrawdownLimit: req.DrawdownLimit,
		BuyBase: req.BuyBase, BuyOffset: req.BuyOffset, SellBase: req.SellBase,
		TakeProfit: req.TakeProfit, TrailDrawdown: req.TrailDrawdown, StopLoss: req.StopLoss, MaxHoldDays: req.MaxHoldDays,
	})
	if err != nil {
		r.releaseRun(historicalBacktestRunLock)
		return run, err
	}
	go func() {
		defer r.releaseRun(historicalBacktestRunLock)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()
		result, runErr := r.runHistoricalBacktest(ctx, run, req, strategies)
		status, errorText := "success", ""
		if runErr != nil {
			status, errorText = "failed", runErr.Error()
			if errors.Is(runErr, context.Canceled) || r.store.HistoricalBacktestCancelRequested(run.ID) {
				status, errorText = "cancelled", "任务已取消"
			}
		}
		_ = r.store.FinishHistoricalBacktestRun(run.ID, status, mustJSON(result), errorText)
	}()
	return run, nil
}

func (r *AutomationRunner) runHistoricalBacktest(ctx context.Context, run HistoricalBacktestRun, req historicalBacktestRequest, strategies []Strategy) (map[string]interface{}, error) {
	_, start, end, err := normalizeHistoricalBacktestRequest(req)
	if err != nil {
		return nil, err
	}
	plans := make([]historicalStrategyPlan, 0, len(strategies))
	allSymbols := map[string]bool{}
	for _, strategy := range strategies {
		var config StrategyConfig
		if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
			return nil, err
		}
		normalizeSystemStrategyConfig(&config)
		if config.Period != "day" {
			return nil, fmt.Errorf("策略 %s 不是日线策略", strategy.Name)
		}
		symbols, err := r.strategyUniverse(config)
		if err != nil {
			return nil, fmt.Errorf("策略 %s 候选范围失败: %w", strategy.Name, err)
		}
		for _, symbol := range symbols {
			allSymbols[symbol] = true
		}
		plans = append(plans, historicalStrategyPlan{strategy: strategy, config: config, symbols: symbols})
	}
	symbols := make([]string, 0, len(allSymbols))
	for symbol := range allSymbols {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	maxHorizon := req.Horizons[len(req.Horizons)-1]
	historyCount := historicalHistoryCount(start, end, time.Now().In(time.Local), maxHorizon)
	loadSymbols := append([]string{}, symbols...)
	for _, plan := range plans {
		if strategyUsesMarketContext(plan.config) {
			loadSymbols = append(loadSymbols, strategyBenchmarkSymbol)
			break
		}
	}
	klines, loadErrors := r.loadSystemBatchKlines(ctx, loadSymbols, historyCount)
	loadedCount := countLoadedStrategySymbols(symbols, klines)
	failedCount := countFailedStrategySymbols(symbols, loadErrors)
	dates := historicalTradingDates(klines, dateInt(start), dateInt(end))
	if len(dates) == 0 {
		return nil, errors.New("所选日期区间没有可用交易日K线")
	}
	if err := r.store.UpdateHistoricalBacktestProgress(run.ID, len(dates), 0, "", len(symbols), 0); err != nil {
		return nil, err
	}
	summaries := map[string]*historicalStrategySummary{}
	for _, plan := range plans {
		summary := &historicalStrategySummary{StrategyID: plan.strategy.ID, StrategyName: plan.strategy.Name, Horizons: map[string]*historicalHorizonAccumulator{}}
		for _, horizon := range req.Horizons {
			summary.Horizons[fmt.Sprintf("d%d", horizon)] = &historicalHorizonAccumulator{}
		}
		summaries[plan.strategy.ID] = summary
	}
	consensus := map[string]*historicalConsensusAccumulator{}
	trackingBars := map[string][]workbench.TrackingBar{}
	signalCount := 0
	sim := newPortfolioSimulator(portfolioSimConfig{
		InitialCash: req.InitialCash, MaxPositions: req.MaxPositions, BuyCost: req.BuyCost, SellCost: req.SellCost,
		BuyBase: req.BuyBase, BuyOffset: req.BuyOffset, SellBase: req.SellBase,
		TakeProfit: req.TakeProfit, TrailDrawdown: req.TrailDrawdown, StopLoss: req.StopLoss, MaxHoldDays: req.MaxHoldDays,
	})
	pendingBuys := []portfolioPendingBuy{}
	for dateIndex, signalDate := range dates {
		if ctx.Err() != nil || r.store.HistoricalBacktestCancelRequested(run.ID) {
			return historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount), context.Canceled
		}
		dateText := historicalDateText(signalDate)
		// 当日开盘:先执行昨日收盘触发的卖出,再执行昨日信号的买入
		sim.onDayOpen(signalDate, dateText, klines, pendingBuys)
		pendingBuys = nil
		daySignals := make([]HistoricalBacktestSignal, 0)
		for _, plan := range plans {
			strategyResult := &StrategyRunResult{
				Strategy: plan.strategy, Config: plan.config, Errors: map[string]string{}, KlineCache: map[string][]FormulaKline{},
				PoolCache: map[string]map[string]bool{}, FormulaCache: map[string]map[string]bool{}, FormulaDetail: map[string]map[string]any{},
				FormulaByName: map[string]Formula{}, FormulaByID: map[string]Formula{},
			}
			available := make([]string, 0, len(plan.symbols))
			if strategyUsesMarketContext(plan.config) {
				if rows, ok := historicalRowsThroughDate(klines[strategyBenchmarkSymbol], signalDate); ok {
					strategyResult.KlineCache[strategyBenchmarkSymbol] = rows
				}
			}
			if !r.strategyMarketContextPasses(strategyResult) {
				continue
			}
			for _, symbol := range plan.symbols {
				rows, ok := historicalRowsThroughDate(klines[symbol], signalDate)
				if !ok {
					continue
				}
				strategyResult.KlineCache[symbol] = rows
				available = append(available, symbol)
			}
			if err := r.prepareStrategyFormulasWithKlines(ctx, strategyResult, available, strategyResult.KlineCache); err != nil {
				return historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount), fmt.Errorf("策略 %s 在 %s 的公式计算失败: %w", plan.strategy.Name, dateText, err)
			}
			matches := make([]StrategySelectionItem, 0)
			for _, symbol := range available {
				item, ok := r.evaluateStrategySymbol(ctx, plan.strategy, strategyResult, symbol)
				if ok {
					matches = append(matches, item)
				}
			}
			matches = rankHistoricalMatches(matches, plan.config.Pass.TopN)
			for _, item := range matches {
				selection := SelectionResult{Symbol: item.Symbol, Latest: item.Latest, CreatedAt: dateText + "T16:00:00+08:00"}
				bars, ok := trackingBars[item.Symbol]
				if !ok {
					bars = historicalTrackingBars(klines[item.Symbol])
					trackingBars[item.Symbol] = bars
				}
				tracking := workbench.EvaluateSelectionTracking(selection, bars, req.Horizons, req.TargetReturn, req.DrawdownLimit, time.Now())
				trackingJSON := mustJSON(tracking)
				daySignals = append(daySignals, HistoricalBacktestSignal{
					RunID: run.ID, StrategyID: plan.strategy.ID, StrategyName: plan.strategy.Name, SignalDate: dateText,
					Symbol: item.Symbol, Latest: item.Latest, Score: item.Score, DetailJSON: mustJSON(item), TrackingJSON: trackingJSON,
				})
				pendingBuys = mergePendingBuy(pendingBuys, item.Symbol, item.Score, signalDate, plan.strategy.Name, item.Latest, plan.config.Execution)
				updateHistoricalSummary(summaries[plan.strategy.ID], tracking, req.Horizons)
				key := dateText + ":" + item.Symbol
				entry := consensus[key]
				if entry == nil {
					entry = &historicalConsensusAccumulator{date: dateText, symbol: item.Symbol, strategies: map[string]bool{}, tracking: trackingJSON}
					consensus[key] = entry
				}
				entry.strategies[plan.strategy.Name] = true
			}
		}
		if err := r.store.InsertHistoricalBacktestSignals(daySignals); err != nil {
			return nil, err
		}
		// 当日收盘:持仓退出条件判定与权益结算
		sim.onDayClose(signalDate, dateText, klines)
		signalCount += len(daySignals)
		if err := r.store.UpdateHistoricalBacktestProgress(run.ID, len(dates), dateIndex+1, dateText, len(symbols), signalCount); err != nil {
			return nil, err
		}
	}
	// 组合模拟收尾:未平仓转 open 记录并落库
	trades := sim.finalize(run.ID)
	if len(trades) > 0 {
		if err := r.store.InsertHistoricalBacktestTrades(trades); err != nil {
			return nil, fmt.Errorf("交易流水保存失败: %w", err)
		}
	}
	result := historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount)
	result["portfolio"] = sim.summary()
	return result, nil
}

func historicalHistoryCount(start, end, reference time.Time, maxHorizon int) int {
	// K-line loaders return the latest N bars, so an older, narrow replay still
	// needs enough rows to reach its start date plus indicator warm-up history.
	if reference.Before(end) {
		reference = end
	}
	calendarDays := int(reference.Sub(start).Hours()/24) + 1
	historyCount := calendarDays*5/7 + 180 + maxHorizon
	if historyCount < 260 {
		historyCount = 260
	}
	if historyCount > 2000 {
		historyCount = 2000
	}
	return historyCount
}

func historicalTradingDates(klines map[string][]FormulaKline, startDate, endDate int) []int {
	set := map[int]bool{}
	for _, rows := range klines {
		for _, row := range rows {
			if row.Date >= startDate && row.Date <= endDate {
				set[row.Date] = true
			}
		}
	}
	dates := make([]int, 0, len(set))
	for date := range set {
		dates = append(dates, date)
	}
	sort.Ints(dates)
	return dates
}

func historicalRowsThroughDate(rows []FormulaKline, signalDate int) ([]FormulaKline, bool) {
	endIndex := sort.Search(len(rows), func(i int) bool { return rows[i].Date > signalDate })
	if endIndex == 0 || rows[endIndex-1].Date != signalDate {
		return nil, false
	}
	return rows[:endIndex], true
}

func rankHistoricalMatches(items []StrategySelectionItem, topN int) []StrategySelectionItem {
	return rankStrategyMatches(items, topN)
}

func historicalTrackingBars(rows []FormulaKline) []workbench.TrackingBar {
	bars := make([]workbench.TrackingBar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, workbench.TrackingBar{Date: row.Date, YClose: row.YClose, Open: row.Open, High: row.High, Low: row.Low, Close: row.Close})
	}
	return bars
}

func historicalDateText(date int) string {
	if date <= 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", date/10000, (date/100)%100, date%100)
}

func updateHistoricalSummary(summary *historicalStrategySummary, tracking workbench.SelectionTracking, horizons []int) {
	if summary == nil {
		return
	}
	summary.SignalCount++
	for _, horizon := range horizons {
		key := fmt.Sprintf("d%d", horizon)
		acc := summary.Horizons[key]
		metric, ok := tracking.Horizons[key]
		if !ok || metric.Status != "complete" {
			acc.Pending++
			continue
		}
		acc.Completed++
		if metric.Success {
			acc.SuccessCount++
		}
		acc.returnTotal += metric.CloseReturn
		acc.gainTotal += metric.MaxGain
		acc.drawdownTotal += metric.MaxDrawdown
	}
}

func historicalBacktestResult(req historicalBacktestRequest, dates []int, summaries map[string]*historicalStrategySummary, consensus map[string]*historicalConsensusAccumulator, candidates, loaded, failed, historyCount int) map[string]interface{} {
	strategyRows := make([]*historicalStrategySummary, 0, len(summaries))
	for _, summary := range summaries {
		for _, acc := range summary.Horizons {
			if acc.Completed > 0 {
				count := float64(acc.Completed)
				acc.SuccessRate = float64(acc.SuccessCount) / count * 100
				acc.AverageReturn = acc.returnTotal / count
				acc.AverageMaxGain = acc.gainTotal / count
				acc.AverageDrawdown = acc.drawdownTotal / count
			}
		}
		strategyRows = append(strategyRows, summary)
	}
	sort.Slice(strategyRows, func(i, j int) bool {
		left, right := strategyRows[i].Horizons["d5"], strategyRows[j].Horizons["d5"]
		if left != nil && right != nil && left.SuccessRate != right.SuccessRate {
			return left.SuccessRate > right.SuccessRate
		}
		return strategyRows[i].SignalCount > strategyRows[j].SignalCount
	})
	consensusRows := make([]historicalConsensusSummary, 0)
	for _, item := range consensus {
		if len(item.strategies) < 2 {
			continue
		}
		names := make([]string, 0, len(item.strategies))
		for name := range item.strategies {
			names = append(names, name)
		}
		sort.Strings(names)
		consensusRows = append(consensusRows, historicalConsensusSummary{SignalDate: item.date, Symbol: item.symbol, StrategyCount: len(names), Strategies: names, TrackingJSON: item.tracking})
	}
	sort.Slice(consensusRows, func(i, j int) bool {
		if consensusRows[i].StrategyCount != consensusRows[j].StrategyCount {
			return consensusRows[i].StrategyCount > consensusRows[j].StrategyCount
		}
		if consensusRows[i].SignalDate != consensusRows[j].SignalDate {
			return consensusRows[i].SignalDate > consensusRows[j].SignalDate
		}
		return consensusRows[i].Symbol < consensusRows[j].Symbol
	})
	if len(consensusRows) > 500 {
		consensusRows = consensusRows[:500]
	}
	totalSignals := 0
	for _, summary := range strategyRows {
		totalSignals += summary.SignalCount
	}
	return map[string]interface{}{
		"kline_source":       "market-service (Hikyuu local first, TDX fallback)",
		"strategy_summaries": strategyRows, "consensus": consensusRows, "signal_count": totalSignals,
		"strategy_count": len(strategyRows), "trading_days": len(dates), "candidate_symbols": candidates,
		"kline_loaded": loaded, "kline_failed": failed, "history_count": historyCount,
		"policy":   map[string]interface{}{"horizons": req.Horizons, "target_return": req.TargetReturn, "drawdown_limit": req.DrawdownLimit, "entry": "signal_day_close"},
		"warnings": []string{"候选范围使用当前股票列表，历史退市股票可能未被包含", "信号按当日收盘生成，D3/D5/D10均按后续交易日计算"},
	}
}
