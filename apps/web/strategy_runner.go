package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type StrategyConfig struct {
	Universe        StrategyUniverseExpression `json:"universe"`
	PoolID          string                     `json:"pool_id"`
	Symbols         []string                   `json:"symbols"`
	Filters         []StrategyFactorRule       `json:"filters"`
	Scores          []StrategyFactorRule       `json:"scores"`
	Pass            StrategyPassConfig         `json:"pass"`
	Period          string                     `json:"period"`
	Right           int                        `json:"right"`
	CalcCount       int                        `json:"calc_count"`
	BatchSize       int                        `json:"batch_size"`
	ScanLimit       *int                       `json:"scan_limit,omitempty"`
	ContinueOnError bool                       `json:"continue_on_error"`
}

type StrategyFactorRule struct {
	ID     string                 `json:"id"`
	Factor string                 `json:"factor"`
	Weight float64                `json:"weight"`
	Params map[string]interface{} `json:"params"`
}

type StrategyPassConfig struct {
	MinScore float64 `json:"min_score"`
	TopN     int     `json:"top_n"`
}

type StrategyRunResult struct {
	Strategy      Strategy                   `json:"strategy"`
	Total         int                        `json:"total"`
	Matched       int                        `json:"matched"`
	Items         []StrategySelectionItem    `json:"items"`
	Errors        map[string]string          `json:"errors,omitempty"`
	Warnings      []string                   `json:"warnings,omitempty"`
	Universe      StrategyUniverseResult     `json:"universe"`
	FormulaCache  map[string]map[string]bool `json:"-"`
	FormulaDetail map[string]map[string]any  `json:"-"`
	KlineCache    map[string][]FormulaKline  `json:"-"`
	PoolCache     map[string]map[string]bool `json:"-"`
	FormulaByName map[string]Formula         `json:"-"`
	FormulaByID   map[string]Formula         `json:"-"`
	Config        StrategyConfig             `json:"config"`
}

type StrategySelectionItem struct {
	Symbol         string                 `json:"symbol"`
	Score          float64                `json:"score"`
	Hit            bool                   `json:"hit"`
	Latest         float64                `json:"latest"`
	Reasons        []string               `json:"reasons"`
	FactorResults  []StrategyFactorResult `json:"factor_results"`
	SignalAt       string                 `json:"signal_at,omitempty"`
	SignalPrice    float64                `json:"signal_price,omitempty"`
	QuoteAt        string                 `json:"quote_at,omitempty"`
	QuoteSource    string                 `json:"quote_source,omitempty"`
	PriceType      string                 `json:"price_type,omitempty"`
	QuoteAgeMS     int64                  `json:"quote_age_ms,omitempty"`
	SnapshotStatus string                 `json:"snapshot_status,omitempty"`
}

type StrategyFactorResult struct {
	ID     string  `json:"id"`
	Factor string  `json:"factor"`
	Hit    bool    `json:"hit"`
	Score  float64 `json:"score"`
	Value  any     `json:"value,omitempty"`
	Reason string  `json:"reason"`
}

const strategyBenchmarkSymbol = "sh000001"

func (r *AutomationRunner) runStrategySelection(ctx context.Context, task AutomationTask, run AutomationRun) (interface{}, []string, error) {
	var payload struct {
		StrategyID string `json:"strategy_id"`
	}
	if err := json.Unmarshal([]byte(task.PayloadJSON), &payload); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(payload.StrategyID) == "" {
		return nil, nil, errors.New("策略选股任务缺少strategy_id")
	}
	strategy, err := r.store.GetStrategy(payload.StrategyID)
	if err != nil {
		return nil, nil, err
	}
	result, err := r.executeStrategy(ctx, strategy)
	if err != nil {
		return nil, nil, err
	}
	items := strategyItemsToSelectionResults(result.Items)
	if err := r.store.SaveSelectionResults(run, Formula{ID: "strategy:" + strategy.ID, Name: strategy.Name}, items); err != nil {
		return nil, nil, err
	}
	symbols := make([]string, 0, len(items))
	for _, item := range items {
		symbols = append(symbols, item.Symbol)
	}
	return result, symbols, nil
}

