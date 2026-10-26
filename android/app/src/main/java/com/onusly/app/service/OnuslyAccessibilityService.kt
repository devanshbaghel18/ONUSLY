package com.onusly.app.service

import android.accessibilityservice.AccessibilityService
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.os.SystemClock
import android.provider.Settings
import android.text.TextUtils
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityWindowInfo
import com.onusly.app.OnuslyApplication
import com.onusly.app.data.model.BlockEvaluationResult
import com.onusly.app.data.model.CachedBlocklist
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

    @Volatile
    private var cachedBlocklist: CachedBlocklist? = null
    private var blocklistPrefs: SharedPreferences? = null

    private val prefChangeListener = SharedPreferences.OnSharedPreferenceChangeListener { _, _ ->
        reloadCachedBlocklist()
    }

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

        blocklistPrefs = getSharedPreferences("onusly_blocklist_cache", Context.MODE_PRIVATE)
        blocklistPrefs?.registerOnSharedPreferenceChangeListener(prefChangeListener)
        reloadCachedBlocklist()

        // Also observe StateFlow updates from SyncRepository for in-memory cache consistency
        serviceScope.launch {
            val app = application as? OnuslyApplication
            app?.syncRepository?.cachedBlocklistFlow?.collect { updatedCache ->
                cachedBlocklist = updatedCache
            }
        }

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

    private fun reloadCachedBlocklist() {
        val app = application as? OnuslyApplication ?: return
        cachedBlocklist = app.syncRepository.getActiveCache()
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        if (event == null) return
        if (event.eventType != AccessibilityEvent.TYPE_WINDOW_STATE_CHANGED &&
            event.eventType != AccessibilityEvent.TYPE_WINDOWS_CHANGED
        ) return

        // In-memory cached blocklist: avoid parsing SharedPreferences JSON synchronously on every window event
        var activeCache = cachedBlocklist
        if (activeCache == null) {
            reloadCachedBlocklist()
            activeCache = cachedBlocklist
        }

        val pkgName = event.packageName?.toString()

        // 1. Evaluate primary event package if present
        if (!pkgName.isNullOrBlank()) {
            val evaluation = BlockerEngine.evaluatePackage(pkgName, activeCache)
            if (evaluation.isBlocked) {
                launchLockActivity(pkgName, evaluation)
                return
            }
        }

        // 2. Multi-window / Picture-in-Picture interactive window evaluation (ONUS-MED-04)
        val blockedWindowHandled = inspectInteractiveWindows(activeCache)
        if (blockedWindowHandled) {
            return
        }

        // 3. Debounce cleanup when foreground package is unblocked
        if (!pkgName.isNullOrBlank()) {
            // When evaluation.isBlocked is false:
            // If the current package is ONUSLY itself (pkgName == packageName) or an allowed app,
            // ensure lastBlockedPackage = null. This prevents the 1000ms debounce bypass when the
            // user taps "Return to ONUSLY App" and immediately switches back to the blocked app.
            lastBlockedPackage = null
        }
    }

    /**
     * Evaluates interactive windows (multi-window, floating windows, Picture-in-Picture)
     * enabled by flagRetrieveInteractiveWindows (ONUS-MED-04).
     *
     * Privacy & Security Note:
     * canRetrieveWindowContent is set to false in accessibility_service_config.xml to ensure ONUSLY
     * never accesses sensitive on-screen content (passwords, messages, etc.). Because of this,
     * window.root may return null on standard Android runtimes. When window hierarchy / root is
     * accessible, we inspect and enforce restrictions on blocked applications while strictly
     * protecting system UI, input method (keyboard), and emergency interfaces.
     */
    private fun inspectInteractiveWindows(activeCache: CachedBlocklist?): Boolean {
        val currentWindows = try {
            windows
        } catch (_: Exception) {
            null
        } ?: return false

        for (window in currentWindows) {
            // Strictly exclude non-application windows: system UI, status bar, navigation bar,
            // input methods (keyboard), and accessibility overlays.
            if (window.type != AccessibilityWindowInfo.TYPE_APPLICATION) {
                continue
            }

            // Verify if the application window is active, focused, or in picture-in-picture mode
            val isInteractive = window.isActive || window.isFocused || window.isInPictureInPictureMode
            if (!isInteractive) {
                continue
            }

            // Obtain package name from window root if available
            val winPkg = try {
                window.root?.packageName?.toString()
            } catch (_: Exception) {
                null
            } ?: continue

            // Evaluate package against cached blocklist and allowed system/emergency packages
            val winEvaluation = BlockerEngine.evaluatePackage(winPkg, activeCache)
            if (winEvaluation.isBlocked) {
                launchLockActivity(winPkg, winEvaluation)
                return true
            }
        }
        return false
    }

    private fun launchLockActivity(pkgName: String, evaluation: BlockEvaluationResult) {
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
    }

    override fun onInterrupt() {
    }

    override fun onDestroy() {
        super.onDestroy()
        isServiceRunning = false
        blocklistPrefs?.unregisterOnSharedPreferenceChangeListener(prefChangeListener)
        blocklistPrefs = null
        cachedBlocklist = null
        periodicSyncJob?.cancel()
        serviceScope.cancel()
    }
}
