package com.onusly.app

import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.engine.BlockerEngine
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BlockerEngineTest {

    @Test
    fun testNullOrEmptyCache_NeverBlocks() {
        val resNull = BlockerEngine.evaluatePackage("com.instagram.android", null)
        assertFalse(resNull.isBlocked)
        assertTrue(resNull.matchingTargets.isEmpty())

        val emptyCache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = emptyList()
        )
        val resEmpty = BlockerEngine.evaluatePackage("com.instagram.android", emptyCache)
        assertFalse(resEmpty.isBlocked)
        assertTrue(resEmpty.matchingTargets.isEmpty())
    }

    @Test
    fun testNullOrBlankPackage_NeverBlocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Goal 1", "active", listOf("com.instagram.android"), emptyList())
            )
        )

        assertFalse(BlockerEngine.evaluatePackage(null, cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("   ", cache).isBlocked)
    }

    @Test
    fun testActiveGoal_BlocksMatchedApp() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Focus Goal", "active", listOf("com.instagram.android"), emptyList())
            )
        )

        val resultBlocked = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue(resultBlocked.isBlocked)
        assertEquals(1, resultBlocked.matchingTargets.size)
        assertEquals("Focus Goal", resultBlocked.matchingTargets[0].goalTitle)

        // Non-blocked app
        val resultAllowed = BlockerEngine.evaluatePackage("com.spotify.music", cache)
        assertFalse(resultAllowed.isBlocked)
        assertTrue(resultAllowed.matchingTargets.isEmpty())
    }

    @Test
    fun testProofSubmittedGoal_BlocksMatchedApp() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Waiting Review", "proof_submitted", listOf("com.twitter.android"), emptyList())
            )
        )

        val result = BlockerEngine.evaluatePackage("com.twitter.android", cache)
        assertTrue(result.isBlocked)
        assertEquals("proof_submitted", result.matchingTargets[0].status)
    }

    @Test
    fun testCompletedGoal_DoesNotBlock() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Done Goal", "completed", listOf("com.instagram.android"), emptyList())
            )
        )

        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertFalse("Completed goal must not block", result.isBlocked)
        assertTrue(result.matchingTargets.isEmpty())
    }

    @Test
    fun testMultipleGoals_TargetingSameApp() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Morning Focus", "active", listOf("com.instagram.android"), emptyList()),
                BlocklistTarget("g2", "Evening Study", "proof_submitted", listOf("com.instagram.android", "com.reddit.frontpage"), emptyList())
            )
        )

        val instagramResult = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue(instagramResult.isBlocked)
        assertEquals(2, instagramResult.matchingTargets.size)
        assertEquals("Morning Focus", instagramResult.matchingTargets[0].goalTitle)
        assertEquals("Evening Study", instagramResult.matchingTargets[1].goalTitle)

        val redditResult = BlockerEngine.evaluatePackage("com.reddit.frontpage", cache)
        assertTrue(redditResult.isBlocked)
        assertEquals(1, redditResult.matchingTargets.size)
    }

    @Test
    fun testAlwaysAllowedPackages_NeverBlocked() {
        // Even if explicitly added to a target
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Rogue Goal", "active", listOf("com.onusly.app", "com.android.settings", "com.android.systemui"), emptyList())
            )
        )

        assertFalse(BlockerEngine.evaluatePackage("com.onusly.app", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.android.settings", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.android.systemui", cache).isBlocked)
        assertFalse(BlockerEngine.evaluatePackage("com.google.android.apps.nexuslauncher", cache).isBlocked)
    }

    @Test
    fun testCriticalSystemAppsAndLaunchers_NeverBlockedEvenIfInBlocklist() {
        val criticalApps = listOf(
            // Google / AOSP
            "com.android.phone",
            "com.android.incallui",
            "com.android.server.telecom",
            "com.google.android.dialer",
            "com.google.android.emergency",
            "com.google.android.apps.safetyhub",
            "com.google.android.apps.nexuslauncher",

            // Samsung
            "com.samsung.android.dialer",
            "com.samsung.android.incallui",
            "com.samsung.android.emergency",
            "com.sec.android.app.launcher",

            // Xiaomi
            "com.miui.home",
            "com.miui.incallui",
            "com.miui.emergency",

            // Oppo / ColorOS
            "com.oppo.launcher",
            "com.coloros.dialer",
            "com.oppo.emergency",

            // OnePlus
            "com.oneplus.launcher",
            "com.oneplus.dialer",
            "com.oneplus.emergency",

            // Vivo
            "com.vivo.launcher",
            "com.vivo.dialer",
            "com.vivo.emergency",

            // Huawei
            "com.huawei.android.launcher",
            "com.huawei.android.incallui",
            "com.huawei.emergency",

            // Motorola
            "com.motorola.launcher",
            "com.motorola.incallui",
            "com.motorola.dialer",
            "com.motorola.emergency"
        )

        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Rogue Block", "active", criticalApps, emptyList())
            )
        )

        for (pkg in criticalApps) {
            val result = BlockerEngine.evaluatePackage(pkg, cache)
            assertFalse("Critical app '$pkg' must never be blocked", result.isBlocked)
            assertTrue("Matching targets must be empty for critical app '$pkg'", result.matchingTargets.isEmpty())
        }
    }

    @Test
    fun testThirdPartyAppsContainingEmergencyOrTelecom_CanBeBlocked() {
        val thirdPartyApps = listOf(
            "com.emergency.response.simulator",
            "com.telecom.customerapp",
            "com.telecom.billing",
            "com.thirdparty.emergencyapp",
            "com.random.settingswidget"
        )

        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Block Third Party Apps", "active", thirdPartyApps, emptyList())
            )
        )

        for (pkg in thirdPartyApps) {
            val result = BlockerEngine.evaluatePackage(pkg, cache)
            assertTrue("Third party app '$pkg' must be blockable", result.isBlocked)
            assertEquals("Matching targets must match for third party app '$pkg'", 1, result.matchingTargets.size)
        }
    }

    @Test
    fun testCaseInsensitiveMatching_TargetBlocksPackage() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Focus Goal", "active", listOf("COM.INSTAGRAM.ANDROID"), emptyList())
            )
        )

        val resultBlocked = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Upper-case target COM.INSTAGRAM.ANDROID must block lower-case package", resultBlocked.isBlocked)
        assertEquals(1, resultBlocked.matchingTargets.size)
        assertEquals("Focus Goal", resultBlocked.matchingTargets[0].goalTitle)

        // Lower-case target with upper-case package
        val cacheLower = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g2", "Focus Goal 2", "active", listOf("com.instagram.android"), emptyList())
            )
        )

        val resultUpper = BlockerEngine.evaluatePackage("COM.INSTAGRAM.ANDROID", cacheLower)
        assertTrue("Lower-case target must block upper-case package COM.INSTAGRAM.ANDROID", resultUpper.isBlocked)
        assertEquals(1, resultUpper.matchingTargets.size)
        assertEquals("Focus Goal 2", resultUpper.matchingTargets[0].goalTitle)
    }

    // --- P0 Client Enforcement Invariant Repair Tests ---

    @Test
    fun testActiveGoal_WithMatchingDomain_Blocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Active Domain Goal", "active", emptyList(), listOf("instagram.com"))
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Active goal with matching domain must block", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
        assertEquals("active", result.matchingTargets[0].status)
    }

    @Test
    fun testProofSubmittedGoal_WithMatchingDomain_Blocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Proof Submitted Domain Goal", "proof_submitted", emptyList(), listOf("instagram.com"))
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Proof-submitted goal with matching domain must block", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
        assertEquals("proof_submitted", result.matchingTargets[0].status)
    }

    @Test
    fun testRejectedGoal_WithMatchingDomain_Blocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Rejected Domain Goal", "rejected", emptyList(), listOf("instagram.com"))
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Rejected goal with matching domain must block", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
        assertEquals("rejected", result.matchingTargets[0].status)
    }

    @Test
    fun testCompletedGoal_WithMatchingDomain_Allows() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Completed Domain Goal", "completed", emptyList(), listOf("instagram.com"))
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertFalse("Completed goal with matching domain must not block", result.isBlocked)
        assertTrue(result.matchingTargets.isEmpty())
    }

    @Test
    fun testRejectedGoal_WithMatchingApplication_Blocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Rejected App Goal", "rejected", listOf("com.twitter.android"), emptyList())
            )
        )
        val result = BlockerEngine.evaluatePackage("com.twitter.android", cache)
        assertTrue("Rejected goal with matching application must block", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
        assertEquals("rejected", result.matchingTargets[0].status)
    }

    @Test
    fun testRejectedGoal_WithNoTargets_DoesNotBlock() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Rejected Empty Goal", "rejected", emptyList(), emptyList())
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertFalse("Rejected goal with no targets must not block", result.isBlocked)
        assertTrue(result.matchingTargets.isEmpty())
    }

    @Test
    fun testTwoMatchingGoals_ActiveAndRejected_BlocksBoth() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Active Goal", "active", listOf("com.instagram.android"), emptyList()),
                BlocklistTarget("g2", "Rejected Goal", "rejected", listOf("com.instagram.android"), emptyList())
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Multiple matching goals (active + rejected) must block", result.isBlocked)
        assertEquals(2, result.matchingTargets.size)
        val statuses = result.matchingTargets.map { it.status }
        assertTrue(statuses.contains("active"))
        assertTrue(statuses.contains("rejected"))
    }

    @Test
    fun testTwoMatchingGoals_CompletedAndRejected_Blocks() {
        val cache = CachedBlocklist(
            userId = "u1",
            version = "v1-1",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Completed Goal", "completed", listOf("com.instagram.android"), emptyList()),
                BlocklistTarget("g2", "Rejected Goal", "rejected", listOf("com.instagram.android"), emptyList())
            )
        )
        val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("Two matching goals (completed + rejected) must block on rejected target", result.isBlocked)
        assertEquals(1, result.matchingTargets.size)
        assertEquals("rejected", result.matchingTargets[0].status)
    }

    @Test
    fun testUnknownStatus_PreservesSafeHandling() {
        val unknownStatuses = listOf("unknown", "pending", "paused", "archived", "deleted", "", "   ")
        for (st in unknownStatuses) {
            assertFalse("Status '$st' must not be enforcing", BlockerEngine.isEnforcingStatus(st))
            val cache = CachedBlocklist(
                userId = "u1",
                version = "v1-1",
                generatedAt = "2026-10-08T12:00:00Z",
                lastSyncTimestamp = 1000L,
                targets = listOf(
                    BlocklistTarget("g1", "Unknown Status Goal", st, listOf("com.instagram.android"), emptyList())
                )
            )
            val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
            assertFalse("Unknown status '$st' must never block", result.isBlocked)
            assertTrue(result.matchingTargets.isEmpty())
        }
        assertFalse("Null status must not be enforcing", BlockerEngine.isEnforcingStatus(null))
    }

    @Test
    fun testWhitespaceAndCaseNormalization_EnforcesCorrectly() {
        val variations = listOf(" REJECTED ", "Rejected", "rejected ", "  proof_submitted  ", "PROOF_SUBMITTED", " Active ")
        for (st in variations) {
            assertTrue("Status '$st' must be recognized as enforcing", BlockerEngine.isEnforcingStatus(st))
            val cache = CachedBlocklist(
                userId = "u1",
                version = "v1-1",
                generatedAt = "2026-10-08T12:00:00Z",
                lastSyncTimestamp = 1000L,
                targets = listOf(
                    BlocklistTarget("g_var", "Normalized Goal", st, listOf("com.instagram.android"), emptyList())
                )
            )
            val result = BlockerEngine.evaluatePackage("com.instagram.android", cache)
            assertTrue("Status '$st' must enforce blocking", result.isBlocked)
            assertEquals(1, result.matchingTargets.size)
        }
    }
}
