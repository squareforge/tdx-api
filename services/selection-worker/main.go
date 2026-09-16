package main

import (
	"log"
	"net/http"
)

func main() {
	if !useMarketService() || allowDirectMarketFallback() {
		if err := ensureLocalMarketRuntime(); err != nil {
			log.Fatal(err)
		}
	}
	if err := initAutomationServices(); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/api/health", handleHealthCheck)
	http.HandleFunc("/api/server-status", handleGetServerStatus)
	http.HandleFunc("/api/formula/health", handleFormulaHealth)
	http.HandleFunc("/api/formula/run", handleFormulaRun)
	http.HandleFunc("/api/formulas", handleFormulas)
	http.HandleFunc("/api/formulas/", handleFormulaOperations)
	http.HandleFunc("/api/strategies", handleStrategies)
	http.HandleFunc("/api/strategies/", handleStrategyOperations)
	http.HandleFunc("/api/factors", handleStrategyFactors)
	http.HandleFunc("/api/stock-pools", handleStockPools)
	http.HandleFunc("/api/stock-pools/", handleStockPoolOperations)
	http.HandleFunc("/api/automations", handleAutomationTasks)
	http.HandleFunc("/api/automations/templates", handleAutomationTemplates)
	http.HandleFunc("/api/automations/reload", handleAutomationReload)
	http.HandleFunc("/api/automations/runs", handleAutomationRuns)
	http.HandleFunc("/api/selection-results", handleSelectionResults)
	http.HandleFunc("/api/selection-results/tracking", handleSelectionTracking)
	http.HandleFunc("/api/decision-notes", handleDecisionNotes)
	http.HandleFunc("/api/decision-notes/", handleDecisionNoteOperations)
	http.HandleFunc("/api/quote-alerts", handleQuoteAlerts)
	http.HandleFunc("/api/daily-review", handleDailyReview)
	http.HandleFunc("/api/historical-backtests", handleHistoricalBacktests)
	http.HandleFunc("/api/historical-backtests/scan", handleHistoricalScan)
	http.HandleFunc("/api/historical-backtests/scan/runs", handleHistoricalScanRuns)
	http.HandleFunc("/api/historical-backtests/scan/", handleHistoricalScanResults)
	http.HandleFunc("/api/historical-backtests/", handleHistoricalBacktests)
	http.HandleFunc("/api/automations/", handleAutomationOperations)
	http.HandleFunc("/api/webhooks", handleWebhooks)
	http.HandleFunc("/api/webhooks/", handleWebhookOperations)
	http.HandleFunc("/api/hqchart/kline", handleHQChartKline)
	http.HandleFunc("/api/hqchart/history", handleHQChartHistory)

	listenAndServe("8082")
}
