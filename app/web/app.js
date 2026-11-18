/* NEXUS console frontend. Talks to the local NEXUS service over /api. */
'use strict';

// ------------------------------------------------------------------ utils
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'style' && typeof v === 'object') { for (const [sk, sv] of Object.entries(v)) { if (sk.startsWith('--')) el.style.setProperty(sk, sv); else el.style[sk] = sv; } }
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (k === 'html') el.innerHTML = v;
    else if (k === 'dataset') Object.assign(el.dataset, v);
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat(Infinity)) if (kid !== null && kid !== undefined && kid !== false) el.append(kid.nodeType ? kid : document.createTextNode(kid));
  return el;
}
const svg = (path, cls = '', evenodd = false) => {
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.setAttribute('viewBox', '0 0 24 24'); if (cls) s.setAttribute('class', cls);
  s.innerHTML = `<path d="${path}"${evenodd ? ' fill-rule="evenodd"' : ''}/>`;
  return s;
};
async function api(path, opts = {}) {
  const o = { method: opts.method || (opts.body !== undefined ? 'POST' : 'GET'), headers: { 'X-Nexus': '1' } };
  if (opts.body !== undefined) {
    if (opts.body instanceof Blob) { o.body = opts.body; o.headers['Content-Type'] = opts.body.type || 'application/octet-stream'; }
    else { o.body = JSON.stringify(opts.body); o.headers['Content-Type'] = 'application/json'; }
  }
  const r = await fetch(path, o);
  let data = null;
  try { data = await r.json(); } catch { }
  if (!r.ok) { const e = new Error((data && data.error) || r.statusText); e.status = r.status; e.data = data; throw e; }
  return data;
}
const sleep = ms => new Promise(r => setTimeout(r, ms));
const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
function initials(t) { const w = (t || '?').split(/[\s\-_:]+/).map(x => x.replace(/[^\p{L}\p{N}]/gu, '')).filter(Boolean); return (w.length > 1 ? w[0][0] + w[1][0] : (w[0] || '?').slice(0, 2)).toUpperCase(); }
function hue(str) { let x = 0; for (const c of str || '') x = (x * 31 + c.charCodeAt(0)) >>> 0; return x % 360; }
function fmtBytes(b) { if (!b) return '0 B'; const u = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0; while (b >= 1024 && i < 4) { b /= 1024; i++; } return `${b.toFixed(b < 10 && i ? 1 : 0)} ${u[i]}`; }
function fmtPlay(s) {
  if (!s) return '';
  if (s < 60) return 'Played for less than a minute';
  const m = Math.round(s / 60); if (m < 60) return `Played for ${m} minute${m === 1 ? '' : 's'}`;
  const hrs = Math.floor(s / 3600); const rem = Math.round((s % 3600) / 60);
  return `Played for ${hrs} hour${hrs === 1 ? '' : 's'}${rem && hrs < 10 ? ` ${rem} min` : ''}`;
}
function fmtAgo(ts) {
  if (!ts) return ''; const d = Date.now() / 1000 - ts;
  if (d < 90) return 'just now'; if (d < 3600) return `${Math.round(d / 60)} min ago`;
  if (d < 86400) return `${Math.round(d / 3600)} hours ago`; if (d < 86400 * 2) return 'yesterday';
  if (d < 86400 * 30) return `${Math.round(d / 86400)} days ago`;
  return new Date(ts * 1000).toLocaleDateString();
}
function luminance(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex || ''); if (!m) return 0;
  const n = parseInt(m[1], 16); const c = [n >> 16, (n >> 8) & 255, n & 255].map(v => { v /= 255; return v <= .03928 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4; });
  return .2126 * c[0] + .7152 * c[1] + .0722 * c[2];
}
function contrast(a, b) { const x = luminance(a), y = luminance(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05); }
// text on an accent-coloured surface: whichever of near-black / white reads better
function inkFor(hex) { return contrast(hex, '#111111') >= contrast(hex, '#ffffff') ? '#111' : '#fff'; }
function mixHex(a, b, t) {
  const p = x => parseInt(x.slice(1), 16), A = p(a), B = p(b);
  const ch = s => Math.round(((A >> s) & 255) * (1 - t) + ((B >> s) & 255) * t);
  return '#' + ((ch(16) << 16) | (ch(8) << 8) | ch(0)).toString(16).padStart(6, '0');
}
// the accent used as text (labels, values, selected items): the profile colour, moved towards
// the theme's text colour just enough to stay readable on the theme's surfaces (4.5:1)
function accentTextFor(hex, light) {
  const surf = light ? '#ffffff' : '#2e3035', text = light ? '#1c1d20' : '#f4f5f7';
  if (!/^#[0-9a-f]{6}$/i.test(hex || '')) return text;
  for (let t = 0; t <= 1.0001; t += .05) { const c = mixHex(hex, text, t); if (contrast(c, surf) >= 4.5) return c; }
  return text;
}

// ------------------------------------------------------------------ icons
const I = {
  back: 'M15.4 4.6 17 6.2 11.2 12l5.8 5.8-1.6 1.6L8 12Z',
  more: 'M6 10a2 2 0 1 1 0 4 2 2 0 0 1 0-4Zm6 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4Zm6 0a2 2 0 1 1 0 4 2 2 0 0 1 0-4Z',
  puzzle: 'M20.5 11H19V7a2 2 0 0 0-2-2h-4V3.5a2.5 2.5 0 0 0-5 0V5H4a2 2 0 0 0-2 2v3.8h1.5a2.7 2.7 0 0 1 0 5.4H2V20a2 2 0 0 0 2 2h3.8v-1.5a2.7 2.7 0 0 1 5.4 0V22H17a2 2 0 0 0 2-2v-4h1.5a2.5 2.5 0 0 0 0-5Z',
  download: 'M11 4h2v8.2l3.3-3.3 1.4 1.4L12 16l-5.7-5.7 1.4-1.4 3.3 3.3ZM5 18h14v2H5Z',
  cloud: 'M6.5 19a4.5 4.5 0 0 1-.6-8.96A6.5 6.5 0 0 1 18.3 8.6 5.2 5.2 0 0 1 17.5 19Zm0-2h11a3.2 3.2 0 0 0 .3-6.39l-1.06-.1-.3-1.02A4.5 4.5 0 0 0 7.9 11.2l-.2.95-.97.1A2.5 2.5 0 0 0 6.5 17Z',
  media: 'M4 5h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Zm0 2v10h16V7Zm3 8 3-4 2.2 2.7L15 10l4 5Zm1-6.5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3Z',
  grid: 'M3 3h8v8H3zM13 3h8v8h-8zM3 13h8v8H3zM13 13h8v8h-8z',
  person: 'M12 12a5 5 0 1 0 0-10 5 5 0 0 0 0 10Zm0 2c-5 0-9 2.5-9 5.5V22h18v-2.5C21 16.5 17 14 12 14Z',
  pad: 'M7 6h10a5 5 0 0 1 4.9 6l-1 5.2a3 3 0 0 1-5.4 1.2L14 16h-4l-1.5 2.4a3 3 0 0 1-5.4-1.2L2.1 12A5 5 0 0 1 7 6Zm0 2.5V10H5.5v2H7v1.5h2V12h1.5v-2H9V8.5Zm9 0a1.2 1.2 0 1 0 0 2.4 1.2 1.2 0 0 0 0-2.4Zm-2 2.6a1.2 1.2 0 1 0 0 2.4 1.2 1.2 0 0 0 0-2.4Zm4 0a1.2 1.2 0 1 0 0 2.4 1.2 1.2 0 0 0 0-2.4Z',
  palette: 'M12 2a10 10 0 0 0 0 20c1.1 0 2-.9 2-2 0-.5-.2-1-.5-1.3-.3-.4-.5-.8-.5-1.3 0-1.1.9-2 2-2h2.4A4.6 4.6 0 0 0 22 10.8C22 5.9 17.5 2 12 2Zm-5.5 10a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3Zm3-4a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3Zm5 0a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3Zm3 4a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3Z',
  chip: 'M9 2h2v2h2V2h2v2h2a2 2 0 0 1 2 2v2h2v2h-2v2h2v2h-2v2a2 2 0 0 1-2 2h-2v2h-2v-2h-2v2H9v-2H7a2 2 0 0 1-2-2v-2H3v-2h2v-2H3V8h2V6a2 2 0 0 1 2-2h2Zm-1 6v8h8V8Zm2 2h4v4h-4Z',
  gear: 'M19.4 13a7.5 7.5 0 0 0 0-2l2.1-1.6-2-3.5-2.5 1a7.4 7.4 0 0 0-1.7-1L14.9 3h-4l-.4 2.9a7.4 7.4 0 0 0-1.7 1l-2.5-1-2 3.5L6.4 11a7.5 7.5 0 0 0 0 2l-2.1 1.6 2 3.5 2.5-1c.5.4 1.1.7 1.7 1l.4 2.9h4l.4-2.9c.6-.3 1.2-.6 1.7-1l2.5 1 2-3.5ZM12.9 15.5a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7Z',
  power: 'M11 2h2v10h-2zM6.3 5.3l1.4 1.4A7 7 0 1 0 16.3 6.7l1.4-1.4A9 9 0 1 1 6.3 5.3Z',
  wifi: 'M12 18a2 2 0 1 1 0 4 2 2 0 0 1 0-4Zm0-5.5c2 0 3.9.8 5.3 2.2l-1.6 1.6A5.2 5.2 0 0 0 12 14.7c-1.5 0-2.8.6-3.7 1.6l-1.6-1.6A7.4 7.4 0 0 1 12 12.5Zm0-5c3.4 0 6.5 1.4 8.8 3.6l-1.6 1.6A10.2 10.2 0 0 0 12 9.7c-2.8 0-5.4 1.1-7.2 3L3.2 11.1A12.4 12.4 0 0 1 12 7.5Zm0-5c4.8 0 9.1 1.9 12.3 5L22.7 9A15.2 15.2 0 0 0 12 4.7 15.2 15.2 0 0 0 1.3 9L-.3 7.5C2.9 4.4 7.2 2.5 12 2.5Z',
  nowifi: 'M3.3 2 22 20.7 20.7 22l-6.9-6.9a5.2 5.2 0 0 0-5.5 1.2l-1.6-1.6a7.4 7.4 0 0 1 4.6-2.2L8.6 9.8a10.3 10.3 0 0 0-3.8 2.9l-1.6-1.6a12.4 12.4 0 0 1 3.6-2.8L4.9 6.4a15 15 0 0 0-3.6 2.6L-.3 7.5a17 17 0 0 1 3.5-2.6L2 3.3ZM12 18a2 2 0 1 1 0 4 2 2 0 0 1 0-4Z',
  plus: 'M11 4h2v7h7v2h-7v7h-2v-7H4v-2h7z',
  check: 'M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z',
  info: 'M11 7h2v2h-2zm0 4h2v6h-2zm1-9a10 10 0 1 0 0 20 10 10 0 0 0 0-20Z',
  warn: 'M1 21h22L12 2 1 21Zm12-3h-2v-2h2v2Zm0-4h-2v-4h2v4Z',
  play: 'M8 5v14l11-7z',
  refresh: 'M17.7 6.3A8 8 0 1 0 19.7 14h-2.1a6 6 0 1 1-1.4-6.2L13 11h7V4z',
  trash: 'M9 3h6l1 2h4v2H4V5h4zM6 9h12l-1 12H7z',
  folder: 'M10 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-8l-2-2Z',
  vol: 'M3 9v6h4l5 5V4L7 9H3Zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4Z',
  volHigh: 'M3 9v6h4l5 5V4L7 9H3Zm13.5 3A4.5 4.5 0 0 0 14 8v8a4.5 4.5 0 0 0 2.5-4ZM14 3.2v2.1a7 7 0 0 1 0 13.4v2.1a9 9 0 0 0 0-17.6Z',
  volMute: 'M3 9v6h4l5 5V4L7 9H3Zm18.6 1.4-1.4-1.4L18 11.2 15.8 9l-1.4 1.4 2.2 2.2-2.2 2.2 1.4 1.4 2.2-2.2 2.2 2.2 1.4-1.4-2.2-2.2Z',
};
const GLYPHS = {
  'grip-l': ['M7.5 3h6A3.5 3.5 0 0 1 17 6.6L16 18a3.4 3.4 0 0 1-3.4 3H8.4A3.4 3.4 0 0 1 5 18L4 6.6A3.5 3.5 0 0 1 7.5 3Zm1.5 4v2H7v2h2v2h2v-2h2V9h-2V7Zm1 9a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z', true],
  'grip-r': ['M10.5 3h6A3.5 3.5 0 0 1 20 6.6L19 18a3.4 3.4 0 0 1-3.4 3h-4.2A3.4 3.4 0 0 1 8 18L7 6.6A3.5 3.5 0 0 1 10.5 3Zm3 3.2a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2Zm0 4.4a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2Zm-2.2-2.2a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2Zm4.4 0a1.1 1.1 0 1 0 0 2.2 1.1 1.1 0 0 0 0-2.2ZM13.5 16a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z', true],
  star: ['M12 2.5l2.9 6.1 6.6.8-4.9 4.6 1.3 6.6L12 17.3l-5.9 3.3 1.3-6.6L2.5 9.4l6.6-.8Z'],
  bolt: ['M13 2 4 14h6l-1 8 9-12h-6Z'],
  leaf: ['M20 4c-9 0-15 4-15 11 0 1.6.5 3 1.3 4.1L4 21.4 5.4 22.8l2.3-2.3c1.1.8 2.5 1.3 4.1 1.3 7 0 8.2-8 8.2-17.8Z'],
  flame: ['M12 2s6 5 6 11a6 6 0 0 1-12 0c0-2.5 1.2-4.4 2.5-5.8.3 1.7 1.3 2.8 2.5 3.3C10.5 7.5 12 2 12 2Z'],
  planet: ['M12 6a6 6 0 1 1 0 12 6 6 0 0 1 0-12ZM2 15.5c1.5 2 9-1 13.5-4.5S22 4.5 20.5 3.5c-.8-.5-2.6 0-4.6 1.1l1 1.2c1.2-.6 2-.8 2.2-.6.6.6-2.5 3.6-6.4 6.2-4 2.7-8.2 4.4-8.8 3.8-.2-.2.2-1 1-2l-1.1-1.1C2.7 13.6 1.6 15 2 15.5Z'],
  heart: ['M12 21s-8-5.3-8-11a4.5 4.5 0 0 1 8-2.8A4.5 4.5 0 0 1 20 10c0 5.7-8 11-8 11Z'],
  crown: ['M3 7l4.5 4L12 4l4.5 7L21 7l-2 12H5Z'],
  ghost: ['M12 2a7 7 0 0 0-7 7v13l2.3-2 2.4 2 2.3-2 2.3 2 2.4-2L19 22V9a7 7 0 0 0-7-7Zm-2.5 6a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3Zm5 0a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3Z', true],
  note: ['M9 3v11.3A3.5 3.5 0 1 0 11 17.5V8h8V3Z'],
  rocket: ['M12 2c3 2 5 6 5 10l2 3v4l-3-2H8l-3 2v-4l2-3c0-4 2-8 5-10Zm0 6a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM10 19h4l-2 3Z', true],
};

// ------------------------------------------------------------------ state
const S = {
  app: {}, settings: {}, profiles: [], emulator: {}, engine: {}, formats: [],
  games: [], stats: {}, recent: [], profile: null, screen: null, system: {}, libSort: null, libExtras: false,
  libQuery: '', lastInput: 'keyboard', pads: [], playing: null,
};
const profileById = id => S.profiles.find(p => p.id === id);
const gameByKey = k => S.games.find(g => g.key === k);
const isPlayable = g => g.kind !== 'update' && g.kind !== 'dlc';

// ------------------------------------------------------------------ sound
const Sound = {
  ctx: null, buf: {}, gain: null,
  init() {
    try {
      this.ctx = new (window.AudioContext || window.webkitAudioContext)();
      this.gain = this.ctx.createGain(); this.gain.connect(this.ctx.destination);
    } catch { this.ctx = null; }
  },
  failed: [],
  async load(names) {
    if (!this.ctx) { this.report(names.length, 0, 'no audio context'); return; }
    let ok = 0; const errs = [];
    await Promise.all(names.map(async n => {
      try {
        const r = await fetch(`/res/sounds/${n}.wav`);
        if (!r.ok) throw new Error('HTTP ' + r.status);
        this.buf[n] = await this.ctx.decodeAudioData(await r.arrayBuffer()); ok++;
      } catch (e) { errs.push(`${n}: ${e?.message || e}`); }
    }));
    this.report(names.length, ok, errs.slice(0, 3).join('; '));
  },
  // the result goes to NEXUS's log, so a silent interface can be diagnosed (and CI checks it)
  report(total, ok, why) {
    const st = this.ctx ? this.ctx.state + ', ' + this.ctx.sampleRate + ' Hz' : 'none';
    api('/api/app/log', { body: { level: ok === total ? 'info' : 'error', message: `ui: sounds loaded ${ok}/${total} (audio ${st})${why ? ' - ' + why : ''}` } }).catch(() => { });
  },
  live: new Set(),
  play(n, vol = 1, { at = 0, pan = 0, rate = 1, force = false } = {}) {
    const s = S.settings.sound || {};
    if (!this.ctx || (!s.enabled && !force) || !this.buf[n]) return null;
    if (this.ctx.state === 'suspended') this.ctx.resume();
    const src = this.ctx.createBufferSource(); src.buffer = this.buf[n]; src.playbackRate.value = rate;
    const g = this.ctx.createGain(); g.gain.value = (s.volume ?? .7) * vol;
    let node = src.connect(g);
    if (pan && this.ctx.createStereoPanner) { const p = this.ctx.createStereoPanner(); p.pan.value = pan; node = g.connect(p); node.connect(this.gain); }
    else g.connect(this.gain);
    src.start(this.ctx.currentTime + Math.max(0, at));
    const rec = { src, g }; this.live.add(rec); src.onended = () => this.live.delete(rec);
    return rec;
  },
  stopAll(fade = .15) {
    if (!this.ctx) return;
    const t = this.ctx.currentTime;
    for (const r of this.live) { try { r.g.gain.setTargetAtTime(0, t, fade / 3); r.src.stop(t + fade); } catch { } }
    this.live.clear();
  },
};

// ------------------------------------------------------------------ navigation & input
// a field the user types in (the keyboard shortcuts below leave it alone)
function isTextField(el) {
  return !!(el && el.matches && (el.matches('textarea') || el.matches('input:not([type]), input[type=text], input[type=search], input[type=email], input[type=password], input[type=url], input[type=number], input[type=tel]')));
}
const Nav = {
  layers: [], // {root, onBack, onOptions, name, restore}
  cur: null,
  top() { return this.layers[this.layers.length - 1]; },
  push(layer) {
    if (this.cur) layer.restore = this.cur;
    this.layers.push(layer);
    this.setFocus(layer.initial || this.items(layer.root)[0], { silent: true });
  },
  pop(layer) {
    const i = this.layers.indexOf(layer); if (i < 0) return;
    this.layers.splice(i, 1);
    const t = this.top();
    if (t) this.setFocus(layer.restore && t.root.contains(layer.restore) ? layer.restore : this.items(t.root)[0], { silent: true });
  },
  base(layer) { // replace the screen layer (bottom of the stack)
    this.layers = [layer, ...this.layers.filter(l => l.dialog)];
    if (!this.layers.some(l => l.dialog)) this.setFocus(layer.initial || this.items(layer.root)[0], { silent: true });
  },
  items(root) {
    return $$('[data-nav]', root).filter(el => !el.disabled && el.offsetParent !== null && !el.closest('[hidden]'));
  },
  setFocus(el, { silent = false, scroll = true } = {}) {
    if (!el) return;
    if (this.cur && this.cur !== el) { this.cur.classList.remove('focus'); this.cur.dispatchEvent(new CustomEvent('navblur')); }
    const hintsBefore = !!resetTarget(this.cur);
    this.cur = el;
    el.classList.add('focus');
    if (hintsBefore || resetTarget(el) || S.screen === 'home') renderHints();
    if (isTextField(el) && S.lastInput !== 'mouse') el.focus({ preventScroll: true });
    else if (document.activeElement && document.activeElement !== el && document.activeElement.matches('input') && !OSK.target) document.activeElement.blur();
    if (scroll) el.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: S.anim === 'off' ? 'auto' : 'smooth' });
    el.dispatchEvent(new CustomEvent('navfocus'));
    if (!silent) Sound.play('move', .6);
  },
  move(dir) {
    const layer = this.top(); if (!layer) return;
    const items = this.items(layer.root);
    if (!this.cur || !items.includes(this.cur)) { this.setFocus(items[0]); return; }
    if (this.cur.dataset.navHandler && window[this.cur.dataset.navHandler]?.(this.cur, dir)) return;
    const custom = this.cur._navMove && this.cur._navMove(dir);
    if (custom === true) return;
    const a = this.cur.getBoundingClientRect();
    const ac = { x: a.left + a.width / 2, y: a.top + a.height / 2 };
    const sameRow = this.cur.closest('[data-row]');
    let best = null, bestScore = Infinity;
    for (const el of items) {
      if (el === this.cur) continue;
      if ((dir === 'left' || dir === 'right') && sameRow && !sameRow.contains(el) && sameRow.dataset.row === 'strict') continue;
      const b = el.getBoundingClientRect();
      const bc = { x: b.left + b.width / 2, y: b.top + b.height / 2 };
      // items that mostly overlap vertically are on the same row, even if the focused one is lifted/scaled
      if ((dir === 'up' || dir === 'down') && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 0.5 * Math.min(a.height, b.height)) continue;
      let primary, ortho;
      if (dir === 'left') { if (b.right > a.left + 2 && bc.x >= ac.x) continue; primary = ac.x - bc.x; ortho = Math.abs(bc.y - ac.y); if (b.bottom <= a.top || b.top >= a.bottom) ortho += 400; }
      if (dir === 'right') { if (b.left < a.right - 2 && bc.x <= ac.x) continue; primary = bc.x - ac.x; ortho = Math.abs(bc.y - ac.y); if (b.bottom <= a.top || b.top >= a.bottom) ortho += 400; }
      if (dir === 'up') { if (b.bottom > a.top + 2 && bc.y >= ac.y) continue; primary = ac.y - bc.y; ortho = Math.abs(bc.x - ac.x); }
      if (dir === 'down') { if (b.top < a.bottom - 2 && bc.y <= ac.y) continue; primary = bc.y - ac.y; ortho = Math.abs(bc.x - ac.x); }
      if (primary <= 0) continue;
      const score = primary + ortho * 2.2;
      if (score < bestScore) { bestScore = score; best = el; }
    }
    if (best) this.setFocus(best);
  },
  activate() {
    const el = this.cur; if (!el || !this.top()?.root.contains(el)) return;
    if (isTextField(el)) {
      el.focus();
      // a text field chosen with a controller opens the on-screen keyboard (web/osk.js); on a
      // Steam Deck (no physical keyboard: its controls may arrive as keys) it opens however the
      // field was chosen
      if (S.lastInput === 'gamepad' || oskAutoDevice()) openKeyboardFor(el);
      return;
    }
    Sound.play('select');
    el.click();
  },
  back() {
    const l = this.top(); if (!l) return;
    if (isTextField(document.activeElement) && S.lastInput === 'keyboard' && l.root.contains(document.activeElement)) { document.activeElement.blur(); return; }
    if (l.onBack) { Sound.play('back'); l.onBack(); }
  },
  options() {
    // on a setting that differs from its default, Options restores the default (web/settings.js)
    const r = resetTarget(this.cur);
    if (r) { Sound.play('select'); r.run(); return; }
    const l = this.top(); if (l?.onOptions) { Sound.play('select'); l.onOptions(this.cur); }
  },
  tab(d) { const l = this.top(); if (l?.onTab) { Sound.play('move'); l.onTab(d); } },
};

function setInputMode(mode) {
  if (S.lastInput === mode) return;
  S.lastInput = mode;
  document.body.classList.toggle('no-cursor', mode === 'gamepad');
  // the focus follows the mouse pointer: with the mouse it is shown as a hover, never as a
  // selection (app.css, "selection states")
  document.body.classList.toggle('input-mouse', mode === 'mouse');
  renderHints();
}

document.addEventListener('keydown', e => {
  if (document.body.classList.contains('booting')) { Boot.skip(); e.preventDefault(); return; }
  setInputMode('keyboard');
  const t = e.target;
  const typing = isTextField(t);
  const k = e.key;
  if ((e.ctrlKey && ['+', '-', '=', '0', 'p', 's', 'f', 'r', 'u'].includes(k.toLowerCase())) || k === 'F5' || k === 'F12' || k === 'F3') { e.preventDefault(); }
  if (Mapper.listening) { if (k === 'Escape') Mapper.cancel(); e.preventDefault(); return; }
  if (typing && (k === 'ArrowLeft' || k === 'ArrowRight' || (k.length === 1 && k !== ' ') || k === 'Backspace' || k === 'Delete' || k === 'Home' || k === 'End')) return;
  if (typing && k === ' ') return;
  const range = Nav.cur && Nav.cur.querySelector && (Nav.cur.matches('input[type=range]') ? Nav.cur : null);
  switch (k) {
    case 'ArrowUp': Nav.move('up'); break;
    case 'ArrowDown': Nav.move('down'); break;
    case 'ArrowLeft': if (!adjust(-1)) Nav.move('left'); break;
    case 'ArrowRight': if (!adjust(1)) Nav.move('right'); break;
    case 'Enter': if (typing) { t.dispatchEvent(new CustomEvent('submit')); t.blur(); } else Nav.activate(); break;
    case ' ': Nav.activate(); break;
    case 'Escape': case 'Backspace': Nav.back(); break;
    case 'x': case 'X': case 'ContextMenu': Nav.options(); break;
    case 'q': case 'Q': case 'PageUp': Nav.tab(-1); break;
    case 'e': case 'E': case 'PageDown': Nav.tab(1); break;
    case 'Tab': Nav.move(e.shiftKey ? 'left' : 'right'); break;
    case '/': { const s = $('#lib-search'); if (S.screen === 'library' && s) { Nav.setFocus(s); s.focus(); } else return; break; }
    case 'Home': case 'h': case 'H': if (!typing) goHome({ button: true }); break;
    default: return;
  }
  void range;
  e.preventDefault();
});
document.addEventListener('wheel', e => { if (e.ctrlKey) e.preventDefault(); }, { passive: false });
document.addEventListener('contextmenu', e => e.preventDefault());
document.addEventListener('mousemove', () => { setInputMode('mouse'); }, { passive: true });
document.addEventListener('mouseover', e => {
  if (S.lastInput !== 'mouse') return;
  const el = e.target.closest('[data-nav]');
  if (el && Nav.top()?.root.contains(el) && el !== Nav.cur) Nav.setFocus(el, { silent: true, scroll: false });
});
document.addEventListener('click', e => { if (document.body.classList.contains('booting')) Boot.skip(); });
// a text field tapped or clicked on a Steam Deck (touch screen, trackpad) opens the keyboard
document.addEventListener('click', e => {
  const t = e.target;
  if (isTextField(t) && oskAutoDevice() && !OSK.target && Nav.top()?.root.contains(t)) openKeyboardFor(t);
});

// ------------------------------------------------------------------ typing without a keyboard
// Settings › General › On-screen keyboard (settings.go Input.Keyboard)
function oskMode() { return S.settings.input?.keyboard || 'auto'; }
// the keyboard opens whenever a text field is chosen, not only with a controller
function oskAutoDevice() {
  const m = oskMode();
  return m === 'always' || m === 'steam' || (m === 'auto' && !!(S.platform?.steamDeck || S.platform?.gamingMode));
}
// openKeyboardFor(field): Steam's keyboard when chosen and Steam runs (Linux); otherwise, or if
// Steam does not answer, NEXUS's own keyboard (web/osk.js), which always types into the field
async function openKeyboardFor(el) {
  if (!el || !isTextField(el)) return;
  el.focus({ preventScroll: true });
  if (oskMode() === 'steam' && S.platform?.steam) {
    try { await api('/api/app/steam-keyboard', { body: {} }); return; } catch (e) {
      api('/api/app/log', { body: { level: 'warn', message: 'ui: Steam keyboard unavailable, NEXUS keyboard instead: ' + (e?.message || e) } }).catch(() => { });
    }
  }
  OSK.open(el);
}
document.addEventListener('dragstart', e => e.preventDefault());

// left/right adjust for sliders and option pickers
function adjust(d) {
  const el = Nav.cur; if (!el) return false;
  if (el._adjust) { el._adjust(d); return true; }
  return false;
}

// Which controller each browser gamepad is (web/pads.js), and the active one for prompts.
const PadInfo = {
  update() { S.padInfo = identifyGamepads(S.pads || [], S.enginePads || [], S.padModels || {}); },
  // the engine's controller list names the model; asked only when controllers change
  async refreshEngine() {
    try { const d = await api(`/api/controllers?profile=${encodeURIComponent(S.profile?.id || '')}`); S.enginePads = d.pads || []; }
    catch { return; }
    const before = JSON.stringify(S.padInfo || []);
    this.update();
    if (JSON.stringify(S.padInfo) !== before) { renderHints(); renderTopbar(); Controllers.refreshList(); }
  },
  of(index) { return (S.padInfo || []).find(i => i.index === index); },
  // the controller whose buttons the prompts show: the one used last, else the chosen one, else the first
  active() {
    const pref = S.profile?.controls?.preferredPad;
    return this.of(S.activePad) || (pref && (S.padInfo || []).find(i => i.key === pref)) || (S.padInfo || [])[0] || null;
  },
  matches(gp, prefKey) { const i = this.of(gp.index); return i?.key === prefKey || gp.id === prefKey; },
  label(index) { const a = this.active(); return padLabelFor(a?.family || 'xbox', index, a?.model); },
  glyph(index, cls = 'key') { const a = this.active(); const el = h('span', { class: cls + ' pad-key', title: padLabelFor(a?.family || 'xbox', index, a?.model) }); el.innerHTML = padGlyphSVG(a?.family || 'xbox', index, a?.model); return el; },
};

// Gamepad polling (Gamepad API, standard mapping). Works with XInput, DualShock/DualSense and most HID pads.
const Pad = {
  prev: {}, held: {}, repeatAt: {}, connected: [], rejoin: new Set(),
  mapping() { return (S.profile?.controls?.mapping) || { confirm: 0, back: 1, options: 3, search: 2, prevTab: 4, nextTab: 5, menu: 9, home: 16, view: 8 }; },
  poll() {
    const pads = navigator.getGamepads ? [...navigator.getGamepads()].filter(Boolean) : [];
    const ids = pads.map(p => p.index + ':' + p.id).join('|');
    if (ids !== this.connected.join('|')) {
      const before = this.connected.length, beforeKeys = new Set((S.padInfo || []).map(i => i.index + ':' + i.name));
      const gone = (S.padInfo || []).filter(i => !pads.some(p => p.index === i.index));
      this.connected = pads.map(p => p.index + ':' + p.id);
      S.pads = pads.map(p => ({ index: p.index, id: p.id, mapping: p.mapping }));
      PadInfo.update();
      // a controller that left: its held buttons are forgotten, so it cannot trigger anything
      // when it comes back (no duplicate or stuck input)
      for (const g of gone) { delete this.prev[g.index]; this.rejoin.add(g.index); for (const k of Object.keys(this.repeatAt)) if (k.startsWith(g.index + '')) delete this.repeatAt[k]; if (S.activePad === g.index) S.activePad = null; }
      renderTopbar(); renderHints(); Controllers.refreshList();
      const booting = document.body.classList.contains('booting');
      if (pads.length > before && !booting) { Sound.play('connect'); const n = (S.padInfo || []).find(i => !beforeKeys.has(i.index + ':' + i.name)); toast(`${n?.name || 'Controller'} connected`, I.pad); }
      if (pads.length < before && !booting) toast(`${gone[0]?.name || 'Controller'} disconnected`, I.pad);
      PadInfo.refreshEngine(); // the engine's detection names the model (event-driven, not polled)
    }
    const prefKey = S.profile?.controls?.preferredPad;
    const active = prefKey ? pads.filter(p => PadInfo.matches(p, prefKey)) : pads;
    const use = active.length ? active : pads; // the chosen controller is not connected: any controller works
    const now = performance.now();
    const map = this.mapping();
    const repeatMs = S.profile?.controls?.keyRepeatMs || 140;
    for (const p of use) {
      const b = i => !!(p.buttons[i] && (p.buttons[i].pressed || p.buttons[i].value > .6));
      const ax = p.axes[0] || 0, ay = p.axes[1] || 0;
      const dirs = {
        up: b(12) || ay < -.6, down: b(13) || ay > .6, left: b(14) || ax < -.6, right: b(15) || ax > .6,
      };
      const acts = {};
      for (const [a, idx] of Object.entries(map)) acts[a] = b(idx);
      const key = p.index;
      const state = { ...dirs, ...acts };
      // a controller that comes back: buttons already held count only after they are released
      if (this.rejoin.has(key)) { this.rejoin.delete(key); this.prev[key] = state; continue; }
      const prev = this.prev[key] || {};
      if (Mapper.listening) {
        for (let i = 0; i < p.buttons.length; i++) if (b(i) && !(prev['b' + i])) { Mapper.capture(i); }
        const raw = {}; for (let i = 0; i < p.buttons.length; i++) raw['b' + i] = b(i);
        this.prev[key] = { ...state, ...raw }; Controllers.liveUpdate(p); continue;
      }
      const any = Object.values(state).some(Boolean);
      if (any) { setInputMode('gamepad'); this.activeUntil = now + 1500; if (S.activePad !== p.index) { S.activePad = p.index; renderHints(); if (S.screen === 'controllers') Controllers.refreshList(); } }
      if (document.body.classList.contains('booting')) { if (any && !Object.values(prev).some(Boolean)) Boot.skip(); this.prev[key] = state; continue; }
      for (const d of ['up', 'down', 'left', 'right']) {
        if (state[d] && !prev[d]) { this.repeatAt[key + d] = now + 380; this.dir(d); }
        else if (state[d] && now >= (this.repeatAt[key + d] || Infinity)) { this.repeatAt[key + d] = now + repeatMs; this.dir(d); }
      }
      const edge = a => state[a] && !prev[a];
      if (edge('confirm')) Nav.activate();
      if (edge('back')) Nav.back();
      if (edge('options')) Nav.options();
      if (OSK.target && OSK.pad(state, prev)) { this.prev[key] = state; continue; } // the on-screen keyboard's own buttons
      if (edge('search')) {
        const box = S.screen === 'library' ? $('#lib-search') : S.screen === 'settings' && !Nav.top()?.dialog ? $('#set-search') : null;
        const menu = homeContextMenu(Nav.cur); // X on a Home tile: its menu, as a right-click
        if (box) { Nav.setFocus(box); box.focus(); OSK.open(box); } else if (menu) { Sound.play('select'); menu(); } else if (S.screen === 'home') showLibrary(true);
      }
      if (edge('prevTab')) Nav.tab(-1);
      if (edge('nextTab')) Nav.tab(1);
      if (edge('menu')) { if (!Nav.top()?.dialog) openPower(); }
      if (edge('home')) goHome({ button: true });
      this.prev[key] = state;
      Controllers.liveUpdate(p);
    }
    this.schedule(pads.length > 0);
  },
  // While a controller is in use (input in the last 1.5 s): every frame, for smooth repeat and
  // navigation. Connected but idle: 30 times a second on a timer, which keeps the first press
  // within ~33 ms but does not make the page produce a frame on every refresh (measured with a
  // connected controller on Home: 60 style recalculations a second before, see PERFORMANCE.md).
  // No controller: four times a second (connecting one is also announced by the browser).
  activeUntil: 0,
  schedule(connected) {
    clearTimeout(this.t); cancelAnimationFrame(this.r);
    if (connected && performance.now() < this.activeUntil) this.r = requestAnimationFrame(() => this.poll());
    else this.t = setTimeout(() => this.poll(), connected ? 33 : 250);
  },
  dir(d) { if ((d === 'left' || d === 'right') && adjust(d === 'left' ? -1 : 1)) return; Nav.move(d); },
  rumble(ms = 120, strong = .4) {
    if (!S.profile?.controls?.vibration) return;
    for (const p of navigator.getGamepads ? navigator.getGamepads() : []) {
      p?.vibrationActuator?.playEffect?.('dual-rumble', { duration: ms, strongMagnitude: strong, weakMagnitude: .6 }).catch?.(() => { });
    }
  },
};
window.addEventListener('gamepaddisconnected', () => Pad.schedule(true));

// ------------------------------------------------------------------ layers (dialogs, sheets, toasts)
function openLayer(content, { onBack, onOptions, onTab, sheet = false, name = '', initial } = {}) {
  const wrap = h('div', { class: 'layer' + (sheet ? ' has-sheet' : '') }, content);
  $('#layers').append(wrap);
  const layer = { root: wrap, dialog: true, name, onBack: onBack || (() => close()), onOptions, onTab };
  const close = () => {
    if (wrap._closed) return; wrap._closed = true;
    wrap.classList.add('closing'); Nav.pop(layer); setTimeout(() => wrap.remove(), 150); renderHints();
  };
  layer.close = close;
  wrap.addEventListener('mousedown', e => { if (e.target === wrap) layer.onBack(); });
  layer.initial = initial ? (typeof initial === 'string' ? $(initial, wrap) : initial) : null;
  Nav.push(layer);
  renderHints();
  return layer;
}

function dialog({ title, text, pre, buttons = [{ label: 'OK', value: true, primary: true }], input, cancelValue = null }) {
  return new Promise(resolve => {
    let inp = null;
    if (input) {
      inp = h('input', { type: 'text', value: input.value || '', placeholder: input.placeholder || '', maxlength: input.max || 80, 'data-nav': true, spellcheck: 'false' });
    }
    const btns = buttons.map(b => h('button', { 'data-nav': true, class: (b.danger ? 'danger' : '') + (b.primary ? ' primary' : '') }, b.label));
    const box = h('div', { class: 'dialog', role: 'dialog' },
      h('div', { class: 'd-body' }, title && h('h2', {}, title), text && h('p', {}, text), pre && h('pre', {}, pre), inp),
      h('div', { class: 'd-actions', 'data-row': 'strict' }, btns));
    let done = false;
    const finish = v => { if (done) return; done = true; layer.close(); resolve(v); };
    btns.forEach((el, i) => el.addEventListener('click', () => finish(input ? (buttons[i].value === null ? null : (buttons[i].value === true ? inp.value : buttons[i].value)) : buttons[i].value)));
    if (inp) inp.addEventListener('submit', () => { const p = buttons.find(b => b.primary) || buttons[0]; finish(p.value === true ? inp.value : p.value); });
    const primary = buttons.findIndex(b => b.primary);
    const layer = openLayer(box, { onBack: () => finish(cancelValue), initial: inp || btns[primary >= 0 ? primary : 0] });
    if (inp && S.lastInput !== 'gamepad') setTimeout(() => inp.focus(), 50);
  });
}
const confirmDlg = (title, text, ok = 'OK', danger = false) => dialog({ title, text, buttons: [{ label: 'Cancel', value: false }, { label: ok, value: true, primary: !danger, danger }], cancelValue: false });
const alertDlg = (title, text, pre) => dialog({ title, text, pre, buttons: [{ label: 'OK', value: true, primary: true }] });

function listPick(title, items, current) {
  return new Promise(resolve => {
    const btns = items.map(it => h('button', { 'data-nav': true, class: String(it.v) === String(current) ? 'sel' : '' }, it.l, String(it.v) === String(current) ? svg(I.check) : ''));
    const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, title), h('div', { class: 'list-pick' }, btns)));
    btns.forEach((b, i) => { const s = b.querySelector('svg'); if (s) { s.style.width = '1.2em'; s.style.fill = 'var(--accent)'; } b.addEventListener('click', () => { layer.close(); resolve(items[i].v); }); });
    const layer = openLayer(box, { onBack: () => { layer.close(); resolve(undefined); }, initial: btns.find(b => b.classList.contains('sel')) || btns[0] });
  });
}

function sheet(title, build, { onClose, headExtra } = {}) {
  const body = h('div', { class: 'sheet-body' });
  const el = h('div', { class: 'sheet' }, h('div', { class: 'sheet-head' }, h('h2', {}, title), headExtra || ''), body);
  const layer = openLayer(el, { sheet: true, onBack: () => { layer.close(); onClose && onClose(); } });
  const rebuild = () => { const keep = Nav.cur && body.contains(Nav.cur) ? Nav.items(body).indexOf(Nav.cur) : 0; body.innerHTML = ''; build(body, layer, rebuild); const items = Nav.items(body); Nav.setFocus(items[clamp(keep, 0, items.length - 1)] || Nav.items(el)[0], { silent: true }); };
  layer.rebuild = rebuild;
  rebuild();
  return layer;
}

function toast(text, icon = I.info, ms = 2600) {
  const t = h('div', { class: 'toast' }, svg(icon), text);
  $('#toasts').append(t);
  setTimeout(() => { t.classList.add('out'); setTimeout(() => t.remove(), 300); }, ms);
}

function pickImage() { return pickFile('image/png,image/jpeg,image/webp,image/gif'); }
function pickFile(accept) {
  return new Promise(resolve => {
    const inp = $('#file-input');
    inp.value = '';
    inp.accept = accept;
    inp.onchange = () => resolve(inp.files[0] || null);
    inp.click();
  });
}

// ------------------------------------------------------------------ setting row builders
function rowButton(label, { sub, value, onClick, danger, disabled, icon } = {}) {
  const el = h('button', { class: 'row' + (danger ? ' danger' : '') + (disabled ? ' disabled' : ''), 'data-nav': !disabled || undefined },
    h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)),
    h('span', { class: 'rv' }, value ?? '', icon ? svg(icon) : ''));
  const ic = el.querySelector('.rv svg'); if (ic) { ic.style.width = '1.2em'; ic.style.fill = 'currentColor'; }
  if (onClick && !disabled) el.addEventListener('click', onClick);
  return el;
}
function rowToggle(label, on, onChange, sub) {
  const tg = h('span', { class: 'toggle' + (on ? ' on' : '') });
  const el = h('button', { class: 'row', 'data-nav': true }, h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)), tg);
  el.addEventListener('click', () => { on = !on; tg.classList.toggle('on', on); onChange(on); });
  return el;
}
function rowChoice(label, choices, value, onChange, sub) {
  let idx = Math.max(0, choices.findIndex(c => String(c.v) === String(value)));
  const val = h('span', {}, choices[idx]?.l ?? '—');
  const el = h('button', { class: 'row', 'data-nav': true }, h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)),
    h('span', { class: 'rv' }, h('span', { class: 'arrows' }, '‹'), val, h('span', { class: 'arrows' }, '›')));
  const set = i => { idx = (i + choices.length) % choices.length; val.textContent = choices[idx].l; onChange(choices[idx].v); };
  el._adjust = d => { Sound.play('move', .5); set(idx + d); };
  el.addEventListener('click', async () => {
    const v = await listPick(label, choices, choices[idx].v);
    if (v !== undefined) set(choices.findIndex(c => c.v === v));
  });
  return el;
}
function rowRange(label, { min, max, step, value, fmt = v => v, onChange, sub }) {
  const out = h('span', {}, fmt(value));
  const inp = h('input', { type: 'range', min, max, step, value, tabindex: -1 });
  const el = h('div', { class: 'row', 'data-nav': true }, h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)), h('span', { class: 'rv' }, inp, out));
  let t = null;
  const commit = () => { clearTimeout(t); t = setTimeout(() => onChange(+inp.value, true), 250); };
  inp.addEventListener('input', () => { out.textContent = fmt(+inp.value); onChange(+inp.value, false); commit(); });
  el._adjust = d => { inp.value = clamp(+inp.value + d * step, min, max); inp.dispatchEvent(new Event('input')); };
  el.addEventListener('click', e => { if (e.target !== inp) el._adjust(1); });
  return el;
}
// NEXUS volume control, 0-200 %. 0 is silent, 100 % is the game's own level (marked on the
// track), above it the engine boosts with a soft limiter. Drag, click, or use left/right
// (D-pad / arrow keys, 5 % steps). onChange(value, commit) fires live while dragging and once
// more with commit=true when the user lets go.
function volumeSlider({ value = 100, onChange, compact = false, label = 'Volume' }) {
  const fill = h('i', { class: 'vol-fill' }), knob = h('i', { class: 'vol-knob' });
  const track = h('span', { class: 'vol-track' }, h('i', { class: 'vol-boost' }), fill, h('i', { class: 'vol-ref', title: '100 %' }), knob);
  const out = h('b', { class: 'vol-val' });
  const icon = h('span', { class: 'vol-icon' });
  const el = h('div', { class: 'vol' + (compact ? ' compact' : ''), 'data-nav': true, role: 'slider', 'aria-label': label, 'aria-valuemin': 0, 'aria-valuemax': 200 }, icon, track, out);
  let v = clamp(Math.round(value), 0, 200), t = null;
  const paint = () => {
    const pct = v / 2;
    fill.style.width = pct + '%'; knob.style.left = pct + '%';
    el.classList.toggle('boost', v > 100); el.classList.toggle('muted', v === 0);
    out.textContent = v === 0 ? 'Mute' : v + '%';
    icon.innerHTML = ''; icon.append(svg(v === 0 ? I.volMute : v > 100 ? I.volHigh : I.vol));
    el.setAttribute('aria-valuenow', v);
  };
  const set = (nv, commitNow) => {
    nv = clamp(Math.round(nv), 0, 200);
    if (nv !== v) { v = nv; paint(); onChange && onChange(v, false); }
    clearTimeout(t);
    if (commitNow) onChange && onChange(v, true); else t = setTimeout(() => onChange && onChange(v, true), 300);
  };
  const fromX = x => { const r = track.getBoundingClientRect(); let nv = (x - r.left) / r.width * 200; if (Math.abs(nv - 100) < 4) nv = 100; return nv; };
  track.addEventListener('pointerdown', e => {
    e.preventDefault(); track.setPointerCapture(e.pointerId); el.classList.add('drag'); set(fromX(e.clientX), false);
    const move = ev => set(fromX(ev.clientX), false);
    const up = ev => { track.releasePointerCapture(ev.pointerId); el.classList.remove('drag'); track.removeEventListener('pointermove', move); track.removeEventListener('pointerup', up); set(fromX(ev.clientX), true); };
    track.addEventListener('pointermove', move); track.addEventListener('pointerup', up);
  });
  el._adjust = d => { Sound.play('move', .4); set(v + d * 5, false); };
  icon.addEventListener('click', () => set(v === 0 ? 100 : 0, true));
  el.setValue = nv => { v = clamp(Math.round(nv), 0, 200); paint(); };
  paint();
  return el;
}
function rowVolume(label, value, onChange, sub) {
  const sl = volumeSlider({ value, onChange, label });
  const el = h('div', { class: 'row vol-row' }, h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)), sl);
  el._slider = sl;
  return el;
}
let volPost = null;
function sendVolume(v, commit) {
  // live: applied to the running game immediately; stored when the user lets go
  clearTimeout(volPost);
  volPost = setTimeout(() => api('/api/emulator/volume', { body: { percent: v } }).then(r => {
    S.settings.engine = S.settings.engine || {}; S.settings.engine['audio.volume'] = String(r.volume);
    if (commit) toast(`Volume ${r.volume === 0 ? 'muted' : r.volume + '%'}`, I.check, 900);
  }).catch(e => toast(e.message, I.warn)), commit ? 0 : 40);
}
function rowSwatches(label, colors, value, onChange, { custom = true } = {}) {
  const wrap = h('div', { class: 'swatches', 'data-row': 'strict' });
  const el = h('div', { class: 'row' }, h('span', { class: 'rl' }, h('span', {}, label)), wrap);
  const draw = () => {
    wrap.innerHTML = '';
    for (const c of colors) {
      const b = h('button', { class: 'sw' + (c.toLowerCase() === (value || '').toLowerCase() ? ' on' : ''), style: { '--c': c }, 'data-nav': true, title: c });
      b.addEventListener('click', () => { value = c; onChange(c); draw(); Nav.setFocus($$('.sw', wrap)[colors.indexOf(c)], { silent: true }); });
      wrap.append(b);
    }
    if (custom) {
      const isCustom = !colors.some(c => c.toLowerCase() === (value || '').toLowerCase());
      const picker = h('input', { type: 'color', value: /^#[0-9a-f]{6}$/i.test(value) ? value : '#ffffff', style: { position: 'absolute', opacity: 0, width: 0, height: 0 } });
      const b = h('button', { class: 'sw' + (isCustom ? ' on' : ''), style: { '--c': isCustom ? value : 'conic-gradient(red,yellow,lime,cyan,blue,magenta,red)' }, 'data-nav': true, title: 'Custom colour' });
      b.addEventListener('click', () => picker.click());
      picker.addEventListener('change', () => { value = picker.value; onChange(value); draw(); });
      wrap.append(b, picker);
    }
  };
  draw();
  return el;
}

// ------------------------------------------------------------------ theme
function applyTheme() {
  const p = S.profile;
  const th = p?.theme || { mode: 'dark', accent: '#ff2a3a', animations: 'full', wallpaper: {} };
  const b = document.body;
  b.classList.toggle('theme-dark', th.mode !== 'light');
  b.classList.toggle('theme-light', th.mode === 'light');
  ['full', 'reduced', 'off'].forEach(a => b.classList.toggle('anim-' + a, (th.animations || 'full') === a));
  S.anim = th.animations;
  document.documentElement.style.setProperty('--accent', th.accent || '#ff2a3a');
  document.documentElement.style.setProperty('--accent-ink', inkFor(th.accent || '#ff2a3a'));
  document.documentElement.style.setProperty('--accent-text', accentTextFor(th.accent || '#ff2a3a', th.mode === 'light'));
  Wall.apply(th.wallpaper || {});
  const bg = th.mode === 'light' ? '#ececee' : '#1d1e21';
  document.querySelector('meta[name=theme-color]')?.setAttribute('content', bg);
}

// ------------------------------------------------------------------ wallpaper (static + animated)
// The animated wallpaper (GIF, animated WebP, MP4, WebM) only plays while it can be seen: on the
// Home, Library and user screens, with NEXUS in front. Otherwise — game running, NEXUS minimised or
// in the background, other screens, animations off — the still frame is shown and nothing decodes.
const Wall = {
  wp: {}, key: '', media: null, failed: false, focused: true,
  url(f) { return f ? `/wallpaper/${encodeURIComponent(f)}` : ''; },
  apply(wp) {
    this.wp = wp || {};
    const b = document.body, box = $('#wallpaper .wp-img');
    const anim = this.wp.use === 'animated' && this.wp.animated;
    const still = anim ? this.wp.poster : this.wp.file;
    // no picture of the user's own: the OOT Launch background, when that animation is chosen
    const oot = !anim && !this.wp.file && typeof ootHomeWallpaper === 'function' ? ootHomeWallpaper() : '';
    b.classList.toggle('has-wp', !!(anim || this.wp.file || oot));
    b.classList.toggle('oot-wp', !!oot);
    box.style.backgroundImage = still ? `url("${this.url(still)}")` : oot ? `url("${oot}")` : '';
    const fit = { cover: ['cover', 'no-repeat'], contain: ['contain', 'no-repeat'], stretch: ['100% 100%', 'no-repeat'], center: ['auto', 'no-repeat'], tile: ['auto', 'repeat'] }[this.wp.fit || 'cover'];
    box.style.backgroundSize = fit[0]; box.style.backgroundRepeat = fit[1];
    if (oot) { box.style.filter = ''; box.style.inset = '0'; box.style.backgroundSize = 'cover'; }
    else if (anim || this.wp.file) {
      box.style.filter = `blur(${this.wp.blur || 0}px) brightness(${this.wp.brightness ?? .55})`;
      box.style.inset = this.wp.blur ? `-${Math.ceil(this.wp.blur * 2 + 10)}px` : '0';
    } else { box.style.filter = ''; box.style.inset = '0'; }
    const key = anim ? this.wp.animated : '';
    if (key !== this.key) {
      this.media?.remove(); this.media = null; this.failed = false; this.key = key;
      if (anim) {
        const objFit = { cover: 'cover', contain: 'contain', stretch: 'fill', center: 'none', tile: 'cover' }[this.wp.fit || 'cover'];
        if (this.wp.animatedType?.startsWith('video/')) {
          const v = h('video', { class: 'wp-media', muted: true, loop: true, playsinline: true, preload: 'auto', disablepictureinpicture: true });
          v.muted = true; v.src = this.url(this.wp.animated);
          if (this.wp.poster) v.poster = this.url(this.wp.poster);
          v.addEventListener('error', () => this.fail());
          this.media = v;
        } else {
          const i = h('img', { class: 'wp-media', alt: '', decoding: 'async' });
          i.addEventListener('error', () => this.fail());
          i._anim = this.url(this.wp.animated);
          this.media = i;
        }
        this.media.style.objectFit = objFit;
        box.append(this.media);
      }
    }
    if (this.media) this.media.style.objectFit = { cover: 'cover', contain: 'contain', stretch: 'fill', center: 'none', tile: 'cover' }[this.wp.fit || 'cover'];
    this.update();
  },
  fail() {
    if (this.failed) return;
    this.failed = true;
    this.media?.remove(); this.media = null;
    if (!this.wp.poster) document.body.classList.toggle('has-wp', !!this.wp.file);
    toast('The animated wallpaper could not be played; showing a still image instead.', I.warn, 4000);
  },
  shouldPlay() {
    return !!this.media && !this.failed && this.wp.motion !== 'off' && S.anim !== 'off' && !document.hidden && this.focused &&
      ['home', 'library', 'who'].includes(S.screen) && !(S.playing && !S.playing.suspended) &&
      !matchMedia('(prefers-reduced-motion: reduce)').matches;
  },
  update() {
    const m = this.media; if (!m) return;
    const play = this.shouldPlay();
    m.classList.toggle('on', play || (m.tagName === 'VIDEO' && !this.wp.poster));
    if (m.tagName === 'VIDEO') {
      if (play) { const pr = m.play(); pr?.catch?.(() => { }); } else m.pause();
    } else {
      const want = play ? m._anim : '';
      if (want && m.getAttribute('src') !== want) m.src = want;
      if (!want && m.getAttribute('src')) m.removeAttribute('src'); // stop decoding the GIF/WebP
    }
  },
};
document.addEventListener('visibilitychange', () => Wall.update());
window.addEventListener('focus', () => { Wall.focused = true; Wall.update(); });
window.addEventListener('blur', () => { Wall.focused = false; Wall.update(); });

// ------------------------------------------------------------------ avatar
function avatarEl(p, cls = '') {
  // --pc: the user's profile colour (their accent), drawn as a ring wherever the user is shown
  const el = h('span', { class: 'avatar ' + cls, style: { '--c': p?.avatar?.color || '#666' } });
  if (p?.theme?.accent) el.style.setProperty('--pc', p.theme.accent);
  if (p?.avatar?.kind === 'image') el.append(h('img', { src: `/avatar/${p.id}?v=${p.avatar.rev || 0}`, alt: '' }));
  else { const g = GLYPHS[p?.avatar?.preset] || GLYPHS.star; el.append(svg(g[0], '', !!g[1])); }
  return el;
}

// ------------------------------------------------------------------ boot sequence
const Boot = {
  done: false, skipping: false, resolve: null, anims: [],
  build(logo) {
    const part = (cls, img) => h('div', { class: 'bm ' + cls, style: { '--m': `url(/assets/brand/${img})` } });
    // emblem as two halves (left grip, right grip) that snap together, then the wordmark
    const el = { el: part('em', 'emblem-l.png'), er: part('em', 'emblem-r.png'), wn: part('wn', 'word-nexus.png'), ws: part('ws', 'word-switch.png') };
    el.shine = h('div', { class: 'shine' }, h('i'));
    logo.replaceChildren(el.el, el.er, el.wn, el.ws, el.shine);
    return el;
  },
  async preview(kind) {
    this.previewKind = kind || S.settings.startup?.animation || 'nexus';
    if (this.previewKind === 'oot' && !Sound.buf.oot_launch) await Sound.load(['oot_launch']);
    if (!$('#boot')) document.body.prepend(h('section', { id: 'boot', 'aria-label': 'NEXUS startup' }, h('div', { id: 'boot-logo', 'aria-hidden': 'true' })));
    Object.assign(this, { done: false, skipping: false, resolve: null, anims: [] });
    document.body.classList.add('booting');
    await this.run(true);
  },
  async run(force = false) {
    const st = S.settings.startup || {};
    const boot = $('#boot');
    const logo = $('#boot-logo');
    const color = st.color || '#e60012';
    logo.style.setProperty('--ink', luminance(color) > .55 ? '#141414' : '#ffffff');
    boot.style.setProperty('--boot', color);
    if (!st.enabled && !force) { this.finishNow(); return; } // nothing to show: no artwork to wait for
    const el = this.build(logo);
    // make sure the artwork is decoded before the animation starts
    const missing = new Set();
    await Promise.all(['emblem-l.png', 'emblem-r.png', 'word-nexus.png', 'word-switch.png', 'logo-full.png'].map(n => new Promise(r => { const i = new Image(); i.onload = r; i.onerror = () => { missing.add(n); r(); }; i.src = '/assets/brand/' + n; })));
    // fallback: if a wordmark image is unavailable, draw the word with the same typeface (Outfit) as live text
    if (missing.has('word-nexus.png')) el.wn.replaceWith(el.wn = h('div', { class: 'bm wn txt' }, 'NEXUS'));
    if (missing.has('word-switch.png')) el.ws.replaceWith(el.ws = h('div', { class: 'bm ws txt' }, 'SWITCH'));
    const short = st.style === 'short';
    const T = short ? .7 : 1;
    const reduced = S.anim === 'off';
    const E = (n, kf, o) => { const a = n.animate(kf, { fill: 'both', ...o }); this.anims.push(a); return a; };
    // the special-edition animation (web/bootoot.js); its sound is one track timed to it
    if ((force && this.previewKind ? this.previewKind : st.animation) === 'oot' && this.goOOT) {
      await new Promise(r => { this.resolve = r; this.goOOT(boot, reduced, E); });
      return;
    }
    [el.el, el.er, el.wn, el.ws].forEach(e => e.style.opacity = 0);
    await new Promise(r => { this.resolve = r; this.go(boot, color, T, reduced, E, { ...el, short, logo }); });
  },
  go(boot, color, T, reduced, E, el) {
    const ms = v => v * T;
    const snd = (S.settings.startup?.sound ?? true) && !reduced;
    const sfx = (n, t, vol = 1, o = {}) => { if (snd) Sound.play(n, vol, { ...o, at: t * T / 1000, force: true }); };
    // the emblem starts centred on screen; the logo rises when the wordmark appears
    const centre = 'translateY(22.3%)';
    // 1) power on: black -> startup colour, with a soft glow blooming from the centre
    E(boot, [{ background: '#000' }, { background: color }], { duration: ms(380), easing: 'ease-out' });
    const glow = h('div', { class: 'glow' }); boot.prepend(glow);
    E(glow, [{ opacity: 0 }, { opacity: 1 }], { duration: ms(1400), delay: ms(120), easing: 'ease-out' });
    sfx('boot_power', 0, .9);
    // 2) the two halves of the emblem slide in from either side and click together
    const ease = 'cubic-bezier(.2,.85,.25,1.12)';
    E(el.el, [{ opacity: 0, transform: 'translateX(-95%) rotate(-12deg) scale(.86)' }, { opacity: 1, offset: .3 }, { opacity: 1, transform: 'none' }], { duration: ms(560), delay: ms(330), easing: ease });
    E(el.er, [{ opacity: 0, transform: 'translateX(95%) rotate(12deg) scale(.86)' }, { opacity: 1, offset: .3 }, { opacity: 1, transform: 'none' }], { duration: ms(560), delay: ms(400), easing: ease });
    sfx('boot_whoosh', 300, .7, { pan: -.7 }); sfx('boot_whoosh', 370, .7, { pan: .7, rate: 1.08 });
    sfx('boot_snap', 810, 1, { pan: -.45 }); sfx('boot_snap', 880, 1, { pan: .45, rate: 1.12 });
    E(el.el, [{ transform: 'none' }, { transform: 'translateX(1.5%)' }, { transform: 'none' }], { duration: 200, delay: ms(890), easing: 'ease-out', composite: 'add' });
    E(el.er, [{ transform: 'none' }, { transform: 'translateX(-1.5%)' }, { transform: 'none' }], { duration: 200, delay: ms(960), easing: 'ease-out', composite: 'add' });
    // 3) the joined emblem pops and rings of light pulse outward
    for (const p of [el.el, el.er]) E(p, [{ transform: 'none' }, { transform: 'scale(1.06)' }, { transform: 'none' }], { duration: ms(420), delay: ms(1010), easing: 'cubic-bezier(.3,.7,.3,1)', composite: 'add' });
    sfx('boot_draw', 980, .6);
    for (const [i, d] of [[0, 1040], [1, 1220]]) {
      const ring = h('div', { class: 'flash' }); boot.append(ring);
      E(ring, [{ transform: 'scale(.25)', opacity: 0 }, { transform: 'scale(.4)', opacity: i ? .4 : .75, offset: .08 }, { transform: 'scale(2.8)', opacity: 0 }], { duration: ms(1100), delay: ms(d), easing: 'cubic-bezier(.2,.7,.3,1)' });
    }
    sfx('boot_sparkle', 1060, .7);
    // 4) the logo rises; NEXUS fades up and SWITCH opens out from the centre
    E(el.logo, [{ transform: centre }, { transform: 'none' }], { duration: ms(620), delay: ms(1300), easing: 'cubic-bezier(.2,.8,.2,1)' });
    E(el.wn, [{ opacity: 0, transform: 'translateY(30%)' }, { opacity: 1, transform: 'none' }], { duration: ms(420), delay: ms(1420), easing: 'cubic-bezier(.2,.8,.2,1)' });
    E(el.ws, [{ opacity: 0, clipPath: 'inset(0 50% 0 50%)', transform: 'scaleX(.9)' }, { opacity: 1, offset: .2 }, { opacity: 1, clipPath: 'inset(0 0 0 0)', transform: 'none' }], { duration: ms(640), delay: ms(1620), easing: 'cubic-bezier(.2,.8,.2,1)' });
    sfx('boot_chime', 1400, 1);
    // light sweep across the finished logo
    E(el.shine.firstChild, [{ transform: 'translateX(-40%) skewX(-18deg)', opacity: 0 }, { opacity: 1, offset: .15 }, { opacity: 1, offset: .85 }, { transform: 'translateX(300%) skewX(-18deg)', opacity: 0 }], { duration: ms(1000), delay: ms(2150), easing: 'ease-in-out' });
    // 5) hold, then fade to the home screen
    const hold = el.short ? 2200 : 3400;
    E(el.logo, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.97)' }], { duration: ms(520), delay: ms(hold), easing: 'ease-in', fill: 'forwards' });
    E(glow, [{ opacity: 1 }, { opacity: 0 }], { duration: ms(500), delay: ms(hold), easing: 'ease-in' });
    sfx('boot_outro', hold - 40, .8);
    const bgTo = getComputedStyle(document.body).getPropertyValue('--bg').trim() || '#1d1e21';
    const fade = E(boot, [{ background: color }, { background: bgTo }], { duration: ms(600), delay: ms(hold + 120), easing: 'ease-in-out' });
    fade.finished.then(() => this.finish()).catch(() => { });
    if (reduced) this.finish();
  },
  skip() {
    if (this.done || this.skipping) return;
    this.skipping = true;
    const boot = $('#boot');
    this.anims.forEach(a => { try { a.cancel(); } catch { } });
    Sound.stopAll(.2);
    const a = boot.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 220, fill: 'both' });
    a.finished.then(() => this.finish());
    if (!this.resolve) this.finish();
  },
  finishNow() { this.finish(); },
  finish() {
    if (this.done) return;
    this.done = true;
    document.body.classList.remove('booting');
    $('#boot').remove();
    this.resolve && this.resolve();
  },
};

// ------------------------------------------------------------------ data loading
async function loadState(boot) {
  const st = await api('/api/state' + (boot ? '?boot=1' : ''));
  S.bootGames = st.bootGames || null;
  Object.assign(S, { app: st.app, settings: st.settings, profiles: st.profiles || [], emulator: st.emulator, engine: st.engine || {}, formats: st.formats, scanDirs: st.scanDirs, gamesDir: st.gamesDir, avatarColors: st.avatarColors, avatarGlyphs: st.avatarGlyphs, padModels: st.padModels || {}, maxProfiles: st.maxProfiles || 8, library: st.library, features: st.features || {} });
  if (S.profile) S.profile = profileById(S.profile.id) || null;
  api('/api/platform').then(pl => { S.platform = pl; }).catch(() => { });
  AccountUI.load().then(() => { if (S.screen === 'home') renderBanner(); });
}
async function loadGames() {
  const r = await api('/api/games?profile=' + encodeURIComponent(S.profile?.id || ''));
  S.games = r.games || []; S.stats = r.stats || {}; S.recent = r.recent || []; S.library = r.status;
  api('/api/compat').then(c => { S.compat = c || {}; }).catch(() => { });
}
async function refreshEmu() { S.emulator = (await api('/api/state')).emulator; }

// ------------------------------------------------------------------ screens
const Screens = {};
function show(name, { initial, anim = true } = {}) {
  for (const s of ['who', 'home', 'library', 'settings', 'controllers', 'playing', 'media']) $('#' + s).hidden = s !== name;
  S.screen = name;
  const root = $('#' + name);
  if (anim && S.anim !== 'off') { root.classList.remove('screen-enter'); void root.offsetWidth; root.classList.add('screen-enter'); }
  const def = Screens[name] || {};
  Nav.base({ root, name, onBack: def.onBack, onOptions: def.onOptions, onTab: def.onTab, initial });
  renderHints();
  Wall.update();
}
// Home: from anywhere in NEXUS, or from a running game (the game is suspended, not closed).
function goHome(opts = {}) {
  // NEXUS Home is always the Home screen: a game in front is set aside first (it keeps running
  // in the background or pauses, per Settings) and its tile resumes it. While it is still
  // starting, the loading screen stays.
  if (S.playing && !S.playing.suspended) {
    if (S.playing.phase === 'playing') { suspendGame(); return; } // emulator.home then shows Home
    showPlaying(); return;
  }
  if (!S.profile) { showWho(); return; }
  [...Nav.layers].filter(l => l.dialog).forEach(l => l.close());
  const already = S.screen === 'home';
  renderHome(); show('home', { initial: $('#tiles .tile'), anim: false });
  if (!already || opts.fromGame) homeTransition(opts.fromGame);
}
function homeTransition(fromGame) {
  const style = S.profile?.theme?.homeTransition || 'zoom';
  if (style === 'off' || S.anim === 'off') return;
  const home = $('#home');
  home.classList.remove('ht-zoom', 'ht-fade', 'ht-slide', 'ht-game'); void home.offsetWidth;
  home.classList.add('ht-' + style); if (fromGame) home.classList.add('ht-game');
  setTimeout(() => home.classList.remove('ht-zoom', 'ht-fade', 'ht-slide', 'ht-game'), 600);
}
// Android: the app shows the game surface instead of this interface while a game is in front
// (MainActivity.kt). On Windows the game has its own window and this does nothing.
// A game in front covers NEXUS: its animations pause (game-front) and periodic refreshes wait,
// so the hidden interface does not keep compositing next to the game.
function gameInFront() { const st = S.playing; return !!(st?.running && st.phase === 'playing' && !st.suspended); }
function syncGameInFront() {
  const front = gameInFront();
  document.body.classList.toggle('game-front', front);
  if (!front) { tickClock(); }
  if (!window.NexusAndroid) return;
  try { NexusAndroid.gameVisible(front); } catch { }
}
async function suspendGame() {
  try { await api('/api/emulator/suspend', { body: { on: true } }); } catch (e) { toast(e.message, I.warn); }
}
async function resumeGame() {
  const style = S.profile?.theme?.homeTransition || 'zoom';
  if (S.screen === 'home' && style !== 'off' && S.anim !== 'off') { $('#home').classList.add('ht-out'); await sleep(180 * (S.anim === 'reduced' ? .45 : 1)); }
  $('#home').classList.remove('ht-out');
  try { await api('/api/emulator/suspend', { body: { on: false } }); } catch (e) { toast(e.message, I.warn); return; }
  if (S.playing) { S.playing.suspended = false; showPlaying(); }
}

// ---------- hints bar
function renderHints() {
  const list = $('#hint-list'); if (!list) return;
  const pad = S.lastInput === 'gamepad' || (S.lastInput !== 'keyboard' && S.pads.length);
  const map = Pad.mapping();
  const items = [];
  const l = Nav.top();
  // controller prompts show the active controller's own buttons (web/pads.js)
  const k = (padIdx, kb) => pad ? PadInfo.glyph(padIdx) : h('span', { class: 'key kb' }, kb);
  if (resetTarget(Nav.cur)) items.push([k(map.options, 'X'), 'Reset to default', () => Nav.options()]);
  else if (homeContextMenu(Nav.cur)) items.push([pad ? PadInfo.glyph(map.search) : h('span', { class: 'key kb' }, 'X'), 'Game menu', () => homeContextMenu(Nav.cur)?.()]);
  else if (l?.onOptions) items.push([k(map.options, 'X'), 'Options', () => Nav.options()]);
  if (S.screen === 'home' && !l?.dialog) items.push([k(map.menu, 'Esc'), 'Power', () => openPower()]);
  if (l?.onBack && (l.dialog || S.screen !== 'home')) items.push([k(map.back, 'Esc'), 'Back', () => Nav.back()]);
  items.push([k(map.confirm, 'Enter'), 'OK', () => Nav.activate()]);
  list.innerHTML = '';
  for (const [key, text, fn] of items) { const e = h('span', { class: 'hint' }, key, text); e.addEventListener('click', fn); list.append(e); }
  const hp = $('#hint-pad'); hp.innerHTML = '';
  const act = PadInfo.active();
  hp.append(svg(I.pad), S.pads.length ? (act ? act.name : 'Controller') + (S.pads.length > 1 ? ` (+${S.pads.length - 1})` : '') : 'Keyboard & mouse');
  hp.title = (S.padInfo || []).map(i => i.name).join(', ');
}

// ---------- who's playing
function showWho() {
  if (!S.profiles.length) { setupWizard({ first: true }); return; } // first launch: console-style setup
  const list = $('#who-list'); list.innerHTML = '';
  for (const p of S.profiles) {
    const last = p.id === S.settings.lastProfileId;
    const b = h('button', { class: 'who-item' + (last ? ' current' : ''), 'data-nav': true, title: p.name, 'aria-current': last ? 'true' : null }, avatarEl(p), h('span', {}, p.name), last ? h('i', { class: 'last' }, 'LAST') : '');
    b.addEventListener('click', () => selectProfile(p.id));
    b._profile = p;
    list.append(b);
  }
  const full = S.profiles.length >= S.maxProfiles;
  if (!full) {
    const add = h('button', { class: 'who-item who-add', 'data-nav': true }, h('span', { class: 'avatar' }, svg(I.plus)), h('span', {}, 'Add user'));
    add.addEventListener('click', () => createProfileFlow());
    list.append(add);
  }
  $('#who-hint').textContent = full ? `NEXUS has ${S.profiles.length} users, the maximum. Delete a user (Options › User settings) to add another.` : 'Each user has their own saves, play history, theme and controls.';
  const last = $$('.who-item', list).find(b => b._profile?.id === S.settings.lastProfileId);
  show('who', { initial: last || list.firstChild });
  $('#who').querySelector('.who-title').textContent = S.profiles.length ? "Who's playing?" : 'Welcome to NEXUS';
}
Screens.who = {
  onBack: () => { if (S.profile) goHome(); },
  onOptions: cur => { if (cur?._profile) editProfile(cur._profile.id); },
};

async function selectProfile(id) {
  S.profile = profileById(id);
  applyTheme();
  api(`/api/profiles/${id}/select`, { body: {} }).catch(() => { });
  S.settings.lastProfileId = id;
  await loadGames();
  goHome();
}

async function createProfileFlow() {
  if (S.profiles.length >= S.maxProfiles) { alertDlg('No room for another user', `NEXUS supports up to ${S.maxProfiles} users. Delete a user to make room for a new one.`); return; }
  setupWizard({ first: false });
}

// Console-style user setup: name → picture → colour → review. Shown automatically on the first
// launch (no users yet) and used for "Add user". Nothing is created until the review is confirmed.
function setupWizard({ first }) {
  const glyphs = (S.avatarGlyphs || Object.keys(GLYPHS)).filter(g => GLYPHS[g]);
  const cols = S.avatarColors?.length ? S.avatarColors : ['#0a84ff', '#ff453a', '#30d158', '#ff9f0a', '#bf5af2', '#64d2ff'];
  const d = { name: '', pic: { type: 'preset', glyph: glyphs[Math.floor(Math.random() * glyphs.length)] || 'star', color: cols[S.profiles.length % cols.length] }, accent: ACCENTS[S.profiles.length % ACCENTS.length] };
  let step = 0;
  const steps = ['Name', 'Picture', 'Colour', 'Review'];
  const root = h('div', { class: 'setup' });
  const layer = openLayer(root, { name: 'setup' });
  root.parentElement.classList.add('setup-layer');
  layer.onBack = () => { if (step > 0) { step--; draw(); } else if (!first) layer.close(); };
  const previewAvatar = (size) => {
    const fake = { name: d.name, avatar: d.pic.type === 'preset' ? { kind: 'preset', preset: d.pic.glyph, color: d.pic.color } : { kind: 'draft' }, theme: { accent: d.accent } };
    const el = avatarEl(fake, 'setup-avatar');
    if (d.pic.type !== 'preset') { el.replaceChildren(h('img', { src: d.pic.url, alt: '' })); }
    el.style.setProperty('--sz', size);
    return el;
  };
  const nav = (back, next, nextLabel = 'Next', nextOk = true) => {
    const row = h('div', { class: 'setup-nav', 'data-row': 'strict' });
    if (back) { const b = h('button', { class: 'btn', 'data-nav': true }, 'Back'); b.addEventListener('click', back); row.append(b); }
    const n = h('button', { class: 'btn primary' + (nextOk ? '' : ' disabled'), 'data-nav': true }, nextLabel); n.addEventListener('click', next); row.append(n);
    return row;
  };
  // "Sign in with my NEXUS account": GitHub sign-in, then the account's users arrive; with users
  // there is nothing to create (Who's playing? shows them), without any the local setup follows
  const firstSignIn = async () => {
    const ok = await AccountUI.githubSignIn();
    if (!ok) return;
    const wait = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, 'Getting your NEXUS account'), h('p', { class: 'note' }, 'Your users and their game saves are being copied to this device…')));
    const wl = openLayer(wait, { onBack: () => { } });
    try { await api('/api/account/sync', { body: {} }); } catch (e) { toast(e.message, I.warn, 5000); }
    await loadState();
    wl.close();
    if (S.profiles.length) { layer.close(); showWho(); toast(`${S.profiles.length === 1 ? 'Your user is' : 'Your users are'} here. Choose who is playing.`, I.check, 5000); return; }
    toast('This NEXUS account has no users yet: create yours now.', I.info, 5000);
    d.mode = 'local'; draw();
  };
  const firstCreateAccount = async () => {
    const r = await dialog({ title: 'Create a NEXUS account', text: 'NEXUS accounts use GitHub, so there is no separate password to remember and no NEXUS server holding your data: your sync data stays in a private Gist in your own GitHub account.\n\n1. Create a free GitHub account (github.com/signup) in a browser on this device, your phone or a computer.\n2. Come back and choose Sign in: NEXUS shows a code to enter at github.com/login/device.',
      buttons: [{ label: 'Back', value: 'back' }, { label: 'Open github.com/signup', value: 'open' }, { label: 'I have an account: sign in', value: 'signin', primary: true }], cancelValue: 'back' });
    if (r === 'open') { api('/api/app/open-link', { body: { which: 'github-signup' } }).catch(e => toast(e.message, I.warn)); return firstCreateAccount(); }
    if (r === 'signin') return firstSignIn();
  };
  const draw = () => {
    root.replaceChildren();
    root.style.setProperty('--accent', d.accent);
    const dots = h('div', { class: 'setup-steps' }, steps.map((t, i) => h('span', { class: i === step ? 'on' : i < step ? 'done' : '' }, h('i'), t)));
    const head = h('header', { class: 'setup-head' }, h('div', { class: 'setup-brand' }), h('h1', {}, first && step === 0 ? 'Welcome to NEXUS' : first ? 'Create your user' : 'Add a user'), dots);
    const card = h('section', { class: 'setup-card' });
    root.append(head, card);
    let initial = null;
    if (step === 0 && first && !d.mode) {
      // first launch: sign in to an existing NEXUS account (its users arrive, no extra local user
      // is needed), create one, or use NEXUS with a local user only
      const opt = (title, text, fn) => { const b = h('button', { class: 'setup-option', 'data-nav': true }, h('b', {}, title), h('span', {}, text)); b.addEventListener('click', fn); return b; };
      const opts = [
        opt('Sign in with my NEXUS account', 'You already use NEXUS on another PC or a Steam Deck: sign in, and your users, their pictures, play time and game saves come to this device. No new user is created.', () => firstSignIn()),
        opt('Create a NEXUS account', 'New to NEXUS accounts: a NEXUS account is your free GitHub account. Create one, then sign in here.', () => firstCreateAccount()),
        opt('Continue with a local user', 'Use NEXUS on this device only. You can sign in to a NEXUS account later in Settings; your local user and saves are kept.', () => { d.mode = 'local'; draw(); }),
      ];
      if (S.features?.deviceLink) opts.push(opt('Link with a code (experimental)', 'Link this device to another NEXUS on your network with a code.', async () => {
        const pid = await linkEnterCode();
        if (pid) { layer.close(); await selectProfile(pid); }
      }));
      card.append(h('h2', {}, 'Welcome'), h('div', { class: 'setup-options', 'data-row': 'strict' }, ...opts),
        h('p', { class: 'setup-sub' }, 'Your NEXUS account keeps your users, play time, settings and game saves the same on every PC and Steam Deck you sign in on. Games, keys and firmware never leave this device.'));
      initial = $('.setup-option', card);
    } else if (step === 0) {
      const inp = h('input', { type: 'text', class: 'setup-input', maxlength: 20, placeholder: 'Your name', value: d.name, 'data-nav': true, spellcheck: 'false', autocomplete: 'off' });
      const err = h('p', { class: 'setup-err' });
      const go = () => {
        const n = inp.value.trim();
        if (!n) { err.textContent = 'Type a name to continue.'; Sound.play('error'); return; }
        if (S.profiles.some(p => p.name.toLowerCase() === n.toLowerCase())) { err.textContent = `There is already a user called ${n}.`; Sound.play('error'); return; }
        d.name = n; step = 1; draw();
      };
      inp.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); go(); } });
      card.append(h('h2', {}, first ? 'Let’s set up your user.' : 'Choose a name'), h('p', { class: 'setup-sub' }, 'This is how you appear on the user screen and on the Home screen. Up to 20 characters; you can change it later.'), inp, err, nav(null, go));
      initial = inp; setTimeout(() => inp.focus(), 50);
    } else if (step === 1) {
      const big = previewAvatar('9em');
      const grid = h('div', { class: 'setup-glyphs' });
      for (const g of glyphs.slice(0, 24)) {
        const b = h('button', { class: 'pk-item pk-glyphitem' + (d.pic.type === 'preset' && d.pic.glyph === g ? ' on' : ''), style: { '--c': d.pic.color }, 'data-nav': true, title: g });
        const gg = GLYPHS[g]; b.append(svg(gg[0], '', !!gg[1]));
        b.addEventListener('click', () => { d.pic = { type: 'preset', glyph: g, color: d.pic.color || cols[0] }; draw(); Nav.setFocus($$('.setup-glyphs .pk-item', root).find(x => x.title === g), { silent: true }); });
        grid.append(b);
      }
      const bgRow = h('div', { class: 'pk-colors setup-bg', 'data-row': 'strict' }, cols.map(c => {
        const b = h('button', { class: 'sw' + (d.pic.color === c ? ' on' : ''), style: { '--c': c }, 'data-nav': true, title: 'Icon background' });
        b.addEventListener('click', () => { if (d.pic.type === 'preset') { d.pic.color = c; draw(); } }); return b;
      }));
      const more = h('button', { class: 'btn', 'data-nav': true }, 'Console icons & more…');
      more.addEventListener('click', () => pickCharacter(null, null, true, { name: d.name || 'you', avatar: d.pic.type === 'preset' ? { kind: 'preset', preset: d.pic.glyph, color: d.pic.color } : {}, onPick: r => { d.pic = r; draw(); } }));
      const pc = h('button', { class: 'btn', 'data-nav': true }, 'Picture from this PC…');
      pc.addEventListener('click', async () => { const f = await pickImage(); if (f) { d.pic = { type: 'file', file: f, url: URL.createObjectURL(f) }; draw(); } });
      card.append(h('div', { class: 'setup-split' }, h('div', { class: 'setup-preview' }, big, h('b', {}, d.name)),
        h('div', { class: 'setup-choose' }, h('h2', {}, 'Choose a picture'), grid, d.pic.type === 'preset' ? h('div', { class: 'setup-label' }, 'Icon background') : '', d.pic.type === 'preset' ? bgRow : '',
          h('div', { class: 'setup-more', 'data-row': 'strict' }, more, pc))),
        nav(() => { step = 0; draw(); }, () => { step = 2; draw(); }));
      initial = $('.setup-glyphs .pk-item.on', card) || $('.setup-glyphs .pk-item', card);
    } else if (step === 2) {
      const row = h('div', { class: 'setup-colors', 'data-row': 'strict' }, ACCENTS.map(c => {
        const b = h('button', { class: 'sw' + (d.accent === c ? ' on' : ''), style: { '--c': c }, 'data-nav': true });
        b.addEventListener('click', () => { d.accent = c; draw(); Nav.setFocus($$('.setup-colors .sw', root)[ACCENTS.indexOf(c)], { silent: true }); }); return b;
      }));
      card.append(h('div', { class: 'setup-split' }, h('div', { class: 'setup-preview' }, previewAvatar('9em'), h('b', {}, d.name)),
        h('div', { class: 'setup-choose' }, h('h2', {}, 'Choose your colour'), h('p', { class: 'setup-sub' }, 'Your colour marks your user everywhere in NEXUS (the ring around your picture) and is the highlight colour of the interface while you are signed in.'), row,
          h('div', { class: 'setup-demo' }, h('span', { class: 'chip on' }, 'Selected'), h('span', { class: 'setup-demo-bar' }, h('i'))))),
        nav(() => { step = 1; draw(); }, () => { step = 3; draw(); }));
      initial = $('.setup-colors .sw.on', card);
    } else {
      const create = async () => {
        try {
          const av = d.pic.type === 'preset' ? { kind: 'preset', preset: d.pic.glyph, color: d.pic.color } : {};
          const p = await api('/api/profiles', { body: { name: d.name, avatar: av } });
          if (d.pic.type === 'file') await api(`/api/profiles/${p.id}/avatar`, { body: d.pic.file });
          if (d.pic.type === 'composed') await api(`/api/profiles/${p.id}/avatar?` + d.pic.query, { body: d.pic.blob });
          await api(`/api/profiles/${p.id}`, { method: 'PATCH', body: { theme: { accent: d.accent } } });
          await loadState();
          layer.close();
          Sound.play('select');
          toast(`${d.name} was created`, I.check);
          if (first || !S.profile) { await selectProfile(p.id); await maybeOfferSaveImport(p.id); }
          else if (S.screen === 'who') showWho();
          else refreshTop();
        } catch (e) { Sound.play('error'); alertDlg('Could not create the user', e.message); }
      };
      card.append(h('div', { class: 'setup-review' }, previewAvatar('11em'), h('h2', {}, d.name),
        h('dl', { class: 'kv' }, h('dt', {}, 'Name'), h('dd', {}, d.name), h('dt', {}, 'Picture'), h('dd', {}, d.pic.type === 'preset' ? 'NEXUS icon' : d.pic.type === 'file' ? 'Your picture' : 'Console icon'),
          h('dt', {}, 'Colour'), h('dd', {}, h('span', { class: 'setup-swatch', style: { background: d.accent } }))),
        h('p', { class: 'setup-sub' }, 'You can change all of this later in User settings.')),
        nav(() => { step = 2; draw(); }, create, first ? 'Start using NEXUS' : 'Create user'));
      initial = $('.setup-nav .primary', card);
    }
    const it = initial || Nav.items(root)[0];
    if (it) Nav.setFocus(it, { silent: true });
  };
  draw();
}

function editProfile(id) {
  const p0 = profileById(id); if (!p0) return;
  const l = sheet('User settings', (body, layer, rebuild) => {
    const p = profileById(id) || p0;
    const top = h('div', { class: 'game-hero' }, avatarEl(p, ''), h('div', { class: 'gh-info' }, h('b', {}, p.name), h('span', {}, `Created ${new Date(p.createdAt * 1000).toLocaleDateString()}`)));
    top.querySelector('.avatar').style.setProperty('--sz', '5em');
    body.append(top);
    body.append(rowButton('Name', { value: p.name, onClick: async () => {
      const n = await dialog({ title: 'Change name', input: { value: p.name, max: 20 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n && n.trim() && n.trim() !== p.name) await patchProfile(id, { name: n.trim() }).then(rebuild).catch(e => alertDlg('Could not rename', e.message));
    } }));
    body.append(rowSwatches('Profile colour', ACCENTS, p.theme?.accent, c => patchProfile(id, { theme: { accent: c } }).then(rebuild), { custom: false }));
    body.append(h('h3', {}, 'Icon'));
    body.append(rowButton('Change icon…', { sub: 'Console character icons, imported icon packs and backgrounds, with a live preview.', onClick: () => pickCharacter(id, rebuild) }));
    body.append(rowButton('Use a picture…', { sub: 'PNG, JPG or WEBP from your PC', onClick: async () => {
      const f = await pickImage(); if (!f) return;
      try { await api(`/api/profiles/${id}/avatar`, { body: f }); await loadState(); rebuild(); refreshTop(); } catch (e) { alertDlg('Could not use that picture', e.message); }
    } }));
    body.append(h('h3', {}, 'NEXUS Account'));
    body.append(AccountUI.row());
    body.append(h('h3', {}, 'Save data'));
    body.append(rowButton('Save Data…', { sub: 'This user’s saves: view slots, import from a folder or ZIP, export, back up, restore or delete.', onClick: () => saveDataPage(id) }));
    body.append(h('h3', {}, ''));
    body.append(rowButton('Delete this user', { danger: true, sub: 'The user and their saves are moved to NEXUS data\\Archived, not erased.', onClick: () => deleteProfile(id, layer) }));
  }, { onClose: () => { if (S.screen === 'who') showWho(); else refreshTop(); } });
  return l;
}

// ---------- profile page (statistics)
async function profilePage(id) {
  const p = profileById(id); if (!p) return;
  const root = h('div', { class: 'profile-page' });
  const layer = openLayer(root, { name: 'profile' });
  const build = async () => {
    let st;
    try { st = await api(`/api/profiles/${id}/stats`); } catch (e) { st = { totalSeconds: 0, gamesPlayed: 0, sessions: 0, recent: [], mostPlayed: [], games: [], byEmulator: {} }; }
    const pr = profileById(id) || p;
    root.replaceChildren();
    const head = h('header', { class: 'pp-head' }, avatarEl(pr, 'pp-avatar'),
      h('div', { class: 'pp-name' }, h('h1', {}, pr.name), h('span', {}, st.lastPlayed ? `Last played ${st.lastGame} · ${fmtAgo(st.lastPlayed)}` : 'No games played yet')));
    const edit = h('button', { class: 'chip', 'data-nav': true }, 'Edit profile');
    edit.addEventListener('click', () => editProfile(id).root && null);
    const icon = h('button', { class: 'chip', 'data-nav': true }, 'Change icon');
    icon.addEventListener('click', () => pickCharacter(id, () => build()));
    const close = h('button', { class: 'chip', 'data-nav': true }, 'Close');
    close.addEventListener('click', () => layer.close());
    head.append(h('div', { class: 'pp-actions', 'data-row': 'strict' }, edit, icon, close));
    root.append(head);
    root.append(h('div', { class: 'stat-cards big' },
      statCard('Total play time', fmtDur(st.totalSeconds)),
      statCard('Games played', String(st.gamesPlayed)),
      statCard('Play sessions', String(st.sessions)),
      statCard('Last played', st.lastPlayed ? fmtDate(st.lastPlayed) : 'Never')));
    const earlier = Object.entries(st.byEmulator || {}).filter(([k, v]) => k !== 'nexus' && v > 0).reduce((a, [, v]) => a + v, 0);
    if (earlier) root.append(h('p', { class: 'note pp-note' }, `Includes ${fmtDur(earlier)} played with earlier NEXUS versions.`));
    if (st.live) root.append(h('p', { class: 'note pp-note' }, `Playing ${st.live.title} now — this session is saved every 30 seconds.`));
    const tileFor = gs => {
      const g = gameByKey(gs.key);
      const t = g ? gameTile(g, 'pp-tile') : h('button', { class: 'tile pp-tile', 'data-nav': true }, h('div', { class: 'art ph' }, h('span', {}, initials(gs.title))));
      t.addEventListener('click', e => { e.stopImmediatePropagation(); e.preventDefault(); gameStatsSheet(gs); }, true);
      return h('div', { class: 'pp-item' }, t, h('div', { class: 'pp-cap' }, h('b', {}, gs.title), h('span', {}, `${fmtDur(gs.seconds)} · ${fmtAgo(gs.lastPlayed)}`)));
    };
    if (st.recent.length) {
      root.append(h('h2', {}, 'Recently played'));
      root.append(h('div', { class: 'pp-row', 'data-row': 'strict' }, ...st.recent.map(tileFor)));
    }
    if (st.mostPlayed.length) {
      root.append(h('h2', {}, 'Most played'));
      const max = st.mostPlayed[0].seconds || 1;
      const list = h('div', { class: 'pp-bars' });
      st.mostPlayed.forEach((gs, i) => {
        const b = h('button', { class: 'pp-bar', 'data-nav': true }, h('span', { class: 'n' }, String(i + 1)), h('span', { class: 't' }, gs.title),
          h('span', { class: 'track' }, h('i', { style: { width: Math.max(2, gs.seconds / max * 100) + '%' } })), h('span', { class: 'v' }, fmtDur(gs.seconds)));
        b.addEventListener('click', () => gameStatsSheet(gs));
        list.append(b);
      });
      root.append(list);
    }
    root.append(h('h2', {}, 'Play time per game'));
    if (!st.games.length) root.append(h('p', { class: 'note' }, 'Play time is recorded automatically while a game is running in NEXUS.'));
    else {
      const tbl = h('div', { class: 'pp-table' }, h('div', { class: 'pp-tr th' }, h('span', {}, 'Game'), h('span', {}, 'Play time'), h('span', {}, 'Sessions'), h('span', {}, 'Last played')));
      for (const gs of st.games) {
        const r = h('button', { class: 'pp-tr', 'data-nav': true }, h('span', {}, gs.title + (gs.inLibrary ? '' : ' (not in library)')), h('span', {}, fmtDur(gs.seconds)), h('span', {}, String(gs.sessions || 0)), h('span', {}, gs.lastPlayed ? fmtAgo(gs.lastPlayed) : '—'));
        r.addEventListener('click', () => gameStatsSheet(gs));
        tbl.append(r);
      }
      root.append(tbl);
    }
    root.append(h('p', { class: 'note pp-note' }, 'Statistics are stored with this user in the NEXUS data folder, so they survive NEXUS and engine updates.'));
    const first = $('[data-nav]', root);
    if (first && (!Nav.cur || !root.contains(Nav.cur))) Nav.setFocus(first, { silent: true });
  };
  layer.refresh = build;
  S.profilePage = layer;
  await build();
}
// Correct play time: hours and minutes, behind a clearly labelled button and a confirmation.
async function correctPlaytime(key, title, seconds, pid = S.profile?.id) {
  const cur = Math.round((seconds || 0) / 60);
  const v = await formDlg({ title: 'Correct play time', text: `${title}: NEXUS has ${fmtDur(seconds || 0)} for ${profileById(pid)?.name || 'this user'}. Enter the correct total. Play time recorded from now on is added to it, and the correction is synchronised with your account.`, ok: 'Save',
    fields: [{ id: 'hours', label: 'Hours', type: 'number', value: String(Math.floor(cur / 60)), max: 6 }, { id: 'minutes', label: 'Minutes', type: 'number', value: String(cur % 60), max: 2 }],
    validate: v => {
      const hh = Number(v.hours), mm = Number(v.minutes);
      if (!Number.isInteger(hh) || hh < 0 || hh > 100000) return 'Hours: a whole number from 0.';
      if (!Number.isInteger(mm) || mm < 0 || mm > 59) return 'Minutes: a whole number from 0 to 59.';
      return null;
    } });
  if (!v) return false;
  try {
    await api(`/api/profiles/${pid}/playtime`, { body: { key, hours: Number(v.hours), minutes: Number(v.minutes) } });
    toast(`Play time for ${title} set to ${v.hours} h ${v.minutes} min`, I.check);
    await loadGames().catch(() => { });
    return true;
  } catch (e) { alertDlg('Not saved', e.message); return false; }
}

function gameStatsSheet(gs) {
  sheet(gs.title, (body, layer) => {
    body.append(gameStatsBlock(gs));
    if (gs.history?.length) {
      body.append(h('h3', {}, 'Recent sessions'));
      const tbl = h('div', { class: 'pp-table' }, h('div', { class: 'pp-tr th' }, h('span', {}, 'Started'), h('span', {}, 'Length'), h('span', {}, 'Engine'), h('span', {}, '')));
      for (const x of gs.history) tbl.append(h('div', { class: 'pp-tr' }, h('span', {}, fmtDate(x.start)), h('span', {}, fmtDur(x.seconds)), h('span', {}, emuName(x.emulator)), h('span', {}, x.recovered ? 'recovered' : x.crashed ? 'stopped with an error' : '')));
      body.append(tbl);
    }
    const g = gameByKey(gs.key);
    body.append(rowButton('Correct play time\u2026', { sub: 'If NEXUS shows less (or more) time than you played.', onClick: async () => { if (await correctPlaytime(gs.key, gs.title, gs.seconds)) { layer.close(); S.profilePage?.close(); profilePage(S.profile.id); } } }));
    if (g && isPlayable(g) && !g.hidden) body.append(rowButton('Play', { icon: I.play, onClick: () => { layer.close(); S.profilePage?.close(); launchGame(g.key); } }));
  });
}

// ---------- profile icon picker
// Categories: "Console icons" read from the user's own firmware (the console's own
// avatar archive), imported icon packs (characters / backgrounds / frames),
// and the built-in NEXUS icons. NEXUS ships no Nintendo artwork.
async function pickCharacter(id, after, fresh = false, draft = null) {
  let info;
  try { info = await api('/api/avatars'); } catch (e) { alertDlg('Could not load icons', e.message); return; }
  // draft: choosing for a user that does not exist yet (setup) — the choice is returned, not saved
  const p0 = draft ? { name: draft.name, avatar: draft.avatar || {} } : profileById(id); if (!p0) return;
  const av = p0.avatar || {};
  const sel = { char: av.char || '', bg: av.bg || '', frame: av.frame || '', color: av.color || '#0a84ff', glyph: av.kind === 'preset' && !fresh ? av.preset : '' };
  const cats = [...(info.categories || [])];
  if (!cats.some(c => c.source === 'firmware')) cats.unshift({ id: 'console-missing', name: 'Console icons', source: 'missing', characters: [], backgrounds: [], frames: [] });
  cats.push({ id: 'nexus', name: 'NEXUS', source: 'builtin', characters: (S.avatarGlyphs || Object.keys(GLYPHS)).map(g => 'glyph:' + g), backgrounds: [], frames: [] });
  let cat = cats.find(c => sel.char && c.id !== 'firmware' && c.source !== 'firmware' && c.characters.includes(sel.char)) || cats.find(c => sel.char && c.characters.includes(sel.char)) || cats.find(c => c.source !== 'builtin' && c.characters.length) || cats[0];
  let tab = 'characters';
  let query = '';
  const iconName = u => u.startsWith('glyph:') ? u.slice(6) : decodeURIComponent(u.split('/').pop()).replace(/\.[a-z]+$/i, '').replace(/^(chr|chara|icon|avatar)_/i, '').replace(/[_-]+/g, ' ');
  const search = h('input', { type: 'text', placeholder: 'Search icons', 'data-nav': true, class: 'pk-search', spellcheck: 'false' });
  search.addEventListener('input', () => { query = search.value.trim().toLowerCase(); drawGrid(); });
  const selName = h('div', { class: 'pk-name' });
  const root = h('div', { class: 'picker' });
  const catList = h('nav', { class: 'pk-cats' });
  const tabs = h('div', { class: 'pk-tabs', 'data-row': 'strict' });
  const grid = h('div', { class: 'pk-grid' });
  const preview = h('div', { class: 'pk-preview' });
  const swatches = h('div', { class: 'pk-colors' });
  const side = h('section', { class: 'pk-side' }, h('h2', {}, 'Preview'), preview, selName, h('div', { class: 'pk-label' }, 'Background colour'), swatches);
  root.append(h('div', { class: 'pk-head' }, h('h2', {}, `Choose an icon for ${p0.name}`)), h('div', { class: 'pk-body' }, catList, h('main', { class: 'pk-main' }, tabs, grid), side));
  const saveBtn = h('button', { class: 'btn primary', 'data-nav': true }, 'Save icon');
  const cancelBtn = h('button', { class: 'btn', 'data-nav': true }, 'Cancel');
  side.append(h('div', { class: 'pk-actions', 'data-row': 'strict' }, cancelBtn, saveBtn));
  const glyphOf = u => u.startsWith('glyph:') ? u.slice(6) : '';

  const drawPreview = () => {
    preview.innerHTML = '';
    selName.textContent = sel.glyph ? 'NEXUS · ' + sel.glyph : sel.char ? iconName(sel.char) : 'No icon selected';
    preview.style.background = sel.bg ? `center/cover url("${sel.bg}")` : sel.color;
    if (sel.glyph) { const g = GLYPHS[sel.glyph] || GLYPHS.star; const sv = svg(g[0], '', !!g[1]); sv.classList.add('pk-glyph'); preview.append(sv); }
    else if (sel.char) preview.append(h('img', { src: sel.char, alt: '' }));
    if (sel.frame) preview.append(h('img', { src: sel.frame, alt: '', class: 'pk-frame' }));
  };
  const drawColors = () => {
    swatches.innerHTML = '';
    const cols = info.palette?.length ? info.palette : [...(S.avatarColors || []), '#ffffff', '#2b2b2b'];
    swatches.classList.toggle('pk-palette', !!info.palette?.length);
    for (const c of cols) {
      const b = h('button', { class: 'sw' + (!sel.bg && c.toLowerCase() === sel.color.toLowerCase() ? ' on' : ''), style: { '--c': c }, 'data-nav': true });
      b.addEventListener('click', () => { sel.color = c; sel.bg = ''; drawPreview(); drawColors(); drawGrid(); Nav.setFocus($$('.sw', swatches)[cols.indexOf(c)], { silent: true }); });
      swatches.append(b);
    }
  };
  const drawTabs = () => {
    tabs.innerHTML = '';
    tabs.append(search);
    for (const [t, l] of [['characters', 'Characters'], ['backgrounds', 'Backgrounds'], ['frames', 'Frames']]) {
      const n = (cat[t] || []).length;
      if (t !== 'characters' && !n) continue;
      const b = h('button', { class: 'chip' + (tab === t ? ' on' : ''), 'data-nav': true }, `${l} (${n})`);
      b.addEventListener('click', () => { tab = t; drawTabs(); drawGrid(); });
      tabs.append(b);
    }
  };
  const drawGrid = () => {
    grid.innerHTML = '';
    if (cat.source === 'missing') { grid.append(consoleIconSetup(info.status || {}, () => { layer.close(); pickCharacter(id, after, fresh, draft); })); return; }
    // searching looks through every category, not just the open one
    const pool = query && tab === 'characters' ? [...new Set(cats.filter(c => c.id !== 'firmware' || !cats.some(x => x.source === 'firmware' && x.id !== 'firmware')).flatMap(c => c.characters))] : (cat[tab] || []);
    const list = pool.filter(u => !query || iconName(u).toLowerCase().includes(query) || decodeURIComponent(u).toLowerCase().includes(query));
    if (tab !== 'characters') {
      const none = h('button', { class: 'pk-item pk-none' + ((tab === 'backgrounds' ? !sel.bg : !sel.frame) ? ' on' : ''), 'data-nav': true }, tab === 'backgrounds' ? 'Colour' : 'None');
      none.addEventListener('click', () => { if (tab === 'backgrounds') sel.bg = ''; else sel.frame = ''; drawPreview(); drawGrid(); });
      grid.append(none);
    }
    for (const u of list) {
      const g = glyphOf(u);
      const isOn = tab === 'characters' ? (g ? sel.glyph === g : sel.char === u && !sel.glyph) : (tab === 'backgrounds' ? sel.bg === u : sel.frame === u);
      const b = h('button', { class: 'pk-item' + (isOn ? ' on' : ''), 'data-nav': true, style: { '--c': sel.color }, title: iconName(u) });
      if (g) { const gg = GLYPHS[g]; b.append(svg(gg[0], '', !!gg[1])); b.classList.add('pk-glyphitem'); }
      else b.append(h('img', { src: u, alt: '', loading: 'lazy', decoding: 'async' }));
      b.addEventListener('click', () => {
        if (tab === 'characters') { if (g) { sel.glyph = g; sel.char = ''; } else { sel.char = u; sel.glyph = ''; } }
        else if (tab === 'backgrounds') sel.bg = u; else sel.frame = u;
        drawPreview(); $$('.pk-item', grid).forEach(x => x.classList.remove('on')); b.classList.add('on');
      });
      grid.append(b);
    }
    if (!list.length && tab === 'characters') grid.append(h('p', { class: 'note' }, query ? `No icons match “${query}”.` : 'No icons in this category.'));
  };
  const drawCats = () => {
    catList.innerHTML = '';
    for (const c of cats) {
      const b = h('button', { class: 'cat' + (c === cat ? ' sel' : '') + (c.source === 'firmware' && c.id !== 'firmware' ? ' sub' : ''), 'data-nav': true }, c.name, c.source === 'missing' ? h('small', {}, ' set up') : h('small', {}, ` ${c.characters.length}`));
      b.addEventListener('click', () => { cat = c; tab = 'characters'; drawCats(); drawTabs(); drawGrid(); const f = $('.pk-item', grid); if (f) Nav.setFocus(f); });
      catList.append(b);
    }
    const more = h('button', { class: 'cat pk-import', 'data-nav': true }, 'More options…');
    more.addEventListener('click', async () => {
      const v = await listPick('More options', [{ v: 'pc', l: 'Use a picture from this PC…' }, { v: 'import', l: 'Import extra icon packs (optional)…' }, { v: 'reload', l: 'Reload console icons from firmware' }]);
      if (v === 'pc' && draft) { const f = await pickImage(); if (!f) return; layer.close(); draft.onPick({ type: 'file', file: f, url: URL.createObjectURL(f) }); return; }
      if (v === 'pc') { const f = await pickImage(); if (!f) return; try { await api(`/api/profiles/${id}/avatar`, { body: f }); await loadState(); if (S.profile?.id === id) S.profile = profileById(id); layer.close(); refreshTop(); after && after(); } catch (e) { alertDlg('Could not use that picture', e.message); } }
      if (v === 'import') importIconsFlow().then(r => { if (r) { layer.close(); pickCharacter(id, after, fresh, draft); } });
      if (v === 'reload') { const st = await api('/api/avatars/provision', { body: {} }); if (st.state !== 'ready') alertDlg('Console icons unavailable', st.message); layer.close(); pickCharacter(id, after); }
    });
    catList.append(more);
  };
  const save = async () => {
    if (!sel.glyph && !sel.char) { toast('Choose an icon first', I.person); return; }
    if (draft) {
      if (sel.glyph && !sel.bg && !sel.frame) draft.onPick({ type: 'preset', glyph: sel.glyph, color: sel.color });
      else {
        const blob = await composeAvatar(sel);
        draft.onPick({ type: 'composed', blob, url: URL.createObjectURL(blob), query: new URLSearchParams({ char: sel.glyph ? '' : sel.char, bg: sel.bg, frame: sel.frame, color: sel.color }).toString() });
      }
      layer.close(); return;
    }
    try {
      if (sel.glyph && !sel.bg && !sel.frame) {
        await patchProfile(id, { avatar: { kind: 'preset', preset: sel.glyph, color: sel.color, char: '', bg: '', frame: '' } });
      } else {
        const blob = await composeAvatar(sel);
        const q = new URLSearchParams({ char: sel.glyph ? '' : sel.char, bg: sel.bg, frame: sel.frame, color: sel.color });
        await api(`/api/profiles/${id}/avatar?` + q, { body: blob });
        await loadState();
        if (S.profile?.id === id) { S.profile = profileById(id); }
      }
      layer.close(); refreshTop(); after && after(); toast('Icon saved', I.check);
    } catch (e) { alertDlg('Could not save the icon', e.message); }
  };
  saveBtn.addEventListener('click', save);
  cancelBtn.addEventListener('click', () => layer.close());
  drawCats(); drawTabs(); drawGrid(); drawPreview(); drawColors();
  Picker.reopen = () => { if (!layer.root.isConnected) return; layer.close(); pickCharacter(id, after); };
  const layer = openLayer(root, { sheet: false, name: 'picker', initial: $('.pk-item.on', grid) || $('.pk-item', grid),
    onTab: d => { const ts = $$('.chip', tabs); const i = ts.findIndex(x => x.classList.contains('on')); const n = ts[(i + d + ts.length) % ts.length]; if (n) n.click(); } });
  layer.onBack = () => layer.close();
  root.parentElement.classList.add('picker-layer');
}

const Picker = { reopen: null };

// Shown in the picker until the console icons are available. The icons are the
// console's own avatar picker shows: they come from the user's
// firmware (system archive 010000000000080A), so NEXUS loads them automatically
// as soon as firmware and prod.keys are installed.
function consoleIconSetup(st, retry) {
  const steps = {
    'no-firmware': ['Install your console firmware', 'NEXUS loads the Switch profile icons (Mario, Link, Kirby, Inkling, Isabelle and more) straight from the firmware you dumped from your own console. Install it in Settings › System files.'],
    'no-keys': ['Add your prod.keys', 'Your firmware is installed, but the icons are encrypted. Put the prod.keys dumped from your own console in the keys folder and NEXUS will load the icons automatically.'],
    'not-found': ['Icon archive not found', st.message || 'The installed firmware does not contain the avatar archive. Install the complete firmware again in Settings › System files.'],
    'error': ['Could not read the icons', st.message || 'Something went wrong while reading the firmware.'],
    'working': ['Loading profile icons…', 'Reading the icons from your firmware. This only happens once.'],
  }[st.state] || ['Console icons', 'Checking for installed firmware…'];
  const box = h('div', { class: 'pk-setup' }, svg(st.state === 'working' ? I.refresh : I.person), h('h3', {}, steps[0]), h('p', {}, steps[1]));
  const row = h('div', { class: 'pk-setup-actions', 'data-row': 'strict' });
  const btn = (label, fn, primary) => { const b = h('button', { class: 'btn' + (primary ? ' primary' : ''), 'data-nav': true }, label); b.addEventListener('click', fn); row.append(b); };
  if (st.state !== 'working') btn('Try again', async () => { const r = await api('/api/avatars/provision', { body: {} }); if (r.state === 'ready') toast(`Loaded ${r.count} profile icons`, I.check); retry(); }, true);
  if (st.state === 'no-firmware' || st.state === 'not-found') btn('System files', () => showSettings('sysfiles'));
  if (st.state === 'no-keys') btn('Open keys folder', () => api('/api/app/open-folder', { body: { which: 'keys' } }));
  box.append(row, h('p', { class: 'note' }, 'NEXUS keeps a local copy of the icons in NEXUS data\\Avatars\\firmware so they work offline. The icons are Nintendo artwork, so they are not shipped with NEXUS.'));
  return box;
}

// Draw the chosen layers into a 256x256 PNG.
async function composeAvatar(sel) {
  const load = src => new Promise((res, rej) => { const im = new Image(); im.onload = () => res(im); im.onerror = () => rej(new Error('Could not load ' + src)); im.src = src; });
  const c = document.createElement('canvas'); c.width = c.height = 256;
  const x = c.getContext('2d');
  if (sel.bg) { const im = await load(sel.bg); const s = Math.max(256 / im.width, 256 / im.height); x.drawImage(im, (256 - im.width * s) / 2, (256 - im.height * s) / 2, im.width * s, im.height * s); }
  else { x.fillStyle = sel.color || '#0a84ff'; x.fillRect(0, 0, 256, 256); }
  if (sel.glyph) {
    const g = GLYPHS[sel.glyph] || GLYPHS.star;
    const p = new Path2D(g[0]); x.save(); x.translate(128 - 62 * 1, 128 - 62); x.scale(124 / 24, 124 / 24); x.fillStyle = '#fff'; x.fill(p, g[1] ? 'evenodd' : 'nonzero'); x.restore();
  } else if (sel.char) { const im = await load(sel.char); const s = Math.min(256 / im.width, 256 / im.height); x.drawImage(im, (256 - im.width * s) / 2, (256 - im.height * s) / 2, im.width * s, im.height * s); }
  if (sel.frame) { const im = await load(sel.frame); x.drawImage(im, 0, 0, 256, 256); }
  return new Promise(res => c.toBlob(b => res(b), 'image/png'));
}

async function importIconsFlow() {
  const how = await listPick('Import profile icons', [
    ...(S.app.native ? [{ v: 'folder', l: 'Import a folder…' }] : []),
    { v: 'zip', l: 'Import a .zip file…' },
    { v: 'path', l: 'Type a folder path…' },
    { v: 'open', l: 'Open the icons folder' },
  ]);
  if (!how) return false;
  if (how === 'open') { api('/api/app/open-folder', { body: { which: 'avatars' } }); return false; }
  try {
    let r;
    if (how === 'folder') r = await api('/api/avatars/import', { body: { path: '' } });
    if (how === 'path') {
      const path = await dialog({ title: 'Import icon folder', text: 'Full path of a folder with PNG/JPG/WEBP icons. Sub-folders become categories; characters/backgrounds/frames folders are recognised.', input: { placeholder: 'D:\\Icons\\nso-icons', max: 400 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Import', value: true, primary: true }] });
      if (!path) return false;
      r = await api('/api/avatars/import', { body: { path } });
    }
    if (how === 'zip') {
      const f = await pickFile('.zip,application/zip'); if (!f) return false;
      toast('Importing icons…', I.person);
      r = await api('/api/avatars/import-zip?name=' + encodeURIComponent(f.name), { body: f });
    }
    toast(`Imported ${r.count} icons in ${r.categories.length} categor${r.categories.length === 1 ? 'y' : 'ies'}`, I.check, 3500);
    return true;
  } catch (e) { if (e.message !== 'cancelled') alertDlg('Import failed', e.message); return false; }
}
function pickFile(accept) {
  return new Promise(resolve => {
    const inp = h('input', { type: 'file', accept, hidden: true });
    document.body.append(inp);
    inp.onchange = () => { resolve(inp.files[0] || null); inp.remove(); };
    inp.click();
  });
}


async function deleteProfile(id, layer) {
  const p = profileById(id);
  const typed = await dialog({ title: `Delete ${p.name}?`, text: `Type the name "${p.name}" to confirm. Their save data will be archived in NEXUS data\\Archived and can be restored manually.`, input: { placeholder: p.name }, buttons: [{ label: 'Cancel', value: null }, { label: 'Delete', value: true, danger: true }] });
  if (typed === null || typed === undefined) return;
  if (typed !== p.name) { alertDlg('Not deleted', 'The name did not match.'); return; }
  try {
    const r = await api(`/api/profiles/${id}`, { method: 'DELETE', body: { confirm: typed } });
    layer.close();
    await loadState();
    toast(`${p.name} was removed. Data archived.`, I.check);
    if (S.profile?.id === id) { S.profile = null; }
    void r;
    showWho();
  } catch (e) { alertDlg('Could not delete', e.message); }
}

async function patchProfile(id, patch) {
  const p = await api(`/api/profiles/${id}`, { method: 'PATCH', body: patch });
  const i = S.profiles.findIndex(x => x.id === id); if (i >= 0) S.profiles[i] = p;
  if (S.profile?.id === id) { S.profile = p; applyTheme(); }
  return p;
}

// ---------- top bar
function refreshTop() { renderTopbar(); }
function renderTopbar() {
  AccountUI.top();
  const left = $('#tb-profiles'); if (!left) return;
  left.innerHTML = '';
  const ordered = S.profile ? [S.profile, ...S.profiles.filter(p => p.id !== S.profile.id)] : S.profiles;
  ordered.slice(0, 8).forEach((p, i) => {
    const a = avatarEl(p, p.id === S.profile?.id ? 'active' : '');
    const b = h('button', { class: a.className, style: { '--c': a.style.getPropertyValue('--c'), '--i': i }, 'data-nav': true, title: p.name }, ...a.childNodes);
    if (p.theme?.accent) b.style.setProperty('--pc', p.theme.accent);
    b._profile = p;
    b.addEventListener('click', () => p.id === S.profile?.id ? profilePage(p.id) : switchProfile(p.id));
    left.append(b);
  });
  const sys = S.system || {};
  const pads = $('#tb-pads'); pads.innerHTML = ''; if (S.pads.length) pads.append(svg(I.pad), String(S.pads.length));
  const net = $('#tb-net'); net.innerHTML = ''; net.append(svg(sys.online === false ? I.nowifi : I.wifi));
  net.title = sys.online === false ? 'Offline' : 'Connected';
  const bat = $('#tb-bat'); bat.innerHTML = '';
  if (sys.battery?.present) {
    const pct = sys.battery.percent;
    const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg'); s.setAttribute('viewBox', '0 0 28 16');
    s.innerHTML = `<rect x="1" y="2" width="23" height="12" rx="2.5" fill="none" stroke="currentColor" stroke-width="1.6"/><rect x="25" y="5.5" width="2" height="5" rx="1" fill="currentColor"/><rect x="3" y="4" width="${19 * pct / 100}" height="8" rx="1.2" style="fill:${pct < 20 && !sys.battery.charging ? 'var(--danger)' : sys.battery.charging ? 'var(--ok)' : 'currentColor'}"/>`;
    s.style.width = '1.7em';
    bat.append(`${pct}%`, s);
  }
}
async function switchProfile(id) {
  if (S.playing) { toast('Close the running game before switching users.', I.warn); return; }
  await selectProfile(id);
  toast(`Welcome back, ${S.profile.name}`, I.person);
}

// the clock is written only when the minute changes (it used to be rewritten every second)
function tickClock() {
  const c = $('#tb-clock'); if (!c) return;
  const t = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (c.textContent !== t) c.textContent = t;
}
async function pollSystem() {
  if (gameInFront()) return; // refreshed when NEXUS shows again (syncGameInFront)
  try { S.system = await api('/api/system'); renderTopbar(); if (S.screen === 'home') markTopFocus(); } catch { }
}
function markTopFocus() { if (Nav.cur && Nav.cur.closest('#tb-profiles')) { const p = Nav.cur._profile; const b = $$('#tb-profiles button').find(x => x._profile?.id === p?.id); if (b) Nav.setFocus(b, { silent: true }); } }

// ---------- home
function gameTile(g, extraClass = '') {
  const t = h('button', { class: 'tile ' + extraClass, 'data-nav': true, role: 'listitem', title: g.title });
  const art = h('div', { class: 'art' });
  const ph = h('div', { class: 'ph' }, svg(I.pad, 'ph-ic'), h('div', { class: 'ph-title' }, g.title));
  art.append(ph);
  if (g.artSource) {
    const img = h('img', { src: `/art/${g.key}?v=${g.artRev || 0}&w=${Math.min(1024, Math.ceil(300 * (devicePixelRatio || 1) / 64) * 64)}`, alt: '', loading: 'lazy', decoding: 'async', class: g.artFit === 'contain' ? 'fit-contain' : '' });
    if (g.artFit === 'contain') art.style.backgroundImage = `url("/art/${g.key}?v=${g.artRev || 0}")`, art.classList.add('contain');
    img.addEventListener('load', () => ph.remove());
    img.addEventListener('error', () => img.remove());
    art.append(img);
  }
  t.append(art);
  if (S.stats[g.key]?.lastPlayed && S.recent.indexOf(g.key) > -1 && S.recent.indexOf(g.key) < 3) t.append(h('span', { class: 'recent-dot', title: 'Recently played' }));
  if (g.kind === 'homebrew') t.append(h('span', { class: 'badge' }, 'HOMEBREW'));
  // a game kept running in the background (NEXUS Home): subtle green outline and status light
  if (S.playing?.running && S.playing.gameKey === g.key) { t.classList.add('running'); t.append(h('span', { class: 'run-dot', title: 'Running' })); }
  if (g.kind === 'update') t.append(h('span', { class: 'badge' }, 'UPDATE'));
  if (g.kind === 'dlc') t.append(h('span', { class: 'badge' }, 'DLC'));
  if (g.perGame) t.append(h('span', { class: 'cfg-dot', title: 'Has its own game settings' }));
  t._game = g;
  t.addEventListener('contextmenu', e => { e.preventDefault(); if (!t.hasAttribute('data-nav') && !t.closest('[data-nav]')) return; gameMenu(g.key); });
  return t;
}

function renderHome() {
  renderTopbar();
  homeLayoutChrome(); // layout, page strip (web/home.js)
  const layout = homePrefs().layout;
  const row = $('#tiles'); row.innerHTML = '';
  if (layout === 'grid') delete row.dataset.row; else row.dataset.row = 'strict';
  const items = homeItems();
  const games = items.filter(x => x.g).map(x => x.g);
  if (!S.games.length) {
    const scanning = S.library?.scanning;
    const empty = h('div', { class: 'tile empty', 'data-nav': true },
      h('h3', {}, scanning ? 'Looking for games…' : 'No games yet'),
      h('p', {}, scanning ? 'NEXUS is scanning your game folders.' : `Put your own game dumps (NSP, XCI, NSZ, XCZ or NRO) in the Games folder, or add another folder in Settings › Game library.`),
      h('p', {}, h('b', {}, S.gamesDir || '')));
    empty.addEventListener('click', async () => {
      const which = await listPick('Add games', [{ v: 'open', l: 'Open the Games folder' }, { v: 'add', l: 'Add another folder…' }, { v: 'scan', l: 'Scan again' }]);
      if (which === 'open') api('/api/app/open-folder', { body: { which: 'games' } });
      if (which === 'add') addLibraryDir();
      if (which === 'scan') rescan();
    });
    row.append(empty);
  }
  items.forEach((it, i) => {
    if (it.folder) { const t = folderTile(it.folder); t.style.setProperty('--i', i); t._navMove = homeTileUp; row.append(t); return; }
    const g = it.g;
    const t = gameTile(g, homePrefs().favorites.includes(g.key) ? 'fav' : ''); t.style.setProperty('--i', i);
    t.addEventListener('click', () => launchGame(g.key));
    t.addEventListener('navfocus', () => { updateSelLabel(g); homeDetails(g); });
    t.addEventListener('navblur', () => updateSelLabel(null));
    t._navMove = homeTileUp;
    row.append(t);
  });
  if (S.games.length && !items.length) row.append(h('div', { class: 'tile empty' }, h('h3', {}, 'Nothing on this page'), h('p', {}, HomeState.page === 'all' ? 'Every game is hidden from Home or in a folder.' : 'Add games to it with their Options › Organise.')));
  if (S.games.length) {
    const all = h('button', { class: 'tile all', 'data-nav': true, style: { '--i': games.length } }, svg(I.grid, 'all-ic'), h('span', { class: 'all-n' }, `All Software (${S.games.filter(isPlayable).length})`));
    all.addEventListener('click', () => showLibrary());
    all.addEventListener('navfocus', () => { const l = $('#sel-label'); l.querySelector('.sel-title').textContent = 'All Software'; l.querySelector('.sel-meta').textContent = ''; });
    row.append(all);
  }
  // dock
  const dock = $('#dock'); dock.innerHTML = ''; dock.dataset.row = 'strict';
  const dockItems = homeDockOrder(homeDockItems()).map(x => [x.label, x.icon, x.color, x.run]);
  dockItems.forEach(([label, icon, col, fn], i) => {
    const b = h('button', { class: 'dock-btn', 'data-nav': true, style: { '--ic': col, '--i': i }, 'aria-label': label }, svg(icon), h('span', { class: 'tip' }, label));
    b.addEventListener('click', fn);
    b.addEventListener('navfocus', () => { $('#dock-label').textContent = label; });
    b.addEventListener('navblur', () => { $('#dock-label').textContent = ''; });
    b.addEventListener('mouseenter', () => { $('#dock-label').textContent = label; });
    b.addEventListener('mouseleave', () => { if (!b.classList.contains('focus')) $('#dock-label').textContent = ''; });
    dock.append(b);
  });
  renderBanner();
  homeWidgets();
  homeDetails(games.find(g => g.key === S.playing?.gameKey) || games[0] || null);
}

// The shortcut buttons (dock / sidebar), with ids for the user's order (web/home.js)
function homeDockItems() {
  return [
    { id: 'library', label: 'Library', icon: I.grid, color: '#ff4f4f', run: () => showLibrary() },
    { id: 'users', label: 'Users', icon: I.person, color: '#ff9f0a', run: () => showWho() },
    { id: 'account', label: 'NEXUS Account', icon: I.cloud, color: '#0a84ff', run: () => showSettings('account') },
    { id: 'media', label: 'Media', icon: I.media, color: '#ffd60a', run: () => Media.show() },
    { id: 'controllers', label: 'Controllers', icon: I.pad, color: '#30d158', run: () => Controllers.show() },
    { id: 'home', label: 'Home screen', icon: I.palette, color: '#bf5af2', run: () => showSettings('home') },
    { id: 'themes', label: 'Themes', icon: I.palette, color: '#ff6ac1', run: () => showSettings('appearance') },
    { id: 'shaders', label: 'Shaders', icon: I.chip, color: '#5e5ce6', run: () => showSettings('shaders') },
    { id: 'perf', label: 'Performance', icon: I.chip, color: '#ff9f0a', run: () => showSettings('perf') },
    { id: 'emulation', label: 'Graphics & Emulation', icon: I.chip, color: '#64d2ff', run: () => showSettings('emusettings') },
    { id: 'settings', label: 'System Settings', icon: I.gear, color: 'var(--muted)', run: () => showSettings() },
    { id: 'power', label: 'Power', icon: I.power, color: 'var(--muted)', run: () => openPower() },
  ];
}

// Up from a Home tile goes to the banner's buttons when it has any (Resume / Close game while a
// game runs in the background), so a controller always reaches them; otherwise normal movement.
function homeTileUp(dir) {
  // Cards and Grid: Down from the lowest games goes to the selected game's panel (Play), not to
  // the shortcut bar at the side (web/home.js)
  if (dir === 'down' && homePrefs().layout !== 'classic') {
    const cur = Nav.cur, r = cur?.getBoundingClientRect();
    const below = r && $$('#tiles .tile').some(t => t !== cur && t.getBoundingClientRect().top > r.bottom - 4);
    const play = $('#home-details .hd-play');
    if (!below && play && play.offsetParent) { Nav.setFocus(play); return true; }
    return false;
  }
  if (dir !== 'up') return false;
  const b = $('#home-banner');
  if (b.hidden) return false;
  const btns = Nav.items(b);
  if (!btns.length) return false;
  Nav.setFocus(btns.find(x => x.classList.contains('primary')) || btns[0]);
  return true;
}

function renderBanner() {
  const b = $('#home-banner');
  const hadFocus = Nav.cur && b.contains(Nav.cur);
  renderBannerContent(b);
  // the banner changed under the focus (the game closed, or resumed elsewhere): keep focus on Home
  if (hadFocus && !(Nav.cur && b.contains(Nav.cur) && Nav.cur.isConnected)) {
    const t = $$('#tiles .tile').find(t => t._game?.key === S.playing?.gameKey) || $('#tiles .tile');
    if (t && !Nav.top()?.dialog) Nav.setFocus(t, { silent: true });
  }
}

function renderBannerContent(b) {
  b.innerHTML = '';
  const en = S.engine || {};
  let msg = null, action = null, label = '';
  if (S.playing?.suspended) {
    b.hidden = false; b.innerHTML = '';
    const bg = !!S.playing.background;
    const ic = bg ? h('i', { class: 'bg-run-dot', title: 'Running in the background' }) : svg(I.play);
    if (!bg) ic.style.cssText = 'width:1.4em;height:1.4em;fill:var(--accent);flex:none';
    const resume = h('button', { class: 'btn primary', 'data-nav': true }, 'Resume');
    const close = h('button', { class: 'btn', 'data-nav': true }, 'Close game');
    resume.addEventListener('click', () => resumeGame());
    close.addEventListener('click', () => closeRunningGame());
    b.append(ic, h('span', { class: 'grow' }, h('b', {}, bg ? `${S.playing.title} is running in the background. ` : `${S.playing.title} is paused. `), bg ? 'It is muted and ignores controls until you resume it.' : 'It keeps its place until you resume it.'), resume, close);
    return;
  }
  if (en.job?.active) { msg = h('span', { class: 'grow' }, h('b', {}, 'Installing the NEXUS engine… '), en.job.message || ''); }
  else if (en.checking) { /* the engine is being verified and loaded in the background */ }
  else if (!en.installed) { msg = h('span', { class: 'grow' }, h('b', {}, 'The NEXUS engine cannot be used. '), en.problem || 'Extract the complete NEXUS ZIP again.'); label = 'Details'; action = () => showSettings('engine'); }
  else if (S.games.some(g => g.kind !== 'homebrew') && !en.keys) { msg = h('span', { class: 'grow' }, h('b', {}, 'System files needed. '), 'Install the prod.keys and firmware from your own console to play commercial games.'); label = 'Install'; action = () => showSettings('sysfiles'); }
  if (!msg && AccountUI.onboard(b)) return;
  if (!msg) { b.hidden = true; return; }
  b.hidden = false;
  b.append(svg(I.info), msg);
  b.querySelector('svg').style.cssText = 'width:1.4em;height:1.4em;fill:var(--accent);flex:none';
  if (action) { const btn = h('button', { class: 'btn primary', 'data-nav': true }, label); btn.addEventListener('click', action); b.append(btn); }
}

function updateSelLabel(g) {
  const l = $('#sel-label');
  if (!g) { l.querySelector('.sel-title').textContent = ''; l.querySelector('.sel-meta').textContent = ''; return; }
  l.querySelector('.sel-title').textContent = g.title;
  const st = S.stats[g.key];
  const bits = [];
  if (st?.seconds) bits.push(fmtPlay(st.seconds));
  if (st?.lastPlayed) bits.push('Last played ' + fmtAgo(st.lastPlayed));
  l.querySelector('.sel-meta').textContent = bits.join(' · ');
}

// The menu a right-click opens on a Home tile, for the focused element (a game, a folder, or the
// game shown in the Cards / Grid details panel); null when there is none.
function homeContextMenu(cur) {
  if (S.screen !== 'home' || Nav.top()?.dialog || !cur) return null;
  if (cur._game) return () => gameMenu(cur._game.key);
  if (cur._folder && typeof folderMenu === 'function') return () => folderMenu(cur._folder.id);
  if (cur._menuGame) return () => gameMenu(cur._menuGame);
  return null;
}
Screens.home = {
  onBack: () => openPower(),
  onTab: d => homeTab(d), // pages (web/home.js)
  onOptions: cur => { const m = homeContextMenu(cur); if (m) m(); else if (cur?._profile) editProfile(cur._profile.id); },
};

// ---------- closing the running game (Home banner, the game's Options menu, the Power menu)
// One flow for every place, mouse or controller. Cancel is focused first, so a stray A press
// does not close the game. The engine ends the session (sound, graphics, emulation); the tile's
// running light and the banner follow the emulator.state event.
async function closeRunningGame() {
  const p = S.playing; if (!p?.running) return false;
  const ok = await confirmDlg(`Close ${p.title}?`, 'Unsaved progress will be lost. The game stops completely; NEXUS stays on Home.', 'Close game', true);
  if (!ok || !S.playing?.running) return false;
  try { await api('/api/emulator/stop', { body: {} }); }
  catch (e) { toast(e.message, I.warn); return false; }
  toast(`Closing ${p.title}…`, I.power);
  return true;
}

// ---------- power menu
async function openPower() {
  if (Nav.top()?.name === 'power') { Nav.top().close(); return; }
  const fs = S.system.fullscreen ?? S.settings.display?.fullscreen;
  const items = [
    S.playing?.running ? { v: 'resume', l: `Resume ${S.playing.title}` } : null,
    S.playing?.running ? { v: 'close', l: `Close ${S.playing.title}` } : null,
    { v: 'users', l: 'Switch user' },
    { v: 'fs', l: fs ? 'Switch to windowed mode' : 'Switch to full screen' },
    { v: 'min', l: 'Minimise NEXUS' },
    { v: 'replay', l: 'Replay startup animation' },
    { v: 'quit', l: 'Exit NEXUS' },
  ].filter(Boolean);
  const v = await listPick('Power options', items);
  if (v === 'resume') resumeGame();
  if (v === 'close') closeRunningGame();
  if (v === 'users') showWho();
  if (v === 'fs') { const r = await api('/api/app/fullscreen', { body: { on: !fs } }); S.system.fullscreen = r.fullscreen; S.settings.display.fullscreen = !fs; }
  if (v === 'min') api('/api/app/minimize', { body: {} });
  if (v === 'replay') setTimeout(() => Boot.preview(), 200);
  if (v === 'quit') {
    if (S.playing) { const ok = await confirmDlg('A game is running', 'Exit NEXUS anyway? The game will keep running.', 'Exit'); if (!ok) return; }
    document.body.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 300, fill: 'both' });
    await sleep(250); api('/api/app/quit', { body: {} });
  }
}

// ---------- library
function libList() {
  const q = S.libQuery.trim().toLowerCase();
  let list = S.games.filter(g => (S.libExtras || isPlayable(g)) && (S.libHidden ? g.hidden : !g.hidden));
  if (q) list = list.filter(g => g.title.toLowerCase().includes(q) || g.fileTitle.toLowerCase().includes(q) || (g.titleId || '').toLowerCase().includes(q));
  const sort = S.libSort || S.profile?.sort || 'recent';
  if (sort === 'alpha') list.sort((a, b) => a.title.localeCompare(b.title));
  else if (sort === 'playtime') list.sort((a, b) => (S.stats[b.key]?.seconds || 0) - (S.stats[a.key]?.seconds || 0) || a.title.localeCompare(b.title));
  else list.sort((a, b) => (S.stats[b.key]?.lastPlayed || 0) - (S.stats[a.key]?.lastPlayed || 0) || a.title.localeCompare(b.title));
  return list;
}
const SORTS = [{ v: 'recent', l: 'Recently played' }, { v: 'alpha', l: 'Title A–Z' }, { v: 'playtime', l: 'Play time' }];
function renderLibrary(keepKey) {
  const grid = $('#lib-grid'); grid.innerHTML = '';
  const sort = S.libSort || S.profile?.sort || 'recent';
  $('#lib-sort').textContent = 'Sort: ' + SORTS.find(s => s.v === sort).l;
  const extras = S.games.filter(g => !isPlayable(g)).length;
  $('#lib-extras').textContent = S.libExtras ? `Hide updates & DLC` : `Show updates & DLC (${extras})`;
  $('#lib-extras').hidden = !extras;
  const hiddenN = S.games.filter(g => g.hidden).length;
  $('#lib-hidden').hidden = !hiddenN && !S.libHidden;
  $('#lib-hidden').textContent = S.libHidden ? 'Back to library' : `Removed (${hiddenN})`;
  $('#lib-hidden').classList.toggle('on', !!S.libHidden);
  const list = libList();
  const st = S.library || {};
  $('#lib-status').textContent = st.scanning ? 'Scanning…' : `${list.length} title${list.length === 1 ? '' : 's'}${S.libQuery ? ` matching “${S.libQuery}”` : ''} · Folders: ${(S.scanDirs || []).length}`;
  if (!list.length) grid.append(h('div', { class: 'lib-empty' }, S.libQuery ? 'No games match your search.' : 'No games found. Put game dumps in the Games folder or add a folder in Settings › Game library.'));
  let keep = null;
  for (const g of list) {
    const st2 = S.stats[g.key];
    const it = h('button', { class: 'lib-item', 'data-nav': true }, gameTile(g), h('div', { class: 'li-title' }, g.title),
      h('div', { class: 'li-meta' }, st2?.seconds ? fmtPlay(st2.seconds).replace('Played for ', '') : (st2?.lastPlayed ? 'Played ' + fmtAgo(st2.lastPlayed) : g.format.toUpperCase())));
    it.querySelector('.tile').removeAttribute('data-nav');
    it._game = g;
    it.addEventListener('click', () => launchGame(g.key));
    grid.append(it);
    if (g.key === keepKey) keep = it;
  }
  return keep;
}
function showLibrary(focusSearch = false) {
  S.libQuery = ''; $('#lib-search').value = '';
  const first = renderLibrary();
  show('library', { initial: focusSearch ? $('#lib-search') : ($('#lib-grid .lib-item') || $('#lib-search')) });
  if (focusSearch) $('#lib-search').focus();
  void first;
}
Screens.library = {
  onBack: () => goHome(),
  onOptions: cur => { if (cur?._game) gameMenu(cur._game.key); },
  onTab: d => { const i = SORTS.findIndex(s => s.v === (S.libSort || S.profile?.sort || 'recent')); S.libSort = SORTS[(i + d + 3) % 3].v; renderLibrary(); },
};
$('#lib-search').addEventListener('input', e => { S.libQuery = e.target.value; renderLibrary(); });
$('#lib-search').addEventListener('submit', () => { const f = $('#lib-grid .lib-item'); if (f) Nav.setFocus(f); });
$('#lib-sort').addEventListener('click', async () => {
  const v = await listPick('Sort by', SORTS, S.libSort || S.profile?.sort || 'recent');
  if (v) { S.libSort = v; if (S.profile) patchProfile(S.profile.id, { sort: v }).catch(() => { }); renderLibrary(); }
});
$('#lib-extras').addEventListener('click', () => { S.libExtras = !S.libExtras; renderLibrary(); });
$('#lib-hidden').addEventListener('click', () => { S.libHidden = !S.libHidden; renderLibrary(); });
$('#lib-refresh').addEventListener('click', () => rescan());

async function rescan() { try { await api('/api/library/scan', { body: {} }); toast('Scanning game folders…', I.grid); } catch (e) { toast(e.message, I.warn); } }

// ---------- game details sheet
// ---------- shader preparation on Home ("shaders.prepare" events, shaderprep.go)
const PREP_WORDS = { 'Reading game files': 'reading the game’s files', 'Extracting shaders': 'finding shaders', Converting: 'converting', Writing: 'saving' };
const prepEta = new ShaderEta();
function prepLabel(p) {
  const w = PREP_WORDS[p?.stage] || 'starting';
  if (!p?.total) return w;
  const r = p.eta;
  return `${w} ${p.done} of ${p.total}` + (r && (r.state === 'estimate' || r.state === 'stalled') ? ` \u00b7 ${r.text}` : '');
}
function onShaderPrep(p) {
  if (p.state === 'running') {
    const eta = p.total ? prepEta.add(p.gameKey + '|' + p.stage, p.done || 0, Math.max(0, p.total - (p.done || 0))) : null;
    S.prep = { key: p.gameKey, stage: p.stage, done: p.done, total: p.total, eta };
    return;
  }
  S.prep = null;
  const n = v => Number(v || 0).toLocaleString();
  if (p.state === 'done') toast(`${p.title}: ${n(p.programs)} shader programs found and prepared. They are compiled the next time the game starts.`, I.check, 7000);
  else if (p.state === 'current') toast(`${p.title}: shaders are already prepared.`, I.check);
  else if (p.state === 'stopped') toast(`${p.title}: shader preparation stopped.`, I.chip);
  else if (p.state === 'failed') toast(`${p.title}: shaders could not be prepared. ${p.error || ''}`, I.warn, 8000);
}

// ---------- portable shader data between your own devices (shadercache.go, docs/SHADERS.md)
async function shaderExport(g) {
  try {
    const r = await fetch(`/api/games/${encodeURIComponent(g.key)}/shaders/export`, { headers: { 'X-Nexus': '1' } });
    if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || `Export failed (${r.status})`); }
    const blob = await r.blob();
    const name = (r.headers.get('Content-Disposition') || '').match(/filename="?([^";]+)"?/)?.[1] || `${g.title}.nxshaders`;
    const a = h('a', { href: URL.createObjectURL(blob), download: decodeURIComponent(name) }); document.body.append(a); a.click(); a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 10000);
    alertDlg('Shader data exported', `${g.title}'s shader data was saved to your Downloads folder (${(blob.size / 1048576).toFixed(1)} MB). On your other device, open the game's Options and choose Import Shader Data. It holds the game's shader code, so keep it to your own devices; graphics-card-specific data is not included.`);
  } catch (e) { alertDlg('Export failed', e.message); }
}
async function shaderImport(g) {
  const f = await pickFile('.nxshaders'); if (!f) return;
  try {
    toast('Importing shader data…', I.chip);
    const r = await api(`/api/games/${encodeURIComponent(g.key)}/shaders/import`, { body: f });
    if (!r.added) toast(`${g.title}: nothing new: this device already has all of it (${r.present} entries).`, I.check, 6000);
    else toast(`${g.title}: ${r.added.toLocaleString()} shader entries added; they are built before you play.${r.present ? ` ${r.present.toLocaleString()} were already here and were kept.` : ''}`, I.check, 8000);
  } catch (e) { alertDlg('Import failed', e.message); }
}

// ---------- game options menu
// Grouped so it stays short: playing, the game itself, shaders, then the library actions under
// "More". Every action of earlier versions is still here (some one level down, under More).
function gameMenu(key, view = 'main') {
  const g = gameByKey(key); if (!g) return;
  if (Nav.top()?.name === 'gamemenu') Nav.top().close();
  const running = S.playing?.running && S.playing.gameKey === key;
  const idle = isPlayable(g) && !S.playing?.running;
  const sections = view === 'more' ? [
    [null, [
      ['back', 'Back', I.back],
      ['art', 'Edit Game Artwork', I.palette],
      ['refresh', 'Refresh Artwork & Information', I.refresh],
      ['open', 'Open Game Location', I.folder],
      g.hidden ? ['restore', 'Restore to Library', I.plus] : ['remove', 'Remove from Library', I.trash],
    ]],
  ] : [
    [null, [
      running ? ['resume', 'Resume', I.play] : null,
      running ? ['close', 'Close Game', I.power] : null,
      isPlayable(g) && !g.hidden && !running ? ['play', 'Play', I.play] : null,
    ]],
    ['Game', [
      isPlayable(g) ? ['config', 'Game Settings', I.gear, 'Graphics, controls, updates and per-game options'] : null,
      isPlayable(g) && g.titleId && g.kind !== 'homebrew' ? ['mods', 'Manage Mods', I.puzzle, modsSummary(g)] : null,
      ['organise', 'Organise', I.grid, homePrefs().favorites.includes(key) ? 'Favourite · folders, pages, order, hide from Home' : 'Favourites, folders, pages, order, hide from Home'],
      ['saves', 'Save Data', I.folder, 'View, back up, import or restore saves'],
      ['info', 'Game Information', I.info],
    ]],
    isPlayable(g) ? ['Shaders', [
      idle ? ['shaders', S.prep?.key === key ? 'Stop Preparing Shaders' : 'Prepare Shaders Now', I.chip, S.prep?.key === key ? prepLabel(S.prep) : 'Find and convert this game’s shaders before you play'] : null,
      idle ? ['shexport', 'Export Shader Data', I.download || I.folder, 'For your other devices'] : null,
      idle ? ['shimport', 'Import Shader Data…', I.plus, 'From your other devices (.nxshaders)'] : null,
      !idle ? ['', 'Close the game to manage its shaders', I.chip] : null,
    ]] : null,
    [null, [['more', 'More…', I.more, 'Artwork, game location, remove from library']]],
  ];
  const tile = gameTile(g); tile.removeAttribute('data-nav');
  const btns = [], list = h('div', { class: 'gm-list' });
  for (const sec of sections.filter(Boolean)) {
    const items = sec[1].filter(Boolean); if (!items.length) continue;
    if (sec[0]) list.append(h('div', { class: 'gm-sec', role: 'presentation' }, sec[0]));
    for (const [v, l, ic, sub] of items) {
      const b = h('button', { 'data-nav': v ? true : null, disabled: v ? null : '', class: v === 'remove' || v === 'close' ? 'danger' : '' }, svg(ic), h('span', { class: 'gm-l' }, l, sub ? h('small', {}, sub) : ''));
      b._v = v; btns.push(b); list.append(b);
    }
  }
  const box = h('div', { class: 'dialog game-menu' }, h('div', { class: 'gm-head' }, tile, h('div', {}, h('b', {}, g.title), h('small', {}, [g.titleId, g.format.toUpperCase(), g.perGame ? 'Custom settings' : ''].filter(Boolean).join(' · ')))), list);
  const layer = openLayer(box, { name: 'gamemenu', initial: btns.find(b => b._v), onBack: view === 'more' ? () => { layer.close(); gameMenu(key); } : undefined });
  btns.forEach(b => b._v && b.addEventListener('click', async () => {
    layer.close();
    switch (b._v) {
      case 'play': launchGame(key); break;
      case 'resume': resumeGame(); break;
      case 'close': closeRunningGame(); break;
      case 'config': gameConfig(key); break;
      case 'mods': ModsUI.open(key); break;
      case 'organise': homeOrganise(key); break;
      case 'saves': saveDataPage(S.profile?.id); break;
      case 'more': gameMenu(key, 'more'); break;
      case 'back': gameMenu(key); break;
      case 'shexport': shaderExport(g); break;
      case 'shimport': shaderImport(g); break;
      case 'shaders':
        if (S.prep?.key === key) { api('/api/shaders/prepare/cancel', { body: {} }).catch(() => { }); break; }
        api(`/api/games/${encodeURIComponent(key)}/prepare-shaders`, { body: {} })
          .then(() => toast(`Preparing shaders for ${g.title} in the background`, I.chip))
          .catch(e => toast(e.message, I.warn));
        break;
      case 'art': artEditor(key); break;
      case 'info': gameInfo(key); break;
      case 'open': api(`/api/games/${key}/reveal`, { body: {} }).catch(e => toast(e.message, I.warn)); break;
      case 'refresh': await api(`/api/games/${key}/refresh`, { body: {} }); toast('Refreshing game information and artwork…', I.refresh); break;
      case 'remove': if (await confirmDlg(`Remove ${g.title}?`, 'It will be hidden from your library. The game file is not deleted; restore it any time from All Software › Removed.', 'Remove', true)) { await api(`/api/games/${key}/hidden`, { body: { hidden: true } }); await loadGames(); refreshViews(); toast('Removed from library', I.check); } break;
      case 'restore': await api(`/api/games/${key}/hidden`, { body: { hidden: false } }); await loadGames(); refreshViews(); toast('Restored to library', I.check); break;
    }
  }));
}
// modsSummary: "2 of 3 on" from the library entry (no request while the menu opens)
function modsSummary(g) {
  const m = g.mods; if (!m || !m.total) return 'Install and switch mods for this game';
  return `${m.enabled} of ${m.total} on`;
}

function gameInfo(key) {
  sheet('Game information', (body, layer, rebuild) => {
    const g = gameByKey(key); if (!g) { layer.close(); return; }
    const st = S.stats[g.key] || {};
    const tile = gameTile(g); tile.removeAttribute('data-nav');
    body.append(h('div', { class: 'game-hero' }, tile, h('div', { class: 'gh-info' }, h('b', {}, g.title),
      h('span', {}, st.seconds ? fmtPlay(st.seconds) : 'Not played yet'),
      st.lastPlayed ? h('span', {}, 'Last played ' + fmtAgo(st.lastPlayed)) : '',
      h('span', {}, 'Runs in the NEXUS engine'))));
    body.append(gameStatsBlock(st));
    body.append(rowButton('Correct play time\u2026', { sub: 'If NEXUS shows less (or more) time than you played.', onClick: async () => { if (await correctPlaytime(g.key, g.title, st.seconds)) { rebuild(); } } }));
    if (isPlayable(g) && !g.hidden) body.append(rowButton('Play', { icon: I.play, onClick: () => { layer.close(); launchGame(g.key); } }));
    body.append(rowButton('Options…', { onClick: () => { layer.close(); gameMenu(key); } }));
    body.append(rowButton('Rename', { value: g.title, onClick: async () => {
      const n = await dialog({ title: 'Display name', text: 'Leave empty to use the detected name.', input: { value: g.title, max: 80 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n === null || n === undefined) return;
      const ng = await api(`/api/games/${g.key}/title`, { body: { title: n } }); Object.assign(g, ng); rebuild(); refreshViews();
    } }));
    const kind = { base: 'Game', update: 'Update', dlc: 'DLC', homebrew: 'Homebrew', unknown: 'Unknown' }[g.kind] || g.kind;
    body.append(h('dl', { class: 'kv' },
      h('dt', {}, 'Type'), h('dd', {}, kind),
      h('dt', {}, 'Format'), h('dd', {}, `${g.format.toUpperCase()} · ${fmtBytes(g.size)}`),
      g.titleId ? [h('dt', {}, 'Title ID'), h('dd', {}, g.fullId && g.fullId !== g.titleId ? `${g.fullId} (base ${g.titleId})` : g.titleId)] : '',
      g.version ? [h('dt', {}, 'Version'), h('dd', {}, g.version)] : '',
      h('dt', {}, 'Identified by'), h('dd', {}, { nca: 'Game header (prod.keys)', ticket: 'Ticket', filename: 'File name', nro: 'Homebrew metadata', '': 'File name only' }[g.idSource || ''] || g.idSource),
      g.embedName ? [h('dt', {}, 'Name in game'), h('dd', {}, g.embedName)] : '',
      g.publisher ? [h('dt', {}, 'Publisher'), h('dd', {}, g.publisher)] : '',
      g.released ? [h('dt', {}, 'Released'), h('dd', {}, /^\d{8}$/.test(g.released) ? `${g.released.slice(0, 4)}-${g.released.slice(4, 6)}-${g.released.slice(6)}` : g.released)] : '',
      g.builtinArt ? [h('dt', {}, 'Built-in artwork'), h('dd', {}, `Matched by ${g.builtinWhy}`)] : '',
      g.onlineName ? [h('dt', {}, 'Name online'), h('dd', {}, g.onlineName)] : '',
      h('dt', {}, 'Artwork'), h('dd', {}, artNote(g)),
      h('dt', {}, 'Compatibility'), h('dd', {}, compatText(S.compat?.[g.key])),
      h('dt', {}, 'Game settings'), h('dd', {}, g.perGame ? `${g.perGame} setting(s) override the global settings` : 'Uses global settings'),
      h('dt', {}, 'File'), h('dd', {}, g.path)));
    if (g.titleId && isPlayable(g)) {
      const sh = h('dl', { class: 'kv' }); body.append(sh);
      api(`/api/games/${encodeURIComponent(g.key)}/shaders/cache`).then(r => {
        const c = r.cache || {};
        const where = c.location === 'shared' ? 'shared with every NEXUS user of this PC' : 'yours only';
        sh.append(h('dt', {}, 'Shaders'), h('dd', {}, r.have && (c.graphics + c.compute) > 0
          ? `${(c.graphics + c.compute).toLocaleString()} ready from earlier play (${where})${c.precache ? (c.doneHere ? '; prepared for your graphics card' : '; prepared shaders available, your graphics card prepares them at its next start') : ''}`
          : 'None yet: they are built as the game is played and kept for next time'));
      }).catch(() => { });
    }
    if (g.note) body.append(h('p', { class: 'note warn' }, g.note));
    if (g.onlineError && g.onlineError !== 'removed by user') body.append(h('p', { class: 'note' }, 'Online artwork: ' + g.onlineError));
  });
}

function fmtDur(s) { s = Math.round(s || 0); const hh = Math.floor(s / 3600), mm = Math.floor(s % 3600 / 60); return hh ? `${hh} h ${mm} min` : mm ? `${mm} min` : `${s % 60} s`; }
function fmtDate(ts) { return ts ? new Date(ts * 1000).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }) : '—'; }
function gameStatsBlock(st) {
  const sessions = st.sessions || (st.seconds ? 1 : 0);
  const box = h('div', { class: 'stat-cards' },
    statCard('Play time', st.seconds ? fmtDur(st.seconds) : '0 min'),
    statCard('Sessions', String(sessions)),
    statCard('Last played', st.lastPlayed ? fmtAgo(st.lastPlayed) : 'Never'),
    statCard('Average session', sessions ? fmtDur((st.seconds || 0) / sessions) : '—'));
  const wrap = h('div', {}, box);
  const ls = st.lastSession;
  if (ls) wrap.append(h('p', { class: 'note' }, `Most recent session: ${fmtDate(ls.start)} – ${new Date(ls.end * 1000).toLocaleTimeString([], { timeStyle: 'short' })}, ${fmtDur(ls.seconds)}${ls.emulator && ls.emulator !== 'nexus' ? ' (played with an earlier NEXUS version)' : ''}` +
    (ls.recovered ? ' (recovered after NEXUS closed unexpectedly)' : '') + (ls.crashed ? ' (the game stopped with an error)' : '') + '.'));
  const by = Object.entries(st.byEmulator || {}).filter(([, v]) => v > 0);
  const before = by.filter(([k]) => k !== 'nexus').reduce((a, [, v]) => a + v, 0);
  if (before > 0) wrap.append(h('p', { class: 'note' }, `Includes ${fmtDur(before)} played with earlier NEXUS versions.`));
  if (st.firstPlayed) wrap.append(h('p', { class: 'note' }, 'First played ' + fmtDate(st.firstPlayed) + '.'));
  return wrap;
}
function statCard(label, value, sub) { return h('div', { class: 'stat-card' }, h('small', {}, label), h('b', {}, value), sub ? h('span', {}, sub) : ''); }

// ---------- artwork editor
function artEditor(key) {
  sheet('Edit game artwork', (body, layer, rebuild) => {
    const g = gameByKey(key); if (!g) { layer.close(); return; }
    const big = gameTile(g, 'art-preview'); big.removeAttribute('data-nav');
    body.append(h('div', { class: 'art-top' }, big, h('div', { class: 'gh-info' }, h('b', {}, g.title), h('span', {}, artNote(g)))));
    body.append(h('h3', {}, 'Sources'));
    const srcs = h('div', { class: 'art-sources', 'data-row': 'strict' });
    const opt = (pref, label, has, url) => {
      const b = h('button', { class: 'art-src' + (g.artSource === pref ? ' on' : '') + (has ? '' : ' empty'), 'data-nav': has || undefined },
        has ? h('img', { src: url, alt: '' }) : h('span', {}, 'None'), h('small', {}, label));
      if (has) b.addEventListener('click', async () => { const ng = await api(`/api/games/${key}/art-options`, { body: { preference: pref } }); Object.assign(g, ng); rebuild(); refreshViews(); });
      srcs.append(b);
    };
    const v = `v=${Date.now()}`;
    if (g.builtinArt) opt('builtin', 'NEXUS database', true, `/art/${key}?src=builtin&${v}`);
    opt('embedded', 'From game file', g.hasEmbedded, `/art/${key}?src=embedded&${v}`);
    opt('online', 'Downloaded', g.hasOnline, `/art/${key}?src=online&${v}`);
    opt('custom', 'Your image', g.hasCustom, `/art/${key}?src=custom&${v}`);
    body.append(srcs);
    body.append(rowButton('Choose an image from your PC…', { sub: 'PNG, JPG or WEBP. 512 × 512 or larger is ideal; the original file is kept at full quality.', onClick: async () => {
      const f = await pickImage(); if (!f) return;
      try { const ng = await api(`/api/games/${key}/art`, { body: f }); Object.assign(g, ng); rebuild(); refreshViews(); toast('Artwork updated', I.check); } catch (e) { alertDlg('Could not use that image', e.message); }
    } }));
    body.append(rowChoice('Image fit', [{ v: 'cover', l: 'Fill the tile' }, { v: 'contain', l: 'Show whole image' }], g.artFit || 'cover', async val => { const ng = await api(`/api/games/${key}/art-options`, { body: { fit: val } }); Object.assign(g, ng); rebuild(); refreshViews(); }, 'Non-square images keep their aspect ratio either way.'));
    if (g.hasEmbedded) body.append(rowButton('Reset to the original game icon', { onClick: async () => { const ng = await api(`/api/games/${key}/art-options`, { body: { preference: 'embedded' } }); Object.assign(g, ng); rebuild(); refreshViews(); } }));
    if (g.hasOnline) body.append(rowButton('Reset to downloaded artwork', { onClick: async () => { const ng = await api(`/api/games/${key}/art-options`, { body: { preference: 'online' } }); Object.assign(g, ng); rebuild(); refreshViews(); } }));
    body.append(rowButton(g.hasOnline ? 'Download artwork again' : 'Search online artwork', { sub: g.titleId ? `Looks up Title ID ${g.titleId} on ${hostName(S.settings.library?.nlibUrl) || 'the configured provider'}.` : 'Needs a Title ID (add prod.keys or [TitleID] in the file name).', disabled: !g.titleId || g.kind === 'homebrew', onClick: async () => { await api(`/api/games/${key}/refresh`, { body: {} }); toast('Searching for artwork…', I.refresh); } }));
    if (g.hasCustom) body.append(rowButton('Remove my image', { danger: true, onClick: async () => { const ng = await api(`/api/games/${key}/art`, { method: 'DELETE' }); Object.assign(g, ng); rebuild(); refreshViews(); } }));
    if (g.hasOnline) body.append(rowButton('Remove downloaded artwork', { danger: true, sub: 'NEXUS will not download it again unless you ask.', onClick: async () => { const ng = await api(`/api/games/${key}/online-art`, { method: 'DELETE' }); Object.assign(g, ng); rebuild(); refreshViews(); } }));
    if (g.onlineError && g.onlineError !== 'removed by user') body.append(h('p', { class: 'note' }, 'Last online lookup: ' + g.onlineError));
    // the Cards home layout's background and title pictures (web/home.js)
    body.append(h('h3', {}, 'Cards layout pictures'));
    const extra = (kind, label, sub, rev) => {
      body.append(rowButton(rev ? `Change the ${label}…` : `Choose a ${label}…`, { sub, onClick: async () => {
        const f = await pickImage(); if (!f) return;
        try { const ng = await api(`/api/games/${key}/art/${kind}`, { body: f }); Object.assign(g, ng); rebuild(); refreshViews(); toast('Picture saved', I.check); } catch (e) { alertDlg('Could not use that image', e.message); }
      } }));
      if (rev) body.append(rowButton(`Remove the ${label}`, { danger: true, onClick: async () => { const ng = await api(`/api/games/${key}/art/${kind}`, { method: 'DELETE' }); Object.assign(g, ng); rebuild(); refreshViews(); } }));
    };
    extra('background', 'background picture', 'Shown behind the cards while this game is selected (wide images, 1920 × 1080 ideal).', g.bgRev);
    extra('logo', 'title picture', 'The game’s logo over its background (a PNG with transparency is best).', g.logoRev);
    body.append(rowButton('Display name', { value: g.title, sub: 'The name NEXUS shows for this game. Empty: the detected name.', onClick: async () => {
      const n = await dialog({ title: 'Display name', text: 'Leave empty to use the detected name.', input: { value: g.title, max: 80 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n === null || n === undefined) return;
      const ng = await api(`/api/games/${key}/title`, { body: { title: n } }); Object.assign(g, ng); rebuild(); refreshViews();
    } }));
    body.append(h('p', { class: 'note' }, 'Artwork and names are stored in NEXUS data\\GameProfiles\\' + (g.titleId || 'file-…') + ' and are the same for every user of this NEXUS. Game files are never modified.'));
  });
}
function hostName(u) { try { return new URL(u).host; } catch { return ''; } }

// ---------- per-game settings
// Compatibility: what happened when this game ran in the NEXUS engine on this PC, or your rating.
const COMPAT_TEXT = { works: 'Works', issues: 'Known issues', unsupported: 'Unsupported', boots: 'Starts', untested: 'Not tested' };
function emuName(id) { return id === 'nexus' || !id ? 'NEXUS Engine' : 'an earlier NEXUS version'; }
function compatText(c) { return c ? (COMPAT_TEXT[c.status] || c.status) + (c.source === 'user' ? ' (your rating)' : '') : COMPAT_TEXT.untested; }
function compatRow(c, g, onRated) {
  c = c || { status: 'untested', why: 'Not started in NEXUS yet.', source: 'none' };
  const sys = { switch1: 'Nintendo Switch program', switch2: 'Nintendo Switch 2 program (experimental engine support)' }[c.system];
  const facts = [c.sessions ? `${c.sessions} session(s), reached gameplay in ${c.booted}, ${c.crashes} crash(es), ${fmtDur(c.seconds)}` : '', sys || '', c.lastCode ? `last error: ${c.lastCode}` : ''].filter(Boolean).join(' · ');
  const box = h('div', {}, h('div', { class: 'compat' }, h('span', { class: 'compat-badge c-' + c.status }, COMPAT_TEXT[c.status] || c.status),
    h('span', {}, c.why || ''), facts ? h('small', {}, facts) : ''));
  if (g) box.append(rowChoice('Your rating', [{ v: '', l: 'Automatic (from your sessions)' }, { v: 'works', l: 'Works' }, { v: 'issues', l: 'Known issues' }, { v: 'unsupported', l: 'Unsupported' }],
    c.source === 'user' ? c.status : '', async v => {
      try { const r = await api(`/api/games/${g.key}/compat`, { body: { rating: v } }); S.compat = S.compat || {}; S.compat[g.key] = r; onRated && onRated(r); } catch (e) { toast(e.message, I.warn); }
    }, 'NEXUS rates a game from what happens when you play it here. Your rating replaces the automatic one.'));
  return box;
}
// ---------- NEXUS configuration (global + per game), engine-independent
function nlabel(st, v) {
  if (v === undefined || v === null || v === '') return '—';
  if (st.kind === 'bool') return v === 'true' ? 'On' : 'Off';
  if (st.kind === 'enum') return (st.choices || []).find(c => c.v === v)?.l || v;
  return v + (st.suffix || '');
}
function nchoices(st, only) {
  let out = [];
  if (st.kind === 'bool') out = [{ v: 'true', l: 'On' }, { v: 'false', l: 'Off' }];
  else if (st.kind === 'enum') out = (st.choices || []).map(c => ({ v: c.v, l: c.l }));
  else if (st.kind === 'int' && st.presets?.length) out = st.presets.map(x => ({ v: String(x), l: x + (st.suffix || '') }));
  else if (st.kind === 'int') for (let x = st.min; x <= st.max; x += st.step) out.push({ v: String(x), l: x + (st.suffix || '') });
  if (only?.length) out = out.filter(c => only.includes(c.v));
  return out;
}
// int settings that accept any value (speed limit): the presets, the current custom value and
// "Custom…", which asks for a number
function withCustom(st, ch, cur, setter, unset) {
  if (!st.custom) return [ch, setter];
  if (cur !== undefined && cur !== null && !ch.some(c => c.v === String(cur)) && /^\d+$/.test(String(cur))) ch.push({ v: String(cur), l: cur + (st.suffix || '') + ' (custom)' });
  ch.push({ v: '__custom', l: 'Custom…' });
  return [ch, async v => {
    if (v !== '__custom') return setter(v);
    const txt = await dialog({ title: st.label, text: `Enter a value from ${st.min} to ${st.max}${st.suffix || ''}.`, input: { value: /^\d+$/.test(String(cur)) ? String(cur) : '100', max: 5 },
      buttons: [{ label: 'Cancel', value: null }, { label: 'Set', value: true, primary: true }] });
    if (txt === null || txt === undefined) return setter(cur ?? unset); // cancelled: unchanged
    const n = parseInt(String(txt).replace(/[^0-9]/g, ''), 10);
    if (!(n >= st.min && n <= st.max)) { alertDlg('Not a valid value', `${st.label} can be ${st.min}–${st.max}${st.suffix || ''}.`); return; }
    return setter(String(n));
  }];
}
function infoRow(label, value, sub, cls = '') {
  return h('div', { class: 'row info ' + cls }, h('span', { class: 'rl' }, h('span', {}, label), sub && h('small', {}, sub)), h('span', { class: 'rv muted' }, value || ''));
}
function resolutionOf(scale, docked) {
  const f = parseFloat(scale || '1') || 1, hgt = docked === 'handheld' ? 720 : 1080;
  return `${Math.round(hgt * 16 / 9 * f)} × ${Math.round(hgt * f)} (${f}×, ${docked === 'handheld' ? 'handheld' : 'docked'})`;
}
// effective value of a setting for an engine: game override > NEXUS global > engine's own config
function effValue(view, id, engine, withGame = true) {
  if (withGame && view.overrides && view.overrides[id] !== undefined) return [view.overrides[id], 'game'];
  if (view.global[id] !== undefined) return [view.global[id], 'nexus'];
  const ev = view.engineValues?.[engine]?.[id];
  if (ev !== undefined) return [ev, 'engine'];
  return [undefined, 'default'];
}
function settingRows(container, view, section, { mode, engine, save, filter }) {
  const engines = view.engines.map(e => e.id);
  for (const st of view.settings.filter(x => x.section === section && (!filter || filter(x)))) {
    const id = st.id;
    if (mode === 'game') {
      const cap = view.caps[engine][id] || {};
      if (id === 'graphics.resolution') {
        container.append(infoRow(st.label, resolutionOf(effValue(view, 'graphics.scale', engine)[0], effValue(view, 'system.docked', engine)[0]), st.help));
        continue;
      }
      if (id === 'system.firmware') {
        container.append(infoRow(st.label, view.firmware || 'Not installed', view.firmware ? 'Installed in Settings › System files.' : 'Install your firmware in Settings › System files.'));
        continue;
      }
      if (id === 'controls.profile' && view.game) {
        const box = h('div'); container.append(box);
        const key = view.game.key;
        const drawLayout = async () => {
          const r = await api(`/api/games/${key}/layout?profile=${S.profile?.id || ''}`);
          box.replaceChildren();
          if (r.preset) box.append(h('p', { class: 'note' + (/cannot|not playable/.test(r.preset.note) ? ' warn' : '') }, r.preset.note));
          box.append(rowChoice('Controller layout for this game', [{ v: '', l: 'Use the user’s layout' }, { v: 'own', l: 'Its own layout' }], r.own ? 'own' : '',
            async v => { await api(`/api/games/${key}/layout`, { body: { layout: v ? (r.own || { base: r.layout.base, buttons: r.layout.buttons || {}, controllerType: r.layout.controllerType }) : null } }); drawLayout(); },
            r.own ? 'This game uses its own buttons and controller type.' : `Uses ${r.source === 'preset' ? 'the user’s layout with this game’s preset' : r.source === 'user' ? 'the user’s layout' : 'the default layout'}.`));
          if (r.own) layoutRows(box, r.own, async l => { await api(`/api/games/${key}/layout`, { body: { layout: l } }); drawLayout(); }, 'game');
        };
        drawLayout();
      }
      if (id === 'controls.profile') {
        if (!cap.available) { container.append(infoRow(st.label, 'Not available', cap.note, 'unavail')); continue; }
        const ips = view.inputProfiles || [];
        const r = rowChoice(st.label, [{ v: '', l: 'Use the user’s controls' }, ...ips.map(x => ({ v: x, l: x }))], view.inputProfile || '', v => save({ inputProfile: v }),
          ips.length ? 'Loads this controller profile only for this game.' : 'No saved controller profiles yet. Without one, player 1 is mapped automatically to the first connected controller.');
        if (view.inputProfile) r.classList.add('overridden');
        srcTag(r, !!view.inputProfile);
        addReset(r, { changed: () => !!view.inputProfile, def: () => 'the user’s controls', run: () => save({ inputProfile: '' }) });
        container.append(r);
        continue;
      }
      if (id === 'audio.volume') {
        const cur = view.overrides[id];
        const gv = +(effValue(view, id, engine, false)[0] ?? 100);
        const r = rowVolume(st.label, cur !== undefined ? +cur : gv, (v, commit) => { if (commit) save({ settings: { [id]: String(v) } }); },
          cur !== undefined ? 'This game has its own volume.' : `Follows the global volume (${gv}%). Move the slider to give this game its own volume.`);
        if (cur !== undefined) {
          r.classList.add('overridden');
          const b = h('button', { class: 'chip', 'data-nav': true }, 'Use global'); b.addEventListener('click', () => save({ settings: { [id]: null } }));
          r.append(b);
        }
        srcTag(r, cur !== undefined);
        container.append(r);
        continue;
      }
      if (st.kind === 'info' || !cap.available) {
        container.append(infoRow(st.label, cap.available ? '' : 'Not available', cap.note || st.help || '', cap.available ? '' : 'unavail'));
        continue;
      }
      if (!cap.perGame) {
        const [gv] = effValue(view, id, engine, false);
        container.append(infoRow(st.label, nlabel(st, gv), (cap.note || '') + ' Change it in Settings › Graphics & emulation.', 'unavail'));
        continue;
      }
      const [gv, gsrc] = effValue(view, id, engine, false);
      const glabel = gv !== undefined ? nlabel(st, gv) : 'engine default';
      const cur = view.overrides[id];
      const [ch, onPick] = withCustom(st, [{ v: '__global', l: `Use global (${glabel})` }, ...nchoices(st, cap.choices)], cur, v => save({ settings: { [id]: v === '__global' ? null : v } }), '__global');
      if (cur !== undefined && !ch.some(c => c.v === cur)) ch.push({ v: cur, l: nlabel(st, cur) + ' (not supported by the engine, ignored)' });
      let sub = st.help || '';
      if (gsrc === 'engine') sub = (sub ? sub + ' ' : '') + 'Global value comes from the engine configuration.';
      const r = rowChoice(st.label, ch, cur ?? '__global', onPick, sub);
      if (cur !== undefined) r.classList.add('overridden');
      srcTag(r, cur !== undefined);
      addReset(r, { changed: () => cur !== undefined, def: () => `use the global setting: ${glabel}`, run: () => save({ settings: { [id]: null } }) });
      container.append(r);
    } else {
      if (id === 'graphics.resolution') { container.append(infoRow(st.label, resolutionOf(view.global['graphics.scale'] || '1', view.global['system.docked'] || 'docked'), st.help)); continue; }
      if (id === 'system.firmware') { container.append(infoRow(st.label, view.firmware || 'Not installed', 'Managed in Settings › System files.')); continue; }
      if (id === 'controls.profile') { container.append(infoRow(st.label, '', 'Each user picks their controller profile in Controllers; a game can override it in Configure Game Settings.')); continue; }
      if (id === 'audio.volume') { container.append(volumeReset(rowVolume(st.label, +(view.global[id] ?? 100), sendVolume, st.help))); continue; }
      const sup = engines.filter(e => view.caps[e][id]?.available);
      if (st.kind === 'info' || !sup.length) {
        const note = engines.map(e => view.caps[e][id]?.note).find(Boolean) || st.help;
        container.append(infoRow(st.label, sup.length ? '' : 'Not available', note, sup.length ? '' : 'unavail'));
        continue;
      }
      const engineNow = engines.map(e => view.caps[e][id]?.available ? `Engine default: ${view.engineValues[e]?.[id] !== undefined ? nlabel(st, view.engineValues[e][id]) : 'built-in'}` : 'Not supported').join(' · ');
      const cur = view.global[id];
      const [ch, onPick] = withCustom(st, [{ v: '__engine', l: 'Engine default' }, ...nchoices(st, view.caps[engines[0]][id]?.choices)], cur, v => save({ settings: { [id]: v === '__engine' ? null : v } }), '__engine');
      let sub = (st.help ? st.help + ' ' : '') + engineNow;
      const r = rowChoice(st.label, ch, cur ?? '__engine', onPick, sub);
      if (cur !== undefined) r.classList.add('overridden');
      addReset(r, { changed: () => cur !== undefined, def: () => 'engine default', run: () => save({ settings: { [id]: null } }) });
      container.append(r);
    }
  }
}

async function gameConfig(key, startSection = 'general') {
  const g = gameByKey(key); if (!g) return;
  let view;
  try { view = await api(`/api/games/${key}/settings`); } catch (e) { alertDlg('Could not load game settings', e.message); return; }
  const withContent = v => { if (g.titleId && g.kind !== 'homebrew' && !v.sections.some(x => x.v === 'updates')) v.sections = [...v.sections, { v: 'updates', l: 'Game Updates' }, { v: 'mods', l: 'Mods' }]; return v; };
  withContent(view);
  let section = startSection;
  // search within this game's settings (and "only this game's own settings")
  let query = '', onlyOwn = false;
  const own = st => view.overrides?.[st.id] !== undefined || (st.id === 'controls.profile' && !!view.inputProfile);
  const searchable = () => view.sections.filter(x => !['general', 'updates', 'mods'].includes(x.v));
  const matches = st => {
    if (onlyOwn && !own(st)) return false;
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!words.length) return true;
    const sec = view.sections.find(x => x.v === st.section)?.l || '';
    const text = [st.label, st.help, sec, ...(st.choices || []).map(c => c.l), own(st) ? 'override this game changed' : 'global inherited'].join(' ').toLowerCase();
    return words.every(w => text.includes(w));
  };
  const root = h('div', { class: 'cfg-page' });
  const layer = openLayer(root, { name: 'gameconfig', onTab: d => { const i = view.sections.findIndex(x => x.v === section); section = view.sections[(i + d + view.sections.length) % view.sections.length].v; draw(true); } });
  const nav = h('nav', { class: 'cfg-nav' });
  const body = h('div', { class: 'cfg-body' });
  const head = h('header', { class: 'cfg-head' });
  root.append(head, h('div', { class: 'cfg-main' }, nav, body));
  let saving = null;
  const save = async patch => {
    try {
      view = withContent(await api(`/api/games/${key}/settings`, { body: patch }));
      clearTimeout(saving); saving = setTimeout(() => toast('Saved', I.check, 1200), 150);
      await loadGames(); const ng = gameByKey(key); if (ng) Object.assign(g, ng); refreshViews();
      draw(false);
    } catch (e) { alertDlg('Could not save', e.message); draw(false); }
  };
  const drawHead = () => {
    const tile = gameTile(g); tile.removeAttribute('data-nav');
    const n = Object.keys(view.overrides || {}).length + (view.inputProfile ? 1 : 0);
    const close = h('button', { class: 'chip', 'data-nav': true }, 'Done'); close.addEventListener('click', () => layer.close());
    head.replaceChildren(tile, h('div', { class: 'gh-info' }, h('b', {}, g.title),
      h('span', {}, [g.version ? `Version ${g.version}` : '', g.titleId ? `Title ID ${g.titleId}` : ''].filter(Boolean).join(' · ')),
      h('span', {}, `NEXUS Engine · ${n ? `${n} game-specific setting${n > 1 ? 's' : ''}` : 'Uses global settings'} · changes save automatically`)), close);
  };
  let searchBox = null, ownChip = null;
  const drawNav = () => {
    const keepSearch = searchBox && Nav.cur === searchBox;
    nav.replaceChildren();
    if (!searchBox) {
      searchBox = h('input', { type: 'search', placeholder: 'Search this game’s settings', 'data-nav': true, autocomplete: 'off', spellcheck: 'false', 'aria-label': `Search ${g.title}’s settings` });
      let t = null;
      searchBox.addEventListener('input', () => { clearTimeout(t); t = setTimeout(() => { query = searchBox.value.trim(); drawBody(); drawNavCounts(); }, 100); });
      searchBox.addEventListener('submit', () => { const f = Nav.items(body)[0]; f && Nav.setFocus(f); });
      ownChip = h('button', { class: 'chip', 'data-nav': true, title: 'Show only the settings this game changes from the global settings' }, 'Only this game’s');
      ownChip.addEventListener('click', () => { onlyOwn = !onlyOwn; ownChip.classList.toggle('on', onlyOwn); drawBody(); });
    }
    nav.append(h('label', { class: 'search cfg-search' }, svg('M10 3a7 7 0 1 1 0 14 7 7 0 0 1 0-14Zm0 2a5 5 0 1 0 0 10 5 5 0 0 0 0-10Zm6.3 10.9 4.4 4.4-1.4 1.4-4.4-4.4z'), searchBox), h('div', { class: 'cfg-filter' }, ownChip));
    if (keepSearch) Nav.setFocus(searchBox, { silent: true });
    for (const sct of view.sections) {
      const cnt = sct.v === 'general' ? 0 : view.settings.filter(x => x.section === sct.v && view.overrides[x.id] !== undefined).length + (sct.v === 'controls' && view.inputProfile ? 1 : 0);
      const b = h('button', { class: 'cat' + (sct.v === section ? ' sel' : ''), 'data-nav': true }, sct.l, cnt ? h('small', { class: 'cnt' }, String(cnt)) : '');
      b._sec = sct.v;
      b.addEventListener('click', () => { section = sct.v; clearSearch(); draw(true); });
      b.addEventListener('navfocus', () => { if (S.lastInput !== 'mouse' && section !== sct.v && !searching()) { section = sct.v; drawBody(); drawNav(); Nav.setFocus($$('.cat', nav).find(x => x._sec === sct.v), { silent: true }); } });
      nav.append(b);
    }
    $$('.cat', nav).forEach(b => b.classList.toggle('sel', !searching() && b._sec === section));
  };
  const searching = () => !!query || onlyOwn;
  const clearSearch = () => { query = ''; onlyOwn = false; if (searchBox) searchBox.value = ''; ownChip?.classList.remove('on'); };
  const drawNavCounts = () => $$('.cat', nav).forEach(b => b.classList.toggle('sel', !searching() && b._sec === section));
  // results across every section of this game's settings, grouped by section
  const drawResults = () => {
    const eng = view.effective;
    body.append(h('h2', {}, query ? `Settings matching “${query}”` : 'This game’s own settings'));
    body.append(h('p', { class: 'note' }, 'A “This game” label marks a setting this game changes for itself (an override); “Global” follows Settings › Graphics & emulation. Changing a setting here changes it for this game only.'));
    if (!view.perGameOk) { body.append(h('p', { class: 'note warn' }, view.perGameReason)); return; }
    let n = 0;
    for (const sct of searchable()) {
      const box = h('div');
      settingRows(box, view, sct.v, { mode: 'game', engine: eng, save, filter: matches });
      const rows = $$(':scope > .row, :scope > div > .row', box);
      if (!rows.length) continue;
      n += rows.length;
      const head = h('h3', {}, sct.l); body.append(head, ...box.childNodes);
    }
    if (!n) body.append(h('p', { class: 'note' }, onlyOwn && !query ? 'This game uses the global settings for everything.' : 'No setting of this game matches. Try another word, such as “resolution”, “vsync”, “frame” or “volume”.'));
  };
  const drawBody = () => {
    body.replaceChildren();
    if (searching()) return drawResults();
    const eng = view.effective;
    body.append(h('h2', {}, view.sections.find(x => x.v === section).l));
    if (section === 'general') {
      const ext = (g.format || '').toUpperCase();
      const kind = { base: 'Game', update: 'Update', dlc: 'DLC', homebrew: 'Homebrew', unknown: 'Unknown' }[g.kind] || g.kind;
      body.append(h('dl', { class: 'kv' },
        h('dt', {}, 'Title'), h('dd', {}, g.title),
        h('dt', {}, 'Version'), h('dd', {}, g.version || 'Unknown'),
        h('dt', {}, 'Title ID'), h('dd', {}, g.titleId || 'Unknown'),
        h('dt', {}, 'Type'), h('dd', {}, kind),
        h('dt', {}, 'File'), h('dd', {}, `${ext} · ${fmtBytes(g.size)}`)));
      body.append(rowToggle('Performance monitor in this game', perfOn(g.key), v => perfToggleGame(g.key, v), 'The FPS / CPU / GPU bar while you play (Settings \u203a Performance Monitor).'));
      body.append(h('h3', {}, 'Compatibility'));
      body.append(compatRow(view.compat, g, r => { view.compat = r; drawBody(); }));
      body.append(h('h3', {}, 'Optimise for my device'));
      body.append(rowButton('Optimise this game…', { sub: 'Settings for this game only, from your hardware and what is known about the game. Other games are not changed.', onClick: () => {
        sheet(`Optimise ${g.title}`, (sb) => { const ob = h('div'); sb.append(ob); optimiserPanel(ob, key, async () => { view = withContent(await api(`/api/games/${key}/settings`)); draw(false); }); });
      } }));
      if (!S.engine?.installed) body.append(h('p', { class: 'note warn' }, 'The NEXUS engine files are missing or damaged. Extract the complete NEXUS ZIP again; Settings › NEXUS Engine shows what is wrong.'));
      if (!view.perGameOk) body.append(h('p', { class: 'note warn' }, view.perGameReason));
      body.append(h('h3', {}, 'Game-specific settings'));
      const ov = Object.entries(view.overrides || {});
      if (!ov.length && !view.inputProfile) body.append(h('p', { class: 'note' }, 'This game uses the global settings. Change anything in Graphics, Advanced Graphics, Audio, Controls or System to override it for this game only.'));
      for (const [id, v] of ov) {
        const st = view.settings.find(x => x.id === id); if (!st) continue;
        const cap = view.caps[eng][id] || {};
        body.append(rowButton(st.label, { value: nlabel(st, v), sub: (view.sections.find(x => x.v === st.section)?.l || '') + (cap.available && cap.perGame ? '' : ' · not applied per game'), onClick: () => { section = st.section; draw(true); } }));
      }
      body.append(h('h3', {}, ''));
      body.append(rowButton('Restore defaults', { danger: true, disabled: !ov.length && !view.inputProfile, sub: 'All settings for this game go back to the global settings.', onClick: async () => {
        if (await confirmDlg('Restore defaults?', `${g.title} will use the global settings again.`, 'Restore', true)) save({ resetAll: true });
      } }));
      return;
    }
    if (section === 'updates' || section === 'mods') { const cb = h('div'); body.append(cb); gameContentPanel(cb, g, section, () => drawBody()); return; }
    if (!view.perGameOk) { body.append(h('p', { class: 'note warn' }, view.perGameReason)); return; }
    const note = { enhance: 'Applies only to this game. Frame generation suits games that already run at a steady frame rate.', graphics: 'Applies only to this game. “Use global” follows Settings › Graphics & emulation.', advanced: 'For advanced users. If a game misbehaves, use Restore defaults on the General page.',
      audio: '', controls: '', system: '' }[section];
    if (note) body.append(h('p', { class: 'note' }, note));
    if (section === 'enhance') { const eb = h('div'); body.append(eb); enhancementsPanel(eb); }
    settingRows(body, view, section, { mode: 'game', engine: eng, save });
  };
  const draw = focusBody => {
    drawHead(); drawNav();
    const keep = !focusBody && Nav.cur && body.contains(Nav.cur) ? Nav.items(body).indexOf(Nav.cur) : -1;
    const sc = body.scrollTop;
    drawBody();
    if (!focusBody) body.scrollTop = sc;
    if (keep >= 0) { const it = Nav.items(body); Nav.setFocus(it[Math.min(keep, it.length - 1)], { silent: true, scroll: false }); }
    else if (focusBody) { const it = Nav.items(body); Nav.setFocus(it[0] || $$('.cat', nav).find(x => x._sec === section), { silent: true }); body.scrollTop = 0; }
  };
  draw(false);
  Nav.setFocus($$('.cat', nav).find(x => x._sec === section), { silent: true });
}

function artNote(g) {
  return {
    builtin: 'Using artwork from the built-in NEXUS artwork database.', custom: 'Using artwork you chose.', sidecar: 'Using the image next to the game file.', 'artwork-folder': 'Using an image from NEXUS data\\Artwork.',
    embedded: 'Using the icon stored inside the game.', online: 'Using artwork downloaded for this Title ID.',
  }[g.artSource] || 'No artwork yet. NEXUS reads the icon from the game file (needs prod.keys), downloads it by Title ID, or uses an image you choose.';
}
function refreshViews() { if (S.screen === 'home') { const cur = Nav.cur; const k = cur?._game?.key; const dockIdx = cur ? $$('#dock .dock-btn').indexOf(cur) : -1; const topIdx = cur ? $$('#tb-profiles button').indexOf(cur) : -1; const inHome = cur && $('#home').contains(cur); renderHome(); if (Nav.top()?.dialog) return; let t = null; if (dockIdx >= 0) t = $$('#dock .dock-btn')[dockIdx]; else if (topIdx >= 0) t = $$('#tb-profiles button')[topIdx]; else t = $$('#tiles .tile').find(t => t._game?.key === k) || (inHome || !cur ? $('#tiles .tile') : null); if (t) Nav.setFocus(t, { silent: true }); } if (S.screen === 'library') { const k = Nav.cur?._game?.key; const keep = renderLibrary(k); if (keep) Nav.setFocus(keep, { silent: true }); } }

// ---------- launching: Who's playing? → the game opens → NEXUS loading screen → the game
async function launchGame(key, force = false) {
  const g = gameByKey(key); if (!g) return;
  if (!isPlayable(g)) { alertDlg('Not a game', 'This file is an update or DLC. Keep it in one of your game folders: NEXUS applies it automatically when you start the base game.'); return; }
  if (!S.profile) { showWho(); return; }
  if (S.playing?.suspended) {
    if (S.playing.gameKey === key) { resumeGame(); return; } // the running game: straight back into it (Close game is on the Home banner)
    const v = await dialog({ title: `${S.playing.title} is still running`, text: `Close it to start ${g.title}? Unsaved progress in ${S.playing.title} will be lost.`, buttons: [{ label: 'Cancel', value: false }, { label: `Resume ${S.playing.title}`, value: 'resume' }, { label: 'Close it', value: 'close', danger: true }], cancelValue: false });
    if (v === 'resume') resumeGame();
    if (v === 'close') api('/api/emulator/stop', { body: {} }).catch(e => toast(e.message, I.warn));
    return;
  }
  if (Launch.busy) return;
  // Stage A: who's playing (the session, play time and saves belong to this user)
  if (!force) {
    const pid = await whoIsPlaying(g);
    if (!pid) return;
    if (pid !== S.profile.id) {
      S.profile = profileById(pid); applyTheme(); refreshTop();
      api(`/api/profiles/${pid}/select`, { body: {} }).catch(() => { });
      S.settings.lastProfileId = pid;
    }
  }
  Launch.busy = true;
  try {
    const launching = api('/api/launch', { body: { key, profileId: S.profile.id, force } });
    Sound.play('launch'); Pad.rumble(160, .5);
    // Stage B: the launch animation plays on the game's own icon on the Home screen; the
    // loading screen appears only when it has finished
    const fromTile = await Launch.tileAnimation(key);
    let st = await launching;
    // state updates for this session that arrived during the tile animation are newer than the
    // launch reply: a quick start could otherwise be overwritten and the loading screen would
    // wait for a "playing" it already missed (docs/REGRESSION_HISTORY.md)
    if (S.emuLatest?.running && S.emuLatest.started === st.started && S.emuLatest.gameKey === st.gameKey) st = S.emuLatest;
    S.playing = st;
    Launch.open(g, st, fromTile);   // Stage C; Stage D when the engine shows the game
  } catch (e) {
    Launch.busy = false;
    Sound.play('error');
    if (e.data?.warnings) {
      const w = e.data.warnings;
      const go = await dialog({ title: w.length === 1 ? w[0].message : 'Before you play', text: w.map(x => (w.length > 1 ? '• ' + x.message + '\n' : '') + x.detail).join('\n\n'), buttons: [{ label: 'Cancel', value: false }, { label: 'Settings', value: 'settings' }, { label: 'Play anyway', value: true, primary: true }], cancelValue: false });
      if (go === true) return launchGame(key, true);
      if (go === 'settings') showSettings(w.some(x => x.code === 'no-keys' || x.code === 'no-firmware') ? 'sysfiles' : 'engine');
      return;
    }
    const d = e.data || {};
    const btn = await dialog({ title: d.error || 'NEXUS could not start this game', text: d.detail || e.message, buttons: d.code === 'no-engine' ? [{ label: 'Close', value: false }, { label: 'NEXUS Engine', value: 'emu', primary: true }] : [{ label: 'OK', value: false, primary: true }] });
    if (btn === 'emu') showSettings('engine');
  }
}

// Stage A — "Who's playing?". Skipped when only one user exists (setting: Game sessions › Ask
// who's playing). Resolves with the chosen profile id, or null when cancelled.
function whoIsPlaying(g) {
  const mode = S.settings.emulator?.whoPlays || 'several';
  if (mode === 'never' || (mode === 'several' && S.profiles.length < 2)) return Promise.resolve(S.profile.id);
  return new Promise(resolve => {
    let done = false;
    const row = h('div', { class: 'wp-row', 'data-row': 'strict' });
    const card = h('div', { class: 'wp-card' },
      h('div', { class: 'wp-head' }, h('div', { class: 'wp-game' }, launchArt(g, 'wp-art')), h('div', {}, h('h2', {}, 'Who\u2019s playing?'), h('p', {}, g.title))),
      row);
    const layer = openLayer(card, { name: 'who-plays', onBack: () => finish(null) });
    layer.root.classList.add('wp-layer');
    const finish = (pid, btn) => {
      if (done) return; done = true;
      if (!pid) { layer.close(); resolve(null); return; }
      Sound.play('select'); Pad.rumble(60, .3);
      btn?.classList.add('picked');
      card.classList.add('confirmed');
      $$('button', card).forEach(b => b.setAttribute('disabled', '')); // no second pick or late cancel
      setTimeout(() => { layer.close(); resolve(pid); }, S.anim === 'off' ? 0 : 420);
    };
    for (const p of S.profiles) {
      const a = avatarEl(p, 'wp-avatar');
      const b = h('button', { class: 'wp-user' + (p.id === S.profile?.id ? ' current' : ''), 'data-nav': true, 'aria-current': p.id === S.profile?.id ? 'true' : null, style: { '--pc': p.theme?.accent || 'var(--accent)' } }, a, h('span', {}, p.name));
      b.addEventListener('click', () => finish(p.id, b));
      row.append(b);
    }
    const cancel = h('button', { class: 'btn wp-cancel', 'data-nav': true }, 'Cancel');
    cancel.addEventListener('click', () => finish(null));
    card.append(h('div', { class: 'wp-actions' }, cancel));
    const cur = $$('.wp-user', row)[S.profiles.findIndex(p => p.id === S.profile?.id)] || $('.wp-user', row);
    Nav.setFocus(cur, { silent: true });
  });
}

// Waits for one key (for the NEXUS Home key). Esc keeps the old key, Backspace/Delete turns it off.
const keyCombo = (ev, k) => (ev.ctrlKey ? 'Ctrl+' : '') + (ev.altKey ? 'Alt+' : '') + (ev.shiftKey ? 'Shift+' : '') + k;
// does a browser key event match a NEXUS shortcut such as "F12" or "Alt+F12"?
function shortcutMatches(ev, sc) {
  if (!sc) return false;
  const k = SDL_KEY(ev.code); if (!k) return false;
  return keyCombo(ev, k).toLowerCase() === sc.replace(/control\+/i, 'Ctrl+').toLowerCase();
}
const SDL_KEY = c => /^Key[A-Z]$/.test(c) ? c.slice(3) : /^Digit\d$/.test(c) ? c.slice(5) : /^F([1-9]|1[0-2])$/.test(c) ? c :
  ({ Home: 'Home', End: 'End', Insert: 'Insert', PageUp: 'PageUp', PageDown: 'PageDown', Pause: 'Pause', ScrollLock: 'ScrollLock', Backquote: '`' })[c];
// captureKey({ mods: true }) also accepts Ctrl / Alt / Shift combinations ("Alt+F12")
function captureKey({ mods = false } = {}) {
  return new Promise(resolve => {
    const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, mods ? 'Press a key or a combination' : 'Press a key'),
      h('p', {}, 'Letters, numbers, F1–F12, Home, End, Insert, Page Up/Down, Pause or Scroll Lock' + (mods ? ', optionally with Ctrl, Alt or Shift (for example Alt+F12)' : '') + '. Backspace turns the shortcut off; Esc cancels.')));
    let layer;
    const onKey = ev => {
      ev.preventDefault(); ev.stopPropagation();
      if (/^(Control|Alt|Shift|Meta|OS)/.test(ev.code)) return; // waiting for the key itself
      if (ev.code === 'Escape') return done(undefined);
      if (ev.code === 'Backspace' || ev.code === 'Delete') return done('');
      const k = SDL_KEY(ev.code); if (!k) { toast('That key cannot be used', I.warn, 1500); return; }
      done(mods ? keyCombo(ev, k) : k);
    };
    const done = v => { window.removeEventListener('keydown', onKey, true); layer.close(); resolve(v); };
    layer = openLayer(box, { onBack: () => done(undefined) });
    window.addEventListener('keydown', onKey, true);
  });
}

function launchArt(g, cls) {
  const el = h('div', { class: 'launch-art ' + cls });
  if (g?.artSource) el.append(h('img', { src: `/art/${g.key}?v=${g.artRev || 0}&w=640`, alt: '' }));
  else el.append(h('div', { class: 'launch-ph' }, svg(I.pad), h('span', {}, g?.title || '')));
  return el;
}

// Stages B–D: the game opens (artwork with an expanding ghost), the NEXUS loading screen with the
// engine's real progress steps, then the game. One design for every game for now.
const Launch = {
  busy: false, el: null, key: null, t0: 0,
  // stages: prepare = finding and converting the game's shaders in its files (before the engine
  // starts), precache = compiling them (engine boot), shaders = building the pipelines earlier
  // sessions recorded. Each is a different kind of work, so each has its own words.
  STAGES: { prepare: ['Preparing shaders', 0], preparing: ['Preparing your session', 0], loading: ['Loading the game', 1], precache: ['Compiling the game’s shaders', 1], shaders: ['Building shaders from earlier sessions', 1], starting: ['Starting', 2] },
  PREP: { 'Reading game files': 'Reading the game’s files', 'Extracting shaders': 'Finding shaders in the game’s files', Converting: 'Converting the shader programs', Writing: 'Saving the prepared shaders' },
  // the selected icon on Home: a press, a bright pulse and a zoom toward the viewer, in place
  tileAnimation(key) {
    const tile = S.screen === 'home' && S.anim !== 'off' ? $$('#tiles .tile').find(t => t._game?.key === key) : null;
    if (!tile) return Promise.resolve(false);
    tile.classList.remove('launching'); void tile.offsetWidth;
    tile.classList.add('launching');
    return new Promise(res => {
      const done = () => { tile.removeEventListener('animationend', done); res(true); setTimeout(() => tile.classList.remove('launching'), 400); };
      tile.addEventListener('animationend', done);
      setTimeout(done, 900); // safety
    });
  },
  open(g, st, fromTile = false) {
    this.close(true);
    this.key = g.key; this.t0 = performance.now() - (fromTile ? 1500 : 0);
    this.etaW = new ShaderEta(); this.sawPrep = this.prepFailed = false;
    const ghost = fromTile ? null : launchArt(g, 'lx-ghost'), art = launchArt(g, 'lx-art');
    const steps = h('div', { class: 'lx-steps' }, ['preparing', 'loading', 'starting'].map(k => h('span', {}, h('i'), this.STAGES[k][0])));
    const el = h('div', { class: 'launchfx' + (S.anim === 'off' ? ' no-anim' : '') + (fromTile ? ' from-tile loading' : ''), role: 'status' },
      h('div', { class: 'lx-backdrop', style: g.artSource ? { backgroundImage: `url("/art/${g.key}?v=${g.artRev || 0}&w=640")` } : {} }),
      h('div', { class: 'lx-stage' }, h('div', { class: 'lx-artwrap' }, ghost, art),
        h('div', { class: 'lx-info' }, h('h1', {}, g.title), h('div', { class: 'lx-status' }, ''), h('div', { class: 'lx-bar' }, h('i')), h('div', { class: 'lx-eta', 'aria-live': 'polite' }), steps, h('div', { class: 'lx-shaders', 'aria-live': 'polite' }))),
      h('div', { class: 'lx-brand' }, h('span', { class: 'lx-mark' }), 'NEXUS'));
    const cancel = h('button', { class: 'btn lx-cancel', 'data-nav': true }, 'Cancel');
    cancel.addEventListener('click', () => this.cancel());
    // while the game's shaders compile before it starts: start now, the rest compiles when used
    const skip = h('button', { class: 'btn lx-skip', 'data-nav': true, hidden: true, title: 'The shaders not compiled yet are compiled when the game first uses them (they may stutter once). The next start finishes them.' }, 'Start now');
    skip.addEventListener('click', () => { skip.disabled = true; api('/api/emulator/skip-precache', { body: {} }).catch(() => { }); });
    el.append(skip, cancel);
    document.body.append(el); this.el = el;
    // the loading screen takes the controls while it is up (nothing behind it reacts)
    // close: goHome closes every dialog layer, the loading screen included (it had none: goHome threw)
    this.layer = { root: el, dialog: true, name: 'launch', onBack: () => this.cancel(), initial: cancel, close: () => this.close() };
    Nav.push(this.layer); renderHints();
    requestAnimationFrame(() => el.classList.add('in'));
    // Stage B → C: after the opening animation the loading screen takes over
    if (!fromTile) setTimeout(() => el.classList.add('loading'), S.anim === 'off' ? 0 : 950);
    this.update(st);
  },
  update(st) {
    const el = this.el; if (!el || !st) return;
    let [label, idx] = this.STAGES[st.stage] || this.STAGES.preparing;
    if (st.stage === 'prepare' && st.prep?.stage) label = this.PREP[st.prep.stage] || st.prep.stage;
    // a determinate bar only when the engine measures progress (a count with a known total);
    // never invented: reading the game's files has no total, so it shows no bar
    const measured = (st.stage === 'shaders' || st.stage === 'precache' || st.stage === 'prepare') && st.loadTotal > 0;
    const skip = $('.lx-skip', el); if (skip) skip.hidden = st.stage !== 'precache';
    $('.lx-status', el).textContent = measured ? `${label}: ${(st.loadDone || 0).toLocaleString()} of ${st.loadTotal.toLocaleString()}` : label + '…';
    const bar = $('.lx-bar', el); bar.classList.toggle('measured', measured);
    if (measured) bar.firstChild.style.width = `${Math.min(100, Math.round((st.loadDone || 0) / st.loadTotal * 100))}%`; else bar.firstChild.style.width = '';
    $$('.lx-steps span', el).forEach((s, i) => { s.classList.toggle('done', i < idx); s.classList.toggle('on', i === idx); });
    this.shaders(st.shaders);
    this.eta(st, measured, label);
    if (st.phase === 'playing') this.enter();
  },
  // remaining time for measured shader work, from the throughput measured on this screen only
  // (eta.js); the preparation's stages and the compiling stages each have their own estimate
  eta(st, measured, label) {
    const box = this.el && $('.lx-eta', this.el); if (!box) return;
    if (st.stage === 'prepare') this.sawPrep = true;
    const prepDone = this.sawPrep && !this.prepFailed && st.stage !== 'prepare';
    let r = null;
    if (measured) {
      r = this.etaW.add(st.stage + '|' + (st.prep?.stage || ''), st.loadDone || 0, Math.max(0, st.loadTotal - (st.loadDone || 0)));
      if (r.state === 'complete') r = null; // one stage done, not the whole preparation
    } else if (st.shaders && st.shaders.queued != null && st.stage !== 'prepare') {
      const sh = st.shaders, left = Math.max(0, sh.queued + (sh.running || 0));
      if (left > 0) r = this.etaW.add('pipelines', (sh.fromCache || 0) + (sh.fromPrebuild || 0) + (sh.compiled || 0), left);
    }
    if (!r && prepDone) r = { state: 'complete', text: 'Shader preparation complete.' };
    const rate = r?.state === 'estimate' ? fmtRate(r.rate) : '';
    box.dataset.state = r?.state || '';
    box.textContent = r?.text ? r.text + (rate ? ` \u00b7 ${rate}` : '') : '';
  },
  // the shader section: pipelines ready from this PC's cache, builds in flight, compiled now.
  // Counts only; shaders the game has not asked for yet are unknown, so no percentage here.
  shaders(sh) {
    const box = this.el && $('.lx-shaders', this.el); if (!box) return;
    if (!sh) { box.replaceChildren(); return; }
    const n = v => Number(v || 0).toLocaleString();
    const building = sh.queued == null ? null : Math.max(0, sh.queued + (sh.running || 0) - (sh.backgroundQueued || 0));
    const items = [h('span', {}, h('b', {}, n(sh.fromCache + (sh.fromPrebuild || 0))), ' ready from the shader cache')];
    if (building) items.push(h('span', { class: 'busy' }, h('b', {}, n(building)), ' compiling'));
    if (sh.backgroundQueued) items.push(h('span', {}, h('b', {}, n(sh.backgroundQueued)), ' more preparing in the background'));
    if (sh.compiled) items.push(h('span', {}, h('b', {}, n(sh.compiled)), ' new this session'));
    if (sh.failed) items.push(h('span', { class: 'busy' }, h('b', {}, n(sh.failed)), ' could not be built'));
    box.replaceChildren(...items);
  },
  // Stage D: the engine shows the game — the loading screen fades and NEXUS steps aside
  // The game's first frame is ready: the loading screen's content fades out over the artwork
  // backdrop, then NEXUS asks the engine to show the game (which fades in on top). The loading
  // screen stays underneath until the game covers it, so there is never an empty or black frame.
  enter() {
    const el = this.el; if (!el || el.classList.contains('revealing') || this.cancelling) return;
    const wait = Math.max(0, (S.anim === 'off' ? 0 : 1500) - (performance.now() - this.t0)); // let the opening finish
    setTimeout(() => {
      if (this.el !== el || this.cancelling) return;
      $('.lx-status', el).textContent = 'Have fun!';
      $$('.lx-steps span', el).forEach(s => { s.classList.add('done'); s.classList.remove('on'); });
      el.classList.add('revealing');
      setTimeout(() => {
        if (this.el !== el || this.cancelling) return;
        api('/api/emulator/reveal', { body: {} }).catch(() => { });
        // the player may already have pressed Home (the game set aside): stay on Home then
        setTimeout(() => { if (this.el === el && !S.playing?.suspended) showPlaying(true); if (this.el === el) this.close(); }, S.anim === 'off' ? 200 : 900);
      }, S.anim === 'off' ? 0 : 320);
    }, wait);
  },
  // Cancel: the game has not started for the user yet, so it stops at once (no question that
  // the game could appear over), the loading screen closes, and nothing is revealed afterwards.
  cancel() {
    if (!this.el || this.el.classList.contains('out') || this.cancelling) return;
    this.cancelling = true;
    const el = this.el;
    $('.lx-status', el).textContent = 'Stopping\u2026';
    $('.lx-cancel', el)?.setAttribute('disabled', '');
    api('/api/emulator/stop', { body: {} }).catch(() => { }).finally(() => {
      if (this.el === el) this.close();
      this.cancelling = false;
      if (S.screen !== 'home') goHome();
    });
  },
  close(now) {
    if (this.layer) { Nav.pop(this.layer); this.layer = null; renderHints(); }
    if (this.el) { const el = this.el; this.el = null; if (now) el.remove(); else { el.classList.add('out'); setTimeout(() => el.remove(), 300); } }
    this.busy = false;
  },
};

// A failed or crashed game, in NEXUS's words; the engine's technical details only on request.
async function launchErrorDlg(title, err) {
  const btns = [{ label: 'OK', value: false, primary: true }];
  if (err.technical) btns.unshift({ label: 'Technical details', value: 'tech' });
  if (err.log) btns.unshift({ label: 'Open diagnostic log', value: 'log' });
  const v = await dialog({ title: err.message || 'NEXUS could not start this game', text: (title ? title + '\n\n' : '') + (err.detail || ''), buttons: btns });
  if (v === 'log') api('/api/app/reveal-log', { body: { path: err.log } }).catch(e => toast(e.message, I.warn));
  if (v === 'tech') { await dialog({ title: 'Technical details', pre: err.technical + (err.log ? '\n\nDiagnostic log: ' + err.log : ''), buttons: [{ label: 'Close', value: true, primary: true }] }); }
}

function showPlaying(justLaunched = false) {
  const st = S.playing; if (!st) return;
  const g = gameByKey(st.gameKey);
  const art = $('#play-art'); art.innerHTML = '';
  if (g) { const t = gameTile(g); t.removeAttribute('data-nav'); art.append(t); }
  else art.append(h('div', { class: 'tile all' }, svg(I.chip, 'all-ic')));
  $('#play-title').textContent = st.title || 'NEXUS';
  $('#play-sub').textContent = st.phase === 'playing' ? `Playing as ${S.profile?.name || ''} · NEXUS Engine` : 'Starting the NEXUS engine…';
  const hk = S.settings.emulator?.homeKey;
  $('#play-tip').textContent = (hk && !st.homeKeyConflict ? `Press ${hk} or the controller\u2019s Home button` : 'Press the controller\u2019s Home button') + ' to go back to NEXUS Home; the game keeps running.' + (S.settings.emulator?.fullscreenKey ? ` ${S.settings.emulator.fullscreenKey}: full screen / window.` : '') + (S.settings.emulator?.screenshotKey ? ` ${S.settings.emulator.screenshotKey}: screenshot.` : '') + (st.homeKeyConflict ? ` (${hk} is one of this game\u2019s controls, so it goes to the game. You can choose another Home key in Settings \u203a Game sessions.)` : '') + (S.settings.emulator?.quitCombo ? ' Hold View + Menu (Back + Start) to close the game.' : '') + frameGenNote(st.frameGen);
  // no menu here: Home goes straight to the Home screen, where the game's tile resumes it and
  // the banner offers Close game; this view is only what stays behind a game in its own window
  $('#play-close').hidden = true; $('#play-perf').hidden = true; $('#play-volume').hidden = true;
  $('#play-return').textContent = 'Return to game';
  $('#play-perf').textContent = perfOn(st.gameKey) ? 'Hide performance monitor' : 'Show performance monitor';
  const vh = $('#play-volume'); let sl = vh.firstChild;
  if (!sl) { sl = volumeSlider({ value: st.volume ?? S.emulator?.volume ?? 100, onChange: sendVolume, compact: true, label: 'Game volume' }); vh.append(sl); }
  else if (!sl.classList.contains('drag') && Nav.cur !== sl) sl.setValue(st.volume ?? 100);
  show('playing', { initial: $('#play-return'), anim: !justLaunched });
  syncGameInFront();
}
Screens.playing = { onBack: () => goHome() };
$('#play-perf').addEventListener('click', async () => {
  const key = S.playing?.gameKey; if (!key) return;
  const on = !perfOn(key);
  await perfToggleGame(key, on);
  toast(on ? 'Performance monitor on for this game' : 'Performance monitor off for this game', I.check, 2000);
  showPlaying();
});
$('#play-return').addEventListener('click', () => resumeGame());
$('#play-close').addEventListener('click', async () => {
  const ok = await confirmDlg('Close the game?', 'Unsaved progress will be lost.', 'Close', true);
  if (!ok) return;
  await api('/api/emulator/stop', { body: {} }).catch(() => { });
  setTimeout(async () => {
    if (S.playing) {
      const force = await confirmDlg('The game is not responding', 'Force it to close? Unsaved progress is lost.', 'Force close', true);
      if (force) api('/api/emulator/stop', { body: { force: true } }).catch(() => { });
    }
  }, 6000);
});

// ---------- settings
// Settings are grouped the way people look for them (refinement brief, 17 Nov). Category ids
// are kept from earlier versions so links and shortcuts keep working; CAT_ALIAS sends the ids of
// categories that were merged to their new place.
const CATS = [
  ['-', 'General'], ['general', 'General'], ['users', 'Users'],
  ['-', 'Personalisation'], ['appearance', 'Appearance'], ['home', 'Home screen'],
  ['-', 'Games'], ['library', 'Game library'], ['emulator', 'Playing games'], ['savedata', 'Save data'], ['amiibo', 'Amiibo'], ['recording', 'Screen recording'],
  ['-', 'Controllers'], ['controllers', 'Controllers'],
  ['-', 'Graphics & performance'], ['emusettings', 'Graphics & emulation'], ['shaders', 'Shader management'], ['perf', 'Performance monitor'],
  ['-', 'Accounts & cloud'], ['account', 'NEXUS Account & GitHub'],
  ['-', 'Advanced'], ['engine', 'NEXUS Engine'], ['sysfiles', 'System files'], ['system', 'Storage & data'],
  ['-', 'About'], ['updates', 'Updates'], ['about', 'About NEXUS'],
];
const CAT_ALIAS = { startup: 'general' };
const NAVCATS = () => catList().filter(c => c[0] !== '-' && c[0] !== 'controllers');
// NEXUS Sync appears only when the experimental Device Linking feature is switched on
const catList = () => {
  if (!S.features?.deviceLink) return CATS;
  const i = CATS.findIndex(c => c[0] === 'account');
  return [...CATS.slice(0, i + 1), ['sync', 'Local Sync (experimental)'], ...CATS.slice(i + 1)];
};
// the group a category belongs to (shown above the panel's title)
function catGroup(id) {
  let g = '';
  for (const [c, l] of catList()) { if (c === '-') g = l; else if (c === id) return g; }
  return '';
}
function catLabel(id) { return (catList().find(c => c[0] === id) || [])[1] || ''; }
function showSettings(cat = 'general', opts = {}) {
  cat = CAT_ALIAS[cat] || cat;
  const cats = $('#set-cats'); cats.innerHTML = '';
  cats.append(settingsSearchBox());
  for (const [id, label] of catList()) {
    if (id === '-') { cats.append(h('div', { class: 'cat-group' }, label)); continue; }
    const b = h('button', { class: 'cat' + (id === cat ? ' sel' : ''), 'data-nav': true }, label);
    b._cat = id;
    b.addEventListener('click', () => { if (id === 'controllers') { Controllers.show(); return; } selectCat(id, true); });
    b.addEventListener('navfocus', () => { if (S.lastInput !== 'mouse' && id !== 'controllers') selectCat(id, false); });
    cats.append(b);
  }
  selectCat(cat, false).then(() => { if (opts.find) settingsReveal(opts.find); });
  show('settings', { initial: opts.find ? null : $$('.cat', cats).find(b => b._cat === cat) });
}
let curCat = 'appearance';
let catToken = 0;
async function selectCat(id, focusPanel) {
  id = CAT_ALIAS[id] || id;
  curCat = id;
  const tok = ++catToken;
  $$('#set-cats .cat').forEach(b => b.classList.toggle('sel', b._cat === id));
  $('#set-cats .cat.sel')?.scrollIntoView({ block: 'nearest' });
  const panel = $('#set-panel'); panel.innerHTML = ''; panel.scrollTop = 0;
  const fresh = h('div');
  const grp = catGroup(id);
  if (grp && grp !== catLabel(id)) fresh.append(h('div', { class: 'set-crumb' }, grp));
  const r = (Panels[id] || (() => { }))(fresh);
  panel.replaceChildren(...fresh.childNodes);
  if (r?.then) { const holder = h('div'); await r; if (tok !== catToken) return; panel.append(...fresh.childNodes); void holder; }
  if (focusPanel && tok === catToken) { const f = Nav.items(panel)[0]; if (f) Nav.setFocus(f); }
}
async function rebuildPanel() {
  const panel = $('#set-panel'); const items = Nav.items(panel); const idx = items.indexOf(Nav.cur);
  const scroll = panel.scrollTop;
  const fresh = h('div');
  const grp = catGroup(curCat);
  if (grp && grp !== catLabel(curCat)) fresh.append(h('div', { class: 'set-crumb' }, grp));
  await Panels[curCat](fresh);   // build off-screen (some panels load data first), then swap in
  panel.replaceChildren(...fresh.childNodes); panel.scrollTop = scroll;
  if (idx >= 0) { const n = Nav.items(panel); Nav.setFocus(n[Math.min(idx, n.length - 1)], { silent: true, scroll: false }); }
}
Screens.settings = {
  onBack: () => { if (Nav.cur && $('#set-panel').contains(Nav.cur)) { const c = $$('#set-cats .cat').find(b => b._cat === curCat); Nav.setFocus(c); } else goHome(); },
  onTab: d => { const L = NAVCATS(); const i = L.findIndex(c => c[0] === curCat); const n = L[(i + d + L.length) % L.length][0]; selectCat(n, false); Nav.setFocus($$('#set-cats .cat').find(b => b._cat === n)); },
};
async function saveSettings(patch) { S.settings = await api('/api/settings', { body: patch }); return S.settings; }
// ---------- wallpaper settings (static image + optional animated wallpaper)
const ANIM_ACCEPT = 'image/gif,image/webp,video/mp4,video/webm,.gif,.webp,.mp4,.webm';
function wallpaperSettings(p, th) {
  const wp = th.wallpaper || {};
  const pid = S.profile?.id;
  const done = pr => { S.profile = pr; S.profiles = S.profiles.map(x => x.id === pr.id ? pr : x); applyTheme(); rebuildPanel(); };
  p.append(h('h3', {}, 'Wallpaper'));
  const showing = wp.use === 'animated' && wp.animated ? 'Animated wallpaper' : wp.file ? 'Static wallpaper' : 'Default background';
  p.append(infoRow('Home screen background', showing));
  p.append(rowButton(wp.file ? 'Change static wallpaper…' : 'Choose static wallpaper…', { sub: 'PNG, JPG or WEBP from your PC.', onClick: async () => {
    const f = await pickImage(); if (!f) return;
    try { done(await api(`/api/profiles/${pid}/wallpaper`, { body: f })); toast('Wallpaper set', I.check); } catch (e) { alertDlg('Could not use that image', e.message); }
  } }));
  p.append(rowButton(wp.animated ? 'Import a different animated wallpaper…' : 'Import animated wallpaper…', { sub: 'GIF, animated WebP, MP4 (H.264) or WebM. You see a preview before it is applied.', onClick: () => importAnimatedWallpaper(done) }));
  if (wp.animated && wp.file) p.append(rowChoice('Use', [{ v: 'static', l: 'Static wallpaper' }, { v: 'animated', l: 'Animated wallpaper' }], wp.use, async v => { try { done(await api(`/api/profiles/${pid}/wallpaper/use`, { body: { use: v } })); } catch (e) { toast(e.message, I.warn); } }));
  else if (wp.animated && wp.use !== 'animated') p.append(rowButton('Use the animated wallpaper', { onClick: async () => done(await api(`/api/profiles/${pid}/wallpaper/use`, { body: { use: 'animated' } })) }));
  if (wp.animated) {
    p.append(changedMark(rowToggle('Play animated wallpaper', wp.motion !== 'off', v => themePatch({ wallpaper: { motion: v ? 'on' : 'off' } }), 'Off shows a still frame for maximum performance. It always pauses while a game runs, NEXUS is in the background, or another screen is open.'), 'profile:theme.wallpaper.motion'));
  }
  if (wp.file || wp.animated) {
    p.append(changedMark(rowRange('Brightness', { min: 10, max: 100, step: 5, value: Math.round((wp.brightness ?? .55) * 100), fmt: v => v + '%', onChange: (v, commit) => { S.profile.theme.wallpaper.brightness = v / 100; applyTheme(); if (commit) themePatch({ wallpaper: { brightness: v / 100 } }); } }), 'profile:theme.wallpaper.brightness'));
    p.append(changedMark(rowRange('Blur', { min: 0, max: 40, step: 2, value: wp.blur || 0, fmt: v => v ? v + ' px' : 'Off', onChange: (v, commit) => { S.profile.theme.wallpaper.blur = v; applyTheme(); if (commit) themePatch({ wallpaper: { blur: v } }); } }), 'profile:theme.wallpaper.blur'));
    p.append(changedMark(rowChoice('Scaling', [{ v: 'cover', l: 'Fill' }, { v: 'contain', l: 'Fit' }, { v: 'stretch', l: 'Stretch' }, { v: 'center', l: 'Centre' }, { v: 'tile', l: 'Tile (static only)' }], wp.fit || 'cover', v => themePatch({ wallpaper: { fit: v } })), 'profile:theme.wallpaper.fit'));
  }
  if (wp.animated) p.append(rowButton('Remove animated wallpaper', { danger: true, sub: wp.file ? 'Your static wallpaper is used again.' : 'The default background is used again.', onClick: async () => done(await api(`/api/profiles/${pid}/wallpaper?kind=animated`, { method: 'DELETE' })) }));
  if (wp.file || wp.animated) p.append(rowButton('Reset to default background', { danger: true, sub: 'Removes your custom wallpapers.', onClick: async () => done(await api(`/api/profiles/${pid}/wallpaper`, { method: 'DELETE' })) }));
}

// Preview an animated wallpaper as it would look behind the Home screen, then apply it.
async function importAnimatedWallpaper(done) {
  const f = await pickFile(ANIM_ACCEPT); if (!f) return;
  const isVideo = /^video\//.test(f.type) || /\.(mp4|webm)$/i.test(f.name);
  if (!isVideo && !/^image\/(gif|webp)$/.test(f.type) && !/\.(gif|webp)$/i.test(f.name)) { alertDlg('Unsupported file', 'Use GIF, animated WebP, MP4 or WebM.'); return; }
  const url = URL.createObjectURL(f);
  const wp = S.profile?.theme?.wallpaper || {};
  const media = isVideo ? h('video', { class: 'wpp-media', muted: true, loop: true, autoplay: true, playsinline: true, src: url }) : h('img', { class: 'wpp-media', src: url, alt: '' });
  if (isVideo) media.muted = true;
  media.style.filter = `blur(${wp.blur || 0}px) brightness(${wp.brightness ?? .55})`;
  const info = h('p', { class: 'note' }, `${f.name} · ${fmtBytes(f.size)}`);
  let ok = true;
  const ready = new Promise(res => {
    media.addEventListener(isVideo ? 'loadeddata' : 'load', () => {
      const w = isVideo ? media.videoWidth : media.naturalWidth, hh = isVideo ? media.videoHeight : media.naturalHeight;
      info.textContent = `${f.name} · ${w} × ${hh} · ${fmtBytes(f.size)}` + (w * hh > 2560 * 1440 ? ' · Large: NEXUS pauses it whenever it is not visible, but a 1080p file uses less GPU while it plays.' : '');
      res();
    }, { once: true });
    media.addEventListener('error', () => { ok = false; info.textContent = 'This file cannot be played by NEXUS (unsupported codec or damaged file). Try MP4 (H.264) or WebM.'; info.classList.add('warn'); res(); }, { once: true });
  });
  const tiles = h('div', { class: 'wpp-tiles' }, [0, 1, 2, 3].map(() => h('i')));
  const stage = h('div', { class: 'wpp-stage' }, media, h('div', { class: 'wpp-scrim' }), tiles);
  const v = await new Promise(resolve => {
    sheet('Animated wallpaper preview', (body, layer) => {
      body.append(stage, info);
      const row = h('div', { class: 'pk-setup-actions', 'data-row': 'strict' });
      const b1 = h('button', { class: 'btn', 'data-nav': true }, 'Cancel'); b1.addEventListener('click', () => { layer.close(); resolve(false); });
      const b2 = h('button', { class: 'btn primary', 'data-nav': true }, 'Apply'); b2.addEventListener('click', () => { layer.close(); resolve(true); });
      row.append(b1, b2); body.append(row);
      body.append(h('p', { class: 'note' }, 'It plays behind the Home screen only, loops, and pauses when a game runs or NEXUS is not visible. You can switch back to a static image at any time.'));
    }, { onClose: () => resolve(false) });
  });
  await ready;
  if (!v || !ok) { URL.revokeObjectURL(url); return; }
  // still frame for pausing and as a fallback (first frame of the file)
  let poster = null;
  try {
    const c = document.createElement('canvas');
    const w = isVideo ? media.videoWidth : media.naturalWidth, hh = isVideo ? media.videoHeight : media.naturalHeight;
    const sc = Math.min(1, 1920 / w); c.width = Math.round(w * sc); c.height = Math.round(hh * sc);
    if (isVideo) media.currentTime = 0;
    c.getContext('2d').drawImage(media, 0, 0, c.width, c.height);
    poster = await new Promise(r => c.toBlob(r, 'image/jpeg', .88));
  } catch { }
  try {
    toast('Importing animated wallpaper…', I.refresh);
    let pr = await api(`/api/profiles/${S.profile.id}/wallpaper?kind=animated`, { body: f });
    if (poster) pr = await api(`/api/profiles/${S.profile.id}/wallpaper?kind=poster`, { body: poster });
    done(pr); toast('Animated wallpaper applied', I.check);
  } catch (e) { alertDlg('Could not use that file', e.message); }
  URL.revokeObjectURL(url);
}

async function themePatch(patch) {
  if (!S.profile) return;
  await patchProfile(S.profile.id, { theme: patch });
  // keep the Appearance panel in step when the theme was changed from somewhere else
  if (('mode' in patch || 'accent' in patch) && S.screen === 'settings' && curCat === 'appearance' && !document.activeElement?.closest?.('.swatches')) rebuildPanel();
}

const ACCENTS = ['#ff2a3a', '#00c8ff', '#ff3b5c', '#ff9f0a', '#ffd60a', '#30d158', '#64d2ff', '#5e5ce6', '#bf5af2', '#ff6ec7', '#ffffff'];
const BOOT_COLORS = ['#e60012', '#ff5a1f', '#f5b300', '#1fae5b', '#0a84ff', '#5e2bff', '#e0218a', '#1d1e21', '#f2f2f2'];

// ---------- amiibo (amiibo.go): the user's own dumps and NEXUS virtual amiibo
const AmiiboUI = {
  KIND: { dump: 'Imported dump', mifare: 'Imported MIFARE dump', virtual: 'NEXUS virtual amiibo' },
  ownerName(id) { return id ? (S.profiles.find(p => p.id === id)?.name || 'A removed user') : 'Everyone'; },
  async load() { return api('/api/amiibo'); },
  async import(owner) {
    try { const r = await api('/api/amiibo/import', { body: { owner: owner || '' } }); if (!r.cancelled) { toast(`Added ${r.name}`, I.check); return r; } }
    catch (e) { alertDlg('The amiibo was not added', e.message); }
    return null;
  },
  // ----- Amiibo Creator: create or edit a NEXUS virtual amiibo (or edit an imported dump's details)
  TYPES: ['Figure', 'Card', 'Yarn', 'Band', 'Other'],
  createVirtual(owner) { return this.creator(null, owner); },
  creator(existing, owner) {
    api('/api/app/log', { body: { level: 'info', message: `amiibo creator: open (${existing ? 'edit' : 'new'})` } }).catch(() => { });
    return new Promise(resolve => { try { this.creatorBuild(existing, owner, resolve); } catch (e) {
      api('/api/app/log', { body: { level: 'error', message: 'amiibo creator: ' + (e.stack || e) } }).catch(() => { });
      alertDlg('The Amiibo Creator could not open', `${e.message}\n\nThis has been written to the NEXUS log (Logs\\nexus.log).`); resolve(null);
    } });
  },
  creatorBuild(existing, owner, resolve) {
    {
      const ed = existing || {};
      const isNew = !existing;
      const v = { amiiboId: ed.amiiboId || '', name: ed.name || '', character: ed.character || '', series: ed.series || '', gameSeries: ed.gameSeries || '', type: ed.type || '', notes: ed.notes || '', owner: existing ? (ed.owner || '') : (owner || '') };
      let artFile = null, artClear = false;
      const inp = (key, attrs = {}) => { const el = h(attrs.tag || 'input', { type: 'text', 'data-nav': true, spellcheck: 'false', maxlength: attrs.max || 60, placeholder: attrs.placeholder || '', ...(attrs.ro ? { readonly: true } : {}) }); el.value = v[key]; el.addEventListener('input', () => { v[key] = el.value; preview(); }); return el; };
      const field = (label, el, sub) => h('label', { class: 'field' }, h('span', {}, label), el, sub && h('small', {}, sub));
      const sel = (key, opts) => { const el = h('select', { 'data-nav': true }, ...opts.map(o => h('option', { value: o.v, selected: o.v === v[key] || undefined }, o.l))); el.addEventListener('change', () => { v[key] = el.value; preview(); }); return el; };
      // search the open amiibo database (AmiiboAPI; names and IDs only)
      const q = h('input', { type: 'text', 'data-nav': true, placeholder: 'Search: Link, Pikachu, Super Smash Bros.…', spellcheck: 'false' });
      const by = h('select', { 'data-nav': true }, h('option', { value: 'name' }, 'Name'), h('option', { value: 'character' }, 'Character'), h('option', { value: 'series' }, 'Amiibo series'), h('option', { value: 'gameseries' }, 'Game series'));
      const go = h('button', { 'data-nav': true, class: 'btn' }, 'Search');
      const results = h('div', { class: 'am-results' });
      const search = async () => {
        if (!q.value.trim()) return;
        results.replaceChildren(h('p', { class: 'note' }, 'Searching…'));
        try {
          const r = (await api(`/api/amiibo/lookup?q=${encodeURIComponent(q.value.trim())}&by=${by.value}`)).results || [];
          results.replaceChildren(...(r.length ? r.map(x => {
            const b = h('button', { class: 'am-result', 'data-nav': true }, h('b', {}, x.name), h('span', {}, `${x.amiiboSeries} · ${x.type}`));
            b.addEventListener('click', () => {
              Object.assign(v, { amiiboId: (x.head + x.tail).toLowerCase(), character: x.character || x.name, series: x.amiiboSeries, gameSeries: x.gameSeries, type: this.TYPES.includes(x.type) ? x.type : 'Other' });
              if (!v.name) v.name = x.name;
              fill(); preview(); $$('.am-result', results).forEach(e => e.classList.toggle('sel', e === b));
            });
            return b;
          }) : [h('p', { class: 'note' }, 'Nothing found. Check the spelling, try another kind of search, or type the amiibo ID below.')]));
        } catch (e) { results.replaceChildren(h('p', { class: 'note' }, e.message)); }
      };
      go.addEventListener('click', search);
      q.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); search(); } });
      const els = {
        amiiboId: inp('amiiboId', { max: 18, placeholder: '16 hexadecimal digits ending in 02', ro: !isNew }), name: inp('name', { max: 40, placeholder: 'Shown in NEXUS' }), character: inp('character'), series: inp('series'), gameSeries: inp('gameSeries'),
        type: sel('type', [{ v: '', l: '—' }, ...this.TYPES.map(t => ({ v: t, l: t }))]), owner: sel('owner', [{ v: '', l: 'Everyone' }, ...S.profiles.map(p => ({ v: p.id, l: p.name }))]), notes: inp('notes', { tag: 'textarea', max: 500, placeholder: 'Anything you want to remember' }),
      };
      const fill = () => { for (const k of ['amiiboId', 'name', 'character', 'series', 'gameSeries', 'type', 'owner', 'notes']) els[k].value = v[k]; };
      const artBtn = h('button', { 'data-nav': true, class: 'btn' }, 'Choose picture…');
      const artRm = h('button', { 'data-nav': true, class: 'btn' }, 'Remove picture');
      const artUrl = () => artFile ? URL.createObjectURL(artFile) : (!artClear && ed.art ? `/api/amiibo/art/${ed.id}?r=${ed.updated || 0}` : '');
      artBtn.addEventListener('click', async () => { const f = await pickImage(); if (f) { artFile = f; artClear = false; preview(); } });
      artRm.addEventListener('click', () => { artFile = null; artClear = true; preview(); });
      // live preview
      const card = h('div', { class: 'am-card' });
      const preview = () => {
        const url = artUrl();
        const pic = url ? h('div', { class: 'am-pic', style: { backgroundImage: `url("${url}")` } }) : h('div', { class: 'am-pic none' }, (v.character || v.name || '?').trim().slice(0, 1).toUpperCase());
        const okId = /^[0-9a-f]{14}02$/i.test(v.amiiboId.replace(/^0x/i, ''));
        card.replaceChildren(pic, h('b', {}, v.name || v.character || 'Unnamed amiibo'),
          h('span', {}, [v.character, v.type].filter(Boolean).join(' · ') || 'No details yet'), h('span', {}, v.series || ''),
          h('code', { class: okId ? '' : 'bad' }, okId ? v.amiiboId.replace(/^0x/i, '').toLowerCase() : (v.amiiboId ? 'Not a valid amiibo ID' : 'No amiibo ID yet')),
          h('span', {}, 'User: ' + this.ownerName(v.owner)),
          h('span', { class: 'am-badge' }, isNew || ed.kind === 'virtual' ? 'NEXUS virtual amiibo · works only in NEXUS' : (this.KIND[ed.kind] || 'Imported dump') + ' · data unchanged'));
        artRm.hidden = !url;
      };
      fill(); preview();
      const err = h('p', { class: 'form-err', role: 'alert' });
      const form = h('div', { class: 'am-form' },
        isNew && h('div', { class: 'am-search' }, h('div', { class: 'am-q' }, q, by, go), results),
        field('Amiibo ID', els.amiiboId, isNew ? 'Filled in when you pick a search result, or type it (from a database or your figure’s details).' : 'The amiibo ID of an existing entry cannot change. Duplicate it, or create a new one.'),
        field('Display name', els.name), h('div', { class: 'am-2' }, field('Character', els.character), field('Type', els.type)),
        h('div', { class: 'am-2' }, field('Amiibo series', els.series), field('Game series', els.gameSeries)),
        field('User', els.owner, 'Whose amiibo this is. Shown first when that user plays.'), field('Notes', els.notes),
        h('div', { class: 'am-art' }, artBtn, artRm, h('small', {}, 'Your own picture (PNG, JPEG or WEBP). Kept on this device.')), err);
      const cancel = h('button', { 'data-nav': true }, 'Cancel');
      const save = h('button', { 'data-nav': true, class: 'primary' }, isNew ? 'Create' : 'Save');
      const box = h('div', { class: 'dialog am-creator', role: 'dialog' },
        h('div', { class: 'd-body' }, h('h2', {}, isNew ? 'Amiibo Creator' : `Edit ${ed.name}`),
          h('p', { class: 'note' }, isNew ? 'A NEXUS virtual amiibo is a new, blank figure that NEXUS can give a game when it scans. It works only in NEXUS: it is not an authenticated Nintendo amiibo and cannot be written to a figure or card.' : 'Only the details NEXUS shows change; the amiibo data itself stays the same.'),
          h('div', { class: 'am-grid' }, form, card)),
        h('div', { class: 'd-actions', 'data-row': 'strict' }, cancel, save));
      let done = false;
      const finish = r => { if (done) return; done = true; layer.close(); resolve(r); };
      cancel.addEventListener('click', () => finish(null));
      save.addEventListener('click', async () => {
        err.textContent = '';
        const body = { name: v.name.trim(), character: v.character, series: v.series, gameSeries: v.gameSeries, type: v.type, notes: v.notes, owner: v.owner };
        save.disabled = true;
        try {
          let a = isNew ? await api('/api/amiibo/virtual', { body: { amiiboId: v.amiiboId, ...body } }) : await api('/api/amiibo/edit', { body: { id: ed.id, ...body } });
          if (artFile) a = await api(`/api/amiibo/art/${a.id}`, { body: artFile }).catch(e => { toast(e.message, I.warn); return a; });
          else if (artClear && ed.art) a = await api(`/api/amiibo/art/${a.id}`, { body: new Blob([]) });
          toast(isNew ? `Created ${a.name} (virtual)` : 'Saved', I.check);
          finish(a);
        } catch (e) { err.textContent = e.message; } finally { save.disabled = false; }
      });
      const layer = openLayer(box, { onBack: () => finish(null), initial: isNew ? q : els.name });
      if (S.lastInput !== 'gamepad') setTimeout(() => (isNew ? q : els.name).focus(), 50);
    }
  },
  async manage(a) {
    const opts = [{ v: 'edit', l: 'Edit details…' }, { v: 'duplicate', l: 'Duplicate' }, { v: 'owner', l: `User: ${this.ownerName(a.owner)}` }, { v: 'info', l: 'Details' }, { v: 'remove', l: 'Remove from library' }];
    const v = await listPick(a.name, a.file ? opts : opts.filter(o => o.v !== 'duplicate'));
    if (v === 'edit') await this.creator(a);
    else if (v === 'duplicate') {
      try { const d = await api('/api/amiibo/duplicate', { body: { id: a.id } }); toast(`${d.name} created${a.kind === 'virtual' ? ' (a new virtual amiibo with its own serial number)' : ''}`, I.check, 3500); } catch (e) { alertDlg('Not duplicated', e.message); }
    } else if (v === 'owner') {
      const o = await listPick('Whose amiibo is this?', [{ v: '', l: 'Everyone' }, ...S.profiles.map(p => ({ v: p.id, l: p.name }))], a.owner || '');
      if (o !== undefined) await api('/api/amiibo/update', { body: { id: a.id, owner: o } }).catch(e => toast(e.message, I.warn));
    } else if (v === 'info') {
      await alertDlg(a.name, null, [`Kind: ${this.KIND[a.kind] || a.kind}`, a.character && `Character: ${a.character}`, a.type && `Type: ${a.type}`, a.series && `Amiibo series: ${a.series}`, a.gameSeries && `Game series: ${a.gameSeries}`, a.amiiboId && `Amiibo ID: ${a.amiiboId}`, a.uid && `Serial number: ${a.uid}`, `User: ${this.ownerName(a.owner)}`, `Added: ${new Date(a.added * 1000).toLocaleString()}`, a.notes && `Notes: ${a.notes}`,
        !a.file ? `\nImported on ${a.origin || 'another device'}. Imported dumps are not synchronised; import it on this device to use it here.` : a.kind === 'virtual' ? '\nCreated by NEXUS. It is not an authenticated Nintendo amiibo and works only in NEXUS. Needs no amiibo keys.' : '\nNEXUS keeps a copy; the file you imported is unchanged.' + (a.kind === 'dump' ? ' Games can read it without amiibo keys; saving game data on it needs your key_retail.bin.' : '')].filter(Boolean).join('\n'));
    } else if (v === 'remove') {
      if (await confirmDlg(`Remove ${a.name}?`, a.kind === 'virtual' ? 'Data games saved on this virtual amiibo is lost.' : 'NEXUS’s copy is deleted, including anything games saved on it since you imported it. The file you imported is not touched.', 'Remove', true))
        await api('/api/amiibo/remove', { body: { id: a.id } }).catch(e => toast(e.message, I.warn));
    }
  },
  filter: { q: '', kind: 'all' },
  async panel(p) {
    const st = await this.load().catch(() => ({ list: [] })) || {};
    st.list = st.list || []; // an empty library arrives as null
    const es = S.settings.emulator || {};
    p.append(h('h2', {}, 'Amiibo'), h('p', { class: 'note' }, 'When a game asks you to scan an amiibo, NEXUS shows this library and gives the game the one you choose. Import dumps of your own amiibo, or create a NEXUS virtual amiibo for use in NEXUS only. NEXUS includes no amiibo data.'));
    p.append(changedMark(rowChoice('When a game scans for an amiibo', [{ v: '', l: 'Show the amiibo library' }, { v: 'off', l: 'Do nothing' }], es.amiiboPrompt || '', v => saveSettings({ emulator: { amiiboPrompt: v } })), 'emulator.amiiboPrompt'));
    p.append(rowButton('Import amiibo dump…', { sub: 'A .bin or .nfc dump of your own figure or card (NTAG215, 532/540/572 bytes). NEXUS copies it; the original is not changed.', onClick: async () => { if (await this.import(S.profile?.id)) rebuildPanel(); } }));
    p.append(rowButton('Amiibo Creator\u2026', { icon: I.plus, sub: 'Create a NEXUS virtual amiibo: search by name, character or series, or type its ID; add your own name, notes and picture. Works only in NEXUS; it is not an authenticated amiibo.', onClick: async () => { if (await this.creator(null, S.profile?.id)) rebuildPanel(); } }));
    p.append(h('h3', {}, `Library (${st.list.length})`));
    if (!st.list.length) p.append(h('p', { class: 'note' }, 'No amiibo yet. Create one with the Amiibo Creator, or import a dump of your own.'));
    else {
      const f = this.filter;
      const q = h('input', { type: 'text', class: 'am-filter', 'data-nav': true, placeholder: 'Search the library: name, character, series, ID or notes', spellcheck: 'false' });
      q.value = f.q;
      const list = h('div');
      const draw = () => {
        const t = f.q.trim().toLowerCase();
        const match = a => (f.kind === 'all' || (f.kind === 'virtual') === (a.kind === 'virtual')) && (!t || [a.name, a.character, a.series, a.gameSeries, a.amiiboId, a.notes].some(x => (x || '').toLowerCase().includes(t)));
        list.replaceChildren();
        const groups = [[S.profile?.id, `${S.profile?.name || 'You'}`], ['', 'Everyone'], ...S.profiles.filter(x => x.id !== S.profile?.id).map(x => [x.id, x.name])];
        let n = 0;
        for (const [id, label] of groups) {
          const items = st.list.filter(a => ((a.owner || '') === (id || '') || (id === '' && a.owner && !S.profiles.some(x => x.id === a.owner))) && match(a));
          if (!items.length) continue;
          list.append(h('h4', { class: 'sub' }, label));
          for (const a of items) {
            n++;
            const r = rowButton(a.name, { sub: [a.kind === 'virtual' ? 'Virtual (NEXUS only)' : a.file ? 'Imported dump' : `Imported on ${a.origin || 'another device'}`, a.character, a.series].filter(Boolean).join(' · '), value: a.amiiboId || '', onClick: async () => { await this.manage(a); rebuildPanel(); } });
            const pic = a.art ? h('span', { class: 'am-thumb', style: { backgroundImage: `url("/api/amiibo/art/${a.id}?r=${a.updated || 0}")` } }) : h('span', { class: 'am-thumb none' }, (a.character || a.name || '?').slice(0, 1).toUpperCase());
            if (a.kind === 'virtual') pic.classList.add('virtual');
            const rl = r.querySelector('.rl');
            const text = h('span', { class: 'rl-text' }, ...rl.childNodes);
            rl.replaceChildren(pic, text); rl.classList.add('with-thumb');
            list.append(r);
          }
        }
        if (!n) list.append(h('p', { class: 'note' }, 'No amiibo match.'));
      };
      q.addEventListener('input', () => { f.q = q.value; draw(); });
      p.append(q, rowChoice('Show', [{ v: 'all', l: 'All' }, { v: 'virtual', l: 'Virtual amiibo' }, { v: 'dump', l: 'Imported dumps' }], f.kind, v => { f.kind = v; draw(); }), list);
      draw();
    }
    p.append(h('h3', {}, 'Amiibo keys'));
    p.append(infoRow('Your amiibo keys (key_retail.bin)', st.keys ? 'Added' : 'Not added', 'Needed only when a game saves data on an imported encrypted dump; reading works without them, and virtual amiibo never need them. NEXUS does not include them.'));
    p.append(rowButton(st.keys ? 'Replace amiibo keys…' : 'Add amiibo keys…', { onClick: async () => {
      try { const r = await api('/api/amiibo/keys', { body: {} }); if (!r.cancelled) { toast('Amiibo keys added', I.check); rebuildPanel(); } } catch (e) { alertDlg('The keys were not added', e.message); }
    } }));
  },
  busy: false,
  // the running game asks for an amiibo
  async scanning(d) {
    if ((S.settings.emulator?.amiiboPrompt || '') === 'off' || this.busy) return;
    this.busy = true;
    try {
      const st = await this.load().catch(() => ({ list: [] }));
      const pid = S.playing?.profileId || S.profile?.id;
      await suspendGame(); // show NEXUS over the game while choosing
      const list = [...st.list].sort((a, b) => (b.owner === pid) - (a.owner === pid));
      const items = [...list.map(a => ({ v: a.id, l: `${a.name}${a.kind === 'virtual' ? ' (virtual)' : ''}${a.owner && a.owner !== pid ? ' · ' + this.ownerName(a.owner) : ''}` })),
        { v: '+import', l: 'Import an amiibo dump…' }, { v: '+virtual', l: 'Create a virtual amiibo…' }, { v: '', l: 'Not now' }];
      let v = await listPick(`${d?.title || 'The game'} is looking for an amiibo`, items);
      if (v === '+import') v = (await this.import(pid))?.id;
      else if (v === '+virtual') v = (await this.createVirtual(pid))?.id;
      if (v) {
        try { await api('/api/amiibo/place', { body: { id: v } }); toast('Amiibo placed', I.check, 2000); }
        catch (e) { await alertDlg('The amiibo was not given to the game', e.message); }
      }
      if (S.playing?.running) await resumeGame();
    } finally { this.busy = false; }
  },
};

// ---------- multi-field form dialog (account sign-in and the amiibo creator)
function formDlg({ title, text, fields, ok = 'OK', extra, validate }) {
  return new Promise(resolve => {
    const inputs = {};
    const err = h('p', { class: 'form-err', role: 'alert' });
    const body = h('div', { class: 'd-body' }, h('h2', {}, title), text && h('p', {}, text));
    for (const f of fields) {
      if (f.type === 'note') { body.append(h('p', { class: 'note' }, f.label)); continue; }
      let el;
      if (f.type === 'select') {
        el = h('select', { 'data-nav': true }, ...f.options.map(o => h('option', { value: o.v, selected: String(o.v) === String(f.value ?? '') || undefined }, o.l)));
      } else if (f.type === 'textarea') {
        el = h('textarea', { 'data-nav': true, rows: 3, maxlength: f.max || 500, placeholder: f.placeholder || '' });
        el.value = f.value || '';
      } else {
        el = h('input', { type: f.type || 'text', value: f.value || '', placeholder: f.placeholder || '', maxlength: f.max || 120, 'data-nav': true, spellcheck: 'false', autocomplete: f.autocomplete || 'off' });
      }
      inputs[f.id] = el;
      body.append(h('label', { class: 'field' }, h('span', {}, f.label), el, f.sub && h('small', {}, f.sub)));
    }
    if (extra) body.append(extra(inputs));
    body.append(err);
    const cancel = h('button', { 'data-nav': true }, 'Cancel');
    const okb = h('button', { 'data-nav': true, class: 'primary' }, ok);
    const box = h('div', { class: 'dialog form', role: 'dialog' }, body, h('div', { class: 'd-actions', 'data-row': 'strict' }, cancel, okb));
    let done = false;
    const values = () => Object.fromEntries(Object.entries(inputs).map(([k, el]) => [k, el.value]));
    const finish = v => { if (done) return; done = true; layer.close(); resolve(v); };
    const submit = async () => {
      const v = values();
      for (const f of fields) if (f.required && !String(v[f.id] || '').trim()) { err.textContent = `${f.label} is needed.`; inputs[f.id].focus(); return; }
      if (validate) {
        okb.disabled = true; const label = okb.textContent; okb.textContent = 'Please wait…';
        try { const e = await validate(v); if (e) { err.textContent = e; return; } }
        catch (e) { err.textContent = e.message; return; }
        finally { okb.disabled = false; okb.textContent = label; }
      }
      finish(v);
    };
    cancel.addEventListener('click', () => finish(null));
    okb.addEventListener('click', submit);
    Object.values(inputs).forEach(el => el.addEventListener('keydown', e => { if (e.key === 'Enter' && el.tagName !== 'TEXTAREA') { e.preventDefault(); submit(); } }));
    const layer = openLayer(box, { onBack: () => finish(null), initial: Object.values(inputs)[0] || okb });
    if (S.lastInput !== 'gamepad') setTimeout(() => Object.values(inputs)[0]?.focus(), 50);
  });
}

// ---------- NEXUS Account (account.go): sign-in, linked devices, synchronisation
const AccountUI = {
  st: null,
  STATE: { synced: 'Synced', syncing: 'Syncing…', offline: 'Waiting for connection', pending: 'Changes pending', failed: 'Sync failed', conflict: 'Conflict needs attention', held: 'Waiting for your choice', off: 'Synchronisation off', signedout: 'Not signed in', unavailable: 'Not available yet' },
  PLAT: { windows: 'Windows PC', linux: 'Linux', macos: 'Mac', android: 'Android', ios: 'iPhone / iPad', other: 'Device' },
  CATS: [
    ['profile', 'Users', 'Names, pictures, colours and themes, and controller layouts.'],
    ['playtime', 'Play time', 'Each device\u2019s play time per game, added up on every device, including corrections you make.'],
    ['library', 'Game library', 'Names you gave games, hidden games, artwork choices and each game’s controller layout.'],
    ['prefs', 'NEXUS preferences', 'Sounds, the start-up animation, shortcut keys and what Home does.'],
    ['saves', 'Game saves', 'Each user\u2019s game saves, in a private Gist of their own in your GitHub account. A save changed on two devices is never overwritten: you choose. A save that is replaced is backed up first (Save Data \u203a Backups).'],
    ['amiibo', 'Amiibo library', 'Names, details and owners of your amiibo, and NEXUS virtual amiibo. Imported amiibo dumps stay on the device you imported them on.'],
    ['emulation', 'Graphics & emulation settings', 'Global and per-game emulation settings. Off by default: a Steam Deck and a gaming PC usually need different settings. The graphics card choice never synchronises.'],
  ],
  async load() { this.st = await api('/api/account').catch(() => this.st); this.top(); return this.st; },
  update(st) {
    this.st = st; this.top();
    if (S.screen === 'settings' && curCat === 'account' && !this._editing) rebuildPanel();
  },
  // the GitHub connection on its own (account.go Status.Connection), apart from synchronisation
  CONN: { connected: 'Connected', notconnected: 'Not connected', reconnecting: 'Reconnecting…', expired: 'Authentication expired', problem: 'Temporary connection problem', syncing: 'Synchronisation in progress' },
  label(st = this.st) { if (!st) return ''; if (st.connection === 'expired') return 'Reconnect GitHub'; return st.state === 'synced' && st.lastSync ? `Synced ${this.ago(st.lastSync)}` : this.STATE[st.state] || st.state; },
  connCard(st) {
    const conn = st.connection || (st.signedIn ? 'connected' : 'notconnected');
    const who = st.provider === 'github' ? 'GitHub' : 'NEXUS account service';
    const detail = {
      connected: st.renewable ? `NEXUS renews the sign-in with ${who} by itself.` : `Signed in with ${who}.`,
      syncing: 'Sending and receiving changes now.',
      reconnecting: `Renewing the sign-in with ${who}…`,
      problem: `${who} cannot be reached just now. You stay connected; NEXUS tries again by itself.`,
      expired: `${who} no longer accepts NEXUS’s sign-in on this device. Your data on this device is untouched. Reconnect to continue synchronising.`,
      notconnected: 'Not connected on this device.',
    }[conn];
    return h('div', { class: 'acct-card' },
      h('div', { class: 'acct-ic' }, svg(I.cloud)),
      h('div', {}, h('b', {}, st.displayName || st.email || who), st.email ? h('span', {}, st.email + (st.provider === 'github' ? ' on GitHub' : '')) : null,
        h('span', { class: 'conn-state ' + conn }, this.CONN[conn] || conn), h('span', {}, detail)));
  },
  async test() {
    const r = await api('/api/account/test', { body: {} }).catch(e => ({ ok: false, error: e.message }));
    if (r.ok) toast(`Connected as ${r.login} · answered in ${r.ms} ms` + (r.renews ? ` · renews itself` : ''), I.check, 4000);
    else alertDlg('Test connection', r.error);
    await this.load(); if (S.screen === 'settings' && curCat === 'account') rebuildPanel();
  },
  async disconnect() {
    if (await confirmDlg('Disconnect GitHub on this device?', 'This device stops synchronising and leaves your device list, and NEXUS forgets the sign-in here. Your users, saves, play time and amiibo stay on this device. Connect again at any time.', 'Disconnect', true)) this.run(() => api('/api/account/signout', { body: {} }), 'Disconnected');
  },
  ago(t) { const d = Math.max(0, Date.now() / 1000 - t); return d < 60 ? 'just now' : d < 3600 ? `${Math.round(d / 60)} min ago` : d < 86400 ? `${Math.round(d / 3600)} h ago` : new Date(t * 1000).toLocaleDateString(); },
  // top bar: always shows whether you are signed in
  top() {
    const el = $('#tb-account'); if (!el || !this.st) return;
    const st = this.st;
    el.hidden = false;
    el.className = 'tb-item tb-acct ' + st.state;
    const expired = st.connection === 'expired';
    if (expired) el.className += ' expired';
    el.replaceChildren(svg(I.cloud), h('span', {}, st.signedIn || expired ? this.label() : 'Not signed in'));
    el.title = expired ? `GitHub no longer accepts NEXUS’s sign-in (${st.email}). Click to reconnect.` : st.signedIn ? `NEXUS Account: ${st.email} · ${this.label()}` : 'NEXUS Account: not signed in. Click to set up.';
    el.onclick = () => showSettings('account');
  },
  // a row for Users / User settings
  row() {
    const st = this.st || {};
    return rowButton('NEXUS Account', { value: st.signedIn ? st.email : 'Not signed in', sub: st.signedIn ? `${this.label()} · this device: ${st.deviceName}` : 'Create an account or sign in to use your users, settings and amiibo on your other PCs and a Steam Deck.', icon: I.cloud, onClick: () => showSettings('account') });
  },
  // once, on Home: what an account is for (never again after "Not now")
  onboard(b) {
    const st = this.st;
    if (!st || st.signedIn || st.onboardSeen || !st.available) return false;
    b.hidden = false; b.innerHTML = '';
    const ic = svg(I.cloud); ic.style.cssText = 'width:1.4em;height:1.4em;fill:var(--accent);flex:none';
    const go = h('button', { class: 'btn primary', 'data-nav': true }, 'Set up');
    const no = h('button', { class: 'btn', 'data-nav': true }, 'Not now');
    const seen = () => api('/api/account/onboard-seen', { body: {} }).then(st => { this.st = st; renderBanner(); }).catch(() => { });
    go.addEventListener('click', () => { seen(); showSettings('account'); });
    no.addEventListener('click', seen);
    b.append(ic, h('span', { class: 'grow' }, h('b', {}, 'Play on more than one device? '), 'Sign in with your free GitHub account to keep your users, play time, settings and amiibo the same on every PC and on a Steam Deck. NEXUS works fully without it.'), go, no);
    return true;
  },
  async run(fn, okMsg) {
    try { const r = await fn(); if (okMsg) toast(okMsg, I.check); await this.load(); rebuildPanel(); return r; }
    catch (e) { await alertDlg('NEXUS Account', e.message); await this.load(); rebuildPanel(); return null; }
  },
  async showRecovery(code, first) {
    await dialog({ title: first ? 'Your account is ready' : 'Your new recovery code', text: 'Write this recovery code down and keep it somewhere safe. It is the only way to set a new password if you forget yours, and NEXUS shows it only now.', pre: code, buttons: [{ label: 'I have saved it', value: true, primary: true }] });
  },
  // Sign in with GitHub: NEXUS shows a code; the user enters it at github.com/login/device
  // Game saves (savesync.go): status, conflicts with their two choices
  savesRows(p) {
    const box = h('div');
    p.append(h('h3', {}, 'Game saves'), box);
    const draw = ss => {
      box.replaceChildren();
      if (!ss.enabled) { box.append(h('p', { class: 'note' }, 'Game saves are not synchronised on this device (What synchronises \u203a Game saves).')); return; }
      const state = { idle: 'Not synchronised yet', running: 'Synchronising\u2026', done: 'Up to date', failed: 'Failed', offline: 'Waiting for connection' }[ss.state] || ss.state;
      const parts = [ss.last ? `Last ${this.ago(ss.last)}.` : '', ss.message || '', ss.pending ? `${ss.pending} save(s) changed here wait to be sent.` : '',
        ss.waitingFor?.length ? `Waits until ${ss.waitingFor.join(', ')} closes the game.` : '', ss.tooLarge?.length ? `Too large to synchronise: ${ss.tooLarge.join(', ')}.` : ''];
      box.append(infoRow('Game saves', ss.conflicts.length ? `${ss.conflicts.length} need your choice` : state, parts.filter(Boolean).join(' ')));
      for (const c of ss.conflicts) {
        const here = c.localModified ? new Date(c.localModified * 1000).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' }) : '?';
        const there = c.remote.modified ? new Date(c.remote.modified * 1000).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' }) : '?';
        box.append(rowButton(`${c.title} \u2014 ${c.userName}`, { icon: I.warn, sub: `Changed on this device (saved ${here}) and on ${c.remote.deviceName} (saved ${there}). Neither was overwritten.`, onClick: async () => {
          const v = await dialog({ title: `${c.title}: which save?`, text: `This device: last saved ${here}.\n${c.remote.deviceName}: last saved ${there}.\n\nThe save you do not keep is backed up first (Save Data \u203a Backups) on this device; GitHub also keeps every earlier version.`,
            buttons: [{ label: 'Decide later', value: '' }, { label: `Use ${c.remote.deviceName}\u2019s`, value: 'remote' }, { label: 'Keep this device\u2019s', value: 'local', primary: true }], cancelValue: '' });
          if (!v) return;
          try { draw(await api('/api/saves/sync/resolve', { body: { key: c.key, keep: v } })); toast('Save synchronised', I.check); } catch (e) { alertDlg('Game saves', e.message); }
        } }));
      }
    };
    api('/api/saves/sync').then(draw).catch(() => { });
  },
  // returns true once this device is signed in
  async githubSignIn() {
    if (!this.st) await this.load();
    let v, ok = false;
    try { v = await api('/api/account/github/start', { body: { deviceName: this.st.deviceName } }); } catch (e) { return alertDlg('Sign in with GitHub', e.message); }
    let open = true;
    const status = h('p', { class: 'note' }, 'Waiting for you to approve NEXUS on GitHub…');
    const openBtn = h('button', { 'data-nav': true, class: 'primary' }, 'Open github.com/login/device');
    const cancel = h('button', { 'data-nav': true }, 'Cancel');
    const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, 'Connect to GitHub'),
      h('ol', { class: 'steps' }, h('li', {}, 'On this PC, your phone or any computer, open github.com/login/device and sign in to GitHub.'), h('li', {}, 'Enter this code:')),
      h('div', { class: 'pair-code' }, v.userCode),
      h('ol', { class: 'steps', start: 3 }, h('li', {}, 'Approve “NEXUS”. NEXUS only gets access to Gists, where it keeps your sync data in a private Gist.')),
      h('p', { class: 'note' }, `The code works until ${new Date(v.expires * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}.`), status),
      h('div', { class: 'd-actions', 'data-row': 'strict' }, cancel, openBtn));
    const close = () => { if (!open) return; open = false; layer.close(); };
    openBtn.addEventListener('click', () => api('/api/app/open-link', { body: { which: 'github-device' } }).catch(e => toast(e.message, I.warn)));
    cancel.addEventListener('click', () => { api('/api/account/github/cancel', { body: {} }).catch(() => { }); close(); });
    const layer = openLayer(box, { onBack: () => cancel.click(), initial: openBtn });
    while (open) {
      await sleep(2500);
      if (!open) break;
      const r = await api('/api/account/github/status', { body: {} }).catch(e => ({ status: 'pending', error: e.message }));
      if (r.status === 'approved') { ok = true; close(); toast(`Signed in as ${r.account?.email || 'your GitHub account'}`, I.check, 4000); break; }
      if (r.status === 'denied') { close(); alertDlg('Not signed in', r.error || 'NEXUS was not approved on GitHub.'); break; }
      if (r.status === 'expired' || r.status === 'none') { close(); if (r.status === 'expired') alertDlg('The code expired', 'Choose Sign in with GitHub again for a new code.'); break; }
      if (r.error) status.textContent = r.error;
    }
    await this.load(); if (S.screen === 'settings' && curCat === 'account') rebuildPanel();
    return ok;
  },
  unavailable() {
    return alertDlg('NEXUS accounts are not available yet', 'Creating an account and signing in need the NEXUS account service on the internet, and it has not been put online for this version of NEXUS. Nothing is wrong with your PC or your data, and everything else in NEXUS works without an account.\n\nIf you run your own NEXUS account service, enter its address under Advanced \u203a Account service address.');
  },
  async create() {
    const st = this.st;
    let rec = null;
    const v = await formDlg({ title: 'Create a NEXUS account', text: 'Your e-mail address is your account name; NEXUS does not send you e-mail. Your users, settings and amiibo then follow you to every device you sign in on.', ok: 'Create account',
      fields: [{ id: 'displayName', label: 'Your name', required: true, max: 40, value: S.profile?.name || '' }, { id: 'email', label: 'E-mail address', type: 'email', required: true, autocomplete: 'username' },
        { id: 'password', label: 'Password', type: 'password', required: true, sub: 'At least 10 characters.', autocomplete: 'new-password' }, { id: 'password2', label: 'Password again', type: 'password', required: true, autocomplete: 'new-password' },
        { id: 'deviceName', label: 'Name of this device', value: st.deviceName, max: 60, sub: 'Shown in your device list, for example “Living-room PC”.' }],
      validate: async v => {
        if (v.password !== v.password2) return 'The two passwords are different.';
        const r = await api('/api/account/register', { body: { email: v.email, password: v.password, displayName: v.displayName, deviceName: v.deviceName } });
        rec = r.recoveryCode;
      } });
    if (!v) return;
    await this.showRecovery(rec, true);
    toast(`Signed in as ${v.email}`, I.check); await this.load(); rebuildPanel();
  },
  async signIn() {
    const v = await formDlg({ title: 'Sign in to NEXUS', text: 'Use the e-mail address and password of your NEXUS account. This device is added to your account’s devices.', ok: 'Sign in',
      fields: [{ id: 'email', label: 'E-mail address', type: 'email', required: true, value: this.st.email || '', autocomplete: 'username' }, { id: 'password', label: 'Password', type: 'password', required: true, autocomplete: 'current-password' },
        { id: 'deviceName', label: 'Name of this device', value: this.st.deviceName, max: 60 }],
      validate: v => api('/api/account/signin', { body: v }).then(() => null) });
    if (v) { toast(`Signed in as ${v.email}`, I.check); await this.load(); rebuildPanel(); }
  },
  async recover() {
    let rec = null;
    const v = await formDlg({ title: 'Forgot your password?', text: 'Enter the recovery code you saved when you created the account, and choose a new password. All your devices are signed out and sign in again with the new password.', ok: 'Set new password',
      fields: [{ id: 'email', label: 'E-mail address', type: 'email', required: true, value: this.st.email || '' }, { id: 'code', label: 'Recovery code', required: true, placeholder: 'XXXXXX-XXXXXX-XXXXXX-XXXXXX', max: 40 },
        { id: 'newPassword', label: 'New password', type: 'password', required: true, sub: 'At least 10 characters.' }, { id: 'deviceName', label: 'Name of this device', value: this.st.deviceName, max: 60 }],
      validate: async v => { const r = await api('/api/account/recover', { body: v }); rec = r.recoveryCode; } });
    if (!v) return;
    await this.showRecovery(rec, false);
    await this.load(); rebuildPanel();
  },
  // this device enters a code from a signed-in device, which then approves it
  async linkThis() {
    const v = await formDlg({ title: 'Link this device to your account', text: 'On a device that is already signed in (for example your PC), open NEXUS Account › Link another device. Enter the code it shows here.', ok: 'Continue',
      fields: [{ id: 'code', label: 'Code', required: true, placeholder: 'ABCD-EFGH', max: 12 }, { id: 'deviceName', label: 'Name of this device', value: this.st.deviceName, max: 60 }],
      validate: v => api('/api/account/link/claim', { body: v }).then(() => null) });
    if (!v) return;
    await this.load(); rebuildPanel(); this.waitApproval();
  },
  async waitApproval() {
    if (this._waiting) return; this._waiting = true;
    try {
      for (;;) {
        await sleep(2000);
        if (!this.st?.linking) break;
        const r = await api('/api/account/link/status', { body: {} }).catch(e => ({ status: 'pending', error: e.message }));
        if (r.status === 'approved') { toast('This device is now linked to your NEXUS account', I.check, 4000); break; }
        if (r.status === 'declined') { alertDlg('Linking was declined', 'The other device declined this request. Make a new code there and try again.'); break; }
        if (r.status === 'expired' || r.status === 'none') { if (r.status === 'expired') alertDlg('The request expired', 'Make a new code on your other device and enter it again.'); break; }
      }
    } finally { this._waiting = false; await this.load(); if (S.screen === 'settings' && curCat === 'account') rebuildPanel(); }
  },
  // a signed-in device shows a code for another device and approves it
  async linkOther() {
    let code;
    try { code = await api('/api/account/link/code', { body: {} }); } catch (e) { return alertDlg('NEXUS Account', e.message); }
    let open = true, timer = null;
    const status = h('p', { class: 'note' }, 'Waiting for the other device…');
    const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, 'Link another device'),
      h('ol', { class: 'steps' }, h('li', {}, 'On the other device (another PC, or NEXUS on your Steam Deck), open NEXUS Account.'), h('li', {}, 'Choose “Link this device with a code”.'), h('li', {}, 'Enter this code, then confirm the device here.')),
      h('div', { class: 'pair-code' }, code.code), h('p', { class: 'note' }, `The code works once and expires at ${new Date(code.expires * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}. It does not give access by itself: you approve the device here.`), status),
      h('div', { class: 'd-actions' }, h('button', { 'data-nav': true, class: 'primary' }, 'Close')));
    const close = () => { open = false; clearTimeout(timer); layer.close(); };
    box.querySelector('.d-actions button').addEventListener('click', close);
    const layer = openLayer(box, { onBack: close, initial: '.d-actions button' });
    const poll = async () => {
      if (!open) return;
      if (Date.now() / 1000 > code.expires) { status.textContent = 'This code has expired. Close this and make a new one.'; return; }
      const r = await api('/api/account/link/requests').catch(() => ({ requests: [] }));
      for (const q of r.requests || []) {
        const ok = await dialog({ title: 'Link this device?', text: `“${q.name}” (${this.PLAT[q.platform] || q.platform}, network address ${q.address}) wants to join your NEXUS account. Approve it only if it is your device and you entered the code on it just now.`, buttons: [{ label: 'Decline', value: false, danger: true }, { label: 'Approve', value: true, primary: true }], cancelValue: false });
        try { await api('/api/account/link/decide', { body: { id: q.id, approve: ok } }); } catch (e) { toast(e.message, I.warn); }
        if (ok) { toast(`${q.name} is linked to your account`, I.check, 4000); close(); await this.load(); rebuildPanel(); return; }
        status.textContent = 'Declined. You can enter the code again only with a new code.';
      }
      timer = setTimeout(poll, 2500);
    };
    timer = setTimeout(poll, 2000);
  },
  async conflict(c) {
    const d = await api('/api/account/conflict?id=' + encodeURIComponent(c.id)).catch(e => { alertDlg('NEXUS Account', e.message); return null; });
    if (!d) return;
    const flat = (v, pre = '', out = {}) => { if (v && typeof v === 'object' && !Array.isArray(v)) { for (const [k, x] of Object.entries(v)) flat(x, pre ? pre + '.' + k : k, out); } else out[pre] = v; return out; };
    const NAMES = { name: 'Name', 'theme.accent': 'Colour', 'theme.mode': 'Theme', 'avatar.preset': 'Icon', 'avatar.color': 'Icon colour', 'avatar.kind': 'Icon type', title: 'Game name', hidden: 'Hidden', series: 'Series', notes: 'Notes', owner: 'User', character: 'Character', gameSeries: 'Game series', type: 'Type' };
    const show = v => v === undefined || v === null || v === '' ? '(none)' : Array.isArray(v) ? v.join(', ') : typeof v === 'boolean' ? (v ? 'Yes' : 'No') : String(v);
    const diff = (a, b) => {
      const fa = flat(a || {}), fb = flat(b || {}), lines = [];
      for (const k of new Set([...Object.keys(fa), ...Object.keys(fb)])) if (JSON.stringify(fa[k]) !== JSON.stringify(fb[k])) lines.push(`${NAMES[k] || k}:  ${show(fa[k])}  \u2192  ${show(fb[k])}`);
      return lines.length ? lines.slice(0, 14).join('\n') + (lines.length > 14 ? `\n\u2026 and ${lines.length - 14} more` : '') : 'Only internal details differ.';
    };
    const fmt = v => v == null ? '(deleted)' : Object.entries(flat(v)).filter(([k]) => NAMES[k]).map(([k, x]) => `${NAMES[k]}: ${show(x)}`).join('\n') || 'Details';
    const buttons = d.kind === 'user'
      ? [{ label: 'Cancel', value: null }, { label: 'Keep both users', value: 'separate' }, { label: `Use ${d.otherDevice}’s`, value: 'other', primary: true }]
      : [{ label: 'Cancel', value: null }, { label: 'Keep this device’s', value: 'this' }, { label: `Use ${d.otherDevice}’s`, value: 'other', primary: true }];
    const text = d.kind === 'user' ? `A different user with the same name already exists on this device. Keep both (the account’s user is added as “… 2”), or use ${d.otherDevice}’s details.` : `${c.title} was changed on this device and on ${d.otherDevice} since they last synchronised. Nothing was overwritten. Choose which version to keep on all devices.`;
    const keep = await dialog({ title: 'Choose a version', text, pre: d.kind === 'user' ? fmt(d.other) : d.otherDeleted ? `Deleted on ${d.otherDevice}; changed on this device.` : `What differs (this device \u2192 ${d.otherDevice}):\n${diff(d.this, d.other)}`, buttons });
    if (keep) this.run(() => api('/api/account/conflict', { body: { id: c.id, keep } }), 'Resolved');
  },
  async panel(p) {
    const st = await this.load() || {};
    p.append(h('h2', {}, 'NEXUS Account & GitHub'));
    if (!st.signedIn) {
      p.append(h('p', { class: 'note' }, st.provider === 'github' ? 'Your NEXUS account keeps your users, play time, settings and amiibo the same on all your devices: this PC, another PC and, later, a Steam Deck. It uses your free GitHub account: sign in with GitHub on each device. Everything in NEXUS works without an account.' : 'A NEXUS account lets you use the same users, settings and amiibo on several devices. Sign in on each device, or link a device with a one-time code. Everything in NEXUS works without an account.'));
      if (st.connection === 'expired') {
        p.append(this.connCard(st));
        if (st.provider === 'github') {
          p.append(rowButton('Reconnect to GitHub', { icon: I.cloud, sub: `Sign in again as ${st.email}. This device keeps its place in your device list and its synchronisation state.`, onClick: () => this.githubSignIn() }));
          p.append(rowButton('Disconnect', { sub: 'Forget the GitHub connection on this device.', onClick: () => this.disconnect() }));
          return;
        }
        p.append(h('p', { class: 'banner-inline warn' }, st.ended));
      } else if (st.ended) p.append(h('p', { class: 'banner-inline warn' }, st.ended));
      p.append(infoRow(st.provider === 'github' ? 'GitHub connection' : 'Account', st.provider === 'github' ? 'Not connected' : 'Not signed in', `This device: ${st.deviceName} (${this.PLAT[st.platform] || st.platform})`));
      if (st.linking) {
        p.append(infoRow('Linking this device', 'Waiting for approval', `Approve this device on your signed-in device (account ${st.linking.account}).`));
        p.append(rowButton('Cancel linking', { onClick: () => this.run(() => api('/api/account/link/cancel', { body: {} })) }));
        this.waitApproval();
        return;
      }
      if (!st.available) {
        p.append(infoRow('Account service', 'Not set up yet', 'NEXUS accounts need the NEXUS account service on the internet. It has not been put online for this version of NEXUS yet, so you cannot create an account in this build. Nothing else in NEXUS is affected.'));
      }
      const need = fn => () => st.available ? fn() : this.unavailable();
      if (st.provider === 'github') {
        p.append(rowButton('Connect to GitHub', { icon: I.cloud, sub: 'Use your free GitHub account as your NEXUS account. NEXUS shows a code; you enter it at github.com/login/device. Do the same on your other PCs and your Steam Deck to link them. Your sync data is kept in a private Gist in your GitHub account; no NEXUS server is involved.', onClick: () => this.githubSignIn() }));
        p.append(h('p', { class: 'note' }, 'No GitHub account yet? Create one for free at github.com, then come back here.'));
        advancedSection(p, 'Advanced', box => box.append(rowButton('Use a self-hosted NEXUS account service', { value: st.custom ? st.server : 'Off', sub: 'Only if you run your own NEXUS account service (docs/ACCOUNTS.md).', onClick: async () => {
          const v = await dialog({ title: 'Account service address', text: 'Leave empty to use GitHub.', input: { value: st.custom ? st.server : '', placeholder: 'https://accounts.example.org', max: 200 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
          if (v !== null) this.run(() => api('/api/account/server', { body: { server: v } }), 'Saved');
        } })), { sub: 'A self-hosted account service instead of GitHub.' });
        return;
      }

      p.append(rowButton('Create a NEXUS account', { icon: I.plus, sub: 'With your e-mail address and a password. On this device (your PC, for example).', onClick: need(() => this.create()) }));
      p.append(rowButton('Sign in', { sub: 'You already have an account: sign in on this device too.', onClick: need(() => this.signIn()) }));
      p.append(rowButton('Link this device with a code', { sub: 'Another device is signed in: it shows a code, you enter it here and approve it there. Handy on a Steam Deck.', onClick: need(() => this.linkThis()) }));
      p.append(rowButton('Forgot your password?', { sub: 'Use your recovery code.', onClick: need(() => this.recover()) }));
      p.append(h('h3', {}, 'Advanced'));
      p.append(rowButton('Account service address', { value: st.custom ? st.server : (st.server || 'None'), sub: 'Only if you run your own NEXUS account service (see the NEXUS documentation). Must start with https://.', onClick: async () => {
        const v = await dialog({ title: 'Account service address', text: 'Leave empty to use NEXUS’s own service when it is available.', input: { value: st.custom ? st.server : '', placeholder: 'https://accounts.example.org', max: 200 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
        if (v !== null) this.run(() => api('/api/account/server', { body: { server: v } }), 'Saved');
      } }));
      return;
    }
    // signed in
    p.append(this.connCard(st));
    const acts = h('div', { class: 'acct-actions', 'data-row': 'strict' });
    const act = (label, fn, opts = {}) => { const b = h('button', { class: 'btn' + (opts.primary ? ' primary' : ''), 'data-nav': true, disabled: opts.disabled || undefined }, label); b.addEventListener('click', fn); acts.append(b); };
    act('Synchronise now', () => this.run(() => api('/api/account/sync', { body: {} }), null), { primary: true, disabled: !st.syncOn });
    act('Test connection', () => this.test());
    if (st.provider === 'github') act('Reconnect', () => this.githubSignIn());
    act('Disconnect', () => this.disconnect());
    p.append(acts);
    p.append(infoRow('This device', st.deviceName, this.PLAT[st.platform] || st.platform));
    p.append(infoRow('Synchronisation', this.STATE[st.state] || st.state, [st.lastSync ? `Last synchronised ${this.ago(st.lastSync)}.` : 'Not synchronised yet.', st.pending ? `${st.pending} change(s) on this device not sent yet.` : '', st.lastError && st.state !== 'synced' ? st.lastError : ''].filter(Boolean).join(' ')));
    if (st.holdUploads) {
      // signed in to another account than before on this device: nothing goes up until chosen
      p.append(h('div', { class: 'banner-inline warn' }, h('b', {}, `This device was signed in to ${st.holdFrom} before. `),
        `NEXUS receives ${st.email}'s data but does not send this device's users, library or settings to it until you choose.`));
      p.append(rowButton(`Send this device's data to ${st.email}`, { icon: I.cloud, onClick: async () => { if (await confirmDlg('Send this device’s data?', `Users, library details and preferences on this device are added to ${st.email}. Items that differ on both sides ask for your choice; nothing is overwritten.`, 'Send')) this.run(() => api('/api/account/hold', { body: { send: true } }), 'This device’s data will be sent'); } }));
      p.append(rowButton('Keep this device’s data here', { sub: 'Only changes you make from now on are sent.', onClick: () => this.run(() => api('/api/account/hold', { body: { send: false } }), 'Kept on this device') }));
    }
    p.append(rowToggle('Synchronise this device', st.syncOn, v => this.run(() => api('/api/account/sync/settings', { body: { on: v } })), 'Off: this device keeps its changes to itself and receives nothing until you switch it on again.'));
    if (st.provider === 'github') this.savesRows(p);
    if (st.conflicts?.length) {
      p.append(h('h3', {}, `Needs your choice (${st.conflicts.length})`));
      for (const c of st.conflicts) p.append(rowButton(c.title, { sub: `Changed here and on ${c.remoteFrom}. Nothing was overwritten.`, icon: I.warn, onClick: () => this.conflict(c) }));
    }
    p.append(h('h3', {}, 'Linked devices'));
    for (const d of st.devices || []) {
      p.append(rowButton(d.name + (d.current ? ' (this device)' : ''), { sub: `${this.PLAT[d.platform] || d.platform} · linked ${new Date(d.created * 1000).toLocaleDateString()} · last seen ${this.ago(d.lastSeen)}`, onClick: async () => {
        if (d.current) { if (await confirmDlg('Sign out on this device?', 'This device leaves your account. Your users, saves and amiibo stay on it.', 'Sign out', true)) this.run(() => api('/api/account/signout', { body: {} }), 'Signed out'); return; }
        if (await confirmDlg(`Remove ${d.name}?`, 'It is signed out of your NEXUS account at once and stops synchronising. Nothing on that device is deleted.', 'Remove', true)) this.run(() => api('/api/account/devices/remove', { body: { id: d.id } }), `${d.name} removed`);
      } }));
    }
    if (st.provider === 'github') p.append(infoRow('Link another device', 'Sign in with GitHub there', 'On your other PC or NEXUS on a Steam Deck: NEXUS Account \u203a Sign in with GitHub, with the same GitHub account. It appears in this list.'));
    else p.append(rowButton('Link another device\u2026', { icon: I.plus, sub: 'Another PC, or NEXUS for Linux on a Steam Deck. Shows a one-time code to enter there. Or simply sign in there with your e-mail and password.', onClick: () => this.linkOther() }));
    p.append(h('h3', {}, 'What synchronises'));
    for (const [id, label, sub] of this.CATS) p.append(rowToggle(label, !!st.categories?.[id], v => this.run(() => api('/api/account/sync/settings', { body: { categories: { [id]: v } } })), sub));
    p.append(h('p', { class: 'note' }, st.provider === 'github' ? 'Never synchronised: games, updates and DLC, keys and firmware, imported amiibo dumps, shader caches, screenshots and recordings.'
      : 'Never synchronised: games, updates and DLC, keys and firmware, imported amiibo dumps, shader caches, screenshots and recordings. Game saves synchronise with a GitHub NEXUS account only.'));
    if (st.history?.length) {
      p.append(h('h3', {}, 'Recent activity'));
      for (const e of [...st.history].reverse().slice(0, 8)) p.append(infoRow(e.message, new Date(e.at * 1000).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })));
    }
    if (st.provider === 'github') {
      p.append(rowButton('Rename this device', { value: st.deviceName, onClick: async () => {
        const n = await dialog({ title: 'Name of this device', input: { value: st.deviceName, max: 60 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
        if (n && n.trim()) this.run(() => api('/api/account/update', { body: { deviceName: n.trim() } }), 'Saved');
      } }));
      p.append(infoRow('Sign-in on this device', 'Stored safely', `Kept with ${st.tokenStorage}.` + (st.renewable ? ' GitHub’s sign-ins last eight hours; NEXUS renews them by itself, so you stay connected.' : '')));
      advancedSection(p, 'Advanced', box => {
        box.append(rowButton('Name, picture and password', { value: 'On GitHub', sub: 'Your GitHub account is your NEXUS account.', onClick: () => api('/api/app/open-link', { body: { which: 'github-settings' } }) }));
        box.append(rowButton('Revoke NEXUS on GitHub', { sub: 'Signs every device out for good: github.com › Settings › Applications.', onClick: () => api('/api/app/open-link', { body: { which: 'github-apps' } }) }));
      }, { sub: 'Your GitHub account settings, and revoking NEXUS on every device.' });
      p.append(rowButton('Disconnect on this device', { danger: true, sub: 'Your users, saves and amiibo stay on this device.', onClick: () => this.disconnect() }));
      return;
    }
    p.append(rowButton('Your name', { value: st.displayName, onClick: async () => {
      const n = await dialog({ title: 'Your name', input: { value: st.displayName, max: 40 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n && n.trim()) this.run(() => api('/api/account/update', { body: { displayName: n.trim() } }), 'Saved');
    } }));
    p.append(rowButton('Rename this device', { value: st.deviceName, onClick: async () => {
      const n = await dialog({ title: 'Name of this device', input: { value: st.deviceName, max: 60 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n && n.trim()) this.run(() => api('/api/account/update', { body: { deviceName: n.trim() } }), 'Saved');
    } }));
    p.append(rowButton('Change password', { sub: 'Your other devices are signed out and sign in again with the new password.', onClick: async () => {
      const v = await formDlg({ title: 'Change password', ok: 'Change', fields: [{ id: 'password', label: 'Current password', type: 'password', required: true }, { id: 'newPassword', label: 'New password', type: 'password', required: true, sub: 'At least 10 characters.' }],
        validate: v => api('/api/account/update', { body: v }).then(() => null) });
      if (v) { toast('Password changed', I.check); await this.load(); rebuildPanel(); }
    } }));
    p.append(infoRow('Sign-in on this device', 'Stored safely', `Kept with ${st.tokenStorage}. It can be revoked from any of your devices.`));
    p.append(rowButton('Sign out on this device', { danger: true, sub: 'Your users, saves and amiibo stay on this device.', onClick: async () => {
      if (await confirmDlg('Sign out?', 'This device stops synchronising and leaves your device list. Nothing on this device is deleted.', 'Sign out', true)) this.run(() => api('/api/account/signout', { body: {} }), 'Signed out');
    } }));
  },
};

const Panels = {
  account(p) { return AccountUI.panel(p); },
  amiibo(p) { return AmiiboUI.panel(p); },
  perf(p) { return renderPerfSettings(p); },
  sync(p) { return renderSync(p); },
  home(p) { homeSettingsPanel(p); },
  appearance(p) {
    const th = S.profile?.theme || {};
    p.append(h('h2', {}, 'Appearance'), h('p', { class: 'note' }, `These settings belong to ${S.profile?.name || 'the current user'}.`));
    p.append(changedMark(rowChoice('Theme', [{ v: 'dark', l: 'Dark' }, { v: 'light', l: 'Light' }], th.mode, v => themePatch({ mode: v })), 'profile:theme.mode'));
    p.append(changedMark(rowSwatches('Accent colour', ACCENTS, th.accent, c => themePatch({ accent: c })), 'profile:theme.accent'));
    p.append(h('h3', {}, 'Interface animations'));
    p.append(changedMark(rowChoice('Interface animations', [{ v: 'full', l: 'Full' }, { v: 'reduced', l: 'Reduced (faster)' }, { v: 'off', l: 'Off' }], th.animations, v => themePatch({ animations: v }).then(rebuildPanel), 'Off also stops animated wallpapers and the Home transition.'), 'profile:theme.animations'));
    p.append(changedMark(rowChoice('Home transition', [{ v: 'zoom', l: 'Zoom' }, { v: 'fade', l: 'Fade' }, { v: 'slide', l: 'Slide' }, { v: 'off', l: 'Off' }], th.homeTransition || 'zoom', v => { themePatch({ homeTransition: v }).then(() => { if (v !== 'off') { const pv = $('#set-panel .ht-demo'); pv && (pv.classList.remove('ht-' + v), void pv.offsetWidth, pv.classList.add('ht-' + v)); } }); },
      'How the Home screen appears when you press Home, including from a game (what the game does meanwhile is set in Emulation › Home button).'), 'profile:theme.homeTransition'));
    wallpaperSettings(p, th);
    p.append(h('h3', {}, 'More'));
    p.append(rowButton('Home screen layout, order and shortcuts…', { sub: 'Classic, Cards or Grid, widgets, folders and pages.', onClick: () => selectCat('home', true) }));
    p.append(rowButton('Startup animation and sounds…', { sub: 'In General. They apply to every user.', onClick: () => selectCat('general', true) }));
  },
  general(p) {
    p.append(h('h2', {}, 'General'), h('p', { class: 'note' }, 'How NEXUS starts, sounds and fills the screen. These apply to every user on this PC.'));
    p.append(h('h3', {}, 'Startup'));
    startupRows(p);
    p.append(h('h3', {}, 'Display'));
    p.append(changedMark(rowToggle('Full screen', S.system?.fullscreen ?? S.settings.display?.fullscreen, async v => { const r = await api('/api/app/fullscreen', { body: { on: v } }); if (S.system) S.system.fullscreen = r.fullscreen; }, 'NEXUS fills the screen when it starts. F11 also switches.'), 'display.fullscreen'));
    p.append(h('h3', {}, 'Typing'));
    const kbChoices = [{ v: 'auto', l: 'Automatic' }, { v: 'always', l: 'Always' }, { v: 'controller', l: 'With a controller only' }];
    if (S.platform?.platform === 'linux') kbChoices.push({ v: 'steam', l: 'Steam keyboard' });
    p.append(changedMark(rowChoice('On-screen keyboard', kbChoices, oskMode(), v => saveSettings({ input: { keyboard: v } }).then(rebuildPanel),
      oskMode() === 'steam' ? 'NEXUS asks Steam for its keyboard when you choose a text field (Steam must be running). If Steam does not answer, NEXUS\u2019s own keyboard opens.'
        : 'Automatic: on a Steam Deck and in Steam\u2019s Gaming Mode the keyboard opens whenever you choose a text field (touch, trackpad or controller); elsewhere only with a controller. A physical keyboard always types directly.'), 'input.keyboard'));
    if (S.platform?.platform === 'linux') steamRows(p);
    p.append(h('h3', {}, 'Sounds'));
    p.append(changedMark(rowToggle('Interface sounds', S.settings.sound?.enabled, v => saveSettings({ sound: { enabled: v } }).then(rebuildPanel)), 'sound.enabled'));
    p.append(changedMark(rowRange('Sound volume', { min: 0, max: 100, step: 10, value: Math.round((S.settings.sound?.volume ?? .7) * 100), fmt: v => v + '%', onChange: (v, commit) => { S.settings.sound.volume = v / 100; if (commit) { saveSettings({ sound: { volume: v / 100 } }); Sound.play('select'); } } }), 'sound.volume'));
    p.append(h('p', { class: 'note' }, 'Game sound has its own volume, in Playing games and while playing.'));
    p.append(h('h3', {}, 'Restore defaults'));
    p.append(h('p', { class: 'note' }, 'One setting: its restore button appears next to it once it is changed (controller: Options on the setting).'));
    p.append(rowButton('Restore all keybinds…', { sub: 'Keyboard shortcuts and controller buttons only.', onClick: () => resetAllKeybinds() }));
    p.append(rowButton('Reset all settings…', { danger: true, sub: 'Every setting back to its default. Users, saves, games, keys, firmware and your GitHub connection are kept.', onClick: () => resetAllSettings() }));
  },
  startup(p) { this.general(p); },
};
// Steam (Linux): app menu entry and library artwork, each only on request (steam.go)
function steamRows(p) {
  const box = h('div');
  p.append(h('h3', {}, 'Steam'), box);
  const draw = st => {
    box.replaceChildren();
    box.append(rowButton(st.menuEntry ? 'App menu entry: installed' : st.menuStale ? 'Update the app menu entry' : 'Add NEXUS to the app menu', {
      icon: I.plus, sub: st.menuEntry ? 'NEXUS appears in your app menu with its name and icon, and Steam\u2019s \u201cAdd a Non-Steam Game\u201d lists it that way.'
        : `Adds NEXUS (${st.launcher}) with its icon to your app menu, in your own folders only. Then Steam \u203a Games \u203a Add a Non-Steam Game lists it as NEXUS with its icon.`,
      onClick: async () => { try { draw(await api('/api/steam/menu-entry', { body: {} })); toast('NEXUS is in your app menu', I.check); } catch (e) { alertDlg('App menu entry', e.message); } } }));
    const sc = st.shortcuts || [];
    const art = sc.length && sc.every(x => x.artDone);
    box.append(rowButton(art ? 'Steam artwork: in place' : 'Add NEXUS artwork to Steam', {
      icon: I.image || I.plus,
      sub: sc.length ? `NEXUS is in Steam ${sc.length === 1 ? 'once' : sc.length + ' times'}. Steam does not let a program set its own artwork; NEXUS copies its library images into Steam\u2019s custom artwork folder for that entry (your own images there are kept). Restart Steam (or the Steam Deck) to see them.`
        : 'Add NEXUS to Steam first (Desktop Mode: right-click the AppImage \u203a Add to Steam, or Steam \u203a Games \u203a Add a Non-Steam Game), then come back here.',
      onClick: async () => {
        if (!await confirmDlg('Add NEXUS artwork to Steam?', 'NEXUS copies five images (library tile, wide tile, header, logo, icon) into Steam\u2019s custom artwork folder for its own entry. Steam\u2019s other files and your other games are not changed, and images you set yourself are kept.', 'Add artwork')) return;
        try { const r = await api('/api/steam/artwork', { body: {} }); draw(r.status); toast(r.installed ? 'Artwork added. Restart Steam to see it.' : 'Your own artwork is already set', I.check, 5000); } catch (e) { alertDlg('Steam artwork', e.message); }
      } }));
  };
  api('/api/steam').then(draw).catch(() => { });
}
function startupRows(p) {
  const st = S.settings.startup || {};
  const oot = st.animation === 'oot';
  const secs = oot ? 'about 6.5' : st.style === 'short' ? 'about 3' : 'about 4';
  p.append(h('p', { class: 'note' }, st.enabled ? `The power-on animation when NEXUS starts lasts ${secs} seconds; Home is prepared meanwhile. Press any key or button to skip it.` : 'No power-on animation: Home appears as soon as NEXUS has loaded.'));
  p.append(changedMark(rowToggle('Play startup animation', st.enabled, v => saveSettings({ startup: { enabled: v } }).then(rebuildPanel), 'Off: Home appears as soon as NEXUS has loaded.'), 'startup.enabled'));
  if (st.enabled) {
    p.append(changedMark(rowChoice('Startup animation', [{ v: 'nexus', l: 'NEXUS (standard)' }, { v: 'oot', l: 'OOT Launch' }], st.animation || 'nexus', v => saveSettings({ startup: { animation: v } }).then(() => { applyTheme(); rebuildPanel(); }),
      'OOT Launch: a special-edition launch in gold — a pixel emblem, a green and gold crest over a mosaic, a flash of white and gold, then Home.'), 'startup.animation'));
    if (oot) p.append(changedMark(rowToggle('OOT Launch background on Home', st.ootHome !== false, v => saveSettings({ startup: { ootHome: v } }).then(() => { applyTheme(); rebuildPanel(); }),
      'The dark gold pattern the animation ends on stays behind the Home screen. A wallpaper you chose in Appearance is used instead.'), 'startup.ootHome'));
    if (!oot) p.append(changedMark(rowChoice('Length', [{ v: 'standard', l: 'Standard' }, { v: 'short', l: 'Short' }], st.style, v => saveSettings({ startup: { style: v } }).then(rebuildPanel)), 'startup.style'));
    p.append(changedMark(rowToggle('Startup sound', st.sound, v => saveSettings({ startup: { sound: v } }).then(rebuildPanel)), 'startup.sound'));
    if (!oot) {
      p.append(changedMark(rowSwatches('Background colour', BOOT_COLORS, st.color, c => saveSettings({ startup: { color: c } }).then(rebuildPanel)), 'startup.color'));
      const prev = h('div', { class: 'boot-preview', style: { background: st.color } });
      prev.append(h('div', { class: 'bp-logo', style: { background: luminance(st.color) > .55 ? '#141414' : '#fff' } })); p.append(prev);
    }
  }
  p.append(rowButton('Preview startup animation', { sub: st.enabled ? '' : 'Shows the chosen animation once.', onClick: () => setTimeout(() => Boot.preview(st.animation || 'nexus'), 300) }));
  p.append(changedMark(rowToggle('Choose the user when NEXUS starts', S.settings.askProfileOnStart, v => saveSettings({ askProfileOnStart: v }).then(rebuildPanel), 'On: “Who’s playing?” at every start. Off: NEXUS opens with the last user.'), 'askProfileOnStart'));
}
Object.assign(Panels, {
  library(p) {
    const lib = S.settings.library || {};
    p.append(h('h2', {}, 'Game library'));
    p.append(h('h3', {}, 'Folders'));
    p.append(rowButton(S.gamesDir || 'Games', { sub: 'NEXUS Games folder (always scanned)', onClick: () => api('/api/app/open-folder', { body: { which: 'games' } }) }));
    for (const d of lib.dirs || []) p.append(rowButton(d, { sub: 'Select to remove from the library', onClick: async () => {
      if (await confirmDlg('Remove folder?', `Stop scanning ${d}? Your files are not touched.`, 'Remove')) { await api('/api/library/dirs', { body: { action: 'remove', path: d } }); await loadState(); rebuildPanel(); }
    } }));
    p.append(rowButton('Add a folder…', { icon: I.plus, onClick: () => addLibraryDir() }));
    p.append(rowButton('Scan for games now', { sub: S.library?.lastScan ? `Last scan ${fmtAgo(S.library.lastScan)} · ${S.library.count} file(s)` : '', onClick: () => rescan() }));
    if (S.library?.errors?.length) p.append(h('p', { class: 'note warn' }, S.library.errors.slice(0, 5).join('\n')));
    p.append(changedMark(rowToggle('Include sub-folders', lib.recursive, v => saveSettings({ library: { recursive: v } })), 'library.recursive'));
    p.append(changedMark(rowToggle('Scan when NEXUS starts', lib.scanOnStart, v => saveSettings({ library: { scanOnStart: v } })), 'library.scanOnStart'));
    p.append(h('h3', {}, 'Formats'));
    p.append(h('p', { class: 'note' }, 'All of these are opened by the NEXUS engine (checked against its file loader). Updates and DLC in these folders are applied automatically when you start the base game.'));
    for (const f of S.formats || []) {
      const on = (lib.formats || []).includes(f.ext);
      const fr = rowToggle(`.${f.ext.toUpperCase()}`, on, v => { const set = new Set(S.settings.library.formats); v ? set.add(f.ext) : set.delete(f.ext); saveSettings({ library: { formats: [...set] } }); }, f.label);
      const defOn = () => (SETTING_DEFAULTS?.library?.formats || []).includes(f.ext);
      addReset(fr, { changed: () => !!SETTING_DEFAULTS && (S.settings.library.formats || []).includes(f.ext) !== defOn(), def: () => defOn() ? 'On' : 'Off', run: async () => {
        const set = new Set(S.settings.library.formats); defOn() ? set.add(f.ext) : set.delete(f.ext); await saveSettings({ library: { formats: [...set] } });
      } });
      p.append(fr);
    }
    p.append(h('h3', {}, 'Artwork & names'));
    p.append(changedMark(rowToggle('Read game info with prod.keys', lib.readKeysForIds, v => saveSettings({ library: { readKeysForIds: v } }), 'Decrypts game headers locally with your own keys to read the real title, version and icon. Nothing leaves your PC.'), 'library.readKeysForIds'));
    p.append(changedMark(rowToggle('Download missing artwork', lib.onlineArtwork, v => saveSettings({ library: { onlineArtwork: v } }), 'Looks up the Title ID on the Nlib database (api.nlib.cc) when a game has no icon of its own. Only the Title ID is sent. Images are checked and saved locally.'), 'library.onlineArtwork'));
    p.append(rowButton('Artwork database', { value: hostName(lib.nlibUrl) || 'Off', sub: 'Nlib API address. Leave empty to disable.', onClick: async () => {
      const v = await dialog({ title: 'Nlib API address', input: { value: lib.nlibUrl || '', max: 200 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Default', value: 'https://api.nlib.cc' }, { label: 'Save', value: true, primary: true }] });
      if (v === null || v === undefined) return; await saveSettings({ library: { nlibUrl: v.trim() } }); rebuildPanel();
    } }));
    p.append(rowButton('Extra artwork providers', { value: String((lib.artProviders || []).length), sub: 'Optional fallback image URLs containing {tid}, one per line. Tried in order if Nlib has no icon.', onClick: async () => {
      const v = await dialog({ title: 'Extra artwork providers', text: 'Image URL templates, separated by spaces. Use {tid} for the Title ID, e.g. https://example.org/icons/{tid}.jpg', input: { value: (lib.artProviders || []).join(' '), max: 1000 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (v === null || v === undefined) return; await saveSettings({ library: { artProviders: v.split(/\s+/).filter(x => x.includes('{tid}')) } }); rebuildPanel();
    } }));
    if (S.library?.online?.last) p.append(h('p', { class: 'note' }, 'Last lookup: ' + S.library.online.last));
    p.append(rowButton('Open artwork folder', { sub: 'Drop <Title ID>.png or <Game name>.png here to set artwork.', onClick: () => api('/api/app/open-folder', { body: { which: 'artwork' } }) }));
    p.append(h('h3', {}, 'Artwork database'));
    const dbRow = rowButton('Downloaded artwork', { sub: 'Loading…' }); p.append(dbRow);
    api('/api/artwork-db').then(db => {
      dbRow.querySelector('small').textContent = `${db.cache.count} game(s), ${fmtBytes(db.cache.bytes)} kept offline in ${db.cache.dir}. Each Title ID is downloaded once.`;
      for (const b of db.builtin || []) dbRow.after(rowButton(b.title, { sub: (b.matched?.length ? `Applied to: ${b.matched.join(', ')}. ` : 'Not in your library. ') + 'Built into NEXUS; applied only when the game is identified with certainty.' }));
    }).catch(() => { });
    p.append(rowButton('Clear downloaded artwork', { danger: true, sub: 'Removes the offline copies. Your own artwork and the built-in artwork are kept; games re-download on Refresh.', onClick: async () => {
      if (await confirmDlg('Clear the artwork database?', 'Downloaded artwork will be fetched again when needed. Artwork you chose is not affected.', 'Clear', true)) { await api('/api/artwork-db/clear', { body: {} }); toast('Artwork database cleared', I.check); rebuildPanel(); }
    } }));
    moveToAdvanced(p, ['Artwork database', 'Extra artwork providers', 'Downloaded artwork', 'Clear downloaded artwork'], { sub: 'Where missing artwork comes from, and the offline copies.' });
  },
  engine(p) { return renderEngine(p); },
  emusettings(p) { renderEmuSettings(p); },
  sysfiles(p) { renderSystemFiles(p); },
  emulator(p) {
    const es = S.settings.emulator || {};
    p.append(h('h2', {}, 'Playing games'), h('p', { class: 'note' }, 'How NEXUS starts and runs games, and the keys and buttons you use while playing. Graphics, audio and system options are in Graphics & emulation; shader preparation and the shared cache in Shader management.'));
    p.append(volumeReset(rowVolume('Game volume', +(S.settings.engine?.['audio.volume'] ?? 100), sendVolume, '0% is silent, 100% is the game’s own level, up to 200% boosts quiet games (a soft limiter prevents harsh clipping). Also adjustable while playing.')));
    p.append(changedMark(rowChoice('Ask who\u2019s playing a game', [{ v: 'several', l: 'When there is more than one user' }, { v: 'always', l: 'Always' }, { v: 'never', l: 'Never (the current user plays)' }], es.whoPlays || 'several', v => saveSettings({ emulator: { whoPlays: v } }), 'Before a game starts, NEXUS asks which user is playing. Play time, saves and controller layout belong to that user.'), 'emulator.whoPlays'));
    p.append(h('h3', {}, 'NEXUS Home while playing'));
    p.append(changedMark(rowButton('Home key', { value: es.homeKey || 'Off', sub: 'Press it in a game to go back to NEXUS Home; the game keeps running. Standard: Shift+H, so H alone stays a game control. A single key that is one of the game\u2019s controls stays with the game.', onClick: async () => {
      const k = await captureKey({ mods: true }); if (k === undefined) return;
      saveSettings({ emulator: { homeKey: k } }).then(rebuildPanel);
    } }), 'emulator.homeKey'));
    p.append(changedMark(rowChoice('When you press Home in a game', [{ v: 'background', l: 'Keep the game running (muted)' }, { v: 'pause', l: 'Pause the game' }], es.homeAction || 'background', v => saveSettings({ emulator: { homeAction: v } }),
      'Keep running: the game carries on in the background without sound and ignores your controls until you choose Resume (a green dot shows it is running). Pause: the game stops where it is. Either way one press of Home is one trip to NEXUS Home.'), 'emulator.homeAction'));
    p.append(changedMark(rowToggle('Controller Home button', es.homeButton !== false, v => saveSettings({ emulator: { homeButton: v } }), 'The Xbox Guide button, the PlayStation button or a controller\u2019s Home button takes you straight to NEXUS Home (the game never sees it). Choose Resume Game to go back. If Steam or Windows uses the button for itself, use the Home key instead.'), 'emulator.homeButton'));
    p.append(h('h3', {}, 'While playing'));
    p.append(changedMark(rowChoice('Pause a game left in the background after', [{ v: 5, l: '5 minutes' }, { v: 15, l: '15 minutes' }, { v: 30, l: '30 minutes' }, { v: 60, l: '1 hour' }, { v: 0, l: 'Never' }], es.backgroundMinutes ?? 15, v => saveSettings({ emulator: { backgroundMinutes: v } }),
      'A game kept running behind NEXUS Home is paused after this long, so a forgotten game stops using your processor and graphics card. Resume continues where it was.'), 'emulator.backgroundMinutes'));
    p.append(changedMark(rowChoice('Game window', [{ v: 'inside', l: 'Inside NEXUS' }, { v: 'separate', l: 'Separate window' }], es.gameWindow || 'inside', v => saveSettings({ emulator: { gameWindow: v } }),
      'Inside NEXUS: the game is drawn in the NEXUS window, and the fullscreen key switches that window between full screen and its previous size. Separate window: the game opens in a window of its own (use this if a game shows a black picture inside NEXUS). Applies from the next game.'), 'emulator.gameWindow'));
    p.append(changedMark(rowButton('Fullscreen / window key', { value: es.fullscreenKey || 'Off', sub: 'Switches the game between full screen and a window without stopping it. If the key is one of the game\u2019s controls, it stays with the game.', onClick: async () => {
      const k = await captureKey(); if (k === undefined) return;
      saveSettings({ emulator: { fullscreenKey: k } }).then(rebuildPanel);
    } }), 'emulator.fullscreenKey'));
    p.append(changedMark(rowButton('Screenshot key', { value: es.screenshotKey || 'Off', sub: 'Saves a screenshot to Media (NEXUS data\\Media\\Screenshots). Alt+F12 by default; F12 starts and stops a recording (Settings › Screen recording).', onClick: async () => {
      const k = await captureKey({ mods: true }); if (k === undefined) return;
      if (k && k.toLowerCase() === (S.settings.recording?.key || '').toLowerCase()) { alertDlg('That shortcut is taken', `${k} starts and stops recordings. Choose another key for screenshots.`); return; }
      saveSettings({ emulator: { screenshotKey: k } }).then(rebuildPanel);
    } }), 'emulator.screenshotKey'));
    p.append(changedMark(rowToggle('Controller Share / Capture button takes screenshots', es.captureButton !== false, v => saveSettings({ emulator: { captureButton: v } }), 'The Xbox Share button, the Switch Pro Controller\u2019s Capture button or a similar button.'), 'emulator.captureButton'));
    p.append(changedMark(rowToggle('Fade into the game', es.fadeIn !== false, v => saveSettings({ emulator: { fadeIn: v } }), 'The loading screen fades out and the game fades in. Turn off if the game window stays dark or flickers with your graphics driver.'), 'emulator.fadeIn'));
    p.append(rowButton('Shader management…', { sub: 'Prepare shaders before playing, and the shader cache shared by all users of this PC.', onClick: () => selectCat('shaders', true) }));
    p.append(changedMark(rowToggle('Start games in full screen', es.fullscreen, v => saveSettings({ emulator: { fullscreen: v } }), 'The game window belongs to NEXUS; when the game ends you are back on the NEXUS home screen.'), 'emulator.fullscreen'));
    p.append(changedMark(rowToggle('Quit shortcut: hold View + Menu', es.quitCombo, v => saveSettings({ emulator: { quitCombo: v } }), 'Hold Back + Start for 2 seconds on an XInput controller to close the game.'), 'emulator.quitCombo'));
    p.append(rowButton('Restore all keybinds…', { sub: 'Home, full screen, screenshot and recording keys, the controller’s Home and Capture buttons and the quit combination, and your controller buttons.', onClick: () => resetAllKeybinds() }));
    p.append(rowButton('Save Data…', { sub: 'Every user has their own saves.', onClick: () => selectCat('savedata', true) }));
    p.append(rowButton('Keys and firmware…', { sub: 'Install or replace prod.keys and firmware.', onClick: () => selectCat('sysfiles', true) }));
    moveToAdvanced(p, ['Game window', 'Fade into the game', 'Quit shortcut: hold View + Menu', 'Pause a game left in the background after'], { sub: 'The game window, fading in, the quit shortcut and pausing a forgotten game.' });
  },
  async recording(p) { await renderRecordingSettings(p); },
  users(p) {
    p.append(h('h2', {}, 'Users'));
    p.append(AccountUI.row());
    for (const pr of S.profiles) {
      const r = rowButton(pr.name, { sub: pr.id === S.profile?.id ? 'Current user' : '', onClick: () => editProfile(pr.id) });
      r.querySelector('.rl').prepend(avatarEl(pr)); r.querySelector('.rl').style.cssText = 'flex-direction:row;align-items:center;gap:.8em';
      r.querySelector('.avatar').style.setProperty('--sz', '2.2em');
      p.append(r);
    }
    p.append(rowButton('Add user', { icon: I.plus, onClick: () => createProfileFlow() }));
    p.append(rowButton('Save Data…', { sub: 'Each user’s saves', onClick: () => selectCat('savedata', true) }));
    p.append(rowButton('Open archived data', { onClick: () => api('/api/app/open-folder', { body: { which: 'archived' } }) }));
  },
  system(p) {
    p.append(h('h2', {}, 'Storage & data'));
    p.append(h('p', { class: 'note' }, S.app.portable ? 'Portable mode (portable.txt next to NEXUS.exe): your data is stored inside the NEXUS folder. Back it up before deleting the folder.' :
      'Your users, saves, settings and artwork are stored in your Windows user folder, not in the NEXUS program folder. You can delete or replace the NEXUS folder to update it without losing anything. The engine’s keys, firmware, shader caches and console data are kept in NEXUS data\\Engine.'));
    p.append(rowButton('Open my NEXUS data folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'data' } }) }));
    p.append(rowButton('Open engine data folder', { sub: S.emulator?.dataDir || '', onClick: () => api('/api/app/open-folder', { body: { which: 'enginedata' } }) }));
    p.append(h('h3', {}, 'Storage'));
    const holder = h('div', {}, h('p', { class: 'note' }, 'Measuring…'));
    p.append(holder);
    api('/api/storage').then(s => {
      holder.innerHTML = '';
      const parts = [['Games', s.games, '#ff4f4f'], ['NEXUS engine', { bytes: s.engine.bytes + s.engineData.bytes }, '#64d2ff'], ['Saves', s.saves, '#30d158'], ['Artwork & wallpapers', { bytes: s.artwork.bytes + s.wallpaper.bytes }, '#bf5af2'], ['Logs & archive', { bytes: s.logs.bytes + s.archived.bytes }, '#ff9f0a']];
      const total = s.disk.total || 1;
      const bar = h('div', { class: 'storage-bar' }, parts.map(([, v, c]) => h('i', { style: { width: (v.bytes / total * 100) + '%', background: c } })));
      holder.append(bar, h('dl', { class: 'kv' }, parts.map(([n, v, c]) => [h('dt', {}, h('span', { style: { color: c } }, '● '), n), h('dd', {}, fmtBytes(v.bytes))]),
        h('dt', {}, 'Free space'), h('dd', {}, `${fmtBytes(s.disk.free)} of ${fmtBytes(s.disk.total)}`)));
    }).catch(() => { holder.textContent = 'Storage information unavailable.'; });
    p.append(h('h3', {}, 'Folders'));
    p.append(rowButton('Open Games folder', { sub: S.gamesDir || '', onClick: () => api('/api/app/open-folder', { body: { which: 'games' } }) }));
    p.append(rowButton('Open Saves folder', { sub: 'A folder per user (“ID - Name”), with a folder per game inside.', onClick: () => api('/api/app/open-folder', { body: { which: 'saves' } }) }));
    p.append(rowButton('Open Profiles folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'profiles' } }) }));
    p.append(rowButton('Save organisation', { sub: 'How the Saves folder is laid out', onClick: () => alertDlg('Save organisation',
      'Each user has a folder named "<profile ID> - <name>" in the Saves folder. Inside it, every game has a folder with a readable name, such as "BOTW - The Legend of Zelda Breath of the Wild" (well-known games get a short code; others use their full name).\n\n' +
      'You can copy save files into a game\'s folder yourself. When the game starts, NEXUS hands the folder to the engine under the game\'s title ID and brings it back afterwards; a hidden mapping file links each folder to its game. Please don\'t rename the game folders.\n\n' +
      'Nothing is ever deleted or overwritten: if two folders would get the same name, the second gets " (2)". Profile IDs and title IDs are never changed.') }));
    p.append(h('h3', {}, 'Maintenance'));
    p.append(rowButton('Open logs folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'logs' } }) }));
    p.append(rowButton('Open NEXUS folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'root' } }) }));
    p.append(rowButton('Reset all settings…', { danger: true, sub: 'Every setting back to its default. Users, saves, games, keys, firmware and your GitHub connection are kept.', onClick: () => resetAllSettings() }));
  },
  about(p) {
    p.append(h('h2', {}, 'About NEXUS'));
    p.append(h('dl', { class: 'kv' },
      h('dt', {}, 'Console'), h('dd', {}, S.app.model || 'NEXUS'),
      h('dt', {}, 'NEXUS version'), h('dd', {}, S.app.version),
      h('dt', {}, 'NEXUS engine'), h('dd', {}, S.engine?.installed ? (S.engine.info?.version || S.engine.active) : 'Not installed'),
      h('dt', {}, 'Program folder'), h('dd', {}, S.app.root || ''),
      h('dt', {}, 'Your data'), h('dd', {}, (S.app.data || '') + (S.app.portable ? '  (portable mode)' : ''))));
    p.append(rowButton('Where your data is kept…', { sub: 'Folders, storage and maintenance.', onClick: () => selectCat('system', true) }));
    p.append(h('h3', {}, 'Updates'));
    p.append(h('p', { class: 'note' }, 'NEXUS and its engine update together: the update is checked before it is installed, and the previous version comes back if the new one does not start. Your data is never touched.'));
    p.append(rowButton('Check for updates…', { onClick: () => selectCat('updates', true) }));
    if (S.platform?.readiness) p.append(...readinessRows(S.platform));
    p.append(h('h3', {}, 'Legal'));
    p.append(h('p', { class: 'note' }, 'NEXUS is independent software, not affiliated with or endorsed by Nintendo. NEXUS and its engine are free software under the GPL-3.0-or-later. The NEXUS Engine still contains code from open-source Switch emulation projects; their names, licences and notices are acknowledged in the Licences folder, and the complete source code is in the Source folder. NEXUS does not include games, keys or firmware: use only content you legally own.'));
    p.append(rowButton('Licences and acknowledgements', { sub: 'Open the Licences folder: NEXUS, the NEXUS Engine and every third-party component.', onClick: () => api('/api/app/open-folder', { body: { which: 'licences' } }).catch(e => toast(e.message, I.warn)) }));
  },
});
// ---------- hardware information and "Optimise for my device"
const NA = h => h === undefined || h === null || h === '' || h === 0 ? 'Not detected' : h;
function fmtMB(mb) { return mb ? (mb >= 1024 ? `${(mb / 1024).toFixed(mb >= 10240 ? 0 : 1)} GB` : `${mb} MB`) : 'Not detected'; }
async function hardwarePanel(holder, refresh = false) {
  let r;
  try { r = await api('/api/hardware' + (refresh ? '?refresh=1' : '')); } catch (e) { holder.append(h('p', { class: 'note warn' }, e.message)); return; }
  const hw = r.hardware, c = hw.cpu, m = hw.memory;
  const card = (title, rows) => h('div', { class: 'emu-card' }, h('div', { class: 'emu-head' }, h('b', {}, title)), h('dl', { class: 'kv' }, rows.map(([k, v]) => [h('dt', {}, k), h('dd', { class: v === 'Not detected' ? 'muted' : '' }, v)])));
  if (hw.source === 'override') holder.append(h('p', { class: 'note warn' }, 'Showing a test hardware profile (NEXUS_HARDWARE), not this PC.'));
  holder.append(card('Processor', [['Manufacturer', NA(c.vendor)], ['Model', NA(c.model)], ['Cores', NA(c.cores)], ['Threads', NA(c.threads)],
    ['Instruction sets', c.features?.length ? c.features.join(', ') : 'Not detected']]));
  if (!hw.gpus.length) holder.append(card('Graphics', [['Graphics card', 'Not detected']]));
  hw.gpus.forEach((g, i) => holder.append(card(hw.gpus.length > 1 ? `Graphics ${i + 1}${g.primary ? ' (main display)' : ''}` : 'Graphics', [
    ['Manufacturer', NA(g.vendor)], ['Model', NA(g.model)],
    ['Type', { discrete: 'Dedicated graphics card', integrated: 'Integrated graphics', virtual: 'Virtual', software: 'Software' }[g.kind] || 'Not detected'],
    ['Dedicated video memory', g.kind === 'integrated' ? (g.dedicatedMb ? `${fmtMB(g.dedicatedMb)} (shares system memory)` : 'Shares system memory') : fmtMB(g.dedicatedMb)],
    ['Driver version', NA(g.driver)],
    ['Graphics APIs', [g.vulkanApi ? `Vulkan ${g.vulkanApi}` : '', g.directX || ''].filter(Boolean).join(' · ') || 'Not detected'],
  ])));
  holder.append(card('Memory', [['Installed', fmtMB(m.totalMb)], ['Available', fmtMB(m.availableMb)], ['In use', m.totalMb ? `${fmtMB(m.usedMb)} (${m.loadPercent}%)` : 'Not detected']]));
  holder.append(card('System', [['Windows', NA([hw.os.name, hw.os.version].filter(Boolean).join(' '))], ['Build', NA(hw.os.build)],
    ...(hw.displays.length ? hw.displays.map(d => [d.name + (d.primary ? ' (main)' : ''), `${d.width} × ${d.height} at ${d.refreshHz} Hz` + (d.refreshRates && d.refreshRates.length > 1 ? ` (supports ${d.refreshRates.join(', ')} Hz)` : '')]) : [['Display', 'Not detected']]),
    ...hw.storage.map(d => [d.label, `${fmtMB(d.freeMb)} free of ${fmtMB(d.totalMb)}`]),
    ['Vulkan', hw.vulkan.available ? `Available (${hw.vulkan.instance || 'loader'})` : hw.vulkan.loader ? 'Driver found, no Vulkan GPU' : 'Not detected']]));
  for (const n of hw.notes || []) holder.append(h('p', { class: 'note' }, n));
  holder.append(rowButton('Detect again', { icon: I.refresh, onClick: () => { holder.replaceChildren(); hardwarePanel(holder, true); } }));
}

let optProfile = 'balanced';
// The optimiser for all games (gameKey '') or one game: shows the recommendation, applies the
// chosen changes, and can restore what was there before.
async function optimiserPanel(holder, gameKey = '', after) {
  const draw = async (plan, backup) => {
    holder.replaceChildren();
    holder.append(rowChoice('Performance profile', [{ v: 'performance', l: 'Performance (smoother, higher FPS)' }, { v: 'balanced', l: 'Balanced (recommended)' }, { v: 'quality', l: 'Quality (best image)' }], optProfile, v => { optProfile = v; load(); }));
    if (!plan) return;
    const cl = plan.class;
    holder.append(h('div', { class: 'emu-card' }, h('div', { class: 'emu-head' }, h('b', {}, 'Your device'),
      h('span', { class: 'pill ok' }, ['Basic', 'Entry-level', 'Mid-range', 'High-end', 'Very high-end'][cl.gpuTier] + ' graphics')),
      h('ul', { class: 'opt-reasons' }, cl.reasons.map(r => h('li', {}, r)), cl.display ? h('li', {}, 'Display: ' + cl.display) : '')));
    for (const n of plan.notes || []) holder.append(h('p', { class: 'note' }, n));
    const diff = plan.changes.filter(c => !c.same);
    holder.append(h('h3', {}, diff.length ? `Recommended changes (${diff.length})` : 'Already optimised for this profile'));
    const chosen = new Set(diff.map(c => c.id));
    for (const c of plan.changes) {
      if (c.same) continue;
      const r = rowToggle(c.label, true, v => { v ? chosen.add(c.id) : chosen.delete(c.id); }, `${c.fromLabel} → ${c.toLabel}. ${c.reason}`);
      holder.append(r);
    }
    const same = plan.changes.filter(c => c.same);
    if (same.length) holder.append(h('p', { class: 'note' }, 'Already set: ' + same.map(c => `${c.label} (${c.toLabel})`).join(', ') + '.'));
    for (const sk of plan.skipped || []) holder.append(h('p', { class: 'note' }, sk));
    if (diff.length) holder.append(rowButton(gameKey ? 'Apply to this game' : 'Apply to all games', { icon: I.check, sub: 'Only the changes switched on above are applied. Your previous settings are kept so you can restore them.', onClick: async () => {
      if (!chosen.size) { toast('Nothing selected', I.info); return; }
      try { const r = await api('/api/optimise', { body: { profile: optProfile, game: gameKey, only: [...chosen] } }); toast('Optimised settings applied', I.check); await draw(r.plan, r.backup); after && after(); }
      catch (e) { alertDlg('Could not apply', e.message); }
    } }));
    if (backup) holder.append(rowButton('Restore previous settings', { danger: true, sub: `From before the ${backup.profile} optimisation on ${fmtDate(backup.at)}.`, onClick: async () => {
      try { await api('/api/optimise/restore', { body: { game: gameKey } }); toast('Previous settings restored', I.check); load(); after && after(); } catch (e) { alertDlg('Could not restore', e.message); }
    } }));
  };
  const load = async () => {
    holder.replaceChildren(h('p', { class: 'note' }, 'Analysing your hardware…'));
    try { const r = await api(`/api/optimise?profile=${optProfile}&game=${encodeURIComponent(gameKey)}`); await draw(r.plan, r.backup); }
    catch (e) { holder.replaceChildren(h('p', { class: 'note warn' }, e.message)); }
  };
  await load();
}

// ---------- frame generation, DLSS and FSR 4: what this PC can use, and the files they need
// frameGenNote: what frame generation is really doing in this session, from the engine (never
// from the setting alone). Empty while it is off or the engine does not report it.
function frameGenNote(fg) {
  const why = {
    pending: 'Frame generation: starting.',
    running: 'Frame generation: running.',
    nvidia: 'Frame generation: running (NVIDIA).',
    'smooth-motion': 'Frame generation: NVIDIA Smooth Motion was requested from the graphics driver (NEXUS cannot see whether the driver applies it).',
    'unsupported-renderer': 'Frame generation is not running: it needs the Vulkan renderer.',
    'unsupported-build': 'Frame generation is not running: this engine build does not include it.',
    'unsupported-gpu': 'Frame generation is not running: your graphics card or driver lacks a Vulkan feature it needs.',
    'unsupported-surface': 'Frame generation is not running: the game window\u2019s display format is not supported.',
    'missing-lossless-dll': 'Frame generation is not running: Lossless Scaling needs your own Lossless.dll (Emulation settings \u203a Frame generation & DLSS).',
  };
  return fg ? ' ' + (why[fg] || `Frame generation: ${fg}.`) : '';
}

async function enhancementsPanel(box) {
  let en;
  try { en = await api('/api/enhancements'); } catch (e) { box.append(h('p', { class: 'note warn' }, e.message)); return; }
  const pill = (cls, label) => h('span', { class: 'pill ' + cls }, label);
  const draw = () => {
    box.replaceChildren();
    const st = f => f.ready ? pill('ok', 'Ready') : f.supported ? pill('warn', 'Needs files') : pill('bad', 'Not supported');
    box.append(h('dl', { class: 'kv enh-kv' },
      h('dt', {}, 'Graphics card'), h('dd', {}, en.gpu || 'Not detected'),
      h('dt', {}, 'Built-in frame generation'), h('dd', {}, st(en.frameGen)),
      h('dt', {}, 'Lossless Scaling frame generation'), h('dd', {}, st(en.lossless)),
      h('dt', {}, 'NVIDIA DLSS'), h('dd', {}, st(en.dlss)),
      h('dt', {}, 'AMD FSR 4'), h('dd', {}, st(en.fsr4)),
      h('dt', {}, 'NVIDIA Smooth Motion (Linux driver)'), h('dd', { title: en.smoothMotion?.why || '' }, st(en.smoothMotion || {}))));
    if (en.frameGen.supported) { const lb = h('div'); box.append(lb); losslessPanel(lb, async () => { en = await api('/api/enhancements'); draw(); }); }
    if (en.dlss.supported) { const db = h('div'); box.append(db); dlssPanel(db); }
  };
  draw();
}

// ---------- Shared shader cache (sharedcache.go, docs/SHADERS.md)
async function sharedCachePanel(box) {
  let st;
  try { st = await api('/api/shaders/cache'); } catch (e) { box.replaceChildren(h('p', { class: 'note warn' }, e.message)); return; }
  const es = S.settings.emulator || {};
  box.replaceChildren(h('h3', { class: 'set-h' }, 'Shader cache'));
  box.append(changedMark(rowToggle('Share shaders with other NEXUS users on this PC', !es.privateShaderCache, async v => {
    await saveSettings({ emulator: { privateShaderCache: !v } }); sharedCachePanel(box);
  }, 'Shaders one user\u2019s games have built are ready for every NEXUS user of this PC, so the second person to play a game has fewer stutters. Only shader data that works on any graphics card is shared; what your graphics driver builds for your card stays yours. Nothing is uploaded. Some effects are still built the first time anyone meets them, and each user\u2019s graphics driver still prepares its own copy once.'), 'emulator.privateShaderCache'));
  const games = st.titles.length;
  box.append(h('p', { class: 'note' }, `${st.shared ? 'Shared cache' : 'Your own cache'}: ${games} game${games === 1 ? '' : 's'}, ${st.records.toLocaleString()} shaders and pipelines, ${fmtBytes(st.bytes)}.`));
  for (const t of st.titles.slice(0, 12)) {
    const bits = [`${(t.graphics + t.compute).toLocaleString()} pipelines`, `${t.programs.toLocaleString()} programs`, fmtBytes(t.bytes)];
    if (t.precache) bits.push(t.doneHere ? 'prepared for your graphics card' : 'prepared shaders available');
    if (t.damaged) bits.push('cannot be read: the engine rebuilds it as you play');
    else if (!t.usable) bits.push('another cache version: not used by this engine');
    box.append(h('div', { class: 'row static' }, h('div', { class: 'rl' }, h('b', {}, t.title || t.titleId), h('small', {}, bits.join(' \u00b7 ')))));
  }
  box.append(h('p', { class: 'note' }, st.dir));
}

// ---------- Lossless Scaling setup (lossless.go, docs/LOSSLESS.md)
async function losslessPanel(box, changed) {
  const render = (st, folder) => {
    box.replaceChildren(h('h3', { class: 'set-h' }, 'Lossless Scaling frame generation'));
    const pill = { ready: ['ok', 'Ready'], update: ['ok', 'Ready'], found: ['warn', 'Detected'], notfound: ['warn', 'Not found'], invalid: ['bad', 'Cannot be used'], unsupported: ['bad', 'Not supported'] }[st.state] || ['warn', st.state];
    box.append(h('p', { class: 'lsfg-status' }, h('span', { class: 'pill ' + pill[0] }, pill[1]), ' ', st.message));
    const f = st.found || st.installed;
    const where = (label, x) => x ? [h('dt', {}, label), h('dd', {}, `${x.path} · ${fmtBytes(x.size)} · ${fmtDate(x.modified)}${x.machine ? ' · ' + x.machine : ''}${x.valid ? ` · ${x.shaders} shaders checked` : ''}`)] : '';
    if (st.installed || st.found) box.append(h('dl', { class: 'kv' }, where('In NEXUS', st.installed), st.found && st.found.sha256 !== st.installed?.sha256 ? where('In Lossless Scaling', st.found) : ''));
    const run = async (body, okMsg) => {
      try {
        const r = await api('/api/enhancements/lossless', { body });
        if (r.cancelled) return;
        if (r.error) { alertDlg('Lossless Scaling was not set up', r.error); if (r.status) render(r.status, body.path); return; }
        render(r.status, body.path); toast(okMsg || r.status.message, I.check); changed && changed();
      } catch (e) { alertDlg('Lossless Scaling was not set up', e.message); }
    };
    if (st.state === 'found') box.append(rowButton('Set up Lossless Scaling', { icon: I.check, sub: `NEXUS copies Lossless.dll from ${st.found.path} into its engine folder. Nothing else is changed.`, onClick: () => run({ path: st.found.path }, 'Lossless Scaling integration ready.') }));
    if (st.state === 'update') box.append(rowButton('Update the copy in NEXUS', { sub: 'Optional. Replaces the Lossless.dll in NEXUS with the one from your Lossless Scaling.', onClick: () => run({ path: st.found.path, replace: true }, 'Lossless Scaling updated.') }));
    if (st.state !== 'ready' && st.state !== 'update' && st.state !== 'unsupported' || folder) {
      box.append(rowButton('Locate Lossless Scaling installation…', { icon: I.folder, sub: 'Choose the Lossless Scaling folder, a Steam library or your Steam folder. NEXUS finds the files in it.', onClick: async () => {
        if (!S.app.native) { const v = await dialog({ title: 'Lossless Scaling folder', text: 'Full path of the Lossless Scaling folder or your Steam folder.', input: { max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Look there', value: true, primary: true }] }); if (!v) return; render(await api('/api/enhancements/lossless?folder=' + encodeURIComponent(v)), v); return; }
        const r = await api('/api/enhancements/lossless', { body: { pickFolder: true, locate: true } }); if (!r.cancelled) render(r, true);
      } }));
    }
    if (st.state === 'notfound' && !folder) box.append(h('p', { class: 'note' }, st.libraries?.length ? `Looked in your Steam libraries: ${st.libraries.join(', ')}.` : 'Steam was not found on this PC. If Lossless Scaling is installed elsewhere, locate it.'));
    box.append(h('p', { class: 'note' }, 'Lossless Scaling is a paid app: you need your own copy (Steam). NEXUS only copies Lossless.dll from your installation on this PC; it never downloads, includes or shares it.'));
  };
  box.replaceChildren(h('p', { class: 'note' }, 'Looking for Lossless Scaling…'));
  try { render(await api('/api/enhancements/lossless')); } catch (e) { box.replaceChildren(h('p', { class: 'note warn' }, e.message)); }
}

// ---------- Performance Monitor: the in-game bar (a Windows overlay window) and its live preview
async function perfToggleGame(key, on) {
  const pm = S.settings.perfMonitor || {};
  let games = (pm.games || []).filter(k => k !== key);
  if (on) games.push(key);
  const mode = on && pm.mode === 'off' ? 'selected' : pm.mode;
  await saveSettings({ perfMonitor: { games, mode } });
}
function perfOn(key) { const pm = S.settings.perfMonitor || {}; return pm.mode === 'all' || (pm.mode === 'selected' && (pm.games || []).includes(key)); }
async function renderPerfSettings(p) {
  const pm = S.settings.perfMonitor || {};
  p.append(h('h2', {}, 'Performance monitor'), h('p', { class: 'note' }, 'A thin strip over the game\u2019s picture while you play, in the style of a handheld\u2019s performance overlay. It stays on the picture when you move or resize the window or press F11, and hides when the game is minimised or closed. Every value is measured on this PC; what cannot be measured shows N/A. It needs borderless fullscreen or a window (exclusive fullscreen covers it).'));
  const preview = h('div', { class: 'perf-preview hud ' + (pm.position === 'bottom' ? 'bottom' : 'top') + (pm.layout === 'fps' ? ' fps' : ''), style: { '--hud-bg': `rgba(14,15,18,${(pm.opacity || 80) / 100})`, fontSize: `${(pm.scale || 100) / 100}em` } }, 'Measuring\u2026');
  p.append(h('div', { class: 'perf-stage' }, preview));
  const save = async patch => { await saveSettings({ perfMonitor: patch }); rebuildPanel(); };
  p.append(changedMark(rowChoice('Show it', [{ v: 'off', l: 'Off' }, { v: 'all', l: 'In every game' }, { v: 'selected', l: 'Only in games I choose' }], pm.mode || 'off', v => save({ mode: v }), pm.mode === 'selected' ? `${(pm.games || []).length} game(s) chosen — switch it on for a game in its Configure Game Settings, or while playing.` : ''), 'perfMonitor.mode'));
  p.append(changedMark(rowChoice('Position', [{ v: 'top', l: 'Top' }, { v: 'bottom', l: 'Bottom' }], pm.position || 'top', v => save({ position: v })), 'perfMonitor.position'));
  p.append(changedMark(rowChoice('Layout', [{ v: 'bar', l: 'Strip across the picture' }, { v: 'fps', l: 'Frame rate only, in the corner' }], pm.layout || 'bar', v => save({ layout: v })), 'perfMonitor.layout'));
  p.append(changedMark(rowRange('Background opacity', { min: 30, max: 100, step: 5, value: pm.opacity || 80, fmt: v => v + '%', onChange: v => saveSettings({ perfMonitor: { opacity: v } }).then(() => preview.style.setProperty('--hud-bg', `rgba(14,15,18,${v / 100})`)) }), 'perfMonitor.opacity'));
  p.append(changedMark(rowRange('Size', { min: 75, max: 200, step: 5, value: pm.scale || 100, fmt: v => v + '%', onChange: v => saveSettings({ perfMonitor: { scale: v } }).then(() => preview.style.fontSize = `${v / 100}em`) }), 'perfMonitor.scale'));
  p.append(h('h3', {}, 'What it shows'));
  let info; try { info = await api('/api/perf'); } catch { info = { metrics: [] }; }
  const notes = { cputemp: 'Windows does not give applications a reliable CPU temperature sensor, so this shows N/A.', gputemp: 'Measured on NVIDIA graphics cards (through the NVIDIA driver); N/A on others.', ramspeed: 'The configured memory speed of your RAM modules — a speed, not how much is in use.', fps: 'Frames the game presented in the last second.', frametime: 'Average time per presented frame over the last second.', vram: 'Graphics memory in use, of the card’s total.', gpu: 'Busiest graphics engine (3D) of your graphics card.', shaders: 'Shaders compiling now (these can make a game stutter), cached pipelines still being prepared in the background, or ready with the number compiled this session. A count, not a percentage: a game reveals new shaders as you play.' };
  const sel = pm.metrics || [];
  for (const m of info.metrics) p.append(rowToggle(m.l, sel.includes(m.v), v => save({ metrics: v ? [...sel, m.v] : sel.filter(x => x !== m.v) }), notes[m.v] || ''));
  // live preview while this page is open (stops by itself when the page closes)
  const tick = async () => {
    if (!preview.isConnected) return;
    try {
      const r = await api('/api/perf');
      let segs = r.segments || [];
      if (preview.classList.contains('fps')) segs = segs.filter(x => x.kind === 'fps' || x.kind === 'frame');
      preview.replaceChildren(...(segs.length ? segs.map(x => h('span', { class: 'hud-seg ' + x.kind + (x.label ? '' : ' joined') }, x.label && h('i', {}, x.label), h('b', {}, x.value), x.unit && h('small', {}, x.unit))) : ['Nothing selected']));
    } catch { }
    setTimeout(tick, 1000);
  };
  tick();
}

// ---------- NEXUS Sync: experimental Device Linking (only with the feature switched on)
async function renderSync(p) {
  p.append(h('h2', {}, 'NEXUS Sync'), h('p', { class: 'note warn' }, 'Experimental. Linked devices synchronise each user’s saves, profile and play time directly over your local network. Games, keys and firmware are never synchronised.'));
  let st; try { st = await api('/api/link'); } catch (e) { p.append(h('p', { class: 'note warn' }, e.message)); return; }
  p.append(infoRow('This device', st.device.name, `Listening on your local network, port ${st.port}`));
  p.append(h('h3', {}, 'Linked devices'));
  if (!st.devices.length) p.append(h('p', { class: 'note' }, 'No devices are linked yet.'));
  for (const d of st.devices) {
    const users = d.accounts.map(a => (profileById(a)?.name || a)).join(', ');
    const sub = [d.role === 'approved-by-us' ? 'Approved on this device' : 'Linked through it', users, d.lastSync ? `last synchronised ${fmtAgo(d.lastSync)}` : 'not synchronised yet', d.lastError ? '⚠ ' + d.lastError : ''].filter(Boolean).join(' · ');
    p.append(rowButton(d.name, { sub, onClick: async () => {
      const v = await dialog({ title: d.name, text: sub, buttons: [{ label: 'Close', value: null }, { label: 'Remove device', value: 'remove', danger: true }, { label: 'Synchronise now', value: 'sync', primary: true }] });
      if (v === 'sync') { try { const r = await api('/api/link/sync', { body: { device: d.deviceId } }); toast(`Synchronised: ${r.pulled} in, ${r.pushed} out${r.conflicts ? `, ${r.conflicts} conflict(s)` : ''}`, I.check, 4000); } catch (e) { alertDlg('Could not synchronise', e.message); } rebuildPanel(); }
      if (v === 'remove' && await confirmDlg(`Remove ${d.name}?`, 'It can no longer synchronise with this device. Its own copy of the saves stays on it.', 'Remove', true)) { await api('/api/link/remove', { body: { device: d.deviceId } }); rebuildPanel(); }
    } }));
  }
  if (st.conflicts.length) {
    p.append(h('h3', {}, 'Save conflicts'));
    for (const c of st.conflicts) {
      p.append(rowButton(`${profileById(c.account)?.name || c.account}: ${c.path.replace(/^saves\//, '')}`, { sub: `Changed here and on ${c.device} · ${fmtAgo(c.detected)}`, onClick: async () => {
        const v = await dialog({ title: 'Which version do you keep?', text: `This save was changed on this device and on ${c.device}. Both versions are kept until you choose.\n\nThe other version is stored in ${c.theirs}`, buttons: [{ label: 'Later', value: null }, { label: `Use ${c.device}’s`, value: 'theirs' }, { label: 'Keep this device’s', value: 'mine', primary: true }] });
        if (v) { try { await api('/api/link/resolve', { body: { id: c.id, keep: v } }); toast('Done', I.check); } catch (e) { alertDlg('Could not resolve', e.message); } rebuildPanel(); }
      } }));
    }
  }
  p.append(h('h3', {}, 'Link'));
  p.append(rowButton('Link another device…', { sub: 'Shows a one-time code for one of your users. Enter it on the other device; you approve it here.', onClick: () => linkShowCode() }));
  p.append(rowButton('Link this device to an existing user…', { sub: 'Enter the User ID and the code shown on your other device.', onClick: () => linkEnterCode() }));
  p.append(h('h3', {}, 'User IDs'));
  for (const pr of S.profiles) p.append(infoRow(pr.name, st.userIds[pr.id]));
}
async function linkShowCode(profileId) {
  let pid = profileId;
  if (!pid) {
    pid = S.profiles.length === 1 ? S.profiles[0].id : await dialog({ title: 'Which user?', buttons: [{ label: 'Cancel', value: null }, ...S.profiles.map(p => ({ label: p.name, value: p.id }))] });
    if (!pid) return;
  }
  try {
    const r = await api('/api/link/code', { body: { profile: pid } });
    const mins = Math.round((r.expires * 1000 - Date.now()) / 60000);
    await dialog({ title: 'Link another device', text: `On the other device, choose “Link an existing account” and enter:\n\nUser ID: ${r.userId}\nCode: ${r.code}\n\nThe code works once and expires in ${mins} minutes. You will be asked here to approve the device.`, buttons: [{ label: 'Done', value: true, primary: true }] });
  } catch (e) { alertDlg('Could not create a code', e.message); }
}
async function linkEnterCode() {
  const uid = await dialog({ title: 'Your NEXUS User ID', text: 'It is shown on your other device (Settings › NEXUS Sync).', input: { placeholder: 'NX-XXXX-XXXX-XXXX', max: 20 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Next', value: true, primary: true }] });
  if (!uid) return false;
  const code = await dialog({ title: 'Link code', text: 'The one-time code shown on your other device.', input: { placeholder: 'XXXX-XXXX', max: 12 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Link', value: true, primary: true }] });
  if (!code) return false;
  toast('Looking for your other device… approve the link there.', I.info, 6000);
  try {
    const r = await api('/api/link/pair', { body: { userId: uid, code } });
    await loadState();
    toast(`Linked with ${r.device.name}` + (r.syncError ? '' : ` · ${r.sync.pulled} item(s) synchronised`), I.check, 4000);
    if (r.syncError) alertDlg('Linked, but not synchronised yet', r.syncError);
    return r.device.accounts[0];
  } catch (e) { alertDlg('Could not link', e.message); return false; }
}

// ---------- global emulation settings
let emuSection = 'graphics';
async function renderEmuSettings(p) {
  p.append(h('h2', {}, 'Graphics & emulation'), h('p', { class: 'note' }, 'Global settings for every game. A game can override any of them in its Configure Game Settings. Settings the engine cannot apply are shown as not available, never as a control that does nothing.'));
  const holder = h('div'); p.append(holder);
  let view;
  try { view = await api('/api/engine-settings'); } catch (e) { holder.append(h('p', { class: 'note warn' }, e.message)); return; }
  const draw = () => {
    const keep = Nav.cur && holder.contains(Nav.cur) ? Nav.items(holder).indexOf(Nav.cur) : -1;
    holder.replaceChildren();
    const tabs = h('div', { class: 'chips', 'data-row': 'strict' });
    for (const sc of [{ v: 'hardware', l: 'Hardware' }, { v: 'optimise', l: 'Optimise for my device' }, ...view.sections.filter(x => x.v !== 'general')]) {
      const n = view.settings.filter(x => x.section === sc.v && view.global[x.id] !== undefined).length;
      const b = h('button', { class: 'chip' + (sc.v === emuSection ? ' on' : ''), 'data-nav': true }, sc.l + (n ? ` · ${n}` : ''));
      b.addEventListener('click', () => { emuSection = sc.v; draw(); Nav.setFocus($$('.chip', holder).find(x => x.classList.contains('on')), { silent: true }); });
      tabs.append(b);
    }
    holder.append(tabs);
    if (emuSection === 'hardware' || emuSection === 'optimise') {
      const box = h('div'); holder.append(box);
      if (emuSection === 'hardware') hardwarePanel(box);
      else { box.append(h('p', { class: 'note' }, 'NEXUS reads your processor, graphics card, memory and display, and recommends settings the engine really uses. You see every change before it is applied.')); const ob = h('div'); box.append(ob); optimiserPanel(ob, '', async () => { view = await api('/api/engine-settings'); }); }
      if (keep >= 0) { const it = Nav.items(holder); Nav.setFocus(it[Math.min(keep, it.length - 1)], { silent: true, scroll: false }); }
      return;
    }
    if (emuSection === 'enhance') { const eb = h('div'); holder.append(eb); enhancementsPanel(eb); }
    if (emuSection === 'graphics') holder.append(rowButton('Optimise for my device…', { icon: I.bolt || I.check, sub: 'Recommended settings for your hardware, with Performance, Balanced and Quality profiles.', onClick: () => { emuSection = 'optimise'; draw(); } }));
    settingRows(holder, view, emuSection, { mode: 'global', save: async patch => {
      try { view = await api('/api/engine-settings', { body: patch }); toast('Saved', I.check, 1200); draw(); } catch (e) { alertDlg('Could not save', e.message); }
    } });
    const n = Object.keys(view.global).length;
    holder.append(h('h3', {}, ''));
    holder.append(rowButton('Restore defaults', { danger: true, disabled: !n, sub: 'All global settings go back to the engine defaults. Game-specific settings are kept.', onClick: async () => {
      if (await confirmDlg('Restore global defaults?', 'All global settings go back to the engine defaults.', 'Restore', true)) { view = await api('/api/engine-settings', { body: { resetAll: true } }); draw(); }
    } }));
    if (keep >= 0) { const it = Nav.items(holder); Nav.setFocus(it[Math.min(keep, it.length - 1)], { silent: true, scroll: false }); }
  };
  draw();
}

// ---------- system files (keys + firmware)
function fmtDay(ts) { return ts ? new Date(ts * 1000).toLocaleDateString([], { dateStyle: 'medium' }) : ''; }
async function sysAction(path, body, { pickText, okMsg } = {}) {
  try {
    if (!S.app.native && body && !body.path && !body.prod) {
      const v = await dialog({ title: pickText || 'Location', text: 'Full path on this PC.', input: { value: '', max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'OK', value: true, primary: true }] });
      if (!v) return null;
      if (path.endsWith('/keys')) body.prod = v; else body.path = v;
    }
    const r = await api(path, { body });
    if (r?.cancelled) return null;
    if (okMsg) toast(okMsg, I.check);
    return r;
  } catch (e) {
    const errs = e.data?.keys?.errors;
    alertDlg('That file was not accepted', errs?.length ? errs.join('\n') : e.message);
    return null;
  } finally { if (curCat === 'sysfiles') rebuildPanel(); }
}
async function renderSystemFiles(p) {
  p.append(h('h2', {}, 'System files'));
  p.append(h('p', { class: 'note' }, 'NEXUS keeps its own system files in its engine data folder; it never needs another emulator. Install the keys and firmware dumped from your own console below.'));
  const holder = h('div'); p.append(holder);
  let st;
  try { st = await api('/api/system-files'); } catch (e) { holder.append(h('p', { class: 'note warn' }, e.message)); return; }
  S.sysfiles = st;
  const k = st.keys || {}, fw = st.firmware, job = st.job;
  const card = (title, pill, cls, rows) => h('div', { class: 'emu-card' }, h('div', { class: 'emu-head' }, h('b', {}, title), h('span', { class: 'pill ' + cls }, pill)), rows);
  holder.append(h('div', { class: 'sys-summary ' + (st.ready ? 'ok' : 'warn') }, h('b', {}, st.ready ? 'System files are installed and verified' : 'System files are needed to play commercial games'),
    h('span', {}, st.detected ? `Detected system version: ${st.detected}` : 'No system version detected yet.')));
  if (!st.ready) holder.append(h('ol', { class: 'steps' },
    h('li', { class: k.valid ? 'done' : '' }, 'Install the prod.keys file dumped from your own console (for example with Lockpick_RCM).'),
    h('li', { class: fw && st.firmwareState === 'ok' ? 'done' : '' }, 'Install the firmware dumped from the same console: the folder of .nca files, or a .zip of it.'),
    h('li', {}, 'NEXUS checks both and installs them into the NEXUS engine automatically.')));
  if (job && (job.active || (job.done && job.error))) {
    holder.append(h('p', { class: 'note' + (job.error ? ' warn' : '') }, job.error || job.message));
    if (job.active) holder.append(h('div', { class: 'progress' }, h('i', { style: { width: Math.round((job.progress || 0) * 100) + '%' } })));
  }
  // keys
  const kState = !k.present ? ['Not installed', 'bad'] : k.valid ? ['Installed · valid', 'ok'] : ['Invalid', 'bad'];
  holder.append(card('Prod keys', kState[0], kState[1], h('dl', { class: 'kv' },
    h('dt', {}, 'Status'), h('dd', {}, !k.present ? 'No prod.keys installed' : k.valid ? `${k.count} keys, header key present, ${k.keyAreaKeys} key generation(s)` : (k.errors || []).join(' ')),
    k.present ? [h('dt', {}, 'Covers firmware'), h('dd', {}, k.supportsUpTo ? `up to ${k.supportsUpTo}` : 'unknown (no master keys listed)')] : '',
    k.present ? [h('dt', {}, 'Checked against firmware'), h('dd', {}, k.verified || '—')] : '',
    k.present ? [h('dt', {}, 'title.keys'), h('dd', {}, k.titleKeys ? `${k.titleKeys} entries` : 'not installed (optional)')] : '',
    k.installedAt ? [h('dt', {}, 'Installed'), h('dd', {}, fmtDay(k.installedAt))] : '')));
  for (const w of k.warnings || []) holder.append(h('p', { class: 'note warn' }, w));
  if (!k.present) for (const e of (st.engines || []).filter(e => e.keys !== 'missing')) holder.append(rowButton(`Use the prod.keys already in the ${e.name} data folder`, { sub: 'Found from an earlier setup. They are validated first.',
    onClick: () => sysAction('/api/system-files/keys', { engine: e.engine, prod: '-' }, { okMsg: 'prod.keys imported and verified' }) }));
  holder.append(rowButton(k.present ? 'Replace prod.keys…' : 'Install prod.keys…', { sub: 'Select the prod.keys file from your PC. It is checked before anything is replaced; the old file is archived.', disabled: job?.active,
    onClick: () => sysAction('/api/system-files/keys', {}, { pickText: 'prod.keys location', okMsg: 'prod.keys installed and verified' }) }));
  if (k.valid) holder.append(rowButton(k.titleKeys ? 'Replace title.keys…' : 'Add title.keys (optional)…', { sub: 'Only needed for some games with personalised tickets.',
    onClick: () => sysAction('/api/system-files/title-keys', {}, { pickText: 'title.keys location', okMsg: 'title.keys installed' }) }));
  // firmware
  const fState = !fw ? ['Not installed', 'bad'] : st.firmwareState === 'ok' ? ['Recognised', 'ok'] : ['Not recognised', 'warn'];
  holder.append(h('h3', {}, ''));
  holder.append(card('Firmware', fState[0], fState[1], h('dl', { class: 'kv' },
    h('dt', {}, 'Installed version'), h('dd', {}, fw && st.firmwareState === 'ok' ? fw.version : fw ? 'Unknown' : '—'),
    fw?.title ? [h('dt', {}, 'Identified as'), h('dd', {}, fw.title)] : '',
    fw ? [h('dt', {}, 'Contents'), h('dd', {}, `${fw.ncas} files · ${fmtBytes(fw.bytes)}`)] : '',
    fw?.installedAt ? [h('dt', {}, 'Installed'), h('dd', {}, fmtDay(fw.installedAt))] : '',
    st.firmwareState && st.firmwareState !== 'ok' && st.firmwareState !== 'not-installed' ? [h('dt', {}, 'Problem'), h('dd', {}, st.firmwareState.replace(/^unrecognised: /, ''))] : '',
    h('dt', {}, 'Updates'), h('dd', {}, st.updateCheck))));
  if (!fw && k.valid) for (const e of (st.engines || []).filter(e => e.fwState !== 'missing')) holder.append(rowButton(`Use the firmware already in the ${e.name} data folder${e.firmware ? ' (' + e.firmware + ')' : ''}`, { sub: 'Copied into NEXUS and verified.', disabled: job?.active,
    onClick: () => sysAction('/api/system-files/firmware', { engine: e.engine, path: '-' }) }));
  const fwLabel = fw ? 'Update or replace firmware' : 'Install firmware';
  holder.append(rowButton(fwLabel + ' from a folder…', { sub: k.valid ? 'Select the folder that contains the firmware .nca files.' : 'Install prod.keys first: they are needed to verify the firmware.', disabled: !k.valid || job?.active,
    onClick: () => sysAction('/api/system-files/firmware', { zip: false }, { pickText: 'Firmware folder' }) }));
  holder.append(rowButton(fwLabel + ' from a .zip…', { disabled: !k.valid || job?.active, onClick: () => sysAction('/api/system-files/firmware', { zip: true }, { pickText: 'Firmware .zip' }) }));
  // engines
  holder.append(h('h3', {}, 'Installed in the NEXUS engine'));
  for (const e of st.engines || []) {
    const kc = { match: ['Keys up to date', 'ok'], different: ['Different keys', 'warn'], missing: ['No keys', 'bad'] }[e.keys];
    const fc = e.fwState === 'match' ? [`Firmware ${e.firmware}`, 'ok'] : e.fwState === 'different' ? [`Firmware ${e.firmware}`, 'warn'] : e.fwState === 'missing' ? ['No firmware', 'bad'] : ['Firmware unreadable', 'warn'];
    holder.append(h('div', { class: 'row info' }, h('span', { class: 'rl' }, h('span', {}, e.name + (e.installed ? '' : ' (not installed)')), h('small', {}, e.firmwareDir)),
      h('span', { class: 'rv' }, h('span', { class: 'pill ' + kc[1] }, kc[0]), h('span', { class: 'pill ' + fc[1] }, fc[0]))));
  }
  holder.append(rowButton('Reinstall system files into the engine', { sub: 'Copies NEXUS’s verified keys and firmware into the engine data folder again. A different copy there is archived first.', disabled: !k.present || job?.active,
    onClick: async () => { const r = await api('/api/system-files/sync', { body: {} }).catch(e => { alertDlg('Could not install', e.message); return null; }); if (r) { toast('System files installed into the engine', I.check); if (r.warnings?.length) alertDlg('Not everything was installed', r.warnings.join('\n')); rebuildPanel(); } } }));
  holder.append(h('h3', {}, 'Storage'));
  holder.append(rowButton('Open the NEXUS system files folder', { sub: st.root + ' — separate from the engine and from save data.', onClick: () => api('/api/app/open-folder', { body: { which: 'systemfiles' } }) }));
  holder.append(rowButton('Remove prod.keys…', { danger: true, disabled: !k.present, sub: 'Moved to NEXUS data\\Archived, not deleted.', onClick: () => removeSys('keys') }));
  holder.append(rowButton('Remove firmware…', { danger: true, disabled: !fw, sub: 'Moved to NEXUS data\\Archived, not deleted.', onClick: () => removeSys('firmware') }));
  holder.append(h('p', { class: 'note' }, 'NEXUS never includes keys or firmware. Use only files dumped from a console you own.'));
}
async function removeSys(which) {
  const v = await dialog({ title: `Remove ${which === 'keys' ? 'prod.keys' : 'firmware'}?`, text: 'The files are moved to NEXUS data\\Archived so you can restore them. Also remove the copy in the engine data folder?',
    buttons: [{ label: 'Cancel', value: null }, { label: 'NEXUS copy only', value: 'nexus' }, { label: 'NEXUS and engine', value: 'all', danger: true }] });
  if (!v) return;
  try { await api('/api/system-files/remove', { body: { which, engines: v === 'all' } }); toast('Removed (archived)', I.check); } catch (e) { alertDlg('Could not remove', e.message); }
  rebuildPanel();
}

// ---------- NEXUS engine
async function refreshEngine() { try { S.engine = await api('/api/engine'); S.emulator = S.engine; } catch { } }
const SYS_STATUS = { 'core': 'Engine core', 'experimental': 'Experimental' };
async function renderEngine(p) {
  await refreshEngine();
  const e = S.engine || {}, info = e.info || {};
  p.append(h('h2', {}, 'NEXUS Engine'), h('p', { class: 'note' }, 'NEXUS runs games itself: the NEXUS engine is loaded inside NEXUS, opens the game window and returns you to the home screen when the game ends. No other emulator program is started.'));
  const pill = (ok, label) => h('span', { class: 'pill ' + ok }, label);
  p.append(h('dl', { class: 'kv' },
    h('dt', {}, 'Status'), h('dd', {}, e.state?.running ? pill('ok', 'Running a game') : e.installed ? pill('ok', 'Ready') : pill('bad', 'Not installed')),
    h('dt', {}, 'Engine'), h('dd', {}, e.active === 'bundled' ? `Included with NEXUS — version ${e.activeLabel}` : e.active ? `Added build ${e.activeLabel}` : 'Missing from the NEXUS folder'),
    h('dt', {}, 'Location'), h('dd', {}, e.engineDir || '—'),
    info.upstream ? [h('dt', {}, 'Engine source'), h('dd', {}, `${info.upstream.describe || ''} ${(info.upstream.commit || '').slice(0, 10)} · acknowledgements in System › Legal`.trim())] : '',
    h('dt', {}, 'Interface'), h('dd', {}, `version ${e.apiVersion}`),
    h('dt', {}, 'Keys'), h('dd', {}, e.keys ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'Firmware'), h('dd', {}, e.firmware ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'VC++ runtime'), h('dd', {}, e.vcRuntime ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'Data folder'), h('dd', {}, e.dataDir || '')));
  if (!e.installed && e.problem) p.append(h('p', { class: 'note warn' }, e.problem));
  if (!e.vcRuntime) p.append(h('p', { class: 'note warn' }, 'The Microsoft Visual C++ runtime the engine needs was not found (it normally ships in NEXUS\\Engine\\Libraries). Install it from Microsoft: aka.ms/vs/17/release/vc_redist.x64.exe'));
  if (e.vulkan === false) p.append(h('p', { class: 'note warn' }, 'No Vulkan graphics driver was found. Install the latest driver for your graphics card, or choose OpenGL in Settings › Graphics & emulation › Graphics.'));
  if (e.infoError) p.append(h('p', { class: 'note warn' }, 'The engine did not describe itself: ' + e.infoError));
  if (e.job?.active || e.job?.error) {
    p.append(h('p', { class: 'note' + (e.job.error ? ' warn' : '') }, e.job.error || e.job.message));
    if (e.job.active) p.append(h('div', { class: 'progress' }, h('i', { style: { width: Math.round((e.job.progress || 0) * 100) + '%' } })));
  }

  p.append(h('h3', {}, 'Supported systems'));
  const systems = info.systems?.length ? info.systems : [];
  if (!systems.length) p.append(h('p', { class: 'note' }, e.installed ? 'This engine build does not list its systems.' : 'Shown once an engine build is installed.'));
  for (const sy of systems) p.append(infoRow(sy.name, SYS_STATUS[sy.status] || sy.status, sy.id === 'switch2' ? 'Switch 2 support comes from experimental upstream work and has not been tested with games in NEXUS. NEXUS reports what actually happens per game under Compatibility.' : 'Compatibility differs per game; NEXUS records the result of every session.'));
  if (info.renderers?.length) p.append(infoRow('Renderers', info.renderers.join(', ')));
  if (info.components?.length) p.append(rowButton('Engine components and licences', { value: String(info.components.length), sub: 'Open-source projects built into the engine.', onClick: () => alertDlg('Engine components', info.components.join('\n') + '\n\nThe engine is free software under the GPL-3.0-or-later. Licence texts and the source code offer are in the Licences folder.') }));

  p.append(h('h3', {}, 'Engine builds'));
  const builds = e.builds || [];
  p.append(h('p', { class: 'note' }, 'The NEXUS engine comes inside the NEXUS folder (Engine) and is updated together with NEXUS — nothing to install. Developers can add other engine builds here; the included engine is always kept and you can switch back to it.'));
  if (!builds.length) p.append(h('p', { class: 'note warn' }, 'This NEXUS folder has no engine. Extract the complete NEXUS ZIP again, keeping all its folders together.'));
  for (const b of builds) {
    const m = b.manifest || {};
    const sub = b.problem ? b.problem : [m.upstreamBase ? `based on ${m.upstreamBase}` : '', m.commit ? `source ${m.commit.slice(0, 10)}` : '', b.installed ? `installed ${fmtAgo(b.installed)}` : ''].filter(Boolean).join(' · ');
    p.append(rowButton((b.bundled ? `Included engine ${m.version || ''}` : `Added build ${b.version}`) + (b.active ? ' (in use)' : ''), { sub, value: b.active ? '✓' : '', onClick: () => buildMenu(b) }));
  }
  p.append(rowButton('Add an engine build (developers)…', { icon: I.plus, sub: 'A nexus-engine-…-win64.zip from the engine CI. The included engine is kept.', disabled: e.state?.running, onClick: () => installEngine() }));
  p.append(rowButton('Install from a download link…', { sub: 'https:// address of a build zip, optionally with its published SHA-256.', disabled: e.state?.running, onClick: () => installEngineURL() }));
  if (e.active && e.active !== 'bundled') p.append(rowButton('Go back to the previous build', { disabled: e.state?.running, onClick: async () => { try { S.engine = await api('/api/engine/rollback', { body: {} }); toast('Previous engine build activated', I.check); restartNote(); } catch (err) { alertDlg('Could not go back', err.message); } rebuildPanel(); } }));
  p.append(rowButton('Open engine data folder', { sub: 'Keys, firmware, console data, shader caches, logs.', onClick: () => api('/api/engine/open-folder', { body: {} }) }));
  p.append(rowButton('Open builds folder', { onClick: () => api('/api/engine/open-folder', { body: { which: 'builds' } }) }));
  p.append(h('p', { class: 'note' }, 'Graphics, audio and system options are in Graphics & emulation. Keys and firmware are in System files.'));
}
function restartNote() { if (S.engine?.state?.running === false) toast('The new build is used for the next game', I.info, 3000); }
async function buildMenu(b) {
  const name = b.bundled ? 'the included engine' : `engine ${b.version}`;
  const v = await dialog({ title: b.bundled ? 'Included NEXUS engine' : `Engine ${b.version}`, text: b.problem || (b.active ? `NEXUS uses ${name}.` : `Use ${name} for the next game?`), buttons: [{ label: 'Close', value: null }, ...(b.active ? [] : [...(b.bundled ? [] : [{ label: 'Remove', value: 'remove', danger: true }]), { label: 'Use it', value: 'activate', primary: true }])] });
  try {
    if (v === 'activate') { S.engine = await api('/api/engine/activate', { body: { version: b.version } }); toast(`Engine ${b.version} activated`, I.check); restartNote(); }
    if (v === 'remove' && await confirmDlg(`Remove engine ${b.version}?`, 'The build is moved to NEXUS data\\Archived.', 'Remove', true)) { S.engine = await api('/api/engine/remove', { body: { version: b.version } }); }
  } catch (err) { alertDlg('Could not change the engine build', err.message); }
  rebuildPanel();
}
async function installEngine() {
  let path = '';
  if (!S.app.native) { path = await dialog({ title: 'Install an engine build', text: 'Full path to nexus-engine-<version>-win64.zip.', input: { max: 400 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Install', value: true, primary: true }] }); if (!path) return; }
  try { const r = await api('/api/engine/install', { body: { path } }); if (r.cancelled) return; S.engine = r; await loadState(); toast('Engine build installed', I.check); }
  catch (err) { alertDlg('The engine build was not installed', err.message); }
  if (S.screen === 'settings') rebuildPanel();
}
async function installEngineURL() {
  const url = await dialog({ title: 'Download an engine build', text: 'https:// link to nexus-engine-<version>-win64.zip', input: { max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Next', value: true, primary: true }] });
  if (!url) return;
  const sha = await dialog({ title: 'SHA-256 (optional)', text: 'Paste the published SHA-256 of the zip to verify the download, or leave empty.', input: { max: 140 }, buttons: [{ label: 'Skip', value: '' }, { label: 'Verify', value: true, primary: true }] });
  toast('Downloading the engine build…', I.refresh);
  try { S.engine = await api('/api/engine/install', { body: { url: url.trim(), sha256: (sha || '').trim() } }); await loadState(); toast('Engine build installed', I.check); }
  catch (err) { alertDlg('The engine build was not installed', err.message); }
  if (S.screen === 'settings') rebuildPanel();
}
async function addLibraryDir() {
  let path = null;
  if (S.app.native) { try { path = (await api('/api/app/pick-folder', { body: {} })).path; } catch (e) { if (e.message === 'cancelled') return; } }
  if (!path) path = await dialog({ title: 'Add a games folder', text: 'Enter the full path of a folder that contains your game dumps.', input: { placeholder: 'D:\\Switch\\Games', max: 400 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Add', value: true, primary: true }] });
  if (!path) return;
  try { await api('/api/library/dirs', { body: { action: 'add', path } }); await loadState(); if (S.screen === 'settings') rebuildPanel(); toast('Folder added. Scanning…', I.folder); }
  catch (e) { alertDlg('Could not add folder', e.message); }
}

// ---------- controllers
const Controllers = {
  show() {
    this.render();
    show('controllers', { initial: Nav.items($('#ctl-body'))[0] });
  },
  refreshList() { if (S.screen === 'controllers' && !Mapper.listening) { const idx = Nav.items($('#ctl-body')).indexOf(Nav.cur); this.render(); const n = Nav.items($('#ctl-body')); if (idx >= 0) Nav.setFocus(n[Math.min(idx, n.length - 1)], { silent: true }); } },
  render() {
    const body = $('#ctl-body'); body.innerHTML = '';
    const c = S.profile?.controls || {};
    const left = h('div', {}), right = h('div', {});
    left.append(h('h3', { class: 'set-h' }, 'Controllers in NEXUS menus'));
    if (!S.pads.length) left.append(h('div', { class: 'pad-card' }, h('h4', {}, 'No controller active in the menus yet'), h('small', {}, 'Press any button on a controller to use it in NEXUS menus. Xbox, PlayStation and most USB/Bluetooth pads work. Keyboard: arrows to move, Enter = OK, Esc = Back, X = Options.')));
    for (const p of S.pads) {
      const info = PadInfo.of(p.index) || {};
      const card = h('div', { class: 'pad-card' + (PadInfo.active()?.index === p.index ? ' active' : ''), dataset: { pad: p.index } }, h('h4', {}, info.name || 'Controller', PadInfo.active()?.index === p.index && S.pads.length > 1 ? h('span', { class: 'pill ok' }, 'In use') : ''), h('small', {}, `Slot ${p.index + 1}${info.exact === false && info.family === 'xbox' ? ' \u00b7 connected through XInput: the exact model is shown when only one Xbox controller is connected' : ''}`),
        h('div', { class: 'pad-btns' }, Array.from({ length: 17 }, (_, i) => h('span', { dataset: { b: i } }, i))),
        h('div', { class: 'axes' }, h('div', { class: 'stick', dataset: { s: 0 } }, h('i')), h('div', { class: 'stick', dataset: { s: 1 } }, h('i'))));
      left.append(card);
    }
    // the controller for NEXUS menus: kept per user (and across restarts) by the controller's
    // identity, so it is found again when it reconnects
    const infos = S.padInfo || [];
    const padChoices = [{ v: '', l: 'Any controller' }, ...infos.map(i => ({ v: i.key, l: `${i.name} (slot ${i.index + 1})` }))];
    const prefOn = infos.some(i => i.key === c.preferredPad) || S.pads.some(p => p.id === c.preferredPad);
    if (c.preferredPad && !prefOn) padChoices.push({ v: c.preferredPad, l: `${c.preferredPadName || 'Chosen controller'} (not connected: any controller works until it is back)` });
    const curPref = infos.find(i => i.key === c.preferredPad) ? c.preferredPad : (infos.find(i => S.pads.find(p => p.index === i.index)?.id === c.preferredPad)?.key || c.preferredPad || '');
    left.append(rowChoice('Controller for NEXUS menus', padChoices, curPref, v => ctlPatch({ preferredPad: v, preferredPadName: infos.find(i => i.key === v)?.name || '' }),
      infos.length > 1 ? 'Only this controller moves around NEXUS menus. In games, players are set below.' : ''));
    left.append(changedMark(rowToggle('Vibration', c.vibration, v => { ctlPatch({ vibration: v }); if (v) setTimeout(() => Pad.rumble(250, .8), 100); }), 'profile:controls.vibration'));
    left.append(changedMark(rowRange('Repeat speed', { min: 60, max: 300, step: 20, value: c.keyRepeatMs || 140, fmt: v => v + ' ms', onChange: (v, commit) => { if (commit) ctlPatch({ keyRepeatMs: v }); } }), 'profile:controls.keyRepeatMs'));

    right.append(h('h3', { class: 'set-h' }, 'NEXUS button mapping'));
    right.append(changedMark(rowChoice('Confirm button', [{ v: 'south', l: `Bottom (${PadInfo.label(0)})` }, { v: 'east', l: `Right (${PadInfo.label(1)})` }], c.confirmButton, v => {
      const m = { ...c.mapping }; if (v === 'east') { m.confirm = 1; m.back = 0; } else { m.confirm = 0; m.back = 1; }
      ctlPatch({ confirmButton: v, mapping: m });
    }), 'profile:controls.confirmButton'));
    const ACT = [['confirm', 'OK / Select'], ['back', 'Back'], ['options', 'Options'], ['search', 'Search'], ['prevTab', 'Previous tab'], ['nextTab', 'Next tab'], ['menu', 'Power menu'], ['home', 'Home']];
    // a button's default follows the confirm-button choice (Right swaps OK and Back)
    const defIdx = a => { const d = { confirm: 0, back: 1, options: 3, search: 2, prevTab: 4, nextTab: 5, menu: 9, home: 16, view: 8 }; if (c.confirmButton === 'east') { d.confirm = 1; d.back = 0; } return d[a]; };
    for (const [a, l] of ACT) {
      const idx = c.mapping?.[a]; const r = rowButton(l, { value: idx == null ? '\u2014' : PadInfo.label(idx), sub: '', onClick: () => Mapper.start(a, l) });
      if (idx != null) r.querySelector('.rv').prepend(PadInfo.glyph(idx, 'key inline'));
      addReset(r, { changed: () => (S.profile?.controls?.mapping?.[a]) !== defIdx(a), def: () => PadInfo.label(defIdx(a)), run: async () => {
        const m = { ...S.profile.controls.mapping }, want = defIdx(a);
        const clash = Object.keys(m).find(k => m[k] === want && k !== a); // the button another action has: they swap, as when mapping
        if (clash) m[clash] = m[a];
        m[a] = want;
        await ctlPatch({ mapping: m });
      } });
      right.append(r);
    }
    right.append(rowButton('Reset menu buttons', { sub: 'OK, Back, Options, Search, tabs, Power and Home back to the standard buttons.', onClick: async () => {
      if (await confirmDlg('Reset the menu buttons?', 'The buttons that move around NEXUS menus go back to the standard ones (OK on the bottom button). Your controller layout for games is not changed.', 'Reset', true))
        ctlPatch({ confirmButton: 'south', mapping: { confirm: 0, back: 1, options: 3, search: 2, prevTab: 4, nextTab: 5, menu: 9, home: 16, view: 8 } });
    } }));
    right.append(rowButton('Restore all keybinds…', { sub: 'Keyboard shortcuts, controller Home and Capture buttons, menu buttons and the controller layout for games.', onClick: () => resetAllKeybinds() }));
    right.append(h('h3', { class: 'set-h' }, 'In-game controls'));
    const ips = S.emulator?.inputProfiles || [];
    right.append(changedMark(rowChoice('Controller profile for games', [{ v: '', l: 'Automatic: first connected controller' }, ...ips.map(x => ({ v: x, l: x }))], c.emuInputProfile || '', v => ctlPatch({ emuInputProfile: v }),
      ips.length ? 'Player 1 uses this saved profile whenever this user starts a game. A game can choose its own in Configure Game Settings.' : 'Player 1 is mapped automatically to the first connected controller with its standard layout (keyboard if none is connected).'), 'profile:controls.emuInputProfile'));
    if (!c.emuInputProfile) layoutRows(right, c.gameLayout || {}, async l => { await api(`/api/profiles/${S.profile.id}/game-layout`, { body: l }); await loadState(); S.profile = profileById(S.profile.id); Controllers.refreshList(); }, 'user');
    right.append(h('p', { class: 'note' }, 'In games the NEXUS engine detects controllers automatically (Xbox/XInput, PlayStation, Switch Pro and most USB or Bluetooth pads) and uses this layout for player 1. A game can have its own layout in Configure Game Settings › Controls. Keyboard: the engine’s standard keys work when no controller is connected.'));
    const known = h('div'); left.prepend(known);
    controllerList(known).then(() => $$('.set-h', known).forEach(e => e.style.cssText = 'font-size:.8em;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin:1em 0 .6em'));
    body.append(left, right);
    $$('.set-h', body).forEach(e => e.style.cssText = 'font-size:.8em;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin:1em 0 .6em');
  },
  liveUpdate(p) {
    if (S.screen !== 'controllers') return;
    const card = $(`.pad-card[data-pad="${p.index}"]`); if (!card) return;
    $$('.pad-btns span', card).forEach(s => { const b = p.buttons[+s.dataset.b]; s.classList.toggle('on', !!(b && (b.pressed || b.value > .5))); });
    $$('.stick', card).forEach(s => { const i = +s.dataset.s; const x = p.axes[i * 2] || 0, y = p.axes[i * 2 + 1] || 0; s.firstChild.style.transform = `translate(${x * 20}px,${y * 20}px)`; });
  },
};
Screens.controllers = { onBack: () => goHome() };
async function ctlPatch(patch) { if (!S.profile) return; await patchProfile(S.profile.id, { controls: patch }); if (S.screen === 'controllers' && !Mapper.listening) Controllers.refreshList(); renderHints(); }

// ---------- controller layout for games (Switch button -> physical position)
const POS_LABEL = { east: 'Right face button', south: 'Bottom face button', north: 'Top face button', west: 'Left face button', lshoulder: 'Left bumper (LB / L1)', rshoulder: 'Right bumper (RB / R1)', ltrigger: 'Left trigger (LT / L2)', rtrigger: 'Right trigger (RT / R2)', start: 'Start / Menu', back: 'Back / View', lstick: 'Left stick click', rstick: 'Right stick click' };
const SW_LABEL = { a: 'A', b: 'B', x: 'X', y: 'Y', l: 'L', r: 'R', zl: 'ZL', zr: 'ZR', plus: '+ (Plus)', minus: '− (Minus)', lstick: 'Left stick press', rstick: 'Right stick press' };
const GP_INDEX_POS = ['south', 'east', 'west', 'north', 'lshoulder', 'rshoulder', 'ltrigger', 'rtrigger', 'back', 'start', 'lstick', 'rstick'];
let LAYOUTS = null;
async function layoutInfo() { if (!LAYOUTS) LAYOUTS = await api('/api/controls/layouts'); return LAYOUTS; }
// Waits for a button press on any controller (standard mapping), returns the physical position.
function captureButton(ms = 6000) {
  return new Promise(resolve => {
    Mapper.listening = true;
    const held = new Set();
    for (const gp of navigator.getGamepads?.() || []) gp?.buttons.forEach((b, i) => b.pressed && held.add(gp.index + ':' + i));
    const t0 = performance.now();
    const tick = () => {
      for (const gp of navigator.getGamepads?.() || []) {
        if (!gp) continue;
        for (let i = 0; i < 12; i++) {
          const k = gp.index + ':' + i;
          if (gp.buttons[i]?.pressed && !held.has(k)) { Mapper.listening = false; resolve(GP_INDEX_POS[i]); return; }
          if (!gp.buttons[i]?.pressed) held.delete(k);
        }
      }
      if (performance.now() - t0 > ms) { Mapper.listening = false; resolve(null); return; }
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
}
function layoutRows(container, layout, save, scope) {
  const l = { base: layout.base || 'nintendo', buttons: { ...(layout.buttons || {}) }, controllerType: layout.controllerType || '' };
  container.append(addReset(rowChoice('Button layout', [{ v: 'nintendo', l: 'Nintendo positions (A on the right)' }, { v: 'xbox', l: 'Xbox labels (A at the bottom)' }], l.base,
    v => save({ ...l, base: v, buttons: {} }), 'Nintendo positions keep the console’s layout; Xbox labels match the letters printed on Xbox-style pads.'), { changed: () => l.base !== 'nintendo', def: () => 'Nintendo positions', run: () => save({ ...l, base: 'nintendo', buttons: {} }) }));
  container.append(addReset(rowChoice('Controller type', [{ v: '', l: 'Automatic' }, { v: 'pro', l: 'Pro Controller' }, { v: 'dual_joycon', l: 'Joy-Con pair' }, { v: 'handheld', l: 'Handheld' }], l.controllerType,
    v => save({ ...l, controllerType: v }), 'What the game sees. Automatic uses the game’s own preset if it has one, otherwise a Pro Controller.'), { changed: () => !!l.controllerType, def: () => 'Automatic', run: () => save({ ...l, controllerType: '' }) }));
  const n = Object.keys(l.buttons).length;
  container.append(rowButton('Customise buttons…', { value: n ? `${n} changed` : '', sub: 'Choose which button on your controller does each Switch button.', onClick: () => layoutEditor(l, save, scope) }));
  if (n || l.base !== 'nintendo' || l.controllerType) container.append(rowButton('Restore default layout', { danger: true, onClick: () => save({ base: 'nintendo', buttons: {}, controllerType: '' }) }));
}
async function layoutEditor(l0, save, scope) {
  const info = await layoutInfo();
  const l = { ...l0, buttons: { ...(l0.buttons || {}) } };
  sheet('Customise buttons', (body, layer, rebuild) => {
    body.append(h('p', { class: 'note' }, scope === 'game' ? 'These buttons apply to this game only.' : 'These buttons apply to every game, unless a game has its own layout.'));
    const base = info.bases[l.base] || info.bases.nintendo;
    for (const sw of info.switchButtons) {
      const cur = l.buttons[sw] || base[sw];
      const r = rowChoice(`Switch ${SW_LABEL[sw] || sw}`, info.positions.map(p => ({ v: p, l: POS_LABEL[p] || p })), cur,
        v => { if (v === base[sw]) delete l.buttons[sw]; else l.buttons[sw] = v; }, l.buttons[sw] ? 'Changed' : '');
      if (l.buttons[sw]) r.classList.add('overridden');
      addReset(r, { changed: () => !!l.buttons[sw], def: () => POS_LABEL[base[sw]] || base[sw], run: async () => { delete l.buttons[sw]; rebuild(); } });
      const press = h('button', { class: 'chip', 'data-nav': true }, 'Press…');
      press.addEventListener('click', async () => {
        press.textContent = 'Press a button…';
        const pos = await captureButton();
        if (pos) { if (pos === base[sw]) delete l.buttons[sw]; else l.buttons[sw] = pos; rebuild(); }
        else { press.textContent = 'Press…'; toast('No button pressed', I.info); }
      });
      r.append(press);
      body.append(r);
    }
    const dup = Object.entries(Object.fromEntries(info.switchButtons.map(sw => [sw, l.buttons[sw] || base[sw]]))).reduce((m, [sw, p]) => (m[p] = [...(m[p] || []), sw], m), {});
    const clashes = Object.entries(dup).filter(([, v]) => v.length > 1);
    if (clashes.length) body.append(h('p', { class: 'note warn' }, 'Several Switch buttons share a controller button: ' + clashes.map(([p, v]) => `${POS_LABEL[p]} → ${v.map(x => SW_LABEL[x]).join(' + ')}`).join('; ') + '.'));
    const row = h('div', { class: 'pk-setup-actions', 'data-row': 'strict' });
    const reset = h('button', { class: 'btn', 'data-nav': true }, 'Restore defaults'); reset.addEventListener('click', () => { l.buttons = {}; rebuild(); });
    const ok = h('button', { class: 'btn primary', 'data-nav': true }, 'Save'); ok.addEventListener('click', async () => { try { await save(l); layer.close(); toast('Controller layout saved', I.check); } catch (e) { alertDlg('Could not save', e.message); } });
    row.append(reset, ok); body.append(row);
  });
}

const Mapper = {
  listening: null, layer: null,
  start(action, label) {
    if (!S.pads.length) { alertDlg('No controller', 'Connect a controller first.'); return; }
    this.listening = action;
    const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, `Map “${label}”`), h('p', {}, 'Press the controller button you want to use. Press Esc to cancel.')));
    this.layer = openLayer(box, { onBack: () => this.cancel() });
  },
  capture(i) {
    const a = this.listening; if (!a) return;
    const m = { ...S.profile.controls.mapping };
    const clash = Object.keys(m).find(k => m[k] === i && k !== a);
    if (clash) m[clash] = m[a];
    m[a] = i;
    this.listening = null; this.layer?.close();
    Pad.prev = {}; // avoid the press triggering an action
    setTimeout(() => ctlPatch({ mapping: m }).then(() => toast(`Mapped to ${PadInfo.label(i)}`, I.check)), 150);
  },
  cancel() { this.listening = null; this.layer?.close(); },
};

// ------------------------------------------------------------------ events from the service
function connectEvents() {
  const es = new EventSource('/api/events');
  es.onmessage = async ev => {
    let m; try { m = JSON.parse(ev.data); } catch { return; }
    switch (m.type) {
      case 'library.scan':
        if (m.data.state === 'done') {
          await loadGames();
          if (S.screen === 'home') refreshViews();
          if (S.screen === 'library') renderLibrary();
          if (S.screen === 'settings' && curCat === 'library') { S.library = (await api('/api/state')).library; rebuildPanel(); }
          if (m.data.errors?.length) console.warn(m.data.errors);
        } else if (m.data.state === 'start') { S.library = { ...(S.library || {}), scanning: true }; if (S.screen === 'library') renderLibrary(); }
        break;
      case 'engine.ready': await refreshEngine(); if (S.screen === 'home') renderBanner(); if (S.screen === 'settings' && curCat === 'engine') rebuildPanel(); break;
      case 'library.changed': await loadGames(); if (['home', 'library'].includes(S.screen)) refreshViews(); break;
      case 'emulator.state': {
        const st = m.data;
        S.emuLatest = st;
        const was = S.playing?.suspended;
        if (st.running) {
          S.playing = st;
          if (st.keyboardMoves && S._movesShown !== st.started) { // once per session
            S._movesShown = st.started;
            toast(`Keyboard: ${st.keyboardMoves.replace(/(\w+) (\S+) -> (\S+)/g, (_, b, f, t) => `${b} moved from ${f} to ${t}`)}, so ${S.settings.emulator?.homeKey || 'Shift+H'} opens NEXUS Home.`, I.info || I.check, 6000);
          }
          if (Launch.el) { Launch.update(st); if (st.phase !== 'playing') break; }
          if (was && !st.suspended && S.screen === 'home') showPlaying(true); // resumed (e.g. with the Home button)
          else if (S.screen === 'playing') showPlaying(true);
          if (S.screen === 'home') renderBanner();
        }
        Wall.update();
        syncGameInFront();
        break;
      }
      case 'input.home': // Home pressed while NEXUS shows: go to the Home screen (never back to the game)
        if (!Launch.el) goHome({ button: true });
        break;
      case 'launch.reveal': if (Launch.el) Launch.enter(); break;
      case 'launch.warning': Launch.prepFailed = true; toast(m.data.message, I.warn, 9000); break;
      case 'shaders.prepare': onShaderPrep(m.data); break;
      case 'media.added':
        toast(`Screenshot saved${m.data.title ? ' · ' + m.data.title : ''}`, I.media, 2200);
        if (S.screen === 'media') { await Media.load(); Media.render(); }
        break;
      case 'recording.started': RecUI.start(); break;
      case 'recording.saved':
        RecUI.stop();
        toast(`Recording saved${m.data.title ? ' · ' + m.data.title : ''} (${fmtDur(m.data.seconds)})`, I.media, 3500);
        if (S.screen === 'media') { await Media.load(); Media.render(); }
        break;
      case 'recording.failed': RecUI.stop(); Sound.play('error'); alertDlg('The recording could not be made', m.data.error || 'Unknown error.'); break;
      case 'saves.import': SaveImportUI.update(m.data); break;
      case 'amiibo.scanning': AmiiboUI.scanning(m.data); break;
      case 'account.changed': AccountUI.update(m.data); break;
      case 'emulator.autopaused': toast(`${m.data.title} was paused after ${m.data.minutes >= 60 ? '1 hour' : m.data.minutes + ' minutes'} in the background`, I.info, 5000); if (S.screen === 'home') renderBanner(); break;
      case 'profiles.changed': await loadState(); if (S.screen === 'home') refreshTop(); break;
      case 'saves.sync': if (S.screen === 'settings' && curCat === 'account' && m.data?.state !== 'running') rebuildPanel(); break;
      case 'platform.device': S.platform = await api('/api/platform').catch(() => S.platform); if (S.screen === 'settings' && curCat === 'system') rebuildPanel(); break;
      case 'platform.thermal': if (S.platform) S.platform.device = { ...(S.platform.device || {}), thermalStatus: m.data.status };
        if (m.data.hot) toast(`Your device is very warm (${m.data.name}). Games slow down until it cools; take a break or remove a case.`, I.warn, 6000); break;
      case 'media.changed': if (S.screen === 'media') { await Media.load(); Media.render(); } break;
      case 'dlss.job':
        if (!m.data.running) { if (m.data.ok) toast('NVIDIA DLSS installed', I.check, 3000); else if (m.data.error) alertDlg('NVIDIA DLSS was not installed', m.data.error + '\n\nNothing incomplete was kept.'); }
        if (S.screen === 'settings' && curCat === 'emusettings') rebuildPanel();
        break;
      case 'gamecontent.changed':
        if (m.data.error) alertDlg('The update was not added', m.data.error); else toast('Update added', I.check);
        break;
      case 'update.available': if (!Launch.el && !S.playing?.running) updateDialog(m.data); else toast(`NEXUS ${m.data.latest} is available (Settings › Updates)`, I.info, 5000); break;
      case 'update.job': UpdateUI.job(m.data); break;
      case 'update.done': toast(`NEXUS was updated to ${m.data.version}`, I.check, 5000); break;
      case 'emulator.home': { // the game was paused with Home: show the Home screen
        S.playing = m.data;
        Launch.close(); // Home pressed while the loading screen was still fading: it must not keep the controls
        Sound.play('home_pop');
        goHome({ fromGame: true });
        Wall.update();
        syncGameInFront();
        break;
      }
      case 'emulator.exit': {
        S.playing = null;
        syncGameInFront();
        Launch.close();
        await loadGames();
        await refreshEmu();
        goHome();
        const r = m.data;
        if (r.error) { Sound.play('error'); launchErrorDlg(r.title, r.error); }
        else if (r.mode === 'game' && r.seconds > 0) toast(`${r.title}: ${fmtPlay(r.seconds).replace('Played for', 'played for')}`, I.check, 3500);
        if (r.mode === 'config') { await loadState(); }
        refreshEngine();
        if (S.profilePage?.refresh && !S.profilePage.root._closed) S.profilePage.refresh();
        break;
      }
      case 'engine.job': {
        S.engine = { ...S.engine, job: m.data };
        if (!m.data.active) await refreshEngine();
        if (S.screen === 'settings' && curCat === 'engine') rebuildPanel();
        if (S.screen === 'home') renderBanner();
        break;
      }
      case 'link.request': {
        const r = m.data; Sound.play('select');
        const ok = await dialog({ title: 'A new device wants to connect to your NEXUS account', text: `User: ${profileById(r.account)?.name || r.account}
Device: ${r.device}
Network address: ${r.addr}

Approve only if this is your own device and you started the link.`, buttons: [{ label: 'Decline', value: false }, { label: 'Approve', value: true, primary: true }], cancelValue: false });
        api('/api/link/decide', { body: { id: r.id, approve: !!ok } }).catch(() => { });
        break;
      }
      case 'link.changed': if (S.screen === 'settings' && curCat === 'sync') rebuildPanel(); break;
      case 'enginedata.job': {
        if (m.data.running === false) {
          if (m.data.done) toast(`Import finished: ${m.data.copied} file${m.data.copied === 1 ? '' : 's'} copied${m.data.skipped ? `, ${m.data.skipped} already in NEXUS` : ''}`, I.check, 4000);
          else alertDlg('The import did not finish', (m.data.error || '') + '\n\nNothing was lost. Try again in Settings › System files.');
          await refreshEngine(); if (S.screen === 'home') renderBanner();
        }
        if (S.screen === 'settings' && curCat === 'sysfiles' && (m.data.running === false || Math.random() < .2)) rebuildPanel();
        break;
      }
      case 'engine.changed': await refreshEngine(); if (S.screen === 'home') renderBanner(); break;
      case 'system.job': {
        if (S.screen === 'settings' && curCat === 'sysfiles' && (m.data.done || Math.random() < .25)) rebuildPanel();
        if (m.data.done) { if (m.data.error) alertDlg('Firmware was not installed', m.data.error); else { toast(m.data.message, I.check, 4000); if (m.data.warnings?.length) alertDlg('Installed, with notes', m.data.warnings.join('\n\n')); } }
        break;
      }
      case 'app.fullscreen': S.system.fullscreen = m.data; break;
      case 'app.focus': if (m.data && S.playing && !S.playing.suspended && S.screen !== 'playing') showPlaying(); break; // never over Home while the game waits there
      case 'profiles.changed': break;
      case 'avatars.state': if (m.data.state !== 'working' && Picker.reopen) Picker.reopen(); if (m.data.state === 'ready' && m.data.count) toast(`Profile icons ready (${m.data.count})`, I.person); break;
    }
  };
  es.onerror = () => { /* EventSource reconnects automatically */ };
}

// controller navigation needs the Gamepad API (WebView2; WebKitGTK with libmanette): say once in the
// log whether this window has it, so a missing one is visible in a report (Linux, Steam Deck)
api('/api/app/log', { body: { level: 'info', message: `ui: gamepad API ${typeof navigator.getGamepads === 'function' ? 'available' : 'MISSING'}` } }).catch(() => { });
window.addEventListener('error', e => { api('/api/app/log', { body: { level: 'error', message: `${e.message} @ ${e.filename}:${e.lineno}` } }).catch(() => { }); });
window.addEventListener('unhandledrejection', e => { api('/api/app/log', { body: { level: 'error', message: 'promise: ' + (e.reason?.stack || e.reason) } }).catch(() => { }); });

// ------------------------------------------------------------------ start

// ======================================================================================
// NEXUS 0.3: Save Data, Media, game updates & mods, controllers, DLSS files, NEXUS updates
// ======================================================================================

function download(url) { const a = h('a', { href: url, download: '' }); document.body.append(a); a.click(); a.remove(); }

// ---------- Save Data
// One user at a time; each game shows its save slots and files the way people think of them
// ("Slot 00", "Account Data"), never nested engine paths.
async function saveDataPage(profileId) {
  let pid = profileId || S.profile?.id || S.profiles[0]?.id;
  if (!pid) { alertDlg('No users yet', 'Create a user first.'); return; }
  const l = sheet('Save Data', async (body, layer, rebuild) => {
    let d;
    try { d = await api(`/api/profiles/${pid}/saves`); } catch (e) { body.append(h('p', { class: 'note warn' }, e.message)); return; }
    const pl = d.player;
    if (d.migrationError) body.append(h('p', { class: 'note warn' }, d.migrationError));
    // user picker
    const users = h('div', { class: 'sd-users' });
    for (const pr of S.profiles) {
      const b = h('button', { class: 'sd-user' + (pr.id === pid ? ' sel' : ''), 'data-nav': true }, avatarEl(pr), h('span', {}, pr.name));
      b.addEventListener('click', () => { if (pr.id !== pid) { pid = pr.id; rebuild(); } });
      users.append(b);
    }
    body.append(users);
    body.append(h('p', { class: 'note' }, `Player ID ${pl.playerId || '—'} · each user has their own saves; games only ever see the saves of the user who is playing.`));
    if (d.busy) body.append(h('p', { class: 'note warn' }, `${pl.name} is playing right now. Saves can be viewed, but changes wait until the game is closed.`));
    const acts = h('div', { class: 'sd-actions' });
    const imp = h('button', { class: 'btn primary', 'data-nav': true }, svg(I.plus), 'Import saves…');
    imp.addEventListener('click', () => saveImportWizard(pid, null, rebuild));
    const exp = h('button', { class: 'btn', 'data-nav': true }, 'Export all');
    exp.addEventListener('click', () => { download(`/api/profiles/${pid}/saves/export`); toast('Export started', I.check); });
    const bk = h('button', { class: 'btn', 'data-nav': true }, 'Back up all');
    bk.addEventListener('click', async () => { try { const b = await api(`/api/profiles/${pid}/saves/backup`, { body: { folder: '' } }); toast(`Backed up (${fmtBytes(b.bytes)})`, I.check); rebuild(); } catch (e) { alertDlg('Could not back up', e.message); } });
    const op = h('button', { class: 'btn', 'data-nav': true }, 'Open folder');
    op.addEventListener('click', () => api(`/api/profiles/${pid}/saves/open`, { body: {} }).catch(e => toast(e.message, I.warn)));
    acts.append(imp, exp, bk, op);
    body.append(acts);
    if (!pl.games.length) body.append(h('div', { class: 'sd-empty' }, h('b', {}, 'No save data yet'), h('span', {}, 'Saves appear here after playing a game, or import them from another PC or emulator.')));
    for (const g of pl.games) {
      const lib = S.games.find(x => x.titleId === g.titleId && x.kind !== 'update' && x.kind !== 'dlc');
      const head = h('div', { class: 'sd-ghead' },
        lib ? gameTile(lib) : h('div', { class: 'sd-noart' }, svg(I.pad)),
        h('div', { class: 'sd-ginfo' }, h('b', {}, g.title + (g.kind === 'device' ? ' — device save' : g.kind === 'other-copy' ? ' — other copy' : '')),
          h('span', {}, `Game ID ${g.titleId}`), h('span', {}, `${fmtBytes(g.bytes)} · last changed ${fmtDate(g.modified)}`)));
      head.querySelector('.tile')?.removeAttribute('data-nav');
      const items = h('div', { class: 'sd-items' }, g.items.map(it => h('span', { class: 'sd-item' + (it.dir ? ' slot' : '') }, it.label, h('small', {}, fmtBytes(it.bytes)))));
      const card = h('div', { class: 'sd-game' }, head, items);
      const btns = h('div', { class: 'sd-gbtns' });
      const add = (label, fn, cls = '') => { const b = h('button', { class: 'chip ' + cls, 'data-nav': true }, label); b.addEventListener('click', fn); btns.append(b); };
      if (g.kind === 'other-copy') add('Use this copy', async () => {
        if (await confirmDlg('Use this copy?', `This copy becomes ${g.title}'s save; the current save is kept as another copy. Everything is backed up first.`, 'Use this copy')) {
          try { await api(`/api/profiles/${pid}/saves/use-copy`, { body: { folder: g.folder } }); toast('Done', I.check); rebuild(); } catch (e) { alertDlg('Could not switch', e.message); }
        }
      });
      add('Import into this game…', () => saveImportWizard(pid, g.titleId, rebuild));
      add('Export', () => { download(`/api/profiles/${pid}/saves/export?folder=${encodeURIComponent(g.folder)}`); toast('Export started', I.check); });
      add('Back up', async () => { try { await api(`/api/profiles/${pid}/saves/backup`, { body: { folder: g.folder } }); toast('Backed up', I.check); rebuild(); } catch (e) { alertDlg('Could not back up', e.message); } });
      add('Restore…', () => restoreDlg(pid, g, rebuild));
      add('Open', () => api(`/api/profiles/${pid}/saves/open`, { body: { folder: g.folder } }).catch(e => toast(e.message, I.warn)));
      add('Delete…', async () => {
        if (await confirmDlg(`Delete ${g.title}'s save?`, `${pl.name}'s save for ${g.title} (${g.items.length} item${g.items.length === 1 ? '' : 's'}, ${fmtBytes(g.bytes)}) is deleted. A backup is made first, so it can be restored from Restore….`, 'Delete', true)) {
          try { await api(`/api/profiles/${pid}/saves/delete`, { body: { folder: g.folder, confirm: true } }); toast('Deleted (a backup was kept)', I.check); rebuild(); } catch (e) { alertDlg('Could not delete', e.message); }
        }
      }, 'danger');
      card.append(btns);
      body.append(card);
    }
    if (pl.otherFolders.length) body.append(h('p', { class: 'note' }, `Other folders in this user's save folder (not used by games): ${pl.otherFolders.join(', ')}. Use Import to assign one to a game.`));
    if (d.backups.length) body.append(h('p', { class: 'note' }, `${d.backups.length} backup${d.backups.length === 1 ? '' : 's'} in NEXUS data\\UserData\\Backups\\Saves.`));
  });
  return l;
}

async function restoreDlg(pid, g, done) {
  let list = [];
  try { list = await api(`/api/profiles/${pid}/saves/backups?titleId=${g.titleId}`); } catch { }
  if (!list.length) { alertDlg('No backups', `There are no backups of ${g.title}'s save yet. NEXUS makes one before every import, restore and delete, and you can make one with Back up.`); return; }
  const why = { manual: 'Backup', 'before-import': 'Before an import', 'before-restore': 'Before a restore', 'before-delete': 'Before deleting', migration: 'Before the 0.3 update' };
  const pick = await dialog({ title: `Restore ${g.title}`, text: 'Choose a backup. The current save is backed up first.', buttons: [{ label: 'Cancel', value: null }, ...list.slice(0, 6).map(b => ({ label: `${fmtDate(b.created)} · ${why[b.reason] || b.reason}`, value: b.file }))] });
  if (!pick) return;
  if (!(await confirmDlg('Replace the current save?', 'The save is replaced with the backup. The current save is backed up first.', 'Restore', true))) return;
  try { await api(`/api/profiles/${pid}/saves/restore`, { body: { file: pick, confirm: true } }); toast('Restored', I.check); done(); } catch (e) { alertDlg('Could not restore', e.message); }
}

// Device readiness (mobile only): what the device has against NEXUS's requirements.
function readinessRows(pl) {
  const r = pl.readiness, d = pl.device || {};
  const icon = { ok: '✓', warn: '!', fail: '✕' };
  const rows = [h('h3', {}, 'Device readiness'),
    h('p', { class: 'note' }, `${[d.manufacturer, d.model].filter(Boolean).join(' ')}${d.socModel ? ' · ' + d.socModel : ''}. ` + (r.ready ? 'This device meets NEXUS’s minimum requirements.' : 'This device does not meet all of NEXUS’s requirements yet (see below).'))];
  for (const it of r.items) rows.push(rowButton(`${icon[it.state] || ''} ${it.label}`, { value: it.value, sub: it.detail || '' }));
  rows.push(h('p', { class: 'note' }, 'Minimum: Android 13, 64-bit ARM, Vulkan 1.1, 8 GB RAM. Recommended: Snapdragon 8 Gen 2 class (Adreno 730 or newer) or a recent Mali/Immortalis GPU, Vulkan 1.3, 12–16 GB RAM.'));
  return rows;
}
const SaveImportUI = {
  el: null,
  update(d) {
    if (!this.el || this.el.hidden) return;
    const pct = d.total ? Math.round(d.done / d.total * 100) : 0;
    this.el.textContent = d.phase === 'backup' ? 'Backing up the saves that will be replaced…' : d.phase === 'done' ? 'Finishing…' : `Copying and checking saves… ${pct}%`;
  },
};
// Import: pick a folder or ZIP → NEXUS inspects it → shows what it found (player, game, slots) →
// the user confirms the user and game → import (existing saves only replaced after confirming).
async function saveImportWizard(pid, gameId, done) {
  const how = await dialog({ title: 'Import saves', text: 'Select a ZIP archive containing your Nintendo Switch save data. NEXUS will inspect the archive and identify compatible saves before importing them. You can also choose a save folder. Nothing is copied until you confirm, and the archive or folder is never changed.',
    buttons: [{ label: 'Cancel', value: null }, { label: 'Folder…', value: 'folder' }, { label: 'Import Save from ZIP', value: 'zip', primary: true }] });
  if (!how) return;
  let insp;
  try {
    if (!S.app.native && how === 'folder') {
      const v = await dialog({ title: 'Folder', text: 'Full path of the save folder on this PC.', input: { value: '', max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'OK', value: true, primary: true }] });
      if (!v) return;
      insp = await api('/api/saves/inspect', { body: { path: v } });
    } else if (!S.app.native && how === 'zip') {
      const f = await pickFile('.zip,application/zip'); if (!f) return;
      insp = await api(`/api/saves/inspect?name=${encodeURIComponent(f.name)}`, { body: new Blob([f], { type: 'application/zip' }) });
    } else insp = await api('/api/saves/inspect', { body: { pick: how } });
  } catch (e) { alertDlg('Could not read that', e.message); return; }
  if (!insp || insp.cancelled) return;
  if (!insp.candidates?.length) { alertDlg('No saves found in the archive', insp.note || 'No save data was found there.'); return; }
  const libGames = S.games.filter(g => g.titleId && g.kind !== 'update' && g.kind !== 'dlc');
  const picks = insp.candidates.map(c => ({ c, on: !gameId || !c.titleId || c.titleId === gameId, titleId: c.titleId || gameId || '' }));
  let target = (insp.candidates.find(c => c.profileId)?.profileId) || pid;
  let running = false;
  const progressEl = h('p', { class: 'note import-progress', hidden: true, role: 'status' });
  SaveImportUI.el = progressEl;
  const l = sheet('Import saves', (body, layer, rebuild) => {
    body.append(h('dl', { class: 'kv' }, h('dt', {}, 'From'), h('dd', {}, insp.source), h('dt', {}, 'Recognised as'), h('dd', {}, insp.format),
      h('dt', {}, 'Saves found'), h('dd', {}, String(insp.candidates.length))));
    if (insp.note) body.append(h('p', { class: 'note' }, insp.note));
    body.append(rowChoice('Import into', S.profiles.map(p => ({ v: p.id, l: p.name })), target, v => { target = v; rebuild(); },
      insp.candidates.some(c => c.playerId && !c.profileId) ? 'The player ID in these saves does not belong to a NEXUS user, so choose who gets them.' : 'Matched by player ID where the saves carry one.'));
    for (const p of picks) {
      const c = p.c;
      const card = h('div', { class: 'sd-game' });
      const title = c.titleId ? (c.title || c.titleId) : 'Unknown game';
      card.append(h('div', { class: 'sd-ginfo' }, h('b', {}, title + (c.device ? ' — device save' : '')),
        h('span', {}, [c.titleId ? `Game ID ${c.titleId}` : 'Game not identified', c.playerId ? `Player ${c.playerId}` : ''].filter(Boolean).join(' · ')),
        h('span', {}, c.evidence), h('span', {}, `${c.files} file${c.files === 1 ? '' : 's'}, ${fmtBytes(c.bytes)}`)));
      card.append(h('div', { class: 'sd-items' }, c.items.map(it => h('span', { class: 'sd-item' + (it.dir ? ' slot' : '') }, it.label))));
      card.append(rowToggle('Import this save', p.on, v => { p.on = v; }));
      if (!c.confident) {
        card.append(rowChoice('Game', [{ v: '', l: 'Choose the game…' }, ...libGames.map(g => ({ v: g.titleId, l: `${g.title} (${g.titleId})` }))], p.titleId, v => { p.titleId = v; },
          'NEXUS could not tell which game this save belongs to, so it is only imported into the game you choose.'));
      }
      body.append(card);
    }
    const go = h('button', { class: 'btn primary', 'data-nav': true }, 'Import selected saves');
    go.addEventListener('click', () => run(false));
    body.append(go, progressEl);
  });
  async function run(replace) {
    if (running) return;
    const sel = picks.filter(p => p.on);
    if (!sel.length) { toast('Nothing selected', I.warn); return; }
    if (sel.some(p => !p.titleId)) { alertDlg('Choose the game', 'Choose the game for every save NEXUS could not identify.'); return; }
    running = true; progressEl.hidden = false; progressEl.textContent = 'Importing…';
    try {
      await api('/api/saves/import', { body: { token: insp.token, profileId: target, picks: sel.map(p => ({ id: p.c.id, titleId: p.titleId, device: p.c.device, replace })) } });
      toast(`${sel.length} save${sel.length === 1 ? '' : 's'} imported into ${profileById(target)?.name || 'the user'}`, I.check, 3500); l.close(); done?.();
    } catch (e) {
      progressEl.hidden = true;
      if (e.status === 409 && e.data?.conflicts) {
        const names = e.data.conflicts.map(c => '• ' + c.title).join('\n');
        if (await confirmDlg('Replace existing saves?', `${profileById(target)?.name || 'This user'} already has saves for:\n${names}\n\nThey are backed up first, and can be restored from Save Data › Restore.`, 'Replace', true)) run(true);
      } else alertDlg('The saves were not imported', e.message);
    } finally { running = false; }
  }
}

// ---------- Media
const Media = {
  items: [], filter: 'all', game: '', sort: 'new',
  async show() {
    await this.load();
    this.render();
    show('media', { initial: Nav.items($('#media-body'))[0] });
  },
  async load() {
    try { const d = await api('/api/media'); this.items = d.items || []; this.dirs = d; } catch (e) { this.items = []; toast(e.message, I.warn); }
  },
  visible() {
    let l = this.items.filter(i => (this.filter === 'all' || i.type === this.filter) && (!this.game || i.gameKey === this.game));
    if (this.sort === 'old') l = [...l].reverse();
    return l;
  },
  render() {
    const body = $('#media-body'); body.innerHTML = '';
    const games = [...new Map(this.items.filter(i => i.gameKey).map(i => [i.gameKey, i.title || i.gameKey])).entries()];
    const bar = h('div', { class: 'media-bar' });
    const seg = (v, label) => { const b = h('button', { class: 'chip' + (this.filter === v ? ' sel' : ''), 'data-nav': true }, label); b.addEventListener('click', () => { this.filter = v; this.render(); Nav.setFocus(b); }); return b; };
    const n = t => this.items.filter(i => t === 'all' || i.type === t).length;
    bar.append(seg('all', `All (${n('all')})`), seg('screenshot', `Screenshots (${n('screenshot')})`), seg('recording', `Recordings (${n('recording')})`));
    const gsel = h('select', { class: 'media-sel', 'data-nav': true }, h('option', { value: '' }, 'All games'), games.map(([k, t]) => h('option', { value: k }, t)));
    gsel.value = this.game; gsel.addEventListener('change', () => { this.game = gsel.value; this.render(); });
    const ssel = h('select', { class: 'media-sel', 'data-nav': true }, h('option', { value: 'new' }, 'Newest first'), h('option', { value: 'old' }, 'Oldest first'));
    ssel.value = this.sort; ssel.addEventListener('change', () => { this.sort = ssel.value; this.render(); });
    const folder = h('button', { class: 'chip', 'data-nav': true }, 'Open Media folder');
    folder.addEventListener('click', () => api('/api/app/open-folder', { body: { which: 'media' } }).catch(e => toast(e.message, I.warn)));
    bar.append(gsel, ssel, folder);
    body.append(bar);
    const list = this.visible();
    if (!list.length) {
      body.append(h('div', { class: 'sd-empty' }, h('b', {}, this.items.length ? 'Nothing matches' : 'No captures yet'),
        h('span', {}, `Press ${S.settings.emulator?.screenshotKey || 'Alt+F12'} or your controller’s Share / Capture button while playing to take a screenshot, and ${S.settings.recording?.key || 'F12'} to start and stop a recording. Video files you put in Media\\Recordings appear here too.`)));
      return;
    }
    const grid = h('div', { class: 'media-grid' });
    for (const it of list) {
      const src = `/media/file?id=${encodeURIComponent(it.id)}`;
      const thumb = it.type === 'screenshot' ? h('img', { src: src + '&w=480', loading: 'lazy', alt: '' }) : h('video', { src, preload: 'metadata', muted: true });
      const cell = h('button', { class: 'media-item', 'data-nav': true }, h('div', { class: 'media-thumb' }, thumb, it.type === 'recording' ? h('span', { class: 'media-badge' }, svg(I.play), 'Video') : ''),
        h('div', { class: 'media-meta' }, h('b', {}, it.title || 'Unknown game'), h('span', {}, fmtDate(it.captured)), h('small', {}, it.name)));
      cell.addEventListener('click', () => this.view(it));
      grid.append(cell);
    }
    body.append(grid);
  },
  view(it) {
    const src = `/media/file?id=${encodeURIComponent(it.id)}`;
    const media = it.type === 'screenshot' ? h('img', { src, class: 'media-full', alt: it.name }) : h('video', { src, class: 'media-full', controls: true, autoplay: true });
    media.addEventListener('error', () => toast('This capture could not be shown. It may have been moved or deleted.', I.warn, 4000));
    const l = sheet(it.title || it.name, body => {
      body.append(media);
      body.append(h('dl', { class: 'kv' }, h('dt', {}, 'Type'), h('dd', {}, it.type === 'screenshot' ? 'Screenshot' : 'Recording'), h('dt', {}, 'Game'), h('dd', {}, it.title || 'Unknown'),
        h('dt', {}, 'Captured'), h('dd', {}, fmtDate(it.captured)), h('dt', {}, 'File'), h('dd', {}, `${it.name} · ${fmtBytes(it.bytes)}`)));
      const row = h('div', { class: 'sd-gbtns' });
      const add = (label, fn, cls = '') => { const b = h('button', { class: 'chip ' + cls, 'data-nav': true }, label); b.addEventListener('click', fn); row.append(b); };
      add('Open', () => api('/api/media/open', { body: { id: it.id } }).catch(e => toast(e.message, I.warn)));
      add('Show in folder', () => api('/api/media/reveal', { body: { id: it.id } }).catch(e => toast(e.message, I.warn)));
      if (it.gameKey && gameByKey(it.gameKey)) add('View game', () => { l.close(); showLibrary(); setTimeout(() => { const t = $$('#lib-grid .tile, #library .tile').find(x => x._game?.key === it.gameKey); if (t) Nav.setFocus(t); }, 50); });
      add('Delete…', async () => {
        if (await confirmDlg('Delete this capture?', `${it.name} is moved to the Recycle Bin.`, 'Delete', true)) {
          try { await api('/api/media/delete', { body: { id: it.id, confirm: true } }); l.close(); await this.load(); this.render(); toast('Deleted', I.check); } catch (e) { alertDlg('Could not delete', e.message); }
        }
      }, 'danger');
      body.append(row);
    }, { onClose: () => { if (media.pause) media.pause(); } });
  },
};
Screens.media = { onBack: () => goHome() };

// ---------- per-game Game Updates and Mods (Configure Game Settings)
async function gameContentPanel(body, g, which, redraw) {
  let d;
  try { d = await api(`/api/games/${g.key}/content`); } catch (e) { body.append(h('p', { class: 'note warn' }, e.message)); return; }
  if (which === 'updates') {
    body.append(h('p', { class: 'note' }, 'Add update packages for this game (NSP, NSZ, XCI, XCZ or NUP). NEXUS reads each package and only accepts a real update for this game. Updates in your games folder keep working too.'));
    if (d.busy) body.append(h('p', { class: 'note' }, `Adding ${d.busy}…`));
    const add = rowButton('Add an update…', { icon: I.plus, onClick: async () => {
      try {
        let body2 = {};
        if (!S.app.native) { const v = await dialog({ title: 'Update package', text: 'Full path of the update file on this PC.', input: { value: '', max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'OK', value: true, primary: true }] }); if (!v) return; body2 = { path: v }; }
        const u = await api(`/api/games/${g.key}/updates/add`, { body: body2 });
        if (u?.cancelled) return;
        toast(`Adding update ${u.version || u.fullId}…`, I.info, 3000); redraw();
      } catch (e) { alertDlg('That update was not added', e.message); }
    } });
    body.append(add);
    if (!d.updates.length) body.append(h('p', { class: 'note' }, 'No updates added in NEXUS.'));
    for (const u of d.updates) {
      const missing = u.source === 'missing';
      body.append(rowToggle(`Update ${u.version || ''}`.trim(), u.enabled, async v => {
        try { await api(`/api/games/${g.key}/updates/toggle`, { body: { file: u.file, on: v } }); } catch (e) { alertDlg('Could not change it', e.message); redraw(); }
      }, missing ? `The file ${u.file} is missing from the Updates folder.` : `${u.fullId} · ${fmtBytes(u.bytes)} · added ${fmtDate(u.added)}${u.enabled ? '' : ' · switched off: the game starts without it'}`));
      body.append(rowButton(`Remove update ${u.version || u.fullId}`, { danger: true, sub: 'Deletes NEXUS’s copy. The file you added it from is not touched.', onClick: async () => {
        if (await confirmDlg('Remove this update?', `${g.title} starts without update ${u.version || u.fullId} (unless another copy is in your games folder).`, 'Remove', true)) {
          try { await api(`/api/games/${g.key}/updates/remove`, { body: { file: u.file, confirm: true } }); redraw(); } catch (e) { alertDlg('Could not remove it', e.message); }
        }
      } }));
    }
    return;
  }
  await ModsUI.build(body, g.key, redraw);
}

// ---------- Manage Mods (mods.go, docs/MODS.md): one game's installed and built-in mods
const MOD_CAT = { fps: 'Frame rate', performance: 'Performance', resolution: 'Resolution', visual: 'Visual', gameplay: 'Gameplay', other: 'Other' };
const ModsUI = {
  open(key) {
    const g = gameByKey(key); if (!g) return;
    sheet(`Mods: ${g.title}`, (body, layer, rebuild) => { this.build(body, key, rebuild); });
  },
  async build(body, key, redraw) {
    const g = gameByKey(key);
    let d;
    try { d = await api(`/api/games/${encodeURIComponent(key)}/mods`); } catch (e) { body.append(h('p', { class: 'note warn' }, e.message)); return; }
    const kv = h('dl', { class: 'kv' },
      h('dt', {}, 'Game'), h('dd', {}, `${d.title} (${d.titleId})${d.version ? ' · version ' + d.version : ''}`),
      h('dt', {}, 'Game build'), h('dd', {}, d.buildId ? d.buildId : 'Not known yet: play the game once and NEXUS checks code mods against it'));
    body.append(kv);
    if (!d.canChange) body.append(h('p', { class: 'note warn' }, 'The game is running: mods can be switched, and take effect the next time it starts. Installing and removing needs the game closed.'));
    if (d.busy) body.append(h('p', { class: 'note' }, `Working: ${d.busy}…`));
    const addMod = async pickHow => {
      try {
        let r;
        if (!S.app.native && pickHow === 'zip') { const f = await pickFile('.zip,application/zip'); if (!f) return; r = await api(`/api/games/${key}/mods/add?name=${encodeURIComponent(f.name)}`, { body: new Blob([f], { type: 'application/zip' }) }); }
        else if (!S.app.native) { const v = await dialog({ title: 'Mod folder', text: 'Full path of the mod folder on this PC.', input: { value: '', max: 500 }, buttons: [{ label: 'Cancel', value: null }, { label: 'OK', value: true, primary: true }] }); if (!v) return; r = await api(`/api/games/${key}/mods/add`, { body: { path: v } }); }
        else r = await api(`/api/games/${key}/mods/add`, { body: { pick: pickHow } });
        if (r?.cancelled) return;
        toast(`Installed: ${r.added.join(', ')}`, I.check); redraw();
      } catch (e) { alertDlg('That mod was not installed', e.message); }
    };
    body.append(h('h3', { class: 'set-h' }, 'Install'));
    body.append(rowButton('Import a mod from a ZIP…', { icon: I.plus, disabled: !d.canChange, onClick: () => addMod('zip') }));
    body.append(rowButton('Import a mod folder…', { icon: I.folder, disabled: !d.canChange, sub: 'A folder with romfs, exefs, romfs_ext or cheats inside, or .pchtxt / .ips patches. Mods for another game are refused; the game’s own files are never changed.', onClick: () => addMod('folder') }));

    body.append(h('h3', { class: 'set-h' }, `Installed mods${d.mods.length ? ` (${d.mods.filter(m => m.enabled).length} of ${d.mods.length} on)` : ''}`));
    if (!d.mods.length) body.append(h('p', { class: 'note' }, 'No mods installed for this game.'));
    for (const m of d.mods) body.append(this.row(key, m, redraw, d));

    if (d.builtin.length) {
      body.append(h('h3', { class: 'set-h' }, 'Built into the NEXUS engine'));
      body.append(h('p', { class: 'note' }, 'Patches made for this game that come with the NEXUS engine. They only apply to the game build they were made for. Switch one off if you want the game’s own behaviour.'));
      for (const m of d.builtin) body.append(this.row(key, m, redraw, d));
    }
    body.append(rowButton('Open the mods folder', { sub: d.modsDir, onClick: () => api('/api/app/open-folder', { body: { which: 'enginedata' } }).catch(() => { }) }));
  },
  // one mod: switch, status line, and its information on OK
  row(key, m, redraw, d) {
    const name = m.meta?.name || m.name;
    const bits = [];
    if (m.meta?.version) bits.push('v' + m.meta.version);
    if (m.meta?.author && !m.builtin) bits.push('by ' + m.meta.author);
    if (m.meta?.category) bits.push(MOD_CAT[m.meta.category] || m.meta.category);
    if (!m.builtin) bits.push(m.files ? `${m.files} file${m.files === 1 ? '' : 's'}` : (m.contents || []).join(', '), fmtBytes(m.bytes));
    const warn = [];
    if (m.compat?.state === 'mismatch') warn.push('Not for this game build');
    if (m.conflicts?.length) warn.push('Conflicts with ' + m.conflicts.join(', '));
    if (m.missing?.length) warn.push('Needs ' + m.missing.map(x => x.replace(' (built-in)', '')).join(', '));
    if (m.applied?.startsWith('skipped')) warn.push('Last start: ' + m.applied.replace('skipped: ', 'skipped (') + ')');
    const sub = [bits.filter(Boolean).join(' · '), m.applied === 'applied' && m.enabled ? 'Applied at the last start' : '', warn.join(' · ')].filter(Boolean).join(' — ');
    const wrap = h('div', { class: 'mod-row' + (warn.length ? ' warn' : '') });
    const t = rowToggle(name, m.enabled, async v => {
      try { await api(`/api/games/${encodeURIComponent(key)}/mods/toggle`, { body: { name: m.name, on: v } }); redraw(); }
      catch (e) { alertDlg('Could not change it', e.message); redraw(); }
    }, sub);
    wrap.append(t);
    const info = h('button', { class: 'chip', 'data-nav': true }, 'Details');
    info.addEventListener('click', () => this.details(key, m, redraw, d));
    wrap.append(info);
    return wrap;
  },
  async details(key, m, redraw, d) {
    const meta = m.meta || {};
    const lines = [
      meta.description || (m.builtin ? '' : 'This mod has no description (a nexus-mod.json in its folder can give one).'),
      '',
      `Status: ${m.enabled ? 'on' : 'off'}${m.applied ? ' · last start: ' + m.applied : ''}`,
      meta.version ? `Version: ${meta.version}` : '', meta.author ? `Author: ${meta.author}` : '',
      meta.category ? `Category: ${MOD_CAT[meta.category] || meta.category}` : '',
      meta.gameVersion ? `For game version: ${meta.gameVersion}` : '',
      `Compatibility: ${m.compat?.note || ''}`,
      m.buildIds?.length ? `Game builds: ${m.buildIds.join(', ')}` : '',
      meta.requires?.length ? `Needs: ${meta.requires.join(', ')}${m.missing?.length ? ' (not on: ' + m.missing.join(', ') + ')' : ''}` : '',
      m.conflicts?.length ? `Conflicts: replaces the same game files as ${m.conflicts.join(', ')}. Only one of them takes effect; switch one off.` : '',
      !m.builtin ? `Contents: ${(m.contents || []).join(', ')} · ${m.files} file(s) · ${fmtBytes(m.bytes)}` : 'Part of the NEXUS engine; it cannot be removed, only switched off.',
      meta.website ? `Website: ${meta.website}` : '',
    ].filter((x, i) => x !== '' || i === 1).join('\n').trim();
    const buttons = [{ label: 'Close', value: null, primary: true }];
    if (!m.builtin && d.canChange) buttons.unshift({ label: 'Remove', value: 'remove', danger: true });
    const v = await dialog({ title: meta.name || m.name, pre: lines, buttons });
    if (v === 'remove' && await confirmDlg(`Remove ${m.name}?`, 'The mod is moved to the Recycle Bin. The game’s files and your saves are not touched.', 'Remove', true)) {
      try { await api(`/api/games/${encodeURIComponent(key)}/mods/remove`, { body: { name: m.name, confirm: true } }); toast('Mod removed', I.check); redraw(); }
      catch (e) { alertDlg('Could not remove it', e.message); }
    }
  },
};

// ---------- remembered controllers (Controllers screen)
async function controllerList(box) {
  box.append(h('h3', { class: 'set-h' }, 'Your controllers'));
  let d;
  try { d = await api(`/api/controllers?profile=${S.profile?.id || ''}`); } catch (e) { box.append(h('p', { class: 'note warn' }, e.message)); return; }
  if (d.error && !d.pads.length) box.append(h('p', { class: 'note' }, `Controllers are listed by the NEXUS engine: ${d.error}`));
  if (!d.pads.length) box.append(h('p', { class: 'note' }, 'No controller has been connected yet.'));
  const chosen = d.pads.filter(p => p.player).sort((a, b) => a.player - b.player).map(p => p.key);
  const setPlayers = async keys => { try { await api('/api/controllers/players', { body: { profileId: S.profile.id, keys } }); Controllers.refreshList(); } catch (e) { alertDlg('Could not change it', e.message); } };
  for (const p of d.pads) {
    const name = p.nickname || p.label || p.name; // the model (padmodels.go), not the driver's name
    const status = [p.nickname ? p.label : '', p.connected ? 'connected' : `not connected · last seen ${fmtAgo(p.lastSeen)}`, p.serial ? `serial ${p.serial}` : ''].filter(Boolean).join(' · ');
    const card = h('div', { class: 'pad-card known' + (p.player ? ' chosen' : '') + (p.connected ? '' : ' off') },
      h('h4', {}, p.player ? h('span', { class: 'pl-badge' }, `Player ${p.player}`) : '', name), h('small', {}, status));
    const btns = h('div', { class: 'sd-gbtns' });
    const add = (label, fn, cls = '') => { const b = h('button', { class: 'chip ' + cls, 'data-nav': true }, label); b.addEventListener('click', fn); btns.append(b); };
    if (p.player !== 1) add('Use as Player 1', () => setPlayers([p.key, ...chosen.filter(k => k !== p.key)]));
    if (!p.player) add('Add as next player', () => setPlayers([...chosen, p.key]));
    if (p.player) add('Stop using', () => setPlayers(chosen.filter(k => k !== p.key)));
    add('Configure…', () => configurePad(p));
    add('Forget', async () => {
      if (await confirmDlg(`Forget ${name}?`, 'NEXUS forgets this controller and its settings. If you connect it again it is listed as new.', 'Forget', true)) {
        try { await api('/api/controllers/forget', { body: { key: p.key } }); Controllers.refreshList(); } catch (e) { alertDlg('Could not forget it', e.message); }
      }
    }, 'danger');
    card.append(btns);
    box.append(card);
  }
  box.append(h('p', { class: 'note' }, chosen.length ? `${S.profile?.name || 'This user'} plays with the controllers marked Player 1, 2… (connected ones, in that order). Multiplayer games get one player per controller.` :
    'With no controller chosen, player 1 is the first connected controller. Choose Use as Player 1 to always play with a specific one.'));
}
function configurePad(p) {
  sheet(p.nickname || p.name || p.label, (body, layer, rebuild) => {
    body.append(rowButton('Name', { value: p.nickname || p.name, onClick: async () => {
      const n = await dialog({ title: 'Controller name', input: { value: p.nickname || p.name || '', max: 30 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
      if (n == null) return;
      try { const kp = await api('/api/controllers/update', { body: { key: p.key, nickname: n } }); Object.assign(p, kp); rebuild(); Controllers.refreshList(); } catch (e) { alertDlg('Could not rename', e.message); }
    } }));
    body.append(h('dl', { class: 'kv' }, h('dt', {}, 'Type'), h('dd', {}, p.label), h('dt', {}, 'Model'), h('dd', {}, p.name || '—'),
      h('dt', {}, 'First connected'), h('dd', {}, fmtDate(p.firstSeen)), h('dt', {}, 'Identified by'), h('dd', {}, p.serial ? 'its serial number' : 'its model and USB/Bluetooth position')));
    body.append(h('h3', {}, 'Button layout of this controller'));
    body.append(rowToggle('Own layout', !!p.layout, async v => {
      const kp = await api('/api/controllers/update', { body: v ? { key: p.key, layout: { base: 'nintendo' } } : { key: p.key, clearLayout: true } }).catch(e => { alertDlg('Could not change it', e.message); return null; });
      if (kp) { Object.assign(p, kp); if (!v) p.layout = null; rebuild(); }
    }, 'When this controller is player 1 it uses its own layout instead of the user’s (a game’s own layout still comes first).'));
    if (p.layout) layoutRows(body, p.layout, async l => {
      const kp = await api('/api/controllers/update', { body: { key: p.key, layout: l } }).catch(e => { alertDlg('Could not save', e.message); return null; });
      if (kp) { Object.assign(p, kp); rebuild(); }
    }, 'controller');
  });
}

// ---------- NVIDIA DLSS files (Frame generation & DLSS)
async function dlssPanel(box) {
  let st;
  try { st = await api('/api/dlss'); } catch (e) { box.append(h('p', { class: 'note warn' }, e.message)); return; }
  box.append(h('h3', {}, 'NVIDIA DLSS files'));
  if (!st.supported) { box.append(h('p', { class: 'note' }, st.why || 'DLSS needs an NVIDIA GeForce RTX graphics card.')); return; }
  if (st.ready) {
    box.append(infoRow('NVIDIA DLSS', 'Installed', `${st.installed.join(', ')}${st.appModel ? ' · DLSS model from the NVIDIA app' : ''}`));
    return;
  }
  const j = st.job;
  if (j?.running) {
    box.append(infoRow('Downloading from NVIDIA', `${Math.round((j.done / Math.max(1, j.total)) * 100)}%`, j.file || ''));
    return;
  }
  if (j?.error) box.append(h('p', { class: 'note warn' }, `The last download did not finish: ${j.error}. Nothing incomplete was kept.`));
  box.append(h('p', { class: 'note' }, `Your ${st.gpu} supports DLSS. The NEXUS engine needs NVIDIA’s Streamline files to use it: ${st.missing.join(', ')}. NEXUS can download them from NVIDIA’s own Streamline ${st.version} release, check them (checksum and NVIDIA’s signature) and install them. Until then games use AMD FSR 3.1.`));
  box.append(rowButton('Download from NVIDIA…', { icon: I.check, onClick: async () => {
    let offer;
    try { toast('Contacting NVIDIA’s download…', I.info, 2000); offer = await api('/api/dlss/prepare', { body: {} }); } catch (e) { alertDlg('Could not reach NVIDIA’s download', e.message + `\n\nYou can also download the Streamline SDK yourself from ${st.source} and copy ${st.missing.join(', ')} from bin\\x64 into ${st.folder}.`); return; }
    const text = `NEXUS will download ${offer.files.join(', ')} (${fmtBytes(offer.bytes)}) from ${offer.source}.\n\nThese are NVIDIA’s files, under NVIDIA’s licences:\n\n` + offer.licences.map(l => `==== ${l.title} ====\n${l.text.slice(0, 4000)}${l.text.length > 4000 ? '\n…' : ''}`).join('\n\n');
    const ok = await dialog({ title: 'NVIDIA DLSS', pre: text, buttons: [{ label: 'Cancel', value: null }, { label: 'Accept and download', value: true, primary: true }] });
    if (!ok) return;
    try { await api('/api/dlss/install', { body: { accepted: true } }); rebuildPanel?.(); } catch (e) { alertDlg('Could not download', e.message); }
  } }));
  box.append(rowButton('Open the DLSS folder', { sub: st.folder, onClick: () => api('/api/app/open-folder', { body: { which: 'streamline' } }).catch(e => toast(e.message, I.warn)) }));
}

// ---------- NEXUS updates
async function updatesPanel(p) {
  p.append(h('h2', {}, 'Updates'));
  let d;
  try { d = await api('/api/update'); } catch (e) { p.append(h('p', { class: 'note warn' }, e.message)); return; }
  if (d.failed) p.append(h('p', { class: 'note warn' }, `The last update did not start correctly, so NEXUS went back to this version: ${d.failed}`));
  const info = d.info || {};
  p.append(infoRow('Installed version', d.current));
  if (info.checked) p.append(infoRow('Latest version', info.latest || '—', `Checked ${fmtAgo(info.checked)}`));
  if (info.error) p.append(h('p', { class: 'note warn' }, info.error));
  if (info.available) p.append(rowButton(`Update to ${info.latest}…`, { icon: I.check, onClick: () => updateDialog(info) }));
  else if (info.checked && !info.error) p.append(h('p', { class: 'note' }, 'NEXUS is up to date.'));
  p.append(rowButton('Check for updates', { onClick: async () => { await api('/api/update/check', { body: {} }).catch(() => { }); rebuildPanel(); } }));
  p.append(changedMark(rowToggle('Check when NEXUS starts', d.settings.autoCheck, v => api('/api/update/settings', { body: { autoCheck: v } })), 'updates.autoCheck'));
  p.append(h('h3', {}, 'Release page'));
  p.append(rowButton('Repository', { value: d.settings.repo || '0ryzin/Nexus', sub: 'Where NEXUS looks for official releases (GitHub owner/name).', onClick: async () => {
    const v = await dialog({ title: 'Release repository', input: { value: d.settings.repo || '', max: 100 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
    if (v == null) return;
    try { await api('/api/update/settings', { body: { repo: v } }); rebuildPanel(); } catch (e) { alertDlg('Not saved', e.message); }
  } }));
  p.append(rowButton('Access token', { value: d.hasToken ? 'Saved' : 'None', disabled: !d.canStoreToken, sub: 'Only needed while the release page is private: a GitHub fine-grained token with read-only access to that repository’s contents. NEXUS stores it encrypted for your Windows account and sends it only to GitHub’s API.', onClick: async () => {
    const v = await dialog({ title: 'GitHub access token', text: 'Paste a read-only token, or leave it empty to remove the saved one.', input: { value: '', max: 300 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
    if (v == null) return;
    try { await api('/api/update/settings', { body: { token: v } }); rebuildPanel(); } catch (e) { alertDlg('Not saved', e.message); }
  } }));
  p.append(h('p', { class: 'note' }, 'Updates are checked against the published SHA-256 checksum before they are used. Your users, saves, settings, captures, mods, game updates and controller settings are never touched. If a new version does not start, NEXUS puts the previous one back automatically.'));
}

async function updateDialog(info) {
  const notes = (info.notes || '').replace(/\r/g, '').slice(0, 6000);
  const v = await dialog({ title: `NEXUS ${info.latest} is available`, pre: `Installed: ${S.app.version}\nNew: ${info.latest}${info.size ? `\nDownload: ${fmtBytes(info.size)}` : ''}\n\n${notes}`,
    buttons: [{ label: 'Later', value: 'later' }, { label: info.manual ? 'Open the download page' : 'Update now', value: 'now', primary: true }], cancelValue: 'later' });
  if (v === 'later') { api('/api/update/later', { body: {} }).catch(() => { }); return; }
  // Linux: the new AppImage / tar.gz comes from the release page (NEXUS does not replace itself)
  if (info.manual) { api('/api/app/open-link', { body: { which: 'release' } }).catch(e => alertDlg('Could not open the page', e.message)); return; }
  try { await api('/api/update/download', { body: {} }); } catch (e) { alertDlg('Could not download', e.message); return; }
  const box = h('div', { class: 'dialog' }, h('div', { class: 'd-body' }, h('h2', {}, `Updating to NEXUS ${info.latest}`), h('p', { class: 'upd-status' }, 'Downloading…'), h('div', { class: 'lx-bar measured' }, h('i'))));
  const layer = openLayer(box, { onBack: () => { } });
  UpdateUI.box = box; UpdateUI.layer = layer;
}
const UpdateUI = {
  box: null, layer: null,
  async job(j) {
    if (!this.box) return;
    const st = $('.upd-status', this.box), bar = $('.lx-bar i', this.box);
    if (j.state === 'downloading') { st.textContent = `Downloading… ${fmtBytes(j.done)} of ${fmtBytes(j.total)}`; bar.style.width = `${Math.round(j.done / Math.max(1, j.total) * 100)}%`; }
    if (j.state === 'verifying') st.textContent = 'Checking the download…';
    if (j.state === 'error') { this.layer.close(); this.box = null; alertDlg('The update was not installed', j.error + '\n\nNEXUS has not changed.'); }
    if (j.state === 'ready') {
      st.textContent = 'Ready. NEXUS restarts to finish the update.';
      bar.style.width = '100%';
      if (S.playing?.running) { st.textContent = 'Ready. Close your game to finish the update (NEXUS restarts).'; return; }
      await sleep(800);
      try { await api('/api/update/install', { body: {} }); st.textContent = 'Installing… NEXUS restarts in a moment.'; } catch (e) { this.layer.close(); this.box = null; alertDlg('The update was not installed', e.message); }
    }
  },
};

// ---------- settings: Save Data and Updates categories
Object.assign(Panels, {
  savedata(p) {
    p.append(h('h2', {}, 'Save data'));
    p.append(h('p', { class: 'note' }, 'Every user has their own save folder, NEXUS data\\UserData\\Saves\\<player ID>-<name>, with a folder per game named after its game ID and a short name (for example 0400486024DEC000-OOT). Games read and write there directly.'));
    for (const pr of S.profiles) {
      const r = rowButton(pr.name, { sub: 'View, import, export, back up, restore or delete saves', onClick: () => saveDataPage(pr.id) });
      r.querySelector('.rl').prepend(avatarEl(pr)); r.querySelector('.rl').style.cssText = 'flex-direction:row;align-items:center;gap:.8em';
      r.querySelector('.avatar').style.setProperty('--sz', '2.2em');
      p.append(r);
    }
    p.append(rowButton('Import saves…', { icon: I.plus, sub: 'From NEXUS, another PC or another emulator’s save folder, or a ZIP.', onClick: () => saveImportWizard(S.profile?.id || S.profiles[0]?.id, null, () => { }) }));
    p.append(rowButton('Open the saves folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'saves' } }) }));
    p.append(rowButton('Open save backups', { sub: 'Made before every import, restore, delete and the 0.3 conversion.', onClick: () => api('/api/app/open-folder', { body: { which: 'savebackups' } }) }));
  },
  updates(p) { return updatesPanel(p); },
});

// startup milestones: logged by NEXUS once each (startupmarks.go) and kept for measurement
const startupMarks = {};
function markStartup(name, detail) {
  if (startupMarks[name] != null) return;
  startupMarks[name] = Math.round(performance.now());
  api('/api/startup/mark', { body: { name, detail: detail || '' } }).catch(() => { });
}

(async function main() {
  Sound.init();
  try { await loadState(true); markStartup('interface ready'); }
  catch (e) { document.body.classList.remove('booting'); document.body.innerHTML = `<div style="padding:3em;font-family:sans-serif;color:#fff">NEXUS could not reach its service: ${e.message}</div>`; return; }
  const last = profileById(S.settings.lastProfileId);
  if (last && !S.settings.askProfileOnStart) S.profile = last;
  applyTheme();
  // Optional user stylesheets: Resources/Themes/*.css are loaded after the built-in theme.
  api('/res/themes').then(list => (list || []).forEach(n => document.head.append(h('link', { rel: 'stylesheet', href: '/res/themes/' + encodeURIComponent(n) })))).catch(() => { });
  // the game list (unless it came with the first answer) is asked first: the browser opens only six connections to
  // NEXUS at a time, and the fourteen sound files used to queue the game list behind them
  const bootGames = S.bootGames; S.bootGames = null;
  if (S.profile && bootGames?.profile === S.profile.id) { // the game list came with the first answer
    S.games = bootGames.games || []; S.stats = bootGames.stats || {}; S.recent = bootGames.recent || []; S.library = bootGames.status;
    api('/api/compat').then(c => { S.compat = c || {}; }).catch(() => { });
  }
  const gamesLoad = bootGames && S.profile && bootGames.profile === S.profile.id ? Promise.resolve(markStartup('library available', S.games.length + ' games')) : S.profile ? loadGames().then(() => markStartup('library available', (S.games || []).length + ' games')).catch(() => { }) : Promise.resolve();
  const crashLoad = api('/api/crash/last').catch(() => null);
  const soundLoad = Sound.load(['boot_power', 'boot_whoosh', 'boot_snap', 'boot_draw', 'boot_sparkle', 'boot_chime', 'boot_outro', 'home_pop', 'move', 'select', 'back', 'launch', 'error', 'connect',
    ...(S.settings.startup?.enabled && S.settings.startup?.animation === 'oot' ? ['oot_launch'] : [])]);
  // the startup animation's sounds are timed to it, so it waits for them (up to 0.6 s); without
  // the animation (or its sound) nothing needs to wait: sounds finish loading in the background
  const bootSt = S.settings.startup || {};
  if (bootSt.enabled && (bootSt.sound ?? true) && S.anim !== 'off') await Promise.race([soundLoad, sleep(600)]);
  connectEvents();
  pollSystem(); setInterval(pollSystem, 15000);
  tickClock(); setInterval(tickClock, 1000);
  Pad.schedule(true); window.addEventListener('gamepadconnected', () => Pad.schedule(true));
  // Prepare the home screen underneath the boot animation
  await Boot.run();
  await gamesLoad;
  const st = { emulator: S.emulator?.state }; // a game already running (from the first answer)
  if (S.app.migrated?.length) toast(S.app.migrated[0], I.check, 6000);
  // the last game session ended with NEXUS closing unexpectedly: say so once, with the report
  crashLoad.then(n => { if (n) { Sound.play('error'); launchErrorDlg(n.title, n); } });
  if (st.emulator?.running) { S.playing = st.emulator; RecUI.sync(); showPlaying(); return; }
  if (!S.profile) { showWho(); markStartup('profile picker shown'); return; }
  renderHome();
  const home = $('#home');
  if (S.anim !== 'off') home.classList.add('intro');
  show('home', { initial: $('#tiles .tile'), anim: false });
  if (S.anim !== 'off') {
    requestAnimationFrame(() => { home.classList.remove('intro'); home.classList.add('build'); setTimeout(() => home.classList.remove('build'), 2200); });
    if (S.settings.startup?.sound ?? true) {
      const tiles = $$('#tiles .tile').length, dock = $$('#dock .dock-btn').length;
      for (let i = 0; i < Math.min(tiles, 8); i++) Sound.play('home_pop', .35, { at: (250 + i * 55) / 1000, rate: .9 + i * .05, pan: -.5 + i * .14 });
      for (let i = 0; i < dock; i++) Sound.play('home_pop', .3, { at: (450 + i * 45) / 1000 + .05, rate: 1.35 + i * .04 });
    }
  }
  if (Nav.cur?._game) updateSelLabel(Nav.cur._game);
  markStartup('home interactive', S.settings.startup?.enabled ? 'after the startup animation' : '');
})();

// ------------------------------------------------------------------ screen recording (0.4.0)
// The game window is recorded by the NEXUS capture module (never the NEXUS interface). While a
// recording runs, a small red indicator with the time is shown here and, over the game, by the
// NEXUS overlay window.
const RecUI = {
  el: null, t0: 0, timer: null,
  start(seconds = 0) {
    this.t0 = performance.now() - seconds * 1000;
    if (!this.el) { this.el = h('div', { class: 'rec-badge', role: 'status', 'aria-live': 'polite' }, h('i'), h('span', {}, '0:00')); document.body.append(this.el); }
    clearInterval(this.timer);
    const tick = () => { const s = Math.floor((performance.now() - this.t0) / 1000); this.el.lastChild.textContent = `REC ${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`; };
    tick(); this.timer = setInterval(tick, 500);
  },
  stop() { clearInterval(this.timer); this.timer = null; this.el?.remove(); this.el = null; },
  async sync() { try { const r = await api('/api/recording'); if (r.status?.active) this.start(r.status.seconds || 0); else this.stop(); } catch { } },
};

// While a game runs in the background behind NEXUS, the record and screenshot shortcuts reach
// NEXUS rather than the game, so they are handled here.
window.addEventListener('keydown', ev => {
  if (!S.playing?.running || !S.playing.suspended || ev.repeat) return;
  if (Nav.layers.some(l => l.dialog)) return;
  const rec = S.settings.recording || {}, es = S.settings.emulator || {};
  if (rec.enabled !== false && shortcutMatches(ev, rec.key)) {
    ev.preventDefault(); ev.stopPropagation();
    api('/api/recording/toggle', { body: {} }).catch(e => toast(e.message, I.warn, 3500));
  } else if (shortcutMatches(ev, es.screenshotKey)) {
    ev.preventDefault(); ev.stopPropagation();
    api('/api/emulator/screenshot', { body: {} }).catch(e => toast(e.message, I.warn, 3500));
  }
}, true);

async function renderRecordingSettings(p) {
  let info;
  try { info = await api('/api/recording'); } catch (e) { p.append(h('h2', {}, 'Screen recording'), h('p', { class: 'note' }, e.message)); return; }
  const rs = info.settings || {}, caps = info.caps || {}, eff = info.effective || {};
  const save = patch => saveSettings({ recording: patch }).then(rebuildPanel);
  p.append(h('h2', {}, 'Screen recording'), h('p', { class: 'note' }, 'Records the game window only — never the NEXUS screens — with the game’s sound. Recordings go to Media › Recordings.'));
  if (!caps.available) {
    p.append(h('p', { class: 'note warn' }, 'Recording is not available on this PC: ' + (caps.reason || 'the NEXUS capture module could not be loaded.') + ' Screenshots still work.'));
  }
  p.append(changedMark(rowToggle('Screen recording', rs.enabled !== false, v => save({ enabled: v }), 'When off, the record shortcut does nothing.'), 'recording.enabled'));
  if (rs.enabled === false) return;
  p.append(changedMark(rowButton('Start / stop shortcut', { value: rs.key || 'Off', sub: 'Press it while playing to start a recording and again to stop it. Changes apply to the next game you start.', onClick: async () => {
    const k = await captureKey({ mods: true }); if (k === undefined) return;
    if (k && k.toLowerCase() === (S.settings.emulator?.screenshotKey || '').toLowerCase()) { alertDlg('That shortcut is taken', `${k} takes screenshots. Choose another key, or change the screenshot key in Playing games.`); return; }
    save({ key: k });
  } }), 'recording.key'));
  // encoder line: what will actually be used
  const hw = (caps.encoders || []).filter(e => e.hardware);
  const encLine = !caps.available ? 'None' : rs.hardware !== false && caps.hardware
    ? `Hardware: ${hw.map(e => e.name).filter((v, i, a) => a.indexOf(v) === i).join(', ')}`
    : 'Software (Windows Media Foundation)';
  const VEND = { nvidia: 'NVIDIA NVENC', amd: 'AMD', intel: 'Intel Quick Sync' };
  p.append(rowButton('Encoder', { value: caps.vendor && rs.hardware !== false && caps.hardware ? (VEND[caps.vendor] || caps.vendor) : (caps.available ? 'Software' : '—'), sub: encLine + (caps.hardware ? '' : '. No hardware video encoder was found, so recording uses the processor; resolution is limited to 1080p to keep games smooth.') }));
  if (caps.hardware) p.append(changedMark(rowToggle('Use the graphics card to encode', rs.hardware !== false, v => save({ hardware: v }), 'NVIDIA NVENC, AMD or Intel Quick Sync. Much lighter on games than software encoding. Turn off only if recordings fail with your driver.'), 'recording.hardware'));
  const RES = { native: 'Game resolution', '720p': '720p', '1080p': '1080p', '1440p': '1440p', '2160p': '4K (2160p)' };
  const resList = (caps.resolutions?.length ? caps.resolutions : ['native', '720p', '1080p']).map(v => ({ v, l: RES[v] || v }));
  p.append(changedMark(rowChoice('Resolution', resList, eff.resolution || rs.resolution, v => save({ resolution: v }), 'The game picture is scaled to fit (never stretched).'), 'recording.resolution'));
  const fpsList = (caps.fps?.length ? caps.fps : [30, 60]).map(v => ({ v, l: v + ' fps' }));
  p.append(changedMark(rowChoice('Frame rate', fpsList, eff.fps || rs.fps, v => save({ fps: +v })), 'recording.fps'));
  p.append(changedMark(rowChoice('Quality', [{ v: 'standard', l: 'Standard' }, { v: 'high', l: 'High' }, { v: 'max', l: 'Maximum' }, { v: 'custom', l: 'Custom bit rate' }], rs.quality || 'high', v => save({ quality: v }),
    `About ${(info.bitrate / 1000).toFixed(info.bitrate < 10000 ? 1 : 0)} Mbit/s, roughly ${fmtBytes(info.bitrate * 1000 / 8 * 60)} per minute.`), 'recording.quality'));
  if (rs.quality === 'custom') p.append(changedMark(rowRange('Bit rate', { min: 2000, max: 80000, step: 1000, value: rs.bitrate || 12000, fmt: v => (v / 1000) + ' Mbit/s', onChange: (v, commit) => { if (commit) saveSettings({ recording: { bitrate: v } }); } }), 'recording.bitrate'));
  const codecs = [{ v: 'h264', l: 'H.264 (plays everywhere)' }];
  if (caps.hardwareHevc) codecs.push({ v: 'hevc', l: 'HEVC / H.265 (smaller files)' });
  p.append(changedMark(rowChoice('Video format', codecs, codecs.some(c => c.v === rs.codec) ? rs.codec : 'h264', v => save({ codec: v }), 'MP4 file. HEVC is offered only when your graphics card can encode it.'), 'recording.codec'));
  p.append(changedMark(rowToggle('Record game sound', rs.audio !== false, v => save({ audio: v }), caps.processAudio ? 'Only the game’s sound — not other apps.' : 'This version of Windows cannot record one app’s sound on its own, so everything you hear is recorded.'), 'recording.audio'));
  p.append(changedMark(rowToggle('Record microphone', !!rs.mic, v => save({ mic: v }), 'Your default microphone, as a separate audio track.'), 'recording.mic'));
  p.append(changedMark(rowButton('Save recordings to', { value: rs.folder ? '' : 'Media › Recordings', sub: info.folder, onClick: async () => {
    const r = await api('/api/recording/folder', { body: {} }).catch(e => { toast(e.message, I.warn); return null; });
    if (r?.path) save({ folder: r.path });
  } }), 'recording.folder'));
  if (rs.folder) p.append(rowButton('Use Media › Recordings again', { onClick: () => save({ folder: '' }) }));
}
