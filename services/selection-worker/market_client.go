package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

type MarketServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

type marketEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type MarketQuoteStreamEvent struct {
	Type     string               `json:"type"`
	Snapshot *MarketQuoteSnapshot `json:"snapshot,omitempty"`
}

type MarketQuoteSnapshot struct {
	Symbol string          `json:"symbol"`
	Code   string          `json:"code"`
	Quote  *protocol.Quote `json:"quote"`
}

type MarketKlineBatch struct {
	Data             map[string]*protocol.KlineResp `json:"data"`
	Errors           map[string]string              `json:"errors"`
	Source           string                         `json:"source"`
	HikyuuCount      int                            `json:"hikyuu_count"`
	TDXFallbackCount int                            `json:"tdx_fallback_count"`
}

type MarketHikyuuTask struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	StartedAt string                 `json:"started_at"`
	EndedAt   string                 `json:"ended_at"`
	ExitCode  *int                   `json:"exit_code"`
	Error     string                 `json:"error"`
	Progress  *int                   `json:"progress,omitempty"`
	Stage     string                 `json:"stage,omitempty"`
	Message   string                 `json:"message,omitempty"`
	LogTail   []string               `json:"log_tail,omitempty"`
	Freshness *MarketHikyuuFreshness `json:"freshness,omitempty"`
}

type MarketHikyuuMarketFreshness struct {
	Total            int            `json:"total"`
	FreshCount       int            `json:"fresh_count"`
	StaleCount       int            `json:"stale_count"`
	MissingCount     int            `json:"missing_count"`
	LatestDate       string         `json:"latest_date"`
	Coverage         float64        `json:"coverage"`
	DateDistribution map[string]int `json:"date_distribution"`
	Errors           []string       `json:"errors"`
	OK               bool           `json:"ok"`
}

type MarketHikyuuFreshness struct {
	ExpectedDate    string                                 `json:"expected_date"`
	MinimumCoverage float64                                `json:"minimum_coverage"`
	OK              bool                                   `json:"ok"`
	Markets         map[string]MarketHikyuuMarketFreshness `json:"markets"`
	CheckedAt       string                                 `json:"checked_at"`
}

