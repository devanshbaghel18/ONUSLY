package com.onusly.app.data.realtime

import android.util.Log
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
import java.util.concurrent.TimeUnit

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
        .build()
) {
    private val tag = "RealtimeWSManager"
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var webSocket: WebSocket? = null
    private var isConnected = false
    private var reconnectJob: Job? = null
    private val joinedCommunities = mutableSetOf<String>()

    private val _events = MutableSharedFlow<RealtimeEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<RealtimeEvent> = _events.asSharedFlow()

    fun connect() {
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
        try {
            webSocket?.close(1000, "Reconnecting")
        } catch (_: Exception) {}
        webSocket = null

        val request = Request.Builder()
            .url(wsUrl)
            .addHeader("Authorization", "Bearer $token")
            .build()

        Log.d(tag, "Connecting to WebSocket: $wsUrl")

        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Log.d(tag, "WebSocket connected successfully")
                isConnected = true

                try {
                    // Send first-message auth handshake as secondary confirmation
                    val authMsg = JSONObject().apply {
                        put("type", "auth")
                        put("token", token)
                    }
                    webSocket.send(authMsg.toString())

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
                Log.d(tag, "WebSocket closing: $code / $reason")
                webSocket.close(1000, null)
                isConnected = false
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                Log.d(tag, "WebSocket closed: $code / $reason")
                isConnected = false
                scheduleReconnect()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.e(tag, "WebSocket failure: ${t.message}")
                isConnected = false
                scheduleReconnect()
            }
        })
    }

    fun joinCommunity(communityId: String) {
        val cleanId = communityId.trim()
        if (cleanId.isBlank()) return
        joinedCommunities.add(cleanId)
        if (isConnected) {
            sendJoinCommunityRoom(cleanId)
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

    private fun scheduleReconnect() {
        reconnectJob?.cancel()
        reconnectJob = scope.launch {
            delay(3000)
            if (isActive && !isConnected) {
                connect()
            }
        }
    }

    fun disconnect() {
        reconnectJob?.cancel()
        try {
            webSocket?.close(1000, "App closed")
        } catch (_: Exception) {}
        webSocket = null
        isConnected = false
    }
}
