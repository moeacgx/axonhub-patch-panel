package thread

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type RedisStore struct {
	conn redisConn
}

type RedisOptions struct {
	Addr     string
	Password string
	DB       int
	Timeout  time.Duration
}

func NewRedisStore(ctx context.Context, opts RedisOptions) (*RedisStore, error) {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:6379"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 3 * time.Second
	}
	conn := &tcpRedisConn{addr: opts.Addr, timeout: opts.Timeout}
	if opts.Password != "" {
		if _, err := conn.Do(ctx, "AUTH", opts.Password); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	if opts.DB > 0 {
		if _, err := conn.Do(ctx, "SELECT", strconv.Itoa(opts.DB)); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return &RedisStore{conn: conn}, nil
}

func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	reply, err := s.conn.Do(ctx, "GET", key)
	if err != nil {
		return "", err
	}
	if reply.nil {
		return "", ErrNotFound
	}
	return reply.value, nil
}

func (s *RedisStore) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	args := []string{"SET", key, value, "NX"}
	if ttl > 0 {
		args = append(args, "EX", strconv.FormatInt(int64(ttl.Seconds()), 10))
	}
	reply, err := s.conn.Do(ctx, args...)
	if err != nil {
		return false, err
	}
	if reply.nil {
		return false, nil
	}
	return strings.EqualFold(reply.value, "OK"), nil
}

func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	args := []string{"SET", key, value}
	if ttl > 0 {
		args = append(args, "EX", strconv.FormatInt(int64(ttl.Seconds()), 10))
	}
	_, err := s.conn.Do(ctx, args...)
	return err
}

func (s *RedisStore) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

type redisConn interface {
	Do(ctx context.Context, args ...string) (redisReply, error)
	Close() error
}

type tcpRedisConn struct {
	addr    string
	timeout time.Duration
}

func (c *tcpRedisConn) Do(ctx context.Context, args ...string) (redisReply, error) {
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return redisReply{}, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(c.timeout))
	}

	if _, err := conn.Write(encodeRESP(args)); err != nil {
		return redisReply{}, err
	}
	return parseRedisReply(bufio.NewReader(conn))
}

func (c *tcpRedisConn) Close() error { return nil }

type redisReply struct {
	value string
	nil   bool
}

func encodeRESP(args []string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return []byte(b.String())
}

func parseRedisReply(r *bufio.Reader) (redisReply, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return redisReply{}, err
	}
	switch prefix {
	case '+':
		line, err := readLine(r)
		return redisReply{value: line}, err
	case '$':
		line, err := readLine(r)
		if err != nil {
			return redisReply{}, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return redisReply{}, err
		}
		if n < 0 {
			return redisReply{nil: true}, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return redisReply{}, err
		}
		return redisReply{value: string(buf[:n])}, nil
	case ':':
		line, err := readLine(r)
		return redisReply{value: line}, err
	case '-':
		line, _ := readLine(r)
		return redisReply{}, errors.New(line)
	default:
		return redisReply{}, fmt.Errorf("unsupported redis reply prefix %q", prefix)
	}
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func stringsReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}
