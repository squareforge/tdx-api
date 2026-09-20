import copy
import ast
import io
import json
import os
import subprocess
import threading
import time
import uuid
import hashlib
import sqlite3
from configparser import ConfigParser
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.cron import CronTrigger
from fastapi import FastAPI, HTTPException, Query
from pydantic import BaseModel, Field
from zoneinfo import ZoneInfo


APP_NAME = "hikyuu-data-service"
TZ_NAME = os.getenv("TZ", "Asia/Shanghai")
TZ = ZoneInfo(TZ_NAME)
STOCKS_DIR = Path(os.getenv("HIKYUU_STOCKS_DIR", "/root/stocks"))
CONFIG_DIR = Path(os.getenv("HIKYUU_CONFIG_DIR", "/root/.hikyuu"))
LOG_DIR = Path(os.getenv("HIKYUU_LOG_DIR", "/app/logs"))
PYTHON_BIN = os.getenv("HIKYUU_PYTHON", "python3")
IMPORT_SCRIPT = os.getenv("HIKYUU_IMPORT_SCRIPT", "/app/importdata_runner.py")
FRESHNESS_SCRIPT = os.getenv("HIKYUU_FRESHNESS_SCRIPT", "/app/freshness_runner.py")
QUERY_SCRIPT = os.getenv("HIKYUU_QUERY_SCRIPT", "/app/query_runner.py")
RESEARCH_SCRIPT = os.getenv("HIKYUU_RESEARCH_SCRIPT", "/app/research_runner.py")
QUERY_TIMEOUT_SECONDS = int(os.getenv("HIKYUU_QUERY_TIMEOUT_SECONDS", "120"))
FRESHNESS_MIN_COVERAGE = float(os.getenv("HIKYUU_FRESHNESS_MIN_COVERAGE", "0.95"))
SCHEDULER_ENABLED = os.getenv("HIKYUU_SCHEDULER_ENABLED", "true").lower() in ("1", "true", "yes", "on")
AFTER_CLOSE_CRON = os.getenv("HIKYUU_AFTER_CLOSE_CRON", "30 16 * * 1-5")
MAX_TASKS = int(os.getenv("HIKYUU_MAX_TASKS", "200"))
MAX_CONCURRENT_TASKS = int(os.getenv("HIKYUU_MAX_CONCURRENT_TASKS", "2"))
MAX_CONCURRENT_QUERIES = max(1, int(os.getenv("HIKYUU_MAX_CONCURRENT_QUERIES", "4")))
DATA_REVISION_FILE = STOCKS_DIR / ".data-revision"
SYNC_SCOPE_KEYS = ("day", "min", "min5", "trans", "time", "finance", "block", "weight")
SYNC_SCOPE_LABELS = {
    "day": "日线",
    "min": "1分钟线",
    "min5": "5分钟线",
    "trans": "分笔",
    "time": "分时",
    "finance": "财务",
    "block": "板块",
    "weight": "权重",
}
INDICATOR_CACHE_SECONDS = int(os.getenv("HIKYUU_INDICATOR_CACHE_SECONDS", "60"))
indicator_cache: Dict[str, Any] = {}
indicator_cache_lock = threading.Lock()


class SyncRequest(BaseModel):
    day: bool = True
    min: bool = True
    min5: bool = True
    trans: bool = False
    time: bool = False
    stock: bool = True
    fund: bool = True
    weight: bool = True
    finance: bool = True
    block: bool = True
    day_start_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")
    min_start_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")
    min5_start_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")
    trans_start_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")
    time_start_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")
    use_tdx_number: int = Field(default=10, ge=1, le=32)
    expected_date: Optional[str] = Field(default=None, pattern=r"^\d{4}-\d{2}-\d{2}$")


class KlineBatchRequest(BaseModel):
    symbols: List[str]
    period: str = "day"
    start: str = ""
    end: str = ""
    limit: int = Field(default=260, ge=1, le=2000)
    recover: str = "qfq"


class TaskRecord(BaseModel):
    id: str
    type: str
    status: str
    started_at: str
    ended_at: Optional[str] = None
    exit_code: Optional[int] = None
    error: Optional[str] = None
    log_file: str
    request: Dict[str, Any]


app = FastAPI(title=APP_NAME)
tasks: Dict[str, Dict[str, Any]] = {}
task_order: List[str] = []
task_lock = threading.Lock()
query_slots = threading.BoundedSemaphore(MAX_CONCURRENT_QUERIES)
data_state = threading.Condition()
active_imports = 0
pending_imports = 0
active_queries = 0
scheduler: Optional[BackgroundScheduler] = None


