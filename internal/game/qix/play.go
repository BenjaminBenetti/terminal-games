package qix

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// All tuning lives at the top. Speeds are in cells/sec. Distances are
// in cells. Times are in seconds. The constants were picked by feel
// against an 80×48 canvas (the engine's default for an 80×24 terminal).
const (
	// HUD layout: number of cell rows reserved at top and bottom for
	// score / lives / level. The playfield occupies the rest.
	hudTopRows    = 1
	hudBottomRows = 2
	// Pixel padding on left and right of the playfield.
	hudSidePad = 2
	// Minimum playfield size — under this the level isn't playable;
	// the game complains and refuses to start a match.
	minFieldW = 30
	minFieldH = 20
	// Maximum playfield size — cap so very large terminals don't
	// produce a field so big the player can't sweep across it.
	maxFieldW = 120
	maxFieldH = 64

	// Player movement speeds.
	edgeSpeed = 26.0
	fastSpeed = 32.0
	slowSpeed = 14.0

	// Scoring.
	pointsPerCellFast = 5
	pointsPerCellSlow = 12
	levelClearBonus   = 1000
	bonusLifeEvery    = 20000

	// Win threshold.
	targetPct = 75

	// Fuse parameters.
	fuseStartDelay = 0.6
	fuseSpeed      = 11.0

	// Sparx parameters. Speed grows per level so later levels feel
	// genuinely tighter, not just "more sparx".
	sparxBaseSpeed       = 11.0
	sparxLevelSpeedBoost = 0.7
	sparxLevelExtraEvery = 2 // +1 sparx every N levels (in addition to base 2)
	sparxBaseCount       = 2
	sparxMaxCount        = 6
	sparxSpawnGap        = 0.6 // seconds between sequential sparx emerging

	// Qix parameters.
	qixBaseSpeed       = 14.0
	qixLevelSpeedBoost = 1.2
	qixJoints          = 8

	// Lives & timing.
	startingLives    = 3
	deathDuration    = 1.4
	respawnDelay     = 0.8
	levelClearedDur  = 2.2
)

// playState is the gameplay sub-state machine.
type playState int

const (
	psPlaying      playState = iota
	psDying                  // brief death animation + drain trail
	psRespawning             // dead but waiting for next-life delay
	psLevelCleared           // hit target%; show bonus, then advance
	psGameOver
)

// drawMode is the speed/scoring class of the current player line.
type drawMode int

const (
	drawNone drawMode = iota
	drawFast
	drawSlow
)

// player is the marker the user controls. Position is in cell coords
// inside the playfield (0,0 = top-left of playfield).
type player struct {
	x, y       int
	moveAccum  float64
	dirX, dirY int // last cardinal step direction
	drawing    bool
	drawMode   drawMode
	trail      []point
	idleT      float64 // seconds since last successful step while drawing
	deathT     float64
	alive      bool
	// blinkT advances every frame for the post-respawn invulnerability
	// visual.
	invul float64
}

// playScene owns the full match state.
type playScene struct {
	e   *engine.Engine
	rng *rand.Rand

	field  *field
	player *player
	qix    *qixMonster
	sparx  []*sparx
	fuse   *fuse

	// Pixel offset of the playfield's top-left within the canvas.
	playX int
	playY int

	score     int
	hiScore   int
	lives     int
	level     int
	nextBonus int

	// Spawn pool of pending sparx for this life (so they enter staggered
	// from the spawn door instead of materialising in a clump).
	pendingSparx int
	sparxCDspawn float64

	state    playState
	stateT   float64

	// True once the player has signalled an explicit ESC/Q to bail
	// from a match back to the title.
	wantQuit bool

	// Per-level claim color so the playfield's filled regions don't
	// look identical from level to level.
	claimPalette []engine.Color
}

