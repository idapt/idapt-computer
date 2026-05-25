package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSEEvent struct {
	Event string
	Data  string
	ID    string
}

var (
	ErrHeartbeatTimeout = errors.New("sse: heartbeat timeout")
	ErrStreamClosed = errors.New("sse: stream closed")
)

type SSEOption func(*sseOptions)

type sseOptions struct {
	lastEventID string
	heartbeat   time.Duration
}

func WithResume(lastEventID string) SSEOption {
	return func(o *sseOptions) { o.lastEventID = lastEventID }
}

func WithHeartbeat(interval time.Duration) SSEOption {
	return func(o *sseOptions) { o.heartbeat = interval }
}

type SSEReader struct {
	scanner     *bufio.Scanner
	body        io.ReadCloser
	heartbeat   time.Duration
	mu          sync.Mutex
	lastEventID string
	err         error
	closed      bool
}

func (c *Client) StreamSSE(ctx context.Context, method, path string, reqBody interface{}, opts ...SSEOption) (*SSEReader, error) {
	o := &sseOptions{}
	for _, opt := range opts {
		opt(o)
	}

	cleanPath, query := splitPathQuery(path)

	var body io.Reader
	reqOpts := []RequestOption{
		WithHeader("Accept", "text/event-stream"),
	}
	if o.lastEventID != "" {
		reqOpts = append(reqOpts, WithHeader("Last-Event-ID", o.lastEventID))
	}
	if query != "" {
		reqOpts = append(reqOpts, withRawQuery(query))
	}

	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
		reqOpts = append(reqOpts, WithHeader("Content-Type", "application/json"))
	}

	resp, err := c.Do(ctx, method, cleanPath, body, reqOpts...)
	if err != nil {
		return nil, err
	}

	r := newSSEReader(resp.Body)
	r.heartbeat = o.heartbeat
	r.lastEventID = o.lastEventID
	return r, nil
}

func splitPathQuery(p string) (path, query string) {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return p, ""
}

func withRawQuery(raw string) RequestOption {
	return func(req *http.Request) {
		if req.URL.RawQuery == "" {
			req.URL.RawQuery = raw
			return
		}
		req.URL.RawQuery = req.URL.RawQuery + "&" + raw
	}
}

func (c *Client) StreamSSEGet(ctx context.Context, path string, opts ...interface{}) (*SSEReader, error) {
	reqOpts := []RequestOption{WithHeader("Accept", "text/event-stream")}
	o := &sseOptions{}
	for _, raw := range opts {
		switch v := raw.(type) {
		case RequestOption:
			reqOpts = append(reqOpts, v)
		case SSEOption:
			v(o)
		default:
			return nil, fmt.Errorf("StreamSSEGet: unsupported option %T", raw)
		}
	}
	if o.lastEventID != "" {
		reqOpts = append(reqOpts, WithHeader("Last-Event-ID", o.lastEventID))
	}

	cleanPath, query := splitPathQuery(path)
	if query != "" {
		reqOpts = append(reqOpts, withRawQuery(query))
	}
	resp, err := c.Do(ctx, "GET", cleanPath, nil, reqOpts...)
	if err != nil {
		return nil, err
	}
	r := newSSEReader(resp.Body)
	r.heartbeat = o.heartbeat
	r.lastEventID = o.lastEventID
	return r, nil
}

func newSSEReader(body io.ReadCloser) *SSEReader {
	return &SSEReader{
		scanner: bufio.NewScanner(body),
		body:    body,
	}
}

func NewSSEReaderFromReader(r io.ReadCloser) *SSEReader {
	return newSSEReader(r)
}

func (r *SSEReader) LastEventID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastEventID
}

func (r *SSEReader) Next() (*SSEEvent, error) {
	r.mu.Lock()
	if r.err != nil {
		err := r.err
		r.mu.Unlock()
		return nil, err
	}
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, ErrStreamClosed
	}

	if r.heartbeat <= 0 {
		return r.readOne()
	}
	type result struct {
		ev  *SSEEvent
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ev, err := r.readOne()
		ch <- result{ev, err}
	}()
	timer := time.NewTimer(r.heartbeat)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res.ev, res.err
	case <-timer.C:
		_ = r.Close()
		<-ch
		r.mu.Lock()
		r.err = ErrHeartbeatTimeout
		r.mu.Unlock()
		return nil, ErrHeartbeatTimeout
	}
}

func (r *SSEReader) readOne() (*SSEEvent, error) {
	event := &SSEEvent{}
	hasData := false

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if line == "" {
			if hasData || event.Event != "" || event.ID != "" {
				if event.ID != "" {
					r.mu.Lock()
					r.lastEventID = event.ID
					r.mu.Unlock()
				}
				return event, nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "event":
			event.Event = value
		case "data":
			if hasData {
				event.Data += "\n" + value
			} else {
				event.Data = value
				hasData = true
			}
		case "id":
			event.ID = value
		}
	}

	if err := r.scanner.Err(); err != nil {
		r.mu.Lock()
		r.err = err
		r.mu.Unlock()
		return nil, err
	}

	if hasData || event.Event != "" || event.ID != "" {
		if event.ID != "" {
			r.mu.Lock()
			r.lastEventID = event.ID
			r.mu.Unlock()
		}
		r.mu.Lock()
		r.err = io.EOF
		r.mu.Unlock()
		return event, nil
	}

	r.mu.Lock()
	r.err = io.EOF
	r.mu.Unlock()
	return nil, io.EOF
}

func (r *SSEReader) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	return r.body.Close()
}
