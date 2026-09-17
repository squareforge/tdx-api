import { Empty, Layout, Tabs } from 'antd';
import { useEffect, useState } from 'react';
import { AppHeader } from './components/AppHeader';
import { AIConfigsWorkspace } from './components/AIConfigsWorkspace';
import { AutomationsWorkspace } from './components/AutomationsWorkspace';
import { DataCenterWorkspace } from './components/DataCenterWorkspace';
import { DailyReviewWorkspace } from './components/DailyReviewWorkspace';
import { HistoricalBacktestWorkspace } from './components/HistoricalBacktestWorkspace';
import { SelectionResultsWorkspace } from './components/SelectionResultsWorkspace';
import { StrategiesWorkspace } from './components/StrategiesWorkspace';
import { TradingSystemWorkspace } from './components/TradingSystemWorkspace';
import { UniverseWorkspace } from './components/UniverseWorkspace';
import { WebhooksWorkspace } from './components/WebhooksWorkspace';
import { MarketWorkspace } from './components/MarketWorkspace';
import { KlineAnalysisWorkspace } from './components/KlineAnalysisWorkspace';
import { AISelectionWorkspace } from './components/AISelectionWorkspace';
import { FloatingResearchAssistant } from './components/FloatingResearchAssistant';
import { AlertsWorkspace } from './components/AlertsWorkspace';
import { WatchlistTable } from './components/WatchlistTable';
import { apiFetch } from './lib/api';
import type { ServiceStatusResult } from './types';

const primaryWorkspaces = [
  { key: 'market', label: '自选' },
  { key: 'proChart', label: '市场行情' },
  { key: 'klineAnalysis', label: 'K线分析' },
  { key: 'dataCenter', label: '数据中心' },
  { key: 'tradingSystem', label: '交易系统' },
  { key: 'selection', label: '选股' },
  { key: 'automations', label: '自动化' },
  { key: 'aiConfigs', label: 'AI 模型' },
  { key: 'alerts', label: '预警中心' },
  { key: 'webhooks', label: 'Webhook' }
];

const selectionWorkspaces = [
  { key: 'universe', label: '选股范围' },
  { key: 'strategies', label: '策略中心' },
  { key: 'selectionResults', label: '选股结果' },
  { key: 'aiSelection', label: 'AI 选股' },
  { key: 'dailyReview', label: '每日复盘' },
  { key: 'historicalBacktest', label: '历史回测' }
];

const workspaceLabels = Object.fromEntries(
  [...primaryWorkspaces, ...selectionWorkspaces].map((workspace) => [workspace.key, workspace.label])
);

const workspaceCodes: Record<string, string> = {
  market: 'WATCHLIST',
  proChart: 'MARKET_DATA',
  klineAnalysis: 'KLINE_ANALYSIS',
  dataCenter: 'DATA_CENTER',
  selectionResults: 'SIGNAL_RESULTS',
  dailyReview: 'DAILY_REVIEW',
  historicalBacktest: 'HISTORICAL_REPLAY',
  tradingSystem: 'TRADING_PLAN',
  universe: 'UNIVERSE',
  strategies: 'STRATEGY_LAB',
  automations: 'RUN_SCHEDULER',
  aiConfigs: 'MODEL_ROUTER',
  aiSelection: 'AI_SELECTION',
  alerts: 'RISK_CALENDAR',
  webhooks: 'WEBHOOKS'
};

const selectionWorkspaceKeys = new Set(selectionWorkspaces.map((workspace) => workspace.key));
const primaryWorkspaceKeys = new Set(primaryWorkspaces.map((workspace) => workspace.key));

function Placeholder({ name }: { name: string }) {
  return (
    <div className="placeholder-panel">
      <Empty description={`${name} 会在下一阶段迁移`} />
    </div>
  );
}

