#!/bin/bash
# Build script for Multi-Model Router
# Usage: ./build.sh [dev|release]

set -e

MODE="${1:-release}"
BINARY="MultiModelRouter.exe"
OUTPUT_DIR="build/bin"

echo "==> Building Multi-Model Router ($MODE mode)..."

# Check dependencies
if ! command -v go &>/dev/null; then
    echo "ERROR: Go is not installed"
    exit 1
fi

if ! command -v ~/go/bin/wails &>/dev/null && ! command -v wails &>/dev/null; then
    echo "ERROR: Wails CLI is not installed. Run: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    exit 1
fi

WAILS=$(command -v wails 2>/dev/null || echo ~/go/bin/wails)

# Install frontend dependencies
echo "==> Installing frontend dependencies..."
cd frontend && npm install && cd ..

# Build
echo "==> Compiling..."
if [ "$MODE" = "dev" ]; then
    $WAILS build
else
    $WAILS build -clean -ldflags "-s -w"
fi

# Verify
if [ -f "$OUTPUT_DIR/$BINARY" ]; then
    SIZE=$(ls -lh "$OUTPUT_DIR/$BINARY" | awk '{print $5}')
    echo ""
    echo "==> Build successful!"
    echo "    Output: $OUTPUT_DIR/$BINARY ($SIZE)"
    echo ""
    echo "Usage:"
    echo "    ./$OUTPUT_DIR/$BINARY                  # GUI mode"
    echo "    ./$OUTPUT_DIR/$BINARY serve --port 9680 # Headless proxy"
    echo "    ./$OUTPUT_DIR/$BINARY tui               # Terminal UI"
    echo "    ./$OUTPUT_DIR/$BINARY version           # Print version"
else
    echo "ERROR: Build failed - binary not found"
    exit 1
fi
