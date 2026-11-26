package com.onusly.app.service

import android.app.job.JobParameters
import android.app.job.JobService
import android.util.Log
import com.onusly.app.OnuslyApplication
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

class EnforcementSyncJobService : JobService() {

    private val jobScope = CoroutineScope(Dispatchers.IO)
    private var syncJob: Job? = null

    override fun onStartJob(params: JobParameters?): Boolean {
        Log.d(TAG, "Starting periodic background enforcement sync job")
        val app = application as? OnuslyApplication
        if (app == null || !app.authManager.isSignedIn()) {
            Log.d(TAG, "User not signed in or app context missing. Ending job.")
            return false // No work remaining
        }

        syncJob = jobScope.launch {
            try {
                // 1. Sync blocklist from backend
                app.syncRepository.syncBlocklist()

                // 2. Check accessibility service status & DNS Shield status and transmit heartbeat
                val issues = mutableListOf<String>()
                val accessibilityActive = OnuslyAccessibilityService.isServiceRunning ||
                        OnuslyAccessibilityService.isServiceEnabled(this@EnforcementSyncJobService)
                if (!accessibilityActive) {
                    issues.add("accessibility_disabled")
                }
                val ctx = this@EnforcementSyncJobService
                val activeDomainsCount = com.onusly.app.engine.DomainMatcher.getActiveBlockedDomains(
                    app.syncRepository.getActiveCache()
                ).size
                if (activeDomainsCount > 0 && com.onusly.app.dns.DnsShieldManager.isUserEnabled(ctx) &&
                    (!com.onusly.app.dns.DnsShieldManager.isRunning.value || com.onusly.app.dns.DnsShieldManager.isStrictPrivateDnsActive(ctx))
                ) {
                    issues.add("dns_drift")
                }
                app.syncRepository.sendHeartbeat(issues)
                app.syncRepository.syncWellbeingToday(this@EnforcementSyncJobService)

                Log.d(TAG, "Periodic background enforcement sync completed successfully")
                jobFinished(params, false) // Finished, do not reschedule immediately (periodic scheduler handles next)
            } catch (e: Exception) {
                Log.e(TAG, "Error during periodic background sync", e)
                jobFinished(params, true) // Reschedule with backoff if network or transient error
            }
        }

        return true // Work is running asynchronously in background coroutine
    }

    override fun onStopJob(params: JobParameters?): Boolean {
        Log.d(TAG, "Periodic background enforcement sync job was cancelled by system")
        syncJob?.cancel()
        return true // Reschedule if aborted prematurely
    }

    companion object {
        private const val TAG = "EnforcementSyncJob"
    }
}
