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

	"github.com/idapt/idapt-computer/internal/hardware"
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
	hardware      *hardware.Info
	runsAsRoot    bool
	installMode   string
	client        *http.Client
	startTime     time.Time
}

func New(
	appURL, computerID, computerToken, cliVersion string,
	loadedModels LoadedModelsProvider,
	hw *hardware.Info,
	runsAsRoot bool,
	installMode string,
) *Heartbeat {
	return &Heartbeat{
		appURL:        appURL,
		computerID:    computerID,
		computerToken: computerToken,
		cliVersion:    cliVersion,
		loadedModels:  loadedModels,
		hardware:      hw,
		runsAsRoot:    runsAsRoot,
		installMode:   installMode,
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
		"runsAsRoot":  h.runsAsRoot,
		"installMode": h.installMode,
	}

	if h.loadedModels != nil {
		loaded := h.loadedModels(ctx)
		if loaded == nil {
			loaded = []string{}
		}
		payload["loadedModels"] = loaded
	}

	if h.hardware != nil {
		payload["hardware"] = h.hardware
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
	bodyHashBytes := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodyHashBytes[:])
	message := "POST:/api/cloud-computers/" + h.computerID + "/heartbeat:" + timestamp + ":" + bodyHash
	keyBytes, err := hex.DecodeString(h.computerToken)
	if err != nil {
		keyBytes = []byte(h.computerToken) // fallback: raw bytes if not valid hex
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Computer-Signature", signature)
	req.Header.Set("X-Computer-Timestamp", timestamp)
	req.Header.Set("X-Computer-Content-SHA256", bodyHash)

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
