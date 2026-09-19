package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type StrategyConfig struct {
	Universe        json.RawMessage         `json:"universe"`
	PoolID          string                  `json:"pool_id"`
	Symbols         []string                `json:"symbols"`
	Filters         []StrategyFactorRule    `json:"filters"`
	Scores          []StrategyFactorRule    `json:"scores"`
	Pass            StrategyPassConfig      `json:"pass"`
	Period          string                  `json:"period"`
	Right           int                     `json:"right"`
	CalcCount       int                     `json:"calc_count"`
	BatchSize       int                     `json:"batch_size"`
	ScanLimit       *int                    `json:"scan_limit,omitempty"`
	ContinueOnError bool                    `json:"continue_on_error"`
	Execution       StrategyExecutionConfig `json:"execution"`
}

// StrategyExecutionConfig describes constraints that can only be evaluated
// after a signal has been produced. Keeping these out of factor evaluation
// prevents next-session prices from leaking into the signal day.
type StrategyExecutionConfig struct {
	MinOpenGap *float64 `json:"min_open_gap,omitempty"`
	MaxOpenGap *float64 `json:"max_open_gap,omitempty"`
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
	FormulaCache  map[string]map[string]bool `json:"-"`
	FormulaDetail map[string]map[string]any  `json:"-"`
	KlineCache    map[string][]FormulaKline  `json:"-"`
	PoolCache     map[string]map[string]bool `json:"-"`
	FormulaByName map[string]Formula         `json:"-"`
	FormulaByID   map[string]Formula         `json:"-"`
	Config        StrategyConfig             `json:"config"`
}

type StrategySelectionItem struct {
	Symbol        string                 `json:"symbol"`
	Score         float64                `json:"score"`
	Hit           bool                   `json:"hit"`
	Latest        float64                `json:"latest"`
	Reasons       []string               `json:"reasons"`
	FactorResults []StrategyFactorResult `json:"factor_results"`
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
	symbols, err := r.strategyUniverse(cfg)
	if err != nil {
		return StrategyRunResult{}, err
	}
	if len(symbols) == 0 {
		return StrategyRunResult{}, errors.New("策略股票范围为空")
	}

	result := StrategyRunResult{
		Strategy:      strategy,
		Config:        cfg,
		Total:         len(symbols),
		Errors:        map[string]string{},
		FormulaCache:  map[string]map[string]bool{},
		FormulaDetail: map[string]map[string]any{},
		KlineCache:    map[string][]FormulaKline{},
		PoolCache:     map[string]map[string]bool{},
		FormulaByName: map[string]Formula{},
		FormulaByID:   map[string]Formula{},
	}
	// 批量加载候选股票 K 线并盘中补K，与系统策略日报路径保持一致，
	// 避免逐只串行拉取导致全市场策略超时。
	loadSymbols := append([]string{}, symbols...)
	if strategyUsesMarketContext(cfg) {
		loadSymbols = append(loadSymbols, strategyBenchmarkSymbol)
	}
	klines, loadErrors := r.loadSystemBatchKlines(ctx, loadSymbols, cfg.CalcCount)
	for symbol, reason := range r.patchIntradayKlines(ctx, klines, time.Now()) {
		loadErrors[symbol] = reason
	}
	result.KlineCache = klines
	for symbol, reason := range loadErrors {
		if symbol == strategyBenchmarkSymbol {
			continue
		}
		result.Errors[symbol] = reason
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
		item, ok := r.evaluateStrategySymbol(ctx, strategy, &result, symbol)
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
	if marketClient == nil || !marketClient.Enabled() {
		return errors.New("市场状态因子需要行情服务")
	}
	resp, err := marketClient.IndexKline(ctx, strategyBenchmarkSymbol, formulaPeriodToKlineType(result.Config.Period), result.Config.CalcCount)
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
	var legacy string
	if len(cfg.Universe) > 0 && json.Unmarshal(cfg.Universe, &legacy) != nil {
		return r.evaluateStrategyUniverseExpression(cfg)
	}
	return r.evaluateLegacyStrategyUniverse(cfg, legacy)
}

func (r *AutomationRunner) evaluateLegacyStrategyUniverse(cfg StrategyConfig, universe string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(universe)) {
	case "symbols":
		return normalizeSymbols(cfg.Symbols), nil
	case "all_a", "all":
		if len(cfg.Symbols) > 0 {
			return normalizeSymbols(cfg.Symbols), nil
		}
		if symbols := limitedMarketPoolSymbols("market-all-a", strategyMaxCodes(cfg)); len(symbols) > 0 {
			return symbols, nil
		}
		return nil, errors.New("全市场代码列表不可用")
	case "market":
		poolID := cfg.PoolID
		if poolID == "" {
			poolID = "market-all-a"
		}
		if symbols := limitedMarketPoolSymbols(poolID, strategyMaxCodes(cfg)); len(symbols) > 0 {
			return symbols, nil
		}
		return nil, fmt.Errorf("市场分组代码列表不可用: %s", poolID)
	case "", "pool":
		poolID := cfg.PoolID
		if poolID == "" {
			poolID = DecisionWatchPoolID
		}
		pool, err := r.store.GetStockPool(poolID)
		if err != nil {
			return nil, err
		}
		return normalizeSymbols(pool.Symbols), nil
	default:
		return nil, fmt.Errorf("未知策略股票范围: %s", cfg.Universe)
	}
}

