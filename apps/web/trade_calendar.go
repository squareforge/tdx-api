package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/tdx"
	"github.com/robfig/cron/v3"
)

const (
	tradeCalendarMarket               = tdx.WorkdayMarketCN_A
	tradeCalendarSourcePMC            = tdx.WorkdaySourcePMC
	tradeCalendarSourceTDX            = tdx.WorkdaySourceTDX
	tradeCalendarConfidencePre        = tdx.WorkdayConfidencePre
	tradeCalendarConfidenceOK         = tdx.WorkdayConfidenceOK
	tradeCalendarDefaultFutureYears   = 2
	tradeCalendarMinFutureCoverageDay = 180
)

var (
	tradeCalendarMu            sync.Mutex
	tradeCalendarSchemaMu      sync.Mutex
	tradeCalendarSchemaReady   bool
	tradeCalendarSchedulerOnce sync.Once
)

type tradeCalendarRow struct {
	Date        string `json:"date"`
	ISODate     string `json:"iso_date"`
	Market      string `json:"market"`
	IsTradeDay  bool   `json:"is_trade_day"`
	Source      string `json:"source"`
	Confidence  string `json:"confidence"`
	OpenTime    string `json:"open_time,omitempty"`
	BreakStart  string `json:"break_start,omitempty"`
	BreakEnd    string `json:"break_end,omitempty"`
	CloseTime   string `json:"close_time,omitempty"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
	UpdatedAt   string `json:"updated_at"`
	Remark      string `json:"remark,omitempty"`
}

type tradeCalendarSyncRequest struct {
	Mode      string `json:"mode"`
	Market    string `json:"market"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Years     int    `json:"years"`
}

type tradeCalendarSyncResult struct {
	Mode             string                 `json:"mode"`
	Market           string                 `json:"market"`
	StartDate        string                 `json:"start_date"`
	EndDate          string                 `json:"end_date"`
	Upserted         int                    `json:"upserted"`
	Predicted        int                    `json:"predicted"`
	Confirmed        int                    `json:"confirmed"`
	ConfirmedThrough string                 `json:"confirmed_through,omitempty"`
	Source           string                 `json:"source"`
	Message          string                 `json:"message,omitempty"`
	Parts            map[string]interface{} `json:"parts,omitempty"`
}

type pmcCalendarDay struct {
	Date      string `json:"date"`
	IsTrade   bool   `json:"is_trade_day"`
	OpenTime  string `json:"open_time"`
	BreakFrom string `json:"break_start"`
	BreakTo   string `json:"break_end"`
	CloseTime string `json:"close_time"`
}

func tradeCalendarDBPath() string {
	if v := strings.TrimSpace(os.Getenv("TRADE_CALENDAR_DB")); v != "" {
		return v
	}
	return filepath.Join(tdx.DefaultDatabaseDir, "workday.db")
}

