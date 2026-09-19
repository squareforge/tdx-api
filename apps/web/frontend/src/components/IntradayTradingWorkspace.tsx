import { AutoComplete, Badge, Button, Card, Descriptions, Empty, Form, InputNumber, Modal, Space, Switch, Table, Tag, Typography, message } from 'antd';
import { CheckOutlined, CloseOutlined, ExclamationCircleOutlined, HistoryOutlined, ReloadOutlined, SettingOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiFetch } from '../lib/api';

const { Text, Title } = Typography;

// 日内做T信号驾驶舱,对齐 docs/INTRADAY_T_SIGNAL_UI_V1.md。
// 数据源为 /api/t-signal/snapshot(MinuteBarBuilder 合成分钟bar),与信号引擎同一份数据。

interface TSignalFollowUp {
  rule: string;
  ref: number;
}

interface TSignalRecord {
  id: string;
  symbol: string;
  time: string;
  direction: string;
  rule: string;
  trigger: string;
  ref_price: number;
  tolerance: number;
  target: number;
  stop: number;
  expire_at: string;
  position_hint: string;
  follow_up?: TSignalFollowUp;
  status: string;
  ack_at?: string;
}

interface TSignalLedgerEntry {
  id: string;
  sell_signal_id: string;
  buy_signal_id?: string;
  sell_price: number;
  buy_price?: number;
  sell_time: string;
  buy_time?: string;
  shares: number;
  net_pnl?: number;
  closed: boolean;
}

interface MinuteBar {
  time: string;
  chart_time?: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  amount: number;
  complete: boolean;
  trusted: boolean;
}

interface TSignalTrendVeto {
  status: string;
  detail: string;
  updated_at?: string;
}

interface TSignalCooldown {
  active: boolean;
  reason: string;
  loss_pnl?: number;
}

interface TSignalRuleParams {
  s1_enabled: boolean;
  s1_dev_pct: number;
  s1_back_pct: number;
  s1_stop_pct: number;
  s2_enabled: boolean;
  s2_stop_pct: number;
  s3_enabled: boolean;
  s3_lookback: number;
  s4_enabled: boolean;
  s4_target_pct: number;
  s4_vol_ratio: number;
  s5_enabled: boolean;
  s5_move_pct: number;
  s5_stop_pct: number;
  s5_cooldown: number;
  s6_enabled: boolean;
  s6_peak_dev_pct: number;
  s6_pullback_pct: number;
  s6_min_dev_pct: number;
  s6_min_day_gain_pct: number;
  s6_max_day_gain_pct: number;
  s6_min_rsi: number;
  s6_max_vwap_slope_pct: number;
  s6_max_vol_ratio: number;
  s6_min_prev_day_pct: number;
  s6_max_prev_day_pct: number;
  s6_min_gap_pct: number;
  s6_max_gap_pct: number;
  s6_target_pct: number;
  s6_stop_pct: number;
  s6_max_hold_minutes: number;
}

interface TSignalFeeConfig {
  commission: number;
  stamp_tax: number;
  slippage: number;
  min_round: number;
}

interface TSignalConfig {
  symbol: string;
  position_shares: number;
  trend_veto: boolean;
  rules: TSignalRuleParams;
  fees: TSignalFeeConfig;
}

interface TSignalHealth {
  last_snapshot_at: string;
  streaming: boolean;
  vwap: number;
  ma5: number;
  vol_ratio: number;
  day: string;
  prev_close: number;
  prev_volume: number;
}

interface TSignalSnapshot {
  bars: MinuteBar[];
  signals: TSignalRecord[];
  ledger: TSignalLedgerEntry[];
  trend: TSignalTrendVeto;
  cooldown: TSignalCooldown;
  health: TSignalHealth;
}

interface StockSearchResult {
  code: string;
  name: string;
  exchange?: string;
  type?: string;
}

interface MarketMinutePoint {
  Time: string;
  Price: number;
  Number: number;
}

interface MarketMinuteResponse {
  date: string;
  Count: number;
  List: MarketMinutePoint[];
}

interface MarketDailyKline {
  Time: string;
  Last: number;
  Close: number;
  Volume: number;
}

interface MarketDailyKlineResponse {
  List: MarketDailyKline[];
}

interface IntradayReplay {
  date: string;
  bars: MinuteBar[];
  frames: IntradayReplayFrame[];
  skipped_rules: string[];
  prev_close: number;
  summary: IntradayReplaySummary;
}

interface IntradayReplayFrame {
  time: string;
  signals: TSignalRecord[];
  ledger: TSignalLedgerEntry[];
  trend: TSignalTrendVeto;
  cooldown: TSignalCooldown;
}

interface IntradayReplayResult {
  date: string;
  frames: IntradayReplayFrame[];
  skipped_rules?: string[];
  summary: IntradayReplaySummary;
}

interface IntradayReplaySummary {
  closed_trades: number;
  wins: number;
  win_rate: number;
  net_pnl: number;
  return_pct: number;
}

const replaySpeeds = [2, 4, 8, 16];
const replayInitialBars = 2;
const replayBaseInterval = 1000;
const intradaySlotCount = 240;

function stockCode(symbol?: string) {
  return (symbol || '').trim().replace(/^(?:sh|sz)/i, '').toUpperCase();
}

function intradayMinuteSlot(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return -1;
  const minute = date.getHours() * 60 + date.getMinutes();
  if (minute >= 9 * 60 + 30 && minute < 11 * 60 + 30) return minute - (9 * 60 + 30);
  if (minute >= 13 * 60 && minute < 15 * 60) return 120 + minute - 13 * 60;
  return -1;
}

