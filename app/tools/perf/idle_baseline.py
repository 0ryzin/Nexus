# Developer tool (docs/PERFORMANCE.md): idle cost of the NEXUS interface and server.
# Runs a NEXUS build (Linux, stand-in engine) against a portable data folder and drives it with
# Chrome through Playwright. It measures this machine and headless Chrome, not a Windows PC with
# WebView2: compare builds with it, do not quote its numbers as Windows numbers.
#
#   python3 idle_baseline.py <NEXUS binary> <portable data folder> <label> [seconds]
#
# Per screen, over <seconds> of doing nothing: the page's main-thread busy time (% of a core),
# script time, layouts and style recalculations per second, JS heap; the NEXUS server process's
# CPU and resident memory. "home+pad" is Home with a (scripted) controller connected, which
# makes the interface poll for input.
import asyncio, json, os, sys, time
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from nexus_harness import *
import nexus_harness

BIN, D, LABEL = sys.argv[1], sys.argv[2], sys.argv[3]
SECS = float(sys.argv[4]) if len(sys.argv) > 4 else 10
configure(BIN, D)

async def main():
    out = {"label": LABEL, "seconds": SECS, "server_ready_ms": [], "page_ready_ms": [], "screens": {}}
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        for i in range(3):  # start-up: server ready, then interface loaded with its games
            p, ms = start(); out["server_ready_ms"].append(round(ms, 1))
            pg = await b.new_page(viewport={"width": 1440, "height": 860})
            t = time.perf_counter()
            await pg.goto(f"http://localhost:{PORT}/")
            await pg.wait_for_function("typeof S!=='undefined' && !!S.screen && (S.screen === 'who' || (S.games && S.games.length>0))", timeout=20000)
            out["page_ready_ms"].append(round((time.perf_counter() - t) * 1000, 1))
            await pg.close(); p.terminate(); p.wait()
        p, _ = start()
        ctx = await b.new_context(viewport={"width": 1440, "height": 860})
        await ctx.add_init_script(FAKE_PAD)
        pg = await ctx.new_page()
        cdp = await pg.context.new_cdp_session(pg); await cdp.send("Performance.enable")
        await enter(pg)
        for name, js in [("home", "goHome()"), ("home+pad", "(window.__connectPad(), goHome())"),
                         ("library+pad", "showLibrary()"), ("settings+pad", "showSettings('appearance')"),
                         ("home (pad removed)", "(window.__disconnectPad(), goHome())"),
                         # the Home layouts (web/home.js), each with its panel and widgets
                         ("home cards", "homePatch({ layout: 'cards', gameBackground: true, widgets: { details: true, recent: true, playtime: true } }).then(goHome)"),
                         ("home grid", "homePatch({ layout: 'grid', widgets: { details: true, recent: true, playtime: true, storage: true } }).then(goHome)"),
                         ("home grid flat", "homePatch({ layout: 'grid', effects: 'flat' }).then(goHome)"),
                         ("home classic", "homePatch({ layout: 'classic', effects: 'glass' }).then(goHome)"),
                         ("game in front", "launchGame(S.games.find(isPlayable).key, true)")]:
            await pg.evaluate("() => { void (" + js + ") }"); await pg.wait_for_timeout(2500)
            if name == "game in front":  # the interface stays behind a running game (Windows)
                await pg.wait_for_function("S.screen === 'playing'", timeout=30000); await pg.wait_for_timeout(1500)
                print("animations running behind the game:", await pg.evaluate(
                    "document.getAnimations().filter(a => a.playState === 'running').map(a => (a.effect?.target?.id || a.effect?.target?.className || '?') + ' ' + (a.animationName || a.transitionProperty || ''))"), file=sys.stderr)
            m0 = {x['name']: x['value'] for x in (await cdp.send("Performance.getMetrics"))['metrics']}
            c0 = cpu_ticks(p.pid); t0 = time.perf_counter()
            await pg.wait_for_timeout(SECS * 1000)
            m1 = {x['name']: x['value'] for x in (await cdp.send("Performance.getMetrics"))['metrics']}
            dt = time.perf_counter() - t0; c1 = cpu_ticks(p.pid)
            d = lambda k: (m1[k] - m0[k]) / dt
            out["screens"][name] = {
                "page_busy_pct_core": round(d('TaskDuration') * 100, 2),
                "script_ms_per_s": round(d('ScriptDuration') * 1000, 2),
                "layouts_per_s": round(d('LayoutCount'), 2),
                "style_recalcs_per_s": round(d('RecalcStyleCount'), 2),
                "js_heap_mb": round(m1['JSHeapUsedSize'] / 2**20, 1),
                "server_cpu_pct": round((c1 - c0) / HZ / dt * 100, 2),
                "server_rss_mb": round(rss_mb(p.pid), 1),
            }
        await b.close()
    p.terminate()
    print(json.dumps(out, indent=1))

asyncio.run(main())
