# UI test (CI): restoring defaults, keybinds, the on-screen keyboard and searching a game's own
# settings (usability update, 17 Nov 2026). Runs a NEXUS build (Linux, stand-in engine) and
# drives the real interface in Chrome: a scripted controller through the Gamepad API (the same
# path as a real pad: Pad.poll -> Nav), the mouse and the keyboard through Playwright.
#   python3 resets_osk.py <NEXUS binary> <portable data folder>
import asyncio, json, os, subprocess, sys, time
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
DATA = sys.argv[2]
A, B, X, Y, LB, RB, VIEW, START, UP, DOWN, LEFT, RIGHT = 0, 1, 2, 3, 4, 5, 8, 9, 12, 13, 14, 15
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

def settings(): return H.api("/api/state")["settings"]
def profile(pid): return next(p for p in H.api("/api/profiles") if p["id"] == pid)

async def press(pg, *buttons, wait=220):
    for b in buttons:
        await pg.evaluate(f"window.__press({b})")
        await pg.wait_for_timeout(wait)

ROW = "[...document.querySelectorAll('#set-panel .row')].find(r => r.querySelector('.rl > span')?.firstChild?.textContent?.trim() === {name})"

async def row_state(pg, name):
    return await pg.evaluate("(() => { const r = " + ROW.format(name=json.dumps(name)) + "; if (!r) return null; const b = r.querySelector('.rst'); return { reset: !!b && !b.classList.contains('off'), changed: r.classList.contains('changed'), value: r.querySelector('.rv')?.textContent?.trim() || '', on: r.querySelector('.toggle')?.classList.contains('on') ?? null }; })()")

async def open_cat(pg, cat):
    await pg.evaluate(f"selectCat('{cat}', false)"); await pg.wait_for_timeout(500)

def make_user_data():
    """Things a reset must never touch: saves, the account / GitHub sign-in, engine files."""
    sysd = os.path.join(DATA, "System")
    files = {
        os.path.join(sysd, "Account", "session.bin"): b"stand-in sign-in (not a real token)",
        os.path.join(sysd, "UserData", "Saves", "test-save", "save.dat"): b"save data",
        os.path.join(sysd, "Engine", "nand", "test.bin"): b"engine file",
    }
    for p, b in files.items():
        os.makedirs(os.path.dirname(p), exist_ok=True)
        open(p, "wb").write(b)
    return files

