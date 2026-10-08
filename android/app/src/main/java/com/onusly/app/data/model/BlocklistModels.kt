package com.onusly.app.data.model

data class BlocklistResponse(
    val userId: String,
    val version: String,
    val generatedAt: String,
    val targets: List<BlocklistTarget>
)

data class BlocklistTarget(
    val goalId: String,
    val goalTitle: String,
    val status: String,
    val apps: List<String>,
    val domains: List<String>
)

data class CachedBlocklist(
    val userId: String,
    val version: String,
    val generatedAt: String,
    val lastSyncTimestamp: Long,
    val targets: List<BlocklistTarget>
)

data class UserSession(
    val userId: String,
    val email: String,
    val name: String,
    val jwtToken: String
)

sealed class SyncResult {
    data class Success(val response: BlocklistResponse) : SyncResult()
    object NotModified : SyncResult()
    data class Error(val statusCode: Int, val message: String) : SyncResult()
}

data class BlockEvaluationResult(
    val isBlocked: Boolean,
    val matchingTargets: List<BlocklistTarget>
)
