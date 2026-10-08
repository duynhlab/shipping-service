//go:build integration

// Integration tests for the PostgreSQL ShipmentRepository. They run a real
// Postgres via testcontainers-go, migrate and seed it as the migrator, and run
// the repository as the runtime login, so they exercise the actual SQL and
// grants (not a mock). Run with:
//
//	go test -tags=integration ./internal/core/repository/...
//
// Requires a reachable Docker daemon. Excluded from the default `go test ./...`
// unit run by the `integration` build tag.
package postgres

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/duynhlab/pkg/migratex"
	"github.com/duynhlab/shipping-service/db/migrations"
	"github.com/duynhlab/shipping-service/db/seed"
	"github.com/duynhlab/shipping-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testDB holds the three logins of the RFC-0029 shape: a superuser that plays
// the platform (creates the roles, as CNPG does), the migrator, and the
// runtime pool the repository runs on.
type testDB struct {
	adminDSN    string
	migratorDSN string
	runtimeDSN  string
	runtime     *pgxpool.Pool
}

const ownerRole = "shipping_owner"

// startWithRoles starts a throwaway Postgres and creates shipping_owner /
// shipping_migrator / shipping_runtime the way the platform does. Everything
// is torn down via t.Cleanup.
func startWithRoles(t *testing.T) *testDB {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("shipping"),
		postgres.WithUsername("platform"),
		postgres.WithPassword("secret"),
		// Ready twice (initdb restarts the server once), then the published
		// port: the module's own strategy, so a test never races the restart.
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	db := &testDB{
		adminDSN:    adminDSN,
		migratorDSN: withUser(t, adminDSN, "shipping_migrator", "migrator"),
		runtimeDSN:  withUser(t, adminDSN, "shipping_runtime", "runtime"),
	}

	execAll(t, ctx, adminDSN,
		`CREATE ROLE shipping_owner NOLOGIN`,
		`CREATE ROLE shipping_migrator LOGIN NOINHERIT PASSWORD 'migrator'`,
		`CREATE ROLE shipping_runtime LOGIN PASSWORD 'runtime'`,
		`GRANT shipping_owner TO shipping_migrator WITH INHERIT FALSE, SET TRUE, ADMIN FALSE`,
		// The platform makes the owner own the database; on PG15+ that is what
		// gives it CREATE on the public schema (owned by pg_database_owner).
		`ALTER DATABASE shipping OWNER TO shipping_owner`,
	)
	return db
}

// newBareDB is newTestDB without migrations or seed; it returns the
// migrator's DSN.
func newBareDB(t *testing.T) string {
	t.Helper()
	return startWithRoles(t).migratorDSN
}

// newTestDB starts a throwaway Postgres with the three roles, migrates and
// seeds as the migrator after SET ROLE shipping_owner, and returns it with a
// pool connected as shipping_runtime.
func newTestDB(t *testing.T) *testDB {
	t.Helper()
	ctx := context.Background()
	db := startWithRoles(t)

	if err := migratex.Run(migrations.FS, "sql", db.migratorDSN, migratex.WithSetRole(ownerRole)); err != nil {
		t.Fatalf("migrate as the migrator: %v", err)
	}
	if err := seed.Apply(ctx, db.migratorDSN, ownerRole); err != nil {
		t.Fatalf("seed as the migrator: %v", err)
	}

	pool, err := pgxpool.New(ctx, db.runtimeDSN)
	if err != nil {
		t.Fatalf("new runtime pool: %v", err)
	}
	t.Cleanup(pool.Close)
	db.runtime = pool
	return db
}

func withUser(t *testing.T, dsn, user, password string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}

