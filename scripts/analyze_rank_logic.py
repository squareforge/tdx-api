#!/usr/bin/env python3
"""
分析每个策略的信号和rank逻辑
验证排名算法是否合理
"""

import pandas as pd
import numpy as np
import glob
from datetime import datetime

print("="*80)
print("🔍 策略信号分析 - Rank逻辑验证")
print("="*80)
print()

# 加载所有信号文件
signal_files = sorted(glob.glob('reports/signals_*.csv'))
signal_files = [f for f in signal_files if 'top3' not in f and 'smoke' not in f]

if not signal_files:
    print("❌ 没有找到信号文件")
    exit(1)

print(f"信号文件数: {len(signal_files)}")
print()

# 加载所有信号
all_signals = []
for f in signal_files:
    df = pd.read_csv(f, encoding='utf-8-sig')
    if len(df) > 0:
        all_signals.append(df)

if not all_signals:
    print("❌ 没有有效的信号数据")
    exit(1)

signals_df = pd.concat(all_signals, ignore_index=True)

print(f"总信号数: {len(signals_df)}")
print(f"时间范围: {signals_df['date'].min()} 到 {signals_df['date'].max()}")
print()

# 按日期统计
print("="*80)
print("📅 每日信号统计")
print("="*80)
print()

daily_stats = signals_df.groupby('date').agg({
    'code': 'count',
    'rank_score': ['mean', 'std', 'min', 'max']
}).round(2)

daily_stats.columns = ['信号数', 'rank均值', 'rank标准差', 'rank最小', 'rank最大']
print(daily_stats.to_string())
print()

# 分析rank_score的构成
print("="*80)
print("🎯 Rank Score 分析")
print("="*80)
print()

print(f"Rank Score 范围: {signals_df['rank_score'].min():.2f} ~ {signals_df['rank_score'].max():.2f}")
print(f"Rank Score 均值: {signals_df['rank_score'].mean():.2f}")
print(f"Rank Score 中位数: {signals_df['rank_score'].median():.2f}")
print(f"Rank Score 标准差: {signals_df['rank_score'].std():.2f}")
print()

# 解析rank_reason，分析各因子得分
print("="*80)
print("📊 因子得分分析")
print("="*80)
print()

def parse_rank_reason(reason):
    """解析rank_reason字符串"""
    factors = {}
    if pd.isna(reason):
        return factors

    parts = reason.split(',')
    for part in parts:
        part = part.strip()
        if '=' in part and '/' in part:
            key, value = part.split('=')
            key = key.strip()
            try:
                score_str, total_str = value.split('/')
                score = float(score_str)
                total = float(total_str)
                factors[key] = {'score': score, 'total': total, 'ratio': score/total if total > 0 else 0}
            except:
                pass
    return factors

# 解析所有信号的因子得分
signals_df['factors'] = signals_df['rank_reason'].apply(parse_rank_reason)

# 统计各因子的平均得分
factor_names = ['trend', 'pullback', 'volume', 'strength', 'liquidity', 'risk']
factor_stats = {}

for factor in factor_names:
    scores = []
    totals = []
    ratios = []

    for factors in signals_df['factors']:
        if factor in factors:
            scores.append(factors[factor]['score'])
            totals.append(factors[factor]['total'])
            ratios.append(factors[factor]['ratio'])

    if scores:
        factor_stats[factor] = {
            'avg_score': np.mean(scores),
            'avg_total': np.mean(totals),
            'avg_ratio': np.mean(ratios),
            'count': len(scores)
        }

print("因子平均得分:")
print("-"*80)
print(f"{'因子':<12s} {'平均得分':<10s} {'满分':<10s} {'得分率':<10s} {'样本数':<10s}")
print("-"*80)

for factor, stats in sorted(factor_stats.items(), key=lambda x: x[1]['avg_ratio'], reverse=True):
    print(f"{factor.upper():<12s} {stats['avg_score']:<10.2f} {stats['avg_total']:<10.2f} {stats['avg_ratio']:<10.1%} {stats['count']:<10d}")

