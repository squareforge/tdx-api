#!/usr/bin/env python3
"""
策略信号深度分析工具
分析历史信号的因子有效性、成功率、风险收益比等关键指标
"""

import pandas as pd
import numpy as np
import glob
import json
from datetime import datetime
from collections import defaultdict

def load_all_signals():
    """加载所有历史信号"""
    files = sorted(glob.glob('reports/signals_*.csv'))
    files = [f for f in files if 'top3' not in f and 'smoke' not in f]

    all_signals = []
    for f in files:
        try:
            df = pd.read_csv(f, encoding='utf-8-sig')
            all_signals.append(df)
            print(f"✓ 加载 {f}: {len(df)} 条信号")
        except Exception as e:
            print(f"✗ 加载失败 {f}: {e}")

    if not all_signals:
        return None

    combined = pd.concat(all_signals, ignore_index=True)
    print(f"\n总计: {len(combined)} 条信号, 覆盖 {combined['date'].nunique()} 个交易日")
    return combined

def load_all_performance():
    """加载所有历史表现追踪数据"""
    files = sorted(glob.glob('reports/signal_performance_*.csv'))

    all_perf = []
    for f in files:
        try:
            df = pd.read_csv(f, encoding='utf-8-sig')
            if len(df) > 0:
                all_perf.append(df)
                print(f"✓ 加载 {f}: {len(df)} 条追踪")
        except Exception as e:
            print(f"✗ 加载失败 {f}: {e}")

    if not all_perf:
        return None

    combined = pd.concat(all_perf, ignore_index=True)
    print(f"\n总计: {len(combined)} 条追踪记录")
    return combined

def parse_factor_scores(rank_reason):
    """解析因子得分字符串"""
    # 格式: "trend=21.6/25, pullback=19.1/20, volume=9.3/20, strength=11.1/15, liquidity=6.6/10, risk=8.2/10"
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
                factors[key] = {
                    'score': float(score_str),
                    'total': float(total_str),
                    'ratio': float(score_str) / float(total_str) if float(total_str) > 0 else 0
                }
            except:
                pass
    return factors

def analyze_factor_effectiveness(df):
    """分析各因子的预测有效性"""
    print("\n" + "="*80)
    print("📊 因子有效性分析")
    print("="*80)

    # 解析所有因子得分
    df['factors'] = df['rank_reason'].apply(parse_factor_scores)

    factor_names = ['trend', 'pullback', 'volume', 'strength', 'liquidity', 'risk']

    for factor in factor_names:
        # 提取该因子的得分率
        df[f'{factor}_ratio'] = df['factors'].apply(
            lambda x: x.get(factor, {}).get('ratio', np.nan)
        )

    print(f"\n各因子得分分布:")
    print("-" * 80)
    for factor in factor_names:
        col = f'{factor}_ratio'
        if col in df.columns:
            stats = df[col].describe()
            print(f"\n{factor.upper()} (满分率):")
            print(f"  均值: {stats['mean']:.2%}  中位数: {stats['50%']:.2%}")
            print(f"  最小: {stats['min']:.2%}  最大: {stats['max']:.2%}")
            print(f"  25%分位: {stats['25%']:.2%}  75%分位: {stats['75%']:.2%}")

def analyze_rank_distribution(df):
    """分析rank_score分布和top3选择效果"""
    print("\n" + "="*80)
    print("🎯 排名分数分析")
    print("="*80)

    print(f"\nrank_score统计:")
    print(f"  均值: {df['rank_score'].mean():.2f}")
    print(f"  中位数: {df['rank_score'].median():.2f}")
    print(f"  最高: {df['rank_score'].max():.2f}")
    print(f"  最低: {df['rank_score'].min():.2f}")
    print(f"  标准差: {df['rank_score'].std():.2f}")

    # 分数段分布
    bins = [0, 50, 60, 70, 80, 100]
    labels = ['0-50', '50-60', '60-70', '70-80', '80-100']
    df['score_range'] = pd.cut(df['rank_score'], bins=bins, labels=labels, include_lowest=True)

    print(f"\n分数段分布:")
    dist = df['score_range'].value_counts().sort_index()
    for score_range, count in dist.items():
        pct = count / len(df) * 100
        print(f"  {score_range}分: {count:3d} ({pct:5.1f}%)")

    # Top3分析
    top3_count = df['selected_top3'].value_counts()
    print(f"\nTop3选择:")
    print(f"  选入top3: {top3_count.get('yes', 0)} 条")
    print(f"  未入top3: {top3_count.get('no', 0)} 条")

