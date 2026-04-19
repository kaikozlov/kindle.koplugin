#!/bin/bash
set -euo pipefail

# Ensure Go module dependencies are downloaded
cd "$(git rev-parse --show-toplevel)"
go mod download

echo "Environment ready."
