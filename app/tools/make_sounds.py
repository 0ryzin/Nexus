"""Synthesises NEXUS's original UI sound effects (no samples, pure DSP)."""
import numpy as np, wave, os, sys

SR = 44100
OUT = sys.argv[1] if len(sys.argv) > 1 else "../web/assets/sounds"
os.makedirs(OUT, exist_ok=True)
rng = np.random.default_rng(7)


def t(d):
    return np.arange(int(SR * d)) / SR


def env(n, a=0.002, d=0.1):
    x = np.arange(n) / SR
    e = np.minimum(1, x / max(a, 1e-6)) * np.exp(-x / d)
    return e


def lp(x, k):  # one-pole low-pass
    y = np.zeros_like(x); acc = 0.0
    for i, v in enumerate(x):
        acc += k * (v - acc); y[i] = acc
    return y


def hp(x, k):
    return x - lp(x, k)


def bell(f, d, dec=0.6, amp=1.0):
    tt = t(d)
    partials = [(1, 1.0), (2.01, 0.35), (3.02, 0.12), (4.17, 0.06)]
    s = sum(a * np.sin(2 * np.pi * f * m * tt) * np.exp(-tt * (1.0 + m) / dec) for m, a in partials)
    return amp * s * np.minimum(1, tt / 0.003)


def click(d=0.06, tone=2400, body=180, amp=1.0):
    n = int(SR * d)
    noise = hp(rng.standard_normal(n), 0.35) * env(n, 0.0005, 0.006)
    tick = np.sin(2 * np.pi * tone * t(d)) * env(n, 0.0003, 0.012)
    thump = np.sin(2 * np.pi * body * t(d) * (1 - 0.3 * t(d) / d)) * env(n, 0.001, 0.025)
    return amp * (0.55 * noise + 0.5 * tick + 0.8 * thump)


def place(buf, sig, at):
    i = int(at * SR)
    end = min(len(buf), i + len(sig))
    buf[i:end] += sig[: end - i]


def write(name, x, gain=0.8):
    x = x / (np.max(np.abs(x)) + 1e-9) * gain
    # tiny fade-out to avoid clicks
    f = min(len(x), int(0.01 * SR)); x[-f:] *= np.linspace(1, 0, f)
    pcm = (x * 32767).astype(np.int16)
    with wave.open(os.path.join(OUT, name), "wb") as w:
        w.setnchannels(1); w.setsampwidth(2); w.setframerate(SR); w.writeframes(pcm.tobytes())
    print(name, f"{len(x)/SR:.2f}s")


# ---------- startup sequence layers (scheduled individually by the UI) ----------
def sweep_noise(d, k0, k1, shape):
    tt = t(d); n = rng.standard_normal(len(tt)); y = np.zeros_like(n); acc = 0.0
    k = k0 + (k1 - k0) * (tt / d)
    for i, v in enumerate(n):
        acc += k[i] * (v - acc); y[i] = acc
    return y * shape(tt / d)

# power on: soft sub thump + rising air as the screen lights up
d = 1.1; tt = t(d)
sub = np.sin(2*np.pi*(48 + 20*tt/d)*tt) * np.minimum(1, tt/0.04) * np.exp(-tt/0.45)
air = sweep_noise(d, 0.01, 0.08, lambda x: np.sin(np.pi*np.minimum(1, x))**2) * 0.5
write("boot_power.wav", sub*0.9 + air, 0.6)

# grip whoosh (played twice, panned left/right)
write("boot_whoosh.wav", sweep_noise(0.55, 0.03, 0.35, lambda x: np.sin(np.pi*x)**1.5), 0.5)

# grip snap: the controller "click into place"
s2 = np.zeros(int(SR*0.25)); place(s2, click(tone=2800, body=160), 0); place(s2, click(0.03, 4200, 400, 0.35), 0.018)
write("boot_snap.wav", s2, 0.8)

