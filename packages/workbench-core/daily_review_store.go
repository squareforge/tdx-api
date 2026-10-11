package workbench

import (
	"database/sql"
	"encoding/json"
	"math"
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
	RunID        string                           `json:"run_id,omitempty"`
	StrategyID   string                           `json:"strategy_id"`
	StrategyName string                           `json:"strategy_name"`
	Status       string                           `json:"status"`
	Matched      int                              `json:"matched"`
	Errors       int                              `json:"errors"`
	Error        string                           `json:"error,omitempty"`
	StartedAt    string                           `json:"started_at,omitempty"`
	FinishedAt   string                           `json:"finished_at,omitempty"`
	DurationMS   int64                            `json:"duration_ms"`
	HorizonStats map[string]DailyReviewPriceStats `json:"horizon_stats"`
}

type DailyReviewPriceStats struct {
	Completed        int     `json:"completed"`
	TargetReturn     float64 `json:"target_return"`
	HighTargetCount  int     `json:"high_target_count"`
	LowTargetCount   int     `json:"low_target_count"`
	OpenTargetCount  int     `json:"open_target_count"`
	CloseTargetCount int     `json:"close_target_count"`
	HighTargetRate   float64 `json:"high_target_rate"`
	LowTargetRate    float64 `json:"low_target_rate"`
	OpenTargetRate   float64 `json:"open_target_rate"`
	CloseTargetRate  float64 `json:"close_target_rate"`
}

type DailyReviewSignal struct {
	ID              string             `json:"id"`
	RunID           string             `json:"run_id"`
	StrategyID      string             `json:"strategy_id"`
	StrategyName    string             `json:"strategy_name"`
	SignalDate      string             `json:"signal_date"`
	Symbol          string             `json:"symbol"`
	Latest          float64            `json:"latest"`
	SignalAt        string             `json:"signal_at,omitempty"`
	SignalPrice     float64            `json:"signal_price,omitempty"`
	QuoteAt         string             `json:"quote_at,omitempty"`
	QuoteSource     string             `json:"quote_source,omitempty"`
	PriceType       string             `json:"price_type,omitempty"`
	QuoteAgeMS      int64              `json:"quote_age_ms,omitempty"`
	SnapshotStatus  string             `json:"snapshot_status,omitempty"`
	StrategyVersion string             `json:"strategy_version,omitempty"`
	Score           float64            `json:"score"`
	ExecutionPolicy string             `json:"execution_policy,omitempty"`
	ExecutionStatus string             `json:"execution_status,omitempty"`
	EntryPrice      float64            `json:"entry_price,omitempty"`
	ExecutionCost   float64            `json:"execution_cost,omitempty"`
	FailureReason   string             `json:"failure_reason,omitempty"`
	DetailJSON      string             `json:"detail_json"`
	Tracking        *SelectionTracking `json:"tracking,omitempty"`
	CreatedAt       string             `json:"created_at"`
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
	StrategyID          string                               `json:"strategy_id"`
	StrategyName        string                               `json:"strategy_name"`
	RunDays             int                                  `json:"run_days"`
	SignalCount         int                                  `json:"signal_count"`
	AverageDailySignals float64                              `json:"average_daily_signals"`
	UniqueSymbols       int                                  `json:"unique_symbols"`
	AverageScore        float64                              `json:"average_score"`
	ConsensusSignals    int                                  `json:"consensus_signals"`
	D1Completed         int                                  `json:"d1_completed"`
	D1SuccessCount      int                                  `json:"d1_success_count"`
	D1SuccessRate       float64                              `json:"d1_success_rate"`
	D1PositiveCount     int                                  `json:"d1_positive_count"`
	D1PositiveRate      float64                              `json:"d1_positive_rate"`
	D1AverageReturn     float64                              `json:"d1_average_return"`
	D1MaxReturn         float64                              `json:"d1_max_return"`
	D3Completed         int                                  `json:"d3_completed"`
	D3SuccessCount      int                                  `json:"d3_success_count"`
	D3SuccessRate       float64                              `json:"d3_success_rate"`
	D3PositiveCount     int                                  `json:"d3_positive_count"`
	D3PositiveRate      float64                              `json:"d3_positive_rate"`
	D3AverageReturn     float64                              `json:"d3_average_return"`
	D3MaxReturn         float64                              `json:"d3_max_return"`
	D5Completed         int                                  `json:"d5_completed"`
	D5SuccessCount      int                                  `json:"d5_success_count"`
	D5SuccessRate       float64                              `json:"d5_success_rate"`
	D5PositiveCount     int                                  `json:"d5_positive_count"`
	D5PositiveRate      float64                              `json:"d5_positive_rate"`
	D5AverageReturn     float64                              `json:"d5_average_return"`
	D5MaxReturn         float64                              `json:"d5_max_return"`
	D10Completed        int                                  `json:"d10_completed"`
	D10SuccessCount     int                                  `json:"d10_success_count"`
	D10SuccessRate      float64                              `json:"d10_success_rate"`
	D10PositiveCount    int                                  `json:"d10_positive_count"`
	D10PositiveRate     float64                              `json:"d10_positive_rate"`
	D10AverageReturn    float64                              `json:"d10_average_return"`
	D10MaxReturn        float64                              `json:"d10_max_return"`
	HorizonStats        map[string]DailyAnalysisHorizonStats `json:"horizon_stats"`
	ScoreBuckets        map[string]DailyScoreBucket          `json:"score_buckets"`
	D5IC                float64                              `json:"d5_ic"`
}

