// Controllers in NEXUS menus: which controller a browser gamepad is, and its button prompts
// (docs/CONTROLLERS.md). Pure functions, so they are tested in Node (tools/uitest/pads_test.js).
//
// The menus read controllers through the browser's Gamepad API (standard mapping: button 0 is
// the bottom face button, 1 right, 2 left, 3 top, 4/5 shoulders, 6/7 triggers, 8 back/view/
// create, 9 start/menu/options, 10/11 stick presses, 12-15 D-pad, 16 home/guide, 17 share/
// capture/touchpad). The Gamepad API calls every XInput controller "Xbox 360 Controller" and gives
// no IDs for them, so the model comes from the NEXUS engine's detection (SDL, vendor and product
// IDs) when it can be matched; otherwise the name stays generic. Prompts follow the controller's
// family; the button they show is the one the same index reads, so a prompt always names the
// physical button that performs the action.
'use strict';

const PAD_LABELS = {
  xbox: { 0: 'A', 1: 'B', 2: 'X', 3: 'Y', 4: 'LB', 5: 'RB', 6: 'LT', 7: 'RT', 8: 'View', 9: 'Menu', 10: 'LS', 11: 'RS', 12: 'D-pad up', 13: 'D-pad down', 14: 'D-pad left', 15: 'D-pad right', 16: 'Xbox', 17: 'Share' },
  playstation: { 0: 'Cross', 1: 'Circle', 2: 'Square', 3: 'Triangle', 4: 'L1', 5: 'R1', 6: 'L2', 7: 'R2', 8: 'Create', 9: 'Options', 10: 'L3', 11: 'R3', 12: 'D-pad up', 13: 'D-pad down', 14: 'D-pad left', 15: 'D-pad right', 16: 'PS', 17: 'Touchpad' },
  // Nintendo: the standard mapping is by position, so the bottom button (index 0) is B
  nintendo: { 0: 'B', 1: 'A', 2: 'Y', 3: 'X', 4: 'L', 5: 'R', 6: 'ZL', 7: 'ZR', 8: '−', 9: '+', 10: 'LS', 11: 'RS', 12: 'D-pad up', 13: 'D-pad down', 14: 'D-pad left', 15: 'D-pad right', 16: 'Home', 17: 'Capture' },
  steam: { 0: 'A', 1: 'B', 2: 'X', 3: 'Y', 4: 'L1', 5: 'R1', 6: 'L2', 7: 'R2', 8: 'View', 9: 'Menu', 10: 'L3', 11: 'R3', 12: 'D-pad up', 13: 'D-pad down', 14: 'D-pad left', 15: 'D-pad right', 16: 'Steam', 17: '…' },
  generic: { 0: 'A', 1: 'B', 2: 'X', 3: 'Y', 4: 'L1', 5: 'R1', 6: 'L2', 7: 'R2', 8: 'Select', 9: 'Start', 10: 'L3', 11: 'R3', 12: 'D-pad up', 13: 'D-pad down', 14: 'D-pad left', 15: 'D-pad right', 16: 'Home', 17: 'Button 17' },
};

// parseGamepadId: what the browser's id string says. Chromium: "Xbox 360 Controller (XInput
// STANDARD GAMEPAD)" for XInput, "<name> (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)" otherwise;
// Firefox: "054c-0ce6-DualSense Wireless Controller".
function parseGamepadId(id) {
  const s = String(id || '');
  const out = { xinput: /xinput/i.test(s), vendor: 0, product: 0, label: s.replace(/\s*\([^)]*\)\s*$/, '').trim() };
  let m = s.match(/Vendor:\s*([0-9a-f]{4})\s+Product:\s*([0-9a-f]{4})/i);
  if (!m) { m = s.match(/^([0-9a-f]{4})-([0-9a-f]{4})-(.*)$/i); if (m) out.label = m[3].trim(); }
  if (m) { out.vendor = parseInt(m[1], 16); out.product = parseInt(m[2], 16); }
  return out;
}

const hex4 = n => n.toString(16).padStart(4, '0');

