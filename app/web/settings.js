// NEXUS Settings: search, "changed from the default" markers, advanced sections and the panels
// added when the settings were regrouped (refinement brief, 17 Nov 2026). Loaded after app.js.

// ---------------------------------------------------------------- changed from the default
// The defaults come from NEXUS itself (GET /api/settings/defaults), so the marker can never
// disagree with what "Reset NEXUS settings" would do.
let SETTING_DEFAULTS = null;
api('/api/settings/defaults').then(d => { SETTING_DEFAULTS = d; if (S.screen === 'settings') markAllChanged(); }).catch(() => { });

const SETTING_UNSET = { 'emulator.gameWindow': 'inside', 'emulator.backgroundMinutes': 15, 'emulator.homeAction': 'background', 'emulator.whoPlays': 'several', 'startup.style': 'standard' };
function settingAt(obj, path) { return path.split('.').reduce((o, k) => (o == null ? undefined : o[k]), obj); }
function sameSetting(a, b) {
  if (a === undefined || a === null) a = defaultBlank(b);
  if (b === undefined || b === null) b = defaultBlank(a);
  return JSON.stringify(a) === JSON.stringify(b);
}
function defaultBlank(v) { return typeof v === 'boolean' ? false : typeof v === 'number' ? 0 : typeof v === 'string' ? '' : v; }
function fmtDefault(v) { return v === true ? 'On' : v === false ? 'Off' : v === '' || v == null ? 'none' : typeof v === 'number' && v > 0 && v <= 1 ? Math.round(v * 100) + '%' : String(v); }

// changedMark(row, 'sound.enabled'): marks the row while the setting differs from its default.
// Returns the row, so it wraps a rowToggle / rowChoice / rowRange call.
function changedMark(row, path) {
  row.dataset.setting = path;
  markChanged(row);
  return row;
}
function markChanged(row) {
  const path = row.dataset.setting;
  if (!path || !SETTING_DEFAULTS) return;
  let cur = path === 'display.fullscreen' ? (S.system?.fullscreen ?? settingAt(S.settings, path)) : settingAt(S.settings, path);
  let def = settingAt(SETTING_DEFAULTS, path);
  // settings stored as "not set" mean the value the interface shows for them
  if (path in SETTING_UNSET) { if (cur == null || cur === '') cur = SETTING_UNSET[path]; if (def == null || def === '') def = SETTING_UNSET[path]; }
  const changed = !sameSetting(cur, def);
  row.classList.toggle('changed', changed);
  let tag = row.querySelector('.chg');
  if (changed && !tag) {
    tag = h('span', { class: 'chg', title: '' }, 'Changed');
    row.querySelector('.rl > span')?.append(tag);
  }
  if (tag) { tag.hidden = !changed; tag.title = `Changed from the default (${fmtDefault(def)})`; }
}
function markAllChanged() { $$('#set-panel [data-setting]').forEach(markChanged); }
// saving a setting updates the markers of the rows on screen
{
  const save = saveSettings;
  // eslint-disable-next-line no-global-assign
  saveSettings = async patch => { const r = await save(patch); markAllChanged(); return r; };
}

// ---------------------------------------------------------------- advanced sections
// advancedSection(p, 'Advanced', box => { ...rows }): less-used options behind one row; open
// it and its rows appear below. Nothing is removed: search finds them and opens the section.
function advancedSection(p, title, build, { sub, open = false } = {}) {
  const key = curCat + ':' + title;
  const isOpen = open || advancedSection.opened.has(key);
  const box = h('div', { class: 'adv-body' });
  const head = rowButton(title, { sub: sub || 'Options most people never need to change.', value: isOpen ? 'Hide' : 'Show' });
  head.classList.add('adv-head');
  head.setAttribute('aria-expanded', String(isOpen));
  build(box);
  box.hidden = !isOpen;
  head.addEventListener('click', () => {
    const now = box.hidden;
    box.hidden = !now;
    head.setAttribute('aria-expanded', String(now));
    head.querySelector('.rv').firstChild.textContent = now ? 'Hide' : 'Show';
    now ? advancedSection.opened.add(key) : advancedSection.opened.delete(key);
    if (now) { const f = Nav.items(box)[0]; f && f.scrollIntoView({ block: 'nearest' }); }
  });
  p.append(head, box);
  return box;
}
advancedSection.opened = new Set();

// moveToAdvanced(p, ['Row name', ...]): moves rows (and headings) already in the panel into
// one advanced section at the end, in their order.
function moveToAdvanced(p, names, opts) {
  const rows = [...p.children].filter(el => (el.classList?.contains('row') && names.includes(el.querySelector('.rl > span')?.firstChild?.textContent?.trim()))
    || (el.tagName === 'H3' && names.includes(el.textContent.trim())));
  if (!rows.length) return;
  advancedSection(p, 'Advanced', box => box.append(...rows), opts);
}

