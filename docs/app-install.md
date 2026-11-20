# UNIFUSION as a phone app

fieldd serves the Living Universe at `http://127.0.0.1:8899/living`. Three
pieces turn it into a home-screen app that starts without pasting anything.
A home-screen PWA cannot launch a Termux program, so fieldd is kept running
by a Termux:Boot watchdog rather than by the app icon itself.

## 1. Installable app (PWA)

fieldd serves a web manifest and a small service worker:

| Route | Type | Notes |
|---|---|---|
| `/manifest.webmanifest` | `application/manifest+json` | name UNIFUSION, standalone, carbon black `#0a0a0a` |
| `/sw.js` | `text/javascript` | caches only the shell |
| `/icon.svg`, `/icon-192.png`, `/icon-512.png` | SVG / PNG | royal blue on carbon black, generated in `cmd/fieldd/pwa.go` |

The service worker caches only the manifest and icons. It never caches
`/living`, `/field.json`, `/projection.json`, `/events.json`, `/decide`,
`/decide-batch-human`, `/authorize` or any other data route. `/living` is
always fetched with `no-store`, so the `fieldd-token` meta is injected fresh on
every loopback request with an exact Host match, as before. Non-GET requests
pass straight through. If fieldd is down, opening the app shows "fieldd is
restarting, try again in a few seconds" and retries `/status` every 5 seconds
(with `no-store`). When `/status` returns 200, the page reloads.

`http://127.0.0.1` counts as a secure context, so install works without HTTPS.

**To install:** open `/living` in Chrome, then menu (⋮) → **Add to Home screen**
or **Install app**.

Wire formats, the GOV gate, A01/A02 and HUMAN_FINAL are unchanged. Nothing is
drawn over the field render.

## 2. Keep fieldd running (Termux:Boot watchdog)

`scripts/termux-boot-fieldd.sh`, installed to `~/.termux/boot/`, starts
`scripts/fieldd-watchdog.sh` if it is not already running. The watchdog:

- takes `termux-wake-lock` when that command exists, so Android is less
  likely to freeze Termux
- runs as a single instance, guarded by a pid and lock file in `$TMPDIR`
- checks `http://127.0.0.1:8899/status` every 10 seconds
- starts or restarts `$TMPDIR/fieldd -http 127.0.0.1:8899` from the repo
  directory when the status is not 200, with backoff after repeated failures
- rebuilds `$TMPDIR/fieldd` only if it is missing or built from a different
  commit
- logs to `$TMPDIR/fieldd-watchdog.log` (capped) and `$TMPDIR/fieldd.log`
- never edits, resets or cleans the repo, never touches `.field`, and never
  passes `-state`

**Turn off battery optimization for Termux.** Android may stop Termux after a
few minutes unless it is allowed to run in the background. On the phone open
Settings → Apps → Termux → Battery (or App battery usage) and set it to
**Unrestricted** or **Don't optimize**. If Termux:Boot appears as its own
app, do the same for it. Also open the Termux:Boot app once so Android lets
it run at boot.

## 3. One-tap shortcut (Termux:Widget)

`scripts/termux-widget-unifusion.sh`, installed as `~/.shortcuts/UNIFUSION`,
starts the watchdog if it is not running and then opens `/living` with
`termux-open-url`. Add the Termux:Widget widget to the home screen and tap
**UNIFUSION**. If fieldd is still coming up, the app page retries until it
answers.

Termux:Boot and Termux:Widget are separate apps from F-Droid. Install them from
the same source as Termux, and open Termux:Boot once so Android lets it run at
boot.

## Install commands

```sh
mkdir -p ~/.termux/boot ~/.shortcuts
cp scripts/termux-boot-fieldd.sh ~/.termux/boot/termux-boot-fieldd.sh
cp scripts/termux-widget-unifusion.sh ~/.shortcuts/UNIFUSION
chmod +x ~/.termux/boot/termux-boot-fieldd.sh ~/.shortcuts/UNIFUSION
```

The watchdog script stays in the repo and is started from there; it does not
need to be copied into `~/.termux/boot`. After a `git checkout` of a newer
commit, the next watchdog loop will rebuild fieldd if the commit SHA changed.

Settings (all optional): `UNIFUSION_REPO` for the repo path, `FIELDD_ADDR` for
the bind address (default `127.0.0.1:8899`), `FIELDD_WATCHDOG_INTERVAL` for the
health-check period in seconds (default `10`).
