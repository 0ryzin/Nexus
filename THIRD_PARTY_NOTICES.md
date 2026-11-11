# Third-party notices, licences and acknowledgements

NEXUS builds on the work of established open-source emulation projects and uses third-party libraries, fonts and other components. This document records the upstream projects and licence families identified in the supplied development package. It is an attribution guide, not a replacement for the original licence texts or notices that must accompany source code and binaries.

## Emulation projects and upstream acknowledgements

NEXUS's emulation development draws on the following projects and their wider upstream lineages:

- **Lichii** — https://github.com/zortos293/Lichii
- **Eden** — https://github.com/eden-emulator/mirror
- **yuzu** — upstream project and code lineage associated with later Switch emulator projects.
- **Ryujinx** — https://github.com/Ryujinx
- **Ryubing** — a fork of Ryujinx, also part of the related emulation work referenced by NEXUS.

The NEXUS Engine's code comes from yuzu, Eden and Lichii (currently Lichii build-150); no Ryujinx or Ryubing code is included. See [docs/ENGINE_SOURCES.md](docs/ENGINE_SOURCES.md).

These projects are acknowledged as part of the NEXUS emulation foundation and development. Their code, contributions and individual components remain subject to their respective upstream licences and copyright notices. The exact origin and applicable licence of each source file or component must be preserved and checked before redistribution; this summary does not replace those notices.

## Third-party engine components

The supplied engine source package contains licence texts for third-party components and dependencies, including these licence families:

- Apache-2.0
- Boost Software License 1.0
- BSD-2-Clause and BSD-3-Clause
- Creative Commons Attribution 4.0 and Attribution-ShareAlike 3.0
- CC0-1.0
- GPL-2.0-or-later and GPL-3.0-or-later
- LGPL-3.0-or-later
- LLVM exception
- MIT
- MPL-2.0
- Unlicense
- WTFPL
- zlib

The exact licence applicable to each component must be checked against that component's own source and notices. Do not infer that every listed licence applies to every dependency.

## NEXUS application dependencies

The supplied application package identifies these dependencies:

| Component | Licence / notice |
|---|---|
| Go standard library | BSD-3-Clause |
| golang.org/x/sys | BSD-3-Clause |
| github.com/jchv/go-webview2 | MIT |
| Microsoft WebView2Loader SDK component | Microsoft BSD-style licence |
| github.com/jchv/go-winloader | ISC |
| Inter font | SIL Open Font License 1.1 |
| Outfit font | SIL Open Font License 1.1 |

The supplied ZIP contains corresponding licence text files under NEXUS/Licences/. Those individual third-party licence text files have not yet been copied into this GitHub repository. Before publishing a binary release, add the applicable texts and preserve the upstream notices alongside the source and release package. The root LICENSE file currently contains the project's GPL-3.0-or-later text.

## Artwork and trademarks

The supplied personal development package contains some Nintendo-derived artwork, including a Switch-inspired emblem/profile artwork and an Ocarina of Time image. The package's own notices identify these assets as owner-supplied personal-use material. They are **not covered by the NEXUS GPL licence and are not cleared for redistribution by this repository**. Remove or replace them before publishing a public build unless you have separate permission.

Nintendo, Nintendo Switch and related marks belong to their respective owners. NEXUS is an independent project and is not affiliated with or endorsed by Nintendo.

## Games, keys and firmware

NEXUS does not include games, prod.keys, title.keys or console firmware. Users are responsible for obtaining and using files lawfully.

## Keeping notices complete

When source code or a release package is added to this repository:

1. Preserve each upstream project's copyright and licence notices.
2. Include the exact licence texts required by each bundled component.
3. Keep the complete corresponding source available for GPL-covered binaries, including NEXUS modifications and build scripts as required by the GPL.
4. Update this file whenever an upstream source, dependency, font, library or redistributed asset changes.
5. Do not add a blanket NEXUS licence to third-party files that carry their own licence.
