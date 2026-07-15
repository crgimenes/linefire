package game

import (
	"io/fs"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/crgimenes/gion"

	"linefire/asset"
	"linefire/config"
	"linefire/filoio"
	"linefire/sfx"
)

// Sound is synthesized with gion (a library, not files): each effect is a small
// deterministic preset, and at play time we round-robin through several gion.Mutate
// variations so a run of shots or explosions does not sound identical, at zero asset
// cost. Sounds are DATA-DRIVEN: an event's sound comes from the acting asset's
// `sounds` block (a weapon carries its own fire sound), falling back to a per-event
// default when the asset does not declare one — so nothing is silent before
// authoring. Volume is applied per play, so tuning (and a future mute) never re-renders.
// The audio context and players exist only for the real game (created in Run), so
// headless tests never touch the device.

const soundGap = 2 // min frames between plays of the SAME sound (anti-spam)

// soundReq is a resolved sound: a gion preset base name, a seed for the base
// variation, and a play volume in (0,1) (<=0 or >=1 plays at full level).
type soundReq struct {
	base string
	seed int64
	vol  float64
}

// eventFallback is the default sound per event, used when an asset does not declare
// its own. Keeps every event audible without authoring, and lets an asset's `sounds`
// block override. Volumes are modest so the mix is not harsh (playtest: shots were
// too loud). Weapon fire sounds live on the weapon, not here.
var eventFallback = map[string]soundReq{
	"pickup":    {"collect_soft", 404, 0.7}, // low warm whoop, not the shrill pickup preset
	"hit":       {"hit", 303, 0.7},
	"destroy":   {"explosion", 220, 0.85},
	"explosion": {"explosion", 202, 0.9},
	"swap":      {"arrival_low", 505, 0.6}, // a low arrival cue that won't bury a nearby explosion
	"thruster":  {"engine", 700, 0.2},      // continuous (a loop recipe, not a preset)
	"break":     {"hit", 313, 0.5},         // a pickup shot to pieces: a crack, not a reward
}

// resolveEvent picks the sound for an event: the acting asset's declared sound if it
// has one, else the per-event fallback. ok is false for an unknown event with no
// fallback (and no asset override).
func resolveEvent(a *asset.Asset, event string) (soundReq, bool) {
	req, ok := eventFallback[event]
	if a != nil {
		for i := range a.Sounds {
			s := &a.Sounds[i]
			if s.Event != event {
				continue
			}
			if s.Muted {
				return soundReq{}, false // switched off: silence, not the fallback
			}
			if s.Base != "" {
				req.base, req.seed, ok = s.Base, s.Seed, true
			}
			if s.Volume > 0 {
				req.vol = s.Volume
			}
			break
		}
	}
	return req, ok
}

// soundBank owns the audio context, a cache of pre-rendered variations per sound
// (lazily rendered, keyed by base+seed), a per-sound throttle and round-robin
// cursor, and the live players (pruned once finished).
type soundBank struct {
	ctx      *audio.Context
	rendered map[string][][]byte
	cooldown map[string]int
	next     map[string]int
	players  []*audio.Player
	loops    map[string]*audio.Player // running continuous sounds, by loop name
	loopVol  map[string]float64       // each loop's own relative volume (for live master changes)
	music    *audio.Player            // the looping soundtrack, one track at a time
	musicKey string                   // cache key of the track on air
	files    map[string][]byte        // raw music files (mp3), by path
	content  fs.FS                    // where music files live (embedded bundle or a directory)

	// The player's audio settings (persisted via the config package). master
	// scales every play, loop and track; muted (F6 or the pause menu) silences
	// everything. cfgPath is where changes are saved ("" = no persistence).
	master  float64
	muted   bool
	cfgPath string

	// highScore is the persisted arcade HI score. It lives here, not on Game, because the
	// soundBank is the one object carried across every world reset (New / enterMap / attract
	// regen) — so the HI survives a restart the way cfgPath does, without threading it through.
	highScore int

	// demoMute silences the credits attract demo WITHOUT touching the player's saved
	// mute preference. It is transient — never persisted — so a save that fires while
	// the demo is running can never strand the game muted on the next launch (the bug
	// that muted crg's config). Playback gates read silent(), saveConfig writes muted.
	demoMute bool
}

