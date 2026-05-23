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

func TestResolverReusesThreadWhenOnlySystemWrapperChanges(t *testing.T) {
	store := NewMemoryStore()
	threads := []string{"thread-first", "thread-second"}
	index := 0
	resolver := NewResolver(store, Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			id := threads[index]
			if index < len(threads)-1 {
				index++
			}
			return id
		},
	})

	first := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。剩余预算 120000。"},
			{Role: "user", Text: "去把美国 netcup 那个 axonhub 的补丁容器找出来"},
		},
	}
	firstResult, err := resolver.Resolve(context.Background(), first, nil)
	if err != nil {
		t.Fatalf("Resolve first returned error: %v", err)
	}

	second := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date><elapsed_seconds>31</elapsed_seconds></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。剩余预算 119500。"},
			{Role: "user", Text: "去把美国 netcup 那个 axonhub 的补丁容器找出来"},
		},
	}
	secondResult, err := resolver.Resolve(context.Background(), second, nil)
	if err != nil {
		t.Fatalf("Resolve second returned error: %v", err)
	}

	if secondResult.ThreadID != firstResult.ThreadID {
		t.Fatalf("expected same thread when only wrapper changes\nfirst: %s\nsecond: %s", firstResult.ThreadID, secondResult.ThreadID)
	}
}

func TestResolverReusesRememberedStateWhenWrapperChanges(t *testing.T) {
	store := NewMemoryStore()
	resolver := NewResolver(store, Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-first"
		},
	})

	first := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。剩余预算 120000。"},
			{Role: "user", Text: "去把美国 netcup 那个 axonhub 的补丁容器找出来"},
		},
	}
	firstResult, err := resolver.Resolve(context.Background(), first, nil)
	if err != nil {
		t.Fatalf("Resolve first returned error: %v", err)
	}

	stateHash, _, err := normalize.StateAfterResponse(first, []byte(`{"id":"resp_1","output":[{"role":"assistant","content":"收到"}]}`))
	if err != nil {
		t.Fatalf("StateAfterResponse returned error: %v", err)
	}
	if err := resolver.RememberState(context.Background(), stateHash, "resp_1", firstResult.ThreadID); err != nil {
		t.Fatalf("RememberState returned error: %v", err)
	}

	second := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date><elapsed_seconds>31</elapsed_seconds></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。剩余预算 119500。"},
			{Role: "user", Text: "去把美国 netcup 那个 axonhub 的补丁容器找出来"},
			{Role: "assistant", Text: "收到"},
			{Role: "user", Text: "继续"},
		},
	}
	secondResult, err := resolver.Resolve(context.Background(), second, nil)
	if err != nil {
		t.Fatalf("Resolve second returned error: %v", err)
	}

	if secondResult.ThreadID != firstResult.ThreadID {
		t.Fatalf("expected same thread from remembered state when only wrapper changes\nfirst: %s\nsecond: %s", firstResult.ThreadID, secondResult.ThreadID)
	}
}

func TestResolverDoesNotMergeDifferentFirstUserTurnsWithSameWrapper(t *testing.T) {
	store := NewMemoryStore()
	threads := []string{"thread-first", "thread-second"}
	index := 0
	resolver := NewResolver(store, Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			id := threads[index]
			if index < len(threads)-1 {
				index++
			}
			return id
		},
	})

	first := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。"},
			{Role: "user", Text: "去把美国 netcup 那个 axonhub 的补丁容器找出来"},
		},
	}
	firstResult, err := resolver.Resolve(context.Background(), first, nil)
	if err != nil {
		t.Fatalf("Resolve first returned error: %v", err)
	}

	second := normalize.Document{
		Format: normalize.FormatOpenAIResponses,
		Messages: []normalize.Message{
			{Role: "system", Text: "<environment_context><current_date>2026-05-24</current_date></environment_context>"},
			{Role: "developer", Text: "你是一个编码助手。"},
			{Role: "user", Text: "帮我检查这个 Go 补丁为什么不并线程"},
		},
	}
	secondResult, err := resolver.Resolve(context.Background(), second, nil)
	if err != nil {
		t.Fatalf("Resolve second returned error: %v", err)
	}

	if secondResult.ThreadID == firstResult.ThreadID {
		t.Fatalf("expected different first user turns to create different threads, both got %s", secondResult.ThreadID)
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
