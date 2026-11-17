# UI test (CI): the regrouped System Settings. Every category opens without script errors, the
# group is shown above each panel, search finds the words from the refinement brief and opens
# the setting (also inside an "Advanced" section), and "Changed" follows a setting's value.
#   python3 settings_search.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'perf'))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

ROW_NAMES = "[...document.querySelectorAll('#set-panel .row')].map(r=>r.querySelector('.rl > span')?.firstChild?.textContent?.trim())"

async def main():
    p, _ = H.start()
    errs = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1440, "height": 900})
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await H.enter(pg)
        await pg.evaluate("showSettings()"); await pg.wait_for_timeout(600)
        check(await pg.evaluate("curCat") == 'general', "Settings open on General")
        groups = await pg.evaluate("[...document.querySelectorAll('#set-cats .cat-group')].map(g=>g.textContent)")
        check(groups[:4] == ['General', 'Personalisation', 'Games', 'Controllers'] and 'About' in groups, f"grouped categories {groups}")
        cats = await pg.evaluate("catList().filter(c=>c[0]!=='-'&&c[0]!=='controllers').map(c=>c[0])")
        for c in cats:
            await pg.evaluate(f"selectCat('{c}', false)"); await pg.wait_for_timeout(350)
            n = await pg.evaluate("document.querySelectorAll('#set-panel h2').length")
            check(n >= 1, f"category {c} shows its panel")
        await pg.evaluate("selectCat('shaders', false)"); await pg.wait_for_timeout(500)
        check(await pg.evaluate("document.querySelector('#set-panel .set-crumb')?.textContent") == 'Graphics & performance', "group shown above the panel")
        check('Prepare shaders before playing' in await pg.evaluate(ROW_NAMES), "Shader management has the preparation switch")
        # old category ids still work
        await pg.evaluate("showSettings('startup')"); await pg.wait_for_timeout(500)
        check(await pg.evaluate("curCat") == 'general', "the old Startup link opens General")
        # search
        want = {"Controller": "Controller Home button", "GitHub": "Connect to GitHub", "Shader cache": "Share shaders with other NEXUS users on this PC",
                "FPS": "FPS", "Background": "Home screen background", "Graphics": "Graphics backend"}
        for q, name in want.items():
            await pg.fill("#set-search", q); await pg.wait_for_timeout(2500 if q == "Controller" else 500)
            got = await pg.evaluate(ROW_NAMES)
            check(name in got[:8], f"search {q!r} finds {name!r} (top results {got[:4]})")
        await pg.fill("#set-search", "fade into"); await pg.wait_for_timeout(500)
        await pg.evaluate("document.querySelector('#set-panel .row').click()"); await pg.wait_for_timeout(800)
        r = await pg.evaluate("[curCat, Nav.cur?.querySelector('.rl > span')?.firstChild?.textContent, !Nav.cur?.closest('.adv-body')?.hidden]")
        check(r == ['emulator', 'Fade into the game', True], f"a result inside Advanced opens it and focuses the setting {r}")
        await pg.fill("#set-search", "resolution"); await pg.wait_for_timeout(500)
        await pg.evaluate("[...document.querySelectorAll('#set-panel .row')].find(r=>r.textContent.startsWith('Resolution') && r.textContent.includes('Graphics & emulation')).click()"); await pg.wait_for_timeout(1200)
        check(await pg.evaluate("curCat + ':' + emuSection") == 'emusettings:graphics', "an engine setting opens its section of Graphics & emulation")
        # changed markers
        await pg.evaluate("selectCat('general', false)"); await pg.wait_for_timeout(500)
        before = await pg.evaluate("document.querySelector('#set-panel [data-setting=\"sound.enabled\"]').classList.contains('changed')")
        await pg.evaluate("document.querySelector('#set-panel [data-setting=\"sound.enabled\"]').click()"); await pg.wait_for_timeout(800)
        after = await pg.evaluate("document.querySelector('#set-panel [data-setting=\"sound.enabled\"]').classList.contains('changed')")
        await pg.evaluate("document.querySelector('#set-panel [data-setting=\"sound.enabled\"]').click()"); await pg.wait_for_timeout(800)
        back = await pg.evaluate("document.querySelector('#set-panel [data-setting=\"sound.enabled\"]').classList.contains('changed')")
        check((before, after, back) == (False, True, False), f"Changed follows the value {before, after, back}")
        check(H.api("/api/state")["settings"]["sound"]["enabled"] is True, "the setting is back to its value")
        # account: not connected is said plainly
        await pg.evaluate("selectCat('account', false)"); await pg.wait_for_timeout(800)
        check('Connect to GitHub' in await pg.evaluate(ROW_NAMES), "account panel offers Connect to GitHub")
        await b.close()
    p.terminate()
    check(not errs, f"no script errors {errs}")
    sys.exit(1 if fails else 0)

asyncio.run(main())
