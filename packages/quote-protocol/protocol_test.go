package quoteprotocol

import (
	"encoding/binary"
	"testing"
)

func TestBuildAskData(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	body := BuildAskData(RT_SERVERTIME, 7, 3, 2, payload)
	if got, want := len(body), 23; got != want {
		t.Fatalf("len(body) = %d, want %d", got, want)
	}
	if got := binary.LittleEndian.Uint16(body[0:2]); got != RT_SERVERTIME {
		t.Fatalf("type = 0x%04X, want 0x%04X", got, RT_SERVERTIME)
	}
	if got := body[2]; got != 7 {
		t.Fatalf("index = %d, want 7", got)
	}
	if got := int16(binary.LittleEndian.Uint16(body[16:18])); got != 3 {
		t.Fatalf("size = %d, want 3", got)
	}
	if got := int16(binary.LittleEndian.Uint16(body[18:20])); got != 2 {
		t.Fatalf("option = %d, want 2", got)
	}
	if got := body[20:]; string(got) != string(payload) {
		t.Fatalf("payload = %x, want %x", got, payload)
	}
}

func TestBuildKeepAlive(t *testing.T) {
	body := BuildKeepAlive(9)
	if got, want := len(body), 4; got != want {
		t.Fatalf("len(body) = %d, want %d", got, want)
	}
	if got := binary.LittleEndian.Uint16(body[0:2]); got != RT_KEEPACTIVE {
		t.Fatalf("type = 0x%04X, want 0x%04X", got, RT_KEEPACTIVE)
	}
	if body[2] != 9 || body[3] != 1 {
		t.Fatalf("operator bytes = %x, want index=9 operator=1", body[2:])
	}
}

func TestParseFunction(t *testing.T) {
	tests := map[string]uint16{
		"RT_LOGIN":      RT_LOGIN,
		"rt_servertime": RT_SERVERTIME,
		"0902":          RT_SERVERTIME,
		"0x120F":        RT_REALTIME_EXT_INT64,
		"120F":          RT_REALTIME_EXT_INT64,
		"4623":          RT_REALTIME_EXT_INT64,
	}
	for input, want := range tests {
		got, err := ParseFunction(input)
		if err != nil {
			t.Fatalf("ParseFunction(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseFunction(%q) = 0x%04X, want 0x%04X", input, got, want)
		}
	}
}

func TestParseAndEncodeSymbol(t *testing.T) {
	info, err := ParseSymbol("sh600570")
	if err != nil {
		t.Fatal(err)
	}
	if info.Market != StockMarket|SHBourse|KindStockA {
		t.Fatalf("market = 0x%04X", info.Market)
	}
	roundTrip, err := ParseCodeInfo(EncodeCodeInfo(info))
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.Symbol != "sh600570" {
		t.Fatalf("symbol = %q, want sh600570", roundTrip.Symbol)
	}
}
