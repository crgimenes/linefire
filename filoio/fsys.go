package filoio

import (
	"fmt"
	"io/fs"
	"os"
	"path"

	"linefire/asset"
	"linefire/level"
)

// The game runtime reads its data through an fs.FS, so the shipped binary can serve
// it from an embed.FS (a standalone executable, no files on disk) while dev and
// community builds pass an os-backed FS to load the same names from a directory.
// Only the game reads this way; the editors still take real filesystem paths, since
// they also WRITE and browse arbitrary locations.

// osFS reads a name straight from the operating system filesystem. Unlike os.DirFS it
// imposes no rooting or validation, so callers can keep passing working-directory
// paths — including "" and ".." (the tests load "../gameassets") — that fs.ValidPath
// would reject. The shipped binary swaps in an embed.FS instead.
type osFS struct{}

func (osFS) Open(name string) (fs.File, error) {
	// #nosec G304 -- opening the asset/level/music path the game names is the purpose
	return os.Open(name)
}

// OSFS returns an fs.FS backed directly by the operating system filesystem, the
// default the game uses when no embedded bundle is supplied.
func OSFS() fs.FS { return osFS{} }

// fsOrOS treats a nil FS as the operating system filesystem, so a caller that never
// set one (a bare struct literal in a test) reads from the working directory rather
// than panicking in fs.ReadFile.
func fsOrOS(fsys fs.FS) fs.FS {
	if fsys == nil {
		return osFS{}
	}
	return fsys
}

// LoadAssetFS reads and validates the asset named ref (a bare name such as "turret")
// from dir within fsys. It is the fs.FS twin of LoadAsset, sharing ParseAsset.
func LoadAssetFS(fsys fs.FS, dir, ref string) (*asset.Asset, error) {
	if ref == "" {
		return nil, fmt.Errorf("empty asset reference")
	}
	name := path.Join(dir, ref+ExtAsset)
	data, err := fs.ReadFile(fsOrOS(fsys), name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	a, err := ParseAsset(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return a, nil
}

// LoadLevelFS reads and validates the map named name (a file stem) from dir within
// fsys. It is the fs.FS twin of LoadLevel, sharing ParseLevel.
func LoadLevelFS(fsys fs.FS, dir, name string) (*level.Level, error) {
	p := path.Join(dir, name+ExtLevel)
	data, err := fs.ReadFile(fsOrOS(fsys), p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	l, err := ParseLevel(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return l, nil
}
