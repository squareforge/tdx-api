export interface RiskKline {
  Time?: string;
  High?: number;
  Low?: number;
  Close?: number;
}

export interface TradeRiskLevels {
  stopPrice: number;
  targetPrice1: number;
  targetPrice2: number;
  riskAsOf: string;
  riskBasis: string;
}

export function riskTargets(entryPrice: number, stopPrice: number): { targetPrice1: number; targetPrice2: number } {
  if (!Number.isFinite(entryPrice + stopPrice) || stopPrice <= 0 || stopPrice >= entryPrice) {
    return { targetPrice1: 0, targetPrice2: 0 };
  }
  const risk = entryPrice - stopPrice;
  return {
    targetPrice1: Math.ceil((entryPrice + 1.5 * risk - 1e-9) * 100) / 100,
    targetPrice2: Math.ceil((entryPrice + 2.5 * risk - 1e-9) * 100) / 100
  };
}

export function shanghaiDate(now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit'
  }).format(now);
}

// API Kline prices are integer thousandths of a yuan. Every input bar must be
// completed before entry; the expected session comes from the trading calendar.
export function calculateTradeRisk(
  rows: RiskKline[], entryPrice: number, entryDate: string, previousTradingDate: string
): { levels?: TradeRiskLevels; reason: string } {
  if (!Number.isFinite(entryPrice) || entryPrice <= 0) return { reason: '填写买入价后计算止盈止损' };
  if (!/^\d{4}-\d{2}-\d{2}$/.test(entryDate)) return { reason: '请填写买入日期' };
  if (!previousTradingDate || previousTradingDate >= entryDate) return { reason: '无法确认买入前一交易日，请更新交易日历后重算' };
  const unique = new Map<string, RiskKline>();
  for (const row of rows) {
    const date = String(row.Time || '').slice(0, 10);
    if (/^\d{4}-\d{2}-\d{2}$/.test(date) && date < entryDate) unique.set(date, row);
  }
  const completed = [...unique.entries()].sort(([a], [b]) => a.localeCompare(b)).slice(-120);
  const latest = completed.at(-1)?.[0];
  if (latest !== previousTradingDate) return { reason: `日K仅更新至 ${latest || '未知'}，需要 ${previousTradingDate} 的数据；暂不自动生成价格` };
  if (completed.length < 15) return { reason: '买入前完整日K不足 15 根，暂不自动生成价格' };
  const points = completed.map(([, row]) => ({ high: Number(row.High) / 1000, low: Number(row.Low) / 1000, close: Number(row.Close) / 1000 }));
  if (points.some((p) => !Number.isFinite(p.high + p.low + p.close) || p.low <= 0 || p.high < p.low || p.close < p.low || p.close > p.high)) {
    return { reason: '历史日K价格异常，请更新数据后重算' };
  }
  const ranges = points.slice(1).map((p, i) => Math.max(p.high - p.low, Math.abs(p.high - points[i].close), Math.abs(p.low - points[i].close)));
  // Wilder ATR(14), seeded with the first 14 true ranges.
  let atr = ranges.slice(0, 14).reduce((sum, value) => sum + value, 0) / 14;
  for (const range of ranges.slice(14)) atr = (atr * 13 + range) / 14;
  if (atr <= 0) return { reason: '历史波动不足，无法生成有效止盈止损价' };

  // The most recent confirmed swing low uses two already completed bars on each
  // side. Distant lows are excluded so a stale structure cannot widen risk.
  let support = 0;
  let supportName = '';
  for (let i = Math.max(2, points.length - 22); i < points.length - 2; i++) {
    const p = points[i];
    const nearby = p.low < entryPrice && entryPrice - p.low <= 3 * atr;
    if (nearby && p.low <= points[i - 1].low && p.low <= points[i - 2].low &&
        p.low < points[i + 1].low && p.low < points[i + 2].low) {
      support = p.low;
      supportName = '已确认波段低点';
    }
  }
  if (!support) {
    const recentLow = Math.min(...points.slice(-5).map((p) => p.low));
    if (recentLow < entryPrice && entryPrice - recentLow <= 3 * atr) {
      support = recentLow;
      supportName = '近5日低点';
    }
  }
  const candidate = Math.min(entryPrice - 2 * atr, support ? support - 0.2 * atr : entryPrice - 2 * atr);
  const stopPrice = Math.floor((candidate + 1e-9) * 100) / 100;
  if (stopPrice <= 0 || stopPrice >= entryPrice) return { reason: '波动相对买入价过大，无法生成有效止损价' };
  const risk = entryPrice - stopPrice;
  const { targetPrice1, targetPrice2 } = riskTargets(entryPrice, stopPrice);
  const pressure = points.slice(-60).map((p) => p.high).filter((high) => high > entryPrice).sort((a, b) => a - b)[0];
  const pressureNote = pressure && pressure < targetPrice1 ? `；上方参考高点 ${pressure.toFixed(2)} 早于第一止盈价，需留意阻力` : '';
  const riskBasis = `截至 ${latest} · Wilder ATR(14)=${atr.toFixed(3)}；止损取买入价−2ATR${support ? `与${supportName} ${support.toFixed(2)}−0.2ATR 中较低值` : ''}；第一/第二止盈为1.5R/2.5R；每股风险 ${risk.toFixed(2)}（${(risk / entryPrice * 100).toFixed(2)}%）${pressureNote}。倍数为初始规则，尚未经过本策略回测校准。`;
  return { levels: { stopPrice, targetPrice1, targetPrice2, riskAsOf: latest!, riskBasis }, reason: riskBasis };
}

export function allowsTradeAlerts(hook: { events: string; enabled: boolean }): boolean {
  if (!hook.enabled) return false;
  try {
    const events = JSON.parse(hook.events || '[]');
    return Array.isArray(events) && (!events.length || events.includes('*') || events.includes('trade.alert'));
  } catch { return false; }
}
