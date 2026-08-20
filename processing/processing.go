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

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeaturesFromSource(source Source, features chan<- Feature) {
	source.ReadFeatures(features)
}

func mergeGeometries(g, h geom.Geometry) geom.Geometry {
	if g == nil {
		return h
	}
	if h == nil {
		return g
	}

	switch g := g.(type) {
	case geom.Polygon:
		switch h := h.(type) {
		case geom.Polygon:
			return geom.MultiPolygon{g, h}
		case geom.MultiPolygon:
			return append(h, g)
		default:
			panic("Trying to merge a non-polygon geometry.")

		}
	case geom.MultiPolygon:
		switch h := h.(type) {
		case geom.Polygon:
			return append(g, h)
		case geom.MultiPolygon:
			return append(g, h.Polygons()...)
		default:
			panic("Trying to merge a non-polygon geometry.")
		}

	default:
		panic("Trying to merge a non-polygon geometry.")
	}
}

func ProcessMultiGeometry(multiGeometry []geom.Geometry, tmIDs []tms20.TMID, config Config, f SnapFunc, factory IndexFactory) map[tms20.TMID]SnapResult {
	newMultiGeometryPerTileMatrix := make(map[tms20.TMID]SnapResult, len(tmIDs))
	for _, geometry := range multiGeometry {
		snapResultPerTileMatrix := ProcessSingleGeometry(geometry, tmIDs, config, f, factory)
		for tmID, snapResult := range snapResultPerTileMatrix {
			currentResult := newMultiGeometryPerTileMatrix[tmID]
			currentResult.Tiles = append(currentResult.Tiles, snapResult.Tiles...)
			currentResult.Geometry = mergeGeometries(currentResult.Geometry, snapResult.Geometry)
			newMultiGeometryPerTileMatrix[tmID] = currentResult
		}
	}
	return newMultiGeometryPerTileMatrix
}

// processMultiPolygon will split itself into the separated polygons that will be processed before building a new MULTIPOLYGON
func processMultiPolygon(multiPolygon geom.MultiPolygon, tileMatrixIDs []tms20.TMID, f processPolygonFunc) map[tms20.TMID]SnapResult {
	newMultiPolygonPerTileMatrix := make(map[tms20.TMID]SnapResult, len(tileMatrixIDs))
	for _, polygon := range multiPolygon {
		snapResultsPerTileMatrix := f(polygon, tileMatrixIDs)
		for tmID, snapResult := range snapResultsPerTileMatrix {
			currentResult := newMultiPolygonPerTileMatrix[tmID]
			currentResult.Tiles = append(currentResult.Tiles, snapResult.Tiles...)
			currentResult.Geometry = mergeGeometries(currentResult.Geometry, snapResult.Geometry)
			newMultiPolygonPerTileMatrix[tmID] = currentResult

		}
	}
	return newMultiPolygonPerTileMatrix
}

type SnapFunc func(ix *pointindex.PointIndex, g geom.Geometry, tmsIDs []tms20.TMID, config Config) map[tms20.TMID][]geom.Geometry

type IndexFactory func() *pointindex.PointIndex

func ProcessSingleGeometry(geometry geom.Geometry, tmIDs []tms20.TMID, config Config, f SnapFunc, factory IndexFactory) map[tms20.TMID]SnapResult {
	ix := factory()
	err := ix.InsertGeometry(geometry)
	if err != nil {
		// TODO This can be a helper
		outsideGridErr := new(pointindex.OutsideGridError)
		if errors.As(err, outsideGridErr) && config.IgnoreOutsideGrid {
			log.Println("[WARNING] skipping polygon because: " + err.Error())
			return make(map[tms20.TMID]SnapResult)
		}
		panic(err)
	}

	newGeometriesPerTileMatrix := f(ix, geometry, tmIDs, config)

	geomsAndTilesPerTileMatrix := make(map[tms20.TMID]SnapResult, len(tmIDs))
	for _, tmsID := range tmIDs {
		newGeometries := newGeometriesPerTileMatrix[tmsID]
		var tiles []tile.Tile
		if config.EncodeTiles && config.UseLineTrace {
			for _, newGeometry := range newGeometries {
				tiles = append(tiles, ix.DetectTilesViaLineTrace(newGeometry, tmsID, config.Buffer)...)
			}
		} else if config.EncodeTiles {
			tiles = ix.GetQBBoxWithBuffer(pointindex.LevelFromTmsId(tmsID), config.Buffer)
		}

		singleGeometry := geomhelp.GeometrySliceToGeom(newGeometries)
		geomsAndTilesPerTileMatrix[tmsID] = SnapResult{singleGeometry, tiles}
	}
	return geomsAndTilesPerTileMatrix
}

