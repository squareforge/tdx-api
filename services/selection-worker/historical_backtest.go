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
	StartDate       string   `json:"start_date"`
	EndDate         string   `json:"end_date"`
	StrategyIDs     []string `json:"strategy_ids"`
	UniversePoolID  string   `json:"universe_pool_id,omitempty"`
	UniversePoolIDs []string `json:"universe_pool_ids"`
	Horizons        []int    `json:"horizons"`
	TargetReturn    float64  `json:"target_return"`
	DrawdownLimit   float64  `json:"drawdown_limit"`
	InitialCash     float64  `json:"initial_cash"`
	MaxPositions    int      `json:"max_positions"`
	BuyCost         float64  `json:"buy_cost"`
	SellCost        float64  `json:"sell_cost"`
	BuyBase         string   `json:"buy_base"`
	BuyOffset       float64  `json:"buy_offset"`
	SellBase        string   `json:"sell_base"`
	TakeProfit      float64  `json:"take_profit"`
	TrailDrawdown   float64  `json:"trail_drawdown"`
	StopLoss        float64  `json:"stop_loss"`
	MaxHoldDays     int      `json:"max_hold_days"`
}

type historicalStrategyPlan struct {
	strategy        Strategy
	config          StrategyConfig
	symbols         []string
	fundamentalData *strategyFundamentalData
	rawKlines       map[string][]FormulaKline
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
	SignalDate    string                      `json:"signal_date"`
	Symbol        string                      `json:"symbol"`
	StrategyCount int                         `json:"strategy_count"`
	Strategies    []string                    `json:"strategies"`
	Tracking      workbench.SelectionTracking `json:"tracking"`
}

type historicalConsensusAccumulator struct {
	date       string
	symbol     string
	strategies map[string]bool
	tracking   workbench.SelectionTracking
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
	if len(parts) >= 2 && parts[1] == "comparisons" {
		handleHistoricalComparisonOperations(w, r, id, parts[2:])
		return
	}
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
		horizons = []int{1, 3, 5, 10}
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
	// universe_pool_id is kept as a compatibility input for callers created
	// before the replay page supported selecting more than one stock pool.
	req.UniversePoolIDs = normalizeIDList(append(req.UniversePoolIDs, req.UniversePoolID))
	req.UniversePoolID = ""
	return req, start, end, nil
}

func (r *AutomationRunner) historicalBacktestUniverse(poolIDs []string) ([]string, error) {
	poolIDs = normalizeIDList(poolIDs)
	if len(poolIDs) == 0 {
		return nil, nil
	}
	// Multiple pools form one replay range by union. A set keyed by normalized
	// stock code removes overlaps before the range is intersected with each
	// strategy's own universe.
	poolSymbols := make([][]string, 0, len(poolIDs))
	for _, poolID := range poolIDs {
		pool, err := r.store.GetStockPool(poolID)
		if err != nil {
			return nil, fmt.Errorf("选股范围不存在: %s", poolID)
		}
		poolSymbols = append(poolSymbols, pool.Symbols)
	}
	symbols := mergeHistoricalSymbols(poolSymbols...)
	if len(symbols) == 0 {
		return nil, errors.New("所选股票池合并后为空")
	}
	return symbols, nil
}

