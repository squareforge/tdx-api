package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	workbench "workbench-core"
)

var comparisonBenchmarks = map[string]string{
	"sh000001": "上证指数", "sh000300": "沪深300", "sh000905": "中证500", "sh000852": "中证1000",
}

type historicalComparisonRequest struct {
	StrategyID        string   `json:"strategy_id"`
	Mode              string   `json:"mode"`
	ControlSampleSize int      `json:"control_sample_size"`
	TargetReturn      float64  `json:"target_return"`
	DrawdownLimit     float64  `json:"drawdown_limit"`
	BenchmarkIDs      []string `json:"benchmark_ids"`
	Seed              int64    `json:"seed"`
	Resample          bool     `json:"resample"`
}

type comparisonControl struct {
	Symbol   string                                `json:"symbol"`
	Base     float64                               `json:"base"`
	Horizons map[string]workbench.SelectionHorizon `json:"horizons"`
}

type comparisonHorizon struct {
	Status            string             `json:"status"`
	SignalClose       float64            `json:"signal_close"`
	SignalReturn      float64            `json:"signal_return"`
	SignalMFE         float64            `json:"signal_mfe"`
	SignalMAE         float64            `json:"signal_mae"`
	ControlReturn     float64            `json:"control_return"`
	ControlMedian     float64            `json:"control_median"`
	ControlMFE        float64            `json:"control_mfe"`
	ControlMAE        float64            `json:"control_mae"`
	ControlCount      int                `json:"control_count"`
	PairedExcess      float64            `json:"paired_excess"`
	SignalTargetHit   bool               `json:"signal_target_hit"`
	ControlTargetRate float64            `json:"control_target_rate"`
	BenchmarkReturns  map[string]float64 `json:"benchmark_returns"`
}

type comparisonEventResult struct {
	StoredSignalBase  float64                      `json:"stored_signal_base"`
	CurrentSignalBase float64                      `json:"current_signal_base"`
	BaseChanged       bool                         `json:"base_changed"`
	CandidateCount    int                          `json:"candidate_count"`
	Controls          []comparisonControl          `json:"controls"`
	Horizons          map[string]comparisonHorizon `json:"horizons"`
}

type comparisonSummaryRow struct {
	Completed         int                `json:"completed"`
	SignalAverage     float64            `json:"signal_average"`
	SignalMedian      float64            `json:"signal_median"`
	ControlAverage    float64            `json:"control_average"`
	ControlMedian     float64            `json:"control_median"`
	PairedAverage     float64            `json:"paired_average"`
	PairedMedian      float64            `json:"paired_median"`
	PairedWinRate     float64            `json:"paired_win_rate"`
	SignalTargetRate  float64            `json:"signal_target_rate"`
	ControlTargetRate float64            `json:"control_target_rate"`
	SignalMFE         float64            `json:"signal_mfe"`
	ControlMFE        float64            `json:"control_mfe"`
	SignalMAE         float64            `json:"signal_mae"`
	ControlMAE        float64            `json:"control_mae"`
	ConfidenceLow     float64            `json:"confidence_low"`
	ConfidenceHigh    float64            `json:"confidence_high"`
	BenchmarkAverage  map[string]float64 `json:"benchmark_average"`
}

type comparisonSummary struct {
	EventCount        int                                        `json:"event_count"`
	CompletedEvents   int                                        `json:"completed_events"`
	PartialEvents     int                                        `json:"partial_events"`
	UnavailableEvents int                                        `json:"unavailable_events"`
	BaseChangedEvents int                                        `json:"base_changed_events"`
	ControlCount      int                                        `json:"control_count"`
	CandidateSymbols  int                                        `json:"candidate_symbols"`
	HorizonRows       map[string]comparisonSummaryRow            `json:"horizons"`
	Years             map[string]map[string]comparisonSummaryRow `json:"years"`
	Warnings          []string                                   `json:"warnings"`
}

type comparisonAccumulator struct {
	signal, control, paired, signalMFE, controlMFE, signalMAE, controlMAE []float64
	signalTargets                                                         int
	controlTargetRates                                                    []float64
	benchmarks                                                            map[string][]float64
}

