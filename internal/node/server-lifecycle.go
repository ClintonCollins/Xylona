package node

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// lockServerLifecycle serializes process transitions and configuration writes
// for the same server directory, including requests using a directory alias.
func (n *Node) lockServerLifecycle(directory string) (func(), error) {
	absolute, errAbsolute := filepath.Abs(directory)
	if errAbsolute != nil {
		return nil, fmt.Errorf("node: resolve lifecycle directory: %w", errAbsolute)
	}
	candidate := absolute
	suffix := ""
	for {
		resolved, errResolve := filepath.EvalSymlinks(candidate)
		if errResolve == nil {
			absolute = filepath.Join(resolved, suffix)
			break
		}
		if !isNotExistError(errResolve) {
			return nil, fmt.Errorf("node: resolve lifecycle directory symlinks: %w", errResolve)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return nil, fmt.Errorf("node: resolve lifecycle directory: %w", errResolve)
		}
		suffix = filepath.Join(filepath.Base(candidate), suffix)
		candidate = parent
	}
	key := filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	value, _ := n.serverLifecycleLocks.LoadOrStore(key, &sync.Mutex{})
	mutex, valid := value.(*sync.Mutex)
	if !valid {
		return nil, errors.New("node: invalid lifecycle guard")
	}
	mutex.Lock()
	return mutex.Unlock, nil
}
