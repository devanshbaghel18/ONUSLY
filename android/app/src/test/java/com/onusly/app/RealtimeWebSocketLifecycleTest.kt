package com.onusly.app

import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.FriendRequestItem
import com.onusly.app.data.realtime.RealtimeEvent
import com.onusly.app.data.realtime.RealtimeWebSocketManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import okhttp3.Protocol
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.IOException

class RealtimeWebSocketLifecycleTest {

    private lateinit var testScope: CoroutineScope
    private val receivedEvents = mutableListOf<RealtimeEvent>()
    private var eventCollectionJob: Job? = null

    @Before
    fun setUp() {
        testScope = CoroutineScope(Dispatchers.Unconfined)
        receivedEvents.clear()
    }

    @After
    fun tearDown() {
        eventCollectionJob?.cancel()
        testScope.cancel()
    }

    private fun startEventCollector(manager: RealtimeWebSocketManager) {
        eventCollectionJob?.cancel()
        eventCollectionJob = testScope.launch {
            manager.events.collect { receivedEvents.add(it) }
        }
    }

    private fun createDummyResponse(
        code: Int = 101,
        message: String = "Switching Protocols",
        url: String = "http://localhost/ws"
    ): Response {
        return Response.Builder()
            .request(Request.Builder().url(url).build())
            .protocol(Protocol.HTTP_1_1)
            .code(code)
            .message(message)
            .build()
    }

    /**
     * Test double for okhttp3.WebSocket that intercepts all send, close, and cancel calls.
     */
    private class FakeWebSocket(
        private val request: Request = Request.Builder().url("http://localhost/ws").build()
    ) : WebSocket {
        val sentMessages = mutableListOf<String>()
        var closedCode: Int? = null
        var closedReason: String? = null
        var isCancelled: Boolean = false
        var isClosed: Boolean = false

        override fun request(): Request = request
        override fun queueSize(): Long = 0L

        override fun send(text: String): Boolean {
            if (isClosed || isCancelled) return false
            sentMessages.add(text)
            return true
        }

        override fun send(bytes: ByteString): Boolean = true

        override fun close(code: Int, reason: String?): Boolean {
            if (isClosed) return false
            isClosed = true
            closedCode = code
            closedReason = reason
            return true
        }

        override fun cancel() {
            isCancelled = true
        }
    }

    // -------------------------------------------------------------------------
    // 1. Logout followed by old socket's onClosed callback
    // -------------------------------------------------------------------------

    @Test
    fun testLogout_followedByOldSocketOnClosed_doesNotTriggerReconnect() {
        var token: String? = "jwt-session-123"
        val fakeSocket = FakeWebSocket()
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { token },
            webSocketFactory = { _, _ -> fakeSocket }
        )

        // Establish connection generation 1
        manager.connect()
        val gen1 = manager.currentGeneration
        assertEquals(1L, gen1)

        val listener = manager.createWebSocketListener(gen1, token!!)
        val dummyResponse = createDummyResponse()
        listener.onOpen(fakeSocket, dummyResponse)
        assertTrue("Active socket should mark manager connected", manager.isConnectedState)

        // User logs out
        token = null
        manager.disconnect()
        assertTrue("Intentional disconnect must be true after logout", manager.isIntentionalDisconnect)
        assertEquals("Generation must increment upon disconnect", 2L, manager.currentGeneration)
        assertFalse("Socket state should be disconnected", manager.isConnectedState)
        assertEquals("No reconnect should be scheduled during disconnect", 0, manager.reconnectScheduleCount)

        // Stale socket receives an onClosed callback (e.g. server shutdown or network tear-down)
        listener.onClosed(fakeSocket, 1001, "Going Away")

