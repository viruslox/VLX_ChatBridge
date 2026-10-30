#!/bin/bash
set -e

if [ "$EUID" -eq 0 ]; then
    echo "Error: Please do not run this build script as root."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Error: go is not installed or not in PATH."
    exit 1
fi

echo "Building VLX_ChatBridge..."
go build -o ./VLX_ChatBridge ./cmd/chatbridge

echo "Building frontend assets..."
if command -v npm >/dev/null 2>&1; then
  ( cd frontend_app && npm ci && npm run build )
  mkdir -p internal/ui/dist
  rm -rf internal/ui/dist/*
  cp -r frontend_app/dist/* internal/ui/dist/
else
  echo "Warning: npm not found; embedding existing internal/ui/dist."
fi

echo "Building frontend server..."
go build -o ./VLX_ChatBridge_frontend ./cmd/frontend

echo "Build complete. Executables: VLX_ChatBridge, VLX_ChatBridge_frontend"
