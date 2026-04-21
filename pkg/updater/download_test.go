package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDownloadToFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            []byte
		contentLength   string
		flushBeforeBody bool
		expectedSHA256  string
		expectedSize    int64
		wantErr         error
		wantWritten     int64
		wantFile        bool
	}{
		{
			name:           "writes artifact when checksum and size match",
			body:           []byte("artifact"),
			expectedSHA256: sha256HexBytes([]byte("artifact")),
			expectedSize:   int64(len("artifact")),
			wantWritten:    int64(len("artifact")),
			wantFile:       true,
		},
		{
			name:           "rejects content length mismatch before writing",
			contentLength:  "64",
			expectedSHA256: sha256HexBytes([]byte("artifact")),
			expectedSize:   int64(len("artifact")),
			wantErr:        ErrArtifactSizeMismatch,
		},
		{
			name:            "rejects oversized stream without content length",
			body:            []byte("artifact-extra"),
			flushBeforeBody: true,
			expectedSHA256:  sha256HexBytes([]byte("artifact")),
			expectedSize:    int64(len("artifact")),
			wantErr:         ErrArtifactSizeMismatch,
			wantWritten:     int64(len("artifact")) + 1,
		},
		{
			name:            "rejects short stream without content length",
			body:            []byte("short"),
			flushBeforeBody: true,
			expectedSHA256:  sha256HexBytes([]byte("artifact")),
			expectedSize:    int64(len("artifact")),
			wantErr:         ErrArtifactSizeMismatch,
			wantWritten:     int64(len("short")),
		},
		{
			name:           "removes partial file on checksum mismatch",
			body:           []byte("artifact"),
			expectedSHA256: sha256HexBytes([]byte("different")),
			expectedSize:   int64(len("artifact")),
			wantErr:        ErrChecksumMismatch,
			wantWritten:    int64(len("artifact")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentLength != "" {
					w.Header().Set("Content-Length", tt.contentLength)
				}
				if tt.flushBeforeBody {
					w.WriteHeader(http.StatusOK)
					flusher, ok := w.(http.Flusher)
					if ok {
						flusher.Flush()
					}
				}
				if len(tt.body) == 0 {
					return
				}
				_, errWrite := w.Write(tt.body)
				if errWrite != nil {
					t.Logf("write response body: %v", errWrite)
				}
			}))
			t.Cleanup(server.Close)

			destPath := t.TempDir() + "/artifact.bin"
			gotSHA, gotWritten, errDownload := DownloadToFile(
				context.Background(),
				server.Client(),
				server.URL,
				destPath,
				tt.expectedSHA256,
				tt.expectedSize,
				nil,
			)
			if tt.wantErr != nil {
				if !errors.Is(errDownload, tt.wantErr) {
					t.Fatalf("DownloadToFile() error = %v, want %v", errDownload, tt.wantErr)
				}
			} else if errDownload != nil {
				t.Fatalf("DownloadToFile() error = %v, want nil", errDownload)
			}
			if gotWritten != tt.wantWritten {
				t.Fatalf("DownloadToFile() written = %d, want %d", gotWritten, tt.wantWritten)
			}
			if tt.wantFile {
				data, errRead := os.ReadFile(destPath)
				if errRead != nil {
					t.Fatalf("ReadFile(%q) error = %v", destPath, errRead)
				}
				if string(data) != string(tt.body) {
					t.Fatalf("downloaded file = %q, want %q", string(data), string(tt.body))
				}
				if gotSHA != tt.expectedSHA256 {
					t.Fatalf("DownloadToFile() sha = %q, want %q", gotSHA, tt.expectedSHA256)
				}
				return
			}

			_, errStat := os.Stat(destPath)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("Stat(%q) error = %v, want os.ErrNotExist", destPath, errStat)
			}
		})
	}
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
