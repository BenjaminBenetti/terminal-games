package battlezone

import (
	"math"
	"math/rand"
)

// The world is a square plain that wraps toroidally. Obstacles are
// generated once at match start and never move; the player and enemies
// wrap their X/Z coordinates modulo worldSize so the playfield feels
// infinite. Battlezone's original world was vast but bounded; wrapping
// is the simplest faithful approximation in a small terminal canvas.
const worldSize = 200.0

// obstacleKind selects the silhouette of a single static obstacle. The
// original cabinet placed cubes and pyramids as cover.
type obstacleKind int

const (
	obstacleCube obstacleKind = iota
	obstaclePyramid
)

// obstacle is a single static block of cover.
type obstacle struct {
	kind obstacleKind
	pos  vec3
	size float64
	// Cached edge list to avoid rebuilding per frame.
	edges []edge
	// radius is the X-Z bounding circle used for collision checks.
	radius float64
}

// generateObstacles places n obstacles across the world, avoiding the
// player's spawn point at the centre.
func generateObstacles(rng *rand.Rand, n int) []*obstacle {
	out := make([]*obstacle, 0, n)
	const minClearance = 12.0 // keep spawn area open
	for len(out) < n {
		x := rng.Float64()*worldSize - worldSize/2
		z := rng.Float64()*worldSize - worldSize/2
		// Reject placements too close to spawn so the first enemy isn't
		// born inside a cube.
		if math.Hypot(x, z) < minClearance {
			continue
		}
		// Reject placements that overlap an existing obstacle.
		const sizeMin = 1.6
		const sizeMax = 3.2
		size := sizeMin + rng.Float64()*(sizeMax-sizeMin)
		overlap := false
		for _, o := range out {
			if math.Hypot(o.pos.x-x, o.pos.z-z) < o.radius+size+1.0 {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		var k obstacleKind
		if rng.Intn(2) == 0 {
			k = obstacleCube
		} else {
			k = obstaclePyramid
		}
		var es []edge
		if k == obstacleCube {
			es = cubeEdges(size)
		} else {
			es = pyramidEdges(size)
		}
		out = append(out, &obstacle{
			kind:   k,
			pos:    vec3{x: x, y: 0, z: z},
			size:   size,
			edges:  es,
			radius: size * 0.6,
		})
	}
	return out
}

// wrapWorld returns v reduced to (-worldSize/2, worldSize/2]. Used for
// player and enemy positions so the playfield is toroidal.
func wrapWorld(v float64) float64 {
	half := worldSize / 2
	for v > half {
		v -= worldSize
	}
	for v <= -half {
		v += worldSize
	}
	return v
}

// shortestDelta returns the shortest signed XZ-plane offset from
// 'from' to 'to' on the toroidal world. Used so AI navigation and
// collision detection both pick the nearest wrapped copy.
func shortestDelta(from, to vec3) vec3 {
	dx := wrapWorld(to.x - from.x)
	dz := wrapWorld(to.z - from.z)
	return vec3{x: dx, y: to.y - from.y, z: dz}
}

// nearestCopy returns the position of 'target' translated to the
// nearest toroidal copy relative to 'observer'. Drawing uses this so an
// enemy near a wrap edge appears where it visually should.
func nearestCopy(observer, target vec3) vec3 {
	d := shortestDelta(observer, target)
	return vec3{
		x: observer.x + d.x,
		y: target.y,
		z: observer.z + d.z,
	}
}

// segmentBlocked reports whether the line segment from a to b in world
// space is blocked by any obstacle whose XZ bounding circle the segment
// passes through. Used by enemy AI for line-of-sight checks and by
// shells for collision against obstacles.
func segmentBlocked(obstacles []*obstacle, a, b vec3) (*obstacle, bool) {
	for _, o := range obstacles {
		// Move the obstacle to the nearest toroidal copy of 'a'.
		pos := nearestCopy(a, o.pos)
		if segmentCircleIntersects(a.x, a.z, b.x, b.z, pos.x, pos.z, o.radius) {
			return o, true
		}
	}
	return nil, false
}

// segmentCircleIntersects tests a 2D line segment against a circle.
// Returns true if any point of the segment is within radius r of the
// circle's centre.
func segmentCircleIntersects(x0, y0, x1, y1, cx, cy, r float64) bool {
	dx := x1 - x0
	dy := y1 - y0
	fx := x0 - cx
	fy := y0 - cy
	a := dx*dx + dy*dy
	if a == 0 {
		// Degenerate segment.
		return fx*fx+fy*fy <= r*r
	}
	b := 2 * (fx*dx + fy*dy)
	cc := fx*fx + fy*fy - r*r
	disc := b*b - 4*a*cc
	if disc < 0 {
		return false
	}
	sq := math.Sqrt(disc)
	t1 := (-b - sq) / (2 * a)
	t2 := (-b + sq) / (2 * a)
	return (t1 >= 0 && t1 <= 1) || (t2 >= 0 && t2 <= 1)
}
