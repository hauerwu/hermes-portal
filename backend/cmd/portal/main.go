// Hermes Portal — management portal for multiple hermes-agent instances.
//
//   - Manage local (docker container) and remote (URL-onboarded) instances
//   - Multi-tenant RBAC (super admin / tenant admin / member)
//   - Local username/password + OIDC single sign-on
//   - Embedded hermes dashboards (reverse-proxied, zero code changes)
//   - Unified gateway: OpenAI API (API-key auth) + channel webhooks
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hermesportal/internal/config"
	"hermesportal/internal/database"
	"hermesportal/internal/router"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("[portal] data dir: %v", err)
	}

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("[portal] database: %v", err)
	}

	engine := router.New(cfg, db)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           engine,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("[portal] %s listening on %s (data=%s)", cfg.AppName, cfg.ListenAddr, cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[portal] server: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[portal] shutdown: %v", err)
	}
	log.Printf("[portal] bye")
}
