package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseBLSReleasePage(t *testing.T) {
	data := []byte(`<table><tr><td>Friday, September 4, 2026</td><td>8:30 AM</td><td>Employment Situation</td></tr><tr><td>October 2, 2026</td><td>Employment Situation</td></tr></table>`)
	events, err := parseBLSReleasePage(data, "NFP", "非农就业报告", "employment", blsEmploymentURL, "desc", "Employment Situation")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ID != "macro-official-nfp-20260904" || events[0].ExternalID != "nfp-20260904" || events[0].SourceKind != "official" || events[0].StartsAt != "2026-09-04T20:30:00+08:00" {
		t.Fatalf("unexpected BLS events: %+v", events)
	}
}

func TestParseFOMCPage(t *testing.T) {
	meetingMonth := time.Now().AddDate(0, 2, 0)
	firstDay := time.Date(meetingMonth.Year(), meetingMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	decisionDay := firstDay.AddDate(0, 0, 1)
	data := []byte(fmt.Sprintf(`<h3>%d</h3><div class="fomc-meeting"><div>%s</div><div>1-2</div></div>`, meetingMonth.Year(), meetingMonth.Month()))
	events, err := parseFOMCPage(data)
	if err != nil {
		t.Fatal(err)
	}
	wantID := fmt.Sprintf("macro-official-fomc-%s", firstDay.Format("20060102"))
	wantStartsAt := releaseAtET(decisionDay, 14, 0)
	if len(events) != 1 || events[0].ID != wantID || events[0].StartsAt != wantStartsAt {
		t.Fatalf("unexpected FOMC events: %+v", events)
	}
}

func TestParseBEASchedulePage(t *testing.T) {
	data := []byte(`<table><tr><td>September 30, 2026</td><td>8:30 AM</td><td>Personal Income and Outlays, August 2026</td></tr></table>`)
	events, err := parseBEASchedulePage(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "macro-official-pce-20260930" || events[0].StartsAt != "2026-09-30T20:30:00+08:00" {
		t.Fatalf("unexpected BEA events: %+v", events)
	}
}

func TestMacroCalendarSyncKeepsOtherBLSFeedOnPartialFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := OpenAppStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	previousStore := appStore
	appStore = store
	t.Cleanup(func() { appStore = previousStore })

	oldCPI, err := store.UpsertMacroEvent(MacroEvent{
		ID: "macro-official-cpi-old", Code: "CPI", Name: "美国 CPI 通胀", Source: "BLS",
		SourceKind: "official", ExternalID: "cpi-old", LastSeenAt: "previous-sync",
		StartsAt: "2026-10-13T20:30:00+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMacroEventSyncState(MacroEventSyncState{
		ID: macroSyncStateID("BLS"), Provider: "BLS", Status: "success",
		LastSuccessAt: "2026-09-28T12:00:00+08:00",
	}); err != nil {
		t.Fatal(err)
	}

	serveCPI := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cpi" && !serveCPI {
			http.Error(w, "temporarily unavailable", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("calendar"))
	}))
	defer server.Close()
	previousProviders := macroCalendarProviders
	macroCalendarProviders = []macroCalendarProvider{
		{name: "BLS", url: server.URL + "/nfp", parse: func([]byte) ([]MacroEvent, error) {
			return []MacroEvent{{ID: "macro-official-nfp-new", Code: "NFP", Name: "非农就业报告", Source: "BLS", StartsAt: "2026-10-02T20:30:00+08:00"}}, nil
		}},
		{name: "BLS", url: server.URL + "/cpi", parse: func([]byte) ([]MacroEvent, error) {
			return []MacroEvent{{ID: "macro-official-cpi-new", Code: "CPI", Name: "美国 CPI 通胀", Source: "BLS", StartsAt: "2026-11-13T20:30:00+08:00"}}, nil
		}},
		{name: "Federal Reserve", url: server.URL + "/fomc", parse: func([]byte) ([]MacroEvent, error) {
			return []MacroEvent{{ID: "macro-official-fomc-new", Code: "FOMC", Name: "美联储议息会议", Source: "Federal Reserve", StartsAt: "2026-10-30T02:00:00+08:00"}}, nil
		}},
		{name: "BEA", url: server.URL + "/pce", parse: func([]byte) ([]MacroEvent, error) {
			return []MacroEvent{{ID: "macro-official-pce-new", Code: "PCE", Name: "美国 PCE 物价指数", Source: "BEA", StartsAt: "2026-10-30T20:30:00+08:00"}}, nil
		}},
	}
	t.Cleanup(func() { macroCalendarProviders = previousProviders })

	result, err := syncMacroCalendars(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" {
		t.Fatalf("expected partial sync, got %s", result.Status)
	}
	if _, err := store.GetMacroEvent(oldCPI.ID); err != nil {
		t.Fatalf("CPI was removed while its feed failed: %v", err)
	}
	state, err := store.GetMacroEventSyncState(macroSyncStateID("BLS"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "partial" || state.LastSuccessAt != "2026-09-28T12:00:00+08:00" {
		t.Fatalf("partial sync advanced last success: %+v", state)
	}

	serveCPI = true
	result, err = syncMacroCalendars(context.Background())
	if err != nil || result.Status != "success" {
		t.Fatalf("complete sync failed: %+v err=%v", result, err)
	}
	if _, err := store.GetMacroEvent(oldCPI.ID); err == nil {
		t.Fatal("stale CPI record survived a complete sync")
	}
}
