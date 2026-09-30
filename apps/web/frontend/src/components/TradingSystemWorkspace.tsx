import { AutoComplete, Button, Card, Form, Input, InputNumber, Modal, Select, Space, Table, Tag, Typography, message } from 'antd';
import { CheckCircleOutlined, CloseOutlined, PlusOutlined, ReloadOutlined, SaveOutlined, WarningOutlined } from '@ant-design/icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiFetch } from '../lib/api';
import { normalizeSymbol, priceFromMilli, quoteKline } from '../lib/format';
import type { AICredential, MacroEventOverview, Quote, TradingSystemState, TradingTrade } from '../types';

const { Text } = Typography;

interface StockSearchResult {
  code: string;
  name: string;
  exchange?: string;
  type?: string;
}

interface KlinePoint {
  Time?: string;
  Open?: number;
  High?: number;
  Low?: number;
  Close?: number;
}

interface KlineHistory {
  List?: KlinePoint[];
}

type AnalysisSource = 'idle' | 'loading' | 'quote' | 'ai' | 'technical' | 'error';

interface TradeAnalysis {
  source: AnalysisSource;
  note: string;
  basis?: string;
  updatedAt?: string;
}

const autoAnalysisFields = ['currentPrice', 'invalidPrice', 'targetOne', 'targetTwo'] as const;
type AutoAnalysisField = (typeof autoAnalysisFields)[number];
const AI_ANALYSIS_TIMEOUT_MS = 18000;

const currencyFormatter = new Intl.NumberFormat('zh-CN', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2
});

function formatMoney(value?: number) {
  return currencyFormatter.format(Number(value || 0));
}

function formatShares(value?: number) {
  return Number(value || 0).toLocaleString('zh-CN');
}

function tradePnl(trade: TradingTrade) {
  if (trade.direction === 'sell') return 0;
  return (Number(trade.currentPrice || 0) - Number(trade.entryPrice || 0)) * Number(trade.shares || 0);
}

function tradePnlRate(trade: TradingTrade) {
  if (trade.direction === 'sell') return 0;
  const entryPrice = Number(trade.entryPrice || 0);
  if (entryPrice <= 0) return 0;
  return ((Number(trade.currentPrice || 0) - entryPrice) / entryPrice) * 100;
}

function tradeDirection(trade: TradingTrade) {
  return trade.direction === 'sell' ? 'sell' : 'buy';
}

function getAvailableShares(trades: TradingTrade[], stockCode: string, excludeId?: string) {
  const code = stockCode.trim();
  if (!code) return 0;
  return Math.max(0, trades.reduce((total, trade) => {
    if (trade.id === excludeId || trade.stockCode.trim() !== code) return total;
    const shares = Math.max(0, Number(trade.shares || 0));
    if (tradeDirection(trade) === 'sell') return total - shares;
    return trade.status === 'active' ? total + shares : total;
  }, 0));
}

function getCurrentHoldings(trades: TradingTrade[]) {
  const holdings = new Map<string, { shares: number; currentPrice: number }>();
  trades.forEach((trade) => {
    const code = trade.stockCode.trim();
    if (!code) return;
    const current = holdings.get(code) || { shares: 0, currentPrice: 0 };
    current.currentPrice = Number(trade.currentPrice || current.currentPrice || 0);
    const shares = Math.max(0, Number(trade.shares || 0));
    if (tradeDirection(trade) === 'sell') {
      current.shares -= shares;
    } else if (trade.status === 'active') {
      current.shares += shares;
    }
    holdings.set(code, current);
  });
  return holdings;
}

function signalLabel(value: boolean) {
  return value ? '已确认' : '待确认';
}

function numericPrice(value: unknown) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return number > 1000 ? priceFromMilli(number) : number;
}

function roundPrice(value: number) {
  return Math.max(0, Math.round(value * 100) / 100);
}

function formatPriceRange(low: number, high: number) {
  const normalizedLow = roundPrice(Math.min(low, high));
  const normalizedHigh = roundPrice(Math.max(low, high));
  return `${normalizedLow.toFixed(2)}-${normalizedHigh.toFixed(2)}`;
}

function average(values: number[]) {
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0;
}

