#!/usr/bin/env bash
set -euo pipefail

"${GO:-go}" run ./cmd/teleantispam
