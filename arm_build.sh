#!/bin/bash
set -e

BUILD_DIR="build"
BIN_DIR="$BUILD_DIR/bin"
PLUGIN_NAME="kindle.koplugin"
PLUGIN_SRC="lua"
BINARY_NAME="kindle-helper"
DEVICE_PLUGIN_DIR="/mnt/us/koreader/plugins/kindle.koplugin"

# Defaults
PACKAGE_ONLY=false
DEPLOY=false
DEPLOY_TARGET=""
DEPLOY_PORT="22"
DEPLOY_ARCH="armv7"

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -p, --package              Package only (skip Go compilation, reuse existing binaries)"
    echo "  -d, --deploy TARGET        Build + deploy to device via SSH (e.g. root@10.0.0.103)"
    echo "  --port PORT                SSH port (default: 22)"
    echo "  -a, --arch ARCH            Target architecture: arm-legacy, armv7, arm64 (default: armv7)"
    echo "  -h, --help                 Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Build all architectures, create zips"
    echo "  $0 -p                                 # Re-package zips from existing binaries"
    echo "  $0 --deploy root@10.0.0.103           # Build + deploy armv7 via SSH port 22"
    echo "  $0 -d root@10.0.0.103 --port 5132     # Build + deploy via custom SSH port"
    echo "  $0 -d root@10.0.0.103 -a arm64        # Deploy arm64 build instead"
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--package)
            PACKAGE_ONLY=true
            shift
            ;;
        -d|--deploy)
            DEPLOY=true
            DEPLOY_TARGET="$2"
            if [[ -z "$DEPLOY_TARGET" ]]; then
                echo "Error: --deploy requires a target (e.g. root@10.0.0.103)"
                exit 1
            fi
            shift 2
            ;;
        --port)
            DEPLOY_PORT="$2"
            if [[ -z "$DEPLOY_PORT" ]]; then
                echo "Error: --port requires a port number"
                exit 1
            fi
            shift 2
            ;;
        -a|--arch)
            DEPLOY_ARCH="$2"
            if [[ -z "$DEPLOY_ARCH" ]]; then
                echo "Error: --arch requires a value (arm-legacy, armv7, arm64)"
                exit 1
            fi
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage."
            exit 1
            ;;
    esac
done

# Validate arch
case "$DEPLOY_ARCH" in
    arm-legacy|armv7|arm64) ;;
    *)
        echo "Error: unknown architecture '$DEPLOY_ARCH' (use arm-legacy, armv7, or arm64)"
        exit 1
        ;;
esac

# Check for existing binaries if package-only mode
if $PACKAGE_ONLY; then
    if [[ ! -f "$BIN_DIR/$BINARY_NAME-arm-legacy" ]] || [[ ! -f "$BIN_DIR/$BINARY_NAME-armv7" ]] || [[ ! -f "$BIN_DIR/$BINARY_NAME-arm64" ]]; then
        echo "Error: Binaries not found. Run a full build first."
        exit 1
    fi
    echo "Package-only mode: reusing existing binaries"
fi

