package workbench

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/google/uuid"
	"github.com/injoyai/tdx"
)

type AppStore struct {
	db *sql.DB
}

type Formula struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Script      string `json:"script"`
	ArgsJSON    string `json:"args_json"`
	Period      string `json:"period"`
	Right       int    `json:"right"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type StockPool struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Symbols     []string `json:"symbols"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	System      bool     `json:"system"`
	Readonly    bool     `json:"readonly"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type Strategy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ConfigJSON  string `json:"config_json"`
	Enabled     bool   `json:"enabled"`
	Readonly    bool   `json:"readonly"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

const (
	DecisionWatchPoolID              = "watchlist"
	DecisionExcludePoolID            = "exclude"
	FixedCloseSyncTaskID             = "fixed-close-sync"
	FixedSelectionTrackingTaskID     = "fixed-selection-tracking"
	SystemStrategyDailyBatchTaskID   = "system-strategy-daily-batch"
	HistoricalBacktestTaskID         = "historical-backtest"
	HistoricalBacktestTaskName       = "历史选股回放"
	LegacyMarketInfoSyncTaskID       = "market-info-sync"
	LegacyMarketInfoSyncPayload      = `{"scope":"market_info","kinds":["long-tiger","hot_money","research","notice"],"max_codes":120,"continue_on_error":true}`
	MarketLongTigerSyncTaskID        = "market-long-tiger-sync"
	MarketHotMoneySyncTaskID         = "market-hot-money-sync"
	MarketResearchSyncTaskID         = "market-research-sync"
	MarketNoticeSyncTaskID           = "market-notice-sync"
	MarketIndustryResearchSyncTaskID = "market-industry-research-sync"
)

type AutomationTask struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Cron         string `json:"cron"`
	Enabled      bool   `json:"enabled"`
	OnlyTradeDay bool   `json:"only_trade_day"`
	PayloadJSON  string `json:"payload_json"`
	WebhookIDs   string `json:"webhook_ids"`
	LastRunAt    string `json:"last_run_at"`
	NextRunAt    string `json:"next_run_at"`
	LastStatus   string `json:"last_status"`
	LastMessage  string `json:"last_message"`
	CronEntryID  int    `json:"-"`
	Readonly     bool   `json:"readonly"`
	System       bool   `json:"system"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type AutomationRun struct {
	ID           string `json:"id"`
	ParentRunID  string `json:"parent_run_id"`
	TaskID       string `json:"task_id"`
	TaskName     string `json:"task_name"`
	TaskType     string `json:"task_type"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Log          string `json:"log"`
	ResultJSON   string `json:"result_json"`
	MatchedCount int    `json:"matched_count"`
}

type SelectionResult struct {
	ID          string  `json:"id"`
	RunID       string  `json:"run_id"`
	TaskID      string  `json:"task_id"`
	TaskName    string  `json:"task_name"`
	FormulaID   string  `json:"formula_id"`
	FormulaName string  `json:"formula_name"`
	Symbol      string  `json:"symbol"`
	Latest      float64 `json:"latest"`
	SignalAt    string  `json:"signal_at,omitempty"`
	SignalPrice float64 `json:"signal_price,omitempty"`
	QuoteAt     string  `json:"quote_at,omitempty"`
	QuoteSource string  `json:"quote_source,omitempty"`
	PriceType   string  `json:"price_type,omitempty"`
	QuoteAgeMS  int64   `json:"quote_age_ms,omitempty"`
	// NextQuote* are transient inputs for the follow-up paper execution
	// capture. They are deliberately not persisted as signal evidence.
	NextQuoteAt       string             `json:"-"`
	NextQuotePrice    float64            `json:"-"`
	NextQuoteSource   string             `json:"-"`
	SignalScore       float64            `json:"signal_score,omitempty"`
	StrategyVersion   string             `json:"strategy_version,omitempty"`
	MarketContextJSON string             `json:"market_context_json,omitempty"`
	SnapshotStatus    string             `json:"snapshot_status,omitempty"`
	DetailJSON        string             `json:"detail_json"`
	Tracking          *SelectionTracking `json:"tracking,omitempty"`
	CreatedAt         string             `json:"created_at"`
}

type SelectionResultTracking struct {
	ID                   string  `json:"id"`
	SelectionResultID    string  `json:"selection_result_id"`
	RunID                string  `json:"run_id"`
	TaskID               string  `json:"task_id"`
	TaskName             string  `json:"task_name"`
	FormulaID            string  `json:"formula_id"`
	FormulaName          string  `json:"formula_name"`
	Symbol               string  `json:"symbol"`
	SignalDate           int     `json:"signal_date"`
	BaseDate             int     `json:"base_date"`
	BasePrice            float64 `json:"base_price"`
	BasePriceFrom        string  `json:"base_price_from"`
	HorizonDays          int     `json:"horizon_days"`
	Status               string  `json:"status"`
	AsOfDate             int     `json:"as_of_date"`
	FinalDate            int     `json:"final_date"`
	FinalOpen            float64 `json:"final_open"`
	FinalClose           float64 `json:"final_close"`
	FinalHigh            float64 `json:"final_high"`
	FinalLow             float64 `json:"final_low"`
	AverageClose         float64 `json:"average_close"`
	AverageCloseReturn   float64 `json:"average_close_return"`
	OpenReturn           float64 `json:"open_return"`
	CloseReturn          float64 `json:"close_return"`
	HighReturn           float64 `json:"high_return"`
	LowReturn            float64 `json:"low_return"`
	MFE                  float64 `json:"mfe"`
	MAE                  float64 `json:"mae"`
	MaxPathDrawdown      float64 `json:"max_path_drawdown"`
	BenchmarkCloseReturn float64 `json:"benchmark_close_return"`
	ExcessCloseReturn    float64 `json:"excess_close_return"`
	GrossReturn          float64 `json:"gross_return"`
	NetReturn            float64 `json:"net_return"`
	TotalCost            float64 `json:"total_cost"`
	FirstEvent           string  `json:"first_event"`
	CalculationVersion   string  `json:"calculation_version"`
	DataQuality          string  `json:"data_quality"`
	MaxGain              float64 `json:"max_gain"`
	MaxGainDate          int     `json:"max_gain_date"`
	MaxGainPrice         float64 `json:"max_gain_price"`
	DaysToMaxGain        int     `json:"days_to_max_gain"`
	MaxDrawdown          float64 `json:"max_drawdown"`
	MaxDrawdownDate      int     `json:"max_drawdown_date"`
	MaxDrawdownPrice     float64 `json:"max_drawdown_price"`
	DaysToMaxDrawdown    int     `json:"days_to_max_drawdown"`
	TargetReturn         float64 `json:"target_return"`
	TargetHit            bool    `json:"target_hit"`
	TargetHitDate        int     `json:"target_hit_date"`
	DaysToTargetHit      int     `json:"days_to_target_hit"`
	DrawdownLimit        float64 `json:"drawdown_limit"`
	StopLossHit          bool    `json:"stop_loss_hit"`
	StopLossHitDate      int     `json:"stop_loss_hit_date"`
	DaysToStopLoss       int     `json:"days_to_stop_loss"`
	Success              bool    `json:"success"`
	Reason               string  `json:"reason"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type DecisionNote struct {
	Symbol          string  `json:"symbol"`
	Status          string  `json:"status"`
	AddedPrice      float64 `json:"added_price"`
	AddReason       string  `json:"add_reason"`
	PlanBuy         float64 `json:"plan_buy"`
	StopLoss        float64 `json:"stop_loss"`
	ReviewNote      string  `json:"review_note"`
	ExcludeCategory string  `json:"exclude_category"`
	ExcludeReason   string  `json:"exclude_reason"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type Webhook struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Method      string `json:"method"`
	HeadersJSON string `json:"headers_json"`
	Events      string `json:"events"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type PlatformConfig struct {
	ID        string `json:"id"`
	RiseColor string `json:"rise_color"`
	FallColor string `json:"fall_color"`
	UpdatedAt string `json:"updated_at"`
}

type TradingAccount struct {
	Principal         float64 `json:"principal"`
	TotalAssets       float64 `json:"totalAssets"`
	MarketValue       float64 `json:"marketValue"`
	DailyProfit       float64 `json:"dailyProfit"`
	MaxTradeRisk      float64 `json:"maxTradeRisk"`
	MaxPositionWeight float64 `json:"maxPositionWeight"`
}

type TradingDiscipline struct {
	Reason    bool `json:"reason"`
	Invalid   bool `json:"invalid"`
	Risk      bool `json:"risk"`
	NoImpulse bool `json:"noImpulse"`
}

type TradingFeeConfig struct {
	BuyCommissionRate  float64 `json:"buyCommissionRate"`
	SellCommissionRate float64 `json:"sellCommissionRate"`
	StampTaxRate       float64 `json:"stampTaxRate"`
	TransferFeeRate    float64 `json:"transferFeeRate"`
	MinCommission      float64 `json:"minCommission"`
}

type TradingTrade struct {
	ID            string  `json:"id"`
	StockName     string  `json:"stockName"`
	StockCode     string  `json:"stockCode"`
	Direction     string  `json:"direction"`
	Status        string  `json:"status"`
	EntryDate     string  `json:"entryDate"`
	EntryPrice    float64 `json:"entryPrice"`
	ExitDate      string  `json:"exitDate"`
	ExitPrice     float64 `json:"exitPrice"`
	ExitShares    float64 `json:"exitShares"`
	CurrentPrice  float64 `json:"currentPrice"`
	PreviousClose float64 `json:"previousClosePrice"`
	Shares        float64 `json:"shares"`
	InvalidPrice  float64 `json:"invalidPrice"`
	PositionLabel string  `json:"positionLabel"`
	TargetOne     string  `json:"targetOne"`
	TargetTwo     string  `json:"targetTwo"`
	TradeMode     string  `json:"tradeMode"`
	BuyReason     string  `json:"buyReason"`
	ExitRules     string  `json:"exitRules"`
	Review        string  `json:"review"`
	PlannedR      float64 `json:"plannedR"`
	ActualR       float64 `json:"actualR"`
	TimeStopDate  string  `json:"timeStopDate"`
	MarketGate    string  `json:"marketGate"`
}

type TradingSystemState struct {
	Account    TradingAccount    `json:"account"`
	Discipline TradingDiscipline `json:"discipline"`
	Fees       TradingFeeConfig  `json:"fees"`
	Filter     string            `json:"filter"`
	Trades     []TradingTrade    `json:"trades"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	UpdatedAt  string            `json:"updatedAt,omitempty"`
}

const TradingSystemStateID = "default"

