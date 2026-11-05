package gamedefinitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
)

func TestEnshroudedDefinition(t *testing.T) {
	data, errRead := FS.ReadFile("official/enshrouded.json")
	if errRead != nil {
		t.Fatal(errRead)
	}
	var document Document
	errDecode := json.Unmarshal(data, &document)
	if errDecode != nil {
		t.Fatal(errDecode)
	}
	hash, errHash := HashDocument(document)
	if errHash != nil {
		t.Fatal(errHash)
	}
	if document.ContentHash != hash {
		t.Errorf("Enshrouded definition hash must be %s", hash)
	}
	definition, errParse := Parse(data)
	if errParse != nil {
		t.Fatal(errParse)
	}
	issues := ValidateModel(definition.Model)
	if len(issues) != 0 {
		t.Fatalf("invalid Enshrouded definition: %v", issues)
	}
	schemas, errSchemas := cfgschema.ParseConfigSchemas(string(document.ConfigSchemas))
	if errSchemas != nil {
		t.Fatal(errSchemas)
	}
	if len(schemas) != 1 {
		t.Fatalf("config schema count = %d, want 1", len(schemas))
	}
	entry := schemas[0]
	parser := &cfgparse.JSONParser{}
	root, errConfig := parser.Parse([]byte(enshroudedConfig))
	if errConfig != nil {
		t.Fatal(errConfig)
	}
	resolver := cfgschema.GameServerSettingsResolver(cfgschema.GameServerSettings{
		Name: "Test Enshrouded", IP: "127.0.0.1", Port: 15637, QueryPort: 15637, MaxPlayers: 16,
	})
	matched := cfgschema.MatchFields(cfgparse.Flatten(root), entry.Schema, entry.ManagedFields, resolver)
	if len(matched.AdvancedFields) != 0 {
		t.Errorf("unrecognized fields = %v, want none", matched.AdvancedFields)
	}
	if len(matched.Fields) != 56 {
		t.Errorf("recognized field count = %d, want 56", len(matched.Fields))
	}
	for key, property := range entry.Schema.Properties {
		if strings.HasPrefix(key, "userGroups.") && property.Default != nil {
			t.Errorf("player group field %q must not synthesize defaults", key)
		}
	}
	validationErrors := cfgschema.ValidateFields(matched.Fields, entry.Schema)
	if len(validationErrors) != 0 {
		t.Errorf("current configuration validation errors = %v", validationErrors)
	}
	cfgschema.MergeStructuredFields(root, matched.Fields, matched.AdvancedFields, entry.Schema)
	roundTrip, errRoundTrip := parser.Write(root)
	if errRoundTrip != nil {
		t.Fatal(errRoundTrip)
	}
	var original, saved map[string]any
	errOriginal := json.Unmarshal([]byte(enshroudedConfig), &original)
	if errOriginal != nil {
		t.Fatal(errOriginal)
	}
	errSaved := json.Unmarshal(roundTrip, &saved)
	if errSaved != nil {
		t.Fatal(errSaved)
	}
	if !reflect.DeepEqual(saved, original) {
		t.Fatalf("schema round trip changed config: %s", roundTrip)
	}

	for _, existing := range []bool{false, true} {
		name := "first start leaves native player group defaults to the game"
		if existing {
			name = "restart preserves existing player groups"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, entry.Path)
			if existing {
				errWrite := os.WriteFile(configPath, []byte(enshroudedConfig), 0o600)
				if errWrite != nil {
					t.Fatal(errWrite)
				}
			}
			errStart := cfgschema.RunPreStartStrict(dir, string(document.ConfigSchemas), resolver)
			if errStart != nil {
				t.Fatal(errStart)
			}
			generated, errGenerated := os.ReadFile(configPath)
			if errGenerated != nil {
				t.Fatal(errGenerated)
			}
			var config map[string]any
			errJSON := json.Unmarshal(generated, &config)
			if errJSON != nil {
				t.Fatal(errJSON)
			}
			if existing {
				if !reflect.DeepEqual(config, original) {
					t.Fatalf("pre-start changed existing configuration: %s", generated)
				}
				return
			}
			_, hasGroups := config["userGroups"]
			if hasGroups {
				t.Fatalf("first start synthesized player groups: %s", generated)
			}
			generatedRoot, errGeneratedRoot := parser.Parse(generated)
			if errGeneratedRoot != nil {
				t.Fatal(errGeneratedRoot)
			}
			generatedFields := cfgparse.Flatten(generatedRoot)
			if len(generatedFields) != 48 {
				t.Fatalf("generated field count = %d, want 48", len(generatedFields))
			}
			if config["name"] != "Test Enshrouded" || config["queryPort"] != float64(15637) {
				t.Fatalf("managed fields were not applied: %s", generated)
			}
		})
	}
}

const enshroudedConfig = `{
  "name": "Test Enshrouded",
  "saveDirectory": "./savegame",
  "logDirectory": "./logs",
  "ip": "127.0.0.1",
  "queryPort": 15637,
  "slotCount": 16,
  "voiceChatMode": "Proximity",
  "enableVoiceChat": false,
  "enableTextChat": false,
  "gameSettingsPreset": "Default",
  "gameSettings": {
    "playerHealthFactor": 1,
    "playerManaFactor": 1,
    "playerStaminaFactor": 1,
    "playerBodyHeatFactor": 1,
    "playerDivingTimeFactor": 1,
    "enableDurability": true,
    "enableStarvingDebuff": false,
    "foodBuffDurationFactor": 1,
    "fromHungerToStarving": 600000000000,
    "shroudTimeFactor": 1,
    "tombstoneMode": "AddBackpackMaterials",
    "enableGliderTurbulences": true,
    "weatherFrequency": "Normal",
    "fishingDifficulty": "Normal",
    "miningDamageFactor": 1,
    "plantGrowthSpeedFactor": 1,
    "resourceDropStackAmountFactor": 1,
    "factoryProductionSpeedFactor": 1,
    "perkUpgradeRecyclingFactor": 0.5,
    "perkCostFactor": 1,
    "experienceCombatFactor": 1,
    "experienceMiningFactor": 1,
    "experienceExplorationQuestsFactor": 1,
    "randomSpawnerAmount": "Automatic",
    "aggroPoolAmount": "Normal",
    "enemyDamageFactor": 1,
    "enemyHealthFactor": 1,
    "enemyStaminaFactor": 1,
    "enemyPerceptionRangeFactor": 1,
    "bossDamageFactor": 1,
    "bossHealthFactor": 1,
    "threatBonus": 1,
    "pacifyAllEnemies": false,
    "tamingStartleRepercussion": "LoseSomeProgress",
    "dayTimeDuration": 1800000000000,
    "nightTimeDuration": 720000000000,
    "curseModifier": "Normal",
    "flameLevelFogRemoval": 11
  },
  "userGroups": [{
    "name": "Default",
    "password": "test-group-secret",
    "canKickBan": false,
    "canAccessInventories": true,
    "canEditWorld": true,
    "canEditBase": true,
    "canExtendBase": true,
    "reservedSlots": 0
  }]
}`
