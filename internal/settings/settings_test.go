package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerPersistsAndLoadsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	defaults := Settings{
		ThreadEnabled:                true,
		TraceEnabled:                 true,
		KeyPrefix:                    "ahpatch",
		ThreadTTL:                    "720h",
		RespectExistingThread:        true,
		RespectExistingTrace:         false,
		ClaudeThinkingRewriteEnabled: false,
		ClaudeThinkingRewriteModels:  []string{"claude-opus-4-7"},
		ClaudeThinkingRewriteEffort:  "xhigh",
	}

	manager, err := NewManager(path, defaults)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	next := defaults
	next.ClaudeThinkingRewriteEnabled = true
	next.ClaudeThinkingRewriteModels = []string{"claude-opus-4-7", "claude-sonnet-4-6"}
	next.ClaudeThinkingRewriteEffort = "max"
	if err := manager.Update(next); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected persisted config")
	}

	reloaded, err := NewManager(path, defaults)
	if err != nil {
		t.Fatalf("NewManager reload returned error: %v", err)
	}
	got := reloaded.Current()
	if !got.ClaudeThinkingRewriteEnabled {
		t.Fatal("rewrite should be enabled after reload")
	}
	if got.ClaudeThinkingRewriteEffort != "max" {
		t.Fatalf("effort mismatch: %s", got.ClaudeThinkingRewriteEffort)
	}
	if len(got.ClaudeThinkingRewriteModels) != 2 {
		t.Fatalf("models mismatch: %#v", got.ClaudeThinkingRewriteModels)
	}
}

func TestModelsFromTextSplitsAndDeduplicates(t *testing.T) {
	got := ModelsFromText("claude-opus-4-7\nclaude-sonnet-4-6, claude-opus-4-7")
	want := []string{"claude-opus-4-7", "claude-sonnet-4-6"}
	if len(got) != len(want) {
		t.Fatalf("length mismatch\nwant: %#v\n got: %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("model[%d] mismatch\nwant: %s\n got: %s", i, want[i], got[i])
		}
	}
}

func TestNormalizeRejectsEnabledRewriteWithoutModels(t *testing.T) {
	_, err := Normalize(Settings{
		ThreadEnabled:                true,
		TraceEnabled:                 true,
		KeyPrefix:                    "ahpatch",
		ThreadTTL:                    "720h",
		ClaudeThinkingRewriteEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
