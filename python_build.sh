#!/usr/bin/env bash
# python_build.sh — Build kindle-helper using pre-built CPython + wheels.
#
# No compilation of Python. Downloads:
#   1. CPython standalone (armv7) from astral-sh/python-build-standalone
#   2. C extension wheels from PyPI (lxml) and piwheels (Pillow, pycryptodome)
#   3. Pure Python packages (beautifulsoup4)
# Only Docker step: cross-compile tiny C wrapper + syscall shim (~30 seconds)
#
# Produces a deployable ZIP:
#   kindle.koplugin/
#     kindle-helper          - C wrapper (static ARM binary, invokes python3)
#     libsyscall_wrapper.so  - Syscall compatibility shim (preadv2/pwritev2)
#     dist/                  - Python runtime + dependencies
#       bin/python3          - CPython interpreter (glibc 2.17+, Kindle OK)
#       lib/python3.11/      - Stdlib + site-packages
#       kindle_helper.py     - Entry point
#       kfxlib/              - KFX conversion engine
#       dedrm/               - DRM decryption
#     lua/                   - Lua plugin modules
#     main.lua, _meta.lua    - KOReader plugin entry points
#
# Usage:
#   ./python_build.sh
#
# Prerequisites:
#   - Docker with buildx (for C wrapper only, ~30 seconds)
#   - Internet access for downloads

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

TARGET="armv7"
VERSION="$(date +%Y%m%d)"
OUTPUT_DIR="build"

# Versions
PYTHON_BUILD_STANDALONE_TAG="20260414"
CPYTHON_VERSION="3.11.15"
LXML_VERSION="6.0.3"
PILLOW_VERSION="12.2.0"
PYCRYPTODOME_VERSION="3.9.9"

echo "=== Kindle Helper Build (download-based) ==="
echo "Python: CPython $CPYTHON_VERSION"
echo "Version: $VERSION"
echo ""

# Create output directory
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

STAGING="$OUTPUT_DIR/kindle.koplugin"
mkdir -p "$STAGING"

# ---------------------------------------------------------------------------
# Step 1: Download and extract CPython standalone
# ---------------------------------------------------------------------------
echo "[1/5] Downloading CPython $CPYTHON_VERSION (armv7)..."

CPYTHON_TARBALL="$OUTPUT_DIR/cpython.tar.gz"
CPYTHON_URL="https://github.com/astral-sh/python-build-standalone/releases/download/${PYTHON_BUILD_STANDALONE_TAG}/cpython-${CPYTHON_VERSION}%2B${PYTHON_BUILD_STANDALONE_TAG}-armv7-unknown-linux-gnueabihf-install_only.tar.gz"

if [ ! -f "$CPYTHON_TARBALL" ]; then
    curl -fSL --progress-bar -o "$CPYTHON_TARBALL" "$CPYTHON_URL"
fi

echo "  Extracting..."
mkdir -p "$OUTPUT_DIR/dist"
tar xzf "$CPYTHON_TARBALL" -C "$OUTPUT_DIR/dist" --strip-components=1

echo "  OK: $(ls "$OUTPUT_DIR/dist/bin/python3")"

# ---------------------------------------------------------------------------
# Step 2: Install packages
# ---------------------------------------------------------------------------
echo "[2/5] Installing packages..."

DIST_DIR="$OUTPUT_DIR/dist"
SITE_PACKAGES="$DIST_DIR/lib/python3.11/site-packages"
mkdir -p "$SITE_PACKAGES"

# lxml — available on PyPI as manylinux_2_31_armv7l
echo "  lxml $LXML_VERSION (PyPI)..."
curl -fSL -o /tmp/lxml.whl "https://files.pythonhosted.org/packages/$(pip3 index versions lxml 2>/dev/null | head -1 || echo 'cp311/cp311-manylinux_2_31_armv7l')/lxml-${LXML_VERSION}-cp311-cp311-manylinux_2_31_armv7l.whl" 2>/dev/null || {
    echo "  Trying piwheels fallback..."
    curl -fSL -o /tmp/lxml.whl "https://archive1.piwheels.org/simple/lxml/lxml-${LXML_VERSION}-cp311-cp311-linux_armv7l.whl"
}
unzip -q -o /tmp/lxml.whl -d "$SITE_PACKAGES"

