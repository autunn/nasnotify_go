#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-NasNotify-Go}"
BUNDLE_ID="${BUNDLE_ID:-com.autunn.nasnotify-go}"
EXECUTABLE_NAME="NasNotifyGo"
VERSION="${VERSION:-v$(date +'%Y.%m.%d')}"
MARKETING_VERSION="${MARKETING_VERSION:-${VERSION#v}}"
MARKETING_VERSION="${MARKETING_VERSION%%-*}"
BUILD_VERSION="${BUILD_VERSION:-${GITHUB_RUN_NUMBER:-$(date +'%Y%m%d%H%M')}}"
SIGN_AND_NOTARIZE="${SIGN_AND_NOTARIZE:-0}"
CREATE_DMG="${CREATE_DMG:-1}"
NOTARIZE_DMG="${NOTARIZE_DMG:-1}"

BUILD_DIR="$ROOT_DIR/build/macos"
DIST_DIR="$ROOT_DIR/dist"
APP_DIR="$DIST_DIR/$APP_NAME.app"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "macOS app builds must run on macOS." >&2
  exit 1
fi

case "${GOARCH:-$(uname -m)}" in
  arm64) GOARCH="arm64" ;;
  amd64|x86_64) GOARCH="amd64" ;;
  *) echo "Unsupported macOS architecture: ${GOARCH:-$(uname -m)}" >&2; exit 1 ;;
esac

if [[ ! "$MARKETING_VERSION" =~ ^[0-9]+(\.[0-9]+){0,2}$ ]]; then
  echo "MARKETING_VERSION must look like 1, 1.2 or 1.2.3. Current: $MARKETING_VERSION" >&2
  exit 1
fi

if [[ ! "$BUILD_VERSION" =~ ^[0-9]+(\.[0-9]+){0,2}$ ]]; then
  echo "BUILD_VERSION must look like 1, 1.2 or 1.2.3. Current: $BUILD_VERSION" >&2
  exit 1
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

