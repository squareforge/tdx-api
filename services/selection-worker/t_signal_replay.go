package main

import (
	"fmt"
	"time"
)

// TSignalReplayRequest 是盘后回放输入。分钟bar来自市场服务，配置始终取服务端已保存版本，
// 这样回放和盘中使用完全一致的规则与费用过滤。
type TSignalReplayRequest struct {
	Bars          []MinuteBar `json:"bars"`
	PrevClose     float64     `json:"prev_close"`
	PrevVolume    float64     `json:"prev_volume,omitempty"`
	PrevDayReturn *float64    `json:"prev_day_return,omitempty"`
}

// TSignalReplayFrame 是每根bar处理完成后可见的策略状态快照。
// 前端按回放游标取对应帧，确保未来信号和未来成交不会提前显示。
type TSignalReplayFrame struct {
	Time     string               `json:"time"`
	Signals  []TSignalRecord      `json:"signals"`
	Ledger   []TSignalLedgerEntry `json:"ledger"`
	Trend    TSignalTrendVeto     `json:"trend"`
	Cooldown TSignalCooldown      `json:"cooldown"`
}

type TSignalReplayResult struct {
	Date         string               `json:"date"`
	Frames       []TSignalReplayFrame `json:"frames"`
	SkippedRules []string             `json:"skipped_rules,omitempty"`
	Summary      TSignalReplaySummary `json:"summary"`
}

type TSignalReplaySummary struct {
	ClosedTrades int     `json:"closed_trades"`
	Wins         int     `json:"wins"`
	WinRate      float64 `json:"win_rate"`
	NetPnl       float64 `json:"net_pnl"`
	ReturnPct    float64 `json:"return_pct"`
}

