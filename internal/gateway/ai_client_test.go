package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIClientComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected auth header %q", got)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Fatalf("unexpected model %v", payload["model"])
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Hello caller"}}]}`))
	}))
	defer server.Close()

	client := NewAIClient(AIConfig{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "gpt-test",
	})
	reply, err := client.Complete(context.Background(), "hi")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if reply != "Hello caller" {
		t.Fatalf("unexpected reply %q", reply)
	}
}

func TestAIClientCompleteRequiresConfig(t *testing.T) {
	client := NewAIClient(AIConfig{Model: "gpt-test"})
	_, err := client.Complete(context.Background(), "hi")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not configured") {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestAIClientCompleteRequiresPrompt(t *testing.T) {
	client := NewAIClient(AIConfig{APIKey: "secret", Model: "gpt-test"})
	_, err := client.Complete(context.Background(), "  ")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "prompt") {
		t.Fatalf("expected prompt error, got %v", err)
	}
}
