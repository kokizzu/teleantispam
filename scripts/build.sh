#!/usr/bin/env bash
set -euo pipefail

mkdir -p bin
CGO_ENABLED=0 "${GO:-go}" build -trimpath -ldflags "${LDFLAGS:-}" -o "bin/${BINARY:-teleantispam}" ./cmd/teleantispam
