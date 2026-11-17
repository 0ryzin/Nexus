// Unit tests for web/pads.js (run: node tools/uitest/pads_test.js). No dependencies.
'use strict';
const assert = require('assert');
const path = require('path');
const P = require(path.join(__dirname, '..', '..', 'web', 'pads.js'));

let n = 0;
function test(name, fn) { fn(); n++; console.log('ok', name); }
const models = {
  '045e:0b12': { model: 'xbox-series', name: 'Xbox Series X|S Controller', family: 'xbox' },
  '045e:0b13': { model: 'xbox-series', name: 'Xbox Series X|S Controller', family: 'xbox' },
  '054c:0ce6': { model: 'dualsense', name: 'PlayStation 5 Controller (DualSense)', family: 'playstation' },
  '054c:09cc': { model: 'dualshock4', name: 'PlayStation 4 Controller (DualShock 4)', family: 'playstation' },
  '057e:2009': { model: 'switch-pro', name: 'Nintendo Switch Pro Controller', family: 'nintendo' },
};
const XINPUT = 'Xbox 360 Controller (XInput STANDARD GAMEPAD)';
const series = { key: 'sn:abc:045e0b12', label: 'Xbox Series X|S Controller', model: 'xbox-series', family: 'xbox', vendor: 0x045e, product: 0x0b12, connected: true };

test('parse the browser ids', () => {
  assert.deepStrictEqual(P.parseGamepadId(XINPUT), { xinput: true, vendor: 0, product: 0, label: 'Xbox 360 Controller' });
  const d = P.parseGamepadId('DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)');
  assert.strictEqual(d.vendor, 0x054c); assert.strictEqual(d.product, 0x0ce6); assert.strictEqual(d.label, 'DualSense Wireless Controller');
  const f = P.parseGamepadId('054c-09cc-Wireless Controller');
  assert.strictEqual(f.product, 0x09cc); assert.strictEqual(f.label, 'Wireless Controller');
});

test('a Series X|S controller through XInput is named from the engine, never "Xbox 360"', () => {
  const [i] = P.identifyGamepads([{ index: 0, id: XINPUT }], [series], models);
  assert.strictEqual(i.name, 'Xbox Series X|S Controller'); assert.strictEqual(i.family, 'xbox'); assert.strictEqual(i.key, series.key); assert.ok(i.exact);
});

test('without the engine an XInput pad gets a generic Xbox name', () => {
  const [i] = P.identifyGamepads([{ index: 0, id: XINPUT }], [], models);
  assert.strictEqual(i.name, 'Xbox controller'); assert.strictEqual(i.family, 'xbox'); assert.ok(!i.exact);
});

test('two different Xbox controllers through XInput: generic names, separate keys', () => {
  const one = { ...series, key: 'k1' }, s360 = { key: 'k2', label: 'Xbox 360 Controller', model: 'xbox-360', family: 'xbox', vendor: 0x045e, product: 0x028e, connected: true };
  const r = P.identifyGamepads([{ index: 0, id: XINPUT }, { index: 1, id: XINPUT }], [one, s360], models);
  assert.deepStrictEqual(r.map(x => x.name), ['Xbox controller', 'Xbox controller']);
  assert.notStrictEqual(r[0].key, r[1].key);
  const same = P.identifyGamepads([{ index: 0, id: XINPUT }, { index: 1, id: XINPUT }], [one, { ...series, key: 'k3' }], models);
  assert.deepStrictEqual(same.map(x => x.name), ['Xbox Series X|S Controller', 'Xbox Series X|S Controller']);
});

test('PlayStation and Nintendo by vendor and product, with the engine key when connected', () => {
  const ds = { key: 'sn:a4:054c0ce6', label: 'PlayStation 5 Controller (DualSense)', family: 'playstation', vendor: 0x054c, product: 0x0ce6, connected: true };
  const r = P.identifyGamepads([
    { index: 0, id: 'DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)' },
    { index: 1, id: 'Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)' },
    { index: 2, id: 'Unknown Pad (STANDARD GAMEPAD Vendor: 1234 Product: 5678)' },
  ], [ds], models);
  assert.strictEqual(r[0].name, 'PlayStation 5 Controller (DualSense)'); assert.strictEqual(r[0].key, ds.key); assert.strictEqual(r[0].family, 'playstation');
  assert.strictEqual(r[1].family, 'nintendo'); assert.strictEqual(r[1].name, 'Nintendo Switch Pro Controller');
  assert.strictEqual(r[2].family, 'generic'); assert.strictEqual(r[2].name, 'Unknown Pad');
});

test('prompts name the physical button that the index reads', () => {
  assert.strictEqual(P.padLabelFor('xbox', 0), 'A');
  assert.strictEqual(P.padLabelFor('playstation', 0), 'Cross');
  assert.strictEqual(P.padLabelFor('playstation', 1), 'Circle');
  assert.strictEqual(P.padLabelFor('playstation', 8, 'dualsense'), 'Create');
  assert.strictEqual(P.padLabelFor('playstation', 8, 'dualshock4'), 'Share');
  // the bottom button of a Switch Pro Controller is B, the right one A (standard mapping is by position)
  assert.strictEqual(P.padLabelFor('nintendo', 0), 'B');
  assert.strictEqual(P.padLabelFor('nintendo', 1), 'A');
  assert.strictEqual(P.padLabelFor('xbox', 4), 'LB'); assert.strictEqual(P.padLabelFor('playstation', 6), 'L2');
});

test('glyphs are SVG with a title, no emoji', () => {
  for (const fam of ['xbox', 'playstation', 'nintendo', 'steam', 'generic']) for (let i = 0; i <= 17; i++) {
    const s = P.padGlyphSVG(fam, i);
    assert.ok(s.startsWith('<svg') && s.includes('<title>'), fam + i);
    assert.ok(!/[\u{1F300}-\u{1FAFF}☀-➿]/u.test(s), 'emoji in ' + fam + i);
  }
  assert.ok(P.padGlyphSVG('playstation', 0).includes('<path d="M8 8l8 8')); // the cross is drawn, not a letter
  assert.ok(P.padGlyphSVG('xbox', 0).includes('>A</text>'));
});
console.log(`${n} passed`);
