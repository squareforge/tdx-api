# 策略到交易系统实施方案

> 版本：V1.0  
> 日期：2026-09-24  
> 项目：`tdx-api`  
> 目标：把当前的定时选股、信号跟踪和个人交易记录，逐步连接成一套低频、规则化、可审计的量化交易系统。

## 1. 先给结论

你的想法是成立的，而且它已经属于量化交易的范畴。

量化交易不一定要高频，也不一定要每秒扫描行情。只要下面这些环节由明确规则驱动，而不是临时凭感觉决定，就可以称为低频量化或事件驱动量化：

```text
定时运行策略
    -> 产生买入信号
    -> 风控检查与资金分配
    -> 生成并执行买入订单
    -> 持仓实时或定时监控
    -> 触发卖出规则
    -> 执行卖出订单
    -> 记录成交、持仓和收益
    -> 将收益归因到具体策略和信号
```

你的系统和典型实盘量化系统的区别主要在于：

- 扫描频率可能是每天一次或每天几次，而不是持续高频扫描；
- 策略信号可能在 14:45 的固定时点产生，而不是每个行情事件都重新计算；
- 交易执行可以先通过定时任务和行情接口完成，再逐步接入券商；
- 重点不是极低延迟，而是规则正确、风险可控、成交可追溯。

但是，“有信号就直接买入”不能理解为没有任何保护的无条件下单。正确链路应该是：

```text
信号
    -> 交易计划
    -> 账户和风险检查
    -> 订单
    -> 成交
    -> 持仓
```

卖出也必须在买入之前就定义好。买入时同时写入止损、止盈、移动止盈和最大持有期，后续由持仓监控任务自动判断。

## 2. 当前项目是什么状态

### 2.1 已有的策略侧能力

当前项目已经具备策略研究和信号跟踪基础：

- `selection_results` 保存策略命中的股票；
- `selection_result_tracking` 保存 D1/D3/D5/D10 的跟踪结果；
- `EvaluateSelectionTracking` 已计算未来开盘、收盘、最大上涨、最大下跌和目标触达；
- 历史回测中已经有组合模拟撮合；
- 组合模拟已经考虑 T+1、涨跌停、100 股整手、资金不足、最大持仓和费用；
- 策略分析方案已经准备把 `signal_price`、`entry_price`、MFE、MAE 和超额收益分开记录。

相关位置：

- `packages/workbench-core/store.go`
- `packages/workbench-core/selection_tracking.go`
- `services/selection-worker/portfolio_simulator.go`
- `services/selection-worker/automation_runner.go`
- `services/selection-worker/strategy_runner.go`
- `docs/STRATEGY_SIGNAL_ANALYSIS_PLAN.md`

### 2.2 当前交易系统的定位

当前交易模块的核心结构是：

- `TradingAccount`：账户资金和风险参数；
- `TradingTrade`：买入、卖出、持仓和交易卡字段；
- `TradingSystemState`：把账户、费用、纪律和交易数组整体保存；
- `/api/trading-system`：读取和整体更新交易系统状态；
- `TradingSystemWorkspace.tsx`：手工添加、编辑、查看和导入导出交易记录。

当前实现更接近：

```text
个人交易记录 + 交易计划 + 风控面板 + 手工复盘工具
```

它还不是完整的订单系统，因为目前没有严格拆分：

```text
信号 Signal
交易计划 Trade Plan
委托 Order
成交 Fill
持仓 Position
账户 Account
收益归因 Attribution
```

### 2.3 本地交易上传是否鸡肋

如果系统的目标只是记录手工交易，那么上传本地交易是有价值的。

如果系统的目标是“策略产生信号并执行交易”，那么上传功能不应该再是主流程。它应该降级为以下辅助能力：

1. 第一次使用时导入历史成交；
2. 券商没有 API 时导入 CSV、Excel 或交割单；
3. API 漏单或人工操作后进行对账和补录；
4. 从旧系统迁移到新订单系统。

新的主流程应该是：

```text
策略自动产生信号
    -> 系统自动生成交易计划
    -> 系统自动做风险检查
    -> 模拟盘或实盘执行
    -> 成交自动回写
    -> 交易面板自动更新
```

因此，不建议删除导入功能，但建议把它改名或放入“数据导入与对账”入口，而不是让用户每天靠上传文件驱动交易。

## 3. 目标系统的整体架构

推荐最终形成下面六个模块：

```text
策略引擎
  产生 signal
      |
      v
信号中心
  保存 signal_at、signal_price、score、strategy_version
      |
      v
交易计划引擎
  计算 entry policy、仓位、止损、止盈、最大持有期
      |
      v
风险引擎
  检查资金、仓位、重复订单、市场开关、T+1、涨跌停
      |
      v
订单执行引擎
  模拟执行 / 券商执行 / 手工确认
      |
      v
账户与持仓账本
  orders、fills、positions、cash、pnl、attribution
      |
      v
收益面板和复盘
```

