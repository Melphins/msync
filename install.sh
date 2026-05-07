#!/bin/bash
# msync installation script
# ==========================
# For Linux, macOS, and other Unix-like systems (including WSL)
#
# Windows users:
#   - PowerShell:      use install.ps1
#   - Command Prompt: use build.bat

set -euo pipefail
IFS=$'\n\t'

# Detect if running on Windows (not WSL)
if [[ "$(uname -s 2>/dev/null || echo unknown)" == *"CYGWIN"* ]] || \
   [[ "$(uname -s 2>/dev/null)" == *"MINGW"* ]] || \
   [[ "$(uname -s 2>/dev/null)" == *"MSYS"* ]]; then
    echo -e "\033[1;33m[WARNING]\033[0m This script is designed for Unix-like systems."
    echo "For native Windows, please use one of these instead:"
    echo "  - install.ps1 (PowerShell, recommended)"
    echo "  - build.bat (Command Prompt)"
    echo ""
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Exiting. Please use the Windows installation script."
        exit 1
    fi
    echo ""
fi

# --- 2. Environment Variable Definitions ---
readonly SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$SCRIPT_DIR"

# Colors for output
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly RED='\033[0;31m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Configuration
readonly DEFAULT_INSTALL_DIR="${HOME}/.local/bin"
readonly LOG_FILE="/tmp/msync-install-$(date +%Y%m%d-%H%M%S).log"
readonly BINARY_NAME="msync"
readonly VERSION="0.0.1"

# --- 3. Environment Check Function ---
check_environment() {
    local missing_deps=()

    echo -e "${BLUE}[*] Checking environment...${NC}" | tee -a "$LOG_FILE"

    # Check if we're in the project root
    if [[ ! -f "$PROJECT_ROOT/go.mod" ]]; then
        log_error "go.mod not found. Please run this script from the msync project root."
        return 1
    fi

    # Check Go installation
    if ! command -v go &> /dev/null; then
        missing_deps+=("Go (https://go.dev/dl/)")
    fi

    # Check Go version (require 1.21+)
    if command -v go &> /dev/null; then
        local go_version
        go_version=$(go version | awk '{print $3}' | sed 's/go//')
        local required_version="1.21"
        if ! printf '%s\n%s\n' "$required_version" "$go_version" | sort -V -C; then
            log_error "Go version $go_version is too old. Please upgrade to Go 1.21 or later."
            return 1
        fi
        echo -e "${GREEN}[OK] Go $go_version detected${NC}" | tee -a "$LOG_FILE"
    fi

    # Check for missing dependencies
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        echo -e "${RED}[!] Missing dependencies:${NC}" | tee -a "$LOG_FILE"
        for dep in "${missing_deps[@]}"; do
            echo -e "  - $dep" | tee -a "$LOG_FILE"
        done
        return 1
    fi

    echo -e "${GREEN}[OK] Environment check passed${NC}" | tee -a "$LOG_FILE"
    return 0
}

# --- Logging Functions ---
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" | tee -a "$LOG_FILE"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*" | tee -a "$LOG_FILE"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" | tee -a "$LOG_FILE" >&2
}

# --- Architecture Detection ---
detect_architecture() {
    local machine
    machine="$(uname -m)"

    case "$machine" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l|armv6l)
            echo "arm"
            ;;
        i386|i686)
            echo "386"
            ;;
        *)
            log_warning "Unknown architecture: $machine. Defaulting to amd64."
            echo "amd64"
            ;;
    esac
}

# --- Build Function ---
build_binary() {
    local build_arch="$1"
    local build_os="$2"
    local output_name="$3"

    log_info "Building msync for ${build_os}/${build_arch}..."

    # Clean previous builds
    go clean -cache 2>&1 | tee -a "$LOG_FILE" || true
    rm -f "$BINARY_NAME" "msync-*" 2>/dev/null || true

    # Download dependencies
    log_info "Downloading dependencies..."
    go mod download 2>&1 | tee -a "$LOG_FILE"
    go mod tidy 2>&1 | tee -a "$LOG_FILE" || true

    # Build command
    local build_cmd="go build -ldflags '-s -w' -o $output_name ./cmd/msync"

    if [[ -n "$build_os" ]] || [[ -n "$build_arch" ]]; then
        export GOOS="${build_os:-$(go env GOOS)}"
        export GOARCH="${build_arch:-$(go env GOARCH)}"
        build_cmd="GOOS=$GOOS GOARCH=$GOARCH $build_cmd"
    fi

    log_info "Executing: $build_cmd"
    if eval "$build_cmd" 2>&1 | tee -a "$LOG_FILE"; then
        log_success "Build completed successfully"
        return 0
    else
        log_error "Build failed. Check $LOG_FILE for details."
        return 1
    fi
}

