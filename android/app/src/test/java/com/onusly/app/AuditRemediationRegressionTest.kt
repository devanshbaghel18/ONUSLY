package com.onusly.app

import com.onusly.app.data.api.ApprovalResponse
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import com.onusly.app.engine.BlockerEngine
import com.onusly.app.engine.InstalledAppsManager
import com.onusly.app.ui.ChatAdapter
import com.onusly.app.ui.GoalsFragment
import com.onusly.app.ui.LockActivity
import com.onusly.app.ui.ReviewsFragment
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.ResponseBody.Companion.toResponseBody
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Base64

class AuditRemediationRegressionTest {

    private fun createJwt(sub: String, expEpochSeconds: Long?): String {
        val header = Base64.getUrlEncoder().withoutPadding().encodeToString("""{"alg":"HS256","typ":"JWT"}""".toByteArray())
        val payloadJson = if (expEpochSeconds != null) {
            """{"sub":"$sub","email":"user@test.com","exp":$expEpochSeconds}"""
        } else {
            """{"sub":"$sub","email":"user@test.com"}"""
        }
        val payload = Base64.getUrlEncoder().withoutPadding().encodeToString(payloadJson.toByteArray())
        val sig = Base64.getUrlEncoder().withoutPadding().encodeToString("fakesig".toByteArray())
        return "$header.$payload.$sig"
    }

    @Test
    fun testONUS_HIGH_05_JwtExpirationCheck() {
        val nowSeconds = System.currentTimeMillis() / 1000

        // 1. Expired token (10 minutes in the past)
        val expiredToken = createJwt("user-123", nowSeconds - 600)
        assertTrue("Expired token must evaluate to isTokenExpired = true", AuthManager.isTokenExpired(expiredToken))

        // 2. Valid token (1 hour in future)
        val validToken = createJwt("user-123", nowSeconds + 3600)
        assertFalse("Future token must evaluate to isTokenExpired = false", AuthManager.isTokenExpired(validToken))

        // 3. Token without exp claim (e.g. dev/test tokens)
        val tokenWithoutExp = createJwt("user-123", null)
        assertFalse("Token without exp should not be considered expired by default", AuthManager.isTokenExpired(tokenWithoutExp))

        // 4. Null or blank token
        assertTrue("Null token is expired", AuthManager.isTokenExpired(null))
        assertTrue("Blank token is expired", AuthManager.isTokenExpired(""))
    }

    @Test
    fun testONUS_LOW_01_TrailingNewlinesInErrorBodyTrimmed() {
        val client = OnuslyApiClient()
        val dummyRequest = Request.Builder().url("http://localhost:8080/test").build()

        // Plain text error from Go's http.Error
        val rawBodyWithNewline = "accountability_required\n"
        val response = Response.Builder()
            .request(dummyRequest)
            .protocol(Protocol.HTTP_1_1)
            .code(400)
            .message("Bad Request")
            .body(rawBodyWithNewline.toResponseBody("text/plain".toMediaType()))
            .build()

        val extracted = client.extractErrorMessage(response, rawBodyWithNewline)
        assertEquals("accountability_required", extracted)
        assertFalse(extracted.endsWith("\n"))

        // JSON error with trailing newline inside string
        val jsonError = """{"error": "targets_locked\n"}"""
        val extractedJson = client.extractErrorMessage(response, jsonError)
        assertEquals("targets_locked", extractedJson)
    }

    @Test
    fun testONUS_LOW_03_UnboundedErrorBodyTruncated() {
        val client = OnuslyApiClient()
        val dummyRequest = Request.Builder().url("http://localhost:8080/test").build()

        // 5000 character HTML error page from nginx/proxy
        val giantHtml = "<html><body>" + "A".repeat(5000) + "</body></html>"
        val response = Response.Builder()
            .request(dummyRequest)
            .protocol(Protocol.HTTP_1_1)
            .code(502)
            .message("Bad Gateway")
            .body(giantHtml.toResponseBody("text/html".toMediaType()))
            .build()

        val extracted = client.extractErrorMessage(response, giantHtml)
        assertTrue("Extracted error message must be truncated to 300 chars max", extracted.length <= 300)
        assertEquals(300, extracted.length)
    }

