package workbench

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// MacroEvent is a scheduled macro-economic event used by the warning center.
// Times are stored in RFC3339 so the UI can render the user's local timezone.
type MacroEvent struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Country       string `json:"country"`
	Impact        string `json:"impact"`
	StartsAt      string `json:"starts_at"`
	ScheduledAt   string `json:"scheduled_at,omitempty"`
	ReleasedAt    string `json:"released_at,omitempty"`
	PreviousValue string `json:"previous_value,omitempty"`
	ForecastValue string `json:"forecast_value,omitempty"`
	ActualValue   string `json:"actual_value,omitempty"`
	Revision      string `json:"revision,omitempty"`
	AShareDate    string `json:"a_share_date"`
	Source        string `json:"source"`
	SourceURL     string `json:"source_url"`
	SourceKind    string `json:"source_kind"`
	ExternalID    string `json:"external_id,omitempty"`
	LastSeenAt    string `json:"last_seen_at,omitempty"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	Acknowledged  bool   `json:"acknowledged"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// MacroEventSyncState records the last attempt for an official calendar provider.
// A failed provider must not erase the last known local schedule.
type MacroEventSyncState struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Status        string `json:"status"`
	LastAttemptAt string `json:"last_attempt_at"`
	LastSuccessAt string `json:"last_success_at"`
	EventCount    int    `json:"event_count"`
	Message       string `json:"message"`
	UpdatedAt     string `json:"updated_at"`
}

type MacroAlertSettings struct {
	ID                  string `json:"id"`
	Enabled             bool   `json:"enabled"`
	LeadMinutes         int    `json:"lead_minutes"`
	WindowBeforeMinutes int    `json:"window_before_minutes"`
	WindowAfterMinutes  int    `json:"window_after_minutes"`
	CriticalOnly        bool   `json:"critical_only"`
	NotifyWebhooks      bool   `json:"notify_webhooks"`
	WebhookIDs          string `json:"webhook_ids"`
	UpdatedAt           string `json:"updated_at"`
}

// These signatures identify reference rows created by older versions. They
// are used only for migration; they are never inserted as calendar events.
var legacyMacroEventSignatures = []struct {
	id, code, name, category, impact, startsAt, aShareDate, source, sourceURL, description string
}{
	{"macro-20260904-nfp", "NFP", "非农就业报告", "employment", "high", "2026-09-04T20:30:00+08:00", "2026-09-07", "BLS", "https://www.bls.gov/schedule/news_release/empsit.htm", "观察就业新增、失业率与平均时薪，判断美国经济冷热和通胀压力。"},
	{"macro-20260910-cpi", "CPI", "美国 CPI 通胀", "inflation", "high", "2026-09-10T20:30:00+08:00", "2026-09-11", "BLS", "https://www.bls.gov/schedule/news_release/cpi.htm", "核心通胀是利率预期和风险资产定价的重要输入。"},
	{"macro-20260916-fomc", "FOMC", "美联储议息会议", "central_bank", "critical", "2026-09-18T02:00:00+08:00", "2026-09-18", "Federal Reserve", "https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm", "关注利率决议、点阵图和主席发布会，事件窗口可能扩大跨市场波动。"},
	{"macro-20260930-pce", "PCE", "美国 PCE 物价指数", "inflation", "high", "2026-09-30T20:30:00+08:00", "2026-10-01", "BEA", "https://www.bea.gov/news/schedule", "美联储偏好的通胀指标，结合核心 PCE 观察降息空间。"},
	{"macro-20261002-nfp", "NFP", "非农就业报告", "employment", "high", "2026-10-02T20:30:00+08:00", "2026-10-05", "BLS", "https://www.bls.gov/schedule/news_release/empsit.htm", "观察就业新增、失业率与平均时薪，判断美国经济冷热和通胀压力。"},
	{"macro-20261013-cpi", "CPI", "美国 CPI 通胀", "inflation", "high", "2026-10-13T20:30:00+08:00", "2026-10-14", "BLS", "https://www.bls.gov/schedule/news_release/cpi.htm", "核心通胀是利率预期和风险资产定价的重要输入。"},
	{"macro-20261028-fomc", "FOMC", "美联储议息会议", "central_bank", "critical", "2026-10-30T02:00:00+08:00", "2026-10-30", "Federal Reserve", "https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm", "关注利率决议、点阵图和主席发布会，事件窗口可能扩大跨市场波动。"},
	{"macro-20261030-pce", "PCE", "美国 PCE 物价指数", "inflation", "high", "2026-10-30T20:30:00+08:00", "2026-11-02", "BEA", "https://www.bea.gov/news/schedule", "美联储偏好的通胀指标，结合核心 PCE 观察降息空间。"},
}