各模块的职责要保持清楚：

| 模块 | 负责什么 | 不负责什么 |
| --- | --- | --- |
| 策略引擎 | 判断什么时候出现买入或退出信号 | 不直接管理现金和券商订单 |
| 信号中心 | 记录策略当时看到的事实 | 不修改历史信号 |
| 交易计划 | 把信号转换成具体买卖方案 | 不代表订单已经成交 |
| 风险引擎 | 判断是否允许下单 | 不决定策略是否有效 |
| 订单执行 | 提交、撤销、查询订单 | 不重新解释策略信号 |
| 账户账本 | 根据成交计算持仓和资金 | 不用行情推测真实成交 |
| 收益面板 | 展示当前状态和历史绩效 | 不直接修改成交事实 |

## 4. 一次完整交易的生命周期

### 4.1 信号产生

以交易日 14:45 为例：

1. 定时任务启动策略。
2. 策略使用当时已经可获得的行情和历史数据计算。
3. 股票通过过滤条件并达到评分门槛。
4. 系统记录 `signal_at` 和 `signal_price`。
5. 系统保存行情时间、数据源、行情延迟和策略版本。
6. 系统为该信号创建 `pending` 交易计划。

信号必须先落库再生成订单。这样即使后续行情接口或券商接口失败，也能知道信号确实产生过。

### 4.2 生成交易计划

交易计划至少应包含：

```text
signal_id
strategy_id
symbol
signal_at
signal_price
entry_policy
reference_price
planned_entry_price
planned_shares
planned_amount
invalid_price
stop_loss_rate
take_profit_rate
trailing_drawdown_rate
max_hold_days
market_gate
reason
status
```

交易计划不是“已经买入”，而是“系统准备按什么规则买入”。

建议的初始状态：

```text
created -> risk_checked -> approved -> submitted -> partially_filled -> filled
                                      |             |
                                      v             v
                                   rejected       canceled
```

### 4.3 风控检查

策略信号进入交易执行前，至少检查：

- 行情是否仍然有效；
- 是否已经持有该股票；
- 是否存在同一信号的未完成订单；
- 当日是否已经买过该股票；
- 可用资金是否充足；
- 单票仓位是否超过上限；
- 总持仓数是否达到上限；
- 行业集中度是否超过上限；
- 计算后的股数是否至少为 100 股整手；
- 股票是否停牌或涨停；
- 大盘开关是否允许开仓；
- 当日是否触发账户级熔断或连续亏损限制；
- 系统是否处于“允许真实下单”状态。

风险检查失败也要保存记录，并标记具体原因。例如：

```text
rejected: 已有持仓
rejected: 涨停无法买入
rejected: 现金不足一手
rejected: 超过单票仓位上限
rejected: 市场开关关闭
rejected: 行情超过允许延迟
```

不能只保存成功下单的股票，否则后续分析会产生选择偏差。

### 4.4 买入执行

建议先支持三种执行模式：

| 模式 | 说明 | 适用阶段 |
| --- | --- | --- |
| `paper` | 使用实时行情模拟成交，不产生真实委托 | 第一阶段默认 |
| `manual_confirm` | 系统生成订单，用户点击确认后执行 | 接入实盘初期 |
| `broker_live` | 经过全部风控后自动提交券商订单 | 稳定运行后 |

对 14:45 信号，建议初始使用：

```text
14:45 产生信号
14:45+ 等待下一条有效行情
使用卖一价或下一条成交价模拟买入
记录报价时间、参考价、成交价和滑点
```

如果暂时拿不到可靠的盘口数据，可以使用：

```text
次日开盘价 + 配置滑点
```

但必须明确这是 `next_day_open` 模型，不要把它称为 14:45 即时成交。

### 4.5 成交回写

订单提交后不能直接认为买入成功。必须根据订单状态回写：

- 委托编号；
- 券商返回状态；
- 已成交数量；
- 成交均价；
- 费用；
- 最后一条同步时间；
- 拒单或撤单原因。

一笔订单可以对应多条成交，所以 `Order` 和 `Fill` 必须拆开。

### 4.6 持仓建立

只有成交回报才能增加持仓。持仓记录至少要包含：

```text
symbol
available_quantity
frozen_quantity
total_quantity
average_cost
market_price
market_value
unrealized_pnl
realized_pnl
buy_date
sellable_date
signal_id
strategy_id
```

A 股 T+1 下：

