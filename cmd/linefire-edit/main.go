// Command linefire-edit is the Linefire editor: a single window that edits stage
// (level) files and, via a mode switch, the vector assets they reference.
//
// Usage:
//
//	linefire-edit                 start a new level
//	linefire-edit level.lfm       load and edit an existing level
//	linefire-edit -assets dir     scan dir for placeable assets and maps
//
// Saving is automatic (Apple-style): the file is written when a field or the
// window loses focus, on Cmd+S, and on close. The asset list lists both assets
// and maps (double-click an asset to edit it; "New asset" creates one); placing a
// map drops a portal, and "portal exit" drops a named arrival point.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crgimenes/devengine/log"

	"linefire/editapp"
	"linefire/filoio"
	"linefire/level"
)

func main() {
	log.SetFlags(0)
	assetDir := flag.String("assets", "", "directory scanned for placeable assets and maps (default: level file's directory)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-assets dir] [level.lfm]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	var (
		doc      *level.Level
		savePath string
	)

	path := flag.Arg(0)
	if path != "" {
		loaded, err := filoio.LoadLevel(path)
		if err != nil {
			log.Fatalf("linefire-edit: %v", err)
		}
		doc = loaded
		savePath = path
		log.Printf("loaded %s", path)
	} else {
		doc = level.New()
		savePath = "" // untitled: Cmd+S opens the native save panel to name it
		log.Printf("started a new untitled map")
	}

	dir := *assetDir
	if dir == "" {
		dir = filepath.Dir(savePath)
	}

	err := editapp.Run(doc, savePath, dir)
	if err != nil {
		log.Fatalf("linefire-edit: %v", err)
	}
}