func normalizeMacroEvent(event MacroEvent) (MacroEvent, error) {
	event.Code = strings.ToUpper(strings.TrimSpace(event.Code))
	event.Name = strings.TrimSpace(event.Name)
	event.Category = strings.TrimSpace(event.Category)
	event.Country = strings.ToUpper(strings.TrimSpace(event.Country))
	event.Impact = strings.ToLower(strings.TrimSpace(event.Impact))
	event.StartsAt = strings.TrimSpace(event.StartsAt)
	event.ScheduledAt = strings.TrimSpace(event.ScheduledAt)
	event.ReleasedAt = strings.TrimSpace(event.ReleasedAt)
	event.PreviousValue = strings.TrimSpace(event.PreviousValue)
	event.ForecastValue = strings.TrimSpace(event.ForecastValue)
	event.ActualValue = strings.TrimSpace(event.ActualValue)
	event.Revision = strings.TrimSpace(event.Revision)
	event.AShareDate = strings.TrimSpace(event.AShareDate)
	event.Source = strings.TrimSpace(event.Source)
	event.SourceURL = strings.TrimSpace(event.SourceURL)
	event.SourceKind = strings.ToLower(strings.TrimSpace(event.SourceKind))
	event.ExternalID = strings.TrimSpace(event.ExternalID)
	event.LastSeenAt = strings.TrimSpace(event.LastSeenAt)
	event.Description = strings.TrimSpace(event.Description)
	if event.Code == "" || event.Name == "" || event.StartsAt == "" {
		return event, errors.New("事件代码、名称和发布时间不能为空")
	}
	if _, err := time.Parse(time.RFC3339, event.StartsAt); err != nil {
		return event, errors.New("starts_at必须是RFC3339时间")
	}
	if event.ScheduledAt == "" {
		event.ScheduledAt = event.StartsAt
	}
	for _, value := range []struct {
		name string
		text string
	}{
		{name: "scheduled_at", text: event.ScheduledAt}, {name: "released_at", text: event.ReleasedAt},
	} {
		if value.text != "" {
			if _, err := time.Parse(time.RFC3339, value.text); err != nil {
				return event, errors.New(value.name + "必须是RFC3339时间")
			}
		}
	}
	if event.Impact == "" {
		event.Impact = "medium"
	}
	if event.Country == "" {
		event.Country = "US"
	}
	if event.SourceKind == "" {
		event.SourceKind = "manual"
	}
	switch event.SourceKind {
	case "manual", "official":
	default:
		return event, errors.New("source_kind必须是manual或official")
	}
	if event.Status == "" {
		event.Status = "scheduled"
	}
	switch event.Status {
	case "scheduled", "released", "revised", "cancelled", "stale":
	default:
		event.Status = "scheduled"
	}
	return event, nil
}

