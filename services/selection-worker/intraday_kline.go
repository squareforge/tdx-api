package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/injoyai/tdx/protocol"
)

// 盘中实盘选股补K:三层判定(交易日 → 最后一根K日期 → 快照兜底) + 合成今日未收盘K线。
// 设计基线:docs/intraday-selection-realtime-kline.md
// 仅实盘选股路径调用;回测路径(historical_backtest.go)保持纯收盘口径,不经过本逻辑。

// intradayQuoteBatchSize 单批快照查询上限(market-service 侧单次上限为50)
const intradayQuoteBatchSize = 50

// intradayQuoteCode 剥离市场前缀取纯6位代码,用于匹配 tdx Quote.Code
func intradayQuoteCode(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	if len(s) > 6 {
		if strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") || strings.HasPrefix(s, "bj") {
			return s[2:]
		}
	}
	return s
}

// intradayQuoteMarket 推断代码所属市场:带前缀的直接取前缀,纯代码按编码规则推断。
// 用于同一纯代码的指数与股票(如 sh000001 与 000001)同批查询时的行情消歧。
func intradayQuoteMarket(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	switch {
	case strings.HasPrefix(s, "sh"):
		return "sh"
	case strings.HasPrefix(s, "sz"):
		return "sz"
	case strings.HasPrefix(s, "bj"):
		return "bj"
	}
	if len(s) >= 3 {
		switch s[:3] {
		case "600", "601", "603", "605", "688", "689", "900":
			return "sh"
		case "000", "001", "002", "003", "300", "301", "200":
			return "sz"
		case "430", "830", "831", "832", "833", "834", "835", "836", "837", "838", "839", "870", "871", "872", "873", "920":
			return "bj"
		}
	}
	return ""
}

// popIntradayQuote 从同代码行情队列中取出一条:优先匹配目标市场,取不到则取第一条。
// 返回取出的行情与剩余队列。
func popIntradayQuote(queue []*protocol.Quote, wantMarket string) (*protocol.Quote, []*protocol.Quote) {
	if len(queue) == 0 {
		return nil, queue
	}
	idx := 0
	if wantMarket != "" {
		for i, q := range queue {
			if q.Exchange.String() == wantMarket {
				idx = i
				break
			}
		}
	}
	quote := queue[idx]
	remaining := make([]*protocol.Quote, 0, len(queue)-1)
	remaining = append(remaining, queue[:idx]...)
	remaining = append(remaining, queue[idx+1:]...)
	return quote, remaining
}

// buildIntradayKline 用实时快照合成今日未收盘K线(前复权口径,adj比值法)。
// adj = 历史最后一根K的Close(前复权) / 快照昨收(真实价):
// 非除权日≈1;除权除息日等于除权调整比例,保证拼接后序列无假缺口。
func buildIntradayKline(rows []FormulaKline, quote *protocol.Quote) (FormulaKline, bool) {
	if quote == nil || quote.Kline == nil || quote.Kline.Close <= 0 {
		return FormulaKline{}, false
	}
	k := quote.Kline
	lastClose := 0.0
	if len(rows) > 0 {
		lastClose = rows[len(rows)-1].Close
	}
	rawLast := k.Last.Float64()
	adj := 1.0
	if lastClose > 0 && rawLast > 0 {
		adj = lastClose / rawLast
	}
	yClose := lastClose
	if yClose <= 0 {
		yClose = rawLast * adj
	}
	return FormulaKline{
		Date:           dateInt(k.Time),
		Time:           timeInt(k.Time),
		YClose:         yClose,
		Open:           k.Open.Float64() * adj,
		High:           k.High.Float64() * adj,
		Low:            k.Low.Float64() * adj,
		Close:          k.Close.Float64() * adj,
		Vol:            float64(k.Volume),
		Amount:         amountToYuan(k.Amount, k.Close.Float64(), float64(k.Volume)),
		SnapshotAt:     k.Time.Format(time.RFC3339),
		SnapshotSource: "market-service.quote",
	}, true
}

