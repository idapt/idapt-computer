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

const maxMachineHMACDrift = 90 * time.Second

func ValidateMachineHMAC(r *http.Request, machineToken string) error {
	signature := r.Header.Get("X-Machine-Signature")
	if signature == "" {
		return fmt.Errorf("missing X-Machine-Signature header")
	}

	timestamp := r.Header.Get("X-Machine-Timestamp")
	if timestamp == "" {
		return fmt.Errorf("missing X-Machine-Timestamp header")
	}

	message := r.Method + ":" + r.URL.Path + ":" + timestamp
	keyBytes, decodeErr := hex.DecodeString(machineToken)
	if decodeErr != nil {
		keyBytes = []byte(machineToken) // fallback: raw bytes if not valid hex
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

	signedAt, err := parseMachineTimestamp(timestamp)
	if err != nil {
		return err
	}
	drift := time.Since(signedAt)
	if drift < -maxMachineHMACDrift || drift > maxMachineHMACDrift {
		return fmt.Errorf("stale timestamp")
	}

	return nil
}

func parseMachineTimestamp(timestamp string) (time.Time, error) {
	value, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || value <= 0 {
		return time.Time{}, fmt.Errorf("invalid timestamp")
	}
	if value > 100_000_000_000 {
		return time.UnixMilli(value), nil
	}
	return time.Unix(value, 0), nil
}
