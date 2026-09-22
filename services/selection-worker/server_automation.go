package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx/protocol"
)

var (
	appStore         *AppStore
	formulaWorker    *FormulaWorkerClient
	automationRunner *AutomationRunner
)

func initAutomationServices() error {
	store, err := OpenAppStore()
	if err != nil {
		return err
	}
	appStore = store
	formulaWorker = NewFormulaWorkerClient()
	automationRunner = NewAutomationRunner(appStore, formulaWorker)
	quoteMonitor = NewQuoteMonitor(appStore, marketClient)
	quoteMonitor.Start()
	if envBool("AUTOMATION_SCHEDULER_ENABLED", true) {
		if err := automationRunner.Start(); err != nil {
			return err
		}
	}
	return nil
}

func handleFormulaHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	info, err := formulaWorker.HealthInfo(ctx)
	if info == nil {
		info = map[string]interface{}{}
	}
	info["ok"] = err == nil
	if err != nil {
		info["error"] = err.Error()
	} else {
		info["error"] = ""
	}
	successResponse(w, info)
}

func handleFormulas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := appStore.ListFormulas()
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req Formula
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		if req.ArgsJSON == "" {
			var raw struct {
				Args json.RawMessage `json:"args"`
			}
			_ = json.NewDecoder(strings.NewReader("{}")).Decode(&raw)
		}
		item, err := appStore.UpsertFormula(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleFormulaOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/formulas/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "test" {
		handleFormulaTest(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetFormula(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "公式不存在"))
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		var req Formula
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.ID = id
		item, err := appStore.UpsertFormula(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodDelete:
		if err := appStore.DeleteFormula(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, map[string]string{"id": id})
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleFormulaTest(w http.ResponseWriter, r *http.Request, formulaID string) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	formula, err := appStore.GetFormula(formulaID)
	if err != nil {
		errorResponse(w, notFoundMessage(err, "公式不存在"))
		return
	}
	var req struct {
		Symbol    string          `json:"symbol"`
		Period    string          `json:"period"`
		Right     int             `json:"right"`
		OutCount  int             `json:"out_count"`
		CalcCount int             `json:"calc_count"`
		Args      json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	if req.Symbol == "" {
		errorResponse(w, "symbol不能为空")
		return
	}
	period := req.Period
	if period == "" {
		period = formula.Period
	}
	right := req.Right
	if right == 0 {
		right = formula.Right
	}
	args := json.RawMessage(formula.ArgsJSON)
	if len(req.Args) > 0 && string(req.Args) != "null" {
		args = req.Args
	}
	resp, err := formulaWorker.Run(r.Context(), FormulaRunRequest{
		Symbol:    req.Symbol,
		Script:    formula.Script,
		Args:      args,
		Period:    period,
		Right:     right,
		OutCount:  req.OutCount,
		CalcCount: req.CalcCount,
	})
	if err != nil {
		errorResponse(w, "公式执行失败: "+err.Error())
		return
	}
	successResponse(w, resp)
}

func handleFormulaRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req FormulaRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	resp, err := formulaWorker.Run(r.Context(), req)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, resp)
}

func handleStockPools(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := appStore.ListStockPools()
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req StockPool
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		item, err := appStore.UpsertStockPool(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleStockPoolOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/stock-pools/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 3 && parts[1] == "symbols" {
		handleStockPoolSymbolOperation(w, r, id, parts[2])
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetStockPool(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "股票池不存在"))
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		var req StockPool
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.ID = id
		item, err := appStore.UpsertStockPool(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodDelete:
		if err := appStore.DeleteStockPool(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, map[string]string{"id": id})
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleStockPoolSymbolOperation(w http.ResponseWriter, r *http.Request, poolID, symbol string) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}
	switch r.Method {
	case http.MethodPost:
		item, err := appStore.AddStockPoolSymbol(poolID, symbol)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "股票池不存在"))
			return
		}
		status := ""
		if poolID == DecisionWatchPoolID {
			status = "watch"
		}
		if poolID == DecisionExcludePoolID {
			status = "exclude"
		}
		if status != "" {
			_ = appStore.SetDecisionStatus(symbol, status)
		}
		successResponse(w, item)
	case http.MethodDelete:
		item, err := appStore.RemoveStockPoolSymbol(poolID, symbol)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "股票池不存在"))
			return
		}
		status := ""
		if poolID == DecisionWatchPoolID {
			status = "watch"
		}
		if poolID == DecisionExcludePoolID {
			status = "exclude"
		}
		if status != "" {
			if note, err := appStore.GetDecisionNote(symbol); err == nil && note.Status == status {
				_ = appStore.SetDecisionStatus(symbol, "")
			}
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleDecisionNotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := appStore.ListDecisionNotes(r.URL.Query().Get("status"), limit)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req DecisionNote
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		item, err := appStore.UpsertDecisionNote(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleDecisionNoteOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/decision-notes/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	symbol := parts[0]
	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetDecisionNote(symbol)
		if err == sql.ErrNoRows {
			successResponse(w, DecisionNote{Symbol: strings.ToUpper(symbol)})
			return
		}
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		var req DecisionNote
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.Symbol = symbol
		item, err := appStore.UpsertDecisionNote(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleTradingSystemState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state, err := appStore.GetTradingSystemState()
		if err == sql.ErrNoRows {
			successResponse(w, defaultTradingSystemState())
			return
		}
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, state)
	case http.MethodPut, http.MethodPost:
		var req TradingSystemState
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		state, err := appStore.UpsertTradingSystemState(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, state)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleAutomationTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := appStore.ListAutomationTasks()
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req AutomationTask
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		if isFixedAutomationTaskID(req.ID) {
			errorResponse(w, "固定任务只能开启或关闭，不能编辑")
			return
		}
		item, err := appStore.UpsertAutomationTask(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		if err := automationRunner.Reload(); err != nil {
			errorResponse(w, "任务已保存，但调度重载失败: "+err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleAutomationTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req struct {
		Template string `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	task, err := buildAutomationTemplate(req.Template)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	item, err := appStore.UpsertAutomationTask(task)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	if err := automationRunner.Reload(); err != nil {
		errorResponse(w, "模板已保存，但调度重载失败: "+err.Error())
		return
	}
	successResponse(w, item)
}

func handleAutomationReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	if err := automationRunner.Reload(); err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, map[string]string{"status": "reloaded"})
}

func buildAutomationTemplate(name string) (AutomationTask, error) {
	switch name {
	case "morning_sync":
		return AutomationTask{
			Name:        "早盘基础数据同步",
			Type:        "system_sync",
			Cron:        "0 0 8 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"basic"}`,
			WebhookIDs:  "[]",
		}, nil
	case "evening_kline":
		return AutomationTask{
			Name:        "晚盘日K同步",
			Type:        "system_sync",
			Cron:        "0 30 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"kline","tables":["day"],"limit":4}`,
			WebhookIDs:  "[]",
		}, nil
	case "evening_full":
		return AutomationTask{
			Name:        "晚盘完整数据同步",
			Type:        "system_sync",
			Cron:        "0 0 21 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"all","tables":["day"],"limit":4,"max_codes":200,"block_files":["gn","fg","zs","hy","block"],"with_index":true,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "selection_tracking":
		return AutomationTask{
			Name:        "选股结果跟踪评估",
			Type:        "selection_tracking",
			Cron:        "0 15 17 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"horizons":[3,5,10],"history_buffer":30,"target_return":3,"drawdown_limit":5,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "strategy_execution":
		return AutomationTask{
			Name:        "策略执行",
			Type:        "strategy_selection",
			Cron:        "0 0 15 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"strategy_id":""}`,
			WebhookIDs:  "[]",
		}, nil
	case "market_long_tiger_sync":
		return AutomationTask{
			Name:        "龙虎榜同步",
			Type:        "system_sync",
			Cron:        "0 0 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"market_info","kinds":["long-tiger"],"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "market_hot_money_sync":
		return AutomationTask{
			Name:        "游资动向同步",
			Type:        "system_sync",
			Cron:        "0 5 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"market_info","kinds":["hot_money"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "market_research_sync":
		return AutomationTask{
			Name:        "个股研报同步",
			Type:        "system_sync",
			Cron:        "0 10 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"market_info","kinds":["research"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "market_notice_sync":
		return AutomationTask{
			Name:        "公司公告同步",
			Type:        "system_sync",
			Cron:        "0 15 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"market_info","kinds":["notice"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	case "market_industry_research_sync":
		return AutomationTask{
			Name:        "行业研究同步",
			Type:        "system_sync",
			Cron:        "0 20 18 * * 1-5",
			Enabled:     false,
			PayloadJSON: `{"scope":"market_industry_research","days":7,"limit":500,"continue_on_error":true}`,
			WebhookIDs:  "[]",
		}, nil
	default:
		return AutomationTask{}, fmt.Errorf("未知任务模板: %s", name)
	}
}

func handleAutomationOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/automations/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "enabled" {
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			errorResponse(w, "只支持PUT/POST请求")
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		item, err := appStore.SetAutomationTaskEnabled(id, req.Enabled)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "任务不存在"))
			return
		}
		if err := automationRunner.Reload(); err != nil {
			errorResponse(w, "任务已更新，但调度重载失败: "+err.Error())
			return
		}
		if refreshed, err := appStore.GetAutomationTask(id); err == nil {
			item = refreshed
		}
		successResponse(w, item)
		return
	}
	if len(parts) == 2 && parts[1] == "run" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		if id == SystemStrategyDailyBatchTaskID {
			run, err := automationRunner.startSystemStrategyBatch()
			if err != nil {
				errorResponse(w, "任务运行失败: "+err.Error())
				return
			}
			successResponse(w, run)
			return
		}
		run, err := automationRunner.RunTask(r.Context(), id)
		if err != nil {
			errorResponse(w, "任务执行失败: "+err.Error())
			return
		}
		successResponse(w, run)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetAutomationTask(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "任务不存在"))
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		if isFixedAutomationTaskID(id) {
			errorResponse(w, "固定任务只能开启或关闭，不能编辑")
			return
		}
		var req AutomationTask
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.ID = id
		item, err := appStore.UpsertAutomationTask(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		if err := automationRunner.Reload(); err != nil {
			errorResponse(w, "任务已保存，但调度重载失败: "+err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodDelete:
		if isFixedAutomationTaskID(id) {
			errorResponse(w, "固定任务不能删除")
			return
		}
		if err := appStore.DeleteAutomationTask(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		if err := automationRunner.Reload(); err != nil {
			errorResponse(w, "任务已删除，但调度重载失败: "+err.Error())
			return
		}
		successResponse(w, map[string]string{"id": id})
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleAutomationRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	taskID := r.URL.Query().Get("task_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := appStore.ListAutomationRuns(taskID, limit)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	if items == nil {
		items = []AutomationRun{}
	}
	successResponse(w, items)
}

func handleSelectionResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	onlyLatest := r.URL.Query().Get("latest") == "true" || r.URL.Query().Get("latest") == "1"
	items, err := appStore.ListSelectionResults(
		r.URL.Query().Get("task_id"),
		r.URL.Query().Get("formula_id"),
		r.URL.Query().Get("symbol"),
		onlyLatest,
		limit,
	)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	if items == nil {
		items = []SelectionResult{}
	}
	successResponse(w, items)
}

type SignalScore struct {
	Trend  int `json:"trend"`
	Volume int `json:"volume"`
	Place  int `json:"place"`
	Risk   int `json:"risk"`
	Total  int `json:"total"`
}

type NextDayTrack struct {
	Available   bool    `json:"available"`
	Date        int     `json:"date"`
	OpenChange  float64 `json:"open_change"`
	MaxGain     float64 `json:"max_gain"`
	Drawdown    float64 `json:"drawdown"`
	CloseChange float64 `json:"close_change"`
	Summary     string  `json:"summary"`
}

type ReviewItem struct {
	Result   SelectionResult `json:"result"`
	Score    SignalScore     `json:"score"`
	Track    NextDayTrack    `json:"track"`
	Note     DecisionNote    `json:"note"`
	Status   string          `json:"status"`
	Watch    bool            `json:"watch"`
	Excluded bool            `json:"excluded"`
}

func handleDailyReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	data, err := appStore.BuildDailyReview(
		strings.TrimSpace(r.URL.Query().Get("date")),
		strings.TrimSpace(r.URL.Query().Get("batch_id")),
		limit,
	)
	if err == sql.ErrNoRows {
		errorResponse(w, "日报批次不存在")
		return
	}
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, data)
}

func handleDailyReviewAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	data, err := appStore.BuildDailyStrategyAnalysis(
		strings.TrimSpace(r.URL.Query().Get("start_date")),
		strings.TrimSpace(r.URL.Query().Get("end_date")),
		days,
		limit,
	)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, data)
}

func symbolSet(symbols []string) map[string]bool {
	out := map[string]bool{}
	for _, symbol := range normalizeSymbols(symbols) {
		out[symbol] = true
	}
	return out
}

func countHandled(items []ReviewItem) int {
	count := 0
	for _, item := range items {
		if item.Watch || item.Excluded || strings.TrimSpace(item.Note.Status) != "" {
			count++
		}
	}
	return count
}

func buildReviewMetrics(ctx context.Context, item SelectionResult) (SignalScore, NextDayTrack) {
	rows, err := loadFormulaKline(ctx, item.Symbol, "day", 260)
	if err != nil || len(rows) < 30 {
		return SignalScore{}, NextDayTrack{Summary: "K线不足"}
	}
	score := scoreSignal(rows)
	track := trackNextDay(item, rows)
	return score, track
}

func scoreSignal(rows []FormulaKline) SignalScore {
	last := rows[len(rows)-1]
	close := last.Close
	ma5 := avgClose(rows, 5)
	ma20 := avgClose(rows, 20)
	ma20Prev := avgClose(rows[:len(rows)-5], 20)
	vol20 := avgVol(rows[:len(rows)-1], 20)
	high60, low60 := highLow(rows, 60)
	volatility := avgRange(rows, 20)

	trend := clampScore(35 + int((ma5-ma20)/math.Max(close, 0.01)*600) + int((ma20-ma20Prev)/math.Max(close, 0.01)*900))
	if close > ma20 {
		trend += 15
	}
	trend = clampScore(trend)

	volume := 50
	if vol20 > 0 {
		ratio := last.Vol / vol20
		volume = clampScore(35 + int(ratio*35))
		if ratio > 3 {
			volume -= 15
		}
	}

	place := 50
	if high60 > low60 {
		pos := (close - low60) / (high60 - low60)
		place = clampScore(85 - int(math.Abs(pos-0.65)*120))
	}

	risk := clampScore(85 - int(volatility*450))
	if high60 > 0 && close/high60 > 0.97 {
		risk -= 10
	}
	risk = clampScore(risk)

	total := int(math.Round(float64(trend)*0.32 + float64(volume)*0.22 + float64(place)*0.24 + float64(risk)*0.22))
	return SignalScore{Trend: trend, Volume: volume, Place: place, Risk: risk, Total: clampScore(total)}
}

func trackNextDay(item SelectionResult, rows []FormulaKline) NextDayTrack {
	signalDate := selectionSignalDate(item)
	base := 0.0
	for _, row := range rows {
		if row.Date <= signalDate && row.Close > 0 {
			base = row.Close
		}
	}
	if base <= 0 {
		base = item.Latest
	}
	for _, row := range rows {
		if row.Date > signalDate {
			if base <= 0 {
				base = row.YClose
			}
			if base <= 0 {
				base = row.Open
			}
			track := NextDayTrack{
				Available:   true,
				Date:        row.Date,
				OpenChange:  pctChange(row.Open, base),
				MaxGain:     pctChange(row.High, base),
				Drawdown:    pctChange(row.Low, base),
				CloseChange: pctChange(row.Close, base),
			}
			track.Summary = trackSummary(track)
			return track
		}
	}
	return NextDayTrack{Summary: "暂无次日K线"}
}

func selectionSignalDate(item SelectionResult) int {
	if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
		return dateInt(t)
	}
	if len(item.CreatedAt) >= 10 {
		if t, err := time.Parse("2006-01-02", item.CreatedAt[:10]); err == nil {
			return dateInt(t)
		}
	}
	return 0
}

func trackSummary(track NextDayTrack) string {
	if !track.Available {
		return track.Summary
	}
	if track.OpenChange >= 2 {
		return "高开"
	}
	if track.OpenChange <= -2 {
		return "低开"
	}
	if track.MaxGain >= 5 {
		return "冲高"
	}
	if track.Drawdown <= -4 {
		return "回撤"
	}
	if track.CloseChange > 0 {
		return "收涨"
	}
	return "待观察"
}

func avgClose(rows []FormulaKline, n int) float64 {
	if len(rows) == 0 {
		return 0
	}
	if len(rows) < n {
		n = len(rows)
	}
	sum := 0.0
	for _, row := range rows[len(rows)-n:] {
		sum += row.Close
	}
	return sum / float64(n)
}

func avgVol(rows []FormulaKline, n int) float64 {
	if len(rows) == 0 {
		return 0
	}
	if len(rows) < n {
		n = len(rows)
	}
	sum := 0.0
	for _, row := range rows[len(rows)-n:] {
		sum += row.Vol
	}
	return sum / float64(n)
}

func highLow(rows []FormulaKline, n int) (float64, float64) {
	if len(rows) < n {
		n = len(rows)
	}
	subset := rows[len(rows)-n:]
	high := subset[0].High
	low := subset[0].Low
	for _, row := range subset {
		if row.High > high {
			high = row.High
		}
		if row.Low < low {
			low = row.Low
		}
	}
	return high, low
}

func avgRange(rows []FormulaKline, n int) float64 {
	if len(rows) == 0 {
		return 0
	}
	if len(rows) < n {
		n = len(rows)
	}
	sum := 0.0
	for _, row := range rows[len(rows)-n:] {
		if row.Close > 0 {
			sum += (row.High - row.Low) / row.Close
		}
	}
	return sum / float64(n)
}

func pctChange(value, base float64) float64 {
	if base <= 0 {
		return 0
	}
	return roundFloat((value-base)/base*100, 2)
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func roundFloat(v float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(v*pow) / pow
}

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := appStore.ListWebhooks()
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req Webhook
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		item, err := appStore.UpsertWebhook(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleWebhookOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/webhooks/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		hook, err := appStore.GetWebhook(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "Webhook不存在"))
			return
		}
		logs := sendWebhooks(r.Context(), []Webhook{hook}, WebhookEvent{
			Event:   "webhook.test",
			Status:  "success",
			Message: "tdx-workbench webhook测试",
		})
		successResponse(w, logs)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetWebhook(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "Webhook不存在"))
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		var req Webhook
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.ID = id
		item, err := appStore.UpsertWebhook(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodDelete:
		if err := appStore.DeleteWebhook(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, map[string]string{"id": id})
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleHQChartKline(w http.ResponseWriter, r *http.Request) {
	code := normalizeStockCode(r.URL.Query().Get("symbol"))
	if code == "" {
		code = normalizeStockCode(r.URL.Query().Get("code"))
	}
	if code == "" {
		errorResponse(w, "symbol不能为空")
		return
	}
	period := r.URL.Query().Get("period")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 800
	}
	data, err := loadFormulaKline(r.Context(), code, period, limit)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, map[string]interface{}{
		"symbol": strings.ToUpper(code),
		"period": formulaPeriodToKlineType(period),
		"data":   data,
	})
}

