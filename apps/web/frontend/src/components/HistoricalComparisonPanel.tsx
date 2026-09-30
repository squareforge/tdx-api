import {
  Alert, Button, Card, Checkbox, Drawer, Empty, InputNumber, Progress, Select, Space, Table, Tag, Typography, message
} from 'antd';
import { DownloadOutlined, ExperimentOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import type { HistoricalBacktestRun, HistoricalComparison, HistoricalComparisonEvent, HistoricalComparisonEventPage } from '../types';

const { Text } = Typography;

interface Props { run: HistoricalBacktestRun; }
interface SummaryRow {
  completed?: number; signal_average?: number; signal_median?: number; control_average?: number; control_median?: number;
  paired_average?: number; paired_median?: number; paired_win_rate?: number; signal_target_rate?: number;
  control_target_rate?: number; signal_mfe?: number; control_mfe?: number; signal_mae?: number; control_mae?: number;
  confidence_low?: number; confidence_high?: number; benchmark_average?: Record<string, number>;
}
interface Summary {
  event_count?: number; completed_events?: number; partial_events?: number; unavailable_events?: number;
  base_changed_events?: number; control_count?: number; candidate_symbols?: number; warnings?: string[];
  horizons?: Record<string, SummaryRow>; years?: Record<string, Record<string, SummaryRow>>;
}
interface EventResult { stored_signal_base?: number; current_signal_base?: number; base_changed?: boolean; candidate_count?: number; controls?: Array<{ symbol: string }>; horizons?: Record<string, { status?: string; signal_return?: number; control_return?: number; paired_excess?: number; signal_mfe?: number; control_mfe?: number; signal_mae?: number; control_mae?: number; benchmark_returns?: Record<string, number> }>; }

const benchmarkNames: Record<string, string> = { sh000001: '上证指数', sh000300: '沪深300', sh000905: '中证500', sh000852: '中证1000' };

function parseJSON<T>(value?: string, fallback?: T): T {
  try { const parsed = JSON.parse(value || '') as T | null; return parsed == null ? fallback as T : parsed; } catch { return fallback as T; }
}
function pct(value?: number) { return Number.isFinite(value) ? `${Number(value) >= 0 ? '+' : ''}${Number(value).toFixed(2)}%` : '--'; }
function color(value?: number) { return Number(value || 0) >= 0 ? 'trading-positive' : 'trading-negative'; }
function statusTag(status?: string) { if (status === 'completed') return <Tag color="success">已完成</Tag>; if (status === 'failed') return <Tag color="error">失败</Tag>; if (status === 'cancelled') return <Tag>已取消</Tag>; return <Tag color="processing">分析中</Tag>; }

export function HistoricalComparisonPanel({ run }: Props) {
  const [items, setItems] = useState<HistoricalComparison[]>([]);
  const [selected, setSelected] = useState<HistoricalComparison | null>(null);
  const [events, setEvents] = useState<HistoricalComparisonEventPage>({ items: [], total: 0, limit: 50, offset: 0 });
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [selectedEvent, setSelectedEvent] = useState<HistoricalComparisonEvent | null>(null);
  const [launchOpen, setLaunchOpen] = useState(false);
  const [starting, setStarting] = useState(false);
  const [sampleSize, setSampleSize] = useState(5);
  const [targetReturn, setTargetReturn] = useState(1);
  const [drawdownLimit, setDrawdownLimit] = useState(0);
  const [mode, setMode] = useState<'strategy' | 'all_strategies'>('strategy');
  const [strategyID, setStrategyID] = useState(run.strategy_ids?.[0] || '');
  const [seed, setSeed] = useState<number | null>(null);

  const loadItems = useCallback(async (preferID?: string) => {
    try {
      const response = await apiFetch<{ items: HistoricalComparison[] }>(`/api/historical-backtests/${encodeURIComponent(run.id)}/comparisons`);
      setItems(response.items || []);
      setSelected(current => {
        const id = preferID || current?.id;
        return (id && response.items.find(item => item.id === id)) || response.items[0] || null;
      });
    } catch (error) { message.error(error instanceof Error ? error.message : '对照分析记录加载失败'); }
  }, [run.id]);

  const loadEvents = useCallback(async (comparison: HistoricalComparison) => {
    try { setEvents(await apiFetch<HistoricalComparisonEventPage>(`/api/historical-backtests/${run.id}/comparisons/${comparison.id}/events?limit=100`)); }
    catch (error) { message.error(error instanceof Error ? error.message : '对照明细加载失败'); }
  }, [run.id]);

  useEffect(() => { void loadItems(); }, [loadItems]);
  useEffect(() => { if (selected) void loadEvents(selected); }, [loadEvents, selected]);
  useEffect(() => {
    if (!selected || !['queued', 'running'].includes(selected.status)) return undefined;
    const timer = window.setInterval(() => { void loadItems(selected.id); }, 2500);
    return () => window.clearInterval(timer);
  }, [loadItems, selected]);

  const summary = useMemo(() => parseJSON<Summary>(selected?.summary_json, {}), [selected?.summary_json]);
  const eventResult = useMemo(() => parseJSON<EventResult>(selectedEvent?.result_json, {}), [selectedEvent?.result_json]);
  const config = useMemo(() => parseJSON<{ strategy_id?: string; mode?: string; control_sample_size?: number; target_return?: number; seed?: number }>(selected?.config_json, {}), [selected?.config_json]);
  const horizonRows = useMemo(() => Object.entries(summary.horizons || {}).sort(([a], [b]) => Number(a.slice(1)) - Number(b.slice(1))).map(([key, row]) => ({ key, ...row })), [summary.horizons]);
  const years = useMemo(() => Object.keys(summary.years || {}).sort(), [summary.years]);

  async function start() {
    if (mode === 'strategy' && !strategyID) { message.warning('请选择分析策略'); return; }
    setStarting(true);
    try {
      const item = await apiFetch<HistoricalComparison>(`/api/historical-backtests/${run.id}/comparisons`, { method: 'POST', body: JSON.stringify({
        strategy_id: mode === 'strategy' ? strategyID : '', mode, control_sample_size: sampleSize, target_return: targetReturn,
        drawdown_limit: drawdownLimit, benchmark_ids: Object.keys(benchmarkNames), seed: seed || 0
      }) });
      setLaunchOpen(false); await loadItems(item.id); message.success('对照分析已开始');
    } catch (error) { message.error(error instanceof Error ? error.message : '对照分析启动失败'); }
    finally { setStarting(false); }
  }

  async function cancel() {
    if (!selected) return;
    try { await apiFetch(`/api/historical-backtests/${run.id}/comparisons/${selected.id}/cancel`, { method: 'POST' }); await loadItems(selected.id); message.info('已提交取消请求'); }
    catch (error) { message.error(error instanceof Error ? error.message : '取消失败'); }
  }

  async function resample() {
    if (!selected) return;
    try {
      const item = await apiFetch<HistoricalComparison>(`/api/historical-backtests/${run.id}/comparisons/${selected.id}/resample`, { method: 'POST' });
      await loadItems(item.id);
      message.success('已创建新的抽样分析');
    } catch (error) { message.error(error instanceof Error ? error.message : '重新抽样失败'); }
  }

  const columns = [
    { title: '信号日期', dataIndex: 'signal_date', width: 110 },
    { title: '策略股票', dataIndex: 'signal_symbol', width: 100 },
    { title: '对照数', dataIndex: 'control_count', width: 80 },
    { title: '状态', dataIndex: 'status', width: 90, render: (value: string) => value === 'complete' ? <Tag color="success">完成</Tag> : <Tag color="warning">{value === 'partial' ? '部分可用' : '不可用'}</Tag> },
    { title: '说明', dataIndex: 'reason', ellipsis: true },
    { title: '操作', key: 'action', width: 80, render: (_: unknown, row: HistoricalComparisonEvent) => <Button size="small" onClick={() => { setSelectedEvent(row); setDrawerOpen(true); }}>查看</Button> }
  ];

  return <div className="historical-comparison-panel">
    <div className="historical-result-toolbar">
      <Space wrap>
        <Button type="primary" icon={<ExperimentOutlined />} onClick={() => setLaunchOpen(true)} disabled={items.some(item => ['queued', 'running'].includes(item.status))}>新建对照分析</Button>
        <Button icon={<ReloadOutlined />} onClick={() => void loadItems(selected?.id)}>刷新</Button>
        {selected?.status === 'completed' ? <Button icon={<DownloadOutlined />} onClick={() => { window.location.href = `/api/historical-backtests/${run.id}/comparisons/${selected.id}/export`; }}>导出 CSV</Button> : null}
        {selected?.status === 'completed' ? <Button onClick={() => void resample()}>重新抽样</Button> : null}
        {selected && ['queued', 'running'].includes(selected.status) ? <Button danger icon={<StopOutlined />} onClick={() => void cancel()}>取消分析</Button> : null}
      </Space>
      {selected ? <Space wrap>
        <Select size="small" value={selected.id} onChange={(value) => setSelected(items.find(item => item.id === value) || null)} options={items.map(item => ({ value: item.id, label: `${item.created_at} · ${item.status} · 种子 ${item.seed}` }))} />
        {statusTag(selected.status)}<Text type="secondary">{selected.processed_events}/{selected.total_events || '--'} 个事件</Text>
      </Space> : null}
    </div>
    {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未生成同日对照。对照分析只用于回测研究，不影响实盘选股。" /> : <>
      {['queued', 'running'].includes(selected.status) ? <Progress percent={selected.total_events ? Math.round(selected.processed_events * 100 / selected.total_events) : undefined} status="active" /> : null}
      {selected.error ? <Alert type="error" showIcon message={selected.error} /> : null}
      {summary.warnings?.map(warning => <Alert key={warning} type="warning" showIcon message={warning} />)}
      <div className="historical-facts">
        <div><span>信号事件</span><strong>{summary.event_count || selected.total_events || 0}</strong><small>完成 {summary.completed_events || 0} · 部分 {summary.partial_events || 0}</small></div>
        <div><span>对照股票</span><strong>{summary.control_count || 0}</strong><small>每条默认 {config.control_sample_size || sampleSize} 只</small></div>
        <div><span>达标阈值</span><strong>{config.target_return ?? 1}%</strong><small>基准：信号日收盘</small></div>
        <div><span>抽样种子</span><strong>{selected.seed}</strong><small>{selected.algorithm_version}</small></div>
      </div>
      {horizonRows.length ? <Table size="small" rowKey="key" pagination={false} scroll={{ x: 1150 }} dataSource={horizonRows} columns={[
        { title: '周期', dataIndex: 'key', width: 70 },
        { title: '策略平均', dataIndex: 'signal_average', render: (v: number) => <span className={color(v)}>{pct(v)}</span> },
        { title: '对照平均', dataIndex: 'control_average', render: (v: number) => pct(v) },
        { title: '配对差值', dataIndex: 'paired_average', render: (v: number) => <strong className={color(v)}>{pct(v)}</strong> },
        { title: '策略中位', dataIndex: 'signal_median', render: (v: number) => pct(v) },
        { title: '对照中位', dataIndex: 'control_median', render: (v: number) => pct(v) },
        { title: '达标率', key: 'target', render: (_: unknown, row: SummaryRow) => `${Number(row.signal_target_rate || 0).toFixed(1)}% / ${Number(row.control_target_rate || 0).toFixed(1)}%` },
        { title: 'MFE', key: 'mfe', render: (_: unknown, row: SummaryRow) => `${pct(row.signal_mfe)} / ${pct(row.control_mfe)}` },
        { title: 'MAE', key: 'mae', render: (_: unknown, row: SummaryRow) => `${pct(row.signal_mae)} / ${pct(row.control_mae)}` },
        { title: '95%区间', key: 'ci', render: (_: unknown, row: SummaryRow) => `${pct(row.confidence_low)} ~ ${pct(row.confidence_high)}` }
      ]} /> : null}
      {years.length ? <Card size="small" title="按年份拆分" className="historical-comparison-year-card">
        <Table size="small" rowKey="year" pagination={false} scroll={{ x: 1050 }} dataSource={years.map(year => ({ year, ...summary.years?.[year] }))} columns={[
          { title: '年份', dataIndex: 'year', fixed: 'left' as const, width: 90 },
          { title: 'D1 配对差值', key: 'd1', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => pct(row.d1?.paired_average) },
          { title: 'D3 配对差值', key: 'd3', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => pct(row.d3?.paired_average) },
          { title: 'D5 配对差值', key: 'd5', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => pct(row.d5?.paired_average) },
          { title: 'D10 配对差值', key: 'd10', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => pct(row.d10?.paired_average) },
          { title: 'D5 达标率差', key: 'd5hit', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => `${Number(row.d5?.signal_target_rate || 0).toFixed(1)}% / ${Number(row.d5?.control_target_rate || 0).toFixed(1)}%` },
          { title: 'D5 信号样本', key: 'n', render: (_: unknown, row: { year: string } & Record<string, SummaryRow>) => row.d5?.completed || '--' }
        ]} />
      </Card> : null}
      {selected.status === 'completed' ? <Table size="small" rowKey="id" dataSource={events.items} columns={columns} pagination={{ pageSize: 100, total: events.total, showSizeChanger: false }} scroll={{ x: 900 }} /> : null}
    </>}
    <Drawer title="新建对照分析" open={launchOpen} onClose={() => setLaunchOpen(false)} width={420}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <label>分析模式<Select style={{ width: '100%', marginTop: 8 }} value={mode} onChange={setMode} options={[{ value: 'strategy', label: '单策略' }, { value: 'all_strategies', label: '全部策略合并' }]} /></label>
        {mode === 'strategy' ? <label>分析策略<Select style={{ width: '100%', marginTop: 8 }} value={strategyID} onChange={setStrategyID} options={run.strategy_ids.map(id => ({ value: id, label: id }))} /></label> : null}
        <label>每条信号对照数量<InputNumber style={{ width: '100%', marginTop: 8 }} min={1} max={20} value={sampleSize} onChange={(v) => setSampleSize(v || 5)} /></label>
        <label>收盘达标阈值（%）<InputNumber style={{ width: '100%', marginTop: 8 }} min={0} max={100} step={0.5} value={targetReturn} onChange={(v) => setTargetReturn(v ?? 1)} /></label>
        <label>最大不利波动限制（%，0 表示不启用）<InputNumber style={{ width: '100%', marginTop: 8 }} min={0} max={100} step={0.5} value={drawdownLimit} onChange={(v) => setDrawdownLimit(v ?? 0)} /></label>
        <label>随机种子（留空自动生成）<InputNumber style={{ width: '100%', marginTop: 8 }} value={seed ?? undefined} onChange={(v) => setSeed(v ?? null)} /></label>
        <Checkbox checked disabled>上证、沪深300、中证500、中证1000</Checkbox>
        <Alert type="info" showIcon message={`预计规模：每条信号 ${sampleSize} 只对照股票；不会加入交易记录。`} />
        <Button type="primary" block loading={starting} onClick={() => void start()}>开始分析</Button>
      </Space>
    </Drawer>
    <Drawer title={selectedEvent ? `${selectedEvent.signal_date} · ${selectedEvent.signal_symbol}` : '对照事件详情'} open={drawerOpen} onClose={() => setDrawerOpen(false)} width={720}>
      {selectedEvent ? <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Alert type={selectedEvent.status === 'complete' ? 'success' : 'warning'} showIcon message={selectedEvent.reason || '信号和对照行情已完成计算'} />
        <div className="historical-facts">
          <div><span>策略信号基准</span><strong>{Number(eventResult.stored_signal_base || 0).toFixed(3)}</strong></div>
          <div><span>当前行情基准</span><strong>{Number(eventResult.current_signal_base || 0).toFixed(3)}</strong><small>{eventResult.base_changed ? '与原回测不同' : '与原回测相同'}</small></div>
          <div><span>候选股票</span><strong>{eventResult.candidate_count || 0}</strong></div>
          <div><span>对照数量</span><strong>{eventResult.controls?.length || 0}</strong></div>
        </div>
        <Table size="small" rowKey="key" pagination={false} dataSource={Object.entries(eventResult.horizons || {}).map(([key, row]) => ({ key, ...row }))} columns={[
          { title: '周期', dataIndex: 'key' }, { title: '策略收益', dataIndex: 'signal_return', render: (v: number) => pct(v) },
          { title: '对照收益', dataIndex: 'control_return', render: (v: number) => pct(v) },
          { title: '配对差值', dataIndex: 'paired_excess', render: (v: number) => <strong className={color(v)}>{pct(v)}</strong> },
          { title: '指数收益', dataIndex: 'benchmark_returns', render: (v: Record<string, number>) => Object.entries(v || {}).map(([id, value]) => `${benchmarkNames[id]} ${pct(value)}`).join(' · ') || '--' }
        ]} scroll={{ x: 650 }} />
        <Text strong>同日对照股票</Text>
        <Space wrap>{(eventResult.controls || []).map(item => <Tag key={item.symbol}>{item.symbol}</Tag>)}</Space>
        <Text type="secondary">收益按信号日收盘基准、前复权日线计算。当前股票池可能不等于历史成分，行情源可能包含服务回退。</Text>
      </Space> : <Empty description="选择一条事件查看详情" />}
    </Drawer>
  </div>;
}
