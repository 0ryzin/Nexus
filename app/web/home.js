// Home screen layouts and organisation (docs/HOME.md). NEXUS's own design, after the ideas of
// HomeNX's two looks (CardsNX: large cards on frosted glass with a panel for the selected game;
// the Wii U look: a grid with folders, widgets and a sidebar). No HomeNX code or assets are used.
//
// Everything here belongs to the user who is signed in (Profile.home, profiles.go): layout,
// favourites, folders, pages, order, shortcuts, widgets, background. Artwork and display names
// belong to the game and are the same for every user.
'use strict';

const HOME_LAYOUTS = [
  { v: 'classic', l: 'Classic', d: 'One row of game icons and the shortcut bar, as NEXUS has always looked.' },
  { v: 'cards', l: 'Cards', d: 'Large game cards on frosted glass, with the selected game’s details and its picture behind.' },
  { v: 'grid', l: 'Grid', d: 'All your games in a grid with folders and widgets, shortcuts down the side.' },
];
const HOME_WIDGETS = [
  ['details', 'Selected game', 'Play time, last played and version of the game you are on (Cards and Grid).'],
  ['recent', 'Recently played', 'Your last three games.'],
  ['playtime', 'Total play time', 'All your play time in NEXUS.'],
  ['favorites', 'Favourites', 'How many favourites you have, and the first ones.'],
  ['added', 'Recently added', 'Games NEXUS found most recently.'],
  ['storage', 'Library size', 'How many games, and the space they take.'],
];
const HOME_SORTS = [{ v: 'recent', l: 'Recently played' }, { v: 'alpha', l: 'Title A–Z' }, { v: 'playtime', l: 'Play time' }, { v: 'added', l: 'Date added' }, { v: 'custom', l: 'My order' }];

function homePrefs() {
  const h0 = S.profile?.home || {};
  return {
    layout: h0.layout || 'classic', order: h0.order || [], favorites: h0.favorites || [], hidden: h0.hidden || [],
    folders: h0.folders || [], pages: h0.pages || [], dock: h0.dock || [], dockHidden: h0.dockHidden || [],
    widgets: h0.widgets || { recent: true, details: true }, effects: h0.effects || 'glass', gameBackground: !!h0.gameBackground,
    bgColor: h0.bgColor || '', count: h0.count || 12,
  };
}
// homePatch: saves part of this user's home preferences, then redraws Home
async function homePatch(patch) {
  if (!S.profile) return;
  await patchProfile(S.profile.id, { home: patch });
  if (S.screen === 'home') renderHome();
}
const homeId = () => Math.random().toString(36).slice(2, 10);

// ---------- which games, in which order, on which page
const HomeState = { page: 'all' };

function homeSorted(list, sort) {
  const p = homePrefs();
  const st = k => S.stats[k] || {};
  if (sort === 'alpha') return [...list].sort((a, b) => a.title.localeCompare(b.title));
  if (sort === 'playtime') return [...list].sort((a, b) => (st(b.key).seconds || 0) - (st(a.key).seconds || 0) || a.title.localeCompare(b.title));
  if (sort === 'added') return [...list].sort((a, b) => (b.added || 0) - (a.added || 0) || a.title.localeCompare(b.title));
  if (sort === 'custom') {
    const pos = new Map(p.order.map((k, i) => [k, i]));
    return [...list].sort((a, b) => (pos.has(a.key) ? pos.get(a.key) : 1e9) - (pos.has(b.key) ? pos.get(b.key) : 1e9) || a.title.localeCompare(b.title));
  }
  const recentIdx = new Map(S.recent.map((k, i) => [k, i]));
  return [...list].sort((a, b) => (recentIdx.has(a.key) ? recentIdx.get(a.key) : 1e9) - (recentIdx.has(b.key) ? recentIdx.get(b.key) : 1e9) || a.title.localeCompare(b.title));
}

function homePageList() {
  const p = homePrefs();
  const out = [{ id: 'all', name: 'Games' }];
  if (p.favorites.length) out.push({ id: 'fav', name: 'Favourites' });
  if (S.recent.length) out.push({ id: 'recent', name: 'Recently played' });
  for (const pg of p.pages) out.push({ id: pg.id, name: pg.name });
  return out;
}

