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
BUILD_CACHE_REV="2"

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
# Steps 1-2: CPython + packages (cached in build-cache/)
#
# The cache is keyed on all version pins.  If the versions haven't changed,
# we skip downloading and installing — just copy from the cache.
# ---------------------------------------------------------------------------
CACHE_DIR="build-cache"
CACHE_KEY="r${BUILD_CACHE_REV}_cpython-${CPYTHON_VERSION}+${PYTHON_BUILD_STANDALONE_TAG}_lxml-${LXML_VERSION}_pillow-${PILLOW_VERSION}_pycrypto-${PYCRYPTODOME_VERSION}"
CACHE_STAMP="$CACHE_DIR/$CACHE_KEY/.stamp"

if [ -f "$CACHE_STAMP" ]; then
    echo "[1/5] CPython $CPYTHON_VERSION — cached"
    echo "[2/5] Packages — cached"
    cp -a "$CACHE_DIR/$CACHE_KEY/dist" "$OUTPUT_DIR/dist"
else
    mkdir -p "$CACHE_DIR/$CACHE_KEY"

    # --- Step 1: Download CPython ---
    echo "[1/5] Downloading CPython $CPYTHON_VERSION (armv7)..."

    CPYTHON_TARBALL="$CACHE_DIR/cpython-${CPYTHON_VERSION}+${PYTHON_BUILD_STANDALONE_TAG}.tar.gz"
    CPYTHON_URL="https://github.com/astral-sh/python-build-standalone/releases/download/${PYTHON_BUILD_STANDALONE_TAG}/cpython-${CPYTHON_VERSION}%2B${PYTHON_BUILD_STANDALONE_TAG}-armv7-unknown-linux-gnueabihf-install_only.tar.gz"

    if [ ! -f "$CPYTHON_TARBALL" ]; then
        curl -fSL --progress-bar -o "$CPYTHON_TARBALL" "$CPYTHON_URL"
    fi

    echo "  Extracting..."
    mkdir -p "$CACHE_DIR/$CACHE_KEY/dist"
    tar xzf "$CPYTHON_TARBALL" -C "$CACHE_DIR/$CACHE_KEY/dist" --strip-components=1
    echo "  OK: $(ls "$CACHE_DIR/$CACHE_KEY/dist/bin/python3")"

    # --- Step 2: Install packages ---
    echo "[2/5] Installing packages..."

    CACHE_DIST="$CACHE_DIR/$CACHE_KEY/dist"
    SITE_PACKAGES="$CACHE_DIST/lib/python3.11/site-packages"
    mkdir -p "$SITE_PACKAGES"

    # lxml — use the official manylinux armv7 wheel.  Unlike the piwheels
    # build, this statically carries libxml2/libxslt and does not inherit a
    # large set of distro-specific native dependencies.
    echo "  lxml $LXML_VERSION (PyPI manylinux armv7)..."
    rm -rf /tmp/lxmldl && mkdir -p /tmp/lxmldl
    python3 -m pip download \
        --only-binary=:all: --no-deps \
        --platform manylinux_2_31_armv7l \
        --python-version 3.11 --implementation cp --abi cp311 \
        --dest /tmp/lxmldl \
        "lxml==$LXML_VERSION"
    unzip -q -o /tmp/lxmldl/lxml-*.whl -d "$SITE_PACKAGES"

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
    rm -rf /tmp/bs4dl && mkdir -p /tmp/bs4dl
    pip3 download --only-binary=:all: --python-version 3.11 --no-deps --dest /tmp/bs4dl beautifulsoup4 2>/dev/null
    unzip -q -o /tmp/bs4dl/beautifulsoup4*.whl -d "$SITE_PACKAGES"

    touch "$CACHE_STAMP"
    echo "  Cached to $CACHE_DIR/$CACHE_KEY/"

    # Copy to output
    cp -a "$CACHE_DIR/$CACHE_KEY/dist" "$OUTPUT_DIR/dist"
fi

DIST_DIR="$OUTPUT_DIR/dist"
SITE_PACKAGES="$DIST_DIR/lib/python3.11/site-packages"

# ---------------------------------------------------------------------------
# Step 3: Copy plugin Python source into dist
# ---------------------------------------------------------------------------
echo "[3/5] Copying plugin source..."

