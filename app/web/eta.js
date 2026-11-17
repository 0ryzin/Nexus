// Remaining-time estimates for shader work (docs/SHADERS.md, "Remaining time").
//
// An estimate comes only from throughput measured in this run: a rolling window of
// (time, completed) samples. Nothing is assumed: with too little data it says it is still
// calculating, when the work makes no progress it says so instead of showing a stale time, and
// each stage has its own estimate (a new stage key starts a new window), because reading files,
// converting programs and compiling pipelines run at very different speeds.
'use strict';
class ShaderEta {
  constructor({ windowMs = 15000, minSpanMs = 3000, minSamples = 3, stallMs = 6000 } = {}) {
    Object.assign(this, { windowMs, minSpanMs, minSamples, stallMs });
    this.reset();
  }
  reset(key = null) { this.key = key; this.s = []; this.lastProgress = 0; this.lastCompleted = -1; }
  // add: one observation. key names the stage; completed only grows within a stage; remaining is
  // what is still to do (null when unknown). Returns {state, seconds, rate, text}:
  // state = complete | calculating | stalled | estimate | unknown.
  add(key, completed, remaining, now = Date.now()) {
    if (key !== this.key || completed < this.lastCompleted) this.reset(key);
    if (completed > this.lastCompleted) this.lastProgress = now;
    this.lastCompleted = completed;
    if (remaining === 0) return { state: 'complete', text: 'Shader preparation complete.' };
    if (remaining == null || !(remaining > 0)) return { state: 'unknown', text: '' };
    this.s.push({ t: now, c: completed });
    // keep the newest sample older than the window, so the window is always fully spanned
    while (this.s.length > 2 && this.s[1].t <= now - this.windowMs) this.s.shift();
    const first = this.s[0], span = now - first.t, done = completed - first.c;
    if (this.lastProgress && now - this.lastProgress >= this.stallMs && this.s.length >= this.minSamples) {
      const secs = Math.round((now - this.lastProgress) / 1000);
      return { state: 'stalled', text: `Waiting: no progress for ${fmtDuration(secs)}` };
    }
    if (this.s.length < this.minSamples || span < this.minSpanMs || done <= 0) {
      return { state: 'calculating', text: 'Calculating remaining time...' };
    }
    const rate = done / (span / 1000); // per second
    const seconds = remaining / rate;
    return { state: 'estimate', seconds, rate, text: seconds < 10 ? 'Less than 10 seconds remaining' : `About ${fmtDuration(seconds)} remaining` };
  }
}

// fmtDuration: a rounded, readable duration (estimates are not precise to the second).
function fmtDuration(sec) {
  sec = Math.max(0, sec);
  if (sec < 10) return 'a few seconds';
  if (sec < 60) return `${Math.ceil(sec / 5) * 5} seconds`;
  if (sec < 3600) { const m = Math.round(sec / 60); return m === 1 ? '1 minute' : `${m} minutes`; }
  const hrs = Math.floor(sec / 3600), m = Math.round((sec - hrs * 3600) / 60);
  return `${hrs} h ${m} min`;
}

// rate text: "140 per second", only when meaningful
function fmtRate(rate) {
  if (!(rate > 0)) return '';
  return rate >= 10 ? `${Math.round(rate).toLocaleString()} per second` : rate >= 1 ? `${rate.toFixed(1)} per second` : `${Math.round(rate * 60)} per minute`;
}

if (typeof module !== 'undefined') module.exports = { ShaderEta, fmtDuration, fmtRate };
