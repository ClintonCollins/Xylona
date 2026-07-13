package rpc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/launchenv"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type importGameChangeBuilder struct {
	changes []*xylona.GameImportChange
}

type importConfigSchemaPropertyField struct {
	key   string
	label string
}

var configSchemaPropertyFields = []importConfigSchemaPropertyField{
	{key: "type", label: "type"},
	{key: "title", label: "title"},
	{key: "description", label: "description"},
	{key: "default", label: "default"},
	{key: "enum", label: "enum"},
	{key: "x-enum-labels", label: "enum labels"},
	{key: "minimum", label: "minimum"},
	{key: "maximum", label: "maximum"},
	{key: "maxLength", label: "max length"},
	{key: "required", label: "required"},
	{key: "x-allow-multiple", label: "allow multiple"},
	{key: "x-group", label: "group"},
	{key: "x-order", label: "order"},
	{key: "x-managed", label: "managed source"},
}

func importGameChanges(existing *models.Game, imported *models.Game) ([]*xylona.GameImportChange, error) {
	existingConfig, errExistingConfig := updateconfig.LoadGameConfigFromModel(existing)
	if errExistingConfig != nil {
		return nil, fmt.Errorf("load existing game config: %w", errExistingConfig)
	}
	importedConfig, errImportedConfig := updateconfig.LoadGameConfigFromModel(imported)
	if errImportedConfig != nil {
		return nil, fmt.Errorf("load imported game config: %w", errImportedConfig)
	}

	builder := &importGameChangeBuilder{}
	builder.addString("General", "Name", "game.name", existing.Name, imported.Name)
	builder.addInt("General", "Default port", "game.defaultPort", existing.DefaultPort, imported.DefaultPort)
	builder.addInt("General", "Default query port", "game.defaultQueryPort", existing.DefaultQueryPort, imported.DefaultQueryPort)
	builder.addInt("General", "Default max players", "game.defaultMaxPlayers", existing.DefaultMaxPlayers, imported.DefaultMaxPlayers)
	builder.addBool("General", "Bind to all IPs", "game.bindsToAllIps", existing.BindsToAllIps, imported.BindsToAllIps)
	builder.addBool("General", "Uses Source query", "game.usesSourceQuery", existing.UsesSourceQuery, imported.UsesSourceQuery)
	builder.addBool("General", "Uses SteamCMD", "game.usesSteamcmd", existing.UsesSteamcmd, imported.UsesSteamcmd)
	builder.addString("General", "Steam app ID", "game.steamAppid", existing.SteamAppID, imported.SteamAppID)
	builder.addBool("General", "Requires Steam GSLT", "game.requiresSteamGameServerLoginToken", existing.RequiresSteamGameServerLoginToken, imported.RequiresSteamGameServerLoginToken)
	builder.addBool("General", "Allow start argument editing", "game.allowStartArgEditing", existing.AllowStartArgEditing, imported.AllowStartArgEditing)

	builder.addBool("Linux", "Linux support", "game.linuxSupport", existing.LinuxSupport, imported.LinuxSupport)
	builder.addString("Linux", "Stop command", "game.linuxStopCommand", existing.LinuxStopCommand, imported.LinuxStopCommand)
	builder.addString("Linux", "Install command", "game.linuxInstallCommand", existing.LinuxInstallCommand, imported.LinuxInstallCommand)
	builder.addString("Linux", "Install type", "game.linuxInstallType", existing.LinuxInstallCommandType, imported.LinuxInstallCommandType)
	builder.addString("Linux", "Update command", "game.linuxUpdateCommand", existing.LinuxUpdateCommand, imported.LinuxUpdateCommand)
	builder.addString("Linux", "Update type", "game.linuxUpdateType", existing.LinuxUpdateCommandType, imported.LinuxUpdateCommandType)
	builder.addString("Linux", "Working directory", "game.linuxWorkingDirectory", existing.LinuxWorkingDirectory, imported.LinuxWorkingDirectory)
	builder.addString("Linux", "Base command", "game.linuxBaseCommand", existing.LinuxBaseCommand, imported.LinuxBaseCommand)

	builder.addBool("Windows", "Windows support", "game.windowsSupport", existing.WindowsSupport, imported.WindowsSupport)
	builder.addString("Windows", "Stop command", "game.windowsStopCommand", existing.WindowsStopCommand, imported.WindowsStopCommand)
	builder.addString("Windows", "Install command", "game.windowsInstallCommand", existing.WindowsInstallCommand, imported.WindowsInstallCommand)
	builder.addString("Windows", "Install type", "game.windowsInstallType", existing.WindowsInstallCommandType, imported.WindowsInstallCommandType)
	builder.addString("Windows", "Update command", "game.windowsUpdateCommand", existing.WindowsUpdateCommand, imported.WindowsUpdateCommand)
	builder.addString("Windows", "Update type", "game.windowsUpdateType", existing.WindowsUpdateCommandType, imported.WindowsUpdateCommandType)
	builder.addString("Windows", "Working directory", "game.windowsWorkingDirectory", existing.WindowsWorkingDirectory, imported.WindowsWorkingDirectory)
	builder.addString("Windows", "Base command", "game.windowsBaseCommand", existing.WindowsBaseCommand, imported.WindowsBaseCommand)

	errConfigSchemas := builder.addConfigSchemas(existing.ConfigSchemas.GetOr(""), imported.ConfigSchemas.GetOr(""))
	if errConfigSchemas != nil {
		return nil, errConfigSchemas
	}
	errLinuxStartArgs := builder.addStartArgs("Linux start arguments", "linux_start_args_template", existing.LinuxStartArgsTemplate.GetOr(""), imported.LinuxStartArgsTemplate.GetOr(""))
	if errLinuxStartArgs != nil {
		return nil, errLinuxStartArgs
	}
	errWindowsStartArgs := builder.addStartArgs("Windows start arguments", "windows_start_args_template", existing.WindowsStartArgsTemplate.GetOr(""), imported.WindowsStartArgsTemplate.GetOr(""))
	if errWindowsStartArgs != nil {
		return nil, errWindowsStartArgs
	}
	errBlocklist := builder.addStartArgBlocklist(existing.StartArgBlocklist, imported.StartArgBlocklist)
	if errBlocklist != nil {
		return nil, errBlocklist
	}
	errDefaultEnv := builder.addDefaultEnv(existing.DefaultEnvVars, imported.DefaultEnvVars)
	if errDefaultEnv != nil {
		return nil, errDefaultEnv
	}

	builder.addString("Updates", "Update provider", "update_config.update_provider", providerConfigSummary(existingConfig.UpdateProvider), providerConfigSummary(importedConfig.UpdateProvider))
	builder.addString("Updates", "Default target", "update_config.default_target", existingConfig.DefaultTarget, importedConfig.DefaultTarget)
	builder.addModProfile(existingConfig.ModProfile, importedConfig.ModProfile)
	builder.addVariants(existingConfig.Variants, importedConfig.Variants)

	return builder.changes, nil
}

