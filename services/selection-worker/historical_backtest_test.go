package main

import (
	"testing"
	"time"
)

func TestHistoricalRowsThroughDateDoesNotExposeFutureBars(t *testing.T) {
	rows := []FormulaKline{
		{Date: 20260105, Close: 10},
		{Date: 20260106, Close: 11},
		{Date: 20260107, Close: 12},
	}
	visible, ok := historicalRowsThroughDate(rows, 20260106)
	if !ok {
		t.Fatal("expected signal date to be available")
	}
	if len(visible) != 2 || visible[len(visible)-1].Date != 20260106 {
		t.Fatalf("visible rows = %+v, want rows through 20260106", visible)
	}
	if _, ok := historicalRowsThroughDate(rows, 20260108); ok {
		t.Fatal("non-trading date must not reuse a stale bar")
	}
}

func TestRankHistoricalMatchesAppliesDailyTopN(t *testing.T) {
	items := []StrategySelectionItem{
		{Symbol: "000003", Score: 70},
		{Symbol: "000002", Score: 80},
		{Symbol: "000001", Score: 80},
	}
	ranked := rankHistoricalMatches(items, 2)
	if len(ranked) != 2 {
		t.Fatalf("len(ranked) = %d, want 2", len(ranked))
	}
	if ranked[0].Symbol != "000001" || ranked[1].Symbol != "000002" {
		t.Fatalf("ranked symbols = %s,%s, want stable score/symbol order", ranked[0].Symbol, ranked[1].Symbol)
	}
}

func TestNormalizeHistoricalBacktestRequestDefaults(t *testing.T) {
	req, _, _, err := normalizeHistoricalBacktestRequest(historicalBacktestRequest{StartDate: "2026-01-01", EndDate: "2026-02-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Horizons) != 3 || req.Horizons[0] != 3 || req.Horizons[1] != 5 || req.Horizons[2] != 10 {
		t.Fatalf("horizons = %v, want [3 5 10]", req.Horizons)
	}
	if req.TargetReturn != 10 || req.DrawdownLimit != 5 {
		t.Fatalf("达标标准 = %.1f/%.1f, want 10/5(复用止盈/回撤)", req.TargetReturn, req.DrawdownLimit)
	}
	if req.TakeProfit != 10 || req.TrailDrawdown != 5 || req.StopLoss != 3 {
		t.Fatalf("退出参数 = %.1f/%.1f/%.1f, want 10/5/3", req.TakeProfit, req.TrailDrawdown, req.StopLoss)
	}
	if req.MaxHoldDays != 15 || req.BuyBase != "open" || req.SellBase != "next_open" {
		t.Fatalf("买卖参数 = %d/%s/%s, want 15/open/next_open", req.MaxHoldDays, req.BuyBase, req.SellBase)
	}
}

func TestDefaultScanGridIncludesShortSwingExits(t *testing.T) {
	grid := defaultScanGrid()
	if len(grid.combinations()) != 240 {
		t.Fatalf("scan combinations = %d, want 240", len(grid.combinations()))
	}
	if !containsScanFloat(grid.TakeProfits, 2) || !containsScanFloat(grid.TakeProfits, 3) {
		t.Fatalf("take profits = %v, want short-swing 2%% and 3%% targets", grid.TakeProfits)
	}
	if !containsScanInt(grid.MaxHoldDays, 15) {
		t.Fatalf("max hold days = %v, want 15-day candidate", grid.MaxHoldDays)
	}
}

func containsScanFloat(values []float64, target float64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsScanInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestHistoricalHistoryCountIncludesGapFromReplayToLatestData(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, location)
	end := time.Date(2024, 1, 2, 0, 0, 0, 0, location)
	reference := time.Date(2026, 6, 5, 0, 0, 0, 0, location)

	count := historicalHistoryCount(start, end, reference, 10)
	if count <= 260 {
		t.Fatalf("history count = %d, want enough rows to bridge an old narrow replay", count)
	}
}

func TestHistoricalHistoryCountUsesMinimumAndMaximumBounds(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	reference := time.Date(2026, 6, 5, 0, 0, 0, 0, location)
	if count := historicalHistoryCount(reference, reference, reference, 10); count != 260 {
		t.Fatalf("minimum history count = %d, want 260", count)
	}
	oldStart := time.Date(2000, 1, 1, 0, 0, 0, 0, location)
	if count := historicalHistoryCount(oldStart, oldStart, reference, 10); count != 2000 {
		t.Fatalf("maximum history count = %d, want 2000", count)
	}
}
