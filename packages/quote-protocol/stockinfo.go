package quoteprotocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"time"
)

var DefaultStockInfoMarkets = []uint16{
	0x1101, // SH A
	0x1201, // SZ A
}

type StockInfo struct {
	CodeType     uint16  `json:"code_type"`
	CodeTypeHex  string  `json:"code_type_hex"`
	Exchange     string  `json:"exchange"`
	Code         string  `json:"code"`
	Symbol       string  `json:"symbol"`
	Name         string  `json:"name"`
	PrevCloseRaw int32   `json:"prev_close_raw"`
	PrevClose    float64 `json:"prev_close"`
	FiveDayVol   uint32  `json:"five_day_vol"`
}

func (c *Client) GetStockInfoList(ctx context.Context, markets []uint16) ([]StockInfo, error) {
	if len(markets) == 0 {
		markets = DefaultStockInfoMarkets
	}
	payload := BuildStockInfoListPayload(markets)
	resp, err := c.Request(ctx, Request{
		Type:       RT_STOCKINFOLIST,
		ExpectType: RT_STOCKINFOLIST,
		Size:       int16(len(markets)),
		Payload:    payload,
		Timeout:    15 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return ParseStockInfoList(resp.Body)
}

func BuildStockInfoListPayload(markets []uint16) []byte {
	payload := make([]byte, 0, len(markets)*2)
	for _, market := range markets {
		item := make([]byte, 2)
		binary.LittleEndian.PutUint16(item, market)
		payload = append(payload, item...)
	}
	return payload
}

func ParseStockInfoList(body []byte) ([]StockInfo, error) {
	if len(body) < 20 {
		return nil, fmt.Errorf("RT_STOCKINFOLIST body too short: %d", len(body))
	}
	if binary.LittleEndian.Uint16(body[0:2]) != RT_STOCKINFOLIST {
		return nil, fmt.Errorf("unexpected stock info response type 0x%04X", binary.LittleEndian.Uint16(body[0:2]))
	}

	byKey := map[string]StockInfo{}
	for offset := 20; offset+32 <= len(body); offset++ {
		info, ok := parseStockInitInfoAt(body, offset)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%04X:%s", info.CodeType, info.Code)
		if _, exists := byKey[key]; !exists {
			byKey[key] = info
		}
		offset += 31
	}

	out := make([]StockInfo, 0, len(byKey))
	for _, info := range byKey {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CodeType != out[j].CodeType {
			return out[i].CodeType < out[j].CodeType
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func parseStockInitInfoAt(body []byte, offset int) (StockInfo, bool) {
	if offset+32 > len(body) {
		return StockInfo{}, false
	}
	codeType := binary.LittleEndian.Uint16(body[offset+16 : offset+18])
	if !plausibleCodeType(codeType) {
		return StockInfo{}, false
	}
	codeBytes := body[offset+18 : offset+24]
	if !asciiCode6(codeBytes) {
		return StockInfo{}, false
	}
	code := strings.TrimRight(string(codeBytes), "\x00 ")
	if code == "" {
		return StockInfo{}, false
	}
	name := strings.TrimSpace(decodeMarketText(bytes.TrimRight(body[offset:offset+16], "\x00 ")))
	exchange := ExchangeByCodeType(codeType)
	rawPrevClose := int32(binary.LittleEndian.Uint32(body[offset+24 : offset+28]))
	return StockInfo{
		CodeType:     codeType,
		CodeTypeHex:  HexType(codeType),
		Exchange:     exchange,
		Code:         code,
		Symbol:       exchange + code,
		Name:         name,
		PrevCloseRaw: rawPrevClose,
		PrevClose:    MarketPrice(rawPrevClose),
		FiveDayVol:   binary.LittleEndian.Uint32(body[offset+28 : offset+32]),
	}, true
}

func plausibleCodeType(codeType uint16) bool {
	return codeType&0xF000 == StockMarket
}

func ExchangeByCodeType(codeType uint16) string {
	switch codeType & 0x0F00 {
	case SHBourse:
		return "sh"
	case SZBourse:
		return "sz"
	case 0x0C00:
		return "bj"
	default:
		return ""
	}
}

func asciiCode6(data []byte) bool {
	if len(data) != 6 {
		return false
	}
	for _, b := range data {
		if b >= '0' && b <= '9' {
			continue
		}
		if b >= 'A' && b <= 'Z' {
			continue
		}
		if b >= 'a' && b <= 'z' {
			continue
		}
		return false
	}
	return true
}