const directionMeta: Record<string, { label: string; color: string }> = {
  SELL_FIRST: { label: '高抛', color: '#f5222d' },
  BUY_BACK: { label: '买回', color: '#52c41a' },
  REDUCE: { label: '减仓', color: '#fa8c16' },
  STAND_ASIDE: { label: '观望', color: '#8c8c8c' }
};

const statusMeta: Record<string, { label: string; color: string }> = {
  NEW: { label: '新触发', color: 'processing' },
  ACTIVE: { label: '有效', color: 'processing' },
  EXECUTED: { label: '已成交', color: 'success' },
  CANCELLED: { label: '已放弃', color: 'default' }
};

const trendMeta: Record<string, { label: string; color: string }> = {
  range: { label: '震荡', color: 'green' },
  strong_up: { label: '强势单边', color: 'red' },
  strong_down: { label: '弱势单边', color: 'blue' },
  limit: { label: '涨跌停附近', color: 'default' }
};

function fmtTime(value?: string) {
  if (!value) return '--';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString('zh-CN', { hour12: false });
}

function fmtPrice(value?: number) {
  if (value === undefined || value === null || !Number.isFinite(value)) return '--';
  return Number(value).toFixed(2);
}

function fmtPnl(value?: number) {
  if (value === undefined || value === null || !Number.isFinite(value)) return '--';
  const sign = value > 0 ? '+' : '';
  return `${sign}${Number(value).toFixed(2)}`;
}

function minuteReplayTime(date: string, time: string) {
  const normalizedDate = date.replace(/\D/g, '');
  const normalizedTime = time.trim();
  if (normalizedDate.length !== 8 || !/^\d{2}:\d{2}$/.test(normalizedTime)) return '';
  // TDX 分时点的时间是分钟区间结束时刻(09:31 表示 09:30-09:31)。
  // 回放统一到信号引擎使用的分钟起点,才能和 240 个交易槽位一一对应。
  const [hour, minute] = normalizedTime.split(':').map(Number);
  const slotStart = hour * 60 + minute - 1;
  const mappedHour = Math.floor(slotStart / 60).toString().padStart(2, '0');
  const mappedMinute = (slotStart % 60).toString().padStart(2, '0');
  return `${normalizedDate.slice(0, 4)}-${normalizedDate.slice(4, 6)}-${normalizedDate.slice(6, 8)}T${mappedHour}:${mappedMinute}:00+08:00`;
}

function minuteReplayChartTime(date: string, time: string) {
  const normalizedDate = date.replace(/\D/g, '');
  const normalizedTime = time.trim();
  if (normalizedDate.length !== 8 || !/^\d{2}:\d{2}$/.test(normalizedTime)) return '';
  return `${normalizedDate.slice(0, 4)}-${normalizedDate.slice(4, 6)}-${normalizedDate.slice(6, 8)}T${normalizedTime}:00+08:00`;
}

function minuteReplayBars(payload: MarketMinuteResponse): MinuteBar[] {
  return (payload.List || [])
    .map((point) => {
      const close = Number(point.Price) / 1000;
      // TDX minute Number is measured in lots; MinuteBar uses shares.
      const volume = Math.max(0, Number(point.Number) || 0) * 100;
      return {
        time: minuteReplayTime(payload.date, point.Time),
        chart_time: minuteReplayChartTime(payload.date, point.Time),
        open: close,
        high: close,
        low: close,
        close,
        volume,
        amount: close * volume,
        complete: true,
        trusted: true
      };
    })
    .filter((bar) => bar.time && Number.isFinite(bar.close) && bar.close > 0);
}

function tradingDate(value: string) {
  return value.replace(/\D/g, '').slice(0, 8);
}

// 历史分时必须使用该交易日对应的昨收，不能复用实时订阅标的的 prev_close。
function replayPrevClose(payload: MarketDailyKlineResponse, date: string) {
  const targetDate = tradingDate(date);
  const bars = payload.List || [];
  const index = bars.findIndex((bar) => tradingDate(bar.Time) === targetDate);
  if (index < 0) return 0;
  const recordedPreviousClose = Number(bars[index].Last) / 1000;
  if (Number.isFinite(recordedPreviousClose) && recordedPreviousClose > 0) return recordedPreviousClose;
  const precedingClose = Number(bars[index - 1]?.Close) / 1000;
  return Number.isFinite(precedingClose) && precedingClose > 0 ? precedingClose : 0;
}

function replayPreviousSession(payload: MarketDailyKlineResponse, date: string) {
  const targetDate = tradingDate(date);
  const bars = payload.List || [];
  const index = bars.findIndex((bar) => tradingDate(bar.Time) === targetDate);
  const previous = index > 0 ? bars[index - 1] : undefined;
  const previousLast = Number(previous?.Last) / 1000;
  const previousClose = Number(previous?.Close) / 1000;
  return {
    volume: Math.max(0, Number(previous?.Volume) || 0),
    dayReturn: previousLast > 0 && previousClose > 0
      ? (previousClose - previousLast) / previousLast * 100
      : 0
  };
}

