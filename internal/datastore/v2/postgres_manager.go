package v2

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	postgresMaxOpenConns    = 25
	postgresMaxIdleConns    = 10
	postgresConnMaxLifetime = time.Hour
	postgresConnMaxIdleTime = 10 * time.Minute
	postgresConnectTimeout  = 5
	postgresAdvisoryLockKey = 0x626e65747632
	postgresSQLStateLength  = 5
)

// PostgresManager manages the PostgreSQL v2 schema. It never accesses the legacy schema.
type PostgresManager struct {
	db       *gorm.DB
	location string
}

// postgresConnectionURL constructs an escaped URL so special characters in credentials
// and database names cannot change the connection parameters.
func postgresConnectionURL(cfg *conf.PostgreSQLSettings) string {
	u := &url.URL{
		Scheme:  "postgresql",
		User:    url.UserPassword(cfg.Username, cfg.Password),
		Host:    net.JoinHostPort(cfg.Host, cfg.Port),
		Path:    "/" + cfg.Database,
		RawPath: "/" + url.PathEscape(cfg.Database),
	}
	q := u.Query()
	q.Set("sslmode", cfg.SSLMode)
	q.Set("connect_timeout", strconv.Itoa(postgresConnectTimeout))
	if cfg.SSLRootCert != "" {
		q.Set("sslrootcert", cfg.SSLRootCert)
	}
	if cfg.SSLCert != "" {
		q.Set("sslcert", cfg.SSLCert)
	}
	if cfg.SSLKey != "" {
		q.Set("sslkey", cfg.SSLKey)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// postgresSafeError never exposes a driver error: connection errors can include
// credentials, certificate paths, and URL-escaped versions of those values.
func postgresSafeError(err error) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		code := pgErr.Code
		if len(code) == postgresSQLStateLength && strings.IndexFunc(code, func(r rune) bool {
			return r < '0' || r > 'Z' || (r > '9' && r < 'A')
		}) == -1 {
			return fmt.Errorf("PostgreSQL server error (SQLSTATE %s)", code)
		}
		return errors.NewStd("PostgreSQL server error")
	}
	if safe := postgresSafeConnectivityError(err); safe != nil {
		return fmt.Errorf("PostgreSQL connection failed: %w", safe)
	}
	return errors.NewStd("PostgreSQL connection failed; check database settings and server availability")
}

// postgresSafeConnectivityError retains only the verified failure category.
// Network error wrappers may contain the host, user or connection URL.
func postgresSafeConnectivityError(err error) error {
	if !errors.IsConnectivityError(err) {
		return nil
	}
	if errors.Is(err, driver.ErrBadConn) {
		return driver.ErrBadConn
	}
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		return &net.DNSError{Err: "lookup failed", IsTimeout: dnsErr.IsTimeout, IsNotFound: dnsErr.IsNotFound, IsTemporary: dnsErr.IsTemporary}
	}
	if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Op == "dial" {
		if opErr.Timeout() {
			return &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
		}
		if sysErr, ok := errors.AsType[*os.SyscallError](opErr.Err); ok {
			if errno, ok := errors.AsType[syscall.Errno](sysErr.Err); ok {
				return &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: errno}}
			}
		}
	}
	// Some driver versions flatten the dial chain. Preserve only a known,
	// fixed reason found in the original classified message.
	message := strings.ToLower(err.Error())
	for _, reason := range []string{"connection refused", "no route to host", "network is unreachable", "i/o timeout", "no such host", "server misbehaving"} {
		if strings.Contains(message, reason) {
			return errors.NewStd("dial tcp: " + reason)
		}
	}
	return nil
}

