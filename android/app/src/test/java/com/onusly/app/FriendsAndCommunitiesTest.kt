package com.onusly.app

import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.model.CommunityItem
import com.onusly.app.data.model.ConversationItem
import com.onusly.app.data.model.FriendItem
import com.onusly.app.data.model.FriendRequestItem
import com.onusly.app.data.model.FriendRequestsResponse
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class FriendsAndCommunitiesTest {

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
    fun testFriendItemParsing() {
        val json = JSONObject("""
            {
                "userId": "u-1",
                "friendId": "u-2",
                "friendHandle": "bob",
                "friendEmail": "bob@example.com",
                "friendName": "Bob Ross",
                "createdAt": "2026-10-20T10:00:00Z"
            }
        """.trimIndent())

        val item = FriendItem.fromJson(json)
        assertEquals("u-1", item.userId)
        assertEquals("u-2", item.friendId)
        assertEquals("bob", item.friendHandle)
        assertEquals("Bob Ross", item.friendName)
    }

    @Test
    fun testCommunityItemParsing() {
        val json = JSONObject("""
            {
                "id": "c-coding",
                "name": "5AM Founders",
                "description": "Ship code daily",
                "category": "Coding & Tech",
                "members": 150,
                "streak": "15d",
                "isJoined": true
            }
        """.trimIndent())

        val item = CommunityItem.fromJson(json)
        assertEquals("c-coding", item.id)
        assertEquals("5AM Founders", item.name)
        assertEquals("Coding & Tech", item.category)
        assertEquals(150, item.members)
        assertTrue(item.isJoined)
    }

    @Test
    fun testConversationItemPolymorphism() {
        val friendItem = FriendItem(
            userId = "me",
            friendId = "f-1",
            friendHandle = "alice",
            friendEmail = "alice@gmail.com",
            friendName = "Alice",
            createdAt = ""
        )
        val friendConvo: ConversationItem = ConversationItem.FriendConversation(friendItem)
        assertEquals("f-1", friendConvo.id)
        assertEquals("Alice", friendConvo.name)

        val commItem = CommunityItem(
            id = "c-fitness",
            name = "Morning Runners",
            description = "Run 5k every day",
            category = "Health & Fitness",
            members = 42,
            streak = "5d",
            isJoined = true
        )
        val commConvo: ConversationItem = ConversationItem.CommunityConversation(commItem)
        assertEquals("c-fitness", commConvo.id)
        assertEquals("Morning Runners", commConvo.name)
        assertTrue(commConvo.subtitle.contains("Health & Fitness"))
    }

    @Test
    fun testGetFriendsApi() = runBlocking {
        val json = """
            [
                {
                    "userId": "me",
                    "friendId": "friend-1",
                    "friendHandle": "charlie",
                    "friendEmail": "charlie@test.com",
                    "friendName": "Charlie",
                    "createdAt": "2026-10-20T10:00:00Z"
                }
            ]
        """.trimIndent()

        server.enqueue(MockResponse().setResponseCode(200).setBody(json))
        val baseUrl = server.url("/").toString()

        val result = client.getFriends(baseUrl, "test-jwt")
        assertTrue(result.isSuccess)
        val friends = result.getOrNull()!!
        assertEquals(1, friends.size)
        assertEquals("charlie", friends[0].friendHandle)
    }

    @Test
    fun testSendFriendRequestApi() = runBlocking {
        val json = """
            {
                "id": "req-123",
                "senderId": "me",
                "senderHandle": "myhandle",
                "recipientHandle": "targetuser",
                "status": "pending",
                "createdAt": "2026-10-20T10:00:00Z"
            }
        """.trimIndent()

        server.enqueue(MockResponse().setResponseCode(201).setBody(json))
        val baseUrl = server.url("/").toString()

        val result = client.sendFriendRequest(baseUrl, "test-jwt", "@targetuser")
        assertTrue(result.isSuccess)
        val req = result.getOrNull()!!
        assertEquals("req-123", req.id)
        assertEquals("pending", req.status)
    }

    @Test
    fun testGetFriendRequestsApi() = runBlocking {
        val json = """
            {
                "incoming": [
                    {
                        "id": "req-in-1",
                        "senderId": "user-a",
                        "senderHandle": "alice",
                        "senderName": "Alice",
                        "status": "pending",
                        "createdAt": "2026-10-20T10:00:00Z"
                    }
                ],
                "outgoing": []
            }
        """.trimIndent()

        server.enqueue(MockResponse().setResponseCode(200).setBody(json))
        val baseUrl = server.url("/").toString()

        val result = client.getFriendRequests(baseUrl, "test-jwt")
        assertTrue(result.isSuccess)
        val data = result.getOrNull()!!
        assertEquals(1, data.incoming.size)
        assertEquals("alice", data.incoming[0].senderHandle)
    }

    @Test
    fun testTrendingCommunitiesApi() = runBlocking {
        val json = """
            [
                {
                    "id": "comm-1",
                    "name": "Focus Club",
                    "description": "Daily deep work",
                    "category": "General Focus",
                    "members": 88,
                    "streak": "10d",
                    "isJoined": false
                }
            ]
        """.trimIndent()

        server.enqueue(MockResponse().setResponseCode(200).setBody(json))
        val baseUrl = server.url("/").toString()

        val result = client.getTrendingCommunities(baseUrl, "test-jwt")
        assertTrue(result.isSuccess)
        val communities = result.getOrNull()!!
        assertEquals(1, communities.size)
        assertEquals("Focus Club", communities[0].name)
    }
}
