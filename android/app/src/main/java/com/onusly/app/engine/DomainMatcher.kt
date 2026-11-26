package com.onusly.app.engine

import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.TargetCatalog
import java.net.IDN
import java.net.URI

data class DomainEvaluationResult(
    val isBlocked: Boolean,
    val normalizedDomain: String,
    val matchingTargets: List<BlocklistTarget>
)

/**
 * Pure-Kotlin DNS domain normalization, protected-domain allowlisting, and blocklist evaluation.
 * Mirrors the desktop DNS enforcement matcher to guarantee cross-platform parity.
 */
object DomainMatcher {

    /**
     * Canonicalizes a domain/hostname or URL into lowercase ASCII Punycode suitable for DNS matching.
     */
    fun normalizeDomain(rawDomain: String?): String {
        if (rawDomain.isNullOrBlank()) return ""
        var d = rawDomain.trim().lowercase()

        // Strip URI scheme if inadvertently present
        val schemeIdx = d.indexOf("://")
        if (schemeIdx != -1) {
            d = d.substring(schemeIdx + 3)
        }

        // Preserve YouTube Shorts specificity before stripping URL paths
        if (d == "shorts.youtube.com" ||
            d == "www.shorts.youtube.com" ||
            d.startsWith("shorts.youtube.com/") ||
            d.startsWith("www.shorts.youtube.com/") ||
            d == "youtube.com/shorts" ||
            d.startsWith("youtube.com/shorts/") ||
            d == "www.youtube.com/shorts" ||
            d.startsWith("www.youtube.com/shorts/") ||
            d == "m.youtube.com/shorts" ||
            d.startsWith("m.youtube.com/shorts/")
        ) {
            return "shorts.youtube.com"
        }

        // Strip path, query, fragment
        d = d.substringBefore('/').substringBefore('?').substringBefore('#')

        // Strip port unless bracketed IPv6 literal
        if (d.contains(':') && !d.startsWith("[")) {
            d = d.substringBefore(':')
        }

        // Strip surrounding dots
        d = d.trim('.')
        if (d.isEmpty()) return ""

        // Punycode / IDNA ASCII conversion for DNS wire compatibility
        return try {
            val ascii = IDN.toASCII(d, IDN.ALLOW_UNASSIGNED)
            if (ascii.isNotBlank()) ascii.lowercase().trim('.') else d
        } catch (_: Exception) {
            d
        }
    }

    /**
     * Returns true if [queryHostname] matches [targetDomain] via exact equality,
     * subdomain suffix (e.g. "m.instagram.com" -> "instagram.com"), or "www." parity.
     * Prevents unsafe substring matches (e.g. "notexample.com" never matches "example.com").
     */
    fun isDomainMatch(queryHostname: String?, targetDomain: String?): Boolean {
        val q = normalizeDomain(queryHostname)
        val t = normalizeDomain(targetDomain)
        if (q.isEmpty() || t.isEmpty()) return false

        if (q == t) return true
        if (q.endsWith(".$t")) return true

        val cleanQ = q.removePrefix("www.")
        val cleanT = t.removePrefix("www.")
        if (cleanQ == cleanT) return true
        if (cleanQ.endsWith(".$cleanT")) return true

        return false
    }

    /**
     * Domains that must NEVER be blocked so ONUSLY core services, loopback, and local LAN keep working.
     */
    fun isProtectedDomain(domain: String?, extraProtectedHosts: Set<String> = emptySet()): Boolean {
        val d = normalizeDomain(domain)
        if (d.isEmpty()) return true

        if (d == "localhost" || d.endsWith(".localhost")) return true
        if (d == "127.0.0.1" || d == "::1" || d == "0.0.0.0") return true

        if (d.endsWith(".local") ||
            d.endsWith(".internal") ||
            d.endsWith(".home.arpa") ||
            d.endsWith(".lan") ||
            d.endsWith(".in-addr.arpa") ||
            d.endsWith(".ip6.arpa")
        ) {
            return true
        }

        if (d == "onusly.com" || d.endsWith(".onusly.com") ||
            d == "onusly.app" || d.endsWith(".onusly.app") ||
            d == "onusly.duckdns.org"
        ) {
            return true
        }

        for (extra in extraProtectedHosts) {
            val normExtra = normalizeDomain(extra)
            if (normExtra.isNotEmpty() && isDomainMatch(d, normExtra)) {
                return true
            }
        }

        return false
    }

