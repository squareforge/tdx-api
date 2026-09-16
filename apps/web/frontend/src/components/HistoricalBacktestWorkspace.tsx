import {
  Alert, Button, Card, DatePicker, Empty, Input, InputNumber, Progress, Segmented, Select, Space, Table, Tag, Typography, message
} from 'antd';
import {
  BarChartOutlined, ClockCircleOutlined, ExperimentOutlined, PlayCircleOutlined, ReloadOutlined, StopOutlined, SwapOutlined, TeamOutlined
} from '@ant-design/icons';
import dayjs, { type Dayjs } from 'dayjs';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiFetch } from '../lib/api';
import { normalizeSymbol } from '../lib/format';
import type {
  HistoricalBacktestResult, HistoricalBacktestRun, HistoricalBacktestSignal, HistoricalBacktestSignalPage,
  HistoricalBacktestTrade, HistoricalBacktestTradePage,
  HistoricalConsensusSummary, HistoricalHorizonSummary, HistoricalStrategySummary, SelectionTracking, Strategy
} from '../types';
import { StockQuoteModal, type StockQuoteTarget } from './StockQuoteModal';

const { Text } = Typography;
type ResultView = 'ranking' | 'signals' | 'consensus' | 'trades';
interface StockDirectoryItem { code?: string; name?: string; }
interface StockDirectory { codes?: StockDirectoryItem[]; }

function parseJSON<T>(value?: string, fallback?: T): T {
  try { return JSON.parse(value || '') as T; } catch { return fallback as T; }
}

function statusMeta(status?: string) {
  if (status === 'success') return { label: '已完成', color: 'success' } as const;
  if (status === 'failed') return { label: '失败', color: 'error' } as const;
  if (status === 'cancelled') return { label: '已取消', color: 'default' } as const;
  return { label: '执行中', color: 'processing' } as const;
}

function pct(value?: number) {
  if (!Number.isFinite(value)) return '--';
  const number = Number(value);
  return `${number >= 0 ? '+' : ''}${number.toFixed(2)}%`;
}

function formatMoney(value?: number) {
  return Number(value || 0).toLocaleString('zh-CN', { maximumFractionDigits: 0 });
}

function horizonMetric(raw: string | undefined, horizon: number) {
  return parseJSON<SelectionTracking>(raw, { horizons: {} }).horizons?.[`d${horizon}`];
}

function horizonCell(raw: string | undefined, horizon: number) {
  const value = horizonMetric(raw, horizon);
  if (!value || value.status !== 'complete') return <Tag>待观察</Tag>;
  return <Tag color={value.success ? 'success' : 'error'}>{pct(value.close_return)}</Tag>;
}

function compareNullableNumber(left?: number, right?: number) {
  const leftValid = Number.isFinite(left);
  const rightValid = Number.isFinite(right);
  if (!leftValid && !rightValid) return 0;
  if (!leftValid) return 1;
  if (!rightValid) return -1;
  return Number(left) - Number(right);
}

function horizonReturn(raw: string | undefined, horizon: number) {
  const value = horizonMetric(raw, horizon);
  return value?.status === 'complete' && Number.isFinite(value.close_return) ? value.close_return : undefined;
}

