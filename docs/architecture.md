# Architecture

Inoichi is a Go Web Only project: one `serve` command, an embedded single-page frontend, and JSON files on disk. There is no database, no build step for the frontend, and no process other than the binary.

## Tree

```
main.go                       cmd.Execute()
cmd/root.go                   zerolog setup, --debug, AppVersion
cmd/serve.go                  the one command, flags and environment defaults
internal/mindmap/             the data model: types, validation, tree operations, layout, sample
internal/storage/             one JSON file per map, atomic writes, id guarding
internal/server/              routes, handlers, //go:embed static
internal/server/static/       index.html, app.js, downloaded assets, icons
test/e2e_test.go              the headless-browser smoke test, behind the e2e build tag
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

`Validate` is the single gate. Every write path calls `Normalize` then `Validate` before anything reaches disk, so a hand-edited file, an imported file, and an autosave all pass the same checks:

- ids match `^[A-Za-z0-9_-]{1,64}$`, which is what makes `<id>.json` safe to join onto the data directory
- ids are unique, exactly one node has no parent, and `rootId` names that node
- every `parentId` exists, and the parent chain has no cycle
- every link joins two existing and different nodes
- text, note, node count, link count, coordinates and sizes are inside their limits

`Normalize` clamps rather than rejects where a value can be repaired: a non-finite coordinate becomes zero, a zero width becomes the default, an unknown accent is dropped.

`MaxDocumentBytes` is the one size ceiling. The request body limit, the write check and the read limit all take it from the model, so a map that is accepted is always a map that can be read back.

## Storage

`storage.Store` writes `<data-dir>/maps/<id>.json`. There is no `Store` interface, because there is one implementation and an interface with one implementation costs a jump and buys nothing.

Writes go to a temp file in the same directory, get `fsync`ed, and are then renamed over the target, so a crash mid-write leaves the previous version intact rather than a truncated file. The directory is created at `0700` and every file at `0600`.

`List` loads each file to build its summary and skips any file that fails to parse, so one corrupt map does not hide the rest. It returns the skipped filenames alongside the summaries rather than logging them itself, because a task package does not decide how the program talks to its user; the handler logs them.

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
| `DELETE /api/maps/{id}` | remove the file |
| `POST /api/layout` | return the posted map with fresh coordinates, saving nothing |
| `GET /static/` | the embedded asset tree |
| `GET /` | `index.html`, on the catch-all so a deep link still loads the app |

`PUT` keeps the server's own `id`, `createdAt` and `sample` rather than the client's, so a client cannot rewrite a map's identity or relabel the sample. It also compares the incoming `updatedAt` against the stored one and answers `409` on a mismatch, which is what stops a second tab holding stale state from overwriting the first tab's edits.

Every mutating route refuses a request whose `Origin` header names something other than the server's own host. A simple cross-origin `POST` needs no preflight, so without that check a page on any origin could create maps on a running instance.

Layout lives behind an endpoint rather than in `app.js` because it is the one piece of geometry the server also needs, for the sample map. One implementation means a tidied map and a seeded map are laid out by the same code.

## Frontend

One `index.html` and one `app.js`, no framework and no bundler.

Nodes are absolutely positioned `div` elements inside a `#scene` that carries a single `translate` and `scale` transform, with an SVG layer underneath drawing the edges in the same coordinate space. HTML nodes rather than SVG shapes is what makes text editing, focus rings, wrapping and screen-reader roles work without reimplementing any of them.

- **Pan and zoom** are that one transform. Screen-to-map conversion is `(screen - origin - translate) / scale`.
- **Selection** is a roving `tabindex` over `role="treeitem"` elements inside a `role="tree"`, so arrow keys and `Tab` both behave.
- **Undo** is a snapshot stack of deep-cloned maps, capped at 60. Snapshot cloning beats a command log here because every mutation is small and the whole map is a few kilobytes.
- **Autosave** debounces 700ms, collapses a save requested during an in-flight save into one follow-up, and surfaces failure as a retry rather than a lost edit. A revision counter stops a completing save from reporting "Saved" over an edit made while it was in flight, and switching or creating a map flushes the pending save first.
- **Lookups** go through one `id -> node` and `parent -> children` index rebuilt per render, so a drag does not rescan the node list on every pointer move.
- **Node heights** are read back from the DOM after each render, so wrapped text, the drawn edges, and the exported SVG all agree on where a node ends.
- **Export** builds the SVG from the model rather than from the DOM, wrapping text with a canvas measurement. PNG is that SVG drawn onto a canvas, so both exports come from one code path.

Styling is Tailwind utility classes through the browser build, on the Catppuccin Mocha palette, with `crust` as the page ground. Hand-written CSS is limited to what has no utility form: `@font-face`, scrollbar pseudo-elements, the editing caret, and one keyframe.

## Assets

`css/`, `js/` and `fonts/` are downloaded by `make assets` at pinned versions and are never committed. `//go:embed static` then compiles whatever is there into the binary, which is why `make build` depends on `assets` and `verify-assets` fails early when a file is missing.

The asset step keeps only the `latin` and `latin-ext` blocks of each Google Fonts stylesheet and rewrites the URLs to `/static/fonts/`, so nothing is fetched from a third party at run time.

## Tests

`go test ./...` covers the data model and the storage layer: the guards that must stay impossible, and the save-and-load round trip.

`make smoke` builds the binary, starts it on a temp data directory, and drives Chrome through the actual core loop: create a map, add a child with the keyboard, wait for autosave to reach disk, reload, confirm the node survived, export an SVG through the toolbar, and fail on any console error along the way. It carries the `e2e` build tag so it never runs as part of the unit suite, and it skips with a clear message when no browser is installed.
