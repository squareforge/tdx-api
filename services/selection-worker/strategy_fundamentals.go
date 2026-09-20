package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/tdx"
)

const (
	historyFinanceProfitGrowthField = 191
	historyFinanceTotalSharesField  = 238
	historyFinanceNetProfitTTMField = 276
	historyFinanceROEField          = 281
)

type strategyFinancialRecord struct {
	PeriodDate    int
	AvailableDate int
	TotalShares   float64
	NetProfitTTM  float64
	ProfitGrowth  float64
	ROE           float64
}

type strategyWeightRecord struct {
	Date              int
	TotalShares       float64
	CashDividendPer10 float64
}

type strategyFundamentalData struct {
	Financials map[string][]strategyFinancialRecord
	Weights    map[string][]strategyWeightRecord
}

type strategyFundamentalMetrics struct {
	MarketCapYi  float64
	DividendRate float64
	PE           float64
	PEG          float64
	ProfitGrowth float64
	ROE          float64
	AvailableAt  int
}

func strategyUsesFundamentals(config StrategyConfig) bool {
	for _, rule := range append(append([]StrategyFactorRule{}, config.Filters...), config.Scores...) {
		switch rule.Factor {
		case "market_cap_range", "market_cap_ascending", "dividend_yield_percentile", "peg_range", "raw_price_range":
			return true
		}
	}
	return false
}

func historicalPlansUseFundamentals(plans []historicalStrategyPlan) bool {
	for _, plan := range plans {
		if strategyUsesFundamentals(plan.config) {
			return true
		}
	}
	return false
}

func hikyuuStockDatabasePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("HIKYUU_STOCK_DB")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("HIKYUU_STOCK_DB不可用: %w", err)
		}
		return configured, nil
	}
	candidates := []string{
		"/app/data/hikyuu/stocks/stock.db",
		filepath.Join("data", "hikyuu", "stocks", "stock.db"),
		filepath.Join("..", "..", "data", "hikyuu", "stocks", "stock.db"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("未找到Hikyuu历史财务库stock.db")
}

func loadHikyuuStockCodeModels(ctx context.Context) ([]*tdx.CodeModel, error) {
	path, err := hikyuuStockDatabasePath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	rows, err := db.QueryContext(ctx, `SELECT s.code,s.name,LOWER(m.market)
		FROM Stock s JOIN Market m ON m.marketid=s.marketid
		WHERE s.valid=1 AND s.type IN (1,8,9,11) AND LOWER(m.market) IN ('sh','sz','bj')
		ORDER BY s.code`)
	if err != nil {
		return nil, fmt.Errorf("读取Hikyuu股票代码失败: %w", err)
	}
	defer rows.Close()
	models := make([]*tdx.CodeModel, 0, 4000)
	for rows.Next() {
		var code, name, exchange string
		if err := rows.Scan(&code, &name, &exchange); err != nil {
			return nil, err
		}
		models = append(models, &tdx.CodeModel{Code: code, Name: name, Exchange: exchange})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("Hikyuu股票代码表为空")
	}
	return models, nil
}

func loadStrategyFundamentalData(ctx context.Context, symbols []string, startDate, endDate int) (*strategyFundamentalData, error) {
	path, err := hikyuuStockDatabasePath()
	if err != nil {
		return nil, err
	}
	if endDate <= 0 {
		endDate = dateInt(time.Now())
	}
	if startDate <= 0 || startDate > endDate {
		startDate = endDate
	}
	// Two years of disclosed reports provides a point-in-time fallback for
	// sparse reporters and covers the one-year dividend window.
	lowerDate := shiftStrategyDate(startDate, -800)
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		return nil, err
	}

	data := &strategyFundamentalData{
		Financials: map[string][]strategyFinancialRecord{},
		Weights:    map[string][]strategyWeightRecord{},
	}
	for _, batch := range chunkSymbols(normalizeSymbols(symbols), 300) {
		if err := loadFinancialBatch(ctx, db, data, batch, lowerDate, endDate); err != nil {
			return nil, err
		}
		if err := loadWeightBatch(ctx, db, data, batch, lowerDate, endDate); err != nil {
			return nil, err
		}
	}
	for symbol := range data.Financials {
		sort.Slice(data.Financials[symbol], func(i, j int) bool {
			left, right := data.Financials[symbol][i], data.Financials[symbol][j]
			if left.AvailableDate == right.AvailableDate {
				return left.PeriodDate < right.PeriodDate
			}
			return left.AvailableDate < right.AvailableDate
		})
	}
	for symbol := range data.Weights {
		sort.Slice(data.Weights[symbol], func(i, j int) bool {
			return data.Weights[symbol][i].Date < data.Weights[symbol][j].Date
		})
	}
	return data, nil
}

