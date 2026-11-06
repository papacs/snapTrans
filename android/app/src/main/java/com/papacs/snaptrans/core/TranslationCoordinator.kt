package com.papacs.snaptrans.core

import com.papacs.snaptrans.config.AppSettings
import com.papacs.snaptrans.network.OpenAITranslationClient
import java.security.MessageDigest
import java.util.Base64
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

class TranslationCoordinator(
    private val client: OpenAITranslationClient,
    private val cache: TranslationCache = TranslationCache(),
) {
    suspend fun translate(
        settings: AppSettings,
        regions: List<ScreenRegion>,
        onUpdate: (List<TranslatedRegion>, completed: Int, total: Int) -> Unit,
    ): List<TranslatedRegion> {
        val scope = Base64.getEncoder().encodeToString(MessageDigest.getInstance("SHA-256")
            .digest("${settings.baseUrl.trim().trimEnd('/')}\u0000${settings.model.trim()}\u0000${settings.apiKey}".toByteArray()))
        val partialBySource = mutableMapOf<String, String>()
        fun translatedRegions(): List<TranslatedRegion> = regions.mapNotNull { region ->
            val text = partialBySource[EnglishTextSelector.normalize(region.source).lowercase()]
                ?: cache.get(region.source, scope)
            text?.let { TranslatedRegion(region, it) }
        }
        fun emit(completed: Int) = onUpdate(translatedRegions(), completed, regions.size)
        regions.forEach { region ->
            LocalTranslations.find(region.source)?.let { cache.put(region.source, it, scope) }
        }
        emit(regions.count { cache.get(it.source, scope) != null })

        val uniqueMissing = regions
            .distinctBy { EnglishTextSelector.normalize(it.source).lowercase() }
            .filter { cache.get(it.source, scope) == null }
            .map { TranslationItem(it.id, it.source) }
        val batches = TranslationProtocol.batches(
            uniqueMissing,
            maxCharacters = settings.maxCharacters,
        )
        var completed = regions.count { cache.get(it.source, scope) != null }

        for (batch in batches) {
            val sourceById = batch.associate { it.id to it.source }
            val translated = client.translate(settings, batch) { partial ->
                partial.forEach { (id, value) ->
                    sourceById[id]?.let { partialBySource[EnglishTextSelector.normalize(it).lowercase()] = value }
                }
                emit(completed)
            }
            currentCoroutineContext().ensureActive()
            translated.forEach { (id, value) ->
                sourceById[id]?.let {
                    cache.put(it, value, scope)
                    partialBySource.remove(EnglishTextSelector.normalize(it).lowercase())
                }
            }
            completed = regions.count { cache.get(it.source, scope) != null }
            emit(completed)
        }
        return translatedRegions()
    }
}
