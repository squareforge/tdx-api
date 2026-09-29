package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	blsEmploymentURL = "https://www.bls.gov/schedule/news_release/empsit.htm"
	blsCPIURL        = "https://www.bls.gov/schedule/news_release/cpi.htm"
	fedFOMCURL       = "https://www.federalreserve.gov/monetarypolicy/fomccalendars.htm"
	beaScheduleURL   = "https://www.bea.gov/news/schedule"
)

type macroCalendarProvider struct {
	name  string
	url   string
	parse func([]byte) ([]MacroEvent, error)
}

var (
	macroCalendarProviders = []macroCalendarProvider{
		{name: "BLS", url: macroProviderURL("MACRO_EVENT_BLS_EMPLOYMENT_URL", blsEmploymentURL), parse: func(data []byte) ([]MacroEvent, error) {
			return parseBLSReleasePage(data, "NFP", "非农就业报告", "employment", blsEmploymentURL, "观察就业新增、失业率与平均时薪，判断美国经济冷热和通胀压力。", "Employment Situation")
		}},
		{name: "BLS", url: macroProviderURL("MACRO_EVENT_BLS_CPI_URL", blsCPIURL), parse: func(data []byte) ([]MacroEvent, error) {
			return parseBLSReleasePage(data, "CPI", "美国 CPI 通胀", "inflation", blsCPIURL, "核心通胀是利率预期和风险资产定价的重要输入。", "Consumer Price Index")
		}},
		{name: "Federal Reserve", url: macroProviderURL("MACRO_EVENT_FOMC_URL", fedFOMCURL), parse: parseFOMCPage},
		{name: "BEA", url: macroProviderURL("MACRO_EVENT_BEA_URL", beaScheduleURL), parse: parseBEASchedulePage},
	}
	macroCalendarSyncMu       sync.Mutex
	macroCalendarSyncLoopOnce sync.Once
)

func macroProviderURL(envName, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}
	return fallback
}

type macroCalendarSyncResult struct {
	Status   string                `json:"status"`
	Imported int                   `json:"imported"`
	States   []MacroEventSyncState `json:"states"`
}

func startMacroCalendarSyncLoop() {
	macroCalendarSyncLoopOnce.Do(func() {
		syncEnabled := envBool("MACRO_EVENT_SYNC_ENABLED", true)
		interval := 12 * time.Hour
		if value := strings.TrimSpace(os.Getenv("MACRO_EVENT_SYNC_INTERVAL")); value != "" {
			if parsed, err := time.ParseDuration(value); err == nil && parsed >= time.Hour {
				interval = parsed
			}
		}
		go func() {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			<-timer.C
			if syncEnabled {
				if result, err := syncMacroCalendars(context.Background()); err != nil {
					log.Printf("宏观日历自动同步失败: %v", err)
				} else {
					log.Printf("宏观日历自动同步完成: %s, 导入 %d 条", result.Status, result.Imported)
				}
			}
			syncTicker := time.NewTicker(interval)
			var syncC <-chan time.Time
			if syncEnabled {
				syncC = syncTicker.C
			}
			alertTicker := time.NewTicker(time.Minute)
			defer syncTicker.Stop()
			defer alertTicker.Stop()
			for {
				select {
				case <-syncC:
					if result, err := syncMacroCalendars(context.Background()); err != nil {
						log.Printf("宏观日历自动同步失败: %v", err)
					} else {
						log.Printf("宏观日历自动同步完成: %s, 导入 %d 条", result.Status, result.Imported)
					}
				case <-alertTicker.C:
					dispatchMacroAlerts(context.Background())
				}
			}
		}()
	})
}

