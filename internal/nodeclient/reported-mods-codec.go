package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
)

var (
	errSevenDaysToDieReportedModsWireCount = errors.New("reported mod wire count exceeds limit")
)

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
