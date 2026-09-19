#!/usr/bin/env python3

"""Calibrate conservative sell-first intraday signals for one subscribed stock.

The optimizer deliberately keeps the final time segment untouched. Candidate
selection only sees the training and validation segments; the test segment is
evaluated once after a configuration has been selected.
"""

import argparse
import json
import math
import random
import urllib.parse
import urllib.request
from dataclasses import asdict, dataclass
from datetime import datetime, timedelta
from pathlib import Path


HIKYUU_URL = "http://localhost:18091/api/hikyuu/kline"
ROUND_TRIP_COST_PCT = 0.20


@dataclass(frozen=True)
class Config:
    family: str
    session: str
    peak_dev: float
    pullback: float
    min_dev: float
    min_day_gain: float
    max_day_gain: float
    min_rsi: float
    confirmation: str
    max_vwap_slope: float
    max_volume_ratio: float
    min_previous_day_return: float
    max_previous_day_return: float
    min_gap: float
    max_gap: float
    take_profit: float
    stop_loss: float
    max_hold_bars: int


@dataclass
class Trade:
    date: str
    signal_time: str
    entry_time: str
    exit_time: str
    entry: float
    exit: float
    net_pct: float
    reason: str


def fetch_rows(symbol: str, start: str, end: str, cache: Path) -> list[dict]:
    if cache.exists():
        return json.loads(cache.read_text())
    query = urllib.parse.urlencode(
        {
            "code": symbol,
            "type": "minute5",
            "start": start,
            "end": end,
            "limit": 100000,
            "recover": "none",
        }
    )
    with urllib.request.urlopen(f"{HIKYUU_URL}?{query}", timeout=180) as response:
        envelope = json.load(response)
    if envelope.get("code") != 0:
        raise RuntimeError(envelope.get("message") or "Hikyuu query failed")
    rows = envelope["data"].get("list", [])
    cache.parent.mkdir(parents=True, exist_ok=True)
    cache.write_text(json.dumps(rows, ensure_ascii=False))
    return rows


def rsi_series(values: list[float], period: int = 6) -> list[float]:
    result = [50.0] * len(values)
    if len(values) <= period:
        return result
    gains = [max(0.0, values[index] - values[index - 1]) for index in range(1, len(values))]
    losses = [max(0.0, values[index - 1] - values[index]) for index in range(1, len(values))]
    avg_gain = sum(gains[:period]) / period
    avg_loss = sum(losses[:period]) / period
    result[period] = 100.0 if avg_loss == 0 else 100 - 100 / (1 + avg_gain / avg_loss)
    for index in range(period + 1, len(values)):
        avg_gain = (avg_gain * (period - 1) + gains[index - 1]) / period
        avg_loss = (avg_loss * (period - 1) + losses[index - 1]) / period
        result[index] = 100.0 if avg_loss == 0 else 100 - 100 / (1 + avg_gain / avg_loss)
    return result