// newPlayScene constructs a playScene sized to the engine's canvas.
// Returns a scene whose first frame is the level-1 playfield with a
// fresh Qix and two sparx queued.
func newPlayScene(e *engine.Engine, hiScore int, rng *rand.Rand) *playScene {
	c := e.Canvas()

	// Playfield pixel area — top hud at top, bottom hud at bottom,
	// padded on the sides. Cell coords inside the playfield are 0-based.
	fw := c.Width() - 2*hudSidePad
	fh := c.Height() - 2*(hudTopRows+hudBottomRows)
	if fw < minFieldW {
		fw = minFieldW
	}
	if fh < minFieldH {
		fh = minFieldH
	}
	if fw > maxFieldW {
		fw = maxFieldW
	}
	if fh > maxFieldH {
		fh = maxFieldH
	}
	// Centre the (possibly-clamped) field inside the canvas.
	playX := (c.Width() - fw) / 2
	playY := 2*hudTopRows + (c.Height()-2*(hudTopRows+hudBottomRows)-fh)/2

	p := &playScene{
		e:         e,
		rng:       rng,
		hiScore:   hiScore,
		lives:     startingLives,
		level:     0,
		nextBonus: bonusLifeEvery,
		playX:     playX,
		playY:     playY,
		claimPalette: []engine.Color{
			{R: 30, G: 60, B: 160, A: 255},
			{R: 130, G: 30, B: 130, A: 255},
			{R: 30, G: 130, B: 60, A: 255},
			{R: 150, G: 80, B: 30, A: 255},
			{R: 60, G: 30, B: 150, A: 255},
		},
		player: &player{},
	}
	p.startLevel(1, fw, fh)
	return p
}

// startLevel resets the playfield, the Qix, the sparx, and the player
// to fresh-level state for the given level number.
func (p *playScene) startLevel(level, fw, fh int) {
	p.level = level
	p.field = newField(fw, fh)
	p.field.claimColor = p.claimPalette[(level-1)%len(p.claimPalette)]

	p.qix = newQix(
		p.rng,
		p.field,
		qixBaseSpeed+qixLevelSpeedBoost*float64(level-1),
		qixJoints,
	)

	// Sparx schedule: base count + 1 per N levels, capped.
	p.sparx = nil
	p.pendingSparx = sparxBaseCount + (level-1)/sparxLevelExtraEvery
	if p.pendingSparx > sparxMaxCount {
		p.pendingSparx = sparxMaxCount
	}
	p.sparxCDspawn = 0.8 // small grace before the first sparx emerges

	p.fuse = newFuse(fuseSpeed)
	p.resetPlayer(true)
	p.state = psPlaying
	p.stateT = 0
}

// resetPlayer puts the marker on the centre of the bottom border with
// a fresh trail, optionally giving a half-second of invulnerability so
// a sparx camping the spawn doesn't insta-kill on respawn.
func (p *playScene) resetPlayer(grace bool) {
	p.player.x = p.field.w / 2
	p.player.y = p.field.h - 1
	p.player.dirX = 0
	p.player.dirY = 0
	p.player.moveAccum = 0
	p.player.drawing = false
	p.player.drawMode = drawNone
	p.player.trail = nil
	p.player.idleT = 0
	p.player.alive = true
	if grace {
		p.player.invul = 1.2
	} else {
		p.player.invul = 0
	}
	if p.fuse != nil {
		p.fuse.extinguish()
	}
}

// spawnDoor returns the cell where sparx emerge (top centre of the
// playfield's outer rectangle).
func (p *playScene) spawnDoor() point {
	return point{p.field.w / 2, 0}
}

