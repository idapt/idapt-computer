package httpclient

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var APIVersion = "2026-03-28"

type versionTransport struct {
	base       http.RoundTripper
	userAgent  string
	apiVersion string
}

func (t *versionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", t.userAgent)
	if r.Header.Get("X-Idapt-Version") == "" {
		r.Header.Set("X-Idapt-Version", t.apiVersion)
	}
	return t.base.RoundTrip(r)
}

func SecureTransport() *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if base.TLSClientConfig == nil {
		base.TLSClientConfig = &tls.Config{}
	}
	base.TLSClientConfig.MinVersion = tls.VersionTLS12
	return base
}

func New(cliVersion string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &versionTransport{
			base:       SecureTransport(),
			userAgent:  "idapt-computer/" + cliVersion,
			apiVersion: APIVersion,
		},
	}
}

func RequireSecureURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	switch u.Scheme {
	case "https", "wss":
		return nil
	case "http", "ws":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		if os.Getenv("IDAPT_ALLOW_INSECURE_APP_URL") == "1" {
			return nil
		}
		return fmt.Errorf("refusing insecure %q URL for non-loopback host %q (set IDAPT_ALLOW_INSECURE_APP_URL=1 to allow cleartext for self-hosting)", u.Scheme, u.Hostname())
	default:
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