def prepare_sessions(rows: list[dict]) -> list[dict]:
    grouped: dict[str, list[dict]] = {}
    for row in rows:
        grouped.setdefault(row["time"][:10], []).append(row)

    sessions = []
    previous_close = 0.0
    previous_volume = 0.0
    previous_day_return = 0.0
    for date in sorted(grouped):
        day_rows = sorted(grouped[date], key=lambda item: item["time"])
        if len(day_rows) < 44:
            if day_rows:
                previous_close = float(day_rows[-1]["close"])
                previous_volume = sum(float(item.get("volume") or 0) for item in day_rows)
            continue
        closes = [float(row["close"]) for row in day_rows]
        rsis = rsi_series(closes)
        cumulative_volume = 0.0
        cumulative_amount = 0.0
        day_high = 0.0
        peak_dev = -999.0
        bars = []
        for index, row in enumerate(day_rows):
            at = datetime.fromisoformat(row["time"]) - timedelta(minutes=5)
            volume = max(0.0, float(row.get("volume") or 0))
            amount = max(0.0, float(row.get("amount") or 0)) * 10000
            cumulative_volume += volume
            cumulative_amount += amount
            close = float(row["close"])
            high = float(row["high"])
            day_high = max(day_high, high)
            vwap = cumulative_amount / cumulative_volume if cumulative_volume > 0 else close
            dev = (close - vwap) / vwap * 100 if vwap > 0 else 0.0
            peak_dev = max(peak_dev, dev)
            elapsed = index + 1
            expected = previous_volume / 48 if previous_volume > 0 else 0.0
            volume_ratio = (cumulative_volume / elapsed) / expected if expected > 0 else 0.0
            vwap_slope = 0.0
            if index >= 3 and bars[index - 3]["vwap"] > 0:
                vwap_slope = (vwap - bars[index - 3]["vwap"]) / bars[index - 3]["vwap"] * 100
            bars.append(
                {
                    "time": at,
                    "hm": at.hour * 100 + at.minute,
                    "open": float(row["open"]),
                    "high": high,
                    "low": float(row["low"]),
                    "close": close,
                    "vwap": vwap,
                    "dev": dev,
                    "peak_dev": peak_dev,
                    "dev_pullback": peak_dev - dev,
                    "high_pullback": (day_high - close) / day_high * 100 if day_high > 0 else 0.0,
                    "day_return": (close - previous_close) / previous_close * 100 if previous_close > 0 else 0.0,
                    "rsi": rsis[index],
                    "vwap_slope": vwap_slope,
                    "volume_ratio": volume_ratio,
                    "gap": (
                        (float(day_rows[0]["open"]) - previous_close) / previous_close * 100
                        if previous_close > 0
                        else 0.0
                    ),
                }
            )
        if previous_close > 0 and previous_volume > 0:
            sessions.append(
                {
                    "date": date,
                    "bars": bars,
                    "previous_day_return": previous_day_return,
                }
            )
        current_close = float(day_rows[-1]["close"])
        previous_day_return = (
            (current_close - previous_close) / previous_close * 100 if previous_close > 0 else 0.0
        )
        previous_close = current_close
        previous_volume = sum(float(item.get("volume") or 0) for item in day_rows)
    return sessions


def in_session(hm: int, session: str) -> bool:
    if session == "morning":
        return 1000 <= hm < 1100
    if session == "afternoon":
        return 1330 <= hm < 1430
    return (1000 <= hm < 1100) or (1330 <= hm < 1430)


def is_signal(bars: list[dict], index: int, config: Config) -> bool:
    if index < 6 or index + 1 >= len(bars):
        return False
    bar = bars[index]
    previous = bars[index - 1]
    if not in_session(bar["hm"], config.session):
        return False
    if not (config.min_day_gain <= bar["day_return"] <= config.max_day_gain):
        return False
    if bar["peak_dev"] < config.peak_dev or bar["dev"] < config.min_dev:
        return False
    if bar["rsi"] < config.min_rsi:
        return False
    if bar["vwap_slope"] > config.max_vwap_slope:
        return False
    if config.max_volume_ratio > 0 and bar["volume_ratio"] > config.max_volume_ratio:
        return False
    if not (config.min_gap <= bar["gap"] <= config.max_gap):
        return False
    if config.family == "vwap" and bar["dev_pullback"] < config.pullback:
        return False
    if config.family == "high" and bar["high_pullback"] < config.pullback:
        return False
    if config.family == "rsi":
        if previous["rsi"] < config.min_rsi or bar["rsi"] >= previous["rsi"]:
            return False
        if max(bar["dev_pullback"], bar["high_pullback"]) < config.pullback:
            return False
    if config.confirmation == "red" and bar["close"] >= bar["open"]:
        return False
    if config.confirmation == "lower" and bar["close"] >= previous["close"]:
        return False
    if config.confirmation == "two_lower":
        if index < 2 or bar["close"] >= previous["close"] or previous["close"] >= bars[index - 2]["close"]:
            return False
    return True


