#!/bin/bash
set -e

if [ "$EUID" -eq 0 ] && [ "$CI" != "true" ]; then
    echo "Error: Please do not run this build script as root."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "Error: go is not installed or not in PATH."
    exit 1
fi

TMP_FRONTEND=""
cleanup_build_temp() {
    [ -n "${TMP_FRONTEND:-}" ] && rm -rf "${TMP_FRONTEND}"
    if [ "$CI" != "true" ]; then
        rm -rf frontend_app/node_modules/ frontend_app/dist/ frontend_app/.svelte-kit/ node_modules/ internal/ui/dist/
        find . \( -name "*.test" -o -name "*.out" -o -name "*.swp" -o -name "*~" -o -name ".DS_Store" -o -name ".goutputstream-*" \) -type f -delete 2>/dev/null || true
    fi
}
trap cleanup_build_temp EXIT

echo "Building VLX_ChatBridge..."
go build -o ./VLX_ChatBridge ./cmd/chatbridge

echo "Building frontend assets..."
if command -v npm >/dev/null 2>&1; then
  if [ "$CI" = "true" ]; then
    ( cd frontend_app && npm ci --no-fund --no-audit && npm run build )
    mkdir -p internal/ui/dist
    rm -rf internal/ui/dist/*
    cp -r frontend_app/dist/* internal/ui/dist/
  else
    TMP_FRONTEND="$(mktemp -d /tmp/vlx_chatbridge_ui.XXXXXX)"
    cp -r frontend_app/. "${TMP_FRONTEND}/"
    ( cd "${TMP_FRONTEND}" && npm ci --no-fund --no-audit && npm run build )
    mkdir -p internal/ui/dist
    rm -rf internal/ui/dist/*
    cp -r "${TMP_FRONTEND}/dist/"* internal/ui/dist/
    rm -rf "${TMP_FRONTEND}"
    TMP_FRONTEND=""
  fi
else
  echo "Warning: npm not found; embedding placeholder internal/ui/dist."
  mkdir -p internal/ui/dist
  [ -f internal/ui/dist/index.html ] || echo "<!doctype html><html><body>Frontend not built</body></html>" > internal/ui/dist/index.html
fi

echo "Building frontend server..."
go build -o ./VLX_ChatBridge_frontend ./cmd/frontend

echo "Cleaning temporary build artifacts (keeping only clean repo + binaries)..."
echo "Build complete. Executables: VLX_ChatBridge, VLX_ChatBridge_frontend"
