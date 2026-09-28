package workbench

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ExecutionPolicyNextQuote   = "next_quote"
	ExecutionPolicyNextDayOpen = "next_day_open"
	ExecutionVersion           = "execution-v1"
	NextQuoteCaptureWindow     = 5 * time.Minute
)

var paperMarketLocation = time.FixedZone("CST", 8*60*60)

func PaperTradingSession(at time.Time) bool {
	local := at.In(paperMarketLocation)
	minute := local.Hour()*60 + local.Minute()
	return minute >= 9*60+30 && minute < 11*60+30 || minute >= 13*60 && minute < 15*60
}

func ValidNextQuoteWindow(signalAt, quoteAt time.Time) bool {
	signal, quote := signalAt.In(paperMarketLocation), quoteAt.In(paperMarketLocation)
	return trackingDateInt(signal) == trackingDateInt(quote) && PaperTradingSession(signal) &&
		PaperTradingSession(quote) && quote.After(signal) && quote.Sub(signal) <= NextQuoteCaptureWindow
}

// PaperExecutionConfig is deliberately small and explicit. It models a buy
// plan only; it never sends an order to a broker. Exit rates are applied when
// a completed horizon is evaluated because the sell amount depends on its
// final close.
type PaperExecutionConfig struct {
	Quantity           int
	NextQuoteBPS       float64
	NextOpenBPS        float64
	CommissionRate     float64
	SellCommissionRate float64
	StampDutyRate      float64
	MinCommission      float64
	TransferRate       float64
}

func DefaultPaperExecutionConfig() PaperExecutionConfig {
	return PaperExecutionConfig{
		Quantity: 100, NextQuoteBPS: 5, NextOpenBPS: 10,
		CommissionRate: 0.0003, SellCommissionRate: 0.0003,
		StampDutyRate: 0.001, MinCommission: 5, TransferRate: 0.00001,
	}
}