require_notary_credentials() {
  if [[ -n "${APPLE_NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
    return
  fi

  if [[ -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" && -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
    return
  fi

  if [[ -n "${APP_STORE_CONNECT_API_KEY_PATH:-}" && -n "${APP_STORE_CONNECT_KEY_ID:-}" && -n "${APP_STORE_CONNECT_ISSUER_ID:-}" ]]; then
    return
  fi

  cat >&2 <<'ERROR'
Missing notarization credentials. Provide one of:
  APPLE_NOTARY_KEYCHAIN_PROFILE
  APPLE_ID + APPLE_TEAM_ID + APPLE_APP_SPECIFIC_PASSWORD
  APP_STORE_CONNECT_API_KEY_PATH + APP_STORE_CONNECT_KEY_ID + APP_STORE_CONNECT_ISSUER_ID
ERROR
  exit 1
}

submit_for_notarization() {
  local target="$1"

  if [[ -n "${APPLE_NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
    xcrun notarytool submit "$target" \
      --keychain-profile "$APPLE_NOTARY_KEYCHAIN_PROFILE" \
      --wait
    return
  fi

  if [[ -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" && -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]]; then
    xcrun notarytool submit "$target" \
      --apple-id "$APPLE_ID" \
      --team-id "$APPLE_TEAM_ID" \
      --password "$APPLE_APP_SPECIFIC_PASSWORD" \
      --wait
    return
  fi

  xcrun notarytool submit "$target" \
    --key "$APP_STORE_CONNECT_API_KEY_PATH" \
    --key-id "$APP_STORE_CONNECT_KEY_ID" \
    --issuer "$APP_STORE_CONNECT_ISSUER_ID" \
    --wait
}

codesign_item() {
  local target="$1"
  shift

  codesign \
    --force \
    --timestamp \
    --options runtime \
    --sign "$MACOS_CODESIGN_IDENTITY" \
    "$@" \
    "$target"
}

sign_app_bundle() {
  if [[ -z "${MACOS_CODESIGN_IDENTITY:-}" ]]; then
    echo "MACOS_CODESIGN_IDENTITY is required when SIGN_AND_NOTARIZE=1." >&2
    exit 1
  fi

  require_notary_credentials
  require_command codesign
  require_command xcrun

  echo "==> Sign nested executables"
  codesign_item "$APP_DIR/Contents/Resources/nasnotify-go-app" --identifier "$BUNDLE_ID.backend"
  codesign_item "$APP_DIR/Contents/MacOS/$EXECUTABLE_NAME"

  echo "==> Sign app bundle with Developer ID"
  codesign_item "$APP_DIR"
  codesign --verify --strict --deep --verbose=2 "$APP_DIR"

  echo "==> Notarize app bundle"
  rm -f "$BUILD_DIR/$APP_NAME-macOS-$GOARCH.zip"
  ditto -c -k --keepParent "$APP_DIR" "$BUILD_DIR/$APP_NAME-macOS-$GOARCH.zip"
  submit_for_notarization "$BUILD_DIR/$APP_NAME-macOS-$GOARCH.zip"

  echo "==> Staple app notarization ticket"
  xcrun stapler staple "$APP_DIR"
  xcrun stapler validate "$APP_DIR"
  spctl --assess --type execute --verbose=4 "$APP_DIR"
}

create_dmg() {
  local dmg_root="$BUILD_DIR/dmgroot"
  local dmg_path="$DIST_DIR/$APP_NAME-macOS-$GOARCH.dmg"

  echo "==> Build DMG"
  rm -rf "$dmg_root" "$dmg_path"
  mkdir -p "$dmg_root"
  cp -R "$APP_DIR" "$dmg_root/"
  ln -s /Applications "$dmg_root/Applications"

  hdiutil create \
    -volname "$APP_NAME" \
    -srcfolder "$dmg_root" \
    -ov \
    -format UDZO \
    "$dmg_path"

  if [[ "$SIGN_AND_NOTARIZE" == "1" ]]; then
    echo "==> Sign DMG"
    codesign --force --timestamp --sign "$MACOS_CODESIGN_IDENTITY" "$dmg_path"
    codesign --verify --verbose=2 "$dmg_path"

    if [[ "$NOTARIZE_DMG" == "1" ]]; then
      echo "==> Notarize DMG"
      submit_for_notarization "$dmg_path"
      xcrun stapler staple "$dmg_path"
      xcrun stapler validate "$dmg_path"
      spctl --assess --type open --context context:primary-signature --verbose=4 "$dmg_path"
    fi
  fi

  echo "==> Created: $dmg_path"
}

require_command go
require_command npm
require_command swift
require_command hdiutil

rm -rf "$BUILD_DIR" "$APP_DIR"
mkdir -p "$BUILD_DIR" "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources" "$DIST_DIR"

echo "==> Build Vite frontend"
(
  cd "$ROOT_DIR/frontend/ugreen-app"
  npm ci
  npm run build
)

echo "==> Build Go backend ($GOARCH)"
(
  cd "$ROOT_DIR"
  GOOS=darwin GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -ldflags "-s -w -X main.Version=$VERSION" \
    -o "$BUILD_DIR/nasnotify-go-app" ./cmd/nasnotify
)

echo "==> Build Swift desktop app"
(
  cd "$ROOT_DIR/macos/NasNotifyGo"
  swift build -c release
)

SWIFT_BIN="$ROOT_DIR/macos/NasNotifyGo/.build/release/NasNotifyGo"
ICON_ICNS="$ROOT_DIR/macos/NasNotifyGo/Sources/NasNotifyGo/Resources/AppIcon.icns"
ICON_PNG="$ROOT_DIR/macos/NasNotifyGo/Sources/NasNotifyGo/Resources/AppIcon.png"

if [[ ! -f "$SWIFT_BIN" ]]; then
  echo "Swift binary not found: $SWIFT_BIN" >&2
  exit 1
fi

if [[ ! -f "$ICON_ICNS" ]]; then
  echo "App icon not found: $ICON_ICNS" >&2
  exit 1
fi

if [[ ! -f "$ICON_PNG" ]]; then
  echo "App icon png not found: $ICON_PNG" >&2
  exit 1
fi

cp "$SWIFT_BIN" "$APP_DIR/Contents/MacOS/$EXECUTABLE_NAME"
cp "$BUILD_DIR/nasnotify-go-app" "$APP_DIR/Contents/Resources/nasnotify-go-app"
cp "$ICON_ICNS" "$APP_DIR/Contents/Resources/AppIcon.icns"
cp "$ICON_PNG" "$APP_DIR/Contents/Resources/AppIcon.png"
cp -R "$ROOT_DIR/frontend/ugreen-app/dist" "$APP_DIR/Contents/Resources/www"

cat > "$APP_DIR/Contents/Resources/service-runner.sh" <<'RUNNER'
#!/usr/bin/env bash
set -euo pipefail
APP_SUPPORT="$HOME/Library/Application Support/NasNotify-Go"
mkdir -p "$APP_SUPPORT/config" "$APP_SUPPORT/data"
cd "$APP_SUPPORT"
RESOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$RESOURCE_DIR/nasnotify-go-app"
RUNNER
chmod +x "$APP_DIR/Contents/Resources/service-runner.sh"

cat > "$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>zh_CN</string>
  <key>CFBundleDisplayName</key>
  <string>$APP_NAME</string>
  <key>CFBundleExecutable</key>
  <string>$EXECUTABLE_NAME</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$MARKETING_VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUILD_VERSION</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
  </dict>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
PLIST

chmod +x "$APP_DIR/Contents/MacOS/$EXECUTABLE_NAME"
chmod +x "$APP_DIR/Contents/Resources/nasnotify-go-app"

if [[ "$SIGN_AND_NOTARIZE" == "1" ]]; then
  sign_app_bundle
fi

echo "==> Created: $APP_DIR"

if [[ "$CREATE_DMG" == "1" ]]; then
  create_dmg
else
  echo "You can run it with: open '$APP_DIR'"
fi
