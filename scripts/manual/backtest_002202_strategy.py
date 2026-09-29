#!/usr/bin/env python3
"""Backtest a high-win-rate daily swing strategy against the local TDX API."""

from __future__ import annotations

import argparse
import json
import math
import urllib.request
from dataclasses import dataclass
from datetime import datetime
from itertools import product

import numpy as np
import pandas as pd


DEFAULT_URL = "http://localhost:8080/api/kline?code={code}&type=day"


@dataclass(frozen=True)
class Params:
    ma_short: int
    ma_long: int
    rsi_period: int
    rsi_buy: float
    pullback: float
    target: float
    stop: float
    max_hold: int


@dataclass
class Result:
    params: Params
    metrics: dict
    trades: pd.DataFrame
    equity: pd.Series


def fetch_kline(code: str, url_tpl: str) -> pd.DataFrame:
    with urllib.request.urlopen(url_tpl.format(code=code), timeout=20) as resp:
        payload = json.loads(resp.read())
    if payload.get("code") != 0:
        raise RuntimeError(payload)

    rows = payload["data"]["List"]
    df = pd.DataFrame(rows)
    df["date"] = pd.to_datetime(df["Time"]).dt.tz_localize(None)
    for col in ["Open", "High", "Low", "Close"]:
        df[col.lower()] = df[col].astype(float)
    df["volume"] = df["Volume"].astype(float)
    df = df[["date", "open", "high", "low", "close", "volume"]].sort_values("date")
    df = df.drop_duplicates("date").reset_index(drop=True)
    return df


def rsi(close: pd.Series, period: int) -> pd.Series:
    delta = close.diff()
    gain = delta.clip(lower=0).ewm(alpha=1 / period, adjust=False).mean()
    loss = (-delta.clip(upper=0)).ewm(alpha=1 / period, adjust=False).mean()
    rs = gain / loss.replace(0, np.nan)
    return 100 - 100 / (1 + rs)


def add_indicators(df: pd.DataFrame, params: Params) -> pd.DataFrame:
    out = df.copy()
    out["ma_short"] = out["close"].rolling(params.ma_short).mean()
    out["ma_long"] = out["close"].rolling(params.ma_long).mean()
    out["rsi"] = rsi(out["close"], params.rsi_period)
    out["vol_ma20"] = out["volume"].rolling(20).mean()
    return out


