# 策略信号分析与模拟跟踪实施方案

> 版本：V1.0  
> 日期：2026-09-24  
> 适用项目：`tdx-api`  
> 主要场景：策略在交易日 14:45 左右产生选股信号，对信号之后 1、3、5、10 个交易日的表现进行可复现、可交易、可比较的分析。

## 1. 结论先行

目前的方向是有价值的，但不能只做“信号日收盘价与未来收盘价比较”。这种做法可以快速看到股票后来涨没涨，却无法回答下面几个关键问题：

1. 信号在 14:45 产生时，策略当时真正看到了什么价格？
2. 如果按可执行规则模拟买入，实际能以什么价格成交？
3. 收益来自策略本身，还是来自大盘、行业或特定市场环境？
4. 持有期间是否先大跌再上涨，策略的风险是否可接受？
5. 参数或样本稍微变化后，结论是否仍然成立？

推荐把分析系统拆成四层：

1. **信号层**：保存策略触发瞬间的事实，之后不可覆盖。
2. **执行层**：按明确规则生成模拟成交，不等同于真实下单。
3. **路径层**：保存 D0、D1、D2……每天的价格和基准表现，支持重算。
4. **汇总层**：生成 D1/D3/D5/D10 收益、MFE、MAE、最大回撤、胜率等指标。

第一阶段只做纸面交易（paper trading），不要自动向券商发送真实委托。等数据稳定、执行规则和风险控制通过验证后，再单独设计真实交易模块。

## 2. 当前项目评估

### 2.1 已有能力

项目已经具备一个不错的基础：

- `selection_results` 保存每次策略运行命中的股票，包含 `latest`、`detail_json` 和 `created_at`。
- `selection_result_tracking` 已按 `selection_result_id + horizon_days` 保存 D1/D3/D5/D10 汇总。
- `EvaluateSelectionTracking` 已计算周期末开盘/收盘收益、最大上涨、最大下跌、止盈和止损触达。
- 自动化任务已经支持每天补算未完成周期。
- 历史回测和每日复盘已经消费这些跟踪结果。

主要代码位置：

- `packages/workbench-core/store.go`
- `packages/workbench-core/selection_tracking.go`
- `packages/workbench-core/historical_backtest_store.go`
- `packages/workbench-core/daily_review_store.go`
- `services/selection-worker/strategy_runner.go`
- `services/selection-worker/automation_runner.go`
- `services/selection-worker/selection_tracking_api.go`

### 2.2 当前主要问题

#### 2.2.1 `latest` 的业务语义不够明确

`selection_results.latest` 是目前最接近信号价的字段，但无法单独证明：

- 它是不是 14:45 的实时 `lastPrice`；
- 行情的实际时间是什么；
- 数据来自实时快照、分钟 K 线还是日 K 线；
- 行情是否已经过期；
- 使用了前复权、不复权还是其他价格口径。

所以不能直接把 `latest` 永久解释为可靠的模拟成交价。

#### 2.2.2 14:45 信号当前可能错误使用收盘价作为基准

当前 `EvaluateSelectionTracking` 对 09:30 以后产生的信号，会优先把信号日的日 K 收盘价设为 `signal_day_close`。对 14:45 的信号来说，15:00 收盘价在触发时还未知，因此它可以作为 **D0 收盘观察价**，但不应冒充 **信号价** 或 **成交价**。

#### 2.2.3 当前 `max_drawdown` 实际更接近 MAE

现有计算是：

```text
min((每日最低价 / 基准价) - 1)
```

这个指标表示持仓相对买入成本的最大不利波动，标准名称是 **MAE（Maximum Adverse Excursion）**。真正的最大回撤应计算持有路径中任意历史高点到后续低点的最大跌幅：

```text
MaxDrawdown = min((当前价格 / 截至当前的历史最高价格) - 1)
```

两者都应该保留，但不能混用名称。

#### 2.2.4 只有周期汇总，没有完整路径

目前一条信号只保存 D1/D3/D5/D10 的结果。以后改变手续费、成交规则、基准指数或止盈止损规则时，如果没有每日路径，就必须重新获取行情，难以审计和重算。

#### 2.2.5 信号事实与分析结果耦合

