#!/bin/bash
# Starts a NEXUS AppImage in a virtual display and waits until its interface is running
# (the interface reports "profile picker shown" / "home interactive" to NEXUS, which logs it),
# then takes a screenshot. Used by CI on the build machine and in clean containers.
#   smoke.sh <AppImage> <output folder> <label> [extra env assignments...]
set -u
ai="$1"; out="$2"; label="$3"; shift 3
mkdir -p "$out"
run() { # $1 = name, rest = env
  local name="$1"; shift
  local data="$out/data-$name"; rm -rf "$data"; mkdir -p "$data"
  env "$@" NEXUS_DATA="$data" APPIMAGE_EXTRACT_AND_RUN=1 xvfb-run -a -s "-screen 0 1280x800x24" bash -c '
    "$0" >"$1/stdout.log" 2>&1 & pid=$!
    for i in $(seq 1 90); do
      if grep -qE "startup: (profile picker shown|home interactive)" "$1/Logs/nexus.log" 2>/dev/null; then break; fi
      kill -0 $pid 2>/dev/null || break
      sleep 1
    done
    sleep 2; import -window root "$1/screen.png" 2>/dev/null || true
    kill $pid 2>/dev/null; sleep 1; kill -9 $pid 2>/dev/null; true' "$ai" "$data"
  cp "$data/Logs/nexus.log" "$out/$label-$name.log" 2>/dev/null || true
  cp "$data/stdout.log" "$out/$label-$name-stdout.log" 2>/dev/null || true
  cp "$data/screen.png" "$out/$label-$name.png" 2>/dev/null || true
  if ! grep -qE "startup: (profile picker shown|home interactive)" "$data/Logs/nexus.log" 2>/dev/null; then
    echo "::error::$label/$name: the NEXUS interface did not start"
    echo "--- first lines of the output"; grep -v '^\s*$' "$data/stdout.log" 2>/dev/null | grep -vE '^\s+(/|nexus/|runtime/)|^(r[0-9]+|lr|sp|pc|fault)\s' | head -40
    echo "--- NEXUS log"; tail -25 "$data/Logs/nexus.log" 2>/dev/null
    return 1
  fi
  grep -E "startup: " "$data/Logs/nexus.log"
}
ok=0
# the engine library loads here, with only what this system has (no missing libraries)
APPIMAGE_EXTRACT_AND_RUN=1 "$ai" --engine-check "$out/$label-engine.json" >/dev/null 2>&1 || true
cat "$out/$label-engine.json" 2>/dev/null; echo
grep -q '"ready": *true' "$out/$label-engine.json" 2>/dev/null || { echo "::error::$label: the engine library does not load"; ok=1; }
run desktop || ok=1
# Steam's Gaming Mode sets SteamGamepadUI=1 for the games it starts: NEXUS must go full screen
run gamingmode SteamGamepadUI=1 SteamDeck=1 || ok=1
grep -q "Gaming Mode true" "$out/$label-gamingmode.log" 2>/dev/null || { echo "::error::$label: Gaming Mode was not recognised"; ok=1; }
# controller navigation in NEXUS needs the web view's Gamepad API (WebKitGTK built with libmanette)
grep -q "gamepad API available" "$out/$label-desktop.log" 2>/dev/null || { echo "::error::$label: the web view has no Gamepad API (no controller navigation)"; ok=1; }
exit $ok
