import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography, message } from 'antd';
import { CheckCircleOutlined, CloseOutlined, DeleteOutlined, EditOutlined, FieldTimeOutlined, PlayCircleOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { JsonPane } from './JsonPane';
import type { AutomationRun, AutomationTask, Formula, StockPool, Strategy, Webhook } from '../types';

const { Text } = Typography;

const taskTypeOptions = [
  { value: 'stock_selection', label: '选股任务' },
  { value: 'strategy_selection', label: '策略选股' },
  { value: 'system_sync', label: '系统同步' },
  { value: 'custom', label: '自定义任务' }
];

const taskTypeLabels: Record<string, string> = {
  stock_selection: '选股',
  strategy_selection: '策略',
  system_sync: '同步',
  custom: '自定义'
};

const templateActions = [
  { key: 'morning_sync', name: '早盘基础同步', note: '08:00 交易日前置数据' },
  { key: 'evening_kline', name: '晚盘日K同步', note: '18:30 更新日线数据' },
  { key: 'evening_full', name: '晚盘完整同步', note: '21:00 批量补全数据' },
  { key: 'market_long_tiger_sync', name: '龙虎榜同步', note: '18:00 同步龙虎榜' },
  { key: 'market_hot_money_sync', name: '游资动向同步', note: '18:05 批量同步游资动向' },
  { key: 'market_research_sync', name: '个股研报同步', note: '18:10 批量同步个股研报' },
  { key: 'market_notice_sync', name: '公司公告同步', note: '18:15 批量同步公司公告' },
  { key: 'market_industry_research_sync', name: '行业研究同步', note: '18:20 批量同步行业研报' },
  { key: 'strategy_execution', name: '策略执行', note: '盘后执行策略（选策略后创建）' }
];

function statusText(status?: string) {
  if (!status) return '暂无';
  if (status === 'success') return '成功';
  if (status === 'failed') return '失败';
  if (status === 'running') return '运行中';
  if (status === 'skipped') return '已跳过';
  return status;
}

function parseWebhookCount(value?: string) {
  try {
    const ids = JSON.parse(value || '[]');
    return Array.isArray(ids) ? ids.length : 0;
  } catch {
    return 0;
  }
}

export function AutomationsWorkspace() {
  const [tasks, setTasks] = useState<AutomationTask[]>([]);
  const [runs, setRuns] = useState<AutomationRun[]>([]);
  const [pools, setPools] = useState<StockPool[]>([]);
  const [formulas, setFormulas] = useState<Formula[]>([]);
  const [strategies, setStrategies] = useState<Strategy[]>([]);
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [selected, setSelected] = useState<AutomationTask | null>(null);
  const [payloadOutput, setPayloadOutput] = useState<unknown>('暂无运行');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [strategyDialogOpen, setStrategyDialogOpen] = useState(false);
  const [selectedStrategyId, setSelectedStrategyId] = useState('');
  const [editing, setEditing] = useState<AutomationTask | null>(null);
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm<AutomationTask>();

  const load = async () => {
    setLoading(true);
    try {
      const [taskList, runList, poolList, formulaList, strategyList, webhookList] = await Promise.all([
        apiFetch<AutomationTask[]>('/api/automations'),
        apiFetch<AutomationRun[]>('/api/automations/runs?limit=50'),
        apiFetch<StockPool[]>('/api/stock-pools'),
        apiFetch<Formula[]>('/api/formulas'),
        apiFetch<Strategy[]>('/api/strategies'),
        apiFetch<Webhook[]>('/api/webhooks')
      ]);
      const nextTasks = taskList || [];
      setTasks(nextTasks);
      setRuns(runList || []);
      setPools(poolList || []);
      setFormulas(formulaList || []);
      setStrategies(strategyList || []);
      setWebhooks(webhookList || []);
      setSelected((current) => nextTasks.find((item) => item.id === current?.id) || nextTasks[0] || null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '自动化加载失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const enabledCount = useMemo(() => tasks.filter((task) => task.enabled).length, [tasks]);
  const runningCount = useMemo(() => runs.filter((run) => run.status === 'running').length, [runs]);
  const successCount = useMemo(() => runs.filter((run) => run.status === 'success').length, [runs]);

  function openDialog(task?: AutomationTask) {
    const next = task ? { ...task, only_trade_day: task.only_trade_day ?? true } : {
      id: '',
      name: '',
      type: 'stock_selection',
      cron: '0 0 9 * * 1-5',
      enabled: false,
      only_trade_day: true,
      payload_json: '{}',
      webhook_ids: '[]'
    };
    setEditing(task || null);
    form.resetFields();
    form.setFieldsValue(next);
    setDialogOpen(true);
  }

  async function saveTask(values: AutomationTask) {
    try {
      await apiFetch(values.id ? `/api/automations/${values.id}` : '/api/automations', {
        method: values.id ? 'PUT' : 'POST',
        body: JSON.stringify(values)
      });
      message.success(values.id ? '任务已更新' : '任务已添加');
      setDialogOpen(false);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '任务保存失败');
    }
  }

  async function runTask(id: string) {
    try {
      const run = await apiFetch(`/api/automations/${id}/run`, { method: 'POST' });
      setPayloadOutput(run);
      message.success('任务运行已提交');
      await load();
    } catch (error) {
      const text = error instanceof Error ? error.message : '任务运行失败';
      setPayloadOutput(text);
      message.error(text);
    }
  }

  async function toggleTask(id: string, enabled: boolean) {
    try {
      await apiFetch(`/api/automations/${id}/enabled`, { method: 'POST', body: JSON.stringify({ enabled }) });
      message.success(enabled ? '任务已开启' : '任务已关闭');
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '任务状态更新失败');
    }
  }

  async function deleteTask(id: string) {
    try {
      await apiFetch(`/api/automations/${id}`, { method: 'DELETE' });
      message.success('任务已删除');
      if (selected?.id === id) setSelected(null);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '任务删除失败');
    }
  }

  async function createTemplate(template: string) {
    if (template === 'strategy_execution') {
      setSelectedStrategyId(strategies[0]?.id || '');
      setStrategyDialogOpen(true);
      return;
    }
    try {
      await apiFetch('/api/automations/templates', { method: 'POST', body: JSON.stringify({ template }) });
      message.success('任务模板已创建');
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '模板创建失败');
    }
  }

  async function createStrategyExecutionTask() {
    const strategy = strategies.find((item) => item.id === selectedStrategyId);
    if (!strategy) {
      message.warning('请先选择策略');
      return;
    }
    try {
      await apiFetch('/api/automations', {
        method: 'POST',
        body: JSON.stringify({
          id: '',
          name: `策略执行-${strategy.name}`,
          type: 'strategy_selection',
          cron: '0 0 15 * * 1-5',
          enabled: false,
          only_trade_day: true,
          payload_json: JSON.stringify({ strategy_id: strategy.id }),
          webhook_ids: '[]'
        })
      });
      message.success('策略执行任务已创建');
      setStrategyDialogOpen(false);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '策略执行任务创建失败');
    }
  }

  return (
    <div className="automation-workspace">
      <Card
        className="work-card automation-overview"
        title={
          <div className="automation-panel-title">
            <div><span>自动化</span><Text type="secondary">定时任务与运行队列</Text></div>
            <small>RUN SCHEDULER</small>
          </div>
        }
        extra={
          <Space>
            <Button aria-label="刷新自动化" icon={<ReloadOutlined />} onClick={load} loading={loading} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openDialog()}>新建任务</Button>
          </Space>
        }
      >
        <div className="automation-overview-grid">
          <div className="automation-overview-lead">
            <span>当前任务</span>
            <strong>{selected?.name || '尚未选择'}</strong>
            <small>{selected ? `${taskTypeLabels[selected.type] || selected.type} · ${selected.cron || '--'}` : '选择一个调度任务查看运行状态'}</small>
          </div>
          <div><span>任务数</span><strong>{tasks.length}</strong><small>个配置</small></div>
          <div><span>启用数</span><strong>{enabledCount}</strong><small>调度中</small></div>
          <div><span>运行中</span><strong>{runningCount}</strong><small>队列任务</small></div>
          <div className="automation-overview-status">
            <CheckCircleOutlined />
            <div><strong>调度器就绪</strong><small>{successCount} 次成功记录</small></div>
          </div>
        </div>
      </Card>

      <div className="automation-main-grid">
        <Card
          className="work-card automation-task-card"
          title={
            <div className="automation-panel-title">
              <div><span>任务列表</span><Text type="secondary">{tasks.length} 个调度</Text></div>
              <small>TASKS</small>
            </div>
          }
        >
          <Table
            size="middle"
            rowKey="id"
            dataSource={tasks}
            loading={loading}
            pagination={{ pageSize: 8, size: 'small', showSizeChanger: false }}
            rowClassName={(record) => (record.id === selected?.id ? 'row-active' : '')}
            onRow={(record) => ({ onClick: () => setSelected(record) })}
            locale={{ emptyText: '暂无自动化任务，先新建一个调度' }}
            columns={[
              {
                title: '任务名称',
                dataIndex: 'name',
                width: 190,
                render: (value: string, record: AutomationTask) => (
                  <div className="automation-name-cell"><strong>{value || '--'}</strong><span>{record.id || '--'}</span></div>
                )
              },
              { title: '类型', dataIndex: 'type', width: 92, render: (value: string) => <span className="automation-type">{taskTypeLabels[value] || value || '--'}</span> },
              { title: 'Cron', dataIndex: 'cron', width: 156, render: (value: string) => <span className="automation-code">{value || '--'}</span> },
              {
                title: '状态',
                width: 82,
                render: (_value, record) => <Tag className={`automation-status-tag ${record.enabled ? 'is-enabled' : ''}`}>{record.enabled ? '已开启' : '已关闭'}</Tag>
              },
              {
                title: '执行日',
                width: 96,
                render: (_value, record) => <Tag>{record.only_trade_day ? '交易日' : '每日'}</Tag>
              },
              { title: '最近结果', dataIndex: 'last_status', width: 90, render: (value: string) => <span className={`automation-run-state ${value || 'empty'}`}>{statusText(value)}</span> },
              {
                title: '下次运行',
                dataIndex: 'next_run_at',
                width: 156,
                render: (value: string) => <span className="automation-time">{value || '--'}</span>
              },
              {
                title: '操作',
                width: 228,
                render: (_value, record) => (
                  <Space size={4}>
                    <Button size="small" icon={<PlayCircleOutlined />} onClick={(event) => { event.stopPropagation(); runTask(record.id); }}>运行</Button>
                    <Button size="small" onClick={(event) => { event.stopPropagation(); toggleTask(record.id, !record.enabled); }}>{record.enabled ? '关闭' : '开启'}</Button>
                    {!record.readonly ? <Button size="small" icon={<EditOutlined />} onClick={(event) => { event.stopPropagation(); openDialog(record); }}>编辑</Button> : null}
                    {!record.readonly ? (
                      <Popconfirm
                        title="删除任务"
                        description="删除后不会再触发这个调度。"
                        okText="删除"
                        cancelText="取消"
                        onConfirm={(event) => { event?.stopPropagation(); deleteTask(record.id); }}
                      >
                        <Button size="small" danger icon={<DeleteOutlined />} onClick={(event) => event.stopPropagation()} />
                      </Popconfirm>
                    ) : null}
                  </Space>
                )
              }
            ]}
            scroll={{ x: 1090 }}
          />
        </Card>

        <Card
          className="work-card automation-inspector-card"
          title={
            <div className="automation-panel-title">
              <div><span>调度检查</span><Text type="secondary">{selected?.name || '未选择任务'}</Text></div>
              <small>INSPECTOR</small>
            </div>
          }
        >
          {selected ? (
            <div className="automation-inspector">
              <div className="automation-inspector-heading">
                <div>
                  <span>当前调度</span>
                  <strong>{selected.name}</strong>
                </div>
                <Tag className={`automation-status-tag ${selected.enabled ? 'is-enabled' : ''}`}>{selected.enabled ? '已开启' : '已关闭'}</Tag>
              </div>
              <div className="automation-inspector-meta">
                <div><span>类型</span><strong>{taskTypeLabels[selected.type] || selected.type}</strong></div>
                <div><span>Cron</span><strong>{selected.cron || '--'}</strong></div>
                <div><span>执行日</span><strong>{selected.only_trade_day ? '仅交易日' : '每日'}</strong></div>
                <div><span>Webhook</span><strong>{parseWebhookCount(selected.webhook_ids)} 个</strong></div>
                <div><span>最近结果</span><strong>{statusText(selected.last_status)}</strong></div>
              </div>
              <p className="automation-last-message">{selected.last_message || '暂无最近运行消息。'}</p>
              <div className="automation-inspector-actions">
                <Button type="primary" icon={<ThunderboltOutlined />} onClick={() => runTask(selected.id)}>立即运行</Button>
                <Button onClick={() => toggleTask(selected.id, !selected.enabled)}>{selected.enabled ? '关闭调度' : '开启调度'}</Button>
                {!selected.readonly ? <Button icon={<EditOutlined />} onClick={() => openDialog(selected)}>编辑任务</Button> : null}
              </div>
              <div className="automation-payload-preview">
                <div><span>Payload</span><FieldTimeOutlined /></div>
                <pre>{selected.payload_json || '{}'}</pre>
              </div>
            </div>
          ) : (
            <div className="automation-empty-inspector">选择一个任务查看调度细节。</div>
          )}
        </Card>
      </div>

      <div className="automation-support-grid">
        <Card
          className="work-card automation-template-card"
          title={
            <div className="automation-panel-title">
              <div><span>快速模板</span><Text type="secondary">常用同步任务</Text></div>
              <small>TEMPLATES</small>
            </div>
          }
        >
          <div className="automation-template-list">
            {templateActions.map((template) => (
              <button key={template.key} type="button" onClick={() => createTemplate(template.key)}>
                <strong>{template.name}</strong>
                <span>{template.note}</span>
              </button>
            ))}
          </div>
          <div className="automation-resource-grid">
            <div><span>股票池</span><strong>{pools.length}</strong></div>
            <div><span>公式</span><strong>{formulas.length}</strong></div>
            <div><span>策略</span><strong>{strategies.length}</strong></div>
            <div><span>Webhook</span><strong>{webhooks.length}</strong></div>
          </div>
        </Card>

        <Card
          className="work-card automation-run-card"
          title={
            <div className="automation-panel-title">
              <div><span>运行记录</span><Text type="secondary">{runs.length} 条记录</Text></div>
              <small>RUNS</small>
            </div>
          }
        >
          <Table
            size="small"
            rowKey="id"
            dataSource={runs}
            pagination={{ pageSize: 6, size: 'small', showSizeChanger: false }}
            locale={{ emptyText: '暂无运行记录' }}
            columns={[
              { title: '任务', dataIndex: 'task_name', width: 190, render: (value: string) => <span className="automation-run-name">{value || '--'}</span> },
              { title: '状态', dataIndex: 'status', width: 82, render: (value: string) => <span className={`automation-run-state ${value || 'empty'}`}>{statusText(value)}</span> },
              { title: '命中', dataIndex: 'matched_count', width: 68, render: (value: number) => value ?? '--' },
              { title: '开始', dataIndex: 'started_at', width: 150, render: (value: string) => <span className="automation-time">{value || '--'}</span> }
            ]}
            scroll={{ x: 490 }}
          />
          <JsonPane value={payloadOutput} />
        </Card>
      </div>

      <Modal
        open={dialogOpen}
        onCancel={() => setDialogOpen(false)}
        footer={null}
        width={940}
        centered
        destroyOnHidden
        closeIcon={<CloseOutlined />}
        className="app-themed-modal automation-edit-modal"
        title={
          <div className="quote-dialog-title automation-dialog-title">
            <div>
              <strong>{editing ? '编辑任务' : '新建任务'}</strong>
              <span>{editing?.name || '调度配置'}</span>
            </div>
            <small>定时执行 · 结果通知</small>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={saveTask} className="automation-form">
          <Form.Item name="id" hidden><Input /></Form.Item>
          <section className="automation-form-section">
            <div className="automation-form-section-head">
              <div><strong>调度信息</strong><span>定义任务名称、类型和运行节奏</span></div>
              <small>SCHEDULE</small>
            </div>
            <div className="automation-form-grid automation-form-grid-basic">
              <Form.Item name="name" label="任务名称" rules={[{ required: true, message: '请输入任务名称' }]}><Input placeholder="例如：早盘选股扫描" /></Form.Item>
              <Form.Item name="type" label="任务类型"><Select options={taskTypeOptions} /></Form.Item>
              <Form.Item name="cron" label="Cron" rules={[{ required: true, message: '请输入 Cron 表达式' }]}><Input placeholder="0 0 9 * * 1-5" /></Form.Item>
              <Form.Item name="enabled" label="启用调度" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="only_trade_day" label="仅交易日执行" valuePropName="checked"><Switch /></Form.Item>
            </div>
          </section>
          <section className="automation-form-section">
            <div className="automation-form-section-head">
              <div><strong>执行参数</strong><span>任务运行时提交给服务端的 JSON</span></div>
              <small>PAYLOAD</small>
            </div>
            <div className="automation-form-grid automation-form-grid-json">
              <Form.Item name="payload_json" label="Payload JSON"><Input.TextArea rows={8} placeholder={'例如：{"scope":"kline","limit":4}'} /></Form.Item>
            </div>
          </section>
          <section className="automation-form-section">
            <div className="automation-form-section-head">
              <div><strong>通知通道</strong><span>填写需要通知的 Webhook ID 数组</span></div>
              <small>WEBHOOKS</small>
            </div>
            <div className="automation-form-grid automation-form-grid-json">
              <Form.Item name="webhook_ids" label="Webhook ID 列表"><Input.TextArea rows={3} placeholder={'例如：["webhook-id"]'} /></Form.Item>
            </div>
          </section>
          <div className="automation-form-actions">
            <Text type="secondary">{editing?.readonly ? '固定任务只能开启或关闭，不能直接编辑' : '保存后调度器会重新载入任务配置'}</Text>
            <Space>
              <Button onClick={() => setDialogOpen(false)}>取消</Button>
              {!editing?.readonly ? <Button type="primary" htmlType="submit" icon={<SaveOutlined />}>保存任务</Button> : null}
            </Space>
          </div>
        </Form>
      </Modal>
      <Modal
        title="选择策略"
        open={strategyDialogOpen}
        onOk={createStrategyExecutionTask}
        onCancel={() => setStrategyDialogOpen(false)}
        okText="创建任务"
        cancelText="取消"
      >
        <p>为策略执行任务选择一个策略，任务将在每个交易日 15:00 运行。</p>
        <Select
          style={{ width: '100%' }}
          value={selectedStrategyId}
          onChange={setSelectedStrategyId}
          options={strategies.map((item) => ({ value: item.id, label: item.name }))}
          placeholder="请选择策略"
        />
      </Modal>
    </div>
  );
}
