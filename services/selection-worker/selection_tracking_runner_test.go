package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
