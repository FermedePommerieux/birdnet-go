package dbserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPostgreSQLVersionSupported(t *testing.T) {
	tests := []struct {
		name    string
		version string
		number  int
		want    bool
	}{
		{"17 stable", "17.5", 170005, true},
		{"18 stable", "18.0", 180000, true},
		{"18 distro stable", "18.6 (Ubuntu 18.6-1.pgdg26.04+1)", 180006, true},
		{"distro stable", "17.6 (Debian 17.6-1.pgdg)", 170006, true},
		{"older", "16.9", 160009, false},
		{"newer", "19.0", 190000, false},
		{"beta", "18beta1", 180000, false},
		{"minor beta", "18.0beta1", 180000, false},
		{"rc", "18rc1", 180000, false},
		{"devel", "18devel", 180000, false},
		{"mismatched major", "17.5", 180005, false},
		{"mismatched minor", "17.5", 170006, false},
		{"malformed", "17.x", 170000, false},
		{"negative number", "17.5", -1, false},
		{"extra version part", "17.5.1", 170005, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := PostgreSQLReport{Version: tt.version, VersionNum: tt.number}
			assert.Equal(t, tt.want, report.VersionSupported())
		})
	}
}

func TestPostgreSQLEncodingSupported(t *testing.T) {
	assert.True(t, (PostgreSQLReport{ServerEncoding: "UTF8", ClientEncoding: "UTF8"}).EncodingSupported())
	assert.False(t, (PostgreSQLReport{ServerEncoding: "LATIN1", ClientEncoding: "UTF8"}).EncodingSupported())
	assert.False(t, (PostgreSQLReport{ServerEncoding: "UTF8", ClientEncoding: "LATIN1"}).EncodingSupported())
}
