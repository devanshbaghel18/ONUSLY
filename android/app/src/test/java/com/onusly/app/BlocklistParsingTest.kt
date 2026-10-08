package com.onusly.app

import com.onusly.app.data.model.BlocklistParser
import com.onusly.app.data.model.BlocklistTarget
import com.onusly.app.data.model.CachedBlocklist
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class BlocklistParsingTest {

    @Test
    fun testParseValidBlocklistResponse() {
        val json = """
            {
              "userId": "user_12345",
              "version": "v1-abcdef0123456789",
              "generatedAt": "2026-10-08T12:00:00Z",
              "targets": [
                {
                  "goalId": "goal_1",
                  "goalTitle": "No Social Media During Focus",
                  "status": "active",
                  "apps": ["com.instagram.android", "com.twitter.android"],
                  "domains": ["instagram.com", "x.com"]
                },
                {
                  "goalId": "goal_2",
                  "goalTitle": "Study Session",
                  "status": "proof_submitted",
                  "apps": ["com.netflix.mediaclient"],
                  "domains": ["netflix.com"]
                }
              ]
            }
        """.trimIndent()

        val response = BlocklistParser.parseResponse(json)

        assertEquals("user_12345", response.userId)
        assertEquals("v1-abcdef0123456789", response.version)
        assertEquals("2026-10-08T12:00:00Z", response.generatedAt)
        assertEquals(2, response.targets.size)

        val target1 = response.targets[0]
        assertEquals("goal_1", target1.goalId)
        assertEquals("No Social Media During Focus", target1.goalTitle)
        assertEquals("active", target1.status)
        assertEquals(listOf("com.instagram.android", "com.twitter.android"), target1.apps)
        assertEquals(listOf("instagram.com", "x.com"), target1.domains)

        val target2 = response.targets[1]
        assertEquals("goal_2", target2.goalId)
        assertEquals("proof_submitted", target2.status)
        assertEquals(listOf("com.netflix.mediaclient"), target2.apps)
    }

    @Test
    fun testParseEmptyTargets() {
        val json = """
            {
              "userId": "user_empty",
              "version": "v1-0000000000000000",
              "generatedAt": "2026-10-08T12:00:00Z",
              "targets": []
            }
        """.trimIndent()

        val response = BlocklistParser.parseResponse(json)
        assertEquals("user_empty", response.userId)
        assertEquals(0, response.targets.size)
    }

    @Test
    fun testParseWhitespaceTrimming() {
        val json = """
            {
              "userId": "user_spaces",
              "version": "v1-1234",
              "generatedAt": "2026-10-08T12:00:00Z",
              "targets": [
                {
                  "goalId": "g1",
                  "goalTitle": "Goal",
                  "status": "active",
                  "apps": ["  com.test.app  ", "", "   "],
                  "domains": [" example.com "]
                }
              ]
            }
        """.trimIndent()

        val response = BlocklistParser.parseResponse(json)
        assertEquals(1, response.targets.size)
        assertEquals(listOf("com.test.app"), response.targets[0].apps)
        assertEquals(listOf("example.com"), response.targets[0].domains)
    }

    @Test
    fun testCachedBlocklistSerializationRoundtrip() {
        val original = CachedBlocklist(
            userId = "user_roundtrip",
            version = "v1-fedcba9876543210",
            generatedAt = "2026-10-08T12:00:00Z",
            lastSyncTimestamp = 1791460800000L,
            targets = listOf(
                BlocklistTarget(
                    goalId = "g1",
                    goalTitle = "Title 1",
                    status = "active",
                    apps = listOf("com.app.one", "com.app.two"),
                    domains = listOf("one.com", "two.com")
                )
            )
        )

        val json = BlocklistParser.cachedToJson(original)
        assertTrue(json.contains("user_roundtrip"))
        assertTrue(json.contains("v1-fedcba9876543210"))

        val parsed = BlocklistParser.parseCached(json)
        assertNotNull(parsed)
        assertEquals(original.userId, parsed!!.userId)
        assertEquals(original.version, parsed.version)
        assertEquals(original.lastSyncTimestamp, parsed.lastSyncTimestamp)
        assertEquals(1, parsed.targets.size)
        assertEquals("com.app.one", parsed.targets[0].apps[0])
    }

    @Test
    fun testParseCachedInvalidJsonReturnsNull() {
        assertNull(BlocklistParser.parseCached(""))
        assertNull(BlocklistParser.parseCached("{ invalid json"))
    }
}
