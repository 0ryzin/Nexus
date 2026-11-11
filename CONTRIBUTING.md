# Contributing to NEXUS

Thank you for your interest in NEXUS. The project is experimental and its first priority is to make the application and integrated engine build reliably and behave correctly.

## Before opening an issue

- Search existing issues to avoid duplicates.
- Include your NEXUS application version and, when relevant, the NEXUS Engine build version and source commit.
- Describe what you expected, what happened, and the exact steps to reproduce it.
- Include relevant logs from `%APPDATA%\\NEXUS\\`, after removing personal information and sensitive paths.
- Do not attach game dumps, console keys, firmware, account credentials or copyrighted game files.

## Code contributions

- Keep changes focused and explain the reason for the change.
- Do not introduce a separate emulator executable launch path. The intended architecture is an engine library loaded and controlled by NEXUS.
- Keep application versioning separate from engine build versioning.
- Add or update tests for changed behaviour.
- Clearly label experimental or unverified system support.
- Preserve upstream copyright and licence notices in third-party code.
- Include corresponding source and required notices when changing distributed GPL-covered components.

## Pull requests

A pull request should explain:

1. What changed and why.
2. Which parts of the application or engine are affected.
3. How the change was tested.
4. Any remaining limitations or unverified behaviour.
5. Any new third-party dependencies, assets or licence obligations.

Please do not claim real-game compatibility unless it has been tested with a real engine build and the result can be reproduced.

## Legal note

Only contribute code, assets and documentation that you have the right to submit. Do not submit game files, console keys, firmware or other material you are not authorised to redistribute.
