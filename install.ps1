# msync Windows Installer
# PowerShell script for installing msync on Windows

[CmdletBinding()]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\msync",
    [switch]$BuildOnly,
    [string]$Arch = "amd64",
    [string]$Os = "windows",
    [switch]$Help
)

function Show-Usage {
    Write-Host "msync Installer - Install msync on Windows" -ForegroundColor Green
    Write-Host ""
    Write-Host "Usage: $($MyInvocation.MyCommand.Name) [OPTIONS]" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Options:" -ForegroundColor Blue
    Write-Host "  -Dir DIR          Install directory (default: `$env:LOCALAPPDATA\msync)"
    Write-Host "  -BuildOnly        Build only, don't install"
    Write-Host "  -Arch ARCH        Architecture (amd64, 386, arm64) - default: amd64"
    Write-Host "  -Os OS            Target OS - default: windows"
    Write-Host "  -Help             Show this help message"
    Write-Host ""
    Write-Host "Examples:" -ForegroundColor Blue
    Write-Host "  $($MyInvocation.MyCommand.Name)                    # Build and install"
    Write-Host "  $($MyInvocation.MyCommand.Name) -Dir C:\tools     # Install to custom dir"
    Write-Host "  $($MyInvocation.MyCommand.Name) -BuildOnly       # Build only"
    Write-Host ""
}

function Write-Log {
    param([string]$Message, [string]$Level = "INFO")
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logEntry = "[$timestamp] [$Level] $Message"
    Write-Host $logEntry
    Add-Content -Path $global:LogFile -Value $logEntry
}

function Write-Error-Log {
    param([string]$Message)
    Write-Log -Message $Message -Level "ERROR"
}

function Write-Success-Log {
    param([string]$Message)
    Write-Log -Message $Message -Level "SUCCESS"
}

function Write-Warning-Log {
    param([string]$Message)
    Write-Log -Message $Message -Level "WARNING"
}

function Check-Environment {
    Write-Log "Checking environment..."

    # Check if we're in the project root
    if (-not (Test-Path "go.mod")) {
        Write-Error-Log "go.mod not found. Please run this script from the msync project root."
        return $false
    }

    # Check Go installation
    try {
        $goVersionOutput = go version 2>$null
        if (-not $goVersionOutput) {
            Write-Error-Log "Go is not installed or not in PATH."
            Write-Log "Download Go from: https://go.dev/dl/"
            return $false
        }

        # Extract version number
        if ($goVersionOutput -match 'go(\d+\.\d+\.\d+)') {
            $goVersion = $matches[1]
            Write-Log "Go $goVersion detected"

            # Check minimum version (1.21+)
            $minVersion = [System.Version]"1.21.0"
            $currentVersion = [System.Version]$goVersion
            if ($currentVersion -lt $minVersion) {
                Write-Error-Log "Go version $goVersion is too old. Please upgrade to Go 1.21 or later."
                return $false
            }
        }
    }
    catch {
        Write-Error-Log "Failed to check Go installation: $_"
        return $false
    }

    Write-Success-Log "Environment check passed"
    return $true
}

function Build-Binary {
    param([string]$Arch, [string]$TargetOs, [string]$OutputName)

    Write-Log "Building msync for $TargetOs/$Arch..."

    # Clean previous builds
    Write-Log "Cleaning previous builds..."
    go clean -cache 2>&1 | Out-Null
    Remove-Item -ErrorAction SilentlyContinue -Path "msync.exe", "msync-*"

    # Download dependencies
    Write-Log "Downloading dependencies..."
    go mod download 2>&1 | ForEach-Object { Write-Log $_ }
    go mod tidy 2>&1 | ForEach-Object { Write-Log $_ }

    # Build command
    Write-Log "Building with GOARCH=$Arch GOOS=$TargetOs..."
    $env:GOARCH = $Arch
    $env:GOOS = $TargetOs

    $buildCmd = "go build -ldflags '-s -w' -o $OutputName ./cmd/msync"
    Write-Log "Executing: $buildCmd"

    try {
        Invoke-Expression $buildCmd | ForEach-Object { Write-Log $_ }
        if ($LASTEXITCODE -eq 0) {
            Write-Success-Log "Build completed successfully"
            return $true
        }
        else {
            Write-Error-Log "Build failed with exit code $LASTEXITCODE"
            return $false
        }
    }
    catch {
        Write-Error-Log "Build failed: $_"
        return $false
    }
}

