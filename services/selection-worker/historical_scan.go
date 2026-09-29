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

// 参数扫描:逐策略遍历买卖参数组合,信号只计算一次,每种组合重放轻量买卖模拟,
// 找出每个策略各自的最优参数。设计见 docs 中历史回测参数扫描说明。

// historicalScanSignal 单个策略在某交易日的命中信号。
type historicalScanSignal struct {
	SignalDate  int
	Symbol      string
	Score       float64
	SignalClose float64 // 信号日收盘价,买入基准"close"时作为限价参考
	Execution   StrategyExecutionConfig
}

// scanParamSet 一组买卖参数组合。
type scanParamSet struct {
	BuyBase       string  `json:"buy_base"`
	BuyOffset     float64 `json:"buy_offset"`
	SellBase      string  `json:"sell_base"`
	TakeProfit    float64 `json:"take_profit"`
	TrailDrawdown float64 `json:"trail_drawdown"`
	StopLoss      float64 `json:"stop_loss"`
	MaxHoldDays   int     `json:"max_hold_days"`
}

// scanGrid 参数扫描网格的候选值。
type scanGrid struct {
	BuyBases       []string
	SellBases      []string
	TakeProfits    []float64
	StopLosses     []float64
	TrailDrawdowns []float64
	MaxHoldDays    []int
}

type historicalScanScope struct {
	PoolIDs []string
	Name    string
	Symbols []string
}

const maxHistoricalScanPools = 12

func (r *AutomationRunner) historicalScanScopes(poolIDs []string) ([]historicalScanScope, error) {
	poolIDs = normalizeIDList(poolIDs)
	if len(poolIDs) == 0 {
		return []historicalScanScope{{Name: "策略自带范围"}}, nil
	}
	if len(poolIDs) > maxHistoricalScanPools {
		return nil, fmt.Errorf("参数扫描最多选择 %d 个股票池，当前选择了 %d 个", maxHistoricalScanPools, len(poolIDs))
	}
	pools := make([]StockPool, 0, len(poolIDs))
	for _, poolID := range poolIDs {
		pool, err := r.store.GetStockPool(poolID)
		if err != nil {
			return nil, fmt.Errorf("选股范围不存在: %s", poolID)
		}
		if len(normalizeSymbols(pool.Symbols)) == 0 {
			return nil, fmt.Errorf("选股范围为空: %s", pool.Name)
		}
		pools = append(pools, pool)
	}
	return buildHistoricalScanScopes(pools)
}

func buildHistoricalScanScopes(pools []StockPool) ([]historicalScanScope, error) {
	scopes := make([]historicalScanScope, 0, (1<<len(pools))-1)
	for mask := 1; mask < (1 << len(pools)); mask++ {
		ids := make([]string, 0, len(pools))
		names := make([]string, 0, len(pools))
		groups := make([][]string, 0, len(pools))
		for index, pool := range pools {
			if mask&(1<<index) == 0 {
				continue
			}
			ids = append(ids, pool.ID)
			names = append(names, pool.Name)
			groups = append(groups, pool.Symbols)
		}
		symbols := mergeHistoricalSymbols(groups...)
		if len(symbols) == 0 {
			continue
		}
		scopes = append(scopes, historicalScanScope{PoolIDs: ids, Name: strings.Join(names, " + "), Symbols: symbols})
	}
	if len(scopes) == 0 {
		return nil, errors.New("所选股票池组合合并后为空")
	}
	return scopes, nil
}

func defaultScanGrid() scanGrid {
	return scanGrid{
		BuyBases:       []string{"open", "close"},
		SellBases:      []string{"next_open", "same_close"},
		TakeProfits:    []float64{2, 3, 5, 10, 15},
		StopLosses:     []float64{3, 5},
		TrailDrawdowns: []float64{5, 8},
		MaxHoldDays:    []int{5, 10, 15, 20},
	}
}

