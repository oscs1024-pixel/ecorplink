#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
APP_SRC="${APP_SRC:-}"
DAEMON_SRC="$ROOT_DIR/cmd/gui/daemon/ecorplink-daemon"
DAEMON_DST="$HOME/.ecorplink/bin/ecorplink-daemon"

if [ -z "$APP_SRC" ]; then
  APP_SRC="$(find "$ROOT_DIR/build/package" -name ECorpLink.app -type d 2>/dev/null | sort | tail -n 1)"
fi
if [ ! -d "$APP_SRC" ]; then
  printf 'error: app bundle not found: %s\n' "$APP_SRC" >&2
  exit 1
fi
if [ ! -x "$DAEMON_SRC" ]; then
  printf 'error: daemon binary not found: %s\n' "$DAEMON_SRC" >&2
  exit 1
fi

if [ -x "$DAEMON_DST" ]; then
  "$DAEMON_DST" stop >/dev/null 2>&1 || true
  sudo -n "$DAEMON_DST" stop >/dev/null 2>&1 || true
fi

mkdir -p "$(dirname "$DAEMON_DST")"
install -m 0755 "$DAEMON_SRC" "$DAEMON_DST"

for app in /Applications/ECorpLink.app /Applications/Ecorplink.app; do
  if [ -d "$app" ]; then
    rm -rf "$app"
    ditto "$APP_SRC" "$app"
  fi
done

printf 'installed daemon: %s\n' "$DAEMON_DST"
printf 'installed app bundle from: %s\n' "$APP_SRC"
