package tunnelproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	clockSkew    = 30 * time.Second
	maxTokenSize = 8192
	es256SigLen  = 64
)

const (
	audDaemon = "tunnel-daemon"
	audVisitor = "tunnel"
	audSSH = "tunnel-ssh"
)

type tokenClaims struct {
	Sub     string `json:"sub"`
	Aud     string `json:"aud"`
	Host    string `json:"host,omitempty"`
	Computer string `json:"computer,omitempty"`
	Exp     int64  `json:"exp"`
	Iat     int64  `json:"iat"`
}

type jwtVerifier struct {
	key *ecdsa.PublicKey
}

func newJWTVerifier(pemKey string) (*jwtVerifier, error) {
	key, err := parseES256PublicKey(pemKey)
	if err != nil {
		return nil, err
	}
	return &jwtVerifier{key: key}, nil
}

func (v *jwtVerifier) verify(token string) (*tokenClaims, error) {
	if token == "" || len(token) > maxTokenSize {
		return nil, errors.New("token missing or too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	headerJSON, err := b64urlDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}
	if header.Alg != "ES256" {
		return nil, fmt.Errorf("unsupported algorithm %q", header.Alg)
	}
	sig, err := b64urlDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != es256SigLen {
		return nil, fmt.Errorf("bad signature length %d", len(sig))
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(v.key, digest[:], r, s) {
		return nil, errors.New("invalid signature")
	}
	payloadJSON, err := b64urlDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims tokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if claims.Exp == 0 {
		return nil, errors.New("missing exp claim")
	}
	if time.Now().Unix() > claims.Exp+int64(clockSkew.Seconds()) {
		return nil, errors.New("token expired")
	}
	if claims.Sub == "" {
		return nil, errors.New("missing sub claim")
	}
	return &claims, nil
}

func parseES256PublicKey(pemKey string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemKey)))
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, errors.New("key is not an ECDSA P-256 public key")
	}
	return ec, nil
}

func b64urlDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
