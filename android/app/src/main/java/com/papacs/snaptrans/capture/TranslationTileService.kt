package com.papacs.snaptrans.capture

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import com.papacs.snaptrans.MainActivity

class TranslationTileService : TileService() {
    override fun onStartListening() {
        super.onStartListening()
        qsTile?.apply {
            state = if (ScreenTranslationService.isRunning.value) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
            label = if (ScreenTranslationService.isRunning.value) "翻译屏幕" else "启动翻译"
            updateTile()
        }
    }

    @SuppressLint("StartActivityAndCollapseDeprecated")
    override fun onClick() {
        super.onClick()
        if (ScreenTranslationService.isRunning.value) {
            startService(
                Intent(this, ScreenTranslationService::class.java)
                    .setAction(ScreenTranslationService.ACTION_TOGGLE),
            )
            return
        }

        val intent = Intent(this, MainActivity::class.java)
            .putExtra(MainActivity.EXTRA_AUTO_START, true)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        if (Build.VERSION.SDK_INT >= 34) {
            val pending = PendingIntent.getActivity(
                this, 20, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            startActivityAndCollapse(pending)
        } else {
            @Suppress("DEPRECATION") startActivityAndCollapse(intent)
        }
    }
}
