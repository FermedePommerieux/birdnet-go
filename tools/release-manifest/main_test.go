package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOmitsDisabledDockerHub(t *testing.T) {
	t.Parallel()
	const archive = "birdnet-go-linux-arm64-20261108.tar.gz"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksums.txt" {
			_, err := fmt.Fprintf(w, "%s  %s\n", strings.Repeat("a", 64), archive)
			assert.NoError(t, err)
			return
		}
		assert.NoError(t, json.NewEncoder(w).Encode([]map[string]any{{
			"tag_name": "20261108", "published_at": "2026-11-08T12:00:00Z",
			"assets": []map[string]any{
				{"name": archive, "browser_download_url": "https://example.invalid/" + archive},
				{"name": "checksums.txt", "browser_download_url": "http://" + r.Host + "/checksums.txt"},
			},
		}}))
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "manifest.json")
	require.NoError(t, run("example/fork", output, server.URL, "", "-", 0))
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "dockerhub")
	assert.Contains(t, string(data), "ghcr.io/example/fork:20261108")
	assert.Contains(t, string(data), "ghcr.io/example/fork:latest")
}