func (builder *importGameChangeBuilder) addString(section string, label string, path string, previous string, imported string) {
	builder.add(section, label, path, formatStringValue(previous), formatStringValue(imported))
}

func (builder *importGameChangeBuilder) addBool(section string, label string, path string, previous bool, imported bool) {
	builder.add(section, label, path, formatBoolValue(previous), formatBoolValue(imported))
}

func (builder *importGameChangeBuilder) addInt(section string, label string, path string, previous int64, imported int64) {
	builder.add(section, label, path, fmt.Sprintf("%d", previous), fmt.Sprintf("%d", imported))
}

func (builder *importGameChangeBuilder) add(section string, label string, path string, previous string, imported string) {
	if previous == imported {
		return
	}
	builder.append(section, label, path, previous, imported)
}

func (builder *importGameChangeBuilder) addDetected(section string, label string, path string, previous string, imported string) {
	if previous == imported {
		imported += " (details changed)"
	}
	builder.append(section, label, path, previous, imported)
}

func (builder *importGameChangeBuilder) append(section string, label string, path string, previous string, imported string) {
	builder.changes = append(builder.changes, &xylona.GameImportChange{
		Section:       section,
		Label:         label,
		Path:          path,
		PreviousValue: previous,
		ImportedValue: imported,
	})
}

