package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestLoadSystemBatchKlinesUsesMarketService(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hikyuu/kline/batch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeMarketTestResponse(t, w, map[string]interface{}{
			"source": "market-service",
			"data": map[string]interface{}{
				"000001": map[string]interface{}{
					"Count": 1,
					"List": []map[string]interface{}{{
						"Time": "2026-09-11T15:00:00+08:00", "Open": 10000, "High": 10500,
						"Low": 9900, "Close": 10200, "Volume": 1000, "Amount": 102000000,
					}},
				},
			},
			"errors": map[string]string{"000002": "本地和通达信均无数据"},
		})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })

	runner := &AutomationRunner{}
	loaded, failures := runner.loadSystemBatchKlines(context.Background(), []string{"000001", "000002"}, 260)
	if len(loaded["000001"]) != 1 || loaded["000001"][0].Date != 20260911 {
		t.Fatalf("unexpected loaded rows: %+v", loaded)
	}
	if failures["000002"] == "" {
		t.Fatalf("expected per-symbol failure, got %+v", failures)
	}
}

func TestNormalizeSystemStrategyConfigLoadsSharedRankingHistory(t *testing.T) {
	config := StrategyConfig{CalcCount: 20}
	normalizeSystemStrategyConfig(&config)
	if config.CalcCount != strategyRankingMinimumBars {
		t.Fatalf("calc count = %d, want shared ranking minimum %d", config.CalcCount, strategyRankingMinimumBars)
	}
}

func TestLoadSystemBatchKlinesRunsBatchesConcurrently(t *testing.T) {
	t.Setenv("STRATEGY_KLINE_BATCH_CONCURRENCY", "3")
	var mu sync.Mutex
	active := 0
	maxActive := 0
	calls := 0
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Symbols []string `json:"symbols"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		active++
		calls++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		errorsBySymbol := make(map[string]string, len(request.Symbols))
		for _, symbol := range request.Symbols {
			errorsBySymbol[symbol] = "test"
		}
		writeMarketTestResponse(t, w, map[string]interface{}{
			"data":   map[string]interface{}{},
			"errors": errorsBySymbol,
		})
	})
	defer closeServer()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })

	symbols := make([]string, 257)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("%06d", i)
	}
	_, failures := (&AutomationRunner{}).loadSystemBatchKlines(context.Background(), symbols, 260)

	if calls != 3 {
		t.Fatalf("batch calls = %d, want 3", calls)
	}
	if maxActive < 2 {
		t.Fatalf("max active requests = %d, want concurrent requests", maxActive)
	}
	if len(failures) != len(symbols) {
		t.Fatalf("failures = %d, want %d", len(failures), len(symbols))
	}
}

func TestStrategyKlineBatchConcurrencyBounds(t *testing.T) {
	t.Setenv("STRATEGY_KLINE_BATCH_CONCURRENCY", "20")
	if got := strategyKlineBatchConcurrency(); got != 8 {
		t.Fatalf("concurrency = %d, want 8", got)
	}
	t.Setenv("STRATEGY_KLINE_BATCH_CONCURRENCY", "invalid")
	if got := strategyKlineBatchConcurrency(); got != 4 {
		t.Fatalf("default concurrency = %d, want 4", got)
	}
}
