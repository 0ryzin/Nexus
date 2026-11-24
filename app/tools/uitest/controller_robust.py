# UI test (CI): controller input keeps working in the cases behind "the D-pad can't navigate"
# (Tom's PC, 0.8.1-0.9.2):
#  1. a mistake while handling one press must not stop the menus reading the controller
#  2. a controller without the standard mapping reports the D-pad as a hat axis: it navigates
#  3. what the controller sends is written to the log (for diagnosis from nexus.log)
#   python3 controller_robust.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

DOWN, RIGHT = 13, 15
CUR = "Nav.cur ? [...document.querySelectorAll('[data-nav]')].indexOf(Nav.cur) : -1"

async def main():
    p, _ = H.start()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await pg.add_init_script(H.FAKE_PAD)
        await H.enter(pg)
        await pg.evaluate("window.__connectPad()")
        await pg.wait_for_timeout(400)

        # 0. XInput assist (Windows): the browser never reported a D-pad press, NEXUS sends one:
        #    the focus moves once; after the browser reports the D-pad, the assist is ignored
        await pg.evaluate("goHome()"); await pg.wait_for_timeout(400)
        await pg.evaluate("(() => { Nav.setFocus(document.querySelector('#tiles .tile')); return 1; })()")
        f0 = await pg.evaluate(CUR)
        await pg.evaluate("XAssist.event({right: true})"); await pg.wait_for_timeout(150)
        await pg.evaluate("XAssist.event({right: false})"); await pg.wait_for_timeout(100)
        f1 = await pg.evaluate(CUR)
        check(f1 != f0 and await pg.evaluate("XAssist.on"), f"XInput assist moves the focus when the browser reports no D-pad ({f0} -> {f1})")
        await pg.evaluate(f"window.__press({RIGHT}, 70)"); await pg.wait_for_timeout(200)
        f2 = await pg.evaluate(CUR)
        await pg.evaluate("XAssist.event({right: true})"); await pg.wait_for_timeout(150)
        await pg.evaluate("XAssist.event({right: false})"); await pg.wait_for_timeout(100)
        f3 = await pg.evaluate(CUR)
        check(f2 != f1 and f3 == f2 and await pg.evaluate("XAssist.browserDpad && !XAssist.on"), f"once the browser reports the D-pad the assist does nothing ({f1} -> {f2} -> {f3})")

        # 1. one press throws: the next presses still navigate
        await pg.evaluate("(() => { window.__origMove = Nav.move; let once = true; Nav.move = function (d) { if (once) { once = false; throw new Error('test: one bad press'); } return window.__origMove.call(this, d); }; return 1; })()")
        await pg.evaluate(f"window.__press({DOWN}, 70)"); await pg.wait_for_timeout(200)
        before = await pg.evaluate(CUR)
        await pg.evaluate(f"window.__press({RIGHT}, 70)"); await pg.wait_for_timeout(250)
        after = await pg.evaluate(CUR)
        check(before != after and after >= 0, f"after an error in one press the D-pad still moves the focus ({before} -> {after})")
        await pg.evaluate("(() => { Nav.move = window.__origMove; return 1; })()")

        # 2. a controller with no standard mapping: D-pad as a hat on axis 9
        await pg.evaluate("""(() => {
          const p = { index: 0, id: 'Generic USB Joystick (Vendor: 0079 Product: 0006)', mapping: '', connected: true,
            buttons: Array.from({ length: 12 }, () => ({ pressed: false, value: 0 })), axes: [0, 0, 0, 0, 0, 0, 0, 0, 0, 3.2857] };
          window.__pads = [p]; window.dispatchEvent(new Event('gamepadconnected'));
          window.__hat = async (v, ms = 90) => { p.axes[9] = v; await new Promise(r => setTimeout(r, ms)); p.axes[9] = 3.2857; await new Promise(r => setTimeout(r, ms)); };
        })()""")
        await pg.wait_for_timeout(500)
        await pg.evaluate("goHome()"); await pg.wait_for_timeout(500)
        seen = set()
        for v in [1/7, 1/7, -3/7, -3/7, 5/7]:  # down, down, right, right, left
            await pg.evaluate(f"window.__hat({v})"); await pg.wait_for_timeout(200)
            seen.add(await pg.evaluate(CUR))
        check(len(seen) >= 3, f"a hat D-pad (no standard mapping) moves the focus ({sorted(seen)})")

        # 3. the log names the controller and its presses
        await pg.wait_for_timeout(500)
        log = open(os.path.join(H.D, "Logs", "nexus.log"), encoding="utf-8", errors="replace").read() if os.path.exists(os.path.join(H.D, "Logs", "nexus.log")) else ""
        check('ui: controller "Generic USB Joystick' in log and "mapping=none" in log, "the log names the controller and its mapping")
        check("ui: controller input axis 9" in log, "the log records the controller's first presses")
        check("ui: controller input: test: one bad press" in log, "the error in a press is logged")
        await b.close()
    p.terminate()
    print("ALL PASSED" if not fails else f"{len(fails)} FAILED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