func execAll(t *testing.T, ctx context.Context, dsn string, stmts ...string) {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestShipmentRepository_Integration(t *testing.T) {
	db := newTestDB(t)
	repo := NewShipmentRepository(db.runtime)
	ctx := context.Background()
	const orderID = "1001" // arbitrary order id; the seed uses orders 1, 2 and 4

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
		// Second cancel is a no-op (still succeeds) and leaves the status cancelled.
		if err := repo.CancelShipment(ctx, orderID); err != nil {
			t.Errorf("second CancelShipment: %v", err)
		}
		got, err = repo.GetByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("GetByOrderID after second cancel: %v", err)
		}
		if got.Status != "cancelled" {
			t.Errorf("status after second cancel = %q, want cancelled", got.Status)
		}
	})

	// Regression: the proto promises CancelShipment succeeds for an order with no
	// shipment (the UPDATE simply affects 0 rows). The order-fulfillment saga
	// relies on this — its CancelShipment compensation can fire before, or without,
	// a shipment ever being created, and must not error.
	t.Run("CancelShipment on a nonexistent order is a no-op success", func(t *testing.T) {
		const noSuchOrder = "999999" // numeric, but no shipment is ever created for it in this test
		if err := repo.CancelShipment(ctx, noSuchOrder); err != nil {
			t.Errorf("CancelShipment(nonexistent) = %v, want nil (idempotent no-op)", err)
		}
	})

	t.Run("CreateShipment rejects a non-numeric order id", func(t *testing.T) {
		if _, err := repo.CreateShipment(ctx, "not-a-number"); err == nil {
			t.Error("CreateShipment(non-numeric) = nil error, want error")
		}
	})
}

// The RFC-0029 authorization contract, checked as the real logins: the owner
// owns every object, the runtime can serve traffic and nothing more, and the
// migration refuses to run without its role.
func TestAuthorization_Integration(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	t.Run("every relation belongs to shipping_owner", func(t *testing.T) {
		// Extension members belong to whoever ran CREATE EXTENSION; they are
		// not the schema's own objects.
		rows, err := db.runtime.Query(ctx, `
			SELECT c.relname || ':' || pg_get_userbyid(c.relowner)
			  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname = 'public' AND pg_get_userbyid(c.relowner) <> 'shipping_owner'
			   AND NOT EXISTS (SELECT 1 FROM pg_depend d
			                    WHERE d.classid = 'pg_class'::regclass
			                      AND d.objid = c.oid AND d.deptype = 'e')`)
		if err != nil {
			t.Fatalf("query owners: %v", err)
		}
		others, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("scan owners: %v", err)
		}
		if len(others) != 0 {
			t.Fatalf("relations not owned by shipping_owner: %v", others)
		}
	})

	t.Run("the runtime cannot change the schema or reach the migration table", func(t *testing.T) {
		for _, stmt := range []string{
			`CREATE TABLE public.evil (i int)`,
			`ALTER TABLE public.shipments ADD COLUMN evil int`,
			`DROP TABLE public.shipments`,
			`SELECT version FROM public.schema_migrations`,
			`SET ROLE shipping_owner`,
		} {
			_, err := db.runtime.Exec(ctx, stmt)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || (pgErr.Code != "42501" && pgErr.Code != "42704") {
				t.Errorf("%s as shipping_runtime: got %v, want permission denied", stmt, err)
			}
		}
		// Without GRANT OPTION, PostgreSQL only warns "no privileges were
		// granted"; the assertion is the effect, not an error code.
		if _, err := db.runtime.Exec(ctx, `GRANT SELECT ON public.shipments TO PUBLIC`); err != nil {
			t.Fatalf("grant attempt: %v", err)
		}
		var leaked bool
		if err := db.runtime.QueryRow(ctx,
			`SELECT has_table_privilege('public', 'public.shipments', 'SELECT')`).Scan(&leaked); err != nil {
			t.Fatalf("check PUBLIC access: %v", err)
		}
		if leaked {
			t.Fatal("shipping_runtime handed SELECT on shipments to PUBLIC")
		}
	})

	t.Run("migrate and seed refuse an empty DB_MIGRATION_ROLE", func(t *testing.T) {
		if err := migratex.Run(migrations.FS, "sql", db.migratorDSN, migratex.WithSetRole("")); err == nil {
			t.Fatal("migrate with an empty role succeeded")
		}
		if err := seed.Apply(ctx, db.migratorDSN, ""); err == nil {
			t.Fatal("seed with an empty role succeeded")
		}
	})

	t.Run("the migrator creates nothing as itself", func(t *testing.T) {
		// No SET ROLE at all: the NOINHERIT migrator has no right on the
		// owner's schema, so even golang-migrate's version table is refused.
		fresh := newBareDB(t)
		err := migratex.Run(migrations.FS, "sql", fresh)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("migrate without SET ROLE = %v, want permission denied", err)
		}
	})

	t.Run("seed as a role the login cannot switch to fails", func(t *testing.T) {
		err := seed.Apply(ctx, db.runtimeDSN, ownerRole)
		if err == nil || !strings.Contains(err.Error(), "SET ROLE") {
			t.Fatalf("seed as shipping_runtime = %v, want SET ROLE error", err)
		}
	})
}