        assertFalse("Disconnected manager should not be in connected state", manager.isConnectedState)
        assertFalse("Old socket onClosed must not trigger reconnect after logout", manager.isReconnectScheduled)
        assertEquals("Reconnect schedule count must remain 0", 0, manager.reconnectScheduleCount)
    }

    // -------------------------------------------------------------------------
    // 2. Account A switching to Account B: generation increments & A's callbacks discarded
    // -------------------------------------------------------------------------

    @Test
    fun testAccountSwitch_generationIncrements_andAccountACallbacksDiscarded() {
        var activeToken = "token-account-a"
        var createdSockets = mutableListOf<FakeWebSocket>()

        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { activeToken },
            webSocketFactory = { _, _ ->
                val s = FakeWebSocket()
                createdSockets.add(s)
                s
            }
        )
        startEventCollector(manager)

        // 1. Connect Account A
        manager.connect()
        val genA = manager.currentGeneration
        assertEquals(1L, genA)
        val socketA = createdSockets[0]
        val listenerA = manager.createWebSocketListener(genA, "token-account-a")

        listenerA.onOpen(socketA, createDummyResponse())
        assertTrue(manager.isConnectedState)
        // Handshake sent for Account A
        assertTrue(socketA.sentMessages.any { it.contains("token-account-a") })

        // 2. Switch to Account B (connect increments generation and closes old socket)
        activeToken = "token-account-b"
        manager.connect()
        val genB = manager.currentGeneration
        assertTrue("Account B connect must increment generation", genB > genA)
        assertEquals(2L, genB)
        val socketB = createdSockets[1]
        val listenerB = manager.createWebSocketListener(genB, "token-account-b")

        // Old socket A was closed during reconnect
        assertEquals(1000, socketA.closedCode)

        // 3. Stale callbacks from Account A socket must be completely discarded
        val staleSocketNew = FakeWebSocket()

        // 3a. Stale onOpen
        listenerA.onOpen(staleSocketNew, createDummyResponse())
        assertEquals("Stale onOpen must close socket with 1000", 1000, staleSocketNew.closedCode)
        assertEquals("Stale connection", staleSocketNew.closedReason)
        assertTrue("No auth handshake sent on stale socket", staleSocketNew.sentMessages.isEmpty())

        // 3b. Stale onMessage
        val sampleChatJson = JSONObject().apply {
            put("type", "chat.message")
            put("payload", JSONObject().apply {
                put("id", "msg-stale-account-a")
                put("senderId", "u-account-a")
                put("text", "Secret message for Account A")
            })
        }
        listenerA.onMessage(socketA, sampleChatJson.toString())
        assertTrue("Stale message from Account A must not emit to events flow", receivedEvents.isEmpty())

        // 3c. Stale onFailure
        listenerA.onFailure(socketA, IOException("Account A connection timed out"), null)
        assertFalse("Stale failure from Account A must not schedule reconnect", manager.isReconnectScheduled)
        assertEquals(0, manager.reconnectScheduleCount)

        // 3d. Stale onClosed
        listenerA.onClosed(socketA, 1006, "Account A socket terminated abnormally")
        assertFalse("Stale onClosed from Account A must not schedule reconnect", manager.isReconnectScheduled)
        assertEquals(0, manager.reconnectScheduleCount)

        // 4. Valid callbacks from Account B socket must function normally
        listenerB.onOpen(socketB, createDummyResponse())
        assertTrue("Account B socket sends auth with token B", socketB.sentMessages.any { it.contains("token-account-b") })

        val bChatJson = JSONObject().apply {
            put("type", "chat.message")
            put("payload", JSONObject().apply {
                put("id", "msg-account-b-1")
                put("senderId", "u-account-b")
                put("text", "Hello Account B")
            })
        }
        listenerB.onMessage(socketB, bChatJson.toString())
        assertEquals("Account B event should be delivered", 1, receivedEvents.size)
        val event = receivedEvents[0] as RealtimeEvent.ChatMessageReceived
        assertEquals("msg-account-b-1", event.message.id)
    }

    // -------------------------------------------------------------------------
    // 3. Stale Callbacks: onOpen, onMessage, onFailure, onClosed
    // -------------------------------------------------------------------------

    @Test
    fun testStaleCallback_onOpen_closesStaleSocketAndDoesNotSendHandshake() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "test-token" }
        )
        // Advance generation to 5
        manager.setConnectionGenerationForTesting(5L)

        // Create listener with stale generation 4
        val staleListener = manager.createWebSocketListener(gen = 4L, token = "test-token")
        val staleSocket = FakeWebSocket()

        staleListener.onOpen(staleSocket, createDummyResponse())

        assertEquals("Stale socket must be closed with code 1000", 1000, staleSocket.closedCode)
        assertEquals("Stale socket must have reason 'Stale connection'", "Stale connection", staleSocket.closedReason)
        assertTrue("Stale socket must not have any handshake messages sent", staleSocket.sentMessages.isEmpty())
        assertFalse("Manager isConnected state must remain false", manager.isConnectedState)
    }

    @Test
    fun testStaleCallback_onMessage_dropsMessageAndDoesNotEmitEvent() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "test-token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(10L)

        // Stale listener with gen 9
        val staleListener = manager.createWebSocketListener(gen = 9L, token = "test-token")
        val fakeSocket = FakeWebSocket()

        val json = JSONObject().apply {
            put("type", "chat.message")
            put("payload", JSONObject().apply {
                put("id", "msg-drop-me")
                put("text", "Ignored payload")
            })
        }

        staleListener.onMessage(fakeSocket, json.toString())

        assertTrue("Stale onMessage must be dropped without emitting any event", receivedEvents.isEmpty())
    }

    @Test
    fun testStaleCallback_onFailure_dropsErrorAndDoesNotTriggerReconnect() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" }
        )
        manager.setConnectionGenerationForTesting(3L)

        // Stale listener with gen 2
        val staleListener = manager.createWebSocketListener(gen = 2L, token = "valid-token")
        val fakeSocket = FakeWebSocket()

        staleListener.onFailure(fakeSocket, IOException("Network broke on old socket"), null)

        assertFalse("Stale onFailure must not schedule reconnect", manager.isReconnectScheduled)
        assertEquals("Reconnect count must remain 0", 0, manager.reconnectScheduleCount)
    }

    @Test
    fun testStaleCallback_onClosed_dropsClosureAndDoesNotTriggerReconnect() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" }
        )
        manager.setConnectionGenerationForTesting(8L)

        // Stale listener with gen 7
        val staleListener = manager.createWebSocketListener(gen = 7L, token = "valid-token")
        val fakeSocket = FakeWebSocket()

        // 1006 abnormal closure on old socket
        staleListener.onClosed(fakeSocket, 1006, "Abnormal socket closure")

        assertFalse("Stale onClosed must not schedule reconnect", manager.isReconnectScheduled)
        assertEquals("Reconnect count must remain 0", 0, manager.reconnectScheduleCount)
    }

    // -------------------------------------------------------------------------
    // 4. Clean closure with code 1000 on active socket
    // -------------------------------------------------------------------------

    @Test
    fun testCleanClosure_code1000_onActiveSocket_doesNotTriggerReconnect() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" }
        )
        manager.setConnectionGenerationForTesting(1L)
        val activeListener = manager.createWebSocketListener(gen = 1L, token = "valid-token")
        val fakeSocket = FakeWebSocket()

        activeListener.onOpen(fakeSocket, createDummyResponse())
        assertTrue("Socket should be marked connected", manager.isConnectedState)

        // Server cleanly closes socket with 1000
        activeListener.onClosed(fakeSocket, 1000, "Clean closure")

        assertFalse("Manager must update isConnected to false", manager.isConnectedState)
        assertFalse("Code 1000 clean closure must NOT trigger reconnect", manager.isReconnectScheduled)
        assertEquals("Reconnect count must remain 0", 0, manager.reconnectScheduleCount)
    }

    @Test
    fun testAbnormalClosure_non1000_onActiveSocket_triggersReconnect() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" }
        )
        manager.setConnectionGenerationForTesting(1L)
        val activeListener = manager.createWebSocketListener(gen = 1L, token = "valid-token")
        val fakeSocket = FakeWebSocket()

        activeListener.onOpen(fakeSocket, createDummyResponse())
        assertTrue(manager.isConnectedState)

        // Server abnormal closure (1006 or 1001)
        activeListener.onClosed(fakeSocket, 1006, "Abnormal closure")

        assertFalse(manager.isConnectedState)
        assertTrue("Abnormal closure on active socket MUST schedule reconnect", manager.isReconnectScheduled)
        assertEquals("Reconnect count must be 1", 1, manager.reconnectScheduleCount)
    }

    @Test
    fun testFailure_onActiveSocket_triggersReconnect() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" }
        )
        manager.setConnectionGenerationForTesting(1L)
        val activeListener = manager.createWebSocketListener(gen = 1L, token = "valid-token")
        val fakeSocket = FakeWebSocket()

        activeListener.onFailure(fakeSocket, IOException("Connection reset by peer"), null)

        assertFalse(manager.isConnectedState)
        assertTrue("onFailure on active socket MUST schedule reconnect", manager.isReconnectScheduled)
        assertEquals(1, manager.reconnectScheduleCount)
    }

    // -------------------------------------------------------------------------
    // 5. Cancellation of pending reconnect work
    // -------------------------------------------------------------------------

    @Test
    fun testDisconnect_cancelsPendingReconnectWork() {
        val fakeSocket = FakeWebSocket()
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "valid-token" },
            webSocketFactory = { _, _ -> fakeSocket }
        )
        manager.setConnectionGenerationForTesting(1L)
        val activeListener = manager.createWebSocketListener(gen = 1L, token = "valid-token")

        // Trigger a failure to cause reconnect scheduling
        activeListener.onFailure(fakeSocket, IOException("Socket failure"), null)

        assertTrue("Reconnect job should be scheduled and active", manager.isReconnectScheduled)
        val pendingJob = manager.getReconnectJob()
        assertNotNull("Reconnect job reference must not be null", pendingJob)
        assertTrue("Pending reconnect job must be active", pendingJob!!.isActive)

        // Call disconnect()
        manager.disconnect()

        assertTrue("Intentional disconnect flag must be set", manager.isIntentionalDisconnect)
        assertFalse("Reconnect must no longer be scheduled", manager.isReconnectScheduled)
        assertNull("Reconnect job reference must be null after disconnect", manager.getReconnectJob())
        assertTrue("Previous reconnect job must have been cancelled", pendingJob.isCancelled)
    }

    // -------------------------------------------------------------------------
    // 6. Message parsing & event emission for matching generation
    // -------------------------------------------------------------------------

    @Test
    fun testMessageParsing_chatMessageReceived() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        val json = JSONObject().apply {
            put("type", "chat.message")
            put("payload", JSONObject().apply {
                put("id", "msg-101")
                put("senderId", "u-alice")
                put("senderEmail", "alice@example.com")
                put("senderHandle", "alice_h")
                put("recipientId", "u-bob")
                put("recipientEmail", "bob@example.com")
                put("recipientHandle", "bob_h")
                put("text", "Hey Bob, goal checked!")
                put("isProof", true)
                put("goalId", "goal-99")
                put("goalTitle", "Morning Run")
                put("ownerId", "u-alice")
                put("approved", false)
                put("time", "2026-10-26T08:00:00Z")
            })
        }

        listener.onMessage(fakeSocket, json.toString())

        assertEquals("One event should be emitted", 1, receivedEvents.size)
        val event = receivedEvents[0] as RealtimeEvent.ChatMessageReceived
        val msg = event.message
        assertEquals("msg-101", msg.id)
        assertEquals("u-alice", msg.senderId)
        assertEquals("alice_h", msg.senderHandle)
        assertEquals("Hey Bob, goal checked!", msg.text)
        assertTrue(msg.isProof)
        assertEquals("goal-99", msg.goalId)
        assertEquals("Morning Run", msg.goalTitle)
    }

    @Test
    fun testMessageParsing_communityMessageReceived() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        val json = JSONObject().apply {
            put("type", "community.message")
            put("payload", JSONObject().apply {
                put("communityId", "comm-running-club")
                put("id", "comm-msg-202")
                put("senderId", "u-coach")
                put("senderEmail", "coach@runners.org")
                put("senderHandle", "coach_bob")
                put("text", "5k completed by the team today!")
                put("time", "2026-10-26T09:30:00Z")
            })
        }

        listener.onMessage(fakeSocket, json.toString())

        assertEquals("One event should be emitted", 1, receivedEvents.size)
        val event = receivedEvents[0] as RealtimeEvent.CommunityMessageReceived
        assertEquals("comm-running-club", event.communityId)
        assertEquals("comm-msg-202", event.message.id)
        assertEquals("5k completed by the team today!", event.message.text)
        assertEquals("coach_bob", event.message.senderHandle)
    }

    @Test
    fun testMessageParsing_friendRequestReceived_nestedAndFlat() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        // 1. Nested request payload: { payload: { request: { ... } } }
        val nestedJson = JSONObject().apply {
            put("type", "friend.request")
            put("payload", JSONObject().apply {
                put("request", JSONObject().apply {
                    put("id", "freq-1")
                    put("senderId", "u-charlie")
                    put("senderHandle", "charlie")
                    put("senderName", "Charlie Brown")
                    put("status", "pending")
                })
            })
        }
        listener.onMessage(fakeSocket, nestedJson.toString())

        assertEquals(1, receivedEvents.size)
        val event1 = receivedEvents[0] as RealtimeEvent.FriendRequestReceived
        assertEquals("freq-1", event1.request.id)
        assertEquals("Charlie Brown", event1.request.senderName)

        // 2. Flat payload: { payload: { id: "freq-2", ... } }
        val flatJson = JSONObject().apply {
            put("type", "friend.request")
            put("payload", JSONObject().apply {
                put("id", "freq-2")
                put("senderId", "u-diana")
                put("senderHandle", "diana")
                put("senderName", "Diana Prince")
                put("status", "pending")
            })
        }
        listener.onMessage(fakeSocket, flatJson.toString())

        assertEquals(2, receivedEvents.size)
        val event2 = receivedEvents[1] as RealtimeEvent.FriendRequestReceived
        assertEquals("freq-2", event2.request.id)
        assertEquals("Diana Prince", event2.request.senderName)
    }

    @Test
    fun testMessageParsing_friendAccepted() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        val json = JSONObject().apply {
            put("type", "friend.accepted")
            put("payload", JSONObject().apply {
                put("friendId", "u-friend-777")
                put("friendName", "Clark Kent")
            })
        }

        listener.onMessage(fakeSocket, json.toString())

        assertEquals(1, receivedEvents.size)
        val event = receivedEvents[0] as RealtimeEvent.FriendAccepted
        assertEquals("u-friend-777", event.friendId)
        assertEquals("Clark Kent", event.friendName)
    }

    @Test
    fun testMalformedMessage_doesNotCrashOrEmitEvent() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        startEventCollector(manager)
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        // Invalid JSON string
        listener.onMessage(fakeSocket, "not a valid json {[[[")
        // Unknown type
        listener.onMessage(fakeSocket, """{"type":"unknown.system.ping","payload":{}}""")

        assertTrue("Malformed or unknown messages must not emit events", receivedEvents.isEmpty())

        // Followed by valid message to ensure parser remained healthy
        val validJson = JSONObject().apply {
            put("type", "friend.accepted")
            put("payload", JSONObject().apply {
                put("friendId", "f-valid")
                put("friendName", "Valid User")
            })
        }
        listener.onMessage(fakeSocket, validJson.toString())
        assertEquals(1, receivedEvents.size)
    }

    // -------------------------------------------------------------------------
    // 7. Active socket onOpen handshake and community subscription tests
    // -------------------------------------------------------------------------

    @Test
    fun testActiveSocket_onOpen_sendsHandshakeAndRejoinsCommunities() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "secret-token-xyz" }
        )
        manager.joinCommunity("community-alpha")
        manager.joinCommunity("community-beta")

        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "secret-token-xyz")
        val fakeSocket = FakeWebSocket()
        manager.setWebSocketForTesting(fakeSocket)

        listener.onOpen(fakeSocket, createDummyResponse())

        assertTrue("Active socket should mark isConnected as true", manager.isConnectedState)

        val messages = fakeSocket.sentMessages
        assertTrue("Auth message must be sent", messages.any { it.contains("\"type\":\"auth\"") && it.contains("secret-token-xyz") })
        assertTrue("user.online message must be sent", messages.any { it.contains("\"type\":\"user.online\"") })
        assertTrue("presence.query message must be sent", messages.any { it.contains("\"type\":\"presence.query\"") })
        assertTrue("Join room for community-alpha must be sent", messages.any { it.contains("community.join_room") && it.contains("community-alpha") })
        assertTrue("Join room for community-beta must be sent", messages.any { it.contains("community.join_room") && it.contains("community-beta") })
    }

    @Test
    fun testDisconnect_clearsJoinedCommunitiesAndClosesSocket() {
        val fakeSocket = FakeWebSocket()
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" },
            webSocketFactory = { _, _ -> fakeSocket }
        )
        manager.joinCommunity("c1")
        manager.joinCommunity("c2")
        assertEquals(2, manager.activeJoinedCommunities.size)

        manager.setWebSocketForTesting(fakeSocket)
        manager.disconnect()

        assertTrue("Joined communities must be cleared on disconnect", manager.activeJoinedCommunities.isEmpty())
        assertEquals("Active socket must be closed with code 1000", 1000, fakeSocket.closedCode)
        assertEquals("App closed or signed out", fakeSocket.closedReason)
        assertFalse(manager.isConnectedState)
    }

    @Test
    fun testConnect_withBlankToken_doesNotInitiateConnection() {
        var createdSocketCount = 0
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "" },
            webSocketFactory = { _, _ ->
                createdSocketCount++
                FakeWebSocket()
            }
        )

        manager.connect()

        assertEquals("No socket should be created when token is blank", 0, createdSocketCount)
        assertFalse(manager.isConnectedState)
    }

    @Test
    fun testOnClosing_closesSocketAndMarksDisconnected() {
        val manager = RealtimeWebSocketManager(
            getBaseUrl = { "http://localhost:8080" },
            getToken = { "token" }
        )
        manager.setConnectionGenerationForTesting(1L)
        val listener = manager.createWebSocketListener(gen = 1L, token = "token")
        val fakeSocket = FakeWebSocket()

        listener.onOpen(fakeSocket, createDummyResponse())
        assertTrue(manager.isConnectedState)

        listener.onClosing(fakeSocket, 1000, "Normal closing")

        assertFalse("onClosing should mark isConnected as false", manager.isConnectedState)
        assertEquals("onClosing should call close(1000, null)", 1000, fakeSocket.closedCode)
    }
}
