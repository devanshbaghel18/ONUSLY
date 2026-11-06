package com.onusly.app

import android.content.Context
import android.content.SharedPreferences
import com.onusly.app.data.ChatRepository
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.BlocklistParser
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.SyncResult
import com.onusly.app.data.model.TargetCatalog
import com.onusly.app.data.model.UserSession
import com.onusly.app.data.realtime.RealtimeEvent
import com.onusly.app.data.realtime.RealtimeWebSocketManager
import com.onusly.app.engine.BlockerEngine
import com.onusly.app.engine.InstalledAppsManager
import com.onusly.app.ui.ChatAdapter
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import okhttp3.WebSocket
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONArray
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/**
 * SystemAuditVerificationTest
 *
 * Independent Verification & QA test suite for ONUSLY Android multi-agent audit fixes.
 * Rigorously verifies all 10 areas of bug fixes and security remediations:
 * 1. TargetCatalog domain and alias normalization
 * 2. BlockerEngine OEM launcher loop protection & alias target resolution
 * 3. InstalledAppsManager universal launcher filtering
 * 4. EnforcementApiClient HTTP 304 socket closure
 * 5. OnuslyApiClient URL path segment encoding & safe JSON parsing
 * 6. GoalParser & BlocklistParser null string safety & approval status/decision fallback
 * 7. SyncRepository sign-out listener removal & hardened HTTP 401 detection
 * 8. RealtimeWebSocketManager backoff jitter & auth error handling
 * 9. ChatRepository HTTP 401 session expiration propagation
 * 10. ChatAdapter timestamp parsing with UTC Z & fractional seconds
 */
class SystemAuditVerificationTest {

    private lateinit var server: MockWebServer
    private lateinit var authPrefs: FakeSharedPreferences
    private lateinit var cachePrefs: FakeSharedPreferences
    private lateinit var settingsPrefs: FakeSharedPreferences
    private lateinit var authManager: AuthManager
    private lateinit var blocklistCache: SharedPreferencesBlocklistCache
    private lateinit var syncRepository: SyncRepository
    private lateinit var apiClient: OnuslyApiClient
    private lateinit var testScope: CoroutineScope

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()

        authPrefs = FakeSharedPreferences()
        cachePrefs = FakeSharedPreferences()
        settingsPrefs = FakeSharedPreferences()

        settingsPrefs.edit().putString(SyncRepository.KEY_BASE_URL, server.url("/").toString()).apply()