def now_text() -> str:
    return datetime.now(TZ).isoformat()


def bool_text(value: bool) -> str:
    return "True" if value else "False"


def ensure_dirs() -> None:
    STOCKS_DIR.mkdir(parents=True, exist_ok=True)
    (STOCKS_DIR / "tmp").mkdir(parents=True, exist_ok=True)
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    LOG_DIR.mkdir(parents=True, exist_ok=True)


def write_text_atomic(path: Path, content: str) -> None:
    temp_path = path.with_name(f".{path.name}.{uuid.uuid4().hex[:8]}.tmp")
    temp_path.write_text(content, encoding="utf-8")
    os.replace(temp_path, path)


def hikyuu_version() -> str:
    try:
        from importlib.metadata import version
        return version("hikyuu")
    except Exception:
        return "unknown"


def data_revision() -> str:
    if DATA_REVISION_FILE.exists():
        try:
            value = DATA_REVISION_FILE.read_text(encoding="utf-8").strip()
            if value:
                return value
        except OSError:
            pass
    try:
        latest = max((item.stat().st_mtime_ns for item in STOCKS_DIR.glob("*")), default=0)
    except OSError:
        latest = 0
    return hashlib.sha1(f"{STOCKS_DIR}:{latest}".encode()).hexdigest()[:12]


def dataset_metadata() -> Dict[str, Any]:
    files: List[Dict[str, Any]] = []
    try:
        for item in sorted(STOCKS_DIR.glob("*.h5")) + sorted(STOCKS_DIR.glob("*.db")):
            stat = item.stat()
            files.append({"name": item.name, "bytes": stat.st_size, "modified_at": datetime.fromtimestamp(stat.st_mtime, TZ).isoformat()})
    except OSError:
        pass
    symbols = None
    stock_db = STOCKS_DIR / "stock.db"
    if stock_db.exists():
        try:
            with sqlite3.connect(stock_db, timeout=2) as conn:
                for table in ("stock", "stock_basic", "stocks"):
                    try:
                        symbols = int(conn.execute(f"SELECT COUNT(*) FROM {table}").fetchone()[0])
                        break
                    except sqlite3.Error:
                        continue
        except sqlite3.Error:
            symbols = None
    return {"data_revision": data_revision(), "stocks": {"path": str(STOCKS_DIR), "symbols": symbols, "files": files}, "hikyuu_version": hikyuu_version(), "checked_at": now_text()}


def dataset_quality(code: str = "", period: str = "day", sample: Optional[List[Dict[str, Any]]] = None) -> Dict[str, Any]:
    metadata = dataset_metadata()
    files = metadata["stocks"]["files"]
    h5_files = [item for item in files if str(item["name"]).endswith(".h5")]
    total_bytes = sum(int(item["bytes"]) for item in files)
    checks = [
        {"id": "directory", "label": "数据目录", "status": "pass" if STOCKS_DIR.exists() else "fail", "detail": str(STOCKS_DIR)},
        {"id": "hdf5", "label": "HDF5 文件", "status": "pass" if h5_files else "warn", "detail": f"{len(h5_files)} 个文件"},
        {"id": "stock_db", "label": "基础信息库", "status": "pass" if (STOCKS_DIR / "stock.db").exists() else "warn", "detail": f"{metadata['stocks']['symbols'] or 0} 个证券"},
        {"id": "size", "label": "数据体积", "status": "pass" if total_bytes > 0 else "warn", "detail": total_bytes},
        {"id": "revision", "label": "修订标识", "status": "pass" if metadata["data_revision"] else "warn", "detail": metadata["data_revision"]},
    ]
    if code:
        try:
            if sample is None:
                raise LookupError("sample data was not loaded")
            times = [str(row.get("time")) for row in sample]
            closes = [float(row.get("close") or 0) for row in sample]
            duplicate_count = len(times) - len(set(times))
            invalid_count = sum(1 for row in sample if float(row.get("high") or 0) < float(row.get("low") or 0) or float(row.get("close") or 0) <= 0)
            checks.extend([
                {"id": "sample_count", "label": "样本记录", "status": "pass" if sample else "warn", "detail": len(sample)},
                {"id": "duplicates", "label": "重复时间", "status": "pass" if duplicate_count == 0 else "fail", "detail": duplicate_count},
                {"id": "ohlc", "label": "OHLC 合法性", "status": "pass" if invalid_count == 0 else "fail", "detail": invalid_count},
                {"id": "monotonic", "label": "时间序列", "status": "pass" if times == sorted(times) and len(closes) == len(sample) else "warn", "detail": "升序" if times == sorted(times) else "需检查"},
            ])
        except Exception as exc:
            checks.append({"id": "sample_count", "label": "样本记录", "status": "warn", "detail": str(exc)})
    status = "pass" if all(item["status"] == "pass" for item in checks) else "warn"
    return {"status": status, "checks": checks, "data_revision": metadata["data_revision"], "checked_at": now_text()}


