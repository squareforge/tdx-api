package main

import (
	"context"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
)

// 补K测试统一时间:2026-09-15 14:45(周二,交易日)
var patchNow = time.Date(2026, 9, 15, 14, 45, 0, 0, time.Local)

// patchTestServer 构造补K测试的 fake market-service:
//   - workdayErr=true 时 /api/workday 返回500,否则返回 is_workday=workday
//   - quoteErr=true 时 /api/quote 返回500,否则返回 quotes
func patchTestServer(t *testing.T, workday bool, workdayErr bool, quotes []*protocol.Quote, quoteErr bool) (*MarketServiceClient, func()) {
	t.Helper()
	return newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/workday":
			if workdayErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeMarketTestResponse(t, w, map[string]interface{}{
				"date":       map[string]string{"iso": "2026-09-15", "numeric": "20260915"},
				"is_workday": workday,
			})
		case "/api/quote":
			if quoteErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeMarketTestResponse(t, w, quotes)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
}

func patchQuote(code string, exchange protocol.Exchange, t time.Time, last, open, high, low, close protocol.Price, vol int64) *protocol.Quote {
	return &protocol.Quote{
		Exchange: exchange,
		Code:     code,
		Kline: &protocol.Kline{
			Time:   t,
			Last:   last,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: vol,
		},
	}
}

func usePatchClient(t *testing.T, client *MarketServiceClient) {
	t.Helper()
	previous := marketClient
	marketClient = client
	t.Cleanup(func() { marketClient = previous })
}

// 非交易日:不补K,klines 原样返回
func TestPatchIntradayNotWorkday(t *testing.T) {
	client, closeServer := patchTestServer(t, false, false, nil, false)
	defer closeServer()
	usePatchClient(t, client)

	rows := []FormulaKline{{Date: 20260914, YClose: 10, Open: 10, High: 10.2, Low: 9.8, Close: 10}}
	klines := map[string][]FormulaKline{"000001": rows}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
	if len(klines["000001"]) != 1 {
		t.Fatalf("rows = %d, want 1(非交易日不补)", len(klines["000001"]))
	}
}

func TestBuildIntradayKlineCarriesSnapshotMetadata(t *testing.T) {
	quoteTime := time.Date(2026, 9, 15, 14, 45, 3, 0, time.Local)
	quote := patchQuote("000001", protocol.ExchangeSZ, quoteTime, 10000, 10100, 10300, 10050, 10200, 800)
	row, ok := buildIntradayKline([]FormulaKline{{Close: 10}}, quote)
	if !ok {
		t.Fatal("buildIntradayKline() returned false")
	}
	if row.SnapshotAt != quoteTime.Format(time.RFC3339) || row.SnapshotSource != "market-service.quote" {
		t.Fatalf("snapshot metadata = %q/%q", row.SnapshotAt, row.SnapshotSource)
	}
}

func TestApplyStrategySnapshotMarksHistoricalRowsAsInferred(t *testing.T) {
	item := StrategySelectionItem{Symbol: "000001", Latest: 10}
	applyStrategySnapshot(&item, []FormulaKline{{Date: 20260915, Close: 10}})
	if item.SignalAt == "" || item.SignalPrice != 10 || item.SnapshotStatus != "inferred" {
		t.Fatalf("historical snapshot = %+v", item)
	}
	if item.QuoteAt != "" || item.QuoteSource != "" {
		t.Fatalf("historical row unexpectedly has quote evidence: %+v", item)
	}
}

// 今日K已落库:不补,也不调快照接口
func TestPatchIntradayAlreadyHasToday(t *testing.T) {
	client, closeServer := newTestMarketClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/workday" {
			writeMarketTestResponse(t, w, map[string]interface{}{
				"is_workday": true,
			})
			return
		}
		t.Fatalf("unexpected path: %s(已落库不应调快照)", r.URL.Path)
	})
	defer closeServer()
	usePatchClient(t, client)

	rows := []FormulaKline{{Date: 20260914, Close: 10}, {Date: 20260915, Close: 10.2}}
	klines := map[string][]FormulaKline{"000001": rows}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
	if len(klines["000001"]) != 2 {
		t.Fatalf("rows = %d, want 2", len(klines["000001"]))
	}
}

