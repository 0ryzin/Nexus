# Third-party notices and acknowledgements

This document summarises the third-party projects, libraries, fonts and licence families identified in the supplied NEXUS development package. It is an attribution guide, not a replacement for the licence text or notices that must accompany the corresponding source and binaries.

## Integrated emulation engine

NEXUS's planned in-process engine is based on **Lichii, build-130**, under GNU GPL-3.0-or-later:

- Lichii: https://github.com/zortos293/Lichii
- Eden: https://github.com/eden-emulator/mirror
- yuzu: upstream project lineage used by Eden and related forks.

The supplied package identifies the NEXUS engine as a fork of Lichii build-130, itself within the Eden/yuzu lineage. Preserve upstream copyright notices and the complete corresponding source, including NEXUS engine patches, when distributing covered binaries.

The engine source package also carries third-party component licence texts for dependencies and components including:

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

The relevant licence texts are included in the supplied package under NEXUS/Licences/.

## Other emulator projects

**Ryujinx** is listed in the supplied project documentation as a technical reference only. The package states that no Ryujinx code or executable is included or launched. Its MIT notice is retained for attribution if a specific fix is ported in the future. Do not describe Ryujinx as an integrated NEXUS engine or as a current code dependency.

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
4. Update this file whenever a dependency, upstream base, font, library or redistributed asset changes.
5. Do not add a blanket NEXUS licence to third-party files that carry their own licence.
