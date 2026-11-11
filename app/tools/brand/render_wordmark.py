"""Renders the NEXUS startup wordmarks ("NEXUS" and "SWITCH") with the Outfit typeface
(SIL Open Font License 1.1, tools/brand/fonts/OFL-Outfit.txt) into the exact canvases and
boxes the startup animation uses, so layout, timing and tinting are unchanged:

  web/assets/brand/word-nexus.png   482 x 76   white on transparent, 6 px margin
  web/assets/brand/word-switch.png  680 x 126  white on transparent, 6 px margin
  web/assets/brand/logo-full.png    680 x 466  emblem (unchanged) + both words at their boot positions

usage: python3 render_wordmark.py <path to web/assets/brand>
"""
import os, sys
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
FONT = os.path.join(HERE, "fonts", "Outfit-VF.ttf")
SS = 4  # supersampling


def font(px, weight):
    f = ImageFont.truetype(FONT, px)
    f.set_variation_by_axes([weight])
    return f


def render_word(text, size, weight, tracking):
    """text centred in size (w,h) with a 6 px margin, scaled to fill the box; tracking in em."""
    w, h = size
    bw, bh = (w - 12) * SS, (h - 12) * SS
    probe = 400
    f = font(probe, weight)
    # measure glyph by glyph so letter spacing is exact
    adv = [f.getlength(ch) for ch in text]
    gap = tracking * probe
    total_w = sum(adv) + gap * (len(text) - 1)
    asc_box = f.getbbox("".join(text), anchor="ls")  # cap height area relative to baseline
    cap_top, cap_bot = asc_box[1], asc_box[3]
    total_h = cap_bot - cap_top
    scale = min(bw / total_w, bh / total_h)
    px = max(8, int(probe * scale))
    f = font(px, weight)
    adv = [f.getlength(ch) for ch in text]
    gap = tracking * px
    tw = sum(adv) + gap * (len(text) - 1)
    bb = f.getbbox(text, anchor="ls")
    th = bb[3] - bb[1]
    img = Image.new("L", (w * SS, h * SS), 0)
    d = ImageDraw.Draw(img)
    x = (w * SS - tw) / 2
    base = (h * SS - th) / 2 - bb[1]
    for ch, a in zip(text, adv):
        d.text((x, base), ch, font=f, fill=255, anchor="ls")
        x += a + gap
    img = img.resize((w, h), Image.LANCZOS)
    out = Image.new("RGBA", (w, h), (255, 255, 255, 0))
    out.putalpha(img)
    return out


def main(brand):
    nexus = render_word("NEXUS", (482, 76), 600, 0.62)
    switch = render_word("SWITCH", (680, 126), 800, 0.02)
    nexus.save(os.path.join(brand, "word-nexus.png"), optimize=True)
    switch.save(os.path.join(brand, "word-switch.png"), optimize=True)
    # logo-full: keep the emblem rows, place the words where the boot animation places them
    full = Image.open(os.path.join(brand, "logo-full.png")).convert("RGBA")
    W, H = full.size
    emblem_bottom = round(H * 0.55579)  # #boot-logo .em height
    out = Image.new("RGBA", (W, H), (255, 255, 255, 0))
    out.paste(full.crop((0, 0, W, emblem_bottom)), (0, 0))
    out.alpha_composite(nexus, (round(W * 0.14559), round(H * 0.57511)))
    out.alpha_composite(switch, (0, round(H * 0.72961)))
    out.save(os.path.join(brand, "logo-full.png"), optimize=True)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else os.path.join(HERE, "..", "..", "web", "assets", "brand"))
