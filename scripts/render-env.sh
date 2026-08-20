#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${TELEANTISPAM_ENV_OUTPUT:-}" ]]; then
  echo "missing TELEANTISPAM_ENV_OUTPUT" >&2
  exit 1
fi

while IFS= read -r line; do
  key="${line%%=*}"
  case "$line" in
    *=*)
      if [[ -n "${!key+x}" ]]; then
        printf '%s=%s\n' "$key" "${!key}"
      else
        printf '%s\n' "$line"
      fi
      ;;
    *)
      printf '%s\n' "$line"
      ;;
  esac
done < deploy/teleantispam.env.example > "$TELEANTISPAM_ENV_OUTPUT"
