package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestHistoryFinanceFloatUsesOneBasedFieldIDs(t *testing.T) {
	values := make([]byte, historyFinanceROEField*4)
	binary.LittleEndian.PutUint32(values[(historyFinanceProfitGrowthField-1)*4:], math.Float32bits(18.5))
	binary.LittleEndian.PutUint32(values[(historyFinanceTotalSharesField-1)*4:], math.Float32bits(2.5e8))
	if got := historyFinanceFloat(values, historyFinanceProfitGrowthField); math.Abs(got-18.5) > 1e-6 {
		t.Fatalf("profit growth = %v, want 18.5", got)
	}
	if got := historyFinanceFloat(values, historyFinanceTotalSharesField); got != 2.5e8 {
		t.Fatalf("total shares = %v, want 250000000", got)
	}
}

func TestStrategyNormalizeSymbolSupportsExchangePrefixes(t *testing.T) {
	for input, expected := range map[string]string{
		"SZ000001":    "000001",
		"SH600000":    "600000",
		"BJ830001":    "830001",
		"000001.XSHE": "000001",
		"600000.XSHG": "600000",
		"000001.SZ":   "000001",
	} {
		if got := strategyNormalizeSymbol(input); got != expected {
			t.Fatalf("strategyNormalizeSymbol(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestStrategyMarketCodeSupportsCurrentBeijingCodes(t *testing.T) {
	for input, expected := range map[string]string{
		"000001": "SZ000001",
		"600000": "SH600000",
		"920992": "BJ920992",
		"830001": "BJ830001",
	} {
		if got := strategyMarketCode(input); got != expected {
			t.Fatalf("strategyMarketCode(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestFundamentalMetricsNeverUseFutureDisclosureOrDividend(t *testing.T) {
	data := &strategyFundamentalData{
		Financials: map[string][]strategyFinancialRecord{
			"000001": {
				{PeriodDate: 20231231, AvailableDate: 20240320, TotalShares: 1e8, NetProfitTTM: 1e7, ProfitGrowth: 20},
				{PeriodDate: 20240331, AvailableDate: 20240430, TotalShares: 2e8, NetProfitTTM: 4e7, ProfitGrowth: 40},
			},
		},
		Weights: map[string][]strategyWeightRecord{
			"000001": {
				{Date: 20240110, TotalShares: 1e8, CashDividendPer10: 1},
				{Date: 20240510, TotalShares: 2e8, CashDividendPer10: 2},
			},
		},
	}
	metrics, ok := data.metrics("000001", 20240415, 10)
	if !ok {
		t.Fatal("expected metrics before second disclosure")
	}
	if metrics.MarketCapYi != 10 || metrics.ProfitGrowth != 20 || metrics.AvailableAt != 20240320 {
		t.Fatalf("future disclosure leaked into metrics: %+v", metrics)
	}
	if math.Abs(metrics.DividendRate-1) > 1e-9 {
		t.Fatalf("future dividend leaked into yield: %+v", metrics)
	}
	if metrics.PE != 100 || metrics.PEG != 5 {
		t.Fatalf("point-in-time PE/PEG = %+v, want PE 100 PEG 5", metrics)
	}
}

func TestRebalanceDayRecognizesWeekAndMonthBoundaries(t *testing.T) {
	weekRows := []FormulaKline{{Date: 20260918, Close: 10}, {Date: 20260921, Close: 10}}
	weekRule := StrategyFactorRule{Factor: "rebalance_day", Params: map[string]interface{}{"mode": "week_first"}}
	if _, hit, _, _, _ := evaluateAdvancedStrategyFactor(weekRows, weekRule, true); !hit {
		t.Fatal("Monday after Friday should be the first weekly trading day")
	}
	monthRows := []FormulaKline{{Date: 20260930, Close: 10}, {Date: 20261009, Close: 10}}
	monthRule := StrategyFactorRule{Factor: "rebalance_day", Params: map[string]interface{}{"mode": "month_first"}}
	if _, hit, _, _, _ := evaluateAdvancedStrategyFactor(monthRows, monthRule, true); !hit {
		t.Fatal("first available October session should be the monthly rebalance day")
	}
}
