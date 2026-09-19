package main

import (
	"fmt"
	"math"
)

// evaluateLowThreeBullFirstBoard adapts the JoinQuant "低位3连阳首板" setup
// to the local point-in-time daily K-line model. Every input row is visible on
// the signal day; the next-session opening-gap rule is handled by the portfolio
// simulator instead of looking ahead here.
func evaluateLowThreeBullFirstBoard(symbol string, rows []FormulaKline, rule StrategyFactorRule) (bool, float64, any, string) {
	if !strategyMainBoardASymbol(symbol) {
		return false, 0, nil, "仅支持沪深主板A股"
	}
	lookback := intParam(rule.Params, "position_lookback", 60)
	maxPosition := floatParam(rule.Params, "max_pre_position", 85)
	minAmount := floatParam(rule.Params, "min_amount", 5e8)
	maxAmount := floatParam(rule.Params, "max_amount", 4e9)
	minVolumeRatio := floatParam(rule.Params, "min_volume_ratio", 2)
	maxPriorVolumeRatio := floatParam(rule.Params, "max_prior_volume_ratio", 2)
	maxPriorGain := floatParam(rule.Params, "max_prior_gain", 5)
	maxFiveDayRange := floatParam(rule.Params, "max_five_day_range", 20)
	limitTolerance := floatParam(rule.Params, "limit_tolerance", 0.25)
	if lookback < 20 || maxPosition <= 0 || maxPosition > 100 || minVolumeRatio <= 0 || maxPriorVolumeRatio <= 0 {
		return false, 0, nil, "低位首板参数无效"
	}
	if len(rows) < lookback+1 || len(rows) < 31 {
		return false, 0, nil, fmt.Sprintf("低位首板数据不足，需要至少%d根K线", maxInt(lookback+1, 31))
	}

	n := len(rows)
	current, previous := rows[n-1], rows[n-2]
	previous2, previous3 := rows[n-3], rows[n-4]
	if !strategyLimitUpClose(symbol, current, limitTolerance) {
		return false, 0, nil, "信号日不是收盘涨停"
	}
	if strategyLimitUpClose(symbol, previous, limitTolerance) {
		return false, 0, nil, "前一交易日已涨停，不是首板"
	}
	if !(previous.Close > previous.Open && previous2.Close > previous2.Open) {
		return false, 0, nil, "首板前两日不是连续阳线"
	}
	if previous2.Close <= 0 || previous3.Close <= 0 {
		return false, 0, nil, "前序收盘价无效"
	}
	previousGain := (previous.Close - previous2.Close) * 100 / previous2.Close
	previous2Gain := (previous2.Close - previous3.Close) * 100 / previous3.Close
	if previousGain >= maxPriorGain || previous2Gain >= maxPriorGain {
		return false, 0, nil, fmt.Sprintf("首板前涨幅过快 %.2f%%/%.2f%%", previousGain, previous2Gain)
	}
	if current.Amount < minAmount || current.Amount > maxAmount {
		return false, 0, nil, fmt.Sprintf("首板成交额 %.0f 不在 %.0f-%.0f", current.Amount, minAmount, maxAmount)
	}
	if previous.Vol <= 0 || current.Vol < previous.Vol*minVolumeRatio {
		return false, 0, nil, fmt.Sprintf("首板放量不足 %.2fx", minVolumeRatio)
	}
	if previous2.Vol <= 0 || previous.Vol > previous2.Vol*maxPriorVolumeRatio {
		return false, 0, nil, fmt.Sprintf("首板前一日已异常放量 > %.2fx", maxPriorVolumeRatio)
	}

	fiveDay := rows[n-5:]
	high5, low5 := 0.0, math.MaxFloat64
	for _, row := range fiveDay {
		if row.High > high5 {
			high5 = row.High
		}
		if row.Low > 0 && row.Low < low5 {
			low5 = row.Low
		}
	}
	if low5 == math.MaxFloat64 || low5 <= 0 {
		return false, 0, nil, "近5日价格无效"
	}
	fiveDayRange := (high5 - low5) * 100 / low5
	if fiveDayRange > maxFiveDayRange {
		return false, 0, nil, fmt.Sprintf("近5日区间振幅 %.2f%% > %.2f%%", fiveDayRange, maxFiveDayRange)
	}

	priorFive := rows[n-6 : n-1]
	volumeSum, minVolume := 0.0, math.MaxFloat64
	for _, row := range priorFive {
		volumeSum += row.Vol
		if row.Vol > 0 && row.Vol < minVolume {
			minVolume = row.Vol
		}
	}
	priorHigh30 := 0.0
	for _, row := range rows[n-31 : n-1] {
		if row.High > priorHigh30 {
			priorHigh30 = row.High
		}
	}
	averageVolume := volumeSum / float64(len(priorFive))
	explosive := averageVolume > 0 && current.Vol > averageVolume*8
	if minVolume != math.MaxFloat64 && minVolume > 0 && current.Vol > minVolume*12 {
		explosive = true
	}
	if explosive && current.Close > priorHigh30 {
		return false, 0, nil, "爆量且创30日新高，排除情绪末端"
	}

	positionRows := rows[n-1-lookback : n-1]
	positionHigh, positionLow := 0.0, math.MaxFloat64
	for _, row := range positionRows {
		if row.High > positionHigh {
			positionHigh = row.High
		}
		if row.Low > 0 && row.Low < positionLow {
			positionLow = row.Low
		}
	}
	if positionHigh <= positionLow || positionLow == math.MaxFloat64 {
		return false, 0, nil, "低位区间数据无效"
	}
	prePosition := (previous.Close - positionLow) * 100 / (positionHigh - positionLow)
	if prePosition > maxPosition {
		return false, 0, nil, fmt.Sprintf("首板前处于%d日区间 %.2f%% > %.2f%%", lookback, prePosition, maxPosition)
	}

	volumeRatio := current.Vol / previous.Vol
	positionQuality := clamp01((maxPosition - prePosition) / math.Max(maxPosition, 1))
	volumeQuality := clamp01((volumeRatio - minVolumeRatio) / math.Max(4-minVolumeRatio, 0.5))
	rangeQuality := clamp01(1 - fiveDayRange/math.Max(maxFiveDayRange, 0.1))
	strength := clamp01(0.45*positionQuality + 0.35*volumeQuality + 0.20*rangeQuality)
	value := map[string]float64{
		"pre_position": prePosition, "volume_ratio": volumeRatio,
		"five_day_range": fiveDayRange, "amount": current.Amount,
	}
	return true, strength, value, fmt.Sprintf("低位三连阳首板：位置%.1f%%，量比%.2f，5日振幅%.1f%%", prePosition, volumeRatio, fiveDayRange)
}

func strategyMainBoardASymbol(symbol string) bool {
	symbol = strategyNormalizeSymbol(symbol)
	if len(symbol) != 6 {
		return false
	}
	for _, prefix := range []string{"600", "601", "603", "605", "000", "001", "002", "003"} {
		if len(symbol) >= len(prefix) && symbol[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func strategyLimitUpClose(symbol string, row FormulaKline, tolerancePct float64) bool {
	if row.YClose <= 0 || row.Close <= 0 {
		return false
	}
	expected := row.YClose * (1 + portfolioPriceLimitRate(symbol))
	return row.Close >= expected*(1-math.Max(tolerancePct, 0)/100)
}
