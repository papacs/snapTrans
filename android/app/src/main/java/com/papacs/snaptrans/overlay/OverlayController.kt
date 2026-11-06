package com.papacs.snaptrans.overlay

import android.annotation.SuppressLint
import android.animation.ValueAnimator
import android.content.Context
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.drawable.GradientDrawable
import android.hardware.input.InputManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.view.animation.DecelerateInterpolator
import android.widget.TextView
import com.papacs.snaptrans.core.TranslatedRegion
import com.papacs.snaptrans.core.TranslationState
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.math.abs

class OverlayController(
    private val context: Context,
    private val onBubbleTap: () -> Unit,
) {
    private val mainHandler = Handler(Looper.getMainLooper())
    private val windowManager = context.getSystemService(WindowManager::class.java)
    private val density = context.resources.displayMetrics.density
    private val overlayView = TranslationOverlayView(context).apply { visibility = View.GONE }
    private val bubble = BubbleTextView(context).apply {
        gravity = Gravity.CENTER
        setTextColor(Color.WHITE)
        textSize = 20f
        elevation = 12f * density
        contentDescription = "翻译当前屏幕"
        background = bubbleBackground(Color.rgb(37, 99, 235))
    }
    private val overlayParams = WindowManager.LayoutParams(
        WindowManager.LayoutParams.MATCH_PARENT,
        WindowManager.LayoutParams.MATCH_PARENT,
        WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
        WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
            WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE or
            WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN or
            WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
        PixelFormat.TRANSLUCENT,
    ).apply {
        gravity = Gravity.TOP or Gravity.START
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            // Android 12 blocks touches through an opaque, untrusted overlay.
            alpha = context.getSystemService(InputManager::class.java).maximumObscuringOpacityForTouch
        }
    }
    private val bubbleSize = (58 * density).toInt()
    private val renderScheduled = AtomicBoolean(false)
    @Volatile private var pendingRegions: List<TranslatedRegion> = emptyList()
    private val renderRunnable = object : Runnable {
        override fun run() {
            val rendering = pendingRegions
            overlayView.render(rendering)
            renderScheduled.set(false)
            if (pendingRegions !== rendering) scheduleRender()
        }
    }
    private val bubbleParams = WindowManager.LayoutParams(
        bubbleSize,
        bubbleSize,
        WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
        WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
            WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN,
        PixelFormat.TRANSLUCENT,
    ).apply {
        gravity = Gravity.TOP or Gravity.START
        x = context.resources.displayMetrics.widthPixels - bubbleSize - (12 * density).toInt()
        y = (220 * density).toInt()
    }
    private var attached = false
    private var snapAnimator: ValueAnimator? = null

    fun attach() = onMain {
        if (attached) return@onMain
        try {
            windowManager.addView(overlayView, overlayParams)
            windowManager.addView(bubble, bubbleParams)
        } catch (error: Throwable) {
            runCatching { windowManager.removeView(bubble) }
            runCatching { windowManager.removeView(overlayView) }
            throw error
        }
        bubble.setOnClickListener { onBubbleTap() }
        bubble.setOnTouchListener(BubbleDragListener())
        attached = true
        setState(TranslationState.Idle)
    }

    fun detach() = onMain {
        mainHandler.removeCallbacksAndMessages(null)
        pendingRegions = emptyList()
        renderScheduled.set(false)
        overlayView.clear()
        snapAnimator?.cancel()
        snapAnimator = null
        bubble.animate().cancel()
        if (!attached) return@onMain
        runCatching { windowManager.removeView(overlayView) }
        runCatching { windowManager.removeView(bubble) }
        attached = false
    }

    fun onDisplayChanged() = onMain {
        snapAnimator?.cancel()
        bubbleParams.x = bubbleParams.x.coerceIn(0, (screenWidth() - bubbleSize).coerceAtLeast(0))
        bubbleParams.y = bubbleParams.y.coerceIn(0, (screenHeight() - bubbleSize).coerceAtLeast(0))
        if (attached) windowManager.updateViewLayout(bubble, bubbleParams)
    }

    fun setState(state: TranslationState) = onMain {
        val (label, color, description) = when (state) {
            TranslationState.Idle -> Triple("译", Color.rgb(37, 99, 235), "翻译当前屏幕")
            TranslationState.Capturing -> Triple("…", Color.rgb(14, 165, 233), "正在截取屏幕")
            TranslationState.Recognizing -> Triple("识", Color.rgb(14, 165, 233), "正在识别英文")
            is TranslationState.Translating -> Triple(
                if (state.total > 0) "${state.completed}/${state.total}" else "译",
                Color.rgb(124, 58, 237),
                "正在翻译，点击取消",
            )
            is TranslationState.Showing -> Triple("×", Color.rgb(15, 23, 42), "关闭翻译")
            is TranslationState.Error -> Triple("!", Color.rgb(220, 38, 38), state.message)
        }
        bubble.text = label
        bubble.textSize = if (label.length > 2) 11f else 20f
        bubble.contentDescription = description
        bubble.background = bubbleBackground(color)
    }

    fun showTranslations(regions: List<TranslatedRegion>) {
        if (!attached) return
        pendingRegions = regions
        scheduleRender()
    }

    fun clearTranslations() = onMain {
        pendingRegions = emptyList()
        mainHandler.removeCallbacks(renderRunnable)
        renderScheduled.set(false)
        overlayView.clear()
    }

    fun hideForCapture() = onMain {
        overlayView.visibility = View.INVISIBLE
        bubble.visibility = View.INVISIBLE
    }

    fun restoreBubble() = onMain {
        bubble.visibility = View.VISIBLE
        if (overlayView.visibility == View.INVISIBLE) overlayView.visibility = View.GONE
    }

    private fun bubbleBackground(color: Int) = GradientDrawable().apply {
        shape = GradientDrawable.OVAL
        setColor(color)
        setStroke((2 * density).toInt(), Color.argb(150, 255, 255, 255))
    }

    private fun onMain(block: () -> Unit) {
        if (Looper.myLooper() == Looper.getMainLooper()) block() else mainHandler.post(block)
    }

    private fun scheduleRender() {
        if (renderScheduled.compareAndSet(false, true)) {
            mainHandler.postDelayed(renderRunnable, 32)
        }
    }

    private inner class BubbleDragListener : View.OnTouchListener {
        private var downRawX = 0f
        private var downRawY = 0f
        private var startX = 0
        private var startY = 0

        override fun onTouch(view: View, event: MotionEvent): Boolean {
            when (event.actionMasked) {
                MotionEvent.ACTION_DOWN -> {
                    snapAnimator?.cancel()
                    downRawX = event.rawX
                    downRawY = event.rawY
                    startX = bubbleParams.x
                    startY = bubbleParams.y
                    view.animate().scaleX(0.94f).scaleY(0.94f).setDuration(80).start()
                    return true
                }
                MotionEvent.ACTION_MOVE -> {
                    val dx = event.rawX - downRawX
                    val dy = event.rawY - downRawY
                    bubbleParams.x = (startX + dx).toInt().coerceIn(0, (screenWidth() - bubbleSize).coerceAtLeast(0))
                    bubbleParams.y = (startY + dy).toInt().coerceIn(0, (screenHeight() - bubbleSize).coerceAtLeast(0))
                    if (attached) windowManager.updateViewLayout(bubble, bubbleParams)
                    return true
                }
                MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                    view.animate().scaleX(1f).scaleY(1f).setDuration(100).start()
                    val moved = abs(event.rawX - downRawX) + abs(event.rawY - downRawY)
                    if (event.actionMasked == MotionEvent.ACTION_UP && moved < 12f * density) view.performClick()
                    else snapToEdge()
                    return true
                }
            }
            return false
        }
    }

    private fun snapToEdge() {
        val margin = (12 * density).toInt()
        val target = if (bubbleParams.x + bubbleSize / 2 < screenWidth() / 2) {
            margin
        } else {
            screenWidth() - bubbleSize - margin
        }
        snapAnimator?.cancel()
        snapAnimator = ValueAnimator.ofInt(bubbleParams.x, target.coerceIn(0, (screenWidth() - bubbleSize).coerceAtLeast(0))).apply {
            duration = 180
            interpolator = DecelerateInterpolator()
            addUpdateListener {
                bubbleParams.x = it.animatedValue as Int
                if (attached) windowManager.updateViewLayout(bubble, bubbleParams)
            }
            start()
        }
    }

    private fun screenWidth() = context.resources.displayMetrics.widthPixels
    private fun screenHeight() = context.resources.displayMetrics.heightPixels
}

@SuppressLint("AppCompatCustomView")
private class BubbleTextView(context: Context) : TextView(context) {
    override fun performClick(): Boolean {
        super.performClick()
        return true
    }
}
