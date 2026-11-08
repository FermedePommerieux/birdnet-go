//go:build integration

package v2only

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgreSQLDetectionRateDistinctZones checks the live inspector methods
// with different application and database session time zones.
func TestPostgreSQLDetectionRateDistinctZones(t *testing.T) {
	ds := setupPostgresDatastore(t)
	db := ds.manager.DB()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec("SET TIME ZONE '+05:45'").Error)
	appZone, err := time.LoadLocation("Europe/Paris")
	require.NoError(t, err)
	ds.timezone = appZone

	assertDetectionRateLabelsUTC(t, ds)
}

// TestPostgreSQLDetectionRateDSTBuckets exercises the actual inspector methods
// with a controlled PostgreSQL clock at both European DST transitions.
func TestPostgreSQLDetectionRateDSTBuckets(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
	}{
		{name: "spring forward", day: time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC)},
		{name: "fall back", day: time.Date(2026, time.October, 25, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := setupPostgresDatastore(t)
			db := ds.manager.DB()
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.Exec("SET TIME ZONE 'America/New_York'").Error)
			require.NoError(t, db.Exec("SET search_path = public, pg_catalog").Error)
			appZone, err := time.LoadLocation("Europe/Paris")
			require.NoError(t, err)
			ds.timezone = appZone

			// Search path selects this clock only in the isolated test database.
			require.NoError(t, db.Exec(`
				CREATE FUNCTION public.now() RETURNS timestamptz
				LANGUAGE sql STABLE AS $$
					SELECT current_setting('birdnet.test_now')::timestamptz
				$$
			`).Error)
			now := tc.day.Add(4 * time.Hour)
			require.NoError(t, db.Exec("SELECT set_config('birdnet.test_now', ?, false)", now.Format(time.RFC3339)).Error)
			var databaseNow time.Time
			require.NoError(t, db.Raw("SELECT now()").Scan(&databaseNow).Error)
			assert.True(t, databaseNow.Equal(now), "test clock must shadow pg_catalog.now")

			instants := []time.Time{
				tc.day.Add(-30 * time.Minute), // prior UTC date, same Paris date
				tc.day.Add(15 * time.Minute),
				tc.day.Add(45 * time.Minute), // same UTC hour as previous
				tc.day.Add(75 * time.Minute),
			}
			for _, at := range instants {
				seedDetection(t, ds, detectionRateSpecies, at)
			}
			seedDetection(t, ds, detectionRateSpecies, now.Add(-25*time.Hour))

			hourly, err := ds.GetDetectionRate24h()
			require.NoError(t, err)
			gotHours := make(map[string]int64, len(hourly))
			for _, row := range hourly {
				gotHours[row.Hour] = row.Count
			}
			assert.Equal(t, map[string]int64{
				tc.day.Add(-time.Hour).Format(detectionRateHourLayout): 1,
				tc.day.Format(detectionRateHourLayout):                 2,
				tc.day.Add(time.Hour).Format(detectionRateHourLayout):  1,
			}, gotHours)

			daily, err := ds.GetDetectionRateDaily(1)
			require.NoError(t, err)
			gotDays := make(map[string]int64, len(daily))
			for _, row := range daily {
				gotDays[row.Date] = row.Count
			}
			assert.Equal(t, map[string]int64{
				tc.day.AddDate(0, 0, -1).Format(detectionRateDayLayout): 1,
				tc.day.Format(detectionRateDayLayout):                   3,
			}, gotDays)
		})
	}
}
