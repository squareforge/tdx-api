package workbench

import (
	"fmt"
	"testing"
	"time"
)

func TestSelectionSignalExecutionAndObservationPersistenceIsIdempotent(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run := AutomationRun{ID: "run-execution", TaskID: "task", TaskName: "task"}
	item := SelectionResult{ID: "selection-execution", Symbol: "000001", Latest: 10, SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10, QuoteAt: "2026-09-01T14:45:05+08:00", QuoteSource: "test", SnapshotStatus: "fresh", DetailJSON: `{}`, CreatedAt: "2026-09-01T14:45:00+08:00"}
	if err := store.SaveSelectionResults(run, Formula{ID: "formula", Name: "formula"}, []SelectionResult{item}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_signal_executions WHERE selection_result_id=?`, item.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("execution count = %d, want 2", count)
	}
	if err := store.EnsureSelectionSignalExecutions([]SelectionResult{item}, map[string][]TrackingBar{
		"000001": {{Date: 20260902, YClose: 10, Open: 10.2, High: 10.4, Low: 10, Close: 10.3}},
	}); err != nil {
		t.Fatal(err)
	}
	executions, err := store.ListSelectionSignalExecutions(item.ID, "next_day_open")
	if err != nil || len(executions) != 1 || executions[0].Status != "filled" {
		t.Fatalf("next day execution = %#v, err=%v", executions, err)
	}
	rows := BuildSelectionObservations(item, executions[0], []TrackingBar{{Date: 20260902, Open: 10.2, High: 10.4, Low: 10, Close: 10.3}}, time.Now())
	if err := store.SaveSelectionSignalObservations(rows); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.ListSelectionSignalObservations(item.ID, executions[0].ID)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("observations = %#v, err=%v", loaded, err)
	}
	if err := store.SaveSelectionResults(run, Formula{ID: "formula", Name: "formula"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_signal_executions WHERE run_id=?`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("retry removed executions: got %d, want 2", count)
	}
	loaded, err = store.ListSelectionSignalObservations(item.ID, executions[0].ID)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("retry removed observations: %#v, err=%v", loaded, err)
	}
}

func TestSelectionSignalExecutionKeepsFilledNextQuoteImmutable(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run := AutomationRun{ID: "run-next-quote", TaskID: "task", TaskName: "task"}
	signalAt := time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, 1)
	signalAt = time.Date(signalAt.Year(), signalAt.Month(), signalAt.Day(), 14, 45, 0, 0, signalAt.Location())
	item := SelectionResult{ID: "selection-next-quote", Symbol: "000001", Latest: 10, SignalAt: signalAt.Format(time.RFC3339), SignalPrice: 10, QuoteAt: signalAt.Format(time.RFC3339), QuoteSource: "test", SnapshotStatus: "fresh", DetailJSON: `{}`, CreatedAt: signalAt.Format(time.RFC3339)}
	if err := store.SaveSelectionResults(run, Formula{ID: "formula", Name: "formula"}, []SelectionResult{item}); err != nil {
		t.Fatal(err)
	}
	filled := item
	filled.NextQuoteAt = signalAt.Add(5 * time.Second).Format(time.RFC3339)
	filled.NextQuotePrice = 10.2
	filled.NextQuoteSource = "test-follow-up"
	if err := store.EnsureSelectionSignalExecutions([]SelectionResult{filled}, nil); err != nil {
		t.Fatal(err)
	}
	changed := filled
	changed.NextQuoteAt = signalAt.Add(10 * time.Second).Format(time.RFC3339)
	changed.NextQuotePrice = 10.5
	if err := store.EnsureSelectionSignalExecutions([]SelectionResult{changed}, nil); err != nil {
		t.Fatal(err)
	}
	executions, err := store.ListSelectionSignalExecutions(item.ID, "next_quote")
	if err != nil || len(executions) != 1 {
		t.Fatalf("executions = %#v, err=%v", executions, err)
	}
	if executions[0].Status != "filled" || executions[0].ReferencePrice != 10.2 {
		t.Fatalf("filled execution was overwritten: %#v", executions[0])
	}
}

