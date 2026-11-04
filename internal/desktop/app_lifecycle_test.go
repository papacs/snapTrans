package desktop

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/codex"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"
)

func TestShutdownCancelsBackgroundWorkAndRejectsLateAdmissions(t *testing.T) {
	app := NewApp(nil)
	_, epoch, _ := app.beginCapture()
	processing, cancelProcessing := app.newProcessingContext(epoch)
	defer cancelProcessing()
	extraction, cancelExtraction := app.newExtractionContext(epoch)
	defer cancelExtraction()
	action, cancelAction, _ := app.newTextActionContext("reply", epoch, time.Minute)
	defer cancelAction()
	_, warmParent := app.translationWarmSnapshot()

	app.stopBackgroundWork()
	require.ErrorIs(t, processing.Err(), context.Canceled)
	require.ErrorIs(t, extraction.Err(), context.Canceled)
	require.ErrorIs(t, action.Err(), context.Canceled)
	require.ErrorIs(t, warmParent.Err(), context.Canceled)
	require.False(t, app.captureRequestCurrent(epoch))
	_, _, admitted := app.beginCapture()
	require.False(t, admitted)
	ctx, cancel := app.newProcessingContext(app.currentCaptureEpoch())
	require.Nil(t, ctx)
	require.Nil(t, cancel)
	ctx, cancel = app.newExtractionContext(app.currentCaptureEpoch())
	require.Nil(t, ctx)
	require.Nil(t, cancel)
	ctx, cancel, _ = app.newTextActionContext("late", app.currentCaptureEpoch(), time.Minute)
	require.Nil(t, ctx)
	require.Nil(t, cancel)
	_, warmParent = app.translationWarmSnapshot()
	require.Nil(t, warmParent)
	_, err := app.ExtractText("data:image/png;base64,not-decoded-after-shutdown")
	require.ErrorIs(t, err, context.Canceled)
}

func TestQueuedWarmupCannotRestartAfterCancellation(t *testing.T) {
	app := NewApp(nil)
	_, queued := app.translationWarmSnapshot()
	app.cancelTranslationWarmups()
	require.ErrorIs(t, queued.Err(), context.Canceled)
	// Simulate a goroutine that first runs after settings invalidated the pool.
	// Cancellation must win before even resolving the deliberately invalid path.
	err := codex.WarmSession(queued, filepath.Join(t.TempDir(), "not-an-executable.exe"), "test-model", "translate only", "low")
	require.ErrorIs(t, err, context.Canceled)
	_, current := app.translationWarmSnapshot()
	require.NoError(t, current.Err())
	app.stopBackgroundWork()
	require.ErrorIs(t, current.Err(), context.Canceled)
}

func TestTranslationSettingsInvalidatePendingWorkButHistoryPreferenceDoesNot(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	app.cfg.PersistentOCR = false
	app.configStore = config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"), "")
	_, epoch, _ := app.beginCapture()
	processing, cancelProcessing := app.newProcessingContext(epoch)
	defer cancelProcessing()
	extraction, cancelExtraction := app.newExtractionContext(epoch)
	defer cancelExtraction()
	action, cancelAction, _ := app.newTextActionContext("reply", epoch, time.Minute)
	defer cancelAction()
	_, warmParent := app.translationWarmSnapshot()
	defer app.cancelTranslationWarmups()

	next := app.cfg
	next.RecordHistory = !next.RecordHistory
	require.NoError(t, app.SaveConfig(next))
	require.NoError(t, processing.Err())
	require.NoError(t, extraction.Err())
	require.NoError(t, action.Err())
	require.ErrorIs(t, warmParent.Err(), context.Canceled)
	next.SystemPrompt = "new translation instructions"
	require.NoError(t, app.SaveConfig(next))
	require.ErrorIs(t, processing.Err(), context.Canceled)
	require.ErrorIs(t, extraction.Err(), context.Canceled)
	require.ErrorIs(t, action.Err(), context.Canceled)
	late, cancelLate := app.newProcessingContext(epoch)
	require.Nil(t, late)
	require.Nil(t, cancelLate)

	app.stopBackgroundWork()
	require.ErrorIs(t, app.saveConfigAndShortcuts(next, next), context.Canceled)
}

func TestCanceledOCRWorkflowDoesNotEmitOrStartEngine(t *testing.T) {
	app := NewApp(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A nil Wails context deliberately catches any event emitted by an obsolete
	// goroutine; the canceled workflow must return before touching the runtime.
	app.processImage(ctx, config.Default(), "not-an-image", translator.DirectionToChinese, 1)
}

func TestStandaloneOCRContextsCancelTogetherWithoutCancelingEachOther(t *testing.T) {
	app := NewApp(nil)
	_, epoch, _ := app.beginCapture()
	first, cancelFirst := app.newExtractionContext(epoch)
	second, cancelSecond := app.newExtractionContext(epoch)
	defer cancelSecond()
	cancelFirst()
	require.NoError(t, second.Err())
	require.NoError(t, app.HideWindow())
	require.ErrorIs(t, first.Err(), context.Canceled)
	require.ErrorIs(t, second.Err(), context.Canceled)
	late, cancel := app.newExtractionContext(epoch)
	require.Nil(t, late)
	require.Nil(t, cancel)
}

func TestOldTextActionWithReusedIDCannotClearOrEmitIntoNewAction(t *testing.T) {
	app := NewApp(nil)
	first, cancelFirst, oldRevision := app.newTextActionContext("same-id", 0, time.Minute)
	defer cancelFirst()
	current, cancelCurrent, revision := app.newTextActionContext("same-id", 0, time.Minute)
	defer cancelCurrent()
	require.ErrorIs(t, first.Err(), context.Canceled)
	app.finishTextAction(oldRevision)
	require.False(t, app.textActionCurrent(oldRevision))
	require.True(t, app.textActionCurrent(revision))
	require.NoError(t, current.Err())
	require.NoError(t, app.CancelTextAction("same-id"))
	require.ErrorIs(t, current.Err(), context.Canceled)

	app.invalidateCaptureRequest()
	late, cancelLate, _ := app.newTextActionContext("stale-frame", 0, time.Minute)
	require.Nil(t, late)
	require.Nil(t, cancelLate)
}

func TestShutdownCancelsSettingsConnectionCheckAndRejectsLateCodexCalls(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	app := NewApp(nil)
	draft := config.Default()
	draft.APIKey, draft.BaseURL = "test-key", server.URL
	finished := make(chan error, 1)
	go func() { finished <- app.TestConnection(draft) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("connection check did not reach the local test server")
	}
	app.stopBackgroundWork()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the connection check")
	}
	_, err := app.GetCodexStatus("not-an-executable")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, app.LoginCodex("not-an-executable"), context.Canceled)
	require.ErrorIs(t, app.TestConnection(draft), context.Canceled)
}

func TestLifetimeContextSupportsZeroValueAppFixture(t *testing.T) {
	app := &App{}
	parent := app.lifetimeContext()
	require.NoError(t, parent.Err())
	processing, cancel := app.newProcessingContext(0)
	defer cancel()
	app.stopBackgroundWork()
	app.stopBackgroundWork()
	require.ErrorIs(t, parent.Err(), context.Canceled)
	require.ErrorIs(t, processing.Err(), context.Canceled)
	require.ErrorIs(t, app.lifetimeContext().Err(), context.Canceled)
}
