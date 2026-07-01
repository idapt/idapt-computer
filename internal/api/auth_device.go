package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/idapt/idapt-computer/internal/httpclient"
)

const (
	deviceCodePath  = "/api/auth/device/code"
	deviceTokenPath = "/api/auth/device/token"
	DeviceClientID  = "idapt-computer"
	deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

var (
	ErrDeviceCodeExpired = errors.New("the login code expired before it was approved")
	ErrDeviceAccessDenied = errors.New("the login request was denied")
)

type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

func LoginDevice(ctx context.Context, baseURL, cliVersion string, out io.Writer) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	client := httpclient.New(cliVersion, 30*time.Second)

	code, err := requestDeviceCode(ctx, client, base)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out, "")
	if code.VerificationURIComplete != "" {
		fmt.Fprintf(out, "To finish signing in, open this URL in your browser:\n\n    %s\n\n", code.VerificationURIComplete)
		fmt.Fprintf(out, "(it confirms code %s)\n", code.UserCode)
	} else {
		fmt.Fprintf(out, "To finish signing in, open:\n\n    %s\n\n", code.VerificationURI)
		fmt.Fprintf(out, "and enter the code:  %s\n", code.UserCode)
	}
	fmt.Fprintln(out, "\nWaiting for approval… (Ctrl-C to cancel)")

	return pollDeviceToken(ctx, client, base, code)
}

func requestDeviceCode(ctx context.Context, client *http.Client, base string) (*deviceCodeResponse, error) {
	body, _ := json.Marshal(map[string]string{"client_id": DeviceClientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+deviceCodePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	setTrustedOrigin(req, base)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting a login code failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("could not start device login (%d): %s", resp.StatusCode, serverMessage(resp))
	}
	var code deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&code); err != nil {
		return nil, fmt.Errorf("parsing the login-code response: %w", err)
	}
	if code.DeviceCode == "" || code.UserCode == "" {
		return nil, errors.New("the server returned an incomplete login-code response")
	}
	return &code, nil
}

func pollDeviceToken(ctx context.Context, client *http.Client, base string, code *deviceCodeResponse) (string, error) {
	interval := code.Interval
	if interval < 1 {
		interval = 5
	}
	deadline := time.Now().Add(15 * time.Minute)
	if code.ExpiresIn > 0 {
		deadline = time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)
	}

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
		if time.Now().After(deadline) {
			return "", ErrDeviceCodeExpired
		}

		token, slowDown, err := pollOnce(ctx, client, base, code.DeviceCode)
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
		if slowDown {
			interval += 5
		}
	}
}

func pollOnce(ctx context.Context, client *http.Client, base, deviceCode string) (token string, slowDown bool, err error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":  deviceGrantType,
		"device_code": deviceCode,
		"client_id":   DeviceClientID,
	})
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, base+deviceTokenPath, bytes.NewReader(body))
	if reqErr != nil {
		return "", false, reqErr
	}
	req.Header.Set("Content-Type", "application/json")
	setTrustedOrigin(req, base)

	resp, doErr := client.Do(req)
	if doErr != nil {
		return "", false, fmt.Errorf("polling for approval failed: %w", doErr)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var parsed deviceTokenResponse
	_ = json.Unmarshal(data, &parsed)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 && parsed.AccessToken != "" {
		return parsed.AccessToken, false, nil
	}
	switch parsed.Error {
	case "authorization_pending":
		return "", false, nil
	case "slow_down":
		return "", true, nil
	case "expired_token":
		return "", false, ErrDeviceCodeExpired
	case "access_denied":
		return "", false, ErrDeviceAccessDenied
	default:
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return "", false, fmt.Errorf("device login failed (%d): %s", resp.StatusCode, msg)
	}
}

func serverMessage(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if len(data) == 0 {
		return http.StatusText(resp.StatusCode)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &env); err == nil {
		if env.Error.Message != "" {
			return env.Error.Message
		}
		if env.Message != "" {
			return env.Message
		}
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return http.StatusText(resp.StatusCode)
	}
	return s
}
