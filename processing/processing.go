package processing

// Orchestrating logic around processing the snap command.

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/pdok/texel/geomhelp"
	"github.com/pdok/texel/pointindex"
	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
)

type Config struct {
	KeepPointsAndLines  bool
	IgnoreOutsideGrid   bool
	ReverseWindingOrder bool
	EncodeTiles         bool
	// Buffer is the number of internal pixels that tiles get
	// inflated for detecting which geometries lie on them
	Buffer uint
	// Decide whether to use lineTrace or BBox for tile detection
	UseLineTrace bool
}

//////////////////////////
// Channel manipulation //
//////////////////////////

// Entry point for processing. Initialize channels and create processor.
// Then kickstart processing.
// Closing channels is the responsibility of functions supplying them.
func ProcessFeatures(source Source, targets map[tms20.TMID]Target, f SnapFunc, newIndex func() *pointindex.PointIndex, config Config) {
	featuresBefore := make(chan Feature)
	featuresAfter := make(chan FeatureForTileMatrix)
	tileMatrixIDs := make([]tms20.TMID, 0, len(targets))
	for tmID := range targets {
		tileMatrixIDs = append(tileMatrixIDs, tmID)
	}

	//  Initialize geometry processor
	processor := NewGeometryProcessor(tileMatrixIDs, config, f, newIndex)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeFeaturesToTargets(featuresAfter, targets)
	}()
	go processFeatures(featuresBefore, featuresAfter, processor)
	go source.ReadFeatures(featuresBefore)

	wg.Wait()
}

// writeFeatures collects the processed features by the processFeatures and
// creates a WKB binary from the geometry
// The collected feature array, based on the pagesize, is then passed to the writeFeaturesArray
func writeFeaturesToTargets(featuresForTileMatrices <-chan FeatureForTileMatrix, targets map[int]Target) {
	targetChannels := make(map[int]chan<- FeatureForTileMatrix)
	wg := sync.WaitGroup{}

	// create a channel and start a goroutine per tile matrix target
	for tmID, target := range targets {
		targetChannel := make(chan FeatureForTileMatrix)
		targetChannels[tmID] = targetChannel
		wg.Add(1)
		go func(target Target) {
			defer wg.Done()
			target.WriteFeatures(targetChannel)
		}(target)
	}

	// distribute the incoming features over the targets
	for {
		feature, ok := <-featuresForTileMatrices
		if !ok {
			break
		}
		tmID := feature.TileMatrixID()
		channel := targetChannels[tmID]
		if channel == nil { // should never happen
			panic(fmt.Errorf(`no target channel for %v`, tmID))
		}
		channel <- feature
	}

	// close the channels, the targets will do their last writing
	for _, targetChannel := range targetChannels {
		close(targetChannel)
	}

	wg.Wait()
}

// Read inputs from channel and call the processor on them. Results wired to output channel.
// Also keep track of statistics.
func processFeatures(featuresIn <-chan Feature, featuresOut chan<- FeatureForTileMatrix, processor *GeometryProcessor) {
	stats := initStats()
	for {
		feature, hasMore := <-featuresIn
		if !hasMore {
			break
		}
		stats.preCount++
		geometry := feature.Geometry()
		stats.countGeometry(geometry)
		newGeometriesPerTileMatrix := processor.Process(geometry)

		if len(newGeometriesPerTileMatrix) > 0 {
			stats.postCount++
		}
		for tmID, snapResult := range newGeometriesPerTileMatrix {
			encGeoms := processor.encode(snapResult)
			featuresOut <- wrapFeatureForTileMatrix(feature, tmID, snapResult.Geometry, encGeoms)
		}

	}
	close(featuresOut)

	log.Printf("    total features: %d", stats.preCount)
	log.Printf("      non-polygons: %d", stats.nonPolygonCount)
	if stats.preCount != stats.nonPolygonCount {
		log.Printf("     multipolygons: %d", stats.multiPolygonCount)
	}
	log.Printf("              kept: %d", stats.postCount)
}

// Statistics //

type countStats struct {
	preCount          uint64
	postCount         uint64
	nonPolygonCount   uint64
	multiPolygonCount uint64
}

func initStats() countStats {
	return countStats{0, 0, 0, 0}
}

