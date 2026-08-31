package startargs

import (
	"path"
	"path/filepath"
	"strings"
)

// IsProtectedServerPath reports whether a relative path targets a protected
// launch binary or an ancestor of one, including the server root when a
// protection policy exists.
func IsProtectedServerPath(relativePath string, baseCommand string, serverExecutable string) bool {
	executablePath := normalizeRelativePath(serverExecutable)
	baseCommandPath := normalizeBaseCommandPath(baseCommand)
	if executablePath == "" && baseCommandPath == "" {
		return false
	}

	normalizedPath := normalizeRelativePath(relativePath)
	if normalizedPath == "" {
		return true
	}
	if protectsPath(normalizedPath, executablePath) {
		return true
	}
	if protectsPath(normalizedPath, baseCommandPath) {
		return true
	}

	return false
}

// IsReservedManagedPath reports whether a relative path is inside Xylona's
// managed BlueMap directory and must not be mutated through generic file APIs.
func IsReservedManagedPath(relativePath string) bool {
	normalizedPath := normalizeRelativePath(relativePath)
	if normalizedPath == "" {
		return false
	}
	if pathNamesEqual(normalizedPath, ".xylona") || pathNamesEqual(normalizedPath, ".xylona/bluemap") {
		return true
	}
	return hasPathPrefixFold(normalizedPath, ".xylona/bluemap/")
}

func protectsPath(candidate string, protectedPath string) bool {
	if protectedPath == "" {
		return false
	}
	if pathNamesEqual(candidate, protectedPath) {
		return true
	}

	return hasPathPrefixFold(protectedPath, candidate+"/")
}

func pathNamesEqual(left string, right string) bool {
	return strings.EqualFold(left, right)
}

func hasPathPrefixFold(value string, prefix string) bool {
	if prefix == "" || len(value) < len(prefix) {
		return false
	}
	return strings.EqualFold(value[:len(prefix)], prefix)
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
