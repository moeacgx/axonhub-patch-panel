package proxy

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"axonhub-patch-panel/internal/normalize"
	"axonhub-patch-panel/internal/thread"
)

type Options struct {
	UpstreamURL          string
	Resolver             *thread.Resolver
	NewTraceID           func() string
	RespectExistingTrace bool
}

type Proxy struct {
	upstream             *url.URL
	client               *http.Client
	resolver             *thread.Resolver
	newTraceID           func() string
	respectExistingTrace bool
}

func New(opts Options) (*Proxy, error) {
	upstream, err := url.Parse(opts.UpstreamURL)
	if err != nil {
		return nil, err
	}
	if opts.Resolver == nil {
		return nil, fmt.Errorf("resolver is required")
	}
	if opts.NewTraceID == nil {
		opts.NewTraceID = defaultTraceID
	}
	return &Proxy{
		upstream:             upstream,
		client:               http.DefaultClient,
		resolver:             opts.Resolver,
		newTraceID:           opts.NewTraceID,
		respectExistingTrace: opts.RespectExistingTrace,
	}, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	doc, _ := normalize.Canonicalize(body, r.URL.Path)
	headerMap := mapHeaders(r.Header)
	result, err := p.resolver.Resolve(r.Context(), doc, headerMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, p.targetURL(r), bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.Header.Set("AH-Thread-Id", result.ThreadID)
	if !p.respectExistingTrace || upstreamReq.Header.Get("AH-Trace-Id") == "" {
		upstreamReq.Header.Set("AH-Trace-Id", p.newTraceID())
	}
	upstreamReq.Host = p.upstream.Host

	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if isStreamingResponse(resp.Header, body) {
		p.streamResponse(w, r, resp, doc, result.ThreadID)
		return
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if stateHash, responseID, err := normalize.StateAfterResponse(doc, responseBody); err == nil {
		_ = p.resolver.RememberState(r.Context(), stateHash, responseID, result.ThreadID)
	}

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func (p *Proxy) streamResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, doc normalize.Document, threadID string) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	var captured bytes.Buffer
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			_, _ = captured.Write(chunk)
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if stateHash, responseID, err := normalize.StateAfterStream(doc, captured.Bytes()); err == nil {
					_ = p.resolver.RememberState(r.Context(), stateHash, responseID, threadID)
				}
			}
			return
		}
	}
}

func (p *Proxy) targetURL(r *http.Request) string {
	target := *p.upstream
	target.Path = singleJoiningSlash(p.upstream.Path, r.URL.Path)
	target.RawQuery = r.URL.RawQuery
	return target.String()
}

func mapHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isStreamingResponse(headers http.Header, requestBody []byte) bool {
	contentType := strings.ToLower(headers.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return true
	}
	return requestHasStreamFlag(requestBody)
}

func requestHasStreamFlag(body []byte) bool {
	return bytes.Contains(bytes.ToLower(body), []byte(`"stream":true`)) ||
		bytes.Contains(bytes.ToLower(body), []byte(`"stream": true`))
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

func defaultTraceID() string {
	return "at-" + randomUUID()
}

func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", b[:])
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
