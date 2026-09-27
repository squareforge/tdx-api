package workbench

import (
	"testing"
	"time"
)

func TestEvaluateSelectionTrackingAfterCloseStartsNextSession(t *testing.T) {
	item := SelectionResult{ID: "hit-1", Symbol: "000001", Latest: 100, CreatedAt: "2026-01-01T18:00:00+08:00"}
	bars := []TrackingBar{
		{Date: 20260102, Open: 101, High: 103, Low: 99, Close: 102},
		{Date: 20260103, Open: 102, High: 104, Low: 98, Close: 101},
		{Date: 20260104, Open: 101, High: 105, Low: 100, Close: 103},
		{Date: 20260105, Open: 103, High: 106, Low: 101, Close: 104},
		{Date: 20260106, Open: 104, High: 107, Low: 102, Close: 104},
	}
	tracking := EvaluateSelectionTracking(item, bars, []int{1, 5, 10}, 3, 5, time.Date(2026, 1, 8, 18, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if tracking.Version != "2" || tracking.SignalDate != 20260101 || tracking.BasePrice != 100 || tracking.BasePriceFrom != "selection_latest" {
		t.Fatalf("unexpected signal/base: %#v", tracking)
	}
	d1 := tracking.Horizons["d1"]
	if d1.Status != "complete" || d1.CloseReturn != 2 || d1.Success {
		t.Fatalf("unexpected d1: %#v", d1)
	}
	d5 := tracking.Horizons["d5"]
	if d5.Status != "complete" || d5.CloseReturn != 4 || d5.MaxDrawdown != -2 || d5.AverageClose != 102.8 || d5.AverageCloseReturn != 2.8 || !d5.Success {
		t.Fatalf("unexpected d5: %#v", d5)
	}
	if tracking.Horizons["d10"].Status != "pending" {
		t.Fatalf("expected pending d10: %#v", tracking.Horizons["d10"])
	}
}

func TestEvaluateSelectionTrackingPreOpenUsesSignalDayOpen(t *testing.T) {
	item := SelectionResult{ID: "hit-2", Symbol: "000002", Latest: 98, CreatedAt: "2026-09-08T08:14:02+08:00"}
	bars := []TrackingBar{
		{Date: 20260907, Open: 97, High: 99, Low: 96, Close: 98},
		{Date: 20260908, YClose: 98, Open: 100, High: 104, Low: 99, Close: 103},
		{Date: 20260909, YClose: 103, Open: 102, High: 105, Low: 101, Close: 104},
	}
	tracking := EvaluateSelectionTracking(item, bars, []int{1, 5}, 3, 5, time.Now())
	if tracking.BasePrice != 100 || tracking.BaseDate != 20260908 || tracking.BasePriceFrom != "signal_day_open" {
		t.Fatalf("expected signal-day open as base: %#v", tracking)
	}
	d1 := tracking.Horizons["d1"]
	if d1.Status != "complete" || d1.AsOfDate != 20260908 || d1.OpenReturn != 0 || d1.CloseReturn != 3 || !d1.Success {
		t.Fatalf("unexpected pre-open d1: %#v", d1)
	}
	d5 := tracking.Horizons["d5"]
	if d5.Status != "pending" || d5.AsOfDate != 20260909 || d5.Reason != "已获得2/5个交易日" {
		t.Fatalf("unexpected pre-open d5: %#v", d5)
	}
}

func TestEvaluateSelectionTrackingIgnoresIncompleteBars(t *testing.T) {
	item := SelectionResult{ID: "hit-3", Symbol: "000003", Latest: 10, CreatedAt: "2026-09-08T18:00:00+08:00"}
	bars := []TrackingBar{
		{Date: 20260909, YClose: 10, Open: 10, High: 11, Low: 10, Close: 11},
		{Date: 20260910, YClose: 11, Open: 0, High: 0, Low: 0, Close: 11},
		{Date: 20260911, YClose: 11, Open: 11, High: 12, Low: 10.5, Close: 11.5},
	}
	tracking := EvaluateSelectionTracking(item, bars, []int{1, 5}, 3, 5, time.Now())
	d1 := tracking.Horizons["d1"]
	if d1.Status != "complete" || d1.MaxDrawdown != 0 {
		t.Fatalf("incomplete bar affected d1: %#v", d1)
	}
	d5 := tracking.Horizons["d5"]
	if d5.Status != "pending" || d5.AsOfDate != 20260911 || d5.Reason != "已获得2/5个交易日" {
		t.Fatalf("incomplete bar counted toward horizon: %#v", d5)
	}
}

func TestSummarizeSelectionTracking(t *testing.T) {
	items := []SelectionTrackingItem{
		{Tracking: SelectionTracking{Horizons: map[string]SelectionHorizon{"d5": {Status: "complete", CloseReturn: 4, MaxDrawdown: -2, Success: true}}}},
		{Tracking: SelectionTracking{Horizons: map[string]SelectionHorizon{"d5": {Status: "complete", CloseReturn: -1, MaxDrawdown: -6, Success: false}}}},
	}
	summary := SummarizeSelectionTracking(items, []int{5})
	d5 := summary.Horizons["d5"]
	if d5.Completed != 2 || d5.SuccessCount != 1 || d5.SuccessRate != 50 || d5.AverageCloseReturn != 1.5 {
		t.Fatalf("unexpected summary: %#v", d5)
	}
}
