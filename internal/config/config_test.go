package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("HASHID_SALT", "")
	t.Setenv("SECURE_COOKIES", "true")
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.HTTPAddress != ":8080" || settings.HashIDSalt == "" || !settings.SecureCookies || settings.AssetDir != "data/assets" {
		t.Fatalf("unexpected non-secret settings: address=%q secure_cookies=%t hashid_salt_empty=%t", settings.HTTPAddress, settings.SecureCookies, settings.HashIDSalt == "")
	}
}

func TestLoadRejectsMissingDatabaseAndInvalidBoolean(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing DATABASE_URL was accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("SECURE_COOKIES", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("invalid SECURE_COOKIES was accepted")
	}
}

func TestLoadRejectsDevelopmentHashIDSaltInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("APP_ENV", "production")
	t.Setenv("HASHID_SALT", "")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted the public development HashID salt")
	}
	t.Setenv("HASHID_SALT", "a-private-production-salt-with-32-chars")
	if _, err := Load(); err == nil {
		t.Fatal("production accepted insecure cookies")
	}
	t.Setenv("SECURE_COOKIES", "true")
	if _, err := Load(); err != nil {
		t.Fatalf("production rejected a private HashID salt: %v", err)
	}
}

func TestLoadParsesTrustedProxyCIDRs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32, 10.0.0.0/8")
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.TrustedProxyCIDRs) != 2 || !settings.TrustedProxyCIDRs[1].Contains([]byte{10, 1, 2, 3}) {
		t.Fatalf("unexpected trusted proxy CIDRs: %#v", settings.TrustedProxyCIDRs)
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("invalid trusted proxy CIDR was accepted")
	}
}

func TestLoadAdminBootstrapCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_EMAIL", "  ADMIN@Example.com ")
	t.Setenv("ADMIN_PASSWORD", "correct horse battery staple")

	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.AdminEmail != "admin@example.com" {
		t.Fatalf("admin email = %q", settings.AdminEmail)
	}
	if settings.AdminPassword != "correct horse battery staple" {
		t.Fatal("admin password was changed while loading configuration")
	}
}

func TestLoadRejectsInvalidAdminBootstrapCredentials(t *testing.T) {
	const secret = "do-not-leak-this-password"
	tests := []struct {
		name     string
		email    string
		password string
	}{
		{name: "missing password", email: "admin@example.com"},
		{name: "missing email", password: secret},
		{name: "invalid email", email: "not-an-email", password: secret},
		{name: "short password", email: "admin@example.com", password: "too-short"},
		{name: "short multibyte password", email: "admin@example.com", password: "四个汉字"},
		{name: "long password", email: "admin@example.com", password: strings.Repeat("x", 129)},
		{name: "multibyte password over byte limit", email: "admin@example.com", password: strings.Repeat("密", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://example")
			t.Setenv("ADMIN_EMAIL", test.email)
			t.Setenv("ADMIN_PASSWORD", test.password)
			_, err := Load()
			if err == nil {
				t.Fatal("invalid administrator bootstrap credentials were accepted")
			}
			if strings.Contains(err.Error(), test.password) && test.password != "" {
				t.Fatalf("configuration error leaked the password: %v", err)
			}
		})
	}
}
