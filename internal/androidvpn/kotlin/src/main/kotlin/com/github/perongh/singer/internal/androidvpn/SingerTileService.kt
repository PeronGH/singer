package com.github.perongh.singer.internal.androidvpn

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import androidx.core.content.ContextCompat

class SingerTileService : TileService() {
    override fun onStartListening() {
        super.onStartListening()
        refreshTile()
    }

    override fun onClick() {
        super.onClick()
        val live = VpnStateStore.readLive(this)
        if (live.running) {
            val stopIntent = Intent(this, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_STOP
            }
            runCatching { startService(stopIntent) }
            updateTileState(active = false, subtitle = live.nodeTag.ifEmpty { "Disconnected" })
            return
        }

        val canStartDirectly = live.configJSON.isNotEmpty() &&
            (!live.tunMode || VpnService.prepare(this) == null)
        if (canStartDirectly) {
            val startIntent = Intent(this, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_START
                putExtra(SingerVpnService.EXTRA_CONFIG, live.configJSON)
                putExtra(SingerVpnService.EXTRA_TUN_MODE, live.tunMode)
                putExtra(SingerVpnService.EXTRA_GROUP_NAME, live.groupName)
                putExtra(SingerVpnService.EXTRA_NODE_TAG, live.nodeTag)
                putExtra(SingerVpnService.EXTRA_NODE_INDEX, live.nodeIndex)
                putExtra(SingerVpnService.EXTRA_API_ADDR, live.apiAddr)
                putExtra(SingerVpnService.EXTRA_API_SECRET, live.apiSecret)
            }
            ContextCompat.startForegroundService(this, startIntent)
            updateTileState(active = true, subtitle = live.nodeTag.ifEmpty { "Starting…" })
            return
        }

        openApp()
    }

    private fun refreshTile() {
        val live = VpnStateStore.readLive(this)
        val subtitle = when {
            live.running -> live.nodeTag.ifEmpty { "Connected" }
            live.configJSON.isNotEmpty() -> live.nodeTag.ifEmpty { "Disconnected" }
            else -> "Open to select node"
        }
        updateTileState(active = live.running, subtitle = subtitle)
    }

    private fun updateTileState(active: Boolean, subtitle: String) {
        val tile = qsTile ?: return
        tile.label = "Singer"
        tile.state = if (active) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            tile.subtitle = subtitle
        }
        tile.updateTile()
    }

    private fun openApp() {
        val launch = packageManager.getLaunchIntentForPackage(packageName)?.apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        } ?: return

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            val pi = PendingIntent.getActivity(
                this,
                0,
                launch,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
            startActivityAndCollapse(pi)
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(launch)
        }
    }

    companion object {
        fun requestUpdate(context: Context) {
            runCatching {
                requestListeningState(
                    context.applicationContext,
                    ComponentName(context.applicationContext, SingerTileService::class.java),
                )
            }
        }
    }
}
