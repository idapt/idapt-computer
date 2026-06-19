package tunnelproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/idapt/idapt-computer/internal/tunnel"
)

type Server struct {
	cfg      *Config
	registry *Registry
	hub      *Hub
	jwt      *jwtVerifier
	http     *http.Server
}

func NewServer(cfg *Config) (*Server, error) {
	registry, err := NewRegistry(cfg.RedisURL)
	if err != nil {
		return nil, err
	}
	verifier, err := newJWTVerifier(cfg.JWTPublicKeyPEM)
	if err != nil {
		_ = registry.Close()
		return nil, err
	}
	s := &Server{cfg: cfg, registry: registry, hub: NewHub(), jwt: verifier}
	s.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           http.HandlerFunc(s.route),
		ReadHeaderTimeout: 15 * time.Second,
	}
	return s, nil
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__healthz":
		s.handleHealthz(w, r)
	case "/__readyz":
		s.handleReadyz(w, r)
	case "/__tunnel/connect":
		s.handleDaemonConnect(w, r)
	case "/__tunnel/ssh":
		s.handleSSHConnect(w, r)
	case "/__idapt/init":
		s.handleInit(w, r)
	default:
		s.handlePublic(w, r)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.registry.Ping(ctx); err != nil {
		http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ready")
}

func (s *Server) handleDaemonConnect(w http.ResponseWriter, r *http.Request) {
	computerID, err := s.authenticateDaemon(r)
	if err != nil {
		log.Printf("tunnelproxy: daemon auth rejected: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("tunnelproxy: websocket accept (computer %s): %v", computerID, err)
		return
	}
	session, err := tunnel.ServerSession(tunnel.WebSocketNetConn(context.Background(), c))
	if err != nil {
		log.Printf("tunnelproxy: yamux setup (computer %s): %v", computerID, err)
		_ = c.Close(websocket.StatusInternalError, "session setup failed")
		return
	}
	release := s.hub.Register(computerID, session)
	log.Printf("tunnelproxy: daemon connected computer=%s total=%d", computerID, s.hub.Count())

	<-session.CloseChan()
	release()
	_ = session.Close()
	_ = c.Close(websocket.StatusNormalClosure, "session closed")
	log.Printf("tunnelproxy: daemon disconnected computer=%s total=%d", computerID, s.hub.Count())
}

func (s *Server) authenticateDaemon(r *http.Request) (string, error) {
	if s.cfg.DevAuthBypass {
		if mid := r.Header.Get("X-Idapt-Computer-Id"); mid != "" {
			return mid, nil
		}
	}
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		return "", errors.New("missing bearer token")
	}
	claims, err := s.jwt.verify(strings.TrimPrefix(authz, "Bearer "))
	if err != nil {
		return "", err
	}
	if claims.Aud != audDaemon {
		return "", fmt.Errorf("token audience %q is not %q", claims.Aud, audDaemon)
	}
	return claims.Sub, nil
}

func (s *Server) Run() error {
	log.Printf("tunnelproxy: listening on %s (base domain %s, app %s)",
		s.cfg.ListenAddr, s.cfg.BaseDomain, s.cfg.AppURL)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	_ = s.registry.Close()
	return err
}

func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}
