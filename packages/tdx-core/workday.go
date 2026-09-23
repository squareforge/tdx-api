package tdx

import (
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
	"xorm.io/xorm"
)

const (
	WorkdayMarketCN_A       = "CN_A"
	WorkdaySourcePMC        = "pandas_market_calendars"
	WorkdaySourceTDX        = "tdx_sh000001"
	WorkdayConfidencePre    = "predicted"
	WorkdayConfidenceOK     = "confirmed"
	workdayDefaultOpenTime  = "09:30"
	workdayDefaultBreakFrom = "11:30"
	workdayDefaultBreakTo   = "13:00"
	workdayDefaultCloseTime = "15:00"
)

type (
	WorkdayOption   func(w *Workday)
	DialWorkdayFunc func(c *Client) (*Workday, error)
)

func WithWorkdaySpec(spec string) WorkdayOption {
	return func(w *Workday) {
		w.spec = spec
	}
}

func WithWorkdayRetry(retry int) WorkdayOption {
	return func(w *Workday) {
		w.retry = retry
	}
}

func WithWorkdayDB(db *xorms.Engine) WorkdayOption {
	return func(w *Workday) {
		w.db = db
	}
}

func WithWorkdayDialDB(dial DialDBFunc) WorkdayOption {
	return func(w *Workday) {
		w.dialDB = dial
	}
}

func WithWorkdayClient(c *Client) WorkdayOption {
	return func(w *Workday) {
		w.c = c
	}
}

func WithWorkdayDialClient(dial DialClientFunc) WorkdayOption {
	return func(w *Workday) {
		w.dialClient = dial
	}
}

func WithWorkdayOption(op ...WorkdayOption) WorkdayOption {
	return func(w *Workday) {
		for _, v := range op {
			v(w)
		}
	}
}

func NewWorkdayMysql(dsn string, op ...WorkdayOption) (*Workday, error) {
	return NewWorkday(
		WithWorkdayDialDB(func() (*xorms.Engine, error) { return xorms.NewMysql(dsn) }),
		WithWorkdayOption(op...),
	)
}

func NewWorkdaySqlite(op ...WorkdayOption) (*Workday, error) {
	return NewWorkday(op...)
}

func NewWorkday(op ...WorkdayOption) (*Workday, error) {

	w := &Workday{
		spec:       DefaultWorkdaySpec,
		retry:      DefaultRetry,
		dialDB:     nil,
		dialClient: nil,

		c:     nil,
		db:    nil,
		cache: map[string]bool{},
	}

	for _, v := range op {
		v(w)
	}

	var err error
	if w.db == nil {
		if w.dialDB == nil {
			w.dialDB = func() (*xorms.Engine, error) {
				return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "workday.db"))
			}
		}
		w.db, err = w.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if err := w.ensureSchema(); err != nil {
		return nil, err
	}

	if w.c == nil {
		if w.dialClient == nil {
			w.dialClient = func() (*Client, error) { return DialDefault() }
		}
		w.c, err = w.dialClient()
		if err != nil {
			return nil, err
		}
	}

	//设置定时器,每天早上9点更新数据,8点多获取不到今天的数据
	err = NewTimer(w.spec, w.retry, w)

	return w, err
}

type Workday struct {
	spec       string
	retry      int
	dialDB     DialDBFunc
	dialClient DialClientFunc

	c  *Client
	db *xorms.Engine

	cacheMu sync.RWMutex
	cache   map[string]bool
}

// Update 更新
func (this *Workday) Update() error {
	if err := this.ensureSchema(); err != nil {
		return err
	}

	if this.c == nil {
		return errors.New("client is nil")
	}

	now := time.Now()
	confirmEnd := IntegerDay(now).AddDate(0, 0, -1)
	if now.Hour() >= 16 {
		confirmEnd = IntegerDay(now)
	}
	confirmedThrough, err := this.confirmedThrough()
	if err != nil {
		return err
	}
	if confirmedThrough != "" && confirmedThrough >= confirmEnd.Format("20060102") {
		return this.loadCache()
	}

	resp, err := this.c.GetIndexDayAll("sh000001")
	if err != nil {
		logs.Err(err)
		return err
	}
	workdays := map[string]bool{}
	start := ""
	end := ""
	for _, v := range resp.List {
		date := v.Time.Format("20060102")
		workdays[date] = true
		if start == "" || date < start {
			start = date
		}
		if end == "" || date > end {
			end = date
		}
	}
	if end > confirmEnd.Format("20060102") {
		end = confirmEnd.Format("20060102")
	}
	if start != "" && end != "" {
		if err := this.upsertConfirmedRange(start, end, workdays); err != nil {
			return err
		}
	}
	return this.loadCache()
}

