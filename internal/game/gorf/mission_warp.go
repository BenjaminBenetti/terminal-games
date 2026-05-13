package gorf

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// MISSION 4 — SPACE WARP
// =====================================================================
//
// Ships emerge from a "warp point" near the upper centre of the play
// field and spiral outward, growing as they approach the player. The
// mission ends when the player destroys the required number of ships;
// ships that escape past the edges of the play field count toward an
// "escape" budget — too many escapees and the Gorfian invasion
// succeeds (player loses the mission with a shield penalty).

const (
	warpKillsToClear     = 14 // destroy this many to clear the mission
	warpSpawnInterval    = 0.55
	warpRadialSpeedBase  = 7.5
	warpRadialAccel      = 4.5
	warpAngularSpeed     = 0.8 // radians/sec spiral spin
	warpMaxRadius        = 80.0 // ship is gone past this radius
	warpMaxEscapes       = 6
	warpFrameTime        = 0.18
)

// warpShip — one outward-spiraling enemy.
type warpShip struct {
	alive   bool
	theta   float64 // current angle from warp centre
	radius  float64 // current distance from warp centre
	vRad    float64 // radial speed (grows over time)
	spinDir float64 // ±1 — handedness of the spiral
	t       float64
	frame   int
}

type warpState struct {
	cx, cy   float64
	ships    []*warpShip
	spawnT   float64
	kills    int
	escapes  int
	cleared  bool
	// pulse animation for the warp point itself.
	pulseT float64
}

func newWarpState(p *playScene) *warpState {
	s := &warpState{
		cx:     float64(p.w) / 2,
		cy:     float64(p.playTop + (p.playerYMin-p.playTop)/2),
		spawnT: 0.6,
	}
	return s
}

// shipSize returns the visual size bucket for the given radius:
// 0 = tiny, 1 = small, 2 = medium, 3 = big.
func (s *warpState) shipSize(r float64) int {
	switch {
	case r < 4:
		return 0
	case r < 12:
		return 1
	case r < 26:
		return 2
	default:
		return 3
	}
}

// shipSprite returns the appropriate sprite for the ship's current
// distance from the warp centre.
func (s *warpState) shipSprite(sh *warpShip) (sprite, map[byte]engine.Color) {
	size := s.shipSize(sh.radius)
	switch size {
	case 0:
		return warpShipSmall, warpPaletteFar
	case 1:
		return warpShipTiny, warpPaletteFar
	case 2:
		return warpShipMed, warpPaletteMid
	default:
		return warpShipBig, warpPaletteNear
	}
}

// shipBounds returns the AABB of ship sh in canvas pixels.
func (s *warpState) shipBounds(sh *warpShip) rect {
	spr, _ := s.shipSprite(sh)
	w := spr.width()
	h := spr.height()
	cx := s.cx + sh.radius*math.Cos(sh.theta)
	cy := s.cy + sh.radius*math.Sin(sh.theta)
	x0 := int(cx) - w/2
	y0 := int(cy) - h/2
	return rect{x0: x0, y0: y0, x1: x0 + w, y1: y0 + h}
}

func (s *warpState) tick(p *playScene, dt float64, active bool) {
	s.pulseT += dt
	for _, sh := range s.ships {
		if !sh.alive {
			continue
		}
		sh.t += dt
		sh.vRad += warpRadialAccel * dt
		sh.radius += sh.vRad * dt
		sh.theta += sh.spinDir * warpAngularSpeed * dt
		// Frame flip — every few frames so the ship "blinks" as it grows.
		if int(sh.t/warpFrameTime)%2 != sh.frame {
			sh.frame = 1 - sh.frame
		}

		// Out-of-bounds: ship escapes if its centre exits the play
		// rectangle.
		cx := s.cx + sh.radius*math.Cos(sh.theta)
		cy := s.cy + sh.radius*math.Sin(sh.theta)
		if cx < -8 || cx > float64(p.w)+8 || cy > float64(p.h)+8 || cy < float64(p.playTop)-8 {
			sh.alive = false
			if active {
				s.escapes++
			}
		}
		// Or radius is too large.
		if sh.radius > warpMaxRadius {
			sh.alive = false
		}
	}
	// Cull dead ships.
	kept := s.ships[:0]
	for _, sh := range s.ships {
		if sh.alive {
			kept = append(kept, sh)
		}
	}
	s.ships = kept

	if !active {
		return
	}

	// Spawn new ships periodically.
	s.spawnT -= dt
	if s.spawnT <= 0 {
		s.spawnShip(p)
		scale := p.cycleScale()
		s.spawnT = warpSpawnInterval * scale * (0.8 + p.rng.Float64()*0.4)
	}

	// Player-laser collisions.
	s.collidePlayerLaser(p)

	// Ship-vs-player collision.
	s.collideShipsVsPlayer(p)

	// Win / lose.
	if s.kills >= warpKillsToClear+p.cycle-1 {
		s.cleared = true
		p.addScore(500)
	}
	if s.escapes >= warpMaxEscapes {
		// Penalty: player loses a shield each time threshold crosses.
		// Reset escape counter for the next penalty band.
		s.escapes = 0
		if p.player.explodeT <= 0 {
			p.player.lives--
			if p.player.lives <= 0 {
				p.player.lives = 0
				p.player.explodeT = playerExplodeDur
				p.state = psPlayerHit
				p.stateT = 0
				p.setTaunt("GORFIANS ESCAPED THROUGH THE WARP", playerExplodeDur)
			} else {
				p.setTaunt("WARP BREACHED", 1.0)
			}
		}
	}
}

