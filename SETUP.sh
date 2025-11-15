#!/bin/bash
# Quick Start Guide for Instagram Auto-DM

echo "🚀 Instagram Auto-DM - Quick Start"
echo "===================================="
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "📋 Creating .env from .env.example..."
    cp .env.example .env
    echo "✅ .env created. Please edit it with your values:"
    echo ""
    echo "Required values:"
    echo "  - DATABASE_URL (PostgreSQL connection string)"
    echo "  - IG_BUSINESS_ID (Instagram Business Account ID)"
    echo "  - ACCESS_TOKEN (Long-lived Instagram API token)"
    echo "  - VERIFY_TOKEN (Your webhook verification token)"
    echo ""
    echo "Then run this script again."
    exit 1
fi

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.23+"
    exit 1
fi

echo "✅ Go is installed"
echo "✅ .env file exists"
echo ""

# Download dependencies
echo "📦 Downloading dependencies..."
go mod download

# Build
echo "🔨 Building application..."
go build -o instagram-autodm .

if [ $? -eq 0 ]; then
    echo "✅ Build successful!"
    echo ""
    echo "🎯 Next steps:"
    echo "1. Edit .env with your Instagram credentials"
    echo "2. Ensure PostgreSQL is running"
    echo "3. Run: ./instagram-autodm"
    echo ""
    echo "📖 For detailed setup, see README.md"
else
    echo "❌ Build failed. Check error messages above."
    exit 1
fi
