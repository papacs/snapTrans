package desktop

import (
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// Release builds inject a timestamp and source digest with -ldflags -X.
var buildID string

func buildVersion() string {
	info, _ := debug.ReadBuildInfo()
	var executableTime time.Time
	if path, err := os.Executable(); err == nil {
		if file, err := os.Stat(path); err == nil {
			executableTime = file.ModTime()
		}
	}
	return versionWithBuild(appVersion, buildID, info, executableTime)
}

func versionWithBuild(version, id string, info *debug.BuildInfo, executableTime time.Time) string {
	if id != "" {
		return version + " · " + id
	}
	var revision, modified string
	if info != nil {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					modified = "-dirty"
				}
			}
		}
	}
	if len(revision) > 8 {
		revision = revision[:8]
	}
	identity := strings.TrimSpace(revision + modified)
	if identity == "" {
		identity = "local"
	}
	// A dirty checkout can produce many different builds of the same commit.
	if !executableTime.IsZero() {
		identity += "-" + executableTime.UTC().Format("20060102T150405Z")
	}
	return version + " · " + identity
}
