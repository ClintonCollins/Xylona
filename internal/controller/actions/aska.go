package actions

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	askaGameID         = "aska"
	askaPropertiesPath = "server properties.txt"
	askaGSLTSetting    = "authentication token"
)

func (inst *Instance) writeGameLaunchSecrets(
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
	secretVars map[string]string,
) error {
	if gameServer == nil || gameServer.GameID != askaGameID {
		return nil
	}
	if client == nil {
		return errors.New("ASKA node client is nil")
	}

	token := strings.TrimSpace(secretVars[steamGSLTPlaceholder])
	if token == "" {
		return errors.New("ASKA Steam GSLT is not configured")
	}
	properties, errRead := client.ReadFile(inst.ctx, gameServer.Directory, askaPropertiesPath)
	if errRead != nil {
		return fmt.Errorf("read ASKA server properties: %w", errRead)
	}
	patched, errPatch := patchKeyValueSetting(properties, askaGSLTSetting, token)
	if errPatch != nil {
		return fmt.Errorf("patch ASKA server properties: %w", errPatch)
	}
	errWrite := client.WriteFile(
		inst.ctx,
		gameServer.Directory,
		askaPropertiesPath,
		patched,
		node.ProtectionPolicy{},
	)
	if errWrite != nil {
		return fmt.Errorf("write ASKA server properties: %w", errWrite)
	}
	return nil
}

func patchKeyValueSetting(data []byte, key string, value string) ([]byte, error) {
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return nil, errors.New("setting key is empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		return nil, errors.New("setting value contains a line break")
	}

	lineEnding := []byte("\n")
	if bytes.Contains(data, []byte("\r\n")) {
		lineEnding = []byte("\r\n")
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	hadTrailingLineEnding := strings.HasSuffix(normalized, "\n")
	body := strings.TrimSuffix(normalized, "\n")
	lines := []string{}
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	found := false
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "//") ||
			strings.HasPrefix(trimmedLine, "#") || strings.HasPrefix(trimmedLine, ";") {
			continue
		}
		equalsIndex := strings.Index(line, "=")
		if equalsIndex < 0 || !strings.EqualFold(strings.TrimSpace(line[:equalsIndex]), trimmedKey) {
			continue
		}
		valueIndex := equalsIndex + 1
		for valueIndex < len(line) && (line[valueIndex] == ' ' || line[valueIndex] == '\t') {
			valueIndex++
		}
		lines[i] = line[:valueIndex] + value
		found = true
		break
	}
	if !found {
		lines = append(lines, trimmedKey+" = "+value)
	}

	patched := strings.Join(lines, string(lineEnding))
	if hadTrailingLineEnding || len(data) == 0 {
		patched += string(lineEnding)
	}
	return []byte(patched), nil
}