func (a *comparisonAccumulator) add(row comparisonHorizon) {
	if row.Status != "complete" {
		return
	}
	a.signal = append(a.signal, row.SignalReturn)
	a.control = append(a.control, row.ControlReturn)
	a.paired = append(a.paired, row.PairedExcess)
	a.signalMFE = append(a.signalMFE, row.SignalMFE)
	a.controlMFE = append(a.controlMFE, row.ControlMFE)
	a.signalMAE = append(a.signalMAE, row.SignalMAE)
	a.controlMAE = append(a.controlMAE, row.ControlMAE)
	if row.SignalTargetHit {
		a.signalTargets++
	}
	a.controlTargetRates = append(a.controlTargetRates, row.ControlTargetRate)
	if a.benchmarks == nil {
		a.benchmarks = map[string][]float64{}
	}
	for id, value := range row.BenchmarkReturns {
		a.benchmarks[id] = append(a.benchmarks[id], value)
	}
}

func comparisonMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func comparisonMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	n := len(copyValues)
	if n%2 == 1 {
		return copyValues[n/2]
	}
	return (copyValues[n/2-1] + copyValues[n/2]) / 2
}

func comparisonBootstrapCI(values []float64, seed int64) (float64, float64) {
	if len(values) < 2 {
		return 0, 0
	}
	// Deterministic bootstrap keeps a saved report reproducible across reloads.
	n := len(values)
	means := make([]float64, 500)
	state := uint64(seed)
	if state == 0 {
		state = 1
	}
	for i := range means {
		sum := 0.0
		for j := 0; j < n; j++ {
			state ^= state << 13
			state ^= state >> 7
			state ^= state << 17
			sum += values[int(state%uint64(n))]
		}
		means[i] = sum / float64(n)
	}
	sort.Float64s(means)
	return means[12], means[487]
}

func (a *comparisonAccumulator) summary(seed int64) comparisonSummaryRow {
	n := len(a.signal)
	row := comparisonSummaryRow{Completed: n, BenchmarkAverage: map[string]float64{}}
	if n == 0 {
		return row
	}
	row.SignalAverage = comparisonMean(a.signal)
	row.SignalMedian = comparisonMedian(a.signal)
	row.ControlAverage = comparisonMean(a.control)
	row.ControlMedian = comparisonMedian(a.control)
	row.PairedAverage = comparisonMean(a.paired)
	row.PairedMedian = comparisonMedian(a.paired)
	wins := 0
	for _, v := range a.paired {
		if v > 0 {
			wins++
		}
	}
	row.PairedWinRate = float64(wins) / float64(n) * 100
	row.SignalTargetRate = float64(a.signalTargets) / float64(n) * 100
	row.ControlTargetRate = comparisonMean(a.controlTargetRates)
	row.SignalMFE = comparisonMean(a.signalMFE)
	row.ControlMFE = comparisonMean(a.controlMFE)
	row.SignalMAE = comparisonMean(a.signalMAE)
	row.ControlMAE = comparisonMean(a.controlMAE)
	row.ConfidenceLow, row.ConfidenceHigh = comparisonBootstrapCI(a.paired, seed)
	for id, values := range a.benchmarks {
		row.BenchmarkAverage[id] = comparisonMean(values)
	}
	return row
}

