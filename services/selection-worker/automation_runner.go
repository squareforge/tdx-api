package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend"
	"github.com/injoyai/tdx/protocol"
	"github.com/robfig/cron/v3"
	workbench "workbench-core"
)

type AutomationRunner struct {
	store   *AppStore
	worker  *FormulaWorkerClient
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[string]cron.EntryID
	runMu   sync.Mutex
	running map[string]bool
}

type StockSelectionPayload struct {
	FormulaID       string   `json:"formula_id"`
	PoolID          string   `json:"pool_id"`
	Symbols         []string `json:"symbols"`
	Period          string   `json:"period"`
	Right           int      `json:"right"`
	OutCount        int      `json:"out_count"`
	CalcCount       int      `json:"calc_count"`
	BatchSize       int      `json:"batch_size"`
	ContinueOnError bool     `json:"continue_on_error"`
}

type SelectionTrackingPayload struct {
	HistoryBuffer   int     `json:"history_buffer"`
	Horizons        []int   `json:"horizons"`
	TargetReturn    float64 `json:"target_return"`
	DrawdownLimit   float64 `json:"drawdown_limit"`
	ContinueOnError bool    `json:"continue_on_error"`
}

type SelectionTrackingRunResult struct {
	Summary        SelectionTrackingSummary `json:"summary"`
	Policy         map[string]float64       `json:"policy"`
	Errors         map[string]string        `json:"errors"`
	SymbolErrors   map[string]string        `json:"symbol_errors"`
	Freshness      *MarketHikyuuFreshness   `json:"freshness,omitempty"`
	CandidateCount int                      `json:"candidate_count"`
	SymbolCount    int                      `json:"symbol_count"`
	UpdatedCount   int                      `json:"updated_count"`
	FailedCount    int                      `json:"failed_count"`
	SkippedSymbols int                      `json:"skipped_symbols"`
	HistoryStart   string                   `json:"history_start,omitempty"`
	ExpectedDate   string                   `json:"expected_date,omitempty"`
	KlineCount     int                      `json:"kline_count,omitempty"`
}

const (
	defaultSelectionTrackingHistoryBuffer = 30
	maxSelectionTrackingKlineCount        = 2000
	selectionTrackingLoadAttempts         = 3
)

type SystemSyncPayload struct {
	Scope           string   `json:"scope"`
	Date            string   `json:"date"`
	Codes           []string `json:"codes"`
	Tables          []string `json:"tables"`
	Kinds           []string `json:"kinds"`
	BlockFiles      []string `json:"block_files"`
	Limit           int      `json:"limit"`
	Days            int      `json:"days"`
	MaxCodes        int      `json:"max_codes"`
	StartDate       string   `json:"start_date"`
	WithIndex       bool     `json:"with_index"`
	IncludeF10      bool     `json:"include_f10"`
	F10Length       uint32   `json:"f10_length"`
	ContinueOnError bool     `json:"continue_on_error"`
}

type CustomTaskPayload struct {
	Action  string                 `json:"action"`
	Method  string                 `json:"method"`
	URL     string                 `json:"url"`
	Headers map[string]string      `json:"headers"`
	Body    interface{}            `json:"body"`
	Sync    SystemSyncPayload      `json:"sync"`
	Data    map[string]interface{} `json:"data"`
}

func NewAutomationRunner(store *AppStore, worker *FormulaWorkerClient) *AutomationRunner {
	return &AutomationRunner{
		store:   store,
		worker:  worker,
		cron:    cron.New(cron.WithSeconds()),
		entries: map[string]cron.EntryID{},
		running: map[string]bool{},
	}
}

// acquireRun prevents the same task from running concurrently when a manual
// trigger overlaps a cron tick, which would otherwise double-compute and
// double-write selection results.
func (r *AutomationRunner) acquireRun(taskID string) bool {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	if r.running[taskID] {
		return false
	}
	r.running[taskID] = true
	return true
}

func (r *AutomationRunner) releaseRun(taskID string) {
	r.runMu.Lock()
	delete(r.running, taskID)
	r.runMu.Unlock()
}

// taskLockKey serializes executions that would otherwise race on the same
// underlying work. A strategy-selection task is keyed by its strategy_id, so a
// manual run and a cron run of the same strategy cannot overlap and drag each
// other past the 20-minute timeout; every other task type keys on its own id.
func (r *AutomationRunner) taskLockKey(task AutomationTask) string {
	if task.Type != "strategy_selection" {
		return task.ID
	}
	var payload struct {
		StrategyID string `json:"strategy_id"`
	}
	if json.Unmarshal([]byte(task.PayloadJSON), &payload) == nil && strings.TrimSpace(payload.StrategyID) != "" {
		return "strategy:" + payload.StrategyID
	}
	return task.ID
}

func (r *AutomationRunner) Start() error {
	r.cron.Start()
	if err := r.Reload(); err != nil {
		ctx := r.cron.Stop()
		<-ctx.Done()
		return err
	}
	return nil
}

func (r *AutomationRunner) Stop() {
	if r == nil || r.cron == nil {
		return
	}
	ctx := r.cron.Stop()
	<-ctx.Done()
}

func (r *AutomationRunner) Reload() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for taskID, entryID := range r.entries {
		r.cron.Remove(entryID)
		delete(r.entries, taskID)
	}

	tasks, err := r.store.ListAutomationTasks()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if !task.Enabled || task.Type == "system_strategy_batch" {
			continue
		}
		entryID, err := r.cron.AddFunc(task.Cron, func(taskID string) func() {
			return func() {
				timeout := 20 * time.Minute
				if taskID == FixedCloseSyncTaskID {
					timeout = 2 * time.Hour
				}
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				if err := r.runScheduledTask(ctx, taskID); err != nil {
					log.Printf("自动化任务执行失败: %s %v", taskID, err)
				}
			}
		}(task.ID))
		if err != nil {
			return fmt.Errorf("%s cron配置错误: %w", task.Name, err)
		}
		r.entries[task.ID] = entryID
		next := r.cron.Entry(entryID).Next
		if !next.IsZero() {
			_ = r.store.UpdateTaskNextRun(task.ID, next.Format(time.RFC3339))
		}
	}
	return nil
}

// runScheduledTask applies the task's OnlyTradeDay setting before dispatching
// a cron-triggered run. Manual RunTask calls intentionally bypass this gate.
func (r *AutomationRunner) runScheduledTask(ctx context.Context, taskID string) error {
	task, err := r.store.GetAutomationTask(taskID)
	if err != nil {
		return err
	}
	if taskID == FixedSelectionTrackingTaskID {
		return r.triggerSelectionTrackingOncePerDay()
	}
	allowed, err := r.gateScheduledTask(ctx, task)
	if err != nil || !allowed {
		return err
	}
	_, err = r.runTask(ctx, task)
	return err
}

func (r *AutomationRunner) gateScheduledTask(ctx context.Context, task AutomationTask) (bool, error) {
	if !task.OnlyTradeDay {
		return true, nil
	}

	today := time.Now().In(time.Local).Format("2006-01-02")
	isWorkday, err := r.scheduledTradeDay(ctx, today)
	if err != nil {
		gateErr := fmt.Errorf("交易日判断失败: %w", err)
		if recordErr := r.recordScheduledGateRun(task, "failed", gateErr.Error(), map[string]interface{}{
			"skipped": false,
			"reason":  "trade_day_check_failed",
			"date":    today,
		}); recordErr != nil {
			return false, fmt.Errorf("%w；记录任务状态失败: %v", gateErr, recordErr)
		}
		return false, gateErr
	}
	if isWorkday {
		return true, nil
	}
	if err := r.recordScheduledGateRun(task, "skipped", "非交易日，已跳过", map[string]interface{}{
		"skipped": true,
		"reason":  "non_trade_day",
		"date":    today,
	}); err != nil {
		return false, err
	}
	return false, nil
}

