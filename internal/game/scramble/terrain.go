package scramble

import (
	"math"
	"math/rand"
)

// terrain is a per-pixel-column profile of the ground top y and the
// ceiling bottom y for one stage. Outside the playfield the values are
// pinned to pfBot (ground) and pfTop (ceiling) — i.e. the open sky and
// floor only intrude into the playfield where the heightmap says so.
//
// Coordinates: ground[x] is the y-pixel of the topmost terrain pixel of
// the ground column at world-x (the playable area is at y < ground[x]).
// ceil[x] is the y-pixel of the bottom-most ceiling pixel (the playable
// area is at y > ceil[x]). For a stage without a ceiling the ceil values
// equal pfTop-1 so they never collide.
type terrain struct {
	ground []int
	ceil   []int
	pfTop  int
	pfBot  int
	// kind selects the rendering style.
	kind terrainKind
}

type terrainKind int

const (
	terMountain terrainKind = iota // green jagged silhouette
	terFlat                        // green low-relief ground, no ceiling
	terMountainNoCeil              // same as terMountain without ceiling stalactites
	terCavern                      // tunnel — both ground and ceiling, tight
	terCity                        // blue blocky skyline of buildings
	terBase                        // grey base corridor, both ground and ceiling
)

// stageWorldWidth returns the world width in pixels for the given stage.
// It scales with the canvas so each stage lasts ~25–40 seconds at the
// default scroll speed regardless of terminal size.
func stageWorldWidth(stage, canvasW int) int {
	mult := 6
	if stage == 6 {
		mult = 4
	}
	w := mult * canvasW
	if w < 480 {
		w = 480
	}
	if w > 1600 {
		w = 1600
	}
	return w
}

// newTerrain builds the heightmap for a stage. pfTop and pfBot are the
// pixel y-bounds of the playfield (HUD ends at pfTop).
func newTerrain(stage, worldW, pfTop, pfBot int, rng *rand.Rand) *terrain {
	t := &terrain{
		ground: make([]int, worldW),
		ceil:   make([]int, worldW),
		pfTop:  pfTop,
		pfBot:  pfBot,
	}
	for i := 0; i < worldW; i++ {
		t.ground[i] = pfBot
		t.ceil[i] = pfTop - 1
	}
	switch stage {
	case 1:
		t.kind = terMountain
		t.genMountains(rng, 0.55, 0.92, true)
	case 2:
		t.kind = terFlat
		t.genFlat(rng, 0.83)
	case 3:
		t.kind = terMountainNoCeil
		t.genMountains(rng, 0.50, 0.88, false)
	case 4:
		t.kind = terCavern
		t.genCavern(rng)
	case 5:
		t.kind = terCity
		t.genCity(rng)
	case 6:
		t.kind = terBase
		t.genBase()
	}
	return t
}

// at clamps x and returns the ground/ceiling y for that column.
func (t *terrain) at(x int) (groundY, ceilY int) {
	if x < 0 {
		x = 0
	}
	if x >= len(t.ground) {
		x = len(t.ground) - 1
	}
	return t.ground[x], t.ceil[x]
}

// hits reports whether the rectangle [x0,x1) × [y0,y1) overlaps any
// terrain column. Used for player-vs-terrain and projectile-vs-terrain.
func (t *terrain) hits(x0, y0, x1, y1 int) bool {
	if x0 < 0 {
		x0 = 0
	}
	if x1 > len(t.ground) {
		x1 = len(t.ground)
	}
	for x := x0; x < x1; x++ {
		g, c := t.ground[x], t.ceil[x]
		if y1 > g || y0 <= c {
			return true
		}
	}
	return false
}

// genMountains lays down a rolling hill silhouette. gMin/gMax are the
// fraction of playfield height the ground occupies. withCeiling adds a
// matching jagged ceiling above (stalactites in stage 1).
func (t *terrain) genMountains(rng *rand.Rand, gMin, gMax float64, withCeiling bool) {
	pfH := float64(t.pfBot - t.pfTop)
	w := len(t.ground)
	phase1 := rng.Float64() * 2 * math.Pi
	phase2 := rng.Float64() * 2 * math.Pi
	phase3 := rng.Float64() * 2 * math.Pi
	for i := 0; i < w; i++ {
		f := float64(i)
		h := 0.5 + 0.3*math.Sin(f/14+phase1) + 0.2*math.Sin(f/29+phase2)
		if h < 0 {
			h = 0
		}
		if h > 1 {
			h = 1
		}
		depth := gMin + (gMax-gMin)*h
		t.ground[i] = t.pfTop + int(pfH*depth)
	}
	// Add a little pixel-noise jitter so the silhouette doesn't read as
	// a perfectly smooth analytic curve.
	for i := 0; i < w; i++ {
		if rng.Intn(7) == 0 {
			t.ground[i] += rng.Intn(2)
		}
		if t.ground[i] > t.pfBot {
			t.ground[i] = t.pfBot
		}
	}
	if withCeiling {
		for i := 0; i < w; i++ {
			f := float64(i)
			h := 0.5 + 0.3*math.Sin(f/17+phase3) + 0.2*math.Sin(f/41+phase1)
			depth := 0.04 + 0.18*h
			c := t.pfTop + int(pfH*depth)
			if c < t.pfTop {
				c = t.pfTop
			}
			t.ceil[i] = c
		}
	}
}