func TestListIncompleteSelectionTrackingResults(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	rows := []struct {
		id       string
		tracking *SelectionTracking
	}{
		{id: "empty"},
		{id: "pending", tracking: &SelectionTracking{Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete"}, "d5": {HorizonDays: 5, Status: "pending"}, "d10": {HorizonDays: 10, Status: "pending"},
		}}},
		{id: "missing", tracking: &SelectionTracking{Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete"}, "d5": {HorizonDays: 5, Status: "complete"},
		}}},
		{id: "retryable", tracking: &SelectionTracking{Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete"}, "d5": {HorizonDays: 5, Status: "complete"}, "d10": {HorizonDays: 10, Status: "retryable_error"},
		}}},
		{id: "complete", tracking: &SelectionTracking{Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete"}, "d5": {HorizonDays: 5, Status: "complete"}, "d10": {HorizonDays: 10, Status: "complete"},
		}}},
		{id: "unavailable", tracking: &SelectionTracking{Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "unavailable"}, "d5": {HorizonDays: 5, Status: "unavailable"}, "d10": {HorizonDays: 10, Status: "unavailable"},
		}}},
	}
	itemsToSave := make([]SelectionResult, 0, len(rows))
	for index, row := range rows {
		createdAt := fmt.Sprintf("2026-09-%02dT15:00:00+08:00", index+1)
		itemsToSave = append(itemsToSave, SelectionResult{
			ID: row.id, Symbol: fmt.Sprintf("%06d", index+1), Latest: 10, DetailJSON: `{}`, Tracking: row.tracking, CreatedAt: createdAt,
		})
	}
	if err := store.SaveSelectionResults(AutomationRun{ID: "run", TaskID: "task", TaskName: "task"}, Formula{ID: "formula", Name: "formula"}, itemsToSave); err != nil {
		t.Fatal(err)
	}

	items, err := store.ListIncompleteSelectionTrackingResults([]int{3, 5, 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"empty", "pending", "missing", "retryable"}
	if len(items) != len(want) {
		t.Fatalf("items = %d, want %d: %+v", len(items), len(want), items)
	}
	for index, id := range want {
		if items[index].ID != id {
			t.Fatalf("items[%d].ID = %q, want %q", index, items[index].ID, id)
		}
	}
}

func TestSaveSelectionResultsPreservesSnapshotEvidenceAndIsIdempotent(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run := AutomationRun{ID: "run-snapshot", TaskID: "task", TaskName: "task"}
	formula := Formula{ID: "formula", Name: "策略"}
	items := []SelectionResult{
		{ID: "inferred", Symbol: "000001", Latest: 10, DetailJSON: `{}`, CreatedAt: "2026-09-01T14:45:00+08:00"},
		{ID: "inferred-duplicate", Symbol: "000001", Latest: 11, DetailJSON: `{}`, CreatedAt: "2026-09-01T14:45:00+08:00"},
		{ID: "explicit", Symbol: "000002", Latest: 20, SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 20.1, QuoteAt: "2026-09-01T14:44:59+08:00", QuoteSource: "tdx-realtime", SnapshotStatus: "fresh", DetailJSON: `{}`, CreatedAt: "2026-09-01T14:45:00+08:00"},
	}
	if err := store.SaveSelectionResults(run, formula, items); err != nil {
		t.Fatal(err)
	}

	var quoteAt, quoteSource, snapshotStatus string
	var signalPrice float64
	if err := store.db.QueryRow(`SELECT signal_price,quote_at,quote_source,snapshot_status FROM selection_results WHERE id=?`, "inferred").Scan(&signalPrice, &quoteAt, &quoteSource, &snapshotStatus); err != nil {
		t.Fatal(err)
	}
	if signalPrice != 10 || quoteAt != "" || quoteSource != "" || snapshotStatus != "inferred" {
		t.Fatalf("legacy snapshot = price %v, quote_at %q, source %q, status %q", signalPrice, quoteAt, quoteSource, snapshotStatus)
	}
	if err := store.db.QueryRow(`SELECT quote_at,quote_source,snapshot_status FROM selection_results WHERE id=?`, "explicit").Scan(&quoteAt, &quoteSource, &snapshotStatus); err != nil {
		t.Fatal(err)
	}
	if quoteAt != "2026-09-01T14:44:59+08:00" || quoteSource != "tdx-realtime" || snapshotStatus != "fresh" {
		t.Fatalf("explicit snapshot = quote_at %q, source %q, status %q", quoteAt, quoteSource, snapshotStatus)
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_results WHERE run_id=?`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("business-key dedup count = %d, want 2", count)
	}
	if err := store.SaveSelectionResults(run, formula, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_results WHERE run_id=?`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("empty retry changed %d selection results, want 2", count)
	}
}

