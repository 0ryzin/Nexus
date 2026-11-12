#!/usr/bin/env python3
"""Writes rsrc_windows_amd64.syso: the Windows resources linked into NEXUS.exe.

Keeps the existing application manifest and icons from the current .syso and adds (or replaces)
the version information Windows shows in the file's Properties › Details and in the
"publisher" column of some tools:

    CompanyName      Nexus Development
    ProductName      NEXUS
    FileDescription  NEXUS
    FileVersion / ProductVersion  from app/VERSION
    LegalCopyright   Copyright (C) Nexus Development

This is application metadata only. It is NOT a digital signature: Windows shows a verified
publisher (and SmartScreen trusts the file) only when the .exe is Authenticode-signed with a
code-signing certificate, which NEXUS does not have. See docs/SIGNING.md.

Usage: make_rsrc.py [--version 0.4.0] [--syso path]   (offline, standard library only)
"""
import argparse, os, struct, sys

RT_VERSION = 16
LANG = 0x0809  # English (United Kingdom)
CODEPAGE = 1200  # Unicode


# ---------------------------------------------------------------- read the existing .syso
def read_resources(data):
    """Returns [(type, id, lang, bytes)] from a COFF object holding one .rsrc section."""
    _, nsec, _, symptr, nsym, optsz, _ = struct.unpack_from('<HHIIIHH', data, 0)
    sec = None
    for i in range(nsec):
        o = 20 + optsz + i * 40
        if data[o:o + 8].rstrip(b'\0') == b'.rsrc':
            sec = struct.unpack_from('<IIIIIIHHI', data, o + 8)
    if not sec:
        raise SystemExit('no .rsrc section')
    _, _, size, ptr, relptr, _, nrel, _, _ = sec
    raw = data[ptr:ptr + size]
    # in an object file the data offsets are 0-based within the section (relocated at link time)
    out = []

    def entries(off):
        _, _, _, _, nnamed, nid = struct.unpack_from('<IIHHHH', raw, off)
        for k in range(nnamed + nid):
            name, target = struct.unpack_from('<II', raw, off + 16 + 8 * k)
            yield name, target

    for t, toff in entries(0):
        for n, noff in entries(toff & 0x7fffffff):
            for lang, leaf in entries(noff & 0x7fffffff):
                rva, sz, _, _ = struct.unpack_from('<IIII', raw, leaf)
                out.append((t, n, lang, raw[rva:rva + sz]))
    return out


# ---------------------------------------------------------------- VS_VERSIONINFO
def _pad4(b):
    return b + b'\0' * (-len(b) % 4)


def _node(key, value=b'', children=b'', text=False, wtype=None):
    """One VS_VERSIONINFO-style block: header, key, value, children (each 32-bit aligned)."""
    pre = _pad4(struct.pack('<HHH', 0, 0, 0) + (key + '\0').encode('utf-16-le'))
    body = pre + value
    if children:
        body = _pad4(body) + children
    vlen = len(value) // 2 if text else len(value)
    if wtype is None:
        wtype = 1 if (text or not value) else 0
    return struct.pack('<HHH', len(body), vlen, wtype) + body[6:]


def _string(key, val):
    return _pad4(_node(key, (val + '\0').encode('utf-16-le'), text=True))


def version_info(ver, fields):
    parts = [int(x) for x in (ver.split('-')[0].split('.') + ['0', '0', '0'])[:4]]
    ms, ls = (parts[0] << 16) | parts[1], (parts[2] << 16) | parts[3]
    ffi = struct.pack('<13I', 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x4, 0x1, 0, 0, 0)
    table = b''.join(_string(k, v) for k, v in fields)
    st = _pad4(_node('%04X%04X' % (LANG, CODEPAGE), children=table))
    sfi = _pad4(_node('StringFileInfo', children=st))
    var = _pad4(_node('Translation', struct.pack('<HH', LANG, CODEPAGE)))
    vfi = _pad4(_node('VarFileInfo', children=var))
    return _node('VS_VERSION_INFO', ffi, sfi + vfi)