func (r *AutomationRunner) executeStrategy(ctx context.Context, strategy Strategy) (StrategyRunResult, error) {
	var cfg StrategyConfig
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &cfg); err != nil {
		return StrategyRunResult{}, err
	}
	if cfg.Period == "" {
		cfg.Period = "day"
	}
	if cfg.CalcCount <= 0 {
		cfg.CalcCount = 260
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.BatchSize > 200 {
		cfg.BatchSize = 200
	}
	universe, err := r.strategyUniverseResult(cfg, strategyMaxCodes(cfg))
	if err != nil {
		return StrategyRunResult{}, err
	}
	symbols := universe.Symbols
	if len(symbols) == 0 {
		return StrategyRunResult{}, errors.New("策略股票范围为空")
	}

	result := StrategyRunResult{
		Strategy:      strategy,
		Config:        cfg,
		Total:         len(symbols),
		Universe:      universe,
		Errors:        map[string]string{},
		FormulaCache:  map[string]map[string]bool{},
		FormulaDetail: map[string]map[string]any{},
		KlineCache:    map[string][]FormulaKline{},
		PoolCache:     map[string]map[string]bool{},
		FormulaByName: map[string]Formula{},
		FormulaByID:   map[string]Formula{},
	}
	if universe.Truncated {
		result.Warnings = append(result.Warnings, fmt.Sprintf("选股范围覆盖 %d 只，当前运行保护仅扫描前 %d 只", universe.Total, len(universe.Symbols)))
	}
	if err := r.prepareStrategyMarketContext(ctx, &result); err != nil {
		return StrategyRunResult{}, err
	}
	if !r.strategyMarketContextPasses(&result) {
		result.Items = []StrategySelectionItem{}
		result.Matched = 0
		result.Errors = nil
		return result, nil
	}
	if err := r.prepareStrategyFormulas(ctx, &result, symbols); err != nil {
		return StrategyRunResult{}, err
	}

	items := make([]StrategySelectionItem, 0)
	for _, symbol := range symbols {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		item, ok, err := r.evaluateStrategySymbol(ctx, strategy, &result, symbol)
		if err != nil {
			result.Errors[symbol] = err.Error()
			if !result.Config.ContinueOnError {
				return result, err
			}
			continue
		}
		if !ok {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Symbol < items[j].Symbol
		}
		return items[i].Score > items[j].Score
	})
	if result.Config.Pass.TopN > 0 && len(items) > result.Config.Pass.TopN {
		items = items[:result.Config.Pass.TopN]
	}
	result.Items = items
	result.Matched = len(items)
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result, nil
}

func strategyUsesMarketContext(config StrategyConfig) bool {
	for _, rule := range append(append([]StrategyFactorRule{}, config.Filters...), config.Scores...) {
		if rule.Factor == "market_momentum" {
			return true
		}
	}
	return false
}

func (r *AutomationRunner) prepareStrategyMarketContext(ctx context.Context, result *StrategyRunResult) error {
	if result == nil || !strategyUsesMarketContext(result.Config) {
		return nil
	}
	if rows := result.KlineCache[strategyBenchmarkSymbol]; len(rows) > 0 {
		return nil
	}
	resp, err := marketServiceIndexKline(ctx, strategyBenchmarkSymbol, formulaPeriodToKlineType(result.Config.Period), result.Config.CalcCount)
	if err != nil {
		return fmt.Errorf("上证指数K线加载失败: %w", err)
	}
	if resp == nil || len(resp.List) == 0 {
		return errors.New("上证指数K线为空")
	}
	result.KlineCache[strategyBenchmarkSymbol] = protocolKlinesToFormulaRows(resp.List, result.Config.CalcCount)
	return nil
}

func (r *AutomationRunner) strategyMarketContextPasses(result *StrategyRunResult) bool {
	if result == nil || !strategyUsesMarketContext(result.Config) {
		return true
	}
	rows := result.KlineCache[strategyBenchmarkSymbol]
	for _, rule := range result.Config.Filters {
		if rule.Factor == "market_momentum" && !r.evaluateFactor(result, strategyBenchmarkSymbol, rows, rule, true).Hit {
			return false
		}
	}
	return true
}

func (r *AutomationRunner) strategyUniverse(cfg StrategyConfig) ([]string, error) {
	result, err := r.strategyUniverseResult(cfg, strategyMaxCodes(cfg))
	return result.Symbols, err
}

func strategyMaxCodes(cfg StrategyConfig) int {
	// scan_limit is opt-in: 0 explicitly means scan the complete universe.
	// Omitting it preserves the default request-protection cap.
	if cfg.ScanLimit != nil && *cfg.ScanLimit >= 0 {
		return *cfg.ScanLimit
	}
	maxCodes := 300
	if cfg.BatchSize > 0 && cfg.BatchSize > maxCodes {
		maxCodes = cfg.BatchSize
	}
	return maxCodes
}

