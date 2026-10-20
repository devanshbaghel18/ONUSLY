package com.onusly.app.data.model

data class Goal(
    val id: String,
    val ownerId: String,
    val approverId: String = "",
    val approverEmail: String = "",
    val approvalType: String = "none",
    val title: String,
    val description: String = "",
    val status: String,
    val createdAt: String = "",
    val updatedAt: String = "",
    val blockedApps: List<String> = emptyList(),
    val blockedDomains: List<String> = emptyList()
) {
    val isTargeted: Boolean
        get() = blockedApps.isNotEmpty() || blockedDomains.isNotEmpty()

    val isActive: Boolean
        get() = status.equals("active", ignoreCase = true)

    val isProofSubmitted: Boolean
        get() = status.equals("proof_submitted", ignoreCase = true)

    val isCompleted: Boolean
        get() = status.equals("completed", ignoreCase = true)
}

data class CreateGoalRequest(
    val title: String,
    val description: String = "",
    val approvalType: String? = null,
    val approverEmail: String? = null,
    val blockedApps: List<String> = emptyList(),
    val blockedDomains: List<String> = emptyList()
)

data class UpdateGoalRequest(
    val title: String? = null,
    val description: String? = null,
    val approvalType: String? = null,
    val approverEmail: String? = null,
    val blockedApps: List<String>? = null,
    val blockedDomains: List<String>? = null
)

data class Proof(
    val id: String,
    val goalId: String,
    val ownerId: String,
    val proofType: String,
    val textExplanation: String = "",
    val externalLink: String = "",
    val photoUrl: String = "",
    val submittedAt: String = ""
)

data class CreateProofRequest(
    val proofType: String,
    val textExplanation: String = "",
    val externalLink: String = "",
    val photoUrl: String = ""
)

data class ApprovalDecisionRequest(
    val ownerId: String,
    val status: String,
    val comment: String = ""
)

data class ApprovalResult(
    val id: String,
    val goalId: String,
    val proofId: String,
    val ownerId: String,
    val approverId: String,
    val status: String,
    val comment: String = "",
    val decidedAt: String = ""
)
