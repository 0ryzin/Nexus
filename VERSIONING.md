# NEXUS versioning

NEXUS uses Semantic Versioning for the application: MAJOR.MINOR.PATCH.

The first intended stable application release is **1.0.0**. The current supplied source defaults to 1.0.0-dev, which identifies a development build and is not itself a stable release.

## Version increments

- **Patch** (1.0.1): backwards-compatible fixes, small corrections and security updates.
- **Minor** (1.1.0): backwards-compatible features or substantial improvements.
- **Major** (2.0.0): changes that significantly alter behaviour, compatibility, architecture or user-facing workflows.

Pre-release identifiers may be used while developing a release, for example 1.0.0-dev, 1.0.0-beta.1 or 1.0.0-rc.1.

## Application and engine versions are separate

- **NEXUS application version:** identifies the NEXUS application, interface and user-facing release across supported platforms.
- **NEXUS Engine version:** identifies a compatible engine build and its source commit.

An engine rebuild does not automatically require a NEXUS application version change. Conversely, an application update does not imply that the engine changed.

## Git tags and release names

Use v<version> for application releases, for example v1.0.0 and v1.0.1.

Use the separate engine-v<version> tag namespace for engine builds, for example engine-v0.1.0.

The release archive filename, in-app application version, GitHub release title and application Git tag should agree. Do not use an old development folder name as the canonical version.

## Before the first stable release

Before publishing v1.0.0:

1. Set the application build version to 1.0.0 rather than 1.0.0-dev.
2. Remove stale 2.0.0 labels from documentation and build scripts where they refer to the NEXUS application version.
3. Keep upstream engine version numbers and commit identifiers unchanged.
4. Complete the engine build and real-game validation checklist for each platform being advertised.
5. Publish the complete corresponding source and third-party licence notices alongside any binary distribution.
