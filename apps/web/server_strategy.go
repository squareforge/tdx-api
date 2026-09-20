package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

type StrategyFactorDefinition struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Kind        string                   `json:"kind"`
	Description string                   `json:"description"`
	Params      []StrategyFactorParamDef `json:"params"`
}

type StrategyFactorParamDef struct {
	Name    string      `json:"name"`
	Label   string      `json:"label"`
	Type    string      `json:"type"`
	Default interface{} `json:"default"`
}

func handleStrategies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := appStore.ListStrategies()
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, items)
	case http.MethodPost:
		var req Strategy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		item, err := appStore.UpsertStrategy(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleStrategyOperations(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path, "/api/strategies/")
	if len(parts) == 0 {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if id == "range-preview" {
		handleStrategyRangePreview(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "clone" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		item, err := appStore.CloneStrategy(id)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
		return
	}
	if len(parts) == 2 && parts[1] == "run" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		proxyToService("SELECTION_WORKER_URL", func(w http.ResponseWriter, r *http.Request) {
			handleStrategyRun(w, r, id)
		})(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "apply-universe" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		handleStrategyApplyUniverse(w, r, id)
		return
	}
	if len(parts) == 2 && parts[1] == "backtest" {
		if r.Method != http.MethodPost {
			errorResponse(w, "只支持POST请求")
			return
		}
		handleStrategyBacktest(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := appStore.GetStrategy(id)
		if err != nil {
			errorResponse(w, notFoundMessage(err, "策略不存在"))
			return
		}
		successResponse(w, item)
	case http.MethodPut:
		var req Strategy
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, "请求参数错误: "+err.Error())
			return
		}
		req.ID = id
		item, err := appStore.UpsertStrategy(req)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, item)
	case http.MethodDelete:
		if err := appStore.DeleteStrategy(id); err != nil {
			errorResponse(w, err.Error())
			return
		}
		successResponse(w, map[string]string{"id": id})
	default:
		errorResponse(w, "不支持的请求方法")
	}
}

func handleStrategyRangePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req struct {
		StrategyConfig
		MaxCodes *int `json:"max_codes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	maxCodes := 300
	if req.MaxCodes != nil && *req.MaxCodes >= 0 {
		maxCodes = *req.MaxCodes
	}
	result, err := automationRunner.strategyUniverseResult(req.StrategyConfig, maxCodes)
	if err != nil {
		errorResponse(w, "选股范围预览失败: "+err.Error())
		return
	}
	successResponse(w, result)
}

// handleStrategyApplyUniverse merges a range expression into a strategy's config_json,
// replacing only the universe field while keeping filters/scores/pass untouched.
func handleStrategyApplyUniverse(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Universe StrategyUniverseExpression `json:"universe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	if !req.Universe.isExpression() {
		errorResponse(w, "选股范围表达式为空，请先设置起点池")
		return
	}
	strategy, err := appStore.GetStrategy(id)
	if err != nil {
		errorResponse(w, notFoundMessage(err, "策略不存在"))
		return
	}
	var cfg StrategyConfig
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &cfg); err != nil {
		errorResponse(w, "策略配置解析失败: "+err.Error())
		return
	}
	cfg.Universe = req.Universe
	// 先跑一次范围求值做校验:引用的池必须都存在,避免保存后运行时才报错。
	if _, err := automationRunner.strategyUniverseResult(cfg, strategyMaxCodes(cfg)); err != nil {
		errorResponse(w, "选股范围校验失败: "+err.Error())
		return
	}
	// Preserve every field other than universe. This also prevents this focused
	// endpoint from becoming a back door for editing a system template's rules.
	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &rawConfig); err != nil {
		errorResponse(w, "策略配置解析失败: "+err.Error())
		return
	}
	if rawConfig == nil {
		errorResponse(w, "策略配置必须是JSON对象")
		return
	}
	universeRaw, err := json.Marshal(req.Universe)
	if err != nil {
		errorResponse(w, "策略配置生成失败: "+err.Error())
		return
	}
	rawConfig["universe"] = universeRaw
	if strategy.Readonly {
		customizedRaw, _ := json.Marshal(true)
		rawConfig["universe_customized"] = customizedRaw
	}
	raw, err := json.Marshal(rawConfig)
	if err != nil {
		errorResponse(w, "策略配置生成失败: "+err.Error())
		return
	}
	strategy.ConfigJSON = string(raw)
	var item Strategy
	if strategy.Readonly {
		item, err = appStore.SetStrategyUniverseConfig(strategy.ID, strategy.ConfigJSON)
	} else {
		item, err = appStore.UpsertStrategy(strategy)
	}
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, item)
}