- 当日买入数量增加到 `total_quantity`；
- 当日买入数量不能加入 `available_quantity`；
- 下一交易日才能卖出；
- 交易系统必须区分总持仓和可卖持仓。

## 5. 卖出规则应该怎么设计

卖出不是一个单独的“卖出按钮”，而是由持仓监控根据一组提前定义好的退出规则产生卖出计划。

### 5.1 四类退出规则

推荐把退出拆成四大类。

#### A. 风险退出

保护本金，优先级最高。

例子：

```text
收盘价 <= 技术无效点
收盘价 <= 成本价 * (1 - 止损比例)
```

按照当前 `TRADING_SYSTEM_V1_1.md` 的纪律，第一版建议：

```text
收盘确认触发
下一交易日开盘执行
```

这与现有文档的“收盘跌破则次日开盘无条件执行”一致，也与当前组合模拟器的处理方式接近。

如果未来需要盘中硬止损，就必须增加分钟级或实时行情监控，不能只靠 14:45 的策略任务。

#### B. 趋势或策略失效退出

当原来的买入逻辑不再成立时退出，例如：

- 收盘跌破 MA20；
- 趋势结构破坏；
- 策略评分低于退出阈值；
- 大盘开关关闭且策略规定需要清仓；
- 独立的卖出策略产生信号；
- 行业或个股状态从强势变为弱势。

“今天没有再次入选买入列表”不能直接等于卖出。买入条件和卖出条件要分别定义，否则策略会在信号消失时过度交易。

#### C. 盈利退出

用于锁定收益或保护浮盈。

可选规则：

```text
固定止盈：收益达到 +10% 后卖出
移动止盈：最高收盘价达到 +8% 后，回撤 4% 卖出
分批止盈：达到第一目标卖出 50%，剩余仓位使用移动止盈
```

第一版建议先用移动止盈，不要同时叠加太多复杂条件：

```text
最高收盘价 >= 成本价 * 1.08
且当前收盘价 <= 最高收盘价 * 0.96
=> 下一交易日开盘卖出
```

#### D. 时间退出

防止资金被长期占用。

例如：

```text
持有 10 个交易日仍未达到目标
=> 下一交易日开盘卖出
```

时间退出要使用交易日数量，不能使用自然日数量。停牌日是否计入持有天数必须提前确定，推荐按有效交易日计数。

### 5.2 推荐的第一版卖出优先级

建议固定为：

```text
账户级风控
    > 强制止损
    > 技术无效
    > 移动止盈
    > 固定止盈
    > 时间退出
```

如果同一天多个规则同时满足，卖出原因应保存为最高优先级规则，同时把所有命中的规则保存到 `triggered_rules_json`。

### 5.3 14:45 定时系统的卖出时机

如果系统目前主要在 14:45 执行策略，推荐增加收盘后的持仓检查：

```text
09:25 处理上一交易日已经生成的待卖订单
09:30 以开盘价或可执行盘口执行卖出
14:45 运行买入策略并生成候选计划
14:46 通过风险检查后执行或模拟买入
15:05 同步完整日 K 和成交数据
15:10 检查所有持仓的卖出条件
15:15 生成下一交易日待卖订单
```

这样，卖出不是等待下一次买入信号，而是每天都对现有持仓进行检查。

### 5.4 如果要做盘中卖出

盘中卖出需要单独增加 `position_monitor` 任务：

```text
09:35 到 14:55
每 1 分钟或每 5 分钟获取持仓行情
检查硬止损、跌停、流动性和账户级风险
触发后生成卖出订单
```

注意：

- A 股当日买入仍然不能当日卖出；
- 盘中止损必须有明确的价格和行情时间；
- 只用日 K 无法判断盘中止盈和止损谁先发生；
- 日 K 同时触发两个条件时，应标记为 `ambiguous`，不能伪造顺序。

### 5.5 无法卖出的情况

卖出信号产生不等于卖出成功。以下情况要保留为未完成状态：

- 跌停无法卖出；
- 停牌；
- 委托被拒；
- 可卖数量不足；
- 网络或券商接口暂时失败；
- 当日订单已撤销但没有重新提交。

建议状态流转：

```text
exit_triggered
    -> pending_sell
    -> submitted
    -> partially_filled
    -> filled

pending_sell -> blocked / rejected / retrying
```

如果卖出失败，不能把持仓标记为已清仓，也不能把这笔交易提前计入已实现收益。

## 6. 推荐的交易状态机

### 6.1 信号状态

```text
created
  -> plan_created
  -> ignored
  -> expired
```

### 6.2 交易计划状态

```text
created
  -> risk_checked
  -> approved
  -> rejected
  -> expired
```

### 6.3 订单状态

