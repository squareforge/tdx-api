import { Empty, Layout, Tabs } from 'antd';
import {
  AppstoreOutlined,
  BarChartOutlined,
  BellOutlined,
  ClusterOutlined,
  CloudServerOutlined,
  ControlOutlined,
  DatabaseOutlined,
  ExperimentOutlined,
  FileSearchOutlined,
  FundProjectionScreenOutlined,
  LineChartOutlined,
  PartitionOutlined,
  RobotOutlined,
  SettingOutlined,
  StarOutlined,
  ThunderboltOutlined,
  ToolOutlined,
  FundOutlined,
  MenuOutlined,
  CloseOutlined
} from '@ant-design/icons';
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { AppHeader } from './components/AppHeader';
import { AIConfigsWorkspace } from './components/AIConfigsWorkspace';
import { AutomationsWorkspace } from './components/AutomationsWorkspace';
import { DataCenterWorkspace } from './components/DataCenterWorkspace';
import { DailyReviewWorkspace } from './components/DailyReviewWorkspace';
import { HistoricalBacktestWorkspace } from './components/HistoricalBacktestWorkspace';
import { SelectionResultsWorkspace } from './components/SelectionResultsWorkspace';
import { StrategiesWorkspace } from './components/StrategiesWorkspace';
import { TradingSystemWorkspace } from './components/TradingSystemWorkspace';
import { IntradayTradingWorkspace } from './components/IntradayTradingWorkspace';
import { UniverseWorkspace } from './components/UniverseWorkspace';
import { WebhooksWorkspace } from './components/WebhooksWorkspace';
import { PlatformConfigWorkspace, defaultPlatformConfig } from './components/PlatformConfigWorkspace';
import { MarketWorkspace } from './components/MarketWorkspace';
import { KlineAnalysisWorkspace } from './components/KlineAnalysisWorkspace';
import { AISelectionWorkspace } from './components/AISelectionWorkspace';
import { FloatingResearchAssistant } from './components/FloatingResearchAssistant';
import { AlertsWorkspace } from './components/AlertsWorkspace';
import { WatchlistTable } from './components/WatchlistTable';
import { WorkspaceErrorBoundary } from './components/WorkspaceErrorBoundary';
import { apiFetch } from './lib/api';
import type { PlatformConfig, ServiceStatusResult } from './types';

const primaryWorkspaces = [
  { key: 'market', label: '自选', icon: <StarOutlined /> },
  { key: 'proChart', label: '市场行情', icon: <BarChartOutlined /> },
  { key: 'klineAnalysis', label: 'K线分析', icon: <LineChartOutlined /> },
  { key: 'dataCenter', label: '数据中心', icon: <DatabaseOutlined /> },
  { key: 'tradingSystem', label: '交易系统', icon: <FundProjectionScreenOutlined /> },
  { key: 'selection', label: '选股', icon: <FileSearchOutlined /> },
  { key: 'automations', label: '自动化', icon: <ThunderboltOutlined /> },
  { key: 'aiConfigs', label: 'AI 模型', icon: <RobotOutlined /> },
  { key: 'alerts', label: '预警中心', icon: <BellOutlined /> },
  { key: 'webhooks', label: 'Webhook', icon: <CloudServerOutlined /> },
  { key: 'platformConfig', label: '平台配置', icon: <SettingOutlined /> }
];

const selectionWorkspaces = [
  { key: 'universe', label: '选股范围', icon: <AppstoreOutlined /> },
  { key: 'strategies', label: '策略中心', icon: <ControlOutlined /> },
  { key: 'selectionResults', label: '选股结果', icon: <FundOutlined /> },
  { key: 'aiSelection', label: 'AI 选股', icon: <RobotOutlined /> },
  { key: 'dailyReview', label: '每日复盘', icon: <FileSearchOutlined /> },
  { key: 'historicalBacktest', label: '历史回测', icon: <ExperimentOutlined /> }
];

// 交易系统内子 tab:持仓与交易(原有) + 日内交易(日内做T信号驾驶舱)。
const tradingWorkspaces = [
  { key: 'holdingsTrading', label: '持仓与交易', icon: <PartitionOutlined /> },
  { key: 'intradayT', label: '日内交易', icon: <FundProjectionScreenOutlined /> }
];

const workspaceLabels = Object.fromEntries(
  [...primaryWorkspaces, ...selectionWorkspaces, ...tradingWorkspaces].map((workspace) => [workspace.key, workspace.label])
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
  holdingsTrading: 'HOLDINGS_TRADING',
  intradayT: 'INTRADAY_T',
  universe: 'UNIVERSE',
  strategies: 'STRATEGY_LAB',
  automations: 'RUN_SCHEDULER',
  aiConfigs: 'MODEL_ROUTER',
  aiSelection: 'AI_SELECTION',
  alerts: 'RISK_CALENDAR',
  webhooks: 'WEBHOOKS',
  platformConfig: 'PLATFORM_CONFIG'
};

