import { Button, Card, Empty, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography, message } from 'antd';
import { CopyOutlined, DeleteOutlined, PlusOutlined, ReloadOutlined, SafetyCertificateOutlined, SendOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { parseConfigJson, summarizeStrategyUniverse } from '../lib/format';
import { JsonPane } from './JsonPane';
import type { StockPool, Strategy } from '../types';

const { Text } = Typography;

type UniverseRole = 'include' | 'intersect' | 'exclude';
type UniverseTerm = { pool?: string; symbols?: string[] };
type UniverseExpression = {
  include: UniverseTerm[];
  intersect: UniverseTerm[];
  exclude: UniverseTerm[];
};
type UniversePreview = {
  expression: UniverseExpression;
  total: number;
  scanned: number;
  limit?: number;
  truncated: boolean;
  symbols: string[];
  sample: string[];
  sources?: Array<{ role: UniverseRole; pool_id?: string; name: string; count: number }>;
};

const roleMeta: Record<UniverseRole, { label: string; hint: string; color: string }> = {
  include: { label: '起点池 · 并集', hint: '至少选择一个起点池，多个起点会合并', color: 'blue' },
  intersect: { label: '限定池 · 交集', hint: '候选必须同时属于这些池', color: 'cyan' },
  exclude: { label: '剔除池 · 差集', hint: '从最终范围中排除这些池', color: 'red' }
};

function emptyExpression(): UniverseExpression {
  return {
    include: [{ pool: 'market-all-a' }],
    intersect: [],
    exclude: [{ pool: 'exclude' }, { pool: 'market-star' }, { pool: 'market-bj' }]
  };
}

function poolCategoryLabel(pool: StockPool) {
  if (pool.category === 'market') return '市场';
  if (pool.category === 'decision') return '决策';
  return '自定义';
}

function termLabel(term: UniverseTerm, pools: StockPool[]) {
  if (term.symbols?.length) return `手动代码 · ${term.symbols.length}只`;
  const pool = pools.find((item) => item.id === term.pool);
  return pool ? `${pool.name} · ${pool.symbols?.length || 0}只` : term.pool || '未选择';
}

export function UniverseWorkspace() {
  const [pools, setPools] = useState<StockPool[]>([]);
  const [expression, setExpression] = useState<UniverseExpression>(emptyExpression);
  const [preview, setPreview] = useState<UniversePreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [poolDialogOpen, setPoolDialogOpen] = useState(false);
  const [poolSaving, setPoolSaving] = useState(false);
  const [applyDialogOpen, setApplyDialogOpen] = useState(false);
  const [applySaving, setApplySaving] = useState(false);
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [applyTarget, setApplyTarget] = useState<string>('');
  const [poolForm] = Form.useForm();

  async function loadPools() {
    setLoading(true);
    try {
      const data = await apiFetch<StockPool[]>('/api/stock-pools');
      setPools(data || []);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '股票池加载失败');
    } finally {
      setLoading(false);
    }
  }

  async function previewRange(nextExpression = expression) {
    setPreviewLoading(true);
    try {
      const data = await apiFetch<UniversePreview>('/api/strategies/range-preview', {
        method: 'POST',
        body: JSON.stringify({ universe: nextExpression, max_codes: 300 })
      });
      setPreview(data);
    } catch (error) {
      setPreview(null);
      message.error(error instanceof Error ? error.message : '范围预览失败');
    } finally {
      setPreviewLoading(false);
    }
  }

  useEffect(() => { void loadPools(); }, []);

  useEffect(() => {
    if (!pools.length) return;
    const timer = window.setTimeout(() => { void previewRange(expression); }, 180);
    return () => window.clearTimeout(timer);
  }, [expression, pools.length]);

  const poolOptions = useMemo(
    () => pools.map((pool) => ({
      value: pool.id,
      label: `${pool.name} · ${pool.symbols?.length || 0}只`,
      category: poolCategoryLabel(pool)
    })),
    [pools]
  );

  function updateTerms(role: UniverseRole, terms: UniverseTerm[]) {
    setExpression((current) => ({ ...current, [role]: terms }));
  }

  function addTerm(role: UniverseRole) {
    updateTerms(role, [...expression[role], { pool: pools[0]?.id || '' }]);
  }

  function removeTerm(role: UniverseRole, index: number) {
    updateTerms(role, expression[role].filter((_term, termIndex) => termIndex !== index));
  }

  function changePool(role: UniverseRole, index: number, pool: string) {
    updateTerms(role, expression[role].map((term, termIndex) => termIndex === index ? { pool } : term));
  }

  async function copyExpression() {
    await navigator.clipboard.writeText(JSON.stringify({ universe: expression }, null, 2));
    message.success('范围配置已复制');
  }

  async function savePool(values: { name: string; description?: string; symbols: string }) {
    setPoolSaving(true);
    try {
      await apiFetch('/api/stock-pools', {
        method: 'POST',
        body: JSON.stringify({
          name: values.name,
          description: values.description || '',
          symbols: values.symbols.split(/[\s,，;；]+/).map((item) => item.trim()).filter(Boolean)
        })
      });
      message.success('股票池已创建');
      setPoolDialogOpen(false);
      poolForm.resetFields();
      await loadPools();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '股票池保存失败');
    } finally {
      setPoolSaving(false);
    }
  }

  async function deletePool(pool: StockPool) {
    try {
      await apiFetch(`/api/stock-pools/${pool.id}`, { method: 'DELETE' });
      message.success('股票池已删除');
      await loadPools();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '股票池删除失败');
    }
  }

  async function openApplyDialog() {
    if (!expression.include.length || expression.include.some((term) => !term.pool)) {
      message.warning('请先设置至少一个有效的起点池');
      return;
    }
    try {
      const data = await apiFetch<Strategy[]>('/api/strategies');
      setStrategies(data || []);
      setApplyTarget('');
      setApplyDialogOpen(true);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '策略列表加载失败');
    }
  }

  async function applyToStrategy() {
    if (!applyTarget) {
      message.warning('请选择目标策略');
      return;
    }
    setApplySaving(true);
    try {
      await apiFetch(`/api/strategies/${applyTarget}/apply-universe`, {
        method: 'POST',
        body: JSON.stringify({ universe: expression })
      });
      message.success('选股范围已应用到策略');
      setApplyDialogOpen(false);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '应用范围失败');
    } finally {
      setApplySaving(false);
    }
  }

  const applyTargetSummary = useMemo(() => {
    const target = strategies.find((item) => item.id === applyTarget);
    if (!target) return '';
    return summarizeStrategyUniverse(parseConfigJson(target.config_json), pools);
  }, [strategies, applyTarget, pools]);

  return (
    <div className="universe-workspace">
      <Card
        className="work-card universe-overview"
        title={<div className="universe-panel-title"><div><span>选股范围</span><Text type="secondary">先圈定候选，再交给策略和 AI</Text></div><small>UNIVERSE BUILDER</small></div>}
        extra={<Space><Button aria-label="刷新股票池" icon={<ReloadOutlined />} onClick={() => void loadPools()} loading={loading} /><Button icon={<SendOutlined />} onClick={() => void openApplyDialog()}>应用到策略</Button><Button type="primary" icon={<PlusOutlined />} onClick={() => setPoolDialogOpen(true)}>新建股票池</Button></Space>}
      >
        <div className="universe-overview-grid">
          <div><span>最终覆盖</span><strong>{previewLoading ? '...' : preview?.total ?? '--'}</strong><small>只候选股票</small></div>
          <div><span>起点池</span><strong>{expression.include.length}</strong><small>并集来源</small></div>
          <div><span>限定池</span><strong>{expression.intersect.length}</strong><small>交集条件</small></div>
          <div><span>剔除池</span><strong>{expression.exclude.length}</strong><small>差集条件</small></div>
          <div className="universe-overview-status"><SafetyCertificateOutlined /><div><strong>{preview?.truncated ? '范围偏大，运行受保护' : '范围可用于执行'}</strong><small>{preview?.truncated ? `策略运行会扫描前 ${preview.scanned} 只` : '当前没有数量截断'}</small></div></div>
        </div>
      </Card>

      <div className="universe-main-grid">
        <Card className="work-card universe-builder-card" title={<div className="universe-panel-title"><div><span>范围配方</span><Text type="secondary">集合运算，不拉 K 线</Text></div><small>SET RECIPE</small></div>}>
          <div className="universe-flow">
            {(Object.keys(roleMeta) as UniverseRole[]).map((role) => {
              const meta = roleMeta[role];
              return (
                <section className="universe-role" key={role}>
                  <div className="universe-role-head"><div><Tag color={meta.color}>{role === 'include' ? 'IN' : role === 'intersect' ? 'AND' : 'OUT'}</Tag><strong>{meta.label}</strong></div><Button size="small" icon={<PlusOutlined />} onClick={() => addTerm(role)}>添加</Button></div>
                  <Text type="secondary">{meta.hint}</Text>
                  <div className="universe-term-list">
                    {expression[role].map((term, index) => (
                      <div className="universe-term-row" key={`${role}-${index}`}>
                        <Select
                          showSearch
                          value={term.pool}
                          placeholder="选择股票池"
                          options={poolOptions}
                          optionRender={(option) => <span>{option.data.label}</span>}
                          onChange={(value) => changePool(role, index, value)}
                        />
                        <span className="universe-term-count">{termLabel(term, pools)}</span>
                        <Button aria-label={`移除${meta.label}`} type="text" danger icon={<DeleteOutlined />} onClick={() => removeTerm(role, index)} />
                      </div>
                    ))}
                    {!expression[role].length ? <div className="universe-term-empty">暂无{meta.label}</div> : null}
                  </div>
                </section>
              );
            })}
          </div>
          <div className="universe-recipe-summary">
            <span>当前语义</span>
            <strong>{expression.include.map((term) => termLabel(term, pools)).join(' + ') || '未设置起点'}{expression.intersect.length ? ` ∩ ${expression.intersect.map((term) => termLabel(term, pools)).join(' ∩ ')}` : ''}{expression.exclude.length ? ` − ${expression.exclude.map((term) => termLabel(term, pools)).join(' − ')}` : ''}</strong>
          </div>
          <div className="universe-builder-actions"><Text type="secondary">范围只读取池成员表；动态标签由池刷新任务维护。</Text><Button icon={<CopyOutlined />} onClick={() => void copyExpression()}>复制范围 JSON</Button></div>
        </Card>

        <Card className="work-card universe-preview-card" title={<div className="universe-panel-title"><div><span>覆盖预览</span><Text type="secondary">确认范围再运行策略</Text></div><small>PREVIEW</small></div>} extra={<Button type="text" icon={<ReloadOutlined />} loading={previewLoading} onClick={() => void previewRange()} />}>
          {preview ? (
            <>
              <div className="universe-preview-number"><strong>{preview.total}</strong><span>只最终候选</span>{preview.truncated ? <Tag color="orange">运行上限 {preview.limit}</Tag> : null}</div>
              <div className="universe-source-list">{(preview.sources || []).map((source, index) => <div key={`${source.role}-${source.pool_id || index}`}><Tag color={roleMeta[source.role]?.color}>{roleMeta[source.role]?.label.split(' · ')[0]}</Tag><span>{source.name}</span><strong>{source.count}</strong></div>)}</div>
              <div className="universe-sample"><span>样本代码</span><div>{preview.sample.map((symbol) => <Tag key={symbol}>{symbol}</Tag>)}</div></div>
            </>
          ) : <Empty description={previewLoading ? '正在计算覆盖范围' : '尚未生成范围预览'} />}
        </Card>
      </div>

      <Card className="work-card universe-pool-card" title={<div className="universe-panel-title"><div><span>股票池注册表</span><Text type="secondary">{pools.length} 个可引用池</Text></div><small>POOL REGISTRY</small></div>}>
        <Table<StockPool> size="small" rowKey="id" dataSource={pools} loading={loading} pagination={{ pageSize: 8, size: 'small', showSizeChanger: false }} scroll={{ x: 760 }} columns={[
          { title: '名称', dataIndex: 'name', width: 190, render: (value: string, item) => <div className="universe-pool-name"><strong>{value}</strong><span>{item.id}</span></div> },
          { title: '类型', width: 90, render: (_value, item) => <Tag>{poolCategoryLabel(item)}</Tag> },
          { title: '成员数', width: 90, render: (_value, item) => item.symbols?.length || 0 },
          { title: '说明', dataIndex: 'description', render: (value: string) => value || '--' },
          { title: '操作', fixed: 'right', width: 80, render: (_value, item) => item.readonly || item.system ? null : <Popconfirm title="删除股票池" description={`确认删除 ${item.name}？`} okText="删除" cancelText="取消" onConfirm={() => void deletePool(item)}><Button aria-label={`删除${item.name}`} type="text" danger icon={<DeleteOutlined />} /></Popconfirm> }
        ]} />
      </Card>

      <Modal open={applyDialogOpen} onCancel={() => setApplyDialogOpen(false)} footer={null} width={560} centered destroyOnHidden title={<div className="quote-dialog-title"><div><strong>应用到策略</strong><span>把当前范围写入目标策略</span></div><small>UNIVERSE → STRATEGY</small></div>}>
        <div className="universe-apply-body">
          <Text type="secondary">仅替换目标策略的候选覆盖；过滤、评分与输出参数保持不变。系统策略也可调整候选覆盖，因子和评分条件仍保持锁定。</Text>
          <Select
            showSearch
            value={applyTarget || undefined}
            placeholder="选择目标策略"
            optionFilterProp="label"
            options={strategies.map((item) => ({ value: item.id, label: `${item.name}${item.readonly ? ' · 系统策略' : ''}` }))}
            onChange={setApplyTarget}
          />
          {applyTarget ? <div className="universe-apply-current"><span>该策略当前范围</span><strong>{applyTargetSummary || '--'}</strong></div> : null}
        </div>
        <div className="universe-modal-actions"><Text type="secondary">当前范围覆盖 {previewLoading ? '...' : preview?.total ?? '--'} 只候选股票</Text><Space><Button onClick={() => setApplyDialogOpen(false)}>取消</Button><Button type="primary" icon={<SendOutlined />} loading={applySaving} onClick={() => void applyToStrategy()}>应用范围</Button></Space></div>
      </Modal>

      <Modal open={poolDialogOpen} onCancel={() => setPoolDialogOpen(false)} footer={null} width={640} centered destroyOnHidden title="新建股票池">
        <Form form={poolForm} layout="vertical" onFinish={savePool}>
          <Form.Item name="name" label="股票池名称" rules={[{ required: true, message: '请输入股票池名称' }]}><Input placeholder="例如：新能源核心观察池" /></Form.Item>
          <Form.Item name="description" label="说明"><Input placeholder="描述这个池子的用途或来源" /></Form.Item>
          <Form.Item name="symbols" label="股票代码" rules={[{ required: true, message: '请输入至少一个股票代码' }]}><Input.TextArea rows={5} placeholder="手动输入股票代码，多个代码用逗号或换行分隔" /></Form.Item>
          <div className="universe-modal-actions"><Text type="secondary">保存后即可在起点、限定或剔除池中引用。</Text><Space><Button onClick={() => setPoolDialogOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={poolSaving}>保存股票池</Button></Space></div>
        </Form>
      </Modal>

      <Card className="work-card universe-json-card" title={<div className="universe-panel-title"><div><span>当前配置</span><Text type="secondary">可直接放入策略 config_json</Text></div><small>CONFIG</small></div>}>
        <JsonPane value={{ universe: expression }} />
      </Card>
    </div>
  );
}