def simulate_session(session: dict, config: Config) -> Trade | None:
    if not (
        config.min_previous_day_return
        <= session["previous_day_return"]
        <= config.max_previous_day_return
    ):
        return None
    bars = session["bars"]
    signal_index = -1
    for index in range(len(bars) - 1):
        if is_signal(bars, index, config):
            signal_index = index
            break
    if signal_index < 0:
        return None
    entry_index = signal_index + 1
    entry = bars[entry_index]["open"]
    if entry <= 0:
        return None
    target = entry * (1 - config.take_profit / 100)
    stop = entry * (1 + config.stop_loss / 100)
    exit_price = bars[-1]["close"]
    exit_index = len(bars) - 1
    reason = "close"
    for index in range(entry_index, len(bars)):
        bar = bars[index]
        if bar["high"] >= stop:
            exit_price = max(bar["open"], stop)
            exit_index = index
            reason = "stop"
            break
        if bar["low"] <= target:
            exit_price = min(bar["open"], target)
            exit_index = index
            reason = "target"
            break
        if index - entry_index >= config.max_hold_bars or bar["hm"] >= 1455:
            exit_price = bar["close"]
            exit_index = index
            reason = "time"
            break
    net_pct = (entry - exit_price) / entry * 100 - ROUND_TRIP_COST_PCT
    return Trade(
        date=session["date"],
        signal_time=bars[signal_index]["time"].isoformat(),
        entry_time=bars[entry_index]["time"].isoformat(),
        exit_time=bars[exit_index]["time"].isoformat(),
        entry=entry,
        exit=exit_price,
        net_pct=net_pct,
        reason=reason,
    )


def evaluate(sessions: list[dict], config: Config) -> tuple[dict, list[Trade]]:
    trades = []
    for session in sessions:
        trade = simulate_session(session, config)
        if trade is not None:
            trades.append(trade)
    return metrics(trades), trades


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
    position_returns = [trade.net_pct / 3 for trade in trades]
    wins = sum(value > 0 for value in position_returns)
    profit = sum(value for value in position_returns if value > 0)
    loss = -sum(value for value in position_returns if value < 0)
    equity = 0.0
    peak = 0.0
    drawdown = 0.0
    for value in position_returns:
        equity += value
        peak = max(peak, equity)
        drawdown = max(drawdown, peak - equity)
    return {
        "trades": len(trades),
        "wins": wins,
        "win_rate": wins / len(trades) * 100,
        "total_return": sum(position_returns),
        "avg_net": sum(trade.net_pct for trade in trades) / len(trades),
        "profit_factor": profit / loss if loss > 0 else 99.0,
        "max_drawdown": drawdown,
    }


def rank_score(result: dict, minimum_trades: int) -> float:
    if result["trades"] < minimum_trades or result["total_return"] <= 0 or result["profit_factor"] <= 1:
        return -10000 + result["total_return"]
    return (
        result["total_return"]
        - 1.5 * result["max_drawdown"]
        + 0.05 * (result["win_rate"] - 50)
        + min(2.0, result["profit_factor"]) * 0.25
    )


