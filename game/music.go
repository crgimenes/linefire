package game

import (
	"bytes"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"

	"github.com/crgimenes/gion"

	"linefire/filoio"
	"linefire/sfx"
)

// Procedural stages (bonus caves, edge rooms) carry no authored (music ...), so they used to run
// silent. Instead we hand them a random MP3 from the music/ directory, resolved the same way an
// authored level's (music "music/foo.mp3") is — through the content FS, so the embedded bundle
// and a directory on disk both work.

// musicTracks lists the available MP3 theme paths under the content's music/ directory, sorted for
// a stable order (empty when it is absent — e.g. headless tests — so callers leave the stage silent).
func (g *Game) musicTracks() []string {
	return tracksFromDir(g.content, "music")
}

// tracksFromDir returns the ".mp3" files in dir as "<dir>/<name>" theme paths, sorted for a
// stable order. nil when the directory is unreadable.
func tracksFromDir(fsys fs.FS, dir string) []string {
	if fsys == nil {
		fsys = filoio.OSFS()
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mp3") {
			out = append(out, dir+"/"+e.Name())
		}
	}
	slices.Sort(out)
	return out
}

// randomTrack picks a random MP3 theme for a procedural stage, or "" if none are available.
func (g *Game) randomTrack() string {
	tracks := g.musicTracks()
	if len(tracks) == 0 {
		return ""
	}
	if g.rng == nil {
		return tracks[0]
	}
	return tracks[g.rng.IntN(len(tracks))]
}

// Theme music. A theme names either an MP3 FILE (a path ending in .mp3 — produced
// outside, e.g. AI-generated stage songs) or a gion music mood, synthesized on the
// fly (see sfx.Track — lead muted). The level declares the ambient theme
// (level.Music/MusicSeed) and an enemy declares a combat theme on its asset (a
// "theme" sound). While any themed enemy is engaged the strongest one's theme
// plays; evade or destroy them all and the stage theme comes back. Everything
// loops; gion tracks are pre-rendered and files are pre-read when a map is built,
// so a switch never hitches a fight.

// musicVolume sits the soundtrack under the effects.
const musicVolume = 0.4

// trackPCM returns (and caches) the rendered track for a gion mood, sharing the
// bank's rendered cache under a "music:" key.
func (b *soundBank) trackPCM(mood string, seed int64) []byte {
	key := "music:" + soundKey(mood, seed)
	if vs, ok := b.rendered[key]; ok {
		if len(vs) == 0 {
			return nil
		}
		return vs[0]
	}
	pcm := sfx.Track(mood, seed)
	if pcm == nil {
		b.rendered[key] = nil
		return nil
	}
	b.rendered[key] = [][]byte{pcm}
	return pcm
}

// fileBytes returns (and caches) a music file's raw bytes; nil when unreadable (the
// game plays on in silence). The compressed bytes stay cached — decoding streams
// during playback, so no multi-minute PCM buffer is ever built.
func (b *soundBank) fileBytes(path string) []byte {
	if data, ok := b.files[path]; ok {
		return data
	}
	content := b.content
	if content == nil {
		content = filoio.OSFS()
	}
	data, err := fs.ReadFile(content, path)
	if err != nil {
		data = nil
	}
	if b.files == nil {
		b.files = map[string][]byte{}
	}
	b.files[path] = data
	return data
}

// musicSource builds the playback source for a theme: a decoded audio file, or a pre-rendered
// gion track. loop wraps it in an InfiniteLoop (the stage/combat default); without it the track
// plays through ONCE (the attract demo, which advances to a new random song on the end). nil when
// the theme cannot be resolved.
func (b *soundBank) musicSource(name string, seed int64, loop bool) io.Reader {
	if sfx.IsMusicFile(name) {
		data := b.fileBytes(name)
		if data == nil {
			return nil
		}
		stream, err := mp3.DecodeWithSampleRate(gion.DefaultRate, bytes.NewReader(data))
		if err != nil {
			return nil
		}
		if !loop {
			return stream
		}
		return audio.NewInfiniteLoop(stream, stream.Length())
	}
	pcm := b.trackPCM(name, seed)
	if pcm == nil {
		return nil
	}
	if !loop {
		return bytes.NewReader(pcm)
	}
	return audio.NewInfiniteLoop(bytes.NewReader(pcm), int64(len(pcm)))
}

