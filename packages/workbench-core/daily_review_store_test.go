package workbench

import (
	"math"
	"testing"
	"time"
)

func TestAggregateDailyReviewPriceStatsUsesSavedPricesAndInclusiveTarget(t *testing.T) {
	complete := func(high, low, open, close float64) *SelectionTracking {
		tracking := &SelectionTracking{BasePrice: 100, Horizons: map[string]SelectionHorizon{}}
		for _, days := range []int{1, 3, 5, 10} {
			tracking.Horizons[horizonKey(days)] = SelectionHorizon{
				HorizonDays: days, Status: "complete", MaxGainPrice: high, MaxDrawdownPrice: low,
				FinalOpen: open, FinalClose: close, StopLossHit: true, Success: false,
			}
		}
		return tracking
	}
	items := []SelectionResult{
		// The saved signal price takes priority over Latest and tracking.BasePrice.
		{SignalPrice: 10, Latest: 100, Tracking: complete(10.3, 10.3, 10.3, 10.3)},
		// Historical signals can use their saved Latest. 2.999% must not round up.
		{Latest: 20, Tracking: complete(21, 20.5998, 20.5998, 20.6)},
		{SignalPrice: 10},
		{Tracking: complete(11, 10, 10, 10)},
		{SignalPrice: math.Inf(1), Tracking: complete(11, 10, 10, 10)},
		{SignalPrice: 10, Tracking: complete(math.NaN(), 10, 10, 10)},
		{SignalPrice: 10, Tracking: complete(11, math.Inf(1), 10, 10)},
		{SignalPrice: 10, Tracking: complete(11, 10, 0, 10)},
		{SignalPrice: 10, Tracking: complete(11, 10, 10, -1)},
	}
	for _, status := range []string{"pending", "unavailable"} {
		tracking := complete(11, 11, 11, 11)
		for key, horizon := range tracking.Horizons {
			horizon.Status = status
			tracking.Horizons[key] = horizon
		}
		items = append(items, SelectionResult{SignalPrice: 10, Tracking: tracking})
	}

	stats := aggregateDailyReviewPriceStats(items)
	want := DailyReviewPriceStats{
		Completed: 2, TargetReturn: 3,
		HighTargetCount: 2, LowTargetCount: 1, OpenTargetCount: 1, CloseTargetCount: 2,
		HighTargetRate: 100, LowTargetRate: 50, OpenTargetRate: 50, CloseTargetRate: 100,
	}
	for _, key := range []string{"d1", "d3", "d5", "d10"} {
		if stats[key] != want {
			t.Errorf("%s stats = %+v, want %+v", key, stats[key], want)
		}
	}
	for key, row := range aggregateDailyReviewPriceStats(nil) {
		if row != (DailyReviewPriceStats{TargetReturn: 3}) {
			t.Errorf("empty %s stats = %+v", key, row)
		}
	}
}

