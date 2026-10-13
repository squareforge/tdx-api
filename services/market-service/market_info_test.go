package main

import (
	"encoding/json"
	"testing"
)

func TestMarketResearchResponseAllowsMissingAndStringRatingChange(t *testing.T) {
	// Eastmoney mixes numeric ratings with an empty string for unrated reports.
	// One unrated report must not discard every report for the stock.
	var response struct {
		Data []MarketResearchReport `json:"data"`
	}
	err := json.Unmarshal([]byte(`{"data":[
		{"stockCode":"600009","infoCode":"numeric","ratingChange":3},
		{"stockCode":"600009","infoCode":"unrated","ratingChange":""},
		{"stockCode":"600009","infoCode":"string","ratingChange":"2"},
		{"stockCode":"600009","infoCode":"null","ratingChange":null}
	]}`), &response)
	if err != nil {
		t.Fatalf("decode mixed research ratings: %v", err)
	}
	if len(response.Data) != 4 {
		t.Fatalf("decoded %d reports, want 4", len(response.Data))
	}
	for index, want := range []flexibleInt{3, 0, 2, 0} {
		if got := response.Data[index]; got.StockCode != "600009" || got.RatingChange != want {
			t.Fatalf("report %d = %+v, want stock 600009 and rating %d", index, got, want)
		}
	}
}