func OpenAppStore() (*AppStore, error) {
	if err := os.MkdirAll(tdx.DefaultDatabaseDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(tdx.DefaultDatabaseDir, "automation.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=10000`); err != nil {
		db.Close()
		return nil, err
	}

	store := &AppStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.seedDefaults(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *AppStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *AppStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS formulas (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'indicator',
			script TEXT NOT NULL,
			args_json TEXT NOT NULL DEFAULT '[]',
			period TEXT NOT NULL DEFAULT 'day',
			right INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS stock_pools (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			symbols_json TEXT NOT NULL DEFAULT '[]',
			description TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automation_tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			cron TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			only_trade_day INTEGER NOT NULL DEFAULT 1,
			payload_json TEXT NOT NULL DEFAULT '{}',
			webhook_ids TEXT NOT NULL DEFAULT '[]',
			last_run_at TEXT NOT NULL DEFAULT '',
			next_run_at TEXT NOT NULL DEFAULT '',
			last_status TEXT NOT NULL DEFAULT '',
			last_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS strategies (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			config_json TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			readonly INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS automation_runs (
			id TEXT PRIMARY KEY,
			parent_run_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			task_name TEXT NOT NULL,
			task_type TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			log TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '{}',
			matched_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_runs_started_at ON automation_runs(started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_automation_runs_task_started_at ON automation_runs(task_id, started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS selection_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_name TEXT NOT NULL,
			formula_id TEXT NOT NULL,
			formula_name TEXT NOT NULL,
			symbol TEXT NOT NULL,
			latest REAL NOT NULL DEFAULT 0,
			signal_at TEXT NOT NULL DEFAULT '',
			signal_price REAL NOT NULL DEFAULT 0,
			quote_at TEXT NOT NULL DEFAULT '',
			quote_source TEXT NOT NULL DEFAULT '',
			price_type TEXT NOT NULL DEFAULT '',
			quote_age_ms INTEGER NOT NULL DEFAULT 0,
			signal_score REAL NOT NULL DEFAULT 0,
			strategy_version TEXT NOT NULL DEFAULT '',
			market_context_json TEXT NOT NULL DEFAULT '{}',
			snapshot_status TEXT NOT NULL DEFAULT 'missing',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_created_at ON selection_results(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_run_created_at ON selection_results(run_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_task_created_at ON selection_results(task_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_formula_created_at ON selection_results(formula_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_symbol_created_at ON selection_results(symbol, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_symbol ON selection_results(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_formula ON selection_results(formula_id)`,
		`CREATE TABLE IF NOT EXISTS selection_signal_executions (
			id TEXT PRIMARY KEY,
			selection_result_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			formula_id TEXT NOT NULL DEFAULT '',
			symbol TEXT NOT NULL,
			policy_name TEXT NOT NULL,
			policy_version TEXT NOT NULL DEFAULT '1',
			execution_version TEXT NOT NULL DEFAULT 'execution-v1',
			side TEXT NOT NULL DEFAULT 'buy',
			status TEXT NOT NULL DEFAULT 'planned',
			signal_at TEXT NOT NULL DEFAULT '',
			signal_price REAL NOT NULL DEFAULT 0,
			planned_at TEXT NOT NULL DEFAULT '',
			entry_at TEXT NOT NULL DEFAULT '',
			reference_price REAL NOT NULL DEFAULT 0,
			fill_price REAL NOT NULL DEFAULT 0,
			quantity INTEGER NOT NULL DEFAULT 0,
			slippage_bps REAL NOT NULL DEFAULT 0,
			gross_amount REAL NOT NULL DEFAULT 0,
			commission REAL NOT NULL DEFAULT 0,
			stamp_duty REAL NOT NULL DEFAULT 0,
			transfer_fee REAL NOT NULL DEFAULT 0,
			total_cost REAL NOT NULL DEFAULT 0,
			price_source TEXT NOT NULL DEFAULT '',
			failure_reason TEXT NOT NULL DEFAULT '',
			data_quality TEXT NOT NULL DEFAULT 'partial',
			strategy_version TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(selection_result_id, policy_name, policy_version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_signal_executions_status ON selection_signal_executions(status, planned_at)`,
		`CREATE INDEX IF NOT EXISTS idx_signal_executions_result ON selection_signal_executions(selection_result_id, policy_name)`,
		`CREATE TABLE IF NOT EXISTS selection_signal_observations (
			id TEXT PRIMARY KEY,
			selection_result_id TEXT NOT NULL,
			execution_id TEXT NOT NULL DEFAULT '',
			symbol TEXT NOT NULL,
			trade_date INTEGER NOT NULL,
			market_day_index INTEGER NOT NULL DEFAULT 0,
			symbol_day_index INTEGER NOT NULL DEFAULT 0,
			session_kind TEXT NOT NULL DEFAULT 'forward',
			open REAL NOT NULL DEFAULT 0,
			high REAL NOT NULL DEFAULT 0,
			low REAL NOT NULL DEFAULT 0,
			close REAL NOT NULL DEFAULT 0,
			volume REAL NOT NULL DEFAULT 0,
			amount REAL NOT NULL DEFAULT 0,
			signal_open_return REAL NOT NULL DEFAULT 0,
			signal_high_return REAL NOT NULL DEFAULT 0,
			signal_low_return REAL NOT NULL DEFAULT 0,
			signal_close_return REAL NOT NULL DEFAULT 0,
			trade_open_return REAL NOT NULL DEFAULT 0,
			trade_high_return REAL NOT NULL DEFAULT 0,
			trade_low_return REAL NOT NULL DEFAULT 0,
			trade_close_return REAL NOT NULL DEFAULT 0,
			benchmark_symbol TEXT NOT NULL DEFAULT '',
			benchmark_close_return REAL NOT NULL DEFAULT 0,
			excess_close_return REAL NOT NULL DEFAULT 0,
			adjustment_type TEXT NOT NULL DEFAULT '',
			data_status TEXT NOT NULL DEFAULT 'valid',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(selection_result_id, execution_id, trade_date, session_kind)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_signal_observations_result ON selection_signal_observations(selection_result_id, execution_id, symbol_day_index)`,
		`CREATE TABLE IF NOT EXISTS selection_result_tracking (
			id TEXT PRIMARY KEY,                         -- 主键，单条跟踪记录 ID
			selection_result_id TEXT NOT NULL,           -- 对应 selection_results.id，一条选股结果
			run_id TEXT NOT NULL,                        -- 对应 automation_runs.id，本次策略执行批次
			task_id TEXT NOT NULL,                       -- 自动化任务 ID
			task_name TEXT NOT NULL,                     -- 自动化任务名称
			formula_id TEXT NOT NULL,                    -- 策略 ID，策略选股时通常是 strategy:<strategy_id>
			formula_name TEXT NOT NULL,                  -- 策略名称
			symbol TEXT NOT NULL,                        -- 股票代码
			signal_date INTEGER NOT NULL DEFAULT 0,      -- 信号日期，格式 YYYYMMDD
			base_date INTEGER NOT NULL DEFAULT 0,        -- 实际用于计算收益的基准日期
			base_price REAL NOT NULL DEFAULT 0,          -- 收益计算基准价
			base_price_from TEXT NOT NULL DEFAULT '',    -- 基准价来源，例如 signal_day_close / signal_day_open
			horizon_days INTEGER NOT NULL,               -- 跟踪周期，比如 1、3、5、10
			status TEXT NOT NULL DEFAULT 'pending',      -- 跟踪状态：pending / complete / unavailable
			as_of_date INTEGER NOT NULL DEFAULT 0,       -- 当前已经跟踪到的日期
			final_date INTEGER NOT NULL DEFAULT 0,       -- 周期结束日期
			final_open REAL NOT NULL DEFAULT 0,          -- 周期结束日开盘价
			final_close REAL NOT NULL DEFAULT 0,         -- 周期结束日收盘价
			final_high REAL NOT NULL DEFAULT 0,
			final_low REAL NOT NULL DEFAULT 0,
			average_close REAL NOT NULL DEFAULT 0,
			average_close_return REAL NOT NULL DEFAULT 0,
			open_return REAL NOT NULL DEFAULT 0,         -- 周期结束日开盘收益率，百分比
			close_return REAL NOT NULL DEFAULT 0,        -- 周期结束日收盘收益率，百分比
			high_return REAL NOT NULL DEFAULT 0,
			low_return REAL NOT NULL DEFAULT 0,
			mfe REAL NOT NULL DEFAULT 0,
			mae REAL NOT NULL DEFAULT 0,
			max_path_drawdown REAL NOT NULL DEFAULT 0,
			benchmark_close_return REAL NOT NULL DEFAULT 0,
			excess_close_return REAL NOT NULL DEFAULT 0,
			gross_return REAL NOT NULL DEFAULT 0,
			net_return REAL NOT NULL DEFAULT 0,
			total_cost REAL NOT NULL DEFAULT 0,
			first_event TEXT NOT NULL DEFAULT 'none',
			calculation_version TEXT NOT NULL DEFAULT 'tracking-v3',
			data_quality TEXT NOT NULL DEFAULT 'complete',
			max_gain REAL NOT NULL DEFAULT 0,            -- 跟踪周期内最大涨幅，百分比
			max_gain_date INTEGER NOT NULL DEFAULT 0,    -- 最大涨幅发生日期
			max_gain_price REAL NOT NULL DEFAULT 0,      -- 最大涨幅对应价格
			days_to_max_gain INTEGER NOT NULL DEFAULT 0, -- 信号后第几天达到最大涨幅
			max_drawdown REAL NOT NULL DEFAULT 0,        -- 跟踪周期内最大回撤，百分比，通常为负数
			max_drawdown_date INTEGER NOT NULL DEFAULT 0,-- 最大回撤发生日期
			max_drawdown_price REAL NOT NULL DEFAULT 0,  -- 最大回撤对应价格
			days_to_max_drawdown INTEGER NOT NULL DEFAULT 0, -- 信号后第几天出现最大回撤
			target_return REAL NOT NULL DEFAULT 0,       -- 目标收益率，例如 3 表示 3%
			target_hit INTEGER NOT NULL DEFAULT 0,       -- 是否触达目标收益，0 否，1 是
			target_hit_date INTEGER NOT NULL DEFAULT 0,  -- 首次触达目标收益的日期
			days_to_target_hit INTEGER NOT NULL DEFAULT 0,-- 信号后第几天触达目标收益
			drawdown_limit REAL NOT NULL DEFAULT 0,      -- 最大允许回撤阈值，例如 5 表示 -5%
			stop_loss_hit INTEGER NOT NULL DEFAULT 0,    -- 是否触发止损/回撤限制，0 否，1 是
			stop_loss_hit_date INTEGER NOT NULL DEFAULT 0,-- 首次触发止损的日期
			days_to_stop_loss INTEGER NOT NULL DEFAULT 0,-- 信号后第几天触发止损
			success INTEGER NOT NULL DEFAULT 0,          -- 该周期是否成功，0 否，1 是
			reason TEXT NOT NULL DEFAULT '',             -- 结果说明，例如 达到收益与回撤标准 / 未达到收益与回撤标准
			created_at TEXT NOT NULL,                   -- 创建时间
			updated_at TEXT NOT NULL                    -- 更新时间
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_selection_result_tracking_unique ON selection_result_tracking(selection_result_id, horizon_days)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_result_tracking_symbol ON selection_result_tracking(symbol, signal_date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_result_tracking_strategy ON selection_result_tracking(formula_id, signal_date DESC, horizon_days)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_result_tracking_status ON selection_result_tracking(status, horizon_days, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_result_tracking_run ON selection_result_tracking(run_id, horizon_days)`,
		`CREATE TABLE IF NOT EXISTS historical_backtest_runs (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'pending',
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			strategy_ids_json TEXT NOT NULL DEFAULT '[]',
			strategy_snapshot_json TEXT NOT NULL DEFAULT '[]',
			horizons_json TEXT NOT NULL DEFAULT '[1,3,5,10]',
			target_return REAL NOT NULL DEFAULT 3,
			drawdown_limit REAL NOT NULL DEFAULT 5,
			buy_base TEXT NOT NULL DEFAULT 'open',
			buy_offset REAL NOT NULL DEFAULT 0,
			sell_base TEXT NOT NULL DEFAULT 'next_open',
			take_profit REAL NOT NULL DEFAULT 10,
			trail_drawdown REAL NOT NULL DEFAULT 5,
			stop_loss REAL NOT NULL DEFAULT 3,
			max_hold_days INTEGER NOT NULL DEFAULT 15,
			total_dates INTEGER NOT NULL DEFAULT 0,
			processed_dates INTEGER NOT NULL DEFAULT 0,
			current_date TEXT NOT NULL DEFAULT '',
			candidate_symbols INTEGER NOT NULL DEFAULT 0,
			signal_count INTEGER NOT NULL DEFAULT 0,
			result_json TEXT NOT NULL DEFAULT '{}',
			error TEXT NOT NULL DEFAULT '',
			cancel_requested INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_runs_created_at ON historical_backtest_runs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS historical_scan_runs (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'running',
			start_date TEXT NOT NULL,
			end_date TEXT NOT NULL,
			strategy_ids_json TEXT NOT NULL DEFAULT '[]',
			total_strategies INTEGER NOT NULL DEFAULT 0,
			processed_strategies INTEGER NOT NULL DEFAULT 0,
			current_strategy TEXT NOT NULL DEFAULT '',
			signal_count INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_scan_runs_created_at ON historical_scan_runs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS historical_scan_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			strategy_id TEXT NOT NULL,
			strategy_name TEXT NOT NULL,
			buy_base TEXT NOT NULL,
			sell_base TEXT NOT NULL,
			take_profit REAL NOT NULL,
			trail_drawdown REAL NOT NULL,
			stop_loss REAL NOT NULL,
			max_hold_days INTEGER NOT NULL,
			total_return REAL NOT NULL DEFAULT 0,
			max_drawdown REAL NOT NULL DEFAULT 0,
			win_rate REAL NOT NULL DEFAULT 0,
			closed_count INTEGER NOT NULL DEFAULT 0,
			open_count INTEGER NOT NULL DEFAULT 0,
			final_equity REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_scan_results_run_strategy ON historical_scan_results(run_id, strategy_id)`,
		`CREATE TABLE IF NOT EXISTS historical_backtest_signals (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			strategy_id TEXT NOT NULL,
			strategy_name TEXT NOT NULL,
			signal_date TEXT NOT NULL,
			symbol TEXT NOT NULL,
			latest REAL NOT NULL DEFAULT 0,
			score REAL NOT NULL DEFAULT 0,
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_signals_run_date ON historical_backtest_signals(run_id, signal_date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_signals_run_strategy ON historical_backtest_signals(run_id, strategy_id)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_signals_run_symbol ON historical_backtest_signals(run_id, symbol)`,
		`CREATE TABLE IF NOT EXISTS historical_backtest_trades (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			symbol TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			entry_date TEXT NOT NULL DEFAULT '',
			entry_price REAL NOT NULL DEFAULT 0,
			shares INTEGER NOT NULL DEFAULT 0,
			exit_date TEXT NOT NULL DEFAULT '',
			exit_price REAL NOT NULL DEFAULT 0,
			pnl REAL NOT NULL DEFAULT 0,
			pnl_rate REAL NOT NULL DEFAULT 0,
			hold_days INTEGER NOT NULL DEFAULT 0,
			strategy_name TEXT NOT NULL DEFAULT '',
			signal_date TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_trades_run_date ON historical_backtest_trades(run_id, entry_date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_trades_run_status ON historical_backtest_trades(run_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_historical_backtest_trades_run_symbol ON historical_backtest_trades(run_id, symbol)`,
		`CREATE TABLE IF NOT EXISTS decision_notes (
			symbol TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT '',
			added_price REAL NOT NULL DEFAULT 0,
			add_reason TEXT NOT NULL DEFAULT '',
			plan_buy REAL NOT NULL DEFAULT 0,
			stop_loss REAL NOT NULL DEFAULT 0,
			review_note TEXT NOT NULL DEFAULT '',
			exclude_category TEXT NOT NULL DEFAULT '',
			exclude_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_decision_notes_status ON decision_notes(status)`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			method TEXT NOT NULL DEFAULT 'POST',
			headers_json TEXT NOT NULL DEFAULT '{}',
				events TEXT NOT NULL DEFAULT '["automation.failed","stock_selection.finished","t-signal.triggered"]',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS platform_config (
			id TEXT PRIMARY KEY,
			rise_color TEXT NOT NULL DEFAULT '#c93d35',
			fall_color TEXT NOT NULL DEFAULT '#16805b',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS trading_system_state (
			id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS t_signal_state (
			id TEXT PRIMARY KEY,
			state_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS macro_events (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'macro',
			country TEXT NOT NULL DEFAULT 'US',
			impact TEXT NOT NULL DEFAULT 'medium',
			starts_at TEXT NOT NULL,
			scheduled_at TEXT NOT NULL DEFAULT '',
			released_at TEXT NOT NULL DEFAULT '',
			previous_value TEXT NOT NULL DEFAULT '',
			forecast_value TEXT NOT NULL DEFAULT '',
			actual_value TEXT NOT NULL DEFAULT '',
			revision TEXT NOT NULL DEFAULT '',
			a_share_date TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			source_url TEXT NOT NULL DEFAULT '',
			source_kind TEXT NOT NULL DEFAULT 'manual',
			external_id TEXT NOT NULL DEFAULT '',
			last_seen_at TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'scheduled',
			acknowledged INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_macro_events_starts_at ON macro_events(starts_at)`,
		`CREATE INDEX IF NOT EXISTS idx_macro_events_impact ON macro_events(impact)`,
		`CREATE TABLE IF NOT EXISTS macro_event_sync_state (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'idle',
			last_attempt_at TEXT NOT NULL DEFAULT '',
			last_success_at TEXT NOT NULL DEFAULT '',
			event_count INTEGER NOT NULL DEFAULT 0,
			message TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS macro_alert_settings (
			id TEXT PRIMARY KEY,
			enabled INTEGER NOT NULL DEFAULT 1,
			lead_minutes INTEGER NOT NULL DEFAULT 240,
			window_before_minutes INTEGER NOT NULL DEFAULT 240,
			window_after_minutes INTEGER NOT NULL DEFAULT 120,
			critical_only INTEGER NOT NULL DEFAULT 0,
			notify_webhooks INTEGER NOT NULL DEFAULT 0,
			webhook_ids TEXT NOT NULL DEFAULT '[]',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS macro_alert_deliveries (
			event_id TEXT NOT NULL,
			webhook_id TEXT NOT NULL,
			alert_kind TEXT NOT NULL,
			sent_at TEXT NOT NULL,
			PRIMARY KEY(event_id, webhook_id, alert_kind)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_macro_alert_deliveries_sent_at ON macro_alert_deliveries(sent_at)`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Existing workbench databases predate the extended event and notification fields.
	for table, columns := range map[string]map[string]string{
		"selection_results": {
			"signal_at": "TEXT NOT NULL DEFAULT ''", "signal_price": "REAL NOT NULL DEFAULT 0",
			"quote_at": "TEXT NOT NULL DEFAULT ''", "quote_source": "TEXT NOT NULL DEFAULT ''",
			"price_type": "TEXT NOT NULL DEFAULT ''", "quote_age_ms": "INTEGER NOT NULL DEFAULT 0",
			"signal_score": "REAL NOT NULL DEFAULT 0", "strategy_version": "TEXT NOT NULL DEFAULT ''",
			"market_context_json": "TEXT NOT NULL DEFAULT '{}'", "snapshot_status": "TEXT NOT NULL DEFAULT 'missing'",
		},
		"selection_result_tracking": {
			"final_high": "REAL NOT NULL DEFAULT 0", "final_low": "REAL NOT NULL DEFAULT 0",
			"average_close": "REAL NOT NULL DEFAULT 0", "average_close_return": "REAL NOT NULL DEFAULT 0",
			"high_return": "REAL NOT NULL DEFAULT 0", "low_return": "REAL NOT NULL DEFAULT 0",
			"mfe": "REAL NOT NULL DEFAULT 0", "mae": "REAL NOT NULL DEFAULT 0",
			"max_path_drawdown": "REAL NOT NULL DEFAULT 0", "benchmark_close_return": "REAL NOT NULL DEFAULT 0",
			"excess_close_return": "REAL NOT NULL DEFAULT 0", "gross_return": "REAL NOT NULL DEFAULT 0",
			"net_return": "REAL NOT NULL DEFAULT 0", "total_cost": "REAL NOT NULL DEFAULT 0",
			"first_event": "TEXT NOT NULL DEFAULT 'none'", "calculation_version": "TEXT NOT NULL DEFAULT 'tracking-v3'",
			"data_quality": "TEXT NOT NULL DEFAULT 'complete'",
		},
		"macro_events": {
			"scheduled_at": "TEXT NOT NULL DEFAULT ''", "released_at": "TEXT NOT NULL DEFAULT ''",
			"previous_value": "TEXT NOT NULL DEFAULT ''", "forecast_value": "TEXT NOT NULL DEFAULT ''",
			"actual_value": "TEXT NOT NULL DEFAULT ''", "revision": "TEXT NOT NULL DEFAULT ''",
			"source_kind": "TEXT NOT NULL DEFAULT 'manual'", "external_id": "TEXT NOT NULL DEFAULT ''",
			"last_seen_at": "TEXT NOT NULL DEFAULT ''",
		},
		"macro_alert_settings": {"notify_webhooks": "INTEGER NOT NULL DEFAULT 0", "webhook_ids": "TEXT NOT NULL DEFAULT '[]'"},
		"automation_runs":      {"parent_run_id": "TEXT NOT NULL DEFAULT ''"},
		"automation_tasks":     {"only_trade_day": "INTEGER NOT NULL DEFAULT 1"},
		"historical_backtest_runs": {
			"buy_base": "TEXT NOT NULL DEFAULT 'open'", "buy_offset": "REAL NOT NULL DEFAULT 0",
			"sell_base": "TEXT NOT NULL DEFAULT 'next_open'", "take_profit": "REAL NOT NULL DEFAULT 10",
			"trail_drawdown": "REAL NOT NULL DEFAULT 5", "stop_loss": "REAL NOT NULL DEFAULT 3",
			"max_hold_days": "INTEGER NOT NULL DEFAULT 15",
		},
	} {
		for column, definition := range columns {
			if err := s.ensureColumn(table, column, definition); err != nil {
				return err
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE macro_events
			SET source_kind='official', external_id=CASE WHEN TRIM(external_id)='' THEN id ELSE external_id END
			WHERE id LIKE 'macro-official-%' AND (TRIM(source_kind)='' OR source_kind='manual')`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_automation_runs_parent_started_at ON automation_runs(parent_run_id, started_at)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_selection_results_signal_at ON selection_results(signal_at DESC)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_macro_events_source_external ON macro_events(source_kind, source, external_id)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE selection_results SET
		signal_at=CASE WHEN TRIM(signal_at)='' THEN created_at ELSE signal_at END,
		signal_price=CASE WHEN signal_price<=0 THEN latest ELSE signal_price END,
		price_type=CASE WHEN TRIM(price_type)='' THEN 'latest' ELSE price_type END,
		strategy_version=CASE WHEN TRIM(strategy_version)='' THEN formula_id ELSE strategy_version END,
		market_context_json=CASE WHEN TRIM(market_context_json)='' THEN '{}' ELSE market_context_json END,
		snapshot_status=CASE
			WHEN latest<=0 THEN 'missing'
			WHEN TRIM(quote_at)='' OR TRIM(quote_source)='' THEN 'inferred'
			WHEN TRIM(snapshot_status)='' THEN 'inferred'
			ELSE snapshot_status
		END
		WHERE TRIM(signal_at)='' OR signal_price<=0 OR TRIM(price_type)='' OR TRIM(strategy_version)='' OR TRIM(market_context_json)='' OR TRIM(snapshot_status)=''`); err != nil {
		return err
	}
	// Older releases used these internal source labels while copying
	// created_at into quote_at. Remove that synthetic evidence so legacy rows
	// remain visibly inferred instead of looking like real-time snapshots.
	if _, err := s.db.Exec(`UPDATE selection_results
		SET quote_at='', quote_source='', snapshot_status=CASE WHEN latest>0 THEN 'inferred' ELSE 'missing' END
		WHERE snapshot_status='inferred' AND quote_source IN ('legacy_selection','selection_result')`); err != nil {
		return err
	}
	if err := s.migrateSelectionResultBusinessKey(); err != nil {
		return err
	}
	// Runs created before parent_run_id existed are linked to the enclosing
	// system-strategy batch so historical daily reviews remain available.
	if _, err := s.db.Exec(`UPDATE automation_runs AS child
		SET parent_run_id=COALESCE((
			SELECT parent.id FROM automation_runs AS parent
			WHERE parent.task_id=?
				AND parent.started_at<=child.started_at
				AND (parent.finished_at='' OR child.started_at<=parent.finished_at)
			ORDER BY parent.started_at DESC LIMIT 1
		), '')
		WHERE child.task_id LIKE 'system-strategy:%' AND child.parent_run_id=''`, SystemStrategyDailyBatchTaskID); err != nil {
		return err
	}
	if err := s.migrateLegacySelectionTracking(); err != nil {
		return err
	}
	if err := s.migrateHistoricalBacktestTracking(); err != nil {
		return err
	}
	return nil
}

// migrateSelectionResultBusinessKey makes the signal business key unique on
// databases that already contain rows from the pre-constraint implementation.
// A retry could previously write the same signal twice. Keep the earliest row
// as the canonical signal and fold structured tracking/execution observations
// into it before removing the duplicate row.
func (s *AppStore) migrateSelectionResultBusinessKey() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT sr.run_id,sr.formula_id,sr.symbol,sr.signal_at,
			(SELECT canonical.id FROM selection_results AS canonical
			 WHERE canonical.run_id=sr.run_id AND canonical.formula_id=sr.formula_id
			   AND canonical.symbol=sr.symbol AND canonical.signal_at=sr.signal_at
			 ORDER BY canonical.created_at,canonical.id LIMIT 1) AS canonical_id
		FROM selection_results AS sr
		GROUP BY sr.run_id,sr.formula_id,sr.symbol,sr.signal_at
		HAVING COUNT(*)>1`)
	if err != nil {
		return err
	}
	type duplicateGroup struct {
		runID, formulaID, symbol, signalAt, canonicalID string
	}
	groups := make([]duplicateGroup, 0)
	for rows.Next() {
		var group duplicateGroup
		if err := rows.Scan(&group.runID, &group.formulaID, &group.symbol, &group.signalAt, &group.canonicalID); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, group := range groups {
		var duplicateIDs []string
		duplicateRows, err := tx.Query(`SELECT id FROM selection_results WHERE run_id=? AND formula_id=? AND symbol=? AND signal_at=? AND id<>? ORDER BY created_at,id`, group.runID, group.formulaID, group.symbol, group.signalAt, group.canonicalID)
		if err != nil {
			return err
		}
		for duplicateRows.Next() {
			var id string
			if err := duplicateRows.Scan(&id); err != nil {
				duplicateRows.Close()
				return err
			}
			duplicateIDs = append(duplicateIDs, id)
		}
		if err := duplicateRows.Err(); err != nil {
			duplicateRows.Close()
			return err
		}
		if err := duplicateRows.Close(); err != nil {
			return err
		}

		for _, duplicateID := range duplicateIDs {
			// Merge each horizon one at a time so duplicate rows for the same
			// horizon cannot collide on selection_result_tracking's primary key.
			var trackingRows []struct {
				id      string
				horizon int
			}
			rows, err := tx.Query(`SELECT id,horizon_days FROM selection_result_tracking WHERE selection_result_id=? ORDER BY horizon_days,id`, duplicateID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var row struct {
					id      string
					horizon int
				}
				if err := rows.Scan(&row.id, &row.horizon); err != nil {
					rows.Close()
					return err
				}
				trackingRows = append(trackingRows, row)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, row := range trackingRows {
				var kept int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days=?`, group.canonicalID, row.horizon).Scan(&kept); err != nil {
					return err
				}
				if kept > 0 {
					if _, err := tx.Exec(`DELETE FROM selection_result_tracking WHERE id=?`, row.id); err != nil {
						return err
					}
				} else if _, err := tx.Exec(`UPDATE selection_result_tracking SET selection_result_id=?,id=? WHERE id=?`, group.canonicalID, fmt.Sprintf("%s:d%d", group.canonicalID, row.horizon), row.id); err != nil {
					return err
				}
			}

			var executionRows []struct{ id, policy, version string }
			rows, err = tx.Query(`SELECT id,policy_name,policy_version FROM selection_signal_executions WHERE selection_result_id=?`, duplicateID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var row struct{ id, policy, version string }
				if err := rows.Scan(&row.id, &row.policy, &row.version); err != nil {
					rows.Close()
					return err
				}
				executionRows = append(executionRows, row)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, row := range executionRows {
				var kept int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM selection_signal_executions WHERE selection_result_id=? AND policy_name=? AND policy_version=?`, group.canonicalID, row.policy, row.version).Scan(&kept); err != nil {
					return err
				}
				if kept > 0 {
					if _, err := tx.Exec(`DELETE FROM selection_signal_executions WHERE id=?`, row.id); err != nil {
						return err
					}
				} else if _, err := tx.Exec(`UPDATE selection_signal_executions SET selection_result_id=? WHERE id=?`, group.canonicalID, row.id); err != nil {
					return err
				}
			}

			var observationRows []struct {
				id, executionID string
				tradeDate       int
				sessionKind     string
			}
			rows, err = tx.Query(`SELECT id,execution_id,trade_date,session_kind FROM selection_signal_observations WHERE selection_result_id=?`, duplicateID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var row struct {
					id, executionID string
					tradeDate       int
					sessionKind     string
				}
				if err := rows.Scan(&row.id, &row.executionID, &row.tradeDate, &row.sessionKind); err != nil {
					rows.Close()
					return err
				}
				observationRows = append(observationRows, row)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, row := range observationRows {
				var kept int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM selection_signal_observations WHERE selection_result_id=? AND execution_id=? AND trade_date=? AND session_kind=?`, group.canonicalID, row.executionID, row.tradeDate, row.sessionKind).Scan(&kept); err != nil {
					return err
				}
				if kept > 0 {
					if _, err := tx.Exec(`DELETE FROM selection_signal_observations WHERE id=?`, row.id); err != nil {
						return err
					}
				} else if _, err := tx.Exec(`UPDATE selection_signal_observations SET selection_result_id=? WHERE id=?`, group.canonicalID, row.id); err != nil {
					return err
				}
			}

			if _, err := tx.Exec(`DELETE FROM selection_results WHERE id=?`, duplicateID); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_selection_results_business_key ON selection_results(run_id, formula_id, symbol, signal_at)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AppStore) ensureColumn(table, column, definition string) error {
	exists, err := s.tableHasColumn(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func (s *AppStore) tableHasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (s *AppStore) seedDefaults() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM formulas`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		now := NowText()
		_, err := s.db.Exec(`INSERT INTO formulas
			(id,name,type,script,args_json,period,right,enabled,description,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			uuid.NewString(), "5日上穿20日均线", "selection",
			"CROSS(MA(C,5),MA(C,20));", "[]", "day", 1, 1,
			"示例选股公式：短期均线上穿长期均线。", now, now)
		if err != nil {
			return err
		}
	}
	now := NowText()
	defaultFormulas := []Formula{
		{
			ID: "default-ma-overlay", Name: "MA均线叠加", Type: "indicator",
			Script: "MA5:MA(C,5);\nMA10:MA(C,10);\nMA20:MA(C,20);", ArgsJSON: "[]", Period: "day", Right: 1, Enabled: true,
			Description: "默认图表指标：在专业行情K线上叠加5/10/20日均线。",
		},
		{
			ID: "default-macd-golden", Name: "MACD金叉", Type: "selection",
			Script: "DIF:EMA(C,12)-EMA(C,26);\nDEA:EMA(DIF,9);\nMACD:CROSS(DIF,DEA);", ArgsJSON: "[]", Period: "day", Right: 1, Enabled: true,
			Description: "默认选股公式：MACD DIF上穿DEA。",
		},
		{
			ID: "default-kdj-golden", Name: "KDJ金叉", Type: "selection",
			Script: "RSV:=(C-LLV(L,9))/(HHV(H,9)-LLV(L,9))*100;\nK:SMA(RSV,3,1);\nD:SMA(K,3,1);\nKDJ:CROSS(K,D);", ArgsJSON: "[]", Period: "day", Right: 1, Enabled: true,
			Description: "默认选股公式：KDJ的K线上穿D线。",
		},
		{
			ID: "default-rsi-oversold", Name: "RSI超卖", Type: "selection",
			Script: "U:=MAX(C-REF(C,1),0);\nD:=MAX(REF(C,1)-C,0);\nRSI:100*MA(U,6)/(MA(U,6)+MA(D,6));", ArgsJSON: "[]", Period: "day", Right: 1, Enabled: true,
			Description: "默认选股公式：RSI低位反弹观察。",
		},
		{
			ID: "default-boll-breakout", Name: "BOLL突破", Type: "selection",
			Script: "MID:MA(C,20);\nUP:MID+2*STD(C,20);\nBOLL:C>UP;", ArgsJSON: "[]", Period: "day", Right: 1, Enabled: true,
			Description: "默认选股公式：收盘价突破BOLL上轨。",
		},
	}
	for _, item := range defaultFormulas {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO formulas
			(id,name,type,script,args_json,period,right,enabled,description,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			item.ID, item.Name, item.Type, item.Script, item.ArgsJSON, item.Period, item.Right, boolInt(item.Enabled), item.Description, now, now); err != nil {
			return err
		}
	}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM stock_pools`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		pool := StockPool{
			Name:        "示例股票池",
			Symbols:     []string{"000001", "600000", "000858", "002202"},
			Description: "第一版内置的小股票池，可在页面里改成你的自选列表。",
		}
		if _, err := s.UpsertStockPool(pool); err != nil {
			return err
		}
	}

	if err := s.ensureDecisionPools(); err != nil {
		return err
	}
	if err := s.ensureStrategyTemplates(); err != nil {
		return err
	}
	if err := s.ensureFixedAutomationTasks(); err != nil {
		return err
	}
	if err := s.ensureMarketInfoAutomationTasks(); err != nil {
		return err
	}
	if err := s.ensureTradingSystemState(); err != nil {
		return err
	}
	return s.ensureMacroEvents()
}

func defaultStrategyTemplates() []Strategy {
	templates := []Strategy{
		// ========== 中线趋势策略 ==========
		{
			ID:          "template-a-share-v3",
			Name:        "A股V3强势启动",
			Description: "中线趋势：先过滤流动性、趋势斜率、追高风险和弱收盘，再由放量、突破、MACD与主力拉升共振排序。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":200000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":8}},{"id":"change_guard","factor":"change_range","params":{"min":1,"max":6.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.65,"max_upper_shadow":2.5}}],"scores":[{"id":"volume_up","factor":"volume_up","weight":25,"params":{"days":5,"ratio":1.3}},{"id":"break_high","factor":"break_high","weight":25,"params":{"days":20}},{"id":"macd_golden","factor":"macd_golden_cross","weight":15,"params":{"fast":12,"slow":26,"signal":9}},{"id":"main_force","factor":"formula","weight":35,"params":{"formula_name":"主力拉升"}}],"pass":{"min_score":45,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-macd-trend",
			Name:        "MACD趋势启动",
			Description: "中线趋势：MACD金叉必须发生在中期多头且均线继续上行的结构中，过滤弱收盘和过度追涨。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.2,"max":7}},{"id":"macd_trigger","factor":"macd_golden_cross","params":{"fast":12,"slow":26,"signal":9}},{"id":"change_guard","factor":"change_range","params":{"min":-1,"max":5.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.6,"max_upper_shadow":2.5}}],"scores":[{"id":"volume_up","factor":"volume_up","weight":35,"params":{"days":5,"ratio":1.2}},{"id":"break_high","factor":"break_high","weight":30,"params":{"days":20}},{"id":"gain_quality","factor":"gain_days","weight":20,"params":{"days":20,"min":3,"max":25}},{"id":"slope_quality","factor":"ma_slope","weight":15,"params":{"period":20,"lookback":5,"min":0.2,"max":7}}],"pass":{"min_score":12,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-macd-calm-confirm",
			Name:        "MACD温和确认",
			Description: "短波段趋势：强势市场中只保留MACD金叉、中期多头、20日涨幅充足且近5日波动可控的温和启动信号。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":5}},{"id":"macd_trigger","factor":"macd_golden_cross","params":{"fast":12,"slow":26,"signal":9}},{"id":"change_guard","factor":"change_range","params":{"min":0,"max":4}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.65,"max_upper_shadow":2.3}},{"id":"market_regime","factor":"market_momentum","params":{"days":20,"min":6,"max":100}},{"id":"gain_guard","factor":"gain_days","params":{"days":20,"min":6,"max":30}},{"id":"amplitude_guard","factor":"max_amplitude","params":{"days":5,"max":6}}],"scores":[{"id":"volume_up","factor":"volume_up","weight":35,"params":{"days":5,"ratio":1.2}},{"id":"break_high","factor":"break_high","weight":30,"params":{"days":20}},{"id":"gain_quality","factor":"gain_days","weight":20,"params":{"days":20,"min":3,"max":25}},{"id":"slope_quality","factor":"ma_slope","weight":15,"params":{"period":20,"lookback":5,"min":0.2,"max":7}}],"pass":{"min_score":20,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-macd-broad-regime",
			Name:        "MACD宽市况精选",
			Description: "短波段趋势：在上证指数20日涨幅达到2%后扩大候选日期，再用趋势、涨幅、波动和40分质量门槛精选。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":5}},{"id":"macd_trigger","factor":"macd_golden_cross","params":{"fast":12,"slow":26,"signal":9}},{"id":"change_guard","factor":"change_range","params":{"min":0,"max":4}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.65,"max_upper_shadow":2.3}},{"id":"market_regime","factor":"market_momentum","params":{"days":20,"min":2,"max":100}},{"id":"gain_guard","factor":"gain_days","params":{"days":20,"min":6,"max":30}},{"id":"amplitude_guard","factor":"max_amplitude","params":{"days":5,"max":6}}],"scores":[{"id":"volume_up","factor":"volume_up","weight":35,"params":{"days":5,"ratio":1.2}},{"id":"break_high","factor":"break_high","weight":30,"params":{"days":20}},{"id":"gain_quality","factor":"gain_days","weight":20,"params":{"days":20,"min":3,"max":25}},{"id":"slope_quality","factor":"ma_slope","weight":15,"params":{"period":20,"lookback":5,"min":0.2,"max":7}}],"pass":{"min_score":40,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-pullback",
			Name:        "强势股回调低吸",
			Description: "中线回调：只在中期上行股票完成7%至12%回撤并出现KDJ止跌金叉时入选，排除高波动弱修复。",
			ConfigJSON:  `{"universe":{"include":[{"pool":"market-all-a"}],"exclude":[{"pool":"exclude"}]},"calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"max_amplitude","factor":"max_amplitude","params":{"days":5,"max":7.5}},{"id":"long_slope","factor":"ma_slope","params":{"period":60,"lookback":10,"min":0,"max":12}},{"id":"prior_strength","factor":"gain_days","params":{"days":20,"min":8,"max":35}},{"id":"pullback_depth","factor":"drawdown_from_high","params":{"days":60,"min":7,"max":12}},{"id":"kdj_trigger","factor":"kdj_golden_cross","params":{"n":9,"k":3,"d":3}},{"id":"change_guard","factor":"change_range","params":{"min":0,"max":1.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.55,"max_upper_shadow":2.5}}],"scores":[{"id":"rsi_rebound","factor":"rsi_rebound","weight":35,"params":{"period":6,"lookback":5,"oversold":35,"min_current":38,"max_current":60}},{"id":"gain_quality","factor":"gain_days","weight":30,"params":{"days":20,"min":8,"max":25}},{"id":"drawdown_quality","factor":"drawdown_from_high","weight":35,"params":{"days":60,"min":7,"max":12}}],"pass":{"min_score":35,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-trend-pullback-confirm",
			Name:        "趋势回踩双确认",
			Description: "短中线回调：多头趋势温和回踩后必须出现KDJ确认，MACD、量能与回撤位置用于质量排序。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":10,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0,"max":6}},{"id":"kdj_trigger","factor":"kdj_golden_cross","params":{"n":9,"k":3,"d":3}},{"id":"calm_pullback","factor":"change_range","params":{"min":-3,"max":3}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.55,"max_upper_shadow":2.5}}],"scores":[{"id":"macd_confirm","factor":"macd_golden_cross","weight":40,"params":{"fast":12,"slow":26,"signal":9}},{"id":"volume_confirm","factor":"volume_up","weight":25,"params":{"days":5,"ratio":1.1}},{"id":"drawdown_quality","factor":"drawdown_from_high","weight":35,"params":{"days":20,"min":2,"max":8}}],"pass":{"min_score":15,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-rsi-trend-reversal",
			Name:        "RSI趋势反转",
			Description: "反转捕捉：RSI近期进入超卖区后重新回到强弱分界，且长期均线仍向上，避免把持续下跌误判为反转。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"price_range","factor":"price_range","params":{"min":3,"max":120}},{"id":"long_slope","factor":"ma_slope","params":{"period":60,"lookback":10,"min":0,"max":10}},{"id":"rsi_rebound_trigger","factor":"rsi_rebound","params":{"period":6,"lookback":5,"oversold":32,"min_current":36,"max_current":55}},{"id":"long_gain_guard","factor":"gain_days","params":{"days":60,"min":0,"max":40}},{"id":"change_guard","factor":"change_range","params":{"min":0,"max":5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.6,"max_upper_shadow":2.5}}],"scores":[{"id":"kdj_golden","factor":"kdj_golden_cross","weight":40,"params":{"n":9,"k":3,"d":3}},{"id":"ma_trend","factor":"ma_trend","weight":30,"params":{"short":5,"mid":20,"long":60}},{"id":"volume_up","factor":"volume_up","weight":30,"params":{"days":5,"ratio":1.1}}],"pass":{"min_score":5,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-volume-breakout-confirm",
			Name:        "量价突破确认",
			Description: "大资金策略：放量突破、均线多头和趋势斜率均为硬条件，重点保留流动性好且收盘承接强的标的。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":250000000}},{"id":"price_range","factor":"price_range","params":{"min":5,"max":120}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":10,"long":20}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":8}},{"id":"breakout_trigger","factor":"volume_breakout","params":{"days":20,"ratio":1.6,"min_change":2}},{"id":"change_guard","factor":"change_range","params":{"min":2,"max":6.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.72,"max_upper_shadow":2}}],"scores":[{"id":"macd_golden","factor":"macd_golden_cross","weight":35,"params":{"fast":12,"slow":26,"signal":9}},{"id":"boll_breakout","factor":"boll_breakout","weight":25,"params":{"period":20,"width":2}},{"id":"gain_quality","factor":"gain_days","weight":25,"params":{"days":20,"min":5,"max":25}},{"id":"slope_quality","factor":"ma_slope","weight":15,"params":{"period":20,"lookback":5,"min":0.3,"max":8}}],"pass":{"min_score":15,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-new-high-trend",
			Name:        "新高趋势延续",
			Description: "强势延续：20日收盘新高、长期多头、均线抬升和量能确认全部满足后才进入候选。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":250000000}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":10,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":8}},{"id":"break_high_trigger","factor":"break_high","params":{"days":20}},{"id":"volume_filter","factor":"volume_up","params":{"days":5,"ratio":1.2}},{"id":"change_guard","factor":"change_range","params":{"min":0.5,"max":6.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.68,"max_upper_shadow":2.5}}],"scores":[{"id":"macd_golden","factor":"macd_golden_cross","weight":35,"params":{"fast":12,"slow":26,"signal":9}},{"id":"boll_breakout","factor":"boll_breakout","weight":25,"params":{"period":20,"width":2}},{"id":"gain_quality","factor":"gain_days","weight":25,"params":{"days":20,"min":5,"max":25}},{"id":"slope_quality","factor":"ma_slope","weight":15,"params":{"period":20,"lookback":5,"min":0.3,"max":8}}],"pass":{"min_score":15,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-rocket-confirm",
			Name:        "火箭启动确认",
			Description: "爆发捕捉：火箭信号、均线多头、趋势斜率与强收盘全部确认，进一步限制流动性和追高幅度。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":300000000}},{"id":"price_range","factor":"price_range","params":{"min":5,"max":100}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":10,"long":20}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.5,"max":8}},{"id":"rocket_trigger","factor":"local_rocket","params":{"lookback":20,"volume_days":5,"volume_ratio":1.8,"min_change":3,"short_ma":5,"mid_ma":10}},{"id":"change_guard","factor":"change_range","params":{"min":3,"max":6.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.75,"max_upper_shadow":1.8}}],"scores":[{"id":"macd_golden","factor":"macd_golden_cross","weight":40,"params":{"fast":12,"slow":26,"signal":9}},{"id":"boll_breakout","factor":"boll_breakout","weight":30,"params":{"period":20,"width":2}},{"id":"gain_quality","factor":"gain_days","weight":30,"params":{"days":20,"min":8,"max":25}}],"pass":{"min_score":15,"top_n":5}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-boll-macd-resonance",
			Name:        "BOLL MACD共振",
			Description: "多指标共振：BOLL突破与MACD金叉必须同日共振，并要求均线抬升、强收盘和适度量能。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":250000000}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":10,"long":20}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.3,"max":8}},{"id":"boll_trigger","factor":"boll_breakout","params":{"period":20,"width":2}},{"id":"macd_trigger","factor":"macd_golden_cross","params":{"fast":12,"slow":26,"signal":9}},{"id":"change_guard","factor":"change_range","params":{"min":1,"max":6.5}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.7,"max_upper_shadow":2}}],"scores":[{"id":"volume_up","factor":"volume_up","weight":50,"params":{"days":5,"ratio":1.25}},{"id":"break_high","factor":"break_high","weight":30,"params":{"days":20}},{"id":"gain_quality","factor":"gain_days","weight":20,"params":{"days":20,"min":5,"max":25}}],"pass":{"min_score":12,"top_n":5}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-strong-trend-continuation",
			Name:        "强趋势延续精选",
			Description: "精选龙头：高流动性股票必须同时满足长期多头、均线抬升、放量新高与强收盘，辅助指标只负责排序。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":300000000}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":20,"long":60}},{"id":"ma_slope_filter","factor":"ma_slope","params":{"period":20,"lookback":5,"min":0.5,"max":8}},{"id":"volume_filter","factor":"volume_up","params":{"days":5,"ratio":1.3}},{"id":"break_high_trigger","factor":"break_high","params":{"days":20}},{"id":"change_guard","factor":"change_range","params":{"min":1,"max":6}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.7,"max_upper_shadow":2}}],"scores":[{"id":"macd_golden","factor":"macd_golden_cross","weight":30,"params":{"fast":12,"slow":26,"signal":9}},{"id":"kdj_golden","factor":"kdj_golden_cross","weight":25,"params":{"n":9,"k":3,"d":3}},{"id":"gain_quality","factor":"gain_days","weight":25,"params":{"days":20,"min":8,"max":25}},{"id":"boll_breakout","factor":"boll_breakout","weight":20,"params":{"period":20,"width":2}}],"pass":{"min_score":15,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},

		// ========== 新增4个早期预警策略 ==========
		{
			ID:          "template-early-macd-approach",
			Name:        "MACD即将金叉预警",
			Description: "早期预警：DIF接近DEA且差值快速收敛，在金叉确认前1-2天提前买入，适合捕捉趋势启动初期。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":10,"long":20}},{"id":"macd_approaching","factor":"macd_approaching","params":{"fast":12,"slow":26,"signal":9,"max_distance":0.15,"min_convergence":0.02,"require_positive":false}},{"id":"change_guard","factor":"change_range","params":{"min":-1,"max":4}},{"id":"volume_warming","factor":"volume_up","params":{"days":5,"ratio":1.1}}],"scores":[{"id":"convergence_speed","factor":"macd_convergence_speed","weight":40,"params":{"fast":12,"slow":26,"signal":9}},{"id":"ma_distance","factor":"distance_from_ma","weight":30,"params":{"period":20,"min":2,"max":8}},{"id":"volume_change","factor":"volume_change_rate","weight":30,"params":{"days":5}}],"pass":{"min_score":15,"top_n":10}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-early-kdj-lift",
			Name:        "KDJ底部抬升预警",
			Description: "早期预警：K值在底部连续向上但还未上穿D值，提前1-2天埋伏，适合抓住反弹初期机会。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":150000000}},{"id":"kdj_approaching","factor":"kdj_approaching","params":{"n":9,"k":3,"d":3,"max_distance":8,"k_range_min":20,"k_range_max":50,"require_consecutive_rise":2}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":10,"mid":20}},{"id":"change_guard","factor":"change_range","params":{"min":-2,"max":3}}],"scores":[{"id":"k_slope","factor":"kdj_k_slope","weight":35,"params":{"n":9,"k":3,"d":3}},{"id":"rsi_confirm","factor":"rsi_rebound","weight":30,"params":{"period":6,"lookback":5,"oversold":30,"min_current":32,"max_current":55}},{"id":"volume_quality","factor":"volume_steady_rise","weight":35,"params":{"days":3}}],"pass":{"min_score":12,"top_n":10}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-early-volume-ignition",
			Name:        "放量启动日捕捉",
			Description: "早期预警：缩量整理后突然放量启动当天，收盘强势但涨幅适中，捕捉启动首日的最佳买点。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":200000000}},{"id":"volume_burst","factor":"volume_burst_after_shrink","params":{"lookback":5,"shrink_threshold":2,"burst_ratio":2.0,"min_turnover":3}},{"id":"price_position","factor":"distance_from_high","params":{"days":60,"min":-15,"max":-3}},{"id":"ma_support","factor":"ma_trend","params":{"short":10,"mid":20}},{"id":"change_guard","factor":"change_range","params":{"min":2,"max":7}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.7,"max_upper_shadow":2}}],"scores":[{"id":"volume_explosion","factor":"volume_ratio","weight":40,"params":{"days":5}},{"id":"close_position","factor":"close_strength","weight":30,"params":{"min_position":0.6,"max_upper_shadow":3}},{"id":"ma_slope","factor":"ma_slope","weight":30,"params":{"period":20,"lookback":5,"min":0,"max":10}}],"pass":{"min_score":15,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-early-breakout-ambush",
			Name:        "突破前夜埋伏",
			Description: "早期预警：价格逼近前期高点但还未突破，成交量连续放大，在突破前1-2天提前埋伏。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":200000000}},{"id":"near_high","factor":"distance_from_high","params":{"days":20,"min":-5,"max":-1}},{"id":"consecutive_rise","factor":"consecutive_gain_days","params":{"min_days":2,"max_days":4}},{"id":"volume_consecutive","factor":"volume_consecutive_rise","params":{"days":3}},{"id":"ma_trend_filter","factor":"ma_trend","params":{"short":5,"mid":10,"long":20}},{"id":"macd_positive","factor":"macd_positive","params":{"fast":12,"slow":26,"signal":9}},{"id":"change_guard","factor":"change_range","params":{"min":0.5,"max":3}},{"id":"drawdown_protection","factor":"max_amplitude","params":{"days":5,"max":5}}],"scores":[{"id":"distance_quality","factor":"distance_from_high","weight":35,"params":{"days":20,"optimal_min":-3,"optimal_max":-2}},{"id":"volume_steadiness","factor":"volume_consecutive_rise","weight":35,"params":{"days":3}},{"id":"macd_strength","factor":"macd_histogram","weight":30,"params":{"fast":12,"slow":26,"signal":9}}],"pass":{"min_score":18,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},

		// ========== 新增3个震荡市策略 ==========
		{
			ID:          "template-range-bounce",
			Name:        "区间震荡下沿买入",
			Description: "震荡市策略：价格在明确的箱体区间内震荡，跌到下沿且RSI超卖时买入，反弹到均线附近止盈。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":150000000}},{"id":"range_bound","factor":"range_volatility","params":{"days":60,"max_range":25}},{"id":"at_lower_edge","factor":"distance_from_ma","params":{"period":20,"min":-5,"max":-2}},{"id":"rsi_oversold","factor":"rsi_value","params":{"period":6,"max":35}},{"id":"decline_slowing","factor":"decline_deceleration","params":{"lookback":3}},{"id":"volume_shrink","factor":"volume_below_average","params":{"days":5,"ratio":0.8}},{"id":"support_hold","factor":"support_not_broken","params":{"lookback":10}},{"id":"no_downtrend","factor":"ma_slope","params":{"period":60,"lookback":10,"min":-2,"max":2}}],"scores":[{"id":"oversold_degree","factor":"rsi_value","weight":40,"params":{"period":6,"optimal_min":25,"optimal_max":35}},{"id":"volume_exhaustion","factor":"volume_ratio","weight":30,"params":{"days":5,"lower_better":true}},{"id":"support_distance","factor":"distance_from_support","weight":30,"params":{"lookback":20}}],"pass":{"min_score":12,"top_n":8}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-ma-convergence-breakout",
			Name:        "均线缠绕突破方向",
			Description: "震荡市策略：多条均线高度粘合充分整理后，突然选择向上方向并放量确认，适合震荡末期。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":180000000}},{"id":"ma_convergence","factor":"ma_convergence","params":{"ma_periods":[5,10,20],"max_divergence":3,"min_duration":5,"max_duration":15}},{"id":"breakout_up","factor":"price_above_ma_cluster","params":{"ma_periods":[5,10,20],"min_distance":1}},{"id":"volume_confirm","factor":"volume_up","params":{"days":10,"ratio":1.3}},{"id":"change_guard","factor":"change_range","params":{"min":1,"max":5}},{"id":"boll_expand","factor":"boll_width_expanding","params":{"period":20,"width":2}},{"id":"no_weak_stock","factor":"gain_days","params":{"days":60,"min":-15,"max":100}}],"scores":[{"id":"convergence_duration","factor":"ma_convergence_days","weight":35,"params":{"ma_periods":[5,10,20],"optimal_min":10,"optimal_max":12}},{"id":"breakout_strength","factor":"breakout_strength","weight":35,"params":{"volume_weight":0.6,"price_weight":0.4}},{"id":"ma_alignment","factor":"ma_distance","weight":30,"params":{"ma_periods":[5,10,20]}}],"pass":{"min_score":15,"top_n":6}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-platform-breakout",
			Name:        "缩量平台突破",
			Description: "震荡市策略：横盘平台长期缩量整理后放量突破，适合个股走独立行情，不依赖大盘环境。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":200000000}},{"id":"platform_pattern","factor":"narrow_range","params":{"days":20,"max_range":12}},{"id":"low_turnover","factor":"average_turnover","params":{"days":10,"max":2}},{"id":"volume_burst","factor":"turnover_surge","params":{"min_turnover":3,"min_ratio":1.8}},{"id":"breakout_high","factor":"price_break_high","params":{"days":20}},{"id":"change_guard","factor":"change_range","params":{"min":2,"max":8}},{"id":"close_quality","factor":"close_strength","params":{"min_position":0.75,"max_upper_shadow":2}},{"id":"mid_term_stable","factor":"ma_slope","params":{"period":60,"lookback":10,"min":-5,"max":10}}],"scores":[{"id":"platform_duration","factor":"consolidation_days","weight":35,"params":{"min":15,"max":25,"optimal_min":18,"optimal_max":22}},{"id":"breakout_power","factor":"breakout_strength","weight":40,"params":{"volume_weight":0.7,"price_weight":0.3}},{"id":"close_quality","factor":"close_strength","weight":25,"params":{"min_position":0.6,"max_upper_shadow":3}}],"pass":{"min_score":18,"top_n":6}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-low-three-bull-first-board",
			Name:        "低位三连阳首板",
			Description: "首板接力：首板前两日连续阳线、信号日主板首次涨停，要求放量不低于2.5倍、首板前60日位置不高于60%、近5日区间振幅不高于15%；次日仅在开盘涨幅0%-6%时参与。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":500000000}},{"id":"price_range","factor":"price_range","params":{"min":2,"max":80}},{"id":"first_board_pattern","factor":"low_three_bull_first_board","params":{"position_lookback":60,"max_pre_position":60,"min_amount":500000000,"max_amount":4000000000,"min_volume_ratio":2.5,"max_prior_volume_ratio":2,"max_prior_gain":5,"max_five_day_range":15,"limit_tolerance":0.25}}],"scores":[{"id":"pattern_quality","factor":"low_three_bull_first_board","weight":100,"params":{"position_lookback":60,"max_pre_position":60,"min_amount":500000000,"max_amount":4000000000,"min_volume_ratio":2.5,"max_prior_volume_ratio":2,"max_prior_gain":5,"max_five_day_range":15,"limit_tolerance":0.25}}],"pass":{"min_score":12,"top_n":5},"execution":{"min_open_gap":0,"max_open_gap":6}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-jq-ma5-momentum",
			Name:        "聚宽MA5百分之一动量",
			Description: "聚宽双均线示例的系统化版本：收盘价至少高于MA5 1%，控制短线乖离、单日涨幅与流动性后，按均线距离、量能和收盘强度排序。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":100000000}},{"id":"price_range","factor":"price_range","params":{"min":2,"max":120}},{"id":"ma5_trigger","factor":"distance_from_ma","params":{"period":5,"min":1,"max":6}},{"id":"change_guard","factor":"change_range","params":{"min":-1,"max":6.5}}],"scores":[{"id":"ma5_distance","factor":"distance_from_ma","weight":50,"params":{"period":5,"optimal_min":1,"optimal_max":3}},{"id":"volume_quality","factor":"volume_change_rate","weight":25,"params":{"days":5}},{"id":"close_quality","factor":"close_strength","weight":25,"params":{"min_position":0.55,"max_upper_shadow":3}}],"pass":{"min_score":12,"top_n":10}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-jq-small-cap-rotation",
			Name:        "聚宽20至30亿小市值轮动",
			Description: "聚宽小市值策略适配版：每周首个交易日从点时总市值20至30亿元的股票中按市值升序选3只；股本按当日已生效记录计算，避免历史回测使用未来数据。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":10000000}},{"id":"weekly_rebalance","factor":"rebalance_day","params":{"mode":"week_first"}},{"id":"market_cap_band","factor":"market_cap_range","params":{"min_yi":20,"max_yi":30}}],"scores":[{"id":"smallest_cap_first","factor":"market_cap_range","weight":100,"params":{"min_yi":20,"max_yi":30}}],"pass":{"min_score":1,"top_n":3}}`,
			Enabled:     true,
			Readonly:    true,
		},
		{
			ID:          "template-jq-yellow-banner-value",
			Name:        "聚宽正黄旗高股息价值",
			Description: "聚宽正黄旗大妈选股法适配版：月初从近一年股息率市场前25%、点时PEG介于-1和3、原始股价低于9元的股票中，按市值与股息质量选10只；财报只在实际披露后参与计算。",
			ConfigJSON:  `{"universe":"market","pool_id":"market-all-a","calc_count":260,"batch_size":50,"continue_on_error":true,"filters":[{"id":"exclude_pool","factor":"pool_exclude","params":{"pool_id":"exclude"}},{"id":"min_amount","factor":"min_amount","params":{"value":10000000}},{"id":"monthly_rebalance","factor":"rebalance_day","params":{"mode":"month_first"}},{"id":"low_raw_price","factor":"raw_price_range","params":{"min":0.5,"max":9}},{"id":"high_dividend","factor":"dividend_yield_percentile","params":{"min_percentile":75,"min_yield":0}},{"id":"peg_band","factor":"peg_range","params":{"min":-1,"max":3}}],"scores":[{"id":"small_cap_first","factor":"market_cap_ascending","weight":70,"params":{}},{"id":"dividend_quality","factor":"dividend_yield_percentile","weight":30,"params":{"min_percentile":75,"min_yield":0}}],"pass":{"min_score":1,"top_n":10},"execution":{"min_open_gap":-9,"max_open_gap":9}}`,
			Enabled:     true,
			Readonly:    true,
		},
	}
	for index := range templates {
		applyHighQualityStrategyProfile(&templates[index])
		applyDefaultStrategyUniverse(&templates[index])
	}
	return templates
}

var retiredStrategyTemplateIDs = []string{
	"template-local-rocket",
	"template-ma-volume-breakout",
	"template-macd-kdj-resonance",
	"template-boll-breakout-confirm",
	"template-kdj-pullback-confirm",
	"template-macd-follow-through",
	"template-breakout-pullback-reentry",
}

// strongMarketStrategyIDs 定义了需要强势市场环境（上证指数20日涨幅≥6%）才启用的策略
// 这些策略高度依赖趋势环境，在震荡市或弱市中容易失效
var strongMarketStrategyIDs = map[string]bool{
	// 中线趋势策略（保留的7个需要强势环境）
	"template-a-share-v3":                true,
	"template-macd-trend":                true,
	"template-volume-breakout-confirm":   true,
	"template-new-high-trend":            true,
	"template-rocket-confirm":            true,
	"template-boll-macd-resonance":       true,
	"template-strong-trend-continuation": true,
	// 早期预警策略不需要强势市场环境（它们本身就是捕捉启动初期）
	// 震荡市策略也不需要强势市场环境（它们专为震荡市设计）
}

func applyHighQualityStrategyProfile(strategy *Strategy) {
	var config map[string]interface{}
	if strategy == nil || json.Unmarshal([]byte(strategy.ConfigJSON), &config) != nil {
		return
	}
	filters, _ := config["filters"].([]interface{})
	if strongMarketStrategyIDs[strategy.ID] {
		filters = append(filters, map[string]interface{}{
			"id": "market_regime", "factor": "market_momentum",
			"params": map[string]interface{}{"days": 20, "min": 6, "max": 100},
		})
		strategy.Description += " 仅在上证指数20日涨幅达到6%时启用。"
	}
	if strategy.ID == "template-pullback" {
		for _, raw := range filters {
			rule, _ := raw.(map[string]interface{})
			switch rule["id"] {
			case "pullback_depth":
				rule["params"] = map[string]interface{}{"days": 60, "min": 7, "max": 12}
			case "change_guard":
				rule["params"] = map[string]interface{}{"min": 0, "max": 1.5}
			}
		}
		for _, raw := range config["scores"].([]interface{}) {
			rule, _ := raw.(map[string]interface{})
			if rule["id"] == "drawdown_quality" {
				rule["params"] = map[string]interface{}{"days": 60, "min": 7, "max": 12}
			}
		}
		strategy.Description += " 二次优化限定信号日温和上涨0%-1.5%、距60日高点回撤7%-12%。"
	}
	config["filters"] = filters
	if raw, err := json.Marshal(config); err == nil {
		strategy.ConfigJSON = string(raw)
	}
}

// System templates deliberately exclude the higher-volatility STAR Market and
// Beijing Stock Exchange from their default candidates. Their candidate
// universe can be adjusted independently from the immutable strategy rules.
func applyDefaultStrategyUniverse(strategy *Strategy) {
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
		return
	}
	config["universe"] = map[string]interface{}{
		"include": []map[string]string{{"pool": "market-all-a"}},
		"exclude": []map[string]string{
			{"pool": "exclude"},
			{"pool": "market-star"},
			{"pool": "market-bj"},
		},
	}
	if strategy.ID == "template-low-three-bull-first-board" {
		universe := config["universe"].(map[string]interface{})
		universe["exclude"] = append(universe["exclude"].([]map[string]string), map[string]string{"pool": "market-gem"})
	}
	if strategy.ID == "template-jq-yellow-banner-value" {
		universe := config["universe"].(map[string]interface{})
		universe["exclude"] = append(universe["exclude"].([]map[string]string), map[string]string{"pool": "market-gem"})
	}
	// System strategies evaluate their whole candidate universe by default.
	// A zero scan_limit is the explicit full-scan value in the strategy runner.
	config["scan_limit"] = 0
	delete(config, "pool_id")
	if raw, err := json.Marshal(config); err == nil {
		strategy.ConfigJSON = string(raw)
	}
	if !strings.Contains(strategy.Description, "不含科创板和北交所") {
		strategy.Description += " 默认范围不含科创板和北交所。"
	}
	if strategy.ID == "template-low-three-bull-first-board" && !strings.Contains(strategy.Description, "仅沪深主板") {
		strategy.Description += " 仅沪深主板。"
	}
	if !strings.Contains(strategy.Description, "默认全量扫描") {
		strategy.Description += " 默认全量扫描。"
	}
}

func (s *AppStore) ensureStrategyTemplates() error {
	now := NowText()
	for _, item := range defaultStrategyTemplates() {
		if strings.TrimSpace(item.ConfigJSON) == "" {
			item.ConfigJSON = "{}"
		}
		_, err := s.db.Exec(`INSERT OR IGNORE INTO strategies
			(id,name,description,config_json,enabled,readonly,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			item.ID, item.Name, item.Description, item.ConfigJSON, boolInt(item.Enabled), boolInt(item.Readonly), now, now)
		if err != nil {
			return err
		}
		// A system template owns its scoring definition, but its candidate
		// universe is user-configurable. Keep that narrow customization when
		// refreshing built-in template definitions during startup.
		current, err := s.GetStrategy(item.ID)
		if err != nil {
			return err
		}
		configJSON := item.ConfigJSON
		if current.Readonly && hasCustomizedStrategyUniverse(current.ConfigJSON) {
			configJSON = withCustomizedStrategyUniverse(item.ConfigJSON, current.ConfigJSON)
		}
		_, err = s.db.Exec(`UPDATE strategies
			SET name=?,description=?,config_json=?,enabled=?,readonly=1,updated_at=?
			WHERE id=? AND readonly=1`,
			item.Name, item.Description, configJSON, boolInt(item.Enabled), now, item.ID)
		if err != nil {
			return err
		}
	}
	for _, id := range retiredStrategyTemplateIDs {
		if _, err := s.db.Exec(`DELETE FROM strategies WHERE id=? AND readonly=1`, id); err != nil {
			return err
		}
	}
	return nil
}

