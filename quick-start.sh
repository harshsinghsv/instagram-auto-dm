#!/bin/bash

# Instagram Auto-DM Quick Start Script
# This script automates the initial setup process

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored message
print_message() {
    color=$1
    message=$2
    echo -e "${color}${message}${NC}"
}

print_header() {
    echo ""
    print_message "$BLUE" "================================================"
    print_message "$BLUE" "$1"
    print_message "$BLUE" "================================================"
    echo ""
}

print_success() {
    print_message "$GREEN" "✓ $1"
}

print_error() {
    print_message "$RED" "✗ $1"
}

print_warning() {
    print_message "$YELLOW" "⚠ $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Main setup function
main() {
    print_header "Instagram Auto-DM Setup Script"
    
    # Step 1: Check prerequisites
    print_message "$YELLOW" "Step 1: Checking prerequisites..."
    check_prerequisites
    
    # Step 2: Setup environment
    print_message "$YELLOW" "Step 2: Setting up environment..."
    setup_environment
    
    # Step 3: Configure application
    print_message "$YELLOW" "Step 3: Configuring application..."
    configure_app
    
    # Step 4: Start services
    print_message "$YELLOW" "Step 4: Starting services..."
    start_services
    
    # Step 5: Verify setup
    print_message "$YELLOW" "Step 5: Verifying setup..."
    verify_setup
    
    # Print next steps
    print_next_steps
}

# Check prerequisites
check_prerequisites() {
    local missing_deps=()
    
    # Check Docker
    if command_exists docker; then
        print_success "Docker is installed"
    else
        missing_deps+=("Docker")
        print_error "Docker is not installed"
    fi
    
    # Check Docker Compose
    if command_exists docker-compose; then
        print_success "Docker Compose is installed"
    else
        missing_deps+=("Docker Compose")
        print_error "Docker Compose is not installed"
    fi
    
    # Check Go (optional for development)
    if command_exists go; then
        print_success "Go is installed ($(go version))"
    else
        print_warning "Go is not installed (optional for development)"
    fi
    
    # Check curl
    if command_exists curl; then
        print_success "curl is installed"
    else
        missing_deps+=("curl")
        print_error "curl is not installed"
    fi
    
    # If missing dependencies, show install instructions
    if [ ${#missing_deps[@]} -gt 0 ]; then
        echo ""
        print_error "Missing dependencies: ${missing_deps[*]}"
        echo ""
        print_message "$YELLOW" "Install instructions:"
        
        if [[ " ${missing_deps[@]} " =~ " Docker " ]]; then
            echo "  Docker: https://docs.docker.com/get-docker/"
        fi
        
        if [[ " ${missing_deps[@]} " =~ " Docker Compose " ]]; then
            echo "  Docker Compose: https://docs.docker.com/compose/install/"
        fi
        
        if [[ " ${missing_deps[@]} " =~ " curl " ]]; then
            echo "  curl: sudo apt-get install curl (Ubuntu) or brew install curl (Mac)"
        fi
        
        echo ""
        exit 1
    fi
    
    echo ""
}

# Setup environment
setup_environment() {
    # Check if .env already exists
    if [ -f .env ]; then
        print_warning ".env file already exists"
        read -p "Do you want to overwrite it? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_message "$YELLOW" "Skipping environment setup"
            return
        fi
    fi
    
    # Copy .env.example to .env
    if [ -f .env.example ]; then
        cp .env.example .env
        print_success "Created .env file from template"
    else
        print_error ".env.example not found!"
        exit 1
    fi
    
    echo ""
}

# Configure application
configure_app() {
    print_message "$BLUE" "Let's configure your Instagram Auto-DM system"
    echo ""
    
    # Read configuration values
    read -p "Enter your Meta App VERIFY_TOKEN (random string): " verify_token
    read -p "Enter your Instagram ACCESS_TOKEN: " access_token
    read -p "Enter your Instagram Business Account ID: " ig_business_id
    read -p "Enter trigger keywords (comma-separated, e.g., subscribe,download): " keywords
    read -p "Enter DM message (use \\n for newlines): " dm_message
    
    # Update .env file
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' "s|VERIFY_TOKEN=.*|VERIFY_TOKEN=$verify_token|g" .env
        sed -i '' "s|ACCESS_TOKEN=.*|ACCESS_TOKEN=$access_token|g" .env
        sed -i '' "s|IG_BUSINESS_ID=.*|IG_BUSINESS_ID=$ig_business_id|g" .env
        sed -i '' "s|KEYWORDS=.*|KEYWORDS=$keywords|g" .env
        sed -i '' "s|DM_MESSAGE=.*|DM_MESSAGE=$dm_message|g" .env
    else
        # Linux
        sed -i "s|VERIFY_TOKEN=.*|VERIFY_TOKEN=$verify_token|g" .env
        sed -i "s|ACCESS_TOKEN=.*|ACCESS_TOKEN=$access_token|g" .env
        sed -i "s|IG_BUSINESS_ID=.*|IG_BUSINESS_ID=$ig_business_id|g" .env
        sed -i "s|KEYWORDS=.*|KEYWORDS=$keywords|g" .env
        sed -i "s|DM_MESSAGE=.*|DM_MESSAGE=$dm_message|g" .env
    fi
    
    print_success "Configuration saved to .env"
    echo ""
}

# Start services
start_services() {
    print_message "$BLUE" "Building and starting Docker containers..."
    echo ""
    
    # Build and start with Docker Compose
    if docker-compose up -d --build; then
        print_success "Services started successfully"
        echo ""
        
        # Wait for services to be ready
        print_message "$YELLOW" "Waiting for services to be ready..."
        sleep 5
        
        # Check if containers are running
        if docker-compose ps | grep -q "Up"; then
            print_success "All containers are running"
        else
            print_error "Some containers failed to start"
            docker-compose ps
            exit 1
        fi
    else
        print_error "Failed to start services"
        exit 1
    fi
    
    echo ""
}

# Verify setup
verify_setup() {
    print_message "$BLUE" "Running verification checks..."
    echo ""
    
    # Check health endpoint
    print_message "$YELLOW" "Checking health endpoint..."
    sleep 2
    
    if curl -s http://localhost:8080/health > /dev/null; then
        print_success "Health endpoint is responding"
        
        # Get health status
        health_response=$(curl -s http://localhost:8080/health)
        echo "Health Status: $health_response"
    else
        print_error "Health endpoint is not responding"
        print_warning "Check logs with: docker-compose logs app"
    fi
    
    echo ""
    
    # Check database connection
    print_message "$YELLOW" "Checking database connection..."
    if docker-compose exec -T postgres pg_isready -U autodm_user > /dev/null 2>&1; then
        print_success "Database is ready"
    else
        print_error "Database is not ready"
    fi
    
    echo ""
    
    # Show container status
    print_message "$YELLOW" "Container status:"
    docker-compose ps
    
    echo ""
}

# Print next steps
print_next_steps() {
    print_header "Setup Complete! 🎉"
    
    print_message "$GREEN" "Your Instagram Auto-DM system is now running!"
    echo ""
    
    print_message "$BLUE" "Next Steps:"
    echo ""
    echo "1. Configure Meta Webhook:"
    echo "   - Go to Meta Developer Console"
    echo "   - Set webhook URL to: https://YOUR_DOMAIN/webhook"
    echo "   - Use your VERIFY_TOKEN from .env file"
    echo "   - Subscribe to 'comments' field"
    echo ""
    
    echo "2. Test the webhook:"
    echo "   curl \"http://localhost:8080/webhook?hub.mode=subscribe&hub.verify_token=YOUR_TOKEN&hub.challenge=test\""
    echo ""
    
    echo "3. View logs:"
    echo "   docker-compose logs -f app"
    echo ""
    
    echo "4. Access health check:"
    echo "   curl http://localhost:8080/health"
    echo ""
    
    echo "5. Stop services:"
    echo "   docker-compose down"
    echo ""
    
    print_message "$YELLOW" "Important Links:"
    echo "   - Meta Developer Console: https://developers.facebook.com/"
    echo "   - Documentation: README.md"
    echo "   - Testing Guide: TESTING_GUIDE.md"
    echo ""
    
    print_message "$GREEN" "Happy automating! 🚀"
    echo ""
}

# Run main function
main