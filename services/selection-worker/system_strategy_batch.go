package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

type systemStrategyBatchPlan struct {
	strategy Strategy
	config   StrategyConfig
	symbols  []string
}

// triggerSystemStrategyDailyBatch is invoked after a successful close sync.
// A same-day successful batch is retained as the daily result and not repeated.
func (r *AutomationRunner) triggerSystemStrategyDailyBatch() {
	task, err := r.store.GetAutomationTask(SystemStrategyDailyBatchTaskID)
	if err != nil || !task.Enabled {
		return
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	runs, err := r.store.ListAutomationRuns(SystemStrategyDailyBatchTaskID, 20)
	if err != nil {
		return
	}
	for _, existing := range runs {
		startedAt, parseErr := time.Parse(time.RFC3339, existing.StartedAt)
		if parseErr == nil && startedAt.In(time.Local).Format("2006-01-02") == today && (existing.Status == "running" || existing.Status == "success") {
			return
		}
	}
	_, _ = r.startSystemStrategyBatch()
}

// startSystemStrategyBatch records an immediately-running parent task and
// executes asynchronously so the automation page can return without waiting
// for the full-market scan.
func (r *AutomationRunner) startSystemStrategyBatch() (AutomationRun, error) {
	task, err := r.store.GetAutomationTask(SystemStrategyDailyBatchTaskID)
	if err != nil {
		return AutomationRun{}, err
	}
	if !r.acquireRun(task.ID) {
		return AutomationRun{}, errors.New("系统策略日报正在执行")
	}
	run, err := r.store.CreateAutomationRun(task)
	if err != nil {
		r.releaseRun(task.ID)
		return AutomationRun{}, err
	}
	go func() {
		defer r.releaseRun(task.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		result, matched, runErr := r.runSystemStrategyBatch(ctx, run.ID)
		status, logText := "success", ""
		if runErr != nil {
			status, logText = "failed", runErr.Error()
		}
		if err := r.store.FinishAutomationRun(run.ID, status, logText, mustJSON(result), matched); err == nil {
			message := logText
			if message == "" {
				message = fmt.Sprintf("完成，累计命中 %d 条", matched)
			}
			_ = r.store.UpdateTaskRunState(task.ID, status, message)
		}
	}()
	return run, nil
}

func (r *AutomationRunner) runSystemStrategyBatch(ctx context.Context, parentRunID string) (map[string]interface{}, int, error) {
	strategies, err := r.store.ListStrategies()
	if err != nil {
		return nil, 0, err
	}
	plans := make([]systemStrategyBatchPlan, 0, len(strategies))
	allSymbols := map[string]bool{}
	maxCalcCount := 260
	for _, strategy := range strategies {
		if !strategy.Readonly || !strategy.Enabled {
			continue
		}
		var config StrategyConfig
		if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
			return nil, 0, fmt.Errorf("系统策略 %s 配置解析失败: %w", strategy.Name, err)
		}
		normalizeSystemStrategyConfig(&config)
		if config.Period != "day" {
			return nil, 0, fmt.Errorf("系统策略 %s 使用了不支持的周期 %s", strategy.Name, config.Period)
		}
		symbols, err := r.strategyUniverse(config)
		if err != nil {
			return nil, 0, fmt.Errorf("系统策略 %s 候选范围失败: %w", strategy.Name, err)
		}
		if len(symbols) == 0 {
			continue
		}
		if config.CalcCount > maxCalcCount {
			maxCalcCount = config.CalcCount
		}
		for _, symbol := range symbols {
			allSymbols[symbol] = true
		}
		plans = append(plans, systemStrategyBatchPlan{strategy: strategy, config: config, symbols: symbols})
	}
	if len(plans) == 0 {
		return nil, 0, errors.New("没有可执行的系统策略")
	}
	symbols := make([]string, 0, len(allSymbols))
	for symbol := range allSymbols {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	loadSymbols := append([]string{}, symbols...)
	for _, plan := range plans {
		if strategyUsesMarketContext(plan.config) {
			loadSymbols = append(loadSymbols, strategyBenchmarkSymbol)
			break
		}
	}
	klines, loadErrors := r.loadSystemBatchKlines(ctx, loadSymbols, maxCalcCount)
	// 实盘盘中补K:仅实盘执行路径;历史回测(historical_backtest.go)独立加载K线,不经此处
	for symbol, reason := range r.patchIntradayKlines(ctx, klines, time.Now()) {
		loadErrors[symbol] = reason
	}
	summaries := make([]map[string]interface{}, 0, len(plans))
	totalMatched := 0
	failedStrategies := 0
	for _, plan := range plans {
		childTask := AutomationTask{
			ID:          "system-strategy:" + plan.strategy.ID,
			Name:        plan.strategy.Name,
			Type:        "strategy_selection",
			PayloadJSON: mustJSON(map[string]string{"strategy_id": plan.strategy.ID}),
			WebhookIDs:  "[]",
		}
		childRun, createErr := r.store.CreateAutomationRun(childTask, parentRunID)
		if createErr != nil {
			failedStrategies++
			summaries = append(summaries, map[string]interface{}{"strategy_id": plan.strategy.ID, "name": plan.strategy.Name, "status": "failed", "error": createErr.Error()})
			continue
		}
		result, executeErr := r.executeSystemStrategyWithKlines(ctx, plan, klines, loadErrors)
		matched := len(result.Items)
		status, logText := "success", ""
		if executeErr != nil {
			status, logText = "failed", executeErr.Error()
		} else if err := r.store.SaveSelectionResults(childRun, Formula{ID: "strategy:" + plan.strategy.ID, Name: plan.strategy.Name}, strategyItemsToSelectionResults(result.Items)); err != nil {
			status, logText = "failed", err.Error()
		}
		if err := r.store.FinishAutomationRun(childRun.ID, status, logText, mustJSON(result), matched); err != nil {
			return nil, totalMatched, err
		}
		if status == "success" {
			totalMatched += matched
		} else {
			failedStrategies++
		}
		summaries = append(summaries, map[string]interface{}{
			"strategy_id": plan.strategy.ID,
			"name":        plan.strategy.Name,
			"status":      status,
			"matched":     matched,
			"errors":      len(result.Errors),
			"error":       logText,
		})
	}
	result := map[string]interface{}{
		"strategies":        summaries,
		"strategy_count":    len(plans),
		"failed_strategies": failedStrategies,
		"candidate_symbols": len(symbols),
		"kline_loaded":      countLoadedStrategySymbols(symbols, klines),
		"kline_failed":      countFailedStrategySymbols(symbols, loadErrors),
		"calc_count":        maxCalcCount,
	}
	if failedStrategies > 0 {
		return result, totalMatched, fmt.Errorf("%d/%d 个系统策略执行失败", failedStrategies, len(plans))
	}
	return result, totalMatched, nil
}

func countLoadedStrategySymbols(symbols []string, klines map[string][]FormulaKline) int {
	count := 0
	for _, symbol := range symbols {
		if len(klines[symbol]) > 0 {
			count++
		}
	}
	return count
}

func countFailedStrategySymbols(symbols []string, failures map[string]string) int {
	count := 0
	for _, symbol := range symbols {
		if failures[symbol] != "" {
			count++
		}
	}
	return count
}

func normalizeSystemStrategyConfig(config *StrategyConfig) {
	if config.Period == "" {
		config.Period = "day"
	}
	if config.CalcCount <= 0 {
		config.CalcCount = 260
	}
	if config.CalcCount < strategyRankingMinimumBars {
		config.CalcCount = strategyRankingMinimumBars
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.BatchSize > 200 {
		config.BatchSize = 200
	}
}

func (r *AutomationRunner) loadSystemBatchKlines(ctx context.Context, symbols []string, calcCount int) (map[string][]FormulaKline, map[string]string) {
	return r.loadSystemBatchKlinesWithRecover(ctx, symbols, calcCount, "qfq")
}

func (r *AutomationRunner) loadSystemBatchKlinesWithRecover(ctx context.Context, symbols []string, calcCount int, recover string) (map[string][]FormulaKline, map[string]string) {
	loaded := make(map[string][]FormulaKline, len(symbols))
	failed := map[string]string{}
	if !useMarketService() {
		for _, symbol := range symbols {
			failed[symbol] = "历史K线必须通过行情服务读取"
		}
		return loaded, failed
	}
	type batchResult struct {
		symbols []string
		resp    *MarketKlineBatch
		err     error
	}
	batches := chunkSymbols(symbols, 128)
	results := make(chan batchResult, len(batches))
	semaphore := make(chan struct{}, strategyKlineBatchConcurrency())
	var workers sync.WaitGroup
	for _, batch := range batches {
		batch := batch
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- batchResult{symbols: batch, err: ctx.Err()}
				return
			}
			defer func() { <-semaphore }()
			resp, err := marketClient.KlineHistoryBatchWithRecover(ctx, batch, "day", calcCount, recover)
			results <- batchResult{symbols: batch, resp: resp, err: err}
		}()
	}
	go func() {
		workers.Wait()
		close(results)
	}()
	for result := range results {
		if result.err != nil {
			for _, symbol := range result.symbols {
				failed[symbol] = result.err.Error()
			}
			continue
		}
		if result.resp == nil {
			for _, symbol := range result.symbols {
				failed[symbol] = "行情服务未返回批量结果"
			}
			continue
		}
		for symbol, item := range result.resp.Data {
			if item == nil || len(item.List) == 0 {
				failed[symbol] = "K线为空"
				continue
			}
			loaded[symbol] = protocolKlinesToFormulaRows(item.List, calcCount)
		}
		for symbol, message := range result.resp.Errors {
			failed[symbol] = message
		}
	}
	for _, symbol := range symbols {
		if _, ok := loaded[symbol]; !ok {
			if _, exists := failed[symbol]; !exists {
				failed[symbol] = "行情服务未返回K线"
			}
		}
	}
	return loaded, failed
}

func strategyKlineBatchConcurrency() int {
	value, err := strconv.Atoi(os.Getenv("STRATEGY_KLINE_BATCH_CONCURRENCY"))
	if err != nil || value <= 0 {
		return 4
	}
	if value > 8 {
		return 8
	}
	return value
}

func (r *AutomationRunner) executeSystemStrategyWithKlines(ctx context.Context, plan systemStrategyBatchPlan, klines map[string][]FormulaKline, loadErrors map[string]string) (StrategyRunResult, error) {
	result := StrategyRunResult{
		Strategy:         plan.strategy,
		Config:           plan.config,
		Total:            len(plan.symbols),
		Errors:           map[string]string{},
		FormulaCache:     map[string]map[string]bool{},
		FormulaDetail:    map[string]map[string]any{},
		KlineCache:       klines,
		PoolCache:        map[string]map[string]bool{},
		FormulaByName:    map[string]Formula{},
		FormulaByID:      map[string]Formula{},
		RawKlineCache:    map[string][]FormulaKline{},
		FundamentalRanks: map[string]map[string]float64{},
	}
	available := make([]string, 0, len(plan.symbols))
	for _, symbol := range plan.symbols {
		if _, ok := klines[symbol]; ok {
			available = append(available, symbol)
			continue
		}
		result.Errors[symbol] = loadErrors[symbol]
	}
	if len(available) == 0 {
		return result, errors.New("候选股票K线均未加载成功")
	}
	if err := r.prepareStrategyFundamentalContext(ctx, &result, available, 0, 0); err != nil {
		return result, err
	}
	if !r.strategyMarketContextPasses(&result) {
		result.Errors = nil
		result.Items = []StrategySelectionItem{}
		return result, nil
	}
	if err := r.prepareStrategyFormulasWithKlines(ctx, &result, available, klines); err != nil {
		return result, err
	}
	items := make([]StrategySelectionItem, 0)
	for _, symbol := range available {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		item, ok := r.evaluateStrategySymbol(ctx, plan.strategy, &result, symbol)
		if ok {
			items = append(items, item)
		}
	}
	items = rankStrategyMatches(items, result.Config.Pass.TopN)
	result.Items = items
	result.Matched = len(items)
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result, nil
}
