package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/injoyai/tdx/protocol"
)

func TestHikyuuDataServiceClientFetchKlineConvertsUnits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hikyuu/kline" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("code") != "000001.SZ" ||
			r.URL.Query().Get("type") != "day" ||
			r.URL.Query().Get("recover") != "qfq" ||
			r.URL.Query().Get("limit") != "2" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "success",
			"data": map[string]interface{}{
				"symbol":  "000001.SZ",
				"period":  "day",
				"recover": "qfq",
				"count":   1,
				"list": []map[string]interface{}{
					{
						"time":   "2026-08-28T15:00:00+08:00",
						"last":   9.5,
						"open":   10.0,
						"high":   10.5,
						"low":    9.8,
						"close":  10.2,
						"volume": 1234,
						"amount": 567.8,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := &HikyuuDataServiceClient{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	resp, err := client.FetchKline(context.Background(), "sz000001", "day", "qfq", 2)
	if err != nil {
		t.Fatalf("FetchKline returned error: %v", err)
	}
	if resp.Count != 1 || len(resp.List) != 1 {
		t.Fatalf("unexpected response size: count=%d list=%d", resp.Count, len(resp.List))
	}

	item := resp.List[0]
	if item.Open != protocol.Yuan(10) || item.Close != protocol.Yuan(10.2) {
		t.Fatalf("unexpected prices: open=%v close=%v", item.Open, item.Close)
	}
	if item.Volume != 1234 {
		t.Fatalf("unexpected day volume conversion: %d", item.Volume)
	}
	if item.Amount != protocol.Yuan(567.8*10000) {
		t.Fatalf("unexpected amount conversion: %v", item.Amount)
	}
}

func TestHikyuuVolumeToTDX(t *testing.T) {
	if got := hikyuuVolumeToTDX("day", 1234); got != 1234 {
		t.Fatalf("unexpected day volume: %d", got)
	}
	if got := hikyuuVolumeToTDX("minute1", 123400); got != 1234 {
		t.Fatalf("unexpected minute volume: %d", got)
	}
}

func TestFetchHikyuuKlineRejectsUnsupportedPeriod(t *testing.T) {
	oldClient := hikyuuClient
	hikyuuClient = &HikyuuDataServiceClient{baseURL: "http://example.invalid"}
	t.Cleanup(func() {
		hikyuuClient = oldClient
	})

	if _, err := fetchHikyuuKline(context.Background(), "000001", "minute15", 100); err == nil {
		t.Fatal("expected unsupported period error")
	}
}

func TestHikyuuDataServiceClientFetchKlineBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/hikyuu/kline/batch" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0, "message": "success",
			"data": map[string]interface{}{
				"period": "day", "recover": "qfq",
				"data": map[string]interface{}{
					"000001": map[string]interface{}{
						"symbol": "000001", "period": "day", "recover": "qfq", "count": 1,
						"list": []map[string]interface{}{{
							"time": "2026-09-11T15:00:00+08:00", "open": 10.0, "high": 10.5,
							"low": 9.8, "close": 10.2, "volume": 1234, "amount": 567.8,
						}},
					},
				},
				"errors": map[string]string{"000002": "no kline data"},
			},
		})
	}))
	defer server.Close()

	client := &HikyuuDataServiceClient{baseURL: server.URL, httpClient: server.Client()}
	data, failures, err := client.FetchKlineBatch(context.Background(), []string{"000001", "000002"}, "day", "qfq", 260)
	if err != nil {
		t.Fatal(err)
	}
	if len(data["000001"].List) != 1 || data["000001"].List[0].Close != protocol.Yuan(10.2) {
		t.Fatalf("unexpected batch data: %+v", data)
	}
	if failures["000002"] != "no kline data" {
		t.Fatalf("unexpected failures: %+v", failures)
	}
}

