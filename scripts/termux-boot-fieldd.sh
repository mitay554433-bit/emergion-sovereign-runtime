#!/data/data/com.termux/files/usr/bin/sh
# UNIFUSION fieldd auto-start for Termux:Boot.
# Install: cp scripts/termux-boot-fieldd.sh ~/.termux/boot/ && chmod +x ~/.termux/boot/termux-boot-fieldd.sh
#
# Idempotent: if fieldd already answers on 127.0.0.1:8899 it does nothing.
# Builds fieldd into $TMPDIR only when the binary is missing or older than
# the checked-out commit. Logs to $TMPDIR/fieldd-boot.log and $TMPDIR/fieldd.log.
# It never edits, resets or cleans the repo and never touches .field.
# Pass --open to also open /living (the Termux:Widget shortcut does this).

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
PATH="$PREFIX/bin:$HOME/go/bin:$PATH"
TMPDIR="${TMPDIR:-$PREFIX/tmp}"
export PREFIX PATH TMPDIR
REPO="${UNIFUSION_REPO:-$HOME/AXM/emergion-sovereign-runtime}"
ADDR="${FIELDD_ADDR:-127.0.0.1:8899}"
URL="http://$ADDR/living"
BIN="$TMPDIR/fieldd"
STAMP="$TMPDIR/fieldd.built-sha"
LOG="$TMPDIR/fieldd-boot.log"
mkdir -p "$TMPDIR"

say() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >> "$LOG"; }

up() { [ "$(curl -s -o /dev/null -m 3 -w '%{http_code}' "http://$ADDR/status" 2>/dev/null)" = "200" ]; }

open_living() {
  case " $* " in *" --open "*) command -v termux-open-url >/dev/null 2>&1 && termux-open-url "$URL" ;; esac
}

if up; then
  say "fieldd already running on $ADDR; nothing to do"
  open_living "$@"
  exit 0
fi

command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock

if [ ! -d "$REPO" ]; then
  say "repo not found at $REPO; not starting"
  exit 0
fi
cd "$REPO" || exit 0

HEAD_SHA="$(git rev-parse HEAD 2>/dev/null)"
if [ ! -x "$BIN" ] || [ "$(cat "$STAMP" 2>/dev/null)" != "$HEAD_SHA" ]; then
  say "building fieldd at ${HEAD_SHA:-unknown}"
  if go build -o "$BIN" ./cmd/fieldd >> "$LOG" 2>&1; then
    echo "$HEAD_SHA" > "$STAMP"
    say "BUILD_OK"
  else
    say "build failed; see above"
    [ -x "$BIN" ] || exit 0
    say "starting the previous build"
  fi
fi

# A stale fieldd that is not answering HTTP would hold the socket/pid; clear only that.
if pgrep -f "$BIN -http" >/dev/null 2>&1; then
  say "fieldd process present but not answering; restarting it"
  pkill -f "$BIN -http"
  sleep 1
fi

nohup "$BIN" -http "$ADDR" >> "$TMPDIR/fieldd.log" 2>&1 &
i=0
while [ $i -lt 20 ]; do
  sleep 1
  if up; then
    say "fieldd up on $ADDR (pid $(pgrep -f "$BIN -http" | head -1))"
    open_living "$@"
    exit 0
  fi
  i=$((i + 1))
done
say "fieldd did not answer within 20s; see $TMPDIR/fieldd.log"
exit 0
