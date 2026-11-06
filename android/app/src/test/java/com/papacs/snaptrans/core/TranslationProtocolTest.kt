package com.papacs.snaptrans.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class TranslationProtocolTest {
    @Test fun `numbered protocol is compact and stable`() {
        val value = TranslationProtocol.numberedText(
            listOf(TranslationItem(2, "Save"), TranslationItem(7, "Open settings")),
        )
        assertEquals("[2] Save\n[7] Open settings", value)
        assertTrue(TranslationProtocol.SYSTEM_PROMPT.contains("Simplified Chinese"))
        assertFalse(TranslationProtocol.SYSTEM_PROMPT.contains("detect"))
    }

    @Test fun `parser accepts streamed completed lines`() {
        val parsed = TranslationProtocol.parseNumbered("[2] 保存\n[7] 打开设置\n")
        assertEquals("保存", parsed[2])
        assertEquals("打开设置", parsed[7])
    }

    @Test fun `empty numbered line cannot consume the following translation`() {
        val parsed = TranslationProtocol.parseNumbered("[1]\n[2] 完整第二行\n")
        assertNull(parsed[1])
        assertEquals("完整第二行", parsed[2])
    }

    @Test fun `batching observes item and character limits`() {
        val items = (1..5).map { TranslationItem(it, "abcdefghij") }
        val batches = TranslationProtocol.batches(items, maxCharacters = 30, maxItems = 2)
        assertEquals(listOf(1, 1, 1, 1, 1), batches.map { it.size })
        assertEquals((1..5).toList(), batches.flatten().map { it.id })
    }

    @Test fun `cache normalizes sources and evicts least recently used`() {
        val cache = TranslationCache(2)
        cache.put(" Save ", "保存")
        cache.put("Open", "打开")
        assertEquals("保存", cache.get("save"))
        cache.put("Close", "关闭")
        assertNull(cache.get("Open"))
        assertEquals("关闭", cache.get("close"))
    }

    @Test fun `common controls translate locally`() {
        assertEquals("设置", LocalTranslations.find(" SETTINGS "))
        assertNull(LocalTranslations.find("A sentence that needs context"))
    }
}
