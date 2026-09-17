# Architecture

Inoichi is a Go Web Only project: one `serve` command, an embedded single-page frontend, and JSON files on disk. There is no database, no build step for the frontend, and no process other than the binary.

## Tree

```
main.go                       cmd.Execute()
cmd/root.go                   zerolog setup, --debug, AppVersion
cmd/serve.go                  the one command, flags, environment defaults, signal handling
internal/mindmap/             the data model: types, validation, tree operations, layout, sample
internal/storage/             every map in memory, write-behind to one JSON file per map
internal/server/              routes, handlers, graceful shutdown, //go:embed static
internal/server/static/       index.html, app.js, sw.js, manifest, downloaded assets, icons
test/e2e_test.go              the headless-browser smoke test, behind the e2e build tag
.github/workflows/            CI on pull requests, release on every push to main
```

`internal/mindmap` and `internal/storage` are task packages: they return errors unwrapped and log nothing, so either could move to `pkg/` unchanged. `internal/server` is the interaction boundary, and it is the only package that logs or turns an error into a status code.

## The data model

A map is a title, a root id, a flat list of nodes, and a flat list of cross links.

```
Map { id, title, rootId, nodes[], links[], sample, schema, createdAt, updatedAt }
Node { id, parentId, text, note, x, y, width, height, accent, collapsed }
Link { id, from, to, label }
```

Nodes are flat rather than nested because every operation the editor performs is a lookup or a filter, and both are awkward against a nested tree. `parentId` carries the hierarchy, and the root is the one node whose `parentId` is empty.

`note` is the node's Markdown document. There is no separate node type: a node with a non-empty note shows a page icon and opens rendered, and a node with an empty one is a plain label.

`Validate` is the single gate. Every write path calls `Normalize` then `Validate` before anything reaches the store, so a hand-edited file, an imported file, and an autosave all pass the same checks:

- ids match `^[A-Za-z0-9_-]{1,64}$`, which is what makes `<id>.json` safe to join onto the data directory
- ids are unique, exactly one node has no parent, and `rootId` names that node
- every `parentId` exists, and the parent chain has no cycle
- every link joins two existing and different nodes
- text, note, node count, link count, coordinates and sizes are inside their limits

`Normalize` clamps rather than rejects where a value can be repaired: a non-finite coordinate becomes zero, a zero width becomes the default, an unknown accent is dropped.

`MaxDocumentBytes` is the one size ceiling. The request body limit, the save check and the read limit all take it from the model.

## Storage

`storage.Store` owns `<data-dir>/maps/<id>.json`. There is no `Store` interface, because there is one implementation and an interface with one implementation costs a jump and buys nothing.

`New` reads every file once into memory. From then on `List`, `Load` and `Count` answer from memory, and `Load` hands out a copy so a handler mutating the map it received cannot change the cache. A file that fails to parse is remembered by name so `List` can still report it, and one corrupt map does not hide the rest.

`Save` stores a copy, marks the id dirty, and returns. One goroutine writes dirty maps to disk `FlushDelay` after the first save in a burst, so a run of autosaves costs one write. A write that fails keeps the map dirty, reports through `OnWriteError` (the server logs it), and retries with a doubling delay. `Close` writes everything still dirty and is what the command calls after the HTTP server has drained on SIGINT or SIGTERM.

Writes go to a temp file in the same directory, get `fsync`ed, and are then renamed over the target, so a crash mid-write leaves the previous version intact rather than a truncated file. The directory is created at `0700` and every file at `0600`. `Delete` removes the file and the cache entry immediately, under the same lock the writer holds, so a delete cannot interleave with a rename that would bring the file back.

## HTTP

The standard library's `ServeMux` with method patterns. No third-party router.

| Route | Does |
|---|---|
| `GET /api/health` | status, version, and the resolved data directory |
| `GET /api/maps` | summaries, newest first |
| `POST /api/maps` | create from a title |
| `POST /api/maps/import` | validate and store a posted map under a fresh id |
| `GET /api/maps/{id}` | load one map |
| `PUT /api/maps/{id}` | replace one map, which is what autosave calls |
| `DELETE /api/maps/{id}` | remove the map |
| `POST /api/layout` | return the posted map with fresh coordinates, saving nothing |
| `GET /manifest.webmanifest` | the web app manifest, served at the root so the app installs |
| `GET /sw.js` | the service worker, served at the root so its scope covers the app, never cached |
| `GET /static/` | the embedded asset tree |
| `GET /` | `index.html`, on the catch-all so a deep link still loads the app |

`PUT` keeps the server's own `id`, `createdAt` and `sample` rather than the client's, so a client cannot rewrite a map's identity or relabel the sample. It also compares the incoming `updatedAt` against the stored one and answers `409` on a mismatch, which is what stops a second tab holding stale state from overwriting the first tab's edits.

Every mutating route refuses a request whose `Origin` header names something other than the server's own host. A simple cross-origin `POST` needs no preflight, so without that check a page on any origin could create maps on a running instance.

Layout lives behind an endpoint rather than in `app.js` because it is the one piece of geometry the server also needs, for the sample map. One implementation means a tidied map and a seeded map are laid out by the same code.

