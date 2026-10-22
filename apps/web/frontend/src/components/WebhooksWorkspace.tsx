import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography, message } from 'antd';
import { ApiOutlined, BellOutlined, CalendarOutlined, CheckCircleOutlined, ClockCircleOutlined, CloseOutlined, DeleteOutlined, EditOutlined, LinkOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, WarningOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { JsonPane } from './JsonPane';
import type { AutomationTask, MacroAlertSettings, MacroEvent, Webhook } from '../types';

const { Text } = Typography;

const methodOptions = [
  { value: 'POST', label: 'POST' },
  { value: 'PUT', label: 'PUT' },
  { value: 'PATCH', label: 'PATCH' }
];

const defaultAlertSettings: MacroAlertSettings = {
  id: 'default',
  enabled: false,
  lead_minutes: 240,
  window_before_minutes: 240,
  window_after_minutes: 120,
  critical_only: false,
  notify_webhooks: false,
  webhook_ids: '[]'
};

const taskTypeLabels: Record<string, string> = {
  stock_selection: '选股',
  strategy_selection: '策略选股',
  system_sync: '同步',
  custom: '自定义'
};

const macroTriggerLabels: Record<string, string> = {
  alert_due: '提前提醒',
  window_started: '风险窗口开始'
};

interface WebhookDeliveryTask {
  key: string;
  taskName: string;
  source: string;
  detail: string;
  event: string;
  channelName: string;
  channelId: string;
  nextAt?: string;
  sortAt: number;
  status: '待推送' | '已触发' | '任务停用' | '渠道停用' | '未订阅' | '渠道不存在' | '未配置';
}

function parseEventCount(value?: string) {
  try {
    const events = JSON.parse(value || '[]');
    return Array.isArray(events) ? events.length : 0;
  } catch {
    return 0;
  }
}

function parseWebhookIds(value?: string) {
  try {
    const ids = JSON.parse(value || '[]');
    return Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string' && id.trim() !== '') : [];
  } catch {
    return [];
  }
}

function parseWebhookEvents(value?: string) {
  try {
    const events = JSON.parse(value || '[]');
    return Array.isArray(events) ? events.filter((event): event is string => typeof event === 'string') : [];
  } catch {
    return [];
  }
}

function webhookAllowsEvent(hook: Webhook | undefined, event: string) {
  if (!hook) return false;
  const events = parseWebhookEvents(hook.events);
  return events.length === 0 || events.includes('*') || events.includes(event) || (event.endsWith('.finished') && events.includes('automation.finished'));
}

function formatDateTime(value?: string) {
  if (!value) return '--';
  const date = dayjs(value);
  return date.isValid() ? date.format('MM月DD日 HH:mm') : value;
}

function formatRelative(value: string | undefined, now: dayjs.Dayjs) {
  if (!value) return '等待调度器计算';
  const date = dayjs(value);
  if (!date.isValid()) return '时间格式待确认';
  const minutes = date.diff(now, 'minute');
  if (minutes < 0) return '已到触发时间';
  if (minutes < 60) return `${Math.max(minutes, 1)} 分钟后`;
  if (minutes < 24 * 60) return `${Math.floor(minutes / 60)} 小时后`;
  return `${Math.floor(minutes / (24 * 60))} 天后`;
}

function automationEvents(task: AutomationTask) {
  if (task.type === 'stock_selection') return ['stock_selection.finished', 'automation.failed'];
  if (task.type === 'strategy_selection') return ['strategy_selection.finished', 'automation.failed'];
  return ['automation.finished', 'automation.failed'];
}

function deliveryStatusClass(status: WebhookDeliveryTask['status']) {
  if (status === '待推送') return 'is-ready';
  if (status === '已触发') return 'is-triggered';
  if (status === '未订阅' || status === '渠道不存在' || status === '未配置') return 'is-warning';
  return 'is-muted';
}

function endpointKind(url?: string) {
  const text = (url || '').toLowerCase();
  if (text.includes('open.feishu.cn') || text.includes('open.larksuite.com')) return '飞书';
  if (text.startsWith('https://')) return 'HTTPS';
  if (text.startsWith('http://')) return 'HTTP';
  return '未识别';
}

export function WebhooksWorkspace() {
  const [items, setItems] = useState<Webhook[]>([]);
  const [tasks, setTasks] = useState<AutomationTask[]>([]);
  const [macroEvents, setMacroEvents] = useState<MacroEvent[]>([]);
  const [alertSettings, setAlertSettings] = useState<MacroAlertSettings>(defaultAlertSettings);
  const [selected, setSelected] = useState<Webhook | null>(null);
  const [testOutput, setTestOutput] = useState<unknown>('尚未测试');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<Webhook | null>(null);
  const [loading, setLoading] = useState(false);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [form] = Form.useForm<Webhook>();

  const load = async () => {
    setLoading(true);
    try {
      const [data, taskList, eventList, settings] = await Promise.all([
        apiFetch<Webhook[]>('/api/webhooks'),
        apiFetch<AutomationTask[]>('/api/automations'),
        apiFetch<MacroEvent[]>('/api/macro-events'),
        apiFetch<MacroAlertSettings>('/api/macro-events/settings')
      ]);
      const nextItems = data || [];
      setItems(nextItems);
      setTasks(taskList || []);
      setMacroEvents(eventList || []);
      setAlertSettings(settings || defaultAlertSettings);
      setSelected((current) => nextItems.find((item) => item.id === current?.id) || nextItems[0] || null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'Webhook 加载失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);
  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  const enabledCount = useMemo(() => items.filter((item) => item.enabled).length, [items]);
  const eventCount = useMemo(() => items.reduce((sum, item) => sum + parseEventCount(item.events), 0), [items]);
  const now = useMemo(() => dayjs(nowMs), [nowMs]);
  const deliveryTasks = useMemo<WebhookDeliveryTask[]>(() => {
    const hooksById = new Map(items.map((item) => [item.id, item]));
    const result: WebhookDeliveryTask[] = [];

    tasks.forEach((task) => {
      const ids = parseWebhookIds(task.webhook_ids);
      if (!ids.length) return;
      const events = automationEvents(task);
      ids.forEach((id) => {
        const hook = hooksById.get(id);
        const subscribed = events.some((event) => webhookAllowsEvent(hook, event));
        const status: WebhookDeliveryTask['status'] = !task.enabled
          ? '任务停用'
          : !hook
            ? '渠道不存在'
            : !hook.enabled
              ? '渠道停用'
              : !subscribed
                ? '未订阅'
                : '待推送';
        result.push({
          key: `automation:${task.id}:${id}`,
          taskName: task.name || '未命名任务',
          source: '自动化任务',
          detail: `${taskTypeLabels[task.type] || task.type} · ${task.cron || '--'}`,
          event: events.join(' / '),
          channelName: hook?.name || '渠道不存在',
          channelId: id,
          nextAt: task.next_run_at,
          sortAt: task.next_run_at && dayjs(task.next_run_at).isValid() ? dayjs(task.next_run_at).valueOf() : Number.MAX_SAFE_INTEGER,
          status
        });
      });
    });

    if (alertSettings.enabled && alertSettings.notify_webhooks) {
      const ids = parseWebhookIds(alertSettings.webhook_ids);
      const triggerDefinitions = alertSettings.lead_minutes > alertSettings.window_before_minutes
        ? [['alert_due', alertSettings.lead_minutes], ['window_started', alertSettings.window_before_minutes]] as const
        : [['window_started', alertSettings.window_before_minutes]] as const;
      macroEvents.forEach((event) => {
        const startsAt = dayjs(event.starts_at);
        if (alertSettings.critical_only && event.impact !== 'high' && event.impact !== 'critical') return;
        if (!startsAt.isValid() || startsAt.add(alertSettings.window_after_minutes, 'minute').isBefore(now)) return;
        triggerDefinitions.forEach(([kind, minutesBefore]) => {
          const nextAt = startsAt.subtract(minutesBefore, 'minute');
          const eventName = `macro_event.${kind}`;
          ids.forEach((id) => {
            const hook = hooksById.get(id);
            const status: WebhookDeliveryTask['status'] = !hook
              ? '渠道不存在'
              : !hook.enabled
                ? '渠道停用'
                : !webhookAllowsEvent(hook, eventName)
                  ? '未订阅'
                  : nextAt.isBefore(now)
                    ? '已触发'
                    : '待推送';
            result.push({
              key: `macro:${event.id}:${kind}:${id}`,
              taskName: event.name || event.code || '宏观事件',
              source: '宏观预警',
              detail: `${event.code || '--'} · ${formatDateTime(event.starts_at)}`,
              event: `${macroTriggerLabels[kind]} · ${eventName}`,
              channelName: hook?.name || '渠道不存在',
              channelId: id,
              nextAt: nextAt.format(),
              sortAt: nextAt.valueOf(),
              status
            });
          });
        });
      });
    }

    return result.sort((left, right) => left.sortAt - right.sortAt);
  }, [alertSettings, items, macroEvents, now, tasks]);

  const readyDeliveryCount = useMemo(() => deliveryTasks.filter((task) => task.status === '待推送').length, [deliveryTasks]);
  const nextDelivery = useMemo(() => deliveryTasks.find((task) => task.status === '待推送'), [deliveryTasks]);

  function openDialog(item?: Webhook) {
    const next = item || {
      id: '',
      name: '',
      url: '',
      method: 'POST',
      headers_json: '{}',
      events: '["automation.failed","automation.finished","stock_selection.finished","macro_event.alert_due","macro_event.window_started","t-signal.triggered"]',
      enabled: true
    };
    setEditing(item || null);
    form.resetFields();
    form.setFieldsValue(next);
    setDialogOpen(true);
  }

  async function save(values: Webhook) {
    try {
      await apiFetch(values.id ? `/api/webhooks/${values.id}` : '/api/webhooks', {
        method: values.id ? 'PUT' : 'POST',
        body: JSON.stringify(values)
      });
      message.success(values.id ? 'Webhook 已更新' : 'Webhook 已添加');
      setDialogOpen(false);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'Webhook 保存失败');
    }
  }

  async function test(id: string) {
    const item = items.find((value) => value.id === id);
    setSelected(item || null);
    try {
      const data = await apiFetch(`/api/webhooks/${id}/test`, { method: 'POST' });
      setTestOutput(data);
      message.success('Webhook 测试完成');
    } catch (error) {
      const text = error instanceof Error ? error.message : 'Webhook 测试失败';
      setTestOutput(text);
      message.error(text);
    }
  }

  async function remove(id: string) {
    try {
      await apiFetch(`/api/webhooks/${id}`, { method: 'DELETE' });
      message.success('Webhook 已删除');
      if (selected?.id === id) setSelected(null);
      await load();
    } catch (error) {
      message.error(error instanceof Error ? error.message : 'Webhook 删除失败');
    }
  }

  return (
    <div className="webhook-workspace">
      <Card
        className="work-card webhook-overview"
        title={
          <div className="webhook-panel-title">
            <div><span>Webhook</span><Text type="secondary">自动化通知通道</Text></div>
            <small>DELIVERY BUS</small>
          </div>
        }
        extra={
          <Space>
            <Button aria-label="刷新 Webhook" icon={<ReloadOutlined />} onClick={load} loading={loading} />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openDialog()}>添加通道</Button>
          </Space>
        }
      >
        <div className="webhook-overview-grid">
          <div className="webhook-lead">
            <span>当前通道</span>
            <strong>{selected?.name || items[0]?.name || '尚未配置'}</strong>
            <small>{selected ? `${endpointKind(selected.url)} · ${selected.method || 'POST'}` : '添加一个通知通道后接收任务结果'}</small>
          </div>
          <div><span>通道数</span><strong>{items.length}</strong><small>个配置</small></div>
          <div><span>启用数</span><strong>{enabledCount}</strong><small>可发送</small></div>
          <div><span>事件数</span><strong>{eventCount}</strong><small>已订阅</small></div>
          <div><span>推送任务</span><strong>{readyDeliveryCount}</strong><small>待推送</small></div>
          <div className="webhook-overview-status">
            <CheckCircleOutlined />
            <div><strong>通知总线就绪</strong><small>按事件白名单发送</small></div>
          </div>
        </div>
      </Card>

      <Card
        className="work-card webhook-schedule-card"
        title={
          <div className="webhook-panel-title">
            <div><span>推送计划</span><Text type="secondary">已绑定的自动化与宏观预警</Text></div>
            <small>SCHEDULE</small>
          </div>
        }
        extra={<Text type="secondary">{deliveryTasks.length} 条记录</Text>}
      >
        <div className="webhook-schedule-summary">
          <div className="webhook-next-delivery">
            <ClockCircleOutlined />
            <div>
              <span>下一次推送</span>
              <strong>{nextDelivery?.nextAt ? formatDateTime(nextDelivery.nextAt) : '暂无待推送'}</strong>
            </div>
          </div>
          <div className="webhook-schedule-rules">
            <span><CalendarOutlined /> 宏观预警：{alertSettings.notify_webhooks ? `事件前 ${alertSettings.window_before_minutes} 分钟进入窗口` : '未开启'}</span>
            <span><WarningOutlined /> 推送前会按渠道事件白名单过滤</span>
          </div>
        </div>
        <Table
          className="webhook-schedule-table"
          size="middle"
          rowKey="key"
          dataSource={deliveryTasks}
          pagination={{ pageSize: 10, size: 'small', showSizeChanger: false }}
          locale={{ emptyText: '暂无已绑定的推送任务' }}
          columns={[
            {
              title: '推送任务',
              width: 250,
              render: (_value, record: WebhookDeliveryTask) => <div className="webhook-delivery-task-cell"><strong>{record.taskName}</strong><span>{record.source} · {record.detail}</span></div>
            },
            { title: '触发事件', dataIndex: 'event', width: 220, render: (value: string) => <span className="webhook-event-code">{value}</span> },
            {
              title: '推送渠道',
              width: 170,
              render: (_value, record: WebhookDeliveryTask) => <div className="webhook-delivery-channel-cell"><strong>{record.channelName}</strong><span>{record.channelId}</span></div>
            },
            {
              title: '下次推送',
              width: 170,
              render: (_value, record: WebhookDeliveryTask) => <div className="webhook-delivery-time-cell"><strong>{record.nextAt ? formatDateTime(record.nextAt) : '等待调度'}</strong><span>{formatRelative(record.nextAt, now)}</span></div>
            },
            {
              title: '状态',
              width: 100,
              render: (_value, record: WebhookDeliveryTask) => <Tag className={`webhook-delivery-status-tag ${deliveryStatusClass(record.status)}`}>{record.status}</Tag>
            }
          ]}
          scroll={{ x: 910 }}
        />
      </Card>

      <div className="webhook-main-grid">
        <Card
          className="work-card webhook-list-card"
          title={
            <div className="webhook-panel-title">
              <div><span>通知通道</span><Text type="secondary">{items.length} 个端点</Text></div>
              <small>ENDPOINTS</small>
            </div>
          }
        >
          <Table
            size="middle"
            rowKey="id"
            dataSource={items}
            loading={loading}
            pagination={{ pageSize: 8, size: 'small', showSizeChanger: false }}
            rowClassName={(record) => (record.id === selected?.id ? 'row-active' : '')}
            locale={{ emptyText: '暂无 Webhook，先添加一个通知通道' }}
            onRow={(record) => ({ onClick: () => setSelected(record) })}
            columns={[
              {
                title: '通道名称',
                dataIndex: 'name',
                width: 156,
                render: (value: string, record: Webhook) => (
                  <div className="webhook-name-cell"><strong>{value || '--'}</strong><span>{record.id || '--'}</span></div>
                )
              },
              { title: '类型', dataIndex: 'url', width: 86, render: (value: string) => <span className="webhook-kind">{endpointKind(value)}</span> },
              { title: '方法', dataIndex: 'method', width: 82, render: (value: string) => <span className="webhook-code">{value || 'POST'}</span> },
              { title: '事件', dataIndex: 'events', width: 92, render: (value: string) => `${parseEventCount(value)} 个` },
              {
                title: '状态',
                width: 88,
                render: (_value, record) => <Tag className={`webhook-status-tag ${record.enabled ? 'is-enabled' : ''}`}>{record.enabled ? '已启用' : '已停用'}</Tag>
              },
              { title: 'URL', dataIndex: 'url', width: 230, render: (value: string) => <span className="webhook-url">{value || '--'}</span> },
              {
                title: '操作',
                width: 194,
                render: (_value, record) => (
                  <Space size={4}>
                    <Button size="small" icon={<ApiOutlined />} onClick={(event) => { event.stopPropagation(); test(record.id); }}>测试</Button>
                    <Button size="small" icon={<EditOutlined />} onClick={(event) => { event.stopPropagation(); openDialog(record); }}>编辑</Button>
                    <Popconfirm
                      title="删除 Webhook"
                      description="删除后自动化任务将不再使用这个通道。"
                      okText="删除"
                      cancelText="取消"
                      onConfirm={(event) => { event?.stopPropagation(); remove(record.id); }}
                    >
                      <Button size="small" danger icon={<DeleteOutlined />} onClick={(event) => event.stopPropagation()} />
                    </Popconfirm>
                  </Space>
                )
              }
            ]}
            scroll={{ x: 928 }}
          />
        </Card>

        <Card
          className="work-card webhook-test-card"
          title={
            <div className="webhook-panel-title">
              <div><span>发送测试</span><Text type="secondary">{selected?.name || '未选择通道'}</Text></div>
              <small>PROBE</small>
            </div>
          }
          extra={selected ? <Button size="small" icon={<ApiOutlined />} onClick={() => test(selected.id)}>重新测试</Button> : null}
        >
          <div className="webhook-test-intro">
            <BellOutlined />
            <div><strong>{selected ? '发送一条本地测试通知' : '选择一个通道查看测试结果'}</strong><span>测试事件为 webhook.test</span></div>
          </div>
          <JsonPane value={testOutput} />
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
        className="app-themed-modal webhook-edit-modal"
        title={
          <div className="quote-dialog-title webhook-dialog-title">
            <div>
              <strong>{editing ? '编辑 Webhook' : '添加 Webhook'}</strong>
              <span>{editing?.name || '通知通道'}</span>
            </div>
            <small>事件通知 · 结果投递</small>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={save} className="webhook-form">
          <Form.Item name="id" hidden><Input /></Form.Item>
          <section className="webhook-form-section">
            <div className="webhook-form-section-head">
              <div><strong>端点信息</strong><span>定义通知发往哪里、用什么方法发送</span></div>
              <small>ENDPOINT</small>
            </div>
            <div className="webhook-form-grid webhook-form-grid-endpoint">
              <Form.Item name="name" label="通道名称" rules={[{ required: true, message: '请输入通道名称' }]}><Input placeholder="例如：飞书交易提醒" /></Form.Item>
              <Form.Item name="method" label="请求方法" rules={[{ required: true, message: '请选择请求方法' }]}><Select options={methodOptions} /></Form.Item>
              <Form.Item name="enabled" label="启用通道" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="url" label="Webhook URL" rules={[{ required: true, message: '请输入 Webhook URL' }]}><Input prefix={<LinkOutlined />} placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." /></Form.Item>
            </div>
          </section>

          <section className="webhook-form-section">
            <div className="webhook-form-section-head">
              <div><strong>事件白名单</strong><span>空数组表示接收所有事件，也可以填写具体事件名</span></div>
              <small>EVENTS</small>
            </div>
            <div className="webhook-form-grid webhook-form-grid-events">
              <Form.Item name="events" label="事件 JSON" extra="持仓止盈止损预警的事件名为 trade.alert；空数组接收全部事件。"><Input.TextArea rows={4} placeholder={'例如：["trade.alert","t-signal.triggered","automation.failed"]'} /></Form.Item>
              <Button size="small" onClick={() => {
                const events = parseWebhookEvents(form.getFieldValue('events'));
                if (events.length && !events.includes('*') && !events.includes('trade.alert')) {
                  form.setFieldValue('events', JSON.stringify([...events, 'trade.alert']));
                }
                message.success('此渠道已允许持仓止盈止损预警，保存后生效');
              }}>订阅持仓止盈止损预警</Button>
            </div>
          </section>

          <section className="webhook-form-section">
            <div className="webhook-form-section-head">
              <div><strong>请求头</strong><span>用于兼容自建网关或签名认证</span></div>
              <small>HEADERS</small>
            </div>
            <div className="webhook-form-grid webhook-form-grid-headers">
              <Form.Item name="headers_json" label="请求头 JSON"><Input.TextArea rows={4} placeholder={'例如：{"Authorization": "Bearer token"}'} /></Form.Item>
            </div>
          </section>

          <div className="webhook-form-actions">
            <Text type="secondary">保存后可在自动化任务中选择这个通知通道</Text>
            <Space>
              <Button onClick={() => setDialogOpen(false)}>取消</Button>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />}>保存通道</Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
