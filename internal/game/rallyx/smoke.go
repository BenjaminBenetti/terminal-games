package rallyx

// smokePuff is one drop of the smoke screen that the player lays
// behind their car. Each puff lives for a fixed TTL, during which any
// enemy that touches it is stunned. Puffs are placed at the player's
// rear tile when the smoke button is held and fuel is available.
type smokePuff struct {
	x, y float64 // tile-space centre
	age  float64 // seconds since spawn
	ttl  float64 // total lifetime in seconds
}

// alive reports whether the puff still exerts an effect.
func (s *smokePuff) alive() bool { return s.age < s.ttl }

// fade returns a 0..1 visual intensity for rendering — full at spawn,
// linearly down to 0 at ttl.
func (s *smokePuff) fade() float64 {
	if s.ttl <= 0 {
		return 0
	}
	f := 1 - s.age/s.ttl
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
