import { Button, Card, Modal, Space, Table, Tag, Typography, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { BarChartOutlined, CloseOutlined, ReloadOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { apiFetch } from '../lib/api';
import { HQChartPanel } from './HQChartPanel';
import { AIResearchReport } from './AIResearchReport';
import {
  formatAmount,
  formatPrice,
  formatSigned,
  normalizeSymbol,
  priceFromMilli,
  quoteAmountYuan,
  quoteKline,
  quoteVolumeShares
} from '../lib/format';
import type { Quote, WatchlistRow } from '../types';

const { Text } = Typography;

const initialRows: WatchlistRow[] = [];

function changeClassName(value?: string) {
  const number = Number.parseFloat(String(value || '').replace('%', ''));
  if (!Number.isFinite(number) || number === 0) return '';
  return number > 0 ? 'market-up' : 'market-down';
}

function bidAskRatio(quote: Quote): string {
  const buyTotal = (quote.BuyLevel || []).reduce((sum, item) => sum + Number(item?.Number || 0), 0);
  const sellTotal = (quote.SellLevel || []).reduce((sum, item) => sum + Number(item?.Number || 0), 0);
  const total = buyTotal + sellTotal;
  if (total <= 0) return '--';
  return formatSigned(((buyTotal - sellTotal) / total) * 100, '%');
}

function mergeQuote(row: WatchlistRow, quote?: Quote): WatchlistRow {
  const kline = quoteKline(quote);
  if (!quote || !kline) return row;
  const previousClose = priceFromMilli(kline.Last);
  const currentPrice = priceFromMilli(kline.Close);
  if (currentPrice <= 0) return row;
  const change = previousClose > 0 ? currentPrice - previousClose : 0;
  const changePercent = previousClose > 0 ? (change / previousClose) * 100 : 0;
  return {
    ...row,
    price: formatPrice(currentPrice),
    change: formatSigned(change),
    changePercent: formatSigned(changePercent, '%'),
    volume: formatAmount(quoteVolumeShares(quote)),
    amount: formatAmount(quoteAmountYuan(quote)),
    speed: Number.isFinite(Number(quote.Rate)) && Number(quote.Rate) !== 0 ? formatSigned(quote.Rate, '%') : '--',
    entrust: bidAskRatio(quote)
  };
}

export function WatchlistTable() {
  const [rows, setRows] = useState<WatchlistRow[]>(initialRows);
  const [loading, setLoading] = useState(false);
  const [updatedAt, setUpdatedAt] = useState('');
  const [selectedRow, setSelectedRow] = useState<WatchlistRow | null>(null);
  const [selectedQuote, setSelectedQuote] = useState<Quote | null>(null);
  const [quoteLoading, setQuoteLoading] = useState(false);
  const [quotePeriod, setQuotePeriod] = useState('day');

  const selectedKline = quoteKline(selectedQuote);
  const quotePrice = priceFromMilli(selectedKline?.Close);
  const quotePreviousClose = priceFromMilli(selectedKline?.Last);
  const quoteChange = quotePrice - quotePreviousClose;
  const quoteChangePercent = quotePreviousClose > 0 ? (quoteChange / quotePreviousClose) * 100 : 0;

  const openQuote = useCallback(async (row: WatchlistRow) => {
    setSelectedRow(row);
    setSelectedQuote(null);
    setQuotePeriod('day');
    setQuoteLoading(true);
    try {
      const quotes = await apiFetch<Quote[]>(`/api/quote?code=${encodeURIComponent(normalizeSymbol(row.code))}`);
      setSelectedQuote(Array.isArray(quotes) ? quotes[0] || null : null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '行情加载失败');
    } finally {
      setQuoteLoading(false);
    }
  }, []);

  const closeQuote = useCallback(() => {
    setSelectedRow(null);
    setSelectedQuote(null);
  }, []);

  const refreshQuotes = useCallback(async () => {
    const codes = rows.map((row) => normalizeSymbol(row.code)).filter(Boolean);
    if (!codes.length) return;
    setLoading(true);
    try {
      const quotes = await apiFetch<Quote[]>(`/api/quote?code=${encodeURIComponent(codes.join(','))}`);
      const quoteMap = new Map((Array.isArray(quotes) ? quotes : []).map((quote) => [normalizeSymbol(quote.Code), quote]));
      setRows((current) =>
        current.map((row) => {
          const code = normalizeSymbol(row.code);
          return mergeQuote(row, quoteMap.get(code));
        })
      );
      setUpdatedAt(new Date().toLocaleTimeString('zh-CN', { hour12: false }));
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '行情刷新失败');
    } finally {
      setLoading(false);
    }
  }, [rows]);

  useEffect(() => {
    refreshQuotes();
  }, []);

  const columns = useMemo<ColumnsType<WatchlistRow>>(
    () => [
      {
        title: '序号',
        dataIndex: 'index',
        width: 70,
        fixed: 'left',
        render: (_value, _record, index) => index + 1
      },
      {
        title: '代码',
        dataIndex: 'code',
        width: 104,
        fixed: 'left',
        render: (value: string, record) => (
          <Button type="link" className="code-link" onClick={() => openQuote(record)}>
            {value}
          </Button>
        )
      },
      {
        title: '名称',
        dataIndex: 'name',
        width: 130,
        fixed: 'left',
        render: (value: string) => <strong>{value}</strong>
      },
      {
        title: '涨幅 %',
        dataIndex: 'changePercent',
        width: 110,
        sorter: (a, b) => Number.parseFloat(a.changePercent || '0') - Number.parseFloat(b.changePercent || '0'),
        render: (value?: string) => <span className={changeClassName(value)}>{value || '--'}</span>
      },
      {
        title: '现价',
        dataIndex: 'price',
        width: 96,
        render: (value: string | undefined, record: WatchlistRow) => (
          <span className={changeClassName(record.changePercent)}>{value || '--'}</span>
        )
      },
      { title: '成交量', dataIndex: 'volume', width: 116 },
      { title: '成交额', dataIndex: 'amount', width: 116 },
      { title: '涨速 %', dataIndex: 'speed', width: 100 },
      {
        title: '涨跌',
        dataIndex: 'change',
        width: 96,
        render: (value: string | undefined, record: WatchlistRow) => (
          <span className={changeClassName(record.changePercent)}>{value || '--'}</span>
        )
      },
      { title: '主力净量', dataIndex: 'mainNetVolume', width: 112, render: (value?: string) => value || '--' },
      { title: '主力净流入', dataIndex: 'mainNetInflow', width: 128, render: (value?: string) => value || '--' },
      { title: '所属行业', dataIndex: 'industry', width: 150, render: (value?: string) => value || '--' },
      { title: '换手 %', dataIndex: 'turnover', width: 100, render: (value?: string) => value || '--' },
      {
        title: '分类标示',
        dataIndex: 'category',
        width: 112,
        render: (value?: string) => (value ? <Tag color="blue">{value}</Tag> : '--')
      },
      { title: '委比 %', dataIndex: 'entrust', width: 100, render: (value?: string) => value || '--' },
      { title: '量比', dataIndex: 'volumeRatio', width: 96, render: (value?: string) => value || '--' }
    ],
    [openQuote]
  );

  return (
    <Card
      className="work-card"
      title={
        <div>
          <span>自选列表</span>
          <Text type="secondary" className="card-subtitle">
            共 {rows.length} 只{updatedAt ? ` · 更新于 ${updatedAt}` : ' · 等待行情'}
          </Text>
        </div>
      }
      extra={
        <Space>
          <Button icon={<BarChartOutlined />}>自选分析</Button>
          <Button type="primary" icon={<ReloadOutlined />} loading={loading} onClick={refreshQuotes}>
            刷新
          </Button>
        </Space>
      }
    >
      <Table
        className="watchlist-table"
        rowKey="key"
        size="middle"
        loading={loading}
        columns={columns}
        dataSource={rows}
        pagination={false}
        scroll={{ x: 1740, y: 520 }}
      />
      <Modal
        open={Boolean(selectedRow)}
        onCancel={closeQuote}
        footer={null}
        width={1240}
        centered
        destroyOnHidden
        closeIcon={<CloseOutlined />}
        className="app-themed-modal watchlist-quote-modal"
        title={
          <div className="quote-dialog-title">
            <div>
              <strong>{selectedRow?.name || selectedQuote?.Name || '--'}</strong>
              <span>{normalizeSymbol(selectedRow?.code || selectedQuote?.Code || '')}</span>
            </div>
            <small>实时行情 · 五档盘口</small>
          </div>
        }
      >
        <div className="quote-dialog-toolbar">
          <Space wrap>
            {[
              ['minute', '分时'],
              ['day', '日K'],
              ['week', '周K'],
              ['month', '月K'],
              ['minute30', '30分'],
              ['minute5', '5分']
            ].map(([value, label]) => (
              <Button
                key={value}
                size="small"
                type={quotePeriod === value ? 'primary' : 'default'}
                onClick={() => setQuotePeriod(value)}
              >
                {label}
              </Button>
            ))}
          </Space>
          <Button size="small" icon={<ReloadOutlined />} loading={quoteLoading} onClick={() => selectedRow && openQuote(selectedRow)}>
            刷新
          </Button>
        </div>
        <div className="quote-dialog-grid">
          <div className="quote-dialog-chart">
            {selectedRow ? (
              <HQChartPanel
                key={`${selectedRow.code}-${quotePeriod}`}
                symbol={normalizeSymbol(selectedRow.code)}
                period={quotePeriod}
                count={800}
                pageSize={80}
              />
            ) : null}
          </div>
          <aside className="quote-dialog-side">
            <section className="quote-dialog-summary">
              <span>现价</span>
              <strong className={quoteChange >= 0 ? 'market-up' : 'market-down'}>{quotePrice > 0 ? formatPrice(quotePrice) : '--'}</strong>
              <div className={quoteChange >= 0 ? 'market-up' : 'market-down'}>
                {quotePreviousClose > 0 ? `${formatSigned(quoteChange)} · ${formatSigned(quoteChangePercent, '%')}` : '--'}
              </div>
            </section>
            {selectedRow ? <div className="quote-dialog-ai-action"><AIResearchReport symbol={normalizeSymbol(selectedRow.code)} name={selectedRow.name} /></div> : null}
            <section className="quote-dialog-metrics">
              {[
                ['开盘', selectedKline?.Open ? formatPrice(priceFromMilli(selectedKline.Open)) : '--'],
                ['最高', selectedKline?.High ? formatPrice(priceFromMilli(selectedKline.High)) : '--'],
                ['最低', selectedKline?.Low ? formatPrice(priceFromMilli(selectedKline.Low)) : '--'],
                ['成交量', formatAmount(quoteVolumeShares(selectedQuote))],
                ['成交额', formatAmount(quoteAmountYuan(selectedQuote))],
                ['涨速', selectedQuote?.Rate ? formatSigned(selectedQuote.Rate, '%') : '--']
              ].map(([label, value]) => (
                <div key={label}>
                  <span>{label}</span>
                  <strong>{value}</strong>
                </div>
              ))}
            </section>
            <section className="quote-dialog-levels">
              <h4>五档盘口</h4>
              <div className="quote-dialog-level-columns">
                <div>
                  <small>卖盘</small>
                  {(selectedQuote?.SellLevel || []).slice().reverse().map((level, index) => (
                    <div className="quote-dialog-level sell" key={`sell-${index}`}>
                      <span>卖{5 - index}</span>
                      <b>{level.Price ? formatPrice(priceFromMilli(level.Price)) : '--'}</b>
                      <em>{level.Number ? Math.round(level.Number / 100) : '--'}</em>
                    </div>
                  ))}
                </div>
                <div>
                  <small>买盘</small>
                  {(selectedQuote?.BuyLevel || []).map((level, index) => (
                    <div className="quote-dialog-level buy" key={`buy-${index}`}>
                      <span>买{index + 1}</span>
                      <b>{level.Price ? formatPrice(priceFromMilli(level.Price)) : '--'}</b>
                      <em>{level.Number ? Math.round(level.Number / 100) : '--'}</em>
                    </div>
                  ))}
                </div>
              </div>
            </section>
          </aside>
        </div>
      </Modal>
    </Card>
  );
}
