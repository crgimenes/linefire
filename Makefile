# linefire — vector asset/map editor + game runtime (CLIs under cmd/)
#
# Targets:
#   make build       — build every cmd/<name> for the current OS/arch into bin/
#   make build-cross — also build for linux/amd64
#   make release     — standalone game binaries (embedded data) for the desktop matrix
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

# Release: the game carries its data in the binary (go:embed), so these run standalone.
# VERSION is stamped into the binary (linefire -version). The desktop matrix below builds
# pure-Go from any host; linux needs cgo for ebiten (X11/OpenGL), so it is built on a linux
# runner in CI rather than cross-compiled here.
VERSION         := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
RELEASE_LDFLAGS := -s -w -X main.version=$(VERSION)
RELEASE_TARGETS := darwin/arm64 darwin/amd64 windows/amd64

.PHONY: all build build-cross release run run-edit run-game check fix fix-inline vet fmt test security tidy clean

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

release:
	@mkdir -p bin
	@for t in $(RELEASE_TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
	  echo "  -> linefire-$$os-$$arch$$ext ($(VERSION))"; \
	  GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" \
	    -o bin/linefire-$$os-$$arch$$ext ./cmd/linefire || exit 1 ; \
	done

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
