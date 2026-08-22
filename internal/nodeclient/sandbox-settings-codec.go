package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

var errSevenDaysToDieSandboxSettingsWireCount = errors.New("sandbox setting wire count exceeds limit")

// sandboxSettingsProtoCodec scans the bounded response before protobuf can
// allocate its repeated settings graph.
type sandboxSettingsProtoCodec struct{}

func (sandboxSettingsProtoCodec) Name() string {
	return "proto"
}

func (sandboxSettingsProtoCodec) Marshal(message any) ([]byte, error) {
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

func (sandboxSettingsProtoCodec) Unmarshal(data []byte, message any) error {
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return fmt.Errorf("unmarshal into %T: message does not implement proto.Message", message)
	}
	_, boundedResponse := message.(*nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse)
	if boundedResponse {
		errWire := validateSevenDaysToDieSandboxSettingsResponseWire(data)
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

func validateSevenDaysToDieSandboxSettingsResponseWire(data []byte) error {
	settingCount := 0
	for len(data) > 0 {
		fieldNumber, wireType, fieldLength := protowire.ConsumeField(data)
		if fieldLength < 0 {
			return fmt.Errorf("scan sandbox settings response: %w", protowire.ParseError(fieldLength))
		}
		if fieldNumber == 1 && wireType == protowire.BytesType {
			_, _, tagLength := protowire.ConsumeTag(data)
			if tagLength < 0 {
				return fmt.Errorf("scan sandbox settings result tag: %w", protowire.ParseError(tagLength))
			}
			result, resultLength := protowire.ConsumeBytes(data[tagLength:])
			if resultLength < 0 {
				return fmt.Errorf("scan sandbox settings result: %w", protowire.ParseError(resultLength))
			}
			errResult := countSevenDaysToDieSandboxSettingsWire(result, &settingCount)
			if errResult != nil {
				return errResult
			}
		}
		data = data[fieldLength:]
	}
	return nil
}

func countSevenDaysToDieSandboxSettingsWire(data []byte, settingCount *int) error {
	for len(data) > 0 {
		fieldNumber, wireType, fieldLength := protowire.ConsumeField(data)
		if fieldLength < 0 {
			return fmt.Errorf("scan sandbox settings result: %w", protowire.ParseError(fieldLength))
		}
		if fieldNumber == 6 && wireType == protowire.BytesType {
			if *settingCount >= node.SevenDaysToDieSandboxSettingCountLimit {
				return errSevenDaysToDieSandboxSettingsWireCount
			}
			(*settingCount)++
		}
		data = data[fieldLength:]
	}
	return nil
}
