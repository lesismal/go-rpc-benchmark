// Package pbcodec encodes the benchmark's messages with
// google.golang.org/protobuf, for the frameworks that take a codec of the
// program's own: arpc and fib's arpc set it as their default codec, and fib's
// grpc registers it as "proto". Kitex, rpcx and grpc encode protobuf
// themselves.
package pbcodec

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// Codec is a protobuf codec of google.golang.org/protobuf messages.
type Codec struct{}

// Name is the gRPC content subtype the codec serves.
func (Codec) Name() string { return "proto" }

// Marshal encodes v, which must be a proto.Message.
func (Codec) Marshal(v any) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("pbcodec: %T is not a proto.Message", v)
	}
	return proto.Marshal(m)
}

// Unmarshal decodes data into v, which must be a proto.Message.
func (Codec) Unmarshal(data []byte, v any) error {
	m, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("pbcodec: %T is not a proto.Message", v)
	}
	return proto.Unmarshal(data, m)
}
