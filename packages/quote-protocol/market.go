package quoteprotocol

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	StockMarket = uint16(0x1000)
	SHBourse    = uint16(0x0100)
	SZBourse    = uint16(0x0200)

	KindIndex  = uint16(0x0000)
	KindStockA = uint16(0x0001)
	KindBond   = uint16(0x0003)
	KindETF    = uint16(0x0009)
)

type CodeInfo struct {
	Market uint16 `json:"market"`
	Code   string `json:"code"`
	Symbol string `json:"symbol"`
}

func ParseSymbol(symbol string) (CodeInfo, error) {
	raw := strings.TrimSpace(strings.ToLower(symbol))
	raw = strings.TrimPrefix(raw, "stock:")
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.ReplaceAll(raw, "_", "")
	if raw == "" {
		return CodeInfo{}, fmt.Errorf("empty symbol")
	}
	marketPrefix := ""
	code := raw
	if strings.HasPrefix(raw, "sh") || strings.HasPrefix(raw, "sz") {
		marketPrefix = raw[:2]
		code = raw[2:]
	}
	if len(code) != 6 {
		return CodeInfo{}, fmt.Errorf("symbol %q must contain a 6-digit code", symbol)
	}
	if marketPrefix == "" {
		if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "5") {
			marketPrefix = "sh"
		} else {
			marketPrefix = "sz"
		}
	}
	market := CodeTypeByRule(marketPrefix, code)
	if market == 0 {
		return CodeInfo{}, fmt.Errorf("unsupported symbol %q", symbol)
	}
	return CodeInfo{Market: market, Code: code, Symbol: marketPrefix + code}, nil
}

func CodeTypeByRule(exchange, code string) uint16 {
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	code = strings.TrimSpace(code)
	var bourse uint16
	switch exchange {
	case "sh":
		bourse = SHBourse
	case "sz":
		bourse = SZBourse
	default:
		return 0
	}
	switch {
	case exchange == "sh" && strings.HasPrefix(code, "000"):
		return StockMarket | bourse | KindIndex
	case exchange == "sz" && strings.HasPrefix(code, "399"):
		return StockMarket | bourse | KindIndex
	case exchange == "sh" && (strings.HasPrefix(code, "50") || strings.HasPrefix(code, "51") ||
		strings.HasPrefix(code, "52") || strings.HasPrefix(code, "53") ||
		strings.HasPrefix(code, "56") || strings.HasPrefix(code, "58")):
		return StockMarket | bourse | KindETF
	case exchange == "sz" && (strings.HasPrefix(code, "15") || strings.HasPrefix(code, "16")):
		return StockMarket | bourse | KindETF
	case exchange == "sh" && (strings.HasPrefix(code, "110") || strings.HasPrefix(code, "111") ||
		strings.HasPrefix(code, "113") || strings.HasPrefix(code, "118")):
		return StockMarket | bourse | KindBond
	case exchange == "sz" && (strings.HasPrefix(code, "123") || strings.HasPrefix(code, "125") ||
		strings.HasPrefix(code, "126") || strings.HasPrefix(code, "127") || strings.HasPrefix(code, "128")):
		return StockMarket | bourse | KindBond
	case exchange == "sh" && strings.HasPrefix(code, "6"):
		return StockMarket | bourse | KindStockA
	case exchange == "sz" && (strings.HasPrefix(code, "0") || strings.HasPrefix(code, "30")):
		return StockMarket | bourse | KindStockA
	default:
		return 0
	}
}

func EncodeCodeInfo(info CodeInfo) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint16(out[0:2], info.Market)
	copy(out[2:8], []byte(info.Code))
	return out
}

func ParseCodeInfo(data []byte) (CodeInfo, error) {
	if len(data) < 8 {
		return CodeInfo{}, fmt.Errorf("CodeInfo requires 8 bytes")
	}
	market := binary.LittleEndian.Uint16(data[0:2])
	code := strings.TrimRight(string(data[2:8]), "\x00 ")
	prefix := "xx"
	if market&SHBourse == SHBourse {
		prefix = "sh"
	} else if market&SZBourse == SZBourse {
		prefix = "sz"
	}
	return CodeInfo{Market: market, Code: code, Symbol: prefix + code}, nil
}
