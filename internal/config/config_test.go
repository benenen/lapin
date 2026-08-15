package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("HASHID_SALT", "")
	t.Setenv("SECURE_COOKIES", "true")
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.HTTPAddress != ":8080" || settings.HashIDSalt == "" || !settings.SecureCookies {
		t.Fatalf("unexpected settings: %#v", settings)
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
