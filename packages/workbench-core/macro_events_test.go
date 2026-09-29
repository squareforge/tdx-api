package workbench

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

func newMacroEventTestStore(t *testing.T) *AppStore {
	t.Helper()
	db, err := sql.Open("sqlite", "file:macro-events-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	store := &AppStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMacroEventsInitializationDoesNotSeedFixedDates(t *testing.T) {
	store := newMacroEventTestStore(t)
	if err := store.ensureMacroEvents(); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListMacroEvents("", "", "inflation", "high")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("fixed-date macro events should not be seeded, got %d: %+v", len(items), items)
	}
	states, err := store.ListMacroEventSyncStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("expected three provider sync states, got %d", len(states))
	}
}

func TestMacroEventValidationAndAcknowledgement(t *testing.T) {
	store := newMacroEventTestStore(t)
	if _, err := store.UpsertMacroEvent(MacroEvent{Code: "cpi", Name: "CPI", StartsAt: "2026-09-10 20:30"}); err == nil {
		t.Fatal("expected RFC3339 validation error")
	}
	event, err := store.UpsertMacroEvent(MacroEvent{Code: "cpi", Name: "CPI", StartsAt: "2026-09-10T20:30:00+08:00"})
	if err != nil {
		t.Fatal(err)
	}
	if event.Code != "CPI" || event.Country != "US" || event.Impact != "medium" || event.SourceKind != "manual" {
		t.Fatalf("normalization failed: %+v", event)
	}
	if _, err := store.SetMacroEventAcknowledged(event.ID, true); err != nil {
		t.Fatal(err)
	}
	event.Description = "updated"
	updated, err := store.UpsertMacroEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Acknowledged {
		t.Fatal("editing an acknowledged event should preserve acknowledgement")
	}
	if err := store.DeleteMacroEvent("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestMacroEventsInitializationRemovesLegacyReferenceRows(t *testing.T) {
	store := newMacroEventTestStore(t)
	if _, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-20260904-nfp", Code: "NFP", Name: "非农就业报告", Category: "employment", Country: "US", Impact: "high",
		StartsAt: "2026-09-04T20:30:00+08:00", AShareDate: "2026-09-07", Source: "BLS",
		SourceURL:   "https://www.bls.gov/schedule/news_release/empsit.htm",
		Description: "观察就业新增、失业率与平均时薪，判断美国经济冷热和通胀压力。",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureMacroEvents(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMacroEvent("macro-20260904-nfp"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected legacy reference row to be removed, got %v", err)
	}
}

func TestMacroEventsInitializationPreservesEditedLegacyReferenceRows(t *testing.T) {
	store := newMacroEventTestStore(t)
	legacy, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-20261002-nfp", Code: "NFP", Name: "非农就业报告", Category: "employment", Country: "US", Impact: "high",
		StartsAt: "2026-10-02T20:30:00+08:00", AShareDate: "2026-10-05", Source: "BLS",
		SourceURL:   "https://www.bls.gov/schedule/news_release/empsit.htm",
		Description: "观察就业新增、失业率与平均时薪，判断美国经济冷热和通胀压力。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMacroEventAcknowledged(legacy.ID, true); err != nil {
		t.Fatal(err)
	}
	customized, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-20260904-nfp", Code: "NFP", Name: "非农就业报告", Category: "employment", Country: "US", Impact: "high",
		StartsAt: "2026-09-04T20:30:00+08:00", AShareDate: "2026-09-07", Source: "BLS",
		SourceURL:   "https://www.bls.gov/schedule/news_release/empsit.htm",
		Description: "用户修改过的风险说明",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureMacroEvents(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMacroEvent(legacy.ID); err != nil {
		t.Fatalf("acknowledged legacy row was removed: %v", err)
	}
	if _, err := store.GetMacroEvent(customized.ID); err != nil {
		t.Fatalf("edited legacy row was removed: %v", err)
	}
}

func TestOfficialMacroEventSyncKeepsIdentityAndRemovesStaleRows(t *testing.T) {
	store := newMacroEventTestStore(t)
	manual, err := store.UpsertMacroEvent(MacroEvent{
		ID: "custom-bls", Code: "NFP", Name: "自定义就业事件",
		StartsAt: "2026-09-04T20:30:00+08:00", Source: "BLS",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-official-nfp-stale", Code: "NFP", Name: "非农就业报告",
		StartsAt: "2026-09-04T20:30:00+08:00", Source: "BLS", SourceKind: "official",
		ExternalID: "nfp-20260904", LastSeenAt: "sync-1",
	}); err != nil {
		t.Fatal(err)
	}
	first, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-official-nfp-20261002", Code: "NFP", Name: "非农就业报告",
		StartsAt: "2026-10-02T20:30:00+08:00", Source: "BLS", SourceKind: "official",
		ExternalID: "nfp-20261002", LastSeenAt: "sync-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMacroEventAcknowledged(first.ID, true); err != nil {
		t.Fatal(err)
	}
	edited, err := store.UpsertMacroEvent(MacroEvent{
		ID: first.ID, Code: "NFP", Name: "非农就业报告",
		StartsAt: "2026-10-02T20:30:00+08:00", Source: "BLS",
	})
	if err != nil || edited.SourceKind != "official" || edited.ExternalID != "nfp-20261002" {
		t.Fatalf("legacy edit dropped official metadata: %+v err=%v", edited, err)
	}
	updated, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-official-nfp-moved", Code: "NFP", Name: "非农就业报告",
		StartsAt: "2026-10-05T20:30:00+08:00", Source: "BLS", SourceKind: "official",
		ExternalID: "nfp-20261002", LastSeenAt: "sync-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != first.ID || !updated.Acknowledged {
		t.Fatalf("official update lost identity or acknowledgement: %+v", updated)
	}
	if err := store.DeleteStaleOfficialMacroEvents("BLS", "sync-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMacroEvent(first.ID); err != nil {
		t.Fatalf("current official event was removed: %v", err)
	}
	if _, err := store.GetMacroEvent("macro-official-nfp-stale"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale official event was not removed: %v", err)
	}
	if _, err := store.GetMacroEvent(manual.ID); err != nil {
		t.Fatalf("manual event was removed: %v", err)
	}
}