// 盘中补K成功:合成K字段正确(非除权日 adj=1)
func TestPatchIntradayPatchSuccess(t *testing.T) {
	quote := patchQuote("000001", protocol.ExchangeSZ, patchNow,
		10000, 10100, 10300, 10050, 10200, 800) // 厘:昨收10 开10.1 高10.3 低10.05 收10.2
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{quote}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{"000001": {
		{Date: 20260914, YClose: 9.8, Open: 10, High: 10.2, Low: 9.7, Close: 10, Vol: 1000},
	}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
	rows := klines["000001"]
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2(历史+合成)", len(rows))
	}
	synth := rows[1]
	if synth.Date != 20260915 {
		t.Fatalf("synth.Date = %d, want 20260915", synth.Date)
	}
	if synth.YClose != 10 {
		t.Fatalf("synth.YClose = %v, want 10(衔接前收)", synth.YClose)
	}
	if synth.Open != 10.1 || synth.High != 10.3 || synth.Low != 10.05 || synth.Close != 10.2 {
		t.Fatalf("synth OHLC = %v/%v/%v/%v, want 10.1/10.3/10.05/10.2", synth.Open, synth.High, synth.Low, synth.Close)
	}
	if synth.Vol != 800 {
		t.Fatalf("synth.Vol = %v, want 800(手,与库内K同单位)", synth.Vol)
	}
}

