# UI test (docs/CONTROLLERS.md): controller names and button prompts in NEXUS menus. Runs a NEXUS
# build (Linux, stand-in engine, which reports an Xbox One S controller and a DualSense) in
# Chrome with scripted controllers through the Gamepad API.
#
#   python3 controller_prompts.py <NEXUS binary> <portable data folder> [screenshot folder]
#
# Checks:
#   1. an XInput controller (the browser calls it "Xbox 360 Controller") is named from the
#      engine's detection, and the prompts show Xbox buttons (A to confirm);
#   2. using a DualSense switches the prompts to PlayStation symbols (Cross) and the name;
#   3. the DualSense leaving: prompts go back to Xbox; coming back with a button already held
#      does not press it;
#   4. "Controller for NEXUS menus" lists the real names and keeps the choice after a reload;
#      only the chosen controller moves the menus.
import asyncio, os, sys
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
from nexus_harness import *
configure(sys.argv[1], sys.argv[2])
SHOTS = sys.argv[3] if len(sys.argv) > 3 else None
XINPUT = "Xbox 360 Controller (XInput STANDARD GAMEPAD)"
DS = "DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)"

async def prompts(pg):
    return await pg.evaluate("""({ glyphs: [...document.querySelectorAll('#hint-list .pad-key svg')].map(s => s.getAttribute('aria-label')),
        pad: document.querySelector('#hint-pad')?.textContent || '' })""")

async def main():
    p, _ = start()
    ok = True
    def check(cond, msg):
        nonlocal ok
        print(("PASS " if cond else "FAIL ") + msg)
        ok &= bool(cond)
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 760})
        await pg.add_init_script(FAKE_PAD)
        await enter(pg)
        await pg.evaluate("ctlPatch({ preferredPad: '', preferredPadName: '' })")  # a fresh start: any controller
        await pg.evaluate(f"window.__connectPad({XINPUT!r})")
        await pg.wait_for_timeout(600)
        await pg.evaluate("window.__press(13)")  # D-pad down: use the controller
        await pg.wait_for_function("(S.padInfo||[]).length === 1 && S.padInfo[0].exact", timeout=5000)
        r = await prompts(pg)
        check("Xbox One S Controller" in r["pad"] and "360" not in r["pad"] and "A" in r["glyphs"], f"XInput pad named from the engine, Xbox prompts: {r}")
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "prompts-xbox.png"))
        await pg.evaluate(f"window.__addPad(1, {DS!r})")
        await pg.wait_for_timeout(600)
        await pg.evaluate("window.__press(13, 90, 1)")
        await pg.wait_for_timeout(400)
        r = await prompts(pg)
        check("DualSense" in r["pad"] and "Cross" in r["glyphs"] and "A" not in r["glyphs"], f"DualSense in use, PlayStation prompts: {r}")
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "prompts-playstation.png"))
        await pg.evaluate("window.__removePad(1)")
        await pg.wait_for_timeout(600)
        r = await prompts(pg)
        check("Xbox One S Controller" in r["pad"] and "A" in r["glyphs"], f"DualSense gone, Xbox prompts again: {r}")
        # back with Cross held: no press
        layers = await pg.evaluate("Nav.layers.length"); screen = await pg.evaluate("S.screen")
        await pg.evaluate("window.__acts = 0; const __a = Nav.activate; Nav.activate = function () { window.__acts++; (window.__why = window.__why || []).push(new Error().stack.split(String.fromCharCode(10)).slice(1,4).join('|')); return __a.apply(this, arguments); }; 0")
        await pg.evaluate(f"window.__addPad(1, {DS!r}); window.__pads.find(p => p.index === 1).buttons[0].pressed = true")
        await pg.wait_for_timeout(700)
        await pg.evaluate("window.__pads.find(p => p.index === 1).buttons[0].pressed = false")
        await pg.wait_for_timeout(300)
        after = await pg.evaluate("({ layers: Nav.layers.length, screen: S.screen, top: Nav.top()?.name, activations: window.__acts, why: window.__why })")
        check(after["activations"] == 0 and after["layers"] == layers and after["screen"] == screen, f"a button held while reconnecting is not pressed ({layers} layers on {screen} before, {after} after)")
        # the menu controller choice, by name, kept after a reload
        await pg.evaluate("Controllers.show()")
        await pg.wait_for_timeout(1500)
        names = await pg.evaluate("[...document.querySelectorAll('.pad-card h4')].map(e => e.textContent)")
        check(any("Xbox One S" in n for n in names) and any("DualSense" in n for n in names) and not any("360" in n or n.startswith("Xbox Wireless") for n in names), f"controller cards {names}")
        dskey = await pg.evaluate("S.padInfo.find(i => i.family === 'playstation').key")
        await pg.evaluate(f"ctlPatch({{ preferredPad: {dskey!r}, preferredPadName: 'PlayStation 5 Controller (DualSense)' }})")
        await pg.wait_for_timeout(500)
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "controllers-screen.png"))
        await pg.reload()
        await pg.add_init_script(FAKE_PAD)
        await pg.wait_for_function("typeof S!=='undefined' && S.profiles", timeout=20000)
        await pg.wait_for_timeout(2500)
        pref = await pg.evaluate("(S.profile || profileById(S.settings.lastProfileId))?.controls?.preferredPad")
        check(pref == dskey, f"menu controller kept after a reload: {pref}")
        await b.close()
    p.terminate()
    sys.exit(0 if ok else 1)

asyncio.run(main())
