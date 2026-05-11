package spaceinvaders

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Tuning constants. Speeds are in pixels per second; intervals in seconds.
const (
	playerSpeed       = 36.0
	playerFireGap     = 0.10
	playerBulletSpeed = 70.0
	playerExplodeDur  = 1.1

	alienBulletSpeed   = 26.0
	alienFrameInterval = 0.45
	alienBaseInterval  = 0.90 // step interval at full health
	alienMinInterval   = 0.06 // step interval at one alien remaining
	alienDropPx        = 2
	alienHStepPx       = 2
	alienFireMin       = 0.55
	alienFireMax       = 1.9
	maxAlienBullets    = 4
	waveSpeedup        = 0.85 // each wave multiplies step intervals by this

	ufoSpeed       = 18.0
	ufoSpawnMinDur = 14.0
	ufoSpawnMaxDur = 28.0

	waveClearDelay = 1.6
)

// playState is the gameplay sub-state machine. The top-level scene only
// distinguishes menu / play / gameover; this is internal to playScene.
type playState int

const (
	psPlaying playState = iota
	psPlayerHit
	psWaveCleared
	psGameOver
)

// alienKind selects the sprite and score value for a cell in the
// formation. The classic shooter formula is one row of high-value
// aliens, two rows of mid, two rows of low.
type alienKind int

const (
	alienTopKind alienKind = iota
	alienMidKind
	alienBotKind
)

func (k alienKind) score() int {
	switch k {
	case alienTopKind:
		return 30
	case alienMidKind:
		return 20
	default:
		return 10
	}
}

func (k alienKind) color() engine.Color {
	switch k {
	case alienTopKind:
		return engine.Color{R: 240, G: 220, B: 90, A: 255}
	case alienMidKind:
		return engine.Color{R: 90, G: 220, B: 240, A: 255}
	default:
		return engine.Color{R: 90, G: 230, B: 120, A: 255}
	}
}

func (k alienKind) frames() (sprite, sprite) {
	switch k {
	case alienTopKind:
		return alienTopA, alienTopB
	case alienMidKind:
		return alienMidA, alienMidB
	default:
		return alienBotA, alienBotB
	}
}

// alienCell is a single alien slot in the formation.
type alienCell struct {
	alive bool
	kind  alienKind
}

// alienGrid is the marching formation. The grid origin moves; individual
// alien pixel positions are derived from row/col + origin + pitch.
type alienGrid struct {
	cols, rows int
	cells      [][]alienCell // [row][col]
	originX    float64       // pixel x of column 0's left edge
	originY    float64       // pixel y of row 0's top edge
	colPitch   int
	rowPitch   int
	spriteW    int
	spriteH    int
	dir        int     // +1 right, -1 left
	pendingDrop bool   // next step should be a drop+reverse rather than horizontal
	stepT      float64 // elapsed time since last step
	frame      int
	frameT     float64
	alive      int // count of live cells
	total      int // initial cell count
}

// alienPos returns the pixel (x, y) of the top-left of the alien at row r,
// col c, taking the current origin into account.
func (g *alienGrid) alienPos(r, c int) (int, int) {
	return int(g.originX) + c*g.colPitch, int(g.originY) + r*g.rowPitch
}

// leftRightOccupied returns the leftmost and rightmost column indices
// that contain at least one live alien. (-1, -1) when none.
func (g *alienGrid) leftRightOccupied() (int, int) {
	left, right := -1, -1
	for c := 0; c < g.cols; c++ {
		anyAlive := false
		for r := 0; r < g.rows; r++ {
			if g.cells[r][c].alive {
				anyAlive = true
				break
			}
		}
		if anyAlive {
			if left == -1 {
				left = c
			}
			right = c
		}
	}
	return left, right
}

// bottomOccupiedRow returns the row index of the deepest live alien, or -1.
func (g *alienGrid) bottomOccupiedRow() int {
	for r := g.rows - 1; r >= 0; r-- {
		for c := 0; c < g.cols; c++ {
			if g.cells[r][c].alive {
				return r
			}
		}
	}
	return -1
}