func (r *AutomationRunner) scheduledTradeDay(ctx context.Context, date string) (bool, error) {
	if useMarketService() {
		return marketClient.IsWorkday(ctx, date)
	}
	if manager != nil && manager.Workday != nil {
		parsed, err := time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return false, err
		}
		return manager.Workday.Is(parsed), nil
	}
	return false, errors.New("交易日服务未配置")
}

func (r *AutomationRunner) recordScheduledGateRun(task AutomationTask, status, message string, result interface{}) error {
	run, err := r.store.CreateAutomationRun(task)
	if err != nil {
		return err
	}
	if err := r.store.FinishAutomationRun(run.ID, status, message, mustJSON(result), 0); err != nil {
		return err
	}
	return r.store.UpdateTaskRunState(task.ID, status, message)
}

func (r *AutomationRunner) RunTask(ctx context.Context, taskID string) (AutomationRun, error) {
	task, err := r.store.GetAutomationTask(taskID)
	if err != nil {
		return AutomationRun{}, err
	}
	return r.runTask(ctx, task)
}

func (r *AutomationRunner) runTask(ctx context.Context, task AutomationTask) (AutomationRun, error) {
	lockKey := r.taskLockKey(task)
	if !r.acquireRun(lockKey) {
		return AutomationRun{}, fmt.Errorf("任务已在运行中，跳过重复触发: %s", task.Name)
	}
	defer r.releaseRun(lockKey)

	run, err := r.store.CreateAutomationRun(task)
	if err != nil {
		return run, err
	}

	status := "success"
	logText := ""
	resultJSON := "{}"
	matchedCount := 0
	var result interface{}
	var matchedSymbols []string

	switch task.Type {
	case "system_strategy_batch":
		return AutomationRun{}, errors.New("系统策略日报必须通过异步入口运行")
	case "stock_selection":
		result, matchedSymbols, err = r.runStockSelection(ctx, task, run)
		matchedCount = len(matchedSymbols)
	case "strategy_selection":
		result, matchedSymbols, err = r.runStrategySelection(ctx, task, run)
		matchedCount = len(matchedSymbols)
	case "selection_tracking":
		result, matchedCount, err = r.runSelectionTracking(ctx, task)
	case "system_sync":
		result, matchedCount, err = r.runSystemSync(ctx, task)
	case "custom":
		result, matchedCount, err = r.runCustomTask(ctx, task)
	default:
		err = fmt.Errorf("暂不支持的任务类型: %s", task.Type)
	}

	if err != nil {
		status = "failed"
		logText = err.Error()
	} else {
		resultJSON = mustJSON(result)
	}

	if finishErr := r.store.FinishAutomationRun(run.ID, status, logText, resultJSON, matchedCount); finishErr != nil {
		log.Printf("更新运行记录失败: run=%s %v", run.ID, finishErr)
		if err == nil {
			err = finishErr
		}
	}
	message := logText
	if message == "" {
		if task.Type == "selection_tracking" {
			message = selectionTrackingRunMessage(result, matchedCount)
		} else {
			message = fmt.Sprintf("完成，命中 %d 条", matchedCount)
		}
	}
	_ = r.store.UpdateTaskRunState(task.ID, status, message)

	hooks := r.resolveTaskWebhooks(task)
	eventName := "automation.finished"
	if task.Type == "stock_selection" && status == "success" {
		eventName = "stock_selection.finished"
	}
	if task.Type == "strategy_selection" && status == "success" {
		eventName = "strategy_selection.finished"
	}
	if status == "failed" {
		eventName = "automation.failed"
	}
	hookLogs := sendWebhooks(ctx, hooks, WebhookEvent{
		Event:          eventName,
		TaskID:         task.ID,
		TaskName:       task.Name,
		TaskType:       task.Type,
		RunID:          run.ID,
		Status:         status,
		Message:        message,
		MatchedCount:   matchedCount,
		Result:         result,
		MatchedSymbols: matchedSymbols,
	})
	if len(hookLogs) > 0 {
		log.Printf("Webhook通知: %s", strings.Join(hookLogs, "; "))
	}
	// The daily batch must see the completed close-sync state before it starts.
	// Run it independently so the close-sync run remains an accurate record of
	// data synchronization time rather than including strategy evaluation.
	if task.ID == FixedCloseSyncTaskID && status == "success" && err == nil && !automationResultSkipped(result) {
		go func() {
			if trackingErr := r.triggerSelectionTrackingOncePerDay(); trackingErr != nil {
				log.Printf("同步完成后启动选股跟踪失败: %v", trackingErr)
			}
		}()
	}
	if task.ID == FixedSelectionTrackingTaskID && status == "success" && err == nil {
		go r.triggerSystemStrategyDailyBatch()
	}

	if latest, latestErr := r.store.GetAutomationRun(run.ID); latestErr == nil {
		run = latest
	}
	return run, err
}

