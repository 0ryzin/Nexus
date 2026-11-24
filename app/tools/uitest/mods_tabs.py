# UI test (CI): a game's Mods sheet has Installed / Browse GameBanana / Import tabs, reachable with
# the controller; Browse lists GameBanana's mods (answers stubbed: no network in CI) and a mod
# opens its files, with only ZIP files offered for install.
#   python3 mods_tabs.py <NEXUS binary> <portable data folder>
import asyncio, json, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

A, RIGHT, DOWN = 0, 15, 13

async def main():
    p, _ = H.start(); print("started", flush=True)
    errs = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await pg.route("**/api/games/*/gamebanana?*", lambda r: r.fulfill(status=200, content_type="application/json", body=json.dumps(
            {"gameId": 7, "gameName": "Alpha Quest", "complete": True, "mods": [{"id": 100, "name": "Better Text", "author": "modder", "likes": 3}]})))
        await pg.route("**/api/gamebanana/mod/100", lambda r: r.fulfill(status=200, content_type="application/json", body=json.dumps(
            {"id": 100, "name": "Better Text", "author": "modder", "description": "Nice", "files": [
                {"id": 1, "name": "better.zip", "size": 1000, "zip": True}, {"id": 3, "name": "mod.7z", "size": 10, "zip": False}]})))
        await pg.add_init_script(H.FAKE_PAD)
        await H.enter(pg); print("entered", flush=True)
        await pg.evaluate("window.__connectPad()")
        key = await pg.evaluate("S.games[0].key")
        await pg.evaluate(f"ModsUI.open({json.dumps(key)})"); await pg.wait_for_timeout(800)
        tabs = await pg.evaluate("[...document.querySelectorAll('.mods-tabs button')].map(b => b.textContent)")
        check(tabs == ["Installed", "Browse GameBanana", "Import"], f"tabs {tabs}")
        # controller: to the tab row, right to Browse, A
        await pg.evaluate("(() => { Nav.setFocus(document.querySelector('.mods-tabs button')); return 1; })()")
        await pg.evaluate(f"window.__press({RIGHT}, 70)"); await pg.wait_for_timeout(200)
        await pg.evaluate(f"window.__press({A}, 70)"); await pg.wait_for_timeout(1200)
        rows = await pg.evaluate("[...document.querySelectorAll('.sheet-body .row')].map(r => r.textContent)")
        check(any("Better Text" in r for r in rows), f"Browse lists GameBanana's mods ({rows[:3]})")
        await pg.evaluate("(() => { ModsUI.gbMod(S.games[0].key, 100, () => {}); return 1; })()"); await pg.wait_for_timeout(600)
        btns = await pg.evaluate("[...document.querySelectorAll('.dialog .d-actions button')].map(b => b.textContent)")
        check("Install better.zip" in btns and not any("7z" in x for x in btns), f"only ZIP files offered ({btns})")
        check(not errs, f"no page errors ({errs[:2]})")
        await b.close()
    p.terminate()
    print("ALL PASSED" if not fails else f"{len(fails)} FAILED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
