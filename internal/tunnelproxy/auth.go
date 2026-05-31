package tunnelproxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) visitorCookieName() string {
	if s.cfg.CookieSecure {
		return "__Secure-idapt_tunnel"
	}
	return "idapt_tunnel"
}

func (s *Server) authenticateVisitor(w http.ResponseWriter, r *http.Request, host, expectedAuthMode string) (actorID string, ok bool) {
	if claims := s.validVisitorToken(r, host, expectedAuthMode); claims != nil {
		return claims.Sub, true
	}
	if isAPIRequest(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="idapt-tunnel"`)
		http.Error(w, "tunnel authentication required", http.StatusUnauthorized)
		return "", false
	}
	returnTo := r.URL.RequestURI()
	authorizeURL := fmt.Sprintf("%s/api/tunnel/authorize?host=%s&return=%s",
		s.cfg.AppURL, url.QueryEscape(host), url.QueryEscape(returnTo))
	http.Redirect(w, r, authorizeURL, http.StatusFound)
	return "", false
}

func (s *Server) validVisitorToken(r *http.Request, host, expectedAuthMode string) *tokenClaims {
	token := ""
	if c, err := r.Cookie(s.visitorCookieName()); err == nil {
		token = c.Value
	}
	if token == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		}
	}
	if token == "" {
		return nil
	}
	claims, err := s.jwt.verify(token)
	if err != nil {
		return nil
	}
	if claims.Aud != audVisitor || !strings.EqualFold(claims.Host, host) {
		return nil
	}
	if claims.AuthMode != "" && expectedAuthMode != "" &&
		!strings.EqualFold(claims.AuthMode, expectedAuthMode) {
		return nil
	}
	return claims
}

func (s *Server) handleInit(w http.ResponseWriter, r *http.Request) {
	host := requestHost(r)
	token := r.URL.Query().Get("token")
	claims, err := s.jwt.verify(token)
	if err != nil || claims.Aud != audVisitor || !strings.EqualFold(claims.Host, host) {
		http.Error(w, "invalid or expired tunnel token", http.StatusBadRequest)
		return
	}
	returnTo := r.URL.Query().Get("return")
	if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = "/"
	}
	expires := time.Unix(claims.Exp, 0)
	http.SetCookie(w, &http.Cookie{
		Name:     s.visitorCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
	http.Redirect(w, r, returnTo, http.StatusFound)
}

func isAPIRequest(r *http.Request) bool {
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}
	switch r.Header.Get("Sec-Fetch-Mode") {
	case "navigate", "nested-navigate":
		return false
	case "cors", "no-cors", "same-origin", "websocket":
		return true
	}
	accept := r.Header.Get("Accept")
	return accept != "" && !strings.Contains(accept, "text/html")
}
