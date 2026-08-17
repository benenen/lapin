package httpapi_test

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/benenen/lapin/internal/bootstrap"
	"github.com/benenen/lapin/internal/database"
)

func TestBootstrapAdminCanLogInAndIsIdempotent(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	credentials := bootstrap.AdminCredentials{
		Email:    "admin@example.com",
		Password: "correct horse battery staple",
	}
	if err := bootstrap.EnsureAdmin(ctx, pool, credentials); err != nil {
		t.Fatal(err)
	}

	response := performJSON(h, "POST", "/api/v1/auth/login", `{"email":"admin@example.com","password":"correct horse battery staple"}`)
	assertStatus(t, response, 200)
	var payload struct {
		Data struct {
			User struct {
				Roles []string `json:"roles"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.User.Roles) != 1 || payload.Data.User.Roles[0] != "admin" {
		t.Fatalf("bootstrap roles = %#v", payload.Data.User.Roles)
	}

	changedCredentials := bootstrap.AdminCredentials{
		Email:    credentials.Email,
		Password: "a different secure password",
	}
	if err := bootstrap.EnsureAdmin(ctx, pool, changedCredentials); err != nil {
		t.Fatalf("idempotent bootstrap failed: %v", err)
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"admin@example.com","password":"correct horse battery staple"}`), 200)
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"admin@example.com","password":"a different secure password"}`), 401)
}

func TestBootstrapAdminRejectsExistingLearner(t *testing.T) {
	h := newTestApp(t)
	response := performJSON(h, "POST", "/api/v1/auth/register", `{"email":"claimed@example.com","name":"Claimed","password":"learner password phrase"}`)
	assertStatus(t, response, 201)

	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := bootstrap.EnsureAdmin(ctx, pool, bootstrap.AdminCredentials{
		Email:    "claimed@example.com",
		Password: "administrator password phrase",
	}); err == nil {
		t.Fatal("bootstrap promoted an existing non-admin account")
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"claimed@example.com","password":"learner password phrase"}`), 200)
}

func TestBootstrapAdminIsSafeAcrossConcurrentStarts(t *testing.T) {
	h := newTestApp(t)
	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	credentials := bootstrap.AdminCredentials{
		Email:    "concurrent-admin@example.com",
		Password: "concurrent administrator password",
	}
	startErrors := make(chan error, 2)
	var starts sync.WaitGroup
	for range 2 {
		starts.Add(1)
		go func() {
			defer starts.Done()
			startErrors <- bootstrap.EnsureAdmin(ctx, pool, credentials)
		}()
	}
	starts.Wait()
	close(startErrors)
	for startErr := range startErrors {
		if startErr != nil {
			t.Fatalf("concurrent bootstrap failed: %v", startErr)
		}
	}
	assertStatus(t, performJSON(h, "POST", "/api/v1/auth/login", `{"email":"concurrent-admin@example.com","password":"concurrent administrator password"}`), 200)
}