def backtest(
    source_df: pd.DataFrame,
    params: Params,
    start: str | None = None,
    end: str | None = None,
    initial_cash: float = 100_000.0,
    buy_cost: float = 0.0005,
    sell_cost: float = 0.0010,
) -> Result:
    df = add_indicators(source_df, params)
    if start:
        df = df[df["date"] >= pd.Timestamp(start)]
    if end:
        df = df[df["date"] <= pd.Timestamp(end)]
    df = df.reset_index(drop=True)

    cash = initial_cash
    shares = 0.0
    entry_price = 0.0
    entry_date = None
    entry_idx = None
    trades = []
    equity_curve = []

    for i in range(max(params.ma_long, params.ma_short, params.rsi_period) + 2, len(df) - 1):
        row = df.iloc[i]
        nxt = df.iloc[i + 1]
        market_value = shares * row.close
        equity_curve.append((row.date, cash + market_value))

        if shares <= 0:
            prev = df.iloc[i - 1]
            signal = (
                row.close > row.ma_long
                and row.ma_short > row.ma_long
                and row.close <= row.ma_short * (1 + params.pullback)
                and row.close >= row.ma_long * 1.01
                and row.rsi <= params.rsi_buy
                and row.close > prev.close
                and row.volume >= row.vol_ma20 * 0.7
            )
            if signal and math.isfinite(row.ma_long) and math.isfinite(row.rsi):
                fill = nxt.open
                shares = cash * (1 - buy_cost) / fill
                cash = 0.0
                entry_price = fill
                entry_date = nxt.date
                entry_idx = i + 1
            continue

        hold_days = i - entry_idx
        gain = row.close / entry_price - 1
        exit_reason = None
        if gain >= params.target:
            exit_reason = "target"
        elif gain <= -params.stop:
            exit_reason = "stop"
        elif hold_days >= params.max_hold:
            exit_reason = "time"
        elif row.close < row.ma_long:
            exit_reason = "trend"

        if exit_reason:
            fill = nxt.open
            proceeds = shares * fill * (1 - sell_cost)
            ret = proceeds / (shares * entry_price) - 1
            cash = proceeds
            trades.append(
                {
                    "entry_date": entry_date,
                    "exit_date": nxt.date,
                    "entry": entry_price,
                    "exit": fill,
                    "return": ret,
                    "hold_days": hold_days + 1,
                    "reason": exit_reason,
                }
            )
            shares = 0.0
            entry_price = 0.0
            entry_date = None
            entry_idx = None

    if shares > 0:
        row = df.iloc[-1]
        proceeds = shares * row.close * (1 - sell_cost)
        ret = proceeds / (shares * entry_price) - 1
        cash = proceeds
        trades.append(
            {
                "entry_date": entry_date,
                "exit_date": row.date,
                "entry": entry_price,
                "exit": row.close,
                "return": ret,
                "hold_days": len(df) - 1 - entry_idx,
                "reason": "final",
            }
        )
        equity_curve.append((row.date, cash))

    equity = pd.Series(
        [v for _, v in equity_curve],
        index=pd.to_datetime([d for d, _ in equity_curve]),
        dtype=float,
    )
    trades_df = pd.DataFrame(trades)
    metrics = calc_metrics(equity, trades_df, initial_cash, df)
    return Result(params=params, metrics=metrics, trades=trades_df, equity=equity)


def calc_metrics(
    equity: pd.Series, trades: pd.DataFrame, initial_cash: float, df: pd.DataFrame
) -> dict:
    if equity.empty:
        return {
            "trades": 0,
            "win_rate": 0.0,
            "total_return": 0.0,
            "cagr": 0.0,
            "max_drawdown": 0.0,
            "profit_factor": 0.0,
            "avg_trade": 0.0,
            "exposure": 0.0,
        }

    total_return = equity.iloc[-1] / initial_cash - 1
    years = max((equity.index[-1] - equity.index[0]).days / 365.25, 1e-9)
    cagr = (equity.iloc[-1] / initial_cash) ** (1 / years) - 1
    peak = equity.cummax()
    drawdown = equity / peak - 1

    if trades.empty:
        wins = pd.Series(dtype=float)
        losses = pd.Series(dtype=float)
        exposure = 0.0
        avg_trade = 0.0
    else:
        wins = trades.loc[trades["return"] > 0, "return"]
        losses = trades.loc[trades["return"] <= 0, "return"]
        exposure = trades["hold_days"].sum() / max(len(df), 1)
        avg_trade = trades["return"].mean()

    profit_factor = wins.sum() / abs(losses.sum()) if abs(losses.sum()) > 0 else float("inf")
    return {
        "trades": int(len(trades)),
        "win_rate": float(len(wins) / len(trades)) if len(trades) else 0.0,
        "total_return": float(total_return),
        "cagr": float(cagr),
        "max_drawdown": float(drawdown.min()),
        "profit_factor": float(profit_factor),
        "avg_trade": float(avg_trade),
        "exposure": float(exposure),
    }


def score(metrics: dict) -> float:
    if metrics["trades"] < 25:
        return -999
    if metrics["profit_factor"] < 1.05:
        return -999
    return (
        metrics["win_rate"] * 2.0
        + metrics["profit_factor"] * 0.25
        + metrics["cagr"] * 3.0
        + metrics["max_drawdown"] * 0.8
        + min(metrics["trades"], 120) / 300
    )


