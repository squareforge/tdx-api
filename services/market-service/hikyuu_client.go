package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx/protocol"
)

type HikyuuDataServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

type hikyuuEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type hikyuuKlineResponse struct {
	Symbol  string                 `json:"symbol"`
	Period  string                 `json:"period"`
	Recover string                 `json:"recover"`
	Count   int                    `json:"count"`
	List    []hikyuuKlineItem      `json:"list"`
	Meta    map[string]interface{} `json:"meta"`
}

type hikyuuKlineBatchResponse struct {
	Period  string                         `json:"period"`
	Recover string                         `json:"recover"`
	Data    map[string]hikyuuKlineResponse `json:"data"`
	Errors  map[string]string              `json:"errors"`
	Meta    map[string]interface{}         `json:"meta"`
}

type HikyuuTask struct {
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
	Request   map[string]interface{} `json:"request,omitempty"`
	LogTail   []string               `json:"log_tail,omitempty"`
	Freshness *HikyuuFreshness       `json:"freshness,omitempty"`
}

type HikyuuMarketFreshness struct {
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

type HikyuuFreshness struct {
	ExpectedDate    string                           `json:"expected_date"`
	MinimumCoverage float64                          `json:"minimum_coverage"`
	OK              bool                             `json:"ok"`
	Markets         map[string]HikyuuMarketFreshness `json:"markets"`
	CheckedAt       string                           `json:"checked_at"`
}

type hikyuuKlineItem struct {
	Time   string  `json:"time"`
	Last   float64 `json:"last"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
	Amount float64 `json:"amount"`
}

func NewHikyuuDataServiceClient() *HikyuuDataServiceClient {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HIKYUU_DATA_SERVICE_URL")), "/")
	return &HikyuuDataServiceClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 180 * time.Second,
		},
	}
}

func (c *HikyuuDataServiceClient) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *HikyuuDataServiceClient) queryKline(ctx context.Context, code, klineType, recover string, limit int) (*hikyuuKlineResponse, error) {
	if !c.Enabled() {
		return nil, errors.New("HIKYUU_DATA_SERVICE_URL 未配置")
	}
	query := url.Values{}
	query.Set("code", code)
	query.Set("type", klineType)
	query.Set("recover", recover)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	endpoint := c.baseURL + "/api/hikyuu/kline?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope hikyuuEnvelope
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hikyuu-data-service %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 {
		if envelope.Message == "" {
			envelope.Message = "hikyuu-data-service 返回失败"
		}
		return nil, errors.New(envelope.Message)
	}
	if len(envelope.Data) == 0 {
		return nil, errors.New("hikyuu-data-service 返回空数据")
	}
	var result hikyuuKlineResponse
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HikyuuDataServiceClient) FetchKline(ctx context.Context, code, klineType, recover string, limit int) (*protocol.KlineResp, error) {
	symbol, err := NormalizeSymbol(code)
	if err != nil {
		return nil, err
	}
	code = symbol.Symbol

	resp, err := c.queryKline(ctx, code, klineType, recover, limit)
	if err != nil {
		return nil, err
	}
	return hikyuuKlineToProtocol(resp, klineType)
}

func hikyuuKlineToProtocol(resp *hikyuuKlineResponse, klineType string) (*protocol.KlineResp, error) {
	list := make([]*protocol.Kline, 0, len(resp.List))
	for _, item := range resp.List {
		t, err := time.Parse(time.RFC3339, item.Time)
		if err != nil {
			return nil, err
		}
		list = append(list, &protocol.Kline{
			Time:   t,
			Last:   protocol.Yuan(item.Last),
			Open:   protocol.Yuan(item.Open),
			High:   protocol.Yuan(item.High),
			Low:    protocol.Yuan(item.Low),
			Close:  protocol.Yuan(item.Close),
			Volume: hikyuuVolumeToTDX(klineType, item.Volume),
			Amount: protocol.Yuan(item.Amount * 10000),
		})
	}
	count := len(list)
	if count > int(^uint16(0)) {
		count = int(^uint16(0))
	}
	return &protocol.KlineResp{
		Count: uint16(count),
		List:  list,
	}, nil
}

func (c *HikyuuDataServiceClient) FetchKlineBatch(ctx context.Context, symbols []string, klineType, recover string, limit int) (map[string]*protocol.KlineResp, map[string]string, error) {
	if !c.Enabled() {
		return nil, nil, errors.New("HIKYUU_DATA_SERVICE_URL 未配置")
	}
	raw, err := json.Marshal(map[string]interface{}{
		"symbols": symbols,
		"period":  klineType,
		"recover": recover,
		"limit":   limit,
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/hikyuu/kline/batch", bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var envelope hikyuuEnvelope
	if err := c.do(req, &envelope); err != nil {
		return nil, nil, err
	}
	if envelope.Code != 0 {
		return nil, nil, errors.New(envelope.Message)
	}
	var batch hikyuuKlineBatchResponse
	if err := json.Unmarshal(envelope.Data, &batch); err != nil {
		return nil, nil, err
	}
	if batch.Errors == nil {
		batch.Errors = map[string]string{}
	}
	result := make(map[string]*protocol.KlineResp, len(batch.Data))
	for symbol, item := range batch.Data {
		converted, convertErr := hikyuuKlineToProtocol(&item, klineType)
		if convertErr != nil {
			batch.Errors[symbol] = convertErr.Error()
			continue
		}
		result[symbol] = converted
	}
	return result, batch.Errors, nil
}

func (c *HikyuuDataServiceClient) StartAfterCloseSync(ctx context.Context, expectedDate string) (*HikyuuTask, error) {
	payload := map[string]interface{}{
		"day": true, "min": false, "min5": false, "trans": false, "time": false,
		"stock": true, "fund": true, "weight": true, "finance": false, "block": false,
		"use_tdx_number": 10,
	}
	if expectedDate = strings.TrimSpace(expectedDate); expectedDate != "" {
		payload["expected_date"] = expectedDate
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/hikyuu/tasks/after-close-sync", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.readTask(req)
}

func (c *HikyuuDataServiceClient) GetFreshness(ctx context.Context, expectedDate string) (*HikyuuFreshness, error) {
	if !c.Enabled() {
		return nil, errors.New("HIKYUU_DATA_SERVICE_URL 未配置")
	}
	query := url.Values{}
	query.Set("expected_date", strings.TrimSpace(expectedDate))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/hikyuu/freshness?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var envelope hikyuuEnvelope
	if err := c.do(req, &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 {
		return nil, errors.New(envelope.Message)
	}
	var result HikyuuFreshness
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HikyuuDataServiceClient) GetTask(ctx context.Context, id string) (*HikyuuTask, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/hikyuu/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	return c.readTask(req)
}

func (c *HikyuuDataServiceClient) readTask(req *http.Request) (*HikyuuTask, error) {
	if !c.Enabled() {
		return nil, errors.New("HIKYUU_DATA_SERVICE_URL 未配置")
	}
	var envelope hikyuuEnvelope
	if err := c.do(req, &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 {
		return nil, errors.New(envelope.Message)
	}
	var task HikyuuTask
	if err := json.Unmarshal(envelope.Data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (c *HikyuuDataServiceClient) do(req *http.Request, out interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("hikyuu-data-service %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}

func hikyuuVolumeToTDX(klineType string, volume float64) int64 {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(klineType)), "minute") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(klineType)), "min") {
		return int64(math.Round(volume / 100))
	}
	return int64(math.Round(volume))
}

func fetchHikyuuKline(ctx context.Context, code, klineType string, limit int) (*protocol.KlineResp, error) {
	if hikyuuClient == nil {
		return nil, errors.New("hikyuu 数据服务未初始化")
	}

	period := strings.ToLower(strings.TrimSpace(klineType))
	if period == "" {
		period = "day"
	}
	switch period {
	case "day", "week", "month", "minute1", "minute5":
	default:
		return nil, fmt.Errorf("hikyuu 不支持K线类型: %s", klineType)
	}

	recover := "none"
	switch period {
	case "day", "week", "month":
		recover = "qfq"
	}
	return hikyuuClient.FetchKline(ctx, code, period, recover, limit)
}

func fetchQuotesWithFallback(ctx context.Context, symbols []Symbol) (protocol.QuotesResp, error) {
	if len(symbols) == 0 {
		return protocol.QuotesResp{}, nil
	}
	var tdxErr error
	if client != nil {
		codes := make([]string, 0, len(symbols))
		for _, symbol := range symbols {
			codes = append(codes, symbol.TDXCode)
		}
		quotes, err := client.GetQuote(codes...)
		if err == nil && len(quotes) == len(symbols) {
			return quotes, nil
		}
		tdxErr = err
		if tdxErr == nil {
			tdxErr = fmt.Errorf("通达信行情返回不完整")
		}
	} else {
		tdxErr = errors.New("通达信行情客户端未初始化")
	}

	if hikyuuClient == nil || !hikyuuClient.Enabled() {
		return nil, tdxErr
	}
	canonical := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		canonical = append(canonical, symbol.Symbol)
	}
	data, failures, err := hikyuuClient.FetchKlineBatch(ctx, canonical, "day", "none", 2)
	if err != nil {
		return nil, fmt.Errorf("通达信: %v; 本地Hikyuu: %w", tdxErr, err)
	}
	quotes := make(protocol.QuotesResp, 0, len(symbols))
	missing := make([]string, 0)
	for _, symbol := range symbols {
		resp := data[symbol.Symbol]
		if resp == nil || len(resp.List) == 0 {
			message := failures[symbol.Symbol]
			if message == "" {
				message = "无日线数据"
			}
			missing = append(missing, symbol.Symbol+": "+message)
			continue
		}
		latest := *resp.List[len(resp.List)-1]
		if latest.Last <= 0 && len(resp.List) > 1 {
			latest.Last = resp.List[len(resp.List)-2].Close
		}
		if latest.Last <= 0 {
			latest.Last = latest.Open
		}
		quotes = append(quotes, &protocol.Quote{
			Exchange:   exchangeForSymbol(symbol),
			Code:       symbol.Code,
			Kline:      &latest,
			ServerTime: latest.Time.Format(time.RFC3339),
			Rate:       latest.RiseRate(),
		})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("本地Hikyuu收盘快照缺失: %s", strings.Join(missing, "; "))
	}
	return quotes, nil
}

func localMarketFallbackError(prefix string, localErr error) string {
	if localErr != nil {
		return fmt.Sprintf("%s: 本地Hikyuu: %v; 通达信行情客户端未初始化", prefix, localErr)
	}
	return prefix + ": 通达信行情客户端未初始化"
}
