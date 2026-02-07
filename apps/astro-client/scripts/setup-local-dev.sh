#!/bin/bash
#
# Local Development Setup Script for astro-client
#
# This script configures your machine for local frontend development against
# a remote backend (e.g., https://astromode.ai). It sets up:
#   - Local subdomain (local.astromode.ai) for same-site cookie sharing
#   - HTTPS certificates for secure cookie handling
#
# Usage: bun run setup
#

set -e

# Configuration
DOMAIN="local.astromode.ai"
BACKEND_URL="https://astromode.ai"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CERT_DIR="$PROJECT_DIR/.certs"
ENV_FILE="$PROJECT_DIR/.env"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Print functions
print_header() {
    echo ""
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${BLUE}  $1${NC}"
    echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

print_step() {
    echo -e "${CYAN}▶${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

# Check if running on macOS
check_os() {
    if [[ "$OSTYPE" != "darwin"* ]]; then
        print_error "This script currently only supports macOS."
        print_info "For Linux, please see the manual setup instructions in README.md"
        exit 1
    fi
}

# Check if Homebrew is installed
check_homebrew() {
    if ! command -v brew &> /dev/null; then
        print_error "Homebrew is not installed."
        echo ""
        echo "  Please install Homebrew first:"
        echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        echo ""
        exit 1
    fi
}

# Setup hosts file entry
setup_hosts() {
    print_header "Step 1: Configure Local Domain"

    if grep -q "$DOMAIN" /etc/hosts 2>/dev/null; then
        print_success "$DOMAIN is already in /etc/hosts"
        return 0
    fi

    print_info "To share cookies with the backend, we need to add a local domain"
    print_info "that's on the same site as the backend (astromode.ai)."
    echo ""
    print_step "Adding '$DOMAIN' to /etc/hosts..."
    echo ""
    print_warning "This requires administrator privileges (sudo)."
    echo ""

    read -p "$(echo -e ${BOLD}"Press Enter to continue or Ctrl+C to cancel..."${NC})" </dev/tty

    echo "127.0.0.1 $DOMAIN" | sudo tee -a /etc/hosts > /dev/null

    if grep -q "$DOMAIN" /etc/hosts; then
        print_success "Added $DOMAIN to /etc/hosts"
    else
        print_error "Failed to add $DOMAIN to /etc/hosts"
        exit 1
    fi
}

# Install mkcert
setup_mkcert() {
    print_header "Step 2: Install Certificate Tool (mkcert)"

    if command -v mkcert &> /dev/null; then
        print_success "mkcert is already installed"
    else
        print_step "Installing mkcert via Homebrew..."
        brew install mkcert
        print_success "mkcert installed"
    fi

    # Check if CA is installed
    if ! mkcert -check &> /dev/null 2>&1; then
        echo ""
        print_step "Installing local Certificate Authority..."
        print_info "This allows your browser to trust locally-generated certificates."
        echo ""
        mkcert -install
        print_success "Local CA installed"
    else
        print_success "Local CA is already installed"
    fi
}

# Generate certificates
setup_certificates() {
    print_header "Step 3: Generate HTTPS Certificates"

    mkdir -p "$CERT_DIR"

    if [ -f "$CERT_DIR/$DOMAIN.pem" ] && [ -f "$CERT_DIR/$DOMAIN-key.pem" ]; then
        print_success "Certificates already exist"
        return 0
    fi

    print_step "Generating certificates for $DOMAIN..."

    (cd "$CERT_DIR" && mkcert "$DOMAIN")

    if [ -f "$CERT_DIR/$DOMAIN.pem" ]; then
        print_success "Certificates generated in .certs/"
    else
        print_error "Failed to generate certificates"
        exit 1
    fi
}

# Setup environment file
setup_env() {
    print_header "Step 4: Configure Environment"

    if [ -f "$ENV_FILE" ]; then
        # Check if it's already configured for remote backend
        if grep -q "VITE_API_URL=https://" "$ENV_FILE" 2>/dev/null; then
            print_success ".env is already configured for remote backend"
            return 0
        fi
    fi

    print_step "Configuring .env for remote backend development..."

    cat > "$ENV_FILE" << EOF
# Backend API URL for remote development
VITE_API_URL=$BACKEND_URL
EOF

    print_success ".env configured with VITE_API_URL=$BACKEND_URL"
}

# Print completion message
print_completion() {
    print_header "Setup Complete!"

    echo -e "${GREEN}Your local development environment is ready.${NC}"
    echo ""
    echo -e "${BOLD}To start developing:${NC}"
    echo ""
    echo "  1. Start the dev server:"
    echo -e "     ${CYAN}bun run dev${NC}"
    echo ""
    echo "  2. Open your browser to:"
    echo -e "     ${CYAN}https://$DOMAIN:5173${NC}"
    echo ""
    echo -e "${BOLD}What was configured:${NC}"
    echo ""
    echo "  - /etc/hosts    → $DOMAIN points to 127.0.0.1"
    echo "  - .certs/       → HTTPS certificates for local development"
    echo "  - .env          → Backend URL set to $BACKEND_URL"
    echo ""
    print_info "The dev server will automatically use HTTPS when certificates are present."
    echo ""
}

# Main
main() {
    clear
    print_header "Astro Client - Local Development Setup"

    echo "This script will configure your machine for local frontend development"
    echo "against the remote backend at $BACKEND_URL."
    echo ""
    echo "This enables:"
    echo "  - Authentication flow with cookie-based sessions"
    echo "  - HTTPS for secure cookie handling"
    echo "  - Same-site domain for proper cookie sharing"
    echo ""

    read -p "$(echo -e ${BOLD}"Continue with setup? [Y/n] "${NC})" response </dev/tty
    response=${response:-Y}

    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        echo "Setup cancelled."
        exit 0
    fi

    check_os
    check_homebrew
    setup_hosts
    setup_mkcert
    setup_certificates
    setup_env
    print_completion
}

main "$@"
