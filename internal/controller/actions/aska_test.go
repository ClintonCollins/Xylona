package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPatchKeyValueSetting(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		key       string
		value     string
		want      string
		wantError bool
	}{
		{
			name:  "replaces value and preserves spacing",
			input: "display name = Server\nauthentication token = old-token\n",
			key:   "authentication token",
			value: "new-token",
			want:  "display name = Server\nauthentication token = new-token\n",
		},
		{
			name:  "matches key case insensitively and preserves CRLF",
			input: "Authentication Token\t=\told\r\nregion = US\r\n",
			key:   "authentication token",
			value: "new",
			want:  "Authentication Token\t=\tnew\r\nregion = US\r\n",
		},
		{
			name:  "ignores commented setting and appends active value",
			input: "// authentication token = example\nregion = US",
			key:   "authentication token",
			value: "new-token",
			want:  "// authentication token = example\nregion = US\nauthentication token = new-token",
		},
		{
			name:  "creates setting in empty file",
			input: "",
			key:   "authentication token",
			value: "new-token",
			want:  "authentication token = new-token\n",
		},
		{
			name:      "rejects multiline secret",
			input:     "authentication token = old\n",
			key:       "authentication token",
			value:     "first\nsecond",
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, errPatch := patchKeyValueSetting([]byte(testCase.input), testCase.key, testCase.value)
			if testCase.wantError {
				if errPatch == nil {
					t.Fatal("patchKeyValueSetting() error = nil, want error")
				}
				return
			}
			if errPatch != nil {
				t.Fatalf("patchKeyValueSetting() error = %v", errPatch)
			}
			if string(got) != testCase.want {
				t.Errorf("patchKeyValueSetting() = %q, want %q", string(got), testCase.want)
			}
		})
	}
}

func TestWriteGameLaunchSecretsPatchesASKAProperties(t *testing.T) {
	inst := &Instance{ctx: context.Background()}
	client := &askaFileClientFake{
		readResult: []byte("display name = Example\nauthentication token = old-token\n"),
	}
	gameServer := &models.GameServer{
		ID:        "aska-server",
		GameID:    askaGameID,
		Directory: "C:/servers/aska-server",
	}

	errWrite := inst.writeGameLaunchSecrets(
		gameServer,
		client,
		map[string]string{steamGSLTPlaceholder: "new-token"},
	)
	if errWrite != nil {
		t.Fatalf("writeGameLaunchSecrets() error = %v", errWrite)
	}
	if len(client.readCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(client.readCalls))
	}
	readCall := client.readCalls[0]
	if readCall.directory != gameServer.Directory || readCall.relativePath != askaPropertiesPath {
		t.Errorf("ReadFile call = %+v, want directory %q and path %q", readCall, gameServer.Directory, askaPropertiesPath)
	}
	if len(client.writeCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(client.writeCalls))
	}
	writeCall := client.writeCalls[0]
	if writeCall.directory != gameServer.Directory || writeCall.relativePath != askaPropertiesPath {
		t.Errorf("WriteFile call = %+v, want directory %q and path %q", writeCall, gameServer.Directory, askaPropertiesPath)
	}
	wantContent := "display name = Example\nauthentication token = new-token\n"
	if string(writeCall.content) != wantContent {
		t.Errorf("WriteFile content = %q, want %q", string(writeCall.content), wantContent)
	}
}

type askaFileCall struct {
	directory    string
	relativePath string
	content      []byte
}

type askaFileClientFake struct {
	readResult []byte
	readCalls  []askaFileCall
	writeCalls []askaFileCall
}

func (f *askaFileClientFake) ReadFile(_ context.Context, directory string, relativePath string) ([]byte, error) {
	f.readCalls = append(f.readCalls, askaFileCall{directory: directory, relativePath: relativePath})
	return f.readResult, nil
}

func (f *askaFileClientFake) WriteFile(_ context.Context, directory string, relativePath string, content []byte, _ node.ProtectionPolicy) error {
	f.writeCalls = append(f.writeCalls, askaFileCall{
		directory:    directory,
		relativePath: relativePath,
		content:      append([]byte(nil), content...),
	})
	return nil
}