func TestBuildDailyReviewPriceStatsUsesFullWindowAndSeparateStrategies(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	parent, err := store.CreateAutomationRun(AutomationTask{ID: SystemStrategyDailyBatchTaskID, Name: "系统策略日报", Type: "system_strategy_batch"})
	if err != nil {
		t.Fatal(err)
	}
	createStrategy := func(id string, items []SelectionResult) {
		t.Helper()
		child, err := store.CreateAutomationRun(AutomationTask{ID: "system-strategy:" + id, Name: id, Type: "strategy_selection"}, parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveSelectionResults(child, Formula{ID: "strategy:" + id, Name: id}, items); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishAutomationRun(child.ID, "success", "", `{}`, len(items)); err != nil {
			t.Fatal(err)
		}
	}
	trend := SelectionResult{ID: "trend-signal", Symbol: "000001", SignalPrice: 10, Latest: 50, SignalAt: "2026-09-01T16:00:00+08:00"}
	tracking := EvaluateSelectionTracking(trend, []TrackingBar{
		{Date: 20260901, Open: 40, High: 50, Low: 30, Close: 40},
		{Date: 20260902, Open: 10.3, High: 11, Low: 9, Close: 10.3},
		{Date: 20260903, Open: 10, High: 10.2, Low: 9.9, Close: 10},
		{Date: 20260904, Open: 10.3, High: 10.3, Low: 10.3, Close: 10.3},
		{Date: 20260907, Open: 10, High: 10.2, Low: 9.9, Close: 10},
		{Date: 20260908, Open: 10, High: 10.2, Low: 9.9, Close: 10},
	}, nil, 3, 5, time.Now())
	trend.Tracking = &tracking
	createStrategy("trend", []SelectionResult{trend})
	// A signal at its prior day's +10% price remains an eligible sample.
	limitSignal := SelectionResult{ID: "limit-signal", Symbol: "000001", SignalPrice: 11, Latest: 11, SignalAt: "2026-09-01T16:00:00+08:00", DetailJSON: `{"previous_close":10,"limit_up":11}`}
	limitTracking := EvaluateSelectionTracking(limitSignal, []TrackingBar{
		{Date: 20260902, Open: 11.4, High: 11.5, Low: 11.34, Close: 11.45},
	}, nil, 3, 5, time.Now())
	limitSignal.Tracking = &limitTracking
	createStrategy("limit", []SelectionResult{limitSignal, {ID: "untracked", Symbol: "000002", SignalPrice: 10, Latest: 10}})
	if err := store.FinishAutomationRun(parent.ID, "success", "", `{"strategy_count":2}`, 3); err != nil {
		t.Fatal(err)
	}

	review, err := store.BuildDailyReview("", parent.ID, 5000)
	if err != nil {
		t.Fatal(err)
	}
	byStrategy := map[string]DailyReviewStrategySummary{}
	for _, strategy := range review.Strategies {
		byStrategy[strategy.StrategyID] = strategy
	}
	wantTrend := DailyReviewPriceStats{
		Completed: 1, TargetReturn: 3, HighTargetCount: 1, OpenTargetCount: 1, CloseTargetCount: 1,
		HighTargetRate: 100, OpenTargetRate: 100, CloseTargetRate: 100,
	}
	for _, key := range []string{"d1", "d3"} {
		if got := byStrategy["trend"].HorizonStats[key]; got != wantTrend {
			t.Errorf("trend %s stats = %+v, want %+v", key, got, wantTrend)
		}
	}
	wantLimit := wantTrend
	wantLimit.LowTargetCount, wantLimit.LowTargetRate = 1, 100
	if got := byStrategy["limit"].HorizonStats["d1"]; got != wantLimit {
		t.Errorf("limit d1 stats = %+v, want %+v", got, wantLimit)
	}
	for _, key := range []string{"d3", "d5", "d10"} {
		if got := byStrategy["limit"].HorizonStats[key]; got != (DailyReviewPriceStats{TargetReturn: 3}) {
			t.Errorf("pending limit %s stats = %+v", key, got)
		}
	}
	// The D5 final high is below target, but an earlier high in the window passed.
	wantD5 := DailyReviewPriceStats{Completed: 1, TargetReturn: 3, HighTargetCount: 1, HighTargetRate: 100}
	if got := byStrategy["trend"].HorizonStats["d5"]; got != wantD5 {
		t.Errorf("trend d5 stats = %+v, want %+v", got, wantD5)
	}
	if got := byStrategy["trend"].HorizonStats["d10"]; got != (DailyReviewPriceStats{TargetReturn: 3}) {
		t.Errorf("pending trend d10 stats = %+v", got)
	}
	capped, err := store.BuildDailyReview("", parent.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped.Signals) != 1 {
		t.Fatalf("capped signals = %d, want 1", len(capped.Signals))
	}
	for _, strategy := range capped.Strategies {
		for key, row := range strategy.HorizonStats {
			if row != byStrategy[strategy.StrategyID].HorizonStats[key] {
				t.Errorf("stock list limit changed %s %s rate: %+v", strategy.StrategyID, key, row)
			}
		}
	}
}

func TestBuildDailyReviewAggregatesSystemStrategyBatch(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	parent, err := store.CreateAutomationRun(AutomationTask{
		ID: SystemStrategyDailyBatchTaskID, Name: "系统策略日报", Type: "system_strategy_batch",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateAutomationRun(AutomationTask{
		ID: "system-strategy:trend", Name: "趋势策略", Type: "strategy_selection",
	}, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateAutomationRun(AutomationTask{
		ID: "system-strategy:volume", Name: "量价策略", Type: "strategy_selection",
	}, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	tracking := &SelectionTracking{Horizons: map[string]SelectionHorizon{
		"d1": {HorizonDays: 1, Status: "complete", CloseReturn: 1.5, Success: true},
		"d3": {HorizonDays: 3, Status: "complete", CloseReturn: 4.2},
	}}
	if err := store.SaveSelectionResults(first, Formula{ID: "strategy:trend", Name: "趋势策略"}, []SelectionResult{
		{Symbol: "000001", Latest: 10, DetailJSON: `{"score":80}`, Tracking: tracking},
		{Symbol: "000002", Latest: 20, DetailJSON: `{"score":60}`},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSelectionResults(second, Formula{ID: "strategy:volume", Name: "量价策略"}, []SelectionResult{
		{Symbol: "000001", Latest: 10, DetailJSON: `{"score":70}`},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAutomationRun(first.ID, "success", "", `{}`, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAutomationRun(second.ID, "success", "", `{}`, 1); err != nil {
		t.Fatal(err)
	}
	resultJSON := `{"strategy_count":2,"candidate_symbols":4606,"kline_loaded":4602,"kline_failed":4,"strategies":[{"strategy_id":"trend","name":"趋势策略","status":"success","matched":2},{"strategy_id":"volume","name":"量价策略","status":"success","matched":1}]}`
	if err := store.FinishAutomationRun(parent.ID, "success", "", resultJSON, 3); err != nil {
		t.Fatal(err)
	}

	review, err := store.BuildDailyReview("", parent.ID, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if review.SelectedBatch == nil || review.SelectedBatch.ID != parent.ID {
		t.Fatalf("selected batch = %#v", review.SelectedBatch)
	}
	if review.Summary.RawSignals != 3 || review.Summary.StockCount != 2 || review.Summary.ConsensusCount != 1 {
		t.Fatalf("unexpected summary: %#v", review.Summary)
	}
	if review.Summary.StrategyCount != 2 || review.Summary.CandidateSymbols != 4606 {
		t.Fatalf("unexpected batch facts: %#v", review.Summary)
	}
	if len(review.Consensus) != 1 || review.Consensus[0].Symbol != "000001" || review.Consensus[0].StrategyCount != 2 {
		t.Fatalf("unexpected consensus: %#v", review.Consensus)
	}
	if trackingCompleteness(review.Consensus[0].Tracking) != 2 {
		t.Fatalf("completed tracking was not retained: %+v", review.Consensus[0].Tracking)
	}
	if review.Consensus[0].SignalDate != review.Date {
		t.Fatalf("stock signal date = %q, want %q", review.Consensus[0].SignalDate, review.Date)
	}
	if review.Consensus[0].Tracking.Horizons["d1"].CloseReturn != 1.5 {
		t.Fatalf("d1 tracking was not retained: %+v", review.Consensus[0].Tracking)
	}
}

func TestBuildDailyStrategyAnalysisAggregatesDateRange(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	tracking := &SelectionTracking{Horizons: map[string]SelectionHorizon{
		"d1": {HorizonDays: 1, Status: "complete", CloseReturn: 1.2, Success: true},
		"d5": {HorizonDays: 5, Status: "complete", CloseReturn: 4.2, Success: true},
	}}
	createBatch := func(date string, items map[string][]SelectionResult) AutomationRun {
		parent, err := store.CreateAutomationRun(AutomationTask{ID: SystemStrategyDailyBatchTaskID, Name: "系统策略日报", Type: "system_strategy_batch"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.FinishAutomationRun(parent.ID, "success", "", `{"strategy_count":2}`, 0); err != nil {
			t.Fatal(err)
		}
		finished := date + "T17:00:00+08:00"
		if _, err := store.db.Exec(`UPDATE automation_runs SET started_at=?, finished_at=? WHERE id=?`, date+"T16:00:00+08:00", finished, parent.ID); err != nil {
			t.Fatal(err)
		}
		matched := 0
		for strategyID, signals := range items {
			child, err := store.CreateAutomationRun(AutomationTask{ID: "system-strategy:" + strategyID, Name: strategyID, Type: "strategy_selection"}, parent.ID)
			if err != nil {
				t.Fatal(err)
			}
			for index := range signals {
				signals[index].Tracking = tracking
				signals[index].CreatedAt = date + "T16:30:00+08:00"
			}
			if err := store.SaveSelectionResults(child, Formula{ID: "strategy:" + strategyID, Name: strategyID}, signals); err != nil {
				t.Fatal(err)
			}
			if err := store.FinishAutomationRun(child.ID, "success", "", `{}`, len(signals)); err != nil {
				t.Fatal(err)
			}
			matched += len(signals)
		}
		if _, err := store.db.Exec(`UPDATE automation_runs SET matched_count=? WHERE id=?`, matched, parent.ID); err != nil {
			t.Fatal(err)
		}
		return parent
	}
	createBatch("2026-09-18", map[string][]SelectionResult{
		"trend":  {{Symbol: "000001", Latest: 10, DetailJSON: `{"score":80}`}, {Symbol: "000002", Latest: 20, DetailJSON: `{"score":60}`}},
		"volume": {{Symbol: "000001", Latest: 10, DetailJSON: `{"score":70}`}},
	})
	createBatch("2026-09-19", map[string][]SelectionResult{
		"trend": {{Symbol: "000001", Latest: 11, DetailJSON: `{"score":85}`}},
	})

	analysis, err := store.BuildDailyStrategyAnalysis("", "2026-09-19", 2, 20000)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.TradingDays != 2 || analysis.SignalCount != 4 || analysis.UniqueSymbols != 2 || analysis.ConsensusCount != 1 {
		t.Fatalf("unexpected summary: %+v", analysis)
	}
	if len(analysis.Strategies) != 2 || analysis.Strategies[0].D5Completed != 3 || analysis.Strategies[0].D5SuccessCount != 3 || analysis.Strategies[0].D5SuccessRate != 100 || analysis.Strategies[0].D5PositiveCount != 3 || analysis.Strategies[0].D5PositiveRate != 100 || analysis.Strategies[0].D5MaxReturn != 4.2 {
		t.Fatalf("unexpected strategy metrics: %+v", analysis.Strategies)
	}
	if analysis.Strategies[0].D1Completed != 3 || analysis.Strategies[0].D1SuccessRate != 100 || analysis.Strategies[0].D1AverageReturn != 1.2 {
		t.Fatalf("unexpected d1 strategy metrics: %+v", analysis.Strategies[0])
	}
	if len(analysis.Stocks) != 3 || len(analysis.Signals) != 4 || len(analysis.Consensus) != 1 {
		t.Fatalf("unexpected analysis lists: stocks=%d signals=%d consensus=%d", len(analysis.Stocks), len(analysis.Signals), len(analysis.Consensus))
	}
	if analysis.Consensus[0].SignalDate != "2026-09-18" || analysis.Consensus[0].Symbol != "000001" || analysis.Consensus[0].StrategyCount != 2 {
		t.Fatalf("unexpected consensus stock: %+v", analysis.Consensus[0])
	}
}
