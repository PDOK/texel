// Package processing takes care of the logistics around reading and writing to a Target.
// Not the processing operation(s) itself.
package processing

import (
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
			newPolygonPerTileMatrix := f(polygon, tmIDs)
			if len(newPolygonPerTileMatrix) > 0 {
				postCount++
			}
			for tmID, newPolygon := range newPolygonPerTileMatrix {
				featuresOut <- wrapFeatureForTileMatrix(feature, tmID, newPolygon)
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
			for tmID := range tmIDs {
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
		channel <- feature
	}

	// close the channels, the targets will do their last writing
	for _, targetChannel := range targetChannels {
		close(targetChannel)
	}

	wg.Wait()
}

// processMultiPolygon will split itself into the separated polygons that will be processed before building a new MULTIPOLYGON
func processMultiPolygon(mp geom.MultiPolygon, tileMatrixIDs []int, f processPolygonFunc) map[int]geom.MultiPolygon {
	newMultiPolygonPerTileMatrix := make(map[int]geom.MultiPolygon, len(tileMatrixIDs))
	for _, p := range mp {
		newPolygonPerTileMatrix := f(p, tileMatrixIDs)
		for tmID, newPolygon := range newPolygonPerTileMatrix {
			newMultiPolygonPerTileMatrix[tmID] = append(newMultiPolygonPerTileMatrix[tmID], newPolygon)
		}
	}
	return newMultiPolygonPerTileMatrix
}

type processPolygonFunc func(p geom.Polygon, tileMatrixIDs []int) map[int]geom.Polygon

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
