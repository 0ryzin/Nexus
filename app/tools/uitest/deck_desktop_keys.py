# UI test (CI): Steam Deck Desktop Mode without a controller: the keys Steam's Desktop
# configuration sends stand for the buttons. Lone Ctrl/Alt (L1/R1) switch tabs, Space (Y) opens
# the options of the focused item, Enter/Escape work, and a Ctrl+key shortcut is not a tab switch.
#   python3 deck_desktop_keys.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

async def main():
    p, _ = H.start()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await H.enter(pg)
        await pg.evaluate("S.platform = { ...(S.platform || {}), steamDeck: true, gamingMode: false }")
        await pg.evaluate("showSettings()"); await pg.wait_for_timeout(600)
        sel = "document.querySelector('#set-cats .sel, .cat.sel')?.textContent"
        first = await pg.evaluate(sel)
        await pg.keyboard.press("Alt"); await pg.wait_for_timeout(300)
        second = await pg.evaluate(sel)
        check(second != first, f"R1 (lone Alt) moves to the next tab: {first} -> {second}")
        await pg.keyboard.press("Control"); await pg.wait_for_timeout(300)
        check(await pg.evaluate(sel) == first, "L1 (lone Ctrl) moves back")
        await pg.keyboard.down("Control"); await pg.keyboard.press("s"); await pg.keyboard.up("Control"); await pg.wait_for_timeout(300)
        check(await pg.evaluate(sel) == first, "Ctrl+S is not a tab switch")
        toasts = await pg.evaluate("[...document.querySelectorAll('.toast')].map(t => t.textContent).join(' | ')")
        check("Desktop Mode" in toasts, "the one-time Desktop Mode tip appears")
        await pg.evaluate("goHome()"); await pg.wait_for_timeout(600)
        await pg.evaluate("Nav.setFocus(document.querySelector('#home [data-nav].tile, .tile[data-nav]'))")
        await pg.keyboard.press(" "); await pg.wait_for_timeout(500)
        check(await pg.evaluate("!!Nav.top()?.dialog || !!document.querySelector('.ctx-menu, .dialog')"), "Space (Y) opens the focused game's options")
        await pg.keyboard.press("Escape"); await pg.wait_for_timeout(400)
        check(await pg.evaluate("!Nav.top()?.dialog"), "Escape (B) closes it")
        await b.close()
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
