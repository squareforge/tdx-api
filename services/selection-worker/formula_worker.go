package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx/protocol"
)

type FormulaWorkerClient struct {
	baseURL    string
	httpClient *http.Client
}

type FormulaRunRequest struct {
	Symbol        string                    `json:"symbol,omitempty"`
	Symbols       []string                  `json:"symbols,omitempty"`
	Script        string                    `json:"script"`
	Args          json.RawMessage           `json:"args,omitempty"`
	Period        string                    `json:"period"`
	Right         int                       `json:"right"`
	OutCount      int                       `json:"out_count"`
	CalcCount     int                       `json:"calc_count"`
	ForceFallback bool                      `json:"force_fallback,omitempty"`
	Data          map[string][]FormulaKline `json:"data,omitempty"`
}

type FormulaRunResponse struct {
	Code                int         `json:"code"`
	Message             string      `json:"message"`
	Data                interface{} `json:"data"`
	Engine              string      `json:"engine"`
	HQChartPy2Available bool        `json:"hqchartpy2_available"`
	FallbackError       string      `json:"fallback_error,omitempty"`
	TickMS              int64       `json:"tick_ms"`
}

type FormulaKline struct {
	Date           int     `json:"date"`
	Time           int     `json:"time,omitempty"`
	YClose         float64 `json:"yclose"`
	Open           float64 `json:"open"`
	High           float64 `json:"high"`
	Low            float64 `json:"low"`
	Close          float64 `json:"close"`
	Vol            float64 `json:"vol"`
	Amount         float64 `json:"amount"`
	SnapshotAt     string  `json:"snapshot_at,omitempty"`
	SnapshotSource string  `json:"snapshot_source,omitempty"`
}

func NewFormulaWorkerClient() *FormulaWorkerClient {
	baseURL := strings.TrimRight(os.Getenv("FORMULA_WORKER_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8712"
	}
	return &FormulaWorkerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *FormulaWorkerClient) Health(ctx context.Context) error {
	_, err := c.HealthInfo(ctx)
	return err
}

func (c *FormulaWorkerClient) HealthInfo(ctx context.Context) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	info := map[string]interface{}{}
	_ = json.NewDecoder(resp.Body).Decode(&info)
	if resp.StatusCode >= 300 {
		return info, fmt.Errorf("formula worker health status: %s", resp.Status)
	}
	return info, nil
}

func (c *FormulaWorkerClient) Run(ctx context.Context, reqData FormulaRunRequest) (FormulaRunResponse, error) {
	var respData FormulaRunResponse
	if len(reqData.Data) == 0 {
		data, err := buildFormulaData(ctx, reqData.Symbols, reqData.Symbol, reqData.Period, reqData.CalcCount)
		if err != nil {
			return respData, err
		}
		reqData.Data = data
	}
	if reqData.Args == nil {
		reqData.Args = json.RawMessage("[]")
	}
	if reqData.OutCount == 0 {
		reqData.OutCount = 1
	}
	if reqData.CalcCount == 0 {
		reqData.CalcCount = 240
	}

	raw, err := json.Marshal(reqData)
	if err != nil {
		return respData, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return respData, ctx.Err()
			case <-timer.C:
			}
		}
		respData = FormulaRunResponse{}
		httpReq, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/formula/run", bytes.NewReader(raw))
		if requestErr != nil {
			return respData, requestErr
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, requestErr := c.httpClient.Do(httpReq)
		if requestErr != nil {
			if ctx.Err() != nil {
				return respData, ctx.Err()
			}
			lastErr = requestErr
			continue
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&respData)
		_ = resp.Body.Close()
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		if resp.StatusCode >= 300 || respData.Code != 0 {
			if respData.Message == "" {
				respData.Message = resp.Status
			}
			lastErr = errors.New(respData.Message)
			if resp.StatusCode < 500 {
				return respData, lastErr
			}
			continue
		}
		return respData, nil
	}
	return respData, fmt.Errorf("formula worker请求失败，已重试2次: %w", lastErr)
}

func buildFormulaData(ctx context.Context, symbols []string, symbol, period string, calcCount int) (map[string][]FormulaKline, error) {
	if len(symbols) == 0 && symbol != "" {
		symbols = []string{symbol}
	}
	symbols = normalizeSymbols(symbols)
	if len(symbols) == 0 {
		return nil, errors.New("股票代码不能为空")
	}
	if calcCount <= 0 {
		calcCount = 240
	}

	result := make(map[string][]FormulaKline, len(symbols))
	for _, s := range symbols {
		resp, err := loadFormulaKline(ctx, s, period, calcCount)
		if err != nil {
			return nil, fmt.Errorf("%s K线加载失败: %w", s, err)
		}
		result[s] = resp
	}
	return result, nil
}

