# msync pre-commit hook for Windows (PowerShell version)
# This script checks if your local database is in sync with the target before allowing commits.

# Get the project root
try {
    $PROJECT_ROOT = git rev-parse --show-toplevel 2>$null
    if (-not $PROJECT_ROOT) {
        Write-Warning "Not in a git repository. Skipping msync hook."
        exit 0
    }
}
catch {
    Write-Warning "Not in a git repository. Skipping msync hook."
    exit 0
}

$CONFIG_FILE = Join-Path $PROJECT_ROOT ".msync.yml"

# Check if config exists
if (-not (Test-Path $CONFIG_FILE)) {
    Write-Host "[WARNING] msync: No configuration file found at .msync.yml"
    Write-Host "   Run 'msync init' to create one, or disable the hook in config."
    exit 0
}

# Default settings
$HOOK_ENABLED = $true
$EXCLUDE_BRANCHES = @("main", "master", "staging", "develop")
$TRIGGER_PATHS = @("migrations/", "models/", "schema/")

# Check if yq is available (optional)
try {
    $yqCheck = yq --version 2>$null
    if ($yqCheck) {
        # Check if hook is enabled
        $HOOK_ENABLED = (yq e '.hook.enabled // true' $CONFIG_FILE 2>$null) -eq "true"

        # Read exclude branches
        $excludeFromConfig = yq e '.hook.exclude_branches[]?' $CONFIG_FILE 2>$null
        foreach ($branch in $excludeFromConfig) {
            if ($branch) {
                $EXCLUDE_BRANCHES += $branch
            }
        }

        # Read trigger paths
        $TRIGGER_PATHS = @()
        $triggerFromConfig = yq e '.hook.trigger_paths[]?' $CONFIG_FILE 2>$null
        foreach ($path in $triggerFromConfig) {
            if ($path) {
                $TRIGGER_PATHS += $path
            }
        }

        # Add defaults if none configured
        if ($TRIGGER_PATHS.Count -eq 0) {
            $TRIGGER_PATHS = @("migrations/", "models/", "schema/")
        }
    }
}
catch {
    # yq not available, use defaults
}

# Check if hook is enabled
if (-not $HOOK_ENABLED) {
    exit 0
}

# Check for excluded branches
$CURRENT_BRANCH = git rev-parse --abbrev-ref HEAD 2>$null
foreach ($excluded in $EXCLUDE_BRANCHES) {
    if ($CURRENT_BRANCH -eq $excluded) {
        Write-Host "[SKIPPED] msync: Skipping on excluded branch '$CURRENT_BRANCH'"
        exit 0
    }
}

# Check if any staged files match trigger paths
$STAGED_FILES = git diff --cached --name-only 2>$null
$TRIGGERED = $false

foreach ($file in $STAGED_FILES) {
    foreach ($pattern in $TRIGGER_PATHS) {
        $cleanPattern = $pattern.TrimEnd('/')
        # Check if file starts with pattern path
        if ($file.StartsWith("$cleanPattern/") -or $file -like "*$pattern*") {
            $TRIGGERED = $true
            break
        }
    }
    if ($TRIGGERED) { break }
}

if (-not $TRIGGERED) {
    exit 0
}

Write-Host "[CHECK] msync: Checking database migration status..."

# Run msync verify
$TARGET_NAME = "production"
if ($args.Count -gt 0) {
    $TARGET_NAME = $args[0]
}

Set-Location $PROJECT_ROOT
msync verify --target $TARGET_NAME 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "[FAILED] msync: Verification failed!"
    Write-Host ""
    Write-Host "Your database is out of sync with the target environment."
    Write-Host "Please run 'msync up' to apply pending migrations before committing."
    Write-Host ""
    Write-Host "To skip this check (not recommended), use:"
    Write-Host "  git commit --no-verify"
    Write-Host ""
    exit 1
}

Write-Host "[OK] msync: Database is in sync"
exit 0
