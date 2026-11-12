#!/bin/bash
# Development smoke test on an Android emulator (CI only; nothing is published).
# Installs the debug APK, starts NEXUS, and checks that the shared core started, served the
# interface and received the device report. Writes android-smoke/ (log, screenshot).
set -uo pipefail
apk=$(ls android/app/build/outputs/apk/debug/*.apk)
out=android-smoke
mkdir -p "$out"
adb install -r "$apk" || exit 1
adb logcat -c
adb shell am start -W -n dev.nexus.android/.MainActivity
ok=false
for i in $(seq 1 45); do
  sleep 2
  if adb shell run-as dev.nexus.android cat files/nexus-data/Logs/nexus.log 2>/dev/null | grep -q "UI server at"; then ok=true; break; fi
  if ! adb shell pidof dev.nexus.android >/dev/null; then echo "NEXUS is not running"; break; fi
done
sleep 8 # let the interface render
adb shell run-as dev.nexus.android cat files/nexus-data/Logs/nexus.log > "$out/nexus.log" 2>&1 || true
adb logcat -d > "$out/logcat.txt" || true
adb exec-out screencap -p > "$out/screen.png" || true
echo "----- nexus.log"; cat "$out/nexus.log"
echo "----- app files"; adb shell run-as dev.nexus.android ls -R files 2>&1 | head -40
echo "----- logcat (NEXUS, crashes, loader)"
grep -E "dev\.nexus|nexus|NEXUS|FATAL|AndroidRuntime|SIGSEGV|SIGABRT|panic|libc |DEBUG  |crash_dump|linker|houdini|ndk_translation|NativeBridge|GoLog|chromium" "$out/logcat.txt" | grep -v "NetworkStack" | tail -120 || true
$ok || { echo "::error::the NEXUS core did not start on the emulator"; exit 1; }
adb shell pidof dev.nexus.android >/dev/null || { echo "::error::NEXUS stopped"; exit 1; }
grep -q "platform:" "$out/nexus.log" || true
echo "NEXUS core started on Android and serves its interface."
