package workbench

import (
	"encoding/json"
	"testing"
)

func TestDefaultStrategyTemplatesAreValidAndConservative(t *testing.T) {
	templates := defaultStrategyTemplates()
	if len(templates) != 10 {
		t.Fatalf("template count = %d, want 10", len(templates))
	}

	seen := make(map[string]bool, len(templates))
	for _, item := range templates {
		if item.ID == "" || seen[item.ID] {
			t.Fatalf("strategy template ID must be unique and non-empty: %q", item.ID)
		}
		seen[item.ID] = true
		research := len(item.ID) >= len("template-research-") && item.ID[:len("template-research-")] == "template-research-"
		if item.Enabled == research || !item.Readonly {
			t.Fatalf("template %q has unexpected enabled/readonly state", item.ID)
		}

		var config struct {
			ScanLimit *int `json:"scan_limit"`
			Filters   []struct {
				ID     string                 `json:"id"`
				Factor string                 `json:"factor"`
				Params map[string]interface{} `json:"params"`
			} `json:"filters"`
			Scores []struct {
				Factor string `json:"factor"`
			} `json:"scores"`
			Pass struct {
				MinScore float64 `json:"min_score"`
				TopN     int     `json:"top_n"`
			} `json:"pass"`
			Execution struct {
				MinOpenGap   *float64 `json:"min_open_gap"`
				MaxOpenGap   *float64 `json:"max_open_gap"`
				BuyAtOpen    bool     `json:"buy_at_open"`
				ExitPolicy   string   `json:"exit_policy"`
				OnlyWhenFlat bool     `json:"only_when_flat"`
			} `json:"execution"`
			Universe struct {
				Exclude []struct {
					Pool string `json:"pool"`
				} `json:"exclude"`
			} `json:"universe"`
		}
		if err := json.Unmarshal([]byte(item.ConfigJSON), &config); err != nil {
			t.Fatalf("template %q has invalid config JSON: %v", item.ID, err)
		}
		if len(config.Filters) == 0 || len(config.Scores) == 0 {
			t.Fatalf("template %q must include filters and scores", item.ID)
		}
		if config.Pass.MinScore <= 0 || config.Pass.TopN < 0 || config.Pass.TopN > 10 ||
			(config.Pass.TopN == 0 && item.ID != "template-jq-first-board-low-open") {
			t.Fatalf("template %q must use a bounded positive pass threshold: %+v", item.ID, config.Pass)
		}
		filterFactors := map[string]bool{}
		for _, rule := range config.Filters {
			filterFactors[rule.Factor] = true
		}
		if !filterFactors["pool_exclude"] ||
			(!filterFactors["min_amount"] && item.ID != "template-jq-limit-up-leader" && item.ID != "template-jq-first-board-low-open") {
			t.Fatalf("template %q must filter exclusions and liquidity: %+v", item.ID, filterFactors)
		}
		if config.ScanLimit == nil || *config.ScanLimit != 0 {
			t.Fatalf("template %q must use explicit full scan (0), got %+v", item.ID, config.ScanLimit)
		}
		excluded := map[string]bool{}
		for _, term := range config.Universe.Exclude {
			excluded[term.Pool] = true
		}
		for _, poolID := range []string{"exclude", "market-star", "market-bj"} {
			if !excluded[poolID] {
				t.Fatalf("template %q must exclude %s", item.ID, poolID)
			}
		}
		if strongMarketStrategyIDs[item.ID] && !filterFactors["market_momentum"] {
			t.Fatalf("strong-market strategy %q must include market regime filter", item.ID)
		}
		if item.ID == "template-macd-calm-confirm" {
			for _, required := range []string{"ma_trend", "ma_slope", "macd_golden_cross", "market_momentum", "gain_days", "max_amplitude"} {
				if !filterFactors[required] {
					t.Fatalf("high-win-rate template %q missing filter %q", item.ID, required)
				}
			}
		}
		if item.ID == "template-low-three-bull-first-board" {
			if !filterFactors["low_three_bull_first_board"] {
				t.Fatal("first-board template must include its point-in-time pattern filter")
			}
			for _, rule := range config.Filters {
				if rule.Factor != "low_three_bull_first_board" {
					continue
				}
				if rule.Params["max_pre_position"] != float64(60) ||
					rule.Params["min_volume_ratio"] != float64(2.5) ||
					rule.Params["max_five_day_range"] != float64(15) {
					t.Fatalf("first-board calibrated pattern parameters changed: %+v", rule.Params)
				}
			}
			var execution struct {
				MinOpenGap *float64 `json:"min_open_gap"`
				MaxOpenGap *float64 `json:"max_open_gap"`
			}
			var rawConfig struct {
				Execution json.RawMessage `json:"execution"`
			}
			if err := json.Unmarshal([]byte(item.ConfigJSON), &rawConfig); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(rawConfig.Execution, &execution); err != nil {
				t.Fatal(err)
			}
			if execution.MinOpenGap == nil || *execution.MinOpenGap != 0 || execution.MaxOpenGap == nil || *execution.MaxOpenGap != 6 {
				t.Fatalf("first-board opening-gap constraints = %+v, want 0%%-6%%", execution)
			}
		}
		if item.ID == "template-jq-limit-up-leader" {
			if !filterFactors["limit_up_streak"] {
				t.Fatal("leader template must filter by limit-up streak")
			}
			if !config.Execution.BuyAtOpen || config.Execution.ExitPolicy != "leader_close" {
				t.Fatalf("leader execution = %+v", config.Execution)
			}
		}
		if item.ID == "template-jq-first-board-low-open" {
			if !filterFactors["first_board_position"] {
				t.Fatal("first-board low-open template must filter by first-board position")
			}
			if !config.Execution.BuyAtOpen || config.Execution.ExitPolicy != "first_board_close" ||
				!config.Execution.OnlyWhenFlat || config.Execution.MinOpenGap == nil ||
				*config.Execution.MinOpenGap != -4 || config.Execution.MaxOpenGap == nil ||
				*config.Execution.MaxOpenGap != -3 {
				t.Fatalf("first-board low-open execution = %+v", config.Execution)
			}
		}
	}

	for _, id := range []string{
		"template-a-share-v3",
		"template-macd-trend",
		"template-macd-calm-confirm",
		"template-rocket-confirm",
		"template-low-three-bull-first-board",
		"template-jq-limit-up-leader",
		"template-jq-first-board-low-open",
		"template-research-nr7-close-breakout",
		"template-research-spring-reclaim",
		"template-research-gap-down-reclaim",
	} {
		if !seen[id] {
			t.Fatalf("missing template %q", id)
		}
	}
}

