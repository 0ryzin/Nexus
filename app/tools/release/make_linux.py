"""Builds the NEXUS downloads for Linux, one pair per architecture (docs/LINUX.md):

    NEXUS-<version>-Linux-<arch>.tar.gz     NEXUS/ folder (needs WebKitGTK 4.1 from the system)
    NEXUS-<version>-Linux-<arch>.AppImage   everything inside, including GTK and WebKitGTK

<arch> is amd64 (every AMD64 PC and the Steam Deck: one build) or arm64. Both contain the same
NEXUS folder:

    NEXUS/
      nexus                  NEXUS (Go, built with -tags webkitgtk)
      Engine/                nexus_engine.so, the libraries it needs (RUNPATH $ORIGIN), engine.json
      Games/ Customisations/ Resources/ Licences/ Source/
      README.md  README-Linux.txt  NEXUS_ENGINE.md  RELEASE.json  nexus.desktop  72-nexus-switch2.rules

The AppImage is made with linuxdeploy (and its GTK plugin): GTK 3, WebKitGTK and their helpers
are bundled; WebKitGTK's helper processes are found inside the image (see appimage_webkit());
GStreamer's audio plugins come along so the interface's sounds play. Graphics drivers (Mesa,
Vulkan, the NVIDIA driver) and glibc always come from the system.

usage:
  make_linux.py --version 0.8.0 --arch amd64 --bin nexus --engine-dir engine-out
                --engine-source engine-src --engine-patches ../engine/patches --out out/
                [--no-appimage]
"""
import argparse, glob, hashlib, json, os, re, shutil, stat, subprocess, sys, tarfile, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))
TRIPLE = {"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu"}
APPIMAGE_ARCH = {"amd64": "x86_64", "arm64": "aarch64"}
# Windows-only parts of dist-base that a Linux download does not need
WINDOWS_ONLY = {"Microsoft-WebView2Loader-SDK.txt", "go-webview2-MIT.txt", "go-winloader.txt"}


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def patchset_sha(folder):
    h = hashlib.sha256()
    for name in sorted(os.listdir(folder)):
        if name.endswith(".patch"):
            with open(os.path.join(folder, name), "rb") as f:
                h.update(f.read())
    return h.hexdigest()


def run(*cmd, **kw):
    print("+", " ".join(cmd), flush=True)
    subprocess.check_call(cmd, **kw)


def engine_info(lib):
    """The engine's own build information, read by loading it (as NEXUS does)."""
    code = ("import ctypes,json,sys;d=ctypes.CDLL(sys.argv[1]);d.nexus_engine_info.restype=ctypes.c_char_p;"
            "d.nexus_engine_api_version.restype=ctypes.c_int32;"
            "print(json.dumps({'api':d.nexus_engine_api_version(),'info':json.loads(d.nexus_engine_info())}))")
    return json.loads(subprocess.check_output([sys.executable, "-c", code, lib]))


def nexus_api_version():
    src = open(os.path.join(REPO, "enginehost.go"), encoding="utf-8").read()
    return int(re.search(r"const engineAPIVersion = (\d+)", src).group(1))


# Libraries the engine links from the build system that some systems lack (a plain Arch
# install, some Steam Deck images): copied next to the engine in both packages. Graphics,
# input and C runtime libraries (libc, libudev, libdrm, libGL/libvulkan, X11) are left to the
# system, which must provide its own.
BUNDLE_DEPS = re.compile(r"^lib(usb-1\.0|ssl|crypto|lz4|zstd|opus)\.so")


def bundle_engine_deps(lib, dest):
    out = subprocess.check_output(["ldd", lib], text=True)
    for line in out.splitlines():
        m = re.match(r"\s*(\S+) => (/\S+)", line)
        if m and BUNDLE_DEPS.match(m.group(1)) and not os.path.exists(os.path.join(dest, m.group(1))):
            shutil.copy2(os.path.realpath(m.group(2)), os.path.join(dest, m.group(1)))
            os.chmod(os.path.join(dest, m.group(1)), 0o755)
            print(f"  engine dependency bundled: {m.group(1)}")
            copy_licences([m.group(1)], os.path.join(os.path.dirname(dest), "Licences", "linux-libraries"))


def copy_licences(names, outdir):
    """The Debian/Ubuntu copyright file (licence texts) of each package a bundled library came from."""
    names = sorted(set(n for n in names if ".so" in n))
    if not names or not shutil.which("dpkg-query"):
        return
    r = subprocess.run(["dpkg-query", "-S"] + [f"*/{n}" for n in names], capture_output=True, text=True)
    pkgs = set()
    for line in r.stdout.splitlines():
        pk = line.split(":", 1)[0]
        for p in pk.split(","):
            pkgs.add(p.strip())
    os.makedirs(outdir, exist_ok=True)
    for pk in sorted(pkgs):
        for src in (f"/usr/share/doc/{pk}/copyright", f"/usr/share/doc/{pk.split(':')[0]}/copyright"):
            if os.path.exists(src):
                shutil.copy2(src, os.path.join(outdir, pk.split(":")[0] + ".txt"))
                break