def random_configs(count: int, seed: int) -> list[Config]:
    rng = random.Random(seed)
    choices = {
        "family": ("vwap", "high", "rsi"),
        "session": ("morning", "afternoon", "both"),
        "peak_dev": (0.3, 0.6, 0.8, 1.0, 1.2, 1.5, 1.8, 2.2, 2.6, 3.0),
        "pullback": (0.0, 0.15, 0.25, 0.35, 0.5, 0.7, 0.9, 1.2),
        "min_dev": (0.0, 0.3, 0.5, 0.7, 0.9, 1.2, 1.5),
        "min_day_gain": (0.0, 0.5, 1.0, 1.5, 2.0, 3.0, 4.0, 5.0),
        "max_day_gain": (5.0, 7.0, 10.0, 15.0, 25.0),
        "min_rsi": (0.0, 50.0, 60.0, 65.0, 70.0, 75.0, 80.0),
        "confirmation": ("none", "red", "lower", "two_lower"),
        "max_vwap_slope": (0.0, 0.1, 0.2, 0.35, 0.5, 1.0, 99.0),
        "max_volume_ratio": (0.0, 1.0, 1.5, 2.0, 3.0, 5.0),
        "min_previous_day_return": (-20.0, -5.0, -2.0, 0.0, 2.0),
        "max_previous_day_return": (0.0, 2.0, 5.0, 10.0, 20.0),
        "min_gap": (-20.0, -3.0, -1.0, 0.0, 1.0),
        "max_gap": (0.0, 1.0, 3.0, 5.0, 10.0, 20.0),
        "take_profit": (0.4, 0.5, 0.6, 0.8, 1.0, 1.2),
        "stop_loss": (0.6, 0.8, 1.0, 1.2, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0),
        "max_hold_bars": (3, 6, 12, 18, 30, 60),
    }
    configs = set()
    while len(configs) < count:
        values = {key: rng.choice(options) for key, options in choices.items()}
        if len(configs) < count // 2:
            values.update(
                {
                    "min_previous_day_return": -20.0,
                    "max_previous_day_return": 20.0,
                    "min_gap": -20.0,
                    "max_gap": 20.0,
                }
            )
        config = Config(**values)
        if (
            config.min_dev > config.peak_dev
            or config.max_day_gain <= config.min_day_gain
            or config.max_previous_day_return <= config.min_previous_day_return
            or config.max_gap <= config.min_gap
        ):
            continue
        configs.add(config)
    return list(configs)


def neighborhood_configs(base: Config) -> list[Config]:
    """Small perturbations used to reject isolated, overfit random winners."""
    configs = set()
    for peak_dev in (base.peak_dev - 0.3, base.peak_dev, base.peak_dev + 0.3):
        for pullback in (max(0.0, base.pullback - 0.15), base.pullback, base.pullback + 0.15):
            for min_dev in (max(0.0, base.min_dev - 0.2), base.min_dev, base.min_dev + 0.2):
                for min_day_gain in (max(0.0, base.min_day_gain - 0.5), base.min_day_gain, base.min_day_gain + 0.5):
                    for min_rsi in (max(0.0, base.min_rsi - 5), base.min_rsi, min(100.0, base.min_rsi + 5)):
                        for take_profit in (max(0.4, base.take_profit - 0.1), base.take_profit, base.take_profit + 0.1):
                            for stop_loss in (max(0.6, base.stop_loss - 1.0), base.stop_loss, base.stop_loss + 1.0):
                                configs.add(
                                    Config(
                                        **{
                                            **asdict(base),
                                            "peak_dev": round(peak_dev, 2),
                                            "pullback": round(pullback, 2),
                                            "min_dev": round(min_dev, 2),
                                            "min_day_gain": round(min_day_gain, 2),
                                            "min_rsi": round(min_rsi, 2),
                                            "take_profit": round(take_profit, 2),
                                            "stop_loss": round(stop_loss, 2),
                                        }
                                    )
                                )
    return list(configs)


