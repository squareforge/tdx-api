package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
)

const (
	marketInfoResearchTTL         = 5 * time.Minute
	marketInfoNoticeTTL           = time.Minute
	marketInfoHotMoneyTTL         = 10 * time.Second
	marketInfoIndustryResearchTTL = 10 * time.Minute
	marketInfoIndustryDictTTL     = 24 * time.Hour
)

type MarketResearchReport struct {
	Title        string      `json:"title"`
	StockName    string      `json:"stockName"`
	StockCode    string      `json:"stockCode"`
	OrgSName     string      `json:"orgSName"`
	PublishDate  string      `json:"publishDate"`
	InfoCode     string      `json:"infoCode"`
	IndvInduName string      `json:"indvInduName"`
	EmRatingName string      `json:"emRatingName"`
	RatingChange flexibleInt `json:"ratingChange"`
	SRatingName  string      `json:"sRatingName"`
	Researcher   string      `json:"researcher"`
	Market       string      `json:"market"`
}

type flexibleInt int

func (value *flexibleInt) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*value = flexibleInt(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		if text == "" {
			*value = 0
			return nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return err
		}
		*value = flexibleInt(parsed)
		return nil
	}
	*value = 0
	return nil
}

type IndustryResearchReport struct {
	Title          string      `json:"title"`
	StockName      string      `json:"stockName"`
	StockCode      string      `json:"stockCode"`
	OrgCode        string      `json:"orgCode"`
	OrgName        string      `json:"orgName"`
	OrgSName       string      `json:"orgSName"`
	PublishDate    string      `json:"publishDate"`
	InfoCode       string      `json:"infoCode"`
	IndustryCode   string      `json:"industryCode"`
	IndustryName   string      `json:"industryName"`
	EmIndustryCode string      `json:"emIndustryCode"`
	EmRatingName   string      `json:"emRatingName"`
	RatingChange   flexibleInt `json:"ratingChange"`
	SRatingName    string      `json:"sRatingName"`
	Researcher     string      `json:"researcher"`
	ReportType     int         `json:"reportType"`
	EncodeURL      string      `json:"encodeUrl"`
}

type IndustryDictItem struct {
	BKCode      string `json:"bkCode"`
	FuBKCode    string `json:"fubkCode"`
	BKName      string `json:"bkName"`
	PublishCode string `json:"publishCode"`
	FirstLetter string `json:"firstLetter"`
}

type IndustryResearchResponse struct {
	Items        []IndustryResearchReport `json:"items"`
	IndustryCode string                   `json:"industry_code"`
	Days         int                      `json:"days"`
	Limit        int                      `json:"limit"`
	Source       string                   `json:"source"`
	FetchedAt    string                   `json:"fetched_at"`
}

type IndustryDictResponse struct {
	Items     []IndustryDictItem `json:"items"`
	Source    string             `json:"source"`
	FetchedAt string             `json:"fetched_at"`
}

type MarketNotice struct {
	ArtCode     string `json:"art_code"`
	StockCode   string `json:"stock_code"`
	StockName   string `json:"stock_name"`
	Title       string `json:"title"`
	ColumnName  string `json:"column_name"`
	NoticeDate  string `json:"notice_date"`
	DisplayTime string `json:"display_time"`
}

type HotMoneyTrade struct {
	TradeDate    string   `json:"TRADE_DATE"`
	Explanation  string   `json:"EXPLANATION"`
	OperateDept  string   `json:"OPERATEDEPT_NAME"`
	BuyAmount    *float64 `json:"BUY_AMT_REAL"`
	BuyRatio     *float64 `json:"BUY_RATIO"`
	SellAmount   *float64 `json:"SELL_AMT_REAL"`
	SellRatio    *float64 `json:"SELL_RATIO"`
	SecurityCode string   `json:"SECURITY_CODE"`
	SecurityName string   `json:"SECURITY_NAME_ABBR"`
	Secucode     string   `json:"SECUCODE"`
}

type marketInfoCacheEntry struct {
	value   any
	expires time.Time
}

var marketInfoCache = struct {
	sync.Mutex
	items map[string]marketInfoCacheEntry
}{items: make(map[string]marketInfoCacheEntry)}

const (
	marketInfoKindResearch         = "research"
	marketInfoKindNotice           = "notice"
	marketInfoKindHotMoney         = "hot_money"
	marketInfoKindIndustryResearch = "industry-research"
)

