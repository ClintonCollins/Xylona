// Package minecraft provides helpers for Mojang version metadata.
package minecraft

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
)

// GlobalVersionsManifestJSON represents Mojang's global version manifest response.
type GlobalVersionsManifestJSON struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []Version `json:"versions"`
}

// Version represents a single version entry from the global manifest.
type Version struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	Time        time.Time `json:"time"`
	ReleaseTime time.Time `json:"releaseTime"`
}

// VersionManifestJSON represents Mojang's per-version manifest response.
type VersionManifestJSON struct {
	Arguments struct {
		Game []any `json:"game"`
		Jvm  []any `json:"jvm"`
	} `json:"arguments"`
	AssetIndex struct {
		ID        string `json:"id"`
		Sha1      string `json:"sha1"`
		Size      int    `json:"size"`
		TotalSize int    `json:"totalSize"`
		URL       string `json:"url"`
	} `json:"assetIndex"`
	Assets          string `json:"assets"`
	ComplianceLevel int    `json:"complianceLevel"`
	Downloads       struct {
		Client struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"client"`
		ClientMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"client_mappings"`
		Server struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"server"`
		ServerMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			URL  string `json:"url"`
		} `json:"server_mappings"`
	} `json:"downloads"`
	ID          string `json:"id"`
	JavaVersion struct {
		Component    string `json:"component"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion"`
	Libraries []struct {
		Downloads struct {
			Artifact struct {
				Path string `json:"path"`
				Sha1 string `json:"sha1"`
				Size int    `json:"size"`
				URL  string `json:"url"`
			} `json:"artifact"`
			Classifiers struct {
				NativesMacos struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					URL  string `json:"url"`
				} `json:"natives-macos,omitzero"`
				NativesLinux struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					URL  string `json:"url"`
				} `json:"natives-linux,omitzero"`
				NativesWindows struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					URL  string `json:"url"`
				} `json:"natives-windows,omitzero"`
				NativesOsx struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					URL  string `json:"url"`
				} `json:"natives-osx,omitzero"`
			} `json:"classifiers,omitzero"`
		} `json:"downloads"`
		Name  string `json:"name"`
		Rules []struct {
			Action string `json:"action"`
			Os     struct {
				Name string `json:"name"`
			} `json:"os,omitzero"`
		} `json:"rules,omitempty"`
		Natives struct {
			Osx     string `json:"osx,omitempty"`
			Linux   string `json:"linux,omitempty"`
			Windows string `json:"windows,omitempty"`
		} `json:"natives,omitzero"`
		Extract struct {
			Exclude []string `json:"exclude"`
		} `json:"extract,omitzero"`
	} `json:"libraries"`
	Logging struct {
		Client struct {
			Argument string `json:"argument"`
			File     struct {
				ID   string `json:"id"`
				Sha1 string `json:"sha1"`
				Size int    `json:"size"`
				URL  string `json:"url"`
			} `json:"file"`
			Type string `json:"type"`
		} `json:"client"`
	} `json:"logging"`
	MainClass              string    `json:"mainClass"`
	MinimumLauncherVersion int       `json:"minimumLauncherVersion"`
	ReleaseTime            time.Time `json:"releaseTime"`
	Time                   time.Time `json:"time"`
	Type                   string    `json:"type"`
}

// ServerInfo describes a downloadable Minecraft server artifact.
type ServerInfo struct {
	ID          string `json:"id"`
	Sha1        string `json:"sha1"`
	DownloadURL string `json:"downloadURL"`
	Size        int    `json:"size"`
	Type        string
	ReleaseTime time.Time
}

// GetLatestServerDownloadURL returns the latest official server jar URL.
func GetLatestServerDownloadURL() (string, error) {
	manifest, err := getMinecraftManifestAPI()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get Minecraft version manifest")
		return "", err
	}
	latest := manifest.Latest.Release
	latestVersionURL := ""
	for _, version := range manifest.Versions {
		if version.ID == latest {
			latestVersionURL = version.URL
			break
		}
	}
	if latestVersionURL == "" {
		log.Error().Msg("Failed to get latest Minecraft version URL")
		return "", nil
	}
	minecraftVersion, errVersionInfo := getMinecraftVersionInfo(latestVersionURL)
	if errVersionInfo != nil {
		log.Error().Err(errVersionInfo).Msg("Failed to get Minecraft version info")
		return "", errVersionInfo
	}
	return minecraftVersion.Downloads.Server.URL, nil
}

func getMinecraftVersionInfo(url string) (*VersionManifestJSON, error) {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get(url)
	if errGet != nil {
		pterm.Error.Printf("Failed to get Minecraft version info: %s\n", errGet.Error())
		return nil, fmt.Errorf("get minecraft version info from %s: %w", url, errGet)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	minecraftVersion := &VersionManifestJSON{}
	errDecode := json.NewDecoder(response.Body).Decode(&minecraftVersion)
	if errDecode != nil {
		pterm.Error.Printf("Failed to decode Minecraft version info: %s\n", errDecode.Error())
		return nil, fmt.Errorf("decode minecraft version info from %s: %w", url, errDecode)
	}
	return minecraftVersion, nil
}

func getMinecraftManifestAPI() (*GlobalVersionsManifestJSON, error) {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if errGet != nil {
		return nil, fmt.Errorf("get minecraft version manifest: %w", errGet)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	minecraftManifest := &GlobalVersionsManifestJSON{}
	errDecode := json.NewDecoder(response.Body).Decode(&minecraftManifest)
	if errDecode != nil {
		pterm.Error.Printf("Failed to decode version manifest file: %s\n", errDecode.Error())
		return nil, fmt.Errorf("decode minecraft version manifest: %w", errDecode)
	}

	return minecraftManifest, nil
}