# Pillow — piwheels
echo "  Pillow $PILLOW_VERSION (piwheels)..."
curl -fSL -o /tmp/pillow.whl "https://www.piwheels.org/simple/pillow/pillow-${PILLOW_VERSION}-cp311-cp311-linux_armv7l.whl"
unzip -q -o /tmp/pillow.whl -d "$SITE_PACKAGES"

# pycryptodome — piwheels (archive for older versions)
echo "  pycryptodome $PYCRYPTODOME_VERSION (piwheels)..."
curl -fSL -o /tmp/pycryptodome.whl "https://archive1.piwheels.org/simple/pycryptodome/pycryptodome-${PYCRYPTODOME_VERSION}-cp311-cp311-linux_armv7l.whl"
unzip -q -o /tmp/pycryptodome.whl -d "$SITE_PACKAGES"

# beautifulsoup4 — pure Python, download from PyPI as universal wheel
echo "  beautifulsoup4..."
pip3 download --only-binary=:all: --python-version 3.11 --no-deps --dest /tmp/bs4dl beautifulsoup4 2>/dev/null
unzip -q -o /tmp/bs4dl/beautifulsoup4*.whl -d "$SITE_PACKAGES"

# ---------------------------------------------------------------------------
# Step 3: Copy plugin Python source into dist
# ---------------------------------------------------------------------------
echo "[3/5] Copying plugin source..."

cp python/kindle_helper.py "$DIST_DIR/kindle_helper.py"
cp -r python/kfxlib/ "$DIST_DIR/kfxlib/"
cp -r python/dedrm/ "$DIST_DIR/dedrm/"

# Clean bytecode
find "$DIST_DIR" -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
find "$DIST_DIR" -name "*.pyc" -delete 2>/dev/null || true

# Strip unnecessary files from CPython to reduce size
rm -rf "$DIST_DIR/include"                # C headers
rm -rf "$DIST_DIR/share"                  # man pages, etc
rm -rf "$DIST_DIR/bin/2to3"*              # unused tools
rm -rf "$DIST_DIR/bin/idle3"*             # IDE
rm -rf "$DIST_DIR/bin/pydoc3"*            # docs
rm -rf "$DIST_DIR/bin/pip"*               # pip not needed at runtime
rm -rf "$DIST_DIR/lib/libtcl"*            # Tcl/Tk not needed
rm -rf "$DIST_DIR/lib/itcl"*              # Tcl extension
rm -rf "$DIST_DIR/lib/thread"*            # Tcl extension
rm -rf "$DIST_DIR/lib/tcl9"               # Tcl runtime
rm -rf "$DIST_DIR/lib/python3.11/idlelib" # IDE
rm -rf "$DIST_DIR/lib/python3.11/tkinter" # Tk
rm -rf "$DIST_DIR/lib/python3.11/test"    # test suite
rm -rf "$DIST_DIR/lib/python3.11/unittest"# test framework
rm -rf "$DIST_DIR/lib/python3.11/pydoc_data" # docs
rm -rf "$DIST_DIR/lib/python3.11/ensurepip"  # pip bundler
rm -rf "$DIST_DIR/lib/python3.11/lib2to3"   # 2to3 converter
rm -rf "$DIST_DIR/lib/python3.11/turtle.py"  # turtle graphics
rm -rf "$DIST_DIR/lib/python3.11/telnetlib.py"
rm -rf "$DIST_DIR/lib/python3.11/asyncio"    # async framework
find "$DIST_DIR/lib/python3.11" -name "tests" -exec rm -rf {} + 2>/dev/null || true
find "$DIST_DIR/lib/python3.11" -name "test" -type d -exec rm -rf {} + 2>/dev/null || true

