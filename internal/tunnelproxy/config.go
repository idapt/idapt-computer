package tunnelproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr string
	RedisURL string
	BaseDomain string
	AppURL string
	JWTPublicKeyPEM string
	CookieSecure bool
	DevAuthBypass bool
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		ListenAddr:      envOr("IDAPT_TUNNEL_LISTEN_ADDR", ":8080"),
		RedisURL:        os.Getenv("IDAPT_TUNNEL_REDIS_URL"),
		BaseDomain:      strings.ToLower(envOr("IDAPT_TUNNEL_BASE_DOMAIN", "idapt.computer")),
		AppURL:          strings.TrimRight(envOr("IDAPT_TUNNEL_APP_URL", "https://idapt.app"), "/"),
		JWTPublicKeyPEM: os.Getenv("IDAPT_TUNNEL_JWT_PUBLIC_KEY_PEM"),
		CookieSecure:    envBool("IDAPT_TUNNEL_COOKIE_SECURE", true),
		DevAuthBypass:   envBool("IDAPT_TUNNEL_DEV_AUTH_BYPASS", false),
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("IDAPT_TUNNEL_REDIS_URL is required")
	}
	if cfg.JWTPublicKeyPEM == "" {
		return nil, fmt.Errorf("IDAPT_TUNNEL_JWT_PUBLIC_KEY_PEM is required")
	}
	if err := validateTunnelJWTPublicKeyPEM(cfg.JWTPublicKeyPEM); err != nil {
		return nil, fmt.Errorf("IDAPT_TUNNEL_JWT_PUBLIC_KEY_PEM is invalid: %w", err)
	}
	return cfg, nil
}

func validateTunnelJWTPublicKeyPEM(raw string) error {
	block, rest := pem.Decode([]byte(raw))
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}
	if strings.TrimSpace(string(rest)) != "" {
		return fmt.Errorf("contains extra data after PEM block")
	}
	if block.Type != "PUBLIC KEY" {
		return fmt.Errorf("expected PUBLIC KEY PEM block")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	ecdsaKey, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("expected ECDSA public key")
	}
	if ecdsaKey.Curve != elliptic.P256() {
		return fmt.Errorf("expected P-256 public key")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
