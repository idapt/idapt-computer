package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const maxComputerHMACDrift = 90 * time.Second

func ValidateComputerHMAC(r *http.Request, computerToken string) error {
	signature := r.Header.Get("X-Computer-Signature")
	if signature == "" {
		return fmt.Errorf("missing X-Computer-Signature header")
	}

	timestamp := r.Header.Get("X-Computer-Timestamp")
	if timestamp == "" {
		return fmt.Errorf("missing X-Computer-Timestamp header")
	}

	message := r.Method + ":" + r.URL.Path + ":" + timestamp
	keyBytes, decodeErr := hex.DecodeString(computerToken)
	if decodeErr != nil {
		keyBytes = []byte(computerToken) // fallback: raw bytes if not valid hex
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	expectedBytes, _ := hex.DecodeString(expectedSig)

	if !hmac.Equal(sigBytes, expectedBytes) {
		return fmt.Errorf("invalid signature")
	}

	signedAt, err := parseComputerTimestamp(timestamp)
	if err != nil {
		return err
	}
	drift := time.Since(signedAt)
	if drift < -maxComputerHMACDrift || drift > maxComputerHMACDrift {
		return fmt.Errorf("stale timestamp")
	}

	return nil
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
