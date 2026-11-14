package com.onusly.app.receiver

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.onusly.app.OnuslyApplication
import com.onusly.app.service.EnforcementSyncScheduler
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent?) {
        val action = intent?.action ?: return
        val isRecognizedAction = action == Intent.ACTION_BOOT_COMPLETED ||
                action == Intent.ACTION_MY_PACKAGE_REPLACED ||
                action == "android.intent.action.QUICKBOOT_POWERON" ||
                action == "com.htc.intent.action.QUICKBOOT_POWERON"

        if (!isRecognizedAction) {
            return
        }

        Log.d(TAG, "Device lifecycle event received: $action. Ensuring periodic sync is scheduled.")
        // Ensure background JobScheduler task persists and is registered
        EnforcementSyncScheduler.schedule(context)

        val pendingResult = goAsync()
        val app = (context.applicationContext as? OnuslyApplication)
            ?: (context as? OnuslyApplication)

        if (app == null || !app.authManager.isSignedIn()) {
            pendingResult.finish()
            return
        }

        app.applicationScope.launch(Dispatchers.IO) {
            try {
                withTimeoutOrNull(20_000) {
                    try {
                        app.syncRepository.syncBlocklist()
                    } catch (e: Exception) {
                        Log.e(TAG, "Failed to sync blocklist on boot", e)
                    }
                    try {
                        app.realtimeWebSocketManager.connect()
                    } catch (e: Exception) {
                        Log.e(TAG, "Failed to connect WebSocket on boot", e)
                    }
                }
            } finally {
                pendingResult.finish()
            }
        }
    }

    companion object {
        private const val TAG = "BootReceiver"
    }
}
