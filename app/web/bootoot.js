// "OOT Launch": the special-edition startup animation (Settings › General › Startup animation).
// A NEXUS take on the gold, cinematic launch of a special-edition console: power on into red, a
// small pixel emblem cycling its colours, the emblem becoming a crest over a mosaic of gold and
// green triangles, a flash of white, then gold, then Home. Everything here is drawn by NEXUS
// (CSS, SVG and canvas) and the sound is NEXUS's own (tools/brand/make_oot_sound.py); no
// third-party artwork, logos or audio are used. Loaded after app.js (Boot, h, Sound).
//
// Timeline (ms) — the sound file follows the same marks:
//      0  black            200  red rises         700  pixel emblem, 4 colours (700/1000/1300/1600)
//   2500  emblem -> crest  2800  crest fills, mosaic appears   3600 light sweep
//   4450  glow -> white    5300  gold   5650 dark gold pattern   6250 Home

// a 13 x 13 pixel emblem (an original NEXUS gem): # outline, o face, n the letter
const OOT_PIXELS = [
  '....#####....',
  '...#ooooo#...',
  '..#ooooooo#..',
  '.#oonooonoo#.',
  '#ooonnoonooo#',
  '#ooononnoooo#',
  '#ooonoonnooo#',
  '#ooonooonooo#',
  '.#oonooonoo#.',
  '..#ooooooo#..',
  '...#ooooo#...',
  '....#ooo#....',
  '.....###.....',
];
const OOT_COLOURS = [ // face, letter: the emblem changes colour four times, then stays gold
  ['#3d7bff', '#ffffff'], ['#ff8a1f', '#fff3d6'], ['#2fb45a', '#f4ffe9'], ['#f2c63a', '#7a4b00'],
];

function ootPixelSVG() {
  const sz = 10, n = OOT_PIXELS.length;
  const rects = [];
  OOT_PIXELS.forEach((row, y) => [...row].forEach((c, x) => {
    if (c === '.') return;
    rects.push(`<rect x="${x * sz}" y="${y * sz}" width="${sz}" height="${sz}" class="p${c === '#' ? 'e' : c}"/>`);
  }));
  return `<svg viewBox="0 0 ${n * sz} ${n * sz}" shape-rendering="crispEdges" aria-hidden="true">${rects.join('')}</svg>`;
}

// the crest's lettering (gold): a small ornament, 40 with a raised TH, and ANNIVERSARY
function ootCrestTextSVG() {
  const serif = "'Cinzel','Trajan Pro','Georgia','DejaVu Serif',serif";
  return `<svg viewBox="0 0 140 150" aria-hidden="true">
    <defs><linearGradient id="ootTx" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff4c2"/><stop offset=".5" stop-color="#e2b54a"/><stop offset="1" stop-color="#a87a22"/></linearGradient>
      <filter id="ootTs"><feDropShadow dx="0" dy="1" stdDeviation=".8" flood-color="#000" flood-opacity=".55"/></filter></defs>
    <g filter="url(#ootTs)" fill="url(#ootTx)">
      <path d="M70 4 L76 14 L70 24 L64 14 Z M58 14 h-14 M82 14 h14" stroke="url(#ootTx)" stroke-width="1.4"/>
      <text x="64" y="102" text-anchor="middle" font-family="${serif}" font-size="72" font-weight="700" letter-spacing="-2">40</text>
      <text x="116" y="54" text-anchor="middle" font-family="${serif}" font-size="15" font-weight="700">TH</text>
      <rect x="22" y="110" width="96" height="1.4" opacity=".8"/>
      <text x="70" y="127" text-anchor="middle" font-family="${serif}" font-size="12.5" font-weight="700" letter-spacing="1.6">ANNIVERSARY</text>
    </g>
  </svg>`;
}

// the crest: a pennant shield (straight top, pointed bottom), drawn twice: white (the shape the
// emblem turns into) and the finished green-and-gold crest
function ootCrestSVG() {
  const shape = 'M8 6 H192 V170 L100 228 L8 170 Z';
  const inner = 'M20 18 H180 V163 L100 213 L20 163 Z';
  return `<svg viewBox="0 0 200 236" aria-hidden="true">
    <defs>
      <linearGradient id="ootG" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#1f7a3a"/><stop offset=".55" stop-color="#0f5a2a"/><stop offset="1" stop-color="#0a3d1d"/></linearGradient>
      <linearGradient id="ootAu" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff1b0"/><stop offset=".35" stop-color="#e3b84a"/><stop offset=".7" stop-color="#b8862a"/><stop offset="1" stop-color="#f3d27a"/></linearGradient>
    </defs>
    <g class="oot-crest-white"><path d="${shape}" fill="#f4f1ea"/></g>
    <g class="oot-crest-full">
      <path d="${shape}" fill="url(#ootAu)"/>
      <path d="${inner}" fill="url(#ootG)" stroke="#f6dc8a" stroke-width="2.5"/>
      <path d="M30 28 H170 V158 L100 202 L30 158 Z" fill="none" stroke="url(#ootAu)" stroke-width="1.5" opacity=".7"/>
    </g>
  </svg>`;
}

