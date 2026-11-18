package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrameGenShownFollowsTheEngine(t *testing.T) {
	if got := frameGenShown(&FrameGenStatus{State: "running"}, false); got != "" {
		t.Fatalf("off: %q", got)
	}
	if got := frameGenShown(nil, true); got != "" {
		t.Fatalf("older engine: %q", got)
	}
	if got := frameGenShown(&FrameGenStatus{State: "unsupported-gpu"}, true); got != "unsupported-gpu" {
		t.Fatalf("on: %q", got)
	}
	dir := t.TempDir()
	cfg := filepath.Join(dir, "s.ini")
	os.WriteFile(cfg, []byte("[Renderer]\nframe_gen=true\n"), 0o644)
	if !sessionFrameGenOn(cfg) {
		t.Fatal("frame_gen=true not seen")
	}
	os.WriteFile(cfg, []byte("[Renderer]\nframe_gen=false\n"), 0o644)
	if sessionFrameGenOn(cfg) || sessionFrameGenOn(filepath.Join(dir, "missing.ini")) {
		t.Fatal("off or missing read as on")
	}
}
