#!/bin/sh
set -eu

APP_NAME="${APP_NAME:-SunAPI}"
BUNDLE_ID="${BUNDLE_ID:-com.sunapi.console}"
VERSION="${VERSION:-1.0.0}"
ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$ROOT_DIR/build}"
APP_DIR="$BUILD_DIR/$APP_NAME.app"
STAGE_DIR="$BUILD_DIR/dmg-stage"
DIST_DIR="$BUILD_DIR/dist"
ARCH="${ARCH:-$(uname -m)}"
DMG_PATH="$DIST_DIR/$APP_NAME-$VERSION-$ARCH.dmg"
TMP_DMG_PATH="$DIST_DIR/.$APP_NAME-$VERSION-$ARCH.tmp.dmg"
KEEP_APP="${KEEP_APP:-0}"
SKIP_DMG="${SKIP_DMG:-0}"
LAUNCHER_TEMPLATE="$ROOT_DIR/scripts/macos/SunAPI.launcher"
PLIST_TEMPLATE="$ROOT_DIR/scripts/macos/Info.plist"
ICON_SOURCE="${ICON_SOURCE:-$ROOT_DIR/front/public/logo.png}"
LEGACY_ICONSET_DIR="${ICONSET_DIR:-$ROOT_DIR/build/icon.iconset}"
ICNS_PATH="$APP_DIR/Contents/Resources/SunAPI.icns"

cleanup_intermediates() {
  rm -f "$TMP_DMG_PATH"
  if [ "$KEEP_APP" != "1" ]; then
    rm -rf "$APP_DIR" "$STAGE_DIR"
  fi
}

trap cleanup_intermediates EXIT

find_go() {
  if [ -n "${GO_BIN:-}" ]; then
    printf '%s\n' "$GO_BIN"
    return
  fi
  if command -v go >/dev/null 2>&1; then
    command -v go
    return
  fi
  if [ -x "$ROOT_DIR/.tools/go/bin/go" ]; then
    printf '%s\n' "$ROOT_DIR/.tools/go/bin/go"
    return
  fi
  echo "go was not found. Set GO_BIN or install Go." >&2
  exit 1
}

install_app_icon() {
  if [ -f "$ICON_SOURCE" ]; then
    if ! command -v sips >/dev/null 2>&1; then
      echo "sips was not found; app icon will be omitted." >&2
      return
    fi
    sips -s format icns "$ICON_SOURCE" --out "$ICNS_PATH" >/dev/null
    return
  fi

  if [ -d "$LEGACY_ICONSET_DIR" ]; then
    if ! command -v iconutil >/dev/null 2>&1; then
      echo "iconutil was not found; app icon will be omitted." >&2
      return
    fi
    iconutil -c icns "$LEGACY_ICONSET_DIR" -o "$ICNS_PATH"
    return
  fi

  echo "No icon source found; app icon will be omitted." >&2
}

validate_app_bundle() {
  info_plist="$APP_DIR/Contents/Info.plist"
  if command -v plutil >/dev/null 2>&1; then
    plutil -lint "$info_plist" >/dev/null
    package_type="$(plutil -extract CFBundlePackageType raw -o - "$info_plist" 2>/dev/null || true)"
    display_name="$(plutil -extract CFBundleDisplayName raw -o - "$info_plist" 2>/dev/null || true)"
    bundle_id="$(plutil -extract CFBundleIdentifier raw -o - "$info_plist" 2>/dev/null || true)"
    bundle_version="$(plutil -extract CFBundleVersion raw -o - "$info_plist" 2>/dev/null || true)"
    ls_ui_element="$(plutil -extract LSUIElement raw -o - "$info_plist" 2>/dev/null || true)"
    ls_background_only="$(plutil -extract LSBackgroundOnly raw -o - "$info_plist" 2>/dev/null || true)"
  else
    package_type="$(/usr/libexec/PlistBuddy -c 'Print :CFBundlePackageType' "$info_plist" 2>/dev/null || true)"
    display_name="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleDisplayName' "$info_plist" 2>/dev/null || true)"
    bundle_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$info_plist" 2>/dev/null || true)"
    bundle_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$info_plist" 2>/dev/null || true)"
    ls_ui_element="$(/usr/libexec/PlistBuddy -c 'Print :LSUIElement' "$info_plist" 2>/dev/null || true)"
    ls_background_only="$(/usr/libexec/PlistBuddy -c 'Print :LSBackgroundOnly' "$info_plist" 2>/dev/null || true)"
  fi

  if [ "$package_type" != "APPL" ] || [ -z "$display_name" ] || [ -z "$bundle_id" ] || [ -z "$bundle_version" ]; then
    echo "Invalid app bundle metadata: SunAPI must be packaged as a normal macOS app." >&2
    exit 1
  fi
  if [ -n "$ls_ui_element" ] || [ -n "$ls_background_only" ]; then
    echo "Invalid app bundle metadata: do not set LSUIElement or LSBackgroundOnly." >&2
    exit 1
  fi
}

