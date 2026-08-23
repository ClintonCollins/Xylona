package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
)

var errSevenDaysToDieSandboxSettingsWireCount = errors.New("sandbox setting wire count exceeds limit")

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