type DailyScoreBucket struct {
	Count         int     `json:"count"`
	AverageReturn float64 `json:"average_return"`
	PositiveRate  float64 `json:"positive_rate"`
}

type DailyAnalysisHorizonStats struct {
	Completed              int     `json:"completed"`
	PositiveCount          int     `json:"positive_count"`
	PositiveRate           float64 `json:"positive_rate"`
	AverageReturn          float64 `json:"average_return"`
	MedianReturn           float64 `json:"median_return"`
	P25Return              float64 `json:"p25_return"`
	P75Return              float64 `json:"p75_return"`
	MaxReturn              float64 `json:"max_return"`
	AverageMFE             float64 `json:"average_mfe"`
	AverageMAE             float64 `json:"average_mae"`
	AveragePathDrawdown    float64 `json:"average_path_drawdown"`
	AverageBenchmarkReturn float64 `json:"average_benchmark_return"`
	AverageExcessReturn    float64 `json:"average_excess_return"`
	ConfidenceLow          float64 `json:"confidence_low"`
	ConfidenceHigh         float64 `json:"confidence_high"`
	MinimumSampleSize      int     `json:"minimum_sample_size"`
	SampleSufficient       bool    `json:"sample_sufficient"`
	CompleteRate           float64 `json:"complete_rate"`
}

