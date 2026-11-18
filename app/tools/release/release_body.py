#!/usr/bin/env python3
"""Composes the GitHub release page for a NEXUS version.

The page has a download section for Windows (the only platform NEXUS is released for) followed
by the release notes from docs/releases/v<version>.md and links to earlier releases. The
download row is generated from the ZIP that was actually built, with its SHA-256, so the page
can never list a package that does not exist. After publishing, the workflow checks that the
asset really is downloadable (see verify_release.py).

Usage: release_body.py --version 0.4.0 --repo owner/Name --zip out/NEXUS-0.4.0-Windows-x64.zip
                       --notes docs/releases/v0.4.0.md --out out/release-body.md
                       [--linux-dir out/linux]

With --linux-dir the page also offers Linux (AMD64 and ARM64, each as an AppImage and a tar.gz).
All four Linux files and their .sha256 files must be in that folder, or no page is made: a
platform is listed only with every one of its downloads present and checked.
"""
import argparse, glob, hashlib, os, re, sys

REQUIRED_SECTIONS = ["New Features", "Improvements", "Bug Fixes", "Known Issues", "Future Development"]
# other platforms are not advertised in any form (not even as planned) until they are released
FORBIDDEN = re.compile(r"\b(linux|macos|mac os|steam deck|android|ios|iphone|ipad)\b", re.I)


FORBIDDEN_WITH_LINUX = re.compile(r"\b(macos|mac os|android|ios|iphone|ipad)\b", re.I)

LINUX = [  # (file name pattern, architecture, package, who it is for)
    ("NEXUS-{v}-Linux-amd64.AppImage", "AMD64 (x86_64)", "AppImage", " (PCs, Steam Deck)"),
    ("NEXUS-{v}-Linux-amd64.tar.gz", "AMD64 (x86_64)", "tar.gz", " (PCs, Steam Deck)"),
    ("NEXUS-{v}-Linux-arm64.AppImage", "ARM64 (aarch64)", "AppImage", ""),
    ("NEXUS-{v}-Linux-arm64.tar.gz", "ARM64 (aarch64)", "tar.gz", ""),
]


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def linux_files(d, version):
    out = []
    for pat, arch, kind, who in LINUX:
        name = pat.format(v=version)
        p = os.path.join(d, name)
        if not os.path.isfile(p):
            sys.exit(f"{p} does not exist: Linux is offered only with all four downloads")
        if kind == "AppImage" and not os.access(p, os.X_OK):
            sys.exit(f"{p} is not executable")
        sha = sha256(p)
        sf = p + ".sha256"
        if not os.path.isfile(sf) or not open(sf).read().startswith(sha):
            sys.exit(f"{sf} is missing or does not match")
        out.append({"name": name, "arch": arch, "kind": kind, "who": who, "sha": sha, "mb": os.path.getsize(p) / (1 << 20)})
    return out