func handleHQChartHistory(w http.ResponseWriter, r *http.Request) {
	rawSymbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	code := normalizeStockCode(rawSymbol)
	if code == "" {
		rawSymbol = strings.TrimSpace(r.URL.Query().Get("code"))
		code = normalizeStockCode(rawSymbol)
	}
	if code == "" {
		errorResponse(w, "symbol不能为空")
		return
	}
	period := r.URL.Query().Get("period")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 800
	}
	isIndex := parseBool(r.URL.Query().Get("index"))
	var rows []FormulaKline
	var err error
	if isIndex {
		rows, err = loadHQChartIndexKline(r.Context(), code, period, limit)
	} else {
		rows, err = loadFormulaKline(r.Context(), code, period, limit)
	}
	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	chartRows := formulaRowsToHQChartRows(rows)
	symbol := toHQChartSymbol(code)
	if isIndex {
		symbol = toHQChartIndexSymbol(rawSymbol, code)
	}
	successResponse(w, map[string]interface{}{
		"symbol": symbol,
		"name":   symbol,
		"period": formulaPeriodToKlineType(period),
		"data":   chartRows,
		"ver":    2,
	})
}

func loadHQChartIndexKline(ctx context.Context, symbol, period string, calcCount int) ([]FormulaKline, error) {
	code := normalizeStockCode(symbol)
	if code == "" {
		return nil, errors.New("指数代码不能为空")
	}
	if calcCount <= 0 {
		calcCount = 800
	}
	klineType := formulaPeriodToKlineType(period)
	if useMarketService() {
		resp, err := marketClient.IndexKline(ctx, code, klineType, calcCount)
		if err == nil {
			if resp == nil || len(resp.List) == 0 {
				return nil, errors.New("指数K线为空")
			}
			return protocolKlinesToFormulaRows(resp.List, calcCount), nil
		}
		if !allowDirectMarketFallback() {
			return nil, err
		}
	}
	if err := ensureLocalMarketRuntime(); err != nil {
		return nil, err
	}

	limit := uint16(min(calcCount, 800))
	var resp *protocol.KlineResp
	var err error
	switch klineType {
	case "minute1":
		resp, err = client.GetIndex(protocol.TypeKlineMinute, code, 0, limit)
	case "minute5":
		resp, err = client.GetIndex(protocol.TypeKline5Minute, code, 0, limit)
	case "minute15":
		resp, err = client.GetIndex(protocol.TypeKline15Minute, code, 0, limit)
	case "minute30":
		resp, err = client.GetIndex(protocol.TypeKline30Minute, code, 0, limit)
	case "hour":
		resp, err = client.GetIndex(protocol.TypeKline60Minute, code, 0, limit)
	case "week":
		resp, err = client.GetIndexWeekAll(code)
	case "month":
		resp, err = client.GetIndexMonthAll(code)
	default:
		resp, err = client.GetIndexDay(code, 0, limit)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.List) == 0 {
		return nil, errors.New("指数K线为空")
	}
	return protocolKlinesToFormulaRows(resp.List, calcCount), nil
}

