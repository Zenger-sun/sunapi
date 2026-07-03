#!/usr/bin/env bash
# ============================================================
# SunAPI Windows Build Script
# 输出目录: <ROOT>/build/
# 用法: bash scripts/build-windows.sh [版本号]
#       bash scripts/build-windows.sh 1.2.0
# ============================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$ROOT_DIR/build}"
FRONT_DIR="$ROOT_DIR/front"
BACKEND_DIR="$ROOT_DIR/backend"
SUNSTATIC_DIR="$BACKEND_DIR/internal/sunstatic/dist"

VERSION="${1:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo 'none')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
OUTPUT_NAME="sunapi_x86.exe"
OUTPUT_PATH="$BUILD_DIR/$OUTPUT_NAME"

echo "========================================"
echo "  SunAPI Windows Build"
echo "  Version:    $VERSION"
echo "  Commit:     $COMMIT"
echo "  Build Date: $BUILD_DATE"
echo "  Output:     $OUTPUT_PATH"
echo "========================================"

# ── 1. 构建前端 ─────────────────────────────────────────────
echo ""
echo "[1/3] Building frontend..."
cd "$FRONT_DIR"
node node_modules/@rsbuild/core/bin/rsbuild.js build
echo "  ✅ Frontend built -> front/dist"

# ── 2. 同步前端产物到 sunstatic/dist ───────────────────────
echo ""
echo "[2/3] Syncing frontend dist -> sunstatic/dist..."

# 先清理旧的 js/css（带哈希的文件），避免残留
rm -f "$SUNSTATIC_DIR/static/js/"*.js 2>/dev/null || true
rm -f "$SUNSTATIC_DIR/static/css/"*.css 2>/dev/null || true

# 复制新产物
cp -r "$FRONT_DIR/dist/." "$SUNSTATIC_DIR/"
echo "  ✅ Synced to $SUNSTATIC_DIR"

# ── 3. 编译 Go 后端 ──────────────────────────────────────────
echo ""
echo "[3/3] Compiling backend -> $OUTPUT_PATH..."
mkdir -p "$BUILD_DIR"

LDFLAGS="-s -w \
  -X main.Version=$VERSION \
  -X main.Commit=$COMMIT \
  -X main.BuildDate=$BUILD_DATE"

cd "$BACKEND_DIR"
GOOS=windows GOARCH=amd64 go build \
  -trimpath \
  -ldflags "$LDFLAGS" \
  -o "$OUTPUT_PATH" \
  ./cmd/sunapi/...

echo "  ✅ Built: $OUTPUT_PATH"

# ── 完成 ─────────────────────────────────────────────────────
echo ""
echo "========================================"
SIZE=$(du -sh "$OUTPUT_PATH" | cut -f1)
echo "  Build complete!"
echo "  File: $OUTPUT_PATH ($SIZE)"
echo "========================================"