// trySpawnSparx releases one queued sparx if the spawn cooldown has
// expired. Sparx alternate CW/CCW so the two halves of the perimeter
// are both threatened.
func (p *playScene) trySpawnSparx(s float64) {
	if p.pendingSparx <= 0 {
		return
	}
	p.sparxCDspawn -= s
	if p.sparxCDspawn > 0 {
		return
	}
	door := p.spawnDoor()
	// Choose initial cardinal direction: the first sparx heads right,
	// the second heads left, then alternate. Both rules go in
	// opposing rotational directions so they cover the perimeter.
	turnRight := len(p.sparx)%2 == 0
	dirX := 1
	if !turnRight {
		dirX = -1
	}
	speed := sparxBaseSpeed + sparxLevelSpeedBoost*float64(p.level-1)
	p.sparx = append(p.sparx, newSparx(door.x, door.y, dirX, 0, speed, turnRight))
	p.pendingSparx--
	p.sparxCDspawn = sparxSpawnGap
}

// ----------------------------------------------------------------------
// Update
// ----------------------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psPlaying:
		p.tickPlaying(s)
	case psDying:
		// World keeps ticking visually so death reads. The fuse and
		// sparx freeze; the Qix drifts so the level doesn't look paused.
		p.qix.tick(s, p.field)
		if p.stateT >= deathDuration {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.state = psRespawning
				p.stateT = 0
				p.resetPlayer(true)
				// Reset sparx to the door for the new life.
				p.respawnSparx()
			}
		}
	case psRespawning:
		p.qix.tick(s, p.field)
		if p.stateT >= respawnDelay {
			p.state = psPlaying
			p.stateT = 0
		}
	case psLevelCleared:
		p.qix.tick(s, p.field)
		if p.stateT >= levelClearedDur {
			c := p.e.Canvas()
			fw := c.Width() - 2*hudSidePad
			fh := c.Height() - 2*(hudTopRows+hudBottomRows)
			if fw > maxFieldW {
				fw = maxFieldW
			}
			if fh > maxFieldH {
				fh = maxFieldH
			}
			p.startLevel(p.level+1, fw, fh)
		}
	case psGameOver:
		// Wait for the player to acknowledge.
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// respawnSparx puts each existing sparx back at the spawn door with
// fresh direction state. The pending-sparx counter is left alone — if
// the previous life didn't release all of them, they're still queued.
func (p *playScene) respawnSparx() {
	door := p.spawnDoor()
	for i, sp := range p.sparx {
		dirX := 1
		if i%2 == 1 {
			dirX = -1
		}
		sp.x = door.x
		sp.y = door.y
		sp.prevX = door.x - dirX
		sp.prevY = door.y
		sp.dirX = dirX
		sp.dirY = 0
		sp.accum = 0
	}
}

func (p *playScene) tickPlaying(s float64) {
	// Spawn pacing for queued sparx.
	p.trySpawnSparx(s)

	// Movement input.
	dx, dy, fastMod, slowMod := p.readInputDir()

	p.tickPlayer(s, dx, dy, fastMod, slowMod)
	if p.state != psPlaying {
		return
	}

	p.tickFuse(s)
	if p.state != psPlaying {
		return
	}

	for _, sp := range p.sparx {
		sp.tick(s, p.field)
	}
	p.qix.tick(s, p.field)

	p.player.invul = math.Max(0, p.player.invul-s)

	p.resolveLethalChecks()
	if p.state != psPlaying {
		return
	}

	// Win condition: claimed enough.
	if p.field.percentClaimed() >= targetPct {
		p.score += levelClearBonus
		p.checkBonusLife()
		p.state = psLevelCleared
		p.stateT = 0
	}
}

func (p *playScene) readInputDir() (dx, dy int, fast, slow bool) {
	e := p.e
	if e.IsKeyDown(engine.KeyLeft) {
		dx--
	}
	if e.IsKeyDown(engine.KeyRight) {
		dx++
	}
	if e.IsKeyDown(engine.KeyUp) {
		dy--
	}
	if e.IsKeyDown(engine.KeyDown) {
		dy++
	}
	// Disallow diagonals: when both axes are held, pick perpendicular
	// to the player's current heading. Without a current heading
	// (just spawned, never moved), prefer horizontal.
	if dx != 0 && dy != 0 {
		if p.player.dirX != 0 {
			dx = 0
		} else if p.player.dirY != 0 {
			dy = 0
		} else {
			dy = 0
		}
	}
	fast = e.IsCharDown('z') || e.IsCharDown('Z') || e.IsCharDown(' ')
	slow = e.IsCharDown('x') || e.IsCharDown('X')
	return
}

