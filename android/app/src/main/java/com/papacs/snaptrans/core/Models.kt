package com.papacs.snaptrans.core

data class IntRect(
    val left: Int,
    val top: Int,
    val right: Int,
    val bottom: Int,
) {
    val width: Int get() = (right - left).coerceAtLeast(0)
    val height: Int get() = (bottom - top).coerceAtLeast(0)
}

data class ScreenRegion(
    val id: Int,
    val source: String,
    val bounds: IntRect,
)

data class TranslatedRegion(
    val region: ScreenRegion,
    val translation: String,
)

sealed interface TranslationState {
    data object Idle : TranslationState
    data object Capturing : TranslationState
    data object Recognizing : TranslationState
    data class Translating(val completed: Int, val total: Int) : TranslationState
    data class Showing(val count: Int) : TranslationState
    data class Error(val message: String) : TranslationState
}