// ReplayMinuteBars 在隔离引擎中逐根运行历史分钟bar。它不会写库、推送Webhook或影响盘中订阅。
// 回放信号在下一根bar开盘成交，止盈与止损按随后bar的高低价判断；同一bar
// 同时触及时按止损优先，14:55或数据末尾强制平仓。
func (e *TSignalEngine) ReplayMinuteBars(request TSignalReplayRequest) (TSignalReplayResult, error) {
	if len(request.Bars) == 0 {
		return TSignalReplayResult{}, fmt.Errorf("回放分钟数据不能为空")
	}
	if len(request.Bars) > 240 {
		return TSignalReplayResult{}, fmt.Errorf("回放分钟数据不能超过240根")
	}

	firstAt, err := time.Parse(time.RFC3339, request.Bars[0].Time)
	if err != nil {
		return TSignalReplayResult{}, fmt.Errorf("首根分钟bar时间格式错误")
	}
	base := e.State()
	prevClose := request.PrevClose
	if prevClose <= 0 {
		prevClose = base.PrevClose
	}
	if prevClose <= 0 {
		prevClose = request.Bars[0].Open
	}
	if prevClose <= 0 {
		prevClose = request.Bars[0].Close
	}
	prevVolume := request.PrevVolume
	if prevVolume <= 0 {
		prevVolume = base.PrevVol
	}
	prevDayReturn := base.PrevDayReturn
	if request.PrevDayReturn != nil {
		prevDayReturn = *request.PrevDayReturn
	}

	simulation := &TSignalEngine{
		builder: NewMinuteBarBuilder(),
		state: TSignalState{
			Day:           firstAt.Format("20060102"),
			Config:        base.Config,
			PrevClose:     prevClose,
			PrevVol:       prevVolume,
			PrevDayReturn: prevDayReturn,
		},
		lastS5: map[string]time.Time{},
	}
	result := TSignalReplayResult{
		Date:   simulation.state.Day,
		Frames: make([]TSignalReplayFrame, 0, len(request.Bars)),
	}
	// S5 的触发条件依赖90秒内逐笔快照的现量与涨速。分钟bar并不携带这些字段，
	// 不伪造快照，避免在回测界面给出看似准确的买卖点。
	if simulation.state.Config.Rules.S5Enabled {
		result.SkippedRules = []string{TSignalRuleS5Spike}
	}

	pending := make([]string, 0, 2)
	entryIndexes := map[string]int{}
	for index, input := range request.Bars {
		at, err := time.Parse(time.RFC3339, input.Time)
		if err != nil {
			return TSignalReplayResult{}, fmt.Errorf("第%d根分钟bar时间格式错误", index+1)
		}
		if index > 0 {
			previous, _ := time.Parse(time.RFC3339, request.Bars[index-1].Time)
			if !at.After(previous) {
				return TSignalReplayResult{}, fmt.Errorf("分钟bar必须按时间升序排列")
			}
		}
		if input.Open <= 0 || input.High <= 0 || input.Low <= 0 || input.Close <= 0 {
			return TSignalReplayResult{}, fmt.Errorf("第%d根分钟bar价格无效", index+1)
		}

		// A completed bar can only create an order for the next bar. This prevents
		// using the triggering close as a fill that was unknowable beforehand.
		for _, signalID := range pending {
			ledgerID, filled, err := simulation.executeReplaySignal(signalID, input.Open, at)
			if err != nil {
				return TSignalReplayResult{}, fmt.Errorf("第%d根分钟bar模拟成交失败: %w", index+1, err)
			}
			if filled && ledgerID != "" {
				entryIndexes[ledgerID] = index
			}
		}
		pending = pending[:0]

		if err := simulation.replayExitOpenTrades(input, at, index, entryIndexes, index == len(request.Bars)-1); err != nil {
			return TSignalReplayResult{}, fmt.Errorf("第%d根分钟bar模拟退出失败: %w", index+1, err)
		}

		bar := input
		bar.Complete = true
		bar.Trusted = true
		if bar.Amount <= 0 {
			bar.Amount = bar.Close * bar.Volume
		}
		simulation.builder.completed = append(simulation.builder.completed, bar)
		simulation.builder.lastVol += bar.Volume / 100
		simulation.builder.lastAmt += bar.Amount

		before := len(simulation.state.Signals)
		simulation.onMinuteBar(&simulation.builder.completed[len(simulation.builder.completed)-1])
		for signalIndex := before; signalIndex < len(simulation.state.Signals); signalIndex++ {
			signal := &simulation.state.Signals[signalIndex]
			if signal.Direction == TSignalDirSellFirst || signal.Direction == TSignalDirBuyBack {
				if index+1 < len(request.Bars) {
					pending = append(pending, signal.ID)
				} else {
					signal.Status = TSignalStatusCancelled
					signal.AckAt = at.Format(time.RFC3339)
				}
			}
		}

		result.Frames = append(result.Frames, TSignalReplayFrame{
			Time:     bar.Time,
			Signals:  copyTSignalRecords(simulation.state.Signals),
			Ledger:   copyTSignalLedger(simulation.state.Ledger),
			Trend:    simulation.state.Trend,
			Cooldown: simulation.state.Cooldown,
		})
	}
	result.Summary = replaySummary(simulation.state, prevClose)
	return result, nil
}

