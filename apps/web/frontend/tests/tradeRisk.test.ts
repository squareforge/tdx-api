import assert from 'node:assert/strict';
import test from 'node:test';
import { calculateTradeRisk, riskTargets, shanghaiDate } from '../src/lib/tradeRisk.ts';

function bars() {
  const rows = [];
  const date = new Date('2026-09-25T00:00:00Z');
  for (let i = 0; i < 15; i++) {
    rows.push({ Time: date.toISOString(), High: 10200, Low: 9800, Close: 10000 });
    date.setUTCDate(date.getUTCDate() + 1);
  }
  return rows;
}

test('ATR prices use thousandths, rounded stops and risk multiples', () => {
  const result = calculateTradeRisk(bars(), 10, '2026-10-10', '2026-10-09');
  assert.ok(result.levels);
  assert.equal(result.levels.stopPrice, 9.2);
  assert.equal(result.levels.targetPrice1, 11.2);
  assert.equal(result.levels.targetPrice2, 12);
  assert.equal(result.levels.riskAsOf, '2026-10-09');
});

test('entry day and future bars cannot change initial prices', () => {
  const rows = bars();
  const before = calculateTradeRisk(rows, 10, '2026-10-10', '2026-10-09');
  const after = calculateTradeRisk([...rows, { Time: '2026-10-10', High: 99000, Low: 1, Close: 80000 }], 10, '2026-10-10', '2026-10-09');
  assert.deepEqual(before, after);
});

test('changing entry price changes prices and missing sessions block calculation', () => {
  const rows = bars();
  const one = calculateTradeRisk(rows, 10, '2026-10-10', '2026-10-09');
  const two = calculateTradeRisk(rows, 11, '2026-10-10', '2026-10-09');
  assert.notEqual(one.levels?.stopPrice, two.levels?.stopPrice);
  assert.equal(calculateTradeRisk(rows.slice(0, -1), 10, '2026-10-10', '2026-10-09').levels, undefined);
  assert.equal(calculateTradeRisk(rows, 10, '2026-10-10', '').levels, undefined);
});

test('holiday freshness uses the supplied exchange calendar', () => {
  const rows = bars().map((row, i) => ({ ...row, Time: `2026-09-${String(i + 16).padStart(2, '0')}` }));
  const result = calculateTradeRisk(rows, 10, '2026-10-08', '2026-09-30');
  assert.ok(result.levels);
});

test('duplicate, unsorted bars do not double count ATR and invalid bars are rejected', () => {
  const rows = bars();
  assert.deepEqual(calculateTradeRisk([...rows].reverse().concat(rows[0]), 10, '2026-10-10', '2026-10-09'), calculateTradeRisk(rows, 10, '2026-10-10', '2026-10-09'));
  rows[2].Low = -1;
  assert.equal(calculateTradeRisk(rows, 10, '2026-10-10', '2026-10-09').levels, undefined);
});

test('entry date defaults to Shanghai even when the host is on the previous day', () => {
  assert.equal(shanghaiDate(new Date('2026-10-21T20:00:00Z')), '2026-10-22');
});

test('a manual stop reprices targets with the new risk and rejects invalid stops', () => {
  assert.deepEqual(riskTargets(12.9, 12.5), { targetPrice1: 13.5, targetPrice2: 13.9 });
  assert.deepEqual(riskTargets(12.9, 0), { targetPrice1: 0, targetPrice2: 0 });
  assert.deepEqual(riskTargets(12.9, 13), { targetPrice1: 0, targetPrice2: 0 });
});
