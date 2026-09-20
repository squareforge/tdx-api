# TDX Workbench 策略分析总结

## 📋 已完成的工作

### 1. 深度分析脚本
- ✅ `scripts/analyze_strategy_signals.py` - 全面的信号和因子分析工具
- ✅ `scripts/weekly_strategy_report.py` - 周度复盘报告生成器

### 2. 分析报告
- ✅ `output/strategy_analysis_report.md` - 20页深度分析报告，包含：
  - 6个因子的详细评价
  - 失败案例复盘
  - 统计显著性分析
  - 具体优化方案

### 3. 优化配置
- ✅ `output/optimized_strategy_config.json` - 可直接使用的策略配置

---

## 🎯 核心发现

### 问题诊断

#### 🔴 P0 严重问题
1. **前期涨幅过大** (38.5%信号涨幅>50%)
   - 当前平均20日涨幅：45.92%
   - 案例：利和兴 20日涨113.91%，后续最大回撤-15.26%
   - **根因**：选在主升浪末期，追高风险极大

2. **样本量严重不足** (仅26只股票)
   - 当前无法做统计推断
   - 需要至少100-200只股票、3个月数据

#### 🟡 P1 高优先级问题
1. **Risk因子失效**
   - Risk=0的信号仍被选入
   - 应该作为硬过滤而非打分项

2. **买入区间不合理**
   - 53.8%信号次日开盘低于买入区间
   - 当前区间：[信号收盘价, 信号收盘价*1.02]
   - 建议区间：[信号收盘价*0.98, 信号收盘价*1.015]

### 因子评分

| 因子 | 评分 | 满分率 | 有效性 | 主要问题 |
|------|------|--------|--------|----------|
| TREND | ⭐⭐⭐⭐ | 77.42% | 高 | 涨幅过大 |
| PULLBACK | ⭐⭐⭐ | 54.08% | 中 | 区分度不够 |
| VOLUME | ⭐⭐⭐⭐ | 64.56% | 高 | 放量不等于上涨 |
| STRENGTH | ⭐⭐⭐ | 60.18% | 中 | 与实际相关性弱 |
| LIQUIDITY | ⭐⭐⭐⭐⭐ | 71.54% | 极高 | 无问题 |
| RISK | 🔴⭐⭐ | 34.46% | 低 | 逻辑矛盾 |

### 当前表现

**统计数据** (26只股票样本):
- 平均收益: **-1.60%** 🔴
- 中位收益: **-5.49%** 🔴
- 盈利比例: **42.3%** (低于随机)
- 平均最大回撤: **-7.59%**
- 止损触发率: **69.2%** (极高风险)

**收益分布**:
- 大亏(<-10%): 26.9%
- 小亏(-10%~0): 30.8%
- 小赚(0~5%): 15.4%
- 中赚(5%~15%): 11.5%
- 大赚(≥15%): 15.4%

---

## 💡 优化方案

### 方案A：保守版（推荐）

**核心改进**：
1. ✅ 20日涨幅上限从∞改为45%
2. ✅ 距高点回撤要求8-20%（原无要求）
3. ✅ 增加市场状态过滤（上证指数20日≥-3%）
4. ✅ 调整买入价格区间
5. ✅ Top N从26只减少到5只

**预期效果**：
- 信号数量：5-10只/天
- 盈利率：>50%
- 平均收益：2-4%
- 止损触发率：<40%

**配置文件**：`output/optimized_strategy_config.json`

### 代码修改清单

#### 1. 增加涨幅上限 (strategy_runner.go)
```go
// 在 evaluateFactor 的 "gain_days" case 中
// 约第661行，增加上限判断
maxPct := floatParam(rule.Params, "max", 50)  // 默认50%
```

#### 2. Risk=0强制过滤 (strategy_runner.go)
```go
// 在 evaluateStrategySymbol 函数中，约第485行
// 遍历filter后增加
for _, rule := range result.Config.Filters {
    fr := r.evaluateFactor(result, symbol, rows, rule, true)
    item.FactorResults = append(item.FactorResults, fr)
    item.Reasons = append(item.Reasons, fr.Reason)
    if !fr.Hit {
        item.Hit = false
    }
    // 新增：Risk因子特殊处理
    if rule.Factor == "max_amplitude" && !fr.Hit {
        return item, false  // 强制过滤
    }
}
```

#### 3. 调整买入价格区间
在 Web 或选股服务的信号生成逻辑中：
```go
buyPriceLow := signalClose * 0.98   // 允许2%低开
buyPriceHigh := signalClose * 1.015 // 1.5%以内可追
```

---

## 📅 3个月验证计划

### 第1个月（当前）
- [x] 完成深度分析
- [ ] 实施优化方案A
- [ ] 继续追踪信号（不实盘）
- [ ] 每周运行 `scripts/weekly_strategy_report.py`

