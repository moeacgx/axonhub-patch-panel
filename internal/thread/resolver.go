package thread

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"axonhub-patch-panel/internal/normalize"
)

const (
	SourceExistingHeader Source = "existing_header"
	SourceStateHash      Source = "state_hash"
	SourceResponseID     Source = "response_id"
	SourceSessionID      Source = "session_id"
	SourceCreated        Source = "created"
)

type Source string

type Options struct {
	KeyPrefix             string
	TTL                   time.Duration
	RespectExistingThread bool
	NewThreadID           func() string
}

type Resolver struct {
	store       Store
	opts        Options
	optionsFunc func() Options
}

type Result struct {
	ThreadID string
	Source   Source
}

func NewResolver(store Store, opts Options) *Resolver {
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "ahpatch"
	}
	if opts.TTL == 0 {
		opts.TTL = 30 * 24 * time.Hour
	}
	if opts.NewThreadID == nil {
		opts.NewThreadID = defaultThreadID
	}
	return &Resolver{store: store, opts: opts}
}

func (r *Resolver) SetOptionsFunc(fn func() Options) {
	r.optionsFunc = fn
}

func (r *Resolver) Resolve(ctx context.Context, doc normalize.Document, headers map[string]string) (Result, error) {
	opts := r.options()
	if opts.RespectExistingThread {
		if existing := headerValue(headers, "AH-Thread-Id"); existing != "" {
			return Result{ThreadID: existing, Source: SourceExistingHeader}, nil
		}
	}

	if doc.ResponseID != "" {
		if threadID, ok, err := r.lookup(ctx, r.keyWithOptions(opts, "response", doc.ResponseID)); ok || err != nil {
			return Result{ThreadID: threadID, Source: SourceResponseID}, err
		}
	}
	if doc.SessionID != "" {
		if threadID, ok, err := r.lookup(ctx, r.keyWithOptions(opts, "session", doc.SessionID)); ok || err != nil {
			return Result{ThreadID: threadID, Source: SourceSessionID}, err
		}
	}

	lookupHash, err := normalize.LookupHash(doc)
	if err != nil {
		return Result{}, err
	}
	if lookupHash != "" {
		if threadID, ok, err := r.lookup(ctx, r.keyWithOptions(opts, "state", lookupHash)); ok || err != nil {
			return Result{ThreadID: threadID, Source: SourceStateHash}, err
		}
	}

	threadID := opts.NewThreadID()
	if lookupHash != "" {
		key := r.keyWithOptions(opts, "state", lookupHash)
		ok, err := r.store.SetNX(ctx, key, threadID, opts.TTL)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			existing, err := r.store.Get(ctx, key)
			if err != nil {
				return Result{}, err
			}
			return Result{ThreadID: existing, Source: SourceStateHash}, nil
		}
	}
	if doc.ResponseID != "" {
		_ = r.store.Set(ctx, r.keyWithOptions(opts, "response", doc.ResponseID), threadID, opts.TTL)
	}
	if doc.SessionID != "" {
		_ = r.store.Set(ctx, r.keyWithOptions(opts, "session", doc.SessionID), threadID, opts.TTL)
	}
	return Result{ThreadID: threadID, Source: SourceCreated}, nil
}

func (r *Resolver) RememberState(ctx context.Context, stateHash, responseID, threadID string) error {
	opts := r.options()
	if threadID == "" {
		return nil
	}
	if stateHash != "" {
		if err := r.store.Set(ctx, r.keyWithOptions(opts, "state", stateHash), threadID, opts.TTL); err != nil {
			return err
		}
	}
	if responseID != "" {
		if err := r.store.Set(ctx, r.keyWithOptions(opts, "response", responseID), threadID, opts.TTL); err != nil {
			return err
		}
	}
	return nil
}

func (r *Resolver) lookup(ctx context.Context, key string) (string, bool, error) {
	value, err := r.store.Get(ctx, key)
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return "", false, nil
	}
	return "", false, err
}

func (r *Resolver) key(kind, value string) string {
	return r.keyWithOptions(r.options(), kind, value)
}

func (r *Resolver) keyWithOptions(opts Options, kind, value string) string {
	return fmt.Sprintf("%s:%s:%s", opts.KeyPrefix, kind, value)
}

func (r *Resolver) options() Options {
	opts := r.opts
	if r.optionsFunc != nil {
		opts = r.optionsFunc()
	}
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "ahpatch"
	}
	if opts.TTL == 0 {
		opts.TTL = 30 * 24 * time.Hour
	}
	if opts.NewThreadID == nil {
		opts.NewThreadID = defaultThreadID
	}
	return opts
}

func headerValue(headers map[string]string, key string) string {
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
