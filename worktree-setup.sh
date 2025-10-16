#!/bin/bash
# Worktree setup script for TapeDeck development
# Run this after creating a new worktree to copy data and create .env

set -e

MAIN_REPO="../.."
WORKTREE_DIR="$(pwd)"

echo "Setting up TapeDeck worktree..."

# Create data directory
mkdir -p data

# Copy config and database if they exist in main repo
if [ -f "$MAIN_REPO/config.yml" ]; then
    echo "✓ Copying config.yml from main repo"
    cp "$MAIN_REPO/config.yml" .
else
    echo "⚠ No config.yml in main repo (setup wizard will be required)"
fi

if [ -f "$MAIN_REPO/data/tapedeck.db" ]; then
    echo "✓ Copying database from main repo"
    cp "$MAIN_REPO/data/tapedeck.db" data/
else
    echo "⚠ No database in main repo (will be created on first run)"
fi

# Copy secret files if they exist (critical for database encryption compatibility)
if [ -f "$MAIN_REPO/.encryption_key" ]; then
    echo "✓ Copying .encryption_key from main repo"
    cp "$MAIN_REPO/.encryption_key" .
fi

if [ -f "$MAIN_REPO/.session_secret" ]; then
    echo "✓ Copying .session_secret from main repo"
    cp "$MAIN_REPO/.session_secret" .
fi

if [ -f "$MAIN_REPO/.csrf_key" ]; then
    echo "✓ Copying .csrf_key from main repo"
    cp "$MAIN_REPO/.csrf_key" .
fi

# Create .env for development
if [ ! -f ".env" ]; then
    echo "✓ Creating .env file"
    cat > .env << EOF
DEV_MODE=true
LOG_LEVEL=debug
EOF
else
    echo "✓ .env already exists"
fi

# Generate templ files
echo "✓ Generating templ files..."
templ generate

echo ""
echo "✨ Worktree setup complete!"
echo ""
echo "To start the server:"
echo "  air              # Hot reload dev server"
echo "  go run .         # Direct run"
echo ""
echo "Access at:"
echo "  http://localhost:3001  # Direct"
echo "  http://localhost:3002  # Air proxy with auto-refresh"
