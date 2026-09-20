#!/usr/bin/env python3
"""
双轨回测评估工具
1. 信号质量评估（无限制买入）- 评估策略选股能力
2. 实盘模拟回测（有限制）- 评估实际可操作性
"""

import pandas as pd
import numpy as np
from datetime import datetime, timedelta

def evaluate_signal_quality(signals_df, performance_df):
    """
    评估1: 信号质量（无限制买入）
    评估策略选出的所有信号的平均表现
    """
    print("="*80)
    print("📊 评估1: 信号质量分析（无限制买入）")
    print("="*80)

    # 获取每个信号的最终表现
    perf_latest = performance_df.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()

    # 合并信号和表现数据
    # signals的signal_date是信号生成日期
    merged = pd.merge(
        signals_df,
        perf_latest[['signal_date', 'code', 'change_pct', 'max_gain_pct', 'max_drawdown_pct']],
        left_on=['signal_date', 'code'],
        right_on=['signal_date', 'code'],
        how='inner'
    )

    valid = merged[merged['change_pct'].notna()]

    if len(valid) == 0:
        print("\n⚠️  无有效追踪数据")
        return None

    print(f"\n总信号数: {len(signals_df)}")
    print(f"有追踪数据: {len(valid)}")

    # 整体成功率（策略的真实水平）
    win_rate = (valid['change_pct'] > 0).sum() / len(valid)
    avg_return = valid['change_pct'].mean()
    median_return = valid['change_pct'].median()

    print(f"\n【策略真实水平】")
    print(f"  成功率: {win_rate:.1%} ← 这是策略的内在质量")
    print(f"  平均收益: {avg_return:+.2%}")
    print(f"  中位收益: {median_return:+.2%}")

    # 按涨幅分段分析
    print(f"\n收益分布:")
    ranges = [
        ("大亏(<-10%)", lambda x: x < -0.1),
        ("小亏(-10%~0)", lambda x: (x >= -0.1) & (x < 0)),
        ("小赚(0~5%)", lambda x: (x >= 0) & (x < 0.05)),
        ("中赚(5%~10%)", lambda x: (x >= 0.05) & (x < 0.1)),
        ("大赚(≥10%)", lambda x: x >= 0.1)
    ]

    for name, condition in ranges:
        count = condition(valid['change_pct']).sum()
        pct = count / len(valid) * 100
        print(f"  {name:15s}: {count:3d} ({pct:5.1f}%)")

    return valid

def evaluate_ranking_accuracy(valid_df):
    """
    评估2: 排名准确性
    检查rank_score是否能准确预测最终收益
    """
    print("\n" + "="*80)
    print("🎯 评估2: 排名准确性分析")
    print("="*80)

    # 按日期分组，每天单独分析
    daily_results = []

    for date, group in valid_df.groupby('date'):
        if len(group) < 3:
            continue

        # 按rank_score排序
        group = group.sort_values('rank_score', ascending=False)

        # Top3 vs 其他
        top3 = group.head(3)
        others = group.iloc[3:]

        top3_avg = top3['change_pct'].mean()
        others_avg = others['change_pct'].mean() if len(others) > 0 else 0

        # 排名是否准确
        accurate = top3_avg > others_avg

        daily_results.append({
            'date': date,
            'total': len(group),
            'top3_avg': top3_avg,
            'others_avg': others_avg,
            'accurate': accurate,
            'top3_win_rate': (top3['change_pct'] > 0).sum() / 3
        })

    daily_df = pd.DataFrame(daily_results)

    print(f"\n分析天数: {len(daily_df)}")
    print(f"\n【排名准确性】")

    # 排名准确率
    ranking_accuracy = daily_df['accurate'].sum() / len(daily_df)
    print(f"  Top3优于其他: {ranking_accuracy:.1%} ← 排名准确率")

    # Top3平均表现
    avg_top3_return = daily_df['top3_avg'].mean()
    avg_others_return = daily_df['others_avg'].mean()

    print(f"\n平均收益对比:")
    print(f"  Top3平均: {avg_top3_return:+.2%}")
    print(f"  其他平均: {avg_others_return:+.2%}")
    print(f"  优势差距: {(avg_top3_return - avg_others_return):+.2%}")

    if avg_top3_return < avg_others_return:
        print(f"\n⚠️  警告: Top3表现反而更差！排名逻辑可能有问题")

    # Top3成功率
    avg_top3_win_rate = daily_df['top3_win_rate'].mean()
    print(f"\nTop3成功率: {avg_top3_win_rate:.1%}")

    return daily_df

