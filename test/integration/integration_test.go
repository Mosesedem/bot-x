//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegrationSmoke is a scaffolded integration test that starts Postgres and Redis
// using testcontainers. It's build-tagged with `integration` so it won't run by default.
func TestIntegrationSmoke(t *testing.T) {
	ctx := context.Background()

	// Start Postgres
	pgReq := tc.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "secret",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	pgC, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: pgReq, Started: true})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	defer pgC.Terminate(ctx)

	pgHost, err := pgC.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get postgres host: %v", err)
	}
	pgPort, err := pgC.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get postgres port: %v", err)
	}

	dsn := fmt.Sprintf("postgres://postgres:secret@%s:%s/testdb?sslmode=disable", pgHost, pgPort.Port())

	// Wait a moment for Postgres to accept connections
	time.Sleep(2 * time.Second)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}

	// Basic query to ensure the DB is responsive
	var now time.Time
	if err := db.QueryRow("SELECT NOW()").Scan(&now); err != nil {
		t.Fatalf("failed to query now: %v", err)
	}

	t.Logf("postgres is responsive: %s", now)
}
