package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const historicalBacktestProfileDir = "data/logs/historical-backtests"

type historicalBacktestProfile struct {
	runID   string
	file    *os.File
	encoder *json.Encoder
	started time.Time
}

type historicalFormulaTiming struct {
	Requests     int     `json:"requests"`
	Symbols      int     `json:"symbols"`
	Rows         int     `json:"rows"`
	WorkerTickMS int64   `json:"worker_tick_ms"`
	RequestMS    float64 `json:"request_ms"`
}

type historicalStrategyTiming struct {
	Name          string                  `json:"name"`
	Available     int                     `json:"available"`
	Matches       int                     `json:"matches"`
	PreparationMS float64                 `json:"preparation_ms"`
	FormulaMS     float64                 `json:"formula_ms"`
	EvaluationMS  float64                 `json:"evaluation_ms"`
	TrackingMS    float64                 `json:"tracking_ms"`
	Formula       historicalFormulaTiming `json:"formula"`
}

func openHistoricalBacktestProfile(runID string) *historicalBacktestProfile {
	if runID == "" || strings.ContainsAny(runID, `/\\`) || runID == "." || runID == ".." {
		log.Printf("historical backtest: invalid run ID for timing log")
		return nil
	}
	if err := os.MkdirAll(historicalBacktestProfileDir, 0755); err != nil {
		log.Printf("historical backtest %s: cannot create timing directory: %v", runID, err)
		return nil
	}
	path := filepath.Join(historicalBacktestProfileDir, runID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("historical backtest %s: cannot create timing log: %v", runID, err)
		return nil
	}
	return &historicalBacktestProfile{runID: runID, file: file, encoder: json.NewEncoder(file), started: time.Now()}
}

func (p *historicalBacktestProfile) record(event string, fields map[string]any) {
	if p == nil || p.encoder == nil {
		return
	}
	fields["event"] = event
	fields["run_id"] = p.runID
	fields["logged_at"] = time.Now().Format(time.RFC3339Nano)
	if err := p.encoder.Encode(fields); err != nil {
		log.Printf("historical backtest %s: cannot write timing log: %v", p.runID, err)
		p.encoder = nil
	}
}

func (p *historicalBacktestProfile) close(runErr error) {
	if p == nil {
		return
	}
	status := "success"
	fields := map[string]any{"elapsed_ms": profileMillis(time.Since(p.started))}
	if runErr != nil {
		status = "failed"
		fields["error"] = runErr.Error()
	}
	fields["status"] = status
	p.record("finish", fields)
	if err := p.file.Close(); err != nil {
		log.Printf("historical backtest %s: cannot close timing log: %v", p.runID, err)
	}
}

func profileMillis(duration time.Duration) float64 {
	return math.Round(float64(duration)/float64(time.Millisecond)*100) / 100
}