function technicalAnalysis(rows: KlinePoint[], currentPrice: number, direction: 'buy' | 'sell') {
  const points = rows
    .map((item) => ({
      high: numericPrice(item.High),
      low: numericPrice(item.Low),
      close: numericPrice(item.Close)
    }))
    .filter((item) => item.high > 0 && item.low > 0 && item.close > 0);
  if (!points.length || currentPrice <= 0) return null;

  const recent20 = points.slice(-20);
  const recent60 = points.slice(-60);
  const ma20 = average(recent20.map((item) => item.close));
  const ma60 = average(recent60.map((item) => item.close));
  const recentHigh20 = Math.max(...recent20.map((item) => item.high));
  const recentLow20 = Math.min(...recent20.map((item) => item.low));
  const recentHigh60 = Math.max(...recent60.map((item) => item.high));
  const recentLow60 = Math.min(...recent60.map((item) => item.low));
  const trueRanges = points.slice(-15).map((item, index, slice) => {
    const previousClose = index > 0 ? slice[index - 1].close : item.close;
    return Math.max(item.high - item.low, Math.abs(item.high - previousClose), Math.abs(item.low - previousClose));
  });
  const atr14 = Math.max(0.01, average(trueRanges.slice(-14)));
  const tick = Math.max(0.01, currentPrice * 0.001);

  if (direction === 'sell') {
    const invalidPrice = roundPrice(Math.max(recentHigh20, ma20 + atr14 * 1.1));
    const firstLow = Math.max(0, Math.min(currentPrice - atr14 * 0.7, recentLow20 + atr14 * 0.25));
    const firstHigh = Math.max(firstLow + tick, Math.min(currentPrice - tick, recentLow20 + atr14 * 0.75));
    const secondHigh = Math.max(tick, Math.min(firstLow - tick, recentLow60 + atr14 * 0.25));
    const secondLow = Math.max(0, Math.min(secondHigh - tick, recentLow60 - atr14 * 0.75));
    return {
      invalidPrice,
      targetOne: formatPriceRange(firstLow, firstHigh),
      targetTwo: formatPriceRange(secondLow, secondHigh),
      basis: `日K近20/60日高低点、MA20/MA60与ATR14估算`
    };
  }

  const invalidPrice = roundPrice(Math.min(recentLow20, ma20 - atr14 * 1.1));
  const firstLow = Math.max(currentPrice, Math.max(ma20, recentHigh20 - atr14 * 0.6));
  const firstHigh = Math.max(firstLow + tick, recentHigh20 + atr14 * 0.25);
  const secondLow = Math.max(firstHigh + tick, recentHigh60 - atr14 * 0.35);
  const secondHigh = Math.max(secondLow + tick, recentHigh60 + atr14 * 0.75);
  return {
    invalidPrice,
    targetOne: formatPriceRange(firstLow, firstHigh),
    targetTwo: formatPriceRange(secondLow, secondHigh),
    basis: `日K近20/60日高低点、MA20/MA60与ATR14估算`
  };
}

function parseAnalysisResult(result: Record<string, unknown> | undefined, content: string) {
  let parsed = result || {};
  if (!Object.keys(parsed).length && content) {
    try {
      const candidate = JSON.parse(content);
      if (candidate && typeof candidate === 'object') parsed = candidate;
    } catch {
      return null;
    }
  }
  const invalidPrice = numericPrice(
    parsed.invalid_price ?? parsed.technical_invalid_price ?? parsed.invalid_point
  );
  const targetOne = String(
    parsed.first_observation_level ?? parsed.target_one ?? parsed.watch_level ?? ''
  ).trim();
  const targetTwo = String(
    parsed.strong_pressure_level ?? parsed.target_two ?? parsed.take_profit_level ?? ''
  ).trim();
  if (!invalidPrice || !targetOne || !targetTwo) return null;
  return {
    invalidPrice,
    targetOne,
    targetTwo,
    basis: String(parsed.level_basis ?? parsed.invalid_point_basis ?? 'AI基于行情与日K结构生成')
  };
}

async function withTimeout<T>(promise: Promise<T>, timeoutMS: number) {
  let timer: number | undefined;
  const timeout = new Promise<never>((_resolve, reject) => {
    timer = window.setTimeout(() => reject(new Error('AI分析超时')), timeoutMS);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer !== undefined) window.clearTimeout(timer);
  }
}

