package tile

// This file was copied from go-spatial/geom/encoding/mvt/vector_tile/vector_tile.pb.go.
// Modifications made:
// - Renamed Tile/Tile_Layer/Tile_Value to GSTile/GSTileLayer/GSTileValue, and dropped the
//   GeomType/Feature declarations that already live in go_spatial_feature.go.
// - Dropped proto.XXX_InternalExtensions/ExtensionRangeArray/Descriptor: extensions and
//   the file descriptor are not used by this codebase.
// - String() uses fmt instead of proto.CompactTextString, to avoid pulling in the
//   proto package just for debug printing.

import "fmt"

// GSTileValue mirrors vector_tile.pb.go's Tile_Value. Exactly one of these fields
// should be set for a valid value.
type GSTileValue struct {
	StringValue *string  `protobuf:"bytes,1,opt,name=string_value" json:"string_value,omitempty"`
	FloatValue  *float32 `protobuf:"fixed32,2,opt,name=float_value" json:"float_value,omitempty"`
	DoubleValue *float64 `protobuf:"fixed64,3,opt,name=double_value" json:"double_value,omitempty"`
	IntValue    *int64   `protobuf:"varint,4,opt,name=int_value" json:"int_value,omitempty"`
	UintValue   *uint64  `protobuf:"varint,5,opt,name=uint_value" json:"uint_value,omitempty"`
	SintValue   *int64   `protobuf:"zigzag64,6,opt,name=sint_value" json:"sint_value,omitempty"`
	BoolValue   *bool    `protobuf:"varint,7,opt,name=bool_value" json:"bool_value,omitempty"`
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