func (stats *countStats) countGeometry(g geom.Geometry) {
	switch g.(type) {
	case geom.MultiPolygon:
		stats.nonPolygonCount++
	case geom.Polygon:
	default:
		stats.nonPolygonCount++
	}
}

/////////////////////////////////
// Geometry Processor Creation //
/////////////////////////////////

// Abstract data needed for processing.
type GeometryProcessor struct {
	tmIDs       []tms20.TMID
	newIndex    IndexFactory
	snap        func(ix *pointindex.PointIndex, g geom.Geometry) map[tms20.TMID][]geom.Geometry
	detectTiles tileDetector
	encode      func(SnapResult) []tile.EncodedGeometry
}

// Abstract tile detector
type tileDetector func(ix *pointindex.PointIndex, tmsID tms20.TMID, newGeometries []geom.Geometry) []tile.Tile

// Abstract pointindex creator. Is implied to insert geometry in index
type IndexFactory func(geometry geom.Geometry) (*pointindex.PointIndex, error)

type SnapFunc func(ix *pointindex.PointIndex, g geom.Geometry, tmsIDs []tms20.TMID, config Config) map[tms20.TMID][]geom.Geometry

// Initialize GeometryProcessor
// Essentially turns configuration into functionality
func NewGeometryProcessor(tmIDs []tms20.TMID, config Config, f SnapFunc, newIndex func() *pointindex.PointIndex) *GeometryProcessor {
	return &GeometryProcessor{
		tmIDs:    tmIDs,
		newIndex: newIndexFactory(newIndex, config.IgnoreOutsideGrid),
		snap: func(ix *pointindex.PointIndex, g geom.Geometry) map[tms20.TMID][]geom.Geometry {
			return f(ix, g, tmIDs, config)
		},
		detectTiles: newTileDetector(config),
		encode:      newEncoder(config),
	}
}

// Create index and inserts geometry.
// Depending on configuration, will panic or skip when polygon is outside bounds
func newIndexFactory(newIndex func() *pointindex.PointIndex, ignoreOutsideGrid bool) IndexFactory {
	return func(geometry geom.Geometry) (*pointindex.PointIndex, error) {
		ix := newIndex()
		err := ix.InsertGeometry(geometry)
		if err == nil {
			return ix, nil
		}
		outsideGridErr := new(pointindex.OutsideGridError)
		if errors.As(err, outsideGridErr) && ignoreOutsideGrid {
			log.Println("[WARNING] skipping geometry because: " + err.Error())
			return nil, nil
		}
		return nil, err
	}
}

// Create tile detection function based on configuration
func newTileDetector(config Config) tileDetector {
	// No encoding: do not detect tiles
	if !config.EncodeTiles {
		return func(_ *pointindex.PointIndex, _ tms20.TMID, _ []geom.Geometry) []tile.Tile { return nil }
	}

	// Line tracing: line trace each geometry, append results
	if config.UseLineTrace {
		return func(ix *pointindex.PointIndex, tmsID tms20.TMID, newGeometries []geom.Geometry) []tile.Tile {
			tiles := make([]tile.Tile, 0, len(newGeometries))
			for _, newGeometry := range newGeometries {
				tiles = append(tiles, ix.DetectTilesViaLineTrace(newGeometry, tmsID, config.Buffer)...)
			}
			return tiles
		}
	}

	// No line tracing: bbox detection
	return func(ix *pointindex.PointIndex, tmsID tms20.TMID, _ []geom.Geometry) []tile.Tile {
		return ix.GetQBBoxWithBuffer(pointindex.LevelFromTmsId(tmsID), config.Buffer)
	}
}

// Build encoding function based on geometry.
// This involves checking whether encoding is enabled at all,
// and creating the default tile for tile-filling polygons
func newEncoder(config Config) func(SnapResult) []tile.EncodedGeometry {
	if !config.EncodeTiles {
		return func(SnapResult) []tile.EncodedGeometry { return nil }
	}

	// Create tile-filling geometry (reused across polygons)
	defaultEnc, err := tile.NewDefaultEncoding(config.Buffer)
	if err != nil {
		panic(err)
	}

	return func(s SnapResult) []tile.EncodedGeometry {
		encGeoms := make([]tile.EncodedGeometry, len(s.Tiles))
		orig := s.Geometry
		for i, q := range s.Tiles {
			encGeoms[i] = tile.MvtEncodeGeometry(q, orig, defaultEnc)
		}
		return encGeoms
	}
}

