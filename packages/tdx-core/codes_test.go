package tdx

import "testing"

func TestCodeTypeByRule(t *testing.T) {
	tests := []struct {
		exchange string
		code     string
		want     uint16
	}{
		{exchange: "sh", code: "600570", want: 0x1101},
		{exchange: "sz", code: "000001", want: 0x1201},
		{exchange: "sh", code: "000001", want: 0x1100},
		{exchange: "sz", code: "399001", want: 0x1200},
		{exchange: "sh", code: "510300", want: 0x1109},
		{exchange: "sz", code: "159915", want: 0x1209},
	}

	for _, tt := range tests {
		if got := CodeTypeByRule(tt.exchange, tt.code); got != tt.want {
			t.Fatalf("CodeTypeByRule(%q, %q) = 0x%04X, want 0x%04X", tt.exchange, tt.code, got, tt.want)
		}
	}
}
