#!/usr/bin/env bash
# pilot-demo.sh — throwaway UNIFUSION™ pilot walkthrough
#
#   doctor -> capture -> HUMAN_FINAL decision -> verify (receipt) -> tamper
#
# Builds `field` from a local checkout of emergion-sovereign-runtime (the
# checkout's HEAD by default), then:
#   1. creates a throwaway state directory under $TMPDIR and runs `field doctor`
#   2. captures two small synthetic samples, HUMAN_FINAL-approves each,
#      runs `field verify` and saves tip_hash / events as a receipt
#   3. on COPIES of that state only:
#        - one-byte edit of stored evidence  -> verify fails, naming IDN/hash
#        - deleted evidence object           -> verify fails
#        - swapped evidence objects          -> verify fails, naming the hash
#        - deleted last event                -> verify passes without a receipt
#                                            -> fails with -expect-tip / -expect-count
#
# SAFETY
#   Never touches ~/AXM, any path matching */AXM/*, or a repository .field.
#   All state lives under a single target directory inside $TMPDIR.
#   Refuses if the target directory already exists.
#   Unsets FIELD_HOME / FIELD_DROPZONE / FIELD_OUTPUT so defaults cannot
#   write into the current working directory.
#   The HUMAN_FINAL decisions are made on the throwaway state only.
#
# Usage:
#   scripts/pilot-demo.sh [REPO]
#     REPO is a local git checkout of emergion-sovereign-runtime.
#     If omitted, the current directory is used.
#
# Optional environment:
#   UNIFUSION_DEMO_COMMIT      Commit to build (default: HEAD of REPO).
#   UNIFUSION_TAMPER_DEMO_DIR  Target directory under $TMPDIR
#                              (default $TMPDIR/unifusion-pilot-demo).
#   KEEP=1                     Leave the target directory in place on success.
#
# The demo uses `field -reasoner heuristic` so it needs no model or network.
# `field doctor` also checks the local Gemma path; that check may report
# false on a machine without a model, which does not affect this demo.

set -euo pipefail


die() { printf 'error: %s\n' "$*" >&2; exit 1; }
need_cmd() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }

abspath() {
  # POSIX-ish absolute path; does not require GNU readlink.
  local t=$1
  if command -v realpath >/dev/null 2>&1; then
    realpath "$t"
    return
  fi
  if command -v readlink >/dev/null 2>&1 && readlink -f / >/dev/null 2>&1; then
    readlink -f "$t"
    return
  fi
  (cd "$t" && pwd)
}

