# 日线短线策略研究候选

本批策略是可回测的**研究候选**，默认停用。公开资料提供的是形态与交易思路，不构成这些参数在 A 股有效的证据。历史回测页面可手动选择停用的研究模板；默认选择仍只包含已启用策略。

| 模板 | 公开来源 | 本项目的日线改写 |
| --- | --- | --- |
| NR7收盘突破 | [StockCharts NR7](https://chartschool.stockcharts.com/table-of-contents/trading-strategies-and-models/trading-strategies/narrow-range-day-nr7) | 前一日高低价差严格小于此前 6 日；信号日收盘高于前日高点 0.3% 至 5%，量达前 5 日均量 1.2 倍且收盘在 MA20 上。原文是盘中突破，改写为收盘确认。 |
| 箱体假跌破收回 | [StockCharts Wyckoff 教程](https://chartschool.stockcharts.com/table-of-contents/market-analysis/wyckoff-analysis-articles/the-wyckoff-method-a-tutorial) | 今日最低价跌破前 20 日支撑 0.1% 至 3%，收盘收回支撑上方至少 0.5%，位于日内区间上 35%，前 20 日箱体振幅不超 25%。仅是 spring 的 OHLCV 代理，不识别完整吸筹阶段。 |
| 低开收复 | [StockCharts 缺口交易](https://chartschool.stockcharts.com/table-of-contents/trading-strategies-and-models/trading-strategies/gap-trading-strategies) | 今日开盘较昨收低 1% 至 4%，收盘高于昨收至少 0.3%，位于日内区间上 30%，量达前 5 日均量 1.1 倍且收盘在 MA20 上。原文多为开盘后盘中触发，改写为收盘确认。 |

三个模板均要求当日成交额至少 2 亿元、价格在 3 至 100 元，默认排除用户排除池、科创板、北交所。每天最多保留统一排名前 10 个命中股票，因此回测统计衡量的是“形态条件加项目统一排名”的组合。信号日在收盘后才能确定；组合回测按下一交易日开盘及既有 A 股成交、T+1 与费用规则模拟，不能用信号日开盘或最低价买入。形态因子只读取信号日及之前的 K 线。

## 验证口径

在启用自动选股前，应分别检验 2021 至 2023、2024 至 2025、2026 三段（不要用后段反复调参），查看 D1/D3/D5/D10 的收盘达标率、均值、中位数、最大回撤、净收益及信号数。与同日随机股票、风格匹配股票和四个指数对照；记录涨停无法成交、停牌、滑点和费用。若结果只在单一年份或少数股票成立，继续保留停用状态。模板参数是研究起点，并非已经验证的最佳参数。