func loadFinancialBatch(ctx context.Context, db *sql.DB, data *strategyFundamentalData, symbols []string, lowerDate, endDate int) error {
	if len(symbols) == 0 {
		return nil
	}
	marketCodes := make([]string, 0, len(symbols))
	args := []any{lowerDate, endDate}
	for _, symbol := range symbols {
		marketCodes = append(marketCodes, strategyMarketCode(symbol))
		args = append(args, strategyMarketCode(symbol))
	}
	query := `SELECT market_code,file_date,report_date,"values" FROM HistoryFinance
		WHERE report_date BETWEEN ? AND ? AND market_code IN (` + sqlPlaceholders(len(marketCodes)) + `)
		ORDER BY market_code,report_date,file_date`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("读取历史财务失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var marketCode string
		var periodDate, availableDate int
		var values []byte
		if err := rows.Scan(&marketCode, &periodDate, &availableDate, &values); err != nil {
			return err
		}
		symbol := strategyNormalizeSymbol(marketCode)
		data.Financials[symbol] = append(data.Financials[symbol], strategyFinancialRecord{
			PeriodDate: periodDate, AvailableDate: availableDate,
			TotalShares:  historyFinanceFloat(values, historyFinanceTotalSharesField),
			NetProfitTTM: historyFinanceFloat(values, historyFinanceNetProfitTTMField),
			ProfitGrowth: historyFinanceFloat(values, historyFinanceProfitGrowthField),
			ROE:          historyFinanceFloat(values, historyFinanceROEField),
		})
	}
	return rows.Err()
}

func loadWeightBatch(ctx context.Context, db *sql.DB, data *strategyFundamentalData, symbols []string, lowerDate, endDate int) error {
	if len(symbols) == 0 {
		return nil
	}
	args := []any{lowerDate, endDate}
	for _, symbol := range symbols {
		args = append(args, symbol)
	}
	query := `SELECT s.code,w.date,w.totalCount,w.bonus
		FROM stkWeight w JOIN Stock s ON s.stockid=w.stockid
		WHERE w.date BETWEEN ? AND ? AND s.code IN (` + sqlPlaceholders(len(symbols)) + `)
		ORDER BY s.code,w.date`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("读取历史股本与分红失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var symbol string
		var date int
		var totalCount, bonus float64
		if err := rows.Scan(&symbol, &date, &totalCount, &bonus); err != nil {
			return err
		}
		data.Weights[strategyNormalizeSymbol(symbol)] = append(data.Weights[strategyNormalizeSymbol(symbol)], strategyWeightRecord{
			Date: date, TotalShares: totalCount * 10000,
			// Hikyuu stores the SQLite integer at 1/1000 of its exposed value;
			// the exposed bonus is cash yuan per 10 shares.
			CashDividendPer10: bonus / 1000,
		})
	}
	return rows.Err()
}

func (data *strategyFundamentalData) metrics(symbol string, date int, rawClose float64) (strategyFundamentalMetrics, bool) {
	if data == nil || date <= 0 || rawClose <= 0 {
		return strategyFundamentalMetrics{}, false
	}
	symbol = strategyNormalizeSymbol(symbol)
	financial, hasFinancial := latestFinancialRecord(data.Financials[symbol], date)
	totalShares := financial.TotalShares
	for _, item := range data.Weights[symbol] {
		if item.Date > date {
			break
		}
		if item.TotalShares > 0 {
			totalShares = item.TotalShares
		}
	}
	if totalShares <= 0 {
		return strategyFundamentalMetrics{}, false
	}
	marketCap := rawClose * totalShares
	metrics := strategyFundamentalMetrics{
		MarketCapYi: marketCap / 1e8, AvailableAt: financial.AvailableDate,
		ProfitGrowth: financial.ProfitGrowth, ROE: financial.ROE,
	}
	trailingStart := shiftStrategyDate(date, -365)
	dividendPerShare := 0.0
	for _, item := range data.Weights[symbol] {
		if item.Date > date {
			break
		}
		if item.Date > trailingStart && item.CashDividendPer10 > 0 {
			dividendPerShare += item.CashDividendPer10 / 10
		}
	}
	metrics.DividendRate = dividendPerShare * 100 / rawClose
	if hasFinancial && financial.NetProfitTTM != 0 {
		metrics.PE = marketCap / financial.NetProfitTTM
		if financial.ProfitGrowth != 0 {
			metrics.PEG = metrics.PE / financial.ProfitGrowth
		}
	}
	return metrics, true
}

