package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeMinuteDate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "compact", in: "20260610", want: "20260610"},
		{name: "hyphenated", in: "2026-06-10", want: "20260610"},
		{name: "trim spaces", in: " 2026-06-10 ", want: "20260610"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMinuteDate(tt.in)
			if err != nil {
				t.Fatalf("normalizeMinuteDate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeMinuteDate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMinuteDateRejectsInvalidInput(t *testing.T) {
	if _, err := normalizeMinuteDate("2026/06/10"); err == nil {
		t.Fatal("normalizeMinuteDate() expected error for invalid date")
	}
}

func TestDailyReviewAnalysisRejectsUnsupportedReturnBasis(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/daily-review/analysis?basis=unsupported", nil)
	response := httptest.NewRecorder()
	handleDailyReviewAnalysis(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != -1 || envelope.Message == "" {
		t.Fatalf("unexpected error response: %+v", envelope)
	}
}

func TestDailyReviewAnalysisRejectsUnimplementedBenchmark(t *testing.T) {
	for _, benchmark := range []string{"csi300", "none"} {
		request := httptest.NewRequest(http.MethodGet, "/api/daily-review/analysis?benchmark="+benchmark, nil)
		response := httptest.NewRecorder()
		handleDailyReviewAnalysis(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("benchmark %s: status = %d, want 400", benchmark, response.Code)
		}
	}
}

func TestGetAllCodeModelsPrefersMarketService(t *testing.T) {
	t.Setenv("MARKET_SERVICE_URL", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/codes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"codes":[{"code":"000001","name":"平安银行","exchange":"sz"},{"code":"600000","name":"浦发银行","exchange":"sh"}]}}`))
	}))
	defer server.Close()

	t.Setenv("MARKET_SERVICE_URL", server.URL)
	models, err := getAllCodeModels()
	if err != nil {
		t.Fatalf("getAllCodeModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("getAllCodeModels() len = %d, want 2", len(models))
	}
	if models[0].Exchange == "" || models[1].Exchange == "" {
		t.Fatal("expected exchange to be populated")
	}
}

func TestRunAutomationTaskOnWorker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/automations/fixed-close-sync/run" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "success",
			"data": AutomationRun{
				ID:     "run-1",
				TaskID: FixedCloseSyncTaskID,
				Status: "success",
			},
		})
	}))
	defer server.Close()

	t.Setenv("SELECTION_WORKER_URL", server.URL)
	run, err := runAutomationTaskOnWorker(context.Background(), FixedCloseSyncTaskID, time.Second)
	if err != nil {
		t.Fatalf("runAutomationTaskOnWorker() error = %v", err)
	}
	if run.ID != "run-1" || run.TaskID != FixedCloseSyncTaskID || run.Status != "success" {
		t.Fatalf("runAutomationTaskOnWorker() = %#v", run)
	}
}

func TestStrategyRunProxiesToSelectionWorker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/strategies/template-test/run" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":"run-1","status":"success"}}`))
	}))
	defer server.Close()

	t.Setenv("SELECTION_WORKER_URL", server.URL)
	request := httptest.NewRequest(http.MethodPost, "/api/strategies/template-test/run", nil)
	response := httptest.NewRecorder()

	handleStrategyOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 || envelope.Data.ID != "run-1" || envelope.Data.Status != "success" {
		t.Fatalf("unexpected response: %+v", envelope)
	}
}