func (r *AutomationRunner) prepareStrategyFormulas(ctx context.Context, result *StrategyRunResult, symbols []string) error {
	rules := append([]StrategyFactorRule{}, result.Config.Filters...)
	rules = append(rules, result.Config.Scores...)
	for _, rule := range rules {
		if rule.Factor != "formula" {
			continue
		}
		formula, err := r.strategyFormula(rule)
		if err != nil {
			return err
		}
		result.FormulaByID[formula.ID] = formula
		result.FormulaByName[formula.Name] = formula
		if _, ok := result.FormulaCache[formula.ID]; ok {
			continue
		}
		allData := map[string]interface{}{}
		for _, batch := range chunkSymbols(symbols, result.Config.BatchSize) {
			resp, err := r.worker.Run(ctx, FormulaRunRequest{
				Symbols:       batch,
				Script:        formula.Script,
				Args:          json.RawMessage(formula.ArgsJSON),
				Period:        chooseString(result.Config.Period, formula.Period),
				Right:         chooseInt(result.Config.Right, formula.Right),
				OutCount:      1,
				CalcCount:     result.Config.CalcCount,
				ForceFallback: true,
			})
			if err == nil {
				mergeFormulaData(allData, resp.Data)
				continue
			}
			if !result.Config.ContinueOnError {
				return err
			}
			for _, symbol := range batch {
				singleResp, singleErr := r.worker.Run(ctx, FormulaRunRequest{
					Symbols:       []string{symbol},
					Script:        formula.Script,
					Args:          json.RawMessage(formula.ArgsJSON),
					Period:        chooseString(result.Config.Period, formula.Period),
					Right:         chooseInt(result.Config.Right, formula.Right),
					OutCount:      1,
					CalcCount:     result.Config.CalcCount,
					ForceFallback: true,
				})
				if singleErr != nil {
					result.Errors[symbol] = fmt.Sprintf("公式%s执行失败: %v", formula.Name, singleErr)
					continue
				}
				mergeFormulaData(allData, singleResp.Data)
			}
		}
		hits, details := formulaResponseMaps(allData)
		result.FormulaCache[formula.ID] = hits
		result.FormulaDetail[formula.ID] = details
	}
	return nil
}

func (r *AutomationRunner) strategyFormula(rule StrategyFactorRule) (Formula, error) {
	formulaID := stringParam(rule.Params, "formula_id", "")
	formulaName := stringParam(rule.Params, "formula_name", "")
	if formulaID != "" {
		return r.store.GetFormula(formulaID)
	}
	if formulaName == "" {
		return Formula{}, errors.New("公式因子缺少formula_id或formula_name")
	}
	formulas, err := r.store.ListFormulas()
	if err != nil {
		return Formula{}, err
	}
	for _, item := range formulas {
		if item.Name == formulaName {
			return item, nil
		}
	}
	return Formula{}, fmt.Errorf("公式不存在: %s", formulaName)
}

func (r *AutomationRunner) evaluateStrategySymbol(ctx context.Context, strategy Strategy, result *StrategyRunResult, symbol string) (StrategySelectionItem, bool, error) {
	item := StrategySelectionItem{Symbol: symbol, Hit: true}
	rows, err := r.strategyKline(ctx, result, symbol)
	if err != nil {
		return item, false, err
	}
	if len(rows) > 0 {
		item.Latest = rows[len(rows)-1].Close
		item.SignalAt = nowText()
		item.SignalPrice = item.Latest
		item.PriceType = "latest"
		item.SnapshotStatus = "inferred"
	}
	for _, rule := range result.Config.Filters {
		fr := r.evaluateFactor(result, symbol, rows, rule, true)
		item.FactorResults = append(item.FactorResults, fr)
		item.Reasons = append(item.Reasons, fr.Reason)
		if !fr.Hit {
			item.Hit = false
		}
	}
	if !item.Hit {
		return item, false, nil
	}
	score := 0.0
	for _, rule := range result.Config.Scores {
		fr := r.evaluateFactor(result, symbol, rows, rule, false)
		item.FactorResults = append(item.FactorResults, fr)
		if fr.Hit {
			score += fr.Score
		}
		item.Reasons = append(item.Reasons, fr.Reason)
	}
	item.Score = score
	minScore := result.Config.Pass.MinScore
	if minScore <= 0 {
		minScore = 1
	}
	item.Hit = score >= minScore
	if !item.Hit {
		return item, false, nil
	}
	return item, true, nil
}