`SaveSelectionResults` 会按 `run_id` 删除后重写选股结果和跟踪数据。对于研究系统，信号事件应该尽量不可变；分析结果可以重算，但不应改写“当时发生了什么”。

## 3. 分析目标与非目标

### 3.1 目标

系统最终应该能回答：

- 某策略在 14:45 发出信号后，股票在 D1/D3/D5/D10 的开、高、低、收收益分别是多少？
- 相对信号价和相对模拟成交价的收益分别是多少？
- 考虑手续费和滑点后，策略是否仍有收益？
- 最大有利波动、最大不利波动和真正的最大回撤是多少？
- 是否先触发止盈，还是先触发止损？
- 相对沪深 300、中证 1000 或行业指数是否有超额收益？
- 高分信号是否比低分信号表现更好？
- 在上涨、震荡、下跌，以及高低波动环境中是否稳定？
- 多个信号组成真实资金组合后，收益、回撤和容量如何？

### 3.2 暂不包含

V1 不做以下事情：

- 不自动发送真实买卖委托；
- 不承诺用 `signal_price` 一定能够成交；
- 不在没有分钟级顺序数据时臆测同一天内止盈和止损谁先发生；
- 不用未来数据补写信号时点本不可能知道的字段；
- 不因为单个周期收益好就直接判断策略有效。

## 4. 核心概念和价格口径

### 4.1 必须区分的三个价格

| 字段 | 含义 | 用途 |
| --- | --- | --- |
| `signal_price` | 策略触发时刻看到的 `lastPrice` | 评价信号判断能力 |
| `entry_price` | 按执行规则模拟得到的成交价 | 评价可交易收益 |
| `signal_day_close` | 信号当天 15:00 收盘价 | D0 观察值，不等同于成交价 |

三个价格可能相同，但系统不能默认它们相同。

### 4.2 建议的收益口径

每个主要指标至少保留两套结果：

- **Signal return**：相对 `signal_price`，衡量策略触发点质量。
- **Trade return**：相对 `entry_price`，衡量模拟交易可实现性。

前端默认展示 Trade return，同时允许切换到 Signal return。报表必须在标题或字段名中写清楚基准，避免把两套收益混在一起聚合。

### 4.3 复权口径

建议采用以下原则：

- 原始信号快照保存当时的未复权真实价格。
- 跨日收益计算使用同一套复权因子校正除权除息影响。
- 保存 `adjustment_type` 和 `adjustment_version`，保证以后可以解释结果。
- 模拟成交金额、涨跌停和滑点判断仍使用当时可交易的真实价格。

## 5. 14:45 信号的时间定义

### 5.1 交易日编号

对于 14:45 产生的信号：

- `D0`：信号产生当天。
- `D1`：信号后的第一个交易日。
- `D3`：信号后的第三个交易日。
- `D5`：信号后的第五个交易日。
- `D10`：信号后的第十个交易日。

周末、法定节假日和停牌日不能简单按自然日计算。建议“该股票有有效交易 K 线的交易日”才推进持有日序号，同时额外记录市场交易日序号，便于识别停牌。

### 5.2 D0 应单独保留

14:45 到 15:00 仍有 15 分钟价格变化，建议单独计算：

- `d0_close_return`：信号价到当日收盘的变化；
- `d0_mfe`：14:45 之后最高价相对信号价的收益；
- `d0_mae`：14:45 之后最低价相对信号价的收益。

如果暂时没有 14:45 后的分钟行情，只计算 `d0_close_return`，并把 D0 MFE/MAE 标记为 `unavailable`，不能用整日日 K 的最高/最低代替，因为那可能发生在信号之前。

### 5.3 推荐的模拟成交规则

第一版建议同时运行两个纸面成交模型：

#### 模型 A：信号后成交

- 信号产生后等待固定延迟，例如 5 秒或下一个有效行情快照；
- 优先用卖一价；没有盘口时使用下一条成交价；
- 保存滑点、行情时间和数据源；
- 用于评价盘中策略的即时可交易性。

#### 模型 B：次日开盘成交

- 在 D1 开盘价基础上叠加配置的滑点；
- 如果一字涨停、停牌或无成交，则记录 `rejected` 或按明确规则顺延；
- 用于提供更保守、数据更稳定的基准。

