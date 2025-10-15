#!/bin/bash
set -e

echo "→ Running go fmt..."
go fmt ./...

echo "→ Running golangci-lint..."
golangci-lint run --timeout=5m

echo "→ Running tests..."
go test -v -race ./...

echo "✓ All checks passed"
