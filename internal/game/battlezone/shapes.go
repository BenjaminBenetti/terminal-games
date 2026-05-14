package battlezone

import "math"

// Wireframe model definitions used by the renderer. Coordinates are in
// local model space: the model's origin sits at its base centre on the
// ground (y=0), +Y is up, +Z is the model's "forward" direction.

// cubeEdges returns the 12 edges of an axis-aligned cube with the given
// side length, sitting flat on y=0.
func cubeEdges(side float64) []edge {
	h := side / 2
	// Vertices: 4 bottom (y=0), 4 top (y=side).
	v := [8]vec3{
		{-h, 0, -h}, // 0
		{h, 0, -h},  // 1
		{h, 0, h},   // 2
		{-h, 0, h},  // 3
		{-h, side, -h},
		{h, side, -h},
		{h, side, h},
		{-h, side, h},
	}
	return []edge{
		// Bottom square.
		{v[0], v[1]}, {v[1], v[2]}, {v[2], v[3]}, {v[3], v[0]},
		// Top square.
		{v[4], v[5]}, {v[5], v[6]}, {v[6], v[7]}, {v[7], v[4]},
		// Verticals.
		{v[0], v[4]}, {v[1], v[5]}, {v[2], v[6]}, {v[3], v[7]},
	}
}

// pyramidEdges returns the 8 edges of a square pyramid with the given
// base side length and the apex at height = side.
func pyramidEdges(side float64) []edge {
	h := side / 2
	v := [5]vec3{
		{-h, 0, -h},
		{h, 0, -h},
		{h, 0, h},
		{-h, 0, h},
		{0, side, 0},
	}
	return []edge{
		// Base square.
		{v[0], v[1]}, {v[1], v[2]}, {v[2], v[3]}, {v[3], v[0]},
		// Four sloping edges to apex.
		{v[0], v[4]}, {v[1], v[4]}, {v[2], v[4]}, {v[3], v[4]},
	}
}

// tankEdges returns the wireframe model of an enemy tank — two stubby
// treads, a hull between them, a turret, and a cannon barrel pointing
// along +Z. Numbers are tuned so the silhouette reads at the kind of
// distances we use in the play field (~20-60 world units).
func tankEdges() []edge {
	// Tread dimensions.
	const treadHalfLen = 1.4
	const treadHalfWidth = 0.35
	const treadHeight = 0.45
	const treadOffsetX = 0.85
	// Hull (the box between the treads).
	const hullHalfLen = 1.1
	const hullHalfWidth = 0.55
	const hullBottom = treadHeight
	const hullTop = treadHeight + 0.6
	// Turret (small box on top of hull).
	const turretHalfLen = 0.45
	const turretHalfWidth = 0.4
	const turretBottom = hullTop
	const turretTop = hullTop + 0.4
	// Cannon barrel — a single line extending forward from the turret.
	const barrelStart = 0.3 // local z, just inside turret face
	const barrelEnd = 1.7
	const barrelY = (turretBottom + turretTop) / 2

	var es []edge
	// Two treads as small axis-aligned boxes, one per side.
	for _, side := range []float64{-1, 1} {
		ox := side * treadOffsetX
		es = append(es, boxEdges(
			vec3{x: ox - treadHalfWidth, y: 0, z: -treadHalfLen},
			vec3{x: ox + treadHalfWidth, y: treadHeight, z: treadHalfLen})...)
	}
	// Hull box.
	es = append(es, boxEdges(
		vec3{x: -hullHalfWidth, y: hullBottom, z: -hullHalfLen},
		vec3{x: hullHalfWidth, y: hullTop, z: hullHalfLen})...)
	// Turret box.
	es = append(es, boxEdges(
		vec3{x: -turretHalfWidth, y: turretBottom, z: -turretHalfLen},
		vec3{x: turretHalfWidth, y: turretTop, z: turretHalfLen})...)
	// Cannon barrel — short line for visual hint of where it fires.
	es = append(es, edge{
		a: vec3{0, barrelY, barrelStart},
		b: vec3{0, barrelY, barrelEnd},
	})
	return es
}

