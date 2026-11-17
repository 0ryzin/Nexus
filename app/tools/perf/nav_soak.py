# Developer tool (docs/PERFORMANCE.md): does the interface grow when it is used for a while?
# Opens Home, All Software, Settings, a game's Options menu and Media many times, then compares
# the JS heap, DOM nodes and event listeners after a forced garbage collection. Linux, headless
# Chrome, stand-in engine: compare builds with it, not Windows figures.
#
#   python3 nav_soak.py <NEXUS binary> <portable data folder> [rounds]
import asyncio, json, os, sys
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from nexus_harness import *

configure(sys.argv[1], sys.argv[2])
ROUNDS = int(sys.argv[3]) if len(sys.argv) > 3 else 60
STEPS = ["goHome()", "showLibrary()", "showSettings('appearance')", "showSettings('engine')",
         "gameMenu(S.games.find(isPlayable).key)", "Nav.top().close()", "Media.show()", "goHome()"]

async def snapshot(pg, cdp):
    await cdp.send("HeapProfiler.collectGarbage"); await pg.wait_for_timeout(300)
    await cdp.send("HeapProfiler.collectGarbage")
    m = {x['name']: x['value'] for x in (await cdp.send("Performance.getMetrics"))['metrics']}
    return {"js_heap_mb": round(m['JSHeapUsedSize'] / 2**20, 2), "dom_nodes": int(m['Nodes']),
            "listeners": int(m['JSEventListeners']), "documents": int(m['Documents'])}

async def main():
    p, _ = start()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1440, "height": 860})
        cdp = await pg.context.new_cdp_session(pg); await cdp.send("Performance.enable")
        await enter(pg)
        for js in STEPS:  # warm up: every screen built once
            await pg.evaluate("() => { void (" + js + ") }"); await pg.wait_for_timeout(250)
        before = await snapshot(pg, cdp)
        for _ in range(ROUNDS):
            for js in STEPS:
                await pg.evaluate("() => { void (" + js + ") }"); await pg.wait_for_timeout(60)
        await pg.wait_for_timeout(1000)
        after = await snapshot(pg, cdp)
        print(json.dumps({"rounds": ROUNDS, "steps_per_round": len(STEPS), "before": before, "after": after,
                          "server_rss_mb": round(rss_mb(p.pid), 1)}, indent=1))
        await b.close()
    p.terminate()

asyncio.run(main())
