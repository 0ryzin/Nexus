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
  async load(names) {
    if (!this.ctx) return;
    await Promise.all(names.map(async n => {
      try { const r = await fetch(`/res/sounds/${n}.wav`); this.buf[n] = await this.ctx.decodeAudioData(await r.arrayBuffer()); } catch { }
    }));
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
    this.cur = el;
    el.classList.add('focus');
    if (el.matches('input[type=text]') && S.lastInput !== 'mouse') el.focus({ preventScroll: true });
    else if (document.activeElement && document.activeElement !== el && document.activeElement.matches('input')) document.activeElement.blur();
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
    if (el.matches('input[type=text]')) { el.focus(); return; }
    Sound.play('select');
    el.click();
  },
  back() {
    const l = this.top(); if (!l) return;
    if (document.activeElement?.matches('input[type=text]') && S.lastInput === 'keyboard' && l.root.contains(document.activeElement)) { document.activeElement.blur(); return; }
    if (l.onBack) { Sound.play('back'); l.onBack(); }
  },
  options() { const l = this.top(); if (l?.onOptions) { Sound.play('select'); l.onOptions(this.cur); } },
  tab(d) { const l = this.top(); if (l?.onTab) { Sound.play('move'); l.onTab(d); } },
};

function setInputMode(mode) {
  if (S.lastInput === mode) return;
  S.lastInput = mode;
  document.body.classList.toggle('no-cursor', mode === 'gamepad');
  renderHints();
}