# the N being drawn: quick rising glide
d = 0.55; tt = t(d)
f = 420 + 900 * (tt/d)**1.6
ph = 2*np.pi*np.cumsum(f)/SR
glide = (np.sin(ph) + 0.3*np.sin(2*ph)) * np.sin(np.pi*np.minimum(1, tt/d))**1.2
write("boot_draw.wav", lp(glide, 0.35) + 0.15*sweep_noise(d, 0.05, 0.4, lambda x: np.sin(np.pi*x)), 0.35)

# connection sparkle as the ring flashes
sp = np.zeros(int(SR*1.3))
for i, fr in enumerate([2637, 3136, 3951, 4699, 5274, 3520]):
    place(sp, bell(fr, 0.7, 0.18, 0.5 - i*0.05), 0.035*i)
write("boot_sparkle.wav", sp, 0.45)

# signature chime when the NEXUS SWITCH wordmark lands
ch = np.zeros(int(SR*2.6))
place(ch, bell(1318.5, 2.0, 0.6, 0.55), 0.00)    # E6
place(ch, bell(1975.5, 2.2, 0.7, 0.5), 0.12)     # B6
place(ch, bell(2637.0, 2.0, 0.6, 0.25), 0.24)    # E7
pt = t(2.3)
pad = (np.sin(2*np.pi*659.25*pt) + 0.6*np.sin(2*np.pi*987.77*pt) + 0.4*np.sin(2*np.pi*1318.5*pt)) * np.minimum(1, pt/0.2) * np.exp(-pt/0.9) * 0.16
place(ch, pad, 0.05)
write("boot_chime.wav", ch, 0.8)

# fade to home: gentle descending air
write("boot_outro.wav", sweep_noise(0.8, 0.25, 0.02, lambda x: np.sin(np.pi*x)**2), 0.3)

# home screen build-in: soft pop (played per tile/dock icon, pitched by the UI)
pp = t(0.09)
write("home_pop.wav", np.sin(2*np.pi*(900 + 500*np.exp(-pp/0.02))*pp) * env(len(pp), 0.002, 0.03), 0.4)

# Navigation tick
n = t(0.045)
write("move.wav", np.sin(2*np.pi*1750*n) * env(len(n), 0.0008, 0.010) + 0.2*hp(rng.standard_normal(len(n)),0.4)*env(len(n),0.0003,0.003), 0.35)

# Select / confirm: quick up-chirp click
s = np.zeros(int(SR*0.18))
place(s, click(0.05, 2200, 220, 0.6), 0)
place(s, bell(1046.5, 0.16, 0.08, 0.6), 0.012)
place(s, bell(1568, 0.14, 0.07, 0.45), 0.04)
write("select.wav", s, 0.55)

# Back: downward pair
b = np.zeros(int(SR*0.16))
place(b, bell(1174.7, 0.12, 0.06, 0.6), 0)
place(b, bell(880, 0.12, 0.06, 0.6), 0.035)
write("back.wav", b, 0.45)

# Launch: airy whoosh + chord
d = 0.9; tt = t(d)
noise = rng.standard_normal(len(tt))
sweep = np.zeros_like(noise); acc = 0.0
k = 0.02 + 0.3 * (tt / d) ** 1.5
for i, v in enumerate(noise):
    acc += k[i] * (v - acc); sweep[i] = acc
wh = sweep * np.sin(np.pi * np.minimum(1, tt / d)) * 0.7
L = np.zeros(int(SR*1.4)); place(L, wh, 0)
for f, at in [(523.25, .45), (659.25, .5), (783.99, .55), (1046.5, .6)]:
    place(L, bell(f, 0.8, 0.35, 0.35), at)
write("launch.wav", L, 0.7)

# Error: low double buzz
e = np.zeros(int(SR*0.35))
for at in (0, 0.14):
    tt = t(0.12)
    place(e, np.sign(np.sin(2*np.pi*220*tt)) * 0.3 * env(len(tt), 0.002, 0.05), at)
write("error.wav", lp(e, 0.25), 0.5)

# Controller connected: two quick snaps
c = np.zeros(int(SR*0.3))
place(c, click(tone=2600, body=160), 0)
place(c, click(tone=3200, body=190), 0.07)
place(c, bell(1975.5, 0.2, 0.08, 0.3), 0.1)
write("connect.wav", c, 0.6)
