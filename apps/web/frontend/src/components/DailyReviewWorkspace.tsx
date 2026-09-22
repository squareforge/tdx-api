import {
  Alert, Button, Card, Empty, Input, Modal, Segmented, Select, Space, Table, Tag, Tooltip, Typography, message
} from 'antd';
import {
  CheckCircleOutlined, ClearOutlined, ClockCircleOutlined, EditOutlined, EyeOutlined, ReloadOutlined,
  StopOutlined, TeamOutlined, UnorderedListOutlined, BarChartOutlined, CalendarOutlined
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { normalizeSymbol } from '../lib/format';
import type {
  DailyReviewBatchSummary, DailyReviewResponse, DailyReviewStock, DailyReviewStrategySummary,
  DecisionNote, SelectionTracking, DailyStrategyAnalysisResponse, DailyStrategyAnalysisMetric
} from '../types';
import { StockQuoteModal, type StockQuoteTarget } from './StockQuoteModal';

const { Text } = Typography;
const { TextArea } = Input;
type ReviewView = 'overview' | 'stocks' | 'consensus';
type DailyReviewMode = 'single' | 'analysis';
interface StockDirectoryItem { code?: string; name?: string; }
interface StockDirectory { codes?: StockDirectoryItem[]; }

const emptyReview: DailyReviewResponse = {
  batches: [], strategies: [], stocks: [], signals: [], consensus: [], watch: [], exclude: [], notes: [],
  summary: {
    strategy_count: 0, raw_signals: 0, stock_count: 0, consensus_count: 0, watch_count: 0,
    exclude_count: 0, candidate_symbols: 0, kline_loaded: 0, kline_failed: 0, duration_ms: 0
  }
};

function parseJSON<T>(value?: string, fallback?: T): T {
  try { return JSON.parse(value || '') as T; } catch { return fallback as T; }
}

function statusMeta(status?: string) {
  if (status === 'success') return { label: '已完成', color: 'success' } as const;
  if (status === 'failed') return { label: '失败', color: 'error' } as const;
  if (status === 'cancelled') return { label: '已取消', color: 'default' } as const;
  return { label: '执行中', color: 'processing' } as const;
}

function formatDuration(milliseconds?: number) {
  const value = Number(milliseconds || 0);
  if (!value) return '--';
  if (value < 1000) return `${Math.round(value)}ms`;
  if (value < 60000) return `${(value / 1000).toFixed(value < 10000 ? 1 : 0)}s`;
  const minutes = Math.floor(value / 60000);
  const seconds = Math.round((value % 60000) / 1000);
  return `${minutes}分${seconds}秒`;
}

function pct(value?: number) {
  if (!Number.isFinite(value)) return '--';
  const number = Number(value);
  return `${number >= 0 ? '+' : ''}${number.toFixed(2)}%`;
}

function horizonMetric(raw: string | undefined, horizon: number) {
  return parseJSON<SelectionTracking>(raw, { horizons: {} }).horizons?.[`d${horizon}`];
}

function horizonReturn(raw: string | undefined, horizon: number) {
  const value = horizonMetric(raw, horizon);
  return value?.status === 'complete' && Number.isFinite(value.close_return) ? value.close_return : undefined;
}

function horizonCell(raw: string | undefined, horizon: number) {
  const value = horizonMetric(raw, horizon);
  if (!value || value.status !== 'complete') return <Tag>待观察</Tag>;
  const change = Number(value.close_return);
  return <Tag color={change >= 0 ? 'error' : 'success'}>{pct(change)}</Tag>;
}

function compareNullableNumber(left?: number, right?: number) {
  const leftValid = Number.isFinite(left);
  const rightValid = Number.isFinite(right);
  if (!leftValid && !rightValid) return 0;
  if (!leftValid) return 1;
  if (!rightValid) return -1;
  return Number(left) - Number(right);
}

function decisionMeta(stock: DailyReviewStock) {
  if (stock.excluded) return { label: '已排除', color: 'error' } as const;
  if (stock.watch) return { label: '观察中', color: 'processing' } as const;
  return { label: '待处理', color: 'default' } as const;
}

function formatAnalysisPct(value?: number) {
  return Number.isFinite(value) ? `${Number(value) >= 0 ? '+' : ''}${Number(value).toFixed(2)}%` : '--';
}

type AnalysisHorizonKey = 'd3' | 'd5' | 'd10';
interface AnalysisHorizonSummary {
  completed: number;
  successCount: number;
  successRate: number;
  positiveCount: number;
  positiveRate: number;
  averageReturn: number;
  maxReturn: number;
}

const analysisHorizonFields: Record<AnalysisHorizonKey, { completed: keyof DailyStrategyAnalysisMetric; successCount: keyof DailyStrategyAnalysisMetric; successRate: keyof DailyStrategyAnalysisMetric; positiveCount: keyof DailyStrategyAnalysisMetric; positiveRate: keyof DailyStrategyAnalysisMetric; averageReturn: keyof DailyStrategyAnalysisMetric; maxReturn: keyof DailyStrategyAnalysisMetric }> = {
  d3: { completed: 'd3_completed', successCount: 'd3_success_count', successRate: 'd3_success_rate', positiveCount: 'd3_positive_count', positiveRate: 'd3_positive_rate', averageReturn: 'd3_average_return', maxReturn: 'd3_max_return' },
  d5: { completed: 'd5_completed', successCount: 'd5_success_count', successRate: 'd5_success_rate', positiveCount: 'd5_positive_count', positiveRate: 'd5_positive_rate', averageReturn: 'd5_average_return', maxReturn: 'd5_max_return' },
  d10: { completed: 'd10_completed', successCount: 'd10_success_count', successRate: 'd10_success_rate', positiveCount: 'd10_positive_count', positiveRate: 'd10_positive_rate', averageReturn: 'd10_average_return', maxReturn: 'd10_max_return' }
};

function metricHorizon(metric: DailyStrategyAnalysisMetric, horizon: AnalysisHorizonKey): AnalysisHorizonSummary {
  const fields = analysisHorizonFields[horizon];
  return {
    completed: Number(metric[fields.completed] || 0),
    successCount: Number(metric[fields.successCount] || 0),
    successRate: Number(metric[fields.successRate] || 0),
    positiveCount: Number(metric[fields.positiveCount] || 0),
    positiveRate: Number(metric[fields.positiveRate] || 0),
    averageReturn: Number(metric[fields.averageReturn] || 0),
    maxReturn: Number(metric[fields.maxReturn] || 0)
  };
}

function summarizeAnalysisHorizon(metrics: DailyStrategyAnalysisMetric[], horizon: AnalysisHorizonKey): AnalysisHorizonSummary {
  const summary: AnalysisHorizonSummary = { completed: 0, successCount: 0, successRate: 0, positiveCount: 0, positiveRate: 0, averageReturn: 0, maxReturn: 0 };
  let weightedReturn = 0;
  let hasMaxReturn = false;
  for (const metric of metrics) {
    const item = metricHorizon(metric, horizon);
    summary.completed += item.completed;
    summary.successCount += item.successCount;
    summary.positiveCount += item.positiveCount;
    weightedReturn += item.averageReturn * item.completed;
    if (item.completed > 0 && (!hasMaxReturn || item.maxReturn > summary.maxReturn)) {
      summary.maxReturn = item.maxReturn;
      hasMaxReturn = true;
    }
  }
  if (summary.completed > 0) {
    summary.successRate = summary.successCount / summary.completed * 100;
    summary.positiveRate = summary.positiveCount / summary.completed * 100;
    summary.averageReturn = weightedReturn / summary.completed;
  }
  return summary;
}

function analysisLeader(metrics: DailyStrategyAnalysisMetric[], horizon: AnalysisHorizonKey) {
  return [...metrics]
    .filter((metric) => metricHorizon(metric, horizon).completed > 0)
    .sort((left, right) => {
      const a = metricHorizon(left, horizon);
      const b = metricHorizon(right, horizon);
      return b.positiveCount - a.positiveCount || b.positiveRate - a.positiveRate || b.averageReturn - a.averageReturn;
    })[0];
}

export function DailyReviewWorkspace() {
  const [mode, setMode] = useState<DailyReviewMode>('single');
  const [review, setReview] = useState<DailyReviewResponse>(emptyReview);
  const [view, setView] = useState<ReviewView>('stocks');
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [actionSymbol, setActionSymbol] = useState('');
  const [stockNames, setStockNames] = useState<Record<string, string>>({});
  const [quoteTarget, setQuoteTarget] = useState<StockQuoteTarget | null>(null);
  const [noteTarget, setNoteTarget] = useState<DailyReviewStock | null>(null);
  const [noteText, setNoteText] = useState('');
  const [noteSaving, setNoteSaving] = useState(false);
  const [analysis, setAnalysis] = useState<DailyStrategyAnalysisResponse>({ start_date: '', end_date: '', trading_days: 0, strategy_count: 0, signal_count: 0, unique_symbols: 0, consensus_count: 0, average_score: 0, strategies: [] });
  const [analysisLoading, setAnalysisLoading] = useState(false);
  const [analysisPreset, setAnalysisPreset] = useState('20');
  const [analysisStartDate, setAnalysisStartDate] = useState(dayjs().subtract(40, 'day').format('YYYY-MM-DD'));
  const [analysisEndDate, setAnalysisEndDate] = useState(dayjs().format('YYYY-MM-DD'));
  const [analysisStrategies, setAnalysisStrategies] = useState<string[]>([]);

  const load = useCallback(async (batchID?: string) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ limit: '5000' });
      if (batchID) params.set('batch_id', batchID);
      setReview(await apiFetch<DailyReviewResponse>(`/api/daily-review?${params}`));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '每日复盘加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const loadAnalysis = useCallback(async (preset = analysisPreset, endDate = analysisEndDate, startDate = analysisStartDate) => {
    setAnalysisLoading(true);
    try {
      const params = new URLSearchParams({ limit: '20000' });
      params.set('end_date', endDate);
      if (preset === 'custom') params.set('start_date', startDate);
      else params.set('days', preset);
      await apiFetch<DailyStrategyAnalysisResponse>(`/api/daily-review/analysis?${params}`).then(setAnalysis);
    } catch (error) { message.error(error instanceof Error ? error.message : '多日策略分析加载失败'); }
    finally { setAnalysisLoading(false); }
  }, [analysisEndDate, analysisPreset, analysisStartDate]);

  useEffect(() => {
    void Promise.all([
      load(),
      apiFetch<StockDirectory>('/api/codes').then((directory) => {
        setStockNames(Object.fromEntries((directory.codes || []).flatMap((item) => {
          const code = normalizeSymbol(item.code);
          return code && item.name?.trim() ? [[code, item.name.trim()]] : [];
        })));
      }).catch(() => undefined)
    ]);
  }, [load]);

  useEffect(() => {
    if (review.selected_batch?.status !== 'running') return;
    const timer = window.setInterval(() => { void load(review.selected_batch?.id); }, 3000);
    return () => window.clearInterval(timer);
  }, [load, review.selected_batch?.id, review.selected_batch?.status]);

  useEffect(() => { if (mode === 'analysis') void loadAnalysis(); }, [loadAnalysis, mode]);

  function stockName(symbol: string) { return stockNames[normalizeSymbol(symbol)] || ''; }
  function openQuote(symbol: string) { setQuoteTarget({ code: symbol, name: stockName(symbol) || undefined }); }

  const visibleStocks = useMemo(() => {
    const source = view === 'consensus' ? review.consensus : review.stocks;
    const value = keyword.trim().toLowerCase();
    if (!value) return source;
    return source.filter((stock) => [stock.symbol, stockName(stock.symbol), ...stock.strategies].some((item) => item.toLowerCase().includes(value)));
  }, [keyword, review.consensus, review.stocks, stockNames, view]);

  async function setDecision(stock: DailyReviewStock, next: 'watch' | 'exclude' | 'clear') {
    setActionSymbol(stock.symbol);
    try {
      if (next === 'watch') {
        await apiFetch(`/api/stock-pools/watchlist/symbols/${stock.symbol}`, { method: 'POST' });
        if (stock.excluded) await apiFetch(`/api/stock-pools/exclude/symbols/${stock.symbol}`, { method: 'DELETE' });
      } else if (next === 'exclude') {
        await apiFetch(`/api/stock-pools/exclude/symbols/${stock.symbol}`, { method: 'POST' });
        if (stock.watch) await apiFetch(`/api/stock-pools/watchlist/symbols/${stock.symbol}`, { method: 'DELETE' });
      } else {
        if (stock.watch) await apiFetch(`/api/stock-pools/watchlist/symbols/${stock.symbol}`, { method: 'DELETE' });
        if (stock.excluded) await apiFetch(`/api/stock-pools/exclude/symbols/${stock.symbol}`, { method: 'DELETE' });
        await apiFetch(`/api/decision-notes/${stock.symbol}`, {
          method: 'PUT', body: JSON.stringify({ ...stock.note, symbol: stock.symbol, status: '' })
        });
      }
      await load(review.selected_batch?.id);
      message.success(next === 'watch' ? '已加入观察池' : next === 'exclude' ? '已加入排除池' : '状态已清除');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '状态更新失败');
    } finally {
      setActionSymbol('');
    }
  }

  function editNote(stock: DailyReviewStock) {
    setNoteTarget(stock);
    setNoteText(stock.note?.review_note || '');
  }

  async function saveNote() {
    if (!noteTarget) return;
    setNoteSaving(true);
    try {
      await apiFetch<DecisionNote>(`/api/decision-notes/${noteTarget.symbol}`, {
        method: 'PUT', body: JSON.stringify({ ...noteTarget.note, symbol: noteTarget.symbol, review_note: noteText.trim() })
      });
      setNoteTarget(null);
      await load(review.selected_batch?.id);
      message.success('复盘备注已保存');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '备注保存失败');
    } finally {
      setNoteSaving(false);
    }
  }

  const strategyColumns = [
    { title: '策略', dataIndex: 'strategy_name', width: 220, render: (value: string) => <strong>{value || '--'}</strong> },
    { title: '状态', dataIndex: 'status', width: 92, filters: [{ text: '已完成', value: 'success' }, { text: '失败', value: 'failed' }], onFilter: (value: boolean | React.Key, row: DailyReviewStrategySummary) => row.status === value, render: (value: string) => { const meta = statusMeta(value); return <Tag color={meta.color}>{meta.label}</Tag>; } },
    { title: '命中', dataIndex: 'matched', width: 84, sorter: (left: DailyReviewStrategySummary, right: DailyReviewStrategySummary) => left.matched - right.matched },
    { title: '错误股票', dataIndex: 'errors', width: 96, sorter: (left: DailyReviewStrategySummary, right: DailyReviewStrategySummary) => left.errors - right.errors },
    { title: '耗时', dataIndex: 'duration_ms', width: 100, sorter: (left: DailyReviewStrategySummary, right: DailyReviewStrategySummary) => left.duration_ms - right.duration_ms, render: formatDuration },
    { title: '执行信息', dataIndex: 'error', ellipsis: true, render: (value: string) => value || '正常完成' }
  ];

  const stockColumns = [
    { title: '信号日', key: 'signal_date', width: 108, sorter: () => 0, render: () => review.date || '--' },
    { title: '代码', dataIndex: 'symbol', width: 92, sorter: (left: DailyReviewStock, right: DailyReviewStock) => left.symbol.localeCompare(right.symbol), render: (value: string) => <Button type="link" className="code-link" onClick={() => openQuote(value)}>{value}</Button> },
    { title: '名称', dataIndex: 'symbol', width: 118, render: (value: string) => <strong>{stockName(value) || '--'}</strong> },
    { title: '共振', dataIndex: 'strategy_count', width: 82, sorter: (left: DailyReviewStock, right: DailyReviewStock) => left.strategy_count - right.strategy_count, render: (value: number) => <Tag color={value >= 2 ? 'blue' : 'default'}>{value} 策略</Tag> },
    { title: '命中策略', dataIndex: 'strategies', width: 270, render: (value: string[]) => <span className="daily-review-strategy-names">{value.join(' · ')}</span> },
    { title: '最高分', dataIndex: 'max_score', width: 88, sorter: (left: DailyReviewStock, right: DailyReviewStock) => left.max_score - right.max_score, render: (value: number) => value?.toFixed(1) || '--' },
    { title: '均分', dataIndex: 'average_score', width: 78, sorter: (left: DailyReviewStock, right: DailyReviewStock) => left.average_score - right.average_score, render: (value: number) => value?.toFixed(1) || '--' },
    { title: '信号价', dataIndex: 'latest', width: 88, sorter: (left: DailyReviewStock, right: DailyReviewStock) => left.latest - right.latest, render: (value: number) => value > 0 ? value.toFixed(2) : '--' },
    ...[3, 5, 10].map((days) => ({
      title: `D${days}`, key: `d${days}`, width: 92,
      sorter: (left: DailyReviewStock, right: DailyReviewStock) => compareNullableNumber(horizonReturn(left.tracking_json, days), horizonReturn(right.tracking_json, days)),
      render: (_: unknown, row: DailyReviewStock) => horizonCell(row.tracking_json, days)
    })),
    { title: '状态', key: 'status', width: 88, filters: [{ text: '观察中', value: 'watch' }, { text: '已排除', value: 'exclude' }, { text: '待处理', value: '' }], onFilter: (value: boolean | React.Key, row: DailyReviewStock) => (row.status || '') === value, render: (_: unknown, row: DailyReviewStock) => { const meta = decisionMeta(row); return <Tag color={meta.color}>{meta.label}</Tag>; } },
    { title: '操作', key: 'actions', width: 142, fixed: 'right' as const, render: (_: unknown, row: DailyReviewStock) => <Space size={2} onClick={(event) => event.stopPropagation()}>
      <Tooltip title="加入观察池"><Button aria-label="加入观察池" type={row.watch ? 'primary' : 'text'} icon={<EyeOutlined />} loading={actionSymbol === row.symbol} onClick={() => void setDecision(row, 'watch')} /></Tooltip>
      <Tooltip title="加入排除池"><Button aria-label="加入排除池" danger type={row.excluded ? 'primary' : 'text'} icon={<StopOutlined />} loading={actionSymbol === row.symbol} onClick={() => void setDecision(row, 'exclude')} /></Tooltip>
      <Tooltip title="复盘备注"><Button aria-label="复盘备注" type="text" icon={<EditOutlined />} onClick={() => editNote(row)} /></Tooltip>
      <Tooltip title="清除状态"><Button aria-label="清除状态" type="text" icon={<ClearOutlined />} disabled={!row.watch && !row.excluded && !row.status} onClick={() => void setDecision(row, 'clear')} /></Tooltip>
    </Space> }
  ];

  const selected = review.selected_batch;
  const selectedStatus = statusMeta(selected?.status);
  const visibleAnalysisStrategies = analysisStrategies.length === 0
    ? analysis.strategies
    : analysis.strategies.filter((item) => analysisStrategies.includes(item.strategy_id));
  const selectedAnalysisSignalCount = visibleAnalysisStrategies.reduce((total, metric) => total + metric.signal_count, 0);
  const selectedAnalysisHorizons = {
    d3: summarizeAnalysisHorizon(visibleAnalysisStrategies, 'd3'),
    d5: summarizeAnalysisHorizon(visibleAnalysisStrategies, 'd5'),
    d10: summarizeAnalysisHorizon(visibleAnalysisStrategies, 'd10')
  };
  return (
    <div className="daily-review-workspace">
      <div className="daily-review-mode-bar">
        <Segmented value={mode} onChange={(value) => setMode(value as DailyReviewMode)} options={[{ value: 'single', label: '单日复盘', icon: <CalendarOutlined /> }, { value: 'analysis', label: '多日策略分析', icon: <BarChartOutlined /> }]} />
        <Text type="secondary">单日复盘看今天，策略分析看一段时间的稳定性</Text>
      </div>
      {mode === 'analysis' ? <div className="daily-analysis-workspace">
        <Card className="work-card daily-analysis-controls" title={<div className="historical-backtest-title"><div><span>多日策略分析</span><Text type="secondary">系统策略日报 · 按日期范围聚合</Text></div><small>STRATEGY ANALYSIS</small></div>} extra={<Button icon={<ReloadOutlined />} loading={analysisLoading} onClick={() => void loadAnalysis()}>刷新</Button>}>
          <div className="daily-analysis-filter-row">
            <label className="daily-analysis-filter-field daily-analysis-preset-field">
              <span>分析窗口</span>
              <Segmented value={analysisPreset} onChange={(value) => setAnalysisPreset(String(value))} options={[{ value: '10', label: '近10个交易日' }, { value: '20', label: '近20个交易日' }, { value: '60', label: '近60个交易日' }, { value: 'custom', label: '自定义范围' }]} />
            </label>
            {analysisPreset === 'custom' ? <label className="daily-analysis-filter-field"><span>开始日期</span><Input type="date" value={analysisStartDate} onChange={(event) => setAnalysisStartDate(event.target.value)} /></label> : null}
            <label className="daily-analysis-filter-field"><span>结束日期</span><Input type="date" value={analysisEndDate} onChange={(event) => setAnalysisEndDate(event.target.value)} /></label>
            <label className="daily-analysis-filter-field daily-analysis-strategy-field"><span>选择策略（可多选）</span><Select mode="multiple" value={analysisStrategies} onChange={setAnalysisStrategies} allowClear showSearch optionFilterProp="label" maxTagCount={1} maxTagPlaceholder={(omittedValues) => `+${omittedValues.length} 个`} placeholder="全部系统策略" options={analysis.strategies.map((item) => ({ value: item.strategy_id, label: item.strategy_name }))} /></label>
            {analysisPreset === 'custom' ? <Button className="daily-analysis-apply" type="primary" onClick={() => void loadAnalysis('custom', analysisEndDate, analysisStartDate)}>应用范围</Button> : null}
          </div>
          <div className="daily-analysis-range">分析区间 <strong>{analysis.start_date || '--'} 至 {analysis.end_date || '--'}</strong> · {analysis.trading_days} 个有日报的交易日 · 策略范围 {analysisStrategies.length ? `已选 ${analysisStrategies.length} 个` : '全部系统策略'}</div>
        </Card>
        <div className="daily-review-facts daily-analysis-facts"><div><span>分析天数</span><strong>{analysis.trading_days}</strong><small>有系统日报的交易日</small></div><div><span>纳入策略</span><strong>{visibleAnalysisStrategies.length}</strong><small>{analysisStrategies.length ? '当前选择' : '全部系统策略'}</small></div><div><span>区间信号</span><strong>{selectedAnalysisSignalCount}</strong><small>所选策略合计</small></div><div><span>D3 有效</span><strong>{selectedAnalysisHorizons.d3.completed}</strong><small>已完成跟踪</small></div><div><span>D5 有效</span><strong>{selectedAnalysisHorizons.d5.completed}</strong><small>已完成跟踪</small></div><div><span>D10 有效</span><strong>{selectedAnalysisHorizons.d10.completed}</strong><small>已完成跟踪</small></div></div>
        <div className="daily-analysis-horizon-grid">
          {(['d3', 'd5', 'd10'] as AnalysisHorizonKey[]).map((horizon) => {
            const summary = selectedAnalysisHorizons[horizon];
            const leader = analysisLeader(visibleAnalysisStrategies, horizon);
            return <Card key={horizon} className="work-card daily-analysis-horizon-card" title={`${horizon.toUpperCase()} 区间汇总`}>
              <div className="daily-analysis-horizon-metrics">
                <div><span>成功率</span><strong>{summary.completed > 0 ? `${summary.successRate.toFixed(1)}%` : '--'}</strong><small>{summary.successCount} / {summary.completed} 成功</small></div>
                <div><span>收红率</span><strong className={summary.completed > 0 && summary.positiveRate >= 50 ? 'analysis-positive' : ''}>{summary.completed > 0 ? `${summary.positiveRate.toFixed(1)}%` : '--'}</strong><small>{summary.positiveCount} / {summary.completed} 收红</small></div>
                <div><span>平均收益率</span><strong className={summary.completed > 0 ? (summary.averageReturn >= 0 ? 'analysis-positive' : 'analysis-negative') : ''}>{summary.completed > 0 ? formatAnalysisPct(summary.averageReturn) : '--'}</strong><small>已完成信号</small></div>
                <div><span>最高收益率</span><strong className={summary.completed > 0 ? (summary.maxReturn >= 0 ? 'analysis-positive' : 'analysis-negative') : ''}>{summary.completed > 0 ? formatAnalysisPct(summary.maxReturn) : '--'}</strong><small>单次收盘最高</small></div>
              </div>
              <div className="daily-analysis-leader">收红最多 <strong>{leader?.strategy_name || '--'}</strong>{leader ? <span>{metricHorizon(leader, horizon).positiveRate.toFixed(1)}% · {metricHorizon(leader, horizon).positiveCount} 次</span> : null}</div>
            </Card>;
          })}
        </div>
      </div> : null}
      {mode === 'single' ? <>
      <div className="historical-backtest-main daily-review-main">
        <Card className="work-card historical-run-list daily-review-batch-list" title={<div className="historical-backtest-title"><div><span>复盘批次</span><Text type="secondary">最近 {review.batches.length} 次</Text></div><small>DAILY RUNS</small></div>}>
          {review.batches.length ? <div className="historical-run-stack">{review.batches.map((batch: DailyReviewBatchSummary) => { const meta = statusMeta(batch.status); return <button type="button" key={batch.id} className={batch.id === selected?.id ? 'is-active' : ''} onClick={() => void load(batch.id)}><span>{batch.date} · {dayjs(batch.started_at).format('HH:mm:ss')}</span><strong>{batch.strategy_count || '--'} 个策略 · {batch.matched_count} 条信号</strong><small><Tag color={meta.color}>{meta.label}</Tag>{formatDuration(batch.duration_ms)}</small></button>; })}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无系统策略日报" />}
        </Card>

        <Card className="work-card historical-result-card daily-review-result" title={<div className="historical-backtest-title"><div><span>每日策略复盘</span><Text type="secondary">{selected ? `${selected.date} 收盘后选股` : '等待日报任务'}</Text></div><small>DAILY REVIEW</small></div>} extra={<Tooltip title="刷新"><Button aria-label="刷新每日复盘" icon={<ReloadOutlined />} loading={loading} onClick={() => void load(selected?.id)} /></Tooltip>}>
          {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="系统策略日报执行后在这里查看结果" /> : <>
            <div className="daily-review-status-line">
              <div><ClockCircleOutlined /><span>执行时间</span><strong>{dayjs(selected.started_at).format('YYYY-MM-DD HH:mm:ss')}</strong></div>
              <div><CheckCircleOutlined /><span>完成情况</span><strong>{review.summary.strategy_count} 个策略 · {formatDuration(review.summary.duration_ms)}</strong></div>
              <Tag color={selectedStatus.color}>{selectedStatus.label}</Tag>
            </div>
            <div className="daily-review-facts">
              <div><span>系统策略</span><strong>{review.summary.strategy_count}</strong><small>{selected.failed_strategies ? `${selected.failed_strategies} 个失败` : '全部正常'}</small></div>
              <div><span>原始信号</span><strong>{review.summary.raw_signals}</strong><small>保留跨策略重复</small></div>
              <div><span>去重股票</span><strong>{review.summary.stock_count}</strong><small>一只股票一行</small></div>
              <div><span>多策略共振</span><strong>{review.summary.consensus_count}</strong><small>至少 2 个策略命中</small></div>
              <div><span>候选范围</span><strong>{review.summary.candidate_symbols || '--'}</strong><small>K线 {review.summary.kline_loaded || '--'} 成功 / {review.summary.kline_failed} 失败</small></div>
              <div><span>已处理</span><strong>{review.summary.watch_count + review.summary.exclude_count}</strong><small>{review.summary.watch_count} 观察 / {review.summary.exclude_count} 排除</small></div>
            </div>
            {selected.error ? <Alert type="error" showIcon message={selected.error} /> : null}
            <div className="historical-result-toolbar daily-review-toolbar">
              <Segmented value={view} onChange={(value) => setView(value as ReviewView)} options={[
                { value: 'overview', label: '策略概览', icon: <CheckCircleOutlined /> },
                { value: 'stocks', label: '全部股票', icon: <UnorderedListOutlined /> },
                { value: 'consensus', label: '多策略共振', icon: <TeamOutlined /> }
              ]} />
              {view !== 'overview' ? <Input.Search allowClear placeholder="代码、名称或策略" value={keyword} onChange={(event) => setKeyword(event.target.value)} /> : null}
            </div>
            {view === 'overview' ? <Table<DailyReviewStrategySummary> size="small" rowKey={(row) => row.run_id || row.strategy_id} dataSource={review.strategies} columns={strategyColumns} pagination={false} scroll={{ x: 850 }} locale={{ emptyText: selected.status === 'running' ? '策略正在执行' : '暂无策略明细' }} /> : null}
            {view !== 'overview' ? <Table<DailyReviewStock>
              size="small" rowKey="symbol" loading={loading} dataSource={visibleStocks} columns={stockColumns}
              sortDirections={['ascend', 'descend']} pagination={{ pageSize: 50, showSizeChanger: false, showTotal: (total) => `共 ${total} 只` }} scroll={{ x: 1540 }}
              expandable={{ expandedRowRender: (stock) => <div className="daily-review-signal-detail">{stock.signals.map((signal) => { const detail = parseJSON<{ rank?: number; strategy_score?: number; reasons?: string[]; rank_reason?: string }>(signal.detail_json, {}); return <div key={signal.id}><strong>{signal.strategy_name}</strong><span>第 {detail.rank || '--'} 名 · 统一分 {signal.score.toFixed(1)} · 策略分 {Number(detail.strategy_score || 0).toFixed(1)}</span><span>{detail.rank_reason || detail.reasons?.join('；') || '无附加说明'}</span></div>; })}</div> }}
              locale={{ emptyText: view === 'consensus' ? '当天没有多策略共振股票' : '当天没有选股信号' }}
            /> : null}
          </>}
        </Card>
      </div>
      <Modal open={Boolean(noteTarget)} title={`${noteTarget ? `${stockName(noteTarget.symbol) || noteTarget.symbol} ` : ''}复盘备注`} okText="保存" cancelText="取消" confirmLoading={noteSaving} onOk={() => void saveNote()} onCancel={() => setNoteTarget(null)}>
        <TextArea rows={6} maxLength={1000} showCount placeholder="记录关注理由、风险点或后续计划" value={noteText} onChange={(event) => setNoteText(event.target.value)} />
      </Modal>
      </> : null}
      <StockQuoteModal target={quoteTarget} onClose={() => setQuoteTarget(null)} />
    </div>
  );
}
