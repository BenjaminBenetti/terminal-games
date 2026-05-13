package gorf

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// MISSION 1 — ASTRO BATTLES
// =====================================================================
//
// A Space-Invaders-style march of 4 rows × 6 columns of Gorfian aliens
// behind a curved destructible force-field shield. The signature
// difference from plain Space Invaders is the *single* arching shield
// that runs the width of the play field — the player must either wait
// for the aliens to bomb gaps in it, or fire upward through one of the
// natural pixel gaps from the curve.

// Tuning.
const (
	astroCols      = 6
	astroRows      = 4
	astroSpriteW   = 8
	astroSpriteH   = 6
	astroColPitch  = 10
	astroRowPitch  = 8
	astroDropPx    = 2
	astroHStepPx   = 2
	astroFrameTime = 0.42

	// March cadence shrinks as aliens die, just like the original. The
	// values here are seconds per step and chosen so the early formation
	// crawls and the final survivor sprints.
	astroBaseInterval = 1.05
	astroMinInterval  = 0.08

	// Bomb fire rate.
	astroFireMin    = 0.55
	astroFireMax    = 1.8
	astroBombSpeed  = 28.0
	astroMaxBombs   = 4

	// Force-field shield geometry. Width is a fraction of the play width
	// at construction time; height is small.
	shieldThickness = 2
	shieldArchPx    = 6
)

// astroAlien is one cell in the marching formation.
type astroAlien struct {
	alive bool
	kind  int // 0 top (bird), 1 middle (squid), 2 bottom (crab)
}

// astroState owns Mission 1's bespoke data: the formation, the shield
// bitmap, and pacing timers. Embedded in playScene via the `astro`
// pointer for the duration of the mission.
type astroState struct {
	cols, rows int

	cells [][]astroAlien

	originX, originY float64
	dir              int  // +1 right, -1 left
	pendingDrop      bool // true after a horizontal step hit the wall

	stepT  float64
	alive  int // count of still-alive cells
	total  int

	frame  int
	frameT float64

	fireT float64

	// Force-field shield. The mask is a per-pixel boolean grid keyed at
	// (shieldX + x, shieldY + y). The arch is generated at start.
	shieldX, shieldY int
	shieldW, shieldH int
	shieldMask       [][]bool

	loseY int // alien-reaches-here = game over

	cleared bool
}

func newAstroState(p *playScene) *astroState {
	s := &astroState{
		cols: astroCols,
		rows: astroRows,
		dir:  1,
	}
	// Build alien grid: top row = bird (high), middle two rows = squid,
	// bottom row = crab. Kinds 0/1/2 map to the three sprites.
	s.cells = make([][]astroAlien, astroRows)
	for r := 0; r < astroRows; r++ {
		s.cells[r] = make([]astroAlien, astroCols)
		var kind int
		switch r {
		case 0:
			kind = 0
		case 1, 2:
			kind = 1
		default:
			kind = 2
		}
		for c := 0; c < astroCols; c++ {
			s.cells[r][c] = astroAlien{alive: true, kind: kind}
		}
	}
	s.alive = astroRows * astroCols
	s.total = s.alive

	// Centre the formation horizontally.
	formationW := (astroCols-1)*astroColPitch + astroSpriteW
	s.originX = float64((p.w - formationW) / 2)
	s.originY = float64(p.playTop + 2)

	// Build the force-field shield. It sits roughly between the formation
	// and the player roam area — a curved arch spanning ~70% of width.
	s.buildShield(p)

	// Lose if an alien's bottom edge crosses the shield (Gorf used the
	// player's row but the shield works as a more natural "they got
	// through" threshold; we still kill the player if an alien reaches
	// the player ship).
	s.loseY = p.playerYMin - 2

	return s
}

// buildShield populates s.shield* with the curved arch bitmap. The arch
// is a parabola y(x) = arch * (1 − (2x/w − 1)²) reflected so the peak
// points upward.
func (s *astroState) buildShield(p *playScene) {
	w := int(float64(p.w) * 0.74)
	if w < 30 {
		w = p.w - 6
	}
	// Add a 1-px ceiling so the arch never collides directly with the
	// formation's last row.
	h := shieldArchPx + shieldThickness
	s.shieldW = w
	s.shieldH = h
	s.shieldX = (p.w - w) / 2
	// Place the shield's bottom row roughly 1 cell above the player
	// roam area.
	s.shieldY = p.playerYMin - h - 1
	if s.shieldY < p.playTop+8 {
		s.shieldY = p.playTop + 8
	}

	s.shieldMask = make([][]bool, h)
	for y := 0; y < h; y++ {
		s.shieldMask[y] = make([]bool, w)
	}
	for x := 0; x < w; x++ {
		// u in [-1, +1] across the width.
		u := float64(x)/float64(w-1)*2 - 1
		// archY: number of pixels up from the bottom row the arch reaches
		// at column x. Peak (u=0) is shieldArchPx, edges (u=±1) are 0.
		archY := int(math.Round(float64(shieldArchPx) * (1 - u*u)))
		// Draw `shieldThickness` pixels at the top of the arch.
		for t := 0; t < shieldThickness; t++ {
			y := (h - 1) - archY - t
			if y >= 0 && y < h {
				s.shieldMask[y][x] = true
			}
		}
	}
}

