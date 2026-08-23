package nodeclient

import (
	"errors"
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ClintonCollins/Xylona/internal/node"
	nodeprotov1 "github.com/ClintonCollins/Xylona/proto/go/xylona/nodeproto/v1"
)

func TestSevenDaysToDieMapProtoCodecRejectsLargeWireOverlayBeforeUnmarshal(t *testing.T) {
	snapshotPayload := func(count int) []byte {
		snapshot := make([]byte, 0, count*2)
		for range count {
			snapshot = protowire.AppendTag(snapshot, 14, protowire.BytesType)
			snapshot = protowire.AppendBytes(snapshot, nil)
		}
		return snapshot
	}
	appendSnapshot := func(payload []byte, count int) []byte {
		payload = protowire.AppendTag(payload, 1, protowire.BytesType)
		return protowire.AppendBytes(payload, snapshotPayload(count))
	}
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "one snapshot", payload: appendSnapshot(nil, node.SevenDaysToDieMapItemLimit+1)},
		{
			name: "merged snapshot fields",
			payload: appendSnapshot(
				appendSnapshot(nil, node.SevenDaysToDieMapItemLimit/2+1),
				node.SevenDaysToDieMapItemLimit/2,
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			codec := sevenDaysToDieMapProtoCodec{}
			var response nodeprotov1.QuerySevenDaysToDieMapResponse
			errUnmarshal := codec.Unmarshal(test.payload, &response)
			if !errors.Is(errUnmarshal, errSevenDaysToDieMapWireCount) {
				t.Fatalf("Unmarshal() error = %v, want %v", errUnmarshal, errSevenDaysToDieMapWireCount)
			}
			if response.GetSnapshot() != nil {
				t.Fatal("Unmarshal() allocated the map snapshot before rejecting its wire count")
			}
		})
	}
}

func TestSevenDaysToDieMapSnapshotFromProtoValidatesTacticalData(t *testing.T) {
	base := func() *nodeprotov1.SevenDaysToDieMapSnapshot {
		return &nodeprotov1.SevenDaysToDieMapSnapshot{
			Enabled: true, TileSize: 128, MaxZoom: 4,
			MapSize: &nodeprotov1.SevenDaysToDieMapVector{X: 6144, Y: 255, Z: 6144},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*nodeprotov1.SevenDaysToDieMapSnapshot)
		wantErr bool
	}{
		{
			name: "maps independent entity overlays",
			mutate: func(snapshot *nodeprotov1.SevenDaysToDieMapSnapshot) {
				snapshot.HostileState = nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
				snapshot.Hostiles = []*nodeprotov1.SevenDaysToDieMapEntity{{
					Name: "Zombie", Position: &nodeprotov1.SevenDaysToDieMapVector{X: 10, Y: 2, Z: 20},
				}}
			},
		},
		{
			name: "rejects non-finite remote position",
			mutate: func(snapshot *nodeprotov1.SevenDaysToDieMapSnapshot) {
				snapshot.AnimalState = nodeprotov1.SevenDaysToDieWebAPIValueState_SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
				snapshot.Animals = []*nodeprotov1.SevenDaysToDieMapEntity{{
					Name: "Wolf", Position: &nodeprotov1.SevenDaysToDieMapVector{X: math.Inf(1)},
				}}
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := base()
			test.mutate(snapshot)
			mapped, errMap := sevenDaysToDieMapSnapshotFromProto(snapshot)
			if (errMap != nil) != test.wantErr {
				t.Fatalf("sevenDaysToDieMapSnapshotFromProto() error = %v, want error %t", errMap, test.wantErr)
			}
			if !test.wantErr && (mapped == nil || len(mapped.Hostiles) != 1 || mapped.AnimalState != node.SevenDaysToDieWebAPIValueStateUnspecified) {
				t.Fatalf("mapped snapshot = %+v", mapped)
			}
		})
	}
}

func TestSevenDaysToDieMapSnapshotFromProtoRetainsCountValidation(t *testing.T) {
	snapshot := &nodeprotov1.SevenDaysToDieMapSnapshot{
		Animals: make([]*nodeprotov1.SevenDaysToDieMapEntity, node.SevenDaysToDieMapItemLimit+1),
	}
	_, errConvert := sevenDaysToDieMapSnapshotFromProto(snapshot)
	if errConvert == nil {
		t.Fatal("sevenDaysToDieMapSnapshotFromProto() accepted an over-count overlay")
	}
}
