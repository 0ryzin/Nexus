// Unit tests for web/eta.js (run: node tools/uitest/eta_test.js). No dependencies.
'use strict';
const assert = require('assert');
const path = require('path');
const { ShaderEta, fmtDuration, fmtRate } = require(path.join(__dirname, '..', '..', 'web', 'eta.js'));

let n = 0;
function test(name, fn) { fn(); n++; console.log('ok', name); }

test('too little data: calculating, never a number', () => {
  const e = new ShaderEta();
  assert.strictEqual(e.add('a', 0, 1000, 0).state, 'calculating');
  assert.strictEqual(e.add('a', 10, 990, 500).state, 'calculating'); // 2 samples, 0.5 s
  assert.strictEqual(e.add('a', 20, 980, 1000).text, 'Calculating remaining time...');
});

test('steady throughput: remaining / measured rate', () => {
  const e = new ShaderEta();
  let r;
  for (let t = 0; t <= 4000; t += 1000) r = e.add('a', t / 10, 1000 - t / 10, t); // 100 per second
  assert.strictEqual(r.state, 'estimate');
  assert.ok(Math.abs(r.rate - 100) < 1e-9, r.rate);
  assert.ok(Math.abs(r.seconds - 6) < 1e-9, r.seconds); // 600 left at 100/s
  assert.strictEqual(r.text, 'Less than 10 seconds remaining');
  const e2 = new ShaderEta(); for (let t = 0; t <= 4000; t += 1000) r = e2.add('b', t / 100, 10000, t); // 10/s, 10000 left
  assert.strictEqual(r.text, 'About 17 minutes remaining');
});

test('rolling window follows a change of speed', () => {
  const e = new ShaderEta({ windowMs: 5000 });
  let c = 0, r;
  for (let t = 0; t <= 20000; t += 1000) { c += t < 10000 ? 100 : 10; r = e.add('a', c, 5000, t); }
  assert.ok(r.rate < 15 && r.rate > 9, 'rate follows recent throughput: ' + r.rate);
});

test('no progress: stalled, not a stale estimate', () => {
  const e = new ShaderEta();
  for (let t = 0; t <= 4000; t += 1000) e.add('a', t, 10000, t);
  let r;
  for (let t = 5000; t <= 12000; t += 1000) r = e.add('a', 4000, 6000, t);
  assert.strictEqual(r.state, 'stalled');
  assert.match(r.text, /^Waiting: no progress for/);
});

test('a new stage starts its own estimate', () => {
  const e = new ShaderEta();
  for (let t = 0; t <= 5000; t += 1000) e.add('read', t, 100000, t);
  assert.strictEqual(e.add('convert', 0, 500, 6000).state, 'calculating');
});

test('done and unknown', () => {
  const e = new ShaderEta();
  assert.deepStrictEqual(e.add('a', 10, 0, 0), { state: 'complete', text: 'Shader preparation complete.' });
  assert.strictEqual(e.add('a', 10, null, 0).state, 'unknown');
});

test('formatting', () => {
  assert.strictEqual(fmtDuration(4), 'a few seconds');
  assert.strictEqual(fmtDuration(42), '45 seconds');
  assert.strictEqual(fmtDuration(90), '2 minutes');
  assert.strictEqual(fmtDuration(3 * 3600 + 600), '3 h 10 min');
  assert.strictEqual(fmtRate(0), '');
  assert.strictEqual(fmtRate(0.5), '30 per minute');
  assert.strictEqual(fmtRate(2.5), '2.5 per second');
});
console.log(`${n} passed`);
