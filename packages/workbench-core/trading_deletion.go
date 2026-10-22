package workbench

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
)

// DeleteTradingTrade removes a ledger row. Deleting an active buy also removes
// its FIFO allocation from sales so another lot cannot inherit those sales.
// Read, allocation and write share one transaction; no synthetic sale is added.
func (s *AppStore) DeleteTradingTrade(id string) (TradingSystemState, error) {
	var state TradingSystemState
	id = strings.TrimSpace(id)
	if id == "" {
		return state, errors.New("请选择要删除的交易记录")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return state, err
	}
	defer tx.Rollback()
	var raw, createdAt string
	if err := tx.QueryRow(`SELECT state_json,created_at FROM trading_system_state WHERE id=?`, TradingSystemStateID).Scan(&raw, &createdAt); err != nil {
		return state, err
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, err
	}
	state = normalizeTradingSystemState(state)
	state.Trades, err = removeTradingTrade(state.Trades, id)
	if err != nil {
		return state, err
	}
	if err := validateTradingDirections(state.Trades); err != nil {
		return state, err
	}
	if err := validateTradeAlerts(state.Trades); err != nil {
		return state, err
	}
	state.CreatedAt, state.UpdatedAt = createdAt, NowText()
	payload, err := json.Marshal(state)
	if err != nil {
		return state, err
	}
	if _, err := tx.Exec(`UPDATE trading_system_state SET state_json=?,updated_at=? WHERE id=?`, string(payload), state.UpdatedAt, TradingSystemStateID); err != nil {
		return state, err
	}
	return state, tx.Commit()
}

func removeTradingTrade(trades []TradingTrade, id string) ([]TradingTrade, error) {
	var target *TradingTrade
	for i := range trades {
		if trades[i].ID == id {
			target = &trades[i]
			break
		}
	}
	if target == nil {
		return nil, sql.ErrNoRows
	}
	removedShares := map[string]float64{}
	if target.Direction != "sell" && target.Status == "active" {
		buys, sells := []TradingTrade{}, []TradingTrade{}
		for _, trade := range trades {
			if TradingCode(trade.StockCode) != TradingCode(target.StockCode) {
				continue
			}
			if trade.Direction == "sell" {
				sells = append(sells, trade)
			} else if trade.Status == "active" {
				buys = append(buys, trade)
			}
		}
		less := func(a, b TradingTrade) bool {
			if a.EntryDate == b.EntryDate {
				return a.ID < b.ID
			}
			return a.EntryDate < b.EntryDate
		}
		sort.SliceStable(buys, func(i, j int) bool { return less(buys[i], buys[j]) })
		sort.SliceStable(sells, func(i, j int) bool { return less(sells[i], sells[j]) })
		start := 0.0
		for _, buy := range buys {
			if buy.ID == id {
				break
			}
			start += buy.Shares
		}
		end, sold := start+target.Shares, 0.0
		for _, sale := range sells {
			// Buy and sell quantities occupy intervals on the same FIFO ledger.
			// Only the overlap belongs to the deleted lot.
			removedShares[sale.ID] = math.Max(0, math.Min(end, sold+sale.Shares)-math.Max(start, sold))
			sold += sale.Shares
		}
	}
	next := make([]TradingTrade, 0, len(trades))
	for _, trade := range trades {
		if trade.ID == id {
			continue
		}
		if removed := removedShares[trade.ID]; removed > 0 {
			left := trade.Shares - removed
			if left <= 0.000001 {
				continue
			}
			fraction := left / trade.Shares
			trade.ActualR *= fraction
			trade.PlannedR *= fraction
			trade.Shares = left
		}
		next = append(next, trade)
	}
	return next, nil
}
