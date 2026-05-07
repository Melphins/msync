#!/usr/bin/env bash
# msync pre-commit hook
# This script checks if your local database is in sync with the target before allowing commits.

set -e

# Get the project root (assumes hook is in .git/hooks/)
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
CONFIG_FILE="${PROJECT_ROOT}/.msync.yml"

# Check if config exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "[WARNING] msync: No configuration file found at .msync.yml"
    echo "   Run 'msync init' to create one, or disable the hook in config."
    exit 0
fi

# Default settings (can be overridden in config)
HOOK_ENABLED=true
EXCLUDE_BRANCHES=("main" "master" "staging" "develop")
TRIGGER_PATHS=("migrations/" "models/" "schema/")

# Read config if yq is available (optional)
if command -v yq &> /dev/null; then
    # Check if hook is enabled
    HOOK_ENABLED=$(yq e '.hook.enabled // true' "$CONFIG_FILE" 2>/dev/null || echo "true")

    # Read exclude branches
    while IFS= read -r branch; do
        [ -n "$branch" ] && EXCLUDE_BRANCHES+=("$branch")
    done < <(yq e '.hook.exclude_branches[]?' "$CONFIG_FILE" 2>/dev/null || true)

    # Read trigger paths
    TRIGGER_PATHS=()
    while IFS= read -r path; do
        [ -n "$path" ] && TRIGGER_PATHS+=("$path")
    done < <(yq e '.hook.trigger_paths[]?' "$CONFIG_FILE" 2>/dev/null || true)
    # Add defaults if none configured
    if [ ${#TRIGGER_PATHS[@]} -eq 0 ]; then
        TRIGGER_PATHS=("migrations/" "models/" "schema/")
    fi
fi

# Check if hook is enabled
if [ "$HOOK_ENABLED" != "true" ]; then
    exit 0
fi

# Check for excluded branches
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

for excluded in "${EXCLUDE_BRANCHES[@]}"; do
    if [ "$CURRENT_BRANCH" = "$excluded" ]; then
        echo "[SKIPPED] msync: Skipping on excluded branch '$CURRENT_BRANCH'"
        exit 0
    fi
done

# Check if any staged files match trigger paths
STAGED_FILES=$(git diff --cached --name-only)

TRIGGERED=false
for file in $STAGED_FILES; do
    for pattern in "${TRIGGER_PATHS[@]}"; do
        # Remove trailing slash for prefix matching
        pat="${pattern%/}"
        if [[ "$file" == "$pat/"* ]] || [[ "$file" == *"$pat"* ]]; then
            TRIGGERED=true
            break 2
        fi
    done
done

if [ "$TRIGGERED" = false ]; then
    exit 0
fi

echo "[CHECK] msync: Checking database migration status..."

# Run msync verify
cd "$PROJECT_ROOT"
TARGET_NAME="production"
if [ $# -ge 1 ]; then
    TARGET_NAME="$1"
fi

if ! msync verify --target "$TARGET_NAME" 2>/dev/null; then
    echo ""
    echo "[FAILED] msync: Verification failed!"
    echo ""
    echo "Your database is out of sync with the target environment."
    echo "Please run 'msync up' to apply pending migrations before committing."
    echo ""
    echo "To skip this check (not recommended), use:"
    echo "  git commit --no-verify"
    echo ""
    exit 1
fi

echo "[OK] msync: Database is in sync"
exit 0