// homeItems: what the current page shows - games, and on the main page the folders
function homeItems() {
  const p = homePrefs();
  const sort = S.profile?.sort || 'recent';
  const playable = S.games.filter(g => isPlayable(g) && !g.hidden);
  const pages = homePageList();
  if (!pages.some(x => x.id === HomeState.page)) HomeState.page = 'all';
  const page = HomeState.page;
  if (page === 'fav') return homeSorted(playable.filter(g => p.favorites.includes(g.key)), sort).map(g => ({ g }));
  if (page === 'recent') return S.recent.map(gameByKey).filter(g => g && isPlayable(g) && !g.hidden).slice(0, 24).map(g => ({ g }));
  const custom = p.pages.find(x => x.id === page);
  if (custom) return homeSorted(playable.filter(g => custom.games.includes(g.key)), sort).map(g => ({ g }));
  const inFolder = new Set(p.folders.flatMap(f => f.games));
  const hidden = new Set(p.hidden);
  let games = homeSorted(playable.filter(g => !hidden.has(g.key) && !inFolder.has(g.key)), sort);
  if (p.layout === 'classic') games = games.slice(0, p.count);
  const folders = p.folders.filter(f => f.games.some(k => gameByKey(k))).map(f => ({ folder: f }));
  return p.layout === 'classic' ? [...games.map(g => ({ g })), ...folders] : [...folders, ...games.map(g => ({ g }))];
}

// ---------- drawing
function folderTile(f) {
  const games = f.games.map(gameByKey).filter(Boolean);
  const t = h('button', { class: 'tile folder', 'data-nav': true, role: 'listitem', title: f.name });
  const mini = h('div', { class: 'fold-art' });
  for (const g of games.slice(0, 4)) {
    const cell = h('span', { class: 'fold-cell' });
    if (g.artSource) cell.append(h('img', { src: `/art/${g.key}?v=${g.artRev || 0}&w=128`, alt: '', loading: 'lazy', decoding: 'async' }));
    mini.append(cell);
  }
  t.append(h('div', { class: 'art' }, mini), h('span', { class: 'fold-name' }, f.name), h('span', { class: 'fold-n' }, String(games.length)));
  t._folder = f;
  t.addEventListener('click', () => openHomeFolder(f.id));
  t.addEventListener('contextmenu', e => { e.preventDefault(); folderMenu(f.id); });
  t.addEventListener('navfocus', () => { updateSelLabel(null); const l = $('#sel-label'); l.querySelector('.sel-title').textContent = f.name; l.querySelector('.sel-meta').textContent = `Folder · ${games.length} game${games.length === 1 ? '' : 's'}`; homeDetails(null, f); });
  return t;
}

// homeLayoutChrome: classes, the page strip, the shortcut bar's order; called by renderHome
function homeLayoutChrome() {
  const p = homePrefs(), home = $('#home');
  for (const l of HOME_LAYOUTS) home.classList.toggle('hl-' + l.v, p.layout === l.v);
  home.classList.toggle('fx-flat', p.effects === 'flat');
  home.style.setProperty('--home-bg', p.bgColor || '');
  document.body.classList.toggle('home-bg-color', !!p.bgColor && S.screen === 'home');
  const strip = $('#home-pages'); strip.innerHTML = '';
  const pages = homePageList();
  strip.hidden = pages.length < 2;
  for (const pg of pages) {
    const b = h('button', { class: 'hp-tab' + (pg.id === HomeState.page ? ' on' : ''), 'aria-current': pg.id === HomeState.page ? 'page' : null }, pg.name);
    b.addEventListener('click', () => { HomeState.page = pg.id; renderHome(); });
    strip.append(b);
  }
  if (!strip.hidden) strip.append(h('span', { class: 'hp-hint' }, PadInfo?.active?.() && S.lastInput === 'gamepad' ? `${PadInfo.label(4)} / ${PadInfo.label(5)}` : ''));
}

function homeTab(d) {
  const pages = homePageList(); if (pages.length < 2) return;
  const i = Math.max(0, pages.findIndex(x => x.id === HomeState.page));
  HomeState.page = pages[(i + d + pages.length) % pages.length].id;
  renderHome();
  const t = $('#tiles .tile'); if (t) Nav.setFocus(t, { silent: true });
}

// the shortcut bar (dock / sidebar): the user's order, without the hidden ones
function homeDockOrder(items) {
  const p = homePrefs();
  const byId = new Map(items.map(it => [it.id, it]));
  const out = [];
  for (const id of p.dock) if (byId.has(id)) { out.push(byId.get(id)); byId.delete(id); }
  for (const it of items) if (byId.has(it.id)) out.push(it);
  return out.filter(it => !p.dockHidden.includes(it.id) || it.id === 'settings');
}

