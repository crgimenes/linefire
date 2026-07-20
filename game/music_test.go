package game

import (
	"os"
	"testing"

	"linefire/asset"
	"linefire/filoio"
	"linefire/level"
)

// Track/IsMusicFile synthesis is tested in the sfx package; here we test the
// game-side selection and the file playback path.

// themedEnemy is an engaged enemy whose asset declares a combat theme.
func themedEnemy(hp int, mood string, seed int64) entity {
	a := asset.New()
	a.Sounds = []asset.Sound{{Event: "theme", Base: mood, Seed: seed, Continuous: true}}
	return entity{kind: kindEnemy, combat: true, hp: hp, a: a}
}

func TestDesiredMusicPrefersStrongestEngagedTheme(t *testing.T) {
	lvl := level.New()
	lvl.Music = "dark"
	lvl.MusicSeed = 3
	g := New(asset.New(), lvl, "", false)

	// No engagement: the stage theme plays.
	mood, seed, ok := g.desiredMusic()
	if !ok || mood != "dark" || seed != 3 {
		t.Fatalf("idle stage should play its theme, got %q/%d ok=%v", mood, seed, ok)
	}

	// Two themed enemies engaged: the strongest one wins.
	g.entities = []entity{
		themedEnemy(2, "battle", 7),
		themedEnemy(10, "boss", 9),
		{kind: kindEnemy, combat: true, hp: 99, a: asset.New()}, // strongest, but no theme
	}
	mood, seed, ok = g.desiredMusic()
	if !ok || mood != "boss" || seed != 9 {
		t.Fatalf("the strongest THEMED enemy should win, got %q/%d ok=%v", mood, seed, ok)
	}

	// Evaded (combat dropped): back to the stage theme.
	for i := range g.entities {
		g.entities[i].combat = false
	}
	mood, _, ok = g.desiredMusic()
	if !ok || mood != "dark" {
		t.Fatalf("evading should bring the stage theme back, got %q ok=%v", mood, ok)
	}

	// No stage theme and nothing engaged: silence.
	g.level.Music = ""
	if _, _, ok = g.desiredMusic(); ok {
		t.Fatal("no theme anywhere should be silence")
	}
}

func TestMusicLoopDecodesStageFile(t *testing.T) {
	const path = "../music/One_Heart_Remaining.mp3"
	_, err := os.Stat(path)
	if err != nil {
		t.Skipf("stage song not present: %v", err)
	}
	b := &soundBank{} // no audio context needed: decoding is pure
	loop := b.musicSource(path, 0, true)
	if loop == nil {
		t.Fatal("the stage mp3 should decode into a loop")
	}
	if b.musicSource(path, 0, false) == nil {
		t.Fatal("the stage mp3 should also resolve as a one-shot source")
	}
	if b.musicSource("missing/nope.mp3", 0, true) != nil {
		t.Fatal("a missing file should resolve to silence, not a source")
	}
}

func TestMusicNilBankIsSafe(t *testing.T) {
	var b *soundBank
	b.playMusic("dark", 1, true) // must not panic
	b.stopMusic()

	g := &Game{level: level.New()} // nil sfx
	g.updateMusic()
	g.prewarmMusic()
}

// TestJukeboxKeepsPicking: with no authored theme claiming the air and nothing
// playing, the jukebox draws random tracks from the bank's content, never
// repeating the current pick back-to-back — crossing themeless screens chains
// songs instead of going silent. Nil and muted banks stay safe no-ops.
func TestJukeboxKeepsPicking(t *testing.T) {
	var nilBank *soundBank
	nilBank.stepJukebox() // must not panic

	root := t.TempDir()
	if err := os.MkdirAll(root+"/music", 0o750); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.mp3", "b.mp3", "c.mp3"} {
		if err := os.WriteFile(root+"/music/"+n, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := &soundBank{content: os.DirFS(root)}

	b.muted = true
	b.stepJukebox()
	if b.jukeTrack != "" {
		t.Fatal("a muted bank should not spin the jukebox")
	}

	b.muted = false
	b.stepJukebox()
	if b.jukeTrack == "" {
		t.Fatal("the jukebox should pick a track when nothing is playing")
	}
	// Nothing actually plays headless (the fake mp3 fails to decode), so every
	// step re-picks — and the pick must never repeat the current track.
	for range 20 {
		cur := b.jukeTrack
		b.stepJukebox()
		if b.jukeTrack == cur {
			t.Fatalf("the jukebox repeated %q back-to-back", cur)
		}
	}
}

// TestUpdateMusicFallsBackToJukebox: a screen with no theme of its own hands the
// air to the jukebox instead of stopping the music (the old behavior).
func TestUpdateMusicFallsBackToJukebox(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/music", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/music/only.mp3", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &Game{level: level.New(), sfx: &soundBank{content: os.DirFS(root)}}
	g.updateMusic()
	if g.sfx.jukeTrack != "music/only.mp3" {
		t.Fatalf("a themeless screen should start the jukebox, got %q", g.sfx.jukeTrack)
	}
}

// TestTracksFromDir: only .mp3 files are listed (case-insensitive), dir-prefixed and sorted; a
// missing directory yields nil, so a procedural stage falls back to silence instead of crashing.
func TestTracksFromDir(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b_track.mp3", "a_track.MP3", "notes.txt"} {
		if err := os.WriteFile(dir+"/"+n, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got := tracksFromDir(filoio.OSFS(), dir)
	if len(got) != 2 {
		t.Fatalf("should list the two mp3s (case-insensitive), got %v", got)
	}
	if got[0] != dir+"/a_track.MP3" || got[1] != dir+"/b_track.mp3" {
		t.Fatalf("tracks should be dir-prefixed and sorted, got %v", got)
	}
	if tracksFromDir(filoio.OSFS(), dir+"/nope") != nil {
		t.Fatal("a missing directory should yield nil, not crash")
	}
}
