package workbench

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type HistoricalScanRun struct {
	ID                  string   `json:"id"`
	Status              string   `json:"status"`
	StartDate           string   `json:"start_date"`
	EndDate             string   `json:"end_date"`
	StrategyIDs         []string `json:"strategy_ids"`
	TotalStrategies     int      `json:"total_strategies"`
	ProcessedStrategies int      `json:"processed_strategies"`
	CurrentStrategy     string   `json:"current_strategy"`
	SignalCount         int      `json:"signal_count"`
	Error               string   `json:"error"`
	CreatedAt           string   `json:"created_at"`
	StartedAt           string   `json:"started_at"`
	FinishedAt          string   `json:"finished_at"`
	UpdatedAt           string   `json:"updated_at"`
}

type HistoricalScanResult struct {
	ID            string  `json:"id"`
	RunID         string  `json:"run_id"`
	StrategyID    string  `json:"strategy_id"`
	StrategyName  string  `json:"strategy_name"`
	BuyBase       string  `json:"buy_base"`
	SellBase      string  `json:"sell_base"`
	TakeProfit    float64 `json:"take_profit"`
	TrailDrawdown float64 `json:"trail_drawdown"`
	StopLoss      float64 `json:"stop_loss"`
	MaxHoldDays   int     `json:"max_hold_days"`
	TotalReturn   float64 `json:"total_return"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	WinRate       float64 `json:"win_rate"`
	ClosedCount   int     `json:"closed_count"`
	OpenCount     int     `json:"open_count"`
	FinalEquity   float64 `json:"final_equity"`
	CreatedAt     string  `json:"created_at"`
}

func (s *AppStore) CreateHistoricalScanRun(run HistoricalScanRun) (HistoricalScanRun, error) {
	if strings.TrimSpace(run.StartDate) == "" || strings.TrimSpace(run.EndDate) == "" {
		return run, errors.New("扫描开始和结束日期不能为空")
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status == "" {
		run.Status = "running"
	}
	strategyIDsJSON, _ := json.Marshal(run.StrategyIDs)
	now := NowText()
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	run.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO historical_scan_runs
		(id,status,start_date,end_date,strategy_ids_json,total_strategies,processed_strategies,current_strategy,signal_count,error,created_at,started_at,finished_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.Status, run.StartDate, run.EndDate, string(strategyIDsJSON), run.TotalStrategies,
		run.ProcessedStrategies, run.CurrentStrategy, run.SignalCount, run.Error, run.CreatedAt, run.StartedAt, run.FinishedAt, run.UpdatedAt)
	return run, err
}

func (s *AppStore) UpdateHistoricalScanProgress(runID string, processed, total int, currentStrategy string, signalCount int) error {
	_, err := s.db.Exec(`UPDATE historical_scan_runs SET processed_strategies=?,total_strategies=?,current_strategy=?,signal_count=?,updated_at=? WHERE id=?`,
		processed, total, currentStrategy, signalCount, NowText(), runID)
	return err
}

func (s *AppStore) FinishHistoricalScanRun(runID, status, errorText string) error {
	_, err := s.db.Exec(`UPDATE historical_scan_runs SET status=?,error=?,finished_at=?,updated_at=? WHERE id=?`,
		status, errorText, NowText(), NowText(), runID)
	return err
}

func (s *AppStore) SaveHistoricalScanResults(runID string, results []HistoricalScanResult) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := NowText()
	for _, item := range results {
		item.ID = uuid.NewString()
		item.RunID = runID
		item.CreatedAt = now
		if _, err := tx.Exec(`INSERT INTO historical_scan_results
			(id,run_id,strategy_id,strategy_name,buy_base,sell_base,take_profit,trail_drawdown,stop_loss,max_hold_days,
			total_return,max_drawdown,win_rate,closed_count,open_count,final_equity,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			item.ID, item.RunID, item.StrategyID, item.StrategyName, item.BuyBase, item.SellBase, item.TakeProfit,
			item.TrailDrawdown, item.StopLoss, item.MaxHoldDays, item.TotalReturn, item.MaxDrawdown, item.WinRate,
			item.ClosedCount, item.OpenCount, item.FinalEquity, item.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppStore) ListHistoricalScanRuns(limit int) ([]HistoricalScanRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT id,status,start_date,end_date,strategy_ids_json,total_strategies,processed_strategies,current_strategy,signal_count,error,created_at,started_at,finished_at,updated_at
		FROM historical_scan_runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoricalScanRun, 0, limit)
	for rows.Next() {
		var run HistoricalScanRun
		var strategyIDsJSON string
		if err := rows.Scan(&run.ID, &run.Status, &run.StartDate, &run.EndDate, &strategyIDsJSON, &run.TotalStrategies,
			&run.ProcessedStrategies, &run.CurrentStrategy, &run.SignalCount, &run.Error, &run.CreatedAt,
			&run.StartedAt, &run.FinishedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(strategyIDsJSON), &run.StrategyIDs)
		items = append(items, run)
	}
	return items, rows.Err()
}

func (s *AppStore) GetHistoricalScanRun(id string) (HistoricalScanRun, error) {
	var run HistoricalScanRun
	var strategyIDsJSON string
	err := s.db.QueryRow(`SELECT id,status,start_date,end_date,strategy_ids_json,total_strategies,processed_strategies,current_strategy,signal_count,error,created_at,started_at,finished_at,updated_at
		FROM historical_scan_runs WHERE id=?`, id).
		Scan(&run.ID, &run.Status, &run.StartDate, &run.EndDate, &strategyIDsJSON, &run.TotalStrategies,
			&run.ProcessedStrategies, &run.CurrentStrategy, &run.SignalCount, &run.Error, &run.CreatedAt,
			&run.StartedAt, &run.FinishedAt, &run.UpdatedAt)
	if err != nil {
		return run, err
	}
	_ = json.Unmarshal([]byte(strategyIDsJSON), &run.StrategyIDs)
	return run, nil
}

func (s *AppStore) ListHistoricalScanResults(runID, strategyID string) ([]HistoricalScanResult, error) {
	query := `SELECT id,run_id,strategy_id,strategy_name,buy_base,sell_base,take_profit,trail_drawdown,stop_loss,max_hold_days,
		total_return,max_drawdown,win_rate,closed_count,open_count,final_equity,created_at
		FROM historical_scan_results`
	args := []interface{}{}
	conds := []string{}
	if strings.TrimSpace(runID) != "" {
		conds = append(conds, "run_id=?")
		args = append(args, runID)
	}
	if strings.TrimSpace(strategyID) != "" {
		conds = append(conds, "strategy_id=?")
		args = append(args, strategyID)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY total_return DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoricalScanResult, 0, 128)
	for rows.Next() {
		var item HistoricalScanResult
		if err := rows.Scan(&item.ID, &item.RunID, &item.StrategyID, &item.StrategyName, &item.BuyBase, &item.SellBase,
			&item.TakeProfit, &item.TrailDrawdown, &item.StopLoss, &item.MaxHoldDays, &item.TotalReturn,
			&item.MaxDrawdown, &item.WinRate, &item.ClosedCount, &item.OpenCount, &item.FinalEquity, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