func (s *AppStore) ensureMacroEvents() error {
	now := NowText()
	if _, err := s.db.Exec(`UPDATE macro_events SET scheduled_at=starts_at WHERE scheduled_at=''`); err != nil {
		return err
	}
	// Remove only rows that still match the old reference content. This also
	// handles FOMC reference dates corrected by older startup migrations.
	for _, ref := range legacyMacroEventSignatures {
		if _, err := s.db.Exec(`DELETE FROM macro_events WHERE id=? AND source_kind='manual' AND code=? AND name=? AND category=? AND country='US'
			AND impact=? AND starts_at=? AND scheduled_at=starts_at AND a_share_date=? AND source=? AND source_url=?
			AND description=? AND status='scheduled' AND acknowledged=0 AND released_at=''
			AND previous_value='' AND forecast_value='' AND actual_value='' AND revision=''
			AND external_id='' AND last_seen_at=''`,
			ref.id, ref.code, ref.name, ref.category, ref.impact, ref.startsAt, ref.aShareDate, ref.source, ref.sourceURL, ref.description); err != nil {
			return err
		}
	}
	for _, provider := range []string{"BLS", "Federal Reserve", "BEA"} {
		id := "macro-sync-" + strings.ToLower(strings.ReplaceAll(provider, " ", "-"))
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO macro_event_sync_state
			(id,provider,status,last_attempt_at,last_success_at,event_count,message,updated_at)
			VALUES (?,?,?,?,?,?,?,?)`, id, provider, "idle", "", "", 0, "", now); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO macro_alert_settings
		(id,enabled,lead_minutes,window_before_minutes,window_after_minutes,critical_only,notify_webhooks,webhook_ids,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, "default", 1, 240, 240, 120, 0, 0, "[]", now); err != nil {
		return err
	}
	return nil
}

func (s *AppStore) ListMacroEvents(from, to, category, impact string) ([]MacroEvent, error) {
	query := `SELECT id,code,name,category,country,impact,starts_at,scheduled_at,released_at,previous_value,forecast_value,actual_value,revision,a_share_date,source,source_url,source_kind,external_id,last_seen_at,description,status,acknowledged,created_at,updated_at FROM macro_events WHERE 1=1`
	args := []interface{}{}
	if strings.TrimSpace(from) != "" {
		query += " AND starts_at >= ?"
		args = append(args, from)
	}
	if strings.TrimSpace(to) != "" {
		query += " AND starts_at <= ?"
		args = append(args, to)
	}
	if strings.TrimSpace(category) != "" && category != "all" {
		query += " AND category = ?"
		args = append(args, strings.TrimSpace(category))
	}
	if strings.TrimSpace(impact) != "" && impact != "all" {
		query += " AND impact = ?"
		args = append(args, strings.ToLower(strings.TrimSpace(impact)))
	}
	query += " ORDER BY starts_at ASC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MacroEvent{}
	for rows.Next() {
		var event MacroEvent
		var acknowledged int
		if err := rows.Scan(&event.ID, &event.Code, &event.Name, &event.Category, &event.Country, &event.Impact, &event.StartsAt, &event.ScheduledAt, &event.ReleasedAt, &event.PreviousValue, &event.ForecastValue, &event.ActualValue, &event.Revision, &event.AShareDate, &event.Source, &event.SourceURL, &event.SourceKind, &event.ExternalID, &event.LastSeenAt, &event.Description, &event.Status, &acknowledged, &event.CreatedAt, &event.UpdatedAt); err != nil {
			return nil, err
		}
		event.Acknowledged = intBool(acknowledged)
		items = append(items, event)
	}
	return items, rows.Err()
}

func (s *AppStore) GetMacroEvent(id string) (MacroEvent, error) {
	var event MacroEvent
	var acknowledged int
	err := s.db.QueryRow(`SELECT id,code,name,category,country,impact,starts_at,scheduled_at,released_at,previous_value,forecast_value,actual_value,revision,a_share_date,source,source_url,source_kind,external_id,last_seen_at,description,status,acknowledged,created_at,updated_at FROM macro_events WHERE id=?`, id).
		Scan(&event.ID, &event.Code, &event.Name, &event.Category, &event.Country, &event.Impact, &event.StartsAt, &event.ScheduledAt, &event.ReleasedAt, &event.PreviousValue, &event.ForecastValue, &event.ActualValue, &event.Revision, &event.AShareDate, &event.Source, &event.SourceURL, &event.SourceKind, &event.ExternalID, &event.LastSeenAt, &event.Description, &event.Status, &acknowledged, &event.CreatedAt, &event.UpdatedAt)
	event.Acknowledged = intBool(acknowledged)
	return event, err
}

func (s *AppStore) UpsertMacroEvent(event MacroEvent) (MacroEvent, error) {
	var err error
	event, err = normalizeMacroEvent(event)
	if err != nil {
		return event, err
	}
	now := NowText()
	if event.SourceKind == "official" && event.ExternalID != "" {
		var existingID string
		err := s.db.QueryRow(`SELECT id FROM macro_events WHERE source_kind=? AND source=? AND external_id=? ORDER BY updated_at DESC LIMIT 1`, event.SourceKind, event.Source, event.ExternalID).Scan(&existingID)
		if err == nil {
			event.ID = existingID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return event, err
		}
	}
	if event.ID == "" {
		event.ID = "macro-" + strings.ToLower(event.Code) + "-" + strings.ReplaceAll(event.StartsAt, ":", "")
		event.CreatedAt = now
	} else {
		old, getErr := s.GetMacroEvent(event.ID)
		if getErr == nil {
			if event.CreatedAt == "" {
				event.CreatedAt = old.CreatedAt
			}
			if event.SourceKind == "official" && old.SourceKind == "official" {
				if event.ReleasedAt == "" {
					event.ReleasedAt = old.ReleasedAt
				}
				if event.PreviousValue == "" {
					event.PreviousValue = old.PreviousValue
				}
				if event.ForecastValue == "" {
					event.ForecastValue = old.ForecastValue
				}
				if event.ActualValue == "" {
					event.ActualValue = old.ActualValue
				}
				if event.Revision == "" {
					event.Revision = old.Revision
				}
				if event.Status == "scheduled" && old.Status != "scheduled" {
					event.Status = old.Status
				}
			}
			// Older clients do not send source metadata. Preserve it when an
			// existing official event is edited so a normal form save cannot
			// turn it into an unsynchronised manual row.
			if event.SourceKind == "manual" && event.ExternalID == "" && event.LastSeenAt == "" && old.SourceKind == "official" {
				event.SourceKind = old.SourceKind
			}
			if event.ExternalID == "" {
				event.ExternalID = old.ExternalID
			}
			if event.LastSeenAt == "" {
				event.LastSeenAt = old.LastSeenAt
			}
			// Acknowledgement is changed through the dedicated endpoint so an edit
			// cannot silently move an already reviewed event back to pending.
			if !event.Acknowledged {
				event.Acknowledged = old.Acknowledged
			}
		} else {
			event.CreatedAt = now
		}
	}
	event.UpdatedAt = now
	_, err = s.db.Exec(`INSERT INTO macro_events
		(id,code,name,category,country,impact,starts_at,scheduled_at,released_at,previous_value,forecast_value,actual_value,revision,a_share_date,source,source_url,source_kind,external_id,last_seen_at,description,status,acknowledged,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		code=excluded.code,name=excluded.name,category=excluded.category,country=excluded.country,impact=excluded.impact,
		starts_at=excluded.starts_at,scheduled_at=excluded.scheduled_at,released_at=excluded.released_at,previous_value=excluded.previous_value,forecast_value=excluded.forecast_value,actual_value=excluded.actual_value,revision=excluded.revision,a_share_date=excluded.a_share_date,source=excluded.source,source_url=excluded.source_url,
		source_kind=excluded.source_kind,external_id=excluded.external_id,last_seen_at=excluded.last_seen_at,description=excluded.description,status=excluded.status,acknowledged=excluded.acknowledged,updated_at=excluded.updated_at`,
		event.ID, event.Code, event.Name, event.Category, event.Country, event.Impact, event.StartsAt, event.ScheduledAt, event.ReleasedAt, event.PreviousValue, event.ForecastValue, event.ActualValue, event.Revision, event.AShareDate,
		event.Source, event.SourceURL, event.SourceKind, event.ExternalID, event.LastSeenAt, event.Description, event.Status, boolInt(event.Acknowledged), event.CreatedAt, event.UpdatedAt)
	return event, err
}

func (s *AppStore) SetMacroEventAcknowledged(id string, acknowledged bool) (MacroEvent, error) {
	result, err := s.db.Exec(`UPDATE macro_events SET acknowledged=?, updated_at=? WHERE id=?`, boolInt(acknowledged), NowText(), id)
	if err != nil {
		return MacroEvent{}, err
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		return MacroEvent{}, sql.ErrNoRows
	}
	return s.GetMacroEvent(id)
}

func (s *AppStore) DeleteMacroEvent(id string) error {
	result, err := s.db.Exec(`DELETE FROM macro_events WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteStaleOfficialMacroEvents removes official records for a provider that
// were not seen in the latest successful parse. Manual events are untouched.
func (s *AppStore) DeleteStaleOfficialMacroEvents(source, lastSeenAt string) error {
	source = strings.TrimSpace(source)
	lastSeenAt = strings.TrimSpace(lastSeenAt)
	if source == "" || lastSeenAt == "" {
		return errors.New("官方事件来源和同步时间不能为空")
	}
	_, err := s.db.Exec(`DELETE FROM macro_events
		WHERE source_kind='official' AND source=? AND last_seen_at<>?`, source, lastSeenAt)
	return err
}

func (s *AppStore) ListMacroEventSyncStates() ([]MacroEventSyncState, error) {
	rows, err := s.db.Query(`SELECT id,provider,status,last_attempt_at,last_success_at,event_count,message,updated_at FROM macro_event_sync_state ORDER BY provider ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MacroEventSyncState{}
	for rows.Next() {
		var item MacroEventSyncState
		if err := rows.Scan(&item.ID, &item.Provider, &item.Status, &item.LastAttemptAt, &item.LastSuccessAt, &item.EventCount, &item.Message, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *AppStore) GetMacroEventSyncState(id string) (MacroEventSyncState, error) {
	var item MacroEventSyncState
	err := s.db.QueryRow(`SELECT id,provider,status,last_attempt_at,last_success_at,event_count,message,updated_at FROM macro_event_sync_state WHERE id=?`, id).
		Scan(&item.ID, &item.Provider, &item.Status, &item.LastAttemptAt, &item.LastSuccessAt, &item.EventCount, &item.Message, &item.UpdatedAt)
	return item, err
}

func (s *AppStore) UpsertMacroEventSyncState(state MacroEventSyncState) error {
	state.ID = strings.TrimSpace(state.ID)
	state.Provider = strings.TrimSpace(state.Provider)
	state.Status = strings.TrimSpace(state.Status)
	if state.ID == "" || state.Provider == "" {
		return errors.New("同步来源和标识不能为空")
	}
	if state.Status == "" {
		state.Status = "idle"
	}
	if state.LastAttemptAt == "" {
		state.LastAttemptAt = NowText()
	}
	state.UpdatedAt = NowText()
	_, err := s.db.Exec(`INSERT INTO macro_event_sync_state
		(id,provider,status,last_attempt_at,last_success_at,event_count,message,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET provider=excluded.provider,status=excluded.status,
		last_attempt_at=excluded.last_attempt_at,last_success_at=excluded.last_success_at,
		event_count=excluded.event_count,message=excluded.message,updated_at=excluded.updated_at`,
		state.ID, state.Provider, state.Status, state.LastAttemptAt, state.LastSuccessAt,
		state.EventCount, state.Message, state.UpdatedAt)
	return err
}

func (s *AppStore) GetMacroAlertSettings() (MacroAlertSettings, error) {
	var settings MacroAlertSettings
	var enabled, criticalOnly, notifyWebhooks int
	err := s.db.QueryRow(`SELECT id,enabled,lead_minutes,window_before_minutes,window_after_minutes,critical_only,notify_webhooks,webhook_ids,updated_at FROM macro_alert_settings WHERE id=?`, "default").
		Scan(&settings.ID, &enabled, &settings.LeadMinutes, &settings.WindowBeforeMinutes, &settings.WindowAfterMinutes, &criticalOnly, &notifyWebhooks, &settings.WebhookIDs, &settings.UpdatedAt)
	settings.Enabled = intBool(enabled)
	settings.CriticalOnly = intBool(criticalOnly)
	settings.NotifyWebhooks = intBool(notifyWebhooks)
	return settings, err
}

func (s *AppStore) UpsertMacroAlertSettings(settings MacroAlertSettings) (MacroAlertSettings, error) {
	if strings.TrimSpace(settings.WebhookIDs) == "" {
		settings.WebhookIDs = "[]"
	}
	var webhookIDs []string
	if err := json.Unmarshal([]byte(settings.WebhookIDs), &webhookIDs); err != nil {
		return settings, errors.New("webhook_ids不是有效JSON数组")
	}
	settings.ID = "default"
	if settings.LeadMinutes < 0 {
		settings.LeadMinutes = 0
	}
	if settings.LeadMinutes > 7*24*60 {
		settings.LeadMinutes = 7 * 24 * 60
	}
	if settings.WindowBeforeMinutes < 0 {
		settings.WindowBeforeMinutes = 0
	}
	if settings.WindowBeforeMinutes > 7*24*60 {
		settings.WindowBeforeMinutes = 7 * 24 * 60
	}
	if settings.WindowAfterMinutes < 0 {
		settings.WindowAfterMinutes = 0
	}
	if settings.WindowAfterMinutes > 7*24*60 {
		settings.WindowAfterMinutes = 7 * 24 * 60
	}
	settings.UpdatedAt = NowText()
	_, err := s.db.Exec(`INSERT INTO macro_alert_settings
		(id,enabled,lead_minutes,window_before_minutes,window_after_minutes,critical_only,notify_webhooks,webhook_ids,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled,lead_minutes=excluded.lead_minutes,
		window_before_minutes=excluded.window_before_minutes,window_after_minutes=excluded.window_after_minutes,
		critical_only=excluded.critical_only,notify_webhooks=excluded.notify_webhooks,webhook_ids=excluded.webhook_ids,updated_at=excluded.updated_at`,
		settings.ID, boolInt(settings.Enabled), settings.LeadMinutes, settings.WindowBeforeMinutes,
		settings.WindowAfterMinutes, boolInt(settings.CriticalOnly), boolInt(settings.NotifyWebhooks), settings.WebhookIDs, settings.UpdatedAt)
	return settings, err
}

// ClaimMacroAlertDelivery atomically claims one event/channel/kind tuple.
// It returns false when the same notification was already delivered.
func (s *AppStore) ClaimMacroAlertDelivery(eventID, webhookID, alertKind string) (bool, error) {
	eventID = strings.TrimSpace(eventID)
	webhookID = strings.TrimSpace(webhookID)
	alertKind = strings.TrimSpace(alertKind)
	if eventID == "" || webhookID == "" || alertKind == "" {
		return false, errors.New("预警投递标识不能为空")
	}
	result, err := s.db.Exec(`INSERT OR IGNORE INTO macro_alert_deliveries(event_id,webhook_id,alert_kind,sent_at) VALUES (?,?,?,?)`, eventID, webhookID, alertKind, NowText())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// MacroEventAlertState returns the local risk-window state for an event.
func MacroEventAlertState(event MacroEvent, settings MacroAlertSettings, now time.Time) string {
	if !settings.Enabled {
		return "disabled"
	}
	if settings.CriticalOnly && event.Impact != "high" && event.Impact != "critical" {
		return "muted"
	}
	startsAt, err := time.Parse(time.RFC3339, event.StartsAt)
	if err != nil {
		return "unknown"
	}
	windowStart := startsAt.Add(-time.Duration(settings.WindowBeforeMinutes) * time.Minute)
	windowEnd := startsAt.Add(time.Duration(settings.WindowAfterMinutes) * time.Minute)
	if !now.Before(windowStart) && now.Before(windowEnd) {
		return "window_active"
	}
	if now.Before(startsAt.Add(-time.Duration(settings.LeadMinutes) * time.Minute)) {
		return "scheduled"
	}
	if now.Before(startsAt) {
		return "alert_due"
	}
	return "released"
}