// HQChart 绘制分时主图; 业务信号保留在独立覆盖层,避免依赖图表库的非公开标记接口。
function IntradayHQChart({
  symbol,
  bars,
  signals,
  selectedId,
  onSelect,
  emptyDescription,
  prevClose
}: {
  symbol: string;
  bars: MinuteBar[];
  signals: TSignalRecord[];
  selectedId?: string;
  onSelect?: (id: string) => void;
  emptyDescription: string;
  prevClose?: number;
}) {
  const chartRef = useRef<HTMLDivElement | null>(null);
  const [geometry, setGeometry] = useState<{ width: number; height: number; slots: number[]; prices: number[] } | null>(null);
  const hasBars = bars.length > 0;
  const signalPrices = useMemo(() => signals.flatMap((signal) => [signal.ref_price, signal.target, signal.stop]), [signals]);

  const updateGeometry = useCallback(() => {
    const container = chartRef.current;
    const next = container ? window.TDXHQChart?.getMinuteGeometry?.(container, signalPrices) : null;
    setGeometry(next || null);
  }, [signalPrices]);

  useEffect(() => {
    const container = chartRef.current;
    if (!container || !hasBars) return;
    const api = window.TDXHQChart;
    if (!api?.isAvailable?.() || !api.renderMinute?.(container, { symbol, minuteBars: bars, prevClose })) return;
    const observer = new ResizeObserver(() => {
      api.resize?.(container);
      window.requestAnimationFrame(updateGeometry);
    });
    observer.observe(container);
    const timer = window.setTimeout(updateGeometry, 80);
    return () => {
      window.clearTimeout(timer);
      observer.disconnect();
      api.destroy?.(container);
    };
  }, [hasBars, symbol, updateGeometry]);

  useEffect(() => {
    const container = chartRef.current;
    if (!container || !bars.length) return;
    window.TDXHQChart?.updateMinute?.(container, { minuteBars: bars, prevClose });
    const frame = window.requestAnimationFrame(updateGeometry);
    const timer = window.setTimeout(updateGeometry, 60);
    return () => {
      window.cancelAnimationFrame(frame);
      window.clearTimeout(timer);
    };
  }, [bars, prevClose, updateGeometry]);

  if (!bars.length) {
    return (
      <div className="minute-chart-empty">
        <Empty description={emptyDescription} image={Empty.PRESENTED_IMAGE_SIMPLE} />
      </div>
    );
  }

  return (
    <div className="intraday-hqchart">
      <div ref={chartRef} className="intraday-hqchart-surface" />
      {geometry ? (
        <svg className="intraday-signal-overlay" width={geometry.width} height={geometry.height} aria-label="日内信号标记">
          {signals.map((signal, index) => {
            const slot = intradayMinuteSlot(signal.time);
            const endSlot = intradayMinuteSlot(signal.expire_at);
            const x = geometry.slots[slot];
            const targetY = geometry.prices[index * 3 + 1];
            const stopY = geometry.prices[index * 3 + 2];
            if (!Number.isFinite(x) || !Number.isFinite(targetY) || !Number.isFinite(stopY)) return null;
            const active = signal.status === 'NEW' || signal.status === 'ACTIVE';
            const endX = active && Number.isFinite(geometry.slots[endSlot]) ? geometry.slots[endSlot] : x;
            return (
              <g key={`zone-${signal.id}`} opacity={selectedId && selectedId !== signal.id ? 0.35 : 1}>
                {active && endX > x ? <line x1={x} x2={endX} y1={targetY} y2={targetY} stroke="#16825d" strokeWidth={1} strokeDasharray="4 3" /> : null}
                {active && endX > x ? <line x1={x} x2={endX} y1={stopY} y2={stopY} stroke="#c73f3f" strokeWidth={1} strokeDasharray="4 3" /> : null}
              </g>
            );
          })}
          {signals.map((signal, index) => {
            const slot = intradayMinuteSlot(signal.time);
            const x = geometry.slots[slot];
            const y = geometry.prices[index * 3];
            if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
            const meta = directionMeta[signal.direction] || directionMeta.STAND_ASIDE;
            const isSell = signal.direction === 'SELL_FIRST' || signal.direction === 'REDUCE';
            return (
              <g
                key={signal.id}
                className="intraday-signal-mark"
                opacity={selectedId && selectedId !== signal.id ? 0.42 : 1}
                onClick={() => onSelect?.(signal.id)}
                style={{ cursor: 'pointer', pointerEvents: 'all' }}
              >
                <path d={`M${x - 6},${y + (isSell ? 8 : -8)} L${x},${y + (isSell ? -2 : 2)} L${x + 6},${y + (isSell ? 8 : -8)} Z`} fill={meta.color} />
                <text x={x} y={y + (isSell ? -12 : 19)} textAnchor="middle" fill={meta.color}>{signal.rule}</text>
              </g>
            );
          })}
        </svg>
      ) : null}
    </div>
  );
}

