package helpers

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/versiontracker"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGameServerModelStatusToProtoStatusCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  xylona.Status
	}{
		{name: "ONLINE uppercase", input: "ONLINE", want: xylona.Status_ONLINE},
		{name: "OFFLINE uppercase", input: "OFFLINE", want: xylona.Status_OFFLINE},
		{name: "UNKNOWN uppercase", input: "UNKNOWN", want: xylona.Status_UNKNOWN},
		{name: "INSTALLING uppercase", input: "INSTALLING", want: xylona.Status_INSTALLING},
		{name: "UPDATING uppercase", input: "UPDATING", want: xylona.Status_UPDATING},
		{name: "online lowercase", input: "online", want: xylona.Status_ONLINE},
		{name: "offline lowercase", input: "offline", want: xylona.Status_OFFLINE},
		{name: "unknown lowercase", input: "unknown", want: xylona.Status_UNKNOWN},
		{name: "installing lowercase", input: "installing", want: xylona.Status_INSTALLING},
		{name: "updating lowercase", input: "updating", want: xylona.Status_UPDATING},
		{name: "mixed case Online", input: "Online", want: xylona.Status_ONLINE},
		{name: "whitespace padded", input: "  ONLINE  ", want: xylona.Status_ONLINE},
		{name: "rune fallback OFFLINE (1)", input: "\x01", want: xylona.Status_OFFLINE},
		{name: "rune fallback ONLINE (2)", input: "\x02", want: xylona.Status_ONLINE},
		{name: "rune fallback INSTALLING (3)", input: "\x03", want: xylona.Status_INSTALLING},
		{name: "rune fallback UPDATING (4)", input: "\x04", want: xylona.Status_UPDATING},
		{name: "rune fallback UNKNOWN (0)", input: "\x00", want: xylona.Status_UNKNOWN},
		{name: "invalid single byte defaults to UNKNOWN", input: "\x05", want: xylona.Status_UNKNOWN},
		{name: "empty string defaults to UNKNOWN", input: "", want: xylona.Status_UNKNOWN},
		{name: "random invalid string defaults to UNKNOWN", input: "BOGUS", want: xylona.Status_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GameServerModelStatusToProtoStatus(tt.input)
			if got != tt.want {
				t.Errorf("GameServerModelStatusToProtoStatus(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGameServerProtoToModel(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &xylona.GameServer{
		Id:                        "gs-1",
		UserId:                    "user-1",
		Name:                      "My Server",
		GameId:                    "game-1",
		StartArgsPatches:          `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]`,
		Status:                    xylona.Status_ONLINE,
		SetMaxPlayers:             32,
		MaxPlayers:                64,
		Map:                       "de_dust2",
		Ip:                        &xylona.IP{Address: "192.168.1.1"},
		Port:                      27015,
		QueryPort:                 27016,
		Directory:                 "/srv/game",
		MaxMemoryMb:               4096,
		BackupsEnabled:            true,
		SteamGameServerLoginToken: "token-abc",
		BackupDirectory:           "/backups",
		MaxBackups:                5,
		NodeId:                    "node-1",
		SelectedTarget:            "1.21.4",
		SelectedTargetPinned:      true,
		SelectedVariantId:         "vanilla",
		CreatedAt:                 timestamppb.New(now),
		UpdatedAt:                 timestamppb.New(now),
	}

	got := GameServerProtoToModel(input)

	if got.ID != "gs-1" {
		t.Errorf("ID = %q, want %q", got.ID, "gs-1")
	}
	if got.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user-1")
	}
	if got.Name != "My Server" {
		t.Errorf("Name = %q, want %q", got.Name, "My Server")
	}
	if got.StartArgsPatches != `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]` {
		t.Errorf("StartArgsPatches = %q, want %q", got.StartArgsPatches, `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]`)
	}
	if got.Status != "ONLINE" {
		t.Errorf("Status = %q, want %q", got.Status, "ONLINE")
	}
	if got.SetPlayers != 32 {
		t.Errorf("SetPlayers = %d, want %d", got.SetPlayers, 32)
	}
	if got.MaxPlayers != 64 {
		t.Errorf("MaxPlayers = %d, want %d", got.MaxPlayers, 64)
	}
	if got.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want %q", got.IP, "192.168.1.1")
	}
	if got.Port != 27015 {
		t.Errorf("Port = %d, want %d", got.Port, 27015)
	}
	if got.MaxMemoryMB != 4096 {
		t.Errorf("MaxMemoryMB = %d, want %d", got.MaxMemoryMB, 4096)
	}
	if !got.BackupsEnabled {
		t.Errorf("BackupsEnabled = %v, want true", got.BackupsEnabled)
	}
	if got.SteamGameServerLoginToken != "token-abc" {
		t.Errorf("SteamGameServerLoginToken = %q, want %q", got.SteamGameServerLoginToken, "token-abc")
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want %q", got.NodeID, "node-1")
	}
	if got.Branch != "1.21.4" {
		t.Errorf("Branch = %q, want %q", got.Branch, "1.21.4")
	}
	if !got.TargetPinned {
		t.Errorf("TargetPinned = %v, want true", got.TargetPinned)
	}
	if got.ServerSoftware.GetOr("") != "vanilla" {
		t.Errorf("ServerSoftware = %q, want %q", got.ServerSoftware.GetOr(""), "vanilla")
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}
}

