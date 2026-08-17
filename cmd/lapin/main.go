package main

import (
	"context"
	"log"
	"time"

	"github.com/benenen/lapin/internal/assetcleanup"
	"github.com/benenen/lapin/internal/assetstore"
	"github.com/benenen/lapin/internal/bootstrap"
	"github.com/benenen/lapin/internal/config"
	"github.com/benenen/lapin/internal/database"
	"github.com/benenen/lapin/internal/httpapi"
)

func main() {
	settings, err := config.Load()
	if err != nil {
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
	if settings.AdminEmail != "" {
		if err := bootstrap.EnsureAdmin(ctx, pool, bootstrap.AdminCredentials{
			Email:    settings.AdminEmail,
			Password: settings.AdminPassword,
		}); err != nil {
			log.Fatal(err)
		}
	}
	assets, err := assetstore.New(settings.AssetDir)
	if err != nil {
		log.Fatal(err)
	}
	defer assets.Close()
	if err := assetcleanup.Reconcile(ctx, pool, assets); err != nil {
		log.Fatal(err)
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := assetcleanup.Reconcile(ctx, pool, assets); err != nil {
				log.Printf("asset cleanup failed: %v", err)
			}
		}
	}()

	server := httpapi.New(pool, httpapi.Options{
		HostPorts:         settings.HTTPAddress,
		SecureCookies:     settings.SecureCookies,
		HashIDSalt:        settings.HashIDSalt,
		TrustedProxyCIDRs: settings.TrustedProxyCIDRs,
		AssetStore:        assets,
	})
	server.Spin()
}
