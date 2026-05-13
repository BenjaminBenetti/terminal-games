package scramble

import "math/rand"

// populateStage seeds the entity list for the given stage. Positions are
// world-space; entities sit dormant in the slice until the camera draws
// near. The terrain heightmap already reflects the stage's silhouette;
// here we sprinkle rockets, fuel depots, UFOs, fireballs, towers, and
// the reactor according to which sector we're flying through.
func populateStage(stage int, t *terrain, worldW int, rng *rand.Rand) []*entity {
	var list []*entity
	pfH := t.pfBot - t.pfTop
	_ = pfH

	switch stage {
	case 1:
		// Mountains: lots of rockets on launch pads, regular fuel depots,
		// and a sparse UFO escort above.
		placeGroundEntities(t, worldW, &list, 32, 14, entRocket, rng)
		placeGroundEntities(t, worldW, &list, 110, 28, entFuel, rng)
		placeAirEntities(t, worldW, &list, 70, 22, entUFO, rng)
	case 2:
		// UFO fleet — sky is thick with saucers, with periodic refuels.
		placeAirEntities(t, worldW, &list, 40, 18, entUFO, rng)
		placeGroundEntities(t, worldW, &list, 130, 30, entFuel, rng)
	case 3:
		// Fireball storm — meteors fall from above mountainous ground
		// already laid down by terrain. We pre-populate fireballs at
		// staggered world-x positions; they're parked off-screen above
		// the playfield and start falling once the camera approaches.
		placeFireballs(t, worldW, &list, 28, 12, rng)
		placeGroundEntities(t, worldW, &list, 140, 32, entFuel, rng)
	case 4:
		// Cavern of mystery — terrain is the puzzle, but a handful of
		// fuel tanks and the occasional rocket lurking on the floor
		// keep the player honest.
		placeGroundEntities(t, worldW, &list, 100, 28, entFuel, rng)
		placeGroundEntities(t, worldW, &list, 80, 22, entRocket, rng)
	case 5:
		// City — anti-aircraft towers on building rooftops with refuels
		// in the gaps between buildings.
		placeRooftopTowers(t, worldW, &list, rng)
		placeGroundEntities(t, worldW, &list, 130, 30, entFuel, rng)
	case 6:
		// Base — towers along the corridor floor and the reactor parked
		// at the end as the run-clearing objective.
		placeGroundEntities(t, worldW, &list, 60, 22, entTower, rng)
		placeGroundEntities(t, worldW, &list, 110, 28, entFuel, rng)
		placeReactor(t, worldW, &list)
	}
	return list
}

// placeGroundEntities walks the world from left to right placing one
// entity of the given kind every "spacing ± jitter" pixels. Entities
// sit with their feet on the terrain's ground level at that column.
func placeGroundEntities(t *terrain, worldW int, list *[]*entity,
	spacing, jitter int, kind entityKind, rng *rand.Rand) {
	cursor := 60 + rng.Intn(40)
	for cursor < worldW-20 {
		x := cursor
		// Skip columns where the ground would push the entity off the
		// top of the playfield (e.g., a sharp mountain peak in stage 1).
		g, _ := t.at(x)
		_, h := entitySize(kind, false)
		spawnY := g - h
		if spawnY < t.pfTop+2 {
			cursor += spacing/2 + rng.Intn(spacing/2+1)
			continue
		}
		e := &entity{
			kind:  kind,
			x:     float64(x),
			y:     float64(spawnY),
			alive: true,
		}
		*list = append(*list, e)
		cursor += spacing + rng.Intn(jitter*2+1) - jitter
		if cursor < x+8 {
			cursor = x + 8
		}
	}
}

// placeAirEntities scatters UFOs (or similar) at random heights in the
// upper half of the playfield. Each one drifts horizontally and
// oscillates vertically once active.
func placeAirEntities(t *terrain, worldW int, list *[]*entity,
	spacing, jitter int, kind entityKind, rng *rand.Rand) {
	cursor := 80 + rng.Intn(40)
	pfH := t.pfBot - t.pfTop
	for cursor < worldW-20 {
		y := t.pfTop + 3 + rng.Intn(pfH/2)
		_, h := entitySize(kind, false)
		_ = h
		// A small leftward drift makes saucers feel like they're flying
		// at the player against the scrolling backdrop.
		vx := -6.0 - rng.Float64()*4
		vy := 0.0
		e := &entity{
			kind:  kind,
			x:     float64(cursor),
			y:     float64(y),
			vx:    vx,
			vy:    vy,
			alive: true,
		}
		*list = append(*list, e)
		cursor += spacing + rng.Intn(jitter*2+1) - jitter
	}
}

// placeFireballs parks meteors above the playfield. Their downward
// velocity is set at spawn (here) so the diagonals look natural; once
// the camera approaches and they enter the visible window they're free
// to fall. They keep their motion regardless of player position.
func placeFireballs(t *terrain, worldW int, list *[]*entity,
	spacing, jitter int, rng *rand.Rand) {
	cursor := 60 + rng.Intn(30)
	for cursor < worldW-10 {
		startY := t.pfTop - 6
		vx := -4.0 - rng.Float64()*6
		vy := 18.0 + rng.Float64()*12
		e := &entity{
			kind:  entFireball,
			x:     float64(cursor),
			y:     float64(startY),
			vx:    vx,
			vy:    vy,
			alive: true,
		}
		*list = append(*list, e)
		cursor += spacing + rng.Intn(jitter*2+1) - jitter
	}
}

// placeRooftopTowers searches the city skyline for rooftop columns wide
// enough to anchor a tower and places one there.
func placeRooftopTowers(t *terrain, worldW int, list *[]*entity, rng *rand.Rand) {
	cursor := 30
	for cursor < worldW-10 {
		// Find a stretch where the ground is at the same height for a
		// few columns — that's a rooftop the tower can sit on.
		y, _ := t.at(cursor)
		run := 0
		for j := cursor; j < worldW && j < cursor+20; j++ {
			yj, _ := t.at(j)
			if yj == y {
				run++
			} else {
				break
			}
		}
		if run >= baseTower.width()+2 && y < t.pfBot-4 {
			_, h := entitySize(entTower, false)
			spawnY := y - h
			if spawnY < t.pfTop+1 {
				cursor += 5
				continue
			}
			*list = append(*list, &entity{
				kind:  entTower,
				x:     float64(cursor + 1),
				y:     float64(spawnY),
				alive: true,
			})
			cursor += run + 12 + rng.Intn(20)
			continue
		}
		cursor += 4 + rng.Intn(5)
	}
}

// placeReactor drops the reactor target at the right-hand end of the
// base corridor, vertically centred between ground and ceiling there.
func placeReactor(t *terrain, worldW int, list *[]*entity) {
	x := worldW - reactor.width() - 4
	if x < 0 {
		return
	}
	g, c := t.at(x + reactor.width()/2)
	mid := (g + c) / 2
	y := mid - reactor.height()/2
	if y < t.pfTop+1 {
		y = t.pfTop + 1
	}
	*list = append(*list, &entity{
		kind:  entReactor,
		x:     float64(x),
		y:     float64(y),
		alive: true,
		hits:  0,
	})
}
