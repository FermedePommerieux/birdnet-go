//go:build integration

package v2only

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/datastore"
	v2 "github.com/tphakala/birdnet-go/internal/datastore/v2"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/postgresqltest"
	"github.com/tphakala/birdnet-go/internal/datastore/v2/repository"
	"github.com/tphakala/birdnet-go/internal/logger"
	"gorm.io/gorm"
)

func setupPostgresDatastore(t *testing.T) *Datastore {
	t.Helper()
	pg := postgresqltest.NewSettings(t)
	log := logger.NewConsoleLogger("v2only_postgres_test", logger.LogLevelError)
	manager, err := v2.NewPostgresManager(&pg)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, manager.Close()) })
	require.NoError(t, manager.Initialize())
	require.Equal(t, "postgres", manager.DB().Name())
	cfg := buildConfigForManager(t, manager, log, false, nil)
	cfg.AppEvent = repository.NewAppEventRepository(manager.DB(), nil, false, false)
	ds, err := New(cfg)
	require.NoError(t, err)
	ds.timezone = time.UTC
	return ds
}

func TestPostgresFreshInstallReopenAndCRUD(t *testing.T) {
	pg := postgresqltest.NewSettings(t)
	settings := &conf.Settings{}
	settings.Output.PostgreSQL = pg
	ds, err := InitializeFreshInstall(settings, nil, nil)
	require.NoError(t, err)
	note := &datastore.Note{Date: "2026-06-01", Time: "12:30:00", ScientificName: "Turdus merula", Confidence: 0.91}
	require.NoError(t, ds.Save(note, nil))
	require.NotZero(t, note.ID)
	got, err := ds.Get(strconv.FormatUint(uint64(note.ID), 10))
	require.NoError(t, err)
	assert.Equal(t, "Turdus merula", got.ScientificName)
	require.NoError(t, ds.Close())

	log := logger.NewConsoleLogger("v2only_postgres_reopen", logger.LogLevelError)
	manager, err := v2.NewPostgresManager(&pg)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, manager.Close()) })
	require.NoError(t, manager.Initialize(), "reopening must preserve rows and migration state")
	cfg := buildConfigForManager(t, manager, log, false, nil)
	reopened, err := New(cfg)
	require.NoError(t, err)
	count, err := reopened.detection.CountAll(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.NoError(t, reopened.detection.Update(t.Context(), note.ID, map[string]any{"confidence": 0.82}))
	edited, err := reopened.detection.Get(t.Context(), note.ID)
	require.NoError(t, err)
	assert.InDelta(t, 0.82, edited.Confidence, 1e-9)
	require.NoError(t, reopened.detection.Delete(t.Context(), note.ID))
	count, err = reopened.detection.CountAll(t.Context())
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestPostgresDetectionRelationsAndConflicts(t *testing.T) {
	ds := setupPostgresDatastore(t)
	ctx := t.Context()
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	det := seedDetection(t, ds, "Parus major", at)
	require.NotZero(t, det.ID)
	read, err := ds.detection.GetWithRelations(ctx, det.ID)
	require.NoError(t, err)
	require.NotNil(t, read.Label)
	assert.Equal(t, "Parus major", read.Label.ScientificName)
	require.NotNil(t, read.Model)

	review := &entities.DetectionReview{DetectionID: det.ID, Verified: entities.VerificationCorrect}
	require.NoError(t, ds.detection.SaveReview(ctx, review))
	require.NoError(t, ds.detection.SaveReview(ctx, &entities.DetectionReview{DetectionID: det.ID, Verified: entities.VerificationFalsePositive}))
	storedReview, err := ds.detection.GetReview(ctx, det.ID)
	require.NoError(t, err)
	assert.Equal(t, entities.VerificationFalsePositive, storedReview.Verified)
	require.NoError(t, ds.detection.SaveReviewsBatch(ctx, []*entities.DetectionReview{
		{DetectionID: det.ID, Verified: entities.VerificationCorrect},
		{DetectionID: det.ID, Verified: entities.VerificationCorrect},
	}))
	storedReview, err = ds.detection.GetReview(ctx, det.ID)
	require.NoError(t, err)
	assert.Equal(t, entities.VerificationFalsePositive, storedReview.Verified, "batch conflict must preserve stored review")
	comment := &entities.DetectionComment{DetectionID: det.ID, Entry: "original"}
	require.NoError(t, ds.detection.SaveComment(ctx, comment))
	require.NotZero(t, comment.ID)
	require.NoError(t, ds.detection.UpdateComment(ctx, comment.ID, "edited"))
	comments, err := ds.detection.GetComments(ctx, det.ID)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "edited", comments[0].Entry)
	legacyID := uint(123)
	require.NoError(t, ds.detection.SaveCommentsBatch(ctx, []*entities.DetectionComment{
		{DetectionID: det.ID, Entry: "migrated", LegacyID: &legacyID},
		{DetectionID: det.ID, Entry: "duplicate", LegacyID: &legacyID},
	}))
	comments, err = ds.detection.GetComments(ctx, det.ID)
	require.NoError(t, err)
	assert.Len(t, comments, 2, "batch conflict must skip duplicate legacy comments")
	require.NoError(t, ds.detection.Lock(ctx, det.ID))
	require.NoError(t, ds.detection.SaveLocksBatch(ctx, []*entities.DetectionLock{{DetectionID: det.ID}, {DetectionID: det.ID}}))
	locked, err := ds.detection.IsLocked(ctx, det.ID)
	require.NoError(t, err)
	assert.True(t, locked)
	require.ErrorIs(t, ds.detection.Delete(ctx, det.ID), repository.ErrDetectionLocked)
	require.NoError(t, ds.detection.Unlock(ctx, det.ID))
	require.NoError(t, ds.detection.Delete(ctx, det.ID))
	var related int64
	require.NoError(t, ds.manager.DB().Model(&entities.DetectionReview{}).Where("detection_id = ?", det.ID).Count(&related).Error)
	assert.Zero(t, related, "review FK cascade")
	require.NoError(t, ds.manager.DB().Model(&entities.DetectionComment{}).Where("detection_id = ?", det.ID).Count(&related).Error)
	assert.Zero(t, related, "comment FK cascade")
}

