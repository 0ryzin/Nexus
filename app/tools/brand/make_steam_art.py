#!/usr/bin/env python3
"""Steam library artwork for NEXUS as a non-Steam game, from NEXUS's own wordmark.

The artwork uses only the NEXUS wordmark (no console imagery). The list icon is NEXUS's own app
icon file, copied unchanged.

Writes into dist-base/Resources/Steam/ (shipped with NEXUS; Settings › Steam can copy them into
Steam's grid folder for the NEXUS shortcut, with the user's permission):
  capsule.png  600x900   library grid (portrait)
  wide.png     920x430   recent games / wide capsule
  hero.png     1920x620  library page header
  logo.png     1280x720  logo over the header (transparent)
  icon.png     256x256   list icon
"""
import os
from PIL import Image, ImageDraw, ImageFilter

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
OUT = os.path.join(REPO, "dist-base", "Resources", "Steam")
ICON = os.path.join(REPO, "web", "assets", "brand", "app-icon-512.png")
WORD = os.path.join(REPO, "web", "assets", "brand", "word-nexus.png")
BG1, BG2, ACCENT = (29, 30, 33), (16, 18, 20), (230, 0, 18)


def backdrop(w, h):
    im = Image.new("RGB", (w, h), BG2)
    d = ImageDraw.Draw(im)
    for y in range(h):  # vertical gradient
        t = y / max(1, h - 1)
        d.line([(0, y), (w, y)], fill=tuple(int(BG1[i] * (1 - t) + BG2[i] * t) for i in range(3)))
    glow = Image.new("L", (w, h), 0)
    ImageDraw.Draw(glow).ellipse([w * 0.15, h * 0.55, w * 0.85, h * 1.4], fill=90)
    glow = glow.filter(ImageFilter.GaussianBlur(min(w, h) / 6))
    red = Image.new("RGB", (w, h), ACCENT)
    return Image.composite(red, im, glow)


def place(base, img, box_w, cx, cy):
    img = img.crop(img.getbbox())
    r = box_w / img.width
    img = img.resize((int(img.width * r), int(img.height * r)), Image.LANCZOS)
    base.alpha_composite(img, (int(cx - img.width / 2), int(cy - img.height / 2)))


def main():
    os.makedirs(OUT, exist_ok=True)
    icon, word = Image.open(ICON).convert("RGBA"), Image.open(WORD).convert("RGBA")
    cap = backdrop(600, 900).convert("RGBA")
    place(cap, word, 440, 300, 450)
    ImageDraw.Draw(cap).rectangle([230, 500, 370, 506], fill=ACCENT + (255,))
    cap.convert("RGB").save(os.path.join(OUT, "capsule.png"), optimize=True)
    wide = backdrop(920, 430).convert("RGBA")
    place(wide, word, 620, 460, 200)
    ImageDraw.Draw(wide).rectangle([380, 252, 540, 258], fill=ACCENT + (255,))
    wide.convert("RGB").save(os.path.join(OUT, "wide.png"), optimize=True)
    hero = backdrop(1920, 620).convert("RGB")
    hero.save(os.path.join(OUT, "hero.png"), optimize=True)
    logo = Image.new("RGBA", (1280, 720), (0, 0, 0, 0))
    place(logo, word, 1000, 640, 360)
    logo.save(os.path.join(OUT, "logo.png"), optimize=True)
    icon.resize((256, 256), Image.LANCZOS).save(os.path.join(OUT, "icon.png"), optimize=True)
    print("Steam artwork written to", OUT)


if __name__ == "__main__":
    main()
