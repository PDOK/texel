package processing

import (
	"log"

	"github.com/go-spatial/geom"
)

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeaturesFromSource(source Source, features chan<- Feature) {
	source.ReadFeatures(features)
}

// processFeatures processes the geometries in the features with the given function
func processFeatures(featuresIn <-chan Feature, featuresOut chan<- Feature, f processPolygonFunc) {
	var preCount, postCount, nonPolygonCount, multiPolygonCount uint64
	for {
		feature, hasMore := <-featuresIn
		if !hasMore {
			break
		}
		preCount++
		switch feature.Geometry().(type) {
		case geom.Polygon:
			var p geom.Polygon
			p = feature.Geometry().(geom.Polygon)
			if p := f(p); p != nil {
				feature.UpdateGeometry(p)
				postCount++
				featuresOut <- feature
			}
		case geom.MultiPolygon:
			var mp geom.MultiPolygon
			mp = feature.Geometry().(geom.MultiPolygon)
			if mp := processMultiPolygon(mp, f); mp != nil {
				feature.UpdateGeometry(mp)
				multiPolygonCount++
				postCount++
				featuresOut <- feature
			}
		default:
			postCount++
			nonPolygonCount++
			featuresOut <- feature
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
func writeFeaturesToTarget(features <-chan Feature, kill chan bool, target Target) {
	target.WriteFeatures(features)
	kill <- true
}

// processMultiPolygon will split itself into the separated polygons that will be processed before building a new MULTIPOLYGON
func processMultiPolygon(mp geom.MultiPolygon, f processPolygonFunc) geom.MultiPolygon {
	var resultMultiPolygon geom.MultiPolygon
	for _, p := range mp {
		if resultPolygon := f(p); resultPolygon != nil {
			resultMultiPolygon = append(resultMultiPolygon, resultPolygon)
		}
	}
	return resultMultiPolygon
}

type processPolygonFunc func(p geom.Polygon) geom.Polygon

func ProcessFeatures(source Source, target Target, f processPolygonFunc) {
	featuresBefore := make(chan Feature)
	featuresAfter := make(chan Feature)
	kill := make(chan bool)

	go writeFeaturesToTarget(featuresAfter, kill, target)
	go processFeatures(featuresBefore, featuresAfter, f)
	go readFeaturesFromSource(source, featuresBefore)

	for {
		if <-kill {
			break
		}
	}
	close(kill)
}
