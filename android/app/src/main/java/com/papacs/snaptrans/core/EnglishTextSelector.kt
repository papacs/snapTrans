package com.papacs.snaptrans.core

object EnglishTextSelector {
    private val whitespace = Regex("\\s+")
    private val urlLike = Regex("(?i)^(https?://|www\\.)|^[^ ]+@[^ ]+\\.[^ ]+$")
    private val latin = Regex("[A-Za-z]")

    fun select(regions: List<ScreenRegion>, maxRegions: Int = 80): List<ScreenRegion> =
        regions.asSequence()
            .map { it.copy(source = normalize(it.source)) }
            .filter { it.bounds.width >= 4 && it.bounds.height >= 4 }
            .filter { shouldTranslate(it.source) }
            .sortedWith(compareBy<ScreenRegion> { it.bounds.top }.thenBy { it.bounds.left })
            .take(maxRegions.coerceIn(1, 160))
            .mapIndexed { index, region -> region.copy(id = index + 1) }
            .toList()

    fun shouldTranslate(raw: String): Boolean {
        val text = normalize(raw)
        if (text.length !in 2..360 || urlLike.containsMatchIn(text)) return false

        val latinLetters = latin.findAll(text).count()
        if (latinLetters < 2) return false

        val allLetters = text.count(Char::isLetter)
        if (allLetters == 0 || latinLetters.toDouble() / allLetters < 0.65) return false

        // Avoid paying for identifiers, hashes, and number-heavy chart ticks.
        val meaningful = text.count { it.isLetter() || it.isWhitespace() }
        return meaningful.toDouble() / text.length >= 0.55
    }

    fun normalize(value: String): String = value.trim().replace(whitespace, " ")
}
