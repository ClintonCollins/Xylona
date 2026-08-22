package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

var (
	errSevenDaysToDieReportedModsWireCount = errors.New("reported mod wire count exceeds limit")
)

// reportedModsProtoCodec remains a binary "proto" codec, so the dedicated
// client never negotiates JSON. It scans the one bounded response before the
// standard protobuf decoder can allocate its repeated message graph.
type reportedModsProtoCodec struct{}

func (reportedModsProtoCodec) Name() string {
	return "proto"
}

func (reportedModsProtoCodec) Marshal(message any) ([]byte, error) {
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

func (reportedModsProtoCodec) Unmarshal(data []byte, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return fmt.Errorf("unmarshal into %T: message does not implement proto.Message", message)
	}
	_, boundedResponse := message.(*nodeprotov1.QuerySevenDaysToDieReportedModsResponse)
	if boundedResponse {
		errWire := validateSevenDaysToDieReportedModsResponseWire(data)
		if errWire != nil {
			return fmt.Errorf("unmarshal into %T: %w", message, errWire)
		}
	}
	errUnmarshal := proto.Unmarshal(data, protoMessage)
	if errUnmarshal != nil {
		return fmt.Errorf("unmarshal into %T: %w", message, errUnmarshal)
	}
	return nil
}

func validateSevenDaysToDieReportedModsResponseWire(data []byte) error {
	modCount := 0
	for len(data) > 0 {
		fieldNumber, wireType, fieldLength := protowire.ConsumeField(data)
		if fieldLength < 0 {
			return fmt.Errorf("scan reported mod response: %w", protowire.ParseError(fieldLength))
		}
		if fieldNumber == 1 && wireType == protowire.BytesType {
			_, _, tagLength := protowire.ConsumeTag(data)
			if tagLength < 0 {
				return fmt.Errorf("scan reported mod result tag: %w", protowire.ParseError(tagLength))
			}
			result, resultLength := protowire.ConsumeBytes(data[tagLength:])
			if resultLength < 0 {
				return fmt.Errorf("scan reported mod result: %w", protowire.ParseError(resultLength))
			}
			errResult := countSevenDaysToDieReportedModsWire(result, &modCount)
			if errResult != nil {
				return errResult
			}
		}
		data = data[fieldLength:]
	}
	return nil
}

func countSevenDaysToDieReportedModsWire(data []byte, modCount *int) error {
	for len(data) > 0 {
		fieldNumber, wireType, fieldLength := protowire.ConsumeField(data)
		if fieldLength < 0 {
			return fmt.Errorf("scan reported mod inventory: %w", protowire.ParseError(fieldLength))
		}
		if fieldNumber == 3 && wireType == protowire.BytesType {
			if *modCount >= node.SevenDaysToDieReportedModCountLimit {
				return errSevenDaysToDieReportedModsWireCount
			}
			(*modCount)++
		}
		data = data[fieldLength:]
	}
	return nil
}
