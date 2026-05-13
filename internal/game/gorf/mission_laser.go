package gorf

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// MISSION 2 — LASER ATTACK
// =====================================================================
//
// Two staggered rows of "anti-Gorfian" laser ships drift back and forth
// while the lead flagship at the centre directs the attack. At random
// intervals one of the ships locks a continuous beam onto the player's
// column, briefly telegraphing a red warning tracer first. The beams
// stay at a fixed x for their duration and the player must side-step
// out of them while picking off the ships above. The mission clears
// when every ship is destroyed.

const (
	laserShipW = 9
	laserShipH = 6
	laserFlagW = 11
	laserFlagH = 7

	laserCols      = 4
	laserRows      = 2 // plus the flagship
	laserColPitch  = 13
	laserRowPitch  = 8
	laserFlagYGap  = 2

	laserSwayAmp = 4.0
	laserSwayHz  = 0.18
	laserDescent = 0.45 // pixels per second — formation creeps toward player

	laserFireMin = 1.4
	laserFireMax = 2.6

	laserWarnDur = 1.1 // tracer warning duration before the beam locks on
	laserBeamDur = 0.9 // how long the actual beam stays alive

	laserFrameTime = 0.35
)

// beamState — a single laser beam emitted by one ship.
type beamState struct {
	x       int    // fixed canvas x of the beam column
	srcX    float64 // ship anchor x (centre of the firing ship), for drawing the muzzle
	srcY    int    // canvas y at the ship's bottom edge (top of beam)
	t       float64
	dur     float64 // total lifetime
	warning bool    // true while in the telegraph phase
}

// laserShip is a single ship in the formation.
type laserShip struct {
	alive    bool
	flagship bool
	row, col int // formation slot (flagship has row=-1)
	fireCD   float64
}

// laserState — Mission 2's bespoke state.
type laserState struct {
	ships []*laserShip
	flag  *laserShip

	formationT float64
	formationY float64 // top of the topmost row (creeps downward)
	centerX    float64 // formation horizontal centre line

	frame  int
	frameT float64

	beams []*beamState

	cleared bool
}

func newLaserState(p *playScene) *laserState {
	s := &laserState{}
	// Build the two-row formation.
	for r := 0; r < laserRows; r++ {
		for c := 0; c < laserCols; c++ {
			s.ships = append(s.ships, &laserShip{
				alive: true,
				row:   r,
				col:   c,
			})
		}
	}
	// Flagship sits above the rows, centred.
	s.flag = &laserShip{alive: true, flagship: true, row: -1, col: 0}
	s.ships = append(s.ships, s.flag)
	s.centerX = float64(p.w) / 2
	s.formationY = float64(p.playTop + 3)
	return s
}

// shipPos returns the canvas position of ship sh accounting for sway.
func (s *laserState) shipPos(p *playScene, sh *laserShip) (int, int) {
	sway := math.Sin(s.formationT*2*math.Pi*laserSwayHz) * laserSwayAmp
	if sh.flagship {
		x := s.centerX + sway - float64(laserFlagW)/2
		y := s.formationY - float64(laserFlagH+laserFlagYGap)
		return int(x), int(y)
	}
	// Centre the row around centerX.
	rowWidth := float64((laserCols-1)*laserColPitch + laserShipW)
	leftX := s.centerX - rowWidth/2 + sway
	x := leftX + float64(sh.col*laserColPitch)
	y := s.formationY + float64(sh.row*laserRowPitch)
	return int(x), int(y)
}

