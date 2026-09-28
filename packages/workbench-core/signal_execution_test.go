package workbench

import (
	"testing"
	"time"
)

func TestBuildSelectionSignalExecutions(t *testing.T) {
	item := SelectionResult{
		ID: "signal-1", RunID: "run-1", FormulaID: "strategy:test", Symbol: "000001",
		SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10, QuoteAt: "2026-09-01T14:45:00+08:00",
		QuoteSource: "tdx-realtime", SnapshotStatus: "fresh", NextQuoteAt: "2026-09-01T14:45:05+08:00", NextQuotePrice: 10.2, NextQuoteSource: "tdx-realtime", StrategyVersion: "strategy-v1",
	}
	bars := []TrackingBar{{Date: 20260902, YClose: 10, Open: 10.2, High: 10.5, Low: 10, Close: 10.4}}
	executions := BuildSelectionSignalExecutions(item, bars, time.Date(2026, 9, 1, 15, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if len(executions) != 2 {
		t.Fatalf("executions = %d, want 2", len(executions))
	}
	if executions[0].Status != "filled" || executions[0].ReferencePrice != 10.2 || executions[0].FillPrice != 10.205 || executions[0].TotalCost <= 5 {
		t.Fatalf("unexpected next quote execution: %#v", executions[0])
	}
	if executions[1].Status != "filled" || executions[1].EntryAt == "" || executions[1].FillPrice != 10.21 {
		t.Fatalf("unexpected next open execution: %#v", executions[1])
	}
}

func TestBuildSelectionSignalExecutionsDoesNotFillFromSignalSnapshot(t *testing.T) {
	item := SelectionResult{
		ID: "signal-same-snapshot", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00",
		SignalPrice: 10, Latest: 10, QuoteAt: "2026-09-01T14:45:05+08:00", QuoteSource: "tdx-realtime", SnapshotStatus: "fresh",
	}
	executions := BuildSelectionSignalExecutions(item, nil, time.Date(2026, 9, 1, 14, 45, 1, 0, time.FixedZone("CST", 8*3600)))
	if executions[0].Status != "planned" || executions[0].FillPrice != 0 {
		t.Fatalf("same signal snapshot must stay planned: %#v", executions[0])
	}
}

func TestBuildSelectionSignalExecutionsRejectsLimitUpAndKeepsPlan(t *testing.T) {
	item := SelectionResult{ID: "signal-2", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	planned := BuildSelectionSignalExecutions(item, nil, time.Now())
	if planned[1].Status != "planned" || planned[1].FailureReason == "" {
		t.Fatalf("expected next-open plan: %#v", planned[1])
	}
	limitUp := BuildSelectionSignalExecutions(item, []TrackingBar{{Date: 20260902, YClose: 10, Open: 11, High: 11, Low: 11, Close: 11}}, time.Now())
	if limitUp[1].Status != "rejected" || limitUp[1].FailureReason == "" {
		t.Fatalf("expected limit-up rejection: %#v", limitUp[1])
	}
}

func TestBuildSelectionObservationsUsesBothBases(t *testing.T) {
	item := SelectionResult{ID: "signal-3", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	execution := SelectionSignalExecution{ID: "execution-1", Status: "filled", FillPrice: 10.2}
	rows := BuildSelectionObservations(item, execution, []TrackingBar{
		{Date: 20260901, Open: 9, High: 12, Low: 8, Close: 11},
		{Date: 20260902, Open: 10.2, High: 11.22, Low: 10, Close: 11},
	}, time.Now())
	if len(rows) != 2 || rows[1].SignalCloseReturn != 10 || rows[1].TradeCloseReturn != 7.84 {
		t.Fatalf("unexpected observation: %#v", rows)
	}
	if rows[0].SessionKind != "d0" || rows[0].SymbolDayIndex != 0 || rows[0].DataStatus != "partial" || rows[0].SignalHighReturn != 0 || rows[0].SignalLowReturn != 0 {
		t.Fatalf("D0 must retain close only without whole-day path returns: %#v", rows[0])
	}
}

func TestBuildSelectionObservationsOmitsD0AfterCloseSignal(t *testing.T) {
	item := SelectionResult{ID: "signal-d0", Symbol: "000001", SignalAt: "2026-09-01T16:00:00+08:00", SignalPrice: 10}
	rows := BuildSelectionObservations(item, SelectionSignalExecution{}, []TrackingBar{
		{Date: 20260901, Open: 9, High: 12, Low: 8, Close: 11},
		{Date: 20260902, Open: 11, High: 11.5, Low: 10.5, Close: 11.2},
	}, time.Now())
	if len(rows) != 1 || rows[0].SessionKind != "forward" || rows[0].SymbolDayIndex != 1 {
		t.Fatalf("after-close signal must not have a D0 close observation: %#v", rows)
	}
}

func TestBuildSelectionObservationsCountsMarketDaysDuringSuspension(t *testing.T) {
	item := SelectionResult{ID: "signal-suspension", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	stockBars := []TrackingBar{
		{Date: 20260901, Open: 10, High: 10, Low: 10, Close: 10},
		{Date: 20260904, Open: 11, High: 11, Low: 11, Close: 11},
	}
	marketBars := []TrackingBar{
		{Date: 20260901, Open: 100, High: 100, Low: 100, Close: 100},
		{Date: 20260902, Open: 101, High: 101, Low: 101, Close: 101},
		{Date: 20260903, Open: 102, High: 102, Low: 102, Close: 102},
		{Date: 20260904, Open: 103, High: 103, Low: 103, Close: 103},
	}
	rows := BuildSelectionObservations(item, SelectionSignalExecution{}, stockBars, time.Now(), marketBars)
	if len(rows) != 2 || rows[1].MarketDayIndex != 3 || rows[1].SymbolDayIndex != 1 {
		t.Fatalf("market/symbol day indexes after suspension: %#v", rows)
	}
}

func TestNextQuoteCannotFillOutsideSignalSession(t *testing.T) {
	now := time.Date(2026, 9, 2, 17, 15, 0, 0, time.FixedZone("CST", 8*3600))
	item := SelectionResult{ID: "signal-late", Symbol: "000001", SignalAt: "2026-09-01T17:15:00+08:00", SignalPrice: 10}
	if got := BuildSelectionSignalExecutions(item, nil, now)[0]; got.Status != "unavailable" {
		t.Fatalf("after-close signal was treated as executable: %#v", got)
	}
	item.SignalAt = "2026-09-01T14:45:00+08:00"
	item.NextQuoteAt = "2026-09-02T17:15:00+08:00"
	item.NextQuotePrice = 10.2
	if got := BuildSelectionSignalExecutions(item, nil, now)[0]; got.Status != "unavailable" {
		t.Fatalf("next-day close was treated as next quote: %#v", got)
	}
	item.NextQuoteAt, item.NextQuotePrice = "", 0
	if got := BuildSelectionSignalExecutions(item, nil, time.Date(2026, 9, 1, 14, 50, 0, 0, time.FixedZone("CST", 8*3600)))[0]; got.Status != "unavailable" {
		t.Fatalf("expired quote window was left pending: %#v", got)
	}
}

func TestEvaluateSelectionExecutionTrackingIncludesExitFees(t *testing.T) {
	item := SelectionResult{ID: "signal-fees", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	execution := SelectionSignalExecution{ID: "execution-fees", Status: "filled", FillPrice: 10, Quantity: 100, GrossAmount: 1000, TotalCost: 5}
	observations := []SelectionSignalObservation{{
		SelectionResultID: item.ID, ExecutionID: execution.ID, Symbol: item.Symbol, TradeDate: 20260902,
		SymbolDayIndex: 1, SessionKind: "forward", Open: 11, High: 11, Low: 11, Close: 11, TradeCloseReturn: 10,
	}}
	tracking := EvaluateSelectionExecutionTracking(item, execution, observations, []int{1}, 0, 0, time.Now())
	horizon := tracking.Horizons["d1"]
	if horizon.GrossReturn != 10 {
		t.Fatalf("gross return should remain fee-free: %#v", horizon)
	}
	if horizon.TotalCost <= execution.TotalCost || horizon.NetReturn >= horizon.GrossReturn {
		t.Fatalf("exit fees must reduce net return: %#v", horizon)
	}
}
