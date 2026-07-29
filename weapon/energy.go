package weapon

// Pool is a ship's ammunition: one budget that every weapon but the front gun
// draws from. A single pool rather than a magazine per weapon is deliberate —
// it is one number to show, one number for a script to read, and it makes the
// interesting question "which weapon is this fight worth spending on" instead of
// "which of five counters is empty".
//
// It is meant to last: a full pool is a long fight's worth of heavy fire, so
// running dry is a consequence of leaning on the big weapons, not a timer.
type Pool struct {
	Level int
	Max   int

	regen int // ticks since the last point trickled back
}

const (
	// PoolMax is the default capacity. At the catalogue's prices that is dozens of
	// missiles, or about a minute of held beam.
	PoolMax = 1000

	// RegenEvery is how many ticks pass before the pool recovers one point. Slow
	// on purpose: it covers a lull between fights so a ship is never permanently
	// disarmed, while leaning on the heavy weapons still runs it down. Refilling
	// properly is what a pickup is for.
	RegenEvery = 12

	// EnergyPerLevel is what each stacked upgrade adds to a shot's price. A plain
	// front gun is free forever; an upgraded one draws power, which is what makes
	// the upgrades something to keep fed rather than something to hoard.
	EnergyPerLevel = 2

	// PickupEnergy is what a power-up puts back — a quarter of a full pool, so
	// collecting is worth a detour without making the budget irrelevant.
	PickupEnergy = 250
)

// Cost is what one use of w costs at the given stack level: the weapon's own
// price plus a surcharge for every upgrade riding on the shot.
//
// A free weapon stays free at level zero and only then. That is the whole rule
// behind stepping down: a ship that cannot pay for its upgraded gun drops a
// level and fires a cheaper one, all the way back to the plain shot that always
// works.
func Cost(w Weapon, level int) int {
	if level < 0 {
		level = 0
	}
	return w.Energy + level*EnergyPerLevel
}

// NewPool returns a full pool of the given capacity; zero or less uses PoolMax.
func NewPool(maxLevel int) Pool {
	if maxLevel <= 0 {
		maxLevel = PoolMax
	}
	return Pool{Level: maxLevel, Max: maxLevel}
}

// CanAfford reports whether the pool covers a cost. A free shot is always
// affordable, including on an empty pool.
func (p *Pool) CanAfford(cost int) bool {
	return cost <= 0 || p.Level >= cost
}

// Spend pays a cost and reports whether it went through. A refusal leaves the
// pool untouched, so a caller can drop to a cheaper weapon — or a lower stack
// level — and ask again.
func (p *Pool) Spend(cost int) bool {
	if cost <= 0 {
		return true
	}
	if p.Level < cost {
		return false
	}
	p.Level -= cost
	return true
}

// Tick trickles the pool back up, one point every RegenEvery ticks. Call it once
// per frame.
func (p *Pool) Tick() {
	if p.Level >= p.Max {
		p.regen = 0
		return
	}
	p.regen++
	if p.regen < RegenEvery {
		return
	}
	p.regen = 0
	p.Level++
}

// Add tops the pool up, never past its capacity. This is what a pickup does.
func (p *Pool) Add(n int) {
	if n <= 0 {
		return
	}
	p.Level = min(p.Level+n, p.Max)
}

// Fraction is how full the pool is, 0 to 1, for a gauge or an instrument reading.
func (p *Pool) Fraction() float64 {
	if p.Max <= 0 {
		return 0
	}
	return float64(p.Level) / float64(p.Max)
}