func mergeHistoricalSymbols(groups ...[]string) []string {
	selected := map[string]bool{}
	for _, group := range groups {
		for _, rawSymbol := range group {
			if symbol := strategyNormalizeSymbol(rawSymbol); symbol != "" {
				selected[symbol] = true
			}
		}
	}
	symbols := make([]string, 0, len(selected))
	for symbol := range selected {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return symbols
}

func intersectHistoricalSymbols(strategySymbols, requestedSymbols []string) []string {
	if len(requestedSymbols) == 0 {
		return strategySymbols
	}
	requested := make(map[string]bool, len(requestedSymbols))
	for _, symbol := range requestedSymbols {
		requested[symbol] = true
	}
	result := make([]string, 0, len(strategySymbols))
	for _, symbol := range strategySymbols {
		if requested[symbol] {
			result = append(result, symbol)
		}
	}
	return result
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
	if _, err := r.historicalBacktestUniverse(req.UniversePoolIDs); err != nil {
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
		UniversePoolIDs:      req.UniversePoolIDs,
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
		if result == nil {
			result = map[string]interface{}{}
		}
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
	profile := openHistoricalBacktestProfile(run.ID)
	profile.record("start", map[string]any{"start_date": req.StartDate, "end_date": req.EndDate, "strategies": len(strategies)})
	result, err := r.runHistoricalBacktestWithProfile(ctx, run, req, strategies, profile)
	profile.close(err)
	return result, err
}

func (r *AutomationRunner) runHistoricalBacktestWithProfile(ctx context.Context, run HistoricalBacktestRun, req historicalBacktestRequest, strategies []Strategy, profile *historicalBacktestProfile) (map[string]interface{}, error) {
	runStarted := time.Now()

	// 1. 再做一次参数归一化。
	// startHistoricalBacktest 启动任务前已经校验过请求，这里重新调用是为了拿到
	// time.Time 类型的 start/end，方便后面计算需要加载多少历史 K 线，以及生成交易日范围。
	// req 里的默认值也会在这里被补齐，例如 horizons、资金、止盈止损等。
	_, start, end, err := normalizeHistoricalBacktestRequest(req)
	if err != nil {
		return nil, err
	}

	// 2. 为每个策略准备一份执行计划。
	// historicalStrategyPlan 会保存策略本身、解析后的策略配置、该策略要扫描的股票列表，
	// 以及可选的基本面/未复权 K 线缓存。后面的逐日回放只遍历 plans，不再直接遍历原始 strategies。
	plans := make([]historicalStrategyPlan, 0, len(strategies))
	allSymbols := map[string]bool{}
	stageStarted := time.Now()

	// 用户在前端可以选择一个或多个股票池。这里先把股票池合并并去重。
	// 如果用户没有选股票池，requestedUniverse 为空，后面会表示“跟随策略自己的候选范围”。
	requestedUniverse, err := r.historicalBacktestUniverse(req.UniversePoolIDs)
	if err != nil {
		return nil, err
	}
	for _, strategy := range strategies {
		var config StrategyConfig
		if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
			return nil, err
		}
		normalizeSystemStrategyConfig(&config)

		// 历史回测这里按日线重放。非日线策略没有日线 K 线语义，直接拒绝。
		if config.Period != "day" {
			return nil, fmt.Errorf("策略 %s 不是日线策略", strategy.Name)
		}

		// 先取策略自身定义的候选范围，再和用户选择的股票池求交集。
		// 这样用户股票池只做额外限定，不会把策略本来不覆盖的股票强行加入。
		strategySymbols, err := r.strategyUniverse(config)
		if err != nil {
			return nil, fmt.Errorf("策略 %s 候选范围失败: %w", strategy.Name, err)
		}
		symbols := intersectHistoricalSymbols(strategySymbols, requestedUniverse)

		// allSymbols 是所有策略实际需要加载行情的股票全集。
		// 多个策略可能覆盖同一只股票，这里用 map 去重，避免重复加载 K 线。
		for _, symbol := range symbols {
			allSymbols[symbol] = true
		}
		plans = append(plans, historicalStrategyPlan{strategy: strategy, config: config, symbols: symbols})
	}

	// 把股票全集转成稳定排序后的切片。排序本身不影响结果，但能让日志、进度和后续处理更稳定。
	symbols := make([]string, 0, len(allSymbols))
	for symbol := range allSymbols {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	if len(symbols) == 0 {
		return nil, errors.New("选股范围与所选策略的候选范围没有交集")
	}
	profile.record("stage", map[string]any{"name": "strategy_universe", "elapsed_ms": profileMillis(time.Since(stageStarted)), "symbols": len(symbols)})

	// 3. 计算需要加载多少根 K 线。
	// 回测区间可能很早，而行情接口按“最近 N 根”取数，所以不能只按 start/end 的区间长度加载。
	// historicalHistoryCount 会额外留出指标预热期和最大跟踪周期，尽量保证从 start 开始有足够历史数据。
	maxHorizon := req.Horizons[len(req.Horizons)-1]
	historyCount := historicalHistoryCount(start, end, time.Now().In(time.Local), maxHorizon)
	loadSymbols := append([]string{}, symbols...)

	// 如果策略使用大盘/市场环境过滤，需要把基准指数也加入行情加载列表。
	for _, plan := range plans {
		if strategyUsesMarketContext(plan.config) {
			loadSymbols = append(loadSymbols, strategyBenchmarkSymbol)
			break
		}
	}

	// 4. 批量加载前复权日线 K 线。
	// loadSystemBatchKlines 默认使用 recover=qfq。后续策略计算、信号价、D1/D3/D5/D10 跟踪
	// 都主要基于这批前复权日线数据。
	stageStarted = time.Now()
	klines, loadErrors := r.loadSystemBatchKlines(ctx, loadSymbols, historyCount)
	profile.record("stage", map[string]any{"name": "kline_load", "elapsed_ms": profileMillis(time.Since(stageStarted)), "symbols": len(loadSymbols), "history_count": historyCount})

	// 5. 如有策略使用基本面因子，则额外加载点时基本面和未复权 K 线。
	// 一些基本面计算需要未复权价格做辅助，所以这里把 recover=none 的 rawKlines
	// 放进对应策略计划里。没有基本面需求的策略不会走这段，减少额外开销。
	if historicalPlansUseFundamentals(plans) {
		stageStarted = time.Now()
		rawKlines, _ := r.loadSystemBatchKlinesWithRecover(ctx, symbols, historyCount, "none")
		profile.record("stage", map[string]any{"name": "raw_kline_load", "elapsed_ms": profileMillis(time.Since(stageStarted)), "symbols": len(symbols)})
		stageStarted = time.Now()
		fundamentalData, loadErr := loadStrategyFundamentalData(ctx, symbols, dateInt(start), dateInt(end))
		profile.record("stage", map[string]any{"name": "fundamental_load", "elapsed_ms": profileMillis(time.Since(stageStarted)), "symbols": len(symbols)})
		if loadErr != nil {
			return nil, fmt.Errorf("点时基本面数据加载失败: %w", loadErr)
		}
		for index := range plans {
			if strategyUsesFundamentals(plans[index].config) {
				plans[index].fundamentalData = fundamentalData
				plans[index].rawKlines = rawKlines
			}
		}
	}

	// 统计行情加载结果，供前端展示和结果 JSON 记录。
	loadedCount := countLoadedStrategySymbols(symbols, klines)
	failedCount := countFailedStrategySymbols(symbols, loadErrors)

	// 6. 从所有已加载 K 线中提取回测区间内的真实交易日。
	// 这里不使用自然日，而是用行情里实际存在的交易日，避免周末、节假日、停牌等日期影响回放。
	stageStarted = time.Now()
	dates := historicalTradingDates(klines, dateInt(start), dateInt(end))
	profile.record("stage", map[string]any{"name": "trading_dates", "elapsed_ms": profileMillis(time.Since(stageStarted)), "dates": len(dates), "loaded": loadedCount, "failed": failedCount})
	if len(dates) == 0 {
		return nil, errors.New("所选日期区间没有可用交易日K线")
	}
	if err := r.store.UpdateHistoricalBacktestProgress(run.ID, len(dates), 0, "", len(symbols), 0); err != nil {
		return nil, err
	}

	// 7. 初始化结果统计容器。
	// summaries 按策略累计信号数量和各跟踪周期的成功率；
	// consensus 用于记录同一日期、同一股票被多个策略同时选中的“共振”信号。
	summaries := map[string]*historicalStrategySummary{}
	for _, plan := range plans {
		summary := &historicalStrategySummary{StrategyID: plan.strategy.ID, StrategyName: plan.strategy.Name, Horizons: map[string]*historicalHorizonAccumulator{}}
		for _, horizon := range req.Horizons {
			summary.Horizons[fmt.Sprintf("d%d", horizon)] = &historicalHorizonAccumulator{}
		}
		summaries[plan.strategy.ID] = summary
	}
	consensus := map[string]*historicalConsensusAccumulator{}

	// trackingBars 是每只股票转换后的跟踪 K 线缓存。
	// 同一股票可能在很多天、多套策略里反复出现，缓存后可以避免重复构造 TrackingBar 切片。
	trackingBars := map[string][]workbench.TrackingBar{}
	signalCount := 0

	// 组合模拟器负责把每日信号转成真实交易流水和组合收益。
	// 它会处理 T+1、涨停买不进、跌停卖不出、整手、最大持仓、止盈止损等交易规则。
	sim := newPortfolioSimulator(portfolioSimConfig{
		InitialCash: req.InitialCash, MaxPositions: req.MaxPositions, BuyCost: req.BuyCost, SellCost: req.SellCost,
		BuyBase: req.BuyBase, BuyOffset: req.BuyOffset, SellBase: req.SellBase,
		TakeProfit: req.TakeProfit, TrailDrawdown: req.TrailDrawdown, StopLoss: req.StopLoss, MaxHoldDays: req.MaxHoldDays,
	})

	// pendingBuys 保存“今天收盘产生、明天开盘尝试买入”的信号。
	// 每个交易日开盘时先处理上一天留下的 pendingBuys，然后清空。
	pendingBuys := []portfolioPendingBuy{}
	evaluationWorkers := strategyEvaluationConcurrency()
	totalOpen, totalScan, totalInsert, totalClose, totalProgress := time.Duration(0), time.Duration(0), time.Duration(0), time.Duration(0), time.Duration(0)

	// 8. 核心循环：按交易日逐日回放。
	// 这是历史回测最关键的一层：每一天只允许策略看到“截至当天”的数据，
	// 然后生成当天收盘后的选股信号，并把这些信号留给下一交易日执行买入。
	for dateIndex, signalDate := range dates {
		if ctx.Err() != nil || r.store.HistoricalBacktestCancelRequested(run.ID) {
			return historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount), context.Canceled
		}
		dateText := historicalDateText(signalDate)
		dayStarted := time.Now()

		// 当日开盘:先执行此前顺延的卖单,再执行昨日信号的买入
		sim.onDayOpen(signalDate, dateText, klines, pendingBuys)
		openDuration := time.Since(dayStarted)
		totalOpen += openDuration
		pendingBuys = nil

		// daySignals 只保存当天新产生的历史信号，稍后会批量落库。
		daySignals := make([]HistoricalBacktestSignal, 0)
		strategyTimings := make([]historicalStrategyTiming, 0, len(plans))
		scanStarted := time.Now()

		// 逐个策略在当天做一次“点时”扫描。
		for _, plan := range plans {
			strategyStarted := time.Now()
			timing := historicalStrategyTiming{Name: plan.strategy.Name}

			// StrategyRunResult 是策略执行时的上下文容器。
			// 这里手动填充 KlineCache、RawKlineCache、FundamentalData 等缓存，
			// 让 evaluateStrategyDay 可以复用普通策略执行逻辑，同时保证数据只截止到当天。
			strategyResult := &StrategyRunResult{
				Strategy: plan.strategy, Config: plan.config, Errors: map[string]string{}, KlineCache: map[string][]FormulaKline{},
				PoolCache: map[string]map[string]bool{}, FormulaCache: map[string]map[string]bool{}, FormulaDetail: map[string]map[string]any{},
				FormulaByName: map[string]Formula{}, FormulaByID: map[string]Formula{}, RawKlineCache: map[string][]FormulaKline{},
				FundamentalData: plan.fundamentalData, FundamentalRanks: map[string]map[string]float64{},
			}
			available := make([]string, 0, len(plan.symbols))

			// 如果策略需要市场环境过滤，先把基准指数截至当天的数据放进去。
			// 市场环境不过关时，这个策略当天直接跳过，不扫描个股。
			if strategyUsesMarketContext(plan.config) {
				if rows, ok := historicalRowsThroughDate(klines[strategyBenchmarkSymbol], signalDate); ok {
					strategyResult.KlineCache[strategyBenchmarkSymbol] = rows
				}
			}
			if !r.strategyMarketContextPasses(strategyResult) {
				timing.PreparationMS = profileMillis(time.Since(strategyStarted))
				strategyTimings = append(strategyTimings, timing)
				continue
			}

			// 给策略准备每只股票“截至当天”的 K 线。
			// historicalRowsThroughDate 要求当天必须有 K 线；如果股票当天停牌或缺数据，就不会加入 available。
			for _, symbol := range plan.symbols {
				rows, ok := historicalRowsThroughDate(klines[symbol], signalDate)
				if !ok {
					continue
				}
				strategyResult.KlineCache[symbol] = rows
				if rawRows, rawOK := historicalRowsThroughDate(plan.rawKlines[symbol], signalDate); rawOK {
					strategyResult.RawKlineCache[symbol] = rawRows
				}
				available = append(available, symbol)
			}
			timing.Available = len(available)
			timing.PreparationMS = profileMillis(time.Since(strategyStarted))

			// 真正执行当天策略扫描。
			// matches 是当天收盘后命中的选股结果，里面包含股票、得分、命中原因、因子结果等。
			matches, formulaDuration, evaluationDuration, err := r.evaluateStrategyDay(ctx, plan.strategy, strategyResult, available, evaluationWorkers, &timing.Formula)
			if err != nil {
				return historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount), fmt.Errorf("策略 %s 在 %s 的公式计算失败: %w", plan.strategy.Name, dateText, err)
			}
			timing.FormulaMS = profileMillis(formulaDuration)
			timing.EvaluationMS = profileMillis(evaluationDuration)
			timing.Matches = len(matches)
			trackingStarted := time.Now()

			// 把当天命中的每条选股结果转成历史信号，并计算后续 D1/D3/D5/D10 的表现。
			for _, item := range matches {
				// selection.CreatedAt 使用当天 16:00，表达“收盘后产生信号”的时间语义。
				selection := SelectionResult{Symbol: item.Symbol, Latest: item.Latest, CreatedAt: dateText + "T16:00:00+08:00"}
				bars, ok := trackingBars[item.Symbol]
				if !ok {
					bars = historicalTrackingBars(klines[item.Symbol])
					trackingBars[item.Symbol] = bars
				}

				// EvaluateSelectionTracking 会从信号日之后的 K 线计算各观察周期的收益、
				// 最大涨幅、最大回撤，以及是否达到 req.TargetReturn / req.DrawdownLimit 标准。
				tracking := workbench.EvaluateSelectionTracking(selection, bars, req.Horizons, req.TargetReturn, req.DrawdownLimit, time.Now())
				daySignals = append(daySignals, HistoricalBacktestSignal{
					RunID: run.ID, StrategyID: plan.strategy.ID, StrategyName: plan.strategy.Name, SignalDate: dateText,
					Symbol: item.Symbol, Latest: item.Latest, Score: item.Score, DetailJSON: mustJSON(item), Tracking: &tracking,
				})

				// 同一天同一股票可能被多个策略选中，组合交易只买一次。
				// mergePendingBuy 会按得分保留更强的买入配置，并把策略名合并起来。
				pendingBuys = mergePendingBuy(pendingBuys, item.Symbol, item.Score, signalDate, plan.strategy.Name, item.Latest, plan.config.Execution)
				updateHistoricalSummary(summaries[plan.strategy.ID], tracking, req.Horizons)

				// 共振统计的 key 是“日期 + 股票”。如果同一股票同一天被多个策略选中，
				// 最终会在 result["consensus"] 里展示。
				key := dateText + ":" + item.Symbol
				entry := consensus[key]
				if entry == nil {
					entry = &historicalConsensusAccumulator{date: dateText, symbol: item.Symbol, strategies: map[string]bool{}, tracking: tracking}
					consensus[key] = entry
				}
				entry.strategies[plan.strategy.Name] = true
			}
			timing.TrackingMS = profileMillis(time.Since(trackingStarted))
			strategyTimings = append(strategyTimings, timing)
		}
		scanDuration := time.Since(scanStarted)
		totalScan += scanDuration

		// 当天全部策略扫描完后，把当天信号批量写入数据库。
		// InsertHistoricalBacktestSignals 还会同步写 selection_results 和 selection_result_tracking，
		// 所以前端信号明细展开时能看到 D1/D3/D5/D10 跟踪详情。
		stageStarted = time.Now()
		if err := r.store.InsertHistoricalBacktestSignals(daySignals); err != nil {
			return nil, err
		}
		insertDuration := time.Since(stageStarted)
		totalInsert += insertDuration

		// 回放当日盘中风控触价和到期退出,并按收盘价结算剩余持仓权益
		stageStarted = time.Now()
		sim.onDayClose(signalDate, dateText, klines)
		closeDuration := time.Since(stageStarted)
		totalClose += closeDuration
		signalCount += len(daySignals)

		// 更新批次进度，前端会通过轮询展示当前交易日、已处理交易日和信号总数。
		stageStarted = time.Now()
		if err := r.store.UpdateHistoricalBacktestProgress(run.ID, len(dates), dateIndex+1, dateText, len(symbols), signalCount); err != nil {
			return nil, err
		}
		progressDuration := time.Since(stageStarted)
		totalProgress += progressDuration
		profile.record("day", map[string]any{
			"date": dateText, "index": dateIndex + 1, "signals": len(daySignals),
			"elapsed_ms": profileMillis(time.Since(dayStarted)), "open_ms": profileMillis(openDuration),
			"scan_ms": profileMillis(scanDuration), "insert_ms": profileMillis(insertDuration),
			"close_ms": profileMillis(closeDuration), "progress_ms": profileMillis(progressDuration),
			"strategies": strategyTimings,
		})
	}

	// 组合模拟收尾:未平仓转 open 记录并落库
	stageStarted = time.Now()
	trades := sim.finalize(run.ID)
	if len(trades) > 0 {
		if err := r.store.InsertHistoricalBacktestTrades(trades); err != nil {
			return nil, fmt.Errorf("交易流水保存失败: %w", err)
		}
	}
	profile.record("stage", map[string]any{"name": "trade_finalize", "elapsed_ms": profileMillis(time.Since(stageStarted)), "trades": len(trades)})
	stageStarted = time.Now()

	// 9. 生成最终结果 JSON。
	// historicalBacktestResult 汇总策略排行、共振信号、行情加载数量、策略成功率等；
	// portfolio 则来自组合模拟器，包含期末权益、总收益、最大回撤、胜率、未成交数量等。
	result := historicalBacktestResult(req, dates, summaries, consensus, len(symbols), loadedCount, failedCount, historyCount)
	result["portfolio"] = sim.summary()
	profile.record("summary", map[string]any{
		"elapsed_ms": profileMillis(time.Since(runStarted)), "days": len(dates), "signals": signalCount,
		"open_ms": profileMillis(totalOpen), "scan_ms": profileMillis(totalScan),
		"insert_ms": profileMillis(totalInsert), "close_ms": profileMillis(totalClose),
		"progress_ms": profileMillis(totalProgress), "result_ms": profileMillis(time.Since(stageStarted)),
	})
	return result, nil
}

