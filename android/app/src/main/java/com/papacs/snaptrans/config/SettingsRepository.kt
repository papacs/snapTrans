package com.papacs.snaptrans.config

import android.content.Context
import com.papacs.snaptrans.network.TranslationEndpoint

data class AppSettings(
    val apiKey: String = "",
    val baseUrl: String = "https://api.deepseek.com",
    val model: String = "deepseek-chat",
    val maxCharacters: Int = 2400,
) {
    val isConfigured: Boolean
        get() = apiKey.isNotBlank() && TranslationEndpoint.isAllowedBaseUrl(baseUrl) && model.isNotBlank()
}

class SettingsRepository(context: Context) {
    private val preferences = context.getSharedPreferences("snaptrans_settings", Context.MODE_PRIVATE)
    private val secureStore = SecureStore(preferences)

    fun load(): AppSettings = AppSettings(
        apiKey = secureStore.get(KEY_API),
        baseUrl = preferences.getString(KEY_BASE_URL, null)?.trim().orEmpty()
            .ifBlank { "https://api.deepseek.com" },
        model = preferences.getString(KEY_MODEL, null)?.trim().orEmpty()
            .ifBlank { "deepseek-chat" },
        maxCharacters = preferences.getInt(KEY_MAX_CHARACTERS, 2400).coerceIn(600, 6000),
    )

    fun save(settings: AppSettings) {
        secureStore.put(KEY_API, settings.apiKey.trim())
        preferences.edit()
            .putString(KEY_BASE_URL, settings.baseUrl.trim().trimEnd('/'))
            .putString(KEY_MODEL, settings.model.trim())
            .putInt(KEY_MAX_CHARACTERS, settings.maxCharacters.coerceIn(600, 6000))
            .apply()
    }

    private companion object {
        const val KEY_API = "api_key_encrypted"
        const val KEY_BASE_URL = "base_url"
        const val KEY_MODEL = "model"
        const val KEY_MAX_CHARACTERS = "max_characters"
    }
}
