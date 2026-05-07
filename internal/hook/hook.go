package hook

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Melphins/msync/internal/config"
)

//go:embed template.sh
var bashTemplate string

//go:embed template.bat
var batchTemplate string

//go:embed template.ps1
var ps1Template string

// GetHookTemplate returns the appropriate hook script for the current OS
func GetHookTemplate() string {
	os := runtime.GOOS
	switch os {
	case "windows":
		// On Windows, prefer PowerShell if available, otherwise batch
		if _, err := exec.LookPath("powershell"); err == nil {
			return ps1Template
		}
		return batchTemplate
	default:
		// Unix-like systems (Linux, macOS, BSD, etc.)
		return bashTemplate
	}
}

// Manager handles pre-commit hook installation and removal
type Manager struct {
	cfg *config.Config
}

// NewManager creates a new hook manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

// Install installs the pre-commit hook to .git/hooks/pre-commit
func (m *Manager) Install() error {
	// Check if we're in a git repository
	if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Get git hooks directory
	gitDir, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return fmt.Errorf("failed to find git directory: %w", err)
	}
	gitDir = bytes.TrimSpace(gitDir)

	hooksDir := filepath.Join(string(gitDir), "hooks")
	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Create hooks directory if it doesn't exist
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Use embedded template based on OS
	hookScript := GetHookTemplate()

	// Make hook executable (no-op on Windows)
	if err := os.Chmod(hookPath, 0755); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to make hook executable: %w", err)
	}

	// Write the hook file
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return fmt.Errorf("failed to write hook file: %w", err)
	}

	return nil
}

// Uninstall removes the pre-commit hook
func (m *Manager) Uninstall() error {
	// Check if we're in a git repository
	if _, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Get git hooks directory
	gitDir, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return fmt.Errorf("failed to find git directory: %w", err)
	}
	gitDir = bytes.TrimSpace(gitDir)

	hookPath := filepath.Join(string(gitDir), "hooks", "pre-commit")

	// Check if hook exists
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		return fmt.Errorf("pre-commit hook is not installed")
	}

	// Check if it's our hook
	content, err := os.ReadFile(hookPath)
	if err != nil {
		return fmt.Errorf("failed to read hook file: %w", err)
	}

	if !strings.Contains(string(content), "msync") {
		return fmt.Errorf("pre-commit hook does not appear to be a msync hook")
	}

	// Remove the hook
	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	return nil
}

// IsInstalled checks if the pre-commit hook is installed
func (m *Manager) IsInstalled() (bool, error) {
	gitDir, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return false, fmt.Errorf("not in a git repository: %w", err)
	}
	gitDir = bytes.TrimSpace(gitDir)

	hookPath := filepath.Join(string(gitDir), "hooks", "pre-commit")

	// Check if hook exists
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to check hook: %w", err)
	}

	// Check if it's our hook by looking for msync marker
	content, err := os.ReadFile(hookPath)
	if err != nil {
		return false, fmt.Errorf("failed to read hook: %w", err)
	}

	return strings.Contains(string(content), "msync"), nil
}

// BuildHookScript generates the hook script from template with given config
func (m *Manager) BuildHookScript() (string, error) {
	script := GetHookTemplate()

	// Inject configuration values
	if m.cfg.Hook.Enabled {
		// Config values are read dynamically by the script via yq
		// No need to bake them into the script
	}

	return script, nil
}

// Status returns information about the hook installation
type Status struct {
	Installed bool
	Config    *config.HookConfig
}

// GetStatus returns the current hook status
func (m *Manager) GetStatus() (*Status, error) {
	installed, err := m.IsInstalled()
	if err != nil {
		return nil, err
	}
	return &Status{
		Installed: installed,
		Config:    &m.cfg.Hook,
	}, nil
}
