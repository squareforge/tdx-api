package workbench

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func newHistoricalBacktestTestStore(t *testing.T) *AppStore {
	t.Helper()
	db, err := sql.Open("sqlite", "file:historical-backtest-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	store := &AppStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestHistoricalBacktestRunReadsStoredCurrentDate(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateHistoricalBacktestProgress(run.ID, 2, 1, "2026-06-01", 4606, 15); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetHistoricalBacktestRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentDate != "2026-06-01" {
		t.Fatalf("current date = %q, want stored replay date", loaded.CurrentDate)
	}
}

func TestHistoricalBacktestSignalsOrderByStrategyRankScore(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertHistoricalBacktestSignals([]HistoricalBacktestSignal{
		{RunID: run.ID, StrategyID: "strategy-1", StrategyName: "策略A", SignalDate: "2026-06-01", Symbol: "000001", Score: 60},
		{RunID: run.ID, StrategyID: "strategy-1", StrategyName: "策略A", SignalDate: "2026-06-01", Symbol: "000002", Score: 80},
	}); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListHistoricalBacktestSignals(run.ID, HistoricalBacktestSignalQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].Symbol != "000002" || page.Items[1].Symbol != "000001" {
		t.Fatalf("signals = %+v, want descending shared rank score", page.Items)
	}
}

func TestHistoricalBacktestSignalsPersistStructuredTracking(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tracking := SelectionTracking{
		Version: "2", SignalDate: 20260601, BaseDate: 20260601, BasePrice: 10, BasePriceFrom: "signal_day_close",
		TargetReturn: 3, DrawdownLimit: 5, UpdatedAt: "2026-06-06T16:00:00+08:00",
		Horizons: map[string]SelectionHorizon{
			"d3": {HorizonDays: 3, Status: "complete", AsOfDate: 20260604, FinalDate: 20260604, FinalClose: 11, CloseReturn: 10, Success: true},
		},
	}
	if err := store.InsertHistoricalBacktestSignals([]HistoricalBacktestSignal{{
		ID: "historical-selection-1", RunID: run.ID, StrategyID: "strategy-1", StrategyName: "策略A",
		SignalDate: "2026-06-01", Symbol: "000001", Latest: 10, Score: 80, DetailJSON: `{}`, Tracking: &tracking,
	}}); err != nil {
		t.Fatal(err)
	}
	var selectionCount, trackingCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_results WHERE id=? AND task_id=?`, "historical-selection-1", HistoricalBacktestTaskID).Scan(&selectionCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days=3`, "historical-selection-1").Scan(&trackingCount); err != nil {
		t.Fatal(err)
	}
	if selectionCount != 1 || trackingCount != 1 {
		t.Fatalf("structured rows = selection %d, tracking %d; want 1/1", selectionCount, trackingCount)
	}
	page, err := store.ListHistoricalBacktestSignals(run.ID, HistoricalBacktestSignalQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Tracking == nil || page.Items[0].Tracking.Horizons["d3"].CloseReturn != 10 {
		t.Fatalf("structured tracking was not hydrated: %+v", page.Items)
	}
}

