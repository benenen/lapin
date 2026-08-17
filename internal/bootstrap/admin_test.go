package bootstrap

import (
	"context"
	"testing"
)

func TestEnsureAdminRejectsIncompleteCredentialsBeforeDatabaseAccess(t *testing.T) {
	for _, credentials := range []AdminCredentials{
		{},
		{Email: "admin@example.com"},
		{Password: "correct horse battery staple"},
	} {
		if err := EnsureAdmin(context.Background(), nil, credentials); err == nil {
			t.Fatal("incomplete administrator credentials were accepted")
		}
	}
}

func TestContainsRole(t *testing.T) {
	if !containsRole([]string{"learner", "admin"}, "admin") {
		t.Fatal("existing administrator role was not found")
	}
	if containsRole([]string{"learner"}, "admin") {
		t.Fatal("missing administrator role was reported as present")
	}
}
