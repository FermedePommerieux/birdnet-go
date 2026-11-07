//go:build integration

package analysis

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	importsapi "github.com/tphakala/birdnet-go/internal/api/v2/imports"
	"github.com/tphakala/birdnet-go/internal/conf"
	datastoreV2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/detection"
	"github.com/tphakala/birdnet-go/internal/observability"
)

func TestDatabaseServicePostgres(t *testing.T) {
	settings := &conf.Settings{}
	settings.Output.PostgreSQL = postgresqltest.NewSettings(t)
	previousConfig := conf.ConfigPath
	conf.ConfigPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() { conf.ConfigPath = previousConfig })
	settings.Output.SQLite.Path = filepath.Join(t.TempDir(), "unused.db")
	ctx := t.Context()
	t.Cleanup(func() {
		importsapi.SetMigrationDependencies(nil, nil)
		datastoreV2.ResetDatabaseMode()
	})
	metrics, err := observability.NewMetrics()
	require.NoError(t, err)

	start := func() *DatabaseService {
		t.Helper()
		svc := NewDatabaseService(settings, metrics)
		require.NoError(t, svc.Start(ctx))
		require.True(t, svc.IsV2OnlyMode())
		require.Equal(t, datastoreV2.DialectPostgres, datastoreV2.DialectName(svc.V2Manager()))
		return svc
	}
	svc := start()
	result := &detection.Result{
		Timestamp: time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC),
		Species:   detection.Species{ScientificName: "Parus major", CommonName: "Great Tit"},
		Model:     detection.DefaultModelInfo(),
	}
	require.NoError(t, svc.DetectionRepository().Save(ctx, result, nil))
	require.NotZero(t, result.ID)
	require.NoError(t, svc.Stop(ctx))

	restarted := start()
	t.Cleanup(func() { assert.NoError(t, restarted.Stop(ctx)) })
	// Lose a real PostgreSQL session. The next repository operation must obtain
	// a working connection without switching backends.
	db := restarted.V2Manager().DB()
	pool, err := db.DB()
	require.NoError(t, err)
	conn, err := pool.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var pid int
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	var killed bool
	require.NoError(t, db.Raw("SELECT pg_terminate_backend(?)", pid).Scan(&killed).Error)
	require.True(t, killed)
	require.Error(t, conn.PingContext(ctx))
	got, err := restarted.DetectionRepository().Get(ctx, fmt.Sprint(result.ID))
	require.NoError(t, err)
	assert.Equal(t, result.Timestamp.Unix(), got.Timestamp.Unix())
	assert.Equal(t, result.Species.ScientificName, got.Species.ScientificName)
	assert.NoFileExists(t, settings.Output.SQLite.Path)
}