func (builder *importGameChangeBuilder) addConfigSchemaEntryChanges(previous []map[string]any, imported []map[string]any) {
	importedByKey := configSchemaEntryIndexes(imported)
	processed := map[string]struct{}{}

	for previousIndex, previousEntry := range previous {
		key := configSchemaEntryKey(previousEntry, previousIndex)
		importedIndex, found := importedByKey[key]
		if !found {
			label := "Config file " + configSchemaDisplayName(previousEntry, nil, previousIndex, -1)
			path := "config_schemas." + configSchemaPathSegment(previousEntry, previousIndex)
			builder.append("Configuration", label, path, configSchemaEntrySummary(previousEntry), "Missing")
			continue
		}

		importedEntry := imported[importedIndex]
		if previousIndex != importedIndex {
			displayName := configSchemaDisplayName(previousEntry, importedEntry, previousIndex, importedIndex)
			path := "config_schemas." + configSchemaPathSegment(previousEntry, previousIndex) + ".order"
			builder.append("Configuration", "Config file "+displayName+" order", path, fmt.Sprintf("%d", previousIndex+1), fmt.Sprintf("%d", importedIndex+1))
		}
		builder.addConfigSchemaEntryDiff(previousEntry, importedEntry, previousIndex, importedIndex)
		processed[key] = struct{}{}
	}

	for importedIndex, importedEntry := range imported {
		key := configSchemaEntryKey(importedEntry, importedIndex)
		_, found := processed[key]
		if found {
			continue
		}

		label := "Config file " + configSchemaDisplayName(nil, importedEntry, -1, importedIndex)
		path := "config_schemas." + configSchemaPathSegment(importedEntry, importedIndex)
		builder.append("Configuration", label, path, "Missing", configSchemaEntrySummary(importedEntry))
	}
}

func (builder *importGameChangeBuilder) addConfigSchemaEntryDiff(previous map[string]any, imported map[string]any, previousIndex int, importedIndex int) {
	displayName := configSchemaDisplayName(previous, imported, previousIndex, importedIndex)
	pathPrefix := "config_schemas." + configSchemaPathSegment(previous, previousIndex)
	if previous == nil {
		pathPrefix = "config_schemas." + configSchemaPathSegment(imported, importedIndex)
	}

	builder.addConfigJSONValue(displayName+" format", pathPrefix+".format", previous["format"], imported["format"])
	builder.addConfigJSONValue(displayName+" category", pathPrefix+".category", previous["category"], imported["category"])
	builder.addConfigJSONValue(displayName+" generate before start", pathPrefix+".generate_before_start", previous["generate_before_start"], imported["generate_before_start"])
	builder.addConfigJSONValue(displayName+" managed fields", pathPrefix+".managed_fields", previous["managed_fields"], imported["managed_fields"])
	builder.addConfigJSONValue(displayName+" XML key mode", pathPrefix+".xml_key_mode", previous["xml_key_mode"], imported["xml_key_mode"])

	previousSchema := jsonMapValue(previous, "schema")
	importedSchema := jsonMapValue(imported, "schema")
	builder.addConfigJSONValue(displayName+" schema type", pathPrefix+".schema.type", previousSchema["type"], importedSchema["type"])
	builder.addConfigJSONValue(displayName+" schema groups", pathPrefix+".schema.x-groups", previousSchema["x-groups"], importedSchema["x-groups"])
	builder.addConfigSchemaPropertyChanges(displayName, pathPrefix, jsonMapValue(previousSchema, "properties"), jsonMapValue(importedSchema, "properties"))
}

