package conf

import (
	"strconv"
	"strings"
)

// PostgreSQLSettings configures the v2 PostgreSQL database connection.
type PostgreSQLSettings struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Username    string `yaml:"username" json:"username"`
	Password    string `yaml:"password" json:"password"`
	Database    string `yaml:"database" json:"database"`
	Host        string `yaml:"host" json:"host"`
	Port        string `yaml:"port" json:"port"`
	SSLMode     string `yaml:"sslmode" json:"sslMode"`
	SSLRootCert string `yaml:"sslrootcert" json:"sslRootCert"`
	SSLCert     string `yaml:"sslcert" json:"sslCert"`
	SSLKey      string `yaml:"sslkey" json:"sslKey"`
}

const (
	postgresSSLDisable    = "disable"
	postgresSSLRequire    = "require"
	postgresSSLVerifyCA   = "verify-ca"
	postgresSSLVerifyFull = "verify-full"
	postgresMinPort       = 1
	postgresMaxPort       = 65535
)

// validatePostgreSQLSettings checks only an enabled PostgreSQL connection.
// Error messages name fields but never include credentials or connection values.
func validatePostgreSQLSettings(settings *Settings) error {
	pg := &settings.Output.PostgreSQL
	if !pg.Enabled {
		return nil
	}
	if settings.Output.SQLite.Enabled || settings.Output.MySQL.Enabled {
		return &ValidationError{Errors: []string{"output.postgresql.enabled requires output.sqlite.enabled and output.mysql.enabled to be false"}}
	}
	return pg.ValidateConnection()
}

// ValidateConnection checks the PostgreSQL connection fields without including
// configured values in errors. It also applies when a manager is opened directly.
func (pg *PostgreSQLSettings) ValidateConnection() error {
	if pg == nil {
		return &ValidationError{Errors: []string{"output.postgresql settings are required"}}
	}
	var problems []string
	for _, field := range []struct{ name, value string }{
		{"username", pg.Username}, {"database", pg.Database}, {"host", pg.Host}, {"port", pg.Port},
	} {
		if strings.TrimSpace(field.value) == "" {
			problems = append(problems, "output.postgresql."+field.name+" is required")
		}
	}
	if pg.Port != "" {
		port, err := strconv.Atoi(pg.Port)
		if err != nil || port < postgresMinPort || port > postgresMaxPort {
			problems = append(problems, "output.postgresql.port must be a valid TCP port")
		}
	}
	if pg.Host != "" && strings.ContainsAny(pg.Host, ",/?#@\\ \t\r\n") {
		problems = append(problems, "output.postgresql.host must be a single host name or IP address")
	}
	switch pg.SSLMode {
	case postgresSSLDisable, postgresSSLRequire, postgresSSLVerifyCA, postgresSSLVerifyFull:
	default:
		problems = append(problems, "output.postgresql.sslmode must be disable, require, verify-ca, or verify-full")
	}
	if (pg.SSLCert == "") != (pg.SSLKey == "") {
		problems = append(problems, "output.postgresql.sslcert and sslkey must be set together")
	}
	if len(problems) > 0 {
		return &ValidationError{Errors: problems}
	}
	return nil
}
