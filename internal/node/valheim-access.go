package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const valheimAccessFileLimit = 1 << 20

var valheimIdentityPattern = regexp.MustCompile(gameintegrations.ValheimIdentityPattern)

// ValheimAccessList is a complete bounded snapshot of stored access data.
type ValheimAccessList struct {
	ListKind    string
	Identities  []string
	Revision    string
	Diagnostics []string
	Missing     bool
}

func readValheimAccess(root *os.Root, path, kind string) ([]byte, *ValheimAccessList, error) {
	snapshot := &ValheimAccessList{ListKind: kind}
	info, errStat := root.Lstat(path)
	if errStat != nil && !errors.Is(errStat, os.ErrNotExist) {
		return nil, nil, errors.New("stored access file could not be inspected")
	}
	if errStat == nil && !info.Mode().IsRegular() {
		return nil, nil, errors.New("stored access requires a regular file; links and special files are preserved")
	}
	file, errOpen := root.Open(path)
	var data []byte
	switch {
	case errors.Is(errOpen, os.ErrNotExist):
		snapshot.Missing = true
		snapshot.Diagnostics = append(snapshot.Diagnostics, "Stored file is missing; adding an identity creates it.")
	case errOpen != nil:
		return nil, nil, errors.New("stored access file could not be opened")
	default:
		var errRead error
		data, errRead = io.ReadAll(io.LimitReader(file, valheimAccessFileLimit+1))
		errClose := file.Close()
		if errRead != nil || errClose != nil {
			return nil, nil, errors.New("stored access file could not be read")
		}
	}
	if len(data) > valheimAccessFileLimit || bytes.Count(data, []byte("\n")) > 4096 {
		return nil, nil, errors.New("stored access file exceeds the panel limit of 1 MiB or 4096 lines; no partial list is returned")
	}
	if !utf8.Valid(data) || bytes.ContainsAny(data, "\x00") {
		return nil, nil, errors.New("stored access encoding is unsupported; UTF-8 is required and the file was preserved")
	}
	digestData := append([]byte{0}, data...)
	if snapshot.Missing {
		digestData[0] = 1
	}
	digest := sha256.Sum256(digestData)
	snapshot.Revision = hex.EncodeToString(digest[:])
	for index, line := range strings.Split(strings.TrimPrefix(string(data), "\uFEFF"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if valheimIdentityPattern.MatchString(line) {
			snapshot.Identities = append(snapshot.Identities, line)
		} else if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "//") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			snapshot.Diagnostics = append(snapshot.Diagnostics, fmt.Sprintf("Line %d preserved as unknown text: %s", index+1, line))
		}
	}
	return data, snapshot, nil
}

