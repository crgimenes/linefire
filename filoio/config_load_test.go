package filoio

import (
	"os"
	"path/filepath"
	"testing"

	"linefire/config"
)

func TestLoadMissingAndInvalidFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()

	got := LoadConfig(filepath.Join(dir, "nope.filo"))
	if got.Volume != config.Default().Volume || got.Muted != config.Default().Muted {
		t.Fatalf("missing file should load defaults, got %+v", got)
	}

	bad := filepath.Join(dir, "bad.filo")
	err := os.WriteFile(bad, []byte("(config (volume"), 0o600) // unbalanced parens
	if err != nil {
		t.Fatal(err)
	}
	if got := LoadConfig(bad); got.Volume != config.Default().Volume || got.Muted != config.Default().Muted {
		t.Fatalf("corrupt file should load defaults, got %+v", got)
	}

	oor := filepath.Join(dir, "oor.filo")
	err = os.WriteFile(oor, []byte(`(config (volume 4.2) (muted))`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	got = LoadConfig(oor)
	if got.Volume != config.Default().Volume {
		t.Fatalf("an out-of-range volume should reset to the default, got %v", got.Volume)
	}
	if !got.Muted {
		t.Fatal("the valid fields should survive the volume reset")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	// Save creates the directory on first use (a fresh machine).
	path := filepath.Join(t.TempDir(), "linefire", "config.filo")
	want := config.Config{Volume: 0.25, Muted: true}
	err := SaveConfig(path, want)
	if err != nil {
		t.Fatal(err)
	}
	if got := LoadConfig(path); got.Volume != want.Volume || got.Muted != want.Muted {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}