# Strip unnecessary Crypto modules (only keep AES and friends)
rm -rf "$SITE_PACKAGES/Crypto/SelfTest"
rm -rf "$SITE_PACKAGES/Crypto/IO"
for cipher_dir in "$SITE_PACKAGES/Crypto/Cipher"/*.so; do
    base=$(basename "$cipher_dir" .so)
    case "$base" in
        _ARC4|_AES|_Salsa20|_pkcs1_decode|_raw_aes|_raw_aesni|_raw_arc2|_raw_blowfish|_raw_cast|_raw_cbc|_raw_cfb|_raw_ctr|_raw_des|_raw_des3|_raw_ecb|_raw_gcm|_raw_ocb|_raw_ofb|_raw_salsa20|_chacha20|_pkcs1_decode)
        ;;
        *)
            rm -f "$cipher_dir"
            ;;
    esac
done

# Strip pip and setuptools from site-packages (build tools only)
rm -rf "$SITE_PACKAGES/pip"
rm -rf "$SITE_PACKAGES/pip"*.dist-info
rm -rf "$SITE_PACKAGES/setuptools"
rm -rf "$SITE_PACKAGES/setuptools"*.dist-info
rm -rf "$SITE_PACKAGES/_distutils_hack"

# ---------------------------------------------------------------------------
# Step 4: Build C wrapper + syscall shim (tiny, ~30 seconds in Docker)
# ---------------------------------------------------------------------------
echo "[4/5] Building C wrapper..."

WRAPPER_TAG="kindle-wrapper-builder"

docker buildx build \
    --platform linux/arm/v7 \
    -t "$WRAPPER_TAG" \
    -f .github/Dockerfile.wrapper \
    --load \
    .

CONTAINER_ID=$(docker create "$WRAPPER_TAG")
docker cp "$CONTAINER_ID:/build/kindle-helper" "$OUTPUT_DIR/kindle-helper"
docker cp "$CONTAINER_ID:/build/libsyscall_wrapper.so" "$OUTPUT_DIR/libsyscall_wrapper.so"
docker rm "$CONTAINER_ID"

chmod +x "$OUTPUT_DIR/kindle-helper"

# ---------------------------------------------------------------------------
# Step 5: Package the plugin ZIP
# ---------------------------------------------------------------------------
echo "[5/5] Packaging..."

# Copy Lua plugin files
cp -r lua/ "$STAGING/lua/"
cp main.lua "$STAGING/"
cp _meta.lua "$STAGING/"
cp -r patches/ "$STAGING/patches/" 2>/dev/null || true

# Copy the C wrapper
cp "$OUTPUT_DIR/kindle-helper" "$STAGING/"
cp "$OUTPUT_DIR/libsyscall_wrapper.so" "$STAGING/"

# Copy the DRM helpers (crypto hook, Java jar)
mkdir -p "$STAGING/lib"
cp lib/crypto_hook.so "$STAGING/lib/"
cp lib/KFXVoucherExtractor.jar "$STAGING/lib/"

# Copy the Python dist
cp -r "$DIST_DIR" "$STAGING/dist"

# Create ZIP
ZIP_NAME="kindle-koplugin-${TARGET}.zip"
cd "$OUTPUT_DIR"
zip -r "$ZIP_NAME" kindle.koplugin/
cd "$SCRIPT_DIR"

echo ""
echo "=== Done! ==="
echo "Output: $OUTPUT_DIR/$ZIP_NAME"
echo "Size: $(du -sh "$OUTPUT_DIR/$ZIP_NAME" | cut -f1)"
echo ""
echo "Deploy to Kindle:"
echo "  unzip $OUTPUT_DIR/$ZIP_NAME -d /mnt/us/koreader/plugins/"
