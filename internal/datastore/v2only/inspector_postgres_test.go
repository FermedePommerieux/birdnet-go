//go:build integration

package v2only

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/datastore"
	v2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/logger"
)

// TestPostgreSQLInspector checks metadata, table stats and UTC rate buckets on
// a disposable PostgreSQL test database.
func TestPostgreSQLInspector(t *testing.T) {
	cfg := postgresqltest.NewSettings(t)
	log := logger.NewConsoleLogger("postgres_inspector_test", logger.LogLevelDebug)
	mgr, err := v2.NewPostgresManager(&cfg)
	require.NoError(t, err)
	require.NoError(t, mgr.Initialize())
	dsCfg := buildConfigForManager(t, mgr, log, false, nil)
	ds, err := New(dsCfg)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, ds.Close()) })
	ds.timezone = time.UTC
	sqlDB, err := mgr.DB().DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, mgr.DB().Exec("SET TIME ZONE '+05:45'").Error)
	var sessionZone string
	require.NoError(t, mgr.DB().Raw("SHOW TIME ZONE").Scan(&sessionZone).Error)
	assert.Equal(t, "+05:45", sessionZone)

	details, err := ds.GetEngineDetails()
	require.NoError(t, err)
	require.NotNil(t, details.PostgreSQL)
	assert.NotEmpty(t, details.PostgreSQL.EngineVersion)
	var actualSize int64
	require.NoError(t, mgr.DB().Raw("SELECT pg_database_size(current_database())").Scan(&actualSize).Error)
	require.Positive(t, actualSize)
	assert.Equal(t, actualSize, details.PostgreSQL.DatabaseSizeBytes)
	assert.Nil(t, details.SQLite)
	assert.Nil(t, details.MySQL)

	stats, err := ds.GetDatabaseStats(t.Context())
	require.NoError(t, err)
	assert.Equal(t, v2.DialectPostgres, stats.Type)
	assert.Equal(t, actualSize, stats.SizeBytes)

	tables, err := ds.GetTableStats()
	require.NoError(t, err)
	require.NotEmpty(t, tables)
	var detections *datastore.TableStats
	for i := range tables {
		if tables[i].Name == detectionsBaseTable {
			detections = &tables[i]
			break
		}
	}
	require.NotNil(t, detections)
	assert.Positive(t, detections.SizeBytes)
	assert.Equal(t, "PostgreSQL", detections.Engine)

	assertDetectionRateLabelsUTC(t, ds)
}