// patchIntradayKlines 实盘选股路径专用:三层判定后为缺失今日K线的标的合成盘中K。
// 直接修改 klines:
//   - 成功补K的追加合成K;
//   - 停牌/无快照的纯股票代码移除并记入返回的跳过原因(宁缺毋滥,不静默用旧数据);
//   - 带市场前缀的指数类代码(如 sh000001)补K失败时保留库内K(避免市场动量因子因
//     基准数据被删而失效,最坏情况是基准滞后一天,由因子自身的日期对齐检查兜底)。
//
// 回测路径(historical_backtest.go)不调用本函数,保持纯收盘口径。
func (r *AutomationRunner) patchIntradayKlines(ctx context.Context, klines map[string][]FormulaKline, now time.Time) map[string]string {
	skipReasons := map[string]string{}
	if len(klines) == 0 {
		return skipReasons
	}
	// 第1步:今天是否交易日。接口失败按周内日兜底(周六/周日视为非交易日)。
	isWorkday, err := marketClient.IsWorkday(ctx, now.Format("2006-01-02"))
	if err != nil {
		isWorkday = now.Weekday() != time.Saturday && now.Weekday() != time.Sunday
		log.Printf("盘中补K:交易日判断失败(%v),按周内日兜底 is_workday=%v", err, isWorkday)
	}
	if !isWorkday {
		return skipReasons
	}
	// 第2步:最后一根K日期 != 今天 的标的才需要补
	today := dateInt(now)
	var pending []string
	for symbol, rows := range klines {
		if len(rows) == 0 || rows[len(rows)-1].Date != today {
			pending = append(pending, symbol)
		}
	}
	if len(pending) == 0 {
		return skipReasons
	}
	// 第3步:批量拉快照并合成K
	quotes := map[string]*protocol.Quote{}
	for _, batch := range chunkSymbols(pending, intradayQuoteBatchSize) {
		if err := ctx.Err(); err != nil {
			for _, symbol := range batch {
				skipReasons[symbol] = err.Error()
				delete(klines, symbol)
			}
			continue
		}
		resp, err := marketClient.Quotes(ctx, batch)
		if err != nil {
			log.Printf("盘中补K:批量快照失败(%d只): %v", len(batch), err)
			for _, symbol := range batch {
				skipReasons[symbol] = "实时行情不可用"
				delete(klines, symbol)
			}
			continue
		}
		byCode := map[string][]*protocol.Quote{}
		for _, quote := range resp {
			if quote == nil {
				continue
			}
			byCode[quote.Code] = append(byCode[quote.Code], quote)
		}
		for _, symbol := range batch {
			code := intradayQuoteCode(symbol)
			var quote *protocol.Quote
			quote, byCode[code] = popIntradayQuote(byCode[code], intradayQuoteMarket(symbol))
			if quote == nil {
				if isIntradayBenchmarkSymbol(symbol) {
					log.Printf("盘中补K:%s 无今日实时行情,保留库内K", symbol)
					continue
				}
				skipReasons[symbol] = "无今日实时行情(停牌或数据缺失)"
				delete(klines, symbol)
				continue
			}
			if quote.Kline == nil {
				if isIntradayBenchmarkSymbol(symbol) {
					log.Printf("盘中补K:%s 实时行情无K线,保留库内K", symbol)
					continue
				}
				skipReasons[symbol] = "实时行情无K线"
				delete(klines, symbol)
				continue
			}
			quotes[symbol] = quote
		}
	}
	// 合成K并校验:快照日期必须为今天,防止停牌股用昨日快照误补
	for symbol, quote := range quotes {
		rows, ok := klines[symbol]
		if !ok {
			continue
		}
		synth, valid := buildIntradayKline(rows, quote)
		if !valid || synth.Date != today {
			if isIntradayBenchmarkSymbol(symbol) {
				log.Printf("盘中补K:%s 今日实时K线不完整,保留库内K", symbol)
				continue
			}
			skipReasons[symbol] = "今日实时K线不完整"
			delete(klines, symbol)
			continue
		}
		klines[symbol] = append(rows, synth)
	}
	return skipReasons
}

// isIntradayBenchmarkSymbol 带市场前缀的代码视为指数基准(股票池为纯6位代码)
func isIntradayBenchmarkSymbol(symbol string) bool {
	s := strings.ToLower(strings.TrimSpace(symbol))
	return strings.HasPrefix(s, "sh") || strings.HasPrefix(s, "sz") || strings.HasPrefix(s, "bj")
}