func (s *laserState) tick(p *playScene, dt float64, active bool) {
	s.formationT += dt
	s.frameT += dt
	if s.frameT >= laserFrameTime {
		s.frameT -= laserFrameTime
		s.frame = 1 - s.frame
	}
	// Animate beams regardless of active so their warning/lock visuals
	// continue during transition states.
	kept := s.beams[:0]
	for _, b := range s.beams {
		b.t += dt
		if b.warning && b.t >= laserWarnDur {
			b.warning = false
			b.t = 0
		}
		if !b.warning && b.t >= b.dur {
			continue
		}
		kept = append(kept, b)
	}
	s.beams = kept

	// During gameplay, advance the formation and trigger fire.
	if active {
		s.formationY += laserDescent * dt
		// Reset cooldowns and possibly fire.
		anyAlive := false
		for _, sh := range s.ships {
			if !sh.alive {
				continue
			}
			anyAlive = true
			if sh.fireCD > 0 {
				sh.fireCD -= dt
			}
		}
		// Trigger a fire from a random alive ship if its cooldown is up.
		if anyAlive && p.rng.Float64() < dt/0.45 {
			s.tryFireFromRandom(p)
		}

		// Active beams hurt the player on overlap.
		s.collideBeamsVsPlayer(p)

		// Player laser collisions.
		s.collidePlayerLaser(p)

		// Win condition.
		if !anyAlive {
			s.cleared = true
			p.addScore(250)
		}

		// Lose condition: formation reaches player roam zone.
		if s.formationOverrun(p) && p.player.explodeT <= 0 {
			p.player.lives = 0
			p.player.explodeT = playerExplodeDur
			p.state = psPlayerHit
			p.stateT = 0
			p.setTaunt("YOU WERE OVERRUN", playerExplodeDur)
		}
	}
}