print()

# 分析Top3和其他的区别
print("="*80)
print("🏆 Top3 vs 其他 - Rank逻辑验证")
print("="*80)
print()

for date in signals_df['date'].unique():
    date_signals = signals_df[signals_df['date'] == date].copy()

    if len(date_signals) < 5:
        continue

    # 按rank_score排序
    date_signals = date_signals.sort_values('rank_score', ascending=False).reset_index(drop=True)

    top3 = date_signals.head(3)
    others = date_signals.iloc[3:]

    print(f"【{date}】 信号数: {len(date_signals)}")
    print()

    print("Top3:")
    for idx, row in top3.iterrows():
        print(f"  {idx+1}. {row['name']:<10s} ({row['code']}) rank={row['rank_score']:.1f}")
        if 'ret20' in row:
            print(f"      20日涨幅: {row.get('ret20', 0)*100:.1f}%  收盘: {row['close']:.2f}")
        # 解析rank_reason
        factors = parse_rank_reason(row['rank_reason'])
        factor_str = []
        for f in ['trend', 'risk', 'volume', 'strength']:
            if f in factors:
                factor_str.append(f"{f.upper()}={factors[f]['score']:.1f}/{factors[f]['total']:.0f}")
        print(f"      {', '.join(factor_str)}")

    print()
    print(f"其他 (样本3个):")
    for idx, row in others.head(3).iterrows():
        print(f"  {idx+1}. {row['name']:<10s} ({row['code']}) rank={row['rank_score']:.1f}")
        if 'ret20' in row:
            print(f"      20日涨幅: {row.get('ret20', 0)*100:.1f}%  收盘: {row['close']:.2f}")
        factors = parse_rank_reason(row['rank_reason'])
        factor_str = []
        for f in ['trend', 'risk', 'volume', 'strength']:
            if f in factors:
                factor_str.append(f"{f.upper()}={factors[f]['score']:.1f}/{factors[f]['total']:.0f}")
        print(f"      {', '.join(factor_str)}")

    print()
    print(f"Top3平均rank: {top3['rank_score'].mean():.2f}")
    print(f"其他平均rank: {others['rank_score'].mean():.2f}")
    print()

    # 对比因子得分
    print("因子得分对比:")
    for factor in ['trend', 'risk', 'volume']:
        top3_scores = []
        others_scores = []

        for factors in top3['factors']:
            if factor in factors:
                top3_scores.append(factors[factor]['ratio'])

        for factors in others['factors']:
            if factor in factors:
                others_scores.append(factors[factor]['ratio'])

        if top3_scores and others_scores:
            print(f"  {factor.upper():10s}: Top3={np.mean(top3_scores):.1%}  其他={np.mean(others_scores):.1%}  差距={np.mean(top3_scores)-np.mean(others_scores):+.1%}")

    print()
    print("="*80)
    print()

# 总结
print("="*80)
print("💡 Rank逻辑诊断")
print("="*80)
print()

print("【当前权重推测】")
print("基于因子得分分析，当前可能的权重:")
for factor, stats in sorted(factor_stats.items(), key=lambda x: x[1]['avg_total'], reverse=True):
    print(f"  {factor.upper()}: 约 {stats['avg_total']:.0f}分")

print()
print("【问题诊断】")
print("如果Top3选出的股票表现差，可能原因:")
print("  1. TREND因子权重过高，偏好高涨幅股票（追高风险）")
print("  2. RISK因子权重过低，忽视了稳定性")
print("  3. 因子与最终收益的相关性为负（高分反而差）")

print()
print("【验证建议】")
print("需要加载signal_performance数据，对比:")
print("  • Top3的实际表现 vs 其他股票的表现")
print("  • 各因子得分与最终收益的相关性")
print("  • 排名准确率（Top3是否真的优于其他）")

print()
print("="*80)
