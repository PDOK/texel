package pointindex

import (
	"math"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mapslicehelp"
	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/morton"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"
)

type SegmentIdx struct {
	ringIdx  int
	pointIdx int
}

type TileClassification int

const (
	ClassificationUnknown TileClassification = iota
	ClassificationIntersect
	ClassificationInside
	ClassificationOutside
)

func (ix *PointIndex) DetectTilesViaLineTrace(g geom.Geometry, tmsID tms20.TMID, buffer uint) []tile.Tile {
	switch g := g.(type) {
	case geom.Polygon:
		return ix.lineTracePolygon(g, tmsID, buffer)
	case geom.LineString:
		return ix.lineTraceLine(g, tmsID, buffer)
	case geom.Point:
		return ix.lineTracePoint(g, tmsID, buffer)
	default:
		return nil

	}
}

//////////////////////////
// General line tracing //
//////////////////////////

// registerFunc marks a tile at (xCoord, yCoord) (in tile coordinates at
// level l) as touched. Callers that need to associate the touched tile
// with a segment should do so via a variable captured in their closure.
type registerFunc func(xCoord, yCoord uint, l Level)

// Line trace over a line, registering tiles hit with `register`.
// Given a line, detect in which quadrant the line advances. Essentially do a breadth-first search in that direction.
func (ix *PointIndex) lineTrace(line geom.Line, tileLevel, intPixLevel Level, buffer uint, register registerFunc) {
	intLine := intgeom.FromGeomLine(line)

	// Detect direction
	var dx, dy int
	if intLine.Point1().X() < intLine.Point2().X() {
		dx = 1
	} else {
		dx = -1
	}
	if intLine.Point1().Y() < intLine.Point2().Y() {
		dy = 1
	} else {
		dy = -1
	}

	startTileX, startTileY := ix.findTile(intLine.Point1(), tileLevel)
	startX, startY := int(startTileX), int(startTileY)              //nolint:gosec // G115
	bufferSize := ix.getResolution(intPixLevel) * intgeom.M(buffer) //nolint:gosec // G115

	// We will advance in a certain direction.
	// Here, we cover the tiles "behind" us.
	ix.tryRegisterTile(intLine, startX-dx, startY+dy, tileLevel, bufferSize, register)
	ix.tryRegisterTile(intLine, startX-dx, startY, tileLevel, bufferSize, register)
	ix.tryRegisterTile(intLine, startX-dx, startY-dy, tileLevel, bufferSize, register)
	ix.tryRegisterTile(intLine, startX, startY-dy, tileLevel, bufferSize, register)
	ix.tryRegisterTile(intLine, startX+dx, startY-dy, tileLevel, bufferSize, register)
	ix.tryRegisterTile(intLine, startX, startY, tileLevel, bufferSize, register)

	type coord struct{ x, y int }

	// Advance in steps. At each step, check all relevant tiles of an anti-diagonal.
	// Initial anti-diagonal contains only start tile.
	// We keep track of the previous two steps; we need one extra step for the rare case a
	// line passes exactly through a corner at buffer = 0.
	frontier := []coord{{startX, startY}}
	prevFrontier := make([]coord, 0)

	// Candidates for the next step are determined by previous successes. Loop stops after we have
	// run out of successes.
	for len(prevFrontier)+len(frontier) > 0 {
		// Set of next candidates
		nextSet := make(map[coord]bool, len(frontier)*2+len(prevFrontier))
		for _, cur := range frontier {
			nextSet[coord{cur.x + dx, cur.y}] = true
			nextSet[coord{cur.x, cur.y + dy}] = true
		}
		for _, prev := range prevFrontier {
			nextSet[coord{prev.x + dx, prev.y + dy}] = true
		}

		// Try registering and record successes
		next := make([]coord, 0, len(nextSet))
		for c := range nextSet {
			if ix.tryRegisterTile(intLine, c.x, c.y, tileLevel, bufferSize, register) {
				next = append(next, c)
			}
		}

		// Update set of past successes
		prevFrontier = frontier
		frontier = next
	}
}