func (builder *importGameChangeBuilder) addConfigSchemaPropertyChanges(schemaLabel string, pathPrefix string, previousProperties map[string]any, importedProperties map[string]any) {
	propertyKeys := sortedJSONMapKeys(previousProperties, importedProperties)
	for _, propertyKey := range propertyKeys {
		previousProperty := jsonMapValue(previousProperties, propertyKey)
		importedProperty := jsonMapValue(importedProperties, propertyKey)
		propertyLabel := schemaLabel + " > " + configSchemaPropertyDisplayName(propertyKey, previousProperty, importedProperty)
		propertyPath := pathPrefix + ".schema.properties." + propertyKey

		if previousProperty == nil {
			builder.append("Configuration", propertyLabel+" field", propertyPath, "Missing", configSchemaPropertySummary(importedProperty))
			continue
		}
		if importedProperty == nil {
			builder.append("Configuration", propertyLabel+" field", propertyPath, configSchemaPropertySummary(previousProperty), "Missing")
			continue
		}

		for _, field := range configSchemaPropertyFields {
			builder.addConfigJSONValue(propertyLabel+" "+field.label, propertyPath+"."+field.key, previousProperty[field.key], importedProperty[field.key])
		}
	}
}

func (builder *importGameChangeBuilder) addConfigJSONValue(label string, path string, previous any, imported any) {
	if reflect.DeepEqual(previous, imported) {
		return
	}
	builder.addDetected("Configuration", label, path, formatImportJSONValue(previous), formatImportJSONValue(imported))
}

func (builder *importGameChangeBuilder) addStartArgBlockChanges(label string, path string, previous []startargs.ArgBlock, imported []startargs.ArgBlock) {
	importedByKey := startArgBlockIndexes(imported)
	processed := map[string]struct{}{}

	for previousIndex, previousBlock := range previous {
		key := startArgBlockKey(previousBlock, previousIndex)
		importedIndex, found := importedByKey[key]
		if !found {
			blockLabel := label + " > " + startArgBlockDisplayName(previousBlock, previousIndex) + " block"
			blockPath := path + "." + startArgBlockPathSegment(previousBlock, previousIndex)
			builder.append("Runtime", blockLabel, blockPath, startArgBlockSummary(previousBlock), "Missing")
			continue
		}

		importedBlock := imported[importedIndex]
		builder.addStartArgBlockDiff(label, path, previousBlock, importedBlock, previousIndex)
		processed[key] = struct{}{}
	}

	for importedIndex, importedBlock := range imported {
		key := startArgBlockKey(importedBlock, importedIndex)
		_, found := processed[key]
		if found {
			continue
		}

		blockLabel := label + " > " + startArgBlockDisplayName(importedBlock, importedIndex) + " block"
		blockPath := path + "." + startArgBlockPathSegment(importedBlock, importedIndex)
		builder.append("Runtime", blockLabel, blockPath, "Missing", startArgBlockSummary(importedBlock))
	}
}

func (builder *importGameChangeBuilder) addStartArgBlockDiff(label string, path string, previous startargs.ArgBlock, imported startargs.ArgBlock, index int) {
	blockName := startArgBlockDisplayName(previous, index)
	blockPath := path + "." + startArgBlockPathSegment(previous, index)
	labelPrefix := label + " > " + blockName

	builder.add("Runtime", labelPrefix+" tokens", blockPath+".tokens", formatStartArgTokens(previous.Tokens), formatStartArgTokens(imported.Tokens))
	builder.add("Runtime", labelPrefix+" label", blockPath+".label", formatStringValue(previous.Label), formatStringValue(imported.Label))
	builder.add("Runtime", labelPrefix+" order", blockPath+".order", fmt.Sprintf("%d", previous.Order), fmt.Sprintf("%d", imported.Order))
	builder.add("Runtime", labelPrefix+" ownership", blockPath+".ownership", formatStringValue(string(previous.Ownership)), formatStringValue(string(imported.Ownership)))
	builder.add("Runtime", labelPrefix+" managed source", blockPath+".managed_source", formatStringValue(previous.ManagedSource), formatStringValue(imported.ManagedSource))
}

func (builder *importGameChangeBuilder) addConfigSchemas(previous string, imported string) error {
	previousEntries, errPreviousEntries := decodeJSONMapArray(previous)
	if errPreviousEntries != nil {
		return fmt.Errorf("decode existing config schemas: %w", errPreviousEntries)
	}
	importedEntries, errImportedEntries := decodeJSONMapArray(imported)
	if errImportedEntries != nil {
		return fmt.Errorf("decode imported config schemas: %w", errImportedEntries)
	}
	if reflect.DeepEqual(previousEntries, importedEntries) {
		return nil
	}

	builder.addConfigSchemaEntryChanges(previousEntries, importedEntries)
	return nil
}

