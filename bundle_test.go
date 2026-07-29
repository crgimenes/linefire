package linefire

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/crgimenes/linefire/filoio"
)

// bundleMapDir mirrors cmd/linefire's constant: the maps/assets subdirectory inside
// the embedded content.
const bundleMapDir = "gameassets"

// TestBundleRunsStandalone is the packaging smoke test: everything the shipped binary
// needs at startup must resolve from the embedded bundle alone, with no files on disk.
// It loads the player and the opening map exactly as main.go does, so a missing embed
// (or a renamed asset) fails here rather than only on a machine without the repo.
func TestBundleRunsStandalone(t *testing.T) {
	c := Content()

	_, err := filoio.LoadAssetFS(c, bundleMapDir, "player")
	if err != nil {
		t.Fatalf("player asset from bundle: %v", err)
	}
	_, err = filoio.LoadLevelFS(c, bundleMapDir, "map0001")
	if err != nil {
		t.Fatalf("opening map from bundle: %v", err)
	}
}

// TestBundleEmbedsWholeCampaign guards the embed globs: every campaign map and its
// referenced spawn assets, plus at least one music track, must be present in the
// bundle. A silently narrowed //go:embed directive would drop files and strand the
// standalone binary — this catches that at build time.
func TestBundleEmbedsWholeCampaign(t *testing.T) {
	c := Content()

	maps, err := fs.Glob(c, path.Join(bundleMapDir, "map*"+filoio.ExtLevel))
	if err != nil || len(maps) == 0 {
		t.Fatalf("no campaign maps embedded: %v", err)
	}
	for _, mp := range maps {
		name := strings.TrimSuffix(path.Base(mp), filoio.ExtLevel)
		lvl, err := filoio.LoadLevelFS(c, bundleMapDir, name)
		if err != nil {
			t.Fatalf("%s from bundle: %v", name, err)
		}
		for _, s := range lvl.Spawns {
			if s.Asset == "" {
				continue
			}
			if _, err := filoio.LoadAssetFS(c, bundleMapDir, s.Asset); err != nil {
				t.Fatalf("%s references asset %q missing from bundle: %v", name, s.Asset, err)
			}
		}
	}

	tracks, err := fs.Glob(c, "music/*.mp3")
	if err != nil || len(tracks) == 0 {
		t.Fatalf("no music embedded in the bundle: %v", err)
	}
}
