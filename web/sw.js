// TUI Minigames — service worker.
//
// Strategy:
//   - Cache-first for the app shell (this page, the play page, CSS, JS
//     modules, the wasm runtime + payload, icons/manifest): these are
//     versioned together and safe to serve straight from cache, refreshing
//     the cache in the background on each hit.
//   - Network-first, falling back to cache, for everything else (e.g. any
//     request this list doesn't know about yet): try the network so
//     newly-added assets aren't stuck stale, but still work offline once
//     they've been fetched once.
//
// All paths are relative ('./...') so this works when the site is served
// from a subpath (e.g. https://jensen95.github.io/tui-games/).
//
// Bump CACHE_VERSION on every deploy that changes any precached file — it's
// the only thing that busts the old cache.
const CACHE_VERSION = "v1";
const CACHE_NAME = `tui-minigames-${CACHE_VERSION}`;

// The app shell. play.html and its JS are owned by a different part of this
// project and are expected to exist under these names by deploy time.
// js/api.md is documentation, not a runtime asset, and is deliberately not
// precached.
const APP_SHELL = [
  "./",
  "./index.html",
  "./play.html",
  "./manifest.webmanifest",
  "./favicon.svg",
  "./css/style.css",
  "./css/play.css",
  "./icons/icon-192.png",
  "./icons/icon-512.png",
  "./icons/icon-192-maskable.png",
  "./icons/icon-512-maskable.png",
  "./js/app.js",
  "./js/engine.js",
  "./js/wasm_exec.js",
  "./lig.wasm",
  "./js/games/tango.js",
  "./js/games/queens.js",
  "./js/games/minisudoku.js",
  "./js/games/zip.js",
  "./js/games/patches.js",
];

// Resolve the shell list against this worker's own scope so it's correct
// whether the site is served from "/" or from a GitHub Pages subpath.
function shellUrls() {
  return APP_SHELL.map((path) => new URL(path, self.registration.scope).href);
}

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE_NAME);
      // Precache best-effort: a single missing/unavailable file (e.g. a
      // sibling agent's asset not yet deployed) must not block install.
      await Promise.all(
        shellUrls().map(async (url) => {
          try {
            const res = await fetch(url, { cache: "no-cache" });
            if (res && res.ok) {
              await cache.put(url, res);
            }
          } catch (err) {
            /* offline install / missing asset — skip, non-fatal */
          }
        })
      );
      await self.skipWaiting();
    })()
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys
          .filter((key) => key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      );
      await self.clients.claim();
    })()
  );
});

function isAppShellRequest(request) {
  const url = request.url.split("#")[0].split("?")[0];
  return shellUrls().some((shellUrl) => shellUrl.split("?")[0] === url);
}

async function cacheFirst(request) {
  const cache = await caches.open(CACHE_NAME);
  const cached = await cache.match(request, { ignoreSearch: true });
  if (cached) {
    // Refresh in the background so the next load picks up changes without
    // making this load wait on the network.
    fetch(request)
      .then((res) => {
        if (res && res.ok) cache.put(request, res.clone());
      })
      .catch(() => {});
    return cached;
  }
  try {
    const res = await fetch(request);
    if (res && res.ok) cache.put(request, res.clone());
    return res;
  } catch (err) {
    return cached || Response.error();
  }
}

async function networkFirst(request) {
  const cache = await caches.open(CACHE_NAME);
  try {
    const res = await fetch(request);
    if (res && res.ok && request.method === "GET") {
      cache.put(request, res.clone());
    }
    return res;
  } catch (err) {
    const cached = await cache.match(request, { ignoreSearch: true });
    if (cached) return cached;
    throw err;
  }
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;

  // Never intercept cross-origin requests (fonts CDN, analytics, etc. — this
  // project has none, but stay defensive); let the browser handle them.
  if (new URL(request.url).origin !== self.location.origin) return;

  if (isAppShellRequest(request)) {
    event.respondWith(cacheFirst(request));
  } else {
    event.respondWith(networkFirst(request));
  }
});