// the mosaic: rows of triangles in deep greens, teal, olive and gold, drawn once to a canvas
function ootMosaic(canvas, { dark = false } = {}) {
  const dpr = Math.min(2, window.devicePixelRatio || 1);
  const w = canvas.clientWidth || innerWidth, hgt = canvas.clientHeight || innerHeight;
  canvas.width = Math.round(w * dpr); canvas.height = Math.round(hgt * dpr);
  const c = canvas.getContext('2d'); c.scale(dpr, dpr);
  const side = Math.max(54, Math.min(w, hgt) / 7), th = side * Math.sqrt(3) / 2;
  const pal = dark ? ['#1b170e', '#221c10', '#2a2312', '#191510'] : ['#0f3b35', '#1c4a2f', '#2b5a3c', '#5e6338', '#8f8656', '#b9a777', '#c9a54a', '#173a2c', '#3d5f55', '#a8935c'];
  let seed = 1940;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  for (let r = -1, y = -th; y < hgt + th; r++, y += th) {
    for (let k = -2, x = -side; x < w + side; k++, x += side / 2) {
      const up = (k + r) % 2 === 0;
      c.beginPath();
      if (up) { c.moveTo(x, y + th); c.lineTo(x + side / 2, y); c.lineTo(x + side, y + th); }
      else { c.moveTo(x, y); c.lineTo(x + side, y); c.lineTo(x + side / 2, y + th); }
      c.closePath();
      const base = pal[Math.floor(rnd() * pal.length)];
      const g = c.createLinearGradient(x, y, x + side, y + th);
      g.addColorStop(0, base); g.addColorStop(1, dark ? '#120f09' : shade(base, -.25));
      c.fillStyle = g; c.fill();
      c.strokeStyle = dark ? 'rgba(214,176,82,.32)' : 'rgba(240,214,140,.28)'; c.lineWidth = dark ? 1.2 : 1; c.stroke();
    }
  }
  function shade(hex, f) {
    const n = parseInt(hex.slice(1), 16), ch = s => Math.max(0, Math.min(255, Math.round(((n >> s) & 255) * (1 + f))));
    return `rgb(${ch(16)},${ch(8)},${ch(0)})`;
  }
}

