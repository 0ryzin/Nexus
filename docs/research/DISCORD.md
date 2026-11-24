# Discord Rich Presence (0.9.3)

Source: Discord developer documentation, Rich Presence (docs.discord.com/developers/rich-presence),
read 24 Nov 2026.

- The first line is always the Discord application's registered name; it cannot be set per
  presence. NEXUS therefore needs its own application ("NEXUS"); the game is the `details` line.
- Images: up to 300 assets uploaded to the application (PNG, JPEG, WebP), referenced by key, or
  an external https URL for `large_image` / `small_image`. Local files cannot be used, so a
  game's own artwork on the user's computer cannot be shown. NEXUS sends the key `nexus` (an
  asset of that name, if uploaded, is shown).
- Transport: the Discord desktop app's local IPC (`\\.\pipe\discord-ipc-N` on Windows,
  `$XDG_RUNTIME_DIR/discord-ipc-N` on Linux; Flatpak: `app/com.discordapp.Discord/`, Snap:
  `snap.discord/`). Handshake opcode 0 `{v:1, client_id}`, then opcode 1 `SET_ACTIVITY`.
  No network access, no token.
- Detection of other games: Discord detects running games by itself; an application's presence
  is separate. NEXUS clears its presence (activity omitted) when its game ends or the setting is
  turned off and closes the connection, so nothing of NEXUS stays over a game started outside it.
- Rate: NEXUS sends a changed presence at most every 15 s (clearing at once).

Implementation: `app/discord.go`, `discord_windows.go`, `discord_unix.go`; test
`TestDiscordPresence` (a stand-in Discord on a socket). Not tested with the real Discord app.
