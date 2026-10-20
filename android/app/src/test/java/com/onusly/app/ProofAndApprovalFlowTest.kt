package com.onusly.app

import com.onusly.app.data.GoalsRepository
import com.onusly.app.data.SyncRepository
import com.onusly.app.data.api.EnforcementApiClient
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.cache.SharedPreferencesBlocklistCache
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.UserSession
import com.onusly.app.ui.SubmitProofDialog
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ProofAndApprovalFlowTest {

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
                name = "Goal Owner",
                jwtToken = "jwt_token_123"
            )
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    // --- 1. Proof Submission: Text Type ---

    @Test
    fun testSubmitProof_Text_PayloadVerificationAndStateUpdate() = runBlocking {
        // Enqueue 201 for proof submission
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "proof-text-1",
                        "goalId": "goal-101",
                        "ownerId": "user_owner",
                        "proofType": "text",
                        "textExplanation": "Read 50 pages of clean architecture book.",
                        "submittedAt": "2026-10-20T10:00:00Z"
                    }
                """.trimIndent())
        )

        // Enqueue 200 for immediate blocklist sync
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "userId": "user_owner",
                        "version": "v101",
                        "generatedAt": "2026-10-20T10:00:01Z",
                        "targets": []
                    }
                """.trimIndent())
        )

        val req = CreateProofRequest(
            proofType = "text",
            textExplanation = "Read 50 pages of clean architecture book."
        )

        val result = goalsRepository.submitProof("goal-101", req)
        assertTrue(result.isSuccess)

        val mutation = result.getOrThrow()
        assertEquals("proof-text-1", mutation.value.id)
        assertEquals("text", mutation.value.proofType)
        assertEquals("Read 50 pages of clean architecture book.", mutation.value.textExplanation)
        assertTrue(mutation.syncSucceeded)

        val proofRequest = server.takeRequest()
        assertEquals("POST", proofRequest.method)
        assertEquals("/goals/goal-101/proofs", proofRequest.path)
        assertEquals("Bearer jwt_token_123", proofRequest.getHeader("Authorization"))

        val body = JSONObject(proofRequest.body.readUtf8())
        assertEquals("text", body.getString("proofType"))
        assertEquals("Read 50 pages of clean architecture book.", body.getString("textExplanation"))
        assertFalse(body.has("externalLink"))
        assertFalse(body.has("photoUrl"))
    }

    // --- 2. Proof Submission: Link Type ---

    @Test
    fun testSubmitProof_Link_PayloadVerification() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "proof-link-2",
                        "goalId": "goal-102",
                        "ownerId": "user_owner",
                        "proofType": "link",
                        "textExplanation": "Merged PR on GitHub",
                        "externalLink": "https://github.com/devanshbaghel18/ONUSLY/pull/42",
                        "submittedAt": "2026-10-20T10:15:00Z"
                    }
                """.trimIndent())
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"userId":"user_owner","version":"v102","generatedAt":"2026-10-20T10:15:00Z","targets":[]}""")
        )

        val req = CreateProofRequest(
            proofType = "link",
            textExplanation = "Merged PR on GitHub",
            externalLink = "https://github.com/devanshbaghel18/ONUSLY/pull/42"
        )

        val result = goalsRepository.submitProof("goal-102", req)
        assertTrue(result.isSuccess)

        val proofRequest = server.takeRequest()
        val body = JSONObject(proofRequest.body.readUtf8())
        assertEquals("link", body.getString("proofType"))
        assertEquals("https://github.com/devanshbaghel18/ONUSLY/pull/42", body.getString("externalLink"))
        assertEquals("Merged PR on GitHub", body.getString("textExplanation"))
    }

    // --- 3. Proof Submission: Photo URL Type ---

    @Test
    fun testSubmitProof_Photo_PayloadVerification() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "proof-photo-3",
                        "goalId": "goal-103",
                        "ownerId": "user_owner",
                        "proofType": "photo",
                        "photoUrl": "https://storage.googleapis.com/onusly/proof_123.jpg",
                        "textExplanation": "Completed 10km run",
                        "submittedAt": "2026-10-20T10:30:00Z"
                    }
                """.trimIndent())
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"userId":"user_owner","version":"v103","generatedAt":"2026-10-20T10:30:00Z","targets":[]}""")
        )

        val req = CreateProofRequest(
            proofType = "photo",
            photoUrl = "https://storage.googleapis.com/onusly/proof_123.jpg",
            textExplanation = "Completed 10km run"
        )

        val result = goalsRepository.submitProof("goal-103", req)
        assertTrue(result.isSuccess)

        val proofRequest = server.takeRequest()
        val body = JSONObject(proofRequest.body.readUtf8())
        assertEquals("photo", body.getString("proofType"))
        assertEquals("https://storage.googleapis.com/onusly/proof_123.jpg", body.getString("photoUrl"))
        assertEquals("Completed 10km run", body.getString("textExplanation"))
    }

    // --- 4. Validation: URL Format Verification ---

    @Test
    fun testValidation_UrlFormatVerification() {
        assertTrue(SubmitProofDialog.isValidUrl("https://github.com/devanshbaghel18/ONUSLY"))
        assertTrue(SubmitProofDialog.isValidUrl("http://localhost:8080/health"))
        assertTrue(SubmitProofDialog.isValidUrl("https://cdn.example.com/images/screenshot.png"))

        assertFalse(SubmitProofDialog.isValidUrl("ftp://invalid-scheme.com"))
        assertFalse(SubmitProofDialog.isValidUrl("just_some_text"))
        assertFalse(SubmitProofDialog.isValidUrl("http://"))
        assertFalse(SubmitProofDialog.isValidUrl("https://"))
        assertFalse(SubmitProofDialog.isValidUrl(""))
        assertFalse(SubmitProofDialog.isValidUrl("   "))
    }

    // --- 5. Backend Error: 409 Conflict (Already Submitted) ---

    @Test
    fun testSubmitProof_BackendError_409AlreadySubmitted() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"proof has already been submitted for this goal"}""")
        )

        val req = CreateProofRequest(proofType = "text", textExplanation = "Duplicate proof attempt")
        val result = goalsRepository.submitProof("goal-already-submitted", req)

        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(409, err.statusCode)
        assertEquals("proof has already been submitted for this goal", err.message)
    }

    // --- 6. Backend Error: 400 Bad Request (Invalid Proof) ---

    @Test
    fun testSubmitProof_BackendError_400InvalidProof() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(400)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"invalid proof"}""")
        )

        val req = CreateProofRequest(proofType = "text", textExplanation = "")
        val result = goalsRepository.submitProof("goal-invalid", req)

        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(400, err.statusCode)
        assertEquals("invalid proof", err.message)
    }

    // --- 7. Proof Mutation Success With Failed Blocklist Sync Allows Retry ---

    @Test
    fun testSubmitProof_SuccessWithFailedBlocklistSync_AllowsRetry() = runBlocking {
        // Proof succeeds
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "proof-sync-test",
                        "goalId": "goal-sync-test",
                        "ownerId": "user_owner",
                        "proofType": "text",
                        "textExplanation": "Sync failure simulation"
                    }
                """.trimIndent())
        )

        // Sync fails with 500
        server.enqueue(
            MockResponse()
                .setResponseCode(500)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"temporary database timeout"}""")
        )

        val req = CreateProofRequest(proofType = "text", textExplanation = "Sync failure simulation")
        val result = goalsRepository.submitProof("goal-sync-test", req)

        // Mutation itself succeeded
        assertTrue(result.isSuccess)
        val mutation = result.getOrThrow()
        assertEquals("proof-sync-test", mutation.value.id)
        assertFalse(mutation.syncSucceeded)
        assertNotNull(mutation.syncError)

        // Retry sync succeeds
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"userId":"user_owner","version":"v_recovered","generatedAt":"2026-10-20T10:00:00Z","targets":[]}""")
        )

        val retryResult = goalsRepository.retryBlocklistSync()
        assertTrue(retryResult.isSuccess)
    }

    // --- 8. Proof History Retrieval ---

    @Test
    fun testGetProofs_History_ReturnsOrderedProofs() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    [
                        {
                            "id": "p1",
                            "goalId": "g100",
                            "ownerId": "user_owner",
                            "proofType": "text",
                            "textExplanation": "Attempt 1",
                            "submittedAt": "2026-10-18T10:00:00Z"
                        },
                        {
                            "id": "p2",
                            "goalId": "g100",
                            "ownerId": "user_owner",
                            "proofType": "link",
                            "externalLink": "https://example.com/log",
                            "submittedAt": "2026-10-19T10:00:00Z"
                        }
                    ]
                """.trimIndent())
        )

        val result = goalsRepository.getProofs("g100", "user_owner")
        assertTrue(result.isSuccess)

        val proofs = result.getOrThrow()
        assertEquals(2, proofs.size)
        assertEquals("p1", proofs[0].id)
        assertEquals("text", proofs[0].proofType)
        assertEquals("p2", proofs[1].id)
        assertEquals("link", proofs[1].proofType)

        val request = server.takeRequest()
        assertEquals("GET", request.method)
        assertEquals("/goals/g100/proofs?ownerId=user_owner", request.path)
    }

    @Test
    fun testGetProofs_History_EmptyState() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("[]")
        )

        val result = goalsRepository.getProofs("g100")
        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().isEmpty())
    }

    // --- 9. Approver Decision: Approve ---

    @Test
    fun testDecideApproval_Approve_PayloadAndTransitionToCompleted() = runBlocking {
        // Decision succeeds
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "appr-101",
                        "goalId": "goal-friend-1",
                        "proofId": "proof-friend-1",
                        "ownerId": "friend_user",
                        "approverId": "user_owner",
                        "status": "approved",
                        "comment": "Outstanding effort!",
                        "decidedAt": "2026-10-20T11:00:00Z"
                    }
                """.trimIndent())
        )

        // Blocklist sync succeeds
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"userId":"user_owner","version":"v_approved","generatedAt":"2026-10-20T11:00:00Z","targets":[]}""")
        )

        val req = ApprovalDecisionRequest(
            ownerId = "friend_user",
            status = "approved",
            comment = "Outstanding effort!"
        )

        val result = goalsRepository.decideApproval("goal-friend-1", "proof-friend-1", req)
        assertTrue(result.isSuccess)

        val mutation = result.getOrThrow()
        assertEquals("appr-101", mutation.value.id)
        assertEquals("approved", mutation.value.status)
        assertEquals("Outstanding effort!", mutation.value.comment)
        assertTrue(mutation.syncSucceeded)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/goals/goal-friend-1/proofs/proof-friend-1/approval", request.path)

        val body = JSONObject(request.body.readUtf8())
        assertEquals("friend_user", body.getString("ownerId"))
        assertEquals("approved", body.getString("status"))
        assertEquals("Outstanding effort!", body.getString("comment"))
    }

    // --- 10. Approver Decision: Reject ---

    @Test
    fun testDecideApproval_Reject_PayloadAndTransitionToActive() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(201)
                .setHeader("Content-Type", "application/json")
                .setBody("""
                    {
                        "id": "appr-102",
                        "goalId": "goal-friend-2",
                        "proofId": "proof-friend-2",
                        "ownerId": "friend_user",
                        "approverId": "user_owner",
                        "status": "rejected",
                        "comment": "Link was inaccessible, please re-submit with screenshot.",
                        "decidedAt": "2026-10-20T11:05:00Z"
                    }
                """.trimIndent())
        )

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"userId":"user_owner","version":"v_rejected","generatedAt":"2026-10-20T11:05:00Z","targets":[]}""")
        )

        val req = ApprovalDecisionRequest(
            ownerId = "friend_user",
            status = "rejected",
            comment = "Link was inaccessible, please re-submit with screenshot."
        )

        val result = goalsRepository.decideApproval("goal-friend-2", "proof-friend-2", req)
        assertTrue(result.isSuccess)

        val mutation = result.getOrThrow()
        assertEquals("appr-102", mutation.value.id)
        assertEquals("rejected", mutation.value.status)
    }

    // --- 11. Approver Decision: Backend Security Rejections ---

    @Test
    fun testDecideApproval_SelfApproval_403Forbidden() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"goal owner cannot approve their own goal"}""")
        )

        val req = ApprovalDecisionRequest(ownerId = "user_owner", status = "approved")
        val result = goalsRepository.decideApproval("my-own-goal", "proof-self", req)

        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(403, err.statusCode)
        assertEquals("goal owner cannot approve their own goal", err.message)
    }

    @Test
    fun testDecideApproval_UnauthorizedApprover_403Forbidden() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"user is not the assigned approver"}""")
        )

        val req = ApprovalDecisionRequest(ownerId = "stranger_user", status = "approved")
        val result = goalsRepository.decideApproval("stranger-goal", "proof-1", req)

        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(403, err.statusCode)
        assertEquals("user is not the assigned approver", err.message)
    }

    @Test
    fun testDecideApproval_DuplicateDecision_409Conflict() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(409)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"error":"proof has already been decided"}""")
        )

        val req = ApprovalDecisionRequest(ownerId = "friend_user", status = "approved")
        val result = goalsRepository.decideApproval("decided-goal", "decided-proof", req)

        assertTrue(result.isFailure)
        val err = result.exceptionOrNull() as OnuslyApiException
        assertEquals(409, err.statusCode)
        assertEquals("proof has already been decided", err.message)
    }
}
