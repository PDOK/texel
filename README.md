# Sieve

Sieves GeoPackage Polygon geometries.

The reason for this application is to prerefine the POLYGON geometries in a geopackage used for vectortiles by filtering out geometries (based on the given resolution) smaller then the pixels that are generated from the given vectoriles. By doing this specific artifacts regarding the rendering of vectortiles can be omitted.

## Usage

```go
go build .

go run . -s=[source gpkg] -t=[target gpkg] -r=[resolution for filtering]
```

## TODO

- [ ] loop over the available POLYGON tables in a GeoPackage
- [ ] copy source SpatialReferenceSystem information
- [ ] decide on supporting MULTIPOLYGON
- [ ] decide if (MULTI)POINT|LINESTRING also are supported
- [ ] when decide not to support (MULTI)POINT|LINESTRING do we copy the source tables or do nothing at all
- [ ] build spatial indexed (RTREE for the generated tables)
- [ ] usage of a CLI package

## Inspiration

Code is inspired by the PostGis [Sieve function](https://github.com/mapbox/postgis-vt-util/blob/master/src/Sieve.sql) from Mapbox.
