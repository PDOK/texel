package tile

import oldproto "github.com/golang/protobuf/proto" //nolint:staticcheck // needed: matches the reflection-based marshaling vector_tile.pb.go relies on

// MarshalTile serializes a GSTile into MVT protobuf bytes. Kept in this package so
// callers never need to depend on the protobuf library directly.
func MarshalTile(t *GSTile) ([]byte, error) {
	return oldproto.Marshal(t)
}
