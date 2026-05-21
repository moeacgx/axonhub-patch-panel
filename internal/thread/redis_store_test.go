package thread

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisStoreParsesSetNXOK(t *testing.T) {
	store := &RedisStore{conn: &scriptedConn{
		replies: []string{"+OK\r\n"},
	}}

	ok, err := store.SetNX(context.Background(), "k", "v", time.Minute)
	if err != nil {
		t.Fatalf("SetNX returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected SetNX to return true")
	}
}

func TestRedisStoreMapsNilToNotFound(t *testing.T) {
	store := &RedisStore{conn: &scriptedConn{
		replies: []string{"$-1\r\n"},
	}}

	_, err := store.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type scriptedConn struct {
	replies []string
	writes  [][]byte
}

func (c *scriptedConn) Do(ctx context.Context, args ...string) (redisReply, error) {
	if len(c.replies) == 0 {
		return redisReply{}, errors.New("no scripted reply")
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return parseRedisReply(stringsReader(reply))
}

func (c *scriptedConn) Close() error { return nil }