func syncMacroCalendars(ctx context.Context) (macroCalendarSyncResult, error) {
	macroCalendarSyncMu.Lock()
	defer macroCalendarSyncMu.Unlock()

	client := &http.Client{Timeout: 20 * time.Second}
	result := macroCalendarSyncResult{Status: "success"}
	syncAt := time.Now().UTC().Format(time.RFC3339Nano)
	providerEvents := map[string][]MacroEvent{}
	providerMessages := map[string][]string{}
	providerAttempts := map[string]string{}
	providerFeedCounts := map[string]int{}
	providerSuccessfulFeeds := map[string]int{}
	for _, provider := range macroCalendarProviders {
		providerFeedCounts[provider.name]++
		providerAttempts[provider.name] = nowText()
		data, err := fetchMacroCalendar(ctx, client, provider.url)
		if err != nil {
			providerMessages[provider.name] = append(providerMessages[provider.name], provider.url+": "+err.Error())
			continue
		}
		events, err := provider.parse(data)
		if err != nil {
			providerMessages[provider.name] = append(providerMessages[provider.name], provider.url+": "+err.Error())
			continue
		}
		if len(events) == 0 {
			providerMessages[provider.name] = append(providerMessages[provider.name], provider.url+": 未解析到可用事件")
			continue
		}
		providerSuccessfulFeeds[provider.name]++
		providerEvents[provider.name] = append(providerEvents[provider.name], events...)
	}

	for provider, events := range providerEvents {
		if len(events) == 0 {
			providerMessages[provider] = append(providerMessages[provider], "未解析到可用事件")
			continue
		}
		// Upsert first, then remove official records not seen in this
		// successful parse. This lets an unchanged event keep its row and
		// acknowledgement while still removing cancelled or moved dates.
		insertFailed := false
		for index := range events {
			event := &events[index]
			if event.SourceKind == "" {
				event.SourceKind = "official"
			}
			if event.ExternalID == "" {
				event.ExternalID = event.ID
			}
			event.LastSeenAt = syncAt
			if event.ScheduledAt == "" {
				event.ScheduledAt = event.StartsAt
			}
			if event.AShareDate == "" {
				event.AShareDate = nextAShareTradingDate(event.StartsAt)
			}
			if _, err := appStore.UpsertMacroEvent(*event); err != nil {
				providerMessages[provider] = append(providerMessages[provider], event.Code+": "+err.Error())
				insertFailed = true
			}
		}
		allFeedsSucceeded := providerFeedCounts[provider] > 0 && providerSuccessfulFeeds[provider] == providerFeedCounts[provider]
		if !insertFailed && allFeedsSucceeded {
			if err := appStore.DeleteStaleOfficialMacroEvents(provider, syncAt); err != nil {
				providerMessages[provider] = append(providerMessages[provider], "清理过期官方记录失败: "+err.Error())
			}
		}
	}

	for _, provider := range []string{"BLS", "Federal Reserve", "BEA"} {
		messages := providerMessages[provider]
		state := MacroEventSyncState{
			ID:            macroSyncStateID(provider),
			Provider:      provider,
			LastAttemptAt: providerAttempts[provider],
			EventCount:    len(providerEvents[provider]),
			Message:       strings.Join(messages, "；"),
		}
		if len(messages) == 0 && len(providerEvents[provider]) > 0 {
			state.Status = "success"
			state.LastSuccessAt = state.LastAttemptAt
		} else if len(providerEvents[provider]) > 0 {
			state.Status = "partial"
			result.Status = "partial"
		} else {
			state.Status = "failed"
			result.Status = "partial"
		}
		if state.LastSuccessAt == "" {
			if previous, getErr := appStore.GetMacroEventSyncState(state.ID); getErr == nil {
				state.LastSuccessAt = previous.LastSuccessAt
			}
		}
		if err := appStore.UpsertMacroEventSyncState(state); err != nil {
			return result, err
		}
		result.Imported += len(providerEvents[provider])
	}
	if result.Imported == 0 {
		result.Status = "failed"
	}
	result.States, _ = appStore.ListMacroEventSyncStates()
	return result, nil
}

