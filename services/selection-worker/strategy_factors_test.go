package main

import (
	"math"
	"testing"
)

func workerFactorRows(closes []float64, highs []float64) []FormulaKline {
	rows := make([]FormulaKline, len(closes))
	previous := 0.0
	for index, closePrice := range closes {
		high := closePrice + 0.2
		if index < len(highs) && highs[index] > 0 {
			high = highs[index]
		}
		rows[index] = FormulaKline{
			Date: 20260101 + index,
			Open: closePrice - 0.1, High: high, Low: closePrice - 0.2,
			Close: closePrice, YClose: previous, Vol: 1000, Amount: 2e8,
		}
		previous = closePrice
	}
	return rows
}

func TestWorkerMarketMomentumUsesSameDayBenchmarkHistory(t *testing.T) {
	stockRows := workerFactorRows(make([]float64, 21), nil)
	benchmarkCloses := make([]float64, 21)
	for index := range benchmarkCloses {
		benchmarkCloses[index] = 100
	}
	benchmarkCloses[20] = 107
	benchmarkRows := workerFactorRows(benchmarkCloses, nil)
	runner := &AutomationRunner{}
	result := &StrategyRunResult{KlineCache: map[string][]FormulaKline{strategyBenchmarkSymbol: benchmarkRows}}
	rule := StrategyFactorRule{ID: "market", Factor: "market_momentum", Params: map[string]interface{}{"days": 20.0, "min": 6.0, "max": 100.0}}
	if got := runner.evaluateFactor(result, "000001", stockRows, rule, true); !got.Hit {
		t.Fatalf("7%% benchmark momentum should pass: %+v", got)
	}
	benchmarkRows[20].Close = 105
	if got := runner.evaluateFactor(result, "000001", stockRows, rule, true); got.Hit {
		t.Fatalf("5%% benchmark momentum should fail: %+v", got)
	}
}

func workerEvalFactor(factor string, weight float64, params map[string]interface{}, rows []FormulaKline, filter bool) StrategyFactorResult {
	runner := &AutomationRunner{}
	result := &StrategyRunResult{
		PoolCache: map[string]map[string]bool{}, FormulaCache: map[string]map[string]bool{},
		FormulaDetail: map[string]map[string]any{}, KlineCache: map[string][]FormulaKline{},
	}
	return runner.evaluateFactor(result, "000001", rows, StrategyFactorRule{ID: "test", Factor: factor, Weight: weight, Params: params}, filter)
}

func TestWorkerRangeFactorsActAsHardFilters(t *testing.T) {
	closes := make([]float64, 61)
	highs := make([]float64, 61)
	for index := range closes {
		closes[index] = 10
		highs[index] = 10
	}
	closes[60] = 12
	if got := workerEvalFactor("gain_days", 20, map[string]interface{}{"days": 20.0, "min": 5.0, "max": 15.0}, workerFactorRows(closes, highs), true); got.Hit {
		t.Fatalf("20%% gain must fail a 5-15%% hard filter: %+v", got)
	}

	for index := 0; index < 60; index++ {
		highs[index] = 20
	}
	closes[60] = 16
	if got := workerEvalFactor("drawdown_from_high", 20, map[string]interface{}{"days": 60.0, "min": 5.0, "max": 15.0}, workerFactorRows(closes, highs), true); got.Hit {
		t.Fatalf("20%% drawdown must fail a 5-15%% hard filter: %+v", got)
	}
}

func TestWorkerQualityFactors(t *testing.T) {
	closes := make([]float64, 30)
	for index := range closes {
		closes[index] = 10 + float64(index)*0.1
	}
	rows := workerFactorRows(closes, nil)
	if got := workerEvalFactor("ma_slope", 25, map[string]interface{}{"period": 20.0, "lookback": 5.0, "min": 1.0, "max": 10.0}, rows, false); !got.Hit || got.Score <= 0 || got.Score > 25 {
		t.Fatalf("rising MA should receive normalized weighted score: %+v", got)
	}

	rows[len(rows)-1].Open = 12.7
	rows[len(rows)-1].Low = 12.6
	rows[len(rows)-1].High = 13.0
	rows[len(rows)-1].Close = 12.95
	if got := workerEvalFactor("close_strength", 0, map[string]interface{}{"min_position": 0.7, "max_upper_shadow": 1.0}, rows, true); !got.Hit || math.Abs(got.Score) > 1e-9 {
		t.Fatalf("strong close should pass filter without score: %+v", got)
	}
}