function SignalCard({
  signal,
  onAck,
  disableActions = false
}: {
  signal: TSignalRecord;
  onAck: (id: string, action: string) => void;
  disableActions?: boolean;
}) {
  const meta = directionMeta[signal.direction] || directionMeta.STAND_ASIDE;
  const status = statusMeta[signal.status] || statusMeta.NEW;
  const pending = signal.status === 'NEW' || signal.status === 'ACTIVE';
  return (
    <Card size="small" className="signal-card" bordered>
      <div className="signal-card-head">
        <Tag color={meta.color}>{meta.label}</Tag>
        <Tag>{signal.rule}</Tag>
        <Tag color={status.color}>{status.label}</Tag>
      </div>
      <div className="signal-card-body">
        <div className="signal-price-row">
          <Text strong style={{ fontSize: 18, color: meta.color }}>{fmtPrice(signal.ref_price)}</Text>
          <Text type="secondary">触发 {fmtTime(signal.time)}</Text>
        </div>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="目标价">
            <Text style={{ color: '#52c41a' }}>{fmtPrice(signal.target)}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="止损价">
            <Text style={{ color: '#f5222d' }}>{fmtPrice(signal.stop)}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="有效期">{fmtTime(signal.expire_at)} 前有效</Descriptions.Item>
          <Descriptions.Item label="仓位">{signal.position_hint || '--'}</Descriptions.Item>
          {signal.follow_up ? (
            <Descriptions.Item label="回补条件">
              {signal.follow_up.rule} @ {fmtPrice(signal.follow_up.ref)}
            </Descriptions.Item>
          ) : null}
        </Descriptions>
        <Text type="secondary" style={{ fontSize: 12 }}>{signal.trigger}</Text>
      </div>
      {pending && !disableActions ? (
        <div className="signal-card-actions">
          <Button type="primary" size="small" icon={<CheckOutlined />} onClick={() => onAck(signal.id, 'EXECUTED')}>
            已成交
          </Button>
          <Button size="small" icon={<CloseOutlined />} onClick={() => onAck(signal.id, 'CANCELLED')}>
            放弃
          </Button>
        </div>
      ) : null}
    </Card>
  );
}

// 默认配置(与后端 DefaultTSignalRuleParams/DefaultTSignalFeeConfig 对齐,用于恢复默认)。
const DEFAULT_TSIGNAL_CONFIG: TSignalConfig = {
  symbol: '',
  position_shares: 0,
  trend_veto: false,
  rules: {
    s1_enabled: false, s1_dev_pct: 0, s1_back_pct: 0, s1_stop_pct: 0,
    s2_enabled: false, s2_stop_pct: 0,
    s3_enabled: false, s3_lookback: 0,
    s4_enabled: false, s4_target_pct: 0, s4_vol_ratio: 0,
    s5_enabled: false, s5_move_pct: 0, s5_stop_pct: 0, s5_cooldown: 0,
    s6_enabled: false, s6_peak_dev_pct: 0, s6_pullback_pct: 0, s6_min_dev_pct: 0,
    s6_min_day_gain_pct: 0, s6_max_day_gain_pct: 0, s6_min_rsi: 0,
    s6_max_vwap_slope_pct: 0, s6_max_vol_ratio: 0,
    s6_min_prev_day_pct: 0, s6_max_prev_day_pct: 0,
    s6_min_gap_pct: 0, s6_max_gap_pct: 0,
    s6_target_pct: 0, s6_stop_pct: 0, s6_max_hold_minutes: 0
  },
  fees: { commission: 0, stamp_tax: 0, slippage: 0, min_round: 0 }
};

function emptyNumericFields<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, item === 0 ? undefined : item])) as T;
}

function formConfigValues(value: TSignalConfig): TSignalConfig {
  return {
    ...value,
    symbol: value.symbol || undefined as unknown as string,
    position_shares: value.position_shares || undefined as unknown as number,
    rules: emptyNumericFields(value.rules),
    fees: emptyNumericFields(value.fees)
  };
}

