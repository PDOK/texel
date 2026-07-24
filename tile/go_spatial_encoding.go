package tile

// This is the geometry encoding logic copied from go-spatial/geom/encoding/mvt
// It was necessary to take this step, since usually EncodeGeometry is not an
// exported function. Modifications made:
// - Added `const debug = false` which is usually present in another file.
// - Addressed linter issues
// - Removed `context` parameter.

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-spatial/geom"
	vectorTile "github.com/go-spatial/geom/encoding/mvt/vector_tile"
	"github.com/go-spatial/geom/encoding/wkt"
	"github.com/go-spatial/geom/winding"
)

// Definition needed for compilation
const debug = false

// Rest of file is copied from feature.go

// TODO: Need to put in validation for the Geometry, as current the system
// does not check to make sure that the geometry is following the rules as
// laid out by the spec (i.e. polygons must not have the same start and end
// point).

// Feature describes a feature of a Layer. A layer will contain multiple features
// each of which has a geometry describing the interesting thing, and the metadata
// associated with it.

var (
	ErrNilFeature          = errors.New("feature is nil")
	ErrUnknownGeometryType = errors.New("unknown geometry type")
	ErrNilGeometryType     = errors.New("geometry is nil")
)

// These values came from: https://github.com/mapbox/vector-tile-spec/tree/master/2.1
const (
	cmdMoveTo    uint32 = 1
	cmdLineTo    uint32 = 2
	cmdClosePath uint32 = 7

	// maxCmdCount uint32 = 0x1FFFFFFF
)

type Command uint32

// NewCommand return a new command encoder
func NewCommand(cmd uint32, count int) Command {
	return Command((cmd & 0x7) | (uint32(count) << 3)) //nolint:gosec // G115 - This is potentially an issue if we ever get huge polygons
}

// ID encodes the ID of the command
func (c Command) ID() uint32 {
	return uint32(c) & 0x7
}

// Count encode the count of elements in the command
func (c Command) Count() int {
	return int(uint32(c) >> 3)
}

func (c Command) String() string {
	switch c.ID() {
	case cmdMoveTo:
		return fmt.Sprintf("move Command with count %v", c.Count())
	case cmdLineTo:
		return fmt.Sprintf("line To command with count %v", c.Count())
	case cmdClosePath:
		return fmt.Sprintf("close path command with count %v", c.Count())
	default:
		return fmt.Sprintf("unknown command (%v) with count %v", c.ID(), c.Count())
	}
}

// encodeZigZag does the ZigZag encoding for small ints.
func encodeZigZag(i int64) uint32 {
	return uint32((i << 1) ^ (i >> 31)) //nolint:gosec // G115 This can go wrong with polygons with enormous edges
}

// cursor reprsents the current position, this is needed to encode the geometry.
// the origin (0,0) is the top-left of the Tile.
type cursor struct {
	// The coordinates — these should be int64, when they were float64 they
	// introduced a slight drift in the coordinates.
	x int64
	y int64
}

// NewCursor creates a new cursor for drawing and MVT tile
func NewCursor() *cursor { //nolint:revive
	return &cursor{}
}

// GetDeltaPointAndUpdate returns the delta of for the given point from the current
// cursor position
func (c *cursor) GetDeltaPointAndUpdate(p geom.Point) (dx, dy int64) {
	delta := c.moveCursorPoints([2]int64{int64(p.X()), int64(p.Y())})
	return delta[0][0], delta[0][1]
}

// MoveTo encodes a move to command for the given points
func (c *cursor) MoveTo(points ...[2]float64) []uint32 {
	return c.encodeCmd(uint32(NewCommand(cmdMoveTo, len(points))), points)
}

// LineTo encodes a line to command for the given points
func (c *cursor) LineTo(points ...[2]float64) []uint32 {
	return c.encodeCmd(uint32(NewCommand(cmdLineTo, len(points))), points)
}

// ClosePath encodes a close path command
func (c *cursor) ClosePath() uint32 {
	return uint32(NewCommand(cmdClosePath, 1))
}

func (c *cursor) moveCursorPoints(pts ...[2]int64) (deltas [][2]int64) {
	deltas = make([][2]int64, len(pts))
	for i := range pts {
		deltas[i][0] = pts[i][0] - c.x
		deltas[i][1] = pts[i][1] - c.y
		c.x, c.y = pts[i][0], pts[i][1]
	}
	return deltas
}

func (c *cursor) encodeZigZagPt(pts [][2]int64) []uint32 {
	g := make([]uint32, 0, (2 * len(pts)))
	for _, dp := range pts {
		g = append(g, encodeZigZag(dp[0]), encodeZigZag(dp[1]))
	}
	return g
}

func (c *cursor) encodeCmd(cmd uint32, points [][2]float64) []uint32 {
	if len(points) == 0 {
		return []uint32{}
	}
	// new slice to hold our encode bytes. 2 bytes for each point pluse a command byte.
	g := make([]uint32, 0, (2*len(points))+1)
	// add the command integer
	g = append(g, cmd)

	// range through our points
	for _, p := range points {
		dx, dy := c.GetDeltaPointAndUpdate(geom.Point(p))
		// encode our delta point
		g = append(g, encodeZigZag(dx), encodeZigZag(dy))
	}

	return g
}