// Check whether tile hits line and registers upon success. Also returns success.
func (ix *PointIndex) tryRegisterTile(line intgeom.Line, x, y int, l Level, bufferSize intgeom.M, register registerFunc) bool {
	maxCoord := int(mathhelp.Pow2(l)) - 1 //nolint:gosec // G115
	if x < 0 || y < 0 || x > maxCoord || y > maxCoord {
		return false // out of bounds: no tile there, nothing to register
	}
	ux, uy := uint(x), uint(y)
	extent, _ := ix.getQuadrantExtentAndCentroid(l, ux, uy, ix.intExtent)
	if tileIntersectsLine(line, extent, bufferSize) {
		register(ux, uy, l)
		return true
	}
	return false
}

// Detect whether line intersects extent, inflated by buffer.
// Uses same logic as quadrants: left and bottom edge belong to extent, top and right do not.
func tileIntersectsLine(line intgeom.Line, extent intgeom.Extent, buffer intgeom.M) bool {
	bufferedExtent := intgeom.Extent{
		extent.MinX() - buffer,
		extent.MinY() - buffer,
		extent.MaxX() + buffer,
		extent.MaxY() + buffer,
	}

	return lineIntersects(line, bufferedExtent)
}

///////////////////////////////
// Line tracing for polygons //
///////////////////////////////

// Line tracing for polygons is special in the sense that we do not just trace the lines,
// but also find tiles that lie entirely on the interior of the polygon. This second process
// we call "filling".

// Detect tiles for polygon using line tracing.
// First line trace, then fill remaining tiles. This enables us to classify tiles as being "Inside" and "Outside" the polygon.
// Then process output of `fillTiles` to return a list at tile level.
func (ix *PointIndex) lineTracePolygon(polygon geom.Polygon, tmsID tms20.TMID, buffer uint) (tiles []tile.Tile) {
	targetLevel := LevelFromTmsId(tmsID)
	segements, classification := ix.lineTracePolygonEdges(polygon, tmsID, buffer)
	ix.fillTiles(targetLevel, 0, 0, true, segements, classification, polygon)

	// Extract list of tiles at tile level.
	tiles = make([]tile.Tile, 0)
	for level, classificationAtLevel := range classification {
		for z, class := range classificationAtLevel {
			switch class {
			case ClassificationInside:
				size := mathhelp.Pow2((targetLevel - level) * 2)
				baseZ := z << (2 * (targetLevel - level))
				for i := range size {
					tiles = append(tiles, ix.makeTileZ(baseZ+i, targetLevel, true))
				}
			case ClassificationOutside:
				continue
			case ClassificationIntersect:
				if level == targetLevel {
					tiles = append(tiles, ix.makeTileZ(z, targetLevel, false))
				}
			case ClassificationUnknown:
				panic("ClassificationUnknown tile for polygon during linetrace")
			}
		}
	}
	return tiles
}

// Loop over polygon edges and apply line trace.
// Use a special registering function that keeps track of which edges hit which tiles.
func (ix *PointIndex) lineTracePolygonEdges(polygon geom.Polygon, tmsID tms20.TMID, buffer uint) (segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification) {
	tileLevel := LevelFromTmsId(tmsID)
	intPixLevel := ix.InternalPixelLevelFromTmsID(tmsID)

	// Initialize data
	segments = make(map[morton.Z][]SegmentIdx)
	classification = make(map[Level]map[morton.Z]TileClassification, tileLevel+1)
	for l := range tileLevel + 1 {
		classification[l] = make(map[morton.Z]TileClassification)
	}

	// Helper registering function that marks a tile and all its parents as "Intersecting".
	var markIntersected func(l Level, z morton.Z)
	markIntersected = func(l Level, z morton.Z) {
		if classification[l][z] == ClassificationIntersect {
			return
		}
		classification[l][z] = ClassificationIntersect
		if l == 0 {
			return
		}
		markIntersected(l-1, z>>2)
	}

	// Register function that registers when a segment hits a tile and then marks tiles as "Intersected".
	// Note the use of `idx` in this closure: we manually update it in the loop below.
	var idx SegmentIdx
	register := func(x, y uint, l Level) {
		z := morton.MustToZ(x, y)
		segments[z] = append(segments[z], idx)
		markIntersected(l, z)
	}

	// Loop over all edges
	for ringIdx, ring := range polygon.LinearRings() {
		for pointIdx := range ring {
			// Update `idx` for use in the `register` closure
			idx = SegmentIdx{ringIdx: ringIdx, pointIdx: pointIdx}
			// Apply line trace
			line := geom.Line{ring[pointIdx], ring[(pointIdx+1)%len(ring)]}
			ix.lineTrace(line, tileLevel, intPixLevel, buffer, register)
		}
	}
	return segments, classification
}

