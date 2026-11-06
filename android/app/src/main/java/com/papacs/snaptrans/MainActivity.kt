package com.papacs.snaptrans

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.media.projection.MediaProjectionManager
import android.media.projection.MediaProjectionConfig
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.papacs.snaptrans.capture.ScreenTranslationService
import com.papacs.snaptrans.config.AppSettings
import com.papacs.snaptrans.config.SettingsRepository
import com.papacs.snaptrans.ui.SnapTransApp
import com.papacs.snaptrans.network.TranslationEndpoint
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private lateinit var repository: SettingsRepository
    private lateinit var projectionManager: MediaProjectionManager
    private var settings by mutableStateOf(AppSettings())
    private var overlayGranted by mutableStateOf(false)
    private var serviceRunning by mutableStateOf(false)
    private var message by mutableStateOf<String?>(null)
    private var autoStartPending = false
    private var projectionRequestOpen = false
    private var notificationRequestOpen = false

    private val projectionLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        projectionRequestOpen = false
        val data = result.data
        if (result.resultCode != Activity.RESULT_OK || data == null) {
            message = "未授权屏幕捕获，翻译服务没有启动"
            return@registerForActivityResult
        }
        val service = Intent(this, ScreenTranslationService::class.java)
            .setAction(ScreenTranslationService.ACTION_START)
            .putExtra(ScreenTranslationService.EXTRA_RESULT_CODE, result.resultCode)
            .putExtra(ScreenTranslationService.EXTRA_RESULT_DATA, data)
        runCatching { ContextCompat.startForegroundService(this, service) }
            .onSuccess { message = "正在启动，返回其他应用后点击悬浮球" }
            .onFailure { message = "无法启动屏幕翻译：${it.message.orEmpty()}" }
    }

    private val notificationPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { notificationRequestOpen = false; requestProjection() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        repository = SettingsRepository(this)
        projectionManager = getSystemService(MediaProjectionManager::class.java)
        settings = repository.load()
        autoStartPending = savedInstanceState?.getBoolean("autoStartPending")
            ?: intent.getBooleanExtra(EXTRA_AUTO_START, false)
        projectionRequestOpen = savedInstanceState?.getBoolean("projectionRequestOpen") ?: false
        notificationRequestOpen = savedInstanceState?.getBoolean("notificationRequestOpen") ?: false
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                ScreenTranslationService.isRunning.collect { serviceRunning = it }
            }
        }

        setContent {
            SnapTransApp(
                settings = settings,
                overlayGranted = overlayGranted,
                serviceRunning = serviceRunning,
                message = message,
                onMessageShown = { message = null },
                onSettingsChanged = { settings = it },
                onGrantOverlay = ::requestOverlayPermission,
                onSave = ::saveSettings,
                onStart = {
                    if (saveSettings()) startFlow()
                },
                onStop = ::stopTranslation,
            )
        }
    }

    override fun onResume() {
        super.onResume()
        overlayGranted = android.provider.Settings.canDrawOverlays(this)
        serviceRunning = ScreenTranslationService.isRunning.value
        if (autoStartPending && overlayGranted && settings.isConfigured && !projectionRequestOpen && !notificationRequestOpen) {
            autoStartPending = false
            startFlow()
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        if (intent.getBooleanExtra(EXTRA_AUTO_START, false)) {
            autoStartPending = true
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putBoolean("autoStartPending", autoStartPending)
        outState.putBoolean("projectionRequestOpen", projectionRequestOpen)
        outState.putBoolean("notificationRequestOpen", notificationRequestOpen)
        super.onSaveInstanceState(outState)
    }

    private fun saveSettings(): Boolean {
        val validUrl = TranslationEndpoint.isAllowedBaseUrl(settings.baseUrl)
        if (settings.apiKey.isBlank()) {
            message = "请填写 API Key"
            return false
        }
        if (!validUrl || settings.model.isBlank()) {
            message = "请检查接口地址和模型名称"
            return false
        }
        try { repository.save(settings) }
        catch (error: Exception) {
            message = "无法保存设置：${error.message.orEmpty()}"
            return false
        }
        message = "设置已安全保存"
        return true
    }

    private fun startFlow() {
        if (projectionRequestOpen || notificationRequestOpen || serviceRunning) return
        if (!overlayGranted) {
            autoStartPending = true
            requestOverlayPermission()
            return
        }
        autoStartPending = false
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            notificationRequestOpen = true
            notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            return
        }
        requestProjection()
    }

    private fun requestProjection() {
        if (projectionRequestOpen || isFinishing) return
        projectionRequestOpen = true
        val request = if (Build.VERSION.SDK_INT >= 34) {
            // Full-display coordinates are required to paint translations in place.
            projectionManager.createScreenCaptureIntent(MediaProjectionConfig.createConfigForDefaultDisplay())
        } else projectionManager.createScreenCaptureIntent()
        projectionLauncher.launch(request)
    }

    private fun requestOverlayPermission() {
        startActivity(
            Intent(
                android.provider.Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                Uri.parse("package:$packageName"),
            ),
        )
    }

    private fun stopTranslation() {
        startService(
            Intent(this, ScreenTranslationService::class.java)
                .setAction(ScreenTranslationService.ACTION_STOP),
        )
        serviceRunning = false
        message = "屏幕翻译已停止"
    }

    companion object {
        const val EXTRA_AUTO_START = "auto_start_capture"
    }
}
