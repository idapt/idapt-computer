package commands

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	connectTimeout    = 10 * time.Second
	heartbeatTimeout  = 45 * time.Second
	minBackoff        = 1 * time.Second
	maxBackoff        = 60 * time.Second
	revokeStrikesMax  = 3
	lastEventIDFile   = "/var/lib/idapt/last-event-id"
)

type ClientOpts struct {
	AppURL       string
	MachineID    string
	MachineToken string
	Executor     *Executor
	OnRevoked    func() // called after 3 consecutive 401 responses
}

type Client struct {
	opts          ClientOpts
	http          *http.Client
	revokeStrikes int
	stateMu       sync.RWMutex
	connected     bool
	lastError     string
}

func NewClient(opts ClientOpts) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = connectTimeout
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext

	return &Client{
		opts: opts,
		http: &http.Client{
			Transport: transport,
			Timeout:   0,
		},
	}
}

func (c *Client) Connected() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.connected
}

func (c *Client) LastError() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.lastError
}

func (c *Client) setConnectionState(connected bool, err error) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.connected = connected
	if err != nil {
		c.lastError = err.Error()
	} else if connected {
		c.lastError = ""
	}
}

func (c *Client) Run(ctx context.Context) {
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.subscribe(ctx)
		if err == nil {
			backoff = minBackoff
			continue
		}
		if errors.Is(err, errUnauthorized) {
			c.revokeStrikes++
			log.Printf("commands: 401 strike %d/%d (will keep retrying — only heartbeat path triggers revoke)", c.revokeStrikes, revokeStrikesMax)
		} else {
			c.revokeStrikes = 0
		}
		log.Printf("commands: subscribe error: %v (backoff %s)", err, backoff)
		jitter := time.Duration(float64(backoff) * (0.75 + rand.Float64()*0.5))
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

var errUnauthorized = errors.New("unauthorized")

func (c *Client) subscribe(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/machines/%s/stream/commands", c.opts.AppURL, c.opts.MachineID)
	path := fmt.Sprintf("/api/machines/%s/stream/commands", c.opts.MachineID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.setConnectionState(false, err)
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	keyBytes, err := hex.DecodeString(c.opts.MachineToken)
	if err != nil {
		keyBytes = []byte(c.opts.MachineToken)
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte("GET:" + path + ":" + timestamp))
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Machine-Signature", sig)
	req.Header.Set("X-Machine-Timestamp", timestamp)

	if eid := readLastEventID(); eid != "" {
		req.Header.Set("Last-Event-ID", eid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.setConnectionState(false, err)
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.setConnectionState(false, errUnauthorized)
		return errUnauthorized
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		err := fmt.Errorf("subscribe returned %d", resp.StatusCode)
		c.setConnectionState(false, err)
		return err
	}
	defer resp.Body.Close()

	c.setConnectionState(true, nil)
	if err := c.readEvents(ctx, resp.Body); err != nil {
		c.setConnectionState(false, err)
		return err
	}
	return nil
}

func (c *Client) readEvents(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var data, lastID strings.Builder
	deadline := time.Now().Add(heartbeatTimeout)
	for {
		if time.Now().After(deadline) {
			return errors.New("heartbeat watchdog: no activity")
		}
		ok := scanner.Scan()
		if !ok {
			if err := scanner.Err(); err != nil {
				return err
			}
			return errors.New("eof")
		}
		line := scanner.Text()
		deadline = time.Now().Add(heartbeatTimeout)

		if line == "" {
			if data.Len() > 0 && lastID.Len() > 0 {
				c.dispatch(ctx, lastID.String(), data.String())
				saveLastEventID(lastID.String())
			}
			data.Reset()
			lastID.Reset()
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			lastID.WriteString(strings.TrimPrefix(line, "id: "))
		} else if strings.HasPrefix(line, "data: ") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line, "data: "))
		} else if strings.HasPrefix(line, "event: ") {
		}
	}
}

func (c *Client) dispatch(ctx context.Context, lastID, raw string) {
	var env Envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		log.Printf("commands: bad envelope JSON: %v", err)
		return
	}
	_ = c.opts.Executor.Submit(ctx, &env)
}

func readLastEventID() string {
	data, err := os.ReadFile(lastEventIDFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveLastEventID(id string) {
	if id == "" {
		return
	}
	tmp := lastEventIDFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(id), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, lastEventIDFile)
}
