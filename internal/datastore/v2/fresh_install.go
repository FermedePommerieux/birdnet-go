package v2

import (
	"time"

	"github.com/tphakala/birdnet-go/internal/datastore/v2/entities"
	"gorm.io/gorm"
)

// MarkFreshInstallCompleted records that a new v2 database has no legacy
// records or auxiliary data to migrate. Call it only when creating a store.
func MarkFreshInstallCompleted(db *gorm.DB) error {
	now := time.Now()
	state := entities.MigrationState{
		ID:          1,
		State:       entities.MigrationStatusCompleted,
		StartedAt:   &now,
		CompletedAt: &now,
		// Nothing to migrate: record that, with the attempt marker equal to StartedAt.
		AuxiliaryDataStatus:    entities.AuxiliaryDataStatusNotRequired,
		AuxiliaryDataStartedAt: &now,
		TotalRecords:           0,
		MigratedRecords:        0,
		LastMigratedID:         0,
		ErrorMessage:           "",
	}

	return db.Save(&state).Error
}
