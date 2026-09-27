package workbench

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	DefaultTrackingTargetReturn  = 3.0
	DefaultTrackingDrawdownLimit = 5.0
)

// TrackingBar is the small, provider-neutral OHLC shape required to evaluate
// a selection signal. Keeping it in workbench-core lets each market adapter
// reuse the same deterministic calculation.
type TrackingBar struct {
	Date   int
	YClose float64
	Open   float64
	High   float64
	Low    float64
	Close  float64
}

type SelectionHorizon struct {
	HorizonDays          int     `json:"horizon_days"`
	Status               string  `json:"status"` // pending / complete / unavailable
	AsOfDate             int     `json:"as_of_date,omitempty"`
	FinalDate            int     `json:"final_date,omitempty"`
	FinalOpen            float64 `json:"final_open,omitempty"`
	FinalClose           float64 `json:"final_close,omitempty"`
	FinalHigh            float64 `json:"final_high,omitempty"`
	FinalLow             float64 `json:"final_low,omitempty"`
	AverageClose         float64 `json:"average_close,omitempty"`
	AverageCloseReturn   float64 `json:"average_close_return,omitempty"`
	OpenReturn           float64 `json:"open_return,omitempty"`
	CloseReturn          float64 `json:"close_return,omitempty"`
	HighReturn           float64 `json:"high_return,omitempty"`
	LowReturn            float64 `json:"low_return,omitempty"`
	MFE                  float64 `json:"mfe,omitempty"`
	MAE                  float64 `json:"mae,omitempty"`
	MaxPathDrawdown      float64 `json:"max_path_drawdown,omitempty"`
	BenchmarkCloseReturn float64 `json:"benchmark_close_return,omitempty"`
	ExcessCloseReturn    float64 `json:"excess_close_return,omitempty"`
	GrossReturn          float64 `json:"gross_return,omitempty"`
	NetReturn            float64 `json:"net_return,omitempty"`
	TotalCost            float64 `json:"total_cost,omitempty"`
	FirstEvent           string  `json:"first_event,omitempty"`
	CalculationVersion   string  `json:"calculation_version,omitempty"`
	DataQuality          string  `json:"data_quality,omitempty"`
	MaxGain              float64 `json:"max_gain,omitempty"`
	MaxGainDate          int     `json:"max_gain_date,omitempty"`
	MaxGainPrice         float64 `json:"max_gain_price,omitempty"`
	DaysToMaxGain        int     `json:"days_to_max_gain,omitempty"`
	MaxDrawdown          float64 `json:"max_drawdown,omitempty"`
	MaxDrawdownDate      int     `json:"max_drawdown_date,omitempty"`
	MaxDrawdownPrice     float64 `json:"max_drawdown_price,omitempty"`
	DaysToMaxDrawdown    int     `json:"days_to_max_drawdown,omitempty"`
	TargetReturn         float64 `json:"target_return"`
	TargetHit            bool    `json:"target_hit"`
	TargetHitDate        int     `json:"target_hit_date,omitempty"`
	DaysToTargetHit      int     `json:"days_to_target_hit,omitempty"`
	DrawdownLimit        float64 `json:"drawdown_limit"`
	StopLossHit          bool    `json:"stop_loss_hit"`
	StopLossHitDate      int     `json:"stop_loss_hit_date,omitempty"`
	DaysToStopLoss       int     `json:"days_to_stop_loss,omitempty"`
	Success              bool    `json:"success"`
	Reason               string  `json:"reason,omitempty"`
}

type SelectionTracking struct {
	Version       string                      `json:"version"`
	SignalDate    int                         `json:"signal_date"`
	BasePrice     float64                     `json:"base_price"`
	BaseDate      int                         `json:"base_date,omitempty"`
	BasePriceFrom string                      `json:"base_price_from,omitempty"`
	TargetReturn  float64                     `json:"target_return"`
	DrawdownLimit float64                     `json:"drawdown_limit"`
	UpdatedAt     string                      `json:"updated_at"`
	Horizons      map[string]SelectionHorizon `json:"horizons"`
}

type SelectionTrackingItem struct {
	Result   SelectionResult   `json:"result"`
	Tracking SelectionTracking `json:"tracking"`
	Error    string            `json:"error,omitempty"`
}

