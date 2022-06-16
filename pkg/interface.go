package pkg

import "github.com/go-spatial/geom"

type Feature interface {
	Columns() []interface{}
	Geometry() geom.Geometry
	UpdateGeometry(geom.Geometry)
}

type Source interface {
	ReadFeatures(chan Feature)
}

type Target interface {
	WriteFeatures(chan Feature)
}
