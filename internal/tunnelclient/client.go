package tunnelclient

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-computer/internal/tunnel"
)

const (
	connectTimeout   = 15 * time.Second
	localDialTimeout = 10 * time.Second
	minBackoff       = 1 * time.Second
	maxBackoff       = 60 * time.Second
	sshLocalPort = 22
)

type Client struct {
	proxyURL string
	tokens   *Syncer
	ports    *ConfigManager
	http     *http.Client

	mu        sync.RWMutex
	connected bool
	lastError string
}

func NewClient(proxyURL string, tokens *Syncer, ports *ConfigManager) *Client {
	return &Client{
		proxyURL: strings.TrimRight(proxyURL, "/"),
		tokens:   tokens,
		ports:    ports,
		http:     &http.Client{Timeout: connectTimeout},
	}
}

func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) LastError() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

func (c *Client) setState(connected bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		err := c.connectAndServe(ctx)
		c.setState(false, err)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("tunnel: disconnected: %v (retry in ~%s)", err, backoff)
		}
		jitter := time.Duration(float64(backoff) * (0.75 + rand.Float64()*0.5))
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}
		if err == nil {
			backoff = minBackoff
		} else if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context) error {
	tokenCtx, cancelToken := context.WithTimeout(ctx, connectTimeout)
	token, err := c.tokens.MintDaemonToken(tokenCtx)
	cancelToken()
	if err != nil {
		return fmt.Errorf("mint daemon token: %w", err)
	}

	wsURL := c.proxyURL + "/__tunnel/connect"
	dialCtx, cancelDial := context.WithTimeout(ctx, connectTimeout)
	wsConn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient: c.http,
		HTTPHeader: http.Header{"Authorization": {"Bearer " + token}},
	})
	cancelDial()
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}

	session, err := tunnel.ClientSession(tunnel.WebSocketNetConn(ctx, wsConn))
	if err != nil {
		_ = wsConn.Close(websocket.StatusInternalError, "yamux setup failed")
		return fmt.Errorf("tunnel session: %w", err)
	}
	defer func() { _ = session.Close() }()

	c.setState(true, nil)
	log.Printf("tunnel: connected to %s", wsURL)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-stop:
		}
	}()

	for {
		stream, header, err := tunnel.AcceptStream(session)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		go c.handleStream(stream, header)
	}
}

func (c *Client) handleStream(stream *tunnel.Stream, header tunnel.StreamHeader) {
	defer func() { _ = stream.Close() }()

	targetPort := header.Port
	if header.Kind == tunnel.StreamKindSSH {
		targetPort = sshLocalPort
	} else if !c.ports.IsExposed(header.Port) {
		_ = stream.Reject(fmt.Sprintf("port %d is not exposed", header.Port))
		return
	}
	local, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", targetPort), localDialTimeout)
	if err != nil {
		_ = stream.Reject(fmt.Sprintf("dial 127.0.0.1:%d: %v", targetPort, err))
		return
	}
	defer func() { _ = local.Close() }()
	if err := stream.Confirm(); err != nil {
		return
	}
	pipe(stream, local)
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
}
