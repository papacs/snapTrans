package desktop

import (
	"runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildVersionDistinguishesUncommittedBuildsAndReleaseStamp(t *testing.T) {
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "1234567890abcdef"}, {Key: "vcs.modified", Value: "true"}}}
	first := versionWithBuild("1.2.3", "", info, time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC))
	second := versionWithBuild("1.2.3", "", info, time.Date(2026, 10, 28, 8, 1, 0, 0, time.UTC))
	require.NotEqual(t, first, second)
	require.Contains(t, first, "12345678-dirty")
	require.Equal(t, "1.2.3 · release-source123", versionWithBuild("1.2.3", "release-source123", info, time.Time{}))
	require.Equal(t, "1.2.3 · local", versionWithBuild("1.2.3", "", nil, time.Time{}))
}
