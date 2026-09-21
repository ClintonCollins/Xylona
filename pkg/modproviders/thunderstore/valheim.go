package thunderstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

// ValheimLoaderID and ValheimLoaderVersion identify the supported BepInEx release.
const (
	ValheimLoaderID      = "denikson-BepInExPack_Valheim"
	ValheimLoaderVersion = "5.4.2333"
	valheimLoaderSHA256  = "5dd24ccbcaa9260f714b200f23c4c15547e2aa5f06906cafcc0dee56db1bf716"
)

var (
	valheimPackagePattern = regexp.MustCompile(`^[A-Za-z0-9_]+-[A-Za-z0-9_]+$`)
	valheimVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// DownloadValheim extracts a package into an empty staging directory using server-root paths.
// The caller owns staging cleanup and commits the files only after this succeeds.
// An empty result is valid only for a verified dependency-only package.
func (p *Provider) DownloadValheim(ctx context.Context, sourceID, versionID, targetDir, targetOS string) (files []modproviders.DownloadedFile, errResult error) {
	if len(sourceID) > 200 || !valheimPackagePattern.MatchString(sourceID) || len(versionID) > 64 || !valheimVersionPattern.MatchString(versionID) {
		return nil, errors.New("invalid Valheim package ID or version")
	}
	if sourceID == ValheimLoaderID && versionID != ValheimLoaderVersion {
		return nil, fmt.Errorf("supported BepInEx version is %s", ValheimLoaderVersion)
	}
	archiveDir, errTemp := os.MkdirTemp(targetDir, ".archive-")
	if errTemp != nil {
		return nil, fmt.Errorf("extract Valheim package: %w", errTemp)
	}
	defer func() { errResult = errors.Join(errResult, os.RemoveAll(archiveDir)) }()
	downloaded, errDownload := p.Download(ctx, sourceID, versionID, archiveDir)
	if errDownload != nil {
		return nil, errDownload
	}
	reader, errReader := os.Open(filepath.Join(archiveDir, downloaded[0].Path))
	if errReader != nil {
		return nil, fmt.Errorf("extract Valheim package: %w", errReader)
	}
	defer func() { errResult = errors.Join(errResult, reader.Close()) }()
	entries, errInspect := inspectValheimArchive(reader, downloaded[0], sourceID, targetOS)
	if errors.Is(errInspect, errNoValheimFiles) && sourceID != ValheimLoaderID {
		metadata, errMetadata := p.ExactVersion(ctx, sourceID, versionID)
		if errMetadata != nil {
			return nil, fmt.Errorf("verify dependency-only package: %w", errMetadata)
		}
		if len(metadata.Dependencies) > 0 {
			return nil, nil
		}
	}
	if errInspect != nil {
		return nil, fmt.Errorf("extract Valheim package: %w", errInspect)
	}
	root, errRoot := os.OpenRoot(targetDir)
	if errRoot != nil {
		return nil, fmt.Errorf("extract Valheim package: %w", errRoot)
	}
	defer func() { errResult = errors.Join(errResult, root.Close()) }()
	for _, entry := range entries {
		errContext := ctx.Err()
		if errContext != nil {
			return nil, fmt.Errorf("extract Valheim package: %w", errContext)
		}
		errMkdir := root.MkdirAll(filepath.Dir(entry.path), 0o755)
		if errMkdir != nil {
			return nil, fmt.Errorf("extract Valheim package: %w", errMkdir)
		}
		stream, errStream := entry.file.Open()
		if errStream != nil {
			return nil, fmt.Errorf("extract Valheim package: %w", errStream)
		}
		output, errOutput := root.OpenFile(entry.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errOutput != nil {
			return nil, errors.Join(errOutput, stream.Close())
		}
		fileHash := sha256.New()
		written, errCopy := io.Copy(io.MultiWriter(output, fileHash), io.LimitReader(stream, entry.size+1))
		errWrite := errors.Join(errCopy, output.Close(), stream.Close())
		if errWrite != nil {
			return nil, errWrite
		}
		if written != entry.size {
			return nil, fmt.Errorf("archive size changed for %q", entry.path)
		}
		files = append(files, modproviders.DownloadedFile{Path: entry.path, Hash: hex.EncodeToString(fileHash.Sum(nil)), Size: entry.size, IsPrimary: len(files) == 0})
	}
	return files, nil
}
