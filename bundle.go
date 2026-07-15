// Package linefire embeds the shipped game data so the executable runs standalone,
// with no external files. The embed directive can only reach files under its own
// directory, which is why this lives at the module root beside gameassets/ and
// music/ rather than in cmd/linefire.
package linefire

import (
	"embed"
	"io/fs"
)

//go:embed gameassets music
var bundle embed.FS

// Content is the embedded game data: gameassets/ (player, enemies, power-ups and the
// map*.lfm campaign) and music/ (the MP3 themes). Names resolve the same way they do
// on disk — "gameassets/map0001.lfm", "music/One_Heart_Remaining.mp3" — so the game's
// fs.FS reads are identical whether they hit this bundle or a directory.
func Content() fs.FS { return bundle }
