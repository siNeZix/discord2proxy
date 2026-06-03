// Package version provides dotted-numeric version parsing and comparison
// shared by the self-update check (GUI) and the lightweight setup loader.
package version

import (
	"strconv"
	"strings"
)

// IsNewer reports whether candidate is a strictly newer version than current.
// Both are dotted numeric versions; any non-numeric suffix (e.g. "-dev") is
// dropped before comparison. A "-dev" build skips the update check entirely
// regardless of version number, so a developer machine is never prompted to
// "update" to a stable release.
func IsNewer(current, candidate string) bool {
	if strings.Contains(current, "-dev") {
		// Dev builds opt out of update prompts entirely.
		return false
	}
	return Compare(Parse(candidate), Parse(current)) > 0
}

// Parse converts a dotted-numeric version (optionally "v"-prefixed, with an
// optional pre-release/build suffix) into its numeric components.
func Parse(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		nums = append(nums, n)
	}
	return nums
}

// Compare returns >0 if a is newer than b, <0 if older, 0 if equal.
func Compare(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var ai, bi int
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	return 0
}

// Equal reports whether two dotted-numeric versions are equal.
func Equal(a, b string) bool {
	return Compare(Parse(a), Parse(b)) == 0
}
