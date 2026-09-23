package workbench

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type DailyReviewBatchSummary struct {
	ID               string `json:"id"`
	Date             string `json:"date"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	Error            string `json:"error,omitempty"`
	MatchedCount     int    `json:"matched_count"`
	StrategyCount    int    `json:"strategy_count"`
	FailedStrategies int    `json:"failed_strategies"`
	CandidateSymbols int    `json:"candidate_symbols"`
	KlineLoaded      int    `json:"kline_loaded"`
	KlineFailed      int    `json:"kline_failed"`
	DurationMS       int64  `json:"duration_ms"`
}

type DailyReviewStrategySummary struct {
	RunID        string `json:"run_id,omitempty"`
	StrategyID   string `json:"strategy_id"`
	StrategyName string `json:"strategy_name"`
	Status       string `json:"status"`
	Matched      int    `json:"matched"`
	Errors       int    `json:"errors"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	DurationMS   int64  `json:"duration_ms"`
}

type DailyReviewSignal struct {
	ID           string             `json:"id"`
	RunID        string             `json:"run_id"`
	StrategyID   string             `json:"strategy_id"`
	StrategyName string             `json:"strategy_name"`
	SignalDate   string             `json:"signal_date"`
	Symbol       string             `json:"symbol"`
	Latest       float64            `json:"latest"`
	Score        float64            `json:"score"`
	DetailJSON   string             `json:"detail_json"`
	Tracking     *SelectionTracking `json:"tracking,omitempty"`
	CreatedAt    string             `json:"created_at"`
}

type DailyReviewStock struct {
	SignalDate    string              `json:"signal_date"`
	Symbol        string              `json:"symbol"`
	Latest        float64             `json:"latest"`
	StrategyCount int                 `json:"strategy_count"`
	StrategyIDs   []string            `json:"strategy_ids"`
	Strategies    []string            `json:"strategies"`
	MaxScore      float64             `json:"max_score"`
	AverageScore  float64             `json:"average_score"`
	Tracking      *SelectionTracking  `json:"tracking,omitempty"`
	Status        string              `json:"status"`
	Watch         bool                `json:"watch"`
	Excluded      bool                `json:"excluded"`
	Note          DecisionNote        `json:"note"`
	Signals       []DailyReviewSignal `json:"signals"`
}

type DailyReviewSummary struct {
	StrategyCount    int   `json:"strategy_count"`
	RawSignals       int   `json:"raw_signals"`
	StockCount       int   `json:"stock_count"`
	ConsensusCount   int   `json:"consensus_count"`
	WatchCount       int   `json:"watch_count"`
	ExcludeCount     int   `json:"exclude_count"`
	CandidateSymbols int   `json:"candidate_symbols"`
	KlineLoaded      int   `json:"kline_loaded"`
	KlineFailed      int   `json:"kline_failed"`
	DurationMS       int64 `json:"duration_ms"`
}

type DailyReviewData struct {
	Date          string                       `json:"date"`
	SelectedBatch *DailyReviewBatchSummary     `json:"selected_batch,omitempty"`
	Batches       []DailyReviewBatchSummary    `json:"batches"`
	Summary       DailyReviewSummary           `json:"summary"`
	Strategies    []DailyReviewStrategySummary `json:"strategies"`
	Stocks        []DailyReviewStock           `json:"stocks"`
	Signals       []DailyReviewSignal          `json:"signals"`
	Consensus     []DailyReviewStock           `json:"consensus"`
	Watch         []string                     `json:"watch"`
	Exclude       []string                     `json:"exclude"`
	Notes         []DecisionNote               `json:"notes"`
}

