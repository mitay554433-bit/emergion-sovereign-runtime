# UNIFUSION as a phone app

fieldd serves the Living Universe at `http://127.0.0.1:8899/living`. Three
pieces turn it into a home-screen app that starts without pasting anything.

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
pass straight through. If fieldd is down, opening the app shows a short
"fieldd is not running" page.

`http://127.0.0.1` counts as a secure context, so install works without HTTPS.

**To install:** open `/living` in Chrome, then menu (⋮) → **Add to Home screen**
or **Install app**.

Wire formats, the GOV gate, A01/A02 and HUMAN_FINAL are unchanged. Nothing is
drawn over the field render.

## 2. Start on boot (Termux:Boot)

`scripts/termux-boot-fieldd.sh`, installed to `~/.termux/boot/`:

- does nothing if fieldd already answers on 127.0.0.1:8899
- builds `$TMPDIR/fieldd` only if it is missing or built from a different commit
- starts `fieldd -http 127.0.0.1:8899` from `~/AXM/emergion-sovereign-runtime`
- logs to `$TMPDIR/fieldd-boot.log` and `$TMPDIR/fieldd.log`
- never edits, resets or cleans the repo, and never touches `.field`

## 3. One-tap shortcut (Termux:Widget)

`scripts/termux-widget-unifusion.sh`, installed as `~/.shortcuts/UNIFUSION`,
starts fieldd if it is not running and opens `/living` with `termux-open-url`.
Add the Termux:Widget widget to the home screen and tap **UNIFUSION**.

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

Settings (all optional): `UNIFUSION_REPO` for the repo path, `FIELDD_ADDR` for
the bind address (default `127.0.0.1:8899`).
