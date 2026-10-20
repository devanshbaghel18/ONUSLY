package com.onusly.app

import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class Slice4SettingsAndDiagnosticsTest {

    private lateinit var server: MockWebServer
    private lateinit var authPrefs: FakeSharedPreferences
    private lateinit var cachePrefs: FakeSharedPreferences
    private lateinit var settingsPrefs: FakeSharedPreferences

    private lateinit var authManager: AuthManager
    private lateinit var blocklistCache: SharedPreferencesBlocklistCache
    private lateinit var syncRepository: SyncRepository
    private lateinit var goalsRepository: GoalsRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()

        authPrefs = FakeSharedPreferences()
        cachePrefs = FakeSharedPreferences()
        settingsPrefs = FakeSharedPreferences()

        // Configure baseUrl to MockWebServer
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
        goalsRepository = GoalsRepository(
            authManager = authManager,
            syncRepository = syncRepository,
            apiClient = OnuslyApiClient()
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun signInAs(userId: String = "USER#alice", email: String = "alice@example.com") {
        authManager.saveSession(
            UserSession(
                userId = userId,
                email = email,
                name = "Alice",
                jwtToken = "header.payload.signature"
            )
        )
    }

    // --- 1. Configurable Base URL Validation ---

    @Test
    fun testBaseUrlValidation_AcceptsValidHttpAndHttpsUrls() {
        assertTrue(SyncRepository.isValidBaseUrl("http://10.0.2.2:8080"))
        assertTrue(SyncRepository.isValidBaseUrl("http://localhost:8080"))
        assertTrue(SyncRepository.isValidBaseUrl("https://api.onusly.com"))
        assertTrue(SyncRepository.isValidBaseUrl("http://192.168.1.150:8080"))
        assertTrue(SyncRepository.isValidBaseUrl("http://192.168.1.150:8080/"))
    }

    @Test
    fun testBaseUrlValidation_RejectsMalformedAndUnsupportedSchemes() {
        assertFalse(SyncRepository.isValidBaseUrl(""))
        assertFalse(SyncRepository.isValidBaseUrl("   "))
        assertFalse(SyncRepository.isValidBaseUrl("ftp://files.onusly.com"))
        assertFalse(SyncRepository.isValidBaseUrl("ws://socket.onusly.com"))
        assertFalse(SyncRepository.isValidBaseUrl("file:///android_asset/"))
        assertFalse(SyncRepository.isValidBaseUrl("not-a-valid-url"))
        assertFalse(SyncRepository.isValidBaseUrl("http://"))
        assertFalse(SyncRepository.isValidBaseUrl("https://"))
    }

    @Test
    fun testSetBaseUrl_UpdatesWhenValidAndThrowsWhenInvalid() {
        val customUrl = "http://192.168.1.200:8080"
        syncRepository.setBaseUrl(customUrl)
        assertEquals(customUrl, syncRepository.getBaseUrl())
        assertFalse(syncRepository.isDefaultBaseUrl())

        // Reset to default
        syncRepository.setBaseUrl(SyncRepository.DEFAULT_BASE_URL)
        assertEquals(SyncRepository.DEFAULT_BASE_URL, syncRepository.getBaseUrl())
        assertTrue(syncRepository.isDefaultBaseUrl())

        // Attempting to set an invalid URL throws IllegalArgumentException
        try {
            syncRepository.setBaseUrl("ftp://bad.scheme")
            fail("Expected IllegalArgumentException for invalid scheme")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message?.contains("Invalid base URL") == true)
        }

        try {
            syncRepository.setBaseUrl("not_a_url")
            fail("Expected IllegalArgumentException for malformed URL")
        } catch (e: IllegalArgumentException) {
            assertTrue(e.message?.contains("Invalid base URL") == true)
        }
    }

    // --- 2. Logout & Multi-Account Cache Isolation ---

    @Test
    fun testLogoutAndCacheIsolation_ClearsAllSessionAndCacheState() = runBlocking {
        signInAs(userId = "USER#alice", email = "alice@example.com")
        assertTrue(authManager.isSignedIn())

        // Save active cache for User Alice
        val aliceBlocklist = BlocklistResponse(
            userId = "USER#alice",
            version = "W/\"v1\"",
            generatedAt = "2026-10-20T00:00:00Z",
            targets = listOf(
                BlocklistTarget(
                    goalId = "g1",
                    goalTitle = "Study Go",
                    status = "active",
                    apps = listOf("com.instagram.android"),
                    domains = emptyList()
                )
            )
        )
        val saved = blocklistCache.saveCache(aliceBlocklist, "USER#alice")
        assertTrue(saved)
        assertEquals("USER#alice", blocklistCache.getRawCachedUserId())

        // Simulate fetching goals into in-memory flow
        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            [
                {
                    "id": "g1",
                    "title": "Study Go",
                    "status": "active",
                    "approvalType": "friend",
                    "approverId": "USER#bob",
                    "blockedApps": ["com.instagram.android"]
                }
            ]
        """.trimIndent()))
        val goalsResult = goalsRepository.fetchGoals()
        assertTrue(goalsResult.isSuccess)
        assertEquals(1, goalsRepository.goalsFlow.value.size)

        // Perform full logout: syncRepository.onUserSignOut() + goalsRepository.clearLocalData()
        syncRepository.onUserSignOut()
        goalsRepository.clearLocalData()

        // Verify session is wiped
        assertFalse(authManager.isSignedIn())
        assertNull(authManager.getSession())

        // Verify on-disk blocklist cache is wiped
        assertNull(syncRepository.getActiveCache())
        assertNull(blocklistCache.getRawCachedUserId())
        assertNull(blocklistCache.getCache("USER#alice"))

        // Verify in-memory flow is wiped
        assertEquals(0, goalsRepository.goalsFlow.value.size)
        assertFalse(goalsRepository.isOfflineFlow.value)

        // Verify User B can now log in and save their cache without collision or cross-account leakage
        signInAs(userId = "USER#bob", email = "bob@example.com")
        assertTrue(authManager.isSignedIn())

        val bobBlocklist = BlocklistResponse(
            userId = "USER#bob",
            version = "W/\"v2\"",
            generatedAt = "2026-10-20T01:00:00Z",
            targets = listOf(
                BlocklistTarget(
                    goalId = "g2",
                    goalTitle = "TikTok Diet",
                    status = "active",
                    apps = listOf("com.tiktok.android"),
                    domains = emptyList()
                )
            )
        )
        val bobSaved = blocklistCache.saveCache(bobBlocklist, "USER#bob")
        assertTrue("User Bob must be able to save cache after Alice logged out", bobSaved)
        assertEquals("USER#bob", blocklistCache.getRawCachedUserId())
        assertEquals("USER#bob", blocklistCache.getCache("USER#bob")?.userId)
    }

    // --- 3. Profile & Handle Customization ---

    @Test
    fun testGetMyProfile_RetrievesAuthenticatedUserProfile() = runBlocking {
        signInAs()

        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            {
                "id": "USER#alice",
                "email": "alice@example.com",
                "name": "Alice Wonderland",
                "handle": "alice_wonder",
                "createdAt": "2026-10-19T00:00:00Z"
            }
        """.trimIndent()))

        val profileResult = goalsRepository.getMyProfile()
        assertTrue(profileResult.isSuccess)
        val profile = profileResult.getOrNull()
        assertNotNull(profile)
        assertEquals("USER#alice", profile?.id)
        assertEquals("alice_wonder", profile?.handle)
        assertEquals("Alice Wonderland", profile?.name)
    }

    @Test
    fun testUpdateHandle_Success_SendsCorrectPayloadAndReturnsUpdatedProfile() = runBlocking {
        signInAs()

        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            {
                "id": "USER#alice",
                "email": "alice@example.com",
                "name": "Alice Wonderland",
                "handle": "alice_runner",
                "createdAt": "2026-10-19T00:00:00Z"
            }
        """.trimIndent()))

        val updateResult = goalsRepository.updateHandle("alice_runner")
        assertTrue(updateResult.isSuccess)
        assertEquals("alice_runner", updateResult.getOrNull()?.handle)

        val recordedRequest = server.takeRequest()
        assertEquals("PATCH", recordedRequest.method)
        assertEquals("/users/handle", recordedRequest.path)
        assertTrue(recordedRequest.body.readUtf8().contains("\"handle\":\"alice_runner\""))
    }

    @Test
    fun testUpdateHandle_DuplicateHandle400_ReturnsFailureWithoutCrash() = runBlocking {
        signInAs()

        server.enqueue(MockResponse().setResponseCode(400).setBody("""
            {"error":"this handle is already taken, please choose another"}
        """.trimIndent()))

        val updateResult = goalsRepository.updateHandle("taken_handle")
        assertTrue(updateResult.isFailure)
        val exception = updateResult.exceptionOrNull()
        assertTrue(exception is OnuslyApiException)
        val apiEx = exception as OnuslyApiException
        assertEquals(400, apiEx.statusCode)
        assertTrue(apiEx.message.contains("this handle is already taken, please choose another"))
    }

    // --- 4. Offline & Stale-Data Status Tracking ---

    @Test
    fun testOfflineStatus_TogglesOnNetworkFailureAndRecoversOnSuccess() = runBlocking {
        signInAs()

        // 1. Initial successful fetch
        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            [
                {
                    "id": "g10",
                    "title": "Cached Active Goal",
                    "status": "active",
                    "approvalType": "none"
                }
            ]
        """.trimIndent()))

        val initialResult = goalsRepository.fetchGoals()
        assertTrue(initialResult.isSuccess)
        assertEquals(1, goalsRepository.goalsFlow.value.size)
        assertEquals("Cached Active Goal", goalsRepository.goalsFlow.value[0].title)
        assertFalse("Should not be offline after successful fetch", goalsRepository.isOfflineFlow.value)

        // 2. Network failure (server 500 error or connection dropped)
        server.enqueue(MockResponse().setResponseCode(500).setBody("""{"error":"Internal Server Error"}"""))
        val failedResult = goalsRepository.fetchGoals()
        assertTrue(failedResult.isFailure)

        // Offline flow must now indicate offline
        assertTrue("isOfflineFlow must be true after network failure", goalsRepository.isOfflineFlow.value)

        // Crucial invariant: previously cached goals in memory must NOT be wiped
        assertEquals(1, goalsRepository.goalsFlow.value.size)
        assertEquals("Cached Active Goal", goalsRepository.goalsFlow.value[0].title)

        // 3. Network recovers
        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            [
                {
                    "id": "g10",
                    "title": "Cached Active Goal",
                    "status": "active",
                    "approvalType": "none"
                },
                {
                    "id": "g11",
                    "title": "Fresh Goal",
                    "status": "active",
                    "approvalType": "none"
                }
            ]
        """.trimIndent()))

        val recoveredResult = goalsRepository.fetchGoals()
        assertTrue(recoveredResult.isSuccess)
        assertFalse("isOfflineFlow must return to false after recovery", goalsRepository.isOfflineFlow.value)
        assertEquals(2, goalsRepository.goalsFlow.value.size)

        // 4. clearLocalData resets offline state
        goalsRepository.setOffline(true)
        assertTrue(goalsRepository.isOfflineFlow.value)
        goalsRepository.clearLocalData()
        assertFalse("clearLocalData must reset isOfflineFlow to false", goalsRepository.isOfflineFlow.value)
        assertEquals(0, goalsRepository.goalsFlow.value.size)
    }
}
