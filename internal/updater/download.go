package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ProgressFunc receives cumulative downloaded bytes and the expected total.
type ProgressFunc func(downloaded int64, total int64)

// DownloadToFile downloads url into destPath while computing SHA-256.
func DownloadToFile(ctx context.Context, httpClient *http.Client, rawURL string, destPath string, expectedSHA256 string, expectedSize int64, progress ProgressFunc) (string, int64, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", 0, errors.New("updater: download URL is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if errReq != nil {
		return "", 0, fmt.Errorf("updater: create download request: %w", errReq)
	}
	req.Header.Set("User-Agent", "Xylona-Updater")

	resp, errDo := httpClient.Do(req)
	if errDo != nil {
		return "", 0, fmt.Errorf("updater: download artifact: %w", errDo)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("updater: download artifact: status %d", resp.StatusCode)
	}
	if expectedSize > 0 && resp.ContentLength >= 0 && resp.ContentLength != expectedSize {
		return "", 0, fmt.Errorf("%w: content length got %d bytes, want %d", ErrArtifactSizeMismatch, resp.ContentLength, expectedSize)
	}

	file, errCreate := os.Create(destPath)
	if errCreate != nil {
		return "", 0, fmt.Errorf("updater: create artifact file: %w", errCreate)
	}

	var bodyReader io.Reader = resp.Body
	totalSize := resp.ContentLength
	if expectedSize > 0 {
		bodyReader = io.LimitReader(resp.Body, expectedSize+1)
		totalSize = expectedSize
	}
	reader := &progressReader{
		reader:   bodyReader,
		total:    totalSize,
		progress: progress,
	}
	tee := io.TeeReader(reader, file)
	sum, written, errHash := SHA256Hex(tee)
	errResult := errHash
	if errResult == nil && expectedSize > 0 && written != expectedSize {
		errResult = fmt.Errorf("%w: got %d bytes, want %d", ErrArtifactSizeMismatch, written, expectedSize)
	}
	if errResult == nil {
		errSync := file.Sync()
		if errSync != nil {
			errResult = fmt.Errorf("updater: sync artifact file: %w", errSync)
		}
	}
	errClose := file.Close()
	if errResult != nil {
		if errClose != nil {
			errResult = errors.Join(errResult, fmt.Errorf("updater: close partial artifact file: %w", errClose))
		}
		errRemove := os.Remove(destPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			errResult = errors.Join(errResult, fmt.Errorf("updater: remove partial artifact: %w", errRemove))
		}
		return sum, written, errResult
	}
	if errClose != nil {
		errRemove := os.Remove(destPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			return sum, written, errors.Join(fmt.Errorf("updater: close artifact file: %w", errClose), fmt.Errorf("updater: remove partial artifact: %w", errRemove))
		}
		return sum, written, fmt.Errorf("updater: close artifact file: %w", errClose)
	}
	expectedSHA256 = strings.TrimSpace(expectedSHA256)
	if expectedSHA256 != "" && strings.TrimPrefix(strings.ToLower(expectedSHA256), "sha256:") != sum {
		errRemove := os.Remove(destPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			return sum, written, errors.Join(fmt.Errorf("%w: got %s, want %s", ErrChecksumMismatch, sum, expectedSHA256), fmt.Errorf("updater: remove partial artifact: %w", errRemove))
		}
		return sum, written, fmt.Errorf("%w: got %s, want %s", ErrChecksumMismatch, sum, expectedSHA256)
	}
	return sum, written, nil
}

type progressReader struct {
	reader   io.Reader
	read     int64
	total    int64
	progress ProgressFunc
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, errRead := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.progress != nil {
			r.progress(r.read, r.total)
		}
	}
	if errRead != nil && !errors.Is(errRead, io.EOF) {
		return n, fmt.Errorf("updater: read download response: %w", errRead)
	}
	if errors.Is(errRead, io.EOF) {
		return n, io.EOF
	}
	return n, nil
}