install_info_plist_strings() {
  for locale in en zh-Hans zh_CN; do
    locale_dir="$APP_DIR/Contents/Resources/$locale.lproj"
    mkdir -p "$locale_dir"
    {
      printf '%s\n' '"CFBundleDisplayName" = "SunAPI";'
      printf '%s\n' '"CFBundleName" = "SunAPI";'
    } > "$locale_dir/InfoPlist.strings"
  done
}

GO_BIN="$(find_go)"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'none')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
BUILD_NUMBER="${BUILD_NUMBER:-$VERSION}"
LDFLAGS="-s -w -X main.Version=$VERSION -X main.Commit=$COMMIT -X main.BuildDate=$BUILD_DATE"

rm -rf "$APP_DIR" "$STAGE_DIR"
if [ "$SKIP_DMG" != "1" ]; then
  rm -f "$TMP_DMG_PATH"
fi
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources/docs" "$DIST_DIR" "$STAGE_DIR"

printf 'Building %s %s for %s...\n' "$APP_NAME" "$VERSION" "$ARCH"
(
  cd "$ROOT_DIR/backend"
  "$GO_BIN" build -trimpath -ldflags "$LDFLAGS" -o "$APP_DIR/Contents/MacOS/sunapi-server" ./cmd/sunapi
)
(
  cd "$ROOT_DIR"
  CGO_ENABLED=1 "$GO_BIN" build -trimpath -ldflags "-s -w" -o "$APP_DIR/Contents/MacOS/SunAPI" ./scripts/macos/launcher-stub.go
)

install -m 755 "$LAUNCHER_TEMPLATE" "$APP_DIR/Contents/MacOS/SunAPI.launcher"
sed \
  -e "s/@VERSION@/$VERSION/g" \
  -e "s/@BUILD_NUMBER@/$BUILD_NUMBER/g" \
  -e "s/@BUNDLE_ID@/$BUNDLE_ID/g" \
  "$PLIST_TEMPLATE" > "$APP_DIR/Contents/Info.plist"
printf 'APPL????' > "$APP_DIR/Contents/PkgInfo"
install -m 644 "$ROOT_DIR/README.md" "$APP_DIR/Contents/Resources/docs/README.md"
install -m 644 "$ROOT_DIR/LICENSE" "$APP_DIR/Contents/Resources/docs/LICENSE"

install_app_icon
install_info_plist_strings

validate_app_bundle

if command -v codesign >/dev/null 2>&1; then
  codesign --force --deep --sign - "$APP_DIR"
fi

if [ "$SKIP_DMG" != "1" ]; then
  if command -v ditto >/dev/null 2>&1; then
    COPYFILE_DISABLE=1 ditto --norsrc --noextattr --noqtn --noacl "$APP_DIR" "$STAGE_DIR/$APP_NAME.app"
  else
    COPYFILE_DISABLE=1 cp -R "$APP_DIR" "$STAGE_DIR/"
  fi
  ln -s /Applications "$STAGE_DIR/Applications"
  hdiutil create -volname "$APP_NAME" -srcfolder "$STAGE_DIR" -ov -format UDZO "$TMP_DMG_PATH"
  mv "$TMP_DMG_PATH" "$DMG_PATH"
fi

if [ "$KEEP_APP" = "1" ]; then
  printf 'App: %s\n' "$APP_DIR"
else
  printf 'Cleaned intermediate app and staging directory.\n'
fi
if [ "$SKIP_DMG" != "1" ]; then
  printf 'DMG: %s\n' "$DMG_PATH"
fi
