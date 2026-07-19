# Linefire

> [!WARNING]
> **Pre-alpha, work in progress. Not ready for use.**
> This is an early, unfinished project shared in the open while it is built.
> Expect breakage, missing pieces, and changes that break saves and APIs without
> notice. Do not depend on it for anything yet.

Linefire is a 2D twin-stick shoot 'em up written in Go with
[Ebitengine](https://ebitengine.org), using a vector look with a CRT glow.

This repository contains the game and its editors:

- the **game** (`linefire`): a 16-stage campaign of hand-authored maps ending
  in a boss lair; killing the boss opens a one-way portal into **The Rift**, an
  endless chain of procedurally generated screens played for score;
- the **unified editor** (`linefire-edit`): a single window that lays out
  stages (walls, the player start, spawns, zones, portals) and, via a mode
  switch, edits the vector assets they reference; saving is automatic
  (Apple-style: on blur, Cmd+S and close);
- the **standalone asset editor** (`linefire-editor`): opens one asset
  (ship, enemy, projectile, power-up) directly from the command line.

The game keeps the ship centered on screen always pointing up and
rotates/translates the world around it; maps are authored top-down in plain 2D
world coordinates, and that rotation is a runtime camera concern.

## Data format: Filo

**Everything is stored in the Filo language.**
Filo files are programs, not just data: `(def w 64) (def half (/ w 2))` works
inside any document, and maps can use geometry generators like
`(rect x y w h)` and `(circle cx cy r)` that expand at load time.

| Extension | Document | Model package |
|-----------|----------|---------------|
| `.lfa`    | asset (ship, enemy, projectile, power-up) | `asset/` |
| `.lfm`    | map/stage | `level/` |
| `.lfc`    | procedural chunk cache | `procgen/` |

All I/O lives in `filoio/` (parse + emit, with byte-for-byte round-trip
tests); the model packages `asset/` and `level/` are pure structs, free of any
serialization concern. The player's local config (audio, best times) is also
Filo, at `~/.config/linefire/config.filo`.

A minimal map looks like:

```lisp
(level "map0001"
  (version 2)
  (title "Star Raiders")
  (size 1200 1000)
  (music "music/One_Heart_Remaining.mp3")
  (player-start 600 500 -90)
  (wall "walls"
    (stroke "#80ffff") (width 2) (fill "transparent") (glow 0.8)
    (rect 200 200 320 220))
  (spawn "enemy_1" "enemy" 840 110 90 (kind "enemy"))
  (spawn "boss" "tank" 450 150 -90 (kind "tank") (boss))
  (spawn "exit" "portal" 700 80 0 (kind "portal") (target "map0004"))
  (resolution "cleared" "portal" (target "@rift") (at 450 150))
  (editor (snap) (snap-radius 8) (grid) (grid-size 20) (snap-to-grid)))
```

`(boss)` marks an enemy as shielded until every other enemy dies. Resolutions
fire once when their condition is met: `spawn` drops loot, `exit` warps,
`portal` tears a portal open at a position, `win` ends the campaign. Portal
targets starting with `@` are generated at runtime: `@bonus` (a loot cave),
`@rift` (the endless one-way chain).

## Requirements

- Go 1.26+
- Ebitengine `v2.10.0-alpha.x` (downloaded automatically).

`CGO_ENABLED=0` is enough on macOS and Windows; no C toolchain is required
there. On Linux, Ebitengine needs cgo and the X11/OpenGL/ALSA development
headers (see the [Ebitengine install guide](https://ebitengine.org/en/documents/install.html)).

## Running the game

The game data (the `gameassets/` and `music/` trees) is embedded in the binary with
`go:embed`, so a built `linefire` runs standalone: no files alongside it, from any
directory. `-dir` overrides that with a directory on disk (dev iteration or community
content), pointing at a folder laid out like the repository root (`gameassets/` beside
`music/`).

```sh
go run -trimpath ./cmd/linefire              # run from the embedded game data
go run -trimpath ./cmd/linefire -dir .       # read gameassets/ and music/ from the cwd
go run -trimpath ./cmd/linefire -version     # print the build version and exit
```

Controls: arrows/WASD turn and thrust (`S` reverse, ArrowDown brakes), `Q`/`E`
strafe, **Shift** afterburner boost, Space / mouse fire, Tab cycles aim modes,
`G` toggles the combat computer's auto-fire, `M` full-screen automap, `F6`
mute, `F2` render mode, `F3` debug HUD, `F11` fullscreen, Esc pause menu (with
volume, restart and quit).

**Touch (two thumbs).** Each half of the screen is a floating stick that appears
where the finger lands. Left thumb: push where you want to go — the ship turns
there and thrusts while deflected. Right thumb: hold to fire the nose gun
straight ahead; sweep past a short distance and it becomes the turret stick (the
drag direction aims, firing while held). Tap the top-center bars to pause; on
the title/game-over screens any tap continues. Keyboard and mouse stay live —
the touch layer is additive.

## Web build

The game runs in the browser (GitHub Pages): `web/` holds the site —
`index.html` (landing + iframe embed) and `play.html` (the canvas document) —
and the Pages workflow builds `linefire.wasm` + copies `wasm_exec.js` on every
push to trunk. Because all game data is embedded, the wasm module is the whole
game (~26 MB, gzipped in transit).

The web build renders on a lighter profile (`game/perf_js.go`): the offscreen is
capped at 1× — a phone's 3× device scale would make every full-screen pass ~9×
the pixels and WebGL crawls — and the motion-blur budget is smaller. The browser
upscales the frame, which the CRT-glow look absorbs. On a touch device, a THIRD
finger (with both thumbs down) toggles the debug readout (FPS/TPS, blur, scale),
the touch equivalent of F3.

```sh
make wasm        # build web/linefire.wasm + wasm_exec.js locally
make serve-web   # ...and serve web/ at http://localhost:8080 for a browser test
```

The world is destructible: shots and explosions dig through rock, and digging
past a map's edge warps into a fresh, harder procedural screen (with a return
portal home; The Rift has none). The HUD's `ENEMIES` counter
leads the readout: at 0 the stage is clear and its exit opens. The full-screen
automap (`M`) draws the tunnels you have carved as well as the charted walls, so
a dug passage is not an invisible gap.

## Running the editors

```sh
go run -trimpath ./cmd/linefire-edit                          # new map
go run -trimpath ./cmd/linefire-edit gameassets/map0001.lfm   # edit a stage
go run -trimpath ./cmd/linefire-edit -assets gameassets x.lfm # explicit palette dir
go run -trimpath ./cmd/linefire-editor gameassets/player.lfa  # one asset directly
```

Flags come before the file (standard `flag` ordering). `-assets` points at the
directory scanned for the placeable-asset palette (default: the map file's
directory). Saving is automatic (on blur, Cmd+S and close); a new file saves to
`linefire_level.lfm` / `linefire_asset.lfa` until named via the save panel.
Double-clicking an asset in the map editor's list opens it for editing in the
same window; a map opens the map editor on it.

## Map editor

Two toolbar rows: tools + edit actions on top, view + content actions below.
The right panel shows properties, the asset palette (with a category filter
that overlays the list) and the selection's fields.

```
1   select / move a handle (wall vertex, curve control, start, spawn, zone)
2   wall pen: click = corner, click-drag = curve; right-click / double-click /
    Enter finish an OPEN line; click the start vertex to close; Esc cancels
3   set player start
4   place a spawn (its category comes from the palette asset; Tab cycles)
5   add a zone; press 5 again to toggle rect/circle
6   delete node: click a vertex to remove it
```

**Path editing (kutta model).** Deleting a vertex CUTS the path open: a
closed loop reopens there, an open path splits in two; the editor never heals
the gap behind you. Open endpoints always show as **red squares**: in Select,
click one (it rings amber, a rubber band follows the cursor), click another
and they **weld**: the same path closes, two different paths splice into one.
A click and a drag on an endpoint are distinguished by a small movement
threshold, so endpoints still move normally. **Shift+click on an edge inserts
a vertex** there and grabs it. The wall pen never snaps onto existing vertices
(grid only), so drawing near another wall never glues to it.

**Reference image.** `M` (or the `Ref img…` button) loads a raster image
(a screenshot of a classic map, a sketch) behind the canvas as a tracing
template: `Shift+M` shows/hides it, `-`/`=` scale it, `,`/`.` fade it. It is
held in memory only and **never saved into the map**.

```
Del/Backspace  delete the selected node, else the one under the cursor
Tab  cycle the palette      [ ]  rotate selected spawn/start (Shift = fine)
G grid   Shift+G grid-snap  P snap-all   B glow   L wall color
N  negative-space preview (playable interior carved black)
I  import SVG (walls + labeled spawn markers)
F5  playtest: save and launch the game straight onto this map
wheel zoom   Space+drag / middle drag pan   arrows pan (Shift = faster)
0 home   F frame content   Cmd+Z / Cmd+Y undo/redo   S / Cmd+S save   Cmd+O open
Alt  hold for free sub-pixel placement (no snapping)
```

`F5` playtests the open map: it saves, then launches `go run ./cmd/linefire` on
that stage (`-map <name>`), so authoring and trying a screen is one key. The map
must be named first, and the editor must be inside the repository (it runs the
game from the module root). The game window opens beside the editor.

The map is **unbounded**: draw as far as you like; `size` is just the nominal
"home" box that `0` frames. Walls reuse the asset layer format, so they render
with the same vector + glow pipeline as ships. A closed wall is a solid; open
paths render as plain lines (the negative-space preview and the game's flood
fill need closed shapes; open endpoints stay flagged in red).

## Asset editor

```
1   select / move a handle (vertex, origin, hardpoint, collision)
2   line / path pen (click = corner, drag = curve, click start = close)
3   set origin / center      4  add weapon hardpoint   5  add thruster hardpoint
6   collision tool; press 6 again to cycle shape (circle / rect / triangle)
7   delete: click a node to remove it

C / V   cycle stroke / fill color     L  color panel
;  '    decrease / increase current layer glow
O   hardpoint panel     U  sounds panel (gion events: fire, destroy, theme...)
G   grid   Shift+G snap-to-grid   P  snap every coordinate to the grid

Ctrl/Cmd+Z undo   Ctrl/Cmd+Shift+Z / Ctrl/Cmd+Y redo   Ctrl/Cmd+S save
wheel zoom   middle drag / Space+drag pan   0 reset view   F frame content

T   cycle transform scope (asset / layer / path)
H / J   flip horizontal / vertical (around the origin)
R   rotate 90°   Shift+R rotate 15°
B   toggle glow   Shift+B cycle variant (stable / pulse / laser)
K   toggle CRT post-process on the preview

Tab / Shift+Tab   next / previous layer     N add layer   Shift+N delete layer
PageUp / PageDown  move layer in draw order  Shift+H toggle layer visibility
arrows  nudge selected handle (Shift = 10 px)
[  ]    rotate selected hardpoint (Shift = 1°)
-  =    layer stroke width
Esc cancel   double-click / Enter / right-click  finish an open path
Backspace / Delete   remove the last point, or the selected node
Alt  free sub-pixel placement    Shift  constrain a segment to 0/45/90°
```

Layers are drawn in order (later on top), each with its own stroke, fill,
`glow` and optional `(hidden)` flag. `collision` shapes are circle / rect /
triangle. Sounds are gion recipes declared per asset event (`fire`,
`destroy`, `theme`...), synthesized at runtime.

## Snapping

Within `snap-radius` screen pixels of an existing point the editor snaps to it
(highlighted yellow). The exception is the map editor's wall pen, grid-only by
design. Anywhere else, coordinates land on the integer grid so saved files
stay clean; with snap-to-grid on they round to `grid-size` multiples. Hold
Alt/Option for free sub-pixel placement.

## Glow

The canvas and preview show the CRT-style glow with a Kage shader: each visible
layer's strokes are drawn into an emissive buffer scaled by its `glow` value,
blurred, and added back over the crisp lines (`B` toggles, `Shift+B` cycles
stable / pulse / laser). A subtle CRT post-process (curvature, scanlines,
phosphor mask, aberration) applies to the asset editor's 1:1 preview only
(`K`). This is the same renderer the game uses.

## Project layout

```
cmd/linefire/            game runtime entrypoint
cmd/linefire-edit/       unified map/asset editor entrypoint
cmd/linefire-editor/     standalone asset editor entrypoint
cmd/linefire-svg2map/    SVG -> .lfm walls importer (CLI)
asset/                   asset model + validation (pure structs, no I/O)
level/                   stage model + validation (pure structs, no I/O)
filoio/                  Filo parse/emit for every document (.lfa/.lfm/.lfc, config)
game/                    game runtime (ship, combat, enemies, camera, fog, endless)
editor/                  asset editor (state, input, tools, drawing)
mapeditor/               map editor (state, input, tools, drawing)
editapp/                 unified editor shell hosting both editors
editorkit/               shared camera, dot grid, generic undo history
procgen/                 procedural generation (chunks, bonus caves, edge/Rift rooms)
render/                  shared vector renderer, glow and CRT shaders
sfx/                     gion sound recipes + music synthesis helpers
svgimport/               SVG parser (paths/shapes -> wall polylines)
config/                  player config model (audio, best times)
gameassets/              the shipped campaign: player/enemies/power-ups + map*.lfm
music/                   stage soundtrack MP3s
docs/                    campaign plan and other design docs (Portuguese)
```

## Development

```sh
make run          # run the standalone asset editor (GUI)
make run-edit     # run the unified map/asset editor (GUI)
make run-game     # run the game runtime (GUI)
make build        # build cmd/* into bin/
make build-cross  # also build linux/amd64
make release      # standalone game binaries (embedded data) for the desktop matrix
make check        # go fix + vet + gofmt + test (verify flow)
make test         # run unit tests
```

`make release` cross-builds `linefire` for macOS (arm64/amd64) and Windows (amd64)
with the data embedded and the version stamped in (`linefire -version`). Linux needs
cgo for Ebitengine (X11/OpenGL), so its release binary is built on a Linux runner
rather than cross-compiled.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full verify flow and
conventions. `TODO.md` (Portuguese) tracks the roadmap; `docs/campaign.md` is
the living campaign plan.
