import tempfile
import threading
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
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


class QueryConcurrencyTest(unittest.TestCase):
    def setUp(self):
        app.active_imports = 0
        app.pending_imports = 0
        app.active_queries = 0

    def tearDown(self):
        app.active_imports = 0
        app.pending_imports = 0
        app.active_queries = 0

    def test_queries_run_concurrently_up_to_configured_limit(self):
        entered = threading.Barrier(2)
        release = threading.Event()
        failures = []

        def fake_run(*_args, **_kwargs):
            entered.wait(timeout=2)
            release.wait(timeout=2)
            return SimpleNamespace(returncode=0, stdout='{"ok": true}\n', stderr="")

        def run_query():
            try:
                app.run_json_process("query.py", [])
            except Exception as exc:
                failures.append(exc)

        with patch.object(app, "query_slots", threading.BoundedSemaphore(2)), \
                patch.object(app.subprocess, "run", side_effect=fake_run):
            threads = [threading.Thread(target=run_query) for _ in range(2)]
            for thread in threads:
                thread.start()
            deadline = time.monotonic() + 2
            while app.active_queries < 2 and time.monotonic() < deadline:
                time.sleep(0.01)
            self.assertEqual(app.active_queries, 2)
            release.set()
            for thread in threads:
                thread.join(timeout=2)

        self.assertEqual(failures, [])
        self.assertEqual(app.active_queries, 0)

    def test_pending_import_rejects_new_queries(self):
        app.pending_imports = 1

        with self.assertRaises(app.HTTPException) as ctx:
            app.begin_query_process()

        self.assertEqual(ctx.exception.status_code, 503)
        self.assertEqual(app.active_queries, 0)


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