func strategyMaxCodes(cfg StrategyConfig) int {
	if cfg.ScanLimit != nil && *cfg.ScanLimit >= 0 {
		return *cfg.ScanLimit
	}
	maxCodes := 300
	if cfg.BatchSize > 0 && cfg.BatchSize > maxCodes {
		maxCodes = cfg.BatchSize
	}
	return maxCodes
}

type strategyUniverseExpression struct {
	Include   []strategyUniverseTerm `json:"include"`
	Intersect []strategyUniverseTerm `json:"intersect"`
	Exclude   []strategyUniverseTerm `json:"exclude"`
}

type strategyUniverseTerm struct {
	Pool    string   `json:"pool"`
	PoolID  string   `json:"pool_id"`
	Symbols []string `json:"symbols"`
}

func (r *AutomationRunner) evaluateStrategyUniverseExpression(cfg StrategyConfig) ([]string, error) {
	var expression strategyUniverseExpression
	if err := json.Unmarshal(cfg.Universe, &expression); err != nil {
		return nil, fmt.Errorf("策略候选范围解析失败: %w", err)
	}
	if len(expression.Include) == 0 {
		return nil, errors.New("策略候选范围缺少起点池")
	}
	resolve := func(term strategyUniverseTerm) (map[string]bool, error) {
		set := map[string]bool{}
		for _, symbol := range normalizeSymbols(term.Symbols) {
			set[symbol] = true
		}
		poolID := strings.TrimSpace(term.Pool)
		if poolID == "" {
			poolID = strings.TrimSpace(term.PoolID)
		}
		if poolID == "" {
			if len(set) == 0 {
				return nil, errors.New("候选范围包含空股票池")
			}
			return set, nil
		}
		pool, err := r.store.GetStockPool(poolID)
		if err != nil {
			return nil, fmt.Errorf("候选范围股票池 %s 不可用: %w", poolID, err)
		}
		for _, symbol := range normalizeSymbols(pool.Symbols) {
			set[symbol] = true
		}
		return set, nil
	}

	selected := map[string]bool{}
	for _, term := range expression.Include {
		set, err := resolve(term)
		if err != nil {
			return nil, err
		}
		for symbol := range set {
			selected[symbol] = true
		}
	}
	for _, term := range expression.Intersect {
		set, err := resolve(term)
		if err != nil {
			return nil, err
		}
		for symbol := range selected {
			if !set[symbol] {
				delete(selected, symbol)
			}
		}
	}
	for _, term := range expression.Exclude {
		set, err := resolve(term)
		if err != nil {
			return nil, err
		}
		for symbol := range set {
			delete(selected, symbol)
		}
	}
	symbols := make([]string, 0, len(selected))
	for symbol := range selected {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	if limit := strategyMaxCodes(cfg); limit > 0 && len(symbols) > limit {
		symbols = symbols[:limit]
	}
	return symbols, nil
}

func (r *AutomationRunner) prepareStrategyFormulas(ctx context.Context, result *StrategyRunResult, symbols []string) error {
	return r.prepareStrategyFormulasWithKlines(ctx, result, symbols, nil)
}

// prepareStrategyFormulasWithKlines passes preloaded data to the formula worker
// in bounded batches. The daily system batch uses this to avoid a second K-line
// fetch for formula factors.
func (r *AutomationRunner) prepareStrategyFormulasWithKlines(ctx context.Context, result *StrategyRunResult, symbols []string, klines map[string][]FormulaKline) error {
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
			data := formulaKlineBatch(klines, batch)
			resp, err := r.worker.Run(ctx, FormulaRunRequest{
				Symbols:       batch,
				Script:        formula.Script,
				Args:          json.RawMessage(formula.ArgsJSON),
				Period:        chooseString(result.Config.Period, formula.Period),
				Right:         chooseInt(result.Config.Right, formula.Right),
				OutCount:      1,
				CalcCount:     result.Config.CalcCount,
				ForceFallback: true,
				Data:          data,
			})
			if err != nil {
				return err
			}
			mergeFormulaData(allData, resp.Data)
		}
		hits, details := formulaResponseMaps(allData)
		result.FormulaCache[formula.ID] = hits
		result.FormulaDetail[formula.ID] = details
	}
	return nil
}

