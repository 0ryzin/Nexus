// NEXUS on-screen keyboard, after the Steam Deck's: one keyboard for every text field, opened
// when a field is chosen with a controller (Nav.activate in app.js). Never opened by the mouse or
// a physical keyboard; a physical key pressed while it is open closes it and types as usual.
//
// Controller: D-pad / stick move between keys, A types the key, B closes, X deletes, Y adds a
// space, LB / RB move the cursor, Menu (Start) is Enter, View switches Shift. Mouse: click keys.
// Loaded before app.js; uses its helpers (h, svg, Nav, openLayer, Sound) only at run time.

const OSK = {
  target: null, layer: null, shift: false, caps: false, symbols: false, panel: null, preview: null,
  LETTERS: [
    ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '⌫'],
    ['q', 'w', 'e', 'r', 't', 'y', 'u', 'i', 'o', 'p', '-'],
    ['a', 's', 'd', 'f', 'g', 'h', 'j', 'k', 'l', "'", '⏎'],
    ['⇧', 'z', 'x', 'c', 'v', 'b', 'n', 'm', ',', '.', '?'],
  ],
  SYMBOLS: [
    ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '⌫'],
    ['!', '@', '#', '$', '%', '^', '&', '*', '(', ')', '_'],
    ['~', '`', '=', '+', '[', ']', '{', '}', '\\', '|', '⏎'],
    ['⇧', '<', '>', '/', ':', ';', '"', '€', '£', '…', '?'],
  ],

  // open(input): shows the keyboard for this field. Returns false if it cannot (not a text field).
  open(input) {
    if (!input || !isTextField(input) || input.disabled || input.readOnly) return false;
    if (this.target === input) return true;
    this.close(false);
    this.target = input;
    this.shift = false; this.caps = false; this.symbols = false;
    this.preview = h('div', { class: 'osk-preview', 'aria-live': 'polite' });
    this.keys = h('div', { class: 'osk-keys' });
    const legend = h('div', { class: 'osk-legend' },
      ...[['confirm', 'Type'], ['back', 'Close'], ['search', 'Delete'], ['options', 'Space'], ['menu', 'Enter'], ['view', 'Shift'], ['prevTab', '◀'], ['nextTab', '▶']]
        .map(([a, t]) => h('span', {}, PadInfo.glyph(Pad.mapping()[a] ?? 8, 'key inline'), t)));
    this.panel = h('div', { class: 'osk', role: 'dialog', 'aria-label': 'On-screen keyboard' }, this.preview, this.keys, legend);
    // the mouse must not take the focus away from the field it types into
    this.panel.addEventListener('mousedown', e => e.preventDefault());
    this.layer = openLayer(this.panel, { name: 'osk', onBack: () => this.close(true), onOptions: () => this.type(' '), onTab: d => this.caret(d) });
    this.layer.root.classList.add('osk-layer');
    this.draw();
    this.updatePreview();
    document.addEventListener('keydown', this.onKey, true);
    input.addEventListener('input', this.onInput);
    return true;
  },

  // close(refocus): hides the keyboard. The field keeps its text; refocus puts the controller
  // focus back on it.
  close(refocus = true) {
    if (!this.target) return;
    const t = this.target;
    document.removeEventListener('keydown', this.onKey, true);
    t.removeEventListener('input', this.onInput);
    this.target = null;
    this.layer?.close(); this.layer = null;
    if (refocus && document.contains(t)) { Nav.setFocus(t, { silent: true }); }
  },

  draw() {
    const rows = this.symbols ? this.SYMBOLS : this.LETTERS;
    const upper = this.shift || this.caps;
    const keep = Nav.cur && this.keys.contains(Nav.cur) ? Nav.cur.dataset.pos : null;
    this.keys.replaceChildren();
    rows.forEach((row, r) => {
      const line = h('div', { class: 'osk-row' });
      row.forEach((k, c) => line.append(this.key(k, upper && /^[a-z]$/.test(k) ? k.toUpperCase() : k, `${r}:${c}`)));
      this.keys.append(line);
    });
    const last = h('div', { class: 'osk-row' });
    last.append(this.key('sym', this.symbols ? 'ABC' : '&123', '4:0', 'wide'), this.key('◀', '◀', '4:1'), this.key(' ', 'Space', '4:2', 'space'), this.key('▶', '▶', '4:3'), this.key('close', 'Close', '4:4', 'wide'));
    this.keys.append(last);
    const shiftKey = $$('.osk-key', this.keys).find(b => b._k === '⇧');
    if (shiftKey) { shiftKey.classList.toggle('on', this.shift || this.caps); shiftKey.title = this.caps ? 'Caps lock (press again to turn off)' : 'Shift (press twice for caps lock)'; }
    const again = keep && $$('.osk-key', this.keys).find(b => b.dataset.pos === keep);
    Nav.setFocus(again || $$('.osk-key', this.keys).find(b => b._k === 'q' || b._k === '!'), { silent: true, scroll: false });
  },
  key(k, label, pos, cls = '') {
    const names = { '⌫': 'Delete', '⏎': 'Enter', '⇧': 'Shift', ' ': 'Space', sym: 'Symbols', close: 'Close keyboard', '◀': 'Cursor left', '▶': 'Cursor right' };
    const b = h('button', { class: 'osk-key ' + cls, 'data-nav': true, 'aria-label': names[k] || label, dataset: { pos } }, label);
    b._k = k;
    b.addEventListener('click', () => this.press(k));
    return b;
  },
  press(k) {
    switch (k) {
      case '⌫': return this.backspace();
      case '⏎': return this.enter();
      case '⇧': return this.toggleShift();
      case 'sym': this.symbols = !this.symbols; return this.draw();
      case 'close': return this.close(true);
      case '◀': return this.caret(-1);
      case '▶': return this.caret(1);
    }
    const upper = (this.shift || this.caps) && /^[a-z]$/.test(k);
    this.type(upper ? k.toUpperCase() : k);
    if (this.shift && !this.caps) { this.shift = false; this.draw(); }
  },
  toggleShift() {
    if (this.caps) { this.caps = false; this.shift = false; }
    else if (this.shift) { this.caps = true; }
    else this.shift = true;
    this.draw();
  },

  // ---- editing the field (works for every input type; email/number fields have no caret API)
  sel() {
    const t = this.target;
    try { if (typeof t.selectionStart === 'number') return [t.selectionStart, t.selectionEnd]; } catch { }
    return [t.value.length, t.value.length];
  },
  set(value, caret) {
    const t = this.target; if (!t) return;
    const max = t.maxLength > 0 ? t.maxLength : Infinity;
    if (value.length > max) return;
    t.value = value;
    try { t.setSelectionRange(caret, caret); } catch { }
    t.dispatchEvent(new Event('input', { bubbles: true }));
    this.updatePreview();
  },
  type(ch) {
    if (!this.target) return;
    const [a, b] = this.sel(), v = this.target.value;
    this.set(v.slice(0, a) + ch + v.slice(b), a + ch.length);
  },
  backspace() {
    if (!this.target) return;
    const [a, b] = this.sel(), v = this.target.value;
    if (a !== b) this.set(v.slice(0, a) + v.slice(b), a);
    else if (a > 0) this.set(v.slice(0, a - 1) + v.slice(a), a - 1);
  },
  caret(d) {
    const t = this.target; if (!t) return;
    const [a] = this.sel();
    const n = Math.max(0, Math.min(t.value.length, a + d));
    try { t.setSelectionRange(n, n); } catch { }
    this.updatePreview();
  },
  enter() {
    const t = this.target; if (!t) return;
    this.close(true);
    t.dispatchEvent(new CustomEvent('submit'));
  },
  updatePreview() {
    const t = this.target; if (!t || !this.preview) return;
    const shown = t.type === 'password' ? '•'.repeat(t.value.length) : t.value;
    const [a] = this.sel();
    const label = t.getAttribute('aria-label') || t.placeholder || t.closest('label')?.querySelector('span')?.textContent || '';
    this.preview.replaceChildren(h('small', {}, label), h('span', { class: 'osk-text' }, shown.slice(0, a), h('i', { class: 'osk-caret' }), shown.slice(a)));
  },
  onInput: () => OSK.updatePreview(),

  // A physical key while the keyboard is open: the keyboard closes and the key types into the
  // field as usual (arrow keys still move between the on-screen keys).
  onKey(e) {
    const t = OSK.target; if (!t) return;
    const k = e.key;
    const typing = (k.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) || k === 'Backspace' || k === 'Delete';
    if (k === 'Enter') { e.preventDefault(); e.stopPropagation(); OSK.enter(); return; }
    if (!typing) {
      if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); OSK.close(true); }
      return;
    }
    e.preventDefault(); e.stopPropagation();
    OSK.close(true);
    t.focus();
    if (k === 'Backspace') { OSK.target = t; OSK.backspace(); OSK.target = null; }
    else if (k === 'Delete') { /* nothing typed */ }
    else { OSK.target = t; OSK.type(k); OSK.target = null; }
    setInputMode('keyboard');
  },

  // Controller buttons while the keyboard is open (called from Pad.poll). Returns true when a
  // button belonged to the keyboard; movement and A / B / Y / LB / RB go through Nav as usual.
  pad(state, prev) {
    const edge = a => state[a] && !prev[a];
    if (edge('search')) { this.backspace(); Sound.play('move', .5); return true; }
    if (edge('menu')) { Sound.play('select'); this.enter(); return true; }
    if (edge('view')) { Sound.play('move', .5); this.toggleShift(); return true; }
    return false;
  },
};
