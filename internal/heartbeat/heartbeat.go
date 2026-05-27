package heartbeat

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	Interval = 30 * time.Second
	RequestTimeout = 5 * time.Second
)

type LoadedModelsProvider func(ctx context.Context) []string

type Heartbeat struct {
	appURL        string
	computerID    string
	computerToken string
	cliVersion    string
	loadedModels  LoadedModelsProvider
	client        *http.Client
	startTime     time.Time
}

func New(
	appURL, computerID, computerToken, cliVersion string,
	loadedModels LoadedModelsProvider,
) *Heartbeat {
	return &Heartbeat{
		appURL:        appURL,
		computerID:    computerID,
		computerToken: computerToken,
		cliVersion:    cliVersion,
		loadedModels:  loadedModels,
		client:        &http.Client{Timeout: RequestTimeout},
		startTime:     time.Now(),
	}
}

func (h *Heartbeat) Start(ctx context.Context) {
	h.send(ctx)

	ticker := time.NewTicker(Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("heartbeat: stopping")
			return
		case <-ticker.C:
			h.send(ctx)
		}
	}
}

func (h *Heartbeat) send(ctx context.Context) {
	payload := map[string]interface{}{
		"computerId": h.computerID,
		"cliVersion": h.cliVersion,
		"uptime":     int(time.Since(h.startTime).Seconds()),
		"timestamp":  time.Now().Unix(),
	}

	if h.loadedModels != nil {
		loaded := h.loadedModels(ctx)
		if loaded == nil {
			loaded = []string{}
		}
		payload["loadedModels"] = loaded
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("heartbeat: marshal error: %v", err)
		return
	}

	url := fmt.Sprintf("%s/api/cloud-computers/%s/heartbeat", h.appURL, h.computerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("heartbeat: request error: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	message := "POST:/api/cloud-computers/" + h.computerID + "/heartbeat:" + timestamp
	keyBytes, err := hex.DecodeString(h.computerToken)
	if err != nil {
		keyBytes = []byte(h.computerToken) // fallback: raw bytes if not valid hex
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Computer-Signature", signature)
	req.Header.Set("X-Computer-Timestamp", timestamp)

	resp, err := h.client.Do(req)
	if err != nil {
		log.Printf("heartbeat: send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("heartbeat: server returned %d", resp.StatusCode)
	}
}
