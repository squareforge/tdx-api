import { Alert, Badge, Space, Spin, Typography } from 'antd';
import { ApiOutlined, ClockCircleOutlined } from '@ant-design/icons';
import type { ServiceStatusResult } from '../types';
import { localTime } from '../lib/format';

const { Text, Title } = Typography;

interface AppHeaderProps {
  status?: ServiceStatusResult;
  loading: boolean;
  error?: string;
}

export function AppHeader({ status, loading, error }: AppHeaderProps) {
  const healthyCount = status?.services.filter((item) => item.healthy).length || 0;
  const totalCount = status?.services.length || 0;

  return (
    <header className="app-header">
      <div className="brand-block">
        <img className="brand-mark" src="/favicon.svg" alt="" />
        <div>
          <Text className="eyebrow">A 股研究工作台</Text>
          <Title level={1}>股研台</Title>
          <Text className="subtitle">行情 · 研究 · 交易计划</Text>
        </div>
      </div>

      <div className="status-grid">
        <section className="status-card service-card">
          <div className="status-card-head">
            <Text className="status-label">
              <ApiOutlined /> 服务总览
            </Text>
            {loading ? (
              <Spin size="small" />
            ) : (
              <strong className={status?.ready ? 'healthy-text' : 'warning-text'}>
                {status?.ready ? '全部正常' : `${healthyCount}/${totalCount || '--'} 正常`}
              </strong>
            )}
          </div>
          {error ? (
            <Alert type="warning" showIcon={false} message={error} className="compact-alert" />
          ) : (
            <Space className="service-list" wrap size={[8, 6]}>
              {(status?.services || []).map((service) => (
                <Badge
                  key={service.id}
                  color={service.healthy ? '#4ade80' : '#f87171'}
                  text={
                    <span className="service-chip-text">
                      <b>{service.name}</b>
                      <em>{service.healthy ? '正常' : service.status === 'degraded' ? '降级' : '异常'}</em>
                    </span>
                  }
                />
              ))}
              {!status?.services?.length && !loading ? <Text type="secondary">暂无服务数据</Text> : null}
            </Space>
          )}
        </section>

        <section className="status-card time-card">
          <Text className="status-label">
            <ClockCircleOutlined /> 更新时间
          </Text>
          <strong>{localTime(status?.checked_at)}</strong>
          <span>每 30 秒自动检查</span>
        </section>
      </div>
    </header>
  );
}
