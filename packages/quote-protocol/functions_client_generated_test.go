package quoteprotocol

import (
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedFunctionCallMethodsCoverDefinitions(t *testing.T) {
	clientType := reflect.TypeOf(&Client{})
	count := 0
	for i := 0; i < clientType.NumMethod(); i++ {
		if strings.HasPrefix(clientType.Method(i).Name, "CallRT_") {
			count++
		}
	}
	if count != len(Functions()) {
		t.Fatalf("generated CallRT_* method count = %d, want %d", count, len(Functions()))
	}
}