type SelectionSignalExecution struct {
	ID                string  `json:"id"`
	SelectionResultID string  `json:"selection_result_id"`
	RunID             string  `json:"run_id"`
	FormulaID         string  `json:"formula_id"`
	Symbol            string  `json:"symbol"`
	PolicyName        string  `json:"policy_name"`
	PolicyVersion     string  `json:"policy_version"`
	ExecutionVersion  string  `json:"execution_version"`
	Side              string  `json:"side"`
	Status            string  `json:"status"`
	SignalAt          string  `json:"signal_at"`
	SignalPrice       float64 `json:"signal_price"`
	PlannedAt         string  `json:"planned_at"`
	EntryAt           string  `json:"entry_at"`
	ReferencePrice    float64 `json:"reference_price"`
	FillPrice         float64 `json:"fill_price"`
	Quantity          int     `json:"quantity"`
	SlippageBPS       float64 `json:"slippage_bps"`
	GrossAmount       float64 `json:"gross_amount"`
	Commission        float64 `json:"commission"`
	StampDuty         float64 `json:"stamp_duty"`
	TransferFee       float64 `json:"transfer_fee"`
	TotalCost         float64 `json:"total_cost"`
	PriceSource       string  `json:"price_source"`
	FailureReason     string  `json:"failure_reason"`
	DataQuality       string  `json:"data_quality"`
	StrategyVersion   string  `json:"strategy_version"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type SelectionSignalObservation struct {
	ID                   string  `json:"id"`
	SelectionResultID    string  `json:"selection_result_id"`
	ExecutionID          string  `json:"execution_id"`
	Symbol               string  `json:"symbol"`
	TradeDate            int     `json:"trade_date"`
	MarketDayIndex       int     `json:"market_day_index"`
	SymbolDayIndex       int     `json:"symbol_day_index"`
	SessionKind          string  `json:"session_kind"`
	Open                 float64 `json:"open"`
	High                 float64 `json:"high"`
	Low                  float64 `json:"low"`
	Close                float64 `json:"close"`
	Volume               float64 `json:"volume"`
	Amount               float64 `json:"amount"`
	SignalOpenReturn     float64 `json:"signal_open_return"`
	SignalHighReturn     float64 `json:"signal_high_return"`
	SignalLowReturn      float64 `json:"signal_low_return"`
	SignalCloseReturn    float64 `json:"signal_close_return"`
	TradeOpenReturn      float64 `json:"trade_open_return"`
	TradeHighReturn      float64 `json:"trade_high_return"`
	TradeLowReturn       float64 `json:"trade_low_return"`
	TradeCloseReturn     float64 `json:"trade_close_return"`
	BenchmarkSymbol      string  `json:"benchmark_symbol"`
	BenchmarkCloseReturn float64 `json:"benchmark_close_return"`
	ExcessCloseReturn    float64 `json:"excess_close_return"`
	AdjustmentType       string  `json:"adjustment_type"`
	DataStatus           string  `json:"data_status"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

func BuildSelectionSignalExecutions(item SelectionResult, bars []TrackingBar, now time.Time) []SelectionSignalExecution {
	config := DefaultPaperExecutionConfig()
	result := []SelectionSignalExecution{
		buildSelectionExecution(item, ExecutionPolicyNextQuote, config, nil, now),
		buildSelectionExecution(item, ExecutionPolicyNextDayOpen, config, bars, now),
	}
	return result
}

func buildSelectionExecution(item SelectionResult, policy string, config PaperExecutionConfig, bars []TrackingBar, now time.Time) SelectionSignalExecution {
	createdAt := now.Format(time.RFC3339)
	execution := SelectionSignalExecution{
		SelectionResultID: item.ID, RunID: item.RunID, FormulaID: item.FormulaID, Symbol: item.Symbol,
		PolicyName: policy, PolicyVersion: "1", ExecutionVersion: ExecutionVersion, Side: "buy",
		Status: "planned", SignalAt: item.SignalAt, SignalPrice: item.SignalPrice, PlannedAt: item.SignalAt,
		Quantity: config.Quantity, StrategyVersion: item.StrategyVersion, DataQuality: "partial",
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	if execution.SignalPrice <= 0 {
		execution.Status = "unavailable"
		execution.FailureReason = "信号价缺失"
		execution.DataQuality = "suspect"
		return execution
	}

	var reference, slippage float64
	var entryAt, source string
	switch policy {
	case ExecutionPolicyNextQuote:
		signalAt, signalErr := time.Parse(time.RFC3339, item.SignalAt)
		if signalErr != nil || !PaperTradingSession(signalAt) {
			execution.Status, execution.FailureReason = "unavailable", "信号不在可交易时段"
			return execution
		}
		if strings.TrimSpace(item.NextQuoteAt) == "" || item.NextQuotePrice <= 0 {
			localSignal := signalAt.In(paperMarketLocation)
			marketClose := time.Date(localSignal.Year(), localSignal.Month(), localSignal.Day(), 15, 0, 0, 0, paperMarketLocation)
			if !now.Before(marketClose) || !now.Before(signalAt.Add(NextQuoteCaptureWindow)) {
				execution.Status, execution.FailureReason = "unavailable", "交易时段内未捕获有效后续报价"
			} else {
				execution.FailureReason = "等待信号后的有效行情快照"
			}
			return execution
		}
		if quoteAt, quoteErr := time.Parse(time.RFC3339, item.NextQuoteAt); quoteErr != nil || !ValidNextQuoteWindow(signalAt, quoteAt) {
			execution.Status, execution.FailureReason = "unavailable", "后续报价不在信号后的同一交易时段"
			return execution
		}
		reference, slippage, entryAt, source = item.NextQuotePrice, config.NextQuoteBPS, item.NextQuoteAt, item.NextQuoteSource
	case ExecutionPolicyNextDayOpen:
		if len(bars) == 0 {
			execution.FailureReason = "等待下一交易日开盘行情"
			return execution
		}
		signalDate := selectionSignalDate(item)
		for _, bar := range sortedValidTrackingBars(bars) {
			if bar.Date <= signalDate {
				continue
			}
			if bar.Open <= 0 {
				execution.Status, execution.FailureReason, execution.DataQuality = "unavailable", "开盘价无效", "suspect"
				return execution
			}
			if bar.YClose > 0 && bar.Open >= bar.YClose*(1+priceLimitRate(item.Symbol)-1e-8) {
				execution.Status, execution.FailureReason = "rejected", "次日开盘涨停，无法买入"
				execution.ReferencePrice, execution.EntryAt, execution.PriceSource = bar.Open, dateText(bar.Date), "daily_open"
				return execution
			}
			reference, slippage, entryAt, source = bar.Open, config.NextOpenBPS, dateText(bar.Date), "daily_open"
			break
		}
		if reference <= 0 {
			execution.FailureReason = "没有可用的下一交易日行情"
			return execution
		}
	default:
		execution.Status, execution.FailureReason = "unavailable", "不支持的成交模型"
		execution.DataQuality = "suspect"
		return execution
	}

	execution.ReferencePrice, execution.SlippageBPS, execution.EntryAt, execution.PriceSource = reference, slippage, entryAt, source
	execution.FillPrice = roundExecutionPrice(reference * (1 + slippage/10000))
	if policy == ExecutionPolicyNextDayOpen && item.Symbol != "" && execution.FillPrice >= reference*(1+priceLimitRate(item.Symbol)) {
		execution.Status, execution.FailureReason = "rejected", "滑点后超过涨停价"
		return execution
	}
	execution.GrossAmount = roundExecutionMoney(execution.FillPrice * float64(execution.Quantity))
	execution.Commission = math.Max(roundExecutionMoney(execution.GrossAmount*config.CommissionRate), config.MinCommission)
	execution.TransferFee = roundExecutionMoney(execution.GrossAmount * config.TransferRate)
	execution.TotalCost = roundExecutionMoney(execution.Commission + execution.TransferFee + execution.StampDuty)
	execution.Status, execution.DataQuality = "filled", "complete"
	return execution
}

func BuildSelectionObservations(item SelectionResult, execution SelectionSignalExecution, bars []TrackingBar, now time.Time, marketBars ...[]TrackingBar) []SelectionSignalObservation {
	ordered := sortedValidTrackingBars(bars)
	signalBase := item.SignalPrice
	if signalBase <= 0 {
		signalBase = item.Latest
	}
	tradeBase := execution.FillPrice
	signalDate := selectionSignalDate(item)
	marketDayByDate := map[int]int{}
	if len(marketBars) > 0 {
		marketDay := 0
		for _, bar := range sortedValidTrackingBars(marketBars[0]) {
			if bar.Date > signalDate {
				marketDay++
				marketDayByDate[bar.Date] = marketDay
			}
		}
	}
	result := make([]SelectionSignalObservation, 0, len(ordered))
	dayIndex := 0
	signalTime, hasSignalTime := selectionResultSignalTime(item)
	for _, bar := range ordered {
		if !hasSignalTime || signalTime.Hour() >= 15 {
			break
		}
		if bar.Date != signalDate {
			continue
		}
		// Daily high/low may have occurred before the signal. Without
		// minute-level data only retain the signal-day close as D0 and mark
		// intraday path fields unavailable.
		observation := SelectionSignalObservation{
			SelectionResultID: item.ID, ExecutionID: execution.ID, Symbol: item.Symbol,
			TradeDate: bar.Date, MarketDayIndex: 0, SymbolDayIndex: 0, SessionKind: "d0",
			Close: bar.Close, Volume: bar.Volume, Amount: bar.Amount, DataStatus: "partial",
			CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
		}
		observation.SignalCloseReturn = returnPercent(bar.Close, signalBase)
		if execution.Status == "filled" && tradeBase > 0 {
			observation.TradeCloseReturn = returnPercent(bar.Close, tradeBase)
		}
		result = append(result, observation)
		break
	}
	for _, bar := range ordered {
		if bar.Date <= signalDate {
			continue
		}
		dayIndex++
		observation := SelectionSignalObservation{
			SelectionResultID: item.ID, ExecutionID: execution.ID, Symbol: item.Symbol, TradeDate: bar.Date,
			MarketDayIndex: marketDayByDate[bar.Date], SymbolDayIndex: dayIndex, SessionKind: "forward", Open: bar.Open, High: bar.High, Low: bar.Low,
			Close: bar.Close, Volume: bar.Volume, Amount: bar.Amount, DataStatus: "valid",
			CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
		}
		if len(marketBars) > 0 && observation.MarketDayIndex == 0 {
			observation.DataStatus = "partial"
		}
		observation.SignalOpenReturn = returnPercent(bar.Open, signalBase)
		observation.SignalHighReturn = returnPercent(bar.High, signalBase)
		observation.SignalLowReturn = returnPercent(bar.Low, signalBase)
		observation.SignalCloseReturn = returnPercent(bar.Close, signalBase)
		if execution.Status == "filled" && tradeBase > 0 {
			observation.TradeOpenReturn = returnPercent(bar.Open, tradeBase)
			observation.TradeHighReturn = returnPercent(bar.High, tradeBase)
			observation.TradeLowReturn = returnPercent(bar.Low, tradeBase)
			observation.TradeCloseReturn = returnPercent(bar.Close, tradeBase)
		}
		result = append(result, observation)
	}
	return result
}

func EvaluateSelectionExecutionTracking(item SelectionResult, execution SelectionSignalExecution, observations []SelectionSignalObservation, horizons []int, targetReturn, drawdownLimit float64, now time.Time) SelectionTracking {
	item.SignalPrice = execution.FillPrice
	item.SnapshotStatus = "valid"
	bars := make([]TrackingBar, 0, len(observations))
	for _, observation := range observations {
		bars = append(bars, TrackingBar{Date: observation.TradeDate, Open: observation.Open, High: observation.High, Low: observation.Low, Close: observation.Close, Volume: observation.Volume, Amount: observation.Amount})
	}
	tracking := EvaluateSelectionTracking(item, bars, horizons, targetReturn, drawdownLimit, now)
	for key, horizon := range tracking.Horizons {
		if horizon.Status != "complete" {
			continue
		}
		horizon.GrossReturn = horizon.CloseReturn
		sellCost := paperSellCost(horizon.FinalClose * float64(execution.Quantity))
		horizon.TotalCost = roundExecutionMoney(execution.TotalCost + sellCost)
		if execution.GrossAmount > 0 {
			netAmount := horizon.FinalClose*float64(execution.Quantity) - execution.GrossAmount - horizon.TotalCost
			horizon.NetReturn = math.Round(netAmount/execution.GrossAmount*10000) / 100
		}
		for _, observation := range observations {
			if observation.SymbolDayIndex == horizon.HorizonDays {
				horizon.BenchmarkCloseReturn = observation.BenchmarkCloseReturn
				horizon.ExcessCloseReturn = observation.TradeCloseReturn - observation.BenchmarkCloseReturn
				break
			}
		}
		tracking.Horizons[key] = horizon
	}
	return tracking
}

func paperSellCost(sellAmount float64) float64 {
	if sellAmount <= 0 {
		return 0
	}
	config := DefaultPaperExecutionConfig()
	commission := math.Max(roundExecutionMoney(sellAmount*config.SellCommissionRate), config.MinCommission)
	stampDuty := roundExecutionMoney(sellAmount * config.StampDutyRate)
	transfer := roundExecutionMoney(sellAmount * config.TransferRate)
	return roundExecutionMoney(commission + stampDuty + transfer)
}

func ApplyBenchmarkToObservations(item SelectionResult, observations []SelectionSignalObservation, benchmark []TrackingBar, benchmarkSymbol string) {
	if len(observations) == 0 || len(benchmark) == 0 {
		return
	}
	baseDate := selectionSignalDate(item)
	basePrice := 0.0
	for _, bar := range sortedValidTrackingBars(benchmark) {
		if bar.Date <= baseDate {
			basePrice = bar.Close
		}
	}
	if basePrice <= 0 {
		return
	}
	byDate := map[int]float64{}
	for _, bar := range sortedValidTrackingBars(benchmark) {
		byDate[bar.Date] = bar.Close
	}
	for index := range observations {
		benchmarkClose, ok := byDate[observations[index].TradeDate]
		if !ok || benchmarkClose <= 0 {
			observations[index].DataStatus = "partial"
			continue
		}
		observations[index].BenchmarkSymbol = benchmarkSymbol
		observations[index].BenchmarkCloseReturn = returnPercent(benchmarkClose, basePrice)
		stockReturn := observations[index].SignalCloseReturn
		if observations[index].ExecutionID != "" {
			stockReturn = observations[index].TradeCloseReturn
		}
		observations[index].ExcessCloseReturn = stockReturn - observations[index].BenchmarkCloseReturn
	}
}

func sortedValidTrackingBars(bars []TrackingBar) []TrackingBar {
	ordered := append([]TrackingBar(nil), bars...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
	result := ordered[:0]
	for _, bar := range ordered {
		if bar.Date <= 0 || bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 ||
			bar.High < bar.Open || bar.High < bar.Close || bar.High < bar.Low || bar.Low > bar.Open || bar.Low > bar.Close {
			continue
		}
		result = append(result, bar)
	}
	return result
}

func selectionSignalDate(item SelectionResult) int {
	raw := item.SignalAt
	if strings.TrimSpace(raw) == "" {
		raw = item.CreatedAt
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		local := parsed.In(time.FixedZone("CST", 8*60*60))
		return local.Year()*10000 + int(local.Month())*100 + local.Day()
	}
	if len(raw) >= 10 {
		if parsed, err := time.ParseInLocation("2006-01-02", raw[:10], time.Local); err == nil {
			return parsed.Year()*10000 + int(parsed.Month())*100 + parsed.Day()
		}
	}
	return 0
}

func priceLimitRate(symbol string) float64 {
	symbol = strings.ToLower(strings.TrimSpace(symbol))
	if len(symbol) > 2 && (strings.HasPrefix(symbol, "sh") || strings.HasPrefix(symbol, "sz") || strings.HasPrefix(symbol, "bj")) {
		symbol = symbol[2:]
	}
	if strings.HasPrefix(symbol, "30") || strings.HasPrefix(symbol, "68") || strings.HasPrefix(symbol, "sz30") || strings.HasPrefix(symbol, "sh68") {
		return 0.20
	}
	if strings.HasPrefix(symbol, "8") || strings.HasPrefix(symbol, "4") || strings.HasPrefix(symbol, "92") || strings.HasPrefix(symbol, "bj") {
		return 0.30
	}
	return 0.10
}

func dateText(value int) string {
	text := stringInt(value)
	if len(text) != 8 {
		return ""
	}
	return text[:4] + "-" + text[4:6] + "-" + text[6:] + "T09:30:00+08:00"
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}

func returnPercent(value, base float64) float64 {
	if value <= 0 || base <= 0 {
		return 0
	}
	return math.Round((value/base-1)*10000) / 100
}

func roundExecutionPrice(value float64) float64 { return math.Round(value*1000) / 1000 }
func roundExecutionMoney(value float64) float64 { return math.Round(value*100) / 100 }