模型 A 更贴近 14:45 的真实意图，模型 B 更容易回测和验证。两者并行可以直接看出隔夜跳空和执行延迟吞掉了多少收益。

## 6. 推荐数据模型

### 6.1 总体关系

```text
selection_results（一次不可变信号）
    1 -> N  selection_signal_executions（不同模拟成交模型）
    1 -> N  selection_signal_observations（D0/D1/D2...路径）
    1 -> N  selection_result_tracking（D1/D3/D5/D10汇总）
```

### 6.2 扩展 `selection_results`

保留现有字段，新增显式信号快照字段。建议字段如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `signal_at` | TEXT | 策略真正触发时间，RFC3339，带 `+08:00` |
| `signal_price` | REAL | 触发时 `lastPrice` |
| `quote_at` | TEXT | 该行情自身的时间，不等于写库时间 |
| `quote_source` | TEXT | 如 `tdx_realtime`、`minute1` |
| `price_type` | TEXT | 如 `last`、`ask1`、`minute_close` |
| `quote_age_ms` | INTEGER | 写入时行情延迟 |
| `signal_score` | REAL | 结构化保存策略评分 |
| `strategy_version` | TEXT | 策略代码或配置版本 |
| `market_context_json` | TEXT | 触发时可知的市场环境快照 |
| `snapshot_status` | TEXT | `valid` / `stale` / `missing` |

示例迁移 SQL：

```sql
ALTER TABLE selection_results ADD COLUMN signal_at TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN signal_price REAL NOT NULL DEFAULT 0;
ALTER TABLE selection_results ADD COLUMN quote_at TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN quote_source TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN price_type TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN quote_age_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE selection_results ADD COLUMN signal_score REAL NOT NULL DEFAULT 0;
ALTER TABLE selection_results ADD COLUMN strategy_version TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN market_context_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE selection_results ADD COLUMN snapshot_status TEXT NOT NULL DEFAULT 'missing';

CREATE INDEX IF NOT EXISTS idx_selection_results_signal_at
ON selection_results(signal_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_selection_results_event_unique
ON selection_results(run_id, formula_id, symbol, signal_at);
```

兼容期内可令 `latest = signal_price`，但新代码只读取语义明确的 `signal_price`。确认所有调用方迁移后，再考虑废弃 `latest`。

### 6.3 新增模拟成交表

```sql
CREATE TABLE IF NOT EXISTS selection_signal_executions (
    id TEXT PRIMARY KEY,
    selection_result_id TEXT NOT NULL,
    policy_name TEXT NOT NULL,
    policy_version TEXT NOT NULL DEFAULT '1',
    side TEXT NOT NULL DEFAULT 'buy',
    status TEXT NOT NULL DEFAULT 'pending',
    scheduled_at TEXT NOT NULL DEFAULT '',
    executed_at TEXT NOT NULL DEFAULT '',
    reference_price REAL NOT NULL DEFAULT 0,
    entry_price REAL NOT NULL DEFAULT 0,
    quantity INTEGER NOT NULL DEFAULT 0,
    gross_amount REAL NOT NULL DEFAULT 0,
    commission REAL NOT NULL DEFAULT 0,
    stamp_duty REAL NOT NULL DEFAULT 0,
    transfer_fee REAL NOT NULL DEFAULT 0,
    slippage_bps REAL NOT NULL DEFAULT 0,
    price_source TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(selection_result_id, policy_name, policy_version)
);

CREATE INDEX IF NOT EXISTS idx_signal_executions_status
ON selection_signal_executions(status, scheduled_at);
```

状态建议：`pending`、`filled`、`rejected`、`expired`、`unavailable`。即使无法成交也要留记录，不能只统计成功成交的样本，否则会产生幸存者偏差。

### 6.4 新增逐日观察表

