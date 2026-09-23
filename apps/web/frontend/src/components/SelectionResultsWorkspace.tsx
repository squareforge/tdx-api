import { Button, Card, Empty, Input, Modal, Segmented, Select, Space, Table, Tag, Typography, message } from 'antd';
import { CalendarOutlined, ClockCircleOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import dayjs, { Dayjs } from 'dayjs';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { normalizeSymbol } from '../lib/format';
import type { AutomationRun, SelectionHorizon, SelectionResult, SelectionTracking, SelectionTrackingResponse } from '../types';
import { StockQuoteModal, type StockQuoteTarget } from './StockQuoteModal';

const { Text } = Typography;
const selectionTaskTypes = new Set(['stock_selection', 'strategy_selection']);

interface SelectionRunRecord { run: AutomationRun; results: SelectionResult[]; }
interface RunDayGroup { date: string; runs: SelectionRunRecord[]; }
interface StockDirectoryItem { code?: string; name?: string; }
interface StockDirectory { codes?: StockDirectoryItem[]; }
interface StrategySelectionDetail {
  rank?: number;
  score?: number;
  rank_score?: number;
  strategy_score?: number;
  rank_reason?: string;
}

function eventDate(value?: string): Dayjs | null {
  const date = value ? dayjs(value) : null;
  return date?.isValid() ? date : null;
}

function runStart(run: AutomationRun) { return eventDate(run.started_at) || eventDate(run.finished_at); }
function runDateKey(run: AutomationRun) { return runStart(run)?.format('YYYY-MM-DD') || 'unknown'; }
function formatRunTime(run: AutomationRun) { return runStart(run)?.format('HH:mm:ss') || '--:--'; }
function formatRunDuration(run: AutomationRun) {
  const startedAt = eventDate(run.started_at);
  const finishedAt = eventDate(run.finished_at);
  if (!startedAt) return '--';
  if (!finishedAt) return run.status === 'running' ? '进行中' : '--';
  const seconds = Math.max(0, finishedAt.diff(startedAt, 'second'));
  if (seconds < 1) return '不足 1 秒';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainingSeconds = seconds % 60;
  if (hours) return `${hours}小时${minutes}分${remainingSeconds}秒`;
  if (minutes) return `${minutes}分${remainingSeconds}秒`;
  return `${remainingSeconds}秒`;
}
function formatRunDate(date: string) { return date === 'unknown' ? '未知日期' : dayjs(date).format('YYYY年MM月DD日'); }
function runStatus(status?: string) {
  if (status === 'success') return { label: '已完成', color: 'success' } as const;
  if (status === 'failed') return { label: '失败', color: 'error' } as const;
  if (status === 'running') return { label: '执行中', color: 'processing' } as const;
  return { label: status || '未知', color: 'default' } as const;
}
function runTypeLabel(type?: string) { return type === 'strategy_selection' ? '策略选股' : '公式选股'; }
function resultTracking(result: SelectionResult): SelectionTracking { return result.tracking || {}; }
function formatPct(value?: number) { return Number.isFinite(value) ? `${Number(value) >= 0 ? '+' : ''}${Number(value).toFixed(2)}%` : '--'; }
function trackingLabel(value?: SelectionHorizon) {
  if (!value) return '待观察';
  if (value.status === 'complete') return value.success ? '达标' : '未达标';
  return value.reason?.replace(/^已获得(\d+)\/(\d+)个交易日$/, '已获得 $1/$2 个交易日') || (value.status === 'unavailable' ? '暂无数据' : '待观察');
}
function trackingColor(value?: SelectionHorizon) { if (!value || value.status !== 'complete') return 'default'; return value.success ? 'success' : 'error'; }
function selectionDetail(result: SelectionResult): StrategySelectionDetail {
  try { return JSON.parse(result.detail_json || '{}') as StrategySelectionDetail; } catch { return {}; }
}
function compareSelectionRank(left: SelectionResult, right: SelectionResult) {
  const leftDetail = selectionDetail(left);
  const rightDetail = selectionDetail(right);
  const leftRank = Number.isFinite(leftDetail.rank) && Number(leftDetail.rank) > 0 ? Number(leftDetail.rank) : Number.MAX_SAFE_INTEGER;
  const rightRank = Number.isFinite(rightDetail.rank) && Number(rightDetail.rank) > 0 ? Number(rightDetail.rank) : Number.MAX_SAFE_INTEGER;
  if (leftRank === Number.MAX_SAFE_INTEGER && rightRank === Number.MAX_SAFE_INTEGER) return left.symbol.localeCompare(right.symbol);
  return leftRank - rightRank || Number(rightDetail.rank_score || rightDetail.score || 0) - Number(leftDetail.rank_score || leftDetail.score || 0) || left.symbol.localeCompare(right.symbol);
}

function buildRunRecords(runs: AutomationRun[], results: SelectionResult[]) {
  const resultMap = new Map<string, SelectionResult[]>();
  results.forEach((result) => resultMap.set(result.run_id, [...(resultMap.get(result.run_id) || []), result]));
  const runMap = new Map<string, AutomationRun>();
  runs.filter((run) => selectionTaskTypes.has(run.task_type)).forEach((run) => runMap.set(run.id, run));
  resultMap.forEach((items, runID) => {
    if (runMap.has(runID)) return;
    const first = items[0];
    runMap.set(runID, { id: runID, task_id: first.task_id, task_name: first.task_name, task_type: 'stock_selection', status: 'success', started_at: first.created_at, finished_at: first.created_at, matched_count: items.length });
  });
  return [...runMap.values()].map((run) => ({ run, results: [...(resultMap.get(run.id) || [])].sort(compareSelectionRank) })).sort((left, right) => (right.run.started_at || right.run.finished_at || '').localeCompare(left.run.started_at || left.run.finished_at || ''));
}

export function SelectionResultsWorkspace() {
  const [records, setRecords] = useState<SelectionRunRecord[]>([]);
  const [tracking, setTracking] = useState<SelectionTrackingResponse>({ items: [] });
  const [selectedRun, setSelectedRun] = useState<SelectionRunRecord | null>(null);
  const [selectedResult, setSelectedResult] = useState<SelectionResult | null>(null);
  const [keyword, setKeyword] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [view, setView] = useState<'timeline' | 'calendar'>('timeline');
  const [loading, setLoading] = useState(false);
  const [trackingLoading, setTrackingLoading] = useState(false);
  const [resultModalOpen, setResultModalOpen] = useState(false);
  const [stockNames, setStockNames] = useState<Record<string, string>>({});
  const [quoteTarget, setQuoteTarget] = useState<StockQuoteTarget | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [runs, results, tracked, directory] = await Promise.all([
        apiFetch<AutomationRun[]>('/api/automations/runs?limit=200'),
        apiFetch<SelectionResult[]>('/api/selection-results?limit=500'),
        apiFetch<SelectionTrackingResponse>('/api/selection-results/tracking?latest=1&limit=200&cached=1'),
        apiFetch<StockDirectory>('/api/codes').catch(() => ({ codes: [] }))
      ]);
      const next = buildRunRecords(runs || [], results || []);
      setStockNames(Object.fromEntries((directory.codes || []).flatMap((item) => {
        const symbol = normalizeSymbol(item.code);
        const name = item.name?.trim();
        return symbol && name ? [[symbol, name]] : [];
      })));
      setRecords(next);
      setTracking(tracked || { items: [] });
      setSelectedRun((current) => (current ? next.find((item) => item.run.id === current.run.id) || next[0] || null : next[0] || null));
    } catch (error) { message.error(error instanceof Error ? error.message : '选股执行记录加载失败'); } finally { setLoading(false); }
  }, []);

  const refreshTracking = useCallback(async () => {
    setTrackingLoading(true);
    try { await apiFetch('/api/selection-results/tracking?latest=0&limit=500', { method: 'POST' }); message.success('历史跟踪已刷新'); await load(); }
    catch (error) { message.error(error instanceof Error ? error.message : '历史跟踪刷新失败'); }
    finally { setTrackingLoading(false); }
  }, [load]);

  useEffect(() => { load(); }, [load]);
  const trackingMap = useMemo(() => new Map((tracking.items || []).map((item) => [item.result.id, item.tracking])), [tracking.items]);
  const filteredRecords = useMemo(() => {
    const query = keyword.trim().toLowerCase();
    return records.filter(({ run, results }) => {
      if (typeFilter !== 'all' && run.task_type !== typeFilter) return false;
      if (!query) return true;
      return [run.task_name, run.id, ...results.map((item) => `${item.symbol} ${stockNames[normalizeSymbol(item.symbol)] || ''} ${item.formula_name}`)].some((value) => value.toLowerCase().includes(query));
    });
  }, [keyword, records, stockNames, typeFilter]);
  const groups = useMemo<RunDayGroup[]>(() => {
    const grouped = new Map<string, SelectionRunRecord[]>();
    filteredRecords.forEach((record) => { const date = runDateKey(record.run); grouped.set(date, [...(grouped.get(date) || []), record]); });
    return [...grouped.entries()].map(([date, runs]) => ({ date, runs }));
  }, [filteredRecords]);
  const summary = useMemo(() => ({ total: filteredRecords.length, success: filteredRecords.filter(({ run }) => run.status === 'success').length, matched: filteredRecords.reduce((sum, { run, results }) => sum + (run.matched_count ?? results.length), 0), days: groups.length }), [filteredRecords, groups.length]);

  function openRun(record: SelectionRunRecord) { setSelectedRun(record); setSelectedResult(record.results[0] || null); setResultModalOpen(true); }
  function getTracking(result: SelectionResult) { return trackingMap.get(result.id) || resultTracking(result); }
  function stockName(symbol: string) { return stockNames[normalizeSymbol(symbol)] || ''; }
  function openQuote(result: SelectionResult) {
    setSelectedResult(result);
    setQuoteTarget({ code: result.symbol, name: stockName(result.symbol) || undefined });
  }
  const modalRun = selectedRun?.run;
  const modalResults = selectedRun?.results || [];
  const modalColumns = [
    { title: '名次', key: 'rank', width: 66, render: (_value: unknown, record: SelectionResult) => selectionDetail(record).rank || '--' },
    {
      title: '代码',
      dataIndex: 'symbol',
      width: 110,
      render: (value: string, record: SelectionResult) => (
        <Button type="link" className="code-link" onClick={(event) => { event.stopPropagation(); openQuote(record); }}>
          {value}
        </Button>
      )
    },
    { title: '名称', dataIndex: 'symbol', width: 120, render: (value: string) => <strong>{stockName(value) || '--'}</strong> },
    { title: '公式', dataIndex: 'formula_name', width: 170 },
    { title: '统一分', key: 'rank_score', width: 82, render: (_value: unknown, record: SelectionResult) => { const detail = selectionDetail(record); const value = detail.rank_score ?? detail.score; return Number.isFinite(value) ? Number(value).toFixed(1) : '--'; } },
    { title: '策略分', key: 'strategy_score', width: 82, render: (_value: unknown, record: SelectionResult) => { const value = selectionDetail(record).strategy_score; return Number.isFinite(value) ? Number(value).toFixed(1) : '--'; } },
    { title: '最新值', dataIndex: 'latest', width: 100, render: (value: number) => value ? value.toFixed(2) : '--' },
    ...[1, 5, 10].map((days) => ({
      title: `D${days}`,
      width: 145,
      render: (_value: unknown, record: SelectionResult) => {
        const value = getTracking(record).horizons?.[`d${days}`];
        return <Tag color={trackingColor(value)}>{trackingLabel(value)}{value?.status === 'complete' ? ` ${formatPct(value.close_return)}` : ''}</Tag>;
      }
    }))
  ];

  return (
    <div className="selection-results-workspace">
      <Card className="work-card selection-results-overview" title={<div className="selection-results-title"><div><span>选股执行记录</span><Text type="secondary">每次执行一条记录，点击查看本次全部标的</Text></div><small>SELECTION RUNS</small></div>} extra={<Space><Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新记录</Button><Button onClick={refreshTracking} loading={trackingLoading}>刷新跟踪</Button></Space>}>
        <div className="selection-results-summary"><div><span>执行次数</span><strong>{summary.total}</strong><small>{summary.days} 个交易日</small></div><div><span>已完成</span><strong>{summary.success}</strong><small>选股运行</small></div><div><span>累计命中</span><strong>{summary.matched}</strong><small>只标的</small></div><div className="selection-results-summary-note"><ClockCircleOutlined /><div><strong>运行记录优先</strong><small>命中为 0 的执行也会保留</small></div></div></div>
      </Card>

      <div className="selection-results-toolbar"><Space wrap><Input allowClear prefix={<SearchOutlined />} value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜索任务、公式或股票代码" /><Select value={typeFilter} onChange={setTypeFilter} options={[{ value: 'all', label: '全部选股类型' }, { value: 'stock_selection', label: '公式选股' }, { value: 'strategy_selection', label: '策略选股' }]} /></Space><Segmented value={view} onChange={(value) => setView(value as 'timeline' | 'calendar')} options={[{ value: 'timeline', label: '时间线', icon: <ClockCircleOutlined /> }, { value: 'calendar', label: '日历', icon: <CalendarOutlined /> }]} /></div>

      {view === 'timeline' ? (
        <Card className="work-card selection-runs-card" title={<div className="selection-results-title"><div><span>执行时间线</span><Text type="secondary">按日期查看每次选股运行</Text></div><small>TIMELINE</small></div>}>
          {groups.length ? <div className="selection-timeline">{groups.map((group) => <div className="selection-day" key={group.date}><div className="selection-day-marker"><strong>{group.date === 'unknown' ? '--' : dayjs(group.date).format('DD')}</strong><span>{group.date === 'unknown' ? '未知日期' : dayjs(group.date).format('MM月')}</span></div><div className="selection-day-runs">{group.runs.map((record) => { const status = runStatus(record.run.status); return <button type="button" className={`selection-run-row ${selectedRun?.run.id === record.run.id ? 'is-selected' : ''}`} key={record.run.id} onClick={() => openRun(record)}><span className="selection-run-time">{formatRunTime(record.run)}</span><span className="selection-run-main"><strong>{record.run.task_name || '未命名选股任务'}</strong><small>{runTypeLabel(record.run.task_type)} · {record.results.length ? `${record.results.length} 只命中` : '无命中标的'} · 耗时 {formatRunDuration(record.run)}</small></span><span className="selection-run-meta"><Tag color={status.color}>{status.label}</Tag><em>{record.run.id.slice(0, 8)}</em></span></button>; })}</div></div>)}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前筛选没有选股执行记录" />}
        </Card>
      ) : (
        <Card className="work-card selection-runs-card selection-calendar-card" title={<div className="selection-results-title"><div><span>选股日历</span><Text type="secondary">按日期查看执行记录</Text></div><small>CALENDAR</small></div>}>
          {groups.length ? <div className="selection-calendar-grid">{groups.map((group) => <section className="selection-calendar-day" key={group.date}><header><div><strong>{group.date === 'unknown' ? '--' : dayjs(group.date).format('MM/DD')}</strong><span>{formatRunDate(group.date)}</span></div><em>{group.runs.length} 次执行</em></header><div>{group.runs.map((record) => { const status = runStatus(record.run.status); return <button type="button" className={`selection-calendar-run ${status.color}`} key={record.run.id} onClick={() => openRun(record)}><span>{formatRunTime(record.run)}</span><strong>{record.run.task_name || '未命名选股任务'}</strong><small>{record.results.length} 只命中 · 耗时 {formatRunDuration(record.run)} · {status.label}</small></button>; })}</div></section>)}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前筛选没有选股执行记录" />}
        </Card>
      )}

      <Modal open={resultModalOpen} onCancel={() => setResultModalOpen(false)} footer={null} width={1120} centered destroyOnHidden className="app-themed-modal selection-run-modal" title={<div className="quote-dialog-title selection-run-dialog-title"><div><strong>{modalRun?.task_name || '选股执行结果'}</strong><span>{modalRun ? `${runTypeLabel(modalRun.task_type)} · ${formatRunDate(runDateKey(modalRun))} ${formatRunTime(modalRun)}` : '本次执行的全部命中标的'}</span></div><small>RUN RESULTS</small></div>}>
        {modalRun ? <div className="selection-run-modal-content"><div className="selection-run-facts"><div><span>执行状态</span><strong><Tag color={runStatus(modalRun.status).color}>{runStatus(modalRun.status).label}</Tag></strong></div><div><span>命中数量</span><strong>{modalRun.matched_count ?? modalResults.length} 只</strong></div><div><span>执行耗时</span><strong>{formatRunDuration(modalRun)}</strong></div><div><span>开始时间</span><strong>{modalRun.started_at || '--'}</strong></div><div><span>结束时间</span><strong>{modalRun.finished_at || '--'}</strong></div></div><Table<SelectionResult> size="small" rowKey="id" dataSource={modalResults} pagination={{ pageSize: 12, size: 'small' }} scroll={{ x: 1110 }} onRow={(record) => ({ onClick: () => setSelectedResult(record) })} rowClassName={(record) => (record.id === selectedResult?.id ? 'row-active' : '')} locale={{ emptyText: '本次执行没有命中标的' }} columns={modalColumns} />{selectedResult ? <div className="selection-run-result-detail"><Text type="secondary">当前标的</Text><strong>{stockName(selectedResult.symbol) || '名称未知'} · {selectedResult.symbol}</strong><span>{selectedResult.formula_name} · 信号时间 {selectedResult.created_at}</span>{selectionDetail(selectedResult).rank_reason ? <span>{selectionDetail(selectedResult).rank_reason}</span> : null}</div> : null}</div> : null}
      </Modal>
      <StockQuoteModal target={quoteTarget} onClose={() => setQuoteTarget(null)} />
    </div>
  );
}
