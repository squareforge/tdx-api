package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTradingSystemDeleteHolding(t *testing.T) {
	t.Chdir(t.TempDir())
	store, err := OpenAppStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	previousStore := appStore
	appStore = store
	t.Cleanup(func() { appStore = previousStore })
	_, err = store.UpsertTradingSystemState(TradingSystemState{Trades: []TradingTrade{
		{ID: "buy", StockCode: "600178", Direction: "buy", Status: "active", Shares: 100, EntryPrice: 12.9, StopPrice: 12, AlertEnabled: true, AlertWebhookIDs: []string{"hook"}},
		{ID: "sale", StockCode: "600178", Direction: "sell", Shares: 40},
	}})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handleTradingSystemState(response, httptest.NewRequest(http.MethodDelete, "/api/trading-system?id=buy", nil))
	var result struct {
		Code int                `json:"code"`
		Data TradingSystemState `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 0 || len(result.Data.Trades) != 0 {
		t.Fatalf("delete response: %s", response.Body.String())
	}
	saved, err := store.GetTradingSystemState()
	if err != nil || len(saved.Trades) != 0 {
		t.Fatalf("deleted holding or sale still stored: %+v %v", saved.Trades, err)
	}
	for _, url := range []string{"/api/trading-system", "/api/trading-system?id=missing"} {
		response = httptest.NewRecorder()
		handleTradingSystemState(response, httptest.NewRequest(http.MethodDelete, url, nil))
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code == 0 {
			t.Fatalf("invalid deletion accepted: %s", response.Body.String())
		}
	}
}
