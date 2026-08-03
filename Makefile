# linefire — vector asset/map editor + game runtime (CLIs under cmd/)
#
# Targets:
#   make build       — build every cmd/<name> for the current OS/arch into bin/
#   make build-cross — also build for linux/amd64
#   make release     — publish a release through the shared ./release.sh
#   make icons       — redraw the application icon from the game's own art
#   make wasm        — the web build into web/ (linefire.wasm + wasm_exec.js)
#   make serve-web   — build wasm and serve web/ locally for a browser test
#   make run         — run the standalone asset editor (GUI)
#   make run-edit    — run the unified map/asset editor (GUI)
#   make run-game    — run the game runtime (GUI)
#   make check       — go fix + go fix -inline + go vet + gofmt + go test
#   make fmt         — report files that are not gofmt-clean
#   make security    — gosec ./...
#   make tidy        — go mod tidy
#   make clean       — remove bin/

export CGO_ENABLED=0
BUILD_FLAGS := -trimpath -ldflags "-s -w"
RUN_FLAGS := -trimpath

CMDS   := $(notdir $(wildcard cmd/*))
GOOS   := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# The game carries its data in the binary (go:embed), so every build runs
# standalone. VERSION is stamped into it (linefire -version) for local builds;
# the release stamps its own from the tag. Every target is pure Go — no cgo on
# any platform, Linux included, since Ebitengine v2.10 — so one host builds
# them all and the release needs no runner.
VERSION         := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
RELEASE_LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build build-cross release icons wasm serve-web run run-edit run-game check fix fix-inline vet fmt test security tidy clean

all: build

build:
	@mkdir -p bin
	@for cmd in $(CMDS); do \
	  echo "  -> $$cmd ($(GOOS)-$(GOARCH))"; \
	  go build $(BUILD_FLAGS) -o bin/$$cmd-$(GOOS)-$(GOARCH) ./cmd/$$cmd ; \
	done

build-cross: build
	@for cmd in $(CMDS); do \
	  GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o bin/$$cmd-linux-amd64 ./cmd/$$cmd ; \
	  echo "  -> $$cmd (linux-amd64)"; \
	done

# The web build: what the Pages workflow ships. wasm_exec.js must come from the
# SAME toolchain that built the module, hence the copy from this GOROOT.
wasm:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o web/linefire.wasm ./cmd/linefire
	@cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
	@ls -lh web/linefire.wasm | awk '{print "  -> web/linefire.wasm ("$$5")"}'

serve-web: wasm
	@echo "  -> http://localhost:8080/  (game at /play.html)"
	@cd web && python3 -m http.server 8080

# The shipped release is the SHARED script (~/Projects/scripts/release.sh,
# linked here as ./release.sh): every project in the house releases the same
# way, so the flow is fixed in one place instead of drifting per repository.
# It cross-compiles every target locally with CGO_ENABLED=0 — no cgo anywhere,
# which Ebitengine v2.10 made true for the game projects too — builds the
# signed and notarized universal macOS app from assets/linefire.icns, publishes
# the GitHub release and updates the Homebrew cask. It needs a clean worktree
# and a tag pointing at HEAD.
release:
	./release.sh

# The application icon, drawn from the game's own art (icon/): the .iconset for
# iconutil, the .icns the release bundles, and a PNG to look at.
icons:
	go run assets/genicon.go
	iconutil -c icns assets/linefire.iconset -o assets/linefire.icns
	@ls -lh assets/linefire.icns assets/linefire.png

run:
	go run $(RUN_FLAGS) ./cmd/linefire-editor

run-edit:
	go run $(RUN_FLAGS) ./cmd/linefire-edit -assets ./gameassets

run-game:
	go run $(RUN_FLAGS) ./cmd/linefire

check: fix fix-inline vet fmt test

fix:
	go fix ./...

fix-inline:
	go fix -inline ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

test:
	go test ./...

security:
	gosec ./...

tidy:
	go mod tidy

clean:
	rm -rf bin
