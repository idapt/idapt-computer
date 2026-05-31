package tunnelproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-cli/internal/tunnel"
)

func (s *Server) handleSSHConnect(w http.ResponseWriter, r *http.Request) {
	computerID, err := s.authenticateSSH(r)
	if err != nil {
		log.Printf("tunnelproxy: ssh auth rejected: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	session, ok := s.hub.Get(computerID)
	if !ok {
		http.Error(w, "the computer is offline", http.StatusBadGateway)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("tunnelproxy: ssh websocket accept (computer %s): %v", computerID, err)
		return
	}
	clientConn := tunnel.WebSocketNetConn(context.Background(), c)

	stream, err := tunnel.OpenStream(session, tunnel.StreamHeader{
		Kind:       tunnel.StreamKindSSH,
		Port:       22,
		RemoteAddr: r.RemoteAddr,
	})
	if err != nil {
		log.Printf("tunnelproxy: ssh stream open (computer %s): %v", computerID, err)
		_ = c.Close(websocket.StatusInternalError, "ssh stream open failed")
		return
	}
	log.Printf("tunnelproxy: ssh session opened computer=%s", computerID)

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(stream, clientConn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(clientConn, stream); done <- struct{}{} }()
	<-done
	_ = stream.Close()
	_ = c.Close(websocket.StatusNormalClosure, "ssh session closed")
	log.Printf("tunnelproxy: ssh session closed computer=%s", computerID)
}

func (s *Server) authenticateSSH(r *http.Request) (string, error) {
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return "", errors.New("missing bearer token")
	}
	claims, err := s.jwt.verify(strings.TrimPrefix(authz, "Bearer "))
	if err != nil {
		return "", err
	}
	if claims.Aud != audSSH {
		return "", fmt.Errorf("token audience %q is not %q", claims.Aud, audSSH)
	}
	if claims.Computer == "" {
		return "", errors.New("token has no computer claim")
	}
	return claims.Computer, nil
}