export function HistoricalBacktestWorkspace() {
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [runs, setRuns] = useState<HistoricalBacktestRun[]>([]);
  const [selectedRun, setSelectedRun] = useState<HistoricalBacktestRun | null>(null);
  const [startDate, setStartDate] = useState<Dayjs | null>(dayjs().startOf('year'));
  const [endDate, setEndDate] = useState<Dayjs | null>(dayjs());
  const [strategyIDs, setStrategyIDs] = useState<string[]>([]);
  const [targetReturn, setTargetReturn] = useState(3);
  const [drawdownLimit, setDrawdownLimit] = useState(5);
  const [initialCash, setInitialCash] = useState(100000);
  const [maxPositions, setMaxPositions] = useState(5);
  const [buyBase, setBuyBase] = useState<'open' | 'close'>('open');
  const [buyOffset, setBuyOffset] = useState(0);
  const [sellBase, setSellBase] = useState<'next_open' | 'same_close'>('next_open');
  const [takeProfit, setTakeProfit] = useState(10);
  const [trailDrawdown, setTrailDrawdown] = useState(5);
  const [stopLoss, setStopLoss] = useState(3);
  const [maxHoldDays, setMaxHoldDays] = useState(15);
  const [view, setView] = useState<ResultView>('ranking');
  const [signals, setSignals] = useState<HistoricalBacktestSignalPage>({ items: [], total: 0, limit: 100, offset: 0 });
  const [trades, setTrades] = useState<HistoricalBacktestTradePage>({ items: [], total: 0, limit: 100, offset: 0 });
  const [tradePage, setTradePage] = useState(1);
  const [signalStrategy, setSignalStrategy] = useState('all');
  const [signalKeyword, setSignalKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [starting, setStarting] = useState(false);
  const [stockNames, setStockNames] = useState<Record<string, string>>({});
  const [quoteTarget, setQuoteTarget] = useState<StockQuoteTarget | null>(null);
  const initializedStrategies = useRef(false);

  const loadRuns = useCallback(async (preferID?: string) => {
    const response = await apiFetch<{ items: HistoricalBacktestRun[] }>('/api/historical-backtests?limit=30');
    const items = response.items || [];
    setRuns(items);
    setSelectedRun((current) => {
      const targetID = preferID || current?.id;
      return (targetID ? items.find((item) => item.id === targetID) : null) || items[0] || null;
    });
    return items;
  }, []);

  const loadInitial = useCallback(async () => {
    setLoading(true);
    try {
      const [strategyList, directory] = await Promise.all([
        apiFetch<Strategy[]>('/api/strategies'),
        apiFetch<StockDirectory>('/api/codes').catch(() => ({ codes: [] }))
      ]);
      const available = (strategyList || []).filter((item) => item.enabled);
      setStrategies(available);
      if (!initializedStrategies.current) {
        const systemIDs = available.filter((item) => item.readonly).map((item) => item.id);
        setStrategyIDs(systemIDs.length ? systemIDs : available.map((item) => item.id));
        initializedStrategies.current = true;
      }
      setStockNames(Object.fromEntries((directory.codes || []).flatMap((item) => {
        const code = normalizeSymbol(item.code);
        return code && item.name?.trim() ? [[code, item.name.trim()]] : [];
      })));
      await loadRuns();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '历史回测数据加载失败');
    } finally {
      setLoading(false);
    }
  }, [loadRuns]);

  useEffect(() => { void loadInitial(); }, [loadInitial]);

  useEffect(() => {
    if (!runs.some((item) => item.status === 'running')) return;
    const timer = window.setInterval(() => { void loadRuns(); }, 2500);
    return () => window.clearInterval(timer);
  }, [loadRuns, runs]);

  const loadSignals = useCallback(async (run: HistoricalBacktestRun, nextPage = page) => {
    const params = new URLSearchParams({ limit: '100', offset: String((nextPage - 1) * 100) });
    if (signalStrategy !== 'all') params.set('strategy_id', signalStrategy);
    if (signalKeyword.trim()) params.set('symbol', normalizeSymbol(signalKeyword));
    try {
      setSignals(await apiFetch<HistoricalBacktestSignalPage>(`/api/historical-backtests/${run.id}/signals?${params}`));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '历史信号加载失败');
    }
  }, [page, signalKeyword, signalStrategy]);

  useEffect(() => {
    if (selectedRun && view === 'signals') void loadSignals(selectedRun);
  }, [loadSignals, selectedRun, view]);

  const loadTrades = useCallback(async (run: HistoricalBacktestRun, nextPage = tradePage) => {
    const params = new URLSearchParams({ limit: '100', offset: String((nextPage - 1) * 100) });
    try {
      setTrades(await apiFetch<HistoricalBacktestTradePage>(`/api/historical-backtests/${run.id}/trades?${params}`));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '交易记录加载失败');
    }
  }, [tradePage]);

  useEffect(() => {
    if (selectedRun && view === 'trades') void loadTrades(selectedRun);
  }, [loadTrades, selectedRun, view]);

  async function startBacktest() {
    if (!startDate || !endDate) return message.warning('请选择开始和结束日期');
    if (!strategyIDs.length) return message.warning('至少选择一个策略');
    setStarting(true);
    try {
      const run = await apiFetch<HistoricalBacktestRun>('/api/historical-backtests', {
        method: 'POST',
        body: JSON.stringify({
          start_date: startDate.format('YYYY-MM-DD'), end_date: endDate.format('YYYY-MM-DD'), strategy_ids: strategyIDs,
          horizons: [3, 5, 10], target_return: targetReturn, drawdown_limit: drawdownLimit,
          initial_cash: initialCash, max_positions: maxPositions,
          buy_base: buyBase, buy_offset: buyOffset, sell_base: sellBase,
          take_profit: takeProfit, trail_drawdown: trailDrawdown, stop_loss: stopLoss, max_hold_days: maxHoldDays
        })
      });
      await loadRuns(run.id);
      setView('ranking');
      message.success('历史回测已开始');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '历史回测启动失败');
    } finally {
      setStarting(false);
    }
  }

  async function cancelRun() {
    if (!selectedRun) return;
    try {
      await apiFetch(`/api/historical-backtests/${selectedRun.id}/cancel`, { method: 'POST' });
      await loadRuns(selectedRun.id);
      message.info('已提交取消请求');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '取消失败');
    }
  }

  function stockName(symbol: string) { return stockNames[normalizeSymbol(symbol)] || ''; }
  function openQuote(symbol: string) { setQuoteTarget({ code: symbol, name: stockName(symbol) || undefined }); }

  const result = useMemo(() => parseJSON<HistoricalBacktestResult>(selectedRun?.result_json, {}), [selectedRun?.result_json]);
  const summaries = result.strategy_summaries || [];
  const consensus = result.consensus || [];
  const portfolio = result.portfolio;
  const progress = selectedRun?.total_dates ? Math.round(selectedRun.processed_dates * 100 / selectedRun.total_dates) : 0;
  const runStrategyOptions = strategies.filter((item) => selectedRun?.strategy_ids.includes(item.id)).map((item) => ({ value: item.id, label: item.name }));

  const rankingColumns = [
    { title: '策略', dataIndex: 'strategy_name', width: 190, render: (value: string) => <strong>{value}</strong> },
    { title: '信号', dataIndex: 'signal_count', width: 82 },
    ...[3, 5, 10].flatMap((days) => [
      { title: `D${days}成功率`, key: `d${days}-rate`, width: 105, sorter: (a: HistoricalStrategySummary, b: HistoricalStrategySummary) => (a.horizons[`d${days}`]?.success_rate || 0) - (b.horizons[`d${days}`]?.success_rate || 0), render: (_: unknown, row: HistoricalStrategySummary) => <strong>{(row.horizons[`d${days}`]?.success_rate || 0).toFixed(1)}%</strong> },
      { title: `D${days}均收`, key: `d${days}-return`, width: 95, render: (_: unknown, row: HistoricalStrategySummary) => pct(row.horizons[`d${days}`]?.average_return) }
    ]),
    { title: 'D10回撤', key: 'drawdown', width: 100, render: (_: unknown, row: HistoricalStrategySummary) => pct(row.horizons.d10?.average_max_drawdown) }
  ];

  const signalColumns = [
    { title: '信号日', dataIndex: 'signal_date', width: 108 },
    { title: '代码', dataIndex: 'symbol', width: 92, render: (value: string) => <Button type="link" className="code-link" onClick={() => openQuote(value)}>{value}</Button> },
    { title: '名称', dataIndex: 'symbol', width: 118, render: (value: string) => <strong>{stockName(value) || '--'}</strong> },
    { title: '策略', dataIndex: 'strategy_name', width: 170 },
    { title: '信号价', dataIndex: 'latest', width: 88, render: (value: number) => value?.toFixed(2) || '--' },
    { title: '得分', dataIndex: 'score', width: 72 },
    ...[3, 5, 10].map((days) => ({ title: `D${days}`, key: `d${days}`, width: 92, render: (_: unknown, row: HistoricalBacktestSignal) => horizonCell(row.tracking_json, days) }))
  ];

  const consensusColumns = [
    { title: '信号日', dataIndex: 'signal_date', width: 108, sorter: (left: HistoricalConsensusSummary, right: HistoricalConsensusSummary) => left.signal_date.localeCompare(right.signal_date) },
    { title: '代码', dataIndex: 'symbol', width: 92, render: (value: string) => <Button type="link" className="code-link" onClick={() => openQuote(value)}>{value}</Button> },
    { title: '名称', dataIndex: 'symbol', width: 118, render: (value: string) => <strong>{stockName(value) || '--'}</strong> },
    { title: '共振', dataIndex: 'strategy_count', width: 78, sorter: (left: HistoricalConsensusSummary, right: HistoricalConsensusSummary) => left.strategy_count - right.strategy_count, render: (value: number) => <Tag color="blue">{value} 策略</Tag> },
    { title: '命中策略', dataIndex: 'strategies', width: 310, render: (value: string[]) => value.join(' · ') },
    ...[3, 5, 10].map((days) => ({
      title: `D${days}`,
      key: `d${days}`,
      width: 92,
      sorter: (left: HistoricalConsensusSummary, right: HistoricalConsensusSummary) => compareNullableNumber(horizonReturn(left.tracking_json, days), horizonReturn(right.tracking_json, days)),
      render: (_: unknown, row: HistoricalConsensusSummary) => horizonCell(row.tracking_json, days)
    }))
  ];

  const tradeColumns = [
    {
      title: '状态', dataIndex: 'status', width: 92,
      render: (value: string) => value === 'skipped' ? <Tag color="warning">已跳过</Tag> : value === 'open' ? <Tag color="blue">持仓中</Tag> : <Tag color="success">已平仓</Tag>
    },
    { title: '代码', dataIndex: 'symbol', width: 92, render: (value: string) => <Button type="link" className="code-link" onClick={() => openQuote(value)}>{value}</Button> },
    { title: '名称', dataIndex: 'symbol', width: 110, render: (value: string) => <strong>{stockName(value) || '--'}</strong> },
    { title: '买入日期', dataIndex: 'entry_date', width: 104 },
    { title: '买入价', dataIndex: 'entry_price', width: 82, render: (value: number, row: HistoricalBacktestTrade) => row.status === 'skipped' ? '--' : value?.toFixed(2) || '--' },
    { title: '股数', dataIndex: 'shares', width: 76, render: (value: number, row: HistoricalBacktestTrade) => row.status === 'skipped' ? '--' : Number(value || 0).toLocaleString() },
    { title: '卖出日期', dataIndex: 'exit_date', width: 104, render: (value: string) => value || '--' },
    { title: '卖出价', dataIndex: 'exit_price', width: 82, render: (value: number) => value ? value.toFixed(2) : '--' },
    {
      title: '盈亏', dataIndex: 'pnl', width: 96,
      render: (value: number, row: HistoricalBacktestTrade) => row.status === 'closed' ? <strong className={value >= 0 ? 'trading-positive' : 'trading-negative'}>{value >= 0 ? '+' : ''}{formatMoney(value)}</strong> : '--'
    },
    { title: '盈亏率', dataIndex: 'pnl_rate', width: 82, render: (value: number, row: HistoricalBacktestTrade) => row.status === 'closed' ? pct(value) : '--' },
    { title: '持有天数', dataIndex: 'hold_days', width: 84, render: (value: number, row: HistoricalBacktestTrade) => row.status === 'skipped' ? '--' : value || 0 },
    { title: '触发策略', dataIndex: 'strategy_name', width: 180 },
    { title: '信号日', dataIndex: 'signal_date', width: 104, render: (value: string) => value || '--' },
    { title: '备注', dataIndex: 'reason', width: 220, render: (value: string) => value || '--' }
  ];

  return (
    <div className="historical-backtest-workspace">
      <Card className="work-card historical-backtest-launch" title={<div className="historical-backtest-title"><div><span>历史选股回放</span><Text type="secondary">按交易日重放策略并验证后续表现</Text></div><small>POINT-IN-TIME REPLAY</small></div>} extra={<Button aria-label="刷新历史回测" icon={<ReloadOutlined />} loading={loading} onClick={() => void loadInitial()} />}>
        <div className="historical-backtest-form">
          <label><span>开始日期</span><DatePicker value={startDate} onChange={setStartDate} allowClear={false} /></label>
          <label><span>结束日期</span><DatePicker value={endDate} onChange={setEndDate} allowClear={false} disabledDate={(date) => Boolean(startDate && date.isBefore(startDate, 'day'))} /></label>
          <label className="historical-strategy-picker"><span>执行策略</span><Select mode="multiple" maxTagCount="responsive" value={strategyIDs} onChange={setStrategyIDs} options={strategies.map((item) => ({ value: item.id, label: item.name }))} /></label>
          <label><span>目标收益</span><InputNumber min={0.1} max={100} step={0.5} addonAfter="%" value={targetReturn} onChange={(value) => setTargetReturn(value || 3)} /></label>
          <label><span>回撤上限</span><InputNumber min={0.1} max={100} step={0.5} addonAfter="%" value={drawdownLimit} onChange={(value) => setDrawdownLimit(value || 5)} /></label>
          <label><span>初始资金</span><InputNumber min={10000} step={10000} addonAfter="元" value={initialCash} onChange={(value) => setInitialCash(value || 100000)} /></label>
          <label><span>最大持仓数</span><InputNumber min={1} max={20} value={maxPositions} onChange={(value) => setMaxPositions(value || 5)} /></label>
          <label><span>买入基准</span><Select value={buyBase} onChange={setBuyBase} style={{ width: 110 }} options={[{ value: 'open', label: '开盘价' }, { value: 'close', label: '收盘价' }]} /></label>
          <label><span>买入偏移</span><InputNumber min={-50} max={50} step={0.1} addonAfter="%" value={buyOffset} onChange={(value) => setBuyOffset(value || 0)} /></label>
          <label><span>卖出价</span><Select value={sellBase} onChange={setSellBase} style={{ width: 120 }} options={[{ value: 'next_open', label: '次日开盘' }, { value: 'same_close', label: '当日收盘' }]} /></label>
          <label><span>止盈</span><InputNumber min={0.1} max={100} step={0.5} addonAfter="%" value={takeProfit} onChange={(value) => setTakeProfit(value || 10)} /></label>
          <label><span>移动止损</span><InputNumber min={0.1} max={100} step={0.5} addonAfter="%" value={trailDrawdown} onChange={(value) => setTrailDrawdown(value || 5)} /></label>
          <label><span>固定止损</span><InputNumber min={0.1} max={100} step={0.5} addonAfter="%" value={stopLoss} onChange={(value) => setStopLoss(value || 3)} /></label>
          <label><span>持有天数</span><InputNumber min={1} max={250} value={maxHoldDays} onChange={(value) => setMaxHoldDays(value || 15)} /></label>
          <Button className="historical-start-button" type="primary" icon={<PlayCircleOutlined />} loading={starting} disabled={runs.some((item) => item.status === 'running')} onClick={() => void startBacktest()}>开始回测</Button>
        </div>
      </Card>

      <div className="historical-backtest-main">
        <Card className="work-card historical-run-list" title={<div className="historical-backtest-title"><div><span>回测批次</span><Text type="secondary">{runs.length} 条记录</Text></div><small>RUNS</small></div>}>
          {runs.length ? <div className="historical-run-stack">{runs.map((run) => { const meta = statusMeta(run.status); return <button type="button" key={run.id} className={run.id === selectedRun?.id ? 'is-active' : ''} onClick={() => { setSelectedRun(run); setPage(1); setTradePage(1); }}><span>{run.start_date} → {run.end_date}</span><strong>{run.strategy_ids.length} 个策略 · {run.signal_count} 条信号</strong><small><Tag color={meta.color}>{meta.label}</Tag>{run.processed_dates}/{run.total_dates || '--'} 交易日</small></button>; })}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无历史回测" />}
        </Card>

        <Card className="work-card historical-result-card" title={<div className="historical-backtest-title"><div><span>回测结果</span><Text type="secondary">{selectedRun ? `${selectedRun.start_date} 至 ${selectedRun.end_date}` : '等待任务'}</Text></div><small>RESULT</small></div>} extra={selectedRun?.status === 'running' ? <Button danger icon={<StopOutlined />} onClick={() => void cancelRun()}>取消</Button> : null}>
          {!selectedRun ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="创建回测后查看结果" /> : <>
            <div className="historical-progress-head">
              <div><ClockCircleOutlined /><span>当前交易日</span><strong>{selectedRun.current_date || '正在加载K线'}</strong></div>
              <Progress percent={selectedRun.status === 'success' ? 100 : progress} status={selectedRun.status === 'failed' ? 'exception' : selectedRun.status === 'success' ? 'success' : 'active'} />
              <Tag color={statusMeta(selectedRun.status).color}>{statusMeta(selectedRun.status).label}</Tag>
            </div>
            <div className="historical-facts">
              <div><span>交易日</span><strong>{selectedRun.processed_dates}/{selectedRun.total_dates || '--'}</strong></div>
              <div><span>候选股票</span><strong>{selectedRun.candidate_symbols || '--'}</strong></div>
              <div><span>历史信号</span><strong>{selectedRun.signal_count}</strong></div>
              <div><span>成功标准</span><strong>收益 ≥ {selectedRun.target_return}%</strong><small>回撤不低于 -{selectedRun.drawdown_limit}%</small></div>
            </div>
            {selectedRun.error ? <Alert type="error" showIcon message={selectedRun.error} /> : null}
            <div className="historical-result-toolbar">
              <Segmented value={view} onChange={(value) => setView(value as ResultView)} options={[
                { value: 'ranking', label: '策略排行', icon: <BarChartOutlined /> },
                { value: 'signals', label: '信号明细', icon: <ExperimentOutlined /> },
                { value: 'consensus', label: '多策略共振', icon: <TeamOutlined /> },
                { value: 'trades', label: '交易记录', icon: <SwapOutlined /> }
              ]} />
              {view === 'signals' ? <Space wrap><Select value={signalStrategy} onChange={(value) => { setSignalStrategy(value); setPage(1); }} options={[{ value: 'all', label: '全部策略' }, ...runStrategyOptions]} /><Input allowClear placeholder="股票代码" value={signalKeyword} onChange={(event) => { setSignalKeyword(event.target.value); setPage(1); }} /></Space> : null}
            </div>
            {view === 'ranking' ? <Table<HistoricalStrategySummary> size="small" rowKey="strategy_id" dataSource={summaries} columns={rankingColumns} pagination={false} scroll={{ x: 1050 }} locale={{ emptyText: selectedRun.status === 'running' ? '完成后生成策略统计' : '暂无策略统计' }} /> : null}
            {view === 'signals' ? <Table<HistoricalBacktestSignal> size="small" rowKey="id" dataSource={signals.items} columns={signalColumns} scroll={{ x: 1000 }} pagination={{ current: page, pageSize: 100, total: signals.total, showSizeChanger: false, onChange: (next) => setPage(next) }} locale={{ emptyText: '暂无历史信号' }} /> : null}
            {view === 'consensus' ? <Table<HistoricalConsensusSummary> size="small" rowKey={(row) => `${row.signal_date}-${row.symbol}`} dataSource={consensus} columns={consensusColumns} sortDirections={['ascend', 'descend']} pagination={{ pageSize: 50, showSizeChanger: false }} scroll={{ x: 1050 }} locale={{ emptyText: selectedRun.status === 'running' ? '完成后生成共振统计' : '暂无多策略共振信号' }} /> : null}
            {view === 'trades' ? <>
              {portfolio ? <div className="historical-portfolio-facts">
                <div><span>初始资金</span><strong>{formatMoney(portfolio.initial_cash)}</strong></div>
                <div><span>期末权益</span><strong>{formatMoney(portfolio.final_equity)}</strong></div>
                <div><span>总收益</span><strong className={Number(portfolio.total_return) >= 0 ? 'trading-positive' : 'trading-negative'}>{pct(portfolio.total_return)}</strong></div>
                <div><span>最大回撤</span><strong className="trading-negative">-{Number(portfolio.max_drawdown || 0).toFixed(2)}%</strong></div>
                <div><span>已平仓</span><strong>{portfolio.closed_count || 0} 笔</strong><small>胜 {portfolio.win_count || 0} · 胜率 {Number(portfolio.win_rate || 0).toFixed(1)}%</small></div>
                <div><span>持仓中</span><strong>{portfolio.open_count || 0} 笔</strong><small>最大持仓 {portfolio.max_positions || '--'} 只</small></div>
                <div><span>未成交</span><strong>{portfolio.skipped_count || 0} 笔</strong><small>资金不足或涨停买不进</small></div>
              </div> : null}
              <Table<HistoricalBacktestTrade> size="small" rowKey="id" dataSource={trades.items} columns={tradeColumns} scroll={{ x: 1560 }} pagination={{ current: tradePage, pageSize: 100, total: trades.total, showSizeChanger: false, onChange: (next) => setTradePage(next) }} locale={{ emptyText: selectedRun.status === 'running' ? '完成后生成交易流水' : '暂无交易记录' }} />
            </> : null}
          </>}
        </Card>
      </div>
      <StockQuoteModal target={quoteTarget} onClose={() => setQuoteTarget(null)} />
    </div>
  );
}
