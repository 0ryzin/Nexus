# UI test (CI): the Mii Creator with a controller only. Settings › Users › a user › Miis…:
# New Mii (male base), change the hairstyle and hair colour with D-pad left/right, name it with
# the dialog, save; a second Mii; make it the default; the engine's Mii database written at a
# game's start holds both, default first (checked by NEXUS's own reader through the API); delete.
#   python3 mii_creator.py <NEXUS binary> <portable data folder>
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

async def press(pg, b, n=1):
    for _ in range(n):
        await pg.evaluate(f"window.__press({b}, 60)")
    await pg.wait_for_timeout(150)

async def focus_to(pg, text, key=DOWN, limit=60):
    for _ in range(limit):
        if text in (await pg.evaluate("Nav.cur?.textContent || ''")): return True
        await press(pg, key)
    return False

async def new_mii(pg, name, hair_steps):
    await pg.evaluate("[...document.querySelectorAll('.sheet button')].find(b => b.textContent === 'New Mii…').click()")
    await pg.wait_for_timeout(400)
    await press(pg, A)  # the dialog's primary: Male base
    await pg.wait_for_timeout(500)
    check(await pg.evaluate("!!document.querySelector('.mii-preview svg')"), f"{name}: the editor opens with a drawing")
    check(await focus_to(pg, "Hairstyle"), f"{name}: D-pad reaches Hairstyle")
    before = await pg.evaluate("Nav.cur.textContent")
    await press(pg, RIGHT, hair_steps)
    check(await pg.evaluate("Nav.cur.textContent") != before, f"{name}: right changes the hairstyle")
    await press(pg, DOWN)
    await press(pg, RIGHT, 2)  # hair colour
    check(await focus_to(pg, "Name", UP), f"{name}: back up to Name")
    await press(pg, A); await pg.wait_for_timeout(300)
    await pg.evaluate(f"(i => {{ i.value = {name!r}; }})(document.querySelector('.dialog input'))")
    await pg.evaluate("[...document.querySelectorAll('.dialog button')].find(b => b.textContent === 'OK').click()")
    await pg.wait_for_timeout(400)
    check(await focus_to(pg, "Save Mii"), f"{name}: D-pad reaches Save Mii")
    await press(pg, A); await pg.wait_for_timeout(700)

async def main():
    p, _ = H.start()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await pg.add_init_script(H.FAKE_PAD)
        await H.enter(pg)
        await pg.evaluate("window.__connectPad()")
        pid = await pg.evaluate("S.profile.id")
        await pg.evaluate(f"MiiUI.page({pid!r})"); await pg.wait_for_timeout(600)
        await new_mii(pg, "Tom", 3)
        await new_mii(pg, "Racer", 5)
        c = H.api(f"/api/profiles/{pid}/miis")["collection"]
        names = [m["name"] for m in c["miis"]]
        check(names == ["Tom", "Racer"], f"both saved {names}")
        tom = next(m for m in c["miis"] if m["name"] == "Tom")
        check(tom["fields"]["hairType"] == 36 and tom["fields"]["hairColor"] != 1, f"Tom's hair from the D-pad: style {tom['fields']['hairType']}, colour {tom['fields']['hairColor']}")
        check(c["default"] == tom["id"], "the first Mii is the default")
        # make Racer the default with the controller
        cards = await pg.evaluate("[...document.querySelectorAll('.mii-card')].map(c => c.textContent)")
        check(len(cards) == 2 and "Default" in cards[0], f"cards {cards}")
        await pg.evaluate("Nav.setFocus([...document.querySelectorAll('.mii-card')][1])")
        await press(pg, A); await pg.wait_for_timeout(300)
        await pg.evaluate("[...document.querySelectorAll('.dialog button')].find(b => b.textContent === 'Make default').click()")
        await pg.wait_for_timeout(600)
        c = H.api(f"/api/profiles/{pid}/miis")["collection"]
        racer = next(m for m in c["miis"] if m["name"] == "Racer")
        check(c["default"] == racer["id"], "Racer is now the default")
        # delete Tom (confirmation)
        await pg.evaluate("Nav.setFocus([...document.querySelectorAll('.mii-card')].find(c => c.textContent.includes('Tom')))")
        await press(pg, A); await pg.wait_for_timeout(300)
        await pg.evaluate("[...document.querySelectorAll('.dialog button')].find(b => b.textContent === 'Delete…').click()")
        await pg.wait_for_timeout(300)
        await pg.evaluate("[...document.querySelectorAll('.dialog button')].find(b => b.textContent === 'Delete').click()")
        await pg.wait_for_timeout(600)
        c = H.api(f"/api/profiles/{pid}/miis")["collection"]
        check([m["name"] for m in c["miis"]] == ["Racer"], "Tom deleted after confirming")
        errs = await pg.evaluate("window.__errors || []")
        await b.close()
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
