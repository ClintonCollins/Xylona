package helpers

import (
	"testing"
	"time"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGameServerSteamBranchRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	protoInput := testSteamProtoGameServer(now)

	model := GameServerProtoToModel(protoInput)
	if model.Branch != "latest_experimental" {
		t.Fatalf("GameServerProtoToModel().Branch = %q, want %q", model.Branch, "latest_experimental")
	}

	model.CreatedAt = now
	model.UpdatedAt = now
	model.R.Node = &models.Node{}
	model.Version = "21600865"

	vsm := versiontracker.NewVersionStateMap()
	vsm.Set(model.ID, versiontracker.VersionState{
		Status:                versiontracker.VersionStatusChecked,
		InstalledVersion:      "21600865",
		LatestVersion:         "22422094",
		UpdateAvailable:       true,
		TrackerType:           "steam",
		InstalledVersionLabel: "Public (21600865)",
		LatestVersionLabel:    "Unstable build (22422094)",
		InstalledBranch:       "public",
		LatestBranch:          "latest_experimental",
	})

	got := GameServerModelToProto(model, vsm)
	if got.Branch != "latest_experimental" {
		t.Fatalf("GameServerModelToProto().Branch = %q, want %q", got.Branch, "latest_experimental")
	}
	if got.VersionInfo == nil {
		t.Fatal("VersionInfo = nil, want populated steam version info")
	}
	if got.VersionInfo.InstalledVersionLabel != "Public (21600865)" {
		t.Fatalf("InstalledVersionLabel = %q, want %q", got.VersionInfo.InstalledVersionLabel, "Public (21600865)")
	}
	if got.VersionInfo.LatestVersionLabel != "Unstable build (22422094)" {
		t.Fatalf("LatestVersionLabel = %q, want %q", got.VersionInfo.LatestVersionLabel, "Unstable build (22422094)")
	}
	if got.VersionInfo.InstalledBranch != "public" {
		t.Fatalf("InstalledBranch = %q, want %q", got.VersionInfo.InstalledBranch, "public")
	}
	if got.VersionInfo.LatestBranch != "latest_experimental" {
		t.Fatalf("LatestBranch = %q, want %q", got.VersionInfo.LatestBranch, "latest_experimental")
	}

	setter := GameServerModelToSetter(model)
	branch, okBranch := setter.Branch.Get()
	if !okBranch {
		t.Fatal("GameServerModelToSetter().Branch should be set")
	}
	if branch != "latest_experimental" {
		t.Fatalf("GameServerModelToSetter().Branch = %q, want %q", branch, "latest_experimental")
	}
}

func testSteamProtoGameServer(now time.Time) *xylona.GameServer {
	return &xylona.GameServer{
		Id:        "steam-server-1",
		UserId:    "user-1",
		Name:      "Steam Server",
		GameId:    "7dtd",
		Status:    xylona.Status_OFFLINE,
		Ip:        &xylona.IP{Address: "127.0.0.1"},
		CreatedAt: timestamppb.New(now),
		UpdatedAt: timestamppb.New(now),
		Branch:    "latest_experimental",
	}
}