def summary(label: str, result: dict) -> str:
    return (
        f"{label}: trades={result['trades']} wins={result['wins']} "
        f"win={result['win_rate']:.2f}% return={result['total_return']:.2f}% "
        f"avg_net={result['avg_net']:.3f}% pf={result['profit_factor']:.2f} "
        f"max_dd={result['max_drawdown']:.2f}%"
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--symbol", default="sz300464")
    parser.add_argument("--start", default="2024-09-01")
    parser.add_argument("--end", default="2026-09-19")
    parser.add_argument("--train-end", default="2025-11-01")
    parser.add_argument("--validation-end", default="2026-05-01")
    parser.add_argument("--configs", type=int, default=12000)
    parser.add_argument("--seed", type=int, default=20260919)
    parser.add_argument("--top", type=int, default=160)
    parser.add_argument("--show-test", action="store_true")
    args = parser.parse_args()

    root = Path(__file__).resolve().parent
    rows = fetch_rows(args.symbol, args.start, args.end, root / f"{args.symbol}-minute5.json")
    sessions = prepare_sessions(rows)
    train = [session for session in sessions if session["date"] < args.train_end]
    validation = [
        session for session in sessions if args.train_end <= session["date"] < args.validation_end
    ]
    test = [session for session in sessions if session["date"] >= args.validation_end]
    print(
        f"bars={len(rows)} sessions={len(sessions)} train={len(train)} "
        f"validation={len(validation)} test={len(test)}"
    )

    ranked = []
    for index, config in enumerate(random_configs(args.configs, args.seed), start=1):
        result, _ = evaluate(train, config)
        ranked.append((rank_score(result, 30), config, result))
        if index % 1000 == 0:
            print(f"trained {index}/{args.configs}")
    ranked.sort(key=lambda item: item[0], reverse=True)

    validated = []
    for _, config, train_result in ranked[: args.top]:
        validation_result, _ = evaluate(validation, config)
        stable_score = min(
            rank_score(train_result, 30),
            rank_score(validation_result, 10),
        )
        validated.append((stable_score, config, train_result, validation_result))
    validated.sort(key=lambda item: item[0], reverse=True)

    print("\nTOP_VALIDATED")
    for _, config, train_result, validation_result in validated[:15]:
        print(json.dumps(asdict(config), sort_keys=True))
        print(summary("TRAIN", train_result))
        print(summary("VALID", validation_result))

    stable = [
        item
        for item in validated
        if item[2]["trades"] >= 20
        and item[3]["trades"] >= 10
        and item[2]["total_return"] > 0
        and item[3]["total_return"] > 0
        and item[2]["profit_factor"] > 1
        and item[3]["profit_factor"] > 1
    ]
    print("\nTOP_STABLE")
    for _, config, train_result, validation_result in stable[:10]:
        print(json.dumps(asdict(config), sort_keys=True))
        print(summary("TRAIN", train_result))
        print(summary("VALID", validation_result))

    if not stable:
        print("NO_STABLE_CONFIG")
        return 2

    _, preliminary, _, _ = stable[0]
    neighborhood = []
    for config in neighborhood_configs(preliminary):
        train_result, _ = evaluate(train, config)
        validation_result, _ = evaluate(validation, config)
        if (
            train_result["trades"] >= 20
            and validation_result["trades"] >= 8
            and train_result["total_return"] > 0
            and validation_result["total_return"] > 0
        ):
            neighborhood.append((
                min(rank_score(train_result, 20), rank_score(validation_result, 8)),
                config,
                train_result,
                validation_result,
            ))
    neighborhood.sort(key=lambda item: item[0], reverse=True)
    if len(neighborhood) < 20:
        print(f"UNSTABLE_NEIGHBORHOOD count={len(neighborhood)}")
        return 3
    _, selected, train_result, validation_result = neighborhood[0]
    print(f"\nSTABLE_NEIGHBORHOOD passing={len(neighborhood)}/{len(neighborhood_configs(preliminary))}")
    output = {
        "symbol": args.symbol,
        "selection": asdict(selected),
        "train": train_result,
        "validation": validation_result,
        "periods": {
            "train": [sessions[0]["date"], train[-1]["date"]],
            "validation": [validation[0]["date"], validation[-1]["date"]],
            "test": [test[0]["date"], test[-1]["date"]],
        },
    }
    if args.show_test:
        test_result, test_trades = evaluate(test, selected)
        all_result, _ = evaluate(sessions, selected)
        output["test"] = test_result
        output["all"] = all_result
        output["test_trades"] = [asdict(trade) for trade in test_trades]
        print("\nUNTOUCHED_TEST")
        print(summary("TEST", test_result))
        print(summary("ALL", all_result))
    (root / "selected-symbol-strategy.json").write_text(
        json.dumps(output, ensure_ascii=False, indent=2)
    )
    print("\nSELECTED")
    print(json.dumps(output, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
