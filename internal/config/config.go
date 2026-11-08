package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	JWTSecret     string
	DemoCode      string
	SeedDemo      bool
	MediaDir      string
	MigrationsDir string
	AdminPhone    string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
}

func Load() Config {
	return Config{
		Addr:          env("HTTP_ADDR", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://sonichub:sonichub@localhost:5432/sonichub?sslmode=disable"),
		JWTSecret:     env("JWT_SECRET", "sonichub-dev-secret-change-me"),
		DemoCode:      env("DEMO_AUTH_CODE", "1234"),
		SeedDemo:      envBool("SEED_DEMO", true),
		MediaDir:      env("MEDIA_DIR", "./data/media"),
		MigrationsDir: env("MIGRATIONS_DIR", "./migrations"),
		AdminPhone:    env("ADMIN_PHONE", "13800138000"),
		AccessTTL:     time.Duration(envInt("ACCESS_TOKEN_MINUTES", 1440)) * time.Minute,
		RefreshTTL:    time.Duration(envInt("REFRESH_TOKEN_DAYS", 30)) * 24 * time.Hour,
	}
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
