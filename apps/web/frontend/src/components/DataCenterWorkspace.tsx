import { Button, Card, Checkbox, Form, Input, InputNumber, Modal, Progress, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd';
import { CheckCircleOutlined, DatabaseOutlined, ReloadOutlined, SearchOutlined, WarningOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useState } from 'react';
import { apiFetch } from '../lib/api';
import { DynamicTable } from './DynamicTable';
import { JsonPane } from './JsonPane';

const { Text } = Typography;

function rowsFrom(value: unknown): Array<Record<string, unknown>> {
  if (Array.isArray(value)) return value as Array<Record<string, unknown>>;
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    if (Array.isArray(record.codes)) return record.codes as Array<Record<string, unknown>>;
    if (Array.isArray(record.list)) return record.list.map((item) => (typeof item === 'object' ? item as Record<string, unknown> : { value: item }));
    if (Array.isArray(record.List)) return record.List as Array<Record<string, unknown>>;
  }
  return [];
}

function directoryRowKey(record: Record<string, unknown>) {
  return String(record.code || record.Code || record.value || record.name || record.Name || JSON.stringify(record));
}

const SYNC_SCOPE_ITEMS = [
  { key: 'day', label: '日线' },
  { key: 'min', label: '1分钟线' },
  { key: 'min5', label: '5分钟线' },
  { key: 'finance', label: '财务' },
  { key: 'block', label: '板块' },
  { key: 'weight', label: '权重' }
];

function taskScopeKeys(task: Record<string, unknown>): string[] {
  const request = (task.request || {}) as Record<string, unknown>;
  return SYNC_SCOPE_ITEMS.filter((item) => request[item.key]).map((item) => item.key);
}

