#!/usr/bin/env bash
# Build Invtts.app — native macOS SwiftUI client.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MACOS="$ROOT/macos"
SRC="$MACOS/Sources"
RES="$MACOS/Resources"
APP_NAME="Invtts"
APP_DIR="$ROOT/$APP_NAME.app"
SDK="$(xcrun --show-sdk-path)"
OUT="$MACOS/build/Invtts"

# Build icon assets if missing
if [[ ! -f "$RES/AppIcon.icns" ]]; then
  echo "Generating AppIcon.icns…"
  ICON_SRC="$RES/AppIcon-1024.png"
  if [[ ! -f "$ICON_SRC" ]]; then
    ICON_SRC="$ROOT/assets/AppIcon-1024.png"
  fi
  if [[ -f "$ICON_SRC" ]]; then
  ICONSET="$RES/AppIcon.iconset"
  mkdir -p "$ICONSET"
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" "$ICON_SRC" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
    s2=$((size * 2))
    sips -z "$s2" "$s2" "$ICON_SRC" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
  done
  iconutil -c icns "$ICONSET" -o "$RES/AppIcon.icns"
  sips -z 22 22 "$ICON_SRC" --out "$RES/MenuBarIcon.png" >/dev/null
  fi
fi

echo "Building $APP_NAME (release)…"
mkdir -p "$MACOS/build"

swiftc -O \
  -sdk "$SDK" \
  -target arm64-apple-macosx13.0 \
  -parse-as-library \
  "$SRC"/*.swift \
  -o "$OUT" \
  -framework SwiftUI \
  -framework AppKit \
  -framework AVFoundation \
  -framework UniformTypeIdentifiers

echo "Packaging $APP_DIR …"
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"

cp "$OUT" "$APP_DIR/Contents/MacOS/$APP_NAME"
chmod +x "$APP_DIR/Contents/MacOS/$APP_NAME"

if [[ -f "$RES/AppIcon.icns" ]]; then
  cp "$RES/AppIcon.icns" "$APP_DIR/Contents/Resources/"
fi
if [[ -f "$RES/MenuBarIcon.png" ]]; then
  cp "$RES/MenuBarIcon.png" "$APP_DIR/Contents/Resources/"
fi
if [[ -f "$RES/MenuBarIcon@2x.png" ]]; then
  cp "$RES/MenuBarIcon@2x.png" "$APP_DIR/Contents/Resources/"
fi

cat > "$APP_DIR/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>Invtts</string>
    <key>CFBundleIdentifier</key>
    <string>com.invtts.app</string>
    <key>CFBundleName</key>
    <string>invtts</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>13.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSPrincipalClass</key>
    <string>NSApplication</string>
    <key>NSAppleEventsUsageDescription</key>
    <string>invtts imports text from Microsoft Word to prepare speech.</string>
</dict>
</plist>
PLIST

echo "Done → $APP_DIR"
echo "Open with: open \"$APP_DIR\""
