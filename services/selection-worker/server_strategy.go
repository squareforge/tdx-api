package main

import (
	"context"
	"encoding/json"
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
		handleStrategyRun(w, r, id)
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
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	run, err := automationRunner.runTask(ctx, task)
	if err != nil {
		errorResponse(w, "策略运行失败: "+err.Error())
		return
	}
	successResponse(w, run)
}

func handleStrategyFactors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	successResponse(w, []StrategyFactorDefinition{
		{ID: "nr7_close_breakout", Name: "NR7收盘突破", Kind: "filter", Description: "前一日为近7日最窄振幅，今日收盘突破其高点且放量。", Params: []StrategyFactorParamDef{{Name: "min_breakout", Label: "最低突破%", Type: "number", Default: 0.3}, {Name: "max_breakout", Label: "最高突破%", Type: "number", Default: 5}, {Name: "min_volume_ratio", Label: "最低量比", Type: "number", Default: 1.2}}},
		{ID: "spring_reclaim", Name: "箱体假跌破收回", Kind: "filter", Description: "今日跌破前N日低点后收盘收回，要求箱体及下破深度受限。", Params: []StrategyFactorParamDef{{Name: "lookback", Label: "箱体天数", Type: "number", Default: 20}, {Name: "max_box_range", Label: "最大箱体振幅%", Type: "number", Default: 25}, {Name: "min_break_depth", Label: "最小下破%", Type: "number", Default: 0.1}, {Name: "max_break_depth", Label: "最大下破%", Type: "number", Default: 3}, {Name: "min_reclaim", Label: "最低收复%", Type: "number", Default: 0.5}, {Name: "min_close_position", Label: "最低收盘位置", Type: "number", Default: 0.65}}},
		{ID: "gap_down_reclaim", Name: "低开收复", Kind: "filter", Description: "今日低开后收盘收复昨收，要求强收盘、放量和MA20上方。", Params: []StrategyFactorParamDef{{Name: "min_gap", Label: "最低开盘涨幅%", Type: "number", Default: -4}, {Name: "max_gap", Label: "最高开盘涨幅%", Type: "number", Default: -1}, {Name: "min_reclaim", Label: "最低收复%", Type: "number", Default: 0.3}, {Name: "min_close_position", Label: "最低收盘位置", Type: "number", Default: 0.7}, {Name: "min_volume_ratio", Label: "最低量比", Type: "number", Default: 1.1}}},
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
			ID: "low_three_bull_first_board", Name: "低位三连阳首板", Kind: "filter",
			Description: "信号日首次涨停，前两日连续阳线；同时约束放量、成交额、近5日波动与首板前60日位置。",
			Params: []StrategyFactorParamDef{
				{Name: "position_lookback", Label: "位置回看天数", Type: "number", Default: 60},
				{Name: "max_pre_position", Label: "首板前最高位置%", Type: "number", Default: 85},
				{Name: "min_amount", Label: "最低成交额", Type: "number", Default: 500000000},
				{Name: "max_amount", Label: "最高成交额", Type: "number", Default: 4000000000},
				{Name: "min_volume_ratio", Label: "最低放量倍数", Type: "number", Default: 2},
				{Name: "max_prior_volume_ratio", Label: "前日最大放量倍数", Type: "number", Default: 2},
				{Name: "max_prior_gain", Label: "前序单日最大涨幅%", Type: "number", Default: 5},
				{Name: "max_five_day_range", Label: "5日最大区间振幅%", Type: "number", Default: 20},
			},
		},
		{
			ID: "limit_up_streak", Name: "连续涨停板数", Kind: "filter",
			Description: "统计截至信号日收盘的连续涨停板数；连板龙头还会取当日候选池内的最高连板。",
			Params: []StrategyFactorParamDef{
				{Name: "min_boards", Label: "最低连板数", Type: "number", Default: 2},
				{Name: "max_lookback", Label: "最大回看天数", Type: "number", Default: 20},
				{Name: "min_listing_days", Label: "最低可用历史天数", Type: "number", Default: 50},
				{Name: "limit_tolerance", Label: "涨停价容差%", Type: "number", Default: 0.25},
			},
		},
		{
			ID: "first_board_position", Name: "首板相对位置", Kind: "filter",
			Description: "收盘首板且收盘价处于近60日高低价区间的低位；上市时间仅以可用K线历史保守估算。",
			Params: []StrategyFactorParamDef{
				{Name: "position_lookback", Label: "位置回看天数", Type: "number", Default: 60},
				{Name: "max_position", Label: "最高相对位置%", Type: "number", Default: 50},
				{Name: "min_listing_days", Label: "最低可用历史天数", Type: "number", Default: 250},
				{Name: "limit_tolerance", Label: "涨停价容差%", Type: "number", Default: 0.25},
			},
		},
		{
			ID: "rebalance_day", Name: "周期首个交易日", Kind: "filter",
			Description: "只在每周或每月的第一个交易日触发调仓。",
			Params:      []StrategyFactorParamDef{{Name: "mode", Label: "周期", Type: "string", Default: "week_first"}},
		},
		{
			ID: "market_cap_range", Name: "点时市值区间", Kind: "filter",
			Description: "使用信号日收盘价与当日已生效总股本计算历史总市值。",
			Params:      []StrategyFactorParamDef{{Name: "min_yi", Label: "最低市值(亿)", Type: "number", Default: 20}, {Name: "max_yi", Label: "最高市值(亿)", Type: "number", Default: 30}},
		},
		{
			ID: "market_cap_ascending", Name: "点时市值升序", Kind: "score",
			Description: "按信号日点时总市值从小到大计算横截面分位。",
		},
		{
			ID: "dividend_yield_percentile", Name: "近一年股息率分位", Kind: "filter",
			Description: "按除权除息表计算截至信号日近一年现金股息率并做市场横截面排名。",
			Params:      []StrategyFactorParamDef{{Name: "min_percentile", Label: "最低分位%", Type: "number", Default: 75}, {Name: "min_yield", Label: "最低股息率%", Type: "number", Default: 0}},
		},
		{
			ID: "peg_range", Name: "点时PEG区间", Kind: "filter",
			Description: "只使用信号日前已披露的PE、净利润增速计算PEG。",
			Params:      []StrategyFactorParamDef{{Name: "min", Label: "最低PEG", Type: "number", Default: -1}, {Name: "max", Label: "最高PEG", Type: "number", Default: 3}},
		},
		{
			ID: "raw_price_range", Name: "原始价格区间", Kind: "filter",
			Description: "使用不复权收盘价做价格过滤，适合复刻聚宽低价条件。",
			Params:      []StrategyFactorParamDef{{Name: "min", Label: "最低价格", Type: "number", Default: 0.5}, {Name: "max", Label: "最高价格", Type: "number", Default: 9}},
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
