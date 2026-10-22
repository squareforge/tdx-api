package workbench

import (
	"database/sql"
	"errors"
	"testing"
)

func TestDeleteHoldingRemovesItsPartialSalesAndPreservesOtherLots(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	state := TradingSystemState{
		Account: TradingAccount{Principal: 50000},
		Trades: []TradingTrade{
			{ID: "new", StockCode: "600178.SH", Direction: "buy", Status: "active", EntryDate: "2026-10-23", EntryPrice: 13, Shares: 200},
			{ID: "other-stock", StockCode: "601899", Direction: "buy", Status: "active", EntryPrice: 20, Shares: 100},
			{ID: "sale-shared", StockCode: "SH600178", Direction: "sell", Status: "closed", EntryDate: "2026-10-24", Shares: 100, ActualR: 400, PlannedR: 200},
			{ID: "old", StockCode: "600178", Direction: "buy", Status: "active", EntryDate: "2026-10-22", EntryPrice: 12.9, Shares: 100, StopPrice: 12, AlertEnabled: true, AlertWebhookIDs: []string{"hook"}},
			{ID: "sale-old", StockCode: "600178", Direction: "sell", Status: "closed", EntryDate: "2026-10-23", Shares: 50},
		},
	}
	if _, err := store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	before := RemainingTradeShares(state.Trades)
	got, err := store.DeleteTradingTrade("old")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Trades) != 3 || got.Account.Principal != 50000 {
		t.Fatalf("unexpected remaining state: %+v", got)
	}
	for _, trade := range got.Trades {
		if trade.ID == "old" || trade.ID == "sale-old" {
			t.Fatalf("deleted lot or fully allocated sale remains: %+v", trade)
		}
		if trade.ID == "sale-shared" && (trade.Shares != 50 || trade.ActualR != 200 || trade.PlannedR != 100) {
			t.Fatalf("shared sale not split correctly: %+v", trade)
		}
	}
	after := RemainingTradeShares(got.Trades)
	if after["new"] != before["new"] || after["other-stock"] != before["other-stock"] {
		t.Fatalf("other lots changed: before=%v after=%v", before, after)
	}
	saved, err := store.GetTradingSystemState()
	if err != nil || len(saved.Trades) != len(got.Trades) {
		t.Fatalf("delete not persisted: %+v %v", saved, err)
	}
	// The last remaining buy and its sale can be removed directly as well.
	got, err = store.DeleteTradingTrade("new")
	if err != nil || len(got.Trades) != 1 || got.Trades[0].ID != "other-stock" {
		t.Fatalf("last holding could not be cleared: %+v %v", got.Trades, err)
	}
}

func TestDeleteHoldingOnlyRemovesSalesAllocatedToTheSelectedLot(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	state := TradingSystemState{Trades: []TradingTrade{
		{ID: "a", StockCode: "600178", Status: "active", EntryDate: "2026-10-22", Shares: 100},
		{ID: "b", StockCode: "600178", Status: "active", EntryDate: "2026-10-22", Shares: 100},
		{ID: "c", StockCode: "600178", Status: "active", EntryDate: "2026-10-23", Shares: 100},
		{ID: "sale", StockCode: "600178", Direction: "sell", EntryDate: "2026-10-24", Shares: 250},
	}}
	if _, err := store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	got, err := store.DeleteTradingTrade("b")
	if err != nil {
		t.Fatal(err)
	}
	remaining := RemainingTradeShares(got.Trades)
	if remaining["a"] != 0 || remaining["c"] != 50 {
		t.Fatalf("FIFO allocation changed for other buys: %v", remaining)
	}
	for _, trade := range got.Trades {
		if trade.ID == "sale" && trade.Shares != 150 {
			t.Fatalf("wrong retained sale: %+v", trade)
		}
	}
}

func TestDeleteSaleRestoresSharesAndMissingIDDoesNotChangeState(t *testing.T) {
	store := newHistoricalBacktestTestStore(t)
	state := TradingSystemState{Trades: []TradingTrade{
		{ID: "buy", StockCode: "600178", Status: "active", Shares: 100},
		{ID: "sale", StockCode: "600178", Direction: "sell", Shares: 40},
	}}
	if _, err := store.UpsertTradingSystemState(state); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteTradingTrade("missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing ID: %v", err)
	}
	if _, err := store.DeleteTradingTrade(" "); err == nil {
		t.Fatal("blank ID accepted")
	}
	saved, err := store.GetTradingSystemState()
	if err != nil || len(saved.Trades) != 2 {
		t.Fatalf("failed delete changed state: %+v %v", saved, err)
	}
	got, err := store.DeleteTradingTrade("sale")
	if err != nil || len(got.Trades) != 1 || RemainingTradeShares(got.Trades)["buy"] != 100 {
		t.Fatalf("sale removal did not restore shares: %+v %v", got, err)
	}
}

func TestDeleteHoldingPreservesOtherRemainingShares(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		sold   float64
		target string
	}{
		{name: "unsold", target: "old"},
		{name: "partially_sold", sold: 40, target: "old"},
		{name: "unsold_later_lot", sold: 40, target: "new"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newHistoricalBacktestTestStore(t)
			state := TradingSystemState{Trades: []TradingTrade{
				{ID: "old", StockCode: "600178", Status: "active", EntryDate: "2026-10-22", Shares: 100},
				{ID: "new", StockCode: "600178", Status: "active", EntryDate: "2026-10-23", Shares: 100},
			}}
			if scenario.sold > 0 {
				state.Trades = append(state.Trades, TradingTrade{ID: "sale", StockCode: "600178", Direction: "sell", Shares: scenario.sold})
			}
			if _, err := store.UpsertTradingSystemState(state); err != nil {
				t.Fatal(err)
			}
			before := RemainingTradeShares(state.Trades)
			got, err := store.DeleteTradingTrade(scenario.target)
			if err != nil {
				t.Fatal(err)
			}
			after := RemainingTradeShares(got.Trades)
			for _, trade := range got.Trades {
				if trade.ID == scenario.target {
					t.Fatalf("selected row still exists: %+v", trade)
				}
				if after[trade.ID] != before[trade.ID] {
					t.Fatalf("another lot changed: before=%v after=%v", before, after)
				}
			}
		})
	}
}
