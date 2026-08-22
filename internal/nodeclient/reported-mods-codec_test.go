package nodeclient

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestReportedModsProtoCodecRejectsLargeWireInventoryBeforeUnmarshal(t *testing.T) {
	const modCount = 1_000_000
	result := make([]byte, 0, modCount*2)
	for range modCount {
		result = protowire.AppendTag(result, 3, protowire.BytesType)
		result = protowire.AppendBytes(result, nil)
	}
	payload := protowire.AppendTag(nil, 1, protowire.BytesType)
	payload = protowire.AppendBytes(payload, result)
	if len(payload) >= sevenDaysToDieReportedModsResponseLimit {
		t.Fatalf("wire payload size = %d, want below %d", len(payload), sevenDaysToDieReportedModsResponseLimit)
	}

	codec := reportedModsProtoCodec{}
	if codec.Name() != "proto" {
		t.Fatalf("Name() = %q, want binary proto codec", codec.Name())
	}
	var response nodeprotov1.QuerySevenDaysToDieReportedModsResponse
	errUnmarshal := codec.Unmarshal(payload, &response)
	if !errors.Is(errUnmarshal, errSevenDaysToDieReportedModsWireCount) {
		t.Fatalf("Unmarshal() error = %v, want %v", errUnmarshal, errSevenDaysToDieReportedModsWireCount)
	}
	if response.GetResult() != nil {
		t.Fatal("Unmarshal() allocated the reported mod result before rejecting its wire count")
	}
}

func TestSevenDaysToDieReportedModsFromProtoRetainsCountValidation(t *testing.T) {
	result := &nodeprotov1.SevenDaysToDieReportedMods{
		Mods: make([]*nodeprotov1.SevenDaysToDieReportedMod, node.SevenDaysToDieReportedModCountLimit+1),
	}
	_, errConvert := sevenDaysToDieReportedModsFromProto(result)
	if errConvert == nil {
		t.Fatal("sevenDaysToDieReportedModsFromProto() accepted an over-count inventory")
	}
}

func TestValidateSevenDaysToDieReportedModsResponseWire(t *testing.T) {
	resultWithMod := protowire.AppendTag(nil, 3, protowire.BytesType)
	resultWithMod = protowire.AppendBytes(resultWithMod, nil)
	tests := []struct {
		name    string
		payload []byte
		wantErr bool
	}{
		{
			name:    "ignores nested-looking bytes outside result",
			payload: protowire.AppendBytes(protowire.AppendTag(nil, 2, protowire.BytesType), resultWithMod),
		},
		{
			name: "ignores wrong wire type for mods field",
			payload: protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType),
				protowire.AppendVarint(protowire.AppendTag(nil, 3, protowire.VarintType), 1)),
		},
		{name: "rejects malformed response", payload: []byte{0x0a, 0x02, 0x1a}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errWire := validateSevenDaysToDieReportedModsResponseWire(test.payload)
			if (errWire != nil) != test.wantErr {
				t.Fatalf("validateSevenDaysToDieReportedModsResponseWire() error = %v, want error %t", errWire, test.wantErr)
			}
		})
	}
}
