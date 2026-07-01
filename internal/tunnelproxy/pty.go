package tunnelproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-computer/internal/tunnel"
)

func (s *Server) handlePTYConnect(w http.ResponseWriter, r *http.Request) {
	claims, err := s.authenticatePTY(r)
	if err != nil {
		log.Printf("tunnelproxy: pty auth rejected: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	session, ok := s.hub.Get(claims.Computer)
	if !ok {
		http.Error(w, "the computer is offline", http.StatusBadGateway)
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = claims.Mode
	}
	cols := parseDim(r.URL.Query().Get("cols"))
	rows := parseDim(r.URL.Query().Get("rows"))

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("tunnelproxy: pty websocket accept (computer %s): %v", claims.Computer, err)
		return
	}
	clientConn := tunnel.WebSocketNetConn(context.Background(), c)

	stream, err := tunnel.OpenStream(session, tunnel.StreamHeader{
		Kind:       tunnel.StreamKindPTY,
		RunAs:      claims.RunAs,
		Mode:       mode,
		Cols:       cols,
		Rows:       rows,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		log.Printf("tunnelproxy: pty stream open (computer %s): %v", claims.Computer, err)
		_ = c.Close(websocket.StatusInternalError, "pty stream open failed")
		return
	}
	log.Printf("tunnelproxy: pty session opened computer=%s", claims.Computer)

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(stream, clientConn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(clientConn, stream); done <- struct{}{} }()
	<-done
	_ = stream.Close()
	_ = c.Close(websocket.StatusNormalClosure, "pty session closed")
	log.Printf("tunnelproxy: pty session closed computer=%s", claims.Computer)
}

func (s *Server) authenticatePTY(r *http.Request) (*tokenClaims, error) {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return nil, errors.New("missing bearer token")
	}
	claims, err := s.jwt.verify(strings.TrimPrefix(authz, "Bearer "))
	if err != nil {
		return nil, err
	}
	if claims.Aud != audPTY {
		return nil, fmt.Errorf("token audience %q is not %q", claims.Aud, audPTY)
	}
	if claims.Computer == "" {
		return nil, errors.New("token has no computer claim")
	}
	return claims, nil
}

func parseDim(s string) uint16 {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	if n > 65535 {
		n = 65535
	}
	return uint16(n)
}
