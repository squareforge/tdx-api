package workbench

import (
	"database/sql"
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
