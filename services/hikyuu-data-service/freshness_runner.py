import argparse
import contextlib
import io
import json
import os
import sys
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Iterable
from zoneinfo import ZoneInfo


TZ_NAME = os.getenv("TZ", "Asia/Shanghai")
TZ = ZoneInfo(TZ_NAME)
CONFIG_FILE = Path(os.getenv("HIKYUU_CONFIG_DIR", "/root/.hikyuu")) / "hikyuu.ini"
MARKET_STOCK_TYPES = {"SH": {1}, "SZ": {1}, "BJ": {11}}
DEFAULT_MIN_COVERAGE = float(os.getenv("HIKYUU_FRESHNESS_MIN_COVERAGE", "0.95"))


def parse_expected_date(value: str) -> str:
    return datetime.strptime(value, "%Y-%m-%d").strftime("%Y-%m-%d")


def kline_date_text(value: Any) -> str:
    if hasattr(value, "datetime"):
        value = value.datetime()
    if not isinstance(value, datetime):
        raise TypeError(f"unsupported datetime value: {type(value).__name__}")
    return value.strftime("%Y-%m-%d")


def scan_market_freshness(
    stocks: Iterable[Any],
    query_factory: Any,
    expected_date: str,
    min_coverage: float = DEFAULT_MIN_COVERAGE,
) -> Dict[str, Any]:
    expected_date = parse_expected_date(expected_date)
    if not 0 < min_coverage <= 1:
        raise ValueError("minimum coverage must be greater than 0 and at most 1")

    state: Dict[str, Dict[str, Any]] = {
        market: {"total": 0, "missing_count": 0, "dates": Counter(), "errors": []}
        for market in MARKET_STOCK_TYPES
    }
    query = query_factory(-1, ktype=query_factory.DAY)
    for stock in stocks:
        market = str(getattr(stock, "market", "")).upper()
        if market not in MARKET_STOCK_TYPES:
            continue
        if not bool(getattr(stock, "valid", False)):
            continue
        if int(getattr(stock, "type", -1)) not in MARKET_STOCK_TYPES[market]:
            continue

        market_state = state[market]
        market_state["total"] += 1
        try:
            records = stock.get_kdata(query)
            if not records:
                market_state["missing_count"] += 1
                continue
            latest_date = kline_date_text(records[-1].datetime)
            market_state["dates"][latest_date] += 1
        except Exception as exc:
            market_state["missing_count"] += 1
            if len(market_state["errors"]) < 10:
                code = str(getattr(stock, "market_code", getattr(stock, "code", "unknown")))
                market_state["errors"].append(f"{code}: {exc}")

    markets: Dict[str, Dict[str, Any]] = {}
    for market, market_state in state.items():
        total = int(market_state["total"])
        missing_count = int(market_state["missing_count"])
        dates = market_state["dates"]
        fresh_count = sum(count for date, count in dates.items() if date >= expected_date)
        stale_count = max(0, total - fresh_count - missing_count)
        latest_date = max(dates, default=None)
        coverage = fresh_count / total if total else 0.0
        ok = bool(total and latest_date and latest_date >= expected_date and coverage >= min_coverage)
        markets[market] = {
            "total": total,
            "fresh_count": fresh_count,
            "stale_count": stale_count,
            "missing_count": missing_count,
            "latest_date": latest_date,
            "coverage": round(coverage, 6),
            "date_distribution": dict(sorted(dates.items(), reverse=True)),
            "errors": market_state["errors"],
            "ok": ok,
        }

    return {
        "expected_date": expected_date,
        "minimum_coverage": min_coverage,
        "ok": all(item["ok"] for item in markets.values()),
        "markets": markets,
        "checked_at": datetime.now(TZ).isoformat(),
    }


def load_freshness(expected_date: str, min_coverage: float) -> Dict[str, Any]:
    if not CONFIG_FILE.exists():
        raise FileNotFoundError(f"missing hikyuu config: {CONFIG_FILE}")

    native_output = io.StringIO()
    with contextlib.redirect_stdout(native_output), contextlib.redirect_stderr(native_output):
        from hikyuu import Query, StockManager, hikyuu_init

        hikyuu_init(str(CONFIG_FILE), ignore_preload=True)
        result = scan_market_freshness(StockManager.instance(), Query, expected_date, min_coverage)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--expected-date", required=True)
    parser.add_argument("--min-coverage", type=float, default=DEFAULT_MIN_COVERAGE)
    args = parser.parse_args()
    try:
        result = load_freshness(args.expected_date, args.min_coverage)
    except Exception as exc:
        print(str(exc), file=sys.stderr, flush=True)
        return 1
    print(json.dumps(result, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