def write_hikyuu_ini() -> None:
    ensure_dirs()
    datadir = str(STOCKS_DIR)
    content = f"""
[hikyuu]
tmpdir = {datadir}/tmp
datadir = {datadir}
reload_time = 00:00
quotation_server = ipc:///tmp/hikyuu_real.ipc
lazy_preload = False

[block]
type = sqlite3
db = {datadir}/stock.db

[preload]
day = True
week = True
month = True
quarter = False
halfyear = False
year = False
min = False
min5 = False
min15 = False
min30 = False
min60 = False
hour2 = False
timeline = False
trans = False
day_max = 100000
week_max = 100000
month_max = 100000
quarter_max = 100000
halfyear_max = 100000
year_max = 100000
min_max = 100000
min5_max = 100000
min15_max = 100000
min30_max = 100000
min60_max = 100000
hour2_max = 100000
timeline_max = 100000
trans_max = 100000

[baseinfo]
type = sqlite3
db = {datadir}/stock.db

[kdata]
type = hdf5
sh_day = {datadir}/sh_day.h5
sh_min = {datadir}/sh_1min.h5
sh_min5 = {datadir}/sh_5min.h5
sz_day = {datadir}/sz_day.h5
sz_min = {datadir}/sz_1min.h5
sz_min5 = {datadir}/sz_5min.h5
bj_day = {datadir}/bj_day.h5
bj_min = {datadir}/bj_1min.h5
bj_min5 = {datadir}/bj_5min.h5
sh_time = {datadir}/sh_time.h5
sz_time = {datadir}/sz_time.h5
bj_time = {datadir}/bj_time.h5
sh_trans = {datadir}/sh_trans.h5
sz_trans = {datadir}/sz_trans.h5
bj_trans = {datadir}/bj_trans.h5
""".strip()
    write_text_atomic(CONFIG_DIR / "hikyuu.ini", content + "\n")


def build_import_config(request: SyncRequest, task_id: str) -> None:
    ensure_dirs()
    parser = ConfigParser()
    parser["quotation"] = {
        "stock": bool_text(request.stock),
        "fund": bool_text(request.fund),
        "future": "False",
    }
    parser["ktype"] = {
        "day": bool_text(request.day),
        "min": bool_text(request.min),
        "min5": bool_text(request.min5),
        "trans": bool_text(request.trans),
        "time": bool_text(request.time),
        "day_start_date": request.day_start_date or "1990-12-19",
        "min_start_date": request.min_start_date or "2023-09-19",
        "min5_start_date": request.min5_start_date or "2023-09-19",
        "trans_start_date": request.trans_start_date or "2023-12-11",
        "time_start_date": request.time_start_date or "2023-12-11",
    }
    parser["weight"] = {"enable": bool_text(request.weight)}
    parser["finance"] = {"enable": bool_text(request.finance)}
    parser["block"] = {"enable": bool_text(request.block)}
    parser["tdx"] = {"enable": "False"}
    parser["pytdx"] = {"enable": "True", "use_tdx_number": str(request.use_tdx_number)}
    parser["hdf5"] = {"enable": "True", "dir": str(STOCKS_DIR)}
    parser["mysql"] = {"enable": "False"}
    parser["clickhouse"] = {"enable": "False"}
    parser["sched"] = {"time": "16:30:00"}
    parser["collect"] = {
        "quotation_server": "ipc:///tmp/hikyuu_real.ipc",
        "interval": "305",
        "source": "qq",
        "use_zhima_proxy": "False",
        "phase1_start": "00:00:00",
        "phase1_end": "11:35:00",
        "phase2_start": "12:00:00",
        "phase2_end": "15:05:00",
    }
    parser["lazy_preload"] = {"enable": "False"}
    parser["preload"] = {
        "day": "True",
        "week": "True",
        "month": "True",
        "quarter": "False",
        "halfyear": "False",
        "year": "False",
        "min": "False",
        "min5": "False",
        "min15": "False",
        "min30": "False",
        "min60": "False",
        "hour2": "False",
        "timeline": "False",
        "trans": "False",
        "day_max": "100000",
        "week_max": "100000",
        "month_max": "100000",
        "quarter_max": "100000",
        "halfyear_max": "100000",
        "year_max": "100000",
        "min_max": "100000",
        "min5_max": "100000",
        "min15_max": "100000",
        "min30_max": "100000",
        "min60_max": "100000",
        "hour2_max": "100000",
        "timeline_max": "100000",
        "trans_max": "100000",
    }
    buffer = io.StringIO()
    parser.write(buffer)
    content = buffer.getvalue()
    # 每个任务使用独立配置文件，避免并行同步时互相覆盖；共享文件保留最近一次配置便于排查
    write_text_atomic(CONFIG_DIR / f"importdata-gui-{task_id}.ini", content)
    write_text_atomic(CONFIG_DIR / "importdata-gui.ini", content)
    write_hikyuu_ini()