// ---------------------------------------------------------------- new panels
Object.assign(Panels, {
  async shaders(p) {
    const es = S.settings.emulator || {};
    p.append(h('h2', {}, 'Shader management'), h('p', { class: 'note' }, 'Shaders are the small programs a game’s graphics are drawn with. NEXUS prepares them so games do not stutter the first time an effect appears.'));
    p.append(h('h3', {}, 'Before playing'));
    p.append(changedMark(rowToggle('Prepare shaders before playing', es.prepareShaders !== false, v => saveSettings({ emulator: { prepareShaders: v } }),
      'Before a game starts, NEXUS finds the shader programs in the game’s own files and compiles them, so they do not stutter the first time they appear. The first start of a game takes longer (once per game, and again after a graphics driver update); later starts are quick. Graphics effects the game has never shown on any of your devices are still compiled the first time they appear, and then kept.'), 'emulator.prepareShaders'));
    const sc = h('div'); p.append(sc); await sharedCachePanel(sc);
    p.append(h('h3', {}, 'For one game'));
    p.append(h('p', { class: 'note' }, 'A game’s own shader cache and its preparation progress are in the game’s Options › Shader cache, on Home or in the Library.'));
  },
});

// ---------------------------------------------------------------- search
// Finds settings by name, description and common words ("FPS", "GitHub", "background"). The
// index is built from the panels themselves (rendered off-screen, without saving anything), so
// every setting is found, including the ones in advanced sections; a few extra words point to
// the right place for things people call by another name.
const SEARCH_EXTRA = [
  ['fps frame rate framerate frames per second overlay', 'perf', 'Performance monitor'],
  ['github gist sign in sign out connect reconnect disconnect cloud sync synchronise token', 'account', 'NEXUS Account & GitHub'],
  ['shader cache shaders stutter compile precompile pipeline', 'shaders', 'Shader management'],
  ['background wallpaper picture image blur dim', 'appearance', 'Wallpaper'],
  ['background colour home background', 'home', 'Plain background colour'],
  ['graphics resolution gpu vulkan opengl anti-aliasing filtering scaling fsr dlss upscaling', 'emusettings', 'Graphics & emulation'],
  ['lossless scaling lsfg frame generation dlss', 'emusettings', 'Frame generation & DLSS (Graphics & emulation)'],
  ['controller gamepad pad joystick buttons mapping xbox playstation dualsense', 'controllers', 'Controllers'],
  ['theme dark light accent colour color', 'appearance', 'Theme and accent colour'],
  ['boot intro splash startup animation logo', 'general', 'Startup animation'],
  ['keys prod.keys firmware', 'sysfiles', 'System files'],
  ['saves save data backup', 'savedata', 'Save data'],
  ['volume sound audio', 'general', 'Sounds'],
  ['update version', 'updates', 'Updates'],
  ['crash crashes log logs report', 'system', 'Logs (Storage & data › Maintenance)'],
  ['mods mod cheats', 'library', 'Mods: in a game’s Options › Mods (Home or Library)'],
  ['record video capture screenshot', 'recording', 'Screen recording'],
];
const SEARCH_SKIP = new Set(['controllers', 'sync']); // a screen of its own / experimental
const SEARCH_SKIP_HEADINGS = new Set(['Shortcuts', 'Folders and pages', 'Linked devices', 'Recent activity', 'Storage']);
let searchIndex = null, searchBuilding = null, searchQuery = '';

async function buildSearchIndex() {
  if (searchIndex) return searchIndex;
  if (searchBuilding) return searchBuilding;
  searchBuilding = (async () => {
    const out = [];
    const keepCat = curCat;
    for (const [id, label] of catList()) {
      if (id === '-' || SEARCH_SKIP.has(id) || !Panels[id]) continue;
      const box = h('div');
      try {
        curCat = id; // panels read it while building (advanced sections); put back at once
        let r;
        try { r = Panels[id](box); } finally { curCat = keepCat; }
        if (r?.then) await Promise.race([r, sleep(2500)]);
      } catch { }
      let heading = '';
      for (const el of box.querySelectorAll('h3, .row:not(.adv-head)')) {
        if (el.tagName === 'H3') { heading = el.textContent.trim(); continue; }
        if (SEARCH_SKIP_HEADINGS.has(heading)) continue; // lists of things, not settings
        if (id === 'emusettings') continue; // indexed from the engine's list below
        const name = el.querySelector('.rl > span')?.firstChild?.textContent?.trim();
        if (!name) continue;
        const sub = el.querySelector('.rl small')?.textContent?.trim() || '';
        const adv = !!el.closest('.adv-body');
        out.push({ cat: id, catLabel: label, group: catGroup(id), heading, name, sub, adv, text: (name + ' ' + sub + ' ' + heading + ' ' + label).toLowerCase() });
      }
    }
    // the engine's settings: every section of Graphics & emulation, not only the one open
    try {
      const view = await api('/api/engine-settings');
      const secs = Object.fromEntries((view.sections || []).map(x => [x.v, x.l]));
      for (const st of view.settings || []) {
        if (!st.label) continue;
        const sec = secs[st.section] || st.section;
        out.push({ cat: 'emusettings', catLabel: catLabel('emusettings'), group: catGroup('emusettings'), heading: sec, name: st.label, sub: st.help || '', emuSection: st.section,
          text: (st.label + ' ' + (st.help || '') + ' ' + sec + ' ' + (st.choices || []).map(c => c.l).join(' ') + ' graphics emulation').toLowerCase() });
      }
    } catch { }
    for (const [words, cat, name] of SEARCH_EXTRA) out.push({ cat, catLabel: catLabel(cat), group: catGroup(cat), heading: '', name, sub: '', extra: true, text: words + ' ' + name.toLowerCase() });
    searchIndex = out; searchBuilding = null;
    return out;
  })();
  return searchBuilding;
}

