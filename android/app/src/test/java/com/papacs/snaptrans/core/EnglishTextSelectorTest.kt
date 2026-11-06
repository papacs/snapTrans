package com.papacs.snaptrans.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class EnglishTextSelectorTest {
    @Test fun `selects English and ignores Chinese numeric and URLs`() {
        val input = listOf(
            region("Translate this page", 0),
            region("这是中文", 20),
            region("https://example.com/docs", 40),
            region("42.8%", 60),
            region("  Save   changes  ", 80),
        )

        val selected = EnglishTextSelector.select(input)

        assertEquals(listOf("Translate this page", "Save changes"), selected.map { it.source })
        assertEquals(listOf(1, 2), selected.map { it.id })
    }

    @Test fun `requires predominantly latin letters`() {
        assertTrue(EnglishTextSelector.shouldTranslate("Open settings"))
        assertTrue(EnglishTextSelector.shouldTranslate("Google Play 商店"))
        assertTrue(EnglishTextSelector.shouldTranslate("保存 Settings 设置"))
        assertFalse(EnglishTextSelector.shouldTranslate("A"))
    }

    @Test fun `caps regions to protect latency and token use`() {
        val input = (1..120).map { region("English item number $it", it * 10) }
        assertEquals(80, EnglishTextSelector.select(input).size)
    }

    private fun region(text: String, top: Int) = ScreenRegion(
        id = 0,
        source = text,
        bounds = IntRect(0, top, 300, top + 18),
    )
}