func TestGameServerModelToProto(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	t.Run("fully populated with relations", func(t *testing.T) {
		input := &models.GameServer{
			ID:                        "gs-1",
			UserID:                    "user-1",
			Name:                      "My Server",
			GameID:                    "game-1",
			StartArgsPatches:          `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]`,
			Status:                    "ONLINE",
			SetPlayers:                32,
			MaxPlayers:                64,
			Map:                       "de_dust2",
			IP:                        "192.168.1.1",
			Port:                      27015,
			QueryPort:                 27016,
			Directory:                 "/srv/game",
			MaxMemoryMB:               4096,
			BackupsEnabled:            true,
			SteamGameServerLoginToken: "token-abc",
			BackupDirectory:           "/backups",
			MaxBackups:                5,
			Version:                   "1.0.0",
			NodeID:                    "node-1",
			Branch:                    "1.21.4",
			TargetPinned:              true,
			CreatedAt:                 now,
			UpdatedAt:                 now,
		}
		input.R.Game = &models.Game{Name: "Counter-Strike"}
		input.R.User = &models.User{UserName: "admin"}
		input.R.Node = &models.Node{Name: "Node-1", ListenURL: "https://node1.example.com:8080"}

		got := GameServerModelToProto(input, nil)

		if got.GetId() != "gs-1" {
			t.Errorf("Id = %q, want %q", got.GetId(), "gs-1")
		}
		if got.GetStartArgsPatches() != `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]` {
			t.Errorf("StartArgsPatches = %q, want %q", got.GetStartArgsPatches(), `[{"id":"mem","op":"edit","tokens":["-Xmx4G"]}]`)
		}
		if got.GetStatus() != xylona.Status_ONLINE {
			t.Errorf("Status = %v, want %v", got.GetStatus(), xylona.Status_ONLINE)
		}
		if got.GetGameName() != "Counter-Strike" {
			t.Errorf("GameName = %q, want %q", got.GetGameName(), "Counter-Strike")
		}
		if got.GetUserName() != "admin" {
			t.Errorf("UserName = %q, want %q", got.GetUserName(), "admin")
		}
		if got.GetNodeName() != "Node-1" {
			t.Errorf("NodeName = %q, want %q", got.GetNodeName(), "Node-1")
		}
		if got.GetNodeHost() != "https://node1.example.com:8080" {
			t.Errorf("NodeHost = %q, want %q", got.GetNodeHost(), "https://node1.example.com:8080")
		}
		if got.GetNodePort() != 0 {
			t.Errorf("NodePort = %d, want %d", got.GetNodePort(), 0)
		}
		if got.GetVersion() != "1.0.0" {
			t.Errorf("Version = %q, want %q", got.GetVersion(), "1.0.0")
		}
		if got.GetSelectedTarget() != "1.21.4" {
			t.Errorf("SelectedTarget = %q, want %q", got.GetSelectedTarget(), "1.21.4")
		}
		if !got.GetSelectedTargetPinned() {
			t.Errorf("SelectedTargetPinned = %v, want true", got.GetSelectedTargetPinned())
		}
		if got.GetIp() == nil {
			t.Fatalf("Ip should not be nil")
		}
		if got.GetIp().GetAddress() != "192.168.1.1" {
			t.Errorf("Ip.Address = %q, want %q", got.GetIp().GetAddress(), "192.168.1.1")
		}
	})

	t.Run("nil Game relation uses empty game name", func(t *testing.T) {
		input := &models.GameServer{
			ID:     "gs-2",
			Status: "OFFLINE",
		}
		input.R.User = &models.User{UserName: "testuser"}
		input.R.Node = &models.Node{Name: "Node-2"}

		got := GameServerModelToProto(input, nil)
		if got.GetGameName() != "" {
			t.Errorf("GameName = %q, want empty string", got.GetGameName())
		}
	})

	t.Run("nil User relation uses empty user name", func(t *testing.T) {
		input := &models.GameServer{
			ID:     "gs-3",
			Status: "OFFLINE",
		}
		input.R.Game = &models.Game{Name: "Minecraft"}
		input.R.Node = &models.Node{Name: "Node-3"}

		got := GameServerModelToProto(input, nil)
		if got.GetUserName() != "" {
			t.Errorf("UserName = %q, want empty string", got.GetUserName())
		}
	})

	t.Run("nil Node relation creates empty node", func(t *testing.T) {
		input := &models.GameServer{
			ID:     "gs-4",
			Status: "OFFLINE",
		}

		got := GameServerModelToProto(input, nil)
		if got.GetNodeName() != "" {
			t.Errorf("NodeName = %q, want empty string", got.GetNodeName())
		}
		if got.GetNodeHost() != "" {
			t.Errorf("NodeHost = %q, want empty string", got.GetNodeHost())
		}
		if got.GetNodePort() != 0 {
			t.Errorf("NodePort = %d, want 0", got.GetNodePort())
		}
	})
}