// the panel of the selected game (Cards, Grid) and its picture behind (Cards)
function homeDetails(g, folder) {
  const p = homePrefs();
  const box = $('#home-details');
  const show = p.layout !== 'classic' && p.widgets.details !== false;
  box.hidden = !show;
  const bg = $('#home-gamebg');
  if (p.layout === 'cards' && p.gameBackground && g) {
    const src = g.bgRev ? `/art/${g.key}?src=background&v=${g.bgRev}` : (g.artSource ? `/art/${g.key}?v=${g.artRev || 0}&w=512` : '');
    if (bg._src !== src) {
      bg._src = src;
      bg.classList.toggle('from-icon', !g.bgRev);
      bg.style.backgroundImage = src ? `url("${src}")` : '';
    }
    const logo = $('#home-logo');
    logo.hidden = !g.logoRev; if (g.logoRev) logo.src = `/art/${g.key}?src=logo&v=${g.logoRev}`;
    bg.hidden = !src;
  } else { bg.hidden = true; $('#home-logo').hidden = true; bg._src = ''; }
  if (!show) return;
  box.innerHTML = '';
  if (folder) {
    box.append(h('div', { class: 'hd-text' }, h('b', {}, folder.name), h('span', {}, `Folder with ${folder.games.filter(gameByKey).length} game(s)`)));
    const open = h('button', { class: 'hd-play', 'data-nav': true }, 'Open'); open.addEventListener('click', () => openHomeFolder(folder.id));
    box.append(h('div', { class: 'hd-actions' }, open));
    return;
  }
  if (!g) { box.hidden = true; return; }
  const st = S.stats[g.key] || {};
  const fav = p.favorites.includes(g.key);
  const icon = h('span', { class: 'hd-icon' }); if (g.artSource) icon.append(h('img', { src: `/art/${g.key}?v=${g.artRev || 0}&w=128`, alt: '' }));
  box.append(icon, h('div', { class: 'hd-text' }, h('b', {}, g.title + (fav ? ' ★' : '')),
    h('span', {}, [st.seconds ? fmtPlay(st.seconds) : 'Not played yet', st.lastPlayed ? 'Last played ' + fmtAgo(st.lastPlayed) : '', g.version ? (/^v/i.test(g.version) ? g.version : 'v' + g.version) : ''].filter(Boolean).join(' · '))));
  const more = h('button', { class: 'hd-more', 'data-nav': true, 'aria-label': 'Options' }, svg(I.more || I.gear));
  const play = h('button', { class: 'hd-play', 'data-nav': true }, svg(I.play), S.playing?.running && S.playing.gameKey === g.key ? 'Resume' : 'Play');
  play.addEventListener('click', () => launchGame(g.key));
  // Up from the panel goes back to the game it shows
  play._navMove = more._navMove = dir => { if (dir !== 'up') return false; const t = $$('#tiles .tile').find(t => t._game?.key === g.key); if (t) { Nav.setFocus(t); return true; } return false; };
  more.addEventListener('click', () => gameMenu(g.key));
  box.append(h('div', { class: 'hd-actions' }, play, more));
}

// widgets: drawn once per Home render from data NEXUS already has (no timers, no requests)
function homeWidgets() {
  const p = homePrefs(), box = $('#home-widgets');
  box.innerHTML = '';
  const on = k => p.widgets[k] === true;
  const show = p.layout !== 'classic' && HOME_WIDGETS.some(([k]) => k !== 'details' && on(k));
  box.hidden = !show;
  if (!show) return;
  const card = (title, ...body) => h('div', { class: 'widget' }, h('small', {}, title), ...body);
  const mini = g => {
    const b = h('button', { class: 'w-game', 'data-nav': true, title: g.title });
    if (g.artSource) b.append(h('img', { src: `/art/${g.key}?v=${g.artRev || 0}&w=128`, alt: '', loading: 'lazy' }));
    b.append(h('span', {}, g.title)); b.addEventListener('click', () => launchGame(g.key)); return b;
  };
  if (on('recent')) { const gs = S.recent.map(gameByKey).filter(g => g && isPlayable(g)).slice(0, 2); box.append(card('Recently played', ...(gs.length ? gs.map(mini) : [h('span', { class: 'w-none' }, 'Nothing yet')]))); }
  if (on('playtime')) { const total = Object.values(S.stats).reduce((a, s) => a + (s.seconds || 0), 0); box.append(card('Total play time', h('b', { class: 'w-big' }, total ? fmtPlay(total) : '0 min'))); }
  if (on('favorites')) { const gs = p.favorites.map(gameByKey).filter(Boolean); box.append(card(`Favourites (${gs.length})`, ...(gs.length ? gs.slice(0, 2).map(mini) : [h('span', { class: 'w-none' }, 'Options › Organise')]))); }
  if (on('added')) { const gs = S.games.filter(g => isPlayable(g) && !g.hidden && g.added).sort((a, b) => b.added - a.added).slice(0, 2); box.append(card('Recently added', ...(gs.length ? gs.map(mini) : [h('span', { class: 'w-none' }, 'Nothing yet')]))); }
  if (on('storage')) { const gs = S.games.filter(g => !g.hidden); box.append(card('Library', h('b', { class: 'w-big' }, `${gs.filter(isPlayable).length} games`), h('span', {}, fmtBytes(gs.reduce((a, g) => a + (g.size || 0), 0))))); }
}

