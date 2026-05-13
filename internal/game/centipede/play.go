package centipede

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// --- Tuning constants ----------------------------------------------------

const (
	// HUD reserved at the top of the canvas (pixels). One terminal cell
	// row = 2 pixels; we use the top three for the score line.
	hudHeightPx = 3

	// Player movement speed in pixels per second.
	playerSpeed = 36.0

	// Bullet speed and firing cool-down.
	bulletSpeed   = 120.0
	playerFireGap = 0.08

	// Lives.
	startLives = 3

	// Player death animation duration.
	playerExplodeDur = 0.9
	playerRespawnDur = 0.6

	// Centipede speed (cell-steps per second). The arcade ramps with
	// each level; we cap it so the lategame is still playable.
	centipedeStepBase = 0.20 // seconds per cell-step at level 1
	centipedeStepMin  = 0.06

	// Initial centipede length and how it scales with level. The
	// arcade starts at 12 segments and adds detached "head" intruders
	// at later levels — we approximate by spawning extra short chains.
	centipedeStartLen = 12

	// Spider scheduling.
	spiderFirstDelay = 4.0
	spiderInterval   = 11.0
	spiderJitter     = 5.0

	// Flea scheduling. The flea only spawns once the bottom-band
	// mushroom count drops below fleaMushroomTriggerCount, and even
	// then waits for a brief cool-down so it doesn't spam.
	fleaCheckInterval = 1.5
	fleaCoolDown      = 8.0

	// Scorpion scheduling — appears starting at level 2.
	scorpionFirstLevel    = 2
	scorpionFirstDelay    = 8.0
	scorpionInterval      = 18.0
	scorpionJitter        = 7.0

	// Wave end / level-up banner duration.
	bannerDur = 1.6
)

// rect is a simple AABB in canvas pixel coordinates.
type rect struct{ x0, y0, x1, y1 int }

func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && r.x1 > o.x0 && r.y0 < o.y1 && r.y1 > o.y0
}

// --- Play state machine --------------------------------------------------

type playState int

const (
	psPlaying playState = iota
	psPlayerHit
	psWaveCleared
	psGameOver
)

// bullet is a single player projectile flying straight up.
type bullet struct {
	x, y float64
}

// player is the bug-blaster at the bottom.
type player struct {
	x, y     float64 // sprite top-left pixel
	cooldown float64
	lives    int
	explodeT float64
	respawnT float64
}

// playScene owns all game state during a run.
type playScene struct {
	e *engine.Engine
	w int
	h int

	field *field

	pl       player
	bullets  []*bullet
	chains   []*centipedeChain
	spiders  []*spider
	fleas    []*flea
	scorpions []*scorpion
	booms    []*explosion

	rng *rand.Rand

	score   int
	hiScore int
	level   int

	state  playState
	stateT float64

	// Scheduling timers.
	spiderTimer   float64
	fleaTimer     float64
	fleaCoolT     float64
	scorpionTimer float64

	// Cached player-zone pixel bounds.
	playerZoneTopPx int
	playerZoneBotPx int

	wantQuit bool
}

func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	p := &playScene{
		e:       e,
		w:       c.Width(),
		h:       c.Height(),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		hiScore: hiScore,
		level:   1,
	}
	p.pl.lives = startLives
	p.field = newField(p.w, p.h, hudHeightPx, p.rng)
	p.computeLayout()
	p.beginWave(1, true)
	return p
}

// computeLayout caches pixel bounds used every frame.
// playerZoneTopPx is the first y pixel of the player band (inclusive).
// playerZoneBotPx is the y pixel just past the last row (exclusive),
// so it doubles as a "max y bound" for clamping the player sprite.
func (p *playScene) computeLayout() {
	yTop, _ := p.field.cellPixel(0, p.field.playerZoneTop)
	xLast, yLast := p.field.cellPixel(0, p.field.rows-1)
	_ = xLast
	p.playerZoneTopPx = yTop
	p.playerZoneBotPx = yLast + cellH
}

