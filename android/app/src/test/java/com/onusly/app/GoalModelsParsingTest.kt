package com.onusly.app

import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.UpdateGoalRequest
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class GoalModelsParsingTest {

    @Test
    fun testParseSingleGoal() {
        val json = """
            {
                "id": "goal-123",
                "ownerId": "owner-sub",
                "approverId": "approver-sub",
                "approverEmail": "friend@example.com",
                "approvalType": "friend",
                "title": "Study Kotlin",
                "description": "2 hours daily",
                "status": "active",
                "createdAt": "2026-10-19T10:00:00Z",
                "updatedAt": "2026-10-19T10:00:00Z",
                "blockedApps": ["com.instagram.android", "com.reddit.frontpage"],
                "blockedDomains": ["instagram.com", "reddit.com"]
            }
        """.trimIndent()

        val goal = GoalParser.parseGoal(JSONObject(json))
        assertEquals("goal-123", goal.id)
        assertEquals("owner-sub", goal.ownerId)
        assertEquals("approver-sub", goal.approverId)
        assertEquals("friend@example.com", goal.approverEmail)
        assertEquals("friend", goal.approvalType)
        assertEquals("Study Kotlin", goal.title)
        assertEquals("2 hours daily", goal.description)
        assertEquals("active", goal.status)
        assertTrue(goal.isTargeted)
        assertTrue(goal.isActive)
        assertFalse(goal.isProofSubmitted)
        assertFalse(goal.isCompleted)
        assertEquals(2, goal.blockedApps.size)
        assertEquals("com.instagram.android", goal.blockedApps[0])
        assertEquals("com.reddit.frontpage", goal.blockedApps[1])
        assertEquals(2, goal.blockedDomains.size)
        assertEquals("instagram.com", goal.blockedDomains[0])
    }

    @Test
    fun testParseGoalsList() {
        val json = """
            [
                {
                    "id": "goal-1",
                    "ownerId": "owner-sub",
                    "title": "Goal 1",
                    "status": "proof_submitted",
                    "blockedApps": ["com.twitter.android"]
                },
                {
                    "id": "goal-2",
                    "ownerId": "owner-sub",
                    "title": "Goal 2",
                    "status": "completed",
                    "blockedApps": []
                }
            ]
        """.trimIndent()

        val goals = GoalParser.parseGoals(json)
        assertEquals(2, goals.size)
        assertEquals("goal-1", goals[0].id)
        assertTrue(goals[0].isProofSubmitted)
        assertTrue(goals[0].isTargeted)
        assertEquals("goal-2", goals[1].id)
        assertTrue(goals[1].isCompleted)
        assertFalse(goals[1].isTargeted)
    }

    @Test
    fun testCreateGoalRequestSerialization() {
        val req = CreateGoalRequest(
            title = "  Write Tests  ",
            description = "  Full coverage  ",
            approvalType = "friend",
            approverEmail = "partner@onusly.com",
            blockedApps = listOf("com.youtube.android"),
            blockedDomains = listOf("youtube.com")
        )

        val jsonStr = GoalParser.createGoalRequestToJson(req)
        val json = JSONObject(jsonStr)

        assertEquals("Write Tests", json.getString("title"))
        assertEquals("Full coverage", json.getString("description"))
        assertEquals("friend", json.getString("approvalType"))
        assertEquals("partner@onusly.com", json.getString("approverEmail"))
        val apps = json.getJSONArray("blockedApps")
        assertEquals(1, apps.length())
        assertEquals("com.youtube.android", apps.getString(0))
        val domains = json.getJSONArray("blockedDomains")
        assertEquals(1, domains.length())
        assertEquals("youtube.com", domains.getString(0))
    }

    @Test
    fun testUpdateGoalRequestSerialization() {
        val req = UpdateGoalRequest(
            title = "Updated Title",
            blockedApps = listOf("com.app.a", "com.app.b")
        )

        val jsonStr = GoalParser.updateGoalRequestToJson(req)
        val json = JSONObject(jsonStr)

        assertEquals("Updated Title", json.getString("title"))
        assertFalse(json.has("description"))
        val apps = json.getJSONArray("blockedApps")
        assertEquals(2, apps.length())
        assertEquals("com.app.a", apps.getString(0))
        assertEquals("com.app.b", apps.getString(1))
    }

    @Test
    fun testParseProofAndSerialization() {
        val req = CreateProofRequest(
            proofType = "text",
            textExplanation = "Finished the chapters and took notes."
        )
        val jsonReq = GoalParser.createProofRequestToJson(req)
        val jsonReqObj = JSONObject(jsonReq)
        assertEquals("text", jsonReqObj.getString("proofType"))
        assertEquals("Finished the chapters and took notes.", jsonReqObj.getString("textExplanation"))

        val proofResponse = """
            {
                "id": "proof-999",
                "goalId": "goal-123",
                "ownerId": "owner-sub",
                "proofType": "text",
                "textExplanation": "Finished the chapters and took notes.",
                "submittedAt": "2026-10-19T12:00:00Z"
            }
        """.trimIndent()

        val parsed = GoalParser.parseProof(JSONObject(proofResponse))
        assertEquals("proof-999", parsed.id)
        assertEquals("goal-123", parsed.goalId)
        assertEquals("owner-sub", parsed.ownerId)
        assertEquals("text", parsed.proofType)
        assertEquals("Finished the chapters and took notes.", parsed.textExplanation)
        assertEquals("2026-10-19T12:00:00Z", parsed.submittedAt)
    }

    @Test
    fun testApprovalDecisionSerializationAndParse() {
        val req = ApprovalDecisionRequest(
            ownerId = "owner-sub",
            status = "approved",
            comment = "Great work!"
        )
        val jsonReq = GoalParser.approvalDecisionRequestToJson(req)
        val obj = JSONObject(jsonReq)
        assertEquals("owner-sub", obj.getString("ownerId"))
        assertEquals("approved", obj.getString("status"))
        assertEquals("Great work!", obj.getString("comment"))

        val responseJson = """
            {
                "id": "appr-1",
                "goalId": "goal-123",
                "proofId": "proof-999",
                "ownerId": "owner-sub",
                "approverId": "approver-sub",
                "status": "approved",
                "comment": "Great work!",
                "decidedAt": "2026-10-19T12:05:00Z"
            }
        """.trimIndent()

        val parsed = GoalParser.parseApprovalResult(responseJson)
        assertEquals("appr-1", parsed.id)
        assertEquals("goal-123", parsed.goalId)
        assertEquals("proof-999", parsed.proofId)
        assertEquals("approved", parsed.status)
        assertEquals("Great work!", parsed.comment)
    }

    @Test
    fun testUserProfileParsing() {
        val json = """
            {
                "id": "user-sub-123",
                "email": "user@example.com",
                "handle": "my_handle",
                "name": "Jane Doe",
                "picture": "https://example.com/pic.jpg",
                "createdAt": "2026-10-19T00:00:00Z"
            }
        """.trimIndent()

        val user = GoalParser.parseUserProfile(json)
        assertEquals("user-sub-123", user.id)
        assertEquals("user@example.com", user.email)
        assertEquals("my_handle", user.handle)
        assertEquals("Jane Doe", user.name)
        assertEquals("https://example.com/pic.jpg", user.picture)
    }
}