func TestGameServerModelToSetter(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &models.GameServer{
		ID:             "gs-1",
		UserID:         "user-1",
		Name:           "My Server",
		Status:         "ONLINE",
		Port:           27015,
		BackupsEnabled: true,
		NodeID:         "node-1",
		MaxMemoryMB:    4096,
		Branch:         "1.21.4",
		TargetPinned:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	setter := GameServerModelToSetter(input)

	gotID, okID := setter.ID.Get()
	if !okID {
		t.Fatalf("ID should be set")
	}
	if gotID != "gs-1" {
		t.Errorf("ID = %q, want %q", gotID, "gs-1")
	}

	gotName, okName := setter.Name.Get()
	if !okName {
		t.Fatalf("Name should be set")
	}
	if gotName != "My Server" {
		t.Errorf("Name = %q, want %q", gotName, "My Server")
	}

	gotStatus, okStatus := setter.Status.Get()
	if !okStatus {
		t.Fatalf("Status should be set")
	}
	if gotStatus != "ONLINE" {
		t.Errorf("Status = %q, want %q", gotStatus, "ONLINE")
	}

	gotPort, okPort := setter.Port.Get()
	if !okPort {
		t.Fatalf("Port should be set")
	}
	if gotPort != 27015 {
		t.Errorf("Port = %d, want %d", gotPort, 27015)
	}

	gotBackups, okBackups := setter.BackupsEnabled.Get()
	if !okBackups {
		t.Fatalf("BackupsEnabled should be set")
	}
	if !gotBackups {
		t.Errorf("BackupsEnabled = %v, want true", gotBackups)
	}

	gotNodeID, okNodeID := setter.NodeID.Get()
	if !okNodeID {
		t.Fatalf("NodeID should be set")
	}
	if gotNodeID != "node-1" {
		t.Errorf("NodeID = %q, want %q", gotNodeID, "node-1")
	}

	gotTargetPinned, okTargetPinned := setter.TargetPinned.Get()
	if !okTargetPinned {
		t.Fatalf("TargetPinned should be set")
	}
	if !gotTargetPinned {
		t.Errorf("TargetPinned = %v, want true", gotTargetPinned)
	}
}

func TestGameModelToProto(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &models.Game{
		ID:                                "game-1",
		Name:                              "Minecraft",
		DefaultPort:                       25565,
		DefaultQueryPort:                  25566,
		DefaultMaxPlayers:                 20,
		LinuxSupport:                      true,
		WindowsSupport:                    true,
		LinuxInstallCommandType:           "bash",
		LinuxUpdateCommandType:            "direct",
		WindowsInstallCommandType:         "powershell",
		WindowsUpdateCommandType:          "cmd",
		RequireDedicatedIP:                true,
		CreatedAt:                         now,
		UpdatedAt:                         now,
		UsesSourceQuery:                   true,
		RequiresSteamGameServerLoginToken: true,
		UsesSteamcmd:                      true,
		SteamAppID:                        "294420",
		XylonaOfficial:                    true,
	}

	got := GameModelToProto(input)

	if got.GetId() != "game-1" {
		t.Errorf("Id = %q, want %q", got.GetId(), "game-1")
	}
	if got.GetName() != "Minecraft" {
		t.Errorf("Name = %q, want %q", got.GetName(), "Minecraft")
	}
	if got.GetDefaultPort() != 25565 {
		t.Errorf("DefaultPort = %d, want %d", got.GetDefaultPort(), 25565)
	}
	if got.GetLinuxInstallType() != xylona.CommandType_COMMAND {
		t.Errorf("LinuxInstallType = %v, want COMMAND", got.GetLinuxInstallType())
	}
	if got.GetLinuxInstallCommandProcessor() != xylona.CommandProcessor_BASH {
		t.Errorf("LinuxInstallCommandProcessor = %v, want BASH", got.GetLinuxInstallCommandProcessor())
	}
	if got.GetLinuxUpdateType() != xylona.CommandType_NONE {
		t.Errorf("LinuxUpdateType = %v, want NONE", got.GetLinuxUpdateType())
	}
	if got.GetLinuxUpdateCommandProcessor() != xylona.CommandProcessor_DIRECT {
		t.Errorf("LinuxUpdateCommandProcessor = %v, want DIRECT", got.GetLinuxUpdateCommandProcessor())
	}
	if got.GetWindowsInstallType() != xylona.CommandType_COMMAND {
		t.Errorf("WindowsInstallType = %v, want COMMAND", got.GetWindowsInstallType())
	}
	if got.GetWindowsInstallCommandProcessor() != xylona.CommandProcessor_POWERSHELL {
		t.Errorf("WindowsInstallCommandProcessor = %v, want POWERSHELL", got.GetWindowsInstallCommandProcessor())
	}
	if got.GetWindowsUpdateType() != xylona.CommandType_COMMAND {
		t.Errorf("WindowsUpdateType = %v, want COMMAND", got.GetWindowsUpdateType())
	}
	if got.GetWindowsUpdateCommandProcessor() != xylona.CommandProcessor_CMD {
		t.Errorf("WindowsUpdateCommandProcessor = %v, want CMD", got.GetWindowsUpdateCommandProcessor())
	}
	if !got.GetRequireDedicatedIp() {
		t.Errorf("RequireDedicatedIp = %v, want true", got.GetRequireDedicatedIp())
	}
	if !got.GetUsesSourceQuery() {
		t.Errorf("UsesSourceQuery = %v, want true", got.GetUsesSourceQuery())
	}
	if !got.GetRequiresSteamGameServerLoginToken() {
		t.Errorf("RequiresSteamGameServerLoginToken = %v, want true", got.GetRequiresSteamGameServerLoginToken())
	}
	if !got.GetUsesSteamcmd() {
		t.Errorf("UsesSteamcmd = %v, want true", got.GetUsesSteamcmd())
	}
	if got.GetSteamAppid() != "294420" {
		t.Errorf("SteamAppid = %q, want %q", got.GetSteamAppid(), "294420")
	}
	if !got.GetXylonaOfficial() {
		t.Errorf("XylonaOfficial = %v, want true", got.GetXylonaOfficial())
	}
}

func TestGameProtoToModel(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &xylona.Game{
		Id:                                "game-1",
		Name:                              "Minecraft",
		DefaultPort:                       25565,
		LinuxInstallType:                  xylona.CommandType_COMMAND,
		LinuxInstallCommandProcessor:      xylona.CommandProcessor_BASH,
		LinuxUpdateType:                   xylona.CommandType_COMMAND,
		LinuxUpdateCommandProcessor:       xylona.CommandProcessor_DIRECT,
		WindowsInstallType:                xylona.CommandType_COMMAND,
		WindowsInstallCommandProcessor:    xylona.CommandProcessor_POWERSHELL,
		WindowsUpdateType:                 xylona.CommandType_COMMAND,
		WindowsUpdateCommandProcessor:     xylona.CommandProcessor_CMD,
		RequireDedicatedIp:                true,
		CreatedAt:                         timestamppb.New(now),
		UpdatedAt:                         timestamppb.New(now),
		UsesSourceQuery:                   true,
		RequiresSteamGameServerLoginToken: true,
		UsesSteamcmd:                      true,
		SteamAppid:                        "294420",
	}

	got := GameProtoToModel(input)

	if got.ID != "game-1" {
		t.Errorf("ID = %q, want %q", got.ID, "game-1")
	}
	if got.LinuxInstallCommandType != "bash" {
		t.Errorf("LinuxInstallCommandType = %q, want %q", got.LinuxInstallCommandType, "bash")
	}
	if got.LinuxUpdateCommandType != "direct" {
		t.Errorf("LinuxUpdateCommandType = %q, want %q", got.LinuxUpdateCommandType, "direct")
	}
	if got.WindowsInstallCommandType != "powershell" {
		t.Errorf("WindowsInstallCommandType = %q, want %q", got.WindowsInstallCommandType, "powershell")
	}
	if got.WindowsUpdateCommandType != "cmd" {
		t.Errorf("WindowsUpdateCommandType = %q, want %q", got.WindowsUpdateCommandType, "cmd")
	}
	if !got.RequireDedicatedIP {
		t.Errorf("RequireDedicatedIP = %v, want true", got.RequireDedicatedIP)
	}
	if !got.UsesSourceQuery {
		t.Errorf("UsesSourceQuery = %v, want true", got.UsesSourceQuery)
	}
	if !got.UsesSteamcmd {
		t.Errorf("UsesSteamcmd = %v, want true", got.UsesSteamcmd)
	}
	if got.SteamAppID != "294420" {
		t.Errorf("SteamAppID = %q, want %q", got.SteamAppID, "294420")
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
}

func TestCommandTypeRoundtrip(t *testing.T) {
	tests := []struct {
		name          string
		commandType   string
		wantProcessor xylona.CommandProcessor
	}{
		{name: "direct", commandType: "direct", wantProcessor: xylona.CommandProcessor_DIRECT},
		{name: "bash", commandType: "bash", wantProcessor: xylona.CommandProcessor_BASH},
		{name: "cmd", commandType: "cmd", wantProcessor: xylona.CommandProcessor_CMD},
		{name: "powershell", commandType: "powershell", wantProcessor: xylona.CommandProcessor_POWERSHELL},
		{name: "internal", commandType: "internal", wantProcessor: xylona.CommandProcessor_XYLONA_INTERNAL},
		{name: "unknown defaults to direct", commandType: "unknown_type", wantProcessor: xylona.CommandProcessor_DIRECT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gameModel := &models.Game{LinuxInstallCommandType: tt.commandType}
			proto := GameModelToProto(gameModel)
			if proto.GetLinuxInstallCommandProcessor() != tt.wantProcessor {
				t.Errorf("commandTypeToCommandProcessor(%q) = %v, want %v",
					tt.commandType, proto.GetLinuxInstallCommandProcessor(), tt.wantProcessor)
			}

			gameProto := &xylona.Game{
				LinuxInstallType:             xylona.CommandType_COMMAND,
				LinuxInstallCommandProcessor: tt.wantProcessor,
				CreatedAt:                    timestamppb.Now(),
				UpdatedAt:                    timestamppb.Now(),
			}
			model := GameProtoToModel(gameProto)
			wantType := tt.commandType
			if tt.commandType == "unknown_type" {
				wantType = "direct"
			}
			if model.LinuxInstallCommandType != wantType {
				t.Errorf("commandProcessorToCommandType(%v) = %q, want %q",
					tt.wantProcessor, model.LinuxInstallCommandType, wantType)
			}
		})
	}
}

func TestGameModelToProtoMapsInstallAndUpdateTypes(t *testing.T) {
	tests := []struct {
		name                 string
		modelType            string
		updateProvider       *xylona.UpdateProviderConfig
		wantType             xylona.CommandType
		wantCommandProcessor xylona.CommandProcessor
	}{
		{
			name:                 "none",
			modelType:            "none",
			wantType:             xylona.CommandType_NONE,
			wantCommandProcessor: xylona.CommandProcessor_DIRECT,
		},
		{
			name:                 "steamcmd",
			modelType:            "steamcmd",
			wantType:             xylona.CommandType_STEAMCMD,
			wantCommandProcessor: xylona.CommandProcessor_DIRECT,
		},
		{
			name:                 "papermc",
			modelType:            "papermc",
			wantType:             xylona.CommandType_PAPERMC,
			wantCommandProcessor: xylona.CommandProcessor_DIRECT,
		},
		{
			name:                 "mojang",
			modelType:            "mojang",
			wantType:             xylona.CommandType_MOJANG,
			wantCommandProcessor: xylona.CommandProcessor_DIRECT,
		},
		{
			name:                 "internal mojang keeps internal processor",
			modelType:            "internal",
			updateProvider:       &xylona.UpdateProviderConfig{Kind: xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_MOJANG},
			wantType:             xylona.CommandType_MOJANG,
			wantCommandProcessor: xylona.CommandProcessor_XYLONA_INTERNAL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gameModel := &models.Game{
				LinuxInstallCommandType: tt.modelType,
			}

			if tt.updateProvider != nil {
				errSave := updateproviders.SaveGameConfigToModel(gameModel, updateproviders.GameConfig{
					UpdateProvider: updateproviders.ProviderConfig{
						Kind: updateproviders.ProviderKindMojang,
					},
				})
				if errSave != nil {
					t.Fatalf("SaveGameConfigToModel() error = %v", errSave)
				}
			}

			got := GameModelToProto(gameModel)
			if got.GetLinuxInstallType() != tt.wantType {
				t.Fatalf("LinuxInstallType = %v, want %v", got.GetLinuxInstallType(), tt.wantType)
			}
			if got.GetLinuxInstallCommandProcessor() != tt.wantCommandProcessor {
				t.Fatalf(
					"LinuxInstallCommandProcessor = %v, want %v",
					got.GetLinuxInstallCommandProcessor(),
					tt.wantCommandProcessor,
				)
			}
		})
	}
}

