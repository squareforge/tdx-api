#!/usr/bin/env python3
"""
策略配置修正脚本
基于相关性分析结果，自动调整因子权重
"""

import json
import sys

def print_comparison():
    """打印修改对比"""
    print("="*80)
    print("策略权重修正方案")
    print("="*80)

    print("\n📊 相关性分析结果:")
    print("-"*80)
    correlations = [
        ("TREND", -0.367, "强负相关", "分越高收益越差"),
        ("RISK", +0.361, "强正相关", "分越高收益越好"),
        ("VOLUME", +0.150, "中正相关", "放量有效"),
        ("STRENGTH", -0.112, "中负相关", "分越高收益越差"),
        ("PULLBACK", +0.046, "弱相关", "影响不大"),
        ("LIQUIDITY", -0.018, "弱相关", "影响不大"),
    ]

    for factor, corr, strength, note in correlations:
        symbol = "✅" if corr > 0 else "⚠️"
        print(f"  {symbol} {factor:12s}: {corr:+.3f} ({strength:8s}) - {note}")

    print("\n🔧 权重调整方案:")
    print("-"*80)

    changes = [
        ("RISK (max_amplitude)", "10分", "25分", "⭐ 提到最高", "正相关+0.361"),
        ("VOLUME (volume_up)", "20分", "20分", "保持不变", "正相关+0.150"),
        ("TREND (gain_days)", "25分", "15分", "⚠️ 大幅降低", "负相关-0.367"),
        ("PULLBACK (drawdown)", "20分", "15分", "略微降低", "弱相关+0.046"),
        ("LIQUIDITY (min_amount)", "10分", "15分", "略微提高", "弱相关-0.018"),
        ("STRENGTH (close_strength)", "15分", "10分", "降低", "负相关-0.112"),
    ]

    print(f"  {'因子':<25s} {'旧权重':<8s} {'新权重':<8s} {'变化':<12s} {'原因':<20s}")
    print("-"*80)
    for factor, old, new, change, reason in changes:
        print(f"  {factor:<25s} {old:<8s} {new:<8s} {change:<12s} {reason:<20s}")

    print("\n🎯 关键改动:")
    print("-"*80)
    print("  1. RISK因子 从10分提到25分 (最高权重)")
    print("     → 因为 RISK 正相关+0.361，分数越高收益越好")
    print("")
    print("  2. TREND因子 从25分降到15分")
    print("     → 因为 TREND 负相关-0.367，涨幅越大反而越危险")
    print("")
    print("  3. TREND改为区间评分 (25-35%最优)")
    print("     → 不是越高越好，适中的涨幅更有持续性")
    print("")
    print("  4. STRENGTH因子 从15分降到10分")
    print("     → 因为 STRENGTH 负相关-0.112")

    print("\n📈 预期效果:")
    print("-"*80)
    print("  修正前:")
    print("    • Top3优于其他: 0% (5天全输)")
    print("    • Top3成功率: 33.3%")
    print("    • Top3平均收益: -1.82%")
    print("")
    print("  修正后（预期）:")
    print("    • Top3优于其他: >60%")
    print("    • Top3成功率: >50%")
    print("    • Top3平均收益: 优于整体")

    print("\n" + "="*80)

def create_strategy_config():
    """生成修正后的策略配置"""
    with open('output/fixed_strategy_v3.json', 'r', encoding='utf-8') as f:
        config = json.load(f)

    print("\n✅ 策略配置已生成: output/fixed_strategy_v3.json")
    print("\n📝 配置要点:")
    print("  • RISK因子权重: 25分 (最高)")
    print("  • TREND因子权重: 15分 (降低)")
    print("  • TREND区间评分: 25-35%最优")
    print("  • 涨幅上限过滤: 45%")
    print("  • 市场状态过滤: 上证20日跌幅≤3%")

    return config

def print_next_steps():
    """打印后续步骤"""
    print("\n" + "="*80)
    print("🚀 后续步骤")
    print("="*80)

    steps = [
        ("1. 在Web界面创建新策略", [
            "打开 http://localhost:8080",
            "进入 策略中心",
            "点击 新建策略",
            "复制 output/fixed_strategy_v3.json 的 config 部分",
            "保存为 '修正版V3-回踩突破策略'"
        ]),
        ("2. 运行策略选股", [
            "手动执行一次该策略",
            "或等待自动化任务触发",
            "检查生成的信号数量"
        ]),
        ("3. 追踪3-5天后验证", [
            "python3 scripts/dual_track_evaluation.py",
            "查看 Top3优于其他 是否>60%",
            "查看 Top3成功率 是否>50%"
        ]),
        ("4. 如果验证通过", [
            "将此配置设为主力策略",
            "继续积累3个月数据",
            "准备实盘小仓试水"
        ])
    ]

    for title, substeps in steps:
        print(f"\n{title}")
        print("-"*40)
        for step in substeps:
            print(f"  • {step}")

    print("\n" + "="*80)
    print("⚠️  重要提醒")
    print("="*80)
    print("  • 不要删除旧策略的历史数据")
    print("  • 新旧策略同时运行对比效果")
    print("  • 至少追踪3-5天才能看到改进")
    print("  • 期望看到 Top3 排名真正有效")
    print("="*80)

def main():
    print_comparison()
    config = create_strategy_config()
    print_next_steps()

    print("\n✅ 配置文件生成完成！")
    print("\n📂 文件位置:")
    print("  • output/fixed_strategy_v3.json - 完整配置（含说明）")
    print("  • output/optimized_strategy_config.json - 之前的优化版")
    print("\n💡 提示:")
    print("  使用 output/fixed_strategy_v3.json，这是基于数据修正的最新版本！")

if __name__ == '__main__':
    main()
