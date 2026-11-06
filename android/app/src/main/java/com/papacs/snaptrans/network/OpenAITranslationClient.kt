package com.papacs.snaptrans.network

import com.papacs.snaptrans.BuildConfig
import com.papacs.snaptrans.config.AppSettings
import com.papacs.snaptrans.core.TranslationItem
import com.papacs.snaptrans.core.TranslationProtocol
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okio.BufferedSource
import org.json.JSONArray
import org.json.JSONObject

class OpenAITranslationClient(
    private val httpClient: OkHttpClient = defaultClient(),
) {
    suspend fun translate(
        settings: AppSettings,
        items: List<TranslationItem>,
        onPartial: (Map<Int, String>) -> Unit = {},
    ): Map<Int, String> {
        if (items.isEmpty()) return emptyMap()
        val expectedIds = items.map { it.id }.toSet()
        val first = execute(settings, items) { onPartial(it.filterKeys(expectedIds::contains)) }
            .filterKeys(expectedIds::contains)
        val missing = items.filterNot { first.containsKey(it.id) }
        if (missing.isEmpty()) return first

        // Retry only omitted lines. This bounds extra token use while preventing blank overlays.
        val missingIds = missing.map { it.id }.toSet()
        val completed = first + execute(settings, missing) { onPartial(first + it.filterKeys(missingIds::contains)) }
            .filterKeys(missingIds::contains)
        if (items.any { !completed.containsKey(it.id) }) throw IOException("部分文字未翻译完整，请重试")
        return completed
    }

    private suspend fun execute(
        settings: AppSettings,
        items: List<TranslationItem>,
        onPartial: (Map<Int, String>) -> Unit,
    ): Map<Int, String> = suspendCancellableCoroutine { continuation ->
        val request = Request.Builder()
            .url(TranslationEndpoint.chatCompletions(settings.baseUrl))
            .header("Authorization", "Bearer ${settings.apiKey}")
            .header("Accept", "text/event-stream")
            .post(requestJson(settings, items).toRequestBody(JSON_MEDIA_TYPE))
            .build()
        val call = httpClient.newCall(request)
        continuation.invokeOnCancellation { call.cancel() }
        call.enqueue(object : Callback {
            override fun onFailure(call: Call, error: IOException) {
                if (continuation.isActive) continuation.resumeWithException(error)
            }

            override fun onResponse(call: Call, response: Response) {
                response.use {
                    try {
                        if (!it.isSuccessful) {
                            val detail = it.peekBody(512).string().take(240)
                            throw IOException("翻译接口 ${it.code}: $detail")
                        }
                        val output = StringBuilder()
                        val event = StringBuilder()
                        var completed = false
                        var lastPartial: Map<Int, String> = emptyMap()
                        val source = it.body?.source() ?: error("翻译接口返回空内容")
                        fun consumeEvent(): Boolean {
                            if (event.isEmpty()) return false
                            val payload = event.toString().trim()
                            event.setLength(0)
                            if (payload == "[DONE]") return true
                            val json = JSONObject(payload)
                            json.optJSONObject("error")?.let { error ->
                                throw IOException(error.optString("message", "翻译接口返回错误").take(240))
                            }
                            val choice = json.optJSONArray("choices")?.optJSONObject(0) ?: return false
                            val token = choice.optJSONObject("delta")?.opt("content") as? String ?: ""
                            if (output.length + token.length > MAX_RESPONSE_CHARACTERS) throw IOException("翻译接口返回内容过长")
                            output.append(token)
                            if (token.isNotEmpty()) {
                                val partial = TranslationProtocol.parseNumbered(output.toString())
                                if (partial != lastPartial) {
                                    lastPartial = partial
                                    onPartial(partial)
                                }
                            }
                            val reason = choice.opt("finish_reason") as? String
                            return when {
                                reason.isNullOrBlank() -> false
                                reason == "stop" -> true
                                reason == "length" -> throw IOException("模型输出被截断，请减少每批字符数")
                                reason == "content_filter" -> throw IOException("翻译被接口的内容过滤终止")
                                else -> throw IOException("翻译接口未正常结束：${reason.take(40)}")
                            }
                        }
                        while (continuation.isActive) {
                            val line = readEventLine(source)
                            if (line == null) { completed = consumeEvent(); break }
                            if (line.isEmpty()) {
                                if (consumeEvent()) { completed = true; break }
                            } else if (line.startsWith("data:")) {
                                if (event.isNotEmpty()) event.append('\n')
                                event.append(line.removePrefix("data:").removePrefix(" "))
                                if (event.length > MAX_RESPONSE_CHARACTERS) throw IOException("翻译接口事件过长")
                            }
                        }
                        if (!continuation.isActive) return
                        if (!completed) throw IOException("翻译连接提前结束，请重试")
                        val parsed = TranslationProtocol.parseNumbered(output.toString())
                        if (parsed.isEmpty()) error("翻译返回格式无效，请检查模型配置")
                        if (continuation.isActive) continuation.resume(parsed)
                    } catch (error: Throwable) {
                        if (continuation.isActive) continuation.resumeWithException(error)
                    }
                }
            }
        })
    }

    private fun readEventLine(source: BufferedSource): String? {
        val newline = source.indexOf('\n'.code.toByte(), 0L, MAX_RESPONSE_CHARACTERS.toLong() + 1)
        if (newline < 0 && source.buffer.size > MAX_RESPONSE_CHARACTERS) throw IOException("翻译接口事件过长")
        return source.readUtf8Line()
    }

    private fun requestJson(settings: AppSettings, items: List<TranslationItem>): String {
        val messages = JSONArray()
            .put(JSONObject().put("role", "system").put("content", TranslationProtocol.SYSTEM_PROMPT))
            .put(
                JSONObject().put("role", "user").put(
                    "content",
                    "Translate now:\n${TranslationProtocol.numberedText(items)}",
                ),
            )
        return JSONObject()
            .put("model", settings.model)
            .put("messages", messages)
            .put("temperature", 0.1)
            .put("stream", true)
            .put("max_tokens", (items.sumOf { it.source.length } * 2).coerceIn(128, 2048))
            .toString()
    }

    private companion object {
        const val MAX_RESPONSE_CHARACTERS = 1 shl 20
        val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()

        fun defaultClient() = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(60, TimeUnit.SECONDS)
            .callTimeout(75, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build()
    }
}

object TranslationEndpoint {
    fun isAllowedBaseUrl(baseUrl: String, allowEmulatorHttp: Boolean = BuildConfig.DEBUG): Boolean {
        val url = baseUrl.trim().toHttpUrlOrNull() ?: return false
        return (url.scheme == "https" || (allowEmulatorHttp && url.scheme == "http" && url.host == "10.0.2.2")) &&
            url.username.isEmpty() && url.password.isEmpty() && url.query == null && url.fragment == null
    }

    fun chatCompletions(baseUrl: String): String {
        val base = baseUrl.trim().trimEnd('/')
        require(isAllowedBaseUrl(base)) {
            "接口地址必须使用 HTTPS"
        }
        return if (base.endsWith("/chat/completions")) base else "$base/chat/completions"
    }
}