// tickPlayer steps the marker. Movement is cell-discrete with a
// sub-cell accumulator so the per-frame fractional movement adds up
// cleanly into single-cell hops at the chosen speed.
func (p *playScene) tickPlayer(s float64, dx, dy int, fastMod, slowMod bool) {
	pl := p.player
	if !pl.alive {
		return
	}

	// What speed should we move at?
	speed := edgeSpeed
	if pl.drawing {
		switch pl.drawMode {
		case drawFast:
			speed = fastSpeed
		case drawSlow:
			speed = slowSpeed
		}
	}

	pl.moveAccum += speed * s

	moved := false
	// We pop full cells of accumulated movement, attempting one step per
	// pop. Once a step fails (blocked or no direction held) we drop out;
	// banked progress isn't preserved across frames — saves us from
	// surprise teleports after long blocks.
	for pl.moveAccum >= 1 {
		pl.moveAccum -= 1
		if dx == 0 && dy == 0 {
			break
		}
		stepped, completed, claimed := p.tryStep(dx, dy, fastMod, slowMod)
		if stepped {
			moved = true
		}
		if !stepped {
			break
		}
		if completed {
			p.handleClaim(claimed)
			break
		}
	}

	if moved {
		pl.idleT = 0
		if p.fuse != nil && p.fuse.alive {
			p.fuse.extinguish()
		}
	} else if pl.drawing {
		// Drawing but no progress this frame — accumulate idle time
		// for fuse spawn.
		pl.idleT += s
		if pl.idleT >= fuseStartDelay && p.fuse != nil && !p.fuse.alive {
			p.fuse.spawn()
		}
	}
}

// tryStep is the workhorse for a single 1-cell hop. Returns:
//
//	stepped — did the player actually move this call?
//	completed — did this step close a polyline (player hit border)?
//	claimed — number of cells flipped to claimed (only nonzero when completed).
//
// Movement rules:
//   - Not drawing + step into border → slide along border.
//   - Not drawing + step into open + fast/slow held → start drawing.
//   - Drawing + step into open → extend the line.
//   - Drawing + step into the immediate-previous trail cell → undraw.
//   - Drawing + step into border → complete the polyline.
//   - Anything else → blocked.
func (p *playScene) tryStep(dx, dy int, fastMod, slowMod bool) (stepped, completed bool, claimed int) {
	pl := p.player
	nx, ny := pl.x+dx, pl.y+dy
	if !p.field.inBounds(nx, ny) {
		return false, false, 0
	}
	target := p.field.at(nx, ny)

	if !pl.drawing {
		switch target {
		case cellBorder:
			pl.x, pl.y = nx, ny
			pl.dirX, pl.dirY = dx, dy
			return true, false, 0
		case cellOpen:
			if !(fastMod || slowMod) {
				return false, false, 0
			}
			mode := drawFast
			if slowMod && !fastMod {
				mode = drawSlow
			}
			// Start drawing: include the originating border cell as
			// trail[0]; the new step is the first cellDraw.
			pl.drawing = true
			pl.drawMode = mode
			pl.trail = []point{{pl.x, pl.y}, {nx, ny}}
			p.field.set(nx, ny, cellDraw)
			pl.x, pl.y = nx, ny
			pl.dirX, pl.dirY = dx, dy
			return true, false, 0
		default:
			return false, false, 0
		}
	}

	// Drawing mode.
	if len(pl.trail) >= 2 {
		prev := pl.trail[len(pl.trail)-2]
		if nx == prev.x && ny == prev.y {
			head := pl.trail[len(pl.trail)-1]
			if p.field.isDraw(head.x, head.y) {
				p.field.set(head.x, head.y, cellOpen)
			}
			pl.trail = pl.trail[:len(pl.trail)-1]
			pl.x, pl.y = nx, ny
			pl.dirX, pl.dirY = dx, dy
			if len(pl.trail) <= 1 {
				// Walked all the way back to the start — cancel the
				// in-progress draw entirely.
				pl.drawing = false
				pl.drawMode = drawNone
				pl.trail = nil
				if p.fuse != nil {
					p.fuse.extinguish()
				}
			}
			return true, false, 0
		}
	}

	switch target {
	case cellOpen:
		p.field.set(nx, ny, cellDraw)
		pl.trail = append(pl.trail, point{nx, ny})
		pl.x, pl.y = nx, ny
		pl.dirX, pl.dirY = dx, dy
		return true, false, 0
	case cellBorder:
		// Closing the loop. Add the terminus to the trail; the flood
		// fill that runs against this trail's draw cells will treat
		// every cellBorder (including the terminus and origin) as wall.
		pl.trail = append(pl.trail, point{nx, ny})
		pl.x, pl.y = nx, ny
		pl.dirX, pl.dirY = dx, dy
		mode := pl.drawMode
		probes := p.qix.jointCells()
		claimedCells := p.field.resolveClaim(pl.trail, probes)
		// Reset drawing state.
		pl.drawing = false
		pl.drawMode = drawNone
		pl.trail = nil
		if p.fuse != nil {
			p.fuse.extinguish()
		}
		// Score the claimed area.
		per := pointsPerCellFast
		if mode == drawSlow {
			per = pointsPerCellSlow
		}
		return true, true, claimedCells * per
	case cellDraw, cellClaimed:
		// Blocked by self-line or claimed wall.
		return false, false, 0
	}
	return false, false, 0
}