func TestHistoricalBacktestSignalsUseSignalDateAsBusinessKey(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, signalDate := range []string{"2026-06-01", "2026-06-02"} {
		if err := store.InsertHistoricalBacktestSignals([]HistoricalBacktestSignal{{
			RunID: run.ID, StrategyID: "strategy-1", StrategyName: "策略A", SignalDate: signalDate,
			Symbol: "000001", Latest: 10, Score: 80, DetailJSON: `{}`,
		}}); err != nil {
			t.Fatalf("insert %s: %v", signalDate, err)
		}
	}
	rows, err := store.db.Query(`SELECT signal_at,signal_price,quote_at,quote_source,price_type,signal_score,snapshot_status
		FROM selection_results WHERE run_id=? ORDER BY signal_at`, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	wantDates := []string{"2026-06-01T16:00:00+08:00", "2026-06-02T16:00:00+08:00"}
	index := 0
	for rows.Next() {
		var signalAt, quoteAt, quoteSource, priceType, snapshotStatus string
		var signalPrice, signalScore float64
		if err := rows.Scan(&signalAt, &signalPrice, &quoteAt, &quoteSource, &priceType, &signalScore, &snapshotStatus); err != nil {
			t.Fatal(err)
		}
		if index >= len(wantDates) || signalAt != wantDates[index] || quoteAt != signalAt || signalPrice != 10 || signalScore != 80 ||
			quoteSource != "historical-backtest" || priceType != "close" || snapshotStatus != "valid" {
			t.Fatalf("selection row %d = at %q price %v quote %q/%q type %q score %v status %q", index, signalAt, signalPrice, quoteAt, quoteSource, priceType, signalScore, snapshotStatus)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(wantDates) {
		t.Fatalf("selection row count = %d, want %d", index, len(wantDates))
	}
}

func TestHistoricalBacktestResultAlwaysStoresJSONObject(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishHistoricalBacktestRun(run.ID, "failed", "null", "测试失败"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetHistoricalBacktestRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ResultJSON != "{}" {
		t.Fatalf("result json = %q, want object fallback", loaded.ResultJSON)
	}
}

func TestHistoricalBacktestListOmitsLargeResult(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	run, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{
		StartDate: "2026-06-01", EndDate: "2026-06-02", StrategyIDs: []string{"strategy-1"},
		StrategySnapshotJSON: `[{"id":"strategy-1"}]`, ResultJSON: `{"strategy_summaries":[{"strategy_id":"strategy-1"}]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListHistoricalBacktestRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ResultJSON != "" || items[0].StrategySnapshotJSON != "" {
		t.Fatalf("list item unexpectedly contains detail payload: %+v", items)
	}
	detail, err := store.GetHistoricalBacktestRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ResultJSON == "" || detail.StrategySnapshotJSON == "" {
		t.Fatalf("detail payload was omitted: %+v", detail)
	}
}

func TestMigrateHistoricalBacktestTrackingJSON(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	legacyResult := `{"consensus":[{"signal_date":"2026-06-01","symbol":"000001","strategy_count":2,"strategies":["策略A","策略B"],"tracking_json":"{\"version\":\"2\",\"horizons\":{\"d3\":{\"horizon_days\":3,\"status\":\"complete\",\"close_return\":4}}}"}]}`
	if _, err := store.CreateHistoricalBacktestRun(HistoricalBacktestRun{ID: "run-legacy", StartDate: "2026-06-01", EndDate: "2026-06-02", ResultJSON: legacyResult}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`ALTER TABLE historical_backtest_signals ADD COLUMN tracking_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
		t.Fatal(err)
	}
	tracking := SelectionTracking{
		Version: "2", SignalDate: 20260601, BaseDate: 20260601, BasePrice: 10, BasePriceFrom: "signal_day_close",
		TargetReturn: 3, DrawdownLimit: 5, UpdatedAt: "2026-06-06T16:00:00+08:00",
		Horizons: map[string]SelectionHorizon{"d3": {HorizonDays: 3, Status: "complete", CloseReturn: 4, Success: true}},
	}
	raw, _ := json.Marshal(tracking)
	if _, err := store.db.Exec(`INSERT INTO historical_backtest_signals
		(id,run_id,strategy_id,strategy_name,signal_date,symbol,latest,score,detail_json,tracking_json,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, "legacy-signal", "run-legacy", "strategy-1", "策略A", "2026-06-01", "000001", 10, 80, `{}`, string(raw), "2026-06-01T16:00:00+08:00"); err != nil {
		t.Fatal(err)
	}
	if err := store.migrateHistoricalBacktestTracking(); err != nil {
		t.Fatal(err)
	}
	hasLegacyColumn, err := store.tableHasColumn("historical_backtest_signals", "tracking_json")
	if err != nil {
		t.Fatal(err)
	}
	if hasLegacyColumn {
		t.Fatal("historical_backtest_signals.tracking_json column still exists")
	}
	var closeReturn float64
	if err := store.db.QueryRow(`SELECT close_return FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days=3`, "legacy-signal").Scan(&closeReturn); err != nil {
		t.Fatal(err)
	}
	if closeReturn != 4 {
		t.Fatalf("migrated close return = %v, want 4", closeReturn)
	}
	var migratedResult string
	if err := store.db.QueryRow(`SELECT result_json FROM historical_backtest_runs WHERE id=?`, "run-legacy").Scan(&migratedResult); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(migratedResult, "tracking_json") || !strings.Contains(migratedResult, `"tracking"`) {
		t.Fatalf("legacy result snapshot was not migrated: %s", migratedResult)
	}
}
