package battlezone

import (
	"math"

	"github.com/BenjaminBenetti/terminal-games/internal/engine"
)

// vec3 is a 3D world-space point or vector. Right-handed coordinates:
// +X right, +Y up, +Z forward. The ground plane is y=0.
type vec3 struct {
	x, y, z float64
}

// camera holds the player's eye position, heading, and projection
// parameters. yaw is a rotation around +Y so that yaw=0 looks along +Z
// and yaw=+π/2 looks along +X (turning right is positive yaw).
type camera struct {
	pos   vec3
	yaw   float64
	focal float64 // pixel focal length — bigger = narrower FOV
	cx    int     // horizon pixel column (canvas center)
	cy    int     // horizon pixel row
}

// nearZ is the camera-space depth below which lines are clipped to
// avoid the singular point at z=0 and the inversion behind the camera.
const nearZ = 0.4

// toCamera converts a world point to camera space, where the camera
// sits at the origin looking down +Z.
func (cam *camera) toCamera(w vec3) vec3 {
	dx := w.x - cam.pos.x
	dy := w.y - cam.pos.y
	dz := w.z - cam.pos.z
	cs := math.Cos(cam.yaw)
	sn := math.Sin(cam.yaw)
	return vec3{
		x: cs*dx - sn*dz,
		y: dy,
		z: sn*dx + cs*dz,
	}
}

// project converts a camera-space point to screen pixel coordinates.
// Returns false when the point is at or behind the near plane and must
// not be projected directly.
func (cam *camera) project(p vec3) (sx, sy float64, ok bool) {
	if p.z < nearZ {
		return 0, 0, false
	}
	sx = float64(cam.cx) + cam.focal*p.x/p.z
	sy = float64(cam.cy) - cam.focal*p.y/p.z
	return sx, sy, true
}

// clipNearPlane returns the visible portion of a camera-space line
// segment intersected with z >= nearZ. ok is false if both endpoints
// are behind the near plane.
func clipNearPlane(a, b vec3) (vec3, vec3, bool) {
	if a.z < nearZ && b.z < nearZ {
		return a, b, false
	}
	if a.z >= nearZ && b.z >= nearZ {
		return a, b, true
	}
	// One endpoint is behind. Walk from the behind endpoint toward the
	// front endpoint until we hit z = nearZ.
	if a.z < nearZ {
		t := (nearZ - a.z) / (b.z - a.z)
		a = vec3{
			x: a.x + t*(b.x-a.x),
			y: a.y + t*(b.y-a.y),
			z: nearZ,
		}
		return a, b, true
	}
	t := (nearZ - b.z) / (a.z - b.z)
	b = vec3{
		x: b.x + t*(a.x-b.x),
		y: b.y + t*(a.y-b.y),
		z: nearZ,
	}
	return a, b, true
}

// drawWorldLine projects a world-space line segment and draws it on the
// canvas in the given color, clipping against the near plane. Returns
// true if any pixels were drawn (best-effort — the canvas itself does
// the final pixel-bound clipping).
func (cam *camera) drawWorldLine(c *engine.Canvas, a, b vec3, color engine.Color) bool {
	ca := cam.toCamera(a)
	cb := cam.toCamera(b)
	ca, cb, ok := clipNearPlane(ca, cb)
	if !ok {
		return false
	}
	sx0, sy0, ok0 := cam.project(ca)
	sx1, sy1, ok1 := cam.project(cb)
	if !ok0 || !ok1 {
		return false
	}
	c.DrawLine(int(math.Round(sx0)), int(math.Round(sy0)),
		int(math.Round(sx1)), int(math.Round(sy1)), color)
	return true
}

// drawWorldPoint plots a single pixel for a world point, if it's in
// front of the near plane. Used for shells and sparks.
func (cam *camera) drawWorldPoint(c *engine.Canvas, p vec3, color engine.Color) {
	cp := cam.toCamera(p)
	sx, sy, ok := cam.project(cp)
	if !ok {
		return
	}
	c.Set(int(math.Round(sx)), int(math.Round(sy)), color)
}

// drawModel renders a wireframe model with the given local edges,
// translated and rotated into world space. Edges are pairs of vec3 in
// local model coordinates; modelYaw rotates around +Y and origin
// places the model's local origin in world space.
type edge struct {
	a, b vec3
}

func (cam *camera) drawModel(c *engine.Canvas, edges []edge, origin vec3, modelYaw float64, color engine.Color) {
	cs := math.Cos(modelYaw)
	sn := math.Sin(modelYaw)
	tx := func(p vec3) vec3 {
		return vec3{
			x: origin.x + cs*p.x + sn*p.z,
			y: origin.y + p.y,
			z: origin.z + -sn*p.x + cs*p.z,
		}
	}
	for _, e := range edges {
		cam.drawWorldLine(c, tx(e.a), tx(e.b), color)
	}
}

// distance computes Euclidean distance between two world points using
// only the X-Z plane (ground plane). Battlezone's tanks all sit on the
// ground and most distance checks ignore vertical offset.
func distanceXZ(a, b vec3) float64 {
	dx := a.x - b.x
	dz := a.z - b.z
	return math.Sqrt(dx*dx + dz*dz)
}

// normalizeAngle wraps an angle into (-π, π].
func normalizeAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// shadeGreen scales a base phosphor green by k in [0,1]. Used to fade
// far-away wireframes so depth reads in monochrome.
func shadeGreen(k float64) engine.Color {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	// Phosphor green base: gentle but bright. Floor the dim end so
	// distant edges stay visible against pure black.
	return engine.Color{
		R: uint8(20 + 80*k),
		G: uint8(80 + 175*k),
		B: uint8(30 + 90*k),
		A: 255,
	}
}

// depthShade picks a green intensity for a wireframe object at distance
// d (in world units). Closer = brighter. The fade end is set so an
// enemy entering at maximum spawn range (~96 units) is dim but still
// readable as a silhouette against the horizon.
func depthShade(d float64) engine.Color {
	const fadeStart = 8.0
	const fadeEnd = 110.0
	if d <= fadeStart {
		return shadeGreen(1)
	}
	if d >= fadeEnd {
		return shadeGreen(0.18)
	}
	k := 1 - (d-fadeStart)/(fadeEnd-fadeStart)
	return shadeGreen(k*0.85 + 0.15)
}
