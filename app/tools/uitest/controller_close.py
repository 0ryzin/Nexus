# UI test (docs/DEVELOPMENT_NOTES.md): closing a game that runs in the background, from NEXUS
# Home, with a controller only. Runs a NEXUS build (Linux, stand-in engine) and drives the real
# interface in Chrome; the controller is scripted through the Gamepad API (standard mapping), so
# the same input path as a real pad is used (Pad.poll -> Nav). No mouse, no keyboard.
#
#   python3 controller_close.py <NEXUS binary> <portable data folder>
#
# Three ways, each from a fresh game in the background:
#   1. the game's tile -> Options (Y/X) -> Close Game -> confirm
#   2. Start -> Power options -> Close <game> -> confirm
#   3. Up from the tiles -> the Home banner -> Close game -> confirm
# Each checks: Cancel is focused first in the confirmation, the session ends, the tile's
# running light goes, focus stays on a Home tile. Exit code 0 when all pass.
import asyncio, json, os, sys, time
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
from nexus_harness import *
configure(sys.argv[1], sys.argv[2])

A, B, Y, START, UP, DOWN, LEFT, RIGHT = 0, 1, 3, 9, 12, 13, 14, 15

async def press(pg, *buttons):
    for b in buttons:
        await pg.evaluate(f"window.__press({b})")
        await pg.wait_for_timeout(250)

async def start_game_in_background(pg):
    key = await pg.evaluate("S.games.find(isPlayable).key")
    await pg.evaluate(f"launchGame('{key}', true)")
    await pg.wait_for_function("S.playing && S.playing.phase === 'playing'", timeout=30000)
    await pg.evaluate("goHome({button:true})")
    await pg.wait_for_function("S.screen === 'home' && S.playing && S.playing.suspended", timeout=15000)
    await pg.wait_for_timeout(3000)  # longer than the loading screen's reveal timers
    where = await pg.evaluate("({screen: S.screen, layers: Nav.layers.map(l => l.name || 'screen')})")
    assert where["screen"] == "home" and "launch" not in where["layers"], f"left Home after Home was pressed: {where}"
    # focus the running game's tile, as a player would after pressing Home
    await pg.evaluate("Nav.setFocus($$('#tiles .tile').find(t => t._game?.key === S.playing.gameKey))")
    return key

async def focused(pg):
    return await pg.evaluate("Nav.cur ? (Nav.cur.textContent || '').trim().slice(0, 60) : null")

async def expect_closed(pg, name):
    await pg.wait_for_function("!S.playing || !S.playing.running", timeout=20000)
    await pg.wait_for_timeout(500)
    st = api("/api/state")
    running = ((st.get("emulator") or {}).get("state") or {}).get("running", False)
    dot = await pg.evaluate("!!document.querySelector('#tiles .tile.running')")
    on_tile = await pg.evaluate("!!(Nav.cur && Nav.cur.closest('#tiles'))")
    ok = not running and not dot and on_tile
    print(f"{'PASS' if ok else 'FAIL'} {name}: server running={running}, running light={dot}, focus on a tile={on_tile}")
    return ok

async def confirm_close(pg):
    # the confirmation opens with Cancel focused (a stray A press must not close the game)
    first = await focused(pg)
    await press(pg, RIGHT)
    second = await focused(pg)
    await press(pg, A)
    return first, second

async def main():
    p, _ = start()
    results = []
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        ctx = await b.new_context(viewport={"width": 1440, "height": 860})
        await ctx.add_init_script(FAKE_PAD)
        pg = await ctx.new_page()
        await enter(pg)
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(500)

        # 1. Options on the tile
        await start_game_in_background(pg)
        await press(pg, Y)
        items = await pg.evaluate("Nav.items(Nav.top().root).map(e => e.textContent.trim())")
        print("  game menu:", items)
        await press(pg, DOWN)  # Resume -> Close Game
        assert (await focused(pg)) == "Close Game", await focused(pg)
        await press(pg, A)
        first, second = await confirm_close(pg)
        print(f"  confirmation focus: {first!r} then {second!r}")
        results.append(first == "Cancel" and await expect_closed(pg, "tile Options -> Close Game"))

        # 2. Power menu (Start)
        await start_game_in_background(pg)
        await press(pg, START)
        items = await pg.evaluate("Nav.items(Nav.top().root).map(e => e.textContent.trim())")
        print("  power menu:", items[:3])
        await press(pg, DOWN)  # Resume <game> -> Close <game>
        assert (await focused(pg)).startswith("Close "), await focused(pg)
        await press(pg, A)
        first, _ = await confirm_close(pg)
        results.append(first == "Cancel" and await expect_closed(pg, "Start -> Power -> Close"))

        # 3. Home banner
        await start_game_in_background(pg)
        await press(pg, UP)
        print("  after Up:", await focused(pg))
        await press(pg, RIGHT)
        assert (await focused(pg)) == "Close game", await focused(pg)
        await press(pg, A)
        first, _ = await confirm_close(pg)
        results.append(first == "Cancel" and await expect_closed(pg, "Up -> banner -> Close game"))
        await b.close()
    p.terminate()
    sys.exit(0 if all(results) else 1)

asyncio.run(main())
