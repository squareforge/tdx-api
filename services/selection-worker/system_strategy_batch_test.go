package main

import (
	"context"
	"net/http"
	"testing"
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
