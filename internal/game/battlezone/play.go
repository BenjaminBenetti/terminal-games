package battlezone

import (
	"math"
	"math/rand"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// Player tuning. Battlezone's tank is famously sluggish — slow turn
// and modest forward speed — which is what makes the combat feel like
// the cat-and-mouse it was. The numbers below are tuned to feel
// similarly heavy on a terminal-sized canvas.
const (
	playerTurnSpeed     = 1.3 // rad/s
	playerForwardSpeed  = 9.0
	playerReverseSpeed  = 4.5
	playerEyeHeight     = 1.5
	playerCollideRadius = 1.4
	playerFireCooldown  = 0.35
	bonusLifeEvery      = 15000
	initialLives        = 3
	gameStartDelay      = 1.0
	nextEnemyDelay      = 1.6
	playerDeathFreeze   = 2.2
)

// playState is the gameplay sub-state machine. The top-level scene
// only distinguishes title vs play; this lives inside the playScene.
type playState int

const (
	psWaiting     playState = iota // delay between enemies
	psPlaying                      // an enemy is alive and the player is fighting
	psPlayerDying                  // player took a hit; field freezes for the death animation
	psGameOver                     // out of lives
)

// playScene is the per-match gameplay scene. It owns the player's
// camera, the obstacle field, the single live enemy, projectiles,
// particles, score, lives, and the HUD's transient state (radar sweep,
// crack overlay).
type playScene struct {
	e   *engine.Engine
	cam camera

	// World state.
	obstacles []*obstacle
	enemy     *enemy

	// Projectiles (player + enemy shells) and decorative particles
	// (sparks from missile trail, explosions).
	projectiles []*projectile
	sparks      []*spark
	explosions  []*explosion

	// Match state.
	state        playState
	stateT       float64
	score        int
	hiScore      int
	lives        int
	nextBonus    int
	fireCooldown float64
	enemiesKilled int

	// HUD transient state.
	radarSweep   float64
	crackT       float64
	crackPattern *crackPattern

	// Decorative scenery (moon, volcano, mountains). These are placed
	// once and rotate with the player's heading like infinitely-distant
	// objects.
	mountainPolyline []vec2D // points on a unit circle: (x = azimuth radians, y = horizon offset)
	volcanoAzimuth   float64
	volcanoErupting  bool
	volcanoT         float64
	moonAzimuth      float64

	rng *rand.Rand

	// wantQuit signals back to the top-level scene that ESC was pressed.
	wantQuit bool
}

// spark is a tiny short-lived particle used for missile exhaust trails
// and dust kicked up by movement.
type spark struct {
	pos  vec3
	vel  vec3
	life float64
	dur  float64
}

// explosion is a brief wireframe burst rendered as expanding line
// segments around a centre point. We don't need true volumetric
// particles — a few outward spokes read perfectly fine in green vector.
type explosion struct {
	pos    vec3
	radius float64
	life   float64
	dur    float64
	rays   []float64 // radians around y axis
}

// newPlayScene constructs a fresh match.
func newPlayScene(e *engine.Engine, hiScore int) *playScene {
	c := e.Canvas()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	cam := camera{
		pos:   vec3{x: 0, y: playerEyeHeight, z: 0},
		yaw:   0,
		focal: float64(c.Width()) * 0.72, // ~70° horizontal FOV
		cx:    c.Width() / 2,
		cy:    c.Height() / 2,
	}
	p := &playScene{
		e:         e,
		cam:       cam,
		hiScore:   hiScore,
		lives:     initialLives,
		nextBonus: bonusLifeEvery,
		state:     psWaiting,
		stateT:    -gameStartDelay,
		rng:       rng,
	}
	p.obstacles = generateObstacles(rng, 24)
	p.placeScenery()
	return p
}

// placeScenery generates the celestial / horizon decoration once per
// match: a jagged mountain skyline, a volcano in a fixed direction
// that erupts now and again, and a moon. All three are rendered using
// the player's yaw only (treated as at infinity).
func (p *playScene) placeScenery() {
	// 36 azimuthal samples around the full 2π circle, jittered in
	// height to read as distant mountains.
	const samples = 72
	p.mountainPolyline = make([]vec2D, samples+1)
	for i := 0; i <= samples; i++ {
		az := float64(i) * (2 * math.Pi / samples)
		// Smoothed pseudo-noise: sum of a few sines so the silhouette
		// reads as ridges rather than random spikes.
		h := 0.35*math.Sin(az*3.0+0.7) +
			0.25*math.Sin(az*5.3+1.7) +
			0.18*math.Sin(az*9.1+2.6) +
			0.10*math.Sin(az*13.0+3.0)
		p.mountainPolyline[i] = vec2D{x: az, y: h}
	}
	p.volcanoAzimuth = p.rng.Float64() * 2 * math.Pi
	p.moonAzimuth = math.Mod(p.volcanoAzimuth+math.Pi*0.6, 2*math.Pi)
}

// Update is the engine.Scene update hook.
func (p *playScene) Update(dt time.Duration) error {
	s := dt.Seconds()
	if s <= 0 {
		return nil
	}
	p.stateT += s
	p.radarSweep += s * 1.8
	if p.radarSweep > 2*math.Pi {
		p.radarSweep -= 2 * math.Pi
	}
	p.volcanoT += s
	// Eruption cycle: a few seconds of activity, then a long quiet
	// stretch, repeating. The player can almost time their drives by
	// it, which is part of the original's atmosphere.
	if p.volcanoErupting {
		if p.volcanoT > 3.0 {
			p.volcanoErupting = false
			p.volcanoT = 0
		}
	} else {
		if p.volcanoT > 8.0+p.rng.Float64()*6 {
			p.volcanoErupting = true
			p.volcanoT = 0
		}
	}

	p.handleInput()
	if p.wantQuit {
		return nil
	}

	switch p.state {
	case psWaiting:
		p.tickPlayer(s)
		p.tickProjectiles(s)
		p.tickSparks(s)
		p.tickExplosions(s)
		if p.stateT >= nextEnemyDelay {
			p.spawnEnemy(pickEnemyKind(p.rng, p.score))
			p.state = psPlaying
			p.stateT = 0
		}
	case psPlaying:
		p.tickPlayer(s)
		if p.tickEnemy(s) {
			// Enemy timed out (saucer flew off, missile expired) with
			// no kill credit — go straight to waiting for the next.
			p.enemy = nil
			p.state = psWaiting
			p.stateT = 0
		}
		p.tickProjectiles(s)
		p.tickSparks(s)
		p.tickExplosions(s)
		p.resolveCollisions()
	case psPlayerDying:
		// Field continues animating but the player is locked.
		p.tickProjectiles(s)
		p.tickSparks(s)
		p.tickExplosions(s)
		if p.crackT > 0 {
			p.crackT -= s
		}
		if p.stateT >= playerDeathFreeze {
			if p.lives <= 0 {
				p.state = psGameOver
				p.stateT = 0
			} else {
				p.respawnPlayer()
			}
		}
	case psGameOver:
		p.tickProjectiles(s)
		p.tickSparks(s)
		p.tickExplosions(s)
		if p.crackT > 0 {
			p.crackT -= s
		}
	}
	if p.score > p.hiScore {
		p.hiScore = p.score
	}
	return nil
}

// handleInput drains the keyboard queue. Discrete actions (fire, quit,
// restart) are processed here; held-movement state is queried in
// tickPlayer via IsKeyDown so multi-key combos work cleanly.
func (p *playScene) handleInput() {
	for {
		k, ok := p.e.PollKey()
		if !ok {
			return
		}
		switch p.state {
		case psPlaying, psWaiting:
			p.handlePlayKey(k)
		case psPlayerDying:
			if isQuitKey(k) {
				p.wantQuit = true
			}
		case psGameOver:
			switch k.Code {
			case engine.KeyEnter:
				p.restartMatch()
			case engine.KeyEsc:
				p.wantQuit = true
			case engine.KeyChar:
				switch k.Rune {
				case 'q', 'Q':
					p.wantQuit = true
				case 'r', 'R', ' ':
					p.restartMatch()
				}
			}
		}
	}
}

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

func isQuitKey(k engine.Key) bool {
	if k.Code == engine.KeyEsc {
		return true
	}
	if k.Code == engine.KeyChar && (k.Rune == 'q' || k.Rune == 'Q') {
		return true
	}
	return false
}

// restartMatch resets a finished game in place — keeping the hi-score
// so the player can chase a target between runs in one session.
func (p *playScene) restartMatch() {
	hi := p.hiScore
	rng := p.rng
	e := p.e
	cam := p.cam
	cam.pos = vec3{x: 0, y: playerEyeHeight, z: 0}
	cam.yaw = 0
	*p = playScene{
		e:         e,
		cam:       cam,
		hiScore:   hi,
		lives:     initialLives,
		nextBonus: bonusLifeEvery,
		state:     psWaiting,
		stateT:    -gameStartDelay,
		rng:       rng,
	}
	p.obstacles = generateObstacles(rng, 24)
	p.placeScenery()
}

// tryFire fires the player's shell. Constrained by cooldown and the
// one-shot-at-a-time rule from the original.
func (p *playScene) tryFire() {
	if p.state != psPlaying && p.state != psWaiting {
		return
	}
	if p.fireCooldown > 0 {
		return
	}
	if p.playerShellInFlight() {
		return
	}
	p.spawnPlayerShell()
	p.fireCooldown = playerFireCooldown
}

// tickPlayer handles held-movement input and applies it to the camera.
// Both treads of the original could move independently — we collapse
// that to forward/back + turn for keyboard practicality.
func (p *playScene) tickPlayer(s float64) {
	if p.fireCooldown > 0 {
		p.fireCooldown -= s
	}
	if p.state == psPlayerDying || p.state == psGameOver {
		return
	}

	left := p.e.IsKeyDown(engine.KeyLeft) || p.e.IsCharDown('a') || p.e.IsCharDown('A')
	right := p.e.IsKeyDown(engine.KeyRight) || p.e.IsCharDown('d') || p.e.IsCharDown('D')
	fwd := p.e.IsKeyDown(engine.KeyUp) || p.e.IsCharDown('w') || p.e.IsCharDown('W')
	back := p.e.IsKeyDown(engine.KeyDown) || p.e.IsCharDown('s') || p.e.IsCharDown('S')

	switch {
	case left && !right:
		p.cam.yaw = normalizeAngle(p.cam.yaw - playerTurnSpeed*s)
	case right && !left:
		p.cam.yaw = normalizeAngle(p.cam.yaw + playerTurnSpeed*s)
	}

	move := 0.0
	switch {
	case fwd && !back:
		move = playerForwardSpeed * s
	case back && !fwd:
		move = -playerReverseSpeed * s
	}
	if move != 0 {
		dx := math.Sin(p.cam.yaw) * move
		dz := math.Cos(p.cam.yaw) * move
		newX := wrapWorld(p.cam.pos.x + dx)
		newZ := wrapWorld(p.cam.pos.z + dz)
		if !obstacleAt(p.obstacles, vec3{x: newX, y: 0, z: newZ}, playerCollideRadius) {
			p.cam.pos.x = newX
			p.cam.pos.z = newZ
		}
	}
}

// tickProjectiles advances shells. Shells collide with obstacles, the
// player, and the live enemy. They're removed on hit or expiry.
func (p *playScene) tickProjectiles(s float64) {
	kept := p.projectiles[:0]
	for _, pr := range p.projectiles {
		pr.life -= s
		if pr.life <= 0 {
			continue
		}
		pr.prev = pr.pos
		pr.pos.x = wrapWorld(pr.pos.x + pr.vel.x*s)
		pr.pos.z = wrapWorld(pr.pos.z + pr.vel.z*s)
		// Swept obstacle collision. Use the world-relative segment so
		// obstacle wrap is handled by nearestCopy inside.
		if _, blocked := segmentBlocked(p.obstacles, pr.prev, pr.pos); blocked {
			p.spawnExplosion(pr.pos, 0.7)
			continue
		}
		kept = append(kept, pr)
	}
	p.projectiles = kept
}

// tickSparks fades small particle effects.
func (p *playScene) tickSparks(s float64) {
	kept := p.sparks[:0]
	for _, sp := range p.sparks {
		sp.life -= s
		if sp.life <= 0 {
			continue
		}
		sp.pos.x = wrapWorld(sp.pos.x + sp.vel.x*s)
		sp.pos.z = wrapWorld(sp.pos.z + sp.vel.z*s)
		sp.pos.y += sp.vel.y * s
		kept = append(kept, sp)
	}
	p.sparks = kept
}

// tickExplosions expands the spokes of each explosion outward over its
// lifetime, then drops finished ones.
func (p *playScene) tickExplosions(s float64) {
	kept := p.explosions[:0]
	for _, ex := range p.explosions {
		ex.life -= s
		if ex.life <= 0 {
			continue
		}
		// Radius grows from 0 to a max over the first third of life,
		// then holds; opacity fades over the last two thirds.
		k := 1 - ex.life/ex.dur
		ex.radius = 2.0 + 4.0*k
		kept = append(kept, ex)
	}
	p.explosions = kept
}

// resolveCollisions resolves shell hits against the player and the
// active enemy. Called only in psPlaying.
func (p *playScene) resolveCollisions() {
	// Player shells vs enemy.
	if p.enemy != nil {
		kept := p.projectiles[:0]
		for _, pr := range p.projectiles {
			hit := false
			if pr.fromPlayer {
				if shellHitsEnemy(pr, p.enemy) {
					p.scoreEnemy(p.enemy)
					p.spawnExplosion(p.enemy.pos, 1.4)
					p.enemy = nil
					p.state = psWaiting
					p.stateT = 0
					hit = true
				}
			}
			if !hit {
				kept = append(kept, pr)
			}
		}
		p.projectiles = kept
	}
	// Enemy shells vs player.
	if p.state == psPlaying {
		kept := p.projectiles[:0]
		for _, pr := range p.projectiles {
			if pr.fromPlayer {
				kept = append(kept, pr)
				continue
			}
			if shellHitsPlayer(pr, p.cam.pos) {
				p.spawnExplosion(p.cam.pos, 1.2)
				p.killPlayer()
				continue
			}
			kept = append(kept, pr)
		}
		p.projectiles = kept
	}
}

// shellHitsEnemy returns true if a shell point is within hit radius of
// the enemy's centre. Missile gets a slightly larger bounding box.
func shellHitsEnemy(pr *projectile, e *enemy) bool {
	d := shortestDelta(pr.pos, e.pos)
	rad := shellHitRadius
	if e.kind == enemyMissile {
		rad = 1.4
	}
	if e.kind == enemySaucer {
		// Saucer sits high — match its altitude in collision.
		if math.Abs(pr.pos.y-e.pos.y) > 1.6 {
			return false
		}
		rad = 1.6
	}
	return d.x*d.x+d.z*d.z < rad*rad
}

// shellHitsPlayer is true when an enemy shell passes close enough to
// the player position to count. We use a generous radius since the
// terminal makes very small targets hard to hit cleanly.
func shellHitsPlayer(pr *projectile, player vec3) bool {
	d := shortestDelta(pr.pos, player)
	return d.x*d.x+d.z*d.z < 1.7*1.7
}

// scoreEnemy awards points for killing an enemy of a given kind and
// awards bonus lives at each bonusLifeEvery threshold.
func (p *playScene) scoreEnemy(e *enemy) {
	var pts int
	switch e.kind {
	case enemyTank:
		pts = scoreTank
	case enemySuperTank:
		pts = scoreSuperTank
	case enemyMissile:
		pts = scoreMissile
	case enemySaucer:
		pts = scoreSaucer
	}
	p.score += pts
	p.enemiesKilled++
	for p.score >= p.nextBonus {
		p.lives++
		p.nextBonus += bonusLifeEvery
	}
}

// killPlayer enters the dying state, drops a life, and triggers the
// cracked-screen overlay.
func (p *playScene) killPlayer() {
	if p.state == psPlayerDying || p.state == psGameOver {
		return
	}
	p.lives--
	p.crackT = crackDuration
	p.crackPattern = makeCrackPattern(p.rng, p.cam.cx, p.cam.cy, p.cam.cx)
	p.state = psPlayerDying
	p.stateT = 0
}

// respawnPlayer puts the player back at the spawn point and clears the
// active enemy. Crack overlay finishes fading on its own.
func (p *playScene) respawnPlayer() {
	p.cam.pos = vec3{x: 0, y: playerEyeHeight, z: 0}
	p.cam.yaw = 0
	p.projectiles = nil
	p.enemy = nil
	p.state = psWaiting
	p.stateT = 0
}

// spawnSpark adds a tiny pixel particle that fades out over ~0.5s.
func (p *playScene) spawnSpark(pos vec3) {
	dur := 0.4 + p.rng.Float64()*0.3
	p.sparks = append(p.sparks, &spark{
		pos:  pos,
		vel:  vec3{x: (p.rng.Float64()*2 - 1) * 0.5, y: -1.0, z: (p.rng.Float64()*2 - 1) * 0.5},
		life: dur,
		dur:  dur,
	})
}

// spawnExplosion creates an outward star-burst centred at pos. The size
// argument scales the maximum radius — small for shell-on-obstacle,
// larger for tank kills.
func (p *playScene) spawnExplosion(pos vec3, scale float64) {
	const rays = 9
	r := make([]float64, rays)
	for i := range r {
		r[i] = float64(i)*(2*math.Pi/float64(rays)) + p.rng.Float64()*0.4
	}
	dur := 0.55 + scale*0.25
	p.explosions = append(p.explosions, &explosion{
		pos:    pos,
		life:   dur,
		dur:    dur,
		radius: 1.0 * scale,
		rays:   r,
	})
}

// Draw is the engine.Scene draw hook.
func (p *playScene) Draw(c *engine.Canvas) {
	c.Clear(engine.Black)

	// 3D scene: sky decoration first (volcano, moon), then horizon &
	// mountains, then obstacles, enemy, projectiles, explosions. HUD
	// goes on top.
	p.drawSkyAndHorizon(c)
	p.drawObstacles(c)
	p.drawEnemy(c)
	p.drawProjectiles(c)
	p.drawSparks(c)
	p.drawExplosions(c)
	p.drawHUD(c)
}

// drawSkyAndHorizon paints the mountains, volcano, moon, and a thin
// horizon line. All scenery is treated as at infinity — it rotates
// with the player's yaw but ignores translation.
func (p *playScene) drawSkyAndHorizon(c *engine.Canvas) {
	w := c.Width()
	h := c.Height()
	horizonY := p.cam.cy
	// Horizon — a single horizontal line. Faintly green so it reads as
	// the ground/sky boundary.
	c.DrawLine(0, horizonY, w-1, horizonY, shadeGreen(0.4))

	// Mountains — sample the polyline at the screen's horizontal pixel
	// columns and draw vertical strokes from the horizon up to the
	// silhouette height. Each column's azimuth = (col - cx)*pixToRad
	// + cam.yaw, all wrapped to [0, 2π).
	halfFOV := math.Atan2(float64(w)/2, p.cam.focal)
	radPerPx := 2 * halfFOV / float64(w)
	prevY := -1
	prevX := -1
	for col := 0; col < w; col++ {
		az := p.cam.yaw + float64(col-p.cam.cx)*radPerPx
		az = math.Mod(az, 2*math.Pi)
		if az < 0 {
			az += 2 * math.Pi
		}
		mh := sampleMountains(p.mountainPolyline, az)
		// Convert silhouette height (in world units of ~hundreds of
		// distance) to pixel offset above the horizon.
		mPx := int(math.Round(mh * float64(h) * 0.18))
		y := horizonY - mPx
		if prevX >= 0 {
			c.DrawLine(prevX, prevY, col, y, shadeGreen(0.35))
		}
		prevX = col
		prevY = y
	}

	// Volcano — draw if its azimuth falls within the FOV. The volcano
	// is a fat triangle silhouette with an eruption plume on top.
	p.drawVolcano(c, halfFOV, radPerPx, horizonY)

	// Moon — same azimuth check, drawn as a thin crescent disc.
	p.drawMoon(c, halfFOV, radPerPx, horizonY)
}

// drawVolcano paints the cone silhouette and erupting lava when the
// volcano's azimuth is in the player's view.
func (p *playScene) drawVolcano(c *engine.Canvas, halfFOV, radPerPx float64, horizonY int) {
	relAz := normalizeAngle(p.volcanoAzimuth - p.cam.yaw)
	if math.Abs(relAz) > halfFOV+0.1 {
		return
	}
	col := int(math.Round(float64(p.cam.cx) + relAz/radPerPx))
	// Cone — a flat-topped triangle.
	const baseHalfPx = 16
	const peakHeightPx = 10
	leftX := col - baseHalfPx
	rightX := col + baseHalfPx
	peakX := col
	peakY := horizonY - peakHeightPx
	// Tilt the two sides asymmetrically so it doesn't look perfectly
	// symmetrical (which never reads as "natural mountain").
	c.DrawLine(leftX, horizonY, peakX-2, peakY, shadeGreen(0.7))
	c.DrawLine(rightX, horizonY, peakX+2, peakY, shadeGreen(0.7))
	c.DrawLine(peakX-2, peakY, peakX+2, peakY, shadeGreen(0.7))

	if p.volcanoErupting {
		// Plume — a few diverging streaks from the peak.
		for i := -2; i <= 2; i++ {
			topX := peakX + i*3 + int(math.Round(2*math.Sin(p.volcanoT*4+float64(i))))
			topY := peakY - 8 - i*i
			c.DrawLine(peakX, peakY, topX, topY, shadeGreen(0.9))
		}
		// Lava chunks falling along the sides.
		for i := 0; i < 4; i++ {
			fx := peakX + int(math.Round(10*math.Sin(p.volcanoT*3+float64(i))))
			fy := peakY + int(math.Round(4+float64(i*2)))
			c.Set(fx, fy, hudBright)
		}
	}
}

// drawMoon paints a small crescent moon at the moon's azimuth, well
// above the horizon.
func (p *playScene) drawMoon(c *engine.Canvas, halfFOV, radPerPx float64, horizonY int) {
	relAz := normalizeAngle(p.moonAzimuth - p.cam.yaw)
	if math.Abs(relAz) > halfFOV+0.1 {
		return
	}
	col := int(math.Round(float64(p.cam.cx) + relAz/radPerPx))
	mY := horizonY - 14
	col0 := shadeGreen(0.9)
	c.DrawCircle(col, mY, 3, col0)
	// Shade the right side to suggest a crescent — knock out a half-
	// disc by drawing a partial filled arc.
	for dy := -2; dy <= 2; dy++ {
		for dx := 0; dx <= 3; dx++ {
			if dx*dx+dy*dy <= 9 {
				c.Set(col+dx, mY+dy, engine.Black)
			}
		}
	}
	// Re-stroke the outline.
	c.DrawCircle(col, mY, 3, col0)
}

// sampleMountains returns the silhouette height at azimuth a, given the
// polyline of (azimuth → height) samples. Linear interpolation between
// neighbours; the polyline already wraps at 2π.
func sampleMountains(poly []vec2D, a float64) float64 {
	if len(poly) < 2 {
		return 0
	}
	// Polyline x is the azimuth, sampled monotonically from 0 to 2π.
	stride := poly[1].x - poly[0].x
	if stride <= 0 {
		return 0
	}
	idx := int(a / stride)
	if idx >= len(poly)-1 {
		idx = len(poly) - 2
	}
	t := (a - poly[idx].x) / stride
	return poly[idx].y + t*(poly[idx+1].y-poly[idx].y)
}

// drawObstacles renders all visible obstacles. Each obstacle is drawn
// at its nearest toroidal copy relative to the player, then frustum-
// culled by distance. The cull radius is set wide enough to keep the
// horizon populated even when the active enemy is far away.
func (p *playScene) drawObstacles(c *engine.Canvas) {
	const maxDraw = 120.0
	for _, o := range p.obstacles {
		pos := nearestCopy(p.cam.pos, o.pos)
		d := distanceXZ(p.cam.pos, pos)
		if d > maxDraw {
			continue
		}
		col := depthShade(d)
		p.cam.drawModel(c, o.edges, pos, 0, col)
	}
}

// drawEnemy renders the active enemy.
func (p *playScene) drawEnemy(c *engine.Canvas) {
	if p.enemy == nil {
		return
	}
	e := p.enemy
	pos := nearestCopy(p.cam.pos, e.pos)
	d := distanceXZ(p.cam.pos, pos)
	col := depthShade(d)
	// Subtly brighten close enemies — they're the thing the player
	// most needs to see.
	if d < 20 {
		col = shadeGreen(1.0)
	}
	switch e.kind {
	case enemyTank, enemySuperTank:
		p.cam.drawModel(c, p.tankModel(e.kind), pos, e.yaw, col)
	case enemyMissile:
		p.cam.drawModel(c, p.missileModel(), pos, e.yaw, col)
	case enemySaucer:
		p.cam.drawModel(c, p.saucerModel(), pos, e.yaw, col)
	}
}

// drawProjectiles draws each in-flight shell as a short bright line.
func (p *playScene) drawProjectiles(c *engine.Canvas) {
	for _, pr := range p.projectiles {
		col := hudBright
		if !pr.fromPlayer {
			col = engine.Color{R: 255, G: 180, B: 80, A: 255}
		}
		// Draw the swept segment from prev → current.
		a := nearestCopy(p.cam.pos, pr.prev)
		b := nearestCopy(p.cam.pos, pr.pos)
		// Slight elevation so shells render between the horizon and
		// turret height; gives them a clearly visible line tracer.
		a.y = 1.0
		b.y = 1.0
		p.cam.drawWorldLine(c, a, b, col)
	}
}

// drawSparks plots each spark as a single dim pixel.
func (p *playScene) drawSparks(c *engine.Canvas) {
	for _, sp := range p.sparks {
		k := sp.life / sp.dur
		col := shadeGreen(k)
		pos := nearestCopy(p.cam.pos, sp.pos)
		p.cam.drawWorldPoint(c, pos, col)
	}
}

// drawExplosions renders each explosion as a star of expanding spokes.
func (p *playScene) drawExplosions(c *engine.Canvas) {
	for _, ex := range p.explosions {
		k := ex.life / ex.dur
		col := shadeGreen(k*0.7 + 0.3)
		pos := nearestCopy(p.cam.pos, ex.pos)
		for _, ang := range ex.rays {
			a := vec3{
				x: pos.x,
				y: pos.y + 0.6,
				z: pos.z,
			}
			b := vec3{
				x: pos.x + math.Sin(ang)*ex.radius,
				y: pos.y + 0.6 + math.Cos(ang)*ex.radius*0.4,
				z: pos.z + math.Cos(ang)*ex.radius,
			}
			p.cam.drawWorldLine(c, a, b, col)
		}
	}
}

// tankModel returns the cached tank edge list. We build it once so we
// don't churn allocations every frame. Both standard and super tank
// share the same silhouette in the original.
func (p *playScene) tankModel(_ enemyKind) []edge {
	if cachedTankModel == nil {
		cachedTankModel = tankEdges()
	}
	return cachedTankModel
}

func (p *playScene) missileModel() []edge {
	if cachedMissileModel == nil {
		cachedMissileModel = missileEdges()
	}
	return cachedMissileModel
}

func (p *playScene) saucerModel() []edge {
	if cachedSaucerModel == nil {
		cachedSaucerModel = saucerEdges()
	}
	return cachedSaucerModel
}

var (
	cachedTankModel    []edge
	cachedMissileModel []edge
	cachedSaucerModel  []edge
)