func (s *warpState) spawnShip(p *playScene) {
	dir := 1.0
	if p.rng.Intn(2) == 0 {
		dir = -1.0
	}
	s.ships = append(s.ships, &warpShip{
		alive:   true,
		theta:   p.rng.Float64() * 2 * math.Pi,
		radius:  0.6 + p.rng.Float64()*1.6,
		vRad:    warpRadialSpeedBase,
		spinDir: dir,
	})
}

func (s *warpState) collidePlayerLaser(p *playScene) {
	lr, ok := p.laserRect()
	if !ok {
		return
	}
	for _, sh := range s.ships {
		if !sh.alive {
			continue
		}
		if lr.overlaps(s.shipBounds(sh)) {
			sh.alive = false
			s.kills++
			b := s.shipBounds(sh)
			p.spawnExplosion(b.x0, b.y0, b.x1-b.x0, b.y1-b.y0, 0)
			// Score scales with proximity — closer ships are bigger and
			// worth more (the original Gorf rewarded close calls).
			score := 100 + 60*s.shipSize(sh.radius)
			p.addScore(score)
			p.player.laser = nil
			return
		}
	}
}

func (s *warpState) collideShipsVsPlayer(p *playScene) {
	if p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, sh := range s.ships {
		if !sh.alive {
			continue
		}
		if s.shipBounds(sh).overlaps(pr) {
			sh.alive = false
			b := s.shipBounds(sh)
			p.spawnExplosion(b.x0, b.y0, b.x1-b.x0, b.y1-b.y0, 0)
			p.playerHit()
			return
		}
	}
}

func (s *warpState) draw(p *playScene, c *engine.Canvas) {
	// Warp point — pulsing concentric rings at (cx, cy).
	for r := 0; r < 4; r++ {
		phase := s.pulseT*3 + float64(r)*0.45
		bri := 0.5 + 0.5*math.Sin(phase)
		col := engine.Color{
			R: uint8(255 * bri),
			G: uint8(120 * bri),
			B: uint8(255 * bri),
			A: 255,
		}
		drawCircleOutline(c, int(s.cx), int(s.cy), r+1, col)
	}

	// Ships, painted back-to-front so closer ones overlap distant ones.
	// Sort by radius ascending.
	ordered := append([]*warpShip(nil), s.ships...)
	sortByRadius(ordered)
	for _, sh := range ordered {
		if !sh.alive {
			continue
		}
		spr, pal := s.shipSprite(sh)
		w := spr.width()
		h := spr.height()
		cx := s.cx + sh.radius*math.Cos(sh.theta)
		cy := s.cy + sh.radius*math.Sin(sh.theta)
		drawSprite(c, int(cx)-w/2, int(cy)-h/2, spr, pal)
	}

	// Status — show progress toward kill quota in the lower HUD area
	// (rendered as a small terminal-font line above the player).
	target := warpKillsToClear + p.cycle - 1
	progressLine := "WARP TARGETS " + zeroPad(s.kills, 2) + "/" + zeroPad(target, 2) +
		"   ESCAPES " + zeroPad(s.escapes, 1) + "/" + zeroPad(warpMaxEscapes, 1)
	c.Print((c.Cols()-len(progressLine))/2, c.Rows()-3, progressLine,
		engine.Color{R: 200, G: 200, B: 250, A: 255})
}

// drawCircleOutline is a simple unfilled circle. The canvas package
// provides DrawCircle but we use a bespoke version so we can clip the
// rings naturally to the play area.
func drawCircleOutline(c *engine.Canvas, cx, cy, r int, col engine.Color) {
	if r <= 0 {
		c.Set(cx, cy, col)
		return
	}
	x, y, err := r, 0, 1-r
	for x >= y {
		c.Set(cx+x, cy+y, col)
		c.Set(cx+y, cy+x, col)
		c.Set(cx-y, cy+x, col)
		c.Set(cx-x, cy+y, col)
		c.Set(cx-x, cy-y, col)
		c.Set(cx-y, cy-x, col)
		c.Set(cx+y, cy-x, col)
		c.Set(cx+x, cy-y, col)
		y++
		if err <= 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

// sortByRadius — insertion sort by ascending radius. The ship list is
// usually small (~20) so a quadratic sort is fine and avoids pulling in
// sort/slices for one call site.
func sortByRadius(ss []*warpShip) {
	for i := 1; i < len(ss); i++ {
		j := i
		for j > 0 && ss[j-1].radius > ss[j].radius {
			ss[j-1], ss[j] = ss[j], ss[j-1]
			j--
		}
	}
}
