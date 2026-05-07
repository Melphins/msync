package hook

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Melphins/msync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Install(t *testing.T) {
	// Skip if not in a git repo
	if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err != nil {
		t.Skip("Not in a git repository")
	}

	// Create test config
	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      "postgres",
			Connection:   "postgres://localhost/test",
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{
			"production": {
				Adapter:    "postgres",
				Connection: "postgres://localhost/prod",
				MigrationDir: "./migrations",
			},
		},
		Hook: config.HookConfig{
			Enabled:       true,
			ExcludeBranches: []string{"main", "master"},
			TriggerPaths:   []string{"migrations/"},
		},
	}

	// Temporarily change to project root for test
	origDir, _ := os.Getwd()
	projectRoot, _ := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	projectRoot = bytesTrimSpace(projectRoot)
	os.Chdir(string(projectRoot))

	// Ensure hook doesn't already exist
	gitDir, _ := exec.Command("git", "rev-parse", "--git-dir").Output()
	gitDir = bytesTrimSpace(gitDir)
	hookPath := filepath.Join(string(gitDir), "hooks", "pre-commit")
	os.Remove(string(hookPath))

	mgr := NewManager(cfg)
	err := mgr.Install()
	require.NoError(t, err)

	// Check hook was created
	_, err = os.Stat(hookPath)
	assert.NoError(t, err, "hook file should exist")

	// Check hook is executable
	info, err := os.Stat(hookPath)
	assert.NoError(t, err)
	assert.True(t, info.Mode()&0111 != 0, "hook should be executable")

	// Check content
	content, err := os.ReadFile(hookPath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "msync")
	assert.Contains(t, string(content), "pre-commit")

	os.Chdir(origDir)
}

func TestManager_Uninstall(t *testing.T) {
	// Skip if not in a git repo
	if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err != nil {
		t.Skip("Not in a git repository")
	}

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      "postgres",
			Connection:   "postgres://localhost/test",
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{
			"production": {
				Adapter:    "postgres",
				Connection: "postgres://localhost/prod",
				MigrationDir: "./migrations",
			},
		},
	}

	origDir, _ := os.Getwd()
	projectRoot, _ := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	projectRoot = bytesTrimSpace(projectRoot)
	os.Chdir(string(projectRoot))

	// Ensure hook exists first
	gitDir, _ := exec.Command("git", "rev-parse", "--git-dir").Output()
	gitDir = bytesTrimSpace(gitDir)
	hookPath := filepath.Join(string(gitDir), "hooks", "pre-commit")

	// Create dummy hook
	os.WriteFile(string(hookPath), []byte("#!/bin/bash\necho 'msync hook'"), 0755)

	mgr := NewManager(cfg)
	err := mgr.Uninstall()
	assert.NoError(t, err)

	// Check hook is removed
	_, err = os.Stat(hookPath)
	assert.True(t, os.IsNotExist(err), "hook should be removed")

	os.Chdir(origDir)
}

func TestManager_IsInstalled(t *testing.T) {
	// Skip if not in a git repo
	if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err != nil {
		t.Skip("Not in a git repository")
	}

	cfg := &config.Config{
		Local: config.LocalConfig{
			Adapter:      "postgres",
			Connection:   "postgres://localhost/test",
			MigrationDir: "./migrations",
		},
		Targets: map[string]config.TargetConfig{
			"production": {
				Adapter:    "postgres",
				Connection: "postgres://localhost/prod",
				MigrationDir: "./migrations",
			},
		},
	}

	origDir, _ := os.Getwd()
	projectRoot, _ := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	projectRoot = bytesTrimSpace(projectRoot)
	os.Chdir(string(projectRoot))

	gitDir, _ := exec.Command("git", "rev-parse", "--git-dir").Output()
	gitDir = bytesTrimSpace(gitDir)
	hookPath := filepath.Join(string(gitDir), "hooks", "pre-commit")

	// Ensure hook doesn't exist
	os.Remove(string(hookPath))

	mgr := NewManager(cfg)
	installed, err := mgr.IsInstalled()
	assert.NoError(t, err)
	assert.False(t, installed)

	// Create our hook
	os.WriteFile(string(hookPath), []byte("#!/bin/bash\n# msync pre-commit hook"), 0755)

	installed, err = mgr.IsInstalled()
	assert.NoError(t, err)
	assert.True(t, installed)

	// Create non-msync hook
	os.WriteFile(string(hookPath), []byte("#!/bin/bash\necho 'some other hook'"), 0755)

	installed, err = mgr.IsInstalled()
	assert.NoError(t, err)
	assert.False(t, installed)

	os.Remove(string(hookPath))
	os.Chdir(origDir)
}

// bytesTrimSpace trims whitespace from a byte slice
func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