// beginWave resets centipede + transient enemies for a new wave. When
// `fresh` is true (start of game), we also seed a brand-new mushroom
// field; otherwise existing mushrooms regrow back to full health.
func (p *playScene) beginWave(level int, fresh bool) {
	p.level = level
	if !fresh {
		p.field.regrow(p.rng)
	}
	p.bullets = nil
	p.spiders = nil
	p.fleas = nil
	p.scorpions = nil
	p.booms = nil
	p.chains = nil

	// Main centipede.
	stepInterval := centipedeStepBase * math.Pow(0.88, float64(level-1))
	if stepInterval < centipedeStepMin {
		stepInterval = centipedeStepMin
	}
	mainLen := centipedeStartLen
	if mainLen > p.field.cols {
		mainLen = p.field.cols - 1
	}
	// At higher levels the arcade peels off the main centipede into a
	// shorter one plus several single-head intruders that drop in from
	// the side. We mimic that by reducing the main length by (level-1)
	// and spawning that many single-head chains.
	heads := level - 1
	if heads > 6 {
		heads = 6
	}
	mainLen -= heads
	if mainLen < 6 {
		mainLen = 6
	}
	p.chains = append(p.chains, newCentipede(mainLen, p.field.cols, stepInterval))
	for i := 0; i < heads; i++ {
		single := newCentipede(1, p.field.cols, stepInterval)
		// Stagger entry: drop them in from the side a couple of rows
		// below the top so the screen isn't a wall of heads.
		row := 1 + i
		if row >= p.field.playerZoneTop {
			row = p.field.playerZoneTop - 1
		}
		single.segments[0].row = row
		// Alternate entry side.
		if i%2 == 0 {
			single.segments[0].col = 0
			single.segments[0].dx = 1
		} else {
			single.segments[0].col = p.field.cols - 1
			single.segments[0].dx = -1
		}
		p.chains = append(p.chains, single)
	}

	// Centre the player.
	px := float64(p.w-playerSprite.width()) / 2
	py := float64(p.playerZoneBotPx - playerSprite.height())
	p.pl.x = px
	p.pl.y = py
	p.pl.cooldown = 0
	p.pl.explodeT = 0
	p.pl.respawnT = playerRespawnDur

	p.state = psPlaying
	p.stateT = 0

	// Spawn timers.
	p.spiderTimer = spiderFirstDelay
	p.fleaTimer = fleaCheckInterval
	p.fleaCoolT = 0
	p.scorpionTimer = scorpionFirstDelay
}

// --- Update -------------------------------------------------------------

func (p *playScene) Update(dt time.Duration) error {
	p.handleInput()
	if p.wantQuit {
		return nil
	}
	s := dt.Seconds()
	p.stateT += s

	switch p.state {
	case psPlaying:
		p.updatePlaying(s)
	case psPlayerHit:
		p.tickBullets(s)
		p.tickTimers(s)
		p.pl.explodeT -= s
		if p.pl.explodeT <= 0 {
			if p.pl.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.respawnPlayer()
				p.state = psPlaying
				p.stateT = 0
			}
		}
		p.tickBooms(s)
	case psWaveCleared:
		p.tickBooms(s)
		if p.stateT >= bannerDur {
			p.beginWave(p.level+1, false)
		}
	case psGameOver:
		p.tickBooms(s)
	}

	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying:
			p.handlePlayKey(k)
		case psPlayerHit, psWaveCleared:
			if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		case psGameOver:
			if k.Code == engine.KeyEnter ||
				(k.Code == engine.KeyChar && (k.Rune == 'r' || k.Rune == 'R')) {
				hi := p.hiScore
				p.score = 0
				p.pl.lives = startLives
				p.hiScore = hi
				p.field = newField(p.w, p.h, hudHeightPx, p.rng)
				p.computeLayout()
				p.beginWave(1, true)
			} else if k.Code == engine.KeyEsc ||
				(k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q')) {
				p.wantQuit = true
			}
		}
	}
}

