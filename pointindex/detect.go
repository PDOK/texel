package pointindex

import (
	"github.com/go-spatial/geom"
	"github.com/pdok/texel/intgeom"
)

type SegmentIdx struct {
	ringIdx  int
	pointIdx int
}

// RegisterFunc marks a tile at (xCoord, yCoord) (in tile coordinates at
// level l) as touched by the segment identified by segmentIdx.
type RegisterFunc func(xCoord, yCoord uint, l Level, segmentIdx SegmentIdx)

func (ix *PointIndex) lineTrace(line geom.Line, l Level, ringIdx int, pointIdx int, buffer intgeom.M, register RegisterFunc) {
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

	startX, startY := ix.findTile(intLine.Point1(), l)
	
	// Register tiles otherwise missed
	ix.tryRegisterTile(intLine, startX-dx, startY+dy, l, buffer, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY, l, buffer, register, idx)
	ix.tryRegisterTile(intLine, startX-dx, startY-dy, l, buffer, register, idx)
	ix.tryRegisterTile(intLine, startX, startY-dy, l, buffer, register, idx)
	ix.tryRegisterTile(intLine, startX+dx, startY-dy, l, buffer, register, idx)

	// Register tiles by only walking in direction dx and dy.
	for true {
		// Register tile until no more can be found.
	}

	return
}

func (ix *PointIndex) tryRegisterTile(line intgeom.Line, x, y uint, l Level, buffer intgeom.M, register RegisterFunc, idx SegmentIdx){
	extent, _ := ix.getQuadrantExtentAndCentroid(l, x, y, ix.intExtent)
	if tileIntersectsLine(line, extent, buffer){
		register(x, y, l, idx)
	}
}

func (ix *PointIndex) findTile(p *intgeom.Point, l Level) (tileX, tileY uint) {
	levelDiff := ix.deepestLevel - l
	tileX = uint(p.X()) >> levelDiff
	tileY = uint(p.Y()) >> levelDiff
	return
}

func tileIntersectsLine(line intgeom.Line, extent intgeom.Extent, buffer intgeom.M) bool {
	bufferedExtent := intgeom.Extent{
		extent.MinX() - buffer,
		extent.MinY() - buffer,
		extent.MaxX() + buffer,
		extent.MaxY() + buffer,
	}

	return extentIntersectsLine(line, bufferedExtent)
}

func extentIntersectsLine(line intgeom.Line, extent intgeom.Extent) bool {
	lMinX := min(line.Point1().X(), line.Point2().X())
	lMaxX := max(line.Point1().X(), line.Point2().X())
	lMinY := min(line.Point1().Y(), line.Point2().Y())
	lMaxY := max(line.Point1().Y(), line.Point2().Y())

	return extent.MaxX() >= lMaxX && extent.MinX() <= lMinX && extent.MaxY() >= lMaxY && extent.MinY() <= lMinY
}
