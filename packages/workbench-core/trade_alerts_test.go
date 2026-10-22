package workbench

import (
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTradeAlertDailyClaimSurvivesRestartAndConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.db")
	open := func() *AppStore {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		store := &AppStore{db: db}
		if err := store.migrate(); err != nil {
			t.Fatal(err)
		}
		return store
	}
	store := open()
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := store.ClaimTradeAlertDelivery("lot-1", "stop_loss", "hook-1", "2026-10-22")
			if err != nil {
				t.Error(err)
			}
			if claimed {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("concurrent winners = %d", winners.Load())
	}
	if err := store.FinishTradeAlertDelivery("lot-1", "stop_loss", "hook-1", "2026-10-22", "sent"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	defer store.Close()
	claimed, err := store.ClaimTradeAlertDelivery("lot-1", "stop_loss", "hook-1", "2026-10-22")
	if err != nil || claimed {
		t.Fatalf("sent delivery reclaimed after restart: %v %v", claimed, err)
	}
	for _, key := range [][4]string{
		{"lot-1", "stop_loss", "hook-1", "2026-10-23"},
		{"lot-1", "take_profit_1", "hook-1", "2026-10-22"},
		{"lot-1", "stop_loss", "hook-2", "2026-10-22"},
		{"lot-2", "stop_loss", "hook-1", "2026-10-22"},
	} {
		if claimed, err := store.ClaimTradeAlertDelivery(key[0], key[1], key[2], key[3]); err != nil || !claimed {
			t.Fatalf("independent delivery rejected: %v %v", key, err)
		}
	}
}

func TestTradeAlertRetriesOnlyExplicitFailures(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	for _, status := range []string{"sent", "pending", "uncertain", "cancelled", "failed"} {
		if _, err := store.ClaimTradeAlertDelivery(status, "stop_loss", "hook", "2026-10-22"); err != nil {
			t.Fatal(err)
		}
		if status != "pending" {
			if err := store.FinishTradeAlertDelivery(status, "stop_loss", "hook", "2026-10-22", status); err != nil {
				t.Fatal(err)
			}
		}
		old := time.Now().UTC().Add(-2 * time.Minute).Format("2006-01-02T15:04:05.000000000Z")
		if _, err := store.db.Exec(`UPDATE trade_alert_deliveries SET updated_at=? WHERE trade_id=?`, old, status); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.ClaimTradeAlertDelivery(status, "stop_loss", "hook", "2026-10-22")
		if err != nil || claimed != (status == "failed") {
			t.Fatalf("status %s: claimed=%v err=%v", status, claimed, err)
		}
	}
	for attempt := 2; attempt <= 3; attempt++ {
		if err := store.FinishTradeAlertDelivery("failed", "stop_loss", "hook", "2026-10-22", "failed"); err != nil {
			t.Fatal(err)
		}
		if claimed, _ := store.ClaimTradeAlertDelivery("failed", "stop_loss", "hook", "2026-10-22"); claimed {
			t.Fatal("retry ignored the cooldown")
		}
		if _, err := store.db.Exec(`UPDATE trade_alert_deliveries SET updated_at=? WHERE trade_id='failed'`, time.Now().UTC().Add(-2*time.Minute).Format("2006-01-02T15:04:05.000000000Z")); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.ClaimTradeAlertDelivery("failed", "stop_loss", "hook", "2026-10-22")
		if err != nil || claimed != (attempt < 3) {
			t.Fatalf("attempt %d: %v %v", attempt, claimed, err)
		}
	}
}

func TestRemainingTradeSharesAllocatesSellsToEarlierLots(t *testing.T) {
	trades := []TradingTrade{
		{ID: "new", StockCode: "SH600178", Status: "active", Shares: 300, EntryDate: "2026-10-22"},
		{ID: "sell", StockCode: "600178.SH", Direction: "sell", Shares: 150},
		{ID: "old", StockCode: "600178", Status: "active", Shares: 100, EntryDate: "2026-10-21"},
		{ID: "closed", StockCode: "600178", Status: "closed", Shares: 500},
	}
	remaining := RemainingTradeShares(trades)
	if remaining["old"] != 0 || remaining["new"] != 250 || remaining["closed"] != 0 {
		t.Fatalf("remaining = %v", remaining)
	}
}

func TestTradingRiskFieldsPersistAndValidate(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	trade := TradingTrade{ID: "lot", StockCode: "600178", StockName: "东安动力", Direction: "buy", Status: "active", EntryPrice: 12.9, Shares: 100, StopPrice: 11.82, TargetPrice1: 14.52, TargetPrice2: 15.6, AlertEnabled: true, AlertWebhookIDs: []string{"hook"}, ManualRiskFields: []string{}, RiskBasis: "Wilder ATR14", RiskAsOf: "2026-10-21"}
	state := TradingSystemState{Trades: []TradingTrade{trade}}
	if _, err := store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetTradingSystemState()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Trades[0]
	if got.StopPrice != trade.StopPrice || got.RiskAsOf != trade.RiskAsOf || got.ManualRiskFields == nil || !got.AlertEnabled {
		t.Fatalf("lost risk fields: %+v", got)
	}
	for _, mutate := range []func(*TradingTrade){
		func(x *TradingTrade) { x.StopPrice = 13 },
		func(x *TradingTrade) { x.TargetPrice1 = 12 },
		func(x *TradingTrade) { x.TargetPrice2 = 14 },
		func(x *TradingTrade) { x.AlertWebhookIDs = nil },
	} {
		bad := trade
		mutate(&bad)
		if _, err := store.UpsertTradingSystemState(TradingSystemState{Trades: []TradingTrade{bad}}); err == nil {
			t.Fatalf("invalid alert accepted: %+v", bad)
		}
	}
}
