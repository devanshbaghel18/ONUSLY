package com.onusly.app.data.model

import org.json.JSONArray
import org.json.JSONObject

data class ChatMessage(
    val id: String,
    val senderId: String,
    val senderEmail: String,
    val senderHandle: String,
    val recipientId: String,
    val recipientEmail: String,
    val recipientHandle: String,
    val text: String,
    val isProof: Boolean,
    val goalId: String,
    val goalTitle: String,
    val ownerId: String,
    val approved: Boolean,
    val time: String
) {
    companion object {
        fun fromJson(json: JSONObject): ChatMessage {
            return ChatMessage(
                id = json.optString("id", ""),
                senderId = json.optString("senderId", ""),
                senderEmail = json.optString("senderEmail", ""),
                senderHandle = json.optString("senderHandle", ""),
                recipientId = json.optString("recipientId", ""),
                recipientEmail = json.optString("recipientEmail", ""),
                recipientHandle = json.optString("recipientHandle", ""),
                text = json.optString("text", ""),
                isProof = json.optBoolean("isProof", false),
                goalId = json.optString("goalId", ""),
                goalTitle = json.optString("goalTitle", ""),
                ownerId = json.optString("ownerId", ""),
                approved = json.optBoolean("approved", false),
                time = json.optString("time", "")
            )
        }
    }
}

data class FriendItem(
    val userId: String,
    val friendId: String,
    val friendHandle: String,
    val friendEmail: String,
    val friendName: String,
    val createdAt: String
) {
    companion object {
        fun fromJson(json: JSONObject): FriendItem {
            return FriendItem(
                userId = json.optString("userId", ""),
                friendId = json.optString("friendId", ""),
                friendHandle = json.optString("friendHandle", ""),
                friendEmail = json.optString("friendEmail", ""),
                friendName = json.optString("friendName", ""),
                createdAt = json.optString("createdAt", "")
            )
        }
    }
}

data class FriendRequestItem(
    val id: String,
    val senderId: String,
    val senderHandle: String,
    val senderEmail: String,
    val senderName: String,
    val recipientId: String,
    val recipientHandle: String,
    val recipientEmail: String,
    val recipientName: String,
    val status: String,
    val createdAt: String
) {
    companion object {
        fun fromJson(json: JSONObject): FriendRequestItem {
            return FriendRequestItem(
                id = json.optString("id", ""),
                senderId = json.optString("senderId", ""),
                senderHandle = json.optString("senderHandle", ""),
                senderEmail = json.optString("senderEmail", ""),
                senderName = json.optString("senderName", ""),
                recipientId = json.optString("recipientId", ""),
                recipientHandle = json.optString("recipientHandle", ""),
                recipientEmail = json.optString("recipientEmail", ""),
                recipientName = json.optString("recipientName", ""),
                status = json.optString("status", "pending"),
                createdAt = json.optString("createdAt", "")
            )
        }
    }
}

data class FriendRequestsResponse(
    val incoming: List<FriendRequestItem>,
    val outgoing: List<FriendRequestItem>
) {
    companion object {
        fun fromJson(json: JSONObject): FriendRequestsResponse {
            val incomingList = mutableListOf<FriendRequestItem>()
            val incArr = json.optJSONArray("incoming") ?: JSONArray()
            for (i in 0 until incArr.length()) {
                incArr.optJSONObject(i)?.let { incomingList.add(FriendRequestItem.fromJson(it)) }
            }

            val outgoingList = mutableListOf<FriendRequestItem>()
            val outArr = json.optJSONArray("outgoing") ?: JSONArray()
            for (i in 0 until outArr.length()) {
                outArr.optJSONObject(i)?.let { outgoingList.add(FriendRequestItem.fromJson(it)) }
            }

            return FriendRequestsResponse(
                incoming = incomingList,
                outgoing = outgoingList
            )
        }
    }
}

data class CommunityItem(
    val id: String,
    val name: String,
    val description: String,
    val category: String,
    val members: Int,
    val streak: String,
    val isJoined: Boolean
) {
    companion object {
        fun fromJson(json: JSONObject): CommunityItem {
            return CommunityItem(
                id = json.optString("id", ""),
                name = json.optString("name", ""),
                description = json.optString("description", ""),
                category = json.optString("category", "General"),
                members = json.optInt("members", 1),
                streak = json.optString("streak", ""),
                isJoined = json.optBoolean("isJoined", false)
            )
        }
    }
}

sealed class ConversationItem {
    abstract val id: String
    abstract val name: String
    abstract val subtitle: String
    abstract val time: String
    abstract val unreadCount: Int

    data class FriendConversation(
        val friend: FriendItem,
        override val id: String = friend.friendId,
        override val name: String = friend.friendName.ifBlank {
            if (friend.friendHandle.isNotBlank()) "@${friend.friendHandle.removePrefix("@")}" else friend.friendEmail
        },
        override val subtitle: String = if (friend.friendHandle.isNotBlank()) "@${friend.friendHandle.removePrefix("@")}" else friend.friendEmail,
        override val time: String = "",
        override val unreadCount: Int = 0
    ) : ConversationItem()

    data class CommunityConversation(
        val community: CommunityItem,
        override val id: String = community.id,
        override val name: String = community.name,
        override val subtitle: String = "[Community] ${community.category} • ${community.members} members",
        override val time: String = "",
        override val unreadCount: Int = 0
    ) : ConversationItem()
}
