package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
	workbench "workbench-core"
)

func TestSelectionTrackingSignalDate(t *testing.T) {
	iso, numeric, ok := selectionTrackingSignalDate("2026-09-21T16:30:00+08:00")
	if !ok || iso != "2026-09-21" || numeric != 20260921 {
		t.Fatalf("iso=%q numeric=%d ok=%v", iso, numeric, ok)
	}
	if _, _, ok := selectionTrackingSignalDate("invalid"); ok {
		t.Fatal("invalid date must not parse")
	}
}

func TestSelectionTrackingSignalDatePrefersSignalAt(t *testing.T) {
	iso, numeric, ok := selectionTrackingSignalDateForItem(SelectionResult{
		SignalAt:  "2026-09-18T14:45:00+08:00",
		CreatedAt: "not-a-date",
	})
	if !ok || iso != "2026-09-18" || numeric != 20260918 {
		t.Fatalf("signal_at should be used even when created_at is malformed: %q %d %v", iso, numeric, ok)
	}
	iso, numeric, ok = selectionTrackingSignalDateForItem(SelectionResult{CreatedAt: "2026-09-19T16:00:00+08:00"})
	if !ok || iso != "2026-09-19" || numeric != 20260919 {
		t.Fatalf("legacy rows should fall back to created_at: %q %d %v", iso, numeric, ok)
	}
	iso, numeric, ok = selectionTrackingSignalDateForItem(SelectionResult{SignalAt: "invalid", CreatedAt: "2026-09-20T16:00:00+08:00"})
	if !ok || iso != "2026-09-20" || numeric != 20260920 {
		t.Fatalf("unparseable signal_at should fall back to created_at: %q %d %v", iso, numeric, ok)
	}
}

func TestCaptureNextQuoteItemsRequiresLaterMarketTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 45, 6, 0, time.FixedZone("CST", 8*3600))
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/quote" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeMarketTestResponse(t, w, []*protocol.Quote{{
			Code:       "000001",
			Kline:      &protocol.Kline{Time: now, Close: 10200},
			ServerTime: "144505",
			SellLevel:  protocol.PriceLevels{{Price: 10200, Number: 100}},
		}})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })

	item := SelectionResult{ID: "next-quote", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10, Latest: 10, SnapshotStatus: "fresh"}
	updated := (&AutomationRunner{}).captureNextQuoteItemsAt(context.Background(), []SelectionResult{item}, now)
	captured, ok := updated[item.ID]
	if !ok || captured.NextQuotePrice != 10.2 || captured.NextQuoteAt != "2026-09-01T14:45:05+08:00" || captured.NextQuoteSource != "market-service.quote.ask1" || captured.QuoteAt != "" {
		t.Fatalf("later quote was not captured: %#v", updated)
	}
	if got := (&AutomationRunner{}).captureNextQuoteItemsAt(context.Background(), []SelectionResult{item}, now.Add(24*time.Hour)); len(got) != 0 {
		t.Fatalf("next-day quote filled old signal: %#v", got)
	}
}

func TestExecutionQuoteMarketTimeRejectsStaleAndMissingTimes(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 45, 6, 0, time.FixedZone("CST", 8*3600))
	for _, raw := range []string{"", "144000", "bad", "144505abc"} {
		if _, valid := executionQuoteMarketTime(raw, now); valid {
			t.Fatalf("quote time %q was accepted", raw)
		}
	}
	if got, valid := executionQuoteMarketTime("144505", now); !valid || got.Second() != 5 {
		t.Fatalf("fresh quote time was rejected: %v %v", got, valid)
	}
}

func TestCaptureNextQuoteFallsBackToLastTradeWithVolume(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 45, 6, 0, time.FixedZone("CST", 8*3600))
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		writeMarketTestResponse(t, w, []*protocol.Quote{{
			Code: "000001", Kline: &protocol.Kline{Time: now, Close: 10200},
			ServerTime: "144505", Intuition: 100,
		}})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })
	item := SelectionResult{ID: "last-trade", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	got := (&AutomationRunner{}).captureNextQuoteItemsAt(context.Background(), []SelectionResult{item}, now)[item.ID]
	if got.NextQuotePrice != 10.2 || got.NextQuoteSource != "market-service.quote.last" {
		t.Fatalf("valid last trade was not captured: %#v", got)
	}
}