func hasCustomizedStrategyUniverse(configJSON string) bool {
	var config struct {
		UniverseCustomized bool `json:"universe_customized"`
	}
	return json.Unmarshal([]byte(configJSON), &config) == nil && config.UniverseCustomized
}

// withCustomizedStrategyUniverse carries only the user-controlled field over
// to the current template definition, so later factor and scoring upgrades are
// still applied to system strategies.
func withCustomizedStrategyUniverse(templateConfigJSON, customizedConfigJSON string) string {
	var templateConfig, customizedConfig map[string]json.RawMessage
	if err := json.Unmarshal([]byte(templateConfigJSON), &templateConfig); err != nil || templateConfig == nil {
		return templateConfigJSON
	}
	if err := json.Unmarshal([]byte(customizedConfigJSON), &customizedConfig); err != nil || customizedConfig == nil {
		return templateConfigJSON
	}
	universe, ok := customizedConfig["universe"]
	if !ok {
		return templateConfigJSON
	}
	templateConfig["universe"] = universe
	templateConfig["universe_customized"] = json.RawMessage(`true`)
	raw, err := json.Marshal(templateConfig)
	if err != nil {
		return templateConfigJSON
	}
	return string(raw)
}

func fixedCloseSyncAutomationTask() AutomationTask {
	return AutomationTask{
		ID:           FixedCloseSyncTaskID,
		Name:         "收盘作业：Hikyuu盘后同步",
		Type:         "system_sync",
		Cron:         "0 0 17 * * 1-5",
		Enabled:      true,
		OnlyTradeDay: true,
		PayloadJSON:  `{"scope":"hikyuu_after_close"}`,
		WebhookIDs:   "[]",
		Readonly:     true,
		System:       true,
	}
}