func (r *AutomationRunner) strategyKline(ctx context.Context, result *StrategyRunResult, symbol string) ([]FormulaKline, error) {
	if rows, ok := result.KlineCache[symbol]; ok {
		return rows, nil
	}
	rows, err := loadFormulaKlineWithContext(ctx, symbol, result.Config.Period, result.Config.CalcCount)
	if err != nil {
		return nil, fmt.Errorf("%s K线加载失败: %w", symbol, err)
	}
	result.KlineCache[symbol] = rows
	return rows, nil
}

func (r *AutomationRunner) evaluateFactor(result *StrategyRunResult, symbol string, rows []FormulaKline, rule StrategyFactorRule, filter bool) StrategyFactorResult {
	fr := StrategyFactorResult{ID: rule.ID, Factor: rule.Factor}
	weight := rule.Weight
	if filter {
		weight = 0
	}
	// 每个 case 命中后把 fr.Score 设为归一化强度(0~1]，末尾再乘权重。
	switch rule.Factor {
	case "market_momentum":
		days := intParam(rule.Params, "days", 20)
		minPct := floatParam(rule.Params, "min", -math.MaxFloat64)
		maxPct := floatParam(rule.Params, "max", math.MaxFloat64)
		benchmarkRows := result.KlineCache[strategyBenchmarkSymbol]
		if len(rows) > 0 && len(benchmarkRows) > 0 {
			end := sort.Search(len(benchmarkRows), func(i int) bool { return benchmarkRows[i].Date > latest(rows).Date })
			benchmarkRows = benchmarkRows[:end]
		}
		if days <= 0 || len(benchmarkRows) < days+1 || len(rows) == 0 || latest(benchmarkRows).Date != latest(rows).Date {
			fr.Reason = fmt.Sprintf("市场状态数据不足，需要上证指数与个股同日且至少%d根K线", days+1)
			break
		}
		base := benchmarkRows[len(benchmarkRows)-1-days].Close
		current := latest(benchmarkRows).Close
		if base <= 0 {
			fr.Reason = "市场状态基准价格无效"
			break
		}
		momentum := (current - base) * 100 / base
		fr.Hit = momentum >= minPct && momentum <= maxPct
		fr.Score = 1
		fr.Value = momentum
		fr.Reason = fmt.Sprintf("上证指数%d日涨幅 %.2f%% 在 %.2f-%.2f%%", days, momentum, minPct, maxPct)
	case "pool_exclude":
		poolID := stringParam(rule.Params, "pool_id", DecisionExcludePoolID)
		inPool := r.strategyPoolContains(result, poolID, symbol)
		fr.Hit = !inPool
		fr.Score = 1
		fr.Value = inPool
		fr.Reason = fmt.Sprintf("不在%s: %t", poolID, fr.Hit)
	case "min_amount":
		value := floatParam(rule.Params, "value", 0)
		amount := latest(rows).Amount
		fr.Hit = amount >= value
		if value > 0 {
			fr.Score = clamp01(amount / value / 3)
		} else {
			fr.Score = 1
		}
		fr.Value = amount
		fr.Reason = fmt.Sprintf("成交额 %.0f >= %.0f", amount, value)
	case "price_range":
		minValue := floatParam(rule.Params, "min", 0)
		maxValue := floatParam(rule.Params, "max", math.MaxFloat64)
		closePrice := latest(rows).Close
		fr.Hit = closePrice >= minValue && closePrice <= maxValue
		fr.Score = 1
		fr.Value = closePrice
		fr.Reason = fmt.Sprintf("收盘价 %.2f 在 %.2f-%.2f", closePrice, minValue, maxValue)
	case "change_range":
		minValue := floatParam(rule.Params, "min", -math.MaxFloat64)
		maxValue := floatParam(rule.Params, "max", math.MaxFloat64)
		row := latest(rows)
		change := 0.0
		if row.YClose > 0 {
			change = (row.Close - row.YClose) * 100 / row.YClose
		}
		fr.Hit = change >= minValue && change <= maxValue
		fr.Score = 1
		fr.Value = change
		fr.Reason = fmt.Sprintf("涨跌幅 %.2f%% 在 %.2f-%.2f", change, minValue, maxValue)
	case "max_amplitude":
		days := intParam(rule.Params, "days", 5)
		maxPct := floatParam(rule.Params, "max", 8)
		if days <= 0 || len(rows) < days {
			fr.Reason = fmt.Sprintf("振幅数据不足，需要至少%d根K线", days)
			break
		}
		sum, count := 0.0, 0
		for _, row := range rows[len(rows)-days:] {
			base := row.YClose
			if base <= 0 {
				base = row.Close
			}
			if base <= 0 {
				continue
			}
			sum += (row.High - row.Low) * 100 / base
			count++
		}
		if count == 0 {
			fr.Reason = "振幅基准价格无效"
			break
		}
		avgAmp := sum / float64(count)
		fr.Hit = avgAmp <= maxPct
		fr.Score = 1
		fr.Value = avgAmp
		fr.Reason = fmt.Sprintf("近%d日平均振幅 %.2f%% <= %.2f%%", days, avgAmp, maxPct)
	case "ma_trend":
		short := intParam(rule.Params, "short", 5)
		mid := intParam(rule.Params, "mid", 10)
		long := intParam(rule.Params, "long", 20)
		maShort, maMid, maLong := ma(rows, short), ma(rows, mid), ma(rows, long)
		closePrice := latest(rows).Close
		fr.Hit = closePrice >= maShort && maShort >= maMid && maMid >= maLong
		fr.Score = 1
		fr.Value = map[string]float64{"close": closePrice, "short": maShort, "mid": maMid, "long": maLong}
		fr.Reason = fmt.Sprintf("均线多头 C %.2f / MA%d %.2f / MA%d %.2f / MA%d %.2f", closePrice, short, maShort, mid, maMid, long, maLong)
	case "volume_up":
		days := intParam(rule.Params, "days", 5)
		ratio := floatParam(rule.Params, "ratio", 1.3)
		avg := strategyAvgVol(rows, days)
		vol := latest(rows).Vol
		fr.Hit = avg > 0 && vol >= avg*ratio
		if fr.Hit {
			fr.Score = clamp01((vol/avg - ratio) / (ratio * 0.5))
		}
		fr.Value = map[string]float64{"volume": vol, "avg_volume": avg}
		fr.Reason = fmt.Sprintf("放量 %.0f >= %.2fx %d日均量 %.0f", vol, ratio, days, avg)
	case "break_high":
		days := intParam(rule.Params, "days", 20)
		high := highestHigh(rows, days)
		closePrice := latest(rows).Close
		fr.Hit = high > 0 && closePrice >= high
		if fr.Hit {
			fr.Score = clamp01((closePrice - high) / high * 100 / 3)
		}
		fr.Value = map[string]float64{"close": closePrice, "high": high}
		fr.Reason = fmt.Sprintf("突破%d日高点 C %.2f / H %.2f", days, closePrice, high)
	case "gain_days":
		days := intParam(rule.Params, "days", 20)
		minPct := floatParam(rule.Params, "min", 5)
		maxPct := floatParam(rule.Params, "max", 50)
		if days <= 0 || len(rows) < days+1 {
			fr.Reason = fmt.Sprintf("N日涨幅数据不足，需要至少%d根K线", days+1)
			break
		}
		base := rows[len(rows)-1-days].Close
		closePrice := latest(rows).Close
		if base <= 0 {
			fr.Reason = "N日涨幅基准价格无效"
			break
		}
		gain := (closePrice - base) * 100 / base
		fr.Value = map[string]float64{"base": base, "close": closePrice, "gain": gain}
		if filter {
			fr.Hit = gain >= minPct && gain <= maxPct
			fr.Score = 1
			fr.Reason = fmt.Sprintf("%d日涨幅 %.2f%% 在 %.2f-%.2f%%", days, gain, minPct, maxPct)
			break
		}
		fr.Hit = true
		switch {
		case gain < minPct:
			fr.Score = clamp01(gain / math.Max(minPct, 0.01))
			fr.Reason = fmt.Sprintf("%d日涨幅 %.2f%% 低于下限 %.2f%%", days, gain, minPct)
		case gain > maxPct:
			fr.Score = clamp01(1 - (gain-maxPct)/15)
			fr.Reason = fmt.Sprintf("%d日涨幅 %.2f%% 超出上限 %.2f%%", days, gain, maxPct)
		default:
			fr.Score = 1
			fr.Reason = fmt.Sprintf("%d日涨幅 %.2f%% 落在 %.2f-%.2f%%", days, gain, minPct, maxPct)
		}
	case "drawdown_from_high":
		days := intParam(rule.Params, "days", 60)
		minPct := floatParam(rule.Params, "min", 5)
		maxPct := floatParam(rule.Params, "max", 15)
		high := highestHigh(rows, days)
		closePrice := latest(rows).Close
		if days <= 0 || high <= 0 || closePrice <= 0 {
			fr.Reason = fmt.Sprintf("距高点回撤参数或数据不足 days=%d high=%.2f close=%.2f", days, high, closePrice)
			break
		}
		depth := (high - closePrice) * 100 / high
		fr.Value = map[string]float64{"high": high, "close": closePrice, "depth": depth}
		if filter {
			fr.Hit = depth >= minPct && depth <= maxPct
			fr.Score = 1
			fr.Reason = fmt.Sprintf("距%d日高点回撤 %.2f%% 在 %.2f-%.2f%%", days, depth, minPct, maxPct)
			break
		}
		fr.Hit = true
		switch {
		case depth < minPct:
			fr.Score = clamp01(depth / math.Max(minPct, 0.01))
			fr.Reason = fmt.Sprintf("距%d日高点回撤 %.2f%% 低于下限 %.2f%%", days, depth, minPct)
		case depth > maxPct:
			fr.Score = clamp01(1 - (depth-maxPct)/10)
			fr.Reason = fmt.Sprintf("距%d日高点回撤 %.2f%% 超出上限 %.2f%%", days, depth, maxPct)
		default:
			fr.Score = 1
			fr.Reason = fmt.Sprintf("距%d日高点回撤 %.2f%% 落在 %.2f-%.2f%%", days, depth, minPct, maxPct)
		}
	case "ma_slope":
		hit, raw, reason := evaluateMASlope(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			minSlope := floatParam(rule.Params, "min", 0)
			maxSlope := floatParam(rule.Params, "max", 20)
			fr.Score = clamp01((raw - minSlope) / math.Max(maxSlope-minSlope, 0.01))
		}
	case "close_strength":
		hit, raw, reason := evaluateCloseStrength(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			fr.Score = clamp01(raw)
		}
	case "rsi_rebound":
		hit, raw, reason := evaluateRSIRebound(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			maxCurrent := floatParam(rule.Params, "max_current", 55)
			fr.Score = clamp01(1 - raw/math.Max(maxCurrent, 0.01))
		}
	case "macd_golden_cross":
		hit, raw, reason := evaluateMACDSignal(rows, rule, true)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			closePrice := latest(rows).Close
			fr.Score = clamp01(raw / math.Max(closePrice, 0.01) * 100 / 0.3)
		}
	case "macd_dead_cross":
		hit, raw, reason := evaluateMACDSignal(rows, rule, false)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			closePrice := latest(rows).Close
			fr.Score = clamp01(raw / math.Max(closePrice, 0.01) * 100 / 0.3)
		}
	case "kdj_golden_cross":
		hit, raw, reason := evaluateKDJGoldenCross(rows, rule)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			fr.Score = clamp01((100 - raw) / 100)
		}
	case "rsi_oversold":
		hit, raw, reason := evaluateRSIOversold(rows, rule)
		threshold := floatParam(rule.Params, "threshold", 30)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			fr.Score = clamp01((threshold - raw) / math.Max(threshold, 0.01))
		}
	case "boll_breakout":
		hit, raw, reason := evaluateBOLLBreakout(rows, rule)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			closePrice := latest(rows).Close
			if closePrice > 0 {
				fr.Score = clamp01(raw / closePrice / 0.05)
			} else {
				fr.Score = 1
			}
		}
	case "volume_breakout":
		hit, raw, reason := evaluateVolumeBreakout(rows, rule)
		ratio := floatParam(rule.Params, "ratio", 1.5)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			fr.Score = clamp01((raw - ratio) / (ratio * 0.5))
		}
	case "local_rocket":
		hit, raw, reason := evaluateLocalRocket(rows, rule)
		fr.Hit, fr.Reason = hit, reason
		fr.Value = raw
		if hit {
			fr.Score = clamp01(raw / 12)
		}
	case "formula":
		formula, err := r.strategyFormula(rule)
		if err != nil {
			fr.Hit = false
			fr.Reason = err.Error()
			break
		}
		hits := result.FormulaCache[formula.ID]
		fr.Hit = hits[strategyNormalizeSymbol(symbol)]
		fr.Score = 1
		fr.Value = formula.Name
		fr.Reason = fmt.Sprintf("公式%s命中: %t", formula.Name, fr.Hit)
	default:
		recognized, hit, strength, value, reason := evaluateAdvancedStrategyFactor(rows, rule, filter)
		if recognized {
			fr.Hit, fr.Score, fr.Value, fr.Reason = hit, strength, value, reason
		} else {
			fr.Hit = false
			fr.Reason = "未知因子: " + rule.Factor
		}
	}
	if fr.Hit {
		fr.Score *= weight
	}
	return fr
}

