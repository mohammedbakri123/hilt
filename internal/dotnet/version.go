package dotnet

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a .NET-style version: major.minor.patch[.revision][-prerelease].
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Revision   int
	PreRelease string
	Original   string
}

// ParseVersion parses a .NET SDK or runtime version.
func ParseVersion(value string) (Version, error) {
	original := strings.TrimSpace(value)
	if original == "" {
		return Version{}, fmt.Errorf("empty version")
	}

	core := original
	preRelease := ""
	if index := strings.Index(core, "-"); index >= 0 {
		preRelease = core[index+1:]
		core = core[:index]
		if preRelease == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty prerelease", original)
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return Version{}, fmt.Errorf("invalid version %q: expected 1 to 4 numeric segments", original)
	}

	numbers := [4]int{}
	for i, part := range parts {
		if part == "" {
			return Version{}, fmt.Errorf("invalid version %q: empty segment", original)
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return Version{}, fmt.Errorf("invalid version %q: segment %q is not numeric", original, part)
		}
		numbers[i] = number
	}

	return Version{
		Major:      numbers[0],
		Minor:      numbers[1],
		Patch:      numbers[2],
		Revision:   numbers[3],
		PreRelease: preRelease,
		Original:   original,
	}, nil
}

// String renders the canonical numeric form plus prerelease.
func (v Version) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Revision != 0 {
		base = fmt.Sprintf("%s.%d", base, v.Revision)
	}
	if v.PreRelease != "" {
		return base + "-" + v.PreRelease
	}
	return base
}

// IsPrerelease reports whether the version carries a prerelease label.
func (v Version) IsPrerelease() bool {
	return v.PreRelease != ""
}

// Compare returns -1, 0, or 1.
func (v Version) Compare(other Version) int {
	if result := compareInt(v.Major, other.Major); result != 0 {
		return result
	}
	if result := compareInt(v.Minor, other.Minor); result != 0 {
		return result
	}
	if result := compareInt(v.Patch, other.Patch); result != 0 {
		return result
	}
	if result := compareInt(v.Revision, other.Revision); result != 0 {
		return result
	}
	return comparePrerelease(v.PreRelease, other.PreRelease)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if result := compareIdentifier(aParts[i], bParts[i]); result != 0 {
			return result
		}
	}
	return compareInt(len(aParts), len(bParts))
}

func compareIdentifier(a, b string) int {
	aNumber, aErr := strconv.Atoi(a)
	bNumber, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return compareInt(aNumber, bNumber)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