# --- Installation Function ---
install_binary() {
    local source="$1"
    local destination="$2"
    local use_sudo="$3"

    log_info "Installing to $destination..."

    # Create destination directory if needed
    local dest_dir
    dest_dir="$(dirname "$destination")"

    if [[ ! -d "$dest_dir" ]]; then
        log_info "Creating directory: $dest_dir"
        if [[ -w "$(dirname "$dest_dir")" ]] || [[ "$use_sudo" == "true" ]]; then
            if [[ "$use_sudo" == "true" ]]; then
                sudo mkdir -p "$dest_dir" 2>&1 | tee -a "$LOG_FILE"
            else
                mkdir -p "$dest_dir" 2>&1 | tee -a "$LOG_FILE"
            fi
        else
            log_error "Cannot create directory $dest_dir (permission denied)"
            return 1
        fi
    fi

    # Copy binary
    if [[ -w "$dest_dir" ]] || [[ "$use_sudo" == "true" ]]; then
        if [[ "$use_sudo" == "true" ]]; then
            sudo cp "$source" "$destination" 2>&1 | tee -a "$LOG_FILE"
            sudo chmod 755 "$destination" 2>&1 | tee -a "$LOG_FILE"
        else
            cp "$source" "$destination" 2>&1 | tee -a "$LOG_FILE"
            chmod 755 "$destination" 2>&1 | tee -a "$LOG_FILE"
        fi
    else
        log_error "Cannot write to $dest_dir (permission denied)"
        return 1
    fi

    log_success "Installed msync to $destination"
    return 0
}

# --- PATH Configuration ---
check_path() {
    local install_dir="$1"

    if [[ ":$PATH:" == *":$install_dir:"* ]]; then
        log_success "$install_dir is already in your PATH"
        return 0
    else
        log_warning "$install_dir is NOT in your PATH"
        return 1
    fi
}

add_to_shell_config() {
    local install_dir="$1"
    local shell_config=""

    # Detect shell and determine config file
    local shell_name="$(basename "${SHELL:-/bin/bash}")"

    case "$shell_name" in
        bash)
            shell_config="$HOME/.bashrc"
            ;;
        zsh)
            shell_config="$HOME/.zshrc"
            ;;
        fish)
            shell_config="$HOME/.config/fish/config.fish"
            ;;
        *)
            # Default to bashrc for unknown shells
            shell_config="$HOME/.bashrc"
            ;;
    esac

    # Check if already in config
    if [[ -f "$shell_config" ]] && grep -q "$install_dir" "$shell_config"; then
        log_info "$install_dir already found in $shell_config"
        return 0
    fi

    # Add to config file
    log_info "Adding $install_dir to $shell_config"
    mkdir -p "$(dirname "$shell_config")"

    case "$shell_name" in
        fish)
            echo "set -gx PATH \$PATH $install_dir" >> "$shell_config"
            ;;
        *)
            echo "export PATH=\"\$PATH:$install_dir\"" >> "$shell_config"
            ;;
    esac

    log_success "Added $install_dir to $shell_config"
    log_info "Restart your terminal or run 'source $shell_config' to activate"

    return 0
}

print_path_instructions() {
    local install_dir="$1"

    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                   Installation Complete!                     ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${YELLOW}Installation complete!${NC}"
    echo ""
    echo -e "msync has been installed to: ${BLUE}$install_dir${NC}"
    echo ""
    echo -e "If this is your first installation, you may need to restart your"
    echo -e "terminal or run: ${GREEN}source ~/.$(basename "${SHELL:-bash}" | sed 's/.*\///')rc${NC}"
    echo ""
    echo -e "Next steps:"
    echo -e "  1. Initialize msync in your project:"
    echo -e "     ${GREEN}cd /path/to/your/project${NC}"
    echo -e "     ${GREEN}msync init${NC}"
    echo ""
    echo -e "  2. Verify installation:"
    echo -e "     ${GREEN}msync --version${NC}"
    echo ""
}

# --- Print Usage ---
print_usage() {
    cat << EOF
${GREEN}msync Installer${NC} - Install msync to your system

${YELLOW}Usage:${NC} $SCRIPT_NAME [OPTIONS]

${BLUE}Options:${NC}
  -d, --dir DIR          Install directory (default: $DEFAULT_INSTALL_DIR)
  -s, --sudo             Use sudo for installation (required for /usr/local/bin)
  -l, --local            Build only, don't install (binary in current directory)
  -a, --arch ARCH        Architecture (amd64, arm64, 386, arm) - auto-detected if not set
  -o, --os OS            Target OS (linux, darwin, windows) - auto-detected if not set
  -h, --help             Show this help message

${BLUE}Examples:${NC}
  $SCRIPT_NAME                          # Build and install to ~/.local/bin
  $SCRIPT_NAME --dir /usr/local/bin    # Install system-wide (may need sudo)
  $SCRIPT_NAME --local                 # Build only, don't install
  $SCRIPT_NAME --arch arm64 --os linux # Build for specific platform

${YELLOW}Log file:${NC} $LOG_FILE

EOF
}