func fixedSelectionTrackingAutomationTask() AutomationTask {
	return AutomationTask{
		ID:           FixedSelectionTrackingTaskID,
		Name:         "收盘作业：验证选股表现",
		Type:         "selection_tracking",
		Cron:         "0 15 17 * * 1-5",
		Enabled:      true,
		OnlyTradeDay: true,
		PayloadJSON:  `{"horizons":[1,3,5,10],"history_buffer":30,"target_return":3,"drawdown_limit":5,"continue_on_error":true}`,
		WebhookIDs:   "[]",
		Readonly:     true,
		System:       true,
	}
}

func systemStrategyDailyBatchAutomationTask() AutomationTask {
	return AutomationTask{
		ID:   SystemStrategyDailyBatchTaskID,
		Name: "收盘作业：系统策略日报",
		Type: "system_strategy_batch",
		// This task is dependency-triggered after close sync rather than cron-triggered.
		Cron:         "",
		Enabled:      true,
		OnlyTradeDay: true,
		PayloadJSON:  `{"source":"fixed-close-sync","shared_kline_cache":true}`,
		WebhookIDs:   "[]",
		Readonly:     true,
		System:       true,
	}
}

func IsFixedAutomationTaskID(id string) bool {
	return id == FixedCloseSyncTaskID || id == FixedSelectionTrackingTaskID || id == SystemStrategyDailyBatchTaskID
}

