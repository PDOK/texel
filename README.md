# Sieve

![GitHub license](https://img.shields.io/github/license/PDOK/sieve) [![GitHub
release](https://img.shields.io/github/release/PDOK/sieve.svg)](https://github.com/PDOK/sieve/releases)
[![Go Report
Card](https://goreportcard.com/badge/PDOK/sieve)](https://goreportcard.com/report/PDOK/sieve)
[![Docker
Pulls](https://img.shields.io/docker/pulls/pdok/sieve.svg)](https://hub.docker.com/r/pdok/sieve)

Sieves [GeoPackage](https://www.geopackage.org/) Polygon geometries.

The purpose of this application is to prerefine the (MULTI)POLYGON geometries in
a geopackage used for vectortiles by filtering out geometries (based on the
given resolution) smaller then the pixels that are generated from the given
vectoriles. By doing this specific artifacts/errors regarding the rendering of
vectortiles can be omitted.

## Notes

- It will take a Geopackage and writes a new Geopackage where all the
  (MULTI)POLYGON tables are sieve.
  - All other geometry tables are 'untouched' and copied as-is.
  - Other none geometry tables are not copied to the new geopackage.
- The area of a POLYGON is used for determining if the geometries will be
  sieved, not the extent. So geometries with a extent larger then the given
  resolution but with a area smaller then that resolution will be sieved.
- A MULTIPOLYGON will be spilt into seperated POLYGON's that will be sieved. So
  a MULTIPOLYGON contain parts smaller then the given resolution will 'lose'
  those parts.

## Usage

```go
go build .

go run . -s=[source GPKG] -t=[target GPKG] -r=[resolution for filtering] \
   -p=[pagesize for writing to target GPKG]
```

## Docker

```docker
docker build -t pdok/sieve .
docker run --rm --name sieve -v `pwd`/example:/example pdok/sieve ./sieve \
   -s=./example/example.gpkg -t=./example/example-processed.gpkg -r=50001 -p=10
```

With the docker example above processing the ```example.gpkg``` would result in
the following.

![with interiors](./images/with-interiors.jpg)  ![without
interiors](./images/without-interiors.jpg)

## TODO

- [ ] usage of a CLI package
- [ ] build spatial indexed (RTREE for the generated tables)

## Inspiration

Code is inspired by the PostGis [Sieve
function](https://github.com/mapbox/postgis-vt-util/blob/master/src/Sieve.sql)
from Mapbox.
