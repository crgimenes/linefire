package mapeditor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModuleRoot: moduleRoot walks up to the directory holding go.mod (where `go run
// ./cmd/linefire` resolves), and errors when there is none above the start.
func TestModuleRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "gameassets", "nested")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}

	got, err := moduleRoot(deep)
	if err != nil {
		t.Fatalf("moduleRoot: %v", err)
	}
	if got != root {
		t.Fatalf("moduleRoot = %q, want %q", got, root)
	}

	// A tree with no go.mod above it must error rather than loop or return "".
	bare := t.TempDir()
	if _, err := moduleRoot(bare); err == nil {
		t.Fatal("moduleRoot should error when no go.mod is found above the start")
	}
}

// TestMapStem: the map's launch name is its file stem, with the .lfm extension dropped.
func TestMapStem(t *testing.T) {
	cases := map[string]string{
		filepath.Join("a", "b", "map0005.lfm"): "map0005",
		"x.lfm":                                "x",
		"map0001.lfm":                          "map0001",
	}
	for in, want := range cases {
		if got := mapStem(in); got != want {
			t.Fatalf("mapStem(%q) = %q, want %q", in, got, want)
		}
	}
}
