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
#   - probes http://$FIELDD_ADDR/healthz every 10 seconds (falls back to
#     /status on an older build without /healthz), curl timeout 8 seconds
#   - restarts fieldd with only `-http` (never -state) only after 3
#     consecutive failed probes, or at once when its process is gone; every
#     failed probe is logged with the reason (timeout, refused, HTTP code)
#   - backs off after repeated failed restarts
#   - logs to $TMPDIR/fieldd-watchdog.log, capped at 1 MiB
#
# It never edits, resets or cleans the repo, never touches .field, and
# never exits with status 1.
#
# Settings (all optional): UNIFUSION_REPO, FIELDD_ADDR (default 127.0.0.1:8899),
# FIELDD_WATCHDOG_INTERVAL (default 10), FIELDD_WATCHDOG_LOG,
# FIELDD_WATCHDOG_PROBE_TIMEOUT (curl seconds, default 8),
# FIELDD_WATCHDOG_FAILS (consecutive failed probes before a restart, default 3).

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
PROBE_TIMEOUT="${FIELDD_WATCHDOG_PROBE_TIMEOUT:-8}"
MAX_MISSES="${FIELDD_WATCHDOG_FAILS:-3}"
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

# Only this watchdog's own binary on this address counts; a foreign fieldd
# (for example an older dist/fieldd started with -state) is never ours.
pid_is_fieldd() {
  [ -n "$1" ] || return 1
  kill -0 "$1" 2>/dev/null || return 1
  case "$(pid_cmdline "$1")" in
    *"$BIN -http $ADDR"*) return 0 ;;
  esac
  return 1
}

# fieldd_running: this watchdog's fieldd process exists (answering or not).
fieldd_running() {
  pid_is_fieldd "$(cat "$CHILD_PIDFILE" 2>/dev/null)" && return 0
  command -v pgrep >/dev/null 2>&1 && pgrep -f "$BIN -http $ADDR" >/dev/null 2>&1 && return 0
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

# probe: 0 when fieldd answers. /healthz answers from memory, so a busy
# field.json build or a long model call cannot make a live fieldd look dead.
# On failure PROBE_WHY says why (no answer in time, refused, HTTP code).
PROBE_WHY=""
probe() {
  code=$(curl -s -o /dev/null -m "$PROBE_TIMEOUT" -w '%{http_code}' "http://$ADDR/healthz" 2>/dev/null)
  rc=$?
  if [ "$code" = "404" ]; then
    # older build without /healthz
    code=$(curl -s -o /dev/null -m "$PROBE_TIMEOUT" -w '%{http_code}' "http://$ADDR/status" 2>/dev/null)
    rc=$?
  fi
  [ "$code" = "200" ] && return 0
  case "$rc" in
    7) PROBE_WHY="connection refused" ;;
    28) PROBE_WHY="no answer within ${PROBE_TIMEOUT}s" ;;
    0) PROBE_WHY="HTTP $code" ;;
    *) PROBE_WHY="curl exit $rc (HTTP ${code:-none})" ;;
  esac
  return 1
}

up() { probe; }

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
  while [ $i -lt 30 ]; do
    if up; then
      say "fieldd up on $ADDR (pid $(cat "$CHILD_PIDFILE" 2>/dev/null))"
      return 0
    fi
    # kill -0, not the command line: right after the fork the child may
    # not have exec'd fieldd yet.
    if ! kill -0 "$(cat "$CHILD_PIDFILE" 2>/dev/null)" 2>/dev/null; then
      say "fieldd exited during startup ($PROBE_WHY); see $TMPDIR/fieldd.log"
      return 1
    fi
    sleep 1
    i=$((i + 1))
  done
  say "fieldd did not answer within 30s ($PROBE_WHY); see $TMPDIR/fieldd.log"
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
trap 'drop_lock' EXIT
trap 'drop_lock; exit 0' INT TERM HUP

command -v termux-wake-lock >/dev/null 2>&1 && termux-wake-lock
say "watchdog loop starting (pid $$, addr $ADDR, interval ${INTERVAL}s, probe timeout ${PROBE_TIMEOUT}s, restart after $MAX_MISSES failed probes, repo $REPO)"

fails=0
misses=0
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
    if [ "$misses" -gt 0 ]; then
      say "fieldd answering again after $misses failed probe(s); no restart"
      misses=0
    fi
    # Something answers on $ADDR. If it is not this watchdog's binary (for
    # example an older dist/fieldd started with -state), say so: it is never
    # killed here, but it serves an old build in place of $BIN.
    if command -v pgrep >/dev/null 2>&1 && ! pgrep -f "$BIN -http $ADDR" >/dev/null 2>&1; then
      foreign=$(pgrep -af 'fieldd' 2>/dev/null | grep -v 'fieldd-watchdog' | grep -vF "$BIN -http" | head -3 | tr '\n' ';')
      foreign="${foreign:-unknown}"
      if [ "$foreign" != "$foreign_said" ]; then
        say "FOREIGN: $ADDR is answered by a process that is not $BIN -http $ADDR ($foreign); it is never killed here; stop it so this build can serve"
        foreign_said="$foreign"
      fi
      sleep "$INTERVAL"
      continue
    fi
    foreign_said=""
    sleep "$INTERVAL"
    continue
  fi
  misses=$((misses + 1))
  if fieldd_running && [ "$misses" -lt "$MAX_MISSES" ]; then
    say "probe $misses/$MAX_MISSES failed on $ADDR ($PROBE_WHY); fieldd still running, not restarting yet"
    sleep "$INTERVAL"
    continue
  fi
  if fieldd_running; then
    say "fieldd not answering on $ADDR ($PROBE_WHY; $misses consecutive failed probes); restarting"
  else
    say "fieldd not running on $ADDR ($PROBE_WHY); starting"
  fi
  misses=0
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
