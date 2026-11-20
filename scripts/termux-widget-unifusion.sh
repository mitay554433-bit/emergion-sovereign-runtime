#!/data/data/com.termux/files/usr/bin/sh
# shellcheck shell=sh
# UNIFUSION home-screen shortcut for Termux:Widget.
# Install: cp scripts/termux-widget-unifusion.sh ~/.shortcuts/UNIFUSION && chmod +x ~/.shortcuts/UNIFUSION
# Starts the fieldd watchdog if it is not running, then opens /living.
# The PWA cannot start Termux itself; this shortcut and Termux:Boot do that.

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
PATH="$PREFIX/bin:$HOME/go/bin:$PATH"
TMPDIR="${TMPDIR:-$PREFIX/tmp}"
export PREFIX PATH TMPDIR
REPO="${UNIFUSION_REPO:-$HOME/AXM/emergion-sovereign-runtime}"
ADDR="${FIELDD_ADDR:-127.0.0.1:8899}"
WATCHDOG="$REPO/scripts/fieldd-watchdog.sh"

if [ -f "$WATCHDOG" ]; then
  sh "$WATCHDOG" --ensure
else
  BOOT="$HOME/.termux/boot/termux-boot-fieldd.sh"
  [ -f "$BOOT" ] || BOOT="$REPO/scripts/termux-boot-fieldd.sh"
  [ -f "$BOOT" ] && sh "$BOOT"
fi
command -v termux-open-url >/dev/null 2>&1 && termux-open-url "http://$ADDR/living"