// stepInterval shrinks as more aliens die, producing the classic
// accelerating march.
func (g *alienGrid) stepInterval(waveScale float64) float64 {
	if g.total == 0 {
		return alienBaseInterval
	}
	t := float64(g.alive) / float64(g.total) // 1.0 -> 0.0
	iv := alienMinInterval + (alienBaseInterval-alienMinInterval)*t
	return iv * waveScale
}

// bullet is a single projectile. fromPlayer flips both the colour and
// the collision rules; vy is positive when falling (alien shot) and
// negative when rising (player shot).
type bullet struct {
	x          float64
	y          float64
	vy         float64
	fromPlayer bool
	kind       int // alien-bullet variant
	frame      int
	frameT     float64
}

// bunkerEntity is a destructible shield. The mask is per-pixel; a hit
// erodes a small splotch around the impact point.
type bunkerEntity struct {
	x, y  int
	w, h  int
	mask  [][]bool
	color engine.Color
}

func newBunker(x, y int) *bunkerEntity {
	w := bunkerSprite.width()
	h := bunkerSprite.height()
	b := &bunkerEntity{
		x:     x,
		y:     y,
		w:     w,
		h:     h,
		color: engine.Color{R: 100, G: 220, B: 120, A: 255},
	}
	b.mask = make([][]bool, h)
	for row, line := range bunkerSprite {
		b.mask[row] = make([]bool, w)
		for col := 0; col < len(line) && col < w; col++ {
			if line[col] == '#' {
				b.mask[row][col] = true
			}
		}
	}
	return b
}

// solidAt reports whether (px, py) (canvas pixel coords) is currently a
// solid pixel of this bunker.
func (b *bunkerEntity) solidAt(px, py int) bool {
	lx := px - b.x
	ly := py - b.y
	if lx < 0 || ly < 0 || lx >= b.w || ly >= b.h {
		return false
	}
	return b.mask[ly][lx]
}

// erode knocks out a small chunk of pixels around (px, py) on this bunker.
// The pattern is asymmetric so successive hits don't perfectly retrace
// the same wear pattern.
func (b *bunkerEntity) erode(px, py int, fromBelow bool) {
	lx := px - b.x
	ly := py - b.y
	radius := 3
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			// A jittery diamond, biased toward the direction the bullet came from.
			d := absInt(dx) + absInt(dy)
			if d > radius {
				continue
			}
			if fromBelow && dy > 1 {
				continue
			}
			if !fromBelow && dy < -1 {
				continue
			}
			y := ly + dy
			x := lx + dx
			if y >= 0 && y < b.h && x >= 0 && x < b.w {
				b.mask[y][x] = false
			}
		}
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ufoEntity is the mystery saucer that drifts across the top.
type ufoEntity struct {
	active     bool
	x          float64
	y          int
	vx         float64
	score      int
	nextSpawnT float64
}

// playerEntity is the defender cannon at the bottom of the screen.
type playerEntity struct {
	x        float64 // sprite-left X
	y        int     // sprite top Y (fixed)
	moveDir  int     // -1, 0, +1, recomputed each frame from key state
	cooldown float64
	bullet   *bullet // single in-flight bullet
	lives    int
	explodeT float64 // remaining explosion duration; >0 means exploding
}

// playScene contains the full gameplay state — entities, scoring, and the
// timing of the playState micro state-machine.
type playScene struct {
	e    *engine.Engine
	w, h int

	player       playerEntity
	grid         alienGrid
	alienBullets []*bullet
	bunkers      []*bunkerEntity
	ufo          ufoEntity

	score   int
	hiScore int
	wave    int

	state  playState
	stateT float64

	alienFireTimer float64
	rng            *rand.Rand

	// Layout (filled by computeLayout).
	hudRows  int
	alienY0  int
	bunkerY  int
	playerY  int
	loseY    int // an alien sprite reaching this Y triggers game over

	// Quit signal — top-level scene checks this in Update.
	wantQuit bool
}

