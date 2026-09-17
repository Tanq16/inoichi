const VERSION = "1";
const CACHE = "inoichi-shell-" + VERSION;

const SHELL = [
  "/",
  "/static/app.js",
  "/static/css/inter.css",
  "/static/css/google-sans.css",
  "/static/css/jetbrains-mono.css",
  "/static/js/tailwind.js",
  "/static/js/lucide.min.js",
  "/static/js/marked.js",
  "/static/js/purify.min.js",
  "/static/icons/favicon.ico",
  "/static/icons/favicon.png",
  "/static/icons/apple-touch-icon.png",
  "/static/icons/logo.png",
  "/static/icons/icon-192.png",
  "/static/icons/icon-512.png",
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;

  if (req.mode === "navigate") {
    event.respondWith(networkFirst(req));
    return;
  }
  if (url.pathname.startsWith("/static/")) {
    event.respondWith(cacheFirst(req));
  }
});

async function networkFirst(req) {
  const cache = await caches.open(CACHE);
  try {
    const res = await fetch(req);
    if (res.ok) cache.put("/", res.clone());
    return res;
  } catch (err) {
    const hit = await cache.match("/");
    if (hit) return hit;
    throw err;
  }
}

async function cacheFirst(req) {
  const cache = await caches.open(CACHE);
  const hit = await cache.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res.ok) cache.put(req, res.clone());
  return res;
}