// triggerSelectionTrackingOncePerDay lets the 17:15 schedule remain as a
// fallback while also running tracking immediately after a delayed close sync.
// A failed run is intentionally retried once fresh Hikyuu data is available.
func (r *AutomationRunner) triggerSelectionTrackingOncePerDay() error {
	task, err := r.store.GetAutomationTask(FixedSelectionTrackingTaskID)
	if err != nil || !task.Enabled {
		if err != nil {
			return err
		}
		return errors.New("验证选股表现任务未启用，已阻止系统策略日报")
	}
	allowed, err := r.gateScheduledTask(context.Background(), task)
	if err != nil || !allowed {
		return err
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	runs, err := r.store.ListAutomationRuns(FixedSelectionTrackingTaskID, 20)
	if err != nil {
		log.Printf("检查选股跟踪运行记录失败: %v", err)
		return err
	}
	for _, existing := range runs {
		startedAt, parseErr := time.Parse(time.RFC3339, existing.StartedAt)
		if parseErr == nil && startedAt.In(time.Local).Format("2006-01-02") == today {
			switch existing.Status {
			case "running":
				return nil
			case "success":
				go r.triggerSystemStrategyDailyBatch()
				return nil
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if _, err := r.RunTask(ctx, FixedSelectionTrackingTaskID); err != nil {
		log.Printf("选股跟踪任务执行失败: %v", err)
		return err
	}
	return nil
}

func (r *AutomationRunner) requireHikyuuFreshness(ctx context.Context) (*MarketHikyuuFreshness, string, error) {
	if !useMarketService() {
		return nil, "", errors.New("系统策略日报前无法校验Hikyuu K线完整性：MARKET_SERVICE_URL 未配置")
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	expectedDate, err := marketClient.ExpectedTradingDate(ctx, today)
	if err != nil {
		return nil, "", fmt.Errorf("系统策略日报前确定最新交易日失败: %w", err)
	}
	freshness, err := marketClient.HikyuuFreshness(ctx, expectedDate)
	if err != nil {
		return nil, expectedDate, fmt.Errorf("系统策略日报前校验Hikyuu K线完整性失败: %w", err)
	}
	if freshness == nil || !freshness.OK {
		return freshness, expectedDate, errors.New(hikyuuFreshnessError(freshness))
	}
	return freshness, expectedDate, nil
}

func (r *AutomationRunner) runSelectionTracking(ctx context.Context, task AutomationTask) (interface{}, int, error) {
	payload := SelectionTrackingPayload{HistoryBuffer: defaultSelectionTrackingHistoryBuffer, Horizons: []int{1, 3, 5, 10}, ContinueOnError: true}
	if strings.TrimSpace(task.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
			return nil, 0, err
		}
	}
	if len(payload.Horizons) == 0 {
		payload.Horizons = []int{1, 3, 5, 10}
	}
	if payload.HistoryBuffer <= 0 || payload.HistoryBuffer > defaultSelectionTrackingHistoryBuffer {
		payload.HistoryBuffer = defaultSelectionTrackingHistoryBuffer
	}
	targetReturn, drawdownLimit := workbench.DefaultTrackingPolicy(payload.TargetReturn, payload.DrawdownLimit)
	items, err := r.store.ListIncompleteSelectionTrackingResults(payload.Horizons)
	if err != nil {
		return nil, 0, err
	}
	result := SelectionTrackingRunResult{
		Summary:        workbench.SummarizeSelectionTracking(nil, payload.Horizons),
		Policy:         map[string]float64{"target_return": targetReturn, "drawdown_limit": drawdownLimit},
		Errors:         map[string]string{},
		SymbolErrors:   map[string]string{},
		CandidateCount: len(items),
	}
	if len(items) == 0 {
		return result, 0, nil
	}
	freshness, expectedDate, err := r.requireHikyuuFreshness(ctx)
	if err != nil {
		return map[string]interface{}{"freshness": freshness}, 0, err
	}
	result.Freshness = freshness
	result.ExpectedDate = expectedDate

	itemsBySymbol := map[string][]SelectionResult{}
	signalDates := map[string]int{}
	invalidDateItems := make([]SelectionResult, 0)
	oldestDate := ""
	for _, item := range items {
		signalDate, signalDateInt, ok := selectionTrackingSignalDateForItem(item)
		if !ok {
			invalidDateItems = append(invalidDateItems, item)
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		if symbol == "" {
			result.Errors[item.ID] = "股票代码为空"
			continue
		}
		item.Symbol = symbol
		itemsBySymbol[symbol] = append(itemsBySymbol[symbol], item)
		if current, exists := signalDates[symbol]; !exists || signalDateInt < current {
			signalDates[symbol] = signalDateInt
		}
		if oldestDate == "" || signalDate < oldestDate {
			oldestDate = signalDate
		}
	}

	tracked := make([]SelectionTrackingItem, 0, len(items))
	now := time.Now()
	for _, item := range invalidDateItems {
		tracking := workbench.EvaluateSelectionTracking(item, nil, payload.Horizons, targetReturn, drawdownLimit, now)
		if updateErr := r.saveSelectionTracking(item, tracking); updateErr != nil {
			result.Errors[item.ID] = updateErr.Error()
			continue
		}
		item.Tracking = &tracking
		tracked = append(tracked, SelectionTrackingItem{Result: item, Tracking: tracking})
	}

	symbols := make([]string, 0, len(itemsBySymbol))
	for symbol := range itemsBySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	result.SymbolCount = len(symbols)
	if len(symbols) > 0 {
		// A tracking retry can resolve an intraday signal while its quote window
		// is still open. Later runs only expire the unfilled execution.
		nextQuoteItems := r.captureNextQuoteItems(ctx, items)
		historyStart, dateErr := marketClient.PreviousTradingDate(ctx, oldestDate, payload.HistoryBuffer)
		if dateErr != nil {
			return result, len(tracked), fmt.Errorf("确定选股跟踪历史起点失败: %w", dateErr)
		}
		klineCount, countErr := marketClient.TradingDayCount(ctx, historyStart, expectedDate)
		if countErr != nil {
			return result, len(tracked), fmt.Errorf("计算选股跟踪K线长度失败: %w", countErr)
		}
		if klineCount > maxSelectionTrackingKlineCount {
			klineCount = maxSelectionTrackingKlineCount
		}
		result.HistoryStart = historyStart
		result.KlineCount = klineCount

		benchmarkSymbol := "sh000001"
		trackingSymbols := append(append([]string{}, symbols...), benchmarkSymbol)
		klines, loadErrors := r.loadTrackingKlinesWithRetry(ctx, trackingSymbols, klineCount)
		benchmarkBars := toTrackingBars(klines[benchmarkSymbol])
		for _, symbol := range symbols {
			symbolItems := itemsBySymbol[symbol]
			for index := range symbolItems {
				if captured, ok := nextQuoteItems[symbolItems[index].ID]; ok {
					symbolItems[index] = captured
				}
			}
			rows, loaded := klines[symbol]
			if loaded && !trackingRowsCoverDate(rows, signalDates[symbol]) && len(rows) >= klineCount {
				rows, loadErrors[symbol] = r.expandTrackingKlineCoverage(ctx, symbol, signalDates[symbol], rows, klineCount)
				if loadErrors[symbol] == "" {
					klines[symbol] = rows
				}
			}
			if reason := strings.TrimSpace(loadErrors[symbol]); reason != "" {
				result.SymbolErrors[symbol] = reason
				for _, item := range itemsBySymbol[symbol] {
					result.Errors[item.ID] = reason
				}
				continue
			}
			trackingBars := toTrackingBars(klines[symbol])
			if executionErr := r.store.EnsureSelectionSignalExecutions(symbolItems, map[string][]workbench.TrackingBar{symbol: trackingBars}); executionErr != nil {
				for _, item := range symbolItems {
					result.Errors[item.ID] = executionErr.Error()
				}
				continue
			}
			for _, item := range symbolItems {
				tracking := workbench.EvaluateSelectionTracking(item, trackingBars, payload.Horizons, targetReturn, drawdownLimit, now)
				if len(benchmarkBars) > 0 {
					applyBenchmarkToTracking(&tracking, item, trackingBars, benchmarkBars)
				}
				if observationErr := r.saveSelectionObservations(item, trackingBars, benchmarkBars, now); observationErr != nil {
					result.Errors[item.ID] = observationErr.Error()
					continue
				}
				if updateErr := r.saveSelectionTracking(item, tracking); updateErr != nil {
					result.Errors[item.ID] = updateErr.Error()
					continue
				}
				item.Tracking = &tracking
				tracked = append(tracked, SelectionTrackingItem{Result: item, Tracking: tracking})
			}
		}
	}

	result.Summary = workbench.SummarizeSelectionTracking(tracked, payload.Horizons)
	result.UpdatedCount = len(tracked)
	result.FailedCount = len(result.Errors)
	result.SkippedSymbols = len(result.SymbolErrors)
	return result, len(tracked), nil
}

func (r *AutomationRunner) captureNextQuoteItems(ctx context.Context, items []SelectionResult) map[string]SelectionResult {
	return r.captureNextQuoteItemsAt(ctx, items, time.Now())
}

func (r *AutomationRunner) captureNextQuoteItemsAt(ctx context.Context, items []SelectionResult, now time.Time) map[string]SelectionResult {
	updated := map[string]SelectionResult{}
	if marketClient == nil || !marketClient.Enabled() {
		return updated
	}
	codes := make([]string, 0, len(items))
	seen := map[string]bool{}
	eligible := make([]SelectionResult, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Symbol) == "" {
			continue
		}
		signalAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.SignalAt))
		if err != nil || !workbench.PaperTradingSession(signalAt) || now.Before(signalAt) || now.Sub(signalAt) > workbench.NextQuoteCaptureWindow {
			continue
		}
		eligible = append(eligible, item)
		code := strings.TrimSpace(item.Symbol)
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	for _, batch := range chunkSymbols(codes, intradayQuoteBatchSize) {
		quotes, err := marketClient.Quotes(ctx, batch)
		if err != nil {
			log.Printf("选股下一条行情获取失败(%d只): %v", len(batch), err)
			continue
		}
		byCode := map[string]*protocol.Quote{}
		for _, quote := range quotes {
			if quote == nil || quote.Kline == nil ||
				(quote.SellLevel[0].Price.Float64() <= 0 || quote.SellLevel[0].Number <= 0) &&
					(quote.Kline.Close.Float64() <= 0 || quote.Intuition <= 0) {
				continue
			}
			key := strategyNormalizeSymbol(quote.Code)
			if key != "" {
				byCode[key] = quote
			}
		}
		for _, item := range eligible {
			quote := byCode[strategyNormalizeSymbol(item.Symbol)]
			if quote == nil {
				continue
			}
			signalAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.SignalAt))
			quoteAt, valid := executionQuoteMarketTime(quote.ServerTime, now)
			if err != nil || !valid || !workbench.ValidNextQuoteWindow(signalAt, quoteAt) {
				continue
			}
			captured := item
			captured.NextQuotePrice = quote.SellLevel[0].Price.Float64()
			captured.NextQuoteSource = "market-service.quote.ask1"
			if captured.NextQuotePrice <= 0 || quote.SellLevel[0].Number <= 0 {
				captured.NextQuotePrice = quote.Kline.Close.Float64()
				captured.NextQuoteSource = "market-service.quote.last"
			}
			captured.NextQuoteAt = quoteAt.Format(time.RFC3339)
			updated[captured.ID] = captured
		}
	}
	return updated
}

func executionQuoteMarketTime(raw string, receivedAt time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	var quoteAt time.Time
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		quoteAt = parsed
	} else {
		for _, digit := range raw {
			if digit < '0' || digit > '9' {
				return time.Time{}, false
			}
		}
		clock := raw
		if len(clock) == 5 {
			clock = "0" + clock
		}
		if len(clock) == 8 || len(clock) == 9 {
			clock = clock[:6]
		}
		parsed, err := time.Parse("150405", clock)
		if err != nil || len(clock) != 6 {
			return time.Time{}, false
		}
		local := receivedAt.In(time.FixedZone("CST", 8*60*60))
		quoteAt = time.Date(local.Year(), local.Month(), local.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, local.Location())
	}
	age := receivedAt.Sub(quoteAt)
	return quoteAt, age >= -2*time.Second && age <= 30*time.Second
}