type MarketInfoSyncRequest struct {
	Date            string   `json:"date"`
	Codes           []string `json:"codes"`
	MaxCodes        int      `json:"max_codes"`
	Days            int      `json:"days"`
	Limit           int      `json:"limit"`
	Kinds           []string `json:"kinds"`
	ContinueOnError bool     `json:"continue_on_error"`
}

type MarketInfoSyncResult struct {
	TradeDate string            `json:"trade_date"`
	Kinds     []string          `json:"kinds"`
	CodeCount int               `json:"code_count"`
	Success   int               `json:"success"`
	Failures  map[string]string `json:"failures,omitempty"`
}

var marketInfoSyncMu sync.Mutex
var marketInfoLastRequest struct {
	sync.Mutex
	at time.Time
}

func handleMarketResearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}
	value, err := getMarketInfoCached[[]MarketResearchReport](marketInfoKindResearch, code, marketInfoResearchTTL, func() ([]MarketResearchReport, error) {
		return fetchMarketResearch(r.Context(), code)
	})
	if err != nil {
		errorResponse(w, "获取个股研报失败: "+err.Error())
		return
	}
	successResponse(w, value)
}

func handleMarketNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}
	value, err := getMarketInfoCached[[]MarketNotice](marketInfoKindNotice, code, marketInfoNoticeTTL, func() ([]MarketNotice, error) {
		return fetchMarketNotice(r.Context(), code)
	})
	if err != nil {
		errorResponse(w, "获取公司公告失败: "+err.Error())
		return
	}
	successResponse(w, value)
}

func handleMarketHotMoney(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}
	value, err := getMarketInfoCached[[]HotMoneyTrade](marketInfoKindHotMoney, code, marketInfoHotMoneyTTL, func() ([]HotMoneyTrade, error) {
		return fetchHotMoney(r.Context(), code)
	})
	if err != nil {
		errorResponse(w, "获取游资动向失败: "+err.Error())
		return
	}
	successResponse(w, value)
}

func handleIndustryResearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	industryCode := strings.TrimSpace(r.URL.Query().Get("industry_code"))
	days := parsePositiveInt(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 7
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	dataKey := fmt.Sprintf("%s:%d:500", industryCode, days)
	value, source, fetchedAt, err := getMarketRankCached(
		r.Context(),
		marketInfoKindIndustryResearch,
		dataKey,
		marketInfoIndustryResearchTTL,
		func(ctx context.Context) ([]IndustryResearchReport, error) {
			return fetchIndustryResearch(ctx, industryCode, days, 500, time.Now())
		},
	)
	if err != nil {
		errorResponse(w, "获取行业研究失败: "+err.Error())
		return
	}
	if len(value) > limit {
		value = value[:limit]
	}
	successResponse(w, IndustryResearchResponse{
		Items:        value,
		IndustryCode: industryCode,
		Days:         days,
		Limit:        limit,
		Source:       source,
		FetchedAt:    fetchedAt,
	})
}

func handleIndustryDict(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	value, source, fetchedAt, err := getMarketRankCached(
		r.Context(),
		"industry-dict",
		"016",
		marketInfoIndustryDictTTL,
		func(ctx context.Context) ([]IndustryDictItem, error) {
			return fetchIndustryDict(ctx)
		},
	)
	if err != nil {
		errorResponse(w, "获取行业字典失败: "+err.Error())
		return
	}
	successResponse(w, IndustryDictResponse{
		Items:     value,
		Source:    source,
		FetchedAt: fetchedAt,
	})
}

func handleMarketInfoSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var request MarketInfoSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	result, err := syncMarketInfo(r.Context(), request)
	if err != nil {
		errorResponse(w, "市场信息同步失败: "+err.Error())
		return
	}
	successResponse(w, result)
}

