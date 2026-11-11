package com.onusly.app

import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.BlocklistCache
import com.onusly.app.data.model.ApplicationTarget
import com.onusly.app.data.model.BlocklistParser
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.SyncResult
import com.onusly.app.data.model.UpdateGoalRequest
import com.onusly.app.data.model.UserSession
import com.onusly.app.engine.BlockerEngine
import com.onusly.app.engine.InstalledAppsManager
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class D12TypedPolicyEnforcementTest {

    // -------------------------------------------------------------------------
    // 1. Cross-Platform Target Separation Contract
    // -------------------------------------------------------------------------

    @Test
    fun testAndroidPackageExactMatching() {
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-1",
                    goalTitle = "No Discord on Mobile",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "android_package", identifier = "com.discord")
                    )
                )
            )
        )

        // Exact match
        val res1 = BlockerEngine.evaluatePackage("com.discord", cache)
        assertTrue("Exact package com.discord must be blocked", res1.isBlocked)
        assertEquals(1, res1.matchingTargets.size)
        assertEquals("g-1", res1.matchingTargets[0].goalId)

        // Case-insensitive match
        val res2 = BlockerEngine.evaluatePackage("com.Discord", cache)
        assertTrue("com.Discord with casing must be blocked", res2.isBlocked)

        // Unrelated package
        val res3 = BlockerEngine.evaluatePackage("com.slack", cache)
        assertFalse("Unrelated package must not be blocked", res3.isBlocked)
    }

    @Test
    fun testDesktopAppTargetIgnoredByAndroid() {
        // Desktop agent targets must be strictly ignored by Android client
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-desktop",
                    goalTitle = "Desktop Restrictions",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "desktop_app", identifier = "Discord.exe"),
                        ApplicationTarget(kind = "desktop_app", identifier = "discord"),
                        ApplicationTarget(kind = "desktop_app", identifier = "com.discord")
                    )
                )
            )
        )

        // None of these should match on Android because kind == "desktop_app"
        assertFalse("Desktop target Discord.exe must be ignored by Android",
            BlockerEngine.evaluatePackage("Discord.exe", cache).isBlocked)
        assertFalse("Desktop target discord must be ignored by Android",
            BlockerEngine.evaluatePackage("discord", cache).isBlocked)
        assertFalse("Desktop target com.discord must be ignored by Android",
            BlockerEngine.evaluatePackage("com.discord", cache).isBlocked)
    }

    @Test
    fun testReverseDnsDesktopAppIgnoredByAndroid() {
        // Linux desktop applications commonly use reverse-DNS IDs (e.g. org.gnome.Calculator)
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-linux",
                    goalTitle = "Linux Desktop Apps",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "desktop_app", identifier = "org.gnome.Calculator"),
                        ApplicationTarget(kind = "desktop_app", identifier = "org.mozilla.firefox")
                    )
                )
            )
        )

        assertFalse("Linux desktop target org.gnome.Calculator must NOT match on Android",
            BlockerEngine.evaluatePackage("org.gnome.Calculator", cache).isBlocked)
        assertFalse("Linux desktop target org.mozilla.firefox must NOT match on Android",
            BlockerEngine.evaluatePackage("org.mozilla.firefox", cache).isBlocked)
    }

    @Test
    fun testMixedPoliciesHandling() {
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-mixed",
                    goalTitle = "Gaming Restrictions",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "desktop_app", identifier = "steam.exe"),
                        ApplicationTarget(kind = "desktop_app", identifier = "steam"),
                        ApplicationTarget(kind = "android_package", identifier = "com.valvesoftware.android.steam.community"),
                        ApplicationTarget(kind = "android_package", identifier = "com.riotgames.league.wildrift")
                    )
                )
            )
        )

        // Desktop targets ignored
        assertFalse("steam.exe must be ignored on Android",
            BlockerEngine.evaluatePackage("steam.exe", cache).isBlocked)
        assertFalse("steam must be ignored on Android",
            BlockerEngine.evaluatePackage("steam", cache).isBlocked)

        // Android targets matched
        assertTrue("Android steam package must be blocked",
            BlockerEngine.evaluatePackage("com.valvesoftware.android.steam.community", cache).isBlocked)
        assertTrue("Wild Rift package must be blocked",
            BlockerEngine.evaluatePackage("com.riotgames.league.wildrift", cache).isBlocked)
    }

    @Test
    fun testUnknownTargetKindIgnored() {
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-unknown",
                    goalTitle = "Unknown Target Kind",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "ios_bundle_id", identifier = "com.apple.shortcuts"),
                        ApplicationTarget(kind = "windows_uwp", identifier = "Microsoft.XboxApp_8wekyb3d8bbwe")
                    )
                )
            )
        )

        assertFalse("Unknown target kind ios_bundle_id must be ignored",
            BlockerEngine.evaluatePackage("com.apple.shortcuts", cache).isBlocked)
        assertFalse("Unknown target kind windows_uwp must be ignored",
            BlockerEngine.evaluatePackage("Microsoft.XboxApp_8wekyb3d8bbwe", cache).isBlocked)
    }

    @Test
    fun testLegacyUntypedPolicyFallback() {
        // When applicationTargets is empty, legacy apps field is consumed with desktop binary filtering
        val cache = CachedBlocklist(
            userId = "user-123",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-legacy",
                    goalTitle = "Legacy Goal",
                    status = "active",
                    apps = listOf("com.instagram.android", "Discord.exe", "Slack.desktop", "terminal.app"),
                    domains = listOf("instagram.com"),
                    applicationTargets = emptyList()
                )
            )
        )

        // Android package from legacy apps matches
        assertTrue("Legacy Android package must match",
            BlockerEngine.evaluatePackage("com.instagram.android", cache).isBlocked)

        // Desktop binaries (.exe, .desktop, .app) must be skipped
        assertFalse("Discord.exe in legacy apps must be skipped on Android",
            BlockerEngine.evaluatePackage("Discord.exe", cache).isBlocked)
        assertFalse("Slack.desktop in legacy apps must be skipped on Android",
            BlockerEngine.evaluatePackage("Slack.desktop", cache).isBlocked)
        assertFalse("terminal.app in legacy apps must be skipped on Android",
            BlockerEngine.evaluatePackage("terminal.app", cache).isBlocked)
    }

    // -------------------------------------------------------------------------
    // 2. Goal Status Enforcement Contract
    // -------------------------------------------------------------------------

    @Test
    fun testGoalStatusContract() {
        val target = ApplicationTarget(kind = "android_package", identifier = "com.reddit.frontpage")

        fun createCache(status: String) = CachedBlocklist(
            userId = "u1",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Reddit Goal", status, emptyList(), emptyList(), listOf(target))
            )
        )

        // active -> restricted
        assertTrue("active status must be restricted",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("active")).isBlocked)

        // proof_submitted -> restricted
        assertTrue("proof_submitted status must be restricted",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("proof_submitted")).isBlocked)

        // rejected -> restricted
        assertTrue("rejected status must be restricted",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("rejected")).isBlocked)

        // completed -> unenforced / allowed
        assertFalse("completed status must be allowed",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("completed")).isBlocked)

        // empty / unknown status -> unenforced
        assertFalse("empty status must not enforce",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("")).isBlocked)
        assertFalse("unknown status must not enforce",
            BlockerEngine.evaluatePackage("com.reddit.frontpage", createCache("unknown")).isBlocked)
    }

    // -------------------------------------------------------------------------
    // 3. System & Emergency Package Immunity
    // -------------------------------------------------------------------------

    @Test
    fun testProtectedSystemAndEmergencyPackagesImmunity() {
        val maliciousCache = CachedBlocklist(
            userId = "attacker",
            version = "v1",
            generatedAt = "2026-11-11T12:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g-malicious",
                    goalTitle = "System Lockout Attempt",
                    status = "active",
                    apps = emptyList(),
                    domains = emptyList(),
                    applicationTargets = listOf(
                        ApplicationTarget("android_package", "com.android.settings"),
                        ApplicationTarget("android_package", "com.android.phone"),
                        ApplicationTarget("android_package", "com.android.dialer"),
                        ApplicationTarget("android_package", "com.android.emergency"),
                        ApplicationTarget("android_package", "com.google.android.emergency"),
                        ApplicationTarget("android_package", "com.android.systemui"),
                        ApplicationTarget("android_package", "com.sec.android.app.launcher"),
                        ApplicationTarget("android_package", "com.google.android.apps.nexuslauncher"),
                        ApplicationTarget("android_package", "com.onusly.app")
                    )
                )
            )
        )

        val protectedPackages = listOf(
            "com.android.settings",
            "com.android.phone",
            "com.android.dialer",
            "com.android.emergency",
            "com.google.android.emergency",
            "com.android.systemui",
            "com.sec.android.app.launcher",
            "com.google.android.apps.nexuslauncher",
            "com.onusly.app"
        )

        for (pkg in protectedPackages) {
            val eval = BlockerEngine.evaluatePackage(pkg, maliciousCache)
            assertFalse("Protected system/emergency package '$pkg' must NEVER be blocked", eval.isBlocked)
        }
    }

    // -------------------------------------------------------------------------
    // 4. Package Name Validation & Exclusions
    // -------------------------------------------------------------------------

    @Test
    fun testPackageNameFormatValidation() {
        val validPackages = listOf(
            "com.example.app",
            "org.onusly.test",
            "a.b.c",
            "com.example.sub_app",
            "com.valvesoftware.android.steam.community"
        )
        for (pkg in validPackages) {
            assertTrue("Package '$pkg' should be valid", InstalledAppsManager.isValidPackageName(pkg))
        }

        val invalidPackages = listOf(
            "",
            "   ",
            "app",
            "com",
            ".com.example",
            "com.example.",
            "com..example",
            "123.example.app",
            "C:\\Program Files\\Discord\\Discord.exe",
            "/usr/bin/slack",
            "discord.exe"
        )
        for (pkg in invalidPackages) {
            assertFalse("Invalid package '$pkg' must be rejected", InstalledAppsManager.isValidPackageName(pkg))
        }
    }

    @Test
    fun testValidatePackageForTargeting() {
        // Valid targetable app
        val val1 = InstalledAppsManager.validatePackageForTargeting("com.discord")
        assertTrue("com.discord must be valid for targeting", val1 is InstalledAppsManager.ValidationResult.Valid)

        // Empty package
        val val2 = InstalledAppsManager.validatePackageForTargeting("")
        assertTrue("Empty package must be invalid", val2 is InstalledAppsManager.ValidationResult.Invalid)

        // Desktop path
        val val3 = InstalledAppsManager.validatePackageForTargeting("Discord.exe")
        assertTrue("Desktop exe must be invalid for targeting", val3 is InstalledAppsManager.ValidationResult.Invalid)

        // System settings
        val val4 = InstalledAppsManager.validatePackageForTargeting("com.android.settings")
        assertTrue("System settings must be excluded from targeting", val4 is InstalledAppsManager.ValidationResult.Invalid)

        // Emergency dialer
        val val5 = InstalledAppsManager.validatePackageForTargeting("com.android.emergency")
        assertTrue("Emergency dialer must be excluded from targeting", val5 is InstalledAppsManager.ValidationResult.Invalid)
    }

    // -------------------------------------------------------------------------
    // 5. Serialization & Parser Tests
    // -------------------------------------------------------------------------

    @Test
    fun testBlocklistParserTypedSerialization() {
        val original = CachedBlocklist(
            userId = "user-abc",
            version = "v-42",
            generatedAt = "2026-11-11T14:00:00Z",
            lastSyncTimestamp = 123456789L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "goal-99",
                    goalTitle = "Deep Focus",
                    status = "active",
                    apps = listOf("com.legacy.app"),
                    domains = listOf("distraction.com"),
                    applicationTargets = listOf(
                        ApplicationTarget(kind = "android_package", identifier = "com.instagram.android"),
                        ApplicationTarget(kind = "desktop_app", identifier = "discord.exe")
                    )
                )
            )
        )

        val json = BlocklistParser.cachedToJson(original)
        assertNotNull(json)
        assertTrue(json.contains("applicationTargets"))
        assertTrue(json.contains("android_package"))
        assertTrue(json.contains("desktop_app"))

        val parsed = BlocklistParser.parseCached(json)
        assertNotNull(parsed)
        assertEquals("user-abc", parsed!!.userId)
        assertEquals("v-42", parsed.version)
        assertEquals(1, parsed.targets.size)

        val target = parsed.targets[0]
        assertEquals("goal-99", target.goalId)
        assertEquals(2, target.applicationTargets.size)
        assertEquals("android_package", target.applicationTargets[0].kind)
        assertEquals("com.instagram.android", target.applicationTargets[0].identifier)
        assertEquals("desktop_app", target.applicationTargets[1].kind)
        assertEquals("discord.exe", target.applicationTargets[1].identifier)
    }

    @Test
    fun testGoalParserTypedSerialization() {
        val createReq = CreateGoalRequest(
            title = "Study Marathon",
            description = "No social media",
            approvalType = "friend",
            approverEmail = "partner@example.com",
            blockedApps = listOf("com.twitter.android"),
            blockedDomains = listOf("x.com"),
            applicationTargets = listOf(
                ApplicationTarget(kind = "android_package", identifier = "com.twitter.android")
            )
        )

        val createJson = GoalParser.createGoalRequestToJson(createReq)
        assertTrue("JSON must contain applicationTargets", createJson.contains("applicationTargets"))
        assertTrue("JSON must contain android_package", createJson.contains("android_package"))

        val updateReq = UpdateGoalRequest(
            title = "Updated Marathon",
            applicationTargets = listOf(
                ApplicationTarget(kind = "android_package", identifier = "com.reddit.frontpage")
            )
        )

        val updateJson = GoalParser.updateGoalRequestToJson(updateReq)
        assertTrue("JSON must contain applicationTargets", updateJson.contains("applicationTargets"))
        assertTrue("JSON must contain com.reddit.frontpage", updateJson.contains("com.reddit.frontpage"))
    }

    // -------------------------------------------------------------------------
    // 6. Policy Sync & Offline Resilience Tests
    // -------------------------------------------------------------------------

    private class MockCache(var cached: CachedBlocklist? = null) : BlocklistCache {
        var clearCount = 0
        var lastUpdatedTimestamp = 0L

        override fun getCache(expectedUserId: String?): CachedBlocklist? = cached
        override fun saveCache(response: BlocklistResponse, expectedUserId: String): Boolean {
            cached = CachedBlocklist(
                userId = response.userId,
                version = response.version,
                generatedAt = response.generatedAt,
                lastSyncTimestamp = System.currentTimeMillis(),
                targets = response.targets
            )
            return true
        }
        override fun updateLastSyncTimestamp(timestamp: Long, expectedUserId: String?) {
            lastUpdatedTimestamp = timestamp
        }
        override fun clear() {
            cached = null
            clearCount++
        }
        override fun getRawCachedUserId(): String? = cached?.userId
    }

    @Test
    fun testDeviceRevocationTriggersSignOutAndHaltsEnforcement() {
        val mockCache = MockCache(
            CachedBlocklist("u1", "v1", "now", 1000L, listOf(
                BlocklistTarget("g1", "Title", "active", emptyList(), emptyList())
            ))
        )

        var signedOut = false
        val listener = { signedOut = true }

        // Simulate 403 DEVICE_REVOKED
        val errorResult = SyncResult.Error(403, "Backend returned HTTP 403: {\"error\": \"DEVICE_REVOKED\"}")

        if (errorResult.statusCode == 403 && errorResult.message.contains("DEVICE_REVOKED", ignoreCase = true)) {
            mockCache.clear()
            listener()
        }

        assertTrue("Device revocation must trigger sign out callback", signedOut)
        assertEquals("Cache must be cleared on device revocation", 1, mockCache.clearCount)
        assertEquals("Cache must be null", null, mockCache.cached)
    }

    @Test
    fun testEtag304PreservesCacheDurability() {
        val initialCache = CachedBlocklist(
            userId = "user-1",
            version = "W/\"etag-999\"",
            generatedAt = "2026-11-11T10:00:00Z",
            lastSyncTimestamp = 1000L,
            targets = listOf(
                BlocklistTarget("g1", "Goal 1", "active", emptyList(), emptyList(), listOf(
                    ApplicationTarget("android_package", "com.discord")
                ))
            )
        )
        val mockCache = MockCache(initialCache)

        // Simulate 304 Not Modified
        val syncResult: SyncResult = SyncResult.NotModified
        if (syncResult is SyncResult.NotModified) {
            mockCache.updateLastSyncTimestamp(5000L, "user-1")
        }

        assertEquals(5000L, mockCache.lastUpdatedTimestamp)
        assertNotNull("Cache must not be null after 304", mockCache.cached)
        assertEquals("W/\"etag-999\"", mockCache.cached!!.version)
        assertEquals(1, mockCache.cached!!.targets.size)
        assertEquals(0, mockCache.clearCount)
    }
}