func (r *AutomationRunner) resolveNextQuoteForRun(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), workbench.NextQuoteCaptureWindow+5*time.Second)
	defer cancel()
	items, err := r.store.ListSelectionResultsByRunIDs([]string{runID}, 20000)
	if err != nil {
		log.Printf("读取即时模拟成交信号失败(%s): %v", runID, err)
		return
	}
	active := make(map[string]SelectionResult, len(items))
	now := time.Now()
	for _, item := range items {
		signalAt, err := time.Parse(time.RFC3339, item.SignalAt)
		if err == nil && workbench.PaperTradingSession(signalAt) && workbench.PaperTradingSession(now) &&
			!now.Before(signalAt) && now.Sub(signalAt) < workbench.NextQuoteCaptureWindow {
			active[item.ID] = item
		}
	}
	if len(active) == 0 {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for len(active) > 0 {
		pending := make([]SelectionResult, 0, len(active))
		for _, item := range active {
			pending = append(pending, item)
		}
		captured := r.captureNextQuoteItems(ctx, pending)
		if len(captured) > 0 {
			filled := make([]SelectionResult, 0, len(captured))
			for _, item := range captured {
				filled = append(filled, item)
			}
			if err := r.store.EnsureSelectionSignalExecutions(filled, nil); err != nil {
				log.Printf("保存即时模拟成交失败(%s): %v", runID, err)
			} else {
				for id := range captured {
					delete(active, id)
				}
			}
		}
		now = time.Now()
		for id, item := range active {
			signalAt, _ := time.Parse(time.RFC3339, item.SignalAt)
			if !workbench.PaperTradingSession(now) || now.Sub(signalAt) >= workbench.NextQuoteCaptureWindow {
				delete(active, id)
			}
		}
		if len(active) == 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			active = nil
		}
	}
	if err := r.store.EnsureSelectionSignalExecutions(items, nil); err != nil {
		log.Printf("更新即时模拟成交终态失败(%s): %v", runID, err)
	}
}

func (r *AutomationRunner) saveSelectionObservations(item SelectionResult, bars, benchmarkBars []workbench.TrackingBar, now time.Time) error {
	executions, err := r.store.ListSelectionSignalExecutions(item.ID, "")
	if err != nil {
		return err
	}
	rows := workbench.BuildSelectionObservations(item, workbench.SelectionSignalExecution{}, bars, now, benchmarkBars)
	workbench.ApplyBenchmarkToObservations(item, rows, benchmarkBars, "sh000001")
	for _, execution := range executions {
		if execution.Status != "filled" {
			continue
		}
		tradeRows := workbench.BuildSelectionObservations(item, execution, bars, now, benchmarkBars)
		workbench.ApplyBenchmarkToObservations(item, tradeRows, benchmarkBars, "sh000001")
		rows = append(rows, tradeRows...)
	}
	return r.store.SaveSelectionSignalObservations(rows)
}

func applyBenchmarkToTracking(tracking *workbench.SelectionTracking, item SelectionResult, bars, benchmarkBars []workbench.TrackingBar) {
	if tracking == nil || len(benchmarkBars) == 0 {
		return
	}
	observations := workbench.BuildSelectionObservations(item, workbench.SelectionSignalExecution{}, bars, time.Now())
	workbench.ApplyBenchmarkToObservations(item, observations, benchmarkBars, "sh000001")
	for key, horizon := range tracking.Horizons {
		if horizon.Status != "complete" || horizon.HorizonDays <= 0 {
			continue
		}
		var point *workbench.SelectionSignalObservation
		for index := range observations {
			if observations[index].SymbolDayIndex == horizon.HorizonDays {
				point = &observations[index]
				break
			}
		}
		if point == nil {
			continue
		}
		horizon.BenchmarkCloseReturn = point.BenchmarkCloseReturn
		horizon.ExcessCloseReturn = point.ExcessCloseReturn
		tracking.Horizons[key] = horizon
	}
}

func selectionTrackingRunMessage(value interface{}, updated int) string {
	result, ok := value.(SelectionTrackingRunResult)
	if !ok {
		return fmt.Sprintf("完成，更新 %d 条", updated)
	}
	if result.FailedCount > 0 {
		return fmt.Sprintf("完成，更新 %d/%d 条，跳过 %d 只股票、%d 条记录", result.UpdatedCount, result.CandidateCount, result.SkippedSymbols, result.FailedCount)
	}
	return fmt.Sprintf("完成，更新 %d/%d 条", result.UpdatedCount, result.CandidateCount)
}

func selectionTrackingSignalDate(raw string) (string, int, bool) {
	value := strings.TrimSpace(raw)
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		local := parsed.In(time.FixedZone("CST", 8*60*60))
		return local.Format("2006-01-02"), dateInt(local), true
	}
	if len(value) >= 10 {
		if parsed, err := time.ParseInLocation("2006-01-02", value[:10], time.Local); err == nil {
			return parsed.Format("2006-01-02"), dateInt(parsed), true
		}
	}
	return "", 0, false
}

func selectionTrackingSignalDateForItem(item SelectionResult) (string, int, bool) {
	raw := strings.TrimSpace(item.SignalAt)
	if raw != "" {
		if result, numeric, ok := selectionTrackingSignalDate(raw); ok {
			return result, numeric, true
		}
	}
	return selectionTrackingSignalDate(item.CreatedAt)
}

