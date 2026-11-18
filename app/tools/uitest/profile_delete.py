# UI test (CI): deleting a user, as reported on a Steam Deck ("MiNS": typed the name, pressed
# Enter, nothing was deleted). Checks, with the real interface and server:
#   1. keyboard: "Delete this user", type the name in other capitals ("mins"), Enter -> deleted,
#      the list refreshes at once, and it is still gone after NEXUS restarts;
#   2. controller only: A on "Delete this user", the name in the field, D-pad to Delete, A ->
#      deleted; B in the dialog cancels and keeps the user;
#   3. a wrong name says so and keeps the user. The other user and its data are untouched.
#   python3 profile_delete.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

A, B, DOWN, RIGHT = 0, 1, 13, 15
names = lambda: [p["name"] for p in H.api("/api/profiles")]

async def press(pg, b, n=1):
    for _ in range(n):
        await pg.evaluate(f"window.__press({b}, 70)")
    await pg.wait_for_timeout(200)

async def open_delete(pg, name, pad):
    pid = await pg.evaluate(f"S.profiles.find(p => p.name === {name!r}).id")
    await pg.evaluate(f"editProfile({pid!r})"); await pg.wait_for_timeout(500)
    if pad:
        for _ in range(30):
            if "Delete this user" in (await pg.evaluate("Nav.cur?.textContent || ''")): break
            await press(pg, DOWN)
        check("Delete this user" in await pg.evaluate("Nav.cur?.textContent || ''"), f"{name}: D-pad reaches Delete this user")
        await press(pg, A)
    else:
        await pg.evaluate("[...document.querySelectorAll('.sheet .row')].find(r => r.textContent.includes('Delete this user')).click()")
    await pg.wait_for_timeout(400)
    return await pg.evaluate("!!document.querySelector('.dialog input')")

async def main():
    p, _ = H.start()
    for n in ["MiNS", "Temp", "Keep"]:
        if n not in names(): H.api("/api/profiles", {"name": n})
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await pg.add_init_script(H.FAKE_PAD)
        await H.enter(pg)
        # 3. wrong name
        check(await open_delete(pg, "Keep", False), "the delete dialog asks for the name")
        await pg.keyboard.type("nope"); await pg.keyboard.press("Enter"); await pg.wait_for_timeout(500)
        msg = await pg.evaluate("document.querySelector('.dialog')?.textContent || ''")
        check("did not match" in msg and "Keep" in names(), f"a wrong name is reported and keeps the user ({msg[:60]!r})")
        await pg.keyboard.press("Escape"); await pg.wait_for_timeout(300)
        await pg.evaluate("Nav.top()?.onBack?.()"); await pg.wait_for_timeout(300)
        # 1. keyboard, other capitals, Enter
        await open_delete(pg, "MiNS", False)
        await pg.keyboard.type("mins"); await pg.keyboard.press("Enter")
        for _ in range(30):
            if "MiNS" not in names(): break
            await pg.wait_for_timeout(200)
        check("MiNS" not in names(), f"typing 'mins' and Enter deletes MiNS ({names()})")
        await pg.wait_for_timeout(500)
        shown = await pg.evaluate("S.profiles.map(p => p.name)")
        check("MiNS" not in shown, f"the interface's list refreshes at once {shown}")
        # 2. controller: B cancels, then A on Delete deletes
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(300)
        await press(pg, DOWN)
        await open_delete(pg, "Temp", True)
        await press(pg, B); await pg.wait_for_timeout(300)
        check("Temp" in names(), "B in the dialog cancels")
        await pg.evaluate("Nav.top()?.onBack?.()"); await pg.wait_for_timeout(300)
        await open_delete(pg, "Temp", True)
        await pg.evaluate("(() => { const i = document.querySelector('.dialog input'); i.value = 'Temp'; })()")
        for _ in range(3):
            if await pg.evaluate("!!Nav.cur?.closest('.d-actions')"): break
            await press(pg, DOWN)
        for _ in range(3):
            if (await pg.evaluate("Nav.cur?.textContent")) == "Delete": break
            await press(pg, RIGHT)
        check((await pg.evaluate("Nav.cur?.textContent")) == "Delete", "D-pad reaches the Delete button")
        await press(pg, A)
        for _ in range(30):
            if "Temp" not in names(): break
            await pg.wait_for_timeout(200)
        check("Temp" not in names(), f"A on Delete deletes with a controller ({names()})")
        await b.close()
    p.terminate(); p.wait()
    p, _ = H.start()
    after = names()
    check("MiNS" not in after and "Temp" not in after and "Keep" in after, f"after a restart: deleted users stay gone, others kept {after}")
    archived = os.listdir(os.path.join(sys.argv[2], "System", "Archived")) if os.path.isdir(os.path.join(sys.argv[2], "System", "Archived")) else []
    check(any(a.startswith("MiNS-") for a in archived), f"MiNS's data is archived, not erased {archived}")
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
