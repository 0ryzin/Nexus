# UI test (CI): Home has no "Games" or "Recently played" section, tab, card or shortcut, in every
# layout, after a game was played (so recent games exist) and with a favourite (so the page strip
# shows). The game row still launches games and ends with All Software.
#   python3 home_sections.py <NEXUS binary> <portable data folder>
import asyncio, os, sys
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

VISIBLE = """[...document.querySelectorAll('#home *, #dock *')].filter(e => e.offsetParent && !e.children.length)
  .map(e => (e.textContent || e.getAttribute('aria-label') || '').trim()).concat(
  [...document.querySelectorAll('#home [aria-label], #dock [aria-label]')].filter(e => e.offsetParent).map(e => e.getAttribute('aria-label')))"""

async def main():
    p, _ = H.start()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await H.enter(pg)
        key = await pg.evaluate("S.games.find(isPlayable).key")
        # not awaited: with several users (profile_delete.py runs before this and leaves two)
        # launchGame waits for "Who is playing?", and an awaited evaluate never returned (CI hung
        # 31 minutes on 19 Nov)
        await pg.evaluate(f"void launchGame({key!r})")
        # the game starts; on the way "Who's playing?" (first user) or a notice dialog (its main
        # button) may come first; each step is printed so a CI failure shows where it stopped
        started = False
        for _ in range(120):
            st = await pg.evaluate("({ phase: S.playing?.phase || '', layer: Nav.top()?.name || '', dialog: !!Nav.top()?.dialog, text: (document.querySelector('.dialog')?.innerText || '').slice(0, 160) })")
            if st["phase"] == "playing":
                started = True
                break
            if st["layer"] == "who-plays" and await pg.evaluate("!!document.querySelector('.wp-user')"):
                print("step: who is playing -> first user")
                await pg.evaluate("document.querySelector('.wp-user').click()")
            elif st["dialog"]:
                print("step: dialog:", st["text"].replace("\n", " / "))
                await pg.evaluate("(document.querySelector('.dialog button.primary') || [...document.querySelectorAll('.dialog button')].pop())?.click()")
            await pg.wait_for_timeout(500)
        if not started:
            print("state:", await pg.evaluate("JSON.stringify({ playing: S.playing, layer: Nav.top()?.name, screen: S.screen, profile: S.profile?.name })"))
        check(started, "a game launches from Home")
        await pg.evaluate("api('/api/emulator/stop', { body: {} })"); await pg.wait_for_timeout(2500)
        await pg.evaluate("loadState().then(() => goHome())"); await pg.wait_for_timeout(1000)
        check(await pg.evaluate("S.recent.length > 0"), "recent games exist (the data is kept)")
        for fav in ([], [key]):
            for lay in ("classic", "cards", "grid"):
                await pg.evaluate(f"homePatch({{ layout: '{lay}', favorites: {fav!r}, folders: [], pages: [], widgets: {{ details: true, recent: true, playtime: true, storage: true }} }})")
                await pg.wait_for_timeout(600)
                texts = await pg.evaluate(VISIBLE)
                bad = [t for t in texts if t and (t.lower() in ("games", "recently played") or t.lower().startswith("recently played"))]
                check(not bad, f"{lay}{' + favourite' if fav else ''}: no Games / Recently played section {bad}")
        await pg.evaluate("homePatch({ layout: 'classic', favorites: [] })"); await pg.wait_for_timeout(500)
        first = await pg.evaluate("$$('#tiles .tile').filter(t => t._game)[0]._game.key")
        last = await pg.evaluate("[...document.querySelectorAll('#tiles > *')].slice(-1)[0].textContent")
        check(first == key and "All Software" in last, f"the row: last played first ({first}), ends with All Software ({last!r})")
        await pg.evaluate("showLibrary()"); await pg.wait_for_timeout(600)
        check(await pg.evaluate("document.querySelectorAll('#lib-grid .lib-item').length") > 0, "the Library still lists the games")
        await b.close()
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