# --- Main Script ---
main() {
    # Default options
    local INSTALL_DIR="$DEFAULT_INSTALL_DIR"
    local USE_SUDO=false
    local BUILD_ONLY=false
    local TARGET_ARCH=""
    local TARGET_OS=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--dir)
                INSTALL_DIR="$2"
                shift 2
                ;;
            -s|--sudo)
                USE_SUDO=true
                shift
                ;;
            -l|--local)
                BUILD_ONLY=true
                shift
                ;;
            -a|--arch)
                TARGET_ARCH="$2"
                shift 2
                ;;
            -o|--os)
                TARGET_OS="$2"
                shift 2
                ;;
            -h|--help)
                print_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                print_usage
                exit 1
                ;;
        esac
    done

    # Initialize log file
    echo "msync Installation Log" > "$LOG_FILE"
    echo "Started: $(date)" >> "$LOG_FILE"
    echo "Script: $SCRIPT_NAME" >> "$LOG_FILE"
    echo "Install dir: $INSTALL_DIR" >> "$LOG_FILE"
    echo "" >> "$LOG_FILE"

    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                   msync Installer v$VERSION                  ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Check if installing to system directory and using sudo correctly
    if [[ "$USE_SUDO" == "false" ]] && [[ "$INSTALL_DIR" == "/usr/local/bin" ]]; then
        log_warning "Installing to /usr/local/bin typically requires sudo"
        log_info "Use --sudo flag or run with sudo, or choose a different directory"
        echo ""
        read -p "Continue without sudo? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Exiting. Use: $SCRIPT_NAME --dir /usr/local/bin --sudo"
            exit 1
        fi
    fi

    # Auto-detect architecture if not specified
    if [[ -z "$TARGET_ARCH" ]]; then
        TARGET_ARCH="$(detect_architecture)"
        log_info "Auto-detected architecture: $TARGET_ARCH"
    fi

    # Auto-detect OS if not specified
    if [[ -z "$TARGET_OS" ]]; then
        TARGET_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
        # Map darwin to darwin, linux to linux
        case "$TARGET_OS" in
            darwin) TARGET_OS="darwin" ;;
            linux) TARGET_OS="linux" ;;
            mingw*|msys*) TARGET_OS="windows" ;;
        esac
        log_info "Auto-detected OS: $TARGET_OS"
    fi

    # Determine output binary name
    local OUTPUT_NAME="$BINARY_NAME"
    if [[ -n "$TARGET_ARCH" ]] && [[ "$TARGET_ARCH" != "$(go env GOARCH)" ]] || [[ -n "$TARGET_OS" ]] && [[ "$TARGET_OS" != "$(go env GOOS)" ]]; then
        OUTPUT_NAME="${BINARY_NAME}-${TARGET_OS}-${TARGET_ARCH}"
        if [[ "$TARGET_OS" == "windows" ]]; then
            OUTPUT_NAME="${OUTPUT_NAME}.exe"
        fi
    fi

    # Environment check
    if ! check_environment; then
        log_error "Environment check failed. See $LOG_FILE for details."
        exit 1
    fi

    # Build the binary
    if ! build_binary "$TARGET_ARCH" "$TARGET_OS" "$OUTPUT_NAME"; then
        exit 1
    fi

    # Install if not build-only
    if [[ "$BUILD_ONLY" == "false" ]]; then
        if ! install_binary "$OUTPUT_NAME" "$INSTALL_DIR/$BINARY_NAME" "$USE_SUDO"; then
            log_error "Installation failed. See $LOG_FILE for details."
            exit 1
        fi

        # Add to PATH in shell config automatically
        add_to_shell_config "$INSTALL_DIR"

        # Show success message
        print_path_instructions "$INSTALL_DIR"
    else
        echo ""
        echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${GREEN}║                     Build Complete!                          ║${NC}"
        echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "Binary created: ${YELLOW}./$OUTPUT_NAME${NC}"
        echo -e "Run it with: ${YELLOW}./$OUTPUT_NAME --version${NC}"
        echo ""
    fi

    echo -e "${GREEN}Log file: $LOG_FILE${NC}"
    echo ""
}

# Run main function
main "$@"