document.addEventListener('keydown', e => {
  if (document.body.classList.contains('booting')) { Boot.skip(); e.preventDefault(); return; }
  setInputMode('keyboard');
  const t = e.target;
  const typing = t && t.matches && t.matches('input[type=text]');
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
document.addEventListener('dragstart', e => e.preventDefault());

// left/right adjust for sliders and option pickers
function adjust(d) {
  const el = Nav.cur; if (!el) return false;
  if (el._adjust) { el._adjust(d); return true; }
  return false;
}

// Gamepad polling (Gamepad API, standard mapping). Works with XInput, DualShock/DualSense and most HID pads.
const Pad = {
  prev: {}, held: {}, repeatAt: {}, connected: [],
  mapping() { return (S.profile?.controls?.mapping) || { confirm: 0, back: 1, options: 3, search: 2, prevTab: 4, nextTab: 5, menu: 9, home: 16, view: 8 }; },
  poll() {
    const pads = navigator.getGamepads ? [...navigator.getGamepads()].filter(Boolean) : [];
    const ids = pads.map(p => p.index + ':' + p.id).join('|');
    if (ids !== this.connected.join('|')) {
      const before = this.connected.length;
      this.connected = pads.map(p => p.index + ':' + p.id);
      S.pads = pads.map(p => ({ index: p.index, id: p.id, mapping: p.mapping }));
      renderTopbar(); renderHints(); Controllers.refreshList();
      if (pads.length > before && !document.body.classList.contains('booting')) { Sound.play('connect'); toast(`Controller connected`, I.pad); }
    }
    const pref = S.profile?.controls?.preferredPad;
    const active = pads.filter(p => !pref || p.id === pref);
    const use = active.length ? active : pads;
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
      const prev = this.prev[key] || {};
      const state = { ...dirs, ...acts };
      if (Mapper.listening) {
        for (let i = 0; i < p.buttons.length; i++) if (b(i) && !(prev['b' + i])) { Mapper.capture(i); }
        const raw = {}; for (let i = 0; i < p.buttons.length; i++) raw['b' + i] = b(i);
        this.prev[key] = { ...state, ...raw }; Controllers.liveUpdate(p); continue;
      }
      const any = Object.values(state).some(Boolean);
      if (any) setInputMode('gamepad');
      if (document.body.classList.contains('booting')) { if (any && !Object.values(prev).some(Boolean)) Boot.skip(); this.prev[key] = state; continue; }
      for (const d of ['up', 'down', 'left', 'right']) {
        if (state[d] && !prev[d]) { this.repeatAt[key + d] = now + 380; this.dir(d); }
        else if (state[d] && now >= (this.repeatAt[key + d] || Infinity)) { this.repeatAt[key + d] = now + repeatMs; this.dir(d); }
      }
      const edge = a => state[a] && !prev[a];
      if (edge('confirm')) Nav.activate();
      if (edge('back')) Nav.back();
      if (edge('options')) Nav.options();
      if (edge('search')) { if (S.screen === 'library') { Nav.setFocus($('#lib-search')); $('#lib-search').focus(); } else if (S.screen === 'home') showLibrary(true); }
      if (edge('prevTab')) Nav.tab(-1);
      if (edge('nextTab')) Nav.tab(1);
      if (edge('menu')) { if (!Nav.top()?.dialog) openPower(); }
      if (edge('home')) goHome({ button: true });
      this.prev[key] = state;
      Controllers.liveUpdate(p);
    }
    requestAnimationFrame(() => this.poll());
  },
  dir(d) { if ((d === 'left' || d === 'right') && adjust(d === 'left' ? -1 : 1)) return; Nav.move(d); },
  rumble(ms = 120, strong = .4) {
    if (!S.profile?.controls?.vibration) return;
    for (const p of navigator.getGamepads ? navigator.getGamepads() : []) {
      p?.vibrationActuator?.playEffect?.('dual-rumble', { duration: ms, strongMagnitude: strong, weakMagnitude: .6 }).catch?.(() => { });
    }
  },
};
window.addEventListener('gamepadconnected', () => { });

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
    b.classList.toggle('has-wp', !!(anim || this.wp.file));
    box.style.backgroundImage = still ? `url("${this.url(still)}")` : '';
    const fit = { cover: ['cover', 'no-repeat'], contain: ['contain', 'no-repeat'], stretch: ['100% 100%', 'no-repeat'], center: ['auto', 'no-repeat'], tile: ['auto', 'repeat'] }[this.wp.fit || 'cover'];
    box.style.backgroundSize = fit[0]; box.style.backgroundRepeat = fit[1];
    if (anim || this.wp.file) {
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
  async preview() {
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
    const el = this.build(logo);
    // make sure the artwork is decoded before the animation starts
    const missing = new Set();
    await Promise.all(['emblem-l.png', 'emblem-r.png', 'word-nexus.png', 'word-switch.png', 'logo-full.png'].map(n => new Promise(r => { const i = new Image(); i.onload = r; i.onerror = () => { missing.add(n); r(); }; i.src = '/assets/brand/' + n; })));
    // fallback: if a wordmark image is unavailable, draw the word with the same typeface (Outfit) as live text
    if (missing.has('word-nexus.png')) el.wn.replaceWith(el.wn = h('div', { class: 'bm wn txt' }, 'NEXUS'));
    if (missing.has('word-switch.png')) el.ws.replaceWith(el.ws = h('div', { class: 'bm ws txt' }, 'SWITCH'));
    if (!st.enabled && !force) { this.finishNow(); return; }
    const short = st.style === 'short';
    const T = short ? .7 : 1;
    const reduced = S.anim === 'off';
    const E = (n, kf, o) => { const a = n.animate(kf, { fill: 'both', ...o }); this.anims.push(a); return a; };
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
async function loadState() {
  const st = await api('/api/state');
  Object.assign(S, { app: st.app, settings: st.settings, profiles: st.profiles || [], emulator: st.emulator, engine: st.engine || {}, formats: st.formats, scanDirs: st.scanDirs, gamesDir: st.gamesDir, avatarColors: st.avatarColors, avatarGlyphs: st.avatarGlyphs, maxProfiles: st.maxProfiles || 8, library: st.library });
  if (S.profile) S.profile = profileById(S.profile.id) || null;
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
  for (const s of ['who', 'home', 'library', 'settings', 'controllers', 'playing']) $('#' + s).hidden = s !== name;
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
  if (S.playing && !S.playing.suspended) {
    if (opts.button && S.playing.phase === 'playing') { suspendGame(); return; } // Home while playing: suspend, then show Home
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
  const lbl = i => ({ 0: 'A', 1: 'B', 2: 'X', 3: 'Y', 4: 'LB', 5: 'RB', 6: 'LT', 7: 'RT', 8: 'View', 9: 'Menu', 10: 'LS', 11: 'RS', 16: 'Home' }[i] ?? ('B' + i));
  const items = [];
  const l = Nav.top();
  const k = (padIdx, kb) => pad ? h('span', { class: 'key' }, lbl(padIdx)) : h('span', { class: 'key kb' }, kb);
  if (l?.onOptions) items.push([k(map.options, 'X'), 'Options', () => Nav.options()]);
  if (S.screen === 'home' && !l?.dialog) items.push([k(map.menu, 'Esc'), 'Power', () => openPower()]);
  if (l?.onBack && (l.dialog || S.screen !== 'home')) items.push([k(map.back, 'Esc'), 'Back', () => Nav.back()]);
  items.push([k(map.confirm, 'Enter'), 'OK', () => Nav.activate()]);
  list.innerHTML = '';
  for (const [key, text, fn] of items) { const e = h('span', { class: 'hint' }, key, text); e.addEventListener('click', fn); list.append(e); }
  const hp = $('#hint-pad'); hp.innerHTML = '';
  hp.append(svg(I.pad), S.pads.length ? `${S.pads.length} controller${S.pads.length > 1 ? 's' : ''}` : 'Keyboard & mouse');
}

// ---------- who's playing
function showWho() {
  if (!S.profiles.length) { setupWizard({ first: true }); return; } // first launch: console-style setup
  const list = $('#who-list'); list.innerHTML = '';
  for (const p of S.profiles) {
    const last = p.id === S.settings.lastProfileId;
    const b = h('button', { class: 'who-item' + (last ? ' current' : ''), 'data-nav': true, title: p.name }, avatarEl(p), h('span', {}, p.name), last ? h('i', { class: 'last' }, 'LAST') : '');
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
  maybeOfferEngineImport();
}

// ---------- data from other emulators (NEXUS's own import; nothing is needed from those programs)
let engineImportAsked = false;
function describeSource(src) {
  const items = src.items.map(i => `• ${i.label} — ${i.files} file${i.files === 1 ? '' : 's'}, ${fmtBytes(i.bytes)}`);
  if (src.saves) items.push(`• Saves (${src.saves} files) — offered afterwards for each user, in User settings`);
  return `${src.name}: ${src.path}\n` + items.join('\n');
}
async function maybeOfferEngineImport(force = false) {
  if (engineImportAsked && !force) return;
  let st; try { st = await api('/api/engine-data'); } catch { return; }
  if (!st.ask && !force) return;
  engineImportAsked = true;
  if (!st.sources.length) { alertDlg('Nothing to import', 'No data from other Switch emulators was found on this PC. NEXUS has set up its own engine data folder and configuration.'); return; }
  const interrupted = st.record.state === 'interrupted' || st.record.state === 'error';
  const src = st.sources.find(x => x.id === st.record.source) || st.sources[0];
  const text = (interrupted
    ? `The last import from ${src.name} did not finish${st.record.error ? ' (' + st.record.error + ')' : ''}. Nothing was lost: what was copied is kept, and trying again copies only what is missing.\n\n`
    : 'NEXUS found data from another Switch emulator on this PC. NEXUS can copy it into its own engine data, so your keys, firmware and installed updates work in NEXUS straight away.\n\n')
    + describeSource(src)
    + '\n\nThe original files are only read, never changed or moved. Files NEXUS already has are kept (and backed up). You can also start fresh and import later in Settings › System files.';
  const buttons = [{ label: 'Later', value: 'later' }, { label: 'Start fresh', value: 'fresh' }];
  if (st.sources.length > 1) buttons.push({ label: 'Choose another…', value: 'choose' });
  buttons.push({ label: interrupted ? 'Try again' : `Import from ${src.name}`, value: 'import', primary: true });
  const v = await dialog({ title: interrupted ? 'Finish importing your emulator data?' : 'Bring in your emulator data?', text, buttons, cancelValue: 'later' });
  if (v === 'fresh') { await api('/api/engine-data/fresh', { body: {} }).catch(e => toast(e.message, I.warn)); toast('NEXUS starts fresh. You can import later in System files.', I.check, 3500); }
  if (v === 'import') startEngineImport(src.id);
  if (v === 'choose') {
    const pick = await dialog({ title: 'Import from', text: st.sources.map(describeSource).join('\n\n'), buttons: [{ label: 'Cancel', value: null }, ...st.sources.map(x => ({ label: x.name, value: x.id }))] });
    if (pick) startEngineImport(pick);
  }
}
async function startEngineImport(id) {
  try { await api('/api/engine-data/import', { body: { source: id } }); toast('Importing… you can keep using NEXUS.', I.info, 2500); }
  catch (e) { alertDlg('Could not import', e.message); }
}
function engineDataSection(p) {
  const box = h('div'); p.append(h('h3', {}, 'Data from other emulators'), box);
  api('/api/engine-data').then(st => {
    const r = st.record;
    const state = st.running ? `Importing… ${Math.round(st.progress * 100)}%` : r.state === 'interrupted' ? 'Import interrupted — can be retried'
      : r.state === 'error' ? 'Import failed — can be retried' : r.decision === 'imported' ? `Imported from ${st.sources.find(x => x.id === r.source)?.name || r.source}${r.copied ? ` (${r.copied} files)` : ''}`
      : r.decision === 'fresh' ? 'Started fresh (nothing imported)' : st.sources.length ? 'Not decided yet' : 'None found on this PC';
    box.append(infoRow('NEXUS engine data', st.ready ? 'Set up' : 'Not set up', st.dataDir));
    box.append(infoRow('Import', state, r.error || (r.backup ? `Backup of what NEXUS had: ${r.backup}` : '')));
    for (const src of st.sources) {
      box.append(rowButton(`Import from ${src.name}…`, { sub: `${src.path} · ${src.items.map(i => i.label.split(' (')[0]).join(', ') || 'saves only'}`, disabled: st.running, onClick: async () => {
        if (await confirmDlg(`Import from ${src.name}?`, describeSource(src) + '\n\nOriginals are only read. Files NEXUS already has are kept.', 'Import')) { await startEngineImport(src.id); rebuildPanel(); }
      } }));
    }
    if (st.sources.length && r.decision !== 'fresh' && !st.running) box.append(rowButton('Start fresh', { sub: 'Use NEXUS without importing. Nothing is deleted; you can import later.', onClick: async () => { await api('/api/engine-data/fresh', { body: {} }); rebuildPanel(); } }));
  }).catch(e => box.append(h('p', { class: 'note warn' }, e.message)));
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
  const draw = () => {
    root.replaceChildren();
    root.style.setProperty('--accent', d.accent);
    const dots = h('div', { class: 'setup-steps' }, steps.map((t, i) => h('span', { class: i === step ? 'on' : i < step ? 'done' : '' }, h('i'), t)));
    const head = h('header', { class: 'setup-head' }, h('div', { class: 'setup-brand' }), h('h1', {}, first && step === 0 ? 'Welcome to NEXUS' : first ? 'Create your user' : 'Add a user'), dots);
    const card = h('section', { class: 'setup-card' });
    root.append(head, card);
    let initial = null;
    if (step === 0) {
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

async function maybeOfferSaveImport(pid) {
  try {
    const s = await api(`/api/profiles/${pid}/saves`);
    if (s.managed && s.nandSaves && s.nandSaves.length && !s.titles.length) {
      const ok = await confirmDlg('Existing saves found', `Save data for ${s.nandSaves.length} game(s) was found from the emulator NEXUS used before. Copy those saves into this user? The originals are kept untouched.`, 'Copy saves');
      if (ok) { const r = await api(`/api/profiles/${pid}/import-saves`, { body: { overwrite: false } }); toast(`Copied ${r.files} save file(s)`, I.check); }
    }
  } catch { }
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
    body.append(h('h3', {}, 'Save data'));
    const saveRow = rowButton('Save data', { sub: 'Loading…' });
    body.append(saveRow);
    api(`/api/profiles/${id}/saves`).then(s => {
      saveRow.querySelector('small').textContent = s.managed ? `${s.titles.length} game(s), ${fmtBytes(s.bytes)} — NEXUS data\\Saves\\${s.folderName}` : 'Per-user saves are turned off (Settings › Game sessions & saves).';
      if (s.nandSaves?.length) {
        const imp = rowButton('Copy earlier saves into this user', { sub: `${s.nandSaves.length} game(s) found in the save folder of the emulator NEXUS used before. Originals are never removed.`, onClick: () => importSaves(id) });
        saveRow.after(imp);
      }
    }).catch(() => { });
    body.append(rowButton('Open save folder', { sub: 'One folder per game, named after the game. You can drop your own save files into them.', onClick: () => api(`/api/profiles/${id}/saves/open`, { body: {} }).catch(e => toast(e.message, I.warn)) }));
    body.append(rowButton('Export saves…', { sub: 'Downloads a ZIP of this user’s game folders.', onClick: () => { const a = h('a', { href: `/api/profiles/${id}/saves/export`, download: '' }); document.body.append(a); a.click(); a.remove(); toast('Export started', I.check); } }));
    body.append(rowButton('Import saves from a ZIP…', { sub: 'Adds the game folders from an export. Existing saves are never replaced.', onClick: async () => {
      const f = await pickFile('.zip,application/zip'); if (!f) return;
      try { const r = await api(`/api/profiles/${id}/saves/import`, { body: f }); toast(`${r.folders} game folder(s) imported`, I.check); rebuild(); } catch (e) { alertDlg('Could not import', e.message); }
    } }));
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

async function importSaves(id, overwrite = false) {
  try {
    const r = await api(`/api/profiles/${id}/import-saves`, { body: { overwrite } });
    toast(`Copied ${r.files} save file(s)`, I.check);
  } catch (e) {
    if (e.status === 409 && !overwrite) {
      const ok = await confirmDlg('Replace existing saves?', `${e.message}\n\nThis user's current saves will be backed up to NEXUS data\\Archived first.`, 'Replace', true);
      if (ok) return importSaves(id, true);
    } else alertDlg('Could not copy saves', e.message);
  }
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

function tickClock() {
  const c = $('#tb-clock'); if (c) c.textContent = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
async function pollSystem() {
  try { S.system = await api('/api/system'); renderTopbar(); if (S.screen === 'home') markTopFocus(); } catch { }
}
function markTopFocus() { if (Nav.cur && Nav.cur.closest('#tb-profiles')) { const p = Nav.cur._profile; const b = $$('#tb-profiles button').find(x => x._profile?.id === p?.id); if (b) Nav.setFocus(b, { silent: true }); } }

// ---------- home
function homeGames() {
  const sort = S.profile?.sort || 'recent';
  const playable = S.games.filter(g => isPlayable(g) && !g.hidden);
  const recentSet = new Set(S.recent);
  const recent = S.recent.map(gameByKey).filter(g => g && isPlayable(g));
  let rest = playable.filter(g => !recentSet.has(g.key));
  if (sort === 'alpha') return [...playable].sort((a, b) => a.title.localeCompare(b.title)).slice(0, 12);
  if (sort === 'playtime') return [...playable].sort((a, b) => (S.stats[b.key]?.seconds || 0) - (S.stats[a.key]?.seconds || 0)).slice(0, 12);
  rest = rest.sort((a, b) => a.title.localeCompare(b.title));
  return [...recent, ...rest].slice(0, 12);
}

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
  if (g.kind === 'update') t.append(h('span', { class: 'badge' }, 'UPDATE'));
  if (g.kind === 'dlc') t.append(h('span', { class: 'badge' }, 'DLC'));
  if (g.perGame) t.append(h('span', { class: 'cfg-dot', title: 'Has its own game settings' }));
  t._game = g;
  t.addEventListener('contextmenu', e => { e.preventDefault(); if (!t.hasAttribute('data-nav') && !t.closest('[data-nav]')) return; gameMenu(g.key); });
  return t;
}

function renderHome() {
  renderTopbar();
  const row = $('#tiles'); row.innerHTML = '';
  row.dataset.row = 'strict';
  const games = homeGames();
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
  games.forEach((g, i) => {
    const t = gameTile(g); t.style.setProperty('--i', i);
    t.addEventListener('click', () => launchGame(g.key));
    t.addEventListener('navfocus', () => updateSelLabel(g));
    t.addEventListener('navblur', () => updateSelLabel(null));
    row.append(t);
  });
  if (S.games.length) {
    const all = h('button', { class: 'tile all', 'data-nav': true, style: { '--i': games.length } }, svg(I.grid, 'all-ic'), h('span', { class: 'all-n' }, `All Software (${S.games.filter(isPlayable).length})`));
    all.addEventListener('click', () => showLibrary());
    all.addEventListener('navfocus', () => { const l = $('#sel-label'); l.querySelector('.sel-title').textContent = 'All Software'; l.querySelector('.sel-meta').textContent = ''; });
    row.append(all);
  }
  // dock
  const dock = $('#dock'); dock.innerHTML = ''; dock.dataset.row = 'strict';
  const items = [
    ['Library', I.grid, '#ff4f4f', () => showLibrary()],
    ['Users', I.person, '#ff9f0a', () => showWho()],
    ['Controllers', I.pad, '#30d158', () => Controllers.show()],
    ['Themes', I.palette, '#bf5af2', () => showSettings('appearance')],
    ['System & Emulation', I.chip, '#64d2ff', () => showSettings('sysfiles')],
    ['System Settings', I.gear, 'var(--muted)', () => showSettings()],
    ['Power', I.power, 'var(--muted)', () => openPower()],
  ];
  items.forEach(([label, icon, col, fn], i) => {
    const b = h('button', { class: 'dock-btn', 'data-nav': true, style: { '--ic': col, '--i': i }, 'aria-label': label }, svg(icon), h('span', { class: 'tip' }, label));
    b.addEventListener('click', fn);
    b.addEventListener('navfocus', () => { $('#dock-label').textContent = label; });
    b.addEventListener('navblur', () => { $('#dock-label').textContent = ''; });
    b.addEventListener('mouseenter', () => { $('#dock-label').textContent = label; });
    b.addEventListener('mouseleave', () => { if (!b.classList.contains('focus')) $('#dock-label').textContent = ''; });
    dock.append(b);
  });
  renderBanner();
}

function renderBanner() {
  const b = $('#home-banner'); b.innerHTML = '';
  const en = S.engine || {};
  let msg = null, action = null, label = '';
  if (S.playing?.suspended) {
    b.hidden = false; b.innerHTML = '';
    const ic = svg(I.play); ic.style.cssText = 'width:1.4em;height:1.4em;fill:var(--accent);flex:none';
    const resume = h('button', { class: 'btn primary', 'data-nav': true }, 'Resume');
    const close = h('button', { class: 'btn', 'data-nav': true }, 'Close game');
    resume.addEventListener('click', () => resumeGame());
    close.addEventListener('click', async () => { if (await confirmDlg(`Close ${S.playing.title}?`, 'Unsaved progress will be lost.', 'Close', true)) api('/api/emulator/stop', { body: {} }).catch(e => toast(e.message, I.warn)); });
    b.append(ic, h('span', { class: 'grow' }, h('b', {}, `${S.playing.title} is paused. `), 'It keeps its place until you resume it.'), resume, close);
    return;
  }
  if (en.job?.active) { msg = h('span', { class: 'grow' }, h('b', {}, 'Installing the NEXUS engine… '), en.job.message || ''); }
  else if (en.checking) { /* the engine is being verified and loaded in the background */ }
  else if (!en.installed) { msg = h('span', { class: 'grow' }, h('b', {}, 'The NEXUS engine cannot be used. '), en.problem || 'Extract the complete NEXUS ZIP again.'); label = 'Details'; action = () => showSettings('engine'); }
  else if (S.games.some(g => g.kind !== 'homebrew') && !en.keys) { msg = h('span', { class: 'grow' }, h('b', {}, 'System files needed. '), 'Install the prod.keys and firmware from your own console to play commercial games.'); label = 'Install'; action = () => showSettings('sysfiles'); }
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

Screens.home = {
  onBack: () => openPower(),
  onOptions: cur => { if (cur?._game) gameMenu(cur._game.key); else if (cur?._profile) editProfile(cur._profile.id); },
};

// ---------- power menu
async function openPower() {
  if (Nav.top()?.name === 'power') { Nav.top().close(); return; }
  const fs = S.system.fullscreen ?? S.settings.display?.fullscreen;
  const items = [
    { v: 'users', l: 'Switch user' },
    { v: 'fs', l: fs ? 'Switch to windowed mode' : 'Switch to full screen' },
    { v: 'min', l: 'Minimise NEXUS' },
    { v: 'replay', l: 'Replay startup animation' },
    { v: 'quit', l: 'Exit NEXUS' },
  ];
  const v = await listPick('Power options', items);
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
// ---------- game options menu
function gameMenu(key) {
  const g = gameByKey(key); if (!g) return;
  if (Nav.top()?.name === 'gamemenu') Nav.top().close();
  const items = [
    isPlayable(g) && !g.hidden ? ['play', 'Play', I.play] : null,
    isPlayable(g) ? ['config', 'Configure Game Settings', I.gear] : null,
    ['art', 'Edit Game Artwork', I.palette],
    ['info', 'Game Information', I.info],
    ['open', 'Open Game Location', I.folder],
    ['refresh', 'Refresh Artwork & Information', I.refresh],
    g.hidden ? ['restore', 'Restore to Library', I.plus] : ['remove', 'Remove from Library', I.trash],
  ].filter(Boolean);
  const tile = gameTile(g); tile.removeAttribute('data-nav');
  const btns = items.map(([v, l, ic]) => { const b = h('button', { 'data-nav': true, class: v === 'remove' ? 'danger' : '' }, svg(ic), h('span', {}, l)); b._v = v; return b; });
  const box = h('div', { class: 'dialog game-menu' }, h('div', { class: 'gm-head' }, tile, h('div', {}, h('b', {}, g.title), h('small', {}, [g.titleId, g.format.toUpperCase(), g.perGame ? 'Custom settings' : ''].filter(Boolean).join(' · ')))), h('div', { class: 'gm-list' }, btns));
  const layer = openLayer(box, { name: 'gamemenu', initial: btns[0] });
  btns.forEach(b => b.addEventListener('click', async () => {
    layer.close();
    switch (b._v) {
      case 'play': launchGame(key); break;
      case 'config': gameConfig(key); break;
      case 'art': artEditor(key); break;
      case 'info': gameInfo(key); break;
      case 'open': api(`/api/games/${key}/reveal`, { body: {} }).catch(e => toast(e.message, I.warn)); break;
      case 'refresh': await api(`/api/games/${key}/refresh`, { body: {} }); toast('Refreshing game information and artwork…', I.refresh); break;
      case 'remove': if (await confirmDlg(`Remove ${g.title}?`, 'It will be hidden from your library. The game file is not deleted; restore it any time from All Software › Removed.', 'Remove', true)) { await api(`/api/games/${key}/hidden`, { body: { hidden: true } }); await loadGames(); refreshViews(); toast('Removed from library', I.check); } break;
      case 'restore': await api(`/api/games/${key}/hidden`, { body: { hidden: false } }); await loadGames(); refreshViews(); toast('Restored to library', I.check); break;
    }
  }));
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
    body.append(h('p', { class: 'note' }, 'Artwork is stored in NEXUS data\\GameProfiles\\' + (g.titleId || 'file-…') + '. Game files are never modified.'));
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
  else if (st.kind === 'int') for (let x = st.min; x <= st.max; x += st.step) out.push({ v: String(x), l: x + (st.suffix || '') });
  if (only?.length) out = out.filter(c => only.includes(c.v));
  return out;
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
function settingRows(container, view, section, { mode, engine, save }) {
  const engines = view.engines.map(e => e.id);
  for (const st of view.settings.filter(x => x.section === section)) {
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
        container.append(r);
        continue;
      }
      if (st.kind === 'info' || !cap.available) {
        container.append(infoRow(st.label, cap.available ? '' : 'Not available', cap.note || st.help || '', cap.available ? '' : 'unavail'));
        continue;
      }
      if (!cap.perGame) {
        const [gv] = effValue(view, id, engine, false);
        container.append(infoRow(st.label, nlabel(st, gv), (cap.note || '') + ' Change it in Settings › Emulation settings.', 'unavail'));
        continue;
      }
      const [gv, gsrc] = effValue(view, id, engine, false);
      const glabel = gv !== undefined ? nlabel(st, gv) : 'engine default';
      const ch = [{ v: '__global', l: `Use global (${glabel})` }, ...nchoices(st, cap.choices)];
      const cur = view.overrides[id];
      if (cur !== undefined && !ch.some(c => c.v === cur)) ch.push({ v: cur, l: nlabel(st, cur) + ' (not supported by the engine, ignored)' });
      let sub = st.help || '';
      if (gsrc === 'engine') sub = (sub ? sub + ' ' : '') + 'Global value comes from the engine configuration.';
      const r = rowChoice(st.label, ch, cur ?? '__global', v => save({ settings: { [id]: v === '__global' ? null : v } }), sub);
      if (cur !== undefined) r.classList.add('overridden');
      container.append(r);
    } else {
      if (id === 'graphics.resolution') { container.append(infoRow(st.label, resolutionOf(view.global['graphics.scale'] || '1', view.global['system.docked'] || 'docked'), st.help)); continue; }
      if (id === 'system.firmware') { container.append(infoRow(st.label, view.firmware || 'Not installed', 'Managed in Settings › System files.')); continue; }
      if (id === 'controls.profile') { container.append(infoRow(st.label, '', 'Each user picks their controller profile in Controllers; a game can override it in Configure Game Settings.')); continue; }
      if (id === 'audio.volume') { container.append(rowVolume(st.label, +(view.global[id] ?? 100), sendVolume, st.help)); continue; }
      const sup = engines.filter(e => view.caps[e][id]?.available);
      if (st.kind === 'info' || !sup.length) {
        const note = engines.map(e => view.caps[e][id]?.note).find(Boolean) || st.help;
        container.append(infoRow(st.label, sup.length ? '' : 'Not available', note, sup.length ? '' : 'unavail'));
        continue;
      }
      const engineNow = engines.map(e => view.caps[e][id]?.available ? `Engine default: ${view.engineValues[e]?.[id] !== undefined ? nlabel(st, view.engineValues[e][id]) : 'built-in'}` : 'Not supported').join(' · ');
      const ch = [{ v: '__engine', l: 'Engine default' }, ...nchoices(st, view.caps[engines[0]][id]?.choices)];
      const cur = view.global[id];
      let sub = (st.help ? st.help + ' ' : '') + engineNow;
      const r = rowChoice(st.label, ch, cur ?? '__engine', v => save({ settings: { [id]: v === '__engine' ? null : v } }), sub);
      if (cur !== undefined) r.classList.add('overridden');
      container.append(r);
    }
  }
}

async function gameConfig(key, startSection = 'general') {
  const g = gameByKey(key); if (!g) return;
  let view;
  try { view = await api(`/api/games/${key}/settings`); } catch (e) { alertDlg('Could not load game settings', e.message); return; }
  let section = startSection;
  const root = h('div', { class: 'cfg-page' });
  const layer = openLayer(root, { name: 'gameconfig', onTab: d => { const i = view.sections.findIndex(x => x.v === section); section = view.sections[(i + d + view.sections.length) % view.sections.length].v; draw(true); } });
  const nav = h('nav', { class: 'cfg-nav' });
  const body = h('div', { class: 'cfg-body' });
  const head = h('header', { class: 'cfg-head' });
  root.append(head, h('div', { class: 'cfg-main' }, nav, body));
  let saving = null;
  const save = async patch => {
    try {
      view = await api(`/api/games/${key}/settings`, { body: patch });
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
  const drawNav = () => {
    nav.replaceChildren();
    for (const sct of view.sections) {
      const cnt = sct.v === 'general' ? 0 : view.settings.filter(x => x.section === sct.v && view.overrides[x.id] !== undefined).length + (sct.v === 'controls' && view.inputProfile ? 1 : 0);
      const b = h('button', { class: 'cat' + (sct.v === section ? ' sel' : ''), 'data-nav': true }, sct.l, cnt ? h('small', { class: 'cnt' }, String(cnt)) : '');
      b._sec = sct.v;
      b.addEventListener('click', () => { section = sct.v; draw(true); });
      b.addEventListener('navfocus', () => { if (S.lastInput !== 'mouse' && section !== sct.v) { section = sct.v; drawBody(); drawNav(); Nav.setFocus($$('.cat', nav).find(x => x._sec === sct.v), { silent: true }); } });
      nav.append(b);
    }
  };
  const drawBody = () => {
    body.replaceChildren();
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
      body.append(h('h3', {}, 'Compatibility'));
      body.append(compatRow(view.compat, g, r => { view.compat = r; drawBody(); }));
      body.append(h('h3', {}, 'Optimise for my device'));
      body.append(rowButton('Optimise this game…', { sub: 'Settings for this game only, from your hardware and what is known about the game. Other games are not changed.', onClick: () => {
        sheet(`Optimise ${g.title}`, (sb) => { const ob = h('div'); sb.append(ob); optimiserPanel(ob, key, async () => { view = await api(`/api/games/${key}/settings`); draw(false); }); });
      } }));
      if (!S.engine?.installed) body.append(h('p', { class: 'note warn' }, 'The NEXUS engine is not installed yet. Install it in Settings › NEXUS Engine.'));
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
    if (!view.perGameOk) { body.append(h('p', { class: 'note warn' }, view.perGameReason)); return; }
    const note = { enhance: 'Applies only to this game. Frame generation suits games that already run at a steady frame rate.', graphics: 'Applies only to this game. “Use global” follows Settings › Emulation settings.', advanced: 'For advanced users. If a game misbehaves, use Restore defaults on the General page.',
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
    embedded: 'Using the icon stored inside the game.', online: 'Using artwork downloaded for this Title ID.', 'game-list-cache': 'Using the icon from the game-list cache of the emulator NEXUS used before.',
  }[g.artSource] || 'No artwork yet. NEXUS reads the icon from the game file (needs prod.keys), downloads it by Title ID, or uses an image you choose.';
}
function refreshViews() { if (S.screen === 'home') { const cur = Nav.cur; const k = cur?._game?.key; const dockIdx = cur ? $$('#dock .dock-btn').indexOf(cur) : -1; const topIdx = cur ? $$('#tb-profiles button').indexOf(cur) : -1; const inHome = cur && $('#home').contains(cur); renderHome(); if (Nav.top()?.dialog) return; let t = null; if (dockIdx >= 0) t = $$('#dock .dock-btn')[dockIdx]; else if (topIdx >= 0) t = $$('#tb-profiles button')[topIdx]; else t = $$('#tiles .tile').find(t => t._game?.key === k) || (inHome || !cur ? $('#tiles .tile') : null); if (t) Nav.setFocus(t, { silent: true }); } if (S.screen === 'library') { const k = Nav.cur?._game?.key; const keep = renderLibrary(k); if (keep) Nav.setFocus(keep, { silent: true }); } }

// ---------- launching: Who's playing? → the game opens → NEXUS loading screen → the game
async function launchGame(key, force = false) {
  const g = gameByKey(key); if (!g) return;
  if (!isPlayable(g)) { alertDlg('Not a game', 'This file is an update or DLC. Keep it in one of your game folders: NEXUS applies it automatically when you start the base game.'); return; }
  if (!S.profile) { showWho(); return; }
  if (S.playing?.suspended) {
    if (S.playing.gameKey === key) { resumeGame(); return; }
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
    const st = await api('/api/launch', { body: { key, profileId: S.profile.id, force } });
    Sound.play('launch'); Pad.rumble(160, .5);
    S.playing = st;
    Launch.open(g, st);   // Stage B + C; Stage D when the engine shows the game
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
      setTimeout(() => { layer.close(); resolve(pid); }, S.anim === 'off' ? 0 : 420);
    };
    for (const p of S.profiles) {
      const a = avatarEl(p, 'wp-avatar');
      const b = h('button', { class: 'wp-user', 'data-nav': true, style: { '--pc': p.theme?.accent || 'var(--accent)' } }, a, h('span', {}, p.name));
      b.addEventListener('click', () => finish(p.id, b));
      row.append(b);
    }
    const cur = $$('.wp-user', row)[S.profiles.findIndex(p => p.id === S.profile?.id)] || $('.wp-user', row);
    Nav.setFocus(cur, { silent: true });
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
  STAGES: { preparing: ['Preparing your session', 0], loading: ['Loading the game', 1], starting: ['Starting', 2] },
  open(g, st) {
    this.close(true);
    this.key = g.key; this.t0 = performance.now();
    const ghost = launchArt(g, 'lx-ghost'), art = launchArt(g, 'lx-art');
    const steps = h('div', { class: 'lx-steps' }, Object.values(this.STAGES).map(([t]) => h('span', {}, h('i'), t)));
    const el = h('div', { class: 'launchfx' + (S.anim === 'off' ? ' no-anim' : ''), role: 'status' },
      h('div', { class: 'lx-backdrop', style: g.artSource ? { backgroundImage: `url("/art/${g.key}?v=${g.artRev || 0}&w=640")` } : {} }),
      h('div', { class: 'lx-stage' }, h('div', { class: 'lx-artwrap' }, ghost, art),
        h('div', { class: 'lx-info' }, h('h1', {}, g.title), h('div', { class: 'lx-status' }, ''), h('div', { class: 'lx-bar' }, h('i')), steps)),
      h('div', { class: 'lx-brand' }, h('span', { class: 'lx-mark' }), 'NEXUS'));
    const cancel = h('button', { class: 'btn lx-cancel', 'data-nav': true }, 'Cancel');
    cancel.addEventListener('click', () => this.cancel());
    el.append(cancel);
    document.body.append(el); this.el = el;
    // the loading screen takes the controls while it is up (nothing behind it reacts)
    this.layer = { root: el, dialog: true, name: 'launch', onBack: () => this.cancel(), initial: cancel };
    Nav.push(this.layer); renderHints();
    requestAnimationFrame(() => el.classList.add('in'));
    // Stage B → C: after the opening animation the loading screen takes over
    setTimeout(() => el.classList.add('loading'), S.anim === 'off' ? 0 : 950);
    this.update(st);
  },
  update(st) {
    const el = this.el; if (!el || !st) return;
    const [label, idx] = this.STAGES[st.stage] || this.STAGES.preparing;
    $('.lx-status', el).textContent = label + '…';
    $$('.lx-steps span', el).forEach((s, i) => { s.classList.toggle('done', i < idx); s.classList.toggle('on', i === idx); });
    if (st.phase === 'playing') this.enter();
  },
  // Stage D: the engine shows the game — the loading screen fades and NEXUS steps aside
  enter() {
    const el = this.el; if (!el || el.classList.contains('out')) return;
    const wait = Math.max(0, (S.anim === 'off' ? 0 : 1500) - (performance.now() - this.t0)); // let the opening finish
    setTimeout(() => {
      $('.lx-status', el).textContent = 'Have fun!';
      $$('.lx-steps span', el).forEach(s => { s.classList.add('done'); s.classList.remove('on'); });
      el.classList.add('out');
      showPlaying(true);
      setTimeout(() => this.close(), S.anim === 'off' ? 0 : 520);
    }, wait);
  },
  async cancel() {
    if (!this.el || this.el.classList.contains('out')) return;
    if (await confirmDlg('Stop starting the game?', 'NEXUS closes the game before it opens.', 'Stop', true)) api('/api/emulator/stop', { body: {} }).catch(() => { });
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
  $('#play-tip').textContent = 'Press the Home (Guide) button to pause the game and open the Home screen.' + (S.settings.emulator?.quitCombo ? ' Hold View + Menu (Back + Start) to close the game.' : '');
  $('#play-close').textContent = 'Close game';
  const vh = $('#play-volume'); let sl = vh.firstChild;
  if (!sl) { sl = volumeSlider({ value: st.volume ?? S.emulator?.volume ?? 100, onChange: sendVolume, compact: true, label: 'Game volume' }); vh.append(sl); }
  else if (!sl.classList.contains('drag') && Nav.cur !== sl) sl.setValue(st.volume ?? 100);
  show('playing', { initial: $('#play-return'), anim: !justLaunched });
}
Screens.playing = { onBack: () => resumeGame() };
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
const CATS = [
  ['appearance', 'Appearance'], ['startup', 'Startup'], ['library', 'Game library'], ['controllers', 'Controllers'], ['users', 'Users & saves'], ['system', 'System'],
  ['-', 'System & Emulation'], ['engine', 'NEXUS Engine'], ['sysfiles', 'System files'], ['emusettings', 'Emulation settings'], ['emulator', 'Game sessions & saves'],
];
const NAVCATS = () => CATS.filter(c => c[0] !== '-' && c[0] !== 'controllers');
function showSettings(cat = 'appearance') {
  const cats = $('#set-cats'); cats.innerHTML = '';
  for (const [id, label] of CATS) {
    if (id === '-') { cats.append(h('div', { class: 'cat-group' }, label)); continue; }
    const b = h('button', { class: 'cat' + (id === cat ? ' sel' : ''), 'data-nav': true }, label);
    b._cat = id;
    b.addEventListener('click', () => { if (id === 'controllers') { Controllers.show(); return; } selectCat(id, true); });
    b.addEventListener('navfocus', () => { if (S.lastInput !== 'mouse' && id !== 'controllers') selectCat(id, false); });
    cats.append(b);
  }
  selectCat(cat, false);
  show('settings', { initial: $$('.cat', cats).find(b => b._cat === cat) });
}
let curCat = 'appearance';
let catToken = 0;
async function selectCat(id, focusPanel) {
  curCat = id;
  const tok = ++catToken;
  $$('#set-cats .cat').forEach(b => b.classList.toggle('sel', b._cat === id));
  const panel = $('#set-panel'); panel.innerHTML = ''; panel.scrollTop = 0;
  const fresh = h('div');
  const r = (Panels[id] || (() => { }))(fresh);
  panel.replaceChildren(...fresh.childNodes);
  if (r?.then) { const holder = h('div'); await r; if (tok !== catToken) return; panel.append(...fresh.childNodes); void holder; }
  if (focusPanel && tok === catToken) { const f = Nav.items(panel)[0]; if (f) Nav.setFocus(f); }
}
async function rebuildPanel() {
  const panel = $('#set-panel'); const items = Nav.items(panel); const idx = items.indexOf(Nav.cur);
  const scroll = panel.scrollTop;
  const fresh = h('div');
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
    p.append(rowToggle('Play animated wallpaper', wp.motion !== 'off', v => themePatch({ wallpaper: { motion: v ? 'on' : 'off' } }), 'Off shows a still frame for maximum performance. It always pauses while a game runs, NEXUS is in the background, or another screen is open.'));
  }
  if (wp.file || wp.animated) {
    p.append(rowRange('Brightness', { min: 10, max: 100, step: 5, value: Math.round((wp.brightness ?? .55) * 100), fmt: v => v + '%', onChange: (v, commit) => { S.profile.theme.wallpaper.brightness = v / 100; applyTheme(); if (commit) themePatch({ wallpaper: { brightness: v / 100 } }); } }));
    p.append(rowRange('Blur', { min: 0, max: 40, step: 2, value: wp.blur || 0, fmt: v => v ? v + ' px' : 'Off', onChange: (v, commit) => { S.profile.theme.wallpaper.blur = v; applyTheme(); if (commit) themePatch({ wallpaper: { blur: v } }); } }));
    p.append(rowChoice('Scaling', [{ v: 'cover', l: 'Fill' }, { v: 'contain', l: 'Fit' }, { v: 'stretch', l: 'Stretch' }, { v: 'center', l: 'Centre' }, { v: 'tile', l: 'Tile (static only)' }], wp.fit || 'cover', v => themePatch({ wallpaper: { fit: v } })));
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

async function themePatch(patch) { if (!S.profile) return; await patchProfile(S.profile.id, { theme: patch }); }

const ACCENTS = ['#ff2a3a', '#00c8ff', '#ff3b5c', '#ff9f0a', '#ffd60a', '#30d158', '#64d2ff', '#5e5ce6', '#bf5af2', '#ff6ec7', '#ffffff'];
const BOOT_COLORS = ['#e60012', '#ff5a1f', '#f5b300', '#1fae5b', '#0a84ff', '#5e2bff', '#e0218a', '#1d1e21', '#f2f2f2'];
const Panels = {
  appearance(p) {
    const th = S.profile?.theme || {};
    p.append(h('h2', {}, 'Appearance'), h('p', { class: 'note' }, `These settings belong to ${S.profile?.name || 'the current user'}.`));
    p.append(rowChoice('Theme', [{ v: 'dark', l: 'Dark' }, { v: 'light', l: 'Light' }], th.mode, v => themePatch({ mode: v })));
    p.append(rowSwatches('Accent colour', ACCENTS, th.accent, c => themePatch({ accent: c })));
    p.append(h('h3', {}, 'Interface animations'));
    p.append(rowChoice('Interface animations', [{ v: 'full', l: 'Full' }, { v: 'reduced', l: 'Reduced (faster)' }, { v: 'off', l: 'Off' }], th.animations, v => themePatch({ animations: v }).then(rebuildPanel), 'Off also stops animated wallpapers and the Home transition.'));
    p.append(rowChoice('Home transition', [{ v: 'zoom', l: 'Zoom' }, { v: 'fade', l: 'Fade' }, { v: 'slide', l: 'Slide' }, { v: 'off', l: 'Off' }], th.homeTransition || 'zoom', v => { themePatch({ homeTransition: v }).then(() => { if (v !== 'off') { const pv = $('#set-panel .ht-demo'); pv && (pv.classList.remove('ht-' + v), void pv.offsetWidth, pv.classList.add('ht-' + v)); } }); },
      'How the Home screen appears when you press Home, including from a game (the game pauses and keeps its place).'));
    wallpaperSettings(p, th);
    p.append(h('h3', {}, 'Home'));
    p.append(rowChoice('Home screen order', SORTS, S.profile?.sort || 'recent', v => patchProfile(S.profile.id, { sort: v })));
    p.append(rowToggle('Interface sounds', S.settings.sound?.enabled, v => saveSettings({ sound: { enabled: v } })));
    p.append(rowRange('Sound volume', { min: 0, max: 100, step: 10, value: Math.round((S.settings.sound?.volume ?? .7) * 100), fmt: v => v + '%', onChange: (v, commit) => { S.settings.sound.volume = v / 100; if (commit) { saveSettings({ sound: { volume: v / 100 } }); Sound.play('select'); } } }));
  },
  startup(p) {
    const st = S.settings.startup || {};
    p.append(h('h2', {}, 'Startup'), h('p', { class: 'note' }, 'The power-on animation shown when NEXUS starts. Applies to all users.'));
    p.append(rowToggle('Play startup animation', st.enabled, v => saveSettings({ startup: { enabled: v } })));
    p.append(rowSwatches('Background colour', BOOT_COLORS, st.color, c => saveSettings({ startup: { color: c } }).then(rebuildPanel)));
    p.append(rowChoice('Length', [{ v: 'standard', l: 'Standard' }, { v: 'short', l: 'Short' }], st.style, v => saveSettings({ startup: { style: v } })));
    p.append(rowToggle('Startup sound', st.sound, v => saveSettings({ startup: { sound: v } })));
    p.append(rowToggle('Ask who is playing at every start', S.settings.askProfileOnStart, v => saveSettings({ askProfileOnStart: v })));
    p.append(rowButton('Preview startup animation', { onClick: () => setTimeout(() => Boot.preview(), 300) }));
    const prev = h('div', { class: 'boot-preview', style: { background: st.color } });
    prev.append(h('div', { class: 'bp-logo', style: { background: luminance(st.color) > .55 ? '#141414' : '#fff' } })); p.append(prev);
    p.append(h('p', { class: 'note' }, 'Press any key or button to skip the animation.'));
  },
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
    p.append(rowToggle('Include sub-folders', lib.recursive, v => saveSettings({ library: { recursive: v } })));
    p.append(rowToggle('Scan when NEXUS starts', lib.scanOnStart, v => saveSettings({ library: { scanOnStart: v } })));
    p.append(h('h3', {}, 'Formats'));
    p.append(h('p', { class: 'note' }, 'All of these are opened by the NEXUS engine (checked against its file loader). Updates and DLC in these folders are applied automatically when you start the base game.'));
    for (const f of S.formats || []) {
      const on = (lib.formats || []).includes(f.ext);
      p.append(rowToggle(`.${f.ext.toUpperCase()}`, on, v => { const set = new Set(S.settings.library.formats); v ? set.add(f.ext) : set.delete(f.ext); saveSettings({ library: { formats: [...set] } }); }, f.label));
    }
    p.append(h('h3', {}, 'Artwork & names'));
    p.append(rowToggle('Read game info with prod.keys', lib.readKeysForIds, v => saveSettings({ library: { readKeysForIds: v } }), 'Decrypts game headers locally with your own keys to read the real title, version and icon. Nothing leaves your PC.'));
    p.append(rowToggle('Download missing artwork', lib.onlineArtwork, v => saveSettings({ library: { onlineArtwork: v } }), 'Looks up the Title ID on the Nlib database (api.nlib.cc) when a game has no icon of its own. Only the Title ID is sent. Images are checked and saved locally.'));
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
  },
  engine(p) { return renderEngine(p); },
  emusettings(p) { renderEmuSettings(p); },
  sysfiles(p) { renderSystemFiles(p); },
  emulator(p) {
    const es = S.settings.emulator || {};
    p.append(h('h2', {}, 'Game sessions & saves'), h('p', { class: 'note' }, 'How NEXUS runs games and keeps each user’s saves. Graphics, audio and system options are in Emulation settings; keys and firmware in System files.'));
    p.append(rowVolume('Game volume', +(S.settings.engine?.['audio.volume'] ?? 100), sendVolume, '0% is silent, 100% is the game’s own level, up to 200% boosts quiet games (a soft limiter prevents harsh clipping). Also adjustable while playing.'));
    p.append(rowChoice('Ask who\u2019s playing', [{ v: 'several', l: 'When there is more than one user' }, { v: 'always', l: 'Always' }, { v: 'never', l: 'Never (the current user plays)' }], es.whoPlays || 'several', v => saveSettings({ emulator: { whoPlays: v } }), 'Before a game starts, NEXUS asks which user is playing. Play time, saves and controller layout belong to that user.'));
    p.append(rowToggle('Start games in full screen', es.fullscreen, v => saveSettings({ emulator: { fullscreen: v } }), 'The game window belongs to NEXUS; when the game ends you are back on the NEXUS home screen.'));
    p.append(rowToggle('Quit shortcut: hold View + Menu', es.quitCombo, v => saveSettings({ emulator: { quitCombo: v } }), 'Hold Back + Start for 2 seconds on an XInput controller to close the game.'));
    p.append(rowToggle('Separate saves for each user', es.manageSaves, async v => {
      if (!v && !(await confirmDlg('Share saves between users?', 'All users will share the engine’s save folder. Per-user saves stay in NEXUS data\\Saves and come back if you turn this on again.', 'Turn off'))) { rebuildPanel(); return; }
      saveSettings({ emulator: { manageSaves: v } });
    }, 'Points the engine’s save folder at NEXUS data\\Saves\\<user> for every game the user starts.'));
    p.append(rowButton('Keys and firmware…', { sub: 'Install or replace prod.keys and firmware.', onClick: () => selectCat('sysfiles', true) }));
  },
  users(p) {
    p.append(h('h2', {}, 'Users & saves'));
    for (const pr of S.profiles) {
      const r = rowButton(pr.name, { sub: pr.id === S.profile?.id ? 'Current user' : '', onClick: () => editProfile(pr.id) });
      r.querySelector('.rl').prepend(avatarEl(pr)); r.querySelector('.rl').style.cssText = 'flex-direction:row;align-items:center;gap:.8em';
      r.querySelector('.avatar').style.setProperty('--sz', '2.2em');
      p.append(r);
    }
    p.append(rowButton('Add user', { icon: I.plus, onClick: () => createProfileFlow() }));
    p.append(h('h3', {}, 'How saves work'));
    p.append(h('p', { class: 'note' }, S.settings.emulator?.manageSaves ?
      'Each user has a private save folder (NEXUS data\\Saves\\<user id> - <name>), with a readable folder for every game. When a game starts, the NEXUS engine uses the current user’s folder, so users never share or overwrite each other’s progress. Saves are never deleted: removing a user moves their data to NEXUS data\\Archived.' :
      'Per-user saves are turned off, so all users share the engine’s save folder. Turn it on in Settings › Game sessions & saves.'));
    p.append(rowButton('Open saves folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'saves' } }) }));
    p.append(rowButton('Open archived data', { onClick: () => api('/api/app/open-folder', { body: { which: 'archived' } }) }));
  },
  system(p) {
    p.append(h('h2', {}, 'System'));
    p.append(h('dl', { class: 'kv' },
      h('dt', {}, 'Console'), h('dd', {}, S.app.model || 'NEXUS'),
      h('dt', {}, 'NEXUS version'), h('dd', {}, S.app.version),
      h('dt', {}, 'NEXUS engine'), h('dd', {}, S.engine?.installed ? (S.engine.info?.version || S.engine.active) : 'Not installed'),
      h('dt', {}, 'Program folder'), h('dd', {}, S.app.root || ''),
      h('dt', {}, 'Your data'), h('dd', {}, (S.app.data || '') + (S.app.portable ? '  (portable mode)' : ''))));
    p.append(h('p', { class: 'note' }, S.app.portable ? 'Portable mode (portable.txt next to NEXUS.exe): your data is stored inside the NEXUS folder. Back it up before deleting the folder.' :
      'Your users, saves, settings and artwork are stored in your Windows user folder, not in the NEXUS program folder. You can delete or replace the NEXUS folder to update it without losing anything. The engine’s keys, firmware, shader caches and console data are kept in NEXUS data\\Engine.'));
    p.append(rowButton('Open my NEXUS data folder', { onClick: () => api('/api/app/open-folder', { body: { which: 'data' } }) }));
    p.append(rowButton('Open engine data folder', { sub: S.emulator?.dataDir || '', onClick: () => api('/api/app/open-folder', { body: { which: 'enginedata' } }) }));
    p.append(rowToggle('Full screen', S.system.fullscreen ?? S.settings.display?.fullscreen, async v => { const r = await api('/api/app/fullscreen', { body: { on: v } }); S.system.fullscreen = r.fullscreen; }, 'F11 also toggles full screen.'));
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
    p.append(h('h3', {}, 'Storage & data'));
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
    p.append(rowButton('Reset NEXUS settings', { sub: 'Restores startup, sound, library and game session settings. Users, saves and game folders are kept.', onClick: async () => {
      if (await confirmDlg('Reset settings?', 'Users, saves and game folders are kept.', 'Reset', true)) { S.settings = await api('/api/settings/reset', { body: {} }); rebuildPanel(); toast('Settings reset', I.check); }
    } }));
    p.append(h('h3', {}, 'Updates'));
    p.append(h('p', { class: 'note' }, 'NEXUS does not update itself. New NEXUS engine builds are installed and rolled back in Settings › NEXUS Engine; your data is never touched.'));
    p.append(h('h3', {}, 'Legal'));
    p.append(h('p', { class: 'note' }, 'NEXUS is independent software, not affiliated with or endorsed by Nintendo. NEXUS and its engine are free software under the GPL-3.0-or-later; the engine is built on the yuzu/Eden emulator core and other open-source projects — see Settings › NEXUS Engine and the Licences folder for notices and source code. NEXUS does not include games, keys or firmware: use only content you legally own.'));
  },
};
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
    ...(hw.displays.length ? hw.displays.map(d => [d.name + (d.primary ? ' (main)' : ''), `${d.width} × ${d.height} at ${d.refreshHz} Hz`]) : [['Display', 'Not detected']]),
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
async function enhancementsPanel(box) {
  let en;
  try { en = await api('/api/enhancements'); } catch (e) { box.append(h('p', { class: 'note warn' }, e.message)); return; }
  const pill = (cls, label) => h('span', { class: 'pill ' + cls }, label);
  const draw = () => {
    box.replaceChildren();
    const st = f => f.ready ? pill('ok', 'Ready') : f.supported ? pill('warn', 'Needs files') : pill('bad', 'Not supported');
    box.append(h('dl', { class: 'kv enh-kv' },
      h('dt', {}, 'Graphics card'), h('dd', {}, en.gpu || 'Not detected'),
      h('dt', {}, 'FSR 3 frame generation'), h('dd', {}, st(en.frameGen)),
      h('dt', {}, 'Lossless Scaling frame generation'), h('dd', {}, st(en.lossless)),
      h('dt', {}, 'NVIDIA DLSS'), h('dd', {}, st(en.dlss)),
      h('dt', {}, 'AMD FSR 4'), h('dd', {}, st(en.fsr4))));
    if (en.frameGen.supported && !en.lossless.ready) {
      box.append(rowButton('Find Lossless Scaling…', { sub: (en.lossless.detail ? en.lossless.detail + '. ' : '') + 'Copies Lossless.dll from your own Lossless Scaling (Steam) into NEXUS. NEXUS never includes it.', onClick: async () => {
        try { en = await api('/api/enhancements/lossless', { body: {} }); toast('Lossless Scaling ready', I.check); draw(); }
        catch (e) {
          if (await confirmDlg('Lossless Scaling not found', e.message + '\n\nChoose Lossless.dll yourself?', 'Choose file')) {
            try { const r = await api('/api/enhancements/lossless', { body: { pick: true } }); if (!r.cancelled) { en = r; toast('Lossless Scaling ready', I.check); draw(); } } catch (e2) { alertDlg('Could not use that file', e2.message); }
          }
        }
      } }));
    }
    if (en.dlss.supported && !en.dlss.ready) {
      box.append(h('p', { class: 'note' }, en.dlss.why));
      box.append(rowButton('Open the DLSS folder', { sub: en.dlssDir, onClick: () => api('/api/app/open-folder', { body: { which: 'streamline' } }).catch(e => toast(e.message, I.warn)) }));
      box.append(rowButton('Check again', { onClick: async () => { en = await api('/api/enhancements'); draw(); } }));
    }
  };
  draw();
}

// ---------- global emulation settings
let emuSection = 'graphics';
async function renderEmuSettings(p) {
  p.append(h('h2', {}, 'Emulation settings'), h('p', { class: 'note' }, 'Global settings for every game. A game can override any of them in its Configure Game Settings. Settings the engine cannot apply are shown as not available, never as a control that does nothing.'));
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
  engineDataSection(p);
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
    info.upstream ? [h('dt', {}, 'Built from'), h('dd', {}, `${info.upstream.project || ''} ${info.upstream.describe || ''} ${(info.upstream.commit || '').slice(0, 10)}`.trim())] : '',
    h('dt', {}, 'Interface'), h('dd', {}, `version ${e.apiVersion}`),
    h('dt', {}, 'Keys'), h('dd', {}, e.keys ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'Firmware'), h('dd', {}, e.firmware ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'VC++ runtime'), h('dd', {}, e.vcRuntime ? pill('ok', 'Installed') : pill('bad', 'Missing')),
    h('dt', {}, 'Data folder'), h('dd', {}, e.dataDir || '')));
  if (!e.installed && e.problem) p.append(h('p', { class: 'note warn' }, e.problem));
  if (!e.vcRuntime) p.append(h('p', { class: 'note warn' }, 'The Microsoft Visual C++ runtime the engine needs was not found (it normally ships in NEXUS\\Engine\\Libraries). Install it from Microsoft: aka.ms/vs/17/release/vc_redist.x64.exe'));
  if (e.vulkan === false) p.append(h('p', { class: 'note warn' }, 'No Vulkan graphics driver was found. Install the latest driver for your graphics card, or choose OpenGL in Emulation settings › Graphics.'));
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
  p.append(h('p', { class: 'note' }, 'Graphics, audio and system options are in Emulation settings. Keys and firmware are in System files.'));
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
    left.append(h('h3', { class: 'set-h' }, 'Connected controllers'));
    if (!S.pads.length) left.append(h('div', { class: 'pad-card' }, h('h4', {}, 'No controller detected'), h('small', {}, 'Connect a controller and press any button. Xbox, PlayStation and most USB/Bluetooth pads work. Keyboard: arrows to move, Enter = OK, Esc = Back, X = Options.')));
    for (const p of S.pads) {
      const card = h('div', { class: 'pad-card', dataset: { pad: p.index } }, h('h4', {}, `Player ${p.index + 1}`), h('small', {}, p.id),
        h('div', { class: 'pad-btns' }, Array.from({ length: 17 }, (_, i) => h('span', { dataset: { b: i } }, i))),
        h('div', { class: 'axes' }, h('div', { class: 'stick', dataset: { s: 0 } }, h('i')), h('div', { class: 'stick', dataset: { s: 1 } }, h('i'))));
      left.append(card);
    }
    const padChoices = [{ v: '', l: 'Any controller' }, ...S.pads.map(p => ({ v: p.id, l: `Player ${p.index + 1}: ${p.id.slice(0, 40)}` }))];
    if (c.preferredPad && !S.pads.some(p => p.id === c.preferredPad)) padChoices.push({ v: c.preferredPad, l: c.preferredPad.slice(0, 40) + ' (disconnected)' });
    left.append(rowChoice('Controller for NEXUS menus', padChoices, c.preferredPad || '', v => ctlPatch({ preferredPad: v })));
    left.append(rowToggle('Vibration', c.vibration, v => { ctlPatch({ vibration: v }); if (v) setTimeout(() => Pad.rumble(250, .8), 100); }));
    left.append(rowRange('Repeat speed', { min: 60, max: 300, step: 20, value: c.keyRepeatMs || 140, fmt: v => v + ' ms', onChange: (v, commit) => { if (commit) ctlPatch({ keyRepeatMs: v }); } }));

    right.append(h('h3', { class: 'set-h' }, 'NEXUS button mapping'));
    right.append(rowChoice('Confirm button', [{ v: 'south', l: 'Bottom (Xbox style)' }, { v: 'east', l: 'Right (Nintendo style)' }], c.confirmButton, v => {
      const m = { ...c.mapping }; if (v === 'east') { m.confirm = 1; m.back = 0; } else { m.confirm = 0; m.back = 1; }
      ctlPatch({ confirmButton: v, mapping: m });
    }));
    const ACT = [['confirm', 'OK / Select'], ['back', 'Back'], ['options', 'Options'], ['search', 'Search'], ['prevTab', 'Previous tab'], ['nextTab', 'Next tab'], ['menu', 'Power menu'], ['home', 'Home']];
    for (const [a, l] of ACT) right.append(rowButton(l, { value: `Button ${c.mapping?.[a] ?? '—'}`, sub: '', onClick: () => Mapper.start(a, l) }));
    right.append(rowButton('Reset mapping', { onClick: () => ctlPatch({ confirmButton: 'south', mapping: { confirm: 0, back: 1, options: 3, search: 2, prevTab: 4, nextTab: 5, menu: 9, home: 16, view: 8 } }) }));
    right.append(h('h3', { class: 'set-h' }, 'In-game controls'));
    const ips = S.emulator?.inputProfiles || [];
    right.append(rowChoice('Controller profile for games', [{ v: '', l: 'Automatic: first connected controller' }, ...ips.map(x => ({ v: x, l: x }))], c.emuInputProfile || '', v => ctlPatch({ emuInputProfile: v }),
      ips.length ? 'Player 1 uses this saved profile whenever this user starts a game. A game can choose its own in Configure Game Settings.' : 'Player 1 is mapped automatically to the first connected controller with its standard layout (keyboard if none is connected).'));
    if (!c.emuInputProfile) layoutRows(right, c.gameLayout || {}, async l => { await api(`/api/profiles/${S.profile.id}/game-layout`, { body: l }); await loadState(); S.profile = profileById(S.profile.id); Controllers.refreshList(); }, 'user');
    right.append(h('p', { class: 'note' }, 'In games the NEXUS engine detects controllers automatically (Xbox/XInput, PlayStation, Switch Pro and most USB or Bluetooth pads) and uses this layout for player 1. A game can have its own layout in Configure Game Settings › Controls. Keyboard: the engine’s standard keys work when no controller is connected.'));
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
  container.append(rowChoice('Button layout', [{ v: 'nintendo', l: 'Nintendo positions (A on the right)' }, { v: 'xbox', l: 'Xbox labels (A at the bottom)' }], l.base,
    v => save({ ...l, base: v, buttons: {} }), 'Nintendo positions keep the console’s layout; Xbox labels match the letters printed on Xbox-style pads.'));
  container.append(rowChoice('Controller type', [{ v: '', l: 'Automatic' }, { v: 'pro', l: 'Pro Controller' }, { v: 'dual_joycon', l: 'Joy-Con pair' }, { v: 'handheld', l: 'Handheld' }], l.controllerType,
    v => save({ ...l, controllerType: v }), 'What the game sees. Automatic uses the game’s own preset if it has one, otherwise a Pro Controller.'));
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
    setTimeout(() => ctlPatch({ mapping: m }).then(() => toast(`Mapped to button ${i}`, I.check)), 150);
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
        const was = S.playing?.suspended;
        if (st.running) {
          S.playing = st;
          if (Launch.el) { Launch.update(st); if (st.phase !== 'playing') break; }
          if (was && !st.suspended && S.screen === 'home') showPlaying(true); // resumed (e.g. with the Home button)
          else if (S.screen === 'playing') showPlaying(true);
          if (S.screen === 'home') renderBanner();
        }
        Wall.update();
        break;
      }
      case 'emulator.home': { // the game was paused with Home: show the Home screen
        S.playing = m.data;
        Sound.play('home_pop');
        goHome({ fromGame: true });
        Wall.update();
        break;
      }
      case 'emulator.exit': {
        S.playing = null;
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
      case 'app.focus': if (m.data && S.playing && S.screen !== 'playing') showPlaying(); break;
      case 'profiles.changed': break;
      case 'avatars.state': if (m.data.state !== 'working' && Picker.reopen) Picker.reopen(); if (m.data.state === 'ready' && m.data.count) toast(`Profile icons ready (${m.data.count})`, I.person); break;
    }
  };
  es.onerror = () => { /* EventSource reconnects automatically */ };
}

window.addEventListener('error', e => { api('/api/app/log', { body: { level: 'error', message: `${e.message} @ ${e.filename}:${e.lineno}` } }).catch(() => { }); });
window.addEventListener('unhandledrejection', e => { api('/api/app/log', { body: { level: 'error', message: 'promise: ' + (e.reason?.stack || e.reason) } }).catch(() => { }); });

// ------------------------------------------------------------------ start
(async function main() {
  Sound.init();
  try { await loadState(); }
  catch (e) { document.body.classList.remove('booting'); document.body.innerHTML = `<div style="padding:3em;font-family:sans-serif;color:#fff">NEXUS could not reach its service: ${e.message}</div>`; return; }
  const last = profileById(S.settings.lastProfileId);
  if (last && !S.settings.askProfileOnStart) S.profile = last;
  applyTheme();
  // Optional user stylesheets: Resources/Themes/*.css are loaded after the built-in theme.
  api('/res/themes').then(list => (list || []).forEach(n => document.head.append(h('link', { rel: 'stylesheet', href: '/res/themes/' + encodeURIComponent(n) })))).catch(() => { });
  const soundLoad = Sound.load(['boot_power', 'boot_whoosh', 'boot_snap', 'boot_draw', 'boot_sparkle', 'boot_chime', 'boot_outro', 'home_pop', 'move', 'select', 'back', 'launch', 'error', 'connect']);
  await Promise.race([soundLoad, sleep(600)]);
  const gamesLoad = S.profile ? loadGames().catch(() => { }) : Promise.resolve();
  connectEvents();
  pollSystem(); setInterval(pollSystem, 15000);
  tickClock(); setInterval(tickClock, 1000);
  requestAnimationFrame(() => Pad.poll());
  // Prepare the home screen underneath the boot animation
  await Boot.run();
  await gamesLoad;
  const st = await api('/api/system').catch(() => ({}));
  if (S.app.migrated?.length) toast(S.app.migrated[0], I.check, 6000);
  if (st.emulator?.running) { S.playing = st.emulator; showPlaying(); return; }
  if (!S.profile) { showWho(); return; }
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
})();
