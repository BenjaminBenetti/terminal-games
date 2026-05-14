package vanguard

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// gondBoss is the chamber-boss at the end of the Styx zone. The fight
// stops world scrolling — the boss occupies the top of the play area
// and fires telegraphed energy bolts at the player. The player must
// land 20 hits on the central core (the brain).
//
// The visual is gondBody (a ringed brain) with a periodic eye-glow
// overlay that telegraphs an incoming bolt.
type gondBoss struct {
	x       float64 // sprite-left x
	targetY float64 // resting y (after entry)
	entryY  float64 // current y; equals targetY once entry is done
	hp      int
	maxHP   int

	moveT     float64 // sway timer
	fireCD    float64 // bolt cool-down
	hitFlashT float64 // brief white flash on hit
	deathT    float64 // explosion timer (>=0 once killed)
}

// newGond builds a boss positioned to roll in from above.
func newGond(p *playScene) *gondBoss {
	g := &gondBoss{
		hp:      20,
		maxHP:   20,
		x:       float64((p.w - gondBody.width()) / 2),
		targetY: float64(p.playTop) + 3,
		entryY:  float64(p.playTop) - float64(gondBody.height()),
	}
	return g
}

func (g *gondBoss) alive() bool { return g.hp > 0 }

// coreRect is the hitbox the player must land bullets on. Slightly
// smaller than the full body so corner pixels of the brain ring don't
// register as hits.
func (g *gondBoss) coreRect() rect {
	w := gondBody.width()
	h := gondBody.height()
	pad := 2
	return rect{
		x0: int(g.x) + pad,
		y0: int(g.entryY) + pad,
		x1: int(g.x) + w - pad,
		y1: int(g.entryY) + h - pad,
	}
}

func (g *gondBoss) update(p *playScene, s float64) {
	if !g.alive() {
		g.deathT += s
		return
	}
	g.moveT += s
	g.fireCD -= s
	if g.hitFlashT > 0 {
		g.hitFlashT -= s
	}

	// Sway side-to-side around screen centre. Amplitude shrinks at low
	// hp (the boss is "wounded" and stays nearer the centre).
	centre := float64((p.w - gondBody.width()) / 2)
	hpFrac := float64(g.hp) / float64(g.maxHP)
	amp := float64(p.w/4) * (0.4 + 0.6*hpFrac)
	g.x = centre + amp*math.Sin(g.moveT*0.7)

	if g.fireCD <= 0 && len(p.enemyBullets) < maxEnemyBullets {
		g.fireCD = 0.9 - 0.25*(1-hpFrac) // fires faster as it weakens
		g.fireBolt(p)
	}
}

// fireBolt spawns a tracking bolt out of the Gond's centre toward the
// current player position. At low hp the boss occasionally fires a
// three-way spread.
func (g *gondBoss) fireBolt(p *playScene) {
	cx := g.x + float64(gondBody.width())/2
	cy := g.entryY + float64(gondBody.height())
	pcx := p.player.x + float64(playerShip.width())/2
	pcy := p.player.y + float64(playerShip.height())/2
	dx := pcx - cx
	dy := pcy - cy
	speed := enemyBulletSpeed + 6
	bolt := func(angOff float64) {
		ang := math.Atan2(dy, dx) + angOff
		vx := math.Cos(ang) * speed
		vy := math.Sin(ang) * speed
		p.enemyBullets = append(p.enemyBullets, &bullet{
			x: cx, y: cy, vx: vx, vy: vy, fromPlayer: false,
		})
	}
	bolt(0)
	hpFrac := float64(g.hp) / float64(g.maxHP)
	if hpFrac < 0.5 {
		bolt(0.3)
		bolt(-0.3)
	}
}

func (g *gondBoss) draw(c *engine.Canvas, p *playScene) {
	col := engine.Color{R: 220, G: 110, B: 220, A: 255}
	if g.hitFlashT > 0 {
		col = engine.White
	}
	if !g.alive() {
		// Big crackling explosion frames overlapping the body.
		t := g.deathT
		seed := int(t * 60)
		for i := 0; i < 80; i++ {
			rng := (seed*73 + i*131 + 17) & 0xfff
			ox := float64(rng%gondBody.width()*2-gondBody.width()) * 0.5
			oy := float64((rng/13)%gondBody.height()*2-gondBody.height()) * 0.5
			x := int(g.x) + gondBody.width()/2 + int(ox+t*float64(rng%9-4))
			y := int(g.entryY) + gondBody.height()/2 + int(oy+t*float64(rng%7-3))
			cl := engine.Color{R: 255, G: uint8(180 - int(t*40)%180), B: 80, A: 255}
			c.Set(x, y, cl)
		}
		return
	}
	drawSprite(c, int(g.x), int(g.entryY), gondBody, col)

	// Eye-glow telegraph when the boss is about to fire.
	if g.fireCD < 0.25 {
		drawSprite(c, int(g.x), int(g.entryY), gondEyeGlow, engine.White)
	}

	// HP bar below the boss.
	w := gondBody.width()
	barX := int(g.x)
	barY := int(g.entryY) + gondBody.height() + 1
	if barY < p.playTop+p.playH-2 {
		fillW := int(float64(w) * float64(g.hp) / float64(g.maxHP))
		for i := 0; i < w; i++ {
			cl := engine.Color{R: 60, G: 0, B: 0, A: 255}
			if i < fillW {
				cl = engine.Color{R: 240, G: 60, B: 60, A: 255}
			}
			c.Set(barX+i, barY, cl)
		}
	}
}
