package com.onusly.app.data.api

import com.onusly.app.data.model.BlocklistParser
import com.onusly.app.data.model.SyncResult
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

class EnforcementApiClient(
    private val httpClient: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(10, TimeUnit.SECONDS)
        .build()
) {

    suspend fun fetchBlocklist(
        baseUrl: String,
        jwtToken: String,
        ifNoneMatch: String? = null
    ): SyncResult = withContext(Dispatchers.IO) {
        try {
            val url = "${baseUrl.trimEnd('/')}/enforcement/blocklist"
            val requestBuilder = Request.Builder()
                .url(url)
                .addHeader("Authorization", "Bearer $jwtToken")

            if (!ifNoneMatch.isNullOrBlank()) {
                requestBuilder.addHeader("If-None-Match", ifNoneMatch)
            }

            val request = requestBuilder.build()
            httpClient.newCall(request).execute().use { response ->
                when (response.code) {
                    200 -> {
                        val bodyString = response.body?.string() ?: ""
                        if (bodyString.isBlank()) {
                            SyncResult.Error(200, "Empty response body from blocklist endpoint")
                        } else {
                            try {
                                val parsed = BlocklistParser.parseResponse(bodyString)
                                val etagHeader = response.header("ETag")?.trim('"', ' ')
                                val finalVersion = if (parsed.version.isNotBlank()) parsed.version else (etagHeader ?: "")
                                val finalResponse = parsed.copy(version = finalVersion)
                                SyncResult.Success(finalResponse)
                            } catch (e: Exception) {
                                SyncResult.Error(200, "Failed to parse blocklist response: ${e.message}")
                            }
                        }
                    }
                    304 -> {
                        SyncResult.NotModified
                    }
                    else -> {
                        val errorBody = response.body?.string() ?: ""
                        SyncResult.Error(response.code, "Backend returned HTTP ${response.code}: $errorBody")
                    }
                }
            }
        } catch (e: Exception) {
            SyncResult.Error(-1, e.message ?: "Network error")
        }
    }

    suspend fun sendHeartbeat(
        baseUrl: String,
        jwtToken: String,
        deviceId: String,
        clientVersion: String = "1.0.0",
        enforcing: Boolean = true,
        blocklistVersion: String = "",
        issues: List<String> = emptyList()
    ): Boolean = withContext(Dispatchers.IO) {
        try {
            val url = "${baseUrl.trimEnd('/')}/enforcement/heartbeat"
            val issuesJson = issues.joinToString(separator = ",", prefix = "[", postfix = "]") { "\"$it\"" }
            val sentAt = java.time.format.DateTimeFormatter.ISO_INSTANT.format(java.time.Instant.now())
            val jsonPayload = """
                {
                    "deviceId": "$deviceId",
                    "clientType": "android",
                    "platform": "android",
                    "clientVersion": "$clientVersion",
                    "enforcing": $enforcing,
                    "blocklistVersion": "$blocklistVersion",
                    "issues": $issuesJson,
                    "sentAt": "$sentAt"
                }
            """.trimIndent()
            val requestBody = jsonPayload.toRequestBody("application/json".toMediaType())
            val request = Request.Builder()
                .url(url)
                .addHeader("Authorization", "Bearer $jwtToken")
                .post(requestBody)
                .build()
            httpClient.newCall(request).execute().use { it.isSuccessful }
        } catch (_: Exception) {
            false
        }
    }
}