// playMusic switches the soundtrack to the requested theme (file path or gion mood), keeping it
// when it is already the one playing. loop keeps it looping (the default); pass false for a
// one-shot track. Nil-safe and silent when muted or the theme is unknown.
func (b *soundBank) playMusic(name string, seed int64, loop bool) {
	if b == nil || webFlag("nomusic") {
		return // ?nomusic (web A/B): SFX stay, but no track — no streaming mp3 decode on the main thread
	}
	key := "music:" + soundKey(name, seed)
	if b.musicKey == key && b.music != nil && b.music.IsPlaying() {
		return // already on air (a finished one-shot is NOT playing, so it restarts below)
	}
	b.stopMusic()
	if b.silent() {
		return
	}
	src := b.musicSource(name, seed, loop)
	if src == nil {
		return
	}
	player, err := b.ctx.NewPlayer(src)
	if err != nil {
		return
	}
	player.SetVolume(b.master * musicVolume)
	player.Play()
	b.music = player
	b.musicKey = key
}

// musicDone reports that a one-shot track has played through to its end. A looping track never
// finishes, so this only ever fires for the attract demo's non-looping songs.
func (b *soundBank) musicDone() bool {
	return b != nil && b.music != nil && !b.music.IsPlaying()
}

// stopMusic silences the soundtrack.
func (b *soundBank) stopMusic() {
	if b == nil || b.music == nil {
		return
	}
	_ = b.music.Close()
	b.music = nil
	b.musicKey = ""
}

// desiredMusic picks this frame's track: the strongest engaged enemy that declares
// a combat theme wins (the boss over its escorts); with no themed enemy engaged,
// the stage's ambient theme plays; with neither, silence.
func (g *Game) desiredMusic() (string, int64, bool) {
	best := -1
	var mood string
	var seed int64
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy || !e.combat || e.hp <= best {
			continue
		}
		req, ok := resolveEvent(e.a, "theme")
		if !ok {
			continue
		}
		best, mood, seed = e.hp, req.base, req.seed
	}
	if best >= 0 {
		return mood, seed, true
	}
	if g.level != nil && g.level.Music != "" {
		return g.level.Music, g.level.MusicSeed, true
	}
	return "", 0, false
}

// updateMusic drives the soundtrack from the combat state. Called every live frame;
// track switches are cheap because the PCM is cached.
func (g *Game) updateMusic() {
	// The attract demo runs its own soundtrack: full, non-looping random tracks that play out and
	// only THEN advance — never the combat themes (it is always fighting, which would drown them).
	if g.creditsMode {
		g.updateCreditsMusic()
		return
	}
	mood, seed, ok := g.desiredMusic()
	if !ok {
		g.sfx.stopMusic()
		return
	}
	g.sfx.playMusic(mood, seed, true)
}

// updateCreditsMusic keeps a full random track playing under the attract demo: it plays through
// ONCE, and only when it ends is another random track picked (so the 20s backdrop swap never cuts
// the music). g.creditsTrack survives the swap; g.rng varies the pick across the run.
func (g *Game) updateCreditsMusic() {
	if g.sfx == nil {
		return
	}
	if g.creditsTrack == "" || g.sfx.musicDone() {
		g.creditsTrack = g.nextCreditsTrack(g.creditsTrack)
	}
	if g.creditsTrack == "" {
		return // no music/ directory (e.g. headless tests) — stay silent
	}
	g.sfx.playMusic(g.creditsTrack, 0, false)
}

// nextCreditsTrack picks a random attract track, avoiding an immediate repeat of cur when more
// than one is available. Uses the auto-seeded global RNG (not the deterministic loot RNG), so the
// attract soundtrack varies per launch. "" when there is no music/ directory.
func (g *Game) nextCreditsTrack(cur string) string {
	tracks := g.musicTracks()
	if len(tracks) == 0 {
		return ""
	}
	if len(tracks) == 1 {
		return tracks[0]
	}
	for {
		t := tracks[randIntN(len(tracks))]
		if t != cur {
			return t
		}
	}
}

// prewarmTheme readies a theme for instant playback: file bytes read, or the gion
// track rendered.
func (b *soundBank) prewarmTheme(name string, seed int64) {
	if webFlag("nomusic") {
		return
	}
	if sfx.IsMusicFile(name) {
		b.fileBytes(name)
		return
	}
	b.trackPCM(name, seed)
}

// prewarmMusic readies the stage theme and every present enemy's combat theme up
// front, so the first engagement does not hitch on a render or a disk read.
func (g *Game) prewarmMusic() {
	if g.sfx == nil {
		return
	}
	if g.level != nil && g.level.Music != "" {
		g.sfx.prewarmTheme(g.level.Music, g.level.MusicSeed)
	}
	for i := range g.entities {
		e := &g.entities[i]
		if e.kind != kindEnemy {
			continue
		}
		req, ok := resolveEvent(e.a, "theme")
		if ok {
			g.sfx.prewarmTheme(req.base, req.seed)
		}
	}
}
