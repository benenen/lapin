package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL       string
	HTTPAddress       string
	Environment       string
	SecureCookies     bool
	HashIDSalt        string
	TrustedProxyCIDRs []*net.IPNet
}

func Load() (Config, error) {
	config := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddress: envOr("HTTP_ADDR", ":8080"),
		Environment: strings.ToLower(envOr("APP_ENV", "development")),
		HashIDSalt:  envOr("HASHID_SALT", "lapin-development-salt"),
	}
	if config.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if config.Environment == "production" && (config.HashIDSalt == "lapin-development-salt" || len(config.HashIDSalt) < 32) {
		return Config{}, fmt.Errorf("HASHID_SALT must be private and at least 32 characters in production")
	}
	if raw := os.Getenv("SECURE_COOKIES"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse SECURE_COOKIES: %w", err)
		}
		config.SecureCookies = value
	}
	if config.Environment == "production" && !config.SecureCookies {
		return Config{}, fmt.Errorf("SECURE_COOKIES must be true in production")
	}
	var err error
	config.TrustedProxyCIDRs, err = parseCIDRs(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func parseCIDRs(value string) ([]*net.IPNet, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		_, network, err := net.ParseCIDR(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("parse TRUSTED_PROXY_CIDRS: %w", err)
		}
		result = append(result, network)
	}
	return result, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
