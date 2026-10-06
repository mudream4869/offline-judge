#!/usr/bin/env bash
# Build the static site into dist/ (or `serve` it at localhost:3000).
#   scripts/build.sh [build|serve]
set -euo pipefail

PYODIDE_VERSION=314.0.7
CLANG_VERSION=22.0.0-git20542-10
WASI_SHIM_VERSION=0.4.2
MODE=${1:-build}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CACHE=$ROOT/.cache
ASSETS=$CACHE/assets

cd "$ROOT"

# Self-host runtimes (npm packages), so the site needs no CDN and works offline.
#   fetch <package@version> <dir>
fetch() {
  if [ ! -f "$2/package.json" ]; then
    mkdir -p "$2"
    tgz=$(cd "$CACHE" && npm pack --silent "$1")
    tar -xzf "$CACHE/$tgz" -C "$2" --strip-components=1
    rm "$CACHE/$tgz"
  fi
}
PYO=$CACHE/pyodide-$PYODIDE_VERSION
CLANG=$CACHE/clang-$CLANG_VERSION
WASI=$CACHE/wasi-shim-$WASI_SHIM_VERSION
fetch "pyodide@$PYODIDE_VERSION" "$PYO"
fetch "@yowasp/clang@$CLANG_VERSION" "$CLANG"
fetch "@bjorn3/browser_wasi_shim@$WASI_SHIM_VERSION" "$WASI"

rm -rf "$ASSETS"
mkdir -p "$ASSETS/pyodide" "$ASSETS/clang" "$ASSETS/wasi"
cp web/* "$ASSETS/"
for f in pyodide.mjs pyodide.asm.mjs pyodide.asm.wasm python_stdlib.zip pyodide-lock.json; do
  cp "$PYO/$f" "$ASSETS/pyodide/"
done
cp "$CLANG"/gen/*.js "$CLANG"/gen/*.wasm "$CLANG"/gen/*.tar "$ASSETS/clang/"
cp "$WASI"/dist/*.js "$ASSETS/wasi/"

# Precompiled <bits/stdc++.h>, rebuilt when the header, flags or clang change.
key=$( (cat web/stdc++.h web/cppflags.mjs; echo "$CLANG_VERSION") | sha256sum | cut -c1-16)
PCH=$CACHE/stdc++-$key.pch
if [ ! -f "$PCH" ]; then
  node scripts/mkpch.mjs "$CLANG/gen/bundle.js" web/stdc++.h "$PCH.tmp"
  mv "$PCH.tmp" "$PCH"
fi
cp "$PCH" "$ASSETS/stdc++.h.pch"

go tool toolgui-wasm "$MODE" -o dist -ldflags "-s -w" -assets "$ASSETS" -offline ./cmd/offline-judge
