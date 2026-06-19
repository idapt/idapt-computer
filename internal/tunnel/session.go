package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
)

const streamHandshakeTimeout = 10 * time.Second

type Stream struct {
	net.Conn
	br io.Reader
}

func (s *Stream) Read(p []byte) (int, error) { return s.br.Read(p) }

func WebSocketNetConn(ctx context.Context, c *websocket.Conn) net.Conn {
	c.SetReadLimit(-1)
	return websocket.NetConn(ctx, c, websocket.MessageBinary)
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 15 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}

func ClientSession(conn net.Conn) (*yamux.Session, error) {
	return yamux.Client(conn, yamuxConfig())
}

func ServerSession(conn net.Conn) (*yamux.Session, error) {
	return yamux.Server(conn, yamuxConfig())
}

func OpenStream(session *yamux.Session, h StreamHeader) (*Stream, error) {
	raw, err := session.Open()
	if err != nil {
		return nil, err
	}
	_ = raw.SetDeadline(time.Now().Add(streamHandshakeTimeout))
	if err := WriteHeader(raw, h); err != nil {
		_ = raw.Close()
		return nil, err
	}
	br := NewReader(raw)
	reply, err := ReadReply(br)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	if !reply.OK {
		_ = raw.Close()
		return nil, fmt.Errorf("tunnel: daemon refused stream: %s", reply.Error)
	}
	_ = raw.SetDeadline(time.Time{})
	return &Stream{Conn: raw, br: br}, nil
}

func AcceptStream(session *yamux.Session) (*Stream, StreamHeader, error) {
	raw, err := session.Accept()
	if err != nil {
		return nil, StreamHeader{}, err
	}
	_ = raw.SetDeadline(time.Now().Add(streamHandshakeTimeout))
	br := NewReader(raw)
	h, err := ReadHeader(br)
	if err != nil {
		_ = raw.Close()
		return nil, StreamHeader{}, err
	}
	_ = raw.SetDeadline(time.Time{})
	return &Stream{Conn: raw, br: br}, h, nil
}

func (s *Stream) Confirm() error {
	return WriteReply(s.Conn, StreamReply{OK: true})
}

func (s *Stream) Reject(reason string) error {
	_ = WriteReply(s.Conn, StreamReply{OK: false, Error: reason})
	return s.Conn.Close()
}
