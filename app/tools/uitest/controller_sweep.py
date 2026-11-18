# UI test (CI): every Home shortcut and every System Settings category with a controller only.
# For each one: reach it with the D-pad, open it with A, check that the focus lands on a visible
# item of the screen that opened, walk it with the D-pad (the focus must move, stay visible and
# never escape to a screen underneath), then B goes back without leaving the focus lost.
#   python3 controller_sweep.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

A, B, UP, DOWN, LEFT, RIGHT = 0, 1, 12, 13, 14, 15
FOCUS = """(() => { const c = Nav.cur, t = Nav.top(); if (!c) return { ok: false, why: 'no focus' };
  const r = c.getBoundingClientRect();
  return { ok: !!t && t.root.contains(c) && c.offsetParent !== null && r.width > 0 && r.height > 0,
           inView: r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth,
           layers: Nav.layers.length, name: t?.name || '', text: (c.getAttribute('aria-label') || c.textContent || '').trim().slice(0, 40) }; })()"""

async def press(pg, b, n=1):
    for _ in range(n):
        await pg.evaluate(f"window.__press({b}, 70)")
    await pg.wait_for_timeout(250)

async def walk(pg, label, steps=12):
    """D-pad down through a screen: the focus moves, stays visible and inside the top layer."""
    seen, bad = set(), []
    for d in [DOWN] * steps + [UP] * 3:
        await press(pg, d)
        f = await pg.evaluate(FOCUS)
        if not f.get("ok") or not f.get("inView"): bad.append(f)
        seen.add(await pg.evaluate("Nav.cur ? [...document.querySelectorAll('[data-nav]')].indexOf(Nav.cur) : -1"))
    check(not bad, f"{label}: focus visible and inside the screen while walking ({bad[:1]})")
    return len(seen)

async def main():
    p, _ = H.start()
    errs = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})  # the Deck's screen
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await pg.add_init_script(H.FAKE_PAD)
        await H.enter(pg)
        await pg.evaluate("window.__connectPad()")
        await pg.wait_for_timeout(400)
        await press(pg, DOWN)
        labels = await pg.evaluate("[...document.querySelectorAll('#dock .dock-btn')].map(b => b.getAttribute('aria-label'))")
        check(labels[:2] == ["Library", "Users"] and "Personalisation" in labels and not any(x in labels for x in ["Shaders", "Performance", "Graphics & Emulation", "Themes", "Home screen"]),
              f"Home shortcuts {labels}")
        for i, label in enumerate(labels):
            # reach the shortcut with the D-pad: down to the dock, then left as far as it goes, then right i times
            await pg.evaluate("goHome()"); await pg.wait_for_timeout(500)
            for _ in range(6):
                if await pg.evaluate("!!Nav.cur?.closest('#dock')"): break
                await press(pg, DOWN)
            await press(pg, LEFT, len(labels))
            await press(pg, RIGHT, i)
            got = await pg.evaluate("Nav.cur?.getAttribute('aria-label')")
            check(got == label, f"D-pad reaches '{label}' (focus on '{got}')")
            if got != label: continue
            layers0 = await pg.evaluate("Nav.layers.length")
            await press(pg, A); await pg.wait_for_timeout(600)
            f = await pg.evaluate(FOCUS)
            check(f.get("ok") and f.get("inView"), f"'{label}' opens with the focus on a visible item ({f})")
            await walk(pg, label)
            for _ in range(4):  # B until back on Home
                if await pg.evaluate("S.screen === 'home' && Nav.layers.length === " + str(layers0)): break
                await press(pg, B); await pg.wait_for_timeout(300)
            back = await pg.evaluate("S.screen === 'home' && !!Nav.cur && Nav.top().root.contains(Nav.cur)")
            check(back, f"B from '{label}' returns to Home with a focus")
        # every settings category: the list on the left, Right into the panel, walk, B back to the list
        await pg.evaluate("showSettings()"); await pg.wait_for_timeout(600)
        cats = await pg.evaluate("[...document.querySelectorAll('#set-cats .cat')].map(c => c._cat)")
        await press(pg, UP, len(cats) + 2)
        check(await pg.evaluate("!!Nav.cur?.matches('input')"), "Up past the first category reaches the settings search")
        await press(pg, DOWN)
        for c in cats:
            if c == 'controllers':  # opens the Controllers screen (checked above), no panel
                await press(pg, DOWN); continue
            cur = await pg.evaluate("Nav.cur?._cat")
            if cur != c:
                check(False, f"D-pad down the category list reaches '{c}' (on '{cur}')"); break
            await press(pg, RIGHT); await pg.wait_for_timeout(350)
            inPanel = await pg.evaluate("!!Nav.cur && document.querySelector('#set-panel').contains(Nav.cur)")
            n = await walk(pg, f"settings/{c}", 8) if inPanel else 0
            check(inPanel, f"settings/{c}: Right goes into the panel")
            await press(pg, B)
            back = await pg.evaluate("Nav.cur?._cat")
            check(back == c, f"settings/{c}: B returns to its category (on '{back}')")
            await press(pg, DOWN)
        check(not errs, f"no script errors {errs[:2]}")
        await b.close()
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
