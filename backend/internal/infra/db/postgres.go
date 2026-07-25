package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
	SSLMode  string
}

func New(cfg Config) (*pgxpool.Pool, error) {
	if cfg.Host == "" {
		cfg.Host = os.Getenv("PGHOST")
	}
	if cfg.Port == "" {
		cfg.Port = os.Getenv("PGPORT")
	}
	if cfg.Database == "" {
		cfg.Database = os.Getenv("PGDATABASE")
	}
	if cfg.User == "" {
		cfg.User = os.Getenv("PGUSER")
	}
	if cfg.Password == "" {
		cfg.Password = os.Getenv("PGPASSWORD")
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = os.Getenv("PGSSLMODE")
	}

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxpool.Ping: %w", err)
	}

	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// Ensure migration tracking table exists
	_, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name VARCHAR(256) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") && !strings.HasSuffix(e.Name(), ".down.sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	sort.Strings(sqlFiles)

	// Track applied migrations
	for _, name := range sqlFiles {
		versionStr := extractMigrationVersion(name)
		if versionStr == "" {
			continue
		}
		version, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			continue
		}
		var applied bool
		_ = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied)
		if applied {
			continue
		}

		path := filepath.Join(migrationsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			version, name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}

	return nil
}

func extractMigrationVersion(filename string) string {
	name := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(name, "_", 2)
	if len(parts) > 0 {
		// accept any numeric prefix (e.g. "000001", "20260417090001")
		v := parts[0]
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			return v
		}
	}
	return ""
}

func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pgxpool.Ping: %w", err)
	}
	return nil
}
