import subprocess, os
RED="#E8132B"; RED2="#9E0B1C"; BG1="#15161A"; BG2="#0B0B0D"

def emblem(cx, cy, s, glow=True):
    # a stylised N of three strokes joining four nodes: "nexus", a meeting point
    w=s*0.62; h=s*0.78; x0=cx-w/2; x1=cx+w/2; y0=cy-h/2; y1=cy+h/2
    sw=s*0.11; r=s*0.085
    g=[]
    if glow:
        g.append(f'<circle cx="{cx}" cy="{cy}" r="{s*0.75}" fill="url(#glow)"/>')
    g.append(f'<g stroke="#F4F4F6" stroke-width="{sw}" stroke-linecap="round" fill="none">'
             f'<line x1="{x0}" y1="{y1}" x2="{x0}" y2="{y0}"/>'
             f'<line x1="{x1}" y1="{y1}" x2="{x1}" y2="{y0}"/></g>')
    g.append(f'<line x1="{x0}" y1="{y0}" x2="{x1}" y2="{y1}" stroke="url(#diag)" stroke-width="{sw}" stroke-linecap="round"/>')
    for (x,y,c) in [(x0,y1,"#F4F4F6"),(x1,y0,"#F4F4F6"),(x0,y0,RED),(x1,y1,RED)]:
        g.append(f'<circle cx="{x}" cy="{y}" r="{r}" fill="{c}" stroke="{BG1}" stroke-width="{s*0.03}"/>')
    g.append(f'<circle cx="{cx}" cy="{cy}" r="{r*0.8}" fill="#FFFFFF"/>')
    return "\n".join(g)

def defs(w,h):
    return f'''<defs>
<radialGradient id="bg" cx="50%" cy="100%" r="90%"><stop offset="0" stop-color="#5A0A14"/><stop offset="0.55" stop-color="{BG1}"/><stop offset="1" stop-color="{BG2}"/></radialGradient>
<radialGradient id="glow" cx="50%" cy="50%" r="50%"><stop offset="0" stop-color="{RED}" stop-opacity="0.35"/><stop offset="1" stop-color="{RED}" stop-opacity="0"/></radialGradient>
<linearGradient id="diag" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{RED}"/><stop offset="1" stop-color="#FF4A5C"/></linearGradient>
</defs>'''

def word(cx, y, size, color="#F4F4F6", spacing=0.42, anchor="middle", underline=True):
    t=f'<text x="{cx}" y="{y}" font-family="Inter Display" font-weight="700" font-size="{size}" fill="{color}" text-anchor="{anchor}" letter-spacing="{size*spacing}">NEXUS</text>'
    if underline:
        uw=size*1.35
        t+=f'<rect x="{cx-uw/2+size*spacing/2}" y="{y+size*0.22}" width="{uw}" height="{size*0.07}" rx="{size*0.035}" fill="{RED}"/>'
    return t

def svg(w,h,body,bg=True):
    b=f'<rect width="{w}" height="{h}" fill="url(#bg)"/>' if bg else ''
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{defs(w,h)}{b}{body}</svg>'

def sub(cx, y, size):
    return f'<text x="{cx}" y="{y}" font-family="Inter" font-weight="500" font-size="{size}" fill="#A9AAB2" text-anchor="middle" letter-spacing="{size*0.25}">GAME LAUNCHER</text>'

out={
 "capsule": svg(600,900, emblem(300,330,260)+word(300+0,640,92)+sub(300,742,22)),
 "wide": svg(920,430, emblem(250,215,190)+word(620,245,84)+sub(620,330,18)),
 "hero": svg(1920,620, emblem(1500,310,330,glow=True), ),
 "logo": svg(1280,720, emblem(210,360,250,glow=False)+word(760,405,118,anchor="middle"), bg=False),
 "icon": None,
}
icon=f'''<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">{defs(512,512)}
<rect x="16" y="16" width="480" height="480" rx="110" fill="url(#bg)"/>{emblem(256,256,330,glow=True)}</svg>'''
out["icon"]=icon
sizes={"capsule":(600,900),"wide":(920,430),"hero":(1920,620),"logo":(1280,720),"icon":(256,256)}
for k,v in out.items():
    open(k+".svg","w").write(v)
    w,h=sizes[k]
    subprocess.run(["rsvg-convert","-w",str(w),"-h",str(h),k+".svg","-o",k+".png"],check=True)
print("ok")
