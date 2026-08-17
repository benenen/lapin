package main

import (
	"context"
	"log"

	"github.com/benenen/lapin/internal/bootstrap"
	"github.com/benenen/lapin/internal/config"
	"github.com/benenen/lapin/internal/database"
	"github.com/benenen/lapin/internal/watchadmin"
)

func main() {
	settings, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if settings.AdminEmail == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_PASSWORD are required")
	}
	if err := watchadmin.Validate(settings); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, settings.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	if err := bootstrap.EnsureAdmin(ctx, pool, bootstrap.AdminCredentials{
		Email:    settings.AdminEmail,
		Password: settings.AdminPassword,
	}); err != nil {
		log.Fatal(err)
	}
}
