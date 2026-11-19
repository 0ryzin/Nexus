// Mii Creator (mii.go): each user's own Miis, in the Switch's Mii format, written to the engine's
// Mii database when that user starts a game. Separate from amiibo. Works with a controller: every
// row is a left/right choice or slider, names use the on-screen keyboard.
//
// The drawing is NEXUS's own simplified face (no Nintendo Mii parts): it shows colours, glasses,
// facial hair and proportions so a choice can be seen; games draw the real Mii.
const MiiUI = {
  // Colour palettes: the classic Mii colours as the engine maps them into the Switch's 100-colour
  // table (raw_data.cpp, Ver3*ColorTable). Hex values are approximations for the preview only.
  HAIR: [[8, 'Black', '#1e1a18'], [1, 'Dark brown', '#3a2416'], [2, 'Red brown', '#5d2a14'], [3, 'Brown', '#7a4628'],
    [4, 'Grey', '#8a8682'], [5, 'Olive brown', '#5a4a24'], [6, 'Dark blond', '#8c6430'], [7, 'Blond', '#d0a860']],
  EYE: [[8, 'Black', '#101010'], [9, 'Grey', '#6c7070'], [10, 'Brown', '#6a4a30'], [11, 'Hazel', '#7c7030'], [12, 'Blue', '#4a70c0'], [13, 'Green', '#3c8a50']],
  MOUTH: [[19, 'Orange', '#d86a3c'], [20, 'Red', '#d03030'], [21, 'Pink', '#e07888'], [22, 'Peach', '#e8a080'], [23, 'Dark', '#2a1a1a']],
  GLASS: [[8, 'Black', '#151515'], [14, 'Brown', '#6a4026'], [15, 'Red', '#b02a2a'], [16, 'Blue', '#2a4ab0'], [17, 'Gold', '#c8a030'], [18, 'Grey', '#8a8a8a']],
  SKIN: ['#f6d6b6', '#f0c8a0', '#d8a07a', '#e8b880', '#b07850', '#7c5038', '#fae0c8', '#f2cfa8', '#c8946a', '#5a3a28'],
  SKIN_NAMES: ['Beige', 'Warm beige', 'Natural', 'Honey', 'Chestnut', 'Porcelain', 'Ivory', 'Warm ivory', 'Almond', 'Espresso'],
  FAV: [['Red', '#e0302a'], ['Orange', '#f07818'], ['Yellow', '#f8d820'], ['Lime green', '#88c828'], ['Green', '#18884a'], ['Blue', '#2860c8'],
    ['Light blue', '#38b0e8'], ['Pink', '#f070a8'], ['Purple', '#7838a8'], ['Brown', '#704020'], ['White', '#f8f8f8'], ['Black', '#202020']],
  FACE_SHAPES: ['Sharp', 'Rounded', 'Sharp rounded', 'Sharp rounded (small)', 'Large', 'Large rounded', 'Sharp (small)', 'Flat', 'Bump', 'Angular', 'Flat rounded', 'Angular (small)'],
  WRINKLES: ['None', 'Tear troughs', 'Facial pain', 'Cheeks', 'Folds', 'Under the eyes', 'Split chin', 'Chin', 'Brow droop', 'Mouth frown', 'Crow’s feet', 'Folds and frown'],
  MAKEUP: ['None', 'Porcelain cheeks', 'Natural cheeks', 'Blue eye shadow', 'Porcelain blush', 'Natural blush', 'Porcelain + blue shadow', 'Porcelain + natural shadow', 'Blush + espresso shadow', 'Freckles', 'Light beard shadow', 'Stubble'],
  BEARDS: ['None', 'Goatee', 'Long goatee', 'Long full beard', 'Full beard', 'Short full beard', 'Chin strap'],
  MUSTACHES: ['None', 'Walrus', 'Pencil', 'Horseshoe', 'Normal', 'Toothbrush', 'Thin'],
  GLASSES: ['None', 'Oval', 'Wayfarer', 'Rectangle', 'Top rimless', 'Rounded', 'Oversized', 'Cat eye', 'Square', 'Bottom rimless',
    'Tinted rounded', 'Tinted cat eye', 'Tinted oval', 'Tinted rectangle', 'Tinted aviator', 'Sunglasses rounded', 'Sunglasses cat eye', 'Sunglasses oval', 'Sunglasses rectangle', 'Sunglasses aviator'],

  colorOf(list, v) { return (list.find(c => c[0] === v) || list[0])[2]; },

  // the simplified drawing (SVG string), from the Mii's fields
  face(f, size = 120) {
    const skin = this.SKIN[f.facelineColor] || this.SKIN[0];
    const hair = this.colorOf(this.HAIR, f.hairColor), eye = this.colorOf(this.EYE, f.eyeColor);
    const lip = this.colorOf(this.MOUTH, f.mouthColor), gl = this.colorOf(this.GLASS, f.glassColor), beard = this.colorOf(this.HAIR, f.beardColor);
    const brow = this.colorOf(this.HAIR, f.eyebrowColor);
    const wide = 30 + (f.build - 64) / 12, tall = 36 + (f.height - 64) / 16;
    const round = [6, 14, 10, 10, 6, 16, 6, 4, 8, 3, 14, 3][f.facelineType] ?? 8;
    const ey = 50 + (f.eyeY - 12) * 1.4, ex = 11 + f.eyeX * 0.9, es = 2.6 + f.eyeScale * 0.35;
    const by = ey - 8 - (f.eyebrowY - 7) * 0.6, my = 74 + (f.mouthY - 13) * 1.3, ms = 6 + f.mouthScale;
    const ny = 62 + (f.noseY - 9) * 1.1;
    let s = `<svg viewBox="0 0 100 110" width="${size}" height="${size * 1.1}" xmlns="http://www.w3.org/2000/svg">`;
    s += `<rect x="${50 - wide - 3}" y="18" width="${(wide + 3) * 2}" height="${tall + 30}" rx="22" fill="${hair}"/>`;
    s += `<rect x="${50 - wide}" y="${26}" width="${wide * 2}" height="${tall * 2}" rx="${round + 12}" fill="${skin}"/>`;
    s += `<path d="M${50 - wide - 3} 36 Q50 ${f.hairFlip ? 6 : 10} ${50 + wide + 3} 36 L${50 + wide + 3} 28 Q50 4 ${50 - wide - 3} 28Z" fill="${hair}"/>`;
    if (f.beardType) s += `<path d="M${50 - wide + 4} ${66} Q50 ${100 + tall / 4} ${50 + wide - 4} 66 Q50 ${86} ${50 - wide + 4} 66Z" fill="${beard}"/>`;
    for (const d of [-1, 1]) {
      s += `<ellipse cx="${50 + d * ex}" cy="${ey}" rx="${es}" ry="${es * (1.3 - f.eyeAspect * 0.08)}" fill="${eye}"/>`;
      s += `<rect x="${50 + d * ex - 5}" y="${by}" width="10" height="2.2" rx="1" fill="${brow}" transform="rotate(${d * (f.eyebrowRotate - 6) * -3} ${50 + d * ex} ${by})"/>`;
      if (f.glassType) s += `<rect x="${50 + d * ex - 7}" y="${ey - 6}" width="14" height="12" rx="${f.glassType % 2 ? 6 : 2}" fill="${f.glassType >= 15 ? gl : 'none'}" fill-opacity="${f.glassType >= 15 ? .85 : f.glassType >= 10 ? .35 : 0}" stroke="${gl}" stroke-width="1.6"/>`;
    }
    if (f.glassType) s += `<line x1="${50 - ex + 7}" y1="${ey}" x2="${50 + ex - 7}" y2="${ey}" stroke="${gl}" stroke-width="1.4"/>`;
    s += `<path d="M48 ${ny - 5} Q${50 + 2 + f.noseScale * 0.4} ${ny} 48 ${ny + 2}" stroke="#00000055" stroke-width="1.4" fill="none"/>`;
    if (f.mustacheType) s += `<path d="M${50 - ms} ${my - 3} Q50 ${my - 8} ${50 + ms} ${my - 3} Q50 ${my - 4} ${50 - ms} ${my - 3}Z" fill="${beard}" stroke="${beard}" stroke-width="2"/>`;
    s += `<path d="M${50 - ms} ${my} Q50 ${my + 4 + (f.mouthType % 5)} ${50 + ms} ${my}" stroke="${lip}" stroke-width="2.4" fill="none" stroke-linecap="round"/>`;
    if (f.moleType) s += `<circle cx="${30 + f.moleX * 2.5}" cy="${40 + f.moleY * 1.2}" r="1.3" fill="#3a2418"/>`;
    s += `<rect x="30" y="${28 + tall * 2}" width="40" height="${110}" rx="12" fill="${this.FAV[f.favoriteColor]?.[1] || '#888'}"/>`;
    return s + '</svg>';
  },

  card(m, isDefault, onClick) {
    const pic = h('span', { class: 'mii-pic' }); pic.innerHTML = this.face(m.fields, 72);
    const b = h('button', { class: 'mii-card' + (isDefault ? ' default' : ''), 'data-nav': true }, pic, h('b', {}, m.name), isDefault ? h('small', {}, 'Default') : '');
    b.addEventListener('click', onClick);
    return b;
  },

  // Settings › Users › a user › Miis…
  async page(profileId) {
    const pr = S.profiles.find(p => p.id === profileId);
    return sheet(`Miis — ${pr?.name || ''}`, async (body, layer, rebuild) => {
      let d;
      try { d = await api(`/api/profiles/${profileId}/miis`); } catch (e) { body.append(h('p', { class: 'note' }, e.message)); return; }
      this.meta = d;
      const c = d.collection;
      body.append(h('p', { class: 'note' }, `${pr?.name}’s own Miis, in the Switch’s Mii format. When ${pr?.name} starts a game, NEXUS puts these into the game’s Mii list, the default first, so games that use Miis (Mario Kart 8 Deluxe’s Mii racer, for example) offer them. Other users’ Miis are kept separately. The picture is a simplified drawing; games show the real Mii.`));
      const acts = h('div', { class: 'sd-actions', 'data-row': 'strict' });
      const add = (label, fn) => { const b = h('button', { class: 'btn', 'data-nav': true }, label); b.addEventListener('click', fn); acts.append(b); };
      if (c.miis.length < d.max) add('New Mii…', async () => {
        const g = await dialog({ title: 'New Mii', text: 'Start from:', buttons: [{ label: 'Cancel', value: null }, { label: 'Female base', value: 'female' }, { label: 'Male base', value: 'male', primary: true }], cancelValue: null });
        if (g) this.editor(profileId, { id: '', name: '', fields: { ...d.base[g] } }, rebuild);
      });
      add('Import Mii database…', async () => {
        const v = await dialog({ title: 'Import Miis', text: 'Add the Miis of a Mii database (MiiDatabase.dat) to this user. Miis the user already has are not added twice.', buttons: [{ label: 'Cancel', value: null }, ...(d.engineHasDatabase ? [{ label: 'From this device’s games', value: 'engine' }] : []), { label: 'Choose a file…', value: 'file', primary: true }], cancelValue: null });
        if (!v) return;
        try {
          let r;
          if (v === 'engine') r = await api(`/api/profiles/${profileId}/miis/import?from=engine`, { body: {} });
          else { const f = await pickFile('.dat,application/octet-stream'); if (!f) return; r = await api(`/api/profiles/${profileId}/miis/import`, { body: f }); }
          toast(`${r.added} Mii${r.added === 1 ? '' : 's'} added${r.skipped ? `, ${r.skipped} not usable` : ''}`, I.check, 3500); rebuild();
        } catch (e) { alertDlg('Not imported', e.message); }
      });
      body.append(acts);
      if (!c.miis.length) { body.append(h('div', { class: 'sd-empty' }, h('b', {}, 'No Miis yet'), h('span', {}, 'Create one, or import the Miis games made on this device.'))); return; }
      const grid = h('div', { class: 'mii-grid' });
      for (const m of c.miis) grid.append(this.card(m, m.id === c.default, () => this.actions(profileId, m, m.id === c.default, rebuild)));
      body.append(grid);
      body.append(h('p', { class: 'note' }, `${c.miis.length} of ${d.max}.`));
    });
  },

  async actions(profileId, m, isDefault, rebuild) {
    const v = await dialog({ title: m.name, buttons: [{ label: 'Close', value: null }, { label: 'Delete…', value: 'delete', danger: true }, ...(isDefault ? [] : [{ label: 'Make default', value: 'default' }]), { label: 'Edit', value: 'edit', primary: true }], cancelValue: null });
    if (v === 'edit') this.editor(profileId, JSON.parse(JSON.stringify(m)), rebuild);
    if (v === 'default') { try { await api(`/api/profiles/${profileId}/miis/default`, { body: { id: m.id } }); toast(`${m.name} is the default Mii`, I.check); rebuild(); } catch (e) { alertDlg('Not changed', e.message); } }
    if (v === 'delete' && await confirmDlg(`Delete ${m.name}?`, `${m.name} is removed from this user’s Miis and from the game Mii list the next time this user starts a game. This cannot be undone.`, 'Delete', true)) {
      try { await api(`/api/profiles/${profileId}/miis/delete`, { body: { id: m.id } }); toast('Deleted', I.check); rebuild(); } catch (e) { alertDlg('Not deleted', e.message); }
    }
  },

  // the editor: one row per part, each a left/right choice or slider (controller), with the drawing
  editor(profileId, m, done) {
    const lim = this.meta.limits, f = m.fields;
    const pic = h('div', { class: 'mii-preview' });
    const paint = () => { pic.innerHTML = this.face(f, 140); };
    const num = (key, label, from = 1) => rowRange(label, { min: 0, max: lim[key], step: 1, value: f[key], fmt: v => String(v + from), onChange: v => { f[key] = v; paint(); } });
    const pick = (key, label, names) => rowChoice(label, names.map((n, i) => ({ v: i, l: n })), f[key], v => { f[key] = v; paint(); });
    const pickN = (key, label, word) => rowChoice(label, Array.from({ length: lim[key] + 1 }, (_, i) => ({ v: i, l: `${word} ${i + 1}` })), f[key], v => { f[key] = v; paint(); });
    const col = (key, label, list) => rowChoice(label, list.map(c => ({ v: c[0], l: c[1] })), list.some(c => c[0] === f[key]) ? f[key] : list[0][0], v => { f[key] = v; paint(); });
    const l = sheet(m.id ? `Edit ${m.name}` : 'New Mii', (body, layer) => {
      paint();
      body.append(h('div', { class: 'mii-edit-top' }, pic, h('div', {},
        rowButton('Name', { value: m.name || 'Choose…', onClick: async () => {
          const n = await dialog({ title: 'Mii name', text: `Up to ${this.meta.nameMax} characters.`, input: { value: m.name, max: this.meta.nameMax }, buttons: [{ label: 'Cancel', value: null }, { label: 'OK', value: true, primary: true }] });
          if (n != null && n.trim()) { m.name = n.trim(); layer.rebuild(); }
        } }),
        pick('gender', 'Gender', ['Male', 'Female']),
        rowChoice('Favourite colour', this.FAV.map((c, i) => ({ v: i, l: c[0] })), f.favoriteColor, v => { f.favoriteColor = v; paint(); }))));
      const sect = t => body.append(h('h3', {}, t));
      sect('Body'); body.append(num('height', 'Height'), num('build', 'Build'));
      sect('Face'); body.append(pick('facelineType', 'Face shape', this.FACE_SHAPES), pick('facelineColor', 'Skin tone', this.SKIN_NAMES), pick('facelineWrinkle', 'Lines', this.WRINKLES), pick('facelineMake', 'Make-up', this.MAKEUP));
      sect('Hair'); body.append(pickN('hairType', 'Hairstyle', 'Style'), col('hairColor', 'Hair colour', this.HAIR), pick('hairFlip', 'Parting', ['Left', 'Right']));
      sect('Eyebrows'); body.append(pickN('eyebrowType', 'Eyebrows', 'Type'), col('eyebrowColor', 'Colour', this.HAIR), num('eyebrowScale', 'Size'), num('eyebrowAspect', 'Thickness'), num('eyebrowRotate', 'Angle'), num('eyebrowX', 'Spacing'), num('eyebrowY', 'Height'));
      sect('Eyes'); body.append(pickN('eyeType', 'Eyes', 'Type'), col('eyeColor', 'Colour', this.EYE), num('eyeScale', 'Size'), num('eyeAspect', 'Shape'), num('eyeRotate', 'Angle'), num('eyeX', 'Spacing'), num('eyeY', 'Height'));
      sect('Nose'); body.append(pickN('noseType', 'Nose', 'Type'), num('noseScale', 'Size'), num('noseY', 'Height'));
      sect('Mouth'); body.append(pickN('mouthType', 'Mouth', 'Type'), col('mouthColor', 'Colour', this.MOUTH), num('mouthScale', 'Size'), num('mouthAspect', 'Shape'), num('mouthY', 'Height'));
      sect('Facial hair'); body.append(pick('beardType', 'Beard', this.BEARDS), pick('mustacheType', 'Moustache', this.MUSTACHES), col('beardColor', 'Colour', this.HAIR), num('mustacheScale', 'Moustache size'), num('mustacheY', 'Moustache height'));
      sect('Glasses'); body.append(pick('glassType', 'Glasses', this.GLASSES), col('glassColor', 'Colour', this.GLASS), num('glassScale', 'Size'), num('glassY', 'Height'));
      sect('Mole'); body.append(pick('moleType', 'Mole', ['None', 'Yes']), num('moleScale', 'Size'), num('moleX', 'Across'), num('moleY', 'Down'));
      const save = h('button', { class: 'btn primary', 'data-nav': true }, 'Save Mii');
      save.addEventListener('click', async () => {
        if (!m.name) { alertDlg('Name needed', 'Give the Mii a name first.'); return; }
        try { await api(`/api/profiles/${profileId}/miis`, { body: m }); toast(`${m.name} saved`, I.check); layer.close(); done && done(); }
        catch (e) { alertDlg('Not saved', e.message); }
      });
      body.append(h('div', { class: 'sd-actions', 'data-row': 'strict' }, save));
      body.append(h('p', { class: 'note' }, 'Hairstyles, eyes, eyebrows, noses and mouths are numbered in the order the Switch stores them; games draw them, the picture above does not.'));
    });
    return l;
  },
};
