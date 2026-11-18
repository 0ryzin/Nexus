# UI test (CI): the Home game menu with a controller, and the "OOT Launch" startup animation.
# Runs a NEXUS build (Linux, stand-in engine) in Chrome; the controller is scripted through the
# Gamepad API (Pad.poll -> Nav), so it uses the same path as a real pad.
#   python3 oot_menu.py <NEXUS binary> <portable data folder>
import asyncio, json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
A, B, X, Y, DOWN, UP = 0, 1, 2, 3, 13, 12
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

MENU = "[...document.querySelectorAll('.layer .game-menu .gm-l')].map(e => e.firstChild.textContent)"

async def press(pg, b, wait=350):
    await pg.evaluate(f"window.__press({b})"); await pg.wait_for_timeout(wait)

async def close_layers(pg):
    for _ in range(3):
        if await pg.evaluate("Nav.layers.some(l => l.dialog)"): await pg.keyboard.press("Escape"); await pg.wait_for_timeout(250)

async def main():
    p, _ = H.start()
    errs = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        ctx = await b.new_context(viewport={"width": 1440, "height": 900})
        await ctx.add_init_script(H.FAKE_PAD)
        pg = await ctx.new_page()
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await H.enter(pg)
        await pg.evaluate("goHome()"); await pg.wait_for_timeout(600)

        # ---- the right-click menu, for reference
        tile = "$$('#tiles .tile').find(t => t._game && isPlayable(t._game))"
        await pg.evaluate(f"Nav.setFocus({tile})")
        await pg.evaluate(f"{tile}.dispatchEvent(new MouseEvent('contextmenu', {{ bubbles: true, cancelable: true }}))"); await pg.wait_for_timeout(400)
        mouse_menu = await pg.evaluate(MENU)
        check(len(mouse_menu) > 3, f"right-click opens the game menu {mouse_menu}")
        await close_layers(pg)

        # ---- controller: X on the focused game opens the same menu
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(400)
        await pg.evaluate(f"Nav.setFocus({tile})"); await pg.wait_for_timeout(200)
        hint = await pg.evaluate("document.querySelector('#hint-list').textContent")
        check("Game menu" in hint, f"the hint bar shows the button for the game menu ({hint!r})")
        await press(pg, X)
        pad_menu = await pg.evaluate(MENU)
        check(pad_menu == mouse_menu, f"controller X opens the same menu as right-click {pad_menu}")
        check(await pg.evaluate("S.screen") == "home", "X on a game does not open the Library any more")
        # navigate and select with the controller: down to Game Settings, A
        first = await pg.evaluate("Nav.cur.querySelector('.gm-l')?.firstChild.textContent")
        target = "Game Settings"
        for _ in range(8):
            if await pg.evaluate("Nav.cur.querySelector('.gm-l')?.firstChild.textContent") == target: break
            await press(pg, DOWN, 250)
        on = await pg.evaluate("Nav.cur.querySelector('.gm-l')?.firstChild.textContent")
        await press(pg, A, 900)
        check(on == target and await pg.evaluate("Nav.top()?.name") == "gameconfig", f"controller moves in the menu (from {first!r}) and A opens {target}")
        await press(pg, B, 400)
        await close_layers(pg)

        # ---- Options (Y) opens it too; B closes it and focus returns to the game
        await pg.evaluate(f"Nav.setFocus({tile})")
        await press(pg, Y)
        check(await pg.evaluate(MENU) == mouse_menu, "the Options button opens the same menu")
        await press(pg, B)
        check(not await pg.evaluate("Nav.layers.some(l => l.dialog)") and await pg.evaluate("!!Nav.cur?._game"), "B closes the menu and focus returns to the game")

        # ---- keyboard X, unchanged
        await pg.evaluate(f"Nav.setFocus({tile})"); await pg.keyboard.press("x"); await pg.wait_for_timeout(400)
        check(await pg.evaluate(MENU) == mouse_menu, "keyboard X still opens the menu")
        await close_layers(pg)

        # ---- not on a game: X keeps opening the Library search
        await pg.evaluate("Nav.setFocus($('#dock .dock-btn'))"); await pg.wait_for_timeout(200)
        await press(pg, X, 700)
        check(await pg.evaluate("S.screen") == "library", "X elsewhere on Home still opens the Library search")
        await pg.evaluate("OSK.close(false)"); await pg.evaluate("goHome()"); await pg.wait_for_timeout(600)

        # ---- Cards layout: the details panel's Play button
        await pg.evaluate("homePatch({ layout: 'cards' })"); await pg.wait_for_timeout(700)
        await pg.evaluate("Nav.setFocus($('#home-details .hd-play'))"); await pg.wait_for_timeout(200)
        await press(pg, X)
        check(len(await pg.evaluate(MENU)) > 3, "X on the Cards details panel opens the selected game's menu")
        await close_layers(pg)
        await pg.evaluate("homePatch({ layout: 'classic' })"); await pg.wait_for_timeout(400)

        # ---- OOT Launch: choose it in Settings (controller), it is saved
        await pg.evaluate("saveSettings({ startup: { enabled: true } })")
        await pg.evaluate("showSettings('general')"); await pg.wait_for_timeout(700)
        await pg.evaluate("Nav.setFocus([...document.querySelectorAll('#set-panel .row')].find(r => r.textContent.startsWith('Startup animation')))")
        await press(pg, A, 500)  # the list
        for _ in range(3):
            if await pg.evaluate("Nav.cur.textContent.trim().startsWith('OOT Launch')"): break
            await press(pg, DOWN, 250)
        await press(pg, A, 800)
        s = H.api("/api/state")["settings"]["startup"]
        check(s["animation"] == "oot", f"OOT Launch chosen with the controller is saved ({s['animation']})")
        rows = await pg.evaluate("[...document.querySelectorAll('#set-panel .row')].map(r => r.querySelector('.rl > span')?.firstChild?.textContent)")
        check("Length" not in rows and "Startup sound" in rows, "settings that do not apply to OOT Launch are hidden; sound stays")
        # an unknown value is refused
        H.api("/api/settings", {"startup": {"animation": "something"}})
        check(H.api("/api/state")["settings"]["startup"]["animation"] == "nexus", "an unknown animation falls back to the standard one")
        H.api("/api/settings", {"startup": {"animation": "oot"}})
        await ctx.close()

        # ---- restart: OOT Launch plays at startup, then Home
        p.terminate(); p.wait()
        p, _ = H.start()
        H.api("/api/settings", {"startup": {"enabled": True}})  # H.start switches it off for other tests
        ctx = await b.new_context(viewport={"width": 1280, "height": 720})
        pg = await ctx.new_page()
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await pg.goto(f"http://localhost:{H.PORT}/")
        await pg.wait_for_function("document.querySelector('#boot.oot')", timeout=10000)
        t0 = await pg.evaluate("performance.now()")
        await pg.wait_for_timeout(900)
        state = await pg.evaluate("({ sound: !!Sound.buf.oot_launch, pixel: getComputedStyle(document.querySelector('#boot .oot-pixel')).opacity })")
        check(state["sound"] and float(state["pixel"]) > .5, f"after a restart OOT Launch plays (its sound is loaded, the pixel emblem shows) {state}")
        await pg.wait_for_timeout(3300)
        crest = await pg.evaluate("document.querySelector('#boot .oot-crest')?.classList.contains('full')")
        check(crest, "the crest is complete at about 4 s")
        await pg.wait_for_function("!document.querySelector('#boot')", timeout=8000)
        bg = await pg.evaluate("document.body.classList.contains('oot-wp') && getComputedStyle(document.querySelector('#wallpaper .wp-img')).backgroundImage.startsWith('url(\"data:image/jpeg')")
        check(bg, "Home keeps the OOT Launch gold background behind it")
        took = (await pg.evaluate("performance.now()") - t0) / 1000
        await pg.wait_for_timeout(500)
        check(await pg.evaluate("S.screen") in ("home", "who") and 6.0 < took + .2 < 8.0, f"it ends in Home after about 6.7 s ({took + .2:.1f} s)")
        # skipping
        await pg.goto(f"http://localhost:{H.PORT}/")
        await pg.wait_for_function("document.querySelector('#boot.oot')", timeout=10000)
        await pg.wait_for_timeout(1500); await pg.keyboard.press("Space")
        await pg.wait_for_function("!document.querySelector('#boot')", timeout=3000)
        check(True, "any key skips it")
        await ctx.close(); await b.close()
    H.api("/api/settings", {"startup": {"animation": "nexus", "enabled": False}})
    p.terminate()
    check(not errs, f"no script errors {errs}")
    sys.exit(1 if fails else 0)

asyncio.run(main())
