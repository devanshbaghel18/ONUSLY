package com.onusly.app.data.model

data class CatalogEntry(
    val name: String,
    val packageNames: List<String>,
    val domains: List<String>
)

object TargetCatalog {

    val ENTRIES: List<CatalogEntry> = listOf(
        CatalogEntry(
            name = "Instagram",
            packageNames = listOf("com.instagram.android"),
            domains = listOf("instagram.com")
        ),
        CatalogEntry(
            name = "YouTube",
            packageNames = listOf("com.google.android.youtube"),
            domains = listOf("youtube.com", "youtu.be")
        ),
        CatalogEntry(
            name = "YouTube Shorts",
            packageNames = listOf("com.google.android.youtube.shorts"),
            domains = listOf("shorts.youtube.com")
        ),
        CatalogEntry(
            name = "Twitter / X",
            packageNames = listOf("com.twitter.android"),
            domains = listOf("x.com", "twitter.com")
        ),
        CatalogEntry(
            name = "Reddit",
            packageNames = listOf("com.reddit.frontpage"),
            domains = listOf("reddit.com")
        ),
        CatalogEntry(
            name = "Netflix",
            packageNames = listOf("com.netflix.mediaclient"),
            domains = listOf("netflix.com")
        ),
        CatalogEntry(
            name = "Discord",
            packageNames = listOf("com.discord"),
            domains = listOf("discord.com")
        ),
        CatalogEntry(
            name = "Twitch",
            packageNames = listOf("tv.twitch.android.app"),
            domains = listOf("twitch.tv")
        ),
        CatalogEntry(
            name = "TikTok",
            packageNames = listOf("com.zhiliaoapp.musically", "com.ss.android.ugc.trill"),
            domains = listOf("tiktok.com")
        ),
        CatalogEntry(
            name = "Facebook",
            packageNames = listOf("com.facebook.katana", "com.facebook.orca"),
            domains = listOf("facebook.com")
        ),
        CatalogEntry(
            name = "Snapchat",
            packageNames = listOf("com.snapchat.android"),
            domains = listOf("snapchat.com")
        ),
        CatalogEntry(
            name = "LinkedIn",
            packageNames = listOf("com.linkedin.android"),
            domains = listOf("linkedin.com")
        ),
        CatalogEntry(
            name = "Pinterest",
            packageNames = listOf("com.pinterest"),
            domains = listOf("pinterest.com")
        )
    )

    private val DOMAIN_TO_APPS: Map<String, List<String>> = mapOf(
        "instagram.com" to listOf("com.instagram.android"),
        "youtube.com" to listOf("com.google.android.youtube"),
        "youtu.be" to listOf("com.google.android.youtube"),
        "shorts.youtube.com" to listOf("com.google.android.youtube.shorts"),
        "x.com" to listOf("com.twitter.android"),
        "twitter.com" to listOf("com.twitter.android"),
        "reddit.com" to listOf("com.reddit.frontpage"),
        "netflix.com" to listOf("com.netflix.mediaclient"),
        "discord.com" to listOf("com.discord"),
        "twitch.tv" to listOf("tv.twitch.android.app"),
        "tiktok.com" to listOf("com.zhiliaoapp.musically", "com.ss.android.ugc.trill"),
        "facebook.com" to listOf("com.facebook.katana", "com.facebook.orca"),
        "snapchat.com" to listOf("com.snapchat.android"),
        "linkedin.com" to listOf("com.linkedin.android"),
        "pinterest.com" to listOf("com.pinterest")
    )

    private val APP_TO_DOMAINS: Map<String, List<String>> = mapOf(
        "com.instagram.android" to listOf("instagram.com"),
        "com.google.android.youtube" to listOf("youtube.com", "youtu.be"),
        "com.google.android.youtube.shorts" to listOf("shorts.youtube.com"),
        "com.twitter.android" to listOf("x.com", "twitter.com"),
        "com.reddit.frontpage" to listOf("reddit.com"),
        "com.netflix.mediaclient" to listOf("netflix.com"),
        "com.discord" to listOf("discord.com"),
        "tv.twitch.android.app" to listOf("twitch.tv"),
        "com.zhiliaoapp.musically" to listOf("tiktok.com"),
        "com.ss.android.ugc.trill" to listOf("tiktok.com"),
        "com.facebook.katana" to listOf("facebook.com"),
        "com.facebook.orca" to listOf("facebook.com"),
        "com.snapchat.android" to listOf("snapchat.com"),
        "com.linkedin.android" to listOf("linkedin.com"),
        "com.pinterest" to listOf("pinterest.com"),

        // Aliases for compatibility
        "instagram" to listOf("instagram.com"),
        "youtube" to listOf("youtube.com", "youtu.be"),
        "youtube_shorts" to listOf("shorts.youtube.com"),
        "youtube-shorts" to listOf("shorts.youtube.com"),
        "twitter" to listOf("x.com", "twitter.com"),
        "x" to listOf("x.com", "twitter.com"),
        "reddit" to listOf("reddit.com"),
        "netflix" to listOf("netflix.com"),
        "discord" to listOf("discord.com"),
        "twitch" to listOf("twitch.tv"),
        "tiktok" to listOf("tiktok.com"),
        "facebook" to listOf("facebook.com"),
        "snapchat" to listOf("snapchat.com"),
        "linkedin" to listOf("linkedin.com"),
        "pinterest" to listOf("pinterest.com")
    )

    /**
     * Normalizes a raw domain string: strips scheme, leading www., subpaths, query, port, etc.
     * Special-cases YouTube Shorts patterns so shorts paths never normalize to base youtube.com.
     */
    fun normalizeDomain(rawDomain: String?): String {
        if (rawDomain.isNullOrBlank()) return ""
        var d = rawDomain.trim().lowercase()

        // Strip scheme (e.g. https:// or http://)
        if (d.contains("://")) {
            d = d.substringAfter("://")
        }

        // Special handling for YouTube Shorts paths before stripping subpaths
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

        // Strip subpaths, query parameters, and fragments
        d = d.substringBefore('/').substringBefore('?').substringBefore('#')

        // Strip port if present
        d = d.substringBefore(':')

        // Strip trailing dots
        d = d.trimEnd('.')

        // Strip leading www.
        if (d.startsWith("www.")) {
            d = d.removePrefix("www.")
        }

        return d
    }

    /**
     * Returns mapped package names for a given domain.
     * Strips scheme/leading www/subpath if any; unmapped domains return empty list.
     */
    fun getAppsForDomain(domain: String): List<String> {
        val normalized = normalizeDomain(domain)
        if (normalized.isBlank()) return emptyList()
        return DOMAIN_TO_APPS[normalized] ?: emptyList()
    }

    /**
     * Returns mapped domains for a given Android package name or catalog ID.
     * Unmapped packages return empty list.
     */
    fun getDomainsForApp(packageName: String): List<String> {
        if (packageName.isBlank()) return emptyList()
        val normalizedPkg = packageName.trim().lowercase()
        return APP_TO_DOMAINS[normalizedPkg] ?: emptyList()
    }
}
