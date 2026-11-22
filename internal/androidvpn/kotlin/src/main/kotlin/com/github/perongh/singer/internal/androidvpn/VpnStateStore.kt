package com.github.perongh.singer.internal.androidvpn

import android.app.ActivityManager
import android.content.Context
import java.io.File
import org.json.JSONObject

internal data class StoredVpnState(
    val running: Boolean = false,
    val pid: Int = 0,
    val configJSON: String = "",
    val tunMode: Boolean = true,
    val groupName: String = "",
    val nodeTag: String = "",
    val nodeIndex: Int = -1,
    val apiAddr: String = "",
    val apiSecret: String = "",
)

/**
 * Atomic file-backed state shared across the main UI/Tile process and the isolated :vpn process.
 */
internal object VpnStateStore {
    private const val FILE_NAME = "vpn_state.json"

    private fun file(context: Context): File = File(context.filesDir, FILE_NAME)

    @Synchronized
    fun read(context: Context): StoredVpnState {
        val f = file(context)
        if (!f.exists()) return StoredVpnState()
        return runCatching {
            val obj = JSONObject(f.readText())
            StoredVpnState(
                running = obj.optBoolean("running", false),
                pid = obj.optInt("pid", 0),
                configJSON = obj.optString("configJSON", ""),
                tunMode = obj.optBoolean("tunMode", true),
                groupName = obj.optString("groupName", ""),
                nodeTag = obj.optString("nodeTag", ""),
                nodeIndex = obj.optInt("nodeIndex", -1),
                apiAddr = obj.optString("apiAddr", ""),
                apiSecret = obj.optString("apiSecret", ""),
            )
        }.getOrDefault(StoredVpnState())
    }

    @Synchronized
    fun readLive(context: Context): StoredVpnState {
        val st = read(context)
        if (!st.running) return st
        if (st.pid <= 0 || !isProcessAlive(context, st.pid)) {
            return st.copy(running = false, pid = 0)
        }
        return st
    }

    @Synchronized
    fun write(context: Context, state: StoredVpnState) {
        val obj = JSONObject().apply {
            put("running", state.running)
            put("pid", state.pid)
            put("configJSON", state.configJSON)
            put("tunMode", state.tunMode)
            put("groupName", state.groupName)
            put("nodeTag", state.nodeTag)
            put("nodeIndex", state.nodeIndex)
            put("apiAddr", state.apiAddr)
            put("apiSecret", state.apiSecret)
        }
        val target = file(context)
        val tmp = File(context.filesDir, "$FILE_NAME.tmp")
        tmp.writeText(obj.toString())
        if (!tmp.renameTo(target)) {
            target.writeText(obj.toString())
            tmp.delete()
        }
    }

    @Synchronized
    fun markStopped(context: Context) {
        val current = read(context)
        if (!current.running && current.pid == 0) return
        write(context, current.copy(running = false, pid = 0))
    }

    private fun isProcessAlive(context: Context, pid: Int): Boolean {
        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as? ActivityManager ?: return false
        val procs = am.runningAppProcesses ?: return false
        return procs.any { it.pid == pid }
    }
}