// Recurse into quadtree, filling entire area as either "Inside", "Outside", or "Intersect" at tile level.
// A classification of "Inside" or "Outside" at a higher level means that all children at tile level have this classification.
// We assume that "Intersect" at the tile level is already classified, and that parents are also marked as "Intersect".
// This means that tiles marked as "Intersect" will need to be recursed into for fine-grained classification.
// It also means that at any level, all children of an unmarked tile have the same classification
// This enables the classification at higher levels.
// We also assume that `segments` contains the data of which segments of polygon intersect which tiles at tile level.
func (ix *PointIndex) fillTiles(targetLevel, currentLevel Level, currentZ morton.Z, containsAll bool, segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification, polygon geom.Polygon) {
	if targetLevel == currentLevel {
		return
	}

	// Get children
	children := getChildren(currentZ)
	intersectingChildren := make([]morton.Z, 0, 4)
	nextLevel := currentLevel + 1

	// First classify unknown children. This can be done at a higher level
	for _, child := range children {
		if _, present := classification[nextLevel][child]; present {
			intersectingChildren = append(intersectingChildren, child)
			continue
		}
		if containsAll {
			classification[nextLevel][child] = ClassificationOutside
		} else {
			childAtTargetLevel := child << ((targetLevel - nextLevel) * 2)
			classification[nextLevel][child] = ix.fillTile(childAtTargetLevel, targetLevel, segments, classification, polygon)
		}
	}

	containsAll = containsAll && len(intersectingChildren) < 2

	// For intersecting children we recurse to the next level
	for _, child := range intersectingChildren {
		if _, present := classification[nextLevel][child]; present {
			ix.fillTiles(targetLevel, nextLevel, child, containsAll, segments, classification, polygon)
		}
	}
}

func getChildren(z morton.Z) [4]morton.Z {
	shift := z << 2
	return [4]morton.Z{shift, shift + 1, shift + 2, shift + 3}
}

// Use raycast to classify a tile as "Inside" or "Outside". Does not work on tiles that are "Intersect".
// tileLevel needs to be the actual tile level and z needs to be a coordinate at this level
// Relies on classification and segments as pre-computed values.
func (ix *PointIndex) fillTile(z morton.Z, tileLevel Level, segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification, polygon geom.Polygon) TileClassification {
	x, y := morton.FromZ(z)
	intersectingTilesLeft := ix.findIntersectingTilesLeft(x, y, tileLevel, classification)

	tileHeightCoord := ix.getResolution(tileLevel)*intgeom.M(y) + ix.intExtent.MinY() //nolint:gosec // G115

	seen := make(map[SegmentIdx]bool)
	numIntersections := 0

	// Raycast and loop over intersecting segments
	for _, z := range intersectingTilesLeft {
		for _, segment := range segments[z] {
			if seen[segment] {
				continue
			}
			seen[segment] = true
			ring := polygon.LinearRings()[segment.ringIdx]

			y1 := intgeom.FromGeomOrd(ring[segment.pointIdx][1])
			y2 := intgeom.FromGeomOrd(ring[(segment.pointIdx+1)%len(ring)][1])

			minY := min(y1, y2)
			maxY := max(y1, y2)

			// Logic for deciding relevant segments
			// Needed for when ray intersects vertex of polygon
			switch {
			case minY == maxY:
			case maxY < tileHeightCoord:
			case minY >= tileHeightCoord:
			default:
				numIntersections++
			}
		}
	}

	// Result determined by "Jordan Curve Theorem"
	if numIntersections%2 == 0 {
		return ClassificationOutside
	}
	return ClassificationInside
}