// newPlayScene constructs a play scene sized to the engine's canvas.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		hiScore: hiScore,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.player.lives = 3
	p.computeLayout()
	p.startWave(1, true)
	return p
}

// computeLayout derives Y bands for HUD, aliens, bunkers, and the player
// from the canvas dimensions so the game scales (within reason) with
// the terminal size.
func (p *playScene) computeLayout() {
	p.hudRows = 2
	// Reserve the very bottom pixel for a thin ground strip; place the
	// cannon one pixel above so it doesn't get painted over.
	p.playerY = p.h - playerSprite.height() - 1
	bunkerH := bunkerSprite.height()
	p.bunkerY = p.playerY - bunkerH - 2
	if p.bunkerY < p.hudRows*2+bunkerSprite.height()+8 {
		p.bunkerY = p.hudRows*2 + bunkerSprite.height() + 8
	}
	p.alienY0 = p.hudRows*2 + 2
	// Game over when an alien's sprite bottom reaches the cannon's top.
	p.loseY = p.playerY
}

// startWave (re)initialises the formation, bunkers, and UFO for wave n.
// keepBunkers preserves the bunker damage state — true at game start,
// false between waves (fresh shields).
func (p *playScene) startWave(wave int, keepBunkers bool) {
	p.wave = wave
	p.state = psPlaying
	p.stateT = 0
	p.alienBullets = nil
	p.player.bullet = nil
	p.player.cooldown = 0
	p.player.explodeT = 0
	p.player.moveDir = 0
	p.player.x = float64(p.w-playerSprite.width()) / 2
	p.player.y = p.playerY

	// Formation sizing — adapt to canvas width so the formation has room
	// to march. 8-px wide aliens with 1 px gap; 5 rows always.
	spriteW := 8
	spriteH := 5
	colPitch := spriteW + 1
	rowPitch := spriteH + 1
	margin := 6
	cols := (p.w - 2*margin + 1) / colPitch
	if cols < 4 {
		cols = 4
	}
	if cols > 11 {
		cols = 11
	}
	rows := 5
	formationW := (cols-1)*colPitch + spriteW
	originX := float64(p.w-formationW) / 2
	originY := float64(p.alienY0 + (wave-1)*2)
	// Cap so we don't spawn aliens already touching the player.
	maxY := float64(p.bunkerY - rows*rowPitch - 2)
	if originY > maxY {
		originY = maxY
	}

	g := alienGrid{
		cols:     cols,
		rows:     rows,
		cells:    make([][]alienCell, rows),
		originX:  originX,
		originY:  originY,
		colPitch: colPitch,
		rowPitch: rowPitch,
		spriteW:  spriteW,
		spriteH:  spriteH,
		dir:      1,
	}
	for r := 0; r < rows; r++ {
		var kind alienKind
		switch {
		case r == 0:
			kind = alienTopKind
		case r <= 2:
			kind = alienMidKind
		default:
			kind = alienBotKind
		}
		g.cells[r] = make([]alienCell, cols)
		for c := 0; c < cols; c++ {
			g.cells[r][c] = alienCell{alive: true, kind: kind}
		}
	}
	g.alive = rows * cols
	g.total = g.alive
	p.grid = g

	// Bunkers.
	if !keepBunkers || len(p.bunkers) == 0 {
		bunkerW := bunkerSprite.width()
		count := 4
		if p.w < 60 {
			count = 3
		}
		gap := (p.w - count*bunkerW) / (count + 1)
		if gap < 2 {
			gap = 2
		}
		p.bunkers = nil
		for i := 0; i < count; i++ {
			bx := gap + i*(bunkerW+gap)
			p.bunkers = append(p.bunkers, newBunker(bx, p.bunkerY))
		}
	}

	// UFO timer.
	p.ufo = ufoEntity{
		active:     false,
		nextSpawnT: ufoSpawnMinDur + p.rng.Float64()*(ufoSpawnMaxDur-ufoSpawnMinDur),
	}
	p.alienFireTimer = alienFireMin + p.rng.Float64()*(alienFireMax-alienFireMin)
}

