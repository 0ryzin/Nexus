"""Turns the engine's link map into the symbol table NEXUS ships next to the engine
(docs/CRASHES.md, "Engine symbols").

    make_symbols.py <nexus_engine.dll> <nexus_engine.map> <out: nexus_engine.symbols.gz> [--undname <llvm-undname>] [--min N]

The link map (lld-link /MAP, link.exe format) lists every public and static symbol with its
address. The table keeps, sorted by address:

    NEXUSSYM 1 <SHA-256 of the DLL> <image base hex> <count>
    <rva hex> <object file> <name>

It is made for exactly one DLL: NEXUS decodes a crash only when the SHA-256 of the engine it
loaded matches the first line. Before writing, every function the DLL exports must resolve to a
symbol of its own name; otherwise the map is not this DLL's (or was not read correctly) and the
script fails, so a build never ships a wrong or empty table.
"""
import gzip, hashlib, re, struct, subprocess, sys


def pe_info(path):
    """Image base, size of image and the exports (name -> RVA) of a PE32+ DLL."""
    data = open(path, "rb").read()
    pe = struct.unpack_from("<I", data, 0x3C)[0]
    if data[pe:pe + 4] != b"PE\0\0":
        raise SystemExit(f"{path}: not a PE file")
    nsec = struct.unpack_from("<H", data, pe + 6)[0]
    opt_size = struct.unpack_from("<H", data, pe + 20)[0]
    opt = pe + 24
    magic = struct.unpack_from("<H", data, opt)[0]
    if magic != 0x20B:
        raise SystemExit(f"{path}: not a 64-bit PE")
    image_base = struct.unpack_from("<Q", data, opt + 24)[0]
    image_size = struct.unpack_from("<I", data, opt + 56)[0]
    exp_rva, exp_size = struct.unpack_from("<II", data, opt + 112)
    sections = []
    for i in range(nsec):
        s = pe + 24 + opt_size + 40 * i
        vsize, va, rsize, raw = struct.unpack_from("<IIII", data, s + 8)
        sections.append((va, max(vsize, rsize), raw))

    def off(rva):
        for va, size, raw in sections:
            if va <= rva < va + size:
                return raw + rva - va
        raise ValueError(hex(rva))

    exports = {}
    if exp_rva:
        e = off(exp_rva)
        nfuncs, nnames, funcs, names, ords = struct.unpack_from("<IIIII", data, e + 20)
        for i in range(nnames):
            name_rva = struct.unpack_from("<I", data, off(names) + 4 * i)[0]
            ordinal = struct.unpack_from("<H", data, off(ords) + 2 * i)[0]
            func_rva = struct.unpack_from("<I", data, off(funcs) + 4 * ordinal)[0]
            n = data[off(name_rva):data.index(b"\0", off(name_rva))].decode()
            if not (exp_rva <= func_rva < exp_rva + exp_size):  # not a forwarder
                exports[n] = func_rva
    return image_base, image_size, data, exports


# link.exe-format map lines in "Publics by Value" and "Static symbols":
#   0001:00000000       ?Foo@Bar@@QEAAXXZ          0000000180001000 f   bar.obj
#   0001:00001234       nexus_engine_version       0000000180002234 f   nexus_engine.obj
SYMBOL = re.compile(r"^\s*[0-9a-fA-F]{4}:[0-9a-fA-F]{8}\s+(\S+)\s+([0-9a-fA-F]{8,16})\s+(?:[fi]\s+)*(\S.*)?$")


def read_map(path, image_base, image_size):
    out = {}
    in_symbols = False
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            if "Publics by Value" in line or line.strip().startswith("Static symbols"):
                in_symbols = True
                continue
            if not in_symbols:
                continue
            m = SYMBOL.match(line)
            if not m:
                continue
            name, addr, obj = m.group(1), int(m.group(2), 16), (m.group(3) or "").strip()
            rva = addr - image_base
            if not (0 < rva < image_size):
                continue  # absolute symbols, or another base
            obj = obj.split(":")[-1].split("\\")[-1].split("/")[-1] or "-"
            # keep one name per address: prefer a C++ (decorated) or longer name
            old = out.get(rva)
            if old is None or (not old[1].startswith("?") and name.startswith("?")):
                out[rva] = (obj, name)
    return out


