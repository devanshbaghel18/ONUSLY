package com.onusly.app.data.api

import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.ApprovalResult
import com.onusly.app.data.model.ChatMessage
import com.onusly.app.data.model.CommunityItem
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.FriendItem
import com.onusly.app.data.model.FriendRequestItem
import com.onusly.app.data.model.FriendRequestsResponse
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.GoalParser
import com.onusly.app.data.model.Proof
import com.onusly.app.data.model.PublicProfile
import com.onusly.app.data.model.UpdateGoalRequest
import com.onusly.app.data.model.UserProfile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.json.JSONObject
import java.io.IOException
import java.net.URLEncoder
import java.util.concurrent.TimeUnit

class OnuslyApiException(
    val statusCode: Int,
    override val message: String
) : Exception(message)

data class ApprovalResponse(
    val id: String,
    val goalId: String,
    val proofId: String,
    val approverId: String,
    val decision: String,
    val comment: String = "",
    val decidedAt: String = ""
) {
    companion object {
        fun fromJson(json: JSONObject): ApprovalResponse {
            val decision = when {
                json.has("decision") -> json.optString("decision", "")
                json.has("status") -> json.optString("status", "")
                else -> ""
            }
            return ApprovalResponse(
                id = json.optString("id", ""),
                goalId = json.optString("goalId", ""),
                proofId = json.optString("proofId", ""),
                approverId = json.optString("approverId", ""),
                decision = decision,
                comment = json.optString("comment", ""),
                decidedAt = json.optString("decidedAt", "")
            )
        }

        fun parseList(body: String): List<ApprovalResponse> {
            val list = mutableListOf<ApprovalResponse>()
            val trimmed = body.trim()
            if (trimmed.isNotEmpty() && trimmed != "null") {
                val jsonArr = org.json.JSONArray(trimmed)
                for (i in 0 until jsonArr.length()) {
                    list.add(fromJson(jsonArr.getJSONObject(i)))
                }
            }
            return list
        }
    }
}

