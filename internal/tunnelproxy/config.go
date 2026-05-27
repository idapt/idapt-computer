package tunnelproxy

import (
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
		BaseDomain:      strings.ToLower(envOr("IDAPT_TUNNEL_BASE_DOMAIN", "idapt.app")),
		AppURL:          strings.TrimRight(envOr("IDAPT_TUNNEL_APP_URL", "https://idapt.ai"), "/"),
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
	return cfg, nil
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
