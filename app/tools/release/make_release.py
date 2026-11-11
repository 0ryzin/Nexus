"""Builds the one NEXUS download: NEXUS-<version>-Windows-x64.zip

    NEXUS/
      NEXUS.exe
      Engine/                 the matching NEXUS engine (nexus_engine.dll, engine.json,
        Libraries/            its DLLs incl. the app-local VC++ runtime, Licences/)
      Games/  Resources/  Licences/  Source/
      README.md  NEXUS_ENGINE.md  RELEASE.json

The engine zip is the artifact of the engine CI (tools/nexus/package_engine.py in the engine
repository) for the commit pinned in engine.lock. It is verified file by file and must use the
interface version this NEXUS.exe was built for; otherwise no release is produced.

usage:
  make_release.py --version 2.1.0 --exe NEXUS.exe --engine nexus-engine-2.1.0-win64.zip
                  --engine-source <engine checkout or source dir> --out dist/
  (--no-engine produces a clearly named developer package without an engine; it is never a release)
"""
import argparse, hashlib, io, json, os, re, shutil, subprocess, sys, tempfile, zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def patchset_sha(folder):
    """SHA-256 over the patch series, in order (same as the CI computes)."""
    h = hashlib.sha256()
    for name in sorted(os.listdir(folder)):
        if name.endswith(".patch"):
            with open(os.path.join(folder, name), "rb") as f:
                h.update(f.read())
    return h.hexdigest()


def nexus_api_version():
    src = open(os.path.join(REPO, "enginehost.go"), encoding="utf-8").read()
    return int(re.search(r"const engineAPIVersion = (\d+)", src).group(1))


def verify_engine(engine_dir):
    m = json.load(open(os.path.join(engine_dir, "engine.json"), encoding="utf-8"))
    if m.get("api") != nexus_api_version():
        sys.exit(f"engine interface {m.get('api')} does not match NEXUS ({nexus_api_version()}); no release made")
    if "nexus_engine.dll" not in m.get("files", {}):
        sys.exit("engine.json does not list nexus_engine.dll")
    for name, want in m["files"].items():
        if ".." in name or name.startswith("/") or "\\" in name or ":" in name:
            sys.exit(f"invalid file name in engine.json: {name}")
        p = os.path.join(engine_dir, *name.split("/"))
        if not os.path.exists(p) or sha256(p) != want.lower():
            sys.exit(f"engine file missing or damaged: {name}")
    return m


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", required=True)
    ap.add_argument("--exe", required=True)
    ap.add_argument("--engine")
    ap.add_argument("--no-engine", action="store_true")
    ap.add_argument("--engine-source", help="engine checkout (for the corresponding-source archive)")
    ap.add_argument("--engine-patches", help="engine/patches folder; the engine must have been built from it")
    ap.add_argument("--out", default=os.path.join(REPO, "out"))
    a = ap.parse_args()
    if not a.engine and not a.no_engine:
        sys.exit("--engine is required (use --no-engine only for developer packages)")

    stage = tempfile.mkdtemp(prefix="nexus-release-")
    root = os.path.join(stage, "NEXUS")
    shutil.copytree(os.path.join(REPO, "dist-base"), root)
    shutil.copy2(a.exe, os.path.join(root, "NEXUS.exe"))
    for doc in ("README.md", "NEXUS_ENGINE.md"):
        shutil.copy2(os.path.join(REPO, doc), os.path.join(root, doc))
    os.makedirs(os.path.join(root, "Source"), exist_ok=True)
    shutil.move(os.path.join(root, "SOURCE-README.txt"), os.path.join(root, "Source", "README.txt"))

    # NEXUS source (GPL): the repository without build outputs and the release base
    with zipfile.ZipFile(os.path.join(root, "Source", "NEXUS-source.zip"), "w", zipfile.ZIP_DEFLATED) as z:
        for d, dirs, files in os.walk(REPO):
            rel = os.path.relpath(d, REPO)
            dirs[:] = [x for x in dirs if not (rel == "." and x in (".git", "out", "dist-base"))]
            for f in files:
                z.write(os.path.join(d, f), os.path.join("nexus", rel, f))

    release = {"nexus": a.version, "engine": None, "builtBy": "tools/release/make_release.py"}
    engine_dir = os.path.join(root, "Engine")
    os.makedirs(engine_dir, exist_ok=True)
    if a.engine:
        with zipfile.ZipFile(a.engine) as z:
            for n in z.namelist():
                if ".." in n or n.startswith("/"):
                    sys.exit(f"unsafe path in engine zip: {n}")
            z.extractall(engine_dir)
        m = verify_engine(engine_dir)
        release["engine"] = {"version": m["version"], "commit": m.get("commit"), "upstreamBase": m.get("upstreamBase")}
        release["engine"]["patchset"] = m.get("patchset")
        lock = open(os.path.join(REPO, "engine.lock"), encoding="utf-8").read()
        want = re.search(r"^upstreamBase=(\S+)", lock, re.M)
        if want and m.get("upstreamBase") != want.group(1):
            sys.exit(f"engine upstream base {m.get('upstreamBase')!r} is not the one pinned in engine.lock ({want.group(1)})")
        if a.engine_patches:
            series = patchset_sha(a.engine_patches)
            if m.get("patchset") != series:
                sys.exit("the engine was not built from this repository's patch series (engine/patches)")
    else:
        with open(os.path.join(engine_dir, "README.txt"), "w") as f:
            f.write("DEVELOPER PACKAGE: no NEXUS engine is included. Release packages always contain it.\n")
    if a.engine_source:
        se = os.path.join(root, "Source", "engine")
        os.makedirs(se, exist_ok=True)
        subprocess.check_call(["git", "-C", a.engine_source, "archive", "--format=tar.gz",
                               "-o", os.path.join(se, "nexus-engine-source.tar.gz"), "HEAD"])
    with open(os.path.join(root, "RELEASE.json"), "w") as f:
        json.dump(release, f, indent=2)

    os.makedirs(a.out, exist_ok=True)
    name = f"NEXUS-{a.version}-Windows-x64" + ("" if a.engine else "-DEVELOPER-NO-ENGINE") + ".zip"
    zpath = os.path.join(a.out, name)
    if os.path.exists(zpath):
        os.remove(zpath)
    with zipfile.ZipFile(zpath, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for d, _, files in os.walk(root):
            for f in files:
                p = os.path.join(d, f)
                z.write(p, os.path.relpath(p, stage))
    # self-check: one top folder, NEXUS.exe and (for releases) a complete engine
    with zipfile.ZipFile(zpath) as z:
        names = z.namelist()
        assert all(n.startswith("NEXUS/") for n in names)
        assert "NEXUS/NEXUS.exe" in names
        if a.engine:
            assert "NEXUS/Engine/nexus_engine.dll" in names and "NEXUS/Engine/engine.json" in names
    with open(zpath + ".sha256", "w") as f:
        f.write(f"{sha256(zpath)}  {name}\n")
    shutil.rmtree(stage)
    print(zpath)


if __name__ == "__main__":
    main()
