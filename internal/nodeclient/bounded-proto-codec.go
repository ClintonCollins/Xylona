package nodeclient

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// boundedProtoCodec validates selected responses before protobuf can allocate
// their repeated message graphs.
type boundedProtoCodec struct {
	validate func([]byte, any) error
}

func boundedProtoCodecFor[T proto.Message](validate func([]byte) error) boundedProtoCodec {
	return boundedProtoCodec{
		validate: func(data []byte, message any) error {
			_, boundedResponse := message.(T)
			if !boundedResponse {
				return nil
			}
			return validate(data)
		},
	}
}

func (boundedProtoCodec) Name() string {
	return "proto"
}

func (boundedProtoCodec) Marshal(message any) ([]byte, error) {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("marshal %T: message does not implement proto.Message", message)
	}
	data, errMarshal := proto.Marshal(protoMessage)
	if errMarshal != nil {
		return nil, fmt.Errorf("marshal %T: %w", message, errMarshal)
	}
	return data, nil
}

func (codec boundedProtoCodec) Unmarshal(data []byte, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return fmt.Errorf("unmarshal into %T: message does not implement proto.Message", message)
	}
	if codec.validate != nil {
		errValidate := codec.validate(data, message)
		if errValidate != nil {
			return fmt.Errorf("unmarshal into %T: %w", message, errValidate)
		}
	}
	errUnmarshal := proto.Unmarshal(data, protoMessage)
	if errUnmarshal != nil {
		return fmt.Errorf("unmarshal into %T: %w", message, errUnmarshal)
	}
	return nil
}
