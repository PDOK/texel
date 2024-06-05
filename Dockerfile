ARG GDAL_VERSION=3.6.3

FROM ghcr.io/osgeo/gdal:ubuntu-small-${GDAL_VERSION} AS base
FROM golang:1.21-bullseye AS build-env

ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org

ENV TZ Europe/Amsterdam

WORKDIR /go/src/app

# Because of how the layer caching system works in Docker, the go mod download
# command will _ only_ be re-run when the go.mod or go.sum file change
# (or when we add another docker instruction this line)
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .

# set crosscompiling fla 0/1 => disabled/enabled
ENV CGO_ENABLED=1
# compile linux only
ENV GOOS=linux

# run tests
RUN go test ./... -covermode=atomic

RUN go build -v -buildvcs=true -ldflags='-s -w -linkmode auto' -a -installsuffix cgo -o /texel .

FROM base AS build-image

RUN apt-get update && apt-get install -y \
  libsqlite3-mod-spatialite \
  && rm -rf /var/lib/apt/lists/*

# important for time conversion
ENV TZ Europe/Amsterdam
WORKDIR /

COPY --from=build-env /texel /usr/local/bin/texel

ENTRYPOINT ["texel"]
