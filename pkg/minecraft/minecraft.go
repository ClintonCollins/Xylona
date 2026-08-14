// Package minecraft provides helpers for Mojang version metadata.
package minecraft

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

type globalVersionsManifest struct {
	Latest struct {
		Release string `json:"release"`
	} `json:"latest"`
	Versions []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"versions"`
}

type versionManifest struct {
	Downloads struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
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

func getMinecraftVersionInfo(url string) (*versionManifest, error) {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get(url)
	if errGet != nil {
		return nil, fmt.Errorf("get minecraft version info from %s: %w", url, errGet)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	minecraftVersion := &versionManifest{}
	errDecode := json.NewDecoder(response.Body).Decode(&minecraftVersion)
	if errDecode != nil {
		return nil, fmt.Errorf("decode minecraft version info from %s: %w", url, errDecode)
	}
	return minecraftVersion, nil
}

func getMinecraftManifestAPI() (*globalVersionsManifest, error) {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if errGet != nil {
		return nil, fmt.Errorf("get minecraft version manifest: %w", errGet)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	minecraftManifest := &globalVersionsManifest{}
	errDecode := json.NewDecoder(response.Body).Decode(&minecraftManifest)
	if errDecode != nil {
		return nil, fmt.Errorf("decode minecraft version manifest: %w", errDecode)
	}

	return minecraftManifest, nil
}