func TestGameProtoToModelMapsInstallAndUpdateTypes(t *testing.T) {
	tests := []struct {
		name            string
		inputType       xylona.CommandType
		inputProcessor  xylona.CommandProcessor
		wantCommandType string
	}{
		{
			name:            "command bash",
			inputType:       xylona.CommandType_COMMAND,
			inputProcessor:  xylona.CommandProcessor_BASH,
			wantCommandType: "bash",
		},
		{
			name:            "none",
			inputType:       xylona.CommandType_NONE,
			inputProcessor:  xylona.CommandProcessor_DIRECT,
			wantCommandType: "direct",
		},
		{
			name:            "steamcmd",
			inputType:       xylona.CommandType_STEAMCMD,
			inputProcessor:  xylona.CommandProcessor_DIRECT,
			wantCommandType: "direct",
		},
		{
			name:            "papermc",
			inputType:       xylona.CommandType_PAPERMC,
			inputProcessor:  xylona.CommandProcessor_DIRECT,
			wantCommandType: "internal",
		},
		{
			name:            "mojang",
			inputType:       xylona.CommandType_MOJANG,
			inputProcessor:  xylona.CommandProcessor_DIRECT,
			wantCommandType: "internal",
		},
		{
			name:            "command internal preserves internal",
			inputType:       xylona.CommandType_COMMAND,
			inputProcessor:  xylona.CommandProcessor_XYLONA_INTERNAL,
			wantCommandType: "internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := timestamppb.Now()
			gameProto := &xylona.Game{
				LinuxInstallType:             tt.inputType,
				LinuxInstallCommandProcessor: tt.inputProcessor,
				CreatedAt:                    now,
				UpdatedAt:                    now,
			}

			got := GameProtoToModel(gameProto)
			if got.LinuxInstallCommandType != tt.wantCommandType {
				t.Fatalf("LinuxInstallCommandType = %q, want %q", got.LinuxInstallCommandType, tt.wantCommandType)
			}
		})
	}
}

