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
)

//go:embed release-pubkey.pem
var publicKeyPEM []byte

var ErrPlaceholderKey = errors.New(
	"embedded release public key is a known placeholder — run prod-seed-runtime-secrets.sh with " +
		"IDAPT_SECRET_SEED_SCOPE=cli-signing-keypair and commit the printed hex to " +
		"services/idapt/internal/update/release-pubkey.pem before signing or shipping",
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
	pub, err := parseEmbeddedPubKey()
	if err != nil {
		return err
	}
	digest, err := sha256File(binaryPath)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, digest, signature) {
		return errors.New("signature verification failed")
	}
	return nil
}

func VerifyHexSig(binaryPath, hexSig string) error {
	sig, err := hex.DecodeString(hexSig)
	if err != nil {
		return fmt.Errorf("decode hex sig: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: %d", len(sig))
	}
	return Verify(binaryPath, sig)
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

// parseEmbeddedPubKey decodes the //go:embedded release-pubkey.pem into
func parseEmbeddedPubKey() (ed25519.PublicKey, error) {
	if len(publicKeyPEM) == 0 {
		return nil, errors.New("embedded public key is empty")
	}
	body := publicKeyPEM
	if hasPrefix(body, "-----BEGIN") {
		body = stripPemBoundaries(body)
	}
	clean := stripWhitespace(body)

	var raw []byte
	switch len(clean) {
	case 64:
		decoded, err := hex.DecodeString(string(clean))
		if err != nil {
			return nil, fmt.Errorf("decode hex pubkey: %w", err)
		}
		raw = decoded
	case ed25519.PublicKeySize:
		raw = clean
	default:
		return nil, fmt.Errorf("unrecognized public key format (%d bytes)", len(clean))
	}

	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key wrong length: %d", len(raw))
	}

	for _, placeholder := range knownPlaceholderKeys {
		if bytes.Equal(raw, placeholder) {
			return nil, ErrPlaceholderKey
		}
	}

	return ed25519.PublicKey(raw), nil
}

func hasPrefix(b []byte, p string) bool {
	if len(b) < len(p) {
		return false
	}
	return string(b[:len(p)]) == p
}

func stripPemBoundaries(b []byte) []byte {
	out := make([]byte, 0, len(b))
	skip := false
	for i := 0; i < len(b); i++ {
		if b[i] == '-' {
			skip = true
			continue
		}
		if b[i] == '\n' && skip {
			skip = false
			continue
		}
		if skip {
			continue
		}
		out = append(out, b[i])
	}
	return out
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