func (p *playScene) handlePlayKey(k engine.Key) {
	switch k.Code {
	case engine.KeyEsc:
		p.wantQuit = true
	case engine.KeyChar:
		switch k.Rune {
		case ' ':
			p.tryFire()
		case 'q', 'Q':
			p.wantQuit = true
		}
	}
}

func (p *playScene) tryFire() {
	if p.pl.explodeT > 0 {
		return
	}
	if p.pl.cooldown > 0 {
		return
	}
	// Classic Centipede limit: only one bullet on screen at a time.
	if len(p.bullets) >= 1 {
		return
	}
	bx := p.pl.x + float64(playerSprite.width())/2 - 0.5
	by := p.pl.y - float64(bulletSprite.height())
	p.bullets = append(p.bullets, &bullet{x: bx, y: by})
	p.pl.cooldown = playerFireGap
}

// updatePlaying drives one tick of in-game logic.
func (p *playScene) updatePlaying(s float64) {
	p.tickPlayerInput(s)
	p.tickBullets(s)
	p.tickChains(s)
	p.tickSpiders(s)
	p.tickFleas(s)
	p.tickScorpions(s)
	p.tickBooms(s)
	p.tickTimers(s)

	p.resolveBulletMushroom()
	p.resolveBulletCentipede()
	p.resolveBulletSpider()
	p.resolveBulletFlea()
	p.resolveBulletScorpion()

	p.resolvePlayerCentipede()
	p.resolvePlayerSpider()
	p.resolvePlayerFlea()
	p.resolvePlayerScorpion()

	if p.state == psPlaying && p.allChainsDead() {
		p.state = psWaveCleared
		p.stateT = 0
	}
}

func (p *playScene) tickPlayerInput(s float64) {
	if p.pl.explodeT > 0 {
		return
	}
	if p.pl.respawnT > 0 {
		p.pl.respawnT -= s
	}
	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')
	var dx, dy float64
	if left && !right {
		dx = -1
	} else if right && !left {
		dx = 1
	}
	if up && !down {
		dy = -1
	} else if down && !up {
		dy = 1
	}
	if dx != 0 && dy != 0 {
		inv := 1.0 / math.Sqrt2
		dx *= inv
		dy *= inv
	}

	// Move per axis with separate collision checks so the player can
	// slide along a mushroom wall instead of getting stuck.
	minX, maxX := 0.0, float64(p.w-playerSprite.width())
	minY := float64(p.playerZoneTopPx)
	maxY := float64(p.playerZoneBotPx - playerSprite.height())

	if dx != 0 {
		oldX := p.pl.x
		p.pl.x += dx * playerSpeed * s
		if p.pl.x < minX {
			p.pl.x = minX
		}
		if p.pl.x > maxX {
			p.pl.x = maxX
		}
		if p.playerCollidesMushroom() {
			p.pl.x = oldX
		}
	}
	if dy != 0 {
		oldY := p.pl.y
		p.pl.y += dy * playerSpeed * s
		if p.pl.y < minY {
			p.pl.y = minY
		}
		if p.pl.y > maxY {
			p.pl.y = maxY
		}
		if p.playerCollidesMushroom() {
			p.pl.y = oldY
		}
	}

	if p.pl.cooldown > 0 {
		p.pl.cooldown -= s
	}
}

// playerCollidesMushroom returns true if the player's current bounding
// box overlaps any mushroom cell.
func (p *playScene) playerCollidesMushroom() bool {
	pr := p.playerRect()
	for _, cell := range p.cellsOverlapping(pr) {
		if p.field.hasMushroom(cell.col, cell.row) {
			return true
		}
	}
	return false
}

func (p *playScene) playerRect() rect {
	return rect{
		x0: int(p.pl.x),
		y0: int(p.pl.y),
		x1: int(p.pl.x) + playerSprite.width(),
		y1: int(p.pl.y) + playerSprite.height(),
	}
}

type cellRC struct{ col, row int }

