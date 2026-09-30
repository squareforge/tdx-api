// 全局变量
let currentStock = '';
let watchlistRows = [];
let watchlistDragBound = false;
let watchlistClickBound = false;
let watchlistQuoteRequestID = 0;
let watchlistIndustryPromise = null;
let watchlistQuoteDialogState = {
    open: false,
    symbol: '',
    title: '',
    period: 'minute',
    quote: null,
    quoteData: null
};
let watchlistQuoteIndicatorState = {
    activeWindow: 1,
    windows: [
        { defaultIndex: 'MA', defaultName: 'MA', formulaID: '' },
        { defaultIndex: 'VOL', defaultName: 'VOL', formulaID: '' },
        { defaultIndex: 'MACD', defaultName: 'MACD', formulaID: '' }
    ]
};
let formulas = [];
let pools = [];
let strategies = [];
let factors = [];
let automations = [];
let webhooks = [];
let aiProviders = [];
let aiCredentials = [];
let watchlistAnalysisBusy = false;
let selectionResults = [];
let decisionResults = [];
let dailyReview = null;
let reviewItems = [];
let tradingState = null;
let tradingStateLoaded = false;
let hikyuuDataLoaded = false;
let hikyuuSyncPollTimer = null;
let hikyuuSyncActiveTaskID = '';
let currentHQOverlay = null;
let selectedDecisionResult = null;
let selectedReviewItem = null;
let decisionShowingToday = false;
let selectedStrategyID = '';
let strategyRunState = {
    running: false,
    strategyID: '',
    stepIndex: 0,
    timer: null,
    message: '',
    progress: 0
};
let strategyBacktestState = {
    running: false,
    strategyID: '',
    timer: null,
    message: '',
    progress: 0,
    status: ''
};
let automationRunState = {
    running: false,
    taskID: '',
    stepIndex: 0,
    timer: null,
    message: '',
    progress: 0
};

const defaultTradingState = {
    account: {
        principal: 50000,
        totalAssets: 56279.20,
        marketValue: 0,
        dailyProfit: 0,
        maxTradeRisk: 1,
        maxPositionWeight: 30
    },
    discipline: {
        reason: false,
        invalid: false,
        risk: false,
        noImpulse: false
    },
    fees: {
        buyCommissionRate: 0.03,
        sellCommissionRate: 0.03,
        stampTaxRate: 0.05,
        transferFeeRate: 0.001,
        minCommission: 5
    },
    filter: 'all',
    trades: []
};
const tradingStoreKey = 'tdx-personal-trading-system-v1';
const tradingRefreshIntervalMS = 3 * 60 * 1000;
let tradingRefreshTimer = null;
let tradingNextRefreshAt = 0;
let tradingRefreshing = false;
let tradingLastRefreshAt = '';
let tradingRefreshMessage = '交易时段每 3 分钟刷新';
let tradingAssistRequestID = 0;
const tradingSymbolSearchState = {
    name: { timer: null, requestID: 0, results: [], activeIndex: -1 },
    code: { timer: null, requestID: 0, results: [], activeIndex: -1 }
};

// 工具函数 - 显示加载
function showLoading() {
    document.getElementById('loading').style.display = 'flex';
}

// 工具函数 - 隐藏加载
function hideLoading() {
    document.getElementById('loading').style.display = 'none';
}

async function apiFetch(url, options = {}) {
    const response = await fetch(url, {
        headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
        ...options
    });
    const result = await response.json();
    if (result.code !== 0) {
        throw new Error(result.message || '请求失败');
    }
    return result.data;
}

async function refreshSystemStatus() {
    const serviceNode = document.getElementById('serviceStatusText');
    const timeNode = document.getElementById('systemTimeText');
    const listNode = document.getElementById('serviceStatusList');
    if (!serviceNode || !timeNode || !listNode) return;
    try {
        const result = await apiFetch('/api/services/status');
        const services = Array.isArray(result.services) ? result.services : [];
        serviceNode.textContent = result.ready ? '全部正常' : `${services.filter(service => service.healthy).length}/${services.length} 正常`;
        serviceNode.className = result.ready ? 'is-healthy' : 'is-degraded';
        listNode.innerHTML = services.map(service => {
            const healthy = service.healthy === true;
            const statusText = healthy ? '正常' : (service.status === 'degraded' ? '降级' : '异常');
            return `
                <span class="service-status-chip ${healthy ? 'is-healthy' : 'is-offline'}" title="${escapeHTML(service.id || '')}">
                    <i aria-hidden="true"></i>
                    <b>${escapeHTML(service.name || service.id || '服务')}</b>
                    <em>${statusText}</em>
                </span>
            `;
        }).join('');
        timeNode.textContent = formatSystemTime(result.checked_at);
    } catch (error) {
        serviceNode.textContent = '状态获取失败';
        serviceNode.className = 'is-degraded';
        listNode.innerHTML = '<span class="service-status-loading is-offline">无法获取服务状态</span>';
        timeNode.textContent = new Date().toLocaleTimeString('zh-CN', { hour12: false });
    }
}

function formatSystemTime(value) {
    const date = value ? new Date(value) : new Date();
    return Number.isNaN(date.getTime())
        ? new Date().toLocaleTimeString('zh-CN', { hour12: false })
        : date.toLocaleTimeString('zh-CN', { hour12: false });
}

function loadQuickStock(code) {
    const input = document.getElementById('stockCode');
    if (input) input.value = code;
    currentStock = code;
    loadStockData(code);
}

function renderWatchlist(options = {}) {
    const statusNode = document.getElementById('watchlistStatus');
    const body = document.getElementById('watchlistTableBody');
    if (!body) return;

    const rows = Array.isArray(watchlistRows) ? watchlistRows : [];
    const shouldRefreshQuotes = options.refreshQuotes !== false;
    if (statusNode && !rows.length) statusNode.textContent = '暂无自选股票';
    if (statusNode && rows.length && shouldRefreshQuotes) statusNode.textContent = `共 ${rows.length} 只 · 正在刷新行情`;
    if (statusNode && rows.length && !shouldRefreshQuotes) statusNode.textContent = `共 ${rows.length} 只`;

    if (!rows.length) {
        body.innerHTML = '<tr class="watchlist-empty-row"><td colspan="16">暂无自选股票</td></tr>';
    } else {
        body.innerHTML = rows.map((row, index) => `
            <tr class="watchlist-row" data-watchlist-symbol="${escapeHTML(normalizeSymbol(row.code || row.symbol || ''))}" data-watchlist-name="${escapeHTML(row.name || row.code || '')}" tabindex="0" role="button" aria-label="查看 ${escapeHTML(row.name || row.code || '股票')} 专业行情">
                <td class="watchlist-fixed watchlist-index">${index + 1}</td>
                <td class="watchlist-fixed watchlist-code">
                    <button
                        type="button"
                        class="watchlist-code-link"
                        data-watchlist-symbol="${escapeHTML(normalizeSymbol(row.code || row.symbol || ''))}"
                        data-watchlist-name="${escapeHTML(row.name || row.code || '')}"
                        aria-label="打开 ${escapeHTML(row.code || row.symbol || '--')} 行情详情"
                    >${escapeHTML(row.code || row.symbol || '--')}</button>
                </td>
                <td class="watchlist-fixed watchlist-name">${escapeHTML(row.name || '--')}</td>
                <td class="${watchlistChangeClass(row.changePercent)}">${escapeHTML(row.changePercent ?? '--')}</td>
                <td class="${watchlistChangeClass(row.changePercent)}">${escapeHTML(row.price ?? '--')}</td>
                <td>${escapeHTML(row.volume ?? '--')}</td>
                <td>${escapeHTML(row.amount ?? '--')}</td>
                <td>${escapeHTML(row.speed ?? '--')}</td>
                <td class="${watchlistChangeClass(row.changePercent)}">${escapeHTML(row.change ?? '--')}</td>
                <td>${escapeHTML(row.mainNetVolume ?? '--')}</td>
                <td>${escapeHTML(row.mainNetInflow ?? '--')}</td>
                <td>${escapeHTML(row.industry ?? '--')}</td>
                <td>${escapeHTML(row.turnover ?? '--')}</td>
                <td>${escapeHTML(row.category ?? '--')}</td>
                <td>${escapeHTML(row.entrust ?? '--')}</td>
                <td>${escapeHTML(row.volumeRatio ?? '--')}</td>
            </tr>
        `).join('');
    }

    bindWatchlistClick();
    bindWatchlistDrag();
    if (shouldRefreshQuotes && rows.length) refreshWatchlistQuotes();
}

function watchlistAnalysisCredentials() {
    return (Array.isArray(aiCredentials) ? aiCredentials : [])
        .filter(item => item && item.enabled !== false && item.id)
        .filter((item, index, items) => items.findIndex(value => value.id === item.id) === index);
}

function renderWatchlistAnalysisCredentials() {
    const select = document.getElementById('watchlistAnalysisCredential');
    const submit = document.getElementById('watchlistAnalysisSubmit');
    const credentials = watchlistAnalysisCredentials();
    if (!select) return credentials;
    if (!credentials.length) {
        select.innerHTML = '<option value="">暂无已启用模型，请先在 AI 模型中配置</option>';
        select.disabled = true;
        if (submit) submit.disabled = true;
        updateWatchlistAnalysisModelHint();
        return credentials;
    }
    const current = select.value;
    select.disabled = false;
    select.innerHTML = credentials.map(item => {
        const provider = aiProviderName(item.provider);
        const model = item.model || defaultAIModel(item.provider) || '默认模型';
        return `<option value="${escapeHTML(item.id)}">${escapeHTML(item.name || provider)} · ${escapeHTML(model)}</option>`;
    }).join('');
    if (credentials.some(item => item.id === current)) select.value = current;
    if (!select.value) select.value = credentials[0].id;
    if (submit) submit.disabled = false;
    updateWatchlistAnalysisModelHint();
    return credentials;
}

function updateWatchlistAnalysisModelHint() {
    const node = document.getElementById('watchlistAnalysisModelHint');
    const select = document.getElementById('watchlistAnalysisCredential');
    if (!node) return;
    const credential = watchlistAnalysisCredentials().find(item => item.id === select?.value);
    if (!credential) {
        node.textContent = '请先到“AI 模型”配置并启用一个可用模型。';
        node.className = 'status-note error';
        return;
    }
    node.textContent = `${aiProviderName(credential.provider)} · ${credential.model || defaultAIModel(credential.provider) || '默认模型'}。分析会查询实时行情、近期日K、财务和资讯数据。`;
    node.className = 'status-note';
}

async function openWatchlistAnalysisDialog() {
    const dialog = document.getElementById('watchlistAnalysisDialog');
    if (!dialog) return;
    const rows = Array.isArray(watchlistRows) ? watchlistRows : [];
    const scope = document.getElementById('watchlistAnalysisScope');
    const result = document.getElementById('watchlistAnalysisResult');
    const status = document.getElementById('watchlistAnalysisStatus');
    if (scope) scope.textContent = rows.length ? `当前自选 ${rows.length} 只，最多分析 20 只` : '当前没有自选股票';
    if (result) {
        result.hidden = true;
        result.innerHTML = '';
    }
    if (status) {
        status.textContent = '';
        status.className = 'status-note';
    }
    dialog.classList.add('open');
    dialog.setAttribute('aria-hidden', 'false');
    if (!aiCredentials.length) {
        try {
            await loadAIConfigData();
        } catch (error) {
            if (status) {
                status.textContent = error.message || String(error);
                status.className = 'status-note error';
            }
        }
    }
    renderWatchlistAnalysisCredentials();
}

function closeWatchlistAnalysisDialog() {
    const dialog = document.getElementById('watchlistAnalysisDialog');
    if (!dialog || watchlistAnalysisBusy) return;
    dialog.classList.remove('open');
    dialog.setAttribute('aria-hidden', 'true');
}

function setWatchlistAnalysisStatus(message, type = '') {
    const node = document.getElementById('watchlistAnalysisStatus');
    if (!node) return;
    node.textContent = message || '';
    node.className = `status-note ${type}`.trim();
}

function watchlistAnalysisList(value) {
    if (Array.isArray(value)) return value.filter(item => item !== null && item !== undefined && String(item).trim());
    if (value === null || value === undefined || value === '') return [];
    return [value];
}

function renderWatchlistAnalysisList(title, values, className = '') {
    const items = watchlistAnalysisList(values);
    if (!items.length) return '';
    return `
        <section class="watchlist-analysis-section ${className}">
            <h4>${escapeHTML(title)}</h4>
            <ul>${items.map(item => `<li>${escapeHTML(typeof item === 'object' ? JSON.stringify(item) : item)}</li>`).join('')}</ul>
        </section>
    `;
}

function renderWatchlistStockAnalyses(values) {
    if (!Array.isArray(values) || !values.length) return '';
    return `
        <section class="watchlist-analysis-stock-list">
            <div class="watchlist-analysis-stock-head">
                <h4>逐只分析</h4>
                <span>${values.length} 只</span>
            </div>
            <div class="watchlist-analysis-stocks">
                ${values.map(item => {
                    const stock = item && typeof item === 'object' ? item : { summary: item };
                    const title = [stock.name, stock.symbol].filter(Boolean).join(' · ') || '未命名股票';
                    return `
                        <article class="watchlist-analysis-stock">
                            <div class="watchlist-analysis-stock-title">
                                <strong>${escapeHTML(title)}</strong>
                                ${stock.strength ? `<span class="tag">${escapeHTML(stock.strength)}</span>` : ''}
                            </div>
                            ${stock.summary ? `<p>${escapeHTML(stock.summary)}</p>` : ''}
                            ${stock.risk ? `<div><b>风险：</b>${escapeHTML(stock.risk)}</div>` : ''}
                            ${stock.next_check ? `<div><b>待验证：</b>${escapeHTML(stock.next_check)}</div>` : ''}
                        </article>
                    `;
                }).join('')}
            </div>
        </section>
    `;
}

function renderWatchlistAnalysisResult(data) {
    const node = document.getElementById('watchlistAnalysisResult');
    if (!node) return;
    let result = data?.result || {};
    if ((!result || typeof result !== 'object' || Array.isArray(result)) && data?.content) {
        try {
            result = JSON.parse(data.content);
        } catch (error) {
            result = {};
        }
    }
    const summary = result?.summary || data?.content || '模型没有返回可解析的结构化结论。';
    const confidence = result?.confidence ? ` · 信心：${result.confidence}` : '';
    const stocks = Array.isArray(data?.input?.stocks) ? data.input.stocks : [];
    const stockAnalyses = result?.stock_analyses || result?.stock_analysis || [];
    node.innerHTML = `
        <div class="watchlist-analysis-overview">
            <div>
                <span class="watchlist-analysis-eyebrow">AI 分析结论${escapeHTML(confidence)}</span>
                <h4>${escapeHTML(summary)}</h4>
            </div>
            <span class="tag">${escapeHTML(data?.model || '已配置模型')}</span>
        </div>
        <div class="watchlist-analysis-grid">
            ${renderWatchlistAnalysisList('改善因素', result?.bullish_points, 'is-positive')}
            ${renderWatchlistAnalysisList('风险因素', result?.risk_points, 'is-risk')}
            ${renderWatchlistAnalysisList('重点观察', result?.watch_levels)}
            ${renderWatchlistAnalysisList('下一交易日检查', result?.next_checks)}
            ${renderWatchlistAnalysisList('交易纪律', result?.discipline_notes)}
        </div>
        ${renderWatchlistStockAnalyses(stockAnalyses)}
        <div class="watchlist-analysis-foot">${escapeHTML(result?.disclaimer || '仅供学习研究和复盘，不构成投资建议')} · 覆盖 ${stocks.length || '当前'} 只股票</div>
    `;
    node.hidden = false;
}

async function runWatchlistAnalysis() {
    if (watchlistAnalysisBusy) return;
    const rows = Array.isArray(watchlistRows) ? watchlistRows : [];
    const symbols = rows.map(row => normalizeSymbol(row.code || row.symbol)).filter(Boolean);
    if (!symbols.length) {
        setWatchlistAnalysisStatus('当前没有可分析的自选股票。', 'error');
        return;
    }
    const credentialID = document.getElementById('watchlistAnalysisCredential')?.value || '';
    const credential = watchlistAnalysisCredentials().find(item => item.id === credentialID);
    if (!credential) {
        setWatchlistAnalysisStatus('请先在“AI 模型”中配置并启用一个模型。', 'error');
        return;
    }
    const submit = document.getElementById('watchlistAnalysisSubmit');
    const result = document.getElementById('watchlistAnalysisResult');
    watchlistAnalysisBusy = true;
    if (submit) {
        submit.disabled = true;
        submit.textContent = '分析中...';
    }
    if (result) {
        result.hidden = true;
        result.innerHTML = '';
    }
    setWatchlistAnalysisStatus('正在查询自选行情、日K、财务和资讯，请稍候...', '');
    try {
        await refreshWatchlistQuotes();
        const latestRows = Array.isArray(watchlistRows) ? watchlistRows : rows;
        const formulasForAnalysis = (Array.isArray(formulas) ? formulas : [])
            .filter(formula => formula && formula.enabled !== false && (formula.type || 'indicator') === 'indicator')
            .slice(0, 12)
            .map(formula => ({
                name: formula.name || '未命名指标',
                period: formula.period || 'day',
                script: formula.script || ''
            }));
        const data = await apiFetch('/api/ai/analyze/watchlist', {
            method: 'POST',
            body: JSON.stringify({
                provider: credential.provider,
                model: credential.model || defaultAIModel(credential.provider),
                credential_id: credential.id,
                pool_id: 'watchlist',
                symbols: symbols.slice(0, 20),
                input: {
                    analysis_focus: document.getElementById('watchlistAnalysisFocus')?.value || '综合研判',
                    data_scope: ['实时行情', '60日K线', '财务数据', '近期资讯'],
                    client_snapshot: latestRows.slice(0, 20).map(row => ({
                        symbol: normalizeSymbol(row.code || row.symbol),
                        name: row.name || '',
                        price: row.price ?? '',
                        change_percent: row.changePercent ?? '',
                        volume: row.volume ?? '',
                        amount: row.amount ?? '',
                        industry: row.industry ?? '',
                        turnover: row.turnover ?? '',
                        volume_ratio: row.volumeRatio ?? ''
                    })),
                    custom_indicators: formulasForAnalysis,
                    instruction: '请先给出自选池整体判断，再逐只指出相对强弱、主要风险和下一交易日需要验证的条件。不要编造未提供的数据。'
                },
                options: { max_tokens: 2400 }
            })
        });
        renderWatchlistAnalysisResult(data);
        setWatchlistAnalysisStatus(`分析完成 · ${data?.provider || credential.provider} / ${data?.model || credential.model || '默认模型'} · ${data?.latency_ms || 0}ms`, 'success');
    } catch (error) {
        setWatchlistAnalysisStatus(error.message || String(error), 'error');
    } finally {
        watchlistAnalysisBusy = false;
        if (submit) {
            submit.disabled = false;
            submit.textContent = '开始分析';
        }
    }
}

function watchlistChangeClass(value) {
    const change = Number.parseFloat(String(value ?? '').replace('%', ''));
    if (!Number.isFinite(change) || change === 0) return '';
    return change > 0 ? 'watchlist-up' : 'watchlist-down';
}

function formatWatchlistPrice(value) {
    const price = Number(value);
    return Number.isFinite(price) && price > 0 ? price.toFixed(2) : '--';
}

function formatWatchlistSigned(value, suffix = '') {
    const number = Number(value);
    if (!Number.isFinite(number)) return '--';
    const normalized = Math.abs(number) < 0.005 ? 0 : number;
    return `${normalized > 0 ? '+' : ''}${normalized.toFixed(2)}${suffix}`;
}

function formatWatchlistPercent(value) {
    const number = Number(value);
    if (!Number.isFinite(number) || number < 0) return '--';
    return `${number.toFixed(2)}%`;
}

function formatWatchlistPlain(value) {
    const number = Number(value);
    if (!Number.isFinite(number)) return '--';
    return number.toFixed(2);
}

function watchlistBidAskRatio(quote) {
    const buyTotal = (quote?.BuyLevel || []).reduce((sum, item) => sum + Number(item?.Number || 0), 0);
    const sellTotal = (quote?.SellLevel || []).reduce((sum, item) => sum + Number(item?.Number || 0), 0);
    const total = buyTotal + sellTotal;
    if (total <= 0) return '--';
    return formatWatchlistSigned(((buyTotal - sellTotal) / total) * 100, '%');
}

function watchlistQuoteRow(row, quote) {
    const previousClose = Number(quote?.K?.Last || 0) / 1000;
    const currentPrice = Number(quote?.K?.Close || 0) / 1000;
    if (currentPrice <= 0) return row;

    const change = previousClose > 0 ? currentPrice - previousClose : 0;
    const changePercent = previousClose > 0 ? (change / previousClose) * 100 : 0;
    return {
        ...row,
        price: formatWatchlistPrice(currentPrice),
        change: formatWatchlistSigned(change),
        changePercent: formatWatchlistSigned(changePercent, '%'),
        volume: quote.TotalHand > 0 ? formatAmount(Number(quote.TotalHand) * 100) : '--',
        volumeShares: quote.TotalHand > 0 ? Number(quote.TotalHand) * 100 : 0,
        amount: formatAmount(quote.Amount),
        speed: Number.isFinite(Number(quote.Rate)) && Number(quote.Rate) !== 0
            ? formatWatchlistSigned(quote.Rate, '%')
            : '--',
        entrust: watchlistBidAskRatio(quote)
    };
}

async function refreshWatchlistQuotes() {
    const rows = Array.isArray(watchlistRows) ? watchlistRows.slice() : [];
    const codes = rows.map(row => normalizeSymbol(row.code || row.symbol)).filter(Boolean);
    if (!codes.length) return;

    const requestID = ++watchlistQuoteRequestID;
    const statusNode = document.getElementById('watchlistStatus');
    try {
        const quotes = await apiFetch(`/api/quote?code=${encodeURIComponent(codes.join(','))}`);
        if (requestID !== watchlistQuoteRequestID) return;

        const quoteMap = new Map(
            (Array.isArray(quotes) ? quotes : []).map(quote => [normalizeSymbol(quote.Code), quote])
        );
        const mergedRows = rows.map(row => {
            const code = normalizeSymbol(row.code || row.symbol);
            return watchlistQuoteRow(row, quoteMap.get(code));
        });
        watchlistRows = mergedRows;
        renderWatchlist({ refreshQuotes: false });
        await enrichWatchlistRows(mergedRows, requestID);
        if (statusNode) statusNode.textContent = `共 ${watchlistRows.length} 只 · 更新于 ${localTimeString()}`;
    } catch (error) {
        if (requestID !== watchlistQuoteRequestID) return;
        if (statusNode) statusNode.textContent = `共 ${rows.length} 只 · 行情获取失败`;
        console.warn('刷新自选行情失败:', error);
    }
}

async function enrichWatchlistRows(rows, requestID) {
    const enriched = await Promise.all(rows.map(async row => {
        const code = normalizeSymbol(row.code || row.symbol);
        const [financeResult, klineResult, industryMap] = await Promise.allSettled([
            apiFetch(`/api/finance/standard?code=${encodeURIComponent(code)}`),
            apiFetch(`/api/kline-history?code=${encodeURIComponent(code)}&type=day&limit=8`),
            loadWatchlistIndustryMap()
        ]);

        let nextRow = { ...row };
        if (financeResult.status === 'fulfilled') {
            nextRow = applyWatchlistFinance(nextRow, financeResult.value);
        }
        if (klineResult.status === 'fulfilled') {
            nextRow = applyWatchlistKline(nextRow, klineResult.value);
        }
        if (industryMap.status === 'fulfilled') {
            const industry = industryMap.value.get(code);
            if (industry) nextRow.industry = industry;
        }
        return nextRow;
    }));
    if (requestID !== watchlistQuoteRequestID) return;
    watchlistRows = enriched;
    renderWatchlist({ refreshQuotes: false });
}

function applyWatchlistFinance(row, finance) {
    const volumeShares = Number(row.volumeShares || 0);
    const floatShares = Number(finance?.float_shares || 0);
    return {
        ...row,
        turnover: floatShares > 0 && volumeShares > 0
            ? formatWatchlistPercent((volumeShares / floatShares) * 100)
            : row.turnover ?? '--'
    };
}

function applyWatchlistKline(row, kline) {
    const list = kline?.List || kline?.list || [];
    if (list.length < 2) return row;
    const latest = list[list.length - 1];
    const previous = list.slice(Math.max(0, list.length - 6), list.length - 1);
    const avgVolume = previous.reduce((sum, item) => sum + Number(item.Volume ?? item.volume ?? 0), 0) / previous.length;
    const latestVolume = Number(latest.Volume ?? latest.volume ?? 0);
    return {
        ...row,
        volumeRatio: avgVolume > 0 && latestVolume > 0
            ? formatWatchlistPlain(latestVolume / avgVolume)
            : row.volumeRatio ?? '--'
    };
}

async function loadWatchlistIndustryMap() {
    if (!watchlistIndustryPromise) {
        watchlistIndustryPromise = apiFetch('/api/tdx-hy').then(data => {
            const map = new Map();
            (data?.list || data?.List || []).forEach(item => {
                const code = normalizeSymbol(item.Code || item.code);
                if (!code) return;
                const tdxHy = item.TdxHy || item.tdx_hy || '';
                const swHy = item.SwHy || item.sw_hy || '';
                map.set(code, [tdxHy, swHy].filter(Boolean).join(' / ') || '--');
            });
            return map;
        }).catch(error => {
            watchlistIndustryPromise = null;
            throw error;
        });
    }
    return watchlistIndustryPromise;
}

function bindWatchlistDrag() {
    const viewport = document.getElementById('watchlistScroll');
    if (!viewport || watchlistDragBound) return;
    watchlistDragBound = true;

    let dragging = false;
    let dragArmed = false;
    let startX = 0;
    let startScrollLeft = 0;
    let moved = 0;

    viewport.addEventListener('pointerdown', event => {
        if (event.pointerType === 'mouse' && event.button !== 0) return;
        if (event.target.closest('button, a, input, select, textarea, label')) return;
        dragArmed = true;
        dragging = false;
        moved = 0;
        startX = event.clientX;
        startScrollLeft = viewport.scrollLeft;
        viewport.setPointerCapture?.(event.pointerId);
    });
    viewport.addEventListener('pointermove', event => {
        if (!dragArmed) return;
        const deltaX = event.clientX - startX;
        moved = Math.max(moved, Math.abs(deltaX));
        if (!dragging && moved < 6) return;
        if (!dragging) {
            dragging = true;
            viewport.classList.add('is-dragging');
        }
        viewport.scrollLeft = startScrollLeft - (event.clientX - startX);
    });
    const stopDragging = event => {
        if (!dragArmed) return;
        dragArmed = false;
        dragging = false;
        viewport.classList.remove('is-dragging');
        if (event?.pointerId !== undefined) viewport.releasePointerCapture?.(event.pointerId);
    };
    viewport.addEventListener('pointerup', stopDragging);
    viewport.addEventListener('pointercancel', stopDragging);
    viewport.addEventListener('pointerleave', event => {
        if (event.pointerType === 'mouse') stopDragging(event);
    });
}

function bindWatchlistClick() {
    const body = document.getElementById('watchlistTableBody');
    if (!body || watchlistClickBound) return;
    watchlistClickBound = true;

    const openFromRow = row => {
        const symbol = row?.dataset?.watchlistSymbol;
        const title = row?.dataset?.watchlistName || '';
        if (symbol) openWatchlistSymbol(symbol, title);
    };

    body.addEventListener('click', event => {
        const codeButton = event.target.closest('button[data-watchlist-symbol]');
        if (codeButton && body.contains(codeButton)) {
            event.preventDefault();
            event.stopPropagation();
            openWatchlistSymbol(codeButton.dataset.watchlistSymbol, codeButton.dataset.watchlistName || '');
            return;
        }
        const row = event.target.closest('tr[data-watchlist-symbol]');
        if (!row || !body.contains(row)) return;
        openFromRow(row);
    });
    body.addEventListener('keydown', event => {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        const row = event.target.closest('tr[data-watchlist-symbol]');
        if (!row || !body.contains(row)) return;
        event.preventDefault();
        openFromRow(row);
    });
}

function openWatchlistSymbol(code, title = '') {
    const symbol = normalizeSymbol(code);
    if (!symbol) return;
    openWatchlistQuoteDialog(symbol, 'minute', title);
}

function openWatchlistQuoteDialog(symbol, period = 'minute', title = '') {
    const normalized = normalizeSymbol(symbol);
    if (!normalized) return;
    resetWatchlistQuoteIndicators();
    watchlistQuoteDialogState.open = true;
    watchlistQuoteDialogState.symbol = normalized;
    watchlistQuoteDialogState.title = title || '';
    watchlistQuoteDialogState.period = period;
    syncWatchlistQuotePeriodTabs(period);
    showWatchlistQuoteDialog();
    refreshWatchlistQuoteDialog();
}