def analyze_signal_characteristics(df):
    """分析信号特征"""
    print("\n" + "="*80)
    print("📈 信号特征分析")
    print("="*80)

    # 涨幅分布
    print(f"\n前期涨幅特征:")
    print(f"  20日平均涨幅: {df['ret20'].mean():.2%} (中位数: {df['ret20'].median():.2%})")
    print(f"  60日平均涨幅: {df['ret60'].mean():.2%} (中位数: {df['ret60'].median():.2%})")

    # 涨幅过大的信号占比
    ret20_high = (df['ret20'] > 0.5).sum()
    ret60_high = (df['ret60'] > 1.0).sum()
    print(f"\n  20日涨幅>50%: {ret20_high} ({ret20_high/len(df)*100:.1f}%)")
    print(f"  60日涨幅>100%: {ret60_high} ({ret60_high/len(df)*100:.1f}%)")

    # 价格分布
    print(f"\n价格分布:")
    print(f"  均价: {df['close'].mean():.2f}元")
    print(f"  中位价: {df['close'].median():.2f}元")

    price_ranges = {
        '0-10元': (df['close'] < 10).sum(),
        '10-20元': ((df['close'] >= 10) & (df['close'] < 20)).sum(),
        '20-50元': ((df['close'] >= 20) & (df['close'] < 50)).sum(),
        '50-100元': ((df['close'] >= 50) & (df['close'] < 100)).sum(),
        '100元+': (df['close'] >= 100).sum()
    }

    for range_name, count in price_ranges.items():
        pct = count / len(df) * 100
        print(f"  {range_name}: {count:3d} ({pct:5.1f}%)")

    # 成交量特征
    df['volume_ratio'] = df['volume'] / df['vol20']
    print(f"\n成交量特征:")
    print(f"  当日/20日均量比: {df['volume_ratio'].mean():.2f}x (中位数: {df['volume_ratio'].median():.2f}x)")
    print(f"  放量>1.5倍: {(df['volume_ratio'] > 1.5).sum()} ({(df['volume_ratio'] > 1.5).sum()/len(df)*100:.1f}%)")

def analyze_performance_tracking(perf_df):
    """分析追踪表现"""
    print("\n" + "="*80)
    print("💰 信号后续表现分析")
    print("="*80)

    # 获取最新追踪数据（每只股票最后一条记录）
    latest_perf = perf_df.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()

    print(f"\n追踪样本: {len(latest_perf)} 条信号")

    # 开盘涨跌幅分析
    print(f"\n次日开盘表现:")
    print(f"  平均开盘涨幅: {latest_perf['open_change_pct'].mean():.2%}")
    print(f"  开盘上涨: {(latest_perf['open_change_pct'] > 0).sum()} ({(latest_perf['open_change_pct'] > 0).sum()/len(latest_perf)*100:.1f}%)")
    print(f"  开盘下跌: {(latest_perf['open_change_pct'] < 0).sum()} ({(latest_perf['open_change_pct'] < 0).sum()/len(latest_perf)*100:.1f}%)")
    print(f"  开盘低开>-2%: {(latest_perf['open_change_pct'] < -0.02).sum()} ({(latest_perf['open_change_pct'] < -0.02).sum()/len(latest_perf)*100:.1f}%)")

    # 买入区间分析
    print(f"\n买入价格区间:")
    buy_status = latest_perf['buy_range_status'].value_counts()
    for status, count in buy_status.items():
        pct = count / len(latest_perf) * 100
        print(f"  {status}: {count} ({pct:.1f}%)")

    # 收益分析
    print(f"\n当前收益表现:")
    print(f"  平均收益: {latest_perf['change_pct'].mean():.2%}")
    print(f"  中位数收益: {latest_perf['change_pct'].median():.2%}")
    print(f"  盈利比例: {(latest_perf['change_pct'] > 0).sum()/len(latest_perf)*100:.1f}%")

    # 收益分布
    profit_dist = {
        '大亏(<-10%)': (latest_perf['change_pct'] < -0.1).sum(),
        '亏损(-10%~0)': ((latest_perf['change_pct'] >= -0.1) & (latest_perf['change_pct'] < 0)).sum(),
        '小赚(0~5%)': ((latest_perf['change_pct'] >= 0) & (latest_perf['change_pct'] < 0.05)).sum(),
        '中赚(5%~15%)': ((latest_perf['change_pct'] >= 0.05) & (latest_perf['change_pct'] < 0.15)).sum(),
        '大赚(≥15%)': (latest_perf['change_pct'] >= 0.15).sum()
    }

    print(f"\n收益分布:")
    for range_name, count in profit_dist.items():
        pct = count / len(latest_perf) * 100
        print(f"  {range_name}: {count:3d} ({pct:5.1f}%)")

    # 最大收益和最大回撤
    print(f"\n最大收益 vs 最大回撤:")
    print(f"  平均最大收益: {latest_perf['max_gain_pct'].mean():.2%}")
    print(f"  平均最大回撤: {latest_perf['max_drawdown_pct'].mean():.2%}")
    print(f"  最大收益>10%: {(latest_perf['max_gain_pct'] > 0.1).sum()} ({(latest_perf['max_gain_pct'] > 0.1).sum()/len(latest_perf)*100:.1f}%)")
    print(f"  最大回撤<-7%: {(latest_perf['max_drawdown_pct'] < -0.07).sum()} ({(latest_perf['max_drawdown_pct'] < -0.07).sum()/len(latest_perf)*100:.1f}%)")