// cellsOverlapping returns the list of grid cells that the pixel rect r
// intersects.
func (p *playScene) cellsOverlapping(r rect) []cellRC {
	c0, r0 := p.field.cellAtPixel(r.x0, r.y0)
	c1, r1 := p.field.cellAtPixel(r.x1-1, r.y1-1)
	if c0 < 0 {
		c0 = (r.x0 - p.field.originX) / cellW
	}
	if r0 < 0 {
		r0 = (r.y0 - p.field.originY) / cellH
	}
	if c1 < 0 {
		c1 = (r.x1 - 1 - p.field.originX) / cellW
	}
	if r1 < 0 {
		r1 = (r.y1 - 1 - p.field.originY) / cellH
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 < 0 {
		c0 = 0
	}
	if r0 < 0 {
		r0 = 0
	}
	if c1 >= p.field.cols {
		c1 = p.field.cols - 1
	}
	if r1 >= p.field.rows {
		r1 = p.field.rows - 1
	}
	out := make([]cellRC, 0, (c1-c0+1)*(r1-r0+1))
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			out = append(out, cellRC{col: c, row: r})
		}
	}
	return out
}

func (p *playScene) tickBullets(s float64) {
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		b.y -= bulletSpeed * s
		if b.y+float64(bulletSprite.height()) > 0 {
			kept = append(kept, b)
		}
	}
	p.bullets = kept
}

func (p *playScene) tickChains(s float64) {
	kept := p.chains[:0]
	for _, cp := range p.chains {
		cp.tick(s, p.field)
		if cp.length() > 0 {
			kept = append(kept, cp)
		}
	}
	p.chains = kept
}

func (p *playScene) tickSpiders(s float64) {
	kept := p.spiders[:0]
	for _, sp := range p.spiders {
		if !sp.alive {
			continue
		}
		off := sp.tick(s, p.field, p.rng)
		if !off {
			kept = append(kept, sp)
		}
	}
	p.spiders = kept
}

func (p *playScene) tickFleas(s float64) {
	kept := p.fleas[:0]
	for _, fl := range p.fleas {
		if !fl.alive {
			continue
		}
		gone := fl.tick(s, p.field, p.rng)
		if !gone {
			kept = append(kept, fl)
		}
	}
	p.fleas = kept
}

func (p *playScene) tickScorpions(s float64) {
	kept := p.scorpions[:0]
	for _, sc := range p.scorpions {
		if !sc.alive {
			continue
		}
		off := sc.tick(s, p.field)
		if !off {
			kept = append(kept, sc)
		}
	}
	p.scorpions = kept
}

func (p *playScene) tickBooms(s float64) {
	kept := p.booms[:0]
	for _, b := range p.booms {
		if b.tick(s) {
			continue
		}
		kept = append(kept, b)
	}
	p.booms = kept
}

func (p *playScene) tickTimers(s float64) {
	// Spider.
	if len(p.spiders) == 0 {
		p.spiderTimer -= s
		if p.spiderTimer <= 0 {
			p.spiders = append(p.spiders, spawnSpider(p.rng, p.field, p.level))
			p.spiderTimer = spiderInterval + p.rng.Float64()*spiderJitter
		}
	}

	// Flea.
	if p.fleaCoolT > 0 {
		p.fleaCoolT -= s
	}
	p.fleaTimer -= s
	if p.fleaTimer <= 0 && p.fleaCoolT <= 0 && len(p.fleas) == 0 {
		p.fleaTimer = fleaCheckInterval
		if p.field.lowerCount() < fleaMushroomTriggerCount {
			p.fleas = append(p.fleas, spawnFlea(p.rng, p.field))
			p.fleaCoolT = fleaCoolDown
		}
	}

	// Scorpion.
	if p.level >= scorpionFirstLevel && len(p.scorpions) == 0 {
		p.scorpionTimer -= s
		if p.scorpionTimer <= 0 {
			p.scorpions = append(p.scorpions, spawnScorpion(p.rng, p.field, p.level))
			p.scorpionTimer = scorpionInterval + p.rng.Float64()*scorpionJitter
		}
	}
}