function showWatchlistQuoteDialog() {
    const dialog = document.getElementById('watchlistQuoteDialog');
    if (!dialog) return;
    dialog.classList.add('open');
    dialog.setAttribute('aria-hidden', 'false');
}

function closeWatchlistQuoteDialog() {
    closeWatchlistQuoteIndicatorMenu();
    const dialog = document.getElementById('watchlistQuoteDialog');
    if (dialog) {
        dialog.classList.remove('open');
        dialog.setAttribute('aria-hidden', 'true');
    }
    watchlistQuoteDialogState.open = false;
}

function openWatchlistQuoteInProChart() {
    const symbol = watchlistQuoteDialogState.symbol;
    if (!symbol) return;
    const hqInput = document.getElementById('hqSymbol');
    if (hqInput) hqInput.value = symbol;
    closeWatchlistQuoteDialog();
    switchWorkspace('proChart', document.querySelectorAll('.workspace-tab')[1]);
}

function watchlistQuotePeriodToHqPeriod(period) {
    switch (period) {
        case 'minute':
            return 'minute1';
        case 'minute120':
            return 'minute120';
        case 'minute60':
            return 'hour';
        case 'minute30':
            return 'minute30';
        case 'minute15':
            return 'minute15';
        case 'week':
            return 'week';
        case 'month':
            return 'month';
        default:
            return 'day';
    }
}

function switchWatchlistQuotePeriod(period, button) {
    closeWatchlistQuoteIndicatorMenu();
    watchlistQuoteDialogState.period = period;
    syncWatchlistQuoteIndicatorVisibility();
    syncWatchlistQuotePeriodTabs(period, button);
    refreshWatchlistQuoteDialog();
}

function syncWatchlistQuotePeriodTabs(period, activeButton) {
    document.querySelectorAll('.quote-period-tabs .tab-btn').forEach(btn => {
        const active = activeButton ? btn === activeButton : btn.dataset.period === period;
        btn.classList.toggle('active', active);
    });
}

function watchlistQuoteRenderOptions(period) {
    if (String(period || '').startsWith('minute')) {
        return { count: 240, pageSize: 80, dataWidth: 8 };
    }
    return { count: 800, pageSize: 80 };
}

function resetWatchlistQuoteIndicators() {
    watchlistQuoteIndicatorState.activeWindow = 1;
    watchlistQuoteIndicatorState.windows = [
        { defaultIndex: 'MA', defaultName: 'MA', formulaID: '' },
        { defaultIndex: 'VOL', defaultName: 'VOL', formulaID: '' },
        { defaultIndex: 'MACD', defaultName: 'MACD', formulaID: '' }
    ];
    syncWatchlistQuoteIndicatorVisibility();
    renderWatchlistQuoteIndicatorControls();
    closeWatchlistQuoteIndicatorMenu();
}

function syncWatchlistQuoteIndicatorVisibility() {
    const toolbar = document.querySelector('.quote-indicator-toolbar');
    const isKLine = watchlistQuoteDialogState.period !== 'minute';
    if (toolbar) toolbar.hidden = !isKLine;
    if (!isKLine) closeWatchlistQuoteIndicatorMenu();
}

function watchlistQuoteIndicatorFormula(windowIndex) {
    const setting = watchlistQuoteIndicatorState.windows[windowIndex];
    if (!setting?.formulaID) return null;
    return formulas.find(formula => formula.id === setting.formulaID) || null;
}

function watchlistQuoteIndicatorWindows() {
    return watchlistQuoteIndicatorState.windows.map((setting, windowIndex) => {
        const formula = watchlistQuoteIndicatorFormula(windowIndex);
        if (!formula) return { Index: setting.defaultIndex };
        return {
            Name: formula.name || '自定义指标',
            Script: formula.script,
            Args: parseHQFormulaArgs(formula),
            IsMainIndex: windowIndex === 0
        };
    });
}

function watchlistQuoteIndicatorLabel(windowIndex) {
    const setting = watchlistQuoteIndicatorState.windows[windowIndex];
    return watchlistQuoteIndicatorFormula(windowIndex)?.name || setting?.defaultName || '指标';
}

function renderWatchlistQuoteIndicatorControls() {
    const node = document.getElementById('watchlistQuoteIndicatorControls');
    if (!node) return;
    node.innerHTML = watchlistQuoteIndicatorState.windows.map((setting, index) => `
        <button
            type="button"
            class="quote-indicator-trigger"
            onclick="openWatchlistQuoteIndicatorMenu(${index})"
            title="修改${escapeHTML(setting.defaultName)}指标"
        >
            <span>${escapeHTML(watchlistQuoteIndicatorLabel(index))}</span>
            <small aria-hidden="true">⌄</small>
        </button>
    `).join('');
}

function closeWatchlistQuoteIndicatorMenu() {
    const menu = document.getElementById('watchlistQuoteIndicatorMenu');
    if (!menu) return;
    menu.classList.remove('open');
    menu.setAttribute('aria-hidden', 'true');
}

async function openWatchlistQuoteIndicatorMenu(windowIndex) {
    watchlistQuoteIndicatorState.activeWindow = windowIndex;
    const menu = document.getElementById('watchlistQuoteIndicatorMenu');
    if (!menu) return;
    try {
        if (!formulas.length) formulas = await apiFetch('/api/formulas');
        const indicatorFormulas = formulas.filter(formula => (
            formula.enabled !== false && (formula.type || 'indicator') === 'indicator'
        ));
        menu.innerHTML = `
            <div class="quote-indicator-menu-title">我的自定义指标</div>
            ${indicatorFormulas.length
                ? indicatorFormulas.map(formula => `
                    <button type="button" class="quote-indicator-option" onclick="selectWatchlistQuoteIndicator('${escapeJSString(formula.id)}')">
                        <span>${escapeHTML(formula.name || '未命名指标')}</span>
                        <small>${escapeHTML(formatFormulaArgs(formula))}</small>
                    </button>
                `).join('')
                : '<div class="quote-indicator-empty">暂无图表指标，请先在公式管理中创建</div>'}
            <button type="button" class="quote-indicator-option default-option" onclick="selectWatchlistQuoteIndicator('')">
                <span>恢复默认指标</span>
                <small>${escapeHTML(watchlistQuoteIndicatorState.windows[windowIndex]?.defaultName || '')}</small>
            </button>
        `;
        menu.classList.add('open');
        menu.setAttribute('aria-hidden', 'false');
    } catch (error) {
        menu.innerHTML = `<div class="quote-indicator-empty">${escapeHTML(error.message || error)}</div>`;
        menu.classList.add('open');
        menu.setAttribute('aria-hidden', 'false');
    }
}

function renderWatchlistQuoteChart() {
    const { symbol, period } = watchlistQuoteDialogState;
    if (!watchlistQuoteDialogState.open || !symbol || period === 'minute') return;
    const hqPeriod = watchlistQuotePeriodToHqPeriod(period);
    const chartOptions = {
        ...watchlistQuoteRenderOptions(period),
        windows: watchlistQuoteIndicatorWindows()
    };
    renderHQKLine('watchlistQuoteChart', symbol, hqPeriod, chartOptions);
    window.TDXHQChart?.resize?.(document.getElementById('watchlistQuoteChart'));
}

function selectWatchlistQuoteIndicator(formulaID) {
    const windowIndex = watchlistQuoteIndicatorState.activeWindow;
    const setting = watchlistQuoteIndicatorState.windows[windowIndex];
    if (!setting) return;
    setting.formulaID = formulaID || '';
    closeWatchlistQuoteIndicatorMenu();
    renderWatchlistQuoteIndicatorControls();

    const chartContainer = document.getElementById('watchlistQuoteChart');
    const chart = window.TDXHQChart?.getChart?.(chartContainer);
    const formula = watchlistQuoteIndicatorFormula(windowIndex);
    if (!chart || typeof chart.ChangeScriptIndex !== 'function' || !formula) {
        renderWatchlistQuoteChart();
        return;
    }
    chart.ChangeScriptIndex(windowIndex, {
        Name: formula.name || '自定义指标',
        Script: formula.script,
        Args: parseHQFormulaArgs(formula),
        IsMainIndex: windowIndex === 0
    });
    window.TDXHQChart?.resize?.(chartContainer);
}

async function refreshWatchlistQuoteDialog() {
    const { symbol, period } = watchlistQuoteDialogState;
    if (!symbol) return;
    const titleNode = document.getElementById('watchlistQuoteTitle');
    const codeNode = document.getElementById('watchlistQuoteCode');
    const metaNode = document.getElementById('watchlistQuoteMeta');
    const priceNode = document.getElementById('watchlistQuotePrice');
    const changeNode = document.getElementById('watchlistQuoteChange');
    const changePctNode = document.getElementById('watchlistQuoteChangePct');
    const quoteTitle = watchlistQuoteDialogState.title || symbol;
    if (titleNode) titleNode.textContent = quoteTitle;
    if (codeNode) codeNode.textContent = symbol;
    if (metaNode) metaNode.textContent = '加载中...';
    if (priceNode) priceNode.textContent = '--';
    if (changeNode) changeNode.textContent = '--';
    if (changePctNode) changePctNode.textContent = '--';

    try {
        syncWatchlistQuotePeriodTabs(period);
        const [quoteList, finance, tdxHy, klineResp] = await Promise.all([
            apiFetch(`/api/quote?code=${encodeURIComponent(symbol)}`),
            apiFetch(`/api/finance/standard?code=${encodeURIComponent(symbol)}`),
            loadWatchlistIndustryMap(),
            apiFetch(`/api/kline-history?code=${encodeURIComponent(symbol)}&type=${encodeURIComponent(watchlistQuotePeriodToHqPeriod(period))}&limit=120`)
        ]);
        const quote = Array.isArray(quoteList) ? quoteList[0] : null;
        if (!watchlistQuoteDialogState.title) {
            watchlistQuoteDialogState.title = quote?.Name || quote?.name || symbol;
            if (titleNode) titleNode.textContent = watchlistQuoteDialogState.title;
        }
        watchlistQuoteDialogState.quote = quote || null;
        watchlistQuoteDialogState.quoteData = klineResp || null;
        renderWatchlistQuoteDialog(symbol, quote, finance, tdxHy, klineResp, period);
        const hqPeriod = watchlistQuotePeriodToHqPeriod(period);
        const chartOptions = watchlistQuoteRenderOptions(period);
        const renderChart = () => {
            if (!watchlistQuoteDialogState.open) return;
            if (period === 'minute') {
                renderHQMinute('watchlistQuoteChart', symbol, chartOptions);
            } else {
                renderWatchlistQuoteChart();
            }
            window.TDXHQChart?.resize?.(document.getElementById('watchlistQuoteChart'));
        };
        requestAnimationFrame(() => {
            renderChart();
            setTimeout(renderChart, 180);
        });
    } catch (error) {
        if (metaNode) metaNode.textContent = error.message || String(error);
        const chart = document.getElementById('watchlistQuoteChart');
        if (chart) chart.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
    }
}

function renderWatchlistQuoteDialog(symbol, quote, finance, industryMap, klineResp, period) {
    const titleNode = document.getElementById('watchlistQuoteTitle');
    const metaNode = document.getElementById('watchlistQuoteMeta');
    const priceNode = document.getElementById('watchlistQuotePrice');
    const changeNode = document.getElementById('watchlistQuoteChange');
    const changePctNode = document.getElementById('watchlistQuoteChangePct');
    const metricsNode = document.getElementById('watchlistQuoteMetrics');
    const sellNode = document.getElementById('watchlistQuoteSellLevels');
    const buyNode = document.getElementById('watchlistQuoteBuyLevels');
    if (!metricsNode || !sellNode || !buyNode) return;

    const code = normalizeSymbol(symbol);
    const exchangeName = quote?.Exchange === 1 ? '沪市' : (quote?.Exchange === 0 ? '深市' : '北交所');
    const currentPrice = Number(quote?.K?.Close || 0) / 1000;
    const lastClose = Number(quote?.K?.Last || 0) / 1000;
    const openPrice = Number(quote?.K?.Open || 0) / 1000;
    const highPrice = Number(quote?.K?.High || 0) / 1000;
    const lowPrice = Number(quote?.K?.Low || 0) / 1000;
    const change = currentPrice > 0 && lastClose > 0 ? currentPrice - lastClose : 0;
    const changePct = lastClose > 0 ? (change / lastClose) * 100 : 0;
    const floatShares = Number(finance?.float_shares || 0);
    const volumeShares = Number(quote?.TotalHand || 0) * 100;
    const turnover = floatShares > 0 && volumeShares > 0 ? (volumeShares / floatShares) * 100 : 0;
    const list = klineResp?.List || klineResp?.list || [];
    const avgVolume = list.length > 1
        ? list.slice(Math.max(0, list.length - 6), list.length - 1).reduce((sum, item) => sum + Number(item.Volume ?? 0), 0) / Math.max(1, Math.min(5, list.length - 1))
        : 0;
    const volumeRatio = avgVolume > 0 ? (Number(list[list.length - 1]?.Volume || 0) / avgVolume) : 0;
    const industry = industryMap?.get(code) || '--';
    const quoteTitle = watchlistQuoteDialogState.title || quote?.Name || quote?.name || code;
    if (titleNode) titleNode.textContent = quoteTitle;

    if (metaNode) {
        metaNode.textContent = `${exchangeName} · ${industry} · ${period === 'minute' ? '分时' : watchlistQuotePeriodToHqPeriod(period).toUpperCase()}`;
    }
    if (priceNode) priceNode.textContent = currentPrice > 0 ? currentPrice.toFixed(2) : '--';
    if (changeNode) {
        changeNode.textContent = currentPrice > 0 ? formatWatchlistSigned(change) : '--';
        changeNode.className = `quote-summary-change ${change >= 0 ? 'watchlist-up' : 'watchlist-down'}`;
    }
    if (changePctNode) {
        changePctNode.textContent = currentPrice > 0 ? formatWatchlistSigned(changePct, '%') : '--';
        changePctNode.className = `quote-summary-change-pct ${change >= 0 ? 'watchlist-up' : 'watchlist-down'}`;
    }
    metricsNode.innerHTML = [
        { label: '今开', value: openPrice > 0 ? openPrice.toFixed(2) : '--' },
        { label: '最高', value: highPrice > 0 ? highPrice.toFixed(2) : '--' },
        { label: '最低', value: lowPrice > 0 ? lowPrice.toFixed(2) : '--' },
        { label: '昨收', value: lastClose > 0 ? lastClose.toFixed(2) : '--' },
        { label: '成交量', value: quote?.TotalHand ? formatAmount(Number(quote.TotalHand) * 100) : '--' },
        { label: '成交额', value: quote?.Amount ? formatAmount(quote.Amount) : '--' },
        { label: '换手率', value: turnover > 0 ? formatWatchlistPercent(turnover) : '--' },
        { label: '量比', value: volumeRatio > 0 ? formatWatchlistPlain(volumeRatio) : '--' },
        { label: '委比', value: watchlistBidAskRatio(quote) }
    ].map(item => `
        <div class="quote-summary-item">
            <span>${escapeHTML(item.label)}</span>
            <strong>${escapeHTML(item.value)}</strong>
        </div>
    `).join('');

    sellNode.innerHTML = (quote?.SellLevel || []).slice().reverse().map((item, idx) => `
        <tr>
            <td>卖${5 - idx}</td>
            <td>${escapeHTML(formatWatchlistPrice(Number(item?.Price || 0) / 1000))}</td>
            <td>${escapeHTML(item?.Number ? String(Math.round(Number(item.Number) / 100)) : '--')}</td>
        </tr>
    `).join('') || '<tr><td>卖五</td><td>--</td><td>--</td></tr>';
    buyNode.innerHTML = (quote?.BuyLevel || []).map((item, idx) => `
        <tr>
            <td>买${idx + 1}</td>
            <td>${escapeHTML(formatWatchlistPrice(Number(item?.Price || 0) / 1000))}</td>
            <td>${escapeHTML(item?.Number ? String(Math.round(Number(item.Number) / 100)) : '--')}</td>
        </tr>
    `).join('') || '<tr><td>买一</td><td>--</td><td>--</td></tr>';
}

function switchWorkspace(name, button) {
    document.querySelectorAll('.workspace-tab').forEach(btn => btn.classList.remove('active'));
    if (button) button.classList.add('active');
    document.querySelectorAll('.workspace').forEach(item => item.classList.remove('active'));
    document.getElementById(name + 'Workspace').classList.add('active');
    if (name === 'proChart') {
        setTimeout(() => {
            loadFormulaList();
            loadHQChart();
            if (window.TDXHQChart) window.TDXHQChart.resize();
        }, 50);
    }
    if (name === 'market') renderWatchlist();
    if (name === 'dataCenter') loadDataCenter();
    if (name === 'selectionResults') loadSelectionResults();
    if (name === 'dailyReview') loadDailyReview();
    if (name === 'tradingSystem') {
        renderTradingSystem();
        startTradingAutoRefresh();
        if (isTradingSession() && !tradingLastRefreshAt) {
            refreshTradingQuotes({ silent: true });
        }
    }
    if (name === 'strategies') loadStrategyCenter();
    if (name === 'automations') loadAutomationData();
    if (name === 'aiConfigs') loadAIConfigData();
    if (name === 'webhooks') loadWebhooks();
}

// 工具函数 - 格式化数字
function formatNumber(num, decimals = 2) {
    if (!num || isNaN(num)) return '--';
    return parseFloat(num).toFixed(decimals);
}

// 工具函数 - 格式化金额（转换为万、亿）
function formatAmount(num) {
    if (!num || isNaN(num)) return '--';
    num = parseFloat(num);
    if (num >= 100000000) {
        return (num / 100000000).toFixed(2) + '亿';
    } else if (num >= 10000) {
        return (num / 10000).toFixed(2) + '万';
    }
    return num.toFixed(2);
}

// 工具函数 - 格式化价格（将厘转为元）
function formatPrice(price) {
    if (!price || isNaN(price)) return '--';
    return (parseFloat(price) / 1000).toFixed(2);
}

function prettyJSON(value) {
    return JSON.stringify(value, null, 2);
}

function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>"']/g, ch => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
    }[ch]));
}

function escapeJSString(value) {
    return String(value ?? '').replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n').replace(/\r/g, '');
}

function compactValue(value) {
    if (value === null || value === undefined || value === '') return '--';
    if (typeof value === 'number') return Number.isInteger(value) ? String(value) : value.toFixed(4).replace(/\.?0+$/, '');
    if (typeof value === 'boolean') return value ? '是' : '否';
    if (Array.isArray(value)) return value.length > 6 ? `${value.slice(0, 6).join(', ')} ...` : value.join(', ');
    if (typeof value === 'object') return JSON.stringify(value);
    return String(value);
}

function normalizeSymbol(symbol) {
    return String(symbol || '').trim().toUpperCase().replace(/\.(SH|SZ|BJ)$/i, '');
}

function renderHQKLine(containerId, symbol, period = 'day', options = {}) {
    const container = document.getElementById(containerId);
    if (!container) return false;
    if (!window.TDXHQChart || !window.TDXHQChart.isAvailable()) {
        container.innerHTML = '<div class="data-item">HQChart 未加载，无法显示图表</div>';
        return false;
    }
    const ok = window.TDXHQChart.renderKLine(container, {
        symbol: normalizeSymbol(symbol),
        period,
        ...options
    });
    if (!ok) {
        container.innerHTML = '<div class="data-item">HQChart 初始化失败</div>';
    }
    return ok;
}

function renderHQMinute(containerId, symbol, options = {}) {
    const container = document.getElementById(containerId);
    if (!container) return false;
    if (!window.TDXHQChart || !window.TDXHQChart.isAvailable()) {
        container.innerHTML = '<div class="data-item">HQChart 未加载，无法显示图表</div>';
        return false;
    }
    const ok = window.TDXHQChart.renderMinute(container, {
        symbol: normalizeSymbol(symbol),
        ...options
    });
    if (!ok) {
        container.innerHTML = '<div class="data-item">HQChart 分时图初始化失败</div>';
    }
    return ok;
}

function poolByID(id) {
    return pools.find(pool => pool.id === id);
}

function symbolInPool(symbol, poolID) {
    const pool = poolByID(poolID);
    const normalized = normalizeSymbol(symbol);
    return !!pool && (pool.symbols || []).map(normalizeSymbol).includes(normalized);
}

function localDateString(date = new Date()) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

function localTimeString(date = new Date()) {
    return date.toLocaleTimeString('zh-CN', { hour12: false });
}

function setLoadingText(containerId, text = '加载中...') {
    const container = document.getElementById(containerId);
    if (container) container.innerHTML = `<div class="data-item">${escapeHTML(text)}</div>`;
}

function setErrorText(containerId, error) {
    const container = document.getElementById(containerId);
    if (container) container.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
}

function renderMetricCards(containerId, items) {
    const container = document.getElementById(containerId);
    if (!container) return;
    container.innerHTML = items.map(item => `
        <div class="metric-card">
            <span class="metric-label">${escapeHTML(item.label)}</span>
            <span class="metric-value">${escapeHTML(item.value)}</span>
            ${item.note ? `<span class="metric-note">${escapeHTML(item.note)}</span>` : ''}
        </div>
    `).join('');
}

function renderTable(containerId, rows, columns) {
    const container = document.getElementById(containerId);
    if (!container) return;
    if (!rows || rows.length === 0) {
        container.innerHTML = '<div class="data-item">暂无数据</div>';
        return;
    }
    const visibleRows = rows.slice(0, 300);
    container.innerHTML = `
        <table class="data-table">
            <thead>
                <tr>${columns.map(col => `<th>${escapeHTML(col.label)}</th>`).join('')}</tr>
            </thead>
            <tbody>
                ${visibleRows.map(row => `
                    <tr>
                        ${columns.map(col => `<td>${escapeHTML(compactValue(typeof col.value === 'function' ? col.value(row) : row[col.key]))}</td>`).join('')}
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;
}

function renderKeyValuePanel(title, data) {
    const entries = Object.entries(data || {}).filter(([, value]) => value !== undefined && value !== null && typeof value !== 'object');
    if (entries.length === 0) {
        return `
            <div class="data-panel">
                <div class="data-panel-title">${escapeHTML(title)}</div>
                <div class="data-panel-body"><pre class="json-output">${escapeHTML(prettyJSON(data || {}))}</pre></div>
            </div>
        `;
    }
    return `
        <div class="data-panel">
            <div class="data-panel-title">${escapeHTML(title)}</div>
            <div class="data-panel-body">
                <div class="kv-grid">
                    ${entries.slice(0, 80).map(([key, value]) => `
                        <div class="kv-item">
                            <span class="kv-key">${escapeHTML(key)}</span>
                            <span class="kv-value">${escapeHTML(compactValue(value))}</span>
                        </div>
                    `).join('')}
                </div>
            </div>
        </div>
    `;
}

function renderJsonPanel(title, data) {
    return `
        <div class="data-panel">
            <div class="data-panel-title">${escapeHTML(title)}</div>
            <div class="data-panel-body"><pre class="json-output">${escapeHTML(prettyJSON(data))}</pre></div>
        </div>
    `;
}

function hikyuuTaskItem(task) {
    const statusClass = String(task.status || '').replace(/[^a-z0-9_-]/gi, '');
    const request = task.request || {};
    const progress = Number.isFinite(Number(task.progress)) ? `${Math.max(0, Math.min(100, Number(task.progress)))}%` : '';
    const options = [
        request.day ? '日线' : '',
        request.min ? '1分钟' : '',
        request.min5 ? '5分钟' : '',
        request.stock ? '股票' : '',
        request.fund ? '基金' : '',
        request.weight ? '权息' : '',
        request.finance ? '财务' : '',
        request.block ? '板块' : ''
    ].filter(Boolean).join(' · ');
    return `
        <div class="data-item hikyuu-task-item ${statusClass}">
            <div class="data-item-title">${escapeHTML(task.type || '--')} <span class="tag">${escapeHTML(task.status || '--')}</span></div>
            <div class="data-item-meta">${escapeHTML(task.started_at || '--')}${task.ended_at ? ` · 结束：${escapeHTML(task.ended_at)}` : ''}${task.exit_code !== null && task.exit_code !== undefined ? ` · 退出码：${escapeHTML(task.exit_code)}` : ''}</div>
            <div class="data-item-meta">任务号：${escapeHTML(task.id || '--')}</div>
            <div class="data-item-meta">参数：${escapeHTML(options || '--')}${task.stage ? ` · 阶段：${escapeHTML(task.stage)}` : ''}${progress ? ` · 进度：${escapeHTML(progress)}` : ''}${task.error ? ` · ${escapeHTML(task.error)}` : ''}</div>
            ${Array.isArray(task.log_tail) && task.log_tail.length ? `<pre class="json-output hikyuu-log-tail">${escapeHTML(task.log_tail.join('\n'))}</pre>` : ''}
        </div>
    `;
}

function hikyuuSyncPayload() {
    return {
        day: document.getElementById('hikyuuDay')?.checked ?? true,
        min: document.getElementById('hikyuuMin')?.checked ?? true,
        min5: document.getElementById('hikyuuMin5')?.checked ?? true,
        stock: document.getElementById('hikyuuStock')?.checked ?? true,
        fund: document.getElementById('hikyuuFund')?.checked ?? true,
        weight: document.getElementById('hikyuuWeight')?.checked ?? true,
        finance: document.getElementById('hikyuuFinance')?.checked ?? true,
        block: document.getElementById('hikyuuBlock')?.checked ?? true
    };
}

async function loadHikyuuData(force = false) {
    if (hikyuuDataLoaded && !force) return;
    hikyuuDataLoaded = true;
    await Promise.allSettled([loadHikyuuStatus(), loadHikyuuTasks()]);
    startHikyuuPollingIfNeeded();
}

function updateHikyuuProgress(tasks) {
    const wrap = document.getElementById('hikyuuProgressWrap');
    const bar = document.getElementById('hikyuuProgressBar');
    const text = document.getElementById('hikyuuProgressText');
    if (!wrap || !bar || !text) return;

    const task = (tasks || []).find(item => item.status === 'running' || item.status === 'pending');
    if (!task) {
        hikyuuSyncActiveTaskID = '';
        wrap.style.display = 'none';
        bar.classList.remove('indeterminate');
        bar.style.width = '0%';
        text.textContent = '--';
        return;
    }

    hikyuuSyncActiveTaskID = task.id || '';
    wrap.style.display = 'block';
    const progress = Number(task.progress);
    if (Number.isFinite(progress)) {
        bar.classList.remove('indeterminate');
        bar.style.width = `${Math.max(0, Math.min(100, progress))}%`;
        text.textContent = `${task.stage || '同步中'} · ${Math.round(progress)}%${task.message ? ` · ${task.message}` : ''}`;
    } else {
        bar.classList.add('indeterminate');
        bar.style.width = '35%';
        text.textContent = `${task.stage || '同步中'}${task.message ? ` · ${task.message}` : ''}`;
    }
}

function startHikyuuPollingIfNeeded() {
    if (hikyuuSyncPollTimer || !hikyuuSyncActiveTaskID) return;
    hikyuuSyncPollTimer = setInterval(async () => {
        await Promise.allSettled([loadHikyuuStatus(), loadHikyuuTasks()]);
        if (!hikyuuSyncActiveTaskID) {
            clearInterval(hikyuuSyncPollTimer);
            hikyuuSyncPollTimer = null;
        }
    }, 5000);
}

async function loadHikyuuStatus() {
    setLoadingText('hikyuuStatusStats');
    const hint = document.getElementById('hikyuuStatusHint');
    try {
        const data = await apiFetch('/api/hikyuu/health');
        if (hint) hint.textContent = data.scheduler_enabled ? `定时任务启用 · ${data.after_close_cron}` : '定时任务已停用';
        renderMetricCards('hikyuuStatusStats', [
            { label: '服务状态', value: data.service || 'hikyuu-data-service', note: data.time || '--' },
            { label: '数据目录', value: data.stocks_dir || '--', note: data.config_dir || '--' },
            { label: '日志目录', value: data.log_dir || '--', note: data.active_task_id ? `当前任务 ${data.active_task_id}` : '空闲' },
            { label: '定时器', value: data.scheduler_enabled ? '启用' : '停用', note: data.after_close_cron || '--' }
        ]);
    } catch (error) {
        if (hint) hint.textContent = '数据服务连接异常';
        setErrorText('hikyuuStatusStats', error);
    }
}

async function loadHikyuuTasks() {
    const list = document.getElementById('hikyuuTaskList');
    if (!list) return;
    list.innerHTML = '<div class="data-item">正在加载任务...</div>';
    try {
        const data = await apiFetch('/api/hikyuu/tasks');
        const items = Array.isArray(data) ? data.slice(0, 8) : [];
        list.innerHTML = items.length ? items.map(hikyuuTaskItem).join('') : '<div class="data-item">暂无任务</div>';
        updateHikyuuProgress(items);
    } catch (error) {
        list.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
        updateHikyuuProgress([]);
    }
}

