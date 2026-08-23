package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

var errSevenDaysToDieMapWireCount = errors.New("map wire item count exceeds limit")

// sevenDaysToDieMapProtoCodec scans repeated snapshot fields before protobuf
// can allocate their object graphs.
type sevenDaysToDieMapProtoCodec struct{}

func (sevenDaysToDieMapProtoCodec) Name() string {
	return "proto"
}

func (sevenDaysToDieMapProtoCodec) Marshal(message any) ([]byte, error) {
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

func (sevenDaysToDieMapProtoCodec) Unmarshal(data []byte, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return fmt.Errorf("unmarshal into %T: message does not implement proto.Message", message)
	}
	_, boundedResponse := message.(*nodeprotov1.QuerySevenDaysToDieMapResponse)
	if boundedResponse {
		errWire := validateSevenDaysToDieMapResponseWire(data)
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

func validateSevenDaysToDieMapResponseWire(data []byte) error {
	counts := make(map[protowire.Number]int, 5)
	for len(data) > 0 {
		fieldNumber, wireType, fieldLength := protowire.ConsumeField(data)
		if fieldLength < 0 {
			return fmt.Errorf("scan map response: %w", protowire.ParseError(fieldLength))
		}
		if fieldNumber == 1 && wireType == protowire.BytesType {
			_, _, tagLength := protowire.ConsumeTag(data)
			snapshot, snapshotLength := protowire.ConsumeBytes(data[tagLength:])
			if snapshotLength < 0 {
				return fmt.Errorf("scan map snapshot: %w", protowire.ParseError(snapshotLength))
			}
			for len(snapshot) > 0 {
				number, snapshotWireType, snapshotFieldLength := protowire.ConsumeField(snapshot)
				if snapshotFieldLength < 0 {
					return fmt.Errorf("scan map snapshot: %w", protowire.ParseError(snapshotFieldLength))
				}
				if snapshotWireType == protowire.BytesType && (number == 6 || number == 7 || number == 8 || number == 14 || number == 16) {
					counts[number]++
					if counts[number] > node.SevenDaysToDieMapItemLimit {
						return errSevenDaysToDieMapWireCount
					}
				}
				snapshot = snapshot[snapshotFieldLength:]
			}
		}
		data = data[fieldLength:]
	}
	return nil
}
