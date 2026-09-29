#!/usr/bin/env python3
"""Backtest an A-share trend-pullback strategy with a market filter."""

from __future__ import annotations

import argparse
import json
import urllib.request
from dataclasses import dataclass

import numpy as np
import pandas as pd


URL = "http://localhost:8080/api/kline?code={code}&type=day"


@dataclass(frozen=True)
class V2Params:
    market_code: str = "510300"
    market_short: int = 20
    market_long: int = 60
    ma_fast: int = 20
    ma_mid: int = 60
    ma_slow: int = 120
    high_window: int = 60
    recent_high_days: int = 10
    max_dist_from_high: float = 0.15
    max_pullback_20d: float = 0.18
    pullback_to_ma: float = 0.03
    volume_shrink: float = 1.00
    stop_loss: float = 0.08
    trailing_stop: float = 0.10
    max_hold: int = 35


def fetch(code: str) -> pd.DataFrame:
    with urllib.request.urlopen(URL.format(code=code), timeout=20) as resp:
        payload = json.loads(resp.read())
    if payload.get("code") != 0:
        raise RuntimeError(payload)
    df = pd.DataFrame(payload["data"]["List"])
    df["date"] = pd.to_datetime(df["Time"]).dt.tz_localize(None)
    for col in ["Open", "High", "Low", "Close"]:
        df[col.lower()] = df[col].astype(float)
    df["volume"] = df["Volume"].astype(float)
    return df[["date", "open", "high", "low", "close", "volume"]].sort_values("date").reset_index(drop=True)


def prepare(stock: pd.DataFrame, market: pd.DataFrame, p: V2Params) -> pd.DataFrame:
    s = stock.copy()
    s["ma20"] = s.close.rolling(p.ma_fast).mean()
    s["ma60"] = s.close.rolling(p.ma_mid).mean()
    s["ma120"] = s.close.rolling(p.ma_slow).mean()
    s["vol20"] = s.volume.rolling(20).mean()
    s["hh60"] = s.high.rolling(p.high_window).max()
    s["dd20"] = s.close / s.close.rolling(20).max() - 1
    s["recent_hh60"] = s.high.rolling(p.recent_high_days).max() >= s.hh60.shift(1)

    m = market[["date", "close"]].copy()
    m["m_ma20"] = m.close.rolling(p.market_short).mean()
    m["m_ma60"] = m.close.rolling(p.market_long).mean()
    m["market_ok"] = (m.close > m.m_ma60) & (m.m_ma20 > m.m_ma60)
    s = s.merge(m[["date", "market_ok"]], on="date", how="left")
    s["market_ok"] = s["market_ok"].ffill().fillna(False)
    return s


def backtest(df: pd.DataFrame, start: str | None = None, initial_cash: float = 100_000) -> tuple[dict, pd.DataFrame]:
    if start:
        df = df[df.date >= pd.Timestamp(start)].reset_index(drop=True)
    cash = initial_cash
    shares = 0.0
    entry = 0.0
    entry_date = None
    entry_i = 0
    high_since_entry = 0.0
    trades = []
    equity = []

    min_i = 125
    for i in range(min_i, len(df) - 1):
        r = df.iloc[i]
        nxt = df.iloc[i + 1]
        equity.append(cash + shares * r.close)

        if shares <= 0:
            trend = r.close > r.ma120 and r.ma20 > r.ma60 > r.ma120
            near_high = r.close >= r.hh60 * (1 - PARAMS.max_dist_from_high)
            pullback_ok = r.dd20 >= -PARAMS.max_pullback_20d
            setup = r.recent_hh60 and near_high and pullback_ok
            touch_ma20 = r.low <= r.ma20 * (1 + PARAMS.pullback_to_ma)
            turn_up = r.close > r.open and r.close > df.iloc[i - 1].close
            volume_ok = r.volume <= r.vol20 * PARAMS.volume_shrink
            signal = bool(r.market_ok and trend and setup and touch_ma20 and turn_up and volume_ok)
            if signal:
                entry = nxt.open
                shares = cash * (1 - 0.0005) / entry
                cash = 0.0
                entry_date = nxt.date
                entry_i = i + 1
                high_since_entry = entry
            continue

        high_since_entry = max(high_since_entry, r.high)
        hold = i - entry_i
        gain = r.close / entry - 1
        trail = r.close / high_since_entry - 1
        exit_reason = None
        if gain <= -PARAMS.stop_loss:
            exit_reason = "stop"
        elif trail <= -PARAMS.trailing_stop:
            exit_reason = "trail"
        elif r.close < r.ma20:
            exit_reason = "ma20"
        elif hold >= PARAMS.max_hold:
            exit_reason = "time"

        if exit_reason:
            exit_price = nxt.open
            proceeds = shares * exit_price * (1 - 0.001)
            trades.append(
                {
                    "entry_date": entry_date,
                    "exit_date": nxt.date,
                    "entry": entry,
                    "exit": exit_price,
                    "return": proceeds / (shares * entry) - 1,
                    "hold_days": hold + 1,
                    "reason": exit_reason,
                }
            )
            cash = proceeds
            shares = 0.0

    if shares > 0:
        r = df.iloc[-1]
        proceeds = shares * r.close * (1 - 0.001)
        trades.append(
            {
                "entry_date": entry_date,
                "exit_date": r.date,
                "entry": entry,
                "exit": r.close,
                "return": proceeds / (shares * entry) - 1,
                "hold_days": len(df) - 1 - entry_i,
                "reason": "final",
            }
        )
        cash = proceeds
        equity.append(cash)

    trades_df = pd.DataFrame(trades)
    metrics = calc_metrics(pd.Series(equity, dtype=float), trades_df, initial_cash, df)
    return metrics, trades_df


