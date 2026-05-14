package defender

import (
	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// player is the lone defender's ship. World-x lives in [0, worldW)
// (wrapped each tick); y is bounded to [playZoneTop, playZoneBot).
//
// Movement model:
//   - Horizontal: Defender's classic feel. Left/Right keys both
//     thrust AND set facing in that direction. Released keys drop the
//     ship to coast; momentum decays slowly via drag so the ship feels
//     "floaty" but predictable.
//   - Vertical: Up/Down apply direct acceleration with comparatively
//     more drag — vertical taps don't carry the ship a long way.
type player struct {
	worldX  float64
	y       float64
	vx, vy  float64
	facing  int // -1 (left) or +1 (right)
	cooldown float64

	lives       int
	smartBombs  int
	score       int // mirrored from playScene for bonus-life calculations
	prevScore   int // last threshold we awarded a bonus at

	thrusting bool // true if any left/right key was held this frame (for flame anim)
	thrustT   float64

	// Death/respawn state machine.
	dead     bool
	deadT    float64

	// Hyperspace
	hyperT float64 // > 0 while teleport flicker plays

	// Smart-bomb shockwave.
	bombT  float64 // > 0 while shockwave plays
	bombX  float64 // world x at which the bomb was triggered
	bombY  float64

	// Held humanoid (rescued mid-fall).
	carrying *humanoid
}

// Tuning. Tweaked by ear — Defender's exact feel is intimate; this
// targets "responsive but momentum-y".
const (
	thrustAccel        = 110.0
	verticalAccel      = 90.0
	maxHSpeed          = 80.0
	maxVSpeed          = 55.0
	horizDrag          = 1.2 // per second
	vertDrag           = 4.5 // per second — vertical bleeds momentum faster
	playerFireGap      = 0.10
	maxPlayerShots     = 6
	playerShotSpeed    = 160.0
	playerDeathDur     = 1.4
	playerRespawnInv   = 1.6
	hyperspaceDur      = 0.45
	hyperspaceFatalPct = 0.04 // 4% chance hyperspace kills you
	bonusLifeStep      = 10000
	bombShockwaveDur   = 0.55
	smartBombStart     = 3
)

// initPlayer centres the ship in the world (worldX = 0 by convention)
// and configures starting stocks.
func (p *playScene) initPlayer() {
	p.player.worldX = 0
	p.player.y = float64(p.world.groundY) - 16
	p.player.facing = 1
	p.player.lives = 3
	p.player.smartBombs = smartBombStart
	p.player.dead = false
	p.player.vx = 0
	p.player.vy = 0
	p.world.camLeft = -float64(p.w) / 3
}

// updatePlayer reads input, integrates physics, and applies death/
// respawn transitions for the ship.
func (p *playScene) updatePlayer(dt float64) {
	pl := &p.player

	// Hyperspace flicker — ship is invulnerable & not drawn during
	// this brief blink.
	if pl.hyperT > 0 {
		pl.hyperT -= dt
		if pl.hyperT <= 0 {
			pl.hyperT = 0
		}
		return
	}

	// Death animation timer.
	if pl.dead {
		pl.deadT += dt
		if pl.deadT >= playerDeathDur {
			if pl.lives <= 0 {
				// Final death — no respawn. Park the death animation
				// at its last frame and hand control to game over.
				pl.deadT = playerDeathDur
				if p.state == psPlaying {
					p.state = psGameOver
					p.stateT = 0
				}
				return
			}
			pl.dead = false
			pl.deadT = 0
			pl.vx = 0
			pl.vy = 0
			pl.y = float64(p.world.groundY) - 16
			// Respawn invincibility: re-use deadT as a count-down for
			// blink rendering. Negative => alive, blinking.
			pl.deadT = -playerRespawnInv
		}
		return
	}

	// Blinking countdown after respawn.
	if pl.deadT < 0 {
		pl.deadT += dt
		if pl.deadT >= 0 {
			pl.deadT = 0
		}
	}

	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	up := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	down := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')

	pl.thrusting = left || right
	if left && !right {
		pl.facing = -1
		pl.vx -= thrustAccel * dt
	}
	if right && !left {
		pl.facing = 1
		pl.vx += thrustAccel * dt
	}
	if up && !down {
		pl.vy -= verticalAccel * dt
	}
	if down && !up {
		pl.vy += verticalAccel * dt
	}
	// Drag.
	pl.vx -= pl.vx * horizDrag * dt
	pl.vy -= pl.vy * vertDrag * dt
	// Clamp speeds.
	if pl.vx > maxHSpeed {
		pl.vx = maxHSpeed
	}
	if pl.vx < -maxHSpeed {
		pl.vx = -maxHSpeed
	}
	if pl.vy > maxVSpeed {
		pl.vy = maxVSpeed
	}
	if pl.vy < -maxVSpeed {
		pl.vy = -maxVSpeed
	}

	pl.worldX = p.world.wrapX(pl.worldX + pl.vx*dt)
	pl.y += pl.vy * dt
	// Vertical clamping — ship can't fly into the HUD or below ground.
	minY := float64(p.world.playZoneTop)
	maxY := float64(p.world.groundY) - float64(playerShip.height())
	if pl.y < minY {
		pl.y = minY
		pl.vy = 0
	}
	if pl.y > maxY {
		pl.y = maxY
		pl.vy = 0
	}

	// Fire cooldown.
	if pl.cooldown > 0 {
		pl.cooldown -= dt
	}

	// Thrust flame phase advances when thrusting.
	if pl.thrusting {
		pl.thrustT += dt
	} else {
		pl.thrustT = 0
	}

	// If carrying a humanoid, keep them pinned below the ship.
	if pl.carrying != nil {
		h := pl.carrying
		h.worldX = pl.worldX + 3
		h.y = pl.y + float64(playerShip.height()) - 1
		// Touched ground? Set them down for the rescue bonus.
		ground := p.world.terrainAt(h.worldX) - float64(humanoidSprite.height())
		if h.y >= ground-1 {
			h.y = ground
			h.state = humanWalking
			h.dirX = pickDir(p.rng)
			h.rescued = true
			h.bonusT = 1.0
			p.score += humanRescueDeliver
			pl.carrying = nil
		}
	}

	// Bonus life every bonusLifeStep points.
	if p.score/bonusLifeStep > pl.prevScore/bonusLifeStep {
		pl.lives++
		pl.smartBombs++
	}
	pl.prevScore = p.score

	// Bomb shockwave ticks down.
	if pl.bombT > 0 {
		pl.bombT -= dt
		if pl.bombT < 0 {
			pl.bombT = 0
		}
	}
}

// firePlayer creates a new player laser bolt aimed in the current
// facing direction. The laser is a thin horizontal sliver moving
// fast — Defender's signature.
func (p *playScene) firePlayer() {
	pl := &p.player
	if pl.dead || pl.hyperT > 0 {
		return
	}
	if pl.cooldown > 0 {
		return
	}
	if len(p.playerBolts) >= maxPlayerShots {
		return
	}
	dir := float64(pl.facing)
	startX := pl.worldX + float64(playerShip.width())/2
	startY := pl.y + float64(playerShip.height())/2
	b := &playerBolt{
		worldX: startX,
		y:      startY,
		vx:     dir * playerShotSpeed,
		life:   0.6,
	}
	p.playerBolts = append(p.playerBolts, b)
	pl.cooldown = playerFireGap
}

// triggerSmartBomb fires the screen-clearing nuke. Defender's smart
// bomb kills every visible enemy at once (mines are not destroyed in
// the original).
func (p *playScene) triggerSmartBomb() {
	pl := &p.player
	if pl.smartBombs <= 0 || pl.dead || pl.hyperT > 0 {
		return
	}
	pl.smartBombs--
	pl.bombT = bombShockwaveDur
	pl.bombX = pl.worldX
	pl.bombY = pl.y
	// Kill every enemy currently on-screen.
	for _, e := range p.enemies {
		if !e.alive() {
			continue
		}
		sx := p.world.toScreen(e.worldX)
		if sx < -10 || sx >= p.w+10 {
			continue
		}
		p.score += kindScore(e.kind)
		if e.kind == kPod {
			// Smart-bombing a pod does NOT spawn swarmers — in original
			// arcade behaviour the bomb destroys the pod cleanly.
		}
		// Free any carried human into freefall.
		if e.carrying != nil {
			e.carrying.state = humanFalling
			e.carrying.carrier = nil
			e.carrying = nil
		}
		e.state = esDying
		e.dyingT = 0
	}
	// Clear all enemy bolts and mines too.
	p.enemyBolts = p.enemyBolts[:0]
	p.mines = p.mines[:0]
}

// triggerHyperspace teleports the player to a random world x and y.
// Has a small fatal chance — the gamble that made Defender's
// hyperspace feel like a last resort.
func (p *playScene) triggerHyperspace() {
	pl := &p.player
	if pl.dead || pl.hyperT > 0 {
		return
	}
	pl.hyperT = hyperspaceDur
	pl.worldX = p.rng.Float64() * float64(p.world.worldW)
	pl.y = float64(p.world.playZoneTop) + p.rng.Float64()*float64(p.world.groundY-p.world.playZoneTop-16)
	pl.vx = 0
	pl.vy = 0
	if p.rng.Float64() < hyperspaceFatalPct {
		// Bad luck — fatal jump.
		p.killPlayer()
	}
}

// killPlayer puts the player into the death state. If they were
// carrying a humanoid, the humanoid is released into freefall.
func (p *playScene) killPlayer() {
	pl := &p.player
	if pl.dead {
		return
	}
	pl.dead = true
	pl.deadT = 0
	pl.lives--
	if pl.carrying != nil {
		pl.carrying.state = humanFalling
		pl.carrying.fallV = 0
		pl.carrying = nil
	}
}

// playerInvulnerable returns true while the player can't be hit.
func (pl *player) invulnerable() bool {
	if pl.dead {
		return true
	}
	if pl.hyperT > 0 {
		return true
	}
	if pl.deadT < 0 {
		return true
	}
	return false
}

// boundingBox returns the player's AABB in world+pixel coords.
func (p *playScene) playerBox() (x0, y0, x1, y1 float64) {
	w := float64(playerShip.width())
	h := float64(playerShip.height())
	return p.player.worldX, p.player.y, p.player.worldX + w, p.player.y + h
}

// rectsOverlapWrap tests AABB overlap on the toroidal world. We test
// the closer of (entity x) and (entity x ± worldW) so collisions wrap
// correctly near the seam.
func (wd *world) rectsOverlap(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1 float64) bool {
	if ay0 >= by1 || ay1 <= by0 {
		return false
	}
	// Determine which copy of b is closer to a in x.
	w := float64(wd.worldW)
	for _, shift := range [3]float64{-w, 0, w} {
		sx0 := bx0 + shift
		sx1 := bx1 + shift
		if ax0 < sx1 && ax1 > sx0 {
			return true
		}
	}
	return false
}

