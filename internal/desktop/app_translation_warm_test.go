package desktop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTranslationWarmStateThrottlesSuccessfulWarmups(t *testing.T) {
	var state translationWarmState
	startedAt := time.Unix(1000, 0)

	require.True(t, state.begin("https://gateway.example/v1", startedAt))
	require.False(t, state.begin("https://gateway.example/v1", startedAt.Add(time.Second)))

	state.finish("https://gateway.example/v1", startedAt.Add(2*time.Second), true)
	require.False(t, state.begin("https://gateway.example/v1", startedAt.Add(59*time.Second)))
	require.True(t, state.begin("https://gateway.example/v1", startedAt.Add(63*time.Second)))
}

func TestTranslationWarmStateRetriesFailuresAndEndpointChanges(t *testing.T) {
	var state translationWarmState
	startedAt := time.Unix(1000, 0)

	require.True(t, state.begin("https://first.example/v1", startedAt))
	state.finish("https://first.example/v1", startedAt.Add(time.Second), false)
	require.True(t, state.begin("https://first.example/v1", startedAt.Add(2*time.Second)))
	require.True(t, state.begin("https://second.example/v1", startedAt.Add(3*time.Second)))
}
