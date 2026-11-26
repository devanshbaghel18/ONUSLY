package com.onusly.app.wellbeing

import android.app.AppOpsManager
import android.app.usage.UsageEvents
import android.app.usage.UsageStatsManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Process
import android.provider.Settings
import com.onusly.app.dns.DomainQueryHit
import org.json.JSONArray
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale

data class AndroidAppUsage(
    val appId: String,
    val displayName: String,
    val seconds: Long,
    val opens: Long
)

data class DailyWellbeingSnapshot(
    val date: String,
    val totalScreenSeconds: Long,
    val apps: List<AndroidAppUsage>,
    val domains: List<DomainQueryHit>
)

/**
 * Collects system-accurate Android daily screen time and app launch counts using UsageStatsManager,
 * and merges Mobile DNS domain hit telemetry into a unified Digital Wellbeing ingest payload.
 */
object AppUsageTracker {

    private val EXCLUDED_SYSTEM_PACKAGES = setOf(
        "android",
        "com.android.systemui",
        "com.android.launcher",
        "com.android.launcher3",
        "com.google.android.apps.nexuslauncher",
        "com.sec.android.app.launcher",
        "com.miui.home",
        "com.oneplus.launcher",
        "com.oppo.launcher",
        "com.vivo.launcher"
    )

