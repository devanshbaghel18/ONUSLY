package com.onusly.app

import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.HeartbeatResult
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.UUID

class FleetHeartbeatTest {

    private lateinit var server: MockWebServer
    private lateinit var apiClient: EnforcementApiClient

    private lateinit var authPrefs: FakeSharedPreferences
    private lateinit var cachePrefs: FakeSharedPreferences
    private lateinit var settingsPrefs: FakeSharedPreferences
    private lateinit var authManager: AuthManager
    private lateinit var blocklistCache: SharedPreferencesBlocklistCache
    private lateinit var syncRepository: SyncRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        apiClient = EnforcementApiClient()

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
            apiClient = apiClient,
            settingsPrefs = settingsPrefs
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun testSendHeartbeat_200Success() = runBlocking {
        val testDeviceId = UUID.randomUUID().toString()
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "deviceId": "$testDeviceId",
                        "userId": "user-123",
                        "clientType": "android",
                        "platform": "android",
                        "clientVersion": "1.0.0",
                        "enforcing": true,
                        "blocklistVersion": "v1-0123456789abcdef",
                        "issues": [],
                        "status": "paired",
                        "healthStatus": "HEALTHY",
                        "lastHeartbeatAt": "2026-11-14T12:00:00Z"
                    }
                """.trimIndent())
        )

        val baseUrl = server.url("/").toString()
        val result = apiClient.sendHeartbeat(
            baseUrl = baseUrl,
            jwtToken = "jwt-valid-token",
            deviceId = testDeviceId,
            clientVersion = "1.0.0",
            enforcing = true,
            blocklistVersion = "v1-0123456789abcdef",
            issues = emptyList()
        )

        val recordedRequest = server.takeRequest()
        assertEquals("POST", recordedRequest.method)
        assertEquals("/enforcement/heartbeat", recordedRequest.path)
        assertEquals("Bearer jwt-valid-token", recordedRequest.getHeader("Authorization"))

        val bodyJson = JSONObject(recordedRequest.body.readUtf8())
        assertEquals(testDeviceId, bodyJson.getString("deviceId"))
        assertEquals("android", bodyJson.getString("clientType"))
        assertEquals("android", bodyJson.getString("platform"))
        assertEquals("1.0.0", bodyJson.getString("clientVersion"))
        assertTrue(bodyJson.getBoolean("enforcing"))
        assertEquals("v1-0123456789abcdef", bodyJson.getString("blocklistVersion"))
        assertEquals(0, bodyJson.getJSONArray("issues").length())
        assertTrue(bodyJson.getString("sentAt").isNotBlank())

        assertTrue(result is HeartbeatResult.Success)
        val success = result as HeartbeatResult.Success
        assertEquals(testDeviceId, success.deviceId)
    }

    @Test
    fun testSendHeartbeat_403DeviceRevoked() = runBlocking {
        val testDeviceId = UUID.randomUUID().toString()
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "error": "DEVICE_REVOKED",
                        "code": "device_revoked",
                        "message": "This device has been revoked by the account owner"
                    }
                """.trimIndent())
        )

        val baseUrl = server.url("/").toString()
        val result = apiClient.sendHeartbeat(
            baseUrl = baseUrl,
            jwtToken = "jwt-valid-token",
            deviceId = testDeviceId,
            clientVersion = "1.0.0",
            enforcing = true,
            blocklistVersion = "v1-0123456789abcdef",
            issues = emptyList()
        )

        assertTrue(result is HeartbeatResult.DeviceRevoked)
    }

    @Test
    fun testSendHeartbeat_WithIssues() = runBlocking {
        val testDeviceId = UUID.randomUUID().toString()
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"deviceId": "$testDeviceId", "status": "paired"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = apiClient.sendHeartbeat(
            baseUrl = baseUrl,
            jwtToken = "jwt-valid-token",
            deviceId = testDeviceId,
            clientVersion = "1.0.0",
            enforcing = false,
            blocklistVersion = "v1-0123456789abcdef",
            issues = listOf("accessibility_disabled")
        )

        val recordedRequest = server.takeRequest()
        val bodyJson = JSONObject(recordedRequest.body.readUtf8())
        val issuesArray = bodyJson.getJSONArray("issues")
        assertEquals(1, issuesArray.length())
        assertEquals("accessibility_disabled", issuesArray.getString(0))

        assertTrue(result is HeartbeatResult.Success)
    }

    @Test
    fun testSyncRepository_SendHeartbeat_DeviceRevokedClearsCacheAndNotifies() = runBlocking {
        // Sign in user
        authManager.saveSession(UserSession("user-target", "target@test.com", "Target User", "jwt-target-token"))

        // Populate active blocklist cache
        val initialBlocklist = BlocklistResponse(
            userId = "user-target",
            version = "v1-initial",
            generatedAt = "2026-11-14T10:00:00Z",
            targets = listOf(
                BlocklistTarget("goal-1", "Focus", "active", listOf("com.instagram.android"), emptyList())
            )
        )
        blocklistCache.saveCache(initialBlocklist, "user-target")
        syncRepository.refreshFlowFromCache()
        assertNotNull(syncRepository.getActiveCache())

        var signOutCalled = false
        syncRepository.addSignOutListener {
            signOutCalled = true
        }

        // Server responds with 403 DEVICE_REVOKED
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error": "DEVICE_REVOKED", "message": "Revoked"}""")
        )

        val result = syncRepository.sendHeartbeat()
        assertTrue(result is HeartbeatResult.DeviceRevoked)

        // Cache must be cleared immediately to avoid persistent lockout
        assertNull(syncRepository.getActiveCache())
        assertTrue(signOutCalled)
    }

    @Test
    fun testSyncRepository_NotSignedInReturnsError() = runBlocking {
        // Not signed in
        val result = syncRepository.sendHeartbeat()
        assertTrue(result is HeartbeatResult.Error)
        val err = result as HeartbeatResult.Error
        assertEquals(401, err.statusCode)
        assertEquals(0, server.requestCount)
    }
}