type DailyStrategyAnalysisData struct {
	StartDate       string                        `json:"start_date"`
	EndDate         string                        `json:"end_date"`
	TradingDays     int                           `json:"trading_days"`
	StrategyCount   int                           `json:"strategy_count"`
	SignalCount     int                           `json:"signal_count"`
	UniqueSymbols   int                           `json:"unique_symbols"`
	ConsensusCount  int                           `json:"consensus_count"`
	AverageScore    float64                       `json:"average_score"`
	ReturnBasis     string                        `json:"return_basis"`
	ExecutionPolicy string                        `json:"execution_policy"`
	Benchmark       string                        `json:"benchmark"`
	Strategies      []DailyStrategyAnalysisMetric `json:"strategies"`
	Stocks          []DailyReviewStock            `json:"stocks"`
	Signals         []DailyReviewSignal           `json:"signals"`
	Consensus       []DailyReviewStock            `json:"consensus"`
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
	// Strategy rates cover the whole batch, even when the stock detail list is
	// capped by the request limit.
	results, err := s.listSelectionResultsByRunIDs(runIDs, 0)
	if err != nil {
		return data, err
	}
	resultsByRun := make(map[string][]SelectionResult, len(children))
	for _, item := range results {
		resultsByRun[item.RunID] = append(resultsByRun[item.RunID], item)
	}
	for index := range strategies {
		strategies[index].HorizonStats = aggregateDailyReviewPriceStats(resultsByRun[strategies[index].RunID])
	}
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	if len(results) > limit {
		results = results[:limit]
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
		selectionPrice := effectiveSelectionPrice(item)
		executionPolicy, executionStatus, entryPrice, executionCost, failureReason := s.signalExecutionSummary(item, ExecutionPolicyNextDayOpen)
		signal := DailyReviewSignal{
			ID: item.ID, RunID: item.RunID, StrategyID: strategyIDsByRun[item.RunID], StrategyName: item.FormulaName,
			SignalDate: selected.Date, Symbol: symbol, Latest: selectionPrice, SignalAt: item.SignalAt, SignalPrice: selectionPrice,
			QuoteAt: item.QuoteAt, QuoteSource: item.QuoteSource, PriceType: item.PriceType, QuoteAgeMS: item.QuoteAgeMS,
			SnapshotStatus: item.SnapshotStatus, StrategyVersion: item.StrategyVersion, Score: selectionResultScore(item.DetailJSON),
			ExecutionPolicy: executionPolicy, ExecutionStatus: executionStatus, EntryPrice: entryPrice, ExecutionCost: executionCost, FailureReason: failureReason,
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
				SignalDate: selected.Date, Symbol: symbol, Latest: selectionPrice, MaxScore: signal.Score, Tracking: item.Tracking,
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

// Price target rates use the saved signal price and one shared denominator of
// complete samples. The period extrema are distinct from the final day's high
// and low; existing success/stop-loss policy does not affect these four rates.
func aggregateDailyReviewPriceStats(items []SelectionResult) map[string]DailyReviewPriceStats {
	const targetReturn = 3.0
	stats := make(map[string]DailyReviewPriceStats, 4)
	for _, days := range []int{1, 3, 5, 10} {
		key := horizonKey(days)
		row := DailyReviewPriceStats{TargetReturn: targetReturn}
		for _, item := range items {
			base := effectiveSelectionPrice(item)
			if item.Tracking == nil || !positiveFiniteTrackingValue(base) {
				continue
			}
			horizon, ok := item.Tracking.Horizons[key]
			if !ok || horizon.Status != "complete" {
				continue
			}
			prices := []float64{horizon.MaxGainPrice, horizon.MaxDrawdownPrice, horizon.FinalOpen, horizon.FinalClose}
			valid := true
			for _, price := range prices {
				if !positiveFiniteTrackingValue(price) {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			row.Completed++
			targetPrice := base * (1 + targetReturn/100)
			counts := []*int{&row.HighTargetCount, &row.LowTargetCount, &row.OpenTargetCount, &row.CloseTargetCount}
			for index, price := range prices {
				// Allow only floating-point comparison noise, without rounding
				// a below-target return (e.g. 2.999%) up to a passing 3%.
				if price >= targetPrice || math.Abs(price-targetPrice) <= targetPrice*1e-12 {
					*counts[index]++
				}
			}
		}
		if row.Completed > 0 {
			denominator := float64(row.Completed)
			row.HighTargetRate = roundDailyFloat(float64(row.HighTargetCount) / denominator * 100)
			row.LowTargetRate = roundDailyFloat(float64(row.LowTargetCount) / denominator * 100)
			row.OpenTargetRate = roundDailyFloat(float64(row.OpenTargetCount) / denominator * 100)
			row.CloseTargetRate = roundDailyFloat(float64(row.CloseTargetCount) / denominator * 100)
		}
		stats[key] = row
	}
	return stats
}

func (s *AppStore) BuildDailyStrategyAnalysis(startDate, endDate string, days, limit int) (DailyStrategyAnalysisData, error) {
	return s.BuildDailyStrategyAnalysisWithOptions(startDate, endDate, days, limit, "signal", "")
}

func (s *AppStore) BuildDailyStrategyAnalysisWithOptions(startDate, endDate string, days, limit int, returnBasis, executionPolicy string) (DailyStrategyAnalysisData, error) {
	if returnBasis != "trade" {
		returnBasis = "signal"
	}
	if executionPolicy == "" {
		executionPolicy = ExecutionPolicyNextDayOpen
	}
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
			selectionPrice := effectiveSelectionPrice(item)
			score := selectionResultScore(item.DetailJSON)
			tracking := item.Tracking
			if returnBasis == "trade" {
				tracking = s.selectionExecutionTracking(item, executionPolicy)
			}
			_, executionStatus, entryPrice, executionCost, failureReason := s.signalExecutionSummary(item, executionPolicy)
			reviewSignal := DailyReviewSignal{
				ID: item.ID, RunID: item.RunID, StrategyID: strategyID, StrategyName: strategyName,
				SignalDate: batch.Date, Symbol: symbol, Latest: selectionPrice, SignalAt: item.SignalAt, SignalPrice: selectionPrice,
				QuoteAt: item.QuoteAt, QuoteSource: item.QuoteSource, PriceType: item.PriceType, QuoteAgeMS: item.QuoteAgeMS,
				SnapshotStatus: item.SnapshotStatus, StrategyVersion: item.StrategyVersion, Score: score,
				ExecutionPolicy: executionPolicy, ExecutionStatus: executionStatus, EntryPrice: entryPrice, ExecutionCost: executionCost, FailureReason: failureReason,
				DetailJSON: item.DetailJSON, Tracking: tracking, CreatedAt: item.CreatedAt,
			}
			reviewSignals = append(reviewSignals, reviewSignal)
			s := dailyAnalysisSignal{date: batch.Date, symbol: symbol, strategyID: strategyID, score: score, tracking: tracking}
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
		metric := DailyStrategyAnalysisMetric{StrategyID: strategyID, StrategyName: strategyName, RunDays: len(strategyDays[strategyID]), SignalCount: len(signals), UniqueSymbols: 0, HorizonStats: map[string]DailyAnalysisHorizonStats{}, ScoreBuckets: aggregateScoreBuckets(signals, "d5")}
		metric.D5IC = roundDailyFloat(scoreReturnIC(signals, "d5"))
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
		metric.HorizonStats["d1"] = d1.toPublic()
		metric.HorizonStats["d1"] = withCompleteRate(metric.HorizonStats["d1"], len(signals))
		metric.D1Completed, metric.D1SuccessCount, metric.D1SuccessRate = d1.completed, d1.successCount, d1.successRate
		metric.D1PositiveCount, metric.D1PositiveRate, metric.D1AverageReturn, metric.D1MaxReturn = d1.positiveCount, d1.positiveRate, d1.averageReturn, d1.maxReturn
		d3 := aggregateTrackingStats(signals, "d3")
		metric.HorizonStats["d3"] = d3.toPublic()
		metric.HorizonStats["d3"] = withCompleteRate(metric.HorizonStats["d3"], len(signals))
		metric.D3Completed, metric.D3SuccessCount, metric.D3SuccessRate = d3.completed, d3.successCount, d3.successRate
		metric.D3PositiveCount, metric.D3PositiveRate, metric.D3AverageReturn, metric.D3MaxReturn = d3.positiveCount, d3.positiveRate, d3.averageReturn, d3.maxReturn
		d5 := aggregateTrackingStats(signals, "d5")
		metric.HorizonStats["d5"] = d5.toPublic()
		metric.HorizonStats["d5"] = withCompleteRate(metric.HorizonStats["d5"], len(signals))
		metric.D5Completed, metric.D5SuccessCount, metric.D5SuccessRate = d5.completed, d5.successCount, d5.successRate
		metric.D5PositiveCount, metric.D5PositiveRate, metric.D5AverageReturn, metric.D5MaxReturn = d5.positiveCount, d5.positiveRate, d5.averageReturn, d5.maxReturn
		d10 := aggregateTrackingStats(signals, "d10")
		metric.HorizonStats["d10"] = d10.toPublic()
		metric.HorizonStats["d10"] = withCompleteRate(metric.HorizonStats["d10"], len(signals))
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
	benchmark := "none"
	for _, signal := range reviewSignals {
		if signal.Tracking == nil {
			continue
		}
		for _, horizon := range signal.Tracking.Horizons {
			if horizon.BenchmarkCloseReturn != 0 || horizon.ExcessCloseReturn != 0 {
				benchmark = "sh000001"
				break
			}
		}
		if benchmark != "none" {
			break
		}
	}
	return DailyStrategyAnalysisData{
		StartDate: startDate, EndDate: endDate, TradingDays: len(selected), StrategyCount: len(metrics),
		SignalCount: sumAnalysisSignals(daySignals), UniqueSymbols: len(allSymbols), ConsensusCount: consensus,
		AverageScore: roundDailyFloat(totalScore / float64(maxInt(sumAnalysisSignals(daySignals), 1))), Strategies: metrics,
		ReturnBasis: returnBasis, ExecutionPolicy: executionPolicy, Benchmark: benchmark,
		Stocks: stocks, Signals: reviewSignals, Consensus: consensusStocks,
	}, nil
}

func (s *AppStore) selectionExecutionTracking(item SelectionResult, policy string) *SelectionTracking {
	executions, err := s.ListSelectionSignalExecutions(item.ID, policy)
	if err != nil || len(executions) == 0 || executions[0].Status != "filled" {
		return nil
	}
	observations, err := s.ListSelectionSignalObservations(item.ID, executions[0].ID)
	if err != nil || len(observations) == 0 {
		return nil
	}
	tracking := EvaluateSelectionExecutionTracking(item, executions[0], observations, []int{1, 3, 5, 10}, DefaultTrackingTargetReturn, DefaultTrackingDrawdownLimit, time.Now())
	return &tracking
}

func (s *AppStore) signalExecutionSummary(item SelectionResult, policy string) (string, string, float64, float64, string) {
	executions, err := s.ListSelectionSignalExecutions(item.ID, policy)
	if err != nil || len(executions) == 0 {
		return policy, "unavailable", 0, 0, "未生成模拟成交记录"
	}
	execution := executions[0]
	return execution.PolicyName, execution.Status, execution.FillPrice, execution.TotalCost, execution.FailureReason
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

func effectiveSelectionPrice(item SelectionResult) float64 {
	if item.SignalPrice > 0 {
		return item.SignalPrice
	}
	return item.Latest
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
	completed, successCount, positiveCount      int
	successRate, positiveRate                   float64
	averageReturn, maxReturn                    float64
	medianReturn, p25Return, p75Return          float64
	averageMFE, averageMAE, averagePathDrawdown float64
	averageBenchmarkReturn, averageExcessReturn float64
	confidenceLow, confidenceHigh               float64
}

func aggregateTrackingStats(signals []dailyAnalysisSignal, key string) dailyTrackingStats {
	stats := dailyTrackingStats{}
	total := 0.0
	returns := []float64{}
	mfeTotal, maeTotal, pathDrawdownTotal := 0.0, 0.0, 0.0
	benchmarkTotal, excessTotal, squareTotal := 0.0, 0.0, 0.0
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
		squareTotal += horizon.CloseReturn * horizon.CloseReturn
		benchmarkTotal += horizon.BenchmarkCloseReturn
		excessTotal += horizon.ExcessCloseReturn
		returns = append(returns, horizon.CloseReturn)
		mfeTotal += horizon.MFE
		maeTotal += horizon.MAE
		pathDrawdownTotal += horizon.MaxPathDrawdown
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
	sort.Float64s(returns)
	stats.p25Return = roundDailyFloat(percentileDaily(returns, 0.25))
	stats.medianReturn = roundDailyFloat(percentileDaily(returns, 0.50))
	stats.p75Return = roundDailyFloat(percentileDaily(returns, 0.75))
	stats.averageMFE = roundDailyFloat(mfeTotal / float64(stats.completed))
	stats.averageMAE = roundDailyFloat(maeTotal / float64(stats.completed))
	stats.averagePathDrawdown = roundDailyFloat(pathDrawdownTotal / float64(stats.completed))
	stats.averageBenchmarkReturn = roundDailyFloat(benchmarkTotal / float64(stats.completed))
	stats.averageExcessReturn = roundDailyFloat(excessTotal / float64(stats.completed))
	variance := squareTotal/float64(stats.completed) - stats.averageReturn*stats.averageReturn
	if variance < 0 {
		variance = 0
	}
	margin := 1.96 * math.Sqrt(variance/float64(stats.completed))
	stats.confidenceLow = roundDailyFloat(stats.averageReturn - margin)
	stats.confidenceHigh = roundDailyFloat(stats.averageReturn + margin)
	return stats
}

func aggregateScoreBuckets(signals []dailyAnalysisSignal, key string) map[string]DailyScoreBucket {
	type accumulator struct {
		count, positive int
		total           float64
	}
	buckets := map[string]*accumulator{"low": {}, "middle": {}, "high": {}}
	for _, signal := range signals {
		if signal.tracking == nil {
			continue
		}
		horizon, ok := signal.tracking.Horizons[key]
		if !ok || horizon.Status != "complete" {
			continue
		}
		bucket := "low"
		switch {
		case signal.score >= 66.67:
			bucket = "high"
		case signal.score >= 33.33:
			bucket = "middle"
		}
		item := buckets[bucket]
		item.count++
		item.total += horizon.CloseReturn
		if horizon.CloseReturn > 0 {
			item.positive++
		}
	}
	result := map[string]DailyScoreBucket{}
	for name, item := range buckets {
		if item.count == 0 {
			result[name] = DailyScoreBucket{}
			continue
		}
		result[name] = DailyScoreBucket{Count: item.count, AverageReturn: roundDailyFloat(item.total / float64(item.count)), PositiveRate: roundDailyFloat(float64(item.positive) / float64(item.count) * 100)}
	}
	return result
}

func scoreReturnIC(signals []dailyAnalysisSignal, key string) float64 {
	scores, returns := []float64{}, []float64{}
	for _, signal := range signals {
		if signal.tracking == nil {
			continue
		}
		horizon, ok := signal.tracking.Horizons[key]
		if !ok || horizon.Status != "complete" {
			continue
		}
		scores = append(scores, signal.score)
		returns = append(returns, horizon.CloseReturn)
	}
	if len(scores) < 3 {
		return 0
	}
	meanScore, meanReturn := 0.0, 0.0
	for index := range scores {
		meanScore += scores[index]
		meanReturn += returns[index]
	}
	meanScore /= float64(len(scores))
	meanReturn /= float64(len(returns))
	numerator, scoreVariance, returnVariance := 0.0, 0.0, 0.0
	for index := range scores {
		scoreDelta, returnDelta := scores[index]-meanScore, returns[index]-meanReturn
		numerator += scoreDelta * returnDelta
		scoreVariance += scoreDelta * scoreDelta
		returnVariance += returnDelta * returnDelta
	}
	if scoreVariance == 0 || returnVariance == 0 {
		return 0
	}
	return numerator / math.Sqrt(scoreVariance*returnVariance)
}

func (stats dailyTrackingStats) toPublic() DailyAnalysisHorizonStats {
	return DailyAnalysisHorizonStats{
		Completed: stats.completed, PositiveCount: stats.positiveCount, PositiveRate: stats.positiveRate,
		AverageReturn: stats.averageReturn, MedianReturn: stats.medianReturn, P25Return: stats.p25Return, P75Return: stats.p75Return,
		MaxReturn: stats.maxReturn, AverageMFE: stats.averageMFE, AverageMAE: stats.averageMAE,
		AveragePathDrawdown:    stats.averagePathDrawdown,
		AverageBenchmarkReturn: stats.averageBenchmarkReturn, AverageExcessReturn: stats.averageExcessReturn,
		ConfidenceLow: stats.confidenceLow, ConfidenceHigh: stats.confidenceHigh,
		MinimumSampleSize: 5, SampleSufficient: stats.completed >= 5,
	}
}

func withCompleteRate(stats DailyAnalysisHorizonStats, total int) DailyAnalysisHorizonStats {
	if total > 0 {
		stats.CompleteRate = roundDailyFloat(float64(stats.Completed) / float64(total) * 100)
	}
	return stats
}

func percentileDaily(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if ratio <= 0 {
		return values[0]
	}
	if ratio >= 1 {
		return values[len(values)-1]
	}
	position := ratio * float64(len(values)-1)
	low := int(position)
	high := low + 1
	if high >= len(values) {
		return values[low]
	}
	weight := position - float64(low)
	return values[low]*(1-weight) + values[high]*weight
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
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	return s.listSelectionResultsByRunIDs(runIDs, limit)
}

// A zero limit loads all signals for batch-wide aggregates.
func (s *AppStore) listSelectionResultsByRunIDs(runIDs []string, limit int) ([]SelectionResult, error) {
	if len(runIDs) == 0 {
		return []SelectionResult{}, nil
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, 0, len(runIDs)+1)
	for index, runID := range runIDs {
		placeholders[index] = "?"
		args = append(args, runID)
	}
	query := `SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
		signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
		detail_json,created_at
		FROM selection_results WHERE run_id IN (` + strings.Join(placeholders, ",") + `) ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SelectionResult{}
	for rows.Next() {
		var item SelectionResult
		if err := scanSelectionResult(&item, rows); err != nil {
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
