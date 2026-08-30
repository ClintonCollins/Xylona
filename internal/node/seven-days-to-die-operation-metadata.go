package node

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
)

const (
	sevenDaysToDieOperationMetadataFileLimit = 16 << 20
	sevenDaysToDieItemIconLimit              = 20_000
)

var sevenDaysToDiePlayerIdentityRE = regexp.MustCompile(gameintegrations.PlayerIdentityPattern)

// QuerySevenDaysToDieOperationMetadata reads bounded operation choices from server-owned files.
func (n *Node) QuerySevenDaysToDieOperationMetadata(
	ctx context.Context,
	req SevenDaysToDieOperationMetadataQueryRequest,
) (result *SevenDaysToDieOperationMetadata, err error) {
	workingDirectory := strings.TrimSpace(req.WorkingDirectory)
	if workingDirectory == "" {
		return nil, ErrInvalidPath
	}
	root, errOpen := os.OpenRoot(workingDirectory)
	if errOpen != nil {
		return nil, fmt.Errorf("open 7 Days to Die server root: %w", errOpen)
	}
	defer func() {
		errClose := root.Close()
		if err == nil && errClose != nil {
			result = nil
			err = fmt.Errorf("close 7 Days to Die server root: %w", errClose)
		}
	}()

	metadata := new(SevenDaysToDieOperationMetadata)
	gameName := ""
	adminFileName := "serveradmin.xml"
	configData, errConfig := readOperationMetadataFile(root, "serverconfig.xml")
	if errConfig == nil {
		parsedGameName, parsedAdminFileName, errParseConfig := parseSevenDaysToDieServerConfiguration(ctx, configData)
		if errParseConfig == nil {
			gameName = parsedGameName
			adminFileName = parsedAdminFileName
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read operation server configuration: %w", errContext)
	}

	adminData, errAdmin := readOperationMetadataFile(root, filepath.Join("Saves", adminFileName))
	playerLabels := make(map[string]string)
	if errAdmin == nil {
		parsedPlayerLabels, parsedCommands, errParseAdmin := parseSevenDaysToDieServerAdmin(ctx, adminData)
		if errParseAdmin == nil {
			playerLabels = parsedPlayerLabels
			metadata.Commands = parsedCommands
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read operation command metadata: %w", errContext)
	}

	activeSavePath, errSave := newestSevenDaysToDieSave(root, gameName)
	if errSave != nil {
		activeSavePath = ""
	}
	if activeSavePath != "" {
		players, errPlayers := sevenDaysToDieSavedPlayers(root, activeSavePath, playerLabels)
		if errPlayers == nil {
			metadata.Players = players
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read saved player metadata: %w", errContext)
	}

	icons, errIcons := sevenDaysToDieItemIcons(root)
	if errIcons != nil {
		icons = nil
	}
	localization := make(map[string]string)
	localizationData, errLocalization := readOperationMetadataFile(root, filepath.Join("Data", "Config", "Localization.txt"))
	if errLocalization == nil {
		parsedLocalization, errParseLocalization := parseSevenDaysToDieLocalization(ctx, localizationData)
		if errParseLocalization == nil {
			localization = parsedLocalization
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read operation localization metadata: %w", errContext)
	}
	itemsPath := filepath.Join("Data", "Config", "items.xml")
	buffsPath := filepath.Join("Data", "Config", "buffs.xml")
	if activeSavePath != "" {
		itemsPath = preferOperationMetadataPath(root, filepath.Join(activeSavePath, "ConfigsDump", "items.xml"), itemsPath)
		buffsPath = preferOperationMetadataPath(root, filepath.Join(activeSavePath, "ConfigsDump", "buffs.xml"), buffsPath)
	}
	itemsData, errItems := readOperationMetadataFile(root, itemsPath)
	if errItems == nil {
		items, errParseItems := parseSevenDaysToDieItems(ctx, itemsData, icons, localization)
		if errParseItems == nil {
			metadata.Items = items
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read operation item metadata: %w", errContext)
	}
	buffsData, errBuffs := readOperationMetadataFile(root, buffsPath)
	if errBuffs == nil {
		buffs, errParseBuffs := parseSevenDaysToDieBuffs(ctx, buffsData, localization)
		if errParseBuffs == nil {
			metadata.Buffs = buffs
		}
	}
	if errContext := ctx.Err(); errContext != nil {
		return nil, fmt.Errorf("read operation buff metadata: %w", errContext)
	}

	return metadata, nil
}

func readOperationMetadataFile(root *os.Root, name string) ([]byte, error) {
	file, errOpen := root.Open(name)
	if errOpen != nil {
		return nil, fmt.Errorf("open %q: %w", name, errOpen)
	}
	data, errRead := io.ReadAll(io.LimitReader(file, sevenDaysToDieOperationMetadataFileLimit+1))
	errClose := file.Close()
	if errRead != nil {
		return nil, fmt.Errorf("read %q: %w", name, errRead)
	}
	if errClose != nil {
		return nil, fmt.Errorf("close %q: %w", name, errClose)
	}
	if len(data) > sevenDaysToDieOperationMetadataFileLimit {
		return nil, fmt.Errorf("%q exceeds the metadata file limit", name)
	}
	return data, nil
}

func readOperationMetadataDirectory(root *os.Root, name string) ([]os.DirEntry, error) {
	directory, errOpen := root.Open(name)
	if errOpen != nil {
		return nil, fmt.Errorf("open directory %q: %w", name, errOpen)
	}
	entries, errRead := directory.ReadDir(-1)
	errClose := directory.Close()
	if errRead != nil {
		return nil, fmt.Errorf("read directory %q: %w", name, errRead)
	}
	if errClose != nil {
		return nil, fmt.Errorf("close directory %q: %w", name, errClose)
	}
	return entries, nil
}

func preferOperationMetadataPath(root *os.Root, preferred, fallback string) string {
	_, errStat := root.Stat(preferred)
	if errStat == nil {
		return preferred
	}
	return fallback
}

func parseSevenDaysToDieServerConfiguration(ctx context.Context, data []byte) (string, string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	gameName := ""
	adminFileName := "serveradmin.xml"
	for {
		token, errToken := decoder.Token()
		if errors.Is(errToken, io.EOF) {
			return gameName, adminFileName, nil
		}
		if errToken != nil {
			return "", "serveradmin.xml", fmt.Errorf("decode server configuration: %w", errToken)
		}
		select {
		case <-ctx.Done():
			return "", "serveradmin.xml", fmt.Errorf("parse server configuration: %w", ctx.Err())
		default:
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "property" {
			continue
		}
		switch xmlAttribute(start.Attr, "name") {
		case "GameName":
			gameName = boundedOperationMetadataValue(xmlAttribute(start.Attr, "value"))
		case "AdminFileName":
			configured := safeSevenDaysToDieAdminFileName(xmlAttribute(start.Attr, "value"))
			if configured != "" {
				adminFileName = configured
			}
		}
	}
}

func safeSevenDaysToDieAdminFileName(value string) string {
	value = boundedOperationMetadataValue(value)
	cleaned := filepath.Clean(value)
	if value == "" || cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return ""
	}
	return cleaned
}

func newestSevenDaysToDieSave(root *os.Root, gameName string) (string, error) {
	if gameName == "" || filepath.Base(gameName) != gameName {
		return "", nil
	}
	worlds, errWorlds := readOperationMetadataDirectory(root, "Saves")
	if errors.Is(errWorlds, os.ErrNotExist) {
		return "", nil
	}
	if errWorlds != nil {
		return "", errWorlds
	}
	newestPath := ""
	newestModified := time.Time{}
	for _, world := range worlds {
		if !world.IsDir() {
			continue
		}
		worldPath := filepath.Join("Saves", world.Name())
		saves, errSaves := readOperationMetadataDirectory(root, worldPath)
		if errSaves != nil {
			return "", errSaves
		}
		for _, save := range saves {
			if !save.IsDir() || save.Name() != gameName {
				continue
			}
			savePath := filepath.Join(worldPath, save.Name())
			info, errInfo := save.Info()
			if errInfo != nil {
				return "", fmt.Errorf("stat save %q: %w", savePath, errInfo)
			}
			modified := info.ModTime()
			mainInfo, errMainInfo := root.Stat(filepath.Join(savePath, "main.ttw"))
			if errMainInfo == nil {
				modified = mainInfo.ModTime()
			} else if !errors.Is(errMainInfo, os.ErrNotExist) {
				return "", fmt.Errorf("stat save state %q: %w", savePath, errMainInfo)
			}
			if newestPath == "" || modified.After(newestModified) {
				newestPath = savePath
				newestModified = modified
			}
		}
	}
	return newestPath, nil
}

func sevenDaysToDieSavedPlayers(
	root *os.Root,
	savePath string,
	labels map[string]string,
) ([]SevenDaysToDieOperationOption, error) {
	entries, errEntries := readOperationMetadataDirectory(root, filepath.Join(savePath, "Player"))
	if errors.Is(errEntries, os.ErrNotExist) {
		return nil, nil
	}
	if errEntries != nil {
		return nil, errEntries
	}
	options := make([]SevenDaysToDieOperationOption, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".ttp") {
			continue
		}
		identity := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !sevenDaysToDiePlayerIdentityRE.MatchString(identity) {
			continue
		}
		label := strings.TrimSpace(labels[identity])
		if label == "" {
			label = identity
		}
		options = append(options, SevenDaysToDieOperationOption{
			Label: label, Value: identity, Description: "Saved player",
		})
	}
	return sortedUniqueOperationOptions(options), nil
}

func sevenDaysToDieItemIcons(root *os.Root) (map[string]struct{}, error) {
	entries, errEntries := readOperationMetadataDirectory(root, filepath.Join("Data", "ItemIcons"))
	if errEntries != nil {
		return nil, errEntries
	}
	icons := make(map[string]struct{}, min(len(entries), sevenDaysToDieItemIconLimit))
	for _, entry := range entries {
		if len(icons) >= sevenDaysToDieItemIconLimit {
			break
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}
		icons[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))] = struct{}{}
	}
	return icons, nil
}

func parseSevenDaysToDieItems(
	ctx context.Context,
	data []byte,
	icons map[string]struct{},
	localization map[string]string,
) ([]SevenDaysToDieOperationOption, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	options := make([]SevenDaysToDieOperationOption, 0, 1024)
	currentName := ""
	currentIcon := ""
	currentCategory := ""
	for len(options) < SevenDaysToDieOperationOptionCountLimit {
		token, errToken := decoder.Token()
		if errors.Is(errToken, io.EOF) {
			break
		}
		if errToken != nil {
			return nil, fmt.Errorf("decode items: %w", errToken)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("parse items: %w", ctx.Err())
		default:
		}
		switch typed := token.(type) {
		case xml.StartElement:
			switch typed.Name.Local {
			case "item":
				currentName = boundedOperationMetadataValue(xmlAttribute(typed.Attr, "name"))
				currentIcon = currentName
				currentCategory = ""
			case "property":
				if currentName == "" {
					continue
				}
				switch xmlAttribute(typed.Attr, "name") {
				case "CustomIcon":
					currentIcon = boundedOperationMetadataValue(xmlAttribute(typed.Attr, "value"))
				case "Group":
					category := strings.NewReplacer("/", " / ", ",", " · ").Replace(xmlAttribute(typed.Attr, "value"))
					currentCategory = boundedOperationMetadataValue(category)
				}
			}
		case xml.EndElement:
			if typed.Name.Local != "item" || currentName == "" {
				continue
			}
			if _, found := icons[currentIcon]; !found {
				currentIcon = ""
			}
			label := localization[currentName]
			description := ""
			if label == "" {
				label = currentName
			} else if label != currentName {
				description = currentName
			}
			options = append(options, SevenDaysToDieOperationOption{
				Label: label, Value: currentName, Description: description, IconName: currentIcon,
				Category: currentCategory,
			})
			currentName = ""
			currentIcon = ""
			currentCategory = ""
		}
	}
	return sortedUniqueOperationOptions(options), nil
}

func parseSevenDaysToDieBuffs(
	ctx context.Context,
	data []byte,
	localization map[string]string,
) ([]SevenDaysToDieOperationOption, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	options := make([]SevenDaysToDieOperationOption, 0, 512)
	for len(options) < SevenDaysToDieOperationOptionCountLimit {
		token, errToken := decoder.Token()
		if errors.Is(errToken, io.EOF) {
			break
		}
		if errToken != nil {
			return nil, fmt.Errorf("decode buffs: %w", errToken)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("parse buffs: %w", ctx.Err())
		default:
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "buff" {
			continue
		}
		name := boundedOperationMetadataValue(xmlAttribute(start.Attr, "name"))
		if name == "" {
			continue
		}
		nameKey := boundedOperationMetadataValue(xmlAttribute(start.Attr, "name_key"))
		label := localization[nameKey]
		if label == "" {
			label = localization[name]
		}
		description := ""
		if label == "" {
			label = name
		} else if label != name {
			description = name
		}
		category := ""
		if strings.EqualFold(xmlAttribute(start.Attr, "hidden"), "true") {
			category = "Hidden / internal"
		}
		options = append(options, SevenDaysToDieOperationOption{
			Label: label, Value: name, Description: description, Category: category,
			AccentColor: sevenDaysToDieOperationAccentColor(xmlAttribute(start.Attr, "icon_color")),
		})
	}
	return sortedUniqueOperationOptions(options), nil
}

func sevenDaysToDieOperationAccentColor(value string) string {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return ""
	}
	var channels [3]int
	for index, part := range parts {
		channel, errChannel := strconv.Atoi(strings.TrimSpace(part))
		if errChannel != nil || channel < 0 || channel > 255 {
			return ""
		}
		channels[index] = channel
	}
	return fmt.Sprintf("#%02x%02x%02x", channels[0], channels[1], channels[2])
}

func parseSevenDaysToDieLocalization(ctx context.Context, data []byte) (map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	header, errHeader := reader.Read()
	if errHeader != nil {
		return nil, fmt.Errorf("decode localization header: %w", errHeader)
	}
	keyIndex := -1
	englishIndex := -1
	for index, name := range header {
		name = strings.TrimPrefix(name, "\uFEFF")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "key":
			keyIndex = index
		case "english":
			englishIndex = index
		}
	}
	if keyIndex < 0 || englishIndex < 0 {
		return nil, errors.New("localization header is missing Key or english")
	}
	localization := make(map[string]string)
	for len(localization) < SevenDaysToDieOperationOptionCountLimit*4 {
		record, errRecord := reader.Read()
		if errors.Is(errRecord, io.EOF) {
			break
		}
		if errRecord != nil {
			return nil, fmt.Errorf("decode localization: %w", errRecord)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("parse localization: %w", ctx.Err())
		default:
		}
		if keyIndex >= len(record) || englishIndex >= len(record) {
			continue
		}
		key := boundedOperationMetadataValue(record[keyIndex])
		value := boundedOperationMetadataValue(record[englishIndex])
		if key != "" && value != "" {
			localization[key] = value
		}
	}
	return localization, nil
}

func parseSevenDaysToDieServerAdmin(
	ctx context.Context,
	data []byte,
) (map[string]string, []SevenDaysToDieOperationOption, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	players := make(map[string]string)
	commands := make([]SevenDaysToDieOperationOption, 0, 128)
	for len(commands) < SevenDaysToDieOperationOptionCountLimit {
		token, errToken := decoder.Token()
		if errors.Is(errToken, io.EOF) {
			break
		}
		if errToken != nil {
			return nil, nil, fmt.Errorf("decode server administration: %w", errToken)
		}
		select {
		case <-ctx.Done():
			return nil, nil, fmt.Errorf("parse server administration: %w", ctx.Err())
		default:
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "permission" {
			command := boundedOperationMetadataValue(xmlAttribute(start.Attr, "cmd"))
			if command != "" {
				commands = append(commands, SevenDaysToDieOperationOption{
					Label: command, Value: command, Description: "Configured command",
				})
			}
			continue
		}
		if start.Name.Local != "user" && start.Name.Local != "blacklisted" {
			continue
		}
		platform := boundedOperationMetadataValue(xmlAttribute(start.Attr, "platform"))
		userID := boundedOperationMetadataValue(xmlAttribute(start.Attr, "userid"))
		identity := platform + "_" + userID
		if platform == "" || userID == "" || !sevenDaysToDiePlayerIdentityRE.MatchString(identity) {
			continue
		}
		name := boundedOperationMetadataValue(xmlAttribute(start.Attr, "name"))
		if name != "" {
			players[identity] = name
		}
	}
	return players, sortedUniqueOperationOptions(commands), nil
}

func xmlAttribute(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func boundedOperationMetadataValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > SevenDaysToDieOperationOptionFieldByteLimit || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

func sortedUniqueOperationOptions(options []SevenDaysToDieOperationOption) []SevenDaysToDieOperationOption {
	byValue := make(map[string]SevenDaysToDieOperationOption, len(options))
	for _, option := range options {
		if option.Value == "" {
			continue
		}
		byValue[option.Value] = option
	}
	result := make([]SevenDaysToDieOperationOption, 0, len(byValue))
	for _, option := range byValue {
		result = append(result, option)
	}
	slices.SortFunc(result, func(left, right SevenDaysToDieOperationOption) int {
		labelOrder := cmp.Compare(strings.ToLower(left.Label), strings.ToLower(right.Label))
		if labelOrder != 0 {
			return labelOrder
		}
		return cmp.Compare(left.Value, right.Value)
	})
	return result
}