```sql
CREATE TABLE IF NOT EXISTS selection_signal_observations (
    id TEXT PRIMARY KEY,
    selection_result_id TEXT NOT NULL,
    execution_id TEXT NOT NULL DEFAULT '',
    trade_date INTEGER NOT NULL,
    market_day_index INTEGER NOT NULL DEFAULT 0,
    symbol_day_index INTEGER NOT NULL DEFAULT 0,
    session_kind TEXT NOT NULL DEFAULT 'forward',
    open REAL NOT NULL DEFAULT 0,
    high REAL NOT NULL DEFAULT 0,
    low REAL NOT NULL DEFAULT 0,
    close REAL NOT NULL DEFAULT 0,
    volume REAL NOT NULL DEFAULT 0,
    amount REAL NOT NULL DEFAULT 0,
    signal_open_return REAL NOT NULL DEFAULT 0,
    signal_high_return REAL NOT NULL DEFAULT 0,
    signal_low_return REAL NOT NULL DEFAULT 0,
    signal_close_return REAL NOT NULL DEFAULT 0,
    trade_open_return REAL NOT NULL DEFAULT 0,
    trade_high_return REAL NOT NULL DEFAULT 0,
    trade_low_return REAL NOT NULL DEFAULT 0,
    trade_close_return REAL NOT NULL DEFAULT 0,
    benchmark_symbol TEXT NOT NULL DEFAULT '',
    benchmark_close_return REAL NOT NULL DEFAULT 0,
    excess_close_return REAL NOT NULL DEFAULT 0,
    adjustment_type TEXT NOT NULL DEFAULT '',
    data_status TEXT NOT NULL DEFAULT 'valid',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(selection_result_id, execution_id, trade_date, session_kind)
);

CREATE INDEX IF NOT EXISTS idx_signal_observations_signal_day
ON selection_signal_observations(selection_result_id, symbol_day_index);
```

说明：如果希望减少冗余，也可以只存原始 OHLC、基准价格和基准指数 OHLC，查询时计算收益。第一版保留收益列更方便 API 和排错，但重算任务必须能覆盖这些派生字段。

### 6.5 扩展周期汇总表

保留 `selection_result_tracking`，建议新增：

- `execution_id`：本结果对应哪个模拟成交模型；
- `return_basis`：`signal` 或 `trade`；
- `final_high`、`final_low`；
- `high_return`、`low_return`；
- `mfe`、`mae`；
- `max_path_drawdown`；
- `benchmark_close_return`、`excess_close_return`；
- `gross_return`、`net_return`、`total_cost`；
- `first_event`：`target`、`stop`、`none`、`ambiguous`；
- `calculation_version`：计算口径版本；
- `data_quality`：`complete`、`partial`、`suspect`。

唯一约束需要从：

```text
(selection_result_id, horizon_days)
```

升级为：

```text
(selection_result_id, execution_id, return_basis, horizon_days, calculation_version)
```

不要直接删除旧列。先增加新字段并双写，待 API 和前端完成迁移后再决定是否清理旧口径。

## 7. 指标定义与公式

设：

- `S` = `signal_price`
- `E` = 含买入成本的 `entry_price`
- `O_n/H_n/L_n/C_n` = 第 n 个未来有效交易日的开高低收
- `B_n` = 基准指数第 n 日收盘价
- `B_0` = 信号时刻或信号日对应的基准价格

### 7.1 Dn 开高低收收益

相对信号价：

```text
signal_open_return_n  = (O_n / S - 1) * 100
signal_high_return_n  = (H_n / S - 1) * 100
signal_low_return_n   = (L_n / S - 1) * 100
signal_close_return_n = (C_n / S - 1) * 100
```

相对模拟成交价：

```text
trade_open_return_n  = (O_n / E - 1) * 100
trade_high_return_n  = (H_n / E - 1) * 100
trade_low_return_n   = (L_n / E - 1) * 100
trade_close_return_n = (C_n / E - 1) * 100
```

D1/D3/D5/D10 的“高、低”有两种常见含义，必须分别命名：

- `day_high_return_d5`：只看 D5 当天最高价。
- `mfe_d5`：看 D1 到 D5 整个窗口内的最高价。

推荐报表同时显示 Dn 当日 OHLC 收益和窗口 MFE/MAE，避免歧义。

### 7.2 MFE 与 MAE

```text
MFE_n = max(H_1 ... H_n) / E - 1
MAE_n = min(L_1 ... L_n) / E - 1
```

- MFE 回答“这笔交易最多曾赚多少”。
- MAE 回答“这笔交易最多曾亏多少”。

### 7.3 真正的最大路径回撤

