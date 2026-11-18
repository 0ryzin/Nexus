# Shared by the NEXUS developer tools in tools/perf and tools/uitest: start a NEXUS build
# (Linux, stand-in engine) on a portable data folder, talk to its API, and a scripted controller
# for the Gamepad API (standard mapping) so the interface's real input path is used.
import json, os, shutil, subprocess, time, urllib.request

BIN = D = None  # set with configure()

def configure(binary, data):
    global BIN, D
    BIN, D = binary, data
PORT = "8774"
HZ = os.sysconf("SC_CLK_TCK")

FAKE_PAD = """
window.__pads = [];
navigator.getGamepads = () => window.__pads;
const __pad = (index, id) => ({ index, id, mapping: 'standard', connected: true,
  buttons: Array.from({ length: 18 }, () => ({ pressed: false, value: 0 })), axes: [0, 0, 0, 0] });
window.__connectPad = (id = 'Scripted controller (STANDARD GAMEPAD)') => {
  window.__pads = [__pad(0, id)];
  window.dispatchEvent(new Event('gamepadconnected'));
};
// more controllers: __addPad(index, id), __removePad(index)
window.__addPad = (index, id) => { window.__pads = window.__pads.filter(p => p.index !== index).concat([__pad(index, id)]); window.dispatchEvent(new Event('gamepadconnected')); };
window.__removePad = index => { window.__pads = window.__pads.filter(p => p.index !== index); window.dispatchEvent(new Event('gamepaddisconnected')); };
window.__disconnectPad = () => { window.__pads = []; window.dispatchEvent(new Event('gamepaddisconnected')); };
window.__press = async (i, ms = 90, pad = 0) => {
  const b = window.__pads.find(p => p.index === pad).buttons[i]; b.pressed = true; b.value = 1;
  await new Promise(r => setTimeout(r, ms)); b.pressed = false; b.value = 0;
  await new Promise(r => setTimeout(r, ms));
};
"""

def cpu_ticks(pid):
    f = open(f"/proc/{pid}/stat").read().rsplit(")", 1)[1].split()
    return int(f[11]) + int(f[12])

def rss_mb(pid):
    for l in open(f"/proc/{pid}/status"):
        if l.startswith("VmRSS"):
            return int(l.split()[1]) / 1024

def api(path, body=None, timeout=5):
    rq = urllib.request.Request(f"http://localhost:{PORT}{path}", data=None if body is None else json.dumps(body).encode(),
                                headers={"Content-Type": "application/json", "X-Nexus": "1"}, method="GET" if body is None else "POST")
    return json.loads(urllib.request.urlopen(rq, timeout=timeout).read() or b"null")

def start():
    subprocess.run(["pkill", "-x", "NEXUS"], stderr=subprocess.DEVNULL)  # any earlier run on this port
    for _ in range(50):
        try:
            api("/api/state", timeout=0.2); time.sleep(0.1)
        except Exception:
            break
    shutil.copy(BIN, D + "/NEXUS")
    # the one-time Steam offers (Linux) would open a dialog over every test
    os.makedirs(D + "/System", exist_ok=True)
    with open(D + "/System/steam-offers.json", "w") as f:
        f.write('{"menu": true, "art": true}')
    env = dict(os.environ, NEXUS_PORT=PORT, NEXUS_DEV_NOAUTH="1", NEXUS_FAKE_ENGINE="1", NEXUS_FAKE_PLAY="600s",
               NEXUS_HARDWARE=os.environ.get("NEXUS_HARDWARE", "/tmp/hw-rtx.json"))
    t0 = time.perf_counter()
    p = subprocess.Popen([D + "/NEXUS"], cwd=D, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    while True:
        try:
            api("/api/state"); break
        except Exception:
            time.sleep(0.005)
    ready = (time.perf_counter() - t0) * 1000
    api("/api/settings", {"startup": {"enabled": False}})
    if not api("/api/profiles"):  # a new data folder: one user instead of the first-run setup
        api("/api/profiles", {"name": "Tester"})
    return p, ready

async def enter(pg):
    """Open the interface, skip the start-up animation, pick the first user, wait for the games."""
    await pg.goto(f"http://localhost:{PORT}/")
    await pg.wait_for_function("typeof S!=='undefined' && (!!S.screen || Nav.top()?.name === 'who')", timeout=20000)
    # Space skips the start-up animation; only while it shows: on Home it would start the focused game
    if await pg.evaluate("!!document.querySelector('#boot') && getComputedStyle(document.querySelector('#boot')).display !== 'none' && document.body.classList.contains('booting')"):
        await pg.keyboard.press("Space")
    await pg.wait_for_timeout(1500)
    if await pg.evaluate("S.screen === 'who' || Nav.top()?.name === 'who'"):
        await pg.evaluate("document.querySelector('.who-item').click()"); await pg.wait_for_timeout(900)
    await pg.wait_for_function("S.games && S.games.length > 0 && S.screen === 'home'", timeout=30000)
