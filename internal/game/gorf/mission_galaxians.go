package gorf

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// =====================================================================
// MISSION 3 — GALAXIANS (Anti-Gorfian Squadron)
// =====================================================================
//
// A tight three-row formation of swooper ships sways at the top of the
// play field. At intervals one of them peels off and dives at the
// player on a curving Bezier path, occasionally dropping a bomb. After
// completing its arc the diver exits at the bottom and re-enters from
// the top to retake its formation slot — so the only way to win is to
// shoot every member of the formation.

const (
	galaxCols       = 6
	galaxRows       = 3
	galaxColPitch   = 9
	galaxRowPitch   = 7
	galaxSpriteW    = 7
	galaxSpriteH    = 6
	galaxSwayHz     = 0.22
	galaxSwayAmp    = 3.0
	galaxFrameTime  = 0.32
	galaxDiveSpeed  = 28.0
	galaxDiveAccel  = 18.0
	galaxDiveWaveAmp = 7.0
	galaxDiveWaveHz  = 1.0
	galaxReturnDur   = 1.7
	galaxLaunchMin   = 1.0
	galaxLaunchMax   = 2.0
	galaxMaxDivers   = 2
	galaxBombChance  = 0.55 // per-second chance once diving
)

// galaxAlienState — where this alien is in its life cycle.
type galaxAlienState int

const (
	gsFormation galaxAlienState = iota
	gsDiving
	gsReturning
)

type galaxAlien struct {
	alive bool
	row, col int

	state   galaxAlienState
	phaseT  float64

	// Diving / returning trajectories: explicit pixel positions.
	x, y    float64
	descX0  float64 // base x for sinusoidal descent
	vy      float64
	side    int // -1 left, +1 right

	// Returning curve params.
	retX0, retY0 float64
	retCxOff     float64

	fireCD float64
}

type galaxState struct {
	cells [][]*galaxAlien

	formationT float64
	centerX    float64
	formationY float64

	frame  int
	frameT float64

	launchT float64

	cleared bool
}

func newGalaxState(p *playScene) *galaxState {
	s := &galaxState{}
	s.cells = make([][]*galaxAlien, galaxRows)
	for r := 0; r < galaxRows; r++ {
		s.cells[r] = make([]*galaxAlien, galaxCols)
		for c := 0; c < galaxCols; c++ {
			s.cells[r][c] = &galaxAlien{
				alive: true,
				row:   r,
				col:   c,
				state: gsFormation,
			}
		}
	}
	s.centerX = float64(p.w) / 2
	s.formationY = float64(p.playTop + 3)
	s.launchT = galaxLaunchMin
	return s
}

// slotPos returns the canvas top-left of formation slot (r, c) factoring
// in the current sway.
func (s *galaxState) slotPos(p *playScene, r, c int) (int, int) {
	sway := math.Sin(s.formationT*2*math.Pi*galaxSwayHz) * galaxSwayAmp
	rowWidth := float64((galaxCols-1)*galaxColPitch + galaxSpriteW)
	leftX := s.centerX - rowWidth/2 + sway
	x := leftX + float64(c*galaxColPitch)
	y := s.formationY + float64(r*galaxRowPitch)
	return int(x), int(y)
}

func (s *galaxState) tick(p *playScene, dt float64, active bool) {
	s.formationT += dt
	s.frameT += dt
	if s.frameT >= galaxFrameTime {
		s.frameT -= galaxFrameTime
		s.frame = 1 - s.frame
	}
	if !active {
		// Still let divers physically progress so they don't freeze in
		// mid-arc during transition cards.
		for r := 0; r < galaxRows; r++ {
			for c := 0; c < galaxCols; c++ {
				a := s.cells[r][c]
				if a == nil || !a.alive {
					continue
				}
				if a.state != gsFormation {
					s.tickAlien(p, a, dt, false)
				}
			}
		}
		return
	}

	// Move divers and decide bomb drops.
	for r := 0; r < galaxRows; r++ {
		for c := 0; c < galaxCols; c++ {
			a := s.cells[r][c]
			if a == nil || !a.alive {
				continue
			}
			s.tickAlien(p, a, dt, true)
		}
	}

	// Launch a new diver occasionally.
	s.launchT -= dt
	if s.launchT <= 0 {
		s.tryLaunch(p)
		scale := p.cycleScale()
		s.launchT = galaxLaunchMin*scale + p.rng.Float64()*(galaxLaunchMax-galaxLaunchMin)*scale
	}

	// Resolve player-laser hits.
	s.collidePlayerLaser(p)

	// Diving alien colliding with player kills both.
	s.collideDiversVsPlayer(p)

	// Win condition.
	if s.alive() == 0 {
		s.cleared = true
		p.addScore(300)
	}
}

