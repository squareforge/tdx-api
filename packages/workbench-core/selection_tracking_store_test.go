package workbench

import (
	"fmt"
	"testing"
)

func TestListIncompleteSelectionTrackingResults(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	rows := []struct {
		id       string
		tracking string
	}{
		{id: "invalid", tracking: `not-json`},
		{id: "empty", tracking: `{}`},
		{id: "pending", tracking: `{"horizons":{"d3":{"status":"complete"},"d5":{"status":"pending"},"d10":{"status":"pending"}}}`},
		{id: "missing", tracking: `{"horizons":{"d3":{"status":"complete"},"d5":{"status":"complete"}}}`},
		{id: "retryable", tracking: `{"horizons":{"d3":{"status":"complete"},"d5":{"status":"complete"},"d10":{"status":"retryable_error"}}}`},
		{id: "complete", tracking: `{"horizons":{"d3":{"status":"complete"},"d5":{"status":"complete"},"d10":{"status":"complete"}}}`},
		{id: "unavailable", tracking: `{"horizons":{"d3":{"status":"unavailable"},"d5":{"status":"unavailable"},"d10":{"status":"unavailable"}}}`},
	}
	for index, row := range rows {
		createdAt := fmt.Sprintf("2026-09-%02dT15:00:00+08:00", index+1)
		if _, err := store.db.Exec(`INSERT INTO selection_results
			(id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,detail_json,tracking_json,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, row.id, "run", "task", "task", "formula", "formula", fmt.Sprintf("%06d", index+1), 10, `{}`, row.tracking, createdAt); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.ListIncompleteSelectionTrackingResults([]int{3, 5, 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"invalid", "empty", "pending", "missing", "retryable"}
	if len(items) != len(want) {
		t.Fatalf("items = %d, want %d: %+v", len(items), len(want), items)
	}
	for index, id := range want {
		if items[index].ID != id {
			t.Fatalf("items[%d].ID = %q, want %q", index, items[index].ID, id)
		}
	}
}

func TestListIncompleteSelectionTrackingResultsHasNoTotalLimit(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	for index := 0; index < 550; index++ {
		id := fmt.Sprintf("pending-%03d", index)
		if _, err := store.db.Exec(`INSERT INTO selection_results
			(id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,detail_json,tracking_json,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, id, "run", "task", "task", "formula", "formula", fmt.Sprintf("%06d", index), 10, `{}`, `{}`, "2026-09-01T15:00:00+08:00"); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.ListIncompleteSelectionTrackingResults([]int{3, 5, 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 550 {
		t.Fatalf("items = %d, want 550", len(items))
	}
}
