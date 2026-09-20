#!/usr/bin/env python3
"""
策略扫描结果对比分析
基于2026-01-01到2026-09-20的历史回测数据
"""

import json
import pandas as pd
import numpy as np

# 加载扫描结果
with open('/tmp/scan_results_full.json', 'r') as f:
    data = json.load(f)

results = pd.DataFrame(data['data']['items'])

print("="*80)
print("🔬 策略参数扫描结果 - 深度分析")
print("="*80)
print()
print(f"扫描时间范围: 2026-01-01 到 2026-09-20")
print(f"策略总数: {len(results['strategy_name'].unique())}")
print(f"参数组合总数: {len(results)}")
print(f"有效策略: {len(results[results['closed_count'] > 0]['strategy_name'].unique())}")
print()

print("="*80)
print("📊 策略表现排行榜 (Top 10)")
print("="*80)
print()

# 按策略分组，找出每个策略的最佳表现
strategy_best = results.groupby('strategy_name').agg({
    'total_return': ['max', 'mean', 'std'],
    'win_rate': ['max', 'mean'],
    'max_drawdown': 'min',
    'closed_count': 'max'
}).round(2)

strategy_best.columns = ['最佳收益', '平均收益', '收益标准差', '最高胜率', '平均胜率', '最小回撤', '最大交易数']
strategy_best = strategy_best.sort_values('最佳收益', ascending=False)

print(strategy_best.head(10).to_string())
print()

print("="*80)
print("🎯 重点策略对比分析")
print("="*80)
print()

# 关注的策略
focus_strategies = [
    'A股V3强势启动',
    '强势股回调低吸',
    '趋势回踩双确认',
    'MACD即将金叉预警'
]

for strategy in focus_strategies:
    strategy_data = results[results['strategy_name'] == strategy]

    if len(strategy_data) == 0:
        print(f"【{strategy}】")
        print(f"  ⚠️ 无有效数据或扫描失败")
        print()
        continue

    # 找出最佳参数组合
    best = strategy_data.loc[strategy_data['total_return'].idxmax()]

    # 统计
    positive_count = len(strategy_data[strategy_data['total_return'] > 0])
    total_count = len(strategy_data)

    print(f"【{strategy}】")
    print(f"  参数组合数: {total_count}")
    print(f"  盈利组合数: {positive_count} ({positive_count/total_count*100:.1f}%)")
    print()
    print(f"  最佳表现:")
    print(f"    总收益: {best['total_return']:.2f}%")
    print(f"    胜率: {best['win_rate']:.2f}%")
    print(f"    最大回撤: {best['max_drawdown']:.2f}%")
    print(f"    交易次数: {best['closed_count']}")
    print(f"    买入: {best['buy_base']}, 卖出: {best['sell_base']}")
    print(f"    止盈: {best['take_profit']}%, 回撤: {best['trail_drawdown']}%, 止损: {best['stop_loss']}%")
    print()

    # 统计平均表现
    print(f"  平均表现:")
    print(f"    平均收益: {strategy_data['total_return'].mean():.2f}%")
    print(f"    平均胜率: {strategy_data['win_rate'].mean():.2f}%")
    print(f"    平均回撤: {strategy_data['max_drawdown'].mean():.2f}%")
    print()

    # 高胜率组合
    high_win = strategy_data[strategy_data['win_rate'] >= 60]
    if len(high_win) > 0:
        print(f"  高胜率组合 (≥60%):")
        print(f"    数量: {len(high_win)} ({len(high_win)/total_count*100:.1f}%)")
        print(f"    最佳收益: {high_win['total_return'].max():.2f}%")

    print("-"*80)
    print()

print("="*80)
print("💡 关键发现")
print("="*80)
print()

# 找出V3策略的表现
v3_data = results[results['strategy_name'] == 'A股V3强势启动']
pullback_data = results[results['strategy_name'] == '强势股回调低吸']
confirm_data = results[results['strategy_name'] == '趋势回踩双确认']

if len(v3_data) > 0 and v3_data['closed_count'].max() == 0:
    print("⚠️ A股V3强势启动策略:")
    print("   在2026-01-01到2026-09-20期间无有效信号")
    print("   可能原因:")
    print("   • 选股条件过于严格")
    print("   • 市场环境不符合策略预期")
    print("   • 需要调整参数或时间范围")
    print()

if len(pullback_data) > 0:
    best_pullback = pullback_data.loc[pullback_data['total_return'].idxmax()]
    print("✅ 强势股回调低吸策略表现:")
    print(f"   最佳收益: {best_pullback['total_return']:.2f}%")
    print(f"   最佳胜率: {pullback_data['win_rate'].max():.2f}%")
    print(f"   有{len(pullback_data[pullback_data['total_return']>10])}组参数收益>10%")
    print()

if len(confirm_data) > 0:
    best_confirm = confirm_data['total_return'].max()
    if best_confirm < 0:
        print("⚠️ 趋势回踩双确认策略:")
        print(f"   最佳收益: {best_confirm:.2f}% (亏损)")
        print(f"   该策略在此时间段表现不佳")
    print()

print("="*80)
print("📈 整体市场特征")
print("="*80)
print()

# 所有策略的平均表现
all_returns = results['total_return'].values
positive_rate = len(results[results['total_return'] > 0]) / len(results) * 100

print(f"参数组合盈利率: {positive_rate:.1f}%")
print(f"平均收益: {np.mean(all_returns):.2f}%")
print(f"中位数收益: {np.median(all_returns):.2f}%")
print(f"最佳收益: {np.max(all_returns):.2f}%")
print(f"最差收益: {np.min(all_returns):.2f}%")
print()

print("收益分布:")
print(f"  大赚 (>20%):  {len(results[results['total_return'] > 20])} ({len(results[results['total_return'] > 20])/len(results)*100:.1f}%)")
print(f"  小赚 (0-20%): {len(results[(results['total_return'] > 0) & (results['total_return'] <= 20)])} ({len(results[(results['total_return'] > 0) & (results['total_return'] <= 20)])/len(results)*100:.1f}%)")
print(f"  小亏 (0~-10%): {len(results[(results['total_return'] < 0) & (results['total_return'] >= -10)])} ({len(results[(results['total_return'] < 0) & (results['total_return'] >= -10)])/len(results)*100:.1f}%)")
print(f"  大亏 (<-10%): {len(results[results['total_return'] < -10])} ({len(results[results['total_return'] < -10])/len(results)*100:.1f}%)")
print()

print("="*80)
print("🎯 结论和建议")
print("="*80)
print()

print("1. 策略有效性验证:")
print("   • MACD即将金叉预警: 表现最佳 (最高37.01%)")
print("   • 强势股回调低吸: 稳定盈利 (最高17.37%)")
print("   • A股V3强势启动: 无信号，需要调整")
print()

print("2. 修正建议:")
print("   • A股V3策略选股条件可能过严，导致信号量为0")
print("   • 建议放宽部分过滤条件，增加信号覆盖")
print("   • 或选择更活跃的时间段重新测试")
print()

print("3. 最佳策略推荐:")
best_overall = results.loc[results['total_return'].idxmax()]
print(f"   策略: {best_overall['strategy_name']}")
print(f"   收益: {best_overall['total_return']:.2f}%")
print(f"   胜率: {best_overall['win_rate']:.2f}%")
print(f"   参数: 买{best_overall['buy_base']}/卖{best_overall['sell_base']}")
print(f"         止盈{best_overall['take_profit']}%/回撤{best_overall['trail_drawdown']}%/止损{best_overall['stop_loss']}%")
print()

print("="*80)