export function TradingSystemWorkspace() {
  const [state, setState] = useState<TradingSystemState | null>(null);
  const [macroOverview, setMacroOverview] = useState<MacroEventOverview | null>(null);
  const [loading, setLoading] = useState(false);
  const [quotesUpdatedAt, setQuotesUpdatedAt] = useState<string | null>(null);
  const [refreshingQuotes, setRefreshingQuotes] = useState(false);
  const [tradingTime, setTradingTime] = useState<boolean | null>(null);
  const [marketGateInfo, setMarketGateInfo] = useState<{ gate: string; close: number; ma20: number; date?: string } | null>(null);
  const workdayCacheRef = useRef<{ date: string; value: boolean } | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<TradingTrade | null>(null);
  const [stockSearchField, setStockSearchField] = useState<'name' | 'code' | null>(null);
  const [stockSearchResults, setStockSearchResults] = useState<StockSearchResult[]>([]);
  const [stockSearchLoading, setStockSearchLoading] = useState(false);
  const [analysis, setAnalysis] = useState<TradeAnalysis>({ source: 'idle', note: '选择股票后自动同步行情与建议' });
  const manualAnalysisFieldsRef = useRef<Set<AutoAnalysisField>>(new Set());
  const autoAnalysisFieldsRef = useRef<Set<AutoAnalysisField>>(new Set());
  const applyingAnalysisRef = useRef(false);
  const analysisRequestRef = useRef(0);
  const [form] = Form.useForm<TradingTrade>();

  const load = async () => {
    setLoading(true);
    try {
      setState(await apiFetch<TradingSystemState>('/api/trading-system'));
      void refreshMarketGate();
      try {
        setMacroOverview(await apiFetch<MacroEventOverview>('/api/macro-events/overview'));
      } catch {
        setMacroOverview(null);
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '交易系统加载失败');
    } finally {
      setLoading(false);
    }
  };

  async function isTradingTimeNow() {
    const now = new Date();
    const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
    let isWorkday = true;
    const cached = workdayCacheRef.current;
    if (cached?.date === today) {
      isWorkday = cached.value;
    } else {
      try {
        const workday = await apiFetch<{ is_workday?: boolean }>('/api/workday');
        isWorkday = Boolean(workday?.is_workday);
      } catch {
        const day = now.getDay();
        isWorkday = day !== 0 && day !== 6;
      }
      workdayCacheRef.current = { date: today, value: isWorkday };
    }
    if (!isWorkday) return false;
    const minutes = now.getHours() * 60 + now.getMinutes();
    // A股交易时段：9:30-11:30、13:00-15:00
    return (minutes >= 570 && minutes <= 690) || (minutes >= 780 && minutes <= 900);
  }

  useEffect(() => {
    load();
    const timer = window.setInterval(async () => {
      try { setMacroOverview(await apiFetch<MacroEventOverview>('/api/macro-events/overview')); } catch { /* optional risk context */ }
      void refreshMarketGate();
    }, 60000);
    return () => window.clearInterval(timer);
  }, []);

  const trades = state?.trades || [];

  async function refreshMarketGate() {
    try {
      const history = await apiFetch<KlineHistory>('/api/kline-history?code=sh000001&type=day&limit=25');
      const rows = Array.isArray(history?.List)
        ? history.List.filter((item) => numericPrice(item.Close) > 0)
        : [];
      if (rows.length < 20) return;
      const closes = rows.map((item) => numericPrice(item.Close));
      const close = closes[closes.length - 1];
      const ma20 = closes.slice(-20).reduce((sum, value) => sum + value, 0) / 20;
      setMarketGateInfo({
        gate: close >= ma20 ? '开' : '关',
        close,
        ma20,
        date: String(rows[rows.length - 1].Time || '').slice(0, 10)
      });
    } catch { /* 大盘开关获取失败时保持现状 */ }
  }

  // 批量拉取活跃持仓的实时行情，仅更新内存中的现价（保存交易卡时才持久化）
  const refreshMarketPrices = useCallback(async () => {
    const activeCodes = Array.from(new Set(
      trades
        .filter((trade) => tradeDirection(trade) === 'buy' && trade.status === 'active' && String(trade.stockCode || '').trim())
        .map((trade) => normalizeSymbol(trade.stockCode))
    ));
    if (!activeCodes.length) return;
    const quotes = await apiFetch<Quote[]>(`/api/quote?code=${encodeURIComponent(activeCodes.join(','))}`);
    const priceMap = new Map<string, number>();
    (Array.isArray(quotes) ? quotes : []).forEach((quote) => {
      const code = normalizeSymbol(quote.Code);
      const price = priceFromMilli(quoteKline(quote)?.Close);
      if (code && price > 0) priceMap.set(code, price);
    });
    if (!priceMap.size) return;
    setState((current) => current ? {
      ...current,
      trades: current.trades.map((trade) => {
        if (tradeDirection(trade) !== 'buy' || trade.status !== 'active') return trade;
        const price = priceMap.get(normalizeSymbol(trade.stockCode));
        return price ? { ...trade, currentPrice: price } : trade;
      })
    } : current);
    setQuotesUpdatedAt(new Date().toLocaleTimeString('zh-CN', { hour12: false }));
  }, [trades]);

  const refreshMarketPricesRef = useRef(refreshMarketPrices);
  refreshMarketPricesRef.current = refreshMarketPrices;

  // 交易时间内每 30 秒自动刷新行情，非交易时间由用户点击按钮手动刷新
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      const trading = await isTradingTimeNow();
      if (cancelled) return;
      setTradingTime(trading);
      if (trading) {
        try {
          await refreshMarketPricesRef.current();
        } catch { /* 行情刷新失败时静默，等待下一轮 */ }
      }
    };
    tick();
    const timer = window.setInterval(tick, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  const handleManualQuoteRefresh = async () => {
    setRefreshingQuotes(true);
    try {
      await refreshMarketPrices();
    } catch (error) {
      message.warning(error instanceof Error ? error.message : '行情刷新失败');
    } finally {
      setRefreshingQuotes(false);
    }
  };
  const direction = Form.useWatch('direction', form) || (editing?.direction === 'sell' ? 'sell' : 'buy');
  const stockCode = Form.useWatch('stockCode', form) || '';
  const stockName = Form.useWatch('stockName', form) || '';
  const plannedR = Number(Form.useWatch('plannedR', form) || 0);
  const actualR = Number(Form.useWatch('actualR', form) || 0);
  const availableShares = useMemo(
    () => getAvailableShares(trades, stockCode, editing?.id),
    [trades, stockCode, editing?.id]
  );
  const stockSearchKeyword = stockSearchField === 'name' ? stockName : stockCode;
  const stockSearchOptions = useMemo(
    () => stockSearchResults.map((item) => ({
      value: stockSearchField === 'name' ? item.name : item.code,
      stock: item,
      label: (
        <div className="trading-stock-option">
          <strong>{item.name}</strong>
          <span>{item.code} · {(item.exchange || '').toUpperCase()} · {item.type || '股票'}</span>
        </div>
      )
    })),
    [stockSearchField, stockSearchResults]
  );

  useEffect(() => {
    const keyword = String(stockSearchKeyword || '').trim();
    if (!stockSearchField || !keyword) {
      setStockSearchResults([]);
      setStockSearchLoading(false);
      return;
    }

    let cancelled = false;
    const timer = window.setTimeout(async () => {
      setStockSearchLoading(true);
      try {
        const results = await apiFetch<StockSearchResult[]>(
          `/api/search?keyword=${encodeURIComponent(keyword)}`
        );
        if (cancelled) return;
        const nextResults = Array.isArray(results) ? results.slice(0, 8) : [];
        setStockSearchResults(nextResults);
        const exact = nextResults.find((item) => (
          stockSearchField === 'code'
            ? item.code.toUpperCase() === keyword.toUpperCase()
            : item.name.toLowerCase() === keyword.toLowerCase()
        ));
        if (exact) {
          form.setFieldsValue({ stockName: exact.name, stockCode: exact.code });
        }
      } catch {
        if (!cancelled) setStockSearchResults([]);
      } finally {
        if (!cancelled) setStockSearchLoading(false);
      }
    }, 220);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [form, stockSearchField, stockSearchKeyword]);

  const stats = useMemo(() => {
    const account = state?.account || { principal: 0, totalAssets: 0, marketValue: 0, dailyProfit: 0, maxTradeRisk: 0, maxPositionWeight: 0 };
    return account;
  }, [state]);
  const currentHoldings = useMemo(() => (
    Array.from(getCurrentHoldings(trades).values()).filter((item) => item.shares > 0)
  ), [trades]);
  const holdingMarketValue = currentHoldings.reduce((sum, item) => sum + item.currentPrice * item.shares, 0);
  const floatingPnl = trades
    .filter((trade) => tradeDirection(trade) === 'buy' && trade.status === 'active')
    .reduce((sum, trade) => sum + tradePnl(trade), 0);

  function openTradeDialog(trade?: TradingTrade) {
    setEditing(trade || null);
    setStockSearchField(null);
    setStockSearchResults([]);
    manualAnalysisFieldsRef.current = new Set(
      autoAnalysisFields.filter((field) => field !== 'currentPrice' && Boolean(trade?.[field]))
    );
    autoAnalysisFieldsRef.current = new Set();
    setAnalysis({ source: 'idle', note: '选择股票后自动同步行情与建议' });
    form.resetFields();
    form.setFieldsValue(trade ? {
      ...trade,
      direction: tradeDirection(trade),
      marketGate: trade.marketGate || ''
    } : {
      id: '',
      stockName: '',
      stockCode: '',
      direction: 'buy',
      status: 'active',
      entryDate: '',
      entryPrice: 0,
      currentPrice: 0,
      shares: 0,
      invalidPrice: 0,
      positionLabel: '试错仓',
      targetOne: '',
      targetTwo: '',
      tradeMode: '',
      buyReason: '',
      exitRules: '',
      review: '',
      plannedR: 0,
      actualR: 0,
      timeStopDate: '',
      marketGate: marketGateInfo?.gate || ''
    });
    setDialogOpen(true);
  }

  function resetAutomaticAnalysisFields() {
    const patch: Partial<TradingTrade> = {};
    autoAnalysisFields.forEach((field) => {
      if (manualAnalysisFieldsRef.current.has(field)) return;
      if (field === 'currentPrice') patch.currentPrice = 0;
      if (field === 'invalidPrice') patch.invalidPrice = 0;
      if (field === 'targetOne') patch.targetOne = '';
      if (field === 'targetTwo') patch.targetTwo = '';
      autoAnalysisFieldsRef.current.delete(field);
    });
    if (Object.keys(patch).length) {
      applyingAnalysisRef.current = true;
      form.setFieldsValue(patch);
      applyingAnalysisRef.current = false;
    }
    setAnalysis({ source: 'idle', note: '选择股票后自动同步行情与建议' });
  }

  function updateStockField(field: 'stockName' | 'stockCode', value: string) {
    setStockSearchField(field === 'stockName' ? 'name' : 'code');
    setStockSearchResults([]);
    resetAutomaticAnalysisFields();
    form.setFieldValue(field, value);
  }

  function selectStock(item: StockSearchResult) {
    resetAutomaticAnalysisFields();
    form.setFieldsValue({ stockName: item.name, stockCode: item.code });
    setStockSearchResults([]);
  }

  async function runTradeAnalysis(force = false) {
    const code = normalizeSymbol(String(form.getFieldValue('stockCode') || ''));
    if (!dialogOpen || !code) return;
    const requestID = analysisRequestRef.current + 1;
    analysisRequestRef.current = requestID;
    setAnalysis({ source: 'loading', note: '正在同步行情并计算交易区间' });

    try {
      const [quotes, kline, credentials] = await Promise.all([
        apiFetch<Quote[]>(`/api/quote?code=${encodeURIComponent(code)}`),
        apiFetch<KlineHistory>(`/api/kline-history?code=${encodeURIComponent(code)}&type=day&limit=120`),
        apiFetch<AICredential[]>('/api/ai/credentials').catch(() => [])
      ]);
      if (requestID !== analysisRequestRef.current) return;

      const quote = Array.isArray(quotes) ? quotes[0] : undefined;
      const currentPrice = priceFromMilli(quoteKline(quote)?.Close);
      const rows = Array.isArray(kline?.List) ? kline.List : [];
      const directionValue = form.getFieldValue('direction') === 'sell' ? 'sell' : 'buy';
      const technical = technicalAnalysis(rows, currentPrice, directionValue);
      const enabledCredential = (credentials || []).find((item) => item.enabled !== false && item.has_api_key !== false);

      const applyValues = (values: Partial<TradingTrade>) => {
        const patch: Partial<TradingTrade> = {};
        autoAnalysisFields.forEach((field) => {
          const value = values[field];
          if (value === undefined || manualAnalysisFieldsRef.current.has(field)) return;
          const currentValue = form.getFieldValue(field);
          const empty = field === 'currentPrice' || field === 'invalidPrice'
            ? !Number(currentValue || 0)
            : !String(currentValue || '').trim();
          if (force || empty || autoAnalysisFieldsRef.current.has(field)) {
            patch[field] = value as never;
            autoAnalysisFieldsRef.current.add(field);
          }
        });
        if (Object.keys(patch).length) {
          applyingAnalysisRef.current = true;
          form.setFieldsValue(patch);
          applyingAnalysisRef.current = false;
        }
      };

      if (currentPrice > 0) applyValues({ currentPrice });

      if (enabledCredential && technical) {
        try {
          const aiResponse = await withTimeout(
            apiFetch<{
              result?: Record<string, unknown>;
              content?: string;
            }>('/api/ai/analyze/stock', {
              method: 'POST',
              body: JSON.stringify({
                credential_id: enabledCredential.id,
                provider: enabledCredential.provider,
                model: enabledCredential.model,
                symbol: code,
                input: {
                  task: 'trading_levels',
                  direction: directionValue,
                  entry_price: Number(form.getFieldValue('entryPrice') || 0),
                  shares: Number(form.getFieldValue('shares') || 0),
                  current_price: currentPrice,
                  position_label: form.getFieldValue('positionLabel') || '',
                  lookback_days: 120
                },
                options: { max_tokens: 900 }
              })
            }),
            AI_ANALYSIS_TIMEOUT_MS
          );
          if (requestID !== analysisRequestRef.current) return;
          const aiResult = parseAnalysisResult(aiResponse.result, aiResponse.content || '');
          if (aiResult) {
            applyValues(aiResult);
            setAnalysis({
              source: 'ai',
              note: `AI建议 · ${enabledCredential.name || enabledCredential.model || enabledCredential.provider}`,
              basis: aiResult.basis,
              updatedAt: new Date().toLocaleTimeString('zh-CN', { hour12: false })
            });
            return;
          }
        } catch {
          // Technical levels below keep the form usable when the configured model is unavailable.
        }
      }

      if (technical) {
        applyValues(technical);
        setAnalysis({
          source: 'technical',
          note: '技术估算 · 日K行情',
          basis: technical.basis,
          updatedAt: new Date().toLocaleTimeString('zh-CN', { hour12: false })
        });
        return;
      }

      setAnalysis({ source: currentPrice > 0 ? 'quote' : 'error', note: currentPrice > 0 ? '行情已同步，暂缺日K建议' : '暂时无法生成建议' });
    } catch (error) {
      if (requestID !== analysisRequestRef.current) return;
      setAnalysis({ source: 'error', note: error instanceof Error ? error.message : '行情分析失败' });
    }
  }

  useEffect(() => {
    if (!dialogOpen || !normalizeSymbol(stockCode)) return;
    const timer = window.setTimeout(() => { runTradeAnalysis(); }, 360);
    return () => window.clearTimeout(timer);
  }, [dialogOpen, stockCode, direction]);

  async function saveTrade(values: TradingTrade) {
    if (!state) return;
    const selectedDirection = values.direction === 'sell' ? 'sell' : 'buy';
    const shares = Number(values.shares || 0);
    const selectedStockCode = String(values.stockCode || '').trim();
    if (selectedDirection === 'sell' && shares > getAvailableShares(trades, selectedStockCode, values.id) + 0.000001) {
      message.error(`卖出数量不能超过当前可用仓位（${formatShares(getAvailableShares(trades, selectedStockCode, values.id))} 股）`);
      return;
    }
    const nextTrades = [...trades];
    const id = values.id || `trade-${Date.now()}`;
    const trade: TradingTrade = {
      ...values,
      id,
      stockCode: selectedStockCode,
      direction: selectedDirection,
      shares,
      status: selectedDirection === 'sell' ? 'closed' : (editing?.status || 'active')
    };
    const index = nextTrades.findIndex((item) => item.id === id);
    if (index >= 0) nextTrades[index] = trade;
    else nextTrades.unshift(trade);
    const payload = { ...state, trades: nextTrades };
    await apiFetch('/api/trading-system', { method: 'PUT', body: JSON.stringify(payload) });
    message.success('交易已保存');
    setDialogOpen(false);
    await load();
  }

  return (
    <div className="trading-layout">
      <Card
        className="work-card trading-side"
        title={
          <div className="trading-panel-title">
            <span>账户与纪律</span>
            <Text type="secondary">资金 / 纪律</Text>
          </div>
        }
        extra={<Button aria-label="刷新账户数据" icon={<ReloadOutlined />} onClick={load} loading={loading} />}
      >
        <div className={`trading-macro-risk ${macroOverview?.active_risk_events?.length ? 'is-active' : ''}`}>
          <WarningOutlined />
          <div><strong>{macroOverview?.active_risk_events?.length ? `宏观风险窗口 ${macroOverview.active_risk_events.length} 个` : '宏观风险窗口暂无'}</strong><span>{macroOverview?.holding_risk_events ? `当前持仓相关 ${macroOverview.holding_risk_events} 个，交易前请复核计划。` : '预警只提供复核提示，不会自动阻止交易。'}</span></div>
        </div>
        <div className={`trading-market-gate is-${marketGateInfo?.gate === '开' ? 'open' : marketGateInfo?.gate === '关' ? 'closed' : 'pending'}`}>
          <div>
            <strong>大盘开关</strong>
            <span>{marketGateInfo ? `上证 ${marketGateInfo.close.toFixed(2)} · 20日线 ${marketGateInfo.ma20.toFixed(2)}${marketGateInfo.date ? ` · ${marketGateInfo.date}` : ''}` : '正在获取上证指数 20 日线状态…'}</span>
          </div>
          <em>{marketGateInfo ? (marketGateInfo.gate === '开' ? '开 · 可开新仓' : '关 · 只处理持仓') : '--'}</em>
        </div>
        <section className="trading-account-summary">
          <div className="trading-account-total">
            <span>总资产</span>
            <strong>{formatMoney(stats.totalAssets)}</strong>
            <em className={stats.dailyProfit >= 0 ? 'trading-positive' : 'trading-negative'}>
              {stats.dailyProfit >= 0 ? '+' : ''}{formatMoney(stats.dailyProfit)} 今日
            </em>
          </div>
          <div className="trading-account-grid">
            <div><span>本金</span><strong>{formatMoney(stats.principal)}</strong></div>
            <div><span>总市值</span><strong>{formatMoney(stats.marketValue)}</strong></div>
            <div><span>可用风险额</span><strong>{formatMoney(stats.maxTradeRisk)}</strong></div>
          </div>
        </section>
        <Button className="trading-create-button" type="primary" block icon={<PlusOutlined />} onClick={() => openTradeDialog()}>
          新建交易
        </Button>

        <section className="trading-side-section">
          <div className="trading-section-heading">
            <span>纪律检查</span>
            <small>{Object.values(state?.discipline || {}).filter(Boolean).length}/4 已确认</small>
          </div>
          <div className="trading-discipline-list">
            {[
              ['reason', '买入理由', '进场前写清逻辑'],
              ['invalid', '技术无效点', '失效后执行退出'],
              ['risk', '风险预算', '单笔风险受控'],
              ['noImpulse', '避免冲动交易', '等待计划信号']
            ].map(([key, label, hint]) => {
              const checked = Boolean(state?.discipline?.[key as keyof TradingSystemState['discipline']]);
              return (
                <div className={`trading-discipline-row ${checked ? 'is-checked' : 'is-pending'}`} key={key}>
                  <span className="trading-discipline-icon">{checked ? <CheckCircleOutlined /> : <WarningOutlined />}</span>
                  <div><strong>{label}</strong><small>{hint}</small></div>
                  <em>{signalLabel(checked)}</em>
                </div>
              );
            })}
          </div>
        </section>

        <section className="trading-side-section">
          <div className="trading-section-heading"><span>风控边界</span><small>账户级限制</small></div>
          <div className="trading-limit-grid">
            <div><span>最大单笔风险</span><strong>{formatMoney(stats.maxTradeRisk)}</strong></div>
            <div><span>最大仓位</span><strong>{Number(stats.maxPositionWeight || 0).toFixed(1)}%</strong></div>
          </div>
          <div className="trading-fee-note">
            佣金 {Number(state?.fees?.buyCommissionRate || 0).toFixed(2)}% · 印花税 {Number(state?.fees?.stampTaxRate || 0).toFixed(2)}%
          </div>
        </section>
      </Card>

      <Card
        className="work-card trading-main"
        title={
          <div className="trading-panel-title">
            <span>持仓与交易</span>
            <Text type="secondary">
              {trades.length} 条记录 · {quotesUpdatedAt ? `行情 ${quotesUpdatedAt}` : '行情待刷新'}
              {tradingTime === true ? ' · 盘中自动刷新' : ''}
            </Text>
          </div>
        }
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={handleManualQuoteRefresh} loading={refreshingQuotes}>刷新行情</Button>
            <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openTradeDialog()}>新建交易</Button>
          </Space>
        }
      >
        <div className="trading-main-strip">
          <div><span>当前持仓</span><strong>{currentHoldings.length}</strong><small>持仓标的</small></div>
          <div><span>持仓市值</span><strong>{formatMoney(holdingMarketValue)}</strong><small>净持仓市值</small></div>
          <div><span>浮动盈亏</span><strong className={floatingPnl >= 0 ? 'trading-positive' : 'trading-negative'}>
            {formatMoney(floatingPnl)}
          </strong><small>未实现盈亏</small></div>
          <div><span>执行状态</span><strong className="trading-status-ready">计划中</strong><small>按纪律执行</small></div>
        </div>
        <div className="trading-table-shell">
          <Table
            size="middle"
            loading={loading}
            rowKey="id"
            dataSource={trades}
            pagination={{ pageSize: 8, size: 'small', showSizeChanger: false }}
            locale={{ emptyText: '暂无交易记录，先建立第一笔计划' }}
            columns={[
              {
                title: '标的',
                dataIndex: 'stockName',
                width: 150,
                render: (value: string, record: TradingTrade) => (
                  <div className="trading-symbol-cell"><strong>{value || '--'}</strong><span>{record.stockCode || '--'}</span></div>
                )
              },
              {
                title: '方向',
                width: 82,
                render: (_value, record: TradingTrade) => (
                  <Tag className={`trading-direction-tag ${tradeDirection(record) === 'sell' ? 'is-sell' : 'is-buy'}`}>
                    {tradeDirection(record) === 'sell' ? '卖出' : '买入'}
                  </Tag>
                )
              },
              {
                title: '持仓状态',
                dataIndex: 'status',
                width: 90,
                render: (value: string) => <Tag className={`trading-status-tag ${value === 'active' ? 'is-active' : ''}`}>{value === 'active' ? '持仓中' : '已清仓'}</Tag>
              },
              { title: '成交价', dataIndex: 'entryPrice', width: 88, render: (value: number) => formatMoney(value) },
              { title: '现价', dataIndex: 'currentPrice', width: 88, render: (value: number) => formatMoney(value) },
              {
                title: '数量',
                dataIndex: 'shares',
                width: 82,
                render: (value: number, record: TradingTrade) => (
                  <span className={tradeDirection(record) === 'sell' ? 'trading-sell-quantity' : ''}>{formatShares(value)}</span>
                )
              },
              {
                title: '浮动盈亏',
                width: 122,
                render: (_value, record: TradingTrade) => (
                  tradeDirection(record) === 'sell' ? <span className="trading-realized-placeholder">已卖出</span> : (
                    <div className={tradePnl(record) >= 0 ? 'trading-positive' : 'trading-negative'}>
                      <strong>{tradePnl(record) >= 0 ? '+' : ''}{formatMoney(tradePnl(record))}</strong>
                      <small>{tradePnlRate(record) >= 0 ? '+' : ''}{tradePnlRate(record).toFixed(2)}%</small>
                    </div>
                  )
                )
              },
              { title: '无效点', dataIndex: 'invalidPrice', width: 86, render: (value: number) => formatMoney(value) },
              { title: '仓位', dataIndex: 'positionLabel', width: 82, render: (value: string) => value || '--' },
              {
                title: '操作',
                width: 76,
                render: (_value, record) => <Button className="trading-edit-button" size="small" onClick={() => openTradeDialog(record)}>编辑</Button>
              }
            ]}
            scroll={{ x: 946 }}
          />
        </div>
      </Card>

      <Modal
        open={dialogOpen}
        onCancel={() => setDialogOpen(false)}
        footer={null}
        width={980}
        centered
        destroyOnHidden
        closeIcon={<CloseOutlined />}
        className="app-themed-modal trading-edit-modal"
        title={
          <div className="quote-dialog-title trade-dialog-title">
            <div>
              <strong>{editing ? '编辑交易' : '新建交易'}</strong>
              <span>{editing?.stockName || '交易计划'}</span>
            </div>
            <small>交易计划 · 风险先行</small>
          </div>
        }
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={saveTrade}
          className="trade-form"
          onValuesChange={(changedValues) => {
            if (applyingAnalysisRef.current) return;
            autoAnalysisFields.forEach((field) => {
              if (Object.prototype.hasOwnProperty.call(changedValues, field)) {
                manualAnalysisFieldsRef.current.add(field);
                autoAnalysisFieldsRef.current.delete(field);
              }
            });
          }}
        >
          <Form.Item name="id" hidden><Input /></Form.Item>
          <Form.Item name="status" hidden><Input /></Form.Item>
          <section className="trade-form-section">
            <div className="trade-form-section-head">
              <div><strong>基础信息</strong><span>先确认标的、交易方向与日期</span></div>
              <small>01</small>
            </div>
            <div className="trade-form-grid trade-form-grid-basic">
              <Form.Item name="stockName" label="股票名称" rules={[{ required: true }]}>
                <AutoComplete
                  className="trading-stock-autocomplete"
                  options={stockSearchField === 'name' ? stockSearchOptions : []}
                  value={stockName}
                  onFocus={() => setStockSearchField('name')}
                  onChange={(value) => updateStockField('stockName', value)}
                  onSelect={(_value, option) => selectStock(option.stock as StockSearchResult)}
                  notFoundContent={stockSearchLoading ? '正在匹配...' : '没有找到匹配标的'}
                >
                  <Input placeholder="输入股票名称，例如：紫金矿业" />
                </AutoComplete>
              </Form.Item>
              <Form.Item name="stockCode" label="股票代码" rules={[{ required: true }]}>
                <AutoComplete
                  className="trading-stock-autocomplete"
                  options={stockSearchField === 'code' ? stockSearchOptions : []}
                  value={stockCode}
                  onFocus={() => setStockSearchField('code')}
                  onChange={(value) => updateStockField('stockCode', value)}
                  onSelect={(_value, option) => selectStock(option.stock as StockSearchResult)}
                  notFoundContent={stockSearchLoading ? '正在匹配...' : '没有找到匹配标的'}
                >
                  <Input placeholder="输入股票代码" />
                </AutoComplete>
              </Form.Item>
              <Form.Item name="direction" label="交易方向" rules={[{ required: true }]}><Select options={[{ value: 'buy', label: '买入' }, { value: 'sell', label: '卖出' }]} /></Form.Item>
              <Form.Item name="entryDate" label={direction === 'sell' ? '卖出日期' : '买入日期'}><Input type="date" /></Form.Item>
            </div>
          </section>

          <section className="trade-form-section">
            <div className="trade-form-section-head">
              <div><strong>价格与仓位</strong><span>用数字定义风险边界</span></div>
              <small>02</small>
            </div>
            <div className={`trade-position-context ${direction === 'sell' ? 'is-sell' : 'is-buy'}`}>
              {direction === 'sell'
                ? <>当前可卖 <strong>{formatShares(availableShares)}</strong> 股</>
                : <>买入数量将计入当前持仓</>}
            </div>
            <div className={`trade-analysis-bar is-${analysis.source}`}>
              <div className="trade-analysis-copy">
                <span className="trade-analysis-dot" />
                <div>
                  <strong>{analysis.note}</strong>
                  <small>{analysis.basis || (analysis.source === 'loading' ? '正在读取最新价与日K数据' : '自动值可直接修改，手动填写拥有更高优先级')}</small>
                </div>
              </div>
              <Space size={8}>
                {analysis.updatedAt && <Text type="secondary">{analysis.updatedAt}</Text>}
                <Button size="small" icon={<ReloadOutlined />} onClick={() => runTradeAnalysis(true)} loading={analysis.source === 'loading'}>
                  重新分析
                </Button>
              </Space>
            </div>
            <div className="trade-form-grid trade-form-grid-position">
              <Form.Item name="entryPrice" label={<span className="trade-field-label"><span>{direction === 'sell' ? '卖出价' : '买入价'}</span><em>手填</em></span>}><InputNumber style={{ width: '100%' }} min={0} /></Form.Item>
              <Form.Item name="currentPrice" label={<span className="trade-field-label"><span>当前价</span><em className="is-auto">行情</em></span>}><InputNumber style={{ width: '100%' }} min={0} /></Form.Item>
              <Form.Item
                name="shares"
                label={<span className="trade-field-label"><span>{direction === 'sell' ? '卖出股数' : '买入股数'}</span><em>手填</em></span>}
                rules={[{
                  validator: (_, value) => {
                    const amount = Number(value || 0);
                    if (amount <= 0) return Promise.reject(new Error(direction === 'sell' ? '请输入卖出股数' : '请输入买入股数'));
                    if (direction === 'sell' && amount > availableShares + 0.000001) {
                      return Promise.reject(new Error(`最多可卖 ${formatShares(availableShares)} 股`));
                    }
                    return Promise.resolve();
                  }
                }]}
              >
                <InputNumber style={{ width: '100%' }} min={1} precision={0} />
              </Form.Item>
              <Form.Item name="invalidPrice" label={<span className="trade-field-label"><span>技术无效点</span><em className="is-auto">分析</em></span>}><InputNumber style={{ width: '100%' }} min={0} /></Form.Item>
              <Form.Item name="positionLabel" label={<span className="trade-field-label"><span>仓位标签</span><em>手填</em></span>}><Select options={[{ value: '试错仓' }, { value: '确认仓' }, { value: '趋势仓' }, { value: '观察仓' }]} /></Form.Item>
              <Form.Item name="targetOne" label={<span className="trade-field-label"><span>{direction === 'sell' ? '第一观察 / 支撑位' : '第一观察 / 压力位'}</span><em className="is-auto">分析</em></span>}><Input placeholder="自动生成，可手动调整" /></Form.Item>
              <Form.Item name="targetTwo" label={<span className="trade-field-label"><span>{direction === 'sell' ? '强支撑 / 止盈区' : '强压力 / 止盈区'}</span><em className="is-auto">分析</em></span>}><Input placeholder="自动生成，可手动调整" /></Form.Item>
              <Form.Item name="plannedR" label={<span className="trade-field-label"><span>计划亏损(1R)</span><em>手填</em></span>} extra={plannedR > 0 ? `账户风险 ${(plannedR / Number(stats.principal || 1) * 100).toFixed(2)}%` : '下单前写明的计划亏损金额'}>
                <InputNumber style={{ width: '100%' }} min={0} placeholder="例如：360" />
              </Form.Item>
              <Form.Item name="timeStopDate" label={<span className="trade-field-label"><span>时间止损</span><em>手填</em></span>} extra="买入日+5个交易日未走强离场">
                <Input type="date" />
              </Form.Item>
            </div>
          </section>

          <section className="trade-form-section">
            <div className="trade-form-section-head">
              <div><strong>交易计划</strong><span>把为什么买、何时退出写清楚</span></div>
              <small>03</small>
            </div>
            <div className="trade-form-grid trade-form-grid-plan">
              <Form.Item name="tradeMode" label="交易模式"><Input placeholder="例如：支撑位轻仓试错 / 突破后跟随" /></Form.Item>
              <Form.Item name="marketGate" label="开仓当日大盘开关" extra={marketGateInfo ? `当前：上证 ${marketGateInfo.close.toFixed(2)} vs 20日线 ${marketGateInfo.ma20.toFixed(2)} → ${marketGateInfo.gate}` : '上证收盘是否站上20日线'}>
                <Select options={[{ value: '开', label: '开(站上20日线)' }, { value: '关', label: '关(跌破20日线)' }]} />
              </Form.Item>
              <Form.Item name="buyReason" label={direction === 'sell' ? '卖出理由' : '买入理由'}><Input.TextArea rows={3} placeholder={direction === 'sell' ? '记录减仓、止盈或退出的事实与判断' : '记录触发交易的事实与判断'} /></Form.Item>
              <Form.Item name="exitRules" label={direction === 'sell' ? '成交后处理规则' : '退出 / 加仓规则'}><Input.TextArea rows={3} placeholder={direction === 'sell' ? '记录剩余仓位、止盈或重新买回条件' : '写明失效条件、止损和加仓条件'} /></Form.Item>
              <Form.Item name="review" label="盘后复盘"><Input.TextArea rows={3} placeholder="收盘后补充执行结果与偏差" /></Form.Item>
              <Form.Item name="actualR" label="实际盈亏(元)" extra={plannedR > 0 && actualR !== 0 ? `实际 ${actualR > 0 ? '+' : ''}${(actualR / plannedR).toFixed(2)}R` : '交易结束后填写，用于月度R统计'}>
                <InputNumber style={{ width: '100%' }} placeholder="实际盈亏金额，正盈负亏" />
              </Form.Item>
            </div>
          </section>

          <div className="trade-form-actions">
            <Text type="secondary">保存后会更新交易卡与账户汇总</Text>
            <Space>
              <Button onClick={() => setDialogOpen(false)}>取消</Button>
              <Button type="primary" htmlType="submit" icon={<SaveOutlined />}>保存交易</Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