func (c *cursor) encodeLinearRing(order winding.Order, wo winding.Winding, ring [][2]float64) []uint32 {
	iring := make([][2]int64, len(ring))
	for i := range iring {
		// the process of truncating the float can cause the winding order to flip!
		iring[i][0], iring[i][1] = int64(ring[i][0]), int64(ring[i][1])
	}
	ringWinding := order.OfInt64Points(iring...)

	if ringWinding.IsColinear() {
		return []uint32{}
	}

	if ringWinding != wo {
		if debug {
			log.Printf("(0) RING WKT:\n%v", wkt.MustEncode(geom.LineString(ring)))
			log.Printf("(1) winding order: \n\tpts: %v\n\two : %v", ringWinding, wo)
		}
		// need to reverse the points in the ring
		for i := len(iring)/2 - 1; i >= 0; i-- {
			opp := len(iring) - 1 - i
			iring[i], iring[opp] = iring[opp], iring[i]
		}
		if debug {
			log.Printf("(2) RING WKT:\n%v", wkt.MustEncode(geom.LineString(ring)))
			log.Printf("(2) winding order: \n\tpts: %v\n\two : %v", ringWinding, wo)
		}
	}

	deltas := c.moveCursorPoints(iring...)

	// 3 is for the three commands that it takes to describe a ring: move to, line to, and close
	g := make([]uint32, 0, (2*len(iring))+3)

	// move to first point
	g = append(g,
		uint32(NewCommand(cmdMoveTo, 1)),
		encodeZigZag(deltas[0][0]),
		encodeZigZag(deltas[0][1]),
	)

	// line to each of the other points
	g = append(g, uint32(NewCommand(cmdLineTo, len(deltas)-1)))
	g = append(g, c.encodeZigZagPt(deltas[1:])...)

	// Close path
	g = append(g, uint32(NewCommand(cmdClosePath, 1)))

	return g
}

func (c *cursor) encodePolygon(geo geom.Polygon) []uint32 {
	g := []uint32{}

	lines := geo.LinearRings()
	for i := range lines {
		// bail if number of points is less than two
		if len(lines[i]) < 2 {
			if i != 0 {
				continue
			}
			return g
		}

		// https://github.com/mapbox/vector-tile-spec/tree/master/2.1#4344-polygon-geometry-type
		// An exterior ring is DEFINED as a linear ring having a positive area
		// as calculated by applying the surveyor's formula to the vertices of
		// the polygon in tile coordinates. In the tile coordinate system (with
		// the Y axis positive down and X axis positive to the right) this makes
		// the exterior ring's winding order appear clockwise.
		//
		// An interior ring is DEFINED as a linear ring having a negative area as
		// calculated by applying the surveyor's formula to the vertices of the
		// polygon in tile coordinates. In the tile coordinate system (with the
		// Y axis positive down and X axis positive to the right) this makes the
		// interior ring's winding order appear counterclockwise.
		order := winding.Order{YPositiveDown: true}
		wo := winding.CounterClockwise
		if i == 0 {
			wo = winding.Clockwise
		}
		g = append(g, c.encodeLinearRing(order, wo, lines[i])...)
	}
	return g
}

// EncodeGeometry will take a geom.Geometry and encode it according to the
// mapbox vector_tile spec.
func EncodeGeometry(geometry geom.Geometry) (g []uint32, vtyp vectorTile.Tile_GeomType, err error) {
	if geometry == nil {
		return nil, vectorTile.Tile_UNKNOWN, ErrNilGeometryType
	}

	c := NewCursor()

	switch t := geometry.(type) {
	case geom.Point:
		g = append(g, c.MoveTo(t)...)
		return g, vectorTile.Tile_POINT, nil

	case geom.MultiPoint:
		g = append(g, c.MoveTo(t.Points()...)...)
		return g, vectorTile.Tile_POINT, nil

	case geom.LineString:
		points := t.Vertices()
		g = append(g, c.MoveTo(points[0])...)
		g = append(g, c.LineTo(points[1:]...)...)
		return g, vectorTile.Tile_LINESTRING, nil

	case geom.MultiLineString:
		lines := t.LineStrings()
		for _, l := range lines {
			points := geom.LineString(l).Vertices()
			g = append(g, c.MoveTo(points[0])...)
			g = append(g, c.LineTo(points[1:]...)...)
		}
		return g, vectorTile.Tile_LINESTRING, nil

	case geom.Polygon:
		g = append(g, c.encodePolygon(t)...)
		return g, vectorTile.Tile_POLYGON, nil

	case geom.MultiPolygon:
		polygons := t.Polygons()
		for _, p := range polygons {
			g = append(g, c.encodePolygon(p)...)
		}
		return g, vectorTile.Tile_POLYGON, nil

	case *geom.MultiPolygon:
		if t == nil {
			return g, vectorTile.Tile_POLYGON, nil
		}

		polygons := t.Polygons()
		for _, p := range polygons {
			g = append(g, c.encodePolygon(p)...)
		}
		return g, vectorTile.Tile_POLYGON, nil

	default:
		return nil, vectorTile.Tile_UNKNOWN, ErrUnknownGeometryType
	}
}
