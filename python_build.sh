#!/usr/bin/env bash
# python_build.sh — Build kindle-helper as a Nuitka-compiled Python binary for Kindle ARM.
#
# Produces a deployable ZIP containing:
#   kindle.koplugin/
#     kindle-helper          - C wrapper (static ARM binary, entry point)
#     libsyscall_wrapper.so  - Syscall compatibility shim (preadv2/pwritev2)
#     dist/                  - Nuitka standalone output
#       main.bin             - Python binary
#       ld-linux-armhf.so.3  - Bundled dynamic linker (Debian Bookworm glibc)
#       *.so                 - All shared library dependencies
#     lua/                   - Lua plugin modules (unchanged)
#     main.lua, _meta.lua    - KOReader plugin entry points
#     ...                    - Other plugin files
#
# Usage:
#   ./python_build.sh              # Build ARMv7 (default)
#   ./python_build.sh --target arm64  # Build ARM64
#
# Prerequisites:
#   - Docker with buildx support
#   - QEMU binfmt for ARM emulation (docker run --rm --privileged multiarch/qemu-user-static --reset -p yes)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

TARGET="${1:-armv7}"
VERSION="$(date +%Y%m%d)"
OUTPUT_DIR="build"

echo "=== Kindle Helper Python Build ==="
echo "Target: $TARGET"
echo "Version: $VERSION"
echo ""

# Create output directory
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Build in Docker
case "$TARGET" in
    armv7)
        PLATFORM="linux/arm/v7"
        TAG="kindle-helper-builder-armv7"
        ;;
    arm64)
        PLATFORM="linux/arm64"
        # For arm64, we'd need a different base image
        echo "ARM64 not yet supported — use armv7 for now"
        exit 1
        ;;
    *)
        echo "Unknown target: $TARGET (use armv7 or arm64)"
        exit 1
        ;;
esac

echo "[1/4] Building Docker image..."
docker buildx build \
    --platform "$PLATFORM" \
    -t "$TAG" \
    -f .github/Dockerfile.arm \
    --load \
    .

echo "[2/4] Extracting binaries from container..."
CONTAINER_ID=$(docker create "$TAG")
mkdir -p "$OUTPUT_DIR/dist"

# Extract the built binary and dist directory
docker cp "$CONTAINER_ID:/build/output/kindle-helper" "$OUTPUT_DIR/kindle-helper"
docker cp "$CONTAINER_ID:/build/output/libsyscall_wrapper.so" "$OUTPUT_DIR/libsyscall_wrapper.so"
docker cp "$CONTAINER_ID:/build/output/dist/." "$OUTPUT_DIR/dist/"

docker rm "$CONTAINER_ID"

chmod +x "$OUTPUT_DIR/kindle-helper" "$OUTPUT_DIR/dist/main.bin" "$OUTPUT_DIR/dist/ld-linux-armhf.so.3"

echo "[3/4] Packaging plugin..."
# Create the plugin ZIP with Lua files + Python binary
ZIP_NAME="kindle-koplugin-${TARGET}.zip"
STAGING="$OUTPUT_DIR/kindle.koplugin"
rm -rf "$STAGING"
mkdir -p "$STAGING"

# Copy Lua plugin files
cp -r lua/ "$STAGING/lua/"
cp main.lua "$STAGING/"
cp _meta.lua "$STAGING/"

# Copy patches
cp -r patches/ "$STAGING/patches/" 2>/dev/null || true

# Copy the Python helper binary
cp "$OUTPUT_DIR/kindle-helper" "$STAGING/kindle-helper"
cp "$OUTPUT_DIR/libsyscall_wrapper.so" "$STAGING/libsyscall_wrapper.so"
cp -r "$OUTPUT_DIR/dist" "$STAGING/dist"

# Create ZIP
cd "$OUTPUT_DIR"
zip -r "$ZIP_NAME" kindle.koplugin/
cd "$SCRIPT_DIR"

echo "[4/4] Done!"
echo ""
echo "Output: $OUTPUT_DIR/$ZIP_NAME"
echo "Size: $(du -sh "$OUTPUT_DIR/$ZIP_NAME" | cut -f1)"
echo ""
echo "Deploy to Kindle:"
echo "  unzip $OUTPUT_DIR/$ZIP_NAME -d /mnt/us/koreader/plugins/"
