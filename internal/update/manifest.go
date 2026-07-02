package update

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

const ManifestSchemaVersion = 1

type SignedManifest struct {
	Schema    int                       `json:"schema"`
	IssuedAt  time.Time                 `json:"issuedAt"`
	ExpiresAt time.Time                 `json:"expiresAt"`
	Counter   int64                     `json:"counter"`
	Targets   map[string]ManifestTarget `json:"targets"`
}

type ManifestTarget struct {
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
	SignatureHex  string `json:"signatureHex"`
	BinaryURL     string `json:"binaryUrl"`
	SignatureURL  string `json:"signatureUrl"`
	MinVersion    string `json:"minVersion"`
	StableVersion string `json:"stableVersion"`
	RolloutPct    int    `json:"rolloutPct"`
}

func CurrentTarget() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func ParseSignedManifest(raw []byte, hexSig string, now time.Time) (*SignedManifest, error) {
	if err := VerifyBytesHexSig(raw, hexSig); err != nil {
		return nil, fmt.Errorf("manifest signature: %w", err)
	}
	var m SignedManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Schema != ManifestSchemaVersion {
		return nil, fmt.Errorf("unsupported manifest schema %d (want %d)", m.Schema, ManifestSchemaVersion)
	}
	if m.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("manifest missing required expiresAt")
	}
	if now.After(m.ExpiresAt) {
		return nil, fmt.Errorf("manifest expired at %s", m.ExpiresAt.Format(time.RFC3339))
	}
	return &m, nil
}

func (m *SignedManifest) EffectiveTarget(computerID string) (*ManifestTarget, error) {
	key := CurrentTarget()
	entry, ok := m.Targets[key]
	if !ok {
		return nil, fmt.Errorf("no manifest target for %s", key)
	}
	effective := entry
	if !inRollout(computerID, entry.RolloutPct) {
		stable := entry.StableVersion
		if stable == "" {
			stable = entry.MinVersion
		}
		if stable != "" {
			effective.Version = stable
		}
	}
	return &effective, nil
}

func inRollout(computerID string, pct int) bool {
	if pct >= 100 {
		return true
	}
	if pct <= 0 {
		return false
	}
	if computerID == "" {
		return true
	}
	sum := sha256.Sum256([]byte(computerID))
	bucket := binary.BigEndian.Uint32(sum[:4]) % 100
	return int(bucket) < pct
}