func handleStrategyRun(w http.ResponseWriter, r *http.Request, id string) {
	strategy, err := appStore.GetStrategy(id)
	if err != nil {
		errorResponse(w, notFoundMessage(err, "策略不存在"))
		return
	}
	task := AutomationTask{
		ID:          "manual-strategy-" + strategy.ID,
		Name:        strategy.Name,
		Type:        "strategy_selection",
		Cron:        "0 0 0 * * *",
		PayloadJSON: mustJSON(map[string]string{"strategy_id": strategy.ID}),
		WebhookIDs:  "[]",
	}

	runs, err := appStore.ListAutomationRuns(task.ID, 20)
	if err != nil {
		errorResponse(w, "策略运行失败: "+err.Error())
		return
	}
	for _, existing := range runs {
		if existing.Status == "running" {
			successResponse(w, existing)
			return
		}
	}

	run, err := appStore.CreateAutomationRun(task)
	if err != nil {
		errorResponse(w, "策略运行失败: "+err.Error())
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		if _, runErr := automationRunner.runTaskWithRun(ctx, task, run); runErr != nil {
			log.Printf("手动策略运行失败: %s %v", strategy.ID, runErr)
		}
	}()
	successResponse(w, run)
}

func handleStrategyBacktest(w http.ResponseWriter, r *http.Request, id string) {
	strategy, err := appStore.GetStrategy(id)
	if err != nil {
		errorResponse(w, notFoundMessage(err, "策略不存在"))
		return
	}
	var req StrategyBacktestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	req.StrategyID = strategy.ID
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	result, err := automationRunner.runStrategyBacktest(ctx, strategy, req)
	if err != nil {
		errorResponse(w, "策略回测失败: "+err.Error())
		return
	}
	successResponse(w, result)
}

