package dbserver

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/tphakala/birdnet-go/internal/errors"
	"gorm.io/gorm"
)

// PostgreSQLSupportedVersions lists the stable major releases tested with BirdNET-Go.
const PostgreSQLSupportedVersions = "17, 18"

const (
	postgresMinMajor     = 17
	postgresMaxMajor     = 18
	postgresVersionScale = 10000
)

var postgresStableVersion = regexp.MustCompile(`^(\d+)\.(\d+)(?: \([^()]*\))?$`)

// PostgreSQLReport records the version and encodings of one PostgreSQL session.
type PostgreSQLReport struct {
	Version        string
	VersionNum     int
	ServerEncoding string
	ClientEncoding string
}

// VersionSupported reports whether the version is a tested stable release and
// agrees with PostgreSQL's numeric server_version_num setting.
func (r PostgreSQLReport) VersionSupported() bool {
	match := postgresStableVersion.FindStringSubmatch(r.Version)
	if match == nil {
		return false
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	if majorErr != nil || minorErr != nil || major < postgresMinMajor || major > postgresMaxMajor || minor >= postgresVersionScale {
		return false
	}
	return r.VersionNum == major*postgresVersionScale+minor
}

// EncodingSupported reports whether both the database and this session use UTF8.
func (r PostgreSQLReport) EncodingSupported() bool {
	return r.ServerEncoding == "UTF8" && r.ClientEncoding == "UTF8"
}

// ReadPostgreSQL reads the server and session settings on the provided GORM
// handle. A transaction handle keeps the probe on its migration connection.
func ReadPostgreSQL(ctx context.Context, db *gorm.DB) (PostgreSQLReport, error) {
	var settings struct {
		Version        string
		VersionNumText string
		ServerEncoding string
		ClientEncoding string
	}
	err := db.WithContext(ctx).Raw(`SELECT current_setting('server_version') AS version,
		current_setting('server_version_num') AS version_num_text,
		current_setting('server_encoding') AS server_encoding,
		current_setting('client_encoding') AS client_encoding`).Scan(&settings).Error
	if err != nil {
		return PostgreSQLReport{}, errors.NewStd("could not read PostgreSQL server settings; check database access and availability")
	}
	versionNum, err := strconv.Atoi(strings.TrimSpace(settings.VersionNumText))
	if err != nil {
		return PostgreSQLReport{}, errors.NewStd("PostgreSQL reported an invalid server version number; check the server configuration")
	}
	return PostgreSQLReport{
		Version:        settings.Version,
		VersionNum:     versionNum,
		ServerEncoding: settings.ServerEncoding,
		ClientEncoding: settings.ClientEncoding,
	}, nil
}
