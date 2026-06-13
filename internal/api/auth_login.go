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

	"github.com/idapt/idapt-cli/internal/httpclient"
)

var (
	ErrLoginInvalidCredentials = errors.New("invalid email or password")
	ErrLoginNoSession = errors.New("sign-in succeeded but no session cookie was returned")
	ErrAPIKeyPaidPlanRequired = errors.New("creating a CLI API key requires a paid plan")
)

const signInPath = "/api/auth/sign-in/email"

const apiKeysPath = "/api/v1/api-keys"

func LoginEmailPassword(ctx context.Context, baseURL, cliVersion, email, password, keyName string) (string, error) {
	base := strings.TrimRight(baseURL, "/")

	client := httpclient.New(cliVersion, 30*time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	cookieHeader, err := signIn(ctx, client, base, email, password)
	if err != nil {
		return "", err
	}
	return createUserAPIKey(ctx, client, base, cookieHeader, keyName)
}

func signIn(ctx context.Context, client *http.Client, base, email, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+signInPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sign-in request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w: %s", ErrLoginInvalidCredentials, serverMessage(resp))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sign-in failed (%d): %s", resp.StatusCode, serverMessage(resp))
	}

	cookieHeader := cookieHeaderFromResponse(resp)
	if cookieHeader == "" {
		return "", ErrLoginNoSession
	}
	return cookieHeader, nil
}

func createUserAPIKey(ctx context.Context, client *http.Client, base, cookieHeader, keyName string) (string, error) {
	body, _ := json.Marshal(map[string]any{"name": keyName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+apiKeysPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("Origin", base)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api-key request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusPaymentRequired {
		msg := serverMessage(resp)
		if isPaidPlanMessage(msg) {
			return "", ErrAPIKeyPaidPlanRequired
		}
		return "", fmt.Errorf("could not create an API key (%d): %s", resp.StatusCode, msg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("could not create an API key (%d): %s", resp.StatusCode, serverMessage(resp))
	}

	var parsed struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("parsing api-key response: %w", err)
	}
	if parsed.Data.Key == "" {
		return "", errors.New("api-key response contained no key")
	}
	return parsed.Data.Key, nil
}

func cookieHeaderFromResponse(resp *http.Response) string {
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(cookies))
	for _, ck := range cookies {
		if ck.Value == "" {
			continue
		}
		pairs = append(pairs, ck.Name+"="+ck.Value)
	}
	return strings.Join(pairs, "; ")
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

func isPaidPlanMessage(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "paid") || strings.Contains(m, "plan") || strings.Contains(m, "upgrade")
}
