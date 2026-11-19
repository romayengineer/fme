#!/usr/bin/env bash
#
# Print the workspace modules (repo-relative directory per line), in the order
# go.work declares them. This is the single source of truth used by the justfile
# and CI so adding a module does not require editing several hardcoded lists.
set -euo pipefail

# PocketBase v0.21.3 crashes under Go 1.27; match the justfile/CI toolchain.
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.25.14}"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR" || exit 1

go list -m -f '{{.Dir}}' | sed "s|^$ROOT_DIR/||"
