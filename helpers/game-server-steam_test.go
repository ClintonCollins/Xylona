package helpers

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
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
	if got.GetSelectedTarget() != "latest_experimental" {
		t.Fatalf("GameServerModelToProto().SelectedTarget = %q, want %q", got.GetSelectedTarget(), "latest_experimental")
	}
	if got.GetVersionInfo() == nil {
		t.Fatal("VersionInfo = nil, want populated steam version info")
	}
	if got.GetVersionInfo().GetInstalledVersionLabel() != "Public (21600865)" {
		t.Fatalf("InstalledVersionLabel = %q, want %q", got.GetVersionInfo().GetInstalledVersionLabel(), "Public (21600865)")
	}
	if got.GetVersionInfo().GetLatestVersionLabel() != "Unstable build (22422094)" {
		t.Fatalf("LatestVersionLabel = %q, want %q", got.GetVersionInfo().GetLatestVersionLabel(), "Unstable build (22422094)")
	}
	if got.GetVersionInfo().GetInstalledBranch() != "public" {
		t.Fatalf("InstalledBranch = %q, want %q", got.GetVersionInfo().GetInstalledBranch(), "public")
	}
	if got.GetVersionInfo().GetLatestBranch() != "latest_experimental" {
		t.Fatalf("LatestBranch = %q, want %q", got.GetVersionInfo().GetLatestBranch(), "latest_experimental")
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
		Id:             "steam-server-1",
		UserId:         "user-1",
		Name:           "Steam Server",
		GameId:         "7dtd",
		Status:         xylona.Status_OFFLINE,
		Ip:             &xylona.IP{Address: "127.0.0.1"},
		CreatedAt:      timestamppb.New(now),
		UpdatedAt:      timestamppb.New(now),
		SelectedTarget: "latest_experimental",
	}
}