func (this *Workday) ensureSchema() error {
	if _, err := this.db.Exec(`CREATE TABLE IF NOT EXISTS workday (
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
		exists, err := this.columnExists(name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := this.db.Exec(fmt.Sprintf("ALTER TABLE workday ADD COLUMN %s %s", name, definition)); err != nil {
				return err
			}
		}
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := this.db.Exec(`UPDATE workday SET
		market = CASE WHEN market IS NULL OR market = '' THEN ? ELSE market END,
		is_trade_day = CASE WHEN is_trade_day IS NULL OR is_trade_day = 0 THEN 1 ELSE is_trade_day END,
		source = CASE WHEN source IS NULL OR source = '' THEN ? ELSE source END,
		confidence = CASE WHEN confidence IS NULL OR confidence = '' THEN ? ELSE confidence END,
		open_time = CASE WHEN open_time IS NULL OR open_time = '' THEN ? ELSE open_time END,
		break_start = CASE WHEN break_start IS NULL OR break_start = '' THEN ? ELSE break_start END,
		break_end = CASE WHEN break_end IS NULL OR break_end = '' THEN ? ELSE break_end END,
		close_time = CASE WHEN close_time IS NULL OR close_time = '' THEN ? ELSE close_time END,
		confirmed_at = CASE WHEN confirmed_at IS NULL OR confirmed_at = '' THEN ? ELSE confirmed_at END,
		updated_at = CASE WHEN updated_at IS NULL OR updated_at = '' THEN ? ELSE updated_at END,
		remark = CASE WHEN remark IS NULL OR remark = '' THEN 'legacy workday seed' ELSE remark END
		WHERE Date IS NOT NULL AND Date != ''`,
		WorkdayMarketCN_A, WorkdaySourceTDX, WorkdayConfidenceOK,
		workdayDefaultOpenTime, workdayDefaultBreakFrom, workdayDefaultBreakTo, workdayDefaultCloseTime,
		now, now); err != nil {
		return err
	}
	_, _ = this.db.Exec(`CREATE INDEX IF NOT EXISTS idx_workday_market_date ON workday (market, Date)`)
	_, _ = this.db.Exec(`CREATE INDEX IF NOT EXISTS idx_workday_trade_day ON workday (market, is_trade_day, Date)`)
	return this.migrateTradeCalendarRows()
}

func (this *Workday) columnExists(name string) (bool, error) {
	rows, err := this.db.Query(`PRAGMA table_info(workday)`)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if strings.EqualFold(string(row["name"]), name) {
			return true, nil
		}
	}
	return false, nil
}

func (this *Workday) migrateTradeCalendarRows() error {
	items := []WorkdayModel(nil)
	if err := this.db.SQL(`SELECT date AS Date, market, is_trade_day, source, confidence, open_time, break_start, break_end, close_time, confirmed_at, updated_at, remark FROM trade_calendar ORDER BY date`).Find(&items); err != nil {
		return nil
	}
	for _, item := range items {
		if item.Market == "" {
			item.Market = WorkdayMarketCN_A
		}
		item.Unix = workdayUnix(item.Date)
		if err := this.upsertModel(item, item.Confidence != WorkdayConfidenceOK); err != nil {
			return err
		}
	}
	return nil
}

func (this *Workday) confirmedThrough() (string, error) {
	var rows []WorkdayModel
	if err := this.db.Where("market=? AND confidence=?", WorkdayMarketCN_A, WorkdayConfidenceOK).Desc("Date").Limit(1).Find(&rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].Date, nil
}

func (this *Workday) upsertConfirmedRange(start, end string, workdays map[string]bool) error {
	startTime, err := time.ParseInLocation("20060102", start, time.Local)
	if err != nil {
		return err
	}
	endTime, err := time.ParseInLocation("20060102", end, time.Local)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	return this.db.SessionFunc(func(session *xorm.Session) error {
		for cursor := startTime; !cursor.After(endTime); cursor = cursor.AddDate(0, 0, 1) {
			date := cursor.Format("20060102")
			item := WorkdayModel{
				Unix:        cursor.Add(15 * time.Hour).Unix(),
				Date:        date,
				Market:      WorkdayMarketCN_A,
				IsTradeDay:  0,
				Source:      WorkdaySourceTDX,
				Confidence:  WorkdayConfidenceOK,
				ConfirmedAt: now,
				UpdatedAt:   now,
			}
			if workdays[date] {
				item.IsTradeDay = 1
				item.OpenTime = workdayDefaultOpenTime
				item.BreakStart = workdayDefaultBreakFrom
				item.BreakEnd = workdayDefaultBreakTo
				item.CloseTime = workdayDefaultCloseTime
			}
			if err := upsertWorkdayModel(session, item, false); err != nil {
				return err
			}
		}
		return nil
	})
}

func (this *Workday) upsertModel(item WorkdayModel, keepConfirmed bool) error {
	return this.db.SessionFunc(func(session *xorm.Session) error {
		return upsertWorkdayModel(session, item, keepConfirmed)
	})
}

func upsertWorkdayModel(session *xorm.Session, item WorkdayModel, keepConfirmed bool) error {
	existing := new(WorkdayModel)
	has, err := session.Where("Date=? AND market=?", item.Date, item.Market).Get(existing)
	if err != nil {
		return err
	}
	if has {
		if keepConfirmed && existing.Confidence == WorkdayConfidenceOK {
			return nil
		}
		item.ID = existing.ID
		_, err = session.ID(existing.ID).AllCols().Update(&item)
		return err
	}
	_, err = session.Insert(&item)
	return err
}

func (this *Workday) loadCache() error {
	rows := []WorkdayModel(nil)
	if err := this.db.Where("market=? AND is_trade_day=1", WorkdayMarketCN_A).Find(&rows); err != nil {
		return err
	}
	cache := make(map[string]bool, len(rows))
	for _, v := range rows {
		cache[v.Date] = true
	}
	this.cacheMu.Lock()
	this.cache = cache
	this.cacheMu.Unlock()
	return nil
}

// Is 是否是工作日
func (this *Workday) Is(t time.Time) bool {
	this.cacheMu.RLock()
	ok := this.cache[t.Format("20060102")]
	this.cacheMu.RUnlock()
	return ok
}

// TodayIs 今天是否是工作日
func (this *Workday) TodayIs() bool {
	return this.Is(time.Now())
}

// RangeYear 遍历一年的所有工作日
func (this *Workday) RangeYear(year int, f func(t time.Time) bool) {
	this.Range(
		time.Date(year, 1, 1, 0, 0, 0, 0, time.Local),
		time.Date(year, 12, 31, 0, 0, 0, 1, time.Local),
		f,
	)
}

// Range 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (this *Workday) Range(start, end time.Time, f func(t time.Time) bool) {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	for ; start.Before(end); start = start.Add(time.Hour * 24) {
		if this.Is(start) {
			if !f(start) {
				return
			}
		}
	}
}

func (this *Workday) IterYear(year int, desc ...bool) iter.Seq[time.Time] {
	return this.Iter(
		time.Date(year, 1, 1, 0, 0, 0, 0, time.Local),
		time.Date(year, 12, 31, 0, 0, 0, 1, time.Local),
		desc...,
	)
}

// Iter 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (this *Workday) Iter(start, end time.Time, desc ...bool) iter.Seq[time.Time] {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	if len(desc) > 0 && desc[0] {
		//倒序遍历
		return func(yield func(time.Time) bool) {
			for ; end.After(start); end = end.Add(-time.Hour * 24) {
				if this.Is(end) {
					if !yield(end) {
						return
					}
				}
			}
		}
	}
	//正序遍历
	return func(yield func(time.Time) bool) {
		for ; start.Before(end); start = start.Add(time.Hour * 24) {
			if this.Is(start) {
				if !yield(start) {
					return
				}
			}
		}
	}
}

// WorkdayModel 工作日
type WorkdayModel struct {
	ID          int64  `json:"id" xorm:"'ID' pk autoincr"`         //主键
	Unix        int64  `json:"unix" xorm:"'Unix'"`                 //时间戳
	Date        string `json:"date" xorm:"'Date'"`                 //日期
	Market      string `json:"market" xorm:"'market'"`             //市场
	IsTradeDay  int    `json:"is_trade_day" xorm:"'is_trade_day'"` //是否交易日
	Source      string `json:"source" xorm:"'source'"`             //来源
	Confidence  string `json:"confidence" xorm:"'confidence'"`     //可信度
	OpenTime    string `json:"open_time" xorm:"'open_time'"`       //开盘时间
	BreakStart  string `json:"break_start" xorm:"'break_start'"`   //午休开始
	BreakEnd    string `json:"break_end" xorm:"'break_end'"`       //午休结束
	CloseTime   string `json:"close_time" xorm:"'close_time'"`     //收盘时间
	ConfirmedAt string `json:"confirmed_at" xorm:"'confirmed_at'"` //确认时间
	UpdatedAt   string `json:"updated_at" xorm:"'updated_at'"`     //更新时间
	Remark      string `json:"remark" xorm:"'remark'"`             //备注
}

func (this *WorkdayModel) TableName() string {
	return "workday"
}

func IntegerDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func workdayUnix(date string) int64 {
	t, err := time.ParseInLocation("20060102", date, time.Local)
	if err != nil {
		return 0
	}
	return t.Add(15 * time.Hour).Unix()
}
