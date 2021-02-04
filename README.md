# Sieve

![GitHub license](https://img.shields.io/github/license/WouterVisscher/sieve)
[![GitHub release](https://img.shields.io/github/release/WouterVisscher/sieve.svg)](https://github.com/WouterVisscher/sieve/releases)
[![Go Report Card](https://goreportcard.com/badge/WouterVisscher/sieve)](https://goreportcard.com/report/WouterVisscher/sieve)

Sieves [GeoPackage](https://www.geopackage.org/) Polygon geometries.

The purpose of this application is to prerefine the (MULTI)POLYGON geometries in a geopackage used for vectortiles by filtering out geometries (based on the given resolution) smaller then the pixels that are generated from the given vectoriles. By doing this specific artifacts/errors regarding the rendering of vectortiles can be omitted.

## Notes

- It will take a Geopackage and writes a new Geopackage where all the (MULTI)POLYGON tables are sieve.
  - All other geometrie tables are 'untouched' and copied as-is.
  - Other none geometrie tables are not copied to the new geopackage.
- The area of a POLYGON is used for determining if the geometries will be sieved, not the extent. So geometries with a extent larger then the given resolution but with a area smaller then that resolution will be sieved.
- A MULTIPOLYGON will be spilt into seperated POLYGON's that will be sieved. So a MULTIPOLYGON contain parts smaller then the given resolution will 'lose' those parts.

## Usage

```go
go build .

go run . -s=[source gpkg] -t=[target gpkg] -r=[resolution for filtering]
```

## Docker

```docker
docker build -t pdok/sieve .
docker run --rm --name sieve -v `pwd`/example:/example pdok/sieve ./sieve -s=./example/example.gpkg -t=./example/example-processed.gpkg -r=50001
```

With the docker example above processing the ```example.gpkg``` would result in the following.

![with interiors](./images/with-interiors.jpg)  ![without interiors](./images/without-interiors.jpg)

## TODO

- [ ] usage of a CLI package
- [ ] improve error logging/messaging
- [ ] build spatial indexed (RTREE for the generated tables)

## Inspiration

Code is inspired by the PostGis [Sieve function](https://github.com/mapbox/postgis-vt-util/blob/master/src/Sieve.sql) from Mapbox.
