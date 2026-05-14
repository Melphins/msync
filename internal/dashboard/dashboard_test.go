package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Melphins/msync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_Index(t *testing.T) {
	handler := NewServer(testConfig()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rec.Body.String(), "msync dashboard")
}

func TestServer_Config(t *testing.T) {
	handler := NewServer(testConfig()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.NotContains(t, rec.Body.String(), "postgres://secret")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload["targets"], 2)

	targets := payload["targets"].([]any)
	first := targets[0].(map[string]any)
	second := targets[1].(map[string]any)
	assert.Equal(t, "alpha", first["name"])
	assert.Equal(t, "zeta", second["name"])
}

func TestServer_StatusUnknownTarget(t *testing.T) {
	handler := NewServer(testConfig()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/status?target=missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "target 'missing' not found")
}

func testConfig() *config.Config {
	return &config.Config{
		Project: config.ProjectConfig{Name: "dashboard-test"},
		Local: config.LocalConfig{
			Adapter:        "postgres",
			Connection:     "postgres://secret-local",
			MigrationDir:   "./migrations",
			MigrationTable: "schema_migrations",
		},
		Targets: map[string]config.TargetConfig{
			"zeta": {
				Type:         config.TargetTypeMigrationDir,
				Adapter:      "postgres",
				Connection:   "postgres://secret-zeta",
				MigrationDir: "./migrations/zeta",
			},
			"alpha": {
				Type:         config.TargetTypeMigrationDir,
				Adapter:      "postgres",
				Connection:   "postgres://secret-alpha",
				MigrationDir: "./migrations/alpha",
			},
		},
	}
}
