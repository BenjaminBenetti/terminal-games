package pacman

import (
	"math"
	"math/rand"
)

// ghostKind names the four classic ghosts. The index doubles as
// scatter-corner / colour-table key.
type ghostKind int

const (
	blinky ghostKind = iota // red,  top-right scatter
	pinky                   // pink, top-left scatter
	inky                    // cyan, bottom-right scatter
	clyde                   // orange, bottom-left scatter
)

func (k ghostKind) String() string {
	switch k {
	case blinky:
		return "BLINKY"
	case pinky:
		return "PINKY"
	case inky:
		return "INKY"
	case clyde:
		return "CLYDE"
	}
	return "?"
}

// ghostMode is the per-ghost state. The global scatter/chase phase
// (selected by the mode timer) only applies when the ghost is in the
// "outside, hunting" condition — eaten ghosts, ghosts still bobbing in
// the house, and ghosts climbing out of the house override the global
// phase with their own behaviour.
type ghostMode int

const (
	modeScatter       ghostMode = iota // patrol corner
	modeChase                          // pursue Pac-Man (per-ghost logic)
	modeFrightened                     // edible blue, random wander
	modeEaten                          // eyes returning to house, AI-driven
	modeEntering                       // arrived at entry tile, scripted dive into the house
	modeInHouse                        // bobbing in the house, waiting on release
	modeLeavingHouse                   // climbing from house centre to the door
)

// scatterTarget is each ghost's permanent scatter destination. The
// arcade targets are off-screen at the corners; the ghost continually
// fails to reach the corner, which produces the characteristic
// patrol loop in that quadrant.
var scatterTarget = [4][2]int{
	blinky: {25, 0},
	pinky:  {2, 0},
	inky:   {27, mazeRows - 1},
	clyde:  {0, mazeRows - 1},
}

// ghostHouse coordinates the spatial constants of the ghost house.
const (
	houseEntryX = 13.5 // pixel-x just outside the door (between cols 13/14)
	houseEntryY = 11.5 // row immediately above the door
	houseFloorY = 14.5 // y of the house interior "floor" line that bobbing ghosts oscillate around
)

// ghostHouseSlot holds the inside-house parking spot for each ghost
// kind. Blinky's slot is unused (he starts outside the door); Pinky
// sits in the middle, Inky on the left, Clyde on the right.
var ghostHouseSlot = [4][2]float64{
	blinky: {13.5, 11.5},
	pinky:  {13.5, 14.5},
	inky:   {11.5, 14.5},
	clyde:  {15.5, 14.5},
}

// ghost is one of the four pursuers. It embeds the generic entity so
// the movement system in entity.go drives its position, plus a small
// amount of bookkeeping for AI decisions.
type ghost struct {
	entity
	kind ghostKind
	mode ghostMode

	// dotsRequired is the number of dots Pac-Man must eat before this
	// ghost leaves the house at the start of a level. Blinky's is 0
	// (he starts outside). Mirrors the arcade's per-ghost dot counter
	// approximately — we use the global dot count rather than the
	// preferred-target counter for simplicity.
	dotsRequired int

	// bobT advances while modeInHouse for the up/down bob animation.
	bobT float64

	// rng is the ghost's frightened-mode random source (held by the
	// playScene; injected at construction).
	rng *rand.Rand

	// lastDecisionTile records the tile in which the ghost last picked
	// a direction. Used so the AI doesn't re-decide every frame inside
	// the same tile — only once per tile centre arrival.
	lastDecisionTile [2]int
}

// newGhost constructs a ghost in its starting position. Blinky spawns
// at the door pillar (just outside the house), facing left to fall
// into the top corridor. The others spawn at their house slots,
// facing up so they emerge cleanly when released.
func newGhost(kind ghostKind, rng *rand.Rand, dotsRequired int) *ghost {
	g := &ghost{
		kind:         kind,
		dotsRequired: dotsRequired,
		rng:          rng,
	}
	switch kind {
	case blinky:
		g.x, g.y = houseEntryX, houseEntryY
		g.dir = dirLeft
		g.mode = modeScatter
	default:
		slot := ghostHouseSlot[kind]
		g.x, g.y = slot[0], slot[1]
		g.dir = dirUp
		g.mode = modeInHouse
	}
	g.lastDecisionTile = [2]int{-1, -1}
	return g
}

// ghostAI is the per-frame AI driver. It looks at the ghost's current
// position, mode, and global mode/phase and updates the ghost's
// `desired` direction so the embedded entity's movement step will
// turn at the next tile centre.
//
// pac is Pac-Man's current state (position + direction), blinkyTile
// is Blinky's tile (needed for Inky's "pivot doubled" target), phase
// is the global scatter/chase phase, and frightenedDir provides the
// ghost's randomness source for frightened mode.
type aiInputs struct {
	pacTileX, pacTileY int
	pacDir             direction
	blinkyTileX        int
	blinkyTileY        int
	phase              ghostMode // either modeScatter or modeChase
}

