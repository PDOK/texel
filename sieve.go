package main

import (
	"fmt"
	"log"
	"math"

	"github.com/go-spatial/geom"
)

// readFeatures reads the features from the given Geopackage table
// and decodes the WKB geometry to a geom.Polygon
func readFeaturesFromSource(source Source, preSieve chan feature, t table) {
	source.ReadFeatures(t, preSieve)
}

// sieveFeatures sieves/filters the geometry against the given resolution
// the two steps that are done are:
// 1. filter features with a area smaller then the (resolution*resolution)
// 2. removes interior rings with a area smaller then the (resolution*resolution)
func sieveFeatures(preSieve chan feature, postSieve chan feature, resolution float64) {
	var preSieveCount, postSieveCount, nonPolygonCount, multiPolygonCount uint64
	for {
		feature, hasMore := <-preSieve
		if !hasMore {
			break
		} else {
			preSieveCount++
			switch feature.Geometry().(type) {
			case geom.Polygon:
				var p geom.Polygon
				p = feature.Geometry().(geom.Polygon)
				if p := polygonSieve(p, resolution); p != nil {
					feature.UpdateGeometry(p)
					postSieveCount++
					postSieve <- feature
				}
			case geom.MultiPolygon:
				var mp geom.MultiPolygon
				mp = feature.Geometry().(geom.MultiPolygon)
				if mp := multiPolygonSieve(mp, resolution); mp != nil {
					feature.UpdateGeometry(mp)
					multiPolygonCount++
					postSieveCount++
					postSieve <- feature
				}
			default:
				postSieveCount++
				nonPolygonCount++
				postSieve <- feature
			}
		}
	}
	close(postSieve)

	log.Printf("    total features: %d", preSieveCount)
	log.Printf("      non-polygons: %d", nonPolygonCount)
	if preSieveCount != nonPolygonCount {
		log.Printf("     multipolygons: %d", multiPolygonCount)
	}
	log.Printf("              kept: %d", postSieveCount)
}

// writeFeatures collects the processed features by the sieveFeatures and
// creates a WKB binary from the geometry
// The collected feature array, based on the pagesize, is then passed to the writeFeaturesArray
func writeFeaturesToTarget(postSieve chan feature, kill chan bool, target Target, t table) {

	target.WriteFeatures(t, postSieve)
	kill <- true
}

// multiPolygonSieve will split it self into the separated polygons that will be sieved before building a new MULTIPOLYGON
func multiPolygonSieve(mp geom.MultiPolygon, resolution float64) geom.MultiPolygon {
	var sievedMultiPolygon geom.MultiPolygon
	for _, p := range mp {
		if sievedPolygon := polygonSieve(p, resolution); sievedPolygon != nil {
			sievedMultiPolygon = append(sievedMultiPolygon, sievedPolygon)
		}
	}
	return sievedMultiPolygon
}

// polygonSieve will sieve a given POLYGON
func polygonSieve(p geom.Polygon, resolution float64) geom.Polygon {
	minArea := resolution * resolution
	if area(p) > minArea {
		if len(p) > 1 {
			var sievedPolygon geom.Polygon
			sievedPolygon = append(sievedPolygon, p[0])
			for _, interior := range p[1:] {
				if shoelace(interior) > minArea {
					sievedPolygon = append(sievedPolygon, interior)
				}
			}
			return sievedPolygon
		}
		return p
	}
	return nil
}

// calculate the area of a polygon
func area(geom [][][2]float64) float64 {
	interior := .0
	if geom == nil {
		return 0.
	}
	if len(geom) > 1 {
		for _, i := range geom[1:] {
			interior = interior + shoelace(i)
		}
	}
	return shoelace(geom[0]) - interior
}

// https://en.wikipedia.org/wiki/Shoelace_formula
func shoelace(pts [][2]float64) float64 {
	sum := 0.
	if len(pts) == 0 {
		return 0.
	}

	p0 := pts[len(pts)-1]
	for _, p1 := range pts {
		sum += p0[1]*p1[0] - p0[0]*p1[1]
		p0 = p1
	}
	return math.Abs(sum / 2)
}

func Sieve(source Source, target Target, table table, resolution float64) {
	log.Printf("  sieving %s", table.name)
	preSieve := make(chan feature)
	postSieve := make(chan feature)
	kill := make(chan bool)

	go writeFeaturesToTarget(postSieve, kill, target, table)
	go sieveFeatures(preSieve, postSieve, resolution)
	go readFeaturesFromSource(source, preSieve, table)

	for {
		if <-kill {
			break
		}
	}
	close(kill)
	log.Println(fmt.Sprintf(`  finished %s`, table.name))
	log.Println("")
}
