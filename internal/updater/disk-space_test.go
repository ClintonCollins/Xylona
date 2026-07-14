package updater

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestEnsureFreeSpace(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		requirements []int64
		wantErr      error
	}{
		{
			name:         "available space",
			path:         t.TempDir(),
			requirements: []int64{1, 1},
		},
		{
			name:         "insufficient space",
			path:         t.TempDir(),
			requirements: []int64{math.MaxInt64},
			wantErr:      ErrInsufficientDiskSpace,
		},
		{
			name:         "overflow",
			path:         t.TempDir(),
			requirements: []int64{math.MaxInt64, 1},
			wantErr:      errors.New("disk space requirement overflow"),
		},
		{
			name:         "negative requirement",
			path:         t.TempDir(),
			requirements: []int64{-1},
			wantErr:      errors.New("cannot be negative"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errSpace := EnsureFreeSpace(tt.path, tt.requirements...)
			if tt.wantErr == nil && errSpace != nil {
				t.Fatalf("EnsureFreeSpace() error = %v, want nil", errSpace)
			}
			if tt.wantErr == nil {
				return
			}
			if errors.Is(tt.wantErr, ErrInsufficientDiskSpace) {
				if !errors.Is(errSpace, ErrInsufficientDiskSpace) {
					t.Fatalf("EnsureFreeSpace() error = %v, want ErrInsufficientDiskSpace", errSpace)
				}
				return
			}
			if errSpace == nil || !containsError(errSpace, tt.wantErr.Error()) {
				t.Fatalf("EnsureFreeSpace() error = %v, want error containing %q", errSpace, tt.wantErr)
			}
		})
	}
}

func TestPathsShareVolume(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	same, errSame := PathsShareVolume(first, second)
	if errSame != nil {
		t.Fatalf("PathsShareVolume() error = %v", errSame)
	}
	if !same {
		t.Fatal("PathsShareVolume() = false for two temporary directories, want true")
	}
}

func containsError(err error, fragment string) bool {
	return err != nil && fragment != "" && strings.Contains(err.Error(), fragment)
}
