package com.onusly.app.dns

import com.onusly.app.engine.DomainMatcher

/**
 * Thread-safe bounded LRU cache for unblocked DNS responses.
 * Automatically invalidated whenever enforcement blocklists change.
 */
class DnsResponseCache(
    private val maxEntries: Int = 512,
    private val positiveTtlMillis: Long = 60_000L,
    private val negativeTtlMillis: Long = 10_000L,
    private val nowMillisProvider: () -> Long = { System.currentTimeMillis() }
) {
    private data class CacheEntry(
        val domain: String,
        val payload: ByteArray,
        val expiresAtMillis: Long
    )

    private val entries = object : LinkedHashMap<String, CacheEntry>(64, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, CacheEntry>?): Boolean {
            return size > maxEntries
        }
    }

    @Synchronized
    fun get(normalizedDomain: String, qType: Int, qClass: Int): ByteArray? {
        if (normalizedDomain.isBlank()) return null
        val key = cacheKey(normalizedDomain, qType, qClass)
        val entry = entries[key] ?: return null
        if (nowMillisProvider() > entry.expiresAtMillis) {
            entries.remove(key)
            return null
        }
        return entry.payload.copyOf()
    }

    @Synchronized
    fun put(normalizedDomain: String, qType: Int, qClass: Int, rCode: Int, responseBytes: ByteArray) {
        if (normalizedDomain.isBlank() || responseBytes.isEmpty()) return
        // Never cache server failure / refused responses
        if (rCode != DnsWireCodec.RCODE_NOERROR && rCode != DnsWireCodec.RCODE_NXDOMAIN) {
            return
        }
        val ttl = if (rCode == DnsWireCodec.RCODE_NOERROR) positiveTtlMillis else negativeTtlMillis
        val key = cacheKey(normalizedDomain, qType, qClass)
        entries[key] = CacheEntry(
            domain = normalizedDomain,
            payload = responseBytes.copyOf(),
            expiresAtMillis = nowMillisProvider() + ttl
        )
    }

    /**
     * Invalidates any cached entries matching newly blocked target domains (including subdomains).
     */
    @Synchronized
    fun invalidateMatching(blockedDomains: Collection<String>) {
        if (blockedDomains.isEmpty()) return
        val iter = entries.entries.iterator()
        while (iter.hasNext()) {
            val cachedDomain = iter.next().value.domain
            if (blockedDomains.any { target -> DomainMatcher.isDomainMatch(cachedDomain, target) }) {
                iter.remove()
            }
        }
    }

    @Synchronized
    fun clear() {
        entries.clear()
    }

    @Synchronized
    fun size(): Int = entries.size

    private fun cacheKey(domain: String, qType: Int, qClass: Int): String = "$domain:$qType:$qClass"
}
