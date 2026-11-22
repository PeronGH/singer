package com.github.perongh.singer.internal.androidvpn

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.ResultReceiver
import androidx.activity.ComponentActivity
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import dev.mygo.Native
import java.util.concurrent.atomic.AtomicInteger
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext

fun makeService(): Service = AndroidVpnController()

private class AndroidVpnController : Service {
    private val reqSeq = AtomicInteger(0)

    override suspend fun start(configJSON: String, tunMode: Boolean) {
        val context = Native.context
        if (tunMode) {
            ensureVpnPermission()
        }

        suspendCancellableCoroutine { cont ->
            val receiver = object : ResultReceiver(Handler(Looper.getMainLooper())) {
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

            val intent = Intent(context, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_START
                putExtra(SingerVpnService.EXTRA_CONFIG, configJSON)
                putExtra(SingerVpnService.EXTRA_TUN_MODE, tunMode)
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
                cont.invokeOnCancellation { launcher.unregister() }
                launcher.launch(prepareIntent)
            }
        }
    }
}
