# UI test (docs/HOME.md): the Home screen layouts and organisation, with a scripted controller.
#   python3 home_layouts.py <NEXUS binary> <portable data folder> [screenshot folder]
# Checks: Classic, Cards and Grid draw; a folder shows its games and opens; favourites and a
# custom page are pages the shoulder buttons switch; the controller moves through the grid and
# reaches the selected game's Play button; a game's background picture is behind the cards;
# everything is kept after a reload; Restore brings Classic back; no script errors.
import asyncio, base64, os, sys
from playwright.async_api import async_playwright
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
from nexus_harness import *
configure(sys.argv[1], sys.argv[2])
SHOTS = sys.argv[3] if len(sys.argv) > 3 else None
RB, LB, DOWN, RIGHT, A = 5, 4, 13, 15, 0
PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFklEQVR4nGP4z8DwHwyBNAMDA8N/AABG1Af5qQkFoAAAAABJRU5ErkJggg==")

async def main():
    p, _ = start()
    ok = True
    def check(cond, msg):
        nonlocal ok
        print(("PASS " if cond else "FAIL ") + msg)
        ok &= bool(cond)
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1440, "height": 900})
        await pg.add_init_script(FAKE_PAD)
        errs = []
        pg.on("pageerror", lambda e: errs.append(str(e)[:200]))
        await enter(pg)
        await pg.evaluate("homePatch({ layout: 'classic', folders: [], pages: [], favorites: [], hidden: [] })")
        keys = await pg.evaluate("S.games.filter(g => isPlayable(g) && !g.hidden).map(g => g.key)")
        check(len(keys) >= 4, f"test games {len(keys)}")
        k0 = keys[0]
        await pg.evaluate("""async ([k, b64]) => { const bin = Uint8Array.from(atob(b64), c => c.charCodeAt(0));
            await api(`/api/games/${k}/art/background`, { body: new Blob([bin], { type: 'image/png' }) }); }""", [k0, base64.b64encode(PNG).decode()])
        await pg.reload(); await pg.wait_for_function("S.games && S.games.length && S.screen === 'home'", timeout=20000)
        classic = await pg.evaluate("[$('#home').classList.contains('hl-classic'), $$('#tiles .tile').length, $('#home-details').offsetParent === null]")
        check(classic[0] and classic[1] >= 4 and classic[2], f"Classic: {classic}")
        # Cards with the selected game's picture
        await pg.evaluate("homePatch({ layout: 'cards', gameBackground: true })"); await pg.wait_for_timeout(500)
        await pg.evaluate(f"Nav.setFocus($$('#tiles .tile').find(t => t._game?.key === '{k0}'))"); await pg.wait_for_timeout(400)
        cards = await pg.evaluate("[$('#home').classList.contains('hl-cards'), !$('#home-gamebg').hidden, ($('#home-gamebg').style.backgroundImage || ''), $('#home-details .hd-text b')?.textContent]")
        check(cards[0] and cards[1] and "src=background" in cards[2] and cards[3], f"Cards with the game's background: {cards}")
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "home-cards.png"))
        # Grid with a folder, a favourite and a page
        await pg.evaluate(f"""homePatch({{ layout: 'grid', favorites: ['{keys[1]}'],
            folders: [{{ id: 'f1', name: 'Racing', games: ['{keys[2]}', '{keys[3]}'] }}], pages: [{{ id: 'p1', name: 'Weekend', games: ['{keys[0]}'] }}],
            widgets: {{ details: true, recent: true, playtime: true, storage: true }} }})""")
        await pg.wait_for_timeout(500)
        grid = await pg.evaluate("[$$('#tiles .tile.folder').length, $$('#tiles .tile').filter(t => t._game).map(t => t._game.key), $$('#home-pages .hp-tab').map(t => t.textContent), $$('#home-widgets .widget').length]")
        want_tabs = ["Home", "Favourites", "Weekend"]  # no "Recently played" tab: the main row is in that order
        check(grid[0] == 1 and keys[2] not in grid[1] and keys[3] not in grid[1] and grid[2] == want_tabs and grid[3] == 2,  # "Recently played" card removed
              f"Grid: folder tile, its games only in it, pages, widgets: {grid}")
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "home-grid.png"))
        # the controller: move through the grid, Down to the Play button, shoulder buttons switch pages
        await pg.evaluate("window.__connectPad()"); await pg.wait_for_timeout(300)
        await pg.evaluate("Nav.setFocus($('#tiles .tile'))")
        await pg.evaluate(f"window.__press({RIGHT})"); await pg.wait_for_timeout(250)
        moved = await pg.evaluate("Nav.cur && (Nav.cur._game?.key || '')")
        check(moved != "" , f"Right moves to the next game: {moved}")
        seen = []
        for _ in range(3):
            await pg.evaluate(f"window.__press({DOWN})"); await pg.wait_for_timeout(200)
            seen.append(await pg.evaluate("Nav.cur && Nav.cur.className"))
        check(any("hd-play" in c for c in seen), f"Down reaches the selected game's Play button: {seen}")
        await pg.evaluate(f"window.__press({RB})"); await pg.wait_for_timeout(400)
        fav = await pg.evaluate("[HomeState.page, $$('#tiles .tile').filter(t => t._game).map(t => t._game.key)]")
        check(fav[0] == "fav" and fav[1] == [keys[1]], f"RB: Favourites page {fav}")
        await pg.evaluate(f"window.__press({RB})"); await pg.wait_for_timeout(400)
        wk = await pg.evaluate("[HomeState.page, $$('#tiles .tile').filter(t => t._game).map(t => t._game.key)]")
        check(wk[0] == "p1" and wk[1] == [keys[0]], f"RB: the custom page {wk}")
        await pg.evaluate("HomeState.page = 'all'; renderHome()"); await pg.wait_for_timeout(400)
        await pg.evaluate("$$('#tiles .tile.folder')[0].click()"); await pg.wait_for_timeout(600)
        inside = await pg.evaluate("$$('.fold-grid .tile').map(t => t._game.key)")
        check(sorted(inside) == sorted([keys[2], keys[3]]), f"the folder opens with its games: {inside}")
        if SHOTS: await pg.screenshot(path=os.path.join(SHOTS, "home-folder.png"))
        await pg.keyboard.press("Escape"); await pg.wait_for_timeout(300)
        # kept after a reload
        await pg.reload(); await pg.wait_for_function("S.games && S.games.length && S.screen === 'home'", timeout=20000)
        kept = await pg.evaluate("[S.profile.home.layout, S.profile.home.folders.length, S.profile.home.favorites.length, $('#home').classList.contains('hl-grid')]")
        check(kept == ["grid", 1, 1, True], f"kept after a reload: {kept}")
        # Settings › Home screen, then Restore
        await pg.evaluate("showSettings('home')"); await pg.wait_for_timeout(800)
        n = await pg.evaluate("$$('#set-panel .lp-item').length")
        check(n == 3, f"Settings › Home screen shows the three layouts ({n})")
        await pg.evaluate("patchProfile(S.profile.id, { sort: 'recent', home: { layout: 'classic', effects: 'glass', gameBackground: false, bgColor: '', dock: [], dockHidden: [] } })")
        await pg.evaluate("goHome()"); await pg.wait_for_timeout(600)
        check(await pg.evaluate("$('#home').classList.contains('hl-classic')"), "Restore: Classic again")
        check(not errs, f"no script errors {errs}")
        await b.close()
    p.terminate()
    sys.exit(0 if ok else 1)

asyncio.run(main())
