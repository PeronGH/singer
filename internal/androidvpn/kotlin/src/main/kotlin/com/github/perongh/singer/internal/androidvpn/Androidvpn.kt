package com.github.perongh.singer.internal.androidvpn

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.ResultReceiver
import androidx.activity.ComponentActivity
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import dev.mygo.Native
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.Dispatchers
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

        suspendCancellableCoroutine { cont ->
            val receiver = object : ResultReceiver(null) {
                override fun onReceiveResult(resultCode: Int, resultData: Bundle?) {
                    if (!cont.isActive) return
                    if (resultCode == SingerVpnService.RESULT_OK) {
                        cont.resume(Unit)
                    } else {
                        val msg = resultData?.getString(SingerVpnService.EXTRA_ERROR) ?: "failed to start VPN service"
                        cont.resumeWithException(IllegalStateException(msg))
                    }
                }
            }

            cont.invokeOnCancellation { stop() }

            val intent = Intent(context, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_START
                putExtra(SingerVpnService.EXTRA_CONFIG, req.configJSON)
                putExtra(SingerVpnService.EXTRA_TUN_MODE, req.tunMode)
                putExtra(SingerVpnService.EXTRA_GROUP_NAME, req.groupName)
                putExtra(SingerVpnService.EXTRA_NODE_TAG, req.nodeTag)
                putExtra(SingerVpnService.EXTRA_NODE_INDEX, req.nodeIndex.toInt())
                putExtra(SingerVpnService.EXTRA_API_ADDR, req.apiAddr)
                putExtra(SingerVpnService.EXTRA_API_SECRET, req.apiSecret)
                putExtra(SingerVpnService.EXTRA_RECEIVER, receiver)
            }
            ContextCompat.startForegroundService(context, intent)
        }
    }

    override fun stop() {
        val context = Native.context
        val intent = Intent(context, SingerVpnService::class.java).apply {
            action = SingerVpnService.ACTION_STOP
        }
        runCatching { context.startService(intent) }
    }

    override fun status(): Status {
        val live = VpnStateStore.readLive(Native.context)
        return Status(
            running = live.running,
            tunMode = live.tunMode,
            groupName = live.groupName,
            nodeTag = live.nodeTag,
            nodeIndex = live.nodeIndex.toLong(),
            apiAddr = live.apiAddr,
            apiSecret = live.apiSecret,
        )
    }

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
    }
}