func protocolKlinesToFormulaRows(list []*protocol.Kline, calcCount int) []FormulaKline {
	if calcCount > 0 && len(list) > calcCount {
		list = list[len(list)-calcCount:]
	}
	out := make([]FormulaKline, 0, len(list))
	var prevClose float64
	for i, item := range list {
		closePrice := priceToYuan(item.Close)
		yClose := priceToYuan(item.Last)
		if yClose == 0 && i > 0 {
			yClose = prevClose
		}
		out = append(out, FormulaKline{
			Date:   dateInt(item.Time),
			Time:   timeInt(item.Time),
			YClose: yClose,
			Open:   priceToYuan(item.Open),
			High:   priceToYuan(item.High),
			Low:    priceToYuan(item.Low),
			Close:  closePrice,
			Vol:    float64(item.Volume),
			Amount: amountToYuan(item.Amount, closePrice, float64(item.Volume)),
		})
		prevClose = closePrice
	}
	return out
}

func formulaRowsToHQChartRows(rows []FormulaKline) [][]interface{} {
	chartRows := make([][]interface{}, 0, len(rows))
	for _, item := range rows {
		row := []interface{}{
			item.Date,
			item.YClose,
			item.Open,
			item.High,
			item.Low,
			item.Close,
			item.Vol,
			item.Amount,
		}
		if item.Time > 0 {
			row = append(row, item.Time)
		}
		chartRows = append(chartRows, row)
	}
	return chartRows
}

func normalizeStockCode(symbol string) string {
	s := strings.TrimSpace(strings.ToLower(symbol))
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, "."); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimLeft(s, " ")
}

func toHQChartIndexSymbol(rawSymbol, code string) string {
	raw := strings.ToLower(strings.TrimSpace(rawSymbol))
	if strings.HasSuffix(raw, ".sh") || strings.HasSuffix(raw, ".sz") || strings.HasSuffix(raw, ".bj") {
		return raw
	}
	c := normalizeStockCode(code)
	if strings.HasPrefix(c, "399") {
		return c + ".sz"
	}
	return c + ".sh"
}

func toHQChartSymbol(code string) string {
	c := normalizeStockCode(code)
	if c == "" {
		return ""
	}
	market := "sz"
	if strings.HasPrefix(c, "6") || strings.HasPrefix(c, "9") {
		market = "sh"
	}
	return c + "." + market
}

func pathParts(path, prefix string) []string {
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func notFoundMessage(err error, fallback string) string {
	if err == sql.ErrNoRows {
		return fallback
	}
	return fmt.Sprintf("%s: %v", fallback, err)
}
