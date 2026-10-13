package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestMarketClient(handler http.HandlerFunc) (*MarketServiceClient, func()) {
	server := httptest.NewServer(handler)
	client := &MarketServiceClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	return client, server.Close
}

func writeMarketTestResponse(t *testing.T, w http.ResponseWriter, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    data,
	}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func TestMarketServiceClientDisabled(t *testing.T) {
	t.Setenv("MARKET_SERVICE_URL", "")
	client := NewMarketServiceClient()
	if client.Enabled() {
		t.Fatal("expected client to be disabled without MARKET_SERVICE_URL")
	}
	if _, err := client.StockCodes(context.Background(), 10); err == nil {
		t.Fatal("expected request to fail when MARKET_SERVICE_URL is empty")
	}
}

func TestMarketServiceClientRawDataSuccess(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/codes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeMarketTestResponse(t, w, map[string]interface{}{
			"total": 1,
			"codes": []map[string]string{
				{"code": "000001", "name": "平安银行", "exchange": "sz"},
			},
		})
	})
	defer closeServer()

	raw, err := client.RawData(context.Background(), "/api/codes", nil)
	if err != nil {
		t.Fatalf("RawData returned error: %v", err)
	}
	data, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", raw)
	}
	if data["total"].(float64) != 1 {
		t.Fatalf("unexpected total: %v", data["total"])
	}
}

func TestMarketServiceClientErrorEnvelope(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			Code:    -1,
			Message: "行情服务失败",
		})
	})
	defer closeServer()

	err := client.get(context.Background(), "/api/fail", nil, &struct{}{})
	if err == nil || err.Error() != "行情服务失败" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarketInfoSyncOutlivesQueryTimeoutAndHonorsContext(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			writeMarketTestResponse(t, w, map[string]int{"success": 1})
		case <-r.Context().Done():
		}
	})
	defer closeServer()
	client.httpClient.Timeout = 20 * time.Millisecond
	if err := client.post(context.Background(), "/api/market/sync", nil, nil); err != nil {
		t.Fatalf("market sync was cut off by query timeout: %v", err)
	}
	if client.httpClient.Timeout != 20*time.Millisecond {
		t.Fatal("sync changed the shared query client")
	}
	if err := client.post(context.Background(), "/api/hikyuu/kline/batch", nil, nil); err == nil {
		t.Fatal("ordinary query should still use the query timeout")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.post(ctx, "/api/market/sync", nil, nil); err == nil {
		t.Fatal("market sync should honor task cancellation")
	}
}

func TestMarketServiceClientKlineHistory(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/kline-history" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("code") != "000001" || r.URL.Query().Get("type") != "day" || r.URL.Query().Get("limit") != "2" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		writeMarketTestResponse(t, w, map[string]interface{}{
			"Count": 2,
			"List": []map[string]interface{}{
				{"Open": 1000, "Close": 1100, "Volume": 10000},
				{"Open": 1100, "Close": 1200, "Volume": 20000},
			},
		})
	})
	defer closeServer()

	resp, err := client.KlineHistory(context.Background(), "000001", "day", 2)
	if err != nil {
		t.Fatalf("KlineHistory returned error: %v", err)
	}
	if resp.Count != 2 || len(resp.List) != 2 {
		t.Fatalf("unexpected kline response: count=%d list=%d", resp.Count, len(resp.List))
	}
	if resp.List[1].Close != 1200 {
		t.Fatalf("unexpected close: %v", resp.List[1].Close)
	}
}

func TestMarketServiceClientTrackingWorkdayWindow(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/workday":
			if r.URL.Query().Get("date") != "2026-09-21" || r.URL.Query().Get("count") != "30" {
				t.Fatalf("unexpected workday query: %s", r.URL.RawQuery)
			}
			previous := make([]map[string]string, 30)
			for index := range previous {
				previous[index] = map[string]string{"iso": fmt.Sprintf("2026-08-%02d", 30-index)}
			}
			previous[29]["iso"] = "2026-08-10"
			writeMarketTestResponse(t, w, map[string]interface{}{"previous": previous})
		case "/api/workday/range":
			if r.URL.Query().Get("start") != "2026-08-10" || r.URL.Query().Get("end") != "2026-09-21" {
				t.Fatalf("unexpected range query: %s", r.URL.RawQuery)
			}
			writeMarketTestResponse(t, w, map[string]interface{}{"count": 31})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	defer closeServer()

	start, err := client.PreviousTradingDate(context.Background(), "2026-09-21", 30)
	if err != nil || start != "2026-08-10" {
		t.Fatalf("start = %q, err = %v", start, err)
	}
	count, err := client.TradingDayCount(context.Background(), start, "2026-09-21")
	if err != nil || count != 31 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
}

func TestMarketServiceClientCompanyContent(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/company/content" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeMarketTestResponse(t, w, map[string]string{
			"content": "公司概况",
		})
	})
	defer closeServer()

	raw, err := client.RawData(context.Background(), "/api/company/content", nil)
	if err != nil {
		t.Fatalf("RawData returned error: %v", err)
	}
	content, ok := raw.(map[string]interface{})["content"].(string)
	if !ok || content != "公司概况" {
		t.Fatalf("unexpected content response: %#v", raw)
	}
}

func TestMarketServiceClientSyncGbbqTask(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/tasks/sync-gbbq" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		writeMarketTestResponse(t, w, map[string]string{
			"task_id": "task-1",
		})
	})
	defer closeServer()

	resp, err := client.SyncGbbqTask(context.Background())
	if err != nil {
		t.Fatalf("SyncGbbqTask returned error: %v", err)
	}
	if resp["task_id"] != "task-1" {
		t.Fatalf("unexpected task response: %#v", resp)
	}
}
