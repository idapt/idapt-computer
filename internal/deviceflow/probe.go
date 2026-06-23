package deviceflow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PairingStatus int

const (
	PairingValid PairingStatus = iota
	PairingRevoked
	PairingUnknown
)

func ProbePairing(ctx context.Context, appURL, computerID, computerTokenHex string) PairingStatus {
	if computerID == "" || computerTokenHex == "" {
		return PairingUnknown
	}
	url := strings.TrimRight(appURL, "/") +
		"/api/cloud-computers/" + computerID + "/heartbeat"

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	message := "POST:/api/cloud-computers/" + computerID + "/heartbeat:" + timestamp
	keyBytes, err := hex.DecodeString(computerTokenHex)
	if err != nil {
		keyBytes = []byte(computerTokenHex)
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		probeCtx,
		http.MethodPost,
		url,
		strings.NewReader("{}"),
	)
	if err != nil {
		return PairingUnknown
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Computer-Signature", signature)
	req.Header.Set("X-Computer-Timestamp", timestamp)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return PairingUnknown
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return PairingValid
	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden,
		resp.StatusCode == http.StatusNotFound:
		return PairingRevoked
	default:
		return PairingUnknown
	}
}
