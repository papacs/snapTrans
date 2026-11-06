package com.papacs.snaptrans.capture

import android.app.Activity
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.content.res.Configuration
import android.graphics.Bitmap
import android.os.Build
import android.os.IBinder
import android.provider.Settings
import androidx.core.app.NotificationCompat
import androidx.core.content.getSystemService
import com.papacs.snaptrans.MainActivity
import com.papacs.snaptrans.R
import com.papacs.snaptrans.config.SettingsRepository
import com.papacs.snaptrans.core.EnglishTextSelector
import com.papacs.snaptrans.core.TranslationCoordinator
import com.papacs.snaptrans.core.TranslationState
import com.papacs.snaptrans.core.ResourceLease
import com.papacs.snaptrans.network.OpenAITranslationClient
import com.papacs.snaptrans.ocr.BitmapOptimizer
import com.papacs.snaptrans.ocr.MlKitOcrEngine
import com.papacs.snaptrans.ocr.OcrBitmap
import com.papacs.snaptrans.overlay.OverlayController
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.TimeoutCancellationException
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

class ScreenTranslationService : Service() {
    // State and overlay changes share the main queue; stale callbacks cannot
    // pass a generation check and then race a newer clear on another thread.
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var settingsRepository: SettingsRepository
    private lateinit var overlay: OverlayController
    private lateinit var ocr: MlKitOcrEngine
    private val coordinator = TranslationCoordinator(OpenAITranslationClient())
    private var captureSession: ScreenCaptureSession? = null
    private var task: Job? = null
    private val requestGeneration = AtomicLong(0)
    private val projectionGeneration = AtomicLong(0)
    @Volatile private var state: TranslationState = TranslationState.Idle

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        settingsRepository = SettingsRepository(this)
        ocr = MlKitOcrEngine()
        overlay = OverlayController(this, ::toggleTranslation)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> startProjection(intent)
            ACTION_TOGGLE -> if (captureSession == null) openSetup() else toggleTranslation()
            ACTION_STOP -> stopSelf()
        }
        return START_NOT_STICKY
    }

    private fun startProjection(intent: Intent) {
        if (!Settings.canDrawOverlays(this)) {
            openSetup()
            stopSelf()
            return
        }
        startAsForeground("悬浮按钮已就绪")
        val resultCode = intent.getIntExtra(EXTRA_RESULT_CODE, Activity.RESULT_CANCELED)
        val resultData = if (Build.VERSION.SDK_INT >= 33) {
            intent.getParcelableExtra(EXTRA_RESULT_DATA, Intent::class.java)
        } else {
            @Suppress("DEPRECATION") intent.getParcelableExtra(EXTRA_RESULT_DATA)
        }
        if (resultCode != Activity.RESULT_OK || resultData == null) {
            stopSelf()
            return
        }
        runCatching {
            cancelAndClear()
            val projectionId = projectionGeneration.incrementAndGet()
            captureSession?.close()
            captureSession = ScreenCaptureSession(this, resultCode, resultData) {
                scope.launch {
                    if (projectionGeneration.get() == projectionId) stopSelf()
                }
            }
            overlay.attach()
            running.value = true
            setState(TranslationState.Idle)
        }.onFailure {
            notifyStatus("无法启动屏幕捕获：${it.userMessage()}")
            stopSelf()
        }
    }

    private fun toggleTranslation() {
        when (state) {
            TranslationState.Idle, is TranslationState.Error -> translateCurrentScreen()
            else -> cancelAndClear()
        }
    }

    private fun translateCurrentScreen() {
        val session = captureSession ?: return
        val settings = settingsRepository.load()
        if (!settings.isConfigured) {
            setTemporaryError("请先配置翻译接口")
            openSetup()
            return
        }
        task?.cancel()
        val generation = requestGeneration.incrementAndGet()
        setState(TranslationState.Capturing)
        task = scope.launch {
            var sourceBitmap: ResourceLease<Bitmap>? = null
            var optimizedBitmap: OcrBitmap? = null
            try {
                overlay.clearTranslations()
                overlay.hideForCapture()
                delay(140)
                withTimeout(2_500) { sourceBitmap = session.capture() }
                if (requestGeneration.get() != generation) return@launch
                overlay.restoreBubble()
                setState(TranslationState.Recognizing)

                val source = requireNotNull(sourceBitmap)
                val optimized = withContext(Dispatchers.Default) {
                    BitmapOptimizer.forOcr(source).also { optimizedBitmap = it }
                }
                source.close()
                sourceBitmap = null
                val recognized = withTimeout(20_000) { ocr.recognize(optimized) }
                optimized.pixels.close()
                optimizedBitmap = null
                if (requestGeneration.get() != generation) return@launch
                val regions = EnglishTextSelector.select(recognized)
                if (regions.isEmpty()) {
                    setTemporaryError("当前屏幕未发现英文")
                    return@launch
                }

                setState(TranslationState.Translating(0, regions.size))
                val result = coordinator.translate(settings, regions) { partial, completed, total ->
                    scope.launch {
                        if (requestGeneration.get() == generation && state is TranslationState.Translating) {
                            overlay.showTranslations(partial)
                            setState(TranslationState.Translating(completed, total), notify = false)
                        }
                    }
                }
                if (requestGeneration.get() != generation) return@launch
                if (result.isEmpty()) {
                    setTemporaryError("没有可显示的译文")
                } else {
                    overlay.showTranslations(result)
                    setState(TranslationState.Showing(result.size))
                }
            } catch (_: TimeoutCancellationException) {
                if (requestGeneration.get() == generation) {
                    overlay.clearTranslations()
                    overlay.restoreBubble()
                    setTemporaryError("截图或文字识别超时，请重试")
                }
            } catch (_: CancellationException) {
                if (requestGeneration.get() == generation) {
                    overlay.clearTranslations()
                    overlay.restoreBubble()
                    setState(TranslationState.Idle)
                }
            } catch (error: Throwable) {
                if (requestGeneration.get() == generation) {
                    overlay.clearTranslations()
                    overlay.restoreBubble()
                    setTemporaryError(error.userMessage())
                }
            } finally {
                optimizedBitmap?.pixels?.close()
                sourceBitmap?.close()
            }
        }
    }

    private fun cancelAndClear() {
        requestGeneration.incrementAndGet()
        task?.cancel()
        task = null
        overlay.clearTranslations()
        overlay.restoreBubble()
        setState(TranslationState.Idle)
    }

    private fun setTemporaryError(message: String) {
        val generation = requestGeneration.get()
        val error = TranslationState.Error(message.take(80))
        setState(error)
        scope.launch {
            delay(1_600)
            if (requestGeneration.get() == generation && state === error) setState(TranslationState.Idle)
        }
    }

    private fun setState(value: TranslationState, notify: Boolean = true) {
        state = value
        overlay.setState(value)
        if (notify) notifyStatus(
            when (value) {
                TranslationState.Idle -> "悬浮按钮已就绪"
                TranslationState.Capturing -> "正在截取当前屏幕"
                TranslationState.Recognizing -> "正在本地识别英文"
                is TranslationState.Translating -> "正在翻译 ${value.completed}/${value.total}"
                is TranslationState.Showing -> "已显示 ${value.count} 条译文"
                is TranslationState.Error -> value.message
            },
        )
    }

    private fun startAsForeground(text: String) {
        val notification = notification(text)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun notifyStatus(text: String) {
        getSystemService<NotificationManager>()?.notify(NOTIFICATION_ID, notification(text))
    }

    private fun notification(text: String): Notification {
        val openIntent = PendingIntent.getActivity(
            this, 1, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val stopIntent = PendingIntent.getService(
            this, 2, Intent(this, ScreenTranslationService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_translate)
            .setContentTitle("snapTrans 屏幕翻译")
            .setContentText(text)
            .setContentIntent(openIntent)
            .setOngoing(true)
            .setSilent(true)
            .setOnlyAlertOnce(true)
            .addAction(0, "停止", stopIntent)
            .build()
    }

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            "屏幕翻译服务",
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = "屏幕捕获运行状态和停止入口" }
        getSystemService<NotificationManager>()?.createNotificationChannel(channel)
    }

    private fun openSetup() {
        val pending = PendingIntent.getActivity(
            this,
            3,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        runCatching { pending.send() }
    }

    override fun onDestroy() {
        running.value = false
        projectionGeneration.incrementAndGet()
        requestGeneration.incrementAndGet()
        task?.cancel()
        overlay.detach()
        captureSession?.close()
        captureSession = null
        ocr.close()
        scope.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onConfigurationChanged(newConfig: Configuration) {
        super.onConfigurationChanged(newConfig)
        cancelAndClear()
        captureSession?.onDisplayChanged()
        overlay.onDisplayChanged()
    }

    private fun Throwable.userMessage(): String = message?.takeIf { it.isNotBlank() } ?: "未知错误"

    companion object {
        const val ACTION_START = "com.papacs.snaptrans.action.START"
        const val ACTION_TOGGLE = "com.papacs.snaptrans.action.TOGGLE"
        const val ACTION_STOP = "com.papacs.snaptrans.action.STOP"
        const val EXTRA_RESULT_CODE = "projection_result_code"
        const val EXTRA_RESULT_DATA = "projection_result_data"
        private const val CHANNEL_ID = "screen_translation"
        private const val NOTIFICATION_ID = 4102
        private val running = MutableStateFlow(false)
        val isRunning = running.asStateFlow()
    }
}