    @Test
    fun testONUS_MED_09_ApprovalResponseParsing() {
        val jsonArray = """
            [
                {
                    "id": "appr-1",
                    "goalId": "goal-100",
                    "proofId": "proof-200",
                    "approverId": "user-friend",
                    "decision": "approved",
                    "comment": "Great job completing your workout!",
                    "decidedAt": "2026-10-26T12:00:00Z"
                },
                {
                    "id": "appr-2",
                    "goalId": "goal-101",
                    "proofId": "proof-201",
                    "approverId": "user-friend-2",
                    "status": "rejected",
                    "comment": "Photo is blurry, please retake",
                    "decidedAt": "2026-10-26T12:30:00Z"
                }
            ]
        """.trimIndent()

        val list = ApprovalResponse.parseList(jsonArray)
        assertEquals(2, list.size)

        val first = list[0]
        assertEquals("appr-1", first.id)
        assertEquals("goal-100", first.goalId)
        assertEquals("proof-200", first.proofId)
        assertEquals("approved", first.decision)
        assertEquals("Great job completing your workout!", first.comment)
        assertEquals("2026-10-26T12:00:00Z", first.decidedAt)

        val second = list[1]
        assertEquals("appr-2", second.id)
        assertEquals("rejected", second.decision)
        assertEquals("Photo is blurry, please retake", second.comment)
    }

    @Test
    fun testONUS_LOW_02_LockActivityStatusProductionMethod() {
        // Test production method: LockActivity.isProofSubmittedState
        // 1. Empty list: must NOT evaluate to true (fixed vacuous truth)
        assertFalse("Empty goal list must not be treated as PROOF SUBMITTED", LockActivity.isProofSubmittedState(emptyList()))

        // 2. Active goal present
        assertFalse("List with active goal is not all proof_submitted", LockActivity.isProofSubmittedState(listOf("active", "proof_submitted")))

        // 3. Only proof_submitted
        assertTrue("List with all proof_submitted evaluates to true", LockActivity.isProofSubmittedState(listOf("proof_submitted", "proof_submitted")))
        assertTrue("Single proof_submitted evaluates to true", LockActivity.isProofSubmittedState(listOf("proof_submitted")))

        // 4. Completed goal present
        assertFalse("List with completed goal is not all proof_submitted", LockActivity.isProofSubmittedState(listOf("completed")))
    }

    @Test
    fun testONUS_MED_06_GoalsFragmentProductionDeletionGuard() {
        val activeGoal = Goal(id = "g1", ownerId = "o1", title = "Active", status = "active")
        val reviewGoal = Goal(id = "g2", ownerId = "o1", title = "Review", status = "proof_submitted")
        val completedGoal = Goal(id = "g3", ownerId = "o1", title = "Completed", status = "completed")

        // Test production method: GoalsFragment.canDeleteGoal
        assertFalse("Active goal cannot be deleted", GoalsFragment.canDeleteGoal(activeGoal))
        assertFalse("In-review (proof_submitted) goal cannot be deleted", GoalsFragment.canDeleteGoal(reviewGoal))
        assertTrue("Completed goal can be deleted", GoalsFragment.canDeleteGoal(completedGoal))
    }

    @Test
    fun testONUS_HIGH_09_ChatAdapterProductionDecisionActionGuard() {
        val myUserId = "user-123"
        val partnerUserId = "user-456"

        // Goal owned by me (partner sends message)
        val myGoalProof = ChatMessage(
            id = "proof-1",
            senderId = partnerUserId,
            senderEmail = "partner@test.com",
            senderHandle = "partner",
            recipientId = myUserId,
            recipientEmail = "me@test.com",
            recipientHandle = "me",
            text = "Proof submitted",
            isProof = true,
            goalId = "my-goal-10",
            goalTitle = "Workout",
            ownerId = myUserId, // I am the owner!
            approved = false,
            time = "2026-10-26T12:00:00Z"
        )

        // Production method: ChatAdapter.canShowDecisionActions
        // Invariant 1: Goal owned by caller must NEVER show decision actions
        assertFalse("Owner must NEVER see decision buttons for their own goal",
            ChatAdapter.canShowDecisionActions(myGoalProof, myUserId, isMe = false))

        // Invariant 2: Self-sent message must NEVER show decision actions
        assertFalse("Self-sent message must never show decision buttons",
            ChatAdapter.canShowDecisionActions(myGoalProof, partnerUserId, isMe = true))

        // Invariant 3: Message with missing/blank ownerId must NEVER show decision actions
        val blankOwnerProof = myGoalProof.copy(ownerId = "")
        assertFalse("Message with blank ownerId must never show decision buttons",
            ChatAdapter.canShowDecisionActions(blankOwnerProof, partnerUserId, isMe = false))

        // Invariant 4: Legitimate partner proof for partner's goal: SHOULD show decision actions
        val partnerGoalProof = myGoalProof.copy(ownerId = partnerUserId)
        assertTrue("Approver reviewing partner's proof should see decision actions",
            ChatAdapter.canShowDecisionActions(partnerGoalProof, myUserId, isMe = false))
    }

