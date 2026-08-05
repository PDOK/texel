package pointindex

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
	"github.com/pdok/texel/mathhelp"
)

type SegmentIdx struct {
	ringIdx  int
	pointIdx int
}

// RegisterFunc marks a tile at (xCoord, yCoord) (in tile coordinates at
// level l) as touched by the segment identified by segmentIdx.
type RegisterFunc func(xCoord, yCoord uint, l Level, segmentIdx SegmentIdx)

func (ix *PointIndex) lineTrace(line geom.Line, tileLevel, intPixLevel Level, ringIdx int, pointIdx int, buffer uint, register RegisterFunc) {
	intLine := intgeom.FromGeomLine(line)
	idx := SegmentIdx{
		ringIdx: ringIdx,
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
	startX, startY := int(startTileX), int(startTileY)
	bufferSize := ix.getResolution(intPixLevel) * intgeom.M(buffer)

	// Register tiles otherwise missed
	ix.tryRegisterTile(intLine, startX-dx, startY+dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY-dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX, startY-dy, tileLevel, bufferSize, register, idx)
	ix.tryRegisterTile(intLine, startX+dx, startY-dy, tileLevel, bufferSize, register, idx)

	// Register tiles by only walking in direction dx and dy.
	type coord struct{ x, y int }

	frontier := []coord{{startX, startY}}
	ix.tryRegisterTile(intLine, startX, startY, tileLevel, bufferSize, register, idx)

	for len(frontier) > 0 {
		nextSet := make(map[coord]bool, len(frontier)*2)
		for _, cur := range frontier {
			nextSet[coord{cur.x + dx, cur.y}] = true
			nextSet[coord{cur.x, cur.y + dy}] = true
		}

		next := make([]coord, 0, len(nextSet))
		for c := range nextSet {
			if ix.tryRegisterTile(intLine, c.x, c.y, tileLevel, bufferSize, register, idx) {
				next = append(next, c)
			}
		}
		frontier = next
	}

	return
}

func (ix *PointIndex) getResolution(level Level) intgeom.M {
	return ix.intExtent.XSpan() / int64(mathhelp.Pow2(level))
}

// tryRegisterTile checks whether the (buffered) tile at (x, y) at level l
// intersects line, and if so registers it. Returns whether it was
// registered, so callers can use it to decide whether to keep expanding a
// walk in that direction. x, y may be out of the valid tile coordinate
// range (e.g. when called with a neighbor one step outside the grid); such
// out-of-bounds candidates are simply reported as not registered.
func (ix *PointIndex) tryRegisterTile(line intgeom.Line, x, y int, l Level, bufferSize intgeom.M, register RegisterFunc, idx SegmentIdx) bool {
	maxCoord := int(mathhelp.Pow2(l)) - 1
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

func extentIntersectsLine(line intgeom.Line, extent intgeom.Extent) bool {
	lMinX := min(line.Point1().X(), line.Point2().X())
	lMaxX := max(line.Point1().X(), line.Point2().X())
	lMinY := min(line.Point1().Y(), line.Point2().Y())
	lMaxY := max(line.Point1().Y(), line.Point2().Y())

	return extent.MaxX() >= lMinX && extent.MinX() <= lMaxX && extent.MaxY() >= lMinY && extent.MinY() <= lMaxY
}
