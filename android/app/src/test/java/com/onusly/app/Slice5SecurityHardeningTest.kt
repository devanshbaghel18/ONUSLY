package com.onusly.app

import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class Slice5SecurityHardeningTest {

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

    @Test
    fun testSetBaseUrl_DifferentUrl_AutomaticallyClearsCredentialsCacheAndGoals() = runBlocking {
        signInAs("USER#alice", "alice@example.com")
        assertTrue(authManager.isSignedIn())

        // Populate blocklist cache
        val blocklist = BlocklistResponse(
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
        assertTrue(blocklistCache.saveCache(blocklist, "USER#alice"))
        assertEquals("USER#alice", blocklistCache.getRawCachedUserId())

        // Populate in-memory goals
        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            [
                {
                    "id": "g1",
                    "title": "Study Go",
                    "status": "active",
                    "blockedApps": ["com.instagram.android"]
                }
            ]
        """.trimIndent()))
        val goalsRes = goalsRepository.fetchGoals()
        assertTrue(goalsRes.isSuccess)
        assertEquals(1, goalsRepository.goalsFlow.value.size)

        // Change base URL to a different server
        val newServerUrl = "http://192.168.1.100:8080"
        syncRepository.setBaseUrl(newServerUrl)

        // Verify base URL is updated
        assertEquals(newServerUrl, syncRepository.getBaseUrl())

        // Verify credentials and session are invalidated
        assertFalse(authManager.isSignedIn())
        assertNull(authManager.getSession())
        assertNull(authManager.getUserId())
        assertNull(authManager.getJwtToken())

        // Verify on-disk and in-memory blocklists are invalidated
        assertNull(syncRepository.getActiveCache())
        assertNull(blocklistCache.getRawCachedUserId())
        assertNull(blocklistCache.getCache("USER#alice"))
        assertNull(syncRepository.cachedBlocklistFlow.value)

        // Verify in-memory goals are wiped via sign-out listener
        assertEquals(0, goalsRepository.goalsFlow.value.size)
        assertFalse(goalsRepository.isOfflineFlow.value)
    }

    @Test
    fun testSetBaseUrl_SameUrl_PreservesActiveSessionAndCache() {
        signInAs("USER#alice", "alice@example.com")
        val currentUrl = syncRepository.getBaseUrl()

        // Setting same URL should not trigger destructive logout
        syncRepository.setBaseUrl(currentUrl)

        assertTrue(authManager.isSignedIn())
        assertEquals("USER#alice", authManager.getUserId())
    }

    @Test
    fun testInFlightNetworkRequest_AfterLogoutOrAccountSwitch_DoesNotLeakStaleData() = runBlocking {
        signInAs("USER#alice", "alice@example.com")

        // Enqueue delayed response
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""
                    [
                        {
                            "id": "alice-goal-99",
                            "title": "Alice Secret Goal",
                            "status": "active"
                        }
                    ]
                """.trimIndent())
                .setBodyDelay(200, java.util.concurrent.TimeUnit.MILLISECONDS)
        )

        // Launch fetch in background
        val fetchJob = async(Dispatchers.IO) {
            goalsRepository.fetchGoals()
        }

        // Before the network response finishes, Alice logs out
        delay(50)
        syncRepository.onUserSignOut()

        // Verify session was cleared
        assertFalse(authManager.isSignedIn())
        assertEquals(0, goalsRepository.goalsFlow.value.size)

        // Wait for network response to complete
        fetchJob.await()

        // Guard must prevent stale data from repopulating goalsFlow
        assertEquals(0, goalsRepository.goalsFlow.value.size)
    }

    @Test
    fun testMultiAccountIsolation_NoStaleDataLeakedBetweenAccounts() = runBlocking {
        // 1. Alice logs in, saves cache and goals
        signInAs("USER#alice", "alice@example.com")
        server.enqueue(MockResponse().setResponseCode(200).setBody("""
            [
                {"id": "alice-goal", "title": "Alice Study", "status": "active"}
            ]
        """.trimIndent()))
        goalsRepository.fetchGoals()
        assertEquals(1, goalsRepository.goalsFlow.value.size)

        val aliceBlocklist = BlocklistResponse(
            userId = "USER#alice",
            version = "W/\"v1\"",
            generatedAt = "2026-10-20T00:00:00Z",
            targets = listOf(BlocklistTarget("alice-goal", "Alice Study", "active", listOf("com.insta"), emptyList()))
        )
        blocklistCache.saveCache(aliceBlocklist, "USER#alice")

        // 2. Alice signs out
        syncRepository.onUserSignOut()
        assertEquals(0, goalsRepository.goalsFlow.value.size)
        assertNull(blocklistCache.getRawCachedUserId())

        // 3. Bob logs in
        signInAs("USER#bob", "bob@example.com")
        assertEquals("USER#bob", authManager.getUserId())

        // Verify Bob cannot read Alice's blocklist
        assertNull(syncRepository.getActiveCache())
        assertNull(blocklistCache.getCache("USER#bob"))

        // Bob saves their own blocklist
        val bobBlocklist = BlocklistResponse(
            userId = "USER#bob",
            version = "W/\"v2\"",
            generatedAt = "2026-10-20T01:00:00Z",
            targets = listOf(BlocklistTarget("bob-goal", "Bob Fitness", "active", listOf("com.tiktok"), emptyList()))
        )
        val bobSaved = blocklistCache.saveCache(bobBlocklist, "USER#bob")
        assertTrue(bobSaved)
        assertEquals("USER#bob", blocklistCache.getRawCachedUserId())
        assertEquals("USER#bob", syncRepository.getActiveCache()?.userId)
    }
}