Object.assign(Boot, {
  goOOT(boot, reduced, E) {
    const logo = $('#boot-logo'); logo.replaceChildren(); logo.style.display = 'none';
    boot.classList.add('oot');
    const red = h('div', { class: 'oot-red' });
    const mosaic = h('canvas', { class: 'oot-mosaic' });
    const pix = h('div', { class: 'oot-pixel' }); pix.innerHTML = ootPixelSVG();
    const crest = h('div', { class: 'oot-crest' }); crest.innerHTML = ootCrestSVG();
    // inside the crest: a small ornament at the top, a large gold 40TH and ANNIVERSARY
    const mark = h('div', { class: 'oot-mark' }); mark.innerHTML = ootCrestTextSVG();
    const shine = h('div', { class: 'oot-shine' }, h('i'));
    crest.append(mark, shine);
    const white = h('div', { class: 'oot-white' }), gold = h('div', { class: 'oot-gold' });
    const pattern = h('canvas', { class: 'oot-pattern' });
    boot.append(red, mosaic, pix, crest, white, gold, pattern);
    ootMosaic(mosaic); ootMosaic(pattern, { dark: true });
    const snd = (S.settings.startup?.sound ?? true) && !reduced;
    if (snd) Sound.play('oot_launch', 1, { force: true });
    const hide = el => el.style.opacity = 0;
    [red, mosaic, pix, crest, white, gold, pattern, mark].forEach(hide);
    if (reduced) { // animations off: a short fade from the finished crest to Home
      Object.assign(red.style, { opacity: 0 }); mosaic.style.opacity = 1; crest.style.opacity = 1; mark.style.opacity = 1; crest.classList.add('full');
      E(boot, [{ opacity: 1 }, { opacity: 1, offset: .7 }, { opacity: 0 }], { duration: 1200 }).finished.then(() => this.finish()).catch(() => { });
      return;
    }
    const ease = 'cubic-bezier(.2,.8,.2,1)';
    // 1) power on into red
    E(red, [{ opacity: 0, background: '#000' }, { opacity: 1, background: '#b5121b' }], { duration: 500, delay: 200, easing: 'ease-out' });
    // 2) the pixel emblem appears and changes colour with each note
    E(pix, [{ opacity: 0, transform: 'translateY(8%) scale(.92)' }, { opacity: 1, transform: 'none' }], { duration: 260, delay: 700, easing: ease });
    OOT_COLOURS.forEach(([face, letter], i) => setTimeout(() => { pix.style.setProperty('--face', face); pix.style.setProperty('--letter', letter); }, 700 + i * 300));
    E(pix, [{ transform: 'none' }, { transform: 'translateY(-3%)' }, { transform: 'none' }], { duration: 600, delay: 1900, easing: 'ease-in-out', composite: 'add' });
    // 3) the emblem becomes the crest (white first)
    E(pix, [{ opacity: 1, transform: 'scale(1)', filter: 'brightness(1)' }, { opacity: 0, transform: 'scale(1.5)', filter: 'brightness(3)' }], { duration: 300, delay: 2500, easing: 'ease-in', fill: 'forwards' });
    E(crest, [{ opacity: 0, transform: 'scale(.35)' }, { opacity: 1, transform: 'scale(1.04)', offset: .7 }, { opacity: 1, transform: 'scale(1)' }], { duration: 420, delay: 2480, easing: ease });
    // 4) the crest fills with green and gold; the mosaic comes up over the red
    setTimeout(() => crest.classList.add('full'), 2800);
    E(mark, [{ opacity: 0, transform: 'scale(.9)' }, { opacity: 1, transform: 'none' }], { duration: 500, delay: 2950, easing: ease });
    E(mosaic, [{ opacity: 0, transform: 'scale(1.06)' }, { opacity: 1, transform: 'scale(1)' }], { duration: 900, delay: 2800, easing: 'ease-out' });
    E(red, [{ opacity: 1 }, { opacity: 0 }], { duration: 700, delay: 3000, easing: 'ease-in', fill: 'forwards' });
    E(mosaic, [{ transform: 'scale(1)' }, { transform: 'scale(1.035)' }], { duration: 1700, delay: 3700, easing: 'linear', fill: 'forwards' });
    // 5) a sweep of light across the crest, then a growing glow
    E(shine.firstChild, [{ transform: 'translateX(-60%) skewX(-18deg)', opacity: 0 }, { opacity: 1, offset: .2 }, { opacity: 1, offset: .8 }, { transform: 'translateX(320%) skewX(-18deg)', opacity: 0 }], { duration: 800, delay: 3600, easing: 'ease-in-out' });
    E(crest, [{ filter: 'drop-shadow(0 0 0 rgba(255,220,120,0))' }, { filter: 'drop-shadow(0 0 4vmin rgba(255,220,120,.85))' }], { duration: 700, delay: 3950, easing: 'ease-in', fill: 'forwards' });
    // 6) white, then gold, then the dark gold pattern
    E(white, [{ opacity: 0 }, { opacity: 1 }], { duration: 550, delay: 4450, easing: 'ease-in', fill: 'forwards' });
    E(gold, [{ opacity: 0 }, { opacity: 1 }], { duration: 300, delay: 5300, easing: 'ease-out', fill: 'forwards' });
    E(pattern, [{ opacity: 0, transform: 'scale(1.08)' }, { opacity: 1, transform: 'scale(1)' }], { duration: 600, delay: 5600, easing: ease, fill: 'forwards' });
    E(gold, [{ opacity: 1 }, { opacity: 0 }], { duration: 500, delay: 5650, easing: 'ease-out', fill: 'forwards' });
    // 7) into Home
    const bgTo = getComputedStyle(document.body).getPropertyValue('--bg').trim() || '#1d1e21';
    const out = E(boot, [{ opacity: 1, background: '#000' }, { opacity: 0, background: bgTo }], { duration: 500, delay: 6250, easing: 'ease-in-out', fill: 'forwards' });
    out.finished.then(() => this.finish()).catch(() => { });
  },
});

// Home's background while OOT Launch is the startup animation (Settings › General): the dark
// gold pattern the animation ends on, with a warm light in the middle. Drawn once per window size.
let ootWpCache = { key: '', url: '' };
function ootHomeWallpaper() {
  const st = S.settings?.startup || {};
  if (st.animation !== 'oot' || st.ootHome === false) return '';
  const w = Math.min(1920, innerWidth), hh = Math.min(1200, innerHeight); // drawn at up to 2x
  const key = w + 'x' + hh;
  if (ootWpCache.key === key) return ootWpCache.url;
  const cv = document.createElement('canvas');
  cv.style.cssText = `position:absolute;width:${w}px;height:${hh}px`;
  document.body.append(cv);
  try {
    ootMosaic(cv, { dark: true });
    const c = cv.getContext('2d'); c.setTransform(1, 0, 0, 1, 0, 0);
    const g = c.createRadialGradient(cv.width / 2, cv.height * .42, 0, cv.width / 2, cv.height * .42, Math.max(cv.width, cv.height) * .6);
    g.addColorStop(0, 'rgba(214,170,70,.30)'); g.addColorStop(.5, 'rgba(120,90,30,.10)'); g.addColorStop(1, 'rgba(0,0,0,.45)');
    c.fillStyle = g; c.fillRect(0, 0, cv.width, cv.height);
    ootWpCache = { key, url: cv.toDataURL('image/jpeg', .86) };
  } catch { ootWpCache = { key, url: '' }; }
  cv.remove();
  return ootWpCache.url;
}