// Helper function for raycasting function `fillTile`. Finds all tiles left of argument containing segments of polygon.
// tileLevel needs to be the actual tile level and x and y need to be coordinates for this level.
// Relies on `classification` as pre-computed information.
// Searches by binary search.
func (ix *PointIndex) findIntersectingTilesLeft(x, y, tileLevel Level, classification map[Level]map[morton.Z]TileClassification) []morton.Z {
	intersectingCurrentLevel := []morton.Z{0}
	var intersectingNextLevel []morton.Z
	var leftChild, rightChild morton.Z

	// Binary search over the tile level
	for currentLevel := range tileLevel {
		intersectingNextLevel = make([]morton.Z, 0)

		// Coordinates of parent of tile at the next level
		xAtNextLevel := x >> (tileLevel - currentLevel - 1)
		yAtNextLevel := y >> (tileLevel - currentLevel - 1)

		// Decide whether we need the upper or lower children for the next level
		nextLevelDown := yAtNextLevel%2 == 0
		for _, z := range intersectingCurrentLevel {
			// Compute candidates
			if nextLevelDown {
				leftChild = z << 2
				rightChild = (z << 2) + 1
			} else {
				leftChild = (z << 2) + 2
				rightChild = (z << 2) + 3
			}

			// If left tile present, register it
			if _, present := classification[currentLevel+1][leftChild]; present {
				intersectingNextLevel = append(intersectingNextLevel, leftChild)
			}

			// Right tile needs to be present AND to the left of argument
			rightX, _ := morton.FromZ(rightChild)
			if _, present := classification[currentLevel+1][rightChild]; present && rightX <= xAtNextLevel {
				intersectingNextLevel = append(intersectingNextLevel, rightChild)
			}
		}
		intersectingCurrentLevel = intersectingNextLevel
	}
	return intersectingCurrentLevel
}

////////////////////////////
// Line tracing for lines //
////////////////////////////

// Line trace by looping over segments
func (ix *PointIndex) lineTraceLine(line geom.LineString, tmsID tms20.TMID, buffer uint) []tile.Tile {
	tileSet := make(map[tile.Tile]bool)
	l := LevelFromTmsId(tmsID)

	register := func(x, y uint, l Level) {
		tile := ix.makeTile(x, y, l, false)
		tileSet[tile] = true
	}

	segments, _ := line.AsSegments()
	for _, segment := range segments {
		ix.lineTrace(segment, l, ix.internalPixels, buffer, register)
	}

	return mapslicehelp.MapKeys(tileSet)
}

/////////////////////////////
// Line tracing for points //
/////////////////////////////

// Line tracing is not applicable, fall back to BBox
func (ix *PointIndex) lineTracePoint(_ geom.Point, tmsID tms20.TMID, buffer uint) []tile.Tile {
	l := LevelFromTmsId(tmsID)
	return ix.GetQBBoxWithBuffer(l, buffer)
}

//////////////////////
// Helper functions //
//////////////////////

func (ix *PointIndex) makeTileZ(z morton.Z, l Level, isContained bool) tile.Tile {
	x, y := morton.FromZ(z)
	return ix.makeTile(x, y, l, isContained)
}

func (ix *PointIndex) makeTile(x, y uint, l Level, isContained bool) tile.Tile {
	extent, _ := ix.getQuadrantExtentAndCentroid(l, x, y, ix.intExtent)
	return tile.Tile{
		Extent:      extent,
		X:           x,
		Y:           mathhelp.Pow2(l) - 1 - y,
		IsContained: isContained,
	}
}

func (ix *PointIndex) getResolution(level Level) intgeom.M {
	return ix.intExtent.XSpan() / int64(mathhelp.Pow2(level)) //nolint:gosec // G115
}

func (ix *PointIndex) InternalPixelLevelFromTmsID(deepestTIMID tms20.TMID) Level {
	levelDiff := uint(math.Log2(float64(ix.tilePixels))) + uint(math.Log2(float64(ix.internalPixels)))
	return uint(deepestTIMID) + levelDiff //nolint:gosec // G115
}

func (ix *PointIndex) TmsIDFromInternalPixelLevel(level Level) tms20.TMID {
	levelDiff := uint(math.Log2(float64(ix.tilePixels))) + uint(math.Log2(float64(ix.internalPixels)))
	return int(level) - int(levelDiff) //nolint:gosec // G115
}

func LevelFromTmsId(tms tms20.TMID) Level {
	return Level(tms) //nolint:gosec // G115 integers < 40
}

func (ix *PointIndex) findTile(p *intgeom.Point, l Level) (tileX, tileY uint) {
	levelDiff := ix.deepestLevel - l
	//nolint:gosec // G115
	deepestTileX := uint(p.X()-ix.intExtent.MinX()) / uint(ix.deepestRes)
	//nolint:gosec // G115
	deepestTileY := uint(p.Y()-ix.intExtent.MinY()) / uint(ix.deepestRes)
	tileX = deepestTileX >> levelDiff
	tileY = deepestTileY >> levelDiff
	return
}
