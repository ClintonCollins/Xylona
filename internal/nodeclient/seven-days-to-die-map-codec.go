package nodeclient

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
)

var errSevenDaysToDieMapWireCount = errors.New("map wire item count exceeds limit")

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
