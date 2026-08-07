package singleinstance

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAcquireCreatesMutex(t *testing.T) {
	name := "snapTrans.test.single-instance"
	instance, err := Acquire(name)
	require.NoError(t, err)
	require.NotNil(t, instance)
	require.NoError(t, instance.Release())
}

func TestAcquireRejectsSecondInstance(t *testing.T) {
	name := "snapTrans.test.single-instance-second"
	first, err := Acquire(name)
	require.NoError(t, err)
	defer first.Release()

	second, err := Acquire(name)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrAlreadyRunning), "expected ErrAlreadyRunning, got %v", err)
	require.Nil(t, second)
}

func TestReleaseAllowsReacquire(t *testing.T) {
	name := "snapTrans.test.single-instance-reacquire"
	first, err := Acquire(name)
	require.NoError(t, err)
	require.NoError(t, first.Release())

	second, err := Acquire(name)
	require.NoError(t, err)
	require.NoError(t, second.Release())
}

func TestReleaseIsIdempotent(t *testing.T) {
	name := "snapTrans.test.single-instance-idempotent"
	instance, err := Acquire(name)
	require.NoError(t, err)

	require.NoError(t, instance.Release())
	require.NoError(t, instance.Release())
	require.NoError(t, (*Instance)(nil).Release())
}
