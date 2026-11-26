package com.onusly.app.ui

import android.content.Context
import android.view.LayoutInflater
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.core.content.ContextCompat
import com.google.android.material.button.MaterialButton
import com.onusly.app.R
import com.onusly.app.dns.DnsHitStore
import com.onusly.app.dns.DnsShieldManager
import com.onusly.app.dns.DomainQueryHit
import com.onusly.app.wellbeing.AndroidAppUsage
import com.onusly.app.wellbeing.AppUsageTracker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Native Android dialog displaying Digital Wellbeing breakdown for Today, Yesterday, and Past 7 Days,
 * combining Android UsageStatsManager app screen time and Mobile DNS website opens/blocks.
 */
class WellbeingBreakdownDialog(private val context: Context) {

    fun show() {
        val view = LayoutInflater.from(context).inflate(R.layout.dialog_wellbeing_breakdown, null)
        val btnClose = view.findViewById<MaterialButton>(R.id.btnCloseWellbeingDialog)
        val btnToday = view.findViewById<MaterialButton>(R.id.btnRangeToday)
        val btnYesterday = view.findViewById<MaterialButton>(R.id.btnRangeYesterday)
        val btnWeek = view.findViewById<MaterialButton>(R.id.btnRangeWeek)
        val tvHeader = view.findViewById<TextView>(R.id.tvDialogWellbeingHeader)
        val tvWebsites = view.findViewById<TextView>(R.id.tvDialogWebsitesList)
        val tvApps = view.findViewById<TextView>(R.id.tvDialogAppsList)

        val dialogScope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
        val dialog = AlertDialog.Builder(context)
            .setView(view)
            .create()
        dialog.setOnDismissListener { dialogScope.cancel() }

        var renderJob: Job? = null

        fun renderRange(rangeKey: String) {
            val activeBg = ContextCompat.getColorStateList(context, R.color.primary)
            val inactiveBg = ContextCompat.getColorStateList(context, android.R.color.transparent)
            val activeText = ContextCompat.getColor(context, R.color.text_primary)
            val inactiveText = ContextCompat.getColor(context, R.color.text_secondary)

            btnToday.backgroundTintList = if (rangeKey == "today") activeBg else inactiveBg
            btnToday.setTextColor(if (rangeKey == "today") activeText else inactiveText)
            btnYesterday.backgroundTintList = if (rangeKey == "yesterday") activeBg else inactiveBg
            btnYesterday.setTextColor(if (rangeKey == "yesterday") activeText else inactiveText)
            btnWeek.backgroundTintList = if (rangeKey == "7d") activeBg else inactiveBg
            btnWeek.setTextColor(if (rangeKey == "7d") activeText else inactiveText)

            tvHeader.text = "Loading usage stats..."

            renderJob?.cancel()
            renderJob = dialogScope.launch {
                val (totalSec, totalOpens, totalBlocked, sortedSites, sortedApps, hasPerm) = withContext(Dispatchers.IO) {
                    val hitStore = DnsHitStore.create(context)
                    val dnsByDate = hitStore.loadAllDays().toMutableMap()
                    val todayKey = AppUsageTracker.getTodayDateKey()
                    if (DnsShieldManager.domainHits.value.isNotEmpty()) {
                        dnsByDate[todayKey] = DnsShieldManager.domainHits.value
                    }

                    val snapshots = when (rangeKey) {
                        "today" -> listOf(
                            AppUsageTracker.collectSnapshotForDaysAgo(context, 0, dnsByDate[todayKey].orEmpty())
                        )
                        "yesterday" -> {
                            val ySnap = AppUsageTracker.collectSnapshotForDaysAgo(context, 1)
                            listOf(ySnap.copy(domains = dnsByDate[ySnap.date].orEmpty()))
                        }
                        else -> AppUsageTracker.collectPastWeekSnapshots(context, dnsByDate, daysCount = 7)
                    }

                    val sumSec = snapshots.sumOf { it.totalScreenSeconds }
                    val appTotals = linkedMapOf<String, AndroidAppUsage>()
                    val domTotals = linkedMapOf<String, DomainQueryHit>()

                    for (snap in snapshots) {
                        for (a in snap.apps) {
                            val prev = appTotals[a.appId]
                            appTotals[a.appId] = if (prev == null) a else prev.copy(
                                seconds = prev.seconds + a.seconds,
                                opens = prev.opens + a.opens
                            )
                        }
                        for (d in snap.domains) {
                            val prev = domTotals[d.domain]
                            domTotals[d.domain] = if (prev == null) d else prev.copy(
                                count = prev.count + d.count,
                                opens = prev.opens + d.opens,
                                blockedAttempts = prev.blockedAttempts + d.blockedAttempts
                            )
                        }
                    }

                    val opens = domTotals.values.sumOf { it.opens }
                    val blocked = domTotals.values.sumOf { it.blockedAttempts }
                    val sites = domTotals.values.sortedWith(
                        compareByDescending<DomainQueryHit> { it.opens }.thenByDescending { it.blockedAttempts }
                    )
                    val apps = appTotals.values.sortedByDescending { it.seconds }
                    val perm = AppUsageTracker.hasUsageStatsPermission(context)
                    BreakdownResult(sumSec, opens, blocked, sites, apps, perm)
                }

                tvHeader.text = "Screen Time: ${formatSec(totalSec)} • Websites Opened: $totalOpens • Blocked: $totalBlocked"

                tvWebsites.text = if (sortedSites.isEmpty()) {
                    "No mobile DNS website hits recorded for this period."
                } else {
                    sortedSites.joinToString("\n") { s ->
                        val blockedNote = if (s.blockedAttempts > 0) " • ${s.blockedAttempts} blocked" else ""
                        "• ${s.domain}: ${s.opens} open${if (s.opens == 1L) "" else "s"}$blockedNote"
                    }
                }

                tvApps.text = if (sortedApps.isEmpty()) {
                    if (hasPerm) {
                        "No mobile app usage recorded for this period."
                    } else {
                        "Grant Usage Access permission on Home to display per-app screen time."
                    }
                } else {
                    sortedApps.take(25).joinToString("\n") { a ->
                        "• ${a.displayName}: ${formatSec(a.seconds)} (${a.opens} open${if (a.opens == 1L) "" else "s"})"
                    }
                }
            }
        }

        btnToday.setOnClickListener { renderRange("today") }
        btnYesterday.setOnClickListener { renderRange("yesterday") }
        btnWeek.setOnClickListener { renderRange("7d") }
        btnClose.setOnClickListener { dialog.dismiss() }

        renderRange("today")
        dialog.show()
    }

    private data class BreakdownResult(
        val totalSec: Long,
        val totalOpens: Long,
        val totalBlocked: Long,
        val sortedSites: List<DomainQueryHit>,
        val sortedApps: List<AndroidAppUsage>,
        val hasPerm: Boolean
    )

    private fun formatSec(sec: Long): String {
        val totalMin = (sec / 60L).coerceAtLeast(0L)
        val hrs = totalMin / 60L
        val mins = totalMin % 60L
        return when {
            sec in 1..59 -> "${sec}s"
            hrs > 0 -> "${hrs}h ${mins}m"
            else -> "${mins}m"
        }
    }
}
