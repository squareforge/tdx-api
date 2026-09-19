#!/usr/bin/env python3

import argparse
import json
import math
import sqlite3
import sys
import time
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timedelta
from itertools import product


HIKYUU_BATCH_URL = "http://localhost:18091/api/hikyuu/kline/batch"
ROUND_TRIP_COST_PCT = 0.025 * 2 + 0.05 + 0.05 * 2
MIN_NET_EDGE_PCT = 0.30
BAR_MINUTES = 1


@dataclass(frozen=True)
class Config:
    rules: str
    dev: float = 0.5
    back: float = 0.2
    stop_dev: float = 0.8
    s2_stop: float = 0.3
    s3_lookback: int = 10
    confirm_5m: bool = True
    trend_veto: bool = False
    max_hold: int = 120
    pending_minutes: int = 3
    take_profit: float = 0.4
    stop_loss: float = 0.8
    max_day_gain: float = 99.0
    min_day_gain: float = -99.0
    min_vwap_slope: float = -99.0
    min_volume_ratio: float = 0.0


@dataclass
class Trade:
    symbol: str
    date: str
    rule: str
    entry: float
    exit: float
    net_pct: float
    reason: str
    direction: str


def load_universe(db_path: str, count: int) -> list[str]:
    connection = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        rows = connection.execute(
            """
            WITH liquid AS (
                SELECT code, AVG(amount) AS avg_amount, COUNT(*) AS samples
                FROM daily_kline
                WHERE date >= '2026-04-20'
                GROUP BY code
                HAVING COUNT(*) >= 15
            )
            SELECT m.exchange || m.code
            FROM liquid l
            JOIN stock_meta m ON m.code = l.code
            WHERE m.exchange IN ('sh', 'sz')
              AND m.name NOT LIKE '%ST%'
              AND m.name NOT LIKE '%退%'
            ORDER BY l.avg_amount DESC
            LIMIT 800
            """
        ).fetchall()
    finally:
        connection.close()
    ranked = [row[0] for row in rows]
    if len(ranked) <= count:
        return ranked
    top_count = count // 2
    top = ranked[:top_count]
    tail = ranked[top_count:]
    step = max(1, len(tail) // (count - top_count))
    sampled = tail[::step][: count - top_count]
    return top + sampled


def fetch_batch(symbols: list[str], start: str, end: str, period: str) -> dict[str, list[dict]]:
    payload = json.dumps(
        {
            "symbols": symbols,
            "period": period,
            "start": start,
            "end": end,
            "limit": 2000,
            "recover": "none",
        }
    ).encode()
    request = urllib.request.Request(
        HIKYUU_BATCH_URL,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=180) as response:
        envelope = json.load(response)
    if envelope.get("code") != 0:
        raise RuntimeError(envelope.get("message") or "Hikyuu query failed")
    data = envelope["data"]
    for symbol, error in data.get("errors", {}).items():
        print(f"skip {symbol}: {error}", file=sys.stderr)
    return {symbol: item.get("list", []) for symbol, item in data.get("data", {}).items()}


def fetch_universe(symbols: list[str], start: str, end: str, period: str, batch_size: int = 12) -> dict[str, list[dict]]:
    result: dict[str, list[dict]] = {}
    for index in range(0, len(symbols), batch_size):
        batch = symbols[index : index + batch_size]
        before = time.time()
        loaded = fetch_batch(batch, start, end, period)
        result.update(loaded)
        print(
            f"loaded {min(index + batch_size, len(symbols))}/{len(symbols)} symbols "
            f"({len(loaded)} available, {time.time() - before:.1f}s)",
            file=sys.stderr,
        )
    return result


def parse_time(value: str) -> datetime:
    return datetime.fromisoformat(value) - timedelta(minutes=BAR_MINUTES)


def ema(values: list[float], period: int) -> list[float]:
    if not values:
        return []
    alpha = 2 / (period + 1)
    result = [values[0]]
    for value in values[1:]:
        result.append(result[-1] * (1 - alpha) + value * alpha)
    return result


def prepare_session(symbol: str, date: str, rows: list[dict], prev_volume: float, prev_close: float) -> dict:
    closes = [float(row["close"]) for row in rows]
    ema12 = ema(closes, 12)
    ema26 = ema(closes, 26)
    dif = [fast - slow for fast, slow in zip(ema12, ema26)]
    bars = []
    cumulative_volume = 0.0
    cumulative_amount = 0.0
    completed_five_direction = None
    for index, row in enumerate(rows):
        at = parse_time(row["time"])
        volume = max(0.0, float(row.get("volume") or 0))
        amount = max(0.0, float(row.get("amount") or 0)) * 10000
        cumulative_volume += volume
        cumulative_amount += amount
        vwap = cumulative_amount / cumulative_volume if cumulative_volume > 0 else closes[index]
        ma5 = sum(closes[max(0, index - 4) : index + 1]) / min(5, index + 1)
        if BAR_MINUTES >= 5:
            current_open = float(row["open"])
            completed_five_direction = "up" if closes[index] > current_open else "down" if closes[index] < current_open else "flat"
        elif at.minute % 5 == 4 and index >= 4:
            window = rows[index - 4 : index + 1]
            times = [parse_time(item["time"]) for item in window]
            contiguous = all(times[pos] == times[pos - 1] + timedelta(minutes=1) for pos in range(1, 5))
            if contiguous:
                first_open = float(window[0]["open"])
                last_close = float(window[-1]["close"])
                completed_five_direction = "up" if last_close > first_open else "down" if last_close < first_open else "flat"
        elapsed = index + 1
        expected = prev_volume / (240 / BAR_MINUTES) if prev_volume > 0 else 0
        volume_ratio = (cumulative_volume / elapsed) / expected if expected > 0 else 0
        dev = (closes[index] - vwap) / vwap * 100 if vwap > 0 else 0
        trend = "range"
        if dev > 1 and volume_ratio > 1.5:
            trend = "strong_up"
        elif dev < -1 and volume_ratio > 1.5:
            trend = "strong_down"
        bars.append(
            {
                "time": at,
                "hm": at.hour * 100 + at.minute,
                "open": float(row["open"]),
                "high": float(row["high"]),
                "low": float(row["low"]),
                "close": closes[index],
                "vwap": vwap,
                "ma5": ma5,
                "five": completed_five_direction,
                "trend": trend,
                "dif": dif[index],
                "day_return": (closes[index] - prev_close) / prev_close * 100 if prev_close > 0 else 0,
                "vwap_slope": (vwap - bars[-15]["vwap"]) / bars[-15]["vwap"] * 100 if len(bars) >= 15 else 0,
                "volume_ratio": volume_ratio,
            }
        )
    return {"symbol": symbol, "date": date, "bars": bars}


def prepare_sessions(raw: dict[str, list[dict]], evaluation_start: str) -> list[dict]:
    sessions = []
    for symbol, rows in raw.items():
        by_date: dict[str, list[dict]] = {}
        for row in rows:
            by_date.setdefault(row["time"][:10], []).append(row)
        previous_volume = 0.0
        previous_close = 0.0
        for date in sorted(by_date):
            day_rows = sorted(by_date[date], key=lambda item: item["time"])
            day_volume = sum(max(0.0, float(item.get("volume") or 0)) for item in day_rows)
            minimum_bars = math.floor(220 / BAR_MINUTES)
            if date >= evaluation_start and len(day_rows) >= minimum_bars and previous_volume > 0 and previous_close > 0:
                sessions.append(prepare_session(symbol, date, day_rows, previous_volume, previous_close))
            previous_volume = day_volume
            previous_close = float(day_rows[-1]["close"])
    return sessions


def in_entry_window(hm: int) -> bool:
    return (1000 <= hm < 1100) or (1330 <= hm < 1430)


def net_trade_pct(entry: float, exit_price: float, direction: str) -> float:
    gross = (entry - exit_price) / entry * 100 if direction == "sell" else (exit_price - entry) / entry * 100
    return gross - ROUND_TRIP_COST_PCT


def signal_allowed(bar: dict, config: Config, target: float, stop: float, direction: str = "sell") -> bool:
    if not in_entry_window(bar["hm"]):
        return False
    required_direction = "down" if direction == "sell" else "up"
    if config.confirm_5m and bar["five"] != required_direction:
        return False
    vetoed_trend = "strong_up" if direction == "sell" else "strong_down"
    if config.trend_veto and bar["trend"] == vetoed_trend:
        return False
    if bar["day_return"] > config.max_day_gain or bar["day_return"] < config.min_day_gain:
        return False
    if bar["vwap_slope"] < config.min_vwap_slope or bar["volume_ratio"] < config.min_volume_ratio:
        return False
    if target <= 0 or stop <= 0:
        return False
    if direction == "sell":
        if stop <= bar["close"]:
            return False
        expected = (bar["close"] - target) / bar["close"] * 100 - ROUND_TRIP_COST_PCT
    else:
        if stop >= bar["close"]:
            return False
        expected = (target - bar["close"]) / bar["close"] * 100 - ROUND_TRIP_COST_PCT
    return expected + 1e-9 >= MIN_NET_EDGE_PCT


def candidate_s2(bars: list[dict], index: int, config: Config, prev_above: bool):
    bar = bars[index]
    if prev_above or bar["close"] >= bar["ma5"] or bar["high"] < bar["ma5"]:
        return None
    window = bars[max(0, index - 19) : index + 1]
    target = min(item["low"] for item in window)
    stop = bar["ma5"] * (1 + config.s2_stop / 100)
    if signal_allowed(bar, config, target, stop):
        return {"rule": "S2", "target": target, "stop": stop, "direction": "sell"}
    return None


def candidate_s3(bars: list[dict], index: int, config: Config):
    lookback = config.s3_lookback
    if index < max(35, lookback):
        return None
    previous = bars[index - lookback : index]
    bar = bars[index]
    prev_high = max(item["close"] for item in previous)
    prev_dif_high = max(item["dif"] for item in previous)
    if bar["close"] <= prev_high or bar["dif"] > prev_dif_high:
        return None
    target = prev_high
    stop = bar["close"] * 1.003
    if signal_allowed(bar, config, target, stop):
        return {"rule": "S3", "target": target, "stop": stop, "direction": "sell"}
    return None


def simulate_session(session: dict, config: Config) -> list[Trade]:
    bars = session["bars"]
    trades: list[Trade] = []
    high_pending = False
    high_peak = 0.0
    high_since = 0
    low_pending = False
    low_trough = 0.0
    low_since = 0
    pending_entry = None
    open_trade = None
    prev_above = False

    for index, bar in enumerate(bars):
        if pending_entry is not None:
            entry = bar["open"]
            inside_plan = (
                pending_entry["target"] < entry < pending_entry["stop"]
                if pending_entry["direction"] == "sell"
                else pending_entry["stop"] < entry < pending_entry["target"]
            )
            if inside_plan:
                open_trade = {
                    **pending_entry,
                    "entry": entry,
                    "entry_index": index,
                }
            pending_entry = None

        if open_trade is not None:
            exit_price = None
            reason = ""
            if open_trade["direction"] == "sell":
                if bar["high"] >= open_trade["stop"]:
                    exit_price = max(bar["open"], open_trade["stop"])
                    reason = "stop"
                elif bar["low"] <= open_trade["target"]:
                    exit_price = open_trade["target"]
                    reason = "target"
            else:
                if bar["low"] <= open_trade["stop"]:
                    exit_price = min(bar["open"], open_trade["stop"])
                    reason = "stop"
                elif bar["high"] >= open_trade["target"]:
                    exit_price = open_trade["target"]
                    reason = "target"
            if exit_price is None and (index - open_trade["entry_index"] >= config.max_hold or bar["hm"] >= 1455 or index == len(bars) - 1):
                exit_price = bar["close"]
                reason = "time"
            if exit_price is not None:
                trades.append(
                    Trade(
                        session["symbol"],
                        session["date"],
                        open_trade["rule"],
                        open_trade["entry"],
                        exit_price,
                        net_trade_pct(open_trade["entry"], exit_price, open_trade["direction"]),
                        reason,
                        open_trade["direction"],
                    )
                )
                open_trade = None

        close_above = bar["close"] > bar["ma5"]
        if open_trade is None and pending_entry is None:
            candidate = None
            if "1" in config.rules or "6" in config.rules:
                dev = (bar["close"] - bar["vwap"]) / bar["vwap"] * 100 if bar["vwap"] > 0 else 0
                if dev >= config.dev:
                    if not high_pending:
                        high_pending = True
                        high_peak = dev
                        high_since = index
                    elif dev > high_peak:
                        high_peak = dev
                        high_since = index
                if high_pending:
                    expired = index - high_since > config.pending_minutes
                    if expired or dev <= -config.stop_dev:
                        high_pending = False
                        high_peak = 0
                    elif high_peak - dev >= config.back and dev >= MIN_NET_EDGE_PCT + ROUND_TRIP_COST_PCT:
                        if "6" in config.rules:
                            target = bar["close"] * (1 - config.take_profit / 100)
                            stop = bar["close"] * (1 + config.stop_loss / 100)
                        else:
                            target = bar["vwap"]
                            stop = bar["vwap"] * (1 + config.stop_dev / 100)
                        if signal_allowed(bar, config, target, stop):
                            candidate = {"rule": "S6" if "6" in config.rules else "S1", "target": target, "stop": stop, "direction": "sell"}
                        high_pending = False
                        high_peak = 0
            if candidate is None and "7" in config.rules:
                dev = (bar["close"] - bar["vwap"]) / bar["vwap"] * 100 if bar["vwap"] > 0 else 0
                if dev <= -config.dev:
                    if not low_pending:
                        low_pending = True
                        low_trough = dev
                        low_since = index
                    elif dev < low_trough:
                        low_trough = dev
                        low_since = index
                if low_pending:
                    expired = index - low_since > config.pending_minutes
                    if expired or dev >= config.stop_dev:
                        low_pending = False
                        low_trough = 0
                    elif dev - low_trough >= config.back and -dev >= MIN_NET_EDGE_PCT + ROUND_TRIP_COST_PCT:
                        target = bar["close"] * (1 + config.take_profit / 100)
                        stop = bar["close"] * (1 - config.stop_loss / 100)
                        if signal_allowed(bar, config, target, stop, "buy"):
                            candidate = {"rule": "S7", "target": target, "stop": stop, "direction": "buy"}
                        low_pending = False
                        low_trough = 0
            if candidate is None and "8" in config.rules and index > 0:
                previous = bars[index - 1]
                pullback_recovered = previous["close"] <= previous["ma5"] and bar["close"] > bar["ma5"]
                if pullback_recovered and bar["close"] > bar["vwap"] and bar["close"] > bar["open"]:
                    target = bar["close"] * (1 + config.take_profit / 100)
                    stop = bar["close"] * (1 - config.stop_loss / 100)
                    if signal_allowed(bar, config, target, stop, "buy"):
                        candidate = {"rule": "S8", "target": target, "stop": stop, "direction": "buy"}
            if candidate is None and "2" in config.rules:
                candidate = candidate_s2(bars, index, config, prev_above)
            if candidate is None and "3" in config.rules:
                candidate = candidate_s3(bars, index, config)
            if candidate is not None and index + 1 < len(bars):
                pending_entry = candidate
        prev_above = close_above

    return trades


def metrics(trades: list[Trade]) -> dict:
    if not trades:
        return {
            "trades": 0,
            "wins": 0,
            "win_rate": 0.0,
            "total_return": 0.0,
            "avg_net": 0.0,
            "profit_factor": 0.0,
            "max_drawdown": 0.0,
        }
    returns = [trade.net_pct / 3 for trade in trades]
    wins = sum(value > 0 for value in returns)
    total = sum(returns)
    gross_profit = sum(value for value in returns if value > 0)
    gross_loss = -sum(value for value in returns if value < 0)
    equity = 0.0
    peak = 0.0
    max_drawdown = 0.0
    for value in returns:
        equity += value
        peak = max(peak, equity)
        max_drawdown = max(max_drawdown, peak - equity)
    return {
        "trades": len(trades),
        "wins": wins,
        "win_rate": wins / len(trades) * 100,
        "total_return": total,
        "avg_net": sum(trade.net_pct for trade in trades) / len(trades),
        "profit_factor": gross_profit / gross_loss if gross_loss > 0 else 99.0,
        "max_drawdown": max_drawdown,
    }


def evaluate(sessions: list[dict], config: Config) -> tuple[dict, list[Trade]]:
    trades = []
    for session in sessions:
        trades.extend(simulate_session(session, config))
    return metrics(trades), trades


def score(result: dict) -> float:
    if result["trades"] < 30 or result["avg_net"] <= 0:
        return -10000 + result["total_return"]
    return result["total_return"] - result["max_drawdown"] * 1.5 + (result["win_rate"] - 50) * 0.08


def describe(config: Config, result: dict) -> str:
    return (
        f"rules={config.rules} dev={config.dev:.2f} back={config.back:.2f} stop={config.stop_dev:.2f} "
        f"tp={config.take_profit:.2f} sl={config.stop_loss:.2f} daycap={config.max_day_gain:.1f} "
        f"daymin={config.min_day_gain:.1f} slope={config.min_vwap_slope:.2f} vol={config.min_volume_ratio:.1f} "
        f"hold={config.max_hold * BAR_MINUTES}m 5m={int(config.confirm_5m)} veto={int(config.trend_veto)} "
        f"trades={result['trades']} win={result['win_rate']:.2f}% return={result['total_return']:.2f}% "
        f"avg={result['avg_net']:.3f}% pf={result['profit_factor']:.2f} dd={result['max_drawdown']:.2f}%"
    )


def run_buy_research(train: list[dict], validation: list[dict], sessions: list[dict]) -> None:
    hold_values = tuple(max(1, minutes // BAR_MINUTES) for minutes in (15, 30, 60))
    pending_values = (1, 2, 3) if BAR_MINUTES == 5 else (3, 5, 8)
    reversal_grid = (
        Config(
            "7",
            dev=dev,
            back=back,
            stop_dev=dev + 0.4,
            confirm_5m=confirm,
            trend_veto=True,
            max_hold=hold,
            pending_minutes=pending,
            take_profit=take_profit,
            stop_loss=stop_loss,
            max_day_gain=3.0,
            min_day_gain=min_day_gain,
        )
        for dev, back, take_profit, stop_loss, confirm, hold, pending, min_day_gain in product(
            (1.2, 1.6),
            (0.4, 0.6),
            (0.5, 0.6, 0.8),
            (0.5, 0.8, 1.2),
            (True, False),
            hold_values,
            pending_values[:2],
            (-2.0, -1.0),
        )
    )
    momentum_grid = (
        Config(
            "8",
            confirm_5m=False,
            trend_veto=True,
            max_hold=hold,
            take_profit=take_profit,
            stop_loss=stop_loss,
            max_day_gain=max_day_gain,
            min_day_gain=0.0,
            min_vwap_slope=slope,
            min_volume_ratio=volume_ratio,
        )
        for take_profit, stop_loss, hold, max_day_gain, slope, volume_ratio in product(
            (0.5, 0.6, 0.8, 1.0),
            (0.4, 0.6, 0.8, 1.0),
            hold_values,
            (2.0, 4.0, 6.0),
            (0.0, 0.1, 0.2),
            (0.0, 1.0),
        )
    )
    ranked = []
    for label, grid in (("reversal", reversal_grid), ("momentum", momentum_grid)):
        for index, config in enumerate(grid, start=1):
            result, _ = evaluate(train, config)
            ranked.append((score(result), config, result))
            if index % 500 == 0:
                print(f"evaluated {index} {label} configs", file=sys.stderr)
    ranked.sort(reverse=True, key=lambda item: item[0])
    print("\nBUY-FIRST TRAIN")
    for _, config, result in ranked[:20]:
        print(describe(config, result))

    print("\nBUY-FIRST VALIDATION")
    evaluated = []
    for _, config, train_result in ranked[:30]:
        validation_result, trades = evaluate(validation, config)
        evaluated.append((score(validation_result), config, train_result, validation_result, trades))
        print("TRAIN", describe(config, train_result))
        print("VALID", describe(config, validation_result))
    stable = [item for item in evaluated if item[2]["avg_net"] > 0 and item[3]["avg_net"] > 0 and item[3]["trades"] >= 20]
    if not stable:
        print("\nNO_STABLE_BUY_CONFIG")
        return
    stable.sort(reverse=True, key=lambda item: min(item[2]["total_return"], item[3]["total_return"]))
    _, selected, train_result, validation_result, validation_trades = stable[0]
    combined_result, _ = evaluate(sessions, selected)
    print("\nSELECTED_BUY")
    print("TRAIN", describe(selected, train_result))
    print("VALID", describe(selected, validation_result))
    print("ALL  ", describe(selected, combined_result))
    reasons = {}
    for trade in validation_trades:
        reasons[trade.reason] = reasons.get(trade.reason, 0) + 1
    print("VALIDATION_EXITS", json.dumps(reasons, ensure_ascii=False, sort_keys=True))


def main() -> int:
    global BAR_MINUTES
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="data/market/a_share_daily.db")
    parser.add_argument("--symbols", type=int, default=48)
    parser.add_argument("--query-start", default="2026-07-27")
    parser.add_argument("--start", default="2026-08-03")
    parser.add_argument("--split", default="2026-09-01")
    parser.add_argument("--end", default="2026-09-18")
    parser.add_argument("--period", choices=("minute1", "minute5"), default="minute1")
    parser.add_argument("--mode", choices=("sell", "buy"), default="sell")
    args = parser.parse_args()
    BAR_MINUTES = 5 if args.period == "minute5" else 1

    universe = load_universe(args.db, args.symbols)
    print(f"universe={len(universe)} first={','.join(universe[:8])}", file=sys.stderr)
    raw = fetch_universe(universe, args.query_start, args.end, args.period)
    sessions = prepare_sessions(raw, args.start)
    train = [session for session in sessions if session["date"] < args.split]
    validation = [session for session in sessions if session["date"] >= args.split]
    print(
        f"sessions={len(sessions)} train={len(train)} validation={len(validation)} "
        f"symbols={len(set(session['symbol'] for session in sessions))}",
        file=sys.stderr,
    )

    if args.mode == "buy":
        run_buy_research(train, validation, sessions)
        return 0

    baselines = [
        Config(rules=rules, confirm_5m=confirm, trend_veto=veto)
        for rules, confirm, veto in product(("1", "2", "3", "12", "13", "123"), (True, False), (True, False))
    ]
    baseline_results = []
    for config in baselines:
        result, _ = evaluate(train, config)
        baseline_results.append((score(result), config, result))
    print("\nBASELINE TRAIN")
    for _, config, result in sorted(baseline_results, reverse=True, key=lambda item: item[0])[:12]:
        print(describe(config, result))

    hold_values = tuple(max(1, minutes // BAR_MINUTES) for minutes in (30, 60, 120))
    pending_values = (1, 2) if BAR_MINUTES == 5 else (3, 5)
    grid = (
        Config("1", dev, back, stop_dev, confirm_5m=confirm, trend_veto=veto, max_hold=hold, pending_minutes=pending)
        for dev, back, stop_dev, confirm, veto, hold, pending in product(
            (0.5, 0.7, 0.9, 1.1, 1.3),
            (0.1, 0.2, 0.3, 0.4),
            (0.8, 1.0, 1.2, 1.5),
            (True, False),
            (True, False),
            hold_values,
            pending_values,
        )
        if stop_dev > dev
    )
    ranked = []
    for index, config in enumerate(grid, start=1):
        result, _ = evaluate(train, config)
        ranked.append((score(result), config, result))
        if index % 250 == 0:
            print(f"evaluated {index} configs", file=sys.stderr)
    ranked.sort(reverse=True, key=lambda item: item[0])
    print("\nS1 GRID TRAIN")
    for _, config, result in ranked[:15]:
        print(describe(config, result))

    scalp_grid = (
        Config(
            "6",
            dev=dev,
            back=back,
            stop_dev=max(dev + 0.2, stop_loss),
            confirm_5m=confirm,
            trend_veto=True,
            max_hold=hold,
            pending_minutes=5,
            take_profit=take_profit,
            stop_loss=stop_loss,
            max_day_gain=max_day_gain,
        )
        for dev, back, take_profit, stop_loss, confirm, hold, max_day_gain in product(
            (0.8, 1.2, 1.6),
            (0.2, 0.4, 0.6),
            (0.5, 0.6, 0.8),
            (0.5, 0.8, 1.2),
            (True, False),
            tuple(max(1, minutes // BAR_MINUTES) for minutes in (15, 30, 60)),
            (1.0, 2.0, 3.0),
        )
    )
    scalp_ranked = []
    for index, config in enumerate(scalp_grid, start=1):
        result, _ = evaluate(train, config)
        scalp_ranked.append((score(result), config, result))
        if index % 250 == 0:
            print(f"evaluated {index} scalp configs", file=sys.stderr)
    scalp_ranked.sort(reverse=True, key=lambda item: item[0])
    print("\nS6 SCALP TRAIN")
    for _, config, result in scalp_ranked[:15]:
        print(describe(config, result))

    ranked = scalp_ranked + ranked
    ranked.sort(reverse=True, key=lambda item: item[0])

    print("\nTOP CONFIGS VALIDATION")
    validation_results = []
    seen = set()
    for _, config, train_result in ranked:
        signature = (
            config.rules,
            config.dev,
            config.back,
            config.stop_dev,
            config.confirm_5m,
            config.trend_veto,
            config.max_hold,
            config.pending_minutes,
            config.take_profit,
            config.stop_loss,
            config.max_day_gain,
        )
        if signature in seen:
            continue
        seen.add(signature)
        validation_result, trades = evaluate(validation, config)
        validation_results.append((score(validation_result), config, train_result, validation_result, trades))
        if len(validation_results) >= 30:
            break
    validation_results.sort(reverse=True, key=lambda item: item[0])
    for _, config, train_result, validation_result, _ in validation_results[:15]:
        print("TRAIN", describe(config, train_result))
        print("VALID", describe(config, validation_result))

    best = validation_results[0]
    _, best_config, train_result, validation_result, validation_trades = best
    combined_result, _ = evaluate(sessions, best_config)
    print("\nSELECTED")
    print("TRAIN", describe(best_config, train_result))
    print("VALID", describe(best_config, validation_result))
    print("ALL  ", describe(best_config, combined_result))
    reasons = {}
    rules = {}
    for trade in validation_trades:
        reasons[trade.reason] = reasons.get(trade.reason, 0) + 1
        rules[trade.rule] = rules.get(trade.rule, 0) + 1
    print("VALIDATION_EXITS", json.dumps(reasons, ensure_ascii=False, sort_keys=True))
    print("VALIDATION_RULES", json.dumps(rules, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