    @Test
    fun testONUS_CRIT_01_CriticalTelephonyAndEmergencyUI_AreAlwaysAllowed() {
        val rogueBlockTargets = listOf(
            BlocklistTarget(
                goalId = "g-malicious",
                goalTitle = "Block Everything",
                status = "active",
                apps = listOf(
                    "com.android.phone",
                    "com.android.incallui",
                    "com.samsung.android.dialer",
                    "com.samsung.android.incallui",
                    "com.google.android.dialer",
                    "com.google.android.apps.safetyhub",
                    "com.google.android.emergency",
                    "com.android.cellbroadcastreceiver",
                    "com.google.android.cellbroadcastreceiver",
                    "com.sec.android.app.launcher",
                    "com.miui.home"
                ),
                domains = emptyList()
            )
        )
        val cache = CachedBlocklist(
            userId = "victim-user",
            version = "v1",
            generatedAt = "2026-10-26T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = rogueBlockTargets
        )

        // Core Telephony must never be blocked
        assertFalse("com.android.phone must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.android.phone", cache).isBlocked)

        // Stock in-call UI must never be blocked
        assertFalse("com.android.incallui must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.android.incallui", cache).isBlocked)

        // Samsung dialer and in-call UI must never be blocked
        assertFalse("com.samsung.android.dialer must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.samsung.android.dialer", cache).isBlocked)
        assertFalse("com.samsung.android.incallui must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.samsung.android.incallui", cache).isBlocked)

        // Google Dialer, Safety Hub & Emergency SOS must never be blocked
        assertFalse("com.google.android.dialer must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.google.android.dialer", cache).isBlocked)
        assertFalse("com.google.android.apps.safetyhub must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.google.android.apps.safetyhub", cache).isBlocked)
        assertFalse("com.google.android.emergency must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.google.android.emergency", cache).isBlocked)

        // Emergency Cell Broadcasts (AMBER alerts) must never be blocked
        assertFalse("com.android.cellbroadcastreceiver must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.android.cellbroadcastreceiver", cache).isBlocked)

        // OEM Launchers must never be blocked
        assertFalse("com.sec.android.app.launcher must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.sec.android.app.launcher", cache).isBlocked)
        assertFalse("com.miui.home must NEVER be blocked",
            BlockerEngine.evaluatePackage("com.miui.home", cache).isBlocked)
    }

    @Test
    fun testONUS_CRIT_01_HomeAssistantAndShopping_CanBeBlocked() {
        val targets = listOf(
            BlocklistTarget(
                goalId = "g-distractions",
                goalTitle = "Smart Home & Shopping Distractions",
                status = "active",
                apps = listOf(
                    "com.homeassistant.companion.android",
                    "com.homedepot.digital"
                ),
                domains = emptyList()
            )
        )
        val cache = CachedBlocklist(
            userId = "user-1",
            version = "v1",
            generatedAt = "2026-10-26T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = targets
        )

        // Home Assistant must NOT be falsely excluded by 'home' substring
        assertFalse("Home Assistant is not a system app",
            InstalledAppsManager.isSystemOrExcluded("com.homeassistant.companion.android"))
        assertTrue("Home Assistant CAN be blocked when targeted by user",
            BlockerEngine.evaluatePackage("com.homeassistant.companion.android", cache).isBlocked)

        // Home Depot must NOT be falsely excluded by 'home' substring
        assertFalse("Home Depot is not a system app",
            InstalledAppsManager.isSystemOrExcluded("com.homedepot.digital"))
        assertTrue("Home Depot CAN be blocked when targeted by user",
            BlockerEngine.evaluatePackage("com.homedepot.digital", cache).isBlocked)
    }

    @Test
    fun testONUS_CRIT_01_ThirdPartyAppsWithEmergencyOrTelecom_CanBeBlocked() {
        val thirdPartyApps = listOf(
            "com.emergency.response.simulator",
            "com.telecom.customerapp",
            "com.telecom.billing",
            "com.random.settingswidget",
            "com.thirdparty.dialerapp"
        )
        val targets = listOf(
            BlocklistTarget(
                goalId = "g-thirdparty",
                goalTitle = "Block Distracting Third-Party Apps",
                status = "active",
                apps = thirdPartyApps,
                domains = emptyList()
            )
        )
        val cache = CachedBlocklist(
            userId = "user-1",
            version = "v1",
            generatedAt = "2026-10-26T12:00:00Z",
            lastSyncTimestamp = System.currentTimeMillis(),
            targets = targets
        )

        for (pkg in thirdPartyApps) {
            assertFalse(
                "Third-party app '$pkg' must NOT be mistaken for a protected system app",
                InstalledAppsManager.isSystemOrExcluded(pkg)
            )
            assertTrue(
                "Third-party app '$pkg' CAN be blocked when targeted by user",
                BlockerEngine.evaluatePackage(pkg, cache).isBlocked
            )
        }
    }

    @Test
    fun testONUS_HIGH_04_ProofChronologicalSorting() {
        // 1. Out-of-order timestamps (10:00, 12:00, 09:00 -> findLatestProof returns 12:00)
        val p10 = Proof(
            id = "p-10",
            goalId = "g1",
            ownerId = "o1",
            proofType = "text",
            textExplanation = "10:00",
            submittedAt = "2026-10-26T10:00:00Z"
        )
        val p12 = Proof(
            id = "p-12",
            goalId = "g1",
            ownerId = "o1",
            proofType = "text",
            textExplanation = "12:00",
            submittedAt = "2026-10-26T12:00:00Z"
        )
        val p09 = Proof(
            id = "p-09",
            goalId = "g1",
            ownerId = "o1",
            proofType = "text",
            textExplanation = "09:00",
            submittedAt = "2026-10-26T09:00:00Z"
        )

        val outOfOrder = listOf(p10, p12, p09)
        val latest = ReviewsFragment.findLatestProof(outOfOrder)
        assertNotNull("Latest proof must not be null", latest)
        assertEquals("p-12", latest?.id)
        assertEquals("12:00", latest?.textExplanation)

        val sorted = ReviewsFragment.sortProofsDescending(outOfOrder)
        assertEquals(3, sorted.size)
        assertEquals("p-12", sorted[0].id)
        assertEquals("p-10", sorted[1].id)
        assertEquals("p-09", sorted[2].id)

        // 2. Equal timestamps -> deterministic non-null return
        val pEqA = Proof(id = "p-eq-a", goalId = "g1", ownerId = "o1", proofType = "text", submittedAt = "2026-10-26T12:00:00Z")
        val pEqB = Proof(id = "p-eq-b", goalId = "g1", ownerId = "o1", proofType = "text", submittedAt = "2026-10-26T12:00:00Z")
        val equalProofs = listOf(pEqA, pEqB)
        val latestEqual1 = ReviewsFragment.findLatestProof(equalProofs)
        val latestEqual2 = ReviewsFragment.findLatestProof(equalProofs.reversed())
        assertNotNull(latestEqual1)
        assertNotNull(latestEqual2)
        assertEquals("Equal timestamps must deterministically return the same proof regardless of order", latestEqual1?.id, latestEqual2?.id)

        // 3. Empty list -> returns null
        assertNull("Empty list must return null", ReviewsFragment.findLatestProof(emptyList()))
        assertTrue("Sorting empty list returns empty list", ReviewsFragment.sortProofsDescending(emptyList()).isEmpty())

        // 4. Invalid / blank timestamps (e.g. "" vs "2026-10-26T12:00:00Z" -> valid timestamp wins)
        val pBlank = Proof(id = "p-blank", goalId = "g1", ownerId = "o1", proofType = "text", submittedAt = "")
        val pInvalid = Proof(id = "p-invalid", goalId = "g1", ownerId = "o1", proofType = "text", submittedAt = "invalid-date-format")
        val pValid = Proof(id = "p-valid", goalId = "g1", ownerId = "o1", proofType = "text", submittedAt = "2026-10-26T12:00:00Z")

        val mixed = listOf(pBlank, pValid, pInvalid)
        val latestValid = ReviewsFragment.findLatestProof(mixed)
        assertNotNull(latestValid)
        assertEquals("Valid timestamp must win over blank or invalid timestamps", "p-valid", latestValid?.id)

        val sortedMixed = ReviewsFragment.sortProofsDescending(mixed)
        assertEquals("p-valid", sortedMixed[0].id)
        assertTrue(sortedMixed[1].id == "p-blank" || sortedMixed[1].id == "p-invalid")
        assertTrue(sortedMixed[2].id == "p-blank" || sortedMixed[2].id == "p-invalid")
    }
}