func multiGeometryToSlice(multiGeom geom.Geometry) []geom.Geometry {
	switch geometry := multiGeom.(type) {
	case geom.MultiPolygon:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, polygon := range geometry {
			geoms = append(geoms, geom.Polygon(polygon))
		}
		return geoms
	case geom.MultiLineString:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, line := range geometry {
			geoms = append(geoms, geom.LineString(line))
		}
		return geoms
	case geom.MultiPoint:
		geoms := make([]geom.Geometry, 0, len(geometry))
		for _, point := range geometry {
			geoms = append(geoms, geom.Point(point))
		}
		return geoms
	default:
		panic("Cannot convert to slic: not a multigeometry.")
	}
}

func newProcessGeometry(geometry geom.Geometry, tmIDs []tms20.TMID, config Config, f SnapFunc, factory IndexFactory) map[tms20.TMID]SnapResult {
	switch geometry := geometry.(type) {
	case geom.Polygon:
		return ProcessSingleGeometry(geometry, tmIDs, config, f, factory)
	case geom.LineString:
		return ProcessSingleGeometry(geometry, tmIDs, config, f, factory)
	case geom.Point:
		return ProcessSingleGeometry(geometry, tmIDs, config, f, factory)
	case geom.MultiPolygon:
		geomSlice := multiGeometryToSlice(geometry)
		return ProcessMultiGeometry(geomSlice, tmIDs, config, f, factory)
	case geom.MultiLineString:
		geomSlice := multiGeometryToSlice(geometry)
		return ProcessMultiGeometry(geomSlice, tmIDs, config, f, factory)
	case geom.MultiPoint:
		geomSlice := multiGeometryToSlice(geometry)
		return ProcessMultiGeometry(geomSlice, tmIDs, config, f, factory)
	default:
		newGeometriesPerTileMatrix := make(map[tms20.TMID]SnapResult, len(tmIDs))
		for _, tmID := range tmIDs {
			newGeometriesPerTileMatrix[tmID] = SnapResult{nil, nil}
		}
		return newGeometriesPerTileMatrix
	}
}

func processGeometry(geometry geom.Geometry, tmIDs []tms20.TMID, f processPolygonFunc) map[tms20.TMID]SnapResult {
	newGeometriesPerTileMatrix := make(map[tms20.TMID]SnapResult, len(tmIDs))

	switch geometry := geometry.(type) {
	case geom.Polygon:
		newGeometriesPerTileMatrix = f(geometry, tmIDs)
	case geom.MultiPolygon:
		newGeometriesPerTileMatrix = processMultiPolygon(geometry, tmIDs, f)
	default:
		for _, tmID := range tmIDs {
			newGeometriesPerTileMatrix[tmID] = SnapResult{nil, nil}
		}
	}

	return newGeometriesPerTileMatrix
}

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

func encodeGeometry(s SnapResult, defaultEnc tile.DefaultEncoding) []tile.EncodedGeometry {
	encGeoms := make([]tile.EncodedGeometry, len(s.Tiles))
	orig := s.Geometry

	for i, q := range s.Tiles {
		encGeoms[i] = tile.MvtEncodeGeometry(q, orig, defaultEnc)
	}

	return encGeoms
}

// processFeatures processes the geometries in the features with the given function
func processFeatures(featuresIn <-chan Feature, featuresOut chan<- FeatureForTileMatrix, tmIDs []tms20.TMID, f SnapFunc, factory IndexFactory, config Config, defaultEnc tile.DefaultEncoding) {
	stats := initStats()
	for {
		feature, hasMore := <-featuresIn
		if !hasMore {
			break
		}
		stats.preCount++
		geometry := feature.Geometry()
		stats.countGeometry(geometry)
		newGeometriesPerTileMatrix := newProcessGeometry(geometry, tmIDs, config, f, factory)

		if len(newGeometriesPerTileMatrix) > 0 {
			stats.postCount++
		}
		for tmID, snapResult := range newGeometriesPerTileMatrix {
			var encGeoms []tile.EncodedGeometry
			if config.EncodeTiles {
				encGeoms = encodeGeometry(snapResult, defaultEnc)
			}
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

type processPolygonFunc func(p geom.Polygon, tileMatrixIDs []tms20.TMID) map[tms20.TMID]SnapResult

// ProcessFeatures applies the processing function/operation to each Target.
func ProcessFeatures(source Source, targets map[tms20.TMID]Target, f SnapFunc, factory IndexFactory, config Config) {
	featuresBefore := make(chan Feature)
	featuresAfter := make(chan FeatureForTileMatrix)
	tileMatrixIDs := make([]tms20.TMID, 0, len(targets))
	for tmID := range targets {
		tileMatrixIDs = append(tileMatrixIDs, tmID)
	}

	// When tile is contained in polygon, use default geometry (tile-filling square).
	var defaultEnc tile.DefaultEncoding
	if config.EncodeTiles {
		var err error
		defaultEnc, err = tile.NewDefaultEncoding(config.Buffer)
		if err != nil {
			panic(err)
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeFeaturesToTargets(featuresAfter, targets)
	}()
	go processFeatures(featuresBefore, featuresAfter, tileMatrixIDs, f, factory, config, defaultEnc)
	go readFeaturesFromSource(source, featuresBefore)

	wg.Wait()
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
