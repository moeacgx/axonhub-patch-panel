package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	ThreadEnabled                bool     `json:"threadEnabled"`
	TraceEnabled                 bool     `json:"traceEnabled"`
	KeyPrefix                    string   `json:"keyPrefix"`
	ThreadTTL                    string   `json:"threadTtl"`
	RespectExistingThread        bool     `json:"respectExistingThread"`
	RespectExistingTrace         bool     `json:"respectExistingTrace"`
	ClaudeThinkingRewriteEnabled bool     `json:"claudeThinkingRewriteEnabled"`
	ClaudeThinkingRewriteModels  []string `json:"claudeThinkingRewriteModels"`
	ClaudeThinkingRewriteEffort  string   `json:"claudeThinkingRewriteEffort"`
}

type Manager struct {
	mu       sync.RWMutex
	path     string
	settings Settings
}

func NewManager(path string, defaults Settings) (*Manager, error) {
	normalized, err := Normalize(defaults)
	if err != nil {
		return nil, err
	}
	manager := &Manager{path: path, settings: normalized}
	if strings.TrimSpace(path) == "" {
		return manager, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, err
	}
	var loaded Settings
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, err
	}
	normalized, err = Normalize(mergeDefaults(loaded, normalized))
	if err != nil {
		return nil, err
	}
	manager.settings = normalized
	return manager, nil
}

func (m *Manager) Current() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneSettings(m.settings)
}

func (m *Manager) Update(next Settings) error {
	normalized, err := Normalize(next)
	if err != nil {
		return err
	}
	if m.path != "" {
		if err := writeJSON(m.path, normalized); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.settings = normalized
	m.mu.Unlock()
	return nil
}

func Normalize(in Settings) (Settings, error) {
	out := in
	out.KeyPrefix = strings.TrimSpace(out.KeyPrefix)
	if out.KeyPrefix == "" {
		out.KeyPrefix = "ahpatch"
	}
	out.ThreadTTL = strings.TrimSpace(out.ThreadTTL)
	if out.ThreadTTL == "" {
		out.ThreadTTL = "720h"
	}
	if _, err := out.ThreadTTLDuration(); err != nil {
		return Settings{}, err
	}
	out.ClaudeThinkingRewriteModels = normalizeList(out.ClaudeThinkingRewriteModels)
	out.ClaudeThinkingRewriteEffort = strings.TrimSpace(out.ClaudeThinkingRewriteEffort)
	if out.ClaudeThinkingRewriteEffort == "" {
		out.ClaudeThinkingRewriteEffort = "xhigh"
	}
	if out.ClaudeThinkingRewriteEnabled && len(out.ClaudeThinkingRewriteModels) == 0 {
		return Settings{}, fmt.Errorf("claude thinking rewrite models is required when rewrite is enabled")
	}
	return out, nil
}

func (s Settings) ThreadTTLDuration() (time.Duration, error) {
	ttl, err := time.ParseDuration(s.ThreadTTL)
	if err == nil {
		return ttl, nil
	}
	seconds, intErr := time.ParseDuration(s.ThreadTTL + "s")
	if intErr == nil {
		return seconds, nil
	}
	return 0, fmt.Errorf("invalid thread ttl %q: %v", s.ThreadTTL, err)
}

func ModelsFromText(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	return normalizeList(fields)
}

func ModelsText(models []string) string {
	return strings.Join(normalizeList(models), "\n")
}

func mergeDefaults(loaded, defaults Settings) Settings {
	if loaded.KeyPrefix == "" {
		loaded.KeyPrefix = defaults.KeyPrefix
	}
	if loaded.ThreadTTL == "" {
		loaded.ThreadTTL = defaults.ThreadTTL
	}
	if len(loaded.ClaudeThinkingRewriteModels) == 0 {
		loaded.ClaudeThinkingRewriteModels = defaults.ClaudeThinkingRewriteModels
	}
	if loaded.ClaudeThinkingRewriteEffort == "" {
		loaded.ClaudeThinkingRewriteEffort = defaults.ClaudeThinkingRewriteEffort
	}
	return loaded
}

func normalizeList(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cloneSettings(in Settings) Settings {
	out := in
	out.ClaudeThinkingRewriteModels = append([]string{}, in.ClaudeThinkingRewriteModels...)
	return out
}

func writeJSON(path string, value Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}