func TestRetiredSystemStrategyTemplatesAreRemoved(t *testing.T) {
	store := newMacroEventTestStore(t)
	now := NowText()
	for _, id := range retiredStrategyTemplateIDs {
		if _, err := store.db.Exec(`INSERT INTO strategies
			(id,name,description,config_json,enabled,readonly,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?)`, id, id, "legacy", `{}`, 1, 1, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ensureStrategyTemplates(); err != nil {
		t.Fatal(err)
	}
	for _, id := range retiredStrategyTemplateIDs {
		if _, err := store.GetStrategy(id); err == nil {
			t.Fatalf("retired system strategy %q was not removed", id)
		}
	}
}

func TestSystemTemplateUniverseCustomizationSurvivesTemplateSync(t *testing.T) {
	store := newMacroEventTestStore(t)
	if err := store.ensureStrategyTemplates(); err != nil {
		t.Fatal(err)
	}

	strategy, err := store.GetStrategy("template-a-share-v3")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &config); err != nil {
		t.Fatal(err)
	}
	config["universe"] = json.RawMessage(`{"include":[{"pool":"watchlist"}],"intersect":[],"exclude":[]}`)
	config["universe_customized"] = json.RawMessage(`true`)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStrategyUniverseConfig(strategy.ID, string(raw)); err != nil {
		t.Fatal(err)
	}

	if err := store.ensureStrategyTemplates(); err != nil {
		t.Fatal(err)
	}
	strategy, err = store.GetStrategy(strategy.ID)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Universe struct {
			Include []struct {
				Pool string `json:"pool"`
			} `json:"include"`
		} `json:"universe"`
		UniverseCustomized bool              `json:"universe_customized"`
		Filters            []json.RawMessage `json:"filters"`
	}
	if err := json.Unmarshal([]byte(strategy.ConfigJSON), &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.UniverseCustomized || len(saved.Universe.Include) != 1 || saved.Universe.Include[0].Pool != "watchlist" {
		t.Fatalf("custom universe was not preserved: %s", strategy.ConfigJSON)
	}
	if len(saved.Filters) == 0 {
		t.Fatalf("template rule configuration was unexpectedly lost: %s", strategy.ConfigJSON)
	}
}
