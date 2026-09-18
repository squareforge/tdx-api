(function () {
    const charts = new Map();
    const states = new Map();
    let nextID = 1;

    function hasHQChart() {
        return typeof window.JSChart !== 'undefined' && window.JSChart && typeof window.JSChart.Init === 'function';
    }

    function normalizeSymbol(symbol) {
        const value = String(symbol || '').trim().toLowerCase();
        if (!value) return '000001';
        const prefixed = value.match(/^(sh|sz|bj)(\d{6})$/);
        if (prefixed) return `${prefixed[2]}.${prefixed[1]}`;
        const suffixed = value.match(/^(\d{6})[._-]?(sh|sz|bj)$/);
        if (suffixed) return `${suffixed[1]}.${suffixed[2]}`;
        return value.split('.')[0];
    }

    function normalizeCode(symbol) {
        return normalizeSymbol(symbol).split('.')[0];
    }

    function toHQSymbol(symbol) {
        const raw = normalizeSymbol(symbol);
        if (/^\d{6}\.(sh|sz|bj)$/.test(raw)) return raw;
        const code = normalizeCode(symbol);
        if (code.startsWith('6') || code.startsWith('9')) return `${code}.sh`;
        if (code.startsWith('8') || code.startsWith('4')) return `${code}.bj`;
        return `${code}.sz`;
    }

    function periodToHQChart(period) {
        switch (String(period || '').toLowerCase()) {
            case 'week':
                return 1;
            case 'month':
                return 2;
            case 'minute1':
            case '1m':
                return 4;
            case 'minute5':
            case '5m':
                return 5;
            case 'minute15':
            case '15m':
                return 6;
            case 'minute30':
            case '30m':
                return 7;
            case 'minute60':
            case 'hour':
            case '60m':
                return 8;
            default:
                return 0;
        }
    }

    function isKLineHistoryRequest(name) {
        return [
            'KLineChartContainer::RequestHistoryData',
            'KLineChartContainer::ReqeustHistoryMinuteData',
            'KLineChartContainer::RequestHistoryMinuteData',
            'KLineChartContainer::RequestHistoryPageData',
            'KLineChartContainer::RequestDragDayData',
            'KLineChartContainer::RequestDragMinuteData',
            'KLineChartContainer::RequestZoomDayData',
            'KLineChartContainer::RequestZoomMinuteData'
        ].includes(name);
    }

    function isMinuteRequest(name) {
        return [
            'MinuteChartContainer::RequestMinuteData',
            'MinuteChartContainer::RequestHistoryMinuteData'
        ].includes(name);
    }

    function resolveHistoryLimit(req, state) {
        const requested = Number(req && req.count) || 0;
        const configured = Number(state && state.count) || 0;
        const period = String(state && state.period || '').toLowerCase();
        if (period.startsWith('minute') || period === 'hour') {
            return Math.max(requested, configured, 240);
        }
        return requested || configured || 800;
    }

    function chartKey(container) {
        if (!container) return '';
        if (!container.dataset.hqChartKey) {
            container.dataset.hqChartKey = container.id || `hq-chart-${nextID++}`;
        }
        return container.dataset.hqChartKey;
    }

    // macOS trackpad pinch gestures arrive as small ctrl+wheel deltas in Chromium.
    // HQChart filters those deltas out, so normalize them before handing them to
    // the chart's native wheel zoom implementation.
    const pinchZoomListeners = new Map();
    const pinchZoomStepInterval = 55;

    function invokeNativeZoom(chart, direction, event) {
        const chartContainer = chart && (chart.JSChartContainer || chart);
        if (!chartContainer || typeof chartContainer.OnWheel !== 'function') return false;
        const deltaY = direction < 0 ? -100 : 100;
        chartContainer.OnWheel({
            clientX: event?.clientX || 0,
            clientY: event?.clientY || 0,
            ctrlKey: true,
            deltaY,
            wheelDelta: deltaY < 0 ? 120 : -120,
            preventDefault() {},
            stopPropagation() {}
        });
        return true;
    }

    function installPinchZoom(container, key, chart) {
        const previous = pinchZoomListeners.get(key);
        if (previous) {
            container.removeEventListener('wheel', previous.onWheel, true);
            container.removeEventListener('gesturestart', previous.onGestureStart, true);
            container.removeEventListener('gesturechange', previous.onGestureChange, true);
            container.removeEventListener('gestureend', previous.onGestureEnd, true);
        }

        const state = { lastStepAt: 0, lastScale: 1 };
        const canStep = () => {
            const now = Date.now();
            if (now - state.lastStepAt < pinchZoomStepInterval) return false;
            state.lastStepAt = now;
            return true;
        };
        const onWheel = event => {
            const delta = Number(event.deltaY) || Number(event.deltaX) || 0;
            // A normal mouse wheel (and ordinary page scrolling) must remain on
            // HQChart's existing event path. Pinch wheels are pixel-mode, ctrl
            // modified, and have much smaller deltas than a mouse wheel.
            if (!event.ctrlKey || event.deltaMode !== 0 || !delta || Math.abs(delta) >= 90) return;
            if (canStep()) invokeNativeZoom(chart, delta < 0 ? -1 : 1, event);
            event.preventDefault();
            event.stopPropagation();
        };
        const onGestureStart = event => {
            state.lastScale = Number(event.scale) || 1;
            event.preventDefault();
            event.stopPropagation();
        };
        const onGestureChange = event => {
            const scale = Number(event.scale) || state.lastScale;
            const ratio = scale / state.lastScale;
            state.lastScale = scale;
            if (Math.abs(ratio - 1) < 0.01) return;
            if (canStep()) invokeNativeZoom(chart, ratio > 1 ? -1 : 1, event);
            event.preventDefault();
            event.stopPropagation();
        };
        const onGestureEnd = event => {
            state.lastScale = 1;
            event.preventDefault();
            event.stopPropagation();
        };

        container.addEventListener('wheel', onWheel, { capture: true, passive: false });
        container.addEventListener('gesturestart', onGestureStart, { capture: true, passive: false });
        container.addEventListener('gesturechange', onGestureChange, { capture: true, passive: false });
        container.addEventListener('gestureend', onGestureEnd, { capture: true, passive: false });
        pinchZoomListeners.set(key, { onWheel, onGestureStart, onGestureChange, onGestureEnd });
    }

    function removePinchZoom(container, key) {
        const listeners = pinchZoomListeners.get(key);
        if (!listeners || !container) return;
        container.removeEventListener('wheel', listeners.onWheel, true);
        container.removeEventListener('gesturestart', listeners.onGestureStart, true);
        container.removeEventListener('gesturechange', listeners.onGestureChange, true);
        container.removeEventListener('gestureend', listeners.onGestureEnd, true);
        pinchZoomListeners.delete(key);
    }

    async function fetchHistory(symbol, period, count) {
        const querySymbol = normalizeSymbol(symbol);
        const url = `/api/kline-all/tdx?code=${encodeURIComponent(querySymbol)}&type=${encodeURIComponent(period || 'day')}&limit=${encodeURIComponent(count || 800)}`;
        const response = await fetch(url);
        const result = await response.json();
        if (result.code !== 0) {
            throw new Error(result.message || 'HQChart 数据请求失败');
        }
        return buildHistoryPayload(querySymbol, period, result.data);
    }

    async function fetchIndexHistory(symbol, period, count) {
        const url = `/api/index/all?code=${encodeURIComponent(normalizeCode(symbol))}&type=${encodeURIComponent(period || 'day')}&limit=${encodeURIComponent(count || 800)}`;
        const response = await fetch(url);
        const result = await response.json();
        if (result.code !== 0) {
            throw new Error(result.message || 'HQChart 大盘数据请求失败');
        }
        return buildHistoryPayload(symbol, period, result.data, true);
    }

    async function fetchIndexHistoryWithFallback(symbol, period, count) {
        try {
            return await fetchIndexHistory(symbol, period, count);
        } catch (error) {
            if (String(symbol || '').toLowerCase() !== '399001.sz') {
                console.warn('HQChart primary index failed, retry with 399001.sz:', error);
                return fetchIndexHistory('399001.sz', period, count);
            }
            throw error;
        }
    }

    async function fetchMinute(symbol) {
        const code = normalizeCode(symbol);
        const response = await fetch(`/api/minute?code=${encodeURIComponent(code)}`);
        const result = await response.json();
        if (result.code !== 0) {
            throw new Error(result.message || '分时数据请求失败');
        }
        return result.data || {};
    }

    async function fetchQuote(symbol) {
        const code = normalizeSymbol(symbol);
        const response = await fetch(`/api/quote?code=${encodeURIComponent(code)}`);
        const result = await response.json();
        if (result.code !== 0) {
            throw new Error(result.message || '行情数据请求失败');
        }
        return Array.isArray(result.data) ? result.data[0] : null;
    }

    function defaultIndexSymbol(symbol) {
        const hqSymbol = toHQSymbol(symbol).toLowerCase();
        if (hqSymbol.endsWith('.sh')) return '000001.sh';
        return '399001.sz';
    }

    function latestStockFromHistory(payload, symbol) {
        const rows = payload && Array.isArray(payload.data) ? payload.data : [];
        const latest = rows[rows.length - 1] || [];
        return {
            name: payload?.name || symbol,
            symbol: payload?.symbol || symbol,
            yclose: latest[1] || latest[5] || 0,
            open: latest[2] || 0,
            price: latest[5] || 0,
            high: latest[3] || 0,
            low: latest[4] || 0,
            vol: latest[6] || 0,
            amount: latest[7] || 0,
            date: latest[0] || 0,
            time: latest[8] || 150000,
            increase: latest[1] ? ((latest[5] - latest[1]) * 100 / latest[1]) : 0,
            amplitude: latest[1] ? ((latest[3] - latest[4]) * 100 / latest[1]) : 0
        };
    }

    function dateToNumber(value) {
        if (!value) return 0;
        if (typeof value === 'number') return value;
        const date = new Date(value);
        if (!Number.isNaN(date.getTime())) {
            return Number(`${date.getFullYear()}${String(date.getMonth() + 1).padStart(2, '0')}${String(date.getDate()).padStart(2, '0')}`);
        }
        const number = Number(String(value).replace(/\D/g, '').slice(0, 8));
        return Number.isFinite(number) ? number : 0;
    }

    function timeToNumber(value) {
        if (!value) return 150000;
        if (typeof value === 'number') return value;
        const date = new Date(value);
        if (!Number.isNaN(date.getTime())) {
            return date.getHours() * 10000 + date.getMinutes() * 100 + date.getSeconds();
        }
        const number = Number(String(value).replace(/\D/g, '').slice(8, 14));
        return Number.isFinite(number) && number > 0 ? number : 150000;
    }

    function buildHistoryPayload(symbol, period, payload, isIndex = false) {
        const list = Array.isArray(payload?.list)
            ? payload.list
            : (Array.isArray(payload?.List) ? payload.List : []);
        const hqSymbol = isIndex ? toHQSymbol(symbol) : toHQSymbol(symbol);
        const data = list.map(item => {
            const close = scaledPrice(item?.Close);
            const yclose = scaledPrice(item?.Last) || close;
            const open = scaledPrice(item?.Open) || close;
            const high = scaledPrice(item?.High) || close;
            const low = scaledPrice(item?.Low) || close;
            const row = [
                dateToNumber(item?.Time || item?.time),
                yclose,
                open,
                high,
                low,
                close,
                Number(item?.Volume || item?.volume || 0),
                Number(item?.Amount || item?.amount || 0)
            ];
            if (String(period || '').toLowerCase().startsWith('minute') || String(period || '').toLowerCase() === 'hour') {
                row.push(timeToNumber(item?.Time || item?.time));
            }
            return row;
        }).filter(row => row[0] > 0 && row[5] > 0);
        return {
            symbol: hqSymbol,
            name: hqSymbol,
            period: period || payload?.meta?.type || 'day',
            data,
            ver: 2
        };
    }

    function scaledPrice(value) {
        const number = Number(value || 0);
        if (!Number.isFinite(number) || number <= 0) return 0;
        return number > 1000 ? number / 1000 : number;
    }

    function minuteTimeToNumber(value) {
        const text = String(value || '').trim();
        const match = text.match(/^(\d{1,2}):(\d{2})(?::(\d{2}))?$/);
        if (match) {
            return Number(match[1]) * 10000 + Number(match[2]) * 100 + Number(match[3] || 0);
        }
        const number = Number(text.replace(/\D/g, ''));
        if (!Number.isFinite(number)) return 0;
        return number < 10000 ? number * 100 : number;
    }

    function minuteDateToNumber(value) {
        const number = Number(String(value || '').replace(/\D/g, ''));
        if (Number.isFinite(number) && number > 0) return number;
        const now = new Date();
        return Number(`${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, '0')}${String(now.getDate()).padStart(2, '0')}`);
    }

    function buildMinutePayload(symbol, minutePayload, quote) {
        const hqSymbol = toHQSymbol(symbol);
        const rows = Array.isArray(minutePayload?.List) ? minutePayload.List : [];
        const date = minuteDateToNumber(minutePayload?.date);
        const quoteK = quote?.K || {};
        const yclose = scaledPrice(quoteK.Last) || scaledPrice(rows[0]?.Price);
        let totalVol = 0;
        let totalAmount = 0;
        let high = scaledPrice(quoteK.High) || 0;
        let low = scaledPrice(quoteK.Low) || 0;
        let amountForAverage = 0;
        let volumeForAverage = 0;

        const minute = rows.map(item => {
            const price = scaledPrice(item?.Price);
            const vol = Number(item?.Number || 0);
            const amount = price > 0 && vol > 0 ? price * vol : 0;
            totalVol += Number.isFinite(vol) ? vol : 0;
            totalAmount += amount;
            if (price > 0) {
                high = high > 0 ? Math.max(high, price) : price;
                low = low > 0 ? Math.min(low, price) : price;
                amountForAverage += amount;
                volumeForAverage += vol;
            }
            const average = volumeForAverage > 0 ? amountForAverage / volumeForAverage : price;
            return {
                date,
                time: minuteTimeToNumber(item?.Time),
                open: price,
                high: price,
                low: price,
                price,
                vol,
                amount,
                avprice: average || price
            };
        }).filter(item => item.time > 0 && item.price > 0);

        const first = minute[0] || {};
        const last = minute[minute.length - 1] || {};
        const open = scaledPrice(quoteK.Open) || first.price || 0;
        const close = scaledPrice(quoteK.Close) || last.price || 0;
        const name = quote?.Name || quote?.name || hqSymbol;
        return {
            stock: [{
                name,
                symbol: hqSymbol,
                yclose: yclose || open || close || 0,
                open,
                price: close,
                high: high || close || open || 0,
                low: low || close || open || 0,
                vol: Number(quote?.TotalHand || 0) * 100 || totalVol,
                amount: Number(quote?.Amount || 0) || totalAmount,
                date,
                time: last.time || 150000,
                minute,
                minutecount: minute.length || Number(minutePayload?.Count || 0) || 240
            }],
            AutoUpdate: false
        };
    }

    // 日内信号模块使用自己的分钟bar数据。转换在适配层完成,让 HQChart 与实时/回放共用同一入口。
    function buildIntradayMinutePayload(symbol, options) {
        const bars = Array.isArray(options?.minuteBars)
            ? options.minuteBars
            : (Array.isArray(options?.bars) ? options.bars : []);
        const list = bars
            .map(item => {
                const timeText = String(item?.chart_time || item?.time || '');
                const timeMatch = timeText.match(/T(\d{2}):(\d{2})(?::(\d{2}))?/);
                const price = Number(item?.close || item?.price || 0);
                if (!timeMatch || !Number.isFinite(price) || price <= 0) return null;
                return {
                    Time: `${timeMatch[1]}:${timeMatch[2]}:${timeMatch[3] || '00'}`,
                    Price: Math.round(price * 1000),
                    Number: Math.max(0, Number(item?.volume || item?.vol || 0))
                };
            })
            .filter(Boolean)
            .sort((left, right) => left.Time.localeCompare(right.Time));
        const first = bars[0];
        const date = dateToNumber(first?.time || options?.date);
        const prevClose = Number(options?.prevClose || 0);
        return buildMinutePayload(symbol, { date, Count: 240, List: list }, {
            Name: options?.name || toHQSymbol(symbol),
            K: { Last: prevClose, Open: Number(first?.open || first?.close || 0) }
        });
    }

    async function fetchMinuteChartData(symbol) {
        const [minuteResult, quoteResult] = await Promise.allSettled([
            fetchMinute(symbol),
            fetchQuote(symbol)
        ]);
        if (minuteResult.status === 'rejected') throw minuteResult.reason;
        return buildMinutePayload(symbol, minuteResult.value, quoteResult.status === 'fulfilled' ? quoteResult.value : null);
    }

    function createNetworkFilter(key) {
        return function networkFilter(data, callback) {
            const name = data && data.Name;
            const state = states.get(key) || {};
            if (isMinuteRequest(name)) {
                data.PreventDefault = true;
                const req = data.Request && data.Request.Data ? data.Request.Data : {};
                const reqSymbol = Array.isArray(req.symbol) ? req.symbol[0] : req.symbol;
                const symbol = reqSymbol || state.symbol;
                const dataRequest = Array.isArray(state.minuteBars)
                    ? Promise.resolve(buildIntradayMinutePayload(symbol, state))
                    : fetchMinuteChartData(symbol);
                dataRequest
                    .then(payload => {
                        callback(payload);
                    })
                    .catch(error => {
                        console.warn('HQChart minute failed:', error);
                        callback(buildMinutePayload(symbol, { Count: 0, List: [] }, null));
                    });
                if (name) {
                    console.debug('[HQChart NetworkFilter handled]', name);
                }
                return;
            }
            if (name === 'KLineChartContainer::RequestFlowCapitalData') {
                data.PreventDefault = true;
                const req = data.Request && data.Request.Data ? data.Request.Data : {};
                const symbol = req.symbol || state.symbol;
                callback({ symbol: toHQSymbol(symbol), name: toHQSymbol(symbol), stock: [] });
                return;
            }
            if (name === 'ScriptIndex::RequestAuthorization') {
                data.PreventDefault = true;
                const req = data.Request && data.Request.Data ? data.Request.Data : {};
                callback({
                    code: 0,
                    indexName: req.IndexName || '',
                    indexID: req.IndexID || '',
                    Lock: { IsLocked: false }
                });
                return;
            }
            if (name === 'JSSymbolData::GetIndexData') {
                data.PreventDefault = true;
                const req = data.Request && data.Request.Data ? data.Request.Data : {};
                const sourceSymbol = req.symbol || state.symbol;
                const indexSymbol = sourceSymbol ? defaultIndexSymbol(sourceSymbol) : (req.indexSymbol || '399001.sz');
                const count = req.count || state.count || 800;
                fetchIndexHistoryWithFallback(indexSymbol, state.period, count)
                    .then(payload => callback({
                        symbol: payload.symbol || toHQSymbol(indexSymbol),
                        name: payload.name || payload.symbol || toHQSymbol(indexSymbol),
                        data: payload.data || [],
                        ver: payload.ver || 2
                    }))
                    .catch(error => {
                        console.warn('HQChart index history failed:', error);
                        fetchHistory(state.symbol, state.period, count)
                            .then(payload => callback({
                                symbol: payload.symbol || toHQSymbol(state.symbol),
                                name: payload.name || payload.symbol || toHQSymbol(state.symbol),
                                data: payload.data || [],
                                ver: payload.ver || 2
                            }))
                            .catch(() => callback({ symbol: toHQSymbol(indexSymbol), name: toHQSymbol(indexSymbol), data: [], ver: 2 }));
                    });
                return;
            }
            if (name === 'JSSymbolData::GetLatestIndexData') {
                data.PreventDefault = true;
                const req = data.Request && data.Request.Data ? data.Request.Data : {};
                const indexSymbol = defaultIndexSymbol(state.symbol);
                fetchIndexHistoryWithFallback(indexSymbol, state.period, 2)
                    .then(payload => callback({ stock: [latestStockFromHistory(payload, toHQSymbol(indexSymbol))] }))
                    .catch(error => {
                        console.warn('HQChart latest index failed:', error);
                        callback({ stock: [] });
                    });
                return;
            }
            if (!isKLineHistoryRequest(name)) return;

            data.PreventDefault = true;
            const req = data.Request && data.Request.Data ? data.Request.Data : {};
            const symbol = req.symbol || state.symbol;
            const count = resolveHistoryLimit(req, state);
            fetchHistory(symbol, state.period, count)
                .then(payload => callback({
                    symbol: payload.symbol || toHQSymbol(symbol),
                    name: payload.name || payload.symbol || toHQSymbol(symbol),
                    data: payload.data || [],
                    ver: payload.ver || 2
                }))
                .catch(error => {
                    console.warn('HQChart history failed:', error);
                    callback({ symbol: toHQSymbol(symbol), name: toHQSymbol(symbol), data: [], ver: 2 });
                });
            if (name) {
                console.debug('[HQChart NetworkFilter handled]', name);
            }
        };
    }

    function destroyChart(container) {
        const key = chartKey(container);
        const chart = charts.get(key);
        removePinchZoom(container, key);
        if (chart && typeof chart.ChartDestroy === 'function') {
            chart.ChartDestroy();
        } else if (container && container.JSChart && typeof container.JSChart.ChartDestroy === 'function') {
            container.JSChart.ChartDestroy();
        }
        charts.delete(key);
        states.delete(key);
        if (container) container.innerHTML = '';
    }

    function getChart(container) {
        if (!container) return null;
        return charts.get(chartKey(container)) || null;
    }

    function renderKLine(container, options = {}) {
        if (!container || !hasHQChart()) return false;

        const key = chartKey(container);
        const symbol = normalizeSymbol(options.symbol);
        const period = options.period || 'day';
        const count = options.count || 800;
        const windows = options.windows || [
            { Index: 'MA' },
            { Index: 'VOL' },
            { Index: 'MACD' }
        ];
        states.set(key, { symbol, period, count });
        destroyChart(container);
        states.set(key, { symbol, period, count });

        if (window.MARKET_SUFFIX_NAME && typeof window.MARKET_SUFFIX_NAME.GetMarketStatus === 'function') {
            window.MARKET_SUFFIX_NAME.GetMarketStatus = function () { return 2; };
        }

        const chart = window.JSChart.Init(container, false, true);
        charts.set(key, chart);
        installPinchZoom(container, key, chart);
        chart.SetOption({
            Type: '历史K线图',
            Symbol: toHQSymbol(symbol),
            IsAutoUpdate: false,
            NetworkFilter: createNetworkFilter(key),
            Windows: windows,
            KLine: {
                Right: 1,
                Period: periodToHQChart(period),
                MaxRequestDataCount: count,
                PageSize: options.pageSize || 80,
                IsShowTooltip: true,
                DrawType: 0,
                RightSpaceCount: 2,
                DataWidth: options.dataWidth || 10
            },
            KLineTitle: {
                IsShowName: true,
                IsShowSettingInfo: true,
                IsTitleShowLatestData: true
            },
            Border: {
                Left: 54,
                Right: 76,
                Top: 24,
                Bottom: 24,
                AutoLeft: { Blank: 10, MinWidth: 54 },
                AutoRight: { Blank: 8, MinWidth: 60 }
            },
            CorssCursorInfo: {
                Right: 2,
                DateFormatType: 0,
                IsShowCorss: true
            },
            EnableYDrag: { Right: true, Left: false },
            EnableZoomIndexWindow: true,
            FloatTooltip: { Enable: true }
        });
        chart.ScriptErrorCallback = function (error) {
            const detail = typeof error === 'string'
                ? error
                : (error && (error.Description || error.message || JSON.stringify(error))) || '未知错误';
            if (typeof window.updateHQOverlayStatus === 'function') {
                window.updateHQOverlayStatus(`HQChart 公式错误：${detail}`);
            }
            console.warn('HQChart script error:', error);
        };
        return true;
    }

    function renderMinute(container, options = {}) {
        if (!container || !hasHQChart()) return false;

        const key = chartKey(container);
        const symbol = normalizeSymbol(options.symbol);
        const state = {
            symbol,
            period: 'minute',
            count: 240,
            minuteBars: Array.isArray(options.minuteBars) ? options.minuteBars : undefined,
            prevClose: options.prevClose,
            name: options.name,
            date: options.date
        };
        states.set(key, state);
        destroyChart(container);
        states.set(key, state);

        if (window.MARKET_SUFFIX_NAME && typeof window.MARKET_SUFFIX_NAME.GetMarketStatus === 'function') {
            window.MARKET_SUFFIX_NAME.GetMarketStatus = function () { return 2; };
        }

        const chart = window.JSChart.Init(container, false, true);
        charts.set(key, chart);
        installPinchZoom(container, key, chart);
        chart.SetOption({
            Type: '分钟走势图',
            Symbol: toHQSymbol(symbol),
            IsAutoUpdate: false,
            DayCount: 1,
            NetworkFilter: createNetworkFilter(key),
            Windows: options.windows || [],
            Minute: {
                IsShowTooltip: true
            },
            MinuteLine: {
                IsDrawAreaPrice: options.isDrawAreaPrice === false ? 0 : 1,
                IsShowAveragePrice: options.isShowAveragePrice === false ? 0 : 1
            },
            MinuteTitle: {
                IsShowName: true,
                IsShowDate: false,
                IsShowTime: true,
                IsTitleShowLatestData: true
            },
            Border: {
                Left: 54,
                Right: 76,
                Top: 24,
                Bottom: 24,
                AutoLeft: { Blank: 10, MinWidth: 54 },
                AutoRight: { Blank: 8, MinWidth: 60 }
            },
            CorssCursorInfo: {
                Right: 2,
                IsShowCorss: true
            },
            EnableYDrag: { Right: true, Left: false },
            EnableZoomIndexWindow: true,
            // 项目未引入 HQChart 的浮层表格样式,用画布内十字提示避免其退化为页面流内容。
            FloatTooltip: { Enable: false }
        });
        chart.ScriptErrorCallback = function (error) {
            const detail = typeof error === 'string'
                ? error
                : (error && (error.Description || error.message || JSON.stringify(error))) || '未知错误';
            if (typeof window.updateHQOverlayStatus === 'function') {
                window.updateHQOverlayStatus(`HQChart 公式错误：${detail}`);
            }
            console.warn('HQChart script error:', error);
        };
        return true;
    }

    function updateMinute(container, options = {}) {
        if (!container) return false;
        const key = chartKey(container);
        const chart = charts.get(key);
        const state = states.get(key);
        if (!chart || !state) return false;
        state.minuteBars = Array.isArray(options.minuteBars) ? options.minuteBars : [];
        state.prevClose = options.prevClose;
        state.name = options.name || state.name;
        state.date = options.date || state.date;
        const payload = buildIntradayMinutePayload(state.symbol, state);
        const minuteChart = chart.JSChartContainer || chart;
        if (typeof minuteChart.RecvMinuteData === 'function') {
            minuteChart.RecvMinuteData(payload);
            // HQChart's default minute paint can inherit a white theme from the
            // host page. Keep the native plot, but make its two lines explicit.
            if (minuteChart.ChartPaint?.[0]) {
                minuteChart.ChartPaint[0].Color = '#1769aa';
                minuteChart.ChartPaint[0].LineColor = '#1769aa';
                minuteChart.ChartPaint[0].AreaColor = 'rgba(23, 105, 170, 0.10)';
            }
            if (minuteChart.ChartPaint?.[1]) {
                minuteChart.ChartPaint[1].Color = '#b7791f';
                minuteChart.ChartPaint[1].LineColor = '#b7791f';
            }
            minuteChart.SetSizeChange?.(true);
            minuteChart.Draw?.();
            return true;
        }
        return false;
    }

    function getMinuteGeometry(container, prices = []) {
        const chart = getChart(container);
        const minuteChart = chart?.JSChartContainer || chart;
        const frame = minuteChart?.Frame?.SubFrame?.[0]?.Frame;
        if (!frame || typeof frame.GetXFromIndex !== 'function' || typeof frame.GetYFromData !== 'function') return null;
        const ratio = window.devicePixelRatio || 1;
        try {
            return {
                width: container.clientWidth,
                height: container.clientHeight,
                slots: Array.from({ length: 240 }, (_, index) => frame.GetXFromIndex(index) / ratio),
                prices: prices.map(value => frame.GetYFromData(value) / ratio)
            };
        } catch (error) {
            console.warn('HQChart minute geometry failed:', error);
            return null;
        }
    }

    function resize(container) {
        if (container) {
            const chart = charts.get(chartKey(container));
            if (chart && typeof chart.OnSize === 'function') chart.OnSize();
            return;
        }
        charts.forEach(chart => {
            if (chart && typeof chart.OnSize === 'function') chart.OnSize();
        });
    }

    window.TDXHQChart = { renderKLine, renderMinute, updateMinute, getMinuteGeometry, resize, destroy: destroyChart, getChart, isAvailable: hasHQChart };
})();
