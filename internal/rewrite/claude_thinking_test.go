package rewrite

import (
	"encoding/json"
	"testing"
)

func TestRewriteClaudeThinkingConvertsEnabledToAdaptive(t *testing.T) {
	body := []byte(`{
		"model":"claude-opus-4-7",
		"max_tokens":2048,
		"stream":true,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[{"role":"user","content":"Solve gcd"}]
	}`)

	got, changed, err := RewriteClaudeThinking(body, "/v1/messages", ClaudeThinkingOptions{
		Enabled: true,
		Models:  []string{"claude-opus-4-7"},
		Effort:  "xhigh",
	})
	if err != nil {
		t.Fatalf("RewriteClaudeThinking returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected body to be changed")
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("rewritten body is not JSON: %v", err)
	}
	thinking := payload["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Fatalf("thinking.type mismatch: %v", thinking["type"])
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Fatal("thinking.budget_tokens should be removed")
	}
	outputConfig := payload["output_config"].(map[string]any)
	if outputConfig["effort"] != "xhigh" {
		t.Fatalf("output_config.effort mismatch: %v", outputConfig["effort"])
	}
}

func TestRewriteClaudeThinkingSkipsOtherModels(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-6","thinking":{"type":"enabled","budget_tokens":1024}}`)

	got, changed, err := RewriteClaudeThinking(body, "/v1/messages", ClaudeThinkingOptions{
		Enabled: true,
		Models:  []string{"claude-opus-4-7"},
		Effort:  "xhigh",
	})
	if err != nil {
		t.Fatalf("RewriteClaudeThinking returned error: %v", err)
	}
	if changed {
		t.Fatal("expected body to be unchanged")
	}
	if string(got) != string(body) {
		t.Fatalf("body changed unexpectedly\nwant: %s\n got: %s", body, got)
	}
}

func TestRewriteClaudeThinkingSkipsWhenDisabled(t *testing.T) {
	body := []byte(`{"model":"claude-opus-4-7","thinking":{"type":"enabled","budget_tokens":1024}}`)

	got, changed, err := RewriteClaudeThinking(body, "/v1/messages", ClaudeThinkingOptions{
		Enabled: false,
		Models:  []string{"claude-opus-4-7"},
		Effort:  "xhigh",
	})
	if err != nil {
		t.Fatalf("RewriteClaudeThinking returned error: %v", err)
	}
	if changed {
		t.Fatal("expected body to be unchanged")
	}
	if string(got) != string(body) {
		t.Fatalf("body changed unexpectedly\nwant: %s\n got: %s", body, got)
	}
}