func TestPostgresWeatherNotificationThresholdAndQuery(t *testing.T) {
	ds := setupPostgresDatastore(t)
	ctx := t.Context()
	det := seedDetection(t, ds, "Turdus merula", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	rows, total, err := ds.detection.Search(ctx, &repository.SearchFilters{Query: "tUrDuS"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	assert.Equal(t, det.ID, rows[0].ID)

	events := &entities.DailyEvents{Date: "2026-06-01", Sunrise: 100, Sunset: 200}
	require.NoError(t, ds.weather.SaveDailyEvents(ctx, events))
	_, err = ds.weather.SaveAllDailyEvents(ctx, []entities.DailyEvents{{Date: "2026-06-01", Sunrise: 999}})
	require.NoError(t, err)
	gotEvents, err := ds.weather.GetDailyEvents(ctx, events.Date)
	require.NoError(t, err)
	assert.EqualValues(t, 100, gotEvents.Sunrise, "duplicate daily event keeps first row")
	weather := &entities.HourlyWeather{DailyEventsID: gotEvents.ID, Time: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), Temperature: 17.5, Precipitation: 2.3, PrecipitationType: "rain"}
	require.NoError(t, ds.weather.SaveHourlyWeather(ctx, weather))
	gotWeather, err := ds.weather.GetHourlyWeather(ctx, events.Date)
	require.NoError(t, err)
	require.Len(t, gotWeather, 1)
	assert.InDelta(t, 2.3, gotWeather[0].Precipitation, 1e-9)

	now := time.Now().UTC().Truncate(time.Second)
	history := &entities.NotificationHistory{LabelID: det.LabelID, NotificationType: "new_species", LastSent: now, ExpiresAt: now.Add(time.Hour)}
	require.NoError(t, ds.notification.SaveNotificationHistory(ctx, history))
	found, err := ds.notification.GetNotificationHistory(ctx, "Turdus merula", "new_species")
	require.NoError(t, err)
	assert.Equal(t, det.LabelID, found.LabelID)
	threshold := &entities.DynamicThreshold{SpeciesName: "blackbird", ScientificName: "Turdus merula", Level: 1, CurrentValue: 0.8, BaseThreshold: 0.7, ValidHours: 24, ExpiresAt: now.Add(time.Hour), LastTriggered: now, FirstCreated: now}
	require.NoError(t, ds.threshold.SaveDynamicThreshold(ctx, threshold))
	gotThreshold, err := ds.threshold.GetDynamicThreshold(ctx, threshold.SpeciesName)
	require.NoError(t, err)
	assert.InDelta(t, 0.8, gotThreshold.CurrentValue, 1e-9)

	alerts := repository.NewAlertRuleRepository(ds.manager.DB(), nil)
	rule := &entities.AlertRule{
		Name: "Postgres alert", Enabled: true, ObjectType: "detection",
		TriggerType: "event", EventName: "detection.new_species",
		Conditions: []entities.AlertCondition{{Property: "confidence", Operator: "greater_than", Value: "0.8"}},
		Actions:    []entities.AlertAction{{Target: "bell"}},
	}
	require.NoError(t, alerts.CreateRule(ctx, rule))
	require.NotZero(t, rule.ID)
	foundRule, err := alerts.GetRule(ctx, rule.ID)
	require.NoError(t, err)
	assert.True(t, foundRule.Enabled)
	assert.Len(t, foundRule.Conditions, 1)
	assert.Len(t, foundRule.Actions, 1)
	require.NoError(t, alerts.ToggleRule(ctx, rule.ID, false))
	active, err := alerts.GetEnabledRules(ctx)
	require.NoError(t, err)
	assert.Empty(t, active)
}

func TestPostgresWallClockAndDetectionRate(t *testing.T) {
	t.Run("DST", func(t *testing.T) { assertHourFilterMatchesDashboardOnDSTDays(t, setupPostgresDatastore(t)) })
	t.Run("detection rate", func(t *testing.T) { assertDetectionRateLabelsUTC(t, setupPostgresDatastore(t)) })
}

func TestPostgresPrivacyHourlyConflictDelta(t *testing.T) {
	ds := setupPostgresDatastore(t)
	ctx := t.Context()
	src := &entities.AudioSource{SourceURI: "test", NodeName: "n", SourceType: entities.SourceTypeFile}
	require.NoError(t, ds.manager.DB().Create(src).Error)
	repo := repository.NewPrivacyFilterHourlyRepository(ds.manager.DB(), nil, false, false)
	row := &entities.PrivacyFilterHourly{
		HourStart: 3600, SourceID: src.ID, ModelID: "BirdNET", TriggerType: "label",
		ThresholdMilli: 500, Hits: 2, MaxScore: 0.7, MarginLt50: 2,
	}
	require.NoError(t, repo.AddDelta(ctx, row))
	row.Hits, row.MaxScore, row.MarginLt50 = 3, 0.9, 3
	require.NoError(t, repo.AddDelta(ctx, row))
	var saved []entities.PrivacyFilterHourly
	require.NoError(t, ds.manager.DB().Find(&saved).Error)
	require.Len(t, saved, 1)
	assert.EqualValues(t, 5, saved[0].Hits)
	assert.EqualValues(t, 5, saved[0].MarginLt50)
	assert.InDelta(t, 0.9, saved[0].MaxScore, 1e-9)
}

func TestPostgresTransactionRollsBack(t *testing.T) {
	ds := setupPostgresDatastore(t)
	seed := seedDetection(t, ds, "Parus major", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	err := ds.Transaction(func(tx *gorm.DB) error {
		newRow := &entities.Detection{ModelID: seed.ModelID, LabelID: seed.LabelID, DetectedAt: seed.DetectedAt + 1, Confidence: 0.6}
		if createErr := tx.Create(newRow).Error; createErr != nil {
			return createErr
		}
		return fmt.Errorf("rollback deliberate test transaction")
	})
	require.Error(t, err)
	count, err := ds.detection.CountAll(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}

func TestPostgresRangeAnalyticsAndInsights(t *testing.T) {
	t.Run("range and DST", func(t *testing.T) {
		assertRangeAnalyticsWallClock(t, setupPostgresDatastore(t))
	})
	t.Run("year and top species", func(t *testing.T) {
		ds := setupPostgresDatastore(t)
		ctx := t.Context()
		loc, err := time.LoadLocation("Europe/Helsinki")
		require.NoError(t, err)
		ds.timezone = loc
		first := seedDetection(t, ds, "Turdus merula", time.Date(2024, 3, 31, 0, 30, 0, 0, time.UTC))
		seedDetection(t, ds, "Turdus merula", time.Date(2025, 3, 30, 1, 30, 0, 0, time.UTC))
		seedDetection(t, ds, "Parus major", time.Date(2025, 3, 30, 2, 30, 0, 0, time.UTC))
		insights := repository.NewInsightsRepository(ds.manager.DB(), false, false)
		ranges := []repository.TimeRange{
			{Start: time.Date(2024, 3, 30, 0, 0, 0, 0, time.UTC).Unix(), End: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC).Unix()},
			{Start: time.Date(2025, 3, 29, 0, 0, 0, 0, time.UTC).Unix(), End: time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC).Unix()},
		}
		expected, err := insights.GetExpectedSpeciesToday(ctx, ranges, loc, nil)
		require.NoError(t, err)
		require.NotEmpty(t, expected)
		assert.Equal(t, "Turdus merula", expected[0].ScientificName)
		assert.Equal(t, 2, expected[0].YearsSeen)
		assert.Equal(t, "2025-03-30", expected[0].LastSeenDate)
		top, err := ds.detection.GetTopSpecies(ctx, ranges[1].Start, ranges[1].End, 0, nil, nil, 10)
		require.NoError(t, err)
		require.Len(t, top, 2)
		assert.Equal(t, first.LabelID, top[0].LabelID)
		byModel, err := ds.detection.GetTopSpeciesByModel(ctx, ds.defaultModelID, 10)
		require.NoError(t, err)
		require.Len(t, byModel, 2)
		assert.EqualValues(t, 2, byModel[0].Count)
		todaySince := time.Date(2025, 3, 30, 0, 0, 0, 0, loc).Unix()
		kpis, err := insights.GetDashboardKPIs(ctx, todaySince, loc, nil)
		require.NoError(t, err)
		assert.EqualValues(t, 2, kpis.LifetimeSpecies)
		assert.Equal(t, "2025-03-30", kpis.BestDayDate)
		assert.EqualValues(t, 2, kpis.BestDayCount)
	})
}

func TestPostgresRetentionAndPruning(t *testing.T) {
	ds := setupPostgresDatastore(t)
	ctx := t.Context()
	clipName := "2026/01/bird.wav"
	det := seedDetection(t, ds, "Parus major", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	require.NoError(t, ds.detection.Update(ctx, det.ID, map[string]any{"clip_name": clipName}))
	affected, err := ds.RetainNoteSpectrogramsByClipNames([]string{clipName})
	require.NoError(t, err)
	assert.EqualValues(t, 1, affected)
	stored, err := ds.detection.Get(ctx, det.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.ClipName)
	require.NotNil(t, stored.SpectrogramClipName)
	assert.Equal(t, clipName, *stored.SpectrogramClipName)

	now := time.Now().UTC()
	require.NoError(t, ds.manager.DB().Create(&entities.AppEvent{Timestamp: now.AddDate(0, 0, -30), Category: "test", EventType: "old"}).Error)
	require.NoError(t, ds.manager.DB().Create(&entities.AppEvent{Timestamp: now, Category: "test", EventType: "current"}).Error)
	removed, err := ds.PruneAppEvents(ctx, 7)
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed)
	var remaining int64
	require.NoError(t, ds.manager.DB().Model(&entities.AppEvent{}).Count(&remaining).Error)
	assert.EqualValues(t, 1, remaining)
}

func TestPostgresConcurrentDetectionSaves(t *testing.T) {
	ds := setupPostgresDatastore(t)
	const writers = 8
	var wg sync.WaitGroup
	errors := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			note := &datastore.Note{Date: "2026-06-01", Time: fmt.Sprintf("12:00:%02d", i), ScientificName: "Turdus merula", Confidence: 0.9}
			errors <- ds.Save(note, nil)
		})
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	count, err := ds.detection.CountAll(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, writers, count)
}
