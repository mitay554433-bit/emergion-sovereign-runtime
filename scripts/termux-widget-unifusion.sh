#!/data/data/com.termux/files/usr/bin/sh
# UNIFUSION home-screen shortcut for Termux:Widget.
# Install: cp scripts/termux-widget-unifusion.sh ~/.shortcuts/UNIFUSION && chmod +x ~/.shortcuts/UNIFUSION
# Starts fieldd if it is not running (same idempotent logic as Termux:Boot),
# then opens /living.

BOOT="$HOME/.termux/boot/termux-boot-fieldd.sh"
[ -x "$BOOT" ] || BOOT="${UNIFUSION_REPO:-$HOME/AXM/emergion-sovereign-runtime}/scripts/termux-boot-fieldd.sh"
if [ -f "$BOOT" ]; then
  sh "$BOOT" --open
else
  termux-open-url "http://127.0.0.1:8899/living"
fi
