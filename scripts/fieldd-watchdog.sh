#!/data/data/com.termux/files/usr/bin/sh
# shellcheck shell=sh
# UNIFUSION fieldd watchdog: keep fieldd answering on the loopback address.
#
# Termux:Boot and the Termux:Widget shortcut start this script with --ensure,
# which backgrounds a single long-running instance. A home-screen PWA cannot
# launch Termux, so this process is what keeps /living reachable.
#
# The loop:
#   - takes termux-wake-lock if that command exists
#   - rebuilds $TMPDIR/fieldd when it is missing or built from a different commit
#   - checks http://$FIELDD_ADDR/status every 10 seconds
#   - restarts fieldd with only `-http` (never -state) if the status is not 200
#   - backs off after repeated failed restarts
#   - logs to $TMPDIR/fieldd-watchdog.log, capped at 1 MiB
#
# It never edits, resets or cleans the repo, never touches .field, and
# never exits with status 1.
#
# Settings (all optional): UNIFUSION_REPO, FIELDD_ADDR (default 127.0.0.1:8899),
# FIELDD_WATCHDOG_INTERVAL (default 10), FIELDD_WATCHDOG_LOG.

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
PATH="$PREFIX/bin:$HOME/go/bin:$PATH"
TMPDIR="${TMPDIR:-$PREFIX/tmp}"
export PREFIX PATH TMPDIR
REPO="${UNIFUSION_REPO:-$HOME/AXM/emergion-sovereign-runtime}"
ADDR="${FIELDD_ADDR:-127.0.0.1:8899}"
BIN="$TMPDIR/fieldd"
STAMP="$TMPDIR/fieldd.built-sha"
LOG="${FIELDD_WATCHDOG_LOG:-$TMPDIR/fieldd-watchdog.log}"
LOCKDIR="$TMPDIR/fieldd-watchdog.lock"
PIDFILE="$TMPDIR/fieldd-watchdog.pid"
CHILD_PIDFILE="$TMPDIR/fieldd-watchdog-child.pid"
INTERVAL="${FIELDD_WATCHDOG_INTERVAL:-10}"
SCRIPT="$REPO/scripts/fieldd-watchdog.sh"
mkdir -p "$TMPDIR"

say() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >> "$LOG"; }

cap_log() {
  [ -f "$LOG" ] || return 0
  sz=$(wc -c < "$LOG" | tr -d ' ')
  [ -n "$sz" ] || return 0
  if [ "$sz" -gt 1048576 ]; then
    tail -c 262144 "$LOG" > "$LOG.0" && mv "$LOG.0" "$LOG"
    echo "$(date '+%Y-%m-%d %H:%M:%S') log rotated (was $sz bytes)" >> "$LOG"
  fi
}

# A pidfile PID is only "ours" if it is still running and its command
# line matches. kill -0 alone is not enough: $TMPDIR persists across
# Termux reboots, and a leftover PID can be reused by another process.

pid_cmdline() {
  tr '\0' ' ' < "/proc/$1/cmdline" 2>/dev/null
}

pid_is_watchdog() {
  [ -n "$1" ] || return 1
  kill -0 "$1" 2>/dev/null || return 1
  case "$(pid_cmdline "$1")" in
    *fieldd-watchdog.sh*) return 0 ;;
  esac
  return 1
}

pid_is_fieldd() {
  [ -n "$1" ] || return 1
  kill -0 "$1" 2>/dev/null || return 1
  case "$(pid_cmdline "$1")" in
    *fieldd*-http*) return 0 ;;
  esac
  return 1
}

watchdog_pid_live() {
  [ -f "$PIDFILE" ] || return 1
  pid_is_watchdog "$(cat "$PIDFILE" 2>/dev/null)"
}

already_running() {
  if watchdog_pid_live; then
    wp=$(cat "$PIDFILE")
    if [ "$wp" = "$$" ]; then
      return 1
    fi
    return 0
  fi
  return 1
}

take_lock() {
  if mkdir "$LOCKDIR" 2>/dev/null; then
    echo $$ > "$PIDFILE"
    echo $$ > "$LOCKDIR/pid"
    return 0
  fi
  if pid_is_watchdog "$(cat "$LOCKDIR/pid" 2>/dev/null)"; then
    return 1
  fi
  # Stale lock from a previous boot, a killed process, or a reused PID.
  rm -rf "$LOCKDIR"
  if mkdir "$LOCKDIR" 2>/dev/null; then
    echo $$ > "$PIDFILE"
    echo $$ > "$LOCKDIR/pid"
    return 0
  fi
  return 1
}

drop_lock() {
  rm -f "$PIDFILE"
  rm -rf "$LOCKDIR"
}