def trim_tasks() -> None:
    while len(task_order) > MAX_TASKS:
        old_id = task_order.pop(0)
        tasks.pop(old_id, None)


def request_dict(request: SyncRequest) -> Dict[str, Any]:
    if hasattr(request, "model_dump"):
        return request.model_dump()
    return request.dict()


def task_snapshot(task: Dict[str, Any]) -> Dict[str, Any]:
    item = copy.deepcopy(task)
    log_file = Path(item["log_file"])
    if log_file.exists():
        try:
            lines = log_file.read_text(encoding="utf-8", errors="replace").splitlines()
            item["log_tail"] = lines[-80:]
            item.update(parse_task_progress(lines))
        except OSError:
            item["log_tail"] = []
            item.update({"progress": None, "stage": None, "message": None, "phases": _empty_phases()})
    else:
        item["log_tail"] = []
        item.update({"progress": None, "stage": None, "message": None, "phases": _empty_phases()})
    if item.get("status") == "failed":
        item.update({"progress": 100, "stage": "失败", "message": item.get("error") or "同步失败"})
    return item


def request_scope(request: Dict[str, Any]) -> Dict[str, bool]:
    return {key: bool(request.get(key)) for key in SYNC_SCOPE_KEYS if request.get(key)}


def scope_label(scope: Dict[str, bool]) -> str:
    labels = [SYNC_SCOPE_LABELS[key] for key in SYNC_SCOPE_KEYS if scope.get(key)]
    return "、".join(labels)


def _active_items_locked() -> List[Dict[str, Any]]:
    return [
        tasks[task_id]
        for task_id in task_order
        if tasks.get(task_id, {}).get("status") in ("pending", "running")
    ]


def active_task_items() -> List[Dict[str, Any]]:
    with task_lock:
        return _active_items_locked()


def find_scope_conflict(scope: Dict[str, bool], active_items: List[Dict[str, Any]]) -> Optional[str]:
    overlap: List[str] = []
    for item in active_items:
        running_scope = request_scope(item.get("request") or {})
        for key in SYNC_SCOPE_KEYS:
            if scope.get(key) and running_scope.get(key) and key not in overlap:
                overlap.append(key)
    if not overlap:
        return None
    return "以下数据正在其他任务中同步，请等待完成：" + "、".join(SYNC_SCOPE_LABELS[key] for key in overlap)


def begin_import_run() -> None:
    global active_imports, pending_imports
    with data_state:
        pending_imports += 1
        try:
            while active_queries > 0:
                data_state.wait()
            active_imports += 1
        finally:
            pending_imports -= 1


def end_import_run() -> None:
    global active_imports
    with data_state:
        active_imports -= 1
        data_state.notify_all()


def begin_query_process() -> None:
    global active_queries
    with data_state:
        if active_imports > 0 or pending_imports > 0:
            raise HTTPException(status_code=503, detail="hikyuu 数据同步准备中或同步中，请稍后重试")
        active_queries += 1


def end_query_process() -> None:
    global active_queries
    with data_state:
        active_queries -= 1
        data_state.notify_all()


def run_json_process(script: str, arguments: List[str]) -> Any:
    env = os.environ.copy()
    env["HOME"] = str(CONFIG_DIR.parent if CONFIG_DIR.name == ".hikyuu" else Path("/root"))
    env["HIKYUU_STOCKS_DIR"] = str(STOCKS_DIR)
    env["HIKYUU_CONFIG_DIR"] = str(CONFIG_DIR)
    env["TZ"] = TZ_NAME
    with query_slots:
        begin_query_process()
        try:
            completed = subprocess.run(
                [PYTHON_BIN, script, *arguments],
                capture_output=True,
                text=True,
                env=env,
                timeout=QUERY_TIMEOUT_SECONDS,
                check=False,
            )
        except subprocess.TimeoutExpired as exc:
            raise HTTPException(status_code=504, detail="hikyuu 查询超时") from exc
        except OSError as exc:
            raise HTTPException(status_code=503, detail=f"hikyuu 查询进程启动失败: {exc}") from exc
        finally:
            end_query_process()

    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or "hikyuu 查询失败"
        raise HTTPException(status_code=503, detail=detail[-1000:])
    output_lines = [line.strip() for line in completed.stdout.splitlines() if line.strip()]
    if not output_lines:
        raise HTTPException(status_code=503, detail="hikyuu 查询没有返回数据")
    for line in reversed(output_lines):
        try:
            return json.loads(line)
        except json.JSONDecodeError:
            continue
    raise HTTPException(status_code=503, detail="hikyuu 查询返回格式错误")


