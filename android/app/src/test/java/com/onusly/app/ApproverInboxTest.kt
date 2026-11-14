package com.onusly.app

import com.onusly.app.data.model.ApproverInboxItem
import com.onusly.app.data.model.ApproverInboxResponse
import com.onusly.app.data.model.GoalParser
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ApproverInboxTest {

    @Test
    fun testParseApproverInboxResponse_FullPayload() {
        val json = """
            {
                "items": [
                    {
                        "goalId": "goal-456",
                        "ownerId": "user-alice",
                        "ownerName": "Alice Smith",
                        "ownerHandle": "alice_smith",
                        "ownerEmail": "alice@example.com",
                        "title": "Deep Focus Work",
                        "description": "No social media during work",
                        "status": "proof_submitted",
                        "createdAt": "2026-11-14T10:00:00Z",
                        "updatedAt": "2026-11-14T12:00:00Z",
                        "blockedApps": ["com.instagram.android"],
                        "blockedDomains": ["instagram.com", "tiktok.com"],
                        "latestProof": {
                            "id": "proof-789",
                            "goalId": "goal-456",
                            "ownerId": "user-alice",
                            "proofType": "text",
                            "textExplanation": "Completed all deep focus tasks.",
                            "submittedAt": "2026-11-14T12:00:00Z"
                        },
                        "proofCount": 2
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val response = GoalParser.parseApproverInboxResponse(json)
        assertEquals(1, response.total)
        assertEquals(1, response.items.size)

        val item = response.items[0]
        assertEquals("goal-456", item.goalId)
        assertEquals("user-alice", item.ownerId)
        assertEquals("Alice Smith", item.ownerName)
        assertEquals("alice_smith", item.ownerHandle)
        assertEquals("alice@example.com", item.ownerEmail)
        assertEquals("Deep Focus Work", item.title)
        assertEquals("proof_submitted", item.status)
        assertEquals(1, item.blockedApps.size)
        assertEquals(2, item.blockedDomains.size)
        assertEquals(2, item.proofCount)

        assertNotNull(item.latestProof)
        assertEquals("proof-789", item.latestProof?.id)
        assertEquals("Completed all deep focus tasks.", item.latestProof?.textExplanation)
        assertEquals("text", item.latestProof?.proofType)

        val goal = item.toGoal()
        assertEquals("goal-456", goal.id)
        assertEquals("user-alice", goal.ownerId)
        assertEquals("proof_submitted", goal.status)
        assertTrue(goal.isProofSubmitted)
        assertFalse(goal.isCompleted)
    }

    @Test
    fun testParseApproverInboxResponse_NoProof() {
        val json = """
            {
                "items": [
                    {
                        "goalId": "goal-empty",
                        "ownerId": "user-bob",
                        "title": "Morning Gym",
                        "status": "active"
                    }
                ],
                "total": 1
            }
        """.trimIndent()

        val response = GoalParser.parseApproverInboxResponse(json)
        assertEquals(1, response.items.size)

        val item = response.items[0]
        assertEquals("goal-empty", item.goalId)
        assertNull(item.latestProof)
        assertEquals(0, item.proofCount)

        val goal = item.toGoal()
        assertTrue(goal.isActive)
        assertFalse(goal.isProofSubmitted)
    }

    @Test
    fun testParseApproverInboxResponse_EmptyInbox() {
        val json = """
            {
                "items": [],
                "total": 0
            }
        """.trimIndent()

        val response = GoalParser.parseApproverInboxResponse(json)
        assertEquals(0, response.total)
        assertTrue(response.items.isEmpty())
    }

    @Test
    fun testRejectionFeedbackValidation_5CharsRequired() {
        fun validateComment(status: String, comment: String): Boolean {
            val isReject = status.equals("rejected", ignoreCase = true)
            if (isReject && comment.trim().length < 5) {
                return false
            }
            return true
        }

        // Rejection with less than 5 characters -> INVALID
        assertFalse(validateComment("rejected", ""))
        assertFalse(validateComment("rejected", "   "))
        assertFalse(validateComment("rejected", "no"))
        assertFalse(validateComment("rejected", "bad!"))

        // Rejection with 5 or more characters -> VALID
        assertTrue(validateComment("rejected", "12345"))
        assertTrue(validateComment("rejected", "Please attach clear screenshot."))

        // Approval requires no feedback length -> VALID
        assertTrue(validateComment("approved", ""))
        assertTrue(validateComment("approved", "ok"))
    }
}