// waveScale returns the speed multiplier for the current wave: higher
// waves shrink the step interval, so the formation marches faster from
// the start.
func (p *playScene) waveScale() float64 {
	scale := 1.0
	for i := 1; i < p.wave; i++ {
		scale *= waveSpeedup
	}
	return scale
}

// Update advances the play state by dt seconds. It drains pending input
// at the top, then steps each entity, then runs collision resolution.
func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		// Let the top-level scene decide what wantQuit means (return to
		// title) instead of exiting the engine entirely.
		return nil
	}

	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psPlaying:
		p.updatePlaying(s)
	case psPlayerHit:
		p.player.explodeT -= s
		// Still let alien bullets and aliens animate slowly for life.
		p.tickAlienAnimation(s)
		p.tickBullets(s)
		if p.player.explodeT <= 0 {
			if p.player.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.player.x = float64(p.w-playerSprite.width()) / 2
				p.player.explodeT = 0
				p.state = psPlaying
				p.stateT = 0
			}
		}
	case psWaveCleared:
		if p.stateT >= waveClearDelay {
			p.startWave(p.wave+1, false)
		}
	case psGameOver:
		// Wait for the player to acknowledge with Enter or ESC.
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// handleInput drains the engine's key queue and applies the resulting
// player intent / state-transition presses.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psPlayerHit:
			// ESC still quits even while exploding.
			if k.Code == engine.KeyEsc {
				p.wantQuit = true
			}
		case psGameOver, psWaveCleared:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				if p.state == psGameOver {
					// Restart from wave 1 with fresh lives but keep hi-score.
					hi := p.hiScore
					p.score = 0
					p.player.lives = 3
					p.bunkers = nil
					p.hiScore = hi
					p.startWave(1, false)
				}
			}
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		}
	}
}

// handlePlayKey handles discrete (event-driven) actions during gameplay:
// firing and quitting. Movement is held-state and polled via IsKeyDown in
// updatePlaying, so Left/Right/A/D are intentionally absent from this
// switch — handling them here would re-introduce the move-vs-shoot
// conflict the latch hack used to suffer from.
func (p *playScene) handlePlayKey(k engine.Key) {
	switch k.Code {
	case engine.KeyChar:
		switch k.Rune {
		case ' ':
			p.tryFire()
		case 'q', 'Q':
			p.wantQuit = true
		}
	case engine.KeyEsc:
		p.wantQuit = true
	}
}

func (p *playScene) tryFire() {
	if p.player.bullet != nil {
		return
	}
	if p.player.cooldown > 0 {
		return
	}
	bx := p.player.x + float64(playerSprite.width())/2
	by := float64(p.player.y) - float64(playerBulletSprite.height())
	p.player.bullet = &bullet{
		x:          bx,
		y:          by,
		vy:         -playerBulletSpeed,
		fromPlayer: true,
	}
	p.player.cooldown = playerFireGap
}

