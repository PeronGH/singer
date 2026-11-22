package com.github.perongh.singer.internal.androidvpn

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.DnsResolver
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.CancellationSignal
import android.os.ParcelFileDescriptor
import android.os.Process
import android.os.ResultReceiver
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

class SingerVpnService : VpnService(), CommandServerHandler, PlatformInterface {
    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }
    private val worker = Executors.newSingleThreadExecutor()
    private val dnsExecutor = Executors.newCachedThreadPool()

    @Volatile
    private var commandServer: CommandServer? = null

    @Volatile
    private var tunPfd: ParcelFileDescriptor? = null

    @Volatile
    private var defaultNetwork: Network? = null

    @Volatile
    private var interfaceListener: InterfaceUpdateListener? = null

    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> {
                val config = intent.getStringExtra(EXTRA_CONFIG) ?: ""
                val tunMode = intent.getBooleanExtra(EXTRA_TUN_MODE, false)
                @Suppress("DEPRECATION")
                val receiver = intent.getParcelableExtra<ResultReceiver>(EXTRA_RECEIVER)
                promoteForeground(tunMode)
                worker.execute {
                    try {
                        startLibbox(config)
                        receiver?.send(RESULT_OK, null)
                    } catch (e: Throwable) {
                        Log.e(TAG, "start sing-box failed", e)
                        stopLibbox()
                        receiver?.send(RESULT_ERROR, Bundle().apply {
                            putString(EXTRA_ERROR, e.message ?: e.toString())
                        })
                        stopSelf()
                    }
                }
            }
            ACTION_STOP -> {
                worker.execute {
                    stopLibbox()
                    stopSelf()
                }
            }
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        worker.execute {
            stopLibbox()
            stopSelf()
        }
        super.onRevoke()
    }

    override fun onDestroy() {
        stopLibbox()
        worker.shutdownNow()
        dnsExecutor.shutdownNow()
        super.onDestroy()
    }

    private fun promoteForeground(tunMode: Boolean) {
        val manager = getSystemService(NotificationManager::class.java)
        if (manager.getNotificationChannel(CHANNEL_ID) == null) {
            manager.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "Singer VPN", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val text = if (tunMode) "VPN tunnel active" else "Local proxy active"
        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Singer")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setOngoing(true)
            .build()

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

    private fun startLibbox(configJSON: String) {
        ensureLibboxSetup()
        stopLibbox()
        val server = Libbox.newCommandServer(this, this)
        commandServer = server
        server.start()
        server.startOrReloadService(configJSON, OverrideOptions())
    }

    private fun stopLibbox() {
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
        stopForeground(STOP_FOREGROUND_REMOVE)
    }

    // CommandServerHandler

    override fun serviceStop() {
        worker.execute {
            stopLibbox()
            stopSelf()
        }
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
            ctx.onCancel { signal.cancel() }
            val latch = java.util.concurrent.CountDownLatch(1)
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

        const val ACTION_START = "com.perongh.singer.action.START"
        const val ACTION_STOP = "com.perongh.singer.action.STOP"
        const val EXTRA_CONFIG = "config"
        const val EXTRA_TUN_MODE = "tun_mode"
        const val EXTRA_RECEIVER = "receiver"
        const val EXTRA_ERROR = "error"
        const val RESULT_OK = 0
        const val RESULT_ERROR = 1

        private val libboxInitialized = AtomicBoolean(false)
    }
}