function searchSettings(q) {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length || !searchIndex) return [];
  const scored = [];
  for (const e of searchIndex) {
    if (!words.every(w => e.text.includes(w))) continue;
    const n = e.name.toLowerCase();
    let score = words.reduce((s, w) => s + (n.startsWith(w) ? 6 : n.includes(w) ? 4 : e.heading.toLowerCase().includes(w) ? 2 : e.extra ? 3 : 1), 0);
    if (e.adv) score -= .5;
    scored.push([score, e]);
  }
  scored.sort((a, b) => b[0] - a[0]);
  const seen = new Set(), out = [];
  for (const [, e] of scored) { const k = e.cat + '|' + e.name; if (seen.has(k)) continue; seen.add(k); out.push(e); if (out.length >= 40) break; }
  return out;
}

function settingsSearchBox() {
  const inp = h('input', { id: 'set-search', type: 'search', placeholder: 'Search settings', 'data-nav': true, autocomplete: 'off', spellcheck: 'false', 'aria-label': 'Search settings' });
  inp.value = searchQuery;
  let t = null;
  inp.addEventListener('input', () => { clearTimeout(t); t = setTimeout(() => runSettingsSearch(inp.value), 120); });
  inp.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); const f = Nav.items($('#set-panel'))[0]; f && Nav.setFocus(f); } });
  inp.addEventListener('focus', () => buildSearchIndex());
  return h('label', { class: 'search set-search' }, svg('M10 3a7 7 0 1 1 0 14 7 7 0 0 1 0-14Zm0 2a5 5 0 1 0 0 10 5 5 0 0 0 0-10Zm6.3 10.9 4.4 4.4-1.4 1.4-4.4-4.4z'), inp);
}

async function runSettingsSearch(q) {
  searchQuery = q.trim();
  if (!searchQuery) { if (curCat === 'search') selectCat(searchReturn || 'general', false); return; }
  if (curCat !== 'search') searchReturn = curCat;
  await buildSearchIndex();
  curCat = 'search';
  $$('#set-cats .cat').forEach(b => b.classList.remove('sel'));
  const panel = $('#set-panel'); const fresh = h('div');
  Panels.search(fresh);
  panel.replaceChildren(...fresh.childNodes); panel.scrollTop = 0;
}
let searchReturn = null;

Panels.search = p => {
  const res = searchSettings(searchQuery);
  p.append(h('h2', {}, `Settings matching “${searchQuery}”`));
  if (!res.length) { p.append(h('p', { class: 'note' }, 'Nothing found. Try another word, such as “controller”, “GitHub”, “shader cache”, “FPS”, “background” or “graphics”.')); return; }
  for (const e of res) {
    const where = [e.group !== e.catLabel ? e.group : '', e.catLabel, e.heading].filter(Boolean).join(' › ') + (e.adv ? ' › Advanced' : '');
    p.append(rowButton(e.name, { sub: where, onClick: () => {
      if (e.cat === 'controllers') { Controllers.show(); return; }
      searchQuery = ''; const box = $('#set-search'); if (box) box.value = '';
      if (e.emuSection) emuSection = e.emuSection;
      selectCat(e.cat, false).then(() => settingsReveal(e.extra ? null : e.name));
      $$('#set-cats .cat').forEach(b => b.classList.toggle('sel', b._cat === e.cat));
    } }));
  }
};

// settingsReveal(name): scrolls to the setting called name in the open panel, opening its
// advanced section if needed, focuses it and highlights it for a moment.
function settingsReveal(name) {
  const panel = $('#set-panel');
  const rows = $$('.row', panel);
  const row = name && rows.find(r => r.querySelector('.rl > span')?.firstChild?.textContent?.trim() === name);
  if (!row) { const f = Nav.items(panel)[0]; f && Nav.setFocus(f); return; }
  const adv = row.closest('.adv-body');
  if (adv && adv.hidden) adv.previousElementSibling?.click();
  Nav.setFocus(row);
  row.scrollIntoView({ block: 'center' });
  row.classList.remove('found'); void row.offsetWidth; row.classList.add('found');
}

// the panel on screen gets its markers once the defaults have arrived
const _selectCat = selectCat;
// eslint-disable-next-line no-global-assign
selectCat = async (id, focus) => { const r = await _selectCat(id, focus); markAllChanged(); return r; };
