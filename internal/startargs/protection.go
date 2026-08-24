package startargs

import (
	"path"
	"path/filepath"
	"strings"
)

// IsProtectedServerPath reports whether a relative path targets protected binaries.
func IsProtectedServerPath(relativePath string, baseCommand string, serverExecutable string) bool {
	normalizedPath := normalizeRelativePath(relativePath)
	if normalizedPath == "" {
		return false
	}

	executablePath := normalizeRelativePath(serverExecutable)
	if executablePath != "" && executablePath == normalizedPath {
		return true
	}

	baseCommandPath := normalizeBaseCommandPath(baseCommand)
	if baseCommandPath != "" && baseCommandPath == normalizedPath {
		return true
	}

	return false
}

func normalizeBaseCommandPath(baseCommand string) string {
	normalizedCommand := strings.ReplaceAll(strings.TrimSpace(baseCommand), `\`, "/")
	normalizedCommand = strings.TrimPrefix(normalizedCommand, "{{INSTALL_DIR}}/")
	if normalizedCommand == "" {
		return ""
	}
	if filepath.IsAbs(normalizedCommand) {
		return ""
	}
	if !looksLikeServerRelativeExecutable(normalizedCommand) {
		return ""
	}

	return normalizeRelativePath(normalizedCommand)
}

func normalizeRelativePath(value string) string {
	if value == "" {
		return ""
	}

	normalized := strings.ReplaceAll(value, `\`, "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = path.Clean(normalized)
	if normalized == "." {
		return ""
	}

	return normalized
}

func looksLikeServerRelativeExecutable(baseCommand string) bool {
	if strings.ContainsAny(baseCommand, `/\`) {
		return true
	}

	normalized := normalizeRelativePath(baseCommand)
	if normalized == "" {
		return false
	}
	if strings.HasPrefix(baseCommand, ".") {
		return true
	}
	if path.Ext(normalized) != "" {
		return true
	}
	if strings.ContainsAny(normalized, "-_") {
		return true
	}

	return false
}