async function startHikyuuSync(mode) {
    const url = mode === 'after-close-sync' ? '/api/hikyuu/tasks/after-close-sync' : '/api/hikyuu/tasks/full-sync';
    try {
        const task = await apiFetch(url, { method: 'POST', body: JSON.stringify(hikyuuSyncPayload()) });
        hikyuuSyncActiveTaskID = task?.id || '';
        await loadHikyuuData(true);
        startHikyuuPollingIfNeeded();
    } catch (error) {
        alert(error.message);
    }
}

window.addEventListener('beforeunload', (event) => {
    if (!hikyuuSyncActiveTaskID) return;
    event.preventDefault();
    event.returnValue = 'Hikyuu 行情数据正在同步，确认刷新页面吗？后台任务不会因页面刷新自动停止。';
});

// 搜索股票
async function searchStock() {
    const keyword = document.getElementById('stockCode').value.trim();
    if (!keyword) {
        alert('请输入股票代码或名称');
        return;
    }

    // 如果直接输入的是6位股票代码，直接加载
    if (/^\d{6}$/.test(keyword)) {
        currentStock = keyword;
        loadStockData(keyword);
        return;
    }

    // 否则搜索
    showLoading();
    try {
        const response = await fetch(`/api/search?keyword=${encodeURIComponent(keyword)}`);
        const result = await response.json();
        
        if (result.code === 0 && result.data && result.data.length > 0) {
            displaySearchResults(result.data);
        } else {
            alert('未找到相关股票');
        }
    } catch (error) {
        console.error('搜索失败:', error);
        alert('搜索失败，请重试');
    } finally {
        hideLoading();
    }
}

// 显示搜索结果
function displaySearchResults(results) {
    const container = document.getElementById('searchResults');
    container.innerHTML = '';
    
    results.forEach(item => {
        const div = document.createElement('div');
        div.className = 'search-item';
        div.innerHTML = `
            <span class="search-item-code">${item.code}</span>
            <span class="search-item-name">${item.name}</span>
        `;
        div.onclick = () => {
            currentStock = item.code;
            loadStockData(item.code);
            container.innerHTML = '';
        };
        container.appendChild(div);
    });
}

// 加载股票数据
async function loadStockData(code) {
    showLoading();
    document.getElementById('mainContent').style.display = 'block';
    
    try {
        // 加载五档行情
        await loadQuote(code);
        
        // 默认加载日K线
        await loadKline('day');
        
        // 加载分时数据
        await loadMinute(code);
        
        // 加载分时成交
        await loadTrade(code);
        
    } catch (error) {
        console.error('加载数据失败:', error);
        alert('加载数据失败，请重试');
    } finally {
        hideLoading();
    }
}

// 加载五档行情
async function loadQuote(code) {
    try {
        const response = await fetch(`/api/quote?code=${code}`);
        const result = await response.json();
        
        if (result.code === 0 && result.data && result.data.length > 0) {
            const quote = result.data[0];
            displayQuote(quote);
        }
    } catch (error) {
        console.error('加载五档行情失败:', error);
    }
}

// 显示五档行情
function displayQuote(quote) {
    // 更新股票名称和代码
    document.getElementById('stockName').textContent = quote.Code || '--';
    document.getElementById('stockCode2').textContent = quote.Code || '--';
    
    // 计算价格（从厘转为元）
    const lastPrice = parseFloat(quote.K.Last) / 1000;
    const currentPrice = parseFloat(quote.K.Close) / 1000;
    const openPrice = parseFloat(quote.K.Open) / 1000;
    const highPrice = parseFloat(quote.K.High) / 1000;
    const lowPrice = parseFloat(quote.K.Low) / 1000;
    
    const priceChange = currentPrice - lastPrice;
    const priceChangePercent = lastPrice > 0 ? (priceChange / lastPrice * 100) : 0;
    
    // 更新基本信息
    document.getElementById('lastPrice').textContent = currentPrice.toFixed(2);
    document.getElementById('priceChange').textContent = priceChange > 0 ? '+' + priceChange.toFixed(2) : priceChange.toFixed(2);
    document.getElementById('priceChangePercent').textContent = priceChangePercent > 0 ? '+' + priceChangePercent.toFixed(2) + '%' : priceChangePercent.toFixed(2) + '%';
    document.getElementById('volume').textContent = formatAmount(quote.TotalHand * 100);
    document.getElementById('amount').textContent = formatAmount(quote.Amount);
    document.getElementById('openPrice').textContent = openPrice.toFixed(2);
    document.getElementById('highPrice').textContent = highPrice.toFixed(2);
    document.getElementById('lowPrice').textContent = lowPrice.toFixed(2);
    
    // 设置涨跌颜色
    const priceElements = [document.getElementById('lastPrice'), 
                          document.getElementById('priceChange'), 
                          document.getElementById('priceChangePercent')];
    priceElements.forEach(el => {
        el.className = 'value price ' + (priceChange >= 0 ? 'up' : 'down');
    });
    
    // 更新买卖五档
    const sellLevels = document.getElementById('sellLevels');
    sellLevels.innerHTML = '';
    for (let i = 4; i >= 0; i--) {
        const level = quote.SellLevel[i];
        const price = parseFloat(level.Price) / 1000;
        const volume = Math.round(level.Number / 100);
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>卖${i + 1}</td>
            <td>${price.toFixed(2)}</td>
            <td>${volume}</td>
        `;
        sellLevels.appendChild(tr);
    }
    
    const buyLevels = document.getElementById('buyLevels');
    buyLevels.innerHTML = '';
    for (let i = 0; i < 5; i++) {
        const level = quote.BuyLevel[i];
        const price = parseFloat(level.Price) / 1000;
        const volume = Math.round(level.Number / 100);
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>买${i + 1}</td>
            <td>${price.toFixed(2)}</td>
            <td>${volume}</td>
        `;
        buyLevels.appendChild(tr);
    }
}

// 加载K线数据
async function loadKline(type, buttonElement) {
    if (!currentStock) return;
    
    showLoading();
    try {
        renderHQKLine('klineChart', currentStock, type, { count: 800, pageSize: 80 });
        
        // 更新按钮状态
        document.querySelectorAll('.btn-control').forEach(btn => {
            btn.classList.remove('active');
        });
        // 如果提供了按钮元素，添加active类
        if (buttonElement) {
            buttonElement.classList.add('active');
        } else {
            // 如果没有提供，根据type查找对应按钮
            const buttons = document.querySelectorAll('.btn-control');
            buttons.forEach(btn => {
                if (btn.textContent.includes(getKlineTypeName(type))) {
                    btn.classList.add('active');
                }
            });
        }
    } catch (error) {
        console.error('加载K线失败:', error);
    } finally {
        hideLoading();
    }
}

// 获取K线类型中文名称
function getKlineTypeName(type) {
    const typeMap = {
        'day': '日K',
        'week': '周K',
        'month': '月K',
        'minute30': '30分',
        'minute15': '15分',
        'minute5': '5分'
    };
    return typeMap[type] || '日K';
}

function getActiveKlineType() {
    const active = document.querySelector('.btn-control.active');
    const label = active?.textContent || '日K';
    if (label.includes('周')) return 'week';
    if (label.includes('月')) return 'month';
    if (label.includes('30')) return 'minute30';
    if (label.includes('15')) return 'minute15';
    if (label.includes('5')) return 'minute5';
    return 'day';
}

// 加载分时数据
async function loadMinute(code) {
    try {
        renderHQMinute('minuteChart', code);
    } catch (error) {
        console.error('加载分时数据失败:', error);
    }
}

// 加载分时成交
async function loadTrade(code) {
    try {
        const response = await fetch(`/api/trade?code=${code}`);
        const result = await response.json();
        
        if (result.code === 0 && result.data) {
            displayTrade(result.data);
        }
    } catch (error) {
        console.error('加载分时成交失败:', error);
    }
}

// 显示分时成交
function displayTrade(data) {
    if (!data.List || data.List.length === 0) {
        document.getElementById('tradeTableBody').innerHTML = '<tr><td colspan="5">暂无数据</td></tr>';
        return;
    }
    
    const tbody = document.getElementById('tradeTableBody');
    tbody.innerHTML = '';
    
    // 只显示最近200条
    const trades = data.List.slice(0, 200);
    
    trades.forEach(item => {
        const time = new Date(item.Time);
        const timeStr = String(time.getHours()).padStart(2, '0') + ':' + 
                       String(time.getMinutes()).padStart(2, '0') + ':' +
                       String(time.getSeconds()).padStart(2, '0');
        const price = (parseFloat(item.Price) / 1000).toFixed(2);
        const volume = item.Volume;
        const amount = formatAmount(parseFloat(item.Price) / 1000 * volume * 100);
        const status = item.Status === 0 ? '买入' : (item.Status === 1 ? '卖出' : '--');
        const statusClass = item.Status === 0 ? 'trade-buy' : (item.Status === 1 ? 'trade-sell' : '');
        
        const tr = document.createElement('tr');
        tr.innerHTML = `
            <td>${timeStr}</td>
            <td class="${statusClass}">${price}</td>
            <td>${volume}</td>
            <td>${amount}</td>
            <td class="${statusClass}">${status}</td>
        `;
        tbody.appendChild(tr);
    });
}

// 切换标签页
function switchTab(evt, tabName) {
    // 更新标签按钮状态
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    if (evt && evt.target) {
        evt.target.classList.add('active');
    }
    
    // 更新内容显示
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
    });
    const activeTab = document.getElementById(tabName + 'Tab');
    activeTab.classList.add('active');

    // 切换到图表时触发自适应，解决在隐藏容器中初始化导致的宽度问题
    requestAnimationFrame(() => {
        if (tabName === 'kline' && window.TDXHQChart) {
            setTimeout(() => {
                if (currentStock) renderHQKLine('klineChart', currentStock, getActiveKlineType(), { count: 800, pageSize: 80 });
                window.TDXHQChart.resize(document.getElementById('klineChart'));
            }, 50);
        }
        if (tabName === 'minute' && window.TDXHQChart) {
            setTimeout(() => {
                if (currentStock) renderHQMinute('minuteChart', currentStock);
                window.TDXHQChart.resize(document.getElementById('minuteChart'));
            }, 50);
        }
    });
}

// 监听旧版搜索框回车键；自选页不再渲染该输入框。
const stockCodeInput = document.getElementById('stockCode');
if (stockCodeInput) {
    stockCodeInput.addEventListener('keypress', function(e) {
        if (e.key === 'Enter') {
            searchStock();
        }
    });
}

// 窗口大小改变时重新渲染图表
let resizeTimer;
window.addEventListener('resize', function() {
    // 防抖优化，避免频繁调用
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(function() {
        if (window.TDXHQChart) window.TDXHQChart.resize();
    }, 100);
});

async function loadFormulaList() {
    formulas = await apiFetch('/api/formulas');
    const formulaOptions = formulas.map(f => `<option value="${f.id}">${escapeHTML(f.name)}</option>`).join('');
    const hqFormulaSelect = document.getElementById('hqFormulaSelect');
    if (hqFormulaSelect) {
        hqFormulaSelect.innerHTML = formulaOptions;
        hqFormulaSelect.onchange = renderHQFormulaArgsSummary;
        renderHQFormulaArgsSummary();
    }
    renderFormulaArgsEditor(parseHQFormulaArgs({ args_json: document.getElementById('formulaArgs')?.value || '[]' }));
    const automationFormula = document.getElementById('automationFormula');
    if (automationFormula) automationFormula.innerHTML = formulaOptions;
    const resultFilter = document.getElementById('resultFormulaFilter');
    if (resultFilter) {
        resultFilter.innerHTML = '<option value="">全部公式</option>' + formulaOptions;
    }
    document.getElementById('formulaList').innerHTML = formulas.map(f => `
        <div class="data-item formula-list-item">
            <div class="formula-list-head">
                <strong title="${escapeHTML(f.name)}">${escapeHTML(f.name)}</strong>
                <span>${f.enabled ? '启用' : '停用'}</span>
            </div>
            <div class="formula-list-meta">${escapeHTML(f.type)} · ${escapeHTML(f.period)} · 参数：${escapeHTML(formatFormulaArgs(f))}</div>
            <div class="formula-script-preview" title="${escapeHTML(f.script)}">${escapeHTML(compactFormulaScript(f.script))}</div>
            <div class="item-actions">
                <button onclick="fillFormula('${f.id}')">编辑</button>
                <button class="primary" onclick="quickTestFormula('${f.id}')">测试</button>
                <button onclick="deleteFormula('${f.id}')">删除</button>
            </div>
        </div>
    `).join('') || '<div class="data-item">暂无公式</div>';
}

function fillFormula(id) {
    const f = formulas.find(item => item.id === id);
    if (!f) return;
    document.getElementById('formulaName').dataset.id = f.id;
    document.getElementById('formulaName').value = f.name;
    document.getElementById('formulaType').value = f.type;
    document.getElementById('formulaPeriod').value = f.period;
    document.getElementById('formulaRight').value = String(f.right);
    document.getElementById('formulaScript').value = f.script;
    document.getElementById('formulaArgs').value = f.args_json || '[]';
    renderFormulaArgsEditor(parseHQFormulaArgs(f));
}

function formatFormulaArgs(formula) {
    const args = parseHQFormulaArgs(formula);
    return args.length ? args.map(item => `${item.Name}=${item.Value}`).join('，') : '无';
}

function compactFormulaScript(script) {
    return String(script || '').replace(/\s+/g, ' ').trim() || '暂无脚本';
}

function appendFormulaArgRow(arg = {}) {
    const editor = document.getElementById('formulaArgsEditor');
    if (!editor) return;
    const emptyNode = editor.querySelector('.formula-empty-note');
    if (emptyNode) emptyNode.remove();
    const row = document.createElement('div');
    row.className = 'formula-arg-row';
    row.innerHTML = `
        <input class="formula-arg-name" value="${escapeHTML(arg.Name || '')}" placeholder="参数名">
        <input class="formula-arg-value" value="${escapeHTML(arg.Value ?? '')}" placeholder="参数值">
        <button type="button" onclick="removeFormulaArgRow(this)">删除</button>
    `;
    editor.appendChild(row);
}

function renderFormulaArgsEditor(args = []) {
    const editor = document.getElementById('formulaArgsEditor');
    if (!editor) return;
    editor.innerHTML = '';
    if (!args.length) {
        editor.innerHTML = '<div class="formula-empty-note">暂无参数，可新增</div>';
        return;
    }
    args.forEach(arg => appendFormulaArgRow(arg));
}

function addFormulaArgRow() {
    appendFormulaArgRow();
}

function removeFormulaArgRow(target) {
    target?.closest('.formula-arg-row')?.remove();
    const editor = document.getElementById('formulaArgsEditor');
    if (editor && !editor.querySelector('.formula-arg-row')) {
        editor.innerHTML = '<div class="formula-empty-note">暂无参数，可新增</div>';
    }
    syncFormulaArgsJSON();
}

function getFormulaArgsFromEditor() {
    const editor = document.getElementById('formulaArgsEditor');
    if (!editor) return [];
    return Array.from(editor.querySelectorAll('.formula-arg-row')).map(row => ({
        Name: row.querySelector('.formula-arg-name')?.value.trim() || '',
        Value: normalizeHQArgValue(row.querySelector('.formula-arg-value')?.value || '')
    })).filter(item => item.Name && item.Value !== '');
}

function syncFormulaArgsJSON() {
    const value = JSON.stringify(getFormulaArgsFromEditor());
    const node = document.getElementById('formulaArgs');
    if (node) node.value = value;
    return value;
}

async function saveFormula() {
    try {
        const id = document.getElementById('formulaName').dataset.id || '';
        const argsJSON = syncFormulaArgsJSON();
        JSON.parse(argsJSON || '[]');
        await apiFetch(id ? `/api/formulas/${id}` : '/api/formulas', {
            method: id ? 'PUT' : 'POST',
            body: JSON.stringify({
                id,
                name: document.getElementById('formulaName').value,
                type: document.getElementById('formulaType').value,
                period: document.getElementById('formulaPeriod').value,
                right: Number(document.getElementById('formulaRight').value),
                script: document.getElementById('formulaScript').value,
                args_json: argsJSON || '[]',
                enabled: true
            })
        });
        document.getElementById('formulaName').dataset.id = '';
        await loadFormulaList();
        renderHQFormulaArgsSummary();
        alert('公式已保存');
    } catch (error) {
        alert(error.message);
    }
}

async function deleteFormula(id) {
    if (!confirm('确认删除这个公式？')) return;
    await apiFetch(`/api/formulas/${id}`, { method: 'DELETE' });
    await loadFormulaList();
}

async function quickTestFormula(id) {
    const symbol = currentStock || document.getElementById('hqSymbol').value || '';
    if (!symbol.trim()) {
        alert('请输入股票代码');
        return;
    }
    await runFormulaTest(id, symbol);
    switchWorkspace('proChart', document.querySelectorAll('.workspace-tab')[1]);
}

function editSelectedHQFormula() {
    const id = document.getElementById('hqFormulaSelect')?.value;
    if (!id) {
        alert('请先选择一个公式');
        return;
    }
    openFormulaDialog();
    fillFormula(id);
}

async function loadAIConfigData() {
    const list = document.getElementById('aiConfigList');
    if (list) list.innerHTML = '<div class="data-item">正在加载 AI 模型配置...</div>';
    try {
        const [providers, credentials] = await Promise.all([
            apiFetch('/api/ai/providers'),
            apiFetch('/api/ai/credentials')
        ]);
        aiProviders = providers || [];
        aiCredentials = credentials || [];
        renderAIProviderOptions();
        renderAICredentials();
    } catch (error) {
        if (list) list.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
        setAIConfigStatus(error.message || error, 'error');
    }
}

function renderAIProviderOptions() {
    const select = document.getElementById('aiConfigProvider');
    if (!select) return;
    const current = select.value || 'deepseek';
    select.innerHTML = aiProviders.map(provider => `
        <option value="${escapeHTML(provider.id)}">${escapeHTML(provider.name || provider.id)}</option>
    `).join('');
    if (aiProviders.some(provider => provider.id === current)) {
        select.value = current;
    }
    if (!select.value && aiProviders[0]) select.value = aiProviders[0].id;
    updateAIModelOptions(select.value);
}

function updateAIModelOptions(providerID) {
    const provider = aiProviders.find(item => item.id === providerID) || {};
    const baseURLInput = document.getElementById('aiConfigBaseURL');
    const modelInput = document.getElementById('aiConfigModel');
    const datalist = document.getElementById('aiConfigModelOptions');
    const models = provider.default_models || [];
    if (baseURLInput && !baseURLInput.value.trim() && provider.default_base_url) {
        baseURLInput.value = provider.default_base_url;
    }
    if (modelInput && !modelInput.value.trim() && models[0]) {
        modelInput.value = models[0].id;
    }
    if (datalist) {
        datalist.innerHTML = models.map(model => `
            <option value="${escapeHTML(model.id)}">${escapeHTML(model.name || model.id)}</option>
        `).join('');
    }
}

function onAIProviderChange() {
    const providerID = document.getElementById('aiConfigProvider')?.value || 'deepseek';
    const provider = aiProviders.find(item => item.id === providerID) || {};
    const nameInput = document.getElementById('aiConfigName');
    const baseURLInput = document.getElementById('aiConfigBaseURL');
    const modelInput = document.getElementById('aiConfigModel');
    if (baseURLInput) baseURLInput.value = provider.default_base_url || '';
    if (modelInput) modelInput.value = provider.default_models?.[0]?.id || '';
    if (nameInput && !nameInput.value.trim()) nameInput.value = provider.name || providerID;
    updateAIModelOptions(providerID);
}

function renderAICredentials() {
    const list = document.getElementById('aiConfigList');
    const summary = document.getElementById('aiConfigSummary');
    if (summary) {
        const enabledCount = aiCredentials.filter(item => item.enabled).length;
        summary.textContent = `${aiCredentials.length} 个配置 · ${enabledCount} 个启用`;
    }
    if (!list) return;
    list.innerHTML = aiCredentials.map(item => {
        const providerName = aiProviderName(item.provider);
        const model = item.model || defaultAIModel(item.provider);
        const sourceLabel = item.source === 'env' ? '<span class="tag warning">环境变量</span>' : '<span class="tag">本地保存</span>';
        const enabledLabel = item.enabled ? '<span class="tag success">启用</span>' : '<span class="tag muted">停用</span>';
        const editButton = item.source === 'env' ? '' : `<button onclick="editAIConfig('${escapeJSString(item.id)}')">编辑</button>`;
        const deleteButton = item.source === 'env' ? '' : `<button onclick="deleteAIConfig('${escapeJSString(item.id)}')">删除</button>`;
        return `
            <div class="data-item ai-config-item ${item.enabled ? '' : 'disabled'}">
                <div class="data-item-title">${escapeHTML(item.name || providerName)} ${sourceLabel} ${enabledLabel}</div>
                <div class="ai-config-meta-grid">
                    <span>供应商：${escapeHTML(providerName)}</span>
                    <span>模型：${escapeHTML(model)}</span>
                    <span>Key：${escapeHTML(item.api_key_masked || (item.has_api_key ? '已配置' : '未配置'))}</span>
                </div>
                <div class="data-item-meta">${escapeHTML(item.base_url || defaultAIBaseURL(item.provider) || '--')}</div>
                <div class="item-actions">
                    ${editButton}
                    <button class="primary" onclick="testStoredAIConfig('${escapeJSString(item.id)}')">测试</button>
                    ${deleteButton}
                </div>
            </div>
        `;
    }).join('') || '<div class="data-item">暂无 AI 配置。可以新建一条，或通过环境变量提供 API Key。</div>';
}

function aiProviderName(providerID) {
    return aiProviders.find(item => item.id === providerID)?.name || providerID || '--';
}

function defaultAIBaseURL(providerID) {
    return aiProviders.find(item => item.id === providerID)?.default_base_url || '';
}

function defaultAIModel(providerID) {
    return aiProviders.find(item => item.id === providerID)?.default_models?.[0]?.id || '';
}

function resetAIConfigForm() {
    const title = document.getElementById('aiConfigFormTitle');
    if (title) title.textContent = '添加 AI 模型';
    ['aiConfigID', 'aiConfigName', 'aiConfigBaseURL', 'aiConfigModel', 'aiConfigAPIKey', 'aiConfigAPISecret'].forEach(id => {
        const node = document.getElementById(id);
        if (node) node.value = '';
    });
    const extra = document.getElementById('aiConfigExtraJSON');
    if (extra) extra.value = '{}';
    const enabled = document.getElementById('aiConfigEnabled');
    if (enabled) enabled.checked = true;
    renderAIProviderOptions();
    setAIConfigStatus('新增配置后可以测试连接。');
}

async function editAIConfig(id) {
    try {
        const item = await apiFetch(`/api/ai/credentials/${encodeURIComponent(id)}`);
        const title = document.getElementById('aiConfigFormTitle');
        if (title) title.textContent = '编辑 AI 模型';
        document.getElementById('aiConfigID').value = item.id || '';
        document.getElementById('aiConfigName').value = item.name || '';
        document.getElementById('aiConfigProvider').value = item.provider || 'deepseek';
        document.getElementById('aiConfigBaseURL').value = item.base_url || defaultAIBaseURL(item.provider);
        document.getElementById('aiConfigModel').value = item.model || defaultAIModel(item.provider);
        document.getElementById('aiConfigAPIKey').value = '';
        document.getElementById('aiConfigAPISecret').value = '';
        document.getElementById('aiConfigExtraJSON').value = item.extra_json || '{}';
        document.getElementById('aiConfigEnabled').checked = item.enabled !== false;
        updateAIModelOptions(item.provider || 'deepseek');
        setAIConfigStatus('正在编辑：' + (item.name || item.id));
    } catch (error) {
        setAIConfigStatus(error.message || error, 'error');
    }
}

function aiConfigPayload(includeEmptySecrets = false) {
    const extraJSON = document.getElementById('aiConfigExtraJSON')?.value.trim() || '{}';
    JSON.parse(extraJSON);
    const payload = {
        id: document.getElementById('aiConfigID')?.value.trim() || '',
        name: document.getElementById('aiConfigName')?.value.trim() || '',
        provider: document.getElementById('aiConfigProvider')?.value || 'deepseek',
        base_url: document.getElementById('aiConfigBaseURL')?.value.trim() || '',
        model: document.getElementById('aiConfigModel')?.value.trim() || '',
        extra_json: extraJSON,
        enabled: !!document.getElementById('aiConfigEnabled')?.checked
    };
    const apiKey = document.getElementById('aiConfigAPIKey')?.value.trim() || '';
    const apiSecret = document.getElementById('aiConfigAPISecret')?.value.trim() || '';
    if (includeEmptySecrets || apiKey) payload.api_key = apiKey;
    if (includeEmptySecrets || apiSecret) payload.api_secret = apiSecret;
    if (!payload.name) throw new Error('请填写配置名称');
    if (!payload.provider) throw new Error('请选择供应商');
    if (!payload.model) payload.model = defaultAIModel(payload.provider);
    if (!payload.id && !payload.api_key) throw new Error('新建配置需要填写 API Key');
    return payload;
}

async function saveAIConfig() {
    try {
        const payload = aiConfigPayload(false);
        const url = payload.id ? `/api/ai/credentials/${encodeURIComponent(payload.id)}` : '/api/ai/credentials';
        const method = payload.id ? 'PUT' : 'POST';
        await apiFetch(url, { method, body: JSON.stringify(payload) });
        setAIConfigStatus('配置已保存。', 'success');
        resetAIConfigForm();
        await loadAIConfigData();
    } catch (error) {
        setAIConfigStatus(error.message || error, 'error');
    }
}

async function testAIConfig() {
    try {
        const payload = aiConfigPayload(false);
        if (!payload.id) throw new Error('请先保存配置，再测试连接');
        const data = await apiFetch(`/api/ai/credentials/${encodeURIComponent(payload.id)}/test`, {
            method: 'POST',
            body: JSON.stringify({ provider: payload.provider, model: payload.model })
        });
        renderAITestOutput(data);
        setAIConfigStatus('连接测试成功。', 'success');
    } catch (error) {
        renderAITestOutput({ ok: false, error: error.message || String(error) });
        setAIConfigStatus(error.message || error, 'error');
    }
}

async function testStoredAIConfig(id) {
    try {
        const item = aiCredentials.find(v => v.id === id) || {};
        const data = await apiFetch(`/api/ai/credentials/${encodeURIComponent(id)}/test`, {
            method: 'POST',
            body: JSON.stringify({ provider: item.provider, model: item.model || defaultAIModel(item.provider) })
        });
        renderAITestOutput(data);
        setAIConfigStatus('连接测试成功。', 'success');
    } catch (error) {
        renderAITestOutput({ ok: false, error: error.message || String(error) });
        setAIConfigStatus(error.message || error, 'error');
    }
}

async function deleteAIConfig(id) {
    if (!confirm('确认删除这个 AI 配置？')) return;
    try {
        await apiFetch(`/api/ai/credentials/${encodeURIComponent(id)}`, { method: 'DELETE' });
        setAIConfigStatus('配置已删除。', 'success');
        await loadAIConfigData();
    } catch (error) {
        setAIConfigStatus(error.message || error, 'error');
    }
}

function renderAITestOutput(data) {
    const node = document.getElementById('aiConfigTestOutput');
    if (node) node.textContent = prettyJSON(data);
}

function setAIConfigStatus(message, type = '') {
    const node = document.getElementById('aiConfigFormStatus');
    if (!node) return;
    node.textContent = message || '';
    node.className = `status-note ${type}`.trim();
}

function openFormulaDialog() {
    const dialog = document.getElementById('formulaDialog');
    if (!dialog) return;
    dialog.classList.add('open');
    dialog.setAttribute('aria-hidden', 'false');
    loadFormulaList();
}

function closeFormulaDialog() {
    const dialog = document.getElementById('formulaDialog');
    if (!dialog) return;
    dialog.classList.remove('open');
    dialog.setAttribute('aria-hidden', 'true');
}

async function testSelectedFormula() {
    const id = document.getElementById('hqFormulaSelect').value;
    const symbol = document.getElementById('hqSymbol').value || currentStock || '';
    if (!symbol.trim()) {
        alert('请输入股票代码');
        return;
    }
    await runFormulaTest(id, symbol);
}

async function runFormulaTest(id, symbol, options = {}) {
    try {
        const data = await apiFetch(`/api/formulas/${id}/test`, {
            method: 'POST',
            body: JSON.stringify({
                symbol,
                calc_count: 240,
                out_count: 5,
                ...(options.args ? { args: options.args } : {})
            })
        });
        document.getElementById('formulaTestOutput').textContent = prettyJSON(data);
    } catch (error) {
        document.getElementById('formulaTestOutput').textContent = error.message;
    }
}

function selectedHQFormula() {
    const id = document.getElementById('hqFormulaSelect')?.value;
    return formulas.find(item => item.id === id) || null;
}

function normalizeHQArgValue(value) {
    const text = String(value ?? '').trim();
    if (text === '') return '';
    const numberValue = Number(text);
    return Number.isFinite(numberValue) ? numberValue : text;
}

