#!/bin/bash
set -e

echo "Cleaning ChatBridge build artifacts..."
rm -f VLX_ChatBridge VLX_ChatBridge_frontend
rm -rf frontend_app/node_modules
rm -rf frontend_app/dist
rm -rf internal/ui/dist
echo "Clean complete."
