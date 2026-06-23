package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/httpclient"
)

const (
	oauthAuthorizePath = "/api/auth/oauth2/authorize"
	oauthTokenPath     = "/api/auth/oauth2/token"
	OAuthClientID = "idapt-computer"
	oauthScope = "openid profile email offline_access"
	oauthResource = "idapt-api"
	oauthLoginTimeout = 5 * time.Minute
)

var (
	ErrOAuthAccessDenied = errors.New("the login request was denied")
	ErrOAuthStateMismatch = errors.New("login state mismatch — please try again")
	ErrOAuthTimedOut = errors.New("timed out waiting for browser sign-in")
	ErrOAuthSessionExpired = errors.New("your session has expired — run `idapt-computer auth login` again")
)

type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type oauthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func LoginAuthCode(
	ctx context.Context,
	baseURL, cliVersion string,
	out io.Writer,
	openURL func(string) error,
) (*OAuthTokens, error) {
	base := strings.TrimRight(baseURL, "/")

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not start local sign-in listener: %w", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			writeCallbackPage(w, false)
			if e == "access_denied" {
				errCh <- ErrOAuthAccessDenied
			} else {
				errCh <- fmt.Errorf("sign-in failed: %s", e)
			}
			return
		}
		if q.Get("state") != state {
			writeCallbackPage(w, false)
			errCh <- ErrOAuthStateMismatch
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackPage(w, false)
			errCh <- errors.New("the sign-in response did not include an authorization code")
			return
		}
		writeCallbackPage(w, true)
		codeCh <- code
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authzURL := base + oauthAuthorizePath + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {OAuthClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {oauthScope},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	fmt.Fprintln(out, "")
	if openURL != nil && openURL(authzURL) == nil {
		fmt.Fprintf(out, "Opening your browser to sign in…\n\nIf it doesn't open, visit:\n\n    %s\n\n", authzURL)
	} else {
		fmt.Fprintf(out, "To finish signing in, open this URL in your browser:\n\n    %s\n\n", authzURL)
	}
	fmt.Fprintln(out, "Waiting for sign-in… (Ctrl-C to cancel)")

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(oauthLoginTimeout):
		return nil, ErrOAuthTimedOut
	}

	hc := httpclient.New(cliVersion, 30*time.Second)
	return exchangeAuthCode(ctx, hc, base, code, verifier, redirectURI)
}

func exchangeAuthCode(
	ctx context.Context,
	hc *http.Client,
	base, code, verifier, redirectURI string,
) (*OAuthTokens, error) {
	return postToken(ctx, hc, base, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {OAuthClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"resource":      {oauthResource},
	})
}

func RefreshOAuthToken(ctx context.Context, baseURL, cliVersion, refreshToken string) (*OAuthTokens, error) {
	base := strings.TrimRight(baseURL, "/")
	hc := httpclient.New(cliVersion, 30*time.Second)
	tok, err := postToken(ctx, hc, base, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {OAuthClientID},
		"refresh_token": {refreshToken},
		"resource":      {oauthResource},
	})
	if errors.Is(err, errInvalidGrant) {
		return nil, ErrOAuthSessionExpired
	}
	return tok, err
}

var errInvalidGrant = errors.New("invalid_grant")

func setTrustedOrigin(req *http.Request, base string) {
	if u, err := url.Parse(base); err == nil && u.Scheme != "" && u.Host != "" {
		req.Header.Set("Origin", u.Scheme+"://"+u.Host)
	}
}

func postToken(ctx context.Context, hc *http.Client, base string, form url.Values) (*OAuthTokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+oauthTokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	setTrustedOrigin(req, base)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting the token endpoint failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed oauthTokenResponse
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode != http.StatusOK || parsed.AccessToken == "" {
		if parsed.Error == "invalid_grant" {
			return nil, errInvalidGrant
		}
		msg := parsed.ErrorDescription
		if msg == "" {
			msg = parsed.Error
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, msg)
	}

	return &OAuthTokens{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresIn:    parsed.ExpiresIn,
	}, nil
}

func writeCallbackPage(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if ok {
		_, _ = io.WriteString(w, callbackHTMLSuccess)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, callbackHTMLError)
}

const callbackHTMLSuccess = `<!doctype html><html><head><meta charset="utf-8"><title>Signed in</title></head>` +
	`<body style="font-family:system-ui;text-align:center;padding-top:4rem">` +
	`<h1>You're signed in</h1><p>Return to your terminal — you can close this tab.</p></body></html>`

const callbackHTMLError = `<!doctype html><html><head><meta charset="utf-8"><title>Sign-in failed</title></head>` +
	`<body style="font-family:system-ui;text-align:center;padding-top:4rem">` +
	`<h1>Sign-in failed</h1><p>Return to your terminal and run <code>idapt-computer auth login</code> again.</p></body></html>`