export function DataCenterWorkspace() {
  const [overview, setOverview] = useState<Record<string, unknown>>({});
  const [directoryRows, setDirectoryRows] = useState<Array<Record<string, unknown>>>([]);
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const [exchange, setExchange] = useState('all');
  const [limit, setLimit] = useState(80);
  const [profileCode, setProfileCode] = useState('000001');
  const [profileOutput, setProfileOutput] = useState<unknown>('尚未查询');
  const [historyOutput, setHistoryOutput] = useState<unknown>('尚未查询');
  const [historyCode, setHistoryCode] = useState('000001');
  const [historyType, setHistoryType] = useState('day');
  const [historyLimit, setHistoryLimit] = useState(120);
  const [blockOutput, setBlockOutput] = useState<unknown>('尚未查询');
  const [blockFile, setBlockFile] = useState('gn');
  const [blockWithIndex, setBlockWithIndex] = useState(true);
  const [hikyuuStatus, setHikyuuStatus] = useState<unknown>('尚未查询');
  const [hikyuuMetadata, setHikyuuMetadata] = useState<Record<string, unknown>>({});
  const [hikyuuQuality, setHikyuuQuality] = useState<Record<string, unknown>>({});
  const [syncModalOpen, setSyncModalOpen] = useState(false);
  const [syncScope, setSyncScope] = useState<Record<string, boolean>>({ day: true, min: false, min5: false, finance: true, block: true, weight: true });
  const [syncTasks, setSyncTasks] = useState<Array<Record<string, unknown>>>([]);
  const [calendarSummary, setCalendarSummary] = useState<Record<string, unknown>>({});
  const [calendarRows, setCalendarRows] = useState<Array<Record<string, unknown>>>([]);
  const [calendarOutput, setCalendarOutput] = useState<unknown>('尚未同步');
  const [calendarLoading, setCalendarLoading] = useState(false);
  const syncingKeys = new Set(syncTasks.flatMap((task) => taskScopeKeys(task)));
  const syncingLabels = SYNC_SCOPE_ITEMS.filter((item) => syncingKeys.has(item.key)).map((item) => item.label);

  const loadOverview = useCallback(async () => {
    try {
      const [status, stats, count] = await Promise.allSettled([
        apiFetch<Record<string, unknown>>('/api/server-status'),
        apiFetch<Record<string, unknown>>('/api/market-stats'),
        apiFetch<Record<string, unknown>>('/api/market-count')
      ]);
      setOverview({
        status: status.status === 'fulfilled' ? status.value : {},
        stats: stats.status === 'fulfilled' ? stats.value : {},
        count: count.status === 'fulfilled' ? count.value : {}
      });
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '市场总览加载失败');
    }
  }, []);

  const loadDirectory = useCallback(async (kind: 'codes' | 'etf' = 'codes') => {
    setDirectoryLoading(true);
    try {
      const url =
        kind === 'codes'
          ? `/api/codes?exchange=${encodeURIComponent(exchange)}`
          : `/api/etf?exchange=${encodeURIComponent(exchange)}&limit=${encodeURIComponent(limit)}`;
      const data = await apiFetch<unknown>(url);
      setDirectoryRows(rowsFrom(data).slice(0, limit));
    } catch (error) {
      message.error(error instanceof Error ? error.message : '代码目录加载失败');
    } finally {
      setDirectoryLoading(false);
    }
  }, [exchange, limit]);

  useEffect(() => {
    loadOverview();
    loadDirectory();
  }, []);

  const loadTradeCalendar = useCallback(async () => {
    setCalendarLoading(true);
    try {
      const [summary, range] = await Promise.all([
        apiFetch<Record<string, unknown>>('/api/trade-calendar'),
        apiFetch<{ list?: Array<Record<string, unknown>> }>('/api/trade-calendar/range')
      ]);
      setCalendarSummary(summary);
      setCalendarRows(range.list || []);
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '交易日历加载失败');
    } finally {
      setCalendarLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadTradeCalendar();
  }, [loadTradeCalendar]);

  async function loadProfile() {
    try {
      const [info, finance] = await Promise.allSettled([
        apiFetch(`/api/stock-info?code=${encodeURIComponent(profileCode)}`),
        apiFetch(`/api/finance?code=${encodeURIComponent(profileCode)}`)
      ]);
      setProfileOutput({ info: info.status === 'fulfilled' ? info.value : String(info.reason), finance: finance.status === 'fulfilled' ? finance.value : String(finance.reason) });
    } catch (error) {
      setProfileOutput(error instanceof Error ? error.message : '查询失败');
    }
  }

  async function loadHistory(mode: 'tdx' | 'ths' | 'history' | 'trade' | 'minuteTrade') {
    try {
      const query = `code=${encodeURIComponent(historyCode)}&type=${encodeURIComponent(historyType)}&limit=${encodeURIComponent(historyLimit)}`;
      const path =
        mode === 'tdx'
          ? `/api/kline-all/tdx?${query}`
          : mode === 'ths'
            ? `/api/kline-all/ths?${query}`
            : mode === 'history'
              ? `/api/kline-history?${query}`
              : mode === 'trade'
                ? `/api/trade-history?code=${encodeURIComponent(historyCode)}`
                : `/api/minute-trade-all?code=${encodeURIComponent(historyCode)}`;
      setHistoryOutput(await apiFetch(path));
    } catch (error) {
      setHistoryOutput(error instanceof Error ? error.message : '加载失败');
    }
  }

  async function loadBlock(mode: 'block' | 'hy' | 'stat' | 'stat2' | 'xgsg') {
    try {
      const path =
        mode === 'block'
          ? `/api/block?file=${encodeURIComponent(blockFile)}&with_index=${blockWithIndex ? '1' : '0'}`
          : mode === 'hy'
            ? '/api/tdx-hy'
            : mode === 'stat'
              ? '/api/tdx-stat'
              : mode === 'stat2'
                ? '/api/tdx-stat2'
                : '/api/xgsg';
      setBlockOutput(await apiFetch(path));
    } catch (error) {
      setBlockOutput(error instanceof Error ? error.message : '查询失败');
    }
  }

  async function loadHikyuu(path = '/api/hikyuu/health', options: RequestInit = {}) {
    try {
      setHikyuuStatus(await apiFetch(path, options));
    } catch (error) {
      setHikyuuStatus(error instanceof Error ? error.message : '数据服务不可用');
    }
  }

  function openSyncModal() {
    // 正在同步的数据项自动取消勾选，避免重复提交后被后端拒绝
    setSyncScope((prev) => {
      const next = { ...prev };
      for (const task of syncTasks) {
        for (const key of taskScopeKeys(task)) next[key] = false;
      }
      return next;
    });
    setSyncModalOpen(true);
    void refreshSyncTasks();
  }

  async function startFullSync() {
    const payload: Record<string, unknown> = { use_tdx_number: 20 };
    const selected: string[] = [];
    for (const item of SYNC_SCOPE_ITEMS) {
      const enabled = Boolean(syncScope[item.key]) && !syncingKeys.has(item.key);
      payload[item.key] = enabled;
      if (enabled) selected.push(item.label);
    }
    if (!selected.length) {
      message.warning('请至少勾选一项需要同步的数据');
      return;
    }
    setSyncModalOpen(false);
    try {
      await apiFetch('/api/hikyuu/tasks/full-sync', { method: 'POST', body: JSON.stringify(payload) });
      message.success(syncTasks.length ? `已新增并行同步任务：${selected.join('、')}` : `已开始同步：${selected.join('、')}`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '同步任务创建失败');
    }
    void refreshSyncTasks();
    void loadHikyuu('/api/hikyuu/tasks');
  }

  async function syncTradeCalendar(mode: 'future' | 'confirm' | 'all') {
    setCalendarLoading(true);
    try {
      const result = await apiFetch('/api/trade-calendar/sync', {
        method: 'POST',
        body: JSON.stringify({ mode, years: 2 })
      });
      setCalendarOutput(result);
      message.success(mode === 'future' ? '未来交易日历已同步' : mode === 'confirm' ? '交易日历已校验' : '交易日历同步完成');
      await loadTradeCalendar();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '交易日历同步失败');
      setCalendarOutput(error instanceof Error ? error.message : '交易日历同步失败');
    } finally {
      setCalendarLoading(false);
    }
  }

  const refreshSyncTasks = useCallback(async () => {
    try {
      const tasks = await apiFetch<Array<Record<string, unknown>>>('/api/hikyuu/tasks');
      setSyncTasks((tasks || []).filter((item) => item.status === 'running' || item.status === 'pending'));
    } catch {
      // 同步任务轮询失败时静默,下次再试
    }
  }, []);

  useEffect(() => {
    void refreshSyncTasks();
    const timer = window.setInterval(() => void refreshSyncTasks(), 3000);
    return () => window.clearInterval(timer);
  }, [refreshSyncTasks]);

  const loadHikyuuResearch = useCallback(async () => {
    try {
      const [health, metadata, quality] = await Promise.all([
        apiFetch<Record<string, unknown>>('/api/hikyuu/health'),
        apiFetch<Record<string, unknown>>('/api/hikyuu/metadata'),
        apiFetch<Record<string, unknown>>(`/api/hikyuu/quality?code=${encodeURIComponent(historyCode)}&period=${encodeURIComponent(historyType)}`)
      ]);
      setHikyuuStatus(health);
      setHikyuuMetadata(metadata);
      setHikyuuQuality(quality);
    } catch (error) {
      setHikyuuStatus(error instanceof Error ? error.message : '数据服务不可用');
    }
  }, [historyCode, historyType]);

  useEffect(() => { void loadHikyuuResearch(); }, [loadHikyuuResearch]);

  const status = overview.status as Record<string, unknown> | undefined;
  const count = overview.count as Record<string, unknown> | undefined;
  const stats = overview.stats as Record<string, unknown> | undefined;
  const stockMetadata = hikyuuMetadata.stocks as { files?: Array<Record<string, unknown>>; symbols?: number } | undefined;
  const qualityChecks = Array.isArray(hikyuuQuality.checks) ? hikyuuQuality.checks as Array<Record<string, unknown>> : [];

  return (
    <div className="data-center-grid">
      <Card className="work-card" title="市场总览" extra={<Button icon={<ReloadOutlined />} onClick={loadOverview}>刷新</Button>}>
        <div className="metric-strip">
          <Statistic title="服务状态" value={String(status?.status || '--')} />
          <Statistic title="市场证券数" value={Number(count?.total || 0)} />
          <Statistic title="沪市股票" value={Number((stats?.sh as Record<string, unknown> | undefined)?.total || 0)} />
          <Statistic title="深市股票" value={Number((stats?.sz as Record<string, unknown> | undefined)?.total || 0)} />
        </div>
        <Space wrap className="toolbar-row">
          <Select value={exchange} onChange={setExchange} options={[
            { value: 'all', label: '全部市场' },
            { value: 'sh', label: '沪市' },
            { value: 'sz', label: '深市' },
            { value: 'bj', label: '北交所' }
          ]} />
          <InputNumber min={1} max={500} value={limit} onChange={(value) => setLimit(Number(value || 80))} />
          <Button type="primary" icon={<DatabaseOutlined />} loading={directoryLoading} onClick={() => loadDirectory('codes')}>代码目录</Button>
          <Button onClick={() => loadDirectory('etf')}>ETF</Button>
        </Space>
        <Table
          size="small"
          rowKey={directoryRowKey}
          loading={directoryLoading}
          dataSource={directoryRows}
          pagination={{ pageSize: 12, size: 'small' }}
          columns={[
            { title: '代码', dataIndex: 'code', render: (value, record) => value || record.Code || record.value },
            { title: '名称', dataIndex: 'name', render: (value, record) => value || record.Name || '--' },
            { title: '市场', dataIndex: 'exchange', render: (value, record) => value || record.Exchange || '--' }
          ]}
        />
      </Card>

      <Card className="work-card hikyuu-research-card" title="Hikyuu 研究数据" extra={<Button icon={<ReloadOutlined />} onClick={loadHikyuuResearch}>刷新</Button>}>
        <div className="hikyuu-research-strip">
          <div><span>框架版本</span><strong>{String(hikyuuMetadata.hikyuu_version || '--')}</strong><small>固定运行版本</small></div>
          <div><span>数据修订</span><strong>{String(hikyuuMetadata.data_revision || '--')}</strong><small>回测可追溯</small></div>
          <div><span>数据文件</span><strong>{stockMetadata?.files?.length || 0}</strong><small>HDF5 / SQLite</small></div>
          <div className={hikyuuQuality.status === 'pass' ? 'is-pass' : 'is-warn'}>
            {hikyuuQuality.status === 'pass' ? <CheckCircleOutlined /> : <WarningOutlined />}
            <span>质量状态</span><strong>{hikyuuQuality.status === 'pass' ? '通过' : '待检查'}</strong>
          </div>
        </div>
        <div className="hikyuu-quality-list">
          {qualityChecks.map((check) => (
            <Tag key={String(check.id)} color={check.status === 'pass' ? 'success' : 'warning'}>
              {String(check.label)} · {String(check.detail)}
            </Tag>
          ))}
        </div>
        <Space wrap className="toolbar-row">
          <Button type="primary" onClick={openSyncModal}>全量同步</Button>
          <Button onClick={() => loadHikyuu('/api/hikyuu/tasks/after-close-sync', { method: 'POST', body: '{}' })}>盘后同步</Button>
          <Button onClick={() => loadHikyuu('/api/hikyuu/tasks')}>刷新任务</Button>
        </Space>
        {syncTasks.map((task) => {
          const request = (task.request || {}) as Record<string, unknown>;
          const scopeText = SYNC_SCOPE_ITEMS.filter((item) => request[item.key]).map((item) => item.label).join('、') || '同步';
          return (
            <div className="hikyuu-sync-progress" key={String(task.id)}>
              <div className="hikyuu-sync-progress-head">
                <span>同步中 · {scopeText}</span>
                <strong>{task.progress != null ? `${task.progress}%` : '--'}</strong>
                <small>{String(task.stage || '')}</small>
              </div>
              <Progress percent={Number(task.progress ?? 0)} status="active" size="small" />
              <div className="hikyuu-sync-phases">
                {Object.entries((task.phases || {}) as Record<string, { label: string; progress: number | null; done: boolean }>).map(([key, phase]) => (
                  <div key={key} className="hikyuu-sync-phase">
                    <span>{phase.label}</span>
                    {phase.done ? <Tag color="success">完成</Tag> : phase.progress != null ? <Progress percent={phase.progress} size="small" /> : <Tag>等待</Tag>}
                  </div>
                ))}
              </div>
            </div>
          );
        })}
        <JsonPane value={hikyuuStatus} />
      </Card>

      <Card className="work-card" title="交易日历" extra={<Button icon={<ReloadOutlined />} loading={calendarLoading} onClick={loadTradeCalendar}>刷新</Button>}>
        <div className="metric-strip">
          <Statistic title="日历范围" value={`${String(calendarSummary.start_date || '--')} ~ ${String(calendarSummary.end_date || '--')}`} />
          <Statistic title="交易日" value={Number(calendarSummary.trade_days || 0)} />
          <Statistic title="预测自然日" value={Number(calendarSummary.predicted_days || 0)} />
          <Statistic title="确认自然日" value={Number(calendarSummary.confirmed_days || 0)} />
        </div>
        <Space wrap className="toolbar-row">
          <Tag color="blue">下一交易日：{String(calendarSummary.next_trade_day || '--')}</Tag>
          <Tag>确认至：{String(calendarSummary.confirmed_through || '--')}</Tag>
          <Button type="primary" loading={calendarLoading} onClick={() => syncTradeCalendar('future')}>同步未来2年</Button>
          <Button loading={calendarLoading} onClick={() => syncTradeCalendar('confirm')}>TDX校验</Button>
          <Button loading={calendarLoading} onClick={() => syncTradeCalendar('all')}>一键同步</Button>
        </Space>
        <DynamicTable rows={calendarRows} scrollY={260} />
        {!calendarRows.length ? <JsonPane value={calendarOutput} /> : null}
      </Card>

      <Card className="work-card" title="个股资料" extra={<Button icon={<SearchOutlined />} onClick={loadProfile}>查询</Button>}>
        <Space wrap className="toolbar-row">
          <Input value={profileCode} onChange={(event) => setProfileCode(event.target.value)} />
          <Button type="primary" onClick={loadProfile}>财务/F10/股本</Button>
        </Space>
        <JsonPane value={profileOutput} />
      </Card>

      <Card className="work-card" title="历史数据">
        <Form layout="inline" className="chart-control-form">
          <Form.Item label="代码"><Input value={historyCode} onChange={(event) => setHistoryCode(event.target.value)} /></Form.Item>
          <Form.Item label="周期"><Select value={historyType} onChange={setHistoryType} options={[
            { value: 'day', label: '日K' },
            { value: 'week', label: '周K' },
            { value: 'month', label: '月K' },
            { value: 'minute30', label: '30分钟' },
            { value: 'minute5', label: '5分钟' }
          ]} /></Form.Item>
          <Form.Item label="数量"><InputNumber min={1} max={2000} value={historyLimit} onChange={(value) => setHistoryLimit(Number(value || 120))} /></Form.Item>
        </Form>
        <Space wrap className="toolbar-row">
          <Button type="primary" onClick={() => loadHistory('tdx')}>TDX K线</Button>
          <Button type="primary" onClick={() => loadHistory('ths')}>THS前复权</Button>
          <Button onClick={() => loadHistory('history')}>分页K线</Button>
          <Button onClick={() => loadHistory('trade')}>历史成交</Button>
          <Button onClick={() => loadHistory('minuteTrade')}>全天分时成交</Button>
        </Space>
        <DynamicTable rows={rowsFrom(historyOutput)} scrollY={320} />
        {!rowsFrom(historyOutput).length ? <JsonPane value={historyOutput} /> : null}
      </Card>

      <Card className="work-card" title="板块行业">
        <Space wrap className="toolbar-row">
          <Select value={blockFile} onChange={setBlockFile} options={[
            { value: 'gn', label: '概念板块' },
            { value: 'fg', label: '地域风格' },
            { value: 'zs', label: '指数板块' },
            { value: 'hy', label: '行业板块' }
          ]} />
          <Checkbox checked={blockWithIndex} onChange={(event) => setBlockWithIndex(event.target.checked)}>含指数代码</Checkbox>
          <Button type="primary" onClick={() => loadBlock('block')}>板块成分</Button>
          <Button onClick={() => loadBlock('hy')}>行业归属</Button>
          <Button onClick={() => loadBlock('stat')}>个股统计</Button>
          <Button onClick={() => loadBlock('stat2')}>资金流向</Button>
          <Button onClick={() => loadBlock('xgsg')}>新股申购</Button>
        </Space>
        <DynamicTable rows={rowsFrom(blockOutput)} scrollY={320} />
        {!rowsFrom(blockOutput).length ? <JsonPane value={blockOutput} /> : null}
      </Card>
      <Modal
        title="全量同步"
        open={syncModalOpen}
        onOk={startFullSync}
        onCancel={() => setSyncModalOpen(false)}
        okText="开始同步"
        cancelText="取消"
        okButtonProps={{ disabled: !SYNC_SCOPE_ITEMS.some((item) => syncScope[item.key] && !syncingKeys.has(item.key)) }}
      >
        {syncingLabels.length ? (
          <p className="hikyuu-sync-scope-note">正在同步：{syncingLabels.join('、')}（置灰项不可重复勾选，完成后恢复；其他数据提交后会作为并行任务运行）</p>
        ) : null}
        <p>勾选需要同步的数据周期（分钟线数据量大、耗时长，建议需要时再同步）：</p>
        <div className="hikyuu-sync-scope">
          {SYNC_SCOPE_ITEMS.map((item) => (
            <Checkbox
              key={item.key}
              disabled={syncingKeys.has(item.key)}
              checked={syncScope[item.key]}
              onChange={(event) => setSyncScope((prev) => ({ ...prev, [item.key]: event.target.checked }))}
            >
              {item.label}
              {syncingKeys.has(item.key) ? <Tag className="hikyuu-sync-scope-tag" color="processing">同步中</Tag> : null}
            </Checkbox>
          ))}
        </div>
      </Modal>
    </div>
  );
}
