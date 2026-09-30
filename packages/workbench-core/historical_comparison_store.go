package workbench

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// HistoricalComparison is an immutable analysis configuration with mutable job progress.
// The original backtest signals and trades are never changed by this job.
type HistoricalComparison struct {
	ID               string `json:"id"`
	RunID            string `json:"run_id"`
	StrategyID       string `json:"strategy_id"`
	Mode             string `json:"mode"`
	Status           string `json:"status"`
	ConfigJSON       string `json:"config_json"`
	Seed             int64  `json:"seed"`
	AlgorithmVersion string `json:"algorithm_version"`
	TotalEvents      int    `json:"total_events"`
	ProcessedEvents  int    `json:"processed_events"`
	CurrentDate      string `json:"current_date"`
	SummaryJSON      string `json:"summary_json"`
	Error            string `json:"error"`
	CancelRequested  bool   `json:"cancel_requested"`
	CreatedAt        string `json:"created_at"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	UpdatedAt        string `json:"updated_at"`
}

type HistoricalComparisonEvent struct {
	ID           string   `json:"id"`
	ComparisonID string   `json:"comparison_id"`
	SignalDate   string   `json:"signal_date"`
	SignalSymbol string   `json:"signal_symbol"`
	StrategyIDs  []string `json:"strategy_ids"`
	SignalIDs    []string `json:"signal_ids"`
	ControlCount int      `json:"control_count"`
	Status       string   `json:"status"`
	Reason       string   `json:"reason"`
	ResultJSON   string   `json:"result_json"`
	CreatedAt    string   `json:"created_at"`
}

type HistoricalComparisonEventPage struct {
	Items  []HistoricalComparisonEvent `json:"items"`
	Total  int                         `json:"total"`
	Limit  int                         `json:"limit"`
	Offset int                         `json:"offset"`
}

const historicalComparisonColumns = `id,run_id,strategy_id,mode,status,config_json,seed,algorithm_version,total_events,processed_events,current_date,summary_json,error,cancel_requested,created_at,started_at,finished_at,updated_at`

func scanHistoricalComparison(row interface{ Scan(...any) error }) (HistoricalComparison, error) {
	var item HistoricalComparison
	var cancel int
	err := row.Scan(&item.ID, &item.RunID, &item.StrategyID, &item.Mode, &item.Status, &item.ConfigJSON, &item.Seed,
		&item.AlgorithmVersion, &item.TotalEvents, &item.ProcessedEvents, &item.CurrentDate, &item.SummaryJSON,
		&item.Error, &cancel, &item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt)
	item.CancelRequested = cancel != 0
	return item, err
}

func (s *AppStore) CreateHistoricalComparison(item HistoricalComparison) (HistoricalComparison, error) {
	if item.RunID == "" {
		return item, errors.New("缺少回测 ID")
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.Status == "" {
		item.Status = "queued"
	}
	if item.Mode == "" {
		item.Mode = "strategy"
	}
	if item.AlgorithmVersion == "" {
		item.AlgorithmVersion = "same-day-v1"
	}
	if item.ConfigJSON == "" {
		item.ConfigJSON = "{}"
	}
	if item.SummaryJSON == "" {
		item.SummaryJSON = "{}"
	}
	item.CreatedAt = NowText()
	item.UpdatedAt = item.CreatedAt
	_, err := s.db.Exec(`INSERT INTO historical_backtest_comparisons (`+historicalComparisonColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.RunID, item.StrategyID, item.Mode, item.Status, item.ConfigJSON, item.Seed, item.AlgorithmVersion,
		item.TotalEvents, item.ProcessedEvents, item.CurrentDate, item.SummaryJSON, item.Error, 0, item.CreatedAt, item.StartedAt, item.FinishedAt, item.UpdatedAt)
	return item, err
}

func (s *AppStore) GetHistoricalComparison(runID, id string) (HistoricalComparison, error) {
	return scanHistoricalComparison(s.db.QueryRow(`SELECT `+historicalComparisonColumns+` FROM historical_backtest_comparisons WHERE run_id=? AND id=?`, runID, id))
}

func (s *AppStore) ListHistoricalComparisons(runID string) ([]HistoricalComparison, error) {
	rows, err := s.db.Query(`SELECT `+historicalComparisonColumns+` FROM historical_backtest_comparisons WHERE run_id=? ORDER BY created_at DESC LIMIT 50`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []HistoricalComparison{}
	for rows.Next() {
		item, err := scanHistoricalComparison(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) FindRunningHistoricalComparison(runID string) (HistoricalComparison, error) {
	return scanHistoricalComparison(s.db.QueryRow(`SELECT `+historicalComparisonColumns+` FROM historical_backtest_comparisons WHERE run_id=? AND status IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, runID))
}

func (s *AppStore) UpdateHistoricalComparisonProgress(id string, total, done int, currentDate string) error {
	_, err := s.db.Exec(`UPDATE historical_backtest_comparisons SET status='running',total_events=?,processed_events=?,current_date=?,started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,updated_at=? WHERE id=? AND status IN ('queued','running')`, total, done, currentDate, NowText(), NowText(), id)
	return err
}

