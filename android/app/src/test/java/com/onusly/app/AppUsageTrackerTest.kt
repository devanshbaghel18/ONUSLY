package com.onusly.app

import com.onusly.app.dns.DnsHitStore
import com.onusly.app.dns.DomainQueryHit
import com.onusly.app.wellbeing.AndroidAppUsage
import com.onusly.app.wellbeing.AppUsageTracker
import com.onusly.app.wellbeing.DailyWellbeingSnapshot
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class AppUsageTrackerTest {

    @Test
    fun testDnsHitStore_PersistsAndPrunesToSevenDays() {
        val prefs = FakeSharedPreferences()
        val store = DnsHitStore(prefs)

        for (day in 1..9) {
            val dateKey = "2026-11-0$day"
            store.saveDay(
                dateKey,
                listOf(
                    DomainQueryHit(
                        domain = "site$day.com",
                        count = day.toLong(),
                        opens = day.toLong(),
                        blockedAttempts = if (day % 2 == 0) 1L else 0L,
                        lastSeenMillis = 1000L * day,
                        lastVisitRecordedMillis = 1000L * day,
                        blocked = false
                    )
                )
            )
        }

        val allDays = store.loadAllDays()
        assertEquals(7, allDays.size)
        assertEquals(0, store.loadDay("2026-11-01").size)
        assertEquals(0, store.loadDay("2026-11-02").size)
        val day9 = store.loadDay("2026-11-09")
        assertEquals(1, day9.size)
        assertEquals("site9.com", day9[0].domain)
        assertEquals(9L, day9[0].opens)
    }

    @Test
    fun testBuildIngestJsonPayload_SerializesAppsAndDomainsCorrectly() {
        val snapshot = DailyWellbeingSnapshot(
            date = "2026-11-26",
            totalScreenSeconds = 1860L,
            apps = listOf(
                AndroidAppUsage(
                    appId = "com.instagram.android",
                    displayName = "Instagram",
                    seconds = 1200L,
                    opens = 14L
                ),
                AndroidAppUsage(
                    appId = "com.whatsapp",
                    displayName = "WhatsApp",
                    seconds = 660L,
                    opens = 22L
                )
            ),
            domains = listOf(
                DomainQueryHit(
                    domain = "reddit.com",
                    count = 6L,
                    opens = 3L,
                    blockedAttempts = 1L,
                    lastSeenMillis = 5000L,
                    lastVisitRecordedMillis = 5000L,
                    blocked = false
                )
            )
        )

        val raw = AppUsageTracker.buildIngestJsonPayload(
            deviceId = "android-dev-1",
            snapshot = snapshot,
            sentAtIso = "2026-11-26T12:00:00Z"
        )
        val parsed = JSONObject(raw)
        assertEquals("2026-11-26", parsed.getString("date"))
        assertEquals("android-dev-1", parsed.getString("deviceId"))
        assertEquals("android", parsed.getString("clientType"))
        assertEquals(1860L, parsed.getLong("totalScreenSeconds"))

        val apps = parsed.getJSONArray("apps")
        assertEquals(2, apps.length())
        assertEquals("Instagram", apps.getJSONObject(0).getString("displayName"))
        assertEquals(1200L, apps.getJSONObject(0).getLong("seconds"))
        assertEquals(14L, apps.getJSONObject(0).getLong("opens"))

        val domains = parsed.getJSONArray("domains")
        assertEquals(1, domains.length())
        assertEquals("reddit.com", domains.getJSONObject(0).getString("domain"))
        assertEquals(3L, domains.getJSONObject(0).getLong("opens"))
        assertEquals(1L, domains.getJSONObject(0).getLong("blockedAttempts"))
    }
}
