# Contributing to Linefire

Linefire is a Go project for vector games with a CRT-glow look: strong, bright
lines drawn with `ebiten/v2/vector` and lit by a glow shader. It contains the
editors (asset and map, unified in `linefire-edit`) and the game runtime; the
editors deliberately use the same packages the game uses, so the rendering
pipeline is validated early.

The philosophy, in three lines (shared with the `eprojects`/`devengine` family):

- **Stdlib first.** A new dependency needs a strong reason. The non-ecosystem
  runtime dependencies are Ebitengine and the own-family libraries
  (`crgimenes/minigui` for the immediate-mode UI); logging uses
  `github.com/crgimenes/devengine/log`, as required by the shared agent rules.
- **Naturally light.** No speculative caching or cleverness. Any optimization
  must prove itself with a benchmark before it lands.
- **Teaching example.** Clarity beats brevity.

## Repository layout

```
linefire/
├── cmd/                   entrypoints: linefire (game), linefire-edit,
│                          linefire-editor
├── asset/                 asset model, load, save, validation
├── level/                 stage model, load, save, validation
├── game/                  game runtime (physics, combat, enemies, camera)
├── editor/                asset editor (state, input, tools, drawing)
├── mapeditor/             map editor (state, input, tools, drawing)
├── editapp/               unified editor shell hosting both editors
├── editorkit/             shared camera, dot grid, generic undo history
├── procgen/               procedural chunked-map generation
└── render/                shared vector renderer, glow and CRT shaders
```

`render` is the seam the future game shares with the editor; keep it free of
editor-only concerns.

## The verify flow

Run before considering any change done. All must come back clean:

```sh
go fix ./...
go fix -inline ./...
go vet ./...
gofmt -l .          # must print nothing
go test ./...
staticcheck ./...   # when available
golangci-lint run   # gocyclo threshold 30, tests exempt
gosec ./...
```

`make check` runs fix + fix-inline + vet + fmt + test in one go.

## Conventions

- **Code is US English.** Identifiers and comments. This file and `TODO.md` are
  in Portuguese; code is not.
- **Error handling on separate lines.** Assignment, then check:

  ```go
  err := doThing()
  if err != nil {
      return err
  }
  ```

  Not `if err := doThing(); err != nil`. The comma-ok idiom
  (`if v, ok := m[k]; ok`) is fine; the rule is about error returns. Test files
  may use the inline form freely.
- **Function length is not a metric** (Go is verbose); cyclomatic complexity is,
  enforced by golangci-lint (`gocyclo`, threshold 30).
- **Typed model, no `map[string]any`** for the asset format. The JSON schema is
  versioned; changing it bumps `asset.CurrentVersion` and keeps old files
  loadable when reasonable.
- **Validate at the boundary.** Loading and saving always run `asset.Validate`;
  reject NaN/Inf and malformed paths with a clear error rather than failing
  silently.
- **Build with `-trimpath`.** `CGO_ENABLED=0` is enough on macOS and Windows;
  Linux needs cgo for Ebitengine.

## Testing

- Table-driven tests live next to the code they cover. The asset round-trip and
  color parsing are the current safety net; extend them when the model grows.
- Visual/render changes that cannot be asserted headlessly should at least be
  smoke-run (`make run`) and described in the PR.
