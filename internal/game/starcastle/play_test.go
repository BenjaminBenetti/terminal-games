package starcastle

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// TestComputeGeometry checks that the geometry is well-ordered and fits
// inside the canvas.
func TestComputeGeometry(t *testing.T) {
	for _, tc := range []struct {
		w, h int
	}{
		{80, 48},
		{100, 60},
		{160, 80},
		{60, 40},
	} {
		g := computeGeometry(tc.w, tc.h)
		if !(g.coreR < g.innerInnerR &&
			g.innerInnerR < g.innerOuterR &&
			g.innerOuterR < g.middleInnerR &&
			g.middleInnerR < g.middleOuterR &&
			g.middleOuterR < g.outerInnerR &&
			g.outerInnerR < g.outerOuterR) {
			t.Errorf("geometry not strictly nested for %dx%d: %+v", tc.w, tc.h, g)
		}
		if g.outerOuterR > float64(tc.w)/2 || g.outerOuterR > float64(tc.h)/2 {
			t.Errorf("rings spill off canvas for %dx%d: outerR=%v", tc.w, tc.h, g.outerOuterR)
		}
		if g.coreR < 1 {
			t.Errorf("core too small for %dx%d: coreR=%v", tc.w, tc.h, g.coreR)
		}
	}
}

// TestSegmentAt verifies that the segment index is consistent with the
// segment span and respects ring rotation.
func TestSegmentAt(t *testing.T) {
	r := ring{}
	for i := range r.segments {
		r.segments[i].alive = true
	}
	r.angle = 0
	// Right of centre, angle 0, should be segment 0.
	if got := segmentAt(&r, 0); got != 0 {
		t.Errorf("segmentAt(0)=%d want 0", got)
	}
	// Just inside segment 0.
	if got := segmentAt(&r, segmentSpan()*0.99); got != 0 {
		t.Errorf("end of segment 0 mis-indexed: %d", got)
	}
	// Start of segment 1.
	if got := segmentAt(&r, segmentSpan()*1.01); got != 1 {
		t.Errorf("start of segment 1 mis-indexed: %d", got)
	}
	// Rotate the ring and re-check.
	r.angle = segmentSpan()
	if got := segmentAt(&r, segmentSpan()*1.5); got != 0 {
		t.Errorf("segment 0 after rotation: %d", got)
	}
}

// TestPointHitsRing checks the polar collision routine on a fresh
// ring (all segments alive).
func TestPointHitsRing(t *testing.T) {
	g := computeGeometry(80, 48)
	r := newRing(ringOuter, g)
	// Pick a point right at the centre of the outer ring's annulus.
	rMid := (g.outerOuterR + g.outerInnerR) / 2
	px := g.cx + rMid
	py := g.cy
	if idx := pointHitsRing(&r, g, px, py); idx < 0 {
		t.Errorf("expected hit on alive segment, got -1 at (%v,%v)", px, py)
	}
	// Destroy that segment and confirm no hit.
	idx := pointHitsRing(&r, g, px, py)
	destroySegment(&r, idx)
	if got := pointHitsRing(&r, g, px, py); got != -1 {
		t.Errorf("expected miss on destroyed segment, got %d", got)
	}
	// A point well outside the outer ring shouldn't hit.
	if got := pointHitsRing(&r, g, g.cx+g.outerOuterR+5, g.cy); got != -1 {
		t.Errorf("hit outside outer radius: got %d", got)
	}
	// A point inside the inner radius shouldn't hit.
	if got := pointHitsRing(&r, g, g.cx+g.outerInnerR-1, g.cy); got != -1 {
		t.Errorf("hit inside inner radius: got %d", got)
	}
}

// TestUpdateRingRegen confirms that destroyed segments come back after
// the regen delay.
func TestUpdateRingRegen(t *testing.T) {
	g := computeGeometry(80, 48)
	r := newRing(ringOuter, g)
	destroySegment(&r, 3)
	if r.segments[3].alive {
		t.Fatal("destroySegment didn't kill segment")
	}
	// Tick just under the regen time — still dead.
	updateRing(&r, r.regenDelay*0.9)
	if r.segments[3].alive {
		t.Fatal("segment regenerated too early")
	}
	// Tick past the regen time.
	updateRing(&r, r.regenDelay)
	if !r.segments[3].alive {
		t.Fatal("segment failed to regenerate")
	}
}

// TestPlaySceneSmoke runs a handful of frames through a fresh play
// scene to confirm nothing panics during construction, simulation,
// or drawing.
func TestPlaySceneSmoke(t *testing.T) {
	e, err := engine.New(engine.Options{
		Width:  80,
		Height: 48,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	p := newPlayScene(e, 0)
	canvas := e.Canvas()
	dt := 16 * time.Millisecond
	for i := 0; i < 30; i++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("Update tick %d: %v", i, err)
		}
		p.Draw(canvas)
	}
	// Force a core kill to exercise the level-cleared path too.
	p.killCore()
	for i := 0; i < 5; i++ {
		if err := p.Update(dt); err != nil {
			t.Fatalf("post-kill tick %d: %v", i, err)
		}
		p.Draw(canvas)
	}
}

// TestSceneSmoke does the same for the title scene wrapper.
func TestSceneSmoke(t *testing.T) {
	e, err := engine.New(engine.Options{
		Width:  80,
		Height: 48,
		Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	s := newScene(e)
	canvas := e.Canvas()
	dt := 16 * time.Millisecond
	for i := 0; i < 10; i++ {
		if err := s.Update(dt); err != nil {
			t.Fatalf("Update tick %d: %v", i, err)
		}
		s.Draw(canvas)
	}
}

// TestWrapPi confirms wrapPi normalizes correctly across boundaries.
func TestWrapPi(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 0},
		{math.Pi, math.Pi},
		{-math.Pi + 0.01, -math.Pi + 0.01},
		{2 * math.Pi, 0},
		{3 * math.Pi, math.Pi},
		{-3 * math.Pi, -math.Pi + 2*math.Pi - 2*math.Pi}, // = -π → wrapped to π
	}
	for _, tc := range cases {
		got := wrapPi(tc.in)
		// Allow either ±π for the boundary.
		if math.Abs(got-tc.want) > 1e-9 && math.Abs(math.Abs(got)-math.Pi) > 1e-9 {
			t.Errorf("wrapPi(%v)=%v want %v", tc.in, got, tc.want)
		}
	}
}
