package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"path"
	"sort"
	"syscall"
	"time"

	"github.com/pterm/pterm"
)

type MinecraftVersionManifestJSON struct {
	Arguments struct {
		Game []any `json:"game"`
		Jvm  []any `json:"jvm"`
	} `json:"arguments"`
	AssetIndex struct {
		Id        string `json:"id"`
		Sha1      string `json:"sha1"`
		Size      int    `json:"size"`
		TotalSize int    `json:"totalSize"`
		Url       string `json:"url"`
	} `json:"assetIndex"`
	Assets          string `json:"assets"`
	ComplianceLevel int    `json:"complianceLevel"`
	Downloads       struct {
		Client struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			Url  string `json:"url"`
		} `json:"client"`
		ClientMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			Url  string `json:"url"`
		} `json:"client_mappings"`
		Server struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			Url  string `json:"url"`
		} `json:"server"`
		ServerMappings struct {
			Sha1 string `json:"sha1"`
			Size int    `json:"size"`
			Url  string `json:"url"`
		} `json:"server_mappings"`
	} `json:"downloads"`
	Id          string `json:"id"`
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
				Url  string `json:"url"`
			} `json:"artifact"`
			Classifiers struct {
				NativesMacos struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-macos,omitzero"`
				NativesLinux struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-linux,omitzero"`
				NativesWindows struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-windows,omitzero"`
				NativesOsx struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
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
				Id   string `json:"id"`
				Sha1 string `json:"sha1"`
				Size int    `json:"size"`
				Url  string `json:"url"`
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
		pterm.Error.Println("Failed to get Minecraft versions")
		os.Exit(1)
	}

	errSaveVersionToFile := saveServerVersionsToFile(serverInfo)
	if errSaveVersionToFile != nil {
		pterm.Error.Printf("Failed to save server versions to file: %s\n", errSaveVersionToFile.Error())
		os.Exit(1)
	}

	// sha1Map := generateSha1Map(serverInfo)
	//
	// jars, errRead := os.ReadDir("test_jars")
	// if errRead != nil {
	//	pterm.Error.Println("Failed to read test_jars directory")
	//	os.Exit(1)
	// }
	// for _, jar := range jars {
	//	if jar.IsDir() {
	//		continue
	//	}
	//	pterm.Info.Printf("Checking %s for version based on sha1.\n", jar.Name())
	//	sha1Hash, errCalculate := calculateSha1OfFile(path.Join("test_jars", jar.Name()))
	//	if errCalculate != nil {
	//		pterm.Error.Println("Failed to calculate sha1 for " + jar.Name())
	//		continue
	//	}
	//	version, exists := sha1Map[sha1Hash]
	//	if exists {
	//		pterm.Info.Printf("Found version %s released on %s\n", version.ID, version.ReleaseTime)
	//	} else {
	//		pterm.Error.Printf("Version not found for %s\n", jar.Name())
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
		return errCreate
	}
	defer func() {
		_ = f.Close()
	}()

	b, errMarshalIndent := json.MarshalIndent(serverInfos, "", "  ")
	if errMarshalIndent != nil {
		return errMarshalIndent
	}
	_, errWrite := f.Write(b)
	if errWrite != nil {
		return errWrite
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
		pterm.Error.Println("Failed to read versions directory")
		return nil, errReadDir
	}
	var serverInfos []*MinecraftServerInfo
	for _, file := range files {
		select {
		case <-shutdownSignalChannel:
			pterm.Info.Println("Received shutdown signal. Exiting...")
			os.Exit(0)
		default:
			if file.IsDir() {
				continue
			}
			serverInfo, errGetSha1 := getMinecraftServerInformation(file.Name())
			if errGetSha1 != nil {
				pterm.Error.Println("Failed to get server sha1 for " + file.Name())
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
		return nil, errOpen
	}
	defer func() {
		_ = f.Close()
	}()

	minecraftManifest := MinecraftVersionManifestJSON{}
	errDecode := json.NewDecoder(f).Decode(&minecraftManifest)
	if errDecode != nil {
		return nil, errDecode
	}

	serverInfo := &MinecraftServerInfo{
		ID:          minecraftManifest.Id,
		Sha1:        minecraftManifest.Downloads.Server.Sha1,
		DownloadURL: minecraftManifest.Downloads.Server.Url,
		Size:        minecraftManifest.Downloads.Server.Size,
		Type:        minecraftManifest.Type,
		ReleaseTime: minecraftManifest.ReleaseTime,
	}

	return serverInfo, nil
}
