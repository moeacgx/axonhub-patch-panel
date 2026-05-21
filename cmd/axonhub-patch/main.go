package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"axonhub-patch-panel/internal/proxy"
	"axonhub-patch-panel/internal/rewrite"
	"axonhub-patch-panel/internal/settings"
	"axonhub-patch-panel/internal/thread"
	"axonhub-patch-panel/internal/ui"
)

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := thread.NewRedisStore(ctx, thread.RedisOptions{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		Timeout:  cfg.RedisTimeout,
	})
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	defer store.Close()

	settingsManager, err := settings.NewManager(cfg.SettingsPath, cfg.DefaultSettings())
	if err != nil {
		log.Fatalf("load settings: %v", err)
	}

	resolver := thread.NewResolver(store, thread.Options{
		KeyPrefix: cfg.DefaultSettings().KeyPrefix,
		TTL:       cfg.DefaultTTL(),
	})
	resolver.SetOptionsFunc(func() thread.Options {
		current := settingsManager.Current()
		ttl, err := current.ThreadTTLDuration()
		if err != nil {
			ttl = cfg.DefaultTTL()
		}
		return thread.Options{
			KeyPrefix:             current.KeyPrefix,
			TTL:                   ttl,
			RespectExistingThread: current.RespectExistingThread,
		}
	})

	proxyHandler, err := proxy.New(proxy.Options{
		UpstreamURL: cfg.UpstreamURL,
		Resolver:    resolver,
		RuntimeOptions: func() proxy.RuntimeOptions {
			current := settingsManager.Current()
			return proxy.RuntimeOptions{
				ThreadEnabled:        current.ThreadEnabled,
				TraceEnabled:         current.TraceEnabled,
				RespectExistingTrace: current.RespectExistingTrace,
				ClaudeThinking: rewrite.ClaudeThinkingOptions{
					Enabled: current.ClaudeThinkingRewriteEnabled,
					Models:  current.ClaudeThinkingRewriteModels,
					Effort:  current.ClaudeThinkingRewriteEffort,
				},
			}
		},
	})
	if err != nil {
		log.Fatalf("create proxy: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/_panel/", http.StripPrefix("/_panel", ui.Handler(ui.Options{
		Config: func() ui.Config {
			return ui.Config{
				UpstreamURL: cfg.UpstreamURL,
				RedisAddr:   cfg.RedisAddr,
				Settings:    settingsManager.Current(),
			}
		},
		Update:       settingsManager.Update,
		Username:     cfg.PanelUsername,
		Password:     cfg.PanelPassword,
		AuthRequired: cfg.PanelPassword != "",
	})))
	mux.Handle("/", proxyHandler)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("axonhub patch panel listening on %s, upstream=%s", cfg.ListenAddr, cfg.UpstreamURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

type Config struct {
	ListenAddr             string
	UpstreamURL            string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	RedisTimeout           time.Duration
	SettingsPath           string
	PanelUsername          string
	PanelPassword          string
	DefaultRuntimeSettings settings.Settings
}

func (c Config) DefaultSettings() settings.Settings {
	return c.DefaultRuntimeSettings
}

func (c Config) DefaultTTL() time.Duration {
	ttl, err := c.DefaultRuntimeSettings.ThreadTTLDuration()
	if err != nil {
		return 30 * 24 * time.Hour
	}
	return ttl
}

func loadConfig() Config {
	return Config{
		ListenAddr:    env("LISTEN_ADDR", ":8080"),
		UpstreamURL:   requiredEnv("AXONHUB_URL"),
		RedisAddr:     env("REDIS_ADDR", "redis:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       envInt("REDIS_DB", 0),
		RedisTimeout:  envDuration("REDIS_TIMEOUT", 3*time.Second),
		SettingsPath:  env("SETTINGS_PATH", "/data/config.json"),
		PanelUsername: env("PANEL_USERNAME", "admin"),
		PanelPassword: os.Getenv("PANEL_PASSWORD"),
		DefaultRuntimeSettings: settings.Settings{
			ThreadEnabled:                envBool("THREAD_ENABLED", true),
			TraceEnabled:                 envBool("TRACE_ENABLED", true),
			KeyPrefix:                    env("KEY_PREFIX", "ahpatch"),
			ThreadTTL:                    env("THREAD_TTL", "720h"),
			RespectExistingThread:        envBool("RESPECT_EXISTING_THREAD", true),
			RespectExistingTrace:         envBool("RESPECT_EXISTING_TRACE", false),
			ClaudeThinkingRewriteEnabled: envBool("CLAUDE_THINKING_REWRITE_ENABLED", false),
			ClaudeThinkingRewriteModels:  envList("CLAUDE_THINKING_REWRITE_MODELS", "claude-opus-4-7"),
			ClaudeThinkingRewriteEffort:  env("CLAUDE_THINKING_REWRITE_EFFORT", "xhigh"),
		},
	}
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return n
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return b
}

func envList(key, fallback string) []string {
	value := env(key, fallback)
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err == nil {
		return d
	}
	seconds, intErr := strconv.Atoi(value)
	if intErr == nil {
		return time.Duration(seconds) * time.Second
	}
	log.Fatalf("invalid %s: %v; %v", key, err, intErr)
	return fallback
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	fmt.Print("")
}