    fun hasUsageStatsPermission(context: Context): Boolean {
        return try {
            val appOps = context.getSystemService(Context.APP_OPS_SERVICE) as? AppOpsManager ?: return false
            val mode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                appOps.unsafeCheckOpNoThrow(
                    AppOpsManager.OPSTR_GET_USAGE_STATS,
                    Process.myUid(),
                    context.packageName
                )
            } else {
                @Suppress("DEPRECATION")
                appOps.checkOpNoThrow(
                    AppOpsManager.OPSTR_GET_USAGE_STATS,
                    Process.myUid(),
                    context.packageName
                )
            }
            mode == AppOpsManager.MODE_ALLOWED
        } catch (_: Exception) {
            false
        }
    }

    fun createUsageAccessIntent(): Intent {
        return Intent(Settings.ACTION_USAGE_ACCESS_SETTINGS)
    }

    fun getTodayDateKey(nowMillis: Long = System.currentTimeMillis()): String {
        val sdf = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        return sdf.format(Date(nowMillis))
    }

    /**
     * Collects today's screen time and app launch counts from Android UsageStatsManager.
     * Uses UsageEvents (ACTIVITY_RESUMED / MOVE_TO_FOREGROUND -> ACTIVITY_PAUSED / MOVE_TO_BACKGROUND)
     * so foreground duration matches Android Digital Wellbeing accurately across midnight boundaries.
     */
    fun collectTodaySnapshot(
        context: Context,
        dnsHits: List<DomainQueryHit>,
        nowMillis: Long = System.currentTimeMillis()
    ): DailyWellbeingSnapshot {
        return collectSnapshotForDaysAgo(context, daysAgo = 0, dnsHits = dnsHits, nowMillis = nowMillis)
    }

    fun collectSnapshotForDaysAgo(
        context: Context,
        daysAgo: Int,
        dnsHits: List<DomainQueryHit> = emptyList(),
        nowMillis: Long = System.currentTimeMillis()
    ): DailyWellbeingSnapshot {
        val cal = Calendar.getInstance().apply {
            timeInMillis = nowMillis
            add(Calendar.DAY_OF_YEAR, -daysAgo)
            set(Calendar.HOUR_OF_DAY, 0)
            set(Calendar.MINUTE, 0)
            set(Calendar.SECOND, 0)
            set(Calendar.MILLISECOND, 0)
        }
        val startOfDay = cal.timeInMillis
        val dateKey = getTodayDateKey(startOfDay)
        val endOfDay = if (daysAgo == 0) {
            nowMillis
        } else {
            cal.clone().let { c ->
                (c as Calendar).add(Calendar.DAY_OF_YEAR, 1)
                c.timeInMillis - 1L
            }
        }

        if (!hasUsageStatsPermission(context)) {
            return DailyWellbeingSnapshot(
                date = dateKey,
                totalScreenSeconds = 0L,
                apps = emptyList(),
                domains = dnsHits
            )
        }

        val apps = queryEventsForDay(context, startOfDay, endOfDay)
        val totalScreen = apps.sumOf { it.seconds }.coerceAtMost(86_400L)

        return DailyWellbeingSnapshot(
            date = dateKey,
            totalScreenSeconds = totalScreen,
            apps = apps,
            domains = dnsHits
        )
    }

    fun collectPastWeekSnapshots(
        context: Context,
        dnsHitsByDate: Map<String, List<DomainQueryHit>>,
        daysCount: Int = 7,
        nowMillis: Long = System.currentTimeMillis()
    ): List<DailyWellbeingSnapshot> {
        val list = mutableListOf<DailyWellbeingSnapshot>()
        for (d in 0 until daysCount) {
            val cal = Calendar.getInstance().apply {
                timeInMillis = nowMillis
                add(Calendar.DAY_OF_YEAR, -d)
            }
            val dateKey = getTodayDateKey(cal.timeInMillis)
            val dnsHits = dnsHitsByDate[dateKey].orEmpty()
            val snap = collectSnapshotForDaysAgo(context, daysAgo = d, dnsHits = dnsHits, nowMillis = nowMillis)
            if (snap.apps.isNotEmpty() || snap.domains.isNotEmpty()) {
                list.add(snap)
            }
        }
        return list
    }

    private fun queryEventsForDay(context: Context, startMs: Long, endMs: Long): List<AndroidAppUsage> {
        val usm = context.getSystemService(Context.USAGE_STATS_SERVICE) as? UsageStatsManager ?: return emptyList()
        val pm = context.packageManager

        val durationByPkg = HashMap<String, Long>()
        val opensByPkg = HashMap<String, Long>()
        val activeStartByPkg = HashMap<String, Long>()

        try {
            val events = usm.queryEvents(startMs, endMs)
            val event = UsageEvents.Event()
            while (events.hasNextEvent()) {
                events.getNextEvent(event)
                val pkg = event.packageName ?: continue
                if (EXCLUDED_SYSTEM_PACKAGES.contains(pkg)) continue

                @Suppress("DEPRECATION")
                val isForeground = event.eventType == UsageEvents.Event.ACTIVITY_RESUMED ||
                    event.eventType == UsageEvents.Event.MOVE_TO_FOREGROUND
                @Suppress("DEPRECATION")
                val isBackground = event.eventType == UsageEvents.Event.ACTIVITY_PAUSED ||
                    event.eventType == UsageEvents.Event.MOVE_TO_BACKGROUND
                val isScreenOff = (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P &&
                    (event.eventType == UsageEvents.Event.SCREEN_NON_INTERACTIVE ||
                        event.eventType == UsageEvents.Event.KEYGUARD_SHOWN))

                if (isScreenOff) {
                    for ((openPkg, started) in activeStartByPkg) {
                        if (event.timeStamp > started) {
                            durationByPkg[openPkg] = (durationByPkg[openPkg] ?: 0L) + (event.timeStamp - started)
                        }
                    }
                    activeStartByPkg.clear()
                } else if (isForeground) {
                    if (!activeStartByPkg.containsKey(pkg)) {
                        activeStartByPkg[pkg] = event.timeStamp
                        opensByPkg[pkg] = (opensByPkg[pkg] ?: 0L) + 1L
                    }
                } else if (isBackground) {
                    val started = activeStartByPkg.remove(pkg)
                    if (started != null && event.timeStamp > started) {
                        durationByPkg[pkg] = (durationByPkg[pkg] ?: 0L) + (event.timeStamp - started)
                    }
                }
            }
        } catch (_: Exception) {
        }

        // Flush any currently open foreground app up to endMs
        for ((pkg, started) in activeStartByPkg) {
            if (endMs > started) {
                durationByPkg[pkg] = (durationByPkg[pkg] ?: 0L) + (endMs - started)
            }
        }

        // Fallback to aggregated queryUsageStats if UsageEvents returned empty
        if (durationByPkg.isEmpty()) {
            try {
                val statsList = usm.queryUsageStats(UsageStatsManager.INTERVAL_DAILY, startMs, endMs)
                for (st in statsList.orEmpty()) {
                    val pkg = st.packageName ?: continue
                    if (EXCLUDED_SYSTEM_PACKAGES.contains(pkg)) continue
                    val ms = st.totalTimeInForeground
                    if (ms > 1000L) {
                        durationByPkg[pkg] = (durationByPkg[pkg] ?: 0L) + ms
                        opensByPkg[pkg] = maxOf(opensByPkg[pkg] ?: 0L, 1L)
                    }
                }
            } catch (_: Exception) {
            }
        }

        val results = mutableListOf<AndroidAppUsage>()
        for ((pkg, ms) in durationByPkg) {
            val sec = ms / 1000L
            val opens = opensByPkg[pkg] ?: 0L
            if (sec <= 0L && opens <= 0L) continue
            results.add(
                AndroidAppUsage(
                    appId = pkg,
                    displayName = resolveAppLabel(pm, pkg),
                    seconds = sec,
                    opens = opens
                )
            )
        }
        return results.sortedByDescending { it.seconds }
    }

    private fun resolveAppLabel(pm: PackageManager, pkg: String): String {
        return try {
            val appInfo = pm.getApplicationInfo(pkg, 0)
            pm.getApplicationLabel(appInfo).toString().ifBlank { pkg }
        } catch (_: Exception) {
            pkg
        }
    }

    fun buildIngestJsonPayload(
        deviceId: String,
        snapshot: DailyWellbeingSnapshot,
        sentAtIso: String
    ): String {
        val root = JSONObject()
        root.put("date", snapshot.date)
        root.put("deviceId", deviceId)
        root.put("clientType", "android")
        root.put("platform", "android")
        root.put("totalScreenSeconds", snapshot.totalScreenSeconds)
        root.put("tzOffsetMinutes", java.util.TimeZone.getDefault().getOffset(System.currentTimeMillis()) / 60_000)
        root.put("sentAt", sentAtIso)

        val appsArr = JSONArray()
        for (app in snapshot.apps) {
            val obj = JSONObject()
            obj.put("appId", app.appId)
            obj.put("displayName", app.displayName)
            obj.put("seconds", app.seconds)
            obj.put("opens", app.opens)
            appsArr.put(obj)
        }
        root.put("apps", appsArr)

        val domArr = JSONArray()
        for (dom in snapshot.domains) {
            val obj = JSONObject()
            obj.put("domain", dom.domain)
            obj.put("seconds", 0L)
            obj.put("opens", dom.opens)
            obj.put("blockedAttempts", dom.blockedAttempts)
            domArr.put(obj)
        }
        root.put("domains", domArr)

        return root.toString()
    }
}