func formulaKlineBatch(klines map[string][]FormulaKline, symbols []string) map[string][]FormulaKline {
	if len(klines) == 0 {
		return nil
	}
	data := make(map[string][]FormulaKline, len(symbols))
	for _, symbol := range symbols {
		if rows, ok := klines[symbol]; ok {
			data[symbol] = rows
		}
	}
	return data
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

func (r *AutomationRunner) evaluateStrategySymbol(ctx context.Context, strategy Strategy, result *StrategyRunResult, symbol string) (StrategySelectionItem, bool) {
	item := StrategySelectionItem{Symbol: symbol, Hit: true}
	rows, err := r.strategyKline(ctx, result, symbol)
	if err != nil {
		if _, exists := result.Errors[symbol]; !exists {
			result.Errors[symbol] = err.Error()
		}
		return item, false
	}
	if len(rows) > 0 {
		item.Latest = rows[len(rows)-1].Close
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
		return item, false
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
		return item, false
	}
	return item, true
}

func (r *AutomationRunner) strategyKline(ctx context.Context, result *StrategyRunResult, symbol string) ([]FormulaKline, error) {
	if rows, ok := result.KlineCache[symbol]; ok && len(rows) > 0 {
		return rows, nil
	}
	if reason, ok := result.Errors[symbol]; ok {
		return nil, errors.New(reason)
	}
	return nil, fmt.Errorf("K线未加载: %s", symbol)
}

func (r *AutomationRunner) evaluateFactor(result *StrategyRunResult, symbol string, rows []FormulaKline, rule StrategyFactorRule, filter bool) StrategyFactorResult {
	fr := StrategyFactorResult{ID: rule.ID, Factor: rule.Factor}
	weight := rule.Weight
	if filter {
		weight = 0
	}
	// Factors return a normalized strength in [0,1]. Score rules then apply
	// their configured weight, while filter rules only use the hit flag.
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
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			closePrice := latest(rows).Close
			fr.Score = clamp01(raw / math.Max(closePrice, 0.01) * 100 / 0.3)
		}
	case "macd_dead_cross":
		hit, raw, reason := evaluateMACDSignal(rows, rule, false)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			closePrice := latest(rows).Close
			fr.Score = clamp01(raw / math.Max(closePrice, 0.01) * 100 / 0.3)
		}
	case "kdj_golden_cross":
		hit, raw, reason := evaluateKDJGoldenCross(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			fr.Score = clamp01((100 - raw) / 100)
		}
	case "rsi_oversold":
		hit, raw, reason := evaluateRSIOversold(rows, rule)
		threshold := floatParam(rule.Params, "threshold", 30)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			fr.Score = clamp01((threshold - raw) / math.Max(threshold, 0.01))
		}
	case "boll_breakout":
		hit, raw, reason := evaluateBOLLBreakout(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			closePrice := latest(rows).Close
			fr.Score = clamp01(raw / math.Max(closePrice, 0.01) / 0.05)
		}
	case "volume_breakout":
		hit, raw, reason := evaluateVolumeBreakout(rows, rule)
		ratio := floatParam(rule.Params, "ratio", 1.5)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			fr.Score = clamp01((raw - ratio) / (ratio * 0.5))
		}
	case "local_rocket":
		hit, raw, reason := evaluateLocalRocket(rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, raw
		if hit {
			fr.Score = clamp01(raw / 12)
		}
	case "low_three_bull_first_board":
		hit, strength, value, reason := evaluateLowThreeBullFirstBoard(symbol, rows, rule)
		fr.Hit, fr.Reason, fr.Value = hit, reason, value
		if hit {
			fr.Score = strength
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
			Symbol:     item.Symbol,
			Latest:     item.Latest,
			DetailJSON: mustJSON(item),
			CreatedAt:  nowText(),
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