func TestMacroAlertSettingsAndState(t *testing.T) {
	store := newMacroEventTestStore(t)
	if err := store.ensureMacroEvents(); err != nil {
		t.Fatal(err)
	}
	settings, err := store.GetMacroAlertSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.LeadMinutes != 240 || settings.WindowBeforeMinutes != 240 || settings.WindowAfterMinutes != 120 {
		t.Fatalf("unexpected default settings: %+v", settings)
	}
	event := MacroEvent{Impact: "high", StartsAt: "2026-09-10T20:30:00+08:00"}
	startsAt, _ := time.Parse(time.RFC3339, event.StartsAt)
	if got := MacroEventAlertState(event, settings, startsAt.Add(-5*time.Hour)); got != "scheduled" {
		t.Fatalf("expected scheduled, got %s", got)
	}
	if got := MacroEventAlertState(event, settings, startsAt.Add(-time.Hour)); got != "window_active" {
		t.Fatalf("expected active window, got %s", got)
	}
	if got := MacroEventAlertState(event, settings, startsAt.Add(3*time.Hour)); got != "released" {
		t.Fatalf("expected released, got %s", got)
	}
	settings.WindowBeforeMinutes = 999999
	updated, err := store.UpsertMacroAlertSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if updated.WindowBeforeMinutes != 7*24*60 {
		t.Fatalf("expected settings clamp, got %d", updated.WindowBeforeMinutes)
	}
}

func TestMacroAlertDeliveryIsIdempotent(t *testing.T) {
	store := newMacroEventTestStore(t)
	claimed, err := store.ClaimMacroAlertDelivery("event-1", "hook-1", "alert_due")
	if err != nil || !claimed {
		t.Fatalf("expected first delivery claim, claimed=%v err=%v", claimed, err)
	}
	claimed, err = store.ClaimMacroAlertDelivery("event-1", "hook-1", "alert_due")
	if err != nil || claimed {
		t.Fatalf("expected duplicate delivery to be ignored, claimed=%v err=%v", claimed, err)
	}
}