func (g scanGrid) combinations() []scanParamSet {
	sets := make([]scanParamSet, 0, len(g.BuyBases)*len(g.SellBases)*len(g.TakeProfits)*len(g.StopLosses)*len(g.TrailDrawdowns)*len(g.MaxHoldDays))
	for _, buyBase := range g.BuyBases {
		for _, sellBase := range g.SellBases {
			for _, takeProfit := range g.TakeProfits {
				for _, stopLoss := range g.StopLosses {
					for _, trailDrawdown := range g.TrailDrawdowns {
						for _, maxHoldDays := range g.MaxHoldDays {
							sets = append(sets, scanParamSet{
								BuyBase: buyBase, BuyOffset: 0, SellBase: sellBase,
								TakeProfit: takeProfit, TrailDrawdown: trailDrawdown,
								StopLoss: stopLoss, MaxHoldDays: maxHoldDays,
							})
						}
					}
				}
			}
		}
	}
	return sets
}

// generateStrategySignals 生成单个策略在日期区间内的所有信号,复用现有评估逻辑。
func (r *AutomationRunner) generateStrategySignals(ctx context.Context, plan historicalStrategyPlan, klines map[string][]FormulaKline, dates []int) ([]historicalScanSignal, error) {
	signals := make([]historicalScanSignal, 0)
	evaluationWorkers := strategyEvaluationConcurrency()
	for _, signalDate := range dates {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		strategyResult := &StrategyRunResult{
			Strategy: plan.strategy, Config: plan.config, Errors: map[string]string{}, KlineCache: map[string][]FormulaKline{},
			PoolCache: map[string]map[string]bool{}, FormulaCache: map[string]map[string]bool{}, FormulaDetail: map[string]map[string]any{},
			FormulaByName: map[string]Formula{}, FormulaByID: map[string]Formula{}, RawKlineCache: map[string][]FormulaKline{},
			FundamentalData: plan.fundamentalData, FundamentalRanks: map[string]map[string]float64{},
		}
		if strategyUsesMarketContext(plan.config) {
			if rows, ok := historicalRowsThroughDate(klines[strategyBenchmarkSymbol], signalDate); ok {
				strategyResult.KlineCache[strategyBenchmarkSymbol] = rows
			}
		}
		if !r.strategyMarketContextPasses(strategyResult) {
			continue
		}
		available := make([]string, 0, len(plan.symbols))
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
		matches, _, _, err := r.evaluateStrategyDay(ctx, plan.strategy, strategyResult, available, evaluationWorkers, nil)
		if err != nil {
			return nil, err
		}
		for _, item := range matches {
			signals = append(signals, historicalScanSignal{
				SignalDate: signalDate, Symbol: item.Symbol, Score: item.Score, SignalClose: item.Latest, Execution: plan.config.Execution,
			})
		}
	}
	return signals, nil
}

// replayHistoricalPortfolio 用给定信号与买卖参数重放单策略组合模拟,返回摘要。
// 信号于 T 日收盘产生,撮合在 T+1 日开盘执行,与组合模拟引擎口径一致。
func replayHistoricalPortfolio(klines map[string][]FormulaKline, dates []int, signals []historicalScanSignal, cfg portfolioSimConfig) map[string]interface{} {
	byDate := map[int][]portfolioPendingBuy{}
	for _, sig := range signals {
		byDate[sig.SignalDate] = append(byDate[sig.SignalDate], portfolioPendingBuy{
			symbol: sig.Symbol, score: sig.Score, signalDate: sig.SignalDate, signalClose: sig.SignalClose,
			minOpenGap: sig.Execution.MinOpenGap, maxOpenGap: sig.Execution.MaxOpenGap,
		})
	}
	sim := newPortfolioSimulator(cfg)
	for i, date := range dates {
		dateText := historicalDateText(date)
		var buys []portfolioPendingBuy
		if i > 0 {
			buys = byDate[dates[i-1]]
		}
		sim.onDayOpen(date, dateText, klines, buys)
		sim.onDayClose(date, dateText, klines)
	}
	_ = sim.finalize("scan")
	return sim.summary()
}