func (r *AutomationRunner) saveSelectionTracking(item SelectionResult, tracking SelectionTracking) error {
	return r.store.UpdateSelectionTracking(item.ID, tracking)
}

func (r *AutomationRunner) loadTrackingKlinesWithRetry(ctx context.Context, symbols []string, klineCount int) (map[string][]FormulaKline, map[string]string) {
	loaded := make(map[string][]FormulaKline, len(symbols))
	failures := map[string]string{}
	pending := append([]string(nil), symbols...)
	for attempt := 0; attempt < selectionTrackingLoadAttempts && len(pending) > 0; attempt++ {
		batchLoaded, batchFailures := r.loadSystemBatchKlines(ctx, pending, klineCount)
		for symbol, rows := range batchLoaded {
			loaded[symbol] = rows
			delete(failures, symbol)
		}
		next := make([]string, 0, len(batchFailures))
		for _, symbol := range pending {
			if _, ok := loaded[symbol]; ok {
				continue
			}
			reason := strings.TrimSpace(batchFailures[symbol])
			if reason == "" {
				reason = "K线加载失败"
			}
			failures[symbol] = reason
			next = append(next, symbol)
		}
		pending = next
		if err := ctx.Err(); err != nil {
			break
		}
	}
	return loaded, failures
}

func (r *AutomationRunner) expandTrackingKlineCoverage(ctx context.Context, symbol string, signalDate int, rows []FormulaKline, currentCount int) ([]FormulaKline, string) {
	for currentCount < maxSelectionTrackingKlineCount && !trackingRowsCoverDate(rows, signalDate) && len(rows) >= currentCount {
		nextCount := currentCount * 2
		if nextCount > maxSelectionTrackingKlineCount {
			nextCount = maxSelectionTrackingKlineCount
		}
		loaded, failures := r.loadTrackingKlinesWithRetry(ctx, []string{symbol}, nextCount)
		if reason := strings.TrimSpace(failures[symbol]); reason != "" {
			return rows, reason
		}
		expanded, ok := loaded[symbol]
		if !ok || len(expanded) == 0 {
			return rows, "扩展K线范围后仍无数据"
		}
		rows = expanded
		currentCount = nextCount
	}
	if !trackingRowsCoverDate(rows, signalDate) && len(rows) >= currentCount {
		return rows, fmt.Sprintf("最近%d根K线仍未覆盖信号日期%d", currentCount, signalDate)
	}
	return rows, ""
}

func trackingRowsCoverDate(rows []FormulaKline, signalDate int) bool {
	for _, row := range rows {
		if row.Date > 0 && row.Date <= signalDate {
			return true
		}
	}
	return false
}

func hikyuuFreshnessError(freshness *MarketHikyuuFreshness) string {
	if freshness == nil {
		return "Hikyuu K线完整性校验无结果"
	}
	details := make([]string, 0, len(freshness.Markets))
	for _, market := range []string{"SH", "SZ", "BJ"} {
		item, ok := freshness.Markets[market]
		if ok && !item.OK {
			details = append(details, fmt.Sprintf("%s latest=%s coverage=%.1f%%", market, item.LatestDate, item.Coverage*100))
		}
	}
	if len(details) == 0 {
		details = append(details, "市场状态未知")
	}
	return fmt.Sprintf("Hikyuu K线完整性校验未通过（目标交易日 %s）：%s", freshness.ExpectedDate, strings.Join(details, ", "))
}

func (r *AutomationRunner) resolveTaskWebhooks(task AutomationTask) []Webhook {
	hooks, err := r.store.ResolveWebhooks(task.WebhookIDs)
	if err == nil && len(hooks) > 0 {
		return hooks
	}
	hooks, err = r.store.ListEnabledWebhooks()
	if err == nil {
		return hooks
	}
	return nil
}

func (r *AutomationRunner) runStockSelection(ctx context.Context, task AutomationTask, run AutomationRun) (interface{}, []string, error) {
	var payload StockSelectionPayload
	if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
		return nil, nil, err
	}
	if payload.FormulaID == "" {
		return nil, nil, errors.New("选股任务缺少formula_id")
	}
	formula, err := r.store.GetFormula(payload.FormulaID)
	if err != nil {
		return nil, nil, err
	}

	symbols := payload.Symbols
	if len(symbols) == 0 && payload.PoolID != "" {
		pool, err := r.store.GetStockPool(payload.PoolID)
		if err != nil {
			return nil, nil, err
		}
		symbols = pool.Symbols
	}
	symbols = normalizeSymbols(symbols)
	if len(symbols) == 0 {
		return nil, nil, errors.New("选股任务股票池为空")
	}
	period := payload.Period
	if period == "" {
		period = formula.Period
	}
	right := payload.Right
	if right == 0 {
		right = formula.Right
	}
	outCount := payload.OutCount
	if outCount == 0 {
		outCount = 1
	}
	calcCount := payload.CalcCount
	if calcCount == 0 {
		calcCount = 240
	}
	batchSize := payload.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	if batchSize > 200 {
		batchSize = 200
	}
	continueOnError := payload.ContinueOnError

	allData := map[string]interface{}{}
	errorsBySymbol := map[string]string{}
	batches := chunkSymbols(symbols, batchSize)
	for _, batch := range batches {
		resp, err := r.worker.Run(ctx, FormulaRunRequest{
			Symbols:       batch,
			Script:        formula.Script,
			Args:          json.RawMessage(formula.ArgsJSON),
			Period:        period,
			Right:         right,
			OutCount:      outCount,
			CalcCount:     calcCount,
			ForceFallback: true,
		})
		if err == nil {
			mergeFormulaData(allData, resp.Data)
			continue
		}
		if !continueOnError {
			return nil, nil, err
		}
		for _, symbol := range batch {
			singleResp, singleErr := r.worker.Run(ctx, FormulaRunRequest{
				Symbols:       []string{symbol},
				Script:        formula.Script,
				Args:          json.RawMessage(formula.ArgsJSON),
				Period:        period,
				Right:         right,
				OutCount:      outCount,
				CalcCount:     calcCount,
				ForceFallback: true,
			})
			if singleErr != nil {
				errorsBySymbol[symbol] = singleErr.Error()
				continue
			}
			mergeFormulaData(allData, singleResp.Data)
		}
	}
	if len(allData) == 0 && len(errorsBySymbol) > 0 {
		return nil, nil, fmt.Errorf("选股任务全部失败: %s", mustJSON(errorsBySymbol))
	}

	items := extractSelectionResults(allData)
	if err := r.store.SaveSelectionResults(run, formula, items); err != nil {
		return nil, nil, err
	}
	go r.resolveNextQuoteForRun(run.ID)
	matchedSymbols := make([]string, 0, len(items))
	for _, item := range items {
		matchedSymbols = append(matchedSymbols, item.Symbol)
	}
	result := map[string]interface{}{
		"formula":         formula,
		"symbols":         symbols,
		"batch_size":      batchSize,
		"matched_symbols": matchedSymbols,
		"errors":          errorsBySymbol,
		"response": map[string]interface{}{
			"data": allData,
		},
	}
	return result, matchedSymbols, nil
}

