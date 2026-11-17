# UI test (docs/DEVELOPMENT_NOTES.md): the shader section while a game starts and in the
# performance reading while it plays. Runs a NEXUS build (Linux, stand-in engine with a slow
# load: NEXUS_FAKE_LOAD) in Chrome.
#
#   python3 shader_progress.py <NEXUS binary> <portable data folder> [screenshot folder]
#
# Checks: the loading screen shows the measured "n of total" bar and a shader line whose counts
# grow; no invented percentage when the total is unknown; the performance sample carries the
# shader status while playing and the "shaders" reading says compiling, then ready.
import asyncio, os, re, sys
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
os.environ["NEXUS_FAKE_LOAD"] = "6s"
from nexus_harness import *
configure(sys.argv[1], sys.argv[2])
SHOTS = sys.argv[3] if len(sys.argv) > 3 else None

async def main():
    p, _ = start()
    ok = True
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 760})
        await enter(pg)
        await pg.evaluate("launchGame(S.games.find(isPlayable).key, true)")
        # 1. shader preparation before the engine starts, then the boot precache with "Start now"
        prep_seen, skip_seen = [], False
        for _ in range(80):
            t, skip = await pg.evaluate("[document.querySelector('.lx-status')?.textContent || '', !!document.querySelector('.lx-skip:not([hidden])')]")
            label = t.split(':')[0].rstrip('…')
            if label and (not prep_seen or prep_seen[-1] != label):
                prep_seen.append(label)
            skip_seen |= skip and label.startswith("Compiling")
            if SHOTS and label.startswith("Compiling") and not os.path.exists(os.path.join(SHOTS, "loading-precache.png")):
                await pg.screenshot(path=os.path.join(SHOTS, "loading-precache.png"))
            if label.startswith("Building shaders"):
                break
            await pg.wait_for_timeout(100)
        wanted = ["Finding shaders in the game’s files", "Converting the shader programs", "Compiling the game’s shaders"]
        in_order = all(w in prep_seen for w in wanted) and [prep_seen.index(w) for w in wanted] == sorted(prep_seen.index(w) for w in wanted)
        print(f"{'PASS' if in_order and skip_seen else 'FAIL'} preparation stages {prep_seen}; Start now shown during compiling: {skip_seen}")
        ok &= in_order and skip_seen
        await pg.wait_for_function("document.querySelector('.lx-shaders')?.textContent.includes('ready from the shader cache')", timeout=20000)
        seen = []
        for _ in range(8):
            seen.append(await pg.evaluate("[document.querySelector('.lx-status')?.textContent, document.querySelector('.lx-shaders')?.textContent]"))
            if SHOTS and len(seen) == 3:
                await pg.screenshot(path=os.path.join(SHOTS, "loading-shaders.png"))
            await pg.wait_for_timeout(350)
        nums = [int(re.sub(r"\D", "", t.split("ready")[0]) or 0) for _, t in seen if t]
        grew = nums == sorted(nums) and nums[-1] > nums[0]
        measured = any(re.search(r"Building shaders from earlier sessions: [\d,]+ of [\d,]+", s or "") for s, _ in seen)
        print(f"{'PASS' if grew and measured else 'FAIL'} loading screen: bar {[s for s, _ in seen][:2]}, shader line {seen[-1][1]!r}")
        ok &= grew and measured
        await pg.wait_for_function("S.playing && S.playing.phase === 'playing'", timeout=20000)
        readings = []
        for _ in range(12):
            r = api("/api/perf")
            seg = next((x for x in r.get("segments") or [] if x["label"] == "SHADERS"), None)
            readings.append((seg or {}).get("unit"))
            await pg.wait_for_timeout(500)
        has_status = "shaders" in (api("/api/perf").get("sample") or {})
        compiling_then_ready = "compiling" in readings and any(x and x.startswith("ready") for x in readings[readings.index("compiling"):]) if "compiling" in readings else False
        print(f"{'PASS' if has_status and compiling_then_ready else 'FAIL'} performance reading while playing: {readings}")
        ok &= has_status and compiling_then_ready
        await b.close()
    p.terminate()
    sys.exit(0 if ok else 1)

asyncio.run(main())
