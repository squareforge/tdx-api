package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	workbench "workbench-core"
)

type QuoteAlert struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Rule      string    `json:"rule"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type QuoteMonitor struct {
	store        *AppStore
	market       *MarketServiceClient
	mu           sync.RWMutex
	alerts       []QuoteAlert
	lastTrigger  map[string]time.Time
	cancel       context.CancelFunc
	workdayDate  string
	workdayValid bool
}

func NewQuoteMonitor(store *AppStore, market *MarketServiceClient) *QuoteMonitor {
	return &QuoteMonitor{
		store: store, market: market, lastTrigger: map[string]time.Time{},
	}
}

func (m *QuoteMonitor) Start() {
	if m == nil || m.market == nil || !m.market.Enabled() || !envBool("SELECTION_QUOTE_MONITOR_ENABLED", true) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx)
}

func (m *QuoteMonitor) Stop() {
	if m != nil && m.cancel != nil {
		m.cancel()
	}
}

func (m *QuoteMonitor) run(ctx context.Context) {
	for {
		codes := m.monitorCodes()
		if len(codes) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}
		streamCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := m.market.SubscribeQuotes(streamCtx, codes, m.handleEvent)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || err == nil {
			continue
		}
		if err != nil && ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) {
			log.Printf("行情订阅监测断开: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (m *QuoteMonitor) monitorCodes() []string {
	if m == nil || m.store == nil {
		return nil
	}
	set := map[string]struct{}{}
	priority := map[string]bool{}
	if pool, err := m.store.GetStockPool(DecisionWatchPoolID); err == nil {
		for _, symbol := range pool.Symbols {
			set[normalizeMonitorSymbol(symbol)] = struct{}{}
		}
	}
	if notes, err := m.store.ListDecisionNotes("", 500); err == nil {
		for _, note := range notes {
			if note.PlanBuy > 0 || note.StopLoss > 0 {
				set[normalizeMonitorSymbol(note.Symbol)] = struct{}{}
			}
		}
	}
	if state, err := m.store.GetTradingSystemState(); err == nil {
		remaining := workbench.RemainingTradeShares(state.Trades)
		for _, trade := range state.Trades {
			if strings.EqualFold(trade.Direction, "sell") || trade.Status != "active" || !trade.AlertEnabled || remaining[trade.ID] <= 0 {
				continue
			}
			if symbol := normalizeMonitorSymbol(trade.StockCode); symbol != "" {
				set[symbol] = struct{}{}
				priority[symbol] = true
			}
		}
	}
	codes := make([]string, 0, len(set))
	for code := range set {
		if code != "" {
			codes = append(codes, code)
		}
	}
	sort.Slice(codes, func(i, j int) bool {
		if priority[codes[i]] != priority[codes[j]] {
			return priority[codes[i]]
		}
		return codes[i] < codes[j]
	})
	if len(codes) > 200 {
		codes = codes[:200]
	}
	return codes
}

func normalizeMonitorSymbol(symbol string) string {
	code := workbench.TradingCode(symbol)
	if len(code) != 6 {
		return ""
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return ""
		}
	}
	exchange := "SZ"
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "5") || strings.HasPrefix(code, "9") {
		exchange = "SH"
	}
	if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92") {
		exchange = "BJ"
	}
	return code + "." + exchange
}

func (m *QuoteMonitor) handleEvent(event MarketQuoteStreamEvent) {
	if event.Type != "quote" || event.Snapshot == nil || event.Snapshot.Quote == nil || event.Snapshot.Quote.Kline == nil {
		return
	}
	price := event.Snapshot.Quote.Kline.Close.Float64()
	if price <= 0 {
		return
	}
	m.handleTradeAlerts(event.Snapshot, price)
	note, err := m.store.GetDecisionNote(event.Snapshot.Code)
	if err != nil {
		return
	}
	if note.StopLoss > 0 && price <= note.StopLoss {
		m.trigger(event.Snapshot.Symbol, price, "stop_loss", note.StopLoss,
			fmt.Sprintf("%s 跌破止损价 %.3f，当前 %.3f", event.Snapshot.Symbol, note.StopLoss, price))
		return
	}
	if note.PlanBuy > 0 && price <= note.PlanBuy {
		m.trigger(event.Snapshot.Symbol, price, "plan_buy", note.PlanBuy,
			fmt.Sprintf("%s 达到计划关注价 %.3f，当前 %.3f", event.Snapshot.Symbol, note.PlanBuy, price))
	}
}

// handleTradeAlerts reads saved positions for every quote. Turning off or deleting
// a position therefore stops delivery even if the subscription still has its code.
func (m *QuoteMonitor) handleTradeAlerts(snapshot *MarketQuoteSnapshot, price float64) {
	m.handleTradeAlertsAt(snapshot, price, time.Now())
}

func (m *QuoteMonitor) handleTradeAlertsAt(snapshot *MarketQuoteSnapshot, price float64, now time.Time) {
	if !freshTradeQuote(snapshot, price, now) {
		return
	}
	state, err := m.store.GetTradingSystemState()
	if err != nil {
		return
	}
	remaining := workbench.RemainingTradeShares(state.Trades)
	code := workbench.TradingCode(snapshot.Code)
	date := now.In(shanghaiLocation()).Format("2006-01-02")
	if m.market != nil && m.market.Enabled() && m.workdayDate != date {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		valid, err := m.market.IsWorkday(ctx, date)
		cancel()
		if err != nil {
			return
		}
		m.workdayDate, m.workdayValid = date, valid
	}
	if m.market != nil && m.market.Enabled() && !m.workdayValid {
		return
	}
	for _, trade := range state.Trades {
		if !tradeAlertActive(trade, remaining[trade.ID], code) {
			continue
		}
		// When a gap reaches both targets, notify the higher target and consume the
		// first target too. A later pullback must not produce a delayed lower alert.
		kind, threshold := tradeAlertThreshold(trade, price)
		if kind == "" {
			continue
		}
		hooks, err := m.store.ResolveWebhooks(mustJSONStrings(trade.AlertWebhookIDs))
		if err != nil {
			continue
		}
		for _, hook := range hooks {
			if !webhookAllowsEvent(hook, "trade.alert") {
				continue
			}
			if !m.tradeAlertStillActive(trade, hook.ID, code, kind, threshold) {
				continue
			}
			claimed, err := m.store.ClaimTradeAlertDelivery(trade.ID, kind, hook.ID, date)
			if err != nil || !claimed {
				continue
			}
			if !m.tradeAlertStillActive(trade, hook.ID, code, kind, threshold) {
				_ = m.store.FinishTradeAlertDelivery(trade.ID, kind, hook.ID, date, "cancelled")
				continue
			}
			// Reload the channel immediately before dispatch so a disabled/deleted
			// webhook cannot be sent using the earlier copy.
			currentHook, err := m.store.GetWebhook(hook.ID)
			if err != nil || !currentHook.Enabled || !webhookAllowsEvent(currentHook, "trade.alert") {
				continue
			}
			if kind == "take_profit_2" {
				lower, claimErr := m.store.ClaimTradeAlertDelivery(trade.ID, "take_profit_1", hook.ID, date)
				if claimErr == nil && lower {
					_ = m.store.FinishTradeAlertDelivery(trade.ID, "take_profit_1", hook.ID, date, "cancelled")
				}
			}
			quoteTime := snapshot.Quote.Kline.Time.In(shanghaiLocation()).Format("2006-01-02 15:04:05")
			event := WebhookEvent{
				Event: "trade.alert", Status: "triggered",
				Message:      fmt.Sprintf("%s（%s）%s预警\n买入价：%.2f\n触发价：%.2f\n最新价：%.2f（%+.2f%%）\n剩余持仓：%.0f 股\n行情时间：%s\n同一价位类型每日最多提醒一次", trade.StockName, code, tradeAlertLabel(kind), trade.EntryPrice, threshold, price, (price/trade.EntryPrice-1)*100, remaining[trade.ID], quoteTime),
				MatchedCount: 1, MatchedSymbols: []string{snapshot.Symbol},
				Result: map[string]interface{}{"trade_id": trade.ID, "symbol": snapshot.Symbol, "kind": kind, "entry_price": trade.EntryPrice, "threshold": threshold, "price": price, "shares": remaining[trade.ID], "alert_date": date, "quote_time": quoteTime},
			}
			deliveryStatus := "sent"
			if err := sendWebhook(context.Background(), currentHook, event); err != nil {
				var rejected webhookRejectedError
				deliveryStatus = "uncertain"
				if errors.As(err, &rejected) {
					deliveryStatus = "failed"
				}
				log.Printf("持仓预警发送未完成 trade=%s webhook=%s status=%s", trade.ID, hook.ID, deliveryStatus)
			}
			if err := m.store.FinishTradeAlertDelivery(trade.ID, kind, hook.ID, date, deliveryStatus); err != nil {
				log.Printf("持仓预警发送状态保存失败: %v", err)
			}
		}
	}
}

func freshTradeQuote(snapshot *MarketQuoteSnapshot, price float64, now time.Time) bool {
	if snapshot == nil || snapshot.Stale || snapshot.Quote == nil || snapshot.Quote.Kline == nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return false
	}
	quoteTime := snapshot.Quote.Kline.Time
	if !snapshot.FetchedAt.IsZero() && (now.Sub(snapshot.FetchedAt) > 3*time.Minute || snapshot.FetchedAt.After(now.Add(5*time.Second))) {
		return false
	}
	age := now.Sub(quoteTime)
	local := now.In(shanghaiLocation())
	minute := local.Hour()*60 + local.Minute()
	// Include the closing auction snapshot, but do not alert on a refreshed
	// timestamp attached to yesterday's close outside the A-share session.
	inSession := (minute >= 9*60+30 && minute <= 11*60+30) || (minute >= 13*60 && minute <= 15*60)
	return !quoteTime.IsZero() && age >= -5*time.Second && age <= 3*time.Minute &&
		quoteTime.In(shanghaiLocation()).Format("2006-01-02") == local.Format("2006-01-02") &&
		local.Weekday() != time.Saturday && local.Weekday() != time.Sunday && inSession
}

func tradeAlertActive(trade TradingTrade, shares float64, code string) bool {
	return trade.Direction != "sell" && trade.Status == "active" && trade.AlertEnabled && shares > 0 &&
		trade.EntryPrice > 0 && workbench.TradingCode(trade.StockCode) == code
}

func tradeAlertThreshold(trade TradingTrade, price float64) (string, float64) {
	if trade.StopPrice > 0 && price <= trade.StopPrice {
		return "stop_loss", trade.StopPrice
	}
	if trade.TargetPrice2 > 0 && price >= trade.TargetPrice2 {
		return "take_profit_2", trade.TargetPrice2
	}
	if trade.TargetPrice1 > 0 && price >= trade.TargetPrice1 {
		return "take_profit_1", trade.TargetPrice1
	}
	return "", 0
}

func (m *QuoteMonitor) tradeAlertStillActive(previous TradingTrade, hookID, code, kind string, threshold float64) bool {
	state, err := m.store.GetTradingSystemState()
	if err != nil {
		return false
	}
	remaining := workbench.RemainingTradeShares(state.Trades)
	for _, trade := range state.Trades {
		if trade.ID != previous.ID || !tradeAlertActive(trade, remaining[trade.ID], code) {
			continue
		}
		currentThreshold := map[string]float64{"stop_loss": trade.StopPrice, "take_profit_1": trade.TargetPrice1, "take_profit_2": trade.TargetPrice2}[kind]
		if currentThreshold != threshold {
			return false
		}
		for _, id := range trade.AlertWebhookIDs {
			if id == hookID {
				return true
			}
		}
	}
	return false
}

func tradeAlertLabel(kind string) string {
	switch kind {
	case "stop_loss":
		return "止损"
	case "take_profit_1":
		return "第一止盈"
	default:
		return "第二止盈"
	}
}

func mustJSONStrings(values []string) string { raw, _ := json.Marshal(values); return string(raw) }

func shanghaiLocation() *time.Location { return time.FixedZone("Asia/Shanghai", 8*60*60) }

func (m *QuoteMonitor) trigger(symbol string, price float64, rule string, threshold float64, message string) {
	key := symbol + ":" + rule
	now := time.Now()
	m.mu.Lock()
	if previous, ok := m.lastTrigger[key]; ok && now.Sub(previous) < 15*time.Minute {
		m.mu.Unlock()
		return
	}
	m.lastTrigger[key] = now
	alert := QuoteAlert{
		ID: fmt.Sprintf("%d", now.UnixNano()), Symbol: symbol, Price: price,
		Rule: rule, Threshold: threshold, Message: message, CreatedAt: now,
	}
	m.alerts = append(m.alerts, alert)
	if len(m.alerts) > 200 {
		m.alerts = m.alerts[len(m.alerts)-200:]
	}
	m.mu.Unlock()

	hooks, _ := m.store.ListEnabledWebhooks()
	logs := sendWebhooks(context.Background(), hooks, WebhookEvent{
		Event: "quote.alert", Status: "triggered", Message: message,
		MatchedCount: 1, MatchedSymbols: []string{symbol}, Result: alert,
	})
	if len(logs) > 0 {
		log.Printf("行情告警通知: %s", strings.Join(logs, "; "))
	}
}

func (m *QuoteMonitor) ListAlerts(limit int) []QuoteAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.alerts) {
		limit = len(m.alerts)
	}
	if limit == 0 {
		return []QuoteAlert{}
	}
	result := make([]QuoteAlert, 0, limit)
	for i := len(m.alerts) - 1; i >= 0 && len(result) < limit; i-- {
		result = append(result, m.alerts[i])
	}
	return result
}

func handleQuoteAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || quoteMonitor == nil {
		errorResponse(w, "只支持GET请求")
		return
	}
	successResponse(w, quoteMonitor.ListAlerts(parsePositiveInt(r.URL.Query().Get("limit"))))
}

var quoteMonitor *QuoteMonitor
