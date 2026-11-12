# Developer tool (docs/PERFORMANCE.md): measures the NEXUS interface with the stand-in engine.
# Needs Playwright and Chrome; paths (NEXUS binary, a portable data folder, NEXUS_HARDWARE file) are arguments or set below.
import asyncio, subprocess, time, os, sys, json, urllib.request, statistics, shutil
from playwright.async_api import async_playwright
BIN=sys.argv[1]; LABEL=sys.argv[2]
D=os.environ.get("PD","/tmp/perfdir")
def start():
    subprocess.run(["pkill","-x","NEXUS"]); time.sleep(0.6)
    shutil.copy(BIN, D+"/NEXUS")
    env=dict(os.environ, NEXUS_PORT="8773", NEXUS_DEV_NOAUTH="1", NEXUS_FAKE_ENGINE="1", NEXUS_FAKE_PLAY="600s", NEXUS_HARDWARE="/tmp/hw-rtx.json")
    t0=time.perf_counter()
    p=subprocess.Popen([D+"/NEXUS"], cwd=D, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    while True:
        try: urllib.request.urlopen("http://localhost:8773/api/state", timeout=0.2).read(); break
        except Exception: time.sleep(0.005)
    ms=(time.perf_counter()-t0)*1000
    rq=urllib.request.Request("http://localhost:8773/api/settings", data=b'{"startup":{"enabled":false}}', headers={"Content-Type":"application/json","X-Nexus":"1"}, method="POST")
    urllib.request.urlopen(rq).read()
    return p, ms
def rss(pid):
    for l in open(f"/proc/{pid}/status"):
        if l.startswith("VmRSS"): return int(l.split()[1])/1024
async def main():
    res={"server_ready_ms":[], "page_ready_ms":[], "api_state_ms":[], "nav_ms":{}, "rss_mb":[], "js_heap_mb":[]}
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        for i in range(5):
            p, ms = start(); res["server_ready_ms"].append(ms)
            pg = await b.new_page(viewport={"width":1440,"height":860})
            t=time.perf_counter()
            await pg.goto("http://localhost:8773/")
            await pg.wait_for_function("typeof S!=='undefined' && !!S.screen && S.games && S.games.length>0", timeout=20000)
            res["page_ready_ms"].append((time.perf_counter()-t)*1000)
            await pg.keyboard.press("Space"); await pg.wait_for_timeout(1200)
            if await pg.evaluate("S.screen") == 'who':
                await pg.evaluate("document.querySelector('.who-item').click()"); await pg.wait_for_timeout(900)
            await pg.evaluate("S.profile && themePatch({animations:'full'})")
            for name, js in [("home","goHome()"),("library","showLibrary()"),("settings","showSettings('appearance')"),("settings-engine","selectCat('engine',true)"),("media","Media.show()"),("home2","goHome()")]:
                await pg.keyboard.press("Escape"); await pg.wait_for_timeout(300)
                ms2 = await pg.evaluate("async () => { const t=performance.now(); await (" + js + "); await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))); return performance.now()-t }")
                res["nav_ms"].setdefault(name,[]).append(ms2)
            a = await pg.evaluate("async()=>{let s=0;for(let i=0;i<20;i++){const t=performance.now();await fetch('/api/state');s+=performance.now()-t}return s/20}")
            res["api_state_ms"].append(a)
            res["js_heap_mb"].append(await pg.evaluate("performance.memory ? performance.memory.usedJSHeapSize/1048576 : 0"))
            res["rss_mb"].append(rss(p.pid))
            await pg.close(); p.terminate()
        await b.close()
    med=lambda v: round(statistics.median(v),1)
    out={k:(med(v) if isinstance(v,list) else {n:med(x) for n,x in v.items()}) for k,v in res.items()}
    print(LABEL, json.dumps(out))
asyncio.run(main())
