# Contributing to msync

Thank you for your interest in contributing! This document provides guidelines and information for contributors.

---

## Ways to Contribute

### 1. Add a New Migration Adapter

We need adapters for more migration frameworks! See [adapter-priorities.md](../adapter-priorities.md) for the priority list.

Steps:
1. Read [docs/adapter-development.md](../docs/adapter-development.md)
2. Pick an unimplemented adapter from the priority list
3. Create a new file in `internal/adapters/<name>/`
4. Implement the `Adapter` interface
5. Add tests in `internal/adapters/<name>/test_<name>.go`
6. Submit a PR with your adapter

### 2. Report Bugs

If you find a bug, please open an issue with:

- Version of msync
- Your migration framework (Alembic, Django, etc.)
- Database type and version
- What you expected to happen
- What actually happened (include error messages)
- Steps to reproduce

### 3. Improve Documentation

Found a typo? Confusing section? Missing info?

- Edit the relevant `.md` file in the `docs/` folder
- Submit a PR with your changes

### 4. Share Your Experience

- Write a blog post about how msync helped your team
- Share on social media with #msync
- Leave a review on Product Hunt (when launched)
- Tell your friends/colleagues!

---

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker & Docker Compose (for testing)
- Make (optional, for using Makefile targets)

### Clone and Build

```bash
git clone https://github.com/yourusername/msync
cd msync
go mod download
go build -o msync ./cmd/msync
```

### Run Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Verbose output
go test -v ./...

# Specific package
go test ./internal/adapters/alembic
```

### Test with Docker Databases

```bash
# Start test databases
docker-compose -f testdata/docker-compose.yml up -d

# Run integration tests
go test -tags=integration ./...

# Stop databases
docker-compose -f testdata/docker-compose.yml down
```

---

## Code Style

### Go

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` or `go fmt` before committing
- Error messages should be actionable: `fmt.Errorf("migration table %s not found", tableName)`
- Context cancellation must be respected

### CLI Design

- Commands should be discoverable (`msync --help`)
- Error output goes to stderr
- Machine-readable output uses `--format json`
- Human output is colored and formatted

### Git Commits

```
type(scope): description

body (optional)

footer (BREAKING CHANGE, closes #123)
```

Types: `feat`, `fix`, `docs`, `test`, `chore`, `refactor`

Examples:
```
feat(adapters): add Alembic adapter support
fix(status): handle nil connection gracefully
docs(readme): add troubleshooting section
test(prisma): add integration test with checksums
```

---

## Pull Request Process

1. **Create an issue first** (unless it's a trivial fix)
   - Describe the problem or feature
   - Get feedback from maintainers

2. **Fork and create a branch**
   ```bash
   git checkout -b feat/my-feature
   ```

3. **Make your changes**
   - Write tests for new functionality
   - Update documentation if needed
   - Follow code style guidelines

4. **Run tests**
   ```bash
   go test ./...
   go vet ./...
   ```

5. **Submit PR**
   - Fill out PR template completely
   - Link related issues
   - Screenshots for UI changes
   - Include tests

6. **Code review**
   - Address review comments
   - Keep PRs focused (one feature per PR)
   - Rebase if requested

7. **Merge**
   - Maintainer will merge after approval
   - Squash commits if requested

---

## Adding a New Adapter: Quick Guide

See [docs/adapter-development.md](../docs/adapter-development.md) for full details.

```go
// internal/adapters/myframework/myframework.go
package myframework

import "context"

type MyFrameworkAdapter struct {
    db         *sql.DB
    tableName  string
    migrationDir string
}

func (a *MyFrameworkAdapter) CurrentVersion(ctx context.Context) (string, error) {
    // Query your migration tracking table
    var version string
    err := a.db.QueryRowContext(ctx,
        "SELECT version FROM migrations ORDER BY id DESC LIMIT 1").Scan(&version)
    return version, err
}

func (a *MyFrameworkAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
    // Return all applied versions
}

func (a *MyFrameworkAdapter) ParseMigrationFile(path string) (*migrations.Migration, error) {
    // Parse your migration file format
}
```

---

## Testing Requirements

- **New adapter**: ≥80% unit test coverage + 1 integration test
- **New feature**: Unit tests for logic + integration test if applicable
- **Bug fix**: Test that reproduces the bug before fixing

### Test Fixtures

Place test fixtures in `testdata/`:

```
testdata/
├── alembic/
│   ├── versions/
│   │   ├── 0042_add_user.py
│   │   └── 0043_add_email.py
│   └── alembic_version_table.sql
├── rails/
│   └── db/migrate/
├── postgres-init.sql
└── docker-compose.yml
```

---

## Need Help?

- **Questions**: Open a [discussion](https://github.com/yourusername/msync/discussions)
- **Bugs**: Open an [issue](https://github.com/yourusername/msync/issues)
- **Chat**: Join our Discord (coming soon)

---

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](../CODE_OF_CONDUCT.md).

By participating, you are expected to uphold this code.

---

## License

By contributing, you agree that your contributions will be licensed under the MIT License - see [LICENSE](../LICENSE).

---

**Happy contributing!** 🎉
