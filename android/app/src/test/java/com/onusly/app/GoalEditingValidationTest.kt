package com.onusly.app

import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.UpdateGoalRequest
import com.onusly.app.data.model.UserSession
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class GoalEditingValidationTest {

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

        authManager.saveSession(
            UserSession(
                userId = "user_owner",
                email = "owner@onusly.com",
                name = "Owner",
                jwtToken = "jwt_owner_token"
            )
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    // --- 1. Target Accumulation (Add-Only Editing) ---

    @Test
    fun testGoalEditing_AccumulatesTargetsWithoutRemovingExisting() = runBlocking {
        val existingGoal = Goal(
            id = "goal_100",
            ownerId = "user_owner",
            title = "Original Title",
            description = "Original Description",
            status = "active",
            approvalType = "friend",
            approverEmail = "friend@onusly.com",
            blockedApps = listOf("com.twitter.android"),
            blockedDomains = listOf("x.com")
        )

        // Simulated user selection: existing apps preserved + new app added
        val newlySelectedApps = setOf("com.instagram.android")
        val newlyAddedDomains = setOf("instagram.com")

        val mergedApps = (existingGoal.blockedApps.toSet() + newlySelectedApps).toList()
        val mergedDomains = (existingGoal.blockedDomains.toSet() + newlyAddedDomains).toList()

        assertEquals(listOf("com.twitter.android", "com.instagram.android"), mergedApps)
        assertEquals(listOf("x.com", "instagram.com"), mergedDomains)

        // Mock PATCH response returning updated goal
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "goal_100",
                        "ownerId": "user_owner",
                        "title": "Updated Title",
                        "description": "Updated Description",
                        "status": "active",
                        "approvalType": "friend",
                        "approverEmail": "friend@onusly.com",
                        "blockedApps": ["com.twitter.android", "com.instagram.android"],
                        "blockedDomains": ["x.com", "instagram.com"]
                    }
                """.trimIndent())
        )

        // Mock blocklist sync response following mutation
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "userId": "user_owner",
                        "version": "v2",
                        "generatedAt": "2026-10-20T00:00:00Z",
                        "targets": [
                            {
                                "goalId": "goal_100",
                                "title": "Updated Title",
                                "status": "active",
                                "apps": ["com.twitter.android", "com.instagram.android"],
                                "domains": ["x.com", "instagram.com"]
                            }
                        ]
                    }
                """.trimIndent())
        )

        val updateReq = UpdateGoalRequest(
            title = "Updated Title",
            description = "Updated Description",
            blockedApps = mergedApps,
            blockedDomains = mergedDomains
        )

        val result = goalsRepository.updateGoal(existingGoal.id, updateReq)
        assertTrue(result.isSuccess)

        val mutation = result.getOrThrow()
        assertEquals("goal_100", mutation.value.id)
        assertEquals(2, mutation.value.blockedApps.size)
        assertTrue(mutation.value.blockedApps.contains("com.twitter.android"))
        assertTrue(mutation.value.blockedApps.contains("com.instagram.android"))
        assertTrue(mutation.syncSucceeded)

        val patchRequest = server.takeRequest()
        assertEquals("PATCH", patchRequest.method)
        assertEquals("/goals/goal_100", patchRequest.path)

        // Verify JSON payload sent in PATCH
        val payload = JSONObject(patchRequest.body.readUtf8())
        assertEquals("Updated Title", payload.getString("title"))
        assertEquals("Updated Description", payload.getString("description"))
        val appsJson = payload.getJSONArray("blockedApps")
        assertEquals(2, appsJson.length())
        assertEquals("com.twitter.android", appsJson.getString(0))
        assertEquals("com.instagram.android", appsJson.getString(1))

        // Verify status cannot be sent in PATCH body
        assertFalse(payload.has("status"))
    }

    // --- 2. Add-Only Target Removal Rejection (409 targets_locked) ---

    @Test
    fun testGoalEditing_BackendRejectsTargetRemovalWith409() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"targets_locked"}""")
        )

        // Attempting to reduce targets from ["com.twitter.android"] to []
        val updateReq = UpdateGoalRequest(
            title = "Reduced targets",
            blockedApps = emptyList()
        )

        val result = goalsRepository.updateGoal("goal_100", updateReq)
        assertTrue(result.isFailure)

        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(409, err.statusCode)
        assertEquals("targets_locked", err.message)
    }

    // --- 3. Accountability Required on Targeted Goal (400 accountability_required) ---

    @Test
    fun testGoalCreation_DistractionTargetsRequireFriendApprover() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(400)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"accountability_required"}""")
        )

        val createReq = CreateGoalRequest(
            title = "Targeted without approver",
            approvalType = "none",
            approverEmail = null,
            blockedApps = listOf("com.instagram.android")
        )

        val result = goalsRepository.createGoal(createReq)
        assertTrue(result.isFailure)

        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(400, err.statusCode)
        assertEquals("accountability_required", err.message)
    }

    // --- 4. Active Targeted Goal Deletion Guard (409 goal_locked) ---

    @Test
    fun testActiveTargetedGoalDeletion_ClientGuardAndServer409() = runBlocking {
        val activeTargetedGoal = Goal(
            id = "goal_active_targeted",
            ownerId = "user_owner",
            title = "Active and Targeted",
            status = "active",
            approvalType = "friend",
            approverEmail = "friend@onusly.com",
            blockedApps = listOf("com.youtube.android")
        )

        // Verify client-side property checks
        assertTrue(activeTargetedGoal.isActive)
        assertTrue(activeTargetedGoal.isTargeted)

        // Backend will return 409 goal_locked if delete is attempted
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"goal_locked"}""")
        )

        val deleteResult = goalsRepository.deleteGoal(activeTargetedGoal.id)
        assertTrue(deleteResult.isFailure)

        val err = deleteResult.exceptionOrNull() as OnuslyApiException
        assertEquals(409, err.statusCode)
        assertEquals("goal_locked", err.message)
    }

    // --- 5. Untargeted Goal Deletion Allowed ---

    @Test
    fun testUntargetedGoalDeletion_Succeeds() = runBlocking {
        val untargetedGoal = Goal(
            id = "goal_untargeted",
            ownerId = "user_owner",
            title = "Read 10 Books",
            status = "active",
            approvalType = "none",
            blockedApps = emptyList(),
            blockedDomains = emptyList()
        )

        assertFalse(untargetedGoal.isTargeted)

        server.enqueue(
            MockResponse()
                .setResponseCode(204)
        )

        // Blocklist sync succeeds
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "userId": "user_owner",
                        "version": "v3",
                        "generatedAt": "2026-10-20T00:00:00Z",
                        "targets": []
                    }
                """.trimIndent())
        )

        val deleteResult = goalsRepository.deleteGoal(untargetedGoal.id)
        assertTrue(deleteResult.isSuccess)
        val mutation = deleteResult.getOrThrow()
        assertTrue(mutation.syncSucceeded)
    }
}
