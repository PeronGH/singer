package com.github.perongh.singer.internal.androidvpn

import android.os.Bundle

internal data class VpnSnapshot(
    val state: Int = VpnIpc.STATE_STOPPED,
    val runId: Long = 0L,
    val tunMode: Boolean = true,
    val groupName: String = "",
    val nodeTag: String = "",
    val nodeIndex: Int = -1,
    val apiAddr: String = "",
    val apiSecret: String = "",
    val error: String = "",
) {
    val isRunning: Boolean get() = state == VpnIpc.STATE_RUNNING

    fun toStatus(): Status = Status(
        running = isRunning,
        tunMode = tunMode,
        groupName = groupName,
        nodeTag = nodeTag,
        nodeIndex = nodeIndex.toLong(),
        apiAddr = apiAddr,
        apiSecret = apiSecret,
    )

    fun toBundle(): Bundle = Bundle().apply {
        putInt(VpnIpc.KEY_STATE, state)
        putLong(VpnIpc.KEY_RUN_ID, runId)
        putBoolean(VpnIpc.KEY_TUN_MODE, tunMode)
        putString(VpnIpc.KEY_GROUP_NAME, groupName)
        putString(VpnIpc.KEY_NODE_TAG, nodeTag)
        putInt(VpnIpc.KEY_NODE_INDEX, nodeIndex)
        putString(VpnIpc.KEY_API_ADDR, apiAddr)
        putString(VpnIpc.KEY_API_SECRET, apiSecret)
        putString(VpnIpc.KEY_ERROR, error)
    }

    companion object {
        fun fromBundle(b: Bundle?): VpnSnapshot {
            if (b == null) return VpnSnapshot()
            return VpnSnapshot(
                state = b.getInt(VpnIpc.KEY_STATE, VpnIpc.STATE_STOPPED),
                runId = b.getLong(VpnIpc.KEY_RUN_ID, 0L),
                tunMode = b.getBoolean(VpnIpc.KEY_TUN_MODE, true),
                groupName = b.getString(VpnIpc.KEY_GROUP_NAME, ""),
                nodeTag = b.getString(VpnIpc.KEY_NODE_TAG, ""),
                nodeIndex = b.getInt(VpnIpc.KEY_NODE_INDEX, -1),
                apiAddr = b.getString(VpnIpc.KEY_API_ADDR, ""),
                apiSecret = b.getString(VpnIpc.KEY_API_SECRET, ""),
                error = b.getString(VpnIpc.KEY_ERROR, ""),
            )
        }
    }
}

internal object VpnIpc {
    const val ACTION_BIND = "uk.ihack.singer.action.BIND"

    const val MSG_REGISTER = 1
    const val MSG_UNREGISTER = 2
    const val MSG_STOP = 3
    const val MSG_STATE = 10

    const val STATE_STOPPED = 0
    const val STATE_STARTING = 1
    const val STATE_RUNNING = 2
    const val STATE_ERROR = 3

    const val KEY_STATE = "state"
    const val KEY_RUN_ID = "run_id"
    const val KEY_TUN_MODE = "tun_mode"
    const val KEY_GROUP_NAME = "group_name"
    const val KEY_NODE_TAG = "node_tag"
    const val KEY_NODE_INDEX = "node_index"
    const val KEY_API_ADDR = "api_addr"
    const val KEY_API_SECRET = "api_secret"
    const val KEY_ERROR = "error"
}
