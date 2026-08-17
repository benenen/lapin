package watchadmin

import (
	"strings"
	"testing"

	"github.com/benenen/lapin/internal/config"
)

func TestValidateDefaultCredentialsRequireLocalDevelopment(t *testing.T) {
	base := config.Config{
		DatabaseURL:   "postgres://user:password@127.0.0.1:5433/lapin_test?sslmode=disable",
		HTTPAddress:   "127.0.0.1:8080",
		Environment:   "development",
		AdminEmail:    DefaultEmail,
		AdminPassword: DefaultPassword,
	}
	if err := Validate(base); err != nil {
		t.Fatalf("local development config rejected: %v", err)
	}

	tests := []struct {
		name   string
		change func(*config.Config)
	}{
		{name: "production", change: func(value *config.Config) { value.Environment = "production" }},
		{name: "public HTTP", change: func(value *config.Config) { value.HTTPAddress = "0.0.0.0:8080" }},
		{name: "remote database", change: func(value *config.Config) { value.DatabaseURL = "postgres://user:password@db.example.com/lapin" }},
		{name: "query host override", change: func(value *config.Config) {
			value.DatabaseURL = "postgres://user:password@127.0.0.1/lapin?host=db.example.com"
		}},
		{name: "remote fallback", change: func(value *config.Config) {
			value.DatabaseURL = "host=127.0.0.1,db.example.com user=user password=password dbname=lapin"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.change(&candidate)
			if err := Validate(candidate); err == nil {
				t.Fatalf("unsafe default administrator config was accepted: %#v", candidate)
			}
		})
	}
}

func TestValidateAllowsCustomCredentialsWithoutLeakingThem(t *testing.T) {
	settings := config.Config{
		DatabaseURL:   "postgres://user:password@db.example.com/lapin",
		HTTPAddress:   "0.0.0.0:8080",
		Environment:   "development",
		AdminEmail:    "owner@example.com",
		AdminPassword: "custom-development-password",
	}
	if err := Validate(settings); err != nil {
		t.Fatalf("custom credentials were rejected: %v", err)
	}

	settings.AdminEmail = DefaultEmail
	settings.AdminPassword = DefaultPassword
	settings.DatabaseURL = "not a database URL"
	err := Validate(settings)
	if err == nil {
		t.Fatal("invalid default administrator database URL was accepted")
	}
	if strings.Contains(err.Error(), settings.AdminPassword) || strings.Contains(err.Error(), settings.DatabaseURL) {
		t.Fatalf("validation error leaked configuration: %v", err)
	}
}
