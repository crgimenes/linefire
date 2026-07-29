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
}

// PoolMax is the default capacity. At the catalogue's prices that is dozens of
// missiles, or about a minute of held beam.
const PoolMax = 1000

// NewPool returns a full pool of the given capacity; zero or less uses PoolMax.
func NewPool(maxLevel int) Pool {
	if maxLevel <= 0 {
		maxLevel = PoolMax
	}
	return Pool{Level: maxLevel, Max: maxLevel}
}

// CanAfford reports whether the pool covers one use of w. A free weapon is
// always affordable, including on an empty pool.
func (p *Pool) CanAfford(w Weapon) bool {
	return w.Energy <= 0 || p.Level >= w.Energy
}

// Spend pays for one use of w and reports whether it went through. A refusal
// leaves the pool untouched, so a caller can drop to a cheaper weapon — or a
// lower stack level — and ask again.
func (p *Pool) Spend(w Weapon) bool {
	if w.Energy <= 0 {
		return true
	}
	if p.Level < w.Energy {
		return false
	}
	p.Level -= w.Energy
	return true
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