// updatePlaying is the main per-frame logic for the active gameplay
// state: move player, aliens, and bullets, then resolve collisions.
func (p *playScene) updatePlaying(s float64) {
	// --- Player movement ----------------------------------------------
	//
	// Movement is held-key state, so poll IsKeyDown / IsCharDown every
	// frame rather than driving it off the discrete event queue. This is
	// what lets the player move and shoot at the same time: the OS only
	// auto-repeats the most-recent key (usually Space while firing), but
	// the held arrow / WASD key still reads as down via Kitty release
	// events (or the legacy decay fallback).
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	switch {
	case left && !right:
		p.player.moveDir = -1
	case right && !left:
		p.player.moveDir = 1
	default:
		p.player.moveDir = 0
	}
	if p.player.moveDir != 0 {
		p.player.x += float64(p.player.moveDir) * playerSpeed * s
	}
	maxX := float64(p.w - playerSprite.width())
	if p.player.x < 0 {
		p.player.x = 0
	}
	if p.player.x > maxX {
		p.player.x = maxX
	}
	if p.player.cooldown > 0 {
		p.player.cooldown -= s
	}

	// --- Alien grid ---------------------------------------------------
	p.tickAlienAnimation(s)
	p.tickAlienStep(s)

	// --- Bullets ------------------------------------------------------
	p.tickBullets(s)

	// --- UFO ----------------------------------------------------------
	p.tickUFO(s)

	// --- Alien fire ---------------------------------------------------
	p.alienFireTimer -= s
	if p.alienFireTimer <= 0 {
		p.alienFire()
		p.alienFireTimer = alienFireMin + p.rng.Float64()*(alienFireMax-alienFireMin)
	}

	// --- Collisions and end conditions --------------------------------
	p.resolveCollisions()
	if p.grid.alive == 0 {
		p.state = psWaveCleared
		p.stateT = 0
	}
	if p.alienReachedPlayer() {
		// Skip the explosion animation; the player is overrun.
		p.player.lives = 0
		p.player.explodeT = playerExplodeDur
		p.state = psPlayerHit
		p.stateT = 0
	}
}

func (p *playScene) tickAlienAnimation(s float64) {
	p.grid.frameT += s
	if p.grid.frameT >= alienFrameInterval {
		p.grid.frameT -= alienFrameInterval
		p.grid.frame = 1 - p.grid.frame
	}
}

// tickAlienStep advances the formation by one column (or a drop + reverse)
// once its timer elapses. Drops are deferred from the previous step so
// the side-hit-then-drop sequence reads naturally.
func (p *playScene) tickAlienStep(s float64) {
	p.grid.stepT += s
	iv := p.grid.stepInterval(p.waveScale())
	if p.grid.stepT < iv {
		return
	}
	p.grid.stepT = 0

	if p.grid.pendingDrop {
		p.grid.originY += float64(alienDropPx)
		p.grid.dir = -p.grid.dir
		p.grid.pendingDrop = false
		return
	}

	// Horizontal step. Check if it would push the formation past the
	// canvas edges; if so, queue a drop+reverse for the next step.
	left, right := p.grid.leftRightOccupied()
	if left < 0 {
		return
	}
	step := float64(alienHStepPx * p.grid.dir)
	leftX := p.grid.originX + float64(left*p.grid.colPitch) + step
	rightX := p.grid.originX + float64(right*p.grid.colPitch) + step + float64(p.grid.spriteW)
	if leftX < 1 || rightX > float64(p.w-1) {
		p.grid.pendingDrop = true
		return
	}
	p.grid.originX += step
}

func (p *playScene) tickBullets(s float64) {
	if b := p.player.bullet; b != nil {
		b.y += b.vy * s
		if b.y+float64(playerBulletSprite.height()) < 0 {
			p.player.bullet = nil
		}
	}
	kept := p.alienBullets[:0]
	for _, b := range p.alienBullets {
		b.y += b.vy * s
		b.frameT += s
		if b.frameT >= 0.12 {
			b.frameT = 0
			b.frame = 1 - b.frame
		}
		if b.y < float64(p.h) {
			kept = append(kept, b)
		}
	}
	p.alienBullets = kept
}

func (p *playScene) tickUFO(s float64) {
	if !p.ufo.active {
		p.ufo.nextSpawnT -= s
		if p.ufo.nextSpawnT <= 0 {
			p.spawnUFO()
		}
		return
	}
	p.ufo.x += p.ufo.vx * s
	if p.ufo.vx > 0 && p.ufo.x > float64(p.w) {
		p.ufo.active = false
		p.ufo.nextSpawnT = ufoSpawnMinDur + p.rng.Float64()*(ufoSpawnMaxDur-ufoSpawnMinDur)
	}
	if p.ufo.vx < 0 && p.ufo.x+float64(ufoSprite.width()) < 0 {
		p.ufo.active = false
		p.ufo.nextSpawnT = ufoSpawnMinDur + p.rng.Float64()*(ufoSpawnMaxDur-ufoSpawnMinDur)
	}
}