// ---------- folders
function openHomeFolder(id) {
  sheet('Folder', (body, layer, rebuild) => {
    const f = homePrefs().folders.find(x => x.id === id); if (!f) { layer.close(); return; }
    body.append(h('h3', { class: 'set-h' }, f.name));
    const grid = h('div', { class: 'fold-grid' });
    for (const g of homeSorted(f.games.map(gameByKey).filter(Boolean), S.profile?.sort || 'recent')) {
      const t = gameTile(g); t.addEventListener('click', () => { layer.close(); launchGame(g.key); }); grid.append(t);
    }
    if (!grid.children.length) body.append(h('p', { class: 'note' }, 'This folder is empty. Add games with their Options › Add to folder.'));
    body.append(grid);
    body.append(rowButton('Rename folder', { onClick: async () => { if (await renameGroup('folders', id)) rebuild(); } }));
    body.append(rowButton('Delete folder', { danger: true, sub: 'Its games go back to Home. Nothing is deleted from your PC.', onClick: async () => {
      if (!await confirmDlg(`Delete ${f.name}?`, 'The folder goes; its games stay in your library and on Home.', 'Delete folder', true)) return;
      await homePatch({ folders: homePrefs().folders.filter(x => x.id !== id) }); layer.close();
    } }));
  });
}
async function folderMenu(id) { openHomeFolder(id); }