cp python/kindle_helper.py "$DIST_DIR/kindle_helper.py"
cp python/epub_position.py "$DIST_DIR/epub_position.py"
cp python/kfx_position_map.py "$DIST_DIR/kfx_position_map.py"
cp python/kfx_position_adapter.py "$DIST_DIR/kfx_position_adapter.py"
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
rm -rf "$DIST_DIR/bin/python"             # duplicate binary
mv "$DIST_DIR/bin/python3.11" "$DIST_DIR/bin/python3"
rm -f "$DIST_DIR/bin/python3-config"
rm -f "$DIST_DIR/bin/python3.11-config"
rm -rf "$DIST_DIR/lib/libpython3.11.so"*          # shared lib (28MB, not needed)
rm -rf "$DIST_DIR/lib/libpython3.so"             # linker stub
rm -rf "$DIST_DIR/lib/pkgconfig"                 # build metadata
rm -rf "$DIST_DIR/lib/tcl9"                      # Tcl runtime
rm -rf "$DIST_DIR/lib/tcl9.0"                    # Tcl runtime
rm -rf "$DIST_DIR/lib/tk9.0"                     # Tk runtime
rm -rf "$DIST_DIR/lib/itcl"*                     # Tcl extension
rm -rf "$DIST_DIR/lib/thread"*                   # Tcl extension
rm -f "$DIST_DIR/lib/libtcl"*                    # Tcl/Tk .so
rm -f "$DIST_DIR/lib/libtcl9"*                   # Tcl .so
rm -rf "$DIST_DIR/lib/python3.11/idlelib" # IDE
rm -rf "$DIST_DIR/lib/python3.11/tkinter" # Tk
rm -f "$DIST_DIR/lib/python3.11/lib-dynload/_tkinter"*.so # Tk extension
rm -rf "$DIST_DIR/lib/python3.11/test"    # test suite
rm -rf "$DIST_DIR/lib/python3.11/unittest" # test framework
rm -rf "$DIST_DIR/lib/python3.11/pydoc_data" # docs
rm -rf "$DIST_DIR/lib/python3.11/ensurepip"  # pip bundler
rm -rf "$DIST_DIR/lib/python3.11/lib2to3"   # 2to3 converter
rm -rf "$DIST_DIR/lib/python3.11/turtle.py"  # turtle graphics
rm -rf "$DIST_DIR/lib/python3.11/telnetlib.py"
rm -rf "$DIST_DIR/lib/python3.11/asyncio"    # async framework
find "$DIST_DIR/lib/python3.11" -name "tests" -exec rm -rf {} + 2>/dev/null || true
find "$DIST_DIR/lib/python3.11" -name "test" -type d -exec rm -rf {} + 2>/dev/null || true

# Strip debug symbols from the Python binary (27MB -> ~7MB)
docker run --rm --platform linux/arm/v7 -v "$(cd "$DIST_DIR" && pwd)/bin:/mnt" arm32v7/gcc:12 strip /mnt/python3

# Extract shared libs needed by Pillow (not present on Kindle).
# Keep these and the glibc runtime below from the same armhf image so the
# bundled loader can run independently of the Kindle firmware ABI.
echo "  Bundling shared libs for Pillow..."
mkdir -p "$DIST_DIR/lib/external"
docker run --rm --platform linux/arm/v7 -v "$(cd "$DIST_DIR" && pwd)/lib/external:/out" arm32v7/gcc:12 bash -c '
for lib in libLerc.so.4 libXau.so.6 libXdmcp.so.6 libbrotlicommon.so.1 libbrotlidec.so.1 libbsd.so.0 libdeflate.so.0 libfreetype.so.6 libjbig.so.0 libjpeg.so.62 liblcms2.so.2 liblzma.so.5 libmd.so.0 libopenjp2.so.7 libpng16.so.16 libtiff.so.6 libwebp.so.7 libwebpdemux.so.2 libwebpmux.so.3 libxcb.so.1 libz.so.1 libzstd.so.1; do
    cp -L /lib/arm-linux-gnueabihf/$lib /out/ 2>/dev/null || true
done
'

# Kindle firmware through 5.16.2 uses a softfp userspace and therefore lacks
# /lib/ld-linux-armhf.so.3.  Bundle the armhf loader and its core runtime so
# kindle-helper can invoke our hard-float CPython explicitly on both old and
# new firmware.  The static launcher itself already runs on the older devices.
echo "  Bundling hard-float runtime..."
mkdir -p "$DIST_DIR/lib/runtime"
docker run --rm --platform linux/arm/v7 -v "$(cd "$DIST_DIR" && pwd)/lib/runtime:/out" arm32v7/gcc:12 bash -c '
set -e
cp -L /lib/ld-linux-armhf.so.3 /out/ld-linux-armhf.so.3
for lib in libc.so.6 libdl.so.2 libm.so.6 libpthread.so.0 librt.so.1 libutil.so.1; do
    cp -L /lib/arm-linux-gnueabihf/$lib /out/$lib
done
# The piwheels Pillow build needs the GCC C++ runtime as well.
cp -L /lib/arm-linux-gnueabihf/libgcc_s.so.1 /out/libgcc_s.so.1
cp -L /usr/lib/arm-linux-gnueabihf/libstdc++.so.6 /out/libstdc++.so.6
'

