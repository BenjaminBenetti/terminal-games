package gorf

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// MISSION 5 — FLAG SHIP (Gorfian Boss)
// =====================================================================
//
// The Gorfian mothership descends partway into the play field. A row of
// square shield tiles below the hull slides horizontally, and the
// mothership's central reactor is only exposed where a shield tile is
// missing or has been destroyed. The player must time their quad-laser
// shots through the moving gap to hit the reactor; the flagship returns
// fire with heavy bombs aimed at the player's column.

const (
	flagshipHits      = 12 // reactor hits needed to defeat
	flagShieldTiles   = 11
	flagShieldGap     = 2 // initial gap width (number of missing tiles in the centre)
	flagShipSpeed     = 22.0
	flagShieldSpeed   = 18.0 // pixels/sec lateral shield drift
	flagBombInterval  = 1.3
	flagDescendDur    = 1.4
)

// flagState — Mission 5's bespoke data. The flagship hull is drawn at
// (hullX, hullY); the shield row sits directly below at (shieldX, hullY+hullH).
// Per-tile shield state is in shields[] — true means intact.
type flagState struct {
	hullX, hullY int
	hullW, hullH int

	tileW, tileH int
	shieldY      int
	shieldRowX   float64 // current x of the leftmost tile (drifts laterally)
	shieldDir    float64 // ±1 — sliding direction
	shields      []bool  // length == flagShieldTiles

	// Reactor — its bounding rect relative to (hullX, hullY).
	reactorRel rect

	// Flagship lateral motion.
	shipDir float64 // ±1
	shipVx  float64 // current x-velocity (signed)

	// Descent — flagship rises into position over flagDescendDur, then
	// resumes lateral movement.
	descending bool
	descT      float64
	descStartY int
	descEndY   int

	hp        int
	bombT     float64

	cleared bool
}

func newFlagState(p *playScene) *flagState {
	s := &flagState{
		hullW: flagshipHull.width(),
		hullH: flagshipHull.height(),
		hp:    flagshipHits,
	}
	// Centre the hull horizontally; descend from above into final pos.
	s.hullX = (p.w - s.hullW) / 2
	s.descStartY = -s.hullH
	s.descEndY = p.playTop + 1
	s.hullY = s.descStartY
	s.descending = true

	// Shield tiles below the hull.
	s.tileW = flagshipShieldTile.width()
	s.tileH = flagshipShieldTile.height()
	totalShieldW := s.tileW * flagShieldTiles
	s.shieldRowX = float64((p.w - totalShieldW) / 2)
	s.shieldY = s.descEndY + s.hullH + 1
	s.shields = make([]bool, flagShieldTiles)
	for i := range s.shields {
		s.shields[i] = true
	}
	// Punch the initial gap in the centre — flagShieldGap tiles missing.
	mid := flagShieldTiles / 2
	half := flagShieldGap / 2
	for i := mid - half; i <= mid-half+flagShieldGap-1; i++ {
		if i >= 0 && i < flagShieldTiles {
			s.shields[i] = false
		}
	}

	// Reactor — the central magenta square at roughly the centre of the
	// hull sprite. From the sprite, the reactor occupies columns 8..14,
	// rows 5..7 (approximately). Compute its rect in hull-local pixels.
	s.reactorRel = rect{
		x0: 9,
		y0: 5,
		x1: 14,
		y1: 9,
	}

	s.shieldDir = 1
	s.shipDir = 1
	return s
}

// reactorRect returns the reactor's current AABB in canvas coordinates.
func (s *flagState) reactorRect() rect {
	return rect{
		x0: s.hullX + s.reactorRel.x0,
		y0: s.hullY + s.reactorRel.y0,
		x1: s.hullX + s.reactorRel.x1,
		y1: s.hullY + s.reactorRel.y1,
	}
}

// hullRect returns the flagship hull's full bounding rect.
func (s *flagState) hullRect() rect {
	return rect{
		x0: s.hullX,
		y0: s.hullY,
		x1: s.hullX + s.hullW,
		y1: s.hullY + s.hullH,
	}
}

// shieldTileRect returns the AABB of shield tile i.
func (s *flagState) shieldTileRect(i int) rect {
	x := int(s.shieldRowX) + i*s.tileW
	return rect{
		x0: x,
		y0: s.shieldY,
		x1: x + s.tileW,
		y1: s.shieldY + s.tileH,
	}
}

// shieldBlocks returns true if any shield tile would block a vertical
// laser at canvas x at the shield row's y range.
func (s *flagState) shieldBlocks(x, y0, y1 int) (bool, int) {
	for i, alive := range s.shields {
		if !alive {
			continue
		}
		r := s.shieldTileRect(i)
		if x >= r.x0 && x < r.x1 && y0 < r.y1 && y1 > r.y0 {
			return true, i
		}
	}
	return false, -1
}

