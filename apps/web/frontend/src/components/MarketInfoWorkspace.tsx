import { Button, Card, Empty, Input, Space, Table, Tag, Typography, message } from 'antd';
import { LinkOutlined, SearchOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useState } from 'react';
import { apiFetch } from '../lib/api';
import { formatSigned } from '../lib/format';
import type { HotMoneyTrade, MarketNotice, MarketResearchReport } from '../types';
import { StockQuoteModal } from './StockQuoteModal';

const { Text } = Typography;

type MarketInfoKind = 'hot-money' | 'research' | 'notice';

interface MarketInfoWorkspaceProps {
  kind: MarketInfoKind;
}

const config: Record<MarketInfoKind, { title: string; endpoint: string; hint: string }> = {
  'hot-money': {
    title: '游资动向',
    endpoint: '/api/market/hot-money',
    hint: '输入股票代码，查看营业部买卖明细'
  },
  research: {
    title: '个股研报',
    endpoint: '/api/market/research',
    hint: '输入股票代码，查看近一年研报'
  },
  notice: {
    title: '公司公告',
    endpoint: '/api/market/notice',
    hint: '输入股票代码，查看最新公告'
  }
};

function displayDate(value?: string) {
  return String(value || '').slice(0, 10) || '--';
}

function amountWan(value?: number) {
  const number = Number(value);
  return Number.isFinite(number) ? (number / 10000).toFixed(2) : '--';
}

function percentage(value?: number) {
  const number = Number(value);
  return Number.isFinite(number) ? `${number.toFixed(2)}%` : '--';
}

function ratingChangeName(value?: number) {
  switch (Number(value)) {
    case 0:
      return '调高';
    case 1:
      return '调低';
    case 2:
      return '首次';
    case 3:
      return '维持';
    case 4:
      return '无变化';
    default:
      return '--';
  }
}

function openExternal(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer');
}

async function resolveStockCode(input: string) {
  const value = input.trim().toUpperCase();
  if (!value) return '';
  if (/^\d{6}(\.[A-Z]{2})?$/.test(value)) {
    return value.slice(0, 6);
  }
  if (/^[A-Z]{2}\d{6}$/.test(value)) {
    return value.slice(2);
  }
  const matches = await apiFetch<Array<Record<string, unknown>>>(`/api/search?keyword=${encodeURIComponent(value)}`);
  const first = Array.isArray(matches) ? matches[0] : null;
  return String(first?.code || first?.Code || '').trim();
}

