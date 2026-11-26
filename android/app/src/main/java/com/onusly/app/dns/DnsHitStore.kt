package com.onusly.app.dns

import android.content.Context
import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Persists daily Mobile DNS website visit counts and blocked attempts across VPN restarts and reboots.
 * Stores up to 7 recent days (hostname-only metrics, zero full URLs).
 */
class DnsHitStore(
    private val prefs: SharedPreferences,
    private val nowMillisProvider: () -> Long = { System.currentTimeMillis() }
) {
    companion object {
        private const val PREFS_NAME = "onusly_dns_hits"
        private const val KEY_DAYS_JSON = "daily_hits_json"
        private const val MAX_DAYS = 7

        fun create(context: Context): DnsHitStore {
            return DnsHitStore(context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE))
        }

        fun dateKeyFor(millis: Long): String {
            return SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date(millis))
        }
    }

    @Synchronized
    fun loadDay(dateKey: String = dateKeyFor(nowMillisProvider())): List<DomainQueryHit> {
        val root = loadRoot()
        val arr = root.optJSONArray(dateKey) ?: return emptyList()
        val list = mutableListOf<DomainQueryHit>()
        for (i in 0 until arr.length()) {
            val obj = arr.optJSONObject(i) ?: continue
            val dom = obj.optString("domain", "")
            if (dom.isBlank()) continue
            list.add(
                DomainQueryHit(
                    domain = dom,
                    count = obj.optLong("count", 0L),
                    opens = obj.optLong("opens", 0L),
                    blockedAttempts = obj.optLong("blockedAttempts", 0L),
                    lastSeenMillis = obj.optLong("lastSeenMillis", 0L),
                    lastVisitRecordedMillis = obj.optLong("lastVisitRecordedMillis", 0L),
                    blocked = obj.optBoolean("blocked", false)
                )
            )
        }
        return list
    }

    @Synchronized
    fun saveDay(dateKey: String, hits: List<DomainQueryHit>) {
        val root = loadRoot()
        val arr = JSONArray()
        for (h in hits) {
            val obj = JSONObject()
            obj.put("domain", h.domain)
            obj.put("count", h.count)
            obj.put("opens", h.opens)
            obj.put("blockedAttempts", h.blockedAttempts)
            obj.put("lastSeenMillis", h.lastSeenMillis)
            obj.put("lastVisitRecordedMillis", h.lastVisitRecordedMillis)
            obj.put("blocked", h.blocked)
            arr.put(obj)
        }
        root.put(dateKey, arr)
        pruneOldDays(root)
        prefs.edit().putString(KEY_DAYS_JSON, root.toString()).apply()
    }

    @Synchronized
    fun loadAllDays(): Map<String, List<DomainQueryHit>> {
        val root = loadRoot()
        val result = linkedMapOf<String, List<DomainQueryHit>>()
        val keys = root.keys()
        while (keys.hasNext()) {
            val dateKey = keys.next()
            result[dateKey] = loadDay(dateKey)
        }
        return result
    }

    private fun loadRoot(): JSONObject {
        val raw = prefs.getString(KEY_DAYS_JSON, null)
        if (raw.isNullOrBlank()) return JSONObject()
        return try {
            JSONObject(raw)
        } catch (_: Exception) {
            JSONObject()
        }
    }

    private fun pruneOldDays(root: JSONObject) {
        val keys = mutableListOf<String>()
        val it = root.keys()
        while (it.hasNext()) {
            keys.add(it.next())
        }
        if (keys.size <= MAX_DAYS) return
        keys.sort()
        val toRemove = keys.size - MAX_DAYS
        for (i in 0 until toRemove) {
            root.remove(keys[i])
        }
    }
}
