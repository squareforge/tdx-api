package quoteprotocol

import (
	"context"
	"encoding/binary"
	"fmt"
)

type QuoteLevel struct {
	Price    float64 `json:"price"`
	RawPrice int32   `json:"raw_price"`
	Volume   int64   `json:"volume"`
}

type RealtimeQuote struct {
	Symbol      string       `json:"symbol"`
	CodeInfo    CodeInfo     `json:"code_info"`
	Open        float64      `json:"open"`
	High        float64      `json:"high"`
	Low         float64      `json:"low"`
	Last        float64      `json:"last"`
	RawOpen     int32        `json:"raw_open"`
	RawHigh     int32        `json:"raw_high"`
	RawLow      int32        `json:"raw_low"`
	RawLast     int32        `json:"raw_last"`
	Volume      int64        `json:"volume"`
	Amount      int64        `json:"amount"`
	Bids        []QuoteLevel `json:"bids,omitempty"`
	Asks        []QuoteLevel `json:"asks,omitempty"`
	RecordBytes int          `json:"record_bytes"`
	Version     uint16       `json:"version"`
}

func (c *Client) GetRealtime(ctx context.Context, symbols []string) ([]RealtimeQuote, error) {
	infos, payload, err := BuildRealtimePayload(symbols)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("at least one symbol is required")
	}
	resp, err := c.Request(ctx, Request{
		Type:       RT_REALTIME_EXT_INT64,
		ExpectType: RT_REALTIME_EXT_INT64,
		Size:       int16(len(infos)),
		Payload:    payload,
	})
	if err != nil {
		return nil, err
	}
	return ParseRealtimeInt64(resp.Body)
}

func BuildRealtimePayload(symbols []string) ([]CodeInfo, []byte, error) {
	infos := make([]CodeInfo, 0, len(symbols))
	payload := make([]byte, 0, len(symbols)*8)
	for _, symbol := range symbols {
		info, err := ParseSymbol(symbol)
		if err != nil {
			return nil, nil, err
		}
		infos = append(infos, info)
		payload = append(payload, EncodeCodeInfo(info)...)
	}
	return infos, payload, nil
}

func ParseRealtimeInt64(body []byte) ([]RealtimeQuote, error) {
	if len(body) < 20 {
		return nil, fmt.Errorf("RT_REALTIME_EXT_INT64 body too short: %d", len(body))
	}
	if binary.LittleEndian.Uint16(body[0:2]) != RT_REALTIME_EXT_INT64 {
		return nil, fmt.Errorf("unexpected realtime response type 0x%04X", binary.LittleEndian.Uint16(body[0:2]))
	}
	count := int(binary.LittleEndian.Uint16(body[16:18]))
	quotes := make([]RealtimeQuote, 0, count)
	offset := 20
	for i := 0; i < count && offset+68 <= len(body); i++ {
		recordSize := int(binary.LittleEndian.Uint16(body[offset : offset+2]))
		if recordSize <= 0 || offset+recordSize > len(body) {
			break
		}
		version := binary.LittleEndian.Uint16(body[offset+2 : offset+4])
		codeInfo, err := ParseCodeInfo(body[offset+4 : offset+12])
		if err != nil {
			return nil, err
		}
		nowOffset := offset + 48
		if nowOffset+36 > len(body) {
			break
		}
		rawOpen := int32(binary.LittleEndian.Uint32(body[nowOffset+4 : nowOffset+8]))
		rawHigh := int32(binary.LittleEndian.Uint32(body[nowOffset+8 : nowOffset+12]))
		rawLow := int32(binary.LittleEndian.Uint32(body[nowOffset+12 : nowOffset+16]))
		rawLast := int32(binary.LittleEndian.Uint32(body[nowOffset+16 : nowOffset+20]))
		volume := int64(binary.LittleEndian.Uint64(body[nowOffset+20 : nowOffset+28]))
		amount := int64(binary.LittleEndian.Uint64(body[nowOffset+28 : nowOffset+36]))
		quote := RealtimeQuote{
			Symbol:      codeInfo.Symbol,
			CodeInfo:    codeInfo,
			Open:        MarketPrice(rawOpen),
			High:        MarketPrice(rawHigh),
			Low:         MarketPrice(rawLow),
			Last:        MarketPrice(rawLast),
			RawOpen:     rawOpen,
			RawHigh:     rawHigh,
			RawLow:      rawLow,
			RawLast:     rawLast,
			Volume:      volume,
			Amount:      amount,
			RecordBytes: recordSize,
			Version:     version,
		}
		levelOffset := nowOffset + 36
		quote.Bids = parseQuoteLevels(body, levelOffset, 5)
		quote.Asks = parseQuoteLevels(body, levelOffset+5*12, 5)
		quotes = append(quotes, quote)
		offset += recordSize
	}
	return quotes, nil
}

func parseQuoteLevels(body []byte, offset int, count int) []QuoteLevel {
	levels := make([]QuoteLevel, 0, count)
	for i := 0; i < count; i++ {
		itemOffset := offset + i*12
		if itemOffset+12 > len(body) {
			break
		}
		rawPrice := int32(binary.LittleEndian.Uint32(body[itemOffset : itemOffset+4]))
		volume := int64(binary.LittleEndian.Uint64(body[itemOffset+4 : itemOffset+12]))
		levels = append(levels, QuoteLevel{
			Price:    MarketPrice(rawPrice),
			RawPrice: rawPrice,
			Volume:   volume,
		})
	}
	return levels
}

func MarketPrice(raw int32) float64 {
	return float64(raw) / 1000
}
