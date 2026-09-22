package quoteprotocol

import (
	"encoding/binary"
	"testing"
)

func TestBuildStockInfoListPayload(t *testing.T) {
	got := BuildStockInfoListPayload([]uint16{0x1101, 0x1201})
	want := []byte{0x01, 0x11, 0x01, 0x12}
	if string(got) != string(want) {
		t.Fatalf("payload = %x, want %x", got, want)
	}
}

func TestParseStockInfoList(t *testing.T) {
	body := make([]byte, 20+32)
	binary.LittleEndian.PutUint16(body[0:2], RT_STOCKINFOLIST)
	copy(body[20:36], []byte("sample"))
	binary.LittleEndian.PutUint16(body[36:38], 0x1101)
	copy(body[38:44], []byte("600570"))
	binary.LittleEndian.PutUint32(body[44:48], 20580)
	binary.LittleEndian.PutUint32(body[48:52], 12345)

	list, err := ParseStockInfoList(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	got := list[0]
	if got.CodeType != 0x1101 || got.Exchange != "sh" || got.Code != "600570" || got.PrevClose != 20.58 || got.FiveDayVol != 12345 {
		t.Fatalf("unexpected stock info: %+v", got)
	}
}