func TestFetchQuotesWithFallbackUsesHikyuuClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/hikyuu/kline/batch" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0, "message": "success",
			"data": map[string]interface{}{
				"period": "day", "recover": "none",
				"data": map[string]interface{}{
					"601899.SH": map[string]interface{}{
						"symbol": "601899.SH", "period": "day", "recover": "none", "count": 2,
						"list": []map[string]interface{}{
							{"time": "2026-09-17T15:00:00+08:00", "open": 10.0, "high": 10.5, "low": 9.8, "close": 10.2, "volume": 1000, "amount": 500.0},
							{"time": "2026-09-18T15:00:00+08:00", "last": 10.2, "open": 10.2, "high": 10.8, "low": 10.1, "close": 10.6, "volume": 1200, "amount": 600.0},
						},
					},
				},
				"errors": map[string]string{},
			},
		})
	}))
	defer server.Close()

	oldTDX, oldHikyuu := client, hikyuuClient
	client = nil
	hikyuuClient = &HikyuuDataServiceClient{baseURL: server.URL, httpClient: server.Client()}
	t.Cleanup(func() {
		client, hikyuuClient = oldTDX, oldHikyuu
	})

	symbol, err := NormalizeSymbol("601899.sh")
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := fetchQuotesWithFallback(context.Background(), []Symbol{symbol})
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].Kline == nil {
		t.Fatalf("unexpected quotes: %+v", quotes)
	}
	if quotes[0].Kline.Close != protocol.Yuan(10.6) || quotes[0].Kline.Last != protocol.Yuan(10.2) {
		t.Fatalf("unexpected quote kline: %+v", quotes[0].Kline)
	}
}

func TestHandleGetKlineAllTDXUsesHikyuuWithoutTDXClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hikyuu/kline" || r.URL.Query().Get("type") != "week" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0, "message": "success",
			"data": map[string]interface{}{
				"symbol": "601899.SH", "period": "week", "recover": "qfq", "count": 1,
				"list": []map[string]interface{}{{
					"time": "2026-09-18T15:00:00+08:00", "last": 32.2, "open": 31.8,
					"high": 32.5, "low": 30.5, "close": 31.4, "volume": 1000, "amount": 600.0,
				}},
			},
		})
	}))
	defer server.Close()

	oldTDX, oldHikyuu := client, hikyuuClient
	client = nil
	hikyuuClient = &HikyuuDataServiceClient{baseURL: server.URL, httpClient: server.Client()}
	t.Cleanup(func() {
		client, hikyuuClient = oldTDX, oldHikyuu
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/kline-all/tdx?code=601899.sh&type=week&limit=800", nil)
	handleGetKlineAllTDX(recorder, request)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Count int `json:"count"`
			Meta  struct {
				Source string `json:"source"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 0 || response.Data.Count != 1 || response.Data.Meta.Source != "hikyuu" {
		t.Fatalf("unexpected response: %+v body=%s", response, recorder.Body.String())
	}
}

func TestEntryRiskKlineRequestsUnadjustedPrices(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("recover"); got != "none" {
			t.Errorf("recover=%q, want unadjusted entry-price basis", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"count": 1, "list": []map[string]interface{}{{"time": "2026-10-21T00:00:00+08:00", "open": 10, "high": 11, "low": 9, "close": 10, "volume": 100}},
		}})
	}))
	defer upstream.Close()
	previous := hikyuuClient
	hikyuuClient = &HikyuuDataServiceClient{baseURL: upstream.URL, httpClient: upstream.Client()}
	t.Cleanup(func() { hikyuuClient = previous })
	response := httptest.NewRecorder()
	handleGetKlineHistory(response, httptest.NewRequest(http.MethodGet, "/api/kline-history?code=600178&type=day&limit=120&recover=none", nil))
	var envelope struct {
		Code int                `json:"code"`
		Data protocol.KlineResp `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || len(envelope.Data.List) != 1 || envelope.Data.List[0].Close != protocol.Yuan(10) {
		t.Fatalf("unexpected risk bars: %s", response.Body.String())
	}
}