func (builder *importGameChangeBuilder) addStartArgs(label string, path string, previous string, imported string) error {
	previousBlocks, errPreviousBlocks := parseStartArgTemplateForImport(previous)
	if errPreviousBlocks != nil {
		return fmt.Errorf("parse existing %s: %w", path, errPreviousBlocks)
	}
	importedBlocks, errImportedBlocks := parseStartArgTemplateForImport(imported)
	if errImportedBlocks != nil {
		return fmt.Errorf("parse imported %s: %w", path, errImportedBlocks)
	}
	if reflect.DeepEqual(previousBlocks, importedBlocks) {
		return nil
	}

	builder.addStartArgBlockChanges(label, path, previousBlocks, importedBlocks)
	return nil
}

func (builder *importGameChangeBuilder) addStartArgBlocklist(previous string, imported string) error {
	previousEntries, errPreviousEntries := parseStartArgBlocklistForImport(previous)
	if errPreviousEntries != nil {
		return fmt.Errorf("parse existing start arg blocklist: %w", errPreviousEntries)
	}
	importedEntries, errImportedEntries := parseStartArgBlocklistForImport(imported)
	if errImportedEntries != nil {
		return fmt.Errorf("parse imported start arg blocklist: %w", errImportedEntries)
	}
	if reflect.DeepEqual(previousEntries, importedEntries) {
		return nil
	}

	builder.addDetected("Runtime", "Start argument blocklist", "start_arg_blocklist", blocklistSummary(previousEntries), blocklistSummary(importedEntries))
	return nil
}

func (builder *importGameChangeBuilder) addDefaultEnv(previous string, imported string) error {
	previousVars, errPreviousVars := launchenv.ParseStored(previous)
	if errPreviousVars != nil {
		return fmt.Errorf("parse existing default_env_vars: %w", errPreviousVars)
	}
	importedVars, errImportedVars := launchenv.ParseStored(imported)
	if errImportedVars != nil {
		return fmt.Errorf("parse imported default_env_vars: %w", errImportedVars)
	}
	if reflect.DeepEqual(previousVars, importedVars) {
		return nil
	}

	builder.addDetected("Runtime", "Default environment variables", "default_env_vars", envVarsSummary(previousVars), envVarsSummary(importedVars))
	return nil
}

func (builder *importGameChangeBuilder) addModProfile(previous *updateproviders.ModProfile, imported *updateproviders.ModProfile) {
	if reflect.DeepEqual(previous, imported) {
		return
	}
	builder.addDetected("Mods", "Mod profile", "update_config.mod_profile", modProfileSummary(previous), modProfileSummary(imported))
}

func (builder *importGameChangeBuilder) addVariants(previous []updateproviders.Variant, imported []updateproviders.Variant) {
	if previous == nil {
		previous = []updateproviders.Variant{}
	}
	if imported == nil {
		imported = []updateproviders.Variant{}
	}
	if reflect.DeepEqual(previous, imported) {
		return
	}
	builder.addDetected("Updates", "Variants", "update_config.variants", variantsSummary(previous), variantsSummary(imported))
}

func formatStringValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "Not set"
	}
	return value
}

func formatBoolValue(value bool) string {
	if value {
		return "Enabled"
	}
	return "Disabled"
}

func decodeJSONArray(value string) ([]any, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "null" {
		return []any{}, nil
	}

	var decoded []any
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	errDecode := decoder.Decode(&decoded)
	if errDecode != nil {
		return nil, fmt.Errorf("decode JSON array: %w", errDecode)
	}
	return decoded, nil
}

