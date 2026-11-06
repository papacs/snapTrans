package com.papacs.snaptrans.overlay

internal data class OverlayAxisPlacement(val start: Float, val size: Float)

internal fun fitOverlayAxis(start: Float, requestedSize: Float, viewportSize: Float): OverlayAxisPlacement {
    val available = viewportSize.coerceAtLeast(0f)
    val size = requestedSize.coerceIn(0f, available)
    return OverlayAxisPlacement(start.coerceIn(0f, available - size), size)
}