def stage_engine(engine_dir, dest, lock, patches):
    """Engine/: nexus_engine.so and the libraries the build made, with engine.json listing them."""
    os.makedirs(dest, exist_ok=True)
    for f in sorted(os.listdir(engine_dir)):
        p = os.path.join(engine_dir, f)
        if f.endswith(".so") or ".so." in f:
            shutil.copy2(p, os.path.join(dest, f))
            os.chmod(os.path.join(dest, f), 0o755)
    lib = os.path.join(dest, "nexus_engine.so")
    if not os.path.exists(lib):
        sys.exit("nexus_engine.so is missing from the engine build")
    bundle_engine_deps(lib, dest)
    # the engine finds the libraries next to it, wherever NEXUS is extracted
    for f in os.listdir(dest):
        run("patchelf", "--set-rpath", "$ORIGIN", os.path.join(dest, f))
    got = engine_info(lib)
    if got["api"] != nexus_api_version():
        sys.exit(f"engine interface {got['api']} does not match NEXUS ({nexus_api_version()}); nothing made")
    commit = open(os.path.join(engine_dir, "COMMIT")).read().strip() if os.path.exists(os.path.join(engine_dir, "COMMIT")) else ""
    files = {f: sha256(os.path.join(dest, f)) for f in sorted(os.listdir(dest))}
    m = {"name": "NEXUS Engine", "version": lock["version"], "api": got["api"], "commit": commit,
         "upstreamBase": lock["upstreamBase"], "patchset": patchset_sha(patches) if patches else "",
         "platform": "linux", "files": files}
    with open(os.path.join(dest, "engine.json"), "w") as f:
        json.dump(m, f, indent=2)
    return m


def stage(a):
    st = tempfile.mkdtemp(prefix="nexus-linux-")
    root = os.path.join(st, "NEXUS")
    shutil.copytree(os.path.join(REPO, "dist-base"), root)
    for d, _, files in os.walk(os.path.join(root, "Licences")):
        for f in files:
            if f in WINDOWS_ONLY:
                os.remove(os.path.join(d, f))
    shutil.copy2(a.bin, os.path.join(root, "nexus"))
    os.chmod(os.path.join(root, "nexus"), 0o755)
    for doc in ("README.md", "NEXUS_ENGINE.md"):
        shutil.copy2(os.path.join(REPO, doc), os.path.join(root, doc))
    shutil.copy2(os.path.join(HERE, "linux", "README-Linux.txt"), os.path.join(root, "README-Linux.txt"))
    shutil.copy2(os.path.join(HERE, "linux", "nexus.desktop"), os.path.join(root, "nexus.desktop"))
    shutil.copy2(os.path.join(HERE, "linux", "72-nexus-switch2.rules"), os.path.join(root, "72-nexus-switch2.rules"))
    os.makedirs(os.path.join(root, "Source"), exist_ok=True)
    shutil.move(os.path.join(root, "SOURCE-README.txt"), os.path.join(root, "Source", "README.txt"))
    src_tar = os.path.join(root, "Source", "NEXUS-source.tar.gz")
    with tarfile.open(src_tar, "w:gz") as t:
        for name in sorted(os.listdir(REPO)):
            if name in (".git", "out", "dist-base") or name.startswith("nexus") and not name.endswith(".go"):
                continue
            t.add(os.path.join(REPO, name), arcname=os.path.join("nexus", name))
    if a.engine_source:
        os.makedirs(os.path.join(root, "Source", "engine"), exist_ok=True)
        run("git", "-C", a.engine_source, "archive", "--format=tar.gz", "-o",
            os.path.join(root, "Source", "engine", "nexus-engine-source.tar.gz"), "HEAD")
    lock = dict(l.split("=", 1) for l in open(os.path.join(REPO, "engine.lock")).read().split() if "=" in l)
    m = stage_engine(a.engine_dir, os.path.join(root, "Engine"), lock, a.engine_patches)
    rel = {"nexus": a.version, "platform": "linux", "arch": a.arch, "builtBy": "tools/release/make_linux.py",
           "engine": {"version": m["version"], "commit": m["commit"], "upstreamBase": m["upstreamBase"], "patchset": m["patchset"]}}
    with open(os.path.join(root, "RELEASE.json"), "w") as f:
        json.dump(rel, f, indent=2)
    return st, root


