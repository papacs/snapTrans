# snapTrans Android

Android 26+ client for translating the currently visible screen from English to
Simplified Chinese. OCR runs locally; only filtered English text is sent to the
configured OpenAI-compatible endpoint.

## Current flow

1. Grant draw-over-other-apps permission.
2. Save an API key, endpoint and model.
3. Start screen translation and accept Android's screen-capture prompt.
4. Tap the floating `译` button on any app.
5. Tap `×` to remove the translation overlay.

The Quick Settings tile provides the same start/toggle action. Android requires
fresh system consent whenever a new media-projection session is created.

## Build

```powershell
cd android
.\gradlew.bat testDebugUnitTest lintDebug assembleDebug assembleRelease
```

The debug APK is written to `app/build/outputs/apk/debug/app-debug.apk`.
The optimized unsigned release APK is written to
`app/build/outputs/apk/release/app-release-unsigned.apk`.

## Privacy and performance

- Screenshots are processed in memory and are not saved or uploaded.
- OCR uses the bundled ML Kit Latin model.
- Only text classified as English-like is sent for translation.
- Common UI words and repeated lines use local caches.
- OCR input is capped at 1800 px on its longest edge.
- A capture session is serialized; tapping `×` cancels OCR/network work.
- The virtual display has no capture surface attached between requests, reducing
  idle work and avoiding missed frames on a completely static screen.
- Captured pixels are released before translation starts. An in-flight ML Kit
  task retains its own bitmap lease until completion, even after cancellation.
- Rotation clears stale overlays and updates capture dimensions without reusing
  a media-projection consent token to create another virtual display.
- Android 12+ translation windows use the system's allowed obscuring opacity
  so touch events can pass through to the underlying app.
- Release builds require HTTPS. Debug builds additionally allow cleartext only
  to the Android emulator host `10.0.2.2`; lookalike hosts are rejected.

The JVM regression suite covers text selection, streamed protocol handling,
translation caching, cancellation, and bitmap ownership. Device checks still
need a connected Android device: grant/revoke capture and overlay permissions,
rotate during OCR/streaming, repeat capture/cancel, and inspect overlay alignment.
Building the APK does not install it or change device settings.

Lifecycle behavior follows the official [MediaProjection guide](https://developer.android.com/media/grow/media-projection)
and [ML Kit task completion guidance](https://developers.google.com/ml-kit/vision/text-recognition/v2/android).
Touch passthrough follows the [Android 12 overlay rules](https://developer.android.com/about/versions/12/behavior-changes-all#untrusted-touch-events).
