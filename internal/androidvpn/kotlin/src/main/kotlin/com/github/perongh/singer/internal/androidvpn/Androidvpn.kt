package com.github.perongh.singer.internal.androidvpn

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import androidx.activity.ComponentActivity
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import dev.mygo.Native
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext

fun makeService(): Service = AndroidVpnController()

private class AndroidVpnController : Service {
    private val reqSeq = AtomicInteger(0)

    override suspend fun start(req: StartRequest) {
        val context = Native.context
        maybeRequestNotificationPermission()
        if (req.tunMode) {
            ensureVpnPermission()
        }

        val client = VpnBinderClient.ensureBound(context)
        val runId = nextRunId.incrementAndGet()
        val disconnectBaseline = client.disconnectCount.value

        val (terminal, _) = client.acquireStartBinding(context).use {
            val intent = Intent(context, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_START
                putExtra(SingerVpnService.EXTRA_RUN_ID, runId)
                putExtra(SingerVpnService.EXTRA_CONFIG, req.configJSON)
                putExtra(SingerVpnService.EXTRA_TUN_MODE, req.tunMode)
                putExtra(SingerVpnService.EXTRA_GROUP_NAME, req.groupName)
                putExtra(SingerVpnService.EXTRA_NODE_TAG, req.nodeTag)
                putExtra(SingerVpnService.EXTRA_NODE_INDEX, req.nodeIndex.toInt())
                putExtra(SingerVpnService.EXTRA_API_ADDR, req.apiAddr)
                putExtra(SingerVpnService.EXTRA_API_SECRET, req.apiSecret)
            }
            ContextCompat.startForegroundService(context, intent)

            try {
                combine(client.snapshots, client.disconnectCount) { snap, disconnects -> snap to disconnects }
                    .first { (snap, disconnects) ->
                        (snap.runId == runId && snap.state != VpnIpc.STATE_STARTING) ||
                            (snap.state == VpnIpc.STATE_STOPPED && disconnects > disconnectBaseline)
                    }
            } catch (e: CancellationException) {
                stop()
                throw e
            }
        }

        when (terminal.state) {
            VpnIpc.STATE_RUNNING -> Unit
            VpnIpc.STATE_ERROR -> throw IllegalStateException(terminal.error.ifEmpty { "failed to start VPN service" })
            else -> throw IllegalStateException("VPN service stopped")
        }
    }

    override fun stop() {
        VpnBinderClient.sendStop(Native.context)
    }

    override fun watch(): Flow<Status> = VpnBinderClient.ensureBound(Native.context).statusFlow()

    private suspend fun maybeRequestNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val context = Native.context
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
            return
        }
        if (!askedNotificationPermission.compareAndSet(false, true)) return

        withContext(Dispatchers.Main) {
            val activity = Native.activity as? ComponentActivity ?: return@withContext
            suspendCancellableCoroutine<Unit> { cont ->
                val key = "singer_notif_perm_${reqSeq.incrementAndGet()}"
                var launcher: ActivityResultLauncher<String>? = null
                launcher = activity.activityResultRegistry.register(
                    key,
                    ActivityResultContracts.RequestPermission(),
                ) {
                    launcher?.unregister()
                    if (cont.isActive) cont.resume(Unit)
                }
                cont.invokeOnCancellation {
                    Handler(Looper.getMainLooper()).post { launcher?.unregister() }
                }
                launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    private suspend fun ensureVpnPermission() {
        withContext(Dispatchers.Main) {
            val activity = Native.activity as? ComponentActivity
                ?: throw IllegalStateException("Android activity unavailable for VPN consent")
            val prepareIntent = VpnService.prepare(activity) ?: return@withContext

            suspendCancellableCoroutine { cont ->
                val key = "singer_vpn_prepare_${reqSeq.incrementAndGet()}"
                var launcher: ActivityResultLauncher<Intent>? = null
                launcher = activity.activityResultRegistry.register(
                    key,
                    ActivityResultContracts.StartActivityForResult(),
                ) { result ->
                    launcher?.unregister()
                    if (!cont.isActive) return@register
                    if (result.resultCode == Activity.RESULT_OK) {
                        cont.resume(Unit)
                    } else {
                        cont.resumeWithException(IllegalStateException("VPN permission denied"))
                    }
                }
                cont.invokeOnCancellation {
                    Handler(Looper.getMainLooper()).post { launcher?.unregister() }
                }
                launcher.launch(prepareIntent)
            }
        }
    }

    companion object {
        private val askedNotificationPermission = AtomicBoolean(false)
        private val nextRunId = AtomicLong(System.currentTimeMillis() * 1000L)
    }
}