func (n *Node) executeValheimAccessOperation(ctx context.Context, request GameOperationRequest) GameOperationResult {
	kind, action, found := gameintegrations.ValheimAccessOperation(request.OperationID)
	if !found || request.GameServerID == "" || request.WorkingDirectory == "" {
		return failedGameOperation("A known Valheim operation and owning server identity are required.")
	}
	identity, revision := "", ""
	for _, value := range request.Values {
		if value.StringValue == nil || value.BooleanValue != nil || value.IntegerValue != nil {
			return failedGameOperation("Stored access inputs must be strings.")
		}
		switch value.FieldID {
		case "player":
			if identity != "" || !valheimIdentityPattern.MatchString(*value.StringValue) {
				return failedGameOperation("Use one exact Steam_ identity followed by 17 digits.")
			}
			identity = *value.StringValue
		case "expected_revision":
			decoded, errDecode := hex.DecodeString(*value.StringValue)
			if revision != "" || errDecode != nil || len(decoded) != sha256.Size {
				return failedGameOperation("Read the stored list to obtain its revision before changing it.")
			}
			revision = *value.StringValue
		default:
			return failedGameOperation("Unknown stored access input.")
		}
	}
	if action == "list" && len(request.Values) != 0 || action != "list" && (identity == "" || revision == "") {
		return failedGameOperation("Listing takes no inputs; changes require an exact identity and expected revision.")
	}
	unlock, errLock := n.lockServerLifecycle(request.WorkingDirectory)
	if errLock != nil {
		return failedGameOperation("The server configuration guard is unavailable.")
	}
	defer unlock()
	if ctx.Err() != nil {
		return failedGameOperation("The stored access operation was canceled.")
	}
	if action != "list" {
		process, tracked, errProcess := n.GetProcessSnapshot(request.GameServerID)
		if errProcess != nil || tracked && (process == nil || process.Status != xylona.Status_OFFLINE.String()) {
			return failedGameOperation("Stop the server before changing stored access.")
		}
	}
	root, errRoot := openMutationRoot(request.WorkingDirectory)
	if errRoot != nil {
		return failedGameOperation("The managed server directory is unavailable.")
	}
	defer closeMutationRoot(root)
	path := map[string]string{"administrators": "data/adminlist.txt", "bans": "data/bannedlist.txt", "permitted": "data/permittedlist.txt"}[kind]
	data, snapshot, errRead := readValheimAccess(root, path, kind)
	if errRead != nil {
		return failedGameOperation(errRead.Error())
	}
	result := GameOperationResult{Classification: GameOperationResultConfirmed, Message: "Stored list read back. In-game enforcement is not verified.", ValheimAccessList: snapshot, TransportDetails: GameOperationTransportDetails{Method: "Valheim stored access file", Verification: "Stored bytes read back"}}
	if action == "list" {
		return result
	}
	if snapshot.Revision != revision {
		return failedGameOperation("The stored list changed. Read it again and review the change.")
	}
	updated := mutateValheimAccess(data, identity, action)
	if len(updated) > valheimAccessFileLimit || bytes.Count(updated, []byte("\n")) > 4096 {
		return failedGameOperation("The change would exceed the stored list limit of 1 MiB or 4096 lines.")
	}
	if bytes.Equal(updated, data) {
		return result
	}
	errDirectory := root.MkdirAll("data", 0o750)
	if errDirectory != nil {
		return failedGameOperation("The managed data directory could not be created.")
	}
	_, latest, errLatest := readValheimAccess(root, path, kind)
	if errLatest != nil || latest.Revision != revision {
		return failedGameOperation("The stored list changed before replacement. Read it again.")
	}
	_, errWrite := writeFileFromReaderAtRoot(root, path, bytes.NewReader(updated), 0o600)
	if errWrite != nil {
		return failedGameOperation("The stored access file could not be replaced.")
	}
	readback, confirmed, errReadback := readValheimAccess(root, path, kind)
	if errReadback != nil || !bytes.Equal(readback, updated) {
		result.Classification = GameOperationResultAcceptedButUnverified
		result.Message = "Stored file replacement completed, but readback could not confirm it. Read the list again before retrying."
		result.ValheimAccessList = nil
		return result
	}
	result.ValheimAccessList = confirmed
	result.Message = "Stored file updated and read back. Applies on next start; in-game enforcement and empty permitted-list behavior are not verified."
	return result
}

func mutateValheimAccess(data []byte, identity, action string) []byte {
	bom := ""
	text := string(data)
	withoutBOM, hasBOM := strings.CutPrefix(text, "\uFEFF")
	if hasBOM {
		bom, text = "\uFEFF", withoutBOM
	}
	lines := strings.SplitAfter(text, "\n")
	found := false
	var kept strings.Builder
	kept.WriteString(bom)
	for _, line := range lines {
		if strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r") == identity {
			found = true
			if action == "remove" {
				continue
			}
		}
		kept.WriteString(line)
	}
	if action == "add" && !found {
		newline := "\n"
		if strings.Contains(text, "\r\n") {
			newline = "\r\n"
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			kept.WriteString(newline)
		}
		kept.WriteString(identity)
		if strings.HasSuffix(text, "\n") || text == "" {
			kept.WriteString(newline)
		}
	}
	return []byte(kept.String())
}
