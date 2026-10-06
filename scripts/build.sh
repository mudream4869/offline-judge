#!/usr/bin/env bash
# Build the static site into dist/ (or `serve` it at localhost:3000).
#   scripts/build.sh [build|serve]
set -euo pipefail

PYODIDE_VERSION=314.0.7
MODE=${1:-build}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CACHE=$ROOT/.cache
ASSETS=$CACHE/assets

cd "$ROOT"

# Self-host Pyodide (npm package), so the site needs no CDN and works offline.
PYO=$CACHE/pyodide-$PYODIDE_VERSION
if [ ! -f "$PYO/pyodide.asm.wasm" ]; then
  mkdir -p "$PYO"
  tgz=$(cd "$CACHE" && npm pack --silent "pyodide@$PYODIDE_VERSION")
  tar -xzf "$CACHE/$tgz" -C "$PYO" --strip-components=1
  rm "$CACHE/$tgz"
fi

rm -rf "$ASSETS"
mkdir -p "$ASSETS/pyodide"
cp web/* "$ASSETS/"
for f in pyodide.mjs pyodide.asm.mjs pyodide.asm.wasm python_stdlib.zip pyodide-lock.json; do
  cp "$PYO/$f" "$ASSETS/pyodide/"
done

go tool toolgui-wasm "$MODE" -o dist -ldflags "-s -w" -assets "$ASSETS" -offline ./cmd/offline-judge
