package node

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuerySevenDaysToDieOperationMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeMetadataTestFile(t, root, "serverconfig.xml", `<ServerSettings>
  <property name="GameName" value="Survival" />
  <property name="AdminFileName" value="admin/custom.xml" />
</ServerSettings>`)
	writeMetadataTestFile(t, root, filepath.Join("Data", "Config", "items.xml"), `<items><item name="baseOnly" /></items>`)
	writeMetadataTestFile(t, root, filepath.Join("Data", "Config", "buffs.xml"), `<buffs><buff name="baseBuff" /></buffs>`)
	writeMetadataTestFile(t, root, filepath.Join("Data", "Config", "Localization.txt"), "Key,english\nresourceWood,Wood\nbuffWarmName,Warmth\n")
	writeMetadataTestFile(t, root, filepath.Join("Data", "ItemIcons", "resourceWood.png"), "png")
	writeMetadataTestFile(t, root, filepath.Join("Data", "ItemIcons", "customIcon.png"), "png")
	writeMetadataTestFile(t, root, filepath.Join("Saves", "admin", "custom.xml"), `<adminTools>
  <users><user platform="EOS" userid="abc123" name="Alice" /></users>
  <permissions><permission cmd="teleport" permission_level="0" /><permission cmd="saveworld" permission_level="1000" /></permissions>
</adminTools>`)

	olderSave := filepath.Join("Saves", "Old World", "Survival")
	writeMetadataTestFile(t, root, filepath.Join(olderSave, "ConfigsDump", "items.xml"), `<items><item name="oldItem" /></items>`)
	writeMetadataTestFile(t, root, filepath.Join(olderSave, "main.ttw"), "old world")
	newerSave := filepath.Join("Saves", "New World", "Survival")
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "ConfigsDump", "items.xml"), `<items>
	<item name="resourceWood"><property name="Group" value="Resources/Basics,Materials" /></item>
	<item name="customItem"><property name="CustomIcon" value="customIcon" /></item>
</items>`)
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "ConfigsDump", "buffs.xml"), `<buffs>
	<buff name="buffWarm" name_key="buffWarmName" icon_color="255,128,0" />
	<buff name="buffInternal" hidden="true" />
</buffs>`)
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "Player", "EOS_abc123.ttp"), "player")
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "Player", "EOS_abc123.ttp.bak"), "backup")
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "Player", "invalid.ttp"), "invalid")
	writeMetadataTestFile(t, root, filepath.Join(newerSave, "main.ttw"), "new world")
	oldTime := time.Now().Add(-time.Hour)
	errOldTime := os.Chtimes(filepath.Join(root, olderSave, "main.ttw"), oldTime, oldTime)
	if errOldTime != nil {
		t.Fatalf("set old save time: %v", errOldTime)
	}

	metadata, errQuery := new(Node).QuerySevenDaysToDieOperationMetadata(t.Context(), SevenDaysToDieOperationMetadataQueryRequest{
		WorkingDirectory: root,
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieOperationMetadata() error = %v", errQuery)
	}
	if len(metadata.Players) != 1 || metadata.Players[0].Value != "EOS_abc123" || metadata.Players[0].Label != "Alice" {
		t.Fatalf("players = %+v", metadata.Players)
	}
	if len(metadata.Items) != 2 || metadata.Items[0].Value != "customItem" || metadata.Items[0].Label != "customItem" ||
		metadata.Items[0].IconName != "customIcon" || metadata.Items[1].Value != "resourceWood" ||
		metadata.Items[1].Label != "Wood" || metadata.Items[1].Description != "resourceWood" ||
		metadata.Items[1].IconName != "resourceWood" || metadata.Items[1].Category != "Resources / Basics · Materials" {
		t.Fatalf("items = %+v", metadata.Items)
	}
	if len(metadata.Buffs) != 2 || metadata.Buffs[0].Value != "buffInternal" || metadata.Buffs[0].Category != "Hidden / internal" ||
		metadata.Buffs[1].Value != "buffWarm" || metadata.Buffs[1].Label != "Warmth" ||
		metadata.Buffs[1].Description != "buffWarm" || metadata.Buffs[1].AccentColor != "#ff8000" {
		t.Fatalf("buffs = %+v", metadata.Buffs)
	}
	if len(metadata.Commands) != 2 || metadata.Commands[0].Value != "saveworld" || metadata.Commands[1].Value != "teleport" {
		t.Fatalf("commands = %+v", metadata.Commands)
	}
}

func TestQuerySevenDaysToDieOperationMetadataKeepsIndependentCatalogs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeMetadataTestFile(t, root, "serverconfig.xml", `<ServerSettings><property name="GameName" value="Survival" /></ServerSettings>`)
	writeMetadataTestFile(t, root, filepath.Join("Data", "Config", "items.xml"), `<items><item name="broken"></items>`)
	writeMetadataTestFile(t, root, filepath.Join("Data", "Config", "buffs.xml"), `<buffs><buff name="buffWarm" /></buffs>`)
	writeMetadataTestFile(t, root, filepath.Join("Saves", "serveradmin.xml"), `<adminTools><permissions><permission cmd="saveworld" /></permissions></adminTools>`)
	writeMetadataTestFile(t, root, filepath.Join("Saves", "World", "Survival", "Player", "EOS_abc123.ttp"), "player")

	metadata, errQuery := new(Node).QuerySevenDaysToDieOperationMetadata(t.Context(), SevenDaysToDieOperationMetadataQueryRequest{
		WorkingDirectory: root,
	})
	if errQuery != nil {
		t.Fatalf("QuerySevenDaysToDieOperationMetadata() error = %v", errQuery)
	}
	if len(metadata.Items) != 0 || len(metadata.Buffs) != 1 || metadata.Buffs[0].Value != "buffWarm" ||
		len(metadata.Players) != 1 || metadata.Players[0].Value != "EOS_abc123" ||
		len(metadata.Commands) != 1 || metadata.Commands[0].Value != "saveworld" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestSevenDaysToDieOperationAccentColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "valid RGB", value: "255, 128, 0", want: "#ff8000"},
		{name: "out of range", value: "256,0,0"},
		{name: "CSS value", value: "red"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sevenDaysToDieOperationAccentColor(test.value)
			if got != test.want {
				t.Fatalf("sevenDaysToDieOperationAccentColor(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func writeMetadataTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	errMkdir := os.MkdirAll(filepath.Dir(path), 0o750)
	if errMkdir != nil {
		t.Fatalf("create %q parent: %v", name, errMkdir)
	}
	errWrite := os.WriteFile(path, []byte(content), 0o600)
	if errWrite != nil {
		t.Fatalf("write %q: %v", name, errWrite)
	}
}