使用每日收盘路径时：

```text
running_peak_t = max(C_1 ... C_t)
drawdown_t = C_t / running_peak_t - 1
max_path_drawdown_n = min(drawdown_1 ... drawdown_n)
```

若有分钟级路径，可以计算更精细的盘中最大回撤，并明确命名为 `intraday_max_drawdown`。

### 7.4 超额收益

```text
stock_return_n = C_n / E - 1
benchmark_return_n = B_n / B_0 - 1
excess_return_n = stock_return_n - benchmark_return_n
```

第一版基准建议：

- 沪深 300：大盘股参考；
- 中证 1000：中小盘参考；
- 个股所属申万一级行业指数：行业参考。

可以按股票板块选择默认宽基，但规则必须版本化，不能回看结果后临时挑选最有利基准。

### 7.5 净收益

```text
gross_return = exit_price / entry_price - 1
net_pnl = exit_amount - buy_amount - commission - stamp_duty - transfer_fee
net_return = net_pnl / buy_amount
```

手续费、最低佣金、卖出印花税、过户费和滑点应放入版本化配置。历史统计必须显示使用的成本模型版本。

### 7.6 止盈止损顺序

日 K 同一天同时越过止盈线和止损线时，无法知道谁先发生。处理规则：

- 有分钟数据：按时间顺序判断。
- 只有日 K：标记 `first_event = ambiguous`。
- 保守回测可另算一列“假设止损先发生”，但不能把假设当事实。

## 8. 数据生命周期与任务流程

### 8.1 信号产生时

1. 策略计算完成并确认命中。
2. 立即记录 `signal_at`，不要等整个批次结束后统一赋时。
3. 获取并验证实时行情：价格大于 0、时间不晚于当前、延迟低于阈值。
4. 写入不可变信号快照，包括策略版本和分数。
5. 为每个启用的成交模型创建 `pending` 执行记录。
6. 对模型 A 获取下一条可执行报价并填充成交；模型 B 等待 D1 开盘。
7. 任何失败都写入状态和原因，不能静默丢弃。

### 8.2 收盘后日任务

1. 确认交易日数据同步完成。
2. 找出仍有 D1/D3/D5/D10 未完成的信号。
3. 补齐该股票和基准指数的当日行情。
4. 使用交易日历分配 `market_day_index` 和 `symbol_day_index`。
5. 幂等写入 observation。
6. 从 observation 重算所有受影响的 horizon 汇总。
7. 刷新策略级、分数桶级和市场环境级统计。
8. 运行数据质量检查并输出异常计数。

### 8.3 幂等设计

建议使用稳定业务键：

- 信号：`run_id + formula_id + symbol + signal_at`；
- 执行：`selection_result_id + policy_name + policy_version`；
- 观察：`selection_result_id + execution_id + trade_date + session_kind`；
- 汇总：`selection_result_id + execution_id + return_basis + horizon + calculation_version`。

任务重跑应执行 upsert，结果相同；不应先删除不可变信号。只有派生 observation 和 tracking 允许按计算版本重建。

## 9. 策略有效性分析框架

### 9.1 第一层：基础分布

每个策略、每个周期至少输出：

- 样本数、完成数、缺失数、无法成交数；
- 平均收益、中位数收益；
- 25%、75%、10%、90% 分位数；
- 正收益率、达到目标收益率、触发止损率；
- MFE、MAE、最大路径回撤的均值和分位数；
- 最佳和最差样本，但不能只展示极值。

平均值容易被少数暴涨股拉高，中位数和分位数必须同时看。

### 9.2 第二层：超额收益

分别统计：

- 相对宽基超额收益；
- 相对行业超额收益；
- 超额收益为正的比例；
- 去掉市场 Beta 后的表现。

若绝对收益为正但超额收益长期为负，策略可能只是追随牛市，并没有提供真正选股能力。

### 9.3 第三层：评分单调性

若信号有 `score`，按五分位或十分位分桶：

- 比较各桶 D1/D3/D5/D10 平均和中位收益；
- 计算 Spearman Rank IC；
- 观察高分桶是否稳定优于低分桶；
- 计算 IC 均值、标准差和 ICIR。

如果分数越高并没有更好收益，说明评分只能解释规则命中程度，还没有形成有效排序。