def search(df: pd.DataFrame, train_start: str, train_end: str) -> list[Result]:
    def fast_metrics(params: Params) -> dict:
        d = add_indicators(df, params)
        d = d[(d["date"] >= pd.Timestamp(train_start)) & (d["date"] <= pd.Timestamp(train_end))]
        d = d.reset_index(drop=True)
        if len(d) < params.ma_long + 5:
            return {"trades": 0, "score": -999}

        open_ = d["open"].to_numpy()
        close = d["close"].to_numpy()
        volume = d["volume"].to_numpy()
        ma_short = d["ma_short"].to_numpy()
        ma_long = d["ma_long"].to_numpy()
        rsi_arr = d["rsi"].to_numpy()
        vol_ma20 = d["vol_ma20"].to_numpy()

        initial_cash = 100_000.0
        cash = initial_cash
        shares = 0.0
        entry_price = 0.0
        entry_idx = 0
        trade_returns = []
        hold_days_total = 0
        equity = []

        start_i = max(params.ma_long, params.ma_short, params.rsi_period) + 2
        for i in range(start_i, len(d) - 1):
            equity.append(cash + shares * close[i])
            if shares <= 0:
                signal = (
                    close[i] > ma_long[i]
                    and ma_short[i] > ma_long[i]
                    and close[i] <= ma_short[i] * (1 + params.pullback)
                    and close[i] >= ma_long[i] * 1.01
                    and rsi_arr[i] <= params.rsi_buy
                    and close[i] > close[i - 1]
                    and volume[i] >= vol_ma20[i] * 0.7
                    and np.isfinite(ma_long[i])
                    and np.isfinite(rsi_arr[i])
                )
                if signal:
                    entry_price = open_[i + 1]
                    shares = cash * (1 - 0.0005) / entry_price
                    cash = 0.0
                    entry_idx = i + 1
                continue

            hold = i - entry_idx
            gain = close[i] / entry_price - 1
            if (
                gain >= params.target
                or gain <= -params.stop
                or hold >= params.max_hold
                or close[i] < ma_long[i]
            ):
                proceeds = shares * open_[i + 1] * (1 - 0.0010)
                trade_returns.append(proceeds / (shares * entry_price) - 1)
                hold_days_total += hold + 1
                cash = proceeds
                shares = 0.0

        if shares > 0:
            proceeds = shares * close[-1] * (1 - 0.0010)
            trade_returns.append(proceeds / (shares * entry_price) - 1)
            hold_days_total += len(d) - 1 - entry_idx
            cash = proceeds
            equity.append(cash)

        if not equity:
            return {"trades": 0, "score": -999}

        eq = np.asarray(equity)
        returns = np.asarray(trade_returns, dtype=float)
        wins = returns[returns > 0]
        losses = returns[returns <= 0]
        peak = np.maximum.accumulate(eq)
        days = max((d["date"].iloc[-1] - d["date"].iloc[start_i]).days, 1)
        total_return = eq[-1] / initial_cash - 1
        profit_factor = wins.sum() / abs(losses.sum()) if abs(losses.sum()) > 0 else float("inf")
        metrics = {
            "trades": int(len(returns)),
            "win_rate": float(len(wins) / len(returns)) if len(returns) else 0.0,
            "total_return": float(total_return),
            "cagr": float((eq[-1] / initial_cash) ** (365.25 / days) - 1),
            "max_drawdown": float(np.min(eq / peak - 1)),
            "profit_factor": float(profit_factor),
            "avg_trade": float(np.mean(returns)) if len(returns) else 0.0,
            "exposure": float(hold_days_total / max(len(d), 1)),
        }
        metrics["score"] = score(metrics)
        return metrics

    grid = product(
        [10, 20, 30],
        [120, 200],
        [6, 10, 14],
        [35, 45, 55],
        [0.00, 0.03, 0.06],
        [0.05, 0.08],
        [0.06, 0.10],
        [15, 30, 45],
    )
    results = []
    for values in grid:
        params = Params(*values)
        if params.ma_short >= params.ma_long:
            continue
        metrics = fast_metrics(params)
        if metrics["score"] > -999:
            results.append(Result(params=params, metrics=metrics, trades=pd.DataFrame(), equity=pd.Series(dtype=float)))
    results.sort(key=lambda r: r.metrics["score"], reverse=True)
    return results


