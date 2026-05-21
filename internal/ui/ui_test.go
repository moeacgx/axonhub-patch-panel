package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"axonhub-patch-panel/internal/settings"
)

func TestHandlerRequiresBasicAuth(t *testing.T) {
	handler := Handler(Options{
		Config:       testConfigFunc(settings.Settings{ThreadEnabled: true, TraceEnabled: true, KeyPrefix: "ahpatch", ThreadTTL: "720h"}),
		AuthRequired: true,
		Username:     "admin",
		Password:     "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: %d", rec.Code)
	}
}

func TestHandlerAllowsBasicAuth(t *testing.T) {
	handler := Handler(Options{
		Config:       testConfigFunc(settings.Settings{ThreadEnabled: true, TraceEnabled: true, KeyPrefix: "ahpatch", ThreadTTL: "720h"}),
		AuthRequired: true,
		Username:     "admin",
		Password:     "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status mismatch: %d", rec.Code)
	}
}

func TestHandlerFormUpdate(t *testing.T) {
	current := settings.Settings{
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
	handler := Handler(Options{
		Config: testConfigFunc(current),
		Update: func(next settings.Settings) error {
			current = next
			return nil
		},
	})

	form := url.Values{}
	form.Set("threadEnabled", "on")
	form.Set("traceEnabled", "on")
	form.Set("keyPrefix", "ahpatch")
	form.Set("threadTtl", "720h")
	form.Set("claudeThinkingRewriteEnabled", "on")
	form.Set("claudeThinkingRewriteModels", "claude-opus-4-7\nclaude-sonnet-4-6")
	form.Set("claudeThinkingRewriteEffort", "max")
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status mismatch: %d", rec.Code)
	}
	if !current.ClaudeThinkingRewriteEnabled {
		t.Fatal("rewrite should be enabled")
	}
	if current.ClaudeThinkingRewriteEffort != "max" {
		t.Fatalf("effort mismatch: %s", current.ClaudeThinkingRewriteEffort)
	}
	if len(current.ClaudeThinkingRewriteModels) != 2 {
		t.Fatalf("models mismatch: %#v", current.ClaudeThinkingRewriteModels)
	}
}

func testConfigFunc(s settings.Settings) func() Config {
	return func() Config {
		return Config{
			UpstreamURL: "http://axonhub-app:8090",
			RedisAddr:   "redis:6379",
			Settings:    s,
		}
	}
}