func (s *galaxState) tickAlien(p *playScene, a *galaxAlien, dt float64, active bool) {
	if a.fireCD > 0 {
		a.fireCD -= dt
	}
	switch a.state {
	case gsFormation:
		// idle — position derived from slot in draw/collide.
	case gsDiving:
		a.phaseT += dt
		a.vy += galaxDiveAccel * dt
		a.y += a.vy * dt
		wave := math.Sin(a.phaseT*2*math.Pi*galaxDiveWaveHz) * galaxDiveWaveAmp
		// Slight drift toward the player so the dive feels deliberate.
		targetX := p.player.x + float64(playerSprite.width())/2
		a.descX0 += (targetX - a.descX0) * 0.04 * dt
		a.x = a.descX0 + wave
		if active && a.fireCD <= 0 && p.rng.Float64() < galaxBombChance*dt {
			p.spawnBomb(a.x+float64(galaxSpriteW)/2-1, a.y+float64(galaxSpriteH), bombSpeed, 0)
			a.fireCD = 0.4 + p.rng.Float64()*0.6
		}
		if a.y > float64(p.h)+8 {
			s.startReturn(p, a)
		}
	case gsReturning:
		a.phaseT += dt
		u := a.phaseT / galaxReturnDur
		if u > 1 {
			u = 1
		}
		// Quadratic Bezier back to slot.
		sx, sy := s.slotPos(p, a.row, a.col)
		ctrlX := (a.retX0+float64(sx))/2 + a.retCxOff
		ctrlY := (a.retY0 + float64(sy)) / 2
		a.x = bezier(u, a.retX0, ctrlX, float64(sx))
		a.y = bezier(u, a.retY0, ctrlY, float64(sy))
		if a.phaseT >= galaxReturnDur {
			a.state = gsFormation
			a.phaseT = 0
		}
	}
}

func (s *galaxState) tryLaunch(p *playScene) {
	divers := 0
	candidates := []*galaxAlien{}
	for r := 0; r < galaxRows; r++ {
		for c := 0; c < galaxCols; c++ {
			a := s.cells[r][c]
			if a == nil || !a.alive {
				continue
			}
			if a.state == gsFormation {
				candidates = append(candidates, a)
			} else {
				divers++
			}
		}
	}
	if len(candidates) == 0 {
		return
	}
	if divers >= galaxMaxDivers+p.cycle-1 {
		return
	}
	a := candidates[p.rng.Intn(len(candidates))]
	sx, sy := s.slotPos(p, a.row, a.col)
	a.x = float64(sx)
	a.y = float64(sy)
	a.descX0 = float64(sx)
	a.vy = galaxDiveSpeed
	a.phaseT = 0
	a.state = gsDiving
	if float64(sx) < s.centerX {
		a.side = -1
	} else {
		a.side = +1
	}
	a.fireCD = 0.3 + p.rng.Float64()*0.5
}

func (s *galaxState) startReturn(p *playScene, a *galaxAlien) {
	a.state = gsReturning
	a.phaseT = 0
	side := 1
	sx, _ := s.slotPos(p, a.row, a.col)
	if float64(sx) < float64(p.w)/2 {
		side = -1
	}
	margin := float64(p.w) / 4
	a.retX0 = float64(p.w)/2 - float64(side)*(margin+p.rng.Float64()*margin)
	if a.retX0 < 2 {
		a.retX0 = 2
	}
	if a.retX0 > float64(p.w-2) {
		a.retX0 = float64(p.w - 2)
	}
	a.retY0 = -float64(galaxSpriteH)
	a.retCxOff = float64(side) * (8 + p.rng.Float64()*8)
}

func (s *galaxState) collidePlayerLaser(p *playScene) {
	lr, ok := p.laserRect()
	if !ok {
		return
	}
	for r := 0; r < galaxRows; r++ {
		for c := 0; c < galaxCols; c++ {
			a := s.cells[r][c]
			if a == nil || !a.alive {
				continue
			}
			ax, ay := s.alienPos(p, a)
			ar := rect{x0: ax, y0: ay, x1: ax + galaxSpriteW, y1: ay + galaxSpriteH}
			if lr.overlaps(ar) {
				a.alive = false
				p.spawnExplosion(ax, ay, galaxSpriteW, galaxSpriteH, 0)
				if a.state == gsDiving {
					p.addScore(150)
				} else {
					p.addScore(80)
				}
				p.player.laser = nil
				return
			}
		}
	}
}

func (s *galaxState) collideDiversVsPlayer(p *playScene) {
	if p.player.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for r := 0; r < galaxRows; r++ {
		for c := 0; c < galaxCols; c++ {
			a := s.cells[r][c]
			if a == nil || !a.alive || a.state == gsFormation {
				continue
			}
			ar := rect{
				x0: int(a.x),
				y0: int(a.y),
				x1: int(a.x) + galaxSpriteW,
				y1: int(a.y) + galaxSpriteH,
			}
			if ar.overlaps(pr) {
				a.alive = false
				p.spawnExplosion(int(a.x), int(a.y), galaxSpriteW, galaxSpriteH, 0)
				p.playerHit()
				return
			}
		}
	}
}

func (s *galaxState) alienPos(p *playScene, a *galaxAlien) (int, int) {
	if a.state == gsFormation {
		return s.slotPos(p, a.row, a.col)
	}
	return int(a.x), int(a.y)
}

func (s *galaxState) alive() int {
	n := 0
	for r := 0; r < galaxRows; r++ {
		for c := 0; c < galaxCols; c++ {
			if s.cells[r][c] != nil && s.cells[r][c].alive {
				n++
			}
		}
	}
	return n
}

func (s *galaxState) draw(p *playScene, c *engine.Canvas) {
	for r := 0; r < galaxRows; r++ {
		for col := 0; col < galaxCols; col++ {
			a := s.cells[r][col]
			if a == nil || !a.alive {
				continue
			}
			x, y := s.alienPos(p, a)
			diving := a.state == gsDiving
			var spr sprite
			pal := galaxianPalette
			if diving {
				if s.frame == 0 {
					spr = galaxianDiveA
				} else {
					spr = galaxianDiveB
				}
				pal = galaxianDivePalette
			} else {
				if s.frame == 0 {
					spr = galaxianA
				} else {
					spr = galaxianB
				}
			}
			drawSprite(c, x, y, spr, pal)
		}
	}
}

func bezier(t, p0, p1, p2 float64) float64 {
	u := 1 - t
	return u*u*p0 + 2*u*t*p1 + t*t*p2
}
