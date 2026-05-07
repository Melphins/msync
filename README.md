# msync: Database Migration Synchronization Tool

A lightweight CLI tool that detects out-of-sync databases and prevents commits when developers forget to run migrations. msync ensures that local development databases stay synchronized with reference environments, eliminating "it works on my machine but not work on server :)" issues.

## Problem

Backend teams waste 2-3 hours per week on database synchronization issues:

- New columns exist in code but not in local database
- New hires spend hours debugging missing tables
- CI failures due to schema drift
- Onboarding takes days instead of hours

## Solution

**msync** automates database migration synchronization by:

1. Detecting your current migration version
2. Comparing against production or team reference databases
3. Blocking commits when out of sync
4. Providing clear commands to resolve drift

## Features

| Feature | 
|---------|
| CLI status check |
| Schema diff visualization | 
| Migration apply (`up`) | 
| Pre-commit hook | 
| CI verification mode | 
| Multiple adapter support | 

## Supported Frameworks

**msync** supports major migration frameworks across multiple languages:

| Framework | Language |
|-----------|----------|
| Alembic (SQLAlchemy) | Python | 
| Rails ActiveRecord | Ruby | 
| Django Migrations | Python | 
| Prisma Migrate | TypeScript | 


## Installation

### Linux / macOS / WSL

```bash
git clone https://github.com/Melphins/msync.git
cd msync
./install.sh
```

### Windows

**PowerShell**

```powershell
git clone https://github.com/Melphins/msync.git
cd msync
.\install.ps1
```


## Quick Start

```bash
# In your project directory, run the init wizard
msync init

# Follow the interactive prompts to configure your adapters and targets

# Check sync status
msync status --target production

# Apply pending migrations
msync up --target production

# See what would change
msync diff --target production

# CI/CD verification (exits with code based on status)
msync verify --target production
```

## Configuration

The `msync init` command provides an interactive wizard to generate your `.msync.yml` configuration file. The configuration defines:

- Local database adapter and connection
- Target environment(s) to sync against
- Thresholds for warnings and errors
- Pre-commit hook settings

## Pre-commit Hook

Install the pre-commit hook to automatically verify database sync before each commit:

```bash
msync install-hook
```

The hook will check your database against the target and prevent commits when out of sync. Configure hook behavior in `.msync.yml`. Use `git commit --no-verify` to bypass the check when necessary.

## Platform Support

**msync** is available on all major platforms:

| Platform | 
|----------|
| Linux  | 
| macOS  | 
| Windows |
| WSL | 


## Technical Details

**msync** follows a clean architecture with adapter pattern implementation:

- **Adapter Registry**: Factory-based adapter loading for extensibility
- **Configuration System**: YAML-based with environment variable substitution
- **Transaction Safety**: Each migration runs in its own transaction
- **Dry-run Support**: Preview changes before applying
- **CI/CD Ready**: Exit codes and JSON output for automation

## License

MIT License - see [LICENSE](LICENSE) file for details.
