package com.papacs.snaptrans.ocr

import android.graphics.Bitmap
import com.google.mlkit.vision.common.InputImage
import com.google.mlkit.vision.text.TextRecognition
import com.google.mlkit.vision.text.latin.TextRecognizerOptions
import com.papacs.snaptrans.core.IntRect
import com.papacs.snaptrans.core.ScreenRegion
import com.papacs.snaptrans.core.ResourceLease
import java.util.concurrent.Executor
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Semaphore

data class OcrBitmap(val pixels: ResourceLease<Bitmap>, val scaleToScreen: Float) {
    val bitmap: Bitmap get() = pixels.value
}

class MlKitOcrEngine : AutoCloseable {
    private val recognizer = TextRecognition.getClient(TextRecognizerOptions.DEFAULT_OPTIONS)
    private val recognitionPermit = Semaphore(1)
    @Volatile private var closed = false

    suspend fun recognize(input: OcrBitmap): List<ScreenRegion> {
        recognitionPermit.acquire()
        var taskStarted = false
        try {
            check(!closed) { "OCR 已关闭" }
            return suspendCancellableCoroutine { continuation ->
                if (!continuation.isActive) return@suspendCancellableCoroutine
                val sdkPixels = input.pixels.retain()
                val task = try {
                    recognizer.process(InputImage.fromBitmap(sdkPixels.value, 0))
                } catch (error: Throwable) {
                    sdkPixels.close()
                    throw error
                }
                taskStarted = true
                // Cancelling the coroutine cannot cancel ML Kit's Task. Keep
                // its pixels and permit until the SDK has actually stopped reading.
                task.addOnCompleteListener(DIRECT_EXECUTOR) { completed ->
                    try {
                        if (!continuation.isActive) return@addOnCompleteListener
                        if (!completed.isSuccessful) {
                            continuation.resumeWithException(completed.exception ?: IllegalStateException("OCR 失败"))
                            return@addOnCompleteListener
                        }
                        val result = completed.result
                        val scale = input.scaleToScreen
                        val regions = result.textBlocks.flatMap { block ->
                            block.lines.mapNotNull { line ->
                                val rect = line.boundingBox ?: return@mapNotNull null
                                ScreenRegion(
                                    id = 0,
                                    source = line.text,
                                    bounds = IntRect(
                                        left = (rect.left * scale).toInt(),
                                        top = (rect.top * scale).toInt(),
                                        right = (rect.right * scale).toInt(),
                                        bottom = (rect.bottom * scale).toInt(),
                                    ),
                                )
                            }
                        }
                        if (continuation.isActive) continuation.resume(regions)
                    } catch (error: Throwable) {
                        if (continuation.isActive) continuation.resumeWithException(error)
                    } finally {
                        sdkPixels.close()
                        recognitionPermit.release()
                    }
                }
            }
        } finally {
            if (!taskStarted) recognitionPermit.release()
        }
    }

    override fun close() {
        closed = true
        recognizer.close()
    }

    private companion object { val DIRECT_EXECUTOR = Executor { it.run() } }
}

object BitmapOptimizer {
    fun forOcr(source: ResourceLease<Bitmap>, maxDimension: Int = 1800): OcrBitmap {
        val bitmap = source.value
        val longest = maxOf(bitmap.width, bitmap.height)
        if (longest <= maxDimension) return OcrBitmap(source.retain(), 1f)
        val factor = maxDimension.toFloat() / longest
        val scaled = Bitmap.createScaledBitmap(
            bitmap,
            (bitmap.width * factor).toInt().coerceAtLeast(1),
            (bitmap.height * factor).toInt().coerceAtLeast(1),
            true,
        )
        return OcrBitmap(ResourceLease(scaled) { it.recycle() }, 1f / factor)
    }
}
