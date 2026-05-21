package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"axonhub-patch-panel/internal/proxy"
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

	resolver := thread.NewResolver(store, thread.Options{
		KeyPrefix:             cfg.KeyPrefix,
		TTL:                   cfg.TTL,
		RespectExistingThread: cfg.RespectExistingThread,
	})

	proxyHandler, err := proxy.New(proxy.Options{
		UpstreamURL:          cfg.UpstreamURL,
		Resolver:             resolver,
		RespectExistingTrace: cfg.RespectExistingTrace,
	})
	if err != nil {
		log.Fatalf("create proxy: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/_panel/", http.StripPrefix("/_panel", ui.Handler(cfg.PublicConfig())))
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
	ListenAddr            string
	UpstreamURL           string
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	RedisTimeout          time.Duration
	KeyPrefix             string
	TTL                   time.Duration
	RespectExistingThread bool
	RespectExistingTrace  bool
}

func (c Config) PublicConfig() ui.Config {
	return ui.Config{
		UpstreamURL:           c.UpstreamURL,
		RedisAddr:             c.RedisAddr,
		KeyPrefix:             c.KeyPrefix,
		TTL:                   c.TTL.String(),
		RespectExistingThread: c.RespectExistingThread,
		RespectExistingTrace:  c.RespectExistingTrace,
	}
}

func loadConfig() Config {
	return Config{
		ListenAddr:            env("LISTEN_ADDR", ":8080"),
		UpstreamURL:           requiredEnv("AXONHUB_URL"),
		RedisAddr:             env("REDIS_ADDR", "redis:6379"),
		RedisPassword:         os.Getenv("REDIS_PASSWORD"),
		RedisDB:               envInt("REDIS_DB", 0),
		RedisTimeout:          envDuration("REDIS_TIMEOUT", 3*time.Second),
		KeyPrefix:             env("KEY_PREFIX", "ahpatch"),
		TTL:                   envDuration("THREAD_TTL", 30*24*time.Hour),
		RespectExistingThread: envBool("RESPECT_EXISTING_THREAD", true),
		RespectExistingTrace:  envBool("RESPECT_EXISTING_TRACE", false),
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
