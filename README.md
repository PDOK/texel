<!-- TODO: Rewrite README to reflect purpose/usage of the texel tool -->
# `texel`

![GitHub license](https://img.shields.io/github/license/PDOK/texel) [![GitHub
release](https://img.shields.io/github/release/PDOK/texel.svg)](https://github.com/PDOK/texel/releases)
[![Go Report
Card](https://goreportcard.com/badge/PDOK/texel)](https://goreportcard.com/report/PDOK/texel)
[![Docker
Pulls](https://img.shields.io/docker/pulls/pdok/texel.svg)](https://hub.docker.com/r/pdok/texel)

Sieves [GeoPackage](https://www.geopackage.org/) Polygon geometries.

The purpose of this application is to prerefine the (MULTI)POLYGON geometries in
a geopackage used for vector tiles by filtering out geometries (based on the
given resolution) smaller than the pixels that are generated from the given
vectoriles. By doing this specific artifacts/errors regarding the rendering of
vector tiles can be omitted, and less data needs to be processed.

## Notes

- It will take a Geopackage and writes a new Geopackage where all the
  (MULTI)POLYGON tables are sieved.
  - All other spatial tables are 'untouched' and copied as-is.
  - Other not spatial tables are not copied to the new geopackage.
- The area of a POLYGON is used for determining if the geometries will be
  sieved, not the extent. So geometries with a extent larger then the given
  resolution but with an area smaller then that resolution will be sieved.
- A MULTIPOLYGON will be split into separate POLYGONs that will be sieved. So
  a MULTIPOLYGON containing elements smaller then the given resolution will have
  those parts removed.
- :warning: Spatialite lib is mandatory for running this application. This lib is needed for
  creating the RTree triggers on the spatial tables for updating/maintaining the
  RTree.

## Usage

```go
go build .

go run . -s=[source GPKG] -t=[target GPKG] -m=[tilematrix for filtering] \
   -p=[pagesize for writing to target GPKG] -o=[overwrite target GPKG]

go test ./... -covermode=atomic
```

## Docker

```docker
docker build -t pdok/texel .
docker run --rm --name texel -v `pwd`/example:/example pdok/texel ./texel \
   -s=./example/example.gpkg -t=./example/example-processed.gpkg \
   -m="{\"MinX\": -285401.92, \"MaxY\": 903401.92, \"PixelSize\": 16, \"TileSize\": 256, \"Level\": 5, \"CellSize\": 107.52}" \
   -p=10 -o=false
```

With the docker example above processing the ```example.gpkg``` would result in
the following.

![with interiors](./images/with-interiors.jpg)  ![without
interiors](./images/without-interiors.jpg)

<!-- ## Inspiration

Code is inspired by the PostGis [Sieve
function](https://github.com/mapbox/postgis-vt-util/blob/master/src/texel.sql)
from Mapbox. -->
