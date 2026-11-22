package com.github.perongh.singer.internal.androidvpn

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.DnsResolver
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.CancellationSignal
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import android.os.Message
import android.os.Messenger
import android.os.ParcelFileDescriptor
import android.os.Process
import android.system.ErrnoException
import android.system.OsConstants
import android.util.Log
import androidx.core.app.NotificationCompat
import io.nekohasekai.libbox.BridgeOptions
import io.nekohasekai.libbox.BridgeSession
import io.nekohasekai.libbox.CommandServer
import io.nekohasekai.libbox.CommandServerHandler
import io.nekohasekai.libbox.ConnectionOwner
import io.nekohasekai.libbox.ExchangeContext
import io.nekohasekai.libbox.InterfaceUpdateListener
import io.nekohasekai.libbox.Libbox
import io.nekohasekai.libbox.LocalDNSTransport
import io.nekohasekai.libbox.NeighborUpdateListener
import io.nekohasekai.libbox.NetworkInterfaceIterator
import io.nekohasekai.libbox.Notification
import io.nekohasekai.libbox.OverrideOptions
import io.nekohasekai.libbox.PlatformInterface
import io.nekohasekai.libbox.PlatformUser
import io.nekohasekai.libbox.ShellSession
import io.nekohasekai.libbox.SetupOptions
import io.nekohasekai.libbox.StringIterator
import io.nekohasekai.libbox.SystemProxyStatus
import io.nekohasekai.libbox.TunOptions
import io.nekohasekai.libbox.WIFIState
import io.nekohasekai.libbox.NetworkInterface as LibboxNetworkInterface
import java.io.File
import java.net.Inet6Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.InterfaceAddress
import java.net.NetworkInterface
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

