<div align="center">
  <img src=".github/assets/logo.png" alt="Inoichi Logo" width="200">
  <h1>Inoichi</h1>

  <a href="#features">Features</a> &bull; <a href="#install">Install</a> &bull; <a href="#usage">Usage</a> &bull; <a href="#security">Security</a> &bull; <a href="#notes">Notes</a>
</div>

---

Inoichi is a local-first mind mapping editor. One Go binary serves the whole app, and every map is a JSON file in a directory you choose.

It exists to replace the part of Xmind most people actually use: draw a tree, move it around, get it out again. It is not a collaboration tool, a template gallery, or an Xmind file reader.

## Features

| Area | What you get |
|---|---|
| Editing | Drag, resize, reparent, cross-link, collapse, accent colours, per-node notes |
| Keyboard | Tab, Enter, F2, Delete, arrow-key navigation, undo and redo, zoom, tidy layout |
| Persistence | Debounced autosave to disk, with a visible saved, saving, unsaved and failed state |
| Getting out | Export JSON, SVG or PNG, and import any JSON this app exported |
| Deployment | One static binary with the frontend embedded, or a container image |

Everything is single-user by design. There are no accounts, no sharing, no telemetry, and no outbound network requests at run time.

## Screenshots

<details>
<summary>Click to expand</summary>

### Editor

| Desktop | Mobile |
| :---: | :---: |
| <img src=".github/assets/editor-desktop.png" alt="Editor on a desktop screen" width="100%" /> | <img src=".github/assets/editor-mobile.png" alt="Editor on a phone screen" width="100%" /> |

</details>

## Install

### From source

Needs Go 1.27 and `curl`. `make build` downloads the pinned frontend assets, which are never committed, then compiles them into the binary.

```bash
git clone https://github.com/tanq16/inoichi.git
cd inoichi
make build
./inoichi serve
```

`make build-all` produces `linux/amd64`, `linux/arm64`, `darwin/amd64` and `darwin/arm64` binaries instead.

### Docker

No image is published yet, so build one from the checkout:

```bash
make docker-build
mkdir -p $HOME/.inoichi
```
```bash
docker run -d --name inoichi \
  -p 8080:8080 \
  -v $HOME/.inoichi:/app/data \
  tanq16/inoichi:dev-build
```

Available at `http://localhost:8080`. The container runs as UID and GID 10001, so the mounted directory has to be writable by that user. The same setup as a compose file:

```yaml
services:
  inoichi:
    image: tanq16/inoichi:dev-build
    container_name: inoichi
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data # change as needed
```

## Usage

`inoichi serve` is the only command. Every flag reads an environment variable of the same name as its default, so `.env.example` and the flags describe one set of settings rather than two.

```bash
inoichi serve --host 127.0.0.1 --port 8080 --data-dir ~/.config/inoichi
```

| Flag | Environment | Default | Description |
|------|-------------|---------|-------------|
| `--host` | `INOICHI_HOST` | `127.0.0.1` | Address to bind |
| `--port` | `INOICHI_PORT` | `8080` | Port to listen on |
| `--data-dir` | `INOICHI_DATA_DIR` | `~/.config/inoichi` | Directory holding the map files |
| `--no-sample` | | off | Skip writing the sample map into an empty data directory |
| `--debug` | | off | Log at debug level |

Inoichi holds no secrets and needs no credentials, so `.env.example` carries only those three settings. Copy it to `.env` if you prefer environment variables to flags; `.env` itself is gitignored.

### Where your data lives

One JSON file per map at `<data-dir>/maps/<id>.json`, written at mode `0600` inside a directory at `0700`. The in-app shortcuts panel prints the resolved path, so you never have to guess which directory a running instance is using.

Back it up by copying that directory. Restore it by copying it back. A single map moves between machines through the JSON export and import buttons, which produce and accept the same format the files hold.

The first run on an empty data directory writes one sample map titled `Sample map (delete me)`. Delete it from the map list when you are done reading it; nothing else depends on it, and `--no-sample` skips it entirely.

### Editing

| Keys | Action |
|---|---|
| `Tab` | Add a child to the selected node |
| `Enter` | Add a sibling |
| `F2` or double click | Rename a node |
| `Delete` | Delete the node and its branch |
| Arrow keys | Walk parent, child and siblings |
| `Space` | Collapse or expand a branch |
| `Shift` + drag | Drop a node on another to reparent it |
| `Ctrl/Cmd` + `Z` / `Shift+Z` | Undo and redo |
| `Ctrl/Cmd` + `S` | Save now instead of waiting for autosave |
| `Ctrl/Cmd` + scroll | Zoom at the pointer |
| `Ctrl/Cmd` + `0` | Fit the map to the screen |
| `Ctrl/Cmd` + `Shift` + `L` | Tidy the layout |
| `?` | Open the shortcuts panel |

Dragging the dot on a selected node's left edge onto another node draws a cross link, which is the one relationship that does not follow the tree.

Edits autosave about a second after you stop. A failed save leaves the map on screen, marks the status red, and offers a retry rather than dropping the work.

Dragging the resize handle and drawing a cross link are pointer gestures with no keyboard equivalent. Everything the core loop needs (adding, renaming, navigating, collapsing, deleting, undo, zoom, tidy and export) is on the keyboard.

### Scripts

| Command | Does |
|---|---|
| `make install` | Resolve Go modules and download the pinned frontend assets |
| `make dev` | Run from source with debug logging against `./.devdata` |
| `make test` | Run the unit tests |
| `make smoke` | Build, then drive the real UI in a headless browser end to end |
| `make build` | Build the binary for this platform |
| `make run` | Build and serve exactly as a release would |

`make smoke` needs Chrome or Chromium. It finds the common install paths on its own, and `INOICHI_CHROME` points it somewhere else.

## Security

Inoichi has no login, because it is built for one person on one machine. Anything that can reach the port can read and edit every map, so the default bind is `127.0.0.1` and moving it to `0.0.0.0` puts your maps on your network with no authentication in front of them.

Put a reverse proxy with real authentication in front of it before exposing it anywhere. The server writes no map content to its logs, and it makes no outbound requests, so nothing about your maps leaves the process.

Open one map in one tab. The server rejects a save whose base version has moved on, so a second tab holding stale state gets an error rather than silently overwriting the first, but it will not merge the two.

Imported JSON is validated before it is stored: ids are checked against a strict format that cannot escape the data directory, parent references must exist, cycles are rejected, and text, node counts and coordinates are clamped to their limits.

## Notes

- **No Xmind interoperability.** Inoichi does not read or write `.xmind`, `.mm`, or OPML. Its JSON is the only format that round-trips.
- **Exported SVG and PNG use the reader's fonts.** No font is embedded, so an exported file falls back to the viewer's sans-serif if Inter is not installed.
- **One tree per map.** A node has exactly one parent, and cross links are decoration rather than a second hierarchy.
- **One tab at a time.** There is no merge. A save from a tab whose copy has fallen behind is refused with a conflict rather than applied.
- **Tailwind compiles in the browser.** That is the documented development mode rather than a build step, which is fine for a personal tool and means the first paint does a little work.
- **Layout runs on the server.** Tidying a map posts it to `/api/layout`, so there is one layout implementation rather than one per surface.
- **Architecture** is written up in [docs/architecture.md](docs/architecture.md) for anyone changing the code.
