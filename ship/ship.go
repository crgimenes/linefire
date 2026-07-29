// Package ship holds the profiles that make one hull fly differently from
// another: how much it takes, how far it sees, how fast it moves and shoots, and
// the range it likes to hold. The grunt is the baseline; the rest trade those
// against each other, so a fight between mixed types has texture instead of one
// ship repeated.
//
// The profiles live here rather than inside the game because Linefire Skirmish
// fields the same types, and two tables would drift into two different games.
// The numbers are in Linefire's units — world units, frames at 60 TPS, and hull
// points — so a consumer on a different scale should scale them rather than
// reinterpret them.
package ship

import "slices"

// The grunt's numbers, which the other profiles are written against and which
// the game also uses as its baseline enemy shot.
const (
	BaseHP         = 3
	BaseRadar      = 320.0 // engage/detect range, world units
	BaseFireEvery  = 90    // frames between shots
	BaseShotDamage = 20    // damage to the player per shot
	BaseShotSpeed  = 5.0   // world units per frame
	BaseStandoff   = 90.0  // preferred distance from the target
)

// Profile is one type's stat and behaviour block.
type Profile struct {
	Kind       string
	HP         int
	Radar      float64
	FireEvery  int     // frames between shots
	ShotDamage int     // damage to the player per shot
	ShotSpeed  float64 // projectile speed, world units per frame
	SpeedMul   float64 // movement-speed multiplier; ignored when Stationary
	Standoff   float64 // preferred orbit distance
	Stationary bool    // never moves: only faces and fires
}

// The profiles, by kind. Turret is a stationary heavy gun, rusher a fast melee
// threat, sniper a long-range glass cannon, tank a slow bullet sponge.
var profiles = map[string]Profile{
	"enemy": {
		Kind: "enemy", HP: BaseHP, Radar: BaseRadar, FireEvery: BaseFireEvery,
		ShotDamage: BaseShotDamage, ShotSpeed: BaseShotSpeed, SpeedMul: 1, Standoff: BaseStandoff,
	},
	"turret": {
		Kind: "turret", HP: 6, Radar: BaseRadar * 1.5, FireEvery: 40,
		ShotDamage: 16, ShotSpeed: BaseShotSpeed * 1.25, SpeedMul: 0, Standoff: 0, Stationary: true,
	},
	"rusher": {
		Kind: "rusher", HP: 2, Radar: BaseRadar, FireEvery: 240,
		ShotDamage: BaseShotDamage, ShotSpeed: BaseShotSpeed, SpeedMul: 1.9, Standoff: 24,
	},
	"sniper": {
		Kind: "sniper", HP: 2, Radar: BaseRadar * 1.8, FireEvery: 110,
		ShotDamage: 32, ShotSpeed: BaseShotSpeed * 1.9, SpeedMul: 0.9, Standoff: BaseStandoff * 2.2,
	},
	"tank": {
		Kind: "tank", HP: 10, Radar: BaseRadar, FireEvery: 70,
		ShotDamage: 22, ShotSpeed: BaseShotSpeed, SpeedMul: 0.5, Standoff: BaseStandoff,
	},
}

// For returns the profile for a kind, falling back to the grunt so an unknown
// hull still flies.
func For(kind string) Profile {
	p, ok := profiles[kind]
	if !ok {
		return profiles["enemy"]
	}
	return p
}

// Kinds lists every profile name, sorted. It is sorted because a caller fielding
// all of them must not inherit Go's map iteration order: a battle built from
// this has to come out the same on every run.
func Kinds() []string {
	kinds := make([]string, 0, len(profiles))
	for kind := range profiles {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}
