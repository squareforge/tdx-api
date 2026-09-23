package workbench

import "testing"

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
	tracking := `{"horizons":{"d3":{"status":"complete","close_return":4.2}}}`
	if err := store.SaveSelectionResults(first, Formula{ID: "strategy:trend", Name: "趋势策略"}, []SelectionResult{
		{Symbol: "000001", Latest: 10, DetailJSON: `{"score":80}`, TrackingJSON: tracking},
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
	if trackingCompleteness(review.Consensus[0].TrackingJSON) != 1 {
		t.Fatalf("completed tracking was not retained: %s", review.Consensus[0].TrackingJSON)
	}
	if review.Consensus[0].SignalDate != review.Date {
		t.Fatalf("stock signal date = %q, want %q", review.Consensus[0].SignalDate, review.Date)
	}
}

func TestBuildDailyStrategyAnalysisAggregatesDateRange(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	tracking := `{"horizons":{"d5":{"status":"complete","close_return":4.2,"success":true}}}`
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
				signals[index].TrackingJSON = tracking
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
	if len(analysis.Stocks) != 3 || len(analysis.Signals) != 4 || len(analysis.Consensus) != 1 {
		t.Fatalf("unexpected analysis lists: stocks=%d signals=%d consensus=%d", len(analysis.Stocks), len(analysis.Signals), len(analysis.Consensus))
	}
	if analysis.Consensus[0].SignalDate != "2026-09-18" || analysis.Consensus[0].Symbol != "000001" || analysis.Consensus[0].StrategyCount != 2 {
		t.Fatalf("unexpected consensus stock: %+v", analysis.Consensus[0])
	}
}