func (p *playScene) spawnUFO() {
	p.ufo.active = true
	p.ufo.y = p.hudRows*2 + 1
	// Random direction.
	if p.rng.Intn(2) == 0 {
		p.ufo.x = -float64(ufoSprite.width())
		p.ufo.vx = ufoSpeed
	} else {
		p.ufo.x = float64(p.w)
		p.ufo.vx = -ufoSpeed
	}
	// Mystery score: 50, 100, 150, 200, or 300.
	tab := []int{50, 100, 150, 200, 300}
	p.ufo.score = tab[p.rng.Intn(len(tab))]
}

func (p *playScene) alienFire() {
	if len(p.alienBullets) >= maxAlienBullets {
		return
	}
	// Build the list of columns that still have a live alien.
	var liveCols []int
	for c := 0; c < p.grid.cols; c++ {
		for r := 0; r < p.grid.rows; r++ {
			if p.grid.cells[r][c].alive {
				liveCols = append(liveCols, c)
				break
			}
		}
	}
	if len(liveCols) == 0 {
		return
	}
	col := liveCols[p.rng.Intn(len(liveCols))]
	// Find the bottommost live alien in that column — that's the one
	// that fires.
	row := -1
	for r := p.grid.rows - 1; r >= 0; r-- {
		if p.grid.cells[r][col].alive {
			row = r
			break
		}
	}
	if row < 0 {
		return
	}
	ax, ay := p.grid.alienPos(row, col)
	b := &bullet{
		x:          float64(ax + p.grid.spriteW/2),
		y:          float64(ay + p.grid.spriteH),
		vy:         alienBulletSpeed,
		fromPlayer: false,
		kind:       p.rng.Intn(3),
	}
	p.alienBullets = append(p.alienBullets, b)
}

// alienReachedPlayer returns true when any live alien's sprite has
// descended far enough that it touches the cannon's row.
func (p *playScene) alienReachedPlayer() bool {
	for r := p.grid.rows - 1; r >= 0; r-- {
		for c := 0; c < p.grid.cols; c++ {
			if !p.grid.cells[r][c].alive {
				continue
			}
			_, ay := p.grid.alienPos(r, c)
			if ay+p.grid.spriteH >= p.loseY {
				return true
			}
		}
	}
	return false
}

// --- Collision ---------------------------------------------------------

// rect is a simple AABB in canvas pixel coordinates (x0, y0, x1, y1).
type rect struct {
	x0, y0, x1, y1 int
}

func (r rect) overlaps(other rect) bool {
	return r.x0 < other.x1 && r.x1 > other.x0 &&
		r.y0 < other.y1 && r.y1 > other.y0
}

func (p *playScene) resolveCollisions() {
	p.collidePlayerBullet()
	p.collideAlienBullets()
	p.collideBulletsVsBullets()
}

// collidePlayerBullet checks the (at most one) player bullet against
// aliens, the UFO, and bunkers, in that order.
func (p *playScene) collidePlayerBullet() {
	b := p.player.bullet
	if b == nil {
		return
	}
	br := rect{
		x0: int(b.x),
		y0: int(b.y),
		x1: int(b.x) + playerBulletSprite.width(),
		y1: int(b.y) + playerBulletSprite.height(),
	}

	// Aliens.
	for r := 0; r < p.grid.rows; r++ {
		for c := 0; c < p.grid.cols; c++ {
			if !p.grid.cells[r][c].alive {
				continue
			}
			ax, ay := p.grid.alienPos(r, c)
			ar := rect{x0: ax, y0: ay, x1: ax + p.grid.spriteW, y1: ay + p.grid.spriteH}
			if br.overlaps(ar) {
				p.grid.cells[r][c].alive = false
				p.grid.alive--
				p.score += p.grid.cells[r][c].kind.score()
				p.player.bullet = nil
				return
			}
		}
	}

	// UFO.
	if p.ufo.active {
		ur := rect{
			x0: int(p.ufo.x),
			y0: p.ufo.y,
			x1: int(p.ufo.x) + ufoSprite.width(),
			y1: p.ufo.y + ufoSprite.height(),
		}
		if br.overlaps(ur) {
			p.score += p.ufo.score
			p.ufo.active = false
			p.ufo.nextSpawnT = ufoSpawnMinDur + p.rng.Float64()*(ufoSpawnMaxDur-ufoSpawnMinDur)
			p.player.bullet = nil
			return
		}
	}

	// Bunkers — check pixel-perfect at the bullet tip.
	for _, bk := range p.bunkers {
		hitY := int(b.y) // top of player bullet
		hitX := int(b.x)
		if bk.solidAt(hitX, hitY) {
			bk.erode(hitX, hitY, false)
			p.player.bullet = nil
			return
		}
	}
}

