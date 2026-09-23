import os
import sys
import time
import fcntl
import functools
from concurrent import futures
from configparser import ConfigParser
from pathlib import Path


DEFAULT_TDX_HOSTS = (
    ("117.34.114.27", 7709),
    ("117.34.114.14", 7709),
    ("117.34.114.16", 7709),
    ("117.34.114.20", 7709),
    ("117.34.114.17", 7709),
    ("124.71.187.122", 7709),
    ("218.106.92.182", 7709),
    ("220.178.55.71", 7709),
    ("117.34.114.15", 7709),
)


def parse_host_list(raw):
    result = []
    for item in raw.split(","):
        host, _, port = item.strip().partition(":")
        if host:
            result.append((host, int(port or "7709")))
    return result


def configured_hosts():
    raw = os.getenv("HIKYUU_TDX_HOSTS", "").strip()
    hosts = parse_host_list(raw) if raw else list(DEFAULT_TDX_HOSTS)
    excluded = {host for host, _ in parse_host_list(os.getenv("HIKYUU_TDX_EXCLUDED_HOSTS", ""))}
    return [endpoint for endpoint in hosts if endpoint[0] not in excluded]


def search_reliable_tdx_hosts():
    from pytdx.hq import TdxHq_API

    def probe(endpoint):
        host, port = endpoint
        api = TdxHq_API(multithread=False, heartbeat=True, auto_retry=True, raise_exception=False)
        started = time.monotonic()
        try:
            if api.connect(host, port, time_out=3):
                bars = api.get_security_bars(9, 0, "159915", 0, 1)
                if bars:
                    return True, time.monotonic() - started, host, port
        except Exception:
            pass
        finally:
            try:
                api.disconnect()
            except Exception:
                pass
        return False, time.monotonic() - started, host, port

    endpoints = configured_hosts()
    if not endpoints:
        return []
    with futures.ThreadPoolExecutor(max_workers=len(endpoints)) as executor:
        results = list(executor.map(probe, endpoints))
    return sorted((item for item in results if item[0]), key=lambda item: item[1])


def tdx_security_list(api, market):
    from hikyuu.data.common_pytdx import to_pytdx_market

    pytdx_market = to_pytdx_market(market)
    total = api.get_security_count(pytdx_market) or 0
    result = []
    for offset in range(0, total, 1000):
        try:
            rows = api.get_security_list(pytdx_market, offset) or []
        except Exception as exc:
            print(f"TDX security list page skipped: {market} offset={offset}: {exc}", flush=True)
            continue
        result.extend({"code": str(row["code"]), "name": str(row["name"])} for row in rows)
    return result


def install_import_fallbacks(import_module):
    from hikyuu.data.common_sqlite3 import get_codepre_list, get_marketid

    original_import_stock_name = import_module.sqlite_import_stock_name

    def import_stock_name(connect, api, market, quotations=None):
        count = original_import_stock_name(connect, api, market, quotations)
        marketid = get_marketid(connect, market)
        existing = connect.execute(
            "select count(*) from Stock where marketid=? and type<>?",
            (marketid, import_module.STOCKTYPE.INDEX),
        ).fetchone()[0]
        if count > 0 or existing > 100:
            return count

        rows = tdx_security_list(api, market)
        code_rules = get_codepre_list(connect, marketid, quotations)
        today = int(time.strftime("%Y%m%d"))
        inserted = 0
        for row in rows:
            code = row["code"]
            stock_type = next((kind for prefix, kind in code_rules if code.startswith(prefix)), None)
            if stock_type is None:
                continue
            cursor = connect.execute(
                "insert or ignore into Stock(marketid, code, name, type, valid, startDate, endDate) values (?, ?, ?, ?, 1, ?, 99999999)",
                (marketid, code, row["name"], stock_type, today),
            )
            inserted += cursor.rowcount
        connect.commit()
        print(f"TDX security list fallback: {market} inserted={inserted}", flush=True)
        return inserted

    import_module.search_best_tdx = search_reliable_tdx_hosts
    import_module.sqlite_import_stock_name = import_stock_name


def install_hdf5_write_lock(import_module):
    """Serialize HDF5 import tasks across all forked importer processes."""
    task_class = import_module.ImportPytdxToH5
    if getattr(task_class, "_tdx_hdf5_lock_installed", False):
        return

    original_call = task_class.__call__

    @functools.wraps(original_call)
    def locked_call(task, *args, **kwargs):
        config = getattr(task, "config", None)
        if config is not None and not config.getboolean("hdf5", "enable", fallback=True):
            return original_call(task, *args, **kwargs)

        stocks_dir = Path(os.getenv("HIKYUU_STOCKS_DIR", "/root/stocks"))
        lock_path = Path(os.getenv("HIKYUU_HDF5_LOCK_FILE", str(stocks_dir / ".hdf5-writer.lock")))
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        task_label = f"{getattr(task, 'market', '?')} {getattr(task, 'ktype', '?')}"
        with lock_path.open("a+", encoding="utf-8") as lock_file:
            print(f"HDF5 write lock waiting: {task_label}", flush=True)
            fcntl.flock(lock_file.fileno(), fcntl.LOCK_EX)
            try:
                print(f"HDF5 write lock acquired: {task_label}", flush=True)
                return original_call(task, *args, **kwargs)
            finally:
                fcntl.flock(lock_file.fileno(), fcntl.LOCK_UN)
                print(f"HDF5 write lock released: {task_label}", flush=True)

    task_class.__call__ = locked_call
    task_class._tdx_hdf5_lock_installed = True


def install_sqlite_busy_timeout(import_module, timeout_ms=None):
    """Raise the code-table connection busy timeout so parallel syncs wait instead of failing."""
    if timeout_ms is None:
        timeout_ms = int(os.getenv("HIKYUU_SQLITE_BUSY_TIMEOUT_MS", "600000"))
    original = getattr(import_module, "sqlite_create_database", None)
    if original is None or getattr(original, "_tdx_busy_timeout", False):
        return

    @functools.wraps(original)
    def create_database_with_timeout(connect):
        try:
            connect.execute(f"PRAGMA busy_timeout={int(timeout_ms)}")
        except Exception:
            pass
        return original(connect)

    create_database_with_timeout._tdx_busy_timeout = True
    import_module.sqlite_create_database = create_database_with_timeout


def resolve_import_config_path() -> Path:
    config_dir = Path(os.getenv("HIKYUU_CONFIG_DIR", "/root/.hikyuu"))
    override = os.getenv("HIKYUU_IMPORT_CONFIG", "").strip()
    return Path(override) if override else config_dir / "importdata-gui.ini"


def main() -> int:
    config_file = resolve_import_config_path()
    if not config_file.exists():
        print(f"missing config file: {config_file}", flush=True)
        return 2

    try:
        import hikyuu.gui.data.UsePytdxImportToH5Thread as import_module
        from hikyuu.gui.data.UsePytdxImportToH5Thread import UsePytdxImportToH5Thread
    except Exception as exc:
        print(f"failed to import hikyuu: {exc}", flush=True)
        return 3

    config = ConfigParser()
    config.read(config_file, encoding="utf-8")

    install_import_fallbacks(import_module)
    install_hdf5_write_lock(import_module)
    install_sqlite_busy_timeout(import_module)

    runner = UsePytdxImportToH5Thread(None, config)

    def on_message(msg):
        print(msg, flush=True)

    runner.message.connect(on_message)
    runner.run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
