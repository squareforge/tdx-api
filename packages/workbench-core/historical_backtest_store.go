package workbench

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type HistoricalBacktestRun struct {
	ID                   string   `json:"id"`
	Status               string   `json:"status"`
	StartDate            string   `json:"start_date"`
	EndDate              string   `json:"end_date"`
	StrategyIDs          []string `json:"strategy_ids"`
	StrategySnapshotJSON string   `json:"strategy_snapshot_json,omitempty"`
	Horizons             []int    `json:"horizons"`
	TargetReturn         float64  `json:"target_return"`
	DrawdownLimit        float64  `json:"drawdown_limit"`
	BuyBase              string   `json:"buy_base"`
	BuyOffset            float64  `json:"buy_offset"`
	SellBase             string   `json:"sell_base"`
	TakeProfit           float64  `json:"take_profit"`
	TrailDrawdown        float64  `json:"trail_drawdown"`
	StopLoss             float64  `json:"stop_loss"`
	MaxHoldDays          int      `json:"max_hold_days"`
	TotalDates           int      `json:"total_dates"`
	ProcessedDates       int      `json:"processed_dates"`
	CurrentDate          string   `json:"current_date"`
	CandidateSymbols     int      `json:"candidate_symbols"`
	SignalCount          int      `json:"signal_count"`
	ResultJSON           string   `json:"result_json"`
	Error                string   `json:"error"`
	CancelRequested      bool     `json:"cancel_requested"`
	CreatedAt            string   `json:"created_at"`
	StartedAt            string   `json:"started_at"`
	FinishedAt           string   `json:"finished_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type HistoricalBacktestSignal struct {
	ID           string  `json:"id"`
	RunID        string  `json:"run_id"`
	StrategyID   string  `json:"strategy_id"`
	StrategyName string  `json:"strategy_name"`
	SignalDate   string  `json:"signal_date"`
	Symbol       string  `json:"symbol"`
	Latest       float64 `json:"latest"`
	Score        float64 `json:"score"`
	DetailJSON   string  `json:"detail_json"`
	TrackingJSON string  `json:"tracking_json"`
	CreatedAt    string  `json:"created_at"`
}

type HistoricalBacktestSignalQuery struct {
	StrategyID string
	SignalDate string
	Symbol     string
	Limit      int
	Offset     int
}

type HistoricalBacktestSignalPage struct {
	Items  []HistoricalBacktestSignal `json:"items"`
	Total  int                        `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

func (s *AppStore) CreateHistoricalBacktestRun(run HistoricalBacktestRun) (HistoricalBacktestRun, error) {
	if strings.TrimSpace(run.StartDate) == "" || strings.TrimSpace(run.EndDate) == "" {
		return run, errors.New("回测开始和结束日期不能为空")
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status == "" {
		run.Status = "running"
	}
	if len(run.Horizons) == 0 {
		run.Horizons = []int{3, 5, 10}
	}
	strategyIDsJSON, _ := json.Marshal(run.StrategyIDs)
	horizonsJSON, _ := json.Marshal(run.Horizons)
	if strings.TrimSpace(run.StrategySnapshotJSON) == "" {
		run.StrategySnapshotJSON = "[]"
	}
	if strings.TrimSpace(run.ResultJSON) == "" {
		run.ResultJSON = "{}"
	}
	now := NowText()
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	run.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO historical_backtest_runs
		(id,status,start_date,end_date,strategy_ids_json,strategy_snapshot_json,horizons_json,target_return,drawdown_limit,
		buy_base,buy_offset,sell_base,take_profit,trail_drawdown,stop_loss,max_hold_days,
		total_dates,processed_dates,current_date,candidate_symbols,signal_count,result_json,error,cancel_requested,created_at,started_at,finished_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.Status, run.StartDate, run.EndDate, string(strategyIDsJSON), run.StrategySnapshotJSON, string(horizonsJSON),
		run.TargetReturn, run.DrawdownLimit, run.BuyBase, run.BuyOffset, run.SellBase, run.TakeProfit, run.TrailDrawdown,
		run.StopLoss, run.MaxHoldDays, run.TotalDates, run.ProcessedDates, run.CurrentDate, run.CandidateSymbols,
		run.SignalCount, run.ResultJSON, run.Error, boolInt(run.CancelRequested), run.CreatedAt, run.StartedAt, run.FinishedAt, run.UpdatedAt)
	return run, err
}

func scanHistoricalBacktestRun(scanner interface{ Scan(...interface{}) error }) (HistoricalBacktestRun, error) {
	var run HistoricalBacktestRun
	var strategyIDsJSON, horizonsJSON string
	var cancelRequested int
	err := scanner.Scan(&run.ID, &run.Status, &run.StartDate, &run.EndDate, &strategyIDsJSON, &run.StrategySnapshotJSON,
		&horizonsJSON, &run.TargetReturn, &run.DrawdownLimit, &run.BuyBase, &run.BuyOffset, &run.SellBase,
		&run.TakeProfit, &run.TrailDrawdown, &run.StopLoss, &run.MaxHoldDays, &run.TotalDates, &run.ProcessedDates,
		&run.CurrentDate, &run.CandidateSymbols, &run.SignalCount, &run.ResultJSON, &run.Error, &cancelRequested,
		&run.CreatedAt, &run.StartedAt, &run.FinishedAt, &run.UpdatedAt)
	if err != nil {
		return run, err
	}
	_ = json.Unmarshal([]byte(strategyIDsJSON), &run.StrategyIDs)
	_ = json.Unmarshal([]byte(horizonsJSON), &run.Horizons)
	run.CancelRequested = intBool(cancelRequested)
	return run, nil
}

const historicalBacktestRunColumns = `id,status,start_date,end_date,strategy_ids_json,strategy_snapshot_json,horizons_json,target_return,drawdown_limit,
	buy_base,buy_offset,sell_base,take_profit,trail_drawdown,stop_loss,max_hold_days,
	total_dates,processed_dates,"current_date",candidate_symbols,signal_count,result_json,error,cancel_requested,created_at,started_at,finished_at,updated_at`

func (s *AppStore) GetHistoricalBacktestRun(id string) (HistoricalBacktestRun, error) {
	return scanHistoricalBacktestRun(s.db.QueryRow(`SELECT `+historicalBacktestRunColumns+` FROM historical_backtest_runs WHERE id=?`, id))
}

func (s *AppStore) ListHistoricalBacktestRuns(limit int) ([]HistoricalBacktestRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT `+historicalBacktestRunColumns+` FROM historical_backtest_runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HistoricalBacktestRun, 0, limit)
	for rows.Next() {
		item, err := scanHistoricalBacktestRun(rows)
		if err != nil {
			return nil, err
		}
		item.StrategySnapshotJSON = ""
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) UpdateHistoricalBacktestProgress(id string, totalDates, processedDates int, currentDate string, candidates, signalCount int) error {
	_, err := s.db.Exec(`UPDATE historical_backtest_runs SET total_dates=?,processed_dates=?,current_date=?,candidate_symbols=?,signal_count=?,updated_at=? WHERE id=?`,
		totalDates, processedDates, currentDate, candidates, signalCount, NowText(), id)
	return err
}

func (s *AppStore) FinishHistoricalBacktestRun(id, status, resultJSON, errorText string) error {
	if !json.Valid([]byte(resultJSON)) {
		resultJSON = "{}"
	}
	now := NowText()
	_, err := s.db.Exec(`UPDATE historical_backtest_runs SET status=?,result_json=?,error=?,finished_at=?,updated_at=? WHERE id=?`,
		status, resultJSON, errorText, now, now, id)
	return err
}

func (s *AppStore) RequestHistoricalBacktestCancel(id string) error {
	result, err := s.db.Exec(`UPDATE historical_backtest_runs SET cancel_requested=1,updated_at=? WHERE id=? AND status='running'`, NowText(), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return errors.New("回测任务不存在或已经结束")
	}
	return nil
}

func (s *AppStore) HistoricalBacktestCancelRequested(id string) bool {
	var requested int
	return s.db.QueryRow(`SELECT cancel_requested FROM historical_backtest_runs WHERE id=?`, id).Scan(&requested) == nil && requested != 0
}

func (s *AppStore) InsertHistoricalBacktestSignals(items []HistoricalBacktestSignal) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO historical_backtest_signals
		(id,run_id,strategy_id,strategy_name,signal_date,symbol,latest,score,detail_json,tracking_json,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		if item.CreatedAt == "" {
			item.CreatedAt = NowText()
		}
		if item.DetailJSON == "" {
			item.DetailJSON = "{}"
		}
		if item.TrackingJSON == "" {
			item.TrackingJSON = "{}"
		}
		if _, err := stmt.Exec(item.ID, item.RunID, item.StrategyID, item.StrategyName, item.SignalDate, item.Symbol,
			item.Latest, item.Score, item.DetailJSON, item.TrackingJSON, item.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppStore) ListHistoricalBacktestSignals(runID string, query HistoricalBacktestSignalQuery) (HistoricalBacktestSignalPage, error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	conditions := []string{"run_id=?"}
	args := []interface{}{runID}
	if strings.TrimSpace(query.StrategyID) != "" {
		conditions = append(conditions, "strategy_id=?")
		args = append(args, query.StrategyID)
	}
	if strings.TrimSpace(query.SignalDate) != "" {
		conditions = append(conditions, "signal_date=?")
		args = append(args, query.SignalDate)
	}
	if strings.TrimSpace(query.Symbol) != "" {
		conditions = append(conditions, "symbol=?")
		args = append(args, strings.ToUpper(strings.TrimSpace(query.Symbol)))
	}
	where := strings.Join(conditions, " AND ")
	page := HistoricalBacktestSignalPage{Items: []HistoricalBacktestSignal{}, Limit: query.Limit, Offset: query.Offset}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM historical_backtest_signals WHERE `+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	listArgs := append(append([]interface{}{}, args...), query.Limit, query.Offset)
	rows, err := s.db.Query(`SELECT id,run_id,strategy_id,strategy_name,signal_date,symbol,latest,score,detail_json,tracking_json,created_at
		FROM historical_backtest_signals WHERE `+where+` ORDER BY signal_date DESC,strategy_name,symbol LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item HistoricalBacktestSignal
		if err := rows.Scan(&item.ID, &item.RunID, &item.StrategyID, &item.StrategyName, &item.SignalDate, &item.Symbol,
			&item.Latest, &item.Score, &item.DetailJSON, &item.TrackingJSON, &item.CreatedAt); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

// HistoricalBacktestTrade 是组合模拟的一条交易行：已平仓记录带完整买卖字段，
// 回测结束时未平仓为 open，信号未成交记录为 skipped。
type HistoricalBacktestTrade struct {
	ID           string  `json:"id"`
	RunID        string  `json:"run_id"`
	Symbol       string  `json:"symbol"`
	Status       string  `json:"status"`
	EntryDate    string  `json:"entry_date"`
	EntryPrice   float64 `json:"entry_price"`
	Shares       int     `json:"shares"`
	ExitDate     string  `json:"exit_date"`
	ExitPrice    float64 `json:"exit_price"`
	Pnl          float64 `json:"pnl"`
	PnlRate      float64 `json:"pnl_rate"`
	HoldDays     int     `json:"hold_days"`
	StrategyName string  `json:"strategy_name"`
	SignalDate   string  `json:"signal_date"`
	Reason       string  `json:"reason"`
	CreatedAt    string  `json:"created_at"`
}

type HistoricalBacktestTradeQuery struct {
	Status string
	Symbol string
	Limit  int
	Offset int
}

type HistoricalBacktestTradePage struct {
	Items  []HistoricalBacktestTrade `json:"items"`
	Total  int                       `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

func (s *AppStore) InsertHistoricalBacktestTrades(items []HistoricalBacktestTrade) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO historical_backtest_trades
		(id,run_id,symbol,status,entry_date,entry_price,shares,exit_date,exit_price,pnl,pnl_rate,hold_days,strategy_name,signal_date,reason,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		if item.CreatedAt == "" {
			item.CreatedAt = NowText()
		}
		if _, err := stmt.Exec(item.ID, item.RunID, item.Symbol, item.Status, item.EntryDate, item.EntryPrice, item.Shares,
			item.ExitDate, item.ExitPrice, item.Pnl, item.PnlRate, item.HoldDays, item.StrategyName, item.SignalDate, item.Reason, item.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppStore) ListHistoricalBacktestTrades(runID string, query HistoricalBacktestTradeQuery) (HistoricalBacktestTradePage, error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	conditions := []string{"run_id=?"}
	args := []interface{}{runID}
	if strings.TrimSpace(query.Status) != "" {
		conditions = append(conditions, "status=?")
		args = append(args, query.Status)
	}
	if strings.TrimSpace(query.Symbol) != "" {
		conditions = append(conditions, "symbol=?")
		args = append(args, strings.ToUpper(strings.TrimSpace(query.Symbol)))
	}
	where := strings.Join(conditions, " AND ")
	page := HistoricalBacktestTradePage{Items: []HistoricalBacktestTrade{}, Limit: query.Limit, Offset: query.Offset}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM historical_backtest_trades WHERE `+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	listArgs := append(append([]interface{}{}, args...), query.Limit, query.Offset)
	rows, err := s.db.Query(`SELECT id,run_id,symbol,status,entry_date,entry_price,shares,exit_date,exit_price,pnl,pnl_rate,hold_days,strategy_name,signal_date,reason,created_at
		FROM historical_backtest_trades WHERE `+where+` ORDER BY entry_date DESC,symbol LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item HistoricalBacktestTrade
		if err := rows.Scan(&item.ID, &item.RunID, &item.Symbol, &item.Status, &item.EntryDate, &item.EntryPrice, &item.Shares,
			&item.ExitDate, &item.ExitPrice, &item.Pnl, &item.PnlRate, &item.HoldDays, &item.StrategyName, &item.SignalDate, &item.Reason, &item.CreatedAt); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}
