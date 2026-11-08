package internal

import (
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var Version = "unknown"

func init() {
	if Version == "unknown" {
		var (
			vcs struct { // only set if built with `go build` or `go install` with vcs metadata available
				revision string
				time     time.Time
				modified bool
			}
			major  string // only set if non-v1 (which it shouldn't be...)
			modver string // only set if built with `go install`
		)
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if v := s.Value; len(v) == 20 || len(v) == 40 {
						vcs.revision = v
					} else {
						//panic(fmt.Errorf("parse %s %q: unexpected length %d", s.Key, s.Value, len(v)))
					}
				case "vcs.time":
					if v, err := time.ParseInLocation(time.RFC3339Nano, s.Value, time.UTC); err == nil {
						vcs.time = v
					} else {
						//panic(fmt.Errorf("parse %s %q: %w", s.Key, s.Value, err))
					}
				case "vcs.modified":
					if v, err := strconv.ParseBool(s.Value); err == nil {
						vcs.modified = v
					} else {
						//panic(fmt.Errorf("parse %s %q: %w", s.Key, s.Value, err))
					}
				}
			}
			if i := strings.LastIndex(info.Main.Path, "/"); i >= 0 && strings.Trim(strings.TrimPrefix(info.Main.Path[i+1:], "v"), "0123456789") == "" {
				major = info.Main.Path[i+1:]
			}
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				modver = info.Main.Version
			}
		}
		if modver != "" {
			Version = modver
			if vcs.modified {
				Version += " (dev)" // wouldn't be modified if it was built directly from the module
			}
		} else {
			if major == "" {
				Version = "unknown"
			} else {
				Version = major
			}
			if vcs.revision != "" {
				Version += "-" + vcs.revision[:7]
			}
			if vcs.modified {
				Version += "+dirty"
			}
			Version += " (dev)"
		}
	}
}