func (p *playScene) collideAlienBullets() {
	kept := p.alienBullets[:0]
	for _, b := range p.alienBullets {
		br := rect{
			x0: int(b.x),
			y0: int(b.y),
			x1: int(b.x) + alienBulletStraightA.width(),
			y1: int(b.y) + alienBulletStraightA.height(),
		}

		// Bunkers — check from below.
		hitBunker := false
		for _, bk := range p.bunkers {
			tipY := int(b.y) + alienBulletStraightA.height() - 1
			tipX := int(b.x)
			if bk.solidAt(tipX, tipY) {
				bk.erode(tipX, tipY, true)
				hitBunker = true
				break
			}
		}
		if hitBunker {
			continue // bullet consumed
		}

		// Player.
		if p.player.explodeT <= 0 {
			pr := rect{
				x0: int(p.player.x),
				y0: p.player.y,
				x1: int(p.player.x) + playerSprite.width(),
				y1: p.player.y + playerSprite.height(),
			}
			if br.overlaps(pr) {
				p.player.lives--
				p.player.explodeT = playerExplodeDur
				p.state = psPlayerHit
				p.stateT = 0
				continue // bullet consumed
			}
		}

		kept = append(kept, b)
	}
	p.alienBullets = kept
}

// collideBulletsVsBullets — the classic detail that a player shot
// passing through an alien shot mutually destroys both. We use a
// generous AABB to make this satisfying rather than frame-perfect.
func (p *playScene) collideBulletsVsBullets() {
	pb := p.player.bullet
	if pb == nil {
		return
	}
	pbr := rect{
		x0: int(pb.x) - 1,
		y0: int(pb.y) - 1,
		x1: int(pb.x) + playerBulletSprite.width() + 1,
		y1: int(pb.y) + playerBulletSprite.height() + 1,
	}
	kept := p.alienBullets[:0]
	hit := false
	for _, ab := range p.alienBullets {
		if hit {
			kept = append(kept, ab)
			continue
		}
		abr := rect{
			x0: int(ab.x),
			y0: int(ab.y),
			x1: int(ab.x) + alienBulletStraightA.width(),
			y1: int(ab.y) + alienBulletStraightA.height(),
		}
		if pbr.overlaps(abr) {
			hit = true
			continue
		}
		kept = append(kept, ab)
	}
	if hit {
		p.player.bullet = nil
	}
	p.alienBullets = kept
}

// --- Rendering ---------------------------------------------------------

// Draw paints the entire play scene. Layered: HUD on top, aliens and
// bunkers and player in the middle, bullets and UFO on top of those.
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Color{R: 4, G: 4, B: 12, A: 255})

	p.drawHUD(c)
	p.drawAliens(c)
	p.drawBunkers(c)
	p.drawUFO(c)
	p.drawPlayer(c)
	p.drawBullets(c)
	p.drawGroundLine(c)

	switch p.state {
	case psWaveCleared:
		p.drawCentreBanner(c, fmt.Sprintf("WAVE %d CLEARED", p.wave), engine.Yellow)
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %05d", p.score)
	hiText := fmt.Sprintf("HI %05d", p.hiScore)
	waveText := fmt.Sprintf("WAVE %d", p.wave)
	livesText := fmt.Sprintf("LIVES %d", p.player.lives)

	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	rightCol := cols - len(waveText) - 1
	if rightCol < mid+len(hiText)+2 {
		rightCol = mid + len(hiText) + 2
	}
	c.Print(rightCol, 0, waveText, engine.Cyan)

	c.Print(1, 1, livesText, engine.Color{R: 120, G: 240, B: 140, A: 255})
}

