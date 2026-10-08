package main

import (
	"fmt"
	"math"
	"time"
)

type macdSnapshot struct {
	DIF  float64
	DEA  float64
	Hist float64
}

type kdjSnapshot struct {
	K float64
	D float64
	J float64
}

type bollSnapshot struct {
	Mid   float64
	Upper float64
	Lower float64
}

// evaluateAdvancedStrategyFactor implements the early-warning and range-market
// factors used by the built-in strategy templates. Strength is normalized to
// [0,1]; the runner applies the configured score weight afterwards.
func evaluateAdvancedStrategyFactor(rows []FormulaKline, rule StrategyFactorRule, filter bool) (recognized, hit bool, strength float64, value any, reason string) {
	if recognized, hit, strength, value, reason := evaluateShortTermPattern(rows, rule); recognized {
		return recognized, hit, strength, value, reason
	}
	switch rule.Factor {
	case "rebalance_day":
		if len(rows) < 2 {
			return true, false, 0, nil, "调仓日判断至少需要2根K线"
		}
		previous, current := rows[len(rows)-2], rows[len(rows)-1]
		previousDate := strategyDateTime(previous.Date)
		currentDate := strategyDateTime(current.Date)
		mode := stringParam(rule.Params, "mode", "week_first")
		switch mode {
		case "month_first":
			hit = previousDate.Year() != currentDate.Year() || previousDate.Month() != currentDate.Month()
			return true, hit, 1, current.Date, fmt.Sprintf("%d为当月首个交易日: %t", current.Date, hit)
		case "week_first":
			previousYear, previousWeek := previousDate.ISOWeek()
			currentYear, currentWeek := currentDate.ISOWeek()
			hit = previousYear != currentYear || previousWeek != currentWeek
			return true, hit, 1, current.Date, fmt.Sprintf("%d为本周首个交易日: %t", current.Date, hit)
		default:
			return true, false, 0, nil, "未知调仓周期: " + mode
		}
	case "macd_approaching":
		fast, slow, signal := intParam(rule.Params, "fast", 12), intParam(rule.Params, "slow", 26), intParam(rule.Params, "signal", 9)
		points := macdSeries(rows, fast, slow, signal)
		if fast <= 0 || fast >= slow || signal <= 0 || len(points) < slow+signal || len(points) < 2 || latest(rows).Close <= 0 {
			return true, false, 0, nil, "MACD接近金叉数据或参数不足"
		}
		prev, current := points[len(points)-2], points[len(points)-1]
		closePrice := latest(rows).Close
		distance := (current.DEA - current.DIF) * 100 / closePrice
		previousDistance := (prev.DEA - prev.DIF) * 100 / closePrice
		convergence := previousDistance - distance
		maxDistance := floatParam(rule.Params, "max_distance", 0.15)
		minConvergence := floatParam(rule.Params, "min_convergence", 0.02)
		requirePositive := boolParam(rule.Params, "require_positive", false)
		hit = distance >= 0 && distance <= maxDistance && convergence >= minConvergence && (!requirePositive || current.DIF > 0)
		strength = clamp01((maxDistance - distance) / math.Max(maxDistance, 0.01))
		value = map[string]float64{"distance": distance, "convergence": convergence, "dif": current.DIF, "dea": current.DEA}
		reason = fmt.Sprintf("MACD距金叉 %.3f%% / 收敛 %.3f%%", distance, convergence)
		return true, hit, strength, value, reason

	case "macd_convergence_speed":
		fast, slow, signal := intParam(rule.Params, "fast", 12), intParam(rule.Params, "slow", 26), intParam(rule.Params, "signal", 9)
		points := macdSeries(rows, fast, slow, signal)
		if len(points) < slow+signal || len(points) < 2 || latest(rows).Close <= 0 {
			return true, false, 0, nil, "MACD收敛速度数据不足"
		}
		prev, current := points[len(points)-2], points[len(points)-1]
		closePrice := latest(rows).Close
		speed := ((prev.DEA - prev.DIF) - (current.DEA - current.DIF)) * 100 / closePrice
		return true, speed > 0, clamp01(speed / 0.15), speed, fmt.Sprintf("MACD收敛速度 %.3f%%", speed)

	case "distance_from_ma":
		period := intParam(rule.Params, "period", 20)
		average := ma(rows, period)
		if period <= 0 || average <= 0 || len(rows) < period {
			return true, false, 0, nil, fmt.Sprintf("MA%d距离数据不足", period)
		}
		distance := (latest(rows).Close - average) * 100 / average
		minValue, maxValue := floatParam(rule.Params, "min", -math.MaxFloat64), floatParam(rule.Params, "max", math.MaxFloat64)
		if filter {
			hit = distance >= minValue && distance <= maxValue
			strength = 1
		} else {
			optimalMin := floatParam(rule.Params, "optimal_min", minValue)
			optimalMax := floatParam(rule.Params, "optimal_max", maxValue)
			hit, strength = true, intervalStrength(distance, optimalMin, optimalMax)
		}
		return true, hit, strength, distance, fmt.Sprintf("收盘距MA%d %.2f%%", period, distance)

	case "volume_change_rate":
		days := intParam(rule.Params, "days", 5)
		average := strategyAvgVol(rows, days)
		if average <= 0 {
			return true, false, 0, nil, fmt.Sprintf("%d日量能变化数据不足", days)
		}
		ratio := latest(rows).Vol / average
		return true, true, clamp01((ratio - 0.8) / 1.2), ratio, fmt.Sprintf("成交量为前%d日均量 %.2f 倍", days, ratio)

	case "kdj_approaching":
		n, kPeriod, dPeriod := intParam(rule.Params, "n", 9), intParam(rule.Params, "k", 3), intParam(rule.Params, "d", 3)
		points := kdjSeries(rows, n, kPeriod, dPeriod)
		riseDays := intParam(rule.Params, "require_consecutive_rise", 2)
		if len(points) < n+dPeriod || len(points) < riseDays+1 {
			return true, false, 0, nil, "KDJ接近金叉数据不足"
		}
		current := points[len(points)-1]
		distance := current.D - current.K
		maxDistance := floatParam(rule.Params, "max_distance", 8)
		minK, maxK := floatParam(rule.Params, "k_range_min", 20), floatParam(rule.Params, "k_range_max", 50)
		rising := true
		for index := len(points) - riseDays; index < len(points); index++ {
			if index <= 0 || points[index].K <= points[index-1].K {
				rising = false
				break
			}
		}
		hit = distance >= 0 && distance <= maxDistance && current.K >= minK && current.K <= maxK && rising
		strength = clamp01((maxDistance - distance) / math.Max(maxDistance, 0.01))
		value = map[string]float64{"k": current.K, "d": current.D, "distance": distance}
		return true, hit, strength, value, fmt.Sprintf("KDJ待金叉 K %.2f / D %.2f / 距离 %.2f", current.K, current.D, distance)

	case "kdj_k_slope":
		n, kPeriod, dPeriod := intParam(rule.Params, "n", 9), intParam(rule.Params, "k", 3), intParam(rule.Params, "d", 3)
		points := kdjSeries(rows, n, kPeriod, dPeriod)
		if len(points) < n+dPeriod || len(points) < 3 {
			return true, false, 0, nil, "KDJ斜率数据不足"
		}
		slope := (points[len(points)-1].K - points[len(points)-3].K) / 2
		return true, slope > 0, clamp01(slope / 8), slope, fmt.Sprintf("K值两日平均斜率 %.2f", slope)

	case "volume_steady_rise", "volume_consecutive_rise":
		days := intParam(rule.Params, "days", 3)
		if days < 2 || len(rows) < days {
			return true, false, 0, nil, fmt.Sprintf("连续%d日量能数据不足", days)
		}
		rising := 0
		start := len(rows) - days
		for index := start + 1; index < len(rows); index++ {
			if rows[index].Vol > rows[index-1].Vol {
				rising++
			}
		}
		strength = float64(rising) / float64(days-1)
		hit = strength >= 1
		if rule.Factor == "volume_steady_rise" && !filter {
			hit = true
		}
		return true, hit, strength, strength, fmt.Sprintf("近%d日量能递增比例 %.0f%%", days, strength*100)

	case "volume_burst_after_shrink":
		lookback := intParam(rule.Params, "lookback", 5)
		if lookback <= 0 || len(rows) < lookback*2+1 {
			return true, false, 0, nil, fmt.Sprintf("缩量后放量需要至少%d根K线", lookback*2+1)
		}
		end := len(rows) - 1
		previousAverage := averageVolume(rows[end-lookback : end])
		earlierAverage := averageVolume(rows[end-lookback*2 : end-lookback])
		if previousAverage <= 0 || earlierAverage <= 0 {
			return true, false, 0, nil, "缩量后放量基准无效"
		}
		shrinkPct := (earlierAverage - previousAverage) * 100 / earlierAverage
		ratio := rows[end].Vol / previousAverage
		shrinkNeed := floatParam(rule.Params, "shrink_threshold", 2)
		burstNeed := floatParam(rule.Params, "burst_ratio", 2)
		activityNeed := floatParam(rule.Params, "min_turnover", 0)
		if activityNeed > burstNeed {
			burstNeed = activityNeed
		}
		hit = shrinkPct >= shrinkNeed && ratio >= burstNeed
		strength = clamp01((ratio-burstNeed)/math.Max(burstNeed, 0.01) + 0.5)
		value = map[string]float64{"shrink_pct": shrinkPct, "volume_ratio": ratio}
		return true, hit, strength, value, fmt.Sprintf("前期缩量 %.2f%% / 当日相对量 %.2f倍", shrinkPct, ratio)

	case "distance_from_high":
		days := intParam(rule.Params, "days", 20)
		high := highestHigh(rows, days)
		if high <= 0 {
			return true, false, 0, nil, fmt.Sprintf("%d日高点距离数据不足", days)
		}
		distance := (latest(rows).Close - high) * 100 / high
		minValue, maxValue := floatParam(rule.Params, "min", -math.MaxFloat64), floatParam(rule.Params, "max", math.MaxFloat64)
		if filter {
			hit, strength = distance >= minValue && distance <= maxValue, 1
		} else {
			optimalMin, optimalMax := floatParam(rule.Params, "optimal_min", minValue), floatParam(rule.Params, "optimal_max", maxValue)
			hit, strength = true, intervalStrength(distance, optimalMin, optimalMax)
		}
		return true, hit, strength, distance, fmt.Sprintf("收盘距前%d日高点 %.2f%%", days, distance)

	case "volume_ratio":
		days := intParam(rule.Params, "days", 5)
		average := strategyAvgVol(rows, days)
		if average <= 0 {
			return true, false, 0, nil, fmt.Sprintf("%d日量比数据不足", days)
		}
		ratio := latest(rows).Vol / average
		if boolParam(rule.Params, "lower_better", false) {
			strength = clamp01(1.5 - ratio)
		} else {
			strength = clamp01((ratio - 0.8) / 1.2)
		}
		return true, true, strength, ratio, fmt.Sprintf("%d日量比 %.2f", days, ratio)

	case "consecutive_gain_days":
		minDays, maxDays := intParam(rule.Params, "min_days", 1), intParam(rule.Params, "max_days", 5)
		count := consecutiveGainDays(rows)
		hit = count >= minDays && count <= maxDays
		return true, hit, intervalStrength(float64(count), float64(minDays), float64(maxDays)), count, fmt.Sprintf("连续上涨%d日", count)

	case "macd_positive":
		fast, slow, signal := intParam(rule.Params, "fast", 12), intParam(rule.Params, "slow", 26), intParam(rule.Params, "signal", 9)
		points := macdSeries(rows, fast, slow, signal)
		if len(points) < slow+signal {
			return true, false, 0, nil, "MACD多头数据不足"
		}
		current := points[len(points)-1]
		hit = current.DIF > current.DEA && current.Hist > 0
		return true, hit, 1, current.Hist, fmt.Sprintf("MACD多头 DIF %.4f / DEA %.4f", current.DIF, current.DEA)

	case "macd_histogram":
		fast, slow, signal := intParam(rule.Params, "fast", 12), intParam(rule.Params, "slow", 26), intParam(rule.Params, "signal", 9)
		points := macdSeries(rows, fast, slow, signal)
		if len(points) < slow+signal || latest(rows).Close <= 0 {
			return true, false, 0, nil, "MACD柱数据不足"
		}
		histPct := points[len(points)-1].Hist * 100 / latest(rows).Close
		return true, histPct > 0, clamp01(histPct / 0.6), histPct, fmt.Sprintf("MACD柱占股价 %.3f%%", histPct)

	case "range_volatility", "narrow_range":
		days := intParam(rule.Params, "days", 20)
		maxRange := floatParam(rule.Params, "max_range", 20)
		rangePct, ok := priceRangePercent(rows, days)
		if !ok {
			return true, false, 0, nil, fmt.Sprintf("%d日区间振幅数据不足", days)
		}
		hit = rangePct <= maxRange
		strength = clamp01(1 - rangePct/math.Max(maxRange, 0.01))
		return true, hit, strength, rangePct, fmt.Sprintf("近%d日价格区间 %.2f%% <= %.2f%%", days, rangePct, maxRange)

	case "rsi_value":
		period := intParam(rule.Params, "period", 6)
		values := rsiSeries(rows, period)
		if len(values) < period+1 {
			return true, false, 0, nil, fmt.Sprintf("RSI%d数据不足", period)
		}
		current := values[len(values)-1]
		if filter {
			minValue, maxValue := floatParam(rule.Params, "min", 0), floatParam(rule.Params, "max", 100)
			hit, strength = current >= minValue && current <= maxValue, 1
		} else {
			optimalMin, optimalMax := floatParam(rule.Params, "optimal_min", 25), floatParam(rule.Params, "optimal_max", 35)
			hit, strength = true, intervalStrength(current, optimalMin, optimalMax)
		}
		return true, hit, strength, current, fmt.Sprintf("RSI%d %.2f", period, current)

	case "decline_deceleration":
		lookback := intParam(rule.Params, "lookback", 3)
		if lookback < 2 || len(rows) < lookback+1 {
			return true, false, 0, nil, "跌势减速数据不足"
		}
		changes := make([]float64, 0, lookback)
		for index := len(rows) - lookback; index < len(rows); index++ {
			changes = append(changes, strategyChangePercent(rows[index]))
		}
		improving := true
		for index := 1; index < len(changes); index++ {
			if changes[index] <= changes[index-1] {
				improving = false
			}
		}
		improvement := changes[len(changes)-1] - changes[0]
		return true, improving && changes[0] < 0, clamp01(improvement / 5), changes, fmt.Sprintf("近%d日涨跌幅逐日改善: %v", lookback, changes)

	case "volume_below_average":
		days := intParam(rule.Params, "days", 5)
		ratioLimit := floatParam(rule.Params, "ratio", 0.8)
		average := strategyAvgVol(rows, days)
		if average <= 0 {
			return true, false, 0, nil, "缩量数据不足"
		}
		ratio := latest(rows).Vol / average
		return true, ratio <= ratioLimit, clamp01(1 - ratio), ratio, fmt.Sprintf("当日量为前%d日均量 %.2f倍 <= %.2f", days, ratio, ratioLimit)

	case "support_not_broken":
		lookback := intParam(rule.Params, "lookback", 10)
		support := lowestLow(rows, lookback)
		tolerance := floatParam(rule.Params, "tolerance", 1)
		if support <= 0 {
			return true, false, 0, nil, "支撑位数据不足"
		}
		low := latest(rows).Low
		hit = low >= support*(1-tolerance/100) && latest(rows).Close >= support
		return true, hit, 1, support, fmt.Sprintf("当日低点 %.2f / 前%d日支撑 %.2f", low, lookback, support)

	case "distance_from_support":
		lookback := intParam(rule.Params, "lookback", 20)
		support := lowestLow(rows, lookback)
		if support <= 0 {
			return true, false, 0, nil, "支撑距离数据不足"
		}
		distance := (latest(rows).Close - support) * 100 / support
		return true, distance >= 0, clamp01(1 - distance/8), distance, fmt.Sprintf("收盘距前%d日支撑 %.2f%%", lookback, distance)

	case "ma_convergence":
		periods := intSliceParam(rule.Params, "ma_periods", []int{5, 10, 20})
		maxDivergence := floatParam(rule.Params, "max_divergence", 3)
		minDuration := intParam(rule.Params, "min_duration", 5)
		maxDuration := intParam(rule.Params, "max_duration", 15)
		duration := maConvergenceDuration(rows, periods, maxDivergence, maxDuration)
		hit = duration >= minDuration
		return true, hit, clamp01(float64(duration) / math.Max(float64(minDuration), 1)), duration, fmt.Sprintf("均线粘合持续%d日", duration)

	case "price_above_ma_cluster":
		periods := intSliceParam(rule.Params, "ma_periods", []int{5, 10, 20})
		minimum := floatParam(rule.Params, "min_distance", 1)
		_, maximum, ok := maCluster(rows, periods)
		if !ok || maximum <= 0 {
			return true, false, 0, nil, "均线簇突破数据不足"
		}
		distance := (latest(rows).Close - maximum) * 100 / maximum
		return true, distance >= minimum, clamp01(distance / 3), distance, fmt.Sprintf("收盘高于均线簇 %.2f%%", distance)

	case "boll_width_expanding":
		period, width := intParam(rule.Params, "period", 20), floatParam(rule.Params, "width", 2)
		current, okCurrent := latestBOLL(rows, period, width)
		previous, okPrevious := latestBOLL(rows[:maxInt(len(rows)-1, 0)], period, width)
		if !okCurrent || !okPrevious || previous.Mid <= 0 || current.Mid <= 0 {
			return true, false, 0, nil, "BOLL带宽数据不足"
		}
		currentWidth := (current.Upper - current.Lower) * 100 / current.Mid
		previousWidth := (previous.Upper - previous.Lower) * 100 / previous.Mid
		expansion := currentWidth - previousWidth
		return true, expansion > 0, clamp01(expansion / 2), expansion, fmt.Sprintf("BOLL带宽扩张 %.2f个百分点", expansion)

	case "ma_convergence_days":
		periods := intSliceParam(rule.Params, "ma_periods", []int{5, 10, 20})
		maxDays := intParam(rule.Params, "max", 25)
		duration := maConvergenceDuration(rows, periods, floatParam(rule.Params, "max_divergence", 3), maxDays)
		optimalMin, optimalMax := floatParam(rule.Params, "optimal_min", 10), floatParam(rule.Params, "optimal_max", 12)
		return true, duration > 0, intervalStrength(float64(duration), optimalMin, optimalMax), duration, fmt.Sprintf("均线粘合持续%d日", duration)

	case "breakout_strength":
		volumeWeight, priceWeight := floatParam(rule.Params, "volume_weight", 0.6), floatParam(rule.Params, "price_weight", 0.4)
		average := strategyAvgVol(rows, 10)
		if average <= 0 || len(rows) < 2 {
			return true, false, 0, nil, "突破强度数据不足"
		}
		volumeRatio := latest(rows).Vol / average
		priceChange := strategyChangePercent(latest(rows))
		weightTotal := math.Max(volumeWeight+priceWeight, 0.01)
		strength = (clamp01((volumeRatio-1)/2)*volumeWeight + clamp01(priceChange/6)*priceWeight) / weightTotal
		value = map[string]float64{"volume_ratio": volumeRatio, "price_change": priceChange}
		return true, volumeRatio > 1 && priceChange > 0, strength, value, fmt.Sprintf("突破量比 %.2f / 涨幅 %.2f%%", volumeRatio, priceChange)

	case "ma_distance":
		periods := intSliceParam(rule.Params, "ma_periods", []int{5, 10, 20})
		minimum, maximum, ok := maCluster(rows, periods)
		if !ok || minimum <= 0 {
			return true, false, 0, nil, "均线距离数据不足"
		}
		spread := (maximum - minimum) * 100 / minimum
		return true, true, clamp01(1 - spread/5), spread, fmt.Sprintf("均线簇离散度 %.2f%%", spread)

	case "average_turnover":
		days := intParam(rule.Params, "days", 10)
		maxActivity := floatParam(rule.Params, "max", 2)
		activity, ok := averageRelativeVolume(rows, days, 20)
		if !ok {
			return true, false, 0, nil, "平均相对活跃度数据不足"
		}
		return true, activity <= maxActivity, clamp01(1 - activity/math.Max(maxActivity, 0.01)), activity, fmt.Sprintf("近%d日平均相对量 %.2f倍 <= %.2f倍", days, activity, maxActivity)

	case "turnover_surge":
		minActivity := math.Max(floatParam(rule.Params, "min_turnover", 3), floatParam(rule.Params, "min_ratio", 1.8))
		average := strategyAvgVol(rows, 10)
		if average <= 0 {
			return true, false, 0, nil, "活跃度突增数据不足"
		}
		activity := latest(rows).Vol / average
		return true, activity >= minActivity, clamp01(activity/minActivity - 0.5), activity, fmt.Sprintf("当日相对量 %.2f倍 >= %.2f倍", activity, minActivity)

	case "price_break_high":
		days := intParam(rule.Params, "days", 20)
		high := highestHigh(rows, days)
		if high <= 0 {
			return true, false, 0, nil, "平台突破数据不足"
		}
		distance := (latest(rows).Close - high) * 100 / high
		return true, distance >= 0, clamp01(distance / 3), distance, fmt.Sprintf("收盘突破前%d日高点 %.2f%%", days, distance)

	case "consolidation_days":
		maxDays := intParam(rule.Params, "max", 25)
		duration := trailingNarrowRangeDays(rows, maxDays, floatParam(rule.Params, "max_range", 12))
		optimalMin, optimalMax := floatParam(rule.Params, "optimal_min", 18), floatParam(rule.Params, "optimal_max", 22)
		return true, duration > 0, intervalStrength(float64(duration), optimalMin, optimalMax), duration, fmt.Sprintf("窄幅整理持续%d日", duration)
	}
	return false, false, 0, nil, ""
}

