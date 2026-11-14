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
    val blockedDomains: List<String> = emptyList(),
    val applicationTargets: List<ApplicationTarget> = emptyList()
) {
    val isTargeted: Boolean
        get() = blockedApps.isNotEmpty() || blockedDomains.isNotEmpty() || applicationTargets.isNotEmpty()

    val isActive: Boolean
        get() = status.equals("active", ignoreCase = true)

    val isProofSubmitted: Boolean
        get() = status.equals("proof_submitted", ignoreCase = true)

    val isRejected: Boolean
        get() = status.equals("rejected", ignoreCase = true)

    val isCompleted: Boolean
        get() = status.equals("completed", ignoreCase = true)
}

data class CreateGoalRequest(
    val title: String,
    val description: String = "",
    val approvalType: String? = null,
    val approverEmail: String? = null,
    val blockedApps: List<String> = emptyList(),
    val blockedDomains: List<String> = emptyList(),
    val applicationTargets: List<ApplicationTarget> = emptyList()
)

data class UpdateGoalRequest(
    val title: String? = null,
    val description: String? = null,
    val approvalType: String? = null,
    val approverEmail: String? = null,
    val blockedApps: List<String>? = null,
    val blockedDomains: List<String>? = null,
    val applicationTargets: List<ApplicationTarget>? = null
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

data class ApproverInboxItem(
    val goalId: String,
    val ownerId: String,
    val ownerName: String = "",
    val ownerHandle: String = "",
    val ownerEmail: String = "",
    val title: String,
    val description: String = "",
    val status: String,
    val createdAt: String = "",
    val updatedAt: String = "",
    val blockedApps: List<String> = emptyList(),
    val blockedDomains: List<String> = emptyList(),
    val applicationTargets: List<ApplicationTarget> = emptyList(),
    val latestProof: Proof? = null,
    val proofCount: Int = 0
) {
    fun toGoal(): Goal {
        return Goal(
            id = goalId,
            ownerId = ownerId,
            title = title,
            description = description,
            status = status,
            createdAt = createdAt,
            updatedAt = updatedAt,
            blockedApps = blockedApps,
            blockedDomains = blockedDomains,
            applicationTargets = applicationTargets
        )
    }
}

data class ApproverInboxResponse(
    val items: List<ApproverInboxItem> = emptyList(),
    val total: Int = 0
)

data class UploadedFileItem(
    val id: String,
    val name: String,
    val size: Long = 0,
    val formattedSize: String = "",
    val type: String = "",
    val url: String,
    val dataUrl: String = "",
    val isImage: Boolean = true,
    val isPdf: Boolean = false
)