function normalizeHQFormulaArgs(args) {
    if (!Array.isArray(args)) return [];
    return args.map(item => {
        const name = item?.Name ?? item?.name ?? '';
        const value = item?.Value ?? item?.value ?? '';
        return { Name: String(name).trim(), Value: normalizeHQArgValue(value) };
    }).filter(item => item.Name && item.Value !== '');
}

function parseHQFormulaArgs(formula) {
    const raw = formula?.args_json ?? formula?.args ?? [];
    if (Array.isArray(raw)) return normalizeHQFormulaArgs(raw);
    if (!String(raw || '').trim()) return [];
    try {
        return normalizeHQFormulaArgs(JSON.parse(raw));
    } catch (error) {
        updateHQOverlayStatus(`参数JSON解析失败：${error.message}`);
        return [];
    }
}

function renderHQFormulaArgsSummary() {
    const node = document.getElementById('hqFormulaArgsSummary');
    if (!node) return;
    const args = parseHQFormulaArgs(selectedHQFormula());
    if (!args.length) {
        node.textContent = '公式参数：无';
        return;
    }
    node.textContent = `公式参数：${args.map(item => `${item.Name}=${item.Value}`).join('，')}`;
}

async function loadPools() {
    pools = await apiFetch('/api/stock-pools');
    const automationPool = document.getElementById('automationPool');
    if (automationPool) automationPool.innerHTML = customPools().map(p => `<option value="${p.id}">${escapeHTML(p.name)}</option>`).join('');
    const poolList = document.getElementById('poolList');
    if (!poolList) return;
    const market = marketPools();
    const custom = customPools();
    poolList.innerHTML = `
        ${market.length ? `<div class="data-list-title">系统市场分组</div>${market.map(renderPoolItem).join('')}` : ''}
        ${custom.length ? `<div class="data-list-title">自定义股票池</div>${custom.map(renderPoolItem).join('')}` : ''}
    ` || '<div class="data-item">暂无股票池</div>';
}

function marketPools() {
    return pools.filter(pool => pool.category === 'market' || String(pool.id || '').startsWith('market-'));
}

function customPools() {
    return pools.filter(pool => !(pool.category === 'market' || String(pool.id || '').startsWith('market-')));
}

function renderPoolItem(p) {
    const readonly = p.readonly || p.category === 'market';
    return `
        <div class="data-item">
            <div class="data-item-title">${escapeHTML(p.name)}${readonly ? ' <span class="tag">系统分组</span>' : ''}</div>
            <div class="data-item-meta">${(p.symbols || []).length} 只股票 · ${escapeHTML((p.symbols || []).slice(0, 24).join(', '))}${(p.symbols || []).length > 24 ? ' ...' : ''}</div>
            ${p.description ? `<div class="data-item-meta">${escapeHTML(p.description)}</div>` : ''}
            ${readonly ? '' : `<div class="item-actions">
                <button onclick="fillPool('${p.id}')">编辑</button>
                <button onclick="deletePool('${p.id}')">删除</button>
            </div>`}
        </div>
    `;
}

function fillPool(id) {
    const p = pools.find(item => item.id === id);
    if (!p) return;
    document.getElementById('poolName').dataset.id = p.id;
    document.getElementById('poolName').value = p.name;
    document.getElementById('poolSymbols').value = p.symbols.join('\n');
}

async function savePool() {
    try {
        const id = document.getElementById('poolName').dataset.id || '';
        const symbols = document.getElementById('poolSymbols').value.split(/[\s,，]+/).filter(Boolean);
        await apiFetch(id ? `/api/stock-pools/${id}` : '/api/stock-pools', {
            method: id ? 'PUT' : 'POST',
            body: JSON.stringify({ id, name: document.getElementById('poolName').value, symbols })
        });
        document.getElementById('poolName').dataset.id = '';
        await loadPools();
        alert('股票池已保存');
    } catch (error) {
        alert(error.message);
    }
}

async function deletePool(id) {
    if (!confirm('确认删除这个股票池？')) return;
    await apiFetch(`/api/stock-pools/${id}`, { method: 'DELETE' });
    await loadPools();
}

async function loadStrategyCenter() {
    await Promise.all([loadStrategies(), loadFactors(), loadFormulaList(), loadPools()]);
    if (!selectedStrategyID && strategies.length) {
        fillStrategy(strategies[0].id);
    }
}

async function loadStrategies() {
    strategies = await apiFetch('/api/strategies') || [];
    renderStrategyList();
    const select = document.getElementById('automationStrategy');
    if (select) {
        select.innerHTML = strategies.map(item => `<option value="${item.id}">${escapeHTML(item.name)}${item.readonly ? '（模板）' : ''}</option>`).join('');
    }
}

async function loadFactors() {
    factors = await apiFetch('/api/factors') || [];
    const list = document.getElementById('factorList');
    if (!list) return;
    list.innerHTML = factors.map(factor => `
        <div class="data-item factor-item">
            <div class="data-item-title">${escapeHTML(factor.name)} <span class="tag">${escapeHTML(factor.kind)}</span></div>
            <div class="data-item-meta">${escapeHTML(factor.id)} · ${escapeHTML(factor.description)}</div>
            <div class="factor-param-list">${(factor.params || []).map(p => `<span>${escapeHTML(p.label)}=${escapeHTML(compactValue(p.default))}</span>`).join('')}</div>
            <div class="data-item-meta">${escapeHTML(strategyFactorEditHint(factor.id))}</div>
        </div>
    `).join('') || '<div class="data-item">暂无因子</div>';
}

function strategyFactorEditHint(factorID) {
    const map = {
        pool_exclude: '在“硬过滤条件 / 排除股票池”里勾选并选择股票池。',
        min_amount: '在“硬过滤条件 / 最低成交额”里勾选并输入金额。',
        price_range: '在“硬过滤条件 / 收盘价区间”里勾选并输入最低价、最高价。',
        change_range: '在“硬过滤条件 / 涨跌幅区间”里勾选并输入百分比范围。',
        ma_trend: '在“评分条件 / 均线多头”里勾选并设置均线和权重。',
        volume_up: '在“评分条件 / 阶段放量”里勾选并设置天数、倍数和权重。',
        break_high: '在“评分条件 / 突破新高”里勾选并设置回看天数和权重。',
        macd_golden_cross: '在“评分条件 / MACD金叉”里勾选并设置快慢线参数。',
        macd_dead_cross: '在“评分条件 / MACD死叉”里勾选并设置快慢线参数。',
        kdj_golden_cross: '在“评分条件 / KDJ金叉”里勾选并设置平滑参数。',
        rsi_oversold: '在“评分条件 / RSI超卖”里勾选并设置周期和阈值。',
        boll_breakout: '在“评分条件 / BOLL突破”里勾选并设置周期和标准差倍数。',
        volume_breakout: '在“评分条件 / 放量突破”里勾选并设置高点回看、量比和涨幅。',
        local_rocket: '在“评分条件 / 本地火箭发射”里勾选并调整启动参数。',
        formula: '在“评分条件 / 公式因子”里勾选并选择公式。'
    };
    return map[factorID] || '在策略编辑区勾选并调整参数。';
}

function renderStrategyList() {
    const list = document.getElementById('strategyList');
    if (!list) return;
    list.innerHTML = strategies.map(item => {
        const cfg = parseStrategyConfig(item.config_json);
        const filterCount = Array.isArray(cfg.filters) ? cfg.filters.length : 0;
        const scoreCount = Array.isArray(cfg.scores) ? cfg.scores.length : 0;
        const selected = item.id === selectedStrategyID ? ' selected' : '';
        return `
            <div class="data-item strategy-item${selected}" onclick="fillStrategy('${escapeJSString(item.id)}')">
                <div class="data-item-title">${escapeHTML(item.name)}${item.readonly ? ' <span class="tag">内置模板</span>' : ''}</div>
                <div class="data-item-meta">${escapeHTML(item.description || '暂无说明')}</div>
                <div class="data-item-meta">${escapeHTML(strategyUniverseLabel(cfg))} · 过滤 ${filterCount} · 评分 ${scoreCount} · ${item.enabled ? '启用' : '停用'}</div>
                ${renderStrategyRunStatus(item.id)}
            </div>
        `;
    }).join('') || '<div class="data-item">暂无策略</div>';
}

function renderStrategyRunStatus(strategyID) {
    if (!strategyRunState.running || strategyRunState.strategyID !== strategyID) {
        return '<div class="strategy-run-mini idle">空闲</div>';
    }
    return `
        <div class="strategy-run-mini running">
            <span>${escapeHTML(strategyRunState.message || '运行中')}</span>
            <strong>${strategyRunState.progress}%</strong>
            <div class="mini-progress"><i style="width:${strategyRunState.progress}%"></i></div>
        </div>
    `;
}

function parseStrategyConfig(raw) {
    if (typeof raw === 'object' && raw) return raw;
    try {
        return JSON.parse(raw || '{}');
    } catch {
        return {};
    }
}

function strategyUniverseLabel(cfg = {}) {
    const universe = cfg.universe || 'pool';
    if (universe === 'symbols') return `自定义标的 ${(cfg.symbols || []).length} 只`;
    if (universe === 'all' || universe === 'all_a') return '全市场A股';
    if (universe === 'market') {
        const pool = pools.find(item => item.id === cfg.pool_id);
        return `市场分组 ${pool?.name || cfg.pool_id || '全部A股'}`;
    }
    return `股票池 ${cfg.pool_id || 'watchlist'}`;
}

function defaultStrategyConfig() {
    return {
        universe: 'pool',
        pool_id: 'watchlist',
        period: 'day',
        calc_count: 260,
        batch_size: 50,
        continue_on_error: true,
        filters: [
            { id: 'exclude_pool', factor: 'pool_exclude', params: { pool_id: 'exclude' } }
        ],
        scores: [
            { id: 'ma_trend', factor: 'ma_trend', weight: 20, params: { short: 5, mid: 10, long: 20 } }
        ],
        pass: { min_score: 20, top_n: 50 }
    };
}

function currentStrategyBacktestPayload() {
    return {
        start_date: document.getElementById('strategyBacktestStart')?.value || '',
        end_date: document.getElementById('strategyBacktestEnd')?.value || '',
        history_count: numberFromInput('strategyBacktestHistoryCount', 520),
        symbol_limit: numberFromInput('strategyBacktestSymbolLimit', 80),
        initial_cash: numberFromInput('strategyBacktestInitialCash', 100000),
        stop_loss: numberFromInput('strategyBacktestStopLoss', 8) / 100,
        profit_trigger: numberFromInput('strategyBacktestProfitTrigger', 12) / 100,
        trailing_stop: numberFromInput('strategyBacktestTrailingStop', 10) / 100,
        max_hold: numberFromInput('strategyBacktestMaxHold', 40),
        exit_ma: numberFromInput('strategyBacktestExitMA', 20),
        buy_cost: numberFromInput('strategyBacktestBuyCost', 0.05) / 100,
        sell_cost: numberFromInput('strategyBacktestSellCost', 0.10) / 100
    };
}

function strategyRule(cfg, factor) {
    return [...(cfg.filters || []), ...(cfg.scores || [])].find(rule => rule.factor === factor) || null;
}

function poolOptionsHTML(selectedID) {
    return pools.map(pool => `<option value="${escapeHTML(pool.id)}" ${pool.id === selectedID ? 'selected' : ''}>${escapeHTML(pool.name)}（${(pool.symbols || []).length}只）</option>`).join('');
}

function marketPoolOptionsHTML(selectedID) {
    const items = marketPools();
    return items.map(pool => `<option value="${escapeHTML(pool.id)}" ${pool.id === selectedID ? 'selected' : ''}>${escapeHTML(pool.name)}（${(pool.symbols || []).length}只）</option>`).join('');
}

function customPoolOptionsHTML(selectedID) {
    const items = customPools();
    return items.map(pool => `<option value="${escapeHTML(pool.id)}" ${pool.id === selectedID ? 'selected' : ''}>${escapeHTML(pool.name)}（${(pool.symbols || []).length}只）</option>`).join('');
}

function poolPreviewHTML(poolID) {
    const pool = pools.find(item => item.id === poolID);
    if (!pool) return '<div class="strategy-pool-preview">未找到这个股票池</div>';
    const symbols = pool.symbols || [];
    return `<div class="strategy-pool-preview">${escapeHTML(pool.name)}：${symbols.length ? escapeHTML(symbols.slice(0, 18).join(', ')) : '暂无股票'}${symbols.length > 18 ? ' ...' : ''}</div>`;
}

function renderStrategyVisualEditor(cfg = {}) {
    const box = document.getElementById('strategyVisualEditor');
    if (!box) return;
    const exclude = strategyRule(cfg, 'pool_exclude');
    const minAmount = strategyRule(cfg, 'min_amount');
    const priceRange = strategyRule(cfg, 'price_range');
    const changeRange = strategyRule(cfg, 'change_range');
    const maTrend = strategyRule(cfg, 'ma_trend');
    const volumeUp = strategyRule(cfg, 'volume_up');
    const breakHigh = strategyRule(cfg, 'break_high');
    const macdGolden = strategyRule(cfg, 'macd_golden_cross');
    const macdDead = strategyRule(cfg, 'macd_dead_cross');
    const kdjGolden = strategyRule(cfg, 'kdj_golden_cross');
    const rsiOversold = strategyRule(cfg, 'rsi_oversold');
    const bollBreakout = strategyRule(cfg, 'boll_breakout');
    const volumeBreakout = strategyRule(cfg, 'volume_breakout');
    const localRocket = strategyRule(cfg, 'local_rocket');
    const formulaRule = strategyRule(cfg, 'formula');
    const universe = cfg.universe || 'pool';
    const poolID = cfg.pool_id || (universe === 'market' ? 'market-all-a' : 'watchlist');
    const excludePoolID = exclude?.params?.pool_id || 'exclude';
    const formulaID = formulaRule?.params?.formula_id || '';
    const formulaName = formulaRule?.params?.formula_name || '';
    const selectedFormula = formulas.find(item => item.id === formulaID || item.name === formulaName);

    box.innerHTML = `
        <div class="strategy-section">
            <div class="strategy-section-head">
                <strong>选股范围</strong>
                <span>先决定从哪些股票里选</span>
            </div>
            <div class="strategy-field-grid">
                <label>范围类型
                    <select id="strategyUniverse" onchange="handleStrategyUniverseChange()">
                        <option value="pool" ${universe === 'pool' || universe === '' ? 'selected' : ''}>股票池</option>
                        <option value="market" ${universe === 'market' ? 'selected' : ''}>市场分组</option>
                        <option value="symbols" ${universe === 'symbols' ? 'selected' : ''}>手动输入股票</option>
                        <option value="all_a" ${universe === 'all_a' || universe === 'all' ? 'selected' : ''}>全市场A股</option>
                    </select>
                </label>
                <label id="strategyPoolField">自定义股票池
                    <select id="strategyPoolID" onchange="updateStrategyPoolPreviews()">${customPoolOptionsHTML(poolID)}</select>
                </label>
                <label id="strategyMarketField">市场分组
                    <select id="strategyMarketPoolID" onchange="updateStrategyPoolPreviews()">${marketPoolOptionsHTML(poolID)}</select>
                </label>
            </div>
            <textarea id="strategySymbolsInput" rows="3" placeholder="手动输入股票代码，多个代码分行填写">${escapeHTML((cfg.symbols || []).join('\n'))}</textarea>
            <div id="strategyUniversePreview">${poolPreviewHTML(poolID)}</div>
        </div>

        <div class="strategy-section">
            <div class="strategy-section-head">
                <strong>硬过滤条件</strong>
                <span>不满足这些条件的股票直接淘汰</span>
            </div>
            ${renderStrategyCheckRow('filterExcludePool', '排除股票池', !!exclude, `
                <label>排除哪个池
                    <select id="filterExcludePoolID" onchange="updateStrategyPoolPreviews()">${poolOptionsHTML(excludePoolID)}</select>
                </label>
                <div id="strategyExcludePreview">${poolPreviewHTML(excludePoolID)}</div>
            `)}
            ${renderStrategyCheckRow('filterMinAmount', '最低成交额', !!minAmount, `
                <label>成交额不低于
                    <input id="filterMinAmountValue" type="number" value="${Number(minAmount?.params?.value ?? 100000000)}" min="0" step="1000000">
                </label>
            `)}
            ${renderStrategyCheckRow('filterPriceRange', '收盘价区间', !!priceRange, `
                <label>最低价<input id="filterPriceMin" type="number" value="${Number(priceRange?.params?.min ?? 0)}" min="0" step="0.01"></label>
                <label>最高价<input id="filterPriceMax" type="number" value="${Number(priceRange?.params?.max ?? 9999)}" min="0" step="0.01"></label>
            `)}
            ${renderStrategyCheckRow('filterChangeRange', '涨跌幅区间', !!changeRange, `
                <label>最小涨跌幅 %<input id="filterChangeMin" type="number" value="${Number(changeRange?.params?.min ?? -10)}" step="0.1"></label>
                <label>最大涨跌幅 %<input id="filterChangeMax" type="number" value="${Number(changeRange?.params?.max ?? 10)}" step="0.1"></label>
            `)}
        </div>

        <div class="strategy-section">
            <div class="strategy-section-head">
                <strong>评分条件</strong>
                <span>命中条件后按权重加分</span>
            </div>
            ${renderStrategyCheckRow('scoreMaTrend', '均线多头', !!maTrend, `
                ${weightInput('scoreMaTrendWeight', maTrend?.weight ?? 20)}
                <label>短均线<input id="scoreMaShort" type="number" value="${Number(maTrend?.params?.short ?? 5)}" min="1"></label>
                <label>中均线<input id="scoreMaMid" type="number" value="${Number(maTrend?.params?.mid ?? 10)}" min="1"></label>
                <label>长均线<input id="scoreMaLong" type="number" value="${Number(maTrend?.params?.long ?? 20)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreVolumeUp', '阶段放量', !!volumeUp, `
                ${weightInput('scoreVolumeWeight', volumeUp?.weight ?? 15)}
                <label>对比天数<input id="scoreVolumeDays" type="number" value="${Number(volumeUp?.params?.days ?? 5)}" min="1"></label>
                <label>放量倍数<input id="scoreVolumeRatio" type="number" value="${Number(volumeUp?.params?.ratio ?? 1.3)}" min="0" step="0.1"></label>
            `)}
            ${renderStrategyCheckRow('scoreBreakHigh', '突破新高', !!breakHigh, `
                ${weightInput('scoreBreakWeight', breakHigh?.weight ?? 15)}
                <label>回看天数<input id="scoreBreakDays" type="number" value="${Number(breakHigh?.params?.days ?? 20)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreMACDGolden', 'MACD金叉', !!macdGolden, `
                ${weightInput('scoreMACDGoldenWeight', macdGolden?.weight ?? 20)}
                <label>快线<input id="scoreMACDFast" type="number" value="${Number(macdGolden?.params?.fast ?? 12)}" min="1"></label>
                <label>慢线<input id="scoreMACDSlow" type="number" value="${Number(macdGolden?.params?.slow ?? 26)}" min="2"></label>
                <label>信号<input id="scoreMACDSignal" type="number" value="${Number(macdGolden?.params?.signal ?? 9)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreKDJGolden', 'KDJ金叉', !!kdjGolden, `
                ${weightInput('scoreKDJWeight', kdjGolden?.weight ?? 15)}
                <label>RSV周期<input id="scoreKDJN" type="number" value="${Number(kdjGolden?.params?.n ?? 9)}" min="1"></label>
                <label>K平滑<input id="scoreKDJK" type="number" value="${Number(kdjGolden?.params?.k ?? 3)}" min="1"></label>
                <label>D平滑<input id="scoreKDJD" type="number" value="${Number(kdjGolden?.params?.d ?? 3)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreRSIOversold', 'RSI超卖', !!rsiOversold, `
                ${weightInput('scoreRSIWeight', rsiOversold?.weight ?? 10)}
                <label>周期<input id="scoreRSIPeriod" type="number" value="${Number(rsiOversold?.params?.period ?? 6)}" min="1"></label>
                <label>阈值<input id="scoreRSIThreshold" type="number" value="${Number(rsiOversold?.params?.threshold ?? 30)}" min="1" max="100"></label>
            `)}
            ${renderStrategyCheckRow('scoreBOLLBreakout', 'BOLL突破', !!bollBreakout, `
                ${weightInput('scoreBOLLWeight', bollBreakout?.weight ?? 15)}
                <label>周期<input id="scoreBOLLPeriod" type="number" value="${Number(bollBreakout?.params?.period ?? 20)}" min="2"></label>
                <label>标准差倍数<input id="scoreBOLLWidth" type="number" value="${Number(bollBreakout?.params?.width ?? 2)}" min="0.1" step="0.1"></label>
            `)}
            ${renderStrategyCheckRow('scoreVolumeBreakout', '放量突破', !!volumeBreakout, `
                ${weightInput('scoreVolumeBreakoutWeight', volumeBreakout?.weight ?? 20)}
                <label>回看天数<input id="scoreVolumeBreakoutDays" type="number" value="${Number(volumeBreakout?.params?.days ?? 20)}" min="1"></label>
                <label>放量倍数<input id="scoreVolumeBreakoutRatio" type="number" value="${Number(volumeBreakout?.params?.ratio ?? 1.5)}" min="0" step="0.1"></label>
                <label>最低涨幅 %<input id="scoreVolumeBreakoutChange" type="number" value="${Number(volumeBreakout?.params?.min_change ?? 2)}" step="0.1"></label>
            `)}
            ${renderStrategyCheckRow('scoreLocalRocket', '本地火箭发射', !!localRocket, `
                ${weightInput('scoreLocalRocketWeight', localRocket?.weight ?? 30)}
                <label>突破回看<input id="scoreLocalRocketLookback" type="number" value="${Number(localRocket?.params?.lookback ?? 20)}" min="1"></label>
                <label>均量天数<input id="scoreLocalRocketVolumeDays" type="number" value="${Number(localRocket?.params?.volume_days ?? 5)}" min="1"></label>
                <label>放量倍数<input id="scoreLocalRocketVolumeRatio" type="number" value="${Number(localRocket?.params?.volume_ratio ?? 1.8)}" min="0" step="0.1"></label>
                <label>最低涨幅 %<input id="scoreLocalRocketChange" type="number" value="${Number(localRocket?.params?.min_change ?? 3)}" step="0.1"></label>
                <label>短均线<input id="scoreLocalRocketShortMA" type="number" value="${Number(localRocket?.params?.short_ma ?? 5)}" min="1"></label>
                <label>中均线<input id="scoreLocalRocketMidMA" type="number" value="${Number(localRocket?.params?.mid_ma ?? 10)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreMACDDead', 'MACD死叉', !!macdDead, `
                ${weightInput('scoreMACDDeadWeight', macdDead?.weight ?? 10)}
                <label>快线<input id="scoreMACDDeadFast" type="number" value="${Number(macdDead?.params?.fast ?? 12)}" min="1"></label>
                <label>慢线<input id="scoreMACDDeadSlow" type="number" value="${Number(macdDead?.params?.slow ?? 26)}" min="2"></label>
                <label>信号<input id="scoreMACDDeadSignal" type="number" value="${Number(macdDead?.params?.signal ?? 9)}" min="1"></label>
            `)}
            ${renderStrategyCheckRow('scoreFormula', '公式因子', !!formulaRule, `
                ${weightInput('scoreFormulaWeight', formulaRule?.weight ?? 30)}
                <label>选择公式
                    <select id="scoreFormulaID">
                        <option value="">按公式名称匹配：${escapeHTML(formulaName || '未选择')}</option>
                        ${formulas.map(item => `<option value="${item.id}" ${selectedFormula?.id === item.id ? 'selected' : ''}>${escapeHTML(item.name)}</option>`).join('')}
                    </select>
                </label>
            `)}
        </div>

        <div class="strategy-section">
            <div class="strategy-section-head">
                <strong>通过规则</strong>
                <span>最后按总分筛选</span>
            </div>
            <div class="strategy-field-grid">
                <label>最低分
                    <input id="strategyMinScore" type="number" value="${Number(cfg.pass?.min_score ?? 60)}" min="0" step="1">
                </label>
                <label>最多保留
                    <input id="strategyTopN" type="number" value="${Number(cfg.pass?.top_n ?? 50)}" min="1" step="1">
                </label>
                <label>计算K线数量
                    <input id="strategyCalcCount" type="number" value="${Number(cfg.calc_count ?? 260)}" min="30" step="10">
                </label>
            </div>
        </div>
    `;
    handleStrategyUniverseChange();
}

function renderStrategyCheckRow(id, title, checked, body) {
    return `
        <div class="strategy-rule-row">
            <label class="check-line strategy-rule-toggle"><input id="${id}" type="checkbox" ${checked ? 'checked' : ''}> ${title}</label>
            <div class="strategy-rule-body">${body}</div>
        </div>
    `;
}

function weightInput(id, value) {
    return `<label>权重<input id="${id}" type="number" value="${Number(value)}" min="0" step="1"></label>`;
}

function handleStrategyUniverseChange() {
    const universe = document.getElementById('strategyUniverse')?.value || 'pool';
    const poolField = document.getElementById('strategyPoolField');
    const marketField = document.getElementById('strategyMarketField');
    const symbols = document.getElementById('strategySymbolsInput');
    if (poolField) poolField.style.display = universe === 'pool' ? 'block' : 'none';
    if (marketField) marketField.style.display = universe === 'market' ? 'block' : 'none';
    if (symbols) symbols.style.display = universe === 'symbols' ? 'block' : 'none';
    updateStrategyPoolPreviews();
}

function updateStrategyPoolPreviews() {
    const universePreview = document.getElementById('strategyUniversePreview');
    const excludePreview = document.getElementById('strategyExcludePreview');
    const universe = document.getElementById('strategyUniverse')?.value || 'pool';
    const poolID = document.getElementById('strategyPoolID')?.value || 'watchlist';
    const marketPoolID = document.getElementById('strategyMarketPoolID')?.value || 'market-all-a';
    const excludePoolID = document.getElementById('filterExcludePoolID')?.value || 'exclude';
    if (universePreview) {
        if (universe === 'symbols') {
            const symbols = (document.getElementById('strategySymbolsInput')?.value || '').split(/[\s,，]+/).filter(Boolean);
            universePreview.innerHTML = `<div class="strategy-pool-preview">手动输入：${symbols.length} 只股票${symbols.length ? `，${escapeHTML(symbols.slice(0, 18).join(', '))}` : ''}</div>`;
        } else if (universe === 'market') {
            universePreview.innerHTML = poolPreviewHTML(marketPoolID);
        } else if (universe === 'all_a') {
            universePreview.innerHTML = '<div class="strategy-pool-preview">全市场A股：首版运行时会限制部分代码，避免一次任务过慢。</div>';
        } else {
            universePreview.innerHTML = poolPreviewHTML(poolID);
        }
    }
    if (excludePreview) excludePreview.innerHTML = poolPreviewHTML(excludePoolID);
}

function renderStrategyVisualFromJSON() {
    try {
        renderStrategyVisualEditor(JSON.parse(document.getElementById('strategyConfig').value || '{}'));
    } catch (error) {
        alert('配置不是有效JSON：' + error.message);
    }
}

function syncStrategyFormToJSON() {
    const cfg = collectStrategyVisualConfig();
    document.getElementById('strategyConfig').value = prettyJSON(cfg);
    return cfg;
}

