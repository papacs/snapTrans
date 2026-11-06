package com.papacs.snaptrans.capture

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.PixelFormat
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.Image
import android.media.ImageReader
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.Looper
import android.util.DisplayMetrics
import android.view.WindowManager
import com.papacs.snaptrans.core.ResourceLease
import com.papacs.snaptrans.core.resumeWithResource
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellableContinuation
import kotlinx.coroutines.suspendCancellableCoroutine

class ScreenCaptureSession(
    context: Context,
    resultCode: Int,
    resultData: Intent,
    private val onStopped: () -> Unit,
) : AutoCloseable {
    init { require(resultCode == Activity.RESULT_OK) { "未获得屏幕捕获授权" } }

    private val context = context.applicationContext
    private var dimensions = screenDimensions(this.context)
    private var capturedDimensions: Dimensions? = null
    private val captureThread = HandlerThread("snaptrans-capture").apply { start() }
    private val handler = Handler(captureThread.looper)
    private val closed = AtomicBoolean(false)
    private val pending = CaptureRequestSlot<CancellableContinuation<ResourceLease<Bitmap>>>()
    private var reader: ImageReader? = null
    @Volatile private var projection: MediaProjection? = null
    @Volatile private var virtualDisplay: VirtualDisplay? = null

    private val callback = object : MediaProjection.Callback() {
        override fun onStop() {
            if (!closed.get()) { close(); onStopped() }
        }

        override fun onCapturedContentResize(width: Int, height: Int) {
            if (closed.get() || width <= 0 || height <= 0) return
            capturedDimensions = Dimensions(width, height, dimensions.densityDpi)
            if (reader != null) {
                val current = pending.armedPending
                runCatching {
                    pending.disarm()
                    virtualDisplay?.surface = null
                    prepareReader(requireNotNull(capturedDimensions))
                    reader?.acquireLatestImage()?.close()
                    if (current != null && pending.arm(current)) virtualDisplay?.surface = reader?.surface
                }.onFailure { error -> if (current != null) failPending(error, current) }
            }
        }
    }

    init {
        try {
            val manager = context.getSystemService(MediaProjectionManager::class.java)
            val granted = requireNotNull(manager.getMediaProjection(resultCode, resultData))
            projection = granted
            check(!closed.get()) { "屏幕捕获授权已结束" }
            granted.registerCallback(callback, handler)
            // Keep one virtual display per consent session. Detaching its
            // surface while idle avoids continuously mirroring the screen.
            val display = requireNotNull(granted.createVirtualDisplay(
                "snapTrans-screen", dimensions.width, dimensions.height, dimensions.densityDpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, null, null, handler,
            ))
            virtualDisplay = display
            if (closed.get()) {
                display.release()
                error("屏幕捕获授权已结束")
            }
        } catch (error: Throwable) {
            close()
            throw error
        }
    }

    suspend fun capture(): ResourceLease<Bitmap> = suspendCancellableCoroutine { continuation ->
        if (closed.get()) {
            continuation.resumeWithException(IllegalStateException("屏幕捕获授权已结束"))
            return@suspendCancellableCoroutine
        }
        if (!pending.register(continuation)) {
            continuation.resumeWithException(IllegalStateException("已有截图任务正在运行"))
            return@suspendCancellableCoroutine
        }
        if (closed.get()) {
            if (pending.cancel(continuation)) continuation.resumeWithException(
                IllegalStateException("屏幕捕获授权已结束"),
            )
            return@suspendCancellableCoroutine
        }
        continuation.invokeOnCancellation {
            if (pending.cancel(continuation)) handler.post {
                pending.disarm(continuation)
                if (pending.pending == null) {
                    virtualDisplay?.surface = null
                }
            }
        }
        val posted = handler.post {
            if (closed.get() || !pending.isPending(continuation) || !continuation.isActive) return@post
            try {
                pending.disarm()
                virtualDisplay?.surface = null
                prepareReader(capturedDimensions ?: screenDimensions(context))
                reader?.acquireLatestImage()?.close()
                // Attaching after the bubble was hidden requests a fresh frame
                // even when the screen would otherwise remain completely static.
                if (pending.arm(continuation)) virtualDisplay?.surface = reader?.surface
            } catch (error: Throwable) { failPending(error, continuation) }
        }
        if (!posted && pending.cancel(continuation) && continuation.isActive) {
            continuation.resumeWithException(IllegalStateException("屏幕捕获已关闭"))
        }
    }

    private fun prepareReader(size: Dimensions) {
        if (reader != null && size == dimensions) return
        val replacement = ImageReader.newInstance(size.width, size.height, PixelFormat.RGBA_8888, 2)
        try {
            replacement.setOnImageAvailableListener(::onImageAvailable, handler)
            virtualDisplay?.surface = null
            virtualDisplay?.resize(size.width, size.height, size.densityDpi)
            reader?.setOnImageAvailableListener(null, null)
            reader?.close()
            reader = replacement
            dimensions = size
        } catch (error: Throwable) {
            replacement.close()
            throw error
        }
    }

    private fun onImageAvailable(imageReader: ImageReader) {
        val image = runCatching { imageReader.acquireLatestImage() }.getOrNull() ?: return
        image.use {
            if (closed.get() || imageReader !== reader) return
            val continuation = pending.claimArmed() ?: return
            virtualDisplay?.surface = null
            if (!continuation.isActive) return
            try {
                val bitmap = ResourceLease(it.toBitmap()) { pixels -> pixels.recycle() }
                continuation.resumeWithResource(bitmap)
            } catch (error: Throwable) {
                if (continuation.isActive) continuation.resumeWithException(error)
            }
        }
    }

    private fun failPending(error: Throwable, expected: CancellableContinuation<ResourceLease<Bitmap>>) {
        if (!pending.cancel(expected)) return
        pending.disarm(expected)
        virtualDisplay?.surface = null
        if (expected.isActive) expected.resumeWithException(error)
    }

    fun onDisplayChanged() {
        handler.post { capturedDimensions = null }
    }

    override fun close() {
        if (!closed.compareAndSet(false, true)) return
        pending.clear()?.cancel()
        val cleanup = Runnable {
            pending.disarm()
            runCatching { reader?.setOnImageAvailableListener(null, null) }
            runCatching { virtualDisplay?.release() }
            runCatching { reader?.close() }
            runCatching { projection?.unregisterCallback(callback) }
            runCatching { projection?.stop() }
            virtualDisplay = null
            reader = null
            projection = null
            captureThread.quitSafely()
        }
        if (Looper.myLooper() == handler.looper || !handler.post(cleanup)) cleanup.run()
    }

    private fun Image.toBitmap(): Bitmap {
        val plane = planes.first()
        val pixels = packRgbaPlane(plane.buffer, width, height, plane.pixelStride, plane.rowStride)
        val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
        try {
            bitmap.copyPixelsFromBuffer(pixels)
            return bitmap
        } catch (error: Throwable) {
            bitmap.recycle()
            throw error
        }
    }

    private data class Dimensions(val width: Int, val height: Int, val densityDpi: Int)

    @Suppress("DEPRECATION")
    private fun screenDimensions(context: Context): Dimensions {
        val density = context.resources.displayMetrics.densityDpi
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val bounds = context.getSystemService(WindowManager::class.java).maximumWindowMetrics.bounds
            return Dimensions(bounds.width(), bounds.height(), density)
        }
        val metrics = DisplayMetrics()
        context.getSystemService(WindowManager::class.java).defaultDisplay.getRealMetrics(metrics)
        return Dimensions(metrics.widthPixels, metrics.heightPixels, metrics.densityDpi)
    }
}