def simple_undecorate(m):
    """A readable name from an MSVC-decorated one when llvm-undname is not available:
    ?Run@Worker@Vulkan@@QEAAXXZ -> Vulkan::Worker::Run."""
    if not m.startswith("?"):
        return m
    body = m[1:]
    special = {"0": "{ctor}", "1": "{dtor}"}
    prefix = ""
    if body[:1] in special:
        prefix, body = special[body[0]], body[1:]
    elif body.startswith("?"):
        return m  # operators and other special names: kept decorated
    parts = body.split("@@", 1)[0].split("@")
    parts = [p for p in parts if p and not p.startswith("?$")]
    if prefix:
        cls = parts[0] if parts else ""
        parts = [cls if prefix == "{ctor}" else "~" + cls] + parts
    return "::".join(reversed(parts)) if parts else m


def undecorate_all(names, undname):
    """Readable names. llvm-undname (from the LLVM used for the build) prints, for each name read,
    the name, its undecorated form (nothing when it cannot) and an empty line."""
    out = [simple_undecorate(n) for n in names]
    decorated = [i for i, n in enumerate(names) if n.startswith("?")]
    if not undname or not decorated:
        return out
    try:
        r = subprocess.run([undname], input="\n".join(names[i] for i in decorated) + "\n",
                           capture_output=True, text=True, timeout=900)
        lines = r.stdout.splitlines()
        pos = 0
        for i in decorated:
            while pos < len(lines) and lines[pos] != names[i]:
                pos += 1
            if pos >= len(lines):
                break
            if pos + 1 < len(lines) and lines[pos + 1].strip() and lines[pos + 1] != names[i]:
                out[i] = lines[pos + 1].strip()
                pos += 2
            else:
                pos += 1
    except Exception as e:  # noqa: BLE001
        print(f"warning: llvm-undname failed ({e}); simple names used", file=sys.stderr)
    return out


def main(argv):
    if len(argv) < 4:
        raise SystemExit(__doc__)
    dll, mapfile, out = argv[1], argv[2], argv[3]
    undname = argv[argv.index("--undname") + 1] if "--undname" in argv else None
    minimum = int(argv[argv.index("--min") + 1]) if "--min" in argv else 1000  # tests use small DLLs
    image_base, image_size, data, exports = pe_info(dll)
    sha = hashlib.sha256(data).hexdigest()
    table = read_map(mapfile, image_base, image_size)
    if len(table) < minimum:
        raise SystemExit(f"{mapfile}: only {len(table)} symbols read; not the map of {dll}?")
    rvas = sorted(table)
    raw = [table[r][1] for r in rvas]
    names = undecorate_all(raw, undname)
    # identity check: every export resolves to a symbol named like it
    import bisect
    bad = []
    for name, rva in exports.items():
        i = bisect.bisect_right(rvas, rva) - 1
        if i < 0 or name not in raw[i] and name not in names[i]:
            bad.append(f"{name} at {rva:#x} -> {raw[i] if i >= 0 else None}")
    if not exports or bad:
        raise SystemExit(f"{mapfile} does not match {dll}: " + "; ".join(bad[:10] or ["no exports"]))
    with gzip.open(out, "wt", encoding="utf-8", compresslevel=9) as fh:
        fh.write(f"NEXUSSYM 1 {sha} {image_base:x} {len(rvas)}\n")
        for rva, name in zip(rvas, names):
            obj = table[rva][0].replace(" ", "_")
            fh.write(f"{rva:x} {obj} {name.replace(chr(10), ' ')}\n")
    print(f"{out}: {len(rvas)} symbols for {dll} (sha256 {sha[:16]}…); {len(exports)} exports checked")


if __name__ == "__main__":
    main(sys.argv)