func (p *playScene) drawAliens(c *engine.Canvas) {
	for r := 0; r < p.grid.rows; r++ {
		for col := 0; col < p.grid.cols; col++ {
			if !p.grid.cells[r][col].alive {
				continue
			}
			x, y := p.grid.alienPos(r, col)
			kind := p.grid.cells[r][col].kind
			a, b := kind.frames()
			frame := a
			if p.grid.frame == 1 {
				frame = b
			}
			// Centre the squid sprite (which is narrower) inside its slot.
			off := (p.grid.spriteW - frame.width()) / 2
			drawSprite(c, x+off, y, frame, kind.color())
		}
	}
}

func (p *playScene) drawBunkers(c *engine.Canvas) {
	for _, bk := range p.bunkers {
		for ly := 0; ly < bk.h; ly++ {
			for lx := 0; lx < bk.w; lx++ {
				if bk.mask[ly][lx] {
					c.Set(bk.x+lx, bk.y+ly, bk.color)
				}
			}
		}
	}
}

func (p *playScene) drawUFO(c *engine.Canvas) {
	if !p.ufo.active {
		return
	}
	drawSprite(c, int(p.ufo.x), p.ufo.y, ufoSprite, engine.Color{R: 240, G: 80, B: 90, A: 255})
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.player.lives <= 0 && p.state == psGameOver {
		return
	}
	if p.player.explodeT > 0 {
		// Cycle between the two explosion frames a few times.
		t := playerExplodeDur - p.player.explodeT
		frame := playerExplodeA
		if int(t*10)%2 == 1 {
			frame = playerExplodeB
		}
		drawSprite(c, int(p.player.x), p.player.y, frame,
			engine.Color{R: 240, G: 200, B: 80, A: 255})
		return
	}
	drawSprite(c, int(p.player.x), p.player.y, playerSprite,
		engine.Color{R: 100, G: 240, B: 140, A: 255})
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	if b := p.player.bullet; b != nil {
		drawSprite(c, int(b.x), int(b.y), playerBulletSprite,
			engine.Color{R: 220, G: 240, B: 255, A: 255})
	}
	for _, b := range p.alienBullets {
		var sp sprite
		switch b.kind {
		case 0:
			if b.frame == 0 {
				sp = alienBulletStraightA
			} else {
				sp = alienBulletStraightB
			}
		case 1:
			if b.frame == 0 {
				sp = alienBulletZigA
			} else {
				sp = alienBulletZigB
			}
		default:
			if b.frame == 0 {
				sp = alienBulletForkA
			} else {
				sp = alienBulletForkB
			}
		}
		drawSprite(c, int(b.x)-sp.width()/2, int(b.y), sp,
			engine.Color{R: 250, G: 240, B: 120, A: 255})
	}
}

// drawGroundLine paints the floor the cannon defends — a thin coloured
// strip across the very bottom of the canvas.
func (p *playScene) drawGroundLine(c *engine.Canvas) {
	y := p.h - 1
	c.FillRect(0, y, p.w, 1, engine.Color{R: 90, G: 200, B: 100, A: 255})
}

// drawCentreBanner overlays a centred title using the chunky pixel font.
func (p *playScene) drawCentreBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	// Black background bar for readability.
	c.FillRect(x-3, y-2, w+6, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4, engine.Color{R: 8, G: 8, B: 16, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 80, B: 80, A: 255})

	hint := "ENTER PLAY AGAIN   ESC QUIT"
	hw := len(hint)
	c.Print((c.Cols()-hw)/2, c.Rows()/2+2, hint, engine.White)
}