func (p *playScene) allChainsDead() bool {
	for _, cp := range p.chains {
		if cp.length() > 0 {
			return false
		}
	}
	return true
}

// --- Collisions ---------------------------------------------------------

func (p *playScene) resolveBulletMushroom() {
	if len(p.bullets) == 0 {
		return
	}
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		br := bulletRect(b)
		hit := false
		// Sample the bullet's tip cell and the cell directly below to
		// catch fast-moving bullets that skip a row.
		for _, cell := range p.cellsOverlapping(br) {
			if p.field.hasMushroom(cell.col, cell.row) {
				score, _ := p.field.damage(cell.col, cell.row)
				p.score += score
				hit = true
				break
			}
		}
		if hit {
			continue
		}
		kept = append(kept, b)
	}
	p.bullets = kept
}

func (p *playScene) resolveBulletCentipede() {
	if len(p.bullets) == 0 {
		return
	}
	kept := p.bullets[:0]
	bulletDead := map[*bullet]bool{}
	for _, b := range p.bullets {
		br := bulletRect(b)
		hit := false
		for ci := 0; ci < len(p.chains); ci++ {
			cp := p.chains[ci]
			for i, seg := range cp.segments {
				if segmentRect(p.field, seg).overlaps(br) {
					split, wasHead, empty := cp.applyHitAt(i)
					if wasHead {
						p.score += 100
					} else {
						p.score += 10
					}
					// Plant a mushroom where the segment died (unless
					// a mushroom is already there, which can't happen
					// since the segment was occupying the cell).
					p.field.plant(seg.col, seg.row)
					// Brief explosion visual.
					sx, sy := p.field.cellPixel(seg.col, seg.row)
					p.booms = append(p.booms, &explosion{x: float64(sx), y: float64(sy)})
					if split != nil {
						p.chains = append(p.chains, split)
					}
					_ = empty
					bulletDead[b] = true
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if hit {
			continue
		}
		kept = append(kept, b)
	}
	// Drop empty chains.
	keepChains := p.chains[:0]
	for _, cp := range p.chains {
		if cp.length() > 0 {
			keepChains = append(keepChains, cp)
		}
	}
	p.chains = keepChains
	// Filter kept bullets through bulletDead too (defensive).
	final := kept[:0]
	for _, b := range kept {
		if !bulletDead[b] {
			final = append(final, b)
		}
	}
	p.bullets = final
}

func (p *playScene) resolveBulletSpider() {
	if len(p.bullets) == 0 || len(p.spiders) == 0 {
		return
	}
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		br := bulletRect(b)
		hit := false
		for _, sp := range p.spiders {
			if !sp.alive {
				continue
			}
			if sp.rect().overlaps(br) {
				sp.alive = false
				// Distance from spider center to player center.
				scx := float64(sp.rect().x0+sp.rect().x1) / 2
				scy := float64(sp.rect().y0+sp.rect().y1) / 2
				pcx := p.pl.x + float64(playerSprite.width())/2
				pcy := p.pl.y + float64(playerSprite.height())/2
				dx := scx - pcx
				dy := scy - pcy
				dist := math.Sqrt(dx*dx + dy*dy)
				p.score += spiderScore(dist)
				p.booms = append(p.booms, &explosion{x: float64(sp.rect().x0), y: float64(sp.rect().y0)})
				hit = true
				break
			}
		}
		if hit {
			continue
		}
		kept = append(kept, b)
	}
	p.bullets = kept
	// Drop dead spiders.
	keepSp := p.spiders[:0]
	for _, sp := range p.spiders {
		if sp.alive {
			keepSp = append(keepSp, sp)
		}
	}
	p.spiders = keepSp
}

func (p *playScene) resolveBulletFlea() {
	if len(p.bullets) == 0 || len(p.fleas) == 0 {
		return
	}
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		br := bulletRect(b)
		hit := false
		for _, fl := range p.fleas {
			if !fl.alive {
				continue
			}
			if fl.rect().overlaps(br) {
				dead, sc := fl.hit()
				p.score += sc
				if dead {
					p.booms = append(p.booms, &explosion{x: float64(fl.rect().x0), y: float64(fl.rect().y0)})
				}
				hit = true
				break
			}
		}
		if hit {
			continue
		}
		kept = append(kept, b)
	}
	p.bullets = kept
	keepFl := p.fleas[:0]
	for _, fl := range p.fleas {
		if fl.alive {
			keepFl = append(keepFl, fl)
		}
	}
	p.fleas = keepFl
}

func (p *playScene) resolveBulletScorpion() {
	if len(p.bullets) == 0 || len(p.scorpions) == 0 {
		return
	}
	kept := p.bullets[:0]
	for _, b := range p.bullets {
		br := bulletRect(b)
		hit := false
		for _, sc := range p.scorpions {
			if !sc.alive {
				continue
			}
			if sc.rect().overlaps(br) {
				sc.alive = false
				p.score += 1000
				p.booms = append(p.booms, &explosion{x: float64(sc.rect().x0), y: float64(sc.rect().y0)})
				hit = true
				break
			}
		}
		if hit {
			continue
		}
		kept = append(kept, b)
	}
	p.bullets = kept
	keepSc := p.scorpions[:0]
	for _, sc := range p.scorpions {
		if sc.alive {
			keepSc = append(keepSc, sc)
		}
	}
	p.scorpions = keepSc
}

func (p *playScene) resolvePlayerCentipede() {
	if p.pl.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, cp := range p.chains {
		for _, seg := range cp.segments {
			if segmentRect(p.field, seg).overlaps(pr) {
				p.killPlayer()
				return
			}
		}
	}
}

func (p *playScene) resolvePlayerSpider() {
	if p.pl.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, sp := range p.spiders {
		if !sp.alive {
			continue
		}
		if sp.rect().overlaps(pr) {
			p.killPlayer()
			return
		}
	}
}

func (p *playScene) resolvePlayerFlea() {
	if p.pl.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, fl := range p.fleas {
		if !fl.alive {
			continue
		}
		if fl.rect().overlaps(pr) {
			p.killPlayer()
			return
		}
	}
}

func (p *playScene) resolvePlayerScorpion() {
	if p.pl.explodeT > 0 {
		return
	}
	pr := p.playerRect()
	for _, sc := range p.scorpions {
		if !sc.alive {
			continue
		}
		if sc.rect().overlaps(pr) {
			p.killPlayer()
			return
		}
	}
}

func (p *playScene) killPlayer() {
	p.pl.lives--
	p.pl.explodeT = playerExplodeDur
	p.state = psPlayerHit
	p.stateT = 0
	// Drop a kill explosion.
	p.booms = append(p.booms, &explosion{x: p.pl.x, y: p.pl.y})
}

func (p *playScene) respawnPlayer() {
	// On respawn, the field regenerates damaged mushrooms in the
	// player area — a small mercy that mirrors the arcade's
	// regrowth-on-death rule.
	for r := p.field.playerZoneTop; r < p.field.rows; r++ {
		for c := 0; c < p.field.cols; c++ {
			if p.field.cells[r][c].hp > 0 {
				p.field.cells[r][c].hp = mushroomHP
				p.field.cells[r][c].poisoned = false
			}
		}
	}
	p.pl.explodeT = 0
	p.pl.respawnT = playerRespawnDur
	px := float64(p.w-playerSprite.width()) / 2
	py := float64(p.playerZoneBotPx - playerSprite.height())
	p.pl.x = px
	p.pl.y = py
}

func bulletRect(b *bullet) rect {
	return rect{
		x0: int(b.x),
		y0: int(b.y),
		x1: int(b.x) + bulletSprite.width(),
		y1: int(b.y) + bulletSprite.height(),
	}
}

// --- Draw ---------------------------------------------------------------

func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(colorBackground)
	// Subtle tint behind the player zone so it's visually distinct.
	c.FillRect(0, p.playerZoneTopPx, p.w, p.playerZoneBotPx-p.playerZoneTopPx,
		colorPlayerZoneTint)
	// Thin divider line above the player zone.
	c.FillRect(0, p.playerZoneTopPx-1, p.w, 1,
		engine.Color{R: 40, G: 40, B: 80, A: 255})

	p.field.draw(c)
	for _, cp := range p.chains {
		cp.draw(c, p.field)
	}
	for _, sp := range p.spiders {
		sp.draw(c)
	}
	for _, fl := range p.fleas {
		fl.draw(c)
	}
	for _, sc := range p.scorpions {
		sc.draw(c)
	}
	p.drawBullets(c)
	p.drawPlayer(c)
	for _, b := range p.booms {
		b.draw(c)
	}
	p.drawHUD(c)

	switch p.state {
	case psWaveCleared:
		p.drawBanner(c, fmt.Sprintf("WAVE %d CLEARED", p.level),
			engine.Color{R: 240, G: 240, B: 120, A: 255})
	case psGameOver:
		p.drawGameOver(c)
	}
}

