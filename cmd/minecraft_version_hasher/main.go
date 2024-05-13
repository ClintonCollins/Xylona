package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"sort"
	"syscall"
	"time"

	"github.com/pterm/pterm"

	"github.com/ClintonCollins/Xylona/helpers"
)

type GlobalMinecraftVersionsManifestJSON struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []MinecraftVersion `json:"versions"`
}

type MinecraftVersion struct {
	Id          string    `json:"id"`
	Type        string    `json:"type"`
	Url         string    `json:"url"`
	Time        time.Time `json:"time"`
	ReleaseTime time.Time `json:"releaseTime"`
}

type MinecraftVersionManifestJSON struct {
	Arguments struct {
		Game []interface{} `json:"game"`
		Jvm  []interface{} `json:"jvm"`
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
				} `json:"natives-macos,omitempty"`
				NativesLinux struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-linux,omitempty"`
				NativesWindows struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-windows,omitempty"`
				NativesOsx struct {
					Path string `json:"path"`
					Sha1 string `json:"sha1"`
					Size int    `json:"size"`
					Url  string `json:"url"`
				} `json:"natives-osx,omitempty"`
			} `json:"classifiers,omitempty"`
		} `json:"downloads"`
		Name  string `json:"name"`
		Rules []struct {
			Action string `json:"action"`
			Os     struct {
				Name string `json:"name"`
			} `json:"os,omitempty"`
		} `json:"rules,omitempty"`
		Natives struct {
			Osx     string `json:"osx,omitempty"`
			Linux   string `json:"linux,omitempty"`
			Windows string `json:"windows,omitempty"`
		} `json:"natives,omitempty"`
		Extract struct {
			Exclude []string `json:"exclude"`
		} `json:"extract,omitempty"`
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

func getMinecraftVersionsFromManifestAPI() ([]MinecraftVersion, error) {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
	if errGet != nil {
		return nil, errGet
	}
	defer func() {
		_ = response.Body.Close()
	}()

	minecraftManifest := GlobalMinecraftVersionsManifestJSON{}
	errDecode := json.NewDecoder(response.Body).Decode(&minecraftManifest)
	if errDecode != nil {
		pterm.Error.Printf("Failed to decode version manifest file: %s\n", errDecode.Error())
		return nil, errDecode
	}

	return minecraftManifest.Versions, nil
}

func getMissingVersions(mainManifestVersions []MinecraftVersion) ([]MinecraftVersion, error) {
	var missingVersions []MinecraftVersion
	for _, version := range mainManifestVersions {
		if _, errStat := os.Stat(path.Join("versions", version.Id+".json")); os.IsNotExist(errStat) {
			missingVersions = append(missingVersions, version)
		}
	}

	return missingVersions, nil
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

	//sha1Map := generateSha1Map(serverInfo)
	//
	//jars, errRead := os.ReadDir("test_jars")
	//if errRead != nil {
	//	pterm.Error.Println("Failed to read test_jars directory")
	//	os.Exit(1)
	//}
	//for _, jar := range jars {
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
	//}
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
	//errEncode := json.NewEncoder(f).Encode(serverInfos)
	//if errEncode != nil {
	//	return errEncode
	//}
	return nil
}

func calculateSha1OfFile(filePath string) (string, error) {
	f, errOpen := os.Open(filePath)
	if errOpen != nil {
		return "", errOpen
	}
	defer func() {
		_ = f.Close()
	}()

	hash := sha1.New()
	_, errCopy := io.Copy(hash, f)
	if errCopy != nil {
		return "", errCopy
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil

}

func generateSha1Map(serverInfos []*MinecraftServerInfo) map[string]*MinecraftServerInfo {
	sha1Map := make(map[string]*MinecraftServerInfo)
	for _, serverInfo := range serverInfos {
		sha1Map[serverInfo.Sha1] = serverInfo
	}
	return sha1Map
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

func getVersions(shutdownSignalChannel chan os.Signal) {
	versions, errGetVersions := getMinecraftVersionsFromManifestAPI()
	if errGetVersions != nil {
		pterm.Error.Println("Failed to get Minecraft versions from manifest")
		os.Exit(1)
	}

	versionsToDownload, errGetMissingVersions := getMissingVersions(versions)
	if errGetMissingVersions != nil {
		pterm.Error.Println("Failed to get missing Minecraft versions")
		os.Exit(1)
	}

	pterm.Info.Printf("Found %d total versions and %d missing versions\n", len(versions), len(versionsToDownload))

	p, _ := pterm.DefaultProgressbar.WithTotal(len(versionsToDownload)).WithTitle("Getting Minecraft versions").Start()

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	firstLoop := true
	for i := range p.Total {
		// If this is the first loop, we want to skip the sleep and just print the first item.
		if firstLoop {
			firstLoop = false
			handleProgressIncrement(p, versionsToDownload, i)
			if i < p.Total {
				p.UpdateTitle("Downloading " + versionsToDownload[i+1].Id)
			}
			continue
		}
		select {
		case <-shutdownSignalChannel:
			_, _ = p.Stop()
			pterm.Info.Println("Received shutdown signal. Exiting...")
			os.Exit(0)
		case <-ticker.C:
			handleProgressIncrement(p, versionsToDownload, i)
			if i < p.Total {
				p.UpdateTitle("Downloading " + versionsToDownload[i+1].Id)
			}
		}
	}
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

func handleProgressIncrement(progressBar *pterm.ProgressbarPrinter, versionsToDownload []MinecraftVersion, i int) {
	// Update the title of the progressbar with the current item being downloaded.
	progressBar.UpdateTitle("Downloading " + versionsToDownload[i].Id)

	errDownload := downloadMinecraftVersionManifest(versionsToDownload[i])
	if errDownload != nil {
		pterm.Error.Println("Failed to download " + versionsToDownload[i].Id)
		return
	}

	// Print a success message for the current download. This will be printed above the progressbar.
	fmt.Println()
	pterm.Success.Println("Downloaded " + versionsToDownload[i].Id)

	// Increment the progressbar by one to indicate progress.
	progressBar.Increment()
}

func downloadMinecraftVersionManifest(version MinecraftVersion) error {
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get(version.Url)
	if errGet != nil {
		return errGet
	}
	defer func() {
		_ = response.Body.Close()
	}()

	f, errCreate := os.Create(path.Join("versions", version.Id+".json"))
	if errCreate != nil {
		return errCreate
	}
	defer func() {
		_ = f.Close()
	}()

	_, errCopy := io.Copy(f, response.Body)
	if errCopy != nil {
		return errCopy
	}
	return nil
}
