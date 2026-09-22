package main

import (
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	stockMarket = uint16(0x1000)
	shBourse    = uint16(0x0100)
	szBourse    = uint16(0x0200)

	kindIndex  = uint16(0x0000)
	kindStockA = uint16(0x0001)
)

type CodeInfo struct {
	Market uint16 `json:"market"`
	Code   string `json:"code"`
	Symbol string `json:"symbol"`
}

func parseSymbol(symbol string) (CodeInfo, error) {
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
		if strings.HasPrefix(code, "6") {
			marketPrefix = "sh"
		} else {
			marketPrefix = "sz"
		}
	}

	market := stockMarket
	switch marketPrefix {
	case "sh":
		market |= shBourse
		if strings.HasPrefix(code, "000") {
			market |= kindIndex
		} else {
			market |= kindStockA
		}
	case "sz":
		market |= szBourse
		if strings.HasPrefix(code, "399") {
			market |= kindIndex
		} else {
			market |= kindStockA
		}
	default:
		return CodeInfo{}, fmt.Errorf("unsupported market prefix %q", marketPrefix)
	}

	return CodeInfo{
		Market: market,
		Code:   code,
		Symbol: marketPrefix + code,
	}, nil
}

func encodeCodeInfo(info CodeInfo) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint16(out[0:2], info.Market)
	copy(out[2:8], []byte(info.Code))
	return out
}

func parseCodeInfo(data []byte) (CodeInfo, error) {
	if len(data) < 8 {
		return CodeInfo{}, fmt.Errorf("CodeInfo requires 8 bytes")
	}
	market := binary.LittleEndian.Uint16(data[0:2])
	code := strings.TrimRight(string(data[2:8]), "\x00 ")
	prefix := "xx"
	if market&shBourse == shBourse {
		prefix = "sh"
	} else if market&szBourse == szBourse {
		prefix = "sz"
	}
	return CodeInfo{Market: market, Code: code, Symbol: prefix + code}, nil
}
