package workbench

import "testing"

func TestFixedCloseSyncUsesHikyuuAfterClose(t *testing.T) {
	task := fixedCloseSyncAutomationTask()
	if task.Cron != "0 0 17 * * 1-5" {
		t.Fatalf("unexpected close sync cron: %s", task.Cron)
	}
	if task.PayloadJSON != `{"scope":"hikyuu_after_close"}` {
		t.Fatalf("unexpected close sync payload: %s", task.PayloadJSON)
	}
}