def make_tar(st, out, name):
    path = os.path.join(out, name + ".tar.gz")
    with tarfile.open(path, "w:gz", compresslevel=9) as t:
        t.add(os.path.join(st, "NEXUS"), arcname="NEXUS")
    return path


def appimage_webkit(appdir, triple):
    """WebKitGTK starts helper processes (WebKitWebProcess, WebKitNetworkProcess) from a path fixed
    when it was built (/usr/lib/<triple>/webkit2gtk-4.1). Inside the image they are at
    usr/lib/<triple>/webkit2gtk-4.1, and AppRun starts NEXUS with usr/ as its working folder, so
    the path is rewritten to the same-length relative "././/lib/<triple>/webkit2gtk-4.1"."""
    sysdir = f"/usr/lib/{triple}/webkit2gtk-4.1"
    dest = os.path.join(appdir, "usr", "lib", triple, "webkit2gtk-4.1")
    shutil.copytree(sysdir, dest, dirs_exist_ok=True)
    # "/usr/" (5 bytes) becomes "././/" (5 bytes, the same as "./")
    old, new = sysdir.encode(), ("././/" + sysdir[len("/usr/"):]).encode()
    assert len(old) == len(new)
    libs = glob.glob(os.path.join(appdir, "usr", "lib", "libwebkit2gtk-4.1.so*")) + glob.glob(os.path.join(appdir, "usr", "lib", "libjavascriptcoregtk-4.1.so*"))
    patched = 0
    for p in libs:
        if os.path.islink(p):
            continue
        b = open(p, "rb").read()
        if old in b:
            open(p, "wb").write(b.replace(old, new)); patched += 1
    if not patched:
        sys.exit("WebKitGTK's helper path was not found in the bundled library; the AppImage would not work")
    return dest


def appimage_gstreamer(appdir, triple):
    """GStreamer plugins WebKitGTK needs to play the interface's sounds."""
    src = f"/usr/lib/{triple}/gstreamer-1.0"
    dest = os.path.join(appdir, "usr", "lib", "gstreamer-1.0")
    os.makedirs(dest, exist_ok=True)
    want = ["coreelements", "app", "audioconvert", "audioresample", "autodetect", "playback", "typefindfunctions",
            "volume", "wavparse", "pulseaudio", "pipewire", "alsa", "audiomixer", "audiotestsrc", "uridecodebin3", "isomp4",
            "opus", "vorbis", "ogg", "flac", "mpg123", "gio", "opengl", "videoconvertscale", "videoconvert", "videoscale",
            # WebKit's Web Audio (NEXUS's interface sounds): decodeAudioData uses deinterleave,
            # playback uses interleave; without them every sound fails to load, silently
            "interleave", "audiorate", "rawparse", "audioparsers", "level"]
    found = []
    for w in want:
        p = os.path.join(src, f"libgst{w}.so")
        if os.path.exists(p):
            shutil.copy2(p, dest); found.append(os.path.join(dest, f"libgst{w}.so"))
    scanner = glob.glob(f"/usr/lib/{triple}/gstreamer1.0/gstreamer-1.0/gst-plugin-scanner")
    if scanner:
        shutil.copy2(scanner[0], os.path.join(dest, "gst-plugin-scanner"))
    return found


def appimage_rpaths(appdir):
    """linuxdeploy's GTK plugin sets every ELF file under usr/lib to RUNPATH $ORIGIN, which is
    right only for the libraries directly in usr/lib. Programs and plugins in folders below it
    (NEXUS in usr/lib/nexus, WebKit's helpers, GStreamer plugins) also need usr/lib itself, or
    they find the bundled libraries only on systems that have their own copies."""
    libdir = os.path.join(appdir, "usr", "lib")
    fixed = 0
    for d, _, files in os.walk(libdir):
        if os.path.abspath(d) == os.path.abspath(libdir):
            continue
        rel = os.path.relpath(libdir, d)
        for f in files:
            p = os.path.join(d, f)
            if os.path.islink(p) or not os.path.isfile(p):
                continue
            with open(p, "rb") as fh:
                if fh.read(4) != b"\x7fELF":
                    continue
            r = subprocess.run(["patchelf", "--print-rpath", p], capture_output=True, text=True)
            if r.returncode != 0:
                continue  # not a dynamic ELF file
            subprocess.check_call(["patchelf", "--set-rpath", f"$ORIGIN:$ORIGIN/{rel}", p])
            fixed += 1
    print(f"  RUNPATH set to reach usr/lib for {fixed} programs and plugins below it")