async def main():
    p, _ = H.start()
    pid = H.api("/api/profiles")[0]["id"]
    errs = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        ctx = await b.new_context(viewport={"width": 1440, "height": 900})
        await ctx.add_init_script(H.FAKE_PAD)
        pg = await ctx.new_page()
        pg.on("pageerror", lambda e: errs.append(str(e)))
        await H.enter(pg)
        await pg.evaluate("showSettings('general')"); await pg.wait_for_timeout(700)

        # ---- 1. one setting, with the mouse
        await pg.evaluate(ROW.format(name='"Interface sounds"') + ".click()"); await pg.wait_for_timeout(600)
        st = await row_state(pg, "Interface sounds")
        check(st["reset"] and st["changed"] and settings()["sound"]["enabled"] is False, f"a changed setting shows its restore button {st}")
        await pg.mouse.move(5, 5)
        await pg.evaluate(ROW.format(name='"Interface sounds"') + ".scrollIntoView({block: 'center'})"); await pg.wait_for_timeout(300)
        box = await pg.evaluate("(() => { const b = " + ROW.format(name='"Interface sounds"') + ".querySelector('.rst').getBoundingClientRect(); return [b.x + b.width/2, b.y + b.height/2]; })()")
        await pg.mouse.click(*box); await pg.wait_for_timeout(900)
        st = await row_state(pg, "Interface sounds")
        check(settings()["sound"]["enabled"] is True and st["on"] and not st["reset"], f"mouse: restore button restores, saves and updates the row {st}")

        # ---- 1b. one setting, with the controller (Options on the focused setting)
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(400)
        await pg.evaluate("saveSettings({ emulator: { fadeIn: false } })"); await open_cat(pg, "emulator")
        await pg.evaluate("document.querySelector('#set-panel .adv-head').click()"); await pg.wait_for_timeout(300)
        await pg.evaluate("Nav.setFocus(" + ROW.format(name='"Fade into the game"') + ")")
        hint = await pg.evaluate("document.querySelector('#hint-list').textContent")
        check("Reset to default" in hint, f"the hint bar offers Reset to default on a changed setting ({hint!r})")
        await press(pg, Y, wait=900)
        check(settings()["emulator"]["fadeIn"] is True, "controller: Options restores the focused setting")

        # ---- 1c. a setting of the user (appearance) and a controller button
        await pg.evaluate(f"patchProfile('{pid}', {{ theme: {{ mode: 'light' }} }})"); await open_cat(pg, "appearance")
        await pg.evaluate("(" + ROW.format(name='"Theme"') + ").querySelector('.rst').click()"); await pg.wait_for_timeout(800)
        check(profile(pid)["theme"]["mode"] == "dark" and not await pg.evaluate("document.body.classList.contains('theme-light')"), "a user's setting (Theme) is restored and applied at once")

        # ---- 3. keybinds: one, a clash, all
        await pg.evaluate("saveSettings({ emulator: { homeKey: 'F9' } })"); await open_cat(pg, "emulator")
        await pg.evaluate("(" + ROW.format(name='"Home key"') + ").querySelector('.rst').click()"); await pg.wait_for_timeout(800)
        check(settings()["emulator"]["homeKey"] == "Shift+H", "one keybind (Home key) is restored")
        await pg.evaluate("saveSettings({ emulator: { homeKey: 'Alt+F12', screenshotKey: 'F8' } })"); await open_cat(pg, "emulator")
        await pg.evaluate("(" + ROW.format(name='"Screenshot key"') + ").querySelector('.rst').click()"); await pg.wait_for_timeout(700)
        msg = await pg.evaluate("document.querySelector('.layer .dialog')?.textContent || ''")
        check("Home key" in msg and settings()["emulator"]["screenshotKey"] == "F8", "restoring a keybind whose default is taken explains and changes nothing")
        await pg.keyboard.press("Escape"); await pg.wait_for_timeout(300)
        # a controller button of this user
        await pg.evaluate("ctlPatch({ mapping: { ...S.profile.controls.mapping, options: 2, search: 3 } })"); await pg.wait_for_timeout(400)
        await pg.evaluate("Controllers.show()"); await pg.wait_for_timeout(600)
        await pg.evaluate("[...document.querySelectorAll('#ctl-body .row')].find(r => r.querySelector('.rl > span')?.firstChild?.textContent === 'Options').querySelector('.rst').click()"); await pg.wait_for_timeout(800)
        m = profile(pid)["controls"]["mapping"]
        check(m["options"] == 3 and m["search"] == 2, f"one controller button is restored (and the button it took is swapped back) {m}")
        # all keybinds
        await pg.evaluate("saveSettings({ emulator: { homeKey: 'F7', fullscreenKey: 'F6', homeButton: false }, recording: { key: 'F10' }, sound: { volume: 0.3 } })")
        await pg.evaluate("ctlPatch({ confirmButton: 'east', mapping: { ...S.profile.controls.mapping, confirm: 1, back: 0, menu: 8 } })"); await pg.wait_for_timeout(400)
        await pg.evaluate("Controllers.show()"); await pg.wait_for_timeout(500)
        await pg.evaluate("[...document.querySelectorAll('#ctl-body .row')].find(r => r.textContent.startsWith('Restore all keybinds')).click()"); await pg.wait_for_timeout(500)
        text = await pg.evaluate("document.querySelector('.layer .dialog')?.textContent || ''")
        check("Restore all keybinds?" in text and "Not changed" in text, "Restore all keybinds asks first and says what is kept")
        await pg.evaluate("[...document.querySelectorAll('.layer .dialog button')].find(b => b.textContent === 'Restore keybinds').click()"); await pg.wait_for_timeout(900)
        s, pr = settings(), profile(pid)
        ok = (s["emulator"]["homeKey"], s["emulator"]["fullscreenKey"], s["emulator"]["homeButton"], s["recording"]["key"]) == ("Shift+H", "F11", True, "F12") \
            and pr["controls"]["confirmButton"] == "south" and pr["controls"]["mapping"]["confirm"] == 0 and pr["controls"]["mapping"]["menu"] == 9
        check(ok, "all keybinds restored (keyboard shortcuts, controller buttons, menu buttons)")
        check(abs(s["sound"]["volume"] - 0.3) < 1e-6, "Restore all keybinds leaves other settings alone")

        # ---- 2. reset all settings
        files = make_user_data()
        game = H.api("/api/games")["games"][0]["key"]
        gview = H.api(f"/api/games/{game}/settings")
        st_id = next(x["id"] for x in gview["settings"] if x.get("kind") == "enum" and (gview["caps"][gview["effective"]].get(x["id"]) or {}).get("perGame"))
        choice = next(x for x in gview["settings"] if x["id"] == st_id)["choices"][-1]["v"]
        H.api(f"/api/games/{game}/settings", {"settings": {st_id: choice}})
        dirs_before = settings()["library"]["dirs"]
        await pg.evaluate("saveSettings({ startup: { color: '#00aa00' }, emulator: { fadeIn: false, prepareShaders: false }, perfMonitor: { mode: 'all' } })")
        H.api("/api/engine-settings", {"settings": {"graphics.backend": "opengl"}})
        await pg.evaluate(f"patchProfile('{pid}', {{ theme: {{ accent: '#00ff00' }}, home: {{ layout: 'grid' }} }})")
        await pg.evaluate("showSettings('general')"); await pg.wait_for_timeout(600)
        await pg.evaluate("[...document.querySelectorAll('#set-panel .row')].find(r => r.textContent.startsWith('Reset all settings')).click()"); await pg.wait_for_timeout(500)
        text = await pg.evaluate("document.querySelector('.layer .dialog')?.textContent || ''")
        check("Kept:" in text and "GitHub" in text and "saves" in text, "Reset all settings asks first and says what is kept")
        await pg.evaluate("[...document.querySelectorAll('.layer .dialog button')].find(b => b.textContent === 'Reset all settings').click()"); await pg.wait_for_timeout(1200)
        s, pr = settings(), profile(pid)
        ok = s["startup"]["color"] == "#e60012" and s["emulator"]["fadeIn"] and s["emulator"]["prepareShaders"] and s["perfMonitor"]["mode"] == "off" \
            and "graphics.backend" not in (H.api("/api/engine-settings")["global"] or {}) and pr["theme"]["accent"] == "#ff2a3a" and pr["home"]["layout"] == "classic"
        check(ok, "Reset all settings restores NEXUS, engine and this user's settings")
        accent = await pg.evaluate("getComputedStyle(document.documentElement).getPropertyValue('--accent').trim()")
        check(accent.lower() == "#ff2a3a", f"the interface shows the defaults at once (accent {accent})")
        kept = all(open(p_, "rb").read() == b_ for p_, b_ in files.items())
        check(kept, "saves, the account sign-in file and engine files are untouched")
        check(settings()["library"]["dirs"] == dirs_before, "library folders are kept")
        check(H.api(f"/api/games/{game}/settings")["overrides"].get(st_id) == choice, "per-game settings are kept")
        check(len(H.api("/api/profiles")) >= 1 and pr["name"], "users are kept")

        # ---- 4. on-screen keyboard: controller
        await pg.evaluate("showSettings('general')"); await pg.wait_for_timeout(600)
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(300)
        await press(pg, X, wait=500)  # the Search button opens the settings search with the keyboard
        check(await pg.evaluate("!!OSK.target && OSK.target.id === 'set-search'"), "controller: Search opens the on-screen keyboard on the settings search")
        start = await pg.evaluate("Nav.cur._k")
        await press(pg, A)                         # q
        await press(pg, DOWN, A)                   # a (row below)
        await press(pg, RIGHT, RIGHT, A)           # d
        await press(pg, X)                         # delete the d
        await press(pg, Y)                         # space
        await press(pg, VIEW, UP, RIGHT, A)        # shift, up to e, right to r: R upper case
        v = await pg.evaluate("document.querySelector('#set-search').value")
        check(start == "q" and v == "qa R", f"controller: D-pad moves, A types, X deletes, Y adds a space, View shifts once ({v!r})")
        await pg.evaluate("document.querySelector('#set-search').value = ''; document.querySelector('#set-search').dispatchEvent(new Event('input'))")
        for k in "fps":
            await pg.evaluate(f"OSK.type('{k}')")
        await press(pg, START, wait=800)           # Enter: closes and goes to the results
        res = await pg.evaluate("[...document.querySelectorAll('#set-panel .row')].map(r => r.querySelector('.rl > span').firstChild.textContent)")
        check(not await pg.evaluate("!!OSK.target") and "FPS" in res, f"controller: Menu is Enter (keyboard closes, search runs) {res[:3]}")
        await pg.evaluate("Nav.setFocus($('#set-search'))"); await press(pg, A, wait=400)
        check(await pg.evaluate("!!OSK.target"), "controller: A on a text field opens the keyboard")
        await press(pg, B, wait=400)
        check(await pg.evaluate("!OSK.target && Nav.cur?.id === 'set-search'"), "controller: B closes the keyboard and returns to the field")

        # ---- 4b. mouse: clicking a field never opens it; clicking keys types
        await pg.mouse.move(400, 400); await pg.wait_for_timeout(100)
        await pg.click("#set-search"); await pg.wait_for_timeout(300)
        check(await pg.evaluate("!OSK.target"), "mouse: clicking a text field does not open the keyboard")
        await pg.evaluate("document.querySelector('#set-search').value = ''")
        await pg.evaluate("OSK.open(document.querySelector('#set-search'))"); await pg.wait_for_timeout(300)
        for key in ["g", "i", "t"]:
            await pg.click(f".osk-key >> text='{key}'"); await pg.wait_for_timeout(120)
        await pg.click(".osk-key >> text='Close'"); await pg.wait_for_timeout(300)
        check(await pg.evaluate("document.querySelector('#set-search').value") == "git" and not await pg.evaluate("!!OSK.target"), "mouse: clicking keys types; Close dismisses")

        # ---- 4c. physical keyboard: the keyboard never opens; typing while it is open closes it
        await pg.keyboard.press("Tab"); await pg.evaluate("Nav.setFocus($('#set-search'))")
        await pg.keyboard.press("Enter"); await pg.wait_for_timeout(300)
        check(await pg.evaluate("!OSK.target"), "keyboard: Enter on a text field does not open the on-screen keyboard")
        await pg.evaluate("document.querySelector('#set-search').value = ''; document.querySelector('#set-search').focus()")
        await pg.keyboard.type("hex"); await pg.wait_for_timeout(400)
        check(await pg.evaluate("S.screen") == "settings" and await pg.evaluate("document.querySelector('#set-search').value") == "hex",
              "keyboard: typing h, e, x in a search field types them (no Home, tab or Options shortcut)")
        await pg.evaluate("OSK.open(document.querySelector('#set-search'))"); await pg.wait_for_timeout(200)
        await pg.keyboard.type("ab"); await pg.wait_for_timeout(300)
        check(await pg.evaluate("!OSK.target && document.querySelector('#set-search').value") == "hexab", "keyboard: typing while the on-screen keyboard is open closes it and types into the field")

        # ---- 5. a game's settings: search, overrides, nothing global changes
        H.api(f"/api/games/{game}/settings", {"resetAll": True})
        global_before = H.api("/api/engine-settings")["global"]
        await pg.keyboard.press("Escape"); await pg.evaluate("goHome()"); await pg.wait_for_timeout(500)
        await pg.evaluate(f"gameConfig('{game}', 'general')"); await pg.wait_for_timeout(900)
        label = next(x for x in gview["settings"] if x["id"] == st_id)["label"]
        await pg.fill(".cfg-search input", label.split()[0]); await pg.wait_for_timeout(500)
        names = await pg.evaluate("[...document.querySelectorAll('.cfg-body .row')].map(r => r.querySelector('.rl > span')?.firstChild?.textContent)")
        tags = await pg.evaluate("[...document.querySelectorAll('.cfg-body .row .src')].map(t => t.textContent)")
        check(label in names and tags and set(tags) == {"Global"}, f"search finds {label!r} in the game's settings, marked Global ({names[:4]})")
        await pg.evaluate("[...document.querySelectorAll('.cfg-body .row')].find(r => r.querySelector('.rl > span')?.firstChild?.textContent === " + json.dumps(label) + ")._adjust(-1)")
        await pg.wait_for_timeout(1000)
        ov = H.api(f"/api/games/{game}/settings")["overrides"]
        check(st_id in ov and H.api("/api/engine-settings")["global"] == global_before, f"changing it in the results sets an override for this game only (global unchanged)")
        tag = await pg.evaluate("[...document.querySelectorAll('.cfg-body .row')].find(r => r.querySelector('.rl > span')?.firstChild?.textContent === " + json.dumps(label) + ")?.querySelector('.src')?.textContent")
        check(tag == "This game", f"the changed setting is marked This game ({tag})")
        await pg.fill(".cfg-search input", ""); await pg.wait_for_timeout(300)
        await pg.evaluate("[...document.querySelectorAll('.cfg-nav .chip')].find(c => c.textContent.startsWith('Only this game')).click()"); await pg.wait_for_timeout(500)
        names = await pg.evaluate("[...document.querySelectorAll('.cfg-body .row')].map(r => r.querySelector('.rl > span')?.firstChild?.textContent)")
        check(names == [label], f"Only this game’s lists exactly the overrides ({names})")
        await pg.evaluate("document.querySelector('.cfg-body .row .rst').click()"); await pg.wait_for_timeout(900)
        check(st_id not in H.api(f"/api/games/{game}/settings")["overrides"] and H.api("/api/engine-settings")["global"] == global_before,
              "its restore button returns the game to the global setting (global unchanged)")
        await b.close()

    # ---- 6. persistence after a restart
    H.api("/api/settings", {"emulator": {"fullscreenKey": "F5"}})
    p.terminate(); p.wait()
    p, _ = H.start()
    s = H.api("/api/state")["settings"]
    check(s["emulator"]["fullscreenKey"] == "F5" and s["startup"]["color"] == "#e60012" and s["emulator"]["homeKey"] == "Shift+H"
          and profile(pid)["theme"]["accent"] == "#ff2a3a", "restored values and later changes persist after restarting NEXUS")
    H.api("/api/settings", {"emulator": {"fullscreenKey": "F11"}})
    p.terminate()
    check(not errs, f"no script errors {errs}")
    sys.exit(1 if fails else 0)

asyncio.run(main())