const selectionWorkspaceKeys = new Set(selectionWorkspaces.map((workspace) => workspace.key));
const tradingWorkspaceKeys = new Set(tradingWorkspaces.map((workspace) => workspace.key));
const primaryWorkspaceKeys = new Set(primaryWorkspaces.map((workspace) => workspace.key));

type SidebarWorkspace = {
  key: string;
  label: string;
  icon: ReactNode;
  children?: typeof selectionWorkspaces;
};

const sidebarGroups = [
  { key: 'overview', label: '核心工作台', icon: <AppstoreOutlined />, items: primaryWorkspaces.slice(0, 4) },
  {
    key: 'research',
    label: '交易与研究',
    icon: <ClusterOutlined />,
    items: [
      { ...primaryWorkspaces[4], children: tradingWorkspaces },
      { ...primaryWorkspaces[5], children: selectionWorkspaces }
    ]
  },
  { key: 'operations', label: '自动化与配置', icon: <ToolOutlined />, items: primaryWorkspaces.slice(6) }
];

function Placeholder({ name }: { name: string }) {
  return (
    <div className="placeholder-panel">
      <Empty description={`${name} 会在下一阶段迁移`} />
    </div>
  );
}

function WorkspaceContent({ workspace, platformConfig, onPlatformConfigChange }: { workspace: string; platformConfig?: PlatformConfig; onPlatformConfigChange?: (config: PlatformConfig) => void }) {
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
    case 'holdingsTrading':
      return <TradingSystemWorkspace />;
    case 'intradayT':
      return <IntradayTradingWorkspace />;
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
    case 'platformConfig':
      return <PlatformConfigWorkspace config={platformConfig || defaultPlatformConfig} onChange={onPlatformConfigChange || (() => undefined)} />;
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
  const [activeTradingWorkspace, setActiveTradingWorkspace] = useState(() => {
    const savedWorkspace = window.localStorage.getItem('tdx.activeTradingWorkspace');
    return savedWorkspace && tradingWorkspaceKeys.has(savedWorkspace) ? savedWorkspace : 'holdingsTrading';
  });
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({
    overview: true,
    research: true,
    operations: true,
    tradingSystem: true,
    selection: true
  });
  const [serviceStatus, setServiceStatus] = useState<ServiceStatusResult>();
  const [serviceLoading, setServiceLoading] = useState(false);
  const [serviceError, setServiceError] = useState('');
  const [platformConfig, setPlatformConfig] = useState<PlatformConfig>(defaultPlatformConfig);

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
    let disposed = false;
    apiFetch<PlatformConfig>('/api/platform-config').then((config) => {
      if (!disposed) setPlatformConfig(config);
    }).catch(() => undefined);
    return () => { disposed = true; };
  }, []);

  useEffect(() => {
    document.documentElement.style.setProperty('--market-up', platformConfig.rise_color || defaultPlatformConfig.rise_color);
    document.documentElement.style.setProperty('--market-down', platformConfig.fall_color || defaultPlatformConfig.fall_color);
  }, [platformConfig]);

  useEffect(() => {
    window.localStorage.setItem('tdx.activeSelectionWorkspace', activeSelectionWorkspace);
  }, [activeSelectionWorkspace]);

  useEffect(() => {
    window.localStorage.setItem('tdx.activeTradingWorkspace', activeTradingWorkspace);
  }, [activeTradingWorkspace]);

  useEffect(() => {
    window.localStorage.setItem('tdx.activePrimaryWorkspace', activePrimaryWorkspace);
  }, [activePrimaryWorkspace]);

  useEffect(() => {
    if (!sidebarOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setSidebarOpen(false);
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [sidebarOpen]);

  const selectionTabItems = selectionWorkspaces.map((workspace) => ({
    key: workspace.key,
    label: workspace.label,
    children: workspace.key === 'historicalBacktest' ? (
      <WorkspaceErrorBoundary workspaceName="历史回测" onReset={() => setActiveSelectionWorkspace('dailyReview')}>
        <WorkspaceContent workspace={workspace.key} platformConfig={platformConfig} onPlatformConfigChange={setPlatformConfig} />
      </WorkspaceErrorBoundary>
    ) : <WorkspaceContent workspace={workspace.key} platformConfig={platformConfig} onPlatformConfigChange={setPlatformConfig} />
  }));

  const tradingTabItems = tradingWorkspaces.map((workspace) => ({
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
    ) : workspace.key === 'tradingSystem' ? (
      <div className="selection-workspace">
        <Tabs
          className="selection-subtabs"
          activeKey={activeTradingWorkspace}
          items={tradingTabItems}
          onChange={setActiveTradingWorkspace}
          destroyOnHidden={false}
        />
      </div>
    ) : (
        <WorkspaceContent workspace={workspace.key} platformConfig={platformConfig} onPlatformConfigChange={setPlatformConfig} />
    )
  }));

  const activeWorkspace =
    activePrimaryWorkspace === 'selection'
      ? activeSelectionWorkspace
      : activePrimaryWorkspace === 'tradingSystem'
        ? activeTradingWorkspace
        : activePrimaryWorkspace;
  const activeWorkspaceLabel = workspaceLabels[activeWorkspace] || '工作台';

  const selectWorkspace = (workspace: string, parent?: string) => {
    if (parent === 'selection') {
      setActivePrimaryWorkspace('selection');
      setActiveSelectionWorkspace(workspace);
    } else if (parent === 'tradingSystem') {
      setActivePrimaryWorkspace('tradingSystem');
      setActiveTradingWorkspace(workspace);
    } else {
      setActivePrimaryWorkspace(workspace);
    }
    setSidebarOpen(false);
  };

  const renderSidebarItem = (workspace: SidebarWorkspace, parent?: string) => {
    const selected = activeWorkspace === workspace.key;
    const children = workspace.children;
    const parentSelected = activePrimaryWorkspace === workspace.key;
    return (
      <div key={workspace.key} className="sidebar-item-wrap">
        <button
          type="button"
          className={`sidebar-item${selected || parentSelected ? ' active' : ''}${children ? ' has-children' : ''}`}
          onClick={() => children ? setExpandedGroups((current) => ({ ...current, [workspace.key]: !current[workspace.key] })) : selectWorkspace(workspace.key, parent)}
          aria-expanded={children ? Boolean(expandedGroups[workspace.key]) : undefined}
          aria-current={selected && !children ? 'page' : undefined}
        >
          <span className="sidebar-item-icon">{workspace.icon}</span>
          <span className="sidebar-item-label">{workspace.label}</span>
          {children ? <span className={`sidebar-chevron${expandedGroups[workspace.key] ? ' open' : ''}`}>›</span> : null}
        </button>
        {children && expandedGroups[workspace.key] ? (
          <div className="sidebar-subitems">
            {children.map((child) => (
              <button
                type="button"
                key={child.key}
                className={`sidebar-subitem${activeWorkspace === child.key ? ' active' : ''}`}
                onClick={() => selectWorkspace(child.key, workspace.key)}
                aria-current={activeWorkspace === child.key ? 'page' : undefined}
              >
                <span className="sidebar-subitem-dot" />
                <span>{child.label}</span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
    );
  };

  return (
    <Layout className="page-shell">
      <div className="workbench-shell">
        <button
          type="button"
          className="mobile-menu-trigger"
          aria-label={sidebarOpen ? '关闭菜单' : '打开菜单'}
          onClick={() => setSidebarOpen((open) => !open)}
        >
          {sidebarOpen ? <CloseOutlined /> : <MenuOutlined />}
        </button>
        <div className={`sidebar-backdrop${sidebarOpen ? ' visible' : ''}`} onClick={() => setSidebarOpen(false)} aria-hidden="true" />
        <aside className={`app-sidebar${sidebarOpen ? ' mobile-open' : ''}`} aria-label="主导航">
          <div className="sidebar-brand">
            <img className="sidebar-brand-mark" src="/favicon.svg" alt="" />
            <div className="sidebar-brand-copy">
              <strong>股研台</strong>
              <span>行情 · 研究 · 交易计划</span>
            </div>
          </div>
          <div className="sidebar-context">
            <span className="sidebar-context-dot" />
            <span>工作区导航</span>
            <span className="sidebar-context-code">LOCAL</span>
          </div>
          <nav className="sidebar-nav">
            {sidebarGroups.map((group) => (
              <section className="sidebar-group" key={group.key}>
                <button
                  type="button"
                  className="sidebar-group-title"
                  onClick={() => setExpandedGroups((current) => ({ ...current, [group.key]: !current[group.key] }))}
                  aria-expanded={expandedGroups[group.key]}
                >
                  <span className="sidebar-group-icon">{group.icon}</span>
                  <span>{group.label}</span>
                  <span className={`sidebar-chevron${expandedGroups[group.key] ? ' open' : ''}`}>›</span>
                </button>
                {expandedGroups[group.key] ? (
                  <div className="sidebar-group-items">
                    {group.items.map((workspace) => renderSidebarItem(workspace, group.key === 'research' ? workspace.key : undefined))}
                  </div>
                ) : null}
              </section>
            ))}
          </nav>
          <div className="sidebar-footer">
            <span className="sidebar-footer-line" />
            <span>TDX://LOCAL/{workspaceCodes[activeWorkspace] || activeWorkspace.toUpperCase()}</span>
          </div>
        </aside>
        <main className="page-container">
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
        </main>
      </div>
    </Layout>
  );
}
