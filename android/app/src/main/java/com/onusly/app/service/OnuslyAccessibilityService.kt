package com.onusly.app.service

import android.accessibilityservice.AccessibilityService
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.SystemClock
import android.provider.Settings
import android.text.TextUtils
import android.view.accessibility.AccessibilityEvent
import com.onusly.app.OnuslyApplication
import com.onusly.app.engine.BlockerEngine
import com.onusly.app.ui.LockActivity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class OnuslyAccessibilityService : AccessibilityService() {

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private var periodicSyncJob: Job? = null

    private var lastBlockedPackage: String? = null
    private var lastBlockLaunchTimestamp: Long = 0L

    companion object {
        var isServiceRunning: Boolean = false
            private set

        fun isServiceEnabled(context: Context): Boolean {
            val expectedComponent = ComponentName(context, OnuslyAccessibilityService::class.java)
            val enabledServices = Settings.Secure.getString(
                context.contentResolver,
                Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES
            ) ?: return false

            val colonSplitter = TextUtils.SimpleStringSplitter(':')
            colonSplitter.setString(enabledServices)
            while (colonSplitter.hasNext()) {
                val componentStr = colonSplitter.next()
                val enabledComponent = ComponentName.unflattenFromString(componentStr)
                if (enabledComponent != null && enabledComponent == expectedComponent) {
                    return true
                }
            }
            return false
        }
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        isServiceRunning = true

        // Start periodic sync every 60 seconds while service is running
        periodicSyncJob = serviceScope.launch(Dispatchers.IO) {
            while (isActive) {
                delay(60_000)
                try {
                    val app = application as? OnuslyApplication
                    app?.syncRepository?.syncBlocklist()
                } catch (_: Exception) {
                }
            }
        }
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event == null) return
        if (event.eventType != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED) return

        val pkgName = event.packageName?.toString() ?: return

        val app = application as? OnuslyApplication ?: return
        val activeCache = app.syncRepository.getActiveCache()

        val evaluation = BlockerEngine.evaluatePackage(pkgName, activeCache)
        if (evaluation.isBlocked) {
            val now = SystemClock.uptimeMillis()
            // Throttle duplicate launches for the same package within 1000ms to avoid intent spam
            if (pkgName == lastBlockedPackage && (now - lastBlockLaunchTimestamp) < 1000) {
                return
            }

            lastBlockedPackage = pkgName
            lastBlockLaunchTimestamp = now

            val intent = Intent(this, LockActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or
                        Intent.FLAG_ACTIVITY_CLEAR_TOP or
                        Intent.FLAG_ACTIVITY_SINGLE_TOP
                putExtra(LockActivity.EXTRA_BLOCKED_PACKAGE, pkgName)
                putStringArrayListExtra(
                    LockActivity.EXTRA_GOAL_TITLES,
                    ArrayList(evaluation.matchingTargets.map { it.goalTitle })
                )
                putStringArrayListExtra(
                    LockActivity.EXTRA_GOAL_STATUSES,
                    ArrayList(evaluation.matchingTargets.map { it.status })
                )
            }
            startActivity(intent)
        } else {
            if (pkgName != packageName) {
                lastBlockedPackage = null
            }
        }
    }

    override fun onInterrupt() {
    }

    override fun onDestroy() {
        super.onDestroy()
        isServiceRunning = false
        periodicSyncJob?.cancel()
        serviceScope.cancel()
    }
}
