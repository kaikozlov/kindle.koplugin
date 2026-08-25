# justfile for kindle.koplugin
#
# Shared recipes are vendored from koplugin-dev (just/shared.just).
# No local toolchain required — just Docker (and `just`).
#
# Quick start:
#   just setup        # install git hooks and pull the image (one-time)
#   just verify       # static checks, Lua/Python/Java tests, ARM DRM hook matrix
#   just test         # run Lua tests (quiet; V=1 for verbose)
#   just test-python  # run Python/Java tests in the derived test container
#   just build        # build the ARMv7 release zip
#   just shell        # drop into the container
#
# When shared recipes change upstream:
#   just sync-shared   # refresh just/shared.just (then commit)

plugin_name := "kindle"
koplugin_dev_version := "v2026.07.1_1"
# Git ref used by `just sync-shared` (recipe source). Independent of the image pin.
koplugin_dev_ref := env("KOPLUGIN_DEV_REF", "main")
plugin_path := "/opt/plugin"
spec_dir := "spec"
lua_paths := "_meta.lua main.lua lua spec"
has_go := "0"
go_integration_packages := ""
exclude_tags := "e2e"

import "./just/shared.just"

# =============================================================================
# Derived test image (KOReader runtime + Python/Java toolchains)
# =============================================================================

# Tag of the derived image that adds Python and JDK toolchains to the pinned
# koplugin-dev image so Lua, Python, and Java tests share one container.
[private]
_test_image := "kindle-koplugin-test:" + koplugin_dev_version

# Container prefix for the derived test image. Reuses the shared mount/env so
# plugin discovery and headless KOReader behave exactly like the base image.
[private]
_test_run := if _in_container == "1" { "" } else { "docker run --rm " + _sdl_env + " " + _mount + " " + _test_image }

# Build the derived test image from the pinned koplugin-dev image.
# A future pin changes koplugin_dev_version above and nothing else.
[group('setup')]
test-image:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "$(docker images -q '{{ _test_image }}' 2>/dev/null)" ]; then
        docker build \
            --build-arg KOPLUGIN_DEV_IMAGE='{{ _image }}' \
            -t '{{ _test_image }}' \
            -f Dockerfile.test \
            .
    else
        echo "{{ _test_image }} already exists (rebuild: docker rmi '{{ _test_image }}')"
    fi

# =============================================================================
# Canonical verification
# =============================================================================

# Read-only static checks suitable for pre-commit.
[group('lint')]
verify-static:
    {{ _run }} {{ _reenter }} _verify_static

[private]
_verify_static: fmt-check lint

# Definitive local/CI verification: static checks, all non-e2e Lua specs on the
# real KOReader runtime, the Python/Java suite, and the ARM DRM hook matrix.
# The ARM hook targets stay host-side because the test container has no Docker.
[group('test')]
verify: test-drm-hook test-image
    {{ _test_run }} {{ _reenter }} _verify

[private]
_verify: _verify_static test _test-python

# =============================================================================
# Testing (plugin-local)
# =============================================================================

# Run one exact spec file, e.g. just test-file spec/virtual_library_spec.lua
[group('test')]
test-file path:
    #!/usr/bin/env bash
    set -euo pipefail
    label="Running Lua tests ({{ path }})"
    cmd='{{ _run }} busted-koreader {{ _busted_opts }} --helper={{ _commonrequire }} /opt/plugin/{{ path }}'
    echo "$label"
    if [ "{{ _v }}" = "1" ]; then
        eval "$cmd"
    else
        out="$(mktemp)"
        if eval "$cmd" >"$out" 2>&1; then
            grep -E '^[0-9]+ success' "$out" || tail -n 3 "$out"
            rm -f "$out"
        else
            echo "$label failed — full output:" >&2
            cat "$out" >&2
            rm -f "$out"
            exit 1
        fi
    fi

# Run the Python/Java test suite in the derived test container.
[group('test')]
test-python:
    {{ _test_run }} {{ _reenter }} _test-python

[private]
_test-python:
    cd /opt/plugin && python3 -m unittest discover -s python/tests -p 'test_*.py' -v

# Build and execute the ARM DRM hook matrix against the two OpenSSL
# generations shipped across supported Kindle firmware. Hermetic: QEMU
# emulates linux/arm/v7, exercising the exact shipped crypto_hook.so ELF.
[group('test')]
test-drm-hook:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! docker run --rm --platform linux/arm/v7 arm32v7/debian:bullseye true >/dev/null 2>&1; then
        echo "error: cannot execute linux/arm/v7 containers." >&2
        echo "Install QEMU binfmt first (Docker Desktop includes it; Linux uses docker/setup-qemu-action)." >&2
        exit 1
    fi
    build() {
        echo "Building ARM DRM hook target: $1"
        docker buildx build --platform linux/arm/v7 --target "$1" -f .github/Dockerfile.crypto_hook .
    }
    build test-openssl11
    build test-openssl3
    echo "ARM DRM hook matrix passed (OpenSSL 1.1 + OpenSSL 3)"

# =============================================================================
# Setup (plugin-local)
# =============================================================================

# Refresh just/shared.just from upstream koplugin-dev
[group('setup')]
sync-shared:
    #!/usr/bin/env bash
    set -euo pipefail
    ref="{{ koplugin_dev_ref }}"
    mkdir -p just
    tmp="$(mktemp)"
    url="https://raw.githubusercontent.com/kaikozlov/koplugin-dev/${ref}/shared.just"
    echo "Fetching ${url}"
    curl -fsSL "$url" -o "$tmp"
    {
        echo "# Vendored from https://github.com/kaikozlov/koplugin-dev"
        echo "# Ref: ${ref}"
        echo "# Refresh with: just sync-shared"
        echo
        cat "$tmp"
    } > just/shared.just
    rm -f "$tmp"
    echo "Updated just/shared.just from koplugin-dev@${ref}"

# =============================================================================
# Build (product-specific)
# =============================================================================

# Rebuild the DRM voucher extractor JAR (JDK 8+)
[group('build')]
build-voucher:
    ./scripts/build_voucher_extractor

# Build the self-contained ARMv7 release package (python_build.sh)
[group('build')]
build:
    ./python_build.sh
