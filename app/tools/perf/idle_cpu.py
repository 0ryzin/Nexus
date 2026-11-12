# Developer tool (docs/PERFORMANCE.md): measures the NEXUS interface with the stand-in engine.
# Needs Playwright and Chrome; paths (NEXUS binary, a portable data folder, NEXUS_HARDWARE file) are arguments or set below.
import asyncio, sys, subprocess, time, os, shutil, urllib.request
from playwright.async_api import async_playwright
sys.argv += []
BIN=sys.argv[1]; D=sys.argv[2]
subprocess.run(["pkill","-x","NEXUS"]); time.sleep(.5); shutil.copy(BIN, D+"/NEXUS")
env=dict(os.environ, NEXUS_PORT="8773", NEXUS_DEV_NOAUTH="1", NEXUS_FAKE_ENGINE="1", NEXUS_HARDWARE="/tmp/hw-rtx.json")
p=subprocess.Popen([D+"/NEXUS"], cwd=D, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL); time.sleep(1.2)
async def main():
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width":1440,"height":860})
        cdp = await pg.context.new_cdp_session(pg); await cdp.send("Performance.enable")
        await pg.goto("http://localhost:8773/"); await pg.wait_for_timeout(500); await pg.keyboard.press("Space"); await pg.wait_for_timeout(2500)
        if await pg.evaluate("S.screen") == 'who':
            await pg.evaluate("document.querySelector('.who-item').click()"); await pg.wait_for_timeout(900)
        for name, js in [("home","goHome()"),("library","showLibrary()"),("settings","showSettings('appearance')")]:
            await pg.evaluate("() => { void (" + js + ") }"); await pg.wait_for_timeout(1500)
            m0 = {x['name']:x['value'] for x in (await cdp.send("Performance.getMetrics"))['metrics']}
            await pg.wait_for_timeout(5000)
            m1 = {x['name']:x['value'] for x in (await cdp.send("Performance.getMetrics"))['metrics']}
            d = lambda k: m1[k]-m0[k]
            print(name, f"task {d('TaskDuration')/5*100:.1f}% of a core, script {d('ScriptDuration')*1000/5:.1f} ms/s, layouts {d('LayoutCount')/5:.1f}/s, recalc {d('RecalcStyleCount')/5:.1f}/s")
        await b.close()
asyncio.run(main()); p.terminate()