function collectStrategyVisualConfig() {
    const universe = document.getElementById('strategyUniverse')?.value || 'pool';
    const cfg = {
        universe,
        period: 'day',
        calc_count: numberFromInput('strategyCalcCount', 260),
        batch_size: 50,
        continue_on_error: true,
        filters: [],
        scores: [],
        pass: {
            min_score: numberFromInput('strategyMinScore', 60),
            top_n: numberFromInput('strategyTopN', 50)
        }
    };
    if (universe === 'pool') {
        cfg.pool_id = document.getElementById('strategyPoolID')?.value || 'watchlist';
    }
    if (universe === 'market') {
        cfg.pool_id = document.getElementById('strategyMarketPoolID')?.value || 'market-all-a';
    }
    if (universe === 'symbols') {
        cfg.symbols = (document.getElementById('strategySymbolsInput')?.value || '').split(/[\s,，]+/).filter(Boolean);
    }
    if (checked('filterExcludePool')) {
        cfg.filters.push({ id: 'exclude_pool', factor: 'pool_exclude', params: { pool_id: document.getElementById('filterExcludePoolID')?.value || 'exclude' } });
    }
    if (checked('filterMinAmount')) {
        cfg.filters.push({ id: 'min_amount', factor: 'min_amount', params: { value: numberFromInput('filterMinAmountValue', 100000000) } });
    }
    if (checked('filterPriceRange')) {
        cfg.filters.push({ id: 'price_range', factor: 'price_range', params: { min: numberFromInput('filterPriceMin', 0), max: numberFromInput('filterPriceMax', 9999) } });
    }
    if (checked('filterChangeRange')) {
        cfg.filters.push({ id: 'change_range', factor: 'change_range', params: { min: numberFromInput('filterChangeMin', -10), max: numberFromInput('filterChangeMax', 10) } });
    }
    if (checked('scoreMaTrend')) {
        cfg.scores.push({ id: 'ma_trend', factor: 'ma_trend', weight: numberFromInput('scoreMaTrendWeight', 20), params: { short: numberFromInput('scoreMaShort', 5), mid: numberFromInput('scoreMaMid', 10), long: numberFromInput('scoreMaLong', 20) } });
    }
    if (checked('scoreVolumeUp')) {
        cfg.scores.push({ id: 'volume_up', factor: 'volume_up', weight: numberFromInput('scoreVolumeWeight', 15), params: { days: numberFromInput('scoreVolumeDays', 5), ratio: numberFromInput('scoreVolumeRatio', 1.3) } });
    }
    if (checked('scoreBreakHigh')) {
        cfg.scores.push({ id: 'break_high', factor: 'break_high', weight: numberFromInput('scoreBreakWeight', 15), params: { days: numberFromInput('scoreBreakDays', 20) } });
    }
    if (checked('scoreMACDGolden')) {
        cfg.scores.push({ id: 'macd_golden_cross', factor: 'macd_golden_cross', weight: numberFromInput('scoreMACDGoldenWeight', 20), params: { fast: numberFromInput('scoreMACDFast', 12), slow: numberFromInput('scoreMACDSlow', 26), signal: numberFromInput('scoreMACDSignal', 9) } });
    }
    if (checked('scoreKDJGolden')) {
        cfg.scores.push({ id: 'kdj_golden_cross', factor: 'kdj_golden_cross', weight: numberFromInput('scoreKDJWeight', 15), params: { n: numberFromInput('scoreKDJN', 9), k: numberFromInput('scoreKDJK', 3), d: numberFromInput('scoreKDJD', 3) } });
    }
    if (checked('scoreRSIOversold')) {
        cfg.scores.push({ id: 'rsi_oversold', factor: 'rsi_oversold', weight: numberFromInput('scoreRSIWeight', 10), params: { period: numberFromInput('scoreRSIPeriod', 6), threshold: numberFromInput('scoreRSIThreshold', 30) } });
    }
    if (checked('scoreBOLLBreakout')) {
        cfg.scores.push({ id: 'boll_breakout', factor: 'boll_breakout', weight: numberFromInput('scoreBOLLWeight', 15), params: { period: numberFromInput('scoreBOLLPeriod', 20), width: numberFromInput('scoreBOLLWidth', 2) } });
    }
    if (checked('scoreVolumeBreakout')) {
        cfg.scores.push({ id: 'volume_breakout', factor: 'volume_breakout', weight: numberFromInput('scoreVolumeBreakoutWeight', 20), params: { days: numberFromInput('scoreVolumeBreakoutDays', 20), ratio: numberFromInput('scoreVolumeBreakoutRatio', 1.5), min_change: numberFromInput('scoreVolumeBreakoutChange', 2) } });
    }
    if (checked('scoreLocalRocket')) {
        cfg.scores.push({ id: 'local_rocket', factor: 'local_rocket', weight: numberFromInput('scoreLocalRocketWeight', 30), params: { lookback: numberFromInput('scoreLocalRocketLookback', 20), volume_days: numberFromInput('scoreLocalRocketVolumeDays', 5), volume_ratio: numberFromInput('scoreLocalRocketVolumeRatio', 1.8), min_change: numberFromInput('scoreLocalRocketChange', 3), short_ma: numberFromInput('scoreLocalRocketShortMA', 5), mid_ma: numberFromInput('scoreLocalRocketMidMA', 10) } });
    }
    if (checked('scoreMACDDead')) {
        cfg.scores.push({ id: 'macd_dead_cross', factor: 'macd_dead_cross', weight: numberFromInput('scoreMACDDeadWeight', 10), params: { fast: numberFromInput('scoreMACDDeadFast', 12), slow: numberFromInput('scoreMACDDeadSlow', 26), signal: numberFromInput('scoreMACDDeadSignal', 9) } });
    }
    if (checked('scoreFormula')) {
        const formulaID = document.getElementById('scoreFormulaID')?.value || '';
        const formula = formulas.find(item => item.id === formulaID);
        cfg.scores.push({ id: 'main_formula', factor: 'formula', weight: numberFromInput('scoreFormulaWeight', 30), params: formulaID ? { formula_id: formulaID } : { formula_name: formula?.name || '主力拉升' } });
    }
    return cfg;
}

function checked(id) {
    return !!document.getElementById(id)?.checked;
}

function numberFromInput(id, fallback) {
    const value = Number(document.getElementById(id)?.value);
    return Number.isFinite(value) ? value : fallback;
}

function newStrategy() {
    if (strategyRunState.running) return;
    selectedStrategyID = '';
    document.getElementById('strategyName').dataset.id = '';
    document.getElementById('strategyName').dataset.readonly = 'false';
    document.getElementById('strategyName').value = '我的选股策略';
    document.getElementById('strategyDescription').value = '';
    document.getElementById('strategyEnabled').checked = true;
    document.getElementById('strategyConfig').value = prettyJSON(defaultStrategyConfig());
    renderStrategyVisualEditor(defaultStrategyConfig());
    document.getElementById('strategyEditorHint').textContent = '新策略保存后可用于自动化任务';
    renderStrategyList();
}

function fillStrategy(id) {
    if (strategyRunState.running) return;
    const item = strategies.find(v => v.id === id);
    if (!item) return;
    selectedStrategyID = id;
    document.getElementById('strategyName').dataset.id = item.id;
    document.getElementById('strategyName').dataset.readonly = item.readonly ? 'true' : 'false';
    document.getElementById('strategyName').value = item.name || '';
    document.getElementById('strategyDescription').value = item.description || '';
    document.getElementById('strategyEnabled').checked = !!item.enabled;
    const cfg = parseStrategyConfig(item.config_json);
    document.getElementById('strategyConfig').value = prettyJSON(cfg);
    renderStrategyVisualEditor(cfg);
    document.getElementById('strategyEditorHint').textContent = item.readonly ? '内置模板只读，请复制后修改' : '可保存后在自动化任务中选择';
    renderStrategyList();
}

function currentStrategyPayload() {
    const id = document.getElementById('strategyName').dataset.id || '';
    const readonly = document.getElementById('strategyName').dataset.readonly === 'true';
    const config = syncStrategyFormToJSON();
    return {
        id,
        name: document.getElementById('strategyName').value,
        description: document.getElementById('strategyDescription').value,
        config_json: JSON.stringify(config),
        enabled: document.getElementById('strategyEnabled').checked,
        readonly
    };
}

function formatStrategyConfig() {
    try {
        document.getElementById('strategyConfig').value = prettyJSON(JSON.parse(document.getElementById('strategyConfig').value || '{}'));
    } catch (error) {
        alert('配置不是有效JSON：' + error.message);
    }
}

async function saveStrategy() {
    if (strategyRunState.running) return;
    try {
        const payload = currentStrategyPayload();
        if (payload.readonly) {
            alert('内置模板不能直接编辑，请先复制');
            return;
        }
        const item = await apiFetch(payload.id ? `/api/strategies/${payload.id}` : '/api/strategies', {
            method: payload.id ? 'PUT' : 'POST',
            body: JSON.stringify(payload)
        });
        selectedStrategyID = item.id;
        await loadStrategies();
        fillStrategy(item.id);
        alert('策略已保存');
    } catch (error) {
        alert(error.message);
    }
}

async function cloneCurrentStrategy() {
    if (strategyRunState.running) return;
    const id = document.getElementById('strategyName').dataset.id || selectedStrategyID;
    if (!id) {
        alert('请先选择一个策略');
        return;
    }
    try {
        const item = await apiFetch(`/api/strategies/${id}/clone`, { method: 'POST' });
        selectedStrategyID = item.id;
        await loadStrategies();
        fillStrategy(item.id);
        alert('策略已复制');
    } catch (error) {
        alert(error.message);
    }
}

async function deleteCurrentStrategy() {
    if (strategyRunState.running) return;
    const id = document.getElementById('strategyName').dataset.id || selectedStrategyID;
    const readonly = document.getElementById('strategyName').dataset.readonly === 'true';
    if (!id) return;
    if (readonly) {
        alert('内置模板不能删除');
        return;
    }
    if (!confirm('确认删除这个策略？')) return;
    try {
        await apiFetch(`/api/strategies/${id}`, { method: 'DELETE' });
        selectedStrategyID = '';
        await loadStrategies();
        if (strategies.length) fillStrategy(strategies[0].id);
    } catch (error) {
        alert(error.message);
    }
}

async function runCurrentStrategy() {
    const id = document.getElementById('strategyName').dataset.id || selectedStrategyID;
    if (!id) {
        alert('请先保存或选择一个策略');
        return;
    }
    const list = document.getElementById('strategyRunResults');
    startStrategyRunUI(id);
    if (list) list.innerHTML = '';
    try {
        const run = await apiFetch(`/api/strategies/${id}/run`, { method: 'POST' });
        const result = JSON.parse(run.result_json || '{}');
        const items = result.items || [];
        finishStrategyRunUI('success', `完成 · 扫描 ${result.total || 0} 只 · 命中 ${run.matched_count || items.length} 只`);
        if (list) {
            list.innerHTML = items.slice(0, 80).map(item => `
                <div class="data-item">
                    <div class="result-symbol">${escapeHTML(item.symbol)}</div>
                    <div class="data-item-meta">评分 <span class="result-latest">${Number(item.score || 0).toFixed(2)}</span> · 最新价 ${Number(item.latest || 0).toFixed(2)}</div>
                    <div class="strategy-reasons">${(item.reasons || []).slice(0, 4).map(reason => `<span>${escapeHTML(reason)}</span>`).join('')}</div>
                    ${renderStrategyFactorResults(item.factor_results || [])}
                    <div class="item-actions">
                        <button class="primary" onclick="openResultChart('${escapeJSString(item.symbol)}')">打开图表</button>
                        <button onclick="addSymbolToPool('watchlist', '${escapeJSString(item.symbol)}')">观察</button>
                    </div>
                </div>
            `).join('') || '<div class="data-item">未命中标的</div>';
        }
        await Promise.all([loadRuns(), loadSelectionResults()]);
    } catch (error) {
        finishStrategyRunUI('failed', error.message);
        alert(error.message);
    }
}

async function runCurrentStrategyBacktest() {
    const id = document.getElementById('strategyName').dataset.id || selectedStrategyID;
    if (!id) {
        alert('请先保存或选择一个策略');
        return;
    }
    const summary = document.getElementById('strategyBacktestSummary');
    const metrics = document.getElementById('strategyBacktestMetrics');
    const trades = document.getElementById('strategyBacktestTrades');
    startStrategyBacktestUI(id);
    if (summary) summary.textContent = '正在回测...';
    if (metrics) metrics.innerHTML = '';
    if (trades) trades.innerHTML = '';
    try {
        const run = await apiFetch(`/api/strategies/${id}/backtest`, {
            method: 'POST',
            body: JSON.stringify(currentStrategyBacktestPayload())
        });
        const m = run.metrics || {};
        finishStrategyBacktestUI('success', `完成 · 标的 ${m.symbols || 0} · 交易 ${m.trades || 0} · 胜率 ${(Number(m.win_rate || 0) * 100).toFixed(2)}%`);
        if (summary) summary.textContent = `回测完成：${run.strategy?.name || ''}`;
        if (metrics) {
            metrics.innerHTML = [
                metricItem('标的', m.symbols || 0),
                metricItem('交易', m.trades || 0),
                metricItem('胜率', `${(Number(m.win_rate || 0) * 100).toFixed(2)}%`),
                metricItem('总收益', `${(Number(m.total_return || 0) * 100).toFixed(2)}%`),
                metricItem('年化', `${(Number(m.cagr || 0) * 100).toFixed(2)}%`),
                metricItem('最大回撤', `${(Number(m.max_drawdown || 0) * 100).toFixed(2)}%`),
                metricItem('盈亏比', Number.isFinite(Number(m.profit_factor)) ? Number(m.profit_factor).toFixed(2) : '--'),
                metricItem('单笔均值', `${(Number(m.avg_trade || 0) * 100).toFixed(2)}%`)
            ].join('');
        }
        if (trades) {
            const list = Array.isArray(run.trades) ? run.trades : [];
            trades.innerHTML = list.slice(0, 80).map(item => `
                <div class="data-item">
                    <div class="data-item-title">${escapeHTML(item.symbol || '--')} <span class="tag">${escapeHTML(item.reason || '--')}</span></div>
                    <div class="data-item-meta">买入 ${escapeHTML(item.entry_date || '--')} ${Number(item.entry_price || 0).toFixed(2)} · 卖出 ${escapeHTML(item.exit_date || '--')} ${Number(item.exit_price || 0).toFixed(2)}</div>
                    <div class="data-item-meta">收益 ${(Number(item.return || 0) * 100).toFixed(2)}% · 持有 ${Number(item.hold_days || 0)} 天</div>
                    <div class="strategy-reasons">${(item.entry_reasons || []).slice(0, 4).map(reason => `<span>${escapeHTML(reason)}</span>`).join('')}</div>
                </div>
            `).join('') || '<div class="data-item">没有生成交易</div>';
        }
    } catch (error) {
        finishStrategyBacktestUI('failed', error.message);
        alert(error.message);
    }
}

function metricItem(label, value) {
    return `
        <div class="metric-item">
            <span>${escapeHTML(label)}</span>
            <strong>${escapeHTML(String(value))}</strong>
        </div>
    `;
}

function startStrategyBacktestUI(strategyID) {
    strategyBacktestState = {
        running: true,
        strategyID,
        timer: null,
        message: '准备历史数据',
        progress: 10,
        status: ''
    };
    setStrategyEditorLocked(true);
    renderStrategyBacktestSummary();
}

function finishStrategyBacktestUI(status, message) {
    strategyBacktestState.running = false;
    strategyBacktestState.message = message;
    strategyBacktestState.progress = status === 'success' ? 100 : strategyBacktestState.progress;
    strategyBacktestState.status = status;
    setStrategyEditorLocked(false);
    renderStrategyBacktestSummary();
}

function renderStrategyBacktestSummary() {
    const summary = document.getElementById('strategyBacktestSummary');
    if (!summary) return;
    if (!strategyBacktestState.running) {
        summary.className = `data-item strategy-run-summary ${strategyBacktestState.status || ''}`;
        summary.textContent = strategyBacktestState.message || '暂无回测';
        return;
    }
    summary.className = 'data-item strategy-run-summary running';
    summary.innerHTML = `
        <div class="strategy-progress-head">
            <strong>回测中：${escapeHTML(strategyBacktestState.message)}</strong>
            <span>${strategyBacktestState.progress}%</span>
        </div>
        <div class="strategy-progress-bar"><i style="width:${strategyBacktestState.progress}%"></i></div>
    `;
}

function renderStrategyFactorResults(factorResults = []) {
    if (!Array.isArray(factorResults) || !factorResults.length) {
        return '';
    }
    return `
        <div class="strategy-factor-results">
            ${factorResults.slice(0, 6).map(factor => `
                <div class="strategy-factor-row ${factor.hit ? 'hit' : 'miss'}">
                    <span>${escapeHTML(factor.factor || factor.id || '--')}</span>
                    <strong>${factor.hit ? '命中' : '未命中'}</strong>
                    <em>${escapeHTML(compactValue(factor.value ?? factor.Value ?? factor.score ?? factor.Score ?? ''))}</em>
                </div>
            `).join('')}
        </div>
    `;
}

function startStrategyRunUI(strategyID) {
    stopStrategyProgressTimer();
    strategyRunState = {
        running: true,
        strategyID,
        stepIndex: 0,
        timer: null,
        message: '准备数据',
        progress: 8
    };
    setStrategyEditorLocked(true);
    renderStrategyList();
    renderStrategyProgressPanel();
    strategyRunState.timer = setInterval(advanceStrategyRunStep, 1400);
}

function advanceStrategyRunStep() {
    const steps = strategyProgressSteps();
    if (!strategyRunState.running) return;
    strategyRunState.stepIndex = Math.min(strategyRunState.stepIndex + 1, steps.length - 1);
    const current = steps[strategyRunState.stepIndex];
    strategyRunState.message = current.label;
    strategyRunState.progress = current.progress;
    renderStrategyList();
    renderStrategyProgressPanel();
}

function finishStrategyRunUI(status, message) {
    stopStrategyProgressTimer();
    strategyRunState.running = false;
    strategyRunState.message = message;
    strategyRunState.progress = status === 'success' ? 100 : strategyRunState.progress;
    setStrategyEditorLocked(false);
    renderStrategyList();
    const summary = document.getElementById('strategyRunSummary');
    if (summary) {
        summary.className = `data-item strategy-run-summary ${status}`;
        summary.innerHTML = escapeHTML(message || (status === 'success' ? '完成' : '运行失败'));
    }
}

function stopStrategyProgressTimer() {
    if (strategyRunState.timer) {
        clearInterval(strategyRunState.timer);
    }
    strategyRunState.timer = null;
}

function strategyProgressSteps() {
    return [
        { label: '准备数据', progress: 8 },
        { label: '加载K线', progress: 24 },
        { label: '计算公式', progress: 46 },
        { label: '计算因子', progress: 64 },
        { label: '评分排序', progress: 82 },
        { label: '写入结果', progress: 94 }
    ];
}

function renderStrategyProgressPanel() {
    const summary = document.getElementById('strategyRunSummary');
    if (!summary) return;
    const steps = strategyProgressSteps();
    summary.className = 'data-item strategy-run-summary running';
    summary.innerHTML = `
        <div class="strategy-progress-head">
            <strong>运行中：${escapeHTML(strategyRunState.message)}</strong>
            <span>${strategyRunState.progress}%</span>
        </div>
        <div class="strategy-progress-bar"><i style="width:${strategyRunState.progress}%"></i></div>
        <div class="strategy-step-list">
            ${steps.map((step, index) => `<span class="${index <= strategyRunState.stepIndex ? 'active' : ''}">${escapeHTML(step.label)}</span>`).join('')}
        </div>
    `;
}

function setStrategyEditorLocked(locked) {
    const ids = [
        'strategySaveButton',
        'strategyCloneButton',
        'strategyRunButton',
        'strategyBacktestButton',
        'strategyBacktestRunButton',
        'strategyDeleteButton',
        'strategyApplyJSONButton',
        'strategySyncJSONButton',
        'strategyFormatJSONButton',
        'strategyName',
        'strategyEnabled',
        'strategyDescription',
        'strategyConfig'
    ];
    ids.forEach(id => {
        const node = document.getElementById(id);
        if (node) node.disabled = locked;
    });
    const runButton = document.getElementById('strategyRunButton');
    if (runButton) runButton.textContent = locked ? '运行中...' : '运行';
    const backtestButton = document.getElementById('strategyBacktestRunButton');
    if (backtestButton) backtestButton.textContent = locked ? '回测中...' : '回测';
    document.querySelectorAll('#strategyVisualEditor input, #strategyVisualEditor select, #strategyVisualEditor textarea, #strategyVisualEditor button, .advanced-json-panel button').forEach(node => {
        node.disabled = locked;
    });
    document.querySelectorAll('.strategy-backtest-form input, .strategy-backtest-form button').forEach(node => {
        node.disabled = locked;
    });
    const editor = document.querySelector('.strategy-editor-card');
    if (editor) editor.classList.toggle('locked', locked);
}

function appendStrategyFactor(factorID) {
    const factor = factors.find(item => item.id === factorID);
    if (!factor) return;
    try {
        const cfg = JSON.parse(document.getElementById('strategyConfig').value || '{}');
        const params = {};
        (factor.params || []).forEach(param => {
            if (param.default !== '') params[param.name] = param.default;
        });
        const rule = {
            id: `${factor.id}_${Date.now().toString(36)}`,
            factor: factor.id,
            params
        };
        if (factor.kind === 'score') rule.weight = 10;
        if (factor.kind === 'filter') {
            cfg.filters = Array.isArray(cfg.filters) ? cfg.filters : [];
            cfg.filters.push(rule);
        } else {
            cfg.scores = Array.isArray(cfg.scores) ? cfg.scores : [];
            cfg.scores.push(rule);
        }
        document.getElementById('strategyConfig').value = prettyJSON(cfg);
    } catch (error) {
        alert('请先修正策略配置JSON：' + error.message);
    }
}

async function loadAutomationData() {
    await Promise.all([loadFormulaList(), loadPools(), loadStrategies(), loadWebhooks(), loadAutomations(), loadRuns(), loadSelectionResults()]);
    updateAutomationPayloadMode();
}

async function loadAutomations() {
    automations = await apiFetch('/api/automations') || [];
    renderAutomations();
}

function renderAutomations() {
    document.getElementById('automationList').innerHTML = automations.map(t => `
        <div class="data-item automation-task-item ${automationRunState.running && automationRunState.taskID === t.id ? 'running' : ''}">
            <div class="data-item-title">${escapeHTML(t.name)}${t.readonly ? ' <span class="tag">系统固定</span>' : ''}</div>
            <div class="data-item-meta">${escapeHTML(t.type)} · ${escapeHTML(t.cron)} · ${t.enabled ? '启用' : '停用'}${t.next_run_at ? ` · 下次：${escapeHTML(t.next_run_at)}` : ''}</div>
            <div class="data-item-meta">上次：${escapeHTML(t.last_status || '--')} ${escapeHTML(t.last_message || '')}</div>
            ${renderAutomationRunStatus(t.id)}
            <div class="item-actions">${automationTaskActions(t)}</div>
        </div>
    `).join('') || '<div class="data-item">暂无任务</div>';
}

function automationTaskActions(task) {
    const id = escapeJSString(task.id);
    const running = automationRunState.running;
    if (task.readonly) {
        return `
            <button class="${task.enabled ? '' : 'primary'}" onclick="toggleAutomation('${id}', ${task.enabled ? 'false' : 'true'})" ${running ? 'disabled' : ''}>${task.enabled ? '关闭' : '开启'}</button>
            <button onclick="runAutomation('${id}')" ${running ? 'disabled' : ''}>${automationRunState.taskID === task.id ? '执行中...' : '立即执行一次'}</button>
        `;
    }
    return `
        <button onclick="fillAutomation('${id}')" ${running ? 'disabled' : ''}>编辑</button>
        <button class="primary" onclick="runAutomation('${id}')" ${running ? 'disabled' : ''}>${automationRunState.taskID === task.id ? '运行中...' : '立即运行'}</button>
        <button onclick="deleteAutomation('${id}')" ${running ? 'disabled' : ''}>删除</button>
    `;
}

function renderAutomationRunStatus(taskID) {
    if (!automationRunState.running || automationRunState.taskID !== taskID) {
        return '';
    }
    return `
        <div class="automation-run-progress">
            <div class="strategy-progress-head">
                <strong>执行中：${escapeHTML(automationRunState.message || '准备任务')}</strong>
                <span>${automationRunState.progress}%</span>
            </div>
            <div class="strategy-progress-bar"><i style="width:${automationRunState.progress}%"></i></div>
        </div>
    `;
}

function fillAutomation(id) {
    if (automationRunState.running) return;
    const t = automations.find(item => item.id === id);
    if (!t) return;
    if (t.readonly) {
        alert('固定任务只能开启或关闭，不能编辑');
        return;
    }
    const payload = JSON.parse(t.payload_json || '{}');
    document.getElementById('automationName').dataset.id = t.id;
    document.getElementById('automationName').value = t.name;
    document.getElementById('automationType').value = t.type || 'stock_selection';
    document.getElementById('automationFormula').value = payload.formula_id || '';
    document.getElementById('automationPool').value = payload.pool_id || '';
    const strategySelect = document.getElementById('automationStrategy');
    if (strategySelect) strategySelect.value = payload.strategy_id || '';
    document.getElementById('automationPayload').value = JSON.stringify(payload, null, 2);
    document.getElementById('automationCron').value = t.cron;
    document.getElementById('automationEnabled').checked = !!t.enabled;
    const ids = JSON.parse(t.webhook_ids || '[]');
    Array.from(document.getElementById('automationWebhook').options).forEach(opt => {
        opt.selected = ids.includes(opt.value);
    });
    updateAutomationPayloadMode();
}

function updateAutomationPayloadMode() {
    const type = document.getElementById('automationType')?.value || 'stock_selection';
    const stockFields = document.getElementById('stockSelectionFields');
    const strategyFields = document.getElementById('strategySelectionFields');
    const payloadBox = document.getElementById('automationPayload');
    if (stockFields) stockFields.style.display = type === 'stock_selection' ? 'block' : 'none';
    if (strategyFields) strategyFields.style.display = type === 'strategy_selection' ? 'block' : 'none';
    if (!payloadBox) return;
    payloadBox.style.display = (type === 'stock_selection' || type === 'strategy_selection') ? 'none' : 'block';
    if (!payloadBox.value.trim()) {
        payloadBox.value = type === 'system_sync'
            ? JSON.stringify({ scope: 'all', tables: ['day'], limit: 4, max_codes: 200, continue_on_error: true }, null, 2)
            : JSON.stringify({ action: 'noop', data: {} }, null, 2);
    }
}

async function saveAutomation() {
    if (automationRunState.running) return;
    try {
        const id = document.getElementById('automationName').dataset.id || '';
        const type = document.getElementById('automationType').value || 'stock_selection';
        const webhookIds = Array.from(document.getElementById('automationWebhook').selectedOptions).map(opt => opt.value);
        let payload;
        if (type === 'stock_selection') {
            payload = {
                formula_id: document.getElementById('automationFormula').value,
                pool_id: document.getElementById('automationPool').value,
                calc_count: 240,
                out_count: 1,
                batch_size: 50,
                continue_on_error: true
            };
        } else if (type === 'strategy_selection') {
            payload = {
                strategy_id: document.getElementById('automationStrategy').value
            };
        } else {
            payload = JSON.parse(document.getElementById('automationPayload').value || '{}');
        }
        await apiFetch(id ? `/api/automations/${id}` : '/api/automations', {
            method: id ? 'PUT' : 'POST',
            body: JSON.stringify({
                id,
                name: document.getElementById('automationName').value,
                type,
                cron: document.getElementById('automationCron').value,
                enabled: document.getElementById('automationEnabled').checked,
                payload_json: JSON.stringify(payload),
                webhook_ids: JSON.stringify(webhookIds)
            })
        });
        document.getElementById('automationName').dataset.id = '';
        await loadAutomations();
        alert('任务已保存');
    } catch (error) {
        alert(error.message);
    }
}

async function runAutomation(id) {
    if (automationRunState.running) return;
    startAutomationRunUI(id);
    try {
        await apiFetch(`/api/automations/${id}/run`, { method: 'POST' });
        finishAutomationRunUI('success', '任务执行完成');
        await Promise.all([loadAutomations(), loadRuns(), loadSelectionResults()]);
        alert('任务执行完成');
    } catch (error) {
        finishAutomationRunUI('failed', error.message);
        alert(error.message);
        await loadRuns();
    }
}

function startAutomationRunUI(taskID) {
    stopAutomationProgressTimer();
    automationRunState = {
        running: true,
        taskID,
        stepIndex: 0,
        timer: null,
        message: '准备任务',
        progress: 10
    };
    renderAutomations();
    automationRunState.timer = setInterval(advanceAutomationRunStep, 1300);
}

function advanceAutomationRunStep() {
    const steps = automationProgressSteps();
    if (!automationRunState.running) return;
    automationRunState.stepIndex = Math.min(automationRunState.stepIndex + 1, steps.length - 1);
    const current = steps[automationRunState.stepIndex];
    automationRunState.message = current.label;
    automationRunState.progress = current.progress;
    renderAutomationRunState();
}

function finishAutomationRunUI(status, message) {
    stopAutomationProgressTimer();
    automationRunState.running = false;
    automationRunState.message = message;
    automationRunState.progress = status === 'success' ? 100 : automationRunState.progress;
    renderAutomationRunState();
}

function stopAutomationProgressTimer() {
    if (automationRunState.timer) {
        clearInterval(automationRunState.timer);
    }
    automationRunState.timer = null;
}

function automationProgressSteps() {
    return [
        { label: '准备任务', progress: 10 },
        { label: '读取配置', progress: 25 },
        { label: '执行任务', progress: 48 },
        { label: '处理结果', progress: 72 },
        { label: '写入记录', progress: 90 }
    ];
}

function renderAutomationRunState() {
    renderAutomations();
}

async function toggleAutomation(id, enabled) {
    if (automationRunState.running) return;
    try {
        await apiFetch(`/api/automations/${id}/enabled`, {
            method: 'PUT',
            body: JSON.stringify({ enabled })
        });
        await loadAutomations();
    } catch (error) {
        alert(error.message);
    }
}

async function createSystemTemplate(template) {
    if (automationRunState.running) return;
    try {
        await apiFetch('/api/automations/templates', {
            method: 'POST',
            body: JSON.stringify({ template })
        });
        await loadAutomations();
        alert('系统任务模板已创建');
    } catch (error) {
        alert(error.message);
    }
}

