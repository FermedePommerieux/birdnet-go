package v2

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/dbserver"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/errors"
	"gorm.io/gorm"
)

func TestPostgresManagerInitializeReopen(t *testing.T) {
	settings := &conf.Settings{}
	settings.Output.PostgreSQL = postgresqltest.NewSettings(t)
	cfg := &settings.Output.PostgreSQL
	m, err := NewPostgresManager(cfg)
	require.NoError(t, err)
	require.NoError(t, m.Close())

	initial := CheckMigrationStateBeforeStartup(settings)
	require.NoError(t, initial.Error)
	assert.True(t, initial.FreshInstall)

	m, err = NewPostgresManager(cfg)
	require.NoError(t, err)
	require.NoError(t, m.Initialize())
	require.True(t, m.Exists())
	require.NoError(t, m.DB().Create(&entities.AppMetadata{Key: "postgres-reopen-test", Value: "preserved"}).Error)
	require.NoError(t, m.Close())

	reopened := CheckMigrationStateBeforeStartup(settings)
	require.NoError(t, reopened.Error)
	assert.True(t, reopened.V2Available)
	assert.False(t, reopened.LegacyRequired)
	assert.Equal(t, entities.MigrationStatusCompleted, reopened.MigrationStatus)

	m, err = NewPostgresManager(cfg)
	require.NoError(t, err)
	require.NoError(t, m.Initialize())
	var metadata entities.AppMetadata
	require.NoError(t, m.DB().First(&metadata, "key = ?", "postgres-reopen-test").Error)
	assert.Equal(t, "preserved", metadata.Value)
	require.Error(t, m.Delete())
	require.NoError(t, m.DB().First(&metadata, "key = ?", "postgres-reopen-test").Error)
	require.NoError(t, m.Close())
}

func TestPostgresConnectionURLRedactsError(t *testing.T) {
	cfg := &conf.PostgreSQLSettings{
		Host: "127.0.0.1", Port: "5432", Username: "user@name", Password: "secret:/@?#",
		Database: "database", SSLMode: "disable",
	}
	u := postgresConnectionURL(cfg)
	assert.NotContains(t, u, "secret:/@?#")
	assert.Contains(t, u, "user%40name")
	assert.NotContains(t, postgresSafeError(assert.AnError).Error(), cfg.Password)
	serverErr := postgresSafeError(&pgconn.PgError{Code: "23505", Message: cfg.Password})
	assert.Contains(t, serverErr.Error(), "23505")
	assert.NotContains(t, serverErr.Error(), cfg.Password)
	connectErr := postgresSafeError(fmt.Errorf("dial tcp 127.0.0.1:5432: connect: connection refused"))
	assert.NotContains(t, connectErr.Error(), "127.0.0.1")
	assert.True(t, errors.IsConnectivityError(connectErr))
	cfg.Host = "host,other"
	_, err := NewPostgresManager(cfg)
	require.Error(t, err)
}

func TestPostgresStartupErrorsGiveSafeSettingsHints(t *testing.T) {
	const secret = "user=secret_user password=secret_pw client=192.0.2.7 dsn=postgresql://secret_user:secret_pw@db.example/secret_db"
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{"wrong password", &pgconn.PgError{Code: "28P01", Message: secret, Detail: secret}, []string{"SQLSTATE 28P01", "output.postgresql.username", "output.postgresql.password", "BIRDNET_POSTGRESQL_PASSWORD", "server authentication rules"}},
		{"missing database", &pgconn.PgError{Code: "3D000", Message: secret}, []string{"SQLSTATE 3D000", "output.postgresql.database", "database exists"}},
		{"schema permission", &pgconn.PgError{Code: "42501", Message: secret}, []string{"SQLSTATE 42501", "schema privileges", "output.postgresql.username"}},
		{"certificate rejected", &tls.CertificateVerificationError{Err: fmt.Errorf("%s", secret)}, []string{"output.postgresql.host", "output.postgresql.sslrootcert"}},
		{"server unavailable", &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.7"), Port: 5432}, Err: syscall.ECONNREFUSED}, []string{"connection refused", "output.postgresql.host", "output.postgresql.port", "server availability"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := postgresSafeError(fmt.Errorf("%s: %w", secret, tt.err))
			for _, want := range tt.want {
				assert.Contains(t, got.Error(), want)
			}
			for _, private := range []string{"secret_user", "secret_pw", "192.0.2.7", "postgresql://", "secret_db"} {
				assert.NotContains(t, got.Error(), private)
			}
		})
	}
}

