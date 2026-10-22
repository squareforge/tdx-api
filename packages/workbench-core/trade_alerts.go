package workbench

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// TradingCode makes quote symbols and manually entered trading symbols comparable.
func TradingCode(symbol string) string {
	code := strings.ToUpper(strings.TrimSpace(symbol))
	for _, exchange := range []string{"SH", "SZ", "BJ"} {
		code = strings.TrimPrefix(code, exchange)
		code = strings.TrimSuffix(code, "."+exchange)
	}
	return code
}

func uniqueTradeAlertStrings(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func validateTradeAlerts(trades []TradingTrade) error {
	ids := map[string]bool{}
	for _, trade := range trades {
		if ids[trade.ID] {
			return errors.New("交易记录 ID 不能重复")
		}
		ids[trade.ID] = true
		for _, value := range []float64{trade.EntryPrice, trade.Shares, trade.StopPrice, trade.TargetPrice1, trade.TargetPrice2} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return errors.New("交易价格和股数必须是有效的非负数")
			}
		}
		if trade.Direction == "sell" {
			continue
		}
		if trade.StopPrice > 0 && (trade.EntryPrice <= 0 || trade.StopPrice >= trade.EntryPrice) {
			return fmt.Errorf("%s 止损价必须低于买入价", trade.StockName)
		}
		if (trade.TargetPrice1 > 0 && trade.TargetPrice1 <= trade.EntryPrice) || (trade.TargetPrice2 > 0 && trade.TargetPrice2 <= trade.EntryPrice) {
			return fmt.Errorf("%s 止盈价必须高于买入价", trade.StockName)
		}
		if trade.TargetPrice1 > 0 && trade.TargetPrice2 > 0 && trade.TargetPrice2 <= trade.TargetPrice1 {
			return fmt.Errorf("%s 第二止盈价必须高于第一止盈价", trade.StockName)
		}
		if trade.AlertEnabled && (trade.EntryPrice <= 0 || trade.Shares <= 0 || TradingCode(trade.StockCode) == "" || len(trade.AlertWebhookIDs) == 0 || (trade.StopPrice <= 0 && trade.TargetPrice1 <= 0 && trade.TargetPrice2 <= 0)) {
			return fmt.Errorf("%s 开启预警需要股票、买入价、股数、价位和 Webhook 渠道", trade.StockName)
		}
	}
	return nil
}

// RemainingTradeShares allocates sells to active buy lots in date/ID order (FIFO).
// A sold lot must not keep sending alerts while a newer lot is still held.
func RemainingTradeShares(trades []TradingTrade) map[string]float64 {
	sold := map[string]float64{}
	buys := []TradingTrade{}
	for _, trade := range trades {
		if trade.Direction == "sell" {
			sold[TradingCode(trade.StockCode)] += math.Max(0, trade.Shares)
		} else if trade.Status == "active" {
			buys = append(buys, trade)
		}
	}
	sort.SliceStable(buys, func(i, j int) bool {
		if buys[i].EntryDate == buys[j].EntryDate {
			return buys[i].ID < buys[j].ID
		}
		return buys[i].EntryDate < buys[j].EntryDate
	})
	remaining := map[string]float64{}
	for _, trade := range buys {
		code := TradingCode(trade.StockCode)
		consumed := math.Min(math.Max(0, trade.Shares), sold[code])
		sold[code] -= consumed
		remaining[trade.ID] = math.Max(0, trade.Shares-consumed)
	}
	return remaining
}

// ClaimTradeAlertDelivery reserves a daily delivery before HTTP dispatch. Pending
// claims are never retried: after a crash, remote receipt is unknown. Only an
// explicit rejection can be retried, at most three attempts with a minute gap.
func (s *AppStore) ClaimTradeAlertDelivery(tradeID, kind, webhookID, date string) (bool, error) {
	tradeID, kind, webhookID, date = strings.TrimSpace(tradeID), strings.TrimSpace(kind), strings.TrimSpace(webhookID), strings.TrimSpace(date)
	if tradeID == "" || kind == "" || webhookID == "" || date == "" {
		return false, errors.New("预警投递标识不能为空")
	}
	now := time.Now().UTC()
	const timestamp = "2006-01-02T15:04:05.000000000Z"
	result, err := s.db.Exec(`INSERT INTO trade_alert_deliveries
  (trade_id,alert_kind,webhook_id,alert_date,status,attempts,updated_at)
  VALUES (?,?,?,?,'pending',1,?)
  ON CONFLICT(trade_id,alert_kind,webhook_id,alert_date) DO UPDATE SET
   status='pending',attempts=trade_alert_deliveries.attempts+1,updated_at=excluded.updated_at
  WHERE trade_alert_deliveries.status='failed'
   AND trade_alert_deliveries.attempts < 3
   AND trade_alert_deliveries.updated_at < ?`,
		tradeID, kind, webhookID, date, now.Format(timestamp), now.Add(-time.Minute).Format(timestamp))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *AppStore) FinishTradeAlertDelivery(tradeID, kind, webhookID, date, status string) error {
	if status != "sent" && status != "failed" && status != "uncertain" && status != "cancelled" {
		return errors.New("无效的预警发送状态")
	}
	_, err := s.db.Exec(`UPDATE trade_alert_deliveries SET status=?,updated_at=?
  WHERE trade_id=? AND alert_kind=? AND webhook_id=? AND alert_date=? AND status='pending'`,
		status, time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), tradeID, kind, webhookID, date)
	return err
}
