import { Tag } from 'antd';
import type { SelectionHorizon } from '../types';

interface TrackingHorizonCellProps {
  value?: SelectionHorizon;
}

function formatPercent(value?: number) {
  return Number.isFinite(value) ? `${Number(value) >= 0 ? '+' : ''}${Number(value).toFixed(2)}%` : '--';
}

function formatPrice(value?: number) {
  return Number.isFinite(value) && Number(value) > 0 ? Number(value).toFixed(3) : '--';
}

function statusLabel(value?: SelectionHorizon) {
  if (!value) return '待观察';
  if (value.status === 'complete') return value.success ? '达标' : '未达标';
  if (value.status === 'pending') return '观察中';
  return value.status === 'unavailable' ? '暂无数据' : value.status;
}

export function TrackingHorizonCell({ value }: TrackingHorizonCellProps) {
  const complete = value?.status === 'complete';
  const peakReturn = Number(value?.max_gain ?? value?.mfe);
  const peakPrice = Number(value?.max_gain_price || 0);
  const averageClose = Number(value?.average_close || 0);
  const averageCloseReturn = Number(value?.average_close_return);
  return <div className="tracking-horizon-cell">
    <div className="tracking-horizon-cell-head">
      <Tag color={complete ? (value?.success ? 'success' : 'error') : 'default'}>{statusLabel(value)}</Tag>
      <strong>{complete ? formatPercent(value?.close_return) : '--'}</strong>
    </div>
    <small>均收 {averageClose > 0 ? `${formatPrice(averageClose)} · ${formatPercent(averageCloseReturn)}` : '--'}</small>
    <small>周期最高 {Number.isFinite(peakReturn) ? `${formatPercent(peakReturn)}${peakPrice > 0 ? ` · ${formatPrice(peakPrice)}` : ''}` : '--'}</small>
  </div>;
}
