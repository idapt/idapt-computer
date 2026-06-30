package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed release-pubkey.pem
var publicKeyPEM []byte

var ErrPlaceholderKey = errors.New(
	"embedded release key set has no real key (only placeholders) — run prod-seed-runtime-secrets.sh with " +
		"IDAPT_SECRET_SEED_SCOPE=computer-signing-keypair and commit the printed hex to " +
		"services/idapt-computer/internal/update/release-pubkey.pem before signing or shipping",
)

var knownPlaceholderKeys = [][]byte{
	mustDecodeHex("0000000000000000000000000000000000000000000000000000000000000000"),
	mustDecodeHex("03a6bc32a8a241da0de7c5a9177e20216abde1b2d6a6fe0539eff0087c46c342"),
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic(fmt.Sprintf("knownPlaceholderKeys entry %q: %v", s, err))
	}
	return b
}

func Verify(binaryPath string, signature []byte) error {
	digest, err := sha256File(binaryPath)
	if err != nil {
		return err
	}
	return verifyDigest(digest, signature)
}

func VerifyBytes(data, signature []byte) error {
	digest := sha256.Sum256(data)
	return verifyDigest(digest[:], signature)
}

func verifyDigest(digest, signature []byte) error {
	keys, err := parseEmbeddedPubKeys()
	if err != nil {
		return err
	}
	for _, pub := range keys {
		if ed25519.Verify(pub, digest, signature) {
			return nil
		}
	}
	return errors.New("signature verification failed")
}

func VerifyHexSig(binaryPath, hexSig string) error {
	sig, err := decodeHexSig(hexSig)
	if err != nil {
		return err
	}
	return Verify(binaryPath, sig)
}

func VerifyBytesHexSig(data []byte, hexSig string) error {
	sig, err := decodeHexSig(hexSig)
	if err != nil {
		return err
	}
	return VerifyBytes(data, sig)
}

func decodeHexSig(hexSig string) ([]byte, error) {
	sig, err := hex.DecodeString(strings.TrimSpace(hexSig))
	if err != nil {
		return nil, fmt.Errorf("decode hex sig: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid signature length: %d", len(sig))
	}
	return sig, nil
}

func sha256File(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// parseEmbeddedPubKeys decodes the //go:embedded release-pubkey.pem into the
func parseEmbeddedPubKeys() ([]ed25519.PublicKey, error) {
	if len(publicKeyPEM) == 0 {
		return nil, errors.New("embedded release key set is empty")
	}

	var keys []ed25519.PublicKey
	sawPlaceholder := false

	for _, rawLine := range bytes.Split(publicKeyPEM, []byte("\n")) {
		line := stripWhitespace(rawLine)
		if len(line) == 0 || line[0] == '#' {
			continue // blank or comment
		}
		if len(line) != 2*ed25519.PublicKeySize {
			return nil, fmt.Errorf("unrecognized release key line (%d chars; want %d-char hex)", len(line), 2*ed25519.PublicKeySize)
		}
		raw, err := hex.DecodeString(string(line))
		if err != nil {
			return nil, fmt.Errorf("decode hex release key: %w", err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("release key wrong length: %d", len(raw))
		}
		if isPlaceholderKey(raw) {
			sawPlaceholder = true
			continue
		}
		keys = append(keys, ed25519.PublicKey(raw))
	}

	if os.Getenv("IDAPT_TEST_MODE") == "1" {
		if hexKey := strings.TrimSpace(os.Getenv("IDAPT_TEST_RELEASE_PUBKEY_HEX")); hexKey != "" {
			if raw, err := hex.DecodeString(hexKey); err == nil &&
				len(raw) == ed25519.PublicKeySize && !isPlaceholderKey(raw) {
				keys = append(keys, ed25519.PublicKey(raw))
			}
		}
	}

	if len(keys) == 0 {
		if sawPlaceholder {
			return nil, ErrPlaceholderKey
		}
		return nil, errors.New("no release keys found in embedded key set")
	}
	return keys, nil
}

func isPlaceholderKey(raw []byte) bool {
	for _, placeholder := range knownPlaceholderKeys {
		if bytes.Equal(raw, placeholder) {
			return true
		}
	}
	return false
}

func stripWhitespace(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		switch c {
		case '\r', '\n', '\t', ' ':
			continue
		}
		out = append(out, c)
	}
	return out
}
