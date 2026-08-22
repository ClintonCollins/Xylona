package nodeclient

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestSandboxSettingsProtoCodecRejectsLargeWireInventoryBeforeUnmarshal(t *testing.T) {
	const settingCount = 1_000_000
	result := make([]byte, 0, settingCount*2)
	for range settingCount {
		result = protowire.AppendTag(result, 6, protowire.BytesType)
		result = protowire.AppendBytes(result, nil)
	}
	payload := protowire.AppendTag(nil, 1, protowire.BytesType)
	payload = protowire.AppendBytes(payload, result)
	if len(payload) >= sevenDaysToDieSandboxSettingsResponseLimit {
		t.Fatalf("wire payload size = %d, want below %d", len(payload), sevenDaysToDieSandboxSettingsResponseLimit)
	}

	codec := sandboxSettingsProtoCodec{}
	if codec.Name() != "proto" {
		t.Fatalf("Name() = %q, want binary proto codec", codec.Name())
	}
	var response nodeprotov1.QuerySevenDaysToDieSandboxSettingsResponse
	errUnmarshal := codec.Unmarshal(payload, &response)
	if !errors.Is(errUnmarshal, errSevenDaysToDieSandboxSettingsWireCount) {
		t.Fatalf("Unmarshal() error = %v, want %v", errUnmarshal, errSevenDaysToDieSandboxSettingsWireCount)
	}
	if response.GetResult() != nil {
		t.Fatal("Unmarshal() allocated the sandbox settings result before rejecting its wire count")
	}
}

func TestSevenDaysToDieSandboxSettingsFromProtoRetainsCountValidation(t *testing.T) {
	result := &nodeprotov1.SevenDaysToDieSandboxSettings{
		Settings: make([]*nodeprotov1.SevenDaysToDieSandboxSetting, node.SevenDaysToDieSandboxSettingCountLimit+1),
	}
	_, errConvert := sevenDaysToDieSandboxSettingsFromProto(result)
	if errConvert == nil {
		t.Fatal("sevenDaysToDieSandboxSettingsFromProto() accepted over-count settings")
	}
}

func TestValidateSevenDaysToDieSandboxSettingsResponseWire(t *testing.T) {
	resultWithSetting := protowire.AppendTag(nil, 6, protowire.BytesType)
	resultWithSetting = protowire.AppendBytes(resultWithSetting, nil)
	tests := []struct {
		name    string
		payload []byte
		wantErr bool
	}{
		{
			name:    "ignores nested-looking bytes outside result",
			payload: protowire.AppendBytes(protowire.AppendTag(nil, 2, protowire.BytesType), resultWithSetting),
		},
		{
			name: "ignores wrong wire type for settings field",
			payload: protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType),
				protowire.AppendVarint(protowire.AppendTag(nil, 6, protowire.VarintType), 1)),
		},
		{name: "rejects malformed response", payload: []byte{0x0a, 0x02, 0x32}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errWire := validateSevenDaysToDieSandboxSettingsResponseWire(test.payload)
			if (errWire != nil) != test.wantErr {
				t.Fatalf("validateSevenDaysToDieSandboxSettingsResponseWire() error = %v, want error %t", errWire, test.wantErr)
			}
		})
	}
}