func TestApplyBenchmarkToTrackingSkipsD0Observation(t *testing.T) {
	item := SelectionResult{ID: "benchmark-d0", Symbol: "000001", SignalAt: "2026-09-01T14:45:00+08:00", SignalPrice: 10}
	stockBars := []TrackingBar{
		{Date: 20260901, Open: 9, High: 12, Low: 8, Close: 10},
		{Date: 20260902, Open: 10, High: 11, Low: 10, Close: 11},
	}
	benchmarkBars := []TrackingBar{
		{Date: 20260901, Open: 100, High: 101, Low: 99, Close: 100},
		{Date: 20260902, Open: 100, High: 103, Low: 100, Close: 102},
	}
	tracking := workbench.EvaluateSelectionTracking(item, stockBars, []int{1}, 0, 0, time.Now())
	applyBenchmarkToTracking(&tracking, item, stockBars, benchmarkBars)
	if got := tracking.Horizons["d1"].BenchmarkCloseReturn; got != 2 {
		t.Fatalf("D1 benchmark return = %v, want 2%%", got)
	}
}

func TestTrackingRowsCoverDate(t *testing.T) {
	rows := []FormulaKline{{Date: 20260918}, {Date: 20260921}}
	if !trackingRowsCoverDate(rows, 20260918) {
		t.Fatal("signal date should be covered")
	}
	if trackingRowsCoverDate(rows, 20260917) {
		t.Fatal("rows starting after signal date must not count as covered")
	}
}

func TestSelectionTrackingRunMessageReportsSkippedRecords(t *testing.T) {
	message := selectionTrackingRunMessage(SelectionTrackingRunResult{
		CandidateCount: 100,
		UpdatedCount:   96,
		FailedCount:    4,
		SkippedSymbols: 2,
	}, 96)
	if message != "完成，更新 96/100 条，跳过 2 只股票、4 条记录" {
		t.Fatalf("message = %q", message)
	}
}

func TestLoadTrackingKlinesWithRetryKeepsSuccessfulSymbols(t *testing.T) {
	calls := map[string]int{}
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Symbols []string `json:"symbols"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		data := map[string]interface{}{}
		errorsBySymbol := map[string]string{}
		for _, symbol := range request.Symbols {
			calls[symbol]++
			if symbol == "000001" {
				data[symbol] = map[string]interface{}{
					"Count": 1,
					"List": []map[string]interface{}{{
						"Time": "2026-09-21T15:00:00+08:00", "Open": 10000, "High": 10100,
						"Low": 9900, "Close": 10050, "Volume": 1000, "Amount": 10050000,
					}},
				}
			} else {
				errorsBySymbol[symbol] = "测试K线失败"
			}
		}
		writeMarketTestResponse(t, w, map[string]interface{}{"data": data, "errors": errorsBySymbol})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })

	loaded, failures := (&AutomationRunner{}).loadTrackingKlinesWithRetry(context.Background(), []string{"000001", "000002"}, 40)
	if len(loaded["000001"]) != 1 || failures["000001"] != "" {
		t.Fatalf("successful symbol was not retained: loaded=%+v failures=%+v", loaded, failures)
	}
	if failures["000002"] != "测试K线失败" {
		t.Fatalf("failed symbol reason = %q", failures["000002"])
	}
	if calls["000001"] != 1 || calls["000002"] != selectionTrackingLoadAttempts {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestSelectionTrackingWebhookUsesUpdateCount(t *testing.T) {
	message := renderWebhookText(WebhookEvent{TaskType: "selection_tracking", MatchedCount: 12})
	if !strings.Contains(message, "更新数量: 12") || strings.Contains(message, "命中数量") {
		t.Fatalf("message = %q", message)
	}
}

func TestScheduledTradeDayHonorsTaskSwitch(t *testing.T) {
	runner := &AutomationRunner{}
	allowed, err := runner.gateScheduledTask(context.Background(), AutomationTask{OnlyTradeDay: false})
	if err != nil || !allowed {
		t.Fatalf("task with OnlyTradeDay=false should run without a calendar check: allowed=%v err=%v", allowed, err)
	}

	calls := 0
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/workday" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		calls++
		writeMarketTestResponse(t, w, map[string]interface{}{"is_workday": false})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })

	isWorkday, err := runner.scheduledTradeDay(context.Background(), "2026-09-25")
	if err != nil || isWorkday {
		t.Fatalf("expected calendar to report a non-trading day: isWorkday=%v err=%v", isWorkday, err)
	}
	if calls != 1 {
		t.Fatalf("calendar calls = %d, want 1 for OnlyTradeDay=true path", calls)
	}
}
