package main

import (
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

type marketServiceEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type marketServiceCodesEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Codes []struct {
			Code     string `json:"code"`
			Name     string `json:"name"`
			Exchange string `json:"exchange"`
		} `json:"codes"`
	} `json:"data"`
}

func marketServiceBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("MARKET_SERVICE_URL")), "/")
}

func useMarketService() bool {
	return marketServiceBaseURL() != ""
}

func allowDirectMarketFallback() bool {
	return !useMarketService() || envBool("WEB_MARKET_FALLBACK_DIRECT", envBool("SELECTION_MARKET_FALLBACK_DIRECT", false))
}

func ensureLocalMarketRuntime() error {
	if !allowDirectMarketFallback() {
		return errors.New("web 已配置 MARKET_SERVICE_URL，未启用本地行情 fallback")
	}
	return initMarketRuntime(false, false)
}

func marketServiceGet(ctx context.Context, path string, query url.Values, out interface{}) error {
	baseURL := marketServiceBaseURL()
	if baseURL == "" {
		return errors.New("MARKET_SERVICE_URL 未配置")
	}
	endpoint := baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("market-service %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var envelope marketServiceEnvelope
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

func marketServicePost(ctx context.Context, path string, payload interface{}, out interface{}) error {
	baseURL := marketServiceBaseURL()
	if baseURL == "" {
		return errors.New("MARKET_SERVICE_URL 未配置")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("market-service %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var envelope marketServiceEnvelope
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

func marketServiceCodeModels(ctx context.Context) ([]*tdx.CodeModel, error) {
	var data struct {
		Codes []struct {
			Code              string  `json:"code"`
			Name              string  `json:"name"`
			Exchange          string  `json:"exchange"`
			CodeType          uint16  `json:"code_type,string"`
			CodeTypeHex       string  `json:"code_type_hex"`
			CodeTypeSource    string  `json:"code_type_source"`
			CodeTypeUpdatedAt int64   `json:"code_type_updated_at,string"`
			PrevClose         float64 `json:"prev_close,string"`
			FiveDayVol        int64   `json:"five_day_vol,string"`
		} `json:"codes"`
	}
	if err := marketServiceGet(ctx, "/api/codes", nil, &data); err != nil {
		return nil, err
	}

	models := make([]*tdx.CodeModel, 0, len(data.Codes))
	for _, item := range data.Codes {
		model := &tdx.CodeModel{
			Name:              item.Name,
			Code:              item.Code,
			Exchange:          strings.ToLower(strings.TrimSpace(item.Exchange)),
			CodeType:          item.CodeType,
			CodeTypeHex:       item.CodeTypeHex,
			CodeTypeSource:    item.CodeTypeSource,
			CodeTypeUpdatedAt: item.CodeTypeUpdatedAt,
			PrevClose:         item.PrevClose,
			FiveDayVol:        item.FiveDayVol,
		}
		if model.CodeType == 0 {
			model.FillCodeTypeByRule()
		}
		models = append(models, model)
	}
	return models, nil
}

func marketServiceKlineHistory(ctx context.Context, code, klineType string, limit int) (*protocol.KlineResp, error) {
	query := url.Values{}
	query.Set("code", code)
	query.Set("type", klineType)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var resp protocol.KlineResp
	if err := marketServiceGet(ctx, "/api/kline-history", query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func marketServiceIndexKline(ctx context.Context, code, klineType string, limit int) (*protocol.KlineResp, error) {
	query := url.Values{}
	query.Set("code", code)
	query.Set("type", klineType)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var resp protocol.KlineResp
	if err := marketServiceGet(ctx, "/api/index", query, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
