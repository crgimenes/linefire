// Command linefire is the Linefire game runtime. It loads the player asset and the
// opening map from the game data and runs the game.
//
// By default the data is read from the bundle embedded in the binary, so the
// executable runs standalone with no external files. Pass -dir to read from a
// directory instead (dev iteration, or community content): the directory holds
// gameassets/ and music/ the same way the repository root does. -mapdir and -map
// pick which subdirectory and map to open, so the map editor's playtest key can
// launch the game straight onto the stage being edited.
//
// Usage:
//
//	linefire                       run the campaign from the embedded game data
//	linefire -dir .                read gameassets/ and music/ from the current directory
//	linefire -dir . -map map0005   start the campaign on a specific stage (playtest)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/crgimenes/devengine/log"

	"linefire"
	"linefire/filoio"
	"linefire/game"
)

// version is stamped at build time (see the release target: -X main.version=...).
// It defaults to "dev" for a plain `go build`, so a bug report can name the build.
var version = "dev"

func main() {
	log.SetFlags(0)
	dir := flag.String("dir", "", "read game data from this directory instead of the embedded bundle")
	mapDir := flag.String("mapdir", "gameassets", "maps/assets subdirectory within the game data")
	startMap := flag.String("map", "map0001", "map file stem to open (for playtesting a specific stage)")
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("linefire", version)
		return
	}

	content := linefire.Content()
	if *dir != "" {
		content = os.DirFS(*dir)
	}

	player, err := filoio.LoadAssetFS(content, *mapDir, "player")
	if err != nil {
		log.Fatalf("linefire: %v", err)
	}
	lvl, err := filoio.LoadLevelFS(content, *mapDir, *startMap)
	if err != nil {
		log.Fatalf("linefire: %v", err)
	}

	err = game.Run(content, player, lvl, *mapDir, *startMap)
	if err != nil {
		log.Fatalf("linefire: %v", err)
	}
}
