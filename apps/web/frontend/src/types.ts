export interface ServiceStatus {
  id: string;
  name: string;
  status: string;
  healthy: boolean;
  latency_ms?: number;
}

export interface ServiceStatusResult {
  services: ServiceStatus[];
  ready: boolean;
  checked_at: string;
}

export interface MacroEvent {
  id: string;
  code: string;
  name: string;
  category: string;
  country: string;
  impact: 'low' | 'medium' | 'high' | 'critical' | string;
  starts_at: string;
  scheduled_at?: string;
  released_at?: string;
  previous_value?: string;
  forecast_value?: string;
  actual_value?: string;
  revision?: string;
  a_share_date?: string;
  source?: string;
  source_url?: string;
  description?: string;
  status?: string;
  acknowledged?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface MacroEventSyncState {
  id: string;
  provider: string;
  status: 'idle' | 'success' | 'partial' | 'failed' | string;
  last_attempt_at?: string;
  last_success_at?: string;
  event_count: number;
  message?: string;
  updated_at?: string;
}

export interface MacroAlertSettings {
  id: string;
  enabled: boolean;
  lead_minutes: number;
  window_before_minutes: number;
  window_after_minutes: number;
  critical_only: boolean;
  notify_webhooks: boolean;
  webhook_ids: string;
  updated_at?: string;
}

export interface MacroEventOverview {
  holding_symbols: number;
  watchlist_symbols: number;
  holding_risk_events: number;
  watchlist_risk_events: number;
  active_risk_events: MacroEvent[];
  settings: MacroAlertSettings;
}

export interface QuoteK {
  Last?: number;
  Close?: number;
  Open?: number;
  High?: number;
  Low?: number;
  Volume?: number;
  Amount?: number;
}

export interface PriceLevel {
  Price?: number;
  Number?: number;
}

export interface Quote {
  Code?: string;
  Name?: string;
  Kline?: QuoteK;
  // Kept for compatibility with older quote responses.
  K?: QuoteK;
  TotalHand?: number;
  Amount?: number;
  Rate?: number;
  BuyLevel?: PriceLevel[];
  SellLevel?: PriceLevel[];
}

export interface LongTigerRank {
  ACCUM_AMOUNT?: number;
  BILLBOARD_BUY_AMT?: number;
  BILLBOARD_DEAL_AMT?: number;
  BILLBOARD_NET_AMT?: number;
  BILLBOARD_SELL_AMT?: number;
  CHANGE_RATE?: number;
  CLOSE_PRICE?: number;
  DEAL_AMOUNT_RATIO?: number;
  DEAL_NET_RATIO?: number;
  D1_CLOSE_ADJCHRATE?: number;
  D2_CLOSE_ADJCHRATE?: number;
  D5_CLOSE_ADJCHRATE?: number;
  D10_CLOSE_ADJCHRATE?: number;
  EXPLAIN?: string;
  EXPLANATION?: string;
  FREE_MARKET_CAP?: number;
  SECUCODE?: string;
  SECURITY_CODE?: string;
  SECURITY_NAME_ABBR?: string;
  SECURITY_TYPE_CODE?: string;
  TRADE_DATE?: string;
  TURNOVERRATE?: number;
}

export interface LongTigerResponse {
  requested_date: string;
  trade_date: string;
  items: LongTigerRank[];
  source: string;
  cached_at?: string;
}

export interface MarketResearchReport {
  title?: string;
  stockName?: string;
  stockCode?: string;
  orgSName?: string;
  publishDate?: string;
  infoCode?: string;
  indvInduName?: string;
  emRatingName?: string;
  ratingChange?: number;
  sRatingName?: string;
  researcher?: string;
  market?: string;
}

export interface IndustryResearchReport {
  title?: string;
  stockName?: string;
  stockCode?: string;
  orgCode?: string;
  orgName?: string;
  orgSName?: string;
  publishDate?: string;
  infoCode?: string;
  industryCode?: string;
  industryName?: string;
  emIndustryCode?: string;
  emRatingName?: string;
  ratingChange?: number;
  sRatingName?: string;
  researcher?: string;
  author?: string[];
  reportType?: number;
  encodeUrl?: string;
}

export interface IndustryDictItem {
  bkCode?: string;
  fubkCode?: string;
  bkName?: string;
  publishCode?: string;
  firstLetter?: string;
}

export interface IndustryResearchResponse {
  items: IndustryResearchReport[];
  industry_code: string;
  days: number;
  limit: number;
  source: string;
  fetched_at: string;
}

export interface IndustryDictResponse {
  items: IndustryDictItem[];
  source: string;
  fetched_at: string;
}

export interface MarketNotice {
  art_code?: string;
  stock_code?: string;
  stock_name?: string;
  title?: string;
  column_name?: string;
  notice_date?: string;
  display_time?: string;
}

export interface HotMoneyTrade {
  TRADE_DATE?: string;
  EXPLANATION?: string;
  OPERATEDEPT_NAME?: string;
  BUY_AMT_REAL?: number | null;
  BUY_RATIO?: number | null;
  SELL_AMT_REAL?: number | null;
  SELL_RATIO?: number | null;
  SECURITY_CODE?: string;
  SECURITY_NAME_ABBR?: string;
  SECUCODE?: string;
}

export interface IndustryRankItem {
  bd_name?: string;
  bd_code?: string;
  bd_zxj?: string;
  bd_zd?: string;
  bd_zdf?: string;
  bd_zdf5?: string;
  bd_zdf20?: string;
  nzg_code?: string;
  nzg_name?: string;
  nzg_zxj?: string;
  nzg_zd?: string;
  nzg_zdf?: string;
}

export interface IndustryRankResponse {
  items: IndustryRankItem[];
  sort: string;
  limit: number;
  source: string;
  fetched_at: string;
}

export interface IndustryMoneyRankItem {
  cate_type?: string;
  category?: string;
  name?: string;
  avg_price?: string;
  avg_changeratio?: string;
  turnover?: string;
  inamount?: string;
  outamount?: string;
  netamount?: string;
  ratioamount?: string;
  ts_symbol?: string;
  ts_name?: string;
  ts_trade?: string;
  ts_changeratio?: string;
  ts_ratioamount?: string;
}

export interface IndustryMoneyRankResponse {
  items: IndustryMoneyRankItem[];
  category: string;
  sort: string;
  source: string;
  fetched_at: string;
}

export interface StockMoneyRankItem {
  symbol?: string;
  name?: string | boolean;
  trade?: string;
  changeratio?: string;
  turnover?: string;
  amount?: string;
  inamount?: string;
  outamount?: string;
  netamount?: string;
  ratioamount?: string;
  r0_in?: string;
  r0_out?: string;
  r0_net?: string;
  r3_in?: string;
  r3_out?: string;
  r3_net?: string;
  r0_ratio?: string;
  r3_ratio?: string;
  r0x_ratio?: string;
}

export interface StockMoneyRankResponse {
  items: StockMoneyRankItem[];
  sort: string;
  limit: number;
  source: string;
  fetched_at: string;
}

export interface RzrqRankItem {
  stockCode?: string;
  stockName?: string;
  date?: number;
  lrye?: string;
  lryeRate?: string;
  rzye?: string;
  rzyeRate?: string;
  rqye?: string;
  rqyeRate?: string;
  jmr?: string;
  jmrRate?: string;
  rzmre?: string;
  rzche?: string;
  rzjmce?: string;
  yezf?: string;
  close_price?: string;
  close_profit?: string;
  marketId?: string;
}

export interface RzrqRankResponse {
  type: string;
  list: RzrqRankItem[];
  requested_date?: string;
  data_date?: string;
  source: string;
  fetched_at: string;
}

export interface RzrqTrendItem {
  date?: string;
  rzye?: string;
  rzjlr?: string;
  spj?: string;
  spzf?: string;
}

export interface RzrqTrendResponse {
  type: string;
  code: string;
  items: RzrqTrendItem[];
  rzye_unit?: string;
  rzjlr_unit?: string;
  spj_unit?: string;
  spzf_unit?: string;
  update_time?: string;
  source: string;
  fetched_at: string;
}

export interface MarketTradingStatus {
  date: string;
  is_trading: boolean;
  checked_at: string;
  timezone: string;
}

export interface WatchlistRow {
  key: string;
  code: string;
  name: string;
  price?: string;
  change?: string;
  changePercent?: string;
  volume?: string;
  amount?: string;
  speed?: string;
  mainNetVolume?: string;
  mainNetInflow?: string;
  industry?: string;
  turnover?: string;
  category?: string;
  entrust?: string;
  volumeRatio?: string;
}

export interface FormulaArg {
  Name: string;
  Value: string | number | boolean;
}

export interface Formula {
  id: string;
  name: string;
  type: 'selection' | 'indicator' | string;
  script: string;
  args_json?: string;
  period: string;
  right: number;
  enabled: boolean;
  description?: string;
  created_at?: string;
  updated_at?: string;
}

export interface FormulaRunResponse {
  engine?: string;
  tick_ms?: number;
  data?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface StockPool {
  id: string;
  name: string;
  symbols: string[];
  description?: string;
  category?: string;
  system?: boolean;
  readonly?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface Strategy {
  id: string;
  name: string;
  description?: string;
  config_json: string;
  enabled: boolean;
  readonly?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface AutomationTask {
  id: string;
  name: string;
  type: string;
  cron: string;
  enabled: boolean;
  only_trade_day: boolean;
  payload_json: string;
  webhook_ids: string;
  last_run_at?: string;
  next_run_at?: string;
  last_status?: string;
  last_message?: string;
  readonly?: boolean;
  system?: boolean;
}

export interface AutomationRun {
  id: string;
  task_id: string;
  task_name: string;
  task_type: string;
  status: string;
  started_at: string;
  finished_at?: string;
  log?: string;
  result_json?: string;
  matched_count?: number;
}

export interface SelectionResult {
  id: string;
  run_id: string;
  task_id: string;
  task_name: string;
  formula_id: string;
  formula_name: string;
  symbol: string;
  latest: number;
  detail_json: string;
  tracking?: SelectionTracking;
  created_at: string;
}

export interface SelectionHorizon {
  horizon_days: number;
  status: 'pending' | 'complete' | 'unavailable' | string;
  as_of_date?: number;
  final_date?: number;
  final_open?: number;
  final_close?: number;
  open_return?: number;
  close_return?: number;
  max_gain?: number;
  max_gain_date?: number;
  max_gain_price?: number;
  days_to_max_gain?: number;
  max_drawdown?: number;
  max_drawdown_date?: number;
  max_drawdown_price?: number;
  days_to_max_drawdown?: number;
  target_return?: number;
  target_hit?: boolean;
  target_hit_date?: number;
  days_to_target_hit?: number;
  drawdown_limit?: number;
  stop_loss_hit?: boolean;
  stop_loss_hit_date?: number;
  days_to_stop_loss?: number;
  success?: boolean;
  reason?: string;
}

export interface SelectionTracking {
  version?: string;
  signal_date?: number;
  base_price?: number;
  base_date?: number;
  base_price_from?: string;
  target_return?: number;
  drawdown_limit?: number;
  updated_at?: string;
  horizons?: Record<string, SelectionHorizon>;
}

export interface SelectionTrackingItem {
  result: SelectionResult;
  tracking: SelectionTracking;
  error?: string;
}

export interface SelectionHorizonSummary {
  completed?: number;
  pending?: number;
  unavailable?: number;
  success_count?: number;
  success_rate?: number;
  average_open_return?: number;
  average_close_return?: number;
  average_max_gain?: number;
  average_max_drawdown?: number;
}

export interface SelectionTrackingResponse {
  items: SelectionTrackingItem[];
  summary?: { total?: number; horizons?: Record<string, SelectionHorizonSummary> };
  policy?: { target_return?: number; drawdown_limit?: number };
  as_of?: string;
}

export interface DecisionNote {
  symbol: string;
  status?: string;
  added_price?: number;
  add_reason?: string;
  plan_buy?: number;
  stop_loss?: number;
  review_note?: string;
  exclude_category?: string;
  exclude_reason?: string;
  created_at?: string;
  updated_at?: string;
}

export interface DailyReviewBatchSummary {
  id: string;
  date: string;
  status: string;
  started_at: string;
  finished_at?: string;
  error?: string;
  matched_count: number;
  strategy_count: number;
  failed_strategies: number;
  candidate_symbols: number;
  kline_loaded: number;
  kline_failed: number;
  duration_ms: number;
}

export interface DailyReviewStrategySummary {
  run_id?: string;
  strategy_id: string;
  strategy_name: string;
  status: string;
  matched: number;
  errors: number;
  error?: string;
  started_at?: string;
  finished_at?: string;
  duration_ms: number;
}

export interface DailyReviewSignal {
  id: string;
  run_id: string;
  strategy_id: string;
  strategy_name: string;
  signal_date: string;
  symbol: string;
  latest: number;
  score: number;
  detail_json: string;
  tracking?: SelectionTracking;
  created_at: string;
}

export interface DailyReviewStock {
  signal_date: string;
  symbol: string;
  latest: number;
  strategy_count: number;
  strategy_ids: string[];
  strategies: string[];
  max_score: number;
  average_score: number;
  tracking?: SelectionTracking;
  status?: string;
  watch?: boolean;
  excluded?: boolean;
  note: DecisionNote;
  signals: DailyReviewSignal[];
}

export interface DailyReviewResponse {
  date?: string;
  selected_batch?: DailyReviewBatchSummary;
  batches: DailyReviewBatchSummary[];
  summary: {
    strategy_count: number;
    raw_signals: number;
    stock_count: number;
    consensus_count: number;
    watch_count: number;
    exclude_count: number;
    candidate_symbols: number;
    kline_loaded: number;
    kline_failed: number;
    duration_ms: number;
  };
  strategies: DailyReviewStrategySummary[];
  stocks: DailyReviewStock[];
  signals: DailyReviewSignal[];
  consensus: DailyReviewStock[];
  watch: string[];
  exclude: string[];
  notes: DecisionNote[];
}

export interface DailyStrategyAnalysisMetric {
  strategy_id: string;
  strategy_name: string;
  run_days: number;
  signal_count: number;
  average_daily_signals: number;
  unique_symbols: number;
  average_score: number;
  consensus_signals: number;
  d3_completed: number;
  d3_success_count: number;
  d3_success_rate: number;
  d3_positive_count: number;
  d3_positive_rate: number;
  d3_average_return: number;
  d3_max_return: number;
  d5_completed: number;
  d5_success_count: number;
  d5_success_rate: number;
  d5_positive_count: number;
  d5_positive_rate: number;
  d5_average_return: number;
  d5_max_return: number;
  d10_completed: number;
  d10_success_count: number;
  d10_success_rate: number;
  d10_positive_count: number;
  d10_positive_rate: number;
  d10_average_return: number;
  d10_max_return: number;
}

export interface DailyStrategyAnalysisResponse {
  start_date: string;
  end_date: string;
  trading_days: number;
  strategy_count: number;
  signal_count: number;
  unique_symbols: number;
  consensus_count: number;
  average_score: number;
  strategies: DailyStrategyAnalysisMetric[];
  stocks: DailyReviewStock[];
  signals: DailyReviewSignal[];
  consensus: DailyReviewStock[];
}

export interface HistoricalBacktestRun {
  id: string;
  status: 'running' | 'success' | 'failed' | 'cancelled' | string;
  start_date: string;
  end_date: string;
  strategy_ids: string[];
  strategy_snapshot_json?: string;
  horizons: number[];
  target_return: number;
  drawdown_limit: number;
  buy_base?: string;
  buy_offset?: number;
  sell_base?: string;
  take_profit?: number;
  trail_drawdown?: number;
  stop_loss?: number;
  max_hold_days?: number;
  total_dates: number;
  processed_dates: number;
  current_date?: string;
  candidate_symbols: number;
  signal_count: number;
  result_json: string;
  error?: string;
  cancel_requested?: boolean;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at?: string;
}

export interface HistoricalScanRun {
  id: string;
  status: 'running' | 'success' | 'failed' | string;
  start_date: string;
  end_date: string;
  strategy_ids: string[];
  total_strategies: number;
  processed_strategies: number;
  current_strategy?: string;
  signal_count: number;
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at?: string;
}

export interface HistoricalScanResult {
  id: string;
  run_id: string;
  strategy_id: string;
  strategy_name: string;
  buy_base: string;
  sell_base: string;
  take_profit: number;
  trail_drawdown: number;
  stop_loss: number;
  max_hold_days: number;
  total_return: number;
  max_drawdown: number;
  win_rate: number;
  closed_count: number;
  open_count: number;
  final_equity: number;
  created_at: string;
}

export interface HistoricalHorizonSummary {
  completed: number;
  pending: number;
  success_count: number;
  success_rate: number;
  average_return: number;
  average_max_gain: number;
  average_max_drawdown: number;
}

export interface HistoricalStrategySummary {
  strategy_id: string;
  strategy_name: string;
  signal_count: number;
  horizons: Record<string, HistoricalHorizonSummary>;
}

export interface HistoricalConsensusSummary {
  signal_date: string;
  symbol: string;
  strategy_count: number;
  strategies: string[];
  tracking?: SelectionTracking;
}

export interface HistoricalBacktestResult {
  strategy_summaries?: HistoricalStrategySummary[];
  consensus?: HistoricalConsensusSummary[];
  signal_count?: number;
  strategy_count?: number;
  trading_days?: number;
  candidate_symbols?: number;
  kline_loaded?: number;
  kline_failed?: number;
  history_count?: number;
  policy?: { horizons?: number[]; target_return?: number; drawdown_limit?: number; entry?: string };
  warnings?: string[];
  portfolio?: HistoricalPortfolioSummary;
}

export interface HistoricalBacktestSignal {
  id: string;
  run_id: string;
  strategy_id: string;
  strategy_name: string;
  signal_date: string;
  symbol: string;
  latest: number;
  score: number;
  detail_json: string;
  tracking?: SelectionTracking;
  created_at: string;
}

export interface HistoricalBacktestSignalPage {
  items: HistoricalBacktestSignal[];
  total: number;
  limit: number;
  offset: number;
}

export interface HistoricalBacktestTrade {
  id: string;
  run_id: string;
  symbol: string;
  status: string;
  entry_date: string;
  entry_price: number;
  shares: number;
  exit_date?: string;
  exit_price?: number;
  pnl?: number;
  pnl_rate?: number;
  hold_days?: number;
  strategy_name: string;
  signal_date?: string;
  reason?: string;
  created_at: string;
}

export interface HistoricalBacktestTradePage {
  items: HistoricalBacktestTrade[];
  total: number;
  limit: number;
  offset: number;
}

export interface HistoricalPortfolioSummary {
  initial_cash?: number;
  max_positions?: number;
  final_equity?: number;
  total_return?: number;
  max_drawdown?: number;
  closed_count?: number;
  open_count?: number;
  skipped_count?: number;
  win_count?: number;
  win_rate?: number;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  method: string;
  headers_json: string;
  events: string;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface TradingTrade {
  id: string;
  stockName: string;
  stockCode: string;
  direction?: 'buy' | 'sell';
  status: string;
  entryDate: string;
  entryPrice: number;
  exitDate?: string;
  exitPrice?: number;
  exitShares?: number;
  currentPrice: number;
  previousClosePrice?: number;
  shares: number;
  invalidPrice: number;
  positionLabel: string;
  targetOne: string;
  targetTwo: string;
  tradeMode: string;
  buyReason: string;
  exitRules: string;
  review: string;
  plannedR?: number;
  actualR?: number;
  timeStopDate?: string;
  marketGate?: string;
}

export interface TradingSystemState {
  account: {
    principal: number;
    totalAssets: number;
    marketValue: number;
    dailyProfit: number;
    maxTradeRisk: number;
    maxPositionWeight: number;
  };
  discipline: {
    reason: boolean;
    invalid: boolean;
    risk: boolean;
    noImpulse: boolean;
  };
  fees: {
    buyCommissionRate: number;
    sellCommissionRate: number;
    stampTaxRate: number;
    transferFeeRate: number;
    minCommission: number;
  };
  filter: string;
  trades: TradingTrade[];
}

export interface AIProvider {
  id: string;
  name?: string;
  default_base_url?: string;
  default_models?: Array<{ id: string; name?: string }>;
}

export interface AICredential {
  id: string;
  name?: string;
  provider: string;
  base_url?: string;
  model?: string;
  enabled?: boolean;
  has_api_key?: boolean;
  api_key_masked?: string;
  source?: string;
  extra_json?: string;
}

export interface AIResearchReport {
  summary?: string;
  confidence?: string;
  facts?: Array<{ label?: string; value?: string; source?: string }>;
  technical?: { trend?: string; signals?: Array<{ name?: string; value?: string; evidence?: string[] }> };
  fundamental?: { summary?: string; evidence?: string[] };
  macro_risk?: { level?: string; events?: Array<Record<string, unknown>> };
  strategy_fit?: { status?: string; reason?: string };
  evidence?: Array<{ claim?: string; evidence?: string[]; source?: string }>;
  data_quality?: { status?: string; notes?: string[] };
  next_checks?: string[];
  discipline_notes?: string[];
  disclaimer?: string;
  [key: string]: unknown;
}

export interface AIResearchResponse {
  run_id: string;
  task_type: string;
  provider: string;
  model: string;
  content: string;
  result: AIResearchReport;
  usage?: Record<string, unknown>;
  latency_ms?: number;
  input?: Record<string, unknown>;
  generated_at?: string;
  prompt_version?: string;
  data_revision?: string;
  tools_used?: string[];
}

export interface AISelectionRankItem {
  rank?: number;
  symbol: string;
  name?: string;
  status?: 'candidate' | 'watch' | 'exclude' | string;
  score?: number;
  historical_validation?: {
    sample_count?: number;
    win_rate?: number;
    average_return?: number;
    total_return?: number;
    max_drawdown?: number;
  };
  reasons?: string[];
  risks?: string[];
  next_checks?: string[];
}

export interface AISelectionResult {
  summary?: string;
  methodology?: string;
  ranking?: AISelectionRankItem[];
  data_quality?: { status?: string; notes?: string[] };
  discipline_notes?: string[];
  disclaimer?: string;
}

export interface AISelectionResponse extends Omit<AIResearchResponse, 'result'> {
  result: AISelectionResult;
}

export interface TDXHQChartAPI {
  isAvailable: () => boolean;
  renderKLine: (container: HTMLElement, options: Record<string, unknown>) => boolean;
  renderMinute?: (container: HTMLElement, options: Record<string, unknown>) => boolean;
  updateMinute?: (container: HTMLElement, options: Record<string, unknown>) => boolean;
  getMinuteGeometry?: (container: HTMLElement, prices?: number[]) => {
    width: number;
    height: number;
    slots: number[];
    prices: number[];
  } | null;
  resize?: (container?: HTMLElement | null) => void;
  destroy?: (container?: HTMLElement | null) => void;
  getChart?: (container: HTMLElement) => unknown;
}

declare global {
  interface Window {
    TDXHQChart?: TDXHQChartAPI;
  }
}