func syncMarketInfo(ctx context.Context, request MarketInfoSyncRequest) (MarketInfoSyncResult, error) {
	marketInfoSyncMu.Lock()
	defer marketInfoSyncMu.Unlock()

	tradeDate := strings.TrimSpace(request.Date)
	if tradeDate == "" {
		tradeDate = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", tradeDate); err != nil {
		return MarketInfoSyncResult{}, errors.New("date 必须是 YYYY-MM-DD 格式")
	}

	kinds := normalizeMarketInfoKinds(request.Kinds)
	if len(kinds) == 0 {
		return MarketInfoSyncResult{}, errors.New("kinds 没有可同步的市场信息类型")
	}
	codes := normalizeMarketInfoSyncCodes(request.Codes, request.MaxCodes)
	result := MarketInfoSyncResult{
		TradeDate: tradeDate,
		Kinds:     kinds,
		CodeCount: len(codes),
		Failures:  map[string]string{},
	}

	if containsString(kinds, marketInfoKindIndustryResearch) {
		days := request.Days
		if days <= 0 {
			days = 7
		}
		items, err := fetchIndustryResearch(ctx, "", days, 500, time.Now())
		if err != nil {
			result.Failures[marketInfoKindIndustryResearch] = err.Error()
			if !request.ContinueOnError {
				return result, err
			}
		} else if marketStore == nil {
			result.Failures[marketInfoKindIndustryResearch] = "行情数据库未初始化"
			if !request.ContinueOnError {
				return result, errors.New("行情数据库未初始化")
			}
		} else {
			raw, marshalErr := json.Marshal(items)
			if marshalErr != nil {
				result.Failures[marketInfoKindIndustryResearch] = marshalErr.Error()
				if !request.ContinueOnError {
					return result, marshalErr
				}
			} else if err := marketStore.saveMarketSnapshot(
				marketInfoKindIndustryResearch,
				fmt.Sprintf(":%d:500", days),
				tradeDate,
				raw,
				time.Now().Format(time.RFC3339),
			); err != nil {
				result.Failures[marketInfoKindIndustryResearch] = err.Error()
				if !request.ContinueOnError {
					return result, err
				}
			} else {
				result.Success++
			}
		}
	}

	if containsString(kinds, "long-tiger") {
		if err := paceMarketInfoRequest(ctx); err != nil {
			return result, err
		}
		items, err := fetchLongTigerUpstream(ctx, tradeDate)
		if err != nil {
			result.Failures["long-tiger"] = err.Error()
			if !request.ContinueOnError {
				return result, err
			}
		} else {
			raw, marshalErr := json.Marshal(items)
			if marshalErr != nil {
				result.Failures["long-tiger"] = marshalErr.Error()
				if !request.ContinueOnError {
					return result, marshalErr
				}
			} else if marketStore == nil {
				result.Failures["long-tiger"] = "行情数据库未初始化"
				if !request.ContinueOnError {
					return result, errors.New("行情数据库未初始化")
				}
			} else if err := marketStore.saveLongTigerCache(tradeDate, string(raw), time.Now().Format(time.RFC3339)); err != nil {
				result.Failures["long-tiger"] = err.Error()
				if !request.ContinueOnError {
					return result, err
				}
			} else if err := marketStore.saveMarketSnapshot("long-tiger", "all", tradeDate, raw, time.Now().Format(time.RFC3339)); err != nil {
				result.Failures["long-tiger"] = err.Error()
				if !request.ContinueOnError {
					return result, err
				}
			} else {
				result.Success++
			}
		}
	}

	for _, code := range codes {
		dataKey := normalizeEastmoneyCode(code)
		for _, kind := range []string{marketInfoKindHotMoney, marketInfoKindResearch, marketInfoKindNotice} {
			if !containsString(kinds, kind) {
				continue
			}
			if err := paceMarketInfoRequest(ctx); err != nil {
				return result, err
			}
			items, err := fetchMarketInfoForCode(ctx, kind, code)
			if err != nil {
				result.Failures[kind+":"+dataKey] = err.Error()
				if !request.ContinueOnError {
					return result, err
				}
				continue
			}
			raw, marshalErr := json.Marshal(items)
			if marshalErr != nil {
				result.Failures[kind+":"+dataKey] = marshalErr.Error()
				if !request.ContinueOnError {
					return result, marshalErr
				}
				continue
			}
			if marketStore == nil {
				result.Failures[kind+":"+dataKey] = "行情数据库未初始化"
				if !request.ContinueOnError {
					return result, errors.New("行情数据库未初始化")
				}
				continue
			}
			if err := marketStore.saveMarketSnapshot(kind, dataKey, tradeDate, raw, time.Now().Format(time.RFC3339)); err != nil {
				result.Failures[kind+":"+dataKey] = err.Error()
				if !request.ContinueOnError {
					return result, err
				}
				continue
			}
			result.Success++
		}
	}

	if result.Success == 0 && len(result.Failures) > 0 {
		return result, fmt.Errorf("所有市场信息同步项均失败")
	}
	return result, nil
}