func decorateAutomationTask(t AutomationTask) AutomationTask {
	if IsFixedAutomationTaskID(t.ID) {
		t.Readonly = true
		t.System = true
	}
	return t
}

func (s *AppStore) ensureFixedAutomationTasks() error {
	now := NowText()
	for _, task := range []AutomationTask{fixedCloseSyncAutomationTask(), systemStrategyDailyBatchAutomationTask(), fixedSelectionTrackingAutomationTask()} {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO automation_tasks
			(id,name,type,cron,enabled,only_trade_day,payload_json,webhook_ids,last_run_at,next_run_at,last_status,last_message,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			task.ID, task.Name, task.Type, task.Cron, boolInt(task.Enabled), boolInt(task.OnlyTradeDay), task.PayloadJSON, task.WebhookIDs,
			"", "", "", "", now, now); err != nil {
			return err
		}
		// Fixed task behavior belongs to the application. Keep the user's enabled
		// choice while migrating immutable definitions such as cron and payload.
		if _, err := s.db.Exec(`UPDATE automation_tasks
			SET name=?,type=?,cron=?,only_trade_day=?,payload_json=?,webhook_ids=?,updated_at=?
			WHERE id=?`, task.Name, task.Type, task.Cron, boolInt(task.OnlyTradeDay), task.PayloadJSON, task.WebhookIDs, now, task.ID); err != nil {
			return err
		}
	}
	return nil
}

