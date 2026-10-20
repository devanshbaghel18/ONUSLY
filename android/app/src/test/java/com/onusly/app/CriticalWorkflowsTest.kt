package com.onusly.app

import android.content.Context
import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.BlocklistResponse
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.UpdateGoalRequest
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
import org.junit.Before
import org.junit.Test

class CriticalWorkflowsTest {

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

        // Set base URL to MockWebServer
        settingsPrefs.edit().putString(SyncRepository.KEY_BASE_URL, server.url("/").toString()).apply()

        // Construct AuthManager with fake prefs
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

    // --- 1. Signed-out vs Signed-in states ---

    @Test
    fun testSignedOutState_RejectsGoalsRepositoryOperationsWith401() = runBlocking {
        assertFalse(authManager.isSignedIn())

        val fetchResult = goalsRepository.fetchGoals()
        assertTrue(fetchResult.isFailure)
        val fetchErr = fetchResult.exceptionOrNull() as OnuslyApiException
        assertEquals(401, fetchErr.statusCode)

        val createResult = goalsRepository.createGoal(CreateGoalRequest(title = "Test"))
        assertTrue(createResult.isFailure)
        assertEquals(401, (createResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val updateResult = goalsRepository.updateGoal("g1", UpdateGoalRequest(title = "Updated"))
        assertTrue(updateResult.isFailure)
        assertEquals(401, (updateResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val deleteResult = goalsRepository.deleteGoal("g1")
        assertTrue(deleteResult.isFailure)
        assertEquals(401, (deleteResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val proofResult = goalsRepository.submitProof("g1", CreateProofRequest(proofType = "text", textExplanation = "Done"))
        assertTrue(proofResult.isFailure)
        assertEquals(401, (proofResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val decideResult = goalsRepository.decideApproval("g1", "p1", ApprovalDecisionRequest(ownerId = "o1", status = "approved"))
        assertTrue(decideResult.isFailure)
        assertEquals(401, (decideResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val profileResult = goalsRepository.getMyProfile()
        assertTrue(profileResult.isFailure)
        assertEquals(401, (profileResult.exceptionOrNull() as OnuslyApiException).statusCode)

        val handleResult = goalsRepository.updateHandle("new_handle")
        assertTrue(handleResult.isFailure)
        assertEquals(401, (handleResult.exceptionOrNull() as OnuslyApiException).statusCode)
    }

    @Test
    fun testSignedInState_CarriesAuthorizationHeader() = runBlocking {
        authManager.saveSession(
            UserSession(
                userId = "user_123",
                email = "user@onusly.com",
                name = "Test User",
                jwtToken = "valid_jwt_token"
            )
        )
        assertTrue(authManager.isSignedIn())

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("[]")
        )

        val result = goalsRepository.fetchGoals()
        assertTrue(result.isSuccess)

        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/goals", req.path)
        assertEquals("Bearer valid_jwt_token", req.getHeader("Authorization"))
    }

    // --- 2. Loading goals & handling API errors ---

    @Test
    fun testLoadGoals_Api500Error_DoesNotCorruptLocalFlow() = runBlocking {
        authManager.saveSession(
            UserSession(
                userId = "user_123",
                email = "user@onusly.com",
                name = "Test User",
                jwtToken = "jwt_abc"
            )
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(500)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"internal server error"}""")
        )

        val result = goalsRepository.fetchGoals()
        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(500, err.statusCode)
        assertEquals("internal server error", err.message)

        // Local state remains pristine empty
        assertEquals(0, goalsRepository.goalsFlow.value.size)
    }

    // --- 3. Failed blocklist sync after successful goal mutation ---

    @Test
    fun testFailedBlocklistSyncAfterSuccessfulGoalCreation() = runBlocking {
        authManager.saveSession(
            UserSession(
                userId = "user_123",
                email = "user@onusly.com",
                name = "Test User",
                jwtToken = "jwt_abc"
            )
        )

        // 1. Goal creation succeeds on backend
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "goal-created-1",
                        "ownerId": "user_123",
                        "title": "Study Go",
                        "status": "active",
                        "approvalType": "friend",
                        "approverEmail": "friend@onusly.com",
                        "blockedApps": ["com.twitter.android"]
                    }
                """.trimIndent())
        )

        // 2. Immediate blocklist sync attempt fails with 500
        server.enqueue(
            MockResponse()
                .setResponseCode(500)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"database unavailable"}""")
        )

        val req = CreateGoalRequest(
            title = "Study Go",
            approvalType = "friend",
            approverEmail = "friend@onusly.com",
            blockedApps = listOf("com.twitter.android")
        )

        val result = goalsRepository.createGoal(req)

        // Goal creation must succeed from user's perspective, but with syncSucceeded = false
        assertTrue(result.isSuccess)
        val mutation = result.getOrThrow()
        val goal = mutation.value
        assertEquals("goal-created-1", goal.id)
        assertFalse(mutation.syncSucceeded)
        assertNotNull(mutation.syncError)
        assertTrue(mutation.syncError!!.contains("database unavailable"))

        // Goal is recorded in local goals flow
        assertEquals(1, goalsRepository.goalsFlow.value.size)
        assertEquals("goal-created-1", goalsRepository.goalsFlow.value[0].id)

        // Verify both requests reached the server
        val goalReq = server.takeRequest()
        assertEquals("POST", goalReq.method)
        assertEquals("/goals", goalReq.path)

        val syncReq = server.takeRequest()
        assertEquals("GET", syncReq.method)
        assertEquals("/enforcement/blocklist", syncReq.path)
    }

    @Test
    fun testRetryBlocklistSync_SucceedsAfterInitialFailure() = runBlocking {
        authManager.saveSession(
            UserSession(
                userId = "user_123",
                email = "user@onusly.com",
                name = "Test User",
                jwtToken = "jwt_abc"
            )
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "userId": "user_123",
                        "version": "v_synced",
                        "generatedAt": "2026-10-20T00:00:00Z",
                        "targets": [
                            {
                                "goalId": "goal-1",
                                "title": "Study Go",
                                "status": "active",
                                "apps": ["com.twitter.android"],
                                "domains": []
                            }
                        ]
                    }
                """.trimIndent())
        )

        val retryResult = goalsRepository.retryBlocklistSync()
        assertTrue(retryResult.isSuccess)
        val cache = blocklistCache.getCache("user_123")
        assertNotNull(cache)
        assertEquals("v_synced", cache?.version)
        assertEquals(1, cache?.targets?.size)
        assertEquals("com.twitter.android", cache?.targets?.get(0)?.apps?.get(0))
    }

    // --- 4. Logout followed by login as different account ---

    @Test
    fun testLogoutFollowedByLoginAsDifferentAccount_CacheIsolation() = runBlocking {
        // User Alice signs in
        authManager.saveSession(
            UserSession(userId = "user_alice", email = "alice@onusly.com", name = "Alice", jwtToken = "jwt_alice")
        )
        val aliceResponse = BlocklistResponse(
            userId = "user_alice",
            version = "v1-alice",
            generatedAt = "2026-10-19T10:00:00Z",
            targets = listOf(BlocklistTarget("g_alice", "Alice Goal", "active", listOf("com.insta.app"), emptyList()))
        )
        assertTrue(blocklistCache.saveCache(aliceResponse, "user_alice"))
        assertNotNull(blocklistCache.getCache("user_alice"))

        // Alice signs out
        syncRepository.onUserSignOut()
        goalsRepository.clearLocalData()

        assertFalse(authManager.isSignedIn())
        assertNull(blocklistCache.getCache())
        assertNull(blocklistCache.getCache("user_alice"))
        assertEquals(0, goalsRepository.goalsFlow.value.size)

        // User Bob signs in
        authManager.saveSession(
            UserSession(userId = "user_bob", email = "bob@onusly.com", name = "Bob", jwtToken = "jwt_bob")
        )
        assertTrue(authManager.isSignedIn())

        // Bob cannot read Alice's cache
        assertNull(blocklistCache.getCache("user_bob"))

        // Bob saves his own cache
        val bobResponse = BlocklistResponse(
            userId = "user_bob",
            version = "v1-bob",
            generatedAt = "2026-10-19T10:05:00Z",
            targets = listOf(BlocklistTarget("g_bob", "Bob Goal", "active", listOf("com.reddit.app"), emptyList()))
        )
        assertTrue(blocklistCache.saveCache(bobResponse, "user_bob"))

        val bobCache = blocklistCache.getCache("user_bob")
        assertNotNull(bobCache)
        assertEquals("user_bob", bobCache!!.userId)
        assertEquals(1, bobCache.targets.size)
        assertEquals("com.reddit.app", bobCache.targets[0].apps[0])

        // Verify Bob's cache does not contain Alice's targets
        assertEquals("g_bob", bobCache.targets[0].goalId)
    }

    // --- 5. Server rejection of invalid goal mutations ---

    @Test
    fun testServerRejections_ErrorMapping() = runBlocking {
        authManager.saveSession(
            UserSession(userId = "user_1", email = "u1@onusly.com", name = "User 1", jwtToken = "token_1")
        )

        // 1. 400 accountability_required on create
        server.enqueue(
            MockResponse()
                .setResponseCode(400)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"accountability_required"}""")
        )
        val createResult = goalsRepository.createGoal(
            CreateGoalRequest(title = "No Approver", approvalType = "none", blockedApps = listOf("com.bad.app"))
        )
        assertTrue(createResult.isFailure)
        val errCreate = createResult.exceptionOrNull() as OnuslyApiException
        assertEquals(400, errCreate.statusCode)
        assertEquals("accountability_required", errCreate.message)

        // 2. 409 targets_locked on update
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"targets_locked"}""")
        )
        val updateResult = goalsRepository.updateGoal("g1", UpdateGoalRequest(blockedApps = emptyList()))
        assertTrue(updateResult.isFailure)
        val errUpdate = updateResult.exceptionOrNull() as OnuslyApiException
        assertEquals(409, errUpdate.statusCode)
        assertEquals("targets_locked", errUpdate.message)

        // 3. 409 goal_locked on delete
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"goal_locked"}""")
        )
        val deleteResult = goalsRepository.deleteGoal("active_targeted_goal")
        assertTrue(deleteResult.isFailure)
        val errDelete = deleteResult.exceptionOrNull() as OnuslyApiException
        assertEquals(409, errDelete.statusCode)
        assertEquals("goal_locked", errDelete.message)

        // 4. 403 Forbidden on unauthorized approval decision
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"forbidden: caller is not the assigned approver"}""")
        )
        val decideResult = goalsRepository.decideApproval(
            "g1", "p1", ApprovalDecisionRequest(ownerId = "other_user", status = "approved")
        )
        assertTrue(decideResult.isFailure)
        val errDecide = decideResult.exceptionOrNull() as OnuslyApiException
        assertEquals(403, errDecide.statusCode)
        assertTrue(errDecide.message.contains("forbidden"))

        // 5. 409 Conflict on duplicate approval decision
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"proof has already been decided"}""")
        )
        val dupResult = goalsRepository.decideApproval(
            "g1", "p1", ApprovalDecisionRequest(ownerId = "u1", status = "approved")
        )
        assertTrue(dupResult.isFailure)
        val errDup = dupResult.exceptionOrNull() as OnuslyApiException
        assertEquals(409, errDup.statusCode)
        assertEquals("proof has already been decided", errDup.message)
    }

    // --- 6. Goal model status & target helpers ---

    @Test
    fun testGoalStatusHelpers() {
        val activeTargeted = Goal(
            id = "1", ownerId = "o", title = "Active Targeted", status = "active",
            blockedApps = listOf("com.app"), blockedDomains = emptyList()
        )
        assertTrue(activeTargeted.isActive)
        assertFalse(activeTargeted.isProofSubmitted)
        assertFalse(activeTargeted.isCompleted)
        assertTrue(activeTargeted.isTargeted)

        val inReview = Goal(
            id = "2", ownerId = "o", title = "In Review", status = "proof_submitted",
            blockedApps = emptyList(), blockedDomains = listOf("app.com")
        )
        assertFalse(inReview.isActive)
        assertTrue(inReview.isProofSubmitted)
        assertFalse(inReview.isCompleted)
        assertTrue(inReview.isTargeted)

        val completedNoTargets = Goal(
            id = "3", ownerId = "o", title = "Completed", status = "completed",
            blockedApps = emptyList(), blockedDomains = emptyList()
        )
        assertFalse(completedNoTargets.isActive)
        assertFalse(completedNoTargets.isProofSubmitted)
        assertTrue(completedNoTargets.isCompleted)
        assertFalse(completedNoTargets.isTargeted)
    }
}