/////////////////////////
// Geometry processing //
/////////////////////////

// Process a geometry. Distinguish between single- and multigeometries.
// ProcessSingleGeometry does the actual processing.
func (p *GeometryProcessor) Process(geometry geom.Geometry) map[tms20.TMID]SnapResult {
	switch geometry := geometry.(type) {
	case geom.Polygon, geom.LineString, geom.Point:
		return p.ProcessSingle(geometry)
	case geom.MultiPolygon, geom.MultiLineString, geom.MultiPoint:
		return p.ProcessMulti(geomhelp.MultiGeometryToSlice(geometry))
	default:
		newGeometriesPerTileMatrix := make(map[tms20.TMID]SnapResult, len(p.tmIDs))
		for _, tmID := range p.tmIDs {
			newGeometriesPerTileMatrix[tmID] = SnapResult{nil, nil}
		}
		return newGeometriesPerTileMatrix
	}
}

// Process a single geometry (no multigeometries).
func (p *GeometryProcessor) ProcessSingle(geometry geom.Geometry) map[tms20.TMID]SnapResult {
	ix, err := p.newIndex(geometry)
	// Unknown error
	if err != nil {
		panic(err)
	}
	// Polygon should be skipped
	if ix == nil {
		return make(map[tms20.TMID]SnapResult)
	}

	newGeometriesPerTileMatrix := p.snap(ix, geometry)

	// Create tiles and merge geometries
	geomsAndTilesPerTileMatrix := make(map[tms20.TMID]SnapResult, len(p.tmIDs))
	for _, tmsID := range p.tmIDs {
		newGeometries := newGeometriesPerTileMatrix[tmsID]
		tiles := p.detectTiles(ix, tmsID, newGeometries)
		singleGeometry := geomhelp.GeometrySliceToGeom(newGeometries)
		geomsAndTilesPerTileMatrix[tmsID] = SnapResult{singleGeometry, tiles}
	}
	return geomsAndTilesPerTileMatrix
}

// Process a multigeometry by processing each individual geometry and merging the results
func (p *GeometryProcessor) ProcessMulti(multiGeometry []geom.Geometry) map[tms20.TMID]SnapResult {
	newMultiGeometryPerTileMatrix := make(map[tms20.TMID]SnapResult, len(p.tmIDs))
	for _, geometry := range multiGeometry {
		snapResultPerTileMatrix := p.ProcessSingle(geometry)
		for tmID, snapResult := range snapResultPerTileMatrix {
			currentResult := newMultiGeometryPerTileMatrix[tmID]
			currentResult.Tiles = append(currentResult.Tiles, snapResult.Tiles...)
			currentResult.Geometry = geomhelp.MergeGeometries(currentResult.Geometry, snapResult.Geometry)
			newMultiGeometryPerTileMatrix[tmID] = currentResult
		}
	}
	return newMultiGeometryPerTileMatrix
}

type featureForTileMatrixWrapper struct {
	wrapped      Feature
	newGeometry  geom.Geometry
	encodedGeoms []tile.EncodedGeometry
	tileMatrixID int
}

func (f *featureForTileMatrixWrapper) Columns() []any {
	return f.wrapped.Columns()
}

func (f *featureForTileMatrixWrapper) Geometry() geom.Geometry {
	if f.newGeometry == nil {
		return f.wrapped.Geometry()
	}
	return f.newGeometry
}

func (f *featureForTileMatrixWrapper) TileMatrixID() int {
	return f.tileMatrixID
}

func (f *featureForTileMatrixWrapper) EncodedGeoms() []tile.EncodedGeometry {
	return f.encodedGeoms
}

func wrapFeatureForTileMatrix(feature Feature, tileMatrixID int, newGeometry geom.Geometry, encGeoms []tile.EncodedGeometry) FeatureForTileMatrix {
	return &featureForTileMatrixWrapper{
		wrapped:      feature,
		newGeometry:  newGeometry,
		tileMatrixID: tileMatrixID,
		encodedGeoms: encGeoms,
	}
}