KTYPE_TO_PHASE = {"DAY": "day", "1MIN": "min1", "5MIN": "min5"}
MARKET_COUNT = 3  # SH/SZ/BJ


def _empty_phases() -> Dict[str, Dict[str, Any]]:
    return {
        "day": {"label": "日线", "progress": None, "done": False},
        "min1": {"label": "1分钟", "progress": None, "done": False},
        "min5": {"label": "5分钟", "progress": None, "done": False},
        "finance": {"label": "财务", "progress": None, "done": False},
        "block": {"label": "板块", "progress": None, "done": False},
        "weight": {"label": "权重", "progress": None, "done": False},
    }


def parse_task_progress(lines: List[str]) -> Dict[str, Any]:
    result: Dict[str, Any] = {"progress": None, "stage": None, "message": None, "phases": _empty_phases()}
    finished_markets: Dict[str, set] = {}
    latest_progress: Optional[tuple] = None  # (phase_key, progress)

    for raw_line in lines:
        text = raw_line.strip()
        if not text or not text.startswith("["):
            continue
        try:
            event = ast.literal_eval(text)
        except (ValueError, SyntaxError):
            continue
        if not (isinstance(event, list) and event and event[0] == "HDF5_IMPORT"):
            continue
        kind = str(event[1]) if len(event) > 1 else ""

        if kind == "THREAD":
            status = str(event[2]) if len(event) > 2 else ""
            if status == "FINISHED":
                result["progress"] = 100
                result["stage"] = "完成"
                result["message"] = "同步完成"
                for phase in result["phases"].values():
                    phase["done"] = True
                    phase["progress"] = 100
            elif status == "FAILURE":
                result["progress"] = 100
                result["stage"] = "失败"
                result["message"] = str(event[3]) if len(event) > 3 else "同步失败"
            continue

        if kind == "IMPORT_KDATA":
            sub = str(event[2]) if len(event) > 2 else ""
            if sub == "FINISHED":
                market = str(event[3]) if len(event) > 3 else ""
                ktype = str(event[4]) if len(event) > 4 else ""
                phase = KTYPE_TO_PHASE.get(ktype)
                if phase and market:
                    finished_markets.setdefault(phase, set()).add(market)
                continue
            phase = KTYPE_TO_PHASE.get(sub)
            progress = event[3] if len(event) > 3 else None
            if phase and isinstance(progress, (int, float)):
                result["phases"][phase]["progress"] = int(progress)
                latest_progress = (phase, int(progress))
            continue

        if kind == "IMPORT_FINANCE":
            progress = event[2] if len(event) > 2 else None
            if isinstance(progress, (int, float)):
                result["phases"]["finance"]["progress"] = int(progress)
                latest_progress = ("finance", int(progress))
            continue

        if kind == "IMPORT_WEIGHT":
            latest_progress = ("weight", None)
            continue

        if kind == "IMPORT_BLOCKINFO":
            latest_progress = ("block", None)
            continue

    for phase, markets in finished_markets.items():
        if len(markets) >= MARKET_COUNT:
            result["phases"][phase]["done"] = True
            result["phases"][phase]["progress"] = 100

    if latest_progress is not None:
        phase_key, progress = latest_progress
        label = result["phases"][phase_key]["label"]
        result["stage"] = label
        if progress is not None:
            result["progress"] = progress
            result["message"] = f"{label} {progress}%"
        else:
            result["message"] = f"{label} 同步中"

    return result


IMPORT_FAILURE_MARKERS = (
    "'HDF5_IMPORT', 'THREAD', 'FAILURE'",
    "ImportPytdxToH5Task failed!",
    "Unable to open/create file",
    "All process is finished, but some tasks are running!",
    "连续失败20个股票",
    "已停止导入",
    "停止导入",
)


