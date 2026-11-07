package v2

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/errors"
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
