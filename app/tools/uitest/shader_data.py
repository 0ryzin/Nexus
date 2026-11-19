# UI test (CI): Import Shader Data / Export Shader Data from a game's menu, with the real
# interface and server (the native file and folder pickers are answered by giving the path, as
# the picker would):
#   1. right-click a game: both entries are there; Import offers "From a ZIP file" and
#      "From another device";
#   2. From another device without Device Linking: explained, nothing started;
#   3. Export with no shader data: a clear message;
#   4. import a real export ZIP (1,200 programs, 2 pipelines): progress, then the result counts;
#   5. Export: a ZIP in the chosen folder with the manifest and the pack; importing it again adds
#      nothing and backs up first; the source ZIP is unchanged.
#   python3 shader_data.py <NEXUS binary> <portable data folder> <folder with starfall-shaders.zip>
import asyncio, hashlib, os, sys, tempfile, zipfile
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "perf"))
import nexus_harness as H
from playwright.async_api import async_playwright

H.configure(sys.argv[1], sys.argv[2])
FIX = os.path.join(sys.argv[3], "starfall-shaders.zip")
fails = []
def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok: fails.append(msg)

async def dialog_text(pg):
    return await pg.evaluate("$$('.dialog').slice(-1)[0]?.textContent || ''")

async def click(pg, label):
    await pg.evaluate(f"(() => {{ [...$$('.dialog button')].reverse().find(b => b.textContent.trim().startsWith({label!r})).click(); }})()")
    await pg.wait_for_timeout(400)

async def wait_result(pg, title):
    for _ in range(100):
        t = await pg.evaluate("$$('.dialog h2').map(x => x.textContent)")
        if title in t: return await dialog_text(pg)
        await pg.wait_for_timeout(200)
    return ""

async def main():
    # a clean start for this game's shader data (earlier tests may have left some)
    import shutil
    shutil.rmtree(os.path.join(sys.argv[2], "System", "Engine", "cache", "shader", "0100aaaa00010000"), ignore_errors=True)
    p, _ = H.start()
    src_hash = hashlib.sha256(open(FIX, "rb").read()).hexdigest()
    out = tempfile.mkdtemp()
    async with async_playwright() as pw:
        b = await pw.chromium.launch(executable_path="/usr/bin/google-chrome")
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await H.enter(pg)
        # the pickers: the test answers them with a path (the dialogs are native)
        await pg.evaluate(f"""(() => {{ const real = api; window.api = (url, o = {{}}) => {{
            if (url.endsWith('/shaders/export-zip')) o = {{ ...o, body: {{ dir: {out!r} }} }};
            if (url.endsWith('/shaders/import-zip-file')) o = {{ ...o, body: {{ path: window.__zip }} }};
            return real(url, o); }}; }})()""")
        # Starfall Rally (its update file is a separate entry); found by title ID, whatever key the
        # library gave it on this machine
        base = "S.games.find(g => (g.titleId || '').toUpperCase() === '0100AAAA00010000' && g.kind !== 'update' && !g.key.startsWith('x-'))"
        print("note: games", await pg.evaluate("JSON.stringify(S.games.map(g => [g.key, g.kind, g.title, g.titleId]))"))
        print("note: keys", await pg.evaluate("api('/api/system-files').then(k => JSON.stringify(k).slice(0, 400)).catch(e => e.message)"))
        await pg.wait_for_function(f"!!{base}", timeout=15000)
        key = await pg.evaluate(f"{base}.key")
        print("note: Starfall Rally is", key)
        tile = f"[...$$('#tiles .tile')].find(t => t._game?.key === {key!r})"
        # its Home tile's right-click; if earlier tests left Home showing other games, the same
        # game menu opened directly
        try:
            await pg.wait_for_function(f"!!{tile}", timeout=8000)
            await pg.evaluate(f"{tile}.dispatchEvent(new MouseEvent('contextmenu', {{ bubbles: true }}))")
        except Exception:
            print("note: the game is not on Home; opening its menu directly")
            await pg.evaluate(f"gameMenu({key!r})")
        await pg.wait_for_timeout(500)
        items = await pg.evaluate("$$('.game-menu .gm-l').map(x => x.firstChild.textContent)")
        check("Import Shader Data…" in items and "Export Shader Data…" in items, f"right-click shows both entries {items}")
        # 3. export before there is any data
        await click(pg, "Export Shader Data")
        t = await wait_result(pg, "Export Shader Data")
        check("no shader data on this device yet" in t, f"export without data explains it ({t[:90]!r})")
        await click(pg, "OK")
        # 1/2. import menu; From another device without linking
        await pg.evaluate(f"(() => {{ void gameMenu({key!r}); }})()"); await pg.wait_for_timeout(400)
        await click(pg, "Import Shader Data")
        t = await dialog_text(pg)
        check("From a ZIP file" in t and "From another device" in t, "Import offers a ZIP file and another device")
        await click(pg, "From another device")
        t = await wait_result(pg, "No other device to import from")
        check("Device Linking" in t and "From a ZIP file" in t, "no linked device: explained, with the ZIP way")
        await click(pg, "OK")
        # 4. import the real export
        await pg.evaluate(f"window.__zip = {FIX!r}")
        await pg.evaluate(f"(() => {{ void shaderImportMenu(S.games.find(g => g.key === {key!r})); }})()"); await pg.wait_for_timeout(400)
        await click(pg, "From a ZIP file")
        t = await wait_result(pg, "Import Shader Data")
        check("1,202 shader entries were added" in t and "1 used" in t, f"import: counts reported ({t[:120]!r})")
        await click(pg, "OK")
        # 5. export, then import that again
        await pg.evaluate(f"(() => {{ void shaderExportZip(S.games.find(g => g.key === {key!r})); }})()")
        t = await wait_result(pg, "Shader data exported")
        check("1,200 shader programs and 2 pipelines" in t, f"export result ({t[:100]!r})")
        await click(pg, "OK")
        zips = [f for f in os.listdir(out) if f.endswith(".zip")]
        check(len(zips) == 1 and "[0100AAAA00010000]" in zips[0], f"the ZIP is in the chosen folder {zips}")
        names = zipfile.ZipFile(os.path.join(out, zips[0])).namelist() if zips else []
        check(len(names) == 2 and names[0] == "nexus-shader-export.json" and names[1].endswith(".nxshaders"), f"manifest and pack only {names}")
        await pg.evaluate(f"window.__zip = {os.path.join(out, zips[0]) if zips else ''!r}")
        await pg.evaluate(f"(() => {{ void shaderImportMenu(S.games.find(g => g.key === {key!r})); }})()"); await pg.wait_for_timeout(400)
        await click(pg, "From a ZIP file")
        t = await wait_result(pg, "Import Shader Data")
        check("already here" in t and "backed up first" in t, f"re-import adds nothing, backs up first ({t[:160]!r})")
        await click(pg, "OK")
        check(hashlib.sha256(open(FIX, "rb").read()).hexdigest() == src_hash, "the source ZIP is unchanged")
        await b.close()
    p.terminate()
    print("FAILED" if fails else "ALL PASSED")
    sys.exit(1 if fails else 0)

asyncio.run(main())
