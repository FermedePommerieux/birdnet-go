package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tphakala/birdnet-go/internal/conf"
	"github.com/tphakala/birdnet-go/internal/restart"
)

// TestPostgreSQLSettingsSaveRoundTrip covers both public settings write paths.
// It also pins the camel-case JSON names used by the settings UI.
func TestPostgreSQLSettingsSaveRoundTrip(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			restart.Reset()
			t.Cleanup(restart.Reset)
			e := echo.New()
			c := newMergeController(t, e, func(s *conf.Settings) {
				s.Output.SQLite.Enabled = true
				s.Output.PostgreSQL.Password = "saved-password"
			})
			body := `{"output":{"sqlite":{"enabled":false},"postgresql":{"enabled":true,"username":"birdnet","password":"********","database":"birds","host":"db.example.com","port":"5432","sslMode":"verify-full","sslRootCert":"root.pem","sslCert":"client.pem","sslKey":"client.key"}}}`
			path := "/api/v2/settings"
			if method == http.MethodPatch {
				path += "/output"
				body = `{"sqlite":{"enabled":false},"postgresql":{"enabled":true,"username":"birdnet","password":"********","database":"birds","host":"db.example.com","port":"5432","sslMode":"verify-full","sslRootCert":"root.pem","sslCert":"client.pem","sslKey":"client.key"}}`
			}
			body = strings.ReplaceAll(body, "********", redactedValue)
			req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)
			if method == http.MethodPatch {
				ctx.SetParamNames("section")
				ctx.SetParamValues("output")
				require.NoError(t, c.UpdateSectionSettings(ctx))
			} else {
				require.NoError(t, c.UpdateSettings(ctx))
			}
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			pg := c.Settings.Load().Output.PostgreSQL
			assert.Equal(t, "saved-password", pg.Password)
			assert.Equal(t, "verify-full", pg.SSLMode)
			assert.Equal(t, "root.pem", pg.SSLRootCert)
			assert.Equal(t, "client.pem", pg.SSLCert)
			assert.Equal(t, "client.key", pg.SSLKey)
			assert.False(t, c.Settings.Load().Output.SQLite.Enabled)
			var response struct {
				RestartRequired bool     `json:"restart_required"`
				RestartReasons  []string `json:"restart_reasons"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			assert.True(t, response.RestartRequired)
			assert.Contains(t, response.RestartReasons, reasonDatabaseRestart)
			assert.True(t, restart.IsRestartRequired())
		})
	}
}

func TestPostgreSQLSettingsRejectInvalidSwitchWithoutPublishing(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"two engines", `{"sqlite":{"enabled":true},"postgresql":{"enabled":true,"username":"birdnet","password":"sensitive-password","database":"birds","host":"localhost","port":"5432","sslMode":"disable"}}`},
		{"invalid TLS mode", `{"sqlite":{"enabled":false},"postgresql":{"enabled":true,"username":"birdnet","password":"sensitive-password","database":"birds","host":"localhost","port":"5432","sslMode":"prefer"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			c := newMergeController(t, e, func(s *conf.Settings) { s.Output.SQLite.Enabled = true })
			before := conf.CloneSettings(c.Settings.Load())
			req := httptest.NewRequest(http.MethodPatch, "/api/v2/settings/output", bytes.NewBufferString(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)
			ctx.SetParamNames("section")
			ctx.SetParamValues("output")
			require.NoError(t, c.UpdateSectionSettings(ctx))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, before.Output, c.Settings.Load().Output)
			assert.NotContains(t, rec.Body.String(), "sensitive-password")
		})
	}
}

func TestPostgreSQLConnectionChangesRequireRestart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*conf.PostgreSQLSettings)
	}{
		{"host", func(pg *conf.PostgreSQLSettings) { pg.Host = "other.example.com" }},
		{"database", func(pg *conf.PostgreSQLSettings) { pg.Database = "other" }},
		{"password", func(pg *conf.PostgreSQLSettings) { pg.Password = "changed" }},
		{"TLS mode", func(pg *conf.PostgreSQLSettings) { pg.SSLMode = "require" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := getTestSettings(t)
			before.Output.PostgreSQL = conf.PostgreSQLSettings{Enabled: true, Username: "birdnet", Password: "original", Database: "birds", Host: "db.example.com", Port: "5432", SSLMode: "verify-full"}
			after := conf.CloneSettings(before)
			tc.edit(&after.Output.PostgreSQL)
			assert.True(t, outputSettingsChanged(before, after))
		})
	}
}
