// Package weapon is Linefire's weapon catalogue: what each weapon does, how
// often it can be used, what its shots look like and what they cost the target.
//
// It holds the DEFINITIONS, not the firing. How a shot is spawned, aimed and
// resolved depends on the world it is fired into — a cave with walls, or a
// desktop overlay — so that stays with each game. What must not differ is the
// weapon itself: a missile has to hit as hard and reload as slowly in Linefire
// Skirmish as it does here, or the two are different games wearing the same art.
//
// The numbers are in Linefire's units: world units, frames at 60 TPS, and hull
// points. A consumer on a different scale should convert them rather than
// reinterpret them.
package weapon

import "image/color"

// Kind is how a weapon delivers damage.
type Kind int

const (
	KindProjectile Kind = iota // a bullet or missile (AOE > 0 makes it explode)
	KindMine                   // a deployed proximity mine
	KindLaser                  // a continuous beam that pierces everything it crosses
	KindDevourer               // a deployed black hole: arms, drags everything in, collapses
)

// Aim is how a weapon is pointed when used.
type Aim int

const (
	AimForward Aim = iota // along the ship heading, from the gun muzzle
	AimCursor             // toward a chosen point, from the ship centre
	AimDrop               // no direction: placed at the ship (mines)
)

// Sound names the gion recipe a weapon plays when it fires. An empty Base is
// silent, which is what the laser wants — it hums on a loop instead.
type Sound struct {
	Base   string
	Seed   int64
	Volume float64
}

// Weapon is a data-driven weapon definition. The same struct describes the gun,
// the missile and the mine: a new weapon is new data plus a case in whatever
// fires it, not a new fire function.
type Weapon struct {
	Name     string
	Kind     Kind
	Aim      Aim
	Cooldown int // frames between uses

	Damage int        // hull damage (centre damage for an AOE weapon)
	Col    color.RGBA // floating damage-number colour
	Fire   Sound

	// Projectile.
	Speed     float64
	Life      int
	AOE       float64    // explosion radius; 0 = direct hit
	Core      color.RGBA // shot core colour
	Glow      color.RGBA // shot glow colour
	Width     float64    // crisp stroke width, logical px
	GlowWidth float64    // glow stroke width, logical px

	// Mine.
	ArmFrames     int
	TriggerRadius float64
	MaxLive       int // cap on this weapon's live mines

	// Laser.
	Reach float64 // max beam length, world units
}

// Catalogue indices: which weapon TYPE, used to key an arsenal or a pickup.
const (
	CatFront = iota
	CatMissile
	CatMine
	CatLaser
	CatDevourer
)

// Catalog is every weapon that can be collected and mounted.
var Catalog = []Weapon{
	CatFront: {
		Name: "Front Gun", Kind: KindProjectile, Aim: AimForward, Cooldown: 8,
		Damage: 1, Col: color.RGBA{0xff, 0xf0, 0xc0, 0xff},
		Fire:  Sound{"shot_soft", 101, 0.3}, // low, dark pu — bursty, kept quiet so it will not mask the music
		Speed: 9.0, Life: 90,
		Core: color.RGBA{0xe8, 0xff, 0xff, 0xff}, Glow: color.RGBA{0x80, 0xff, 0xff, 0xff},
		Width: 1.6, GlowWidth: 2.0,
	},
	CatMissile: {
		Name: "Missile", Kind: KindProjectile, Aim: AimCursor, Cooldown: 45,
		Damage: 4, Col: color.RGBA{0xff, 0xa0, 0x40, 0xff},
		Fire:  Sound{"shot_launch", 130, 0.5}, // low rising launch whoosh
		Speed: 7.0, Life: 110, AOE: 70.0,
		Core: color.RGBA{0xff, 0xa0, 0x40, 0xff}, Glow: color.RGBA{0xff, 0x60, 0x20, 0xff},
		Width: 2.4, GlowWidth: 5.0,
	},
	CatMine: {
		Name: "Mine", Kind: KindMine, Aim: AimDrop, Cooldown: 16, // spaces out a held deploy
		Damage: 5, Col: color.RGBA{0xff, 0x80, 0x30, 0xff},
		Fire:      Sound{"shot_thunk", 140, 0.45}, // dull low deploy thunk
		AOE:       64.0,
		ArmFrames: 45, TriggerRadius: 36.0, MaxLive: 5,
	},
	CatLaser: {
		Name: "Laser", Kind: KindLaser, Aim: AimCursor, Cooldown: 4,
		Damage: 1, Col: color.RGBA{0x90, 0xff, 0x90, 0xff},
		Fire: Sound{"beam", 150, 0.3}, // a loop recipe: hums while the beam is held
		// The raymarch stops at the first rock, so a long reach is cheap: this
		// crosses the screen and lands on a wall.
		Reach: 2000,
	},
	CatDevourer: {
		Name: "Devourer", Kind: KindDevourer, Aim: AimDrop, Cooldown: 90,
		Damage: 300, Col: color.RGBA{0xc0, 0x40, 0xff, 0xff},
		Fire: Sound{"shot_thunk", 70, 0.7}, // a deep, ominous drop
	},
}

// For returns a weapon by catalogue index, or the front gun for an index that is
// not in the catalogue — every ship can at least shoot.
func For(cat int) Weapon {
	if cat < 0 || cat >= len(Catalog) {
		return Catalog[CatFront]
	}
	return Catalog[cat]
}
