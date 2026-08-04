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

	"github.com/crgimenes/linefire"
	"github.com/crgimenes/linefire/config"
	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/game"
)

// Version is stamped at build time by the shared release script
// (-ldflags "-X main.Version=<tag>"). It defaults to "dev" for a plain `go build`, so a
// bug report can always name the build it came from. It lives in main, exported, because
// that one name is what the release script can reach in EVERY project without knowing any
// module path; the value is handed to config.Version below so the rest of the program can
// read it without importing main.
var Version = "dev"

func main() {
	log.SetFlags(0)
	config.Version = Version
	dir := flag.String("dir", "", "read game data from this directory instead of the embedded bundle")
	mapDir := flag.String("mapdir", "gameassets", "maps/assets subdirectory within the game data")
	startMap := flag.String("map", "map0001", "map file stem to open (for playtesting a specific stage)")
	showVersion := flag.Bool("version", false, "print the build version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("linefire", config.BuildString())
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