func decodeJSONMapArray(value string) ([]map[string]any, error) {
	decoded, errDecoded := decodeJSONArray(value)
	if errDecoded != nil {
		return nil, errDecoded
	}

	entries := make([]map[string]any, 0, len(decoded))
	for index, entryValue := range decoded {
		entry, ok := entryValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entry %d is %T, want object", index, entryValue)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func configSchemaEntryIndexes(entries []map[string]any) map[string]int {
	indexes := make(map[string]int, len(entries))
	for index, entry := range entries {
		indexes[configSchemaEntryKey(entry, index)] = index
	}
	return indexes
}

func configSchemaEntryKey(entry map[string]any, index int) string {
	path := strings.TrimSpace(jsonStringValue(entry, "path"))
	if path != "" {
		return "path:" + path
	}
	return fmt.Sprintf("index:%d", index)
}

func configSchemaDisplayName(previous map[string]any, imported map[string]any, previousIndex int, importedIndex int) string {
	path := strings.TrimSpace(jsonStringValue(previous, "path"))
	if path == "" {
		path = strings.TrimSpace(jsonStringValue(imported, "path"))
	}
	if path != "" {
		return path
	}
	index := previousIndex
	if index < 0 {
		index = importedIndex
	}
	return fmt.Sprintf("Entry %d", index+1)
}

func configSchemaPathSegment(entry map[string]any, index int) string {
	path := strings.TrimSpace(jsonStringValue(entry, "path"))
	if path != "" {
		return path
	}
	return fmt.Sprintf("entry_%d", index+1)
}

func configSchemaEntrySummary(entry map[string]any) string {
	format := strings.TrimSpace(jsonStringValue(entry, "format"))
	if format == "" {
		format = "config"
	}

	schema := jsonMapValue(entry, "schema")
	properties := jsonMapValue(schema, "properties")
	parts := []string{format, pluralCount(len(properties), "field", "fields")}
	category := strings.TrimSpace(jsonStringValue(entry, "category"))
	if category != "" {
		parts = append(parts, category)
	}
	return strings.Join(parts, ", ")
}

func configSchemaPropertyDisplayName(propertyKey string, previous map[string]any, imported map[string]any) string {
	title := strings.TrimSpace(jsonStringValue(previous, "title"))
	if title == "" {
		title = strings.TrimSpace(jsonStringValue(imported, "title"))
	}
	if title != "" {
		return title
	}
	return propertyKey
}

func configSchemaPropertySummary(property map[string]any) string {
	propertyType := strings.TrimSpace(jsonStringValue(property, "type"))
	if propertyType == "" {
		propertyType = "field"
	}

	parts := []string{propertyType}
	defaultValue, hasDefault := property["default"]
	if hasDefault {
		parts = append(parts, "default "+formatImportJSONValue(defaultValue))
	}
	return strings.Join(parts, ", ")
}

func jsonMapValue(object map[string]any, key string) map[string]any {
	if object == nil {
		return nil
	}
	value, exists := object[key]
	if !exists {
		return nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return mapped
}

func jsonStringValue(object map[string]any, key string) string {
	if object == nil {
		return ""
	}
	value, exists := object[key]
	if !exists {
		return ""
	}
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return stringValue
}

func sortedJSONMapKeys(first map[string]any, second map[string]any) []string {
	keySet := map[string]struct{}{}
	for key := range first {
		keySet[key] = struct{}{}
	}
	for key := range second {
		keySet[key] = struct{}{}
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func formatImportJSONValue(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return "Not set"
	case string:
		if typedValue == "" {
			return `""`
		}
		return typedValue
	case bool:
		return strconv.FormatBool(typedValue)
	case json.Number:
		return typedValue.String()
	case float64:
		return strconv.FormatFloat(typedValue, 'f', -1, 64)
	case []any:
		return formatImportJSONArrayValue(typedValue)
	case map[string]any:
		return formatImportJSONObjectValue(typedValue)
	default:
		return fmt.Sprint(typedValue)
	}
}

func formatImportJSONArrayValue(values []any) string {
	if len(values) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatImportJSONValue(value))
	}
	return strings.Join(parts, ", ")
}

func formatImportJSONObjectValue(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}

	data, errMarshal := json.Marshal(value)
	if errMarshal != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func parseStartArgTemplateForImport(value string) ([]startargs.ArgBlock, error) {
	blocks, errParse := startargs.ParseTemplate(value)
	if errParse != nil {
		return nil, fmt.Errorf("parse start args template: %w", errParse)
	}
	if blocks == nil {
		return []startargs.ArgBlock{}, nil
	}
	return blocks, nil
}

func startArgBlockIndexes(blocks []startargs.ArgBlock) map[string]int {
	indexes := make(map[string]int, len(blocks))
	for index, block := range blocks {
		indexes[startArgBlockKey(block, index)] = index
	}
	return indexes
}

func startArgBlockKey(block startargs.ArgBlock, index int) string {
	id := strings.TrimSpace(block.ID)
	if id != "" {
		return "id:" + id
	}
	return fmt.Sprintf("index:%d", index)
}

func startArgBlockDisplayName(block startargs.ArgBlock, index int) string {
	label := strings.TrimSpace(block.Label)
	if label != "" {
		return label
	}
	id := strings.TrimSpace(block.ID)
	if id != "" {
		return id
	}
	return fmt.Sprintf("Block %d", index+1)
}

func startArgBlockPathSegment(block startargs.ArgBlock, index int) string {
	id := strings.TrimSpace(block.ID)
	if id != "" {
		return id
	}
	return fmt.Sprintf("block_%d", index+1)
}

func startArgBlockSummary(block startargs.ArgBlock) string {
	parts := []string{}
	label := strings.TrimSpace(block.Label)
	if label != "" {
		parts = append(parts, label)
	}
	if block.Order != 0 {
		parts = append(parts, fmt.Sprintf("order %d", block.Order))
	}
	ownership := strings.TrimSpace(string(block.Ownership))
	if ownership != "" {
		parts = append(parts, ownership)
	}
	parts = append(parts, formatStartArgTokens(block.Tokens))
	return strings.Join(parts, ", ")
}

func formatStartArgTokens(tokens []string) string {
	if len(tokens) == 0 {
		return "No tokens"
	}

	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, formatStartArgToken(token))
	}
	return strings.Join(parts, " ")
}

