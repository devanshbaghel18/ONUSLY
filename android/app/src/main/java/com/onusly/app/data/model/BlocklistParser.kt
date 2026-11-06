package com.onusly.app.data.model

import org.json.JSONArray
import org.json.JSONObject

object BlocklistParser {

    fun optSafeString(obj: JSONObject, key: String, fallback: String = ""): String {
        if (!obj.has(key) || obj.isNull(key)) return fallback
        val value = obj.optString(key, fallback)
        return if (value == "null") fallback else value
    }

    fun parseResponse(jsonString: String): BlocklistResponse {
        val root = JSONObject(jsonString)
        val userId = optSafeString(root, "userId")
        val version = optSafeString(root, "version")
        val generatedAt = optSafeString(root, "generatedAt")

        val targetsArray = root.optJSONArray("targets") ?: JSONArray()
        val targetsList = mutableListOf<BlocklistTarget>()

        for (i in 0 until targetsArray.length()) {
            val targetObj = targetsArray.optJSONObject(i) ?: continue
            val goalId = optSafeString(targetObj, "goalId")
            val goalTitle = optSafeString(targetObj, "goalTitle")
            val status = optSafeString(targetObj, "status")

            val appsArray = targetObj.optJSONArray("apps") ?: JSONArray()
            val appsList = mutableListOf<String>()
            for (j in 0 until appsArray.length()) {
                if (appsArray.isNull(j)) continue
                val app = appsArray.optString(j, "").trim()
                if (app.isNotEmpty() && app != "null") {
                    appsList.add(app)
                }
            }

            val domainsArray = targetObj.optJSONArray("domains") ?: JSONArray()
            val domainsList = mutableListOf<String>()
            for (j in 0 until domainsArray.length()) {
                if (domainsArray.isNull(j)) continue
                val domain = domainsArray.optString(j, "").trim()
                if (domain.isNotEmpty() && domain != "null") {
                    domainsList.add(domain)
                }
            }

            targetsList.add(
                BlocklistTarget(
                    goalId = goalId,
                    goalTitle = goalTitle,
                    status = status,
                    apps = appsList,
                    domains = domainsList
                )
            )
        }

        return BlocklistResponse(
            userId = userId,
            version = version,
            generatedAt = generatedAt,
            targets = targetsList
        )
    }

    fun cachedToJson(cached: CachedBlocklist): String {
        val root = JSONObject()
        root.put("userId", cached.userId)
        root.put("version", cached.version)
        root.put("generatedAt", cached.generatedAt)
        root.put("lastSyncTimestamp", cached.lastSyncTimestamp)

        val targetsArray = JSONArray()
        for (target in cached.targets) {
            val targetObj = JSONObject()
            targetObj.put("goalId", target.goalId)
            targetObj.put("goalTitle", target.goalTitle)
            targetObj.put("status", target.status)

            val appsArray = JSONArray()
            for (app in target.apps) {
                appsArray.put(app)
            }
            targetObj.put("apps", appsArray)

            val domainsArray = JSONArray()
            for (domain in target.domains) {
                domainsArray.put(domain)
            }
            targetObj.put("domains", domainsArray)

            targetsArray.put(targetObj)
        }
        root.put("targets", targetsArray)

        return root.toString()
    }

    fun parseCached(jsonString: String): CachedBlocklist? {
        if (jsonString.isBlank()) return null
        return try {
            val root = JSONObject(jsonString)
            val userId = optSafeString(root, "userId")
            val version = optSafeString(root, "version")
            val generatedAt = optSafeString(root, "generatedAt")
            val lastSyncTimestamp = root.optLong("lastSyncTimestamp", 0L)

            val targetsArray = root.optJSONArray("targets") ?: JSONArray()
            val targetsList = mutableListOf<BlocklistTarget>()

            for (i in 0 until targetsArray.length()) {
                val targetObj = targetsArray.optJSONObject(i) ?: continue
                val goalId = optSafeString(targetObj, "goalId")
                val goalTitle = optSafeString(targetObj, "goalTitle")
                val status = optSafeString(targetObj, "status")

                val appsArray = targetObj.optJSONArray("apps") ?: JSONArray()
                val appsList = mutableListOf<String>()
                for (j in 0 until appsArray.length()) {
                    if (appsArray.isNull(j)) continue
                    val app = appsArray.optString(j, "").trim()
                    if (app.isNotEmpty() && app != "null") {
                        appsList.add(app)
                    }
                }

                val domainsArray = targetObj.optJSONArray("domains") ?: JSONArray()
                val domainsList = mutableListOf<String>()
                for (j in 0 until domainsArray.length()) {
                    if (domainsArray.isNull(j)) continue
                    val domain = domainsArray.optString(j, "").trim()
                    if (domain.isNotEmpty() && domain != "null") {
                        domainsList.add(domain)
                    }
                }

                targetsList.add(
                    BlocklistTarget(
                        goalId = goalId,
                        goalTitle = goalTitle,
                        status = status,
                        apps = appsList,
                        domains = domainsList
                    )
                )
            }

            CachedBlocklist(
                userId = userId,
                version = version,
                generatedAt = generatedAt,
                lastSyncTimestamp = lastSyncTimestamp,
                targets = targetsList
            )
        } catch (e: Exception) {
            null
        }
    }
}
