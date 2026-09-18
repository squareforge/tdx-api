package main

import (
	"fmt"
	"time"
)

// TSignalReplayRequest 是盘后回放输入。分钟bar来自市场服务，配置始终取服务端已保存版本，
// 这样回放和盘中使用完全一致的规则与费用过滤。
type TSignalReplayRequest struct {
	Bars      []MinuteBar `json:"bars"`
	PrevClose float64     `json:"prev_close"`
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
}

// ReplayMinuteBars 在隔离引擎中逐根运行历史分钟bar。它不会写库、推送Webhook或影响盘中订阅。
// 回放中的买卖信号以触发价自动成交，便于直观看到信号对和成本后的当日账本。
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

	simulation := &TSignalEngine{
		builder: NewMinuteBarBuilder(),
		state: TSignalState{
			Day:       firstAt.Format("20060102"),
			Config:    base.Config,
			PrevClose: prevClose,
			PrevVol:   base.PrevVol,
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
		// 回放是策略验证，不等待人工点击。新信号立即以参考价成交，下一根bar便可按真实状态
		// 评估回补条件，并把完整买卖对展示到模拟账本。
		for _, signal := range simulation.state.Signals[before:] {
			if signal.Direction == TSignalDirSellFirst || signal.Direction == TSignalDirBuyBack {
				if err := simulation.AckSignal(signal.ID, TSignalStatusExecuted); err != nil {
					return TSignalReplayResult{}, fmt.Errorf("第%d根分钟bar模拟成交失败: %w", index+1, err)
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
	return result, nil
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