def vkey(v):
    return tuple(int(x) for x in re.findall(r"\d+", v)[:3])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--zip", required=True)
    ap.add_argument("--notes", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--linux-dir")
    a = ap.parse_args()
    linux = linux_files(a.linux_dir, a.version) if a.linux_dir else []

    if not os.path.isfile(a.zip):
        sys.exit(f"{a.zip} does not exist: no release page without the package")
    name = os.path.basename(a.zip)
    if name != f"NEXUS-{a.version}-Windows-x64.zip":
        sys.exit(f"unexpected package name {name}")
    h = hashlib.sha256()
    with open(a.zip, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    sha = h.hexdigest()
    size_mb = os.path.getsize(a.zip) / (1 << 20)
    sha_file = a.zip + ".sha256"
    if not os.path.isfile(sha_file) or not open(sha_file).read().startswith(sha):
        sys.exit("the .sha256 file is missing or does not match the ZIP")

    notes = open(a.notes, encoding="utf-8").read().strip()
    for sec in REQUIRED_SECTIONS:
        if not re.search(rf"^##+\s+{re.escape(sec)}\s*$", notes, re.M):
            sys.exit(f"release notes lack the section '{sec}'")
    # "Future Development" is the one place other devices may be mentioned, in general terms
    body_wo_future = re.split(r"^##+\s+Future Development\s*$", notes, flags=re.M)[0]
    forbidden = FORBIDDEN_WITH_LINUX if linux else FORBIDDEN
    m = forbidden.search(body_wo_future)
    if m:
        sys.exit(f"release notes mention '{m.group(0)}': that platform is not released")
    future = re.split(r"^##+\s+Future Development\s*$", notes, flags=re.M)[1]
    if re.search(r"\b(macos|mac os)\b" if linux else r"\b(linux|macos|mac os|steam deck)\b", future, re.I):
        sys.exit("Future Development must not name an unreleased platform")

    base = f"https://github.com/{a.repo}/releases"
    url = f"{base}/download/v{a.version}/{name}"
    pre = "-" in a.version
    out = []
    out.append(f"# NEXUS {a.version}{' (pre-release)' if pre else ''}")
    out.append("")
    out.append("## Download")
    out.append("")
    out.append("| Platform | Status | Architecture | Package | Download |")
    out.append("|---|---|---|---|---|")
    out.append(f"| **Windows** 10 / 11 | ✅ Available | AMD64 (x86_64) | Portable ZIP · {size_mb:.0f} MB | **[{name}]({url})** |")
    for f in linux:
        out.append(f"| **Linux**{f['who']} | ✅ Available | {f['arch']} | {f['kind']} · {f['mb']:.0f} MB | **[{f['name']}]({base}/download/v{a.version}/{f['name']})** |")
    out.append("")
    out.append(f"{'- ' if linux else ''}SHA-256{f' ({name})' if linux else ''}: `{sha}` ([checksum file]({url}.sha256))")
    for f in linux:
        out.append(f"- SHA-256 ({f['name']}): `{f['sha']}` ([checksum file]({base}/download/v{a.version}/{f['name']}.sha256))")
    if linux:
        out.append("")
        out.append("The Steam Deck uses the same **Linux AMD64** download as any other AMD64 Linux PC; there is no separate Steam Deck build. "
                   "Not sure which Linux file? Run `uname -m`: `x86_64` → AMD64, `aarch64` → ARM64.")
    out.append("")
    out.append("### Install")
    out.append("")
    out.append(f"1. **Download** [{name}]({url}).")
    out.append("2. **Extract** the whole ZIP (right-click › *Extract All…*). Do not run NEXUS from inside the ZIP.")
    out.append("3. **Open** the extracted `NEXUS` folder.")
    out.append("4. **Launch** `NEXUS.exe`.")
    out.append("")
    out.append("To update an existing copy, use *Settings › Updates* in NEXUS, or replace your NEXUS folder with the new one. "
               "Your users, saves, screenshots, recordings and settings (in `%APPDATA%\\NEXUS`) are kept.")
    out.append("")
    out.append("NEXUS.exe is **not code-signed** (NEXUS has no code-signing certificate yet). Windows SmartScreen may "
               "therefore show *Windows protected your PC* the first time; *More info* shows the file name. Check the "
               "SHA-256 above if you want to confirm the download is intact. The file's *Details* tab lists "
               "*Nexus Development* as the company; that is the program's own description, not a verified signature.")
    out.append("")
    if linux:
        out.append("### Install on Linux")
        out.append("")
        out.append("**AppImage (recommended):** download it, make it executable (`chmod +x NEXUS-*.AppImage`, or *Properties › Permissions › Is executable*), and run it. "
                   "Everything NEXUS needs is inside; nothing is installed. Your data is kept in `~/.local/share/NEXUS`.")
        out.append("")
        out.append("**tar.gz:** extract it (`tar xzf NEXUS-*.tar.gz`) and run `NEXUS/nexus`. It uses your system's WebKitGTK 4.1 "
                   "(`libwebkit2gtk-4.1-0` on Debian/Ubuntu, `webkit2gtk-4.1` on Arch and Fedora); `README-Linux.txt` inside explains more.")
        out.append("")
        out.append("**Steam Deck:** in Desktop Mode, put the AMD64 AppImage somewhere permanent (for example `~/Applications`), make it executable, "
                   "right-click it › *Add to Steam*, then start NEXUS from your library in Gaming Mode. NEXUS opens full screen, and you return to it when you close a game. "
                   "Keep Steam Input on.")
        out.append("")
        out.append("Linux needs glibc 2.39 or newer (Ubuntu 24.04, Fedora 40, Arch, SteamOS 3.6 or newer) and a Vulkan driver (Mesa or NVIDIA). "
                   "On Linux, NEXUS tells you about new versions; download them from this page.")
        out.append("")
    out.append("### System requirements")
    out.append("")
    out.append("- Windows 10 (64-bit, version 1903 or later) or Windows 11. Screen recording of only the game's sound needs Windows 11.")
    out.append("- A 64-bit Intel or AMD processor (x86-64).")
    out.append("- A graphics card and driver with Vulkan 1.3 (recommended) or OpenGL 4.6.")
    out.append("- Microsoft Edge WebView2 Runtime (included with Windows 11 and current Windows 10).")
    out.append("- Your own legally obtained games, console keys and firmware. NEXUS does not include any.")
    out.append("")
    out.append("---")
    out.append("")
    out.append("<!-- nexus:notes -->")  # NEXUS's update dialog shows what follows this marker
    out.append(notes)
    out.append("")
    hist = sorted({os.path.basename(p)[1:-3] for p in glob.glob(os.path.join(os.path.dirname(a.notes), "v*.md"))},
                  key=vkey, reverse=True)
    hist = [v for v in hist if v != a.version and vkey(v) < vkey(a.version)]
    if hist:
        out.append("## Release history")
        out.append("")
        for v in hist:
            out.append(f"- [NEXUS {v}]({base}/tag/v{v})")
        out.append("")
    os.makedirs(os.path.dirname(os.path.abspath(a.out)), exist_ok=True)
    with open(a.out, "w", encoding="utf-8") as f:
        f.write("\n".join(out))
    print(f"{a.out}: {name}, {size_mb:.0f} MB, sha256 {sha[:12]}…")


if __name__ == "__main__":
    main()
