package codec

import (
	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Register the custom JSON codec for gRPC
func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec implements gRPC encoding.Codec using protojson
// This properly handles protobuf oneof fields which standard encoding/json cannot
type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return protojson.Marshal(v.(proto.Message))
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return protojson.Unmarshal(data, v.(proto.Message))
}

func (jsonCodec) Name() string {
	return "proto" // Override the default proto codec
}
