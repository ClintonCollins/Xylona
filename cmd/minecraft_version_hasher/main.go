// Package main builds a local Minecraft server version hash manifest.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path"
	"sort"
	"syscall"
	"time"
)

type MinecraftVersionManifestJSON struct {
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

type MinecraftServerInfo struct {
	ID          string `json:"id"`
	Sha1        string `json:"sha1"`
	DownloadURL string `json:"downloadURL"`
	Size        int    `json:"size"`
	Type        string
	ReleaseTime time.Time
}

func main() {
	shutdownSignalChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownSignalChannel, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// getVersions(shutdownSignalChannel)
	serverInfo, errGetVersions := getAllServerVersionsInformation(shutdownSignalChannel)
	if errGetVersions != nil {
		fmt.Fprintln(os.Stderr, "Failed to get Minecraft versions")
		os.Exit(1)
	}

	errSaveVersionToFile := saveServerVersionsToFile(serverInfo)
	if errSaveVersionToFile != nil {
		fmt.Fprintf(os.Stderr, "Failed to save server versions to file: %s\n", errSaveVersionToFile.Error())
		os.Exit(1)
	}

	// sha1Map := generateSha1Map(serverInfo)
	//
	// jars, errRead := os.ReadDir("test_jars")
	// if errRead != nil {
	//	fmt.Fprintln(os.Stderr, "Failed to read test_jars directory")
	//	os.Exit(1)
	// }
	// for _, jar := range jars {
	//	if jar.IsDir() {
	//		continue
	//	}
	//	fmt.Fprintf(os.Stderr, "Checking %s for version based on sha1.\n", jar.Name())
	//	sha1Hash, errCalculate := calculateSha1OfFile(path.Join("test_jars", jar.Name()))
	//	if errCalculate != nil {
	//		fmt.Fprintln(os.Stderr, "Failed to calculate sha1 for "+jar.Name())
	//		continue
	//	}
	//	version, exists := sha1Map[sha1Hash]
	//	if exists {
	//		fmt.Fprintf(os.Stderr, "Found version %s released on %s\n", version.ID, version.ReleaseTime)
	//	} else {
	//		fmt.Fprintf(os.Stderr, "Version not found for %s\n", jar.Name())
	//	}
	//
	// }
}

func saveServerVersionsToFile(serverInfos []*MinecraftServerInfo) error {
	sort.Slice(serverInfos, func(i, j int) bool {
		return serverInfos[i].ReleaseTime.After(serverInfos[j].ReleaseTime)
	})

	f, errCreate := os.Create("server_versions.json")
	if errCreate != nil {
		return fmt.Errorf("create server versions file: %w", errCreate)
	}
	defer func() {
		_ = f.Close()
	}()

	b, errMarshalIndent := json.MarshalIndent(serverInfos, "", "  ")
	if errMarshalIndent != nil {
		return fmt.Errorf("marshal server versions: %w", errMarshalIndent)
	}
	_, errWrite := f.Write(b)
	if errWrite != nil {
		return fmt.Errorf("write server versions file: %w", errWrite)
	}
	// errEncode := json.NewEncoder(f).Encode(serverInfos)
	// if errEncode != nil {
	// 	return errEncode
	//}
	return nil
}

func getAllServerVersionsInformation(shutdownSignalChannel chan os.Signal) ([]*MinecraftServerInfo, error) {
	files, errReadDir := os.ReadDir("versions")
	if errReadDir != nil {
		fmt.Fprintln(os.Stderr, "Failed to read versions directory")
		return nil, fmt.Errorf("read versions directory: %w", errReadDir)
	}
	var serverInfos []*MinecraftServerInfo
	for _, file := range files {
		select {
		case <-shutdownSignalChannel:
			fmt.Fprintln(os.Stderr, "Received shutdown signal. Exiting...")
			os.Exit(0)
		default:
			if file.IsDir() {
				continue
			}
			serverInfo, errGetSha1 := getMinecraftServerInformation(file.Name())
			if errGetSha1 != nil {
				fmt.Fprintln(os.Stderr, "Failed to get server sha1 for "+file.Name())
				continue
			}
			serverInfos = append(serverInfos, serverInfo)
		}
	}
	return serverInfos, nil
}

func getMinecraftServerInformation(fileName string) (*MinecraftServerInfo, error) {
	f, errOpen := os.Open(path.Join("versions", fileName))
	if errOpen != nil {
		return nil, fmt.Errorf("open version manifest %s: %w", fileName, errOpen)
	}
	defer func() {
		_ = f.Close()
	}()

	minecraftManifest := MinecraftVersionManifestJSON{}
	errDecode := json.NewDecoder(f).Decode(&minecraftManifest)
	if errDecode != nil {
		return nil, fmt.Errorf("decode version manifest %s: %w", fileName, errDecode)
	}

	serverInfo := &MinecraftServerInfo{
		ID:          minecraftManifest.ID,
		Sha1:        minecraftManifest.Downloads.Server.Sha1,
		DownloadURL: minecraftManifest.Downloads.Server.URL,
		Size:        minecraftManifest.Downloads.Server.Size,
		Type:        minecraftManifest.Type,
		ReleaseTime: minecraftManifest.ReleaseTime,
	}

	return serverInfo, nil
}
