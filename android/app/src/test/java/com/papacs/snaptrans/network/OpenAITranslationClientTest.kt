package com.papacs.snaptrans.network

import com.papacs.snaptrans.config.AppSettings
import com.papacs.snaptrans.core.IntRect
import com.papacs.snaptrans.core.ScreenRegion
import com.papacs.snaptrans.core.TranslationCoordinator
import com.papacs.snaptrans.core.TranslationItem
import java.io.IOException
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Response
import okhttp3.ResponseBody
import okio.Buffer
import okio.BufferedSource
import okio.Source
import okio.Timeout
import okio.buffer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class OpenAITranslationClientTest {
    private val settings = AppSettings(apiKey = "fixture", baseUrl = "https://fixture.invalid/v1", model = "fixture")
    private val items = listOf(TranslationItem(1, "Please confirm the updated schedule."))

    @Test fun `normal stop finishes without waiting for DONE or body EOF`() = runBlocking {
        val client = client { event("[1] 完整译文") + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" }
        val result = withTimeout(3_000) { client.translate(settings, items) }
        assertEquals(mapOf(1 to "完整译文"), result)
    }

    @Test fun `stream errors retain the provider message`() = runBlocking {
        val client = client { "data: {\"error\":{\"message\":\"fixture quota exhausted\"}}\n\n" }
        val error = runCatching { client.translate(settings, items) }.exceptionOrNull()
        assertTrue(error?.message.orEmpty().contains("fixture quota exhausted"))
    }

    @Test fun `clean EOF after partial text is not successful completion`() = runBlocking {
        val client = client(allowCleanEof = true) { event("[1] 半句") }
        val error = runCatching { withTimeout(3_000) { client.translate(settings, items) } }.exceptionOrNull()
        assertTrue(error is IOException)
        assertTrue(error?.message.orEmpty().contains("提前结束"))
    }

    @Test fun `non stop finish reasons do not turn partial text into success`() = runBlocking {
        for (reason in listOf("tool_calls", "function_call", "length", "content_filter")) {
            val client = client {
                event("[1] 半句") + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"$reason\"}]}\n\n"
            }
            val error = runCatching { withTimeout(3_000) { client.translate(settings, items) } }.exceptionOrNull()
            assertTrue(reason, error is IOException)
        }
    }

    @Test fun `terminal event at clean EOF does not require a blank line`() = runBlocking {
        val client = client(allowCleanEof = true) { event("[1] 完整译文") + "data: [DONE]" }
        assertEquals(mapOf(1 to "完整译文"), withTimeout(3_000) { client.translate(settings, items) })
    }

    @Test fun `failed HTTP error body does not leave the coroutine waiting`() = runBlocking {
        val client = client(503) { "temporary error" }
        val error = runCatching { withTimeout(3_000) { client.translate(settings, items) } }.exceptionOrNull()
        assertTrue(error is IOException)
    }

    @Test fun `missing translations after one repair are reported`() = runBlocking {
        val calls = AtomicInteger()
        val client = client { calls.incrementAndGet(); event("[1] 只有第一行") + "data: [DONE]\n\n" }
        val error = runCatching { client.translate(settings, items + TranslationItem(2, "Another complete sentence.")) }.exceptionOrNull()
        assertTrue(error is IOException)
        assertEquals(2, calls.get())
    }

    @Test fun `failed partial translation never becomes a cache hit`() = runBlocking {
        val calls = AtomicInteger()
        val client = client {
            if (calls.incrementAndGet() == 1) event("[1] 半句")
            else event("[1] 完整译文") + "data: [DONE]\n\n"
        }
        val coordinator = TranslationCoordinator(client)
        val regions = listOf(ScreenRegion(1, items[0].source, IntRect(0, 0, 300, 40)))
        assertTrue(runCatching { coordinator.translate(settings, regions) { _, _, _ -> } }.isFailure)
        val result = coordinator.translate(settings, regions) { _, _, _ -> }
        assertEquals(2, calls.get())
        assertEquals("完整译文", result.single().translation)
    }

    @Test fun `cancelled partial translation is retried on the next capture`() = runBlocking {
        val calls = AtomicInteger()
        val client = client { calls.incrementAndGet(); event("[1] 完整译文") + "data: [DONE]\n\n" }
        val coordinator = TranslationCoordinator(client)
        val regions = listOf(ScreenRegion(1, items[0].source, IntRect(0, 0, 300, 40)))
        assertTrue(runCatching {
            coordinator.translate(settings, regions) { partial, _, _ ->
                if (partial.isNotEmpty()) throw CancellationException("fixture cancellation")
            }
        }.isFailure)
        val result = coordinator.translate(settings, regions) { _, _, _ -> }
        assertEquals(2, calls.get())
        assertEquals("完整译文", result.single().translation)
    }

    @Test fun `changing model does not reuse translations from another model`() = runBlocking {
        val calls = AtomicInteger()
        val coordinator = TranslationCoordinator(client { event("[1] 译文${calls.incrementAndGet()}") + "data: [DONE]\n\n" })
        val regions = listOf(ScreenRegion(1, items[0].source, IntRect(0, 0, 300, 40)))
        val first = coordinator.translate(settings, regions) { _, _, _ -> }
        val second = coordinator.translate(settings.copy(model = "another-model"), regions) { _, _, _ -> }
        assertEquals("译文1", first.single().translation)
        assertEquals("译文2", second.single().translation)
        assertEquals(2, calls.get())
    }

    @Test fun `endpoint rejects lookalike emulator hosts and URL credentials`() {
        for (url in listOf("http://10.0.2.2.evil.invalid/v1", "http://10.0.2.20/v1", "https://user:secret@fixture.invalid/v1", "https://fixture.invalid/v1?route=other")) {
            assertTrue(url, runCatching { TranslationEndpoint.chatCompletions(url) }.isFailure)
        }
        assertEquals("https://fixture.invalid/v1/chat/completions", TranslationEndpoint.chatCompletions("https://fixture.invalid/v1/"))
        assertTrue(TranslationEndpoint.isAllowedBaseUrl("http://10.0.2.2:8000/v1", allowEmulatorHttp = true))
        assertTrue(!TranslationEndpoint.isAllowedBaseUrl("http://10.0.2.2:8000/v1", allowEmulatorHttp = false))
    }

    private fun event(text: String) = "data: {\"choices\":[{\"delta\":{\"content\":\"$text\"},\"finish_reason\":null}]}\n\n"

    private fun client(statusCode: Int = 200, allowCleanEof: Boolean = false, body: () -> String): OpenAITranslationClient {
        val http = OkHttpClient.Builder().addInterceptor { chain ->
            Response.Builder().request(chain.request()).protocol(Protocol.HTTP_1_1)
                .code(statusCode).message("fixture").body(object : ResponseBody() {
                    private val content = Buffer().writeUtf8(body())
                    private val stream = object : Source {
                        override fun read(sink: Buffer, byteCount: Long): Long {
                            if (content.size == 0L) {
                                if (allowCleanEof) return -1L
                                throw IOException("fixture read beyond terminal event")
                            }
                            return content.read(sink, byteCount)
                        }
                        override fun timeout() = Timeout.NONE
                        override fun close() = Unit
                    }.buffer()
                    override fun contentType() = "text/event-stream".toMediaType()
                    override fun contentLength() = -1L
                    override fun source(): BufferedSource = stream
                }).build()
        }.build()
        return OpenAITranslationClient(http)
    }
}
