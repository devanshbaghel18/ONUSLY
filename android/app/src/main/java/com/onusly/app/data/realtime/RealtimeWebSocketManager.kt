package com.onusly.app.data.realtime

import android.util.Log
import com.onusly.app.OnuslyApplication
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.FriendItem
import com.onusly.app.data.model.FriendRequestItem
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

sealed class RealtimeEvent {
    data class ChatMessageReceived(val message: ChatMessage) : RealtimeEvent()
    data class CommunityMessageReceived(val communityId: String, val message: ChatMessage) : RealtimeEvent()
    data class FriendRequestReceived(val request: FriendRequestItem) : RealtimeEvent()
    data class FriendAccepted(val friendId: String, val friendName: String) : RealtimeEvent()
}

class RealtimeWebSocketManager(
    private val getBaseUrl: () -> String,
    private val getToken: () -> String?,
    private val client: OkHttpClient = OkHttpClient.Builder()
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .connectTimeout(10, TimeUnit.SECONDS)
        .pingInterval(30, TimeUnit.SECONDS)
        .build(),
    authManager: AuthManager? = null,
    coroutineScope: CoroutineScope? = null,
    private val webSocketFactory: WebSocket.Factory = client
) {
    constructor(
        getBaseUrl: () -> String,
        getToken: () -> String?,
        authManager: AuthManager?
    ) : this(
        getBaseUrl = getBaseUrl,
        getToken = getToken,
        client = OkHttpClient.Builder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .connectTimeout(10, TimeUnit.SECONDS)
            .pingInterval(30, TimeUnit.SECONDS)
            .build(),
        authManager = authManager,
        coroutineScope = null
    )

    private val authManager: AuthManager? = authManager
        get() = field ?: try {
            OnuslyApplication.instance.authManager
        } catch (_: Throwable) {
            null
        }

    private val tag = "RealtimeWSManager"
    private val scope = coroutineScope ?: CoroutineScope(SupervisorJob() + Dispatchers.IO)

    @Volatile private var intentionalDisconnect = false
    private val connectionGeneration = AtomicLong(0)

    @Volatile private var currentSocket: WebSocket? = null
    private var webSocket: WebSocket?
        get() = currentSocket
        set(value) { currentSocket = value }

    @Volatile private var isConnected = false
    private var reconnectJob: Job? = null
    private val joinedCommunities = ConcurrentHashMap.newKeySet<String>()

    private val _events = MutableSharedFlow<RealtimeEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<RealtimeEvent> = _events.asSharedFlow()

    // Testing and lifecycle inspection hooks
    internal val currentGeneration: Long
        get() = connectionGeneration.get()

    internal fun getConnectionGeneration(): Long = connectionGeneration.get()

    internal val isIntentionalDisconnect: Boolean
        get() = intentionalDisconnect

    internal val isConnectedState: Boolean
        get() = isConnected

    internal fun isConnected(): Boolean = isConnected

    internal val isReconnectScheduled: Boolean
        get() = reconnectJob?.isActive == true

    internal fun hasPendingReconnect(): Boolean = reconnectJob?.isActive == true

    internal fun getReconnectJob(): Job? = reconnectJob

    internal var reconnectScheduleCount = 0
        private set

    internal var currentListenerForTesting: WebSocketListener? = null
        private set

    internal val activeJoinedCommunities: Set<String>
        get() = joinedCommunities

    internal fun setWebSocketForTesting(socket: WebSocket?) {
        webSocket = socket
        currentSocket = socket
    }

    internal fun setConnectionGenerationForTesting(value: Long) {
        connectionGeneration.set(value)
    }

    internal fun setIntentionalDisconnectForTesting(value: Boolean) {
        intentionalDisconnect = value
    }

    internal fun scheduleReconnectForTesting() {
        scheduleReconnect()
    }

    fun connect() {
        intentionalDisconnect = false
        val currentGen = connectionGeneration.incrementAndGet()

        val token = getToken()
        if (token.isNullOrBlank()) {
            Log.d(tag, "Cannot connect: token is empty")
            return
        }

        val baseUrl = getBaseUrl().trimEnd('/')
        if (baseUrl.isBlank()) return

        val wsUrl = when {
            baseUrl.startsWith("https://") -> baseUrl.replaceFirst("https://", "wss://") + "/ws"
            baseUrl.startsWith("http://") -> baseUrl.replaceFirst("http://", "ws://") + "/ws"
            else -> "ws://$baseUrl/ws"
        }

        reconnectJob?.cancel()
        reconnectJob = null
        try {
            webSocket?.close(1000, "Reconnecting")
        } catch (_: Exception) {}
        webSocket = null
        currentSocket = null

        val gen = currentGen
        val request = Request.Builder()
            .url(wsUrl)
            .addHeader("Authorization", "Bearer $token")
            .build()

        Log.d(tag, "Connecting to WebSocket: $wsUrl (gen=$gen)")

        val listener = createWebSocketListener(gen, token)
        currentListenerForTesting = listener

        val newSocket = webSocketFactory.newWebSocket(request, listener)
        currentSocket = newSocket
        webSocket = newSocket
    }

    internal fun createWebSocketListener(gen: Long, token: String): WebSocketListener {
        return object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                if (gen != connectionGeneration.get()) {
                    Log.d(tag, "WebSocket onOpen ignored for stale generation $gen (current: ${connectionGeneration.get()})")
                    try {
                        webSocket.close(1000, "Stale connection")
                    } catch (_: Exception) {}
                    return
                }
                Log.d(tag, "WebSocket connected successfully (gen=$gen)")
                this@RealtimeWebSocketManager.webSocket = webSocket
                this@RealtimeWebSocketManager.currentSocket = webSocket
                isConnected = true

                try {
                    // Send first-message auth handshake as secondary confirmation
                    val authMsg = JSONObject().apply {
                        put("type", "auth")
                        put("token", token)
                    }
                    webSocket.send(authMsg.toString())

                    // Send presence initialization
                    val onlineMsg = JSONObject().apply { put("type", "user.online") }
                    webSocket.send(onlineMsg.toString())

                    // Query presence
                    val presenceQuery = JSONObject().apply {
                        put("type", "presence.query")
                    }
                    webSocket.send(presenceQuery.toString())

                    // Re-join active communities
                    joinedCommunities.forEach { commId ->
                        sendJoinCommunityRoom(commId)
                    }
                } catch (e: Exception) {
                    Log.e(tag, "Error in onOpen handshake: ${e.message}")
                }
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                if (gen != connectionGeneration.get()) return
                try {
                    val json = JSONObject(text)
                    val type = json.optString("type", "")
                    val payload = json.optJSONObject("payload") ?: JSONObject()

                    when (type) {
                        "chat.message" -> {
                            val msg = ChatMessage.fromJson(payload)
                            _events.tryEmit(RealtimeEvent.ChatMessageReceived(msg))
                        }
                        "community.message" -> {
                            val commId = payload.optString("communityId", "")
                            val msg = ChatMessage.fromJson(payload)
                            _events.tryEmit(RealtimeEvent.CommunityMessageReceived(commId, msg))
                        }
                        "friend.request" -> {
                            val reqObj = payload.optJSONObject("request") ?: payload
                            val req = FriendRequestItem.fromJson(reqObj)
                            _events.tryEmit(RealtimeEvent.FriendRequestReceived(req))
                        }
                        "friend.accepted" -> {
                            val friendId = payload.optString("friendId", "")
                            val friendName = payload.optString("friendName", "")
                            _events.tryEmit(RealtimeEvent.FriendAccepted(friendId, friendName))
                        }
                    }
                } catch (e: Exception) {
                    Log.e(tag, "Failed to parse incoming WS message: ${e.message}")
                }
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                Log.d(tag, "WebSocket closing (gen=$gen): $code / $reason")
                try {
                    webSocket.close(1000, null)
                } catch (_: Exception) {}
                isConnected = false
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                Log.d(tag, "WebSocket closed (gen=$gen): $code / $reason")
                isConnected = false
                if (intentionalDisconnect || code == 1000 || gen != connectionGeneration.get()) {
                    Log.d(tag, "Not reconnecting: intentionalDisconnect=$intentionalDisconnect, code=$code, genMatch=${gen == connectionGeneration.get()}")
                    return
                }
                scheduleReconnect()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.e(tag, "WebSocket failure (gen=$gen): ${t.message}")
                isConnected = false
                if (intentionalDisconnect || gen != connectionGeneration.get()) {
                    Log.d(tag, "Not reconnecting on failure: intentionalDisconnect=$intentionalDisconnect, genMatch=${gen == connectionGeneration.get()}")
                    return
                }
                scheduleReconnect()
            }
        }
    }

    fun joinCommunity(communityId: String) {
        val cleanId = communityId.trim()
        if (cleanId.isBlank()) return
        joinedCommunities.add(cleanId)
        if (isConnected) {
            sendJoinCommunityRoom(cleanId)
        }
    }

    fun leaveCommunity(communityId: String) {
        val cleanId = communityId.trim()
        if (cleanId.isBlank()) return
        joinedCommunities.remove(cleanId)
        if (isConnected) {
            sendLeaveCommunityRoom(cleanId)
        }
    }

    private fun sendJoinCommunityRoom(communityId: String) {
        try {
            val joinMsg = JSONObject().apply {
                put("type", "community.join_room")
                put("payload", JSONObject().apply {
                    put("communityId", communityId)
                })
            }
            webSocket?.send(joinMsg.toString())
        } catch (_: Exception) {}
    }

    private fun sendLeaveCommunityRoom(communityId: String) {
        try {
            val leaveMsg = JSONObject().apply {
                put("type", "community.leave_room")
                put("payload", JSONObject().apply {
                    put("communityId", communityId)
                })
            }
            webSocket?.send(leaveMsg.toString())
        } catch (_: Exception) {}
    }

    private fun scheduleReconnect() {
        val am = authManager
        if (am != null) {
            if (intentionalDisconnect || !am.isSignedIn()) return
        } else {
            if (intentionalDisconnect || getToken().isNullOrBlank()) return
        }

        reconnectScheduleCount++
        reconnectJob?.cancel()
        reconnectJob = scope.launch {
            delay(3000)
            if (isActive && !isConnected) {
                val currentAm = authManager
                if (currentAm != null) {
                    if (intentionalDisconnect || !currentAm.isSignedIn()) return@launch
                } else {
                    if (intentionalDisconnect || getToken().isNullOrBlank()) return@launch
                }
                connect()
            }
        }
    }

    fun disconnect() {
        intentionalDisconnect = true
        connectionGeneration.incrementAndGet()
        reconnectJob?.cancel()
        reconnectJob = null
        try {
            webSocket?.close(1000, "App closed or signed out")
        } catch (_: Exception) {}
        joinedCommunities.clear()
        currentSocket = null
        webSocket = null
        currentListenerForTesting = null
        isConnected = false
    }
}
