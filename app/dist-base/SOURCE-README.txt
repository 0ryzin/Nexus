SOURCE CODE (GNU GPL v3.0 or later)
===================================

NEXUS-source.zip
    Complete source of NEXUS (NEXUS.exe and its interface), including the release tooling
    (tools/release/make_release.py, .github/workflows/release.yml, engine.lock).
    Build instructions: README.md section 11.

engine\nexus-engine-source.tar.gz
    Complete source of the NEXUS engine at the commit recorded in RELEASE.json / Engine\engine.json:
    the NEXUS fork of Lichii (https://github.com/zortos293/Lichii, based on Eden, yuzu and others)
    with the NEXUS engine library, its tests, CI workflow and packaging tools. External
    dependencies are fetched by its build system (docs/Build.md, cpmfile.json).

Together these are the complete corresponding source of the NEXUS.exe and Engine\nexus_engine.dll
in this package.