up() { [ "$(curl -s -o /dev/null -m 3 -w '%{http_code}' "http://$ADDR/status" 2>/dev/null)" = "200" ]; }

ensure_bin() {
  if [ ! -d "$REPO" ]; then
    say "repo not found at $REPO; cannot build or start"
    return 1
  fi
  cd "$REPO" || return 1
  HEAD_SHA="$(git rev-parse HEAD 2>/dev/null)"
  if [ ! -x "$BIN" ] || [ "$(cat "$STAMP" 2>/dev/null)" != "$HEAD_SHA" ]; then
    say "building fieldd at ${HEAD_SHA:-unknown}"
    if go build -o "$BIN" ./cmd/fieldd >> "$LOG" 2>&1; then
      echo "$HEAD_SHA" > "$STAMP"
      say "BUILD_OK"
    else
      say "build failed; see above"
      if [ ! -x "$BIN" ]; then
        return 1
      fi
      say "starting the previous build"
    fi
  fi
  return 0
}

stop_fieldd() {
  old=""
  if [ -f "$CHILD_PIDFILE" ]; then
    old=$(cat "$CHILD_PIDFILE" 2>/dev/null)
  fi
  if pid_is_fieldd "$old"; then
    say "stopping fieldd pid $old"
    kill "$old" 2>/dev/null
    i=0
    while [ $i -lt 10 ]; do
      kill -0 "$old" 2>/dev/null || break
      sleep 1
      i=$((i + 1))
    done
    kill -0 "$old" 2>/dev/null && kill -9 "$old" 2>/dev/null
  fi
  rm -f "$CHILD_PIDFILE"
  # Only match this binary and this bind address, never some other fieldd.
  if command -v pgrep >/dev/null 2>&1 && pgrep -f "$BIN -http $ADDR" >/dev/null 2>&1; then
    say "clearing leftover $BIN -http $ADDR"
    pkill -f "$BIN -http $ADDR" 2>/dev/null
    sleep 1
  fi
}

start_fieldd() {
  if [ ! -d "$REPO" ]; then
    say "repo not found at $REPO; not starting"
    return 1
  fi
  cd "$REPO" || return 1
  if [ ! -x "$BIN" ]; then
    say "fieldd binary missing at $BIN"
    return 1
  fi
  nohup "$BIN" -http "$ADDR" >> "$TMPDIR/fieldd.log" 2>&1 &
  echo $! > "$CHILD_PIDFILE"
  say "started fieldd pid $! on $ADDR"
  return 0
}

wait_up() {
  i=0
  while [ $i -lt 20 ]; do
    if up; then
      say "fieldd up on $ADDR (pid $(cat "$CHILD_PIDFILE" 2>/dev/null))"
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  say "fieldd did not answer within 20s; see $TMPDIR/fieldd.log"
  return 1
}

# --ensure: start one background instance if needed, then return.
if [ "$1" = "--ensure" ]; then
  cap_log
  if already_running; then
    say "watchdog already running (pid $(cat "$PIDFILE")); nothing to do"
    exit 0
  fi
  if [ ! -f "$SCRIPT" ]; then
    say "watchdog script not found at $SCRIPT"
    exit 0
  fi
  command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock
  nohup sh "$SCRIPT" >> "$LOG" 2>&1 &
  say "watchdog started in background (pid $!)"
  exit 0
fi

cap_log
if already_running; then
  say "watchdog already running (pid $(cat "$PIDFILE")); this instance exiting"
  exit 0
fi
if ! take_lock; then
  say "could not take watchdog lock; another instance is starting"
  exit 0
fi
trap 'drop_lock' EXIT INT TERM HUP

command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock
say "watchdog loop starting (pid $$, addr $ADDR, interval ${INTERVAL}s, repo $REPO)"

fails=0
foreign_said=""
while true; do
  cap_log
  if ! ensure_bin; then
    fails=$((fails + 1))
    backoff=$((INTERVAL * fails))
    [ "$backoff" -gt 60 ] && backoff=60
    say "no runnable fieldd binary; backing off ${backoff}s"
    sleep "$backoff"
    continue
  fi
  if up; then
    fails=0
    # Something answers on $ADDR. If it is not this watchdog's binary (for
    # example an older dist/fieldd started with -state), say so: it is never
    # killed here, but it serves an old build in place of $BIN.
    if command -v pgrep >/dev/null 2>&1 && ! pgrep -f "$BIN -http $ADDR" >/dev/null 2>&1; then
      foreign=$(pgrep -af 'fieldd' 2>/dev/null | grep -v 'fieldd-watchdog' | grep -vF "$BIN -http" | head -3 | tr '\n' ';')
      if [ "$foreign" != "$foreign_said" ]; then
        say "FOREIGN: $ADDR is answered by a process that is not $BIN -http $ADDR (${foreign:-unknown}); stop it so this build can serve"
        foreign_said="$foreign"
      fi
      sleep "$INTERVAL"
      continue
    fi
    foreign_said=""
    sleep "$INTERVAL"
    continue
  fi
  say "fieldd not answering on $ADDR; restarting"
  stop_fieldd
  if start_fieldd && wait_up; then
    fails=0
    sleep "$INTERVAL"
    continue
  fi
  fails=$((fails + 1))
  backoff=$((INTERVAL * fails))
  [ "$backoff" -gt 60 ] && backoff=60
  say "restart failed (attempt $fails); backing off ${backoff}s"
  sleep "$backoff"
done
