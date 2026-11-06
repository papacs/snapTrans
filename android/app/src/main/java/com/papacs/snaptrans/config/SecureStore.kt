package com.papacs.snaptrans.config

import android.content.SharedPreferences
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

internal class SecureStore(private val preferences: SharedPreferences) {
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    fun put(name: String, value: String) {
        if (value.isBlank()) {
            preferences.edit().remove(name).remove("${name}_iv").apply()
            return
        }
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, getOrCreateKey())
        val encrypted = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        preferences.edit()
            .putString(name, Base64.encodeToString(encrypted, Base64.NO_WRAP))
            .putString("${name}_iv", Base64.encodeToString(cipher.iv, Base64.NO_WRAP))
            .apply()
    }

    fun get(name: String): String = runCatching {
        val encrypted = preferences.getString(name, null) ?: return ""
        val iv = preferences.getString("${name}_iv", null) ?: return ""
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(
            Cipher.DECRYPT_MODE,
            keyStore.getKey(KEY_ALIAS, null) as SecretKey,
            GCMParameterSpec(128, Base64.decode(iv, Base64.NO_WRAP)),
        )
        String(cipher.doFinal(Base64.decode(encrypted, Base64.NO_WRAP)), Charsets.UTF_8)
    }.getOrElse {
        preferences.edit().remove(name).remove("${name}_iv").apply()
        ""
    }

    private fun getOrCreateKey(): SecretKey {
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(
                KeyGenParameterSpec.Builder(
                    KEY_ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                ).setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .build(),
            )
            generateKey()
        }
    }

    private companion object {
        const val KEY_ALIAS = "snaptrans_api_key_v1"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
    }
}