func handleStrategyFactors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	successResponse(w, []StrategyFactorDefinition{
		{
			ID:          "pool_exclude",
			Name:        "排除股票池",
			Kind:        "filter",
			Description: "过滤掉已经放入指定股票池的标的。",
			Params: []StrategyFactorParamDef{
				{Name: "pool_id", Label: "股票池ID", Type: "string", Default: DecisionExcludePoolID},
			},
		},
		{
			ID:          "min_amount",
			Name:        "最低成交额",
			Kind:        "filter",
			Description: "要求最新K线成交额达到指定金额。",
			Params: []StrategyFactorParamDef{
				{Name: "value", Label: "最低成交额", Type: "number", Default: 100000000},
			},
		},
		{
			ID:          "price_range",
			Name:        "价格区间",
			Kind:        "filter",
			Description: "要求最新收盘价落在指定区间。",
			Params: []StrategyFactorParamDef{
				{Name: "min", Label: "最低价", Type: "number", Default: 0},
				{Name: "max", Label: "最高价", Type: "number", Default: 9999},
			},
		},
		{
			ID:          "change_range",
			Name:        "涨跌幅区间",
			Kind:        "filter",
			Description: "要求最新K线涨跌幅落在指定百分比区间。",
			Params: []StrategyFactorParamDef{
				{Name: "min", Label: "最小涨跌幅", Type: "number", Default: -10},
				{Name: "max", Label: "最大涨跌幅", Type: "number", Default: 10},
			},
		},
		{
			ID:          "market_momentum",
			Name:        "大盘趋势门槛",
			Kind:        "filter",
			Description: "要求上证指数截至信号日的N日涨幅落在指定区间，用于限制策略只在匹配的市场环境运行。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "回看天数", Type: "number", Default: 20},
				{Name: "min", Label: "最小涨幅%", Type: "number", Default: 0},
				{Name: "max", Label: "最大涨幅%", Type: "number", Default: 100},
			},
		},
		{
			ID:          "ma_trend",
			Name:        "均线多头",
			Kind:        "score",
			Description: "收盘价、短中长期均线呈多头排列时加分。",
			Params: []StrategyFactorParamDef{
				{Name: "short", Label: "短均线", Type: "number", Default: 5},
				{Name: "mid", Label: "中均线", Type: "number", Default: 10},
				{Name: "long", Label: "长均线", Type: "number", Default: 20},
			},
		},
		{
			ID:          "ma_slope",
			Name:        "均线斜率",
			Kind:        "score",
			Description: "比较当前均线与若干交易日前的同周期均线，过滤走平、下行或过度陡峭的趋势。",
			Params: []StrategyFactorParamDef{
				{Name: "period", Label: "均线周期", Type: "number", Default: 20},
				{Name: "lookback", Label: "比较天数", Type: "number", Default: 5},
				{Name: "min", Label: "最小升幅%", Type: "number", Default: 0},
				{Name: "max", Label: "最大升幅%", Type: "number", Default: 20},
			},
		},
		{
			ID:          "close_strength",
			Name:        "收盘质量",
			Kind:        "filter",
			Description: "要求收盘位于当日振幅的较高区域，并限制上影线，过滤冲高回落。",
			Params: []StrategyFactorParamDef{
				{Name: "min_position", Label: "最低收盘位置", Type: "number", Default: 0.65},
				{Name: "max_upper_shadow", Label: "最大上影%", Type: "number", Default: 3},
			},
		},
		{
			ID:          "volume_up",
			Name:        "阶段放量",
			Kind:        "score",
			Description: "最新成交量达到前N日均量的一定倍数时加分。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "对比天数", Type: "number", Default: 5},
				{Name: "ratio", Label: "放量倍数", Type: "number", Default: 1.3},
			},
		},
		{
			ID:          "break_high",
			Name:        "突破新高",
			Kind:        "score",
			Description: "最新收盘价突破前N日高点时加分。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "回看天数", Type: "number", Default: 20},
			},
		},
		{
			ID: "macd_golden_cross", Name: "MACD金叉", Kind: "score",
			Description: "本地根据收盘价计算MACD，判断DIF向上穿越DEA。",
			Params: []StrategyFactorParamDef{
				{Name: "fast", Label: "快线周期", Type: "number", Default: 12},
				{Name: "slow", Label: "慢线周期", Type: "number", Default: 26},
				{Name: "signal", Label: "信号周期", Type: "number", Default: 9},
			},
		},
		{
			ID: "macd_dead_cross", Name: "MACD死叉", Kind: "score",
			Description: "本地根据收盘价计算MACD，判断DIF向下穿越DEA。",
			Params: []StrategyFactorParamDef{
				{Name: "fast", Label: "快线周期", Type: "number", Default: 12},
				{Name: "slow", Label: "慢线周期", Type: "number", Default: 26},
				{Name: "signal", Label: "信号周期", Type: "number", Default: 9},
			},
		},
		{
			ID: "kdj_golden_cross", Name: "KDJ金叉", Kind: "score",
			Description: "本地根据最高价、最低价和收盘价计算KDJ金叉。",
			Params: []StrategyFactorParamDef{
				{Name: "n", Label: "RSV周期", Type: "number", Default: 9},
				{Name: "k", Label: "K平滑", Type: "number", Default: 3},
				{Name: "d", Label: "D平滑", Type: "number", Default: 3},
			},
		},
		{
			ID: "rsi_oversold", Name: "RSI超卖", Kind: "score",
			Description: "本地计算RSI，最新值低于设定阈值时命中。",
			Params: []StrategyFactorParamDef{
				{Name: "period", Label: "RSI周期", Type: "number", Default: 6},
				{Name: "threshold", Label: "超卖阈值", Type: "number", Default: 30},
			},
		},
		{
			ID: "rsi_rebound", Name: "RSI超卖反转", Kind: "score",
			Description: "近期RSI进入超卖区后回升至指定区间且继续走强时命中。",
			Params: []StrategyFactorParamDef{
				{Name: "period", Label: "RSI周期", Type: "number", Default: 6},
				{Name: "lookback", Label: "超卖回看天数", Type: "number", Default: 5},
				{Name: "oversold", Label: "超卖阈值", Type: "number", Default: 30},
				{Name: "min_current", Label: "当前最低RSI", Type: "number", Default: 35},
				{Name: "max_current", Label: "当前最高RSI", Type: "number", Default: 55},
			},
		},
		{
			ID: "boll_breakout", Name: "BOLL突破", Kind: "score",
			Description: "本地计算布林带，最新收盘价突破上轨时命中。",
			Params: []StrategyFactorParamDef{
				{Name: "period", Label: "BOLL周期", Type: "number", Default: 20},
				{Name: "width", Label: "标准差倍数", Type: "number", Default: 2},
			},
		},
		{
			ID: "volume_breakout", Name: "放量突破", Kind: "score",
			Description: "收盘突破前N日高点，同时成交量达到均量倍数且涨幅达标。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "回看天数", Type: "number", Default: 20},
				{Name: "ratio", Label: "放量倍数", Type: "number", Default: 1.5},
				{Name: "min_change", Label: "最低涨幅", Type: "number", Default: 2},
			},
		},
		{
			ID: "local_rocket", Name: "本地火箭发射", Kind: "score",
			Description: "自定义本地规则：涨幅、放量、突破近期高点并保持均线向上。",
			Params: []StrategyFactorParamDef{
				{Name: "lookback", Label: "突破回看天数", Type: "number", Default: 20},
				{Name: "volume_days", Label: "均量天数", Type: "number", Default: 5},
				{Name: "volume_ratio", Label: "放量倍数", Type: "number", Default: 1.8},
				{Name: "min_change", Label: "最低涨幅", Type: "number", Default: 3},
				{Name: "short_ma", Label: "短均线", Type: "number", Default: 5},
				{Name: "mid_ma", Label: "中均线", Type: "number", Default: 10},
			},
		},
		{
			ID:          "drawdown_from_high",
			Name:        "距高点回撤",
			Kind:        "score",
			Description: "最新收盘价相对前N日高点的回撤幅度，落在期望区间时强度最高，用于捕捉强势股回调。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "回看天数", Type: "number", Default: 60},
				{Name: "min", Label: "最小回撤%", Type: "number", Default: 5},
				{Name: "max", Label: "最大回撤%", Type: "number", Default: 15},
			},
		},
		{
			ID:          "gain_days",
			Name:        "N日涨幅",
			Kind:        "score",
			Description: "最新收盘价相对N日前的涨幅，落在期望区间时强度最高，用于圈定强势股。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "回看天数", Type: "number", Default: 20},
				{Name: "min", Label: "最小涨幅%", Type: "number", Default: 5},
				{Name: "max", Label: "最大涨幅%", Type: "number", Default: 50},
			},
		},
		{
			ID:          "max_amplitude",
			Name:        "最大平均振幅",
			Kind:        "filter",
			Description: "近N日平均振幅(高低价差/昨收)不超过指定百分比，过滤日内波动过大的标的。",
			Params: []StrategyFactorParamDef{
				{Name: "days", Label: "统计天数", Type: "number", Default: 5},
				{Name: "max", Label: "最大振幅%", Type: "number", Default: 8},
			},
		},
		{
			ID:          "formula",
			Name:        "公式因子",
			Kind:        "score",
			Description: "调用公式中心里的公式，命中则按权重加分。",
			Params: []StrategyFactorParamDef{
				{Name: "formula_id", Label: "公式ID", Type: "string", Default: ""},
				{Name: "formula_name", Label: "公式名称", Type: "string", Default: ""},
			},
		},
	})
}
