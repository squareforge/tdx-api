#!/usr/bin/env python3
"""Daily A-share v3 breakout-pullback scanner with SQLite kline cache."""

from __future__ import annotations

import argparse
import csv
import json
import sqlite3
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import pandas as pd

from backtest_a_share_breakout_v3 import V3Params, entry_signal, prepare


BASE_URL = "http://localhost:8080"
DEFAULT_DB = Path("data/market/a_share_daily.db")
REPORT_COLUMNS = [
    "rank",
    "rank_score",
    "selected_top3",
    "date",
    "signal_date",
    "code",
    "name",
    "close",
    "buy_price_low",
    "buy_price_high",
    "ma5",
    "ma20",
    "ma60",
    "ma120",
    "ret20",
    "ret60",
    "volume",
    "vol20",
    "signal_reason",
    "rank_reason",
]


@dataclass(frozen=True)
class StockMeta:
    code: str
    exchange: str
    name: str


def request_json(path: str, base_url: str = BASE_URL, timeout: int = 30) -> dict[str, Any]:
    url = urllib.parse.urljoin(base_url.rstrip("/") + "/", path.lstrip("/"))
    with urllib.request.urlopen(url, timeout=timeout) as resp:
        return json.loads(resp.read())


def init_db(conn: sqlite3.Connection) -> None:
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA busy_timeout=60000")
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS stock_meta (
            code TEXT PRIMARY KEY,
            exchange TEXT NOT NULL,
            name TEXT NOT NULL,
            updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
        """
    )
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS daily_kline (
            code TEXT NOT NULL,
            date TEXT NOT NULL,
            open REAL NOT NULL,
            high REAL NOT NULL,
            low REAL NOT NULL,
            close REAL NOT NULL,
            volume REAL NOT NULL,
            amount REAL,
            source TEXT NOT NULL DEFAULT 'api',
            updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (code, date)
        )
        """
    )
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS update_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            run_date TEXT NOT NULL,
            source TEXT NOT NULL,
            status TEXT NOT NULL,
            rows INTEGER NOT NULL DEFAULT 0,
            message TEXT,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )
        """
    )
    conn.execute("CREATE INDEX IF NOT EXISTS idx_daily_kline_code_date ON daily_kline (code, date)")
    conn.commit()


def connect_db(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(path, timeout=60)
    init_db(conn)
    return conn


def is_common_a_share(meta: StockMeta) -> bool:
    code = meta.code
    name = meta.name.upper()
    if meta.exchange not in {"sh", "sz"}:
        return False
    if "ST" in name or "退" in meta.name:
        return False
    if code.startswith(("688", "689")):
        return False
    if meta.exchange == "sh":
        return code.startswith(("600", "601", "603", "605"))
    return code.startswith(("000", "001", "002", "003", "300", "301"))


def fetch_stock_meta(base_url: str) -> list[StockMeta]:
    metas: list[StockMeta] = []
    for exchange in ("sh", "sz"):
        payload = request_json(f"/api/codes?exchange={exchange}", base_url=base_url, timeout=30)
        if payload.get("code") != 0:
            raise RuntimeError(f"failed to fetch codes for {exchange}: {payload}")
        for item in payload.get("data", {}).get("codes", []):
            meta = StockMeta(
                code=str(item.get("code", "")).strip(),
                exchange=str(item.get("exchange", exchange)).strip(),
                name=str(item.get("name", "")).strip(),
            )
            if meta.code and is_common_a_share(meta):
                metas.append(meta)
    return metas


def save_stock_meta(conn: sqlite3.Connection, metas: list[StockMeta]) -> None:
    conn.executemany(
        """
        INSERT INTO stock_meta (code, exchange, name, updated_at)
        VALUES (?, ?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(code) DO UPDATE SET
            exchange = excluded.exchange,
            name = excluded.name,
            updated_at = CURRENT_TIMESTAMP
        """,
        [(m.code, m.exchange, m.name) for m in metas],
    )
    conn.commit()


def price_scale(rows: list[dict[str, Any]]) -> float:
    closes = [abs(float(r.get("Close", 0) or 0)) for r in rows[-30:]]
    closes = [x for x in closes if x > 0]
    if not closes:
        return 1.0
    median_close = sorted(closes)[len(closes) // 2]
    return 1000.0 if median_close > 1000 else 1.0


def normalize_kline_payload(code: str, payload: dict[str, Any]) -> pd.DataFrame:
    if payload.get("code") != 0:
        raise RuntimeError(payload)
    rows = payload.get("data", {}).get("List", [])
    if not rows:
        return pd.DataFrame(columns=["code", "date", "open", "high", "low", "close", "volume", "amount"])

    scale = price_scale(rows)
    df = pd.DataFrame(rows)
    out = pd.DataFrame()
    out["date"] = pd.to_datetime(df["Time"], utc=True).dt.tz_convert("Asia/Shanghai").dt.strftime("%Y-%m-%d")
    for src, dst in [("Open", "open"), ("High", "high"), ("Low", "low"), ("Close", "close")]:
        out[dst] = pd.to_numeric(df[src], errors="coerce") / scale
    out["volume"] = pd.to_numeric(df.get("Volume", 0), errors="coerce")
    out["amount"] = pd.to_numeric(df.get("Amount", 0), errors="coerce").astype(float)
    out = out.dropna(subset=["date", "open", "high", "low", "close"])
    out = out[out["close"] > 0]
    missing_amount = out["amount"].isna() | (out["amount"] <= 0)
    out.loc[missing_amount, "amount"] = out.loc[missing_amount, "close"] * out.loc[missing_amount, "volume"] * 100
    out.insert(0, "code", code)
    return out.sort_values("date").drop_duplicates(["code", "date"], keep="last").reset_index(drop=True)


def fetch_kline_from_api(code: str, base_url: str) -> pd.DataFrame:
    payload = request_json(
        f"/api/kline?code={urllib.parse.quote(code)}&type=day",
        base_url=base_url,
        timeout=45,
    )
    return normalize_kline_payload(code, payload)


def save_kline(conn: sqlite3.Connection, df: pd.DataFrame) -> int:
    if df.empty:
        return 0
    rows = [
        (
            str(r.code),
            str(r.date),
            float(r.open),
            float(r.high),
            float(r.low),
            float(r.close),
            float(r.volume),
            None if pd.isna(r.amount) else float(r.amount),
        )
        for r in df.itertuples(index=False)
    ]
    conn.executemany(
        """
        INSERT INTO daily_kline (code, date, open, high, low, close, volume, amount, source, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'api', CURRENT_TIMESTAMP)
        ON CONFLICT(code, date) DO UPDATE SET
            open = excluded.open,
            high = excluded.high,
            low = excluded.low,
            close = excluded.close,
            volume = excluded.volume,
            amount = excluded.amount,
            source = excluded.source,
            updated_at = CURRENT_TIMESTAMP
        """,
        rows,
    )
    conn.commit()
    return len(rows)


def load_kline(conn: sqlite3.Connection, code: str) -> pd.DataFrame:
    return pd.read_sql_query(
        """
        SELECT date, open, high, low, close, volume, amount
        FROM daily_kline
        WHERE code = ?
        ORDER BY date
        """,
        conn,
        params=(code,),
        parse_dates=["date"],
    )


def has_enough_cache(conn: sqlite3.Connection, code: str, min_bars: int) -> bool:
    row = conn.execute("SELECT COUNT(*) FROM daily_kline WHERE code = ?", (code,)).fetchone()
    return bool(row and int(row[0]) >= min_bars)


def ensure_kline(
    conn: sqlite3.Connection,
    code: str,
    base_url: str,
    min_bars: int,
    refresh: bool,
) -> tuple[pd.DataFrame, str, int]:
    if not refresh and has_enough_cache(conn, code, min_bars):
        return load_kline(conn, code), "cache", 0
    df = fetch_kline_from_api(code, base_url)
    rows = save_kline(conn, df)
    return df[["date", "open", "high", "low", "close", "volume", "amount"]], "api", rows


def format_signal_reason(row: pd.Series) -> str:
    pullback = row.pullback_from_recent5_high * 100
    return (
        "v3: 市场多头; MA20>MA60>MA120; "
        f"20日涨幅{row.ret20 * 100:.2f}%; 60日涨幅{row.ret60 * 100:.2f}%; "
        f"近期高点回撤{pullback:.2f}%; 放量转强站上MA5"
    )


def clamp(value: float, low: float = 0.0, high: float = 1.0) -> float:
    return max(low, min(high, value))


def band_score(value: float, best: float, low: float, high: float) -> float:
    if value < low or value > high:
        return 0.0
    if value == best:
        return 1.0
    if value < best:
        return clamp((value - low) / (best - low))
    return clamp((high - value) / (high - best))


def rank_candidate(prepared: pd.DataFrame) -> tuple[float, str]:
    row = prepared.iloc[-1]
    prev5 = prepared.iloc[max(len(prepared) - 6, 0)]
    prev20 = prepared.iloc[max(len(prepared) - 21, 0)]

    ma20_slope = float(row.ma20 / prev5.ma20 - 1) if prev5.ma20 > 0 else 0.0
    ma60_slope = float(row.ma60 / prev20.ma60 - 1) if prev20.ma60 > 0 else 0.0
    ma20_ma60_gap = float(row.ma20 / row.ma60 - 1) if row.ma60 > 0 else 0.0
    ma60_ma120_gap = float(row.ma60 / row.ma120 - 1) if row.ma120 > 0 else 0.0
    dist_ma20 = float(row.close / row.ma20 - 1) if row.ma20 > 0 else 0.0
    pullback = abs(float(row.pullback_from_recent5_high))
    vol_ratio = float(row.volume / row.vol20) if row.vol20 > 0 else 0.0
    vol_trend = float(row.vol20 / row.vol60) if row.vol60 > 0 else 0.0
    amount = float(getattr(row, "amount", 0) or 0)

    trend_score = (
        clamp(ma20_slope / 0.06) * 0.35
        + clamp(ma60_slope / 0.08) * 0.25
        + band_score(ma20_ma60_gap, best=0.12, low=0.02, high=0.35) * 0.25
        + band_score(ma60_ma120_gap, best=0.12, low=0.01, high=0.35) * 0.15
    )
    pullback_score = band_score(pullback, best=0.06, low=0.02, high=0.14)
    volume_score = band_score(vol_ratio, best=1.25, low=0.80, high=2.80) * 0.65 + clamp((vol_trend - 1.0) / 0.8) * 0.35
    strength_score = (
        band_score(float(row.ret20), best=0.30, low=0.08, high=0.80) * 0.55
        + band_score(float(row.ret60), best=0.55, low=0.15, high=1.50) * 0.45
    )
    liquidity_score = clamp((amount - 100_000_000) / (2_000_000_000 - 100_000_000))
    risk_score = band_score(dist_ma20, best=0.06, low=0.00, high=0.30)

    score = (
        trend_score * 15  # 降低:25->15 减少追高倾向
        + pullback_score * 20
        + volume_score * 20
        + strength_score * 15
        + liquidity_score * 5  # 降低:10->5 为RISK腾出空间
        + risk_score * 25  # 提高:10->25 强化稳定性权重
    )
    reason = (
        f"trend={trend_score * 15:.1f}/15, pullback={pullback_score * 20:.1f}/20, "
        f"volume={volume_score * 20:.1f}/20, strength={strength_score * 15:.1f}/15, "
        f"liquidity={liquidity_score * 5:.1f}/5, risk={risk_score * 25:.1f}/25"
    )
    return round(score, 3), reason


def build_signal_row(run_date: str, meta: StockMeta, prepared: pd.DataFrame) -> dict[str, Any]:
    row = prepared.iloc[-1]
    close = float(row.close)
    rank_score, rank_reason = rank_candidate(prepared)
    return {
        "rank": "",
        "rank_score": rank_score,
        "selected_top3": "",
        "date": run_date,
        "signal_date": row.date.strftime("%Y-%m-%d"),
        "code": meta.code,
        "name": meta.name,
        "close": round(close, 3),
        "buy_price_low": round(close, 3),
        "buy_price_high": round(close * 1.02, 3),
        "ma5": round(float(row.ma5), 3),
        "ma20": round(float(row.ma20), 3),
        "ma60": round(float(row.ma60), 3),
        "ma120": round(float(row.ma120), 3),
        "ret20": round(float(row.ret20), 6),
        "ret60": round(float(row.ret60), 6),
        "volume": round(float(row.volume), 3),
        "vol20": round(float(row.vol20), 3),
        "signal_reason": format_signal_reason(row),
        "rank_reason": rank_reason,
    }


def apply_ranking(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    ranked = sorted(rows, key=lambda r: (-float(r["rank_score"]), str(r["code"])))
    for idx, row in enumerate(ranked, 1):
        row["rank"] = idx
        row["selected_top3"] = "yes" if idx <= 3 else "no"
    return ranked


def write_report(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8-sig") as fh:
        writer = csv.DictWriter(fh, fieldnames=REPORT_COLUMNS)
        writer.writeheader()
        writer.writerows(rows)


def append_log(conn: sqlite3.Connection, run_date: str, status: str, rows: int, message: str) -> None:
    conn.execute(
        "INSERT INTO update_log (run_date, source, status, rows, message) VALUES (?, 'select_v3', ?, ?, ?)",
        (run_date, status, rows, message),
    )
    conn.commit()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default=BASE_URL)
    parser.add_argument("--db", default=str(DEFAULT_DB))
    parser.add_argument("--reports-dir", default="reports")
    parser.add_argument("--workers", type=int, default=8)
    parser.add_argument("--min-bars", type=int, default=160)
    parser.add_argument("--min-amount", type=float, default=30_000_000)
    parser.add_argument("--refresh", action="store_true", help="Fetch API even when cache has enough bars.")
    parser.add_argument("--limit", type=int, help="Limit stock count for smoke tests.")
    parser.add_argument("--output-suffix", default="", help="Optional suffix for the report filename.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    run_date = pd.Timestamp.now(tz="Asia/Shanghai").strftime("%Y-%m-%d")
    started = time.time()
    db_path = Path(args.db)
    db_path.parent.mkdir(parents=True, exist_ok=True)

    conn = connect_db(db_path)

    try:
        metas = fetch_stock_meta(args.base_url)
        save_stock_meta(conn, metas)
    except Exception as exc:
        print(f"failed to fetch stock meta: {exc}", file=sys.stderr)
        cached = conn.execute("SELECT code, exchange, name FROM stock_meta").fetchall()
        metas = [StockMeta(code=row[0], exchange=row[1], name=row[2]) for row in cached if is_common_a_share(StockMeta(row[0], row[1], row[2]))]

    if args.limit:
        metas = metas[: args.limit]

    p = V3Params()
    market_df, market_source, market_rows = ensure_kline(conn, p.market_code, args.base_url, args.min_bars, args.refresh)
    if len(market_df) < args.min_bars:
        raise RuntimeError(f"market kline too short for {p.market_code}: {len(market_df)} bars")

    cache_hits = 1 if market_source == "cache" else 0
    api_fetches = 1 if market_source == "api" else 0
    api_rows = market_rows
    errors: list[str] = []
    signals: list[dict[str, Any]] = []
    skipped_price = 0
    skipped_amount = 0
    scanned = 0

    def fetch_one(meta: StockMeta) -> tuple[StockMeta, pd.DataFrame, str, int, str | None]:
        worker_conn = connect_db(db_path)
        try:
            df, source, rows = ensure_kline(worker_conn, meta.code, args.base_url, args.min_bars, args.refresh)
            return meta, df, source, rows, None
        except Exception as exc:  # noqa: BLE001 - collect and keep scanning.
            return meta, pd.DataFrame(), "error", 0, str(exc)
        finally:
            worker_conn.close()

    with ThreadPoolExecutor(max_workers=max(args.workers, 1)) as executor:
        futures = [executor.submit(fetch_one, meta) for meta in metas]
        total = len(futures)
        for idx, future in enumerate(as_completed(futures), 1):
            meta, stock_df, source, rows, error = future.result()
            if source == "cache":
                cache_hits += 1
            elif source == "api":
                api_fetches += 1
                api_rows += rows

            if error:
                errors.append(f"{meta.code}: {error}")
            elif len(stock_df) >= args.min_bars:
                scanned += 1
                latest = stock_df.iloc[-1]
                if not (10.0 < float(latest.close) < 100.0):
                    skipped_price += 1
                elif "amount" in stock_df.columns and float(getattr(latest, "amount", 0) or 0) < args.min_amount:
                    skipped_amount += 1
                else:
                    prepared = prepare(stock_df, market_df, p)
                    i = len(prepared) - 1
                    if i > 0 and entry_signal(prepared, i, p):
                        signals.append(build_signal_row(run_date, meta, prepared))

            if idx % 100 == 0 or idx == total:
                print(
                    f"progress {idx}/{total}: scanned={scanned}, signals={len(signals)}, "
                    f"cache={cache_hits}, api={api_fetches}, errors={len(errors)}",
                    flush=True,
                )

    signals = apply_ranking(signals)
    suffix = f"_{args.output_suffix}" if args.output_suffix else ""
    report_path = Path(args.reports_dir) / f"signals_{run_date}{suffix}.csv"
    top3_path = Path(args.reports_dir) / f"signals_top3_{run_date}{suffix}.csv"
    write_report(report_path, signals)
    write_report(top3_path, signals[:3])

    elapsed = time.time() - started
    message = (
        f"run_date={run_date}, market_source={market_source}, market_rows={len(market_df)}, "
        f"candidates={len(metas)}, scanned={scanned}, cache_hits={cache_hits}, api_fetches={api_fetches}, "
        f"api_rows={api_rows}, skipped_price={skipped_price}, skipped_amount={skipped_amount}, "
        f"errors={len(errors)}, signals={len(signals)}, top3={min(len(signals), 3)}, "
        f"report={report_path}, top3_report={top3_path}, elapsed={elapsed:.1f}s"
    )
    append_log(conn, run_date, "success" if not errors else "partial", len(signals), message)

    log_dir = Path(args.reports_dir) / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    log_path = log_dir / f"select_v3_{run_date}.log"
    log_path.write_text(message + "\n" + "\n".join(errors[:200]) + ("\n" if errors else ""), encoding="utf-8")

    print(message)
    if errors:
        print(f"first errors: {errors[:5]}", file=sys.stderr)
    conn.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