// alienPos returns the canvas pixel position of cell (r, c).
func (s *astroState) alienPos(r, c int) (int, int) {
	return int(s.originX) + c*astroColPitch, int(s.originY) + r*astroRowPitch
}

// leftRightOccupied returns the leftmost / rightmost column indices that
// still contain a live alien. (-1, -1) if the formation is empty.
func (s *astroState) leftRightOccupied() (int, int) {
	left, right := -1, -1
	for c := 0; c < s.cols; c++ {
		for r := 0; r < s.rows; r++ {
			if s.cells[r][c].alive {
				if left == -1 {
					left = c
				}
				right = c
				break
			}
		}
	}
	return left, right
}

// stepInterval shrinks as more aliens die, producing the classic
// accelerating march.
func (s *astroState) stepInterval(waveScale float64) float64 {
	if s.total == 0 {
		return astroBaseInterval
	}
	t := float64(s.alive) / float64(s.total) // 1.0 → 0.0
	iv := astroMinInterval + (astroBaseInterval-astroMinInterval)*t
	return iv * waveScale
}

// tick advances Mission 1 by `dt` seconds. When `active` is false (intro
// card, player exploding, mission cleared banner), the formation
// freezes but its wing-flap animation keeps going.
func (s *astroState) tick(p *playScene, dt float64, active bool) {
	s.frameT += dt
	if s.frameT >= astroFrameTime {
		s.frameT -= astroFrameTime
		s.frame = 1 - s.frame
	}
	if !active {
		return
	}

	// March step.
	s.stepT += dt
	iv := s.stepInterval(p.cycleScale())
	if s.stepT >= iv {
		s.stepT = 0
		s.stepFormation(p)
	}

	// Bomb fire.
	s.fireT -= dt
	if s.fireT <= 0 {
		s.fireBomb(p)
		s.fireT = astroFireMin + p.rng.Float64()*(astroFireMax-astroFireMin)
	}

	// Player-laser collisions against aliens and shield.
	s.collidePlayerLaser(p)

	// Bomb-vs-shield erosion (so the shield is degraded by enemies, not
	// just the player). Bomb-vs-player collision is handled in the
	// playScene's common bomb pass.
	s.collideBombsVsShield(p)

	// Win / lose checks.
	if s.alive == 0 {
		s.cleared = true
		p.addScore(150) // small clear bonus
	}
	if s.formationOverrun(p) {
		// An alien reached the player's roam zone — treat as a player
		// loss and zero shields out (just like SI).
		if p.player.explodeT <= 0 {
			p.player.lives = 0
			p.player.explodeT = playerExplodeDur
			p.state = psPlayerHit
			p.stateT = 0
			p.setTaunt("YOU WERE OVERRUN", playerExplodeDur)
		}
	}
}

// stepFormation moves the formation one column horizontally, or
// performs a drop+reverse if a step would push past the wall.
func (s *astroState) stepFormation(p *playScene) {
	if s.pendingDrop {
		s.originY += float64(astroDropPx)
		s.dir = -s.dir
		s.pendingDrop = false
		return
	}
	left, right := s.leftRightOccupied()
	if left < 0 {
		return
	}
	step := float64(astroHStepPx * s.dir)
	leftX := s.originX + float64(left*astroColPitch) + step
	rightX := s.originX + float64(right*astroColPitch) + step + float64(astroSpriteW)
	if leftX < 1 || rightX > float64(p.w-1) {
		s.pendingDrop = true
		return
	}
	s.originX += step
}

// fireBomb picks a random live column's bottommost alien to drop a bomb.
func (s *astroState) fireBomb(p *playScene) {
	if len(p.bombs) >= astroMaxBombs {
		return
	}
	cols := []int{}
	for c := 0; c < s.cols; c++ {
		for r := 0; r < s.rows; r++ {
			if s.cells[r][c].alive {
				cols = append(cols, c)
				break
			}
		}
	}
	if len(cols) == 0 {
		return
	}
	col := cols[p.rng.Intn(len(cols))]
	// Find the bottommost alive alien in that column.
	row := -1
	for r := s.rows - 1; r >= 0; r-- {
		if s.cells[r][col].alive {
			row = r
			break
		}
	}
	if row < 0 {
		return
	}
	ax, ay := s.alienPos(row, col)
	p.spawnBomb(float64(ax+astroSpriteW/2-1), float64(ay+astroSpriteH), astroBombSpeed, 0)
}