type SelectionHorizonSummary struct {
	Completed          int     `json:"completed"`
	Pending            int     `json:"pending"`
	Unavailable        int     `json:"unavailable"`
	SuccessCount       int     `json:"success_count"`
	SuccessRate        float64 `json:"success_rate"`
	AverageOpenReturn  float64 `json:"average_open_return"`
	AverageCloseReturn float64 `json:"average_close_return"`
	AverageMaxGain     float64 `json:"average_max_gain"`
	AverageMaxDrawdown float64 `json:"average_max_drawdown"`
}

type SelectionTrackingSummary struct {
	Total    int                                `json:"total"`
	Horizons map[string]SelectionHorizonSummary `json:"horizons"`
}

func DefaultTrackingPolicy(targetReturn, drawdownLimit float64) (float64, float64) {
	if targetReturn <= 0 {
		targetReturn = DefaultTrackingTargetReturn
	}
	if drawdownLimit <= 0 {
		drawdownLimit = DefaultTrackingDrawdownLimit
	}
	return targetReturn, drawdownLimit
}

func SummarizeSelectionTracking(items []SelectionTrackingItem, horizons []int) SelectionTrackingSummary {
	if len(horizons) == 0 {
		horizons = []int{1, 3, 5, 10}
	}
	summary := SelectionTrackingSummary{Total: len(items), Horizons: map[string]SelectionHorizonSummary{}}
	for _, horizon := range horizons {
		key := horizonKey(horizon)
		var openTotal, closeTotal, gainTotal, drawdownTotal float64
		row := SelectionHorizonSummary{}
		for _, item := range items {
			metric, ok := item.Tracking.Horizons[key]
			if !ok {
				row.Pending++
				continue
			}
			switch metric.Status {
			case "complete":
				row.Completed++
				openTotal += metric.OpenReturn
				closeTotal += metric.CloseReturn
				gainTotal += metric.MaxGain
				drawdownTotal += metric.MaxDrawdown
				if metric.Success {
					row.SuccessCount++
				}
			case "unavailable":
				row.Unavailable++
			default:
				row.Pending++
			}
		}
		if row.Completed > 0 {
			count := float64(row.Completed)
			row.SuccessRate = roundTrackingPercent(float64(row.SuccessCount) / count * 100)
			row.AverageOpenReturn = roundTrackingPercent(openTotal / count)
			row.AverageCloseReturn = roundTrackingPercent(closeTotal / count)
			row.AverageMaxGain = roundTrackingPercent(gainTotal / count)
			row.AverageMaxDrawdown = roundTrackingPercent(drawdownTotal / count)
		}
		summary.Horizons[key] = row
	}
	return summary
}

