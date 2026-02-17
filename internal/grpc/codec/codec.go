package codec

import (
	"encoding/json"

	"google.golang.org/grpc/encoding"
)

// Register the custom JSON codec for gRPC
func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec implements gRPC encoding.Codec using JSON
type jsonCodec struct{}

func (jsonCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string {
	return "proto" // Override the default proto codec
}