def evaluate_position_limit_impact(valid_df, max_positions=3):
    """
    评估3: 持仓限制影响
    对比无限制 vs 有限制的收益差异
    """
    print("\n" + "="*80)
    print(f"💰 评估3: 持仓限制影响（最多{max_positions}只）")
    print("="*80)

    # 按日期分组
    unlimited_returns = []
    limited_returns = []

    for date, group in valid_df.groupby('date'):
        # 无限制：所有信号
        unlimited_avg = group['change_pct'].mean()
        unlimited_returns.extend(group['change_pct'].tolist())

        # 有限制：只买Top N
        group_sorted = group.sort_values('rank_score', ascending=False)
        limited_group = group_sorted.head(max_positions)
        limited_avg = limited_group['change_pct'].mean()
        limited_returns.extend(limited_group['change_pct'].tolist())

    print(f"\n【无限制买入】（假设无限资金）")
    print(f"  信号数: {len(unlimited_returns)}")
    print(f"  成功率: {(pd.Series(unlimited_returns) > 0).sum() / len(unlimited_returns):.1%}")
    print(f"  平均收益: {np.mean(unlimited_returns):+.2%}")

    print(f"\n【限制买入Top{max_positions}】（实际约束）")
    print(f"  信号数: {len(limited_returns)}")
    print(f"  成功率: {(pd.Series(limited_returns) > 0).sum() / len(limited_returns):.1%}")
    print(f"  平均收益: {np.mean(limited_returns):+.2%}")

    # 损失评估
    lost_opportunity = np.mean(unlimited_returns) - np.mean(limited_returns)
    print(f"\n【机会成本】")
    print(f"  收益损失: {lost_opportunity:+.2%}")

    if lost_opportunity > 0.02:
        print(f"  ⚠️  因持仓限制损失{lost_opportunity:.2%}，排名算法需要优化！")
    elif lost_opportunity < -0.02:
        print(f"  ✓ Top{max_positions}选择有效，比全部买入好{-lost_opportunity:.2%}")
    else:
        print(f"  ~ 持仓限制影响不大")

def analyze_ranking_factors(valid_df):
    """
    评估4: 哪些因子对最终收益预测最准
    """
    print("\n" + "="*80)
    print("🔬 评估4: 因子预测能力分析")
    print("="*80)

    # 提取因子得分
    def parse_factors(rank_reason):
        factors = {}
        if pd.isna(rank_reason):
            return factors
        parts = rank_reason.split(',')
        for part in parts:
            part = part.strip()
            if '=' in part and '/' in part:
                key, value = part.split('=')
                key = key.strip()
                score_str, total_str = value.split('/')
                try:
                    factors[key] = float(score_str) / float(total_str) if float(total_str) > 0 else 0
                except:
                    pass
        return factors

    valid_df['factors'] = valid_df['rank_reason'].apply(parse_factors)

    factor_names = ['trend', 'pullback', 'volume', 'strength', 'liquidity', 'risk']

    for factor in factor_names:
        valid_df[f'{factor}_ratio'] = valid_df['factors'].apply(
            lambda x: x.get(factor, np.nan)
        )

    print(f"\n因子与最终收益的相关系数:")
    print("-" * 80)

    correlations = []
    for factor in factor_names:
        col = f'{factor}_ratio'
        if col in valid_df.columns:
            clean_data = valid_df[[col, 'change_pct']].dropna()
            if len(clean_data) > 1:
                corr = clean_data[col].corr(clean_data['change_pct'])
                correlations.append((factor, corr))

                symbol = "↑" if corr > 0 else "↓"
                strength = "强" if abs(corr) > 0.3 else "中" if abs(corr) > 0.1 else "弱"
                print(f"  {factor.upper():12s}: {corr:+.3f} {symbol} ({strength})")

    # 建议
    print(f"\n【优化建议】")
    correlations.sort(key=lambda x: abs(x[1]), reverse=True)

    strong_factors = [f for f, c in correlations if abs(c) > 0.2]
    weak_factors = [f for f, c in correlations if abs(c) < 0.05]

    if strong_factors:
        print(f"  ✓ 相关性强的因子: {', '.join([f.upper() for f in strong_factors])}")
        print(f"    → 建议增加这些因子的权重")

    if weak_factors:
        print(f"  ⚠️ 相关性弱的因子: {', '.join([f.upper() for f in weak_factors])}")
        print(f"    → 考虑降低权重或重新设计")

def main():
    print("="*80)
    print("TDX Workbench - 双轨回测评估工具")
    print("="*80)

    # 加载数据
    import glob

    signals_files = sorted(glob.glob('reports/signals_*.csv'))
    signals_files = [f for f in signals_files if 'top3' not in f and 'smoke' not in f]

    perf_files = sorted(glob.glob('reports/signal_performance_*.csv'))

    if not signals_files or not perf_files:
        print("\n❌ 缺少数据文件")
        return

    # 加载信号
    signals_list = []
    for f in signals_files:
        df = pd.read_csv(f, encoding='utf-8-sig')
        signals_list.append(df)

    signals_df = pd.concat(signals_list, ignore_index=True)

    # 加载追踪
    perf_list = []
    for f in perf_files:
        df = pd.read_csv(f, encoding='utf-8-sig')
        if len(df) > 0:
            perf_list.append(df)

    perf_df = pd.concat(perf_list, ignore_index=True)

    print(f"\n数据概况:")
    print(f"  信号数: {len(signals_df)}")
    print(f"  追踪记录: {len(perf_df)}")

    # 评估1: 信号质量
    valid_df = evaluate_signal_quality(signals_df, perf_df)

    if valid_df is None or len(valid_df) < 5:
        print("\n⚠️  数据不足，无法进行后续分析")
        return

    # 评估2: 排名准确性
    evaluate_ranking_accuracy(valid_df)

    # 评估3: 持仓限制影响
    evaluate_position_limit_impact(valid_df, max_positions=3)

    # 评估4: 因子预测能力
    analyze_ranking_factors(valid_df)

    print("\n" + "="*80)
    print("✓ 分析完成")
    print("="*80)

if __name__ == '__main__':
    main()