func historicalHistoryCount(start, end, reference time.Time, maxHorizon int) int {
	// K-line loaders return the latest N bars, so an older, narrow replay still
	// needs enough rows to reach its start date plus indicator warm-up history.
	// 举例：如果今天是 2026-10-09，但回测只跑 2026-01-01 到 2026-01-10，
	// 行情接口仍然是“从今天往前取 N 根”，不是“只取 1 月这段”。N 太小就会只拿到最近
	// 几个月数据，导致回测起点没有 K 线。这里按 start 到 reference 的自然日跨度估算
	// 交易日数量，再额外加 180 根作为指标预热，并加上 maxHorizon 作为信号后续跟踪窗口。
	if reference.Before(end) {
		reference = end
	}
	calendarDays := int(reference.Sub(start).Hours()/24) + 1
	historyCount := calendarDays*5/7 + 180 + maxHorizon
	// 最少取 260 根，保证常见年线/长周期指标有足够预热数据；最多取 2000 根，
	// 避免很早的回测一次性拉取过多行情，把接口和内存压力放大。
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
		consensusRows = append(consensusRows, historicalConsensusSummary{SignalDate: item.date, Symbol: item.symbol, StrategyCount: len(names), Strategies: names, Tracking: item.tracking})
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
		"policy":   map[string]interface{}{"horizons": req.Horizons, "target_return": req.TargetReturn, "drawdown_limit": req.DrawdownLimit, "entry": "signal_day_close", "universe_pool_ids": req.UniversePoolIDs},
		"warnings": []string{"候选范围使用当前股票列表，历史退市股票可能未被包含", "信号按当日收盘生成，D1/D3/D5/D10均按后续交易日计算"},
	}
}