func TestGameModelToGameSetter(t *testing.T) {
	input := &models.Game{
		ID:                      "game-1",
		Name:                    "Minecraft",
		DefaultPort:             25565,
		UsesSteamcmd:            true,
		SteamAppID:              "123456",
		LinuxInstallCommandType: "bash",
	}

	setter := GameModelToGameSetter(input)

	gotID, okID := setter.ID.Get()
	if !okID {
		t.Fatalf("ID should be set")
	}
	if gotID != "game-1" {
		t.Errorf("ID = %q, want %q", gotID, "game-1")
	}

	gotSteamcmd, okSteamcmd := setter.UsesSteamcmd.Get()
	if !okSteamcmd {
		t.Fatalf("UsesSteamcmd should be set")
	}
	if !gotSteamcmd {
		t.Errorf("UsesSteamcmd = %v, want true", gotSteamcmd)
	}

	gotSteamAppID, okSteamAppID := setter.SteamAppID.Get()
	if !okSteamAppID {
		t.Fatalf("SteamAppID should be set")
	}
	if gotSteamAppID != "123456" {
		t.Errorf("SteamAppID = %q, want %q", gotSteamAppID, "123456")
	}

	// CreatedAt and UpdatedAt are set to time.Now() in the function
	gotCreatedAt, okCreatedAt := setter.CreatedAt.Get()
	if !okCreatedAt {
		t.Fatalf("CreatedAt should be set")
	}
	if gotCreatedAt.IsZero() {
		t.Errorf("CreatedAt should not be zero")
	}
}