export function MarketInfoWorkspace({ kind }: MarketInfoWorkspaceProps) {
  const current = config[kind];
  const [code, setCode] = useState('');
  const [items, setItems] = useState<Array<MarketResearchReport | MarketNotice | HotMoneyTrade>>([]);
  const [loading, setLoading] = useState(false);
  const [loadedCode, setLoadedCode] = useState('');
  const [quoteTarget, setQuoteTarget] = useState<{ code: string; name?: string } | null>(null);

  const load = useCallback(async () => {
    let nextCode = '';
    setLoading(true);
    try {
      nextCode = await resolveStockCode(code);
      if (!nextCode) {
        message.warning('没有找到匹配的股票');
        return;
      }
      const data = await apiFetch<Array<MarketResearchReport | MarketNotice | HotMoneyTrade>>(
        `${current.endpoint}?code=${encodeURIComponent(nextCode)}`
      );
      setItems(Array.isArray(data) ? data : []);
      setLoadedCode(nextCode);
    } catch (error) {
      setItems([]);
      message.error(error instanceof Error ? error.message : `${current.title}加载失败`);
    } finally {
      setLoading(false);
    }
  }, [code, current.endpoint, current.title]);

  useEffect(() => {
    if (code.trim()) void load();
  }, []);

  return (
    <Card
      className={`work-card market-info-card market-info-${kind}`}
      title={
        <div className="market-panel-title">
          <strong>{current.title}</strong>
          <Text type="secondary">
            {loadedCode ? `${loadedCode} · ${items.length} 条` : '尚未查询'}
          </Text>
        </div>
      }
      extra={
        <Space className="market-info-toolbar">
          <Input
            value={code}
            placeholder="股票名称或代码"
            onChange={(event) => setCode(event.target.value)}
            onPressEnter={() => void load()}
            className="market-info-code-input"
          />
          <Button type="primary" icon={<SearchOutlined />} loading={loading} onClick={() => void load()}>
            查询
          </Button>
        </Space>
      }
    >
      <div className="market-info-meta">
        <Text type="secondary">数据源：东方财富公开接口</Text>
        <Text type="secondary">{loadedCode ? `查询标的 ${loadedCode}` : current.hint}</Text>
      </div>

      {kind === 'hot-money' ? (
        <Table<HotMoneyTrade>
          className="market-info-table"
          size="small"
          rowKey={(item, index) => `${item.SECUCODE || item.SECURITY_CODE || 'hot-money'}-${item.TRADE_DATE || index}`}
          dataSource={items as HotMoneyTrade[]}
          loading={loading}
          pagination={{ pageSize: 20, size: 'small', showSizeChanger: false }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无营业部买卖明细" /> }}
          scroll={{ x: 1180 }}
          columns={[
            { title: '日期', dataIndex: 'TRADE_DATE', width: 110, render: displayDate },
            {
              title: '股票',
              width: 130,
              render: (_value, item) => {
                const code = item.SECURITY_CODE || item.SECUCODE || '';
                return code ? (
                  <Space size={4}>
                    <Button
                      type="link"
                      className="code-link"
                      onClick={() => setQuoteTarget({ code, name: item.SECURITY_NAME_ABBR })}
                    >
                      {code.replace(/\.[A-Z]+$/, '')}
                    </Button>
                    <span>{item.SECURITY_NAME_ABBR || ''}</span>
                  </Space>
                ) : (
                  '--'
                );
              }
            },
            { title: '营业部', dataIndex: 'OPERATEDEPT_NAME', width: 300, ellipsis: true },
            { title: '买入额(万)', dataIndex: 'BUY_AMT_REAL', width: 120, render: amountWan },
            { title: '买入占比', dataIndex: 'BUY_RATIO', width: 100, render: percentage },
            { title: '卖出额(万)', dataIndex: 'SELL_AMT_REAL', width: 120, render: amountWan },
            { title: '卖出占比', dataIndex: 'SELL_RATIO', width: 100, render: percentage },
            {
              title: '净额(万)',
              width: 110,
              render: (_value, item) => {
                const net = Number(item.BUY_AMT_REAL || 0) - Number(item.SELL_AMT_REAL || 0);
                return <span className={net >= 0 ? 'market-up' : 'market-down'}>{formatSigned(net / 10000)}</span>;
              }
            },
            { title: '上榜原因', dataIndex: 'EXPLANATION', width: 300, ellipsis: true }
          ]}
        />
      ) : kind === 'research' ? (
        <Table<MarketResearchReport>
          className="market-info-table"
          size="small"
          rowKey={(item) => item.infoCode || `${item.stockCode}-${item.publishDate}`}
          dataSource={items as MarketResearchReport[]}
          loading={loading}
          pagination={{ pageSize: 15, size: 'small', showSizeChanger: false }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无个股研报" /> }}
          scroll={{ x: 1180 }}
          columns={[
            {
              title: '代码',
              dataIndex: 'stockCode',
              width: 100,
              render: (value: string, item) =>
                value ? (
                  <Button type="link" className="code-link" onClick={() => setQuoteTarget({ code: value, name: item.stockName })}>
                    {value.replace(/\.[A-Z]+$/, '')}
                  </Button>
                ) : (
                  '--'
                )
            },
            { title: '名称', dataIndex: 'stockName', width: 110 },
            { title: '行业', dataIndex: 'indvInduName', width: 120, render: (value) => value || '--' },
            {
              title: '标题',
              dataIndex: 'title',
              width: 360,
              ellipsis: true,
              render: (value: string, item) => (
                <Button
                  type="link"
                  className="market-info-link"
                  icon={<LinkOutlined />}
                  onClick={() => item.infoCode && openExternal(`https://pdf.dfcfw.com/pdf/H3_${item.infoCode}_1.pdf`)}
                >
                  {value || '--'}
                </Button>
              )
            },
            { title: '东财评级', dataIndex: 'emRatingName', width: 100, render: (value) => value || '--' },
            { title: '评级变动', dataIndex: 'ratingChange', width: 100, render: ratingChangeName },
            { title: '机构评级', dataIndex: 'sRatingName', width: 100, render: (value) => value || '--' },
            { title: '分析师', dataIndex: 'researcher', width: 180, render: (value) => value || '--' },
            { title: '机构', dataIndex: 'orgSName', width: 150, render: (value) => value || '--' },
            { title: '日期', dataIndex: 'publishDate', width: 110, render: displayDate }
          ]}
        />
      ) : (
        <Table<MarketNotice>
          className="market-info-table"
          size="small"
          rowKey={(item) => item.art_code || `${item.stock_code}-${item.notice_date}-${item.title}`}
          dataSource={items as MarketNotice[]}
          loading={loading}
          pagination={{ pageSize: 15, size: 'small', showSizeChanger: false }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无公司公告" /> }}
          scroll={{ x: 1050 }}
          columns={[
            {
              title: '代码',
              dataIndex: 'stock_code',
              width: 100,
              render: (value: string, item) =>
                value ? (
                  <Button type="link" className="code-link" onClick={() => setQuoteTarget({ code: value, name: item.stock_name })}>
                    {value.replace(/\.[A-Z]+$/, '')}
                  </Button>
                ) : (
                  '--'
                )
            },
            { title: '名称', dataIndex: 'stock_name', width: 110 },
            {
              title: '公告标题',
              dataIndex: 'title',
              width: 440,
              ellipsis: true,
              render: (value: string, item) => (
                <Button
                  type="link"
                  className="market-info-link"
                  icon={<LinkOutlined />}
                  onClick={() => item.art_code && openExternal(`https://pdf.dfcfw.com/pdf/H2_${item.art_code}_1.pdf`)}
                >
                  {value || '--'}
                </Button>
              )
            },
            { title: '公告类型', dataIndex: 'column_name', width: 150, render: (value) => value || '--' },
            { title: '公告日期', dataIndex: 'notice_date', width: 120, render: displayDate },
            { title: '发布时间', dataIndex: 'display_time', width: 180, render: (value) => value || '--' },
            { title: '标记', width: 80, render: () => <Tag color="blue">东财</Tag> }
          ]}
        />
      )}
      <StockQuoteModal target={quoteTarget} onClose={() => setQuoteTarget(null)} />
    </Card>
  );
}
