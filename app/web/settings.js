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

// ---------------------------------------------------------------- restore one setting
// Every setting row can carry a small restore button (addReset). It shows while the setting
// differs from its default, restores and saves the default when pressed (mouse), and the
// controller's Options button / the X key does the same on the focused setting (Nav.options).
let PROFILE_DEFAULTS = null;
api('/api/profiles/defaults').then(d => { PROFILE_DEFAULTS = d; if (S.screen === 'settings' || S.screen === 'controllers') markAllChanged(); }).catch(() => { });
const RESET_ICON = 'M12 5V1L7 6l5 5V7a5 5 0 1 1-5 5H5a7 7 0 1 0 7-7z';

// addReset(row, { changed: () => bool, run: async () => {}, def: () => 'label of the default' })
function addReset(row, spec) {
  row._reset = spec;
  let b = row.querySelector(':scope > .rst');
  if (!b) {
    b = h('span', { class: 'rst', role: 'button', 'aria-label': 'Restore the default' }, svg(RESET_ICON));
    b.addEventListener('click', e => { e.stopPropagation(); e.preventDefault(); if (row._reset.changed()) runReset(row); });
    b.addEventListener('mousedown', e => e.stopPropagation());
    const rv = row.querySelector(':scope > .rv, :scope > .swatches, :scope > .toggle, :scope > .vol');
    rv ? row.insertBefore(b, rv) : row.append(b);
  }
  refreshReset(row);
  return row;
}
function refreshReset(row) {
  const sp = row._reset, b = row.querySelector(':scope > .rst'); if (!sp || !b) return;
  let changed = false; try { changed = !!sp.changed(); } catch { }
  const def = (() => { try { return sp.def ? sp.def() : ''; } catch { return ''; } })();
  b.classList.toggle('off', !changed);
  b.setAttribute('aria-disabled', String(!changed));
  b.title = changed ? `Restore the default${def ? ` (${def})` : ''}. Controller: press Options on this setting.` : 'This is the default';
  row.classList.toggle('changed', changed);
  let tag = row.querySelector('.chg');
  if (changed && !tag) { tag = h('span', { class: 'chg' }, 'Changed'); row.querySelector('.rl > span')?.append(tag); }
  if (tag) { tag.hidden = !changed; tag.title = def ? `Changed from the default (${def})` : 'Changed from the default'; }
}
async function runReset(row) {
  const sp = row._reset; if (!sp) return;
  const label = row.querySelector('.rl > span')?.firstChild?.textContent?.trim() || 'Setting';
  try { await sp.run(); toast(`${label}: default restored`, I.check, 1600); }
  catch (e) { alertDlg('The default was not restored', e.message); }
  if (S.screen === 'settings' && !Nav.top()?.dialog) await rebuildPanel();
  else markAllChanged();
  renderHints();
}
// what the Options button restores on the focused element (Nav.options, renderHints)
function resetTarget(el) {
  const row = el?.closest?.('.row');
  if (!row?._reset) return null;
  let changed = false; try { changed = !!row._reset.changed(); } catch { }
  return changed ? { run: () => runReset(row) } : null;
}

// changedMark(row, 'sound.enabled' | 'profile:theme.mode'): the setting's restore button and
// "Changed" mark, from NEXUS's own defaults. Returns the row.
function changedMark(row, path) {
  row.dataset.setting = path;
  addReset(row, { changed: () => pathChanged(path), def: () => fmtDefault(pathDefault(path)), run: () => resetPath(path) });
  return row;
}
const SETTING_UNSET_P = { 'perfMonitor.layout': 'bar', 'perfMonitor.mode': 'off', 'profile:theme.homeTransition': 'zoom', 'profile:theme.wallpaper.motion': 'on', 'profile:controls.confirmButton': 'south',
  'profile:home.widgets.recent': true, 'profile:home.widgets.details': true, 'emulator.amiiboPrompt': '', 'recording.folder': '' };