// collidePlayerLaser checks the player's quad-laser against aliens
// (top-down) and then the force-field shield. The laser passes through
// nothing — first solid contact ends the bolt.
func (s *astroState) collidePlayerLaser(p *playScene) {
	lr, ok := p.laserRect()
	if !ok {
		return
	}
	// Aliens — bottom-up (the deepest one is closest to the laser
	// rising up from the player).
	for r := s.rows - 1; r >= 0; r-- {
		for c := 0; c < s.cols; c++ {
			if !s.cells[r][c].alive {
				continue
			}
			ax, ay := s.alienPos(r, c)
			ar := rect{x0: ax, y0: ay, x1: ax + astroSpriteW, y1: ay + astroSpriteH}
			if lr.overlaps(ar) {
				s.cells[r][c].alive = false
				s.alive--
				p.spawnExplosion(ax, ay, astroSpriteW, astroSpriteH, 0)
				switch s.cells[r][c].kind {
				case 0:
					p.addScore(100)
				case 1:
					p.addScore(60)
				default:
					p.addScore(40)
				}
				p.player.laser = nil
				return
			}
		}
	}

	// Shield — pixel-perfect erosion at the laser top.
	hitX := int(p.player.laser.x) + playerLaserSprite.width()/2
	hitY := int(p.player.laser.y)
	if s.shieldHit(hitX, hitY) {
		s.erodeShield(hitX, hitY, true)
		p.player.laser = nil
	}
}

// shieldHit returns true if there's a solid shield pixel at canvas (x, y).
func (s *astroState) shieldHit(x, y int) bool {
	lx := x - s.shieldX
	ly := y - s.shieldY
	if lx < 0 || ly < 0 || lx >= s.shieldW || ly >= s.shieldH {
		return false
	}
	return s.shieldMask[ly][lx]
}

// erodeShield knocks out a small diamond of pixels around (x, y).
// fromBelow biases the wear so the splash favours the half the shot
// came from (player shots from below, bombs from above).
func (s *astroState) erodeShield(x, y int, fromBelow bool) {
	lx := x - s.shieldX
	ly := y - s.shieldY
	const radius = 2
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			d := absI(dx) + absI(dy)
			if d > radius {
				continue
			}
			if fromBelow && dy > 1 {
				continue
			}
			if !fromBelow && dy < -1 {
				continue
			}
			ax := lx + dx
			ay := ly + dy
			if ax >= 0 && ax < s.shieldW && ay >= 0 && ay < s.shieldH {
				s.shieldMask[ay][ax] = false
			}
		}
	}
}

func (s *astroState) collideBombsVsShield(p *playScene) {
	kept := p.bombs[:0]
	for _, bm := range p.bombs {
		// Use the tip (bottom-centre) of the bomb sprite for shield hits.
		tipX := int(bm.x) + bombA.width()/2
		tipY := int(bm.y) + bombA.height() - 1
		if s.shieldHit(tipX, tipY) {
			s.erodeShield(tipX, tipY, false)
			continue
		}
		kept = append(kept, bm)
	}
	p.bombs = kept
}

// formationOverrun returns true when an alien's bottom edge crosses the
// player's roam zone — i.e. they made it past everything.
func (s *astroState) formationOverrun(p *playScene) bool {
	for r := s.rows - 1; r >= 0; r-- {
		for c := 0; c < s.cols; c++ {
			if !s.cells[r][c].alive {
				continue
			}
			_, ay := s.alienPos(r, c)
			if ay+astroSpriteH >= s.loseY {
				return true
			}
		}
	}
	return false
}

// draw paints aliens + shield. HUD, player, projectiles, explosions are
// painted by the playScene around this.
func (s *astroState) draw(p *playScene, c *engine.Canvas) {
	// Force-field shield.
	col := forceFieldPalette['#']
	for y := 0; y < s.shieldH; y++ {
		for x := 0; x < s.shieldW; x++ {
			if s.shieldMask[y][x] {
				c.Set(s.shieldX+x, s.shieldY+y, col)
			}
		}
	}
	// Aliens.
	for r := 0; r < s.rows; r++ {
		for col := 0; col < s.cols; col++ {
			if !s.cells[r][col].alive {
				continue
			}
			x, y := s.alienPos(r, col)
			var spr sprite
			var pal map[byte]engine.Color
			switch s.cells[r][col].kind {
			case 0:
				if s.frame == 0 {
					spr = astroBirdA
				} else {
					spr = astroBirdB
				}
				pal = astroBirdPalette
			case 1:
				if s.frame == 0 {
					spr = astroSquidA
				} else {
					spr = astroSquidB
				}
				pal = astroSquidPalette
			default:
				if s.frame == 0 {
					spr = astroCrabA
				} else {
					spr = astroCrabB
				}
				pal = astroCrabPalette
			}
			drawSprite(c, x, y, spr, pal)
		}
	}
}

// =====================================================================
// Math helpers shared across missions.
// =====================================================================

func absI(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// cycleScale is the difficulty multiplier for the current cycle. Cycle 1
// is 1.0 (slowest); each subsequent cycle multiplies most timing values
// down by 0.85, like the wave scaling in galaxian/spaceinvaders.
func (p *playScene) cycleScale() float64 {
	scale := 1.0
	for i := 1; i < p.cycle; i++ {
		scale *= 0.85
	}
	return scale
}
