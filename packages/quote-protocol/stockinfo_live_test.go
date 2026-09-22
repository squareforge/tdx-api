package quoteprotocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLiveDumpStockInfoList(t *testing.T) {
	if os.Getenv("WINNER_LIVE") == "" {
		t.Skip("set WINNER_LIVE=1 to run live stock info dump")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := New(Config{RequestTimeout: 15 * time.Second})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	payload := []byte{0x01, 0x11, 0x01, 0x12}
	resp, err := client.Request(ctx, Request{
		Type:       RT_STOCKINFOLIST,
		ExpectType: RT_STOCKINFOLIST,
		Size:       2,
		Payload:    payload,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := resp.Body
	t.Logf("body bytes=%d head=%s", len(body), hex.EncodeToString(body[:minInt(len(body), 160)]))
	if len(body) >= 20 {
		t.Logf("head type=0x%04X index=%d size16=%d align18=%d u32_16=%d",
			binary.LittleEndian.Uint16(body[0:2]),
			body[2],
			binary.LittleEndian.Uint16(body[16:18]),
			binary.LittleEndian.Uint16(body[18:20]),
			binary.LittleEndian.Uint32(body[16:20]),
		)
	}
	for _, code := range []string{"600570", "000001", "300750", "688001"} {
		positions := findAll(body, []byte(code), 10)
		t.Logf("positions for %s: %v", code, positions)
		for _, pos := range positions {
			start := pos - 48
			if start < 0 {
				start = 0
			}
			end := pos + 96
			if end > len(body) {
				end = len(body)
			}
			t.Logf("around %s @%d: %s", code, pos, hex.EncodeToString(body[start:end]))
		}
	}
	for pre := 0; pre <= 64; pre++ {
		total, ok := tryParseInitialLike(body, 20, pre)
		if ok {
			t.Logf("initial-like preamble=%d total=%d", pre, total)
		}
	}
	stats := scanStockInitRecords(body)
	t.Logf("scanned stock init records: total=%d byMarket=%v", stats.Total, stats.ByMarket)
	runs := scanStockInitRuns(body)
	runTotal := 0
	for _, run := range runs {
		runTotal += run.Count
	}
	t.Logf("scanned stock init runs: total=%d runs=%v", runTotal, runs[:minInt(len(runs), 40)])
	samples := sampleStockInitRecords(body, []uint16{
		0x1100, // sh index
		0x1101, // sh A
		0x1103, // sh bond
		0x1104, // sh fund
		0x1106, // sh sci-tech
		0x1108, // sh LOF
		0x1109, // sh ETF
		0x1200, // sz index
		0x1201, // sz A
		0x1203, // sz bond
		0x1204, // sz fund
		0x1208, // sz LOF
		0x1209, // sz ETF
		0x120D, // sz growth enterprise
	}, 3)
	for _, market := range sortedMarkets(samples) {
		t.Logf("sample market=0x%04X %s", market, marketTypeName(market))
		for _, sample := range samples[market] {
			t.Logf("  off=%d code=%s name=%q prevCloseRaw=%d prevClose=%.3f fiveDayVol=%d fullStockUserInfoPlausible=%v tail=%s",
				sample.Offset,
				sample.Code,
				sample.Name,
				sample.PrevCloseRaw,
				float64(sample.PrevCloseRaw)/1000,
				sample.FiveDayVol,
				sample.FullStockUserInfoPlausible,
				sample.TailHex,
			)
		}
	}
}

type stockScanStats struct {
	Total    int
	ByMarket map[uint16]int
}

func scanStockInitRecords(body []byte) stockScanStats {
	stats := stockScanStats{ByMarket: map[uint16]int{}}
	for offset := 20; offset+32 <= len(body); offset++ {
		market := binary.LittleEndian.Uint16(body[offset+16 : offset+18])
		if !plausibleCodeType(market) {
			continue
		}
		if !asciiCode6(body[offset+18 : offset+24]) {
			continue
		}
		stats.Total++
		stats.ByMarket[market]++
		offset += 31
	}
	return stats
}

type stockRun struct {
	Offset int
	Count  int
	Market uint16
}

func scanStockInitRuns(body []byte) []stockRun {
	var runs []stockRun
	for offset := 20; offset+32 <= len(body); offset++ {
		market, ok := stockInitRecordMarket(body, offset)
		if !ok {
			continue
		}
		count := 0
		cur := offset
		for cur+32 <= len(body) {
			if nextMarket, ok := stockInitRecordMarket(body, cur); !ok || nextMarket != market {
				break
			}
			count++
			cur += 32
		}
		if count >= 2 {
			runs = append(runs, stockRun{Offset: offset, Count: count, Market: market})
			offset = cur - 1
		}
	}
	return runs
}

func stockInitRecordMarket(body []byte, offset int) (uint16, bool) {
	if offset+32 > len(body) {
		return 0, false
	}
	market := binary.LittleEndian.Uint16(body[offset+16 : offset+18])
	if !plausibleCodeType(market) {
		return 0, false
	}
	if !asciiCode6(body[offset+18 : offset+24]) {
		return 0, false
	}
	return market, true
}

type stockInitSample struct {
	Offset                     int
	Market                     uint16
	Code                       string
	Name                       string
	PrevCloseRaw               int32
	FiveDayVol                 uint32
	FullStockUserInfoPlausible bool
	TailHex                    string
}

func sampleStockInitRecords(body []byte, markets []uint16, limit int) map[uint16][]stockInitSample {
	want := map[uint16]bool{}
	for _, market := range markets {
		want[market] = true
	}
	out := map[uint16][]stockInitSample{}
	for offset := 20; offset+32 <= len(body); offset++ {
		market, ok := stockInitRecordMarket(body, offset)
		if !ok || !want[market] || len(out[market]) >= limit {
			continue
		}
		nameRaw := bytes.TrimRight(body[offset:offset+16], "\x00 ")
		codeRaw := bytes.TrimRight(body[offset+18:offset+24], "\x00 ")
		tailEnd := minInt(offset+96, len(body))
		tail := body[offset+32 : tailEnd]
		if len(tail) > 24 {
			tail = tail[:24]
		}
		out[market] = append(out[market], stockInitSample{
			Offset:                     offset,
			Market:                     market,
			Code:                       string(codeRaw),
			Name:                       strings.TrimSpace(decodeMarketText(nameRaw)),
			PrevCloseRaw:               int32(binary.LittleEndian.Uint32(body[offset+24 : offset+28])),
			FiveDayVol:                 binary.LittleEndian.Uint32(body[offset+28 : offset+32]),
			FullStockUserInfoPlausible: plausibleFullStockUserInfo(body, offset),
			TailHex:                    hex.EncodeToString(tail),
		})
		offset += 31
	}
	return out
}

func plausibleFullStockUserInfo(body []byte, offset int) bool {
	const stockUserInfoMinSize = 8 + 32 + 16 + 32 + 32 + 4 + 4 + 2 + 2 + 2 + 4 + 4 + 4 + 4 + 16
	if offset+stockUserInfoMinSize > len(body) {
		return false
	}
	// In a real StockUserInfo, CodeInfo is at the beginning, followed by a 32-byte name.
	if _, err := ParseCodeInfo(body[offset : offset+8]); err != nil {
		return false
	}
	if !asciiCode6(body[offset+2 : offset+8]) {
		return false
	}
	decimal := int16(binary.LittleEndian.Uint16(body[offset+8+32+16+32+32+4+4+2 : offset+8+32+16+32+32+4+4+4]))
	return decimal >= 0 && decimal <= 8
}

func sortedMarkets(samples map[uint16][]stockInitSample) []uint16 {
	markets := make([]uint16, 0, len(samples))
	for market := range samples {
		markets = append(markets, market)
	}
	sort.Slice(markets, func(i, j int) bool { return markets[i] < markets[j] })
	return markets
}

func marketTypeName(market uint16) string {
	names := map[uint16]string{
		0x1100: "SH index",
		0x1101: "SH A",
		0x1103: "SH bond",
		0x1104: "SH fund",
		0x1106: "SH sci-tech",
		0x1108: "SH LOF",
		0x1109: "SH ETF",
		0x1200: "SZ index",
		0x1201: "SZ A",
		0x1203: "SZ bond",
		0x1204: "SZ fund",
		0x1208: "SZ LOF",
		0x1209: "SZ ETF",
		0x120D: "SZ growth",
	}
	if name, ok := names[market]; ok {
		return name
	}
	return ""
}

func findAll(data []byte, needle []byte, limit int) []int {
	var out []int
	base := 0
	for len(out) < limit {
		pos := bytes.Index(data[base:], needle)
		if pos < 0 {
			return out
		}
		out = append(out, base+pos)
		base += pos + 1
	}
	return out
}

func tryParseInitialLike(body []byte, offset int, preamble int) (int, bool) {
	if len(body) < 20 {
		return 0, false
	}
	markets := int(binary.LittleEndian.Uint16(body[16:18]))
	if markets <= 0 || markets > 100 {
		return 0, false
	}
	pos := offset
	total := 0
	for i := 0; i < markets; i++ {
		base := pos + preamble
		if base+32 > len(body) {
			return 0, false
		}
		typeCount := int(int16(binary.LittleEndian.Uint16(body[base+22 : base+24])))
		if typeCount < 0 || typeCount > 120 {
			return 0, false
		}
		infoBytes := 32 + typeCount*80
		countOffset := base + infoBytes
		if countOffset+4 > len(body) {
			return 0, false
		}
		count := int(int16(binary.LittleEndian.Uint16(body[countOffset : countOffset+2])))
		if count < 0 || count > 200000 {
			return 0, false
		}
		total += count
		pos = countOffset + 4 + count*32
		if pos > len(body) {
			return 0, false
		}
	}
	if pos != len(body) {
		return 0, false
	}
	fmt.Println(total)
	return total, true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
