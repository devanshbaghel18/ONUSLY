package com.onusly.app

import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.UpdateGoalRequest
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class OnuslyApiClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: OnuslyApiClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = OnuslyApiClient()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun testGetGoals_Success() = runBlocking {
        val json = """
            [
                {
                    "id": "g-1",
                    "ownerId": "owner-1",
                    "title": "Goal 1",
                    "status": "active",
                    "blockedApps": ["com.insta.app"]
                }
            ]
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(json)
        )

        val baseUrl = server.url("/").toString()
        val result = client.getGoals(baseUrl, "test_jwt")

        val req = server.takeRequest()
        assertEquals("GET", req.method)
        assertEquals("/goals", req.path)
        assertEquals("Bearer test_jwt", req.getHeader("Authorization"))

        assertTrue(result.isSuccess)
        val goals = result.getOrThrow()
        assertEquals(1, goals.size)
        assertEquals("g-1", goals[0].id)
        assertEquals("Goal 1", goals[0].title)
    }

    @Test
    fun testCreateGoal_Success() = runBlocking {
        val responseJson = """
            {
                "id": "new-goal-id",
                "ownerId": "owner-1",
                "title": "Deep Work",
                "status": "active",
                "approvalType": "friend",
                "approverEmail": "friend@onusly.com",
                "blockedApps": ["com.twitter.android"]
            }
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody(responseJson)
        )

        val baseUrl = server.url("/").toString()
        val result = client.createGoal(
            baseUrl = baseUrl,
            jwtToken = "test_jwt",
            requestData = CreateGoalRequest(
                title = "Deep Work",
                approvalType = "friend",
                approverEmail = "friend@onusly.com",
                blockedApps = listOf("com.twitter.android")
            )
        )

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/goals", req.path)
        assertTrue(req.body.readUtf8().contains("Deep Work"))

        assertTrue(result.isSuccess)
        val goal = result.getOrThrow()
        assertEquals("new-goal-id", goal.id)
        assertEquals("Deep Work", goal.title)
        assertEquals("friend", goal.approvalType)
    }

    @Test
    fun testCreateGoal_AccountabilityRequired_Error() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(400)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"accountability_required"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = client.createGoal(
            baseUrl = baseUrl,
            jwtToken = "test_jwt",
            requestData = CreateGoalRequest(
                title = "Invalid Goal",
                approvalType = "none",
                blockedApps = listOf("com.insta.app")
            )
        )

        assertTrue(result.isFailure)
        val exception = result.exceptionOrNull() as OnuslyApiException
        assertEquals(400, exception.statusCode)
        assertEquals("accountability_required", exception.message)
    }

    @Test
    fun testUpdateGoal_TargetsLocked_Error() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"targets_locked"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = client.updateGoal(
            baseUrl = baseUrl,
            jwtToken = "test_jwt",
            goalId = "goal-1",
            requestData = UpdateGoalRequest(
                blockedApps = emptyList()
            )
        )

        assertTrue(result.isFailure)
        val exception = result.exceptionOrNull() as OnuslyApiException
        assertEquals(409, exception.statusCode)
        assertEquals("targets_locked", exception.message)
    }

    @Test
    fun testDeleteGoal_GoalLocked_Error() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"goal_locked"}""")
        )

        val baseUrl = server.url("/").toString()
        val result = client.deleteGoal(baseUrl, "test_jwt", "active-goal")

        assertTrue(result.isFailure)
        val exception = result.exceptionOrNull() as OnuslyApiException
        assertEquals(409, exception.statusCode)
        assertEquals("goal_locked", exception.message)
    }

    @Test
    fun testSubmitProof_Success() = runBlocking {
        val proofJson = """
            {
                "id": "p-1",
                "goalId": "goal-1",
                "ownerId": "owner-1",
                "proofType": "text",
                "textExplanation": "All done!"
            }
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody(proofJson)
        )

        val baseUrl = server.url("/").toString()
        val result = client.submitProof(
            baseUrl = baseUrl,
            jwtToken = "test_jwt",
            goalId = "goal-1",
            requestData = CreateProofRequest(
                proofType = "text",
                textExplanation = "All done!"
            )
        )

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/goals/goal-1/proofs", req.path)

        assertTrue(result.isSuccess)
        val proof = result.getOrThrow()
        assertEquals("p-1", proof.id)
        assertEquals("text", proof.proofType)
    }

    @Test
    fun testDecideApproval_Success() = runBlocking {
        val decisionJson = """
            {
                "id": "appr-1",
                "goalId": "goal-1",
                "proofId": "p-1",
                "ownerId": "owner-1",
                "approverId": "approver-1",
                "status": "approved",
                "comment": "Nice!"
            }
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody(decisionJson)
        )

        val baseUrl = server.url("/").toString()
        val result = client.decideApproval(
            baseUrl = baseUrl,
            jwtToken = "test_jwt",
            goalId = "goal-1",
            proofId = "p-1",
            requestData = ApprovalDecisionRequest(
                ownerId = "owner-1",
                status = "approved",
                comment = "Nice!"
            )
        )

        val req = server.takeRequest()
        assertEquals("POST", req.method)
        assertEquals("/goals/goal-1/proofs/p-1/approval", req.path)

        assertTrue(result.isSuccess)
        val decision = result.getOrThrow()
        assertEquals("approved", decision.status)
        assertEquals("Nice!", decision.comment)
    }

    @Test
    fun testGetMyProfile_Success() = runBlocking {
        val userJson = """
            {
                "id": "user-123",
                "email": "user@onusly.com",
                "handle": "my_handle",
                "name": "Alex"
            }
        """.trimIndent()

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(userJson)
        )

        val baseUrl = server.url("/").toString()
        val result = client.getMyProfile(baseUrl, "test_jwt")

        assertTrue(result.isSuccess)
        val user = result.getOrThrow()
        assertEquals("user-123", user.id)
        assertEquals("user@onusly.com", user.email)
        assertEquals("my_handle", user.handle)
    }
}
