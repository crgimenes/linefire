// Command linefire-svg2map imports an SVG drawing as a Linefire map's walls: draw the level
// geometry in Inkscape/Figma/Illustrator, run this, and hand-finish the header and spawns in
// the emitted .lfm. Shapes map 1:1 to world coordinates (y-down, same as the game), so what
// you draw is where it lands. Flatten transforms in the editor before exporting.
//
//	linefire-svg2map [-scale s] [-title t] level.svg [level.lfm]
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
	"linefire/svgimport"
)

func main() {
	scale := flag.Float64("scale", 1, "multiply every SVG coordinate by this (world units per SVG unit)")
	title := flag.String("title", "", "human-facing stage title (defaults to the file name)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: linefire-svg2map [-scale s] [-title t] input.svg [output.lfm]")
		fmt.Fprintln(os.Stderr, "Imports SVG shapes as map walls (1 SVG unit = 1 world unit, y-down).")
		fmt.Fprintln(os.Stderr, "Flatten transforms before exporting; add spawns/music by hand after.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	in := flag.Arg(0)
	out := flag.Arg(1)
	if out == "" {
		out = strings.TrimSuffix(in, filepath.Ext(in)) + ".lfm"
	}
	n, err := run(in, out, *scale, *title)
	if err != nil {
		fmt.Fprintln(os.Stderr, "linefire-svg2map:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d wall paths)\n", out, n)
}

// run reads the SVG at in, converts its shapes to wall paths, and writes a map to out.
// Returns the number of wall paths imported.
func run(in, out string, scale float64, title string) (int, error) {
	data, err := os.ReadFile(in) // #nosec G304 -- a CLI tool reading the file the user named is the point
	if err != nil {
		return 0, err
	}
	paths, err := svgimport.Paths(data)
	if err != nil {
		return 0, err
	}
	scalePaths(paths, scale)
	snapPaths(paths) // walls are coarse: round off the curve-flattening float noise

	lvl := level.New()
	lvl.Name = strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))
	lvl.Title = title
	minX, minY, maxX, maxY := bbox(paths)
	lvl.Size = asset.Size{W: math.Ceil(maxX), H: math.Ceil(maxY)}
	lvl.PlayerStart = level.Start{X: math.Round((minX + maxX) / 2), Y: math.Round((minY + maxY) / 2), Angle: -90}
	lvl.Walls[0].Paths = paths

	err = filoio.SaveLevel(out, lvl)
	if err != nil {
		return 0, err
	}
	return len(paths), nil
}

// scalePaths multiplies every command coordinate by s (a no-op at 1).
func scalePaths(paths []asset.Path, s float64) {
	if s == 1 {
		return
	}
	for pi := range paths {
		for ci := range paths[pi].Commands {
			paths[pi].Commands[ci].X *= s
			paths[pi].Commands[ci].Y *= s
		}
	}
}

// snapPaths rounds every command coordinate to the nearest world unit, matching the
// hand-authored maps and trimming the many-digit float noise curve flattening leaves behind.
func snapPaths(paths []asset.Path) {
	for pi := range paths {
		for ci := range paths[pi].Commands {
			paths[pi].Commands[ci].X = math.Round(paths[pi].Commands[ci].X)
			paths[pi].Commands[ci].Y = math.Round(paths[pi].Commands[ci].Y)
		}
	}
}

// bbox is the bounding box of every path endpoint (close commands carry no coordinate).
func bbox(paths []asset.Path) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for pi := range paths {
		for _, c := range paths[pi].Commands {
			if c.Op == asset.OpClose {
				continue
			}
			minX, maxX = math.Min(minX, c.X), math.Max(maxX, c.X)
			minY, maxY = math.Min(minY, c.Y), math.Max(maxY, c.Y)
		}
	}
	return minX, minY, maxX, maxY
}