// runParameterScan 逐策略遍历参数组合,信号计算一次、组合重放多次,结果落库。
func (r *AutomationRunner) runParameterScan(ctx context.Context, runID string, req historicalBacktestRequest, strategies []Strategy) error {
	_, start, end, err := normalizeHistoricalBacktestRequest(req)
	if err != nil {
		return err
	}
	plans := make([]historicalStrategyPlan, 0, len(strategies))
	allSymbols := map[string]bool{}
	scopes, err := r.historicalScanScopes(req.UniversePoolIDs)
	if err != nil {
		return err
	}
	for _, strategy := range strategies {
		var config StrategyConfig
		if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
			return err
		}
		normalizeSystemStrategyConfig(&config)
		if config.Period != "day" {
			return fmt.Errorf("策略 %s 不是日线策略", strategy.Name)
		}
		strategySymbols, err := r.strategyUniverse(config)
		if err != nil {
			return fmt.Errorf("策略 %s 候选范围失败: %w", strategy.Name, err)
		}
		for _, scope := range scopes {
			for _, symbol := range intersectHistoricalSymbols(strategySymbols, scope.Symbols) {
				allSymbols[symbol] = true
			}
		}
		plans = append(plans, historicalStrategyPlan{strategy: strategy, config: config, symbols: strategySymbols})
	}
	if len(plans) == 0 {
		return errors.New("没有可扫描的策略")
	}
	symbols := make([]string, 0, len(allSymbols))
	for symbol := range allSymbols {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	if len(symbols) == 0 {
		return errors.New("选股范围与所选策略的候选范围没有交集")
	}

	maxHorizon := req.Horizons[len(req.Horizons)-1]
	historyCount := historicalHistoryCount(start, end, time.Now().In(time.Local), maxHorizon)
	loadSymbols := append([]string{}, symbols...)
	for _, plan := range plans {
		if strategyUsesMarketContext(plan.config) {
			loadSymbols = append(loadSymbols, strategyBenchmarkSymbol)
			break
		}
	}
	klines, _ := r.loadSystemBatchKlines(ctx, loadSymbols, historyCount)
	if historicalPlansUseFundamentals(plans) {
		rawKlines, _ := r.loadSystemBatchKlinesWithRecover(ctx, symbols, historyCount, "none")
		fundamentalData, loadErr := loadStrategyFundamentalData(ctx, symbols, dateInt(start), dateInt(end))
		if loadErr != nil {
			return fmt.Errorf("点时基本面数据加载失败: %w", loadErr)
		}
		for index := range plans {
			if strategyUsesFundamentals(plans[index].config) {
				plans[index].fundamentalData = fundamentalData
				plans[index].rawKlines = rawKlines
			}
		}
	}
	dates := historicalTradingDates(klines, dateInt(start), dateInt(end))
	if len(dates) == 0 {
		return errors.New("所选日期区间没有可用交易日K线")
	}

	grid := defaultScanGrid()
	combinations := grid.combinations()
	totalWork := 0
	for _, plan := range plans {
		for _, scope := range scopes {
			if len(intersectHistoricalSymbols(plan.symbols, scope.Symbols)) > 0 {
				totalWork++
			}
		}
	}
	if totalWork == 0 {
		return errors.New("选股范围与所选策略的候选范围没有交集")
	}
	if err := r.store.UpdateHistoricalScanProgress(runID, 0, totalWork, "", 0); err != nil {
		return err
	}
	processed := 0
	signalCount := 0
	for _, plan := range plans {
		for _, scope := range scopes {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			effectivePlan := plan
			effectivePlan.symbols = intersectHistoricalSymbols(plan.symbols, scope.Symbols)
			if len(effectivePlan.symbols) == 0 {
				continue
			}
			signals, err := r.generateStrategySignals(ctx, effectivePlan, klines, dates)
			if err != nil {
				return fmt.Errorf("策略 %s / %s 信号生成失败: %w", plan.strategy.Name, scope.Name, err)
			}
			results := make([]workbench.HistoricalScanResult, 0, len(combinations))
			for _, set := range combinations {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				cfg := portfolioSimConfig{
					InitialCash: req.InitialCash, MaxPositions: req.MaxPositions, BuyCost: req.BuyCost, SellCost: req.SellCost,
					BuyBase: set.BuyBase, BuyOffset: set.BuyOffset, SellBase: set.SellBase,
					TakeProfit: set.TakeProfit, TrailDrawdown: set.TrailDrawdown, StopLoss: set.StopLoss, MaxHoldDays: set.MaxHoldDays,
				}
				summary := replayHistoricalPortfolio(klines, dates, signals, cfg)
				results = append(results, workbench.HistoricalScanResult{
					StrategyID: plan.strategy.ID, StrategyName: plan.strategy.Name,
					UniversePoolIDs: scope.PoolIDs, UniverseName: scope.Name, UniverseSymbolCount: len(effectivePlan.symbols),
					BuyBase: set.BuyBase, SellBase: set.SellBase, TakeProfit: set.TakeProfit,
					TrailDrawdown: set.TrailDrawdown, StopLoss: set.StopLoss, MaxHoldDays: set.MaxHoldDays,
					TotalReturn: scanSummaryFloat(summary, "total_return"),
					MaxDrawdown: scanSummaryFloat(summary, "max_drawdown"),
					WinRate:     scanSummaryFloat(summary, "win_rate"),
					ClosedCount: scanSummaryInt(summary, "closed_count"),
					OpenCount:   scanSummaryInt(summary, "open_count"),
					FinalEquity: scanSummaryFloat(summary, "final_equity"),
				})
			}
			if err := r.store.SaveHistoricalScanResults(runID, results); err != nil {
				return err
			}
			processed++
			signalCount += len(signals)
			if err := r.store.UpdateHistoricalScanProgress(runID, processed, totalWork, fmt.Sprintf("%s · %s", plan.strategy.Name, scope.Name), signalCount); err != nil {
				return err
			}
		}
	}
	return nil
}