func loadFormulaKline(ctx context.Context, symbol, period string, calcCount int) ([]FormulaKline, error) {
	klineType := formulaPeriodToKlineType(period)
	if useMarketService() {
		resp, err := marketClient.KlineHistory(ctx, symbol, klineType, calcCount)
		if err == nil {
			if resp == nil || len(resp.List) == 0 {
				return nil, errors.New("K线为空")
			}
			return protocolKlinesToFormulaRows(resp.List, calcCount), nil
		}
		if !allowDirectMarketFallback() {
			return nil, err
		}
	}
	if err := ensureLocalMarketRuntime(); err != nil {
		return nil, err
	}

	var resp *protocol.KlineResp
	var err error

	switch klineType {
	case "day", "week", "month":
		resp, err = getQfqKlineDay(symbol)
		if err == nil && klineType == "week" {
			resp = convertToWeekKline(resp)
		}
		if err == nil && klineType == "month" {
			resp = convertToMonthKline(resp)
		}
	case "minute1":
		resp, err = client.GetKlineMinute(symbol, 0, uint16(min(calcCount, 800)))
	case "minute5":
		resp, err = client.GetKline5Minute(symbol, 0, uint16(min(calcCount, 800)))
	case "minute15":
		resp, err = client.GetKline15Minute(symbol, 0, uint16(min(calcCount, 800)))
	case "minute30":
		resp, err = client.GetKline30Minute(symbol, 0, uint16(min(calcCount, 800)))
	case "hour":
		resp, err = client.GetKlineHour(symbol, 0, uint16(min(calcCount, 800)))
	default:
		resp, err = getQfqKlineDay(symbol)
	}
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.List) == 0 {
		return nil, errors.New("K线为空")
	}

	list := resp.List
	if len(list) > calcCount {
		list = list[len(list)-calcCount:]
	}
	out := make([]FormulaKline, 0, len(list))
	var prevClose float64
	for i, item := range list {
		closePrice := priceToYuan(item.Close)
		yClose := priceToYuan(item.Last)
		if yClose == 0 && i > 0 {
			yClose = prevClose
		}
		out = append(out, FormulaKline{
			Date:   dateInt(item.Time),
			Time:   timeInt(item.Time),
			YClose: yClose,
			Open:   priceToYuan(item.Open),
			High:   priceToYuan(item.High),
			Low:    priceToYuan(item.Low),
			Close:  closePrice,
			Vol:    float64(item.Volume),
			Amount: amountToYuan(item.Amount, closePrice, float64(item.Volume)),
		})
		prevClose = closePrice
	}
	return out, nil
}

func loadTrackingKline(ctx context.Context, symbol string, calcCount int) ([]FormulaKline, error) {
	return loadFormulaKline(ctx, symbol, "day", calcCount)
}

func formulaPeriodToKlineType(period string) string {
	p := strings.ToLower(strings.TrimSpace(period))
	switch p {
	case "0", "day", "d", "":
		return "day"
	case "1", "week", "w":
		return "week"
	case "2", "month", "m":
		return "month"
	case "4", "minute1", "1m", "min1":
		return "minute1"
	case "5", "minute5", "5m", "min5":
		return "minute5"
	case "6", "minute15", "15m", "min15":
		return "minute15"
	case "7", "minute30", "30m", "min30":
		return "minute30"
	case "8", "hour", "60m", "min60":
		return "hour"
	default:
		return p
	}
}

func periodID(period string) int {
	switch formulaPeriodToKlineType(period) {
	case "week":
		return 1
	case "month":
		return 2
	case "minute1":
		return 4
	case "minute5":
		return 5
	case "minute15":
		return 6
	case "minute30":
		return 7
	case "hour":
		return 8
	default:
		return 0
	}
}

func priceToYuan(v interface{}) float64 {
	switch x := v.(type) {
	case protocol.Price:
		return x.Float64()
	case int:
		return float64(x) / 1000
	case uint32:
		return float64(x) / 1000
	case int64:
		return float64(x) / 1000
	case float64:
		if x > 1000 {
			return x / 1000
		}
		return x
	case float32:
		if x > 1000 {
			return float64(x) / 1000
		}
		return float64(x)
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		if f > 1000 {
			return f / 1000
		}
		return f
	}
}

func amountToYuan(raw interface{}, closePrice float64, volume float64) float64 {
	amount := amountFloat(raw)
	if amount > 0 {
		if closePrice > 0 && volume > 0 {
			estimated := closePrice * volume * 100
			if estimated > 0 && amount/estimated > 100 {
				return amount / 1000
			}
		}
		return amount
	}
	if closePrice > 0 && volume > 0 {
		return closePrice * volume * 100
	}
	return 0
}

func amountFloat(v interface{}) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case uint32:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case float64:
		return x
	case float32:
		return float64(x)
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return f
	}
}

func dateInt(t time.Time) int {
	v, _ := strconv.Atoi(t.Format("20060102"))
	return v
}

func timeInt(t time.Time) int {
	v, _ := strconv.Atoi(t.Format("150405"))
	return v
}
