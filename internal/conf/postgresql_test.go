package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPostgreSQLSettings() *Settings {
	settings := &Settings{}
	settings.Output.PostgreSQL = PostgreSQLSettings{
		Enabled: true, Username: "birdnet", Database: "birdnet", Host: "localhost",
		Port: "5432", SSLMode: postgresSSLVerifyFull,
	}
	return settings
}

func TestValidatePostgreSQLSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*Settings)
		want string
	}{
		{name: "valid"},
		{name: "SQLite ambiguity", edit: func(s *Settings) { s.Output.SQLite.Enabled = true }, want: "output.sqlite.enabled"},
		{name: "MySQL ambiguity", edit: func(s *Settings) { s.Output.MySQL.Enabled = true }, want: "output.mysql.enabled"},
		{name: "missing host", edit: func(s *Settings) { s.Output.PostgreSQL.Host = "" }, want: "output.postgresql.host"},
		{name: "invalid port", edit: func(s *Settings) { s.Output.PostgreSQL.Port = "65536" }, want: "output.postgresql.port"},
		{name: "downgrade mode", edit: func(s *Settings) { s.Output.PostgreSQL.SSLMode = "prefer" }, want: "output.postgresql.sslmode"},
		{name: "client cert without key", edit: func(s *Settings) { s.Output.PostgreSQL.SSLCert = "client.pem" }, want: "output.postgresql.sslcert"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := validPostgreSQLSettings()
			if tc.edit != nil {
				tc.edit(s)
			}
			err := validatePostgreSQLSettings(s)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestValidatePostgreSQLSettingsDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()
	s := validPostgreSQLSettings()
	s.Output.PostgreSQL.Password = "private-password"
	s.Output.PostgreSQL.SSLKey = "private-key-path"
	err := validatePostgreSQLSettings(s)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-password")
	assert.NotContains(t, err.Error(), "private-key-path")
}