def analyze_top3_vs_all(signals_df, perf_df):
    """对比Top3与全部信号的表现"""
    print("\n" + "="*80)
    print("🏆 Top3 vs 全部信号对比")
    print("="*80)

    # 合并数据
    perf_latest = perf_df.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()
    merged = pd.merge(
        perf_latest,
        signals_df[['date', 'code', 'selected_top3', 'rank_score']],
        left_on=['signal_date', 'code'],
        right_on=['date', 'code'],
        how='left'
    )

    top3 = merged[merged['selected_top3'] == 'yes']
    others = merged[merged['selected_top3'] == 'no']

    if len(top3) == 0:
        print("\n⚠️  暂无Top3信号的追踪数据")
        return

    print(f"\nTop3信号 ({len(top3)}条) vs 其他信号 ({len(others)}条):")
    print("-" * 80)

    metrics = [
        ('平均收益', 'change_pct', '{:.2%}'),
        ('中位数收益', 'change_pct', '{:.2%}', 'median'),
        ('盈利比例', 'change_pct', '{:.1%}', lambda x: (x > 0).sum() / len(x)),
        ('平均最大收益', 'max_gain_pct', '{:.2%}'),
        ('平均最大回撤', 'max_drawdown_pct', '{:.2%}'),
        ('次日平均开盘涨幅', 'open_change_pct', '{:.2%}'),
    ]

    for metric_name, col, fmt, *agg in metrics:
        if agg:
            if callable(agg[0]):
                top3_val = agg[0](top3[col])
                others_val = agg[0](others[col]) if len(others) > 0 else 0
            else:
                top3_val = getattr(top3[col], agg[0])()
                others_val = getattr(others[col], agg[0])() if len(others) > 0 else 0
        else:
            top3_val = top3[col].mean()
            others_val = others[col].mean() if len(others) > 0 else 0

        diff = top3_val - others_val
        symbol = "✓" if diff > 0 else "✗"
        print(f"  {metric_name:20s}: Top3={fmt.format(top3_val)}  其他={fmt.format(others_val)}  差距={fmt.format(diff)} {symbol}")

def analyze_factor_correlation_with_performance(signals_df, perf_df):
    """分析因子得分与实际表现的相关性"""
    print("\n" + "="*80)
    print("📉 因子得分 vs 实际收益相关性")
    print("="*80)

    # 合并数据
    perf_latest = perf_df.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()
    merged = pd.merge(
        perf_latest,
        signals_df,
        left_on=['signal_date', 'code'],
        right_on=['date', 'code'],
        how='inner'
    )

    if len(merged) == 0:
        print("\n⚠️  无法匹配信号与追踪数据")
        return

    # 解析因子
    merged['factors'] = merged['rank_reason'].apply(parse_factor_scores)
    factor_names = ['trend', 'pullback', 'volume', 'strength', 'liquidity', 'risk']

    for factor in factor_names:
        merged[f'{factor}_ratio'] = merged['factors'].apply(
            lambda x: x.get(factor, {}).get('ratio', np.nan)
        )

    print(f"\n分析样本: {len(merged)} 条")
    print("\n因子得分率 vs 收益相关系数:")
    print("-" * 80)

    for factor in factor_names:
        col = f'{factor}_ratio'
        if col in merged.columns:
            valid = merged[[col, 'change_pct']].dropna()
            if len(valid) > 1:
                corr = valid[col].corr(valid['change_pct'])
                symbol = "↑" if corr > 0 else "↓"
                strength = "强" if abs(corr) > 0.3 else "中" if abs(corr) > 0.1 else "弱"
                print(f"  {factor.upper():12s}: {corr:+.3f} {symbol} ({strength}相关)")