### 9.4 第四层：市场环境切片

按触发时可知的信息划分：

- 指数位于 20 日均线上/下；
- 市场上涨、震荡、下跌；
- 高波动/低波动；
- 成交额放大/缩小；
- 行业强势/弱势；
- 涨停情绪高/低。

注意市场环境必须在信号时点冻结，不能使用未来数据分类。

### 9.5 第五层：可执行性

重点统计：

- 信号价到模拟成交价的滑点分布；
- 信号价到次日开盘价的隔夜跳空；
- 涨停、停牌、流动性不足导致的无法成交率；
- 成交金额占当时市场成交额的比例；
- 加入真实费用后的净收益变化。

### 9.6 第六层：组合回测

单股信号有效不代表组合可用。组合层需定义：

- 初始资金；
- 单票最大仓位；
- 同日最多买入数量；
- 行业集中度上限；
- 同一股票重复信号的合并规则；
- 持有期和退出规则；
- 资金不足时的信号排序规则。

输出组合净值、年化收益、最大回撤、波动率、Sharpe、Sortino、Calmar、换手率、资金使用率和最大连续亏损。

### 9.7 第七层：稳健性验证

- 按时间做样本内/样本外切分；
- 使用滚动窗口或 walk-forward 验证；
- 对参数做小范围扰动，观察结果是否突然崩塌；
- 按年份、季度、牛熊阶段拆分；
- 多策略、多周期比较时控制数据挖掘偏差；
- 用 bootstrap 给平均收益和胜率估计置信区间。

策略结论至少需要足够样本并经过样本外验证，不能只凭最近几周的好结果上线。

## 10. API 与前端建议

### 10.1 API

建议逐步新增：

```text
GET /api/selection-results/{id}/signal
GET /api/selection-results/{id}/executions
GET /api/selection-results/{id}/observations
GET /api/selection-results/{id}/tracking?basis=trade&policy=next_quote
GET /api/strategy-analysis/summary
GET /api/strategy-analysis/distribution
GET /api/strategy-analysis/score-buckets
GET /api/strategy-analysis/regimes
```

公共查询参数建议统一支持：

- `strategy_id`
- `start_date`、`end_date`
- `horizon`
- `basis`
- `execution_policy`
- `benchmark`
- `market_regime`
- `score_bucket`

### 10.2 前端

在现有每日复盘/策略分析中增加四个视图：

1. **信号明细**：信号价、行情时间、成交价、滑点、D1/D3/D5/D10。
2. **收益分布**：箱线图或分位数图，不只显示平均值。
3. **评分分析**：分数桶收益、胜率、IC。
4. **环境分析**：不同市场状态和行业状态下的表现。

明细表建议固定显示：

```text
信号时间 | 股票 | 策略 | 分数 | 信号价 | 成交价 | 滑点
D1收盘 | D3收盘 | D5收盘 | D10收盘 | MFE | MAE | 超额收益 | 数据质量
```

收益列需要明确标注当前是“相对信号价”还是“相对成交价”。

## 11. 迁移与历史回填

### 11.1 迁移原则

- 先加字段和新表，不破坏旧 API。
- `store.go` 已有 `ensureColumn`，可沿用现有迁移模式。
- 新老字段双写一段时间。
- 用 `calculation_version` 隔离新旧口径。
- 每次迁移记录行数、成功数、缺失数和异常数。

### 11.2 历史信号回填规则

旧数据无法凭空恢复真正的 14:45 实时价格，应诚实标记来源：

1. `latest > 0`：可回填为 `signal_price`，但 `quote_source = legacy_latest`。
2. `signal_at`：可暂用 `created_at`，同时标记 `signal_time_quality = inferred`（可放入状态或扩展字段）。
3. 无法证明行情时间时，`snapshot_status = legacy` 或 `suspect`。
4. 不要把信号日收盘价回填成 14:45 实时价。
5. 历史成交模型优先使用次日开盘，盘中即时成交模型只对有分钟数据的日期回填。
6. 新旧口径报表默认分开，不能悄悄混合。

### 11.3 回填顺序

