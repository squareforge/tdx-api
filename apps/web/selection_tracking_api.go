package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	workbench "workbench-core"
)

func handleSelectionTracking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		errorResponse(w, "只支持GET或POST请求")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	// The results page should be able to load cached tracking snapshots without
	// fetching hundreds of K-line histories. POST remains the explicit refresh path.
	cachedOnly := r.Method == http.MethodGet && (r.URL.Query().Get("cached") == "1" || r.URL.Query().Get("cached") == "true")
	onlyLatest := r.URL.Query().Get("latest") != "false" && r.URL.Query().Get("latest") != "0"
	items, err := appStore.ListSelectionResults(r.URL.Query().Get("task_id"), r.URL.Query().Get("formula_id"), r.URL.Query().Get("symbol"), onlyLatest, limit)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	targetReturn, _ := strconv.ParseFloat(r.URL.Query().Get("target_return"), 64)
	drawdownLimit, _ := strconv.ParseFloat(r.URL.Query().Get("drawdown_limit"), 64)
	targetReturn, drawdownLimit = workbench.DefaultTrackingPolicy(targetReturn, drawdownLimit)
	horizons := parseTrackingHorizons(r.URL.Query().Get("horizons"))
	trackingItems := make([]SelectionTrackingItem, 0, len(items))
	for _, item := range items {
		if cachedOnly {
			tracking := SelectionTracking{}
			if item.Tracking != nil {
				tracking = *item.Tracking
			}
			item.Tracking = nil
			trackingItems = append(trackingItems, SelectionTrackingItem{Result: item, Tracking: tracking})
			continue
		}
		bars, loadErr := loadTrackingKline(r.Context(), item.Symbol, 800)
		tracking := SelectionTracking{}
		if loadErr != nil {
			tracking = workbench.EvaluateSelectionTracking(item, nil, horizons, targetReturn, drawdownLimit, time.Now())
			item.Tracking = nil
			trackingItems = append(trackingItems, SelectionTrackingItem{Result: item, Tracking: tracking, Error: loadErr.Error()})
			continue
		}
		tracking = workbench.EvaluateSelectionTracking(item, toTrackingBars(bars), horizons, targetReturn, drawdownLimit, time.Now())
		_ = appStore.UpdateSelectionTracking(item.ID, tracking)
		item.Tracking = nil
		trackingItems = append(trackingItems, SelectionTrackingItem{Result: item, Tracking: tracking})
	}
	summary := workbench.SummarizeSelectionTracking(trackingItems, horizons)
	successResponse(w, map[string]interface{}{
		"items":   trackingItems,
		"summary": summary,
		"policy":  map[string]float64{"target_return": targetReturn, "drawdown_limit": drawdownLimit},
		"as_of":   time.Now().Format("2006-01-02"),
	})
}

func parseTrackingHorizons(raw string) []int {
	if strings.TrimSpace(raw) == "" {
		return []int{1, 3, 5, 10}
	}
	seen := map[int]bool{}
	result := []int{}
	for _, value := range strings.Split(raw, ",") {
		horizon, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && horizon > 0 && horizon <= 60 && !seen[horizon] {
			seen[horizon] = true
			result = append(result, horizon)
		}
	}
	if len(result) == 0 {
		return []int{1, 3, 5, 10}
	}
	return result
}

func toTrackingBars(rows []FormulaKline) []TrackingBar {
	bars := make([]TrackingBar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, TrackingBar{Date: row.Date, YClose: row.YClose, Open: row.Open, High: row.High, Low: row.Low, Close: row.Close})
	}
	return bars
}