func (p *playScene) drawHUD(c *engine.Canvas) {
	cols := c.Cols()
	scoreText := fmt.Sprintf("SCORE %06d", p.score)
	hiText := fmt.Sprintf("HI %06d", p.hiScore)
	levelText := fmt.Sprintf("WAVE %d", p.level)
	c.Print(1, 0, scoreText, engine.White)
	mid := (cols - len(hiText)) / 2
	if mid < len(scoreText)+2 {
		mid = len(scoreText) + 2
	}
	c.Print(mid, 0, hiText, engine.Yellow)
	rightCol := cols - len(levelText) - 1
	if rightCol < mid+len(hiText)+2 {
		rightCol = mid + len(hiText) + 2
	}
	c.Print(rightCol, 0, levelText, engine.Cyan)

	// Lives indicator: small player icons across the very top of the
	// player zone tint, drawn from the left.
	reserve := p.pl.lives
	if p.state == psPlayerHit || p.state == psGameOver {
		// During the death sequence the just-lost life is already
		// decremented; the still-spawnable count is `reserve`.
	}
	if reserve > 0 {
		for i := 0; i < reserve; i++ {
			x := 1 + i*(playerSprite.width()+1)
			drawSprite(c, x, p.playerZoneTopPx-cellH-1, playerSprite, colorPlayer)
		}
	}
}