# Create build directories (don't wipe bin dir in package mode)
if $PACKAGE_ONLY; then
    rm -rf "$BUILD_DIR/$PLUGIN_NAME"
    rm -f "$BUILD_DIR"/*.zip
else
    rm -rf "$BUILD_DIR"
    mkdir -p "$BIN_DIR"
fi
mkdir -p "$BUILD_DIR/$PLUGIN_NAME"

# Copy Lua plugin files
cp *.lua "$BUILD_DIR/$PLUGIN_NAME/"

# Copy Lua modules
mkdir -p "$BUILD_DIR/$PLUGIN_NAME/lua"
cp "$PLUGIN_SRC"/*.lua "$BUILD_DIR/$PLUGIN_NAME/lua/"

# Copy lib helpers (DRM JAR + hook)
mkdir -p "$BUILD_DIR/$PLUGIN_NAME/lib"
cp lib/* "$BUILD_DIR/$PLUGIN_NAME/lib/" 2>/dev/null || true

if ! $PACKAGE_ONLY; then
    # Build for armv5 (legacy Kindle devices)
    echo "Building for armv5 (legacy)..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5 go build -ldflags="-s -w" -o "$BIN_DIR/$BINARY_NAME-arm-legacy" ./cmd/kindle-helper
    echo "armv5: $(ls -lh "$BIN_DIR/$BINARY_NAME-arm-legacy" | awk '{print $5}')"

    # Build for armv7 (32-bit ARM)
    echo "Building for armv7..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o "$BIN_DIR/$BINARY_NAME-armv7" ./cmd/kindle-helper
    echo "armv7: $(ls -lh "$BIN_DIR/$BINARY_NAME-armv7" | awk '{print $5}')"

    # Build for arm64 (64-bit ARM)
    echo "Building for arm64..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$BIN_DIR/$BINARY_NAME-arm64" ./cmd/kindle-helper
    echo "arm64: $(ls -lh "$BIN_DIR/$BINARY_NAME-arm64" | awk '{print $5}')"
fi

# Create zips
for arch in arm-legacy armv7 arm64; do
    echo "Creating ${arch} zip..."
    cp "$BIN_DIR/$BINARY_NAME-${arch}" "$BUILD_DIR/$PLUGIN_NAME/$BINARY_NAME"
    (cd "$BUILD_DIR" && zip -rq "kindle-koplugin-${arch}.zip" "$PLUGIN_NAME")
done

# Clean up plugin staging dir (keep bins and zips)
rm -rf "$BUILD_DIR/$PLUGIN_NAME"

echo ""
echo "Done! Release files:"
ls -lh "$BUILD_DIR"/*.zip
echo ""
echo "Binaries:"
ls -lh "$BIN_DIR"/*

# ── Deploy ──────────────────────────────────────────────────────────────
if $DEPLOY; then
    echo ""
    echo "Deploying to $DEPLOY_TARGET:$DEPLOY_PORT (arch: $DEPLOY_ARCH)..."

    SSH="ssh -o ConnectTimeout=10 -p $DEPLOY_PORT $DEPLOY_TARGET"
    SCP="scp -o ConnectTimeout=10 -P $DEPLOY_PORT"

    # Test SSH connectivity
    echo "Testing SSH connection..."
    if ! $SSH "echo ok" >/dev/null 2>&1; then
        echo "Error: Cannot connect to $DEPLOY_TARGET on port $DEPLOY_PORT"
        exit 1
    fi
    echo "Connected."

    # Remove stale plugin directory on device
    echo "Cleaning up old plugin..."
    $SSH "rm -rf $DEVICE_PLUGIN_DIR"
    $SSH "mkdir -p $DEVICE_PLUGIN_DIR/lua $DEVICE_PLUGIN_DIR/lib"

    # Copy plugin files
    STAGING="$BUILD_DIR/staging"
    mkdir -p "$STAGING"

    # Reassemble the plugin from the built binary + lua files
    cp *.lua "$STAGING/"
    mkdir -p "$STAGING/lua"
    cp "$PLUGIN_SRC"/*.lua "$STAGING/lua/"
    mkdir -p "$STAGING/lib"
    cp lib/* "$STAGING/lib/" 2>/dev/null || true
    cp "$BIN_DIR/$BINARY_NAME-$DEPLOY_ARCH" "$STAGING/$BINARY_NAME"

    echo "Uploading plugin files..."
    $SCP -r "$STAGING/"* "$DEPLOY_TARGET:$DEVICE_PLUGIN_DIR/"

    # Ensure the binary is executable on device
    $SSH "chmod +x $DEVICE_PLUGIN_DIR/$BINARY_NAME"

    # Clean up local staging
    rm -rf "$STAGING"

    echo ""
    echo "Deployed to $DEPLOY_TARGET:$DEVICE_PLUGIN_DIR ($DEPLOY_ARCH)"
    echo "Restart KOReader on the device to load the updated plugin."
fi