function pathValue(path) {
  if (path === 'display.fullscreen') return S.system?.fullscreen ?? settingAt(S.settings, path);
  if (path.startsWith('profile:')) return settingAt(S.profile, path.slice(8));
  return settingAt(S.settings, path);
}
function pathDefault(path) {
  if (path.startsWith('profile:')) { if (!PROFILE_DEFAULTS) return undefined; const v = settingAt(PROFILE_DEFAULTS, path.slice(8)); return v === undefined && path in SETTING_UNSET_P ? SETTING_UNSET_P[path] : v; }
  if (!SETTING_DEFAULTS) return undefined;
  return settingAt(SETTING_DEFAULTS, path);
}
function pathChanged(path) {
  if (!(path.startsWith('profile:') ? PROFILE_DEFAULTS && S.profile : SETTING_DEFAULTS)) return false;
  let cur = pathValue(path), def = pathDefault(path);
  const unset = SETTING_UNSET[path] ?? SETTING_UNSET_P[path];
  if (unset !== undefined) { if (cur == null || cur === '') cur = unset; if (def == null || def === '') def = unset; }
  return !sameSetting(cur, def);
}
function nest(path, v) { return path.split('.').reduceRight((acc, k) => ({ [k]: acc }), v); }
async function resetPath(path) {
  let def = pathDefault(path);
  const unset = SETTING_UNSET[path] ?? SETTING_UNSET_P[path];
  if (def === undefined) def = unset !== undefined ? unset : null;
  if (path === 'display.fullscreen') { const r = await api('/api/app/fullscreen', { body: { on: !!def } }); if (S.system) S.system.fullscreen = r.fullscreen; S.settings = (await api('/api/state')).settings; return; }
  if (KEYBINDS.some(k => k[0] === path)) return resetKeybind(path, def);
  if (path === 'profile:controls.confirmButton') { const m = { ...(S.profile.controls.mapping || {}), confirm: 0, back: 1 }; return ctlPatch({ confirmButton: 'south', mapping: m }); }
  if (path.startsWith('profile:')) {
    if (!S.profile) return;
    await patchProfile(S.profile.id, nest(path.slice(8), def));
    if (S.screen === 'home') renderHome();
    if (S.screen === 'controllers') Controllers.refreshList();
    return;
  }
  if (path === 'updates.autoCheck') { await api('/api/update/settings', { body: { autoCheck: !!def } }); S.settings = (await api('/api/state')).settings; return; }
  await saveSettings(nest(path, def));
  if (path.startsWith('sound.')) Sound.init?.();
}
function markChanged(row) { if (row._reset) refreshReset(row); }
function markAllChanged() { $$('.row').forEach(r => r._reset && refreshReset(r)); }