```text
pending
  -> submitted
  -> partially_filled
  -> filled
  -> canceled
  -> rejected
  -> unknown
```

`unknown` 很重要：提交请求超时并不代表订单没有成功。遇到网络超时，必须先查询券商订单，再决定是否重试，否则可能重复下单。

### 6.4 持仓状态

```text
opening
  -> holding
  -> exit_triggered
  -> selling
  -> closed
```

持仓状态只能根据成交更新，不能根据策略信号直接跳到 `closed`。

## 7. 推荐的数据表设计

当前 `TradingSystemState` 整体 JSON 适合保存交易卡，但不适合作为自动交易的事实账本。建议保留旧结构用于兼容，同时逐步增加关系表。

### 7.1 交易账户表

```sql
CREATE TABLE IF NOT EXISTS trading_accounts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'paper',
    broker_name TEXT NOT NULL DEFAULT '',
    broker_account_ref TEXT NOT NULL DEFAULT '',
    initial_cash REAL NOT NULL DEFAULT 0,
    available_cash REAL NOT NULL DEFAULT 0,
    total_assets REAL NOT NULL DEFAULT 0,
    max_trade_risk REAL NOT NULL DEFAULT 0,
    max_position_weight REAL NOT NULL DEFAULT 0,
    max_positions INTEGER NOT NULL DEFAULT 0,
    live_trading_enabled INTEGER NOT NULL DEFAULT 0,
    kill_switch INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

`mode` 建议支持：

```text
paper
manual_confirm
broker_readonly
broker_live
```

`live_trading_enabled` 和 `kill_switch` 要分开保存。即使系统处于实盘模式，也可以通过 kill switch 立即阻止新订单。

### 7.2 策略信号表

沿用现有 `selection_results`，补齐：

```sql
ALTER TABLE selection_results ADD COLUMN signal_at TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN signal_price REAL NOT NULL DEFAULT 0;
ALTER TABLE selection_results ADD COLUMN signal_score REAL NOT NULL DEFAULT 0;
ALTER TABLE selection_results ADD COLUMN strategy_version TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN quote_at TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN quote_source TEXT NOT NULL DEFAULT '';
ALTER TABLE selection_results ADD COLUMN snapshot_status TEXT NOT NULL DEFAULT 'missing';
```

信号表是“策略当时发生了什么”的事实，不应因为后续买入失败而删除或修改。

### 7.3 交易计划表

```sql
CREATE TABLE IF NOT EXISTS trading_plans (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    signal_id TEXT NOT NULL,
    strategy_id TEXT NOT NULL DEFAULT '',
    symbol TEXT NOT NULL,
    side TEXT NOT NULL DEFAULT 'buy',
    status TEXT NOT NULL DEFAULT 'created',
    entry_policy TEXT NOT NULL DEFAULT '',
    signal_price REAL NOT NULL DEFAULT 0,
    reference_price REAL NOT NULL DEFAULT 0,
    planned_entry_price REAL NOT NULL DEFAULT 0,
    planned_shares INTEGER NOT NULL DEFAULT 0,
    planned_amount REAL NOT NULL DEFAULT 0,
    invalid_price REAL NOT NULL DEFAULT 0,
    stop_loss_rate REAL NOT NULL DEFAULT 0,
    take_profit_rate REAL NOT NULL DEFAULT 0,
    trailing_drawdown_rate REAL NOT NULL DEFAULT 0,
    max_hold_days INTEGER NOT NULL DEFAULT 0,
    risk_amount REAL NOT NULL DEFAULT 0,
    risk_ratio REAL NOT NULL DEFAULT 0,
    reason TEXT NOT NULL DEFAULT '',
    rejection_reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(account_id, signal_id, side)
);
```

交易计划保存“原本打算怎么交易”，即使最后没有成交，也不能删除。

### 7.4 订单表

```sql
CREATE TABLE IF NOT EXISTS trading_orders (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    plan_id TEXT NOT NULL DEFAULT '',
    position_id TEXT NOT NULL DEFAULT '',
    client_order_id TEXT NOT NULL,
    broker_order_id TEXT NOT NULL DEFAULT '',
    symbol TEXT NOT NULL,
    side TEXT NOT NULL,
    order_type TEXT NOT NULL DEFAULT 'limit',
    status TEXT NOT NULL DEFAULT 'pending',
    limit_price REAL NOT NULL DEFAULT 0,
    requested_quantity INTEGER NOT NULL DEFAULT 0,
    filled_quantity INTEGER NOT NULL DEFAULT 0,
    average_fill_price REAL NOT NULL DEFAULT 0,
    submitted_at TEXT NOT NULL DEFAULT '',
    canceled_at TEXT NOT NULL DEFAULT '',
    rejected_at TEXT NOT NULL DEFAULT '',
    failure_reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(account_id, idempotency_key)
);
```

`client_order_id` 和 `idempotency_key` 用于防止网络重试造成重复订单。

### 7.5 成交表

```sql
CREATE TABLE IF NOT EXISTS trading_fills (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL,
    broker_fill_id TEXT NOT NULL DEFAULT '',
    symbol TEXT NOT NULL,
    side TEXT NOT NULL,
    filled_at TEXT NOT NULL,
    quantity INTEGER NOT NULL,
    price REAL NOT NULL,
    gross_amount REAL NOT NULL DEFAULT 0,
    commission REAL NOT NULL DEFAULT 0,
    stamp_tax REAL NOT NULL DEFAULT 0,
    transfer_fee REAL NOT NULL DEFAULT 0,
    net_amount REAL NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'paper',
    created_at TEXT NOT NULL,
    UNIQUE(order_id, broker_fill_id)
);
```

真实成交和模拟成交都写入同样的结构，只通过 `source` 区分：

```text
paper
broker
import
manual
```

### 7.6 持仓表

```sql
CREATE TABLE IF NOT EXISTS trading_positions (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    strategy_id TEXT NOT NULL DEFAULT '',
    signal_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'holding',
    total_quantity INTEGER NOT NULL DEFAULT 0,
    available_quantity INTEGER NOT NULL DEFAULT 0,
    frozen_quantity INTEGER NOT NULL DEFAULT 0,
    average_cost REAL NOT NULL DEFAULT 0,
    market_price REAL NOT NULL DEFAULT 0,
    market_value REAL NOT NULL DEFAULT 0,
    realized_pnl REAL NOT NULL DEFAULT 0,
    unrealized_pnl REAL NOT NULL DEFAULT 0,
    peak_price REAL NOT NULL DEFAULT 0,
    hold_days INTEGER NOT NULL DEFAULT 0,
    sellable_at TEXT NOT NULL DEFAULT '',
    exit_trigger TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(account_id, symbol, signal_id)
);
```

### 7.7 退出事件表

```sql
CREATE TABLE IF NOT EXISTS trading_exit_events (
    id TEXT PRIMARY KEY,
    position_id TEXT NOT NULL,
    account_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    rule_name TEXT NOT NULL,
    triggered_at TEXT NOT NULL,
    trigger_price REAL NOT NULL DEFAULT 0,
    reference_price REAL NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending_order',
    order_id TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
```

这个表能回答：为什么卖出、什么时候触发、触发时价格是多少、是否真正产生订单、订单最后有没有成交。

## 8. 买入与卖出的任务调度

### 8.1 第一版推荐调度

```text
09:20  同步账户、现金、持仓和昨日未完成订单
09:25  检查待执行卖单，确认交易日和可卖数量
09:30  执行次日开盘卖出订单
09:35  查询订单状态，处理部分成交和拒单

14:40  检查策略任务、行情连接和账户状态
14:45  运行策略并生成信号快照
14:46  生成交易计划，计算股数和风险
14:47  风控检查
14:48  纸面成交、人工确认或提交实盘买单
14:50  查询买单状态并记录成交

15:05  同步完整日 K、账户、订单和成交
15:10  计算所有持仓的退出规则
15:15  生成下一交易日待卖计划
15:20  刷新收益面板和策略归因
```

时间只是初始建议，实际时间应根据行情接口和券商交易接口的延迟测试调整。

### 8.2 盘中监控任务

如果第一版只做收盘确认和次日开盘卖出，盘中可以不持续监控。

如果需要盘中硬止损，新增：

```text
09:35-14:55 每 1 分钟或每 5 分钟执行
```

盘中监控只负责已经持有的仓位和账户级风险，不重新运行完整选股策略。这样可以把“找新股票”和“保护已有仓位”分开，避免任务相互影响。

### 8.3 任务幂等

每个周期任务都必须能够重复执行而不重复买入或卖出。

推荐业务键：

```text
signal:       run_id + formula_id + symbol + signal_at
plan:         account_id + signal_id + side
order:        account_id + idempotency_key
fill:         order_id + broker_fill_id
exit_event:   position_id + rule_name + trigger_date
```

任务重跑时：

1. 先查询本地订单状态；
2. 再查询券商订单状态；
3. 只有确认不存在有效订单时才允许重试；
4. 网络超时状态标记为 `unknown`，不能直接当成失败。

## 9. 账户和仓位计算

### 9.1 单笔风险仓位

如果采用技术无效点止损，建议按风险金额计算股数：

```text
单笔最大风险金额 = 总资产 * 最大风险比例
每股风险 = 计划买入价 - 技术无效点
计划股数 = 单笔最大风险金额 / 每股风险
实际股数 = floor(计划股数 / 100) * 100
```

然后再应用：

- 单票金额上限；
- 总持仓数量上限；
- 可用现金限制；
- 行业集中度限制。

最终股数取所有约束中的最小值。

### 9.2 示例

假设：

```text
总资产：100,000 元
单笔风险：1%
计划买入价：20 元
技术无效点：19 元
```

计算：

```text
最大风险金额 = 100,000 * 1% = 1,000 元
每股风险 = 20 - 19 = 1 元
理论股数 = 1,000 / 1 = 1,000 股
```

如果单票最大仓位是 20%，则金额上限为 20,000 元，也正好限制在 1,000 股以内。若计划价变为 25 元，则金额上限可能先成为约束，最终股数不能只按止损距离计算。

### 9.3 账户级保护

建议加入以下开关：

- 单日最大亏损；
- 连续亏损次数上限；
- 单日最大买入金额；
- 最大持仓数；
- 最大行业集中度；
- 最大总风险；
- 市场环境关闭新开仓；
- 全局 kill switch。

触发账户级保护后：

- 禁止新买入；
- 已有持仓仍然按照止损规则处理；
- 状态写入审计日志；
- 面板显示阻断原因。

## 10. 研究回测、模拟盘和实盘必须分开

这三种模式共享规则，但不能共享状态。

### 10.1 历史回测

使用历史 K 线推演过去结果，允许批量重算。它回答：

```text
如果过去按这套规则交易，理论上会怎样？
```

当前 `portfolio_simulator.go` 属于这一层，可以继续增强，但不要让它直接修改真实账户。

### 10.2 模拟盘

使用实时信号和实时行情，但不发送券商订单。它回答：

```text
今天真实运行这套系统，信号和执行是否正常？
```

模拟盘必须记录实时延迟、盘口可得性、模拟滑点和无法成交原因。

### 10.3 实盘

产生真实委托和真实成交。它回答：

```text
在真实费用、真实成交和真实限制下，账户最终赚亏多少？
```

三种模式的收益不能混在一张表里。面板必须明确显示：

```text
mode = backtest / paper / live
```

## 11. 策略收益面板应该展示什么

交易系统不是只显示“当前赚了多少钱”。至少分为四个区域。

### 11.1 账户总览

- 总资产；
- 可用现金；
- 持仓市值；
- 当日收益；
- 已实现收益；
- 未实现收益；
- 总收益率；
- 当前回撤；
- 当前模式：模拟盘或实盘；
- kill switch 状态。

### 11.2 当前持仓

每个持仓显示：

```text
股票 | 策略 | 信号时间 | 成本 | 当前价 | 数量 | 可卖数量
市值 | 浮盈亏 | 最高价 | 持有天数 | 止损点 | 当前退出状态
```

### 11.3 待执行任务

分别显示：

- 待买入计划；
- 待卖出计划；
- 已提交未成交订单；
- 部分成交订单；
- 被风控拒绝的计划；
- 需要人工处理的 `unknown` 订单。

### 11.4 策略归因

按策略统计：

- 信号数；
- 成交数；
- 拒绝数；
- 持仓数；
- 已平仓数；
- 胜率；
- 平均收益；
- 盈亏比；
- 平均持有天数；
- 最大回撤；
- 手续费和滑点；
- 对总收益贡献；
- 信号收益与真实交易收益的差额。

必须保存 `signal_id` 和 `strategy_id` 到计划、订单、成交、持仓和最终交易结果中。否则以后只能知道赚了多少钱，却不知道哪套策略真正赚钱。

## 12. 与现有代码的对应改造

### 12.1 `packages/workbench-core/store.go`

第一阶段继续保留：

- `TradingSystemState`；
- `GetTradingSystemState`；
- `UpsertTradingSystemState`；
- 旧交易卡字段。

新增：

- `TradingAccountRecord`；
- `TradingPlan`；
- `TradingOrder`；
- `TradingFill`；
- `TradingPosition`；
- `TradingExitEvent`；
- 建表和迁移；
- 事务化的成交回写方法。

建议不要一开始直接删掉 `TradingSystemState`。先通过新表承接自动交易数据，旧 JSON 继续用于兼容手工交易卡和历史导入。

### 12.2 `services/selection-worker/strategy_runner.go`

需要做到：

- 生成信号时明确传入 `signal_at`；
- 保存 `signal_price`，不再只依赖 `latest`；
- 保存策略评分和策略版本；
- 生成信号后调用交易计划服务，而不是直接调用券商接口；
- 任何实际买入都必须通过账户模式和风控检查。

### 12.3 `services/selection-worker/automation_runner.go`

建议新增任务类型：

```text
strategy_signal       运行策略并生成信号
trade_plan            生成交易计划并做风控
order_submit          提交模拟或实盘订单
order_reconcile       查询订单和成交状态
position_monitor      检查持仓退出条件
account_reconcile     对账账户、现金和持仓
```

不要把所有逻辑继续堆进一个自动化任务。每个任务应有独立状态、日志和重试策略。

### 12.4 `services/selection-worker/portfolio_simulator.go`

当前组合撮合器已经有可复用规则：

- 次日开盘执行；
- T+1；
- 涨停不能买入；
- 跌停不能卖出；
- 100 股整手；
- 最大持仓数；
- 资金不足降级；
- 买卖费用；
- 止损、回撤、止盈和最大持有期。

建议提炼这些规则为共享的 `trading-rules` 包，但不要让实时执行直接复用依赖历史回测循环的代码。共享的应该是规则函数和约束，不是回测任务的生命周期。

### 12.5 `apps/web/frontend/src/components/TradingSystemWorkspace.tsx`

现有页面可以逐步从“交易记录编辑器”改成“交易控制台”：

1. 顶部显示账户模式和 kill switch；
2. 增加策略信号和待执行计划区域；
3. 增加订单、成交和持仓状态；
4. 增加卖出触发原因；
5. 增加策略收益归因；
6. 把导入导出放入设置或对账区域；
7. 实盘模式下增加人工确认开关。

## 13. 分阶段落地计划

### 阶段 0：明确交易口径

目标：先把“什么时候买、什么时候卖”写死。

确定以下参数：

- 信号产生时间；
- 即时成交还是次日开盘成交；
- 单笔风险比例；
- 单票最大仓位；
- 最大持仓数；
- 止损规则；
- 移动止盈规则；
- 最大持有交易日；
- 手续费和滑点；
- 模拟盘和实盘的切换条件。

验收：用一笔具体交易写出从信号到买入、持仓、卖出的完整时间线。

### 阶段 1：策略自动生成交易计划

目标：策略信号自动进入交易系统，但暂不自动真实下单。

实现：

- 信号产生后写入 `selection_results`；
- 自动创建 `trading_plans`；
- 自动计算买入价、止损价、股数、风险金额；
- 通过市场开关、资金和重复持仓检查；
- 页面显示待确认交易计划。

验收：每日 14:45 后，系统能自动生成待买计划；没有成交时也能查看拒绝原因。

### 阶段 2：模拟盘自动执行

目标：信号、买入、卖出和收益面板形成完整闭环。

实现：

- 建立 paper account；
- 模拟买入成交；
- 形成真实持仓状态；
- 收盘后执行退出判断；
- 次日开盘模拟卖出；
- 处理 T+1、涨跌停和部分成交；
- 输出每日账户净值和策略归因。

验收：连续运行至少 20 个交易日，所有信号都能追踪到计划、订单、成交或明确失败原因。

### 阶段 3：券商只读同步

目标：先同步真实账户，不自动下单。

实现：

- 同步资金；
- 同步持仓；
- 同步当日委托；
- 同步历史成交；
- 与本地账本对账；
- 导入功能作为异常补录工具保留。

验收：系统能指出本地和券商之间的现金、持仓、成交差异。

### 阶段 4：人工确认实盘

目标：系统生成真实订单，但每笔由用户确认。

实现：

- 默认 `manual_confirm`；
- 展示股票、价格、股数、风险和卖出规则；
- 用户确认后提交券商；
- 订单状态轮询；
- 成交回写；
- 网络异常进入 `unknown` 并强制对账。

验收：连续运行一段时间，订单、成交和持仓始终一致，没有重复订单。

### 阶段 5：受控自动实盘

目标：只有在模拟盘和人工确认阶段稳定后，才开放自动下单。

必须具备：

- 全局 kill switch；
- 单笔和单日金额上限；
- 最大持仓和行业集中度；
- 最大连续亏损限制；
- 行情过期自动停单；
- 券商异常自动停单；
- 订单幂等；
- 完整审计日志；
- 启动前人工确认交易模式；
- 所有订单可追溯到信号和策略版本。

验收：先小资金运行，并且能随时切回只读或模拟模式。

## 14. 测试计划

### 14.1 策略和卖出规则

必须测试：

- 14:45 信号不会使用 15:00 才知道的价格；
- D1 是下一个交易日；
- 收盘跌破止损点后次日开盘卖出；
- 买入当天不允许卖出；
- 达到移动止盈条件后生成卖出计划；
- 持有天数达到上限后生成卖出计划；
- 多个退出条件同时触发时优先级稳定；
- 同时止盈和止损的日 K 样本标记 `ambiguous`；
- 停牌和跌停时卖出订单顺延；
- 交易日跨周末和节假日计算正确。

### 14.2 订单幂等和异常

必须测试：

- 自动任务重复运行不会重复买入；
- 提交超时会先查询订单，不会直接重复下单；
- 部分成交后只对剩余数量处理；
- 撤单和拒单不会增加持仓；
- 成交回报重复推送不会重复计算持仓；
- 账户对账能发现本地和券商差异；
- kill switch 能阻止新订单但不会隐藏已有持仓。

### 14.3 固定样本

准备一组人工可验证样本，至少包含：

- 正常买入并止盈；
- 买入后止损；
- 买入后先盈利再移动止盈；
- 买入当天触发退出但受 T+1 限制；
- 涨停无法买入；
- 跌停无法卖出；
- 部分成交；
- 停牌；
- 网络超时和订单状态未知。

## 15. 最终建议的第一版交易规则

为了尽快跑通闭环，第一版建议不要过于复杂：

### 买入

```text
每天 14:45 运行策略
信号通过风控后生成计划
只买入评分最高且满足仓位约束的股票
同一股票同一交易日只允许一个买入计划
优先使用下一条有效报价模拟成交
无法获得有效报价则不成交并记录原因
```

### 卖出

```text
收盘价跌破技术无效点
    -> 下一交易日开盘卖出

持仓最高收盘价达到 +8%，之后从峰值回撤 4%
    -> 下一交易日开盘卖出

持有超过 10 个交易日仍未退出
    -> 下一交易日开盘卖出

跌停、停牌或接口失败
    -> 保持待卖状态，下一可交易时段继续处理
```

这套规则的优点是：容易解释、容易回测、容易在模拟盘中验证，也符合当前项目已有的“收盘判断、次日开盘执行”和 T+1 处理方式。

## 16. 关键风险提醒

### 16.1 真实自动下单的风险

自动买入不是普通功能开关。必须考虑：

- 网络重试造成重复订单；
- 行情延迟导致价格已经失效；
- 券商接口返回超时但订单实际已成功；
- 进程重启造成状态丢失；
- 时间同步错误；
- 账户资金和本地缓存不一致；
- 部分成交导致仓位和计划不一致；
- 停牌、涨停、跌停和交易规则变化。

所以建议先做到：

```text
全自动信号
全自动模拟盘
人工确认实盘
最后才是受控自动实盘
```

### 16.2 策略收益和交易收益要分开

一只股票后面上涨，不代表你的交易赚到了钱。可能出现：

- 信号产生后没有成交；
- 次日跳空高开导致买入成本过高；
- 买入后先止损，之后才上涨；
- 手续费和滑点吃掉小幅收益；
- 可卖数量受 T+1 限制；
- 卖出规则过早导致错过后续上涨。

因此必须同时保留：

```text
Signal return：信号预测能力
Trade return：成交和退出规则后的收益
Portfolio return：资金和仓位管理后的账户收益
```

只有三者都能追踪，才能知道问题出在选股、执行、卖出还是资金管理。

## 17. 完成定义

当下面这些条件满足时，可以认为“策略接入交易系统 V1”完成：

- 策略信号自动落库；
- 每个信号都有信号时间、信号价格、策略版本和行情来源；
- 信号自动生成交易计划；
- 交易计划包含仓位、止损、止盈、最大持有期；
- 风控拒绝和交易失败都有明确原因；
- 订单、成交和持仓彼此分开；
- 只根据成交更新真实持仓；
- 卖出规则每天自动检查；
- D1/D3/D5/D10 研究收益与真实交易收益分开；
- 能把每笔交易归因到信号和策略；
- 模拟盘至少稳定运行 20 个交易日；
- 订单重复、部分成交、超时和停牌都有测试；
- 有 kill switch 和账户级风险上限；
- 导入功能已经变成对账和补录工具，而不是主交易入口。

## 18. 推荐下一步

下一步最适合做的是阶段 1：先不接真实券商，完成“策略信号自动生成交易计划”。具体顺序：

1. 给 `selection_results` 补齐 `signal_at`、`signal_price`、`signal_score` 和 `strategy_version`。
2. 新增 `trading_plans` 表和 Go 结构体。
3. 把 14:45 信号转换为买入计划。
4. 复用现有仓位和费用规则计算股数。
5. 实现止损、移动止盈和最大持有期的退出计划生成。
6. 在现有交易页面增加“待执行计划”区域。
7. 先用 paper 模式跑通完整买入和卖出闭环。

等这个闭环跑稳定之后，再实现订单和成交表，最后才接券商接口。这样你的系统会从“上传本地交易的记录工具”变成真正由策略驱动、能够解释收益来源的低频量化交易系统。