// silent reports whether audio should be suppressed: the player's own mute, OR the
// transient credits-demo mute. Only muted is ever saved (see saveConfig).
func (b *soundBank) silent() bool {
	return b.muted || b.demoMute
}

func newSoundBank() *soundBank {
	return &soundBank{
		ctx:      audio.NewContext(gion.DefaultRate),
		rendered: map[string][][]byte{},
		cooldown: map[string]int{},
		next:     map[string]int{},
		loops:    map[string]*audio.Player{},
		loopVol:  map[string]float64{},
		master:   config.Default().Volume,
		content:  filoio.OSFS(), // Run overrides this with the game's content bundle
	}
}

// setMaster changes the master volume live: running loops and the music track are
// re-leveled on the spot; one-shots pick it up on their next play.
func (b *soundBank) setMaster(v float64) {
	if b == nil {
		return
	}
	b.master = v
	for name, p := range b.loops {
		p.SetVolume(v * b.loopVol[name])
	}
	if b.music != nil {
		b.music.SetVolume(v * musicVolume)
	}
}

// saveConfig persists the current audio settings; a missing path (no user config
// dir) or a write error just means the choice lasts for this session.
func (b *soundBank) saveConfig() {
	if b == nil || b.cfgPath == "" {
		return
	}
	_ = filoio.SaveAudio(b.cfgPath, b.master, b.muted)
}

func soundKey(base string, seed int64) string {
	return base + "/" + strconv.FormatInt(seed, 10)
}

// variationsFor returns (and caches) the pre-rendered variations for base+seed, or
// nil if base is not a known gion preset.
func (b *soundBank) variationsFor(base string, seed int64) [][]byte {
	key := soundKey(base, seed)
	if vs, ok := b.rendered[key]; ok {
		return vs
	}
	vs := renderVariations(base, seed)
	b.rendered[key] = vs
	return vs
}

// renderVariations resolves a base sound and pre-renders its variations to stereo
// PCM (see the sfx package — shared with the editors so previews match the game).
func renderVariations(base string, seed int64) [][]byte {
	return sfx.Variations(base, seed)
}

// prewarm renders the common sounds (event fallbacks + weapon fires) up front, so a
// fight does not hitch rendering them on the first shot or explosion.
func (b *soundBank) prewarm(weapons []weapon) {
	if b == nil {
		return
	}
	for _, r := range eventFallback {
		b.variationsFor(r.base, r.seed)
	}
	for i := range weapons {
		if weapons[i].fire.base != "" {
			b.variationsFor(weapons[i].fire.base, weapons[i].fire.seed)
		}
	}
}

// play starts the next variation of the requested sound at its volume, unless it is
// muted, still on its throttle, or its base is unknown. Nil-safe (tests, or an
// empty req).
func (b *soundBank) play(r soundReq) {
	if b == nil || b.silent() || r.base == "" {
		return
	}
	key := soundKey(r.base, r.seed)
	vs := b.variationsFor(r.base, r.seed)
	if len(vs) == 0 || b.cooldown[key] > 0 {
		return
	}
	pcm := vs[b.next[key]%len(vs)]
	b.next[key]++
	b.cooldown[key] = soundGap
	p := b.ctx.NewPlayerFromBytes(pcm)
	vol := b.master
	if r.vol > 0 && r.vol < 1 {
		vol *= r.vol
	}
	p.SetVolume(vol)
	p.Play()
	b.players = append(b.players, p)
}

// update ticks the per-sound throttles and closes finished players, so a busy fight
// does not leak a player per shot. Safe to call every frame; nil-safe.
func (b *soundBank) update() {
	if b == nil {
		return
	}
	for k := range b.cooldown {
		if b.cooldown[k] > 0 {
			b.cooldown[k]--
		}
	}
	live := b.players[:0]
	for _, p := range b.players {
		if p.IsPlaying() {
			live = append(live, p)
			continue
		}
		_ = p.Close()
	}
	b.players = live
}

// playEvent plays the acting asset's sound for an event, falling back to the
// per-event default. a may be nil for game-level events (an explosion, a ship swap).
func (g *Game) playEvent(a *asset.Asset, event string) {
	req, ok := resolveEvent(a, event)
	if !ok {
		return
	}
	g.sfx.play(req)
}