// missileEdges returns a slim guided-missile silhouette: a long thin
// body with a pointed nose along +Z and three short fin lines at the
// tail. Sits centered on y=0 since it flies low.
func missileEdges() []edge {
	const halfW = 0.25
	const halfH = 0.25
	const tailZ = -1.5
	const noseZ = 1.6
	// Body box (extends from tailZ to a short of noseZ; nose is a
	// single converging point).
	es := boxEdges(
		vec3{-halfW, 0.4, tailZ},
		vec3{halfW, 0.4 + 2*halfH, noseZ - 0.6})
	// Nose cone — four lines from the body's front face to the apex.
	apex := vec3{0, 0.4 + halfH, noseZ}
	es = append(es,
		edge{vec3{-halfW, 0.4, noseZ - 0.6}, apex},
		edge{vec3{halfW, 0.4, noseZ - 0.6}, apex},
		edge{vec3{-halfW, 0.4 + 2*halfH, noseZ - 0.6}, apex},
		edge{vec3{halfW, 0.4 + 2*halfH, noseZ - 0.6}, apex})
	// Tail fins — two vertical wings.
	es = append(es,
		edge{vec3{-halfW, 0.4, tailZ}, vec3{-halfW - 0.5, 0.4, tailZ}},
		edge{vec3{halfW, 0.4, tailZ}, vec3{halfW + 0.5, 0.4, tailZ}},
		edge{vec3{0, 0.4 + 2*halfH, tailZ}, vec3{0, 0.4 + 2*halfH + 0.5, tailZ}})
	return es
}

// saucerEdges returns the bonus-points UFO — a flattened wireframe disc
// suggested by an octagon outline, a dome on top, and a fin underneath.
func saucerEdges() []edge {
	const r = 1.4
	const yMid = 1.8
	// Octagonal rim around the equator.
	const sides = 8
	var rim [sides]vec3
	for i := 0; i < sides; i++ {
		theta := float64(i) * (2 * math.Pi / sides)
		rim[i] = vec3{r * math.Sin(theta), yMid, r * math.Cos(theta)}
	}
	var es []edge
	for i := 0; i < sides; i++ {
		es = append(es, edge{rim[i], rim[(i+1)%sides]})
	}
	// Dome — a single ring above the rim plus 4 connecting struts.
	const domeR = 0.6
	const domeY = yMid + 0.55
	var dome [sides]vec3
	for i := 0; i < sides; i++ {
		theta := float64(i) * (2 * math.Pi / sides)
		dome[i] = vec3{domeR * math.Sin(theta), domeY, domeR * math.Cos(theta)}
	}
	for i := 0; i < sides; i++ {
		es = append(es, edge{dome[i], dome[(i+1)%sides]})
	}
	// Struts at four cardinal positions.
	for i := 0; i < sides; i += 2 {
		es = append(es, edge{rim[i], dome[i]})
	}
	// Underbody — a small inverted dome.
	const lowerR = 0.5
	const lowerY = yMid - 0.45
	var lower [sides]vec3
	for i := 0; i < sides; i++ {
		theta := float64(i) * (2 * math.Pi / sides)
		lower[i] = vec3{lowerR * math.Sin(theta), lowerY, lowerR * math.Cos(theta)}
	}
	for i := 0; i < sides; i++ {
		es = append(es, edge{lower[i], lower[(i+1)%sides]})
	}
	for i := 0; i < sides; i += 2 {
		es = append(es, edge{rim[i], lower[i]})
	}
	return es
}

// boxEdges returns the 12 edges of an axis-aligned box from corner lo
// to corner hi.
func boxEdges(lo, hi vec3) []edge {
	v := [8]vec3{
		{lo.x, lo.y, lo.z}, // 0
		{hi.x, lo.y, lo.z}, // 1
		{hi.x, lo.y, hi.z}, // 2
		{lo.x, lo.y, hi.z}, // 3
		{lo.x, hi.y, lo.z}, // 4
		{hi.x, hi.y, lo.z}, // 5
		{hi.x, hi.y, hi.z}, // 6
		{lo.x, hi.y, hi.z}, // 7
	}
	return []edge{
		// Bottom rectangle.
		{v[0], v[1]}, {v[1], v[2]}, {v[2], v[3]}, {v[3], v[0]},
		// Top rectangle.
		{v[4], v[5]}, {v[5], v[6]}, {v[6], v[7]}, {v[7], v[4]},
		// Verticals.
		{v[0], v[4]}, {v[1], v[5]}, {v[2], v[6]}, {v[3], v[7]},
	}
}
