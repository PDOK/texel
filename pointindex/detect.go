package pointindex

import (
	"math"

	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mathhelp"
	"github.com/pdok/texel/morton"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"
)

type SegmentIdx struct {
	ringIdx  int
	pointIdx int
}

// RegisterFunc marks a tile at (xCoord, yCoord) (in tile coordinates at
// level l) as touched by the segment identified by segmentIdx.
type RegisterFunc func(xCoord, yCoord uint, l Level, segmentIdx SegmentIdx)

// Logic for searching tile grid for tiles that intersect a line.
// Detect line direction and trace that direction
func (ix *PointIndex) lineTrace(line geom.Line, tileLevel, intPixLevel Level, ringIdx int, pointIdx int, buffer uint, register RegisterFunc) {
	intLine := intgeom.FromGeomLine(line)
	idx := SegmentIdx{
		ringIdx:  ringIdx,
		pointIdx: pointIdx,
	}

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

	// Register tiles at start
	ix.tryRegisterTile(intLine, startX-dx, startY+dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY-dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX, startY-dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX+dx, startY-dy, tileLevel, bufferSize, register, idx)

	type coord struct{ x, y int }

	// Advance over an anti-diagonal, keeping track of the last two "hits"
	frontier := []coord{{startX, startY}}
	prevFrontier := make([]coord, 0)
	ix.tryRegisterTile(intLine, startX, startY, tileLevel, bufferSize, register, idx)

	for len(prevFrontier)+len(frontier) > 0 {
		nextSet := make(map[coord]bool, len(frontier)*2+len(prevFrontier))
		for _, cur := range frontier {
			nextSet[coord{cur.x + dx, cur.y}] = true
			nextSet[coord{cur.x, cur.y + dy}] = true
		}
		for _, prev := range prevFrontier {
			nextSet[coord{prev.x + dx, prev.y + dy}] = true
		}

		next := make([]coord, 0, len(nextSet))
		for c := range nextSet {
			if ix.tryRegisterTile(intLine, c.x, c.y, tileLevel, bufferSize, register, idx) {
				next = append(next, c)
			}
		}
		prevFrontier = frontier
		frontier = next
	}
}

func (ix *PointIndex) getResolution(level Level) intgeom.M {
	return ix.intExtent.XSpan() / int64(mathhelp.Pow2(level)) //nolint:gosec // G115
}

func (ix *PointIndex) getInternalPixelLevel(deepestTIMID tms20.TMID) Level {
	levelDiff := uint(math.Log2(float64(ix.tilePixels))) + uint(math.Log2(float64(ix.internalPixels)))
	return uint(deepestTIMID) + levelDiff //nolint:gosec // G115
}