def make_appimage(st, root, out, name, arch, tools):
    triple = TRIPLE[arch]
    appdir = os.path.join(st, "AppDir")
    nexus_dir = os.path.join(appdir, "usr", "lib", "nexus")
    # the engine is added after linuxdeploy, which would otherwise rewrite its libraries
    shutil.copytree(root, nexus_dir, ignore=lambda d, names: ["Engine"] if d == root else [])
    os.makedirs(os.path.join(appdir, "usr", "bin"), exist_ok=True)
    os.symlink("../lib/nexus/nexus", os.path.join(appdir, "usr", "bin", "nexus"))
    icon = os.path.join(appdir, "nexus.png")
    shutil.copy2(os.path.join(REPO, "web", "assets", "brand", "app-icon-512.png"), icon)
    desktop = os.path.join(appdir, "nexus.desktop")
    shutil.copy2(os.path.join(HERE, "linux", "nexus.desktop"), desktop)
    gst = appimage_gstreamer(appdir, triple)
    env = dict(os.environ, ARCH=APPIMAGE_ARCH[arch], DEPLOY_GTK_VERSION="3", NO_STRIP="1",
               APPIMAGE_EXTRACT_AND_RUN="1", LDAI_OUTPUT=os.path.join(out, name + ".AppImage"))
    cmd = [tools["linuxdeploy"], "--appdir", appdir, "--executable", os.path.join(nexus_dir, "nexus"),
           "--desktop-file", desktop, "--icon-file", icon, "--plugin", "gtk"]
    for lib in gst:
        cmd += ["--library", lib]
    # WebKitGTK and its helpers: deployed with their dependencies, then pointed at the image
    for lib in glob.glob(f"/usr/lib/{triple}/webkit2gtk-4.1/injected-bundle/*.so"):
        cmd += ["--library", lib]
    run(*cmd, env=env)
    appimage_webkit(appdir, triple)
    for helper in glob.glob(os.path.join(appdir, "usr", "lib", triple, "webkit2gtk-4.1", "WebKit*Process")):
        run(tools["linuxdeploy"], "--appdir", appdir, "--executable", helper, env=env)
    # licence texts of every library the AppImage carries (GTK, WebKitGTK, GStreamer ...)
    bundled = [f for _, _, fs in os.walk(os.path.join(appdir, "usr", "lib")) for f in fs]
    copy_licences(bundled, os.path.join(nexus_dir, "Licences", "linux-libraries"))
    # AppRun: from usr/ (WebKit's helpers, above), with the bundled GStreamer plugins, and
    # without a library path from the environment (Steam sets one for its runtime)
    for f in ("AppRun", "AppRun.wrapped"):
        p = os.path.join(appdir, f)
        if os.path.lexists(p):
            os.remove(p)
    shutil.copy2(os.path.join(HERE, "linux", "AppRun"), os.path.join(appdir, "AppRun"))
    os.chmod(os.path.join(appdir, "AppRun"), 0o755)
    appimage_rpaths(appdir)
    shutil.copytree(os.path.join(root, "Engine"), os.path.join(nexus_dir, "Engine"))
    # the engine's files must be exactly those listed in engine.json (NEXUS checks them)
    m = json.load(open(os.path.join(nexus_dir, "Engine", "engine.json")))
    for f, want in m["files"].items():
        if sha256(os.path.join(nexus_dir, "Engine", f)) != want:
            sys.exit(f"Engine/{f} was changed while making the AppImage")
    out_path = os.path.join(out, name + ".AppImage")
    run(tools["appimagetool"], "--no-appstream", appdir, out_path, env=env)
    os.chmod(out_path, os.stat(out_path).st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return out_path


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", required=True)
    ap.add_argument("--arch", required=True, choices=sorted(TRIPLE))
    ap.add_argument("--bin", required=True)
    ap.add_argument("--engine-dir", required=True)
    ap.add_argument("--engine-source")
    ap.add_argument("--engine-patches")
    ap.add_argument("--linuxdeploy")
    ap.add_argument("--appimagetool")
    ap.add_argument("--no-appimage", action="store_true")
    ap.add_argument("--out", default=os.path.join(REPO, "out"))
    a = ap.parse_args()
    os.makedirs(a.out, exist_ok=True)
    st, root = stage(a)
    name = f"NEXUS-{a.version}-Linux-{a.arch}"
    outs = [make_tar(st, a.out, name)]
    if not a.no_appimage:
        outs.append(make_appimage(st, root, a.out, name, a.arch, {"linuxdeploy": a.linuxdeploy, "appimagetool": a.appimagetool}))
    for p in outs:
        with open(p + ".sha256", "w") as f:
            f.write(f"{sha256(p)}  {os.path.basename(p)}\n")
        print(p)
    shutil.rmtree(st)


if __name__ == "__main__":
    main()