func formatStartArgToken(token string) string {
	if token == "" {
		return `""`
	}
	if strings.ContainsAny(token, " \t\r\n\"") {
		return strconv.Quote(token)
	}
	return token
}

func parseStartArgBlocklistForImport(value string) ([]startargs.BlocklistEntry, error) {
	entries, errParse := startargs.ParseBlocklist(value)
	if errParse != nil {
		return nil, fmt.Errorf("parse start arg blocklist: %w", errParse)
	}
	if entries == nil {
		return []startargs.BlocklistEntry{}, nil
	}
	return entries, nil
}

func blocklistSummary(entries []startargs.BlocklistEntry) string {
	return pluralCount(len(entries), "rule", "rules")
}

func providerConfigSummary(config updateproviders.ProviderConfig) string {
	kind := strings.TrimSpace(string(config.Kind))
	if kind == "" || kind == string(updateproviders.ProviderKindNone) {
		return "None"
	}
	sourceID := strings.TrimSpace(config.SourceID)
	if sourceID == "" {
		return kind
	}
	return kind + " / " + sourceID
}

func modProfileSummary(profile *updateproviders.ModProfile) string {
	if profile == nil {
		return "Off"
	}
	installPath := strings.TrimSpace(profile.InstallPath)
	if installPath == "" {
		installPath = "No install path"
	}
	return fmt.Sprintf("%s, %s", installPath, pluralCount(len(profile.Sources), "source", "sources"))
}

func variantsSummary(variants []updateproviders.Variant) string {
	if len(variants) == 0 {
		return "0 variants"
	}

	names := make([]string, 0, len(variants))
	for _, variant := range variants {
		name := strings.TrimSpace(variant.Name)
		if name == "" {
			name = strings.TrimSpace(variant.ID)
		}
		if name == "" {
			name = "Unnamed"
		}
		names = append(names, name)
	}

	visibleNames := names
	if len(visibleNames) > 3 {
		visibleNames = visibleNames[:3]
	}
	summary := pluralCount(len(variants), "variant", "variants") + ": " + strings.Join(visibleNames, ", ")
	remaining := len(names) - len(visibleNames)
	if remaining > 0 {
		summary += fmt.Sprintf(", +%d more", remaining)
	}
	return summary
}

func envVarsSummary(variables []launchenv.Variable) string {
	return pluralCount(len(variables), "variable", "variables")
}

func pluralCount(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
