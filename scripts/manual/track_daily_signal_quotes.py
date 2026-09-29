#!/usr/bin/env python3
"""Track intraday quote performance for daily v3 signal candidates."""

from __future__ import annotations

import argparse
import csv
import json
import sys
import urllib.parse
from pathlib import Path
from typing import Any

import pandas as pd
import requests


BASE_URL = "http://localhost:8080"
SNAPSHOT_COLUMNS = [
    "date",
    "time_label",
    "quote_time",
    "code",
    "name",
    "signal_date",
    "signal_close",
    "buy_price_low",
    "buy_price_high",
    "prev_close",
    "open",
    "high",
    "low",
    "latest_price",
    "change_pct",
    "open_change_pct",
    "max_gain_pct",
    "max_drawdown_pct",
    "amount",
    "volume",
    "buy_range_status",
]


def request_json(path: str, base_url: str = BASE_URL, timeout: int = 20) -> dict[str, Any]:
    url = urllib.parse.urljoin(base_url.rstrip("/") + "/", path.lstrip("/"))
    resp = requests.get(url, timeout=timeout)
    resp.raise_for_status()
    return resp.json()


def price_scale(k: dict[str, Any]) -> float:
    values = [abs(float(k.get(name, 0) or 0)) for name in ("Last", "Open", "High", "Low", "Close")]
    values = [x for x in values if x > 0]
    if not values:
        return 1.0
    median = sorted(values)[len(values) // 2]
    return 1000.0 if median > 1000 else 1.0


def pct(numerator: float, denominator: float) -> float:
    if denominator <= 0:
        return 0.0
    return numerator / denominator - 1


def buy_range_status(low: float, high: float, buy_low: float, buy_high: float, latest: float) -> str:
    touched = high >= buy_low and low <= buy_high
    if touched:
        if buy_low <= latest <= buy_high:
            return "in_range"
        if latest < buy_low:
            return "touched_below"
        return "touched_above"
    if latest < buy_low:
        return "below_range"
    return "above_range"


def load_signals(path: Path) -> pd.DataFrame:
    if not path.exists():
        return pd.DataFrame()
    df = pd.read_csv(path, dtype={"code": str}, encoding="utf-8-sig")
    if "code" in df.columns:
        df["code"] = df["code"].astype(str).str.zfill(6)
    return df


def fetch_quotes(codes: list[str], base_url: str) -> list[dict[str, Any]]:
    if not codes:
        return []
    path = f"/api/quote?code={urllib.parse.quote(','.join(codes))}"
    payload = request_json(path, base_url=base_url, timeout=30)
    if payload.get("code") != 0:
        raise RuntimeError(payload)
    return payload.get("data", [])


def fetch_quotes_resilient(codes: list[str], base_url: str, errors: list[str]) -> list[dict[str, Any]]:
    try:
        return fetch_quotes(codes, base_url)
    except Exception as exc:  # noqa: BLE001 - keep tracking useful when one batch fails.
        errors.append(f"{','.join(codes)}: {exc}")
        if len(codes) == 1:
            return []

    quotes: list[dict[str, Any]] = []
    for code in codes:
        try:
            quotes.extend(fetch_quotes([code], base_url))
        except Exception as exc:  # noqa: BLE001 - report the exact failed code.
            errors.append(f"{code}: {exc}")
    return quotes


def quote_row(signal: pd.Series, quote: dict[str, Any], run_date: str, time_label: str) -> dict[str, Any]:
    k = quote.get("K", {})
    scale = price_scale(k)
    prev_close = float(k.get("Last", 0) or 0) / scale
    open_price = float(k.get("Open", 0) or 0) / scale
    high = float(k.get("High", 0) or 0) / scale
    low = float(k.get("Low", 0) or 0) / scale
    latest = float(k.get("Close", 0) or 0) / scale
    signal_close = float(signal["close"])
    buy_low = float(signal["buy_price_low"])
    buy_high = float(signal["buy_price_high"])
    volume = float(quote.get("TotalHand", 0) or 0)
    amount = float(quote.get("Amount", 0) or 0)
    if amount <= 0 and latest > 0 and volume > 0:
        amount = latest * volume * 100

    return {
        "date": run_date,
        "time_label": time_label,
        "quote_time": str(quote.get("ServerTime", "")),
        "code": str(signal["code"]).zfill(6),
        "name": signal["name"],
        "signal_date": signal["signal_date"],
        "signal_close": round(signal_close, 3),
        "buy_price_low": round(buy_low, 3),
        "buy_price_high": round(buy_high, 3),
        "prev_close": round(prev_close, 3),
        "open": round(open_price, 3),
        "high": round(high, 3),
        "low": round(low, 3),
        "latest_price": round(latest, 3),
        "change_pct": round(pct(latest, signal_close), 6),
        "open_change_pct": round(pct(latest, open_price), 6),
        "max_gain_pct": round(pct(high, signal_close), 6),
        "max_drawdown_pct": round(pct(low, signal_close), 6),
        "amount": round(amount, 3),
        "volume": round(volume, 3),
        "buy_range_status": buy_range_status(low, high, buy_low, buy_high, latest),
    }


def write_csv(path: Path, rows: list[dict[str, Any]], columns: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8-sig") as fh:
        writer = csv.DictWriter(fh, fieldnames=columns)
        writer.writeheader()
        writer.writerows(rows)


def write_performance(reports_dir: Path, run_date: str) -> None:
    frames: list[pd.DataFrame] = []
    for path in sorted(reports_dir.glob(f"intraday_quotes_{run_date}_*.csv")):
        df = pd.read_csv(path, dtype={"code": str}, encoding="utf-8-sig")
        if not df.empty:
            frames.append(df)
    if not frames:
        return

    all_quotes = pd.concat(frames, ignore_index=True)
    all_quotes["code"] = all_quotes["code"].astype(str).str.zfill(6)
    cols = [
        "date",
        "code",
        "name",
        "signal_date",
        "signal_close",
        "buy_price_low",
        "buy_price_high",
        "time_label",
        "latest_price",
        "change_pct",
        "open_change_pct",
        "max_gain_pct",
        "max_drawdown_pct",
        "high",
        "low",
        "amount",
        "buy_range_status",
    ]
    out = all_quotes[cols].sort_values(["code", "time_label"])
    out.to_csv(reports_dir / f"signal_performance_{run_date}.csv", index=False, encoding="utf-8-sig")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default=BASE_URL)
    parser.add_argument("--date", default=pd.Timestamp.now(tz="Asia/Shanghai").strftime("%Y-%m-%d"))
    parser.add_argument("--time-label", required=True, help="Usually 1135 or 1505.")
    parser.add_argument("--reports-dir", default="reports")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    run_date = args.date
    reports_dir = Path(args.reports_dir)
    signal_path = reports_dir / f"signals_{run_date}.csv"
    snapshot_path = reports_dir / f"intraday_quotes_{run_date}_{args.time_label}.csv"
    log_path = reports_dir / "logs" / f"track_quotes_{run_date}_{args.time_label}.log"
    log_path.parent.mkdir(parents=True, exist_ok=True)

    signals = load_signals(signal_path)
    if signals.empty:
        write_csv(snapshot_path, [], SNAPSHOT_COLUMNS)
        message = f"date={run_date}, time_label={args.time_label}, signals=0, quotes=0, output={snapshot_path}"
        log_path.write_text(message + "\n", encoding="utf-8")
        print(message)
        return 0

    quote_map: dict[str, dict[str, Any]] = {}
    errors: list[str] = []
    codes = signals["code"].astype(str).str.zfill(6).tolist()
    for i in range(0, len(codes), 80):
        batch = codes[i : i + 80]
        for quote in fetch_quotes_resilient(batch, args.base_url, errors):
            quote_map[str(quote.get("Code", "")).zfill(6)] = quote

    rows: list[dict[str, Any]] = []
    for signal in signals.itertuples(index=False):
        signal_series = pd.Series(signal._asdict())
        code = str(signal_series["code"]).zfill(6)
        quote = quote_map.get(code)
        if not quote:
            errors.append(f"{code}: missing quote")
            continue
        rows.append(quote_row(signal_series, quote, run_date, args.time_label))

    rows.sort(key=lambda r: r["code"])
    write_csv(snapshot_path, rows, SNAPSHOT_COLUMNS)
    write_performance(reports_dir, run_date)

    message = (
        f"date={run_date}, time_label={args.time_label}, signals={len(signals)}, quotes={len(rows)}, "
        f"errors={len(errors)}, output={snapshot_path}, performance={reports_dir / f'signal_performance_{run_date}.csv'}"
    )
    log_path.write_text(message + "\n" + "\n".join(errors[:100]) + ("\n" if errors else ""), encoding="utf-8")
    print(message)
    if errors:
        print(f"first errors: {errors[:5]}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
