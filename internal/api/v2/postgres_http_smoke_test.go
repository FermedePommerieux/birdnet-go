//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/analysis"
	"github.com/tphakala/birdnet-go/internal/api/v2/analytics"
	"github.com/tphakala/birdnet-go/internal/api/v2/apitest"
	"github.com/tphakala/birdnet-go/internal/api/v2/detections"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/detection"
)

// TestPostgresHTTPReads starts the production database service against a disposable
// PostgreSQL database and serves the two read routes on an in-memory Echo server.
// No listener, audio device, or production database is opened by this test.
func TestPostgresHTTPReads(t *testing.T) {
	previousConfigPath := conf.ConfigPath
	conf.ConfigPath = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() { conf.ConfigPath = previousConfigPath })
	settings := apitest.NewValidTestSettings()
	settings.Output.PostgreSQL = postgresqltest.NewSettings(t)
	settings.Output.SQLite.Enabled = false
	settings.Output.MySQL.Enabled = false
	settings.Realtime.Audio.Export.Path = t.TempDir()
	metrics := apitest.NewTestMetrics(t)
	svc := analysis.NewDatabaseService(settings, metrics)
	require.NoError(t, svc.Start(t.Context()))
	t.Cleanup(func() { assert.NoError(t, svc.Stop(t.Context())) })
	require.True(t, svc.IsV2OnlyMode())
	require.Equal(t, "postgres", svc.V2Manager().DB().Name())
	result := &detection.Result{
		Timestamp:  time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC),
		Species:    detection.Species{ScientificName: "Turdus merula", CommonName: "Common Blackbird"},
		Confidence: 0.91,
		Model:      detection.DefaultModelInfo(),
	}
	require.NoError(t, svc.DetectionRepository().Save(t.Context(), result, nil))
	require.NotZero(t, result.ID)

	core := apitest.NewCore(t, apitest.WithDatastore(svc.DataStore()), apitest.WithSettings(settings), apitest.WithMetrics(metrics))
	core.Repo = svc.DetectionRepository()
	core.V2Manager = svc.V2Manager()
	core.AuthMiddleware = func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	emptyNames := func() map[string]string { return map[string]string{} }
	detectionHandler := detections.New(core, &sync.RWMutex{}, func() *conf.Settings { return settings }, nil, nil,
		func(echo.Context) bool { return false }, emptyNames, emptyNames, emptyNames)
	detectionHandler.RegisterDetectionRoutes(core.Group)
	analytics.New(core, nil, nil, nil).RegisterDatabaseOverviewRoutes(core.Group)
	e := core.Echo

	request := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		return rec
	}
	recent := request("/api/v2/detections/recent?limit=5")
	require.Equal(t, http.StatusOK, recent.Code, recent.Body.String())
	var recentDetections []struct {
		ID             uint   `json:"id"`
		ScientificName string `json:"scientificName"`
	}
	require.NoError(t, json.Unmarshal(recent.Body.Bytes(), &recentDetections))
	require.Len(t, recentDetections, 1)
	assert.Equal(t, result.ID, recentDetections[0].ID)
	assert.Equal(t, "Turdus merula", recentDetections[0].ScientificName)

	overview := request("/api/v2/system/database/overview")
	require.Equal(t, http.StatusOK, overview.Code, overview.Body.String())
	var stats analytics.DatabaseOverviewResponse
	require.NoError(t, json.Unmarshal(overview.Body.Bytes(), &stats))
	assert.Equal(t, "postgres", stats.Engine)
	assert.Equal(t, "connected", stats.Status)
	assert.EqualValues(t, 1, stats.TotalDetections)
	require.NotNil(t, stats.PostgreSQL)
	assert.NotEmpty(t, stats.PostgreSQL.EngineVersion)
	assert.Positive(t, stats.PostgreSQL.DatabaseSizeBytes)
	assert.Positive(t, stats.TotalTables)
}
