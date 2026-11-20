#!/data/data/com.termux/files/usr/bin/sh
# shellcheck shell=sh
# UNIFUSION fieldd auto-start for Termux:Boot.
# Install: cp scripts/termux-boot-fieldd.sh ~/.termux/boot/ && chmod +x ~/.termux/boot/termux-boot-fieldd.sh
#
# Starts the fieldd watchdog (scripts/fieldd-watchdog.sh) if it is not already
# running. The watchdog holds termux-wake-lock, rebuilds $TMPDIR/fieldd when
# the checked-out commit changes, and restarts fieldd whenever
# http://127.0.0.1:8899/status is not 200.
# It never edits, resets or cleans the repo and never touches .field.
# Pass --open to also open /living (the Termux:Widget shortcut does this
# itself after ensuring the watchdog is up).

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
PATH="$PREFIX/bin:$HOME/go/bin:$PATH"
TMPDIR="${TMPDIR:-$PREFIX/tmp}"
export PREFIX PATH TMPDIR
REPO="${UNIFUSION_REPO:-$HOME/AXM/emergion-sovereign-runtime}"
ADDR="${FIELDD_ADDR:-127.0.0.1:8899}"
URL="http://$ADDR/living"
LOG="$TMPDIR/fieldd-boot.log"
WATCHDOG="$REPO/scripts/fieldd-watchdog.sh"
mkdir -p "$TMPDIR"

say() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >> "$LOG"; }

up() { [ "$(curl -s -o /dev/null -m 3 -w '%{http_code}' "http://$ADDR/status" 2>/dev/null)" = "200" ]; }

open_living() {
  case " $* " in *" --open "*) command -v termux-open-url >/dev/null 2>&1 && termux-open-url "$URL" ;; esac
}

if [ ! -f "$WATCHDOG" ]; then
  say "watchdog script not found at $WATCHDOG; not starting"
  open_living "$@"
  exit 0
fi

command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock
sh "$WATCHDOG" --ensure
say "watchdog --ensure finished"

if up; then
  say "fieldd already answering on $ADDR"
  open_living "$@"
  exit 0
fi

# First start after boot can take a few seconds to build or come up.
i=0
while [ $i -lt 20 ]; do
  sleep 1
  if up; then
    say "fieldd up on $ADDR"
    open_living "$@"
    exit 0
  fi
  i=$((i + 1))
done
say "fieldd not answering yet; the watchdog will keep trying. See $TMPDIR/fieldd-watchdog.log"
open_living "$@"
exit 0