    /**
     * Extracts the normalized hostname from a configured API base URL (e.g. "https://api.onusly.com:8081" -> "api.onusly.com").
     */
    fun extractHostFromUrl(baseUrl: String?): String {
        if (baseUrl.isNullOrBlank()) return ""
        return try {
            val uri = URI(baseUrl.trim())
            normalizeDomain(uri.host)
        } catch (_: Exception) {
            ""
        }
    }

    /**
     * Evaluates a DNS query hostname against the user's cached blocklist.
     * Only active, proof_submitted, and rejected goals enforce restrictions; completed goals never block.
     */
    fun evaluateDomain(
        queryHostname: String?,
        cachedBlocklist: CachedBlocklist?,
        extraProtectedHosts: Set<String> = emptySet()
    ): DomainEvaluationResult {
        val cleanQuery = normalizeDomain(queryHostname)
        if (cleanQuery.isEmpty() || isProtectedDomain(cleanQuery, extraProtectedHosts)) {
            return DomainEvaluationResult(
                isBlocked = false,
                normalizedDomain = cleanQuery,
                matchingTargets = emptyList()
            )
        }

        if (cachedBlocklist == null || cachedBlocklist.targets.isEmpty()) {
            return DomainEvaluationResult(
                isBlocked = false,
                normalizedDomain = cleanQuery,
                matchingTargets = emptyList()
            )
        }

        val matchingTargets = mutableListOf<BlocklistTarget>()

        for (target in cachedBlocklist.targets) {
            if (!BlockerEngine.isEnforcingStatus(target.status)) {
                continue
            }

            val blockedInTarget = target.domains.any { domainEntry ->
                val cleanTarget = normalizeDomain(domainEntry)
                cleanTarget.isNotEmpty() &&
                    !isProtectedDomain(cleanTarget, extraProtectedHosts) &&
                    isDomainMatch(cleanQuery, cleanTarget)
            }

            if (blockedInTarget) {
                matchingTargets.add(target)
            }
        }

        return DomainEvaluationResult(
            isBlocked = matchingTargets.isNotEmpty(),
            normalizedDomain = cleanQuery,
            matchingTargets = matchingTargets
        )
    }

    /**
     * Returns a sorted list of distinct, normalized blocked domains across all currently enforcing goals.
     */
    fun getActiveBlockedDomains(
        cachedBlocklist: CachedBlocklist?,
        extraProtectedHosts: Set<String> = emptySet()
    ): List<String> {
        if (cachedBlocklist == null || cachedBlocklist.targets.isEmpty()) {
            return emptyList()
        }
        val result = linkedSetOf<String>()
        for (target in cachedBlocklist.targets) {
            if (!BlockerEngine.isEnforcingStatus(target.status)) continue
            for (raw in target.domains) {
                val clean = normalizeDomain(raw)
                if (clean.isNotEmpty() && !isProtectedDomain(clean, extraProtectedHosts)) {
                    result.add(clean)
                }
            }
            // Fallback: if a legacy target only specified an app alias/package with no domains, map via TargetCatalog
            if (target.domains.isEmpty()) {
                for (app in target.apps) {
                    for (mappedDomain in TargetCatalog.getDomainsForApp(app)) {
                        val clean = normalizeDomain(mappedDomain)
                        if (clean.isNotEmpty() && !isProtectedDomain(clean, extraProtectedHosts)) {
                            result.add(clean)
                        }
                    }
                }
            }
        }
        return result.toList().sorted()
    }
}
