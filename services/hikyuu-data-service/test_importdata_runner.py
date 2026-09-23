import os
import unittest
from configparser import ConfigParser
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import importdata_runner


class ImportDataRunnerTest(unittest.TestCase):
    def test_configured_hosts_uses_environment_override(self):
        with patch.dict(os.environ, {"HIKYUU_TDX_HOSTS": "127.0.0.1:7709,10.0.0.2:7710"}):
            self.assertEqual(
                importdata_runner.configured_hosts(),
                [("127.0.0.1", 7709), ("10.0.0.2", 7710)],
            )

    def test_configured_hosts_uses_defaults_for_empty_value(self):
        with patch.dict(os.environ, {"HIKYUU_TDX_HOSTS": "", "HIKYUU_TDX_EXCLUDED_HOSTS": ""}):
            self.assertEqual(
                importdata_runner.configured_hosts(),
                list(importdata_runner.DEFAULT_TDX_HOSTS),
            )

    def test_configured_hosts_excludes_failed_nodes(self):
        with patch.dict(os.environ, {
            "HIKYUU_TDX_HOSTS": "117.34.114.15:7709,117.34.114.27:7709",
            "HIKYUU_TDX_EXCLUDED_HOSTS": "117.34.114.15",
        }):
            self.assertEqual(
                importdata_runner.configured_hosts(),
                [("117.34.114.27", 7709)],
            )

    def test_install_hdf5_write_lock_wraps_import_task_once(self):
        calls = []

        class FakeTask:
            def __init__(self):
                self.config = ConfigParser()
                self.config["hdf5"] = {"enable": "True"}
                self.market = "SH"
                self.ktype = "DAY"

            def __call__(self):
                calls.append("called")
                return 7

        module = SimpleNamespace(ImportPytdxToH5=FakeTask)
        with patch.dict(os.environ, {"HIKYUU_HDF5_LOCK_FILE": "/tmp/hikyuu-test-writer.lock"}):
            importdata_runner.install_hdf5_write_lock(module)
            wrapped = FakeTask.__call__
            importdata_runner.install_hdf5_write_lock(module)
            self.assertIs(FakeTask.__call__, wrapped)
            self.assertEqual(FakeTask()(), 7)

        self.assertEqual(calls, ["called"])

    def test_install_sqlite_busy_timeout_wraps_create_database_once(self):
        statements = []
        calls = []

        class FakeConnect:
            def execute(self, sql):
                statements.append(sql)

        def create_database(connect):
            calls.append(connect)
            return "created"

        module = SimpleNamespace(sqlite_create_database=create_database)
        importdata_runner.install_sqlite_busy_timeout(module)
        wrapped = module.sqlite_create_database

        self.assertEqual(wrapped(FakeConnect()), "created")
        importdata_runner.install_sqlite_busy_timeout(module)
        self.assertIs(module.sqlite_create_database, wrapped)
        self.assertEqual(len(calls), 1)
        self.assertTrue(any("busy_timeout" in item for item in statements))

    def test_resolve_import_config_path_prefers_environment_override(self):
        with patch.dict(os.environ, {"HIKYUU_IMPORT_CONFIG": "/tmp/custom.ini"}):
            self.assertEqual(importdata_runner.resolve_import_config_path(), Path("/tmp/custom.ini"))

    def test_resolve_import_config_path_falls_back_to_config_dir(self):
        with patch.dict(os.environ, {"HIKYUU_CONFIG_DIR": "/tmp/hk", "HIKYUU_IMPORT_CONFIG": ""}):
            self.assertEqual(
                importdata_runner.resolve_import_config_path(),
                Path("/tmp/hk/importdata-gui.ini"),
            )


if __name__ == "__main__":
    unittest.main()
