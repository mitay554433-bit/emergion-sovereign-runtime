package main

// Installable app (PWA) shell for the Living Universe.
//
// These routes serve only static, token-free assets: the web manifest, the
// service worker and the app icons. Nothing here reads REG, the field, the
// projection or events, and nothing here touches the GOV gate, A01/A02 or
// HUMAN_FINAL. The service worker caches only these shell assets; field
// data, decide routes and the token-injected /living HTML always go to the
// network and are never stored.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"strings"
	"sync"
)

const (
	pwaBackground = "#0a0a0a" // carbon black
	pwaRoyal      = "#2f6bff" // royal blue (matches --royal in living-universe.html)
	pwaCacheName  = "unifusion-shell-v3"
)

// pwaShellAssets is the complete list of URLs the service worker may cache.
var pwaShellAssets = []string{
	"/manifest.webmanifest",
	"/icon.svg",
	"/icon-192.png",
	"/icon-512.png",
}

// pwaNeverCache lists routes the service worker must never store. The worker
// passes them straight through to fieldd.
var pwaNeverCache = []string{
	"/living",
	"/living-universe",
	"/field.json",
	"/projection.json",
	"/events.json",
	"/real_temporal_wave.json",
	"/status",
	"/actions",
	"/authorize",
	"/decide",
	"/decide-batch-human",
	"/field.html",
	"/deliverables/",
}

const pwaManifest = `{
  "name": "UNIFUSION",
  "short_name": "UNIFUSION",
  "id": "/living",
  "start_url": "/living",
  "scope": "/",
  "display": "standalone",
  "orientation": "any",
  "background_color": "` + pwaBackground + `",
  "theme_color": "` + pwaBackground + `",
  "icons": [
    {"src": "/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
    {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
    {"src": "/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "maskable"},
    {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
    {"src": "/icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"}
  ]
}
`

const pwaIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
<defs><radialGradient id="c" cx="50%" cy="45%" r="55%"><stop offset="0" stop-color="#c9d8ff"/><stop offset=".45" stop-color="#2f6bff"/><stop offset="1" stop-color="#1d4ed8"/></radialGradient></defs>
<rect width="512" height="512" fill="#0a0a0a"/>
<circle cx="256" cy="256" r="150" fill="none" stroke="#2f6bff" stroke-width="14" opacity=".9"/>
<circle cx="256" cy="256" r="64" fill="url(#c)"/>
<circle cx="362" cy="150" r="20" fill="#1d4ed8"/>
</svg>
`

func pwaServiceWorkerJS() string {
	q := func(list []string) string {
		parts := make([]string, len(list))
		for i, s := range list {
			parts[i] = "'" + s + "'"
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return `// UNIFUSION shell service worker. Caches only the static app shell.
// Field data, decide routes and the /living page (which carries a fresh
// fieldd-token on every loopback request) are never cached.
const CACHE = '` + pwaCacheName + `';
const SHELL = ` + q(pwaShellAssets) + `;
const NEVER = ` + q(pwaNeverCache) + `;
self.addEventListener('install', e => {
  e.waitUntil(caches.open(CACHE).then(c => c.addAll(SHELL)).then(() => self.skipWaiting()));
});
self.addEventListener('activate', e => {
  e.waitUntil(caches.keys().then(ks => Promise.all(ks.filter(k => k !== CACHE).map(k => caches.delete(k)))).then(() => self.clients.claim()));
});
function offline() {
  return new Response('<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="theme-color" content="#0a0a0a"><title>UNIFUSION</title><body style="margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#0a0a0a;color:#c9d8ff;font:16px system-ui,sans-serif;text-align:center"><div><div style="color:#2f6bff;font-size:28px">&#9672;</div><p>fieldd is restarting, try again in a few seconds</p></div><script>function peek(){fetch("/status",{cache:"no-store"}).then(function(r){if(r.status===200)location.reload()}).catch(function(){})}setInterval(peek,5000);peek();</script>',
    {status: 503, headers: {'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store'}});
}
self.addEventListener('fetch', e => {
  const req = e.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (SHELL.includes(url.pathname)) {
    e.respondWith(caches.match(req).then(hit => hit || fetch(req)));
    return;
  }
  if (req.mode === 'navigate' && (url.pathname === '/living' || url.pathname === '/living-universe')) {
    e.respondWith(fetch(req, {cache: 'no-store'}).catch(offline));
    return;
  }
  // Everything else (NEVER list included) goes straight to fieldd, uncached.
});
`
}

var (
	pwaPNGOnce  sync.Once
	pwaPNG192   []byte
	pwaPNG512   []byte
	pwaSWSource = pwaServiceWorkerJS()
)

func hexRGBA(h string) color.NRGBA {
	var c color.NRGBA
	c.A = 255
	v := strings.TrimPrefix(h, "#")
	parse := func(s string) uint8 {
		var n uint8
		for _, ch := range s {
			n <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				n |= uint8(ch - '0')
			case ch >= 'a' && ch <= 'f':
				n |= uint8(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				n |= uint8(ch-'A') + 10
			}
		}
		return n
	}
	c.R, c.G, c.B = parse(v[0:2]), parse(v[2:4]), parse(v[4:6])
	return c
}

func blend(dst color.NRGBA, src color.NRGBA, a float64) color.NRGBA {
	if a <= 0 {
		return dst
	}
	if a > 1 {
		a = 1
	}
	mix := func(d, s uint8) uint8 { return uint8(math.Round(float64(d)*(1-a) + float64(s)*a)) }
	return color.NRGBA{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), 255}
}

// renderPWAIcon draws the same mark as pwaIconSVG: a royal blue orbit, a
// blue core and one satellite on carbon black. All content sits inside the
// maskable safe zone (radius 0.4 of the icon).
func renderPWAIcon(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	bg, royal, deep, pale := hexRGBA(pwaBackground), hexRGBA(pwaRoyal), hexRGBA("#1d4ed8"), hexRGBA("#c9d8ff")
	s := float64(size) / 512
	cx, cy := 256*s, 256*s
	ringR, ringW := 150*s, 7*s
	coreR := 64 * s
	satX, satY, satR := 362*s, 150*s, 20*s
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			c := bg
			d := math.Hypot(px-cx, py-cy)
			c = blend(c, royal, (ringW-math.Abs(d-ringR)+0.5)*0.9)
			if d < coreR+1 {
				t := math.Hypot(px-cx, py-(cy-0.1*coreR)) / coreR
				var g color.NRGBA
				if t < 0.45 {
					g = blend(pale, royal, t/0.45)
				} else {
					g = blend(royal, deep, math.Min(1, (t-0.45)/0.55))
				}
				c = blend(c, g, coreR-d+0.5)
			}
			c = blend(c, deep, satR-math.Hypot(px-satX, py-satY)+0.5)
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func pwaIcons() ([]byte, []byte) {
	pwaPNGOnce.Do(func() {
		pwaPNG192 = renderPWAIcon(192)
		pwaPNG512 = renderPWAIcon(512)
	})
	return pwaPNG192, pwaPNG512
}

func pwaStatic(contentType, cacheControl string, body func() []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if req.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body())
	}
}

// registerPWARoutes adds the installable-app shell routes. They carry no
// field data and no token, and follow the same loopback/bearer policy as
// the read routes via protect.
func registerPWARoutes(mux *http.ServeMux, protect func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/manifest.webmanifest", protect(pwaStatic("application/manifest+json", "no-cache", func() []byte { return []byte(pwaManifest) })))
	mux.HandleFunc("/sw.js", protect(pwaStatic("text/javascript; charset=utf-8", "no-cache", func() []byte { return []byte(pwaSWSource) })))
	mux.HandleFunc("/icon.svg", protect(pwaStatic("image/svg+xml", "public, max-age=86400", func() []byte { return []byte(pwaIconSVG) })))
	mux.HandleFunc("/icon-192.png", protect(pwaStatic("image/png", "public, max-age=86400", func() []byte { a, _ := pwaIcons(); return a })))
	mux.HandleFunc("/icon-512.png", protect(pwaStatic("image/png", "public, max-age=86400", func() []byte { _, b := pwaIcons(); return b })))
}