path_is_under() {
  # true if $1 is $2 or a path under $2
  local child parent
  child=${1%/}
  parent=${2%/}
  case "$child" in
    "$parent"|"$parent"/*) return 0 ;;
    *) return 1 ;;
  esac
}

looks_like_axm() {
  case "$1" in
    */AXM|*/AXM/*|*/axm|*/axm/*) return 0 ;;
    *) return 1 ;;
  esac
}

need_cmd git
need_cmd go
need_cmd python3

if [ -z "${TMPDIR:-}" ] || [ "$TMPDIR" = "$HOME" ]; then
  TMPDIR=/tmp
fi
TMPDIR=$(cd "$TMPDIR" && pwd) || die "TMPDIR is not a usable directory: ${TMPDIR:-unset}"
if looks_like_axm "$TMPDIR"; then
  die "TMPDIR must not be under AXM: $TMPDIR"
fi

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  sed -n '2,40p' "$0"
  exit 0
fi

if [ -n "${1:-}" ]; then
  REPO=$1
else
  REPO=$PWD
fi
if [ ! -d "$REPO/.git" ] && [ ! -f "$REPO/.git" ]; then
  die "REPO is not a git checkout: $REPO"
fi
REPO=$(cd "$REPO" && pwd) || die "cannot enter REPO: $REPO"
if [ ! -f "$REPO/cmd/field/main.go" ]; then
  die "REPO does not look like emergion-sovereign-runtime (missing cmd/field/main.go): $REPO"
fi
REQUIRED_COMMIT=$(git -C "$REPO" rev-parse "${UNIFUSION_DEMO_COMMIT:-HEAD}^{commit}" 2>/dev/null) \
  || die "REPO does not contain commit ${UNIFUSION_DEMO_COMMIT:-HEAD}"
REQUIRED_SHORT=${REQUIRED_COMMIT:0:7}

TARGET=${UNIFUSION_TAMPER_DEMO_DIR:-"$TMPDIR/unifusion-pilot-demo"}
# If a relative path was given, resolve it only after existence check? Resolve parent.
case "$TARGET" in
  /*) ;;
  *) TARGET="$PWD/$TARGET" ;;
esac
# Reject existing target before mkdir. Do not resolve through a missing path.
if [ -e "$TARGET" ]; then
  die "refusing to use an existing target directory: $TARGET"
fi
PARENT=$(cd "$(dirname "$TARGET")" && pwd) || die "cannot enter parent of target: $TARGET"
TARGET="$PARENT/$(basename "$TARGET")"
path_is_under "$TARGET" "$TMPDIR" || die "target must be under TMPDIR ($TMPDIR), not: $TARGET"
looks_like_axm "$TARGET" && die "target must not be under AXM: $TARGET"
path_is_under "$TARGET" "$REPO" && die "target must not be inside the source repo: $TARGET"

# Prevent inherited env from writing to a real .field or dropzone.
unset FIELD_HOME FIELD_DROPZONE FIELD_OUTPUT FIELD_REASONER FIELD_PROBE_WORKTREE || true

mkdir -p "$TARGET" || die "could not create $TARGET"
# From here, clean up on unexpected failure unless KEEP=1 or we finish cleanly.
cleanup() {
  local st=$?
  if [ "${KEEP:-0}" != "1" ] && [ "$st" -ne 0 ] && [ -d "$TARGET" ]; then
    rm -rf "$TARGET"
  fi
}
trap cleanup EXIT

SRC="$TARGET/src"
STATE="$TARGET/state"
OUTPUTS="$TARGET/outputs"
BINDIR="$TARGET/bin"
SAMPLE="$TARGET/sample.txt"
SAMPLE2="$TARGET/sample-2.txt"
RECEIPT="$TARGET/receipt.json"
COPY_EDIT="$TARGET/copy-one-byte-edit"
COPY_DEL_EV="$TARGET/copy-delete-evidence"
COPY_DEL_TIP="$TARGET/copy-delete-last-event"
COPY_SWAP="$TARGET/copy-swap-evidence"

# Extra refuse: never use repo .field as state.
for p in "$STATE" "$OUTPUTS" "$COPY_EDIT" "$COPY_DEL_EV" "$COPY_DEL_TIP" "$COPY_SWAP"; do
  looks_like_axm "$p" && die "refusing AXM path: $p"
  case "$p" in
    "$REPO/.field"|"$REPO/.field"/*) die "refusing to use the repository .field: $p" ;;
  esac
done

printf '== clone %s at %s into %s ==\n' "$REPO" "$REQUIRED_SHORT" "$SRC"
git clone --no-local --no-checkout "$REPO" "$SRC" >/dev/null
git -C "$SRC" checkout -q "$REQUIRED_COMMIT"
HEAD=$(git -C "$SRC" rev-parse HEAD)
[ "$HEAD" = "$REQUIRED_COMMIT" ] || die "checkout was not $REQUIRED_SHORT (got $HEAD)"

printf '== build field ==\n'
mkdir -p "$BINDIR"
# Prefer the module cache already on this machine; do not require the network.
if ! ( cd "$SRC" && GOPROXY="${GOPROXY:-off}" go build -o "$BINDIR/field" ./cmd/field ); then
  ( cd "$SRC" && go build -o "$BINDIR/field" ./cmd/field )
fi
FIELD="$BINDIR/field"
[ -x "$FIELD" ] || die "field binary was not built"

run_field() {
  # Global flags MUST precede the subcommand (see cmd/field/main.go).
  # -dropzone is set under the throwaway tree so `init` cannot create a
  # dropzone in the caller's current directory or in the source repo.
  "$FIELD" -state "$1" -reasoner heuristic -output "$2" -dropzone "$TARGET/dropzone" "${@:3}"
}

printf '== init throwaway state ==\n'
mkdir -p "$STATE" "$OUTPUTS"
run_field "$STATE" "$OUTPUTS" init
# init prints READY <state> <dropzone>. dropzone defaults to "dropzone" under cwd;
# we do not use the dropzone in this demo. Ignore any cwd dropzone by not calling once/run.

printf '== doctor ==\n'
# doctor prints a JSON readiness report. It is shown, not enforced: the
# Gemma check is not needed for the heuristic reasoner used here.
"$FIELD" -state "$STATE" doctor || true

cat > "$SAMPLE" <<'EOF'
UNIFUSION tamper-demo sample observation.
A cabinet door was observed closed at the start of this throwaway run.
This file is synthetic demo input, not customer evidence.
EOF
cat > "$SAMPLE2" <<'EOF'
UNIFUSION pilot-demo second sample observation.
A gauge read within its normal band at the start of this throwaway run.
This file is synthetic demo input, not customer evidence.
EOF

printf '== capture ==\n'
CAPTURE_OUT=$(run_field "$STATE" "$OUTPUTS" capture "$SAMPLE")
printf '%s\n' "$CAPTURE_OUT"
IDN=${CAPTURE_OUT%% *}
case "$IDN" in
  E-*) ;;
  *) die "capture did not print an EmergION IDN (got: $CAPTURE_OUT)" ;;
esac
printf '%s\n' "$CAPTURE_OUT" | grep -q 'AT_GOV' || die "capture did not report AT_GOV: $CAPTURE_OUT"

printf '== HUMAN_FINAL decide APPROVE ==\n'
DECIDE_OUT=$(run_field "$STATE" "$OUTPUTS" decide "$IDN" APPROVE "pilot-demo throwaway approval")
printf '%s\n' "$DECIDE_OUT"
printf '%s\n' "$DECIDE_OUT" | grep -q 'REG_ACCEPTED' || die "decide did not report REG_ACCEPTED: $DECIDE_OUT"

printf '== capture and HUMAN_FINAL approve a second sample ==\n'
CAPTURE2_OUT=$(run_field "$STATE" "$OUTPUTS" capture "$SAMPLE2")
printf '%s\n' "$CAPTURE2_OUT"
IDN2=${CAPTURE2_OUT%% *}
case "$IDN2" in
  E-*) ;;
  *) die "second capture did not print an EmergION IDN (got: $CAPTURE2_OUT)" ;;
esac
DECIDE2_OUT=$(run_field "$STATE" "$OUTPUTS" decide "$IDN2" APPROVE "pilot-demo throwaway approval 2")
printf '%s\n' "$DECIDE2_OUT"
printf '%s\n' "$DECIDE2_OUT" | grep -q 'REG_ACCEPTED' || die "second decide did not report REG_ACCEPTED: $DECIDE2_OUT"

printf '== field verify (save receipt) ==\n'
# verify writes JSON to stdout on success; failures go to stderr as "error: ..."
if ! VERIFY_OUT=$(run_field "$STATE" "$OUTPUTS" verify); then
  die "untampered verify failed: $VERIFY_OUT"
fi
printf '%s\n' "$VERIFY_OUT"
printf '%s\n' "$VERIFY_OUT" > "$RECEIPT"

eval "$(python3 - "$RECEIPT" <<'PY'
import json, sys
path = sys.argv[1]
data = json.load(open(path))
if data.get("status") != "PASS":
    sys.exit("untampered verify status is not PASS: %r" % (data,))
tip = data.get("tip_hash") or ""
count = data.get("events")
if not tip or count in (None, ""):
    sys.exit("receipt missing tip_hash or events: %r" % (data,))
def sh(name, val):
    # single-quote for eval
    s = str(val).replace("'", "'\\''")
    print(f"{name}='{s}'")
sh("TIP_HASH", tip)
sh("EVENT_COUNT", count)
PY
)"
[ -n "${TIP_HASH:-}" ] && [ -n "${EVENT_COUNT:-}" ] || die "could not parse receipt tip_hash/events"
printf 'RECEIPT tip_hash=%s events=%s\n' "$TIP_HASH" "$EVENT_COUNT"

# Locate a preserved evidence object in the clean state (used as the copy source).
mapfile -t EVFILES < <(find "$STATE/o" -maxdepth 1 -type f -name '*.gz' | sort)
[ "${#EVFILES[@]}" -ge 1 ] || die "no preserved evidence objects under $STATE/o"

copy_state() {
  local dest=$1
  [ ! -e "$dest" ] || die "copy destination already exists: $dest"
  cp -a "$STATE" "$dest"
}

expect_verify_fail() {
  local state=$1
  local outputs=$2
  local label=$3
  shift 3
  local errf outf st
  errf=$(mktemp "$TARGET/err.XXXXXX")
  outf=$(mktemp "$TARGET/out.XXXXXX")
  set +e
  run_field "$state" "$outputs" verify "$@" >"$outf" 2>"$errf"
  st=$?
  set -e
  if [ "$st" -eq 0 ]; then
    printf '%s\n' "--- stdout ---" >&2
    cat "$outf" >&2
    die "$label: verify unexpectedly passed"
  fi
  # field prints failures as: error: <message>
  FAIL_MSG=$(cat "$errf" "$outf")
  printf '%s\n' "$FAIL_MSG"
  [ -n "$FAIL_MSG" ] || die "$label: verify failed but printed no message"
}

expect_verify_pass() {
  local state=$1
  local outputs=$2
  local label=$3
  shift 3
  local out
  if ! out=$(run_field "$state" "$outputs" verify "$@"); then
    die "$label: verify unexpectedly failed: $out"
  fi
  printf '%s\n' "$out"
  printf '%s\n' "$out" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("status")=="PASS", d'
}

printf '\n== COPY 1: one-byte edit of stored evidence ==\n'
copy_state "$COPY_EDIT"
# The first record's evidence object: its file name starts with the IDN hex.
IDN_HEX=$(printf '%s' "${IDN#E-}" | tr 'A-F' 'a-f')
EV_EDIT=$(find "$COPY_EDIT/o" -maxdepth 1 -type f -name "${IDN_HEX}*.gz" | sort | head -n 1)
[ -n "$EV_EDIT" ] || die "no evidence file in copy-one-byte-edit"
python3 - "$EV_EDIT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
b = bytearray(p.read_bytes())
if not b:
    raise SystemExit("empty evidence object: %s" % p)
b[len(b)//2] ^= 0x01
p.write_bytes(b)
print("flipped one byte in", p)
PY
FAIL_MSG=""
expect_verify_fail "$COPY_EDIT" "$TARGET/outputs-edit" "one-byte edit"
printf '%s\n' "$FAIL_MSG" | grep -q "$IDN" || die "one-byte edit error did not name IDN $IDN: $FAIL_MSG"
# Must name a hash: evidence source hash and/or event hash (runtime label uses hash= and/or evidence=).
printf '%s\n' "$FAIL_MSG" | grep -Eq 'hash=|evidence=|hash mismatch hash=' \
  || die "one-byte edit error did not name a hash: $FAIL_MSG"
printf 'PASS one-byte edit: verify failed and named IDN/hash\n'

printf '\n== COPY 2: deleted evidence ==\n'
copy_state "$COPY_DEL_EV"
EV_DEL=$(find "$COPY_DEL_EV/o" -maxdepth 1 -type f -name "${IDN_HEX}*.gz" | sort | head -n 1)
[ -n "$EV_DEL" ] || die "no evidence file in copy-delete-evidence"
rm -f "$EV_DEL"
printf 'deleted %s\n' "$EV_DEL"
FAIL_MSG=""
expect_verify_fail "$COPY_DEL_EV" "$TARGET/outputs-del-ev" "deleted evidence"
printf '%s\n' "$FAIL_MSG" | grep -Eq 'missing evidence|evidence' \
  || die "deleted-evidence error did not mention evidence: $FAIL_MSG"
printf 'PASS deleted evidence: verify failed\n'

printf '\n== COPY 3: swapped evidence objects ==\n'
copy_state "$COPY_SWAP"
mapfile -t SWAP_FILES < <(find "$COPY_SWAP/o" -maxdepth 1 -type f -name '*.gz' | sort)
[ "${#SWAP_FILES[@]}" -ge 2 ] || die "need two evidence objects to swap, found ${#SWAP_FILES[@]}"
SWAP_A=${SWAP_FILES[0]}
SWAP_B=${SWAP_FILES[1]}
mv "$SWAP_A" "$TARGET/swap.tmp"
mv "$SWAP_B" "$SWAP_A"
mv "$TARGET/swap.tmp" "$SWAP_B"
printf 'swapped %s <-> %s\n' "$(basename "$SWAP_A")" "$(basename "$SWAP_B")"
FAIL_MSG=""
expect_verify_fail "$COPY_SWAP" "$TARGET/outputs-swap" "swapped evidence"
SWAP_HASH=$(basename "$SWAP_A" .gz)
SWAP_HASH_B=$(basename "$SWAP_B" .gz)
printf '%s\n' "$FAIL_MSG" | grep -Eq "$SWAP_HASH|$SWAP_HASH_B" \
  || die "swapped-evidence error did not name a swapped hash: $FAIL_MSG"
printf 'PASS swapped evidence: verify failed and named the hash\n'

printf '\n== COPY 4: deleted last event ==\n'
copy_state "$COPY_DEL_TIP"
LEDGER="$COPY_DEL_TIP/field.cosl"
[ -f "$LEDGER" ] || die "missing ledger in copy: $LEDGER"
python3 - "$LEDGER" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
text = p.read_text()
lines = text.splitlines()
if len(lines) < 2:
    raise SystemExit("ledger too short to delete last event: %d lines" % len(lines))
p.write_text("\n".join(lines[:-1]) + "\n")
print("removed last of %d event lines; %d remain" % (len(lines), len(lines)-1))
PY
printf -- '-- verify without receipt (should PASS) --\n'
expect_verify_pass "$COPY_DEL_TIP" "$TARGET/outputs-del-tip" "deleted last event, no receipt"
printf 'PASS deleted last event without receipt: verify passed\n'
printf -- '-- verify with -expect-tip and -expect-count (should FAIL) --\n'
FAIL_MSG=""
expect_verify_fail "$COPY_DEL_TIP" "$TARGET/outputs-del-tip" "deleted last event with receipt" \
  -expect-tip "$TIP_HASH" -expect-count "$EVENT_COUNT"
printf '%s\n' "$FAIL_MSG" | grep -q "expected tip $TIP_HASH" \
  || die "tip-receipt error did not name expected tip: $FAIL_MSG"
printf '%s\n' "$FAIL_MSG" | grep -q "expected count $EVENT_COUNT" \
  || die "tip-receipt error did not name expected count: $FAIL_MSG"
printf 'PASS deleted last event with -expect-tip/-expect-count: verify failed naming expected vs actual\n'

printf '\n== ALL DEMOS PASSED ==\n'
printf 'IDN=%s IDN2=%s\n' "$IDN" "$IDN2"
printf 'receipt: %s\n' "$RECEIPT"
printf 'tip_hash=%s events=%s\n' "$TIP_HASH" "$EVENT_COUNT"
printf 'commit=%s\n' "$REQUIRED_SHORT"

if [ "${KEEP:-0}" = "1" ]; then
  printf 'KEEP=1: left throwaway tree at %s\n' "$TARGET"
  trap - EXIT
else
  rm -rf "$TARGET"
  trap - EXIT
  printf 'removed throwaway tree %s\n' "$TARGET"
fi
