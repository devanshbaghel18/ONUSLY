package com.onusly.app.data.model

import org.json.JSONArray
import org.json.JSONObject

object GoalParser {

    fun optSafeString(obj: JSONObject, key: String, fallback: String = ""): String {
        if (!obj.has(key) || obj.isNull(key)) return fallback
        val value = obj.optString(key, fallback)
        return if (value == "null") fallback else value
    }

    private fun parseApplicationTargets(obj: JSONObject): List<ApplicationTarget> {
        val appTargetsArray = obj.optJSONArray("applicationTargets") ?: return emptyList()
        val appTargetsList = mutableListOf<ApplicationTarget>()
        for (i in 0 until appTargetsArray.length()) {
            val atObj = appTargetsArray.optJSONObject(i) ?: continue
            val kind = optSafeString(atObj, "kind")
            val identifier = optSafeString(atObj, "identifier")
            if (kind.isNotBlank() && identifier.isNotBlank()) {
                appTargetsList.add(ApplicationTarget(kind = kind.trim(), identifier = identifier.trim()))
            }
        }
        return appTargetsList
    }

    fun parseGoal(obj: JSONObject): Goal {
        val id = optSafeString(obj, "id")
        val ownerId = optSafeString(obj, "ownerId")
        val approverId = optSafeString(obj, "approverId")
        val approverEmail = optSafeString(obj, "approverEmail")
        val approvalType = optSafeString(obj, "approvalType", "none")
        val title = optSafeString(obj, "title")
        val description = optSafeString(obj, "description")
        val status = optSafeString(obj, "status", "active")
        val createdAt = optSafeString(obj, "createdAt")
        val updatedAt = optSafeString(obj, "updatedAt")

        val appsArray = obj.optJSONArray("blockedApps") ?: JSONArray()
        val blockedApps = mutableListOf<String>()
        for (i in 0 until appsArray.length()) {
            if (appsArray.isNull(i)) continue
            val app = appsArray.optString(i, "").trim()
            if (app.isNotEmpty() && app != "null") {
                blockedApps.add(app)
            }
        }

        val domainsArray = obj.optJSONArray("blockedDomains") ?: JSONArray()
        val blockedDomains = mutableListOf<String>()
        for (i in 0 until domainsArray.length()) {
            if (domainsArray.isNull(i)) continue
            val domain = domainsArray.optString(i, "").trim()
            if (domain.isNotEmpty() && domain != "null") {
                blockedDomains.add(domain)
            }
        }

        val applicationTargets = parseApplicationTargets(obj)

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
            blockedDomains = blockedDomains,
            applicationTargets = applicationTargets
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

        if (request.applicationTargets.isNotEmpty()) {
            val appTargetsArray = JSONArray()
            for (at in request.applicationTargets) {
                val atObj = JSONObject()
                atObj.put("kind", at.kind.trim())
                atObj.put("identifier", at.identifier.trim())
                appTargetsArray.put(atObj)
            }
            obj.put("applicationTargets", appTargetsArray)
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

        request.applicationTargets?.let { list ->
            val appTargetsArray = JSONArray()
            for (at in list) {
                val atObj = JSONObject()
                atObj.put("kind", at.kind.trim())
                atObj.put("identifier", at.identifier.trim())
                appTargetsArray.put(atObj)
            }
            obj.put("applicationTargets", appTargetsArray)
        }

        return obj.toString()
    }

    fun parseProof(obj: JSONObject): Proof {
        return Proof(
            id = optSafeString(obj, "id"),
            goalId = optSafeString(obj, "goalId"),
            ownerId = optSafeString(obj, "ownerId"),
            proofType = optSafeString(obj, "proofType", "text"),
            textExplanation = optSafeString(obj, "textExplanation"),
            externalLink = optSafeString(obj, "externalLink"),
            photoUrl = optSafeString(obj, "photoUrl"),
            submittedAt = optSafeString(obj, "submittedAt")
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
        val status = when {
            obj.has("status") && !obj.isNull("status") && obj.optString("status").isNotBlank() -> obj.optString("status")
            obj.has("decision") && !obj.isNull("decision") && obj.optString("decision").isNotBlank() -> obj.optString("decision")
            else -> ""
        }
        return ApprovalResult(
            id = optSafeString(obj, "id"),
            goalId = optSafeString(obj, "goalId"),
            proofId = optSafeString(obj, "proofId"),
            ownerId = optSafeString(obj, "ownerId"),
            approverId = optSafeString(obj, "approverId"),
            status = status,
            comment = optSafeString(obj, "comment"),
            decidedAt = optSafeString(obj, "decidedAt")
        )
    }

    fun parseApprovalResult(obj: JSONObject): ApprovalResult {
        val status = when {
            obj.has("status") && !obj.isNull("status") && obj.optString("status").isNotBlank() -> obj.optString("status")
            obj.has("decision") && !obj.isNull("decision") && obj.optString("decision").isNotBlank() -> obj.optString("decision")
            else -> ""
        }
        return ApprovalResult(
            id = optSafeString(obj, "id"),
            goalId = optSafeString(obj, "goalId"),
            proofId = optSafeString(obj, "proofId"),
            ownerId = optSafeString(obj, "ownerId"),
            approverId = optSafeString(obj, "approverId"),
            status = status,
            comment = optSafeString(obj, "comment"),
            decidedAt = optSafeString(obj, "decidedAt")
        )
    }

    fun parseUserProfile(jsonString: String): UserProfile {
        val obj = JSONObject(jsonString)
        return UserProfile(
            id = optSafeString(obj, "id"),
            email = optSafeString(obj, "email"),
            handle = optSafeString(obj, "handle"),
            name = optSafeString(obj, "name"),
            picture = optSafeString(obj, "picture"),
            createdAt = optSafeString(obj, "createdAt")
        )
    }

    fun parsePublicProfile(jsonString: String): PublicProfile {
        val obj = JSONObject(jsonString)
        return PublicProfile(
            id = optSafeString(obj, "id"),
            name = optSafeString(obj, "name"),
            handle = optSafeString(obj, "handle"),
            picture = optSafeString(obj, "picture")
        )
    }

    fun optStringList(obj: JSONObject, key: String): List<String> {
        val array = obj.optJSONArray(key) ?: return emptyList()
        val result = mutableListOf<String>()
        for (i in 0 until array.length()) {
            if (array.isNull(i)) continue
            val item = array.optString(i, "").trim()
            if (item.isNotEmpty() && item != "null") {
                result.add(item)
            }
        }
        return result
    }

    fun parseApproverInboxItem(obj: JSONObject): ApproverInboxItem {
        val latestProofObj = obj.optJSONObject("latestProof")
        val latestProof = if (latestProofObj != null) parseProof(latestProofObj) else null
        return ApproverInboxItem(
            goalId = optSafeString(obj, "goalId"),
            ownerId = optSafeString(obj, "ownerId"),
            ownerName = optSafeString(obj, "ownerName"),
            ownerHandle = optSafeString(obj, "ownerHandle"),
            ownerEmail = optSafeString(obj, "ownerEmail"),
            title = optSafeString(obj, "title"),
            description = optSafeString(obj, "description"),
            status = optSafeString(obj, "status"),
            createdAt = optSafeString(obj, "createdAt"),
            updatedAt = optSafeString(obj, "updatedAt"),
            blockedApps = optStringList(obj, "blockedApps"),
            blockedDomains = optStringList(obj, "blockedDomains"),
            applicationTargets = parseApplicationTargets(obj),
            latestProof = latestProof,
            proofCount = obj.optInt("proofCount", if (latestProof != null) 1 else 0)
        )
    }

    fun parseApproverInboxResponse(jsonString: String): ApproverInboxResponse {
        val obj = JSONObject(jsonString)
        val itemsArray = obj.optJSONArray("items") ?: JSONArray()
        val items = mutableListOf<ApproverInboxItem>()
        for (i in 0 until itemsArray.length()) {
            val itemObj = itemsArray.optJSONObject(i) ?: continue
            items.add(parseApproverInboxItem(itemObj))
        }
        val total = obj.optInt("total", items.size)
        return ApproverInboxResponse(items = items, total = total)
    }

    fun parseUploadedFileItem(obj: JSONObject): UploadedFileItem {
        return UploadedFileItem(
            id = optSafeString(obj, "id"),
            name = optSafeString(obj, "name"),
            size = obj.optLong("size", 0L),
            formattedSize = optSafeString(obj, "formattedSize"),
            type = optSafeString(obj, "type"),
            url = optSafeString(obj, "url"),
            dataUrl = optSafeString(obj, "dataUrl"),
            isImage = obj.optBoolean("isImage", true),
            isPdf = obj.optBoolean("isPdf", false)
        )
    }

    fun parseUploadedFileList(jsonString: String): List<UploadedFileItem> {
        val result = mutableListOf<UploadedFileItem>()
        try {
            val array = JSONArray(jsonString)
            for (i in 0 until array.length()) {
                val obj = array.optJSONObject(i) ?: continue
                result.add(parseUploadedFileItem(obj))
            }
        } catch (_: Exception) {
            try {
                val singleObj = JSONObject(jsonString)
                if (singleObj.has("url")) {
                    result.add(parseUploadedFileItem(singleObj))
                }
            } catch (_: Exception) {}
        }
        return result
    }
}