async function deleteAutomation(id) {
    if (automationRunState.running) return;
    if (!confirm('确认删除这个任务？')) return;
    await apiFetch(`/api/automations/${id}`, { method: 'DELETE' });
    await loadAutomations();
}

async function loadRuns() {
    const runs = await apiFetch('/api/automations/runs?limit=30') || [];
    document.getElementById('runList').innerHTML = runs.map(r => `
        <div class="data-item">
            <div class="data-item-title">${escapeHTML(r.task_name)} · ${escapeHTML(r.status)}</div>
            <div class="data-item-meta">${escapeHTML(r.started_at)} · 命中 ${r.matched_count}</div>
            <div class="data-item-meta">${escapeHTML(r.log || (r.result_json || '').slice(0, 240))}</div>
        </div>
    `).join('') || '<div class="data-item">暂无运行记录</div>';
}

async function loadSelectionResults() {
    const list = document.getElementById('selectionResultList');
    if (!list) return;
    const formulaID = document.getElementById('resultFormulaFilter')?.value || '';
    const symbol = document.getElementById('resultSymbolFilter')?.value || '';
    const latest = document.getElementById('resultLatestFilter')?.value || '';
    const params = new URLSearchParams({ limit: '200' });
    if (formulaID) params.set('formula_id', formulaID);
    if (symbol) params.set('symbol', symbol);
    if (latest) params.set('latest', latest);
    selectionResults = await apiFetch(`/api/selection-results?${params.toString()}`) || [];
    list.innerHTML = selectionResults.map(item => `
        <div class="data-item">
            <div class="result-symbol">${escapeHTML(item.symbol)}</div>
            <div class="data-item-meta">${escapeHTML(item.formula_name)} · ${escapeHTML(item.task_name)}</div>
            <div class="data-item-meta">${escapeHTML(item.created_at)} · 最新值 <span class="result-latest">${Number(item.latest || 0).toFixed(4)}</span></div>
            <div class="item-actions">
                <button class="primary" onclick="openResultChart('${item.symbol}')">打开图表</button>
                <button onclick="showResultDetail('${item.id}')">详情</button>
                <button onclick="addSymbolToPool('watchlist', '${item.symbol}')">观察</button>
                <button onclick="addSymbolToPool('exclude', '${item.symbol}')">排除</button>
            </div>
        </div>
    `).join('') || '<div class="data-item">暂无命中结果</div>';
}

async function refreshDecisionDesk() {
    const hitSummary = document.getElementById('decisionHitSummary');
    const hitList = document.getElementById('decisionHits');
    if (hitSummary) hitSummary.textContent = '加载中';
    if (hitList) hitList.innerHTML = '<div class="data-item">正在加载命中结果...</div>';
    try {
        if (!pools.length) {
            await loadPools();
        }
        await loadDecisionResults();
        renderDecisionPools();
        if (!selectedDecisionResult && decisionResults.length > 0) {
            await selectDecisionResult(decisionResults[0].id);
        } else {
            updateDecisionActionState();
        }
    } catch (error) {
        if (hitSummary) hitSummary.textContent = '加载失败';
        if (hitList) hitList.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
    }
}

async function loadDecisionResults() {
    const today = localDateString();
    dailyReview = await apiFetch('/api/daily-review?limit=200') || {};
    reviewItems = dailyReview.items || [];
    const allResults = reviewItems.map(item => item.result).filter(Boolean);
    const todayItems = allResults.filter(item => String(item.created_at || '').startsWith(today));
    decisionResults = todayItems.length ? todayItems : allResults;
    decisionShowingToday = todayItems.length > 0;
    if (selectedDecisionResult) {
        selectedReviewItem = reviewItemBySymbol(selectedDecisionResult.symbol);
        const latestSelected = decisionResults.find(item => normalizeSymbol(item.symbol) === normalizeSymbol(selectedDecisionResult.symbol));
        if (latestSelected) selectedDecisionResult = latestSelected;
    }
    renderDecisionHits(todayItems.length > 0, allResults.length);
}

function renderDecisionHits(isToday, latestCount) {
    const summary = document.getElementById('decisionHitSummary');
    const list = document.getElementById('decisionHits');
    if (!list) return;
    if (summary) {
        if (decisionResults.length === 0) {
            summary.textContent = '暂无选股命中，去自动化页运行一次选股任务';
        } else {
            summary.textContent = isToday
                ? `今日命中 ${decisionResults.length} 只`
                : `今日暂无命中，显示最近一次运行的 ${latestCount} 只`;
        }
    }
    list.innerHTML = decisionResults.map(item => {
        const review = reviewItemBySymbol(item.symbol);
        const statusInfo = decisionStatusInfo(item.symbol, review);
        const total = review?.score?.total || 0;
        const track = review?.track?.summary || '待跟踪';
        return `
            <button class="decision-hit ${selectedDecisionResult?.id === item.id ? 'active' : ''}" onclick="selectDecisionResult('${item.id}')">
                <span class="decision-hit-top">
                    <span class="decision-symbol">${escapeHTML(item.symbol)}</span>
                    <span class="score-total">${total ? total + '分' : '--'}</span>
                </span>
                <span class="decision-hit-meta">${escapeHTML(item.formula_name)} · ${escapeHTML(item.task_name || '--')}</span>
                <span class="decision-hit-foot">
                    <span>${escapeHTML(formatDecisionTime(item.created_at))}</span>
                    <strong>${Number(item.latest || 0).toFixed(4)}</strong>
                    <em class="${statusInfo.className}">${statusInfo.label}</em>
                </span>
                <span class="decision-hit-meta">${escapeHTML(track)}</span>
            </button>
        `;
    }).join('') || '<div class="data-item">暂无命中结果</div>';
}

function renderDecisionPools() {
    renderPoolChips('watchlist', 'watchPoolList', 'watchPoolSummary');
    renderPoolChips('exclude', 'excludePoolList', 'excludePoolSummary');
}

function renderPoolChips(poolID, listID, summaryID) {
    const pool = poolByID(poolID);
    const list = document.getElementById(listID);
    const summary = document.getElementById(summaryID);
    const symbols = pool?.symbols || [];
    if (summary) summary.textContent = `${symbols.length} 只`;
    if (!list) return;
    list.innerHTML = symbols.slice(0, 80).map(symbol => `
        <button class="pool-chip" onclick="openDecisionSymbol('${normalizeSymbol(symbol)}')">
            ${escapeHTML(symbol)}
        </button>
    `).join('') || '<div class="data-item compact-empty">暂无股票</div>';
}

async function selectDecisionResult(id) {
    const item = decisionResults.find(v => v.id === id);
    if (!item) return;
    selectedDecisionResult = item;
    selectedReviewItem = reviewItems.find(v => v.result?.id === id) || reviewItemBySymbol(item.symbol);
    currentStock = item.symbol;
    const input = document.getElementById('stockCode');
    if (input) input.value = item.symbol;
    renderDecisionHits(decisionShowingToday, decisionResults.length);
    renderDecisionDetail(item);
    await loadDecisionChart(item.symbol);
}

async function openDecisionSymbol(symbol) {
    const normalized = normalizeSymbol(symbol);
    const existing = decisionResults.find(item => normalizeSymbol(item.symbol) === normalized);
    if (existing) {
        await selectDecisionResult(existing.id);
        return;
    }
    selectedDecisionResult = { id: '', symbol: normalized, detail_json: '{}', formula_name: '手动查看', task_name: '股票池' };
    selectedReviewItem = reviewItemBySymbol(normalized) || { result: selectedDecisionResult, note: {}, score: {}, track: {} };
    currentStock = normalized;
    renderDecisionDetail(selectedDecisionResult);
    await loadDecisionChart(normalized);
}

function renderDecisionDetail(item) {
    const symbol = normalizeSymbol(item.symbol);
    const review = selectedReviewItem || reviewItemBySymbol(symbol) || {};
    document.getElementById('decisionDetailTitle').textContent = `${symbol} 命中详情`;
    document.getElementById('decisionDetailMeta').textContent = `${item.formula_name || '--'} · ${item.task_name || '--'} · ${formatDecisionTime(item.created_at)}`;
    document.getElementById('decisionScore').innerHTML = renderScoreGrid(review.score);
    document.getElementById('decisionTrack').innerHTML = renderTrackPanel(review.track);
    document.getElementById('decisionReason').innerHTML = renderDecisionReason(item);
    fillDecisionNoteForm(review.note || { symbol });
    updateDecisionActionState();
}

function renderDecisionReason(item) {
    let detail = {};
    try {
        detail = JSON.parse(item.detail_json || '{}');
    } catch {
        return `<pre class="json-output">${escapeHTML(item.detail_json || '')}</pre>`;
    }
    const rows = extractReasonRows(detail);
    if (!rows.length) {
        return `<pre class="json-output">${escapeHTML(prettyJSON(detail))}</pre>`;
    }
    return rows.map(row => `
        <div class="reason-item">
            <span>${escapeHTML(row.label)}</span>
            <strong>${escapeHTML(compactValue(row.value))}</strong>
        </div>
    `).join('');
}

function extractReasonRows(detail) {
    const rows = [];
    const push = (label, value) => {
        if (value !== undefined && value !== null && value !== '') rows.push({ label, value });
    };
    push('最新输出', detail.latest ?? detail.Latest ?? detail.value ?? detail.Value);
    push('是否命中', detail.hit ?? detail.Hit ?? detail.signal ?? detail.Signal);
    push('公式引擎', detail.engine ?? detail.Engine);
    push('耗时', detail.tick_ms !== undefined ? `${detail.tick_ms} ms` : undefined);
    const data = detail.data ?? detail.Data;
    if (Array.isArray(data)) {
        data.slice(-6).forEach((value, index) => push(`输出 ${data.length - 6 + index + 1}`, value));
    } else if (data && typeof data === 'object') {
        Object.entries(data).slice(0, 16).forEach(([key, value]) => push(key, value));
    }
    Object.entries(detail).forEach(([key, value]) => {
        if (rows.length >= 18) return;
        if (['latest', 'Latest', 'value', 'Value', 'hit', 'Hit', 'signal', 'Signal', 'data', 'Data'].includes(key)) return;
        if (typeof value !== 'object') push(key, value);
    });
    return rows;
}

async function loadDecisionChart(symbol) {
    const chartDom = document.getElementById('decisionChart');
    if (!chartDom) return;
    chartDom.innerHTML = '';
    try {
        renderHQKLine('decisionChart', symbol, 'day', {
            count: 260,
            pageSize: 80,
            windows: [
                { Index: 'MA' },
                { Index: 'VOL' }
            ]
        });
    } catch (error) {
        chartDom.innerHTML = `<div class="data-item">${escapeHTML(error.message || error)}</div>`;
    }
}

async function addDecisionSymbolToPool(poolID) {
    if (!selectedDecisionResult?.symbol) return;
    await addSymbolToPool(poolID, selectedDecisionResult.symbol);
}

async function removeDecisionSymbolFromPools() {
    if (!selectedDecisionResult?.symbol) return;
    await Promise.allSettled([
        removeSymbolFromPool('watchlist', selectedDecisionResult.symbol, false),
        removeSymbolFromPool('exclude', selectedDecisionResult.symbol, false)
    ]);
    await upsertDecisionNote({ symbol: selectedDecisionResult.symbol, status: '' });
    await loadPools();
    await loadDecisionResults();
    renderDecisionPools();
    renderDecisionHits(decisionShowingToday, decisionResults.length);
    if (selectedDecisionResult) {
        selectedReviewItem = reviewItemBySymbol(selectedDecisionResult.symbol);
        renderDecisionDetail(selectedDecisionResult);
    }
    updateDecisionActionState();
}

async function addSymbolToPool(poolID, symbol) {
    const normalized = normalizeSymbol(symbol);
    if (!normalized) return;
    const opposite = poolID === 'watchlist' ? 'exclude' : 'watchlist';
    await removeSymbolFromPool(opposite, normalized, false);
    await apiFetch(`/api/stock-pools/${poolID}/symbols/${encodeURIComponent(normalized)}`, { method: 'POST' });
    await loadPools();
    await loadDecisionResults();
    renderDecisionPools();
    renderDecisionHits(decisionShowingToday, decisionResults.length);
    if (selectedDecisionResult && normalizeSymbol(selectedDecisionResult.symbol) === normalized) {
        selectedReviewItem = reviewItemBySymbol(normalized);
        renderDecisionDetail(selectedDecisionResult);
    }
    updateDecisionActionState();
}

async function removeSymbolFromPool(poolID, symbol, refresh = true) {
    const normalized = normalizeSymbol(symbol);
    if (!normalized) return;
    await apiFetch(`/api/stock-pools/${poolID}/symbols/${encodeURIComponent(normalized)}`, { method: 'DELETE' });
    if (refresh) {
        await loadPools();
        await loadDecisionResults();
        renderDecisionPools();
        renderDecisionHits(decisionShowingToday, decisionResults.length);
        updateDecisionActionState();
    }
}

function updateDecisionActionState() {
    const symbol = selectedDecisionResult?.symbol || '';
    const watchButton = document.getElementById('decisionWatchButton');
    const excludeButton = document.getElementById('decisionExcludeButton');
    const removeButton = document.getElementById('decisionRemoveButton');
    const inWatch = symbolInPool(symbol, 'watchlist');
    const inExclude = symbolInPool(symbol, 'exclude');
    if (watchButton) {
        watchButton.disabled = !symbol || inWatch;
        watchButton.textContent = inWatch ? '已在观察' : '加入观察';
    }
    if (excludeButton) {
        excludeButton.disabled = !symbol || inExclude;
        excludeButton.textContent = inExclude ? '已排除' : '排除';
    }
    if (removeButton) {
        removeButton.disabled = !symbol || (!inWatch && !inExclude);
    }
}

function reviewItemBySymbol(symbol) {
    const normalized = normalizeSymbol(symbol);
    return reviewItems.find(item => normalizeSymbol(item.result?.symbol || item.note?.symbol) === normalized);
}

function decisionStatusInfo(symbol, review) {
    const status = review?.status || review?.note?.status || '';
    const inWatch = review?.watch || symbolInPool(symbol, 'watchlist') || status === 'watch';
    const inExclude = review?.excluded || symbolInPool(symbol, 'exclude') || status === 'exclude';
    if (inExclude) return { label: '已排除', className: 'excluded' };
    if (inWatch) return { label: '观察中', className: 'watched' };
    return { label: '待处理', className: 'pending' };
}

function renderScoreGrid(score = {}) {
    const items = [
        ['总分', score.total],
        ['趋势', score.trend],
        ['量能', score.volume],
        ['位置', score.place],
        ['风险', score.risk]
    ];
    return items.map(([label, value]) => `
        <div class="score-item ${label === '总分' ? 'score-item-total' : ''}">
            <span>${escapeHTML(label)}</span>
            <strong>${value ? escapeHTML(value) : '--'}</strong>
        </div>
    `).join('');
}

function renderTrackPanel(track = {}) {
    if (!track || !track.available) {
        return `<div class="track-empty">${escapeHTML(track?.summary || '暂无次日跟踪')}</div>`;
    }
    const rows = [
        ['日期', track.date],
        ['开盘', formatPercent(track.open_change)],
        ['最高', formatPercent(track.max_gain)],
        ['回撤', formatPercent(track.drawdown)],
        ['收盘', formatPercent(track.close_change)]
    ];
    return `
        <div class="track-title">${escapeHTML(track.summary || '次日跟踪')}</div>
        <div class="track-grid">
            ${rows.map(([label, value]) => `
                <div>
                    <span>${escapeHTML(label)}</span>
                    <strong>${escapeHTML(value)}</strong>
                </div>
            `).join('')}
        </div>
    `;
}

function fillDecisionNoteForm(note = {}) {
    const fields = {
        decisionAddedPrice: note.added_price,
        decisionPlanBuy: note.plan_buy,
        decisionStopLoss: note.stop_loss,
        decisionAddReason: note.add_reason || '',
        decisionReviewNote: note.review_note || '',
        decisionExcludeCategory: note.exclude_category || '',
        decisionExcludeReason: note.exclude_reason || ''
    };
    Object.entries(fields).forEach(([id, value]) => {
        const node = document.getElementById(id);
        if (!node) return;
        node.value = value && value !== 0 ? value : '';
    });
}

function numberInputValue(id) {
    const value = document.getElementById(id)?.value;
    if (value === undefined || value === null || value === '') return 0;
    const num = Number(value);
    return Number.isFinite(num) ? num : 0;
}

async function upsertDecisionNote(notePatch) {
    const symbol = normalizeSymbol(notePatch.symbol);
    if (!symbol) return null;
    const current = reviewItemBySymbol(symbol)?.note || selectedReviewItem?.note || {};
    const payload = {
        ...current,
        ...notePatch,
        symbol
    };
    return apiFetch(`/api/decision-notes/${encodeURIComponent(symbol)}`, {
        method: 'PUT',
        body: JSON.stringify(payload)
    });
}

async function saveDecisionNoteFromForm() {
    if (!selectedDecisionResult?.symbol) return;
    try {
        const symbol = normalizeSymbol(selectedDecisionResult.symbol);
        const statusInfo = decisionStatusInfo(symbol, selectedReviewItem);
        const status = statusInfo.className === 'watched' ? 'watch' : (statusInfo.className === 'excluded' ? 'exclude' : (selectedReviewItem?.note?.status || ''));
        await upsertDecisionNote({
            symbol,
            status,
            added_price: numberInputValue('decisionAddedPrice'),
            plan_buy: numberInputValue('decisionPlanBuy'),
            stop_loss: numberInputValue('decisionStopLoss'),
            add_reason: document.getElementById('decisionAddReason')?.value || '',
            review_note: document.getElementById('decisionReviewNote')?.value || '',
            exclude_category: document.getElementById('decisionExcludeCategory')?.value || '',
            exclude_reason: document.getElementById('decisionExcludeReason')?.value || ''
        });
        await loadDecisionResults();
        selectedReviewItem = reviewItemBySymbol(symbol);
        renderDecisionDetail(selectedDecisionResult);
        renderDecisionHits(decisionShowingToday, decisionResults.length);
        renderDailyReview(dailyReview);
        alert('记录已保存');
    } catch (error) {
        alert(error.message || error);
    }
}

async function loadDailyReview() {
    const summary = document.getElementById('dailyReviewSummary');
    if (summary) summary.textContent = '加载中';
    setLoadingText('dailyReviewList', '正在加载复盘数据...');
    try {
        if (!pools.length) await loadPools();
        dailyReview = await apiFetch('/api/daily-review?limit=200') || {};
        reviewItems = dailyReview.items || [];
        renderDailyReview(dailyReview);
    } catch (error) {
        if (summary) summary.textContent = '加载失败';
        setErrorText('dailyReviewList', error);
    }
}

function renderDailyReview(data = {}) {
    const summary = document.getElementById('dailyReviewSummary');
    const list = document.getElementById('dailyReviewList');
    if (!list) return;
    const items = data.items || [];
    const info = data.summary || {};
    if (summary) {
        summary.textContent = `${escapeHTML(data.date || localDateString())} · 命中 ${info.hits || 0} 只 · 已处理 ${info.handled_count || 0} 只`;
    }
    renderMetricCards('dailyReviewMetrics', [
        { label: '今日命中', value: info.hits || 0, note: '最近一次选股结果' },
        { label: '已处理', value: info.handled_count || 0, note: '观察、排除或已记录' },
        { label: '观察池', value: info.watch_count || 0, note: '当前跟踪标的' },
        { label: '排除池', value: info.exclude_count || 0, note: '今日不再跟踪' },
        { label: '平均评分', value: info.avg_score || '--', note: '趋势/量能/位置/风险' },
        { label: '次日胜率', value: info.tracked_count ? formatPercent(info.win_rate) : '--', note: `${info.tracked_count || 0} 只已跟踪` },
        { label: '平均收盘', value: info.tracked_count ? formatPercent(info.avg_close_change) : '--', note: '次日收盘表现' }
    ]);
    list.innerHTML = items.map(item => renderReviewCard(item)).join('') || '<div class="data-item">暂无复盘数据</div>';
}

function renderReviewCard(item) {
    const result = item.result || {};
    const statusInfo = decisionStatusInfo(result.symbol, item);
    const note = item.note || {};
    const reason = note.exclude_reason || note.add_reason || note.review_note || '暂无记录';
    return `
        <div class="data-item daily-review-card">
            <div class="review-card-head">
                <div>
                    <div class="result-symbol">${escapeHTML(result.symbol || '--')}</div>
                    <div class="data-item-meta">${escapeHTML(result.formula_name || '--')} · ${escapeHTML(result.task_name || '--')}</div>
                </div>
                <span class="review-status ${statusInfo.className}">${escapeHTML(statusInfo.label)}</span>
            </div>
            <div class="score-grid">${renderScoreGrid(item.score)}</div>
            <div class="track-panel">${renderTrackPanel(item.track)}</div>
            <div class="data-item-meta">记录：${escapeHTML(reason)}</div>
            <div class="item-actions">
                <button class="primary" onclick="selectReviewItem('${escapeJSString(result.id || '')}')">查看</button>
                <button onclick="addSymbolToPool('watchlist', '${escapeJSString(result.symbol || '')}')">观察</button>
                <button onclick="addSymbolToPool('exclude', '${escapeJSString(result.symbol || '')}')">排除</button>
            </div>
        </div>
    `;
}

async function selectReviewItem(id) {
    const review = reviewItems.find(item => item.result?.id === id);
    if (!review?.result) return;
    const symbol = normalizeSymbol(review.result.symbol);
    const hqInput = document.getElementById('hqSymbol');
    if (hqInput) hqInput.value = symbol;
    switchWorkspace('proChart', document.querySelectorAll('.workspace-tab')[1]);
}

function cloneTradingState(value) {
    return JSON.parse(JSON.stringify(value));
}

function loadTradingState() {
    if (tradingState) return tradingState;
    tradingState = cloneTradingState(defaultTradingState);
    return tradingState;
}

function normalizeTradingState(raw) {
    const parsed = raw && typeof raw === 'object' ? raw : {};
    return {
        ...cloneTradingState(defaultTradingState),
        ...parsed,
        account: {
            ...defaultTradingState.account,
            ...(parsed.account || {})
        },
        discipline: {
            ...defaultTradingState.discipline,
            ...(parsed.discipline || {})
        },
        fees: {
            ...defaultTradingState.fees,
            ...(parsed.fees || {})
        },
        trades: Array.isArray(parsed.trades) ? parsed.trades : cloneTradingState(defaultTradingState.trades)
    };
}

function tradingStateSignature(raw) {
    const state = normalizeTradingState(raw);
    return JSON.stringify({
        account: state.account,
        discipline: state.discipline,
        fees: state.fees,
        filter: state.filter,
        trades: state.trades
    });
}

async function hydrateTradingState(force = false) {
    if (tradingStateLoaded && tradingState && !force) return tradingState;
    try {
        const remote = normalizeTradingState(await apiFetch('/api/trading-system'));
        const localRaw = localStorage.getItem(tradingStoreKey);
        if (localRaw) {
            try {
                const local = normalizeTradingState(JSON.parse(localRaw));
                const defaultSignature = tradingStateSignature(defaultTradingState);
                if (tradingStateSignature(remote) === defaultSignature && tradingStateSignature(local) !== defaultSignature) {
                    tradingState = local;
                    tradingStateLoaded = true;
                    await saveTradingState();
                    return tradingState;
                }
            } catch {
                // ignore local migration errors and keep remote as the source of truth
            }
        }
        tradingState = remote;
        localStorage.setItem(tradingStoreKey, JSON.stringify(tradingState));
    } catch {
        if (!tradingState) {
            try {
                const raw = localStorage.getItem(tradingStoreKey);
                tradingState = raw ? normalizeTradingState(JSON.parse(raw)) : cloneTradingState(defaultTradingState);
            } catch {
                tradingState = cloneTradingState(defaultTradingState);
            }
        }
    }
    tradingStateLoaded = true;
    return tradingState;
}

async function saveTradingState() {
    if (!tradingState) loadTradingState();
    try {
        const saved = await apiFetch('/api/trading-system', {
            method: 'PUT',
            body: JSON.stringify(tradingState)
        });
        tradingState = normalizeTradingState(saved);
        tradingStateLoaded = true;
        localStorage.setItem(tradingStoreKey, JSON.stringify(tradingState));
        return tradingState;
    } catch (error) {
        localStorage.setItem(tradingStoreKey, JSON.stringify(tradingState));
        throw error;
    }
}

