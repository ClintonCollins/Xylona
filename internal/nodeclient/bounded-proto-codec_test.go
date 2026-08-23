package nodeclient

import (
	"testing"

	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestBoundedProtoCodecZeroValue(t *testing.T) {
	var response nodeprotov1.QuerySevenDaysToDieMapResponse
	errUnmarshal := (boundedProtoCodec{}).Unmarshal(nil, &response)
	if errUnmarshal != nil {
		t.Fatalf("Unmarshal() error = %v", errUnmarshal)
	}
}
