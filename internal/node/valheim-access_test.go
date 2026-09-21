package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValheimStoredAccess(t *testing.T) {
	for _, kind := range []string{"administrators", "bans", "permitted"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			n := &Node{}
			request := GameOperationRequest{GameID: "valheim", GameServerID: "test", WorkingDirectory: directory, OperationID: "valheim.access." + kind + ".list"}
			result := n.ExecuteGameOperation(context.Background(), request)
			if result.Classification != GameOperationResultConfirmed || !result.ValheimAccessList.Missing {
				t.Fatalf("missing list: %+v", result)
			}
			revision := result.ValheimAccessList.Revision
			identity := "Steam_00000000000000000"
			request.OperationID = "valheim.access." + kind + ".add"
			request.Values = []GameOperationValue{{FieldID: "player", StringValue: &identity}, {FieldID: "expected_revision", StringValue: &revision}}
			result = n.ExecuteGameOperation(context.Background(), request)
			if result.Classification != GameOperationResultConfirmed || len(result.ValheimAccessList.Identities) != 1 {
				t.Fatalf("add: %+v", result)
			}
			stale := n.ExecuteGameOperation(context.Background(), request)
			if stale.Classification != GameOperationResultFailed {
				t.Fatalf("stale revision accepted: %+v", stale)
			}
			revision = result.ValheimAccessList.Revision
			duplicate := n.ExecuteGameOperation(context.Background(), request)
			if duplicate.Classification != GameOperationResultConfirmed || duplicate.ValheimAccessList.Revision != revision {
				t.Fatalf("duplicate was not idempotent: %+v", duplicate)
			}
			request.OperationID = "valheim.access." + kind + ".remove"
			result = n.ExecuteGameOperation(context.Background(), request)
			if result.Classification != GameOperationResultConfirmed || len(result.ValheimAccessList.Identities) != 0 {
				t.Fatalf("remove: %+v", result)
			}
		})
	}
}

func TestValheimAccessPreservesUnknownBytes(t *testing.T) {
	identity := "Steam_00000000000000000"
	for _, ending := range []string{"\n", "\r\n"} {
		original := "\uFEFF// comment" + ending + "unknown_identity" + ending + ending + identity
		removed := mutateValheimAccess([]byte(original), identity, "remove")
		want := "\uFEFF// comment" + ending + "unknown_identity" + ending + ending
		if string(removed) != want {
			t.Fatalf("preserved bytes = %q, want %q", removed, want)
		}
		if string(mutateValheimAccess([]byte(original), identity, "add")) != original {
			t.Fatal("duplicate add altered bytes")
		}
	}
}

func TestValheimAccessRejectsInvalidStoredData(t *testing.T) {
	for _, content := range []string{"\xff", "hello\x00world", strings.Repeat("x", valheimAccessFileLimit+1)} {
		directory := t.TempDir()
		errWrite := os.WriteFile(filepath.Join(directory, "list.txt"), []byte(content), 0o600)
		if errWrite != nil {
			t.Fatal(errWrite)
		}
		root, errRoot := os.OpenRoot(directory)
		if errRoot != nil {
			t.Fatal(errRoot)
		}
		_, _, errRead := readValheimAccess(root, "list.txt", "administrators")
		errClose := root.Close()
		if errClose != nil {
			t.Fatal(errClose)
		}
		if errRead == nil {
			t.Fatal("invalid stored data accepted")
		}
	}
}

func TestValheimAccessRejectsMalformedIdentity(t *testing.T) {
	for _, identity := range []string{"123", "steam_00000000000000000", "Steam_00000000000000000\n", "../Steam_00000000000000000", "Steam_00000000000000000\x00"} {
		n := &Node{}
		revision := strings.Repeat("0", 64)
		result := n.ExecuteGameOperation(context.Background(), GameOperationRequest{
			GameID: "valheim", GameServerID: "test", WorkingDirectory: t.TempDir(), OperationID: "valheim.access.administrators.add",
			Values: []GameOperationValue{{FieldID: "player", StringValue: &identity}, {FieldID: "expected_revision", StringValue: &revision}},
		})
		if result.Classification != GameOperationResultFailed {
			t.Fatalf("malformed identity %q accepted", identity)
		}
	}
}