func normalizeMarketInfoKinds(values []string) []string {
	if len(values) == 0 {
		values = []string{"long-tiger", marketInfoKindHotMoney, marketInfoKindResearch, marketInfoKindNotice, marketInfoKindIndustryResearch}
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		kind := strings.ToLower(strings.TrimSpace(value))
		switch kind {
		case "long-tiger", "long_tiger", "longtiger":
			kind = "long-tiger"
		case "hot-money", "hot_money", "hotmoney":
			kind = marketInfoKindHotMoney
		case "research", "report":
			kind = marketInfoKindResearch
		case "notice", "notices", "company":
			kind = marketInfoKindNotice
		case "industry-research", "industry_research", "industryreport", "industry-report":
			kind = marketInfoKindIndustryResearch
		default:
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	return result
}

func normalizeMarketInfoSyncCodes(values []string, maxCodes int) []string {
	if maxCodes <= 0 {
		maxCodes = 200
	}
	if maxCodes > 1000 {
		maxCodes = 1000
	}
	if len(values) == 0 && tdx.DefaultCodes != nil {
		values = tdx.DefaultCodes.GetStockCodes(maxCodes)
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		code := normalizeEastmoneyCode(value)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
		if len(result) >= maxCodes {
			break
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fetchMarketInfoForCode(ctx context.Context, kind, code string) (any, error) {
	switch kind {
	case marketInfoKindHotMoney:
		return fetchHotMoney(ctx, code)
	case marketInfoKindResearch:
		return fetchMarketResearch(ctx, code)
	case marketInfoKindNotice:
		return fetchMarketNotice(ctx, code)
	default:
		return nil, fmt.Errorf("不支持按股票代码同步类型: %s", kind)
	}
}

func paceMarketInfoRequest(ctx context.Context) error {
	interval := 300 * time.Millisecond
	if value := strings.TrimSpace(os.Getenv("MARKET_INFO_SYNC_INTERVAL_MS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 && parsed <= 60000 {
			interval = time.Duration(parsed) * time.Millisecond
		}
	}
	marketInfoLastRequest.Lock()
	wait := interval - time.Since(marketInfoLastRequest.at)
	if wait > 0 {
		timer := time.NewTimer(wait)
		marketInfoLastRequest.Unlock()
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		marketInfoLastRequest.Lock()
	}
	marketInfoLastRequest.at = time.Now()
	marketInfoLastRequest.Unlock()
	return nil
}

func getMarketInfoCached[T any](kind, rawCode string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	now := time.Now()
	dataKey := normalizeEastmoneyCode(rawCode)
	cacheKey := kind + ":" + dataKey
	marketInfoCache.Lock()
	if entry, ok := marketInfoCache.items[cacheKey]; ok && now.Before(entry.expires) {
		value, ok := entry.value.(T)
		if ok {
			marketInfoCache.Unlock()
			return value, nil
		}
	}
	marketInfoCache.Unlock()

	if marketStore != nil {
		row, err := marketStore.getMarketSnapshot(kind, dataKey, now.Format("2006-01-02"))
		if errors.Is(err, sql.ErrNoRows) {
			row, err = marketStore.getLatestMarketSnapshot(kind, dataKey)
		}
		if err == nil {
			var value T
			if err := json.Unmarshal(row.Data, &value); err == nil {
				marketInfoCache.Lock()
				marketInfoCache.items[cacheKey] = marketInfoCacheEntry{value: value, expires: now.Add(ttl)}
				marketInfoCache.Unlock()
				return value, nil
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			// DB 出错时继续回源。
		}
	}

	value, err := fetch()
	if err != nil {
		var zero T
		return zero, err
	}
	if raw, marshalErr := json.Marshal(value); marshalErr == nil && marketStore != nil {
		_ = marketStore.saveMarketSnapshot(kind, dataKey, now.Format("2006-01-02"), raw, now.Format(time.RFC3339))
	}
	marketInfoCache.Lock()
	marketInfoCache.items[cacheKey] = marketInfoCacheEntry{value: value, expires: now.Add(ttl)}
	marketInfoCache.Unlock()
	return value, nil
}

func normalizeEastmoneyCode(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "_", ".")
	value = strings.ReplaceAll(value, "-", ".")
	if strings.Contains(value, ".") {
		parts := strings.SplitN(value, ".", 2)
		value = parts[0]
	}
	value = strings.TrimPrefix(value, "SH")
	value = strings.TrimPrefix(value, "SZ")
	value = strings.TrimPrefix(value, "BJ")
	value = strings.TrimPrefix(value, "HK")
	value = strings.TrimPrefix(value, "US")
	return value
}

func eastmoneySecucode(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if strings.Contains(value, ".") {
		parts := strings.SplitN(value, ".", 2)
		return parts[0] + "." + parts[1]
	}
	code := normalizeEastmoneyCode(value)
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "68") {
		return code + ".SH"
	}
	if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "9") {
		return code + ".BJ"
	}
	return code + ".SZ"
}

func eastmoneyRequest(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("东方财富返回 HTTP %d", resp.StatusCode)
	}
	return data, nil
}

func eastmoneyWget(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error) {
	args := []string{"-qO-", "--timeout=15"}
	for key, value := range headers {
		args = append(args, "--header", key+": "+value)
	}
	if len(body) > 0 {
		args = append(args, "--post-data", string(body))
	}
	args = append(args, endpoint)
	out, err := exec.CommandContext(ctx, "wget", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("wget fallback failed: %w", err)
	}
	return out, nil
}

func fetchMarketResearch(ctx context.Context, rawCode string) ([]MarketResearchReport, error) {
	type requestBody struct {
		BeginTime    string `json:"beginTime"`
		EndTime      string `json:"endTime"`
		IndustryCode string `json:"industryCode"`
		Code         string `json:"code"`
		PageSize     int    `json:"pageSize"`
		PageNo       int    `json:"pageNo"`
		P            int    `json:"p"`
		PageNum      int    `json:"pageNum"`
		PageNumber   int    `json:"pageNumber"`
	}
	now := time.Now()
	payload, err := json.Marshal(requestBody{
		BeginTime:    now.AddDate(-1, 0, 0).Format("2006-01-02"),
		EndTime:      now.Format("2006-01-02"),
		IndustryCode: "*",
		Code:         normalizeEastmoneyCode(rawCode),
		PageSize:     50,
		PageNo:       1,
		P:            1,
		PageNum:      1,
		PageNumber:   1,
	})
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"Content-Type": "application/json",
		"Origin":       "https://data.eastmoney.com",
		"Referer":      "https://data.eastmoney.com/report/stock.jshtml",
		"Host":         "reportapi.eastmoney.com",
	}
	body, err := eastmoneyRequest(ctx, http.MethodPost, "https://reportapi.eastmoney.com/report/list2", payload, headers)
	if err != nil {
		body, err = eastmoneyWget(ctx, "https://reportapi.eastmoney.com/report/list2", payload, headers)
	}
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []MarketResearchReport `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errorsFromBody("研报", body)
	}
	return response.Data, nil
}

func fetchIndustryResearch(ctx context.Context, industryCode string, days, limit int, endDate time.Time) ([]IndustryResearchReport, error) {
	if err := paceMarketInfoRequest(ctx); err != nil {
		return nil, err
	}
	if days <= 0 {
		days = 7
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	end := endDate
	if end.IsZero() {
		end = time.Now()
	}
	query := url.Values{
		"industry":     {"*"},
		"industryCode": {industryCode},
		"beginTime":    {end.AddDate(0, 0, -days).Format("2006-01-02")},
		"endTime":      {end.Format("2006-01-02")},
		"pageNo":       {"1"},
		"pageSize":     {strconv.Itoa(limit)},
		"p":            {"1"},
		"pageNum":      {"1"},
		"pageNumber":   {"1"},
		"qType":        {"1"},
	}
	body, err := eastmoneyRequest(ctx, http.MethodGet, "https://reportapi.eastmoney.com/report/list?"+query.Encode(), nil, map[string]string{
		"Origin":  "https://data.eastmoney.com",
		"Referer": "https://data.eastmoney.com/report/industry.jshtml",
		"Host":    "reportapi.eastmoney.com",
	})
	if err != nil {
		body, err = eastmoneyWget(ctx, "https://reportapi.eastmoney.com/report/list?"+query.Encode(), nil, map[string]string{
			"Origin":  "https://data.eastmoney.com",
			"Referer": "https://data.eastmoney.com/report/industry.jshtml",
			"Host":    "reportapi.eastmoney.com",
		})
	}
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []IndustryResearchReport `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errorsFromBody("行业研究", body)
	}
	if response.Data == nil {
		return []IndustryResearchReport{}, nil
	}
	return response.Data, nil
}

