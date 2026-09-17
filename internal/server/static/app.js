(() => {
  'use strict';

  const PALETTE = {
    rosewater: '#f5e0dc', flamingo: '#f2cdcd', pink: '#f5c2e7', mauve: '#cba6f7',
    red: '#f38ba8', maroon: '#eba0ac', peach: '#fab387', yellow: '#f9e2af',
    green: '#a6e3a1', teal: '#94e2d5', sky: '#89dceb', sapphire: '#74c7ec',
    blue: '#89b4fa', lavender: '#b4befe', text: '#cdd6f4', subtext0: '#a6adc8',
    overlay1: '#7f849c', overlay0: '#6c7086', surface2: '#585b70',
    surface1: '#45475a', surface0: '#313244', base: '#1e1e2e',
    mantle: '#181825', crust: '#11111b',
  };
  const ACCENTS = ['mauve', 'blue', 'green', 'peach', 'pink', 'teal', 'yellow', 'red', 'sapphire', 'lavender'];
  const LIMITS = { text: 512, note: 20000, title: 120, nodes: 2000 };
  const TYPE = {
    root: { size: 15, weight: 600, line: 20 },
    node: { size: 14, weight: 400, line: 20 },
    padX: 14, padY: 10, noteSlot: 22,
    family: 'Inter, system-ui, sans-serif',
  };
  const TINT = { node: 0.22, root: 0.36, edge: 0.75 };
  const SIZE = { minW: 80, maxW: 640, minH: 32, maxH: 640 };
  const ZOOM = { min: 0.2, max: 2.5 };
  const SAVE = { idleMs: 1000, maxWaitMs: 5000 };
  const PANEL = { sidebar: { min: 200, max: 480, initial: 272 }, inspector: { min: 240, max: 560, initial: 300 } };

  const $ = (id) => document.getElementById(id);
  const el = {
    sidebar: $('sidebar'), sidebarResizer: $('sidebar-resizer'), sidebarOpen: $('sidebar-open'), sidebarClose: $('sidebar-close'),
    mapList: $('map-list'), mapListLoading: $('map-list-loading'),
    mapListEmpty: $('map-list-empty'), mapListError: $('map-list-error'),
    mapListErrorDetail: $('map-list-error-detail'), mapListRetry: $('map-list-retry'),
    newMap: $('new-map'), emptyNewMap: $('empty-new-map'), importMap: $('import-map'), importFile: $('import-file'),
    title: $('map-title'), titleError: $('title-error'), saveStatus: $('save-status'),
    canvas: $('canvas'), scene: $('scene'), edges: $('edges'), nodes: $('nodes'),
    canvasLoading: $('canvas-loading'), canvasEmpty: $('canvas-empty'),
    canvasError: $('canvas-error'), canvasErrorDetail: $('canvas-error-detail'), canvasRetry: $('canvas-retry'),
    undo: $('btn-undo'), redo: $('btn-redo'), tidy: $('btn-tidy'),
    zoomIn: $('btn-zoom-in'), zoomOut: $('btn-zoom-out'), zoomReset: $('btn-zoom-reset'),
    exportBtn: $('btn-export'), exportMenu: $('export-menu'),
    inspector: $('inspector'), inspectorResizer: $('inspector-resizer'), inspClose: $('inspector-close'), toggleInspector: $('toggle-inspector'),
    inspText: $('insp-text'), inspNote: $('insp-note'), inspNoteOpen: $('insp-note-open'), inspAccents: $('insp-accents'),
    inspAddChild: $('insp-add-child'), inspDelete: $('insp-delete'),
    toasts: $('toasts'), live: $('live'),
    dialog: $('dialog'), dialogTitle: $('dialog-title'), dialogBody: $('dialog-body'),
    dialogInputWrap: $('dialog-input-wrap'), dialogLabel: $('dialog-label'),
    dialogInput: $('dialog-input'), dialogError: $('dialog-error'), dialogConfirm: $('dialog-confirm'),
    help: $('help'), helpClose: $('help-close'), helpKeys: $('help-keys'), showHelp: $('show-help'),
    md: $('md'), mdTitle: $('md-title'), mdBody: $('md-body'), mdEdit: $('md-edit'), mdClose: $('md-close'),
    desktopOnly: $('desktop-only'),
  };

  const S = {
    summaries: [], map: null, version: '', selected: null, editing: false,
    view: { x: 0, y: 0, k: 1 },
    undo: [], redo: [],
    save: { state: 'idle', timer: null, deadline: 0, inflight: false, pending: false, blipTimer: null },
    ui: loadUI(),
    inspectorOpen: false,
    mapListKey: '',
    nodeEls: new Map(),
    index: null,
    visible: null,
    drag: null,
  };

  const clone = (v) => JSON.parse(JSON.stringify(v));
  const clampNum = (v, lo, hi) => Math.min(Math.max(v, lo), hi);

  function loadUI() {
    const defaults = { sidebarWidth: PANEL.sidebar.initial, sidebarOpen: true, inspectorWidth: PANEL.inspector.initial };
    try {
      const stored = JSON.parse(localStorage.getItem('inoichi:ui') || '{}');
      return { ...defaults, ...stored };
    } catch {
      return defaults;
    }
  }

  function saveUI() {
    try { localStorage.setItem('inoichi:ui', JSON.stringify(S.ui)); } catch {}
  }

  const hexToRgb = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const rgbToHex = (rgb) => `#${rgb.map((c) => Math.round(clampNum(c, 0, 255)).toString(16).padStart(2, '0')).join('')}`;

  // Mixes an accent over the canvas ground so a node reads as its colour without a border.
  function tint(accentHex, ratio, overHex = PALETTE.base) {
    const a = hexToRgb(accentHex);
    const b = hexToRgb(overHex);
    return rgbToHex(a.map((c, i) => c * ratio + b[i] * (1 - ratio)));
  }

  async function api(method, path, body) {
    let res;
    try {
      res = await fetch(path, {
        method,
        headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (e) {
      throw new Error('The Inoichi server is not reachable. Is it still running?');
    }
    if (res.status === 204) return null;
    const raw = await res.text();
    let parsed = null;
    if (raw) {
      try { parsed = JSON.parse(raw); } catch { parsed = null; }
    }
    if (!res.ok) {
      const err = new Error((parsed && parsed.error) || `The server answered ${res.status}.`);
      err.status = res.status;
      throw err;
    }
    return parsed;
  }

  function icons(root) {
    if (window.lucide) lucide.createIcons(root ? { root } : undefined);
  }

  function announce(msg) {
    el.live.textContent = '';
    requestAnimationFrame(() => { el.live.textContent = msg; });
  }

  function toast(message, kind = 'info') {
    const icon = kind === 'error' ? 'circle-alert' : kind === 'success' ? 'check' : 'info';
    const tone = kind === 'error' ? 'text-red' : kind === 'success' ? 'text-green' : 'text-overlay1';
    const box = document.createElement('div');
    box.className = 'bg-surface0 text-subtext1 rounded-lg px-3 py-2 text-sm flex items-start gap-2 shadow-xl';
    box.setAttribute('role', kind === 'error' ? 'alert' : 'status');
    box.innerHTML = `<i data-lucide="${icon}" class="w-4 h-4 mt-0.5 shrink-0 ${tone}"></i><span class="flex-1"></span>`;
    box.querySelector('span').textContent = message;
    const close = document.createElement('button');
    close.className = 'icon-btn w-6 h-6';
    close.setAttribute('aria-label', 'Dismiss');
    close.innerHTML = '<i data-lucide="x" class="w-3.5 h-3.5"></i>';
    close.addEventListener('click', () => box.remove());
    box.append(close);
    el.toasts.append(box);
    icons(box);
    setTimeout(() => box.remove(), kind === 'error' ? 9000 : 3500);
  }

  function askText({ title, body, label, value = '', confirmText = 'Create', validate }) {
    return new Promise((resolve) => {
      el.dialogTitle.textContent = title;
      el.dialogBody.textContent = body || '';
      el.dialogBody.classList.toggle('hidden', !body);
      el.dialogInputWrap.classList.remove('hidden');
      el.dialogLabel.textContent = label;
      el.dialogInput.value = value;
      el.dialogError.classList.add('hidden');
      el.dialogConfirm.textContent = confirmText;
      el.dialogConfirm.className = 'order-2 btn-primary px-4';

      const guard = (ev) => {
        const problem = validate ? validate(el.dialogInput.value) : null;
        if (!problem) return;
        ev.preventDefault();
        el.dialogError.textContent = problem;
        el.dialogError.classList.remove('hidden');
        el.dialogInput.focus();
      };
      el.dialogConfirm.addEventListener('click', guard);
      el.dialog.addEventListener('close', function done() {
        el.dialog.removeEventListener('close', done);
        el.dialogConfirm.removeEventListener('click', guard);
        resolve(el.dialog.returnValue === 'confirm' ? el.dialogInput.value.trim() : null);
      });
      el.dialog.returnValue = '';
      el.dialog.showModal();
      el.dialogInput.focus();
      el.dialogInput.select();
    });
  }

  function askConfirm({ title, body, confirmText = 'Delete', danger = true }) {
    return new Promise((resolve) => {
      el.dialogTitle.textContent = title;
      el.dialogBody.textContent = body;
      el.dialogBody.classList.remove('hidden');
      el.dialogInputWrap.classList.add('hidden');
      el.dialogConfirm.textContent = confirmText;
      el.dialogConfirm.className = danger
        ? 'order-2 inline-flex items-center justify-center gap-2 text-sm bg-red text-crust font-medium rounded-lg px-4 py-2 hover:opacity-90 focus:outline-none focus-visible:ring-2 focus-visible:ring-red'
        : 'order-2 btn-primary px-4';
      el.dialog.addEventListener('close', function done() {
        el.dialog.removeEventListener('close', done);
        resolve(el.dialog.returnValue === 'confirm');
      });
      el.dialog.returnValue = '';
      el.dialog.showModal();
      el.dialogConfirm.focus();
    });
  }

  const isRoot = (id) => S.map && id === S.map.rootId;

  function index() {
    if (S.index) return S.index;
    const byId = new Map();
    const kids = new Map();
    for (const n of S.map.nodes) {
      byId.set(n.id, n);
      if (n.parentId) {
        if (!kids.has(n.parentId)) kids.set(n.parentId, []);
        kids.get(n.parentId).push(n);
      }
    }
    S.index = { byId, kids };
    return S.index;
  }

  const nodeById = (id) => (S.map && id ? index().byId.get(id) : undefined);
  const childrenOf = (id) => index().kids.get(id) || [];

  function descendants(id) {
    const { kids } = index();
    const out = [];
    const queue = [...(kids.get(id) || [])];
    while (queue.length) {
      const cur = queue.shift();
      out.push(cur.id);
      queue.push(...(kids.get(cur.id) || []));
    }
    return out;
  }

  function visibleIds() {
    if (S.visible) return S.visible;
    const hidden = new Set();
    for (const n of S.map.nodes) {
      if (n.collapsed) for (const d of descendants(n.id)) hidden.add(d);
    }
    S.visible = new Set(S.map.nodes.filter((n) => !hidden.has(n.id)).map((n) => n.id));
    return S.visible;
  }

  function depthOf(id) {
    let d = 1;
    let cur = nodeById(id);
    const seen = new Set();
    while (cur && cur.parentId && !seen.has(cur.id)) {
      seen.add(cur.id);
      cur = nodeById(cur.parentId);
      d += 1;
    }
    return d;
  }

  function accentOf(node) {
    if (node.accent) return node.accent;
    if (isRoot(node.id)) return 'mauve';
    let cur = node;
    const seen = new Set();
    while (cur && cur.parentId && !seen.has(cur.id)) {
      seen.add(cur.id);
      cur = nodeById(cur.parentId);
      if (cur && cur.accent) return cur.accent;
    }
    return 'overlay0';
  }

  const accentHex = (node) => PALETTE[accentOf(node)] || PALETTE.overlay0;
  const fillOf = (node) => tint(accentHex(node), isRoot(node.id) ? TINT.root : TINT.node);
  const textWidth = (node) => node.width - TYPE.padX * 2 - (node.note ? TYPE.noteSlot : 0);

  function measureHeight(node) {
    const style = isRoot(node.id) ? TYPE.root : TYPE.node;
    const lines = wrapText(node.text || 'New idea', textWidth(node), style);
    return clampNum(lines.length * style.line + TYPE.padY * 2, SIZE.minH, SIZE.maxH);
  }

  let measureCtx = null;
  function wrapText(text, maxWidth, style) {
    if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
    measureCtx.font = `${style.weight} ${style.size}px ${TYPE.family}`;
    const width = Math.max(maxWidth, 20);
    const out = [];
    for (const paragraph of String(text).split('\n')) {
      const words = paragraph.split(/\s+/).filter(Boolean);
      if (!words.length) { out.push(''); continue; }
      let line = words[0];
      for (let i = 1; i < words.length; i += 1) {
        const candidate = `${line} ${words[i]}`;
        if (measureCtx.measureText(candidate).width <= width) line = candidate;
        else { out.push(line); line = words[i]; }
      }
      out.push(line);
    }
    return out.length ? out : [''];
  }

  function invalidateIndex() {
    S.index = null;
    S.visible = null;
  }

  function pushUndo(before) {
    invalidateIndex();
    S.undo.push(before);
    if (S.undo.length > 60) S.undo.shift();
    S.redo.length = 0;
    scheduleSave();
    updateHistoryButtons();
  }

  function mutate(fn) {
    if (!S.map) return;
    const before = clone(S.map);
    fn();
    pushUndo(before);
  }

  function updateHistoryButtons() {
    el.undo.disabled = S.undo.length === 0;
    el.redo.disabled = S.redo.length === 0;
  }

  function undo() {
    if (!S.undo.length) return;
    S.redo.push(clone(S.map));
    S.map = S.undo.pop();
    invalidateIndex();
    if (S.selected && !nodeById(S.selected)) S.selected = S.map.rootId;
    renderAll();
    scheduleSave();
    updateHistoryButtons();
    announce('Undone');
  }

  function redo() {
    if (!S.redo.length) return;
    S.undo.push(clone(S.map));
    S.map = S.redo.pop();
    invalidateIndex();
    if (S.selected && !nodeById(S.selected)) S.selected = S.map.rootId;
    renderAll();
    scheduleSave();
    updateHistoryButtons();
    announce('Redone');
  }

  function setSaveState(state, detail = '') {
    S.save.state = state;
    clearTimeout(S.save.blipTimer);
    const look = {
      idle: ['check', 'text-overlay0', 'Saved', true],
      unsaved: ['circle', 'text-overlay0', 'Unsaved changes', false],
      saving: ['loader-circle spin', 'text-overlay1', 'Saving', false],
      saved: ['check blip', 'text-green', 'Saved', false],
      error: ['circle-alert', 'text-red', `Not saved. ${detail} Click to retry.`, false],
    };
    const [icon, tone, label, hidden] = look[state] || look.idle;
    const [name, extra] = icon.split(' ');
    el.saveStatus.className = `icon-btn w-7 h-7 ${tone} ${hidden ? 'invisible' : ''}`;
    el.saveStatus.innerHTML = `<i data-lucide="${name}" class="w-3.5 h-3.5 ${extra || ''}"></i>`;
    el.saveStatus.title = label;
    el.saveStatus.setAttribute('aria-label', label);
    el.saveStatus.disabled = state !== 'error';
    icons(el.saveStatus);
    if (state === 'saved') S.save.blipTimer = setTimeout(() => { if (S.save.state === 'saved') setSaveState('idle'); }, 1500);
    if (state === 'error') announce(`Save failed. ${detail}`);
  }

  // Waits for a pause in editing, but never longer than maxWaitMs from the first unsaved change.
  function scheduleSave() {
    if (!S.map) return;
    const now = Date.now();
    if (S.save.state !== 'unsaved') { S.save.deadline = now + SAVE.maxWaitMs; setSaveState('unsaved'); }
    clearTimeout(S.save.timer);
    S.save.timer = setTimeout(saveNow, Math.max(0, Math.min(SAVE.idleMs, S.save.deadline - now)));
  }

  async function saveNow() {
    if (!S.map) return;
    clearTimeout(S.save.timer);
    if (S.save.inflight) { S.save.pending = true; return; }
    S.save.inflight = true;
    setSaveState('saving');
    const snapshot = { ...clone(S.map), updatedAt: S.version };
    const serial = JSON.stringify(snapshot);
    try {
      const saved = await api('PUT', `/api/maps/${snapshot.id}`, snapshot);
      if (S.map && S.map.id === snapshot.id) {
        S.version = saved.updatedAt;
        S.map.updatedAt = saved.updatedAt;
        const same = JSON.stringify({ ...clone(S.map), updatedAt: snapshot.updatedAt }) === serial;
        setSaveState(same ? 'saved' : 'unsaved');
        updateSummary(saved);
      }
    } catch (e) {
      if (S.map && S.map.id === snapshot.id) {
        setSaveState('error', e.message);
        if (e.status === 409) toast('This map changed elsewhere. Export your copy from the toolbar, then reload the page.', 'error');
      }
    } finally {
      S.save.inflight = false;
      if (S.save.pending) { S.save.pending = false; saveNow(); }
    }
  }

  function updateSummary(m) {
    const i = S.summaries.findIndex((s) => s.id === m.id);
    const summary = { id: m.id, title: m.title, nodeCount: m.nodes.length, sample: m.sample, createdAt: m.createdAt, updatedAt: m.updatedAt };
    if (i < 0) S.summaries.unshift(summary);
    else S.summaries[i] = summary;
    S.summaries.sort((a, b) => (a.updatedAt < b.updatedAt ? 1 : a.updatedAt > b.updatedAt ? -1 : 0));
    renderMapList();
  }

  function showMapListState(which, detail = '') {
    el.mapListLoading.classList.toggle('hidden', which !== 'loading');
    el.mapListEmpty.classList.toggle('hidden', which !== 'empty');
    el.mapListError.classList.toggle('hidden', which !== 'error');
    el.mapList.classList.toggle('hidden', which !== 'list');
    if (which === 'error') el.mapListErrorDetail.textContent = detail;
  }

  async function refreshSummaries({ quiet = false } = {}) {
    if (!quiet) showMapListState('loading');
    try {
      S.summaries = await api('GET', '/api/maps');
      renderMapList();
    } catch (e) {
      if (!quiet) showMapListState('error', e.message);
    }
  }

  function renderMapList() {
    const key = JSON.stringify([S.summaries, S.map && S.map.id]);
    if (key === S.mapListKey) return;
    S.mapListKey = key;
    el.mapList.replaceChildren();
    if (!S.summaries.length) { showMapListState('empty'); return; }
    showMapListState('list');
    for (const s of S.summaries) {
      const li = document.createElement('li');
      const active = S.map && S.map.id === s.id;
      const row = document.createElement('div');
      row.className = `group flex items-center rounded-lg ${active ? 'bg-surface0' : 'hover:bg-surface0/60'}`;

      const open = document.createElement('button');
      open.type = 'button';
      open.className = 'flex-1 min-w-0 text-left px-3 py-2 rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-mauve';
      open.setAttribute('aria-current', active ? 'true' : 'false');
      const name = document.createElement('div');
      name.className = `truncate text-sm ${active ? 'text-text' : 'text-subtext0'}`;
      name.textContent = s.title;
      const meta = document.createElement('div');
      meta.className = 'text-xs text-overlay0 truncate';
      meta.textContent = `${s.nodeCount} node${s.nodeCount === 1 ? '' : 's'} · ${relativeTime(s.updatedAt)}${s.sample ? ' · sample' : ''}`;
      open.append(name, meta);
      open.addEventListener('click', () => openMap(s.id));

      const del = document.createElement('button');
      del.type = 'button';
      del.className = 'icon-btn w-7 h-7 mr-1 text-overlay0 opacity-0 group-hover:opacity-100 focus:opacity-100 hover:text-red';
      del.setAttribute('aria-label', `Delete ${s.title}`);
      del.innerHTML = '<i data-lucide="trash-2" class="w-3.5 h-3.5"></i>';
      del.addEventListener('click', () => deleteMap(s));

      row.append(open, del);
      li.append(row);
      el.mapList.append(li);
    }
    icons(el.mapList);
  }

  function relativeTime(iso) {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return 'unknown';
    const secs = Math.round((Date.now() - then) / 1000);
    if (secs < 60) return 'just now';
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
    if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
    return `${Math.floor(secs / 86400)}d ago`;
  }

  function showCanvasState(which, detail = '') {
    el.canvasLoading.classList.toggle('hidden', which !== 'loading');
    el.canvasEmpty.classList.toggle('hidden', which !== 'empty');
    el.canvasError.classList.toggle('hidden', which !== 'error');
    el.scene.classList.toggle('invisible', which !== 'map');
    if (which === 'error') el.canvasErrorDetail.textContent = detail;
    const disabled = which !== 'map';
    el.title.disabled = disabled;
    for (const b of [el.tidy, el.zoomIn, el.zoomOut, el.zoomReset, el.exportBtn, el.toggleInspector]) b.disabled = disabled;
  }

  let lastOpenAttempt = null;

  async function flushPendingSave() {
    clearTimeout(S.save.timer);
    if (S.map && (S.save.state === 'unsaved' || S.save.state === 'error')) await saveNow();
  }

  async function openMap(id) {
    if (S.map && S.map.id === id) return;
    await flushPendingSave();
    lastOpenAttempt = id;
    showCanvasState('loading');
    try {
      const m = await api('GET', `/api/maps/${id}`);
      adoptMap(m);
    } catch (e) {
      showCanvasState('error', e.message);
    }
  }

  function adoptMap(m) {
    S.map = m;
    S.version = m.updatedAt;
    invalidateIndex();
    S.undo.length = 0;
    S.redo.length = 0;
    S.selected = null;
    setSaveState('idle');
    el.title.value = m.title;
    el.titleError.classList.add('hidden');
    updateHistoryButtons();
    showCanvasState('map');
    renderAll();
    fitToView();
    renderMapList();
    try { localStorage.setItem('inoichi:last', m.id); } catch {}
    announce(`Opened ${m.title}`);
  }

  async function createMap() {
    const title = await askText({
      title: 'New map',
      label: 'Title',
      value: '',
      confirmText: 'Create',
      validate: (v) => {
        const t = v.trim();
        if (!t) return 'A title is required.';
        if (t.length > LIMITS.title) return `Keep it under ${LIMITS.title} characters.`;
        return null;
      },
    });
    if (title === null) return;
    await flushPendingSave();
    try {
      const m = await api('POST', '/api/maps', { title });
      adoptMap(m);
      updateSummary(m);
      select(m.rootId);
      startEditing(m.rootId);
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function deleteMap(summary) {
    const ok = await askConfirm({
      title: `Delete "${summary.title}"?`,
      body: 'The JSON file for this map is removed from disk. This cannot be undone.',
      confirmText: 'Delete map',
    });
    if (!ok) return;
    try {
      if (S.map && S.map.id === summary.id) clearTimeout(S.save.timer);
      await api('DELETE', `/api/maps/${summary.id}`);
      if (S.map && S.map.id === summary.id) {
        S.map = null;
        S.selected = null;
        setSaveState('idle');
        showCanvasState('empty');
        renderInspector();
      }
      S.summaries = S.summaries.filter((s) => s.id !== summary.id);
      renderMapList();
      toast('Map deleted.', 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function importMapFile(file) {
    if (!file) return;
    if (file.size > 8 * 1024 * 1024) { toast('That file is larger than the 8 MB import limit.', 'error'); return; }
    let parsed;
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      toast('That file is not valid JSON.', 'error');
      return;
    }
    if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.nodes)) {
      toast('That JSON is not an Inoichi map: it has no "nodes" array.', 'error');
      return;
    }
    await flushPendingSave();
    try {
      const m = await api('POST', '/api/maps/import', parsed);
      adoptMap(m);
      updateSummary(m);
      toast('Map imported.', 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  function renderAll() {
    if (!S.map) return;
    invalidateIndex();
    el.nodes.replaceChildren();
    S.nodeEls.clear();
    const visible = visibleIds();
    for (const node of S.map.nodes) {
      if (!visible.has(node.id)) continue;
      const box = buildNodeEl(node);
      S.nodeEls.set(node.id, box);
      el.nodes.append(box);
    }
    icons(el.nodes);
    syncHeightsFromDom();
    renderEdges();
    applyView();
    syncTitleInput();
    syncSelectionUI();
  }

  function syncHeightsFromDom() {
    for (const [id, box] of S.nodeEls) {
      const node = nodeById(id);
      const h = box.offsetHeight;
      if (node && h && Math.abs(h - node.height) > 0.5) node.height = clampNum(h, SIZE.minH, SIZE.maxH);
    }
  }

  function buildNodeEl(node) {
    const root = isRoot(node.id);
    const box = document.createElement('div');
    box.dataset.id = node.id;
    box.className = 'node absolute box-border rounded-xl flex items-center gap-2 cursor-grab focus:outline-none';
    box.style.left = `${node.x}px`;
    box.style.top = `${node.y}px`;
    box.style.width = `${node.width}px`;
    box.style.minHeight = `${node.height}px`;
    box.style.padding = `${TYPE.padY}px ${TYPE.padX}px`;
    box.style.setProperty('--accent', accentHex(node));
    box.style.background = fillOf(node);
    box.setAttribute('role', 'treeitem');
    box.setAttribute('tabindex', '-1');
    box.setAttribute('aria-level', String(depthOf(node.id)));
    box.setAttribute('aria-selected', 'false');
    const peers = node.parentId ? childrenOf(node.parentId) : [node];
    box.setAttribute('aria-setsize', String(peers.length));
    box.setAttribute('aria-posinset', String(peers.findIndex((p) => p.id === node.id) + 1));
    const kids = childrenOf(node.id);
    if (kids.length) box.setAttribute('aria-expanded', node.collapsed ? 'false' : 'true');
    box.setAttribute('aria-label', node.note ? `${node.text || 'Empty node'}. Has Markdown.` : (node.text || 'Empty node'));

    const text = document.createElement('div');
    text.className = `node-text flex-1 min-w-0 break-words whitespace-pre-wrap leading-5 ${root ? 'text-[15px] font-semibold text-text' : 'text-sm text-text'}`;
    text.dataset.placeholder = 'New idea';
    text.textContent = node.text;
    box.append(text);

    if (node.note) {
      const mark = document.createElement('button');
      mark.type = 'button';
      mark.dataset.role = 'note';
      mark.tabIndex = -1;
      mark.className = 'node-note shrink-0 w-3.5 h-3.5 opacity-80 hover:opacity-100 focus:outline-none';
      mark.title = 'Open the Markdown (Ctrl+Enter)';
      mark.setAttribute('aria-label', 'Open the Markdown');
      mark.innerHTML = '<i data-lucide="file-text" class="w-3.5 h-3.5"></i>';
      box.append(mark);
    }

    if (kids.length) {
      const toggle = document.createElement('button');
      toggle.type = 'button';
      toggle.dataset.role = 'toggle';
      toggle.tabIndex = -1;
      toggle.className = 'node-toggle absolute top-1/2 -translate-y-1/2 -right-2.5 w-5 h-5 rounded-full bg-mantle text-[10px] font-semibold leading-none grid place-items-center focus:outline-none';
      toggle.textContent = node.collapsed ? String(descendants(node.id).length) : '−';
      toggle.setAttribute('aria-label', node.collapsed ? `Expand ${node.text}` : `Collapse ${node.text}`);
      box.append(toggle);
    }

    const handle = document.createElement('div');
    handle.dataset.role = 'resize';
    handle.className = 'node-handle hidden absolute -bottom-1 -right-1 w-2.5 h-2.5 rounded-sm cursor-nwse-resize';
    handle.title = 'Drag to resize';
    box.append(handle);

    const port = document.createElement('div');
    port.dataset.role = 'port';
    port.className = 'node-port hidden absolute top-1/2 -translate-y-1/2 -left-2 w-2.5 h-2.5 rounded-full cursor-crosshair';
    port.title = 'Drag onto another node to link them';
    box.append(port);

    return box;
  }

  function renderEdges() {
    const visible = visibleIds();
    const parts = [];
    for (const n of S.map.nodes) {
      if (!n.parentId || !visible.has(n.id) || !visible.has(n.parentId)) continue;
      const p = nodeById(n.parentId);
      if (!p) continue;
      parts.push(edgePath(p, n, accentHex(n), false));
    }
    for (const l of S.map.links || []) {
      const a = nodeById(l.from);
      const b = nodeById(l.to);
      if (!a || !b || !visible.has(a.id) || !visible.has(b.id)) continue;
      parts.push(edgePath(a, b, PALETTE.overlay1, true));
    }
    el.edges.innerHTML = parts.join('');
  }

  function edgePath(from, to, color, dashed) {
    const x1 = from.x + from.width;
    const y1 = from.y + from.height / 2;
    const x2 = to.x;
    const y2 = to.y + to.height / 2;
    const dx = Math.max(Math.abs(x2 - x1) * 0.5, 32);
    const d = `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
    const dash = dashed ? ' stroke-dasharray="6 5"' : '';
    return `<path d="${d}" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round"${dash} stroke-opacity="${dashed ? 0.7 : TINT.edge}"/>`;
  }

  function renderNodeGeometry(id) {
    const node = nodeById(id);
    const box = S.nodeEls.get(id);
    if (!node || !box) return;
    box.style.left = `${node.x}px`;
    box.style.top = `${node.y}px`;
    box.style.width = `${node.width}px`;
    box.style.minHeight = `${node.height}px`;
  }

  function syncTitleInput() {
    if (!S.map || document.activeElement === el.title) return;
    el.title.value = S.map.title;
    el.titleError.classList.add('hidden');
  }

  function applyView() {
    el.scene.style.transform = `translate(${S.view.x}px, ${S.view.y}px) scale(${S.view.k})`;
    el.zoomReset.textContent = `${Math.round(S.view.k * 100)}%`;
  }

  function syncSelectionUI() {
    for (const [id, box] of S.nodeEls) {
      const on = id === S.selected;
      box.setAttribute('aria-selected', on ? 'true' : 'false');
      box.setAttribute('tabindex', on ? '0' : '-1');
      box.classList.toggle('selected', on);
      box.classList.remove('drop-target');
      box.querySelector('[data-role="resize"]').classList.toggle('hidden', !on);
      box.querySelector('[data-role="port"]').classList.toggle('hidden', !on);
    }
    renderInspector();
  }

  function select(id, { focus = true } = {}) {
    const changed = id !== S.selected;
    S.selected = id;
    if (changed && id) S.inspectorOpen = true;
    if (changed && !id) S.inspectorOpen = false;
    syncSelectionUI();
    const box = S.nodeEls.get(id);
    if (box && focus) box.focus({ preventScroll: true });
    if (box) ensureVisible(id);
    if (!id && focus) el.canvas.focus({ preventScroll: true });
  }

  function ensureVisible(id) {
    const node = nodeById(id);
    if (!node) return;
    const rect = el.canvas.getBoundingClientRect();
    const left = node.x * S.view.k + S.view.x;
    const top = node.y * S.view.k + S.view.y;
    const right = left + node.width * S.view.k;
    const bottom = top + node.height * S.view.k;
    const pad = 48;
    let moved = false;
    if (left < pad) { S.view.x += pad - left; moved = true; }
    if (right > rect.width - pad) { S.view.x -= right - (rect.width - pad); moved = true; }
    if (top < pad) { S.view.y += pad - top; moved = true; }
    if (bottom > rect.height - pad) { S.view.y -= bottom - (rect.height - pad); moved = true; }
    if (moved) applyView();
  }

  function bbox() {
    if (!S.map || !S.map.nodes.length) return { x: 0, y: 0, w: 1, h: 1 };
    const visible = visibleIds();
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const n of S.map.nodes) {
      if (!visible.has(n.id)) continue;
      minX = Math.min(minX, n.x);
      minY = Math.min(minY, n.y);
      maxX = Math.max(maxX, n.x + n.width);
      maxY = Math.max(maxY, n.y + n.height);
    }
    return { x: minX, y: minY, w: Math.max(maxX - minX, 1), h: Math.max(maxY - minY, 1) };
  }

  function fitToView() {
    const rect = el.canvas.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const b = bbox();
    const pad = 64;
    const k = clampNum(Math.min((rect.width - pad * 2) / b.w, (rect.height - pad * 2) / b.h), ZOOM.min, 1);
    S.view.k = k;
    S.view.x = (rect.width - b.w * k) / 2 - b.x * k;
    S.view.y = (rect.height - b.h * k) / 2 - b.y * k;
    applyView();
  }

  function zoomAt(factor, cx, cy) {
    const rect = el.canvas.getBoundingClientRect();
    const px = cx === undefined ? rect.width / 2 : cx - rect.left;
    const py = cy === undefined ? rect.height / 2 : cy - rect.top;
    const next = clampNum(S.view.k * factor, ZOOM.min, ZOOM.max);
    const ratio = next / S.view.k;
    S.view.x = px - (px - S.view.x) * ratio;
    S.view.y = py - (py - S.view.y) * ratio;
    S.view.k = next;
    applyView();
  }

  function applyPanels() {
    const open = S.ui.sidebarOpen;
    el.sidebar.classList.toggle('hidden', !open);
    el.sidebarResizer.classList.toggle('hidden', !open);
    el.sidebar.style.width = `${S.ui.sidebarWidth}px`;
    el.sidebarOpen.classList.toggle('hidden', open);
    el.inspector.style.width = `${S.ui.inspectorWidth}px`;
  }

  function setSidebarOpen(open) {
    S.ui.sidebarOpen = open;
    saveUI();
    applyPanels();
    if (S.map) applyView();
  }

  function setInspectorOpen(open) {
    S.inspectorOpen = open;
    renderInspector();
  }

  function renderInspector() {
    const node = S.selected && nodeById(S.selected);
    const show = !!node && S.inspectorOpen;
    el.inspector.classList.toggle('hidden', !show);
    el.inspector.classList.toggle('flex', show);
    el.inspectorResizer.classList.toggle('hidden', !show);
    el.toggleInspector.setAttribute('aria-expanded', show ? 'true' : 'false');
    el.toggleInspector.classList.toggle('text-text', show);
    if (!node) return;
    if (document.activeElement !== el.inspText) el.inspText.value = node.text;
    if (document.activeElement !== el.inspNote) el.inspNote.value = node.note || '';
    el.inspNoteOpen.disabled = !node.note;
    el.inspDelete.disabled = isRoot(node.id);
    el.inspDelete.classList.toggle('opacity-40', isRoot(node.id));
    for (const btn of el.inspAccents.querySelectorAll('button')) {
      const on = btn.dataset.accent === (node.accent || '');
      btn.setAttribute('aria-checked', on ? 'true' : 'false');
      btn.tabIndex = on ? 0 : -1;
      btn.classList.toggle('ring-2', on);
      btn.classList.toggle('ring-text', on);
      btn.classList.toggle('ring-offset-2', on);
      btn.classList.toggle('ring-offset-mantle', on);
    }
  }

  function buildAccentPicker() {
    const make = (value, label) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.dataset.accent = value;
      b.setAttribute('role', 'radio');
      b.setAttribute('aria-checked', 'false');
      b.setAttribute('aria-label', label);
      b.title = label;
      b.tabIndex = -1;
      b.className = 'w-6 h-6 rounded-full focus:outline-none focus-visible:ring-2 focus-visible:ring-mauve';
      b.style.background = value ? PALETTE[value] : PALETTE.surface1;
      b.addEventListener('click', () => pickAccent(value));
      return b;
    };
    el.inspAccents.append(make('', 'Inherit from parent'));
    for (const a of ACCENTS) el.inspAccents.append(make(a, a));

    el.inspAccents.addEventListener('keydown', (ev) => {
      const swatches = [...el.inspAccents.querySelectorAll('button')];
      const i = swatches.indexOf(document.activeElement);
      if (i < 0) return;
      const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[ev.key];
      if (!step) return;
      ev.preventDefault();
      const next = swatches[(i + step + swatches.length) % swatches.length];
      next.focus();
      pickAccent(next.dataset.accent);
    });
  }

  function pickAccent(value) {
    const node = nodeById(S.selected);
    if (!node || (node.accent || '') === value) return;
    mutate(() => { node.accent = value; });
    renderAll();
    select(S.selected, { focus: false });
  }

  function addChild(parentId, { edit = true } = {}) {
    const parent = nodeById(parentId);
    if (!parent) return;
    if (S.map.nodes.length >= LIMITS.nodes) { toast(`A map holds at most ${LIMITS.nodes} nodes.`, 'error'); return; }
    const id = newId();
    const siblings = childrenOf(parentId);
    mutate(() => {
      if (parent.collapsed) parent.collapsed = false;
      S.map.nodes.push({
        id,
        parentId,
        text: '',
        x: parent.x + parent.width + 72,
        y: siblings.length ? Math.max(...siblings.map((s) => s.y + s.height)) + 20 : parent.y,
        width: 180,
        height: 44,
      });
    });
    renderAll();
    select(id);
    if (edit) startEditing(id);
    announce('Child node added');
  }

  function addSibling(nodeId) {
    const node = nodeById(nodeId);
    if (!node || isRoot(nodeId)) { addChild(nodeId); return; }
    addChild(node.parentId);
  }

  async function deleteNode(nodeId) {
    const node = nodeById(nodeId);
    if (!node) return;
    if (isRoot(nodeId)) { toast('The root node stays. Delete the whole map from the list instead.', 'error'); return; }
    const kids = descendants(nodeId);
    if (kids.length) {
      const ok = await askConfirm({
        title: 'Delete this branch?',
        body: `"${node.text || 'Empty node'}" and ${kids.length} node${kids.length === 1 ? '' : 's'} under it will be removed. Ctrl+Z undoes it.`,
        confirmText: 'Delete branch',
      });
      if (!ok) return;
    }
    const parentId = node.parentId;
    const doomed = new Set([nodeId, ...kids]);
    mutate(() => {
      S.map.nodes = S.map.nodes.filter((n) => !doomed.has(n.id));
      S.map.links = (S.map.links || []).filter((l) => !doomed.has(l.from) && !doomed.has(l.to));
    });
    S.selected = nodeById(parentId) ? parentId : S.map.rootId;
    renderAll();
    select(S.selected);
    announce(`Removed ${doomed.size} node${doomed.size === 1 ? '' : 's'}`);
  }

  function toggleCollapse(nodeId) {
    const node = nodeById(nodeId);
    if (!node || !childrenOf(nodeId).length) return;
    mutate(() => { node.collapsed = !node.collapsed; });
    renderAll();
    select(nodeId);
    announce(node.collapsed ? 'Branch collapsed' : 'Branch expanded');
  }

  function addLink(fromId, toId) {
    if (fromId === toId) return;
    const exists = (S.map.links || []).some((l) => (l.from === fromId && l.to === toId) || (l.from === toId && l.to === fromId));
    const from = nodeById(fromId);
    const to = nodeById(toId);
    if (!from || !to) return;
    if (to.parentId === fromId || from.parentId === toId) {
      toast('Those nodes are already joined by the tree.', 'error');
      return;
    }
    if (exists) { toast('Those nodes are already linked.', 'error'); return; }
    mutate(() => {
      if (!S.map.links) S.map.links = [];
      S.map.links.push({ id: newId(), from: fromId, to: toId });
    });
    renderEdges();
    announce('Cross link added');
  }

  function newId() {
    const bytes = new Uint8Array(12);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  }

  function startEditing(nodeId) {
    const box = S.nodeEls.get(nodeId);
    const node = nodeById(nodeId);
    if (!box || !node || S.editing) return;
    const text = box.querySelector('.node-text');
    const original = node.text;
    S.editing = true;
    text.setAttribute('contenteditable', 'true');
    text.setAttribute('role', 'textbox');
    text.setAttribute('aria-label', 'Node text');
    text.focus();
    const range = document.createRange();
    range.selectNodeContents(text);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);

    const finish = (commit) => {
      text.removeEventListener('keydown', onKey);
      text.removeEventListener('blur', onBlur);
      text.removeEventListener('paste', onPaste);
      text.removeAttribute('contenteditable');
      text.removeAttribute('role');
      S.editing = false;
      const next = commit ? text.innerText.replace(/\s+$/, '').slice(0, LIMITS.text) : original;
      text.textContent = next;
      if (commit && next !== original) {
        mutate(() => {
          node.text = next;
          node.height = measureHeight({ ...node, text: next });
          if (isRoot(nodeId)) {
            S.map.title = next.slice(0, LIMITS.title) || S.map.title;
            el.title.value = S.map.title;
          }
        });
        renderAll();
      }
      if (S.selected === nodeId) select(nodeId);
      else syncSelectionUI();
    };
    const onKey = (ev) => {
      ev.stopPropagation();
      if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); finish(true); }
      else if (ev.key === 'Escape') { ev.preventDefault(); finish(false); }
      else if (ev.key === 'Tab') { ev.preventDefault(); finish(true); addChild(nodeId); }
    };
    const onBlur = () => finish(true);
    const onPaste = (ev) => {
      ev.preventDefault();
      const plain = (ev.clipboardData || window.clipboardData).getData('text/plain');
      document.execCommand('insertText', false, plain.slice(0, LIMITS.text));
    };
    text.addEventListener('keydown', onKey);
    text.addEventListener('blur', onBlur);
    text.addEventListener('paste', onPaste);
  }

  function renderMarkdown(source) {
    const html = marked.parse(source || '', { gfm: true, breaks: false });
    return DOMPurify.sanitize(html, { USE_PROFILES: { html: true }, ADD_ATTR: ['target'] });
  }

  function openMarkdown(nodeId) {
    const node = nodeById(nodeId);
    if (!node) return;
    if (!node.note) {
      setInspectorOpen(true);
      el.inspNote.focus();
      return;
    }
    el.mdTitle.textContent = node.text || 'Empty node';
    el.mdBody.innerHTML = renderMarkdown(node.note);
    for (const a of el.mdBody.querySelectorAll('a[href]')) { a.target = '_blank'; a.rel = 'noopener noreferrer'; }
    el.md.dataset.node = nodeId;
    if (!el.md.open) el.md.showModal();
    el.mdBody.scrollTop = 0;
  }

  function toMap(clientX, clientY) {
    const rect = el.canvas.getBoundingClientRect();
    return {
      x: (clientX - rect.left - S.view.x) / S.view.k,
      y: (clientY - rect.top - S.view.y) / S.view.k,
    };
  }

  function nodeAt(clientX, clientY, exclude) {
    const p = toMap(clientX, clientY);
    const visible = visibleIds();
    for (let i = S.map.nodes.length - 1; i >= 0; i -= 1) {
      const n = S.map.nodes[i];
      if (!visible.has(n.id) || n.id === exclude) continue;
      if (p.x >= n.x && p.x <= n.x + n.width && p.y >= n.y && p.y <= n.y + n.height) return n;
    }
    return null;
  }

  function abandonDrag() {
    const d = S.drag;
    S.drag = null;
    el.canvas.style.cursor = '';
    if (!d) return;
    if (d.before) {
      S.map = d.before;
      renderAll();
      select(S.selected, { focus: false });
    } else if (d.kind === 'link') {
      renderEdges();
    }
  }

  el.canvas.addEventListener('pointerdown', (ev) => {
    if (!S.map || ev.button !== 0) return;
    closeExportMenu();
    const box = ev.target.closest('.node');

    if (!box) {
      if (S.editing) return;
      select(null);
      S.drag = { kind: 'pan', startX: ev.clientX, startY: ev.clientY, vx: S.view.x, vy: S.view.y };
      el.canvas.setPointerCapture(ev.pointerId);
      el.canvas.style.cursor = 'grabbing';
      return;
    }

    const id = box.dataset.id;
    const roleEl = ev.target.closest('[data-role]');
    const role = roleEl ? roleEl.dataset.role : '';
    if (role === 'toggle') { ev.preventDefault(); toggleCollapse(id); return; }
    if (role === 'note') { ev.preventDefault(); select(id, { focus: false }); openMarkdown(id); return; }

    select(id);
    if (S.editing && !role) return;
    const node = nodeById(id);
    el.canvas.setPointerCapture(ev.pointerId);

    if (role === 'resize') {
      S.drag = { kind: 'resize', id, before: clone(S.map), moved: false, startX: ev.clientX, startY: ev.clientY, w: node.width, h: node.height };
    } else if (role === 'port') {
      S.drag = { kind: 'link', id, from: node };
    } else {
      S.drag = {
        kind: 'node', id, before: clone(S.map), moved: false,
        startX: ev.clientX, startY: ev.clientY, nx: node.x, ny: node.y,
        children: descendants(id).map((d) => ({ id: d, x: nodeById(d).x, y: nodeById(d).y })),
      };
      box.style.cursor = 'grabbing';
    }
    ev.preventDefault();
  });

  el.canvas.addEventListener('pointermove', (ev) => {
    const d = S.drag;
    if (!d) return;
    if (d.kind === 'pan') {
      S.view.x = d.vx + (ev.clientX - d.startX);
      S.view.y = d.vy + (ev.clientY - d.startY);
      applyView();
      return;
    }
    if (d.kind === 'node') {
      const dx = (ev.clientX - d.startX) / S.view.k;
      const dy = (ev.clientY - d.startY) / S.view.k;
      if (Math.abs(dx) > 1 || Math.abs(dy) > 1) d.moved = true;
      const node = nodeById(d.id);
      node.x = Math.round(d.nx + dx);
      node.y = Math.round(d.ny + dy);
      if (!ev.shiftKey) {
        for (const c of d.children) {
          const cn = nodeById(c.id);
          cn.x = Math.round(c.x + dx);
          cn.y = Math.round(c.y + dy);
          renderNodeGeometry(c.id);
        }
      }
      renderNodeGeometry(d.id);
      renderEdges();
      highlightDropTarget(ev.shiftKey ? nodeAt(ev.clientX, ev.clientY, d.id) : null, d.id);
      return;
    }
    if (d.kind === 'resize') {
      const node = nodeById(d.id);
      d.moved = true;
      node.width = Math.round(clampNum(d.w + (ev.clientX - d.startX) / S.view.k, SIZE.minW, SIZE.maxW));
      node.height = Math.round(clampNum(d.h + (ev.clientY - d.startY) / S.view.k, SIZE.minH, SIZE.maxH));
      renderNodeGeometry(d.id);
      renderEdges();
      return;
    }
    if (d.kind === 'link') {
      const p = toMap(ev.clientX, ev.clientY);
      const from = d.from;
      const x1 = from.x;
      const y1 = from.y + from.height / 2;
      renderEdges();
      el.edges.innerHTML += `<path d="M ${x1} ${y1} L ${p.x} ${p.y}" fill="none" stroke="${PALETTE.overlay1}" stroke-width="2" stroke-dasharray="4 4"/>`;
      highlightDropTarget(nodeAt(ev.clientX, ev.clientY, d.id), d.id);
    }
  });

  function highlightDropTarget(target, excludeId) {
    for (const [id, box] of S.nodeEls) {
      box.classList.toggle('drop-target', !!target && target.id === id && id !== excludeId);
    }
  }

  function commitDrag(before, id) {
    pushUndo(before);
    renderAll();
    select(id, { focus: false });
  }

  el.canvas.addEventListener('pointerup', (ev) => {
    if (el.canvas.hasPointerCapture(ev.pointerId)) el.canvas.releasePointerCapture(ev.pointerId);
    const d = S.drag;
    S.drag = null;
    el.canvas.style.cursor = '';
    if (!d) return;

    if (d.kind === 'node') {
      const box = S.nodeEls.get(d.id);
      if (box) box.style.cursor = '';
      if (!d.moved) { syncSelectionUI(); return; }
      const target = ev.shiftKey ? nodeAt(ev.clientX, ev.clientY, d.id) : null;
      if (target) {
        const node = nodeById(d.id);
        if (isRoot(d.id)) toast('The root node cannot be reparented.', 'error');
        else if (descendants(d.id).includes(target.id)) toast('A node cannot become a child of its own branch.', 'error');
        else if (node.parentId !== target.id) { node.parentId = target.id; announce('Node reparented'); }
      }
      commitDrag(d.before, d.id);
      return;
    }

    if (d.kind === 'resize') {
      if (!d.moved) { syncSelectionUI(); return; }
      const node = nodeById(d.id);
      node.height = clampNum(Math.max(node.height, measureHeight(node)), SIZE.minH, SIZE.maxH);
      commitDrag(d.before, d.id);
      return;
    }

    if (d.kind === 'link') {
      const target = nodeAt(ev.clientX, ev.clientY, d.id);
      if (target) addLink(d.id, target.id);
      else renderEdges();
      syncSelectionUI();
    }
  });

  el.canvas.addEventListener('pointercancel', () => abandonDrag());

  el.canvas.addEventListener('dblclick', (ev) => {
    const box = ev.target.closest('.node');
    if (box && !ev.target.closest('[data-role]')) { ev.preventDefault(); startEditing(box.dataset.id); }
  });

  el.canvas.addEventListener('wheel', (ev) => {
    if (!S.map) return;
    ev.preventDefault();
    if (ev.ctrlKey || ev.metaKey) {
      zoomAt(Math.exp(-ev.deltaY * 0.0015), ev.clientX, ev.clientY);
      return;
    }
    S.view.x -= ev.deltaX;
    S.view.y -= ev.deltaY;
    applyView();
  }, { passive: false });

  function bindResizer(handle, key, bounds, fromRight) {
    handle.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      ev.preventDefault();
      const startX = ev.clientX;
      const startW = S.ui[key];
      handle.classList.add('active');
      handle.setPointerCapture(ev.pointerId);
      const move = (e) => {
        const delta = fromRight ? startX - e.clientX : e.clientX - startX;
        S.ui[key] = Math.round(clampNum(startW + delta, bounds.min, bounds.max));
        applyPanels();
      };
      const up = () => {
        handle.classList.remove('active');
        handle.removeEventListener('pointermove', move);
        handle.removeEventListener('pointerup', up);
        handle.removeEventListener('pointercancel', up);
        saveUI();
      };
      handle.addEventListener('pointermove', move);
      handle.addEventListener('pointerup', up);
      handle.addEventListener('pointercancel', up);
    });
  }

  function inTextField(target) {
    if (!target) return false;
    const tag = target.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || target.isContentEditable;
  }

  document.addEventListener('keydown', (ev) => {
    const mod = ev.ctrlKey || ev.metaKey;
    const anyDialog = el.help.open || el.dialog.open || el.md.open;

    if (mod && ev.key.toLowerCase() === 's') { ev.preventDefault(); saveNow(); return; }
    if (mod && ev.key.toLowerCase() === 'z') {
      if (inTextField(ev.target)) return;
      ev.preventDefault();
      ev.shiftKey ? redo() : undo();
      return;
    }
    if (mod && ev.key.toLowerCase() === 'y') {
      if (inTextField(ev.target)) return;
      ev.preventDefault();
      redo();
      return;
    }
    if (ev.key === '?' && !inTextField(ev.target) && !anyDialog) {
      ev.preventDefault();
      el.help.showModal();
      return;
    }
    if (mod && ev.key === 'Enter' && S.map && S.selected && !anyDialog) {
      ev.preventDefault();
      openMarkdown(S.selected);
      return;
    }
    if (inTextField(ev.target) || S.editing || !S.map || anyDialog) return;

    if (ev.key === '[') { ev.preventDefault(); setSidebarOpen(!S.ui.sidebarOpen); return; }
    if (ev.key === ']') { ev.preventDefault(); toggleInspector(); return; }
    if (mod && (ev.key === '=' || ev.key === '+')) { ev.preventDefault(); zoomAt(1.2); return; }
    if (mod && ev.key === '-') { ev.preventDefault(); zoomAt(1 / 1.2); return; }
    if (mod && ev.key === '0') { ev.preventDefault(); fitToView(); return; }
    if (mod && ev.shiftKey && ev.key.toLowerCase() === 'l') { ev.preventDefault(); tidyLayout(); return; }
    if (mod) return;

    let id = S.selected;
    if (!id || !nodeById(id)) {
      if (ev.key === 'Tab' || ev.key === 'Enter' || ev.key.startsWith('Arrow')) { ev.preventDefault(); select(S.map.rootId); }
      return;
    }

    switch (ev.key) {
      case 'Tab':
        if (ev.shiftKey) break;
        ev.preventDefault();
        addChild(id);
        break;
      case 'Enter': ev.preventDefault(); addSibling(id); break;
      case ' ': ev.preventDefault(); startEditing(id); break;
      case 'F2': ev.preventDefault(); startEditing(id); break;
      case 'Delete': case 'Backspace': ev.preventDefault(); deleteNode(id); break;
      case 'ArrowLeft': ev.preventDefault(); moveSelection('parent'); break;
      case 'ArrowRight': ev.preventDefault(); moveSelection('child'); break;
      case 'ArrowUp': ev.preventDefault(); moveSelection('prev'); break;
      case 'ArrowDown': ev.preventDefault(); moveSelection('next'); break;
      case '/': ev.preventDefault(); toggleCollapse(id); break;
      case 'Escape': ev.preventDefault(); select(null); break;
      default: break;
    }
  });

  function toggleInspector() {
    if (!S.map) return;
    if (!S.selected) { select(S.map.rootId, { focus: false }); return; }
    setInspectorOpen(!S.inspectorOpen);
  }

  function moveSelection(dir) {
    const node = nodeById(S.selected);
    if (!node) return;
    const visible = visibleIds();
    if (dir === 'parent' && node.parentId && visible.has(node.parentId)) { select(node.parentId); return; }
    if (dir === 'child') {
      if (node.collapsed) { toggleCollapse(node.id); return; }
      const kids = childrenOf(node.id).filter((k) => visible.has(k.id)).sort((a, b) => a.y - b.y);
      if (kids.length) select(kids[0].id);
      return;
    }
    const siblings = (node.parentId ? childrenOf(node.parentId) : [node])
      .filter((s) => visible.has(s.id))
      .sort((a, b) => a.y - b.y);
    const i = siblings.findIndex((s) => s.id === node.id);
    const next = dir === 'prev' ? siblings[i - 1] : siblings[i + 1];
    if (next) select(next.id);
  }

  function download(filename, blob) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
  }

  const slug = (s) => (s || 'map').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 48) || 'map';

  const xmlEscape = (s) => String(s)
    .replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;').replaceAll("'", '&apos;');

  function buildSvg() {
    const b = bbox();
    const pad = 40;
    const w = Math.ceil(b.w + pad * 2);
    const h = Math.ceil(b.h + pad * 2);
    const ox = pad - b.x;
    const oy = pad - b.y;
    const visible = visibleIds();
    const parts = [];

    parts.push(`<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`);
    parts.push(`<rect width="${w}" height="${h}" fill="${PALETTE.crust}"/>`);
    parts.push(`<g transform="translate(${ox} ${oy})">`);

    for (const n of S.map.nodes) {
      if (!n.parentId || !visible.has(n.id) || !visible.has(n.parentId)) continue;
      const p = nodeById(n.parentId);
      if (p) parts.push(edgePath(p, n, accentHex(n), false));
    }
    for (const l of S.map.links || []) {
      const a = nodeById(l.from);
      const c = nodeById(l.to);
      if (a && c && visible.has(a.id) && visible.has(c.id)) parts.push(edgePath(a, c, PALETTE.overlay1, true));
    }

    for (const n of S.map.nodes) {
      if (!visible.has(n.id)) continue;
      const style = isRoot(n.id) ? TYPE.root : TYPE.node;
      const accent = accentHex(n);
      const lines = wrapText(n.text || '', textWidth(n) * 0.92, style);
      const boxH = Math.max(n.height, lines.length * style.line + TYPE.padY * 2);
      parts.push(`<rect x="${n.x}" y="${n.y}" width="${n.width}" height="${boxH}" rx="12" fill="${fillOf(n)}"/>`);
      const firstBaseline = n.y + boxH / 2 - ((lines.length - 1) * style.line) / 2 + style.size * 0.35;
      parts.push(`<text x="${n.x + TYPE.padX}" y="${firstBaseline}" fill="${PALETTE.text}" font-family="${TYPE.family}" font-size="${style.size}" font-weight="${style.weight}">`);
      lines.forEach((line, i) => {
        parts.push(`<tspan x="${n.x + TYPE.padX}" dy="${i === 0 ? 0 : style.line}">${xmlEscape(line)}</tspan>`);
      });
      parts.push('</text>');

      if (n.note) {
        const ix = n.x + n.width - TYPE.padX - 12;
        const iy = n.y + boxH / 2 - 6;
        parts.push(`<rect x="${ix}" y="${iy}" width="10" height="12" rx="1.5" fill="none" stroke="${accent}" stroke-width="1.5"/>`);
        parts.push(`<line x1="${ix + 2.5}" y1="${iy + 4.5}" x2="${ix + 7.5}" y2="${iy + 4.5}" stroke="${accent}" stroke-width="1.2"/>`);
        parts.push(`<line x1="${ix + 2.5}" y1="${iy + 7.5}" x2="${ix + 7.5}" y2="${iy + 7.5}" stroke="${accent}" stroke-width="1.2"/>`);
      }

      if (n.collapsed) {
        const hidden = descendants(n.id).length;
        if (hidden) {
          const cx = n.x + n.width;
          const cy = n.y + boxH / 2;
          parts.push(`<circle cx="${cx}" cy="${cy}" r="10" fill="${PALETTE.mantle}"/>`);
          parts.push(`<text x="${cx}" y="${cy + 3.5}" fill="${accent}" text-anchor="middle" font-family="${TYPE.family}" font-size="10" font-weight="600">${hidden}</text>`);
        }
      }
    }

    parts.push('</g></svg>');
    return parts.join('');
  }

  function exportJson() {
    download(`${slug(S.map.title)}.json`, new Blob([JSON.stringify(S.map, null, 2)], { type: 'application/json' }));
    toast('JSON exported.', 'success');
  }

  function exportSvg() {
    download(`${slug(S.map.title)}.svg`, new Blob([buildSvg()], { type: 'image/svg+xml;charset=utf-8' }));
    toast('SVG exported.', 'success');
  }

  function exportPng() {
    const svg = buildSvg();
    const b = bbox();
    const scale = 2;
    const w = Math.ceil((b.w + 80) * scale);
    const h = Math.ceil((b.h + 80) * scale);
    const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }));
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement('canvas');
      canvas.width = w;
      canvas.height = h;
      const ctx = canvas.getContext('2d');
      ctx.fillStyle = PALETTE.crust;
      ctx.fillRect(0, 0, w, h);
      ctx.drawImage(img, 0, 0, w, h);
      URL.revokeObjectURL(url);
      canvas.toBlob((blob) => {
        if (!blob) { toast('This browser could not rasterise the PNG. The SVG export always works.', 'error'); return; }
        download(`${slug(S.map.title)}.png`, blob);
        toast('PNG exported.', 'success');
      }, 'image/png');
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      toast('This browser could not rasterise the PNG. The SVG export always works.', 'error');
    };
    img.src = url;
  }

  function closeExportMenu() {
    el.exportMenu.classList.add('hidden');
    el.exportBtn.setAttribute('aria-expanded', 'false');
  }

  async function tidyLayout() {
    if (!S.map) return;
    const before = clone(S.map);
    try {
      const laid = await api('POST', '/api/layout', { ...S.map, updatedAt: S.version });
      S.map.nodes = laid.nodes;
      invalidateIndex();
      pushUndo(before);
      renderAll();
      fitToView();
      select(S.selected, { focus: false });
      announce('Layout tidied');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  el.newMap.addEventListener('click', createMap);
  el.emptyNewMap.addEventListener('click', createMap);
  el.mapListRetry.addEventListener('click', () => refreshSummaries());
  el.canvasRetry.addEventListener('click', () => lastOpenAttempt && openMap(lastOpenAttempt));
  el.importMap.addEventListener('click', () => el.importFile.click());
  el.importFile.addEventListener('change', async (ev) => {
    await importMapFile(ev.target.files[0]);
    ev.target.value = '';
  });
  el.saveStatus.addEventListener('click', () => { if (S.save.state === 'error') saveNow(); });
  el.undo.addEventListener('click', undo);
  el.redo.addEventListener('click', redo);
  el.tidy.addEventListener('click', tidyLayout);
  el.zoomIn.addEventListener('click', () => zoomAt(1.2));
  el.zoomOut.addEventListener('click', () => zoomAt(1 / 1.2));
  el.zoomReset.addEventListener('click', fitToView);

  el.exportBtn.addEventListener('click', () => {
    const hidden = el.exportMenu.classList.toggle('hidden');
    el.exportBtn.setAttribute('aria-expanded', hidden ? 'false' : 'true');
    if (!hidden) el.exportMenu.querySelector('.export-item').focus();
  });
  el.exportMenu.addEventListener('keydown', (ev) => {
    const items = [...el.exportMenu.querySelectorAll('.export-item')];
    const i = items.indexOf(document.activeElement);
    if (ev.key === 'Escape') { ev.preventDefault(); closeExportMenu(); el.exportBtn.focus(); return; }
    if (ev.key === 'ArrowDown') { ev.preventDefault(); items[(i + 1) % items.length].focus(); }
    if (ev.key === 'ArrowUp') { ev.preventDefault(); items[(i - 1 + items.length) % items.length].focus(); }
  });
  for (const item of el.exportMenu.querySelectorAll('.export-item')) {
    item.addEventListener('click', () => {
      closeExportMenu();
      if (!S.map) return;
      const f = item.dataset.format;
      if (f === 'json') exportJson();
      else if (f === 'svg') exportSvg();
      else exportPng();
    });
  }
  document.addEventListener('click', (ev) => {
    if (!el.exportMenu.contains(ev.target) && ev.target !== el.exportBtn && !el.exportBtn.contains(ev.target)) closeExportMenu();
  });

  el.title.addEventListener('input', () => {
    const v = el.title.value.trim();
    if (!v) {
      el.titleError.textContent = 'A title is required.';
      el.titleError.classList.remove('hidden');
      return;
    }
    el.titleError.classList.add('hidden');
    if (!S.map || S.map.title === v) return;
    S.map.title = v;
    scheduleSave();
  });
  el.title.addEventListener('blur', () => {
    if (!S.map) return;
    if (!el.title.value.trim()) {
      el.title.value = S.map.title;
      el.titleError.classList.add('hidden');
    }
  });

  // Panel fields update the node live and push one undo step when the field commits.
  let fieldBefore = null;
  const captureBefore = () => { fieldBefore = S.map ? clone(S.map) : null; };
  const commitField = () => {
    if (!S.map || !fieldBefore) return;
    if (JSON.stringify(fieldBefore) !== JSON.stringify(S.map)) pushUndo(fieldBefore);
    fieldBefore = null;
    renderAll();
    select(S.selected, { focus: false });
  };

  el.inspText.addEventListener('focus', captureBefore);
  el.inspText.addEventListener('input', () => {
    const node = nodeById(S.selected);
    if (!node) return;
    node.text = el.inspText.value.slice(0, LIMITS.text);
    node.height = measureHeight(node);
    if (isRoot(node.id) && node.text.trim()) { S.map.title = node.text.trim(); el.title.value = S.map.title; }
    const box = S.nodeEls.get(node.id);
    if (box) box.querySelector('.node-text').textContent = node.text;
    renderNodeGeometry(node.id);
    renderEdges();
    scheduleSave();
  });
  el.inspText.addEventListener('change', commitField);

  el.inspNote.addEventListener('focus', captureBefore);
  el.inspNote.addEventListener('input', () => {
    const node = nodeById(S.selected);
    if (!node) return;
    node.note = el.inspNote.value.slice(0, LIMITS.note);
    el.inspNoteOpen.disabled = !node.note;
    scheduleSave();
  });
  el.inspNote.addEventListener('change', commitField);
  el.inspNote.addEventListener('keydown', (ev) => {
    if (ev.key === 'Tab' && !ev.shiftKey) {
      ev.preventDefault();
      const { selectionStart: a, selectionEnd: b, value } = el.inspNote;
      el.inspNote.value = `${value.slice(0, a)}  ${value.slice(b)}`;
      el.inspNote.selectionStart = el.inspNote.selectionEnd = a + 2;
      el.inspNote.dispatchEvent(new Event('input'));
    }
  });
  el.inspNoteOpen.addEventListener('click', () => S.selected && openMarkdown(S.selected));
  el.inspAddChild.addEventListener('click', () => S.selected && addChild(S.selected));
  el.inspDelete.addEventListener('click', () => S.selected && deleteNode(S.selected));
  el.inspClose.addEventListener('click', () => setInspectorOpen(false));
  el.toggleInspector.addEventListener('click', toggleInspector);

  el.sidebarOpen.addEventListener('click', () => setSidebarOpen(true));
  el.sidebarClose.addEventListener('click', () => setSidebarOpen(false));
  el.showHelp.addEventListener('click', () => { if (!el.help.open) el.help.showModal(); });
  el.helpClose.addEventListener('click', () => el.help.close());
  el.mdClose.addEventListener('click', () => el.md.close());
  el.mdEdit.addEventListener('click', () => {
    const id = el.md.dataset.node;
    el.md.close();
    if (!nodeById(id)) return;
    select(id, { focus: false });
    setInspectorOpen(true);
    el.inspNote.focus();
  });

  bindResizer(el.sidebarResizer, 'sidebarWidth', PANEL.sidebar, false);
  bindResizer(el.inspectorResizer, 'inspectorWidth', PANEL.inspector, true);

  window.addEventListener('beforeunload', (ev) => {
    if (S.save.state === 'unsaved' || S.save.state === 'error') {
      ev.preventDefault();
      ev.returnValue = '';
    }
  });

  const SHORTCUTS = [
    ['Tab', 'Add a child to the selected node'],
    ['Enter', 'Add a sibling'],
    ['Space / double click', 'Rename a node'],
    ['Delete', 'Delete the node and its branch'],
    ['Arrow keys', 'Walk parent, child and siblings'],
    ['/', 'Collapse or expand a branch'],
    ['Ctrl/Cmd + Enter', 'Open the rendered Markdown'],
    ['Esc', 'Deselect'],
    ['Shift + drag', 'Drop a node on another to reparent it'],
    ['Drag the left dot', 'Draw a cross link between two nodes'],
    ['Ctrl/Cmd + Z', 'Undo'],
    ['Ctrl/Cmd + Shift + Z', 'Redo'],
    ['Ctrl/Cmd + S', 'Save now'],
    ['Ctrl/Cmd + scroll', 'Zoom at the pointer'],
    ['Ctrl/Cmd + 0', 'Fit the map to the screen'],
    ['Ctrl/Cmd + Shift + L', 'Tidy the layout'],
    ['[', 'Show or hide the map list'],
    [']', 'Show or hide the node panel'],
    ['?', 'Open this panel'],
  ];

  function buildHelp() {
    for (const [keys, what] of SHORTCUTS) {
      const dt = document.createElement('dt');
      dt.className = 'font-mono text-xs text-text whitespace-nowrap pt-0.5';
      dt.textContent = keys;
      const dd = document.createElement('dd');
      dd.className = 'text-subtext0';
      dd.textContent = what;
      el.helpKeys.append(dt, dd);
    }
  }

  const narrow = window.matchMedia('(orientation: portrait), (max-width: 767px)');
  function checkViewport() {
    el.desktopOnly.classList.toggle('hidden', !narrow.matches);
    el.desktopOnly.classList.toggle('grid', narrow.matches);
  }

  async function boot() {
    buildHelp();
    buildAccentPicker();
    applyPanels();
    showCanvasState('empty');
    setSaveState('idle');
    updateHistoryButtons();
    checkViewport();
    icons();

    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.register('/sw.js').catch(() => {});
    }

    await refreshSummaries();
    let last = null;
    try { last = localStorage.getItem('inoichi:last'); } catch {}
    const target = S.summaries.find((s) => s.id === last) || S.summaries[0];
    if (target) openMap(target.id);
  }

  narrow.addEventListener('change', checkViewport);
  window.addEventListener('resize', () => { if (S.map) applyView(); });

  boot();
})();