// genFlat is a near-level ground with very light variation, used for the
// open-sky UFO sector (stage 2). No ceiling.
func (t *terrain) genFlat(rng *rand.Rand, frac float64) {
	pfH := float64(t.pfBot - t.pfTop)
	w := len(t.ground)
	base := t.pfTop + int(pfH*frac)
	for i := 0; i < w; i++ {
		d := 0
		if rng.Intn(5) == 0 {
			d = rng.Intn(2)
		}
		t.ground[i] = base + d
		if t.ground[i] > t.pfBot {
			t.ground[i] = t.pfBot
		}
	}
}

// genCavern is the cavern of mystery — a snaking tunnel with both a
// ground and a ceiling pinching the playable cross-section. The corridor
// width is held to roughly 35–55% of playfield height.
func (t *terrain) genCavern(rng *rand.Rand) {
	pfH := float64(t.pfBot - t.pfTop)
	w := len(t.ground)
	centreY := t.pfTop + int(pfH*0.5)
	halfBase := int(pfH * 0.22)
	phase := rng.Float64() * 2 * math.Pi
	phase2 := rng.Float64() * 2 * math.Pi
	for i := 0; i < w; i++ {
		f := float64(i)
		swayBase := math.Sin(f/24+phase) + 0.5*math.Sin(f/57+phase2)
		centre := centreY + int(pfH*0.18*swayBase)
		// Periodically pinch the corridor for tension.
		pinch := 1.0
		if int(f/45)%2 == 0 {
			pinch = 0.78
		}
		h := int(float64(halfBase) * pinch)
		t.ceil[i] = centre - h
		t.ground[i] = centre + h
		if t.ceil[i] < t.pfTop {
			t.ceil[i] = t.pfTop
		}
		if t.ground[i] > t.pfBot {
			t.ground[i] = t.pfBot
		}
	}
}

// genCity lays out tall buildings of varying heights against the ground.
// Gaps in between let UFO missiles fire upward, while the rooftops form
// obstacles the player must fly over.
func (t *terrain) genCity(rng *rand.Rand) {
	pfH := float64(t.pfBot - t.pfTop)
	w := len(t.ground)
	base := t.pfTop + int(pfH*0.88)
	for i := 0; i < w; i++ {
		t.ground[i] = base
	}
	cursor := 4 + rng.Intn(5)
	for cursor < w-4 {
		bw := 6 + rng.Intn(11)
		bh := int(pfH * (0.18 + rng.Float64()*0.42))
		top := base - bh
		if top < t.pfTop+3 {
			top = t.pfTop + 3
		}
		for i := 0; i < bw && cursor+i < w; i++ {
			t.ground[cursor+i] = top
		}
		cursor += bw + 4 + rng.Intn(6)
	}
}

// genBase paints the base corridor — broad open until the final 30%
// where the corridor pinches toward the reactor.
func (t *terrain) genBase() {
	pfH := float64(t.pfBot - t.pfTop)
	w := len(t.ground)
	for i := 0; i < w; i++ {
		progress := float64(i) / float64(w)
		var gFrac, cFrac float64
		switch {
		case progress < 0.65:
			gFrac = 0.87
			cFrac = 0.06
		default:
			p := (progress - 0.65) / 0.35
			gFrac = 0.87 - p*0.20
			cFrac = 0.06 + p*0.12
		}
		t.ground[i] = t.pfTop + int(pfH*gFrac)
		t.ceil[i] = t.pfTop + int(pfH*cFrac)
		if t.ground[i] > t.pfBot {
			t.ground[i] = t.pfBot
		}
		if t.ceil[i] < t.pfTop {
			t.ceil[i] = t.pfTop
		}
	}
}
