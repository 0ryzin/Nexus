# Run: python3 make_steam_art.py <path to dist-base/Resources/Icons>  (needs rsvg-convert, Pillow, the Inter fonts); copy the PNGs to dist-base/Resources/Steam
# NEXUS Steam artwork, 0.9.2 (owner's choice): NEXUS's own Joy-Con emblem (Resources/Icons/
# emblem.png) with the NEXUS wordmark, on the dark background with the red glow.
import subprocess, sys
from PIL import Image, ImageFilter
ICONS = sys.argv[1]
RED="#E8132B"; BG1="#15161A"; BG2="#0B0B0D"
DEFS=f'''<defs>
<radialGradient id="bg" cx="50%" cy="100%" r="90%"><stop offset="0" stop-color="#5A0A14"/><stop offset="0.55" stop-color="{BG1}"/><stop offset="1" stop-color="{BG2}"/></radialGradient>
<radialGradient id="glow" cx="50%" cy="50%" r="50%"><stop offset="0" stop-color="{RED}" stop-opacity="0.38"/><stop offset="1" stop-color="{RED}" stop-opacity="0"/></radialGradient></defs>'''
def word(cx,y,size,spacing=0.42):
    uw=size*1.35
    return (f'<text x="{cx}" y="{y}" font-family="Inter Display" font-weight="700" font-size="{size}" fill="#F4F4F6" text-anchor="middle" letter-spacing="{size*spacing}">NEXUS</text>'
            f'<rect x="{cx-uw/2+size*spacing/2}" y="{y+size*0.22}" width="{uw}" height="{size*0.07}" rx="{size*0.035}" fill="{RED}"/>')
def sub(cx,y,size):
    return f'<text x="{cx}" y="{y}" font-family="Inter" font-weight="500" font-size="{size}" fill="#A9AAB2" text-anchor="middle" letter-spacing="{size*0.25}">GAME LAUNCHER</text>'
def render(name,w,h,body,bg=True,glows=()):
    g="".join(f'<circle cx="{x}" cy="{y}" r="{r}" fill="url(#glow)"/>' for x,y,r in glows)
    b=f'<rect width="{w}" height="{h}" fill="url(#bg)"/>' if bg else ''
    open(name+".svg","w").write(f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{DEFS}{b}{g}{body}</svg>')
    subprocess.run(["rsvg-convert","-w",str(w),"-h",str(h),name+".svg","-o",name+".png"],check=True)
    return Image.open(name+".png").convert("RGBA")
emb=Image.open(f"{ICONS}/emblem.png").convert("RGBA").crop(Image.open(f"{ICONS}/emblem.png").getbbox())
def paste(im,cx,cy,height):
    e=emb.resize((round(emb.width*height/emb.height),height),Image.LANCZOS)
    # soft shadow for depth
    sh=Image.new("RGBA",e.size,(0,0,0,0)); sh.putalpha(e.getchannel("A").point(lambda a:a*0.55))
    sh=sh.filter(ImageFilter.GaussianBlur(height/30))
    im.alpha_composite(sh,(round(cx-e.width/2+height/40),round(cy-e.height/2+height/30)))
    im.alpha_composite(e,(round(cx-e.width/2),round(cy-e.height/2)))
    return im
out={}
out["capsule"]=paste(render("capsule",600,900,word(300,640,92)+sub(300,742,22),glows=[(300,330,230)]),300,330,250)
out["wide"]=paste(render("wide",920,430,word(620,245,84)+sub(620,330,18),glows=[(240,215,170)]),240,215,190)
out["hero"]=paste(render("hero",1920,620,"",glows=[(1500,310,300)]),1500,310,300)
out["logo"]=paste(render("logo",1280,720,word(770,405,118),bg=False),220,360,250)
icon=Image.open(f"{ICONS}/app-icon-512.png").convert("RGBA").resize((256,256),Image.LANCZOS)
out["icon"]=icon
for k,v in out.items():
    v.save(k+".png")
print("ok")
