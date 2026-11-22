package com.github.perongh.singer.internal.androidvpn

import android.content.Context
import java.io.File
import org.json.JSONObject

internal data class LastSession(
    val configJSON: String = "",
    val tunMode: Boolean = true,
    val groupName: String = "",
    val nodeTag: String = "",
    val nodeIndex: Int = -1,
    val apiAddr: String = "",
    val apiSecret: String = "",
)

/**
 * Persists the last user-selected node configuration so Quick Settings can
 * cold-start the VPN when :vpn is not running. Live state travels over Binder.
 */
internal object LastSessionStore {
    private const val FILE_NAME = "last_session.json"

    private fun file(context: Context): File = File(context.filesDir, FILE_NAME)

    @Synchronized
    fun read(context: Context): LastSession {
        val f = file(context)
        if (!f.exists()) return LastSession()
        return runCatching {
            val obj = JSONObject(f.readText())
            LastSession(
                configJSON = obj.optString("configJSON", ""),
                tunMode = obj.optBoolean("tunMode", true),
                groupName = obj.optString("groupName", ""),
                nodeTag = obj.optString("nodeTag", ""),
                nodeIndex = obj.optInt("nodeIndex", -1),
                apiAddr = obj.optString("apiAddr", ""),
                apiSecret = obj.optString("apiSecret", ""),
            )
        }.getOrDefault(LastSession())
    }

    @Synchronized
    fun write(context: Context, session: LastSession) {
        val obj = JSONObject().apply {
            put("configJSON", session.configJSON)
            put("tunMode", session.tunMode)
            put("groupName", session.groupName)
            put("nodeTag", session.nodeTag)
            put("nodeIndex", session.nodeIndex)
            put("apiAddr", session.apiAddr)
            put("apiSecret", session.apiSecret)
        }
        val target = file(context)
        val tmp = File(context.filesDir, "$FILE_NAME.tmp")
        tmp.writeText(obj.toString())
        if (!tmp.renameTo(target)) {
            target.writeText(obj.toString())
            tmp.delete()
        }
    }
}
