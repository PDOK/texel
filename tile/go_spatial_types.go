package tile

// These are the types obtained from go-spatial/encoding/mvt/vectortile needed
// for encoding. Modifications made:
// - Renamed types for linter purposes.
// - Dropped some protobuf functionality not used.
// - Use fmt for debug printing rather than proto.

import (
	"fmt"
)

// Geometry types

type GSTileGeomType int32

const (
	TileUNKNOWN    GSTileGeomType = 0
	TilePOINT      GSTileGeomType = 1
	TileLINESTRING GSTileGeomType = 2
	TilePOLYGON    GSTileGeomType = 3
)

var TileGeomTypeName = map[int32]string{
	0: "UNKNOWN",
	1: "POINT",
	2: "LINESTRING",
	3: "POLYGON",
}

var TileGeomTypeValue = map[string]int32{
	"UNKNOWN":    0,
	"POINT":      1,
	"LINESTRING": 2,
	"POLYGON":    3,
}

// GSTileFeature mirrors vector_tile.pb.go's Tile_Feature.

type GSTileFeature struct {
	Id *uint64 `protobuf:"varint,1,opt,name=id,def=0" json:"id,omitempty"`
	// Tags of this feature are encoded as repeated pairs of
	// integers.
	// A detailed description of tags is located in sections
	// 4.2 and 4.4 of the specification
	Tags []uint32 `protobuf:"varint,2,rep,packed,name=tags" json:"tags,omitempty"`
	// The type of geometry stored in this feature.
	Type *GSTileGeomType `protobuf:"varint,3,opt,name=type,enum=vector_tile.Tile_GeomType,def=0" json:"type,omitempty"`
	// Contains a stream of commands and parameters (vertices).
	// A detailed description on geometry encoding is located in
	// section 4.3 of the specification.
	Geometry        []uint32 `protobuf:"varint,4,rep,packed,name=geometry" json:"geometry,omitempty"`
	XXXunrecognized []byte   `json:"-"`
}

func (m *GSTileFeature) Reset()         { *m = GSTileFeature{} }
func (m *GSTileFeature) String() string { return fmt.Sprintf("%+v", *m) }
func (*GSTileFeature) ProtoMessage()    {}

// GSTileValue mirrors vector_tile.pb.go's Tile_Value.
// Exactly one of these fields should be set for a valid value.

type GSTileValue struct {
	StringValue *string  `protobuf:"bytes,1,opt,name=string_value" json:"stringValue,omitempty"`
	FloatValue  *float32 `protobuf:"fixed32,2,opt,name=float_value" json:"floatValue,omitempty"`
	DoubleValue *float64 `protobuf:"fixed64,3,opt,name=double_value" json:"doubleValue,omitempty"`
	IntValue    *int64   `protobuf:"varint,4,opt,name=int_value" json:"intValue,omitempty"`
	UintValue   *uint64  `protobuf:"varint,5,opt,name=uint_value" json:"uintValue,omitempty"`
	SintValue   *int64   `protobuf:"zigzag64,6,opt,name=sint_value" json:"sintValue,omitempty"`
	BoolValue   *bool    `protobuf:"varint,7,opt,name=bool_value" json:"boolValue,omitempty"`
}

func (m *GSTileValue) Reset()         { *m = GSTileValue{} }
func (m *GSTileValue) String() string { return fmt.Sprintf("%+v", *m) }
func (*GSTileValue) ProtoMessage()    {}

// GSTileLayer mirrors vector_tile.pb.go's Tile_Layer.

type GSTileLayer struct {
	Version  *uint32          `protobuf:"varint,15,req,name=version,def=1" json:"version,omitempty"`
	Name     *string          `protobuf:"bytes,1,req,name=name" json:"name,omitempty"`
	Features []*GSTileFeature `protobuf:"bytes,2,rep,name=features" json:"features,omitempty"`
	Keys     []string         `protobuf:"bytes,3,rep,name=keys" json:"keys,omitempty"`
	Values   []*GSTileValue   `protobuf:"bytes,4,rep,name=values" json:"values,omitempty"`
	Extent   *uint32          `protobuf:"varint,5,opt,name=extent,def=4096" json:"extent,omitempty"`
}

func (m *GSTileLayer) Reset()         { *m = GSTileLayer{} }
func (m *GSTileLayer) String() string { return fmt.Sprintf("%+v", *m) }
func (*GSTileLayer) ProtoMessage()    {}

// GSTile mirrors vector_tile.pb.go's Tile.

type GSTile struct {
	Layers []*GSTileLayer `protobuf:"bytes,3,rep,name=layers" json:"layers,omitempty"`
}

func (m *GSTile) Reset()         { *m = GSTile{} }
func (m *GSTile) String() string { return fmt.Sprintf("%+v", *m) }
func (*GSTile) ProtoMessage()    {}