1. 扩展表结构。
2. 回填信号字段和策略分数。
3. 为旧信号生成 execution。
4. 按股票和日期批量加载行情，生成 observation。
5. 按新公式生成 tracking。
6. 对比旧版和新版结果差异。
7. 通过抽样核对后切换默认 API。

## 12. 测试计划

### 12.1 单元测试

至少覆盖：

- 14:45 信号的 D1 是下一交易日；
- 周五信号的 D1 是下一个交易日而不是周六；
- 节假日、停牌、缺失 K 线；
- 信号价、成交价、收盘价不同时的双口径收益；
- D1/D3/D5/D10 当日 OHLC 和窗口 MFE/MAE；
- MAE 与真正最大路径回撤不同的样例；
- 同日同时触发止盈止损时标记 `ambiguous`；
- 涨停无法买入、次日顺延或拒绝规则；
- 手续费和滑点计算；
- 除权除息日的复权一致性；
- 重跑任务不会产生重复记录。

### 12.2 集成测试

- 策略命中后能够同时写入信号和待执行记录；
- 收盘同步完成后 observation 与 tracking 自动更新；
- API 能按策略、周期、口径、成交模型查询；
- 历史回填可中断后继续；
- 旧版每日复盘在迁移期间仍能正常使用。

### 12.3 固定样本测试

准备 5 到 10 个手工可验证样本，固定信号时间、报价、未来 K 线和基准 K 线。把人工计算结果写进测试，防止以后修改算法时悄悄改变统计口径。

## 13. 数据质量与监控

每日应输出以下指标：

- 当日信号数；
- 有效信号快照率；
- 行情延迟 P50/P95/P99；
- 模拟成交成功率、拒绝率、缺失率；
- observation 完整率；
- 各 horizon 待完成和异常数量；
- 股票与基准交易日不一致数量；
- 价格为 0、OHLC 关系异常、单日异常涨跌数量；
- 重复业务键冲突数量；
- 新旧计算版本差异超阈值数量。

建议的数据质量规则：

```text
open/high/low/close > 0
high >= max(open, close)
low <= min(open, close)
quote_at <= signal_at + 允许延迟
entry_at >= signal_at（即时成交模型）
D3 的日期必须晚于或等于 D1
```

异常数据不要直接删除；保留记录并设置 `data_quality = suspect`，默认从策略统计中排除。

## 14. 分阶段实施计划

### 阶段 0：冻结口径（0.5 天）

任务：

- 确认策略 14:45 的实际调度时间。
- 验证 `StrategySelectionItem.Latest` 当前到底来自实时价、分钟价还是日 K 最新收盘。
- 确认行情对象是否能提供行情时间、卖一价和数据源。
- 决定首批成交模型参数和手续费参数。

验收标准：

- 形成一页字段口径表。
- 用一只股票完整说明从 14:45 信号到 D10 的日期和价格选择。

### 阶段 1：可靠记录信号（1 到 2 天）

涉及文件：

- `packages/workbench-core/store.go`
- `services/selection-worker/strategy_runner.go`
- `services/selection-worker/automation_runner.go`
- `services/selection-worker/system_strategy_batch.go`

任务：

- 给 `SelectionResult` 增加显式信号快照字段。
- 增加 SQLite 迁移和索引。
- 在每个信号形成时记录准确时间，不在批次保存阶段统一取时间。
- 将 `score` 从 `detail_json` 提升为结构化字段。
- 加入行情新鲜度校验和快照状态。

验收标准：

- 新信号都能看到 `signal_at`、`signal_price`、`quote_at`、`quote_source`。
- 相同业务键重跑不会生成重复信号。
- 缺行情的信号仍然被记录并有明确原因。

### 阶段 2：模拟成交（2 到 3 天）

涉及文件：

- `packages/workbench-core/store.go`
- 新建 `packages/workbench-core/signal_execution.go`
- `services/selection-worker/strategy_runner.go`
- `services/selection-worker/automation_runner.go`

任务：

- 建立 execution 表和存取方法。
- 实现 `next_quote` 与 `next_day_open` 两种策略。
- 实现手续费、滑点、涨停、停牌和失败状态。
- 保证所有策略版本和执行版本可追踪。

验收标准：

- 每条新信号都有两个执行模型的记录。
- 成交和未成交样本均能查询。
- 人工样本的成交价与费用计算一致。

