package main

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
)

func TestEnsureTradeCalendarSchemaMigratesLegacyRowsAsTradeDays(t *testing.T) {
	db, err := sql.Open("sqlite", "file:trade-calendar-migration?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE workday (ID INTEGER PRIMARY KEY AUTOINCREMENT, Unix INTEGER, Date TEXT); INSERT INTO workday (Unix, Date) VALUES (1, '20260925')`); err != nil {
		t.Fatal(err)
	}
	if err := ensureTradeCalendarSchema(db); err != nil {
		t.Fatal(err)
	}
	var isTradeDay int
	if err := db.QueryRow(`SELECT is_trade_day FROM workday WHERE Date='20260925'`).Scan(&isTradeDay); err != nil {
		t.Fatal(err)
	}
	if isTradeDay != 1 {
		t.Fatalf("legacy row is_trade_day=%d, want 1", isTradeDay)
	}
	if _, err := db.Exec(`UPDATE workday SET is_trade_day=0 WHERE Date='20260925'`); err != nil {
		t.Fatal(err)
	}
	if err := ensureTradeCalendarSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT is_trade_day FROM workday WHERE Date='20260925'`).Scan(&isTradeDay); err != nil {
		t.Fatal(err)
	}
	if isTradeDay != 0 {
		t.Fatalf("explicit upgraded value was overwritten: %d", isTradeDay)
	}
}
