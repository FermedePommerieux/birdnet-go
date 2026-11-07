package conf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const postgresSaveFileConfig = `
output:
  sqlite:
    enabled: false
  postgresql:
    enabled: true
    username: birdnet
    password: file-password
    database: birdnet
    host: localhost
    port: "5432"
    sslmode: verify-full
`

func TestSaveSettingsPreservesPostgreSQLFilePasswordUnderEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvVarPostgreSQLPassword, "environment-password")
	settings, configPath := loadConfigFromYAML(t, postgresSaveFileConfig)
	require.Equal(t, "environment-password", settings.Output.PostgreSQL.Password)
	require.NoError(t, SaveSettings())

	onDisk := readPersistedSettings(t, configPath)
	assert.Equal(t, "file-password", onDisk.Output.PostgreSQL.Password)
	assert.Equal(t, "environment-password", GetSettings().Output.PostgreSQL.Password)
	data, err := os.ReadFile(configPath) //nolint:gosec // test-controlled config path
	require.NoError(t, err)
	assert.NotContains(t, string(data), "environment-password")
}

func TestSaveSettingsPreservesPostgreSQLPasswordWithoutOverride(t *testing.T) {
	t.Setenv(EnvVarPostgreSQLPassword, "")
	settings, configPath := loadConfigFromYAML(t, postgresSaveFileConfig)
	require.Equal(t, "file-password", settings.Output.PostgreSQL.Password)
	require.NoError(t, SaveSettings())
	assert.Equal(t, "file-password", readPersistedSettings(t, configPath).Output.PostgreSQL.Password)
}

func TestSaveYAMLConfigDoesNotPersistPostgreSQLEnvironmentPasswordToNewFile(t *testing.T) {
	t.Setenv(EnvVarPostgreSQLPassword, "environment-password")
	settings := &Settings{}
	settings.Output.PostgreSQL.Password = "environment-password"
	path := filepath.Join(t.TempDir(), "new-config.yaml")
	require.NoError(t, SaveYAMLConfig(path, settings))
	assert.Empty(t, readPersistedSettings(t, path).Output.PostgreSQL.Password)
	assert.Equal(t, "environment-password", settings.Output.PostgreSQL.Password)
}
