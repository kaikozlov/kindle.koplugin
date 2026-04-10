#!/bin/bash
set -euo pipefail

BUILD_DIR="build"
BIN_DIR="$BUILD_DIR/bin"
PLUGIN_NAME="kindle.koplugin"

rm -rf "$BUILD_DIR"
mkdir -p "$BIN_DIR" "$BUILD_DIR/$PLUGIN_NAME"

cp kindle.koplugin/*.lua "$BUILD_DIR/$PLUGIN_NAME/"
mkdir -p "$BUILD_DIR/$PLUGIN_NAME/src"
cp kindle.koplugin/src/*.lua "$BUILD_DIR/$PLUGIN_NAME/src/"

echo "Building armv5 (legacy)..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5 go build -ldflags="-s -w" -o "$BIN_DIR/kindle-helper-arm-legacy" ./kindle.koplugin/cmd/kindle-helper

echo "Building armv7..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o "$BIN_DIR/kindle-helper-armv7" ./kindle.koplugin/cmd/kindle-helper

echo "Building arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$BIN_DIR/kindle-helper-arm64" ./kindle.koplugin/cmd/kindle-helper

for arch in arm-legacy armv7 arm64; do
    cp "$BIN_DIR/kindle-helper-$arch" "$BUILD_DIR/$PLUGIN_NAME/kindle-helper"
    (
        cd "$BUILD_DIR"
        zip -rq "kindle-koplugin-$arch.zip" "$PLUGIN_NAME"
    )
done

rm -f "$BUILD_DIR/$PLUGIN_NAME/kindle-helper"

echo "Build artifacts:"
ls -lh "$BIN_DIR"
ls -lh "$BUILD_DIR"/*.zip