# Strip unnecessary Crypto modules
rm -rf "$SITE_PACKAGES/Crypto/SelfTest"
rm -rf "$SITE_PACKAGES/Crypto/IO"

# Strip pip and setuptools from site-packages (build tools only)
rm -rf "$SITE_PACKAGES/pip"
rm -rf "$SITE_PACKAGES/pip"*.dist-info
rm -rf "$SITE_PACKAGES/setuptools"
rm -rf "$SITE_PACKAGES/setuptools"*.dist-info
rm -rf "$SITE_PACKAGES/_distutils_hack"
rm -f "$SITE_PACKAGES/distutils-precedence.pth"

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

# Build crypto_hook.so against old glibc (Bullseye = glibc 2.31)
# This .so is LD_PRELOADed into the Kindle's JVM, so it must not
# require glibc symbols newer than what older Kindle devices ship.
echo "  Building crypto_hook.so (old glibc)..."
CRYPTO_HOOK_TAG="kindle-crypto-hook-builder"

docker buildx build \
    --platform linux/arm/v7 \
    -t "$CRYPTO_HOOK_TAG" \
    -f .github/Dockerfile.crypto_hook \
    --load \
    .

CRYPTO_CID=$(docker create "$CRYPTO_HOOK_TAG")
docker cp "$CRYPTO_CID:/build/crypto_hook.so" "$OUTPUT_DIR/crypto_hook.so"
docker rm "$CRYPTO_CID"

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
cp -r bin/ "$STAGING/bin/"

# Copy the C wrapper
cp "$OUTPUT_DIR/kindle-helper" "$STAGING/"
cp "$OUTPUT_DIR/libsyscall_wrapper.so" "$STAGING/"

# Copy the DRM helpers (crypto hook, Java jar) into dist/lib/
# Python resolves plugin_dir as dist/ (where kindle_helper.py lives)
mkdir -p "$STAGING/dist/lib"
cp "$OUTPUT_DIR/crypto_hook.so" "$STAGING/dist/lib/"
cp lib/KFXVoucherExtractor.jar "$STAGING/dist/lib/"

# Copy the Python runtime contents into the existing dist/ directory. The
# directory already contains DRM helper assets, so copying DIST_DIR itself
# would incorrectly create dist/dist/ and break the launcher paths.
cp -a "$DIST_DIR/." "$STAGING/dist/"

# Fail the build if the package no longer matches Dockerfile.wrapper's paths.
test -x "$STAGING/dist/bin/python3"
test -f "$STAGING/dist/kindle_helper.py"
test -f "$STAGING/dist/epub_position.py"
test -f "$STAGING/dist/kfx_position_adapter.py"
test -f "$STAGING/dist/dedrm/native_extractor.py"
test -x "$STAGING/bin/sync-native-progress"
test -f "$STAGING/bin/native-reading-progress-agent-v6.jar"
test -x "$STAGING/dist/lib/runtime/ld-linux-armhf.so.3"
test -f "$STAGING/dist/lib/runtime/libc.so.6"
test ! -d "$STAGING/dist/dist"

# Exercise the final package in a scratch rootfs.  There is deliberately no
# system /lib/ld-linux-armhf.so.3 here, reproducing the important constraint
# of pre-5.16.3 Kindle firmware.  Success proves the launcher uses only the
# bundled loader/runtime rather than the host firmware ABI.
echo "  Testing self-contained runtime..."
SMOKE_DOCKERFILE="$OUTPUT_DIR/Dockerfile.runtime-smoke"
cat > "$SMOKE_DOCKERFILE" <<'EOF'
FROM scratch
COPY kindle.koplugin /plugin
ENTRYPOINT ["/plugin/kindle-helper"]
EOF
docker buildx build \
    --platform linux/arm/v7 \
    -t kindle-runtime-smoke \
    -f "$SMOKE_DOCKERFILE" \
    --load \
    "$OUTPUT_DIR" >/dev/null
docker run --rm --platform linux/arm/v7 kindle-runtime-smoke --help | grep -q 'kindle-helper'
docker run --rm --platform linux/arm/v7 \
    --entrypoint /plugin/dist/lib/runtime/ld-linux-armhf.so.3 \
    kindle-runtime-smoke \
    --library-path /plugin/dist/lib/runtime:/plugin/dist/lib/external \
    /plugin/dist/bin/python3 -c \
    'import lxml.etree; from PIL import Image; from Crypto.Cipher import AES; print("native imports ok")' \
    | grep -q 'native imports ok'
rm -f "$SMOKE_DOCKERFILE"

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
