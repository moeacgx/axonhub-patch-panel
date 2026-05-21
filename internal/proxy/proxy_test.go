package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"axonhub-patch-panel/internal/rewrite"
	"axonhub-patch-panel/internal/thread"
)

func TestProxyInjectsThreadAndTraceHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("AH-Thread-Id"); got != "thread-1" {
			t.Fatalf("thread header mismatch: %q", got)
		}
		if got := r.Header.Get("AH-Trace-Id"); !strings.HasPrefix(got, "at-") {
			t.Fatalf("trace header mismatch: %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	resolver := thread.NewResolver(thread.NewMemoryStore(), thread.Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-1"
		},
	})

	handler, err := New(Options{
		UpstreamURL: upstream.URL,
		Resolver:    resolver,
		NewTraceID: func() string {
			return "at-test-trace"
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status mismatch: %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) == "" {
		t.Fatal("empty response body")
	}
}

func TestProxyStreamsResponseWithoutBuffering(t *testing.T) {
	firstChunkSent := make(chan struct{})
	finishResponse := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(firstChunkSent)
		<-finishResponse
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	resolver := thread.NewResolver(thread.NewMemoryStore(), thread.Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-stream"
		},
	})
	handler, err := New(Options{
		UpstreamURL: upstream.URL,
		Resolver:    resolver,
		NewTraceID: func() string {
			return "at-stream"
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"stream":true}`))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	<-firstChunkSent
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil {
		t.Fatalf("Read first chunk returned error: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "Hel") {
		t.Fatalf("first chunk mismatch: %q", string(buf[:n]))
	}

	close(finishResponse)
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if !strings.Contains(string(rest), "[DONE]") {
		t.Fatalf("missing final stream chunk: %q", string(rest))
	}
}

func TestProxyRewritesClaudeThinkingBeforeForwarding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid upstream body: %v", err)
		}
		thinking := payload["thinking"].(map[string]any)
		if thinking["type"] != "adaptive" {
			t.Fatalf("thinking.type mismatch: %v", thinking["type"])
		}
		if _, ok := thinking["budget_tokens"]; ok {
			t.Fatal("thinking.budget_tokens should not be forwarded")
		}
		outputConfig := payload["output_config"].(map[string]any)
		if outputConfig["effort"] != "xhigh" {
			t.Fatalf("output_config.effort mismatch: %v", outputConfig["effort"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","content":[{"type":"text","text":"ok"}]}`))
	}))
	defer upstream.Close()

	resolver := thread.NewResolver(thread.NewMemoryStore(), thread.Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-rewrite"
		},
	})
	handler, err := New(Options{
		UpstreamURL: upstream.URL,
		Resolver:    resolver,
		NewTraceID: func() string {
			return "at-rewrite"
		},
		ClaudeThinking: rewrite.ClaudeThinkingOptions{
			Enabled: true,
			Models:  []string{"claude-opus-4-7"},
			Effort:  "xhigh",
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model":"claude-opus-4-7",
		"max_tokens":2048,
		"stream":true,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status mismatch: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProxyRuntimeCanDisableTraceHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("AH-Thread-Id"); got == "" {
			t.Fatal("thread header should still be injected")
		}
		if got := r.Header.Get("AH-Trace-Id"); got != "" {
			t.Fatalf("trace header should be disabled, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	resolver := thread.NewResolver(thread.NewMemoryStore(), thread.Options{
		KeyPrefix: "test",
		TTL:       time.Hour,
		NewThreadID: func() string {
			return "thread-runtime"
		},
	})
	handler, err := New(Options{
		UpstreamURL: upstream.URL,
		Resolver:    resolver,
		NewTraceID: func() string {
			return "at-runtime"
		},
		RuntimeOptions: func() RuntimeOptions {
			return RuntimeOptions{
				ThreadEnabled: true,
				TraceEnabled:  false,
			}
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status mismatch: %d body=%s", rec.Code, rec.Body.String())
	}
}
