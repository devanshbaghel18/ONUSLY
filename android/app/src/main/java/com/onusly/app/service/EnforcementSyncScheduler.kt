package com.onusly.app.service

import android.app.job.JobInfo
import android.app.job.JobScheduler
import android.content.ComponentName
import android.content.Context
import android.util.Log

object EnforcementSyncScheduler {

    const val JOB_ID = 1001
    private const val TAG = "EnforcementSyncScheduler"
    // 15 minutes: Android JobScheduler minimum periodic interval
    private const val PERIODIC_INTERVAL_MS = 15 * 60 * 1000L

    fun schedule(context: Context): Boolean {
        return try {
            val scheduler = context.getSystemService(Context.JOB_SCHEDULER_SERVICE) as? JobScheduler
                ?: return false

            val component = ComponentName(context, EnforcementSyncJobService::class.java)
            val jobInfo = JobInfo.Builder(JOB_ID, component)
                .setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY)
                .setPeriodic(PERIODIC_INTERVAL_MS)
                .setPersisted(true)
                .build()

            val result = scheduler.schedule(jobInfo)
            val success = result == JobScheduler.RESULT_SUCCESS
            Log.d(TAG, "Scheduled periodic enforcement sync job: success=$success")
            success
        } catch (e: Exception) {
            Log.e(TAG, "Failed to schedule enforcement sync job", e)
            false
        }
    }

    fun cancel(context: Context) {
        try {
            val scheduler = context.getSystemService(Context.JOB_SCHEDULER_SERVICE) as? JobScheduler
            scheduler?.cancel(JOB_ID)
            Log.d(TAG, "Cancelled periodic enforcement sync job")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to cancel enforcement sync job", e)
        }
    }

    fun isJobScheduled(context: Context): Boolean {
        return try {
            val scheduler = context.getSystemService(Context.JOB_SCHEDULER_SERVICE) as? JobScheduler
            scheduler?.allPendingJobs?.any { it.id == JOB_ID } ?: false
        } catch (_: Exception) {
            false
        }
    }
}