// familyFromVendor: when the model table does not know the product
function familyFromVendor(v) { return v === 0x045e ? 'xbox' : v === 0x054c ? 'playstation' : v === 0x057e ? 'nintendo' : v === 0x28de ? 'steam' : 'generic'; }

// identifyGamepads: one entry per browser gamepad, in the same order.
//   gamepads: [{ index, id }]; enginePads: /api/controllers pads (connected ones are used);
//   models: /api/state padModels ("vvvv:pppp" -> { model, name, family }).
// Returns [{ index, key, name, family, model, exact }]: key identifies the controller for the
// "controller for NEXUS menus" preference (the engine's key when the match is certain).
function identifyGamepads(gamepads, enginePads, models) {
  models = models || {};
  const eng = (enginePads || []).filter(p => p.connected !== false);
  const used = new Set();
  const xin = gamepads.filter(g => parseGamepadId(g.id).xinput);
  const engXbox = eng.filter(p => (p.family || familyFromVendor(p.vendor)) === 'xbox');
  return gamepads.map(g => {
    const p = parseGamepadId(g.id);
    if (p.xinput) {
      // XInput: the browser gives no IDs. One XInput pad and one Xbox controller in the engine:
      // they are the same. Several: the name only when they are all the same model.
      if (xin.length === 1 && engXbox.length === 1) {
        const e = engXbox[0]; used.add(e.key);
        return { index: g.index, key: e.key, name: e.label || e.name, family: 'xbox', model: e.model || 'xbox', exact: true };
      }
      const names = [...new Set(engXbox.map(e => e.label || e.name))];
      const same = engXbox.length === xin.length && names.length === 1;
      return { index: g.index, key: 'xinput:' + g.index, name: same ? names[0] : 'Xbox controller', family: 'xbox', model: same ? (engXbox[0].model || 'xbox') : 'xbox', exact: false };
    }
    const m = p.vendor ? models[hex4(p.vendor) + ':' + hex4(p.product)] : null;
    const e = p.vendor ? eng.find(x => x.vendor === p.vendor && x.product === p.product && !used.has(x.key)) : null;
    if (e) used.add(e.key);
    const family = m?.family || e?.family || (p.vendor ? familyFromVendor(p.vendor) : 'generic');
    const name = m?.name || e?.label || p.label || 'Game controller';
    const key = e?.key || (p.vendor ? `hid:${hex4(p.vendor)}:${hex4(p.product)}:${g.index}` : 'pad:' + g.index);
    return { index: g.index, key, name: /xbox 360/i.test(name) && family !== 'xbox' ? 'Game controller' : name, family, model: m?.model || e?.model || family, exact: !!(m || e) };
  });
}

function padLabelFor(family, index, model) {
  if (family === 'playstation' && index === 8 && /dualshock/.test(model || '')) return 'Share'; // PS4; PS5 says Create
  const t = PAD_LABELS[family] || PAD_LABELS.generic;
  return t[index] ?? ('Button ' + index);
}