func (p *playScene) handleClaim(deltaScore int) {
	if deltaScore <= 0 {
		return
	}
	p.score += deltaScore
	p.checkBonusLife()
}

func (p *playScene) checkBonusLife() {
	for p.score >= p.nextBonus {
		p.lives++
		p.nextBonus += bonusLifeEvery
	}
}

// tickFuse advances the fuse if it's alive. If it catches the player's
// trail head, the player dies. The fuse is extinguished by the player
// moving — that happens in tickPlayer above before tickFuse runs.
func (p *playScene) tickFuse(s float64) {
	if p.fuse == nil || !p.fuse.alive || !p.player.drawing {
		return
	}
	p.fuse.tick(s, len(p.player.trail))
	if p.fuse.caughtPlayer(p.player.trail) {
		p.killPlayer()
	}
}

// resolveLethalChecks runs every collision that can kill the player
// after the per-entity ticks: Qix vs draw line, Qix vs player while
// drawing, sparx vs player.
func (p *playScene) resolveLethalChecks() {
	pl := p.player
	if !pl.alive || pl.invul > 0 {
		return
	}

	// Qix touching the drawn line — instant death.
	if pl.drawing && p.qix.touchesDraw(p.field) {
		p.killPlayer()
		return
	}

	// Qix touching the player's current cell — only matters when
	// drawing (because the player is on cellOpen / cellDraw there).
	// On the border the Qix can't reach without first passing through
	// a non-open cell, so it's not a worry.
	if pl.drawing && p.qix.touchesCell(pl.x, pl.y) {
		p.killPlayer()
		return
	}

	// Sparx touching the player — always lethal.
	for _, sp := range p.sparx {
		if sp.x == pl.x && sp.y == pl.y {
			p.killPlayer()
			return
		}
	}
}