// clamp01 squeezes a value into [0,1].
func clamp01(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value >= 1 {
		return 1
	}
	return value
}

func (r *AutomationRunner) strategyPoolContains(result *StrategyRunResult, poolID string, symbol string) bool {
	if _, ok := result.PoolCache[poolID]; !ok {
		pool, err := r.store.GetStockPool(poolID)
		set := map[string]bool{}
		if err == nil {
			for _, item := range normalizeSymbols(pool.Symbols) {
				set[item] = true
			}
		}
		result.PoolCache[poolID] = set
	}
	return result.PoolCache[poolID][strategyNormalizeSymbol(symbol)]
}

func formulaResponseMaps(data interface{}) (map[string]bool, map[string]any) {
	raw, _ := json.Marshal(data)
	decoded := map[string]interface{}{}
	_ = json.Unmarshal(raw, &decoded)
	hits := map[string]bool{}
	details := map[string]any{}
	for symbol, detail := range decoded {
		_, hit := formulaDetailHit(detail)
		normalized := strategyNormalizeSymbol(symbol)
		hits[normalized] = hit
		details[normalized] = detail
	}
	return hits, details
}

func strategyItemsToSelectionResults(items []StrategySelectionItem) []SelectionResult {
	results := make([]SelectionResult, 0, len(items))
	for _, item := range items {
		results = append(results, SelectionResult{
			Symbol:         item.Symbol,
			Latest:         item.Latest,
			SignalAt:       item.SignalAt,
			SignalPrice:    item.SignalPrice,
			QuoteAt:        item.QuoteAt,
			QuoteSource:    item.QuoteSource,
			PriceType:      item.PriceType,
			QuoteAgeMS:     item.QuoteAgeMS,
			SignalScore:    item.Score,
			SnapshotStatus: item.SnapshotStatus,
			DetailJSON:     mustJSON(item),
			CreatedAt:      nowText(),
		})
	}
	return results
}

