package game

import (
	"io/fs"
	"sync"

	"github.com/crgimenes/linefire/asset"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/render"
)

// Session-wide asset cache. Every world rebuild (portal, restart, attract swap)
// used to re-read the Filo file and re-triangulate the meshes of the same
// enemies and pickups — per-call caches died with the world that built them.
// Assets and their meshes are immutable at runtime, so one process-wide cache
// keyed by (content FS, dir, ref) shares them across every world. Keying on the
// FS identity keeps community content (-dir) apart from the embedded bundle and
// keeps tests with different temp dirs from poisoning each other.
type assetKey struct {
	content fs.FS
	dir     string
	ref     string
}

type cachedAsset struct {
	a    *asset.Asset
	mesh *render.Mesh
	glow *render.Mesh
}

var (
	assetCacheMu sync.Mutex
	assetCache   = map[assetKey]cachedAsset{}
)

// loadCachedAsset returns the asset and meshes for ref under dir in content,
// loading and building them on first use. A missing/broken asset caches as nils,
// so a bad reference is not re-parsed every spawn.
func loadCachedAsset(content fs.FS, dir, ref string) (*asset.Asset, *render.Mesh, *render.Mesh) {
	if ref == "" {
		return nil, nil, nil
	}
	key := assetKey{content: content, dir: dir, ref: ref}
	assetCacheMu.Lock()
	defer assetCacheMu.Unlock()
	if c, ok := assetCache[key]; ok {
		return c.a, c.mesh, c.glow
	}
	c := cachedAsset{}
	a, err := filoio.LoadAssetFS(content, dir, ref)
	if err == nil {
		c.a = a
		c.mesh = render.BuildLayersMesh(a.Layers)
		c.glow = render.BuildGlowMesh(a.Layers)
	}
	assetCache[key] = c
	return c.a, c.mesh, c.glow
}