async function renameGroup(kind, id) {
  const list = homePrefs()[kind]; const gr = list.find(x => x.id === id); if (!gr) return false;
  const n = await dialog({ title: kind === 'folders' ? 'Folder name' : 'Page name', input: { value: gr.name, max: 40 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Save', value: true, primary: true }] });
  if (!n || !n.trim()) return false;
  await homePatch({ [kind]: list.map(x => x.id === id ? { ...x, name: n.trim() } : x) });
  return true;
}
async function newGroup(kind, firstGame) {
  const n = await dialog({ title: kind === 'folders' ? 'New folder' : 'New page', input: { value: '', max: 40 }, buttons: [{ label: 'Cancel', value: null }, { label: 'Create', value: true, primary: true }] });
  if (!n || !n.trim()) return null;
  const gr = { id: homeId(), name: n.trim(), games: firstGame ? [firstGame] : [] };
  await homePatch({ [kind]: [...homePrefs()[kind], gr] });
  return gr;
}

// ---------- organising one game (its Options menu › Organise)
async function homeOrganise(key) {
  const g = gameByKey(key); if (!g) return;
  const p = homePrefs();
  const fav = p.favorites.includes(key), hiddenHome = p.hidden.includes(key);
  const inFolder = p.folders.find(f => f.games.includes(key));
  const items = [
    { v: 'fav', l: fav ? 'Remove from Favourites' : 'Add to Favourites' },
    { v: 'folder', l: inFolder ? `Move to another folder (now in ${inFolder.name})` : 'Add to a folder…' },
    inFolder ? { v: 'unfolder', l: `Take out of ${inFolder.name}` } : null,
    { v: 'page', l: 'Add to a page or remove from one…' },
    { v: 'hide', l: hiddenHome ? 'Show on Home again' : 'Hide from Home (stays in All Software)' },
    { v: 'left', l: 'Move earlier (My order)' }, { v: 'right', l: 'Move later (My order)' },
  ].filter(Boolean);
  const v = await listPick(`Organise ${g.title}`, items);
  if (!v) return;
  if (v === 'fav') await homePatch({ favorites: fav ? p.favorites.filter(k => k !== key) : [...p.favorites, key] });
  if (v === 'hide') await homePatch({ hidden: hiddenHome ? p.hidden.filter(k => k !== key) : [...p.hidden, key] });
  if (v === 'unfolder') await homePatch({ folders: p.folders.map(f => ({ ...f, games: f.games.filter(k => k !== key) })) });
  if (v === 'folder') {
    const choice = await listPick('Add to folder', [...p.folders.map(f => ({ v: f.id, l: f.name })), { v: '+', l: 'New folder…' }]);
    if (!choice) return;
    let target = choice;
    if (choice === '+') { const gr = await newGroup('folders'); if (!gr) return; target = gr.id; }
    const fs = homePrefs().folders.map(f => ({ ...f, games: f.id === target ? [...new Set([...f.games, key])] : f.games.filter(k => k !== key) }));
    await homePatch({ folders: fs });
  }
  if (v === 'page') {
    const choice = await listPick('Pages', [...p.pages.map(pg => ({ v: pg.id, l: (pg.games.includes(key) ? '✓ ' : '') + pg.name })), { v: '+', l: 'New page…' }]);
    if (!choice) return;
    if (choice === '+') { await newGroup('pages', key); return; }
    await homePatch({ pages: p.pages.map(pg => pg.id !== choice ? pg : { ...pg, games: pg.games.includes(key) ? pg.games.filter(k => k !== key) : [...pg.games, key] }) });
  }
  if (v === 'left' || v === 'right') {
    // My order starts from what Home shows now, so the first move does what it looks like
    const shown = homeItems().filter(x => x.g).map(x => x.g.key);
    const order = [...new Set([...(p.order.length ? p.order : shown), ...shown])];
    const i = order.indexOf(key), j = v === 'left' ? i - 1 : i + 1;
    if (i < 0 || j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    await patchProfile(S.profile.id, { sort: 'custom', home: { order } });
    renderHome();
    const t = $$('#tiles .tile').find(t => t._game?.key === key); if (t) Nav.setFocus(t, { silent: true });
  }
}

// ---------- Settings › Home screen
function homeSettingsPanel(p) {
  const hp = homePrefs();
  p.append(h('h2', {}, 'Home screen'), h('p', { class: 'note' }, `These settings belong to ${S.profile?.name || 'the current user'}; other users keep theirs.`));
  p.append(h('h3', {}, 'Layout'));
  const lays = h('div', { class: 'layout-pick', 'data-row': 'strict' });
  for (const l of HOME_LAYOUTS) {
    const b = h('button', { class: 'lp-item' + (hp.layout === l.v ? ' sel' : ''), 'data-nav': true, 'aria-pressed': hp.layout === l.v ? 'true' : 'false' },
      h('span', { class: 'lp-demo lp-' + l.v }, h('i'), h('i'), h('i'), h('i')), h('b', {}, l.l), h('small', {}, l.d));
    b.addEventListener('click', async () => { await homePatch({ layout: l.v }); rebuildPanel(); });
    lays.append(b);
  }
  p.append(lays);
  p.append(changedMark(rowChoice('Order of games', HOME_SORTS, S.profile?.sort || 'recent', v => patchProfile(S.profile.id, { sort: v }).then(() => renderHome()),
    'My order: move games with their Options › Organise › Move earlier / later.'), 'profile:sort'));
  if (hp.layout === 'classic') p.append(changedMark(rowChoice('Games on the Home row', [6, 9, 12, 18, 24].map(n => ({ v: n, l: String(n) })), hp.count, v => homePatch({ count: +v })), 'profile:home.count'));
  p.append(h('h3', {}, 'Look'));
  p.append(changedMark(rowChoice('Panels', [{ v: 'glass', l: 'Frosted glass' }, { v: 'flat', l: 'Flat (lightest on the graphics card)' }], hp.effects, v => homePatch({ effects: v })), 'profile:home.effects'));
  p.append(changedMark(rowToggle('Selected game’s picture behind the cards', hp.gameBackground, v => homePatch({ gameBackground: v }), 'Cards layout. A game’s own background picture when it has one (Options › Edit Game Artwork), else its icon, softened.'), 'profile:home.gameBackground'));
  p.append(changedMark(rowSwatches('Plain background colour', ['', '#101214', '#1b2a41', '#2b1d3a', '#173326', '#3a1f1f', '#e9ecef'], hp.bgColor, c => homePatch({ bgColor: c || '' }), { custom: true }), 'profile:home.bgColor'));
  p.append(rowButton('Wallpaper, theme and accent colour…', { sub: 'Your background picture, dimming, blur, dark or light theme and accent.', onClick: () => selectCat('appearance', true) }));
  p.append(h('h3', {}, 'Widgets'));
  p.append(h('p', { class: 'note' }, 'Cards and Grid layouts. They are drawn from what NEXUS already knows and do not run in the background.'));
  for (const [k, l, d] of HOME_WIDGETS) p.append(changedMark(rowToggle(l, hp.widgets[k] === true || (k === 'details' && hp.widgets[k] !== false), v => homePatch({ widgets: { ...hp.widgets, [k]: v } }), d), 'profile:home.widgets.' + k));
  p.append(h('h3', {}, 'Folders and pages'));
  for (const f of hp.folders) p.append(rowButton(f.name, { value: `${f.games.length} game(s)`, sub: 'Folder', onClick: () => openHomeFolder(f.id) }));
  p.append(rowButton('New folder…', { icon: I.plus, onClick: async () => { if (await newGroup('folders')) rebuildPanel(); } }));
  for (const pg of hp.pages) p.append(rowButton(pg.name, { value: `${pg.games.length} game(s)`, sub: 'Page · select to rename or delete', onClick: async () => {
    const v = await listPick(pg.name, [{ v: 'rename', l: 'Rename' }, { v: 'delete', l: 'Delete page' }]);
    if (v === 'rename' && await renameGroup('pages', pg.id)) rebuildPanel();
    if (v === 'delete') { await homePatch({ pages: homePrefs().pages.filter(x => x.id !== pg.id) }); rebuildPanel(); }
  } }));
  p.append(rowButton('New page…', { icon: I.plus, sub: 'A page of games you choose; switch pages with the shoulder buttons or Q / E.', onClick: async () => { if (await newGroup('pages')) rebuildPanel(); } }));
  if (hp.hidden.length) p.append(rowButton('Games hidden from Home', { value: String(hp.hidden.length), sub: 'Select to show them all again.', onClick: async () => { await homePatch({ hidden: [] }); rebuildPanel(); } }));
  p.append(h('h3', {}, 'Shortcuts'));
  p.append(h('p', { class: 'note' }, 'The shortcut bar (along the bottom in Classic, down the side in Cards and Grid). System Settings always stays.'));
  const all = homeDockItems();
  const ordered = (() => { const o = homeDockOrder(all.map(x => ({ ...x }))); const shown = new Set(o.map(x => x.id)); return [...o, ...all.filter(x => !shown.has(x.id))]; })();
  ordered.forEach((it, i) => {
    const hidden = hp.dockHidden.includes(it.id);
    p.append(rowButton(it.label, { value: hidden ? 'Hidden' : `#${i + 1}`, sub: it.id === 'settings' ? 'Always shown' : 'Select to move up, move down, show or hide', onClick: async () => {
      const v = await listPick(it.label, [i > 0 ? { v: 'up', l: 'Move up' } : null, i < ordered.length - 1 ? { v: 'down', l: 'Move down' } : null, it.id !== 'settings' ? { v: 'toggle', l: hidden ? 'Show' : 'Hide' } : null].filter(Boolean));
      if (!v) return;
      const ids = ordered.map(x => x.id);
      if (v === 'up' || v === 'down') { const j = v === 'up' ? i - 1 : i + 1; [ids[i], ids[j]] = [ids[j], ids[i]]; await homePatch({ dock: ids }); }
      if (v === 'toggle') await homePatch({ dockHidden: hidden ? hp.dockHidden.filter(x => x !== it.id) : [...hp.dockHidden, it.id] });
      rebuildPanel();
    } }));
  });
  p.append(h('h3', {}, 'Restore'));
  p.append(rowButton('Restore the default Home screen', { danger: true, sub: 'Classic layout, default widgets and shortcuts, plain theme background. Your folders, pages and favourites are kept.', onClick: async () => {
    if (!await confirmDlg('Restore the default Home screen?', 'Layout, panels, widgets, shortcuts and background colour go back to NEXUS’s defaults. Folders, pages, favourites and hidden games are kept.', 'Restore')) return;
    await patchProfile(S.profile.id, { sort: 'recent', home: { layout: 'classic', effects: 'glass', gameBackground: false, bgColor: '', dock: [], dockHidden: [], widgets: Object.fromEntries(HOME_WIDGETS.map(([k]) => [k, k === 'recent' || k === 'details'])), count: 12 } });
    renderHome(); rebuildPanel();
  } }));
}
