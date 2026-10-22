package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
)

func tradeAlertFixture(t *testing.T, handler http.HandlerFunc) (*QuoteMonitor, TradingSystemState, *MarketQuoteSnapshot, time.Time) {
	t.Helper()
	t.Chdir(t.TempDir())
	store, err := OpenAppStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	if _, err := store.UpsertWebhook(Webhook{ID: "hook", Name: "Local test", URL: server.URL, Method: "POST", Events: `["trade.alert"]`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	state := TradingSystemState{Trades: []TradingTrade{{ID: "lot", StockCode: "600178", StockName: "东安动力", Direction: "buy", Status: "active", EntryPrice: 10, Shares: 100, EntryDate: "2026-10-21", StopPrice: 9, TargetPrice1: 11, TargetPrice2: 12, AlertEnabled: true, AlertWebhookIDs: []string{"hook"}}}}
	if _, err := store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, shanghaiLocation())
	snapshot := &MarketQuoteSnapshot{Code: "600178", Symbol: "600178.SH", FetchedAt: now, Quote: &protocol.Quote{Code: "600178", Kline: &protocol.Kline{Time: now}}}
	return NewQuoteMonitor(store, nil), state, snapshot, now
}

func TestTradeAlertDailyDedupAndNextTradingDay(t *testing.T) {
	var count atomic.Int32
	monitor, _, snapshot, now := tradeAlertFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var event WebhookEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
		}
		if event.Event != "trade.alert" || event.Message == "" {
			t.Errorf("bad event: %+v", event)
		}
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 4; i++ {
		monitor.handleTradeAlertsAt(snapshot, 11.1, now)
	}
	if count.Load() != 1 {
		t.Fatalf("repeated first target: %d", count.Load())
	}
	monitor.handleTradeAlertsAt(snapshot, 12.2, now)
	monitor.handleTradeAlertsAt(snapshot, 11.1, now)
	if count.Load() != 2 {
		t.Fatalf("second target and pullback: %d", count.Load())
	}
	monitor.handleTradeAlertsAt(snapshot, 8.8, now)
	monitor.handleTradeAlertsAt(snapshot, 8.7, now)
	if count.Load() != 3 {
		t.Fatalf("repeated stop: %d", count.Load())
	}
	now = now.AddDate(0, 0, 1)
	snapshot.Quote.Kline.Time = now
	snapshot.FetchedAt = now
	monitor.handleTradeAlertsAt(snapshot, 8.8, now)
	if count.Load() != 4 {
		t.Fatalf("next day failed to rearm: %d", count.Load())
	}
}

func TestTradeAlertGapSkipsDelayedLowerTarget(t *testing.T) {
	var count atomic.Int32
	monitor, _, snapshot, now := tradeAlertFixture(t, func(w http.ResponseWriter, r *http.Request) { count.Add(1) })
	monitor.handleTradeAlertsAt(snapshot, 12.5, now)
	monitor.handleTradeAlertsAt(snapshot, 11.2, now)
	if count.Load() != 1 {
		t.Fatalf("lower target sent after gap: %d", count.Load())
	}
}

func TestTradeAlertStopsAfterDisableDeleteOrSale(t *testing.T) {
	for _, operation := range []string{"disable", "delete", "sell", "channel_disabled", "channel_deleted", "stale"} {
		t.Run(operation, func(t *testing.T) {
			var count atomic.Int32
			monitor, state, snapshot, now := tradeAlertFixture(t, func(w http.ResponseWriter, r *http.Request) { count.Add(1) })
			switch operation {
			case "disable":
				state.Trades[0].AlertEnabled = false
			case "delete":
				var err error
				state, err = monitor.store.DeleteTradingTrade("lot")
				if err != nil {
					t.Fatal(err)
				}
			case "sell":
				state.Trades = append(state.Trades, TradingTrade{ID: "sale", StockCode: "600178", Direction: "sell", Shares: 100, Status: "closed"})
			case "channel_disabled":
				hook, _ := monitor.store.GetWebhook("hook")
				hook.Enabled = false
				if _, err := monitor.store.UpsertWebhook(hook); err != nil {
					t.Fatal(err)
				}
			case "channel_deleted":
				if err := monitor.store.DeleteWebhook("hook"); err != nil {
					t.Fatal(err)
				}
			case "stale":
				snapshot.Stale = true
			}
			if _, err := monitor.store.UpsertTradingSystemState(state); err != nil {
				t.Fatal(err)
			}
			monitor.handleTradeAlertsAt(snapshot, 8.8, now)
			if count.Load() != 0 {
				t.Fatalf("%s still sent %d", operation, count.Load())
			}
		})
	}
}

func TestTradeAlertRechecksStateBetweenChannels(t *testing.T) {
	var second atomic.Int32
	var disable func()
	monitor, state, snapshot, now := tradeAlertFixture(t, func(w http.ResponseWriter, r *http.Request) { disable() })
	state.Trades[0].AlertWebhookIDs = append(state.Trades[0].AlertWebhookIDs, "second")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { second.Add(1) }))
	defer other.Close()
	if _, err := monitor.store.UpsertWebhook(Webhook{ID: "second", Name: "second", URL: other.URL, Enabled: true, Events: `["trade.alert"]`}); err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	disable = func() {
		state.Trades[0].AlertEnabled = false
		if _, err := monitor.store.UpsertTradingSystemState(state); err != nil {
			t.Error(err)
		}
	}
	monitor.handleTradeAlertsAt(snapshot, 8.8, now)
	if second.Load() != 0 {
		t.Fatal("sent stale queued channel after disable")
	}
}

func TestTradeAlertUnknownNetworkReceiptIsNotRetried(t *testing.T) {
	var count atomic.Int32
	monitor, _, snapshot, now := tradeAlertFixture(t, func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	monitor.handleTradeAlertsAt(snapshot, 8.8, now)
	monitor.handleTradeAlertsAt(snapshot, 8.8, now.Add(2*time.Minute))
	if count.Load() != 1 {
		t.Fatalf("ambiguous delivery retried: %d", count.Load())
	}
}

func TestFreshTradeQuoteRejectsStaleAndOffSession(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, shanghaiLocation())
	snapshot := &MarketQuoteSnapshot{Quote: &protocol.Quote{Kline: &protocol.Kline{Time: now}}}
	if !freshTradeQuote(snapshot, 10, now) {
		t.Fatal("fresh quote rejected")
	}
	if freshTradeQuote(snapshot, 10, now.Add(4*time.Minute)) {
		t.Fatal("stale quote accepted")
	}
	late := time.Date(2026, 10, 22, 16, 0, 0, 0, shanghaiLocation())
	snapshot.Quote.Kline.Time = late
	if freshTradeQuote(snapshot, 10, late) {
		t.Fatal("after-hours quote accepted")
	}
}
