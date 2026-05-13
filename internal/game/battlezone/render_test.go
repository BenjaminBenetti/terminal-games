package battlezone

import (
	"math"
	"testing"
)

// TestProjectionCenter verifies that a world point directly in front of
// the camera at yaw=0 projects to the screen centre.
func TestProjectionCenter(t *testing.T) {
	cam := camera{
		pos:   vec3{x: 0, y: 0, z: 0},
		yaw:   0,
		focal: 80,
		cx:    40,
		cy:    20,
	}
	cp := cam.toCamera(vec3{x: 0, y: 0, z: 10})
	sx, sy, ok := cam.project(cp)
	if !ok {
		t.Fatal("expected point in front of camera to project")
	}
	if math.Abs(sx-40) > 0.001 || math.Abs(sy-20) > 0.001 {
		t.Fatalf("expected (40, 20), got (%v, %v)", sx, sy)
	}
}

// TestYawRotation verifies that turning the camera 90° right brings
// the world's +X axis to "directly in front".
func TestYawRotation(t *testing.T) {
	cam := camera{
		pos:   vec3{x: 0, y: 0, z: 0},
		yaw:   math.Pi / 2,
		focal: 80,
		cx:    40,
		cy:    20,
	}
	cp := cam.toCamera(vec3{x: 10, y: 0, z: 0})
	if math.Abs(cp.z-10) > 0.001 {
		t.Fatalf("expected cz≈10 after yaw=90° looking at +X point, got %v", cp.z)
	}
	if math.Abs(cp.x) > 0.001 {
		t.Fatalf("expected cx≈0, got %v", cp.x)
	}
}

// TestNearPlaneCullsBehind ensures that a segment entirely behind the
// near plane is culled.
func TestNearPlaneCullsBehind(t *testing.T) {
	a := vec3{x: 0, y: 0, z: -1}
	b := vec3{x: 0, y: 0, z: -2}
	_, _, ok := clipNearPlane(a, b)
	if ok {
		t.Fatal("expected segment behind near plane to be culled")
	}
}

// TestNearPlaneClipsCrossing ensures that a segment crossing the near
// plane is clipped to z = nearZ.
func TestNearPlaneClipsCrossing(t *testing.T) {
	a := vec3{x: 0, y: 0, z: -1}
	b := vec3{x: 0, y: 0, z: 5}
	ca, cb, ok := clipNearPlane(a, b)
	if !ok {
		t.Fatal("expected clipped segment to remain visible")
	}
	if math.Abs(ca.z-nearZ) > 1e-9 {
		t.Fatalf("expected clipped endpoint at z=nearZ (%v), got %v", nearZ, ca.z)
	}
	if cb.z != 5 {
		t.Fatalf("front endpoint should be untouched, got z=%v", cb.z)
	}
}

// TestWorldWrap exercises wrap and shortest-delta wrapping math.
func TestWorldWrap(t *testing.T) {
	if got := wrapWorld(worldSize/2 + 1); math.Abs(got-(-worldSize/2+1)) > 1e-9 {
		t.Fatalf("wrapWorld did not wrap positive overflow: got %v", got)
	}
	if got := wrapWorld(-worldSize/2 - 1); math.Abs(got-(worldSize/2-1)) > 1e-9 {
		t.Fatalf("wrapWorld did not wrap negative overflow: got %v", got)
	}
	// Two points on opposite ends should choose the short path.
	from := vec3{x: -worldSize/2 + 1, z: 0}
	to := vec3{x: worldSize/2 - 1, z: 0}
	d := shortestDelta(from, to)
	if math.Abs(d.x) > 5 {
		t.Fatalf("expected short wrapped delta, got dx=%v", d.x)
	}
}

// TestNormalizeAngle wraps angles into (-π, π].
func TestNormalizeAngle(t *testing.T) {
	cases := []struct{ in, out float64 }{
		{0, 0},
		{math.Pi, math.Pi},
		{-math.Pi + 0.0001, -math.Pi + 0.0001},
		{3 * math.Pi, math.Pi},
		{-3 * math.Pi, math.Pi},
		{4 * math.Pi, 0},
	}
	for _, c := range cases {
		got := normalizeAngle(c.in)
		if math.Abs(got-c.out) > 1e-6 {
			t.Errorf("normalizeAngle(%v): want %v, got %v", c.in, c.out, got)
		}
	}
}

// TestSegmentCircleIntersects covers the boundary cases used for shell
// vs obstacle and line-of-sight tests.
func TestSegmentCircleIntersects(t *testing.T) {
	// Segment passing through circle.
	if !segmentCircleIntersects(-5, 0, 5, 0, 0, 0, 1) {
		t.Fatal("expected segment crossing origin to hit unit circle")
	}
	// Segment missing the circle.
	if segmentCircleIntersects(-5, 5, 5, 5, 0, 0, 1) {
		t.Fatal("expected segment offset by y=5 to miss unit circle")
	}
	// Endpoint inside circle.
	if !segmentCircleIntersects(0.2, 0, 5, 0, 0, 0, 1) {
		t.Fatal("expected segment starting inside circle to hit")
	}
}
