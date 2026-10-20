package com.onusly.app.data

import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.CommunityItem
import com.onusly.app.data.model.FriendItem
import com.onusly.app.data.model.FriendRequestItem
import com.onusly.app.data.model.FriendRequestsResponse

class ChatRepository(
    private val authManager: AuthManager,
    private val syncRepository: SyncRepository,
    private val apiClient: OnuslyApiClient = OnuslyApiClient()
) {
    suspend fun getChatHistory(peer: String): Result<List<ChatMessage>> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.getChatHistory(baseUrl, token, peer)
    }

    suspend fun sendMessage(
        recipient: String,
        text: String,
        isProof: Boolean = false,
        goalId: String = "",
        goalTitle: String = ""
    ): Result<ChatMessage> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.sendChatMessage(baseUrl, token, recipient, text, isProof, goalId, goalTitle)
    }

    suspend fun getFriends(): Result<List<FriendItem>> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.getFriends(baseUrl, token)
    }

    suspend fun sendFriendRequest(target: String): Result<FriendRequestItem> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.sendFriendRequest(baseUrl, token, target)
    }

    suspend fun getFriendRequests(): Result<FriendRequestsResponse> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.getFriendRequests(baseUrl, token)
    }

    suspend fun respondFriendRequest(requestId: String, action: String): Result<Unit> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.respondFriendRequest(baseUrl, token, requestId, action)
    }

    suspend fun getTrendingCommunities(): Result<List<CommunityItem>> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.getTrendingCommunities(baseUrl, token)
    }

    suspend fun getJoinedCommunities(): Result<List<CommunityItem>> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.getJoinedCommunities(baseUrl, token)
    }

    suspend fun joinCommunity(communityId: String): Result<Unit> {
        val token = authManager.getJwtToken() ?: return Result.failure(Exception("Not authenticated"))
        val baseUrl = syncRepository.getBaseUrl()
        return apiClient.joinCommunity(baseUrl, token, communityId)
    }
}
