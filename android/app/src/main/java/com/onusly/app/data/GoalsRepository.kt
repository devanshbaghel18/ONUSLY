package com.onusly.app.data

import android.content.Context
import com.onusly.app.data.api.OnuslyApiClient
import com.onusly.app.data.api.OnuslyApiException
import com.onusly.app.data.auth.AuthManager
import com.onusly.app.data.model.ApprovalDecisionRequest
import com.onusly.app.data.model.ApprovalResult
import com.onusly.app.data.model.CreateGoalRequest
import com.onusly.app.data.model.CreateProofRequest
import com.onusly.app.data.model.Goal
import com.onusly.app.data.model.Proof
import com.onusly.app.data.model.PublicProfile
import com.onusly.app.data.model.SyncResult
import com.onusly.app.data.model.UpdateGoalRequest
import com.onusly.app.data.model.UserProfile
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class MutationResult<T>(
    val value: T,
    val syncSucceeded: Boolean,
    val syncError: String? = null
)

class GoalsRepository(
    private val authManager: AuthManager,
    private val syncRepository: SyncRepository,
    private val apiClient: OnuslyApiClient
) {

    private val _goalsFlow = MutableStateFlow<List<Goal>>(emptyList())
    val goalsFlow: StateFlow<List<Goal>> = _goalsFlow.asStateFlow()

    private val _isOfflineFlow = MutableStateFlow(false)
    val isOfflineFlow: StateFlow<Boolean> = _isOfflineFlow.asStateFlow()

    init {
        syncRepository.addSignOutListener {
            clearLocalData()
        }
    }

    companion object {
        fun create(
            context: Context,
            syncRepository: SyncRepository,
            authManager: AuthManager
        ): GoalsRepository {
            return GoalsRepository(
                authManager = authManager,
                syncRepository = syncRepository,
                apiClient = OnuslyApiClient()
            )
        }
    }

    private fun getAuth(): Pair<String, String>? {
        val session = authManager.getSession() ?: return null
        if (session.jwtToken.isBlank() || session.userId.isBlank()) return null
        return Pair(syncRepository.getBaseUrl(), session.jwtToken)
    }

    suspend fun fetchGoals(): Result<List<Goal>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()

        val result = apiClient.getGoals(baseUrl, token)
        result.onSuccess { goals ->
            if (!callerUserId.isNullOrBlank() && authManager.getUserId() == callerUserId) {
                _isOfflineFlow.value = false
                _goalsFlow.value = goals
            }
        }.onFailure { err ->
            if (!callerUserId.isNullOrBlank() && authManager.getUserId() == callerUserId) {
                if ((err as? OnuslyApiException)?.statusCode == 401) {
                    syncRepository.notifySessionExpiration()
                } else {
                    _isOfflineFlow.value = true
                }
            }
        }
        return result
    }

    suspend fun fetchGoal(goalId: String, ownerId: String? = null): Result<Goal> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        return apiClient.getGoal(baseUrl, token, goalId, ownerId)
    }

    suspend fun createGoal(request: CreateGoalRequest): Result<MutationResult<Goal>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.createGoal(baseUrl, token, request)
        return result.map { createdGoal ->
            if (authManager.getUserId() == callerUserId) {
                val updated = _goalsFlow.value.toMutableList()
                updated.add(0, createdGoal)
                _goalsFlow.value = updated

                val syncRes = syncRepository.syncBlocklist()
                val syncOk = syncRes is SyncResult.Success || syncRes is SyncResult.NotModified
                val syncErr = if (syncRes is SyncResult.Error) syncRes.message else null
                MutationResult(createdGoal, syncOk, syncErr)
            } else {
                MutationResult(createdGoal, syncSucceeded = false, syncError = "Account changed during operation")
            }
        }
    }

    suspend fun updateGoal(goalId: String, request: UpdateGoalRequest): Result<MutationResult<Goal>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.updateGoal(baseUrl, token, goalId, request)
        return result.map { updatedGoal ->
            if (authManager.getUserId() == callerUserId) {
                val updated = _goalsFlow.value.map { if (it.id == goalId) updatedGoal else it }
                _goalsFlow.value = updated

                val syncRes = syncRepository.syncBlocklist()
                val syncOk = syncRes is SyncResult.Success || syncRes is SyncResult.NotModified
                val syncErr = if (syncRes is SyncResult.Error) syncRes.message else null
                MutationResult(updatedGoal, syncOk, syncErr)
            } else {
                MutationResult(updatedGoal, syncSucceeded = false, syncError = "Account changed during operation")
            }
        }
    }

    suspend fun deleteGoal(goalId: String): Result<MutationResult<Unit>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.deleteGoal(baseUrl, token, goalId)
        return result.map {
            if (authManager.getUserId() == callerUserId) {
                val updated = _goalsFlow.value.filterNot { it.id == goalId }
                _goalsFlow.value = updated

                val syncRes = syncRepository.syncBlocklist()
                val syncOk = syncRes is SyncResult.Success || syncRes is SyncResult.NotModified
                val syncErr = if (syncRes is SyncResult.Error) syncRes.message else null
                MutationResult(Unit, syncOk, syncErr)
            } else {
                MutationResult(Unit, syncSucceeded = false, syncError = "Account changed during operation")
            }
        }
    }

    suspend fun submitProof(goalId: String, request: CreateProofRequest): Result<MutationResult<Proof>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.submitProof(baseUrl, token, goalId, request)
        return result.map { proof ->
            if (authManager.getUserId() == callerUserId) {
                val updated = _goalsFlow.value.map {
                    if (it.id == goalId) it.copy(status = "proof_submitted") else it
                }
                _goalsFlow.value = updated

                val syncRes = syncRepository.syncBlocklist()
                val syncOk = syncRes is SyncResult.Success || syncRes is SyncResult.NotModified
                val syncErr = if (syncRes is SyncResult.Error) syncRes.message else null
                MutationResult(proof, syncOk, syncErr)
            } else {
                MutationResult(proof, syncSucceeded = false, syncError = "Account changed during operation")
            }
        }
    }

    suspend fun getProofs(goalId: String, ownerId: String? = null): Result<List<Proof>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        return apiClient.getProofs(baseUrl, token, goalId, ownerId)
    }

    suspend fun getApprovals(goalId: String): Result<List<ApprovalResult>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.getApprovals(baseUrl, token, goalId)
        return result.map { list ->
            list.map { resp ->
                ApprovalResult(
                    id = resp.id,
                    goalId = resp.goalId,
                    proofId = resp.proofId,
                    ownerId = "",
                    approverId = resp.approverId,
                    status = resp.decision,
                    comment = resp.comment,
                    decidedAt = resp.decidedAt
                )
            }
        }
    }

    suspend fun decideApproval(
        goalId: String,
        proofId: String,
        request: ApprovalDecisionRequest
    ): Result<MutationResult<ApprovalResult>> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))
        val callerUserId = authManager.getUserId()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        val result = apiClient.decideApproval(baseUrl, token, goalId, proofId, request)
        return result.map { decision ->
            if (authManager.getUserId() == callerUserId) {
                val newStatus = if (decision.status.equals("approved", ignoreCase = true)) "completed" else "active"
                val updated = _goalsFlow.value.map {
                    if (it.id == goalId) it.copy(status = newStatus) else it
                }
                _goalsFlow.value = updated

                // Only sync local blocklist if the local user is the owner of the goal being decided
                val isLocalOwner = request.ownerId == callerUserId
                val (syncOk, syncErr) = if (isLocalOwner) {
                    val syncRes = syncRepository.syncBlocklist()
                    val ok = syncRes is SyncResult.Success || syncRes is SyncResult.NotModified
                    val err = if (syncRes is SyncResult.Error) syncRes.message else null
                    Pair(ok, err)
                } else {
                    Pair(true, null)
                }
                MutationResult(decision, syncOk, syncErr)
            } else {
                MutationResult(decision, syncSucceeded = false, syncError = "Account changed during operation")
            }
        }
    }

    suspend fun retryBlocklistSync(): Result<Unit> {
        return when (val res = syncRepository.syncBlocklist()) {
            is SyncResult.Success, is SyncResult.NotModified -> Result.success(Unit)
            is SyncResult.Error -> Result.failure(OnuslyApiException(res.statusCode, res.message))
        }
    }

    suspend fun getMyProfile(): Result<UserProfile> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        return apiClient.getMyProfile(baseUrl, token)
    }

    suspend fun updateHandle(newHandle: String): Result<UserProfile> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        return apiClient.updateHandle(baseUrl, token, newHandle)
    }

    suspend fun lookupUser(handle: String): Result<PublicProfile> {
        val (baseUrl, token) = getAuth()
            ?: return Result.failure(OnuslyApiException(401, "User is not signed in"))

        return apiClient.lookupUser(baseUrl, token, handle)
    }

    fun clearLocalData() {
        _goalsFlow.value = emptyList()
        _isOfflineFlow.value = false
    }

    fun setOffline(isOffline: Boolean) {
        _isOfflineFlow.value = isOffline
    }
}