def generate_recommendations(signals_df, perf_df):
    """生成策略优化建议"""
    print("\n" + "="*80)
    print("💡 策略优化建议")
    print("="*80)

    recommendations = []

    # 合并数据
    perf_latest = perf_df.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()

    # 1. 检查次日低开问题
    low_open_pct = (perf_latest['open_change_pct'] < -0.02).sum() / len(perf_latest)
    if low_open_pct > 0.3:
        recommendations.append({
            'priority': '高',
            'issue': f'次日低开>-2%的比例达到{low_open_pct:.1%}',
            'suggestion': '考虑增加"前期涨幅过大过滤"或"尾盘冲高回落过滤"因子'
        })

    # 2. 检查买入区间问题
    below_range_pct = (perf_latest['buy_range_status'] == 'below_range').sum() / len(perf_latest)
    if below_range_pct > 0.5:
        recommendations.append({
            'priority': '中',
            'issue': f'{below_range_pct:.1%}的信号次日开盘低于买入区间',
            'suggestion': '调整买入价格区间计算方式，或增加"预期次日开盘价"预测模型'
        })

    # 3. 检查前期涨幅
    high_ret20 = (signals_df['ret20'] > 0.5).sum() / len(signals_df)
    if high_ret20 > 0.2:
        recommendations.append({
            'priority': '高',
            'issue': f'{high_ret20:.1%}的信号前20日涨幅>50%，可能处于高位',
            'suggestion': '增加涨幅上限过滤，例如"20日涨幅<60%"或"距高点回撤>8%"'
        })

    # 4. 检查样本量
    if len(perf_latest) < 50:
        recommendations.append({
            'priority': '高',
            'issue': f'当前仅有{len(perf_latest)}条追踪样本',
            'suggestion': '建议至少积累100-200条信号后再进行策略调优，当前数据量不足以做统计推断'
        })

    # 5. 检查盈利比例
    profit_ratio = (perf_latest['change_pct'] > 0).sum() / len(perf_latest)
    if profit_ratio < 0.4:
        recommendations.append({
            'priority': '高',
            'issue': f'盈利比例仅{profit_ratio:.1%}，低于随机水平',
            'suggestion': '策略可能需要重大调整，建议暂停实盘并重新审视选股逻辑'
        })

    if recommendations:
        print("\n发现以下问题和建议:")
        print("-" * 80)
        for i, rec in enumerate(recommendations, 1):
            print(f"\n{i}. [{rec['priority']}优先级]")
            print(f"   问题: {rec['issue']}")
            print(f"   建议: {rec['suggestion']}")
    else:
        print("\n✓ 暂未发现明显问题，继续积累数据观察")

def main():
    print("="*80)
    print("TDX Workbench - 策略信号深度分析工具")
    print("="*80)

    # 1. 加载信号数据
    print("\n[1/7] 加载信号数据...")
    signals_df = load_all_signals()
    if signals_df is None:
        print("❌ 无信号数据")
        return

    # 2. 加载追踪数据
    print("\n[2/7] 加载追踪数据...")
    perf_df = load_all_performance()
    if perf_df is None:
        print("❌ 无追踪数据")
        return

    # 3. 分析因子有效性
    print("\n[3/7] 分析因子有效性...")
    analyze_factor_effectiveness(signals_df)

    # 4. 分析排名分数分布
    print("\n[4/7] 分析排名分数...")
    analyze_rank_distribution(signals_df)

    # 5. 分析信号特征
    print("\n[5/7] 分析信号特征...")
    analyze_signal_characteristics(signals_df)

    # 6. 分析追踪表现
    print("\n[6/7] 分析信号后续表现...")
    analyze_performance_tracking(perf_df)

    # 7. Top3对比
    print("\n[7/7] Top3对比分析...")
    analyze_top3_vs_all(signals_df, perf_df)

    # 8. 因子相关性
    analyze_factor_correlation_with_performance(signals_df, perf_df)

    # 9. 生成建议
    generate_recommendations(signals_df, perf_df)

    print("\n" + "="*80)
    print("✓ 分析完成")
    print("="*80)

if __name__ == '__main__':
    main()