type DailyStrategyAnalysisMetric struct {
	StrategyID          string  `json:"strategy_id"`
	StrategyName        string  `json:"strategy_name"`
	RunDays             int     `json:"run_days"`
	SignalCount         int     `json:"signal_count"`
	AverageDailySignals float64 `json:"average_daily_signals"`
	UniqueSymbols       int     `json:"unique_symbols"`
	AverageScore        float64 `json:"average_score"`
	ConsensusSignals    int     `json:"consensus_signals"`
	D1Completed         int     `json:"d1_completed"`
	D1SuccessCount      int     `json:"d1_success_count"`
	D1SuccessRate       float64 `json:"d1_success_rate"`
	D1PositiveCount     int     `json:"d1_positive_count"`
	D1PositiveRate      float64 `json:"d1_positive_rate"`
	D1AverageReturn     float64 `json:"d1_average_return"`
	D1MaxReturn         float64 `json:"d1_max_return"`
	D3Completed         int     `json:"d3_completed"`
	D3SuccessCount      int     `json:"d3_success_count"`
	D3SuccessRate       float64 `json:"d3_success_rate"`
	D3PositiveCount     int     `json:"d3_positive_count"`
	D3PositiveRate      float64 `json:"d3_positive_rate"`
	D3AverageReturn     float64 `json:"d3_average_return"`
	D3MaxReturn         float64 `json:"d3_max_return"`
	D5Completed         int     `json:"d5_completed"`
	D5SuccessCount      int     `json:"d5_success_count"`
	D5SuccessRate       float64 `json:"d5_success_rate"`
	D5PositiveCount     int     `json:"d5_positive_count"`
	D5PositiveRate      float64 `json:"d5_positive_rate"`
	D5AverageReturn     float64 `json:"d5_average_return"`
	D5MaxReturn         float64 `json:"d5_max_return"`
	D10Completed        int     `json:"d10_completed"`
	D10SuccessCount     int     `json:"d10_success_count"`
	D10SuccessRate      float64 `json:"d10_success_rate"`
	D10PositiveCount    int     `json:"d10_positive_count"`
	D10PositiveRate     float64 `json:"d10_positive_rate"`
	D10AverageReturn    float64 `json:"d10_average_return"`
	D10MaxReturn        float64 `json:"d10_max_return"`
}

type DailyStrategyAnalysisData struct {
	StartDate      string                        `json:"start_date"`
	EndDate        string                        `json:"end_date"`
	TradingDays    int                           `json:"trading_days"`
	StrategyCount  int                           `json:"strategy_count"`
	SignalCount    int                           `json:"signal_count"`
	UniqueSymbols  int                           `json:"unique_symbols"`
	ConsensusCount int                           `json:"consensus_count"`
	AverageScore   float64                       `json:"average_score"`
	Strategies     []DailyStrategyAnalysisMetric `json:"strategies"`
	Stocks         []DailyReviewStock            `json:"stocks"`
	Signals        []DailyReviewSignal           `json:"signals"`
	Consensus      []DailyReviewStock            `json:"consensus"`
}

type dailyAnalysisSignal struct {
	date, symbol, strategyID string
	score                    float64
	tracking                 *SelectionTracking
}

type dailyBatchResult struct {
	Strategies       []dailyBatchStrategy `json:"strategies"`
	StrategyCount    int                  `json:"strategy_count"`
	FailedStrategies int                  `json:"failed_strategies"`
	CandidateSymbols int                  `json:"candidate_symbols"`
	KlineLoaded      int                  `json:"kline_loaded"`
	KlineFailed      int                  `json:"kline_failed"`
}

