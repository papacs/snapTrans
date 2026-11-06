package com.papacs.snaptrans.overlay

import org.junit.Assert.assertEquals
import org.junit.Test

class OverlayPlacementTest {
    @Test
    fun rightEdgeMovesTheWholeMinimumWidthBoxIntoView() {
        val result = fitOverlayAxis(start = 1000f, requestedSize = 48f * 3f, viewportSize = 1080f)
        assertEquals(936f, result.start, 0f)
        assertEquals(144f, result.size, 0f)
        assertEquals(1080f, result.start + result.size, 0f)
    }

    @Test
    fun bottomEdgeMovesTheWholeTranslationUpInsteadOfClippingIt() {
        val result = fitOverlayAxis(start = 1900f, requestedSize = 180f, viewportSize = 1920f)
        assertEquals(1740f, result.start, 0f)
        assertEquals(180f, result.size, 0f)
    }

    @Test
    fun boxLargerThanTheViewportUsesOnlyTheAvailableSize() {
        val result = fitOverlayAxis(start = 70f, requestedSize = 144f, viewportSize = 80f)
        assertEquals(0f, result.start, 0f)
        assertEquals(80f, result.size, 0f)
    }

    @Test
    fun visibleSourceAnchorIsPreserved() {
        val result = fitOverlayAxis(start = 120f, requestedSize = 320f, viewportSize = 1080f)
        assertEquals(120f, result.start, 0f)
        assertEquals(320f, result.size, 0f)
    }

    @Test
    fun sourceAboveTheViewportStartsAtItsVisibleEdge() {
        val result = fitOverlayAxis(start = -40f, requestedSize = 180f, viewportSize = 1920f)
        assertEquals(0f, result.start, 0f)
        assertEquals(180f, result.size, 0f)
    }

    @Test
    fun emptyViewportDoesNotProduceAnInvalidRange() {
        val result = fitOverlayAxis(start = 120f, requestedSize = 144f, viewportSize = 0f)
        assertEquals(0f, result.start, 0f)
        assertEquals(0f, result.size, 0f)
    }
}
