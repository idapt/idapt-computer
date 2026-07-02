package deviceflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type MintRequest struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CLIVersion    string `json:"cli_version"`
	DefaultUser   string `json:"default_user"`
	HostKind      string `json:"host_kind"`
	KernelVersion string `json:"kernel_version,omitempty"`
}

type MintResponse struct {
	Code       string    `json:"code"`
	CodeID     string    `json:"codeId"`
	ExpiresAt  time.Time `json:"expiresAt"`
	ConfirmURL string    `json:"confirmUrl"`
	PollURL    string    `json:"pollUrl"`
}

func (r *MintResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Code            string    `json:"code"`
		CodeID          string    `json:"code_id"`
		CodeIDCamel     string    `json:"codeId"`
		ExpiresAt       time.Time `json:"expires_at"`
		ExpiresAtCamel  time.Time `json:"expiresAt"`
		ConfirmURL      string    `json:"confirm_url"`
		ConfirmURLCamel string    `json:"confirmUrl"`
		PollURL         string    `json:"poll_url"`
		PollURLCamel    string    `json:"pollUrl"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Code = raw.Code
	r.CodeID = firstNonEmpty(raw.CodeID, raw.CodeIDCamel)
	r.ExpiresAt = raw.ExpiresAt
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = raw.ExpiresAtCamel
	}
	r.ConfirmURL = firstNonEmpty(raw.ConfirmURL, raw.ConfirmURLCamel)
	r.PollURL = firstNonEmpty(raw.PollURL, raw.PollURLCamel)
	return nil
}

type StatusResponse struct {
	Status         string `json:"status"`
	ComputerID     string `json:"computerId,omitempty"`
	ComputerToken  string `json:"computerToken,omitempty"`
	Domain         string `json:"domain,omitempty"`
	TunnelProxyURL string `json:"tunnelProxyUrl,omitempty"`
}

func (r *StatusResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		Status              string `json:"status"`
		ComputerID          string `json:"computer_id"`
		ComputerIDCamel     string `json:"computerId"`
		ComputerToken       string `json:"computer_token"`
		ComputerTokenCamel  string `json:"computerToken"`
		Domain              string `json:"domain"`
		TunnelProxyURL      string `json:"tunnel_proxy_url"`
		TunnelProxyURLCamel string `json:"tunnelProxyUrl"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Status = raw.Status
	r.ComputerID = firstNonEmpty(raw.ComputerID, raw.ComputerIDCamel)
	r.ComputerToken = firstNonEmpty(raw.ComputerToken, raw.ComputerTokenCamel)
	r.Domain = raw.Domain
	r.TunnelProxyURL = firstNonEmpty(raw.TunnelProxyURL, raw.TunnelProxyURLCamel)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type Client struct {
	AppURL     string
	HTTPClient *http.Client
}

func NewClient(appURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		AppURL:     strings.TrimRight(appURL, "/"),
		HTTPClient: hc,
	}
}

func (c *Client) Mint(ctx context.Context, req MintRequest) (*MintResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.AppURL+"/api/v1/computers/device-codes",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mint device code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"mint device code: HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(buf)),
		)
	}
	var wrapped struct {
		Data MintResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("decode mint response: %w", err)
	}
	if wrapped.Data.Code == "" || wrapped.Data.ConfirmURL == "" {
		return nil, errors.New(
			"mint response missing code or confirmUrl — server may be misconfigured",
		)
	}
	return &wrapped.Data, nil
}

func (c *Client) PollOnce(ctx context.Context, code string) (*StatusResponse, error) {
	url := c.AppURL + "/api/v1/computers/device-codes/" + code + "/status"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &StatusResponse{Status: "not_found"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"poll status: HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(buf)),
		)
	}
	var wrapped struct {
		Data StatusResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("decode poll response: %w", err)
	}
	return &wrapped.Data, nil
}

type PollResult int

const (
	PollApproved PollResult = iota
	PollDenied
	PollExpired
	PollNotFound
	PollCanceled
)

func (c *Client) Poll(ctx context.Context, code string, interval time.Duration) (*StatusResponse, PollResult, error) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, PollCanceled, err
		}
		view, err := c.PollOnce(ctx, code)
		if err != nil {
			if ctx.Err() != nil {
				return nil, PollCanceled, ctx.Err()
			}
		} else {
			switch view.Status {
			case "approved":
				return view, PollApproved, nil
			case "denied":
				return view, PollDenied, nil
			case "expired":
				return view, PollExpired, nil
			case "not_found":
				return view, PollNotFound, nil
			case "claimed":
				return view, PollApproved, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, PollCanceled, ctx.Err()
		case <-t.C:
		}
	}
}

func GuessOS() string   { return runtime.GOOS }
func GuessArch() string { return runtime.GOARCH }
func GuessHostKind() string {
	for _, env := range []string{"XDG_SESSION_TYPE", "DISPLAY", "WAYLAND_DISPLAY"} {
		if v := getenv(env); v != "" {
			return "desktop"
		}
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return "desktop"
	}
	return "server"
}

var getenv = func(key string) string {
	v, _ := lookupEnv(key)
	return v
}

var lookupEnv = func(key string) (string, bool) {
	return osLookupEnv(key)
}