func TestUserModelToProto(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &models.User{
		ID:           "user-1",
		UserName:     "admin",
		Email:        "admin@example.com",
		FirstName:    "John",
		LastName:     "Doe",
		SuperUser:    true,
		PasswordHash: "hashed-password",
		LastLoginAt:  now,
		CreatedAt:    now,
	}

	got := UserModelToProto(input)

	if got.GetId() != "user-1" {
		t.Errorf("Id = %q, want %q", got.GetId(), "user-1")
	}
	if got.GetUserName() != "admin" {
		t.Errorf("UserName = %q, want %q", got.GetUserName(), "admin")
	}
	if got.GetEmail() != "admin@example.com" {
		t.Errorf("Email = %q, want %q", got.GetEmail(), "admin@example.com")
	}
	if !got.GetSuperUser() {
		t.Errorf("SuperUser = %v, want true", got.GetSuperUser())
	}
	if got.GetLastLogin() == nil {
		t.Fatalf("LastLogin should not be nil")
	}
	if !got.GetLastLogin().AsTime().Equal(now) {
		t.Errorf("LastLogin = %v, want %v", got.GetLastLogin().AsTime(), now)
	}
}

func TestUserProtoToModel(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	input := &xylona.User{
		Id:        "user-1",
		UserName:  "admin",
		Email:     "admin@example.com",
		FirstName: "John",
		LastName:  "Doe",
		SuperUser: true,
		LastLogin: timestamppb.New(now),
		CreatedAt: timestamppb.New(now),
	}

	got := UserProtoToModel(input)

	if got.ID != "user-1" {
		t.Errorf("ID = %q, want %q", got.ID, "user-1")
	}
	if got.UserName != "admin" {
		t.Errorf("UserName = %q, want %q", got.UserName, "admin")
	}
	if got.PasswordHash != "" {
		t.Errorf("PasswordHash = %q, want empty string", got.PasswordHash)
	}
	if !got.LastLoginAt.Equal(now) {
		t.Errorf("LastLoginAt = %v, want %v", got.LastLoginAt, now)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
}

func TestGameServerModelToProtoVersionInfo(t *testing.T) {
	baseInput := func() *models.GameServer {
		gs := &models.GameServer{
			ID:     "gs-vi-1",
			Status: "OFFLINE",
		}
		gs.R.Node = &models.Node{Name: "Node-1"}
		return gs
	}

	t.Run("nil vsm leaves VersionInfo nil", func(t *testing.T) {
		got := GameServerModelToProto(baseInput(), nil)
		if got.GetVersionInfo() != nil {
			t.Errorf("VersionInfo = %v, want nil when vsm is nil", got.GetVersionInfo())
		}
	})

	t.Run("vsm present with no tracker status", func(t *testing.T) {
		vsm := versiontracker.NewVersionStateMap()
		vsm.InitNoTracker("gs-vi-1")
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_NO_TRACKER {
			t.Errorf("Status = %v, want VERSION_STATUS_NO_TRACKER", got.GetVersionInfo().GetStatus())
		}
	})

	t.Run("vsm present with unchecked status", func(t *testing.T) {
		vsm := versiontracker.NewVersionStateMap()
		vsm.InitUnchecked("gs-vi-1", "dummy")
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_UNCHECKED {
			t.Errorf("Status = %v, want VERSION_STATUS_UNCHECKED", got.GetVersionInfo().GetStatus())
		}
		if got.GetVersionInfo().GetTrackerType() != "dummy" {
			t.Errorf("TrackerType = %q, want %q", got.GetVersionInfo().GetTrackerType(), "dummy")
		}
	})

	t.Run("vsm present with checked status populates all fields", func(t *testing.T) {
		checkTime := time.Now().Truncate(time.Second)
		vsm := versiontracker.NewVersionStateMap()
		vsm.Set("gs-vi-1", versiontracker.VersionState{
			Status:           versiontracker.VersionStatusChecked,
			InstalledVersion: "1.0.0",
			LatestVersion:    "1.1.0",
			UpdateAvailable:  true,
			LastCheckTime:    checkTime,
			TrackerType:      "minecraft",
		})
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKED {
			t.Errorf("Status = %v, want VERSION_STATUS_CHECKED", got.GetVersionInfo().GetStatus())
		}
		if got.GetVersionInfo().GetInstalledVersion() != "1.0.0" {
			t.Errorf("InstalledVersion = %q, want %q", got.GetVersionInfo().GetInstalledVersion(), "1.0.0")
		}
		if got.GetVersionInfo().GetLatestVersion() != "1.1.0" {
			t.Errorf("LatestVersion = %q, want %q", got.GetVersionInfo().GetLatestVersion(), "1.1.0")
		}
		if !got.GetVersionInfo().GetUpdateAvailable() {
			t.Errorf("UpdateAvailable = %v, want true", got.GetVersionInfo().GetUpdateAvailable())
		}
		if got.GetVersionInfo().GetLastCheckTime() != checkTime.Unix() {
			t.Errorf("LastCheckTime = %d, want %d", got.GetVersionInfo().GetLastCheckTime(), checkTime.Unix())
		}
		if got.GetVersionInfo().GetTrackerType() != "minecraft" {
			t.Errorf("TrackerType = %q, want %q", got.GetVersionInfo().GetTrackerType(), "minecraft")
		}
	})

	t.Run("vsm present with error status", func(t *testing.T) {
		vsm := versiontracker.NewVersionStateMap()
		vsm.Set("gs-vi-1", versiontracker.VersionState{
			Status:      versiontracker.VersionStatusError,
			TrackerType: "steam",
		})
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_ERROR {
			t.Errorf("Status = %v, want VERSION_STATUS_ERROR", got.GetVersionInfo().GetStatus())
		}
	})

	t.Run("vsm present with checking status", func(t *testing.T) {
		vsm := versiontracker.NewVersionStateMap()
		vsm.Set("gs-vi-1", versiontracker.VersionState{
			Status:      versiontracker.VersionStatusChecking,
			TrackerType: "steam",
		})
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetStatus() != xylona.VersionStatus_VERSION_STATUS_CHECKING {
			t.Errorf("Status = %v, want VERSION_STATUS_CHECKING", got.GetVersionInfo().GetStatus())
		}
	})

	t.Run("zero LastCheckTime yields zero unix timestamp", func(t *testing.T) {
		vsm := versiontracker.NewVersionStateMap()
		vsm.Set("gs-vi-1", versiontracker.VersionState{
			Status:        versiontracker.VersionStatusChecked,
			LastCheckTime: time.Time{},
		})
		got := GameServerModelToProto(baseInput(), vsm)
		if got.GetVersionInfo() == nil {
			t.Fatalf("VersionInfo should not be nil when vsm is provided")
		}
		if got.GetVersionInfo().GetLastCheckTime() != 0 {
			t.Errorf("LastCheckTime = %d, want 0 for zero time", got.GetVersionInfo().GetLastCheckTime())
		}
	})
}

func TestIPModelToProto(t *testing.T) {
	tests := []struct {
		name         string
		input        *models.IP
		wantAddress  string
		wantUsable   bool
		wantExternal bool
		wantNodeID   string
	}{
		{
			name:         "all fields set",
			input:        &models.IP{Address: "192.168.1.1", Usable: true, External: false, NodeID: "node-1"},
			wantAddress:  "192.168.1.1",
			wantUsable:   true,
			wantExternal: false,
			wantNodeID:   "node-1",
		},
		{
			name:         "external IP",
			input:        &models.IP{Address: "8.8.8.8", Usable: true, External: true, NodeID: "node-2"},
			wantAddress:  "8.8.8.8",
			wantUsable:   true,
			wantExternal: true,
			wantNodeID:   "node-2",
		},
		{
			name:         "unusable IP",
			input:        &models.IP{Address: "10.0.0.1", Usable: false, External: false, NodeID: "node-3"},
			wantAddress:  "10.0.0.1",
			wantUsable:   false,
			wantExternal: false,
			wantNodeID:   "node-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IPModelToProto(tt.input)
			if got.GetAddress() != tt.wantAddress {
				t.Errorf("Address = %q, want %q", got.GetAddress(), tt.wantAddress)
			}
			if got.GetUsable() != tt.wantUsable {
				t.Errorf("Usable = %v, want %v", got.GetUsable(), tt.wantUsable)
			}
			if got.GetExternal() != tt.wantExternal {
				t.Errorf("External = %v, want %v", got.GetExternal(), tt.wantExternal)
			}
			if got.GetNodeId() != tt.wantNodeID {
				t.Errorf("NodeId = %q, want %q", got.GetNodeId(), tt.wantNodeID)
			}
		})
	}
}