func (r *AutomationRunner) runSystemSync(ctx context.Context, task AutomationTask) (interface{}, int, error) {
	var payload SystemSyncPayload
	if strings.TrimSpace(task.PayloadJSON) != "" {
		_ = json.Unmarshal([]byte(task.PayloadJSON), &payload)
	}
	switch strings.ToLower(strings.TrimSpace(payload.Scope)) {
	case "basic", "base":
		if useMarketService() {
			codesData, err := marketClient.RawData(ctx, "/api/codes", nil)
			if err != nil {
				return nil, 0, err
			}
			workdayData, err := marketClient.RawData(ctx, "/api/workday", nil)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{"scope": "basic", "codes": codesData, "workday": workdayData}, 2, nil
		}
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		if err := updateManagerCodes(manager); err != nil {
			return nil, 0, err
		}
		if err := manager.Workday.Update(); err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "basic", "codes": "success", "workday": "success"}, 2, nil
	case "", "codes":
		if useMarketService() {
			data, err := marketClient.RawData(ctx, "/api/codes", nil)
			return map[string]interface{}{"scope": "codes", "data": data}, 0, err
		}
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		err := updateManagerCodes(manager)
		return map[string]interface{}{"scope": "codes"}, 0, err
	case "workday":
		if useMarketService() {
			data, err := marketClient.RawData(ctx, "/api/workday", nil)
			return map[string]interface{}{"scope": "workday", "data": data}, 0, err
		}
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		err := manager.Workday.Update()
		return map[string]interface{}{"scope": "workday"}, 0, err
	case "hikyuu_after_close", "hikyuu-after-close":
		if !useMarketService() {
			return nil, 0, errors.New("Hikyuu盘后同步必须通过行情服务执行")
		}
		today := time.Now().In(time.Local).Format("2006-01-02")
		isWorkday, err := marketClient.IsWorkday(ctx, today)
		if err != nil {
			return nil, 0, err
		}
		if !isWorkday {
			return map[string]interface{}{"scope": "hikyuu_after_close", "source": "hikyuu", "skipped": true, "reason": "非交易日"}, 0, nil
		}
		task, err := marketClient.StartHikyuuAfterCloseSync(ctx, today)
		if err != nil {
			return nil, 0, err
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			switch strings.ToLower(strings.TrimSpace(task.Status)) {
			case "success":
				return map[string]interface{}{"scope": "hikyuu_after_close", "source": "hikyuu", "task": task}, 1, nil
			case "failed", "cancelled", "canceled":
				message := strings.TrimSpace(task.Error)
				if message == "" {
					message = strings.TrimSpace(task.Message)
				}
				if message == "" {
					message = "Hikyuu盘后同步失败"
				}
				return map[string]interface{}{"scope": "hikyuu_after_close", "source": "hikyuu", "task": task}, 0, errors.New(message)
			}
			select {
			case <-ctx.Done():
				return map[string]interface{}{"scope": "hikyuu_after_close", "source": "hikyuu", "task": task}, 0, ctx.Err()
			case <-ticker.C:
				task, err = marketClient.HikyuuTask(ctx, task.ID)
				if err != nil {
					return nil, 0, err
				}
			}
		}
	case "gbbq", "xrxd":
		if useMarketService() {
			taskResp, err := marketClient.SyncGbbqTask(ctx)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{
				"scope":         "gbbq",
				"delegated_to":  "market-service",
				"market_task":   taskResp,
				"poll_endpoint": "/api/tasks/{task_id}",
			}, 0, nil
		}
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		gbbq, err := tdx.NewGbbq(tdx.WithGbbqClient(client))
		if err != nil {
			return nil, 0, err
		}
		manager.Gbbq = gbbq
		if err := gbbq.Update(); err != nil {
			return nil, 0, err
		}
		all := gbbq.All()
		return map[string]interface{}{"scope": "gbbq", "codes": len(all)}, len(all), nil
	case "kline":
		if len(payload.Tables) == 0 {
			payload.Tables = []string{"day"}
		}
		validTables := make([]string, 0, len(payload.Tables))
		for _, table := range payload.Tables {
			if _, ok := extend.KlineTableMap[table]; ok {
				validTables = append(validTables, table)
			}
		}
		if len(validTables) == 0 {
			return nil, 0, errors.New("系统K线同步tables无有效值")
		}
		limit := payload.Limit
		if limit <= 0 {
			limit = 4
		}
		codes := normalizeSymbols(payload.Codes)
		if useMarketService() {
			payload.Limit = limit
			taskResp, err := marketClient.PullKlineTask(ctx, payload, validTables, codes)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{
				"scope":         "kline",
				"delegated_to":  "market-service",
				"market_task":   taskResp,
				"tables":        validTables,
				"codes":         codes,
				"limit":         limit,
				"poll_endpoint": "/api/tasks/{task_id}",
			}, len(codes), nil
		}
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		startAt := time.Unix(0, 0)
		if payload.StartDate != "" {
			parsed, err := parseLocalDate(payload.StartDate)
			if err != nil {
				return nil, 0, err
			}
			startAt = parsed
		}
		puller, err := extend.NewPullKline(extend.PullKlineConfig{
			Codes:      codes,
			Types:      validTables,
			Dir:        filepath.Join(tdx.DefaultDatabaseDir, "kline"),
			Goroutines: limit,
			StartAt:    startAt,
		})
		if err != nil {
			return nil, 0, err
		}
		// 新版 PullKline.Run 为定时任务入口,一次性拉取改用 Update
		if err := puller.Update(manager, true); err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{
			"scope":  "kline",
			"tables": validTables,
			"codes":  codes,
			"limit":  limit,
		}, len(codes), nil
	case "finance":
		codes := limitedSyncCodes(ctx, payload)
		if len(codes) == 0 {
			return nil, 0, errors.New("finance 同步需要 codes 或 max_codes")
		}
		rows, failures := syncFinanceSnapshots(ctx, codes, payload.ContinueOnError)
		path, err := writeAutomationSnapshot("finance", rows)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "finance", "path": path, "count": len(rows), "failures": failures}, len(rows), nil
	case "f10", "company":
		codes := limitedSyncCodes(ctx, payload)
		if len(codes) == 0 {
			return nil, 0, errors.New("f10 同步需要 codes 或 max_codes")
		}
		rows, failures := syncF10Snapshots(ctx, codes, payload.F10Length, payload.ContinueOnError)
		path, err := writeAutomationSnapshot("f10", rows)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "f10", "path": path, "count": len(rows), "failures": failures}, len(rows), nil
	case "market_info", "market-information":
		if !useMarketService() {
			return nil, 0, errors.New("市场信息同步需要配置MARKET_SERVICE_URL")
		}
		request := map[string]interface{}{
			"date":              payload.Date,
			"codes":             normalizeSymbols(payload.Codes),
			"max_codes":         payload.MaxCodes,
			"kinds":             payload.Kinds,
			"continue_on_error": payload.ContinueOnError,
		}
		var result struct {
			Success int `json:"success"`
		}
		if err := marketClient.post(ctx, "/api/market/sync", request, &result); err != nil {
			return nil, 0, err
		}
		return result, result.Success, nil
	case "market_industry_research", "industry_research":
		if !useMarketService() {
			return nil, 0, errors.New("行业研究同步需要配置MARKET_SERVICE_URL")
		}
		request := map[string]interface{}{
			"date":              payload.Date,
			"days":              payload.Days,
			"limit":             payload.Limit,
			"kinds":             []string{"industry-research"},
			"continue_on_error": payload.ContinueOnError,
		}
		var result struct {
			Success int `json:"success"`
		}
		if err := marketClient.post(ctx, "/api/market/sync", request, &result); err != nil {
			return nil, 0, err
		}
		return result, result.Success, nil
	case "block", "blocks":
		files := payload.BlockFiles
		if len(files) == 0 {
			files = []string{"gn", "fg", "zs", "hy", "block"}
		}
		rows := map[string]interface{}{}
		for _, file := range files {
			resolved := resolveBlockFile(file)
			if useMarketService() {
				query := url.Values{}
				query.Set("file", resolved)
				if payload.WithIndex {
					query.Set("with_index", "true")
				}
				resp, err := marketClient.RawData(ctx, "/api/block", query)
				if err != nil {
					return nil, 0, err
				}
				rows[resolved] = resp
				continue
			}
			if payload.WithIndex {
				resp, err := client.GetBlockDataWithIndex(resolved)
				if err != nil {
					return nil, 0, err
				}
				rows[resolved] = resp
			} else {
				resp, err := client.GetBlockData(resolved)
				if err != nil {
					return nil, 0, err
				}
				rows[resolved] = resp
			}
		}
		path, err := writeAutomationSnapshot("blocks", rows)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "block", "path": path, "files": files}, len(files), nil
	case "industry", "tdx_hy":
		if useMarketService() {
			resp, err := marketClient.RawData(ctx, "/api/tdx-hy", nil)
			if err != nil {
				return nil, 0, err
			}
			path, err := writeAutomationSnapshot("tdx_hy", resp)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{"scope": "industry", "path": path, "data": resp}, 1, nil
		}
		resp, err := client.GetTdxHy()
		if err != nil {
			return nil, 0, err
		}
		path, err := writeAutomationSnapshot("tdx_hy", resp)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "industry", "path": path, "count": len(resp)}, len(resp), nil
	case "stat":
		if useMarketService() {
			resp, err := marketClient.RawData(ctx, "/api/tdx-stat", nil)
			if err != nil {
				return nil, 0, err
			}
			path, err := writeAutomationSnapshot("tdx_stat", resp)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{"scope": "stat", "path": path, "data": resp}, 1, nil
		}
		resp, err := client.GetTdxStat()
		if err != nil {
			return nil, 0, err
		}
		path, err := writeAutomationSnapshot("tdx_stat", resp)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "stat", "path": path, "count": len(resp)}, len(resp), nil
	case "stat2":
		if useMarketService() {
			resp, err := marketClient.RawData(ctx, "/api/tdx-stat2", nil)
			if err != nil {
				return nil, 0, err
			}
			path, err := writeAutomationSnapshot("tdx_stat2", resp)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{"scope": "stat2", "path": path, "data": resp}, 1, nil
		}
		resp, err := client.GetTdxStat2()
		if err != nil {
			return nil, 0, err
		}
		path, err := writeAutomationSnapshot("tdx_stat2", resp)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "stat2", "path": path, "count": len(resp)}, len(resp), nil
	case "xgsg":
		if useMarketService() {
			resp, err := marketClient.RawData(ctx, "/api/xgsg", nil)
			if err != nil {
				return nil, 0, err
			}
			path, err := writeAutomationSnapshot("xgsg", resp)
			if err != nil {
				return nil, 0, err
			}
			return map[string]interface{}{"scope": "xgsg", "path": path, "data": resp}, 1, nil
		}
		resp, err := client.GetXgsg()
		if err != nil {
			return nil, 0, err
		}
		path, err := writeAutomationSnapshot("xgsg", resp)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "xgsg", "path": path, "count": len(resp)}, len(resp), nil
	case "all":
		results := map[string]interface{}{}
		if useMarketService() {
			basicResult, _, err := r.runSystemSync(ctx, AutomationTask{PayloadJSON: `{"scope":"basic"}`})
			if err != nil {
				return nil, 0, err
			}
			results["basic"] = basicResult
		} else {
			if manager == nil {
				return nil, 0, errors.New("数据管理器未初始化")
			}
			if err := updateManagerCodes(manager); err != nil {
				return nil, 0, err
			}
			results["codes"] = "success"
			if err := manager.Workday.Update(); err != nil {
				return nil, 0, err
			}
			results["workday"] = "success"
		}
		total := 0
		for _, scope := range []string{"gbbq", "block", "industry", "stat", "stat2", "xgsg"} {
			payload.Scope = scope
			raw, _ := json.Marshal(payload)
			task.PayloadJSON = string(raw)
			itemResult, count, err := r.runSystemSync(ctx, task)
			if err != nil {
				return nil, 0, err
			}
			results[scope] = itemResult
			total += count
		}
		if len(payload.Tables) > 0 || len(payload.Codes) > 0 {
			payload.Scope = "kline"
			raw, _ := json.Marshal(payload)
			task.PayloadJSON = string(raw)
			klineResult, count, err := r.runSystemSync(ctx, task)
			if err != nil {
				return nil, 0, err
			}
			results["kline"] = klineResult
			total += count
		}
		if payload.MaxCodes > 0 || len(payload.Codes) > 0 {
			payload.Scope = "finance"
			raw, _ := json.Marshal(payload)
			task.PayloadJSON = string(raw)
			itemResult, count, err := r.runSystemSync(ctx, task)
			if err != nil {
				return nil, 0, err
			}
			results["finance"] = itemResult
			total += count
			if payload.IncludeF10 {
				payload.Scope = "f10"
				raw, _ := json.Marshal(payload)
				task.PayloadJSON = string(raw)
				itemResult, count, err := r.runSystemSync(ctx, task)
				if err != nil {
					return nil, 0, err
				}
				results["f10"] = itemResult
				total += count
			}
		}
		return results, total, nil
	default:
		return nil, 0, fmt.Errorf("未知系统同步scope: %s", payload.Scope)
	}
}