func (s *flagState) tick(p *playScene, dt float64, active bool) {
	if s.descending {
		s.descT += dt
		u := s.descT / flagDescendDur
		if u > 1 {
			u = 1
		}
		// Ease in/out for a satisfying entrance.
		eased := easeOutCubic(u)
		s.hullY = int(float64(s.descStartY) + float64(s.descEndY-s.descStartY)*eased)
		if s.descT >= flagDescendDur {
			s.descending = false
			s.hullY = s.descEndY
		}
	}
	// Shield row drift — bounces between play-area edges.
	totalShieldW := float64(s.tileW * flagShieldTiles)
	s.shieldRowX += s.shieldDir * flagShieldSpeed * dt
	if s.shieldRowX < 2 {
		s.shieldRowX = 2
		s.shieldDir = 1
	}
	if s.shieldRowX+totalShieldW > float64(p.w-2) {
		s.shieldRowX = float64(p.w-2) - totalShieldW
		s.shieldDir = -1
	}

	if !active || s.descending {
		return
	}

	// Hull drifts laterally too, with a slower speed.
	s.hullX += int(s.shipDir * flagShipSpeed * dt)
	if s.hullX < 2 {
		s.hullX = 2
		s.shipDir = 1
	}
	if s.hullX+s.hullW > p.w-2 {
		s.hullX = p.w - 2 - s.hullW
		s.shipDir = -1
	}

	// Bomb every flagBombInterval seconds, aimed at player.
	s.bombT -= dt
	if s.bombT <= 0 {
		s.fireBomb(p)
		s.bombT = flagBombInterval / p.cycleScale()
	}

	// Player-laser hits: check shield row first, then reactor / hull.
	s.collidePlayerLaser(p)

	// Win.
	if s.hp <= 0 {
		s.cleared = true
		p.addScore(2000)
	}
}

func (s *flagState) fireBomb(p *playScene) {
	// Bomb spawns at the reactor's centre and arcs toward the player.
	rr := s.reactorRect()
	bx := float64((rr.x0+rr.x1)/2) - float64(bossBomb.width())/2
	by := float64(rr.y1)
	p.spawnBomb(bx, by, bombSpeed*1.4, 1)
	_ = math.Cos // keep math reachable for callers that might add curves later
}

func (s *flagState) collidePlayerLaser(p *playScene) {
	lr, ok := p.laserRect()
	if !ok {
		return
	}
	// Check shield row first — a shot must pass through a gap to hit the
	// hull or reactor. If it overlaps a live tile, kill that tile.
	shieldRowY0 := s.shieldY
	shieldRowY1 := s.shieldY + s.tileH
	if lr.y0 < shieldRowY1 && lr.y1 > shieldRowY0 {
		// Find any shield tiles the laser crosses.
		laserMidX := (lr.x0 + lr.x1) / 2
		hit, idx := s.shieldBlocks(laserMidX, lr.y0, lr.y1)
		if hit {
			s.shields[idx] = false
			x := int(s.shieldRowX) + idx*s.tileW
			p.spawnExplosion(x, s.shieldY, s.tileW, s.tileH, 0)
			p.player.laser = nil
			p.addScore(40)
			return
		}
	}
	// Reactor hit?
	rr := s.reactorRect()
	if lr.overlaps(rr) {
		s.hp--
		p.spawnExplosion(rr.x0, rr.y0, rr.x1-rr.x0, rr.y1-rr.y0, 0)
		p.player.laser = nil
		p.addScore(150)
		if s.hp <= 0 {
			// Spawn a bigger explosion at the hull centre.
			p.spawnExplosion(s.hullX, s.hullY+s.hullH/2-3, s.hullW, 6, 0)
			p.spawnExplosion(s.hullX+s.hullW/3, s.hullY+s.hullH/3, s.hullW/3, s.hullH/3, 0)
			p.setTaunt("MY EMPIRE FALLS", 1.4)
		}
		return
	}
	// Hull hit (no damage but laser consumed) — only if the shot wasn't
	// blocked by the shield row.
	hr := s.hullRect()
	if lr.overlaps(hr) {
		p.player.laser = nil
		p.addScore(20)
		// Small spark at the impact line.
		p.spawnExplosion(lr.x0, hr.y1-2, lr.x1-lr.x0, 2, 0)
	}
}

func (s *flagState) draw(p *playScene, c *engine.Canvas) {
	// Hull.
	drawSprite(c, s.hullX, s.hullY, flagshipHull, flagshipPalette)
	// Reactor glow overlay — pulses to draw the player's eye to the
	// weak spot. We blink the reactor highlight on a 0.7s cycle.
	pulse := 0.6 + 0.4*math.Sin(p.stateT*8)
	rPal := map[byte]engine.Color{
		'#': {R: uint8(255 * pulse), G: uint8(220 * pulse), B: uint8(150 * pulse), A: 255},
		'=': {R: uint8(255 * pulse), G: uint8(120 * pulse), B: uint8(255 * pulse), A: 255},
		'+': {R: uint8(255 * pulse), G: uint8(250 * pulse), B: uint8(255 * pulse), A: 255},
	}
	drawSprite(c, s.hullX+s.reactorRel.x0, s.hullY+s.reactorRel.y0, flagshipReactor, rPal)

	// Shield row.
	for i, alive := range s.shields {
		if !alive {
			continue
		}
		x := int(s.shieldRowX) + i*s.tileW
		drawSprite(c, x, s.shieldY, flagshipShieldTile, flagshipShieldPalette)
	}

	// Boss HP bar — drawn at the top of the play area.
	barW := p.w - 6
	if barW > 40 {
		barW = 40
	}
	barX := (p.w - barW) / 2
	barY := s.hullY + s.hullH + s.tileH + 3
	if barY > p.h-4 {
		barY = p.h - 4
	}
	// Background.
	c.FillRect(barX, barY, barW, 1, engine.Color{R: 60, G: 30, B: 60, A: 255})
	// Filled portion.
	filled := barW * s.hp / flagshipHits
	if filled > 0 {
		c.FillRect(barX, barY, filled, 1, engine.Color{R: 250, G: 100, B: 240, A: 255})
	}
	// Label.
	lbl := "GORFIAN FLAGSHIP"
	c.Print((c.Cols()-len(lbl))/2, barY/2-1, lbl,
		engine.Color{R: 250, G: 90, B: 240, A: 255})
}

func easeOutCubic(t float64) float64 {
	u := 1 - t
	return 1 - u*u*u
}
