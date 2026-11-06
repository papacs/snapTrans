package com.papacs.snaptrans.overlay

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.RectF
import android.text.Layout
import android.text.StaticLayout
import android.text.TextPaint
import android.util.AttributeSet
import android.view.View
import com.papacs.snaptrans.core.TranslatedRegion
import kotlin.math.max

class TranslationOverlayView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {
    private val density = resources.displayMetrics.density
    private val locationOnScreen = IntArray(2)
    private val drawRect = RectF()
    private val background = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.rgb(15, 23, 42) }
    private val border = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = Color.rgb(59, 130, 246)
        style = Paint.Style.STROKE
        strokeWidth = density
    }
    private val textPaint = TextPaint(Paint.ANTI_ALIAS_FLAG).apply {
        color = Color.WHITE
        typeface = android.graphics.Typeface.create("sans-serif", android.graphics.Typeface.NORMAL)
    }
    private var regions: List<TranslatedRegion> = emptyList()

    fun render(value: List<TranslatedRegion>) {
        regions = value
        visibility = if (value.isEmpty()) GONE else VISIBLE
        invalidate()
    }

    fun clear() = render(emptyList())

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (width <= 0 || height <= 0) return
        getLocationOnScreen(locationOnScreen)
        regions.forEach { translated ->
            val source = translated.region.bounds
            val desiredWidth = max(source.width.toFloat(), translated.translation.length * 11f * density)
            val horizontal = fitOverlayAxis(
                (source.left - locationOnScreen[0]).toFloat(),
                desiredWidth.coerceIn(48f * density, 320f * density),
                width.toFloat(),
            )
            val padding = 5f * density
            val fontSize = (source.height * 0.64f).coerceIn(12f * density, 20f * density)
            textPaint.textSize = fontSize
            val layout = StaticLayout.Builder.obtain(
                translated.translation,
                0,
                translated.translation.length,
                textPaint,
                (horizontal.size - padding * 2).toInt().coerceAtLeast(1),
            ).setAlignment(Layout.Alignment.ALIGN_NORMAL)
                .setIncludePad(false)
                .setLineSpacing(0f, 1.04f)
                .setMaxLines(4)
                .setEllipsize(android.text.TextUtils.TruncateAt.END)
                .build()
            val boxHeight = max(source.height.toFloat(), layout.height + padding * 2)
            val vertical = fitOverlayAxis(
                (source.top - locationOnScreen[1]).toFloat(),
                boxHeight,
                height.toFloat(),
            )
            drawRect.set(horizontal.start, vertical.start, horizontal.start + horizontal.size, vertical.start + vertical.size)

            background.alpha = 242
            canvas.drawRoundRect(drawRect, 7f * density, 7f * density, background)
            canvas.drawRoundRect(drawRect, 7f * density, 7f * density, border)
            canvas.save()
            canvas.clipRect(drawRect)
            canvas.translate(drawRect.left + padding, drawRect.top + padding)
            layout.draw(canvas)
            canvas.restore()
        }
    }
}
