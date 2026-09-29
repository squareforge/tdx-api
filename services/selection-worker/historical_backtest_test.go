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
		{Symbol: "000003", Score: 70, RankScore: 70, StrategyScore: 90},
		{Symbol: "000002", Score: 80, RankScore: 80, StrategyScore: 70},
		{Symbol: "000001", Score: 80, RankScore: 80, StrategyScore: 80},
	}
	ranked := rankHistoricalMatches(items, 2)
	if len(ranked) != 2 {
		t.Fatalf("len(ranked) = %d, want 2", len(ranked))
	}
	if ranked[0].Symbol != "000001" || ranked[1].Symbol != "000002" {
		t.Fatalf("ranked symbols = %s,%s, want rank score then strategy score order", ranked[0].Symbol, ranked[1].Symbol)
	}
	if ranked[0].Rank != 1 || ranked[1].Rank != 2 {
		t.Fatalf("ranks = %d,%d, want 1,2", ranked[0].Rank, ranked[1].Rank)
	}
}

func TestNormalizeHistoricalBacktestRequestDefaults(t *testing.T) {
	req, _, _, err := normalizeHistoricalBacktestRequest(historicalBacktestRequest{StartDate: "2026-01-01", EndDate: "2026-02-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Horizons) != 4 || req.Horizons[0] != 1 || req.Horizons[1] != 3 || req.Horizons[2] != 5 || req.Horizons[3] != 10 {
		t.Fatalf("horizons = %v, want [1 3 5 10]", req.Horizons)
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

func TestNormalizeHistoricalBacktestRequestNormalizesUniversePools(t *testing.T) {
	req, _, _, err := normalizeHistoricalBacktestRequest(historicalBacktestRequest{
		StartDate: "2026-01-01", EndDate: "2026-02-01", UniversePoolID: "  market-gem  ",
		UniversePoolIDs: []string{"market-sz-main", "market-gem", " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(req.UniversePoolIDs) != 2 || req.UniversePoolIDs[0] != "market-sz-main" || req.UniversePoolIDs[1] != "market-gem" {
		t.Fatalf("universe pools = %v, want normalized unique pool ids", req.UniversePoolIDs)
	}
}

func TestHistoricalBacktestUniverseMergesAndDeduplicatesPools(t *testing.T) {
	symbols := mergeHistoricalSymbols(
		[]string{"000001", "000002"},
		[]string{"000002", "600000"},
		[]string{"sz000001"},
	)
	want := []string{"000001", "000002", "600000"}
	if len(symbols) != len(want) {
		t.Fatalf("merged symbols = %v, want %v", symbols, want)
	}
	for index := range want {
		if symbols[index] != want[index] {
			t.Fatalf("merged symbols = %v, want %v", symbols, want)
		}
	}
}

func TestIntersectHistoricalSymbols(t *testing.T) {
	strategySymbols := []string{"000001", "300001", "688001"}
	if got := intersectHistoricalSymbols(strategySymbols, nil); len(got) != len(strategySymbols) {
		t.Fatalf("no limit returned %v, want original strategy range", got)
	}
	got := intersectHistoricalSymbols(strategySymbols, []string{"300001", "600000"})
	if len(got) != 1 || got[0] != "300001" {
		t.Fatalf("intersection = %v, want [300001]", got)
	}
}

func TestDefaultScanGridIncludesShortSwingExits(t *testing.T) {
	grid := defaultScanGrid()
	if len(grid.combinations()) != 320 {
		t.Fatalf("scan combinations = %d, want 320", len(grid.combinations()))
	}
	if !containsScanFloat(grid.TakeProfits, 2) || !containsScanFloat(grid.TakeProfits, 3) {
		t.Fatalf("take profits = %v, want short-swing 2%% and 3%% targets", grid.TakeProfits)
	}
	if !containsScanInt(grid.MaxHoldDays, 5) || !containsScanInt(grid.MaxHoldDays, 15) {
		t.Fatalf("max hold days = %v, want 5-day and 15-day candidates", grid.MaxHoldDays)
	}
}

func TestHistoricalScanScopesBuildEveryNonEmptyPoolCombination(t *testing.T) {
	scopes, err := buildHistoricalScanScopes([]StockPool{
		{ID: "a", Name: "A", Symbols: []string{"SZ000001", "000002"}},
		{ID: "b", Name: "B", Symbols: []string{"000001.SZ", "600000"}},
		{ID: "c", Name: "C", Symbols: []string{"300001"}},
		{ID: "d", Name: "D", Symbols: []string{"688001"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 15 {
		t.Fatalf("scope combinations = %d, want 15", len(scopes))
	}
	wantNames := map[string]bool{
		"A": true, "B": true, "C": true, "D": true,
		"A + B": true, "A + C": true, "A + D": true, "B + C": true, "B + D": true, "C + D": true,
		"A + B + C": true, "A + B + D": true, "A + C + D": true, "B + C + D": true,
		"A + B + C + D": true,
	}
	for _, scope := range scopes {
		delete(wantNames, scope.Name)
		if scope.Name == "A + B" {
			wantSymbols := []string{"000001", "000002", "600000"}
			if len(scope.Symbols) != len(wantSymbols) {
				t.Fatalf("A + B symbols = %v, want deduplicated %v", scope.Symbols, wantSymbols)
			}
			for index := range wantSymbols {
				if scope.Symbols[index] != wantSymbols[index] {
					t.Fatalf("A + B symbols = %v, want deduplicated %v", scope.Symbols, wantSymbols)
				}
			}
		}
	}
	if len(wantNames) != 0 {
		t.Fatalf("missing scope combinations: %v", wantNames)
	}
	if got := len(scopes) * len(defaultScanGrid().combinations()); got != 4800 {
		t.Fatalf("four-pool scan combinations = %d, want 4800 per strategy", got)
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