func fetchIndustryDict(ctx context.Context) ([]IndustryDictItem, error) {
	if err := paceMarketInfoRequest(ctx); err != nil {
		return nil, err
	}
	query := url.Values{"bkCode": {"016"}}
	body, err := eastmoneyRequest(ctx, http.MethodGet, "https://reportapi.eastmoney.com/report/bk?"+query.Encode(), nil, map[string]string{
		"Origin":  "https://data.eastmoney.com",
		"Referer": "https://data.eastmoney.com/report/industry.jshtml",
		"Host":    "reportapi.eastmoney.com",
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []IndustryDictItem `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errorsFromBody("行业字典", body)
	}
	if response.Data == nil {
		return []IndustryDictItem{}, nil
	}
	return response.Data, nil
}

func fetchMarketNotice(ctx context.Context, rawCode string) ([]MarketNotice, error) {
	query := url.Values{
		"page_size":     {"50"},
		"page_index":    {"1"},
		"ann_type":      {"SHA,CYB,SZA,BJA,INV"},
		"client_source": {"web"},
		"f_node":        {"0"},
		"stock_list":    {normalizeEastmoneyCode(rawCode)},
	}
	body, err := eastmoneyRequest(ctx, http.MethodGet, "https://np-anotice-stock.eastmoney.com/api/security/ann?"+query.Encode(), nil, map[string]string{
		"Referer": "https://data.eastmoney.com/notices/hsa/5.html",
		"Host":    "np-anotice-stock.eastmoney.com",
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			List []struct {
				ArtCode string `json:"art_code"`
				Codes   []struct {
					StockCode string `json:"stock_code"`
					ShortName string `json:"short_name"`
				} `json:"codes"`
				Columns []struct {
					ColumnName string `json:"column_name"`
				} `json:"columns"`
				Title       string `json:"title"`
				NoticeDate  string `json:"notice_date"`
				DisplayTime string `json:"display_time"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errorsFromBody("公告", body)
	}
	result := make([]MarketNotice, 0, len(response.Data.List))
	for _, item := range response.Data.List {
		row := MarketNotice{ArtCode: item.ArtCode, Title: item.Title, NoticeDate: item.NoticeDate, DisplayTime: item.DisplayTime}
		if len(item.Codes) > 0 {
			row.StockCode = item.Codes[0].StockCode
			row.StockName = item.Codes[0].ShortName
		}
		if len(item.Columns) > 0 {
			row.ColumnName = item.Columns[0].ColumnName
		}
		result = append(result, row)
	}
	return result, nil
}

func fetchHotMoney(ctx context.Context, rawCode string) ([]HotMoneyTrade, error) {
	query := url.Values{
		"reportName":   {"RPT_OPERATEDEPT_TRADE"},
		"columns":      {"TRADE_DATE,EXPLANATION,OPERATEDEPT_NAME,BUY_AMT_REAL,BUY_RATIO,SELL_AMT_REAL,SELL_RATIO,SECURITY_CODE,SECURITY_NAME_ABBR,SECUCODE"},
		"quoteColumns": {""},
		"filter":       {fmt.Sprintf(`(SECUCODE="%s")(TRADE_DIRECTION="0")`, eastmoneySecucode(rawCode))},
		"pageNumber":   {"1"},
		"pageSize":     {"50"},
		"sortTypes":    {"-1,-1,1"},
		"sortColumns":  {"TRADE_DATE,EXPLANATION,RANK"},
		"source":       {"HSF10"},
		"client":       {"PC"},
		"v":            {strconv.FormatInt(time.Now().Unix(), 10)},
	}
	body, err := eastmoneyRequest(ctx, http.MethodGet, "https://datacenter.eastmoney.com/api/data/v1/get?"+query.Encode(), nil, map[string]string{
		"Referer": "https://emweb.securities.eastmoney.com/",
		"Host":    "datacenter.eastmoney.com",
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Result struct {
			Data []HotMoneyTrade `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errorsFromBody("游资动向", body)
	}
	return response.Result.Data, nil
}

func errorsFromBody(kind string, body []byte) error {
	text := strings.TrimSpace(string(body))
	if len(text) > 120 {
		text = text[:120]
	}
	if text == "" {
		return fmt.Errorf("东方财富%s响应为空", kind)
	}
	return fmt.Errorf("东方财富%s响应解析失败: %s", kind, text)
}