// 左侧参数栏:固定在执行台内,避免配置抢占分时与信号的垂直空间。
function ConfigBar({
  config,
  onSave,
  onReset,
  searchOptions,
  searching,
  onSearch
}: {
  config?: TSignalConfig;
  onSave: (config: TSignalConfig, force: boolean) => void;
  onReset: () => void;
  searchOptions: { value: string; label: string }[];
  searching: boolean;
  onSearch: (keyword: string) => void;
}) {
  const [form] = Form.useForm();
  useEffect(() => {
    if (config) {
      // resetFields ensures an empty saved config also clears the previous
      // AutoComplete/InputNumber values instead of leaving a stale draft visible.
      form.resetFields();
      form.setFieldsValue(formConfigValues(config));
    }
  }, [config, form]);

  const save = () => {
    form.validateFields().then((values) => {
      onSave({ ...config, ...values } as TSignalConfig, false);
    });
  };

  return (
    <Card
      size="small"
      className="work-card t-signal-config-panel"
      title={
        <div className="t-signal-panel-title">
          <div>
            <span>执行参数</span>
            <Text type="secondary">仓位、规则、成本</Text>
          </div>
          <small>POLICY</small>
        </div>
      }
      extra={
        <Space size={6}>
          <Button size="small" icon={<ReloadOutlined />} onClick={onReset}>恢复默认</Button>
          <Button type="primary" size="small" icon={<CheckOutlined />} onClick={save}>保存配置</Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" size="small" className="t-signal-config-form">
        <div className="t-signal-config-grid">
          <section className="t-signal-config-section t-signal-symbol-section">
            <div className="t-signal-config-section-head">
              <div><strong>交易标的</strong><span>搜索后点击保存配置才会生效</span></div>
            </div>
            <Form.Item name="symbol" label="股票代码/名称">
              <AutoComplete
                className="intraday-symbol-search"
                options={searchOptions}
                placeholder="搜索股票代码/名称"
                onSearch={onSearch}
                notFoundContent={searching ? '搜索中...' : '无结果'}
              />
            </Form.Item>
          </section>

          <section className="t-signal-config-section t-signal-config-section-base">
            <div className="t-signal-config-section-head">
              <div><strong>基础设置</strong><span>执行底仓与趋势保护</span></div>
            </div>
            <div className="t-signal-config-fields t-signal-config-fields-base">
              <Form.Item name="position_shares" label="底仓股数">
                <InputNumber min={0} step={3} />
              </Form.Item>
              <Form.Item className="t-signal-switch-field" name="trend_veto" label="趋势否决" valuePropName="checked">
                <Switch size="small" checkedChildren="开" unCheckedChildren="关" />
              </Form.Item>
            </div>
          </section>

          <section className="t-signal-config-section">
            <div className="t-signal-config-section-head">
              <div><strong>S1 VWAP 回归</strong><span>偏离后的均值回归</span></div>
              <Form.Item name={['rules', 's1_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
            </div>
            <div className="t-signal-config-fields t-signal-config-fields-three">
              <Form.Item name={['rules', 's1_dev_pct']} label="偏离 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's1_back_pct']} label="回归 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's1_stop_pct']} label="止损 %"><InputNumber step={0.1} /></Form.Item>
            </div>
          </section>

          <section className="t-signal-config-section t-signal-confirmation">
            <div className="t-signal-config-section-head">
              <div><strong>趋势确认</strong><span>均线支撑与 MACD 背离</span></div>
            </div>
            <div className="t-signal-rule-stack">
              <div className="t-signal-rule-row">
                <Form.Item name={['rules', 's2_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
                <strong>S2 均线支撑</strong>
                <Form.Item name={['rules', 's2_stop_pct']} label="止损 %"><InputNumber step={0.1} /></Form.Item>
              </div>
              <div className="t-signal-rule-row">
                <Form.Item name={['rules', 's3_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
                <strong>S3 MACD 背离</strong>
                <Form.Item name={['rules', 's3_lookback']} label="回看 min"><InputNumber min={1} /></Form.Item>
              </div>
            </div>
          </section>

          <section className="t-signal-config-section t-signal-config-section-wide t-signal-expansion">
            <div className="t-signal-config-section-head">
              <div><strong>扩展触发</strong><span>低频突破与急速反转</span></div>
            </div>
            <div className="t-signal-rule-stack">
              <div className="t-signal-rule-row t-signal-rule-row-three">
                <Form.Item name={['rules', 's4_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
                <strong>S4 放量突破</strong>
                <div className="t-signal-rule-parameters">
                  <Form.Item name={['rules', 's4_target_pct']} label="目标 %"><InputNumber step={0.1} /></Form.Item>
                  <Form.Item name={['rules', 's4_vol_ratio']} label="量比"><InputNumber step={0.1} /></Form.Item>
                </div>
              </div>
              <div className="t-signal-rule-row t-signal-rule-row-four">
                <Form.Item name={['rules', 's5_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
                <strong>S5 急拉反转</strong>
                <div className="t-signal-rule-parameters t-signal-rule-parameters-three">
                  <Form.Item name={['rules', 's5_move_pct']} label="幅度 %"><InputNumber step={0.1} /></Form.Item>
                  <Form.Item name={['rules', 's5_stop_pct']} label="止损 %"><InputNumber step={0.1} /></Form.Item>
                  <Form.Item name={['rules', 's5_cooldown']} label="冷却 min"><InputNumber min={0} /></Form.Item>
                </div>
              </div>
            </div>
          </section>

          <section className="t-signal-config-section t-signal-config-section-wide t-signal-expansion">
            <div className="t-signal-config-section-head">
              <div><strong>S6 标的校准回落</strong><span>5 分钟 VWAP 冲高回落，下一根成交</span></div>
              <Form.Item name={['rules', 's6_enabled']} valuePropName="checked"><Switch size="small" /></Form.Item>
            </div>
            <div className="t-signal-config-fields t-signal-config-fields-four">
              <Form.Item name={['rules', 's6_peak_dev_pct']} label="峰值偏离 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's6_pullback_pct']} label="回落确认 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's6_min_dev_pct']} label="最小偏离 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's6_min_rsi']} label="RSI6 下限"><InputNumber step={1} /></Form.Item>
              <Form.Item name={['rules', 's6_min_day_gain_pct']} label="日涨幅下限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_max_day_gain_pct']} label="日涨幅上限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_min_gap_pct']} label="缺口下限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_max_gap_pct']} label="缺口上限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_min_prev_day_pct']} label="昨涨幅下限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_max_prev_day_pct']} label="昨涨幅上限 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_max_vwap_slope_pct']} label="VWAP斜率上限 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's6_max_vol_ratio']} label="量比上限"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_target_pct']} label="回补目标 %"><InputNumber step={0.1} /></Form.Item>
              <Form.Item name={['rules', 's6_stop_pct']} label="保护止损 %"><InputNumber step={0.5} /></Form.Item>
              <Form.Item name={['rules', 's6_max_hold_minutes']} label="最长持有 min"><InputNumber min={5} step={5} /></Form.Item>
            </div>
          </section>

          <section className="t-signal-config-section t-signal-config-section-fees">
            <div className="t-signal-config-section-head">
              <div><strong>交易成本</strong><span>用于往返收益过滤</span></div>
            </div>
            <div className="t-signal-config-fields t-signal-config-fields-four">
              <Form.Item name={['fees', 'commission']} label="佣金 %"><InputNumber step={0.001} /></Form.Item>
              <Form.Item name={['fees', 'stamp_tax']} label="印花税 %"><InputNumber step={0.001} /></Form.Item>
              <Form.Item name={['fees', 'slippage']} label="滑点 %"><InputNumber step={0.001} /></Form.Item>
              <Form.Item name={['fees', 'min_round']} label="最小往返 %"><InputNumber step={0.05} /></Form.Item>
            </div>
          </section>
        </div>
      </Form>
    </Card>
  );
}

export function IntradayTradingWorkspace() {
  const [snapshot, setSnapshot] = useState<TSignalSnapshot>();
  const [config, setConfig] = useState<TSignalConfig>();
  const [searchOptions, setSearchOptions] = useState<{ value: string; label: string }[]>([]);
  const [stockNames, setStockNames] = useState<Record<string, string>>({});
  const [searching, setSearching] = useState(false);
  const [configExpanded, setConfigExpanded] = useState(true);
  const [selectedSignalId, setSelectedSignalId] = useState<string>();
  const [selectedSignal, setSelectedSignal] = useState<TSignalRecord>();
  const [replay, setReplay] = useState<IntradayReplay>();
  const [replayLoading, setReplayLoading] = useState(false);
  const [replayCursor, setReplayCursor] = useState(0);
  const [replaySpeed, setReplaySpeed] = useState(2);
  const [replayPaused, setReplayPaused] = useState(false);
  const [pendingSymbolChange, setPendingSymbolChange] = useState<TSignalConfig>();
  const snapshotRequestRef = useRef(0);

  const loadState = useCallback(async () => {
    try {
      const stateData = await apiFetch<{ config: TSignalConfig }>('/api/t-signal/state');
      setConfig(stateData.config);
    } catch {
      // 服务未启动时静默,轮询自然恢复
    }
  }, []);

  const loadSnapshot = useCallback(async () => {
    const requestId = ++snapshotRequestRef.current;
    try {
      const nextSnapshot = await apiFetch<TSignalSnapshot>('/api/t-signal/snapshot');
      if (requestId === snapshotRequestRef.current) setSnapshot(nextSnapshot);
    } catch {
      // 服务未启动时静默
    }
  }, []);

  useEffect(() => {
    loadState();
    loadSnapshot();
    const timer = window.setInterval(loadSnapshot, 2000);
    return () => window.clearInterval(timer);
  }, [loadState, loadSnapshot]);

  useEffect(() => {
    setReplay(undefined);
    setReplayCursor(0);
    setReplayPaused(false);
  }, [config?.symbol]);

  useEffect(() => {
    const code = stockCode(config?.symbol);
    if (!code || stockNames[code]) return;
    let cancelled = false;
    apiFetch<StockSearchResult[]>(`/api/search?keyword=${encodeURIComponent(code)}`)
      .then((results) => {
        const match = results.find((item) => item.code.toUpperCase() === code);
        if (!cancelled && match?.name) {
          setStockNames((names) => ({ ...names, [code]: match.name }));
        }
      })
      .catch(() => undefined);
    return () => { cancelled = true; };
  }, [config?.symbol]);

  useEffect(() => {
    if (!replay || replayPaused || replayCursor >= replay.bars.length) return;
    const timer = window.setInterval(() => {
      setReplayCursor((current) => Math.min(current + 1, replay.bars.length));
    }, replayBaseInterval / replaySpeed);
    return () => window.clearInterval(timer);
  }, [replay, replayPaused, replayCursor, replaySpeed]);

  const searchStocks = useCallback((keyword: string) => {
    const trimmed = keyword.trim();
    if (!trimmed) {
      setSearchOptions([]);
      return;
    }
    setSearching(true);
    apiFetch<StockSearchResult[]>(`/api/search?keyword=${encodeURIComponent(trimmed)}`)
      .then((results) => {
        setStockNames((names) => ({
          ...names,
          ...Object.fromEntries(results.map((item) => [stockCode(item.code), item.name]))
        }));
        setSearchOptions(
          results.slice(0, 8).map((item) => ({
            value: `${item.exchange || 'sz'}${item.code}`,
            label: `${item.name} ${item.code}`
          }))
        );
      })
      .catch(() => setSearchOptions([]))
      .finally(() => setSearching(false));
  }, []);

  const loadReplay = useCallback(async () => {
    const symbol = config?.symbol;
    if (!symbol) {
      message.info('请先订阅股票，再加载盘后回放');
      return;
    }
    setReplayPaused(false);
    setReplayLoading(true);
    try {
      const [payload, dailyKline] = await Promise.all([
        apiFetch<MarketMinuteResponse>(`/api/minute?code=${encodeURIComponent(symbol)}`),
        apiFetch<MarketDailyKlineResponse>(`/api/kline-history?code=${encodeURIComponent(symbol)}&period=day&count=100`)
      ]);
      const replayBars = minuteReplayBars(payload);
      if (replayBars.length < 2) throw new Error('该标的暂无足够的分钟数据');
      const prevClose = replayPrevClose(dailyKline, payload.date) || replayBars[0].close;
      const previousSession = replayPreviousSession(dailyKline, payload.date);
      const simulation = await apiFetch<IntradayReplayResult>('/api/t-signal/replay', {
        method: 'POST',
        body: JSON.stringify({
          bars: replayBars,
          prev_close: prevClose,
          prev_volume: previousSession.volume,
          prev_day_return: previousSession.dayReturn
        })
      });
      if (simulation.frames.length !== replayBars.length) throw new Error('回放信号计算结果不完整');
      setReplay({
        date: simulation.date || payload.date,
        bars: replayBars,
        frames: simulation.frames,
        skipped_rules: simulation.skipped_rules || [],
        prev_close: prevClose,
        summary: simulation.summary
      });
      setReplayCursor(Math.min(replayInitialBars, replayBars.length));
      setSelectedSignalId(undefined);
      setSelectedSignal(undefined);
      message.success(`开始回放 ${replayBars.length} 根分钟数据`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '盘后回放加载失败');
    } finally {
      setReplayLoading(false);
    }
  }, [config?.symbol]);

  const ackSignal = useCallback(
    async (id: string, action: string) => {
      try {
        await apiFetch(`/api/t-signal/${id}/ack`, { method: 'POST', body: JSON.stringify({ action }) });
        message.success(action === 'EXECUTED' ? '已标记成交,进入当日账本' : '已放弃该信号');
        loadSnapshot();
      } catch (error) {
        message.error(error instanceof Error ? error.message : '操作失败');
      }
    },
    [loadSnapshot]
  );

  const clearIntradayWorkspace = useCallback(() => {
    // 切标的时使尚未返回的旧轮询结果失效，避免旧分时图重新写回页面。
    snapshotRequestRef.current += 1;
    setSnapshot(undefined);
    setReplay(undefined);
    setReplayCursor(0);
    setReplayPaused(false);
    setSelectedSignalId(undefined);
    setSelectedSignal(undefined);
  }, []);

  const saveConfig = useCallback(
    async (next: TSignalConfig, force: boolean) => {
      const currentSymbol = (config?.symbol || '').trim();
      const nextSymbol = (next.symbol || '').trim();
      const symbolChanged = currentSymbol.toLowerCase() !== nextSymbol.toLowerCase();
      const hasIntradayData = Boolean(snapshot?.bars?.length || replay?.bars.length);
      if (!force && symbolChanged && hasIntradayData) {
        setPendingSymbolChange(next);
        return;
      }
      try {
        const saved = await apiFetch<{ config: TSignalConfig }>('/api/t-signal/config', {
          method: 'PUT',
          body: JSON.stringify({ config: next, force })
        });
        setConfig(saved.config);
        if (symbolChanged) clearIntradayWorkspace();
        setPendingSymbolChange(undefined);
        message.success(symbolChanged ? `已切换并保存 ${stockCode(nextSymbol) || '空标的'}` : '配置已保存');
        await Promise.all([loadState(), loadSnapshot()]);
      } catch (error) {
        message.error(error instanceof Error ? error.message : '保存失败');
      }
    },
    [clearIntradayWorkspace, config?.symbol, loadState, loadSnapshot, replay?.bars.length, snapshot?.bars?.length]
  );

  // 恢复默认参数:清空当前标的、底仓、开关和全部参数，不保留默认股票。
  const resetConfig = useCallback(async () => {
    try {
      const cleared = await apiFetch<{ config: TSignalConfig }>('/api/t-signal/config', {
        method: 'PUT',
        body: JSON.stringify({ config: DEFAULT_TSIGNAL_CONFIG, force: true })
      });
      setConfig(cleared.config);
      await Promise.all([loadState(), loadSnapshot()]);
      message.success('已清空配置');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '恢复失败');
    }
  }, [config, loadState, loadSnapshot]);

  const replayFrame = replay ? replay.frames[Math.max(0, replayCursor - 1)] : undefined;
  const trend = replayFrame?.trend || snapshot?.trend;
  const trendMetaValue = trend ? trendMeta[trend.status] || trendMeta.range : undefined;
  const cooldown = replayFrame?.cooldown || snapshot?.cooldown;
  const health = snapshot?.health;
  const signals = replayFrame?.signals || snapshot?.signals || [];
  const ledger = replayFrame?.ledger || snapshot?.ledger || [];
  const bars = snapshot?.bars || [];
  const displayedBars = replay ? replay.bars.slice(0, replayCursor) : bars;
  const displayedSignals = signals;
  const replayComplete = !!replay && replayCursor >= replay.bars.length;
  const subscribedCode = stockCode(config?.symbol);
  const subscribedLabel = subscribedCode
    ? `${stockNames[subscribedCode] ? `${stockNames[subscribedCode]} ` : ''}${subscribedCode}`
    : '';
  const chartEmptyDescription = !config?.symbol
    ? '请选择要订阅的股票'
    : displayedBars.length === 1
      ? '正在合成首根分钟 bar，下一分钟后显示图表'
      : health?.streaming
        ? '暂未获得可绘制的盘中分钟 bar'
        : '行情流暂未连接，无法合成分钟 bar';

  const totalPnl = useMemo(
    () => ledger.filter((entry) => entry.closed).reduce((sum, entry) => sum + (entry.net_pnl || 0), 0),
    [ledger]
  );

  const ledgerColumns = [
    { title: '卖出', dataIndex: 'sell_price', render: (_: unknown, entry: TSignalLedgerEntry) => `${fmtPrice(entry.sell_price)} (${fmtTime(entry.sell_time)})` },
    { title: '买回', dataIndex: 'buy_price', render: (_: unknown, entry: TSignalLedgerEntry) => (entry.closed ? `${fmtPrice(entry.buy_price)} (${fmtTime(entry.buy_time)})` : '未回补') },
    { title: '股数', dataIndex: 'shares' },
    { title: '净盈亏', dataIndex: 'net_pnl', render: (_: unknown, entry: TSignalLedgerEntry) => (
        <Text style={{ color: entry.closed ? ((entry.net_pnl || 0) >= 0 ? '#52c41a' : '#f5222d') : '#8c8c8c' }}>
          {entry.closed ? fmtPnl(entry.net_pnl) : '--'}
        </Text>
      ) }
  ];

  const showSignalDetail = useCallback(
    (id: string) => {
      setSelectedSignalId(id);
      const found = signals.find((sig) => sig.id === id);
      setSelectedSignal(found);
    },
    [signals]
  );

  return (
    <div className="intraday-workspace">
      <Modal
        open={Boolean(pendingSymbolChange)}
        title={<Space size={8}><ExclamationCircleOutlined /><span>切换日内交易标的？</span></Space>}
        okText="清空并切换"
        cancelText="保留当前标的"
        okButtonProps={{ danger: true }}
        onCancel={() => setPendingSymbolChange(undefined)}
        onOk={() => {
          if (pendingSymbolChange) void saveConfig(pendingSymbolChange, true);
        }}
      >
        当前分时图已有数据。切换至 {stockCode(pendingSymbolChange?.symbol)} 后，将清空分时数据、实时信号和当日账本，且无法恢复。
      </Modal>
      <div className="intraday-topbar">
        <div className="intraday-topbar-content">
          <Space wrap className="intraday-topbar-status">
            {config?.symbol ? <Tag color="blue">订阅中: {subscribedLabel}</Tag> : <Tag>未订阅(订阅即绑定做T标的)</Tag>}
            <Tag>底仓 {config?.position_shares ? `${config.position_shares} 股` : '未录入'}</Tag>
            {trendMetaValue ? <Tag color={trendMetaValue.color}>模式: {trendMetaValue.label}</Tag> : null}
            <Badge status={health?.streaming ? 'processing' : 'default'} text={health?.streaming ? '行情流正常' : '行情流离线'} />
            <Button
              size="small"
              type={configExpanded ? 'primary' : 'default'}
              icon={<SettingOutlined />}
              onClick={() => setConfigExpanded((open) => !open)}
            >
              {configExpanded ? '收起配置' : '展开配置'}
            </Button>
            <Button size="small" icon={<ReloadOutlined />} onClick={() => { loadState(); loadSnapshot(); }}>
              刷新
            </Button>
          </Space>
          <Space className="intraday-replay-controls" size={6}>
            <Button
              size="small"
              icon={<HistoryOutlined />}
              loading={replayLoading}
              onClick={() => {
                if (!replay || replayComplete) {
                  void loadReplay();
                  return;
                }
                setReplayPaused((paused) => !paused);
              }}
            >
              {!replay ? '盘后回放' : replayComplete ? '盘后回放' : replayPaused ? '继续回放' : '暂停回放'}
            </Button>
            <Button
              size="small"
              onClick={() => setReplaySpeed((speed) => replaySpeeds[(replaySpeeds.indexOf(speed) + 1) % replaySpeeds.length])}
            >
              X{replaySpeed}
            </Button>
            {replay ? <Tag color="gold">{replay.date} {replayCursor}/{replay.bars.length}</Tag> : null}
            {replayComplete && replay?.summary ? (
              <Tag color={replay.summary.net_pnl >= 0 ? 'green' : 'red'}>
                {replay.summary.closed_trades} 笔 / 胜率 {replay.summary.win_rate.toFixed(1)}% / 净收益 {fmtPnl(replay.summary.net_pnl)}
              </Tag>
            ) : null}
            {replay?.skipped_rules.length ? <Tag color="default">S5 需逐笔行情</Tag> : null}
          </Space>
        </div>
        {trend?.detail ? <div className="intraday-trend-detail"><Text type="secondary" style={{ fontSize: 12 }}>{trend.detail}</Text></div> : null}
        {cooldown?.active ? (
          <div className="intraday-cooldown"><Tag color="red">已暂停出信号: {cooldown.reason}</Tag></div>
        ) : null}
      </div>

      <div className={`intraday-body ${configExpanded ? 'intraday-body-with-config' : ''}`}>
        {configExpanded ? (
          <ConfigBar
            config={config}
            onSave={saveConfig}
            onReset={resetConfig}
            searchOptions={searchOptions}
            searching={searching}
            onSearch={searchStocks}
          />
        ) : null}
        <Card
          size="small"
          className="intraday-chart-panel"
          title={replay ? `分时图（盘后回放 ${replay.date}${replayComplete ? '，已完成' : ''}）` : '分时图（分钟bar合成 + 信号标记）'}
        >
          <IntradayHQChart
            symbol={config?.symbol || ''}
            bars={displayedBars}
            signals={displayedSignals}
            selectedId={selectedSignalId}
            onSelect={showSignalDetail}
            emptyDescription={chartEmptyDescription}
            prevClose={replay?.prev_close || health?.prev_close}
          />
        </Card>
        <div className="intraday-side">
          <Card size="small" className="intraday-ledger-panel" title={`当日账本 (净盈亏 ${fmtPnl(totalPnl)})`}>
            <Table<TSignalLedgerEntry>
              size="small"
              rowKey="id"
              columns={ledgerColumns}
              dataSource={ledger}
              pagination={false}
              locale={{ emptyText: '暂无交易记录' }}
            />
          </Card>
          <Card size="small" className="intraday-signals-panel" title={`${replay ? '回放信号卡' : '实时信号卡'} (${signals.length})`}>
            <div className="signal-card-list">
              {signals.length === 0 ? <Empty description={replay ? '当前回放进度尚未触发信号' : '暂无信号(等待行情与规则触发)'} image={Empty.PRESENTED_IMAGE_SIMPLE} /> : null}
              {[...signals].reverse().map((sig) => (
            <SignalCard key={sig.id} signal={sig} onAck={ackSignal} disableActions={Boolean(replay)} />
              ))}
            </div>
          </Card>
        </div>
      </div>

      <Modal
        open={Boolean(selectedSignal)}
        footer={null}
        title="信号详情"
        onCancel={() => { setSelectedSignal(undefined); setSelectedSignalId(undefined); }}
      >
        {selectedSignal ? <SignalCard signal={selectedSignal} onAck={ackSignal} disableActions={Boolean(replay)} /> : null}
      </Modal>
    </div>
  );
}
