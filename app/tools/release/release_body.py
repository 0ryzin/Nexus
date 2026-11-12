#!/usr/bin/env python3
"""Composes the GitHub release page for a NEXUS version.

The page has a download section for Windows (the only platform NEXUS is released for) followed
by the release notes from docs/releases/v<version>.md and links to earlier releases. The
download row is generated from the ZIP that was actually built, with its SHA-256, so the page
can never list a package that does not exist. After publishing, the workflow checks that the
asset really is downloadable (see verify_release.py).

Usage: release_body.py --version 0.4.0 --repo owner/Name --zip out/NEXUS-0.4.0-Windows-x64.zip
                       --notes docs/releases/v0.4.0.md --out out/release-body.md
"""
import argparse, glob, hashlib, os, re, sys

REQUIRED_SECTIONS = ["New Features", "Improvements", "Bug Fixes", "Known Issues", "Future Development"]
# other platforms are not advertised in any form (not even as planned) until they are released
FORBIDDEN = re.compile(r"\b(linux|macos|mac os|steam deck|android|ios|iphone|ipad)\b", re.I)


def vkey(v):
    return tuple(int(x) for x in re.findall(r"\d+", v)[:3])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--zip", required=True)
    ap.add_argument("--notes", required=True)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()

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
    m = FORBIDDEN.search(body_wo_future)
    if m:
        sys.exit(f"release notes mention '{m.group(0)}': only Windows is released")
    future = re.split(r"^##+\s+Future Development\s*$", notes, flags=re.M)[1]
    if re.search(r"\b(linux|macos|mac os|steam deck)\b", future, re.I):
        sys.exit("Future Development must not name Linux or macOS")

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
    out.append("")
    out.append(f"SHA-256: `{sha}` ([checksum file]({url}.sha256))")
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
