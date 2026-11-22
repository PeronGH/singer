package com.github.perongh.singer.internal.androidvpn

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class SingerTileService : TileService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var listenJob: Job? = null

    override fun onStartListening() {
        super.onStartListening()
        val client = VpnBinderClient.ensureBound(this)
        listenJob?.cancel()
        listenJob = scope.launch {
            client.snapshots.collect { snap ->
                renderTile(snap)
            }
        }
    }

    override fun onStopListening() {
        listenJob?.cancel()
        listenJob = null
        super.onStopListening()
    }

    override fun onClick() {
        super.onClick()
        val client = VpnBinderClient.ensureBound(this)
        val snap = client.snapshots.value
        if (snap.state == VpnIpc.STATE_RUNNING || snap.state == VpnIpc.STATE_STARTING) {
            client.sendStop(this)
            updateTileState(active = false, subtitle = "Disconnected")
            return
        }
        openApp()
    }

    private fun renderTile(snap: VpnSnapshot) {
        when (snap.state) {
            VpnIpc.STATE_RUNNING -> updateTileState(active = true, subtitle = snap.nodeTag.ifEmpty { "Connected" })
            VpnIpc.STATE_STARTING -> updateTileState(active = true, subtitle = snap.nodeTag.ifEmpty { "Starting…" })
            else -> updateTileState(active = false, subtitle = "Disconnected")
        }
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
