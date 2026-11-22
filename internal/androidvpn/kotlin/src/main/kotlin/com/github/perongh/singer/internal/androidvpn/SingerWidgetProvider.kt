package com.github.perongh.singer.internal.androidvpn

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.widget.RemoteViews

class SingerWidgetProvider : AppWidgetProvider() {
    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        val snap = VpnBinderClient.ensureBound(context).snapshots.value
        val views = buildViews(context.applicationContext, snap)
        for (id in appWidgetIds) {
            appWidgetManager.updateAppWidget(id, views)
        }
    }

    companion object {
        internal fun update(context: Context, snapshot: VpnSnapshot) {
            val app = context.applicationContext
            val manager = AppWidgetManager.getInstance(app)
            val ids = manager.getAppWidgetIds(ComponentName(app, SingerWidgetProvider::class.java))
            if (ids.isEmpty()) return
            val views = buildViews(app, snapshot)
            manager.updateAppWidget(ids, views)
        }

        private fun buildViews(app: Context, snapshot: VpnSnapshot): RemoteViews {
            val views = RemoteViews(app.packageName, R.layout.widget_singer)

            val dotRes: Int
            val subtitle: String
            val clickIntent: PendingIntent?

            when (snapshot.state) {
                VpnIpc.STATE_RUNNING -> {
                    dotRes = R.drawable.widget_dot_active
                    val tag = snapshot.nodeTag.ifEmpty { "Connected" }
                    subtitle = "$tag · Tap to stop"
                    clickIntent = stopPendingIntent(app)
                }

                VpnIpc.STATE_STARTING -> {
                    dotRes = R.drawable.widget_dot_starting
                    val tag = snapshot.nodeTag.ifEmpty { "Starting" }
                    subtitle = "$tag · Starting…"
                    clickIntent = stopPendingIntent(app)
                }

                else -> {
                    dotRes = R.drawable.widget_dot_inactive
                    subtitle = "Disconnected"
                    clickIntent = openAppPendingIntent(app)
                }
            }

            views.setImageViewResource(R.id.widget_dot, dotRes)
            views.setTextViewText(R.id.widget_subtitle, subtitle)
            if (clickIntent != null) {
                views.setOnClickPendingIntent(R.id.widget_root, clickIntent)
            }
            return views
        }

        private fun stopPendingIntent(app: Context): PendingIntent {
            val intent = Intent(app, SingerVpnService::class.java).apply {
                action = SingerVpnService.ACTION_STOP
            }
            return PendingIntent.getService(
                app,
                10,
                intent,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        }

        private fun openAppPendingIntent(app: Context): PendingIntent? {
            val launch = app.packageManager.getLaunchIntentForPackage(app.packageName)?.apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            } ?: return null
            return PendingIntent.getActivity(
                app,
                11,
                launch,
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        }
    }
}
