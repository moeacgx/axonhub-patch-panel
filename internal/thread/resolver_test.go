package thread

import (
	"context"
	"testing"
	"time"

	"axonhub-patch-panel/internal/normalize"
)

func TestResolverReusesThreadFromPreviousStateHash(t *testing.T) {
	store := NewMemoryStore()
	resolver := NewResolver(store, Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-new"
		},
	})

	first := normalize.Document{
		Format: normalize.FormatOpenAIChat,
		Messages: []normalize.Message{
			{Role: "user", Text: "First"},
		},
	}
	firstResult, err := resolver.Resolve(context.Background(), first, nil)
	if err != nil {
		t.Fatalf("Resolve first returned error: %v", err)
	}
	if firstResult.ThreadID != "thread-new" {
		t.Fatalf("thread id mismatch: %s", firstResult.ThreadID)
	}

	stateHash, _, err := normalize.StateAfterResponse(first, []byte(`{"choices":[{"message":{"role":"assistant","content":"Second"}}]}`))
	if err != nil {
		t.Fatalf("StateAfterResponse returned error: %v", err)
	}
	if err := resolver.RememberState(context.Background(), stateHash, "", firstResult.ThreadID); err != nil {
		t.Fatalf("RememberState returned error: %v", err)
	}

	second := normalize.Document{
		Format: normalize.FormatOpenAIChat,
		Messages: []normalize.Message{
			{Role: "user", Text: "First"},
			{Role: "assistant", Text: "Second"},
			{Role: "user", Text: "Third"},
		},
	}
	secondResult, err := resolver.Resolve(context.Background(), second, nil)
	if err != nil {
		t.Fatalf("Resolve second returned error: %v", err)
	}
	if secondResult.ThreadID != firstResult.ThreadID {
		t.Fatalf("expected same thread\nfirst: %s\nsecond: %s", firstResult.ThreadID, secondResult.ThreadID)
	}
}

func TestResolverUsesExistingThreadHeader(t *testing.T) {
	store := NewMemoryStore()
	resolver := NewResolver(store, Options{
		KeyPrefix:             "test",
		TTL:                   time.Hour,
		RespectExistingThread: true,
		NewThreadID: func() string {
			return "thread-new"
		},
	})

	result, err := resolver.Resolve(context.Background(), normalize.Document{}, map[string]string{
		"AH-Thread-Id": "thread-existing",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if result.ThreadID != "thread-existing" {
		t.Fatalf("thread id mismatch: %s", result.ThreadID)
	}
	if result.Source != SourceExistingHeader {
		t.Fatalf("source mismatch: %s", result.Source)
	}
}

func TestResolverUsesDynamicOptions(t *testing.T) {
	store := NewMemoryStore()
	opts := Options{
		KeyPrefix: "before",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-before"
		},
	}
	resolver := NewResolver(store, opts)
	resolver.SetOptionsFunc(func() Options {
		return opts
	})

	opts = Options{
		KeyPrefix: "after",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-after"
		},
	}
	result, err := resolver.Resolve(context.Background(), normalize.Document{
		Format: normalize.FormatOpenAIChat,
		Messages: []normalize.Message{
			{Role: "user", Text: "Hello"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if result.ThreadID != "thread-after" {
		t.Fatalf("thread id mismatch: %s", result.ThreadID)
	}
}
