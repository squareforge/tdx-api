package main

import (
	"fmt"
	"math"
)

// These are end-of-day signals. The runner's backtest buys at a later session;
// no condition here inspects that session's open, high, low, or close.
func evaluateShortTermPattern(rows []FormulaKline, rule StrategyFactorRule) (recognized, hit bool, strength float64, value any, reason string) {
	switch rule.Factor {
	case "nr7_close_breakout":
		if len(rows) < 21 {
			return true, false, 0, nil, "NR7突破需要至少21根日K线"
		}
		prev, current := rows[len(rows)-2], latest(rows)
		prevRange := prev.High - prev.Low
		if !validShortTermBar(prev) || !validShortTermBar(current) || prevRange <= 0 || current.High <= current.Low {
			return true, false, 0, nil, "NR7突破价格或成交量无效"
		}
		for _, row := range rows[len(rows)-8 : len(rows)-2] {
			if !validShortTermBar(row) || prevRange >= row.High-row.Low {
				return true, false, 0, nil, "前一日不是近7日最窄振幅"
			}
		}
		breakout := (current.Close/prev.High - 1) * 100
		avgVolume := strategyAvgVol(rows, 5)
		volumeRatio := current.Vol / math.Max(avgVolume, 1)
		minBreakout := floatParam(rule.Params, "min_breakout", 0.3)
		maxBreakout := floatParam(rule.Params, "max_breakout", 5)
		minVolumeRatio := floatParam(rule.Params, "min_volume_ratio", 1.2)
		hit = avgVolume > 0 && current.Close > ma(rows, 20) && breakout >= minBreakout && breakout <= maxBreakout && volumeRatio >= minVolumeRatio
		strength = clamp01(0.5 + breakout/math.Max(maxBreakout, 0.01)*0.3 + (volumeRatio-minVolumeRatio)*0.2)
		value = map[string]float64{"breakout_pct": breakout, "volume_ratio": volumeRatio, "nr7_high": prev.High}
		return true, hit, strength, value, fmt.Sprintf("前日NR7，收盘突破 %.2f%%，量比 %.2f", breakout, volumeRatio)

	case "spring_reclaim":
		lookback := intParam(rule.Params, "lookback", 20)
		if lookback < 5 || len(rows) < lookback+1 {
			return true, false, 0, nil, "箱体假跌破历史或参数不足"
		}
		previous := rows[len(rows)-1-lookback : len(rows)-1]
		support, ceiling := math.MaxFloat64, 0.0
		for _, row := range previous {
			if !validShortTermBar(row) {
				return true, false, 0, nil, "箱体历史价格无效"
			}
			support = math.Min(support, row.Low)
			ceiling = math.Max(ceiling, row.High)
		}
		current := latest(rows)
		if !validShortTermBar(current) || current.High <= current.Low || support <= 0 {
			return true, false, 0, nil, "假跌破当日价格无效"
		}
		boxRange := (ceiling/support - 1) * 100
		breakDepth := (1 - current.Low/support) * 100
		reclaim := (current.Close/support - 1) * 100
		closePosition := (current.Close - current.Low) / (current.High - current.Low)
		hit = boxRange <= floatParam(rule.Params, "max_box_range", 25) &&
			breakDepth >= floatParam(rule.Params, "min_break_depth", 0.1) &&
			breakDepth <= floatParam(rule.Params, "max_break_depth", 3) &&
			reclaim >= floatParam(rule.Params, "min_reclaim", 0.5) &&
			closePosition >= floatParam(rule.Params, "min_close_position", 0.65)
		strength = clamp01(0.5 + closePosition*0.3 + reclaim*0.1)
		value = map[string]float64{"support": support, "box_range_pct": boxRange, "break_depth_pct": breakDepth, "reclaim_pct": reclaim}
		return true, hit, strength, value, fmt.Sprintf("跌破前%d日支撑 %.2f%% 后收回 %.2f%%，收盘位置 %.0f%%", lookback, breakDepth, reclaim, closePosition*100)

	case "gap_down_reclaim":
		if len(rows) < 21 {
			return true, false, 0, nil, "低开收复需要至少21根日K线"
		}
		prev, current := rows[len(rows)-2], latest(rows)
		if !validShortTermBar(prev) || !validShortTermBar(current) || current.High <= current.Low {
			return true, false, 0, nil, "低开收复价格或成交量无效"
		}
		gap := (current.Open/prev.Close - 1) * 100
		reclaim := (current.Close/prev.Close - 1) * 100
		closePosition := (current.Close - current.Low) / (current.High - current.Low)
		avgVolume := strategyAvgVol(rows, 5)
		volumeRatio := current.Vol / math.Max(avgVolume, 1)
		hit = gap >= floatParam(rule.Params, "min_gap", -4) && gap <= floatParam(rule.Params, "max_gap", -1) &&
			reclaim >= floatParam(rule.Params, "min_reclaim", 0.3) &&
			closePosition >= floatParam(rule.Params, "min_close_position", 0.7) &&
			avgVolume > 0 && volumeRatio >= floatParam(rule.Params, "min_volume_ratio", 1.1) &&
			current.Close > ma(rows, 20)
		strength = clamp01(0.5 + closePosition*0.2 + reclaim*0.1 + (volumeRatio-1)*0.1)
		value = map[string]float64{"open_gap_pct": gap, "reclaim_pct": reclaim, "volume_ratio": volumeRatio}
		return true, hit, strength, value, fmt.Sprintf("低开 %.2f%% 后收盘收复 %.2f%%，量比 %.2f", gap, reclaim, volumeRatio)
	}
	return false, false, 0, nil, ""
}

func validShortTermBar(row FormulaKline) bool {
	return row.Low > 0 && row.Vol > 0 &&
		row.Low <= row.Open && row.Open <= row.High &&
		row.Low <= row.Close && row.Close <= row.High &&
		!math.IsNaN(row.Low) && !math.IsNaN(row.High) && !math.IsNaN(row.Open) && !math.IsNaN(row.Close) && !math.IsNaN(row.Vol) &&
		!math.IsInf(row.Low, 0) && !math.IsInf(row.High, 0) && !math.IsInf(row.Open, 0) && !math.IsInf(row.Close, 0) && !math.IsInf(row.Vol, 0)
}