def calc_metrics(equity: pd.Series, trades: pd.DataFrame, initial_cash: float, df: pd.DataFrame) -> dict:
    if equity.empty:
        return {"trades": 0, "win_rate": 0.0, "total_return": 0.0, "cagr": 0.0, "max_drawdown": 0.0, "profit_factor": 0.0, "avg_trade": 0.0, "exposure": 0.0}
    total = equity.iloc[-1] / initial_cash - 1
    years = max((df.date.iloc[-1] - df.date.iloc[min(125, len(df) - 1)]).days / 365.25, 1e-9)
    peak = equity.cummax()
    wins = trades.loc[trades["return"] > 0, "return"] if not trades.empty else pd.Series(dtype=float)
    losses = trades.loc[trades["return"] <= 0, "return"] if not trades.empty else pd.Series(dtype=float)
    pf = wins.sum() / abs(losses.sum()) if abs(losses.sum()) > 0 else float("inf")
    return {
        "trades": int(len(trades)),
        "win_rate": float(len(wins) / len(trades)) if len(trades) else 0.0,
        "total_return": float(total),
        "cagr": float((equity.iloc[-1] / initial_cash) ** (1 / years) - 1),
        "max_drawdown": float((equity / peak - 1).min()),
        "profit_factor": float(pf),
        "avg_trade": float(trades["return"].mean()) if not trades.empty else 0.0,
        "exposure": float(trades["hold_days"].sum() / max(len(df), 1)) if not trades.empty else 0.0,
    }


def pct(x: float) -> str:
    return f"{x * 100:.2f}%"


def print_metrics(title: str, metrics: dict) -> None:
    print(f"\n{title}")
    print("-" * len(title))
    print(f"trades        : {metrics['trades']}")
    print(f"win_rate      : {pct(metrics['win_rate'])}")
    print(f"total_return  : {pct(metrics['total_return'])}")
    print(f"cagr          : {pct(metrics['cagr'])}")
    print(f"max_drawdown  : {pct(metrics['max_drawdown'])}")
    print(f"profit_factor : {metrics['profit_factor']:.2f}")
    print(f"avg_trade     : {pct(metrics['avg_trade'])}")
    print(f"exposure      : {pct(metrics['exposure'])}")


PARAMS = V2Params()


def main() -> None:
    global PARAMS

    parser = argparse.ArgumentParser()
    parser.add_argument("codes", nargs="+")
    parser.add_argument("--market", default=PARAMS.market_code)
    args = parser.parse_args()

    PARAMS = V2Params(market_code=args.market)
    market = fetch(PARAMS.market_code)
    print(f"market filter: {PARAMS.market_code}, close > MA60 and MA20 > MA60")
    print("execution: signal after close, trade at next trading day's open")
    print("costs: buy 0.05%, sell 0.10%")

    for code in args.codes:
        stock = fetch(code)
        df = prepare(stock, market, PARAMS)
        print(f"\n===== {code} =====")
        print(f"bars: {stock.date.iloc[0].date()} -> {stock.date.iloc[-1].date()}, count={len(stock)}")
        full_m, full_t = backtest(df)
        test_m, test_t = backtest(df, start="2021-01-01")
        print_metrics("full sample", full_m)
        print_metrics("since 2021-01-01", test_m)
        if not full_t.empty:
            out = full_t.tail(8).copy()
            out["entry_date"] = out.entry_date.dt.strftime("%Y-%m-%d")
            out["exit_date"] = out.exit_date.dt.strftime("%Y-%m-%d")
            out["return"] = out["return"].map(pct)
            print("\nlatest trades")
            print(out.to_string(index=False))

        r = df.iloc[-1]
        print(
            "\nlatest bar: "
            f"{r.date.date()} close={r.close:.0f}, ma20={r.ma20:.0f}, ma60={r.ma60:.0f}, "
            f"ma120={r.ma120:.0f}, market_ok={bool(r.market_ok)}"
        )


if __name__ == "__main__":
    main()