func strategyDateTime(date int) time.Time {
	return time.Date(date/10000, time.Month((date/100)%100), date%100, 0, 0, 0, 0, time.Local)
}

func boolParam(params map[string]interface{}, key string, fallback bool) bool {
	if value, ok := params[key].(bool); ok {
		return value
	}
	return fallback
}

func intSliceParam(params map[string]interface{}, key string, fallback []int) []int {
	raw, ok := params[key].([]interface{})
	if !ok {
		return fallback
	}
	result := make([]int, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(float64); ok && value > 0 {
			result = append(result, int(value))
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func intervalStrength(value, minimum, maximum float64) float64 {
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	if value >= minimum && value <= maximum {
		return 1
	}
	width := math.Max(maximum-minimum, math.Max(math.Abs(minimum), 1)*0.5)
	if value < minimum {
		return clamp01(1 - (minimum-value)/width)
	}
	return clamp01(1 - (value-maximum)/width)
}

func averageVolume(rows []FormulaKline) float64 {
	if len(rows) == 0 {
		return 0
	}
	total := 0.0
	for _, row := range rows {
		total += row.Vol
	}
	return total / float64(len(rows))
}

func lowestLow(rows []FormulaKline, days int) float64 {
	if days <= 0 || len(rows) < days+1 {
		return 0
	}
	end := len(rows) - 1
	low := rows[end-days].Low
	for _, row := range rows[end-days : end] {
		if row.Low > 0 && (low <= 0 || row.Low < low) {
			low = row.Low
		}
	}
	return low
}

func consecutiveGainDays(rows []FormulaKline) int {
	count := 0
	for index := len(rows) - 1; index > 0; index-- {
		if rows[index].Close <= rows[index-1].Close {
			break
		}
		count++
	}
	return count
}

func priceRangePercent(rows []FormulaKline, days int) (float64, bool) {
	if days <= 1 || len(rows) < days {
		return 0, false
	}
	window := rows[len(rows)-days:]
	high, low := window[0].High, window[0].Low
	for _, row := range window[1:] {
		if row.High > high {
			high = row.High
		}
		if row.Low > 0 && (low <= 0 || row.Low < low) {
			low = row.Low
		}
	}
	if low <= 0 {
		return 0, false
	}
	return (high - low) * 100 / low, true
}

func maCluster(rows []FormulaKline, periods []int) (float64, float64, bool) {
	if len(periods) == 0 {
		return 0, 0, false
	}
	minimum, maximum := math.MaxFloat64, 0.0
	for _, period := range periods {
		value := ma(rows, period)
		if value <= 0 {
			return 0, 0, false
		}
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	return minimum, maximum, true
}

func maConvergenceDuration(rows []FormulaKline, periods []int, maxDivergence float64, maxDays int) int {
	if maxDays <= 0 {
		maxDays = len(rows)
	}
	duration := 0
	for offset := 0; offset < maxDays && len(rows)-offset > 0; offset++ {
		minimum, maximum, ok := maCluster(rows[:len(rows)-offset], periods)
		if !ok || minimum <= 0 || (maximum-minimum)*100/minimum > maxDivergence {
			break
		}
		duration++
	}
	return duration
}

func averageRelativeVolume(rows []FormulaKline, days, baselineDays int) (float64, bool) {
	if days <= 0 || baselineDays <= 0 || len(rows) < days+baselineDays {
		return 0, false
	}
	start := len(rows) - days
	total := 0.0
	for index := start; index < len(rows); index++ {
		baseline := averageVolume(rows[index-baselineDays : index])
		if baseline <= 0 {
			return 0, false
		}
		total += rows[index].Vol / baseline
	}
	return total / float64(days), true
}

func trailingNarrowRangeDays(rows []FormulaKline, maxDays int, maxRange float64) int {
	if maxDays <= 1 {
		return 0
	}
	if maxDays > len(rows) {
		maxDays = len(rows)
	}
	duration := 0
	for days := 2; days <= maxDays; days++ {
		rangePct, ok := priceRangePercent(rows, days)
		if !ok || rangePct > maxRange {
			break
		}
		duration = days
	}
	return duration
}

func evaluateMASlope(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	period := intParam(rule.Params, "period", 20)
	lookback := intParam(rule.Params, "lookback", 5)
	minSlope := floatParam(rule.Params, "min", 0)
	maxSlope := floatParam(rule.Params, "max", 20)
	if period <= 0 || lookback <= 0 || maxSlope < minSlope {
		return false, 0, "均线斜率参数无效"
	}
	if len(rows) < period+lookback {
		return false, 0, fmt.Sprintf("均线斜率数据不足，需要至少%d根K线", period+lookback)
	}
	current := ma(rows, period)
	previous := ma(rows[:len(rows)-lookback], period)
	if previous <= 0 {
		return false, 0, "均线斜率基准无效"
	}
	slope := (current - previous) * 100 / previous
	hit := slope >= minSlope && slope <= maxSlope
	return hit, slope, fmt.Sprintf("MA%d近%d日斜率 %.2f%% 在 %.2f-%.2f%%", period, lookback, slope, minSlope, maxSlope)
}

func evaluateCloseStrength(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	minPosition := floatParam(rule.Params, "min_position", 0.65)
	maxUpperShadow := floatParam(rule.Params, "max_upper_shadow", 3)
	if len(rows) == 0 || minPosition < 0 || minPosition > 1 || maxUpperShadow < 0 {
		return false, 0, "收盘质量参数或数据无效"
	}
	row := latest(rows)
	rangeValue := row.High - row.Low
	if rangeValue <= 0 {
		return false, 0, "最新K线振幅无效"
	}
	position := (row.Close - row.Low) / rangeValue
	upperBase := math.Max(row.Open, row.Close)
	basePrice := row.YClose
	if basePrice <= 0 {
		basePrice = row.Close
	}
	upperShadow := 0.0
	if basePrice > 0 && row.High > upperBase {
		upperShadow = (row.High - upperBase) * 100 / basePrice
	}
	hit := position >= minPosition && upperShadow <= maxUpperShadow
	return hit, position, fmt.Sprintf("收盘位置 %.0f%% >= %.0f%% / 上影 %.2f%% <= %.2f%%", position*100, minPosition*100, upperShadow, maxUpperShadow)
}

func evaluateRSIRebound(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	period := intParam(rule.Params, "period", 6)
	lookback := intParam(rule.Params, "lookback", 5)
	oversold := floatParam(rule.Params, "oversold", 30)
	minCurrent := floatParam(rule.Params, "min_current", 35)
	maxCurrent := floatParam(rule.Params, "max_current", 55)
	if period <= 0 || lookback <= 0 || minCurrent > maxCurrent {
		return false, 0, "RSI反转参数无效"
	}
	values := rsiSeries(rows, period)
	if len(values) < period+lookback+1 {
		return false, 0, fmt.Sprintf("RSI反转数据不足，需要至少%d根K线", period+lookback+1)
	}
	current, previous := values[len(values)-1], values[len(values)-2]
	start := len(values) - lookback - 1
	minimum := values[start]
	for _, value := range values[start : len(values)-1] {
		if value < minimum {
			minimum = value
		}
	}
	hit := minimum <= oversold && current >= minCurrent && current <= maxCurrent && current > previous
	return hit, current, fmt.Sprintf("RSI%d近期最低 %.2f / 当前 %.2f / 前值 %.2f", period, minimum, current, previous)
}

func evaluateMACDSignal(rows []FormulaKline, rule StrategyFactorRule, golden bool) (bool, float64, string) {
	fast := intParam(rule.Params, "fast", 12)
	slow := intParam(rule.Params, "slow", 26)
	signal := intParam(rule.Params, "signal", 9)
	if fast <= 0 || slow <= 0 || signal <= 0 || fast >= slow {
		return false, 0, "MACD参数无效"
	}
	points := macdSeries(rows, fast, slow, signal)
	if len(points) < slow+signal || len(points) < 2 {
		return false, 0, fmt.Sprintf("MACD数据不足，需要至少%d根K线", slow+signal)
	}
	prev, curr := points[len(points)-2], points[len(points)-1]
	if golden {
		hit := prev.DIF <= prev.DEA && curr.DIF > curr.DEA
		return hit, curr.DIF - curr.DEA, fmt.Sprintf("MACD金叉 DIF %.4f / DEA %.4f / 柱 %.4f", curr.DIF, curr.DEA, curr.Hist)
	}
	hit := prev.DIF >= prev.DEA && curr.DIF < curr.DEA
	return hit, curr.DEA - curr.DIF, fmt.Sprintf("MACD死叉 DIF %.4f / DEA %.4f / 柱 %.4f", curr.DIF, curr.DEA, curr.Hist)
}

func evaluateKDJGoldenCross(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	n := intParam(rule.Params, "n", 9)
	kPeriod := intParam(rule.Params, "k", 3)
	dPeriod := intParam(rule.Params, "d", 3)
	if n <= 0 || kPeriod <= 0 || dPeriod <= 0 {
		return false, 0, "KDJ参数无效"
	}
	points := kdjSeries(rows, n, kPeriod, dPeriod)
	if len(points) < n+dPeriod || len(points) < 2 {
		return false, 0, fmt.Sprintf("KDJ数据不足，需要至少%d根K线", n+dPeriod)
	}
	prev, curr := points[len(points)-2], points[len(points)-1]
	hit := prev.K <= prev.D && curr.K > curr.D
	return hit, curr.J, fmt.Sprintf("KDJ金叉 K %.2f / D %.2f / J %.2f", curr.K, curr.D, curr.J)
}

func evaluateRSIOversold(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	period := intParam(rule.Params, "period", 6)
	threshold := floatParam(rule.Params, "threshold", 30)
	values := rsiSeries(rows, period)
	if period <= 0 {
		return false, 0, "RSI参数无效"
	}
	if len(values) < period+1 {
		return false, 0, fmt.Sprintf("RSI数据不足，需要至少%d根K线", period+1)
	}
	rsi := values[len(values)-1]
	hit := rsi > 0 && rsi <= threshold
	return hit, rsi, fmt.Sprintf("RSI%d %.2f <= %.2f", period, rsi, threshold)
}

func evaluateBOLLBreakout(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	period := intParam(rule.Params, "period", 20)
	width := floatParam(rule.Params, "width", 2)
	boll, ok := latestBOLL(rows, period, width)
	if period <= 1 || width <= 0 {
		return false, 0, "BOLL参数无效"
	}
	if !ok {
		return false, 0, fmt.Sprintf("BOLL数据不足，需要至少%d根K线", period)
	}
	closePrice := latest(rows).Close
	hit := closePrice > boll.Upper
	return hit, closePrice - boll.Upper, fmt.Sprintf("BOLL突破 C %.2f / 上轨 %.2f / 中轨 %.2f", closePrice, boll.Upper, boll.Mid)
}

func evaluateVolumeBreakout(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	days := intParam(rule.Params, "days", 20)
	ratio := floatParam(rule.Params, "ratio", 1.5)
	minChange := floatParam(rule.Params, "min_change", 2)
	if days <= 0 || ratio <= 0 {
		return false, 0, "放量突破参数无效"
	}
	if len(rows) < days+1 {
		return false, 0, fmt.Sprintf("放量突破数据不足，需要至少%d根K线", days+1)
	}
	row := latest(rows)
	avgVol := strategyAvgVol(rows, days)
	high := highestHigh(rows, days)
	change := strategyChangePercent(row)
	volRatio := 0.0
	if avgVol > 0 {
		volRatio = row.Vol / avgVol
	}
	hit := avgVol > 0 && high > 0 && row.Close >= high && volRatio >= ratio && change >= minChange
	return hit, volRatio, fmt.Sprintf("放量突破 C %.2f / %d日高点 %.2f / 量比 %.2f / 涨幅 %.2f%%", row.Close, days, high, volRatio, change)
}

func evaluateLocalRocket(rows []FormulaKline, rule StrategyFactorRule) (bool, float64, string) {
	lookback := intParam(rule.Params, "lookback", 20)
	volumeDays := intParam(rule.Params, "volume_days", 5)
	volumeRatioNeed := floatParam(rule.Params, "volume_ratio", 1.8)
	minChange := floatParam(rule.Params, "min_change", 3)
	shortMA := intParam(rule.Params, "short_ma", 5)
	midMA := intParam(rule.Params, "mid_ma", 10)
	if lookback <= 0 || volumeDays <= 0 || volumeRatioNeed <= 0 || shortMA <= 0 || midMA <= 0 {
		return false, 0, "本地火箭发射参数无效"
	}
	required := maxInt(lookback, maxInt(volumeDays+1, midMA))
	if len(rows) < required {
		return false, 0, fmt.Sprintf("本地火箭发射数据不足，需要至少%d根K线", required)
	}
	row := latest(rows)
	avgVol := strategyAvgVol(rows, volumeDays)
	volRatio := 0.0
	if avgVol > 0 {
		volRatio = row.Vol / avgVol
	}
	high := highestHigh(rows, lookback)
	change := strategyChangePercent(row)
	maShort := ma(rows, shortMA)
	maMid := ma(rows, midMA)
	hit := change >= minChange && volRatio >= volumeRatioNeed && high > 0 && row.Close >= high && maShort >= maMid && row.Close >= maShort
	scoreValue := change + volRatio
	return hit, scoreValue, fmt.Sprintf("本地火箭发射 涨幅 %.2f%% / 量比 %.2f / 突破%d日高点 %.2f / MA%d %.2f / MA%d %.2f", change, volRatio, lookback, high, shortMA, maShort, midMA, maMid)
}

func macdSeries(rows []FormulaKline, fast, slow, signal int) []macdSnapshot {
	closes := closeValues(rows)
	fastEMA := emaValues(closes, fast)
	slowEMA := emaValues(closes, slow)
	dif := make([]float64, len(closes))
	for i := range closes {
		dif[i] = fastEMA[i] - slowEMA[i]
	}
	dea := emaValues(dif, signal)
	points := make([]macdSnapshot, len(closes))
	for i := range closes {
		points[i] = macdSnapshot{DIF: dif[i], DEA: dea[i], Hist: (dif[i] - dea[i]) * 2}
	}
	return points
}

func kdjSeries(rows []FormulaKline, n, kPeriod, dPeriod int) []kdjSnapshot {
	if len(rows) == 0 {
		return nil
	}
	rsv := make([]float64, len(rows))
	for i := range rows {
		start := i - n + 1
		if start < 0 {
			start = 0
		}
		low, high := rows[start].Low, rows[start].High
		for _, row := range rows[start : i+1] {
			if row.Low < low {
				low = row.Low
			}
			if row.High > high {
				high = row.High
			}
		}
		if high == low {
			rsv[i] = 50
		} else {
			rsv[i] = (rows[i].Close - low) * 100 / (high - low)
		}
	}
	k := tdxSMAValues(rsv, kPeriod, 1, 50)
	d := tdxSMAValues(k, dPeriod, 1, 50)
	points := make([]kdjSnapshot, len(rows))
	for i := range rows {
		points[i] = kdjSnapshot{K: k[i], D: d[i], J: 3*k[i] - 2*d[i]}
	}
	return points
}

func rsiSeries(rows []FormulaKline, period int) []float64 {
	if len(rows) == 0 {
		return nil
	}
	up := make([]float64, len(rows))
	down := make([]float64, len(rows))
	for i := 1; i < len(rows); i++ {
		change := rows[i].Close - rows[i-1].Close
		if change > 0 {
			up[i] = change
		} else {
			down[i] = -change
		}
	}
	avgUp := tdxSMAValues(up, period, 1, 0)
	avgDown := tdxSMAValues(down, period, 1, 0)
	values := make([]float64, len(rows))
	for i := range rows {
		total := avgUp[i] + avgDown[i]
		if total > 0 {
			values[i] = avgUp[i] * 100 / total
		}
	}
	return values
}

func latestBOLL(rows []FormulaKline, period int, width float64) (bollSnapshot, bool) {
	if period <= 1 || len(rows) < period {
		return bollSnapshot{}, false
	}
	start := len(rows) - period
	sum := 0.0
	for _, row := range rows[start:] {
		sum += row.Close
	}
	mid := sum / float64(period)
	variance := 0.0
	for _, row := range rows[start:] {
		diff := row.Close - mid
		variance += diff * diff
	}
	std := math.Sqrt(variance / float64(period))
	return bollSnapshot{Mid: mid, Upper: mid + width*std, Lower: mid - width*std}, true
}

func emaValues(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 || period <= 0 {
		return out
	}
	alpha := 2.0 / float64(period+1)
	prev := values[0]
	for i, value := range values {
		prev = alpha*value + (1-alpha)*prev
		out[i] = prev
	}
	return out
}

func tdxSMAValues(values []float64, n int, m float64, initial float64) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 || n <= 0 {
		return out
	}
	prev := initial
	for i, value := range values {
		prev = (m*value + (float64(n)-m)*prev) / float64(n)
		out[i] = prev
	}
	return out
}

func closeValues(rows []FormulaKline) []float64 {
	values := make([]float64, len(rows))
	for i, row := range rows {
		values[i] = row.Close
	}
	return values
}

func strategyChangePercent(row FormulaKline) float64 {
	if row.YClose <= 0 {
		return 0
	}
	return (row.Close - row.YClose) * 100 / row.YClose
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