func TestPostgresTransactionErrorSanitizesExternalFailures(t *testing.T) {
	const secret = "postgresql://secret_user:secret_pw@192.0.2.7/secret_db"
	for _, name := range []string{"BEGIN", "COMMIT"} {
		t.Run(name, func(t *testing.T) {
			got := postgresTransactionError(fmt.Errorf("%s failed for %s: TLS certificate rejected", name, secret), nil)
			require.Error(t, got)
			require.ErrorContains(t, got, "check output.postgresql")
			assert.NotContains(t, got.Error(), secret)
		})
	}
	callbackErr := fmt.Errorf("PostgreSQL preflight rejected settings: UTF8 required")
	require.ErrorIs(t, postgresTransactionError(callbackErr, callbackErr), callbackErr)
	assert.NoError(t, postgresTransactionError(nil, nil))
}

func TestPostgresRefusesUnrelatedSchema(t *testing.T) {
	settings := &conf.Settings{}
	settings.Output.PostgreSQL = postgresqltest.NewSettings(t)
	m, err := NewPostgresManager(&settings.Output.PostgreSQL)
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	require.NoError(t, m.DB().Exec("CREATE TABLE user_data (id bigint PRIMARY KEY)").Error)
	require.NoError(t, m.DB().Exec("INSERT INTO user_data (id) VALUES (1)").Error)

	state := CheckMigrationStateBeforeStartup(settings)
	require.ErrorIs(t, state.Error, ErrV2DatabaseCorrupted)
	require.ErrorIs(t, m.Initialize(), ErrV2DatabaseCorrupted)
	var count int64
	require.NoError(t, m.DB().Table("user_data").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestPostgresConcurrentInitialize(t *testing.T) {
	cfg := postgresqltest.NewSettings(t)
	managers := make([]*PostgresManager, 2)
	for i := range managers {
		var err error
		managers[i], err = NewPostgresManager(&cfg)
		require.NoError(t, err)
	}
	defer func() {
		for _, manager := range managers {
			require.NoError(t, manager.Close())
		}
	}()
	var wg sync.WaitGroup
	results := make(chan error, len(managers))
	for _, manager := range managers {
		wg.Go(func() { results <- manager.Initialize() })
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}

func TestPostgresUnsupportedEncodingCreatesNoSchema(t *testing.T) {
	cfg := postgresqltest.NewSettings(t)
	m, err := NewPostgresManager(&cfg)
	require.NoError(t, err)
	defer func() { require.NoError(t, m.Close()) }()
	report, err := dbserver.ReadPostgreSQL(t.Context(), m.DB())
	require.NoError(t, err)
	assert.True(t, report.VersionSupported())
	assert.True(t, report.EncodingSupported())

	err = m.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL client_encoding TO LATIN1").Error; err != nil {
			return err
		}
		return initializePostgresSchema(tx)
	})
	require.ErrorContains(t, err, "UTF8 server and client encodings")

	var tables int64
	require.NoError(t, m.DB().Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'").Scan(&tables).Error)
	assert.Zero(t, tables)

	require.NoError(t, m.Initialize())
	require.NoError(t, m.DB().Create(&entities.AppMetadata{Key: "preflight-restart", Value: "preserved"}).Error)
	err = m.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL client_encoding TO LATIN1").Error; err != nil {
			return err
		}
		return initializePostgresSchema(tx)
	})
	require.ErrorContains(t, err, "UTF8 server and client encodings")
	var preserved entities.AppMetadata
	require.NoError(t, m.DB().First(&preserved, "key = ?", "preflight-restart").Error)
	assert.Equal(t, "preserved", preserved.Value)
}

func TestValidatePostgresPreflightReportsAllFailedRequirements(t *testing.T) {
	report := dbserver.PostgreSQLReport{
		Version: "16.9", VersionNum: 160009,
		ServerEncoding: "LATIN1", ClientEncoding: "SQL_ASCII",
	}
	err := validatePostgresPreflight(report)
	require.Error(t, err)
	for _, detail := range []string{
		`server_version="16.9"`, "server_version_num=160009",
		`server_encoding="LATIN1"`, `client_encoding="SQL_ASCII"`,
		"version_supported=false", "encodings_supported=false",
		"supported majors (17, 18)", "UTF8 server and client encodings",
		"creating the database with UTF8 server encoding", "configuring UTF8 client encoding",
	} {
		require.ErrorContains(t, err, detail)
	}
	assert.NoError(t, validatePostgresPreflight(dbserver.PostgreSQLReport{
		Version: "18.6", VersionNum: 180006,
		ServerEncoding: "UTF8", ClientEncoding: "UTF8",
	}))
}
