package com.github.perongh.singer.internal.androidvpn

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import android.os.Message
import android.os.Messenger
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/**
 * Main-process Binder client for SingerVpnService (:vpn).
 *
 * Uses bindService(..., 0) so :vpn is never spawned while idle, connects
 * immediately when :vpn is already running, and receives kernel-delivered
 * onServiceDisconnected notifications if :vpn stops or dies.
 */
internal object VpnBinderClient {
    private val ipcThread = HandlerThread("singer-vpn-client").apply { start() }
    private val bound = AtomicBoolean(false)

    @Volatile
    private var appContext: Context? = null

    @Volatile
    private var serviceMessenger: Messenger? = null

    private val _snapshots = MutableStateFlow(VpnSnapshot())
    val snapshots: StateFlow<VpnSnapshot> = _snapshots.asStateFlow()

    private val _disconnectCount = MutableStateFlow(0L)
    val disconnectCount: StateFlow<Long> = _disconnectCount.asStateFlow()

    private val clientMessenger = Messenger(
        object : Handler(ipcThread.looper) {
            override fun handleMessage(msg: Message) {
                if (msg.what == VpnIpc.MSG_STATE) {
                    _snapshots.value = VpnSnapshot.fromBundle(msg.data)
                }
            }
        },
    )

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
            val remote = service?.let { Messenger(it) } ?: return
            serviceMessenger = remote
            val reg = Message.obtain(null, VpnIpc.MSG_REGISTER).apply {
                replyTo = clientMessenger
            }
            if (runCatching { remote.send(reg) }.isFailure) {
                onDisconnected()
            }
        }

        override fun onServiceDisconnected(name: ComponentName?) {
            onDisconnected()
        }

        override fun onBindingDied(name: ComponentName?) {
            onDisconnected()
            rebind()
        }
    }

    fun ensureBound(context: Context): VpnBinderClient {
        val app = context.applicationContext
        appContext = app
        if (bound.compareAndSet(false, true)) {
            bindInternal(app)
        }
        return this
    }

    /**
     * Holds a temporary BIND_AUTO_CREATE binding while a start() call is in flight so
     * Android guarantees onServiceConnected runs and :vpn is not destroyed before
     * delivering STATE_RUNNING or STATE_ERROR.
     */
    fun acquireStartBinding(context: Context): AutoCloseable {
        val app = context.applicationContext
        ensureBound(app)
        val startConn = object : ServiceConnection {
            override fun onServiceConnected(name: ComponentName?, service: IBinder?) = Unit
            override fun onServiceDisconnected(name: ComponentName?) {
                onDisconnected()
            }
            override fun onBindingDied(name: ComponentName?) {
                onDisconnected()
            }
        }
        val intent = Intent(app, SingerVpnService::class.java).apply {
            action = VpnIpc.ACTION_BIND
        }
        val ok = runCatching { app.bindService(intent, startConn, Context.BIND_AUTO_CREATE) }.getOrDefault(false)
        return AutoCloseable {
            if (ok) {
                runCatching { app.unbindService(startConn) }
            }
        }
    }

    fun statusFlow(): Flow<Status> = _snapshots
        .map { it.toStatus() }
        .distinctUntilChanged()

    fun sendStop(context: Context) {
        val remote = serviceMessenger
        if (remote != null) {
            val sent = runCatching {
                remote.send(Message.obtain(null, VpnIpc.MSG_STOP))
            }.isSuccess
            if (sent) return
        }
        val intent = Intent(context.applicationContext, SingerVpnService::class.java).apply {
            action = SingerVpnService.ACTION_STOP
        }
        runCatching { context.applicationContext.startService(intent) }
    }

    private fun bindInternal(app: Context) {
        val intent = Intent(app, SingerVpnService::class.java).apply {
            action = VpnIpc.ACTION_BIND
        }
        runCatching { app.bindService(intent, connection, 0) }
    }

    private fun rebind() {
        val app = appContext ?: return
        runCatching { app.unbindService(connection) }
        bindInternal(app)
    }

    private fun onDisconnected() {
        serviceMessenger = null
        _disconnectCount.value = _disconnectCount.value + 1
        _snapshots.value = VpnSnapshot(state = VpnIpc.STATE_STOPPED)
    }
}
