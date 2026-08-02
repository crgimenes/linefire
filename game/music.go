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

	"github.com/crgimenes/linefire/filoio"
	"github.com/crgimenes/linefire/sfx"
)

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
	if b.musicKey == key && b.onAir() {
		return // already on air (a finished one-shot is NOT playing, so it restarts below)
	}
	b.stopMusic()
	if b.silent() {
		return
	}
	// Web: MP3 themes go to an HTMLAudioElement so the browser decodes them off
	// the wasm main thread (see musicweb_js.go). gion tracks fall through to oto.
	if webAudioMusic && sfx.IsMusicFile(name) {
		data := b.fileBytes(name)
		if data == nil {
			return
		}
		b.webMusic = newWebTrack(name, data, loop, b.master*musicVolume)
		b.musicKey, b.musicName = key, name
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
	b.musicKey, b.musicName = key, name
}

// onAir reports the current track still holds the air: an oto player that is
// playing, or a web track that has not ended. A web track stuck behind the
// browser's autoplay gate COUNTS as on air — poke retries it; recreating the
// element every frame would not get it playing any sooner.
func (b *soundBank) onAir() bool {
	if b.webMusic != nil {
		return !b.webMusic.done()
	}
	return b.music != nil && b.music.IsPlaying()
}

// stopMusic silences the soundtrack.
func (b *soundBank) stopMusic() {
	if b == nil {
		return
	}
	b.webMusic.stop()
	b.webMusic = nil
	b.musicKey, b.musicName = "", ""
	if b.music == nil {
		return
	}
	_ = b.music.Close()
	b.music = nil
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
// The rule (crg's spec): the music NEVER goes silent. A screen with its own theme
// takes the air (combat themes over stage themes, as before); a screen WITHOUT one
// leaves whatever song is playing alone, and when a song ends the jukebox draws the
// next random track — crossing themeless screens just chains random songs until a
// themed one takes over.
func (g *Game) updateMusic() {
	// Skirmish has NO soundtrack (crg). The songs belong to Linefire's arcade
	// cabinet, not to a battle sitting on somebody's desktop while they work.
	// What sound buys here is the fighting itself: shots, hits, blasts.
	if g.skirmishMode {
		return
	}
	// The attract demo runs the jukebox directly: full, non-looping random tracks —
	// never the combat themes (it is always fighting, which would drown them).
	if g.creditsMode {
		g.sfx.stepJukebox()
		return
	}
	mood, seed, ok := g.desiredMusic()
	if ok {
		g.sfx.playMusic(mood, seed, true)
		return
	}
	g.sfx.stepJukebox()
}

// stepJukebox keeps a song on the air when no authored theme claims it: an mp3
// already playing (a previous screen's theme, or an earlier pick) plays on; a
// synthesized combat sting whose fight is over is dropped; and once nothing is
// playing, the next random track starts. It lives on the BANK — the one object
// every world reset carries — so a map change or attract backdrop swap never
// restarts or drops the song mid-play.
func (b *soundBank) stepJukebox() {
	if b == nil || b.silent() {
		return
	}
	if b.onAir() {
		if sfx.IsMusicFile(b.musicName) {
			return // a song holds the air until it ends on its own
		}
		b.stopMusic() // a gion (combat) theme lost its claim: hand the air to the jukebox
	}
	b.jukeTrack = b.nextJukeTrack(b.jukeTrack)
	if b.jukeTrack == "" {
		return // no music/ directory (e.g. headless tests) — stay silent
	}
	b.playMusic(b.jukeTrack, 0, false)
}

// nextJukeTrack picks a random track, avoiding an immediate repeat of cur when more
// than one is available. Uses the auto-seeded global RNG, so the rotation varies per
// launch. "" when there is no music/ directory.
func (b *soundBank) nextJukeTrack(cur string) string {
	tracks := tracksFromDir(b.content, "music")
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
