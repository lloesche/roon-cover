#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p build
CGO_ENABLED=0 go build -o build/roon-cover ./cmd/roon-cover