func automationResultSkipped(result interface{}) bool {
	item, ok := result.(map[string]interface{})
	if !ok {
		return false
	}
	skipped, _ := item["skipped"].(bool)
	return skipped
}

func (r *AutomationRunner) runCustomTask(ctx context.Context, task AutomationTask) (interface{}, int, error) {
	var payload CustomTaskPayload
	if strings.TrimSpace(task.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
			return nil, 0, err
		}
	}
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "", "noop":
		return map[string]interface{}{"action": "noop", "data": payload.Data}, 0, nil
	case "system_sync":
		raw, _ := json.Marshal(payload.Sync)
		return r.runSystemSync(ctx, AutomationTask{PayloadJSON: string(raw)})
	case "http_request":
		result, err := runCustomHTTPRequest(ctx, payload)
		return result, 1, err
	default:
		return nil, 0, fmt.Errorf("未知custom action: %s", payload.Action)
	}
}

func chunkSymbols(symbols []string, size int) [][]string {
	if size <= 0 {
		size = 50
	}
	chunks := make([][]string, 0, (len(symbols)+size-1)/size)
	for start := 0; start < len(symbols); start += size {
		end := start + size
		if end > len(symbols) {
			end = len(symbols)
		}
		chunks = append(chunks, symbols[start:end])
	}
	return chunks
}

func mergeFormulaData(target map[string]interface{}, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return
	}
	for key, value := range decoded {
		target[key] = value
	}
}

// updateManagerCodes 手动更新数据管理器的代码库(新版 ICodes 接口无 Update,实际实现为 *tdx.Codes)
func updateManagerCodes(m *tdx.Manage) error {
	if c, ok := m.Codes.(*tdx.Codes); ok {
		return c.Update()
	}
	return errors.New("代码库未初始化")
}