func latest(rows []FormulaKline) FormulaKline {
	if len(rows) == 0 {
		return FormulaKline{}
	}
	return rows[len(rows)-1]
}

func ma(rows []FormulaKline, days int) float64 {
	if days <= 0 || len(rows) < days {
		return 0
	}
	sum := 0.0
	for _, row := range rows[len(rows)-days:] {
		sum += row.Close
	}
	return sum / float64(days)
}

func strategyAvgVol(rows []FormulaKline, days int) float64 {
	if days <= 0 || len(rows) < days+1 {
		return 0
	}
	end := len(rows) - 1
	start := end - days
	sum := 0.0
	for _, row := range rows[start:end] {
		sum += row.Vol
	}
	return sum / float64(days)
}

func strategyNormalizeSymbol(symbol string) string {
	items := normalizeSymbols([]string{symbol})
	if len(items) == 0 {
		return strings.TrimSpace(strings.ToUpper(symbol))
	}
	return items[0]
}

func highestHigh(rows []FormulaKline, days int) float64 {
	if days <= 0 || len(rows) < days+1 {
		return 0
	}
	end := len(rows) - 1
	start := end - days
	high := 0.0
	for _, row := range rows[start:end] {
		if row.High > high {
			high = row.High
		}
	}
	return high
}

func stringParam(params map[string]interface{}, key string, fallback string) string {
	if params == nil {
		return fallback
	}
	if value, ok := params[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func floatParam(params map[string]interface{}, key string, fallback float64) float64 {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		if n, err := value.Float64(); err == nil {
			return n
		}
	}
	return fallback
}

func intParam(params map[string]interface{}, key string, fallback int) int {
	return int(floatParam(params, key, float64(fallback)))
}

func chooseString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func chooseInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
