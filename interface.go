package main

import "github.com/go-spatial/geom"

type feature interface {
	Columns() []interface{}
	Geometry() geom.Geometry
	UpdateGeometry(geom.Geometry)
}

type features []interface{}

type Source interface {
	ReadFeatures(table, chan feature)
}

type Target interface {
	WriteFeatures(table, chan feature)
}
