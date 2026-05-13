package scramble

// entityKind classifies an entity for behaviour and rendering.
type entityKind int

const (
	entRocket entityKind = iota
	entUFO
	entFireball
	entFuel
	entTower
	entMissile
	entReactor
	entExplosion
)

// entity is a unified shape for every enemy, refuel target, projectile,
// and FX object that lives in world coordinates. Per-kind quirks live in
// flags rather than a tagged union to keep the slice cheap to iterate.
type entity struct {
	kind   entityKind
	x, y   float64 // world coordinates (top-left of bounding sprite)
	vx, vy float64

	alive bool
	hits  int // current damage taken (for reactor, which takes multiple hits)

	// Animation
	frame  int
	frameT float64

	// Rocket: starts on ground, launches when player draws near.
	launched bool

	// Tower / fireball / reactor: cool-down for spawning missiles or
	// fireball trail particles. Generic seconds counter.
	cooldown float64

	// dieT is age of the explosion animation when kind == entExplosion.
	dieT float64
}

// bbox returns the world-space bounding box of the entity's sprite. For
// projectiles and small icons it matches the visible footprint; for the
// reactor it covers the full 13x9 silhouette.
func (e *entity) bbox() (x0, y0, x1, y1 int) {
	w, h := entitySize(e.kind, e.launched)
	x0 = int(e.x)
	y0 = int(e.y)
	x1 = x0 + w
	y1 = y0 + h
	return
}

// entitySize returns the pixel footprint for collision purposes. Rocket
// extends downward when launching to include the flame.
func entitySize(kind entityKind, launched bool) (w, h int) {
	switch kind {
	case entRocket:
		if launched {
			return rocketLaunch.width(), rocketLaunch.height()
		}
		return rocketIdle.width(), rocketIdle.height()
	case entUFO:
		return ufoA.width(), ufoA.height()
	case entFireball:
		return fireballA.width(), fireballA.height()
	case entFuel:
		return fuelTank.width(), fuelTank.height()
	case entTower:
		return baseTower.width(), baseTower.height()
	case entMissile:
		return missile.width(), missile.height()
	case entReactor:
		return reactor.width(), reactor.height()
	case entExplosion:
		return explode0.width(), explode0.height()
	}
	return 0, 0
}

// playerBulletEntity is a forward laser shot in world coordinates.
type playerBulletEntity struct {
	x, y float64
	vx   float64
}

// playerBombEntity is a dropped bomb subject to gravity.
type playerBombEntity struct {
	x, y   float64
	vx, vy float64
}

// star is a parallax background pixel that drifts left across the world.
type star struct {
	x, y  float64
	c     int // index into starPalette
	twink float64
}

// playerEntity holds the player ship's screen-relative position and
// per-frame state (cool-downs, lives, invincibility blink, etc.).
type playerEntity struct {
	x, y          float64 // screen-relative pixel position
	lives         int
	cooldownLaser float64
	cooldownBomb  float64
	explodeT      float64 // > 0 while exploding (death animation playing)
	respawnT      float64 // > 0 while blinking with invincibility
}