// killPlayer triggers the death transition. Call sites are at each
// distinct loss condition: Qix hitting the draw line, the Qix's body
// catching the player mid-draw, a sparx touching the player, and the
// fuse catching the trail head.
func (p *playScene) killPlayer() {
	if !p.player.alive {
		return
	}
	p.player.alive = false
	p.lives--
	// Cancel the in-progress draw, reverting cellDraw → cellOpen so
	// the field is consistent again.
	if p.player.drawing {
		p.field.cancelDraw(p.player.trail)
		p.player.drawing = false
		p.player.drawMode = drawNone
		p.player.trail = nil
	}
	if p.fuse != nil {
		p.fuse.extinguish()
	}
	p.state = psDying
	p.stateT = 0
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying, psDying, psRespawning, psLevelCleared:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psGameOver:
			switch k.Code {
			case engine.KeyEnter:
				p.restart()
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case ' ', 'r', 'R':
					p.restart()
				}
			}
		}
	}
}

func (p *playScene) restart() {
	hi := p.hiScore
	rng := p.rng
	e := p.e
	playX, playY := p.playX, p.playY
	palette := p.claimPalette
	*p = playScene{
		e:            e,
		rng:          rng,
		hiScore:      hi,
		lives:        startingLives,
		level:        0,
		nextBonus:    bonusLifeEvery,
		playX:        playX,
		playY:        playY,
		claimPalette: palette,
		player:       &player{},
	}
	c := e.Canvas()
	fw := c.Width() - 2*hudSidePad
	fh := c.Height() - 2*(hudTopRows+hudBottomRows)
	if fw < minFieldW {
		fw = minFieldW
	}
	if fh < minFieldH {
		fh = minFieldH
	}
	if fw > maxFieldW {
		fw = maxFieldW
	}
	if fh > maxFieldH {
		fh = maxFieldH
	}
	p.startLevel(1, fw, fh)
}

// ----------------------------------------------------------------------
// Draw
// ----------------------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)
	p.drawHUD(c)
	p.drawField(c)
	p.drawQix(c)
	p.drawSparx(c)
	p.drawFuse(c)
	p.drawPlayer(c)
	p.drawOverlays(c)
}

// drawField paints every cell of the playfield. cellOpen renders as a
// subtle dark fill (not pure black) so the playfield reads as a
// distinct region even before any claims happen.
func (p *playScene) drawField(c *engine.Canvas) {
	f := p.field
	openCol := engine.Color{R: 6, G: 6, B: 14, A: 255}
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			var col engine.Color
			switch f.cells[f.idx(x, y)] {
			case cellOpen:
				col = openCol
			case cellClaimed:
				col = f.claimColor
			case cellBorder:
				col = f.borderColor
			case cellDraw:
				col = f.drawFastColor
				if p.player.drawMode == drawSlow {
					col = f.drawSlowColor
				}
			}
			c.Set(p.playX+x, p.playY+y, col)
		}
	}
	// Highlight the sparx spawn door — small notch at the top so the
	// player knows where sparx come from.
	door := p.spawnDoor()
	dcol := engine.Color{R: 255, G: 220, B: 80, A: 255}
	c.Set(p.playX+door.x, p.playY+door.y, dcol)
}

func (p *playScene) drawQix(c *engine.Canvas) {
	if p.qix == nil {
		return
	}
	p.qix.draw(c, p.playX, p.playY)
}

func (p *playScene) drawSparx(c *engine.Canvas) {
	for i, sp := range p.sparx {
		// Two-tone sparx: even-indexed pink, odd-indexed yellow so the
		// pair is visually distinguishable.
		col := engine.Color{R: 255, G: 110, B: 180, A: 255}
		if i%2 == 1 {
			col = engine.Color{R: 255, G: 220, B: 80, A: 255}
		}
		sp.draw(c, p.playX, p.playY, col)
	}
}

func (p *playScene) drawFuse(c *engine.Canvas) {
	if p.fuse == nil || !p.fuse.alive {
		return
	}
	flicker := int(p.stateT*16)%2 == 0
	p.fuse.draw(c, p.player.trail, p.playX, p.playY, flicker)
}