func nextAShareTradingDate(startsAt string) string {
	parsed, err := time.Parse(time.RFC3339, startsAt)
	if err != nil || manager == nil || manager.Workday == nil {
		return ""
	}
	date := parsed.In(time.FixedZone("CST", 8*60*60))
	startOffset := 0
	if date.Hour() >= 15 {
		startOffset = 1
	}
	for offset := startOffset; offset <= 30; offset++ {
		candidate := date.AddDate(0, 0, offset)
		if manager.Workday.Is(candidate) {
			return candidate.Format("2006-01-02")
		}
	}
	return ""
}

func fetchMacroCalendar(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", "TDX-Workbench-macro-calendar/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("官方来源返回 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

var releaseDatePattern = regexp.MustCompile(`(?i)\b(?:january|february|march|april|may|june|july|august|september|october|november|december|jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec)\s+\d{1,2},?\s+\d{4}\b|\b\d{1,2}/\d{1,2}/\d{4}\b`)

func parseReleaseDate(value string) (time.Time, bool) {
	match := releaseDatePattern.FindString(value)
	if match == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"January 2, 2006", "Jan 2, 2006", "1/2/2006", "01/02/2006"} {
		if parsed, err := time.Parse(layout, strings.ReplaceAll(match, ", ", " ")); err == nil {
			return parsed, true
		}
		if parsed, err := time.Parse(layout, match); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func releaseAtET(date time.Time, hour, minute int) string {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		location = time.FixedZone("ET", -5*60*60)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location).In(time.FixedZone("CST", 8*60*60)).Format(time.RFC3339)
}

func parseBLSReleasePage(data []byte, code, name, category, sourceURL, description, titleHint string) ([]MacroEvent, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	rows := []*html.Node{}
	walkHTML(doc, func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			rows = append(rows, node)
		}
	})
	events := make([]MacroEvent, 0, len(rows))
	for _, row := range rows {
		text := normalizeHTMLText(htmlNodeText(row))
		if !strings.Contains(strings.ToLower(text), strings.ToLower(titleHint)) {
			continue
		}
		date, ok := parseReleaseDate(text)
		if !ok {
			continue
		}
		events = append(events, MacroEvent{
			ID:          fmt.Sprintf("macro-official-%s-%s", strings.ToLower(code), date.Format("20060102")),
			ExternalID:  fmt.Sprintf("%s-%s", strings.ToLower(code), date.Format("20060102")),
			Code:        code,
			Name:        name,
			Category:    category,
			Country:     "US",
			Impact:      "high",
			StartsAt:    releaseAtET(date, 8, 30),
			Source:      "BLS",
			SourceURL:   sourceURL,
			SourceKind:  "official",
			Description: description,
			Status:      "scheduled",
		})
	}
	return uniqueMacroEvents(events), nil
}