func scanSummaryFloat(summary map[string]interface{}, key string) float64 {
	if value, ok := summary[key].(float64); ok {
		return value
	}
	return 0
}

func scanSummaryInt(summary map[string]interface{}, key string) int {
	if value, ok := summary[key].(int); ok {
		return value
	}
	return 0
}

const historicalScanRunLock = "historical-parameter-scan"

// startParameterScan 记录扫描任务并异步执行,页面可立即返回。
func (r *AutomationRunner) startParameterScan(req historicalBacktestRequest) (workbench.HistoricalScanRun, error) {
	normalized, _, _, err := normalizeHistoricalBacktestRequest(req)
	if err != nil {
		return workbench.HistoricalScanRun{}, err
	}
	strategies, err := r.historicalBacktestStrategies(normalized.StrategyIDs)
	if err != nil {
		return workbench.HistoricalScanRun{}, err
	}
	if _, err := r.historicalScanScopes(normalized.UniversePoolIDs); err != nil {
		return workbench.HistoricalScanRun{}, err
	}
	if !r.acquireRun(historicalScanRunLock) {
		return workbench.HistoricalScanRun{}, errors.New("已有参数扫描正在执行")
	}
	strategyIDs := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		strategyIDs = append(strategyIDs, strategy.ID)
	}
	run, err := r.store.CreateHistoricalScanRun(workbench.HistoricalScanRun{
		Status: "running", StartDate: normalized.StartDate, EndDate: normalized.EndDate,
		StrategyIDs: strategyIDs, UniversePoolIDs: normalized.UniversePoolIDs, TotalStrategies: len(strategies),
	})
	if err != nil {
		r.releaseRun(historicalScanRunLock)
		return run, err
	}
	go func() {
		defer r.releaseRun(historicalScanRunLock)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()
		runErr := r.runParameterScan(ctx, run.ID, normalized, strategies)
		status, errorText := "success", ""
		if runErr != nil {
			status, errorText = "failed", runErr.Error()
		}
		_ = r.store.FinishHistoricalScanRun(run.ID, status, errorText)
	}()
	return run, nil
}

// handleHistoricalScan POST /api/historical-backtests/scan 发起参数扫描。
func handleHistoricalScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req historicalBacktestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	run, err := automationRunner.startParameterScan(req)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, run)
}

// handleHistoricalScanRuns GET /api/historical-backtests/scan/runs 扫描任务列表。
func handleHistoricalScanRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := appStore.ListHistoricalScanRuns(limit)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, map[string]interface{}{"items": items})
}

// handleHistoricalScanResults GET /api/historical-backtests/scan/{id}/results 扫描结果。
func handleHistoricalScanResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	parts := pathParts(r.URL.Path, "/api/historical-backtests/scan/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	items, err := appStore.ListHistoricalScanResults(id, r.URL.Query().Get("strategy_id"))
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, map[string]interface{}{"items": items})
}
