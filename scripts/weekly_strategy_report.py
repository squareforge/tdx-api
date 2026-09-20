#!/usr/bin/env python3
"""
每周策略复盘报告生成工具
分析一周内的信号表现，生成优化建议
"""

import pandas as pd
import numpy as np
import glob
from datetime import datetime, timedelta
import json

def load_signals_last_n_days(days=7):
    """加载最近N天的信号"""
    files = sorted(glob.glob('reports/signals_*.csv'))
    if len(files) > days:
        files = files[-days:]

    all_signals = []
    for f in files:
        df = pd.read_csv(f, encoding='utf-8-sig')
        all_signals.append(df)

    if not all_signals:
        return None

    return pd.concat(all_signals, ignore_index=True)

def load_performance_last_n_days(days=7):
    """加载最近N天的追踪数据"""
    files = sorted(glob.glob('reports/signal_performance_*.csv'))
    if len(files) > days:
        files = files[-days:]

    all_perf = []
    for f in files:
        df = pd.read_csv(f, encoding='utf-8-sig')
        if len(df) > 0:
            all_perf.append(df)

    if not all_perf:
        return None

    return pd.concat(all_perf, ignore_index=True)

def parse_factor_scores(rank_reason):
    """解析因子得分"""
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

def generate_weekly_report():
    """生成周报"""
    print("="*80)
    print(f"TDX Workbench - 策略周报")
    print(f"生成时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("="*80)

    # 加载数据
    signals = load_signals_last_n_days(7)
    perf = load_performance_last_n_days(7)

    if signals is None:
        print("\n❌ 无信号数据")
        return

    if perf is None:
        print("\n❌ 无追踪数据")
        return

    # 基础统计
    print(f"\n📊 本周概况")
    print(f"  信号数量: {len(signals)} 条")
    print(f"  覆盖股票: {signals['code'].nunique()} 只")
    print(f"  交易日数: {signals['date'].nunique()} 天")
    print(f"  追踪记录: {len(perf)} 条")

    # Top3信号
    top3 = signals[signals['selected_top3'] == 'yes']
    print(f"  Top3信号: {len(top3)} 条")

    # 解析因子
    signals['factors'] = signals['rank_reason'].apply(parse_factor_scores)
    factor_names = ['trend', 'pullback', 'volume', 'strength', 'liquidity', 'risk']

    for factor in factor_names:
        signals[f'{factor}_ratio'] = signals['factors'].apply(
            lambda x: x.get(factor, {}).get('ratio', np.nan)
        )

    # 因子分布
    print(f"\n📈 因子得分概况")
    for factor in factor_names:
        col = f'{factor}_ratio'
        mean_val = signals[col].mean()
        print(f"  {factor.upper():12s}: {mean_val:.1%}")

    # 信号特征
    print(f"\n💡 信号特征")
    print(f"  平均rank_score: {signals['rank_score'].mean():.2f}")
    print(f"  平均20日涨幅: {signals['ret20'].mean():.1%}")
    print(f"  平均60日涨幅: {signals['ret60'].mean():.1%}")
    print(f"  平均价格: {signals['close'].mean():.2f}元")

    # 追踪表现
    perf_latest = perf.sort_values('date').groupby(['signal_date', 'code']).last().reset_index()

    if len(perf_latest) > 0:
        print(f"\n💰 追踪表现 (样本数: {len(perf_latest)})")
        print(f"  平均收益: {perf_latest['change_pct'].mean():.2%}")
        print(f"  中位收益: {perf_latest['change_pct'].median():.2%}")
        print(f"  盈利比例: {(perf_latest['change_pct'] > 0).sum() / len(perf_latest):.1%}")
        print(f"  平均最大回撤: {perf_latest['max_drawdown_pct'].mean():.2%}")
        print(f"  触发止损(-7%)比例: {(perf_latest['max_drawdown_pct'] < -0.07).sum() / len(perf_latest):.1%}")

        # 收益分段
        print(f"\n  收益分布:")
        ranges = [
            ("大亏(<-10%)", lambda x: x < -0.1),
            ("小亏(-10%~-3%)", lambda x: (x >= -0.1) & (x < -0.03)),
            ("微亏(-3%~0)", lambda x: (x >= -0.03) & (x < 0)),
            ("小赚(0~5%)", lambda x: (x >= 0) & (x < 0.05)),
            ("中赚(5%~10%)", lambda x: (x >= 0.05) & (x < 0.1)),
            ("大赚(≥10%)", lambda x: x >= 0.1)
        ]

        for name, condition in ranges:
            count = condition(perf_latest['change_pct']).sum()
            pct = count / len(perf_latest) * 100
            print(f"    {name:15s}: {count:3d} ({pct:5.1f}%)")

        # 最佳和最差
        if len(perf_latest) > 0:
            print(f"\n🏆 本周最佳信号 (Top 5):")
            best = perf_latest.nlargest(5, 'change_pct')[['code', 'name', 'change_pct', 'max_drawdown_pct']]
            for idx, row in best.iterrows():
                print(f"  {row['code']} {row['name']:8s} | 收益:{row['change_pct']:+.1%} | 回撤:{row['max_drawdown_pct']:+.1%}")

            print(f"\n⚠️  本周最差信号 (Bottom 5):")
            worst = perf_latest.nsmallest(5, 'change_pct')[['code', 'name', 'change_pct', 'max_drawdown_pct']]
            for idx, row in worst.iterrows():
                print(f"  {row['code']} {row['name']:8s} | 收益:{row['change_pct']:+.1%} | 回撤:{row['max_drawdown_pct']:+.1%}")

    # 涨幅分段分析
    print(f"\n📊 按前期涨幅分段分析")
    signals['ret20_range'] = pd.cut(
        signals['ret20'],
        bins=[0, 0.3, 0.5, 0.8, 2.0],
        labels=['0-30%', '30-50%', '50-80%', '80%+']
    )

    for range_name in ['0-30%', '30-50%', '50-80%', '80%+']:
        count = (signals['ret20_range'] == range_name).sum()
        pct = count / len(signals) * 100
        print(f"  {range_name:10s}: {count:3d} ({pct:5.1f}%)")

    # 告警
    print(f"\n⚠️  风险提示")
    alerts = []

    if len(perf_latest) > 0:
        win_rate = (perf_latest['change_pct'] > 0).sum() / len(perf_latest)
        if win_rate < 0.4:
            alerts.append(f"盈利率仅{win_rate:.1%}，低于预期")

        avg_return = perf_latest['change_pct'].mean()
        if avg_return < -0.05:
            alerts.append(f"平均收益{avg_return:.1%}，显著为负")

        stop_loss_rate = (perf_latest['max_drawdown_pct'] < -0.07).sum() / len(perf_latest)
        if stop_loss_rate > 0.5:
            alerts.append(f"止损触发率{stop_loss_rate:.1%}，风险过高")

    high_gain_rate = (signals['ret20'] > 0.5).sum() / len(signals)
    if high_gain_rate > 0.3:
        alerts.append(f"{high_gain_rate:.1%}的信号20日涨幅>50%，追高风险")

    if len(alerts) > 0:
        for i, alert in enumerate(alerts, 1):
            print(f"  {i}. {alert}")
    else:
        print(f"  ✓ 暂无明显风险")

    # 建议
    print(f"\n💡 本周建议")

    if len(perf_latest) < 50:
        print(f"  • 当前仅{len(perf_latest)}条样本，继续积累数据")

    if len(perf_latest) > 0 and avg_return < 0:
        print(f"  • 考虑暂停信号执行，等待市场环境改善")

    if high_gain_rate > 0.3:
        print(f"  • 建议增加前期涨幅上限过滤（如<45%）")

    if len(perf_latest) > 0 and stop_loss_rate > 0.5:
        print(f"  • 风控失效，建议重新审视选股逻辑")

    print(f"\n" + "="*80)
    print(f"✓ 周报生成完成")
    print("="*80)

if __name__ == '__main__':
    generate_weekly_report()