func marketInfoAutomationTasks() []AutomationTask {
	return []AutomationTask{
		{
			ID:           MarketLongTigerSyncTaskID,
			Name:         "龙虎榜同步",
			Type:         "system_sync",
			Cron:         "0 0 18 * * 1-5",
			Enabled:      true,
			OnlyTradeDay: true,
			PayloadJSON:  `{"scope":"market_info","kinds":["long-tiger"],"continue_on_error":true}`,
			WebhookIDs:   "[]",
			System:       true,
		},
		{
			ID:           MarketHotMoneySyncTaskID,
			Name:         "游资动向同步",
			Type:         "system_sync",
			Cron:         "0 5 18 * * 1-5",
			Enabled:      true,
			OnlyTradeDay: true,
			PayloadJSON:  `{"scope":"market_info","kinds":["hot_money"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:   "[]",
			System:       true,
		},
		{
			ID:           MarketResearchSyncTaskID,
			Name:         "个股研报同步",
			Type:         "system_sync",
			Cron:         "0 10 18 * * 1-5",
			Enabled:      true,
			OnlyTradeDay: true,
			PayloadJSON:  `{"scope":"market_info","kinds":["research"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:   "[]",
			System:       true,
		},
		{
			ID:           MarketNoticeSyncTaskID,
			Name:         "公司公告同步",
			Type:         "system_sync",
			Cron:         "0 15 18 * * 1-5",
			Enabled:      true,
			OnlyTradeDay: true,
			PayloadJSON:  `{"scope":"market_info","kinds":["notice"],"max_codes":120,"continue_on_error":true}`,
			WebhookIDs:   "[]",
			System:       true,
		},
		{
			ID:           MarketIndustryResearchSyncTaskID,
			Name:         "行业研究同步",
			Type:         "system_sync",
			Cron:         "0 20 18 * * 1-5",
			Enabled:      true,
			OnlyTradeDay: true,
			PayloadJSON:  `{"scope":"market_industry_research","days":7,"limit":500,"continue_on_error":true}`,
			WebhookIDs:   "[]",
			System:       true,
		},
	}
}

func (s *AppStore) ensureMarketInfoAutomationTasks() error {
	now := NowText()
	if _, err := s.db.Exec(`DELETE FROM automation_tasks WHERE id=? OR payload_json=?`, LegacyMarketInfoSyncTaskID, LegacyMarketInfoSyncPayload); err != nil {
		return err
	}
	for _, task := range marketInfoAutomationTasks() {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO automation_tasks
			(id,name,type,cron,enabled,only_trade_day,payload_json,webhook_ids,last_run_at,next_run_at,last_status,last_message,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			task.ID, task.Name, task.Type, task.Cron, boolInt(task.Enabled), boolInt(task.OnlyTradeDay), task.PayloadJSON, task.WebhookIDs,
			"", "", "", "", now, now); err != nil {
			return err
		}
	}
	return nil
}

func DefaultTradingSystemState() TradingSystemState {
	return TradingSystemState{
		Account: TradingAccount{
			Principal:         50000,
			TotalAssets:       56279.20,
			MaxTradeRisk:      1,
			MaxPositionWeight: 30,
		},
		Discipline: TradingDiscipline{
			Reason:    false,
			Invalid:   false,
			Risk:      false,
			NoImpulse: false,
		},
		Fees: TradingFeeConfig{
			BuyCommissionRate:  0.03,
			SellCommissionRate: 0.03,
			StampTaxRate:       0.05,
			TransferFeeRate:    0.001,
			MinCommission:      5,
		},
		Filter: "all",
		Trades: []TradingTrade{
			{
				ID:            "serveyou-2026-08-26",
				StockName:     "税友股份",
				StockCode:     "603171",
				Direction:     "buy",
				Status:        "active",
				EntryDate:     "2026-08-26",
				EntryPrice:    40.80,
				ExitDate:      "",
				ExitPrice:     0,
				ExitShares:    0,
				CurrentPrice:  40.29,
				Shares:        300,
				InvalidPrice:  39.60,
				PositionLabel: "试错仓",
				TargetOne:     "40.80 / 42.60-43.30",
				TargetTwo:     "45-46",
				TradeMode:     "强势股回调到关键支撑附近，轻仓试错，博短线止跌修复。",
				BuyReason:     "1. 前期从 34.42 附近启动，最高冲到 52.10，说明曾经有明显资金推动。\n2. 从 8月6日高点 52.10 回落到 40 元附近，跌幅较大，短线存在修复可能。\n3. 当前价格接近 7月31日加速段低点 39.60 附近，属于重要短线观察区。\n4. 买入价 40.80 距离 39.60 风控位不远，单股风险约 1.20 元，亏损比例约 2.94%，风险可控。",
				ExitRules:     "技术无效点：有效跌破 39.60 元。\n\n有效跌破定义：\n1. 放量跌破 39.60，并且 15-30 分钟内不能快速收回；\n2. 或者当天收盘价低于 39.60；\n3. 或者跌破后反抽到 39.60-40.00 区间明显受压，再次回落。\n\n操作计划：\n1. 如果明天跌破 39.60 且收不回，先卖出 50% 或全部离场，保护本金。\n2. 如果盘中跌破 39.60 后快速拉回 40 元上方，先不急着卖完，观察是否是假破。\n3. 如果重新站稳 40.80 成本价，说明短线压力减轻，继续观察。\n4. 如果站上 42.60-43.30 区间，说明短线企稳增强，可考虑是否加确认仓。\n5. 如果反弹到 45-46 区间但放量滞涨，考虑止盈或减仓。\n6. 如果跌破 39.60 后反抽不过 40 元，不接回，不补仓。\n\n加仓条件：守住 39.60；重新站稳 40.80；放量站上 42.60-43.30。\n\n不加仓条件：跌破 39.60 后没有收回；反弹无量；冲高回落；大盘明显走弱；只是因为怕踏空而想追。\n\n止盈计划：快速反弹到 42.60-43.30 但量能不足，可先减一部分；放量突破 43.30 继续持有观察；到 45-46 附近冲不动，优先锁定利润；反弹途中跌回 40.80 下方，减仓。",
				Review:        "盘后复盘：\n1. 今天有没有按计划执行？\n2. 买入理由是否仍然成立？\n3. 39.60 是否守住？\n4. 成交量是放大还是缩小？\n5. 反弹时有没有主动买盘？\n6. 我的操作是按规则，还是按情绪？\n7. 这笔交易下次哪里可以改进？",
			},
		},
	}
}

func (s *AppStore) ensureTradingSystemState() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM trading_system_state`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	state := DefaultTradingSystemState()
	_, err := s.UpsertTradingSystemState(state)
	return err
}

func (s *AppStore) ensureDecisionPools() error {
	defaults := []StockPool{
		{
			ID:          DecisionWatchPoolID,
			Name:        "观察池",
			Description: "首页决策工作台使用的观察列表，适合放入今日命中后准备继续跟踪的股票。",
		},
		{
			ID:          DecisionExcludePoolID,
			Name:        "排除池",
			Description: "首页决策工作台使用的排除列表，适合放入暂时不想再处理的股票。",
		},
	}
	for _, pool := range defaults {
		now := NowText()
		_, err := s.db.Exec(`INSERT OR IGNORE INTO stock_pools
			(id,name,symbols_json,description,created_at,updated_at)
			VALUES (?,?,?,?,?,?)`,
			pool.ID, pool.Name, "[]", pool.Description, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func NowText() string {
	return time.Now().Format(time.RFC3339)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intBool(v int) bool {
	return v != 0
}

func normalizeFormula(f Formula) Formula {
	f.Name = strings.TrimSpace(f.Name)
	f.Type = strings.TrimSpace(f.Type)
	if f.Type == "" {
		f.Type = "indicator"
	}
	f.Script = strings.TrimSpace(f.Script)
	if strings.TrimSpace(f.ArgsJSON) == "" {
		f.ArgsJSON = "[]"
	}
	f.Period = strings.TrimSpace(f.Period)
	if f.Period == "" {
		f.Period = "day"
	}
	if f.Right < 0 || f.Right > 2 {
		f.Right = 1
	}
	return f
}

func (s *AppStore) ListFormulas() ([]Formula, error) {
	rows, err := s.db.Query(`SELECT id,name,type,script,args_json,period,right,enabled,description,created_at,updated_at FROM formulas ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Formula
	for rows.Next() {
		var f Formula
		var enabled int
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &f.Script, &f.ArgsJSON, &f.Period, &f.Right, &enabled, &f.Description, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Enabled = intBool(enabled)
		list = append(list, f)
	}
	if list == nil {
		list = []Formula{}
	}
	return list, rows.Err()
}

func (s *AppStore) GetFormula(id string) (Formula, error) {
	var f Formula
	var enabled int
	err := s.db.QueryRow(`SELECT id,name,type,script,args_json,period,right,enabled,description,created_at,updated_at FROM formulas WHERE id=?`, id).
		Scan(&f.ID, &f.Name, &f.Type, &f.Script, &f.ArgsJSON, &f.Period, &f.Right, &enabled, &f.Description, &f.CreatedAt, &f.UpdatedAt)
	f.Enabled = intBool(enabled)
	return f, err
}

func (s *AppStore) UpsertFormula(f Formula) (Formula, error) {
	f = normalizeFormula(f)
	if f.Name == "" || f.Script == "" {
		return f, errors.New("公式名称和内容不能为空")
	}
	if !json.Valid([]byte(f.ArgsJSON)) {
		return f, errors.New("args_json不是有效JSON")
	}

	now := NowText()
	if f.ID == "" {
		f.ID = uuid.NewString()
		f.CreatedAt = now
	} else if f.CreatedAt == "" {
		old, err := s.GetFormula(f.ID)
		if err == nil {
			f.CreatedAt = old.CreatedAt
		} else {
			f.CreatedAt = now
		}
	}
	f.UpdatedAt = now

	_, err := s.db.Exec(`INSERT INTO formulas
		(id,name,type,script,args_json,period,right,enabled,description,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,type=excluded.type,script=excluded.script,args_json=excluded.args_json,
			period=excluded.period,right=excluded.right,enabled=excluded.enabled,description=excluded.description,
			updated_at=excluded.updated_at`,
		f.ID, f.Name, f.Type, f.Script, f.ArgsJSON, f.Period, f.Right, boolInt(f.Enabled), f.Description, f.CreatedAt, f.UpdatedAt)
	return f, err
}

func (s *AppStore) DeleteFormula(id string) error {
	_, err := s.db.Exec(`DELETE FROM formulas WHERE id=?`, id)
	return err
}

func (s *AppStore) ListStockPools() ([]StockPool, error) {
	systemPools := listMarketStockPools()
	rows, err := s.db.Query(`SELECT id,name,symbols_json,description,created_at,updated_at FROM stock_pools ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := append([]StockPool{}, systemPools...)
	for rows.Next() {
		var pool StockPool
		var symbols string
		if err := rows.Scan(&pool.ID, &pool.Name, &symbols, &pool.Description, &pool.CreatedAt, &pool.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(symbols), &pool.Symbols)
		pool.Category = "custom"
		if pool.ID == DecisionWatchPoolID || pool.ID == DecisionExcludePoolID {
			pool.Category = "decision"
			pool.System = true
		}
		list = append(list, pool)
	}
	if list == nil {
		list = []StockPool{}
	}
	return list, rows.Err()
}

func (s *AppStore) GetStockPool(id string) (StockPool, error) {
	if pool, ok := getMarketStockPool(id); ok {
		return pool, nil
	}
	var pool StockPool
	var symbols string
	err := s.db.QueryRow(`SELECT id,name,symbols_json,description,created_at,updated_at FROM stock_pools WHERE id=?`, id).
		Scan(&pool.ID, &pool.Name, &symbols, &pool.Description, &pool.CreatedAt, &pool.UpdatedAt)
	_ = json.Unmarshal([]byte(symbols), &pool.Symbols)
	pool.Category = "custom"
	if pool.ID == DecisionWatchPoolID || pool.ID == DecisionExcludePoolID {
		pool.Category = "decision"
		pool.System = true
	}
	return pool, err
}

func (s *AppStore) UpsertStockPool(pool StockPool) (StockPool, error) {
	if isMarketPoolID(pool.ID) {
		return pool, errors.New("系统市场分组不能编辑")
	}
	pool.Name = strings.TrimSpace(pool.Name)
	if pool.Name == "" {
		return pool, errors.New("股票池名称不能为空")
	}
	pool.Symbols = NormalizeSymbols(pool.Symbols)
	raw, err := json.Marshal(pool.Symbols)
	if err != nil {
		return pool, err
	}

	now := NowText()
	if pool.ID == "" {
		pool.ID = uuid.NewString()
		pool.CreatedAt = now
	} else if pool.CreatedAt == "" {
		old, err := s.GetStockPool(pool.ID)
		if err == nil {
			pool.CreatedAt = old.CreatedAt
		} else {
			pool.CreatedAt = now
		}
	}
	pool.UpdatedAt = now

	_, err = s.db.Exec(`INSERT INTO stock_pools
		(id,name,symbols_json,description,created_at,updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,symbols_json=excluded.symbols_json,description=excluded.description,updated_at=excluded.updated_at`,
		pool.ID, pool.Name, string(raw), pool.Description, pool.CreatedAt, pool.UpdatedAt)
	return pool, err
}

func (s *AppStore) DeleteStockPool(id string) error {
	if isMarketPoolID(id) {
		return errors.New("系统市场分组不能删除")
	}
	if id == DecisionWatchPoolID || id == DecisionExcludePoolID {
		return errors.New("系统股票池不能删除")
	}
	_, err := s.db.Exec(`DELETE FROM stock_pools WHERE id=?`, id)
	return err
}

func normalizeStrategy(item Strategy) Strategy {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	if strings.TrimSpace(item.ConfigJSON) == "" {
		item.ConfigJSON = "{}"
	}
	return item
}

func (s *AppStore) ListStrategies() ([]Strategy, error) {
	rows, err := s.db.Query(`SELECT id,name,description,config_json,enabled,readonly,created_at,updated_at FROM strategies ORDER BY readonly DESC, updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Strategy{}
	for rows.Next() {
		var item Strategy
		var enabled, readonly int
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.ConfigJSON, &enabled, &readonly, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = intBool(enabled)
		item.Readonly = intBool(readonly)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) GetStrategy(id string) (Strategy, error) {
	var item Strategy
	var enabled, readonly int
	err := s.db.QueryRow(`SELECT id,name,description,config_json,enabled,readonly,created_at,updated_at FROM strategies WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.Description, &item.ConfigJSON, &enabled, &readonly, &item.CreatedAt, &item.UpdatedAt)
	item.Enabled = intBool(enabled)
	item.Readonly = intBool(readonly)
	return item, err
}

func (s *AppStore) UpsertStrategy(item Strategy) (Strategy, error) {
	item = normalizeStrategy(item)
	if item.Name == "" {
		return item, errors.New("策略名称不能为空")
	}
	if !json.Valid([]byte(item.ConfigJSON)) {
		return item, errors.New("config_json不是有效JSON")
	}
	if item.ID != "" {
		old, err := s.GetStrategy(item.ID)
		if err == nil && old.Readonly {
			return item, errors.New("系统策略模板不能编辑，请复制后修改")
		}
	}
	now := NowText()
	if item.ID == "" {
		item.ID = uuid.NewString()
		item.CreatedAt = now
	} else if item.CreatedAt == "" {
		old, err := s.GetStrategy(item.ID)
		if err == nil {
			item.CreatedAt = old.CreatedAt
		} else {
			item.CreatedAt = now
		}
	}
	item.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO strategies
		(id,name,description,config_json,enabled,readonly,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,description=excluded.description,config_json=excluded.config_json,
			enabled=excluded.enabled,updated_at=excluded.updated_at`,
		item.ID, item.Name, item.Description, item.ConfigJSON, boolInt(item.Enabled), boolInt(item.Readonly), item.CreatedAt, item.UpdatedAt)
	return item, err
}

// SetStrategyUniverseConfig is intentionally narrower than UpsertStrategy: it
// permits the candidate-universe update path for system templates while their
// name, factors, scores, thresholds, and other settings remain immutable.
func (s *AppStore) SetStrategyUniverseConfig(id, configJSON string) (Strategy, error) {
	if !json.Valid([]byte(configJSON)) {
		return Strategy{}, errors.New("config_json不是有效JSON")
	}
	item, err := s.GetStrategy(id)
	if err != nil {
		return Strategy{}, err
	}
	item.ConfigJSON = configJSON
	item.UpdatedAt = NowText()
	if _, err := s.db.Exec(`UPDATE strategies SET config_json=?,updated_at=? WHERE id=?`, item.ConfigJSON, item.UpdatedAt, id); err != nil {
		return Strategy{}, err
	}
	return item, nil
}

func (s *AppStore) DeleteStrategy(id string) error {
	item, err := s.GetStrategy(id)
	if err != nil {
		return err
	}
	if item.Readonly {
		return errors.New("系统策略模板不能删除")
	}
	_, err = s.db.Exec(`DELETE FROM strategies WHERE id=?`, id)
	return err
}

func (s *AppStore) CloneStrategy(id string) (Strategy, error) {
	item, err := s.GetStrategy(id)
	if err != nil {
		return Strategy{}, err
	}
	item.ID = ""
	item.Name = item.Name + " 副本"
	item.Readonly = false
	return s.UpsertStrategy(item)
}

func (s *AppStore) AddStockPoolSymbol(id, symbol string) (StockPool, error) {
	if isMarketPoolID(id) {
		return StockPool{}, errors.New("系统市场分组不能手动增删股票")
	}
	pool, err := s.GetStockPool(id)
	if err != nil {
		return pool, err
	}
	symbols := append(pool.Symbols, symbol)
	pool.Symbols = NormalizeSymbols(symbols)
	return s.UpsertStockPool(pool)
}

func (s *AppStore) RemoveStockPoolSymbol(id, symbol string) (StockPool, error) {
	if isMarketPoolID(id) {
		return StockPool{}, errors.New("系统市场分组不能手动增删股票")
	}
	pool, err := s.GetStockPool(id)
	if err != nil {
		return pool, err
	}
	targets := NormalizeSymbols([]string{symbol})
	if len(targets) == 0 {
		return pool, errors.New("股票代码不能为空")
	}
	target := targets[0]
	next := make([]string, 0, len(pool.Symbols))
	for _, item := range NormalizeSymbols(pool.Symbols) {
		if item != target {
			next = append(next, item)
		}
	}
	pool.Symbols = next
	return s.UpsertStockPool(pool)
}

func (s *AppStore) GetDecisionNote(symbol string) (DecisionNote, error) {
	symbols := NormalizeSymbols([]string{symbol})
	if len(symbols) == 0 {
		return DecisionNote{}, errors.New("股票代码不能为空")
	}
	var note DecisionNote
	err := s.db.QueryRow(`SELECT symbol,status,added_price,add_reason,plan_buy,stop_loss,review_note,exclude_category,exclude_reason,created_at,updated_at
		FROM decision_notes WHERE symbol=?`, symbols[0]).
		Scan(&note.Symbol, &note.Status, &note.AddedPrice, &note.AddReason, &note.PlanBuy, &note.StopLoss, &note.ReviewNote, &note.ExcludeCategory, &note.ExcludeReason, &note.CreatedAt, &note.UpdatedAt)
	return note, err
}

func (s *AppStore) UpsertDecisionNote(note DecisionNote) (DecisionNote, error) {
	symbols := NormalizeSymbols([]string{note.Symbol})
	if len(symbols) == 0 {
		return note, errors.New("股票代码不能为空")
	}
	note.Symbol = symbols[0]
	note.Status = strings.TrimSpace(note.Status)
	note.AddReason = strings.TrimSpace(note.AddReason)
	note.ReviewNote = strings.TrimSpace(note.ReviewNote)
	note.ExcludeCategory = strings.TrimSpace(note.ExcludeCategory)
	note.ExcludeReason = strings.TrimSpace(note.ExcludeReason)
	now := NowText()
	if note.CreatedAt == "" {
		old, err := s.GetDecisionNote(note.Symbol)
		if err == nil {
			note.CreatedAt = old.CreatedAt
		} else {
			note.CreatedAt = now
		}
	}
	note.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO decision_notes
		(symbol,status,added_price,add_reason,plan_buy,stop_loss,review_note,exclude_category,exclude_reason,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(symbol) DO UPDATE SET
			status=excluded.status,added_price=excluded.added_price,add_reason=excluded.add_reason,
			plan_buy=excluded.plan_buy,stop_loss=excluded.stop_loss,review_note=excluded.review_note,
			exclude_category=excluded.exclude_category,exclude_reason=excluded.exclude_reason,updated_at=excluded.updated_at`,
		note.Symbol, note.Status, note.AddedPrice, note.AddReason, note.PlanBuy, note.StopLoss, note.ReviewNote, note.ExcludeCategory, note.ExcludeReason, note.CreatedAt, note.UpdatedAt)
	return note, err
}

func normalizeTradingSystemState(state TradingSystemState) TradingSystemState {
	state.Filter = strings.TrimSpace(state.Filter)
	if state.Filter == "" {
		state.Filter = "all"
	}
	state.Account = TradingAccount{
		Principal:         state.Account.Principal,
		TotalAssets:       state.Account.TotalAssets,
		MarketValue:       state.Account.MarketValue,
		DailyProfit:       state.Account.DailyProfit,
		MaxTradeRisk:      state.Account.MaxTradeRisk,
		MaxPositionWeight: state.Account.MaxPositionWeight,
	}
	state.Discipline = TradingDiscipline{
		Reason:    state.Discipline.Reason,
		Invalid:   state.Discipline.Invalid,
		Risk:      state.Discipline.Risk,
		NoImpulse: state.Discipline.NoImpulse,
	}
	state.Fees = normalizeTradingFeeConfig(state.Fees)
	trades := make([]TradingTrade, 0, len(state.Trades))
	for _, trade := range state.Trades {
		trade.ID = strings.TrimSpace(trade.ID)
		if trade.ID == "" {
			trade.ID = uuid.NewString()
		}
		trade.StockName = strings.TrimSpace(trade.StockName)
		trade.StockCode = strings.TrimSpace(trade.StockCode)
		trade.Direction = strings.ToLower(strings.TrimSpace(trade.Direction))
		if trade.Direction != "sell" {
			trade.Direction = "buy"
		}
		trade.Status = strings.TrimSpace(trade.Status)
		if trade.Status == "" {
			trade.Status = "active"
		}
		trade.EntryDate = strings.TrimSpace(trade.EntryDate)
		trade.ExitDate = strings.TrimSpace(trade.ExitDate)
		trade.PositionLabel = strings.TrimSpace(trade.PositionLabel)
		if trade.PositionLabel == "" {
			trade.PositionLabel = "试错仓"
		}
		trade.TargetOne = strings.TrimSpace(trade.TargetOne)
		trade.TargetTwo = strings.TrimSpace(trade.TargetTwo)
		trade.TradeMode = strings.TrimSpace(trade.TradeMode)
		trade.BuyReason = strings.TrimSpace(trade.BuyReason)
		trade.ExitRules = strings.TrimSpace(trade.ExitRules)
		trade.Review = strings.TrimSpace(trade.Review)
		trades = append(trades, trade)
	}
	state.Trades = trades
	if state.Trades == nil {
		state.Trades = []TradingTrade{}
	}
	return state
}

func normalizeTradingFeeConfig(fees TradingFeeConfig) TradingFeeConfig {
	if fees.BuyCommissionRate < 0 {
		fees.BuyCommissionRate = 0
	}
	if fees.SellCommissionRate < 0 {
		fees.SellCommissionRate = 0
	}
	if fees.StampTaxRate < 0 {
		fees.StampTaxRate = 0
	}
	if fees.TransferFeeRate < 0 {
		fees.TransferFeeRate = 0
	}
	if fees.MinCommission < 0 {
		fees.MinCommission = 0
	}
	if fees.BuyCommissionRate == 0 && fees.SellCommissionRate == 0 && fees.StampTaxRate == 0 && fees.TransferFeeRate == 0 && fees.MinCommission == 0 {
		return TradingFeeConfig{
			BuyCommissionRate:  0.03,
			SellCommissionRate: 0.03,
			StampTaxRate:       0.05,
			TransferFeeRate:    0.001,
			MinCommission:      5,
		}
	}
	return fees
}

func (s *AppStore) GetTradingSystemState() (TradingSystemState, error) {
	var state TradingSystemState
	var raw string
	var createdAt, updatedAt string
	err := s.db.QueryRow(`SELECT state_json,created_at,updated_at FROM trading_system_state WHERE id=?`, TradingSystemStateID).
		Scan(&raw, &createdAt, &updatedAt)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, err
	}
	state = normalizeTradingSystemState(state)
	state.CreatedAt = createdAt
	state.UpdatedAt = updatedAt
	if state.Trades == nil {
		state.Trades = []TradingTrade{}
	}
	return state, nil
}

func (s *AppStore) UpsertTradingSystemState(state TradingSystemState) (TradingSystemState, error) {
	state = normalizeTradingSystemState(state)
	if err := validateTradingDirections(state.Trades); err != nil {
		return state, err
	}
	now := NowText()
	old, err := s.GetTradingSystemState()
	if err == nil {
		state.CreatedAt = old.CreatedAt
	} else {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	raw, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	_, err = s.db.Exec(`INSERT INTO trading_system_state
		(id,state_json,created_at,updated_at)
		VALUES (?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			state_json=excluded.state_json,updated_at=excluded.updated_at`,
		TradingSystemStateID, string(raw), state.CreatedAt, state.UpdatedAt)
	return state, err
}

// TSignalStateRecord 日内做T信号状态的通用持久化记录(JSON 文本)。
// 完整结构由 selection-worker 的信号引擎定义,本包只负责存取。
type TSignalStateRecord struct {
	ID        string `json:"id"`
	StateJSON string `json:"state_json"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func (s *AppStore) GetTSignalState(id string) (TSignalStateRecord, error) {
	if strings.TrimSpace(id) == "" {
		id = "default"
	}
	var record TSignalStateRecord
	err := s.db.QueryRow(`SELECT id,state_json,created_at,updated_at FROM t_signal_state WHERE id=?`, id).
		Scan(&record.ID, &record.StateJSON, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return record, err
	}
	return record, nil
}

func (s *AppStore) UpsertTSignalState(record TSignalStateRecord) (TSignalStateRecord, error) {
	if strings.TrimSpace(record.ID) == "" {
		record.ID = "default"
	}
	if record.StateJSON == "" {
		record.StateJSON = "{}"
	}
	now := NowText()
	old, err := s.GetTSignalState(record.ID)
	if err == nil {
		record.CreatedAt = old.CreatedAt
	} else {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	_, err = s.db.Exec(`INSERT INTO t_signal_state
		(id,state_json,created_at,updated_at)
		VALUES (?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			state_json=excluded.state_json,updated_at=excluded.updated_at`,
		record.ID, record.StateJSON, record.CreatedAt, record.UpdatedAt)
	return record, err
}

func validateTradingDirections(trades []TradingTrade) error {
	holdings := make(map[string]float64)
	for _, trade := range trades {
		if trade.Direction == "sell" || trade.Status != "active" {
			continue
		}
		holdings[trade.StockCode] += trade.Shares
	}
	for _, trade := range trades {
		if trade.Direction != "sell" {
			continue
		}
		if trade.Shares < 0 {
			return fmt.Errorf("%s 卖出数量不能为负数", trade.StockName)
		}
		available := holdings[trade.StockCode]
		if trade.Shares > available+0.000001 {
			return fmt.Errorf("%s 卖出数量 %.0f 股超过当前可用仓位 %.0f 股", trade.StockName, trade.Shares, available)
		}
		holdings[trade.StockCode] = available - trade.Shares
	}
	return nil
}

func (s *AppStore) SetDecisionStatus(symbol, status string) error {
	symbols := NormalizeSymbols([]string{symbol})
	if len(symbols) == 0 {
		return errors.New("股票代码不能为空")
	}
	note, err := s.GetDecisionNote(symbols[0])
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		note = DecisionNote{Symbol: symbols[0]}
	}
	note.Status = strings.TrimSpace(status)
	_, err = s.UpsertDecisionNote(note)
	return err
}

func (s *AppStore) ListDecisionNotes(status string, limit int) ([]DecisionNote, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `SELECT symbol,status,added_price,add_reason,plan_buy,stop_loss,review_note,exclude_category,exclude_reason,created_at,updated_at FROM decision_notes`
	args := []interface{}{}
	if strings.TrimSpace(status) != "" {
		query += ` WHERE status=?`
		args = append(args, strings.TrimSpace(status))
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []DecisionNote
	for rows.Next() {
		var note DecisionNote
		if err := rows.Scan(&note.Symbol, &note.Status, &note.AddedPrice, &note.AddReason, &note.PlanBuy, &note.StopLoss, &note.ReviewNote, &note.ExcludeCategory, &note.ExcludeReason, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, note)
	}
	if list == nil {
		list = []DecisionNote{}
	}
	return list, rows.Err()
}

func NormalizeSymbols(symbols []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		s := strings.TrimSpace(strings.ToUpper(symbol))
		s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, ".SH"), ".SZ"), ".BJ")
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}
	return result
}

func normalizeTask(t AutomationTask) AutomationTask {
	t.Name = strings.TrimSpace(t.Name)
	t.Type = strings.TrimSpace(t.Type)
	if t.Type == "" {
		t.Type = "custom"
	}
	t.Cron = strings.TrimSpace(t.Cron)
	if strings.TrimSpace(t.PayloadJSON) == "" {
		t.PayloadJSON = "{}"
	}
	if strings.TrimSpace(t.WebhookIDs) == "" {
		t.WebhookIDs = "[]"
	}
	return t
}

func (s *AppStore) ListAutomationTasks() ([]AutomationTask, error) {
	rows, err := s.db.Query(`SELECT id,name,type,cron,enabled,only_trade_day,payload_json,webhook_ids,last_run_at,next_run_at,last_status,last_message,created_at,updated_at FROM automation_tasks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AutomationTask
	for rows.Next() {
		var t AutomationTask
		var enabled int
		var onlyTradeDay int
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &t.Cron, &enabled, &onlyTradeDay, &t.PayloadJSON, &t.WebhookIDs, &t.LastRunAt, &t.NextRunAt, &t.LastStatus, &t.LastMessage, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Enabled = intBool(enabled)
		t.OnlyTradeDay = intBool(onlyTradeDay)
		t = decorateAutomationTask(t)
		list = append(list, t)
	}
	if list == nil {
		list = []AutomationTask{}
	}
	return list, rows.Err()
}

func (s *AppStore) GetAutomationTask(id string) (AutomationTask, error) {
	var t AutomationTask
	var enabled int
	var onlyTradeDay int
	err := s.db.QueryRow(`SELECT id,name,type,cron,enabled,only_trade_day,payload_json,webhook_ids,last_run_at,next_run_at,last_status,last_message,created_at,updated_at FROM automation_tasks WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Type, &t.Cron, &enabled, &onlyTradeDay, &t.PayloadJSON, &t.WebhookIDs, &t.LastRunAt, &t.NextRunAt, &t.LastStatus, &t.LastMessage, &t.CreatedAt, &t.UpdatedAt)
	t.Enabled = intBool(enabled)
	t.OnlyTradeDay = intBool(onlyTradeDay)
	return decorateAutomationTask(t), err
}

func (s *AppStore) UpsertAutomationTask(t AutomationTask) (AutomationTask, error) {
	t = normalizeTask(t)
	if t.Name == "" || t.Cron == "" {
		return t, errors.New("任务名称和cron不能为空")
	}
	if !json.Valid([]byte(t.PayloadJSON)) {
		return t, errors.New("payload_json不是有效JSON")
	}
	if !json.Valid([]byte(t.WebhookIDs)) {
		return t, errors.New("webhook_ids不是有效JSON数组")
	}

	now := NowText()
	if t.ID == "" {
		t.ID = uuid.NewString()
		t.CreatedAt = now
	} else if t.CreatedAt == "" {
		old, err := s.GetAutomationTask(t.ID)
		if err == nil {
			t.CreatedAt = old.CreatedAt
		} else {
			t.CreatedAt = now
		}
	}
	t.UpdatedAt = now

	_, err := s.db.Exec(`INSERT INTO automation_tasks
		(id,name,type,cron,enabled,only_trade_day,payload_json,webhook_ids,last_run_at,next_run_at,last_status,last_message,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,type=excluded.type,cron=excluded.cron,enabled=excluded.enabled,only_trade_day=excluded.only_trade_day,payload_json=excluded.payload_json,
			webhook_ids=excluded.webhook_ids,next_run_at=excluded.next_run_at,updated_at=excluded.updated_at`,
		t.ID, t.Name, t.Type, t.Cron, boolInt(t.Enabled), boolInt(t.OnlyTradeDay), t.PayloadJSON, t.WebhookIDs, t.LastRunAt, t.NextRunAt, t.LastStatus, t.LastMessage, t.CreatedAt, t.UpdatedAt)
	return t, err
}

func (s *AppStore) SetAutomationTaskEnabled(id string, enabled bool) (AutomationTask, error) {
	now := NowText()
	nextRun := ""
	if enabled {
		current, err := s.GetAutomationTask(id)
		if err == nil {
			nextRun = current.NextRunAt
		}
	}
	result, err := s.db.Exec(`UPDATE automation_tasks SET enabled=?, next_run_at=?, updated_at=? WHERE id=?`, boolInt(enabled), nextRun, now, id)
	if err != nil {
		return AutomationTask{}, err
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		return AutomationTask{}, sql.ErrNoRows
	}
	return s.GetAutomationTask(id)
}

func (s *AppStore) DeleteAutomationTask(id string) error {
	_, err := s.db.Exec(`DELETE FROM automation_tasks WHERE id=?`, id)
	return err
}

func (s *AppStore) CreateAutomationRun(task AutomationTask, parentRunID ...string) (AutomationRun, error) {
	parentID := ""
	if len(parentRunID) > 0 {
		parentID = strings.TrimSpace(parentRunID[0])
	}
	run := AutomationRun{
		ID:          uuid.NewString(),
		ParentRunID: parentID,
		TaskID:      task.ID,
		TaskName:    task.Name,
		TaskType:    task.Type,
		Status:      "running",
		StartedAt:   NowText(),
		ResultJSON:  "{}",
	}
	_, err := s.db.Exec(`INSERT INTO automation_runs
		(id,parent_run_id,task_id,task_name,task_type,status,started_at,finished_at,log,result_json,matched_count)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		run.ID, run.ParentRunID, run.TaskID, run.TaskName, run.TaskType, run.Status, run.StartedAt, run.FinishedAt, run.Log, run.ResultJSON, run.MatchedCount)
	return run, err
}

func (s *AppStore) FinishAutomationRun(runID, status, logText, resultJSON string, matchedCount int) error {
	if strings.TrimSpace(resultJSON) == "" {
		resultJSON = "{}"
	}
	_, err := s.db.Exec(`UPDATE automation_runs SET status=?,finished_at=?,log=?,result_json=?,matched_count=? WHERE id=?`,
		status, NowText(), logText, resultJSON, matchedCount, runID)
	return err
}

func (s *AppStore) SaveSelectionResults(run AutomationRun, formula Formula, items []SelectionResult) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A run/formula is written atomically. Once it contains signals, a retry
	// must preserve their original timestamps, IDs, executions, and observations.
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM selection_results WHERE run_id=? AND formula_id=?`, run.ID, formula.ID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return tx.Commit()
	}
	if len(items) == 0 {
		return tx.Commit()
	}
	seenBusinessKeys := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		item.RunID = run.ID
		item.TaskID = run.TaskID
		item.TaskName = run.TaskName
		item.FormulaID = formula.ID
		item.FormulaName = formula.Name
		if item.CreatedAt == "" {
			item.CreatedAt = NowText()
		}
		if strings.TrimSpace(item.DetailJSON) == "" {
			item.DetailJSON = "{}"
		}
		normalizeSelectionSnapshot(&item)
		businessKey := strings.Join([]string{item.RunID, item.FormulaID, item.Symbol, item.SignalAt}, "\x00")
		if _, exists := seenBusinessKeys[businessKey]; exists {
			continue
		}
		seenBusinessKeys[businessKey] = struct{}{}
		if _, err := tx.Exec(`INSERT INTO selection_results
			(id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
			 signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
			 detail_json,created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			item.ID, item.RunID, item.TaskID, item.TaskName, item.FormulaID, item.FormulaName, item.Symbol, item.Latest,
			item.SignalAt, item.SignalPrice, item.QuoteAt, item.QuoteSource, item.PriceType, item.QuoteAgeMS, item.SignalScore,
			item.StrategyVersion, item.MarketContextJSON, item.SnapshotStatus, item.DetailJSON, item.CreatedAt); err != nil {
			return err
		}
		if item.Tracking != nil {
			if err := syncSelectionResultTrackingTx(tx, item, *item.Tracking); err != nil {
				return err
			}
		}
		for _, execution := range BuildSelectionSignalExecutions(item, nil, time.Now()) {
			if err := upsertSelectionSignalExecutionTx(tx, execution); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func upsertSelectionSignalExecutionTx(tx *sql.Tx, item SelectionSignalExecution) error {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	_, err := tx.Exec(`INSERT INTO selection_signal_executions
		(id,selection_result_id,run_id,formula_id,symbol,policy_name,policy_version,execution_version,side,status,
		 signal_at,signal_price,planned_at,entry_at,reference_price,fill_price,quantity,slippage_bps,gross_amount,commission,stamp_duty,transfer_fee,total_cost,
		 price_source,failure_reason,data_quality,strategy_version,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(selection_result_id,policy_name,policy_version) DO UPDATE SET
			run_id=COALESCE(NULLIF(excluded.run_id,''),selection_signal_executions.run_id),
			formula_id=COALESCE(NULLIF(excluded.formula_id,''),selection_signal_executions.formula_id),
			symbol=excluded.symbol,execution_version=excluded.execution_version,side=excluded.side,status=excluded.status,
			signal_at=excluded.signal_at,signal_price=excluded.signal_price,planned_at=excluded.planned_at,entry_at=excluded.entry_at,reference_price=excluded.reference_price,fill_price=excluded.fill_price,
			quantity=excluded.quantity,slippage_bps=excluded.slippage_bps,gross_amount=excluded.gross_amount,commission=excluded.commission,stamp_duty=excluded.stamp_duty,transfer_fee=excluded.transfer_fee,total_cost=excluded.total_cost,
				price_source=excluded.price_source,failure_reason=excluded.failure_reason,data_quality=excluded.data_quality,strategy_version=excluded.strategy_version,updated_at=excluded.updated_at
			WHERE selection_signal_executions.status NOT IN ('filled', 'rejected', 'unavailable')`,
		item.ID, item.SelectionResultID, item.RunID, item.FormulaID, item.Symbol, item.PolicyName, item.PolicyVersion, item.ExecutionVersion, item.Side, item.Status,
		item.SignalAt, item.SignalPrice, item.PlannedAt, item.EntryAt, item.ReferencePrice, item.FillPrice, item.Quantity, item.SlippageBPS, item.GrossAmount, item.Commission, item.StampDuty, item.TransferFee, item.TotalCost,
		item.PriceSource, item.FailureReason, item.DataQuality, item.StrategyVersion, item.CreatedAt, item.UpdatedAt)
	return err
}

// EnsureSelectionSignalExecutions resolves the two paper policies once the
// worker has loaded forward daily bars. Re-running it is idempotent.
func (s *AppStore) EnsureSelectionSignalExecutions(items []SelectionResult, bars map[string][]TrackingBar) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	for _, item := range items {
		for _, execution := range BuildSelectionSignalExecutions(item, bars[strings.ToUpper(item.Symbol)], now) {
			if err := upsertSelectionSignalExecutionTx(tx, execution); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *AppStore) ListSelectionSignalExecutions(selectionResultID, policy string) ([]SelectionSignalExecution, error) {
	query := `SELECT id,selection_result_id,run_id,formula_id,symbol,policy_name,policy_version,execution_version,side,status,
		signal_at,signal_price,planned_at,entry_at,reference_price,fill_price,quantity,slippage_bps,gross_amount,commission,stamp_duty,transfer_fee,total_cost,
		price_source,failure_reason,data_quality,strategy_version,created_at,updated_at
		FROM selection_signal_executions WHERE selection_result_id=?`
	args := []interface{}{selectionResultID}
	if strings.TrimSpace(policy) != "" {
		query += " AND policy_name=?"
		args = append(args, policy)
	}
	query += " ORDER BY policy_name"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SelectionSignalExecution{}
	for rows.Next() {
		var item SelectionSignalExecution
		if err := rows.Scan(&item.ID, &item.SelectionResultID, &item.RunID, &item.FormulaID, &item.Symbol, &item.PolicyName, &item.PolicyVersion, &item.ExecutionVersion, &item.Side, &item.Status,
			&item.SignalAt, &item.SignalPrice, &item.PlannedAt, &item.EntryAt, &item.ReferencePrice, &item.FillPrice, &item.Quantity, &item.SlippageBPS, &item.GrossAmount, &item.Commission, &item.StampDuty, &item.TransferFee, &item.TotalCost,
			&item.PriceSource, &item.FailureReason, &item.DataQuality, &item.StrategyVersion, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) SaveSelectionSignalObservations(rows []SelectionSignalObservation) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range rows {
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		if item.SessionKind == "" {
			item.SessionKind = "forward"
		}
		if err := upsertSelectionSignalObservationTx(tx, item); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertSelectionSignalObservationTx(tx *sql.Tx, item SelectionSignalObservation) error {
	_, err := tx.Exec(`INSERT INTO selection_signal_observations
		(id,selection_result_id,execution_id,symbol,trade_date,market_day_index,symbol_day_index,session_kind,open,high,low,close,volume,amount,
		 signal_open_return,signal_high_return,signal_low_return,signal_close_return,trade_open_return,trade_high_return,trade_low_return,trade_close_return,
		 benchmark_symbol,benchmark_close_return,excess_close_return,adjustment_type,data_status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(selection_result_id,execution_id,trade_date,session_kind) DO UPDATE SET
			symbol=excluded.symbol,market_day_index=excluded.market_day_index,symbol_day_index=excluded.symbol_day_index,open=excluded.open,high=excluded.high,low=excluded.low,close=excluded.close,volume=excluded.volume,amount=excluded.amount,
			signal_open_return=excluded.signal_open_return,signal_high_return=excluded.signal_high_return,signal_low_return=excluded.signal_low_return,signal_close_return=excluded.signal_close_return,
			trade_open_return=excluded.trade_open_return,trade_high_return=excluded.trade_high_return,trade_low_return=excluded.trade_low_return,trade_close_return=excluded.trade_close_return,
			benchmark_symbol=excluded.benchmark_symbol,benchmark_close_return=excluded.benchmark_close_return,excess_close_return=excluded.excess_close_return,adjustment_type=excluded.adjustment_type,data_status=excluded.data_status,updated_at=excluded.updated_at`,
		item.ID, item.SelectionResultID, item.ExecutionID, item.Symbol, item.TradeDate, item.MarketDayIndex, item.SymbolDayIndex, item.SessionKind, item.Open, item.High, item.Low, item.Close, item.Volume, item.Amount,
		item.SignalOpenReturn, item.SignalHighReturn, item.SignalLowReturn, item.SignalCloseReturn, item.TradeOpenReturn, item.TradeHighReturn, item.TradeLowReturn, item.TradeCloseReturn,
		item.BenchmarkSymbol, item.BenchmarkCloseReturn, item.ExcessCloseReturn, item.AdjustmentType, item.DataStatus, item.CreatedAt, item.UpdatedAt)
	return err
}

func (s *AppStore) ListSelectionSignalObservations(selectionResultID, executionID string) ([]SelectionSignalObservation, error) {
	query := `SELECT id,selection_result_id,execution_id,symbol,trade_date,market_day_index,symbol_day_index,session_kind,open,high,low,close,volume,amount,
		signal_open_return,signal_high_return,signal_low_return,signal_close_return,trade_open_return,trade_high_return,trade_low_return,trade_close_return,
		benchmark_symbol,benchmark_close_return,excess_close_return,adjustment_type,data_status,created_at,updated_at
		FROM selection_signal_observations WHERE selection_result_id=?`
	args := []interface{}{selectionResultID}
	if strings.TrimSpace(executionID) != "" {
		query += " AND execution_id=?"
		args = append(args, executionID)
	}
	query += " ORDER BY trade_date,symbol_day_index"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SelectionSignalObservation{}
	for rows.Next() {
		var item SelectionSignalObservation
		if err := rows.Scan(&item.ID, &item.SelectionResultID, &item.ExecutionID, &item.Symbol, &item.TradeDate, &item.MarketDayIndex, &item.SymbolDayIndex, &item.SessionKind, &item.Open, &item.High, &item.Low, &item.Close, &item.Volume, &item.Amount,
			&item.SignalOpenReturn, &item.SignalHighReturn, &item.SignalLowReturn, &item.SignalCloseReturn, &item.TradeOpenReturn, &item.TradeHighReturn, &item.TradeLowReturn, &item.TradeCloseReturn,
			&item.BenchmarkSymbol, &item.BenchmarkCloseReturn, &item.ExcessCloseReturn, &item.AdjustmentType, &item.DataStatus, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) ListSelectionSignalPath(selectionResultID string) ([]SelectionSignalObservation, error) {
	rows, err := s.db.Query(`SELECT id,selection_result_id,execution_id,symbol,trade_date,market_day_index,symbol_day_index,session_kind,open,high,low,close,volume,amount,
		signal_open_return,signal_high_return,signal_low_return,signal_close_return,trade_open_return,trade_high_return,trade_low_return,trade_close_return,
		benchmark_symbol,benchmark_close_return,excess_close_return,adjustment_type,data_status,created_at,updated_at
		FROM selection_signal_observations WHERE selection_result_id=? AND execution_id='' ORDER BY trade_date,symbol_day_index`, selectionResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SelectionSignalObservation{}
	for rows.Next() {
		var item SelectionSignalObservation
		if err := rows.Scan(&item.ID, &item.SelectionResultID, &item.ExecutionID, &item.Symbol, &item.TradeDate, &item.MarketDayIndex, &item.SymbolDayIndex, &item.SessionKind, &item.Open, &item.High, &item.Low, &item.Close, &item.Volume, &item.Amount,
			&item.SignalOpenReturn, &item.SignalHighReturn, &item.SignalLowReturn, &item.SignalCloseReturn, &item.TradeOpenReturn, &item.TradeHighReturn, &item.TradeLowReturn, &item.TradeCloseReturn,
			&item.BenchmarkSymbol, &item.BenchmarkCloseReturn, &item.ExcessCloseReturn, &item.AdjustmentType, &item.DataStatus, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// normalizeSelectionSnapshot preserves the original selection row while making
// the signal price/time semantics explicit for legacy and newly generated rows.
func normalizeSelectionSnapshot(item *SelectionResult) {
	if item.SignalAt == "" {
		item.SignalAt = item.CreatedAt
	}
	if item.SignalPrice <= 0 {
		item.SignalPrice = item.Latest
	}
	if item.PriceType == "" {
		item.PriceType = "latest"
	}
	if item.SignalScore == 0 {
		item.SignalScore = selectionResultScore(item.DetailJSON)
	}
	if item.StrategyVersion == "" {
		item.StrategyVersion = item.FormulaID
	}
	if strings.TrimSpace(item.MarketContextJSON) == "" {
		item.MarketContextJSON = "{}"
	}
	if item.SnapshotStatus == "" {
		if item.SignalPrice > 0 {
			item.SnapshotStatus = "inferred"
		} else {
			item.SnapshotStatus = "missing"
		}
	}
	if (item.SnapshotStatus == "fresh" || item.SnapshotStatus == "valid" || item.SnapshotStatus == "stale") &&
		(item.SignalPrice <= 0 || strings.TrimSpace(item.QuoteAt) == "" || strings.TrimSpace(item.QuoteSource) == "") {
		if item.SignalPrice > 0 {
			item.SnapshotStatus = "inferred"
		} else {
			item.SnapshotStatus = "missing"
		}
	}
}

func (s *AppStore) GetAutomationRun(runID string) (AutomationRun, error) {
	var run AutomationRun
	err := s.db.QueryRow(`SELECT id,parent_run_id,task_id,task_name,task_type,status,started_at,finished_at,log,result_json,matched_count FROM automation_runs WHERE id=?`, runID).
		Scan(&run.ID, &run.ParentRunID, &run.TaskID, &run.TaskName, &run.TaskType, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Log, &run.ResultJSON, &run.MatchedCount)
	return run, err
}

func (s *AppStore) UpdateTaskRunState(taskID, status, message string) error {
	_, err := s.db.Exec(`UPDATE automation_tasks SET last_run_at=?,last_status=?,last_message=?,updated_at=? WHERE id=?`,
		NowText(), status, message, NowText(), taskID)
	return err
}

func (s *AppStore) UpdateTaskNextRun(taskID, nextRun string) error {
	_, err := s.db.Exec(`UPDATE automation_tasks SET next_run_at=?,updated_at=? WHERE id=?`, nextRun, NowText(), taskID)
	return err
}

func (s *AppStore) ListAutomationRuns(taskID string, limit int) ([]AutomationRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id,parent_run_id,task_id,task_name,task_type,status,started_at,finished_at,log,result_json,matched_count FROM automation_runs`
	args := []interface{}{}
	if taskID != "" {
		query += ` WHERE task_id=?`
		args = append(args, taskID)
	}
	query += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AutomationRun
	for rows.Next() {
		var run AutomationRun
		if err := rows.Scan(&run.ID, &run.ParentRunID, &run.TaskID, &run.TaskName, &run.TaskType, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Log, &run.ResultJSON, &run.MatchedCount); err != nil {
			return nil, err
		}
		list = append(list, run)
	}
	if list == nil {
		list = []AutomationRun{}
	}
	return list, rows.Err()
}

func (s *AppStore) ListSelectionResults(taskID, formulaID, symbol string, onlyLatest bool, limit int) ([]SelectionResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 5000 {
		limit = 5000
	}
	query := `SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
		signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
		detail_json,created_at FROM selection_results`
	args := []interface{}{}
	conds := []string{}
	if taskID != HistoricalBacktestTaskID {
		conds = append(conds, "task_id<>?")
		args = append(args, HistoricalBacktestTaskID)
	}
	if taskID != "" {
		conds = append(conds, "task_id=?")
		args = append(args, taskID)
	}
	if formulaID != "" {
		conds = append(conds, "formula_id=?")
		args = append(args, formulaID)
	}
	if symbol != "" {
		conds = append(conds, "symbol=?")
		args = append(args, strings.ToUpper(symbol))
	}
	if onlyLatest {
		conds = append(conds, `run_id=(SELECT run_id FROM selection_results WHERE task_id<>? ORDER BY created_at DESC LIMIT 1)`)
		args = append(args, HistoricalBacktestTaskID)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SelectionResult
	for rows.Next() {
		var item SelectionResult
		if err := scanSelectionResult(&item, rows); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	if list == nil {
		list = []SelectionResult{}
	}
	queryErr := rows.Err()
	_ = rows.Close()
	if err := s.hydrateSelectionResultsTracking(list); err != nil {
		return nil, err
	}
	return list, queryErr
}

func (s *AppStore) GetSelectionResult(id string) (SelectionResult, error) {
	item, err := scanSelectionResultRow(s.db.QueryRow(`SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
		signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
		detail_json,created_at FROM selection_results WHERE id=?`, id))
	if err != nil {
		return SelectionResult{}, err
	}
	items := []SelectionResult{item}
	if err := s.hydrateSelectionResultsTracking(items); err != nil {
		return SelectionResult{}, err
	}
	return items[0], nil
}

// ListIncompleteSelectionTrackingResults returns every selection result whose
// requested tracking horizons can still change. Complete and terminally
// unavailable horizons are left out of the daily refresh queue.
func (s *AppStore) ListIncompleteSelectionTrackingResults(horizons []int) ([]SelectionResult, error) {
	seen := map[int]bool{}
	normalized := make([]int, 0, len(horizons))
	for _, horizon := range horizons {
		if horizon <= 0 || seen[horizon] {
			continue
		}
		seen[horizon] = true
		normalized = append(normalized, horizon)
	}
	if len(normalized) == 0 {
		normalized = []int{3, 5, 10}
	}

	structuredChecks := make([]string, 0, len(normalized))
	for range normalized {
		structuredChecks = append(structuredChecks, `NOT EXISTS (
			SELECT 1 FROM selection_result_tracking AS tr
			WHERE tr.selection_result_id=sr.id AND tr.horizon_days=? AND tr.status IN ('complete','unavailable')
		)`)
	}
	args := make([]interface{}, 0, len(normalized)+1)
	args = append(args, HistoricalBacktestTaskID)
	for _, horizon := range normalized {
		args = append(args, horizon)
	}
	query := `SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
		signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
		detail_json,created_at
		FROM selection_results AS sr
		WHERE sr.task_id<>? AND (` + strings.Join(structuredChecks, " OR ") + `)
		ORDER BY created_at ASC,id ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]SelectionResult, 0)
	for rows.Next() {
		var item SelectionResult
		if err := scanSelectionResult(&item, rows); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	queryErr := rows.Err()
	_ = rows.Close()
	if err := s.hydrateSelectionResultsTracking(list); err != nil {
		return nil, err
	}
	return list, queryErr
}

// UpdateSelectionTracking stores the deterministic forward-performance snapshot
// separately from the original formula result, allowing it to be refreshed as
// more trading days become available.
func (s *AppStore) UpdateSelectionTracking(id string, tracking SelectionTracking) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("选股结果ID不能为空")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	item, err := scanSelectionResultRow(tx.QueryRow(`SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
		signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
		detail_json,created_at FROM selection_results WHERE id=?`, id))
	if err != nil {
		return err
	}
	if err := syncSelectionResultTrackingTx(tx, item, tracking); err != nil {
		return err
	}
	return tx.Commit()
}

type selectionResultScanner interface {
	Scan(dest ...interface{}) error
}

func scanSelectionResultRow(row selectionResultScanner) (SelectionResult, error) {
	var item SelectionResult
	err := scanSelectionResult(&item, row)
	return item, err
}

func scanSelectionResult(item *SelectionResult, row selectionResultScanner) error {
	if err := row.Scan(&item.ID, &item.RunID, &item.TaskID, &item.TaskName, &item.FormulaID, &item.FormulaName, &item.Symbol, &item.Latest,
		&item.SignalAt, &item.SignalPrice, &item.QuoteAt, &item.QuoteSource, &item.PriceType, &item.QuoteAgeMS, &item.SignalScore,
		&item.StrategyVersion, &item.MarketContextJSON, &item.SnapshotStatus, &item.DetailJSON, &item.CreatedAt); err != nil {
		return err
	}
	normalizeSelectionSnapshot(item)
	return nil
}

func (s *AppStore) hydrateSelectionResultsTracking(items []SelectionResult) error {
	if len(items) == 0 {
		return nil
	}
	indexByID := make(map[string]int, len(items))
	for index := range items {
		indexByID[items[index].ID] = index
	}
	const batchSize = 400
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		placeholders := make([]string, end-start)
		args := make([]interface{}, 0, end-start)
		for index := start; index < end; index++ {
			placeholders[index-start] = "?"
			args = append(args, items[index].ID)
		}
		rows, err := s.db.Query(`SELECT selection_result_id,signal_date,base_date,base_price,base_price_from,
			horizon_days,status,as_of_date,final_date,final_open,final_close,final_high,final_low,average_close,average_close_return,open_return,close_return,high_return,low_return,
			mfe,mae,max_path_drawdown,benchmark_close_return,excess_close_return,gross_return,net_return,total_cost,first_event,calculation_version,data_quality,
			max_gain,max_gain_date,max_gain_price,days_to_max_gain,max_drawdown,max_drawdown_date,max_drawdown_price,
			days_to_max_drawdown,target_return,target_hit,target_hit_date,days_to_target_hit,drawdown_limit,
			stop_loss_hit,stop_loss_hit_date,days_to_stop_loss,success,reason,updated_at
			FROM selection_result_tracking
			WHERE selection_result_id IN (`+strings.Join(placeholders, ",")+`)
			ORDER BY selection_result_id,horizon_days`, args...)
		if err != nil {
			return err
		}
		trackingByID := map[string]*SelectionTracking{}
		for rows.Next() {
			var selectionResultID string
			var record SelectionResultTracking
			var targetHit, stopLossHit, success int
			if err := rows.Scan(&selectionResultID, &record.SignalDate, &record.BaseDate, &record.BasePrice, &record.BasePriceFrom,
				&record.HorizonDays, &record.Status, &record.AsOfDate, &record.FinalDate, &record.FinalOpen, &record.FinalClose, &record.FinalHigh, &record.FinalLow, &record.AverageClose, &record.AverageCloseReturn,
				&record.OpenReturn, &record.CloseReturn, &record.HighReturn, &record.LowReturn,
				&record.MFE, &record.MAE, &record.MaxPathDrawdown, &record.BenchmarkCloseReturn, &record.ExcessCloseReturn, &record.GrossReturn, &record.NetReturn, &record.TotalCost, &record.FirstEvent, &record.CalculationVersion, &record.DataQuality,
				&record.MaxGain, &record.MaxGainDate, &record.MaxGainPrice, &record.DaysToMaxGain,
				&record.MaxDrawdown, &record.MaxDrawdownDate, &record.MaxDrawdownPrice, &record.DaysToMaxDrawdown,
				&record.TargetReturn, &targetHit, &record.TargetHitDate, &record.DaysToTargetHit, &record.DrawdownLimit,
				&stopLossHit, &record.StopLossHitDate, &record.DaysToStopLoss, &success, &record.Reason, &record.UpdatedAt); err != nil {
				_ = rows.Close()
				return err
			}
			tracking := trackingByID[selectionResultID]
			if tracking == nil {
				tracking = &SelectionTracking{
					Version: "2", SignalDate: record.SignalDate, BaseDate: record.BaseDate,
					BasePrice: record.BasePrice, BasePriceFrom: record.BasePriceFrom,
					TargetReturn: record.TargetReturn, DrawdownLimit: record.DrawdownLimit,
					UpdatedAt: record.UpdatedAt, Horizons: map[string]SelectionHorizon{},
				}
				trackingByID[selectionResultID] = tracking
			}
			mfe, mae := record.MFE, record.MAE
			if mfe == 0 && record.MaxGain != 0 {
				mfe = record.MaxGain
			}
			if mae == 0 && record.MaxDrawdown != 0 {
				mae = record.MaxDrawdown
			}
			tracking.Horizons[horizonKey(record.HorizonDays)] = SelectionHorizon{
				HorizonDays: record.HorizonDays, Status: record.Status, AsOfDate: record.AsOfDate,
				FinalDate: record.FinalDate, FinalOpen: record.FinalOpen, FinalClose: record.FinalClose, FinalHigh: record.FinalHigh, FinalLow: record.FinalLow, AverageClose: record.AverageClose, AverageCloseReturn: record.AverageCloseReturn,
				OpenReturn: record.OpenReturn, CloseReturn: record.CloseReturn, HighReturn: record.HighReturn, LowReturn: record.LowReturn,
				MFE: mfe, MAE: mae, MaxPathDrawdown: record.MaxPathDrawdown, BenchmarkCloseReturn: record.BenchmarkCloseReturn, ExcessCloseReturn: record.ExcessCloseReturn,
				GrossReturn: record.GrossReturn, NetReturn: record.NetReturn, TotalCost: record.TotalCost, FirstEvent: record.FirstEvent, CalculationVersion: record.CalculationVersion, DataQuality: record.DataQuality,
				MaxGain: record.MaxGain, MaxGainDate: record.MaxGainDate, MaxGainPrice: record.MaxGainPrice,
				DaysToMaxGain: record.DaysToMaxGain, MaxDrawdown: record.MaxDrawdown,
				MaxDrawdownDate: record.MaxDrawdownDate, MaxDrawdownPrice: record.MaxDrawdownPrice,
				DaysToMaxDrawdown: record.DaysToMaxDrawdown, TargetReturn: record.TargetReturn,
				TargetHit: intBool(targetHit), TargetHitDate: record.TargetHitDate, DaysToTargetHit: record.DaysToTargetHit,
				DrawdownLimit: record.DrawdownLimit, StopLossHit: intBool(stopLossHit),
				StopLossHitDate: record.StopLossHitDate, DaysToStopLoss: record.DaysToStopLoss,
				Success: intBool(success), Reason: record.Reason,
			}
		}
		queryErr := rows.Err()
		_ = rows.Close()
		if queryErr != nil {
			return queryErr
		}
		for selectionResultID, tracking := range trackingByID {
			if index, ok := indexByID[selectionResultID]; ok {
				items[index].Tracking = tracking
			}
		}
	}
	return nil
}

func syncSelectionResultTrackingTx(tx *sql.Tx, item SelectionResult, tracking SelectionTracking) error {
	if len(tracking.Horizons) == 0 {
		_, err := tx.Exec(`DELETE FROM selection_result_tracking WHERE selection_result_id=?`, item.ID)
		return err
	}
	seen := map[int]bool{}
	for key, horizon := range tracking.Horizons {
		if horizon.HorizonDays <= 0 {
			horizon.HorizonDays = horizonDaysFromKey(key)
		}
		if horizon.HorizonDays <= 0 {
			continue
		}
		seen[horizon.HorizonDays] = true
		record := selectionResultTrackingFromHorizon(item, tracking, horizon)
		if _, err := tx.Exec(`INSERT INTO selection_result_tracking
			(id,selection_result_id,run_id,task_id,task_name,formula_id,formula_name,symbol,
			 signal_date,base_date,base_price,base_price_from,horizon_days,status,as_of_date,final_date,final_open,final_close,final_high,final_low,average_close,average_close_return,
			 open_return,close_return,high_return,low_return,mfe,mae,max_path_drawdown,benchmark_close_return,excess_close_return,gross_return,net_return,total_cost,first_event,calculation_version,data_quality,
			 max_gain,max_gain_date,max_gain_price,days_to_max_gain,
			 max_drawdown,max_drawdown_date,max_drawdown_price,days_to_max_drawdown,
			 target_return,target_hit,target_hit_date,days_to_target_hit,
			 drawdown_limit,stop_loss_hit,stop_loss_hit_date,days_to_stop_loss,
			 success,reason,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(selection_result_id,horizon_days) DO UPDATE SET
				run_id=excluded.run_id,task_id=excluded.task_id,task_name=excluded.task_name,formula_id=excluded.formula_id,formula_name=excluded.formula_name,symbol=excluded.symbol,
				signal_date=excluded.signal_date,base_date=excluded.base_date,base_price=excluded.base_price,base_price_from=excluded.base_price_from,status=excluded.status,
				as_of_date=excluded.as_of_date,final_date=excluded.final_date,final_open=excluded.final_open,final_close=excluded.final_close,final_high=excluded.final_high,final_low=excluded.final_low,average_close=excluded.average_close,average_close_return=excluded.average_close_return,
				open_return=excluded.open_return,close_return=excluded.close_return,high_return=excluded.high_return,low_return=excluded.low_return,mfe=excluded.mfe,mae=excluded.mae,max_path_drawdown=excluded.max_path_drawdown,benchmark_close_return=excluded.benchmark_close_return,excess_close_return=excluded.excess_close_return,gross_return=excluded.gross_return,net_return=excluded.net_return,total_cost=excluded.total_cost,first_event=excluded.first_event,calculation_version=excluded.calculation_version,data_quality=excluded.data_quality,
				max_gain=excluded.max_gain,max_gain_date=excluded.max_gain_date,max_gain_price=excluded.max_gain_price,days_to_max_gain=excluded.days_to_max_gain,
				max_drawdown=excluded.max_drawdown,max_drawdown_date=excluded.max_drawdown_date,max_drawdown_price=excluded.max_drawdown_price,days_to_max_drawdown=excluded.days_to_max_drawdown,
				target_return=excluded.target_return,target_hit=excluded.target_hit,target_hit_date=excluded.target_hit_date,days_to_target_hit=excluded.days_to_target_hit,
				drawdown_limit=excluded.drawdown_limit,stop_loss_hit=excluded.stop_loss_hit,stop_loss_hit_date=excluded.stop_loss_hit_date,days_to_stop_loss=excluded.days_to_stop_loss,
				success=excluded.success,reason=excluded.reason,updated_at=excluded.updated_at`,
			record.ID, record.SelectionResultID, record.RunID, record.TaskID, record.TaskName, record.FormulaID, record.FormulaName, record.Symbol,
			record.SignalDate, record.BaseDate, record.BasePrice, record.BasePriceFrom, record.HorizonDays, record.Status, record.AsOfDate, record.FinalDate, record.FinalOpen, record.FinalClose, record.FinalHigh, record.FinalLow, record.AverageClose, record.AverageCloseReturn,
			record.OpenReturn, record.CloseReturn, record.HighReturn, record.LowReturn, record.MFE, record.MAE, record.MaxPathDrawdown, record.BenchmarkCloseReturn, record.ExcessCloseReturn, record.GrossReturn, record.NetReturn, record.TotalCost, record.FirstEvent, record.CalculationVersion, record.DataQuality,
			record.MaxGain, record.MaxGainDate, record.MaxGainPrice, record.DaysToMaxGain,
			record.MaxDrawdown, record.MaxDrawdownDate, record.MaxDrawdownPrice, record.DaysToMaxDrawdown,
			record.TargetReturn, boolInt(record.TargetHit), record.TargetHitDate, record.DaysToTargetHit,
			record.DrawdownLimit, boolInt(record.StopLossHit), record.StopLossHitDate, record.DaysToStopLoss,
			boolInt(record.Success), record.Reason, record.CreatedAt, record.UpdatedAt); err != nil {
			return err
		}
	}
	args := []interface{}{item.ID}
	placeholders := make([]string, 0, len(seen))
	for horizon := range seen {
		placeholders = append(placeholders, "?")
		args = append(args, horizon)
	}
	if len(placeholders) > 0 {
		_, err := tx.Exec(`DELETE FROM selection_result_tracking WHERE selection_result_id=? AND horizon_days NOT IN (`+strings.Join(placeholders, ",")+`)`, args...)
		return err
	}
	return nil
}

func horizonDaysFromKey(key string) int {
	value := strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimPrefix(value, "d")
	days, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return days
}

func selectionResultTrackingFromHorizon(item SelectionResult, tracking SelectionTracking, horizon SelectionHorizon) SelectionResultTracking {
	updatedAt := tracking.UpdatedAt
	if strings.TrimSpace(updatedAt) == "" {
		updatedAt = NowText()
	}
	createdAt := item.CreatedAt
	if strings.TrimSpace(createdAt) == "" {
		createdAt = updatedAt
	}
	return SelectionResultTracking{
		ID:                   fmt.Sprintf("%s:d%d", item.ID, horizon.HorizonDays),
		SelectionResultID:    item.ID,
		RunID:                item.RunID,
		TaskID:               item.TaskID,
		TaskName:             item.TaskName,
		FormulaID:            item.FormulaID,
		FormulaName:          item.FormulaName,
		Symbol:               strings.ToUpper(strings.TrimSpace(item.Symbol)),
		SignalDate:           tracking.SignalDate,
		BaseDate:             tracking.BaseDate,
		BasePrice:            tracking.BasePrice,
		BasePriceFrom:        tracking.BasePriceFrom,
		HorizonDays:          horizon.HorizonDays,
		Status:               horizon.Status,
		AsOfDate:             horizon.AsOfDate,
		FinalDate:            horizon.FinalDate,
		FinalOpen:            horizon.FinalOpen,
		FinalClose:           horizon.FinalClose,
		FinalHigh:            horizon.FinalHigh,
		FinalLow:             horizon.FinalLow,
		AverageClose:         horizon.AverageClose,
		AverageCloseReturn:   horizon.AverageCloseReturn,
		OpenReturn:           horizon.OpenReturn,
		CloseReturn:          horizon.CloseReturn,
		HighReturn:           horizon.HighReturn,
		LowReturn:            horizon.LowReturn,
		MFE:                  horizon.MFE,
		MAE:                  horizon.MAE,
		MaxPathDrawdown:      horizon.MaxPathDrawdown,
		BenchmarkCloseReturn: horizon.BenchmarkCloseReturn,
		ExcessCloseReturn:    horizon.ExcessCloseReturn,
		GrossReturn:          horizon.GrossReturn,
		NetReturn:            horizon.NetReturn,
		TotalCost:            horizon.TotalCost,
		FirstEvent:           horizon.FirstEvent,
		CalculationVersion:   horizon.CalculationVersion,
		DataQuality:          horizon.DataQuality,
		MaxGain:              horizon.MaxGain,
		MaxGainDate:          horizon.MaxGainDate,
		MaxGainPrice:         horizon.MaxGainPrice,
		DaysToMaxGain:        horizon.DaysToMaxGain,
		MaxDrawdown:          horizon.MaxDrawdown,
		MaxDrawdownDate:      horizon.MaxDrawdownDate,
		MaxDrawdownPrice:     horizon.MaxDrawdownPrice,
		DaysToMaxDrawdown:    horizon.DaysToMaxDrawdown,
		TargetReturn:         horizon.TargetReturn,
		TargetHit:            horizon.TargetHit,
		TargetHitDate:        horizon.TargetHitDate,
		DaysToTargetHit:      horizon.DaysToTargetHit,
		DrawdownLimit:        horizon.DrawdownLimit,
		StopLossHit:          horizon.StopLossHit,
		StopLossHitDate:      horizon.StopLossHitDate,
		DaysToStopLoss:       horizon.DaysToStopLoss,
		Success:              horizon.Success,
		Reason:               horizon.Reason,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
}

func (s *AppStore) migrateLegacySelectionTracking() error {
	hasLegacyColumn, err := s.tableHasColumn("selection_results", "tracking_json")
	if err != nil {
		return err
	}
	if !hasLegacyColumn {
		return nil
	}
	if err := s.backfillLegacySelectionResultTracking(); err != nil {
		return err
	}
	return s.dropSelectionResultsTrackingJSONColumn()
}

func (s *AppStore) backfillLegacySelectionResultTracking() error {
	rows, err := s.db.Query(`SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,detail_json,tracking_json,created_at
		FROM selection_results
		WHERE TRIM(COALESCE(tracking_json,''))<>'' AND TRIM(COALESCE(tracking_json,''))<>'{}' AND json_valid(tracking_json)=1
			AND NOT EXISTS (
				SELECT 1 FROM selection_result_tracking AS tr WHERE tr.selection_result_id=selection_results.id
			)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type legacySelectionResult struct {
		item         SelectionResult
		trackingJSON string
	}
	items := make([]legacySelectionResult, 0)
	for rows.Next() {
		var item SelectionResult
		var trackingJSON string
		if err := rows.Scan(&item.ID, &item.RunID, &item.TaskID, &item.TaskName, &item.FormulaID, &item.FormulaName, &item.Symbol, &item.Latest, &item.DetailJSON, &trackingJSON, &item.CreatedAt); err != nil {
			return err
		}
		items = append(items, legacySelectionResult{item: item, trackingJSON: trackingJSON})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		var tracking SelectionTracking
		if err := json.Unmarshal([]byte(item.trackingJSON), &tracking); err != nil {
			return err
		}
		if err := syncSelectionResultTrackingTx(tx, item.item, tracking); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *AppStore) dropSelectionResultsTrackingJSONColumn() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE selection_results_new (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_name TEXT NOT NULL,
			formula_id TEXT NOT NULL,
			formula_name TEXT NOT NULL,
			symbol TEXT NOT NULL,
			latest REAL NOT NULL DEFAULT 0,
			signal_at TEXT NOT NULL DEFAULT '',
			signal_price REAL NOT NULL DEFAULT 0,
			quote_at TEXT NOT NULL DEFAULT '',
			quote_source TEXT NOT NULL DEFAULT '',
			price_type TEXT NOT NULL DEFAULT '',
			quote_age_ms INTEGER NOT NULL DEFAULT 0,
			signal_score REAL NOT NULL DEFAULT 0,
			strategy_version TEXT NOT NULL DEFAULT '',
			market_context_json TEXT NOT NULL DEFAULT '{}',
			snapshot_status TEXT NOT NULL DEFAULT 'missing',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO selection_results_new (id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
			signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
			detail_json,created_at)
			SELECT id,run_id,task_id,task_name,formula_id,formula_name,symbol,latest,
			signal_at,signal_price,quote_at,quote_source,price_type,quote_age_ms,signal_score,strategy_version,market_context_json,snapshot_status,
			detail_json,created_at FROM selection_results`,
		`DROP TABLE selection_results`,
		`ALTER TABLE selection_results_new RENAME TO selection_results`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_created_at ON selection_results(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_run_created_at ON selection_results(run_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_task_created_at ON selection_results(task_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_formula_created_at ON selection_results(formula_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_symbol_created_at ON selection_results(symbol, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_symbol ON selection_results(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_formula ON selection_results(formula_id)`,
		`CREATE INDEX IF NOT EXISTS idx_selection_results_signal_at ON selection_results(signal_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_selection_results_business_key ON selection_results(run_id, formula_id, symbol, signal_at)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func normalizeWebhook(h Webhook) Webhook {
	h.Name = strings.TrimSpace(h.Name)
	h.URL = strings.TrimSpace(h.URL)
	h.Method = strings.ToUpper(strings.TrimSpace(h.Method))
	if h.Method == "" {
		h.Method = "POST"
	}
	if strings.TrimSpace(h.HeadersJSON) == "" {
		h.HeadersJSON = "{}"
	}
	if strings.TrimSpace(h.Events) == "" {
		h.Events = `["automation.failed","automation.finished","stock_selection.finished","strategy_selection.finished","macro_event.alert_due","macro_event.window_started","t-signal.triggered","t-signal.notice","t-signal.acked"]`
	}
	return h
}

func (s *AppStore) ListWebhooks() ([]Webhook, error) {
	rows, err := s.db.Query(`SELECT id,name,url,method,headers_json,events,enabled,created_at,updated_at FROM webhooks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Webhook
	for rows.Next() {
		var h Webhook
		var enabled int
		if err := rows.Scan(&h.ID, &h.Name, &h.URL, &h.Method, &h.HeadersJSON, &h.Events, &enabled, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		h.Enabled = intBool(enabled)
		list = append(list, h)
	}
	if list == nil {
		list = []Webhook{}
	}
	return list, rows.Err()
}

func (s *AppStore) ListEnabledWebhooks() ([]Webhook, error) {
	items, err := s.ListWebhooks()
	if err != nil {
		return nil, err
	}
	enabled := make([]Webhook, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	return enabled, nil
}

func (s *AppStore) GetWebhook(id string) (Webhook, error) {
	var h Webhook
	var enabled int
	err := s.db.QueryRow(`SELECT id,name,url,method,headers_json,events,enabled,created_at,updated_at FROM webhooks WHERE id=?`, id).
		Scan(&h.ID, &h.Name, &h.URL, &h.Method, &h.HeadersJSON, &h.Events, &enabled, &h.CreatedAt, &h.UpdatedAt)
	h.Enabled = intBool(enabled)
	return h, err
}

func (s *AppStore) UpsertWebhook(h Webhook) (Webhook, error) {
	h = normalizeWebhook(h)
	if h.Name == "" || h.URL == "" {
		return h, errors.New("Webhook名称和URL不能为空")
	}
	if !json.Valid([]byte(h.HeadersJSON)) {
		return h, errors.New("headers_json不是有效JSON")
	}
	if !json.Valid([]byte(h.Events)) {
		return h, errors.New("events不是有效JSON数组")
	}

	now := NowText()
	if h.ID == "" {
		h.ID = uuid.NewString()
		h.CreatedAt = now
	} else if h.CreatedAt == "" {
		old, err := s.GetWebhook(h.ID)
		if err == nil {
			h.CreatedAt = old.CreatedAt
		} else {
			h.CreatedAt = now
		}
	}
	h.UpdatedAt = now

	_, err := s.db.Exec(`INSERT INTO webhooks
		(id,name,url,method,headers_json,events,enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,url=excluded.url,method=excluded.method,headers_json=excluded.headers_json,
			events=excluded.events,enabled=excluded.enabled,updated_at=excluded.updated_at`,
		h.ID, h.Name, h.URL, h.Method, h.HeadersJSON, h.Events, boolInt(h.Enabled), h.CreatedAt, h.UpdatedAt)
	return h, err
}

func (s *AppStore) DeleteWebhook(id string) error {
	_, err := s.db.Exec(`DELETE FROM webhooks WHERE id=?`, id)
	return err
}

func (s *AppStore) ResolveWebhooks(idsJSON string) ([]Webhook, error) {
	var ids []string
	if strings.TrimSpace(idsJSON) == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(idsJSON), &ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	hooks := make([]Webhook, 0, len(ids))
	for _, id := range ids {
		h, err := s.GetWebhook(id)
		if err != nil {
			continue
		}
		if h.Enabled {
			hooks = append(hooks, h)
		}
	}
	return hooks, nil
}

func MustJSON(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(raw)
}