// padGlyphSVG: the prompt icon for a button, as SVG markup (currentColor, 24x24 viewBox). Face
// buttons are round, shoulders and triggers rounded rectangles, PlayStation's face buttons are
// their shapes, and View / Menu / Options / Create their symbols.
function padGlyphSVG(family, index, model) {
  const label = padLabelFor(family, index, model);
  const wrap = (body, title) => `<svg class="pad-glyph" viewBox="0 0 24 24" role="img" aria-label="${title}"><title>${title}</title>${body}</svg>`;
  const ring = '<circle cx="12" cy="12" r="10.2" fill="none" stroke="currentColor" stroke-width="1.6"/>';
  const letter = (t, size = 11) => `<text x="12" y="12" text-anchor="middle" dominant-baseline="central" font-size="${size}" font-weight="700" font-family="system-ui,Segoe UI,sans-serif" fill="currentColor">${t}</text>`;
  if (family === 'playstation' && index <= 3) {
    const shape = [
      '<path d="M8 8l8 8M16 8l-8 8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>',
      '<circle cx="12" cy="12" r="4.6" fill="none" stroke="currentColor" stroke-width="2"/>',
      '<rect x="7.8" y="7.8" width="8.4" height="8.4" rx=".6" fill="none" stroke="currentColor" stroke-width="2"/>',
      '<path d="M12 7.2l5 8.4H7z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>',
    ][index];
    return wrap(ring + shape, label);
  }
  if (index <= 3) return wrap(ring + letter(label), label);
  if (index >= 4 && index <= 7) {
    const trig = index >= 6;
    const shape = trig ? '<path d="M3.5 9.5a5 5 0 0 1 5-5h7a5 5 0 0 1 5 5V19a1.5 1.5 0 0 1-1.5 1.5h-14A1.5 1.5 0 0 1 3.5 19z" fill="none" stroke="currentColor" stroke-width="1.6"/>'
      : '<rect x="2" y="6.5" width="20" height="11" rx="5.5" fill="none" stroke="currentColor" stroke-width="1.6"/>';
    return wrap(shape + `<text x="12" y="${trig ? 13.6 : 12}" text-anchor="middle" dominant-baseline="central" font-size="${label.length > 2 ? 7 : 8.5}" font-weight="700" font-family="system-ui,Segoe UI,sans-serif" fill="currentColor">${label}</text>`, label);
  }
  if (index === 8 || index === 9) {
    let sym;
    if (family === 'xbox' || family === 'steam') sym = index === 8 ? '<rect x="7" y="7.5" width="7" height="6" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/><rect x="10" y="10.5" width="7" height="6" rx="1" fill="none" stroke="currentColor" stroke-width="1.5"/>'
      : '<path d="M7.5 9h9M7.5 12h9M7.5 15h9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>';
    else if (family === 'playstation') sym = index === 8 ? '<path d="M9 7.5v9M12 7.5v9M15 7.5v9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" transform="rotate(30 12 12)"/>'
      : '<path d="M7.5 9h9M7.5 12h9M7.5 15h9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>';
    else sym = letter(family === 'nintendo' ? label : (index === 8 ? 'SEL' : 'ST'), family === 'nintendo' ? 14 : 7);
    return wrap(ring + sym, label);
  }
  if (index >= 12 && index <= 15) {
    const rot = { 12: 0, 15: 90, 13: 180, 14: 270 }[index];
    return wrap(`<path d="M9.5 3h5v5.5L12 11 9.5 8.5zM3 9.5h5.5L11 12l-2.5 2.5H3zM9.5 21v-5.5L12 13l2.5 2.5V21zM21 9.5v5h-5.5L13 12l2.5-2.5z" fill="currentColor" opacity=".3"/><path d="M9.5 3h5v5.5L12 11 9.5 8.5z" fill="currentColor" transform="rotate(${rot} 12 12)"/>`, label);
  }
  // the guide / home button: a house in the ring (no brand logos), named by its label
  if (index === 16) return wrap(ring + '<path d="M7.5 11.6 12 7.6l4.5 4v4.9h-3v-3h-3v3h-3z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>', label);
  return wrap(ring + letter(label.length > 3 ? label.slice(0, 3) : label, label.length > 2 ? 7 : 9), label);
}

// isDirectJoyCon: a Joy-Con the browser reads itself (a single Joy-Con, 057e:2006/2007, or the
// pair Chromium combines, 057e:200e). With Joy-Con Compatibility disabled the menus ignore these
// and use only what the user's Joy-Con software provides (joycond's combined Joy-Cons 057e:2008,
// Steam's virtual controller, an XInput controller from a Joy-Con tool), so a press is not read twice.
function isDirectJoyCon(id) {
  const p = parseGamepadId(id);
  return p.vendor === 0x057e && [0x2006, 0x2007, 0x200e].includes(p.product);
}

if (typeof module !== 'undefined') module.exports = { PAD_LABELS, parseGamepadId, identifyGamepads, padLabelFor, padGlyphSVG, isDirectJoyCon };