func openTradeCalendarDB() (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(tradeCalendarDBPath()), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", tradeCalendarDBPath())
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureTradeCalendarSchemaOnce(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureTradeCalendarSchemaOnce(db *sql.DB) error {
	if tradeCalendarSchemaReady {
		return nil
	}
	tradeCalendarSchemaMu.Lock()
	defer tradeCalendarSchemaMu.Unlock()
	if tradeCalendarSchemaReady {
		return nil
	}
	if err := ensureTradeCalendarSchema(db); err != nil {
		return err
	}
	tradeCalendarSchemaReady = true
	return nil
}

func ensureTradeCalendarSchema(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workday (
		ID INTEGER PRIMARY KEY AUTOINCREMENT,
		Unix INTEGER,
		Date TEXT
	)`); err != nil {
		return err
	}

	columns := map[string]string{
		"market":       "TEXT NOT NULL DEFAULT ''",
		"is_trade_day": "INTEGER NOT NULL DEFAULT 0",
		"source":       "TEXT NOT NULL DEFAULT ''",
		"confidence":   "TEXT NOT NULL DEFAULT ''",
		"open_time":    "TEXT NOT NULL DEFAULT ''",
		"break_start":  "TEXT NOT NULL DEFAULT ''",
		"break_end":    "TEXT NOT NULL DEFAULT ''",
		"close_time":   "TEXT NOT NULL DEFAULT ''",
		"confirmed_at": "TEXT NOT NULL DEFAULT ''",
		"updated_at":   "TEXT NOT NULL DEFAULT ''",
		"remark":       "TEXT NOT NULL DEFAULT ''",
	}
	for name, definition := range columns {
		exists, err := workdayColumnExists(db, name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE workday ADD COLUMN %s %s", name, definition)); err != nil {
				return err
			}
		}
	}
	if _, err := db.Exec(`UPDATE workday SET
		market = CASE WHEN market = '' THEN ? ELSE market END,
		is_trade_day = CASE WHEN is_trade_day IS NULL THEN 1 ELSE is_trade_day END,
		source = CASE WHEN source = '' THEN ? ELSE source END,
		confidence = CASE WHEN confidence = '' THEN ? ELSE confidence END,
		open_time = CASE WHEN open_time = '' THEN '09:30' ELSE open_time END,
		break_start = CASE WHEN break_start = '' THEN '11:30' ELSE break_start END,
		break_end = CASE WHEN break_end = '' THEN '13:00' ELSE break_end END,
		close_time = CASE WHEN close_time = '' THEN '15:00' ELSE close_time END,
		confirmed_at = CASE WHEN confirmed_at = '' THEN ? ELSE confirmed_at END,
		updated_at = CASE WHEN updated_at = '' THEN ? ELSE updated_at END,
		remark = CASE WHEN remark = '' THEN 'legacy workday seed' ELSE remark END
		WHERE Date IS NOT NULL AND Date != ''`,
		tradeCalendarMarket, tradeCalendarSourceTDX, tradeCalendarConfidenceOK, now, now); err != nil {
		return err
	}
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_workday_market_date ON workday (market, Date)`,
		`CREATE INDEX IF NOT EXISTS idx_workday_trade_day ON workday (market, is_trade_day, Date)`,
		`CREATE INDEX IF NOT EXISTS idx_workday_confidence ON workday (market, confidence)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return migrateTradeCalendarToWorkday(db)
}

func workdayColumnExists(db *sql.DB, name string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(workday)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var colName, colType string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(colName, name) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func migrateTradeCalendarToWorkday(db *sql.DB) error {
	rows, err := db.Query(`SELECT date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark FROM trade_calendar ORDER BY date`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO workday
		(Unix, Date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	updateStmt, err := tx.Prepare(`UPDATE workday SET
		Unix = ?,
		is_trade_day = ?,
		source = ?,
		confidence = ?,
		open_time = ?,
		break_start = ?,
		break_end = ?,
		close_time = ?,
		confirmed_at = ?,
		updated_at = ?,
		remark = ?
		WHERE Date = ? AND market = ? AND confidence != 'confirmed'`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()
	existsStmt, err := tx.Prepare(`SELECT COUNT(*) FROM workday WHERE Date = ? AND market = ?`)
	if err != nil {
		return err
	}
	defer existsStmt.Close()
	for rows.Next() {
		var row tradeCalendarRow
		var isTrade int
		if err := rows.Scan(&row.Date, &row.Market, &isTrade, &row.Source, &row.Confidence, &row.OpenTime, &row.BreakStart, &row.BreakEnd, &row.CloseTime, &row.ConfirmedAt, &row.UpdatedAt, &row.Remark); err != nil {
			return err
		}
		if row.Market == "" {
			row.Market = tradeCalendarMarket
		}
		unix := compactDateUnix(row.Date)
		if _, err := updateStmt.Exec(unix, isTrade, row.Source, row.Confidence, row.OpenTime, row.BreakStart, row.BreakEnd, row.CloseTime, row.ConfirmedAt, row.UpdatedAt, row.Remark, row.Date, row.Market); err != nil {
			return err
		}
		var exists int
		if err := existsStmt.QueryRow(row.Date, row.Market).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err := stmt.Exec(unix, row.Date, row.Market, isTrade, row.Source, row.Confidence, row.OpenTime, row.BreakStart, row.BreakEnd, row.CloseTime, row.ConfirmedAt, row.UpdatedAt, row.Remark); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func handleTradeCalendarSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	db, err := openTradeCalendarDB()
	if err != nil {
		errorResponse(w, "打开交易日历失败: "+err.Error())
		return
	}
	defer db.Close()
	summary, err := queryTradeCalendarSummary(db)
	if err != nil {
		errorResponse(w, "查询交易日历失败: "+err.Error())
		return
	}
	successResponse(w, summary)
}

func handleTradeCalendarRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	start, end, err := parseTradeCalendarRange(r)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	db, err := openTradeCalendarDB()
	if err != nil {
		errorResponse(w, "打开交易日历失败: "+err.Error())
		return
	}
	defer db.Close()
	rows, err := queryTradeCalendarRows(db, start, end)
	if err != nil {
		errorResponse(w, "查询交易日历失败: "+err.Error())
		return
	}
	successResponse(w, map[string]interface{}{
		"market":     tradeCalendarMarket,
		"start_date": start,
		"end_date":   end,
		"count":      len(rows),
		"list":       rows,
	})
}

func handleTradeCalendarSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req tradeCalendarSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = "all"
	}
	result, err := syncTradeCalendar(r.Context(), req)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, result)
}

func syncTradeCalendar(ctx context.Context, req tradeCalendarSyncRequest) (tradeCalendarSyncResult, error) {
	tradeCalendarMu.Lock()
	defer tradeCalendarMu.Unlock()

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case "", "all", "future", "predict", "predicted", "confirm", "tdx":
	default:
		return tradeCalendarSyncResult{}, fmt.Errorf("不支持的同步模式: %s", req.Mode)
	}
	if mode == "" {
		mode = "all"
	}
	if req.Market == "" {
		req.Market = tradeCalendarMarket
	}
	if req.Market != tradeCalendarMarket {
		return tradeCalendarSyncResult{}, fmt.Errorf("当前只支持 %s 交易日历", tradeCalendarMarket)
	}

	if mode == "all" {
		future, err := syncTradeCalendarFuture(ctx, req)
		if err != nil {
			return future, err
		}
		confirm, err := syncTradeCalendarConfirmed(req)
		result := tradeCalendarSyncResult{
			Mode:      "all",
			Market:    tradeCalendarMarket,
			StartDate: future.StartDate,
			EndDate:   future.EndDate,
			Upserted:  future.Upserted + confirm.Upserted,
			Predicted: future.Predicted,
			Confirmed: confirm.Confirmed,
			Source:    future.Source + "+" + confirm.Source,
			Parts: map[string]interface{}{
				"future":  future,
				"confirm": confirm,
			},
		}
		if confirm.ConfirmedThrough != "" {
			result.ConfirmedThrough = confirm.ConfirmedThrough
		}
		return result, err
	}
	if mode == "future" || mode == "predict" || mode == "predicted" {
		return syncTradeCalendarFuture(ctx, req)
	}
	return syncTradeCalendarConfirmed(req)
}

func syncTradeCalendarFuture(ctx context.Context, req tradeCalendarSyncRequest) (tradeCalendarSyncResult, error) {
	start, end, err := tradeCalendarSyncWindow(req, true)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	days, err := loadPMCCalendar(ctx, start, end)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	db, err := openTradeCalendarDB()
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	defer db.Close()
	upserted, tradeDays, err := upsertPredictedTradeCalendar(db, days)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	return tradeCalendarSyncResult{
		Mode:      "future",
		Market:    tradeCalendarMarket,
		StartDate: start,
		EndDate:   end,
		Upserted:  upserted,
		Predicted: tradeDays,
		Source:    tradeCalendarSourcePMC,
	}, nil
}

func syncTradeCalendarConfirmed(req tradeCalendarSyncRequest) (tradeCalendarSyncResult, error) {
	if manager == nil && strings.TrimSpace(os.Getenv("MARKET_SERVICE_URL")) != "" {
		return syncTradeCalendarConfirmedRemote(context.Background(), req)
	}
	if manager != nil && manager.Workday != nil {
		if err := manager.Workday.Update(); err != nil {
			return tradeCalendarSyncResult{}, err
		}
	}
	db, err := openTradeCalendarDB()
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	defer db.Close()
	start, end, err := tradeCalendarSyncWindow(req, false)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	workdays, maxWorkday, err := loadConfirmedWorkdays(db, start, end)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	if maxWorkday == "" {
		return tradeCalendarSyncResult{}, errors.New("本地 workday 表暂无可确认交易日")
	}
	if end > maxWorkday {
		end = maxWorkday
	}
	upserted, confirmed, err := upsertConfirmedTradeCalendar(db, start, end, workdays)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	return tradeCalendarSyncResult{
		Mode:             "confirm",
		Market:           tradeCalendarMarket,
		StartDate:        start,
		EndDate:          end,
		Upserted:         upserted,
		Confirmed:        confirmed,
		ConfirmedThrough: end,
		Source:           tradeCalendarSourceTDX,
	}, nil
}

func syncTradeCalendarConfirmedRemote(ctx context.Context, req tradeCalendarSyncRequest) (tradeCalendarSyncResult, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("MARKET_SERVICE_URL")), "/")
	if base == "" {
		return tradeCalendarSyncResult{}, errors.New("MARKET_SERVICE_URL 未配置，无法远程校验交易日历")
	}
	req.Mode = "confirm"
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/trade-calendar/sync", bytes.NewReader(body))
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	defer resp.Body.Close()
	var apiResp Response
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return tradeCalendarSyncResult{}, err
	}
	if apiResp.Code != 0 {
		return tradeCalendarSyncResult{}, errors.New(apiResp.Message)
	}
	raw, err := json.Marshal(apiResp.Data)
	if err != nil {
		return tradeCalendarSyncResult{}, err
	}
	var result tradeCalendarSyncResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return tradeCalendarSyncResult{}, err
	}
	result.Message = "confirmed by market-service"
	return result, nil
}

func tradeCalendarSyncWindow(req tradeCalendarSyncRequest, future bool) (string, string, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	end := start
	if future {
		years := req.Years
		if years <= 0 {
			years = 2
		}
		if years > 5 {
			years = 5
		}
		end = start.AddDate(years, 0, 0)
	} else {
		start = time.Date(1990, 12, 19, 0, 0, 0, 0, time.Local)
		if now.Hour() >= 16 {
			end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		} else {
			end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
		}
	}
	if strings.TrimSpace(req.StartDate) != "" {
		t, err := parseWorkdayDate(req.StartDate)
		if err != nil {
			return "", "", fmt.Errorf("start_date 格式错误，应为 YYYYMMDD 或 YYYY-MM-DD")
		}
		start = t
	}
	if strings.TrimSpace(req.EndDate) != "" {
		t, err := parseWorkdayDate(req.EndDate)
		if err != nil {
			return "", "", fmt.Errorf("end_date 格式错误，应为 YYYYMMDD 或 YYYY-MM-DD")
		}
		end = t
	}
	if end.Before(start) {
		return "", "", errors.New("end_date 必须大于或等于 start_date")
	}
	return start.Format("20060102"), end.Format("20060102"), nil
}

func upsertPredictedTradeCalendar(db *sql.DB, days []pmcCalendarDay) (int, int, error) {
	now := time.Now().Format(time.RFC3339)
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	updateStmt, err := tx.Prepare(`UPDATE workday SET
		Unix = ?,
		is_trade_day = ?,
		source = ?,
		confidence = ?,
		open_time = ?,
		break_start = ?,
		break_end = ?,
		close_time = ?,
		updated_at = ?
		WHERE Date = ? AND market = ? AND confidence != 'confirmed'`)
	if err != nil {
		return 0, 0, err
	}
	defer updateStmt.Close()
	insertStmt, err := tx.Prepare(`INSERT INTO workday
		(Unix, Date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, '')`)
	if err != nil {
		return 0, 0, err
	}
	defer insertStmt.Close()
	existsStmt, err := tx.Prepare(`SELECT COUNT(*) FROM workday WHERE Date = ? AND market = ?`)
	if err != nil {
		return 0, 0, err
	}
	defer existsStmt.Close()
	upserted := 0
	tradeDays := 0
	for _, day := range days {
		isTrade := 0
		if day.IsTrade {
			isTrade = 1
			tradeDays++
		}
		unix := compactDateUnix(day.Date)
		res, err := updateStmt.Exec(unix, isTrade, tradeCalendarSourcePMC, tradeCalendarConfidencePre, day.OpenTime, day.BreakFrom, day.BreakTo, day.CloseTime, now, day.Date, tradeCalendarMarket)
		if err != nil {
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			upserted += int(n)
			continue
		}
		var exists int
		if err := existsStmt.QueryRow(day.Date, tradeCalendarMarket).Scan(&exists); err != nil {
			return 0, 0, err
		}
		if exists == 0 {
			if _, err := insertStmt.Exec(unix, day.Date, tradeCalendarMarket, isTrade, tradeCalendarSourcePMC, tradeCalendarConfidencePre, day.OpenTime, day.BreakFrom, day.BreakTo, day.CloseTime, now); err != nil {
				return 0, 0, err
			}
			upserted++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return upserted, tradeDays, nil
}

func upsertConfirmedTradeCalendar(db *sql.DB, start, end string, workdays map[string]bool) (int, int, error) {
	startTime, _ := parseWorkdayDate(start)
	endTime, _ := parseWorkdayDate(end)
	now := time.Now().Format(time.RFC3339)
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	updateStmt, err := tx.Prepare(`UPDATE workday SET
		Unix = ?,
		is_trade_day = ?,
		source = ?,
		confidence = ?,
		open_time = ?,
		break_start = ?,
		break_end = ?,
		close_time = ?,
		confirmed_at = ?,
		updated_at = ?
		WHERE Date = ? AND market = ?`)
	if err != nil {
		return 0, 0, err
	}
	defer updateStmt.Close()
	insertStmt, err := tx.Prepare(`INSERT INTO workday
		(Unix, Date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`)
	if err != nil {
		return 0, 0, err
	}
	defer insertStmt.Close()
	upserted := 0
	confirmedTradeDays := 0
	for cursor := startTime; !cursor.After(endTime); cursor = cursor.AddDate(0, 0, 1) {
		date := cursor.Format("20060102")
		isTrade := 0
		openTime, breakStart, breakEnd, closeTime := "", "", "", ""
		if workdays[date] {
			isTrade = 1
			confirmedTradeDays++
			openTime, breakStart, breakEnd, closeTime = "09:30", "11:30", "13:00", "15:00"
		}
		unix := cursor.Add(15 * time.Hour).Unix()
		res, err := updateStmt.Exec(unix, isTrade, tradeCalendarSourceTDX, tradeCalendarConfidenceOK, openTime, breakStart, breakEnd, closeTime, now, now, date, tradeCalendarMarket)
		if err != nil {
			return 0, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			upserted += int(n)
			continue
		}
		if _, err := insertStmt.Exec(unix, date, tradeCalendarMarket, isTrade, tradeCalendarSourceTDX, tradeCalendarConfidenceOK, openTime, breakStart, breakEnd, closeTime, now, now); err != nil {
			return 0, 0, err
		}
		upserted++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return upserted, confirmedTradeDays, nil
}

func loadConfirmedWorkdays(db *sql.DB, start, end string) (map[string]bool, string, error) {
	result := map[string]bool{}
	rows, err := db.Query(`SELECT Date FROM workday WHERE market = ? AND is_trade_day = 1 AND Date BETWEEN ? AND ? ORDER BY Date`, tradeCalendarMarket, start, end)
	if err != nil {
		return result, "", err
	}
	defer rows.Close()
	maxDate := ""
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return result, "", err
		}
		result[date] = true
		if date > maxDate {
			maxDate = date
		}
	}
	if err := rows.Err(); err != nil {
		return result, "", err
	}
	if maxDate == "" {
		_ = db.QueryRow(`SELECT MAX(Date) FROM workday WHERE market = ? AND confidence = ?`, tradeCalendarMarket, tradeCalendarConfidenceOK).Scan(&maxDate)
	}
	return result, maxDate, nil
}

func loadPMCCalendar(ctx context.Context, start, end string) ([]pmcCalendarDay, error) {
	depsDir := filepath.Join("data", "python-deps")
	env := os.Environ()
	if stat, err := os.Stat(depsDir); err == nil && stat.IsDir() {
		env = append(env, "PYTHONPATH="+depsDir)
	}
	days, err := runPMCScript(ctx, env, start, end)
	if err == nil {
		return days, nil
	}
	if !envBool("TRADE_CALENDAR_AUTO_INSTALL_PY", false) {
		return nil, err
	}
	python := strings.TrimSpace(os.Getenv("TRADE_CALENDAR_PYTHON"))
	if python == "" {
		python = "python3"
	}
	if mkErr := os.MkdirAll(depsDir, 0755); mkErr != nil {
		return nil, fmt.Errorf("%w；创建 Python 依赖目录失败: %v", err, mkErr)
	}
	installCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	args := []string{"-m", "pip", "install", "-q", "--target", depsDir, "pandas_market_calendars"}
	cmd := exec.CommandContext(installCtx, python, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if installErr := cmd.Run(); installErr != nil {
		return nil, fmt.Errorf("%w；自动安装 pandas_market_calendars 失败: %v %s", err, installErr, strings.TrimSpace(stderr.String()))
	}
	env = append(os.Environ(), "PYTHONPATH="+depsDir)
	return runPMCScript(ctx, env, start, end)
}

func runPMCScript(ctx context.Context, env []string, start, end string) ([]pmcCalendarDay, error) {
	python := strings.TrimSpace(os.Getenv("TRADE_CALENDAR_PYTHON"))
	if python == "" {
		python = "python3"
	}
	script := `
import json
import sys
from datetime import date, timedelta

try:
    import pandas_market_calendars as mcal
except Exception as exc:
    raise SystemExit("IMPORT_ERROR: %s" % exc)

start = sys.argv[1]
end = sys.argv[2]
start_iso = "%s-%s-%s" % (start[0:4], start[4:6], start[6:8])
end_iso = "%s-%s-%s" % (end[0:4], end[4:6], end[6:8])
cal = mcal.get_calendar("SSE")
schedule = cal.schedule(start_date=start_iso, end_date=end_iso)

def fmt_ts(value):
    if value is None:
        return ""
    try:
        return value.tz_convert("Asia/Shanghai").strftime("%H:%M")
    except Exception:
        return ""

schedule_map = {}
for idx, row in schedule.iterrows():
    key = idx.strftime("%Y%m%d")
    schedule_map[key] = {
        "open_time": fmt_ts(row.get("market_open")),
        "break_start": fmt_ts(row.get("break_start")),
        "break_end": fmt_ts(row.get("break_end")),
        "close_time": fmt_ts(row.get("market_close")),
    }

cursor = date.fromisoformat(start_iso)
last = date.fromisoformat(end_iso)
items = []
while cursor <= last:
    key = cursor.strftime("%Y%m%d")
    times = schedule_map.get(key, {})
    items.append({
        "date": key,
        "is_trade_day": key in schedule_map,
        "open_time": times.get("open_time", ""),
        "break_start": times.get("break_start", ""),
        "break_end": times.get("break_end", ""),
        "close_time": times.get("close_time", ""),
    })
    cursor += timedelta(days=1)
print(json.dumps(items, ensure_ascii=False))
`
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, python, "-c", script, start, end)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if strings.Contains(msg, "IMPORT_ERROR") && strings.Contains(msg, "pandas_market_calendars") {
			return nil, errors.New("缺少 pandas_market_calendars，请执行 python3 -m pip install pandas_market_calendars，或设置 TRADE_CALENDAR_AUTO_INSTALL_PY=true 允许服务自动安装")
		}
		return nil, fmt.Errorf("执行 pandas_market_calendars 失败: %v %s", err, msg)
	}
	var days []pmcCalendarDay
	if err := json.Unmarshal(stdout.Bytes(), &days); err != nil {
		return nil, fmt.Errorf("解析 pandas_market_calendars 输出失败: %w", err)
	}
	return days, nil
}

func queryTradeCalendarSummary(db *sql.DB) (map[string]interface{}, error) {
	summary := map[string]interface{}{"market": tradeCalendarMarket}
	var minDate, maxDate, lastUpdated sql.NullString
	var total, tradeDays, nonTradeDays, predicted, confirmed int
	err := db.QueryRow(`SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN is_trade_day = 1 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN is_trade_day = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN confidence = 'predicted' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN confidence = 'confirmed' THEN 1 ELSE 0 END), 0),
		MIN(Date),
		MAX(Date),
		MAX(updated_at)
		FROM workday WHERE market = ?`, tradeCalendarMarket).
		Scan(&total, &tradeDays, &nonTradeDays, &predicted, &confirmed, &minDate, &maxDate, &lastUpdated)
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("20060102")
	var nextTrade sql.NullString
	_ = db.QueryRow(`SELECT Date FROM workday WHERE market = ? AND Date >= ? AND is_trade_day = 1 ORDER BY Date LIMIT 1`, tradeCalendarMarket, today).Scan(&nextTrade)
	var confirmedMax sql.NullString
	_ = db.QueryRow(`SELECT MAX(Date) FROM workday WHERE market = ? AND confidence = ?`, tradeCalendarMarket, tradeCalendarConfidenceOK).Scan(&confirmedMax)
	summary["total_days"] = total
	summary["trade_days"] = tradeDays
	summary["non_trade_days"] = nonTradeDays
	summary["predicted_days"] = predicted
	summary["confirmed_days"] = confirmed
	summary["start_date"] = minDate.String
	summary["end_date"] = maxDate.String
	summary["last_updated_at"] = lastUpdated.String
	summary["next_trade_day"] = nextTrade.String
	summary["confirmed_through"] = confirmedMax.String
	return summary, nil
}

func queryTradeCalendarRows(db *sql.DB, start, end string) ([]tradeCalendarRow, error) {
	rows, err := db.Query(`SELECT Date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark
		FROM workday
		WHERE market = ? AND Date BETWEEN ? AND ?
		ORDER BY Date`, tradeCalendarMarket, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []tradeCalendarRow{}
	for rows.Next() {
		var row tradeCalendarRow
		var isTrade int
		if err := rows.Scan(&row.Date, &row.Market, &isTrade, &row.Source, &row.Confidence, &row.OpenTime, &row.BreakStart, &row.BreakEnd, &row.CloseTime, &row.ConfirmedAt, &row.UpdatedAt, &row.Remark); err != nil {
			return nil, err
		}
		row.IsTradeDay = isTrade == 1
		row.ISODate = compactDateToISO(row.Date)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func isCalendarWorkday(target time.Time) (bool, error) {
	db, err := openTradeCalendarDB()
	if err != nil {
		if manager != nil && manager.Workday != nil {
			return manager.Workday.Is(target), nil
		}
		return false, err
	}
	defer db.Close()
	var isTrade int
	err = db.QueryRow(`SELECT is_trade_day FROM workday WHERE market = ? AND Date = ?`, tradeCalendarMarket, target.Format("20060102")).Scan(&isTrade)
	if err == sql.ErrNoRows {
		if manager != nil && manager.Workday != nil {
			return manager.Workday.Is(target), nil
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isTrade == 1, nil
}

func queryCalendarWorkdayRange(startDate, endDate time.Time) ([]map[string]string, error) {
	db, err := openTradeCalendarDB()
	if err != nil {
		if manager == nil || manager.Workday == nil {
			return nil, err
		}
		list := make([]map[string]string, 0)
		manager.Workday.Range(startDate, endDate.AddDate(0, 0, 1), func(t time.Time) bool {
			list = append(list, map[string]string{
				"iso":     t.Format("2006-01-02"),
				"numeric": t.Format("20060102"),
			})
			return true
		})
		return list, nil
	}
	defer db.Close()
	rows, err := db.Query(`SELECT Date FROM workday
		WHERE market = ? AND is_trade_day = 1 AND Date BETWEEN ? AND ?
		ORDER BY Date`, tradeCalendarMarket, startDate.Format("20060102"), endDate.Format("20060102"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := make([]map[string]string, 0)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		list = append(list, map[string]string{
			"iso":     compactDateToISO(date),
			"numeric": date,
		})
	}
	return list, rows.Err()
}

func parseTradeCalendarRange(r *http.Request) (string, string, error) {
	now := time.Now()
	start := now.AddDate(0, 0, -7)
	end := now.AddDate(0, 0, 60)
	if value := strings.TrimSpace(r.URL.Query().Get("start")); value != "" {
		t, err := parseWorkdayDate(value)
		if err != nil {
			return "", "", fmt.Errorf("start 参数格式错误，应为 YYYYMMDD 或 YYYY-MM-DD")
		}
		start = t
	}
	if value := strings.TrimSpace(r.URL.Query().Get("end")); value != "" {
		t, err := parseWorkdayDate(value)
		if err != nil {
			return "", "", fmt.Errorf("end 参数格式错误，应为 YYYYMMDD 或 YYYY-MM-DD")
		}
		end = t
	}
	if end.Before(start) {
		return "", "", errors.New("end 必须大于或等于 start")
	}
	return start.Format("20060102"), end.Format("20060102"), nil
}

func compactDateToISO(date string) string {
	if len(date) != 8 {
		return date
	}
	return date[:4] + "-" + date[4:6] + "-" + date[6:]
}

func compactDateUnix(date string) int64 {
	t, err := time.ParseInLocation("20060102", date, time.Local)
	if err != nil {
		return 0
	}
	return t.Add(15 * time.Hour).Unix()
}

func startTradeCalendarScheduler() {
	if !envBool("TRADE_CALENDAR_SYNC_ENABLED", true) {
		return
	}
	tradeCalendarSchedulerOnce.Do(func() {
		if envBool("TRADE_CALENDAR_STARTUP_SYNC_ENABLED", true) {
			go ensureTradeCalendarFutureCoverage()
		}
		cr := cron.New(cron.WithSeconds())
		if _, err := cr.AddFunc("0 30 8 * * *", func() {
			result, err := syncTradeCalendar(context.Background(), tradeCalendarSyncRequest{Mode: "future", Years: tradeCalendarDefaultFutureYears})
			if err != nil {
				log.Printf("同步未来交易日历失败: %v", err)
				return
			}
			log.Printf("同步未来交易日历完成: upserted=%d predicted=%d", result.Upserted, result.Predicted)
		}); err != nil {
			log.Printf("注册未来交易日历同步任务失败: %v", err)
		}
		if _, err := cr.AddFunc("0 0 16 * * *", func() {
			result, err := syncTradeCalendar(context.Background(), tradeCalendarSyncRequest{Mode: "confirm"})
			if err != nil {
				log.Printf("校验交易日历失败: %v", err)
				return
			}
			log.Printf("校验交易日历完成: through=%s confirmed=%d", result.ConfirmedThrough, result.Confirmed)
		}); err != nil {
			log.Printf("注册交易日历校验任务失败: %v", err)
		}
		cr.Start()
	})
}

func ensureTradeCalendarFutureCoverage() {
	db, err := openTradeCalendarDB()
	if err != nil {
		log.Printf("检查未来交易日历失败: %v", err)
		return
	}
	defer db.Close()

	var maxDate sql.NullString
	err = db.QueryRow(`SELECT MAX(Date) FROM workday WHERE market = ?`, tradeCalendarMarket).Scan(&maxDate)
	if err != nil {
		log.Printf("检查未来交易日历覆盖范围失败: %v", err)
		return
	}
	minCoverageEnd := time.Now().AddDate(0, 0, tradeCalendarMinFutureCoverageDay).Format("20060102")
	if maxDate.Valid && maxDate.String >= minCoverageEnd {
		return
	}

	result, err := syncTradeCalendar(context.Background(), tradeCalendarSyncRequest{
		Mode:  "future",
		Years: tradeCalendarDefaultFutureYears,
	})
	if err != nil {
		log.Printf("启动补全未来交易日历失败: %v", err)
		return
	}
	log.Printf("启动补全未来交易日历完成: range=%s-%s upserted=%d predicted=%d",
		result.StartDate, result.EndDate, result.Upserted, result.Predicted)
}