func (s *laserState) tryFireFromRandom(p *playScene) {
	candidates := []*laserShip{}
	for _, sh := range s.ships {
		if sh.alive && sh.fireCD <= 0 {
			candidates = append(candidates, sh)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sh := candidates[p.rng.Intn(len(candidates))]
	sx, sy := s.shipPos(p, sh)
	w := laserShipW
	h := laserShipH
	if sh.flagship {
		w = laserFlagW
		h = laserFlagH
	}
	// Beam locks onto the player's current x.
	playerCentre := p.player.x + float64(playerSprite.width())/2
	// Add a touch of jitter so the lock isn't perfect.
	playerCentre += (p.rng.Float64() - 0.5) * 4
	if playerCentre < 1 {
		playerCentre = 1
	}
	if playerCentre > float64(p.w-2) {
		playerCentre = float64(p.w - 2)
	}
	s.beams = append(s.beams, &beamState{
		x:       int(playerCentre),
		srcX:    float64(sx + w/2),
		srcY:    sy + h,
		t:       0,
		dur:     laserBeamDur,
		warning: true,
	})
	sh.fireCD = laserFireMin + p.rng.Float64()*(laserFireMax-laserFireMin)
}

// beamLineX returns the x of the slanted beam at canvas row y. The beam
// is a straight line from (srcX, srcY) at the muzzle down to (b.x, h-1)
// at the bottom of the screen, so warning preview and live beam share
// the same trajectory.
func (b *beamState) beamLineX(y, h int) int {
	dy := (h - 1) - b.srcY
	if dy <= 0 {
		return int(b.srcX)
	}
	dx := float64(b.x) - b.srcX
	t := float64(y-b.srcY) / float64(dy)
	return int(b.srcX + dx*t)
}

func (s *laserState) collideBeamsVsPlayer(p *playScene) {
	if p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, b := range s.beams {
		if b.warning {
			continue
		}
		// Walk the slanted beam pixel-by-pixel and check overlap with
		// the player AABB. The beam is exactly the same line drawn in
		// drawBeam, so what you see is what hits you.
		y0 := b.srcY
		if y0 < pr.y0 {
			y0 = pr.y0
		}
		y1 := pr.y1
		if y1 > p.h {
			y1 = p.h
		}
		hit := false
		for y := y0; y < y1; y++ {
			x := b.beamLineX(y, p.h)
			if x >= pr.x0 && x < pr.x1 {
				hit = true
				break
			}
		}
		if hit {
			p.playerHit()
			return
		}
	}
}

func (s *laserState) collidePlayerLaser(p *playScene) {
	lr, ok := p.laserRect()
	if !ok {
		return
	}
	// Test ships in descending y order so the closest hit registers first.
	type hit struct {
		sh *laserShip
		y  int
	}
	hits := []hit{}
	for _, sh := range s.ships {
		if !sh.alive {
			continue
		}
		x, y := s.shipPos(p, sh)
		w, h := laserShipW, laserShipH
		if sh.flagship {
			w, h = laserFlagW, laserFlagH
		}
		r := rect{x0: x, y0: y, x1: x + w, y1: y + h}
		if lr.overlaps(r) {
			hits = append(hits, hit{sh: sh, y: y + h})
		}
	}
	if len(hits) == 0 {
		return
	}
	best := hits[0]
	for _, hh := range hits[1:] {
		if hh.y > best.y {
			best = hh
		}
	}
	best.sh.alive = false
	x, y := s.shipPos(p, best.sh)
	w, h := laserShipW, laserShipH
	if best.sh.flagship {
		w, h = laserFlagW, laserFlagH
		p.addScore(500)
		p.setTaunt("THE FLAGSHIP FALLS", 1.0)
	} else {
		p.addScore(200)
	}
	p.spawnExplosion(x, y, w, h, 0)
	p.player.laser = nil
}

// formationOverrun returns true when any ship's bottom edge has reached
// the player's roam ceiling.
func (s *laserState) formationOverrun(p *playScene) bool {
	for _, sh := range s.ships {
		if !sh.alive {
			continue
		}
		_, y := s.shipPos(p, sh)
		h := laserShipH
		if sh.flagship {
			h = laserFlagH
		}
		if y+h >= p.playerYMin {
			return true
		}
	}
	return false
}

func (s *laserState) draw(p *playScene, c *engine.Canvas) {
	// Beams first so ships render on top of the muzzle flash origin.
	for _, b := range s.beams {
		s.drawBeam(p, c, b)
	}
	// Flagship — drawn slightly differently from regular ships.
	if s.flag.alive {
		x, y := s.shipPos(p, s.flag)
		spr := laserFlagA
		if s.frame == 1 {
			spr = laserFlagB
		}
		drawSprite(c, x, y, spr, laserFlagPalette)
	}
	for _, sh := range s.ships {
		if !sh.alive || sh.flagship {
			continue
		}
		x, y := s.shipPos(p, sh)
		spr := laserShipA
		if s.frame == 1 {
			spr = laserShipB
		}
		drawSprite(c, x, y, spr, laserShipPalette)
	}
}

func (s *laserState) drawBeam(p *playScene, c *engine.Canvas, b *beamState) {
	beamCol := laserBeamPalette['#']
	warnCol := engine.Color{R: 250, G: 230, B: 100, A: 255}

	if b.warning {
		// Telegraph: a flashing dotted line from ship to the locked-on x.
		// Pulse phase: every 0.08s.
		if int(b.t/0.08)%2 != 0 {
			return
		}
		for y := b.srcY; y < p.h; y += 2 {
			x := b.beamLineX(y, p.h)
			c.Set(x, y, warnCol)
		}
		return
	}

	// Real beam — a solid line along the exact same path the warning
	// telegraphed. This is non-negotiable for fairness: what you see
	// during the lock-on is what fires.
	pulse := 0.7 + 0.3*math.Sin(b.t*18)
	col := engine.Color{
		R: uint8(float64(beamCol.R) * pulse),
		G: uint8(float64(beamCol.G) * pulse),
		B: uint8(float64(beamCol.B) * pulse),
		A: 255,
	}
	core := engine.Color{R: 255, G: 240, B: 255, A: 255}
	for y := b.srcY; y < p.h; y++ {
		x := b.beamLineX(y, p.h)
		c.Set(x, y, col)
		// Brighter core every other row for a hot inner edge.
		if (y-b.srcY)%2 == 0 {
			c.Set(x, y, core)
		}
	}
}