class SingerVpnService : VpnService(), CommandServerHandler, PlatformInterface {
    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }
    private val worker = Executors.newSingleThreadExecutor()
    private val dnsExecutor = Executors.newCachedThreadPool()
    private val ipcThread = HandlerThread("singer-vpn-ipc").apply { start() }
    private val clients = LinkedHashSet<Messenger>()
    private val libboxLock = Any()
    private val generation = AtomicLong(0L)

    @Volatile
    private var currentSnapshot = VpnSnapshot()

    private val ipcHandler = object : Handler(ipcThread.looper) {
        override fun handleMessage(msg: Message) {
            when (msg.what) {
                VpnIpc.MSG_REGISTER -> {
                    val client = msg.replyTo ?: return
                    synchronized(clients) { clients.add(client) }
                    sendSnapshot(client, currentSnapshot)
                }
                VpnIpc.MSG_UNREGISTER -> {
                    val client = msg.replyTo ?: return
                    synchronized(clients) { clients.remove(client) }
                }
                VpnIpc.MSG_STOP -> requestStop()
            }
        }
    }

    private val serviceMessenger = Messenger(ipcHandler)

    @Volatile
    private var commandServer: CommandServer? = null

    @Volatile
    private var tunPfd: ParcelFileDescriptor? = null

    @Volatile
    private var defaultNetwork: Network? = null

    @Volatile
    private var interfaceListener: InterfaceUpdateListener? = null

    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    override fun onBind(intent: Intent?): IBinder? {
        if (intent?.action == VpnIpc.ACTION_BIND) {
            return serviceMessenger.binder
        }
        return super.onBind(intent)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> {
                val runId = intent.getLongExtra(EXTRA_RUN_ID, 0L)
                val config = intent.getStringExtra(EXTRA_CONFIG) ?: ""
                val tunMode = intent.getBooleanExtra(EXTRA_TUN_MODE, false)
                val groupName = intent.getStringExtra(EXTRA_GROUP_NAME) ?: ""
                val nodeTag = intent.getStringExtra(EXTRA_NODE_TAG) ?: ""
                val nodeIndex = intent.getIntExtra(EXTRA_NODE_INDEX, -1)
                val apiAddr = intent.getStringExtra(EXTRA_API_ADDR) ?: ""
                val apiSecret = intent.getStringExtra(EXTRA_API_SECRET) ?: ""
                val gen = generation.incrementAndGet()
                promoteForeground(nodeTag, tunMode)
                val baseSnapshot = VpnSnapshot(
                    state = VpnIpc.STATE_STARTING,
                    runId = runId,
                    tunMode = tunMode,
                    groupName = groupName,
                    nodeTag = nodeTag,
                    nodeIndex = nodeIndex,
                    apiAddr = apiAddr,
                    apiSecret = apiSecret,
                )
                publishState(baseSnapshot)
                submitWorker {
                    if (generation.get() != gen) return@submitWorker
                    try {
                        startLibbox(config)
                        if (generation.get() != gen) {
                            stopLibbox(removeForeground = false, emitStopped = false)
                            return@submitWorker
                        }
                        publishState(baseSnapshot.copy(state = VpnIpc.STATE_RUNNING))
                    } catch (e: Throwable) {
                        Log.e(TAG, "start sing-box failed", e)
                        if (generation.get() != gen) return@submitWorker
                        val msg = e.message ?: e.toString()
                        stopLibbox(removeForeground = true, emitStopped = false)
                        publishState(baseSnapshot.copy(state = VpnIpc.STATE_ERROR, error = msg))
                        stopSelf()
                    }
                }
            }
            ACTION_STOP -> requestStop()
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        requestStop()
        super.onRevoke()
    }

    override fun onDestroy() {
        generation.incrementAndGet()
        worker.shutdownNow()
        dnsExecutor.shutdownNow()
        stopLibbox(removeForeground = true, emitStopped = currentSnapshot.state != VpnIpc.STATE_ERROR)
        ipcThread.quitSafely()
        super.onDestroy()
    }

    private fun requestStop() {
        val gen = generation.incrementAndGet()
        submitWorker {
            if (generation.get() != gen) return@submitWorker
            stopLibbox(removeForeground = true, emitStopped = true)
            stopSelf()
        }
    }

    private fun submitWorker(task: () -> Unit) {
        if (worker.isShutdown) return
        runCatching { worker.execute(task) }
    }

    private fun publishState(snapshot: VpnSnapshot) {
        currentSnapshot = snapshot
        val targets = synchronized(clients) { clients.toList() }
        for (client in targets) {
            sendSnapshot(client, snapshot)
        }
        SingerTileService.requestUpdate(this)
    }

    private fun sendSnapshot(client: Messenger, snapshot: VpnSnapshot) {
        val msg = Message.obtain(null, VpnIpc.MSG_STATE).apply {
            data = snapshot.toBundle()
        }
        if (runCatching { client.send(msg) }.isFailure) {
            synchronized(clients) { clients.remove(client) }
        }
    }

    private fun promoteForeground(nodeTag: String, tunMode: Boolean) {
        val manager = getSystemService(NotificationManager::class.java)
        if (manager.getNotificationChannel(CHANNEL_ID) == null) {
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "Singer VPN", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val modeLabel = if (tunMode) "VPN" else "Local proxy"
        val text = if (nodeTag.isNotEmpty()) "$nodeTag · $modeLabel" else "$modeLabel active"

        val stopIntent = Intent(this, SingerVpnService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this,
            1,
            stopIntent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

        val builder = NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Singer")
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_stat_singer)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .addAction(0, "Stop", stopPendingIntent)

        packageManager.getLaunchIntentForPackage(packageName)?.let { launch ->
            launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            val openPendingIntent = PendingIntent.getActivity(
                this,
                0,
                launch,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
            builder.setContentIntent(openPendingIntent)
        }

        val notification = builder.build()

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun ensureLibboxSetup() {
        if (!libboxInitialized.compareAndSet(false, true)) return
        val baseDir = File(filesDir, "sing-box").apply { mkdirs() }
        val workDir = File(baseDir, "work").apply { mkdirs() }
        val tmpDir = File(cacheDir, "sing-box").apply { mkdirs() }
        val opts = SetupOptions().apply {
            basePath = baseDir.absolutePath
            workingPath = workDir.absolutePath
            tempPath = tmpDir.absolutePath
            fixAndroidStack = true
        }
        Libbox.setup(opts)
    }

    private fun startLibbox(configJSON: String) = synchronized(libboxLock) {
        ensureLibboxSetup()
        stopLibbox(removeForeground = false, emitStopped = false)
        val server = Libbox.newCommandServer(this, this)
        commandServer = server
        server.start()
        server.startOrReloadService(configJSON, OverrideOptions())
    }

    private fun stopLibbox(removeForeground: Boolean = true, emitStopped: Boolean = true) = synchronized(libboxLock) {
        unregisterNetworkMonitor()
        val server = commandServer
        commandServer = null
        if (server != null) {
            runCatching { server.closeService() }
            runCatching { server.close() }
        }
        val pfd = tunPfd
        tunPfd = null
        if (pfd != null) {
            runCatching { pfd.close() }
        }
        if (removeForeground) {
            stopForeground(STOP_FOREGROUND_REMOVE)
        }
        if (emitStopped) {
            publishState(VpnSnapshot(state = VpnIpc.STATE_STOPPED))
        }
    }

    // CommandServerHandler

    override fun serviceStop() {
        requestStop()
    }

    override fun serviceReload() {}

    override fun getSystemProxyStatus(): SystemProxyStatus = SystemProxyStatus()

    override fun setSystemProxyEnabled(enabled: Boolean) {}

    override fun triggerNativeCrash() {}

    override fun writeDebugMessage(message: String?) {
        if (!message.isNullOrEmpty()) {
            Log.d(TAG, message)
        }
    }

    override fun connectSSHAgent(): Int = throw UnsupportedOperationException("not supported")

    // PlatformInterface

    override fun localDNSTransport(): LocalDNSTransport = AndroidLocalResolver()

    override fun usePlatformAutoDetectInterfaceControl(): Boolean = true

    override fun autoDetectInterfaceControl(fd: Int) {
        if (!protect(fd)) {
            throw IllegalStateException("VpnService.protect($fd) failed")
        }
    }

    override fun openTun(options: TunOptions): Int {
        if (prepare(this) != null) {
            throw IllegalStateException("Android VPN permission not granted")
        }

        val builder = Builder()
            .setSession("Singer")
            .setMtu(options.mtu)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            builder.setMetered(false)
        }

        var hasInet4 = false
        val inet4 = options.inet4Address
        while (inet4.hasNext()) {
            val prefix = inet4.next()
            builder.addAddress(prefix.address(), prefix.prefix())
            hasInet4 = true
        }
        var hasInet6 = false
        val inet6 = options.inet6Address
        while (inet6.hasNext()) {
            val prefix = inet6.next()
            builder.addAddress(prefix.address(), prefix.prefix())
            hasInet6 = true
        }

        if (options.autoRoute) {
            val dnsServers = options.dnsServerAddress
            while (dnsServers.hasNext()) {
                builder.addDnsServer(dnsServers.next())
            }

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                val route4 = options.inet4RouteAddress
                var hasRoute4 = false
                while (route4.hasNext()) {
                    val p = route4.next()
                    builder.addRoute(p.address(), p.prefix())
                    hasRoute4 = true
                }
                if (!hasRoute4 && hasInet4) {
                    builder.addRoute("0.0.0.0", 0)
                }
                val ex4 = options.inet4RouteExcludeAddress
                while (ex4.hasNext()) {
                    val p = ex4.next()
                    builder.excludeRoute(android.net.IpPrefix(InetAddress.getByName(p.address()), p.prefix()))
                }
                val route6 = options.inet6RouteAddress
                var hasRoute6 = false
                while (route6.hasNext()) {
                    val p = route6.next()
                    builder.addRoute(p.address(), p.prefix())
                    hasRoute6 = true
                }
                if (!hasRoute6 && hasInet6) {
                    builder.addRoute("::", 0)
                }
                val ex6 = options.inet6RouteExcludeAddress
                while (ex6.hasNext()) {
                    val p = ex6.next()
                    builder.excludeRoute(android.net.IpPrefix(InetAddress.getByName(p.address()), p.prefix()))
                }
            } else {
                val r4 = options.inet4RouteRange
                while (r4.hasNext()) {
                    val p = r4.next()
                    builder.addRoute(p.address(), p.prefix())
                }
                val r6 = options.inet6RouteRange
                while (r6.hasNext()) {
                    val p = r6.next()
                    builder.addRoute(p.address(), p.prefix())
                }
            }
        }

        runCatching { builder.addDisallowedApplication(packageName) }

        val pfd = builder.establish() ?: throw IllegalStateException("VpnService.Builder.establish() returned null")
        tunPfd = pfd
        return pfd.fd
    }

    override fun findConnectionOwner(
        ipProtocol: Int,
        sourceAddress: String,
        sourcePort: Int,
        destinationAddress: String,
        destinationPort: Int,
    ): ConnectionOwner {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            val uid = connectivity.getConnectionOwnerUid(
                ipProtocol,
                InetSocketAddress(sourceAddress, sourcePort),
                InetSocketAddress(destinationAddress, destinationPort),
            )
            if (uid == Process.INVALID_UID) {
                throw IllegalStateException("connection owner not found")
            }
            val pkgs = packageManager.getPackagesForUid(uid)?.toList().orEmpty()
            return ConnectionOwner().apply {
                userId = uid
                userName = pkgs.firstOrNull() ?: ""
                setAndroidPackageNames(ListStringIterator(pkgs))
            }
        }
        throw UnsupportedOperationException("findConnectionOwner requires Android 10+")
    }

    override fun startDefaultInterfaceMonitor(listener: InterfaceUpdateListener) {
        interfaceListener = listener
        registerNetworkMonitor()
        notifyCurrentDefaultNetwork()
    }

    override fun closeDefaultInterfaceMonitor(listener: InterfaceUpdateListener) {
        interfaceListener = null
        unregisterNetworkMonitor()
    }

    @Suppress("DEPRECATION")
    override fun getInterfaces(): NetworkInterfaceIterator {
        val sysIfaces = runCatching { NetworkInterface.getNetworkInterfaces()?.toList().orEmpty() }
            .getOrDefault(emptyList())
        val out = mutableListOf<LibboxNetworkInterface>()
        for (network in connectivity.allNetworks) {
            val lp = connectivity.getLinkProperties(network) ?: continue
            val caps = connectivity.getNetworkCapabilities(network) ?: continue
            val ifaceName = lp.interfaceName ?: continue
            val sysIface = sysIfaces.find { it.name == ifaceName } ?: continue

            val boxIface = LibboxNetworkInterface().apply {
                name = ifaceName
                index = sysIface.index
                runCatching { mtu = sysIface.mtu }
                dnsServer = ListStringIterator(lp.dnsServers.mapNotNull { it.hostAddress })
                gateway = ListStringIterator(
                    lp.routes
                        .filter { it.destination.prefixLength == 0 }
                        .mapNotNull { it.gateway }
                        .filterNot { it.isAnyLocalAddress }
                        .mapNotNull { it.hostAddress },
                )
                addresses = ListStringIterator(sysIface.interfaceAddresses.map { it.toPrefix() })
                type = when {
                    caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> Libbox.InterfaceTypeWIFI
                    caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> Libbox.InterfaceTypeCellular
                    caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> Libbox.InterfaceTypeEthernet
                    else -> Libbox.InterfaceTypeOther
                }
                var flags = 0
                if (caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) {
                    flags = OsConstants.IFF_UP or OsConstants.IFF_RUNNING
                }
                if (sysIface.isLoopback) flags = flags or OsConstants.IFF_LOOPBACK
                if (sysIface.isPointToPoint) flags = flags or OsConstants.IFF_POINTOPOINT
                if (sysIface.supportsMulticast()) flags = flags or OsConstants.IFF_MULTICAST
                this.flags = flags
                metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
            }
            out.add(boxIface)
        }
        return ListNetworkInterfaceIterator(out)
    }

    override fun underNetworkExtension(): Boolean = false

    override fun includeAllNetworks(): Boolean = false

    override fun readWIFIState(): WIFIState? = null

    override fun clearDNSCache() {}

    override fun sendNotification(notification: Notification) {}

    override fun cancelNotification(identifier: String, typeID: Int) {}

    override fun startNeighborMonitor(listener: NeighborUpdateListener?) {}

    override fun closeNeighborMonitor(listener: NeighborUpdateListener?) {}

    override fun registerMyInterface(name: String?) {}

    override fun usePlatformShell(): Boolean = false

    override fun checkPlatformShell() {
        throw UnsupportedOperationException("not supported")
    }

    override fun openShellSession(
        user: PlatformUser?,
        command: String?,
        environ: StringIterator?,
        term: String?,
        rows: Int,
        cols: Int,
    ): ShellSession = throw UnsupportedOperationException("not supported")

    override fun lookupUser(username: String?): PlatformUser = throw UnsupportedOperationException("not supported")

    override fun lookupSFTPServer(): String = throw UnsupportedOperationException("not supported")

    override fun readSystemSSHHostKey(): String = throw UnsupportedOperationException("not supported")

    override fun tailscaleHostname(): String = ""

    override fun usePlatformBridge(): Boolean = false

    override fun createBridge(options: BridgeOptions?): BridgeSession =
        throw UnsupportedOperationException("not supported")

    private fun registerNetworkMonitor() {
        if (networkCallback != null) return
        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                defaultNetwork = network
                notifyDefaultNetwork(network)
            }

            override fun onLinkPropertiesChanged(network: Network, linkProperties: android.net.LinkProperties) {
                defaultNetwork = network
                notifyDefaultNetwork(network)
            }

            override fun onLost(network: Network) {
                if (defaultNetwork == network) {
                    defaultNetwork = null
                    interfaceListener?.updateDefaultInterface("", -1, false, false)
                }
            }
        }
        networkCallback = cb
        runCatching { connectivity.registerNetworkCallback(request, cb) }
    }

    private fun unregisterNetworkMonitor() {
        val cb = networkCallback ?: return
        networkCallback = null
        runCatching { connectivity.unregisterNetworkCallback(cb) }
    }

    private fun notifyCurrentDefaultNetwork() {
        val active = connectivity.activeNetwork ?: return
        defaultNetwork = active
        notifyDefaultNetwork(active)
    }

    private fun notifyDefaultNetwork(network: Network) {
        val listener = interfaceListener ?: return
        val lp = connectivity.getLinkProperties(network) ?: return
        val caps = connectivity.getNetworkCapabilities(network) ?: return
        val name = lp.interfaceName ?: return
        val iface = runCatching { NetworkInterface.getByName(name) }.getOrNull() ?: return
        val expensive = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
        listener.updateDefaultInterface(name, iface.index, expensive, false)
    }

    private inner class AndroidLocalResolver : LocalDNSTransport {
        override fun raw(): Boolean = Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q

        override fun lookup(ctx: ExchangeContext, network: String, domain: String) {
            val active = defaultNetwork ?: connectivity.activeNetwork
            val addresses = if (active != null) {
                active.getAllByName(domain)
            } else {
                InetAddress.getAllByName(domain)
            }
            val result = addresses
                .filter {
                    when (network) {
                        "ip4" -> it !is Inet6Address
                        "ip6" -> it is Inet6Address
                        else -> true
                    }
                }
                .mapNotNull { it.hostAddress }
                .joinToString("\n")
            ctx.success(result)
        }

        override fun exchange(ctx: ExchangeContext, message: ByteArray) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
                throw UnsupportedOperationException("raw DNS requires Android 10+")
            }
            val active = defaultNetwork ?: connectivity.activeNetwork
                ?: throw IllegalStateException("no active network")
            val signal = CancellationSignal()
            val latch = java.util.concurrent.CountDownLatch(1)
            ctx.onCancel {
                signal.cancel()
                latch.countDown()
            }
            DnsResolver.getInstance().rawQuery(
                active,
                message,
                DnsResolver.FLAG_EMPTY,
                dnsExecutor,
                signal,
                object : DnsResolver.Callback<ByteArray> {
                    override fun onAnswer(answer: ByteArray, rcode: Int) {
                        if (rcode == 0) {
                            ctx.rawSuccess(answer)
                        } else {
                            ctx.errorCode(rcode)
                        }
                        latch.countDown()
                    }

                    override fun onError(error: DnsResolver.DnsException) {
                        val cause = error.cause
                        if (cause is ErrnoException) {
                            ctx.errnoCode(cause.errno)
                        } else {
                            ctx.errorCode(2)
                        }
                        latch.countDown()
                    }
                },
            )
            latch.await()
        }
    }

    private class ListStringIterator(items: List<String>) : StringIterator {
        private val list = items
        private var index = 0
        override fun len(): Int = list.size
        override fun hasNext(): Boolean = index < list.size
        override fun next(): String = list[index++]
    }

    private class ListNetworkInterfaceIterator(items: List<LibboxNetworkInterface>) : NetworkInterfaceIterator {
        private val list = items
        private var index = 0
        override fun hasNext(): Boolean = index < list.size
        override fun next(): LibboxNetworkInterface = list[index++]
    }

    private fun InterfaceAddress.toPrefix(): String = if (address is Inet6Address) {
        "${InetAddress.getByAddress(address.address).hostAddress}/$networkPrefixLength"
    } else {
        "${address.hostAddress}/$networkPrefixLength"
    }

    companion object {
        private const val TAG = "SingerVpnService"
        private const val CHANNEL_ID = "singer_vpn"
        private const val NOTIFICATION_ID = 1001

        const val ACTION_START = "uk.ihack.singer.action.START"
        const val ACTION_STOP = "uk.ihack.singer.action.STOP"
        const val EXTRA_RUN_ID = "run_id"
        const val EXTRA_CONFIG = "config"
        const val EXTRA_TUN_MODE = "tun_mode"
        const val EXTRA_GROUP_NAME = "group_name"
        const val EXTRA_NODE_TAG = "node_tag"
        const val EXTRA_NODE_INDEX = "node_index"
        const val EXTRA_API_ADDR = "api_addr"
        const val EXTRA_API_SECRET = "api_secret"

        private val libboxInitialized = AtomicBoolean(false)
    }
}
