//go:build integration

// Integration tests for the PostgreSQL ShipmentRepository. They run a real
// Postgres via testcontainers-go and apply the service's migrations, so they
// exercise the actual SQL (not a mock). Run with:
//
//	go test -tags=integration ./internal/core/repository/...
//
// Requires a reachable Docker daemon. Excluded from the default `go test ./...`
// unit run by the `integration` build tag.
package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/duynhlab/shipping-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestDB starts a throwaway Postgres, applies the migrations, and returns a
// pool for the repository under test. Everything is torn down via t.Cleanup.
func newTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("shipping"),
		postgres.WithUsername("shipping"),
		postgres.WithPassword("secret"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	applyMigrations(t, ctx, dsn)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// applyMigrations runs every db/migrations/sql/*.up.sql in lexical order using a
// simple-protocol connection (so multi-statement files execute in one round).
func applyMigrations(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect for migrations: %v", err)
	}
	defer conn.Close(ctx)

	dir := filepath.Join("..", "..", "..", "..", "db", "migrations", "sql")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := conn.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", f, err)
		}
	}
}

func TestShipmentRepository_Integration(t *testing.T) {
	pool := newTestDB(t)
	repo := NewShipmentRepository(pool)
	ctx := context.Background()
	const orderID = "1001" // not present in the seed data (1,2,4)

	t.Run("CreateShipment is idempotent by order id", func(t *testing.T) {
		first, err := repo.CreateShipment(ctx, orderID)
		if err != nil {
			t.Fatalf("CreateShipment: %v", err)
		}
		if first.TrackingNumber != "MOP0000001001" {
			t.Errorf("tracking = %q, want MOP0000001001", first.TrackingNumber)
		}
		if first.Status != "pending" {
			t.Errorf("status = %q, want pending", first.Status)
		}

		again, err := repo.CreateShipment(ctx, orderID)
		if err != nil {
			t.Fatalf("CreateShipment (retry): %v", err)
		}
		if again.ID != first.ID {
			t.Errorf("retry created a new row: id %d != %d", again.ID, first.ID)
		}
	})

	t.Run("GetByOrderID / GetByTrackingNumber find it", func(t *testing.T) {
		byOrder, err := repo.GetByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("GetByOrderID: %v", err)
		}
		byTrack, err := repo.GetByTrackingNumber(ctx, byOrder.TrackingNumber)
		if err != nil {
			t.Fatalf("GetByTrackingNumber: %v", err)
		}
		if byTrack.ID != byOrder.ID {
			t.Errorf("tracking lookup id %d != order lookup id %d", byTrack.ID, byOrder.ID)
		}
	})

	t.Run("missing rows return ErrShipmentNotFound", func(t *testing.T) {
		if _, err := repo.GetByOrderID(ctx, "987654"); !errors.Is(err, domain.ErrShipmentNotFound) {
			t.Errorf("GetByOrderID(missing) err = %v, want ErrShipmentNotFound", err)
		}
		if _, err := repo.GetByTrackingNumber(ctx, "NOPE"); !errors.Is(err, domain.ErrShipmentNotFound) {
			t.Errorf("GetByTrackingNumber(missing) err = %v, want ErrShipmentNotFound", err)
		}
	})

	t.Run("CancelShipment marks cancelled and is idempotent", func(t *testing.T) {
		if err := repo.CancelShipment(ctx, orderID); err != nil {
			t.Fatalf("CancelShipment: %v", err)
		}
		got, err := repo.GetByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("GetByOrderID after cancel: %v", err)
		}
		if got.Status != "cancelled" {
			t.Errorf("status = %q, want cancelled", got.Status)
		}
		// Second cancel is a no-op (still succeeds).
		if err := repo.CancelShipment(ctx, orderID); err != nil {
			t.Errorf("second CancelShipment: %v", err)
		}
	})

	t.Run("CreateShipment rejects a non-numeric order id", func(t *testing.T) {
		if _, err := repo.CreateShipment(ctx, "not-a-number"); err == nil {
			t.Error("CreateShipment(non-numeric) = nil error, want error")
		}
	})
}