type dailyBatchStrategy struct {
	StrategyID string `json:"strategy_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Matched    int    `json:"matched"`
	Errors     int    `json:"errors"`
	Error      string `json:"error"`
}

func (s *AppStore) ListDailyReviewBatches(limit int) ([]DailyReviewBatchSummary, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	runs, err := s.ListAutomationRuns(SystemStrategyDailyBatchTaskID, limit)
	if err != nil {
		return nil, err
	}
	items := make([]DailyReviewBatchSummary, 0, len(runs))
	for _, run := range runs {
		items = append(items, dailyReviewBatchSummary(run))
	}
	return items, nil
}

func dailyReviewBatchSummary(run AutomationRun) DailyReviewBatchSummary {
	var result dailyBatchResult
	_ = json.Unmarshal([]byte(run.ResultJSON), &result)
	strategyCount := result.StrategyCount
	if strategyCount == 0 {
		strategyCount = len(result.Strategies)
	}
	return DailyReviewBatchSummary{
		ID: run.ID, Date: textDate(run.StartedAt), Status: run.Status,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Error: run.Log,
		MatchedCount: run.MatchedCount, StrategyCount: strategyCount,
		FailedStrategies: result.FailedStrategies, CandidateSymbols: result.CandidateSymbols,
		KlineLoaded: result.KlineLoaded, KlineFailed: result.KlineFailed,
		DurationMS: elapsedMilliseconds(run.StartedAt, run.FinishedAt),
	}
}

func (s *AppStore) BuildDailyReview(date, batchID string, limit int) (DailyReviewData, error) {
	batches, err := s.ListDailyReviewBatches(40)
	if err != nil {
		return DailyReviewData{}, err
	}
	data := DailyReviewData{
		Batches: batches, Strategies: []DailyReviewStrategySummary{}, Stocks: []DailyReviewStock{},
		Signals: []DailyReviewSignal{}, Consensus: []DailyReviewStock{}, Watch: []string{}, Exclude: []string{}, Notes: []DecisionNote{},
	}
	var selected *DailyReviewBatchSummary
	for index := range batches {
		candidate := &batches[index]
		if batchID != "" && candidate.ID == batchID {
			selected = candidate
			break
		}
		if batchID == "" && (date == "" || candidate.Date == date) {
			selected = candidate
			break
		}
	}
	if selected == nil {
		if batchID != "" || date != "" {
			return data, sql.ErrNoRows
		}
		return data, nil
	}
	selectedCopy := *selected
	data.SelectedBatch = &selectedCopy
	data.Date = selected.Date

	parentRun, err := s.GetAutomationRun(selected.ID)
	if err != nil {
		return data, err
	}
	children, err := s.ListAutomationChildRuns(parentRun)
	if err != nil {
		return data, err
	}
	strategies := buildDailyStrategySummaries(parentRun, children)
	runIDs := make([]string, 0, len(children))
	for _, child := range children {
		runIDs = append(runIDs, child.ID)
	}
	results, err := s.ListSelectionResultsByRunIDs(runIDs, limit)
	if err != nil {
		return data, err
	}
	notes, err := s.ListDecisionNotes("", 5000)
	if err != nil {
		return data, err
	}
	noteMap := make(map[string]DecisionNote, len(notes))
	for _, note := range notes {
		noteMap[strings.ToUpper(note.Symbol)] = note
	}
	watchPool, _ := s.GetStockPool(DecisionWatchPoolID)
	excludePool, _ := s.GetStockPool(DecisionExcludePoolID)
	watchSet := stringSet(watchPool.Symbols)
	excludeSet := stringSet(excludePool.Symbols)

	strategyIDsByRun := make(map[string]string, len(children))
	for _, child := range children {
		strategyIDsByRun[child.ID] = strings.TrimPrefix(child.TaskID, "system-strategy:")
	}
	signals := make([]DailyReviewSignal, 0, len(results))
	stockMap := map[string]*DailyReviewStock{}
	for _, item := range results {
		symbol := strings.ToUpper(item.Symbol)
		signal := DailyReviewSignal{
			ID: item.ID, RunID: item.RunID, StrategyID: strategyIDsByRun[item.RunID], StrategyName: item.FormulaName,
			SignalDate: selected.Date, Symbol: symbol, Latest: item.Latest, Score: selectionResultScore(item.DetailJSON),
			DetailJSON: item.DetailJSON, Tracking: item.Tracking, CreatedAt: item.CreatedAt,
		}
		signals = append(signals, signal)
		stock := stockMap[symbol]
		if stock == nil {
			status := noteMap[symbol].Status
			if excludeSet[symbol] {
				status = "exclude"
			} else if watchSet[symbol] {
				status = "watch"
			}
			stock = &DailyReviewStock{
				SignalDate: selected.Date, Symbol: symbol, Latest: item.Latest, MaxScore: signal.Score, Tracking: item.Tracking,
				Status: status, Watch: watchSet[symbol], Excluded: excludeSet[symbol], Note: noteMap[symbol],
				StrategyIDs: []string{}, Strategies: []string{}, Signals: []DailyReviewSignal{},
			}
			stockMap[symbol] = stock
		}
		stock.Signals = append(stock.Signals, signal)
		stock.StrategyIDs = appendUnique(stock.StrategyIDs, signal.StrategyID)
		stock.Strategies = appendUnique(stock.Strategies, signal.StrategyName)
		if signal.Score > stock.MaxScore {
			stock.MaxScore = signal.Score
		}
		if trackingCompleteness(item.Tracking) > trackingCompleteness(stock.Tracking) {
			stock.Tracking = item.Tracking
		}
	}

	stocks := make([]DailyReviewStock, 0, len(stockMap))
	consensus := []DailyReviewStock{}
	watchCount, excludeCount := 0, 0
	for _, stock := range stockMap {
		stock.StrategyCount = len(stock.StrategyIDs)
		totalScore := 0.0
		for _, signal := range stock.Signals {
			totalScore += signal.Score
		}
		if len(stock.Signals) > 0 {
			stock.AverageScore = roundDailyFloat(totalScore / float64(len(stock.Signals)))
		}
		stock.MaxScore = roundDailyFloat(stock.MaxScore)
		if stock.Watch {
			watchCount++
		}
		if stock.Excluded {
			excludeCount++
		}
		stocks = append(stocks, *stock)
	}
	sort.Slice(stocks, func(i, j int) bool {
		if stocks[i].StrategyCount == stocks[j].StrategyCount {
			if stocks[i].MaxScore == stocks[j].MaxScore {
				return stocks[i].Symbol < stocks[j].Symbol
			}
			return stocks[i].MaxScore > stocks[j].MaxScore
		}
		return stocks[i].StrategyCount > stocks[j].StrategyCount
	})
	for _, stock := range stocks {
		if stock.StrategyCount >= 2 {
			consensus = append(consensus, stock)
		}
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Score == signals[j].Score {
			return signals[i].Symbol < signals[j].Symbol
		}
		return signals[i].Score > signals[j].Score
	})

	data.Strategies = strategies
	data.Stocks = stocks
	data.Signals = signals
	data.Consensus = consensus
	data.Watch = watchPool.Symbols
	data.Exclude = excludePool.Symbols
	data.Notes = notes
	data.Summary = DailyReviewSummary{
		StrategyCount: len(strategies), RawSignals: len(signals), StockCount: len(stocks), ConsensusCount: len(consensus),
		WatchCount: watchCount, ExcludeCount: excludeCount, CandidateSymbols: selected.CandidateSymbols,
		KlineLoaded: selected.KlineLoaded, KlineFailed: selected.KlineFailed, DurationMS: selected.DurationMS,
	}
	return data, nil
}

func (s *AppStore) BuildDailyStrategyAnalysis(startDate, endDate string, days, limit int) (DailyStrategyAnalysisData, error) {
	if limit <= 0 || limit > 20000 {
		limit = 20000
	}
	batches, err := s.ListDailyReviewBatches(200)
	if err != nil {
		return DailyStrategyAnalysisData{}, err
	}
	selected := make([]DailyReviewBatchSummary, 0, len(batches))
	selectedDates := map[string]bool{}
	for _, batch := range batches {
		if batch.Status != "success" && batch.Status != "failed" {
			continue
		}
		if startDate != "" && batch.Date < startDate {
			continue
		}
		if endDate != "" && batch.Date > endDate {
			continue
		}
		if selectedDates[batch.Date] {
			continue
		}
		selectedDates[batch.Date] = true
		selected = append(selected, batch)
	}
	if startDate == "" && days > 0 && len(selected) > days {
		selected = selected[:days]
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Date < selected[j].Date })
	if len(selected) == 0 {
		return DailyStrategyAnalysisData{
			StartDate: startDate, EndDate: endDate, Strategies: []DailyStrategyAnalysisMetric{},
			Stocks: []DailyReviewStock{}, Signals: []DailyReviewSignal{}, Consensus: []DailyReviewStock{},
		}, nil
	}
	if startDate == "" {
		startDate = selected[0].Date
	}
	if endDate == "" {
		endDate = selected[len(selected)-1].Date
	}
	strategyDays := map[string]map[string]bool{}
	strategyNames := map[string]string{}
	strategySignals := map[string][]dailyAnalysisSignal{}
	daySignals := map[string][]dailyAnalysisSignal{}
	reviewSignals := []DailyReviewSignal{}
	for _, batch := range selected {
		parent, getErr := s.GetAutomationRun(batch.ID)
		if getErr != nil {
			return DailyStrategyAnalysisData{}, getErr
		}
		children, childErr := s.ListAutomationChildRuns(parent)
		if childErr != nil {
			return DailyStrategyAnalysisData{}, childErr
		}
		runIDs := make([]string, 0, len(children))
		strategyByRun := map[string]AutomationRun{}
		for _, child := range children {
			runIDs = append(runIDs, child.ID)
			strategyByRun[child.ID] = child
			strategyID := strings.TrimPrefix(child.TaskID, "system-strategy:")
			strategyDays[strategyID] = ensureBoolSet(strategyDays[strategyID])
			strategyDays[strategyID][batch.Date] = true
			strategyNames[strategyID] = child.TaskName
		}
		results, resultErr := s.ListSelectionResultsByRunIDs(runIDs, limit)
		if resultErr != nil {
			return DailyStrategyAnalysisData{}, resultErr
		}
		for _, item := range results {
			child := strategyByRun[item.RunID]
			strategyID := strings.TrimPrefix(child.TaskID, "system-strategy:")
			strategyName := item.FormulaName
			if strategyName == "" {
				strategyName = child.TaskName
			}
			strategyNames[strategyID] = strategyName
			symbol := strings.ToUpper(item.Symbol)
			score := selectionResultScore(item.DetailJSON)
			reviewSignal := DailyReviewSignal{
				ID: item.ID, RunID: item.RunID, StrategyID: strategyID, StrategyName: strategyName,
				SignalDate: batch.Date, Symbol: symbol, Latest: item.Latest, Score: score,
				DetailJSON: item.DetailJSON, Tracking: item.Tracking, CreatedAt: item.CreatedAt,
			}
			reviewSignals = append(reviewSignals, reviewSignal)
			s := dailyAnalysisSignal{date: batch.Date, symbol: symbol, strategyID: strategyID, score: score, tracking: item.Tracking}
			strategySignals[strategyID] = append(strategySignals[strategyID], s)
			daySignals[batch.Date] = append(daySignals[batch.Date], s)
		}
	}
	metrics := make([]DailyStrategyAnalysisMetric, 0, len(strategyNames))
	allSymbols := map[string]bool{}
	consensus := 0
	for _, signals := range daySignals {
		seen := map[string]map[string]bool{}
		for _, item := range signals {
			if seen[item.symbol] == nil {
				seen[item.symbol] = map[string]bool{}
			}
			seen[item.symbol][item.strategyID] = true
			allSymbols[item.symbol] = true
		}
		for _, strategyIDs := range seen {
			if len(strategyIDs) >= 2 {
				consensus++
			}
		}
	}
	var totalScore float64
	for strategyID, strategyName := range strategyNames {
		signals := strategySignals[strategyID]
		metric := DailyStrategyAnalysisMetric{StrategyID: strategyID, StrategyName: strategyName, RunDays: len(strategyDays[strategyID]), SignalCount: len(signals), UniqueSymbols: 0}
		symbols := map[string]bool{}
		for _, item := range signals {
			symbols[item.symbol] = true
			totalScore += item.score
		}
		metric.UniqueSymbols = len(symbols)
		metric.AverageDailySignals = roundDailyFloat(float64(metric.SignalCount) / float64(maxInt(metric.RunDays, 1)))
		metric.AverageScore = roundDailyFloat(averageSignalScore(signals))
		metric.ConsensusSignals = strategyConsensusCount(signals, daySignals)
		d1 := aggregateTrackingStats(signals, "d1")
		metric.D1Completed, metric.D1SuccessCount, metric.D1SuccessRate = d1.completed, d1.successCount, d1.successRate
		metric.D1PositiveCount, metric.D1PositiveRate, metric.D1AverageReturn, metric.D1MaxReturn = d1.positiveCount, d1.positiveRate, d1.averageReturn, d1.maxReturn
		d3 := aggregateTrackingStats(signals, "d3")
		metric.D3Completed, metric.D3SuccessCount, metric.D3SuccessRate = d3.completed, d3.successCount, d3.successRate
		metric.D3PositiveCount, metric.D3PositiveRate, metric.D3AverageReturn, metric.D3MaxReturn = d3.positiveCount, d3.positiveRate, d3.averageReturn, d3.maxReturn
		d5 := aggregateTrackingStats(signals, "d5")
		metric.D5Completed, metric.D5SuccessCount, metric.D5SuccessRate = d5.completed, d5.successCount, d5.successRate
		metric.D5PositiveCount, metric.D5PositiveRate, metric.D5AverageReturn, metric.D5MaxReturn = d5.positiveCount, d5.positiveRate, d5.averageReturn, d5.maxReturn
		d10 := aggregateTrackingStats(signals, "d10")
		metric.D10Completed, metric.D10SuccessCount, metric.D10SuccessRate = d10.completed, d10.successCount, d10.successRate
		metric.D10PositiveCount, metric.D10PositiveRate, metric.D10AverageReturn, metric.D10MaxReturn = d10.positiveCount, d10.positiveRate, d10.averageReturn, d10.maxReturn
		metrics = append(metrics, metric)
	}
	sort.Slice(metrics, func(i, j int) bool {
		if metrics[i].D5SuccessRate == metrics[j].D5SuccessRate {
			return metrics[i].SignalCount > metrics[j].SignalCount
		}
		return metrics[i].D5SuccessRate > metrics[j].D5SuccessRate
	})
	notes, err := s.ListDecisionNotes("", 5000)
	if err != nil {
		return DailyStrategyAnalysisData{}, err
	}
	noteMap := make(map[string]DecisionNote, len(notes))
	for _, note := range notes {
		noteMap[strings.ToUpper(note.Symbol)] = note
	}
	watchPool, _ := s.GetStockPool(DecisionWatchPoolID)
	excludePool, _ := s.GetStockPool(DecisionExcludePoolID)
	stocks, consensusStocks := buildDailyAnalysisStocks(reviewSignals, noteMap, stringSet(watchPool.Symbols), stringSet(excludePool.Symbols))
	sort.Slice(reviewSignals, func(i, j int) bool {
		if reviewSignals[i].SignalDate == reviewSignals[j].SignalDate {
			if reviewSignals[i].Score == reviewSignals[j].Score {
				return reviewSignals[i].Symbol < reviewSignals[j].Symbol
			}
			return reviewSignals[i].Score > reviewSignals[j].Score
		}
		return reviewSignals[i].SignalDate > reviewSignals[j].SignalDate
	})
	return DailyStrategyAnalysisData{
		StartDate: startDate, EndDate: endDate, TradingDays: len(selected), StrategyCount: len(metrics),
		SignalCount: sumAnalysisSignals(daySignals), UniqueSymbols: len(allSymbols), ConsensusCount: consensus,
		AverageScore: roundDailyFloat(totalScore / float64(maxInt(sumAnalysisSignals(daySignals), 1))), Strategies: metrics,
		Stocks: stocks, Signals: reviewSignals, Consensus: consensusStocks,
	}, nil
}

func buildDailyAnalysisStocks(signals []DailyReviewSignal, notes map[string]DecisionNote, watch, exclude map[string]bool) ([]DailyReviewStock, []DailyReviewStock) {
	stockMap := map[string]*DailyReviewStock{}
	for _, signal := range signals {
		key := signal.SignalDate + "\x00" + signal.Symbol
		stock := stockMap[key]
		if stock == nil {
			status := notes[signal.Symbol].Status
			if exclude[signal.Symbol] {
				status = "exclude"
			} else if watch[signal.Symbol] {
				status = "watch"
			}
			stock = &DailyReviewStock{
				SignalDate: signal.SignalDate, Symbol: signal.Symbol, Latest: signal.Latest, MaxScore: signal.Score,
				Tracking: signal.Tracking, Status: status, Watch: watch[signal.Symbol], Excluded: exclude[signal.Symbol],
				Note: notes[signal.Symbol], StrategyIDs: []string{}, Strategies: []string{}, Signals: []DailyReviewSignal{},
			}
			stockMap[key] = stock
		}
		stock.Signals = append(stock.Signals, signal)
		stock.StrategyIDs = appendUnique(stock.StrategyIDs, signal.StrategyID)
		stock.Strategies = appendUnique(stock.Strategies, signal.StrategyName)
		if signal.Score > stock.MaxScore {
			stock.MaxScore = signal.Score
		}
		if trackingCompleteness(signal.Tracking) > trackingCompleteness(stock.Tracking) {
			stock.Tracking = signal.Tracking
		}
	}
	stocks := make([]DailyReviewStock, 0, len(stockMap))
	for _, stock := range stockMap {
		stock.StrategyCount = len(stock.StrategyIDs)
		totalScore := 0.0
		for _, signal := range stock.Signals {
			totalScore += signal.Score
		}
		stock.AverageScore = roundDailyFloat(totalScore / float64(maxInt(len(stock.Signals), 1)))
		stock.MaxScore = roundDailyFloat(stock.MaxScore)
		stocks = append(stocks, *stock)
	}
	sort.Slice(stocks, func(i, j int) bool {
		if stocks[i].SignalDate != stocks[j].SignalDate {
			return stocks[i].SignalDate > stocks[j].SignalDate
		}
		if stocks[i].StrategyCount != stocks[j].StrategyCount {
			return stocks[i].StrategyCount > stocks[j].StrategyCount
		}
		if stocks[i].MaxScore != stocks[j].MaxScore {
			return stocks[i].MaxScore > stocks[j].MaxScore
		}
		return stocks[i].Symbol < stocks[j].Symbol
	})
	consensus := []DailyReviewStock{}
	for _, stock := range stocks {
		if stock.StrategyCount >= 2 {
			consensus = append(consensus, stock)
		}
	}
	return stocks, consensus
}

func ensureBoolSet(value map[string]bool) map[string]bool {
	if value == nil {
		return map[string]bool{}
	}
	return value
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func sumAnalysisSignals(days map[string][]dailyAnalysisSignal) int {
	total := 0
	for _, signals := range days {
		total += len(signals)
	}
	return total
}
func averageSignalScore(signals []dailyAnalysisSignal) float64 {
	if len(signals) == 0 {
		return 0
	}
	total := 0.0
	for _, item := range signals {
		total += item.score
	}
	return total / float64(len(signals))
}
func strategyConsensusCount(signals []dailyAnalysisSignal, days map[string][]dailyAnalysisSignal) int {
	total := 0
	for _, item := range signals {
		strategyIDs := map[string]bool{}
		for _, other := range days[item.date] {
			if other.symbol == item.symbol {
				strategyIDs[other.strategyID] = true
			}
		}
		if len(strategyIDs) >= 2 {
			total++
		}
	}
	return total
}

type dailyTrackingStats struct {
	completed, successCount, positiveCount int
	successRate, positiveRate              float64
	averageReturn, maxReturn               float64
}

func aggregateTrackingStats(signals []dailyAnalysisSignal, key string) dailyTrackingStats {
	stats := dailyTrackingStats{}
	total := 0.0
	hasCompleted := false
	for _, item := range signals {
		if item.tracking == nil {
			continue
		}
		horizon, ok := item.tracking.Horizons[key]
		if !ok || horizon.Status != "complete" {
			continue
		}
		stats.completed++
		total += horizon.CloseReturn
		if !hasCompleted || horizon.CloseReturn > stats.maxReturn {
			stats.maxReturn = horizon.CloseReturn
			hasCompleted = true
		}
		if horizon.Success {
			stats.successCount++
		}
		if horizon.CloseReturn > 0 {
			stats.positiveCount++
		}
	}
	if stats.completed == 0 {
		return stats
	}
	stats.successRate = roundDailyFloat(float64(stats.successCount) / float64(stats.completed) * 100)
	stats.positiveRate = roundDailyFloat(float64(stats.positiveCount) / float64(stats.completed) * 100)
	stats.averageReturn = roundDailyFloat(total / float64(stats.completed))
	stats.maxReturn = roundDailyFloat(stats.maxReturn)
	return stats
}

func (s *AppStore) ListAutomationChildRuns(parent AutomationRun) ([]AutomationRun, error) {
	query := `SELECT id,parent_run_id,task_id,task_name,task_type,status,started_at,finished_at,log,result_json,matched_count
		FROM automation_runs WHERE parent_run_id=? ORDER BY started_at`
	runs, err := scanAutomationRuns(s.db.Query(query, parent.ID))
	if err != nil || len(runs) > 0 {
		return runs, err
	}
	// Fallback for databases opened before the parent_run_id migration.
	end := parent.FinishedAt
	if end == "" {
		if started, parseErr := time.Parse(time.RFC3339, parent.StartedAt); parseErr == nil {
			end = started.Add(2 * time.Hour).Format(time.RFC3339)
		}
	}
	return scanAutomationRuns(s.db.Query(`SELECT id,parent_run_id,task_id,task_name,task_type,status,started_at,finished_at,log,result_json,matched_count
		FROM automation_runs WHERE task_id LIKE 'system-strategy:%' AND started_at>=? AND started_at<=? ORDER BY started_at`, parent.StartedAt, end))
}

func scanAutomationRuns(rows *sql.Rows, err error) ([]AutomationRun, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AutomationRun{}
	for rows.Next() {
		var run AutomationRun
		if err := rows.Scan(&run.ID, &run.ParentRunID, &run.TaskID, &run.TaskName, &run.TaskType, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Log, &run.ResultJSON, &run.MatchedCount); err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func (s *AppStore) ListSelectionResultsByRunIDs(runIDs []string, limit int) ([]SelectionResult, error) {
	if len(runIDs) == 0 {
		return []SelectionResult{}, nil
	}
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, 0, len(runIDs)+1)
	for index, runID := range runIDs {
		placeholders[index] = "?"
		args = append(args, runID)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,detail_json,created_at
		FROM selection_results WHERE run_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SelectionResult{}
	for rows.Next() {
		var item SelectionResult
		if err := rows.Scan(&item.ID, &item.RunID, &item.TaskID, &item.TaskName, &item.FormulaID, &item.FormulaName, &item.Symbol, &item.Latest, &item.DetailJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	queryErr := rows.Err()
	_ = rows.Close()
	if queryErr != nil {
		return nil, queryErr
	}
	if err := s.hydrateSelectionResultsTracking(items); err != nil {
		return nil, err
	}
	return items, nil
}

func buildDailyStrategySummaries(parent AutomationRun, children []AutomationRun) []DailyReviewStrategySummary {
	var parentResult dailyBatchResult
	_ = json.Unmarshal([]byte(parent.ResultJSON), &parentResult)
	parentByID := map[string]dailyBatchStrategy{}
	for _, summary := range parentResult.Strategies {
		parentByID[summary.StrategyID] = summary
	}
	items := make([]DailyReviewStrategySummary, 0, len(children)+len(parentByID))
	seen := map[string]bool{}
	for _, child := range children {
		strategyID := strings.TrimPrefix(child.TaskID, "system-strategy:")
		parentSummary := parentByID[strategyID]
		errorCount := parentSummary.Errors
		if errorCount == 0 {
			var result struct {
				Errors map[string]string `json:"errors"`
			}
			if json.Unmarshal([]byte(child.ResultJSON), &result) == nil {
				errorCount = len(result.Errors)
			}
		}
		items = append(items, DailyReviewStrategySummary{
			RunID: child.ID, StrategyID: strategyID, StrategyName: child.TaskName, Status: child.Status,
			Matched: child.MatchedCount, Errors: errorCount, Error: child.Log,
			StartedAt: child.StartedAt, FinishedAt: child.FinishedAt, DurationMS: elapsedMilliseconds(child.StartedAt, child.FinishedAt),
		})
		seen[strategyID] = true
	}
	for strategyID, summary := range parentByID {
		if seen[strategyID] {
			continue
		}
		items = append(items, DailyReviewStrategySummary{
			StrategyID: strategyID, StrategyName: summary.Name, Status: summary.Status,
			Matched: summary.Matched, Errors: summary.Errors, Error: summary.Error,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Matched == items[j].Matched {
			return items[i].StrategyName < items[j].StrategyName
		}
		return items[i].Matched > items[j].Matched
	})
	return items
}

func selectionResultScore(detailJSON string) float64 {
	var detail struct {
		Score float64 `json:"score"`
	}
	_ = json.Unmarshal([]byte(detailJSON), &detail)
	return detail.Score
}

func trackingCompleteness(tracking *SelectionTracking) int {
	if tracking == nil {
		return 0
	}
	count := 0
	for _, horizon := range tracking.Horizons {
		if horizon.Status == "complete" {
			count++
		}
	}
	return count
}

func textDate(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.In(time.Local).Format("2006-01-02")
	}
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func elapsedMilliseconds(startRaw, endRaw string) int64 {
	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		return 0
	}
	end := time.Now()
	if endRaw != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, endRaw); parseErr == nil {
			end = parsed
		}
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func stringSet(symbols []string) map[string]bool {
	result := make(map[string]bool, len(symbols))
	for _, symbol := range NormalizeSymbols(symbols) {
		result[symbol] = true
	}
	return result
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func roundDailyFloat(value float64) float64 {
	return float64(int64(value*100+0.5)) / 100
}