func (s *AppStore) FinishHistoricalComparison(id, status, summaryJSON, errorText string) error {
	if !json.Valid([]byte(summaryJSON)) {
		summaryJSON = "{}"
	}
	now := NowText()
	_, err := s.db.Exec(`UPDATE historical_backtest_comparisons SET status=?,summary_json=?,error=?,finished_at=?,updated_at=? WHERE id=?`, status, summaryJSON, errorText, now, now, id)
	return err
}

func (s *AppStore) RequestHistoricalComparisonCancel(runID, id string) error {
	result, err := s.db.Exec(`UPDATE historical_backtest_comparisons SET cancel_requested=1,updated_at=? WHERE run_id=? AND id=? AND status IN ('queued','running')`, NowText(), runID, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return errors.New("对照分析不存在或已结束")
	}
	return nil
}

func (s *AppStore) HistoricalComparisonCancelRequested(id string) bool {
	var cancel int
	return s.db.QueryRow(`SELECT cancel_requested FROM historical_backtest_comparisons WHERE id=?`, id).Scan(&cancel) == nil && cancel != 0
}

func (s *AppStore) InterruptHistoricalComparisons() error {
	_, err := s.db.Exec(`UPDATE historical_backtest_comparisons SET status='failed',error='服务重启，任务已中断；可重新发起分析',finished_at=?,updated_at=? WHERE status IN ('queued','running')`, NowText(), NowText())
	return err
}

func (s *AppStore) ListHistoricalComparisonSignals(runID, strategyID string) ([]HistoricalBacktestSignal, error) {
	query := `SELECT id,run_id,strategy_id,strategy_name,signal_date,symbol,latest,score,detail_json,created_at FROM historical_backtest_signals WHERE run_id=?`
	args := []any{runID}
	if strategyID != "" {
		query += ` AND strategy_id=?`
		args = append(args, strategyID)
	}
	query += ` ORDER BY signal_date,symbol,strategy_id,id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []HistoricalBacktestSignal{}
	for rows.Next() {
		var x HistoricalBacktestSignal
		if err := rows.Scan(&x.ID, &x.RunID, &x.StrategyID, &x.StrategyName, &x.SignalDate, &x.Symbol, &x.Latest, &x.Score, &x.DetailJSON, &x.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

func (s *AppStore) InsertHistoricalComparisonEvent(item HistoricalComparisonEvent) error {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.CreatedAt == "" {
		item.CreatedAt = NowText()
	}
	if item.ResultJSON == "" {
		item.ResultJSON = "{}"
	}
	strategies, _ := json.Marshal(item.StrategyIDs)
	signals, _ := json.Marshal(item.SignalIDs)
	_, err := s.db.Exec(`INSERT INTO historical_backtest_comparison_events (id,comparison_id,signal_date,signal_symbol,strategy_ids_json,signal_ids_json,control_count,status,reason,result_json,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ComparisonID, item.SignalDate, item.SignalSymbol, string(strategies), string(signals), item.ControlCount, item.Status, item.Reason, item.ResultJSON, item.CreatedAt)
	return err
}

func (s *AppStore) ListHistoricalComparisonEvents(comparisonID string, limit, offset int, onlyUnavailable bool) (HistoricalComparisonEventPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := `comparison_id=?`
	if onlyUnavailable {
		where += ` AND status<>'complete'`
	}
	page := HistoricalComparisonEventPage{Items: []HistoricalComparisonEvent{}, Limit: limit, Offset: offset}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM historical_backtest_comparison_events WHERE `+where, comparisonID).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := s.db.Query(`SELECT id,comparison_id,signal_date,signal_symbol,strategy_ids_json,signal_ids_json,control_count,status,reason,result_json,created_at FROM historical_backtest_comparison_events WHERE `+where+` ORDER BY signal_date,signal_symbol LIMIT ? OFFSET ?`, comparisonID, limit, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var x HistoricalComparisonEvent
		var strategyJSON, signalJSON string
		if err := rows.Scan(&x.ID, &x.ComparisonID, &x.SignalDate, &x.SignalSymbol, &strategyJSON, &signalJSON, &x.ControlCount, &x.Status, &x.Reason, &x.ResultJSON, &x.CreatedAt); err != nil {
			return page, err
		}
		_ = json.Unmarshal([]byte(strategyJSON), &x.StrategyIDs)
		_ = json.Unmarshal([]byte(signalJSON), &x.SignalIDs)
		page.Items = append(page.Items, x)
	}
	return page, rows.Err()
}

func (s *AppStore) HistoricalComparisonEventRows(comparisonID string) (*sql.Rows, error) {
	return s.db.Query(`SELECT signal_date,signal_symbol,strategy_ids_json,control_count,status,reason,result_json FROM historical_backtest_comparison_events WHERE comparison_id=? ORDER BY signal_date,signal_symbol`, strings.TrimSpace(comparisonID))
}