// NewPostgresManager opens a PostgreSQL v2 database with a bounded connection pool.
func NewPostgresManager(cfg *conf.PostgreSQLSettings) (*PostgresManager, error) {
	if err := cfg.ValidateConnection(); err != nil {
		return nil, err
	}
	// A driver error can contain connection parameters. Suppress GORM's direct
	// error logging and return a fixed safe error to the caller instead.
	db, err := gorm.Open(postgres.Open(postgresConnectionURL(cfg)), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, postgresSafeError(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, postgresSafeError(err)
	}
	sqlDB.SetMaxOpenConns(postgresMaxOpenConns)
	sqlDB.SetMaxIdleConns(postgresMaxIdleConns)
	sqlDB.SetConnMaxLifetime(postgresConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(postgresConnMaxIdleTime)
	return &PostgresManager{
		db:       db,
		location: net.JoinHostPort(cfg.Host, cfg.Port) + "/" + cfg.Database,
	}, nil
}

// Initialize creates and seeds the v2 schema. It is safe to call on every boot.
func (m *PostgresManager) Initialize() error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", postgresAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("failed to lock PostgreSQL v2 schema: %w", postgresSafeError(err))
		}
		startup := inspectPostgresSchema(tx)
		if startup.Error != nil {
			return startup.Error
		}
		if err := tx.AutoMigrate(v2Entities()...); err != nil {
			safeErr := postgresSafeError(err)
			reportInitFailure("postgresql", "AutoMigrate", safeErr)
			return fmt.Errorf("failed to migrate PostgreSQL v2 schema: %w", safeErr)
		}
		if err := validatePostgresSchema(tx); err != nil {
			if errors.Is(err, ErrV2SchemaCorrupted) {
				return fmt.Errorf("PostgreSQL v2 schema integrity check failed: %w", err)
			}
			return fmt.Errorf("PostgreSQL v2 schema integrity check failed: %w", postgresSafeError(err))
		}
		if startup.FreshInstall {
			if err := MarkFreshInstallCompleted(tx); err != nil {
				safeErr := postgresSafeError(err)
				reportInitFailure("postgresql", "initMigrationState", safeErr)
				return fmt.Errorf("failed to initialize PostgreSQL migration state: %w", safeErr)
			}
		}
		if err := seedLookupTablesDB(tx); err != nil {
			safeErr := postgresSafeError(err)
			reportInitFailure("postgresql", "seedLookupTables", safeErr)
			return fmt.Errorf("failed to seed PostgreSQL lookup tables: %w", safeErr)
		}
		if err := seedDefaultModelDB(tx); err != nil {
			safeErr := postgresSafeError(err)
			reportInitFailure("postgresql", "seedDefaultModel", safeErr)
			return fmt.Errorf("failed to seed PostgreSQL default model: %w", safeErr)
		}
		return nil
	})
}

// postgresColumnLister lists columns in the active PostgreSQL schema.
func postgresColumnLister(db *gorm.DB, tableName string) ([]string, error) {
	var names []string
	err := db.Raw("SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? ORDER BY ordinal_position", tableName).Scan(&names).Error
	return names, err
}

// validatePostgresSchema rejects missing columns after AutoMigrate and leaves
// historical extra columns alone. It never drops a table on startup.
func validatePostgresSchema(db *gorm.DB) error {
	var missing []missingColumnsEntry
	for _, entity := range v2Entities() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(entity); err != nil {
			return fmt.Errorf("parse PostgreSQL v2 entity: %w", err)
		}
		actual, err := postgresColumnLister(db, stmt.Schema.Table)
		if err != nil {
			return fmt.Errorf("list PostgreSQL v2 columns: %w", err)
		}
		found := make(map[string]bool, len(actual))
		for _, name := range actual {
			found[name] = true
		}
		var absent []string
		for _, name := range stmt.Schema.DBNames {
			if !found[name] {
				absent = append(absent, name)
			}
		}
		if len(absent) > 0 {
			missing = append(missing, missingColumnsEntry{table: stmt.Schema.Table, columns: absent})
		}
	}
	if len(missing) > 0 {
		reportMissingColumns("postgresql", missing)
		return fmt.Errorf("%w: %s", ErrV2SchemaCorrupted, formatMissingColumns(missing))
	}
	return nil
}

// DB returns the underlying GORM database.
func (m *PostgresManager) DB() *gorm.DB { return m.db }

// Path returns a display location without credentials.
func (m *PostgresManager) Path() string { return m.location }

// Close closes the database connection pool.
func (m *PostgresManager) Close() error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL connection pool: %w", err)
	}
	return sqlDB.Close()
}

// CheckpointWAL is unnecessary for PostgreSQL, whose WAL is server managed.
func (m *PostgresManager) CheckpointWAL() error { return nil }

// Exists reports whether the v2 migration marker table exists.
func (m *PostgresManager) Exists() bool { return m.db.Migrator().HasTable(&entities.MigrationState{}) }

// IsMySQL reports false for PostgreSQL.
func (m *PostgresManager) IsMySQL() bool { return false }

// TablePrefix returns the empty prefix used by PostgreSQL v2 tables.
func (m *PostgresManager) TablePrefix() string { return "" }

// Delete refuses to remove a PostgreSQL v2 schema because it is never a
// disposable migration staging database.
func (m *PostgresManager) Delete() error {
	return errors.NewStd("refusing to delete a PostgreSQL v2 database")
}
