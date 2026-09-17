import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import app


class AppTaskTest(unittest.TestCase):
    def test_detect_import_failure_recognizes_stopped_import_log(self):
        message = app.detect_import_failure("SH DAY 连续失败20个股票，已停止导入, 建议重新导入")

        self.assertIsNotNone(message)
        self.assertIn("连续失败20个股票", message)

    def test_failed_task_snapshot_overrides_finished_log_progress(self):
        with tempfile.TemporaryDirectory() as directory:
            log_file = Path(directory) / "task.log"
            log_file.write_text("['HDF5_IMPORT', 'THREAD', 'FINISHED']\n", encoding="utf-8")
            snapshot = app.task_snapshot({
                "id": "test",
                "status": "failed",
                "error": "停止导入",
                "log_file": str(log_file),
            })

        self.assertEqual(snapshot["stage"], "失败")
        self.assertEqual(snapshot["message"], "停止导入")
        self.assertEqual(snapshot["progress"], 100)


class CreateTaskGuardTest(unittest.TestCase):
    def setUp(self):
        app.tasks.clear()
        app.task_order.clear()
        thread_patch = patch.object(app.threading, "Thread")
        self.mock_thread = thread_patch.start()
        self.addCleanup(thread_patch.stop)
        self.addCleanup(app.tasks.clear)
        self.addCleanup(app.task_order.clear)

    def _create(self, **overrides):
        request = app.SyncRequest(**overrides)
        return app.create_task("full_sync", request)

    def test_request_scope_only_keeps_enabled_sync_keys(self):
        scope = app.request_scope({"day": True, "block": True, "min": False, "stock": True})

        self.assertEqual(scope, {"day": True, "block": True})

    def test_accepts_non_overlapping_scope_and_downgrades_shared_flags(self):
        self._create(day=True, min=False, min5=False, weight=False, finance=False, block=False)

        second = self._create(day=False, min=False, min5=False, weight=False, finance=False, block=True)

        self.assertTrue(second["request"]["block"])
        self.assertFalse(second["request"]["stock"])
        self.assertFalse(second["request"]["fund"])

    def test_rejects_overlapping_scope(self):
        self._create(day=True, min=False, min5=False, weight=False, finance=False, block=False)

        with self.assertRaises(app.HTTPException) as ctx:
            self._create(day=True, min=False, min5=False, weight=False, finance=False, block=False)

        self.assertEqual(ctx.exception.status_code, 409)
        self.assertIn("日线", ctx.exception.detail)

    def test_rejects_when_concurrency_limit_reached(self):
        with patch.object(app, "MAX_CONCURRENT_TASKS", 2):
            self._create(day=True, min=False, min5=False, weight=False, finance=False, block=False)
            self._create(day=False, min=False, min5=False, weight=False, finance=False, block=True)

            with self.assertRaises(app.HTTPException) as ctx:
                self._create(day=False, min=False, min5=False, weight=True, finance=False, block=False)

        self.assertEqual(ctx.exception.status_code, 409)
        self.assertIn("上限", ctx.exception.detail)

    def test_rejects_empty_scope(self):
        with self.assertRaises(app.HTTPException) as ctx:
            self._create(day=False, min=False, min5=False, trans=False, time=False, weight=False, finance=False, block=False)

        self.assertEqual(ctx.exception.status_code, 400)


if __name__ == "__main__":
    unittest.main()
