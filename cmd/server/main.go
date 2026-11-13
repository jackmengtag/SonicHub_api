package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/api"
	"github.com/jackmengtag/SonicHub_api/internal/config"
	"github.com/jackmengtag/SonicHub_api/internal/core"
	"github.com/jackmengtag/SonicHub_api/internal/platform/cos"
	"github.com/jackmengtag/SonicHub_api/internal/platform/db"
	"github.com/jackmengtag/SonicHub_api/internal/platform/music"
	"github.com/jackmengtag/SonicHub_api/internal/seed"
	"github.com/jackmengtag/SonicHub_api/internal/store"
	"github.com/jackmengtag/SonicHub_api/internal/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	database, err := db.Open(startCtx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	slog.Info("database connected")

	if err := db.Migrate(startCtx, database, cfg.MigrationsDir); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
	if cfg.SeedDemo {
		if err := seed.Seed(startCtx, database, cfg.AdminPhone); err != nil {
			slog.Error("seed failed", "error", err)
			os.Exit(1)
		}
		if err := seed.SeedOps(startCtx, database); err != nil {
			slog.Error("ops seed failed", "error", err)
			os.Exit(1)
		}
		slog.Info("demo seed checked")
	}
	if err := os.MkdirAll(filepath.Join(cfg.MediaDir, "generated"), 0o755); err != nil {
		slog.Error("create media dir failed", "error", err)
		os.Exit(1)
	}

	st := store.New(database)
	objects, err := cos.New(cfg)
	if err != nil {
		slog.Error("cos client init failed", "error", err)
	}
	slog.Info("object storage",
		"bucket", cfg.COSBucket, "region", cfg.COSRegion, "enabled", objects != nil && objects.Enabled())
	model := music.New(cfg)
	slog.Info("music model", "endpoint", cfg.MusicModelURL, "enabled", model.Enabled())
	svc := core.New(st, cfg.MediaDir, objects, model)
	go (&worker.Worker{Core: svc}).Run(ctx)

	api.New(cfg, st, svc).Start()
	slog.Info("server stopped")
}