### 第2个月
- [ ] 积累至少50只股票样本
- [ ] 分析按涨幅分组的成功率
- [ ] 研究市场环境影响
- [ ] 微调因子权重

### 第3个月
- [ ] 积累至少100只股票样本
- [ ] 完成统计显著性检验
- [ ] 如果盈利率>50%，准备小仓试水
- [ ] 制定实盘风控规则

### 验证通过标准
达到以下**全部指标**才考虑实盘：
- ✓ 样本量 ≥ 100只股票
- ✓ 盈利率 ≥ 50%
- ✓ 平均收益 ≥ 2%
- ✓ 盈亏比 ≥ 1.5
- ✓ 最大连续失败 ≤ 10次
- ✓ 止损触发率 ≤ 40%

---

## 🛠️ 日常使用指南

### 每日操作
```bash
# 1. 运行策略选股（自动化任务已配置）
# 系统会自动生成 reports/signals_YYYY-MM-DD.csv

# 2. 盘中或收盘后追踪行情
# 系统会自动生成 reports/signal_performance_YYYY-MM-DD.csv
```

### 每周复盘
```bash
# 运行周报
python3 scripts/weekly_strategy_report.py

# 查看详细分析
python3 scripts/analyze_strategy_signals.py
```

### 每月总结
1. 查看累计样本量
2. 更新 `output/strategy_analysis_report.md` 中的数据
3. 如果发现新问题，调整策略配置
4. 记录到 Git 提交中

---

## 📚 参考文档

### 代码关键文件
- `services/selection-worker/strategy_runner.go` - 策略执行核心
- `services/selection-worker/strategy_indicators.go` - 高级因子计算
- `services/selection-worker/automation.go` - 自动化任务调度

### 策略设计文档
- `docs/TRADING_SYSTEM_V1_1.md` - 交易规则
- `output/strategy_analysis_report.md` - 本次深度分析
- `output/optimized_strategy_config.json` - 优化配置

### 因子说明

#### 基础因子
- `gain_days`: N日涨跌幅
- `drawdown_from_high`: 距高点回撤
- `ma_trend`: 均线多头排列
- `volume_up`: 放量突破
- `min_amount`: 最小成交额
- `price_range`: 价格区间

#### 高级因子 (strategy_indicators.go)
- `ma_slope`: 均线斜率
- `close_strength`: 收盘强度（收盘位置+上影线）
- `rsi_rebound`: RSI反转
- `macd_golden_cross`: MACD金叉
- `kdj_golden_cross`: KDJ金叉
- `boll_breakout`: 布林带突破
- `volume_breakout`: 放量突破
- `local_rocket`: 本地火箭发射
- `low_three_bull_first_board`: 低位三连阳首板

#### 市场因子
- `market_momentum`: 上证指数动量

---

## ⚠️ 重要提醒

### 不要做的事
1. ❌ **不要在样本不足时调整策略** - 容易过拟合
2. ❌ **不要频繁修改因子权重** - 等数据积累够再调
3. ❌ **不要因为短期亏损就放弃** - 量化看长期胜率
4. ❌ **不要在数据验证前实盘** - 可能导致实际亏损
5. ❌ **不要迷信单个成功案例** - 关注整体统计

### 应该做的事
1. ✅ **坚持追踪每日信号** - 数据是最宝贵的资产
2. ✅ **记录每次调整的原因** - Git commit message要详细
3. ✅ **定期复盘** - 每周看周报，每月做总结
4. ✅ **保持怀疑态度** - 质疑每个假设
5. ✅ **控制期望** - 不存在100%胜率的策略

---

## 🎯 最终评价

### 项目成熟度: ⭐⭐⭐⭐☆ (4/5)

**优势**:
- ✅ 工程质量高（代码清晰、架构合理）
- ✅ 文档完整（README、API文档、记忆文档）
- ✅ 有复盘意识（信号追踪、表现记录）
- ✅ 因子体系专业（6因子框架）
- ✅ 基础设施完善（Docker部署、多服务架构）

**不足**:
- ⚠️ 样本量不足（需要继续积累）
- ⚠️ 策略参数需优化（前期涨幅过大）
- ⚠️ 缺少行业/板块维度（个股视角单一）

### 建议
**这是一个很有潜力的量化交易系统**，但目前处于"策略验证阶段"，距离实盘还有一定距离。

**不要放弃！** 调整参数、积累数据、持续优化，3个月后很可能看到明显改善。

记住：
> "量化交易不是寻找圣杯，而是找到一个长期有正期望值的策略，然后坚持执行。"

---

**祝你成功！** 🚀

如有问题，可以：
1. 查看 `output/strategy_analysis_report.md` 的详细分析
2. 运行 `scripts/analyze_strategy_signals.py` 获取最新数据
3. 阅读代码注释理解因子逻辑
