import unittest
from datetime import datetime

import freshness_runner


class FakeDatetime:
    def __init__(self, value):
        self.value = value

    def datetime(self):
        return self.value


class FakeRecord:
    def __init__(self, value):
        self.datetime = FakeDatetime(datetime.strptime(value, "%Y-%m-%d"))


class FakeStock:
    def __init__(self, market, stock_type, latest=None, valid=True):
        self.market = market
        self.type = stock_type
        self.valid = valid
        self.market_code = market + "000001"
        self.records = [] if latest is None else [FakeRecord(latest)]

    def get_kdata(self, _query):
        return self.records


class FakeQuery:
    DAY = "DAY"

    def __init__(self, start, ktype=None):
        self.start = start
        self.ktype = ktype


class FreshnessRunnerTest(unittest.TestCase):
    def test_scan_market_freshness_checks_each_market_coverage(self):
        stocks = []
        for market, stock_type in (("SH", 1), ("SZ", 1), ("BJ", 11)):
            stocks.extend(FakeStock(market, stock_type, "2026-09-17") for _ in range(19))
            stocks.append(FakeStock(market, stock_type, "2026-09-16"))

        result = freshness_runner.scan_market_freshness(stocks, FakeQuery, "2026-09-17", 0.95)

        self.assertTrue(result["ok"])
        for market in ("SH", "SZ", "BJ"):
            self.assertEqual(result["markets"][market]["fresh_count"], 19)
            self.assertEqual(result["markets"][market]["stale_count"], 1)
            self.assertEqual(result["markets"][market]["coverage"], 0.95)
            self.assertTrue(result["markets"][market]["ok"])

    def test_scan_market_freshness_fails_when_one_market_is_stale(self):
        stocks = [
            FakeStock("SH", 1, "2026-09-17"),
            FakeStock("SZ", 1, "2026-09-16"),
            FakeStock("BJ", 11, "2026-09-17"),
            FakeStock("SH", 5, "2026-09-17"),
        ]

        result = freshness_runner.scan_market_freshness(stocks, FakeQuery, "2026-09-17", 0.95)

        self.assertFalse(result["ok"])
        self.assertTrue(result["markets"]["SH"]["ok"])
        self.assertFalse(result["markets"]["SZ"]["ok"])
        self.assertEqual(result["markets"]["SH"]["total"], 1)


if __name__ == "__main__":
    unittest.main()