class OnuslyApiClient(
    private val httpClient: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .build(),
    var defaultBaseUrl: String = "http://10.0.2.2:8080",
    var defaultJwtToken: String = ""
) {

    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    fun extractErrorMessage(response: Response, responseBody: String): String {
        val trimmedBody = responseBody.trim()
        val rawMessage = try {
            val json = JSONObject(trimmedBody)
            when {
                json.has("error") && json.optString("error").isNotBlank() -> json.optString("error").trim()
                json.has("message") && json.optString("message").isNotBlank() -> json.optString("message").trim()
                else -> trimmedBody.ifBlank { "HTTP ${response.code}" }
            }
        } catch (_: Exception) {
            trimmedBody.ifBlank { "HTTP ${response.code}" }
        }
        return rawMessage.take(300)
    }

    fun extractErrorMessage(response: Response): String {
        val bodyString = try {
            response.peekBody(1024 * 1024).string()
        } catch (_: Exception) {
            ""
        }
        return extractErrorMessage(response, bodyString)
    }

    private suspend fun executeRequest(request: Request): Result<String> = withContext(Dispatchers.IO) {
        try {
            val response = httpClient.newCall(request).execute()
            val bodyString = response.body?.string() ?: ""

            if (!response.isSuccessful) {
                val errorMsg = extractErrorMessage(response, bodyString)
                Result.failure(OnuslyApiException(response.code, errorMsg))
            } else {
                Result.success(bodyString)
            }
        } catch (e: IOException) {
            Result.failure(e)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // --- Goals API ---

    suspend fun getGoals(baseUrl: String, jwtToken: String): Result<List<Goal>> {
        val url = "${baseUrl.trimEnd('/')}/goals"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseGoals(body)
        }
    }

    suspend fun getGoal(baseUrl: String, jwtToken: String, goalId: String, ownerId: String? = null): Result<Goal> {
        val query = if (!ownerId.isNullOrBlank()) {
            "?ownerId=${URLEncoder.encode(ownerId, "UTF-8")}"
        } else ""
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}$query"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseGoal(JSONObject(body))
        }
    }

    suspend fun createGoal(baseUrl: String, jwtToken: String, requestData: CreateGoalRequest): Result<Goal> {
        val url = "${baseUrl.trimEnd('/')}/goals"
        val jsonPayload = GoalParser.createGoalRequestToJson(requestData)
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(jsonPayload.toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseGoal(JSONObject(body))
        }
    }

    suspend fun updateGoal(
        baseUrl: String,
        jwtToken: String,
        goalId: String,
        requestData: UpdateGoalRequest
    ): Result<Goal> {
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}"
        val jsonPayload = GoalParser.updateGoalRequestToJson(requestData)
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .patch(jsonPayload.toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseGoal(JSONObject(body))
        }
    }

    suspend fun deleteGoal(baseUrl: String, jwtToken: String, goalId: String): Result<Unit> {
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .delete()
            .build()

        return executeRequest(request).map { }
    }

    // --- Proofs API ---

    suspend fun submitProof(
        baseUrl: String,
        jwtToken: String,
        goalId: String,
        requestData: CreateProofRequest
    ): Result<Proof> {
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}/proofs"
        val jsonPayload = GoalParser.createProofRequestToJson(requestData)
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(jsonPayload.toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseProof(JSONObject(body))
        }
    }

    suspend fun getProofs(
        baseUrl: String,
        jwtToken: String,
        goalId: String,
        ownerId: String? = null
    ): Result<List<Proof>> {
        val query = if (!ownerId.isNullOrBlank()) {
            "?ownerId=${URLEncoder.encode(ownerId, "UTF-8")}"
        } else ""
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}/proofs$query"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseProofs(body)
        }
    }

    // --- Approvals API ---

    suspend fun getApprovals(
        baseUrl: String,
        jwtToken: String,
        goalId: String
    ): Result<List<ApprovalResponse>> {
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}/approvals"
        val request = Request.Builder()
            .url(url)
            .apply {
                if (jwtToken.isNotBlank()) {
                    addHeader("Authorization", "Bearer $jwtToken")
                }
            }
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            ApprovalResponse.parseList(body)
        }
    }

    suspend fun getApprovals(goalId: String): Result<List<ApprovalResponse>> {
        return getApprovals(defaultBaseUrl, defaultJwtToken, goalId)
    }

    suspend fun decideApproval(
        baseUrl: String,
        jwtToken: String,
        goalId: String,
        proofId: String,
        requestData: ApprovalDecisionRequest
    ): Result<ApprovalResult> {
        val url = "${baseUrl.trimEnd('/')}/goals/${URLEncoder.encode(goalId, "UTF-8")}/proofs/${URLEncoder.encode(proofId, "UTF-8")}/approval"
        val jsonPayload = GoalParser.approvalDecisionRequestToJson(requestData)
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(jsonPayload.toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseApprovalResult(body)
        }
    }

    // --- Users API ---

    suspend fun getMyProfile(baseUrl: String, jwtToken: String): Result<UserProfile> {
        val url = "${baseUrl.trimEnd('/')}/users/me"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseUserProfile(body)
        }
    }

    suspend fun updateHandle(baseUrl: String, jwtToken: String, newHandle: String): Result<UserProfile> {
        val url = "${baseUrl.trimEnd('/')}/users/handle"
        val clean = newHandle.trim().removePrefix("@")
        val jsonPayload = JSONObject().apply { put("handle", clean) }.toString()
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .patch(jsonPayload.toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parseUserProfile(body)
        }
    }

    suspend fun lookupUser(baseUrl: String, jwtToken: String, handle: String): Result<PublicProfile> {
        val clean = handle.trim().removePrefix("@")
        val url = "${baseUrl.trimEnd('/')}/users/search?handle=${URLEncoder.encode(clean, "UTF-8")}"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).map { body ->
            GoalParser.parsePublicProfile(body)
        }
    }

    // --- Chat API ---

    suspend fun getChatHistory(
        baseUrl: String,
        jwtToken: String,
        peer: String
    ): Result<List<ChatMessage>> {
        val url = "${baseUrl.trimEnd('/')}/chat/messages?peer=${URLEncoder.encode(peer, "UTF-8")}"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            val list = mutableListOf<ChatMessage>()
            val trimmed = body.trim()
            if (trimmed.isNotEmpty() && trimmed != "null") {
                val jsonArr = org.json.JSONArray(trimmed)
                for (i in 0 until jsonArr.length()) {
                    list.add(ChatMessage.fromJson(jsonArr.getJSONObject(i)))
                }
            }
            list
        }
    }

    suspend fun sendChatMessage(
        baseUrl: String,
        jwtToken: String,
        recipient: String,
        text: String,
        isProof: Boolean = false,
        goalId: String = "",
        goalTitle: String = ""
    ): Result<ChatMessage> {
        val url = "${baseUrl.trimEnd('/')}/chat/messages"
        val json = JSONObject().apply {
            put("recipient", recipient)
            put("text", text)
            if (isProof) put("isProof", true)
            if (goalId.isNotBlank()) put("goalId", goalId)
            if (goalTitle.isNotBlank()) put("goalTitle", goalTitle)
        }
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(json.toString().toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).mapCatching { body ->
            ChatMessage.fromJson(JSONObject(body))
        }
    }

    suspend fun getFriends(baseUrl: String, jwtToken: String): Result<List<FriendItem>> {
        val url = "${baseUrl.trimEnd('/')}/friends"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            val list = mutableListOf<FriendItem>()
            val trimmed = body.trim()
            if (trimmed.isNotEmpty() && trimmed != "null") {
                val jsonArr = org.json.JSONArray(trimmed)
                for (i in 0 until jsonArr.length()) {
                    list.add(FriendItem.fromJson(jsonArr.getJSONObject(i)))
                }
            }
            list
        }
    }

    suspend fun sendFriendRequest(
        baseUrl: String,
        jwtToken: String,
        target: String
    ): Result<FriendRequestItem> {
        val url = "${baseUrl.trimEnd('/')}/friends/request"
        val json = JSONObject().apply {
            put("target", target)
        }
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(json.toString().toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).mapCatching { body ->
            FriendRequestItem.fromJson(JSONObject(body))
        }
    }

    suspend fun getFriendRequests(baseUrl: String, jwtToken: String): Result<FriendRequestsResponse> {
        val url = "${baseUrl.trimEnd('/')}/friends/requests"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            val trimmed = body.trim()
            if (trimmed.isEmpty() || trimmed == "null") {
                FriendRequestsResponse(emptyList(), emptyList())
            } else {
                FriendRequestsResponse.fromJson(JSONObject(trimmed))
            }
        }
    }

    suspend fun respondFriendRequest(
        baseUrl: String,
        jwtToken: String,
        requestId: String,
        action: String
    ): Result<Unit> {
        val url = "${baseUrl.trimEnd('/')}/friends/request/respond"
        val json = JSONObject().apply {
            put("requestId", requestId)
            put("action", action)
        }
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(json.toString().toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).mapCatching { }
    }

    suspend fun getTrendingCommunities(baseUrl: String, jwtToken: String): Result<List<CommunityItem>> {
        val url = "${baseUrl.trimEnd('/')}/communities/trending"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            val list = mutableListOf<CommunityItem>()
            val trimmed = body.trim()
            if (trimmed.isNotEmpty() && trimmed != "null") {
                val jsonArr = org.json.JSONArray(trimmed)
                for (i in 0 until jsonArr.length()) {
                    list.add(CommunityItem.fromJson(jsonArr.getJSONObject(i)))
                }
            }
            list
        }
    }

    suspend fun getJoinedCommunities(baseUrl: String, jwtToken: String): Result<List<CommunityItem>> {
        val url = "${baseUrl.trimEnd('/')}/communities/joined"
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .get()
            .build()

        return executeRequest(request).mapCatching { body ->
            val list = mutableListOf<CommunityItem>()
            val trimmed = body.trim()
            if (trimmed.isNotEmpty() && trimmed != "null") {
                val jsonArr = org.json.JSONArray(trimmed)
                for (i in 0 until jsonArr.length()) {
                    list.add(CommunityItem.fromJson(jsonArr.getJSONObject(i)))
                }
            }
            list
        }
    }

    suspend fun joinCommunity(baseUrl: String, jwtToken: String, communityId: String): Result<Unit> {
        val url = "${baseUrl.trimEnd('/')}/communities/join"
        val json = JSONObject().apply {
            put("communityId", communityId)
        }
        val request = Request.Builder()
            .url(url)
            .addHeader("Authorization", "Bearer $jwtToken")
            .post(json.toString().toRequestBody(jsonMediaType))
            .build()

        return executeRequest(request).mapCatching { }
    }
}