func latestFinancialRecord(records []strategyFinancialRecord, date int) (strategyFinancialRecord, bool) {
	index := sort.Search(len(records), func(i int) bool { return records[i].AvailableDate > date })
	if index == 0 {
		return strategyFinancialRecord{}, false
	}
	return records[index-1], true
}

func historyFinanceFloat(values []byte, fieldID int) float64 {
	offset := (fieldID - 1) * 4
	if fieldID <= 0 || offset+4 > len(values) {
		return 0
	}
	value := float64(math.Float32frombits(binary.LittleEndian.Uint32(values[offset : offset+4])))
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func strategyMarketCode(symbol string) string {
	symbol = strategyNormalizeSymbol(symbol)
	prefix := "SZ"
	if strings.HasPrefix(symbol, "4") || strings.HasPrefix(symbol, "8") || strings.HasPrefix(symbol, "92") {
		prefix = "BJ"
	} else if strings.HasPrefix(symbol, "6") || strings.HasPrefix(symbol, "5") || strings.HasPrefix(symbol, "9") {
		prefix = "SH"
	}
	return prefix + symbol
}

func sqlPlaceholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func shiftStrategyDate(date, days int) int {
	if date <= 0 {
		return 0
	}
	value := time.Date(date/10000, time.Month((date/100)%100), date%100, 0, 0, 0, 0, time.Local)
	return dateInt(value.AddDate(0, 0, days))
}

func (r *AutomationRunner) prepareStrategyFundamentalContext(ctx context.Context, result *StrategyRunResult, symbols []string, startDate, endDate int) error {
	if result == nil || !strategyUsesFundamentals(result.Config) {
		return nil
	}
	rawKlines, rawErrors := r.loadSystemBatchKlinesWithRecover(ctx, symbols, result.Config.CalcCount, "none")
	if len(rawKlines) == 0 {
		return fmt.Errorf("基本面策略原始价格加载失败: %s", firstStrategyLoadError(rawErrors))
	}
	data, err := loadStrategyFundamentalData(ctx, symbols, startDate, endDate)
	if err != nil {
		return err
	}
	result.RawKlineCache = rawKlines
	result.FundamentalData = data
	result.FundamentalRanks = map[string]map[string]float64{}
	return nil
}

func evaluateFundamentalStrategyFactor(result *StrategyRunResult, symbol string, rows []FormulaKline, rule StrategyFactorRule, filter bool) (bool, float64, any, string) {
	if result == nil || result.FundamentalData == nil || len(rows) == 0 {
		return false, 0, nil, "点时基本面数据未加载"
	}
	date := latest(rows).Date
	rawClose, ok := strategyRawClose(result.RawKlineCache[symbol], date)
	if !ok {
		return false, 0, nil, "信号日原始收盘价不可用"
	}
	metrics, ok := result.FundamentalData.metrics(symbol, date, rawClose)
	if !ok {
		return false, 0, nil, "信号日股本或财务数据不可用"
	}

	switch rule.Factor {
	case "raw_price_range":
		minimum := floatParam(rule.Params, "min", 0)
		maximum := floatParam(rule.Params, "max", math.MaxFloat64)
		hit := rawClose >= minimum && rawClose <= maximum
		return hit, 1, rawClose, fmt.Sprintf("原始收盘价 %.2f 在 %.2f-%.2f", rawClose, minimum, maximum)
	case "market_cap_range":
		minimum := floatParam(rule.Params, "min_yi", 0)
		maximum := floatParam(rule.Params, "max_yi", math.MaxFloat64)
		hit := metrics.MarketCapYi >= minimum && metrics.MarketCapYi <= maximum
		strength := 1.0
		if !filter && maximum > minimum && maximum < math.MaxFloat64 {
			strength = clamp01(1 - (metrics.MarketCapYi-minimum)/(maximum-minimum))
		}
		return hit, strength, metrics.MarketCapYi, fmt.Sprintf("点时总市值 %.2f亿 在 %.2f-%.2f亿", metrics.MarketCapYi, minimum, maximum)
	case "market_cap_ascending":
		rank := fundamentalCrossSectionRank(result, date, "market_cap", symbol)
		if rank < 0 {
			return false, 0, metrics.MarketCapYi, "点时市值横截面排名不可用"
		}
		strength := 1 - rank
		return true, strength, map[string]float64{"market_cap_yi": metrics.MarketCapYi, "ascending_percentile": rank * 100}, fmt.Sprintf("总市值 %.2f亿，升序分位 %.1f%%", metrics.MarketCapYi, rank*100)
	case "dividend_yield_percentile":
		minimumPercentile := floatParam(rule.Params, "min_percentile", 75) / 100
		minimumYield := floatParam(rule.Params, "min_yield", 0)
		rank := fundamentalCrossSectionRank(result, date, "dividend_yield", symbol)
		if rank < 0 {
			return false, 0, metrics.DividendRate, "近一年股息率横截面排名不可用"
		}
		hit := metrics.DividendRate > minimumYield && rank >= minimumPercentile
		return hit, rank, map[string]float64{"dividend_yield": metrics.DividendRate, "percentile": rank * 100}, fmt.Sprintf("近一年股息率 %.2f%%，市场分位 %.1f%%", metrics.DividendRate, rank*100)
	case "peg_range":
		minimum := floatParam(rule.Params, "min", -1)
		maximum := floatParam(rule.Params, "max", 3)
		if metrics.AvailableAt <= 0 || metrics.PE == 0 || metrics.ProfitGrowth == 0 {
			return false, 0, nil, "信号日前没有可用的PE/净利润增速"
		}
		hit := metrics.PEG > minimum && metrics.PEG < maximum
		strength := intervalStrength(metrics.PEG, maximum*0.15, maximum*0.5)
		value := map[string]float64{"pe": metrics.PE, "profit_growth": metrics.ProfitGrowth, "peg": metrics.PEG, "roe": metrics.ROE}
		return hit, strength, value, fmt.Sprintf("点时PEG %.2f 在 %.2f-%.2f，财报披露日%d", metrics.PEG, minimum, maximum, metrics.AvailableAt)
	default:
		return false, 0, nil, "未知点时基本面因子: " + rule.Factor
	}
}

func fundamentalCrossSectionRank(result *StrategyRunResult, date int, metric, target string) float64 {
	if result.FundamentalRanks == nil {
		result.FundamentalRanks = map[string]map[string]float64{}
	}
	key := fmt.Sprintf("%s:%d", metric, date)
	if cached, ok := result.FundamentalRanks[key]; ok {
		if value, exists := cached[strategyNormalizeSymbol(target)]; exists {
			return value
		}
		return -1
	}
	type metricItem struct {
		symbol string
		value  float64
	}
	items := make([]metricItem, 0, len(result.KlineCache))
	for symbol, rows := range result.KlineCache {
		if symbol == strategyBenchmarkSymbol || len(rows) == 0 || latest(rows).Date != date {
			continue
		}
		rawClose, ok := strategyRawClose(result.RawKlineCache[symbol], date)
		if !ok {
			continue
		}
		values, ok := result.FundamentalData.metrics(symbol, date, rawClose)
		if !ok {
			continue
		}
		value := values.MarketCapYi
		if metric == "dividend_yield" {
			value = values.DividendRate
			if value <= 0 {
				continue
			}
		}
		if value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			items = append(items, metricItem{symbol: strategyNormalizeSymbol(symbol), value: value})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].value == items[j].value {
			return items[i].symbol < items[j].symbol
		}
		return items[i].value < items[j].value
	})
	ranks := map[string]float64{}
	denominator := math.Max(float64(len(items)-1), 1)
	for index, item := range items {
		ranks[item.symbol] = float64(index) / denominator
	}
	result.FundamentalRanks[key] = ranks
	if value, ok := ranks[strategyNormalizeSymbol(target)]; ok {
		return value
	}
	return -1
}

func strategyRawClose(rows []FormulaKline, date int) (float64, bool) {
	index := sort.Search(len(rows), func(i int) bool { return rows[i].Date >= date })
	if index >= len(rows) || rows[index].Date != date || rows[index].Close <= 0 {
		return 0, false
	}
	return rows[index].Close, true
}

func firstStrategyLoadError(loadErrors map[string]string) string {
	for _, message := range loadErrors {
		if strings.TrimSpace(message) != "" {
			return message
		}
	}
	return "行情服务未返回数据"
}