`Run` takes a context. When it is cancelled the server stops accepting connections and drains open ones for up to ten seconds, and the command then closes the store.

## Frontend

One `index.html` and one `app.js`, no framework and no bundler.

Nodes are absolutely positioned `div` elements inside a `#scene` that carries a single `translate` and `scale` transform, with an SVG layer underneath drawing the edges in the same coordinate space. HTML nodes rather than SVG shapes is what makes text editing, focus rings, wrapping and screen-reader roles work without reimplementing any of them.

- **Colour** is one accent per node, inherited down the tree. The node's fill is that accent mixed into the canvas ground, the edge into it is the accent at reduced opacity, and there is no border. The mix is computed in JavaScript so the DOM and the SVG export agree on the exact fill.
- **Chrome** sits flush on the `crust` ground with no separator lines, and the canvas is the one surface lifted to `mantle`. The three top bars share one height so their contents align.
- **Panels** are the map list on the left and the node panel on the right. Both hide, both resize by dragging their inner edge, and their widths live in `localStorage`. The node panel opens with a selection and closes with a deselect, so it is only there when there is a node to edit.
- **Pan and zoom** are that one transform. Screen-to-map conversion is `(screen - origin - translate) / scale`. Fitting the map to the screen changes the view only and never saves.
- **Selection** is a roving `tabindex` over `role="treeitem"` elements inside a `role="tree"`, so arrow keys and `Tab` both behave. With nothing selected, `Tab`, `Enter` and the arrows select the root first.
- **Undo** is a snapshot stack of deep-cloned maps, capped at 60. Snapshot cloning beats a command log here because every mutation is small and the whole map is a few kilobytes. Node panel edits update the node live and push one undo step when the field commits.
- **Saving** waits one second after the last change and never more than five seconds after the first, collapses a save requested during an in-flight save into one follow-up, and updates the map list from the response rather than refetching it. The server's `updatedAt` is kept outside the map as `S.version` and stamped onto each request, because a snapshot restored by undo would otherwise carry a stale token and every later save would answer `409`. Switching or creating a map flushes the pending save first. The status is one small icon: a blip on success, a red mark that retries on click after a failure.
- **Markdown** is rendered by marked with a renderer that highlights fenced code through highlight.js, slugs heading ids, and turns `[!TIP]`-style blockquotes into callouts. The output is sanitised by DOMPurify before it reaches the modal, then copy buttons are added to each code block and Lucide draws the callout icons. Links open in a new tab.
- **Lookups** go through one `id -> node` and `parent -> children` index rebuilt per render, so a drag does not rescan the node list on every pointer move.
- **Node heights** are read back from the DOM after each render, so wrapped text, the drawn edges, and the exported SVG all agree on where a node ends.
- **Export** builds the SVG from the model rather than from the DOM, wrapping text with a canvas measurement. PNG is that SVG drawn onto a canvas, so both exports come from one code path.
- **Desktop only.** A portrait or narrow viewport shows a full-screen notice over the app. There is no touch handling.

Styling is Tailwind utility classes through the browser build, on the Catppuccin Mocha palette. Hand-written CSS covers what has no utility form: `@font-face`, scrollbar pseudo-elements, the node fill and selection ring driven by a custom property, the Markdown typography, and the status keyframes.

## Progressive web app

`manifest.webmanifest` and `sw.js` sit in the static tree but are served from the root, because a manifest under `/static/` would not install the app and a service worker under `/static/` would only control that path. The worker is a no-op: it registers so the app is installable and caches nothing, so every load shows the running binary's version.

## Assets

`css/`, `js/` and `fonts/` are downloaded by `make assets` at pinned versions and are never committed. `//go:embed static` then compiles whatever is there into the binary, which is why `make build` depends on `assets` and `verify-assets` fails early when a file is missing.

The asset step keeps only the `latin` and `latin-ext` blocks of each Google Fonts stylesheet and rewrites the URLs to `/static/fonts/`, so nothing is fetched from a third party at run time.

## Tests and delivery

`go test ./...` pins the guards that must stay impossible: the ids `Validate` rejects, the ids the store refuses to turn into paths, a deleted map never reaching disk, a foreign `Origin` being refused, and an out-of-range `INOICHI_PORT` falling back to the default.

`make smoke` builds the binary, starts it on a temp data directory, and drives Chrome through the actual core loop: create a map, add a child with the keyboard, wait for it to reach disk, undo past that save and add another child that must also reach disk, write Markdown on a node and open it rendered, reload, confirm the node survived, export an SVG through the toolbar, and fail on any console error along the way. It carries the `e2e` build tag so it never runs as part of the unit suite, and it skips with a clear message when no browser is installed.

The `CI` workflow runs vet, the unit tests, the build and the smoke test on every pull request. The `Release` workflow runs on every push to `main`: the same checks gate it, then it derives the next version from the last tag and the commit message (`[minor-release]` or `[major-release]` bump more than the patch), cuts a draft release, uploads the four binaries and pushes a multi-arch image to Docker Hub, and publishes the draft once every artifact has landed. A failed artifact job deletes the draft instead.