def fmt_pct(x: float) -> str:
    return f"{x * 100:.2f}%"


def print_result(name: str, result: Result) -> None:
    m = result.metrics
    print(f"\n{name}")
    print("-" * len(name))
    print(f"params        : {result.params}")
    print(f"trades        : {m['trades']}")
    print(f"win_rate      : {fmt_pct(m['win_rate'])}")
    print(f"total_return  : {fmt_pct(m['total_return'])}")
    print(f"cagr          : {fmt_pct(m['cagr'])}")
    print(f"max_drawdown  : {fmt_pct(m['max_drawdown'])}")
    print(f"profit_factor : {m['profit_factor']:.2f}")
    print(f"avg_trade     : {fmt_pct(m['avg_trade'])}")
    print(f"exposure      : {fmt_pct(m['exposure'])}")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--code", default="002202")
    parser.add_argument("--url", default=DEFAULT_URL)
    parser.add_argument("--train-start", default="2008-01-01")
    parser.add_argument("--train-end", default="2020-12-31")
    parser.add_argument("--test-start", default="2021-01-01")
    args = parser.parse_args()

    df = fetch_kline(args.code, args.url)
    print(f"loaded {len(df)} bars: {df.date.iloc[0].date()} -> {df.date.iloc[-1].date()}")
    print("execution: signal after close, trade at next trading day's open")
    print("costs: buy 0.05%, sell 0.10%")

    candidates = search(df, args.train_start, args.train_end)
    if not candidates:
        raise SystemExit("no candidate strategy passed filters")

    best = candidates[0]
    print_result("best train result", best)
    print("\ntop 5 train candidates")
    for idx, res in enumerate(candidates[:5], 1):
        m = res.metrics
        print(
            f"{idx}. win={fmt_pct(m['win_rate'])}, cagr={fmt_pct(m['cagr'])}, "
            f"mdd={fmt_pct(m['max_drawdown'])}, pf={m['profit_factor']:.2f}, "
            f"trades={m['trades']}, params={res.params}"
        )

    full = backtest(df, best.params)
    test = backtest(df, best.params, start=args.test_start)
    print_result("full sample result", full)
    print_result(f"out-of-sample result since {args.test_start}", test)

    if not full.trades.empty:
        print("\nlatest 10 trades")
        out = full.trades.tail(10).copy()
        out["entry_date"] = out["entry_date"].dt.strftime("%Y-%m-%d")
        out["exit_date"] = out["exit_date"].dt.strftime("%Y-%m-%d")
        out["return"] = out["return"].map(fmt_pct)
        print(out.to_string(index=False))

    last = add_indicators(df, best.params).iloc[-1]
    prev = add_indicators(df, best.params).iloc[-2]
    today_signal = (
        last.close > last.ma_long
        and last.ma_short > last.ma_long
        and last.close <= last.ma_short * (1 + best.params.pullback)
        and last.close >= last.ma_long * 1.01
        and last.rsi <= best.params.rsi_buy
        and last.close > prev.close
        and last.volume >= last.vol_ma20 * 0.7
    )
    print("\nlatest bar")
    print(
        f"{last.date.date()} close={last.close:.0f}, ma{best.params.ma_short}={last.ma_short:.0f}, "
        f"ma{best.params.ma_long}={last.ma_long:.0f}, rsi{best.params.rsi_period}={last.rsi:.2f}, "
        f"entry_signal={today_signal}"
    )


if __name__ == "__main__":
    main()