// EvaluateSelectionTracking computes forward returns from valid daily bars.
// A signal created before the A-share open uses that day's open as its entry
// and counts the signal day as D1; later signals start on the next session.
// Values are percentages (3 means +3%), matching the review API convention.
func EvaluateSelectionTracking(item SelectionResult, bars []TrackingBar, horizons []int, targetReturn, drawdownLimit float64, now time.Time) SelectionTracking {
	targetReturn, drawdownLimit = DefaultTrackingPolicy(targetReturn, drawdownLimit)
	if len(horizons) == 0 {
		horizons = []int{1, 3, 5, 10}
	}
	signalTime, hasSignalTime := selectionResultSignalTime(item)
	tracking := SelectionTracking{
		Version:       "2",
		SignalDate:    trackingDateInt(signalTime),
		BasePrice:     0,
		TargetReturn:  targetReturn,
		DrawdownLimit: drawdownLimit,
		UpdatedAt:     now.Format(time.RFC3339),
		Horizons:      map[string]SelectionHorizon{},
	}
	if tracking.SignalDate == 0 {
		for _, horizon := range horizons {
			tracking.Horizons[horizonKey(horizon)] = unavailableHorizon(horizon, targetReturn, drawdownLimit, "无法解析信号日期")
		}
		return tracking
	}

	preOpenSignal := hasSignalTime && signalTime.Hour()*60+signalTime.Minute() < 9*60+30
	ordered := append([]TrackingBar(nil), bars...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
	forward := make([]TrackingBar, 0, len(ordered))
	for _, bar := range ordered {
		if !validTrackingBar(bar) {
			continue
		}
		if preOpenSignal && bar.Date == tracking.SignalDate {
			tracking.BasePrice = bar.Open
			tracking.BaseDate = bar.Date
			tracking.BasePriceFrom = "signal_day_open"
		}
		if !preOpenSignal && bar.Date <= tracking.SignalDate {
			tracking.BasePrice = bar.Close
			tracking.BaseDate = bar.Date
			tracking.BasePriceFrom = "signal_day_close"
		}
		if (preOpenSignal && bar.Date >= tracking.SignalDate) || (!preOpenSignal && bar.Date > tracking.SignalDate) {
			forward = append(forward, bar)
		}
	}
	if preOpenSignal && tracking.BasePrice <= 0 {
		for _, horizon := range horizons {
			result := SelectionHorizon{HorizonDays: horizon, Status: "pending", TargetReturn: targetReturn, DrawdownLimit: drawdownLimit}
			result.Reason = "暂无信号当日有效K线"
			tracking.Horizons[horizonKey(horizon)] = result
		}
		return tracking
	}
	if tracking.BasePrice <= 0 {
		if positiveFiniteTrackingValue(item.Latest) {
			tracking.BasePrice = item.Latest
			tracking.BaseDate = tracking.SignalDate
			tracking.BasePriceFrom = "selection_latest"
		}
	}
	if tracking.BasePrice <= 0 && len(forward) > 0 {
		if positiveFiniteTrackingValue(forward[0].YClose) {
			tracking.BasePrice = forward[0].YClose
			tracking.BaseDate = forward[0].Date
			tracking.BasePriceFrom = "first_forward_yclose"
		} else {
			tracking.BasePrice = forward[0].Open
			tracking.BaseDate = forward[0].Date
			tracking.BasePriceFrom = "first_forward_open"
		}
	}
	if tracking.BasePrice <= 0 {
		for _, horizon := range horizons {
			tracking.Horizons[horizonKey(horizon)] = unavailableHorizon(horizon, targetReturn, drawdownLimit, "缺少有效基准价格")
		}
		return tracking
	}

	for _, horizon := range horizons {
		result := SelectionHorizon{HorizonDays: horizon, TargetReturn: targetReturn, DrawdownLimit: drawdownLimit, CalculationVersion: "tracking-v3", DataQuality: "complete", FirstEvent: "none"}
		if horizon <= 0 {
			result.Status = "unavailable"
			result.Reason = "观察窗口必须大于0"
			tracking.Horizons[horizonKey(horizon)] = result
			continue
		}
		if len(forward) == 0 {
			result.Status = "pending"
			result.Reason = "暂无信号后的交易日K线"
			tracking.Horizons[horizonKey(horizon)] = result
			continue
		}
		if len(forward) < horizon {
			result.Status = "pending"
			result.AsOfDate = forward[len(forward)-1].Date
			result.Reason = fmt.Sprintf("已获得%d/%d个交易日", len(forward), horizon)
			tracking.Horizons[horizonKey(horizon)] = result
			continue
		}
		window := forward[:horizon]
		finalBar := window[len(window)-1]
		result.Status = "complete"
		result.AsOfDate = finalBar.Date
		result.FinalDate = finalBar.Date
		result.FinalOpen = finalBar.Open
		result.FinalClose = finalBar.Close
		result.FinalHigh = finalBar.High
		result.FinalLow = finalBar.Low
		closeTotal := 0.0
		for _, bar := range window {
			closeTotal += bar.Close
		}
		result.AverageClose = roundTrackingPrice(closeTotal / float64(len(window)))
		result.AverageCloseReturn = roundTrackingPercent(pctTracking(result.AverageClose, tracking.BasePrice))
		result.OpenReturn = roundTrackingPercent(pctTracking(finalBar.Open, tracking.BasePrice))
		result.CloseReturn = roundTrackingPercent(pctTracking(finalBar.Close, tracking.BasePrice))
		result.HighReturn = roundTrackingPercent(pctTracking(finalBar.High, tracking.BasePrice))
		result.LowReturn = roundTrackingPercent(pctTracking(finalBar.Low, tracking.BasePrice))
		maxGain := math.Inf(-1)
		maxDrawdown := math.Inf(1)
		maxPathDrawdown := 0.0
		peakPrice := tracking.BasePrice
		for index, bar := range window {
			if gain := pctTracking(bar.High, tracking.BasePrice); gain > maxGain {
				maxGain = gain
				result.MaxGainDate = bar.Date
				result.MaxGainPrice = bar.High
				result.DaysToMaxGain = index + 1
			}
			if drawdown := pctTracking(bar.Low, tracking.BasePrice); drawdown < maxDrawdown {
				maxDrawdown = drawdown
				result.MaxDrawdownDate = bar.Date
				result.MaxDrawdownPrice = bar.Low
				result.DaysToMaxDrawdown = index + 1
			}
			if bar.High > peakPrice {
				peakPrice = bar.High
			}
			if peakPrice > 0 {
				pathDrawdown := pctTracking(bar.Low, peakPrice)
				if pathDrawdown < maxPathDrawdown {
					maxPathDrawdown = pathDrawdown
				}
			}
			if !result.TargetHit && pctTracking(bar.High, tracking.BasePrice) >= targetReturn {
				result.TargetHit = true
				result.TargetHitDate = bar.Date
				result.DaysToTargetHit = index + 1
			}
			if !result.StopLossHit && pctTracking(bar.Low, tracking.BasePrice) <= -drawdownLimit {
				result.StopLossHit = true
				result.StopLossHitDate = bar.Date
				result.DaysToStopLoss = index + 1
			}
		}
		result.MaxGain = roundTrackingPercent(maxGain)
		result.MaxDrawdown = roundTrackingPercent(maxDrawdown)
		result.MFE = result.MaxGain
		result.MAE = result.MaxDrawdown
		result.MaxPathDrawdown = roundTrackingPercent(maxPathDrawdown)
		result.MaxGainPrice = roundTrackingPrice(result.MaxGainPrice)
		result.MaxDrawdownPrice = roundTrackingPrice(result.MaxDrawdownPrice)
		if result.TargetHit && result.StopLossHit {
			switch {
			case result.DaysToTargetHit < result.DaysToStopLoss:
				result.FirstEvent = "target"
			case result.DaysToStopLoss < result.DaysToTargetHit:
				result.FirstEvent = "stop"
			default:
				result.FirstEvent = "ambiguous"
			}
		} else if result.TargetHit {
			result.FirstEvent = "target"
		} else if result.StopLossHit {
			result.FirstEvent = "stop"
		}
		result.Success = result.CloseReturn >= targetReturn && result.MaxDrawdown >= -drawdownLimit
		if result.Success {
			result.Reason = "达到收益与回撤标准"
		} else {
			result.Reason = "未达到收益与回撤标准"
		}
		tracking.Horizons[horizonKey(horizon)] = result
	}
	return tracking
}

func selectionResultSignalTime(item SelectionResult) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
		return t.In(time.FixedZone("CST", 8*60*60)), true
	}
	if len(item.CreatedAt) >= 10 {
		if t, err := time.Parse("2006-01-02", item.CreatedAt[:10]); err == nil {
			return t, false
		}
	}
	return time.Time{}, false
}

func trackingDateInt(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

func validTrackingBar(bar TrackingBar) bool {
	if bar.Date <= 0 {
		return false
	}
	for _, value := range []float64{bar.Open, bar.High, bar.Low, bar.Close} {
		if !positiveFiniteTrackingValue(value) {
			return false
		}
	}
	return bar.High >= bar.Open && bar.High >= bar.Close && bar.High >= bar.Low &&
		bar.Low <= bar.Open && bar.Low <= bar.Close
}

func positiveFiniteTrackingValue(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func horizonKey(horizon int) string { return fmt.Sprintf("d%d", horizon) }

func unavailableHorizon(horizon int, targetReturn, drawdownLimit float64, reason string) SelectionHorizon {
	return SelectionHorizon{HorizonDays: horizon, Status: "unavailable", TargetReturn: targetReturn, DrawdownLimit: drawdownLimit, Reason: reason}
}

func pctTracking(value, base float64) float64 {
	if base <= 0 || value == 0 {
		return 0
	}
	return (value/base - 1) * 100
}

func roundTrackingPercent(value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return math.Round(value*100) / 100
}

func roundTrackingPrice(value float64) float64 {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0
	}
	return math.Round(value*1000) / 1000
}