func TestWorkerAdvancedStrategyFactorsAreRegistered(t *testing.T) {
	factors := []string{
		"average_turnover", "boll_width_expanding", "breakout_strength", "consecutive_gain_days",
		"consolidation_days", "decline_deceleration", "distance_from_high", "distance_from_ma",
		"distance_from_support", "kdj_approaching", "kdj_k_slope", "ma_convergence",
		"ma_convergence_days", "ma_distance", "macd_approaching", "macd_convergence_speed",
		"macd_histogram", "macd_positive", "narrow_range", "price_above_ma_cluster",
		"price_break_high", "range_volatility", "rsi_value", "support_not_broken", "turnover_surge",
		"volume_below_average", "volume_burst_after_shrink", "volume_change_rate",
		"volume_consecutive_rise", "volume_ratio", "volume_steady_rise",
	}
	rows := workerFactorRows(make([]float64, 100), nil)
	for index := range rows {
		rows[index].Open = 10
		rows[index].High = 10.2
		rows[index].Low = 9.8
		rows[index].Close = 10
		rows[index].YClose = 10
		rows[index].Vol = 1000
	}
	for _, factor := range factors {
		recognized, _, _, _, _ := evaluateAdvancedStrategyFactor(rows, StrategyFactorRule{Factor: factor, Params: map[string]interface{}{}}, false)
		if !recognized {
			t.Errorf("advanced factor %q is not registered", factor)
		}
	}
}

func TestWorkerAdvancedBreakoutAndVolumeFactors(t *testing.T) {
	closes := make([]float64, 21)
	highs := make([]float64, 21)
	for index := range closes {
		closes[index], highs[index] = 10, 10
	}
	closes[20], highs[20] = 10.2, 10.3
	rows := workerFactorRows(closes, highs)
	if got := workerEvalFactor("price_break_high", 0, map[string]interface{}{"days": 20.0}, rows, true); !got.Hit {
		t.Fatalf("price breakout should pass: %+v", got)
	}
	for index := 0; index < 15; index++ {
		rows[index].Vol = 100
	}
	for index := 15; index < 20; index++ {
		rows[index].Vol = 80
	}
	rows[20].Vol = 300
	params := map[string]interface{}{"lookback": 5.0, "shrink_threshold": 2.0, "burst_ratio": 2.0, "min_turnover": 3.0}
	if got := workerEvalFactor("volume_burst_after_shrink", 0, params, rows, true); !got.Hit {
		t.Fatalf("volume burst after contraction should pass: %+v", got)
	}
}

func firstBoardFactorRows() []FormulaKline {
	rows := make([]FormulaKline, 61)
	for index := range rows {
		rows[index] = FormulaKline{
			Date: 20260101 + index, YClose: 10, Open: 9.9, High: 10.2, Low: 9.8,
			Close: 10, Vol: 1000, Amount: 8e8,
		}
	}
	// Two bullish sessions before the first limit-up day.
	rows[57].Open, rows[57].Close, rows[57].YClose, rows[57].Vol = 9.85, 9.95, 9.9, 800
	rows[58].Open, rows[58].Close, rows[58].YClose, rows[58].Vol = 9.9, 10, 9.95, 1000
	rows[59].Open, rows[59].Close, rows[59].YClose, rows[59].Vol = 9.95, 10, 10, 1000
	rows[60] = FormulaKline{
		Date: 20260302, YClose: 10, Open: 10, High: 11, Low: 10,
		Close: 11, Vol: 2500, Amount: 1e9,
	}
	return rows
}

func TestLowThreeBullFirstBoardFactor(t *testing.T) {
	rows := firstBoardFactorRows()
	params := map[string]interface{}{
		"position_lookback": 60.0, "max_pre_position": 85.0,
		"min_amount": 5e8, "max_amount": 4e9, "min_volume_ratio": 2.0,
		"max_prior_volume_ratio": 2.0, "max_prior_gain": 5.0,
		"max_five_day_range": 20.0, "limit_tolerance": 0.25,
	}
	got := workerEvalFactor("low_three_bull_first_board", 100, params, rows, true)
	if !got.Hit {
		t.Fatalf("valid low three-bull first-board pattern should pass: %+v", got)
	}
	scored := workerEvalFactor("low_three_bull_first_board", 100, params, rows, false)
	if !scored.Hit || scored.Score <= 0 || scored.Score > 100 {
		t.Fatalf("valid pattern should receive a bounded score: %+v", scored)
	}
}

func TestLowThreeBullFirstBoardRejectsSecondBoard(t *testing.T) {
	rows := firstBoardFactorRows()
	rows[59].YClose = rows[59].Close / 1.1
	rows[59].High = rows[59].Close
	got := workerEvalFactor("low_three_bull_first_board", 100, map[string]interface{}{
		"position_lookback": 60.0, "min_amount": 5e8, "max_amount": 4e9,
	}, rows, true)
	if got.Hit {
		t.Fatalf("second consecutive limit-up must not be treated as a first board: %+v", got)
	}
}

func TestLowThreeBullFirstBoardRejectsNonMainBoard(t *testing.T) {
	rows := firstBoardFactorRows()
	rule := StrategyFactorRule{Params: map[string]interface{}{"position_lookback": 60.0}}
	for _, symbol := range []string{"300001", "688001", "900901", "200001", "830001"} {
		if hit, _, _, _ := evaluateLowThreeBullFirstBoard(symbol, rows, rule); hit {
			t.Fatalf("non-main-board symbol %s must be rejected", symbol)
		}
	}
}