function Install-Binary {
    param([string]$Source, [string]$Destination)

    Write-Log "Installing to $Destination..."

    # Create destination directory if needed
    $destDir = Split-Path $Destination -Parent
    if (-not (Test-Path $destDir)) {
        Write-Log "Creating directory: $destDir"
        try {
            New-Item -ItemType Directory -Path $destDir -Force | Out-Null
        }
        catch {
            Write-Error-Log "Failed to create directory: $_"
            return $false
        }
    }

    # Copy binary
    try {
        Copy-Item -Path $Source -Destination $Destination -Force
        Set-ItemProperty -Path $Destination -Name IsReadOnly -Value $false
        Write-Success-Log "Installed msync to $Destination"
        return $true
    }
    catch {
        Write-Error-Log "Failed to install binary: $_"
        Write-Log "You may need to run this script as Administrator or choose a different directory."
        return $false
    }
}

function Add-To-Path {
    param([string]$InstallDir)

    Write-Log "Adding $InstallDir to user PATH..."

    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")

    # Check if already in PATH
    if ($currentPath -like "*$InstallDir*") {
        Write-Success-Log "$InstallDir is already in your PATH"
        return $true
    }

    # Add to user PATH
    try {
        $newPath = "$currentPath;$InstallDir"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Success-Log "Added $InstallDir to your user PATH"
        Write-Log "You may need to restart your terminal or PowerShell session to use msync"
        return $true
    }
    catch {
        Write-Error-Log "Failed to add to PATH: $_"
        return $false
    }
}

function Show-Path-Instructions {
    param([string]$InstallDir)

    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "║                   Installation Complete!                     ║" -ForegroundColor Green
    Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Host "Installation complete!" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "msync has been installed to: $InstallDir" -ForegroundColor Cyan
    Write-Host "It has been added to your user PATH." -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Next steps:" -ForegroundColor Yellow
    Write-Host "  1. Initialize msync in your project:" -ForegroundColor Cyan
    Write-Host "     cd C:\path\to\your\project" -ForegroundColor Green
    Write-Host "     msync init" -ForegroundColor Green
    Write-Host ""
    Write-Host "  2. Verify installation:" -ForegroundColor Cyan
    Write-Host "     msync --version" -ForegroundColor Green
    Write-Host ""
    Write-Host "Note: You may need to restart your PowerShell session or terminal" -ForegroundColor Gray
    Write-Host "      if msync is not immediately available in your PATH." -ForegroundColor Gray
    Write-Host ""
}

# --- Main Script ---

# Initialize log file
$global:LogFile = Join-Path $env:TEMP "msync-install-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"
Write-Log "msync Installation Log"
Write-Log "Started: $(Get-Date)"
Write-Log "Script: $($MyInvocation.MyCommand.Name)"
Write-Log "Install dir: $InstallDir"
Write-Log ""

if ($Help) {
    Show-Usage
    exit 0
}

Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║                   msync Installer v0.0.1                     ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""

# Check if running as Administrator (needed for system installs)
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if ($InstallDir -like "C:\Program Files*" -or $InstallDir -like "C:\Windows*") {
    if (-not $isAdmin) {
        Write-Warning-Log "Installing to $InstallDir requires Administrator privileges."
        Write-Log "Please run this script as Administrator or choose a different directory."
        Write-Log "Alternative: $env:LOCALAPPDATA\msync or $env:APPDATA\msync"
        exit 1
    }
}

# Check environment
if (-not (Check-Environment)) {
    Write-Error-Log "Environment check failed. See $LogFile for details."
    exit 1
}

# Determine output binary name
$outputName = "msync.exe"
if ($Arch -ne "amd64" -or $Os -ne "windows") {
    $outputName = "msync-$Os-$Arch.exe"
}

# Build the binary
if (-not (Build-Binary -Arch $Arch -TargetOs $Os -OutputName $outputName)) {
    exit 1
}

# Install unless build-only
if (-not $BuildOnly) {
    $destPath = Join-Path $InstallDir "msync.exe"
    if (-not (Install-Binary -Source $outputName -Destination $destPath)) {
        Write-Error-Log "Installation failed. See $LogFile for details."
        exit 1
    }

    # Add to PATH automatically
    Add-To-Path -InstallDir $InstallDir

    # Show success message
    Show-Path-Instructions -InstallDir $InstallDir
}
else {
    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "║                     Build Complete!                          ║" -ForegroundColor Green
    Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Host "Binary created: $outputName" -ForegroundColor Yellow
    Write-Host "Run it with: .\$outputName --version" -ForegroundColor Yellow
    Write-Host ""
}

Write-Log "Completed: $(Get-Date)"
Write-Host "Log file: $LogFile" -ForegroundColor Gray
Write-Host ""