func (p *playScene) drawBullets(c *engine.Canvas) {
	for _, b := range p.bullets {
		drawSprite(c, int(b.x), int(b.y), bulletSprite, colorBullet)
	}
}

func (p *playScene) drawPlayer(c *engine.Canvas) {
	if p.state == psGameOver {
		return
	}
	if p.pl.explodeT > 0 {
		spr := playerExplodeA
		if int((playerExplodeDur-p.pl.explodeT)*10)%2 == 1 {
			spr = playerExplodeB
		}
		drawSprite(c, int(p.pl.x), int(p.pl.y), spr, colorExplosion)
		return
	}
	// Blink during respawn invincibility.
	if p.pl.respawnT > 0 && int(p.pl.respawnT*10)%2 == 0 {
		return
	}
	drawSprite(c, int(p.pl.x), int(p.pl.y), playerSprite, colorPlayer)
}

func (p *playScene) drawBanner(c *engine.Canvas, text string, col engine.Color) {
	w := engine.TextWidth(text)
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, text, col)
}

func (p *playScene) drawGameOver(c *engine.Canvas) {
	w := engine.TextWidth("GAME OVER")
	x := (p.w - w) / 2
	y := (p.h - engine.FontHeight) / 2 - 4
	c.FillRect(x-4, y-2, w+8, engine.FontHeight+4,
		engine.Color{R: 8, G: 8, B: 24, A: 255})
	c.DrawText(x, y, "GAME OVER", engine.Color{R: 255, G: 90, B: 90, A: 255})
	hint := "ENTER PLAY AGAIN   ESC QUIT"
	c.Print((c.Cols()-len(hint))/2, c.Rows()/2+2, hint, engine.White)
}
