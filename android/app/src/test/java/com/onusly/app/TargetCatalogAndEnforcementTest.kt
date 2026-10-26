package com.onusly.app

import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.TargetCatalog
import com.onusly.app.engine.BlockerEngine
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TargetCatalogAndEnforcementTest {

    private fun createCacheWithTarget(
        goalId: String = "goal_test",
        title: String = "Test Goal",
        status: String = "active",
        apps: List<String> = emptyList(),
        domains: List<String> = emptyList()
    ): CachedBlocklist {
        return CachedBlocklist(
            userId = "test_user",
            version = "v1",
            generatedAt = "2026-10-26T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = listOf(
                BlocklistTarget(
                    goalId = goalId,
                    goalTitle = title,
                    status = status,
                    apps = apps,
                    domains = domains
                )
            )
        )
    }

    // --- 1. TargetCatalog Canonical Mappings ---

    @Test
    fun testTargetCatalog_CanonicalMappings() {
        // Instagram
        assertEquals(listOf("com.instagram.android"), TargetCatalog.getAppsForDomain("instagram.com"))
        assertEquals(listOf("instagram.com"), TargetCatalog.getDomainsForApp("com.instagram.android"))

        // YouTube
        assertEquals(listOf("com.google.android.youtube"), TargetCatalog.getAppsForDomain("youtube.com"))
        assertEquals(listOf("com.google.android.youtube"), TargetCatalog.getAppsForDomain("youtu.be"))
        assertEquals(listOf("youtube.com", "youtu.be"), TargetCatalog.getDomainsForApp("com.google.android.youtube"))

        // YouTube Shorts (Isolated)
        assertEquals(listOf("com.google.android.youtube.shorts"), TargetCatalog.getAppsForDomain("shorts.youtube.com"))
        assertEquals(listOf("shorts.youtube.com"), TargetCatalog.getDomainsForApp("com.google.android.youtube.shorts"))

        // Twitter / X
        assertEquals(listOf("com.twitter.android"), TargetCatalog.getAppsForDomain("x.com"))
        assertEquals(listOf("com.twitter.android"), TargetCatalog.getAppsForDomain("twitter.com"))
        assertEquals(listOf("x.com", "twitter.com"), TargetCatalog.getDomainsForApp("com.twitter.android"))

        // Reddit
        assertEquals(listOf("com.reddit.frontpage"), TargetCatalog.getAppsForDomain("reddit.com"))
        assertEquals(listOf("reddit.com"), TargetCatalog.getDomainsForApp("com.reddit.frontpage"))

        // Netflix
        assertEquals(listOf("com.netflix.mediaclient"), TargetCatalog.getAppsForDomain("netflix.com"))
        assertEquals(listOf("netflix.com"), TargetCatalog.getDomainsForApp("com.netflix.mediaclient"))

        // Discord
        assertEquals(listOf("com.discord"), TargetCatalog.getAppsForDomain("discord.com"))
        assertEquals(listOf("discord.com"), TargetCatalog.getDomainsForApp("com.discord"))

        // Twitch
        assertEquals(listOf("tv.twitch.android.app"), TargetCatalog.getAppsForDomain("twitch.tv"))
        assertEquals(listOf("twitch.tv"), TargetCatalog.getDomainsForApp("tv.twitch.android.app"))

        // TikTok
        val tikTokApps = TargetCatalog.getAppsForDomain("tiktok.com")
        assertTrue(tikTokApps.contains("com.zhiliaoapp.musically"))
        assertTrue(tikTokApps.contains("com.ss.android.ugc.trill"))
        assertEquals(listOf("tiktok.com"), TargetCatalog.getDomainsForApp("com.zhiliaoapp.musically"))
        assertEquals(listOf("tiktok.com"), TargetCatalog.getDomainsForApp("com.ss.android.ugc.trill"))

        // Facebook
        val facebookApps = TargetCatalog.getAppsForDomain("facebook.com")
        assertTrue(facebookApps.contains("com.facebook.katana"))
        assertTrue(facebookApps.contains("com.facebook.orca"))
        assertEquals(listOf("facebook.com"), TargetCatalog.getDomainsForApp("com.facebook.katana"))
        assertEquals(listOf("facebook.com"), TargetCatalog.getDomainsForApp("com.facebook.orca"))

        // Snapchat
        assertEquals(listOf("com.snapchat.android"), TargetCatalog.getAppsForDomain("snapchat.com"))
        assertEquals(listOf("snapchat.com"), TargetCatalog.getDomainsForApp("com.snapchat.android"))

        // LinkedIn
        assertEquals(listOf("com.linkedin.android"), TargetCatalog.getAppsForDomain("linkedin.com"))
        assertEquals(listOf("linkedin.com"), TargetCatalog.getDomainsForApp("com.linkedin.android"))

        // Pinterest
        assertEquals(listOf("com.pinterest"), TargetCatalog.getAppsForDomain("pinterest.com"))
        assertEquals(listOf("pinterest.com"), TargetCatalog.getDomainsForApp("com.pinterest"))
    }

    // --- 2. YouTube Shorts Isolation ---

    @Test
    fun testYouTubeShorts_IsIsolatedFromFullYouTube() {
        // Shorts domain must NEVER return full YouTube app
        val shortsApps = TargetCatalog.getAppsForDomain("shorts.youtube.com")
        assertFalse("Shorts domain must NEVER map to com.google.android.youtube", shortsApps.contains("com.google.android.youtube"))
        assertTrue(shortsApps.contains("com.google.android.youtube.shorts"))

        // Full YouTube domain must NEVER return YouTube Shorts app
        val youtubeApps = TargetCatalog.getAppsForDomain("youtube.com")
        assertFalse("Full YouTube domain must NOT map to com.google.android.youtube.shorts", youtubeApps.contains("com.google.android.youtube.shorts"))
        assertTrue(youtubeApps.contains("com.google.android.youtube"))

        // Full YouTube app must NEVER return shorts.youtube.com
        val youtubeDomains = TargetCatalog.getDomainsForApp("com.google.android.youtube")
        assertFalse("Full YouTube app must NOT map to shorts.youtube.com", youtubeDomains.contains("shorts.youtube.com"))
        assertTrue(youtubeDomains.contains("youtube.com"))

        // YouTube Shorts app must return shorts.youtube.com
        val shortsDomains = TargetCatalog.getDomainsForApp("com.google.android.youtube.shorts")
        assertEquals(listOf("shorts.youtube.com"), shortsDomains)
    }

    @Test
    fun testShortsDomain_DoesNotBlockFullYouTubeApp() {
        val cache = createCacheWithTarget(
            title = "Block Shorts Only",
            domains = listOf("shorts.youtube.com"),
            apps = emptyList()
        )

        // The full YouTube app must NOT be blocked when only Shorts is targeted!
        val resultYoutube = BlockerEngine.evaluatePackage("com.google.android.youtube", cache)
        assertFalse("Full YouTube app must NOT be blocked by shorts.youtube.com domain target", resultYoutube.isBlocked)
        assertTrue(resultYoutube.matchingTargets.isEmpty())

        // The shorts package itself would match if targeted
        val resultShorts = BlockerEngine.evaluatePackage("com.google.android.youtube.shorts", cache)
        assertTrue(resultShorts.isBlocked)
        assertEquals(1, resultShorts.matchingTargets.size)
    }

    @Test
    fun testYouTubeFullDomain_DoesBlockFullYouTubeApp() {
        val cache = createCacheWithTarget(
            title = "Block Full YouTube",
            domains = listOf("youtube.com"),
            apps = emptyList()
        )

        val resultYoutube = BlockerEngine.evaluatePackage("com.google.android.youtube", cache)
        assertTrue("Full YouTube app MUST be blocked by youtube.com domain target", resultYoutube.isBlocked)
        assertEquals(1, resultYoutube.matchingTargets.size)
    }

    // --- 3. Domain-Only and App-Only Target Enforcement ---

    @Test
    fun testDomainOnlyTarget_BlocksMappedApp() {
        // Target synced from web or created with only domains
        val cache = createCacheWithTarget(
            title = "Block Instagram Web",
            domains = listOf("instagram.com"),
            apps = emptyList()
        )

        // Android MUST block com.instagram.android via domain association
        val resultBlocked = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("com.instagram.android must be blocked when target has domain instagram.com", resultBlocked.isBlocked)
        assertEquals(1, resultBlocked.matchingTargets.size)
        assertEquals("Block Instagram Web", resultBlocked.matchingTargets[0].goalTitle)

        // Unrelated app should not be blocked
        val resultUnrelated = BlockerEngine.evaluatePackage("com.spotify.music", cache)
        assertFalse(resultUnrelated.isBlocked)
        assertTrue(resultUnrelated.matchingTargets.isEmpty())
    }

    @Test
    fun testAppOnlyTarget_BlocksAppDirectly() {
        val cache = createCacheWithTarget(
            title = "Block Instagram App",
            domains = emptyList(),
            apps = listOf("com.instagram.android")
        )

        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("com.instagram.android must be blocked by app target", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
    }

    @Test
    fun testCustomUnmappedDomain_DoesNotBlockArbitraryApps() {
        val cache = createCacheWithTarget(
            title = "Block Custom Website",
            domains = listOf("example.com", "mycustomsite.org"),
            apps = emptyList()
        )

        // Custom domains not mapped to any app must not cause arbitrary app blocking
        assertFalse(BlockerEngine.evaluatePackage("com.instagram.android", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.google.android.youtube", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.twitter.android", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.reddit.frontpage", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.spotify.music", cache).isBlocked)
    }

    // --- 4. Case Insensitivity & Whitespace Handling ---

    @Test
    fun testCaseInsensitivityAndWhitespaceHandling() {
        // Domain with uppercase, whitespace, and full URL scheme/path
        val cache = createCacheWithTarget(
            title = "Uppercase & Url Domain",
            domains = listOf("  https://WWW.INSTAGRAM.COM/explore/  "),
            apps = emptyList()
        )

        // Should block case-varied and whitespace-padded package name
        val result = BlockerEngine.evaluatePackage("  COM.INSTAGRAM.ANDROID  ", cache)
        assertTrue("BlockerEngine must handle case-insensitivity and whitespace", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)

        // Test Twitter / X with mixed case
        val twitterCache = createCacheWithTarget(
            title = "Twitter Target",
            domains = listOf("  X.COM  "),
            apps = emptyList()
        )
        val twitterResult = BlockerEngine.evaluatePackage("com.twitter.android", twitterCache)
        assertTrue(twitterResult.isBlocked)

        // Test package lookup case-insensitivity
        assertEquals(listOf("instagram.com"), TargetCatalog.getDomainsForApp("  COM.INSTAGRAM.ANDROID  "))
        assertEquals(listOf("x.com", "twitter.com"), TargetCatalog.getDomainsForApp("  COM.TWITTER.ANDROID  "))
    }

    // --- 5. Goal Status Respect ---

    @Test
    fun testCompletedGoal_WithDomainTarget_DoesNotBlock() {
        val cache = createCacheWithTarget(
            title = "Completed Goal",
            status = "completed",
            domains = listOf("instagram.com"),
            apps = emptyList()
        )

        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertFalse("Completed goals must never block", result.isBlocked)
        assertTrue(result.matchingTargets.isEmpty())
    }

    @Test
    fun testProofSubmittedGoal_WithDomainTarget_Blocks() {
        val cache = createCacheWithTarget(
            title = "Pending Proof",
            status = "proof_submitted",
            domains = listOf("reddit.com"),
            apps = emptyList()
        )

        val result = BlockerEngine.evaluatePackage("com.reddit.frontpage", cache)
        assertTrue("proof_submitted goals with domain targets must block", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
    }
}
