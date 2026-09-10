package sender

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTransportsExposeOnlyConfiguredProfilesWithoutCredentials(t *testing.T) {
	cfg := &Config{Providers: map[string]*ProviderConfig{
		"mail":    {Provider: "smtp", Params: map[string]any{"from": "sales@example.com", "password": "SECRET", "username": "SECRET"}},
		"beeline": {Provider: "beeline", Params: map[string]any{"password": "SECRET"}},
		"missing": nil,
	}}
	items := cfg.Transports()
	if len(items) != 2 || items[0].Code != "beeline" || items[1].Sender != "sales@example.com" || !items[0].Available {
		t.Fatalf("unexpected catalog: %+v", items)
	}
	data, err := json.Marshal(items)
	if err != nil || strings.Contains(string(data), "SECRET") {
		t.Fatalf("catalog leaks credentials or cannot be encoded: %s, %v", data, err)
	}
	if empty, _ := json.Marshal((&Config{}).Transports()); string(empty) != "[]" {
		t.Fatalf("empty catalog must be an array: %s", empty)
	}
}