func parseFOMCPage(data []byte) ([]MacroEvent, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	events := []MacroEvent{}
	var visit func(*html.Node, *int)
	visit = func(node *html.Node, currentYear *int) {
		if node.Type == html.ElementNode && (node.Data == "h3" || node.Data == "h4") {
			if match := regexp.MustCompile(`\b(20\d{2})\b`).FindStringSubmatch(htmlNodeText(node)); len(match) == 2 {
				*currentYear, _ = strconv.Atoi(match[1])
			}
		}
		if node.Type == html.ElementNode && hasHTMLClass(node, "fomc-meeting") && *currentYear > 0 {
			text := normalizeHTMLText(htmlNodeText(node))
			match := regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})(?:\s*-\s*(\d{1,2}))?`).FindStringSubmatch(text)
			if len(match) >= 3 {
				month, _ := time.Parse("January", match[1])
				firstDay, _ := strconv.Atoi(match[2])
				lastDay := firstDay
				if len(match) >= 4 && match[3] != "" {
					lastDay, _ = strconv.Atoi(match[3])
				}
				meetingDate := time.Date(*currentYear, month.Month(), firstDay, 0, 0, 0, 0, time.UTC)
				decisionDate := time.Date(*currentYear, month.Month(), lastDay, 0, 0, 0, 0, time.UTC)
				events = append(events, MacroEvent{
					ID:          fmt.Sprintf("macro-official-fomc-%s", meetingDate.Format("20060102")),
					ExternalID:  fmt.Sprintf("fomc-%s", meetingDate.Format("20060102")),
					Code:        "FOMC",
					Name:        "美联储议息会议",
					Category:    "central_bank",
					Country:     "US",
					Impact:      "critical",
					StartsAt:    releaseAtET(decisionDate, 14, 0),
					Source:      "Federal Reserve",
					SourceURL:   fedFOMCURL,
					SourceKind:  "official",
					Description: "关注利率决议、点阵图和主席发布会，事件窗口可能扩大跨市场波动。",
					Status:      "scheduled",
				})
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, currentYear)
		}
	}
	currentYear := 0
	visit(doc, &currentYear)
	cutoff := time.Now().Add(-24 * time.Hour)
	end := time.Now().AddDate(1, 6, 0)
	filtered := make([]MacroEvent, 0, len(events))
	for _, event := range uniqueMacroEvents(events) {
		startsAt, parseErr := time.Parse(time.RFC3339, event.StartsAt)
		if parseErr == nil && !startsAt.Before(cutoff) && !startsAt.After(end) {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func parseBEASchedulePage(data []byte) ([]MacroEvent, error) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	defaultYear := time.Now().Year()
	if match := regexp.MustCompile(`\bYear\s+(20\d{2})\b`).FindStringSubmatch(normalizeHTMLText(htmlNodeText(doc))); len(match) == 2 {
		defaultYear, _ = strconv.Atoi(match[1])
	}
	rows := []*html.Node{}
	walkHTML(doc, func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" && strings.Contains(strings.ToLower(htmlNodeText(node)), "personal income and outlays") {
			rows = append(rows, node)
		}
	})
	events := make([]MacroEvent, 0, len(rows))
	for _, row := range rows {
		text := normalizeHTMLText(htmlNodeText(row))
		date, ok := parseReleaseDate(text)
		if !ok {
			date, ok = parseMonthDay(text, defaultYear)
		}
		if !ok {
			continue
		}
		events = append(events, MacroEvent{
			ID:          fmt.Sprintf("macro-official-pce-%s", date.Format("20060102")),
			ExternalID:  fmt.Sprintf("pce-%s", date.Format("20060102")),
			Code:        "PCE",
			Name:        "美国 PCE 物价指数",
			Category:    "inflation",
			Country:     "US",
			Impact:      "high",
			StartsAt:    releaseAtET(date, 8, 30),
			Source:      "BEA",
			SourceURL:   beaScheduleURL,
			SourceKind:  "official",
			Description: "美联储偏好的通胀指标，结合核心 PCE 观察降息空间。",
			Status:      "scheduled",
		})
	}
	return uniqueMacroEvents(events), nil
}

var monthDayPattern = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})\b`)

func parseMonthDay(value string, year int) (time.Time, bool) {
	match := monthDayPattern.FindStringSubmatch(value)
	if len(match) != 3 {
		return time.Time{}, false
	}
	month, err := time.Parse("January", match[1])
	if err != nil {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(match[2])
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(year, month.Month(), day, 0, 0, 0, 0, time.UTC), true
}

func uniqueMacroEvents(items []MacroEvent) []MacroEvent {
	seen := map[string]bool{}
	result := make([]MacroEvent, 0, len(items))
	for _, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		result = append(result, item)
	}
	return result
}

func walkHTML(node *html.Node, visit func(*html.Node)) {
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, visit)
	}
}

func hasHTMLClass(node *html.Node, class string) bool {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			for _, value := range strings.Fields(attr.Val) {
				if value == class {
					return true
				}
			}
		}
	}
	return false
}

func htmlNodeText(node *html.Node) string {
	var builder strings.Builder
	walkHTML(node, func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
	})
	return builder.String()
}

func normalizeHTMLText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func macroSyncStateID(provider string) string {
	return "macro-sync-" + strings.ToLower(strings.ReplaceAll(provider, " ", "-"))
}
