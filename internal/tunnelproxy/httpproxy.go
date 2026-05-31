package tunnelproxy

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/hashicorp/yamux"
	"github.com/idapt/idapt-cli/internal/tunnel"
)

func (s *Server) handlePublic(w http.ResponseWriter, r *http.Request) {
	host := requestHost(r)

	entry, err := s.registry.Lookup(r.Context(), host)
	if err == ErrNoTunnel {
		http.Error(w, "no tunnel is registered for this hostname", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("tunnelproxy: registry lookup %q: %v", host, err)
		http.Error(w, "tunnel registry unavailable", http.StatusBadGateway)
		return
	}

	session, ok := s.hub.Get(entry.ComputerID)
	if !ok {
		http.Error(w, "the computer for this tunnel is offline", http.StatusBadGateway)
		return
	}

	if _, ok := s.authenticateVisitor(w, r, host, entry.AuthMode); !ok {
		return // 302 bounce or 401 already written
	}

	s.forward(w, r, session, entry.LocalPort, host)
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, session *yamux.Session, localPort int, host string) {
	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			st, err := tunnel.OpenStream(session, tunnel.StreamHeader{
				Kind:       tunnel.StreamKindHTTP,
				Port:       localPort,
				RemoteAddr: r.RemoteAddr,
			})
			if err != nil {
				return nil, err
			}
			return st, nil
		},
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: host})
			pr.Out.Host = host
		},
		Transport:     transport,
		FlushInterval: -1, // stream responses immediately (SSE, chunked)
		ErrorHandler: func(ew http.ResponseWriter, _ *http.Request, err error) {
			log.Printf("tunnelproxy: forward %q -> :%d: %v", host, localPort, err)
			http.Error(ew, "the tunneled service did not respond", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, r)
}