func (e *TSignalEngine) executeReplaySignal(id string, price float64, at time.Time) (string, bool, error) {
	for i := range e.state.Signals {
		signal := &e.state.Signals[i]
		if signal.ID != id || (signal.Status != TSignalStatusNew && signal.Status != TSignalStatusActive) {
			continue
		}
		if signal.Direction == TSignalDirSellFirst {
			if signal.Rule == TSignalRuleS6Vwap && signal.RefPrice > 0 {
				targetPct := (signal.RefPrice - signal.Target) / signal.RefPrice
				stopPct := (signal.Stop - signal.RefPrice) / signal.RefPrice
				signal.Target = price * (1 - targetPct)
				signal.Stop = price * (1 + stopPct)
				if signal.FollowUp != nil {
					signal.FollowUp.Ref = signal.Target
				}
			} else if signal.Target >= price || signal.Stop <= price {
				signal.Status = TSignalStatusCancelled
				signal.AckAt = at.Format(time.RFC3339)
				return "", false, nil
			}
		}
		executed := *signal
		executed.RefPrice = price
		executed.Time = at.Format(time.RFC3339)
		if err := e.onExecuted(executed); err != nil {
			return "", false, err
		}
		signal.Status = TSignalStatusExecuted
		signal.AckAt = at.Format(time.RFC3339)
		if signal.Direction == TSignalDirSellFirst && len(e.state.Ledger) > 0 {
			return e.state.Ledger[len(e.state.Ledger)-1].ID, true, nil
		}
		return "", true, nil
	}
	return "", false, nil
}

func (e *TSignalEngine) replayExitOpenTrades(bar MinuteBar, at time.Time, index int, entryIndexes map[string]int, final bool) error {
	for ledgerIndex := range e.state.Ledger {
		entry := &e.state.Ledger[ledgerIndex]
		if entry.Closed {
			continue
		}
		var sell *TSignalRecord
		for signalIndex := range e.state.Signals {
			if e.state.Signals[signalIndex].ID == entry.SellSignalID {
				sell = &e.state.Signals[signalIndex]
				break
			}
		}
		if sell == nil {
			continue
		}
		exitPrice, rule, reason := 0.0, "", ""
		if sell.Stop > 0 && bar.High >= sell.Stop {
			exitPrice, rule, reason = maxFloat(bar.Open, sell.Stop), TSignalRuleStopExit, "止损价触发"
		} else if sell.Target > 0 && bar.Low <= sell.Target {
			exitPrice, rule, reason = minFloat(bar.Open, sell.Target), TSignalRuleFollowUp, "目标价触发"
		} else {
			holdBars := 120
			if sell.Rule == TSignalRuleS6Vwap && e.state.Config.Rules.S6MaxHoldMinutes > 0 {
				holdBars = e.state.Config.Rules.S6MaxHoldMinutes
			}
			hm := at.Hour()*100 + at.Minute()
			if final || hm >= 1455 || index-entryIndexes[entry.ID] >= holdBars {
				exitPrice, rule, reason = bar.Close, TSignalRuleCloseExit, "到时或收盘强制回补"
			}
		}
		if exitPrice <= 0 {
			continue
		}
		e.emitSignalFromPrice(at, TSignalDirBuyBack, rule, reason, exitPrice, exitPrice, exitPrice, nil)
		buyID := e.state.Signals[len(e.state.Signals)-1].ID
		if _, _, err := e.executeReplaySignal(buyID, exitPrice, at); err != nil {
			return err
		}
	}
	return nil
}

func replaySummary(state TSignalState, prevClose float64) TSignalReplaySummary {
	summary := TSignalReplaySummary{}
	for _, entry := range state.Ledger {
		if !entry.Closed {
			continue
		}
		summary.ClosedTrades++
		summary.NetPnl += entry.NetPnl
		if entry.NetPnl > 0 {
			summary.Wins++
		}
	}
	if summary.ClosedTrades > 0 {
		summary.WinRate = float64(summary.Wins) / float64(summary.ClosedTrades) * 100
	}
	if state.Config.PositionShares > 0 && prevClose > 0 {
		summary.ReturnPct = summary.NetPnl / (float64(state.Config.PositionShares) * prevClose) * 100
	}
	return summary
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func copyTSignalRecords(source []TSignalRecord) []TSignalRecord {
	result := make([]TSignalRecord, len(source))
	copy(result, source)
	return result
}

func copyTSignalLedger(source []TSignalLedgerEntry) []TSignalLedgerEntry {
	result := make([]TSignalLedgerEntry, len(source))
	copy(result, source)
	return result
}