# ---------------------------------------------------------------- write a COFF .rsrc object
def write_syso(resources):
    """resources: [(type, id, lang, bytes)], integer types/ids only."""
    tree = {}
    for t, n, lang, b in resources:
        tree.setdefault(t, {}).setdefault(n, {})[lang] = b

    def dir_size(n):
        return 16 + 8 * n

    # layout: all directories, then data entries, then the data
    dirs, order = [], []
    offs = 0
    root_off = offs; offs += dir_size(len(tree))
    type_offs, name_offs = {}, {}
    for t in sorted(tree):
        type_offs[t] = offs; offs += dir_size(len(tree[t]))
    for t in sorted(tree):
        for n in sorted(tree[t]):
            name_offs[(t, n)] = offs; offs += dir_size(len(tree[t][n]))
    leaf_offs = {}
    for t in sorted(tree):
        for n in sorted(tree[t]):
            for lang in sorted(tree[t][n]):
                leaf_offs[(t, n, lang)] = offs; offs += 16
    data_offs = {}
    for t in sorted(tree):
        for n in sorted(tree[t]):
            for lang in sorted(tree[t][n]):
                offs += -offs % 8
                data_offs[(t, n, lang)] = offs; offs += len(tree[t][n][lang])
    raw = bytearray(offs + (-offs % 8))

    def put_dir(at, items):
        struct.pack_into('<IIHHHH', raw, at, 0, 0, 0, 0, 0, len(items))
        for k, (ident, target, is_dir) in enumerate(items):
            struct.pack_into('<II', raw, at + 16 + 8 * k, ident, target | (0x80000000 if is_dir else 0))

    put_dir(root_off, [(t, type_offs[t], True) for t in sorted(tree)])
    relocs = []
    for t in sorted(tree):
        put_dir(type_offs[t], [(n, name_offs[(t, n)], True) for n in sorted(tree[t])])
        for n in sorted(tree[t]):
            put_dir(name_offs[(t, n)], [(lang, leaf_offs[(t, n, lang)], False) for lang in sorted(tree[t][n])])
            for lang in sorted(tree[t][n]):
                lo, do, b = leaf_offs[(t, n, lang)], data_offs[(t, n, lang)], tree[t][n][lang]
                struct.pack_into('<IIII', raw, lo, do, len(b), 0, 0)
                raw[do:do + len(b)] = b
                relocs.append(lo)  # OffsetToData is an RVA: relocated against the section symbol

    sec_ptr = 20 + 40
    rel_ptr = sec_ptr + len(raw)
    sym_ptr = rel_ptr + 10 * len(relocs)
    out = bytearray()
    out += struct.pack('<HHIIIHH', 0x8664, 1, 0, sym_ptr, 1, 0, 0)
    out += b'.rsrc\0\0\0' + struct.pack('<IIIIIIHHI', 0, 0, len(raw), sec_ptr, rel_ptr, 0, len(relocs), 0, 0xC0000040)
    out += raw
    for r in relocs:
        out += struct.pack('<IIH', r, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB
    out += b'.rsrc\0\0\0' + struct.pack('<IhHBB', 0, 1, 0, 3, 0)  # static section symbol
    out += struct.pack('<I', 4)  # empty string table
    return bytes(out)


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    app = os.path.abspath(os.path.join(here, '..', '..'))
    ap = argparse.ArgumentParser()
    ap.add_argument('--version', default=open(os.path.join(app, 'VERSION')).read().strip())
    ap.add_argument('--syso', default=os.path.join(app, 'rsrc_windows_amd64.syso'))
    a = ap.parse_args()
    res = [r for r in read_resources(open(a.syso, 'rb').read()) if r[0] != RT_VERSION]
    ver = a.version
    fields = [
        ('CompanyName', 'Nexus Development'),
        ('FileDescription', 'NEXUS'),
        ('FileVersion', ver),
        ('InternalName', 'NEXUS'),
        ('LegalCopyright', 'Copyright (C) Nexus Development. Third-party notices: Licences folder.'),
        ('OriginalFilename', 'NEXUS.exe'),
        ('ProductName', 'NEXUS'),
        ('ProductVersion', ver),
    ]
    res.append((RT_VERSION, 1, LANG, version_info(ver, fields)))
    data = write_syso(res)
    # sanity: the result must read back with the same resources
    back = read_resources(data)
    assert sorted((t, n, l, b) for t, n, l, b in back) == sorted(res), 'round trip failed'
    open(a.syso, 'wb').write(data)
    print(f'{a.syso}: {len(res)} resources, version {ver}, CompanyName "Nexus Development"')


if __name__ == '__main__':
    main()