// 除权日比值法:10送10(adj=0.5),合成K归入前复权口径且无假缺口
func TestPatchIntradayAdjOnExDate(t *testing.T) {
	quote := patchQuote("000001", protocol.ExchangeSZ, patchNow,
		50000, 25000, 26500, 24800, 26000, 900) // 真实价:昨收50 开25 现26
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{quote}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{"000001": {
		{Date: 20260914, YClose: 24, Open: 24.5, High: 25, Low: 23.8, Close: 25}, // 前复权除权前收25
	}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
	synth := klines["000001"][1]
	// adj = 25/50 = 0.5
	if math.Abs(synth.YClose-25) > 1e-9 {
		t.Fatalf("synth.YClose = %v, want 25(与前收衔接)", synth.YClose)
	}
	if math.Abs(synth.Open-12.5) > 1e-9 || math.Abs(synth.Close-13) > 1e-9 {
		t.Fatalf("synth Open/Close = %v/%v, want 12.5/13(adj=0.5)", synth.Open, synth.Close)
	}
	if math.Abs(synth.High-13.25) > 1e-9 || math.Abs(synth.Low-12.4) > 1e-9 {
		t.Fatalf("synth High/Low = %v/%v, want 13.25/12.4", synth.High, synth.Low)
	}
}

// 停牌/无快照:纯股票代码被移除并记录原因
func TestPatchIntradaySkipNoQuote(t *testing.T) {
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{"000001": {{Date: 20260914, Close: 10}}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if reasons["000001"] == "" {
		t.Fatalf("reasons = %v, want 跳过原因", reasons)
	}
	if _, ok := klines["000001"]; ok {
		t.Fatal("000001 应被移除(宁缺毋滥)")
	}
}

// 快照K线是昨日数据(停牌股):合成K日期校验不通过,移除
func TestPatchIntradaySkipStaleTime(t *testing.T) {
	stale := patchQuote("000001", protocol.ExchangeSZ, time.Date(2026, 9, 14, 15, 0, 0, 0, time.Local),
		10000, 10000, 10200, 9900, 10100, 500)
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{stale}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{"000001": {{Date: 20260914, Close: 10}}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if reasons["000001"] == "" {
		t.Fatalf("reasons = %v, want 跳过原因", reasons)
	}
	if _, ok := klines["000001"]; ok {
		t.Fatal("000001 应被移除(昨日快照不可误补)")
	}
}

// 快照接口整体失败:该批全部移除
func TestPatchIntradayQuoteError(t *testing.T) {
	client, closeServer := patchTestServer(t, true, false, nil, true)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{
		"000001": {{Date: 20260914, Close: 10}},
		"000002": {{Date: 20260914, Close: 20}},
	}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if reasons["000001"] == "" || reasons["000002"] == "" {
		t.Fatalf("reasons = %v, want 两支均跳过", reasons)
	}
	if len(klines) != 0 {
		t.Fatalf("klines = %v, want 全部移除", klines)
	}
}

// 交易日判断接口失败 + 周六:周内日兜底为非交易日,不补
func TestPatchIntradayWorkdayAPIErrorWeekend(t *testing.T) {
	client, closeServer := patchTestServer(t, true, true, nil, false)
	defer closeServer()
	usePatchClient(t, client)

	saturday := time.Date(2026, 9, 19, 10, 0, 0, 0, time.Local) // 周六
	klines := map[string][]FormulaKline{"000001": {{Date: 20260918, Close: 10}}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, saturday)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty(周末兜底不补)", reasons)
	}
	if len(klines["000001"]) != 1 {
		t.Fatalf("rows = %d, want 1", len(klines["000001"]))
	}
}

// 交易日判断接口失败 + 工作日:兜底继续尝试补K
func TestPatchIntradayWorkdayAPIErrorWeekday(t *testing.T) {
	quote := patchQuote("000001", protocol.ExchangeSZ, patchNow, 10000, 10100, 10300, 10050, 10200, 800)
	client, closeServer := patchTestServer(t, true, true, []*protocol.Quote{quote}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{"000001": {{Date: 20260914, Close: 10}}}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty(工作日兜底应补K)", reasons)
	}
	if len(klines["000001"]) != 2 {
		t.Fatalf("rows = %d, want 2", len(klines["000001"]))
	}
}

// 指数基准无快照:保留库内K,不移除(避免市场因子因基准缺失而失效)
func TestPatchIntradayBenchmarkKept(t *testing.T) {
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{
		"sh000001": {{Date: 20260914, Close: 3100}},
		"000001":   {{Date: 20260914, Close: 10}},
	}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if reasons["000001"] == "" {
		t.Fatalf("reasons = %v, want 股票被跳过", reasons)
	}
	if _, ok := klines["sh000001"]; !ok {
		t.Fatal("sh000001 应保留(指数基准不因补K失败被删)")
	}
	if _, ok := klines["000001"]; ok {
		t.Fatal("000001 应被移除")
	}
}

// 指数与股票同纯代码(000001 vs sh000001)同批查询:按市场消歧,行情不串
func TestPatchIntradayMarketDisambiguation(t *testing.T) {
	// 指数行情(沪):昨收3100 现3120;股票行情(深):昨收10 现10.2
	indexQuote := patchQuote("000001", protocol.ExchangeSH, patchNow, 3100000, 3105000, 3130000, 3095000, 3120000, 1000)
	stockQuote := patchQuote("000001", protocol.ExchangeSZ, patchNow, 10000, 10100, 10300, 10050, 10200, 800)
	client, closeServer := patchTestServer(t, true, false, []*protocol.Quote{indexQuote, stockQuote}, false)
	defer closeServer()
	usePatchClient(t, client)

	klines := map[string][]FormulaKline{
		"sh000001": {{Date: 20260914, Close: 3100}},
		"000001":   {{Date: 20260914, Close: 10}},
	}
	reasons := (&AutomationRunner{}).patchIntradayKlines(context.Background(), klines, patchNow)
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
	indexSynth := klines["sh000001"][1]
	if indexSynth.Close != 3120 {
		t.Fatalf("sh000001 合成Close = %v, want 3120(沪市指数行情)", indexSynth.Close)
	}
	stockSynth := klines["000001"][1]
	if stockSynth.Close != 10.2 {
		t.Fatalf("000001 合成Close = %v, want 10.2(深市股票行情)", stockSynth.Close)
	}
}

// 合成K兜底:昨收缺失(Last=0)时 adj=1 裸拼;历史K为空时仍可合成
func TestBuildIntradayKlineFallback(t *testing.T) {
	quote := patchQuote("000001", protocol.ExchangeSZ, patchNow, 0, 10100, 10300, 10050, 10200, 800)
	rows := []FormulaKline{{Date: 20260914, Close: 10}}
	synth, ok := buildIntradayKline(rows, quote)
	if !ok {
		t.Fatal("buildIntradayKline = false, want true")
	}
	if synth.Open != 10.1 || synth.Close != 10.2 {
		t.Fatalf("synth Open/Close = %v/%v, want 10.1/10.2(adj=1裸拼)", synth.Open, synth.Close)
	}
	if synth.YClose != 10 {
		t.Fatalf("synth.YClose = %v, want 10(用历史最后Close兜底)", synth.YClose)
	}
	// 历史K为空
	emptySynth, ok := buildIntradayKline(nil, quote)
	if !ok {
		t.Fatal("buildIntradayKline(nil) = false, want true")
	}
	if emptySynth.Date != 20260915 {
		t.Fatalf("emptySynth.Date = %d, want 20260915", emptySynth.Date)
	}
	// 快照无效:Close<=0 / Kline为空 / quote为空
	if _, ok := buildIntradayKline(rows, &protocol.Quote{Code: "000001"}); ok {
		t.Fatal("Kline为nil 应返回 false")
	}
	if _, ok := buildIntradayKline(rows, nil); ok {
		t.Fatal("quote为nil 应返回 false")
	}
}