function WorkspaceContent({ workspace }: { workspace: string }) {
  switch (workspace) {
    case 'market':
      return <WatchlistTable />;
    case 'proChart':
      return <MarketWorkspace />;
    case 'klineAnalysis':
      return <KlineAnalysisWorkspace />;
    case 'dataCenter':
      return <DataCenterWorkspace />;
    case 'selectionResults':
      return <SelectionResultsWorkspace />;
    case 'dailyReview':
      return <DailyReviewWorkspace />;
    case 'historicalBacktest':
      return <HistoricalBacktestWorkspace />;
    case 'tradingSystem':
      return <TradingSystemWorkspace />;
    case 'universe':
      return <UniverseWorkspace />;
    case 'strategies':
      return <StrategiesWorkspace />;
    case 'automations':
      return <AutomationsWorkspace />;
    case 'aiConfigs':
      return <AIConfigsWorkspace />;
    case 'aiSelection':
      return <AISelectionWorkspace />;
    case 'alerts':
      return <AlertsWorkspace />;
    case 'webhooks':
      return <WebhooksWorkspace />;
    default:
      return <Placeholder name={workspaceLabels[workspace] || '工作区'} />;
  }
}

export default function App() {
  const [activePrimaryWorkspace, setActivePrimaryWorkspace] = useState(() => {
    const savedWorkspace = window.localStorage.getItem('tdx.activePrimaryWorkspace');
    return savedWorkspace && primaryWorkspaceKeys.has(savedWorkspace) ? savedWorkspace : 'market';
  });
  const [activeSelectionWorkspace, setActiveSelectionWorkspace] = useState(() => {
    const savedWorkspace = window.localStorage.getItem('tdx.activeSelectionWorkspace');
    return savedWorkspace && selectionWorkspaceKeys.has(savedWorkspace) ? savedWorkspace : 'selectionResults';
  });
  const [serviceStatus, setServiceStatus] = useState<ServiceStatusResult>();
  const [serviceLoading, setServiceLoading] = useState(false);
  const [serviceError, setServiceError] = useState('');

  useEffect(() => {
    let disposed = false;
    async function refreshStatus() {
      setServiceLoading(true);
      try {
        const data = await apiFetch<ServiceStatusResult>('/api/services/status');
        if (disposed) return;
        setServiceStatus(data);
        setServiceError('');
      } catch (error) {
        if (disposed) return;
        setServiceError(error instanceof Error ? error.message : '服务状态获取失败');
      } finally {
        if (!disposed) setServiceLoading(false);
      }
    }
    refreshStatus();
    const timer = window.setInterval(refreshStatus, 30000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    window.localStorage.setItem('tdx.activeSelectionWorkspace', activeSelectionWorkspace);
  }, [activeSelectionWorkspace]);

  useEffect(() => {
    window.localStorage.setItem('tdx.activePrimaryWorkspace', activePrimaryWorkspace);
  }, [activePrimaryWorkspace]);

  const selectionTabItems = selectionWorkspaces.map((workspace) => ({
    key: workspace.key,
    label: workspace.label,
    children: <WorkspaceContent workspace={workspace.key} />
  }));

  const tabItems = primaryWorkspaces.map((workspace) => ({
    key: workspace.key,
    label: workspace.label,
    children: workspace.key === 'selection' ? (
      <div className="selection-workspace">
        <Tabs
          className="selection-subtabs"
          activeKey={activeSelectionWorkspace}
          items={selectionTabItems}
          onChange={setActiveSelectionWorkspace}
          destroyOnHidden={false}
        />
      </div>
    ) : (
      <WorkspaceContent workspace={workspace.key} />
    )
  }));

  const activeWorkspace = activePrimaryWorkspace === 'selection' ? activeSelectionWorkspace : activePrimaryWorkspace;
  const activeWorkspaceLabel = workspaceLabels[activeWorkspace] || '工作台';

  return (
    <Layout className="page-shell">
      <div className="page-container">
        <AppHeader status={serviceStatus} loading={serviceLoading} error={serviceError} />
        <div className="terminal-bar" aria-label="当前工作区">
          <span className="terminal-path">TDX://LOCAL/{workspaceCodes[activeWorkspace] || activeWorkspace.toUpperCase()}</span>
          <strong>{activeWorkspaceLabel}</strong>
          <span>{serviceStatus?.checked_at ? `CHECKED ${new Date(serviceStatus.checked_at).toLocaleTimeString('zh-CN', { hour12: false })}` : 'CHECKING'}</span>
        </div>
        <Tabs
          className="workspace-tabs"
          activeKey={activePrimaryWorkspace}
          items={tabItems}
          onChange={setActivePrimaryWorkspace}
          destroyOnHidden={false}
        />
        <FloatingResearchAssistant workspace={activeWorkspaceLabel} />
      </div>
    </Layout>
  );
}