### 阶段 3：路径与新指标（2 到 4 天）

涉及文件：

- 新建 `packages/workbench-core/selection_observation.go`
- `packages/workbench-core/selection_tracking.go`
- `packages/workbench-core/store.go`
- `services/selection-worker/automation_runner.go`
- `services/selection-worker/selection_tracking_api.go`

任务：

- 建立 observation 表。
- 明确 D0 和 D1 的边界。
- 计算 Dn 当日 OHLC 收益、MFE、MAE、真正最大路径回撤。
- 同时产出 signal 和 trade 两种口径。
- 增加计算版本和数据质量状态。

验收标准：

- D1/D3/D5/D10 四价收益可查询。
- MAE 与最大路径回撤分别展示。
- 14:45 之后的 D0 不使用信号前最高/最低价。
- 重跑结果幂等。

### 阶段 4：基准与统计分析（3 到 5 天）

涉及文件：

- `packages/workbench-core/daily_review_store.go`
- `packages/workbench-core/historical_backtest_store.go`
- `services/selection-worker/historical_backtest.go`
- 新建策略分析 API 文件。

任务：

- 接入宽基和行业基准。
- 增加超额收益、分位数、评分桶和 IC。
- 增加市场环境切片。
- 给统计接口增加最小样本数和置信区间。

验收标准：

- 能判断策略收益是否高于市场和行业。
- 能看到评分是否具有单调性。
- 所有分析都显示样本数和缺失率。

### 阶段 5：前端分析台（3 到 5 天）

任务：

- 增加信号明细、收益分布、评分分析、环境分析四个视图。
- 增加信号价/成交价口径切换。
- 展示执行失败原因和数据质量状态。
- 提供 CSV 导出，导出内容带计算版本和筛选条件。

验收标准：

- 用户能从策略汇总钻取到单条信号路径。
- 页面不会只展示平均收益和胜率。
- 每个数字都能追溯到价格基准、成交模型和计算版本。

### 阶段 6：组合回测与稳健性（后续）

任务：

- 加入资金、仓位、并发持仓和行业约束。
- 计算组合净值与风险指标。
- 增加 walk-forward、参数敏感性和 bootstrap。
- 设定纸面运行观察期，例如至少 2 到 3 个月或达到最低有效样本数。

验收标准：

- 样本外表现没有明显失效。
- 成本后收益仍为正。
- 回撤、换手和容量处于可接受范围。
- 通过书面评审后才讨论真实委托接口。

## 15. 推荐的近期工作顺序

下一步最值得做的不是继续堆更多图表，而是先把原始事实记录准确。推荐顺序：

1. 核实当前 `Latest` 的数据来源和时间戳。
2. 改造信号表，准确记录 14:45 的 `signal_price` 和 `quote_at`。
3. 建立模拟成交表，先跑 `next_quote` 与 `next_day_open` 两种模型。
4. 建立逐日 observation，补齐 D1/D3/D5/D10 的 OHLC 双口径收益。
5. 修正 MAE 与最大回撤的命名和算法。
6. 再做超额收益、评分分桶和市场环境分析。
7. 样本积累后做组合回测和稳健性检验。

这条路线会让系统从“观察信号后股票涨没涨”，升级为“验证信号是否有预测力、能否成交、成本后是否赚钱、风险是否可控、在什么环境下有效”。这才是后续决定是否值得继续优化甚至进入实盘验证的可靠依据。

## 16. 第一版完成定义

满足以下条件时，可以认为策略分析 V1 完成：

- 所有新信号都有可审计的触发时间、信号价、行情时间和策略版本；
- 至少有一个即时模型和一个次日开盘模型；
- D0 与 D1 定义明确，交易日计算正确；
- 可查询 D1/D3/D5/D10 当日 OHLC 收益；
- 可查询窗口 MFE、MAE 和真正最大路径回撤；
- 信号收益与模拟交易收益分开；
- 有宽基超额收益；
- 失败成交和缺失数据被纳入统计；
- 全流程可重跑、幂等、带计算版本；
- 单元测试和固定人工样本通过；
- 纸面运行一段时间后，能输出一份包含样本数、收益分布、风险、超额收益和适用环境的策略评估报告。
