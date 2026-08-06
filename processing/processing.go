package processing

// Orchestrating logic around processing the snap command.

import (
	"fmt"
	"log"
	"sync"

	"github.com/pdok/texel/tile"
	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
)

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

func encodeGeometry(s SnapResult) []tile.EncodedGeometry {
	encGeoms := make([]tile.EncodedGeometry, len(s.Tiles))
	orig := s.Geometry

	for i, q := range s.Tiles {
		encGeoms[i] = tile.MvtEncodeGeometry(q, orig)
	}

	return encGeoms
}

// processFeatures processes the geometries in the features with the given function
func processFeatures(featuresIn <-chan Feature, featuresOut chan<- FeatureForTileMatrix, tmIDs []tms20.TMID, f processPolygonFunc, encodeTiles bool) {
	stats := initStats()
	for {
		feature, hasMore := <-featuresIn
		if !hasMore {
			break
		}
		stats.preCount++
		geometry := feature.Geometry()
		stats.countGeometry(geometry)
		newGeometriesPerTileMatrix := processGeometry(feature.Geometry(), tmIDs, f)

		if len(newGeometriesPerTileMatrix) > 0 {
			stats.postCount++
		}
		for tmID, snapResult := range newGeometriesPerTileMatrix {
			var encGeoms []tile.EncodedGeometry
			if encodeTiles {
				encGeoms = encodeGeometry(snapResult)
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
func ProcessFeatures(source Source, targets map[tms20.TMID]Target, f processPolygonFunc, encodeTiles bool) {
	featuresBefore := make(chan Feature)
	featuresAfter := make(chan FeatureForTileMatrix)
	tileMatrixIDs := make([]tms20.TMID, 0, len(targets))
	for tmID := range targets {
		tileMatrixIDs = append(tileMatrixIDs, tmID)
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeFeaturesToTargets(featuresAfter, targets)
	}()
	go processFeatures(featuresBefore, featuresAfter, tileMatrixIDs, f, encodeTiles)
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
