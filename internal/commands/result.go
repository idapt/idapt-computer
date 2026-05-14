package commands

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type HMACPoster struct {
	appURL       string
	machineID    string
	machineToken string
	client       *http.Client
}

func NewHMACPoster(appURL, machineID, machineToken string) *HMACPoster {
	return &HMACPoster{
		appURL:       appURL,
		machineID:    machineID,
		machineToken: machineToken,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *HMACPoster) Post(ctx context.Context, r Result) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/machines/%s/commands/%s/result", p.appURL, p.machineID, r.ID)
	path := fmt.Sprintf("/api/machines/%s/commands/%s/result", p.machineID, r.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	keyBytes, err := hex.DecodeString(p.machineToken)
	if err != nil {
		keyBytes = []byte(p.machineToken)
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte("POST:" + path + ":" + timestamp))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Machine-Signature", signature)
	req.Header.Set("X-Machine-Timestamp", timestamp)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("result post returned %d", resp.StatusCode)
	}
	return nil
}
