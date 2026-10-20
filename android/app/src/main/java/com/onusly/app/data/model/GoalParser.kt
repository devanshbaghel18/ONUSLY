package com.onusly.app.data.model

import org.json.JSONArray
import org.json.JSONObject

object GoalParser {

    fun parseGoal(obj: JSONObject): Goal {
        val id = obj.optString("id", "")
        val ownerId = obj.optString("ownerId", "")
        val approverId = obj.optString("approverId", "")
        val approverEmail = obj.optString("approverEmail", "")
        val approvalType = obj.optString("approvalType", "none")
        val title = obj.optString("title", "")
        val description = obj.optString("description", "")
        val status = obj.optString("status", "active")
        val createdAt = obj.optString("createdAt", "")
        val updatedAt = obj.optString("updatedAt", "")

        val appsArray = obj.optJSONArray("blockedApps") ?: JSONArray()
        val blockedApps = mutableListOf<String>()
        for (i in 0 until appsArray.length()) {
            val app = appsArray.optString(i, "").trim()
            if (app.isNotEmpty()) {
                blockedApps.add(app)
            }
        }

        val domainsArray = obj.optJSONArray("blockedDomains") ?: JSONArray()
        val blockedDomains = mutableListOf<String>()
        for (i in 0 until domainsArray.length()) {
            val domain = domainsArray.optString(i, "").trim()
            if (domain.isNotEmpty()) {
                blockedDomains.add(domain)
            }
        }

        return Goal(
            id = id,
            ownerId = ownerId,
            approverId = approverId,
            approverEmail = approverEmail,
            approvalType = approvalType,
            title = title,
            description = description,
            status = status,
            createdAt = createdAt,
            updatedAt = updatedAt,
            blockedApps = blockedApps,
            blockedDomains = blockedDomains
        )
    }

    fun parseGoals(jsonString: String): List<Goal> {
        if (jsonString.isBlank()) return emptyList()
        val result = mutableListOf<Goal>()
        try {
            val array = JSONArray(jsonString)
            for (i in 0 until array.length()) {
                val obj = array.optJSONObject(i) ?: continue
                result.add(parseGoal(obj))
            }
        } catch (_: Exception) {
            // Check if root is single object
            try {
                val singleObj = JSONObject(jsonString)
                if (singleObj.has("id")) {
                    result.add(parseGoal(singleObj))
                }
            } catch (_: Exception) {
            }
        }
        return result
    }

    fun createGoalRequestToJson(request: CreateGoalRequest): String {
        val obj = JSONObject()
        obj.put("title", request.title.trim())
        if (request.description.isNotBlank()) {
            obj.put("description", request.description.trim())
        }
        if (!request.approvalType.isNullOrBlank()) {
            obj.put("approvalType", request.approvalType.trim())
        }
        if (!request.approverEmail.isNullOrBlank()) {
            obj.put("approverEmail", request.approverEmail.trim())
        }

        if (request.blockedApps.isNotEmpty()) {
            val appsArray = JSONArray()
            request.blockedApps.forEach { appsArray.put(it.trim()) }
            obj.put("blockedApps", appsArray)
        }

        if (request.blockedDomains.isNotEmpty()) {
            val domainsArray = JSONArray()
            request.blockedDomains.forEach { domainsArray.put(it.trim()) }
            obj.put("blockedDomains", domainsArray)
        }

        return obj.toString()
    }

    fun updateGoalRequestToJson(request: UpdateGoalRequest): String {
        val obj = JSONObject()
        request.title?.let { obj.put("title", it.trim()) }
        request.description?.let { obj.put("description", it.trim()) }
        request.approvalType?.let { obj.put("approvalType", it.trim()) }
        request.approverEmail?.let { obj.put("approverEmail", it.trim()) }

        request.blockedApps?.let { list ->
            val appsArray = JSONArray()
            list.forEach { appsArray.put(it.trim()) }
            obj.put("blockedApps", appsArray)
        }

        request.blockedDomains?.let { list ->
            val domainsArray = JSONArray()
            list.forEach { domainsArray.put(it.trim()) }
            obj.put("blockedDomains", domainsArray)
        }

        return obj.toString()
    }

    fun parseProof(obj: JSONObject): Proof {
        return Proof(
            id = obj.optString("id", ""),
            goalId = obj.optString("goalId", ""),
            ownerId = obj.optString("ownerId", ""),
            proofType = obj.optString("proofType", "text"),
            textExplanation = obj.optString("textExplanation", ""),
            externalLink = obj.optString("externalLink", ""),
            photoUrl = obj.optString("photoUrl", ""),
            submittedAt = obj.optString("submittedAt", "")
        )
    }

    fun parseProofs(jsonString: String): List<Proof> {
        if (jsonString.isBlank()) return emptyList()
        val result = mutableListOf<Proof>()
        try {
            val array = JSONArray(jsonString)
            for (i in 0 until array.length()) {
                val obj = array.optJSONObject(i) ?: continue
                result.add(parseProof(obj))
            }
        } catch (_: Exception) {
            try {
                val singleObj = JSONObject(jsonString)
                if (singleObj.has("id")) {
                    result.add(parseProof(singleObj))
                }
            } catch (_: Exception) {
            }
        }
        return result
    }

    fun createProofRequestToJson(request: CreateProofRequest): String {
        val obj = JSONObject()
        obj.put("proofType", request.proofType.trim())
        if (request.textExplanation.isNotBlank()) {
            obj.put("textExplanation", request.textExplanation.trim())
        }
        if (request.externalLink.isNotBlank()) {
            obj.put("externalLink", request.externalLink.trim())
        }
        if (request.photoUrl.isNotBlank()) {
            obj.put("photoUrl", request.photoUrl.trim())
        }
        return obj.toString()
    }

    fun approvalDecisionRequestToJson(request: ApprovalDecisionRequest): String {
        val obj = JSONObject()
        obj.put("ownerId", request.ownerId.trim())
        obj.put("status", request.status.trim())
        if (request.comment.isNotBlank()) {
            obj.put("comment", request.comment.trim())
        }
        return obj.toString()
    }

    fun parseApprovalResult(jsonString: String): ApprovalResult {
        val obj = JSONObject(jsonString)
        return ApprovalResult(
            id = obj.optString("id", ""),
            goalId = obj.optString("goalId", ""),
            proofId = obj.optString("proofId", ""),
            ownerId = obj.optString("ownerId", ""),
            approverId = obj.optString("approverId", ""),
            status = obj.optString("status", ""),
            comment = obj.optString("comment", ""),
            decidedAt = obj.optString("decidedAt", "")
        )
    }

    fun parseUserProfile(jsonString: String): UserProfile {
        val obj = JSONObject(jsonString)
        return UserProfile(
            id = obj.optString("id", ""),
            email = obj.optString("email", ""),
            handle = obj.optString("handle", ""),
            name = obj.optString("name", ""),
            picture = obj.optString("picture", ""),
            createdAt = obj.optString("createdAt", "")
        )
    }

    fun parsePublicProfile(jsonString: String): PublicProfile {
        val obj = JSONObject(jsonString)
        return PublicProfile(
            id = obj.optString("id", ""),
            name = obj.optString("name", ""),
            handle = obj.optString("handle", ""),
            picture = obj.optString("picture", "")
        )
    }
}