func NewMarketServiceClient() *MarketServiceClient {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MARKET_SERVICE_URL")), "/")
	return &MarketServiceClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (c *MarketServiceClient) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *MarketServiceClient) get(ctx context.Context, path string, query url.Values, out interface{}) error {
	if !c.Enabled() {
		return errors.New("MARKET_SERVICE_URL 未配置")
	}
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("market-service %s: %s", resp.Status, string(body))
	}
	var envelope marketEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if envelope.Code != 0 {
		if envelope.Message == "" {
			envelope.Message = "market-service 返回失败"
		}
		return errors.New(envelope.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

func (c *MarketServiceClient) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	if !c.Enabled() {
		return errors.New("MARKET_SERVICE_URL 未配置")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("market-service %s: %s", resp.Status, string(respBody))
	}
	var envelope marketEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return err
	}
	if envelope.Code != 0 {
		if envelope.Message == "" {
			envelope.Message = "market-service 返回失败"
		}
		return errors.New(envelope.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

func (c *MarketServiceClient) KlineHistory(ctx context.Context, code, klineType string, limit int) (*protocol.KlineResp, error) {
	query := url.Values{}
	query.Set("code", code)
	query.Set("type", klineType)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var resp protocol.KlineResp
	if err := c.get(ctx, "/api/kline-history", query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) KlineHistoryBatch(ctx context.Context, symbols []string, klineType string, limit int) (*MarketKlineBatch, error) {
	return c.KlineHistoryBatchWithRecover(ctx, symbols, klineType, limit, "qfq")
}

func (c *MarketServiceClient) KlineHistoryBatchWithRecover(ctx context.Context, symbols []string, klineType string, limit int, recover string) (*MarketKlineBatch, error) {
	var resp MarketKlineBatch
	if err := c.post(ctx, "/api/hikyuu/kline/batch", map[string]interface{}{
		"symbols": symbols,
		"period":  klineType,
		"recover": recover,
		"limit":   limit,
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) StartHikyuuAfterCloseSync(ctx context.Context, expectedDate string) (*MarketHikyuuTask, error) {
	var resp MarketHikyuuTask
	if err := c.post(ctx, "/api/hikyuu/tasks/after-close-sync", map[string]interface{}{
		"expected_date": strings.TrimSpace(expectedDate),
	}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) HikyuuFreshness(ctx context.Context, expectedDate string) (*MarketHikyuuFreshness, error) {
	query := url.Values{}
	query.Set("expected_date", strings.TrimSpace(expectedDate))
	var resp MarketHikyuuFreshness
	if err := c.get(ctx, "/api/hikyuu/freshness", query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) HikyuuTask(ctx context.Context, id string) (*MarketHikyuuTask, error) {
	var resp MarketHikyuuTask
	if err := c.get(ctx, "/api/hikyuu/tasks/"+url.PathEscape(id), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) IsWorkday(ctx context.Context, date string) (bool, error) {
	query := url.Values{}
	if strings.TrimSpace(date) != "" {
		query.Set("date", date)
	}
	var resp struct {
		IsWorkday bool `json:"is_workday"`
	}
	if err := c.get(ctx, "/api/workday", query, &resp); err != nil {
		return false, err
	}
	return resp.IsWorkday, nil
}

func (c *MarketServiceClient) ExpectedTradingDate(ctx context.Context, date string) (string, error) {
	query := url.Values{}
	query.Set("date", strings.TrimSpace(date))
	query.Set("count", "1")
	var resp struct {
		Date struct {
			ISO string `json:"iso"`
		} `json:"date"`
		IsWorkday bool `json:"is_workday"`
		Previous  []struct {
			ISO string `json:"iso"`
		} `json:"previous"`
	}
	if err := c.get(ctx, "/api/workday", query, &resp); err != nil {
		return "", err
	}
	if resp.IsWorkday && strings.TrimSpace(resp.Date.ISO) != "" {
		return resp.Date.ISO, nil
	}
	if len(resp.Previous) == 0 || strings.TrimSpace(resp.Previous[0].ISO) == "" {
		return "", errors.New("无法确定最近交易日")
	}
	return resp.Previous[0].ISO, nil
}

// Quotes 批量获取实时行情快照(单批建议不超过50只)。
// 返回 tdx Quote 列表,匹配键为 Quote.Code(纯6位代码);停牌股可能不返回或无今日K线。
func (c *MarketServiceClient) Quotes(ctx context.Context, codes []string) ([]*protocol.Quote, error) {
	query := url.Values{}
	query.Set("code", strings.Join(codes, ","))
	var resp []*protocol.Quote
	if err := c.get(ctx, "/api/quote", query, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *MarketServiceClient) IndexKline(ctx context.Context, code, klineType string, limit int) (*protocol.KlineResp, error) {
	query := url.Values{}
	query.Set("code", code)
	query.Set("type", klineType)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var resp protocol.KlineResp
	if err := c.get(ctx, "/api/index", query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *MarketServiceClient) Codes(ctx context.Context) ([]*tdx.CodeModel, error) {
	var resp struct {
		Codes []struct {
			Code     string `json:"code"`
			Name     string `json:"name"`
			Exchange string `json:"exchange"`
		} `json:"codes"`
	}
	if err := c.get(ctx, "/api/codes", nil, &resp); err != nil {
		return nil, err
	}
	models := make([]*tdx.CodeModel, 0, len(resp.Codes))
	for _, item := range resp.Codes {
		models = append(models, &tdx.CodeModel{
			Name:     item.Name,
			Code:     item.Code,
			Exchange: item.Exchange,
		})
	}
	return models, nil
}

func (c *MarketServiceClient) StockCodes(ctx context.Context, maxCodes int) ([]string, error) {
	query := url.Values{}
	query.Set("prefix", "false")
	if maxCodes > 0 {
		query.Set("limit", strconv.Itoa(maxCodes))
	}
	var resp struct {
		List []string `json:"list"`
	}
	if err := c.get(ctx, "/api/stock-codes", query, &resp); err != nil {
		return nil, err
	}
	return resp.List, nil
}

func (c *MarketServiceClient) RawData(ctx context.Context, path string, query url.Values) (interface{}, error) {
	var raw interface{}
	if err := c.get(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *MarketServiceClient) SubscribeQuotes(ctx context.Context, codes []string, onEvent func(MarketQuoteStreamEvent)) error {
	if !c.Enabled() {
		return errors.New("MARKET_SERVICE_URL 未配置")
	}
	query := url.Values{}
	query.Set("codes", strings.Join(codes, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/stream/quotes?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("market-service SSE %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	eventType := ""
	data := ""
	emit := func() {
		if data == "" {
			return
		}
		var event MarketQuoteStreamEvent
		if json.Unmarshal([]byte(data), &event) == nil {
			if event.Type == "" {
				event.Type = eventType
			}
			onEvent(event)
		}
		eventType = ""
		data = ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data != "" {
				data += "\n"
			}
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			emit()
		}
	}
	emit()
	return scanner.Err()
}

func (c *MarketServiceClient) PullKlineTask(ctx context.Context, payload SystemSyncPayload, tables []string, codes []string) (map[string]interface{}, error) {
	req := map[string]interface{}{
		"codes":      codes,
		"tables":     tables,
		"limit":      payload.Limit,
		"start_date": payload.StartDate,
	}
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/tasks/pull-kline", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *MarketServiceClient) SyncGbbqTask(ctx context.Context) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.post(ctx, "/api/tasks/sync-gbbq", map[string]interface{}{}, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