func normalizeHistoricalComparisonRequest(req historicalComparisonRequest, run HistoricalBacktestRun) (historicalComparisonRequest, error) {
	if req.Mode == "" {
		req.Mode = "strategy"
	}
	if req.Mode != "strategy" && req.Mode != "all_strategies" {
		return req, errors.New("分析模式无效")
	}
	if req.Mode == "strategy" {
		if req.StrategyID == "" && len(run.StrategyIDs) == 1 {
			req.StrategyID = run.StrategyIDs[0]
		}
		found := false
		for _, id := range run.StrategyIDs {
			if id == req.StrategyID {
				found = true
			}
		}
		if !found {
			return req, errors.New("策略不属于该回测")
		}
	} else {
		req.StrategyID = ""
	}
	if req.ControlSampleSize == 0 {
		req.ControlSampleSize = 5
	}
	if req.ControlSampleSize < 1 || req.ControlSampleSize > 20 {
		return req, errors.New("每条信号对照数量须为 1 至 20")
	}
	if req.TargetReturn == 0 {
		req.TargetReturn = 1
	}
	if req.TargetReturn < 0 || req.TargetReturn > 100 {
		return req, errors.New("收盘达标阈值须为 0 至 100%")
	}
	if req.DrawdownLimit < 0 || req.DrawdownLimit > 100 {
		return req, errors.New("最大不利波动限制须为 0 至 100%")
	}
	if len(req.BenchmarkIDs) == 0 {
		req.BenchmarkIDs = []string{"sh000001", "sh000300", "sh000905", "sh000852"}
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range req.BenchmarkIDs {
		if _, ok := comparisonBenchmarks[id]; !ok {
			return req, fmt.Errorf("不支持的指数基准: %s", id)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	req.BenchmarkIDs = ids
	if req.Seed == 0 {
		req.Seed = time.Now().UnixNano()
	}
	return req, nil
}

func handleHistoricalComparisonOperations(w http.ResponseWriter, r *http.Request, runID string, parts []string) {
	if len(parts) == 0 {
		switch r.Method {
		case http.MethodGet:
			items, err := appStore.ListHistoricalComparisons(runID)
			if err != nil {
				errorResponse(w, err.Error())
				return
			}
			successResponse(w, map[string]any{"items": items})
		case http.MethodPost:
			var req historicalComparisonRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				errorResponse(w, "请求参数错误: "+err.Error())
				return
			}
			item, err := automationRunner.startHistoricalComparison(runID, req)
			if err != nil {
				errorResponse(w, err.Error())
				return
			}
			successResponse(w, item)
		default:
			errorResponse(w, "只支持 GET 或 POST")
		}
		return
	}
	item, err := appStore.GetHistoricalComparison(runID, parts[0])
	if err != nil {
		errorResponse(w, "对照分析不存在")
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		successResponse(w, item)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		if err := appStore.RequestHistoricalComparisonCancel(runID, item.ID); err != nil {
			errorResponse(w, err.Error())
			return
		}
		updated, _ := appStore.GetHistoricalComparison(runID, item.ID)
		successResponse(w, updated)
		return
	}
	if len(parts) == 2 && parts[1] == "resample" && r.Method == http.MethodPost {
		var req historicalComparisonRequest
		if err := json.Unmarshal([]byte(item.ConfigJSON), &req); err != nil {
			errorResponse(w, "历史分析配置损坏")
			return
		}
		req.Resample = true
		req.Seed = 0
		created, err := automationRunner.startHistoricalComparison(runID, req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, created)
		return
	}
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page, err := appStore.ListHistoricalComparisonEvents(item.ID, limit, offset, r.URL.Query().Get("only_unavailable") == "true")
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, page)
		return
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodGet {
		exportHistoricalComparisonCSV(w, item)
		return
	}
	errorResponse(w, "不支持的对照分析操作")
}

func exportHistoricalComparisonCSV(w http.ResponseWriter, item workbench.HistoricalComparison) {
	rows, err := appStore.HistoricalComparisonEventRows(item.ID)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="comparison-%s.csv"`, item.ID))
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"signal_date", "signal_symbol", "strategy_ids", "status", "reason", "horizon", "stored_signal_base", "current_signal_base", "control_symbols", "signal_return_pct", "control_return_pct", "paired_excess_pct", "signal_mfe_pct", "control_mfe_pct", "signal_mae_pct", "control_mae_pct", "benchmark_returns_json"})
	for rows.Next() {
		var date, symbol, strategies, status, reason, raw string
		var controlCount int
		if err := rows.Scan(&date, &symbol, &strategies, &controlCount, &status, &reason, &raw); err != nil {
			break
		}
		var result comparisonEventResult
		_ = json.Unmarshal([]byte(raw), &result)
		controlSymbols := []string{}
		for _, c := range result.Controls {
			controlSymbols = append(controlSymbols, c.Symbol)
		}
		keys := make([]string, 0, len(result.Horizons))
		for key := range result.Horizons {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			h := result.Horizons[key]
			bench, _ := json.Marshal(h.BenchmarkReturns)
			_ = writer.Write([]string{date, symbol, strategies, status, reason, key, strconv.FormatFloat(result.StoredSignalBase, 'f', 4, 64), strconv.FormatFloat(result.CurrentSignalBase, 'f', 4, 64), strings.Join(controlSymbols, "|"), strconv.FormatFloat(h.SignalReturn, 'f', 4, 64), strconv.FormatFloat(h.ControlReturn, 'f', 4, 64), strconv.FormatFloat(h.PairedExcess, 'f', 4, 64), strconv.FormatFloat(h.SignalMFE, 'f', 4, 64), strconv.FormatFloat(h.ControlMFE, 'f', 4, 64), strconv.FormatFloat(h.SignalMAE, 'f', 4, 64), strconv.FormatFloat(h.ControlMAE, 'f', 4, 64), string(bench)})
		}
	}
	writer.Flush()
}

func (r *AutomationRunner) startHistoricalComparison(runID string, raw historicalComparisonRequest) (workbench.HistoricalComparison, error) {
	run, err := r.store.GetHistoricalBacktestRun(runID)
	if err != nil {
		return workbench.HistoricalComparison{}, errors.New("回测不存在")
	}
	if run.Status != "success" && run.SignalCount == 0 {
		return workbench.HistoricalComparison{}, errors.New("回测尚无可分析的信号")
	}
	req, err := normalizeHistoricalComparisonRequest(raw, run)
	if err != nil {
		return workbench.HistoricalComparison{}, err
	}
	if active, err := r.store.FindRunningHistoricalComparison(runID); err == nil {
		return active, nil
	}
	if !r.acquireRun("historical-comparison") {
		return workbench.HistoricalComparison{}, errors.New("已有对照分析正在执行")
	}
	configJSON := mustJSON(req)
	item, err := r.store.CreateHistoricalComparison(workbench.HistoricalComparison{RunID: runID, StrategyID: req.StrategyID, Mode: req.Mode, ConfigJSON: configJSON, Seed: req.Seed})
	if err != nil {
		r.releaseRun("historical-comparison")
		return item, err
	}
	go func() {
		defer r.releaseRun("historical-comparison")
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()
		summary, err := r.runHistoricalComparison(ctx, run, item, req)
		status, errorText := "completed", ""
		if err != nil {
			status = "failed"
			errorText = err.Error()
			if errors.Is(err, context.Canceled) || r.store.HistoricalComparisonCancelRequested(item.ID) {
				status = "cancelled"
				errorText = "任务已取消"
			}
		}
		_ = r.store.FinishHistoricalComparison(item.ID, status, mustJSON(summary), errorText)
	}()
	return item, nil
}

func comparisonSignalGroups(signals []HistoricalBacktestSignal) [][]HistoricalBacktestSignal {
	groups := [][]HistoricalBacktestSignal{}
	for _, s := range signals {
		n := len(groups)
		if n == 0 || groups[n-1][0].SignalDate != s.SignalDate || groups[n-1][0].Symbol != s.Symbol {
			groups = append(groups, []HistoricalBacktestSignal{s})
		} else {
			groups[n-1] = append(groups[n-1], s)
		}
	}
	return groups
}

func comparisonSample(symbols []string, excluded map[string]bool, n int, seed int64, key string) []string {
	if len(symbols) == 0 || n <= 0 {
		return nil
	}
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d:%s", seed, key)
	value := h.Sum64()
	start := int(value % uint64(len(symbols)))
	step := int((value>>32)%uint64(len(symbols))) + 1
	for gcdComparison(step, len(symbols)) != 1 {
		step++
	}
	result := make([]string, 0, n)
	for i := 0; i < len(symbols) && len(result) < n; i++ {
		symbol := symbols[(start+i*step)%len(symbols)]
		if !excluded[symbol] {
			result = append(result, symbol)
		}
	}
	return result
}

func gcdComparison(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func comparisonBarIndex(rows []FormulaKline, date int) int {
	i := sort.Search(len(rows), func(i int) bool { return rows[i].Date >= date })
	if i < len(rows) && rows[i].Date == date {
		return i
	}
	return -1
}

func comparisonValidBar(bar FormulaKline) bool {
	return bar.Open > 0 && bar.Close > 0 && bar.High >= math.Max(bar.Open, bar.Close) && bar.Low > 0 && bar.Low <= math.Min(bar.Open, bar.Close)
}

func comparisonMetrics(rows []FormulaKline, date int, baseOverride float64, horizons []int, target, drawdown float64) map[string]workbench.SelectionHorizon {
	result := map[string]workbench.SelectionHorizon{}
	i := comparisonBarIndex(rows, date)
	if i < 0 || !comparisonValidBar(rows[i]) {
		return result
	}
	base := rows[i].Close
	if baseOverride > 0 {
		base = baseOverride
	}
	maxH := 0
	for _, h := range horizons {
		if h > maxH {
			maxH = h
		}
	}
	forward := make([]FormulaKline, 0, maxH)
	for j := i + 1; j < len(rows) && len(forward) < maxH; j++ {
		if comparisonValidBar(rows[j]) {
			forward = append(forward, rows[j])
		}
	}
	for _, h := range horizons {
		key := fmt.Sprintf("d%d", h)
		metric := workbench.SelectionHorizon{HorizonDays: h, Status: "pending", TargetReturn: target, DrawdownLimit: drawdown}
		if h <= 0 || len(forward) < h {
			result[key] = metric
			continue
		}
		window := forward[:h]
		final := window[h-1]
		peak := window[0].High
		trough := window[0].Low
		for _, bar := range window {
			if bar.High > peak {
				peak = bar.High
			}
			if bar.Low < trough {
				trough = bar.Low
			}
		}
		percent := func(price float64) float64 { return math.Round((price/base-1)*10000) / 100 }
		metric.Status = "complete"
		metric.FinalDate = final.Date
		metric.FinalClose = final.Close
		metric.MaxGain = percent(peak)
		metric.MaxDrawdown = percent(trough)
		metric.CloseReturn = percent(final.Close)
		metric.Success = metric.CloseReturn >= target && (drawdown == 0 || metric.MaxDrawdown >= -drawdown)
		result[key] = metric
	}
	return result
}

func (r *AutomationRunner) comparisonUniverse(run HistoricalBacktestRun, req historicalComparisonRequest) ([]string, error) {
	var snapshots []Strategy
	if err := json.Unmarshal([]byte(run.StrategySnapshotJSON), &snapshots); err != nil || len(snapshots) == 0 {
		return nil, errors.New("回测缺少策略配置快照，无法重建候选范围")
	}
	requested, err := r.historicalBacktestUniverse(run.UniversePoolIDs)
	if err != nil {
		return nil, err
	}
	groups := [][]string{}
	for _, strategy := range snapshots {
		if req.StrategyID != "" && strategy.ID != req.StrategyID {
			continue
		}
		var config StrategyConfig
		if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
			return nil, err
		}
		normalizeSystemStrategyConfig(&config)
		symbols, err := r.strategyUniverse(config)
		if err != nil {
			return nil, err
		}
		groups = append(groups, intersectHistoricalSymbols(symbols, requested))
	}
	if len(groups) == 0 {
		return nil, errors.New("没有可用的策略快照")
	}
	return mergeHistoricalSymbols(groups...), nil
}

func (r *AutomationRunner) runHistoricalComparison(ctx context.Context, run HistoricalBacktestRun, job workbench.HistoricalComparison, req historicalComparisonRequest) (comparisonSummary, error) {
	summary := comparisonSummary{HorizonRows: map[string]comparisonSummaryRow{}, Years: map[string]map[string]comparisonSummaryRow{}, Warnings: []string{"候选股票池按当前股票列表重建，历史退市股票和历史成分变化可能缺失；对照结果存在幸存者偏差"}}
	signals, err := r.store.ListHistoricalComparisonSignals(run.ID, req.StrategyID)
	if err != nil {
		return summary, err
	}
	groups := comparisonSignalGroups(signals)
	summary.EventCount = len(groups)
	if len(groups) == 0 {
		return summary, errors.New("没有可分析的信号")
	}
	symbols, err := r.comparisonUniverse(run, req)
	if err != nil {
		return summary, err
	}
	symbols = mergeHistoricalSymbols(symbols, func() []string {
		out := []string{}
		for _, s := range signals {
			out = append(out, s.Symbol)
		}
		return out
	}())
	summary.CandidateSymbols = len(symbols)
	start, _ := time.ParseInLocation("2006-01-02", run.StartDate, time.Local)
	end, _ := time.ParseInLocation("2006-01-02", run.EndDate, time.Local)
	maxHorizon := 10
	for _, h := range run.Horizons {
		if h > maxHorizon {
			maxHorizon = h
		}
	}
	count := historicalHistoryCount(start, end, time.Now().In(time.Local), maxHorizon)
	klines, failures := r.loadSystemBatchKlines(ctx, symbols, count)
	if len(failures) > 0 {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("%d 只股票行情不可用，已从对应事件或对照候选中剔除", len(failures)))
	}
	indices, indexFailures := r.loadSystemBatchKlinesWithRecover(ctx, req.BenchmarkIDs, count, "none")
	if len(indexFailures) > 0 {
		summary.Warnings = append(summary.Warnings, "部分指数行情不可用，对应指数收益留空")
	}
	// The market-service batch API can fall back to TDX; it currently does not
	// identify the fallback symbol. Disclose this limitation instead of claiming
	// every price came from Hikyuu.
	summary.Warnings = append(summary.Warnings, "行情通过 market-service 批量接口读取；该接口未返回逐只股票的数据源标记，无法独立确认回退股票")
	if err := r.store.UpdateHistoricalComparisonProgress(job.ID, len(groups), 0, ""); err != nil {
		return summary, err
	}
	acc := map[string]*comparisonAccumulator{}
	yearAcc := map[string]map[string]*comparisonAccumulator{}
	dateCandidates := []string{}
	currentDate := ""
	excludedByDate := map[string]map[string]bool{}
	for _, s := range signals {
		if req.Mode == "strategy" && s.StrategyID != req.StrategyID {
			continue
		}
		if excludedByDate[s.SignalDate] == nil {
			excludedByDate[s.SignalDate] = map[string]bool{}
		}
		excludedByDate[s.SignalDate][s.Symbol] = true
	}
	for groupIndex, group := range groups {
		if ctx.Err() != nil || r.store.HistoricalComparisonCancelRequested(job.ID) {
			return summary, context.Canceled
		}
		first := group[0]
		dateInt, _ := strconv.Atoi(strings.ReplaceAll(first.SignalDate, "-", ""))
		if first.SignalDate != currentDate {
			currentDate = first.SignalDate
			dateCandidates = dateCandidates[:0]
			for _, symbol := range symbols {
				rows := klines[symbol]
				i := comparisonBarIndex(rows, dateInt)
				if i >= 0 && comparisonValidBar(rows[i]) {
					dateCandidates = append(dateCandidates, symbol)
				}
			}
		}
		excluded := excludedByDate[first.SignalDate]
		controls := comparisonSample(dateCandidates, excluded, req.ControlSampleSize, req.Seed, first.SignalDate+":"+first.Symbol+":"+req.StrategyID)
		result := comparisonEventResult{StoredSignalBase: first.Latest, CandidateCount: len(dateCandidates), Controls: []comparisonControl{}, Horizons: map[string]comparisonHorizon{}}
		strategyIDs := []string{}
		signalIDs := []string{}
		for _, s := range group {
			strategyIDs = append(strategyIDs, s.StrategyID)
			signalIDs = append(signalIDs, s.ID)
		}
		status, reason := "complete", ""
		signalRows := klines[first.Symbol]
		signalIndex := comparisonBarIndex(signalRows, dateInt)
		if signalIndex < 0 || !comparisonValidBar(signalRows[signalIndex]) {
			status = "unavailable"
			reason = "信号日当前行情缺失"
		} else {
			result.CurrentSignalBase = signalRows[signalIndex].Close
			result.BaseChanged = math.Abs(result.CurrentSignalBase-result.StoredSignalBase) > 0.0011
			if result.BaseChanged {
				summary.BaseChangedEvents++
				status = "partial"
				reason = "当前前复权信号价与原回测保存值不同；本报告按当前行情统一重算"
			}
		}
		if len(controls) < req.ControlSampleSize && status == "complete" {
			status = "partial"
			reason = "同日可用对照股票不足"
		}
		var signalMetrics map[string]workbench.SelectionHorizon
		if status != "unavailable" {
			signalMetrics = comparisonMetrics(signalRows, dateInt, first.Latest, run.Horizons, req.TargetReturn, req.DrawdownLimit)
		}
		for _, symbol := range controls {
			rows := klines[symbol]
			index := comparisonBarIndex(rows, dateInt)
			if index < 0 {
				continue
			}
			metrics := comparisonMetrics(rows, dateInt, rows[index].Close, run.Horizons, req.TargetReturn, req.DrawdownLimit)
			result.Controls = append(result.Controls, comparisonControl{Symbol: symbol, Base: rows[index].Close, Horizons: metrics})
		}
		for _, h := range run.Horizons {
			key := fmt.Sprintf("d%d", h)
			out := comparisonHorizon{Status: "unavailable", BenchmarkReturns: map[string]float64{}}
			signalH := signalMetrics[key]
			returns, mfes, maes := []float64{}, []float64{}, []float64{}
			hits := 0
			for _, control := range result.Controls {
				metric := control.Horizons[key]
				if metric.Status != "complete" {
					continue
				}
				returns = append(returns, metric.CloseReturn)
				mfes = append(mfes, metric.MaxGain)
				maes = append(maes, metric.MaxDrawdown)
				if metric.CloseReturn >= req.TargetReturn && (req.DrawdownLimit == 0 || metric.MaxDrawdown >= -req.DrawdownLimit) {
					hits++
				}
			}
			if signalH.Status == "complete" && len(returns) > 0 {
				out.Status = "complete"
				out.SignalClose = signalH.FinalClose
				out.SignalReturn = signalH.CloseReturn
				out.SignalMFE = signalH.MaxGain
				out.SignalMAE = signalH.MaxDrawdown
				out.SignalTargetHit = signalH.CloseReturn >= req.TargetReturn && (req.DrawdownLimit == 0 || signalH.MaxDrawdown >= -req.DrawdownLimit)
				out.ControlReturn = comparisonMean(returns)
				out.ControlMedian = comparisonMedian(returns)
				out.ControlMFE = comparisonMean(mfes)
				out.ControlMAE = comparisonMean(maes)
				out.ControlCount = len(returns)
				out.PairedExcess = out.SignalReturn - out.ControlReturn
				out.ControlTargetRate = float64(hits) / float64(len(returns)) * 100
			}
			for _, id := range req.BenchmarkIDs {
				rows := indices[id]
				i := comparisonBarIndex(rows, dateInt)
				if i < 0 || i+h >= len(rows) || !comparisonValidBar(rows[i]) || !comparisonValidBar(rows[i+h]) {
					continue
				}
				out.BenchmarkReturns[id] = (rows[i+h].Close/rows[i].Close - 1) * 100
			}
			result.Horizons[key] = out
			if out.Status == "complete" {
				if acc[key] == nil {
					acc[key] = &comparisonAccumulator{}
				}
				acc[key].add(out)
				year := first.SignalDate[:4]
				if yearAcc[year] == nil {
					yearAcc[year] = map[string]*comparisonAccumulator{}
				}
				if yearAcc[year][key] == nil {
					yearAcc[year][key] = &comparisonAccumulator{}
				}
				yearAcc[year][key].add(out)
			}
		}
		if status == "complete" {
			summary.CompletedEvents++
		} else if status == "partial" {
			summary.PartialEvents++
		} else {
			summary.UnavailableEvents++
		}
		summary.ControlCount += len(result.Controls)
		if err := r.store.InsertHistoricalComparisonEvent(workbench.HistoricalComparisonEvent{ComparisonID: job.ID, SignalDate: first.SignalDate, SignalSymbol: first.Symbol, StrategyIDs: strategyIDs, SignalIDs: signalIDs, ControlCount: len(result.Controls), Status: status, Reason: reason, ResultJSON: mustJSON(result)}); err != nil {
			return summary, err
		}
		if groupIndex%10 == 0 || groupIndex+1 == len(groups) {
			if err := r.store.UpdateHistoricalComparisonProgress(job.ID, len(groups), groupIndex+1, currentDate); err != nil {
				return summary, err
			}
		}
	}
	for key, item := range acc {
		summary.HorizonRows[key] = item.summary(req.Seed)
	}
	for year, rows := range yearAcc {
		summary.Years[year] = map[string]comparisonSummaryRow{}
		for key, item := range rows {
			summary.Years[year][key] = item.summary(req.Seed)
		}
	}
	return summary, nil
}