def detect_import_failure(log_text: str) -> Optional[str]:
    for line in log_text.splitlines():
        if any(marker in line for marker in IMPORT_FAILURE_MARKERS):
            return line.strip()[-1000:] or "hikyuu import reported failure"
    return None


def parse_json_output(output: str) -> Any:
    for line in reversed([line.strip() for line in output.splitlines() if line.strip()]):
        try:
            return json.loads(line)
        except json.JSONDecodeError:
            continue
    raise RuntimeError("hikyuu freshness check returned invalid JSON")


def run_freshness_check(expected_date: str, env: Dict[str, str]) -> Dict[str, Any]:
    try:
        completed = subprocess.run(
            [
                PYTHON_BIN,
                FRESHNESS_SCRIPT,
                "--expected-date",
                expected_date,
                "--min-coverage",
                str(FRESHNESS_MIN_COVERAGE),
            ],
            capture_output=True,
            text=True,
            env=env,
            timeout=QUERY_TIMEOUT_SECONDS,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError("hikyuu freshness check timed out") from exc
    if completed.returncode != 0:
        detail = completed.stderr.strip() or completed.stdout.strip() or "unknown error"
        raise RuntimeError(f"hikyuu freshness check failed: {detail[-1000:]}")
    result = parse_json_output(completed.stdout)
    if not isinstance(result, dict):
        raise RuntimeError("hikyuu freshness check returned a non-object result")
    return result


def freshness_failure_message(result: Dict[str, Any]) -> str:
    details = []
    for market, item in result.get("markets", {}).items():
        if not item.get("ok"):
            coverage = float(item.get("coverage") or 0) * 100
            details.append(
                f"{market} latest={item.get('latest_date') or 'none'} coverage={coverage:.1f}%"
            )
    suffix = ", ".join(details) or "unknown market state"
    return f"K线完整性校验失败（目标交易日 {result.get('expected_date') or 'unknown'}）：{suffix}"


def run_task(task_id: str) -> None:
    with task_lock:
        task = tasks[task_id]
        task["status"] = "running"

    request = SyncRequest(**task["request"])
    log_path = Path(task["log_file"])
    env = os.environ.copy()
    env["HOME"] = str(CONFIG_DIR.parent if CONFIG_DIR.name == ".hikyuu" else Path("/root"))
    env["HIKYUU_STOCKS_DIR"] = str(STOCKS_DIR)
    env["HIKYUU_CONFIG_DIR"] = str(CONFIG_DIR)
    env["HIKYUU_IMPORT_CONFIG"] = str(CONFIG_DIR / f"importdata-gui-{task_id}.ini")
    env["TZ"] = TZ_NAME

    begin_import_run()
    try:
        build_import_config(request, task_id)
        with log_path.open("a", encoding="utf-8") as log:
            log.write(f"[{now_text()}] task {task_id} started: {task['type']} · {scope_label(request_scope(task['request']))}\n")
            log.flush()
            proc = subprocess.Popen(
                [PYTHON_BIN, IMPORT_SCRIPT],
                stdout=log,
                stderr=subprocess.STDOUT,
                env=env,
                text=True,
            )
            exit_code = proc.wait()
            log.flush()
            try:
                log_text = log_path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                log_text = ""
            failure_error = detect_import_failure(log_text)
            freshness = None
            task_error = failure_error
            if exit_code == 0 and task_error is None and request.expected_date:
                try:
                    freshness = run_freshness_check(request.expected_date, env)
                    for market, item in freshness.get("markets", {}).items():
                        log.write(
                            "K线完整性校验 "
                            f"{market}: latest={item.get('latest_date')} "
                            f"fresh={item.get('fresh_count')}/{item.get('total')} "
                            f"coverage={float(item.get('coverage') or 0) * 100:.1f}% "
                            f"ok={item.get('ok')}\n"
                        )
                    log.flush()
                    if not freshness.get("ok"):
                        task_error = freshness_failure_message(freshness)
                except Exception as exc:
                    task_error = str(exc)
            with task_lock:
                task["exit_code"] = exit_code
                task["ended_at"] = now_text()
                task["freshness"] = freshness
                if exit_code == 0 and task_error is None:
                    task["status"] = "success"
                    try:
                        write_text_atomic(DATA_REVISION_FILE, f"{uuid.uuid4().hex[:12]}\n")
                    except OSError:
                        pass
                else:
                    task["status"] = "failed"
                    task["error"] = task_error or f"import process exited with code {exit_code}"
                final_status = task["status"]
            log.write(f"[{now_text()}] task {task_id} ended: status={final_status} exit_code={exit_code}\n")
    except Exception as exc:
        with task_lock:
            task["status"] = "failed"
            task["error"] = str(exc)
            task["ended_at"] = now_text()
    finally:
        end_import_run()


def create_task(task_type: str, request: SyncRequest) -> Dict[str, Any]:
    payload = request_dict(request)
    scope = request_scope(payload)
    if not scope:
        raise HTTPException(status_code=400, detail="未选择同步内容")

    with task_lock:
        active_items = _active_items_locked()
        if len(active_items) >= MAX_CONCURRENT_TASKS:
            raise HTTPException(
                status_code=409,
                detail=f"并发同步任务已达上限（{MAX_CONCURRENT_TASKS}），请等待现有任务完成",
            )
        conflict = find_scope_conflict(scope, active_items)
        if conflict:
            raise HTTPException(status_code=409, detail=conflict)
        # 代码表导入由已有任务负责，并行任务跳过，避免重复写入 stock.db
        if any((item.get("request") or {}).get("stock") for item in active_items):
            payload["stock"] = False
        if any((item.get("request") or {}).get("fund") for item in active_items):
            payload["fund"] = False

        task_id = str(uuid.uuid4())
        log_file = LOG_DIR / f"{task_id}.log"
        task = {
            "id": task_id,
            "type": task_type,
            "status": "pending",
            "started_at": now_text(),
            "ended_at": None,
            "exit_code": None,
            "error": None,
            "log_file": str(log_file),
            "request": payload,
        }
        tasks[task_id] = task
        task_order.append(task_id)
        trim_tasks()

    thread = threading.Thread(target=run_task, args=(task_id,), daemon=True)
    thread.start()
    return task


def scheduled_after_close_sync() -> None:
    request = SyncRequest()
    try:
        create_task("after_close_sync", request)
    except HTTPException:
        return


@app.on_event("startup")
def startup() -> None:
    global scheduler
    ensure_dirs()
    write_hikyuu_ini()
    if SCHEDULER_ENABLED:
        scheduler = BackgroundScheduler(timezone=TZ)
        scheduler.add_job(
            scheduled_after_close_sync,
            CronTrigger.from_crontab(AFTER_CLOSE_CRON, timezone=TZ),
            id="after-close-sync",
            name="after-close-sync",
            replace_existing=True,
        )
        scheduler.start()


@app.on_event("shutdown")
def shutdown() -> None:
    if scheduler is not None:
        scheduler.shutdown(wait=False)


@app.get("/api/hikyuu/kline")
def query_kline(
    code: str = Query(..., min_length=1),
    kline_type: str = Query("day", alias="type"),
    start: str = "",
    end: str = "",
    limit: int = Query(0, ge=0, le=100000),
    recover: str = "none",
) -> Dict[str, Any]:
    ensure_dirs()
    if not (CONFIG_DIR / "hikyuu.ini").exists():
        write_hikyuu_ini()
    data = run_json_process(QUERY_SCRIPT, [
        "--symbol",
        code.strip(),
        "--period",
        kline_type.strip().lower() or "day",
        "--start",
        start.strip(),
        "--end",
        end.strip(),
        "--limit",
        str(limit),
        "--recover",
        recover.strip().lower() or "none",
    ])
    return {"code": 0, "message": "success", "data": data}


@app.post("/api/hikyuu/kline/batch")
def query_kline_batch(request: KlineBatchRequest) -> Dict[str, Any]:
    symbols = list(dict.fromkeys(symbol.strip() for symbol in request.symbols if symbol.strip()))
    if not symbols:
        raise HTTPException(status_code=400, detail="symbols must not be empty")
    if len(symbols) > 128:
        raise HTTPException(status_code=400, detail="a batch supports at most 128 symbols")
    ensure_dirs()
    if not (CONFIG_DIR / "hikyuu.ini").exists():
        write_hikyuu_ini()
    data = run_json_process(QUERY_SCRIPT, [
        "--symbols-json", json.dumps(symbols, ensure_ascii=False),
        "--period", request.period.strip().lower() or "day",
        "--start", request.start.strip(),
        "--end", request.end.strip(),
        "--limit", str(request.limit),
        "--recover", request.recover.strip().lower() or "qfq",
    ])
    return {"code": 0, "message": "success", "data": data}


@app.get("/api/hikyuu/metadata")
def metadata() -> Dict[str, Any]:
    ensure_dirs()
    return {"code": 0, "message": "success", "data": dataset_metadata()}


@app.get("/api/hikyuu/quality")
def quality(code: str = Query(""), period: str = Query("day")) -> Dict[str, Any]:
    ensure_dirs()
    normalized_code = code.strip()
    normalized_period = period.strip().lower() or "day"
    sample = None
    if normalized_code:
        sample = run_json_process(QUERY_SCRIPT, [
            "--symbol", normalized_code,
            "--period", normalized_period,
            "--limit", "2000",
            "--recover", "none",
        ])["list"]
    return {"code": 0, "message": "success", "data": dataset_quality(normalized_code, normalized_period, sample)}


@app.get("/api/hikyuu/freshness")
def freshness(expected_date: str = Query(..., pattern=r"^\d{4}-\d{2}-\d{2}$")) -> Dict[str, Any]:
    ensure_dirs()
    if not (CONFIG_DIR / "hikyuu.ini").exists():
        write_hikyuu_ini()
    data = run_json_process(FRESHNESS_SCRIPT, [
        "--expected-date", expected_date,
        "--min-coverage", str(FRESHNESS_MIN_COVERAGE),
    ])
    return {"code": 0, "message": "success", "data": data}


@app.post("/api/hikyuu/indicators")
def indicators(payload: Dict[str, Any]) -> Dict[str, Any]:
    cache_key = json.dumps(payload, sort_keys=True, ensure_ascii=False, default=str)
    current_time = time.monotonic()
    with indicator_cache_lock:
        cached = indicator_cache.get(cache_key)
        if cached and current_time - cached["created_at"] < INDICATOR_CACHE_SECONDS:
            return {"code": 0, "message": "success", "data": cached["data"]}
    result = run_json_process(RESEARCH_SCRIPT, [
        "--operation", "indicator",
        "--payload-json", json.dumps(payload, ensure_ascii=False),
    ])
    with indicator_cache_lock:
        indicator_cache[cache_key] = {"created_at": current_time, "data": result}
        if len(indicator_cache) > 128:
            oldest = min(indicator_cache, key=lambda key: indicator_cache[key]["created_at"])
            indicator_cache.pop(oldest, None)
    return {"code": 0, "message": "success", "data": result}


@app.post("/api/hikyuu/backtest")
def backtest(payload: Dict[str, Any]) -> Dict[str, Any]:
    result = run_json_process(RESEARCH_SCRIPT, [
        "--operation", "backtest",
        "--payload-json", json.dumps(payload, ensure_ascii=False),
    ])
    return {"code": 0, "message": "success", "data": result}


@app.get("/api/hikyuu/health")
def health() -> Dict[str, Any]:
    active_ids = [item["id"] for item in active_task_items()]
    return {
        "code": 0,
        "message": "success",
        "data": {
            "service": APP_NAME,
            "time": now_text(),
            "stocks_dir": str(STOCKS_DIR),
            "config_dir": str(CONFIG_DIR),
            "log_dir": str(LOG_DIR),
            "scheduler_enabled": SCHEDULER_ENABLED,
            "after_close_cron": AFTER_CLOSE_CRON,
            "active_task_id": active_ids[0] if active_ids else None,
            "active_task_ids": active_ids,
            "hikyuu_version": hikyuu_version(),
            "data_revision": data_revision(),
            "quality_status": dataset_quality()["status"],
        },
    }


@app.post("/api/hikyuu/tasks/full-sync")
def full_sync(request: SyncRequest = SyncRequest()) -> Dict[str, Any]:
    task = create_task("full_sync", request)
    return {"code": 0, "message": "success", "data": task_snapshot(task)}


@app.post("/api/hikyuu/tasks/after-close-sync")
def after_close_sync(request: SyncRequest = SyncRequest()) -> Dict[str, Any]:
    task = create_task("after_close_sync", request)
    return {"code": 0, "message": "success", "data": task_snapshot(task)}


@app.get("/api/hikyuu/tasks")
def list_tasks() -> Dict[str, Any]:
    with task_lock:
        items = [task_snapshot(tasks[task_id]) for task_id in reversed(task_order)]
    return {"code": 0, "message": "success", "data": items}


@app.get("/api/hikyuu/tasks/{task_id}")
def get_task(task_id: str) -> Dict[str, Any]:
    with task_lock:
        task = tasks.get(task_id)
        if task is None:
            raise HTTPException(status_code=404, detail="task not found")
        item = task_snapshot(task)
    return {"code": 0, "message": "success", "data": item}


@app.get("/api/hikyuu/config")
def get_config() -> Dict[str, Any]:
    paths = {
        "hikyuu_ini": str(CONFIG_DIR / "hikyuu.ini"),
        "importdata_gui_ini": str(CONFIG_DIR / "importdata-gui.ini"),
    }
    return {"code": 0, "message": "success", "data": paths}


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8091")))
