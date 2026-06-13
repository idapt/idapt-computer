package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const maxComputerHMACDrift = 90 * time.Second

const maxManagementBodySize = 1 << 20 // 1 MiB

func ValidateComputerHMAC(r *http.Request, computerToken string) error {
	signature := r.Header.Get("X-Computer-Signature")
	if signature == "" {
		return fmt.Errorf("missing X-Computer-Signature header")
	}

	timestamp := r.Header.Get("X-Computer-Timestamp")
	if timestamp == "" {
		return fmt.Errorf("missing X-Computer-Timestamp header")
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}

	signedAt, err := parseComputerTimestamp(timestamp)
	if err != nil {
		return err
	}
	drift := time.Since(signedAt)
	if drift < -maxComputerHMACDrift || drift > maxComputerHMACDrift {
		return fmt.Errorf("stale timestamp")
	}

	rawBody, err := readAndRestoreBody(r)
	if err != nil {
		return err
	}

	actualBytes := sha256.Sum256(rawBody)
	bodyHash := hex.EncodeToString(actualBytes[:])

	provided := r.Header.Get("X-Computer-Content-SHA256")
	hasBodyHashHeader := provided != ""
	if hasBodyHashHeader {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(bodyHash)) != 1 {
			return fmt.Errorf("body hash mismatch")
		}
	}

	keyBytes, decodeErr := hex.DecodeString(computerToken)
	if decodeErr != nil {
		keyBytes = []byte(computerToken)
	}

	bodyBoundMessage := r.Method + ":" + pathWithQuery(r) + ":" + timestamp + ":" + bodyHash
	legacyMessage := r.Method + ":" + r.URL.Path + ":" + timestamp

	matched := hmacMatches(keyBytes, bodyBoundMessage, sigBytes)
	if !matched {
		matched = hmacMatches(keyBytes, legacyMessage, sigBytes)
	}
	if !matched {
		return fmt.Errorf("invalid signature")
	}

	if seenSignatures.seenBefore(signature, signedAt) {
		return fmt.Errorf("replayed signature")
	}

	return nil
}

func hmacMatches(key []byte, message string, sig []byte) bool {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

func pathWithQuery(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return r.URL.Path
	}
	return r.URL.Path + "?" + r.URL.RawQuery
}

func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxManagementBodySize+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read body failed")
	}
	if len(buf) > maxManagementBodySize {
		return nil, fmt.Errorf("body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return buf, nil
}

func parseComputerTimestamp(timestamp string) (time.Time, error) {
	value, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, fmt.Errorf("invalid timestamp")
	}
	if value > 100_000_000_000 {
		return time.UnixMilli(value), nil
	}
	return time.Unix(value, 0), nil
}
type replayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

var seenSignatures = &replayCache{seen: make(map[string]time.Time)}

const replayRetention = 2 * maxComputerHMACDrift

func (c *replayCache) seenBefore(signature string, signedAt time.Time) bool {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	for sig, exp := range c.seen {
		if now.After(exp) {
			delete(c.seen, sig)
		}
	}

	if _, exists := c.seen[signature]; exists {
		return true
	}
	c.seen[signature] = signedAt.Add(replayRetention)
	return false
}

func ResetReplayCacheForTest() {
	seenSignatures.mu.Lock()
	defer seenSignatures.mu.Unlock()
	seenSignatures.seen = make(map[string]time.Time)
}
