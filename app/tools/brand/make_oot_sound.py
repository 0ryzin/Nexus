# Generates web/assets/sounds/oot_launch.wav: the sound of the "OOT Launch" startup animation.
# Entirely synthesised here (no recorded or third-party audio): plucked notes, bells, a pad,
# filtered noise and a small reverb. Its events line up with the animation's timeline in
# web/app.js (Boot.goOOT); change both together.
#   python3 make_oot_sound.py <output.wav>
import sys
import numpy as np
from scipy.io import wavfile
from scipy.signal import fftconvolve

SR = 32000
LEN = 6.8
out = np.zeros(int(SR * LEN))
rng = np.random.default_rng(40)

def at(t, sig, gain=1.0):
    i = int(t * SR)
    n = min(len(sig), len(out) - i)
    out[i:i + n] += sig[:n] * gain

def t_(d): return np.arange(int(d * SR)) / SR

def pluck(f, d=1.6):
    t = t_(d)
    s = sum(np.sin(2 * np.pi * f * k * t) / k ** 1.6 * np.exp(-t * (2.2 + k * 1.3)) for k in range(1, 7))
    return s * np.minimum(1, t * 400)

def bell(f, d=2.5):
    t = t_(d)
    parts = [(1, 1, 1.4), (2.0, .5, 2.2), (2.76, .35, 3.0), (5.4, .2, 5.0), (8.9, .1, 7.0)]
    return sum(a * np.sin(2 * np.pi * f * r * t) * np.exp(-t * dcy) for r, a, dcy in parts) * np.minimum(1, t * 600)

def pad(freqs, d, attack, release):
    t = t_(d)
    s = sum(np.sin(2 * np.pi * f * t + np.sin(2 * np.pi * .3 * t) * .4) + .3 * np.sin(2 * np.pi * 2 * f * 1.002 * t) for f in freqs)
    env = np.minimum(1, t / attack) * np.minimum(1, (d - t) / release)
    return s * env / len(freqs)

def noise_sweep(d, f0, f1, rise=True):
    n = rng.standard_normal(int(d * SR))
    t = t_(d)
    cut = f0 * (f1 / f0) ** (t / d)
    y = np.zeros_like(n); a = 0.0
    alpha = 1 - np.exp(-2 * np.pi * cut / SR)
    for i in range(len(n)):  # one-pole low-pass with a moving cutoff
        a += alpha[i] * (n[i] - a); y[i] = a
    env = (t / d) ** 2 if rise else np.exp(-t * 4)
    return (n - y) * .15 * env + y * env  # some air above the cutoff

def boom(d=1.6, f=55):
    t = t_(d)
    return np.sin(2 * np.pi * f * t * (1 + .3 * np.exp(-t * 6))) * np.exp(-t * 2.4) * np.minimum(1, t * 200)

note = lambda m: 440 * 2 ** ((m - 69) / 12)
# 0.2 s: power on, a low pad opening
at(0.2, pad([note(38), note(45), note(50)], 2.6, 0.5, 1.0), .35)
# 0.7-1.6 s: four rising plucks, one per colour of the pixel emblem (D major pentatonic)
for i, (t, m) in enumerate([(0.7, 74), (1.0, 78), (1.3, 81), (1.6, 83)]):
    at(t, pluck(note(m)), .5)
    at(t + .004, pluck(note(m + 12), 1.0), .12)
# 1.9 s: the chord the gold emblem holds on
for m in (74, 78, 81, 86):
    at(1.9, pluck(note(m), 2.2), .28)
at(1.9, pad([note(62), note(66), note(69)], 1.0, .2, .6), .25)
# 2.5 s: air rising into 2.8 s, where the emblem becomes the badge
at(2.2, noise_sweep(0.6, 300, 6000), .35)
at(2.8, bell(note(86)), .45); at(2.8, bell(note(93), 1.6), .2); at(2.8, boom(1.2, 73), .35)
# 2.9-3.5 s: sparkle as the badge fills and the mosaic appears
for k, m in enumerate([98, 102, 105, 110, 105, 109]):
    at(2.95 + k * .09, bell(note(m), .7), .07)
# 3.6 s: the light sweep across the badge
at(3.55, noise_sweep(0.5, 2000, 9000), .12)
for k, m in enumerate([105, 109, 112]):
    at(3.65 + k * .07, bell(note(m), .6), .06)
# 3.9-4.95 s: swell into the white
at(3.9, pad([note(62), note(69), note(74), note(78)], 1.15, 1.0, .1), .5)
at(4.2, noise_sweep(0.8, 500, 10000), .3)
# 5.0 s: the white flash
for m, g in ((86, .35), (90, .25), (93, .25), (98, .2)):
    at(5.0, bell(note(m), 1.8), g)
# 5.4 s: gold, a warm low hit
at(5.4, boom(1.4, 49), .55); at(5.42, bell(note(74), 1.3), .2)

# small room: decaying noise as the impulse response
ir_t = t_(1.2); ir = rng.standard_normal(len(ir_t)) * np.exp(-ir_t * 5); ir[0] = 6
wet = fftconvolve(out, ir)[:len(out)]
mix = out + wet * .06
fade = np.ones_like(mix); n = int(.4 * SR); fade[-n:] = np.linspace(1, 0, n)
mix *= fade
mix /= np.max(np.abs(mix)) / 0.84
wavfile.write(sys.argv[1], SR, (mix * 32767).astype(np.int16))
print("wrote", sys.argv[1], f"{len(mix) / SR:.2f} s")
