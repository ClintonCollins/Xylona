// Package updater resolves and verifies Xylona release artifacts.
package updater

import (
	"strconv"
	"strings"
)

// CompareVersions compares two release version strings. It returns 1 when a
// is newer than b, -1 when it is older, and 0 when they compare equal.
func CompareVersions(a string, b string) int {
	left := normalizeVersion(a)
	right := normalizeVersion(b)
	for i := range 3 {
		if left.nums[i] > right.nums[i] {
			return 1
		}
		if left.nums[i] < right.nums[i] {
			return -1
		}
	}
	if left.prerelease == right.prerelease {
		return 0
	}
	if left.prerelease == "" {
		return 1
	}
	if right.prerelease == "" {
		return -1
	}
	if left.prerelease > right.prerelease {
		return 1
	}
	return -1
}

type normalizedVersion struct {
	nums       [3]int
	prerelease string
}

func normalizeVersion(v string) normalizedVersion {
	trimmed := strings.TrimSpace(strings.TrimPrefix(v, "v"))
	base := trimmed
	prerelease := ""
	dash := strings.Index(base, "-")
	if dash >= 0 {
		prerelease = base[dash+1:]
		base = base[:dash]
	}
	parts := strings.Split(base, ".")
	var nums [3]int
	for i := 0; i < len(parts) && i < len(nums); i++ {
		n, errAtoi := strconv.Atoi(numericPrefix(parts[i]))
		if errAtoi != nil {
			continue
		}
		nums[i] = n
	}
	return normalizedVersion{nums: nums, prerelease: prerelease}
}

func numericPrefix(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "0"
	}
	return b.String()
}