// ---------------------------------------------------------------- keyboard shortcuts
// NEXUS's own keys (the game's controls are the engine's). Restoring one checks that its
// default is not in use by another shortcut, so two actions never share a key.
const KEYBINDS = [
  ['emulator.homeKey', 'Home key'], ['emulator.fullscreenKey', 'Fullscreen / window key'],
  ['emulator.screenshotKey', 'Screenshot key'], ['recording.key', 'Start / stop recording'],
];
async function resetKeybind(path, def) {
  const clash = KEYBINDS.find(([p]) => p !== path && def && String(settingAt(S.settings, p) || '').toLowerCase() === String(def).toLowerCase());
  if (clash) throw new Error(`${def} is now used by “${clash[1]}”. Change or restore that shortcut first, or use Restore all keybinds.`);
  await saveSettings(nest(path, def));
}
// Restore all keybinds: NEXUS's keyboard shortcuts, the controller buttons NEXUS listens to
// (Home, Capture, quit combination) and this user's controller buttons in menus and games.
async function resetAllKeybinds() {
  const who = S.profile ? `${S.profile.name}’s` : 'the current user’s';
  const ok = await dialog({ title: 'Restore all keybinds?', text: `These go back to their defaults:\n• Keyboard shortcuts: Home (Shift+H), full screen (F11), screenshot (Alt+F12), recording (F12)\n• Controller Home and Capture buttons, and the quit combination (View + Menu)\n• ${who} NEXUS menu buttons and controller layout for games\n\nNot changed: other settings, per-game controller layouts, the engine’s saved controller profiles, users and saves.`,
    buttons: [{ label: 'Cancel', value: false }, { label: 'Restore keybinds', value: true, danger: true }], cancelValue: false });
  if (!ok) return false;
  const r = await api('/api/keybinds/reset', { body: { profile: S.profile?.id || '' } });
  S.settings = r.settings;
  if (r.profile) { const i = S.profiles.findIndex(x => x.id === r.profile.id); if (i >= 0) S.profiles[i] = r.profile; S.profile = r.profile; }
  toast('Keybinds restored to their defaults', I.check);
  if (S.screen === 'settings') rebuildPanel();
  if (S.screen === 'controllers') Controllers.refreshList();
  renderHints();
  return true;
}

// ---------------------------------------------------------------- restore all settings
async function resetAllSettings() {
  const who = S.profile ? S.profile.name : 'the current user';
  const ok = await dialog({ title: 'Reset all settings?',
    text: `Every NEXUS setting goes back to its default: startup, sounds, full screen, playing games, keyboard shortcuts, shader preparation, screen recording, the performance monitor, library options, updates and the global graphics & emulation settings. ${who}’s appearance, Home screen look and controller settings are reset too.\n\nKept: your users, saves and play time, games and library folders, keys and firmware, the engine and its shader caches, your NEXUS account and GitHub connection, wallpapers and artwork, Home folders and pages, per-game settings, and other users’ preferences.`,
    buttons: [{ label: 'Cancel', value: false }, { label: 'Reset all settings', value: true, danger: true }], cancelValue: false });
  if (!ok) return false;
  const wasFull = S.system?.fullscreen ?? S.settings.display?.fullscreen;
  const r = await api('/api/settings/reset-all', { body: { profile: S.profile?.id || '' } });
  S.settings = r.settings;
  if (r.profile) { const i = S.profiles.findIndex(x => x.id === r.profile.id); if (i >= 0) S.profiles[i] = r.profile; S.profile = r.profile; }
  // what the interface shows changes at once: theme, sounds, the window
  applyTheme();
  if (r.settings.display?.fullscreen !== wasFull) { const f = await api('/api/app/fullscreen', { body: { on: !!r.settings.display.fullscreen } }).catch(() => null); if (f && S.system) S.system.fullscreen = f.fullscreen; }
  advancedSection.opened.clear();
  toast('All settings restored to their defaults', I.check, 3000);
  if (S.screen === 'settings') rebuildPanel();
  renderHints();
  return true;
}
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

// the game volume (global): 100 % is the default
function volumeReset(row) {
  const cur = () => +(S.settings.engine?.['audio.volume'] ?? 100);
  return addReset(row, { changed: () => cur() !== 100, def: () => '100%', run: async () => {
    const r = await api('/api/emulator/volume', { body: { percent: 100 } });
    S.settings.engine = S.settings.engine || {}; S.settings.engine['audio.volume'] = String(r.volume);
  } });
}

// In a game's settings: where a value comes from — this game's own setting (an override) or
// the global settings it follows.
function srcTag(row, overridden) {
  const host = row.querySelector('.rl > span'); if (!host) return row;
  host.querySelector('.src')?.remove();
  host.append(h('span', { class: 'src ' + (overridden ? 'game' : 'global'), title: overridden ? 'Set for this game only; the global setting is not changed.' : 'Follows Settings › Graphics & emulation.' }, overridden ? 'This game' : 'Global'));
  return row;
}
