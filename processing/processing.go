// Package processing takes care of the logistics around reading and writing to a Target.
// Not the processing operation(s) itself.
package processing

import (
	"fmt"
	"log"
	"sync"

	"github.com/pdok/texel/tms20"

	"github.com/go-spatial/geom"
)

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeaturesFromSource(source Source, features chan<- Feature) {
	source.ReadFeatures(features)
}

// processFeatures processes the geometries in the features with the given function
func processFeatures(featuresIn <-chan Feature, featuresOut chan<- FeatureForTileMatrix, tmIDs []tms20.TMID, f processPolygonFunc) {
	var preCount, postCount, nonPolygonCount, multiPolygonCount uint64
	for {
		feature, hasMore := <-featuresIn
		if !hasMore {
			break
		}
		preCount++
		switch feature.Geometry().(type) {
		case geom.Polygon:
			polygon := feature.Geometry().(geom.Polygon)
			newPolygonsPerTileMatrix := f(polygon, tmIDs)
			if len(newPolygonsPerTileMatrix) > 0 {
				postCount++
			}
			for tmID, newPolygons := range newPolygonsPerTileMatrix {
				var newGeometry geom.Geometry
				if len(newPolygons) == 0 { // should never happen
					panic(fmt.Errorf("no new polygon for level %v", tmID))
				}
				if len(newPolygons) == 1 {
					newGeometry = newPolygons[0]
				} else {
					// TODO polygons are combined into multipolygons, for now here
					// later, processPolygonFunc could return abstract geometry(s) if also lines/points are returned
					newGeometry = polygonsToMulti(newPolygons)
				}
				featuresOut <- wrapFeatureForTileMatrix(feature, tmID, newGeometry)
			}
		case geom.MultiPolygon:
			multiPolygon := feature.Geometry().(geom.MultiPolygon)
			newMultiPolygonPerTileMatrix := processMultiPolygon(multiPolygon, tmIDs, f)
			if len(newMultiPolygonPerTileMatrix) > 0 {
				postCount++
			}
			for tmID, newMultiPolygon := range newMultiPolygonPerTileMatrix {
				featuresOut <- wrapFeatureForTileMatrix(feature, tmID, newMultiPolygon)
			}
		default:
			postCount++
			nonPolygonCount++
			for _, tmID := range tmIDs {
				featuresOut <- wrapFeatureForTileMatrix(feature, tmID, nil)
			}
		}
	}
	close(featuresOut)

	log.Printf("    total features: %d", preCount)
	log.Printf("      non-polygons: %d", nonPolygonCount)
	if preCount != nonPolygonCount {
		log.Printf("     multipolygons: %d", multiPolygonCount)
	}
	log.Printf("              kept: %d", postCount)
}

// writeFeatures collects the processed features by the processFeatures and
// creates a WKB binary from the geometry
// The collected feature array, based on the pagesize, is then passed to the writeFeaturesArray
func writeFeaturesToTargets(featuresForTileMatrices <-chan FeatureForTileMatrix, targets map[int]Target) {
	targetChannels := make(map[int]chan<- Feature)
	wg := sync.WaitGroup{}

	// create a channel and start a goroutine per tile matrix target
	for tmID, target := range targets {
		targetChannel := make(chan Feature)
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

// processMultiPolygon will split itself into the separated polygons that will be processed before building a new MULTIPOLYGON
func processMultiPolygon(multiPolygon geom.MultiPolygon, tileMatrixIDs []tms20.TMID, f processPolygonFunc) map[tms20.TMID]geom.MultiPolygon {
	newMultiPolygonPerTileMatrix := make(map[tms20.TMID]geom.MultiPolygon, len(tileMatrixIDs))
	for _, polygon := range multiPolygon {
		newPolygonsPerTileMatrix := f(polygon, tileMatrixIDs)
		for tmID, newPolygons := range newPolygonsPerTileMatrix {
			for _, newPolygon := range newPolygons {
				// if the processing results in multiple polygons, they are just added to the single resulting multipoly
				newMultiPolygonPerTileMatrix[tmID] = append(newMultiPolygonPerTileMatrix[tmID], newPolygon)
			}
		}
	}
	return newMultiPolygonPerTileMatrix
}

type processPolygonFunc func(p geom.Polygon, tileMatrixIDs []tms20.TMID) map[tms20.TMID][]geom.Polygon

// ProcessFeatures applies the processing function/operation to each Target.
func ProcessFeatures(source Source, targets map[tms20.TMID]Target, f processPolygonFunc) {
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
	go processFeatures(featuresBefore, featuresAfter, tileMatrixIDs, f)
	go readFeaturesFromSource(source, featuresBefore)

	wg.Wait()
}

type featureForTileMatrixWrapper struct {
	wrapped      Feature
	newGeometry  geom.Geometry
	tileMatrixID int
}

func (f *featureForTileMatrixWrapper) Columns() []interface{} {
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

func wrapFeatureForTileMatrix(feature Feature, tileMatrixID int, newGeometry geom.Geometry) FeatureForTileMatrix {
	return &featureForTileMatrixWrapper{
		wrapped:      feature,
		newGeometry:  newGeometry,
		tileMatrixID: tileMatrixID,
	}
}

func polygonsToMulti(polygons []geom.Polygon) geom.MultiPolygon {
	l := len(polygons)
	multiPolygon := make(geom.MultiPolygon, l)
	for i := 0; i < l; i++ {
		multiPolygon[i] = polygons[i]
	}
	return multiPolygon
}