// Logic for deciding whether line intersects tile with buffer.
func (ix *PointIndex) tryRegisterTile(line intgeom.Line, x, y int, l Level, bufferSize intgeom.M, register RegisterFunc, idx SegmentIdx) bool {
	maxCoord := int(mathhelp.Pow2(l)) - 1 //nolint:gosec // G115
	if x < 0 || y < 0 || x > maxCoord || y > maxCoord {
		return false // out of bounds: no tile there, nothing to register
	}
	ux, uy := uint(x), uint(y)
	extent, _ := ix.getQuadrantExtentAndCentroid(l, ux, uy, ix.intExtent)
	if tileIntersectsLine(line, extent, bufferSize) {
		register(ux, uy, l, idx)
		return true
	}
	return false
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

func tileIntersectsLine(line intgeom.Line, extent intgeom.Extent, buffer intgeom.M) bool {
	bufferedExtent := intgeom.Extent{
		extent.MinX() - buffer,
		extent.MinY() - buffer,
		extent.MaxX() + buffer,
		extent.MaxY() + buffer,
	}

	return lineIntersects(line, bufferedExtent)
}

type TileClassification int

const (
	ClassificationUnknown TileClassification = iota
	ClassificationIntersect
	ClassificationInside
	ClassificationOutside
)

// Create register logic, then loop over polygon edges
func (ix *PointIndex) registerPolygonEdges(polygon geom.Polygon, tmsID tms20.TMID, buffer uint) (segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification) {
	segments = make(map[morton.Z][]SegmentIdx)
	tileLevel := Level(tmsID) //nolint:gosec // G115
	intPixLevel := ix.getInternalPixelLevel(tmsID)
	classification = make(map[Level]map[morton.Z]TileClassification, tileLevel+1)
	for l := range tileLevel + 1 {
		classification[l] = make(map[morton.Z]TileClassification)
	}

	// Logic for keeping track of intersected tiles
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

	// Logic for keeping track which segment hits which tile
	register := func(x, y uint, l Level, segmentIdx SegmentIdx) {
		z := morton.MustToZ(x, y)
		segments[z] = append(segments[z], segmentIdx)
		markIntersected(l, z)
	}

	// Loop
	for ringIdx, ring := range polygon.LinearRings() {
		for pointIdx := range ring {
			line := geom.Line{ring[pointIdx], ring[(pointIdx+1)%len(ring)]}
			ix.lineTrace(line, tileLevel, intPixLevel, ringIdx, pointIdx, buffer, register)
		}
	}
	return segments, classification
}

// Given a tile, binary search for tiles left of it that have segments on them
// Note: tileLevel must be the actual tile level and x and y coordinates for that level
func (ix *PointIndex) findIntersectingTilesLeft(x, y, tileLevel Level, classification map[Level]map[morton.Z]TileClassification) []morton.Z {
	intersectingCurrentLevel := []morton.Z{0}
	var intersectingNextLevel []morton.Z
	var leftChild, rightChild morton.Z
	for currentLevel := range tileLevel {
		intersectingNextLevel = make([]morton.Z, 0)

		xAtNextLevel := x >> (tileLevel - currentLevel - 1)
		yAtNextLevel := y >> (tileLevel - currentLevel - 1)

		nextLevelDown := yAtNextLevel%2 == 0
		for _, z := range intersectingCurrentLevel {
			if nextLevelDown {
				leftChild = z << 2
				rightChild = (z << 2) + 1
			} else {
				leftChild = (z << 2) + 2
				rightChild = (z << 2) + 3
			}

			if _, present := classification[currentLevel+1][leftChild]; present {
				intersectingNextLevel = append(intersectingNextLevel, leftChild)
			}
			rightX, _ := morton.FromZ(rightChild)
			if _, present := classification[currentLevel+1][rightChild]; present && rightX <= xAtNextLevel {
				intersectingNextLevel = append(intersectingNextLevel, rightChild)
			}
		}
		intersectingCurrentLevel = intersectingNextLevel
	}
	return intersectingCurrentLevel
}

// Implements raycast to detect whether a non-intersecting tile lies on the inside or outside.
// tileLevel needs to be the tile level and z needs to be a morton coordinate for this level
func (ix *PointIndex) classifyNonIntersectingTile(z morton.Z, tileLevel Level, segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification, polygon geom.Polygon) TileClassification {
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

func getChildren(z morton.Z) [4]morton.Z {
	shift := z << 2
	return [4]morton.Z{shift, shift + 1, shift + 2, shift + 3}
}

// Classify all tiles on being inside or outside using quadtree. An "outside" or "inside"
// classification of a parent means children have the same class.
func (ix *PointIndex) classifyNonIntersectingTiles(targetLevel, currentLevel Level, currentZ morton.Z, containsAll bool, segments map[morton.Z][]SegmentIdx, classification map[Level]map[morton.Z]TileClassification, polygon geom.Polygon) {
	if targetLevel == currentLevel {
		return
	}

	children := getChildren(currentZ)
	intersectingChildren := make([]morton.Z, 0, 4)
	nextLevel := currentLevel + 1

	// Process unknown children
	for _, child := range children {
		if _, present := classification[nextLevel][child]; present {
			intersectingChildren = append(intersectingChildren, child)
			continue
		}
		if containsAll {
			classification[nextLevel][child] = ClassificationOutside
		} else {
			childAtTargetLevel := child << ((targetLevel - nextLevel) * 2)
			classification[nextLevel][child] = ix.classifyNonIntersectingTile(childAtTargetLevel, targetLevel, segments, classification, polygon)
		}
	}

	containsAll = containsAll && len(intersectingChildren) < 2

	// Recurse for intersecting children
	for _, child := range intersectingChildren {
		if _, present := classification[nextLevel][child]; present {
			ix.classifyNonIntersectingTiles(targetLevel, nextLevel, child, containsAll, segments, classification, polygon)
		}
	}
}

func (ix *PointIndex) ClassifyTiles(polygon geom.Polygon, tmsID tms20.TMID, buffer uint) map[Level]map[morton.Z]TileClassification {
	targetLevel := Level(tmsID) //nolint:gosec // G115
	segments, classification := ix.registerPolygonEdges(polygon, tmsID, buffer)
	ix.classifyNonIntersectingTiles(targetLevel, 0, 0, true, segments, classification, polygon)
	return classification
}

// Plumbing function
// Process output of ClassifyTiles
func (ix *PointIndex) GetLineTraceResult(polygon geom.Polygon, tmsID tms20.TMID, buffer uint) (tiles []tile.Tile) {
	classification := ix.ClassifyTiles(polygon, tmsID, buffer)
	tileLevel := Level(tmsID) //nolint:gosec // G115 integers < 40
	tiles = make([]tile.Tile, 0)

	for level, classificationAtLevel := range classification {
		for z, class := range classificationAtLevel {
			switch class {
			case ClassificationInside:
				size := mathhelp.Pow2((tileLevel - level) * 2)
				baseZ := z << (2 * (tileLevel - level))
				for i := range size {
					tiles = append(tiles, ix.makeTile(baseZ+i, tileLevel, true))
				}
			case ClassificationOutside:
				continue
			case ClassificationIntersect:
				if level == tileLevel {
					tiles = append(tiles, ix.makeTile(z, tileLevel, false))
				}
			case ClassificationUnknown:
				panic("ClassificationUnknown tile for polygon during linetrace")
			}
		}
	}
	return tiles
}

func (ix *PointIndex) makeTile(z morton.Z, l Level, isContained bool) tile.Tile {
	x, y := morton.FromZ(z)
	extent, _ := ix.getQuadrantExtentAndCentroid(l, x, y, ix.intExtent)
	return tile.Tile{
		Extent:      extent,
		X:           x,
		Y:           mathhelp.Pow2(l) - 1 - y,
		IsContained: isContained,
	}
}
