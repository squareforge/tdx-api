package tdx

import (
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/lib/xorms"
)

func TestWorkdayEnsureSchemaMigratesLegacyRowsAsTradeDays(t *testing.T) {
	engine, err := xorms.NewSqlite(filepath.Join(t.TempDir(), "workday.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if _, err := engine.Exec(`CREATE TABLE workday (ID INTEGER PRIMARY KEY AUTOINCREMENT, Unix INTEGER, Date TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Exec(`INSERT INTO workday (Unix, Date) VALUES (?, ?)`, 1, "20260925"); err != nil {
		t.Fatal(err)
	}
	w := &Workday{db: engine}
	if err := w.ensureSchema(); err != nil {
		t.Fatal(err)
	}
	var isTradeDay int
	if ok, err := engine.SQL(`SELECT is_trade_day FROM workday WHERE Date=?`, "20260925").Get(&isTradeDay); err != nil || !ok {
		t.Fatalf("read migrated row: ok=%v err=%v", ok, err)
	}
	if isTradeDay != 1 {
		t.Fatalf("legacy row is_trade_day=%d, want 1", isTradeDay)
	}
	if _, err := engine.Exec(`UPDATE workday SET is_trade_day=0 WHERE Date=?`, "20260925"); err != nil {
		t.Fatal(err)
	}
	if err := w.ensureSchema(); err != nil {
		t.Fatal(err)
	}
	if ok, err := engine.SQL(`SELECT is_trade_day FROM workday WHERE Date=?`, "20260925").Get(&isTradeDay); err != nil || !ok {
		t.Fatalf("read upgraded row: ok=%v err=%v", ok, err)
	}
	if isTradeDay != 0 {
		t.Fatalf("explicit upgraded value was overwritten: %d", isTradeDay)
	}
}