func TestSaveSelectionResultsRetryKeepsOriginalSignalIdentity(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run := AutomationRun{ID: "run-retry", TaskID: "task", TaskName: "task"}
	formula := Formula{ID: "formula", Name: "formula"}
	first := SelectionResult{ID: "original", Symbol: "000001", Latest: 10, SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10, CreatedAt: "2026-09-01T14:45:00+08:00"}
	if err := store.SaveSelectionResults(run, formula, []SelectionResult{first}); err != nil {
		t.Fatal(err)
	}
	retry := SelectionResult{ID: "replacement", Symbol: "000001", Latest: 11, SignalAt: "2026-09-01T14:46:00+08:00", SignalPrice: 11, CreatedAt: "2026-09-01T14:46:00+08:00"}
	if err := store.SaveSelectionResults(run, formula, []SelectionResult{retry}); err != nil {
		t.Fatal(err)
	}
	var id string
	var count int
	if err := store.db.QueryRow(`SELECT id,COUNT(*) FROM selection_results WHERE run_id=? AND formula_id=?`, run.ID, formula.ID).Scan(&id, &count); err != nil {
		t.Fatal(err)
	}
	if id != first.ID || count != 1 {
		t.Fatalf("retry changed original signal: id=%q count=%d", id, count)
	}
}

