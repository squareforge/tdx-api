import { Alert, Button, Card, Empty, Input, InputNumber, Select, Space, Table, Tag, Typography, message } from 'antd';
import { ExperimentOutlined, ReloadOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { normalizeSymbol } from '../lib/format';
import type { AICredential, AISelectionRankItem, AISelectionResponse, SelectionResult, StockPool } from '../types';
import { AIResearchReport } from './AIResearchReport';

const { Text } = Typography;

interface StockSearchResult {
  code?: string;
  name?: string;
}

interface SelectionCandidate {
  symbol: string;
  name?: string;
  formula_name?: string;
  task_name?: string;
}

function percent(value?: number) {
  return Number.isFinite(value) ? `${(Number(value) * 100).toFixed(1)}%` : '--';
}

function statusTag(status?: string) {
  const labels: Record<string, string> = { candidate: '候选', watch: '观察', exclude: '淘汰' };
  const colors: Record<string, string> = { candidate: 'green', watch: 'gold', exclude: 'red' };
  return <Tag color={colors[status || ''] || 'default'}>{labels[status || ''] || status || '未知'}</Tag>;
}

function hasSameSymbol(left?: string, right?: string) {
  return Boolean(left && right && normalizeSymbol(left) === normalizeSymbol(right));
}

function resolveStockName(item: Pick<AISelectionRankItem, 'symbol' | 'name'>, stockNames: Record<string, string>) {
  const mappedName = stockNames[normalizeSymbol(item.symbol)];
  if (mappedName) return mappedName;
  const modelName = item.name?.trim();
  return modelName && !hasSameSymbol(modelName, item.symbol) ? modelName : item.symbol;
}

async function fetchStockNames(symbols: string[]) {
  const uniqueSymbols = [...new Set(symbols.map(normalizeSymbol).filter(Boolean))];
  const entries = await Promise.all(uniqueSymbols.map(async (symbol) => {
    try {
      const matches = await apiFetch<StockSearchResult[]>(
        `/api/search?keyword=${encodeURIComponent(symbol)}`
      );
      const exact = (Array.isArray(matches) ? matches : []).find((item) => (
        hasSameSymbol(item.code, symbol) && Boolean(item.name?.trim())
      ));
      return exact?.name?.trim() ? [symbol, exact.name.trim()] as const : null;
    } catch {
      return null;
    }
  }));

  return Object.fromEntries(entries.filter((entry): entry is readonly [string, string] => Boolean(entry)));
}

export function AISelectionWorkspace() {
  const [credentials, setCredentials] = useState<AICredential[]>([]);
  const [credentialID, setCredentialID] = useState('');
  const [selectionResults, setSelectionResults] = useState<SelectionResult[]>([]);
  const [pools, setPools] = useState<StockPool[]>([]);
  const [source, setSource] = useState('selection');
  const [poolID, setPoolID] = useState('');
  const [manualSymbols, setManualSymbols] = useState('');
  const [candidateLimit, setCandidateLimit] = useState(10);
  const [fastPeriod, setFastPeriod] = useState(5);
  const [slowPeriod, setSlowPeriod] = useState(20);
  const [historyCount, setHistoryCount] = useState(520);
  const [loading, setLoading] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [response, setResponse] = useState<AISelectionResponse | null>(null);
  const [stockNames, setStockNames] = useState<Record<string, string>>({});

  async function loadSources() {
    setPreparing(true);
    try {
      const [models, hits, stockPools] = await Promise.all([
        apiFetch<AICredential[]>('/api/ai/credentials'),
        apiFetch<SelectionResult[]>('/api/selection-results?latest=1&limit=100'),
        apiFetch<StockPool[]>('/api/stock-pools')
      ]);
      setCredentials(models || []);
      setSelectionResults(hits || []);
      setPools(stockPools || []);
      const enabled = (models || []).find((item) => item.enabled !== false);
      setCredentialID((current) => current || enabled?.id || '');
      setPoolID((current) => current || stockPools?.[0]?.id || '');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '候选来源加载失败');
    } finally {
      setPreparing(false);
    }
  }

  useEffect(() => { void loadSources(); }, []);

  const candidates = useMemo(() => {
    const map = new Map<string, SelectionCandidate>();
    if (source === 'selection') {
      for (const item of selectionResults) if (!map.has(item.symbol)) map.set(item.symbol, { symbol: item.symbol, formula_name: item.formula_name, task_name: item.task_name });
    } else if (source === 'pool') {
      for (const symbol of pools.find((item) => item.id === poolID)?.symbols || []) map.set(symbol, { symbol });
    } else {
      for (const symbol of manualSymbols.split(/[\s,，;；]+/).map((item) => item.trim()).filter(Boolean)) map.set(symbol, { symbol });
    }
    return [...map.values()].slice(0, Math.max(1, candidateLimit));
  }, [candidateLimit, manualSymbols, poolID, pools, selectionResults, source]);

  useEffect(() => {
    let cancelled = false;
    void fetchStockNames(candidates.map((item) => item.symbol)).then((names) => {
      if (cancelled || !Object.keys(names).length) return;
      setStockNames((current) => ({ ...current, ...names }));
    });
    return () => {
      cancelled = true;
    };
  }, [candidates]);

  async function runSelection() {
    const credential = credentials.find((item) => item.id === credentialID);
    if (!credential) return message.warning('请先选择已启用的 AI 模型');
    if (candidates.length < 2) return message.warning('至少需要 2 只候选股票才能排序');
    setLoading(true);
    try {
      const result = await apiFetch<AISelectionResponse>('/api/ai/select/rank', {
        method: 'POST',
        body: JSON.stringify({
          provider: credential.provider,
          model: credential.model,
          credential_id: credential.id,
          symbols: candidates.map((item) => item.symbol),
          input: {
            source,
            candidates: candidates.map((item) => {
              const name = stockNames[normalizeSymbol(item.symbol)];
              return name ? { ...item, name } : item;
            }),
            fast_period: fastPeriod,
            slow_period: slowPeriod,
            history_count: historyCount
          },
          options: { max_tokens: 2200 }
        })
      });
      setResponse(result);
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'AI 选股失败');
    } finally {
      setLoading(false);
    }
  }

  const ranking = response?.result?.ranking || [];

  return (
    <div className="ai-selection-workspace">
      <Card className="work-card ai-selection-control" title={<div className="ai-assistant-title"><ExperimentOutlined /><div><strong>AI 选股</strong><Text type="secondary">确定性候选 · Hikyuu 验证 · AI 排序</Text></div></div>} extra={<Button icon={<ReloadOutlined />} loading={preparing} onClick={() => void loadSources()}>刷新候选</Button>}>
        <div className="ai-selection-flow"><div><span>候选</span><strong>{candidates.length}</strong><small>公式或股票池</small></div><div><span>验证</span><strong>MA {fastPeriod}/{slowPeriod}</strong><small>{historyCount} 根日 K</small></div><div><span>输出</span><strong>Top {Math.min(candidates.length, candidateLimit)}</strong><small>相对排序</small></div><div><span>边界</span><strong>非概率</strong><small>历史统计不代表未来</small></div></div>
        <div className="ai-selection-form">
          <label><span>候选来源</span><Select value={source} onChange={setSource} options={[{ value: 'selection', label: '最近选股结果' }, { value: 'pool', label: '股票池' }, { value: 'manual', label: '手动输入' }]} /></label>
          {source === 'pool' ? <label><span>股票池</span><Select value={poolID || undefined} onChange={setPoolID} options={pools.map((item) => ({ value: item.id, label: `${item.name} · ${item.symbols?.length || 0}只` }))} /></label> : null}
          {source === 'manual' ? <label className="ai-selection-manual"><span>股票代码</span><Input value={manualSymbols} onChange={(event) => setManualSymbols(event.target.value)} placeholder="手动输入股票代码，多个代码用逗号分隔" /></label> : null}
          <label><span>候选上限</span><InputNumber min={2} max={20} value={candidateLimit} onChange={(value) => setCandidateLimit(value || 10)} /></label>
          <label><span>快速均线</span><InputNumber min={2} max={60} value={fastPeriod} onChange={(value) => setFastPeriod(value || 5)} /></label>
          <label><span>慢速均线</span><InputNumber min={3} max={120} value={slowPeriod} onChange={(value) => setSlowPeriod(value || 20)} /></label>
          <label><span>历史长度</span><Select value={historyCount} onChange={setHistoryCount} options={[260, 520, 780, 1000].map((value) => ({ value, label: `${value} 根` }))} /></label>
          <label><span>AI 模型</span><Select value={credentialID || undefined} onChange={setCredentialID} options={credentials.map((item) => ({ value: item.id, label: `${item.name || item.provider} · ${item.model || '默认'}`, disabled: item.enabled === false }))} /></label>
          <Button type="primary" icon={<ExperimentOutlined />} loading={loading} onClick={() => void runSelection()}>验证并排序</Button>
        </div>
        <Text type="secondary">第一版使用统一 MA 交叉作为参考验证，不能代替候选公式本身的历史回测。</Text>
      </Card>

      <Card className="work-card ai-selection-result" title="候选排名" extra={response ? <Space><Tag>{response.prompt_version}</Tag><Tag color="blue">数据 {response.data_revision || '未知'}</Tag></Space> : null}>
        {response?.result?.summary ? <Alert className="ai-selection-summary" type="info" showIcon message={response.result.summary} /> : null}
        {ranking.length ? <Table<AISelectionRankItem> size="small" rowKey={(item) => item.symbol} pagination={false} scroll={{ x: 1100 }} dataSource={ranking} columns={[
          { title: '排名', dataIndex: 'rank', width: 62, render: (value, _item, index) => <strong>{value || index + 1}</strong> },
          { title: '股票', width: 145, render: (_value, item) => <div className="ai-selection-symbol"><strong>{resolveStockName(item, stockNames)}</strong><span>{item.symbol}</span></div> },
          { title: '状态', dataIndex: 'status', width: 82, render: statusTag },
          { title: '评分', dataIndex: 'score', width: 76, render: (value) => Number.isFinite(value) ? Number(value).toFixed(0) : '--' },
          { title: '样本', width: 72, render: (_value, item) => item.historical_validation?.sample_count ?? '--' },
          { title: '历史胜率', width: 96, render: (_value, item) => percent(item.historical_validation?.win_rate) },
          { title: '平均收益', width: 96, render: (_value, item) => percent(item.historical_validation?.average_return) },
          { title: '最大回撤', width: 96, render: (_value, item) => percent(item.historical_validation?.max_drawdown) },
          { title: '主要依据', width: 240, render: (_value, item) => (item.reasons || []).slice(0, 2).join('；') || '--' },
          { title: '风险', width: 210, render: (_value, item) => (item.risks || []).slice(0, 2).join('；') || '--' },
          { title: '研究', fixed: 'right', width: 118, render: (_value, item) => <AIResearchReport symbol={item.symbol} name={resolveStockName(item, stockNames)} /> }
        ]} /> : <Empty description={loading ? '正在执行历史验证与排序' : '选择候选来源后执行验证'} />}
        {response?.result?.disclaimer ? <Alert className="ai-selection-disclaimer" type="warning" message={response.result.disclaimer} /> : null}
      </Card>
    </div>
  );
}
