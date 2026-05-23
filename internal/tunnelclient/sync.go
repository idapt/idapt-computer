package tunnelclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SyncedTunnel struct {
	Port     int    `json:"port"`
	AuthMode string `json:"authMode"`
	Hostname string `json:"hostname"`
	URL      string `json:"url"`
}

type Syncer struct {
	appURL       string
	machineID    string
	machineToken string
	http         *http.Client
}

func NewSyncer(appURL, machineID, machineToken string) *Syncer {
	return &Syncer{
		appURL:       strings.TrimRight(appURL, "/"),
		machineID:    machineID,
		machineToken: machineToken,
		http:         &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *Syncer) Push(ctx context.Context, cfg Config) ([]SyncedTunnel, error) {
	path := fmt.Sprintf("/api/machines/%s/tunnels", s.machineID)
	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.appURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.sign(req, http.MethodPut, path, body)

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tunnels sync returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Tunnels []SyncedTunnel `json:"tunnels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode tunnels sync response: %w", err)
	}
	return out.Tunnels, nil
}

func (s *Syncer) MintDaemonToken(ctx context.Context) (string, error) {
	path := fmt.Sprintf("/api/machines/%s/tunnel-token", s.machineID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.appURL+path, nil)
	if err != nil {
		return "", err
	}
	s.sign(req, http.MethodGet, path, nil)

	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("tunnel-token returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("tunnel-token response carried no token")
	}
	return out.Token, nil
}

func (s *Syncer) sign(req *http.Request, method, path string, body []byte) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	msg := method + ":" + path + ":" + ts
	if method != http.MethodGet && method != http.MethodHead {
		sum := sha256.Sum256(body)
		bodyHash := hex.EncodeToString(sum[:])
		req.Header.Set("X-Machine-Content-SHA256", bodyHash)
		msg += ":" + bodyHash
	}
	key, err := hex.DecodeString(s.machineToken)
	if err != nil {
		key = []byte(s.machineToken)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	req.Header.Set("X-Machine-Signature", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Machine-Timestamp", ts)
}
