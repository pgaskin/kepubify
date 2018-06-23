package kobo

import (
	"strconv"
	"strings"
)

// VersionCompare compares two firmware versions.
// a < b = -1
// a = b = 0
// a > b = 1
func VersionCompare(a, b string) int {
	aspl, bspl := strSplitInt(a), strSplitInt(b)
	if len(aspl) != len(bspl) {
		return 0
	}
	for i := range aspl {
		switch {
		case aspl[i] > bspl[i]:
			return 1
		case bspl[i] > aspl[i]:
			return -1
		}
	}
	return 0
}

func strSplitInt(str string) []int64 {
	spl := strings.Split(str, ".")
	ints := make([]int64, len(spl))
	for i, p := range spl {
		ints[i], _ = strconv.ParseInt(p, 10, 64)
	}
	return ints
}
