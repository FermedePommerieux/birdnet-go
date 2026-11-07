// Package postgresqltest creates disposable PostgreSQL databases for integration tests.
package postgresqltest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const postgresMaintenanceDB = "postgres"

// NewSettings creates a uniquely named, disposable database and returns its
// connection settings. Set BIRDNET_PG_TEST_PORT to enable these tests.
func NewSettings(t *testing.T) conf.PostgreSQLSettings {
	t.Helper()
	port := os.Getenv("BIRDNET_PG_TEST_PORT")
	if port == "" {
		t.Skip("set BIRDNET_PG_TEST_PORT to run PostgreSQL integration tests")
	}
	host := os.Getenv("BIRDNET_PG_TEST_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	user := os.Getenv("BIRDNET_PG_TEST_USER")
	if user == "" {
		user = os.Getenv("USER")
	}
	password := os.Getenv("BIRDNET_PG_TEST_PASSWORD")
	adminURL := &url.URL{
		Scheme: "postgresql",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + postgresMaintenanceDB,
	}
	if password == "" {
		adminURL.User = url.User(user)
	} else {
		adminURL.User = url.UserPassword(user, password)
	}
	q := adminURL.Query()
	q.Set("sslmode", "disable")
	adminURL.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(adminURL.String()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	var random [12]byte
	_, err = rand.Read(random[:])
	require.NoError(t, err)
	name := "birdnet_test_" + hex.EncodeToString(random[:])
	// name is built only from a fixed prefix and hex digits, safe as an SQL identifier.
	require.NoError(t, db.Exec(fmt.Sprintf("CREATE DATABASE %q", name)).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec(fmt.Sprintf("DROP DATABASE %q", name)).Error)
	})
	return conf.PostgreSQLSettings{
		Enabled: true, Username: user, Password: password, Database: name,
		Host: host, Port: port, SSLMode: "disable",
	}
}
