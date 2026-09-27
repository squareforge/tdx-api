package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	Limit           int     `json:"limit"`
	Horizons        []int   `json:"horizons"`
	TargetReturn    float64 `json:"target_return"`
	DrawdownLimit   float64 `json:"drawdown_limit"`
	ContinueOnError bool    `json:"continue_on_error"`
}

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
	}
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
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
				defer cancel()
				if _, err := r.RunTask(ctx, taskID); err != nil {
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

func (r *AutomationRunner) RunTask(ctx context.Context, taskID string) (AutomationRun, error) {
	task, err := r.store.GetAutomationTask(taskID)
	if err != nil {
		return AutomationRun{}, err
	}
	return r.runTask(ctx, task)
}

func (r *AutomationRunner) runTask(ctx context.Context, task AutomationTask) (AutomationRun, error) {
	run, err := r.store.CreateAutomationRun(task)
	if err != nil {
		return run, err
	}
	return r.runTaskWithRun(ctx, task, run)
}

func (r *AutomationRunner) runTaskWithRun(ctx context.Context, task AutomationTask, run AutomationRun) (AutomationRun, error) {

	status := "success"
	logText := ""
	resultJSON := "{}"
	matchedCount := 0
	var result interface{}
	var matchedSymbols []string
	var err error

	if task.OnlyTradeDay {
		today := time.Now()
		tradeDay, tradeDayErr := isCalendarWorkday(today)
		if tradeDayErr != nil {
			err = fmt.Errorf("交易日判断失败: %w", tradeDayErr)
		} else if !tradeDay {
			status = "skipped"
			logText = "非交易日，已跳过"
			result = map[string]interface{}{
				"skipped": true,
				"reason":  "non_trade_day",
				"date":    today.Format("20060102"),
			}
		}
	}

	if err == nil && status != "skipped" {
		switch task.Type {
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
	}

	if err != nil {
		status = "failed"
		logText = err.Error()
	} else {
		resultJSON = mustJSON(result)
	}

	if finishErr := r.store.FinishAutomationRun(run.ID, status, logText, resultJSON, matchedCount); finishErr != nil && err == nil {
		err = finishErr
	}
	message := logText
	if message == "" {
		message = fmt.Sprintf("完成，命中 %d 条", matchedCount)
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
	if status == "skipped" {
		eventName = "automation.skipped"
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

	if latest, latestErr := r.store.GetAutomationRun(run.ID); latestErr == nil {
		run = latest
	}
	return run, err
}

func (r *AutomationRunner) runSelectionTracking(ctx context.Context, task AutomationTask) (interface{}, int, error) {
	payload := SelectionTrackingPayload{Limit: 500, Horizons: []int{1, 3, 5, 10}, ContinueOnError: true}
	if strings.TrimSpace(task.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
			return nil, 0, err
		}
	}
	if payload.Limit <= 0 || payload.Limit > 500 {
		payload.Limit = 500
	}
	items, err := r.store.ListSelectionResults("", "", "", false, payload.Limit)
	if err != nil {
		return nil, 0, err
	}
	targetReturn, drawdownLimit := workbench.DefaultTrackingPolicy(payload.TargetReturn, payload.DrawdownLimit)
	tracked := make([]SelectionTrackingItem, 0, len(items))
	errorsByResult := map[string]string{}
	for _, item := range items {
		bars, loadErr := loadTrackingKline(ctx, item.Symbol, 800)
		if loadErr != nil {
			errorsByResult[item.ID] = loadErr.Error()
			if !payload.ContinueOnError {
				return nil, len(tracked), loadErr
			}
			continue
		}
		tracking := workbench.EvaluateSelectionTracking(item, toTrackingBars(bars), payload.Horizons, targetReturn, drawdownLimit, time.Now())
		if updateErr := r.store.UpdateSelectionTracking(item.ID, tracking); updateErr != nil {
			return nil, len(tracked), updateErr
		}
		item.Tracking = &tracking
		tracked = append(tracked, SelectionTrackingItem{Result: item, Tracking: tracking})
	}
	return map[string]interface{}{
		"summary": workbench.SummarizeSelectionTracking(tracked, payload.Horizons),
		"policy":  map[string]float64{"target_return": targetReturn, "drawdown_limit": drawdownLimit},
		"errors":  errorsByResult,
	}, len(tracked), nil
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
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		err := updateManagerCodes(manager)
		return map[string]interface{}{"scope": "codes"}, 0, err
	case "workday":
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
		err := manager.Workday.Update()
		return map[string]interface{}{"scope": "workday"}, 0, err
	case "gbbq", "xrxd":
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
		if manager == nil {
			return nil, 0, errors.New("数据管理器未初始化")
		}
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
		startAt := time.Unix(0, 0)
		if payload.StartDate != "" {
			parsed, err := parseLocalDate(payload.StartDate)
			if err != nil {
				return nil, 0, err
			}
			startAt = parsed
		}
		codes := normalizeSymbols(payload.Codes)
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
		codes := limitedSyncCodes(payload)
		if len(codes) == 0 {
			return nil, 0, errors.New("finance 同步需要 codes 或 max_codes")
		}
		rows, failures := syncFinanceSnapshots(codes, payload.ContinueOnError)
		path, err := writeAutomationSnapshot("finance", rows)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "finance", "path": path, "count": len(rows), "failures": failures}, len(rows), nil
	case "f10", "company":
		codes := limitedSyncCodes(payload)
		if len(codes) == 0 {
			return nil, 0, errors.New("f10 同步需要 codes 或 max_codes")
		}
		rows, failures := syncF10Snapshots(codes, payload.F10Length, payload.ContinueOnError)
		path, err := writeAutomationSnapshot("f10", rows)
		if err != nil {
			return nil, 0, err
		}
		return map[string]interface{}{"scope": "f10", "path": path, "count": len(rows), "failures": failures}, len(rows), nil
	case "market_info", "market-information":
		if marketServiceBaseURL() == "" {
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
		if err := marketServicePost(ctx, "/api/market/sync", request, &result); err != nil {
			return nil, 0, err
		}
		return result, result.Success, nil
	case "market_industry_research", "industry_research":
		if marketServiceBaseURL() == "" {
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
		if err := marketServicePost(ctx, "/api/market/sync", request, &result); err != nil {
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

func limitedSyncCodes(payload SystemSyncPayload) []string {
	codes := normalizeSymbols(payload.Codes)
	if len(codes) == 0 && payload.MaxCodes > 0 && tdx.DefaultCodes != nil {
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

func syncFinanceSnapshots(codes []string, continueOnError bool) ([]map[string]interface{}, map[string]string) {
	rows := make([]map[string]interface{}, 0, len(codes))
	failures := map[string]string{}
	for _, code := range codes {
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

func syncF10Snapshots(codes []string, contentLength uint32, continueOnError bool) ([]map[string]interface{}, map[string]string) {
	if contentLength == 0 {
		contentLength = 4096
	}
	rows := make([]map[string]interface{}, 0, len(codes))
	failures := map[string]string{}
	for _, code := range codes {
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
		results = append(results, SelectionResult{
			Symbol:     strings.ToUpper(symbol),
			Latest:     latest,
			DetailJSON: mustJSON(detail),
			CreatedAt:  nowText(),
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
