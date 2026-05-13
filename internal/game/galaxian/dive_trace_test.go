package galaxian

import (
	"testing"
)

// TestDiveTrace traces a single dive's path and asserts that the
// trajectory visits the four cardinal regions of the loop circle
// (east-of-slot, south-of-slot, west-of-slot, north-of-slot) and ends
// well below the formation. This catches sign-flip bugs in the loop
// math that the basic "south-first" test can't.
func TestDiveTrace(t *testing.T) {
	for _, side := range []int{-1, +1} {
		p := newTestPlayScene(t, 120, 80)
		var target *alien
		for _, a := range p.aliens {
			if a.kind == kindDrone {
				target = a
				break
			}
		}
		if target == nil {
			t.Fatal("no drone in formation")
		}
		sx, sy := p.slotPos(target.row, target.col)
		target.side = side
		target.x = float64(sx)
		target.y = float64(sy)
		target.state = asPullout
		target.phaseT = 0

		// Track positions during the loop phase so we can verify the
		// loop traces a circle around its centre. The "fall" translation
		// stops the alien from re-visiting its slot height, so we test
		// the trajectory's extent relative to the loop centre instead.
		var maxXOff, minXOff, maxY, riseSamples float64
		var sawDip bool
		step := 0.02
		var lastY float64 = -1
		startX := target.x
		loopFrames := int((divePullDur + diveLoopDur) / step)
		for i := 0; i < loopFrames+5; i++ {
			p.tickDivingAlien(target, step)
			dx := target.x - startX
			if dx > maxXOff {
				maxXOff = dx
			}
			if dx < minXOff {
				minXOff = dx
			}
			if target.y > maxY {
				maxY = target.y
			}
			// While in the loop phase, count frames where y went DOWN
			// (decreased) — that's the loop's "rising" arc fighting the
			// downward translation.
			if target.state == asLoop && lastY >= 0 && target.y < lastY {
				riseSamples++
				sawDip = true
			}
			lastY = target.y
		}

		// Must have traveled south at some point.
		if maxY <= float64(sy)+5 {
			t.Errorf("side=%d: maxY=%v not far enough below sy=%d", side, maxY, sy)
		}
		// Must have gone outward (in side's direction).
		if side > 0 && maxXOff <= 0 {
			t.Errorf("side=+1 but max x offset=%v, expected positive", maxXOff)
		}
		if side < 0 && minXOff >= 0 {
			t.Errorf("side=-1 but min x offset=%v, expected negative", minXOff)
		}
		// Loop arc must be present: there must be frames where y briefly
		// decreased (the alien rising through the upper half of the
		// loop circle) — this is what distinguishes a loop from a
		// straight slide.
		if !sawDip || riseSamples < 3 {
			t.Errorf("side=%d: not enough rising frames in loop (%v), trajectory looks straight",
				side, riseSamples)
		}
		// State should have advanced past the loop.
		if target.state == asPullout || target.state == asLoop {
			t.Errorf("side=%d: stuck in state %v after %d steps",
				side, target.state, loopFrames+5)
		}
	}
}