        val dummyContext = object : android.content.ContextWrapper(null) {}
        authManager = AuthManager(dummyContext, authPrefs)
        blocklistCache = SharedPreferencesBlocklistCache(cachePrefs)
        syncRepository = SyncRepository(
            authManager = authManager,
            blocklistCache = blocklistCache,
            apiClient = EnforcementApiClient(),
            settingsPrefs = settingsPrefs
        )
        apiClient = OnuslyApiClient()
        testScope = CoroutineScope(Dispatchers.Unconfined)
    }

    @After
    fun tearDown() {
        testScope.cancel()
        server.shutdown()
    }

    // =========================================================================
    // 1. TargetCatalog.kt Verifications
    // =========================================================================

    @Test
    fun testTargetCatalog_SubdomainMatching() {
        // Mobile / web subdomains of mapped platforms
        val instaApps = TargetCatalog.getAppsForDomain("m.instagram.com")
        assertEquals(listOf("com.instagram.android"), instaApps)

        val whatsappApps = TargetCatalog.getAppsForDomain("web.whatsapp.com")
        assertTrue("web.whatsapp.com must resolve to com.whatsapp", whatsappApps.contains("com.whatsapp"))
        assertTrue("web.whatsapp.com must resolve to com.whatsapp.w4b", whatsappApps.contains("com.whatsapp.w4b"))

        val tiktokApps = TargetCatalog.getAppsForDomain("sub.tiktok.com")
        assertTrue(tiktokApps.contains("com.zhiliaoapp.musically"))
        assertTrue(tiktokApps.contains("com.ss.android.ugc.trill"))

        val twitterApps = TargetCatalog.getAppsForDomain("mobile.twitter.com")
        assertEquals(listOf("com.twitter.android"), twitterApps)

        val ytApps = TargetCatalog.getAppsForDomain("music.youtube.com")
        assertEquals(listOf("com.google.android.youtube"), ytApps)

        // YouTube Shorts subdomain must remain isolated to shorts app, NOT full youtube
        val shortsApps = TargetCatalog.getAppsForDomain("shorts.youtube.com")
        assertEquals(listOf("com.google.android.youtube.shorts"), shortsApps)
        assertFalse(shortsApps.contains("com.google.android.youtube"))

        // Unmapped arbitrary domain
        val unmapped = TargetCatalog.getAppsForDomain("api.github.com")
        assertTrue("Unmapped domain must return empty app list", unmapped.isEmpty())
    }

    @Test
    fun testTargetCatalog_GetPackagesForAppOrAlias() {
        // Common colloquial aliases
        val igPkgs = TargetCatalog.getPackagesForAppOrAlias("ig")
        assertEquals(listOf("com.instagram.android"), igPkgs)

        val instaPkgs = TargetCatalog.getPackagesForAppOrAlias("insta")
        assertEquals(listOf("com.instagram.android"), instaPkgs)

        val ytPkgs = TargetCatalog.getPackagesForAppOrAlias("yt")
        assertEquals(listOf("com.google.android.youtube"), ytPkgs)

        val fbPkgs = TargetCatalog.getPackagesForAppOrAlias("fb")
        assertTrue(fbPkgs.contains("com.facebook.katana"))
        assertTrue(fbPkgs.contains("com.facebook.orca"))

        val netflixPkgs = TargetCatalog.getPackagesForAppOrAlias("netflix")
        assertEquals(listOf("com.netflix.mediaclient"), netflixPkgs)

        val whatsappPkgs = TargetCatalog.getPackagesForAppOrAlias("whatsapp")
        assertTrue(whatsappPkgs.contains("com.whatsapp"))

        // Case-insensitivity & trimming
        assertEquals(listOf("com.instagram.android"), TargetCatalog.getPackagesForAppOrAlias("  IG  "))
        assertEquals(listOf("com.google.android.youtube"), TargetCatalog.getPackagesForAppOrAlias("  YT  "))

        // Direct package names
        assertEquals(listOf("com.instagram.android"), TargetCatalog.getPackagesForAppOrAlias("com.instagram.android"))
        assertEquals(listOf("com.discord"), TargetCatalog.getPackagesForAppOrAlias("com.discord"))

        // Catalog entry names
        assertEquals(listOf("com.instagram.android"), TargetCatalog.getPackagesForAppOrAlias("Instagram"))
        assertEquals(listOf("com.reddit.frontpage"), TargetCatalog.getPackagesForAppOrAlias("Reddit"))

        // Blank or non-existent
        assertTrue(TargetCatalog.getPackagesForAppOrAlias("").isEmpty())
        assertTrue(TargetCatalog.getPackagesForAppOrAlias("   ").isEmpty())
        assertTrue(TargetCatalog.getPackagesForAppOrAlias("unknown_nonexistent_alias_xyz").isEmpty())
    }

    @Test
    fun testTargetCatalog_NormalizeDomain() {
        // Scheme stripping
        assertEquals("instagram.com", TargetCatalog.normalizeDomain("https://instagram.com"))
        assertEquals("x.com", TargetCatalog.normalizeDomain("http://x.com"))

        // Lowercase conversion
        assertEquals("facebook.com", TargetCatalog.normalizeDomain("HTTPS://WWW.FACEBOOK.COM/"))

        // Port removal
        assertEquals("example.com", TargetCatalog.normalizeDomain("https://example.com:8080/api/v1"))

        // Subpath, query, fragment stripping
        assertEquals("reddit.com", TargetCatalog.normalizeDomain("https://reddit.com/r/technology?sort=top#comments"))

        // Trailing dot removal
        assertEquals("snapchat.com", TargetCatalog.normalizeDomain("snapchat.com."))

        // Leading www removal
        assertEquals("tiktok.com", TargetCatalog.normalizeDomain("www.tiktok.com"))

        // Special handling for YouTube Shorts
        assertEquals("shorts.youtube.com", TargetCatalog.normalizeDomain("https://youtube.com/shorts"))
        assertEquals("shorts.youtube.com", TargetCatalog.normalizeDomain("https://www.youtube.com/shorts/dQw4w9WgXcQ"))
        assertEquals("shorts.youtube.com", TargetCatalog.normalizeDomain("m.youtube.com/shorts"))

        // Null and blank handling
        assertEquals("", TargetCatalog.normalizeDomain(null))
        assertEquals("", TargetCatalog.normalizeDomain(""))
        assertEquals("", TargetCatalog.normalizeDomain("   "))
    }

    // =========================================================================
    // 2. BlockerEngine.kt Verifications
    // =========================================================================

    @Test
    fun testBlockerEngine_AlwaysAllowedLaunchers_InescapableLoopProtection() {
        val launcherTargets = listOf(
            "com.teslacoilsw.launcher",          // Nova Launcher
            "app.lawnchair",                     // Lawnchair Modern
            "app.lawnchair.play",                // Lawnchair Play Store
            "ch.deletescape.lawnchair",          // Lawnchair Classic
            "com.google.android.apps.nexuslauncher", // Pixel Launcher
            "com.sec.android.app.launcher",      // Samsung OneUI
            "com.sec.android.emergencylauncher", // Samsung Emergency Launcher
            "com.miui.home",                     // Xiaomi MIUI Launcher
            "com.mi.android.globallauncher",     // Xiaomi Poco / Global Launcher
            "com.nothing.launcher",              // Nothing Launcher
            "com.sonymobile.home",               // Sony Launcher
            "com.asus.launcher",                 // Asus ZenUI Launcher
            "com.transsion.hilauncher",          // Tecno HiOS Launcher
            "com.transsion.itel.launcher",       // Itel Launcher
            "com.microsoft.launcher",            // Microsoft Launcher
            "bitpit.launcher",                   // Niagara Launcher
            "ginlemon.flowerfree",               // Smart Launcher
            "com.motorola.launcher",             // Motorola Launcher
            "com.motorola.launcher3",            // Motorola Launcher3
            "com.oppo.launcher",                 // Oppo Launcher
            "com.oneplus.launcher",              // OnePlus Launcher
            "com.vivo.launcher",                 // Vivo Launcher
            "com.bbk.launcher2",                 // BBK Launcher
            "com.huawei.android.launcher"        // Huawei EMUI Launcher
        )

        val maliciousCache = CachedBlocklist(
            userId = "test_user",
            version = "v1",
            generatedAt = "2026-10-31T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = listOf(
                BlocklistTarget(
                    goalId = "rogue_goal",
                    goalTitle = "Attempt To Lock User Out",
                    status = "active",
                    apps = launcherTargets,
                    domains = emptyList()
                )
            )
        )

        for (launcherPkg in launcherTargets) {
            val eval = BlockerEngine.evaluatePackage(launcherPkg, maliciousCache)
            assertFalse(
                "Launcher '$launcherPkg' MUST NEVER BE BLOCKED by BlockerEngine to prevent LockActivity trap loops",
                eval.isBlocked
            )
            assertTrue("Matching targets must be empty for allowed launcher: $launcherPkg", eval.matchingTargets.isEmpty())
        }
    }

    @Test
    fun testBlockerEngine_ResolvesAliasesAndSubdomains() {
        // Goal targeting alias "ig" and subdomain "web.whatsapp.com"
        val cache = CachedBlocklist(
            userId = "user_1",
            version = "v1",
            generatedAt = "2026-10-31T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = listOf(
                BlocklistTarget(
                    goalId = "goal_alias",
                    goalTitle = "Block Social Aliases",
                    status = "active",
                    apps = listOf("ig", "yt", "fb"),
                    domains = listOf("web.whatsapp.com", "sub.tiktok.com")
                )
            )
        )

        // "ig" alias must block com.instagram.android
        val resIg = BlockerEngine.evaluatePackage("com.instagram.android", cache)
        assertTrue("com.instagram.android must be blocked when target app is 'ig'", resIg.isBlocked)

        // "yt" alias must block com.google.android.youtube
        val resYt = BlockerEngine.evaluatePackage("com.google.android.youtube", cache)
        assertTrue("com.google.android.youtube must be blocked when target app is 'yt'", resYt.isBlocked)

        // "fb" alias must block com.facebook.katana
        val resFb = BlockerEngine.evaluatePackage("com.facebook.katana", cache)
        assertTrue("com.facebook.katana must be blocked when target app is 'fb'", resFb.isBlocked)

        // "web.whatsapp.com" domain must block com.whatsapp
        val resWa = BlockerEngine.evaluatePackage("com.whatsapp", cache)
        assertTrue("com.whatsapp must be blocked when target domain is 'web.whatsapp.com'", resWa.isBlocked)

        // "sub.tiktok.com" domain must block musically
        val resTt = BlockerEngine.evaluatePackage("com.zhiliaoapp.musically", cache)
        assertTrue("com.zhiliaoapp.musically must be blocked when target domain is 'sub.tiktok.com'", resTt.isBlocked)
    }

    // =========================================================================
    // 3. InstalledAppsManager.kt Verifications
    // =========================================================================

    @Test
    fun testInstalledAppsManager_UniversalLauncherExclusion() {
        // Packages ending with .launcher or .launcher3
        assertTrue(InstalledAppsManager.isSystemOrExcluded("com.custom.vendor.launcher"))
        assertTrue(InstalledAppsManager.isSystemOrExcluded("com.oem.device.launcher3"))

        // Packages containing .launcher. in the package name
        assertTrue(InstalledAppsManager.isSystemOrExcluded("com.custom.launcher.theme"))
        assertTrue(InstalledAppsManager.isSystemOrExcluded("com.transsion.itel.launcher.overlay"))

        // Standard apps that don't match launcher pattern should NOT be excluded
        assertFalse(InstalledAppsManager.isSystemOrExcluded("com.instagram.android"))
        assertFalse(InstalledAppsManager.isSystemOrExcluded("com.spotify.music"))
        assertFalse(InstalledAppsManager.isSystemOrExcluded("com.homeassistant.companion.android"))
        assertFalse(InstalledAppsManager.isSystemOrExcluded("com.homedepot.digital"))
    }

    // =========================================================================
    // 4. EnforcementApiClient.kt Verifications
    // =========================================================================

    @Test
    fun testEnforcementApiClient_Http304SocketClosure() = runBlocking {
        val client = EnforcementApiClient()

        // 1. Enqueue 304 Not Modified
        server.enqueue(MockResponse().setResponseCode(304))

        val result304 = client.fetchBlocklist(
            baseUrl = server.url("/").toString(),
            jwtToken = "test_token",
            ifNoneMatch = "v1-existing"
        )

        assertTrue("HTTP 304 must return SyncResult.NotModified", result304 is SyncResult.NotModified)

        // 2. Enqueue subsequent 200 response to verify socket pool is healthy and was not leaked
        val validJson = """
            {
                "userId": "user_1",
                "version": "v2-updated",
                "generatedAt": "2026-10-31T12:00:00Z",
                "targets": []
            }
        """.trimIndent()
        server.enqueue(MockResponse().setResponseCode(200).setBody(validJson))

        val result200 = client.fetchBlocklist(
            baseUrl = server.url("/").toString(),
            jwtToken = "test_token"
        )

        assertTrue("Subsequent request must succeed, proving socket was closed cleanly", result200 is SyncResult.Success)
        val resp = (result200 as SyncResult.Success).response
        assertEquals("v2-updated", resp.version)
    }

    // =========================================================================
    // 5. OnuslyApiClient.kt Verifications
    // =========================================================================

    @Test
    fun testOnuslyApiClient_EncodePathSegment() {
        // Must use %20 instead of + for path segments
        assertEquals("goal%20with%20spaces", OnuslyApiClient.encodePathSegment("goal with spaces"))
        assertEquals("abc%20123%20xyz", OnuslyApiClient.encodePathSegment("abc 123 xyz"))
        assertEquals("a%2Bb", OnuslyApiClient.encodePathSegment("a+b"))
        assertEquals("item%2Fdetail", OnuslyApiClient.encodePathSegment("item/detail"))
        assertEquals("simple-goal_id", OnuslyApiClient.encodePathSegment("simple-goal_id"))
    }

    @Test
    fun testOnuslyApiClient_MapCatchingHandlesMalformedJsonSafely() = runBlocking {
        // Enqueue 200 OK with malformed JSON body
        server.enqueue(MockResponse().setResponseCode(200).setBody("{not-valid-json-syntax..."))

        val result = apiClient.getGoal(server.url("/").toString(), "fake_jwt", "goal-1")

        // Must return Result.failure rather than throwing uncaught JSONException
        assertTrue("Malformed JSON response must yield Result.failure", result.isFailure)
        assertNotNull("Failure exception must be present", result.exceptionOrNull())
    }

    @Test
    fun testOnuslyApiClient_MapCatchingAcrossEndpoints() = runBlocking {
        val baseUrl = server.url("/").toString()

        // 1. getGoal with malformed HTML body
        server.enqueue(MockResponse().setResponseCode(200).setBody("<html>502 Bad Gateway</html>"))
        val goalResult = apiClient.getGoal(baseUrl, "jwt", "goal-1")
        assertTrue("getGoal must fail gracefully on malformed JSON", goalResult.isFailure)

        // 2. getFriends with malformed JSON
        server.enqueue(MockResponse().setResponseCode(200).setBody("not a json array"))
        val friendsResult = apiClient.getFriends(baseUrl, "jwt")
        assertTrue("getFriends must fail gracefully on malformed JSON", friendsResult.isFailure)

        // 3. getChatHistory with malformed JSON
        server.enqueue(MockResponse().setResponseCode(200).setBody("{invalid}"))
        val chatResult = apiClient.getChatHistory(baseUrl, "jwt", "user-2")
        assertTrue("getChatHistory must fail gracefully on malformed JSON", chatResult.isFailure)

        // 4. getMyProfile with malformed JSON
        server.enqueue(MockResponse().setResponseCode(200).setBody("corrupted-profile-json"))
        val profileResult = apiClient.getMyProfile(baseUrl, "jwt")
        assertTrue("getMyProfile must fail gracefully on malformed JSON", profileResult.isFailure)
    }

    // =========================================================================
    // 6. GoalParser.kt & BlocklistParser.kt Verifications
    // =========================================================================

    @Test
    fun testOptSafeString_LiteralNullHandling() {
        val json = JSONObject("""
            {
                "normal": "value",
                "nullVal": null,
                "nullString": "null",
                "empty": ""
            }
        """.trimIndent())

        assertEquals("value", GoalParser.optSafeString(json, "normal"))
        assertEquals("", GoalParser.optSafeString(json, "nullVal"))
        assertEquals("", GoalParser.optSafeString(json, "nullString"))
        assertEquals("", GoalParser.optSafeString(json, "empty"))
        assertEquals("fallback", GoalParser.optSafeString(json, "missing", "fallback"))
        assertEquals("fallback", GoalParser.optSafeString(json, "nullVal", "fallback"))

        // Also test BlocklistParser.optSafeString
        assertEquals("value", BlocklistParser.optSafeString(json, "normal"))
        assertEquals("", BlocklistParser.optSafeString(json, "nullVal"))
        assertEquals("", BlocklistParser.optSafeString(json, "nullString"))
    }

    @Test
    fun testGoalParser_ParseGoalWithNullFields() {
        val json = JSONObject("""
            {
                "id": null,
                "ownerId": null,
                "title": null,
                "description": null,
                "status": null,
                "blockedApps": [null, "com.instagram.android", "null"],
                "blockedDomains": [null, "instagram.com", ""]
            }
        """.trimIndent())

        val goal = GoalParser.parseGoal(json)
        assertEquals("", goal.id)
        assertEquals("", goal.ownerId)
        assertEquals("", goal.title)
        assertEquals("", goal.description)
        assertEquals("active", goal.status) // default fallback

        // Blocked apps must contain only valid apps and skip null / "null"
        assertEquals(listOf("com.instagram.android"), goal.blockedApps)
        assertEquals(listOf("instagram.com"), goal.blockedDomains)
    }

    @Test
    fun testGoalParser_ParseApprovalResult_DecisionAndStatusSupport() {
        // Case 1: Payload has "decision" field (e.g. backend POST /approval response)
        val jsonDecision = """
            {
                "id": "appr_1",
                "goalId": "goal_1",
                "proofId": "proof_1",
                "approverId": "appr_user",
                "decision": "approved",
                "comment": "Nice work",
                "decidedAt": "2026-10-31T12:00:00Z"
            }
        """.trimIndent()
        val res1 = GoalParser.parseApprovalResult(jsonDecision)
        assertEquals("approved", res1.status)
        assertEquals("Nice work", res1.comment)

        // Case 2: Payload has "status" field
        val jsonStatus = """
            {
                "id": "appr_2",
                "goalId": "goal_2",
                "proofId": "proof_2",
                "approverId": "appr_user",
                "status": "rejected",
                "comment": "Needs better proof"
            }
        """.trimIndent()
        val res2 = GoalParser.parseApprovalResult(jsonStatus)
        assertEquals("rejected", res2.status)

        // Case 3: Payload has literal null status but valid decision
        val jsonNullStatus = """
            {
                "id": "appr_3",
                "status": null,
                "decision": "approved"
            }
        """.trimIndent()
        val res3 = GoalParser.parseApprovalResult(jsonNullStatus)
        assertEquals("approved", res3.status)
    }

    @Test
    fun testBlocklistParser_NullSafetyInResponseAndCache() {
        val rawResponse = """
            {
                "userId": null,
                "version": null,
                "generatedAt": null,
                "targets": [
                    {
                        "goalId": null,
                        "goalTitle": null,
                        "status": null,
                        "apps": [null, "com.test.app"],
                        "domains": [null, "test.com"]
                    }
                ]
            }
        """.trimIndent()

        val parsed = BlocklistParser.parseResponse(rawResponse)
        assertEquals("", parsed.userId)
        assertEquals("", parsed.version)
        assertEquals("", parsed.generatedAt)
        assertEquals(1, parsed.targets.size)
        assertEquals("", parsed.targets[0].goalId)
        assertEquals(listOf("com.test.app"), parsed.targets[0].apps)
        assertEquals(listOf("test.com"), parsed.targets[0].domains)

        val cachedJson = """
            {
                "userId": null,
                "version": null,
                "generatedAt": null,
                "lastSyncTimestamp": 1000,
                "targets": []
            }
        """.trimIndent()

        val cached = BlocklistParser.parseCached(cachedJson)
        assertNotNull(cached)
        assertEquals("", cached?.userId)
        assertEquals("", cached?.version)
    }

    // =========================================================================
    // 7. SyncRepository.kt Verifications
    // =========================================================================

    @Test
    fun testSyncRepository_RemoveSignOutListenerPreventsMemoryLeak() {
        val countA = AtomicInteger(0)
        val countB = AtomicInteger(0)

        val listenerA: () -> Unit = { countA.incrementAndGet() }
        val listenerB: () -> Unit = { countB.incrementAndGet() }

        syncRepository.addSignOutListener(listenerA)
        syncRepository.addSignOutListener(listenerB)

        // Trigger sign-out
        syncRepository.onUserSignOut()
        assertEquals(1, countA.get())
        assertEquals(1, countB.get())

        // Remove listenerA
        syncRepository.removeSignOutListener(listenerA)

        // Reset sign-out state via reflection to simulate a fresh sign-in and subsequent sign-out
        val isSigningOutField = SyncRepository::class.java.getDeclaredField("isSigningOut")
        isSigningOutField.isAccessible = true
        isSigningOutField.set(syncRepository, false)

        syncRepository.onUserSignOut()
        assertEquals("ListenerA must NOT be called after removal", 1, countA.get())
        assertEquals("ListenerB must continue receiving events", 2, countB.get())
    }

    @Test
    fun testSyncRepository_Hardened401Detection() = runBlocking {
        val signOutCalled = AtomicBoolean(false)
        syncRepository.addSignOutListener { signOutCalled.set(true) }

        // Set session
        authManager.saveSession(UserSession("user_1", "user@test.com", "Test User", "valid_token"))

        // 1. Error with statusCode 401 must trigger sign-out
        server.enqueue(MockResponse().setResponseCode(401).setBody("Unauthorized"))
        syncRepository.syncBlocklist()
        assertTrue("HTTP 401 status code must trigger sign-out", signOutCalled.get())

        // Re-authenticate session because onUserSignOut cleared it
        signOutCalled.set(false)
        authManager.saveSession(UserSession("user_1", "user@test.com", "Test User", "valid_token"))

        // 2. Error message containing "HTTP 401" must trigger sign-out
        server.enqueue(MockResponse().setResponseCode(500).setBody("Proxy error: HTTP 401 received from upstream"))
        syncRepository.syncBlocklist()
        assertTrue("Message containing 'HTTP 401' must trigger sign-out", signOutCalled.get())

        // Re-authenticate session
        signOutCalled.set(false)
        authManager.saveSession(UserSession("user_1", "user@test.com", "Test User", "valid_token"))

        // 3. Error with legitimate number 401 inside message (e.g. 40123) must NOT trigger false-positive sign-out
        server.enqueue(MockResponse().setResponseCode(500).setBody("Database item 40123 not found"))
        syncRepository.syncBlocklist()
        assertFalse("Message containing '40123' without HTTP 401 must NOT trigger sign-out", signOutCalled.get())
    }

    // =========================================================================
    // 8. RealtimeWebSocketManager.kt Verifications
    // =========================================================================

    @Test
    fun testRealtimeWebSocketManager_BackoffDelayAndJitter() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "fake_token" },
            client = OkHttpClient(),
            coroutineScope = testScope
        )

        // Test attempt 0: base 1000ms + up to 20% jitter
        manager.setReconnectAttemptsForTesting(0)
        val delay0 = manager.getBackoffDelayForTesting()
        assertTrue("Attempt 0 delay must be in [1000, 1200], got $delay0", delay0 in 1000L..1200L)

        // Test attempt 1: base 2000ms + jitter
        manager.setReconnectAttemptsForTesting(1)
        val delay1 = manager.getBackoffDelayForTesting()
        assertTrue("Attempt 1 delay must be in [2000, 2400], got $delay1", delay1 in 2000L..2400L)

        // Test attempt 4: base 16000ms + jitter
        manager.setReconnectAttemptsForTesting(4)
        val delay4 = manager.getBackoffDelayForTesting()
        assertTrue("Attempt 4 delay must be in [16000, 19200], got $delay4", delay4 in 16000L..19200L)

        // Test attempt 5+: base 32000ms capped at 30000ms
        manager.setReconnectAttemptsForTesting(5)
        val delay5 = manager.getBackoffDelayForTesting()
        assertTrue("Attempt 5+ delay must be capped at 30000ms, got $delay5", delay5 in 1000L..30000L)

        // Disconnect must reset attempts to 0
        manager.disconnect()
        assertEquals(0, manager.getReconnectAttemptsForTesting())
    }

    @Test
    fun testRealtimeWebSocketManager_AuthErrorAbortsReconnect() {
        val tokenExpiredCalled = AtomicBoolean(false)
        val customAuthManager = AuthManager(
            context = object : android.content.ContextWrapper(null) {},
            prefs = FakeSharedPreferences()
        )
        customAuthManager.addTokenExpirationListener {
            tokenExpiredCalled.set(true)
        }

        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "fake_token" },
            client = OkHttpClient(),
            authManager = customAuthManager,
            coroutineScope = testScope
        )

        // Connect creates listener
        manager.connect()
        val listener = manager.currentListenerForTesting
        assertNotNull("Listener must be registered on connect", listener)

        // Simulate WebSocket failure with HTTP 401 response
        val dummyRequest = Request.Builder().url("http://localhost:8080/ws").build()
        val response401 = Response.Builder()
            .request(dummyRequest)
            .protocol(Protocol.HTTP_1_1)
            .code(401)
            .message("Unauthorized")
            .body("".toResponseBody("text/plain".toMediaType()))
            .build()

        val dummySocket: WebSocket = object : WebSocket {
            override fun cancel() {}
            override fun close(code: Int, reason: String?): Boolean = true
            override fun queueSize(): Long = 0
            override fun request(): Request = dummyRequest
            override fun send(text: String): Boolean = true
            override fun send(bytes: okio.ByteString): Boolean = true
        }

        listener?.onFailure(dummySocket, Exception("HTTP 401"), response401)

        // Must notify token expired and NOT schedule reconnect
        assertTrue("AuthManager must be notified of token expiration on HTTP 401", tokenExpiredCalled.get())
        assertFalse("Reconnect must NOT be scheduled on auth failure (HTTP 401)", manager.hasPendingReconnect())

        manager.disconnect()
    }

    // =========================================================================
    // 9. ChatRepository.kt Verifications
    // =========================================================================

    @Test
    fun testChatRepository_Propagates401ToSessionExpiration() = runBlocking {
        val sessionExpired = AtomicBoolean(false)
        syncRepository.addSignOutListener { sessionExpired.set(true) }

        // Setup signed in session
        authManager.saveSession(UserSession("my_user_id", "me@example.com", "Me", "valid_token"))

        val chatRepo = ChatRepository(
            authManager = authManager,
            syncRepository = syncRepository,
            apiClient = apiClient
        )

        // 1. Enqueue 401 on getChatHistory
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"error":"token_expired"}"""))
        val historyResult = chatRepo.getChatHistory("partner_user")
        assertTrue(historyResult.isFailure)
        assertTrue("ChatRepository must trigger session expiration on HTTP 401", sessionExpired.get())

        // Reset
        sessionExpired.set(false)
        val isSigningOutField = SyncRepository::class.java.getDeclaredField("isSigningOut")
        isSigningOutField.isAccessible = true
        isSigningOutField.set(syncRepository, false)

        // 2. Enqueue 500 error: must NOT trigger session expiration
        server.enqueue(MockResponse().setResponseCode(500).setBody("Internal Server Error"))
        val friendsResult = chatRepo.getFriends()
        assertTrue(friendsResult.isFailure)
        assertFalse("HTTP 500 must NOT trigger session expiration", sessionExpired.get())
    }

    // =========================================================================
    // 10. ChatAdapter.kt Verifications
    // =========================================================================

    @Test
    fun testChatAdapter_TimestampParsingWithUTCAndFractions() {
        // Standard ISO timestamp ending with Z
        val t1 = ChatAdapter.formatTime("2026-10-31T17:30:00Z")
        assertTrue("Must format standard ISO UTC timestamp cleanly, got: $t1", t1.isNotBlank())
        assertTrue("Time must contain minutes: $t1", t1.contains("30"))

        // ISO timestamp with fractional seconds and trailing Z
        val t2 = ChatAdapter.formatTime("2026-10-31T09:15:30.123Z")
        assertTrue("Must format timestamp with fractional seconds and Z, got: $t2", t2.isNotBlank())
        assertTrue("Time must contain minutes: $t2", t2.contains("15"))

        // ISO timestamp without Z or fractions
        val t3 = ChatAdapter.formatTime("2026-10-31T14:45:00")
        assertTrue("Must format timestamp without Z, got: $t3", t3.isNotBlank())
        assertTrue("Time must contain minutes: $t3", t3.contains("45"))

        // Blank input returns blank
        assertEquals("", ChatAdapter.formatTime(""))
        assertEquals("", ChatAdapter.formatTime("   "))

        // Malformed string returns itself safely
        assertEquals("invalid-time-format", ChatAdapter.formatTime("invalid-time-format"))
    }
}