func TestListIncompleteSelectionTrackingResultsHasNoTotalLimit(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	items := make([]SelectionResult, 550)
	for index := range items {
		items[index] = SelectionResult{ID: fmt.Sprintf("pending-%03d", index), Symbol: fmt.Sprintf("%06d", index), Latest: 10, DetailJSON: `{}`}
	}
	if err := store.SaveSelectionResults(AutomationRun{ID: "run"}, Formula{}, items); err != nil {
		t.Fatal(err)
	}
	incomplete, err := store.ListIncompleteSelectionTrackingResults([]int{3, 5, 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(incomplete) != 550 {
		t.Fatalf("items = %d, want 550", len(incomplete))
	}
}

func TestUpdateSelectionTrackingPersistsStructuredRows(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	if err := store.SaveSelectionResults(AutomationRun{ID: "run-1", TaskID: "task-1", TaskName: "策略任务"}, Formula{ID: "strategy:alpha", Name: "Alpha策略"}, []SelectionResult{{
		ID: "selection-1", Symbol: "000001", Latest: 10, DetailJSON: `{}`, CreatedAt: "2026-09-01T15:00:00+08:00",
	}}); err != nil {
		t.Fatal(err)
	}

	tracking := SelectionTracking{
		Version: "2", SignalDate: 20260901, BaseDate: 20260901, BasePrice: 10,
		BasePriceFrom: "signal_day_close", TargetReturn: 3, DrawdownLimit: 5,
		UpdatedAt: "2026-09-04T16:00:00+08:00",
		Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete", AsOfDate: 20260904, FinalDate: 20260904, FinalOpen: 10.5,
				FinalClose: 11, OpenReturn: 5, CloseReturn: 10, MaxGain: 12, MaxGainDate: 20260903,
				MaxGainPrice: 11.2, DaysToMaxGain: 2, MaxDrawdown: -1, MaxDrawdownDate: 20260902,
				MaxDrawdownPrice: 9.9, DaysToMaxDrawdown: 1, TargetReturn: 3, TargetHit: true,
				TargetHitDate: 20260902, DaysToTargetHit: 1, DrawdownLimit: 5, Success: true, Reason: "达到收益与回撤标准"},
			"d5": {HorizonDays: 5, Status: "pending", AsOfDate: 20260904, TargetReturn: 3, DrawdownLimit: 5, Reason: "已获得3/5个交易日"},
		},
	}
	if err := store.UpdateSelectionTracking("selection-1", tracking); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_result_tracking WHERE selection_result_id=?`, "selection-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("structured tracking rows = %d, want 2", count)
	}
	var closeReturn, maxGain float64
	var targetHit, success int
	if err := store.db.QueryRow(`SELECT close_return,max_gain,target_hit,success FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days=3`, "selection-1").
		Scan(&closeReturn, &maxGain, &targetHit, &success); err != nil {
		t.Fatal(err)
	}
	if closeReturn != 10 || maxGain != 12 || targetHit != 1 || success != 1 {
		t.Fatalf("unexpected d3 row: close=%v max=%v target=%d success=%d", closeReturn, maxGain, targetHit, success)
	}
	items, err := store.ListSelectionResults("", "", "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Tracking == nil {
		t.Fatalf("structured tracking was not loaded: %+v", items)
	}
	if got := items[0].Tracking.Horizons["d3"].CloseReturn; got != 10 {
		t.Fatalf("hydrated d3 close return = %v, want 10", got)
	}

	if err := store.UpdateSelectionTracking("selection-1", SelectionTracking{}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_result_tracking WHERE selection_result_id=?`, "selection-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("structured tracking rows after empty update = %d, want 0", count)
	}
}

func TestMigrateLegacySelectionTrackingDropsColumnAfterBackfill(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	if _, err := store.db.Exec(`ALTER TABLE selection_results ADD COLUMN tracking_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":"2","signal_date":20260901,"base_date":20260901,"base_price":10,"base_price_from":"signal_day_close","target_return":3,"drawdown_limit":5,"updated_at":"2026-09-04T16:00:00+08:00","horizons":{"d3":{"horizon_days":3,"status":"complete","as_of_date":20260904,"close_return":4,"target_return":3,"drawdown_limit":5,"success":true,"reason":"达到收益与回撤标准"}}}`
	if _, err := store.db.Exec(`INSERT INTO selection_results
		(id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,detail_json,tracking_json,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, "selection-backfill", "run-1", "task-1", "策略任务", "strategy:alpha", "Alpha策略", "000001", 10, `{}`, legacy, "2026-09-01T15:00:00+08:00"); err != nil {
		t.Fatal(err)
	}
	if err := store.migrateLegacySelectionTracking(); err != nil {
		t.Fatal(err)
	}
	hasLegacyColumn, err := store.tableHasColumn("selection_results", "tracking_json")
	if err != nil {
		t.Fatal(err)
	}
	if hasLegacyColumn {
		t.Fatal("selection_results.tracking_json column still exists")
	}
	var count int
	var closeReturn float64
	if err := store.db.QueryRow(`SELECT COUNT(*),MAX(close_return) FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days=3`, "selection-backfill").Scan(&count, &closeReturn); err != nil {
		t.Fatal(err)
	}
	if count != 1 || closeReturn != 4 {
		t.Fatalf("backfilled tracking rows = %d, close_return = %v", count, closeReturn)
	}
	if err := store.migrateLegacySelectionTracking(); err != nil {
		t.Fatalf("migration should be idempotent: %v", err)
	}
}
