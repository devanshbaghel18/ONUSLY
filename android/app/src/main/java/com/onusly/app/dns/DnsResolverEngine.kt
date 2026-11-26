package com.onusly.app.dns

import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.engine.DomainMatcher
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference

data class DnsEngineStats(
    val queriesTotal: Long = 0L,
    val queriesBlocked: Long = 0L,
    val queriesForwarded: Long = 0L,
    val queriesCached: Long = 0L,
    val queriesFailed: Long = 0L,
    val lastBlockedDomain: String? = null,
    val lastBlockedAtMillis: Long = 0L,
    val blockedDomainsCount: Int = 0
)

data class DomainQueryHit(
    val domain: String,
    val count: Long,
    val opens: Long,
    val blockedAttempts: Long,
    val lastSeenMillis: Long,
    val lastVisitRecordedMillis: Long,
    val blocked: Boolean
)

/**
 * Coordinates DNS policy evaluation, response caching, upstream forwarding,
 * and aggregate query telemetry (including debounced domain open counts for Digital Wellbeing).
 */
class DnsResolverEngine(
    private val upstream: UpstreamDnsTransport,
    private val cache: DnsResponseCache = DnsResponseCache(),
    private val nowMillisProvider: () -> Long = { System.currentTimeMillis() },
    private val visitDebounceMillis: Long = 30_000L,
    private val onDomainBlocked: ((String) -> Unit)? = null
) {
    private val currentBlocklist = AtomicReference<CachedBlocklist?>(null)
    private val protectedHosts = AtomicReference<Set<String>>(emptySet())

    private val queriesTotal = AtomicLong(0L)
    private val queriesBlocked = AtomicLong(0L)
    private val queriesForwarded = AtomicLong(0L)
    private val queriesCached = AtomicLong(0L)
    private val queriesFailed = AtomicLong(0L)
    private val lastBlockedDomain = AtomicReference<String?>(null)
    private val lastBlockedAtMillis = AtomicLong(0L)

    @Volatile
    private var currentDayKey: String = DnsHitStore.dateKeyFor(nowMillisProvider())
    private val domainHits = ConcurrentHashMap<String, DomainQueryHit>()

    fun seedDayHits(dateKey: String, hits: List<DomainQueryHit>) {
        currentDayKey = dateKey
        domainHits.clear()
        for (h in hits) {
            domainHits[h.domain] = h
        }
    }

    fun getCurrentDayKey(): String = currentDayKey

    fun updatePolicy(blocklist: CachedBlocklist?, extraProtectedHosts: Set<String> = emptySet()) {
        currentBlocklist.set(blocklist)
        protectedHosts.set(extraProtectedHosts)
        val activeDomains = DomainMatcher.getActiveBlockedDomains(blocklist, extraProtectedHosts)
        if (activeDomains.isNotEmpty()) {
            cache.invalidateMatching(activeDomains)
        } else {
            // When goals unlock or clear, also clear negative/stale entries cleanly
            cache.clear()
        }
    }

    fun handleDnsQuery(rawQuery: ByteArray): ByteArray {
        queriesTotal.incrementAndGet()

        val question = DnsWireCodec.parseQuery(rawQuery)
        if (question == null) {
            queriesFailed.incrementAndGet()
            return DnsWireCodec.buildServFailResponse(rawQuery)
        }

        val normalizedDomain = DomainMatcher.normalizeDomain(question.qName)
        val extraProtected = protectedHosts.get()
        val evaluation = DomainMatcher.evaluateDomain(
            queryHostname = normalizedDomain,
            cachedBlocklist = currentBlocklist.get(),
            extraProtectedHosts = extraProtected
        )

        if (evaluation.isBlocked) {
            queriesBlocked.incrementAndGet()
            lastBlockedDomain.set(normalizedDomain)
            lastBlockedAtMillis.set(nowMillisProvider())
            recordHit(normalizedDomain, blocked = true)
            onDomainBlocked?.invoke(normalizedDomain)
            return DnsWireCodec.buildNxDomainResponse(rawQuery, question)
        }

        // Check response cache for allowed domain
        val cached = cache.get(normalizedDomain, question.qType, question.qClass)
        if (cached != null) {
            queriesCached.incrementAndGet()
            recordHit(normalizedDomain, blocked = false)
            return DnsWireCodec.rewriteTransactionId(cached, question.transactionId)
        }

        // Forward to upstream resolver
        val upstreamResp = upstream.exchange(rawQuery)
        if (upstreamResp == null) {
            queriesFailed.incrementAndGet()
            return DnsWireCodec.buildServFailResponse(rawQuery)
        }

        queriesForwarded.incrementAndGet()
        recordHit(normalizedDomain, blocked = false)

        val rCode = DnsWireCodec.extractRCode(upstreamResp)
        cache.put(normalizedDomain, question.qType, question.qClass, rCode, upstreamResp)

        return upstreamResp
    }

    private fun recordHit(domain: String, blocked: Boolean) {
        if (domain.isBlank()) return
        val now = nowMillisProvider()
        val todayKey = DnsHitStore.dateKeyFor(now)
        if (todayKey != currentDayKey) {
            synchronized(this) {
                if (todayKey != currentDayKey) {
                    currentDayKey = todayKey
                    domainHits.clear()
                }
            }
        }
        val analyticsDomain = domain.removePrefix("www.")
        domainHits.compute(analyticsDomain) { _, prev ->
            if (prev == null) {
                DomainQueryHit(
                    domain = analyticsDomain,
                    count = 1L,
                    opens = if (blocked) 0L else 1L,
                    blockedAttempts = if (blocked) 1L else 0L,
                    lastSeenMillis = now,
                    lastVisitRecordedMillis = now,
                    blocked = blocked
                )
            } else {
                val isNewOpen = !blocked && (now - prev.lastVisitRecordedMillis >= visitDebounceMillis)
                prev.copy(
                    count = prev.count + 1L,
                    opens = prev.opens + if (isNewOpen) 1L else 0L,
                    blockedAttempts = prev.blockedAttempts + if (blocked) 1L else 0L,
                    lastSeenMillis = now,
                    lastVisitRecordedMillis = if (isNewOpen) now else prev.lastVisitRecordedMillis,
                    blocked = blocked
                )
            }
        }
    }

    fun snapshotDailyDomainHits(): List<DomainQueryHit> {
        return domainHits.values.toList()
    }

    fun getStats(): DnsEngineStats {
        val activeDomainsCount = DomainMatcher.getActiveBlockedDomains(
            currentBlocklist.get(),
            protectedHosts.get()
        ).size

        return DnsEngineStats(
            queriesTotal = queriesTotal.get(),
            queriesBlocked = queriesBlocked.get(),
            queriesForwarded = queriesForwarded.get(),
            queriesCached = queriesCached.get(),
            queriesFailed = queriesFailed.get(),
            lastBlockedDomain = lastBlockedDomain.get(),
            lastBlockedAtMillis = lastBlockedAtMillis.get(),
            blockedDomainsCount = activeDomainsCount
        )
    }

    fun getRecentDomainHits(limit: Int = 50): List<DomainQueryHit> {
        return domainHits.values
            .sortedByDescending { it.lastSeenMillis }
            .take(limit)
    }
}