func limitedSyncCodes(ctx context.Context, payload SystemSyncPayload) []string {
	codes := normalizeSymbols(payload.Codes)
	if len(codes) == 0 && payload.MaxCodes > 0 && useMarketService() {
		remoteCodes, err := marketClient.StockCodes(ctx, payload.MaxCodes)
		if err == nil {
			codes = normalizeSymbols(remoteCodes)
		}
	}
	if len(codes) == 0 && payload.MaxCodes > 0 && !useMarketService() && tdx.DefaultCodes != nil {
		codes = normalizeSymbols(tdx.DefaultCodes.GetStockCodes(payload.MaxCodes))
	}
	if payload.MaxCodes > 0 && len(codes) > payload.MaxCodes {
		codes = codes[:payload.MaxCodes]
	}
	return codes
}

func writeAutomationSnapshot(name string, data interface{}) (string, error) {
	dir := filepath.Join(tdx.DefaultDatabaseDir, "snapshots", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := time.Now().Format("20060102-150405") + ".json"
	path := filepath.Join(dir, filename)
	raw, err := json.MarshalIndent(map[string]interface{}{
		"created_at": nowText(),
		"name":       name,
		"data":       data,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func syncFinanceSnapshots(ctx context.Context, codes []string, continueOnError bool) ([]map[string]interface{}, map[string]string) {
	rows := make([]map[string]interface{}, 0, len(codes))
	failures := map[string]string{}
	for _, code := range codes {
		if useMarketService() {
			query := url.Values{}
			query.Set("code", code)
			resp, err := marketClient.RawData(ctx, "/api/finance", query)
			if err == nil {
				rows = append(rows, map[string]interface{}{
					"code": code,
					"data": resp,
				})
				continue
			}
			failures[code] = err.Error()
			if !continueOnError {
				break
			}
			continue
		}
		exchange, number, err := protocol.DecodeCode(protocol.AddPrefix(code))
		if err == nil {
			var resp interface{}
			resp, err = client.GetFinanceInfo(exchange, number)
			if err == nil {
				rows = append(rows, map[string]interface{}{
					"code": code,
					"data": resp,
				})
				continue
			}
		}
		failures[code] = err.Error()
		if !continueOnError {
			break
		}
	}
	return rows, failures
}

func syncF10Snapshots(ctx context.Context, codes []string, contentLength uint32, continueOnError bool) ([]map[string]interface{}, map[string]string) {
	if contentLength == 0 {
		contentLength = 4096
	}
	rows := make([]map[string]interface{}, 0, len(codes))
	failures := map[string]string{}
	for _, code := range codes {
		if useMarketService() {
			query := url.Values{}
			query.Set("code", code)
			categoriesRaw, err := marketClient.RawData(ctx, "/api/company/categories", query)
			if err != nil {
				failures[code] = err.Error()
				if !continueOnError {
					break
				}
				continue
			}
			item := map[string]interface{}{
				"code":       code,
				"categories": categoriesRaw,
			}
			categories := []protocol.CompanyCategory{}
			raw, _ := json.Marshal(categoriesRaw)
			_ = json.Unmarshal(raw, &categories)
			contents := map[string]string{}
			for _, category := range categories {
				if strings.TrimSpace(category.Filename) == "" {
					continue
				}
				contentQuery := url.Values{}
				contentQuery.Set("code", code)
				contentQuery.Set("filename", category.Filename)
				contentQuery.Set("length", fmt.Sprintf("%d", contentLength))
				contentRaw, err := marketClient.RawData(ctx, "/api/company/content", contentQuery)
				if err != nil {
					failures[code+":"+category.Filename] = err.Error()
					if !continueOnError {
						break
					}
					continue
				}
				contentMap, ok := contentRaw.(map[string]interface{})
				if !ok {
					contents[category.Filename] = fmt.Sprint(contentRaw)
					continue
				}
				if content, ok := contentMap["content"].(string); ok {
					contents[category.Filename] = content
				}
			}
			item["contents"] = contents
			rows = append(rows, item)
			if !continueOnError && len(failures) > 0 {
				break
			}
			continue
		}
		exchange, number, err := protocol.DecodeCode(protocol.AddPrefix(code))
		if err != nil {
			failures[code] = err.Error()
			if !continueOnError {
				break
			}
			continue
		}
		categories, err := client.GetCompanyCategory(exchange, number)
		if err != nil {
			failures[code] = err.Error()
			if !continueOnError {
				break
			}
			continue
		}
		item := map[string]interface{}{
			"code":       code,
			"categories": categories,
		}
		contents := map[string]string{}
		for _, category := range categories {
			if strings.TrimSpace(category.Filename) == "" {
				continue
			}
			content, err := client.GetCompanyContent(exchange, number, category.Filename, 0, contentLength)
			if err != nil {
				failures[code+":"+category.Filename] = err.Error()
				if !continueOnError {
					break
				}
				continue
			}
			contents[category.Filename] = content
		}
		item["contents"] = contents
		rows = append(rows, item)
		if !continueOnError && len(failures) > 0 {
			break
		}
	}
	return rows, failures
}

func runCustomHTTPRequest(ctx context.Context, payload CustomTaskPayload) (map[string]interface{}, error) {
	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodPost
	}
	if strings.TrimSpace(payload.URL) == "" {
		return nil, errors.New("custom http_request 缺少 url")
	}
	var bodyReader *bytes.Reader
	if payload.Body == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload.Body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, payload.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range payload.Headers {
		req.Header.Set(key, value)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	result := map[string]interface{}{
		"action":      "http_request",
		"method":      method,
		"url":         payload.URL,
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"body":        buf.String(),
	}
	if resp.StatusCode >= 300 {
		return result, fmt.Errorf("custom http_request 返回状态: %s", resp.Status)
	}
	return result, nil
}

func parseLocalDate(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", "20060102"} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("日期格式错误: %s", value)
}

func countFormulaMatches(data interface{}) int {
	raw, err := json.Marshal(data)
	if err != nil {
		return 0
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return 0
	}
	return countTruthyLeaves(decoded)
}

func extractSelectionResults(data interface{}) []SelectionResult {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	results := []SelectionResult{}
	for symbol, detail := range decoded {
		latest, hit := formulaDetailHit(detail)
		if !hit {
			continue
		}
		signalAt := nowText()
		results = append(results, SelectionResult{
			Symbol:         strings.ToUpper(symbol),
			Latest:         latest,
			SignalAt:       signalAt,
			SignalPrice:    latest,
			PriceType:      "latest",
			SnapshotStatus: "inferred",
			DetailJSON:     mustJSON(detail),
			CreatedAt:      signalAt,
		})
	}
	return results
}

func formulaDetailHit(detail interface{}) (float64, bool) {
	switch v := detail.(type) {
	case map[string]interface{}:
		if hit, ok := v["hit"].(bool); ok {
			latest := 0.0
			if n, ok := v["latest"].(float64); ok {
				latest = n
			}
			return latest, hit
		}
		if matched, ok := v["matched"].(bool); ok {
			latest := 0.0
			if n, ok := v["latest"].(float64); ok {
				latest = n
			}
			return latest, matched
		}
		if n, ok := v["latest"].(float64); ok {
			return n, n > 0
		}
	case bool:
		return 0, v
	case float64:
		return v, v > 0
	}
	return 0, false
}

func countTruthyLeaves(v interface{}) int {
	switch x := v.(type) {
	case map[string]interface{}:
		if hit, ok := x["hit"].(bool); ok && hit {
			return 1
		}
		if matched, ok := x["matched"].(bool); ok && matched {
			return 1
		}
		total := 0
		for _, item := range x {
			total += countTruthyLeaves(item)
		}
		return total
	case []interface{}:
		total := 0
		for _, item := range x {
			total += countTruthyLeaves(item)
		}
		return total
	case bool:
		if x {
			return 1
		}
	case float64:
		if x > 0 {
			return 1
		}
	}
	return 0
}
