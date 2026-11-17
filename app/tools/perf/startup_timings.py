# Developer tool (docs/PERFORMANCE.md, "Startup"): NEXUS's startup milestones, measured.
#   python3 startup_timings.py <NEXUS binary> <portable data folder> <label> [runs]
# Starts the build (Linux, stand-in engine) once per run and opens the interface in Chrome as
# soon as the server answers, the way the NEXUS window does. Reports, per startup-animation
# setting (off / on, the default), the median of:
#   server        process start -> local server answers
#   ui_ready      process start -> the interface has its data       ("interface ready")
#   library       process start -> the game list is available        ("library available")
#   home          process start -> Home is shown and accepts input   ("home interactive")
#   account       process start -> the account is restored           ("account restored")
# On Windows, WebView2 starting (0.5-1.5 s on the user's PC) comes before the interface; that
# part is measured from nexus.log, not here.
import asyncio, json, os, statistics, subprocess, sys, time
sys.path.insert(0, os.path.dirname(__file__))
import nexus_harness as H
from playwright.async_api import async_playwright

BIN, DATA, LABEL = sys.argv[1], sys.argv[2], sys.argv[3]
RUNS = int(sys.argv[4]) if len(sys.argv) > 4 else 7
H.configure(BIN, DATA)

def launch():
    subprocess.run(["pkill", "-x", "NEXUS"], stderr=subprocess.DEVNULL)
    for _ in range(100):
        try: H.api("/api/state", timeout=0.2); time.sleep(0.05)
        except Exception: break
    import shutil; shutil.copy(BIN, DATA + "/NEXUS")
    env = dict(os.environ, NEXUS_PORT=H.PORT, NEXUS_DEV_NOAUTH="1", NEXUS_FAKE_ENGINE="1",
               NEXUS_HARDWARE=os.environ.get("NEXUS_HARDWARE", "/tmp/hw-rtx.json"))
    t0 = time.perf_counter()
    p = subprocess.Popen([DATA + "/NEXUS"], cwd=DATA, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    while True:
        try: H.api("/api/state", timeout=0.5); break
        except Exception: time.sleep(0.003)
    return p, t0, (time.perf_counter() - t0) * 1000

async def main():
    # one-time setup: a user who is picked automatically, so Home is what opens
    p, _, _ = launch()
    profs = H.api("/api/profiles") or [H.api("/api/profiles", {"name": "Tester"})]
    H.api(f"/api/profiles/{profs[0]['id']}/select", {})
    H.api("/api/settings", {"askProfileOnStart": False})
    p.terminate(); p.wait()
    out = {}
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        for anim in (False, True):
            res = {k: [] for k in ("server", "ui_ready", "library", "home", "account")}
            p, _, _ = launch(); H.api("/api/settings", {"startup": {"enabled": anim}}); p.terminate(); p.wait()
            for i in range(RUNS):
                ctx = await b.new_context(viewport={"width": 1440, "height": 860})
                pg = await ctx.new_page()
                p, t0, server = launch()
                nav = (time.perf_counter() - t0) * 1000  # page opens now
                await pg.goto(f"http://localhost:{H.PORT}/")
                await pg.wait_for_function("typeof startupMarks!=='undefined' && startupMarks['home interactive']!=null", timeout=30000)
                js = await pg.evaluate("startupMarks")
                srv = {m["name"]: m["ms"] for m in H.api("/api/startup")}
                res["server"].append(server)
                res["ui_ready"].append(nav + js["interface ready"])
                res["library"].append(nav + js.get("library available", js["home interactive"]))
                res["home"].append(nav + js["home interactive"])
                res["account"].append(srv.get("account restored", float("nan")))
                await ctx.close(); p.terminate(); p.wait()
            out["animation " + ("on" if anim else "off")] = {k: round(statistics.median(v)) for k, v in res.items()}
        await b.close()
    print(LABEL, json.dumps(out))

asyncio.run(main())
