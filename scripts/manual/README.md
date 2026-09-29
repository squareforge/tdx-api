# Manual scripts

These scripts are run on demand against local services. They are not part of the automated test suite.

| Script | Purpose |
| --- | --- |
| `run_api_checks.py` | Smoke check the HTTP API. |
| `api_usage_example.py` | Demonstrate API calls. |
| `backtest_002202_strategy.py` | Explore a daily strategy on symbol 002202. |
| `backtest_a_share_pullback_v2.py` | Explore the older trend-pullback strategy. |
| `track_daily_signal_quotes.py` | Track quotes from `reports/signals_<date>.csv`. Its former signal producer was removed, so supply this CSV separately before running it. |

Run scripts from the repository root, for example `python scripts/manual/run_api_checks.py`.