function tradingMoney(value, decimals = 2) {
    const num = Number(value || 0);
    return num.toLocaleString('zh-CN', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
}

function tradingPrice(value, decimals = 3) {
    return tradingMoney(value, decimals);
}

function tradingQuantity(value) {
    const num = Number(value || 0);
    if (!Number.isFinite(num)) return '--';
    return num.toLocaleString('zh-CN', { maximumFractionDigits: 0 });
}

function tradingTrendClass(delta) {
    const num = Number(delta || 0);
    if (num > 0) return 'trading-positive';
    if (num < 0) return 'trading-negative';
    return 'trading-neutral';
}

function tradingSignedAmount(value, decimals = 2) {
    const num = Number(value || 0);
    const abs = tradingMoney(Math.abs(num), decimals);
    return `${num >= 0 ? '+' : '-'}${abs}`;
}

function tradingPercent(value) {
    const num = Number(value || 0);
    return `${num.toFixed(2)}%`;
}

function tradingLineCount(text) {
    return String(text || '')
        .split('\n')
        .map(line => line.trim())
        .filter(Boolean).length;
}

function tradingSnippet(text, maxLines = 2) {
    return String(text || '')
        .split('\n')
        .map(line => line.trim())
        .filter(Boolean)
        .slice(0, maxLines)
        .join(' ');
}

function tradingNumberInput(id) {
    const value = document.getElementById(id)?.value;
    if (value === undefined || value === null || value === '') return 0;
    const num = Number(value);
    return Number.isFinite(num) ? num : 0;
}

function tradingRateToDecimal(value) {
    const num = Number(value || 0);
    return Number.isFinite(num) ? num / 100 : 0;
}

function tradingCommission(turnover, ratePercent, minCommission = 0) {
    const rate = tradingRateToDecimal(ratePercent);
    if (!(turnover > 0) || !(rate > 0)) return 0;
    return Math.max(turnover * rate, Number(minCommission || 0));
}

function tradingIsShanghai(symbol) {
    return String(symbol || '').trim().startsWith('6');
}

function isTradingSession(date = new Date()) {
    const day = date.getDay();
    if (day === 0 || day === 6) return false;
    const minutes = date.getHours() * 60 + date.getMinutes();
    const morning = minutes >= 9 * 60 + 30 && minutes <= 11 * 60 + 30;
    const afternoon = minutes >= 13 * 60 && minutes <= 15 * 60;
    return morning || afternoon;
}

function isTradingWorkspaceActive() {
    return document.getElementById('tradingSystemWorkspace')?.classList.contains('active');
}

function formatTradingCountdown(ms) {
    const safe = Math.max(0, Math.ceil(ms / 1000));
    const minutes = String(Math.floor(safe / 60)).padStart(2, '0');
    const seconds = String(safe % 60).padStart(2, '0');
    return `${minutes}:${seconds}`;
}

function updateTradingRefreshStatus() {
    const statusNode = document.getElementById('tradingRefreshStatus');
    const countdownNode = document.getElementById('tradingRefreshCountdown');
    if (!statusNode || !countdownNode) return;

    if (tradingRefreshing) {
        statusNode.textContent = '正在刷新行情';
        countdownNode.textContent = '--:--';
        return;
    }

    if (!isTradingSession()) {
        statusNode.textContent = tradingLastRefreshAt ? `上次刷新 ${tradingLastRefreshAt}` : '非交易时段';
        countdownNode.textContent = '手动可刷';
        return;
    }

    statusNode.textContent = tradingRefreshMessage;
    countdownNode.textContent = formatTradingCountdown(tradingNextRefreshAt - Date.now());
}

function startTradingAutoRefresh() {
    if (!tradingNextRefreshAt) {
        tradingNextRefreshAt = Date.now() + tradingRefreshIntervalMS;
    }
    if (tradingRefreshTimer) {
        updateTradingRefreshStatus();
        return;
    }
    tradingRefreshTimer = setInterval(() => {
        if (!isTradingWorkspaceActive()) return;
        if (isTradingSession() && Date.now() >= tradingNextRefreshAt) {
            refreshTradingQuotes({ silent: true });
            return;
        }
        updateTradingRefreshStatus();
    }, 1000);
    updateTradingRefreshStatus();
}

function quoteClosePrice(quote) {
    const raw = Number(quote?.K?.Close || 0);
    return raw > 0 ? raw / 1000 : 0;
}

function quotePrevClosePrice(quote) {
    const raw = Number(quote?.K?.Last || 0);
    return raw > 0 ? raw / 1000 : 0;
}

function activeTradingCodes(state) {
    return Array.from(new Set((state.trades || [])
        .filter(trade => trade.status === 'active')
        .map(trade => normalizeSymbol(trade.stockCode))
        .filter(Boolean)));
}

async function refreshTradingQuotes(options = {}) {
    if (tradingRefreshing) return;
    tradingRefreshing = true;
    updateTradingRefreshStatus();
    try {
        const state = await hydrateTradingState();
        const codes = activeTradingCodes(state);
        if (codes.length === 0) {
            tradingRefreshMessage = '没有持仓股票';
            return;
        }

        const quotes = await apiFetch(`/api/quote?code=${encodeURIComponent(codes.join(','))}`);
        const quoteMap = new Map((Array.isArray(quotes) ? quotes : []).map(quote => [normalizeSymbol(quote.Code), quote]));
        let updated = 0;
        state.trades.forEach(trade => {
            if (trade.status !== 'active') return;
            const code = normalizeSymbol(trade.stockCode);
            const quote = quoteMap.get(code);
            const price = quoteClosePrice(quote);
            if (price <= 0) return;
            const previousClose = quotePrevClosePrice(quote);
            if (Number(trade.currentPrice || 0) !== price) {
                trade.currentPrice = Number(price.toFixed(3));
                updated += 1;
            }
            if (previousClose > 0) {
                trade.previousClosePrice = Number(previousClose.toFixed(3));
            }
        });
        tradingState = state;
        if (updated > 0) {
            try {
                await saveTradingState();
            } catch {
                localStorage.setItem(tradingStoreKey, JSON.stringify(tradingState));
            }
        } else {
            localStorage.setItem(tradingStoreKey, JSON.stringify(tradingState));
        }
        tradingLastRefreshAt = localTimeString();
        tradingRefreshMessage = updated > 0 ? `已更新 ${updated} 只持仓` : '行情已是最新';
        renderTradingSystem();
    } catch (error) {
        tradingRefreshMessage = `刷新失败：${error.message || error}`;
        if (options.manual) alert(tradingRefreshMessage);
    } finally {
        tradingRefreshing = false;
        tradingNextRefreshAt = Date.now() + tradingRefreshIntervalMS;
        updateTradingRefreshStatus();
    }
}

function calcTradingTrade(trade) {
    const state = loadTradingState();
    const fees = state.fees || {};
    const entryPrice = Number(trade.entryPrice || 0);
    const currentPrice = Number(trade.currentPrice || 0);
    const previousClosePrice = Number(trade.previousClosePrice || 0);
    const invalidPrice = Number(trade.invalidPrice || 0);
    const shares = Number(trade.shares || 0);
    const entryValue = entryPrice * shares;
    const marketValue = currentPrice * shares;
    const buyCommission = tradingCommission(entryValue, fees.buyCommissionRate, fees.minCommission);
    const buyTransferFee = tradingIsShanghai(trade.stockCode)
        ? entryValue * tradingRateToDecimal(fees.transferFeeRate)
        : 0;
    const sellCommission = tradingCommission(marketValue, fees.sellCommissionRate, fees.minCommission);
    const stampTax = marketValue > 0 ? marketValue * tradingRateToDecimal(fees.stampTaxRate) : 0;
    const sellTransferFee = tradingIsShanghai(trade.stockCode)
        ? marketValue * tradingRateToDecimal(fees.transferFeeRate)
        : 0;
    const transferFee = buyTransferFee + sellTransferFee;
    const feeAmount = buyCommission + sellCommission + stampTax + transferFee;
    const holdingCostValue = entryValue + buyCommission + buyTransferFee;
    const holdingCostPrice = shares ? holdingCostValue / shares : 0;
    const grossPnl = (currentPrice - entryPrice) * shares;
    const pnl = grossPnl - feeAmount;
    const pnlPct = entryValue ? (pnl / entryValue) * 100 : 0;
    const latestVsEntry = currentPrice - entryPrice;
    const latestVsHolding = currentPrice - holdingCostPrice;
    const latestVsPrevClose = previousClosePrice > 0 ? currentPrice - previousClosePrice : 0;
    const riskPerShare = Math.max(entryPrice - invalidPrice, 0);
    const riskAmount = riskPerShare * shares;
    const totalAssets = Number(state.account.totalAssets || 0);
    const riskPct = totalAssets ? (riskAmount / totalAssets) * 100 : 0;
    const weight = totalAssets ? (marketValue / totalAssets) * 100 : 0;
    return {
        entryValue,
        marketValue,
        holdingCostValue,
        holdingCostPrice,
        grossPnl,
        pnl,
        pnlPct,
        latestVsEntry,
        latestVsHolding,
        latestVsPrevClose,
        feeAmount,
        buyCommission,
        buyTransferFee,
        sellCommission,
        stampTax,
        sellTransferFee,
        transferFee,
        riskPerShare,
        riskAmount,
        riskPct,
        weight
    };
}

function tradingRiskClass(calc) {
    const state = loadTradingState();
    const maxRisk = Number(state.account.maxTradeRisk || 0);
    if (!maxRisk) return 'trading-risk-watch';
    if (calc.riskPct <= maxRisk * 0.7) return 'trading-risk-safe';
    if (calc.riskPct <= maxRisk) return 'trading-risk-watch';
    return 'trading-risk-bad';
}

function renderTradingAccount() {
    const state = loadTradingState();
    const fields = {
        tradingPrincipal: state.account.principal,
        tradingTotalAssets: state.account.totalAssets,
        tradingMarketValue: state.account.marketValue,
        tradingDailyProfit: state.account.dailyProfit,
        tradingMaxTradeRisk: state.account.maxTradeRisk,
        tradingMaxPositionWeight: state.account.maxPositionWeight
    };
    Object.entries(fields).forEach(([id, value]) => {
        const node = document.getElementById(id);
        if (node) node.value = value ?? '';
    });
}

function renderTradingFees() {
    const state = loadTradingState();
    const fields = {
        tradingBuyCommissionRate: state.fees.buyCommissionRate,
        tradingSellCommissionRate: state.fees.sellCommissionRate,
        tradingStampTaxRate: state.fees.stampTaxRate,
        tradingTransferFeeRate: state.fees.transferFeeRate,
        tradingMinCommission: state.fees.minCommission
    };
    Object.entries(fields).forEach(([id, value]) => {
        const node = document.getElementById(id);
        if (node) node.value = value ?? '';
    });
}

function renderTradingStats() {
    const state = loadTradingState();
    const activeTrades = state.trades.filter(trade => trade.status === 'active');
    const totals = activeTrades.reduce((acc, trade) => {
        const calc = calcTradingTrade(trade);
        acc.position += calc.marketValue;
        acc.risk += calc.riskAmount;
        acc.pnl += calc.pnl;
        acc.entry += calc.entryValue;
        return acc;
    }, { position: 0, risk: 0, pnl: 0, entry: 0 });
    const assets = Number(state.account.totalAssets || 0);
    const marketValue = Number(state.account.marketValue || 0);
    const dailyProfit = Number(state.account.dailyProfit || 0);
    const principal = Number(state.account.principal || 0);
    const cash = assets - (marketValue || totals.position);
    const realized = assets - principal;
    const riskPct = assets ? (totals.risk / assets) * 100 : 0;
    const pnlPct = totals.entry ? (totals.pnl / totals.entry) * 100 : 0;

    const statAssets = document.getElementById('tradingStatAssets');
    const statProfit = document.getElementById('tradingStatProfit');
    const statPosition = document.getElementById('tradingStatPosition');
    const statCash = document.getElementById('tradingStatCash');
    const statRisk = document.getElementById('tradingStatRisk');
    const statRiskPct = document.getElementById('tradingStatRiskPct');
    const statPnl = document.getElementById('tradingStatPnl');
    const statPnlPct = document.getElementById('tradingStatPnlPct');

    if (statAssets) statAssets.textContent = `¥${tradingMoney(assets)}`;
    if (statProfit) {
        statProfit.textContent = `累计盈亏：${realized >= 0 ? '+' : ''}¥${tradingMoney(realized)}`;
        statProfit.className = `metric-note ${realized >= 0 ? 'trading-positive' : 'trading-negative'}`;
    }
    if (statPosition) statPosition.textContent = `¥${tradingMoney(marketValue || totals.position)}`;
    if (statCash) statCash.textContent = `现金估算：¥${tradingMoney(cash)}`;
    if (statRisk) statRisk.textContent = `¥${tradingMoney(totals.risk)}`;
    if (statRiskPct) statRiskPct.textContent = `占账户：${tradingPercent(riskPct)}`;
    if (statPnl) {
        statPnl.textContent = `${totals.pnl >= 0 ? '+' : ''}¥${tradingMoney(totals.pnl)}`;
        statPnl.className = `metric-value ${totals.pnl >= 0 ? 'trading-positive' : 'trading-negative'}`;
    }
    if (statPnlPct) statPnlPct.textContent = `持仓净盈亏率：${pnlPct >= 0 ? '+' : ''}${tradingPercent(pnlPct)}`;
}

function renderTradingDiscipline() {
    const state = loadTradingState();
    const box = document.getElementById('tradingDisciplineBox');
    if (!box) return;
    const items = [
        ['reason', '买入理由写清楚'],
        ['invalid', '技术无效点写清楚'],
        ['risk', '最大亏损能接受'],
        ['noImpulse', '没有因为怕踏空追买']
    ];
    box.innerHTML = `
        <h4>开仓前四问</h4>
        ${items.map(([key, label]) => `
            <label class="trading-check">
                <input type="checkbox" data-trading-discipline="${key}" ${state.discipline[key] ? 'checked' : ''}>
                <span>${escapeHTML(label)}</span>
            </label>
        `).join('')}
    `;
    box.querySelectorAll('[data-trading-discipline]').forEach(input => {
        input.addEventListener('change', () => {
            state.discipline[input.dataset.tradingDiscipline] = input.checked;
            saveTradingState();
        });
    });
}

function renderTradingDetail(title, text, open = false, meta = '') {
    const count = tradingLineCount(text);
    return `
        <details class="trading-detail" ${open ? 'open' : ''}>
            <summary>
                <span>${escapeHTML(title)}</span>
                <em>${escapeHTML(meta || `${count} 条内容`)}</em>
            </summary>
            <div class="trading-detail-body">${escapeHTML(text || '未填写')}</div>
        </details>
    `;
}

function renderTradingCards() {
    const state = loadTradingState();
    const list = document.getElementById('tradingList');
    const empty = document.getElementById('tradingEmptyState');
    if (!list) return;
    const search = (document.getElementById('tradingSearch')?.value || '').trim().toLowerCase();
    const visibleTrades = state.trades.filter(trade => {
        const matchesFilter = state.filter === 'all' || trade.status === state.filter;
        const haystack = `${trade.stockName || ''} ${trade.stockCode || ''}`.toLowerCase();
        return matchesFilter && haystack.includes(search);
    });
    if (empty) empty.classList.toggle('hidden', visibleTrades.length > 0);
    list.innerHTML = visibleTrades.map(trade => {
        const calc = calcTradingTrade(trade);
        const pnlClass = calc.pnl >= 0 ? 'trading-positive' : 'trading-negative';
        const cardClass = calc.pnl >= 0 ? 'trading-card-positive' : 'trading-card-negative';
        const entryClass = tradingTrendClass(calc.latestVsEntry);
        const holdingClass = tradingTrendClass(calc.latestVsHolding);
        const prevCloseClass = tradingTrendClass(calc.latestVsPrevClose);
        const statusText = trade.status === 'active' ? '持仓' : '已清仓';
        const reasonCount = tradingLineCount(trade.buyReason);
        const exitCount = tradingLineCount(trade.exitRules);
        const reviewCount = tradingLineCount(trade.review);
        return `
            <article class="trading-card ${cardClass}">
                <div class="trading-card-head">
                    <div class="trading-stock-title">
                        <div class="trading-title-row">
                            <strong>${escapeHTML(trade.stockName || '--')} <span>${escapeHTML(trade.stockCode || '--')}</span></strong>
                            <span class="trading-hero-chip state-${trade.status}">${statusText}</span>
                        </div>
                        <div class="trading-meta-row">
                            <span>${escapeHTML(trade.entryDate || '--')}</span>
                            <span>${escapeHTML(trade.positionLabel || '未标记')}</span>
                            <span>${escapeHTML(tradingSnippet(trade.tradeMode || '未填写', 1))}</span>
                        </div>
                    </div>
                    <div class="trading-head-stats">
                        <div class="trading-head-row">
                            <div class="trading-num">
                                <span class="trading-mini-label">持仓成本</span>
                                <b class="${holdingClass}">${tradingPrice(calc.holdingCostPrice)}</b>
                            </div>
                            <div class="trading-num">
                                <span class="trading-mini-label">最新价</span>
                                <b class="${prevCloseClass}">${tradingPrice(trade.currentPrice)}</b>
                            </div>
                            <div class="trading-num">
                                <span class="trading-mini-label">持仓数量</span>
                                <b>${tradingQuantity(trade.shares)} 股</b>
                            </div>
                            <div class="trading-num">
                                <span class="trading-mini-label">持仓市值</span>
                                <b>¥${tradingMoney(calc.marketValue)}</b>
                            </div>
                        </div>
                        <div class="trading-head-divider" aria-hidden="true"></div>
                        <div class="trading-head-row">
                            <div class="trading-num">
                                <span class="trading-mini-label">仓位占比</span>
                                <b>${tradingPercent(calc.weight)}</b>
                            </div>
                            <div class="trading-num">
                                <span class="trading-mini-label">浮动盈亏</span>
                                <b class="${pnlClass}">${calc.pnl >= 0 ? '+' : ''}¥${tradingMoney(calc.pnl)}</b>
                            </div>
                            <div class="trading-num">
                                <span class="trading-mini-label">盈亏率</span>
                                <b class="${pnlClass}">${calc.pnlPct >= 0 ? '+' : ''}${tradingPercent(calc.pnlPct)}</b>
                            </div>
                            <div class="trading-num trading-num-empty" aria-hidden="true"></div>
                        </div>
                    </div>
                    <div class="item-actions compact-actions">
                        <button type="button" onclick="openTradingDialog('${escapeJSString(trade.id)}')">编辑</button>
                        <button type="button" onclick="deleteTradingTrade('${escapeJSString(trade.id)}')">删除</button>
                    </div>
                </div>
                <div class="trading-card-body">
                    <div class="trading-left-column">
                        <div class="trading-plan-grid">
                            <div class="trading-pill">
                                <span>当前 / 净盈亏</span>
                                <b>${tradingPrice(trade.currentPrice)} / ${calc.pnl >= 0 ? '+' : ''}¥${tradingMoney(calc.pnl)}</b>
                                <span class="trading-pill-foot">费用 ¥${tradingMoney(calc.feeAmount)}</span>
                            </div>
                            <div class="trading-pill">
                                <span>技术无效点</span>
                                <b>${tradingMoney(trade.invalidPrice)} · 风险 ¥${tradingMoney(calc.riskAmount)}</b>
                            </div>
                            <div class="trading-pill">
                                <span>观察 / 压力</span>
                                <b>${escapeHTML(trade.targetOne || '--')} / ${escapeHTML(trade.targetTwo || '--')}</b>
                            </div>
                        </div>
                        <div class="trading-summary-strip">
                            <div class="trading-summary-block">
                                <span>交易模式</span>
                                <p>${escapeHTML(trade.tradeMode || '未填写')}</p>
                            </div>
                            <div class="trading-summary-block">
                                <span>关键判断</span>
                                <p>${escapeHTML(tradingSnippet(trade.buyReason || '未填写', 2))}</p>
                            </div>
                        </div>
                    </div>
                    <div class="trading-right-column">
                        ${renderTradingDetail('买入理由', trade.buyReason, false, `${reasonCount} 条`)}
                        ${renderTradingDetail('退出 / 加仓规则', trade.exitRules, false, `${exitCount} 条`)}
                        ${renderTradingDetail('盘后复盘', trade.review, false, `${reviewCount} 条`)}
                    </div>
                </div>
            </article>
        `;
    }).join('');
}

function renderTradingSystem() {
    const state = loadTradingState();
    renderTradingAccount();
    renderTradingFees();
    renderTradingStats();
    renderTradingDiscipline();
    renderTradingCards();
    document.querySelectorAll('[data-trading-filter]').forEach(button => {
        button.classList.toggle('active', button.dataset.tradingFilter === state.filter);
    });
    updateTradingRefreshStatus();
}

function saveTradingAccount() {
    const state = loadTradingState();
    state.account = {
        principal: tradingNumberInput('tradingPrincipal'),
        totalAssets: tradingNumberInput('tradingTotalAssets'),
        marketValue: tradingNumberInput('tradingMarketValue'),
        dailyProfit: tradingNumberInput('tradingDailyProfit'),
        maxTradeRisk: tradingNumberInput('tradingMaxTradeRisk'),
        maxPositionWeight: tradingNumberInput('tradingMaxPositionWeight')
    };
    state.fees = {
        buyCommissionRate: tradingNumberInput('tradingBuyCommissionRate'),
        sellCommissionRate: tradingNumberInput('tradingSellCommissionRate'),
        stampTaxRate: tradingNumberInput('tradingStampTaxRate'),
        transferFeeRate: tradingNumberInput('tradingTransferFeeRate'),
        minCommission: tradingNumberInput('tradingMinCommission')
    };
    saveTradingState().then(renderTradingSystem).catch(error => alert(error.message || '保存失败'));
}

function bindTradingSystem() {
    loadTradingState();
    startTradingAutoRefresh();
    document.querySelectorAll('[data-trading-filter]').forEach(button => {
        button.addEventListener('click', () => {
            const state = loadTradingState();
            state.filter = button.dataset.tradingFilter;
            saveTradingState().then(renderTradingSystem).catch(error => alert(error.message || '保存失败'));
        });
    });
    ['tradingPrincipal', 'tradingTotalAssets', 'tradingMarketValue', 'tradingDailyProfit', 'tradingMaxTradeRisk', 'tradingMaxPositionWeight', 'tradingBuyCommissionRate', 'tradingSellCommissionRate', 'tradingStampTaxRate', 'tradingTransferFeeRate', 'tradingMinCommission'].forEach(id => {
        const node = document.getElementById(id);
        if (node) node.addEventListener('change', saveTradingAccount);
    });
    const search = document.getElementById('tradingSearch');
    if (search) search.addEventListener('input', renderTradingCards);
    const importFile = document.getElementById('tradingImportFile');
    if (importFile) importFile.addEventListener('change', importTradingData);
    bindTradingSymbolSearch();
    const form = document.getElementById('tradingForm');
    if (form) {
        form.addEventListener('submit', event => {
            event.preventDefault();
            saveTradingTradeFromForm();
        });
    }
}

function tradingSymbolSearchField(field) {
    return field === 'code'
        ? {
            input: document.getElementById('tradingStockCode'),
            suggestions: document.getElementById('tradingStockCodeSuggestions'),
            otherInput: document.getElementById('tradingStockName')
        }
        : {
            input: document.getElementById('tradingStockName'),
            suggestions: document.getElementById('tradingStockNameSuggestions'),
            otherInput: document.getElementById('tradingStockCode')
        };
}

function bindTradingSymbolSearch() {
    ['name', 'code'].forEach(field => {
        const { input, suggestions } = tradingSymbolSearchField(field);
        if (!input || !suggestions || input.dataset.symbolSearchBound === 'true') return;
        input.dataset.symbolSearchBound = 'true';
        input.addEventListener('input', () => scheduleTradingSymbolSearch(field));
        input.addEventListener('keydown', event => handleTradingSymbolSearchKeydown(event, field));
        input.addEventListener('focus', () => {
            const state = tradingSymbolSearchState[field];
            if (state.results.length) showTradingSymbolSuggestions(field);
        });
        input.addEventListener('blur', () => {
            window.setTimeout(() => hideTradingSymbolSuggestions(field), 180);
        });
    });
}

function scheduleTradingSymbolSearch(field) {
    const state = tradingSymbolSearchState[field];
    const { input, otherInput } = tradingSymbolSearchField(field);
    if (!state || !input) return;
    if (state.timer) window.clearTimeout(state.timer);
    state.requestID += 1;
    state.results = [];
    state.activeIndex = -1;
    hideTradingSymbolSuggestions(field);
    if (otherInput && otherInput.value.trim()) otherInput.value = '';
    const keyword = input.value.trim();
    if (!keyword) return;
    state.timer = window.setTimeout(() => searchTradingSymbols(field, keyword, state.requestID), 260);
}

async function searchTradingSymbols(field, keyword, requestID) {
    const state = tradingSymbolSearchState[field];
    const { input } = tradingSymbolSearchField(field);
    if (!state || !input || requestID !== state.requestID || input.value.trim() !== keyword) return;
    try {
        const data = await apiFetch(`/api/search?keyword=${encodeURIComponent(keyword)}`);
        if (requestID !== state.requestID || input.value.trim() !== keyword) return;
        state.results = Array.isArray(data) ? data.slice(0, 12) : [];
        state.activeIndex = -1;
        renderTradingSymbolSuggestions(field);
        if (field === 'code') {
            const exact = state.results.find(item => normalizeTradingSymbolCode(item.code) === normalizeTradingSymbolCode(keyword));
            if (exact) fillTradingSymbol(exact);
        }
    } catch (error) {
        if (requestID !== state.requestID) return;
        state.results = [];
        state.activeIndex = -1;
        renderTradingSymbolSuggestions(field, error.message || '搜索失败');
    }
}

function normalizeTradingSymbolCode(value) {
    return String(value || '').trim().toUpperCase().replace(/^(SH|SZ|BJ)/, '');
}

function tradingSymbolExchangeLabel(exchange) {
    switch (String(exchange || '').toLowerCase()) {
        case 'sh': return '沪市';
        case 'sz': return '深市';
        case 'bj': return '北交所';
        default: return String(exchange || '').toUpperCase();
    }
}

function renderTradingSymbolSuggestions(field, message = '') {
    const state = tradingSymbolSearchState[field];
    const { suggestions } = tradingSymbolSearchField(field);
    if (!state || !suggestions) return;
    if (message) {
        suggestions.innerHTML = `<div class="trading-symbol-empty">${escapeHTML(message)}</div>`;
        suggestions.hidden = false;
        return;
    }
    if (!state.results.length) {
        suggestions.innerHTML = '<div class="trading-symbol-empty">未找到匹配的股票或 ETF</div>';
        suggestions.hidden = false;
        return;
    }
    suggestions.innerHTML = state.results.map((item, index) => `
        <button type="button" class="trading-symbol-option ${index === state.activeIndex ? 'active' : ''}" role="option" aria-selected="${index === state.activeIndex ? 'true' : 'false'}" data-trading-symbol-index="${index}">
            <strong>${escapeHTML(item.name || '--')}</strong>
            <span>${escapeHTML(item.code || '--')} · ${escapeHTML(tradingSymbolExchangeLabel(item.exchange))}${item.type ? ` · ${escapeHTML(item.type)}` : ''}</span>
        </button>
    `).join('');
    suggestions.hidden = false;
    suggestions.querySelectorAll('[data-trading-symbol-index]').forEach(button => {
        button.addEventListener('mousedown', event => event.preventDefault());
        button.addEventListener('click', () => {
            const item = state.results[Number(button.dataset.tradingSymbolIndex)];
            if (item) fillTradingSymbol(item);
        });
    });
}

function showTradingSymbolSuggestions(field) {
    const { suggestions } = tradingSymbolSearchField(field);
    if (suggestions && tradingSymbolSearchState[field].results.length) suggestions.hidden = false;
}

function hideTradingSymbolSuggestions(field) {
    const { suggestions } = tradingSymbolSearchField(field);
    if (suggestions) suggestions.hidden = true;
}

function fillTradingSymbol(item) {
    const nameInput = document.getElementById('tradingStockName');
    const codeInput = document.getElementById('tradingStockCode');
    if (nameInput) nameInput.value = item.name || '';
    if (codeInput) codeInput.value = item.code || '';
    const currentPriceInput = document.getElementById('tradingCurrentPrice');
    const targetOneInput = document.getElementById('tradingTargetOne');
    const targetTwoInput = document.getElementById('tradingTargetTwo');
    if (currentPriceInput) currentPriceInput.value = '';
    if (targetOneInput) targetOneInput.value = '';
    if (targetTwoInput) targetTwoInput.value = '';
    hideTradingSymbolSuggestions('name');
    hideTradingSymbolSuggestions('code');
    tradingSymbolSearchState.name.results = [];
    tradingSymbolSearchState.code.results = [];
    tradingSymbolSearchState.name.activeIndex = -1;
    tradingSymbolSearchState.code.activeIndex = -1;
    loadTradingTradeAssist(item).catch(() => {});
}

function setTradingAssistStatus(message, type = '') {
    const node = document.getElementById('tradingAssistStatus');
    if (!node) return;
    node.textContent = message || '';
    node.className = `trading-assist-status ${type}`.trim();
}

function tradingQuotePrice(quote) {
    const raw = Number(quote?.K?.Close ?? quote?.K?.Last ?? 0);
    return raw > 0 ? raw / 1000 : 0;
}

function tradingPriceText(value) {
    const price = Number(value);
    return Number.isFinite(price) && price > 0 ? price.toFixed(2) : '';
}

function tradingLevelText(value) {
    if (value === null || value === undefined) return '';
    if (typeof value === 'object' && !Array.isArray(value)) {
        const lower = value.lower ?? value.low ?? value.min ?? value.from;
        const upper = value.upper ?? value.high ?? value.max ?? value.to;
        if (lower !== undefined || upper !== undefined) {
            return tradingLevelText([lower, upper].filter(item => item !== undefined));
        }
    }
    const values = Array.isArray(value)
        ? value.flatMap(item => {
            const match = String(item ?? '').match(/\d+(?:\.\d+)?/g);
            return match ? match.map(Number) : [];
        })
        : String(value).match(/\d+(?:\.\d+)?/g)?.map(Number) || [];
    const prices = values.filter(item => Number.isFinite(item) && item > 0).slice(0, 2);
    if (prices.length >= 2) {
        const low = Math.min(prices[0], prices[1]);
        const high = Math.max(prices[0], prices[1]);
        return `${low.toFixed(2)}-${high.toFixed(2)}`;
    }
    return prices.length === 1 ? prices[0].toFixed(2) : '';
}

function tradingAILevelResult(data) {
    let result = data?.result;
    if (!result || typeof result !== 'object' || !Object.keys(result).length) {
        try {
            result = JSON.parse(data?.content || '{}');
        } catch (error) {
            result = {};
        }
    }
    const first = tradingLevelText(
        result.first_observation_level
        ?? result.first_observation
        ?? result.first_pressure_level
        ?? result.target_one
        ?? result.watch_level
    );
    const strong = tradingLevelText(
        result.strong_pressure_level
        ?? result.strong_pressure
        ?? result.take_profit_level
        ?? result.target_two
        ?? result.profit_zone
    );
    return { first, strong, basis: result.level_basis || result.reason || result.summary || '' };
}

function tradingFallbackLevels(currentPrice, context) {
    const item = Array.isArray(context?.items) ? context.items[0] : null;
    const list = Array.isArray(item?.kline_day?.List) ? item.kline_day.List : [];
    const highs = list
        .slice(-60)
        .map(row => Number(row?.High || row?.Close || 0) / 1000)
        .filter(price => Number.isFinite(price) && price > currentPrice * 1.002);
    const recentHighs = highs.slice(-20);
    const firstPivot = recentHighs.length ? Math.min(...recentHighs) : currentPrice * 1.03;
    const allHigh = highs.length ? Math.max(...highs) : currentPrice * 1.06;
    const strongPivot = allHigh > firstPivot * 1.01 ? allHigh : Math.max(currentPrice * 1.06, firstPivot * 1.02);
    const range = pivot => `${(pivot * 0.995).toFixed(2)}-${(pivot * 1.005).toFixed(2)}`;
    return {
        first: range(firstPivot),
        strong: range(strongPivot)
    };
}

async function loadTradingTradeAssist(item) {
    const code = normalizeTradingSymbolCode(item?.code);
    if (!code) return;
    const requestID = ++tradingAssistRequestID;
    const currentPriceInput = document.getElementById('tradingCurrentPrice');
    const targetOneInput = document.getElementById('tradingTargetOne');
    const targetTwoInput = document.getElementById('tradingTargetTwo');
    setTradingAssistStatus('正在查询最新价，并准备分析观察区间...', 'loading');

    let currentPrice = 0;
    let quoteError = '';
    try {
        const quotes = await apiFetch(`/api/quote?code=${encodeURIComponent(code)}`);
        const quote = Array.isArray(quotes) ? quotes[0] : null;
        currentPrice = tradingQuotePrice(quote);
        if (requestID !== tradingAssistRequestID) return;
        if (currentPrice > 0 && currentPriceInput) currentPriceInput.value = tradingPriceText(currentPrice);
    } catch (error) {
        quoteError = error.message || String(error);
    }

    if (requestID !== tradingAssistRequestID) return;
    let context = null;
    try {
        context = await apiFetch(`/api/analysis/context?codes=${encodeURIComponent(code)}&kline_limit=60&news_limit=0`);
    } catch (error) {
        context = null;
    }

    let aiCredential = (Array.isArray(aiCredentials) ? aiCredentials : [])
        .find(credential => credential && credential.enabled !== false && credential.has_api_key && credential.id);
    let aiError = '';
    if (!aiCredential) {
        try {
            await loadAIConfigData();
            aiCredential = (Array.isArray(aiCredentials) ? aiCredentials : [])
                .find(credential => credential && credential.enabled !== false && credential.has_api_key && credential.id);
        } catch (error) {
            aiError = error.message || String(error);
        }
    }

    if (aiCredential) {
        try {
            const data = await apiFetch('/api/ai/analyze/stock', {
                method: 'POST',
                body: JSON.stringify({
                    provider: aiCredential.provider,
                    model: aiCredential.model || defaultAIModel(aiCredential.provider),
                    credential_id: aiCredential.id,
                    symbol: code,
                    input: {
                        task: 'trading_levels',
                        name: item.name || '',
                        current_price: currentPrice,
                        instruction: '请结合实时行情和近期日K，给出两个可执行的价格区间：第一观察/压力位，以及强压力/止盈区。只返回明确价格或价格区间，不要返回百分比，不要编造数据。'
                    },
                    options: { max_tokens: 700 }
                })
            });
            if (requestID !== tradingAssistRequestID) return;
            const levels = tradingAILevelResult(data);
            if (levels.first && levels.strong) {
                if (targetOneInput) targetOneInput.value = levels.first;
                if (targetTwoInput) targetTwoInput.value = levels.strong;
                const suffix = levels.basis ? ` · ${levels.basis}` : '';
                setTradingAssistStatus(`最新价 ${tradingPriceText(currentPrice) || '--'} · AI 已生成价格区间${suffix}`, 'success');
                return;
            }
            aiError = 'AI 未返回有效价格区间';
        } catch (error) {
            aiError = error.message || String(error);
        }
    }

    const fallback = tradingFallbackLevels(currentPrice, context);
    if (requestID !== tradingAssistRequestID) return;
    if (targetOneInput) targetOneInput.value = fallback.first;
    if (targetTwoInput) targetTwoInput.value = fallback.strong;
    const priceLabel = currentPrice > 0 ? `最新价 ${tradingPriceText(currentPrice)}` : '最新价获取失败';
    const reason = aiCredential
        ? `AI 分析失败，已按近期高点生成区间${aiError ? `：${aiError}` : ''}`
        : `未配置可用 AI，已按近期高点生成区间${aiError ? `：${aiError}` : ''}`;
    setTradingAssistStatus(`${priceLabel} · ${reason}${quoteError ? ` · ${quoteError}` : ''}`, currentPrice > 0 ? 'warning' : 'error');
}

function handleTradingSymbolSearchKeydown(event, field) {
    const state = tradingSymbolSearchState[field];
    if (!state.results.length) {
        if (event.key === 'Escape') hideTradingSymbolSuggestions(field);
        return;
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault();
        const direction = event.key === 'ArrowDown' ? 1 : -1;
        state.activeIndex = (state.activeIndex + direction + state.results.length) % state.results.length;
        renderTradingSymbolSuggestions(field);
        return;
    }
    if (event.key === 'Enter' && state.activeIndex >= 0) {
        event.preventDefault();
        fillTradingSymbol(state.results[state.activeIndex]);
        return;
    }
    if (event.key === 'Escape') hideTradingSymbolSuggestions(field);
}

function openTradingDialog(id = '') {
    const state = loadTradingState();
    const trade = id ? state.trades.find(item => item.id === id) : null;
    const dialog = document.getElementById('tradingDialog');
    if (!dialog) return;
    document.getElementById('tradingDialogTitle').textContent = trade ? '编辑交易' : '新建交易';
    document.getElementById('tradingTradeId').value = trade?.id || '';
    document.getElementById('tradingStockName').value = trade?.stockName || '';
    document.getElementById('tradingStockCode').value = trade?.stockCode || '';
    hideTradingSymbolSuggestions('name');
    hideTradingSymbolSuggestions('code');
    tradingSymbolSearchState.name.results = [];
    tradingSymbolSearchState.code.results = [];
    tradingSymbolSearchState.name.activeIndex = -1;
    tradingSymbolSearchState.code.activeIndex = -1;
    tradingAssistRequestID += 1;
    setTradingAssistStatus('');
    document.getElementById('tradingTradeStatus').value = trade?.status || 'active';
    document.getElementById('tradingEntryDate').value = trade?.entryDate || localDateString();
    document.getElementById('tradingEntryPrice').value = trade?.entryPrice ?? '';
    document.getElementById('tradingShares').value = trade?.shares ?? '';
    document.getElementById('tradingCurrentPrice').value = trade?.currentPrice ?? '';
    document.getElementById('tradingInvalidPrice').value = trade?.invalidPrice ?? '';
    document.getElementById('tradingPositionLabel').value = trade?.positionLabel || '试错仓';
    document.getElementById('tradingTargetOne').value = trade?.targetOne || '';
    document.getElementById('tradingTargetTwo').value = trade?.targetTwo || '';
    document.getElementById('tradingTradeMode').value = trade?.tradeMode || '';
    document.getElementById('tradingBuyReason').value = trade?.buyReason || '';
    document.getElementById('tradingExitRules').value = trade?.exitRules || '';
    document.getElementById('tradingReview').value = trade?.review || '';
    dialog.classList.add('open');
    dialog.setAttribute('aria-hidden', 'false');
    setTimeout(() => document.getElementById('tradingStockName')?.focus(), 50);
}

function closeTradingDialog() {
    const dialog = document.getElementById('tradingDialog');
    if (!dialog) return;
    dialog.classList.remove('open');
    dialog.setAttribute('aria-hidden', 'true');
}

function saveTradingTradeFromForm() {
    const state = loadTradingState();
    const id = document.getElementById('tradingTradeId').value || `trade-${Date.now()}`;
    const trade = {
        id,
        stockName: document.getElementById('tradingStockName').value.trim(),
        stockCode: document.getElementById('tradingStockCode').value.trim(),
        status: document.getElementById('tradingTradeStatus').value,
        entryDate: document.getElementById('tradingEntryDate').value,
        entryPrice: tradingNumberInput('tradingEntryPrice'),
        shares: tradingNumberInput('tradingShares'),
        currentPrice: tradingNumberInput('tradingCurrentPrice'),
        invalidPrice: tradingNumberInput('tradingInvalidPrice'),
        positionLabel: document.getElementById('tradingPositionLabel').value,
        targetOne: document.getElementById('tradingTargetOne').value.trim(),
        targetTwo: document.getElementById('tradingTargetTwo').value.trim(),
        tradeMode: document.getElementById('tradingTradeMode').value.trim(),
        buyReason: document.getElementById('tradingBuyReason').value.trim(),
        exitRules: document.getElementById('tradingExitRules').value.trim(),
        review: document.getElementById('tradingReview').value.trim()
    };
    const idx = state.trades.findIndex(item => item.id === id);
    if (idx >= 0) state.trades[idx] = trade;
    else state.trades.unshift(trade);
    saveTradingState()
        .then(() => {
            closeTradingDialog();
            renderTradingSystem();
        })
        .catch(error => alert(error.message || '保存失败'));
}

function deleteTradingTrade(id) {
    const state = loadTradingState();
    const trade = state.trades.find(item => item.id === id);
    if (!confirm(`删除 ${trade?.stockName || '这笔交易'} 的交易卡？`)) return;
    state.trades = state.trades.filter(item => item.id !== id);
    saveTradingState().then(renderTradingSystem).catch(error => alert(error.message || '保存失败'));
}

function exportTradingData() {
    const state = loadTradingState();
    const blob = new Blob([JSON.stringify(state, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `trading-system-${localDateString()}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
}

function importTradingData(event) {
    const file = event.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
        try {
            const imported = JSON.parse(String(reader.result || '{}'));
            if (!imported.account || !Array.isArray(imported.trades)) throw new Error('bad shape');
            tradingState = {
                ...normalizeTradingState(imported)
            };
            saveTradingState()
                .then(renderTradingSystem)
                .catch(error => alert(error.message || '保存失败'));
        } catch (error) {
            alert('导入失败：请选择交易系统导出的 JSON 文件。');
        } finally {
            event.target.value = '';
        }
    };
    reader.readAsText(file);
}

function formatPercent(value) {
    if (value === null || value === undefined || value === '' || Number.isNaN(Number(value))) return '--';
    const num = Number(value);
    return `${num > 0 ? '+' : ''}${num.toFixed(2)}%`;
}

function formatDecisionTime(value) {
    if (!value) return '--';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return date.toLocaleString('zh-CN', { hour12: false });
}

function openResultChart(symbol) {
    openWatchlistQuoteDialog(symbol);
}

function showResultDetail(id) {
    const item = selectionResults.find(v => v.id === id);
    if (!item) return;
    try {
        document.getElementById('formulaTestOutput').textContent = prettyJSON(JSON.parse(item.detail_json || '{}'));
    } catch {
        document.getElementById('formulaTestOutput').textContent = item.detail_json || '';
    }
    openResultChart(item.symbol);
}

async function loadWebhooks() {
    webhooks = await apiFetch('/api/webhooks') || [];
    const select = document.getElementById('automationWebhook');
    if (select) {
        select.innerHTML = webhooks.map(h => `<option value="${h.id}">${escapeHTML(h.name)}</option>`).join('');
    }
    const list = document.getElementById('webhookList');
    if (!list) return;
    list.innerHTML = webhooks.map(h => `
        <div class="data-item">
            <div class="data-item-title">${escapeHTML(h.name)}</div>
            <div class="data-item-meta">${escapeHTML(h.url)} · ${h.enabled ? '启用' : '停用'}</div>
            <div class="item-actions">
                <button onclick="fillWebhook('${h.id}')">编辑</button>
                <button class="primary" onclick="testWebhook('${h.id}')">测试</button>
                <button onclick="deleteWebhook('${h.id}')">删除</button>
            </div>
        </div>
    `).join('') || '<div class="data-item">暂无 Webhook</div>';
}

function fillWebhook(id) {
    const h = webhooks.find(item => item.id === id);
    if (!h) return;
    document.getElementById('webhookName').dataset.id = h.id;
    document.getElementById('webhookName').value = h.name;
    document.getElementById('webhookURL').value = h.url;
    document.getElementById('webhookHeaders').value = h.headers_json || '{}';
    document.getElementById('webhookEvents').value = h.events || '[]';
    document.getElementById('webhookEnabled').checked = !!h.enabled;
}

async function saveWebhook() {
    try {
        const id = document.getElementById('webhookName').dataset.id || '';
        JSON.parse(document.getElementById('webhookHeaders').value || '{}');
        JSON.parse(document.getElementById('webhookEvents').value || '[]');
        await apiFetch(id ? `/api/webhooks/${id}` : '/api/webhooks', {
            method: id ? 'PUT' : 'POST',
            body: JSON.stringify({
                id,
                name: document.getElementById('webhookName').value,
                url: document.getElementById('webhookURL').value,
                method: 'POST',
                headers_json: document.getElementById('webhookHeaders').value || '{}',
                events: document.getElementById('webhookEvents').value || '[]',
                enabled: document.getElementById('webhookEnabled').checked
            })
        });
        document.getElementById('webhookName').dataset.id = '';
        await loadWebhooks();
        alert('Webhook 已保存');
    } catch (error) {
        alert(error.message);
    }
}

async function testWebhook(id) {
    try {
        const data = await apiFetch(`/api/webhooks/${id}/test`, { method: 'POST' });
        alert(data.join('\n') || '已发送');
    } catch (error) {
        alert(error.message);
    }
}

async function deleteWebhook(id) {
    if (!confirm('确认删除这个 Webhook？')) return;
    await apiFetch(`/api/webhooks/${id}`, { method: 'DELETE' });
    await loadWebhooks();
}

let dataCenterLoaded = false;

async function loadDataCenter() {
    if (dataCenterLoaded) return;
    dataCenterLoaded = true;
    await Promise.allSettled([
        loadMarketOverview(),
        loadStockProfile(),
        loadHistoryData('tdx'),
        loadBlockData(),
        loadHikyuuData()
    ]);
}

async function loadMarketOverview() {
    setLoadingText('marketOverviewStats');
    try {
        const [status, stats, count] = await Promise.all([
            apiFetch('/api/server-status'),
            apiFetch('/api/market-stats'),
            apiFetch('/api/market-count')
        ]);
        renderMetricCards('marketOverviewStats', [
            { label: '服务状态', value: status.status || '--', note: status.connected ? '通达信已连接' : '连接异常' },
            { label: '市场证券数', value: count.total || 0, note: (count.exchanges || []).map(v => `${v.exchange}:${v.count}`).join(' · ') },
            { label: '沪市股票', value: stats.sh?.total || 0, note: `涨 ${stats.sh?.up || 0} · 跌 ${stats.sh?.down || 0} · 平 ${stats.sh?.flat || 0}` },
            { label: '深市股票', value: stats.sz?.total || 0, note: `涨 ${stats.sz?.up || 0} · 跌 ${stats.sz?.down || 0} · 平 ${stats.sz?.flat || 0}` },
            { label: '北交所', value: stats.bj?.total || 0, note: `涨 ${stats.bj?.up || 0} · 跌 ${stats.bj?.down || 0} · 平 ${stats.bj?.flat || 0}` }
        ]);
        await loadCodeDirectory();
    } catch (error) {
        setErrorText('marketOverviewStats', error);
    }
}

async function loadCodeDirectory() {
    setLoadingText('marketDirectoryOutput');
    try {
        const exchange = document.getElementById('marketCodeExchange')?.value || 'all';
        const limit = document.getElementById('marketCodeLimit')?.value || '80';
        const data = await apiFetch(`/api/codes?exchange=${encodeURIComponent(exchange)}`);
        const rows = (data.codes || []).slice(0, Number(limit) || 80);
        renderTable('marketDirectoryOutput', rows, [
            { key: 'code', label: '代码' },
            { key: 'name', label: '名称' },
            { key: 'exchange', label: '市场' }
        ]);
    } catch (error) {
        setErrorText('marketDirectoryOutput', error);
    }
}

async function loadETFDirectory() {
    setLoadingText('marketDirectoryOutput');
    try {
        const exchange = document.getElementById('marketCodeExchange')?.value || 'all';
        const limit = document.getElementById('marketCodeLimit')?.value || '80';
        const data = await apiFetch(`/api/etf?exchange=${encodeURIComponent(exchange)}&limit=${encodeURIComponent(limit)}`);
        renderTable('marketDirectoryOutput', data.list || [], [
            { key: 'code', label: '代码' },
            { key: 'name', label: '名称' },
            { key: 'exchange', label: '市场' },
            { key: 'last_price', label: '最新价' }
        ]);
    } catch (error) {
        setErrorText('marketDirectoryOutput', error);
    }
}

function syncDataStockInputs() {
    const code = document.getElementById('dataStockCode')?.value || currentStock || '';
    const historyCode = document.getElementById('historyCode');
    if (historyCode && !historyCode.value.trim()) historyCode.value = code;
    return code.trim();
}

async function loadStockProfile() {
    setLoadingText('stockProfileOutput');
    try {
        const code = syncDataStockInputs();
        if (!code) throw new Error('请输入股票代码');
        const [finance, categories, gbbq, auction] = await Promise.allSettled([
            apiFetch(`/api/finance?code=${encodeURIComponent(code)}`),
            apiFetch(`/api/company/categories?code=${encodeURIComponent(code)}`),
            apiFetch(`/api/gbbq?code=${encodeURIComponent(code)}`),
            apiFetch(`/api/call-auction?code=${encodeURIComponent(code)}`)
        ]);
        const categoryList = categories.status === 'fulfilled' ? categories.value || [] : [];
        const panels = [
            finance.status === 'fulfilled' ? renderKeyValuePanel('财务信息', finance.value) : renderJsonPanel('财务信息', finance.reason?.message || '加载失败'),
            renderJsonPanel('F10目录', categoryList.slice(0, 20)),
            renderJsonPanel('股本变迁', (gbbq.status === 'fulfilled' ? gbbq.value || [] : []).slice?.(0, 20) || gbbq.value || []),
            renderJsonPanel('集合竞价', auction.status === 'fulfilled' ? auction.value : auction.reason?.message || '加载失败')
        ];
        document.getElementById('stockProfileOutput').innerHTML = panels.join('');
    } catch (error) {
        setErrorText('stockProfileOutput', error);
    }
}

async function loadIncomeReport() {
    const output = document.getElementById('stockProfileOutput');
    if (!output) return;
    setLoadingText('stockProfileOutput');
    try {
        const code = syncDataStockInputs();
        if (!code) throw new Error('请输入股票代码');
        const startDate = document.getElementById('incomeStartDate')?.value || '';
        const days = document.getElementById('incomeDays')?.value || '';
        const data = await apiFetch(`/api/income?code=${encodeURIComponent(code)}&start_date=${encodeURIComponent(startDate)}&days=${encodeURIComponent(days)}`);
        output.innerHTML = renderJsonPanel('收益测算', data);
    } catch (error) {
        setErrorText('stockProfileOutput', error);
    }
}

async function loadHistoryData(mode) {
    setLoadingText('historyOutput');
    try {
        const code = document.getElementById('historyCode')?.value || document.getElementById('dataStockCode')?.value || '';
        if (!code.trim()) throw new Error('请输入股票代码');
        const type = document.getElementById('historyType')?.value || 'day';
        const limit = document.getElementById('historyLimit')?.value || '120';
        const date = document.getElementById('historyDate')?.value || '';
        const startDate = document.getElementById('historyStartDate')?.value || '';
        const endDate = document.getElementById('historyEndDate')?.value || '';
        let data;
        if (mode === 'ths') {
            data = await apiFetch(`/api/kline-all/ths?code=${encodeURIComponent(code)}&type=${encodeURIComponent(type)}&limit=${encodeURIComponent(limit)}`);
            renderKlineRows(data);
        } else if (mode === 'history') {
            data = await apiFetch(`/api/kline-history?code=${encodeURIComponent(code)}&type=${encodeURIComponent(type)}&limit=${encodeURIComponent(limit)}`);
            renderKlineRows(data);
        } else if (mode === 'trade') {
            data = await apiFetch(`/api/trade-history/full?code=${encodeURIComponent(code)}&start_date=${encodeURIComponent(startDate)}&end_date=${encodeURIComponent(endDate)}&limit=${encodeURIComponent(limit)}`);
            renderTradeRows(data.list || []);
        } else if (mode === 'minuteTrade') {
            data = await apiFetch(`/api/minute-trade-all?code=${encodeURIComponent(code)}&date=${encodeURIComponent(date)}`);
            renderTradeRows(data.List || data.list || []);
        } else {
            data = await apiFetch(`/api/kline-all/tdx?code=${encodeURIComponent(code)}&type=${encodeURIComponent(type)}&limit=${encodeURIComponent(limit)}`);
            renderKlineRows(data);
        }
    } catch (error) {
        setErrorText('historyOutput', error);
    }
}

function renderKlineRows(data) {
    const rows = data.list || data.List || [];
    renderTable('historyOutput', rows, [
        { key: 'time', label: '时间', value: row => row.time || row.Time },
        { key: 'open', label: '开盘', value: row => row.open ?? row.Open },
        { key: 'high', label: '最高', value: row => row.high ?? row.High },
        { key: 'low', label: '最低', value: row => row.low ?? row.Low },
        { key: 'close', label: '收盘', value: row => row.close ?? row.Close },
        { key: 'volume', label: '成交量', value: row => row.volume ?? row.Volume },
        { key: 'amount', label: '成交额', value: row => row.amount ?? row.Amount }
    ]);
}

function renderTradeRows(rows) {
    renderTable('historyOutput', rows, [
        { key: 'time', label: '时间', value: row => row.time || row.Time },
        { key: 'price', label: '价格', value: row => row.price ?? row.Price },
        { key: 'volume', label: '成交量', value: row => row.volume ?? row.Volume },
        { key: 'number', label: '笔数', value: row => row.number ?? row.Number },
        { key: 'status', label: '性质', value: row => row.status ?? row.Status }
    ]);
}

async function loadWorkdayRange() {
    setLoadingText('historyOutput');
    try {
        const startDate = document.getElementById('historyStartDate')?.value || '';
        const endDate = document.getElementById('historyEndDate')?.value || '';
        const data = await apiFetch(`/api/workday/range?start=${encodeURIComponent(startDate)}&end=${encodeURIComponent(endDate)}`);
        renderTable('historyOutput', data.list || [], [
            { key: 'iso', label: '日期' },
            { key: 'numeric', label: '数字日期' }
        ]);
    } catch (error) {
        setErrorText('historyOutput', error);
    }
}

async function loadBlockData() {
    setLoadingText('blockIndustryOutput');
    try {
        const file = document.getElementById('blockFile')?.value || 'gn';
        const withIndex = document.getElementById('blockWithIndex')?.checked ? 'true' : 'false';
        const data = await apiFetch(`/api/block?file=${encodeURIComponent(file)}&with_index=${withIndex}`);
        renderTable('blockIndustryOutput', data.list || [], [
            { key: 'Name', label: '板块' },
            { key: 'Index', label: '指数代码' },
            { key: 'Type', label: '类型' },
            { key: 'Codes', label: '成分', value: row => row.Codes || row.codes || [] }
        ]);
    } catch (error) {
        setErrorText('blockIndustryOutput', error);
    }
}

async function loadIndustryData(kind) {
    setLoadingText('blockIndustryOutput');
    try {
        const endpoints = {
            hy: '/api/tdx-hy',
            stat: '/api/tdx-stat',
            stat2: '/api/tdx-stat2',
            xgsg: '/api/xgsg'
        };
        const data = await apiFetch(endpoints[kind] || endpoints.hy);
        const rows = data.list || data.List || [];
        if (kind === 'hy') {
            renderTable('blockIndustryOutput', rows, [
                { key: 'Code', label: '代码' },
                { key: 'TdxHy', label: '通达信行业' },
                { key: 'SwHy', label: '申万行业' }
            ]);
        } else {
            const sample = rows[0] || {};
            const columns = Object.keys(sample).slice(0, 8).map(key => ({ key, label: key }));
            renderTable('blockIndustryOutput', rows, columns.length ? columns : [{ key: 'value', label: '数据', value: row => row }]);
        }
    } catch (error) {
        setErrorText('blockIndustryOutput', error);
    }
}

async function loadHQChart() {
    try {
        const symbol = document.getElementById('hqSymbol').value || '';
        if (!symbol.trim()) {
            updateHQOverlayStatus('请输入股票代码后加载图表');
            return;
        }
        const period = document.getElementById('hqPeriod').value;
        currentHQOverlay = null;
        updateHQOverlayStatus('未叠加公式');
        renderHQKLine('hqChart', symbol, period, { count: 800, pageSize: 80 });
        refreshHQFormulaWindowOptions();
    } catch (error) {
        alert(error.message || error);
    }
}

function updateHQOverlayStatus(text) {
    const node = document.getElementById('hqOverlayStatus');
    if (node) node.textContent = text;
}

window.updateHQOverlayStatus = updateHQOverlayStatus;

function formulaResultForSymbol(resp, symbol) {
    const data = resp?.data || {};
    const normalized = normalizeSymbol(symbol);
    return data[normalized] || data[normalized.toUpperCase()] || Object.values(data)[0] || {};
}

function getHQChartInstance() {
    const container = document.getElementById('hqChart');
    if (!window.TDXHQChart || !container) return null;
    return window.TDXHQChart.getChart ? window.TDXHQChart.getChart(container) : null;
}

function getHQChartFrameCount(chart) {
    return chart?.JSChartContainer?.Frame?.SubFrame?.length
        || chart?.Frame?.SubFrame?.length
        || 2;
}

function refreshHQFormulaWindowOptions() {
    const select = document.getElementById('hqFormulaWindow');
    if (!select) return;
    const currentValue = select.value || '0';
    const chart = getHQChartInstance();
    const frameCount = getHQChartFrameCount(chart);
    const count = Math.max(2, frameCount);
    select.innerHTML = Array.from({ length: count }, (_, index) => {
        const label = index === 0 ? '主图' : `副图${index}`;
        return `<option value="${index}">${label}</option>`;
    }).join('');
    select.value = Number(currentValue) < count ? currentValue : '0';
}

function buildHQFormulaIndexInfo(formula) {
    if (!formula?.script) {
        throw new Error('公式脚本为空，无法应用');
    }
    return {
        Name: formula.name || '自定义公式',
        Script: formula.script,
        Args: parseHQFormulaArgs(formula),
        YAxis: {
            ExcludeValue: !!document.getElementById('hqFormulaExcludeY')?.checked
        }
    };
}

function currentHQSymbolPeriod() {
    const symbol = document.getElementById('hqSymbol').value || '';
    const period = document.getElementById('hqPeriod').value;
    return { symbol, period };
}

async function applySelectedFormulaToHQChart() {
    const formula = selectedHQFormula();
    if (!formula) {
        alert('请先选择一个公式');
        return;
    }
    try {
        updateHQOverlayStatus('正在应用 HQChart 公式...');
        const { symbol, period } = currentHQSymbolPeriod();
        if (!symbol.trim()) throw new Error('请输入股票代码');
        let chart = getHQChartInstance();
        if (!chart) {
            renderHQKLine('hqChart', symbol, period, { count: 800, pageSize: 80 });
            refreshHQFormulaWindowOptions();
            chart = getHQChartInstance();
        }
        if (!chart) throw new Error('HQChart 图表实例未就绪');

        const mode = document.getElementById('hqFormulaApplyMode')?.value || 'overlay';
        const windowIndex = Number(document.getElementById('hqFormulaWindow')?.value || 0);
        const indexInfo = buildHQFormulaIndexInfo(formula);
        const args = indexInfo.Args || [];
        let appliedWindowIndex = windowIndex;

        if (mode === 'change') {
            if (typeof chart.ChangeScriptIndex !== 'function') throw new Error('当前 HQChart 版本不支持 ChangeScriptIndex');
            chart.ChangeScriptIndex(windowIndex, indexInfo);
        } else if (mode === 'new-window') {
            if (typeof chart.AddScriptIndexWindow !== 'function') throw new Error('当前 HQChart 版本不支持 AddScriptIndexWindow');
            chart.AddScriptIndexWindow(indexInfo, { Draw: true });
            refreshHQFormulaWindowOptions();
            appliedWindowIndex = Math.max(1, getHQChartFrameCount(chart) - 1);
        } else {
            if (typeof chart.AddOverlayIndex !== 'function') throw new Error('当前 HQChart 版本不支持 AddOverlayIndex');
            const independentY = !!document.getElementById('hqFormulaIndependentY')?.checked;
            const option = {
                Script: indexInfo.Script,
                WindowIndex: windowIndex,
                Name: indexInfo.Name,
                Args: args,
                IsShareY: !independentY
            };
            if (option.IsShareY) option.YAxis = indexInfo.YAxis;
            chart.AddOverlayIndex(option);
        }

        const resp = await apiFetch(`/api/formulas/${formula.id}/test`, {
            method: 'POST',
            body: JSON.stringify({
                symbol,
                period,
                calc_count: 500,
                out_count: 120
            })
        });
        const result = formulaResultForSymbol(resp, symbol);
        currentHQOverlay = {
            formulaID: formula.id,
            name: formula?.name || '公式',
            mode,
            windowIndex: appliedWindowIndex,
            engine: resp.engine || '',
            tickMS: resp.tick_ms || 0
        };
        document.getElementById('formulaTestOutput').textContent = prettyJSON(resp);
        const modeLabel = mode === 'change' ? '切换窗口' : (mode === 'new-window' ? '新建副图' : '叠加指标');
        const windowLabel = appliedWindowIndex === 0 ? '主图' : `副图${appliedWindowIndex}`;
        updateHQOverlayStatus(`已用 HQChart ${modeLabel}：${currentHQOverlay.name} · ${windowLabel} · ${result.hit ? '命中' : '未命中'} · ${currentHQOverlay.engine || 'engine'} ${currentHQOverlay.tickMS}ms`);
    } catch (error) {
        updateHQOverlayStatus(`应用失败：${error.message || error}`);
        alert(error.message || error);
    }
}

async function overlaySelectedFormulaOnHQChart() {
    await applySelectedFormulaToHQChart();
}

function clearHQFormulaOverlay() {
    currentHQOverlay = null;
    const { symbol, period } = currentHQSymbolPeriod();
    if (!symbol.trim()) {
        updateHQOverlayStatus('请输入股票代码后加载图表');
        return;
    }
    renderHQKLine('hqChart', symbol, period, { count: 800, pageSize: 80 });
    updateHQOverlayStatus('未叠加公式');
}

document.addEventListener('DOMContentLoaded', async () => {
    try {
        await hydrateTradingState();
        bindTradingSystem();
        renderTradingSystem();
        await refreshSystemStatus();
        setInterval(refreshSystemStatus, 30000);
        renderWatchlist();
        await Promise.all([loadFormulaList(), loadWebhooks()]);
        await loadAutomations();
        await loadRuns();
        await loadSelectionResults();
        await loadHQChart();
    } catch (error) {
        console.warn('初始化自动化页面失败:', error);
    }
});