// drawPlayer paints the marker as a 3-pixel cross — small enough to
// not eclipse the cell it's on, big enough to read at 1:1 cell scale.
// During invulnerability the marker blinks.
func (p *playScene) drawPlayer(c *engine.Canvas) {
	pl := p.player
	if !pl.alive {
		return
	}
	if pl.invul > 0 {
		if int(pl.invul*20)%2 == 0 {
			return
		}
	}
	col := engine.Color{R: 80, G: 240, B: 240, A: 255}
	x := p.playX + pl.x
	y := p.playY + pl.y
	c.Set(x, y, col)
	c.Set(x-1, y, col)
	c.Set(x+1, y, col)
	c.Set(x, y-1, col)
	c.Set(x, y+1, col)
}

// drawHUD paints the per-frame status bars: score and target along
// the top; lives, claimed%, and level along the bottom.
func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	rows := c.Rows()

	scoreText := fmt.Sprintf("SCORE %s", zeroPad(p.score, 6))
	hiText := "HI " + zeroPad(p.hiScore, 6)
	levelText := fmt.Sprintf("LEVEL %d", p.level)

	c.Print(1, 0, scoreText, engine.White)
	c.Print((cols-len(hiText))/2, 0, hiText, engine.Yellow)
	c.Print(cols-len(levelText)-1, 0, levelText, engine.Cyan)

	pct := p.field.percentClaimed()
	progressCol := engine.White
	if pct >= targetPct {
		progressCol = engine.Green
	} else if pct >= targetPct*2/3 {
		progressCol = engine.Yellow
	}
	progressText := fmt.Sprintf("CLAIMED %d%%   TARGET %d%%", pct, targetPct)
	c.Print((cols-len(progressText))/2, rows-2, progressText, progressCol)

	// Lives at bottom-left, controls hint at bottom-right.
	livesText := fmt.Sprintf("LIVES %d", p.lives)
	c.Print(1, rows-1, livesText, engine.Color{R: 255, G: 130, B: 130, A: 255})
	// Bonus-life threshold left as a hint of how scoring works.
	if p.nextBonus-p.score < bonusLifeEvery {
		next := p.nextBonus - p.score
		if next > 0 {
			extra := fmt.Sprintf("NEXT LIFE %d", next)
			c.Print(1+len(livesText)+3, rows-1, extra, engine.Gray)
		}
	}
	hint := "Z FAST  X SLOW  ESC QUIT"
	c.Print(cols-len(hint)-1, rows-1, hint, engine.Gray)
}

// drawOverlays paints any banner appropriate for the current state.
func (p *playScene) drawOverlays(c *engine.Canvas) {
	switch p.state {
	case psLevelCleared:
		msg := fmt.Sprintf("LEVEL %d CLEAR", p.level)
		p.drawBanner(c, msg, engine.Yellow, "+"+fmt.Sprintf("%d", levelClearBonus))
	case psGameOver:
		p.drawGameOver(c)
	case psDying:
		// Brief "lost a life" subtext.
		if int(p.stateT*4)%2 == 0 {
			msg := "OUCH!"
			p.drawBanner(c, msg, engine.Color{R: 255, G: 90, B: 90, A: 255}, "")
		}
	}
}

func (p *playScene) drawBanner(c *engine.Canvas, text string, col engine.Color, sub string) {
	w := engine.TextWidth(text)
	x := (c.Width() - w) / 2
	y := (c.Height() - engine.FontHeight) / 2
	c.FillRect(x-4, y-3, w+8, engine.FontHeight+6, engine.Color{R: 8, G: 8, B: 18, A: 255})
	c.DrawText(x, y, text, col)
	if sub != "" {
		c.Print((c.Cols()-len(sub))/2, y/2+engine.FontHeight/2+2, sub, engine.White)
	}
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (c.Width() - w) / 2
	y := (c.Height() - engine.FontHeight) / 2
	c.FillRect(x-6, y-3, w+12, engine.FontHeight+6, engine.Color{R: 8, G: 8, B: 18, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 80, B: 80, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}
