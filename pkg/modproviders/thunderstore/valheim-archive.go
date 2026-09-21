package thunderstore

import (
	"archive/zip"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

const (
	maxValheimArchiveSize = 512 << 20
	maxValheimFileSize    = 128 << 20
	maxValheimFiles       = 10000
)

var errNoValheimFiles = errors.New("archive has no installable files")

type valheimArchiveFile struct {
	file *zip.File
	path string
	size int64
}

func inspectValheimArchive(reader io.ReaderAt, artifact modproviders.DownloadedFile, packageID, targetOS string) ([]valheimArchiveFile, error) {
	size := artifact.Size
	if size <= 0 || size > maxValheimArchiveSize || (targetOS != "windows" && targetOS != "linux") {
		return nil, errors.New("unsupported Valheim archive size or target OS")
	}
	if packageID == ValheimLoaderID && artifact.Hash != valheimLoaderSHA256 {
		return nil, errors.New("unsupported BepInEx loader artifact")
	}
	hash := sha256.New()
	_, errHash := io.Copy(hash, io.NewSectionReader(reader, 0, size))
	if errHash != nil {
		return nil, fmt.Errorf("hash archive: %w", errHash)
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.Hash {
		return nil, errors.New("archive SHA256 does not match package")
	}
	archive, errArchive := zip.NewReader(reader, size)
	if errArchive != nil {
		return nil, fmt.Errorf("read ZIP: %w", errArchive)
	}
	if len(archive.File) > maxValheimFiles {
		return nil, errors.New("archive entry count exceeds limit")
	}
	paths := make(map[string]bool)
	spellings := make(map[string]string)
	targets := make(map[string]bool)
	targetSpellings := make(map[string]string)
	var total uint64
	files := make([]valheimArchiveFile, 0)
	for _, entry := range archive.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !safeValheimPath(name) || (!entry.Mode().IsRegular() && !entry.Mode().IsDir()) {
			return nil, fmt.Errorf("unsafe archive entry %q", entry.Name)
		}
		key := strings.ToLower(name)
		if !consistentValheimPathCase(spellings, name) {
			return nil, fmt.Errorf("archive path casing conflict %q", name)
		}
		_, exists := paths[key]
		if exists {
			return nil, fmt.Errorf("duplicate archive path %q", name)
		}
		paths[key] = entry.Mode().IsRegular()
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > maxValheimFileSize || total > maxValheimArchiveSize {
			return nil, errors.New("archive expanded size exceeds limit")
		}
		if entry.Mode().IsDir() {
			continue
		}
		mapped := name
		loader := packageID == ValheimLoaderID
		if loader {
			mapped = strings.TrimPrefix(mapped, "BepInExPack_Valheim/")
		}
		target, allowed := valheimLoaderPath(mapped, targetOS)
		if !loader {
			target, allowed = valheimInstallPath(mapped, packageID)
		}
		if !allowed {
			if omittedValheimMetadata(mapped) || (loader && (strings.HasSuffix(mapped, ".sh") || mapped == "doorstop_libs/libdoorstop_x64.dylib" || mapped == "winhttp.dll" || mapped == "doorstop_config.ini" || mapped == "doorstop_libs/libdoorstop_x64.so")) {
				continue
			}
			return nil, fmt.Errorf("unsupported archive path %q", name)
		}
		targetKey := strings.ToLower(target)
		if !safeValheimPath(target) || !consistentValheimPathCase(targetSpellings, target) || targets[targetKey] {
			return nil, errors.New("conflicting target paths")
		}
		targets[targetKey] = true
		files = append(files, valheimArchiveFile{file: entry, path: target, size: int64(entry.UncompressedSize64)})
	}
	for name := range targets {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if targets[parent] {
				return nil, fmt.Errorf("target file and directory collision %q", name)
			}
		}
	}
	for name := range paths {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if paths[parent] {
				return nil, fmt.Errorf("archive file and directory collision %q", name)
			}
		}
	}
	if len(files) == 0 {
		return nil, errNoValheimFiles
	}
	slices.SortFunc(files, func(a, b valheimArchiveFile) int { return cmp.Compare(a.path, b.path) })
	return files, nil
}

func valheimInstallPath(name, packageID string) (string, bool) {
	if omittedValheimMetadata(name) {
		return "", false
	}
	name = strings.TrimPrefix(name, "BepInEx/")
	for _, directory := range []string{"plugins/", "patchers/", "config/"} {
		relative, found := strings.CutPrefix(name, directory)
		if found {
			if directory == "config/" {
				return "BepInEx/config/" + relative, true
			}
			return "BepInEx/" + directory + packageID + "/" + relative, true
		}
	}
	// Packages cannot replace the loader or supply executable launch scripts.
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "core/") || strings.HasPrefix(lower, "doorstop") || lower == "winhttp.dll" {
		return "", false
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".exe", ".bat", ".cmd", ".ps1", ".sh":
		return "", false
	}
	return "BepInEx/plugins/" + packageID + "/" + name, true
}

func omittedValheimMetadata(name string) bool {
	switch strings.ToLower(name) {
	case "manifest.json", "readme.md", "icon.png", "changelog.md", "changelog.txt":
		return true
	}
	return false
}

func valheimLoaderPath(name, targetOS string) (string, bool) {
	if strings.HasPrefix(name, "BepInEx/plugins/") || strings.HasPrefix(name, "BepInEx/config/") {
		return name, true
	}
	if strings.HasPrefix(name, "BepInEx/core/") || name == ".doorstop_version" {
		return name, true
	}
	base := strings.ToLower(path.Base(name))
	if !strings.Contains(name, "/") && (strings.HasPrefix(base, "license") || strings.HasPrefix(base, "notice") || strings.HasPrefix(base, "copying")) {
		return name, true
	}
	if targetOS == "windows" && (name == "winhttp.dll" || name == "doorstop_config.ini") {
		return name, true
	}
	if targetOS == "linux" && name == "doorstop_libs/libdoorstop_x64.so" {
		return name, true
	}
	return name, false
}

func safeValheimPath(name string) bool {
	if name == "" || len(name) > 512 || name == "." || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:<>\"|?*\x00") {
		return false
	}
	for component := range strings.SplitSeq(name, "/") {
		if component == ".." || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return false
		}
		for _, char := range component {
			if char < 32 || char > 126 {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
			return false
		}
	}
	return true
}

func consistentValheimPathCase(spellings map[string]string, name string) bool {
	for current := name; current != "."; current = path.Dir(current) {
		key := strings.ToLower(current)
		previous, exists := spellings[key]
		if exists && previous != current {
			return false
		}
		spellings[key] = current
	}
	return true
}
