import { Button, Card, Input, Segmented, Space, Typography, message } from 'antd';
import { BgColorsOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import type { PlatformConfig } from '../types';

const { Text } = Typography;

export const defaultPlatformConfig: PlatformConfig = {
  id: 'default',
  rise_color: '#c93d35',
  fall_color: '#16805b'
};

function validHex(value: string) {
  return /^#[0-9a-f]{6}$/i.test(value.trim());
}

function configMode(config: PlatformConfig) {
  if (config.rise_color.toLowerCase() === defaultPlatformConfig.rise_color && config.fall_color.toLowerCase() === defaultPlatformConfig.fall_color) return 'red-up';
  if (config.rise_color.toLowerCase() === defaultPlatformConfig.fall_color && config.fall_color.toLowerCase() === defaultPlatformConfig.rise_color) return 'green-up';
  return 'custom';
}

export function PlatformConfigWorkspace({ config, onChange }: { config: PlatformConfig; onChange: (config: PlatformConfig) => void }) {
  const [draft, setDraft] = useState<PlatformConfig>(config);
  const [saving, setSaving] = useState(false);

  useEffect(() => { setDraft(config); }, [config]);

  const mode = useMemo(() => configMode(draft), [draft]);
  const valid = validHex(draft.rise_color) && validHex(draft.fall_color);

  function update(field: 'rise_color' | 'fall_color', value: string) {
    setDraft((current) => ({ ...current, [field]: value }));
  }

  function applyPreset(value: string | number) {
    if (value === 'red-up') setDraft((current) => ({ ...current, ...defaultPlatformConfig }));
    if (value === 'green-up') setDraft((current) => ({ ...current, rise_color: defaultPlatformConfig.fall_color, fall_color: defaultPlatformConfig.rise_color }));
  }

  async function save() {
    if (!valid) return message.warning('请输入有效的 6 位十六进制颜色');
    setSaving(true);
    try {
      const next = await apiFetch<PlatformConfig>('/api/platform-config', { method: 'PUT', body: JSON.stringify({ ...draft, rise_color: draft.rise_color.trim().toLowerCase(), fall_color: draft.fall_color.trim().toLowerCase() }) });
      setDraft(next);
      onChange(next);
      message.success('平台颜色配置已保存');
    } catch (error) {
      message.error(error instanceof Error ? error.message : '平台颜色配置保存失败');
    } finally {
      setSaving(false);
    }
  }

  async function reload() {
    try {
      const next = await apiFetch<PlatformConfig>('/api/platform-config');
      setDraft(next);
      onChange(next);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '平台配置加载失败');
    }
  }

  return (
    <div className="platform-config-workspace">
      <Card className="work-card platform-config-card" title={<div className="platform-config-title"><div><span>平台配置</span><Text type="secondary">统一控制行情与收益的涨跌颜色</Text></div><small>PLATFORM CONFIG</small></div>} extra={<Button aria-label="重新加载平台配置" icon={<ReloadOutlined />} onClick={() => void reload()} />}>
        <div className="platform-config-summary">
          <div><span>当前规则</span><strong>{mode === 'red-up' ? '红涨绿跌' : mode === 'green-up' ? '绿涨红跌' : '自定义颜色'}</strong><small>保存后所有工作区即时生效</small></div>
          <div className="platform-config-preview"><span>上涨 <strong className="market-up">+3.24%</strong></span><span>下跌 <strong className="market-down">-2.18%</strong></span></div>
        </div>

        <section className="platform-config-section">
          <div className="platform-config-section-head"><div><strong>涨跌颜色</strong><span>行情、复盘、回测和交易盈亏统一使用</span></div><BgColorsOutlined /></div>
          <Segmented block value={mode === 'custom' ? undefined : mode} onChange={applyPreset} options={[{ value: 'red-up', label: '红色涨 · 绿色跌' }, { value: 'green-up', label: '绿色涨 · 红色跌' }]} />
          <div className="platform-color-grid">
            <label className="platform-color-field"><span>上涨 / 正收益</span><div><input type="color" value={validHex(draft.rise_color) ? draft.rise_color : defaultPlatformConfig.rise_color} onChange={(event) => update('rise_color', event.target.value)} /><Input value={draft.rise_color} status={draft.rise_color && !validHex(draft.rise_color) ? 'error' : undefined} maxLength={7} onChange={(event) => update('rise_color', event.target.value)} /></div></label>
            <label className="platform-color-field"><span>下跌 / 负收益</span><div><input type="color" value={validHex(draft.fall_color) ? draft.fall_color : defaultPlatformConfig.fall_color} onChange={(event) => update('fall_color', event.target.value)} /><Input value={draft.fall_color} status={draft.fall_color && !validHex(draft.fall_color) ? 'error' : undefined} maxLength={7} onChange={(event) => update('fall_color', event.target.value)} /></div></label>
          </div>
          <div className="platform-config-note"><Text type="secondary">配置采用标准 HEX 颜色值。系统状态的成功、失败、警告颜色保持独立，不会被涨跌颜色覆盖。</Text><Space><Button onClick={() => setDraft(config)}>取消修改</Button><Button type="primary" icon={<SaveOutlined />} loading={saving} onClick={() => void save()}>保存配置</Button></Space></div>
        </section>
      </Card>
    </div>
  );
}
