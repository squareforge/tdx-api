package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

func handleHikyuuKlineBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var req struct {
		Symbols []string `json:"symbols"`
		Period  string   `json:"period"`
		Recover string   `json:"recover"`
		Limit   int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	if len(req.Symbols) == 0 || len(req.Symbols) > 128 {
		errorResponse(w, "symbols数量必须在1到128之间")
		return
	}
	period := strings.ToLower(strings.TrimSpace(req.Period))
	if period == "" {
		period = "day"
	}
	recover := strings.ToLower(strings.TrimSpace(req.Recover))
	if recover == "" {
		recover = "qfq"
	}
	if req.Limit <= 0 || req.Limit > 2000 {
		req.Limit = 260
	}
	data, failures, hikyuuErr := hikyuuClient.FetchKlineBatch(r.Context(), req.Symbols, period, recover, req.Limit)
	if data == nil {
		data = map[string]*protocol.KlineResp{}
	}
	if failures == nil {
		failures = map[string]string{}
	}
	if hikyuuErr != nil {
		for _, symbol := range req.Symbols {
			failures[symbol] = hikyuuErr.Error()
		}
	}

	fallbackSymbols := make([]string, 0, len(req.Symbols))
	for _, symbol := range req.Symbols {
		if _, ok := data[symbol]; !ok {
			fallbackSymbols = append(fallbackSymbols, symbol)
		}
	}
	tdxData, tdxFailures := loadTDXKlineBatch(r.Context(), fallbackSymbols, period, req.Limit)
	for symbol, item := range tdxData {
		data[symbol] = item
		delete(failures, symbol)
	}
	for symbol, message := range tdxFailures {
		if hikyuuMessage := failures[symbol]; hikyuuMessage != "" {
			failures[symbol] = "本地Hikyuu: " + hikyuuMessage + "; 通达信: " + message
		} else {
			failures[symbol] = "通达信: " + message
		}
	}
	successResponse(w, map[string]interface{}{
		"data": data, "errors": failures, "source": "market-service",
		"hikyuu_count": len(data) - len(tdxData), "tdx_fallback_count": len(tdxData),
	})
}

func loadTDXKlineBatch(ctx context.Context, symbols []string, period string, limit int) (map[string]*protocol.KlineResp, map[string]string) {
	data := make(map[string]*protocol.KlineResp, len(symbols))
	failures := map[string]string{}
	if len(symbols) == 0 {
		return data, failures
	}
	type loadResult struct {
		symbol string
		resp   *protocol.KlineResp
		err    error
	}
	workerCount := 8
	if len(symbols) < workerCount {
		workerCount = len(symbols)
	}
	jobs := make(chan string)
	results := make(chan loadResult, workerCount)
	var workers sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for symbol := range jobs {
				resp, err := loadTDXKline(ctx, symbol, period, limit)
				select {
				case results <- loadResult{symbol: symbol, resp: resp, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, symbol := range symbols {
			select {
			case jobs <- symbol:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	for item := range results {
		if item.err != nil {
			failures[item.symbol] = item.err.Error()
		} else {
			data[item.symbol] = item.resp
		}
	}
	return data, failures
}

func loadTDXKline(ctx context.Context, symbol, period string, limit int) (*protocol.KlineResp, error) {
	if manager == nil {
		return nil, fmt.Errorf("数据管理器未初始化")
	}
	if period != "day" {
		return nil, fmt.Errorf("通达信批量回退暂只支持日线")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var resp *protocol.KlineResp
	err := manager.Do(func(c *tdx.Client) error {
		var err error
		resp, err = c.GetKlineDayAll(symbol)
		return err
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.List) == 0 {
		return nil, fmt.Errorf("K线为空")
	}
	if limit > 0 && len(resp.List) > limit {
		resp.List = resp.List[len(resp.List)-limit:]
	}
	resp.Count = uint16(len(resp.List))
	return resp, nil
}

func handleHikyuuAfterCloseSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	var payload struct {
		ExpectedDate string `json:"expected_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && err != io.EOF {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}
	task, err := hikyuuClient.StartAfterCloseSync(r.Context(), payload.ExpectedDate)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, task)
}

func handleHikyuuFreshness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	expectedDate := strings.TrimSpace(r.URL.Query().Get("expected_date"))
	if expectedDate == "" {
		errorResponse(w, "expected_date不能为空")
		return
	}
	result, err := hikyuuClient.GetFreshness(r.Context(), expectedDate)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, result)
}

func handleHikyuuTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/hikyuu/tasks/")
	if strings.TrimSpace(id) == "" {
		errorResponse(w, "任务ID不能为空")
		return
	}
	task, err := hikyuuClient.GetTask(r.Context(), id)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	successResponse(w, task)
}
