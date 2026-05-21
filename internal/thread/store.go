package thread

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("thread mapping not found")

type Store interface {
	Get(ctx context.Context, key string) (string, error)
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

type memoryEntry struct {
	value     string
	expiresAt time.Time
}

type MemoryStore struct {
	mu   sync.Mutex
	data map[string]memoryEntry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]memoryEntry{}}
}

func (s *MemoryStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(s.data, key)
		return "", ErrNotFound
	}
	return entry.value, nil
}

func (s *MemoryStore) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.data[key]; ok {
		if entry.expiresAt.IsZero() || time.Now().Before(entry.expiresAt) {
			return false, nil
		}
	}
	s.data[key] = memoryEntry{value: value, expiresAt: expiresAt(ttl)}
	return true, nil
}

func (s *MemoryStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = memoryEntry{value: value, expiresAt: expiresAt(ttl)}
	return nil
}

func expiresAt(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}
