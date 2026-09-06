#!/usr/bin/env bash
set -euo pipefail

git status --short --branch
git diff --stat
git diff --check
git diff
git diff --cached
