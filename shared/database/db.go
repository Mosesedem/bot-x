package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates a pgxpool with max conns 50 and health check period 30s.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 50
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

// Close closes the pool.
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

// Ping pings the database.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("database pool is closed")
	}
	return pool.Ping(ctx)
}
