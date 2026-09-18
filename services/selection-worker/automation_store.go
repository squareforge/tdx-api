package main

import workbench "workbench-core"

type AppStore = workbench.AppStore
type Formula = workbench.Formula
type StockPool = workbench.StockPool
type Strategy = workbench.Strategy
type AutomationTask = workbench.AutomationTask
type AutomationRun = workbench.AutomationRun
type SelectionResult = workbench.SelectionResult
type HistoricalBacktestRun = workbench.HistoricalBacktestRun
type HistoricalBacktestSignal = workbench.HistoricalBacktestSignal
type HistoricalBacktestSignalQuery = workbench.HistoricalBacktestSignalQuery
type HistoricalBacktestSignalPage = workbench.HistoricalBacktestSignalPage
type HistoricalBacktestTrade = workbench.HistoricalBacktestTrade
type HistoricalBacktestTradeQuery = workbench.HistoricalBacktestTradeQuery
type HistoricalBacktestTradePage = workbench.HistoricalBacktestTradePage
type TrackingBar = workbench.TrackingBar
type SelectionHorizon = workbench.SelectionHorizon
type SelectionTracking = workbench.SelectionTracking
type SelectionTrackingItem = workbench.SelectionTrackingItem
type SelectionTrackingSummary = workbench.SelectionTrackingSummary
type SelectionHorizonSummary = workbench.SelectionHorizonSummary
type DecisionNote = workbench.DecisionNote
type Webhook = workbench.Webhook
type TradingAccount = workbench.TradingAccount
type TradingDiscipline = workbench.TradingDiscipline
type TradingFeeConfig = workbench.TradingFeeConfig
type TradingTrade = workbench.TradingTrade
type TradingSystemState = workbench.TradingSystemState
type TSignalStateRecord = workbench.TSignalStateRecord

const (
	DefaultTrackingTargetReturn  = workbench.DefaultTrackingTargetReturn
	DefaultTrackingDrawdownLimit = workbench.DefaultTrackingDrawdownLimit
)

const (
	DecisionWatchPoolID            = workbench.DecisionWatchPoolID
	DecisionExcludePoolID          = workbench.DecisionExcludePoolID
	FixedCloseSyncTaskID           = workbench.FixedCloseSyncTaskID
	FixedSelectionTrackingTaskID   = workbench.FixedSelectionTrackingTaskID
	SystemStrategyDailyBatchTaskID = workbench.SystemStrategyDailyBatchTaskID
	TradingSystemStateID           = workbench.TradingSystemStateID
)

func OpenAppStore() (*AppStore, error) {
	workbench.SetCodeModelProvider(getAllCodeModels)
	return workbench.OpenAppStore()
}

func nowText() string {
	return workbench.NowText()
}

func normalizeSymbols(symbols []string) []string {
	return workbench.NormalizeSymbols(symbols)
}

func defaultTradingSystemState() TradingSystemState {
	return workbench.DefaultTradingSystemState()
}

func isFixedAutomationTaskID(id string) bool {
	return workbench.IsFixedAutomationTaskID(id)
}

func mustJSON(v interface{}) string {
	return workbench.MustJSON(v)
}

func limitedMarketPoolSymbols(poolID string, maxCodes int) []string {
	return workbench.LimitedMarketPoolSymbols(poolID, maxCodes)
}
