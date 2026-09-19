package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// 日内做T信号 API,对齐 docs/INTRADAY_T_SIGNAL_UI_V1.md 4.2 聚合接口。

var tSignalEngine *TSignalEngine

func initTSignalEngine(store *AppStore) {
	if tSignalEngine == nil {
		tSignalEngine = NewTSignalEngine(store, marketClient)
	}
	tSignalEngine.Start()
}

// handleTSignalState GET /api/t-signal/state
// 顶部状态条:当前标的、趋势否决状态、数据流健康度。
func handleTSignalState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || tSignalEngine == nil {
		errorResponse(w, "只支持GET请求")
		return
	}
	state := tSignalEngine.State()
	successResponse(w, map[string]interface{}{
		"symbol":          state.Config.Symbol,
		"position":        state.Config.PositionShares,
		"trend_veto":      state.Trend,
		"cooldown":        state.Cooldown,
		"day":             state.Day,
		"prev_close":      state.PrevClose,
		"prev_volume":     state.PrevVol,
		"prev_day_return": state.PrevDayReturn,
		"health":          tSignalEngine.Health(),
		"config":          state.Config,
	})
}

// handleTSignalSnapshot GET /api/t-signal/snapshot
// UI 轮询主数据源:分钟bar + 当前信号 + 当日记录聚合。
func handleTSignalSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || tSignalEngine == nil {
		errorResponse(w, "只支持GET请求")
		return
	}
	state := tSignalEngine.State()
	successResponse(w, map[string]interface{}{
		"bars":     tSignalEngine.BarsSnapshot(),
		"signals":  state.Signals,
		"ledger":   state.Ledger,
		"trend":    state.Trend,
		"cooldown": state.Cooldown,
		"health":   tSignalEngine.Health(),
	})
}

// handleTSignalRecords GET /api/t-signal/records
// 当日账本(成对交易、净盈亏、冷却状态)。
func handleTSignalRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || tSignalEngine == nil {
		errorResponse(w, "只支持GET请求")
		return
	}
	state := tSignalEngine.State()
	total := 0.0
	closedCount := 0
	for _, entry := range state.Ledger {
		if entry.Closed {
			total += entry.NetPnl
			closedCount++
		}
	}
	successResponse(w, map[string]interface{}{
		"ledger":       state.Ledger,
		"closed_count": closedCount,
		"total_pnl":    total,
		"cooldown":     state.Cooldown,
	})
}

// handleTSignalReplay POST /api/t-signal/replay
// 盘后逐根回放的隔离计算接口。只读取当前配置，绝不改写实时引擎状态。
func handleTSignalReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || tSignalEngine == nil {
		errorResponse(w, "只支持POST请求")
		return
	}
	var body TSignalReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errorResponse(w, "请求体格式错误")
		return
	}
	result, err := tSignalEngine.ReplayMinuteBars(body)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, result)
}

// handleTSignalConfig GET/PUT /api/t-signal/config
// 标的订阅、底仓、S1-S6 参数、费率、开关(UI文档4.2 配置面板)。
func handleTSignalConfig(w http.ResponseWriter, r *http.Request) {
	if tSignalEngine == nil {
		errorResponse(w, "信号引擎未初始化")
		return
	}
	switch r.Method {
	case http.MethodGet:
		state := tSignalEngine.State()
		successResponse(w, map[string]interface{}{
			"config": state.Config,
			"day":    state.Day,
		})
	case http.MethodPut:
		var body struct {
			Config TSignalConfig `json:"config"`
			Force  bool          `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errorResponse(w, "请求体格式错误")
			return
		}
		if err := tSignalEngine.UpdateConfig(body.Config, body.Force); err != nil {
			errorResponse(w, err.Error())
			return
		}
		state := tSignalEngine.State()
		successResponse(w, map[string]interface{}{"config": state.Config})
	default:
		errorResponse(w, "只支持GET/PUT请求")
	}
}

// handleTSignalAck POST /api/t-signal/:id/ack
// 人工标记 EXECUTED / CANCELLED(文档4.4 信号生命周期)。
func handleTSignalAck(w http.ResponseWriter, r *http.Request) {
	if tSignalEngine == nil {
		errorResponse(w, "信号引擎未初始化")
		return
	}
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	parts := pathParts(r.URL.Path, "/api/t-signal/")
	if len(parts) != 2 || parts[1] != "ack" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errorResponse(w, "请求体格式错误")
		return
	}
	body.Action = strings.ToUpper(strings.TrimSpace(body.Action))
	if err := tSignalEngine.AckSignal(id, body.Action); err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, map[string]interface{}{"id": id, "status": body.Action})
}
