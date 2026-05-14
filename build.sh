#!/bin/bash
set -e

echo "Building VLX_ChatBridge..."

# Build the executable
go build -o VLX_ChatBridge ./cmd/chatbridge

echo "Build complete. Executable created: VLX_ChatBridge"