// chooseDirection picks the direction the ghost should be heading
// from the *given* tile, treating r as forbidden (the 180° reversal
// rule). The choice minimises Euclidean distance from each candidate
// neighbour to the target tile, breaking ties in arcade order
// (up, left, down, right).
func (g *ghost) chooseDirection(col, row int, forbidden direction, target [2]int, canPass canPasser) direction {
	bestDir := dirNone
	bestDist := math.Inf(1)
	for _, d := range allMoves {
		if d == forbidden {
			continue
		}
		nx, ny := col+d.dx(), row+d.dy()
		if !canPass(nx, ny) {
			continue
		}
		dx := float64(nx - target[0])
		dy := float64(ny - target[1])
		dist := dx*dx + dy*dy
		if dist < bestDist {
			bestDist = dist
			bestDir = d
		}
	}
	return bestDir
}

// chooseFrightened picks a non-reversal direction at random from the
// walkable neighbours. Used in modeFrightened only.
func (g *ghost) chooseFrightened(col, row int, forbidden direction, canPass canPasser) direction {
	var options []direction
	for _, d := range allMoves {
		if d == forbidden {
			continue
		}
		if canPass(col+d.dx(), row+d.dy()) {
			options = append(options, d)
		}
	}
	if len(options) == 0 {
		return dirNone
	}
	return options[g.rng.Intn(len(options))]
}

// targetTile returns the AI target for the ghost based on its kind and
// the current chase/scatter mode. Frightened mode doesn't consult this
// (it picks randomly), and eaten mode targets the house entry
// directly; both bypass this function.
func (g *ghost) targetTile(in aiInputs) [2]int {
	if in.phase == modeScatter {
		return scatterTarget[g.kind]
	}

	switch g.kind {
	case blinky:
		return [2]int{in.pacTileX, in.pacTileY}

	case pinky:
		// Four tiles ahead of Pac-Man, with the famous "up bug": when
		// Pac-Man faces up, the target shifts four tiles up AND four
		// tiles left — a side effect of the original ROM doing
		// (-=4, -=4) instead of just (-=4) on Y.
		tx, ty := in.pacTileX, in.pacTileY
		switch in.pacDir {
		case dirUp:
			ty -= 4
			tx -= 4
		case dirDown:
			ty += 4
		case dirLeft:
			tx -= 4
		case dirRight:
			tx += 4
		}
		return [2]int{tx, ty}

	case inky:
		// Pivot = 2 tiles ahead of Pac-Man (same up-bug). Target =
		// pivot reflected through Blinky's tile — i.e. (2*pivot - blinky).
		px, py := in.pacTileX, in.pacTileY
		switch in.pacDir {
		case dirUp:
			py -= 2
			px -= 2
		case dirDown:
			py += 2
		case dirLeft:
			px -= 2
		case dirRight:
			px += 2
		}
		return [2]int{2*px - in.blinkyTileX, 2*py - in.blinkyTileY}

	case clyde:
		// 8+ tiles away: chase like Blinky. Closer than 8: revert to
		// own scatter corner, which makes Clyde feel skittish around
		// Pac-Man.
		dx := float64(g.tileX() - in.pacTileX)
		dy := float64(g.tileY() - in.pacTileY)
		if dx*dx+dy*dy >= 64 {
			return [2]int{in.pacTileX, in.pacTileY}
		}
		return scatterTarget[clyde]
	}
	return [2]int{in.pacTileX, in.pacTileY}
}

// pickNextDirection runs the right algorithm for the ghost's current
// mode and returns the direction to set as `desired`. The caller is
// responsible for only invoking this once per tile-centre arrival.
func (g *ghost) pickNextDirection(in aiInputs, canPass canPasser) direction {
	col, row := g.tileX(), g.tileY()
	forbidden := g.dir.opposite()
	switch g.mode {
	case modeFrightened:
		return g.chooseFrightened(col, row, forbidden, canPass)
	case modeEaten:
		// Target the tile directly above the door so the eyes line up
		// to enter the house. (houseEntryX is between cols 13 and 14;
		// targeting col 13 lets the ghost arrive aligned for the dive
		// down through the door.)
		target := [2]int{13, 11}
		return g.chooseDirection(col, row, forbidden, target, canPass)
	case modeChase, modeScatter:
		return g.chooseDirection(col, row, forbidden, g.targetTile(in), canPass)
	}
	return g.dir
}
